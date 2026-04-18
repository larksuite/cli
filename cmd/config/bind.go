// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

// BindOptions holds all inputs for config bind.
type BindOptions struct {
	Factory      *cmdutil.Factory
	Source       string
	AppID        string
	StrictMode   string
	DefaultAs    string
	Lang         string
	langExplicit bool // true when --lang was explicitly passed
	Force        bool

	// IsTUI is the resolved interactive-mode flag: true only when Source is
	// empty and stdin is a terminal. Computed once at the top of
	// configBindRun; downstream branches read this instead of rechecking
	// IOStreams.IsTerminal. Do not set from outside — it is overwritten.
	IsTUI bool
}

// NewCmdConfigBind creates the config bind subcommand.
func NewCmdConfigBind(f *cmdutil.Factory, runF func(*BindOptions) error) *cobra.Command {
	opts := &BindOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "bind",
		Short: "Bind Agent config to a workspace (source / app-id / force)",
		Long: `Bind an AI Agent's (OpenClaw / Hermes) Feishu credentials to a lark-cli workspace.

For AI agents: pass --source and --app-id to bind non-interactively.
Credentials are synced once; subsequent calls in the Agent's process
context automatically use the bound workspace.`,
		Example: `  lark-cli config bind --source openclaw --app-id <id>
  lark-cli config bind --source hermes
  lark-cli config bind --source openclaw --force`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.langExplicit = cmd.Flags().Changed("lang")
			if runF != nil {
				return runF(opts)
			}
			return configBindRun(opts)
		},
	}

	cmd.Flags().StringVar(&opts.Source, "source", "", "Agent source to bind from (openclaw|hermes)")
	cmd.Flags().StringVar(&opts.AppID, "app-id", "", "App ID to bind (required for OpenClaw multi-account)")
	cmd.Flags().StringVar(&opts.StrictMode, "strict-mode", "", "strict mode policy (bot|user|off)")
	cmd.Flags().StringVar(&opts.DefaultAs, "default-as", "", "default identity (user|bot|auto)")
	cmd.Flags().StringVar(&opts.Lang, "lang", "zh", "language for interactive prompts (zh|en)")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "force bind even if workspace already exists")

	return cmd
}

// configBindRun is the top-level orchestrator. Each step delegates to a named
// helper whose signature declares its contract; the body reads as the shape of
// the bind flow itself, not its mechanics.
func configBindRun(opts *BindOptions) error {
	if err := validateBindFlags(opts); err != nil {
		return err
	}

	// Decide TUI-vs-flag mode exactly once; every downstream branch reads
	// opts.IsTUI instead of re-checking IOStreams.IsTerminal.
	opts.IsTUI = opts.Source == "" && opts.Factory.IOStreams.IsTerminal

	source, err := finalizeSource(opts)
	if err != nil {
		return err
	}
	core.SetCurrentWorkspace(core.Workspace(source))
	targetConfigPath := core.GetConfigPath()

	existing, err := reconcileExistingBinding(opts, source, targetConfigPath)
	if err != nil {
		return err
	}
	if existing.Cancelled {
		return nil
	}

	if opts.IsTUI {
		if err := tuiSecurityDisclaimer(opts, source); err != nil {
			return err
		}
	}

	appConfig, err := resolveAccount(opts, source)
	if err != nil {
		return err
	}

	if err := promptMissingPreferences(opts); err != nil {
		return err
	}
	applyPreferences(appConfig, opts)

	return commitBinding(opts, appConfig, existing.ConfigBytes, source, targetConfigPath)
}

// existingBinding is the outcome of checking whether a workspace was already
// bound. ConfigBytes is non-nil iff a previous binding existed (and the caller
// should pass it to commitBinding for stale-keychain cleanup after the new
// config is durably written). Cancelled is true iff the user declined to
// replace it in the TUI prompt; the caller should exit cleanly.
type existingBinding struct {
	ConfigBytes []byte
	Cancelled   bool
}

