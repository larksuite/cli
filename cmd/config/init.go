// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/i18n"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/recovery"
)

// ConfigInitOptions holds all inputs for config init.
type ConfigInitOptions struct {
	Factory        *cmdutil.Factory
	Ctx            context.Context
	AppID          string
	appSecret      string // internal only; populated from stdin, never from a CLI flag
	AppSecretStdin bool   // read app-secret from stdin (avoids process list exposure)
	Brand          string
	New            bool

	Lang         string // raw --lang (string for cobra); normalized to canonical/"" in validateInitLang
	langExplicit bool   // true when --lang was explicitly passed

	// UILang is the language everything this run renders in, and the
	// preference this run persists. --lang wins outright. Otherwise the
	// stored preference seeds the picker and the picker's answer wins — a
	// stored value decides what renders up to that point, it does not stop the
	// question from being asked. Resolved by resolveInitUILang before any
	// output.
	UILang i18n.Lang

	ProfileName string // when set, create/update a named profile instead of replacing Apps[0]

	// ForceInit overrides the agent-workspace guard. Without it, running
	// init under OPENCLAW_HOME / HERMES_HOME refuses so the distribution's
	// supported Agent-app setup flow remains the default. Manual users with
	// a legitimate need for a separate app can pass --force-init to bypass.
	ForceInit bool
}

const (
	configInitLongPrefix = `Initialize configuration (app-id / app-secret-stdin / brand).

For AI agents: use --new to create a new app. The command blocks until the user
completes setup in the browser. Run it in the background and retrieve the
verification URL from its output.

Inside an Agent context (OPENCLAW_HOME / HERMES_HOME set) this command`

	configInitBindGuidance = `
refuses by default — use 'lark-cli config bind' to bind to the Agent's
existing app instead of creating a parallel one.`

	configInitBindFallback = `
refuses by default to avoid creating a parallel app alongside Agent-managed
credentials. Reuse the Agent's existing app through this distribution's
supported setup flow.`

	configInitBindSuffix = ` Pass --force-init only
if the user explicitly wants a separate app inside the Agent workspace.`

	configInitFallbackSuffix = ` Pass --force-init only if the user explicitly wants a
separate app inside the Agent workspace.`

	configInitLongWithBind    = configInitLongPrefix + configInitBindGuidance + configInitBindSuffix
	configInitLongWithoutBind = configInitLongPrefix + configInitBindFallback + configInitFallbackSuffix

	forceInitUsageWithBind = "allow init inside an Agent workspace (OPENCLAW_HOME / HERMES_HOME); use config bind instead unless you really want a separate app"

	forceInitUsageWithoutBind = "allow init inside an Agent workspace (OPENCLAW_HOME / HERMES_HOME) only when the user explicitly wants a separate app"
)

// NewCmdConfigInit creates the config init subcommand.
func NewCmdConfigInit(f *cmdutil.Factory, runF func(*ConfigInitOptions) error) *cobra.Command {
	opts := &ConfigInitOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize configuration (app-id / app-secret-stdin / brand)",
		Long:  configInitLongWithBind,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Ctx = cmd.Context()
			opts.langExplicit = cmd.Flags().Changed("lang")
			if err := validateInitLang(opts); err != nil {
				return err
			}
			if err := guardAgentWorkspace(opts); err != nil {
				return err
			}
			if runF != nil {
				return runF(opts)
			}
			return configInitRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.New, "new", false, "create a new app directly (skip mode selection)")
	cmd.Flags().StringVar(&opts.AppID, "app-id", "", "App ID (non-interactive)")
	cmd.Flags().BoolVar(&opts.AppSecretStdin, "app-secret-stdin", false, "Read App Secret from stdin to avoid process list exposure")
	cmd.Flags().StringVar(&opts.Brand, "brand", "feishu", "feishu or lark (non-interactive, default feishu)")
	cmd.Flags().StringVar(&opts.Lang, "lang", "", "language preference; also selects the interactive display language (e.g. zh or zh_cn)")
	cmd.Flags().StringVar(&opts.ProfileName, "name", "", "create or update a named profile (append instead of replace); a new profile inherits no language preference from existing profiles, so pass --lang to set one")
	cmd.Flags().BoolVar(&opts.ForceInit, "force-init", false, forceInitUsageWithBind)
	cmdutil.SetRisk(cmd, "write")

	return cmd
}

