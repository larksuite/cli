// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doctor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/identitydiag"
	"github.com/larksuite/cli/internal/keylesshelper"
	"github.com/larksuite/cli/internal/keysigner"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/transport"
	"github.com/larksuite/cli/internal/update"
)

// DoctorOptions holds inputs for the doctor command.
type DoctorOptions struct {
	Factory *cmdutil.Factory
	Ctx     context.Context
	Offline bool
}

// NewCmdDoctor creates the doctor command.
func NewCmdDoctor(f *cmdutil.Factory) *cobra.Command {
	return newCmdDoctor(f, nil)
}

// NewCmdDoctorWithRecovery creates the doctor command with a build-local
// recovery presenter. Distribution assembly uses this boundary; ordinary
// callers keep NewCmdDoctor's original function signature and default output.
func NewCmdDoctorWithRecovery(f *cmdutil.Factory, projector *recovery.Projector) *cobra.Command {
	return newCmdDoctor(f, projector)
}

func newCmdDoctor(f *cmdutil.Factory, projector *recovery.Projector) *cobra.Command {
	opts := &DoctorOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "CLI health check: config, auth, and connectivity",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Ctx = cmd.Context()
			return doctorRun(opts, projector)
		},
	}
	cmdutil.DisableAuthCheck(cmd)
	cmd.Flags().BoolVar(&opts.Offline, "offline", false, "skip network checks (only verify local state)")
	cmdutil.SetRisk(cmd, "read")

	return cmd
}

// checkResult represents one diagnostic check.
type checkResult struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // "pass", "warn", "fail", "skip"
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

func pass(name, msg string) checkResult {
	return checkResult{Name: name, Status: "pass", Message: msg}
}

func fail(name, msg, hint string) checkResult {
	return checkResult{Name: name, Status: "fail", Message: msg, Hint: hint}
}

func warn(name, msg, hint string) checkResult {
	return checkResult{Name: name, Status: "warn", Message: msg, Hint: hint}
}

func skip(name, msg string) checkResult {
	return checkResult{Name: name, Status: "skip", Message: msg}
}

func doctorRun(opts *DoctorOptions, projector *recovery.Projector) error {
	f := opts.Factory
	var checks []checkResult

	// ── 0. CLI version & update check ──
	checks = append(checks, pass("cli_version", build.Version))
	if !opts.Offline && projector.CanReference(recovery.TargetUpdate) {
		checks = append(checks, checkCLIUpdate()...)
	}

	// ── 1. Config file ──
	_, err := core.LoadMultiAppConfig()
	if err != nil {
		// For "config not present" cases, prefer the workspace-aware
		// NotConfiguredError message + hint (e.g. "openclaw context
		// detected but lark-cli is not bound to it" → bind --help) over
		// the OS-level "open ... no such file or directory".
		// For other errors (parse, perms), keep the raw error so the
		// underlying problem is still visible.
		msg, hint := err.Error(), ""
		if errors.Is(err, os.ErrNotExist) {
			var cfgErr *errs.ConfigError
			if errors.As(projector.Render(core.NotConfiguredError()), &cfgErr) {
				msg, hint = cfgErr.Message, cfgErr.Hint
			}
		}
		checks = append(checks, fail("config_file", msg, hint))
		return finishDoctor(f, checks)
	}
	checks = append(checks, pass("config_file", "config.json found"))

	// ── 2. App resolved ──
	cfg, err := f.Config()
	if err != nil {
		hint := ""
		var cfgErr *errs.ConfigError
		if errors.As(projector.Render(err), &cfgErr) {
			hint = cfgErr.Hint
		}
		checks = append(checks, fail("app_resolved", err.Error(), hint))
		return finishDoctor(f, checks)
	}
	checks = append(checks, pass("app_resolved", fmt.Sprintf("app: %s (%s)", cfg.AppID, cfg.Brand)))

	// An external credential provider resolves the account without consulting
	// profiles at all, so an explicit selector is silently inert. Say so:
	// nothing else in the session will. ProfileName is only populated by the
	// built-in config-backed provider, which makes it the provider telltale.
	if f.Invocation.Profile != "" && cfg.ProfileName == "" {
		selector := "--profile"
		if f.Invocation.ProfileSource == core.ProfileFromEnvironment {
			selector = envvars.CliProfile
		}
		checks = append(checks, warn("profile_selector",
			fmt.Sprintf("%s=%q is ignored: credentials are provided externally", selector, f.Invocation.Profile),
			fmt.Sprintf("unset %s, or remove the external credential variables to select accounts by profile", selector)))
	}

	ep := core.ResolveEndpoints(cfg.Brand)

	// ── 3. Identity readiness ──
	diagnostics := identitydiag.FilterRecovery(
		identitydiag.Diagnose(opts.Ctx, f, cfg, !opts.Offline),
		projector.CanReference,
	)
	checks = append(checks,
		identityCheck("bot_identity", diagnostics.Bot),
		identityCheck("user_identity", diagnostics.User),
	)
	if diagnostics.Bot.Available || diagnostics.User.Available {
		checks = append(checks, pass("identity_ready", "at least one identity is available"))
	} else {
		// No hint: this only summarizes the two checks above, which already carry
		// the source-appropriate remediation. A command here would be redundant,
		// or wrong (`auth status` is blocked under an external provider).
		checks = append(checks, fail("identity_ready", "no usable bot or user identity is available", ""))
	}

	// ── 3b. private_key_jwt / TEE signer (local; runs even with --offline) ──
	checks = append(checks, teeSignerCheck(opts.Ctx, cfg))

	// ── 4 & 5. Endpoint reachability ──
	checks = append(checks, networkChecks(opts.Ctx, opts, ep)...)

	return finishDoctor(f, checks)
}

