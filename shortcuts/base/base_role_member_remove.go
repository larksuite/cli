// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseRoleMemberRemove = common.Shortcut{
	Service:     "base",
	Command:     "+role-member-remove",
	Description: "Remove collaborators (members) from a custom role in a Base",
	Risk:        "high-risk-write",
	Scopes:      []string{"base:collaborator:delete"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "base-token", Desc: "base token", Required: true},
		{Name: "role-id", Desc: "custom role ID (e.g. rolxxxxxx)", Required: true},
		{Name: "member-ids", Desc: "collaborator IDs, comma-separated (max 100). Interpretation is decided by --member-id-type", Required: true},
		{Name: "member-id-type", Desc: "ID namespace for --member-ids; one of open_id|union_id|user_id|chat_id|department_id|open_department_id (default open_id)", Default: "open_id"},
	},
	Tips: []string{
		baseHighRiskYesTip,
		"Requires advanced permissions to be enabled and the caller to be a Base admin/manager.",
		"Each member is sent with both its type and id. The type is the ID namespace (open_id, chat_id, ...), not the collaborator category. Use --dry-run to inspect the exact payload.",
		"Use +role-member-list first to confirm the target members, then pass --yes to confirm removal.",
		"Removing a member only revokes that role's permissions; it does not delete the user or their other access.",
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
			Desc("Remove collaborators from a custom role").
			POST("/open-apis/bitable/v1/apps/:base_token/roles/:role_id/members/batch_delete").
			Set("base_token", spec.BaseToken).
			Set("role_id", spec.RoleID).
			Body(map[string]interface{}{"member_list": buildRoleMemberList(spec)})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec, err := readRoleMemberSpec(runtime)
		if err != nil {
			return err
		}
		return executeRoleMemberBatch(runtime, spec, "/batch_delete", "remove role members failed")
	},
}
