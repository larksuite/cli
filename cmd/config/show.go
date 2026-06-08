// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/spf13/cobra"
)

// ConfigShowOptions holds all inputs for config show.
type ConfigShowOptions struct {
	Factory *cmdutil.Factory
}

// NewCmdConfigShow creates the config show subcommand.
func NewCmdConfigShow(f *cmdutil.Factory, runF func(*ConfigShowOptions) error) *cobra.Command {
	opts := &ConfigShowOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return configShowRun(opts)
		},
	}
	cmdutil.SetRisk(cmd, "read")

	return cmd
}

func configShowRun(opts *ConfigShowOptions) error {
	f := opts.Factory

	config, err := core.LoadMultiAppConfig()
	if err != nil {
		// R2 transparency: a forward-incompat *core.ConfigError must reach
		// the dispatcher with its upgrade Hint intact. The previous
		// `errs.NewConfigError(SubtypeInvalidConfig, "failed to load
		// config: %v", err).WithCause(err)` outer-typed envelope hid the
		// R2 hint behind isOuterTypedError, leaving operators with a
		// generic "failed to load config" message instead of "upgrade
		// lark-cli". PassThroughOrNotConfigured maps:
		//   - *core.ConfigError (R2 / parse) → returned verbatim for the
		//     dispatcher to promote with its Hint intact
		//   - os.ErrNotExist → workspace-aware NotConfiguredError
		//   - other (permission, ...) → wrapped *core.ConfigError so the
		//     dispatcher still gets a typed envelope.
		return core.PassThroughOrNotConfigured(err)
	}
	if config == nil || len(config.Apps) == 0 {
		return core.NotConfiguredError()
	}
	app := config.CurrentAppConfig(f.Invocation.Profile)
	if app == nil {
		// Apps[] is populated above this branch, so the resolution failure
		// is either (a) operator named a ghost via --profile, or (b) the
		// stored CurrentApp dangles. Neither is "no config" —
		// SubtypeNotConfigured would steer AI agents to `config init` and
		// clobber the existing profiles.
		if f.Invocation.Profile != "" {
			return errs.NewConfigError(errs.SubtypeInvalidArgument,
				"profile %q not found", f.Invocation.Profile).
				WithHint("available profiles: %s", strings.Join(config.ProfileNames(), ", "))
		}
		return errs.NewConfigError(errs.SubtypeInvalidConfig, "no active profile").
			WithHint("run: lark-cli profile list")
	}
	users := "(no logged-in users)"
	if len(app.Users) > 0 {
		var userStrs []string
		for _, u := range app.Users {
			userStrs = append(userStrs, fmt.Sprintf("%s (%s)", u.UserName, u.UserOpenId))
		}
		users = strings.Join(userStrs, ", ")
	}
	output.PrintJson(f.IOStreams.Out, map[string]interface{}{
		"workspace": core.CurrentWorkspace().Display(),
		"profile":   app.ProfileName(),
		"appId":     app.AppId,
		"appSecret": "****",
		"brand":     app.Brand,
		"lang":      app.Lang,
		"users":     users,
	})
	fmt.Fprintf(f.IOStreams.ErrOut, "\nConfig file path: %s\n", core.GetConfigPath())
	return nil
}
