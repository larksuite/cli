// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"sync"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/extension/fileio"
	exttransport "github.com/larksuite/cli/extension/transport"
	"github.com/larksuite/cli/internal/affordance"
	"github.com/larksuite/cli/internal/agentsupport"
	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/skillref"
	"github.com/larksuite/cli/internal/transport"
)

// InvocationContext is the immutable per-invocation input resolved at the
// process boundary, before the command tree is built. Profile carries the
// selected profile name; ProfileSource records which channel selected it so
// downstream gating, errors, and status output can name the actual input.
type InvocationContext struct {
	Profile       string
	ProfileSource core.ProfileSource
}

// Factory holds shared dependencies injected into every command.
// All function fields are lazily initialized and cached after first call.
// In tests, replace any field to stub out external dependencies.
type Factory struct {
	APICatalog apicatalog.Catalog              // Immutable API metadata selected once for this command build.
	Affordance *affordance.Resolver            // Command guidance mapped against APICatalog; nil means no guidance.
	Config     func() (*core.CliConfig, error) // lazily loads app config from Credential
	HttpClient func() (*http.Client, error)    // policy-routed HTTP client for direct requests
	LarkClient func() (*lark.Client, error)    // Lark SDK client for all Open API calls
	IOStreams  *IOStreams                      // stdin/stdout/stderr streams

	Invocation           InvocationContext       // Immutable call context; do not mutate after Factory construction.
	Keychain             keychain.KeychainAccess // secret storage (real keychain in prod, mock in tests)
	IdentityAutoDetected bool                    // set by ResolveAs when identity was auto-detected
	ResolvedIdentity     core.Identity           // identity resolved by the last ResolveAs call
	CurrentCommand       *cobra.Command          // last matched command being executed; set during PersistentPreRun

	Credential *credential.CredentialProvider

	FileIOProvider fileio.Provider // file transfer provider (default: local filesystem)

	SkillContent    fs.FS               // embedded skill tree (rooted at the skill list); nil when the build embeds no skills
	SkillReferences *skillref.Resolver  // build-local projection from canonical skill references to embedded content
	Recovery        *recovery.Projector // build-local recovery presentation; nil means the default fully-visible surface

	// agentIdentityGateOffWarnOnce guards the LARKSUITE_CLI_AGENT_IDENTITY_GATE=off
	// notice (see agent_gate.go) to one print per Factory instance.
	agentIdentityGateOffWarnOnce sync.Once
}

// RenderRecoveryHint renders semantic recovery against this command tree.
// Factories created outside cmd.Build have no projector and therefore retain
// the default fully-visible wording.
func (f *Factory) RenderRecoveryHint(hint recovery.Hint) string {
	if f == nil {
		return hint.String()
	}
	return f.Recovery.RenderHint(hint)
}

// ResolveSkillReference projects a canonical skills-read reference into this
// build's embedded skill tree. Concealed skills-read surfaces never expose a
// reference, even when the underlying content remains embedded.
func (f *Factory) ResolveSkillReference(canonical string) (string, bool) {
	if f == nil || !f.Recovery.CanReference(recovery.TargetSkillsRead) {
		return "", false
	}
	if f.SkillReferences != nil {
		return f.SkillReferences.ResolveString(canonical)
	}

	ref, err := skillref.Parse(canonical)
	if err != nil || f.SkillContent == nil {
		return "", false
	}
	if _, err := fs.Stat(f.SkillContent, ref.StatPath()); err != nil {
		return "", false
	}
	return canonical, true
}

// ExternalHTTPClient returns a clone of the existing Factory client whose
// requests are explicitly classified as external. The underlying client,
// redirect policy, timeout, proxy configuration, and legacy transport provider
// behavior are preserved.
func (f *Factory) ExternalHTTPClient() (*http.Client, error) {
	client, err := f.HttpClient()
	if err != nil {
		return nil, err
	}
	return transport.ClientForRequestClass(client, exttransport.RequestClassExternal), nil
}

// ResolveFileIO resolves a FileIO instance using the current execution context.
// The provider controls whether the returned instance is fresh or cached.
func (f *Factory) ResolveFileIO(ctx context.Context) fileio.FileIO {
	if f == nil || f.FileIOProvider == nil {
		return nil
	}
	return f.FileIOProvider.ResolveFileIO(ctx)
}

