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
	Factory *cmdutil.Factory
	Source  string
	AppID   string
	// Identity selects one of two presets — "bot-only" or "user-default" —
	// that expand to underlying StrictMode + DefaultAs in applyPreferences.
	// Empty means "decide later": TUI prompts, flag mode defaults to bot-only
	// (the safer choice — bot acts under its own identity, no impersonation
	// risk; users can still opt into "user-default" via --identity).
	Identity string

	// Force opts in to an otherwise-blocked flag-mode transition — currently
	// only the bot-only → user-default identity escalation. TUI mode ignores
	// this flag because its own prompts already require human confirmation.
	Force bool

	Lang         string
	langExplicit bool // true when --lang was explicitly passed

	// Brand holds the resolved Lark product brand ("feishu" | "lark") for
	// the account being bound. Populated after resolveAccount; TUI stages
	// that run before that (source / account selection) render brand-aware
	// text with an empty value, which brandDisplay falls back to Feishu.
	Brand string

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
		Long: `Bind an AI Agent's (OpenClaw / Hermes / Lark Channel) Feishu credentials to a lark-cli workspace.

--source is auto-detected from env (OPENCLAW_HOME / HERMES_HOME / LARK_CHANNEL); pass it only to override.

For AI agents — DO NOT bind without user confirmation. Binding may
overwrite an existing one and locks in an identity policy. Ask the user:

  --identity bot-only      bot only (safer default; no impersonation;
                           cannot access user resources like personal
                           calendar / mail / drive)
  --identity user-default  user identity allowed (impersonates the user;
                           needed for personal-resource access)

Default to bot-only if the user is unsure. Only run the command after
the user confirms both intent and identity preset.

If lark-cli is already bound and the user only wants to change identity
policy on the SAME app, use 'config strict-mode' — that's the policy
switch and does not require re-bind. Use 'config bind' only when the
underlying app itself changes.

Interactive terminal use: run with no flags to enter the TUI form.`,
		Example: `  # AI flow: confirm intent + identity with user FIRST, then run:
  lark-cli config bind --source openclaw --app-id <id> --identity bot-only
  lark-cli config bind --source hermes --identity user-default
  lark-cli config bind --source lark-channel

  # Interactive (terminal user) — TUI prompts for everything:
  lark-cli config bind`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.langExplicit = cmd.Flags().Changed("lang")
			if runF != nil {
				return runF(opts)
			}
			return configBindRun(opts)
		},
	}

	cmd.Flags().StringVar(&opts.Source, "source", "", "Agent source to bind from (openclaw|hermes|lark-channel); auto-detected from env signals when omitted")
	cmd.Flags().StringVar(&opts.AppID, "app-id", "", "App ID to bind (required for OpenClaw multi-account)")
	cmd.Flags().StringVar(&opts.Identity, "identity", "", "identity preset (bot-only|user-default); defaults to bot-only in flag mode (safer: no impersonation)")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "confirm a risky transition (currently: bot-only → user-default identity change in flag mode)")
	cmd.Flags().StringVar(&opts.Lang, "lang", "zh", "language for interactive prompts (zh|en)")

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

	appConfig, err := resolveAccount(opts, source)
	if err != nil {
		return err
	}
	opts.Brand = string(appConfig.Brand)

	if err := resolveIdentity(opts); err != nil {
		return err
	}
	if err := warnIdentityEscalation(opts, existing.ConfigBytes); err != nil {
		return err
	}
	applyPreferences(appConfig, opts)
	noticeUserDefaultRisk(opts)

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

