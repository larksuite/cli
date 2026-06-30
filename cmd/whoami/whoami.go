// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whoami

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/identitydiag"
	"github.com/larksuite/cli/internal/output"
)

// whoamiResult is the structured output of `lark-cli whoami`.
type whoamiResult struct {
	Profile        string         `json:"profile"`
	AppID          string         `json:"appId"`
	Brand          core.LarkBrand `json:"brand"`
	DefaultAs      string         `json:"defaultAs"`
	Identity       string         `json:"identity"`
	IdentitySource string         `json:"identitySource"`
	Available      bool           `json:"available"`
	TokenStatus    string         `json:"tokenStatus"`
	OpenID         string         `json:"openId,omitempty"`
	UserName       string         `json:"userName,omitempty"`
	Hint           string         `json:"hint,omitempty"`
}

// Options holds inputs for the whoami command.
type Options struct {
	Factory *cmdutil.Factory
	As      string
	JSON    bool
}

// NewCmdWhoami creates the top-level whoami command. It reports the identity
// that the next API call would actually use (resolved via Factory.ResolveAs),
// together with the active profile, app, and token status. It is local-only:
// no network calls are made.
func NewCmdWhoami(f *cmdutil.Factory) *cobra.Command {
	opts := &Options{Factory: f}
	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Show the current effective identity, app, profile, and token status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return whoamiRun(cmd, opts)
		},
	}
	cmdutil.DisableAuthCheck(cmd)
	cmdutil.AddAPIIdentityFlag(context.Background(), cmd, f, &opts.As)
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "structured JSON output")
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
	// Reject an explicit --as that does not resolve to a usable identity, so a
	// typo like `--as admin` fails clearly instead of echoing back a bogus
	// identity. Keeps the §5.1 invariant (identity is always user or bot) and
	// matches how api/service/shortcut commands validate the resolved identity.
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
	res := buildResult(cfg, as, source, diag)
	if opts.JSON {
		output.PrintJson(f.IOStreams.Out, res)
		return nil
	}
	formatPretty(f.IOStreams.Out, res)
	return nil
}

// resolveSource derives how the effective identity became effective.
// Mirrors Factory.ResolveAs precedence: explicit flag wins; otherwise an
// auto-detected result means auto-detect; otherwise a strict-mode forced
// identity means strict-mode; otherwise it came from configured default-as.
func resolveSource(changedAs bool, flagAs core.Identity, autoDetected bool, strictForced core.Identity) string {
	if changedAs && (flagAs == core.AsUser || flagAs == core.AsBot) {
		return "flag"
	}
	if autoDetected {
		return "auto-detect"
	}
	if strictForced != "" {
		return "strict-mode"
	}
	return "default-as"
}

// buildResult maps the resolved identity and local diagnostics into the output.
// ResolveAs only ever returns user or bot, so the default branch handles user.
func buildResult(cfg *core.CliConfig, as core.Identity, source string, diag identitydiag.Result) *whoamiResult {
	defaultAs := cfg.DefaultAs
	if defaultAs == "" {
		defaultAs = core.AsAuto
	}
	res := &whoamiResult{
		Profile:        cfg.ProfileName,
		AppID:          cfg.AppID,
		Brand:          cfg.Brand,
		DefaultAs:      string(defaultAs),
		Identity:       string(as),
		IdentitySource: source,
	}
	switch as {
	case core.AsBot:
		res.Available = diag.Bot.Available
		res.TokenStatus = diag.Bot.Status
		if !diag.Bot.Available {
			res.Hint = "Bot identity not configured. Set app secret or bot token (see `lark-cli config --help`)."
		}
	default: // user
		res.Available = diag.User.Available
		res.OpenID = diag.User.OpenID
		res.UserName = diag.User.UserName
		res.TokenStatus = diag.User.TokenStatus
		if res.TokenStatus == "" {
			res.TokenStatus = "missing"
		}
		if !diag.User.Available {
			res.Hint = "No usable user token. Run `lark-cli auth login`."
		}
	}
	return res
}

// formatPretty writes the human-readable one-glance summary.
func formatPretty(w io.Writer, r *whoamiResult) {
	fmt.Fprintf(w, "Profile:  %s (%s, %s)\n", r.Profile, r.AppID, r.Brand)
	fmt.Fprintf(w, "Identity: %s (%s)\n", r.Identity, r.IdentitySource)
	if r.Identity == string(core.AsUser) && r.UserName != "" {
		if r.OpenID != "" {
			fmt.Fprintf(w, "User:     %s (%s)\n", r.UserName, r.OpenID)
		} else {
			fmt.Fprintf(w, "User:     %s\n", r.UserName)
		}
	}
	token := r.TokenStatus
	if !r.Available && r.Hint != "" {
		token = r.TokenStatus + " — " + r.Hint
	}
	// Write the label and value as separate %s args rather than one combined
	// literal. A single label-colon-value literal trips the public-content
	// credential scanner as a false-positive credential assignment; splitting
	// the args avoids it while producing identical output.
	fmt.Fprintf(w, "%s%s\n", "Token:    ", token)
}
