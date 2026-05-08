// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/internal/binding"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/vfs"
)

// Candidate is the source-agnostic view of a bindable account.
// It carries only the identity fields needed by selectCandidate / TUI;
// secrets remain inside the SourceBinder implementation.
type Candidate struct {
	AppID string
	Label string
}

// SourceBinder abstracts a bind source (openclaw / hermes / future sources).
// Implementations only list candidates and build an AppConfig for a chosen
// candidate — they stay out of mode (TUI vs flag) and orchestration concerns.
type SourceBinder interface {
	// Name returns the source identifier (used in error envelopes).
	Name() string
	// ConfigPath returns the resolved path to the source's config file.
	ConfigPath() string
	// ListCandidates enumerates bindable accounts from the source config.
	// An empty slice is valid (selectCandidate will turn it into a typed error).
	ListCandidates() ([]Candidate, error)
	// Build resolves secrets, persists to keychain, and returns a ready AppConfig
	// for the chosen candidate AppID. Must be called after ListCandidates succeeds.
	Build(appID string) (*core.AppConfig, error)
}

// newBinder constructs the SourceBinder for the given source name.
func newBinder(source string, opts *BindOptions) (SourceBinder, error) {
	switch source {
	case "openclaw":
		return &openclawBinder{opts: opts, path: resolveOpenClawConfigPath()}, nil
	case "hermes":
		return &hermesBinder{opts: opts, path: resolveHermesEnvPath()}, nil
	case "lark-channel":
		return &larkChannelBinder{opts: opts, path: resolveLarkChannelConfigPath()}, nil
	default:
		return nil, output.ErrValidation("unsupported source: %s", source)
	}
}

// selectCandidate is the single source of truth for account-selection logic.
// Every bind source funnels through this function, so the "how many
// candidates × was --app-id given × is this TUI" policy is defined once.
//
// Decision matrix:
//
//	candidates=0                              → error "no app configured"
//	appID set,   match                        → selected
//	appID set,   no match                     → error + candidate list
//	candidates=1, appID=""                    → auto-select
//	candidates≥2, appID="", isTUI=true        → tuiPrompt
//	candidates≥2, appID="", isTUI=false       → error + candidate list
//
// The last branch is the one that matters for flag-mode callers: an explicit
// --source must never silently drop into an interactive prompt just because
// stdin happens to be a terminal.
func selectCandidate(
	binder SourceBinder,
	candidates []Candidate,
	appIDFlag string,
	isTUI bool,
	tuiPrompt func([]Candidate) (*Candidate, error),
) (*Candidate, error) {
	src := binder.Name()
	cfgBase := filepath.Base(binder.ConfigPath())

	if len(candidates) == 0 {
		// Reader succeeded but yielded nothing — e.g. every openclaw account
		// is disabled. Missing-file / missing-field cases return typed errors
		// from ListCandidates itself and never reach here.
		switch src {
		case "openclaw":
			return nil, output.ErrWithHint(output.ExitValidation, src,
				"no Feishu app configured in openclaw.json",
				"configure channels.feishu.appId in openclaw.json")
		default:
			return nil, output.ErrValidation("%s: no app configured", src)
		}
	}

	if appIDFlag != "" {
		for i := range candidates {
			if candidates[i].AppID == appIDFlag {
				return &candidates[i], nil
			}
		}
		return nil, output.ErrWithHint(output.ExitValidation, src,
			fmt.Sprintf("--app-id %q not found in %s", appIDFlag, cfgBase),
			fmt.Sprintf("available app IDs:\n  %s", formatCandidates(candidates)))
	}

	if len(candidates) == 1 {
		return &candidates[0], nil
	}

	if isTUI {
		return tuiPrompt(candidates)
	}

	return nil, output.ErrWithHint(output.ExitValidation, src,
		fmt.Sprintf("multiple accounts in %s; pass --app-id <id>", cfgBase),
		fmt.Sprintf("available app IDs:\n  %s", formatCandidates(candidates)))
}

// formatCandidates renders candidates as "AppID (Label)" lines for error hints.
func formatCandidates(candidates []Candidate) string {
	ids := make([]string, 0, len(candidates))
	for _, c := range candidates {
		label := c.AppID
		if c.Label != "" {
			label = fmt.Sprintf("%s (%s)", c.AppID, c.Label)
		}
		ids = append(ids, label)
	}
	return strings.Join(ids, "\n  ")
}

