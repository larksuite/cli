// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// AppsRoleMemberList lists members of an app role.
var AppsRoleMemberList = common.Shortcut{
	Service:     appsService,
	Command:     "+role-member-list",
	Description: "List app role members",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +role-member-list --app-id <app_id> --role-id <role_id>",
		"Example: lark-cli apps +role-member-list --app-id <app_id> --role-id <role_id> --member-type user",
		"When only one member type is requested, pass --member-type user|department|chat instead of filtering the full response",
		"--member-type returns only the selected member field; omitted fields are unknown, so omit the flag for pre/post-write baselines",
		"--format table renders the CLI-native member_type/member_id table; this command has no --limit or --page-size flag",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: roleAppIDRequiredDesc, Required: true},
		{Name: "role-id", Desc: roleIDRequiredDesc, Required: true},
		{Name: "member-type", Desc: "filter member type", Enum: []string{"user", "department", "chat"}},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if err := validateRoleID(rctx); err != nil {
			return err
		}
		_, err := buildRoleMemberListParams(rctx)
		return err
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		// Validate already ran and called buildRoleMemberListParams; error is impossible here.
		params, _ := buildRoleMemberListParams(rctx)
		return common.NewDryRunAPI().
			GET(roleMemberListURL(rctx)).
			Desc("List app role members").
			Params(params)
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		params, err := buildRoleMemberListParams(rctx)
		if err != nil {
			return err
		}
		data, err := rctx.CallAPITyped("GET", roleMemberListURL(rctx), params, nil)
		memberType, _ := params["member_type"].(string)
		if shouldRetryRoleMemberListWithoutFilter(err, memberType) {
			fmt.Fprintln(rctx.IO().ErrOut, "warning: the server rejected chat member filtering; retried without the filter and returned only the chats field. Omit --member-type for a complete member baseline.")
			data, err = rctx.CallAPITyped("GET", roleMemberListURL(rctx), nil, nil)
		}
		if err != nil {
			return withRoleErrorHint(err, roleOperationMemberList)
		}
		data, err = normalizeRoleMemberListData(data, memberType)
		if err != nil {
			return err
		}
		if memberType != "" {
			fmt.Fprintf(
				rctx.IO().ErrOut,
				"warning: --member-type=%s returns only the selected member field; omitted member fields are unknown. Omit --member-type for a complete member baseline.\n",
				memberType,
			)
		}
		out := roleMemberListOutputData(rctx, data)
		rctx.OutFormat(out, nil, func(w io.Writer) {
			renderRoleMemberListPretty(w, data)
		})
		return nil
	},
}

// AppsRoleMemberAdd adds members to an app role.
var AppsRoleMemberAdd = common.Shortcut{
	Service:     appsService,
	Command:     "+role-member-add",
	Description: "Add app role members",
	Risk:        "write",
	Tips: []string{
		"Example: lark-cli apps +role-member-add --app-id <app_id> --role-id <role_id> --users ou_x",
		"Example: lark-cli apps +role-member-add --app-id <app_id> --role-id <role_id> --users ou_x,ou_y --departments od-x --chats oc_x",
		"Resolve every name first, then add all resolved users (ou_), departments (od-), and chats (oc_) in one call using the three type-specific flags; if any resolution fails, stop without a partial write",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: roleAppIDRequiredDesc, Required: true},
		{Name: "role-id", Desc: roleIDRequiredDesc, Required: true},
		{Name: "users", Desc: "comma-separated user open IDs; do not pass names or emails"},
		{Name: "departments", Desc: "comma-separated open_department_id values"},
		{Name: "chats", Desc: "comma-separated open_chat_id values"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if err := validateRoleID(rctx); err != nil {
			return err
		}
		_, err := buildRoleMemberGroups(rctx.Str("users"), rctx.Str("departments"), rctx.Str("chats"))
		return err
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		// Validate already ran and called buildRoleMemberAddBody; error is impossible here.
		body, _, _ := buildRoleMemberAddBody(rctx)
		return common.NewDryRunAPI().
			POST(roleMemberAddURL(rctx)).
			Desc("Add app role members").
			Body(body)
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		body, _, err := buildRoleMemberAddBody(rctx)
		if err != nil {
			return err
		}
		data, err := rctx.CallAPITyped("POST", roleMemberAddURL(rctx), nil, body)
		if err != nil {
			return withRoleErrorHint(err, roleOperationMemberAdd)
		}
		data, err = normalizeRoleMemberMutationData(data)
		if err != nil {
			return err
		}
		rctx.OutFormat(data, nil, func(w io.Writer) {
			renderRoleMemberMutationPretty(w, data)
		})
		return nil
	},
}

