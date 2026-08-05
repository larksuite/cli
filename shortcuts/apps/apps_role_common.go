// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	roleListPath         = apiBasePath + "/apps/%s/roles"
	roleItemPath         = apiBasePath + "/apps/%s/roles/%s"
	roleMemberListPath   = apiBasePath + "/apps/%s/roles/%s/member_list"
	roleMemberAddPath    = apiBasePath + "/apps/%s/roles/%s/member_add"
	roleMemberRemovePath = apiBasePath + "/apps/%s/roles/%s/member_remove"
	roleMatchListPath    = apiBasePath + "/apps/%s/user_role_list"
	defaultRolePageSize  = 20
	maxRolePageSize      = 100
	maxRoleMembers       = 100

	roleErrInvalidParameters       = 3340001
	roleErrUserLimitExceeded       = 3344027
	roleErrDepartmentLimitExceeded = 3344028
	roleErrChatLimitExceeded       = 3344029
	roleErrAdminRequired           = 3344030
	roleErrManagerRequired         = 3344031
	roleErrInvalidRoleID           = 3344034
	roleErrRoleNotFound            = 3344035
	roleErrRoleAlreadyExists       = 3344036
	roleErrRoleLimitExceeded       = 3344037
	roleErrInvalidRoleName         = 3344038
	roleErrInvalidRoleDescription  = 3344039
	roleErrUnsupportedMemberType   = 3344040
	roleErrInvalidMemberID         = 3344041
)

var optionalRoleIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

const (
	roleAppHint    = "verify --app-id is a Miaoda app_id you can access; list apps with `lark-cli apps +list`"
	roleItemHint   = "verify --role-id belongs to the app; if you only know a role name, resolve it with `lark-cli apps +role-list --app-id <app_id> --name <exact_name>` and use the unique returned role_id"
	roleCreateHint = "verify --app-id and role fields; omit --role-id unless you need a caller-provided role ID"
	roleMemberHint = "verify --role-id and member IDs; use user open_id, open_department_id, or open_chat_id values"
	roleMatchHint  = "use --user-id with a user open_id; do not pass role_id or enumerate roles manually"

	roleAppIDRequiredDesc  = "Miaoda app ID (required; app_...; use apps +list to find it)"
	roleIDRequiredDesc     = "role ID (required; [A-Za-z0-9_-]{1,64}; use role-list to find it)"
	roleUserIDRequiredDesc = "user open ID (required; ou_...; do not pass a role ID, name, or email)"
)

type roleErrorOperation uint8

const (
	roleOperationList roleErrorOperation = iota
	roleOperationGet
	roleOperationCreate
	roleOperationUpdate
	roleOperationDelete
	roleOperationMemberList
	roleOperationMemberAdd
	roleOperationMemberRemove
	roleOperationMatchList
)

type roleMemberGroups struct {
	Users       []string `json:"users"`
	Departments []string `json:"departments"`
	Chats       []string `json:"chats"`
}

type roleMemberKind struct {
	memberType string
	dataKey    string
	flagName   string
	prefix     string
}

var roleMemberKinds = []roleMemberKind{
	{memberType: "user", dataKey: "users", flagName: "--users", prefix: "ou_"},
	{memberType: "department", dataKey: "departments", flagName: "--departments", prefix: "od-"},
	{memberType: "chat", dataKey: "chats", flagName: "--chats", prefix: "oc_"},
}

func roleAppID(rctx *common.RuntimeContext) string {
	return strings.TrimSpace(rctx.Str("app-id"))
}

func roleID(rctx *common.RuntimeContext) string {
	return strings.TrimSpace(rctx.Str("role-id"))
}

func validateRoleAppID(rctx *common.RuntimeContext) error {
	appID := roleAppID(rctx)
	if appID == "" {
		return appsValidationParamError("--app-id", "--app-id is required").
			WithHint("list your apps with `lark-cli apps +list`")
	}
	if strings.HasPrefix(appID, "cli_") {
		return appsValidationParamError("--app-id", "--app-id must be a Miaoda app_id, not a Lark app_id").
			WithHint("pass the app_... value from `lark-cli apps +list`, not the cli_... credential app id")
	}
	if !strings.HasPrefix(appID, "app_") || len(appID) == len("app_") {
		return appsValidationParamError("--app-id", "--app-id must be a Miaoda app_id starting with app_").
			WithHint("list Miaoda apps with `lark-cli apps +list`, then pass the returned app_id")
	}
	// app-id must not contain forward slashes (apps are identified by app_xxx IDs).
	for _, r := range appID {
		if r == '/' || r == '\\' || unicode.IsSpace(r) || unicode.IsControl(r) {
			return appsValidationParamError("--app-id", "--app-id must not contain slashes, whitespace, or control characters")
		}
	}
	// Defense-in-depth: block path traversal and URL metacharacters.
	if err := validateRolePathSegmentSafe(appID, "--app-id"); err != nil {
		return err
	}
	return nil
}