// ──────────────────────────────────────────────────────────────
// openclawBinder
// ──────────────────────────────────────────────────────────────

type openclawBinder struct {
	opts *BindOptions
	path string

	// Cached between ListCandidates and Build so we don't re-read / re-parse.
	cfg     *binding.OpenClawRoot
	rawApps []binding.CandidateApp
}

func (b *openclawBinder) Name() string       { return "openclaw" }
func (b *openclawBinder) ConfigPath() string { return b.path }

func (b *openclawBinder) ListCandidates() ([]Candidate, error) {
	cfg, err := binding.ReadOpenClawConfig(b.path)
	if err != nil {
		return nil, output.ErrWithHint(output.ExitValidation, "openclaw",
			fmt.Sprintf("cannot read %s: %v", b.path, err),
			"verify OpenClaw is installed and configured")
	}
	if cfg.Channels.Feishu == nil {
		return nil, output.ErrWithHint(output.ExitValidation, "openclaw",
			"openclaw.json missing channels.feishu section",
			"configure Feishu in OpenClaw first")
	}

	raw := binding.ListCandidateApps(cfg.Channels.Feishu)
	b.cfg = cfg
	b.rawApps = raw

	result := make([]Candidate, 0, len(raw))
	for _, c := range raw {
		result = append(result, Candidate{AppID: c.AppID, Label: c.Label})
	}
	return result, nil
}

func (b *openclawBinder) Build(appID string) (*core.AppConfig, error) {
	if b.cfg == nil {
		return nil, output.Errorf(output.ExitInternal, "openclaw",
			"internal: Build called before ListCandidates")
	}

	var selected *binding.CandidateApp
	for i := range b.rawApps {
		if b.rawApps[i].AppID == appID {
			selected = &b.rawApps[i]
			break
		}
	}
	if selected == nil {
		return nil, output.Errorf(output.ExitInternal, "openclaw",
			"internal: appID %q not in candidates", appID)
	}

	if selected.AppSecret.IsZero() {
		return nil, output.ErrWithHint(output.ExitValidation, "openclaw",
			fmt.Sprintf("appSecret is empty for app %s in %s", selected.AppID, b.path),
			"configure channels.feishu.appSecret in openclaw.json")
	}
	secret, err := binding.ResolveSecretInput(selected.AppSecret, b.cfg.Secrets, os.Getenv)
	if err != nil {
		return nil, output.ErrWithHint(output.ExitValidation, "openclaw",
			fmt.Sprintf("failed to resolve appSecret for %s: %v", selected.AppID, err),
			fmt.Sprintf("check appSecret configuration in %s", b.path))
	}

	stored, err := core.ForStorage(selected.AppID, core.PlainSecret(secret), b.opts.Factory.Keychain)
	if err != nil {
		return nil, output.Errorf(output.ExitInternal, "openclaw",
			"keychain unavailable: %v\nhint: use file: reference in config to bypass keychain", err)
	}

	return &core.AppConfig{
		AppId:     selected.AppID,
		AppSecret: stored,
		Brand:     core.LarkBrand(normalizeBrand(selected.Brand)),
	}, nil
}

// ──────────────────────────────────────────────────────────────
// hermesBinder
// ──────────────────────────────────────────────────────────────

type hermesBinder struct {
	opts   *BindOptions
	path   string
	envMap map[string]string // cached between ListCandidates and Build
}

func (b *hermesBinder) Name() string       { return "hermes" }
func (b *hermesBinder) ConfigPath() string { return b.path }

func (b *hermesBinder) ListCandidates() ([]Candidate, error) {
	envMap, err := readDotenv(b.path)
	if err != nil {
		return nil, output.ErrWithHint(output.ExitValidation, "hermes",
			fmt.Sprintf("failed to read Hermes config: %v", err),
			fmt.Sprintf("verify Hermes is installed and configured at %s", b.path))
	}
	appID := envMap["FEISHU_APP_ID"]
	if appID == "" {
		return nil, output.ErrWithHint(output.ExitValidation, "hermes",
			fmt.Sprintf("FEISHU_APP_ID not found in %s", b.path),
			"run 'hermes setup' to configure Feishu credentials")
	}
	b.envMap = envMap
	return []Candidate{{AppID: appID, Label: "default"}}, nil
}