// finalizeSource returns the validated bind source. In TUI mode it prompts
// first for language (if --lang was not explicit) then for the source itself.
// In flag mode it requires --source to be set and to be a known value.
func finalizeSource(opts *BindOptions) (string, error) {
	source := strings.TrimSpace(strings.ToLower(opts.Source))

	if opts.IsTUI {
		if !opts.langExplicit {
			lang, err := promptLangSelection("")
			if err != nil {
				if err == huh.ErrUserAborted {
					return "", output.ErrBare(1)
				}
				return "", err
			}
			opts.Lang = lang
		}
		picked, err := tuiSelectSource(opts)
		if err != nil {
			return "", err
		}
		source = picked
	} else if source == "" {
		return "", output.ErrWithHint(output.ExitValidation, "bind",
			"--source is required (openclaw or hermes)",
			"lark-cli config bind --source openclaw")
	}

	if source != "openclaw" && source != "hermes" {
		return "", output.ErrValidation("invalid --source %q; valid values: openclaw, hermes", source)
	}
	return source, nil
}

// reconcileExistingBinding reads any existing config at configPath and decides
// how to proceed: proceed with --force, prompt the user in TUI, or fail in
// flag mode. See existingBinding for the returned fields.
func reconcileExistingBinding(opts *BindOptions, source, configPath string) (existingBinding, error) {
	oldConfigData, _ := vfs.ReadFile(configPath)
	if oldConfigData == nil {
		return existingBinding{}, nil
	}

	if opts.Force {
		return existingBinding{ConfigBytes: oldConfigData}, nil
	}

	if opts.IsTUI {
		action, err := tuiConflictPrompt(opts, source, configPath)
		if err != nil {
			return existingBinding{}, err
		}
		if action == "cancel" {
			msg := getBindMsg(opts.Lang)
			fmt.Fprintln(opts.Factory.IOStreams.ErrOut, msg.ConflictCancelled)
			return existingBinding{Cancelled: true}, nil
		}
		return existingBinding{ConfigBytes: oldConfigData}, nil
	}

	return existingBinding{}, output.ErrWithHint(output.ExitValidation, "bind",
		fmt.Sprintf("workspace %q already bound at %s", source, configPath),
		"pass --force to replace, or run 'lark-cli config bind' (no flags) for interactive mode")
}

// resolveAccount runs the source-agnostic bind flow: construct the binder,
// enumerate candidates, pick one via the shared decision layer, and build a
// ready-to-persist AppConfig. Adding a new bind source only requires
// implementing SourceBinder — none of the logic below needs to change.
func resolveAccount(opts *BindOptions, source string) (*core.AppConfig, error) {
	binder, err := newBinder(source, opts)
	if err != nil {
		return nil, err
	}
	candidates, err := binder.ListCandidates()
	if err != nil {
		return nil, err
	}
	picked, err := selectCandidate(binder, candidates, opts.AppID, opts.IsTUI,
		func(cs []Candidate) (*Candidate, error) { return tuiSelectApp(opts, cs) })
	if err != nil {
		return nil, err
	}
	return binder.Build(picked.AppID)
}

// promptMissingPreferences asks the user for default-as and strict-mode in
// TUI mode, but only for fields not already set via flags. No-op in flag mode.
func promptMissingPreferences(opts *BindOptions) error {
	if !opts.IsTUI {
		return nil
	}
	if opts.DefaultAs == "" {
		da, err := tuiSelectDefaultAs(opts)
		if err != nil {
			return err
		}
		opts.DefaultAs = da
	}
	if opts.StrictMode == "" {
		sm, err := tuiSelectStrictMode(opts)
		if err != nil {
			return err
		}
		opts.StrictMode = sm
	}
	return nil
}

// applyPreferences copies the validated CLI / TUI preferences onto the
// AppConfig that binder.Build produced. Flag values have already been
// validated by validateBindFlags.
func applyPreferences(appConfig *core.AppConfig, opts *BindOptions) {
	if opts.StrictMode != "" {
		sm := core.StrictMode(opts.StrictMode)
		appConfig.StrictMode = &sm
	}
	if opts.DefaultAs != "" {
		appConfig.DefaultAs = core.Identity(opts.DefaultAs)
	}
	if opts.Lang != "" {
		appConfig.Lang = opts.Lang
	}
}

