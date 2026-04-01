// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
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
	langExplicit   bool // true when --lang was explicitly passed
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

// warnIfEncryptedSecretFallback explains when app-secret persistence downgraded to the encrypted file fallback.
func warnIfEncryptedSecretFallback(w io.Writer, secret core.SecretInput, original core.SecretInput) {
	if !original.IsPlain() || !secret.IsSecretRef() || secret.Ref.Source != "encrypted_file" {
		return
	}
	fmt.Fprintln(w, "warning: keychain unavailable, app secret stored in a local file protected by filesystem permissions (0600) managed by lark-cli")
}

// validateSecretReuse rejects managed secret references that no longer match the selected app ID.
func validateSecretReuse(appID string, secret core.SecretInput) error {
	if !secret.IsSecretRef() {
		return nil
	}
	if secret.Ref.Source == "file" {
		return nil
	}
	expectedID := "appsecret:" + appID
	if secret.Ref.ID == expectedID {
		return nil
	}
	return output.ErrValidation("App Secret must be re-entered when App ID changes")
}

// cleanupReplacedCurrentAppSecret removes the previous current-app secret when the backend changes in place.
func cleanupReplacedCurrentAppSecret(existing *core.MultiAppConfig, f *cmdutil.Factory, appID string, newSecret core.SecretInput) {
	if existing == nil || len(existing.Apps) == 0 {
		return
	}
	current := existing.Apps[0]
	if current.AppId != appID || !current.AppSecret.IsSecretRef() || !newSecret.IsSecretRef() {
		return
	}
	if current.AppSecret.Ref.Source == newSecret.Ref.Source && current.AppSecret.Ref.ID == newSecret.Ref.ID {
		return
	}
	core.RemoveSecretStore(current.AppSecret, f.Keychain)
}

// storeAndSaveOnlyApp persists a single-app config while keeping secret storage, rollback, and cleanup in sync.
func storeAndSaveOnlyApp(existing *core.MultiAppConfig, f *cmdutil.Factory, appID string, plainSecret core.SecretInput, brand core.LarkBrand, lang string) error {
	// Keep the secret persistence pipeline centralized here.
	// New config-init branches should call this helper instead of reimplementing
	// store -> save -> cleanup -> fallback-warning sequencing inline.
	if err := validateSecretReuse(appID, plainSecret); err != nil {
		return err
	}
	secret, err := core.ForStorageWithEncryptedFallback(appID, plainSecret, f.Keychain)
	if err != nil {
		return output.Errorf(output.ExitInternal, "internal", "%v", err)
	}
	if err := saveAsOnlyApp(appID, secret, brand, lang); err != nil {
		if plainSecret.IsPlain() {
			core.RemoveSecretStore(secret, f.Keychain)
		}
		return output.Errorf(output.ExitInternal, "internal", "failed to save config: %v", err)
	}
	cleanupReplacedCurrentAppSecret(existing, f, appID, secret)
	cleanupOldConfig(existing, f, appID)
	warnIfEncryptedSecretFallback(f.IOStreams.ErrOut, secret, plainSecret)
	return nil
}

// configInitRun initializes or updates the local CLI app configuration.
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

	// Mode 1: Non-interactive
	if opts.AppID != "" && opts.appSecret != "" {
		brand := parseBrand(opts.Brand)
		plainSecret := core.PlainSecret(opts.appSecret)
		if err := storeAndSaveOnlyApp(existing, f, opts.AppID, plainSecret, brand, opts.Lang); err != nil {
			return err
		}
		output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("Configuration saved to %s", core.GetConfigPath()))
		output.PrintJson(f.IOStreams.Out, map[string]interface{}{"appId": opts.AppID, "appSecret": "****", "brand": brand})
		return nil
	}

	// For interactive modes, prompt language selection if --lang was not explicitly set
	if f.IOStreams.IsTerminal && !opts.langExplicit && !opts.hasAnyNonInteractiveFlag() {
		savedLang := ""
		if existing != nil && len(existing.Apps) > 0 {
			savedLang = existing.Apps[0].Lang
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
		plainSecret := core.PlainSecret(result.AppSecret)
		if err := storeAndSaveOnlyApp(existing, f, result.AppID, plainSecret, result.Brand, opts.Lang); err != nil {
			return err
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
			plainSecret := core.PlainSecret(result.AppSecret)
			if err := storeAndSaveOnlyApp(existing, f, result.AppID, plainSecret, result.Brand, opts.Lang); err != nil {
				return err
			}
		} else if result.Mode == "existing" && result.AppID != "" {
			// Existing app with unchanged secret — update app ID and brand only
			if existing != nil && len(existing.Apps) > 0 {
				if err := validateSecretReuse(result.AppID, existing.Apps[0].AppSecret); err != nil {
					return err
				}
				existing.Apps[0].AppId = result.AppID
				existing.Apps[0].Brand = result.Brand
				existing.Apps[0].Lang = opts.Lang
				if err := core.SaveMultiAppConfig(existing); err != nil {
					return output.Errorf(output.ExitInternal, "internal", "failed to save config: %v", err)
				}
			} else {
				return output.ErrValidation("App Secret cannot be empty for new configuration")
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
	if existing != nil && len(existing.Apps) > 0 {
		firstApp = &existing.Apps[0]
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

	if err := storeAndSaveOnlyApp(existing, f, resolvedAppId, resolvedSecret, parseBrand(resolvedBrand), opts.Lang); err != nil {
		return err
	}
	output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("Configuration saved to %s", core.GetConfigPath()))
	return nil
}