func (b *hermesBinder) Build(appID string) (*core.AppConfig, error) {
	if b.envMap == nil {
		return nil, output.Errorf(output.ExitInternal, "hermes",
			"internal: Build called before ListCandidates")
	}
	if b.envMap["FEISHU_APP_ID"] != appID {
		return nil, output.Errorf(output.ExitInternal, "hermes",
			"internal: appID %q does not match env", appID)
	}
	appSecret := b.envMap["FEISHU_APP_SECRET"]
	if appSecret == "" {
		return nil, output.ErrWithHint(output.ExitValidation, "hermes",
			fmt.Sprintf("FEISHU_APP_SECRET not found in %s", b.path),
			"run 'hermes setup' to configure Feishu credentials")
	}

	stored, err := core.ForStorage(appID, core.PlainSecret(appSecret), b.opts.Factory.Keychain)
	if err != nil {
		return nil, output.Errorf(output.ExitInternal, "hermes",
			"keychain unavailable: %v\nhint: use file: reference in config to bypass keychain", err)
	}

	return &core.AppConfig{
		AppId:     appID,
		AppSecret: stored,
		Brand:     core.LarkBrand(normalizeBrand(b.envMap["FEISHU_DOMAIN"])),
	}, nil
}

// ──────────────────────────────────────────────────────────────
// larkChannelBinder
// ──────────────────────────────────────────────────────────────

type larkChannelBinder struct {
	opts *BindOptions
	path string

	// Cached between ListCandidates and Build so we don't re-read the file.
	cfg *binding.LarkChannelRoot
}

func (b *larkChannelBinder) Name() string       { return "lark-channel" }
func (b *larkChannelBinder) ConfigPath() string { return b.path }

func (b *larkChannelBinder) ListCandidates() ([]Candidate, error) {
	cfg, err := binding.ReadLarkChannelConfig(b.path)
	if err != nil {
		return nil, output.ErrWithHint(output.ExitValidation, "lark-channel",
			fmt.Sprintf("cannot read %s: %v", b.path, err),
			"verify lark-channel-bridge is installed and configured")
	}
	if cfg.Accounts.App.ID == "" {
		return nil, output.ErrWithHint(output.ExitValidation, "lark-channel",
			fmt.Sprintf("accounts.app.id missing in %s", b.path),
			"run lark-channel-bridge's setup to populate the app credential")
	}
	b.cfg = cfg
	return []Candidate{{AppID: cfg.Accounts.App.ID, Label: "default"}}, nil
}

func (b *larkChannelBinder) Build(appID string) (*core.AppConfig, error) {
	if b.cfg == nil {
		return nil, output.Errorf(output.ExitInternal, "lark-channel",
			"internal: Build called before ListCandidates")
	}
	if b.cfg.Accounts.App.ID != appID {
		return nil, output.Errorf(output.ExitInternal, "lark-channel",
			"internal: appID %q does not match config", appID)
	}
	if b.cfg.Accounts.App.Secret == "" {
		return nil, output.ErrWithHint(output.ExitValidation, "lark-channel",
			fmt.Sprintf("accounts.app.secret is empty in %s", b.path),
			"run lark-channel-bridge's setup to populate the app credential")
	}

	stored, err := core.ForStorage(appID, core.PlainSecret(b.cfg.Accounts.App.Secret), b.opts.Factory.Keychain)
	if err != nil {
		return nil, output.Errorf(output.ExitInternal, "lark-channel",
			"keychain unavailable: %v", err)
	}

	return &core.AppConfig{
		AppId:     appID,
		AppSecret: stored,
		Brand:     core.LarkBrand(normalizeBrand(b.cfg.Accounts.App.Tenant)),
	}, nil
}

// ──────────────────────────────────────────────────────────────
// Source-specific helpers (path / dotenv / brand) — kept private to this package.
// Moved here from bind.go so bind.go can focus on orchestration.
// ──────────────────────────────────────────────────────────────