// ProjectInitHelp keeps the default command-specific guidance intact and
// replaces it only when this build conceals config bind. The config package
// owns both variants; the root presentation pass supplies the build-local
// availability decision after plugin policy has finalized the command tree.
func ProjectInitHelp(cmd *cobra.Command, canReferenceBind bool) {
	if cmd == nil {
		return
	}
	long, forceInitUsage := configInitLongWithBind, forceInitUsageWithBind
	if !canReferenceBind {
		long, forceInitUsage = configInitLongWithoutBind, forceInitUsageWithoutBind
	}
	cmd.Long = long
	if flag := cmd.Flags().Lookup("force-init"); flag != nil {
		flag.Usage = forceInitUsage
	}
}

// printLangPreferenceConfirmation echoes the set preference to stderr, only
// when --lang explicitly set a non-empty value.
func printLangPreferenceConfirmation(opts *ConfigInitOptions) {
	if !opts.langExplicit || opts.Lang == "" {
		return
	}
	msg := getInitMsg(opts.UILang)
	fmt.Fprintln(opts.Factory.IOStreams.ErrOut, fmt.Sprintf(msg.LangPreferenceSet, opts.Lang))
}

func validateInitLang(opts *ConfigInitOptions) error {
	lang, err := cmdutil.ParseLangFlag(opts.Lang)
	if err != nil {
		return err
	}
	opts.Lang = string(lang)
	return nil
}

// guardAgentWorkspace refuses 'config init' when run inside an OpenClaw or
// Hermes Agent context, because the Agent has already provisioned an app
// and 'config bind' is the right tool for hooking lark-cli into it.
// Running init here would create a parallel app under the agent's workspace
// dir, breaking the binding the user actually wants. --force-init lets a
// human user override when they really do want a separate app.
func guardAgentWorkspace(opts *ConfigInitOptions) error {
	if opts.ForceInit {
		return nil
	}
	ws := core.DetectWorkspaceFromEnv(os.Getenv)
	if ws.IsLocal() {
		return nil
	}
	return recovery.Attach(
		errs.NewConfigError(errs.SubtypeNotConfigured,
			"config init is refused inside %s context (would create a parallel app and shadow the existing %s binding)", ws.Display(), ws.Display()),
		recovery.Join(" ",
			recovery.Command(recovery.TargetConfigBind, "see `lark-cli config bind --help` to bind lark-cli to the Agent's existing app instead."),
			recovery.Text("Pass --force-init only if the user explicitly wants a separate app in this workspace."),
		),
	)
}

// hasAnyNonInteractiveFlag returns true if any non-interactive flag is set.
func (o *ConfigInitOptions) hasAnyNonInteractiveFlag() bool {
	return o.New || o.AppID != "" || o.AppSecretStdin
}

// priorInitLang returns the language preference already stored for the profile
// this run will write to, mirroring how saveInitConfig picks the prior value.
func priorInitLang(existing *core.MultiAppConfig, profileName string) i18n.Lang {
	if existing == nil {
		return ""
	}
	if profileName != "" {
		if idx := findProfileIndexByName(existing, profileName); idx >= 0 {
			return existing.Apps[idx].Lang
		}
		return ""
	}
	if app := existing.CurrentAppConfig(""); app != nil {
		return app.Lang
	}
	return ""
}

// resolveInitUILang fixes the display language before anything is rendered:
// it is the preference this run will persist — --lang when given, otherwise
// whatever the target profile already stored.
func resolveInitUILang(opts *ConfigInitOptions, existing *core.MultiAppConfig) {
	opts.UILang = preferredLang(i18n.Lang(opts.Lang), priorInitLang(existing, opts.ProfileName))
}

// shouldPromptInitLang reports whether the language picker should run: only in
// a terminal, only when --lang did not already answer it (including an explicit
// empty --lang, which says "do not ask"), only when no non-interactive flag
// pinned the flow, and only when the picker can express whatever preference is
// already in effect. A stored zh_cn/en_us is not a reason to stop asking — it
// becomes the pre-selected option, so re-running init stays the way to change
// the language.
func shouldPromptInitLang(opts *ConfigInitOptions, isTerminal bool) bool {
	return isTerminal && !opts.langExplicit && !opts.hasAnyNonInteractiveFlag() && pickerCanExpress(opts.UILang)
}

// cleanupOldConfig clears keychain entries (AppSecret + UAT) for all apps in existing config except the app whose AppId equals skipAppID.
func cleanupOldConfig(existing *core.MultiAppConfig, f *cmdutil.Factory, skipAppID string) {
	if existing == nil {
		return
	}
	for _, app := range existing.Apps {
		if app.AppId == skipAppID {
			continue
		}
		core.RemoveSecretStore(app.AppSecret, f.Keychain)
		for _, user := range app.Users {
			auth.RemoveStoredToken(app.AppId, user.UserOpenId)
		}
	}
}

