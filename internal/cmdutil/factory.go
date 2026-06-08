// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"context"
	"io"
	"net/http"
	"strings"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/keychain"
)

type InvocationContext struct {
	Profile string
	// Resolved --user / LARKSUITE_CLI_OPEN_ID override (empty when neither was
	// provided); falls through to AppConfig.CurrentUser then Users[0] in
	// core.ResolveConfigFromMulti.
	UserOpenId string
	// "flag", "env", or ""; downstream error hints branch on this so an env
	// override miss gets different remediation copy than a --user miss.
	UserSource string
}

type Factory struct {
	Config     func() (*core.CliConfig, error)
	HttpClient func() (*http.Client, error)
	LarkClient func() (*lark.Client, error)
	IOStreams  *IOStreams

	Invocation           InvocationContext // immutable; do not mutate after construction
	Keychain             keychain.KeychainAccess
	IdentityAutoDetected bool          // set by ResolveAs when identity was auto-detected
	ResolvedIdentity     core.Identity // identity resolved by the last ResolveAs call
	CurrentCommand       *cobra.Command

	Credential *credential.CredentialProvider

	FileIOProvider fileio.Provider
}

// ResolveFileIO resolves a FileIO instance using the current execution context.
func (f *Factory) ResolveFileIO(ctx context.Context) fileio.FileIO {
	if f == nil || f.FileIOProvider == nil {
		return nil
	}
	return f.FileIOProvider.ResolveFileIO(ctx)
}

// ResolveAs returns the effective identity. Explicit --as wins; otherwise the
// configured default-as, then auto-detect from credential hints.
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
// Error copy differs based on whether the identity was explicit (--as) or auto-detected.
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

// ResolveStrictMode reads Account.SupportedIdentities from the credential
// provider chain.
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
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"strict mode is %q, only %s-identity commands are available", mode, mode.ForcedIdentity()).
			WithHint("if the user explicitly wants to switch policy, see `lark-cli config strict-mode --help` (confirm with the user before switching; switching does NOT require re-bind)")
	}
	return nil
}

// NewAPIClient creates an APIClient using the Factory's base Config (app
// credentials only). For user-mode calls where the correct user profile
// matters, use NewAPIClientWithConfig instead.
func (f *Factory) NewAPIClient() (*client.APIClient, error) {
	cfg, err := f.Config()
	if err != nil {
		return nil, err
	}
	return f.NewAPIClientWithConfig(cfg)
}

// NewAPIClientWithConfig creates an APIClient with an explicit config.
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
// extension provider is actively managing credentials. Intended as
// PersistentPreRunE on the auth and config parent commands. Returns nil when
// f.Credential is nil or no extension provider is active.
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
