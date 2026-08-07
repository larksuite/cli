// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package profile

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/brand"
	"github.com/larksuite/cli/errs"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	configpkg "github.com/larksuite/cli/internal/config"
	"github.com/larksuite/cli/internal/output"
)

// profileListItem is the JSON output for a single profile entry. Active and
// Effective answer different questions and may disagree: Active marks the
// persisted default (who applies when no selector is present), Effective
// marks the profile this very invocation resolved to — a --profile flag or
// LARKSUITE_CLI_PROFILE overrides the default without touching it.
type profileListItem struct {
	Name            string      `json:"name"`
	AppID           string      `json:"appId"`
	Brand           brand.Brand `json:"brand"`
	Active          bool        `json:"active"`
	Effective       bool        `json:"effective,omitempty"`
	EffectiveSource string      `json:"effectiveSource,omitempty"` // config | flag | environment
	User            string      `json:"user,omitempty"`
	TokenStatus     string      `json:"tokenStatus,omitempty"`
}

// NewCmdProfileList creates the profile list subcommand.
func NewCmdProfileList(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			return profileListRun(f)
		},
	}
	cmdutil.SetRisk(cmd, "read")
	return cmd
}

func profileListRun(f *cmdutil.Factory) error {
	multi, err := configpkg.LoadMultiAppConfig()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			output.PrintJson(f.IOStreams.Out, []profileListItem{})
			return nil
		}
		return errs.NewValidationError(errs.SubtypeFailedPrecondition, "failed to load config: %v", err).WithCause(err)
	}
	if multi == nil || len(multi.Apps) == 0 {
		output.PrintJson(f.IOStreams.Out, []profileListItem{})
		return nil
	}

	// Active reports the persisted default on purpose (CurrentAppConfig("")),
	// never the ephemeral override; Effective carries the override dimension.
	currentApp := multi.CurrentAppConfig("")
	currentName := ""
	if currentApp != nil {
		currentName = currentApp.ProfileName()
	}
	effectiveApp, effectiveSource := multi.EffectiveProfile(f.Invocation.Profile, f.Invocation.ProfileSource)
	effectiveName := ""
	if effectiveApp != nil {
		effectiveName = effectiveApp.ProfileName()
	} else if f.Invocation.Profile != "" {
		// The selector is dangling. The list itself is the recovery surface,
		// so stay exit 0 — but say why no entry is marked effective.
		fmt.Fprintf(f.IOStreams.ErrOut, "Warning: %s=%q does not match any profile\n",
			f.Invocation.ProfileSource.SelectorLabel(), f.Invocation.Profile)
	}

	items := make([]profileListItem, 0, len(multi.Apps))
	for i := range multi.Apps {
		app := &multi.Apps[i]
		name := app.ProfileName()

		item := profileListItem{
			Name:   name,
			AppID:  app.AppId,
			Brand:  app.Brand,
			Active: name == currentName,
		}
		if effectiveApp != nil && name == effectiveName {
			item.Effective = true
			item.EffectiveSource = effectiveSource.String()
		}

		if len(app.Users) > 0 {
			item.User = app.Users[0].UserName
			stored := larkauth.GetStoredToken(app.AppId, app.Users[0].UserOpenId)
			if stored != nil {
				item.TokenStatus = larkauth.TokenStatus(stored)
			}
		}

		items = append(items, item)
	}
	output.PrintJson(f.IOStreams.Out, items)
	return nil
}
