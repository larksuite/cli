// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whoami

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/identitydiag"
	"github.com/larksuite/cli/internal/output"
)

// whoamiResult is the structured output of `lark-cli whoami`.
//
// The self-vs-delegated distinction is carried by `identity`: a bot identity is
// the app acting as itself; a user identity is the app acting *on behalf of* a
// person (calls are attributed to that user, who is not necessarily present).
// onBehalfOf only *names* that person and so appears only once a user is
// resolved — a user identity that is not signed in still has identity "user"
// but no onBehalfOf yet. Do not read "no onBehalfOf" as "self"; read `identity`.
type whoamiResult struct {
	Profile        string         `json:"profile"`
	AppID          string         `json:"appId"`
	Brand          core.LarkBrand `json:"brand"`
	DefaultAs      string         `json:"defaultAs"`
	Identity       string         `json:"identity"`
	IdentitySource string         `json:"identitySource"`
	Available      bool           `json:"available"`
	TokenStatus    string         `json:"tokenStatus"`
	OnBehalfOf     *delegatedUser `json:"onBehalfOf,omitempty"`
	Hint           string         `json:"hint,omitempty"`

	// CredentialSource, Explicit, and DirectCredentialEnv surface the cached
	// credential.IdentitySelection computed during resolution (not re-inferred
	// here). On the non-env extension-provider path CredentialSource is
	// "extension:<provider>" (e.g. "extension:sidecar"); an empty value only
	// means the selection was never resolved.
	CredentialSource    string                         `json:"credentialSource"`
	Explicit            bool                           `json:"explicit"`
	DirectCredentialEnv credential.DirectCredentialEnv `json:"directCredentialEnv"`
}

// delegatedUser is the user a user-identity acts on behalf of.
type delegatedUser struct {
	UserName string `json:"userName,omitempty"`
	OpenID   string `json:"openId,omitempty"`
}

// Options holds inputs for the whoami command.
type Options struct {
	Factory *cmdutil.Factory
	As      string
}

// NewCmdWhoami creates the top-level whoami command. It reports the identity
// that the next API call would actually use (resolved via Factory.ResolveAs),
// together with the active profile, app, and token status. Output is always
// JSON — whoami is consumed by agents. With the built-in credential path it is
// local-only; when an external credential provider manages tokens, resolving
// the identity may contact that provider.
func NewCmdWhoami(f *cmdutil.Factory) *cobra.Command {
	opts := &Options{Factory: f}
	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Show the current effective identity, app, profile, and token status (JSON)",
		Long: `Show the effective app identity used by this invocation. This is not OAuth login status;
use ` + "`lark-cli auth status --json`" + ` for OAuth user/token state.
The JSON output includes credentialSource, appId, brand, and whether direct app credential
env is present and matches the selected profile.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return whoamiRun(cmd, opts)
		},
	}
	cmdutil.DisableAuthCheck(cmd)
	cmdutil.AddAPIIdentityFlag(context.Background(), cmd, f, &opts.As)
	// Output is always JSON. Accept (and ignore) --json so existing
	// `whoami --json` callers don't break; hide it to avoid implying a non-JSON
	// mode exists.
	cmd.Flags().Bool("json", true, "deprecated: output is always JSON")
	_ = cmd.Flags().MarkHidden("json")
	cmdutil.SetRisk(cmd, "read")
	return cmd
}

func whoamiRun(cmd *cobra.Command, opts *Options) error {
	f := opts.Factory
	cfg, err := f.Config()
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	flagAs := core.Identity(opts.As)
	as := f.ResolveAs(ctx, cmd, flagAs)
	// Validate as a real API call does (strict mode, then identity) so whoami
	// can't preview an identity the next call would refuse.
	if err := f.CheckStrictMode(ctx, as); err != nil {
		return err
	}
	if err := f.CheckIdentity(as, []string{"user", "bot"}); err != nil {
		return err
	}
	source := resolveSource(
		cmd.Flags().Changed("as"),
		flagAs,
		f.IdentityAutoDetected,
		f.ResolveStrictMode(ctx).ForcedIdentity(),
	)
	diag := identitydiag.Diagnose(ctx, f, cfg, false)
	// Read the cached selection computed during resolution; never re-infer it
	// here. A resolution failure (e.g. under a non-env extension provider that
	// doesn't populate a selection) degrades to the zero value rather than
	// regressing whoami's own error/diagnostic path above.
	var selection credential.IdentitySelection
	if f.Credential != nil {
		if sel, err := f.Credential.Selection(ctx); err == nil {
			selection = sel
		}
	}
	res := buildResult(cfg, as, source, diag, selection)
	output.PrintJson(f.IOStreams.Out, res)
	return nil
}

// resolveSource derives how the effective identity became effective.
// Mirrors Factory.ResolveAs precedence: explicit flag wins; otherwise an
// auto-detected result means auto-detect; otherwise a strict-mode forced
// identity means strict-mode; otherwise it came from configured default-as.
// Values are snake_case to match the other enum fields (e.g. tokenStatus).
func resolveSource(changedAs bool, flagAs core.Identity, autoDetected bool, strictForced core.Identity) string {
	if changedAs && (flagAs == core.AsUser || flagAs == core.AsBot) {
		return "flag"
	}
	if autoDetected {
		return "auto_detect"
	}
	if strictForced != "" {
		return "strict_mode"
	}
	return "default_as"
}

// buildResult maps the resolved identity and local diagnostics into the output.
// ResolveAs only ever returns user or bot, so the default branch handles user.
// selection is the cached credential.IdentitySelection from resolution; it is
// read as-is, never recomputed.
func buildResult(cfg *core.CliConfig, as core.Identity, source string, diag identitydiag.Result, selection credential.IdentitySelection) *whoamiResult {
	defaultAs := cfg.DefaultAs
	if defaultAs == "" {
		defaultAs = core.AsAuto
	}
	res := &whoamiResult{
		Profile:             cfg.ProfileName,
		AppID:               cfg.AppID,
		Brand:               cfg.Brand,
		DefaultAs:           string(defaultAs),
		Identity:            string(as),
		IdentitySource:      source,
		CredentialSource:    string(selection.Source),
		Explicit:            selection.Explicit(),
		DirectCredentialEnv: selection.DirectCredentialEnv,
	}
	// Use the diagnosed hint as-is: it is tailored to the credential source, so
	// it never says "auth login" when that is blocked under an external provider.
	switch as {
	case core.AsBot:
		res.Available = diag.Bot.Available
		res.TokenStatus = diag.Bot.Status
		if !diag.Bot.Available {
			res.Hint = diag.Bot.Hint
		}
	default: // user
		res.Available = diag.User.Available
		// Use Status (not the raw TokenStatus) so the vocab matches the bot
		// branch: "ready" means usable for both. available stays the canonical
		// usable signal; tokenStatus is the readable state behind it.
		res.TokenStatus = diag.User.Status
		// Set onBehalfOf only when a user is actually resolved; an unresolved
		// user identity (not signed in) has no one to act on behalf of yet.
		if diag.User.UserName != "" || diag.User.OpenID != "" {
			res.OnBehalfOf = &delegatedUser{UserName: diag.User.UserName, OpenID: diag.User.OpenID}
		}
		if !diag.User.Available {
			res.Hint = diag.User.Hint
		}
	}
	return res
}
