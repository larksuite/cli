// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/larksuite/cli/errs"
	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
)

// DefaultAccountResolver is implemented by the default account provider.
type DefaultAccountResolver interface {
	ResolveAccount(ctx context.Context) (*Account, error)
}

type dpopPolicyResolver interface {
	ResolveLocalDPoPMode(appID string) core.DPoPMode
}

// DefaultTokenResolver is implemented by the default token provider.
type DefaultTokenResolver interface {
	ResolveToken(ctx context.Context, req TokenSpec) (*TokenResult, error)
}

// DefaultAccessResolver is implemented by the default token provider for the
// credential kinds it owns end to end. Returning handled=false means "this is
// not mine", which sends the request down the pre-existing UAT/TAT path.
type DefaultAccessResolver interface {
	ResolveAccess(ctx context.Context, acct *Account, spec AccessSpec) (*ResolvedAccess, bool, error)
}

// AgentScopePeeker is the optional read-only view a default token provider
// exposes so scope facts can be read without minting or renewing; scope
// resolution also runs on --dry-run. The two warnings live here rather than on
// CredentialProvider because its warnOut is nil in the CLI assembly (see
// buildCredentialProvider) while the default token provider has real stderr.
type AgentScopePeeker interface {
	PeekAgentScope(appID, principalOpenID string) (string, bool)
	// WarnAgentScopeSnapshotOnce reports once that the judgment used the bind-time snapshot; no-op without a writer.
	WarnAgentScopeSnapshotOnce()
	// WarnAgentScopePrecheckOffOnce reports once that the pre-check was turned off; no-op without a writer.
	WarnAgentScopePrecheckOffOnce()
}

var (
	getStoredToken       = auth.GetStoredToken
	getStoredTokenStatus = auth.TokenStatus
	getStoredAAT         = auth.GetStoredAAT
)

type credentialSource interface {
	Name() string
	CredentialSource() string
	TryResolveToken(ctx context.Context, req TokenSpec) (*TokenResult, bool, error)
	// TryResolveAccess returns handled=false when this source has nothing
	// beyond the legacy token flow to offer for the given account.
	TryResolveAccess(ctx context.Context, acct *Account, spec AccessSpec) (*ResolvedAccess, bool, error)
	ResolveIdentityHint(ctx context.Context, acct *Account) (*IdentityHint, error)
}

type extensionTokenSource struct {
	provider extcred.Provider
}

func (s extensionTokenSource) Name() string { return s.provider.Name() }

func (s extensionTokenSource) CredentialSource() string {
	switch s.Name() {
	case "env", "sidecar":
		return s.Name()
	default:
		return "extension"
	}
}

func (s extensionTokenSource) TryResolveToken(ctx context.Context, req TokenSpec) (*TokenResult, bool, error) {
	tok, err := s.provider.ResolveToken(ctx, extcred.TokenSpec{
		Type:  extcred.TokenType(req.Type.String()),
		AppID: req.AppID,
	})
	if err != nil {
		return nil, false, err
	}
	if tok == nil {
		return nil, false, nil
	}
	if tok.Value == "" {
		return nil, false, &MalformedTokenResultError{Source: s.Name(), Type: req.Type, Reason: "empty token"}
	}
	return &TokenResult{Token: tok.Value, Scopes: tok.Scopes, Source: core.CredentialSource(s.CredentialSource())}, true, nil
}

// TryResolveAccess uses the provider's optional access interface when it has
// one. A provider that only implements the legacy interface is left alone, so
// existing extensions keep working without recompilation.
func (s extensionTokenSource) TryResolveAccess(ctx context.Context, acct *Account, spec AccessSpec) (*ResolvedAccess, bool, error) {
	provider, ok := s.provider.(extcred.AccessProvider)
	if !ok {
		return nil, false, nil
	}
	access, err := provider.ResolveAccess(ctx, extcred.AccessSpec{
		Identity: extcred.Identity(spec.Identity),
		AppID:    spec.AppID,
	})
	if err != nil {
		return nil, false, err
	}
	if access == nil {
		// nil means "not handled here": the caller falls back to the token path.
		return nil, false, nil
	}
	return &ResolvedAccess{
		Value:     access.Value,
		Kind:      core.CredentialKind(access.Kind),
		OpenID:    access.OpenID,
		Scopes:    access.Scopes,
		ExpiresAt: access.ExpiresAt,
		Source:    core.CredentialSource(s.CredentialSource()),
		// The provider vouched for this subject; the CLI did not verify it
		// itself, and diagnostics must not imply otherwise.
		VerificationSource: VerificationProviderManaged,
	}, true, nil
}