// ResolveAs returns the effective identity type.
// If the user explicitly passed --as, use that value; otherwise use the configured default.
// When the value is "auto" (or unset), auto-detect based on credential hints.
func (f *Factory) ResolveAs(ctx context.Context, cmd *cobra.Command, flagAs core.Identity) core.Identity {
	f.IdentityAutoDetected = false

	if cmd != nil && cmd.Flags().Changed("as") {
		if flagAs != core.AsAuto {
			f.ResolvedIdentity = flagAs
			return flagAs
		}
		// --as auto: fall through to auto-detect
	}

	mode := f.ResolveStrictMode(ctx)
	// Strict mode forces implicit identity choices. Explicit --as user/bot is
	// preserved above so CheckStrictMode can reject incompatible requests.
	if forced := mode.ForcedIdentity(); forced != "" {
		f.ResolvedIdentity = forced
		return forced
	}

	hint := f.resolveIdentityHint(ctx)
	if cmd == nil || !cmd.Flags().Changed("as") {
		if defaultAs := resolveDefaultAsFromHint(hint); defaultAs != "" && defaultAs != core.AsAuto {
			f.ResolvedIdentity = defaultAs
			return f.ResolvedIdentity
		}
	}

	// Auto-detect based on credential hint
	f.IdentityAutoDetected = true
	result := autoDetectIdentityFromHint(hint)
	f.ResolvedIdentity = result
	return result
}

func resolveDefaultAsFromHint(hint *credential.IdentityHint) core.Identity {
	if hint != nil {
		return hint.DefaultAs
	}
	return ""
}

func autoDetectIdentityFromHint(hint *credential.IdentityHint) core.Identity {
	if hint != nil && hint.AutoAs != "" {
		return hint.AutoAs
	}
	return core.AsBot
}

func (f *Factory) resolveIdentityHint(ctx context.Context) *credential.IdentityHint {
	if f.Credential == nil {
		return nil
	}
	hint, err := f.Credential.ResolveIdentityHint(ctx)
	if err != nil {
		return nil
	}
	return hint
}

// ResolveIdentity returns the identity a command runs as, and reports why it
// cannot when the active profile forbids the one that was asked for.
//
// It reads only static account metadata. Nothing here mints a credential,
// verifies one, or touches credential storage, which is what lets read-only
// diagnostics and dry-run answer "who would this run as" without acquiring
// anything. Failures are returned rather than absorbed: silently falling back
// to bot is how an Agent Employee profile would end up sending a tenant
// credential.
func (f *Factory) ResolveIdentity(ctx context.Context, cmd *cobra.Command, flagAs core.Identity) (core.Identity, error) {
	kind, err := f.credentialKind(ctx)
	if err != nil {
		return "", err
	}
	as := f.ResolveAs(ctx, cmd, flagAs)
	if kind == core.CredentialAAT && as == core.AsBot {
		// Reached via an explicit --as bot, a bot-only strict mode, or a
		// default-as of bot. All three mean the same thing here and get the
		// same answer, before any request goes out.
		f.ResolvedIdentity = ""
		return "", credential.AgentBotIdentityError()
	}
	return as, nil
}

// credentialKind reports how the active profile is classified. A profile that
// cannot be resolved has no classification and reads as empty, which leaves
// every gate built on this a no-op and lets the command itself report why the
// profile does not work. Only an account that actually resolves to an Agent
// Employee turns the gates on.
func (f *Factory) credentialKind(ctx context.Context) (core.CredentialKind, error) {
	if f.Credential == nil {
		return "", nil
	}
	return f.Credential.AccountCredentialKind(ctx)
}

// CheckIdentity verifies the resolved identity is in the supported list.
// On success, sets f.ResolvedIdentity. On failure, returns an error
// tailored to whether the identity was explicit (--as) or auto-detected.
func (f *Factory) CheckIdentity(as core.Identity, supported []string) error {
	for _, t := range supported {
		if string(as) == t {
			f.ResolvedIdentity = as
			return nil
		}
	}
	list := strings.Join(supported, ", ")
	if f.IdentityAutoDetected {
		base := errs.NewValidationError(errs.SubtypeInvalidArgument,
			"resolved identity %q (via auto-detect or default-as) is not supported, this command only supports: %s",
			as, list).
			WithParam("--as")
		if len(supported) > 0 {
			return base.WithHint("use --as %s", supported[0])
		}
		return base
	}
	return errs.NewValidationError(errs.SubtypeInvalidArgument,
		"--as %s is not supported, this command only supports: %s", as, list).
		WithParam("--as")
}