// AppsRoleMemberRemove removes members from an app role.
var AppsRoleMemberRemove = common.Shortcut{
	Service:     appsService,
	Command:     "+role-member-remove",
	Description: "Remove app role members",
	Risk:        "high-risk-write",
	Tips: []string{
		"Example: lark-cli apps +role-member-remove --app-id <app_id> --role-id <role_id> --users ou_x --yes",
		"Example: lark-cli apps +role-member-remove --app-id <app_id> --role-id <role_id> --all --yes",
		"When the user names a member, resolve and verify that exact name before writing; if lookup fails, stop and never infer that the role's only current member is the target",
		"--all clears members but does not delete the role; after a confirmed --all operation, use an unfiltered +role-member-list to verify users, departments, and chats are empty",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: roleAppIDRequiredDesc, Required: true},
		{Name: "role-id", Desc: roleIDRequiredDesc, Required: true},
		{Name: "users", Desc: "comma-separated user open IDs; do not pass names or emails"},
		{Name: "departments", Desc: "comma-separated open_department_id values"},
		{Name: "chats", Desc: "comma-separated open_chat_id values"},
		{Name: "all", Type: "bool", Desc: "remove all members from the role"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if err := validateRoleID(rctx); err != nil {
			return err
		}
		_, _, err := buildRoleMemberRemoveBody(rctx)
		return err
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		// Validate already ran and called buildRoleMemberRemoveBody; error is impossible here.
		body, _, _ := buildRoleMemberRemoveBody(rctx)
		return common.NewDryRunAPI().
			POST(roleMemberRemoveURL(rctx)).
			Desc("Remove app role members").
			Body(body)
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		body, _, err := buildRoleMemberRemoveBody(rctx)
		if err != nil {
			return err
		}
		data, err := rctx.CallAPITyped("POST", roleMemberRemoveURL(rctx), nil, body)
		if err != nil {
			return withRoleErrorHint(err, roleOperationMemberRemove)
		}
		data, err = normalizeRoleMemberMutationData(data)
		if err != nil {
			return err
		}
		rctx.OutFormat(data, nil, func(w io.Writer) {
			renderRoleMemberMutationPretty(w, data)
		})
		return nil
	},
}

// AppsRoleMatchList lists roles matching a user in an app.
var AppsRoleMatchList = common.Shortcut{
	Service:     appsService,
	Command:     "+role-match-list",
	Description: "List app roles matching a user",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +role-match-list --app-id <app_id> --user-id <user_open_id>",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: roleAppIDRequiredDesc, Required: true},
		{Name: "user-id", Desc: roleUserIDRequiredDesc, Required: true},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if err := validateRoleAppID(rctx); err != nil {
			return err
		}
		_, err := roleMatchTargetUserID(rctx)
		return err
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		// Validate already ran and called buildRoleMatchListBody; error is impossible here.
		body, _ := buildRoleMatchListBody(rctx)
		return common.NewDryRunAPI().
			POST(roleMatchListURL(rctx)).
			Desc("List app role matches").
			Body(body)
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		body, err := buildRoleMatchListBody(rctx)
		if err != nil {
			return err
		}
		data, err := rctx.CallAPITyped("POST", roleMatchListURL(rctx), nil, body)
		if err != nil {
			return withRoleErrorHint(err, roleOperationMatchList)
		}
		out, err := normalizeRoleMatchListData(data)
		if err != nil {
			return err
		}
		rctx.OutFormat(out, nil, func(w io.Writer) {
			renderRoleMatchListPretty(w, common.GetSlice(out, "roles"))
		})
		return nil
	},
}

