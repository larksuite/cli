// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	iagents "github.com/larksuite/cli/internal/agents"
	"github.com/larksuite/cli/internal/appmeta"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

// This file implements the scope preflight: between provider resolution and the
// real API call, the session's scopes are checked against what the provider
// declares for the RESOLVED identity (IdentitySpec.Scopes). It is all-or-nothing.
// User scopes are read locally from the credential cache; bot scopes are the
// app's published TenantScopes, fetched best-effort (a failed fetch downgrades to
// a no-op). An identity declaring no scopes skips the check entirely. A miss is a
// missing_scope permission error (exit 3) with a remediation hint, instead of a
// round-trip API 99991679. --dry-run never reaches here.

// storedUserScopes is the token-scope read seam: it returns the granted scope
// list of the stored user token from the LOCAL credential cache (keychain via
// GetStoredToken — same read path as `auth check`), issuing no network
// request. nil/empty means "no usable local scope list" and the caller skips
// preflight. Tests swap it so no unit test touches the real keychain.
var storedUserScopes = func(f *cmdutil.Factory) []string {
	if f == nil || f.Config == nil {
		return nil
	}
	config, err := f.Config()
	if err != nil || config == nil || config.UserOpenId == "" {
		return nil
	}
	stored := larkauth.GetStoredToken(config.AppID, config.UserOpenId)
	if stored == nil {
		return nil
	}
	return strings.Fields(stored.Scope)
}

// preflightInput is the pure input of preflightScopes, so the check itself is
// unit-testable without a Factory, keychain, or provider client. Required is
// the scope set already resolved for Identity (IdentitySpec.Scopes).
type preflightInput struct {
	Identity    core.Identity
	TokenScopes []string
	Required    []string
}

