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
	"github.com/larksuite/cli/internal/keysigner"
	"github.com/larksuite/cli/internal/output"
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
	PrivateKeyJWT  bool // --private-key-jwt: request private_key_jwt instead of the default client_secret

	Lang         string // raw --lang (string for cobra); normalized to canonical/"" in validateInitLang
	langExplicit bool   // true when --lang was explicitly passed

	UILang i18n.Lang // TUI display language (picker-only); intentionally separate from --lang

	ProfileName string // when set, create/update a named profile instead of replacing Apps[0]

	Restore bool // Restore re-registers the app already in config to recover a lost credential

	// ForceInit overrides the agent-workspace guard. Without it, running
	// init under OPENCLAW_HOME / HERMES_HOME refuses and points the caller
	// at config bind — which is what AI agents almost always want. Manual
	// users with a legitimate need for a separate app can pass --force-init
	// to bypass.
	ForceInit bool
}

// NewCmdConfigInit creates the config init subcommand.
func NewCmdConfigInit(f *cmdutil.Factory, runF func(*ConfigInitOptions) error) *cobra.Command {
	opts := &ConfigInitOptions{Factory: f, UILang: i18n.LangZhCN}

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize configuration (app-id / app-secret-stdin / brand)",
		Long: `Initialize configuration (app-id / app-secret-stdin / brand).

For AI agents: use --new to create a new app. The command blocks until the user
completes setup in the browser. Run it in the background and retrieve the
verification URL from its output.

Inside an Agent context (OPENCLAW_HOME / HERMES_HOME set) this command
refuses by default — use 'lark-cli config bind' to bind to the Agent's
existing app instead of creating a parallel one. Pass --force-init only
if the user explicitly wants a separate app inside the Agent workspace.`,
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
	cmd.Flags().BoolVar(&opts.PrivateKeyJWT, "private-key-jwt", false, "create a new app with private_key_jwt (signed by a platform key, no app secret)")
	cmd.Flags().StringVar(&opts.AppID, "app-id", "", "App ID (non-interactive)")
	cmd.Flags().BoolVar(&opts.AppSecretStdin, "app-secret-stdin", false, "Read App Secret from stdin to avoid process list exposure")
	cmd.Flags().StringVar(&opts.Brand, "brand", "feishu", "feishu or lark (non-interactive, default feishu)")
	cmd.Flags().StringVar(&opts.Lang, "lang", "", "language preference (e.g. zh or zh_cn)")
	cmd.Flags().StringVar(&opts.ProfileName, "name", "", "create or update a named profile (append instead of replace)")
	cmd.Flags().BoolVar(&opts.Restore, "restore", false, "re-register the app already in config to recover a lost credential (keychain key / app secret); reuses the stored app ID and auth method")
	cmd.Flags().BoolVar(&opts.ForceInit, "force-init", false, "allow init inside an Agent workspace (OPENCLAW_HOME / HERMES_HOME); use config bind instead unless you really want a separate app")
	cmdutil.SetRisk(cmd, "write")

	return cmd
}

func requestedInitAuthMethod(opts *ConfigInitOptions) string {
	if opts.PrivateKeyJWT {
		return core.AuthMethodPrivateKeyJWT
	}
	return core.AuthMethodClientSecret
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
	return errs.NewConfigError(errs.SubtypeNotConfigured,
		"config init is refused inside %s context (would create a parallel app and shadow the existing %s binding)", ws.Display(), ws.Display()).
		WithHint("see `lark-cli config bind --help` to bind lark-cli to the Agent's existing app instead. Pass --force-init only if the user explicitly wants a separate app in this workspace.")
}

