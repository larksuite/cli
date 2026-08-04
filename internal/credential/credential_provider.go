// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/larksuite/cli/errs"
	extcred "github.com/larksuite/cli/extension/credential"
	envprovider "github.com/larksuite/cli/extension/credential/env"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
)

// DefaultAccountResolver is implemented by the default account provider.
type DefaultAccountResolver interface {
	ResolveAccount(ctx context.Context) (*Account, error)
}

// DefaultTokenResolver is implemented by the default token provider.
type DefaultTokenResolver interface {
	ResolveToken(ctx context.Context, req TokenSpec) (*TokenResult, error)
}

var (
	getStoredToken       = auth.GetStoredToken
	getStoredTokenStatus = auth.TokenStatus
)

type credentialSource interface {
	Name() string
	TryResolveToken(ctx context.Context, req TokenSpec) (*TokenResult, bool, error)
	ResolveIdentityHint(ctx context.Context, acct *Account) (*IdentityHint, error)
}

type extensionTokenSource struct {
	provider extcred.Provider
}

func (s extensionTokenSource) Name() string { return s.provider.Name() }

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
	return &TokenResult{Token: tok.Value, Scopes: tok.Scopes}, true, nil
}

func (s extensionTokenSource) ResolveIdentityHint(ctx context.Context, acct *Account) (*IdentityHint, error) {
	hint := &IdentityHint{}
	if acct == nil {
		return hint, nil
	}
	hint.DefaultAs = acct.DefaultAs
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

func (s defaultTokenSource) Name() string { return "default" }

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
	return result, true, nil
}