func (s extensionTokenSource) ResolveIdentityHint(ctx context.Context, acct *Account) (*IdentityHint, error) {
	hint := &IdentityHint{}
	if acct == nil {
		return hint, nil
	}
	hint.DefaultAs = acct.DefaultAs
	if acct.CredentialKind == core.CredentialAAT {
		// An Agent Employee is a user principal and has no bot lane at all, so
		// auto has only one possible answer and needs no probing.
		hint.AutoAs = core.AsUser
		return hint, nil
	}
	// Extension sources verify user identity via enrichUserInfo, so a resolved
	// UserOpenId is sufficient here; no keychain-backed token status lookup is needed.
	if acct.UserOpenId != "" {
		hint.AutoAs = core.AsUser
		return hint, nil
	}
	ids := extcred.IdentitySupport(acct.SupportedIdentities)
	switch {
	case ids.UserOnly():
		hint.AutoAs = core.AsUser
	case ids.BotOnly():
		hint.AutoAs = core.AsBot
	}
	return hint, nil
}

type defaultTokenSource struct {
	resolver DefaultTokenResolver
}

func (s defaultTokenSource) Name() string             { return "default" }
func (s defaultTokenSource) CredentialSource() string { return "local" }

func (s defaultTokenSource) TryResolveToken(ctx context.Context, req TokenSpec) (*TokenResult, bool, error) {
	if s.resolver == nil {
		return nil, false, nil
	}
	result, err := s.resolver.ResolveToken(ctx, req)
	if err != nil {
		return nil, false, err
	}
	if result == nil {
		return nil, false, &MalformedTokenResultError{Source: s.Name(), Type: req.Type, Reason: "nil token result"}
	}
	if result.Token == "" {
		return nil, false, &MalformedTokenResultError{Source: s.Name(), Type: req.Type, Reason: "empty token"}
	}
	resultCopy := *result
	resultCopy.Source = core.CredentialSource(s.CredentialSource())
	return &resultCopy, true, nil
}

// TryResolveAccess delegates to the built-in resolver for the credential kinds
// it owns and declines everything else.
func (s defaultTokenSource) TryResolveAccess(ctx context.Context, acct *Account, spec AccessSpec) (*ResolvedAccess, bool, error) {
	resolver, ok := s.resolver.(DefaultAccessResolver)
	if !ok {
		return nil, false, nil
	}
	return resolver.ResolveAccess(ctx, acct, spec)
}

func (s defaultTokenSource) ResolveIdentityHint(ctx context.Context, acct *Account) (*IdentityHint, error) {
	hint := &IdentityHint{}
	if acct == nil {
		return hint, nil
	}
	hint.DefaultAs = acct.DefaultAs
	if acct.CredentialKind == core.CredentialAAT {
		hint.AutoAs = core.AsUser
		return hint, nil
	}
	if acct.UserOpenId == "" {
		hint.AutoAs = core.AsBot
		return hint, nil
	}
	stored, _ := getStoredToken(acct.AppID, acct.UserOpenId)
	if stored == nil {
		hint.AutoAs = core.AsBot
		return hint, nil
	}
	if getStoredTokenStatus(stored) == "expired" {
		hint.AutoAs = core.AsBot
		return hint, nil
	}
	hint.AutoAs = core.AsUser
	return hint, nil
}

// CredentialProvider is the unified entry point for all credential resolution.
type CredentialProvider struct {
	providers    []extcred.Provider
	defaultAcct  DefaultAccountResolver
	defaultToken DefaultTokenResolver
	httpClient   func() (*http.Client, error)
	warnOut      io.Writer

	accountOnce    sync.Once
	account        *Account
	accountErr     error
	selectedSource credentialSource

	hintOnce sync.Once
	hint     *IdentityHint
	hintErr  error
}