// hasAnyNonInteractiveFlag returns true if any non-interactive flag is set.
func (o *ConfigInitOptions) hasAnyNonInteractiveFlag() bool {
	return o.New || o.Restore || o.AppID != "" || o.AppSecretStdin
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

// removeStaleSecretForPKJWT clears a secret left in the keychain when the SAME
// appId is migrated from client_secret to private_key_jwt. cleanupOldConfig
// explicitly skips a matching appId, and saveAsProfile only cleans up on an
// appId change, so a same-appId migration would orphan the old secret. This
// fills that gap. RemoveSecretStore only deletes Source=="keychain" entries, so
// the new pkjwt tee key handle is never touched.
func removeStaleSecretForPKJWT(existing *core.MultiAppConfig, profileName, appID string, kc keychain.KeychainAccess) {
	if existing == nil {
		return
	}
	var prior *core.AppConfig
	if profileName != "" {
		if idx := findProfileIndexByName(existing, profileName); idx >= 0 {
			prior = &existing.Apps[idx]
		}
	} else {
		prior = existing.CurrentAppConfig("")
	}
	if prior != nil && prior.AppId == appID && !prior.AppSecret.IsZero() {
		core.RemoveSecretStore(prior.AppSecret, kc)
	}
}

// keyRefFromResult builds the TEE key reference to persist for a private_key_jwt
// registration result, or nil for client_secret.
func keyRefFromResult(r *configInitResult) *core.SecretRef {
	if r != nil && r.AuthMethod == core.AuthMethodPrivateKeyJWT && r.KeyLabel != "" {
		return &core.SecretRef{Source: "tee", ID: r.KeyLabel}
	}
	return nil
}

// saveAsOnlyApp overwrites config.json with a single-app config.
func saveAsOnlyApp(appId string, secret core.SecretInput, brand core.LarkBrand, lang, authMethod string, keyRef *core.SecretRef) error {
	config := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			AppId: appId, AppSecret: secret, Brand: brand, Lang: i18n.Lang(lang), Users: []core.AppUser{},
			AuthMethod: authMethod, KeyRef: keyRef,
		}},
	}
	return saveMultiAppConfigForInit(config)
}

func saveMultiAppConfigForInit(config *core.MultiAppConfig) error {
	return core.SaveMultiAppConfig(config)
}

// saveInitConfig saves a new/updated app config, respecting --profile mode.
// With profileName: appends or updates the named profile (preserves other profiles).
// Without profileName: cleans up old config and saves as the only app.
// authMethod/keyRef carry the credential type: ("", nil) for client_secret,
// (private_key_jwt, &{tee,label}) for the secretless TEE flow.
func saveInitConfig(profileName string, existing *core.MultiAppConfig, f *cmdutil.Factory, appId string, secret core.SecretInput, brand core.LarkBrand, lang, authMethod string, keyRef *core.SecretRef) error {
	if profileName != "" {
		return saveAsProfile(existing, f.Keychain, profileName, appId, secret, brand, lang, authMethod, keyRef)
	}
	cleanupOldConfig(existing, f, appId)
	var prior i18n.Lang
	if existing != nil {
		if app := existing.CurrentAppConfig(""); app != nil {
			prior = app.Lang
		}
	}
	return saveAsOnlyApp(appId, secret, brand, string(preferredLang(i18n.Lang(lang), prior)), authMethod, keyRef)
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
func saveAsProfile(existing *core.MultiAppConfig, kc keychain.KeychainAccess, profileName, appId string, secret core.SecretInput, brand core.LarkBrand, lang, authMethod string, keyRef *core.SecretRef) error {
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
		multi.Apps[idx].AuthMethod = authMethod
		multi.Apps[idx].KeyRef = keyRef
	} else {
		if findAppIndexByAppID(multi, profileName) >= 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"profile name %q conflicts with existing appId", profileName).
				WithParam("--name")
		}
		// Append new profile
		multi.Apps = append(multi.Apps, core.AppConfig{
			Name:       profileName,
			AppId:      appId,
			AppSecret:  secret,
			Brand:      brand,
			Lang:       i18n.Lang(lang),
			Users:      []core.AppUser{},
			AuthMethod: authMethod,
			KeyRef:     keyRef,
		})
	}
	return saveMultiAppConfigForInit(multi)
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
	return saveMultiAppConfigForInit(existing)
}

func persistInitResult(opts *ConfigInitOptions, f *cmdutil.Factory, profileName string, result *configInitResult) error {
	existing, _ := core.LoadMultiAppConfig()

	switch {
	case result.AuthMethod == core.AuthMethodPrivateKeyJWT:
		if err := saveInitConfig(profileName, existing, f, result.AppID, core.SecretInput{}, result.Brand, opts.Lang, result.AuthMethod, keyRefFromResult(result)); err != nil {
			return wrapSaveConfigError(err)
		}
		removeStaleSecretForPKJWT(existing, profileName, result.AppID, f.Keychain)
		return nil
	case result.AppSecret != "":
		secret, err := core.ForStorage(result.AppID, core.PlainSecret(result.AppSecret), f.Keychain)
		if err != nil {
			return errs.NewInternalError(errs.SubtypeSDKError, "%v", err).WithCause(err)
		}
		if err := saveInitConfig(profileName, existing, f, result.AppID, secret, result.Brand, opts.Lang, "", nil); err != nil {
			return wrapSaveConfigError(err)
		}
		return nil
	case result.Mode == "existing" && result.AppID != "":
		return wrapUpdateExistingProfileErr(updateExistingProfileWithoutSecret(existing, profileName, result.AppID, result.Brand, opts.Lang))
	default:
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "App ID and App Secret cannot be empty").WithParam("--app-id")
	}
}