func roleMemberListURL(rctx *common.RuntimeContext) string {
	return fmt.Sprintf(roleMemberListPath,
		validate.EncodePathSegment(roleAppID(rctx)),
		validate.EncodePathSegment(roleID(rctx)),
	)
}

func roleMemberAddURL(rctx *common.RuntimeContext) string {
	return fmt.Sprintf(roleMemberAddPath,
		validate.EncodePathSegment(roleAppID(rctx)),
		validate.EncodePathSegment(roleID(rctx)),
	)
}

func roleMemberRemoveURL(rctx *common.RuntimeContext) string {
	return fmt.Sprintf(roleMemberRemovePath,
		validate.EncodePathSegment(roleAppID(rctx)),
		validate.EncodePathSegment(roleID(rctx)),
	)
}

func roleMatchListURL(rctx *common.RuntimeContext) string {
	return fmt.Sprintf(roleMatchListPath, validate.EncodePathSegment(roleAppID(rctx)))
}

func buildRoleMemberListParams(rctx *common.RuntimeContext) (map[string]interface{}, error) {
	params := map[string]interface{}{}
	if memberType := strings.TrimSpace(rctx.Str("member-type")); memberType != "" {
		if _, ok := roleMemberKindForType(memberType); !ok {
			return nil, appsValidationParamError("--member-type", "--member-type must be one of user, department, or chat").
				WithHint("omit --member-type to list all member types")
		}
		params["member_type"] = memberType
	}
	return params, nil
}

func shouldRetryRoleMemberListWithoutFilter(err error, memberType string) bool {
	if err == nil || memberType != "chat" {
		return false
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		return false
	}
	if problem.Code == roleErrUnsupportedMemberType || problem.Code == 400004040 {
		return true
	}
	return problem.Code == 2 && strings.Contains(strings.ToLower(problem.Message), "member_type")
}

func normalizeRoleMemberListData(data map[string]interface{}, memberType string) (map[string]interface{}, error) {
	if data == nil {
		return nil, errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"role member response data must be an object",
		).WithHint("retry the complete member read; do not treat missing, null, or non-object data as an empty role")
	}
	out := map[string]interface{}{}
	for k, v := range data {
		out[k] = v
	}
	// The role service uses an exact empty data object when the requested member
	// view is empty. For a filtered request, that proves only the selected group
	// is empty; non-selected groups must remain omitted rather than being
	// synthesized as empty.
	if len(data) == 0 {
		if memberType != "" {
			kind, _ := roleMemberKindForType(memberType)
			out[kind.dataKey] = []string{}
			return out, nil
		}
		for _, kind := range roleMemberKinds {
			out[kind.dataKey] = []string{}
		}
		return out, nil
	}
	if memberType != "" {
		selectedKind, _ := roleMemberKindForType(memberType)
		values, err := parseRoleMemberIDs(data, selectedKind)
		if err != nil {
			return nil, err
		}
		for _, kind := range roleMemberKinds {
			if kind.memberType != memberType {
				delete(out, kind.dataKey)
			}
		}
		out[selectedKind.dataKey] = values
		return out, nil
	}
	for _, kind := range roleMemberKinds {
		values, err := parseRoleMemberIDs(data, kind)
		if err != nil {
			return nil, err
		}
		out[kind.dataKey] = values
	}
	return out, nil
}

