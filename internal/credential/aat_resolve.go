// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"context"
	"fmt"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
)

// resolveAgentAccess produces a usable Agent Employee credential for the
// current profile, renewing it when the cached one is spent.
//
// An Agent Employee token has no refresh token: renewal means minting a fresh
// one and re-verifying, through Introspection, that it still belongs to the
// principal this profile was bound to. That re-verification is what stops a
// silent rebinding on the platform side from turning into the CLI acting as a
// different user under the same profile name.
func (p *DefaultTokenProvider) resolveAgentAccess(ctx context.Context, acct *Account) (*ResolvedAccess, error) {
	if acct == nil || acct.AppID == "" || acct.PrincipalOpenID == "" {
		return nil, errs.NewConfigError(errs.SubtypeInvalidConfig,
			"Agent Employee profile has no verified principal identity").
			WithHint("Rerun `lark-cli config init` with the Agent Employee app credentials.")
	}

	resolved, err := auth.ResolveStoredAAT(
		ctx,
		p.defaultAcct.keychain(),
		acct.AppID,
		acct.PrincipalOpenID,
		time.Now,
		p.newAgentToken(acct),
	)
	if err != nil {
		return nil, err
	}

	access := &ResolvedAccess{
		Value:     resolved.Token.AccessToken,
		Kind:      core.CredentialAAT,
		OpenID:    resolved.Token.UserOpenID,
		ExpiresAt: time.UnixMilli(resolved.Token.ExpiresAt),
		// The token endpoint reports the grant the local pre-check compares against.
		Scopes:             resolved.Token.Scope,
		Source:             core.CredentialSource("local"),
		VerificationSource: VerificationLocalIntrospection,
		CacheState:         AccessCacheHit,
	}
	if resolved.Refreshed {
		access.CacheState = AccessCacheRefreshed
	}
	if resolved.RefreshError != nil {
		access.CacheState = AccessCacheStale
		access.RefreshWarning = resolved.RefreshError
		p.warnAgentRefreshFailed(resolved.Token.ExpiresAt)
	}
	return access, nil
}

// newAgentToken returns the mint-and-verify step handed to the cache. It is
// only ever called while the cache holds the app-scoped lock, so at most one
// process mints for a given app at a time.
func (p *DefaultTokenProvider) newAgentToken(acct *Account) auth.AATRefreshFunc {
	return func(ctx context.Context) (*auth.StoredAAToken, error) {
		httpClient, err := p.httpClient()
		if err != nil {
			return nil, errs.NewInternalError(errs.SubtypeSDKError,
				"failed to initialize OAuth client").WithCause(err)
		}
		minted, introspection, err := InspectClientCredentials(
			ctx, httpClient, acct.Brand, acct.AppID, acct.AppSecret)
		if err != nil {
			return nil, err
		}
		if !introspection.IsAgentEmployee() {
			return nil, errs.NewValidationError(errs.SubtypeFailedPrecondition,
				"the configured app no longer resolves to an Agent Employee").
				WithHint("Check the Agent Employee binding and rerun `lark-cli config init`.")
		}
		if introspection.OpenID != acct.PrincipalOpenID {
			return nil, errs.NewValidationError(errs.SubtypeFailedPrecondition,
				"the Agent Employee principal changed").
				WithHint("Rerun `lark-cli config init` to verify and bind the current Agent Employee.")
		}
		return &auth.StoredAAToken{
			AppID:       acct.AppID,
			UserOpenID:  introspection.OpenID,
			AccessToken: minted.Token,
			ExpiresAt:   minted.ExpiresAt.UnixMilli(),
			GrantedAt:   minted.GrantedAt.UnixMilli(),
			// Whole-record write: omitting Scope here would blank it on renewal.
			Scope: minted.Scope,
		}, nil
	}
}

// warnAgentRefreshFailed tells the user that an early renewal did not go
// through and the command is running on the credential already in hand. It goes
// to stderr so a JSON consumer still gets a clean payload on stdout. The expiry
// is rendered the way whoami and auth status render theirs — RFC3339 in the
// local zone with its offset — so the same instant does not appear in two
// spellings within one session.
func (p *DefaultTokenProvider) warnAgentRefreshFailed(expiresAtMS int64) {
	if p.errOut == nil {
		return
	}
	_, _ = fmt.Fprintf(p.errOut,
		"warning: could not renew the Agent Employee token; continuing with the current one, which expires at %s\n",
		time.UnixMilli(expiresAtMS).Format(time.RFC3339))
}

// WarnAgentScopeSnapshotOnce prints, once per provider, that a scope judgment
// used the bind-time snapshot because the keychain holds no live scope. It lives
// here rather than on CredentialProvider because only this provider is built
// with a real stderr stream in the CLI.
func (p *DefaultTokenProvider) WarnAgentScopeSnapshotOnce() {
	if p.errOut == nil {
		return
	}
	p.scopeSnapshotWarnOnce.Do(func() {
		fmt.Fprintln(p.errOut,
			"warning: no live Agent Employee scope in the keychain; falling back to the "+
				"snapshot recorded at bind time. Rerun `lark-cli config init` if the Agent's "+
				"capabilities have changed")
	})
}
