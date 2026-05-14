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

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
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

	return cmd
}

// checkResult represents one diagnostic check.
type checkResult struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // "pass", "fail", "skip"
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
			var cfgErr *core.ConfigError
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
		var cfgErr *core.ConfigError
		if errors.As(err, &cfgErr) {
			hint = cfgErr.Hint
		}
		checks = append(checks, fail("app_resolved", err.Error(), hint))
		return finishDoctor(f, checks)
	}
	checks = append(checks, pass("app_resolved", fmt.Sprintf("app: %s (%s)", cfg.AppID, cfg.Brand)))

	ep := core.ResolveEndpoints(cfg.Brand)

	// ── 3. Token exists ──
	if cfg.UserOpenId == "" {
		checks = append(checks, fail("token_exists", "no user logged in", "run: lark-cli auth login --help"))
		checks = append(checks, networkChecks(opts.Ctx, opts, ep)...)
		return finishDoctor(f, checks)
	}
	stored := larkauth.GetStoredToken(cfg.AppID, cfg.UserOpenId)
	if stored == nil {
		checks = append(checks, fail("token_exists", "no token in keychain for "+cfg.UserOpenId, "run: lark-cli auth login --help"))
		checks = append(checks, networkChecks(opts.Ctx, opts, ep)...)
		return finishDoctor(f, checks)
	}
	checks = append(checks, pass("token_exists", fmt.Sprintf("token found for %s (%s)", cfg.UserName, cfg.UserOpenId)))

	// ── 4. Token local validity ──
	status := larkauth.TokenStatus(stored)
	switch status {
	case "valid":
		checks = append(checks, pass("token_local", "token valid, expires "+time.UnixMilli(stored.ExpiresAt).Format(time.RFC3339)))
	case "needs_refresh":
		checks = append(checks, pass("token_local", "token needs refresh (will auto-refresh on next call)"))
	default: // expired
		checks = append(checks, fail("token_local", "token expired", "run: lark-cli auth login --help"))
		checks = append(checks, networkChecks(opts.Ctx, opts, ep)...)
		return finishDoctor(f, checks)
	}

	// ── 5. Token server verification ──
	if opts.Offline {
		checks = append(checks, skip("token_verified", "skipped (--offline)"))
	} else {
		httpClient := mustHTTPClient(f)
		token, err := larkauth.GetValidAccessToken(httpClient, larkauth.NewUATCallOptions(cfg, f.IOStreams.ErrOut))
		if err != nil {
			checks = append(checks, fail("token_verified", "cannot obtain valid token: "+err.Error(), "run: lark-cli auth login --help"))
		} else {
			sdk, err := f.LarkClient()
			if err != nil {
				checks = append(checks, fail("token_verified", "SDK init failed: "+err.Error(), ""))
			} else if err := larkauth.VerifyUserToken(opts.Ctx, sdk, token); err != nil {
				checks = append(checks, fail("token_verified", "server rejected token: "+err.Error(), "run: lark-cli auth login --help"))
			} else {
				checks = append(checks, pass("token_verified", "server confirmed token is valid"))
			}
		}
	}

	// ── 6 & 7. Endpoint reachability ──
	checks = append(checks, networkChecks(opts.Ctx, opts, ep)...)

	return finishDoctor(f, checks)
}

// networkChecks probes Open API and MCP endpoints concurrently.
func networkChecks(ctx context.Context, opts *DoctorOptions, ep core.Endpoints) []checkResult {
	if opts.Offline {
		return []checkResult{
			skip("endpoint_open", "skipped (--offline)"),
			skip("endpoint_mcp", "skipped (--offline)"),
		}
	}

	httpClient := &http.Client{}
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

// mustHTTPClient returns f.HttpClient() or a default client.
func mustHTTPClient(f *cmdutil.Factory) *http.Client {
	c, err := f.HttpClient()
	if err != nil {
		return &http.Client{Timeout: 30 * time.Second}
	}
	return c
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