// NewCredentialProvider creates a CredentialProvider.
func NewCredentialProvider(providers []extcred.Provider, defaultAcct DefaultAccountResolver, defaultToken DefaultTokenResolver, httpClient func() (*http.Client, error)) *CredentialProvider {
	return &CredentialProvider{
		providers:    providers,
		defaultAcct:  defaultAcct,
		defaultToken: defaultToken,
		httpClient:   httpClient,
	}
}

func (p *CredentialProvider) SetWarnOut(warnOut io.Writer) *CredentialProvider {
	p.warnOut = warnOut
	return p
}

// ResolveAccount resolves app credentials. Result is cached after first call.
// NOTE: Uses sync.Once — only the context from the first call is used for resolution.
// Subsequent calls return the cached result regardless of their context.
// This is acceptable for CLI (single invocation per process) but not for long-running servers.
func (p *CredentialProvider) ResolveAccount(ctx context.Context) (*Account, error) {
	p.accountOnce.Do(func() {
		p.account, p.accountErr = p.doResolveAccount(ctx)
	})
	return p.account, p.accountErr
}

func (p *CredentialProvider) doResolveAccount(ctx context.Context) (*Account, error) {
	for _, prov := range p.providers {
		acct, err := prov.ResolveAccount(ctx)
		if err != nil {
			return nil, err
		}
		if acct != nil {
			internal := convertAccount(acct)
			source := extensionTokenSource{provider: prov}
			internal.CredentialSource = core.CredentialSource(source.CredentialSource())
			if policy, ok := p.defaultAcct.(dpopPolicyResolver); ok {
				internal.DPoPMode = policy.ResolveLocalDPoPMode(internal.AppID)
			}
			if internal.CredentialKind == core.CredentialAAT {
				// The provider has already verified this Agent Employee's
				// principal. Asking the legacy token path for a user access
				// token here would pull a credential during what callers treat
				// as a read-only account lookup, which is exactly what
				// diagnostics and dry-run rely on not happening.
				p.selectedSource = source
				return internal, nil
			}
			if err := p.enrichUserInfo(ctx, internal, source); err != nil {
				if p.warnOut != nil {
					_, _ = fmt.Fprintf(p.warnOut, "warning: unable to verify user identity from credential source %q: %v\n", source.Name(), err)
				}
				// enrichUserInfo failure is non-fatal: SupportedIdentities
				// (used for strict mode) is already set by the provider.
				// Clear unverified user identity for safety.
				internal.UserOpenId = ""
				internal.UserName = ""
			}
			p.selectedSource = source
			return internal, nil
		}
	}
	if p.defaultAcct != nil {
		acct, err := p.defaultAcct.ResolveAccount(ctx)
		if err != nil {
			return nil, err
		}
		p.selectedSource = defaultTokenSource{resolver: p.defaultToken}
		return acct, nil
	}
	return nil, core.NotConfiguredError()
}

// enrichUserInfo resolves user identity when extension provides a UAT.
// If UAT is available, user_info API call is mandatory (security: verify token validity).
// If no UAT from extension, falls back to provider-supplied OpenID.
func (p *CredentialProvider) enrichUserInfo(ctx context.Context, acct *Account, source credentialSource) error {
	if p.httpClient == nil || source == nil {
		return nil
	}
	tok, found, err := source.TryResolveToken(ctx, TokenSpec{Type: TokenTypeUAT, AppID: acct.AppID})
	if err != nil {
		var blockErr *extcred.BlockError
		if errors.As(err, &blockErr) {
			return nil // provider explicitly blocks UAT; skip enrichment
		}
		return fmt.Errorf("failed to resolve UAT for user identity verification: %w", err)
	}
	if !found {
		return nil
	}
	// Have UAT — must verify and resolve identity
	hc, err := p.httpClient()
	if err != nil {
		return fmt.Errorf("failed to get HTTP client for user_info: %w", err)
	}
	requestCtx := core.WithCredentialSource(ctx, tok.Source)
	info, err := fetchUserInfo(requestCtx, hc, acct.Brand, tok.Token)
	if err != nil {
		return fmt.Errorf("failed to verify user identity: %w", err)
	}
	acct.UserOpenId = info.OpenID
	acct.UserName = info.Name
	return nil
}