func identityCheck(name string, id identitydiag.Identity) checkResult {
	if id.Available {
		return pass(name, id.Message)
	}
	return warn(name, id.Message, id.Hint)
}

const teeUnavailableHint = "ensure the device secure hardware is accessible (Linux TPM: add your user to the 'tss' group or run with sufficient privileges)"

// teeSignerCheck reports the private_key_jwt signing backend (TEE/TPM) status.
// The probe is local hardware only (no network), so it runs even with --offline;
// in a build without a TEE signer it short-circuits without touching any
// hardware. It is a hard requirement for private_key_jwt apps and purely
// informational for client_secret apps.
func teeSignerCheck(ctx context.Context, cfg *core.CliConfig) checkResult {
	usesPKJWT := cfg != nil && cfg.AuthMethod == core.AuthMethodPrivateKeyJWT
	helper, err := keylesshelper.Resolve()
	if err != nil {
		hint := fmt.Sprintf("fix the external keyless signer source, or re-run config init to replace/remove config.json keylessSignerCmd: %v", err)
		if usesPKJWT {
			return fail("tee_signer", "external keyless signer is misconfigured", hint)
		}
		return warn("tee_signer", "external keyless signer is misconfigured", hint)
	}
	if helper != nil {
		keyLabel := ""
		if cfg != nil {
			keyLabel = cfg.KeyLabel
		}
		if err := helper.Probe(ctx, keyLabel); err != nil {
			hint := fmt.Sprintf("fix the configured external keyless signer, or re-run config init to replace/remove it: %v", err)
			if usesPKJWT {
				return fail("tee_signer", "external keyless signer is misconfigured", hint)
			}
			return warn("tee_signer", "external keyless signer is misconfigured", hint)
		}
		return pass("tee_signer", "external keyless signer available")
	}
	info, ok, err := keysigner.ProbeActiveHardware(ctx)
	return teeCheckResult(info, ok, err, usesPKJWT)
}

// teeCheckResult maps a hardware probe to a doctor check. Split out from
// teeSignerCheck so the full matrix is unit-testable without a TPM.
func teeCheckResult(info keysigner.HardwareInfo, ok bool, probeErr error, usesPKJWT bool) checkResult {
	const name = "tee_signer"

	// No signer registered → private_key_jwt is unsupported on this build.
	if !ok {
		if usesPKJWT {
			return fail(name,
				"app uses private_key_jwt but this build has no TEE key signer",
				"the platform key signer ships by default on macOS, Linux, and Windows/amd64; this platform (e.g. Windows/arm64) has none — use a supported platform or re-register without --private-key-jwt")
		}
		return skip(name, "no TEE signer in this build (only private_key_jwt is affected; client_secret is unaffected)")
	}

	backend := info.Backend
	if backend == "" {
		backend = "tee"
	}

	switch {
	case probeErr != nil:
		return warn(name, fmt.Sprintf("%s signer present but probe errored: %s", backend, probeErr), "")
	case info.Available:
		if info.VendorName != "" {
			return pass(name, fmt.Sprintf("%s TEE available (%s)", backend, info.VendorName))
		}
		return pass(name, fmt.Sprintf("%s TEE available", backend))
	case usesPKJWT:
		return fail(name, fmt.Sprintf("%s signer present but TEE unavailable: %s", backend, info.Reason), teeUnavailableHint)
	default:
		return warn(name, fmt.Sprintf("%s signer present but TEE unavailable: %s", backend, info.Reason), teeUnavailableHint)
	}
}

