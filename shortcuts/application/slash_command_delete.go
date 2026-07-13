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

// SlashCommandDelete removes a slash command (irreversible; command_id is not
// reused - recreating the same name yields a NEW id).
var SlashCommandDelete = common.Shortcut{
	Service:     "application",
	Command:     "+slash-command-delete",
	Description: "Delete a slash command from the current bound app (high-risk: irreversible; recreating the same name yields a new command_id)",
	Risk:        "high-risk-write",
	Scopes:      []string{"application:app_slash_command:write"},
	ConditionalScopes: []string{
		"application:app_slash_command:read", // only the --command by-name path
	},
	AuthTypes: []string{"bot", "user"},
	Flags: []common.Flag{
		{Name: "command-id", Desc: "target command_id; mutually exclusive with --command"},
		{Name: "command", Desc: "target command name WITHOUT leading slash (resolved via live list, needs read scope); mutually exclusive with --command-id"},
	},
	Tips: []string{
		"lark-cli application +slash-command-delete --command greet --yes --as bot",
		"deleted commands may linger in clients for ~5 minutes (client cache)",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		id := strings.TrimSpace(runtime.Str("command-id"))
		name := strings.TrimSpace(runtime.Str("command"))
		if (id == "") == (name == "") {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"provide exactly one of --command-id or --command").WithParam("--command-id")
		}
		if name != "" {
			return validateCommandName(name, "--command")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		d := common.NewDryRunAPI().Desc("HIGH-RISK: delete a slash command (irreversible; same-name recreate gets a NEW command_id)")
		target := strings.TrimSpace(runtime.Str("command-id"))
		if target == "" {
			name := strings.TrimSpace(runtime.Str("command"))
			d.GET(slashCommandBasePath).
				Desc(fmt.Sprintf("resolve command_id by name %q via GET list first", name))
			target = "<resolved_command_id>"
		} else {
			target = encodeCommandIDPathSegment(target)
		}
		return d.DELETE(slashCommandBasePath + "/" + target)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		id := strings.TrimSpace(runtime.Str("command-id"))
		name := strings.TrimSpace(runtime.Str("command"))
		if id == "" {
			resolved, err := resolveCommandID(runtime, name)
			if err != nil {
				return err
			}
			id = resolved
		}
		if _, err := runtime.CallAPITyped("DELETE", slashCommandBasePath+"/"+encodeCommandIDPathSegment(id), nil, nil); err != nil {
			return err
		}
		out := map[string]interface{}{"action": "deleted", "command_id": id}
		if name != "" {
			out["command"] = name
		}
		fmt.Fprintln(runtime.IO().ErrOut, clientCacheHint)
		fmt.Fprintln(runtime.IO().ErrOut, "note: recreating the same command name will yield a NEW command_id.")
		runtime.OutFormat(out, nil, func(w io.Writer) {
			fmt.Fprintf(w, "deleted command_id %s\n", id)
		})
		return nil
	},
}
