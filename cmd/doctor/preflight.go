// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doctor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	internalauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts"
	"github.com/larksuite/cli/shortcuts/common"
)

// DoctorPreflightOptions holds inputs for doctor preflight.
type DoctorPreflightOptions struct {
	Factory     *cmdutil.Factory
	Ctx         context.Context
	Service     string
	Shortcut    string
	RequestedAs string
	Format      string
}

type preflightTarget struct {
	Service   string   `json:"service"`
	Command   string   `json:"command"`
	Risk      string   `json:"risk"`
	AuthTypes []string `json:"auth_types"`
	Scopes    []string `json:"scopes"`
}

type preflightIdentity struct {
	Requested string `json:"requested"`
	Resolved  string `json:"resolved"`
	Source    string `json:"source"`
}

type preflightCheck struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Blocking bool   `json:"blocking"`
	Message  string `json:"message"`
	Hint     string `json:"hint,omitempty"`
}

type preflightAction struct {
	Type     string `json:"type"`
	Blocking bool   `json:"blocking"`
	Command  string `json:"command,omitempty"`
	Reason   string `json:"reason"`
}

type preflightResult struct {
	OK          bool              `json:"ok"`
	Ready       bool              `json:"ready"`
	Workspace   string            `json:"workspace"`
	Target      preflightTarget   `json:"target"`
	Identity    preflightIdentity `json:"identity"`
	Checks      []preflightCheck  `json:"checks"`
	NextActions []preflightAction `json:"next_actions,omitempty"`
	Notice      map[string]any    `json:"_notice,omitempty"`
}