func (p *CredentialProvider) selectedCredentialSource(ctx context.Context) (credentialSource, error) {
	if p.selectedSource != nil {
		return p.selectedSource, nil
	}
	if p.defaultAcct == nil {
		return nil, nil
	}
	if _, err := p.ResolveAccount(ctx); err != nil {
		return nil, err
	}
	if p.selectedSource == nil {
		return nil, fmt.Errorf("credential provider resolved an account without selecting a token source")
	}
	return p.selectedSource, nil
}

func (p *CredentialProvider) resolveTokenFromSource(ctx context.Context, source credentialSource, req TokenSpec) (*TokenResult, error) {
	result, found, err := source.TryResolveToken(ctx, req)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, &TokenUnavailableError{Source: source.Name(), Type: req.Type}
	}
	return result, nil
}

// ResolveIdentityHint resolves default/auto identity guidance from the selected source.
// NOTE: Uses sync.Once — only the context from the first call is used for resolution.
// This matches ResolveAccount and keeps identity decisions stable within one CLI invocation.
func (p *CredentialProvider) ResolveIdentityHint(ctx context.Context) (*IdentityHint, error) {
	p.hintOnce.Do(func() {
		p.hint, p.hintErr = p.doResolveIdentityHint(ctx)
	})
	return p.hint, p.hintErr
}

func (p *CredentialProvider) doResolveIdentityHint(ctx context.Context) (*IdentityHint, error) {
	acct, err := p.ResolveAccount(ctx)
	if err != nil {
		return nil, err
	}
	if acct == nil {
		return &IdentityHint{}, nil
	}
	source, err := p.selectedCredentialSource(ctx)
	if err != nil {
		return nil, err
	}
	if source == nil {
		return &IdentityHint{}, nil
	}
	hint, err := source.ResolveIdentityHint(ctx, acct)
	if err != nil {
		return nil, err
	}
	if hint == nil {
		return &IdentityHint{}, nil
	}
	return hint, nil
}

// AccountCredentialKind reports how the active profile is classified, without
// resolving any credential. Identity selection needs this before it can know
// whether bot is even reachable.
//
// A wiring with no account resolver at all returns an empty kind rather than an
// error: that configuration carries no classification, and failing here would
// break token-only setups that never had an account to begin with.
func (p *CredentialProvider) AccountCredentialKind(ctx context.Context) (core.CredentialKind, error) {
	source, _ := p.selectedCredentialSource(ctx)
	if source == nil {
		// Either nothing is configured, or resolving the account failed —
		// selectedCredentialSource reports both as no source. Neither is
		// answerable as a classification, and both surface from the next step,
		// which needs the account for its own reasons. Reporting the failure
		// here instead would turn every classification-dependent gate into a
		// second place that rejects an unconfigured CLI, each with its own
		// wording.
		return "", nil
	}
	// A source is only ever selected by a successful account resolution, and
	// that resolution is memoized, so the call below returns what it already
	// decided and cannot newly fail.
	acct, _ := p.ResolveAccount(ctx)
	if acct == nil {
		return "", nil
	}
	return acct.CredentialKind, nil
}

// ResolveAccess is the single entry point for obtaining a credential that will
// actually be sent. Everything that decides which credential to use lives here:
// call sites declare an identity, the resolved account decides the credential,
// and the contract checks on both sides of the resolution make the pairing
// enforceable rather than conventional.
//
// It is deliberately not used by read-only diagnostics, dry-run or scope
// pre-checks — those must not mint, verify or touch stored credentials.
func (p *CredentialProvider) ResolveAccess(ctx context.Context, spec AccessSpec) (*ResolvedAccess, error) {
	// Resolve the account first: selecting a credential source is a side effect
	// of that resolution, so asking for the source beforehand would always look
	// like the token-only wiring below.
	acct, acctErr := p.ResolveAccount(ctx)
	source, srcErr := p.selectedCredentialSource(ctx)
	if source == nil {
		// Token-only wiring: no source claimed this account, so there is no
		// classification to honour and the pre-existing token chain is the whole
		// story, including the error it reports when nothing is configured.
		if srcErr != nil {
			return nil, srcErr
		}
		return p.resolveUnclassifiedAccess(ctx, spec)
	}
	if acctErr != nil {
		return nil, acctErr
	}
	if err := checkAccountForAccess(acct, spec); err != nil {
		return nil, err
	}

	access, handled, err := source.TryResolveAccess(ctx, acct, spec)
	if err != nil {
		return nil, err
	}
	if !handled {
		if acct.CredentialKind == core.CredentialAAT {
			// Falling back here would mean sending some other credential in
			// place of the Agent Employee's own, so there is no fallback.
			return nil, agentAccessUnsupportedError(source.Name())
		}
		access, err = p.resolveLegacyAccess(ctx, source, spec)
		if err != nil {
			return nil, err
		}
	}
	if err := checkResolvedAccess(acct, spec, access, time.Now()); err != nil {
		return nil, err
	}
	return access, nil
}

