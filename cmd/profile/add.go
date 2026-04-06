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

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
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
	cmd.Flags().StringVar(&lang, "lang", "zh", "language for interactive prompts (zh or en)")
	cmd.Flags().BoolVar(&use, "use", false, "switch to this profile after adding")

	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("app-id")

	return cmd
}

func profileAddRun(f *cmdutil.Factory, name, appID string, appSecretStdin bool, brand, lang string, useAfter bool) error {
	if err := core.ValidateProfileName(name); err != nil {
		return output.ErrValidation("%v", err)
	}

	// Read secret from stdin
	if !appSecretStdin {
		return output.ErrValidation("app secret must be provided via stdin: use --app-secret-stdin and pipe the secret")
	}
	scanner := bufio.NewScanner(f.IOStreams.In)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return output.ErrValidation("failed to read secret from stdin: %v", err)
		}
		return output.ErrValidation("stdin is empty, expected app secret")
	}
	appSecret := strings.TrimSpace(scanner.Text())
	if appSecret == "" {
		return output.ErrValidation("app secret read from stdin is empty")
	}

	// Load or create config
	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return output.Errorf(output.ExitInternal, "internal", "failed to load config: %v", err)
		}
		multi = &core.MultiAppConfig{}
	}

	// Check name uniqueness
	if multi.FindApp(name) != nil {
		return output.ErrValidation("profile %q already exists", name)
	}

	// Check app-id uniqueness — keychain stores secrets by appId, so
	// multiple profiles sharing the same appId would collide on credentials.
	for _, a := range multi.Apps {
		if a.AppId == appID {
			return output.ErrValidation("app-id %q is already used by profile %q; each profile must have a unique app-id", appID, a.ProfileName())
		}
	}

	// Store secret securely
	secret, err := core.ForStorage(appID, core.PlainSecret(appSecret), f.Keychain)
	if err != nil {
		return output.Errorf(output.ExitInternal, "internal", "%v", err)
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
		Lang:      lang,
		Users:     []core.AppUser{},
	})

	if useAfter {
		if previousName != "" {
			multi.PreviousApp = previousName
		}
		multi.CurrentApp = name
	}

	if err := core.SaveMultiAppConfig(multi); err != nil {
		return output.Errorf(output.ExitInternal, "internal", "failed to save config: %v", err)
	}

	output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("Profile %q added (%s, %s)", name, appID, parsedBrand))
	if useAfter {
		output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("Switched to profile %q", name))
	}
	return nil
}
