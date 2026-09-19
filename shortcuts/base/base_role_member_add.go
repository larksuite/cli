// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseRoleMemberAdd = common.Shortcut{
	Service:     "base",
	Command:     "+role-member-add",
	Description: "Add collaborators (members) to a custom role in a Base",
	Risk:        "write",
	Scopes:      []string{"base:collaborator:create"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "base-token", Desc: "base token", Required: true},
		{Name: "role-id", Desc: "custom role ID (e.g. rolxxxxxx)", Required: true},
		{Name: "member-ids", Desc: "collaborator IDs, comma-separated (max 100). Interpretation is decided by --member-id-type", Required: true},
		{Name: "member-id-type", Desc: "ID namespace for --member-ids; one of open_id|union_id|user_id|chat_id|department_id|open_department_id (default open_id)", Default: "open_id"},
	},
	Tips: []string{
		"Requires advanced permissions to be enabled and the caller to be a Base admin/manager.",
		"Each member is sent with both its type and id. Sending an id without the matching type makes the API report success but add no member (silent no-op).",
		"The type is the ID namespace (open_id, chat_id, ...), not the collaborator category. Use --dry-run to inspect the exact payload.",
		"Adding the Base owner returns success but the owner never appears in +role-member-list: owners bypass role permissions, so do not retry owners.",
		"An invalid member ID is rejected with code 1254048; check that each ID exists, belongs to the same tenant, and matches --member-id-type.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := readRoleMemberSpec(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec, err := readRoleMemberSpec(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return common.NewDryRunAPI().
			Desc("Add collaborators to a custom role").
			POST("/open-apis/bitable/v1/apps/:base_token/roles/:role_id/members/batch_create").
			Set("base_token", spec.BaseToken).
			Set("role_id", spec.RoleID).
			Body(map[string]interface{}{"member_list": buildRoleMemberList(spec)})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec, err := readRoleMemberSpec(runtime)
		if err != nil {
			return err
		}
		return executeRoleMemberBatch(runtime, spec, "/batch_create", "add role members failed")
	},
}
