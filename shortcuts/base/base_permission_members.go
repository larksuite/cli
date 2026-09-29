// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

type permissionMember struct {
	MemberID   string `json:"member_id"`
	MemberType string `json:"member_type"`
}

type permissionMemberBatch struct {
	Members []permissionMember `json:"members"`
}

var BaseRoleMemberList = common.Shortcut{
	Service:     "base",
	Command:     "+role-member-list",
	Description: "List a page of custom-role members",
	Risk:        "read",
	Scopes:      []string{"base:role:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		{Name: "role-id", Desc: "custom role ID from +role-list", Required: true},
		{Name: "page-size", Type: "int", Default: "50", Desc: "members per page (1-100)"},
		{Name: "page-token", Desc: "continuation token returned by the previous page"},
	},
	Validate: func(_ context.Context, rt *common.RuntimeContext) error {
		if err := validatePermissionTarget(rt, true); err != nil {
			return err
		}
		if rt.Int("page-size") < 1 || rt.Int("page-size") > 100 {
			return baseFlagErrorf("--page-size must be between 1 and 100")
		}
		return nil
	},
	DryRun: func(_ context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().GET(permissionMemberPath(rt)).Params(permissionMemberPage(rt))
	},
	Execute: func(_ context.Context, rt *common.RuntimeContext) error {
		return executePermissionRequest(rt, "GET", permissionMemberPath(rt), permissionMemberPage(rt), nil)
	},
}

var BaseRoleMemberAdd = permissionMemberWriteShortcut("+role-member-add", "Add users, departments or chats to a custom role", "batch_create", "write")

// Removing up to 100 members silently revokes their role access, so it carries
// the same confirmation gate as +role-update / +role-delete.
var BaseRoleMemberRemove = permissionMemberWriteShortcut("+role-member-remove", "Remove specified members from a custom role", "batch_delete", "high-risk-write")

func permissionMemberWriteShortcut(command, description, action, risk string) common.Shortcut {
	var tips []string
	if risk == "high-risk-write" {
		tips = []string{baseHighRiskYesTip}
	}
	return common.Shortcut{
		Service:     "base",
		Command:     command,
		Description: description,
		Risk:        risk,
		Tips:        tips,
		Scopes:      []string{"base:role:update"},
		AuthTypes:   authTypes(),
		Flags: []common.Flag{
			baseTokenFlag(true),
			{Name: "role-id", Desc: "custom role ID from +role-list", Required: true},
			{Name: "user-id", Type: "string_array", Desc: "user open_id; repeat for multiple users"},
			{Name: "department-id", Type: "string_array", Desc: "department open_department_id; repeat for multiple departments"},
			{Name: "chat-id", Type: "string_array", Desc: "chat open_chat_id; repeat for multiple chats"},
		},
		Validate: func(_ context.Context, rt *common.RuntimeContext) error {
			if err := validatePermissionTarget(rt, true); err != nil {
				return err
			}
			_, err := permissionMembers(rt)
			return err
		},
		DryRun: func(_ context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
			members, _ := permissionMembers(rt) // Validate runs before DryRun.
			return common.NewDryRunAPI().POST(permissionMemberPath(rt) + "/" + action).Body(permissionMemberBatch{Members: members})
		},
		Execute: func(_ context.Context, rt *common.RuntimeContext) error {
			members, err := permissionMembers(rt)
			if err != nil {
				return err
			}
			return executePermissionRequest(rt, "POST", permissionMemberPath(rt)+"/"+action, nil, permissionMemberBatch{Members: members})
		},
	}
}

func permissionMemberPath(rt *common.RuntimeContext) string {
	return baseV3Path("bases", rt.Str("base-token"), "roles", rt.Str("role-id"), "members")
}

func permissionMemberPage(rt *common.RuntimeContext) map[string]interface{} {
	params := map[string]interface{}{"page_size": rt.Int("page-size")}
	if rt.Changed("page-token") {
		params["page_token"] = rt.Str("page-token")
	}
	return params
}

func permissionMembers(rt *common.RuntimeContext) ([]permissionMember, error) {
	var members []permissionMember
	seen := make(map[permissionMember]bool)
	count := 0
	for _, kind := range []struct{ flag, memberType string }{
		{"user-id", "openid"}, {"department-id", "opendepartmentid"}, {"chat-id", "openchat"},
	} {
		for _, id := range rt.StrArray(kind.flag) {
			count++
			if strings.TrimSpace(id) == "" || strings.TrimSpace(id) != id {
				return nil, baseFlagErrorf("--%s must be a nonblank ID without surrounding whitespace", kind.flag)
			}
			member := permissionMember{MemberID: id, MemberType: kind.memberType}
			if !seen[member] {
				members = append(members, member)
				seen[member] = true
			}
		}
	}
	if count < 1 || count > 100 {
		return nil, baseFlagErrorf("provide 1-100 members using --user-id, --department-id or --chat-id")
	}
	return members, nil
}

func validatePermissionTarget(rt *common.RuntimeContext, role bool) error {
	flags := []string{"base-token"}
	if role {
		flags = append(flags, "role-id")
	}
	for _, flag := range flags {
		value := rt.Str(flag)
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "/?#") || value == "." || value == ".." {
			return baseFlagErrorf("--%s must be a nonblank ID, not a URL or path", flag)
		}
	}
	return nil
}