func NewCmdDoctorPreflight(f *cmdutil.Factory) *cobra.Command {
	opts := &DoctorPreflightOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "preflight <service> <shortcut>",
		Short: "Check whether a shortcut is ready before execution",
		Long: `Check whether a shortcut is ready before execution.

This command does not execute the target shortcut. It evaluates config,
identity, strict mode, login, scope readiness, and risk hints so users
and AI agents can decide the next action before running the real command.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Ctx = cmd.Context()
			opts.Service = args[0]
			opts.Shortcut = args[1]
			if !slices.Contains([]string{"json", "pretty"}, opts.Format) {
				return output.ErrValidation("invalid --format %q: must be json or pretty", opts.Format)
			}
			return doctorPreflightRun(opts)
		},
	}
	cmd.Flags().StringVar(&opts.RequestedAs, "as", "auto", "identity to evaluate (auto|user|bot)")
	cmd.Flags().StringVar(&opts.Format, "format", "json", "output format: json (default) | pretty")
	return cmd
}

func doctorPreflightRun(opts *DoctorPreflightOptions) error {
	shortcut, err := resolveTargetShortcut(opts.Service, opts.Shortcut)
	if err != nil {
		return err
	}

	target := preflightTarget{
		Service:   shortcut.Service,
		Command:   shortcut.Command,
		Risk:      shortcutRisk(shortcut),
		AuthTypes: shortcutAuthTypes(shortcut),
	}
	identity := preflightIdentity{
		Requested: normalizedRequestedIdentity(opts.RequestedAs),
	}

	cfg, err := resolvePreflightConfig(opts.Factory)
	if err != nil {
		result := buildConfigFailureResult(target, identity, err)
		writePreflightResult(opts.Factory.IOStreams.Out, result, opts.Format)
		return output.ErrBare(1)
	}

	resolvedAs, source, err := resolvePreflightIdentity(opts, cfg)
	if err != nil {
		return err
	}
	target.Scopes = shortcut.ScopesForIdentity(string(resolvedAs))
	identity.Resolved = string(resolvedAs)
	identity.Source = source

	checks, actions := runPreflightChecks(opts, cfg, shortcut, resolvedAs, target.Scopes)
	ready := isPreflightReady(checks)
	result := preflightResult{
		OK:          true,
		Ready:       ready,
		Workspace:   core.CurrentWorkspace().Display(),
		Target:      target,
		Identity:    identity,
		Checks:      checks,
		NextActions: actions,
		Notice:      output.GetNotice(),
	}

	writePreflightResult(opts.Factory.IOStreams.Out, result, opts.Format)
	if !ready {
		return output.ErrBare(1)
	}
	return nil
}

func resolveTargetShortcut(service, command string) (*common.Shortcut, error) {
	service = strings.TrimSpace(service)
	command = strings.TrimSpace(command)
	for _, shortcut := range shortcuts.AllShortcuts() {
		if shortcut.Service == service && shortcut.Command == command {
			sc := shortcut
			return &sc, nil
		}
	}

	var available []string
	for _, shortcut := range shortcuts.AllShortcuts() {
		if shortcut.Service == service {
			available = append(available, shortcut.Command)
		}
	}
	if len(available) == 0 {
		return nil, output.ErrValidation("unknown shortcut target %q %q", service, command)
	}
	return nil, output.ErrWithHint(output.ExitValidation, "target_not_found",
		fmt.Sprintf("shortcut %q not found in service %q", command, service),
		fmt.Sprintf("available shortcuts for %s: %s", service, strings.Join(available, ", ")),
	)
}

func resolvePreflightConfig(f *cmdutil.Factory) (*core.CliConfig, error) {
	multi, err := core.LoadOrNotConfigured()
	if err != nil {
		return nil, err
	}
	cfg, err := core.ResolveConfigFromMulti(multi, f.Keychain, f.Invocation.Profile)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func resolvePreflightIdentity(opts *DoctorPreflightOptions, cfg *core.CliConfig) (core.Identity, string, error) {
	requested := core.Identity(strings.TrimSpace(opts.RequestedAs))
	if requested == "" {
		requested = core.AsAuto
	}
	switch requested {
	case core.AsAuto, core.AsUser, core.AsBot:
	default:
		return "", "", output.ErrValidation("invalid --as %q: must be auto, user, or bot", opts.RequestedAs)
	}

	cmd := &cobra.Command{Use: "preflight"}
	cmd.Flags().String("as", "auto", "")
	if requested != core.AsAuto {
		_ = cmd.Flags().Set("as", string(requested))
	}

	resolved := opts.Factory.ResolveAs(opts.Ctx, cmd, requested)
	source := "auto_detect"
	if cmd.Flags().Changed("as") && requested != core.AsAuto {
		source = "explicit_as"
	} else if forced := opts.Factory.ResolveStrictMode(opts.Ctx).ForcedIdentity(); forced != "" {
		source = "strict_mode"
	} else if hint, err := opts.Factory.Credential.ResolveIdentityHint(opts.Ctx); err == nil && hint != nil && hint.DefaultAs != "" && hint.DefaultAs != core.AsAuto {
		source = "default_as"
	} else if cfg.DefaultAs != "" && cfg.DefaultAs != core.AsAuto {
		source = "default_as"
	}
	return resolved, source, nil
}

func runPreflightChecks(opts *DoctorPreflightOptions, cfg *core.CliConfig, shortcut *common.Shortcut, resolvedAs core.Identity, requiredScopes []string) ([]preflightCheck, []preflightAction) {
	var checks []preflightCheck
	var actions []preflightAction

	checks = append(checks, preflightCheck{
		Name:     "config_ready",
		Status:   "pass",
		Blocking: false,
		Message:  fmt.Sprintf("profile %s resolved for app %s (%s)", cfg.ProfileName, cfg.AppID, cfg.Brand),
	})

	mode := opts.Factory.ResolveStrictMode(opts.Ctx)
	if mode.IsActive() && !mode.AllowsIdentity(resolvedAs) {
		checks = append(checks, preflightCheck{
			Name:     "strict_mode",
			Status:   "fail",
			Blocking: true,
			Message:  fmt.Sprintf("strict mode is %q, only %s identity is allowed", mode, mode.ForcedIdentity()),
			Hint:     "see `lark-cli config strict-mode --help` before switching identity policy",
		})
		actions = append(actions, preflightAction{
			Type:     "strict_mode_help",
			Blocking: true,
			Command:  "lark-cli config strict-mode --help",
			Reason:   "current profile policy blocks the requested identity",
		})
		return appendRiskHints(checks, actions, shortcut, opts.Service, opts.Shortcut)
	}
	if mode.IsActive() {
		checks = append(checks, preflightCheck{
			Name:     "strict_mode",
			Status:   "pass",
			Blocking: false,
			Message:  fmt.Sprintf("strict mode %q allows identity %s", mode, resolvedAs),
		})
	} else {
		checks = append(checks, preflightCheck{
			Name:     "strict_mode",
			Status:   "pass",
			Blocking: false,
			Message:  "strict mode is off",
		})
	}

	if err := opts.Factory.CheckIdentity(resolvedAs, shortcutAuthTypes(shortcut)); err != nil {
		checks = append(checks, preflightCheck{
			Name:     "identity_supported",
			Status:   "fail",
			Blocking: true,
			Message:  err.Error(),
		})
		actions = append(actions, preflightAction{
			Type:     "switch_identity",
			Blocking: true,
			Reason:   "choose an identity supported by the target shortcut",
		})
		return checks, actions
	}
	checks = append(checks, preflightCheck{
		Name:     "identity_supported",
		Status:   "pass",
		Blocking: false,
		Message:  fmt.Sprintf("shortcut supports resolved identity %s", resolvedAs),
	})

	if resolvedAs == core.AsUser {
		tokenResult, tokenCheck, tokenAction, canCheckScopes := evaluateUserTokenReadiness(opts, cfg, requiredScopes)
		checks = append(checks, tokenCheck)
		if tokenAction != nil {
			actions = append(actions, *tokenAction)
		}
		if tokenCheck.Blocking && tokenCheck.Status == "fail" {
			return appendRiskHints(checks, actions, shortcut, opts.Service, opts.Shortcut)
		}
		if canCheckScopes {
			scopeCheck, scopeAction := evaluateScopeReadiness(requiredScopes, tokenResult)
			checks = append(checks, scopeCheck)
			if scopeAction != nil {
				actions = append(actions, *scopeAction)
			}
		} else {
			checks = append(checks, preflightCheck{
				Name:     "scope_ready",
				Status:   "unknown",
				Blocking: false,
				Message:  "scope metadata is unavailable for the current token",
				Hint:     "run the target shortcut once if you need the server-side scope error details",
			})
		}
	} else {
		checks = append(checks, preflightCheck{
			Name:     "login_ready",
			Status:   "skip",
			Blocking: false,
			Message:  "bot identity does not require user login",
		})
		checks = append(checks, preflightCheck{
			Name:     "scope_ready",
			Status:   "unknown",
			Blocking: false,
			Message:  "bot app scope cannot be fully verified locally",
			Hint:     "if execution later returns a permission error, enable the required scope in developer console via console_url",
		})
	}

	return appendRiskHints(checks, actions, shortcut, opts.Service, opts.Shortcut)
}

func evaluateUserTokenReadiness(opts *DoctorPreflightOptions, cfg *core.CliConfig, requiredScopes []string) (*credential.TokenResult, preflightCheck, *preflightAction, bool) {
	loginCommand := buildAuthLoginCommand(requiredScopes)
	if cfg.UserOpenId == "" {
		return nil, preflightCheck{
				Name:     "login_ready",
				Status:   "fail",
				Blocking: true,
				Message:  "no user logged in",
				Hint:     fmt.Sprintf("run `%s`", loginCommand),
			}, &preflightAction{
				Type:     "auth_login",
				Blocking: true,
				Command:  loginCommand,
				Reason:   "user login is required for this shortcut",
			}, false
	}

	tokenResult, err := opts.Factory.Credential.ResolveToken(opts.Ctx, credential.NewTokenSpec(core.AsUser, cfg.AppID))
	if err != nil {
		return nil, preflightCheck{
				Name:     "login_ready",
				Status:   "fail",
				Blocking: true,
				Message:  fmt.Sprintf("cannot resolve user token: %v", err),
				Hint:     fmt.Sprintf("run `%s` to re-authorize the user token", loginCommand),
			}, &preflightAction{
				Type:     "auth_login",
				Blocking: true,
				Command:  loginCommand,
				Reason:   "user token is missing or cannot be refreshed",
			}, false
	}

	statusMsg := fmt.Sprintf("user token resolved for %s (%s)", cfg.UserName, cfg.UserOpenId)
	if stored := internalauth.GetStoredToken(cfg.AppID, cfg.UserOpenId); stored != nil {
		switch internalauth.TokenStatus(stored) {
		case "valid":
			statusMsg = fmt.Sprintf("user token is valid for %s (%s)", cfg.UserName, cfg.UserOpenId)
		case "needs_refresh":
			statusMsg = fmt.Sprintf("user token will refresh on next call for %s (%s)", cfg.UserName, cfg.UserOpenId)
		default:
			return nil, preflightCheck{
					Name:     "login_ready",
					Status:   "fail",
					Blocking: true,
					Message:  "stored user token is expired",
					Hint:     fmt.Sprintf("run `%s`", loginCommand),
				}, &preflightAction{
					Type:     "auth_login",
					Blocking: true,
					Command:  loginCommand,
					Reason:   "stored user token expired and needs re-authorization",
				}, false
		}
	}

	return tokenResult, preflightCheck{
		Name:     "login_ready",
		Status:   "pass",
		Blocking: false,
		Message:  statusMsg,
	}, nil, true
}

func evaluateScopeReadiness(requiredScopes []string, tokenResult *credential.TokenResult) (preflightCheck, *preflightAction) {
	if len(requiredScopes) == 0 {
		return preflightCheck{
			Name:     "scope_ready",
			Status:   "pass",
			Blocking: false,
			Message:  "shortcut does not declare extra user scopes",
		}, nil
	}
	if tokenResult == nil || strings.TrimSpace(tokenResult.Scopes) == "" {
		return preflightCheck{
			Name:     "scope_ready",
			Status:   "unknown",
			Blocking: false,
			Message:  "scope metadata is unavailable for the current token",
			Hint:     "run the target shortcut once if you need the server-side scope error details",
		}, nil
	}

	missing := internalauth.MissingScopes(tokenResult.Scopes, requiredScopes)
	if len(missing) == 0 {
		return preflightCheck{
			Name:     "scope_ready",
			Status:   "pass",
			Blocking: false,
			Message:  fmt.Sprintf("required scopes already granted: %s", strings.Join(requiredScopes, ", ")),
		}, nil
	}

	loginCommand := buildAuthLoginCommand(missing)
	return preflightCheck{
			Name:     "scope_ready",
			Status:   "fail",
			Blocking: true,
			Message:  fmt.Sprintf("missing required scope(s): %s", strings.Join(missing, ", ")),
			Hint:     fmt.Sprintf("run `%s`", loginCommand),
		}, &preflightAction{
			Type:     "auth_login",
			Blocking: true,
			Command:  loginCommand,
			Reason:   "grant the missing user scopes before executing the shortcut",
		}
}

func appendRiskHints(checks []preflightCheck, actions []preflightAction, shortcut *common.Shortcut, service, command string) ([]preflightCheck, []preflightAction) {
	baseCmd := fmt.Sprintf("lark-cli %s %s", service, command)
	switch shortcutRisk(shortcut) {
	case "high-risk-write":
		checks = append(checks, preflightCheck{
			Name:     "risk",
			Status:   "warn",
			Blocking: false,
			Message:  "high-risk write shortcut: preview the request before execution",
			Hint:     fmt.Sprintf("run `%s --dry-run` first; the real execution also requires `--yes`", baseCmd),
		})
		actions = append(actions, preflightAction{
			Type:     "dry_run",
			Blocking: false,
			Command:  baseCmd + " --dry-run",
			Reason:   "preview the high-risk request before confirming with --yes",
		})
	case "write":
		checks = append(checks, preflightCheck{
			Name:     "risk",
			Status:   "warn",
			Blocking: false,
			Message:  "write shortcut: dry-run is recommended before execution",
			Hint:     fmt.Sprintf("run `%s --dry-run` first and then execute with the required business flags", baseCmd),
		})
		actions = append(actions, preflightAction{
			Type:     "dry_run",
			Blocking: false,
			Command:  baseCmd + " --dry-run",
			Reason:   "preview the outgoing request before executing the write shortcut",
		})
	default:
		checks = append(checks, preflightCheck{
			Name:     "risk",
			Status:   "pass",
			Blocking: false,
			Message:  "read-only shortcut",
		})
	}
	return checks, actions
}

func isPreflightReady(checks []preflightCheck) bool {
	for _, check := range checks {
		if check.Blocking && check.Status == "fail" {
			return false
		}
	}
	return true
}

func writePreflightResult(w io.Writer, result preflightResult, format string) {
	if format == "pretty" {
		renderPreflightPretty(w, result)
		return
	}
	output.PrintJson(w, result)
}

func renderPreflightPretty(w io.Writer, result preflightResult) {
	status := "READY"
	if !result.Ready {
		status = "NOT READY"
	}
	fmt.Fprintf(w, "Shortcut Preflight: %s\n", status)
	fmt.Fprintf(w, "Workspace: %s\n", result.Workspace)
	fmt.Fprintf(w, "Target: %s %s\n", result.Target.Service, result.Target.Command)
	fmt.Fprintf(w, "Identity: requested=%s resolved=%s (%s)\n", result.Identity.Requested, result.Identity.Resolved, result.Identity.Source)
	if len(result.Target.Scopes) > 0 {
		fmt.Fprintf(w, "Scopes: %s\n", strings.Join(result.Target.Scopes, ", "))
	}
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Checks:")
	for _, check := range result.Checks {
		blocking := ""
		if check.Blocking {
			blocking = " [blocking]"
		}
		fmt.Fprintf(w, "  - [%s]%s %s: %s\n", check.Status, blocking, check.Name, check.Message)
		if check.Hint != "" {
			fmt.Fprintf(w, "      hint: %s\n", check.Hint)
		}
	}
	if len(result.NextActions) == 0 {
		return
	}
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Next Actions:")
	for _, action := range result.NextActions {
		fmt.Fprintf(w, "  - %s: %s\n", action.Type, action.Reason)
		if action.Command != "" {
			fmt.Fprintf(w, "      command: %s\n", action.Command)
		}
	}
}

func buildConfigFailureResult(target preflightTarget, identity preflightIdentity, err error) preflightResult {
	check := preflightCheck{
		Name:     "config_ready",
		Status:   "fail",
		Blocking: true,
		Message:  err.Error(),
	}
	var cfgErr *core.ConfigError
	if errors.As(err, &cfgErr) {
		check.Message = cfgErr.Message
		check.Hint = cfgErr.Hint
	}

	var actions []preflightAction
	switch {
	case core.CurrentWorkspace().IsLocal():
		actions = append(actions, preflightAction{
			Type:     "config_init",
			Blocking: true,
			Command:  "lark-cli config init --new",
			Reason:   "initialize local app configuration before running the shortcut",
		})
	case !core.CurrentWorkspace().IsLocal():
		actions = append(actions, preflightAction{
			Type:     "config_bind_help",
			Blocking: true,
			Command:  "lark-cli config bind --help",
			Reason:   "bind lark-cli to the Agent workspace before running the shortcut",
		})
	}

	return preflightResult{
		OK:          true,
		Ready:       false,
		Workspace:   core.CurrentWorkspace().Display(),
		Target:      target,
		Identity:    identity,
		Checks:      []preflightCheck{check},
		NextActions: actions,
		Notice:      output.GetNotice(),
	}
}

func shortcutRisk(shortcut *common.Shortcut) string {
	if shortcut == nil || shortcut.Risk == "" {
		return "read"
	}
	return shortcut.Risk
}

func shortcutAuthTypes(shortcut *common.Shortcut) []string {
	if shortcut == nil || len(shortcut.AuthTypes) == 0 {
		return []string{"user"}
	}
	return append([]string(nil), shortcut.AuthTypes...)
}

func buildAuthLoginCommand(scopes []string) string {
	if len(scopes) == 0 {
		return "lark-cli auth login --help"
	}
	return fmt.Sprintf("lark-cli auth login --scope %q", strings.Join(scopes, " "))
}

func normalizedRequestedIdentity(requested string) string {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return string(core.AsAuto)
	}
	return requested
}
