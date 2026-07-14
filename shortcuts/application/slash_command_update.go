// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package application

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// validateUpdateTarget enforces: exactly one of --command-id/--command, and at
// least one editable field; --description-i18n requires --description (PATCH
// replaces the whole description object - sending i18n alone would drop
// default_value, so both values must be provided together).
func validateUpdateTarget(runtime *common.RuntimeContext) error {
	id := strings.TrimSpace(runtime.Str("command-id"))
	name := strings.TrimSpace(runtime.Str("command"))
	if (id == "") == (name == "") {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"provide exactly one of --command-id or --command").WithParam("--command-id")
	}
	if name != "" {
		if err := validateCommandName(name, "--command"); err != nil {
			return err
		}
	}
	hasDesc := strings.TrimSpace(runtime.Str("description")) != ""
	hasI18n := len(runtime.StrArray("description-i18n")) > 0
	hasIcon := strings.TrimSpace(runtime.Str("icon-key")) != ""
	if !hasDesc && !hasI18n && !hasIcon {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"provide at least one of --description / --description-i18n / --icon-key").WithParam("--description")
	}
	if hasI18n && !hasDesc {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--description-i18n requires --description: PATCH replaces the whole description object, so default_value must be provided together").WithParam("--description-i18n")
	}
	if _, err := parseDescriptionI18n(runtime.StrArray("description-i18n")); err != nil {
		return err
	}
	return nil
}

// SlashCommandUpdate updates description/i18n/icon of an existing slash command.
var SlashCommandUpdate = common.Shortcut{
	Service:     "application",
	Command:     "+slash-command-update",
	Description: "Update description / localized descriptions / icon of a slash command on the current bound app, addressed by --command-id or by name via --command",
	Risk:        "write",
	Scopes:      []string{"application:app_slash_command:write"},
	ConditionalScopes: []string{
		"application:app_slash_command:read", // only the --command by-name path lists to resolve the id
	},
	AuthTypes: []string{"bot", "user"},
	Flags: []common.Flag{
		{Name: "command-id", Desc: "target command_id (from +slash-command-list or create output); mutually exclusive with --command"},
		{Name: "command", Desc: "target command name WITHOUT leading slash; resolved via live list (needs read scope); mutually exclusive with --command-id"},
		{Name: "description", Desc: "new default description (description.default_value)"},
		{Name: "description-i18n", Type: "string_array", Desc: "localized description, repeatable <lang>=<text>; REPLACES the whole i18n map (missing languages are dropped); requires --description"},
		{Name: "icon-key", Desc: "new icon key (invalid keys rejected server-side with code 40000031)"},
	},
	Tips: []string{
		`lark-cli application +slash-command-update --command greet --description "new text" --as bot`,
		"PATCH is field-level partial: fields you do not pass are preserved server-side",
		"the command NAME itself cannot be changed (API limitation): rename = delete + create (new command_id)",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateUpdateTarget(runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		i18n, err := parseDescriptionI18n(runtime.StrArray("description-i18n"))
		if err != nil {
			// The CLI validates first; keep this guard for direct DryRun callers.
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		body := buildSlashCommandBody("", runtime.Str("description"), i18n, runtime.Str("icon-key"))
		d := common.NewDryRunAPI()
		target := strings.TrimSpace(runtime.Str("command-id"))
		if target == "" {
			name := strings.TrimSpace(runtime.Str("command"))
			d.GET(slashCommandBasePath).
				Desc(fmt.Sprintf("resolve command_id by name %q via GET list first", name))
			target = "<resolved_command_id>"
		} else {
			target = encodeCommandIDPathSegment(target)
		}
		return d.PATCH(slashCommandBasePath + "/" + target).
			Desc("Update a slash command by command_id").
			Body(body)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		id := strings.TrimSpace(runtime.Str("command-id"))
		if id == "" {
			resolved, err := resolveCommandID(runtime, strings.TrimSpace(runtime.Str("command")))
			if err != nil {
				return err
			}
			id = resolved
		}
		i18n, err := parseDescriptionI18n(runtime.StrArray("description-i18n"))
		if err != nil {
			return err
		}
		body := buildSlashCommandBody("", runtime.Str("description"), i18n, runtime.Str("icon-key"))
		data, err := runtime.CallAPITyped("PATCH", slashCommandBasePath+"/"+encodeCommandIDPathSegment(id), nil, body)
		if err != nil {
			return err
		}
		if data == nil {
			data = map[string]interface{}{}
		}
		data["action"] = "updated"
		fmt.Fprintln(runtime.IO().ErrOut, clientCacheHint)
		runtime.OutFormat(data, nil, func(w io.Writer) {
			fmt.Fprintf(w, "updated /%v (command_id: %v)\n", data["command"], data["command_id"])
		})
		return nil
	},
}