func validateRoleID(rctx *common.RuntimeContext) error {
	if err := validateRoleAppID(rctx); err != nil {
		return err
	}
	roleID := roleID(rctx)
	if roleID == "" {
		return appsValidationParamError("--role-id", "--role-id is required").
			WithHint("list roles with `lark-cli apps +role-list --app-id <app_id>`")
	}
	return validateExistingRoleIDValue(roleID)
}

// validateRolePathSegmentSafe rejects path-traversal segments ("..") and URL
// metacharacters (? # %) in values interpolated into a URL path, providing
// defense-in-depth alongside validate.EncodePathSegment.
func validateRolePathSegmentSafe(value, flagName string) error {
	for _, seg := range strings.Split(value, "/") {
		if seg == ".." {
			return appsValidationParamError(flagName, "%s must not contain '..' path traversal", flagName).
				WithHint("provide a valid %s without path traversal", flagName)
		}
	}
	if strings.ContainsAny(value, "?#%") {
		return appsValidationParamError(flagName, "%s contains invalid URL characters (?, #, %%)", flagName).
			WithHint("provide a valid %s without URL metacharacters", flagName)
	}
	return nil
}

func validateOptionalRoleID(roleID string) error {
	roleID = strings.TrimSpace(roleID)
	if roleID == "" {
		return nil
	}
	return validateCreateRoleIDValue(roleID)
}

func validateCreateRoleIDValue(roleID string) error {
	if !optionalRoleIDPattern.MatchString(roleID) {
		return appsValidationParamError("--role-id", "--role-id must match [A-Za-z0-9_-]{1,64}").
			WithHint("omit --role-id to let the server generate one")
	}
	return nil
}

func validateExistingRoleIDValue(roleID string) error {
	if !optionalRoleIDPattern.MatchString(roleID) {
		return appsValidationParamError("--role-id", "--role-id must match [A-Za-z0-9_-]{1,64}").
			WithHint("resolve the role with `lark-cli apps +role-list --app-id <app_id> --name <exact_name>` and pass its role_id")
	}
	return nil
}

func buildRolePageParams(rctx *common.RuntimeContext) (map[string]interface{}, error) {
	limit := defaultRolePageSize
	if rctx.Changed("page-size") {
		limit = rctx.Int("page-size")
	}
	if limit < 1 || limit > maxRolePageSize {
		return nil, appsValidationParamError("--page-size", "--page-size must be between 1 and %d", maxRolePageSize).
			WithHint("use --page-size between 1 and 100")
	}

	offset := 0
	pageToken := strings.TrimSpace(rctx.Str("page-token"))
	if pageToken != "" {
		parsedOffset, err := strconv.Atoi(pageToken)
		if err != nil || parsedOffset < 0 {
			return nil, appsValidationParamError("--page-token", "--page-token must be a non-negative integer offset").
				WithHint("reuse page_token from the previous +role-list response")
		}
		offset = parsedOffset
	}

	return map[string]interface{}{
		"limit":  limit,
		"offset": offset,
	}, nil
}

func roleNextPageToken(offset, limit int, hasMore bool) string {
	if !hasMore {
		return ""
	}
	return strconv.Itoa(offset + limit)
}

func splitRoleMemberCSV(s, flagName string) ([]string, error) {
	parts := strings.Split(s, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		// Reject values containing whitespace, control characters, or URL metacharacters
		// (member IDs are open_id/open_department_id/open_chat_id which are safe tokens).
		if err := validateMemberID(value, flagName); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

// validateMemberID rejects values containing characters that are invalid in
// open_id / open_department_id / open_chat_id tokens (whitespace, controls, URL metacharacters).
func validateMemberID(value, flagName string) error {
	if err := validateMemberIDPrefix(value, flagName); err != nil {
		return err
	}
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return appsValidationParamError(flagName, "member IDs must not contain whitespace or control characters").
				WithHint("pass comma-separated open_id/open_department_id/open_chat_id values without spaces")
		}
		if r == '?' || r == '#' || r == '%' || r == '/' || r == '\\' {
			return appsValidationParamError(flagName, "member IDs must not contain URL metacharacters (?, #, %, /, \\)").
				WithHint("pass comma-separated open_id/open_department_id/open_chat_id values without URL characters")
		}
	}
	return nil
}