// commitBinding finalizes the bind: atomic write of the new workspace config,
// best-effort cleanup of stale keychain entries from the previous binding (if
// any), and a JSON success envelope. Cleanup runs only after the new config
// is durably written — if anything fails earlier, the old workspace stays
// usable.
func commitBinding(opts *BindOptions, appConfig *core.AppConfig, previousConfigBytes []byte, source, configPath string) error {
	multi := &core.MultiAppConfig{Apps: []core.AppConfig{*appConfig}}

	if err := vfs.MkdirAll(core.GetConfigDir(), 0700); err != nil {
		return output.Errorf(output.ExitInternal, "bind",
			"failed to create workspace directory: %v", err)
	}
	data, err := json.MarshalIndent(multi, "", "  ")
	if err != nil {
		return output.Errorf(output.ExitInternal, "bind",
			"failed to marshal config: %v", err)
	}
	if err := validate.AtomicWrite(configPath, append(data, '\n'), 0600); err != nil {
		return output.Errorf(output.ExitInternal, "bind",
			"failed to write config %s: %v", configPath, err)
	}

	if previousConfigBytes != nil {
		cleanupKeychainFromData(opts.Factory.Keychain, previousConfigBytes)
	}

	resultJSON, _ := json.Marshal(map[string]interface{}{
		"ok":          true,
		"workspace":   source,
		"app_id":      appConfig.AppId,
		"config_path": configPath,
	})
	fmt.Fprintln(opts.Factory.IOStreams.Out, string(resultJSON))
	return nil
}

// cleanupKeychainFromData removes keychain entries referenced by a previous
// config snapshot. Best-effort: errors are silently ignored (same contract as
// config init's cleanup).
func cleanupKeychainFromData(kc keychain.KeychainAccess, data []byte) {
	var multi core.MultiAppConfig
	if err := json.Unmarshal(data, &multi); err != nil {
		return
	}
	for _, app := range multi.Apps {
		core.RemoveSecretStore(app.AppSecret, kc)
	}
}

// ──────────────────────────────────────────────────────────────
// TUI helpers (huh forms, matching config init interactive style)
// ──────────────────────────────────────────────────────────────

// tuiSelectSource prompts user to choose bind source.
func tuiSelectSource(opts *BindOptions) (string, error) {
	msg := getBindMsg(opts.Lang)
	var source string

	// Pre-select based on detected env signals
	detected := core.DetectWorkspaceFromEnv(os.Getenv)
	switch detected {
	case core.WorkspaceOpenClaw:
		source = "openclaw"
	case core.WorkspaceHermes:
		source = "hermes"
	default:
		source = "openclaw" // default first option
	}

	// Resolve actual paths for display
	openclawPath := resolveOpenClawConfigPath()
	hermesEnvPath := resolveHermesEnvPath()

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(msg.SelectSource).
				Description(msg.SelectSourceDesc).
				Options(
					huh.NewOption(fmt.Sprintf(msg.SourceOpenClaw, openclawPath), "openclaw"),
					huh.NewOption(fmt.Sprintf(msg.SourceHermes, hermesEnvPath), "hermes"),
				).
				Value(&source),
		),
	).WithTheme(cmdutil.ThemeFeishu())

	if err := form.Run(); err != nil {
		if err == huh.ErrUserAborted {
			return "", output.ErrBare(1)
		}
		return "", err
	}
	return source, nil
}

// tuiSelectApp prompts the user to choose from multiple account candidates.
// Invoked only via selectCandidate's tuiPrompt callback, and only in TUI mode.
func tuiSelectApp(opts *BindOptions, candidates []Candidate) (*Candidate, error) {
	msg := getBindMsg(opts.Lang)
	options := make([]huh.Option[int], 0, len(candidates))
	for i, c := range candidates {
		label := c.AppID
		if c.Label != "" {
			label = fmt.Sprintf("%s (%s)", c.Label, c.AppID)
		}
		options = append(options, huh.NewOption(label, i))
	}

	var selected int
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[int]().
				Title(msg.SelectAccount).
				Options(options...).
				Value(&selected),
		),
	).WithTheme(cmdutil.ThemeFeishu())

	if err := form.Run(); err != nil {
		if err == huh.ErrUserAborted {
			return nil, output.ErrBare(1)
		}
		return nil, err
	}
	return &candidates[selected], nil
}