// finalizeSource returns the validated bind source, reconciling three inputs:
//   - opts.Source: the value of --source (may be empty)
//   - env signals: OPENCLAW_* / HERMES_* detected via DetectWorkspaceFromEnv
//   - TUI mode: can prompt the user if neither flag nor env yields a source
//
// Resolution (in order):
//  1. If --source is a non-empty invalid value → fail with ErrValidation.
//  2. If both --source and an env signal are present and disagree → fail
//     loud; the user almost certainly ran the command in the wrong context.
//  3. TUI mode only: prompt for language first (so later prompts respect it).
//  4. --source wins if set. Otherwise use the env-detected source. Otherwise
//     fall back to a TUI prompt (TUI mode) or an error (flag mode).
func finalizeSource(opts *BindOptions) (string, error) {
	explicit := strings.TrimSpace(strings.ToLower(opts.Source))
	if explicit != "" && explicit != "openclaw" && explicit != "hermes" && explicit != "lark-channel" {
		return "", output.ErrValidation("invalid --source %q; valid values: openclaw, hermes, lark-channel", explicit)
	}

	var detected string
	switch core.DetectWorkspaceFromEnv(os.Getenv) {
	case core.WorkspaceOpenClaw:
		detected = "openclaw"
	case core.WorkspaceHermes:
		detected = "hermes"
	case core.WorkspaceLarkChannel:
		detected = "lark-channel"
	}

	// Explicit and env detection must agree when both are present. Reject
	// before any interactive prompts — running inside Hermes with
	// --source openclaw (or vice versa) is almost always a mistake.
	if explicit != "" && detected != "" && explicit != detected {
		return "", output.ErrWithHint(output.ExitValidation, "bind",
			fmt.Sprintf("--source %q does not match detected Agent environment (%s)", explicit, detected),
			"remove --source to auto-detect, or run this command in the correct Agent context")
	}

	// TUI: prompt for language before any downstream prompts. The source
	// selection itself may still be skipped entirely if --source or the
	// env already pinned it.
	if opts.IsTUI && !opts.langExplicit {
		lang, err := promptLangSelection("")
		if err != nil {
			if err == huh.ErrUserAborted {
				return "", output.ErrBare(1)
			}
			return "", err
		}
		opts.Lang = lang
	}

	if explicit != "" {
		return explicit, nil
	}
	if detected != "" {
		return detected, nil
	}
	if opts.IsTUI {
		return tuiSelectSource(opts)
	}
	return "", output.ErrWithHint(output.ExitValidation, "bind",
		"cannot determine Agent source: no --source flag and no Agent environment detected",
		"pass --source openclaw|hermes|lark-channel, or run this command inside the corresponding Agent context")
}

// reconcileExistingBinding reads any existing config at configPath and decides
// how to proceed. In TUI mode the user is prompted to keep or replace. In flag
// mode the existing binding is silently overwritten — commitBinding will emit a
// notice on success so the caller still sees that a rebind happened.
// See existingBinding for the returned fields.
func reconcileExistingBinding(opts *BindOptions, source, configPath string) (existingBinding, error) {
	oldConfigData, _ := vfs.ReadFile(configPath)
	if oldConfigData == nil {
		return existingBinding{}, nil
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

	return existingBinding{ConfigBytes: oldConfigData}, nil
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
		func(cs []Candidate) (*Candidate, error) { return tuiSelectApp(opts, source, cs) })
	if err != nil {
		return nil, err
	}
	return binder.Build(picked.AppID)
}

// resolveIdentity ensures opts.Identity is set before applyPreferences runs.
// TUI mode prompts when empty; flag mode defaults to "bot-only" — the safer
// preset (bot acts under its own identity, no impersonation). Users who
// want the broader capability set can pass --identity user-default.
func resolveIdentity(opts *BindOptions) error {
	if opts.Identity != "" {
		return nil
	}
	if opts.IsTUI {
		id, err := tuiSelectIdentity(opts)
		if err != nil {
			return err
		}
		opts.Identity = id
		return nil
	}
	opts.Identity = "bot-only"
	return nil
}

// hasStrictBotLock reports whether the given config bytes declare a
// bot-only lock on at least one app. Unparseable input returns false — it
// signals "no enforceable lock to honor", consistent with how the rest of
// the bind flow treats a corrupt previous config (commitBinding will
// overwrite it cleanly).
func hasStrictBotLock(data []byte) bool {
	var multi core.MultiAppConfig
	if err := json.Unmarshal(data, &multi); err != nil {
		return false
	}
	for _, app := range multi.Apps {
		if app.StrictMode != nil && *app.StrictMode == core.StrictModeBot {
			return true
		}
	}
	return false
}