// networkChecks probes Open API and MCP endpoints concurrently.
func networkChecks(ctx context.Context, opts *DoctorOptions, ep core.Endpoints) []checkResult {
	if opts.Offline {
		return []checkResult{
			skip("endpoint_open", "skipped (--offline)"),
			skip("endpoint_mcp", "skipped (--offline)"),
		}
	}

	// Connectivity checks are platform traffic and must exercise the same
	// provider-aware route as real platform requests.
	httpClient := transport.NewHTTPClient(0)
	mcpURL := ep.MCP + "/mcp"

	type probeResult struct {
		name string
		url  string
		err  error
	}

	var wg sync.WaitGroup
	results := make([]probeResult, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()
		defer func() { recover() }()
		results[0] = probeResult{"endpoint_open", ep.Open, probeEndpoint(ctx, httpClient, ep.Open)}
	}()
	go func() {
		defer wg.Done()
		defer func() { recover() }()
		results[1] = probeResult{"endpoint_mcp", mcpURL, probeEndpoint(ctx, httpClient, mcpURL)}
	}()
	wg.Wait()

	var checks []checkResult
	for _, r := range results {
		if r.err != nil {
			checks = append(checks, fail(r.name, fmt.Sprintf("%s unreachable: %s", r.url, r.err), "check network or proxy settings"))
		} else {
			checks = append(checks, pass(r.name, r.url+" reachable"))
		}
	}
	return checks
}

// probeEndpoint sends a HEAD request to check reachability.
func probeEndpoint(ctx context.Context, client *http.Client, url string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// checkCLIUpdate actively queries the npm registry for the latest version.
// Unlike the root-level async check, this does a synchronous fetch with timeout
// and works regardless of build version (dev builds included).
func checkCLIUpdate() []checkResult {
	latest, err := fetchLatestForDoctor()
	if err != nil {
		return []checkResult{warn("cli_update", "check failed: "+err.Error(), "")}
	}
	current := build.Version
	if update.IsNewer(latest, current) {
		return []checkResult{warn("cli_update",
			fmt.Sprintf("%s → %s available", current, latest),
			"run: lark-cli update")}
	}
	return []checkResult{pass("cli_update", latest+" (up to date)")}
}

var fetchLatestForDoctor = update.FetchLatest

func finishDoctor(f *cmdutil.Factory, checks []checkResult) error {
	allOK := true
	for _, c := range checks {
		if c.Status == "fail" {
			allOK = false
			break
		}
	}

	workspace := core.CurrentWorkspace().Display()
	// A terminal on STDOUT gets a readable report; pipes, redirects, scripts and
	// tests keep the stable JSON contract (NO_COLOR disables ANSI styling).
	// StdoutIsTerminal checks stdout specifically — IOStreams.IsTerminal reflects
	// stdin, which would wrongly send the human report into `doctor | jq`.
	if f.IOStreams.StdoutIsTerminal() {
		renderDoctorHuman(f.IOStreams.Out, workspace, checks, allOK, os.Getenv("NO_COLOR") == "")
	} else {
		output.PrintJson(f.IOStreams.Out, map[string]interface{}{
			"ok":        allOK,
			"workspace": workspace,
			"checks":    checks,
		})
	}
	if !allOK {
		return output.ErrBare(1)
	}
	return nil
}

// renderDoctorHuman writes a readable health report: one aligned line per check
// with a colored status tag, an indented hint when present, and a summary line.
func renderDoctorHuman(w io.Writer, workspace string, checks []checkResult, allOK, color bool) {
	const (
		green  = "\033[32m"
		yellow = "\033[33m"
		red    = "\033[31m"
		gray   = "\033[90m"
		bold   = "\033[1m"
		reset  = "\033[0m"
	)
	colorOf := map[string]string{"pass": green, "warn": yellow, "fail": red, "skip": gray}
	tagOf := map[string]string{"pass": "PASS", "warn": "WARN", "fail": "FAIL", "skip": "SKIP"}
	paint := func(code, s string) string {
		if !color || code == "" {
			return s
		}
		return code + s + reset
	}

	nameW := 0
	for _, c := range checks {
		if len(c.Name) > nameW {
			nameW = len(c.Name)
		}
	}

	fmt.Fprintf(w, "\n%s  (workspace: %s)\n\n", paint(bold, "lark-cli doctor"), workspace)

	var passN, warnN, failN, skipN int
	for _, c := range checks {
		tag := tagOf[c.Status]
		if tag == "" {
			tag = "????"
		}
		fmt.Fprintf(w, "  %s  %-*s  %s\n", paint(colorOf[c.Status], "["+tag+"]"), nameW, c.Name, c.Message)
		if c.Hint != "" {
			fmt.Fprintf(w, "         %-*s  %s\n", nameW, "", paint(gray, "↳ "+c.Hint))
		}
		switch c.Status {
		case "pass":
			passN++
		case "warn":
			warnN++
		case "fail":
			failN++
		case "skip":
			skipN++
		}
	}

	headline := paint(green, "healthy")
	if !allOK {
		headline = paint(red, "problems found")
	}
	fmt.Fprintf(w, "\n  %s — %d passed", headline, passN)
	if warnN > 0 {
		fmt.Fprintf(w, ", %d warning(s)", warnN)
	}
	if failN > 0 {
		fmt.Fprintf(w, ", %d failed", failN)
	}
	if skipN > 0 {
		fmt.Fprintf(w, ", %d skipped", skipN)
	}
	fmt.Fprintln(w)
}