// preflightScopes runs the local scope check, returning nil when it does not
// apply (an unreadable/empty scope list is the downstream not_configured logic's
// business). On any missing scope it returns missing_scope (exit 3) carrying all
// of them.
//
// The hint lists ONLY the missing scopes: the open platform authorizes
// incrementally, so re-login with just those keeps every existing grant and
// re-requesting them would be redundant.
func preflightScopes(in preflightInput) error {
	// No usable scope list → skip (user not logged in, or bot has no published
	// version / the fetch failed); the downstream not_configured / API error owns
	// that path.
	if len(in.TokenScopes) == 0 {
		return nil
	}
	// Only user / bot carry a scope-list concept.
	if in.Identity != core.AsUser && !in.Identity.IsBot() {
		return nil
	}

	granted := make(map[string]bool, len(in.TokenScopes))
	for _, s := range in.TokenScopes {
		granted[s] = true
	}

	var missing []string
	for _, scope := range in.Required {
		if !granted[scope] {
			missing = append(missing, scope)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)

	return errs.NewPermissionError(errs.SubtypeMissingScope,
		"the current %s identity is missing scopes this command needs: %s", in.Identity, strings.Join(missing, ", ")).
		WithIdentity(string(in.Identity)).
		WithMissingScopes(missing...).
		WithHint("%s", scopeRemediationHint(in.Identity, missing))
}

// scopeRemediationHint returns an identity-appropriate fix for the missing
// scopes, mirroring cmd/event's scopeRemediationHint split:
//   - user: re-login requesting ONLY the missing scopes — the open platform
//     authorizes incrementally, so previously-granted scopes are preserved (no
//     merge needed).
//   - bot: the tenant token's scopes come from the app's published version, so
//     the fix is to add the scopes to the app in the developer console and
//     re-publish — not a per-token re-auth. (event additionally offers a
//     one-click scan-to-enable deep link; that generator lives in cmd/event and
//     is not duplicated here.)
func scopeRemediationHint(id core.Identity, missing []string) string {
	if id.IsBot() {
		return fmt.Sprintf(
			"the bot (tenant) token's scopes come from the app's published version — add these scopes to the app in the developer console and re-publish: %s",
			strings.Join(missing, " "))
	}
	// Canonical repo-wide auth login --scope remediation phrasing (see
	// cmd/event, shortcuts/*). Only the missing scopes are listed — the open
	// platform authorizes incrementally, so existing grants are preserved.
	return fmt.Sprintf(
		"run `lark-cli auth login --scope \"%s\"` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login.",
		strings.Join(missing, " "))
}

// preflightScopesForRef is the ref-addressed wrapper: it parses ref for its
// scheme and delegates to preflightScopesForScheme. An unparsable ref yields nil
// — the preflight is an accelerator, never a new failure mode; the paths that
// validate ref/scheme for real have already run inside resolveSpec.
func preflightScopesForRef(f *cmdutil.Factory, id core.Identity, ref string) error {
	r, err := iagents.ParseRef(ref)
	if err != nil {
		return nil //nolint:nilerr // preflight is best-effort: resolveSpec already surfaced any real ref error
	}
	return preflightScopesForScheme(f, id, r.Scheme)
}

// preflightScopesForScheme is the scheme-keyed core of the preflight, shared by
// the ref-addressed verbs (via preflightScopesForRef) and the online
// `agents list <scheme>` enumeration, which has no agent_id. It resolves the
// provider registration for the scheme, resolves the required set declared for
// the resolved identity (IdentitySpec.Scopes), reads the granted scopes through
// the identity-appropriate seam, and runs the all-or-nothing check. Any gap in
// its own inputs (nil Factory, unregistered scheme, no scopes declared for the
// identity) yields nil — for BOT that early exit also skips the app-version
// fetch, so an identity with no declared scopes costs no network.
func preflightScopesForScheme(f *cmdutil.Factory, id core.Identity, scheme string) error {
	if f == nil {
		return nil
	}
	prov, ok := iagents.Info(scheme)
	if !ok {
		return nil
	}

	var required, tokenScopes []string
	switch {
	case id == core.AsUser:
		required = prov.ScopesForIdentity(iagents.IdentityUser)
		if len(required) == 0 {
			return nil // no scopes to check (the provider declares none for this identity)
		}
		tokenScopes = storedUserScopes(f) // local keychain read, no network
	case id.IsBot():
		required = prov.ScopesForIdentity(iagents.IdentityBot)
		if len(required) == 0 {
			return nil
		}
		tokenScopes = botTenantScopes(f) // best-effort app-version fetch
	default:
		return nil
	}
	return preflightScopes(preflightInput{Identity: id, TokenScopes: tokenScopes, Required: required})
}

// botTenantScopes is the bot-scope read seam: it fetches the app's
// currently-published version and returns its TenantScopes (the scopes a tenant
// token actually carries). Any failure — no client, no published version,
// network / appmeta error — yields nil so the caller skips the check (weak
// dependency, mirroring event's console precheck downgrade). Tests swap it so no
// unit test touches the network.
var botTenantScopes = func(f *cmdutil.Factory) []string {
	if f == nil || f.Config == nil {
		return nil
	}
	config, err := f.Config()
	if err != nil || config == nil || config.AppID == "" {
		return nil
	}
	apiClient, err := f.NewAPIClient()
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	appVer, err := appmeta.FetchCurrentPublished(ctx, &appmetaBotClient{client: apiClient}, config.AppID)
	if err != nil || appVer == nil {
		return nil
	}
	return appVer.TenantScopes
}

// appmetaBotClient adapts *client.APIClient to appmeta's APIClient shape under a
// pinned bot identity (/app_versions is app-level and rejects UAT). It returns
// the raw JSON body for appmeta to project; any non-typed transport error is
// classified so callers only see typed errs.* values (though botTenantScopes
// treats every error as a no-op anyway).
type appmetaBotClient struct{ client *client.APIClient }

func (c *appmetaBotClient) CallAPI(ctx context.Context, method, path string, body interface{}) (json.RawMessage, error) {
	resp, err := c.client.DoAPI(ctx, client.RawApiRequest{Method: method, URL: path, Data: body, As: core.AsBot})
	if err != nil {
		if _, ok := errs.ProblemOf(err); ok {
			return nil, err
		}
		return nil, errs.NewNetworkError(errs.SubtypeNetworkTransport, "api %s %s: %s", method, path, err).WithCause(err)
	}
	return json.RawMessage(resp.RawBody), nil
}