// saveAsOnlyApp overwrites config.json with a single-app config.
func saveAsOnlyApp(appId string, secret core.SecretInput, brand core.LarkBrand, lang string) error {
	config := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			AppId: appId, AppSecret: secret, Brand: brand, Lang: i18n.Lang(lang), Users: []core.AppUser{},
		}},
	}
	return core.SaveMultiAppConfig(config)
}

// saveInitConfig saves a new/updated app config, respecting --profile mode.
// With profileName: appends or updates the named profile (preserves other profiles).
// Without profileName: cleans up old config and saves as the only app.
func saveInitConfig(profileName string, existing *core.MultiAppConfig, f *cmdutil.Factory, appId string, secret core.SecretInput, brand core.LarkBrand, lang string) error {
	if profileName != "" {
		return saveAsProfile(existing, f.Keychain, profileName, appId, secret, brand, lang)
	}
	cleanupOldConfig(existing, f, appId)
	var prior i18n.Lang
	if existing != nil {
		if app := existing.CurrentAppConfig(""); app != nil {
			prior = app.Lang
		}
	}
	return saveAsOnlyApp(appId, secret, brand, string(preferredLang(i18n.Lang(lang), prior)))
}

// wrapSaveConfigError passes an already-typed error (e.g. the --name conflict
// validation error from saveAsProfile) through unchanged, and classifies any
// other failure as an internal storage error. Without the passthrough a user
// input error would surface to agents as a system storage failure.
func wrapSaveConfigError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errs.ProblemOf(err); ok {
		return err
	}
	return errs.NewInternalError(errs.SubtypeStorage, "failed to save config: %v", err).WithCause(err)
}

// saveAsProfile appends or updates a named profile in the config.
// If a profile with the same name exists, it updates it; otherwise appends.
// When updating, cleans up old keychain secrets if AppId changed.
func saveAsProfile(existing *core.MultiAppConfig, kc keychain.KeychainAccess, profileName, appId string, secret core.SecretInput, brand core.LarkBrand, lang string) error {
	multi := existing
	if multi == nil {
		multi = &core.MultiAppConfig{}
	}

	if idx := findProfileIndexByName(multi, profileName); idx >= 0 {
		// Clean up old keychain secret and user tokens if AppId changed
		if multi.Apps[idx].AppId != appId {
			core.RemoveSecretStore(multi.Apps[idx].AppSecret, kc)
			for _, user := range multi.Apps[idx].Users {
				auth.RemoveStoredToken(multi.Apps[idx].AppId, user.UserOpenId)
			}
			multi.Apps[idx].Users = []core.AppUser{}
		}
		multi.Apps[idx].AppId = appId
		multi.Apps[idx].AppSecret = secret
		multi.Apps[idx].Brand = brand
		multi.Apps[idx].Lang = preferredLang(i18n.Lang(lang), multi.Apps[idx].Lang)
	} else {
		if findAppIndexByAppID(multi, profileName) >= 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"profile name %q conflicts with existing appId", profileName).
				WithParam("--name")
		}
		// Append new profile
		multi.Apps = append(multi.Apps, core.AppConfig{
			Name:      profileName,
			AppId:     appId,
			AppSecret: secret,
			Brand:     brand,
			Lang:      i18n.Lang(lang),
			Users:     []core.AppUser{},
		})
	}
	return core.SaveMultiAppConfig(multi)
}

func findProfileIndexByName(multi *core.MultiAppConfig, profileName string) int {
	if multi == nil {
		return -1
	}
	for i := range multi.Apps {
		if multi.Apps[i].Name == profileName {
			return i
		}
	}
	return -1
}

func findAppIndexByAppID(multi *core.MultiAppConfig, appID string) int {
	if multi == nil {
		return -1
	}
	for i := range multi.Apps {
		if multi.Apps[i].AppId == appID {
			return i
		}
	}
	return -1
}

// wrapUpdateExistingProfileErr classifies the error returned by
// updateExistingProfileWithoutSecret. Typed errors (e.g. *errs.ValidationError
// for blank-input) pass through unchanged so their exit code semantics
// survive; everything else (filesystem, keychain, etc.) is wrapped as
// InternalError.
func wrapUpdateExistingProfileErr(err error) error {
	if err == nil {
		return nil
	}
	if errs.IsTyped(err) {
		return err
	}
	return errs.NewInternalError(errs.SubtypeSDKError, "failed to save config: %v", err).WithCause(err)
}