// resolveLegacyAccess wraps the pre-existing user/tenant token resolution so
// those two paths reach the same contract checks without their behaviour
// changing.
func (p *CredentialProvider) resolveLegacyAccess(ctx context.Context, source credentialSource, spec AccessSpec) (*ResolvedAccess, error) {
	req := NewTokenSpec(spec.Identity, spec.AppID)
	result, err := p.resolveTokenFromSource(ctx, source, req)
	if err != nil {
		return nil, err
	}
	kind := core.CredentialUAT
	if req.Type == TokenTypeTAT {
		kind = core.CredentialTAT
	}
	return &ResolvedAccess{
		Value:              result.Token,
		DPoP:               result.DPoP,
		Kind:               kind,
		Scopes:             result.Scopes,
		Source:             result.Source,
		VerificationSource: VerificationLegacy,
	}, nil
}

// resolveUnclassifiedAccess covers the wiring where no account resolver exists
// at all. It defers to the pre-existing token chain, including its provider
// walk, so that configuration keeps behaving exactly as it did.
func (p *CredentialProvider) resolveUnclassifiedAccess(ctx context.Context, spec AccessSpec) (*ResolvedAccess, error) {
	req := NewTokenSpec(spec.Identity, spec.AppID)
	result, err := p.ResolveToken(ctx, req)
	if err != nil {
		return nil, err
	}
	kind := core.CredentialUAT
	if req.Type == TokenTypeTAT {
		kind = core.CredentialTAT
	}
	access := &ResolvedAccess{
		Value:              result.Token,
		DPoP:               result.DPoP,
		Kind:               kind,
		Scopes:             result.Scopes,
		Source:             result.Source,
		VerificationSource: VerificationLegacy,
	}
	if err := checkResolvedAccess(nil, spec, access, time.Now()); err != nil {
		return nil, err
	}
	return access, nil
}

// agentAccessUnsupportedError reports a credential source that owns an Agent
// Employee profile but cannot produce an Agent Employee credential.
func agentAccessUnsupportedError(sourceName string) error {
	if sourceName == "" {
		sourceName = "external"
	}
	return errs.NewValidationError(errs.SubtypeFailedPrecondition,
		"credential source %q manages an Agent Employee profile but cannot supply an Agent Employee credential", sourceName).
		WithHint("Update the credential source to support Agent Employee access, or select a profile it can serve.")
}

