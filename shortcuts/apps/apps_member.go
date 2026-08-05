// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/larksuite/cli/shortcuts/common"
)

const (
	memberReadHint  = "verify --app-id identifies a Miaoda app you can access; list apps with `lark-cli apps +list`"
	memberWriteHint = "verify the app, external member ID, and current collaborator policy; read the latest state before retrying"
)

var AppsMemberList = common.Shortcut{
	Service: appsService, Command: "+member-list", Description: "List application collaborators",
	Risk: "read", Scopes: []string{"spark:app:read"}, AuthTypes: []string{"user"},
	Tips:      []string{"Example: lark-cli apps +member-list --app-id <app_id>"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app ID (app_...)"},
		{Name: "role", Desc: "filter permission", Enum: memberRoles},
		{Name: "member-type", Desc: "filter collaborator type", Enum: memberListTypes},
		{Name: "page-size", Type: "int", Default: "20", Desc: "page size (1-100)"},
		{Name: "page-token", Desc: "opaque page token returned by the previous page"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if err := validateMemberAppID(rctx); err != nil {
			return err
		}
		_, err := buildMemberListParams(rctx)
		return err
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		params, _ := buildMemberListParams(rctx)
		return common.NewDryRunAPI().
			GET(memberListURL(rctx)).
			Desc("List application collaborators").
			Params(params)
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		params, err := buildMemberListParams(rctx)
		if err != nil {
			return err
		}
		data, err := rctx.CallAPITyped("GET", memberListURL(rctx), params, nil)
		if err != nil {
			return withAppsHint(normalizeMemberAPIError(err), memberReadHint)
		}
		out, err := projectMemberListData(data)
		if err != nil {
			return err
		}
		rctx.OutFormat(out, nil, func(w io.Writer) { renderMemberListPretty(w, out) })
		return nil
	},
}

var AppsMemberAdd = common.Shortcut{
	Service: appsService, Command: "+member-add", Description: "Add an application collaborator",
	Risk: "high-risk-write", Scopes: []string{"spark:app:write"}, AuthTypes: []string{"user"},
	Tips:      []string{"Example: lark-cli apps +member-add --app-id <app_id> --member-type openid --member-id <open_id> --perm view --yes"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app ID (app_...)"},
		{Name: "member-type", Desc: "collaborator ID type", Required: true, Enum: memberWriteTypes},
		{Name: "member-id", Desc: "one external user, chat, or department ID", Required: true},
		{Name: "perm", Desc: "collaborator permission", Required: true, Enum: memberRoles},
		{Name: "need-notification", Type: "bool", Desc: "notify the collaborator"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		return validateMemberMutation(rctx, true)
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		body, _ := buildMemberAddRequest(rctx)
		return common.NewDryRunAPI().
			POST(memberListURL(rctx)).
			Desc("Add an application collaborator").
			Body(body)
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		body, err := buildMemberAddRequest(rctx)
		if err != nil {
			return err
		}
		data, err := rctx.CallAPITyped("POST", memberListURL(rctx), nil, body)
		if err != nil {
			return withAppsHint(normalizeMemberAPIError(err), memberWriteHint)
		}
		out, err := projectMemberAddData(data)
		if err != nil {
			return err
		}
		rctx.OutFormat(out, nil, func(w io.Writer) { renderMemberMutationPretty(w, "added", out.Member, out.Changed) })
		return nil
	},
}

var AppsMemberUpdate = common.Shortcut{
	Service: appsService, Command: "+member-update", Description: "Update an application collaborator",
	Risk: "high-risk-write", Scopes: []string{"spark:app:write"}, AuthTypes: []string{"user"},
	Tips:      []string{"Example: lark-cli apps +member-update --app-id <app_id> --member-type openid --member-id <open_id> --perm edit --yes"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app ID (app_...)"},
		{Name: "member-type", Desc: "collaborator ID type", Required: true, Enum: memberWriteTypes},
		{Name: "member-id", Desc: "one external user, chat, or department ID", Required: true},
		{Name: "perm", Desc: "collaborator permission", Required: true, Enum: memberRoles},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		return validateMemberMutation(rctx, true)
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		body, _ := buildMemberUpdateRequest(rctx)
		return common.NewDryRunAPI().
			PATCH(memberListURL(rctx)).
			Desc("Update an application collaborator").
			Body(body)
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		body, err := buildMemberUpdateRequest(rctx)
		if err != nil {
			return err
		}
		data, err := rctx.CallAPITyped("PATCH", memberListURL(rctx), nil, body)
		if err != nil {
			return withAppsHint(normalizeMemberAPIError(err), memberWriteHint)
		}
		out, err := projectMemberUpdateData(data)
		if err != nil {
			return err
		}
		rctx.OutFormat(out, nil, func(w io.Writer) {
			renderMemberMutationPretty(w, "updated", out.Member, out.Changed)
			fmt.Fprintf(w, "permission: %s -> %s\n", memberDisplayValue(out.BeforeRole), memberDisplayValue(out.AfterRole))
		})
		return nil
	},
}

var AppsMemberRemove = common.Shortcut{
	Service: appsService, Command: "+member-remove", Description: "Remove an application collaborator",
	Risk: "high-risk-write", Scopes: []string{"spark:app:write"}, AuthTypes: []string{"user"},
	Tips:      []string{"Example: lark-cli apps +member-remove --app-id <app_id> --member-type openid --member-id <open_id> --yes"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app ID (app_...)"},
		{Name: "member-type", Desc: "collaborator ID type", Required: true, Enum: memberWriteTypes},
		{Name: "member-id", Desc: "one external user, chat, or department ID", Required: true},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		return validateMemberMutation(rctx, false)
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		body, _ := buildMemberRemoveRequest(rctx)
		return common.NewDryRunAPI().
			POST(memberRemoveURL(rctx)).
			Desc("Remove an application collaborator").
			Body(body)
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		body, err := buildMemberRemoveRequest(rctx)
		if err != nil {
			return err
		}
		data, err := rctx.CallAPITyped("POST", memberRemoveURL(rctx), nil, body)
		if err != nil {
			return withAppsHint(normalizeMemberAPIError(err), memberWriteHint)
		}
		out, err := projectMemberRemoveData(data)
		if err != nil {
			return err
		}
		rctx.OutFormat(out, nil, func(w io.Writer) { renderMemberMutationPretty(w, "removed", out.Member, out.Changed) })
		return nil
	},
}

var AppsMemberSettingsGet = common.Shortcut{
	Service: appsService, Command: "+member-settings-get", Description: "Get application collaborator settings",
	Risk: "read", Scopes: []string{"spark:app:read"}, AuthTypes: []string{"user"},
	Tips:      []string{"Example: lark-cli apps +member-settings-get --app-id <app_id>"},
	HasFormat: true,
	Flags:     []common.Flag{{Name: "app-id", Desc: "Miaoda app ID (app_...)"}},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		return validateMemberAppID(rctx)
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			GET(memberSettingsURL(rctx)).
			Desc("Get application collaborator settings")
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		data, err := rctx.CallAPITyped("GET", memberSettingsURL(rctx), nil, nil)
		if err != nil {
			return withAppsHint(normalizeMemberAPIError(err), memberReadHint)
		}
		out, err := projectMemberSettingsGetData(data)
		if err != nil {
			return err
		}
		rctx.OutFormat(out, nil, func(w io.Writer) { renderMemberSettingsPretty(w, out.Settings) })
		return nil
	},
}

var AppsMemberSettingsSet = common.Shortcut{
	Service: appsService, Command: "+member-settings-set", Description: "Update application collaborator settings",
	Risk: "high-risk-write", Scopes: []string{"spark:app:write"}, AuthTypes: []string{"user"},
	Tips:      []string{"Example: lark-cli apps +member-settings-set --app-id <app_id> --external-access enabled --yes"},
	HasFormat: true,
	Flags:     memberSettingsSetFlags(),
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		return validateMemberSettingsSet(rctx)
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		body, _ := buildMemberSettingsUpdateRequest(rctx)
		return common.NewDryRunAPI().
			PATCH(memberSettingsURL(rctx)).
			Desc("Update application collaborator settings").
			Body(body)
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		body, err := buildMemberSettingsUpdateRequest(rctx)
		if err != nil {
			return err
		}
		data, err := rctx.CallAPITyped("PATCH", memberSettingsURL(rctx), nil, body)
		if err != nil {
			return withAppsHint(normalizeMemberAPIError(err), memberWriteHint)
		}
		out, err := projectMemberSettingsSetData(data)
		if err != nil {
			return err
		}
		rctx.OutFormat(out, nil, func(w io.Writer) {
			fmt.Fprintf(w, "changed: %t\n", out.Changed)
			renderMemberSettingsPretty(w, out.Settings)
		})
		return nil
	},
}

func renderMemberListPretty(w io.Writer, out memberListOutput) {
	fmt.Fprintf(w, "%d collaborator(s)\n", len(out.Items))
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "MEMBER_TYPE\tMEMBER_ID\tNAME\tROLE")
	for _, item := range out.Items {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			memberDisplayValue(item.MemberType),
			memberDisplayValue(item.MemberID),
			memberDisplayValue(item.Name),
			memberDisplayValue(item.Role),
		)
	}
	_ = tw.Flush()
	if out.HasMore {
		fmt.Fprintf(w, "next page token: %s\n", memberDisplayValue(out.PageToken))
	}
}

func renderMemberMutationPretty(w io.Writer, action string, member memberOutput, changed bool) {
	fmt.Fprintf(w, "%s: %s %s (%s); changed: %t\n",
		action,
		memberDisplayValue(member.MemberType),
		memberDisplayValue(member.MemberID),
		memberDisplayValue(member.Role),
		changed,
	)
}

func renderMemberSettingsPretty(w io.Writer, settings memberSettingsResponse) {
	for _, spec := range memberSettingSpecs {
		if value := spec.responseValue(settings); value != nil {
			fmt.Fprintf(w, "%s: %s\n", spec.field, memberDisplayValue(*value))
		}
	}
}