// ResolveStrictMode returns the effective strict mode by reading
// Account.SupportedIdentities from the credential provider chain.
//
// An Agent Employee is always off. It has exactly one lane, so a forced user
// identity adds nothing, and a forced bot identity is a conflict the access
// contract reports on its own. What strict mode would add is the strict prune,
// which reads a command's declared lanes — and those say nothing about an
// agent token, so it would withhold shortcuts the verdict allows. The verdict
// and the agent filter are the Agent Employee's identity boundary.
func (f *Factory) ResolveStrictMode(ctx context.Context) core.StrictMode {
	if f.Credential == nil {
		return core.StrictModeOff
	}
	acct, err := f.Credential.ResolveAccount(ctx)
	if err != nil || acct == nil {
		return core.StrictModeOff
	}
	if acct.CredentialKind == core.CredentialAAT {
		return core.StrictModeOff
	}
	ids := extcred.IdentitySupport(acct.SupportedIdentities)
	switch {
	case ids.BotOnly():
		return core.StrictModeBot
	case ids.UserOnly():
		return core.StrictModeUser
	default:
		return core.StrictModeOff
	}
}

// CheckStrictMode returns an error if strict mode is active and identity is not allowed.
func (f *Factory) CheckStrictMode(ctx context.Context, as core.Identity) error {
	mode := f.ResolveStrictMode(ctx)
	if mode.IsActive() && !mode.AllowsIdentity(as) {
		hint := recovery.Join("", recovery.Command(recovery.TargetConfigStrictMode,
			"if the user explicitly wants to switch policy, see `lark-cli config strict-mode --help` (confirm with the user before switching; switching does NOT require re-bind)"))
		return recovery.Annotate(
			errs.NewValidationError(errs.SubtypeInvalidArgument,
				"strict mode is %q, only %s-identity commands are available", mode, mode.ForcedIdentity()).
				WithHint("%s", hint.String()),
			hint,
		)
	}
	return nil
}

// NewAPIClient creates an APIClient using the Factory's base Config (app credentials only).
// For user-mode calls where the correct user profile matters, use NewAPIClientWithConfig instead.
func (f *Factory) NewAPIClient() (*client.APIClient, error) {
	cfg, err := f.Config()
	if err != nil {
		return nil, err
	}
	return f.NewAPIClientWithConfig(cfg)
}

// NewAPIClientWithConfig creates an APIClient with an explicit config.
// Use this when the caller has already resolved the correct config.
func (f *Factory) NewAPIClientWithConfig(cfg *core.CliConfig) (*client.APIClient, error) {
	sdk, err := f.LarkClient()
	if err != nil {
		return nil, err
	}
	httpClient, err := f.HttpClient()
	if err != nil {
		return nil, err
	}
	errOut := io.Discard
	if f.IOStreams != nil {
		errOut = f.IOStreams.ErrOut
	}
	return &client.APIClient{
		Config:     cfg,
		SDK:        sdk,
		HTTP:       httpClient,
		ErrOut:     errOut,
		Credential: f.Credential,
	}, nil
}

// RequireHumanCredential rejects a command that only means something for a
// human account when the active profile runs as an Agent Employee. An Agent
// Employee is authorized on the Agent platform and has nobody to walk through
// an interactive login, so offering one would send the user somewhere that
// cannot help. The refusal is decided locally, before any request goes out.
func (f *Factory) RequireHumanCredential(ctx context.Context, command string) error {
	agent, err := f.agentProfile(ctx)
	if err != nil || !agent {
		return err
	}
	return AgentRefusal(command, AgentRefusedHumanOnly)
}

// agentProfile reports whether the active profile runs as an Agent Employee.
// The gates below differ only in why they refuse, so the lookup and "could not
// tell" live here once. A factory with no credential provider is not an agent:
// credentialKind already answers that case.
func (f *Factory) agentProfile(ctx context.Context) (bool, error) {
	kind, err := f.credentialKind(ctx)
	if err != nil {
		return false, err
	}
	return kind == core.CredentialAAT, nil
}

