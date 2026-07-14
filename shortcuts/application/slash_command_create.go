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

// SlashCommandCreate registers a new slash command on the current bound app.
var SlashCommandCreate = common.Shortcut{
	Service:     "application",
	Command:     "+slash-command-create",
	Description: "Register a slash command (/ command) on the current bound Open Platform app; --force converts a name collision into an update (idempotent re-run)",
	Risk:        "write",
	Scopes:      []string{"application:app_slash_command:write"},
	ConditionalScopes: []string{
		"application:app_slash_command:read", // only the --force collision path lists to resolve the id
	},
	AuthTypes: []string{"bot", "user"},
	Flags: []common.Flag{
		{Name: "command", Desc: "command name WITHOUT the leading slash (server enforces uniqueness per app; max 100 commands)", Required: true},
		{Name: "description", Desc: "default description shown in the client command panel (description.default_value)", Required: true},
		{Name: "description-i18n", Type: "string_array", Desc: "localized description, repeatable, format <lang>=<text> (e.g. zh_cn=发送问候); language codes are passed through to the server"},
		{Name: "icon-key", Desc: "icon key (server default: skill_outlined; invalid keys are rejected server-side with code 40000031)"},
		{Name: "force", Type: "bool", Desc: "on name collision, resolve the existing command by name and update it in place"},
	},
	Tips: []string{
		`lark-cli application +slash-command-create --command greet --description "say hi" --description-i18n zh_cn=问候 --as bot`,
		"changes take ~5 minutes to appear in clients (client-side cache); the server updates immediately",
		"user identity needs explicit authorization first: lark-cli auth login --scope application:app_slash_command:write",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateCommandName(runtime.Str("command"), "--command"); err != nil {
			return err
		}
		if len(strings.TrimSpace(runtime.Str("description"))) == 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--description must not be blank").WithParam("--description")
		}
		if _, err := parseDescriptionI18n(runtime.StrArray("description-i18n")); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		i18n, err := parseDescriptionI18n(runtime.StrArray("description-i18n"))
		if err != nil {
			// The CLI validates first; keep this guard for direct DryRun callers.
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		name := strings.TrimSpace(runtime.Str("command"))
		body := buildSlashCommandBody(name, runtime.Str("description"), i18n, runtime.Str("icon-key"))
		d := common.NewDryRunAPI().
			Desc("Create a slash command on the current bound app").
			POST(slashCommandBasePath).
			Body(body)
		if runtime.Bool("force") {
			d.Desc("--force: on 'command already exists' (code 40000000), GET list to resolve command_id then PATCH the same body")
		}
		return d
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		name := strings.TrimSpace(runtime.Str("command"))
		i18n, err := parseDescriptionI18n(runtime.StrArray("description-i18n"))
		if err != nil {
			return err
		}
		body := buildSlashCommandBody(name, runtime.Str("description"), i18n, runtime.Str("icon-key"))

		data, err := runtime.CallAPITyped("POST", slashCommandBasePath, nil, body)
		action := "created"
		if err != nil {
			if !isCommandExists(err) {
				return err
			}
			if !runtime.Bool("force") {
				p, _ := errs.ProblemOf(err)
				rewrapped := errs.NewAPIError(errs.SubtypeAlreadyExists, "slash command %q already exists", name).
					WithHint("rerun with --force to update it, or use `lark-cli application +slash-command-update --command %q`", name).
					WithCause(err)
				if p.Code != 0 {
					rewrapped = rewrapped.WithCode(p.Code)
				}
				if p.LogID != "" {
					rewrapped = rewrapped.WithLogID(p.LogID)
				}
				return rewrapped
			}
			// --force: name collision -> resolve id -> PATCH (idempotent re-run).
			id, rerr := resolveCommandID(runtime, name)
			if rerr != nil {
				return rerr
			}
			patchBody := buildSlashCommandBody("", runtime.Str("description"), i18n, runtime.Str("icon-key"))
			data, err = runtime.CallAPITyped("PATCH", slashCommandBasePath+"/"+encodeCommandIDPathSegment(id), nil, patchBody)
			if err != nil {
				return err
			}
			action = "updated"
		}
		if data == nil {
			data = map[string]interface{}{}
		}
		data["action"] = action
		fmt.Fprintln(runtime.IO().ErrOut, clientCacheHint)
		runtime.OutFormat(data, nil, func(w io.Writer) {
			fmt.Fprintf(w, "%s /%v (command_id: %v)\n", action, data["command"], data["command_id"])
		})
		return nil
	},
}
