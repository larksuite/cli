// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

func validateRestoreFlags(cmd *cobra.Command, opts *ConfigInitOptions) error {
	if !opts.Restore {
		return nil
	}
	for _, name := range []string{"new", "app-id", "app-secret-stdin", "brand", "lang", "name"} {
		if cmd.Flags().Changed(name) {
			flag := "--" + name
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"%s cannot be used with --restore", flag).WithParam(flag)
		}
	}
	return nil
}

func runRestoreFlow(
	opts *ConfigInitOptions,
	existing *core.MultiAppConfig,
	f *cmdutil.Factory,
	msg *initMsg,
) error {
	app, err := existing.RequireAppConfig(f.Invocation.Profile, f.Invocation.ProfileSource)
	if err != nil {
		return err
	}
	if app.AppId == "" {
		return errs.NewConfigError(errs.SubtypeInvalidConfig,
			"app selected for restore has an empty app ID")
	}

	result, err := runCreateAppFlow(opts.Ctx, f, core.ParseBrand(string(app.Brand)), msg, app.AppId)
	if err != nil {
		return err
	}
	if result == nil || result.AppID != app.AppId || result.AppSecret == "" {
		return errs.NewConfigError(errs.SubtypeInvalidClient,
			"app restore returned invalid credentials for the configured app")
	}

	secret, err := core.ForStorage(app.AppId, core.PlainSecret(result.AppSecret), f.Keychain)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeSDKError, "%v", err).WithCause(err)
	}
	app.AppSecret = secret
	app.Brand = result.Brand
	if err := core.SaveMultiAppConfig(existing); err != nil {
		return wrapSaveConfigError(err)
	}

	output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf(msg.AppCreated, app.AppId))
	output.PrintJson(f.IOStreams.Out, map[string]interface{}{
		"appId": app.AppId, "appSecret": "****", "brand": app.Brand,
	})
	return runProbe(opts.Ctx, f, app.AppId, result.AppSecret, app.Brand)
}