func validateMemberIDPrefix(value, flagName string) error {
	kind, ok := roleMemberKindForFlag(flagName)
	if !ok {
		return nil
	}
	if !strings.HasPrefix(value, kind.prefix) || len(value) == len(kind.prefix) {
		return appsValidationParamError(flagName, "%s must use %s IDs", flagName, kind.prefix).
			WithHint("resolve names or emails to open IDs before calling role member commands")
	}
	return nil
}

func roleMemberKindForFlag(flagName string) (roleMemberKind, bool) {
	if flagName == "--user-id" {
		flagName = "--users"
	}
	for _, kind := range roleMemberKinds {
		if kind.flagName == flagName {
			return kind, true
		}
	}
	return roleMemberKind{}, false
}

func roleMemberKindForType(memberType string) (roleMemberKind, bool) {
	for _, kind := range roleMemberKinds {
		if kind.memberType == memberType {
			return kind, true
		}
	}
	return roleMemberKind{}, false
}

func roleDisplayValue(value string) string {
	value = validate.SanitizeForTerminal(value)
	value = strings.NewReplacer("\n", " ", "\t", " ").Replace(value)
	return strings.TrimSpace(value)
}

// withRoleErrorHint refines documented Spark role errors with command-specific
// recovery while preserving the typed error, numeric code, log_id, and any
// server-provided detail. Unknown codes retain the existing Apps fallback.
func withRoleErrorHint(err error, operation roleErrorOperation) error {
	if err == nil {
		return nil
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		return err
	}
	hint := roleErrorHint(problem.Code, operation)
	if hint == "" {
		return withAppsHint(err, roleFallbackHint(operation))
	}

	existing := strings.TrimSpace(problem.Hint)
	canonicalAPIHint := strings.TrimSpace(errclass.APIHint(problem.Subtype))
	switch {
	case existing == "", existing == canonicalAPIHint:
		problem.Hint = hint
	case !strings.Contains(existing, hint):
		problem.Hint = existing + "; " + hint
	}
	return err
}

func roleFallbackHint(operation roleErrorOperation) string {
	switch operation {
	case roleOperationList:
		return roleAppHint
	case roleOperationCreate:
		return roleCreateHint
	case roleOperationMemberList, roleOperationMemberAdd, roleOperationMemberRemove:
		return roleMemberHint
	case roleOperationMatchList:
		return roleMatchHint
	default:
		return roleItemHint
	}
}

func roleErrorHint(code int, operation roleErrorOperation) string {
	switch code {
	case roleErrInvalidParameters:
		return roleFallbackHint(operation)
	case roleErrAdminRequired:
		return "ask an app administrator to perform this operation or grant the calling user app-administrator access"
	case roleErrManagerRequired:
		return "ask an app administrator or app developer to perform this operation, or grant the calling user app-management access"
	case roleErrInvalidRoleID:
		if operation == roleOperationCreate {
			return "omit --role-id to let the server generate one, or provide a role ID accepted by the role service"
		}
	case roleErrRoleNotFound:
		if operation == roleOperationMatchList {
			return "list the app's current roles and retry; role data used for this match may no longer be valid"
		}
		return roleItemHint
	case roleErrRoleAlreadyExists:
		if operation == roleOperationCreate {
			return "choose a different --role-id or omit --role-id to let the server generate one"
		}
	case roleErrRoleLimitExceeded:
		if operation == roleOperationCreate {
			return "delete an unused app role before creating another role"
		}
	case roleErrInvalidRoleName:
		if operation == roleOperationCreate || operation == roleOperationUpdate {
			return "adjust --name to a non-empty value accepted by the role service"
		}
	case roleErrInvalidRoleDescription:
		if operation == roleOperationCreate || operation == roleOperationUpdate {
			return "adjust --description to a value accepted by the role service"
		}
	case roleErrUnsupportedMemberType:
		if operation == roleOperationMemberList {
			return "use --member-type user, department, or chat, or omit --member-type to list all member types"
		}
	case roleErrInvalidMemberID:
		if operation == roleOperationMatchList {
			return "resolve the target user to an open_id and retry with --user-id <open_id>"
		}
		if operation == roleOperationMemberAdd || operation == roleOperationMemberRemove {
			return roleMemberHint
		}
	case roleErrUserLimitExceeded:
		if operation == roleOperationMemberAdd {
			return "reduce the users being added with --users, or remove unused user members before retrying"
		}
	case roleErrDepartmentLimitExceeded:
		if operation == roleOperationMemberAdd {
			return "reduce the departments being added with --departments, or remove unused department members before retrying"
		}
	case roleErrChatLimitExceeded:
		if operation == roleOperationMemberAdd {
			return "reduce the chats being added with --chats, or remove unused chat members before retrying"
		}
	}
	return ""
}