func parseRoleMemberIDs(data map[string]interface{}, kind roleMemberKind) ([]string, error) {
	raw, exists := data[kind.dataKey]
	if !exists {
		return nil, errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"role member response is missing %s",
			kind.dataKey,
		).WithHint("retry the member operation; do not treat a missing member group as empty")
	}
	items, ok := raw.([]interface{})
	if !ok {
		if stringItems, stringOK := raw.([]string); stringOK {
			items = make([]interface{}, len(stringItems))
			for index, value := range stringItems {
				items[index] = value
			}
		} else {
			return nil, errs.NewInternalError(
				errs.SubtypeInvalidResponse,
				"role member response field %s must be an array of strings",
				kind.dataKey,
			).WithHint("retry the member operation; do not use malformed member data as a permission baseline")
		}
	}
	values := make([]string, 0, len(items))
	for index, item := range items {
		value, ok := item.(string)
		value = strings.TrimSpace(value)
		if !ok || value == "" || !strings.HasPrefix(value, kind.prefix) || len(value) == len(kind.prefix) {
			return nil, errs.NewInternalError(
				errs.SubtypeInvalidResponse,
				"role member response field %s contains an invalid ID at index %d",
				kind.dataKey,
				index,
			).WithHint("retry the member operation; expected open IDs with the documented member-type prefix")
		}
		values = append(values, value)
	}
	return values, nil
}

func normalizeRoleMemberMutationData(data map[string]interface{}) (map[string]interface{}, error) {
	if data == nil {
		return nil, nil
	}
	out := map[string]interface{}{}
	for key, value := range data {
		out[key] = value
	}
	for _, kind := range roleMemberKinds {
		if _, exists := data[kind.dataKey]; !exists {
			continue
		}
		values, err := parseRoleMemberIDs(data, kind)
		if err != nil {
			return nil, err
		}
		out[kind.dataKey] = values
	}
	return out, nil
}

func buildRoleMemberAddBody(rctx *common.RuntimeContext) (map[string]interface{}, roleMemberGroups, error) {
	groups, err := buildRoleMemberGroups(rctx.Str("users"), rctx.Str("departments"), rctx.Str("chats"))
	if err != nil {
		return nil, groups, err
	}
	return buildRoleMemberBody(groups), groups, nil
}

func buildRoleMemberRemoveBody(rctx *common.RuntimeContext) (map[string]interface{}, roleMemberGroups, error) {
	if rctx.Bool("all") {
		if hasExplicitRoleMemberFlags(rctx) {
			return nil, roleMemberGroups{}, appsValidationError("--all cannot be used with --users, --departments, or --chats").
				WithParams(roleMemberRemoveConflictParams(rctx)...).
				WithHint("use --all by itself to clear every member, or pass explicit member IDs without --all")
		}
		return map[string]interface{}{"all": true}, roleMemberGroups{}, nil
	}
	if !hasExplicitRoleMemberFlags(rctx) {
		reason := "provide member IDs or use --all"
		return nil, roleMemberGroups{}, appsValidationError("specify members to remove with --users/--departments/--chats, or use --all to clear every member").
			WithParams(
				appsInvalidParam("--users", reason),
				appsInvalidParam("--departments", reason),
				appsInvalidParam("--chats", reason),
				appsInvalidParam("--all", reason),
			).
			WithHint("pass specific member IDs (e.g. --users ou_x), or use --all to remove all members")
	}
	groups, err := buildRoleMemberGroups(rctx.Str("users"), rctx.Str("departments"), rctx.Str("chats"))
	if err != nil {
		return nil, groups, err
	}
	return buildRoleMemberBody(groups), groups, nil
}

func roleMemberRemoveConflictParams(rctx *common.RuntimeContext) []errs.InvalidParam {
	reason := "cannot be combined with --all"
	params := []errs.InvalidParam{appsInvalidParam("--all", "cannot be combined with explicit member flags")}
	for _, kind := range roleMemberKinds {
		if strings.TrimSpace(rctx.Str(strings.TrimPrefix(kind.flagName, "--"))) != "" {
			params = append(params, appsInvalidParam(kind.flagName, reason))
		}
	}
	return params
}

func hasExplicitRoleMemberFlags(rctx *common.RuntimeContext) bool {
	return strings.TrimSpace(rctx.Str("users")) != "" ||
		strings.TrimSpace(rctx.Str("departments")) != "" ||
		strings.TrimSpace(rctx.Str("chats")) != ""
}

func buildRoleMatchListBody(rctx *common.RuntimeContext) (map[string]interface{}, error) {
	targetUserID, err := roleMatchTargetUserID(rctx)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"target_user_id": targetUserID}, nil
}