// ResolveScopeMetadata reports which scopes the current credential carries, for
// pre-checks that run before a command commits to anything.
//
// For an Agent Employee profile it answers from cached facts alone (keychain
// peek, then the bind-time config.json snapshot) and never mints, renews, or
// introspects, because the pre-check also runs on --dry-run. It is temporarily
// off by default; enable with LARKSUITE_CLI_AGENT_SCOPE_PRECHECK=on. User and tenant profiles
// keep the existing token resolution unchanged.
func (p *CredentialProvider) ResolveScopeMetadata(ctx context.Context, spec AccessSpec) (ScopeMetadata, error) {
	acct, acctErr := p.ResolveAccount(ctx)
	source, _ := p.selectedCredentialSource(ctx)
	if source != nil {
		if acctErr != nil {
			return ScopeMetadata{}, acctErr
		}
		if err := checkAccountForAccess(acct, spec); err != nil {
			return ScopeMetadata{}, err
		}
		if acct.CredentialKind == core.CredentialAAT {
			peeker, hasPeeker := p.defaultToken.(AgentScopePeeker)
			if !agentScopePrecheckEnabled() {
				if hasPeeker {
					peeker.WarnAgentScopePrecheckOffOnce()
				}
				return ScopeMetadata{Available: false, NotApplicable: true}, nil
			}
			// Never fall through to ResolveToken: its user path refuses an Agent Employee.
			if hasPeeker {
				if s, found := peeker.PeekAgentScope(acct.AppID, acct.PrincipalOpenID); found && s != "" {
					return ScopeMetadata{Scopes: s, Available: true}, nil
				}
			}
			if acct.AgentScope != "" {
				if hasPeeker {
					peeker.WarnAgentScopeSnapshotOnce()
				}
				return ScopeMetadata{Scopes: acct.AgentScope, Available: true}, nil
			}
			return ScopeMetadata{Available: false, NotApplicable: true}, nil
		}
	}
	result, err := p.ResolveToken(ctx, NewTokenSpec(spec.Identity, spec.AppID))
	if err != nil {
		return ScopeMetadata{}, err
	}
	if result == nil || result.Scopes == "" {
		return ScopeMetadata{Available: false}, nil
	}
	return ScopeMetadata{Scopes: result.Scopes, Available: true}, nil
}

// ResolveToken resolves an access token.
func (p *CredentialProvider) ResolveToken(ctx context.Context, req TokenSpec) (*TokenResult, error) {
	source, err := p.selectedCredentialSource(ctx)
	if err != nil {
		return nil, err
	}
	if source != nil {
		return p.resolveTokenFromSource(ctx, source, req)
	}

	for _, prov := range p.providers {
		source := extensionTokenSource{provider: prov}
		result, found, err := source.TryResolveToken(ctx, req)
		if err != nil {
			return nil, err
		}
		if found {
			return result, nil
		}
	}
	source = defaultTokenSource{resolver: p.defaultToken}
	result, found, err := source.TryResolveToken(ctx, req)
	if err != nil {
		return nil, err
	}
	if found {
		return result, nil
	}
	return nil, &TokenUnavailableError{Type: req.Type}
}

// ActiveExtensionProviderName reports whether an extension provider is managing
// credentials. It probes p.providers (extension providers only, not defaultAcct)
// and returns the name of the first engaged provider.
//
// "Engaged" means: ResolveAccount returns a non-nil account, OR returns a
// *extcred.BlockError (provider configured but misconfigured — still counts as
// external). Any other error is propagated to the caller.
//
// Returns ("", nil) when no extension provider is active (built-in keychain path).
// Safe to call multiple times — probes providers directly without the sync.Once cache.
func (p *CredentialProvider) ActiveExtensionProviderName(ctx context.Context) (string, error) {
	for _, prov := range p.providers {
		acct, err := prov.ResolveAccount(ctx)
		if err != nil {
			var blockErr *extcred.BlockError
			if errors.As(err, &blockErr) {
				name := blockErr.Provider
				if name == "" {
					name = prov.Name()
				}
				if name == "" {
					name = "external"
				}
				return name, nil
			}
			return "", err
		}
		if acct != nil {
			if name := prov.Name(); name != "" {
				return name, nil
			}
			return "external", nil
		}
	}
	return "", nil
}

func convertAccount(ext *extcred.Account) *Account {
	acct := &Account{
		AppID:               ext.AppID,
		AppSecret:           ext.AppSecret,
		Brand:               core.LarkBrand(ext.Brand),
		DefaultAs:           core.Identity(ext.DefaultAs),
		ProfileName:         ext.ProfileName,
		UserOpenId:          ext.OpenID,
		SupportedIdentities: uint8(ext.SupportedIdentities),
		CredentialKind:      core.CredentialKind(ext.CredentialKind),
	}
	// An Agent Employee account carries its principal in OpenID; mirror it into
	// PrincipalOpenID so the contract checks read the same field for local and
	// external accounts instead of branching on where the account came from.
	if acct.CredentialKind == core.CredentialAAT {
		acct.PrincipalOpenID = ext.OpenID
	}
	return acct
}
