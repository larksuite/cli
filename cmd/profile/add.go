// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package profile

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/i18n"
	"github.com/larksuite/cli/internal/output"
)

// NewCmdProfileAdd creates the profile add subcommand.
func NewCmdProfileAdd(f *cmdutil.Factory) *cobra.Command {
	var (
		name           string
		appID          string
		appSecretStdin bool
		brand          string
		lang           string
		use            bool
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			return profileAddRun(f, name, appID, appSecretStdin, brand, lang, use)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "profile name (required)")
	cmd.Flags().StringVar(&appID, "app-id", "", "App ID (required)")
	cmd.Flags().BoolVar(&appSecretStdin, "app-secret-stdin", false, "read App Secret from stdin")
	cmd.Flags().StringVar(&brand, "brand", "feishu", "feishu or lark")
	cmd.Flags().StringVar(&lang, "lang", "", "language preference (e.g. zh or zh_cn)")
	cmd.Flags().BoolVar(&use, "use", false, "switch to this profile after adding")

	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("app-id")
	cmdutil.SetRisk(cmd, "write")

	return cmd
}

func profileAddRun(f *cmdutil.Factory, name, appID string, appSecretStdin bool, brand, lang string, useAfter bool) error {
	if err := core.ValidateProfileName(name); err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "%v", err).
			WithCause(err).
			WithParam("--name")
	}

	langPref, err := cmdutil.ParseLangFlag(lang)
	if err != nil {
		return err
	}
	lang = string(langPref)

	// Read secret from stdin
	if !appSecretStdin {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "app secret must be provided via stdin").
			WithHint("use --app-secret-stdin and pipe the secret").
			WithParam("--app-secret-stdin")
	}
	scanner := bufio.NewScanner(f.IOStreams.In)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return errs.NewValidationError(errs.SubtypeFailedPrecondition, "failed to read secret from stdin: %v", err).
				WithCause(err).
				WithParam("--app-secret-stdin")
		}
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "stdin is empty, expected app secret").
			WithHint("pipe the app secret to stdin").
			WithParam("--app-secret-stdin")
	}
	appSecret := strings.TrimSpace(scanner.Text())
	if appSecret == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "app secret read from stdin is empty").
			WithHint("pipe a non-empty app secret to stdin").
			WithParam("--app-secret-stdin")
	}

	// Load or create config
	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return errs.NewInternalError(errs.SubtypeFileIO, "failed to load config: %v", err).WithCause(err)
		}
		multi = &core.MultiAppConfig{}
	}

	// Check name uniqueness
	if multi.FindApp(name) != nil {
		return errs.NewValidationError(errs.SubtypeFailedPrecondition, "profile %q already exists", name).
			WithHint("choose a different name, or remove the existing profile first").
			WithParam("--name")
	}

	// Check app-id uniqueness — keychain stores secrets by appId, so
	// multiple profiles sharing the same appId would collide on credentials.
	for _, a := range multi.Apps {
		if a.AppId == appID {
			return errs.NewValidationError(errs.SubtypeFailedPrecondition, "app-id %q is already used by profile %q; each profile must have a unique app-id", appID, a.ProfileName()).
				WithParam("--app-id")
		}
	}

	// Store secret securely
	secret, err := core.ForStorage(appID, core.PlainSecret(appSecret), f.Keychain)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeStorage, "%v", err).WithCause(err)
	}

	parsedBrand := core.ParseBrand(brand)

	// Capture current profile before appending (avoid setting PreviousApp to self)
	var previousName string
	if useAfter {
		if currentApp := multi.CurrentAppConfig(""); currentApp != nil {
			previousName = currentApp.ProfileName()
		}
	}

	// Append profile
	multi.Apps = append(multi.Apps, core.AppConfig{
		Name:      name,
		AppId:     appID,
		AppSecret: secret,
		Brand:     parsedBrand,
		Lang:      i18n.Lang(lang),
		Users:     []core.AppUser{},
	})

	if useAfter {
		if previousName != "" {
			multi.PreviousApp = previousName
		}
		multi.CurrentApp = name
	}

	if err := core.SaveMultiAppConfig(multi); err != nil {
		return errs.NewInternalError(errs.SubtypeStorage, "failed to save config: %v", err).WithCause(err)
	}

	output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("Profile %q added (%s, %s)", name, appID, parsedBrand))
	if useAfter {
		output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("Switched to profile %q", name))
	}
	return nil
}