func roleMatchTargetUserID(rctx *common.RuntimeContext) (string, error) {
	raw := strings.TrimSpace(rctx.Str("user-id"))
	if raw == "" {
		return "", appsValidationParamError("--user-id", "--user-id is required").
			WithHint("resolve the user to open_id first, then pass --user-id <open_id>")
	}
	if err := validateMemberID(raw, "--user-id"); err != nil {
		return "", err
	}
	return raw, nil
}

func roleMemberListOutputData(rctx *common.RuntimeContext, data map[string]interface{}) interface{} {
	switch rctx.Format {
	case "table", "csv", "ndjson":
		return roleMemberRows(data)
	default:
		return data
	}
}

func roleMemberRows(data map[string]interface{}) []interface{} {
	rows := []interface{}{}
	addRows := func(memberType string, values []string) {
		for _, value := range values {
			rows = append(rows, map[string]interface{}{
				"member_type": memberType,
				"member_id":   value,
			})
		}
	}
	for _, kind := range roleMemberKinds {
		addRows(kind.memberType, roleIDValues(data[kind.dataKey]))
	}
	return rows
}

func normalizeRoleMatchListData(data map[string]interface{}) (map[string]interface{}, error) {
	rawRoles, exists := data["roles"]
	roles, ok := rawRoles.([]interface{})
	if !exists || !ok {
		return nil, errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"role match response field roles must be an array",
		).WithHint("retry the user-role lookup; do not treat a missing or malformed roles field as no matches")
	}
	if err := validateRoleCollection(roles, "role match response field roles"); err != nil {
		return nil, err
	}
	out := map[string]interface{}{}
	for k, v := range data {
		out[k] = v
	}
	out["roles"] = roles
	return out, nil
}

func renderRoleMemberListPretty(w io.Writer, data map[string]interface{}) {
	renderRoleMemberGroupsPretty(w, data)
}

func renderRoleMemberGroupsPretty(w io.Writer, data map[string]interface{}) {
	for _, kind := range roleMemberKinds {
		value, exists := data[kind.dataKey]
		if !exists {
			continue
		}
		renderRoleMemberSection(w, kind.dataKey, roleIDValues(value))
	}
}

func renderRoleMemberMutationPretty(w io.Writer, data map[string]interface{}) {
	renderedGroup := false
	for _, kind := range roleMemberKinds {
		value, exists := data[kind.dataKey]
		if !exists {
			continue
		}
		renderRoleMemberSection(w, kind.dataKey, roleIDValues(value))
		renderedGroup = true
	}
	if !renderedGroup {
		fmt.Fprintln(w, "Role member update accepted; use +role-member-list to verify current members.")
	}
}

func renderRoleMemberSection(w io.Writer, label string, values []string) {
	if len(values) == 0 {
		fmt.Fprintf(w, "%s: []\n", label)
		return
	}
	fmt.Fprintf(w, "%s:\n", label)
	for _, value := range values {
		fmt.Fprintf(w, "  - %s\n", roleDisplayValue(value))
	}
}

func roleIDValues(value interface{}) []string {
	switch items := value.(type) {
	case []string:
		out := make([]string, 0, len(items))
		for _, item := range items {
			if item = strings.TrimSpace(item); item != "" {
				out = append(out, item)
			}
		}
		return out
	case []interface{}:
		out := make([]string, 0, len(items))
		for _, item := range items {
			v, ok := item.(string)
			if ok && strings.TrimSpace(v) != "" {
				out = append(out, strings.TrimSpace(v))
			}
		}
		return out
	default:
		return nil
	}
}

func renderRoleMatchListPretty(w io.Writer, items []interface{}) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ROLE ID\tNAME\tDESCRIPTION")
	for _, item := range items {
		role, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n",
			roleDisplayValue(firstNonEmpty(common.GetString(role, "role_id"), common.GetString(role, "id"))),
			roleDisplayValue(common.GetString(role, "name")),
			roleDisplayValue(common.GetString(role, "description")),
		)
	}
	_ = tw.Flush()
}