func (s defaultTokenSource) ResolveIdentityHint(ctx context.Context, acct *Account) (*IdentityHint, error) {
	hint := &IdentityHint{}
	if acct == nil {
		return hint, nil
	}
	hint.DefaultAs = acct.DefaultAs
	if acct.UserOpenId == "" {
		hint.AutoAs = core.AsBot
		return hint, nil
	}
	stored := getStoredToken(acct.AppID, acct.UserOpenId)
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

	// profile is the active profile (from --profile or LARKSUITE_CLI_PROFILE);
	// profileSrc records which of the two supplied it, for the reported
	// selection and error attribution.
	profile    string
	profileSrc CredentialSourceKind

	accountOnce    sync.Once
	account        *Account
	accountErr     error
	selectedSource credentialSource
	// selection is the explainable credential-selection result, populated by
	// doResolveAccount under accountOnce. It never carries a secret.
	selection IdentitySelection

	enrichOnce sync.Once

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

// WithProfileFromFlag records the --profile flag value as the active profile.
// It governs credential arbitration and the reported selection source.
func (p *CredentialProvider) WithProfileFromFlag(profile string) *CredentialProvider {
	p.profile = profile
	p.profileSrc = SourceFlagProfile
	return p
}

// WithProfileFromEnv records the LARKSUITE_CLI_PROFILE env fallback as the
// active profile. It governs credential arbitration and the reported
// selection source.
func (p *CredentialProvider) WithProfileFromEnv(profile string) *CredentialProvider {
	p.profile = profile
	p.profileSrc = SourceEnvProfile
	return p
}

// ResolveAccount resolves app credentials. Result is cached after first call.
// NOTE: Uses sync.Once — only the context from the first call is used for resolution.
// Subsequent calls return the cached result regardless of their context.
// This is acceptable for CLI (single invocation per process) but not for long-running servers.
func (p *CredentialProvider) ResolveAccount(ctx context.Context) (*Account, error) {
	acct, err := p.resolveAccountSelection(ctx)
	if err != nil || acct == nil {
		return acct, err
	}
	if _, ok := p.selectedSource.(extensionTokenSource); ok {
		p.enrichOnce.Do(func() {
			p.enrichOrClearIdentity(ctx, acct, p.selectedSource)
		})
	}
	return acct, nil
}

// resolveAccountSelection performs and caches only credential selection. It
// deliberately does not resolve tokens or user_info, so callers can validate
// the selected app before any token work begins.
func (p *CredentialProvider) resolveAccountSelection(ctx context.Context) (*Account, error) {
	p.accountOnce.Do(func() {
		p.account, p.accountErr = p.doResolveAccount(ctx)
	})
	return p.account, p.accountErr
}

// doResolveAccount arbitrates the credential/App selection in three phases:
// gather all arbitration inputs in a single I/O pass, decide the route with a
// pure function, then execute the remaining I/O for the chosen route.
//
// Resolution order (encoded in decideIdentity): a managed extension provider
// (e.g. sidecar) wins outright; then an explicit profile (--profile /
// LARKSUITE_CLI_PROFILE) arbitrates against the direct env credential
// (matching app_id → profile supplies credential and tokens; mismatch → hard
// conflict; incomplete env without a usable app_id → repair error); then a
// complete direct env credential; then the config default (currentApp →
// firstApp).
//
// It populates p.selection (never carries a secret) and p.selectedSource on
// every success path.
func (p *CredentialProvider) doResolveAccount(ctx context.Context) (*Account, error) {
	in, err := p.gatherIdentityInputs(ctx)
	if err != nil {
		return nil, err
	}
	d, err := decideIdentity(in)
	if err != nil {
		return nil, err
	}
	acct, source, err := p.execute(ctx, d, in)
	if err != nil {
		return nil, err
	}
	p.selectedSource = source
	// Assigned only after full success: error paths can never leave a
	// partial selection behind.
	p.selection = d.selection
	return acct, nil
}

// providerAccount pairs an extension-provider account with its token source.
type providerAccount struct {
	acct   *Account
	source extensionTokenSource
}

// identityInputs is one invocation's complete arbitration input, gathered in
// a single pass by gatherIdentityInputs. It is read-only after gathering;
// decideIdentity consumes it without further I/O.
type identityInputs struct {
	profile    string
	profileSrc CredentialSourceKind

	managed *providerAccount // managed extension account; wins arbitration outright
	direct  *providerAccount // complete direct env credential
	// directBlock is a provider's explicit incomplete-direct-credential
	// classification (BlockError.Code == credential_incomplete). It
	// participates in profile arbitration instead of failing outright.
	directBlock *extcred.BlockError

	// directKeys / conflictKeys describe the BUILTIN process-env direct
	// credential surface (LARKSUITE_CLI_* variable NAMES, never values).
	// They annotate DirectCredentialEnv and conflict hints; a third-party
	// AccountDirect provider reports its own inputs via BlockError metadata
	// (PresentKeys/AppID), not through these.
	directKeys   []string
	conflictKeys []string

	config    *core.MultiAppConfig
	configErr error
}

// gatherIdentityInputs performs the arbitration's read phase: it consults the
// extension providers and snapshots the config. Providers classify their own
// failures at the source (BlockError.Code); this layer must not infer them by
// re-reading environment variables or parsing Reason.
func (p *CredentialProvider) gatherIdentityInputs(ctx context.Context) (identityInputs, error) {
	in := identityInputs{
		profile:      p.profile,
		profileSrc:   p.profileSrc,
		directKeys:   presentDirectCredentialKeys(),
		conflictKeys: presentDirectCredentialInputKeys(),
	}
	for _, prov := range p.providers {
		acct, err := prov.ResolveAccount(ctx)
		if err != nil {
			var blockErr *extcred.BlockError
			if errors.As(err, &blockErr) {
				switch blockErr.Code {
				case extcred.BlockReasonCredentialIncomplete:
					// app_credential_incomplete, profile matching, and
					// DirectCredentialEnv diagnostics are defined in terms of
					// the builtin LARKSUITE_CLI_* env surface. Until the SPI
					// carries provider-owned input descriptors, accepting this
					// classification from another provider would produce
					// contradictory arbitration and repair hints.
					if _, builtin := prov.(*envprovider.Provider); !builtin {
						return in, newCredentialIncompleteProviderContractError(prov)
					}
					in.directBlock = blockErr
				case extcred.BlockReasonInvalidPolicy:
					// A user-supplied policy value failed validation; that is
					// a validation error, never an internal one.
					return in, newInvalidPolicyError(blockErr)
				default:
					// Blocks without a recognized Code preserve their
					// original attribution.
					return in, err
				}
				break
			}
			// Any other provider error preserves its original attribution.
			return in, err
		}
		if acct == nil {
			continue
		}
		pa := &providerAccount{acct: convertAccount(acct), source: extensionTokenSource{provider: prov}}
		switch acct.Kind {
		case extcred.AccountDirect:
			// The arbitration's direct-credential surface — DirectCredentialEnv,
			// the env:LARKSUITE_CLI_APP_ID selection source, conflict-hint
			// keys — is defined in terms of the builtin process-env variables.
			// Until the SPI carries provider-reported input descriptors, only
			// the builtin env provider may declare AccountDirect; accepting it
			// from anyone else would produce self-contradictory diagnostics
			// (e.g. credentialSource "env:LARKSUITE_CLI_APP_ID" with
			// directCredentialEnv.present=false). The check is by concrete
			// type: the registry reserves neither names nor uniqueness, so a
			// Name() comparison would be forgeable.
			if _, builtin := prov.(*envprovider.Provider); !builtin {
				return in, errs.NewInternalError(errs.SubtypeUnknown,
					"credential provider %q declared AccountDirect, which is reserved for the builtin env provider", prov.Name())
			}
			in.direct = pa
		case extcred.AccountManaged:
			in.managed = pa
		default:
			return in, errs.NewInternalError(errs.SubtypeUnknown,
				"credential provider %q returned unknown AccountKind %d", prov.Name(), acct.Kind)
		}
		break // the first engaged provider ends the scan (registry priority order)
	}
	// The config snapshot backs profile lookup, the config-default route, and
	// config-default failure attribution. A winning managed or direct-env
	// identity without a profile never needs it — and managed identities must
	// keep working when the config is absent or malformed.
	if in.managed == nil && (in.profile != "" || in.direct == nil) {
		in.config, in.configErr = core.LoadOrNotConfigured()
	}
	return in, nil
}

// credentialRoute names which source serves the selected account and tokens.
type credentialRoute int

const (
	routeManaged credentialRoute = iota
	routeProfile
	routeDirectEnv
	routeConfigDefault
)

// decision is decideIdentity's complete verdict. Nothing in it touched I/O.
type decision struct {
	route     credentialRoute
	selection IdentitySelection
	// profileAppID is set on routeProfile; app_id is plaintext and safe to
	// echo in the secret-invalid error.
	profileAppID string
}

// decideIdentity holds every selection rule in one place: precedence
// (managed > profile > direct env > config default), profile/direct-env
// conflict detection, and error attribution. It is pure — same inputs, same
// verdict — so the full selection matrix is table-testable without env vars
// or config fixtures.
func decideIdentity(in identityInputs) (decision, error) {
	// DirectCredentialEnv reports the direct env vars truthfully on every
	// route: Present always means "direct credential env vars are set".
	directEnv := DirectCredentialEnv{Present: len(in.directKeys) > 0, Keys: in.directKeys}
	if in.direct != nil {
		directEnv.AppID = in.direct.acct.AppID
	}
	switch {
	case in.managed != nil:
		return decision{route: routeManaged, selection: IdentitySelection{
			Source:              SourceExtension(in.managed.source.Name()),
			DirectCredentialEnv: directEnv,
		}}, nil
	case in.profile != "":
		return decideProfile(in, directEnv)
	case in.directBlock != nil:
		return decision{}, newAppCredentialIncompleteError(in.directBlock, false)
	case in.direct != nil:
		return decision{route: routeDirectEnv, selection: IdentitySelection{
			Source:              SourceEnvAppID,
			DirectCredentialEnv: directEnv,
		}}, nil
	default:
		return decision{route: routeConfigDefault, selection: IdentitySelection{
			Source:              selectionSourceForDefault(in.config),
			DirectCredentialEnv: directEnv,
		}}, nil
	}
}

// decideProfile arbitrates an explicit profile against the direct env
// credential state.
func decideProfile(in identityInputs, directEnv DirectCredentialEnv) (decision, error) {
	app, err := findProfile(in)
	if err != nil {
		return decision{}, err
	}
	if in.directBlock != nil {
		// APP_ID-only is sufficient to compare sources: a matching selected
		// profile supplies the credential and tokens; a mismatch is the same
		// hard conflict as a complete direct env. Anything less than a usable
		// app_id keeps the provider's repair error, extended with the
		// unset-to-use-the-profile path.
		if in.directBlock.AppID == "" || !slices.Contains(in.directBlock.PresentKeys, envvars.CliAppID) {
			return decision{}, newAppCredentialIncompleteError(in.directBlock, true)
		}
		if app.AppId != in.directBlock.AppID {
			return decision{}, newProfileAppCredentialConflict(
				in.profile, app.AppId, in.directBlock.AppID, in.directBlock.PresentKeys)
		}
		directEnv.AppID = in.directBlock.AppID
		directEnv.Matched = true
	}
	if in.direct != nil {
		// E == complete: the direct env app_id must match the profile.
		if app.AppId != in.direct.acct.AppID {
			return decision{}, newProfileAppCredentialConflict(
				in.profile, app.AppId, in.direct.acct.AppID, in.conflictKeys)
		}
		directEnv.Matched = true
	}
	return decision{
		route:        routeProfile,
		selection:    IdentitySelection{Source: in.profileSrc, DirectCredentialEnv: directEnv},
		profileAppID: app.AppId,
	}, nil
}

// findProfile resolves the requested profile against the config snapshot.
// A malformed config must surface its real typed cause (invalid_config):
// reporting it as profile_not_found would send the user to `profile list`
// and hide the broken file. Only a genuinely absent config degrades to
// profile_not_found, because the profile then cannot exist anywhere. Both
// deliberately outrank an incomplete direct env: fixing the profile side is
// what makes the selected profile usable.
func findProfile(in identityInputs) (*core.AppConfig, error) {
	if in.configErr != nil {
		if prob, ok := errs.ProblemOf(in.configErr); !ok || prob.Subtype != errs.SubtypeNotConfigured {
			return nil, in.configErr
		}
	}
	if in.config != nil {
		if app := in.config.FindApp(in.profile); app != nil {
			return app, nil
		}
	}
	return nil, errs.NewConfigError(errs.SubtypeProfileNotFound,
		"profile %q not found", in.profile).
		WithProfile(in.profile).
		WithCredentialSource(string(in.profileSrc)).
		WithHint("run `lark-cli profile list` to see available profiles.")
}

// execute performs the remaining I/O for the decided route and returns the
// account together with its token source.
func (p *CredentialProvider) execute(ctx context.Context, d decision, in identityInputs) (*Account, credentialSource, error) {
	switch d.route {
	case routeManaged:
		return in.managed.acct, in.managed.source, nil
	case routeDirectEnv:
		return in.direct.acct, in.direct.source, nil
	case routeProfile:
		// Resolve the profile's own (keychain-backed) credential locally.
		acct, err := p.defaultAcct.ResolveAccount(ctx)
		if err != nil {
			// A typed failure other than not_configured carries its own
			// precise, secret-free diagnosis (typed errors never embed secret
			// material per the error contract) — pass it through instead of
			// flattening it into the generic secret error. Untyped failures
			// and a config that vanished mid-resolution stay masked: their
			// content is not guaranteed secret-free.
			if prob, ok := errs.ProblemOf(err); ok && prob.Subtype != errs.SubtypeNotConfigured {
				return nil, nil, err
			}
			return nil, nil, newProfileSecretInvalidError(in.profile, d.profileAppID)
		}
		// The resolver re-reads the config; a concurrent profile edit between
		// gather and here could hand back a different app. Refuse the mismatch
		// instead of silently using credentials the arbitration never checked.
		if acct.AppID != d.profileAppID {
			return nil, nil, errs.NewInternalError(errs.SubtypeUnknown,
				"config changed during resolution: profile %q resolved to a different app", in.profile).
				WithHint("retry the command.")
		}
		return acct, defaultTokenSource{resolver: p.defaultToken}, nil
	default: // routeConfigDefault
		if p.defaultAcct == nil {
			return nil, nil, core.NotConfiguredError()
		}
		acct, err := p.defaultAcct.ResolveAccount(ctx)
		if err != nil {
			return nil, nil, translateConfigDefaultFailure(err, in.config)
		}
		return acct, defaultTokenSource{resolver: p.defaultToken}, nil
	}
}

// translateConfigDefaultFailure attributes a config-default failure from the
// snapshot: a default profile that EXISTS (has an app_id) but whose secret
// cannot be resolved locally is profile_secret_invalid — "identity is
// configured, its secret is broken" is more actionable than "no active
// profile". Only when there is genuinely no usable default profile do we
// report no_active_profile. Other typed failures pass through unchanged.
func translateConfigDefaultFailure(err error, multi *core.MultiAppConfig) error {
	if prob, ok := errs.ProblemOf(err); !ok || prob.Subtype != errs.SubtypeNotConfigured {
		return err
	}
	if multi != nil {
		if app := multi.CurrentAppConfig(""); app != nil && app.AppId != "" {
			return newProfileSecretInvalidError(app.ProfileName(), app.AppId)
		}
	}
	return errs.NewConfigError(errs.SubtypeNoActiveProfile, "no active profile").
		WithCredentialSource(noActiveProfileCredentialSource).
		WithHint("run `lark-cli config init` / `lark-cli profile add`, or set %s.", envvars.CliProfile)
}

func newProfileAppCredentialConflict(profile, profileAppID, envAppID string, presentKeys []string) error {
	err := errs.NewValidationError(errs.SubtypeProfileAppCredentialConflict,
		"profile %q app_id does not match %s", profile, envvars.CliAppID).
		WithProfileAppConflict(profileAppID, envAppID)
	if len(presentKeys) > 0 {
		return err.WithHint("unset %s, or select a profile whose app_id matches the environment.",
			humanList(presentKeys, "and"))
	}
	return err.WithHint("unset the direct credential environment variables, or select a profile whose app_id matches the environment.")
}

func newAppCredentialIncompleteError(blockErr *extcred.BlockError, selectedProfileAvailable bool) *errs.ConfigError {
	err := errs.NewConfigError(errs.SubtypeAppCredentialIncomplete, "%s", blockErr.Reason).
		WithCause(blockErr)
	if len(blockErr.MissingKeys) > 0 {
		err.WithMissingKeys(blockErr.MissingKeys...)
	}
	if len(blockErr.RequiredAnyOf) > 0 {
		err.WithRequiredAnyOf(blockErr.RequiredAnyOf...)
	}

	hint := credentialRepairHint(blockErr)
	if selectedProfileAvailable && len(blockErr.PresentKeys) > 0 {
		hint += fmt.Sprintf(", or unset %s to use the selected profile", humanList(blockErr.PresentKeys, "and"))
	}
	return err.WithHint("%s.", hint)
}

func credentialRepairHint(blockErr *extcred.BlockError) string {
	if len(blockErr.RequiredAnyOf) > 0 {
		return "set " + humanList(blockErr.RequiredAnyOf, "or")
	}
	return "set " + humanList(blockErr.MissingKeys, "and")
}

func humanList(items []string, conjunction string) string {
	switch len(items) {
	case 0:
		return "the missing direct credential variables"
	case 1:
		return items[0]
	case 2:
		return items[0] + " " + conjunction + " " + items[1]
	default:
		return strings.Join(items[:len(items)-1], ", ") + ", " + conjunction + " " + items[len(items)-1]
	}
}

// newInvalidPolicyError translates a provider's invalid-policy block into the
// typed validation contract: the failed variable name travels in param, the
// repair path in the hint, and the original block stays on the cause chain.
// Reason carries only the variable name and its non-secret value.
func newInvalidPolicyError(blockErr *extcred.BlockError) error {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", blockErr.Reason).
		WithParam(blockErr.Param).
		WithCause(blockErr).
		WithHint("set %s to a supported value or unset it.", blockErr.Param)
}

func newCredentialIncompleteProviderContractError(prov extcred.Provider) error {
	return errs.NewInternalError(errs.SubtypeUnknown,
		"credential provider %q returned credential_incomplete, which is reserved for the builtin env provider", prov.Name())
}

// newProfileSecretInvalidError is deliberately generic (SECURITY): the
// underlying cause may carry secret material, so neither it nor its message
// may reach the envelope. app_id is plaintext and safe to echo.
func newProfileSecretInvalidError(profile, appID string) error {
	return errs.NewConfigError(errs.SubtypeProfileSecretInvalid,
		"profile %q credential could not be resolved locally", profile).
		WithProfile(profile).
		WithAppID(appID).
		WithHint("verify the profile's app secret or re-add the profile with `lark-cli config`.")
}

// enrichOrClearIdentity verifies a provider-supplied user identity via
// enrichUserInfo. Verification failure is non-fatal — SupportedIdentities
// (used for strict mode) is already set by the provider — but an unverified
// identity must not survive it: a stale OpenID would attribute calls to a
// user the token can no longer act for.
func (p *CredentialProvider) enrichOrClearIdentity(ctx context.Context, acct *Account, source credentialSource) {
	err := p.enrichUserInfo(ctx, acct, source)
	if err == nil {
		return
	}
	if p.warnOut != nil {
		_, _ = fmt.Fprintf(p.warnOut, "warning: unable to verify user identity from credential source %q: %v\n", source.Name(), err)
	}
	acct.UserOpenId = ""
	acct.UserName = ""
}

// noActiveProfileCredentialSource is the credential_source reported on the
// no_active_profile error. The error contract fixes this to the literal "config": there is
// no resolved default profile at all, so the more specific config:currentApp /
// config:firstApp source values (used on successful config-default selections)
// would be misleading. It is an enum string, never a secret.
const noActiveProfileCredentialSource = "config"

// selectionSourceForDefault reports whether the config default resolved to the
// explicit currentApp or fell back to the first app.
func selectionSourceForDefault(multi *core.MultiAppConfig) CredentialSourceKind {
	if multi != nil && multi.CurrentApp != "" {
		return SourceConfigCurrentApp
	}
	return SourceConfigFirstApp
}

// presentDirectCredentialKeys returns the NAMES (never values) of the direct
// app credential env vars that are set. Used to annotate DirectCredentialEnv.
func presentDirectCredentialKeys() []string {
	var keys []string
	if os.Getenv(envvars.CliAppID) != "" {
		keys = append(keys, envvars.CliAppID)
	}
	if os.Getenv(envvars.CliAppSecret) != "" {
		keys = append(keys, envvars.CliAppSecret)
	}
	return keys
}

// presentDirectCredentialInputKeys returns all direct env input names that
// must be cleared together to remove a profile/app_id conflict. Values are
// never returned.
func presentDirectCredentialInputKeys() []string {
	keys := presentDirectCredentialKeys()
	if os.Getenv(envvars.CliUserAccessToken) != "" {
		keys = append(keys, envvars.CliUserAccessToken)
	}
	if os.Getenv(envvars.CliTenantAccessToken) != "" {
		keys = append(keys, envvars.CliTenantAccessToken)
	}
	return keys
}

// Selection resolves the account (once) and returns the cached, secret-free
// explanation of how the credential/App was selected. It mirrors
// selectedCredentialSource: resolve-then-return.
func (p *CredentialProvider) Selection(ctx context.Context) (IdentitySelection, error) {
	if _, err := p.ResolveAccount(ctx); err != nil {
		return IdentitySelection{}, err
	}
	return p.selection, nil
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
	info, err := fetchUserInfo(ctx, hc, acct.Brand, tok.Token)
	if err != nil {
		return fmt.Errorf("failed to verify user identity: %w", err)
	}
	acct.UserOpenId = info.OpenID
	acct.UserName = info.Name
	return nil
}

func (p *CredentialProvider) selectedCredentialSource(ctx context.Context) (credentialSource, error) {
	if _, err := p.resolveAccountSelection(ctx); err != nil {
		return nil, err
	}
	if p.selectedSource == nil {
		return nil, errs.NewInternalError(errs.SubtypeUnknown,
			"credential provider resolved an account without selecting a token source").
			WithHint("retry the command.")
	}
	return p.selectedSource, nil
}

func resolveTokenFromSource(ctx context.Context, source credentialSource, req TokenSpec) (*TokenResult, error) {
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

// ResolveToken resolves an access token.
func (p *CredentialProvider) ResolveToken(ctx context.Context, req TokenSpec) (*TokenResult, error) {
	acct, err := p.resolveAccountSelection(ctx)
	if err != nil {
		return nil, err
	}
	if acct == nil {
		return nil, errs.NewInternalError(errs.SubtypeUnknown,
			"credential provider resolved no account before %s token resolution", req.Type).
			WithHint("retry the command.")
	}
	source := p.selectedSource
	if source == nil {
		return nil, errs.NewInternalError(errs.SubtypeUnknown,
			"credential provider resolved app %q without selecting a token source", acct.AppID).
			WithHint("retry the command.")
	}
	if req.AppID == "" {
		return nil, errs.NewInternalError(errs.SubtypeUnknown,
			"TokenSpec.AppID is required for %s token resolution", req.Type).
			WithHint("retry the command.")
	}
	if req.AppID != acct.AppID {
		return nil, errs.NewInternalError(errs.SubtypeUnknown,
			"token requested for app %q but the selected account belongs to app %q", req.AppID, acct.AppID).
			WithHint("retry the command.")
	}
	return resolveTokenFromSource(ctx, source, req)
}

// ActiveExtensionProviderName reports whether an extension provider is managing
// the credentials that actually win selection. With an explicit profile that
// resolves successfully it reuses ResolveAccount's cached arbitration result;
// otherwise it probes extension providers directly and returns the first
// engaged provider.
//
// "Engaged" means: ResolveAccount returns a non-nil account, OR returns a
// *extcred.BlockError (provider configured but misconfigured — still counts as
// external). Any other probe error is propagated to the caller.
//
// A failed profile resolution (profile not found, broken secret, malformed
// config, incomplete direct env, ...) deliberately does NOT propagate: this
// probe guards the builtin setup/repair commands (auth, config), and an
// unresolvable credential must never lock the user out of the commands that
// fix it. It falls back to the engagement probe, which answers the only
// question this function owns: is an extension provider holding credentials?
//
// Returns ("", nil) when no extension provider is active (built-in keychain path).
// Safe to call multiple times: explicit-profile resolution uses sync.Once, while
// the probe path only consults providers.
func (p *CredentialProvider) ActiveExtensionProviderName(ctx context.Context) (string, error) {
	// With an explicit profile, report the source that actually won the same
	// arbitration used by commands. A matching APP_ID-only env block is not an
	// external takeover once the selected profile supplies credentials/tokens.
	if p.profile != "" {
		if _, err := p.ResolveAccount(ctx); err == nil {
			if p.selectedSource == nil {
				return "", nil
			}
			if _, builtin := p.selectedSource.(defaultTokenSource); builtin {
				return "", nil
			}
			return p.selectedSource.Name(), nil
		}
		// Resolution failed — fall through to the engagement probe.
	}
	for _, prov := range p.providers {
		acct, err := prov.ResolveAccount(ctx)
		if err != nil {
			var blockErr *extcred.BlockError
			if errors.As(err, &blockErr) {
				// Align with formal arbitration: a misconfigured policy
				// variable is the same typed validation error everywhere —
				// not an external takeover of the provider that reported it,
				// and not license to keep scanning and blame a later
				// provider instead.
				if blockErr.Code == extcred.BlockReasonInvalidPolicy {
					return "", newInvalidPolicyError(blockErr)
				}
				if blockErr.Code == extcred.BlockReasonCredentialIncomplete {
					if _, builtin := prov.(*envprovider.Provider); !builtin {
						return "", newCredentialIncompleteProviderContractError(prov)
					}
				}
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
	return &Account{
		AppID:               ext.AppID,
		AppSecret:           ext.AppSecret,
		Brand:               core.LarkBrand(ext.Brand),
		DefaultAs:           core.Identity(ext.DefaultAs),
		ProfileName:         ext.ProfileName,
		UserOpenId:          ext.OpenID,
		SupportedIdentities: uint8(ext.SupportedIdentities),
	}
}