// warnIdentityEscalation surfaces the risk of a flag-mode bot-only →
// user-default identity change. Without --force, the CLI refuses so an AI
// Agent has to relay the warning to the user and get explicit opt-in before
// retrying. TUI mode is exempt: tuiConflictPrompt + tuiSelectIdentity
// already require human confirmation in-flow.
func warnIdentityEscalation(opts *BindOptions, previousConfigBytes []byte) error {
	if opts.IsTUI || opts.Force || previousConfigBytes == nil {
		return nil
	}
	if opts.Identity != "user-default" {
		return nil
	}
	if !hasStrictBotLock(previousConfigBytes) {
		return nil
	}
	msg := getBindMsg(opts.Lang)
	return output.ErrWithHint(output.ExitValidation, "bind",
		msg.IdentityEscalationMessage, msg.IdentityEscalationHint)
}

// noticeUserDefaultRisk surfaces the user-identity impersonation risk on every
// flag-mode bind that lands on user-default. The bot-only → user-default
// escalation is already covered by warnIdentityEscalation (errors out before
// applyPreferences runs), and the TUI flow shows IdentityUserDefaultDesc
// during identity selection — so this fires specifically for the case those
// two miss: a fresh flag-mode bind that goes directly to user-default with
// no previous bot lock to escalate from. Without this, AI agents finish such
// a bind with only a "配置成功" message and never relay to the user that the
// AI can now act under their identity.
func noticeUserDefaultRisk(opts *BindOptions) {
	if opts.IsTUI || opts.Identity != "user-default" {
		return
	}
	msg := getBindMsg(opts.Lang)
	fmt.Fprintln(opts.Factory.IOStreams.ErrOut, "⚠️ "+msg.IdentityEscalationMessage)
}

