// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keychain"
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
	Lang           string
	langExplicit   bool   // true when --lang was explicitly passed
	ProfileName    string // when set, create/update a named profile instead of replacing Apps[0]
}

// NewCmdConfigInit creates the config init subcommand.
func NewCmdConfigInit(f *cmdutil.Factory, runF func(*ConfigInitOptions) error) *cobra.Command {
	opts := &ConfigInitOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize configuration (app-id / app-secret-stdin / brand)",
		Long: `Initialize configuration (app-id / app-secret-stdin / brand).

For AI agents: use --new to create a new app. The command blocks until the user
completes setup in the browser. Run it in the background and retrieve the
verification URL from its output.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Ctx = cmd.Context()
			opts.langExplicit = cmd.Flags().Changed("lang")
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
	cmd.Flags().StringVar(&opts.Lang, "lang", "zh", "language for interactive prompts (zh or en)")
	cmd.Flags().StringVar(&opts.ProfileName, "name", "", "create or update a named profile (append instead of replace)")

	return cmd
}

// hasAnyNonInteractiveFlag returns true if any non-interactive flag is set.
func (o *ConfigInitOptions) hasAnyNonInteractiveFlag() bool {
	return o.New || o.AppID != "" || o.AppSecretStdin
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
			AppId: appId, AppSecret: secret, Brand: brand, Lang: lang, Users: []core.AppUser{},
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
	return saveAsOnlyApp(appId, secret, brand, lang)
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
		// Update existing profile
		multi.Apps[idx].AppId = appId
		multi.Apps[idx].AppSecret = secret
		multi.Apps[idx].Brand = brand
		multi.Apps[idx].Lang = lang
	} else {
		if findAppIndexByAppID(multi, profileName) >= 0 {
			return fmt.Errorf("profile name %q conflicts with existing appId", profileName)
		}
		// Append new profile
		multi.Apps = append(multi.Apps, core.AppConfig{
			Name:      profileName,
			AppId:     appId,
			AppSecret: secret,
			Brand:     brand,
			Lang:      lang,
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

func updateExistingProfileWithoutSecret(existing *core.MultiAppConfig, profileName, appID string, brand core.LarkBrand, lang string) error {
	if existing == nil {
		return output.ErrValidation("App Secret cannot be empty for new configuration")
	}

	var app *core.AppConfig
	if profileName != "" {
		if idx := findProfileIndexByName(existing, profileName); idx >= 0 {
			app = &existing.Apps[idx]
		} else {
			return output.ErrValidation("App Secret cannot be empty for new profile")
		}
	} else {
		app = existing.CurrentAppConfig("")
		if app == nil {
			return output.ErrValidation("App Secret cannot be empty for new configuration")
		}
	}

	if app.AppId != appID {
		return output.ErrValidation("App Secret cannot be empty when changing App ID")
	}

	app.AppId = appID
	app.Brand = brand
	app.Lang = lang
	return core.SaveMultiAppConfig(existing)
}

func configInitRun(opts *ConfigInitOptions) error {
	f := opts.Factory

	// Read secret from stdin if --app-secret-stdin is set
	if opts.AppSecretStdin {
		scanner := bufio.NewScanner(f.IOStreams.In)
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return output.ErrValidation("failed to read secret from stdin: %v", err)
			}
			return output.ErrValidation("stdin is empty, expected app secret")
		}
		opts.appSecret = strings.TrimSpace(scanner.Text())
		if opts.appSecret == "" {
			return output.ErrValidation("app secret read from stdin is empty")
		}
	}

	existing, err := core.LoadMultiAppConfig()
	if err != nil {
		existing = nil // treat as empty
	}

	// Validate --profile name if set
	if opts.ProfileName != "" {
		if err := core.ValidateProfileName(opts.ProfileName); err != nil {
			return output.ErrValidation("%v", err)
		}
	}

	// Mode 1: Non-interactive
	if opts.AppID != "" && opts.appSecret != "" {
		brand := parseBrand(opts.Brand)
		secret, err := core.ForStorage(opts.AppID, core.PlainSecret(opts.appSecret), f.Keychain)
		if err != nil {
			return output.Errorf(output.ExitInternal, "internal", "%v", err)
		}
		if err := saveInitConfig(opts.ProfileName, existing, f, opts.AppID, secret, brand, opts.Lang); err != nil {
			return output.Errorf(output.ExitInternal, "internal", "failed to save config: %v", err)
		}
		output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("Configuration saved to %s", core.GetConfigPath()))
		output.PrintJson(f.IOStreams.Out, map[string]interface{}{"appId": opts.AppID, "appSecret": "****", "brand": brand})
		return nil
	}

	// For interactive modes, prompt language selection if --lang was not explicitly set
	if f.IOStreams.IsTerminal && !opts.langExplicit && !opts.hasAnyNonInteractiveFlag() {
		savedLang := ""
		if existing != nil {
			if app := existing.CurrentAppConfig(""); app != nil {
				savedLang = app.Lang
			}
		}
		lang, err := promptLangSelection(savedLang)
		if err != nil {
			if err == huh.ErrUserAborted {
				return output.ErrBare(1)
			}
			return err
		}
		opts.Lang = lang
	}

	msg := getInitMsg(opts.Lang)

	// Mode 3: Create new app directly (--new)
	if opts.New {
		result, err := runCreateAppFlow(opts.Ctx, f, core.BrandFeishu, msg)
		if err != nil {
			return err
		}
		if result == nil {
			return output.ErrValidation("app creation returned no result")
		}
		existing, _ := core.LoadMultiAppConfig()
		secret, err := core.ForStorage(result.AppID, core.PlainSecret(result.AppSecret), f.Keychain)
		if err != nil {
			return output.Errorf(output.ExitInternal, "internal", "%v", err)
		}
		if err := saveInitConfig(opts.ProfileName, existing, f, result.AppID, secret, result.Brand, opts.Lang); err != nil {
			return output.Errorf(output.ExitInternal, "internal", "failed to save config: %v", err)
		}
		output.PrintJson(f.IOStreams.Out, map[string]interface{}{"appId": result.AppID, "appSecret": "****", "brand": result.Brand})
		return nil
	}

	// Mode 4: Interactive TUI (terminal)
	if !opts.hasAnyNonInteractiveFlag() && f.IOStreams.IsTerminal {
		result, err := runInteractiveConfigInit(opts.Ctx, f, msg)
		if err != nil {
			return err
		}
		if result == nil {
			return output.ErrValidation("App ID and App Secret cannot be empty")
		}

		existing, _ := core.LoadMultiAppConfig()

		if result.AppSecret != "" {
			// New secret provided (either from "create" or "existing" with input)
			secret, err := core.ForStorage(result.AppID, core.PlainSecret(result.AppSecret), f.Keychain)
			if err != nil {
				return output.Errorf(output.ExitInternal, "internal", "%v", err)
			}
			if err := saveInitConfig(opts.ProfileName, existing, f, result.AppID, secret, result.Brand, opts.Lang); err != nil {
				return output.Errorf(output.ExitInternal, "internal", "failed to save config: %v", err)
			}
		} else if result.Mode == "existing" && result.AppID != "" {
			// Existing app with unchanged secret — update app ID and brand only
			if err := updateExistingProfileWithoutSecret(existing, opts.ProfileName, result.AppID, result.Brand, opts.Lang); err != nil {
				var exitErr *output.ExitError
				if errors.As(err, &exitErr) {
					return err
				}
				return output.Errorf(output.ExitInternal, "internal", "failed to save config: %v", err)
			}
		} else {
			return output.ErrValidation("App ID and App Secret cannot be empty")
		}

		if result.Mode == "existing" {
			output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf(msg.ConfigSaved, result.AppID))
		}
		return nil
	}

	// Non-terminal: cannot run interactive mode, guide user to --new
	if !f.IOStreams.IsTerminal {
		return output.ErrValidation("config init requires a terminal for interactive mode. Run with --new to create a new app:\n  lark-cli config init --new\nThis command blocks until setup is complete and outputs a verification URL. Run it in the background, then retrieve the URL from its output.")
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
		return output.ErrValidation("%s", err)
	}

	prompt = "App Secret"
	if firstApp != nil && !firstApp.AppSecret.IsZero() {
		prompt += " [****]"
	}
	appSecretInput, err := readLine(prompt)
	if err != nil {
		return output.ErrValidation("%s", err)
	}

	prompt = "Brand (lark/feishu)"
	if firstApp != nil && firstApp.Brand != "" {
		prompt += fmt.Sprintf(" [%s]", firstApp.Brand)
	} else {
		prompt += " [feishu]"
	}
	brandInput, err := readLine(prompt)
	if err != nil {
		return output.ErrValidation("%s", err)
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
		return output.ErrValidation("App ID and App Secret cannot be empty")
	}

	storedSecret, err := core.ForStorage(resolvedAppId, resolvedSecret, f.Keychain)
	if err != nil {
		return output.Errorf(output.ExitInternal, "internal", "%v", err)
	}
	if err := saveInitConfig(opts.ProfileName, existing, f, resolvedAppId, storedSecret, parseBrand(resolvedBrand), opts.Lang); err != nil {
		return output.Errorf(output.ExitInternal, "internal", "failed to save config: %v", err)
	}
	output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("Configuration saved to %s", core.GetConfigPath()))
	return nil
}