func probeInitResult(opts *ConfigInitOptions, f *cmdutil.Factory, result *configInitResult) error {
	if result.AuthMethod == core.AuthMethodPrivateKeyJWT {
		return runProbePKJWT(opts.Ctx, f, result.Brand, result.AppID, keysigner.Active(), result.KeyLabel)
	}
	if result.AppSecret != "" {
		return runProbe(opts.Ctx, f, result.AppID, result.AppSecret, result.Brand)
	}
	return nil
}

// persistAndProbeResult saves a registration/restore result into profileName and
// runs the post-registration probe. profileName == "" replaces the single app
// (legacy); a named profile is updated in place. Shared by --new and --restore.
func persistAndProbeResult(opts *ConfigInitOptions, f *cmdutil.Factory, profileName string, result *configInitResult) error {
	if err := persistInitResult(opts, f, profileName, result); err != nil {
		return err
	}
	printLangPreferenceConfirmation(opts)
	if result.AuthMethod == core.AuthMethodPrivateKeyJWT {
		output.PrintJson(f.IOStreams.Out, map[string]interface{}{"appId": result.AppID, "authMethod": result.AuthMethod, "brand": result.Brand})
	} else {
		output.PrintJson(f.IOStreams.Out, map[string]interface{}{"appId": result.AppID, "appSecret": "****", "brand": result.Brand})
	}
	return probeInitResult(opts, f, result)
}

// runRestoreFlow re-registers the app already in config to recover a lost
// credential (deleted keychain key / lost app secret). It reads the existing
// app id + auth method + brand from config (no secret needed — that's the lost
// part) and re-runs the device-flow registration with the app id sent on begin,
// so the server re-registers that app instead of creating a new one. The
// re-issued credential is written back to the same profile.
func runRestoreFlow(opts *ConfigInitOptions, existing *core.MultiAppConfig, f *cmdutil.Factory, msg *initMsg) error {
	if existing == nil {
		return errs.NewConfigError(errs.SubtypeNotConfigured, "nothing to restore: no config found").
			WithHint("run: lark-cli config init")
	}
	app := existing.CurrentAppConfig(opts.ProfileName)
	if app == nil || app.AppId == "" {
		return errs.NewConfigError(errs.SubtypeNotConfigured, "nothing to restore: no app id in config%s", profileSuffix(opts.ProfileName)).
			WithHint("run: lark-cli config init")
	}
	if app.KeyRef != nil && strings.TrimSpace(app.KeyRef.Provider) != "" {
		return errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"config init --restore does not manage external signer provider %q", app.KeyRef.Provider).
			WithHint("repair the OpenClaw provider with onboarding doctor --fix, then run config bind again")
	}

	restoreAppID := app.AppId
	// Reuse the stored auth method authoritatively — never prompt. Empty on disk
	// means client_secret (omitempty back-compat); pass it explicitly so restore
	// preserves the existing credential type.
	authMethod := app.AuthMethod
	if authMethod == "" {
		authMethod = core.AuthMethodClientSecret
	}
	result, err := runCreateAppFlow(opts.Ctx, f, app.Brand, authMethod, msg, restoreAppID)
	if err != nil {
		return err
	}
	if result == nil {
		return errs.NewInternalError(errs.SubtypeSDKError, "app restore returned no result")
	}

	// Safety: if the server did not honor app_id (e.g. not yet supported), it may
	// have created a NEW app instead of restoring. Warn so the user is not silently
	// switched to a different app id.
	if result.AppID != restoreAppID {
		fmt.Fprintf(f.IOStreams.ErrOut, "[lark-cli] [WARN] restore: server returned app %s, expected %s — it may have created a new app instead of restoring\n", result.AppID, restoreAppID)
	}

	// Write back to the profile we restored: an explicit --name, else the resolved
	// app's own name. Empty name => legacy single-app replace.
	saveProfile := opts.ProfileName
	if saveProfile == "" {
		saveProfile = app.Name
	}
	return persistAndProbeResult(opts, f, saveProfile, result)
}

// profileSuffix renders " (profile %q)" for error messages, or "" when unnamed.
func profileSuffix(profileName string) string {
	if profileName == "" {
		return ""
	}
	return fmt.Sprintf(" (profile %q)", profileName)
}

