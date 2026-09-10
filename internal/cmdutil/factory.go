// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"strings"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/extension/fileio"
	exttransport "github.com/larksuite/cli/extension/transport"
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
func (f *Factory) ResolveStrictMode(ctx context.Context) core.StrictMode {
	if f.Credential == nil {
		return core.StrictModeOff
	}
	acct, err := f.Credential.ResolveAccount(ctx)
	if err != nil || acct == nil {
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

// RequireBuiltinCredentialProvider returns a typed validation error when an
// extension provider is actively managing credentials. Intended for use as
// PersistentPreRunE on the auth and config parent commands.
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
		// A provider that already classified its failure keeps that
		// classification: rewrapping would discard its category, its retry
		// hint and the exit code that goes with them.
		if _, ok := errs.ProblemOf(err); ok {
			return err
		}
		// This runs in PersistentPreRunE, ahead of the command body, so an
		// unclassified error escaping here would be read as a mistake in what
		// the user typed. A provider lookup failure is not that.
		return errs.NewInternalError(errs.SubtypeUnknown,
			"cannot determine the active credential provider: %v", err).WithCause(err)
	}
	if provName == "" {
		return nil
	}
	return errs.NewValidationError(errs.SubtypeInvalidArgument,
		"%q is not supported: credentials are provided externally and do not support interactive management", command).
		WithHint("If another tool or method for authorization is available in this environment, try that. Otherwise, ask the user to set up credentials through the appropriate channel.")
}