// tuiConflictPrompt shows existing binding and asks user to Force or Cancel.
func tuiConflictPrompt(opts *BindOptions, source, configPath string) (string, error) {
	msg := getBindMsg(opts.Lang)

	// Build existing binding summary
	existingSummary := fmt.Sprintf(msg.ConflictDesc, source, "?", "?", configPath)
	if data, err := vfs.ReadFile(configPath); err == nil {
		var multi core.MultiAppConfig
		if json.Unmarshal(data, &multi) == nil && len(multi.Apps) > 0 {
			app := multi.Apps[0]
			existingSummary = fmt.Sprintf(msg.ConflictDesc,
				source, app.AppId, app.Brand, configPath)
		}
	}

	var action string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title(msg.ConflictTitle).
				Description(existingSummary),
			huh.NewSelect[string]().
				Options(
					huh.NewOption(msg.ConflictForce, "force"),
					huh.NewOption(msg.ConflictCancel, "cancel"),
				).
				Value(&action),
		),
	).WithTheme(cmdutil.ThemeFeishu())

	if err := form.Run(); err != nil {
		if err == huh.ErrUserAborted {
			return "cancel", nil
		}
		return "", err
	}
	return action, nil
}

// tuiSecurityDisclaimer shows a note about what bind does (security awareness).
func tuiSecurityDisclaimer(opts *BindOptions, source string) error {
	msg := getBindMsg(opts.Lang)
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title(msg.SecurityTitle).
				Description(fmt.Sprintf(msg.SecurityDesc,
					source, core.GetConfigPath(), source)),
		),
	).WithTheme(cmdutil.ThemeFeishu())

	if err := form.Run(); err != nil {
		if err == huh.ErrUserAborted {
			return output.ErrBare(1)
		}
		return err
	}
	return nil
}

// validateBindFlags validates enum flags early, before any side effects.
func validateBindFlags(opts *BindOptions) error {
	if opts.StrictMode != "" {
		switch opts.StrictMode {
		case "bot", "user", "off":
		default:
			return output.ErrValidation("invalid --strict-mode %q; valid values: bot, user, off", opts.StrictMode)
		}
	}
	if opts.DefaultAs != "" {
		switch opts.DefaultAs {
		case "user", "bot", "auto":
		default:
			return output.ErrValidation("invalid --default-as %q; valid values: user, bot, auto", opts.DefaultAs)
		}
	}
	return nil
}

// tuiSelectStrictMode prompts user to choose strict mode policy.
func tuiSelectStrictMode(opts *BindOptions) (string, error) {
	msg := getBindMsg(opts.Lang)
	var value string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(msg.SelectStrictMode).
				Description(msg.SelectStrictModeDesc).
				Options(
					huh.NewOption(msg.StrictModeOff, "off"),
					huh.NewOption(msg.StrictModeBot, "bot"),
					huh.NewOption(msg.StrictModeUser, "user"),
				).
				Value(&value),
		),
	).WithTheme(cmdutil.ThemeFeishu())

	if err := form.Run(); err != nil {
		if err == huh.ErrUserAborted {
			return "", output.ErrBare(1)
		}
		return "", err
	}
	return value, nil
}

// tuiSelectDefaultAs prompts user to choose default identity.
func tuiSelectDefaultAs(opts *BindOptions) (string, error) {
	msg := getBindMsg(opts.Lang)
	var value string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(msg.SelectDefaultAs).
				Description(msg.SelectDefaultAsDesc).
				Options(
					huh.NewOption(msg.DefaultAsAuto, "auto"),
					huh.NewOption(msg.DefaultAsUser, "user"),
					huh.NewOption(msg.DefaultAsBot, "bot"),
				).
				Value(&value),
		),
	).WithTheme(cmdutil.ThemeFeishu())

	if err := form.Run(); err != nil {
		if err == huh.ErrUserAborted {
			return "", output.ErrBare(1)
		}
		return "", err
	}
	return value, nil
}