func configInitRun(opts *ConfigInitOptions) error {
	f := opts.Factory
	if opts.PrivateKeyJWT {
		switch {
		case opts.Restore:
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--private-key-jwt cannot be combined with --restore; restore preserves the stored auth method").
				WithParam("--private-key-jwt")
		case opts.AppID != "":
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--private-key-jwt cannot be combined with --app-id; use --new to register a private_key_jwt app").
				WithParam("--private-key-jwt")
		case opts.AppSecretStdin:
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--private-key-jwt cannot be combined with --app-secret-stdin; private_key_jwt does not use an app secret").
				WithParam("--private-key-jwt")
		}
	}
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

	// Validate --profile name if set
	if opts.ProfileName != "" {
		if err := core.ValidateProfileName(opts.ProfileName); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%v", err).WithCause(err)
		}
	}

	// --restore recovers an existing app; it is incompatible with creating a new
	// app (--new) or importing one non-interactively (--app-id / stdin secret).
	if opts.Restore {
		if opts.New {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--restore cannot be combined with --new").WithParam("--restore")
		}
		if opts.AppID != "" || opts.AppSecretStdin {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--restore cannot be combined with --app-id / --app-secret-stdin").WithParam("--restore")
		}
	}

	// A user who explicitly asks for private_key_jwt needs immediate feedback
	// before any interactive prompt. Otherwise unsupported machines enter the
	// TUI and fail only after the user chooses a create flow.
	if opts.PrivateKeyJWT && !opts.New && !opts.Restore {
		if _, err := resolveRegisterAuthMethod(opts.Ctx, f, core.AuthMethodPrivateKeyJWT); err != nil {
			return err
		}
	}

	// Mode 1: Non-interactive
	if opts.AppID != "" && opts.appSecret != "" {
		brand := parseBrand(opts.Brand)
		secret, err := core.ForStorage(opts.AppID, core.PlainSecret(opts.appSecret), f.Keychain)
		if err != nil {
			return errs.NewInternalError(errs.SubtypeSDKError, "%v", err).WithCause(err)
		}
		if err := saveInitConfig(opts.ProfileName, existing, f, opts.AppID, secret, brand, opts.Lang, "", nil); err != nil {
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

	// For interactive modes, prompt language selection if --lang was not explicitly set.
	// Picker offers 2 options (中文 / English) and drives BOTH opts.Lang
	// (preference) and opts.UILang (TUI rendering).
	if f.IOStreams.IsTerminal && !opts.langExplicit && !opts.hasAnyNonInteractiveFlag() {
		lang, err := promptLangSelection()
		if err != nil {
			return langSelectionError(err)
		}
		opts.Lang = string(lang)
		opts.UILang = lang
	}

	msg := getInitMsg(opts.UILang)

	// Mode: Restore (--restore) — re-register the app already in config.
	if opts.Restore {
		return runRestoreFlow(opts, existing, f, msg)
	}

	// Mode 3: Create new app directly (--new)
	if opts.New {
		result, err := runCreateAppFlow(opts.Ctx, f, parseBrand(opts.Brand), requestedInitAuthMethod(opts), msg, "")
		if err != nil {
			return err
		}
		if result == nil {
			return errs.NewInternalError(errs.SubtypeSDKError, "app creation returned no result")
		}
		return persistAndProbeResult(opts, f, opts.ProfileName, result)
	}

	// Mode 4: Interactive TUI (terminal)
	if !opts.hasAnyNonInteractiveFlag() && f.IOStreams.IsTerminal {
		result, err := runInteractiveConfigInit(opts.Ctx, f, requestedInitAuthMethod(opts), msg)
		if err != nil {
			return err
		}
		if result == nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "App ID and App Secret cannot be empty").
				WithParam("--app-id")
		}

		if err := persistInitResult(opts, f, opts.ProfileName, result); err != nil {
			return err
		}
		if result.AuthMethod == core.AuthMethodPrivateKeyJWT {
			if err := probeInitResult(opts, f, result); err != nil {
				return err
			}
		}

		if result.Mode == "existing" {
			output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf(msg.ConfigSaved, result.AppID))
		}
		printLangPreferenceConfirmation(opts)
		if result.AuthMethod != core.AuthMethodPrivateKeyJWT {
			return probeInitResult(opts, f, result)
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
	if err := saveInitConfig(opts.ProfileName, existing, f, resolvedAppId, storedSecret, parseBrand(resolvedBrand), opts.Lang, "", nil); err != nil {
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