// CheckAgentEligibility refuses a generated API command the metadata does not
// mark as callable by an Agent Employee. It is a separate gate from
// CheckIdentity because an Agent Employee shares the user wire.
//
// It takes bools rather than meta types so this package does not import
// internal/meta. serviceHasAgentMethod only selects the hint wording. The hint
// never names the bypass variable: its consumer is an autonomous process that
// would set it and leave it set.
func (f *Factory) CheckAgentEligibility(ctx context.Context, supportsAgent bool, serviceHasAgentMethod bool, label string) error {
	agent, err := f.agentProfile(ctx)
	if err != nil || !agent {
		return err
	}
	if !AgentIdentityGateEnabled() {
		f.WarnAgentIdentityGateOffOnce()
		return nil
	}
	if supportsAgent {
		return nil
	}
	reason := AgentRefusedUnmarkedService
	if serviceHasAgentMethod {
		reason = AgentRefusedUnmarkedMethod
	}
	return AgentRefusal(label, reason)
}

// CheckAgentSupport refuses a shortcut whose recorded verdict is denied for an
// Agent Employee. There is no environment switch: the verdict ships with the
// binary, so rollback means clearing shortcuts/agent_support.json.
//
// A verdict that does not deny is answered before the classification lookup:
// most commands are in that state and the lookup would only reach a guaranteed
// nil.
func (f *Factory) CheckAgentSupport(ctx context.Context, support agentsupport.Verdict, label string) error {
	if !support.Denies() {
		return nil
	}
	agent, err := f.agentProfile(ctx)
	if err != nil || !agent {
		return err
	}
	return AgentRefusal(label, AgentRefusedShortcutVerdict)
}

// IsAgentPrincipal reports whether the active profile runs as an Agent
// Employee. It exists for the command-tree build, which has to decide what to
// withhold before any command runs and must not reach into credential kinds
// itself. A profile that cannot be resolved reads as false, which keeps a
// broken or absent config from withholding anything.
func (f *Factory) IsAgentPrincipal(ctx context.Context) bool {
	agent, err := f.agentProfile(ctx)
	return err == nil && agent
}

// CheckShortcutPrincipal is the single identity gate for a shortcut, routed on
// the kind of principal the active profile carries.
//
// An Agent Employee is judged by the recorded verdict, not by the lane. It has
// exactly one lane -- ResolveIdentity refuses the bot lane outright -- so
// AuthTypes cannot tell it anything it does not already know, while the verdict
// is the only record of whether the shortcut's OpenAPI dependencies accept an
// agent token. What the verdict does not replace is the check that --as names
// an identity at all: that value is user input, and without the check a
// --dry-run answers ok with an identity that does not exist.
//
// Every other profile is judged only by the lane: CheckAgentSupport is a no-op
// for them by construction, so running it would cost a credential-kind lookup
// to reach a guaranteed nil.
func (f *Factory) CheckShortcutPrincipal(ctx context.Context, as core.Identity,
	authTypes []string, support agentsupport.Verdict, label string) error {
	agent, err := f.agentProfile(ctx)
	if err != nil {
		return err
	}
	if agent {
		if err := f.CheckIdentity(as, []string{string(core.AsUser)}); err != nil {
			return err
		}
		return f.CheckAgentSupport(ctx, support, label)
	}
	return f.CheckIdentity(as, authTypes)
}

// RequireBuiltinCredentialProvider returns a typed validation error when an
// extension provider is actively managing credentials. Intended for use by the
// subcommands that drive interactive credential management.
//
// Returns nil when:
//   - f.Credential is nil (test environments without credential setup)
//   - No extension provider is active (built-in keychain/config path is used)
func (f *Factory) RequireBuiltinCredentialProvider(ctx context.Context, command string) error {
	if f.Credential == nil {
		return nil
	}
	provName, err := f.Credential.ActiveExtensionProviderName(ctx)
	if err != nil {
		return err
	}
	if provName == "" {
		return nil
	}
	return errs.NewValidationError(errs.SubtypeInvalidArgument,
		"%q is not supported: credentials are provided externally and do not support interactive management", command).
		WithHint("If another tool or method for authorization is available in this environment, try that. Otherwise, ask the user to set up credentials through the appropriate channel.")
}