func roleCollectionItem(item interface{}, collection string, index int) (map[string]interface{}, string, error) {
	role, ok := item.(map[string]interface{})
	if !ok {
		return nil, "", invalidRoleCollectionResponse("%s item %d must be an object", collection, index)
	}
	rawRoleID, exists := role["role_id"]
	roleID, stringOK := rawRoleID.(string)
	roleID = strings.TrimSpace(roleID)
	if !exists || !stringOK || roleID == "" {
		return nil, "", invalidRoleCollectionResponse("%s item %d must contain a non-empty string role_id", collection, index)
	}
	rawName, exists := role["name"]
	name, stringOK := rawName.(string)
	if !exists || !stringOK || strings.TrimSpace(name) == "" {
		return nil, "", invalidRoleCollectionResponse("%s item %d must contain a non-empty string name", collection, index)
	}
	return role, roleID, nil
}

func validateRoleCollection(items []interface{}, collection string) error {
	for index, item := range items {
		if _, _, err := roleCollectionItem(item, collection, index); err != nil {
			return err
		}
	}
	return nil
}

func invalidRoleCollectionResponse(format string, args ...interface{}) error {
	return errs.NewInternalError(errs.SubtypeInvalidResponse, format, args...).
		WithHint("retry the read; do not treat missing or malformed role data as an empty or complete result")
}

func buildRoleMemberGroups(usersCSV, departmentsCSV, chatsCSV string) (roleMemberGroups, error) {
	users, err := splitRoleMemberCSV(usersCSV, "--users")
	if err != nil {
		return roleMemberGroups{}, err
	}
	departments, err := splitRoleMemberCSV(departmentsCSV, "--departments")
	if err != nil {
		return roleMemberGroups{}, err
	}
	chats, err := splitRoleMemberCSV(chatsCSV, "--chats")
	if err != nil {
		return roleMemberGroups{}, err
	}
	groups := roleMemberGroups{
		Users:       users,
		Departments: departments,
		Chats:       chats,
	}
	total := len(groups.Users) + len(groups.Departments) + len(groups.Chats)
	if total == 0 {
		reason := "provide at least one of --users, --departments, or --chats"
		return groups, appsValidationError("at least one of --users, --departments, or --chats is required").
			WithParams(
				appsInvalidParam("--users", reason),
				appsInvalidParam("--departments", reason),
				appsInvalidParam("--chats", reason),
			).
			WithHint("resolve names to IDs first, then pass --users open_id, --departments open_department_id, or --chats open_chat_id")
	}
	if total > maxRoleMembers {
		return groups, appsValidationError("role members cannot exceed %d", maxRoleMembers).
			WithParams(roleMemberLimitParams(groups)...).
			WithHint(fmt.Sprintf("reduce the atomic request to at most %d members; the CLI does not split member writes automatically", maxRoleMembers))
	}
	return groups, nil
}

func buildRoleMemberBody(groups roleMemberGroups) map[string]interface{} {
	body := map[string]interface{}{}
	if len(groups.Users) > 0 {
		body["users"] = groups.Users
	}
	if len(groups.Departments) > 0 {
		body["departments"] = groups.Departments
	}
	if len(groups.Chats) > 0 {
		body["chats"] = groups.Chats
	}
	return body
}

func roleMemberLimitParams(groups roleMemberGroups) []errs.InvalidParam {
	reason := fmt.Sprintf("combined role member count exceeds %d", maxRoleMembers)
	params := make([]errs.InvalidParam, 0, len(roleMemberKinds))
	if len(groups.Users) > 0 {
		params = append(params, appsInvalidParam("--users", reason))
	}
	if len(groups.Departments) > 0 {
		params = append(params, appsInvalidParam("--departments", reason))
	}
	if len(groups.Chats) > 0 {
		params = append(params, appsInvalidParam("--chats", reason))
	}
	return params
}