// sourceDisplayName returns the user-facing label for a source identifier,
// matching the casing used in bind_messages.go (OpenClaw / Hermes).
func sourceDisplayName(source string) string {
	switch source {
	case "openclaw":
		return "OpenClaw"
	case "hermes":
		return "Hermes"
	case "lark-channel":
		return "Lark Channel"
	default:
		return source
	}
}

// normalizeBrand applies .strip().lower() and defaults to "feishu".
// Aligns with Hermes gateway/platforms/feishu.py:1119 behavior.
func normalizeBrand(raw string) string {
	s := strings.TrimSpace(strings.ToLower(raw))
	if s == "" {
		return "feishu"
	}
	return s
}

// resolveHermesEnvPath returns the path to Hermes's .env file.
// Respects HERMES_HOME override; defaults to ~/.hermes/.env.
//
// Note: HERMES_HOME is typically unset when users run bind from a regular
// terminal. When AI agents execute bind within a Hermes subprocess, HERMES_HOME
// may be set and should be respected.
func resolveHermesEnvPath() string {
	hermesHome := os.Getenv("HERMES_HOME")
	if hermesHome == "" {
		home, err := vfs.UserHomeDir()
		if err != nil || home == "" {
			fmt.Fprintf(os.Stderr, "warning: unable to determine home directory: %v\n", err)
		}
		hermesHome = filepath.Join(home, ".hermes")
	}
	return filepath.Join(hermesHome, ".env")
}

// resolveLarkChannelConfigPath returns the path to lark-channel-bridge's
// config.json. Mirrors the bridge's src/config/paths.ts which hardcodes
// ~/.lark-channel/config.json with no env override — multi-instance is not
// a supported scenario today.
func resolveLarkChannelConfigPath() string {
	home, err := vfs.UserHomeDir()
	if err != nil || home == "" {
		fmt.Fprintf(os.Stderr, "warning: unable to determine home directory: %v\n", err)
	}
	return filepath.Join(home, ".lark-channel", "config.json")
}

// resolveOpenClawConfigPath resolves openclaw.json path using the same priority
// chain as OpenClaw's src/config/paths.ts:
//  1. OPENCLAW_CONFIG_PATH env → exact file path
//  2. OPENCLAW_STATE_DIR env → <dir>/openclaw.json
//  3. OPENCLAW_HOME env → <home>/.openclaw/openclaw.json
//  4. ~/.openclaw/openclaw.json (default)
//  5. Legacy: ~/.clawdbot/clawdbot.json, ~/.openclaw/clawdbot.json
func resolveOpenClawConfigPath() string {
	if p := os.Getenv("OPENCLAW_CONFIG_PATH"); p != "" {
		return expandHome(p)
	}

	if stateDir := os.Getenv("OPENCLAW_STATE_DIR"); stateDir != "" {
		dir := expandHome(stateDir)
		return findConfigInDir(dir)
	}

	home := os.Getenv("OPENCLAW_HOME")
	if home == "" {
		h, err := vfs.UserHomeDir()
		if err != nil || h == "" {
			fmt.Fprintf(os.Stderr, "warning: unable to determine home directory: %v\n", err)
		}
		home = h
	} else {
		home = expandHome(home)
	}

	newDir := filepath.Join(home, ".openclaw")
	if configFile := findConfigInDir(newDir); fileExists(configFile) {
		return configFile
	}

	legacyDir := filepath.Join(home, ".clawdbot")
	if configFile := findConfigInDir(legacyDir); fileExists(configFile) {
		return configFile
	}

	return filepath.Join(newDir, "openclaw.json")
}

func findConfigInDir(dir string) string {
	primary := filepath.Join(dir, "openclaw.json")
	if fileExists(primary) {
		return primary
	}
	legacy := filepath.Join(dir, "clawdbot.json")
	if fileExists(legacy) {
		return legacy
	}
	return primary
}

func fileExists(path string) bool {
	_, err := vfs.Stat(path)
	return err == nil
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := vfs.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[1:])
	}
	return path
}

// readDotenv reads a KEY=VALUE .env file. Comments (#) and blank lines skipped.
// Matches Hermes's load_env() in hermes_cli/config.py.
func readDotenv(path string) (map[string]string, error) {
	data, err := vfs.ReadFile(path)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		if key != "" {
			result[key] = value
		}
	}
	return result, nil
}