// applyPreferences expands the chosen identity preset into the underlying
// StrictMode + DefaultAs on the AppConfig. Always writes both fields so the
// profile's intent survives later changes to global strict-mode settings.
func applyPreferences(appConfig *core.AppConfig, opts *BindOptions) {
	switch opts.Identity {
	case "bot-only":
		sm := core.StrictModeBot
		appConfig.StrictMode = &sm
		appConfig.DefaultAs = core.AsBot
	case "user-default":
		sm := core.StrictModeOff
		appConfig.StrictMode = &sm
		appConfig.DefaultAs = core.AsUser
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

	replaced := previousConfigBytes != nil
	msg := getBindMsg(opts.Lang)
	display := sourceDisplayName(source)

	if replaced {
		cleanupKeychainFromData(opts.Factory.Keychain, previousConfigBytes, appConfig)
	}

	fmt.Fprintln(opts.Factory.IOStreams.ErrOut,
		fmt.Sprintf(msg.BindSuccessHeader, display)+"\n"+msg.BindSuccessNotice)

	// TUI mode is a human sitting at a terminal; the BindSuccess notice on
	// stderr is enough and a machine-readable JSON dump on stdout is just
	// noise. Flag mode (Agent orchestration, scripts, piped output) still
	// gets the full envelope for programmatic consumption.
	if opts.IsTUI {
		return nil
	}

	envelope := map[string]interface{}{
		"ok":          true,
		"workspace":   source,
		"app_id":      appConfig.AppId,
		"config_path": configPath,
		"replaced":    replaced,
		"identity":    opts.Identity,
	}
	brand := brandDisplay(string(appConfig.Brand), opts.Lang)
	switch opts.Identity {
	case "bot-only":
		envelope["message"] = fmt.Sprintf(msg.MessageBotOnly, appConfig.AppId, display, brand)
	case "user-default":
		envelope["message"] = fmt.Sprintf(msg.MessageUserDefault, appConfig.AppId, display, display)
	}

	resultJSON, _ := json.Marshal(envelope)
	fmt.Fprintln(opts.Factory.IOStreams.Out, string(resultJSON))
	return nil
}

// cleanupKeychainFromData removes keychain entries referenced by a previous
// config snapshot, skipping any entry whose keychain ID is still in use by
// the new app config. This prevents rebinding the same appId from deleting
// the secret that ForStorage just wrote (old and new secret share the same
// keychain key, derived from appId). Best-effort: errors are silently
// ignored (same contract as config init's cleanup).
func cleanupKeychainFromData(kc keychain.KeychainAccess, data []byte, keep *core.AppConfig) {
	var multi core.MultiAppConfig
	if err := json.Unmarshal(data, &multi); err != nil {
		return
	}
	keepID := ""
	if keep != nil && keep.AppSecret.Ref != nil && keep.AppSecret.Ref.Source == "keychain" {
		keepID = keep.AppSecret.Ref.ID
	}
	for _, app := range multi.Apps {
		if keepID != "" && app.AppSecret.Ref != nil && app.AppSecret.Ref.Source == "keychain" && app.AppSecret.Ref.ID == keepID {
			continue
		}
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
	case core.WorkspaceLarkChannel:
		source = "lark-channel"
	default:
		source = "openclaw" // default first option
	}

	// Resolve actual paths for display
	openclawPath := resolveOpenClawConfigPath()
	hermesEnvPath := resolveHermesEnvPath()
	larkChannelPath := resolveLarkChannelConfigPath()

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(msg.SelectSource).
				Description(fmt.Sprintf(msg.SelectSourceDesc, brandDisplay(opts.Brand, opts.Lang))).
				Options(
					huh.NewOption(fmt.Sprintf(msg.SourceOpenClaw, openclawPath), "openclaw"),
					huh.NewOption(fmt.Sprintf(msg.SourceHermes, hermesEnvPath), "hermes"),
					huh.NewOption(fmt.Sprintf(msg.SourceLarkChannel, larkChannelPath), "lark-channel"),
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
func tuiSelectApp(opts *BindOptions, source string, candidates []Candidate) (*Candidate, error) {
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
				Title(fmt.Sprintf(msg.SelectAccount, sourceDisplayName(source), brandDisplay(opts.Brand, opts.Lang))).
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

// indent prepends two spaces to every line of s. Used to visually nest
// multi-line option descriptions under their label in tuiSelectIdentity.
func indent(s string) string {
	return "  " + strings.ReplaceAll(s, "\n", "\n  ")
}

// validateBindFlags validates enum flags early, before any side effects.
func validateBindFlags(opts *BindOptions) error {
	if opts.Identity != "" {
		switch opts.Identity {
		case "bot-only", "user-default":
		default:
			return output.ErrValidation("invalid --identity %q; valid values: bot-only, user-default", opts.Identity)
		}
	}
	return nil
}

// tuiSelectIdentity prompts user to pick one of two identity presets.
// bot-only is listed first so Enter on the default highlight maps to the
// flag-mode default for consistency across the two modes, and also because
// bot-only is the safer preset (no impersonation risk).
//
// Layout: each option's description is embedded under its label using a
// multi-line option value. huh styles the whole option block (label +
// indented description) as selected / unselected, giving a clear visual
// mapping between picker rows and their explanations — the dynamic
// DescriptionFunc approach breaks here because a longer description on
// hover pushes options out of the field's initial viewport.
func tuiSelectIdentity(opts *BindOptions) (string, error) {
	msg := getBindMsg(opts.Lang)
	brand := brandDisplay(opts.Brand, opts.Lang)
	botLabel := msg.IdentityBotOnly + "\n" + indent(fmt.Sprintf(msg.IdentityBotOnlyDesc, brand))
	userLabel := msg.IdentityUserDefault + "\n" + indent(fmt.Sprintf(msg.IdentityUserDefaultDesc, brand, brand))
	var value string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(msg.SelectIdentity).
				Options(
					huh.NewOption(botLabel, "bot-only"),
					huh.NewOption(userLabel, "user-default"),
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
