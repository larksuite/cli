// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/output"
)

// ResolveConfig returns Config() for bot identity, or AuthConfig() for user identity.
func (f *Factory) ResolveConfig(as core.Identity) (*core.CliConfig, error) {
	if as.IsBot() {
		return f.Config()
	}
	return f.AuthConfig()
}

// Factory holds shared dependencies injected into every command.
// All function fields are lazily initialized and cached after first call.
// In tests, replace any field to stub out external dependencies.
type Factory struct {
	Config     func() (*core.CliConfig, error) // lazily loads app config (credentials, brand, defaultAs)
	AuthConfig func() (*core.CliConfig, error) // like Config but also requires a logged-in user
	HttpClient func() (*http.Client, error)    // HTTP client for non-Lark API calls (with retry and security headers)
	LarkClient func() (*lark.Client, error)    // Lark SDK client for all Open API calls
	IOStreams  *IOStreams                      // stdin/stdout/stderr streams

	Keychain             keychain.KeychainAccess // secret storage (real keychain in prod, mock in tests)
	IdentityAutoDetected bool                    // set by ResolveAs when identity was auto-detected
	ResolvedIdentity     core.Identity           // identity resolved by the last ResolveAs call
}

// ResolveAs returns the effective identity type.
// If the user explicitly passed --as, use that value; otherwise use the configured default.
// When the value is "auto" (or unset), auto-detect based on login state.
func (f *Factory) ResolveAs(cmd *cobra.Command, flagAs core.Identity) core.Identity {
	f.IdentityAutoDetected = false
	if cmd != nil && cmd.Flags().Changed("as") {
		if flagAs != "auto" {
			f.ResolvedIdentity = flagAs
			return flagAs
		}
		// --as auto: fall through to auto-detect
	} else if defaultAs := f.resolveDefaultAs(); defaultAs != "" && defaultAs != "auto" {
		f.ResolvedIdentity = core.Identity(defaultAs)
		return f.ResolvedIdentity
	}
	// Auto-detect based on login state
	f.IdentityAutoDetected = true
	result := f.autoDetectIdentity()
	f.ResolvedIdentity = result
	return result
}

// resolveDefaultAs returns the configured default identity: env var > config file.
func (f *Factory) resolveDefaultAs() string {
	if v := os.Getenv("LARKSUITE_CLI_DEFAULT_AS"); v != "" {
		return v
	}
	if cfg, err := f.Config(); err == nil {
		return cfg.DefaultAs
	}
	return ""
}

// autoDetectIdentity checks the login state and returns user if logged in, bot otherwise.
func (f *Factory) autoDetectIdentity() core.Identity {
	// When token is injected via env, always treat as user identity.
	if os.Getenv("LARK_ACCESS_TOKEN") != "" {
		return core.AsUser
	}
	cfg, err := f.Config()
	if err != nil || cfg.UserOpenId == "" {
		return core.AsBot
	}
	stored := auth.GetStoredToken(cfg.AppID, cfg.UserOpenId)
	if stored == nil {
		return core.AsBot
	}
	if auth.TokenStatus(stored) == "expired" {
		return core.AsBot
	}
	return core.AsUser
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
		return output.ErrValidation(
			"resolved identity %q (via auto-detect or default-as) is not supported, this command only supports: %s\nhint: use --as %s",
			as, list, supported[0])
	}
	return fmt.Errorf("--as %s is not supported, this command only supports: %s", as, list)
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
// Use this when the caller has already resolved the correct user profile
// (e.g. via AuthConfig for user-mode commands).
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
	return &client.APIClient{Config: cfg, SDK: sdk, HTTP: httpClient, ErrOut: errOut}, nil
}
