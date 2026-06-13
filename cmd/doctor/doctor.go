// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doctor

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/identitydiag"
	"github.com/larksuite/cli/internal/output"
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
	opts := &DoctorOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "CLI health check: config, auth, and connectivity",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Ctx = cmd.Context()
			return doctorRun(opts)
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

func doctorRun(opts *DoctorOptions) error {
	f := opts.Factory
	var checks []checkResult

	// ── 0. CLI version & update check ──
	checks = append(checks, pass("cli_version", build.Version))
	if !opts.Offline {
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
			if errors.As(core.NotConfiguredError(), &cfgErr) {
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
		if errors.As(err, &cfgErr) {
			hint = cfgErr.Hint
		}
		checks = append(checks, fail("app_resolved", err.Error(), hint))
		return finishDoctor(f, checks)
	}
	checks = append(checks, pass("app_resolved", fmt.Sprintf("app: %s (%s)", cfg.AppID, cfg.Brand)))

	ep := core.ResolveEndpoints(cfg.Brand)

	// ── 3. Identity readiness ──
	diagnostics := identitydiag.Diagnose(opts.Ctx, f, cfg, !opts.Offline)
	checks = append(checks,
		identityCheck("bot_identity", diagnostics.Bot),
		identityCheck("user_identity", diagnostics.User),
	)
	if diagnostics.Bot.Available || diagnostics.User.Available {
		checks = append(checks, pass("identity_ready", "at least one identity is available"))
	} else {
		checks = append(checks, fail("identity_ready", "no usable bot or user identity is available", "run: lark-cli auth status --verify"))
	}

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

// networkChecks probes Open API and MCP endpoints concurrently.
func networkChecks(ctx context.Context, opts *DoctorOptions, ep core.Endpoints) []checkResult {
	if opts.Offline {
		return []checkResult{
			skip("endpoint_open", "skipped (--offline)"),
			skip("endpoint_mcp", "skipped (--offline)"),
		}
	}

	// Use the shared proxy-plugin-aware transport so connectivity checks reflect
	// the real egress path (and are blocked when proxy plugin fails closed).
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
	latest, err := update.FetchLatest()
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

func finishDoctor(f *cmdutil.Factory, checks []checkResult) error {
	allOK := true
	for _, c := range checks {
		if c.Status == "fail" {
			allOK = false
			break
		}
	}

	result := map[string]interface{}{
		"ok":        allOK,
		"workspace": core.CurrentWorkspace().Display(),
		"checks":    checks,
	}
	output.PrintJson(f.IOStreams.Out, result)
	if !allOK {
		return output.ErrBare(1)
	}
	return nil
}