func updateExistingProfileWithoutSecret(existing *core.MultiAppConfig, profileName, appID string, brand core.LarkBrand, lang string) error {
	if existing == nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "App Secret cannot be empty for new configuration").
			WithParam("--app-secret")
	}

	var app *core.AppConfig
	if profileName != "" {
		if idx := findProfileIndexByName(existing, profileName); idx >= 0 {
			app = &existing.Apps[idx]
		} else {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "App Secret cannot be empty for new profile").
				WithParam("--app-secret")
		}
	} else {
		app = existing.CurrentAppConfig("")
		if app == nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "App Secret cannot be empty for new configuration").
				WithParam("--app-secret")
		}
	}

	if app.AppId != appID {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "App Secret cannot be empty when changing App ID").
			WithParam("--app-secret")
	}

	app.AppId = appID
	app.Brand = brand
	app.Lang = preferredLang(i18n.Lang(lang), app.Lang)
	return core.SaveMultiAppConfig(existing)
}

func configInitRun(opts *ConfigInitOptions) error {
	f := opts.Factory

	// Read secret from stdin if --app-secret-stdin is set
	if opts.AppSecretStdin {
		scanner := bufio.NewScanner(f.IOStreams.In)
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "failed to read secret from stdin: %v", err).WithCause(err)
			}
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "stdin is empty, expected app secret")
		}
		opts.appSecret = strings.TrimSpace(scanner.Text())
		if opts.appSecret == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "app secret read from stdin is empty")
		}
	}

	existing, err := core.LoadMultiAppConfig()
	if err != nil {
		existing = nil // treat as empty
	}
	resolveInitUILang(opts, existing)

	// Validate --profile name if set
	if opts.ProfileName != "" {
		if err := core.ValidateProfileName(opts.ProfileName); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%v", err).WithCause(err)
		}
	}

	// Mode 1: Non-interactive
	if opts.AppID != "" && opts.appSecret != "" {
		brand := parseBrand(opts.Brand)
		secret, err := core.ForStorage(opts.AppID, core.PlainSecret(opts.appSecret), f.Keychain)
		if err != nil {
			return errs.NewInternalError(errs.SubtypeSDKError, "%v", err).WithCause(err)
		}
		if err := saveInitConfig(opts.ProfileName, existing, f, opts.AppID, secret, brand, opts.Lang); err != nil {
			return wrapSaveConfigError(err)
		}
		output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("Configuration saved to %s", core.GetConfigPath()))
		printLangPreferenceConfirmation(opts)
		output.PrintJson(f.IOStreams.Out, map[string]interface{}{"appId": opts.AppID, "appSecret": "****", "brand": brand})
		if err := runProbe(opts.Ctx, f, opts.AppID, opts.appSecret, brand); err != nil {
			return err
		}
		return nil
	}

	// Interactive runs still ask, seeded with whatever is already in effect,
	// so re-running init remains the way to change the language. The picker
	// offers 2 options (中文 / English) and drives BOTH opts.Lang (the
	// preference that gets persisted) and opts.UILang (what renders now).
	if shouldPromptInitLang(opts, f.IOStreams.IsTerminal) {
		lang, err := promptLangSelection(opts.UILang)
		if err != nil {
			return langSelectionError(err)
		}
		opts.Lang = string(lang)
		opts.UILang = lang
	}

	msg := getInitMsg(opts.UILang)

	// Mode 3: Create new app directly (--new)
	if opts.New {
		result, err := runCreateAppFlow(opts.Ctx, f, parseBrand(opts.Brand), msg)
		if err != nil {
			return err
		}
		if result == nil {
			return errs.NewInternalError(errs.SubtypeSDKError, "app creation returned no result")
		}
		existing, _ := core.LoadMultiAppConfig()
		secret, err := core.ForStorage(result.AppID, core.PlainSecret(result.AppSecret), f.Keychain)
		if err != nil {
			return errs.NewInternalError(errs.SubtypeSDKError, "%v", err).WithCause(err)
		}
		if err := saveInitConfig(opts.ProfileName, existing, f, result.AppID, secret, result.Brand, opts.Lang); err != nil {
			return wrapSaveConfigError(err)
		}
		printLangPreferenceConfirmation(opts)
		output.PrintJson(f.IOStreams.Out, map[string]interface{}{"appId": result.AppID, "appSecret": "****", "brand": result.Brand})
		if err := runProbe(opts.Ctx, f, result.AppID, result.AppSecret, result.Brand); err != nil {
			return err
		}
		return nil
	}

	// Mode 4: Interactive TUI (terminal)
	if !opts.hasAnyNonInteractiveFlag() && f.IOStreams.IsTerminal {
		result, err := runInteractiveConfigInit(opts.Ctx, f, msg)
		if err != nil {
			return err
		}
		if result == nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "App ID and App Secret cannot be empty").
				WithParam("--app-id")
		}

		existing, _ := core.LoadMultiAppConfig()

		if result.AppSecret != "" {
			// New secret provided (either from "create" or "existing" with input)
			secret, err := core.ForStorage(result.AppID, core.PlainSecret(result.AppSecret), f.Keychain)
			if err != nil {
				return errs.NewInternalError(errs.SubtypeSDKError, "%v", err).WithCause(err)
			}
			if err := saveInitConfig(opts.ProfileName, existing, f, result.AppID, secret, result.Brand, opts.Lang); err != nil {
				return wrapSaveConfigError(err)
			}
		} else if result.Mode == "existing" && result.AppID != "" {
			// Existing app with unchanged secret — update app ID and brand only
			if err := wrapUpdateExistingProfileErr(updateExistingProfileWithoutSecret(existing, opts.ProfileName, result.AppID, result.Brand, opts.Lang)); err != nil {
				return err
			}
		} else {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "App ID and App Secret cannot be empty").
				WithParam("--app-id")
		}

		if result.Mode == "existing" {
			output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf(msg.ConfigSaved, result.AppID))
		}
		printLangPreferenceConfirmation(opts)
		if result.AppSecret != "" {
			if err := runProbe(opts.Ctx, f, result.AppID, result.AppSecret, result.Brand); err != nil {
				return err
			}
		}
		return nil
	}

	// Non-terminal: cannot run interactive mode, guide user to --new
	if !f.IOStreams.IsTerminal {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "config init requires a terminal for interactive mode. Run with --new to create a new app:\n  lark-cli config init --new\nThis command blocks until setup is complete and outputs a verification URL. Run it in the background, then retrieve the URL from its output.")
	}

	// Mode 5: Legacy interactive (readline fallback)
	firstApp := (*core.AppConfig)(nil)
	if existing != nil {
		firstApp = existing.CurrentAppConfig("")
	}

	reader := bufio.NewReader(f.IOStreams.In)
	readLine := func(prompt string) (string, error) {
		fmt.Fprintf(f.IOStreams.ErrOut, "%s: ", prompt)
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", fmt.Errorf("failed to read input: %w", err)
		}
		if err == io.EOF && strings.TrimSpace(line) == "" {
			return "", fmt.Errorf("input terminated unexpectedly (EOF)")
		}
		return strings.TrimSpace(line), nil
	}

	prompt := "App ID"
	if firstApp != nil && firstApp.AppId != "" {
		prompt += fmt.Sprintf(" [%s]", firstApp.AppId)
	}
	appIdInput, err := readLine(prompt)
	if err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithCause(err)
	}

	prompt = "App Secret"
	if firstApp != nil && !firstApp.AppSecret.IsZero() {
		prompt += " [****]"
	}
	appSecretInput, err := readLine(prompt)
	if err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithCause(err)
	}

	prompt = "Brand (lark/feishu)"
	if firstApp != nil && firstApp.Brand != "" {
		prompt += fmt.Sprintf(" [%s]", firstApp.Brand)
	} else {
		prompt += " [feishu]"
	}
	brandInput, err := readLine(prompt)
	if err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithCause(err)
	}

	resolvedAppId := appIdInput
	if resolvedAppId == "" && firstApp != nil {
		resolvedAppId = firstApp.AppId
	}
	var resolvedSecret core.SecretInput
	if appSecretInput != "" {
		resolvedSecret = core.PlainSecret(appSecretInput)
	} else if firstApp != nil {
		resolvedSecret = firstApp.AppSecret
	}
	resolvedBrand := brandInput
	if resolvedBrand == "" && firstApp != nil {
		resolvedBrand = string(firstApp.Brand)
	}
	if resolvedBrand == "" {
		resolvedBrand = "feishu"
	}

	if resolvedAppId == "" || resolvedSecret.IsZero() {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "App ID and App Secret cannot be empty").
			WithParam("--app-id")
	}

	storedSecret, err := core.ForStorage(resolvedAppId, resolvedSecret, f.Keychain)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeSDKError, "%v", err).WithCause(err)
	}
	if err := saveInitConfig(opts.ProfileName, existing, f, resolvedAppId, storedSecret, parseBrand(resolvedBrand), opts.Lang); err != nil {
		return wrapSaveConfigError(err)
	}
	output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("Configuration saved to %s", core.GetConfigPath()))
	printLangPreferenceConfirmation(opts)
	if appSecretInput != "" {
		if err := runProbe(opts.Ctx, f, resolvedAppId, appSecretInput, parseBrand(resolvedBrand)); err != nil {
			return err
		}
	}
	return nil
}
