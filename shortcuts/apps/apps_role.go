// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const maxRoleListScanPages = 1000

// AppsRoleList lists app roles.
var AppsRoleList = common.Shortcut{
	Service:     appsService,
	Command:     "+role-list",
	Description: "List app roles",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +role-list --app-id <app_id>",
		"Example: lark-cli apps +role-list --app-id <app_id> --name Admin --page-size 20",
		"When only a role name is known, pass --name for exact matching; call +role-get only after resolving one unique role_id",
		"With --name, the CLI scans server pages in batches of 100, then applies --page-size and --page-token to the exact local matches",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: roleAppIDRequiredDesc, Required: true},
		{Name: "name", Desc: "filter roles by exact name"},
		{Name: "page-size", Type: "int", Default: "20", Desc: "page size (1-100)"},
		{Name: "page-token", Desc: "integer offset returned by the previous role-list response"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if err := validateRoleAppID(rctx); err != nil {
			return err
		}
		_, err := buildRoleListParams(rctx)
		return err
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		// Validate already ran and called buildRoleListParams; error is impossible here.
		params, _ := buildRoleListParams(rctx)
		params = roleListRequestParams(params, 0)
		return common.NewDryRunAPI().
			GET(roleListURL(rctx)).
			Desc("List app roles").
			Params(params)
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		params, err := buildRoleListParams(rctx)
		if err != nil {
			return err
		}
		data, err := executeRoleList(rctx, params)
		if err != nil {
			return withRoleErrorHint(err, roleOperationList)
		}
		rctx.OutFormat(data, nil, func(w io.Writer) {
			renderRoleListPretty(w, common.GetSlice(data, "items"))
		})
		return nil
	},
}

// AppsRoleGet gets one app role.
var AppsRoleGet = common.Shortcut{
	Service:     appsService,
	Command:     "+role-get",
	Description: "Get an app role",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +role-get --app-id <app_id> --role-id <role_id>",
		"--role-id is not a human-readable role name; if only a name is known, run +role-list --name <exact_name> and use its unique returned role_id before calling +role-get",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: roleAppIDRequiredDesc, Required: true},
		{Name: "role-id", Desc: roleIDRequiredDesc, Required: true},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		return validateRoleID(rctx)
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			GET(roleItemURL(rctx)).
			Desc("Get app role")
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		data, err := rctx.CallAPITyped("GET", roleItemURL(rctx), nil, nil)
		if err != nil {
			return withRoleErrorHint(err, roleOperationGet)
		}
		role, err := parseRoleDetailResponseData(data, roleID(rctx))
		if err != nil {
			return err
		}
		rctx.OutFormat(data, nil, func(w io.Writer) {
			renderRoleGetPretty(w, role)
		})
		return nil
	},
}

// AppsRoleCreate creates an app role.
var AppsRoleCreate = common.Shortcut{
	Service:     appsService,
	Command:     "+role-create",
	Description: "Create an app role",
	Risk:        "write",
	Tips: []string{
		"Example: lark-cli apps +role-create --app-id <app_id> --name Admin",
		"Example: lark-cli apps +role-create --app-id <app_id> --name Admin --description 'Can manage orders'",
		"Example: lark-cli apps +role-create --app-id <app_id> --name Admin --role-id role_admin",
		"The create response returns data.role; run +role-get with data.role.role_id only when independent verification is required",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: roleAppIDRequiredDesc, Required: true},
		// Keep --name in Validate so the CLI can return the command-specific
		// non-invention hint instead of Cobra's generic required-flag error.
		{Name: "name", Desc: "role name (required)"},
		{Name: "description", Desc: "role description"},
		{Name: "role-id", Desc: "optional caller-provided role ID ([A-Za-z0-9_-]{1,64})"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if err := validateRoleAppID(rctx); err != nil {
			return err
		}
		if strings.TrimSpace(rctx.Str("name")) == "" {
			return appsValidationParamError("--name", "--name is required").
				WithHint("ask for the intended role name and pass it with --name; do not infer a name from --description")
		}
		if rctx.Changed("role-id") {
			roleID := strings.TrimSpace(rctx.Str("role-id"))
			if roleID == "" {
				return appsValidationParamError("--role-id", "--role-id must not be empty when provided")
			}
			return validateOptionalRoleID(roleID)
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			POST(roleListURL(rctx)).
			Desc("Create app role").
			Body(buildRoleCreateBody(rctx))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		data, err := rctx.CallAPITyped("POST", roleListURL(rctx), nil, buildRoleCreateBody(rctx))
		if err != nil {
			return withRoleErrorHint(err, roleOperationCreate)
		}
		expectedRoleID := ""
		if rctx.Changed("role-id") {
			expectedRoleID = roleID(rctx)
		}
		role, err := parseRoleWriteResponseData(data, expectedRoleID)
		if err != nil {
			return err
		}
		rctx.OutFormat(data, nil, func(w io.Writer) {
			renderRoleCreatePretty(w, role)
		})
		return nil
	},
}

// AppsRoleUpdate updates an app role.
var AppsRoleUpdate = common.Shortcut{
	Service:     appsService,
	Command:     "+role-update",
	Description: "Update an app role",
	Risk:        "write",
	Tips: []string{
		"Example: lark-cli apps +role-update --app-id <app_id> --role-id <role_id> --name Operator",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: roleAppIDRequiredDesc, Required: true},
		{Name: "role-id", Desc: roleIDRequiredDesc, Required: true},
		{Name: "name", Desc: "new role name"},
		{Name: "description", Desc: "new role description"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if err := validateRoleID(rctx); err != nil {
			return err
		}
		if rctx.Changed("name") && strings.TrimSpace(rctx.Str("name")) == "" {
			return appsValidationParamError("--name", "--name must not be empty when provided").
				WithHint("omit --name if only updating --description")
		}
		if !rctx.Changed("name") && !rctx.Changed("description") {
			reason := "provide at least one of --name or --description"
			return appsValidationError("at least one of --name or --description is required").
				WithParams(
					appsInvalidParam("--name", reason),
					appsInvalidParam("--description", reason),
				).
				WithHint("provide --name, --description, or both")
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			PATCH(roleItemURL(rctx)).
			Desc("Update app role").
			Body(buildRoleUpdateBody(rctx))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		data, err := rctx.CallAPITyped("PATCH", roleItemURL(rctx), nil, buildRoleUpdateBody(rctx))
		if err != nil {
			return withRoleErrorHint(err, roleOperationUpdate)
		}
		role, err := parseRoleWriteResponseData(data, roleID(rctx))
		if err != nil {
			return err
		}
		rctx.OutFormat(data, nil, func(w io.Writer) {
			renderRoleUpdatePretty(w, role)
		})
		return nil
	},
}

// AppsRoleDelete deletes an app role.
var AppsRoleDelete = common.Shortcut{
	Service:     appsService,
	Command:     "+role-delete",
	Description: "Delete an app role",
	Risk:        "high-risk-write",
	Tips: []string{
		"Example: lark-cli apps +role-delete --app-id <app_id> --role-id <role_id> --yes",
		"A delete request alone is not explicit confirmation: first show the exact app, role, current member scope, and irreversible impact; use --yes only after the user confirms that impact",
		"When independent verification is required, use +role-list --name <exact_name> and confirm the deleted role_id is absent; a failed +role-get alone does not prove deletion",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: roleAppIDRequiredDesc, Required: true},
		{Name: "role-id", Desc: roleIDRequiredDesc, Required: true},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		return validateRoleID(rctx)
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			DELETE(roleItemURL(rctx)).
			Desc("Delete app role")
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		data, err := rctx.CallAPITyped("DELETE", roleItemURL(rctx), nil, nil)
		if err != nil {
			return withRoleErrorHint(err, roleOperationDelete)
		}
		deletedRoleID := roleID(rctx)
		out, err := normalizeRoleDeleteData(data, deletedRoleID)
		if err != nil {
			return err
		}
		rctx.OutFormat(out, nil, func(w io.Writer) {
			renderRoleDeletePretty(w, common.GetString(out, "role_id"))
		})
		return nil
	},
}

func roleListURL(rctx *common.RuntimeContext) string {
	appID := roleAppID(rctx)
	return fmt.Sprintf(roleListPath, validate.EncodePathSegment(appID))
}

func roleItemURL(rctx *common.RuntimeContext) string {
	appID := roleAppID(rctx)
	roleID := roleID(rctx)
	return fmt.Sprintf(roleItemPath, validate.EncodePathSegment(appID), validate.EncodePathSegment(roleID))
}

func buildRoleListParams(rctx *common.RuntimeContext) (map[string]interface{}, error) {
	params, err := buildRolePageParams(rctx)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(rctx.Str("name"))
	if rctx.Changed("name") && name == "" {
		return nil, appsValidationParamError("--name", "--name must not be empty when provided").
			WithHint("omit --name to list all roles, or provide the exact role name to resolve")
	}
	if name != "" {
		params["name"] = name
	}
	return params, nil
}

// roleListRequestParams returns the query parameters for one actual backend
// request. Exact-name lookup always starts from server offset zero and scans in
// maximum-sized batches; the caller's limit/offset are applied to local matches.
func roleListRequestParams(params map[string]interface{}, page int) map[string]interface{} {
	name, _ := params["name"].(string)
	if name == "" {
		return params
	}
	return map[string]interface{}{
		"limit":  maxRolePageSize,
		"offset": page * maxRolePageSize,
		"name":   name,
	}
}

func buildRoleCreateBody(rctx *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{
		"name": strings.TrimSpace(rctx.Str("name")),
	}
	if rctx.Changed("description") {
		body["description"] = strings.TrimSpace(rctx.Str("description"))
	}
	if rctx.Changed("role-id") {
		if roleID := strings.TrimSpace(rctx.Str("role-id")); roleID != "" {
			body["role_id"] = roleID
		}
	}
	return body
}

func buildRoleUpdateBody(rctx *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{}
	if rctx.Changed("name") {
		body["name"] = strings.TrimSpace(rctx.Str("name"))
	}
	if rctx.Changed("description") {
		body["description"] = strings.TrimSpace(rctx.Str("description"))
	}
	return body
}

// executeRoleList compensates for Miaoda environments that accept the name
// query parameter but ignore it. A name lookup scans the complete server-side
// result set, applies exact matching locally, and then applies the CLI's
// offset/limit contract to the filtered result.
func executeRoleList(rctx *common.RuntimeContext, params map[string]interface{}) (map[string]interface{}, error) {
	name, _ := params["name"].(string)
	if name == "" {
		data, err := rctx.CallAPITyped("GET", roleListURL(rctx), params, nil)
		if err != nil {
			return nil, err
		}
		return normalizeRoleListData(data, params)
	}

	requestedLimit := roleIntValue(params["limit"])
	requestedOffset := roleIntValue(params["offset"])
	allMatches := make([]interface{}, 0, requestedLimit)
	var firstPage map[string]interface{}
	seenRoleIDs := map[string]struct{}{}
	seenPageSignatures := map[string]struct{}{}
	expectedTotal := -1
	scannedRoleCount := 0

	for page := 0; ; page++ {
		if page >= maxRoleListScanPages {
			return nil, errs.NewInternalError(
				errs.SubtypeInvalidResponse,
				"role list exceeded %d pages while filtering by name",
				maxRoleListScanPages,
			).WithHint("retry without --name and paginate using the returned page_token")
		}

		scanParams := roleListRequestParams(params, page)
		data, err := rctx.CallAPITyped("GET", roleListURL(rctx), scanParams, nil)
		if err != nil {
			return nil, err
		}
		if firstPage == nil {
			firstPage = data
		}
		items, hasMore, total, err := parseRoleListPage(data)
		if err != nil {
			return nil, err
		}
		if expectedTotal < 0 {
			expectedTotal = total
		} else if total != expectedTotal {
			return nil, roleListProgressError("role list total changed across pages while filtering by name")
		}
		if scannedRoleCount+len(items) > expectedTotal {
			return nil, roleListProgressError("role list returned more roles than its total while filtering by name")
		}
		scannedRoleCount += len(items)
		if hasMore && scannedRoleCount >= expectedTotal {
			return nil, roleListProgressError("role list reported more pages after reaching its total while filtering by name")
		}
		if !hasMore && scannedRoleCount != expectedTotal {
			return nil, roleListProgressError("role list ended before returning its declared total while filtering by name")
		}
		signature, newRoleCount, err := roleListPageProgress(items, seenRoleIDs)
		if err != nil {
			return nil, err
		}
		if newRoleCount != len(items) {
			return nil, roleListProgressError("role list repeated roles across pages while filtering by name")
		}
		if _, duplicate := seenPageSignatures[signature]; duplicate {
			return nil, roleListProgressError("role list repeated a page while filtering by name")
		}
		seenPageSignatures[signature] = struct{}{}
		if hasMore && (len(items) == 0 || newRoleCount == 0) {
			return nil, roleListProgressError("role list reported more pages without returning new roles")
		}
		for _, item := range items {
			role, ok := item.(map[string]interface{})
			if ok && common.GetString(role, "name") == name {
				allMatches = append(allMatches, item)
			}
		}
		if !hasMore {
			break
		}
	}

	if firstPage == nil {
		firstPage = map[string]interface{}{}
	}
	return normalizeFilteredRoleListData(firstPage, allMatches, requestedOffset, requestedLimit), nil
}

func normalizeFilteredRoleListData(data map[string]interface{}, matches []interface{}, offset, limit int) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range data {
		out[k] = v
	}

	start := offset
	if start > len(matches) {
		start = len(matches)
	}
	end := start + limit
	if end > len(matches) {
		end = len(matches)
	}
	hasMore := end < len(matches)
	items := append([]interface{}(nil), matches[start:end]...)
	if items == nil {
		items = []interface{}{}
	}
	out["items"] = items
	out["has_more"] = hasMore
	out["page_token"] = roleNextPageToken(start, limit, hasMore)
	out["total"] = len(matches)
	return out
}

func normalizeRoleListData(data map[string]interface{}, params map[string]interface{}) (map[string]interface{}, error) {
	items, hasMore, total, err := parseRoleListPage(data)
	if err != nil {
		return nil, err
	}
	out := map[string]interface{}{}
	for k, v := range data {
		out[k] = v
	}

	limit := roleIntValue(params["limit"])
	offset := roleIntValue(params["offset"])

	out["items"] = items
	out["has_more"] = hasMore
	out["page_token"] = roleNextPageToken(offset, limit, hasMore)
	out["total"] = total
	return out, nil
}

func parseRoleListPage(data map[string]interface{}) ([]interface{}, bool, int, error) {
	rawItems, hasItems := data["items"]
	items, ok := rawItems.([]interface{})
	if !hasItems || !ok {
		return nil, false, 0, errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"role list response field items must be an array",
		).WithHint("retry the read; do not treat a missing or malformed role list as empty")
	}
	if err := validateRoleCollection(items, "role list response field items"); err != nil {
		return nil, false, 0, err
	}
	rawHasMore, hasHasMore := data["has_more"]
	hasMore, ok := rawHasMore.(bool)
	if !hasHasMore || !ok {
		return nil, false, 0, errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"role list response field has_more must be a boolean",
		).WithHint("retry the read; pagination is incomplete without a valid has_more value")
	}
	total, ok := nonNegativeRoleInteger(data["total"])
	if _, exists := data["total"]; !exists || !ok {
		return nil, false, 0, errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"role list response field total must be a non-negative integer",
		).WithHint("retry the read; do not infer a role count from a missing or malformed total value")
	}
	return items, hasMore, total, nil
}

func roleListPageProgress(items []interface{}, seenRoleIDs map[string]struct{}) (string, int, error) {
	roleIDs := make([]string, 0, len(items))
	newRoleCount := 0
	for index, item := range items {
		_, roleID, err := roleCollectionItem(item, "role list response field items", index)
		if err != nil {
			return "", 0, err
		}
		roleIDs = append(roleIDs, roleID)
		if _, seen := seenRoleIDs[roleID]; !seen {
			seenRoleIDs[roleID] = struct{}{}
			newRoleCount++
		}
	}
	return strings.Join(roleIDs, "\x00"), newRoleCount, nil
}

func nonNegativeRoleInteger(value interface{}) (int, bool) {
	maxInt := uint64(^uint(0) >> 1)
	toInt := func(value int64) (int, bool) {
		if value < 0 || uint64(value) > maxInt {
			return 0, false
		}
		return int(value), true
	}

	switch value := value.(type) {
	case int:
		if value < 0 {
			return 0, false
		}
		return value, true
	case int64:
		return toInt(value)
	case float64:
		maxIntExclusive := math.Ldexp(1, strconv.IntSize-1)
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || math.Trunc(value) != value || value >= maxIntExclusive {
			return 0, false
		}
		return int(value), true
	case json.Number:
		parsed, err := value.Int64()
		if err != nil {
			return 0, false
		}
		return toInt(parsed)
	case string:
		if value == "" || strings.IndexFunc(value, func(r rune) bool {
			return r < '0' || r > '9'
		}) >= 0 {
			return 0, false
		}
		parsed, err := strconv.ParseUint(value, 10, strconv.IntSize)
		if err != nil || parsed > maxInt {
			return 0, false
		}
		return int(parsed), true
	default:
		return 0, false
	}
}

func roleListProgressError(message string) error {
	return errs.NewInternalError(errs.SubtypeInvalidResponse, message).
		WithHint("retry without --name and paginate manually; do not continue an incomplete exact-name scan")
}

func normalizeRoleDeleteData(data map[string]interface{}, requestedRoleID string) (map[string]interface{}, error) {
	if data == nil {
		return nil, invalidRoleDeleteResponse("role delete response data must be an object")
	}
	if len(data) == 0 {
		return map[string]interface{}{
			"role_id": requestedRoleID,
			"deleted": true,
		}, nil
	}

	out := map[string]interface{}{}
	for k, v := range data {
		out[k] = v
	}
	rawRoleID, ok := out["role_id"]
	if !ok {
		return nil, invalidRoleDeleteResponse("role delete response is missing role_id")
	}
	actualRoleID, stringOK := rawRoleID.(string)
	if !stringOK || actualRoleID != requestedRoleID {
		return nil, invalidRoleDeleteResponse(
			"role delete response role_id does not match requested role_id %q",
			requestedRoleID,
		)
	}
	rawDeleted, ok := out["deleted"]
	if !ok {
		return nil, invalidRoleDeleteResponse("role delete response is missing deleted")
	}
	deleted, boolOK := rawDeleted.(bool)
	if !boolOK || !deleted {
		return nil, invalidRoleDeleteResponse("role delete response did not acknowledge deletion")
	}
	return out, nil
}

type roleResponseData struct {
	RoleID      string
	Name        string
	Description string
}

func parseRoleDetailResponseData(data map[string]interface{}, expectedRoleID string) (roleResponseData, error) {
	return parseRoleResponseData(data, expectedRoleID, true)
}

func parseRoleWriteResponseData(data map[string]interface{}, expectedRoleID string) (roleResponseData, error) {
	return parseRoleResponseData(data, expectedRoleID, false)
}

func parseRoleResponseData(data map[string]interface{}, expectedRoleID string, requireName bool) (roleResponseData, error) {
	if data == nil {
		return roleResponseData{}, invalidRoleResponse("role response data must be an object")
	}
	rawRole, exists := data["role"]
	role, ok := rawRole.(map[string]interface{})
	if !exists || !ok || role == nil {
		return roleResponseData{}, invalidRoleResponse("role response field role must be an object")
	}
	rawRoleID, exists := role["role_id"]
	roleID, ok := rawRoleID.(string)
	roleID = strings.TrimSpace(roleID)
	if !exists || !ok || roleID == "" {
		return roleResponseData{}, invalidRoleResponse("role response field role.role_id must be a non-empty string")
	}
	if expectedRoleID != "" && roleID != expectedRoleID {
		return roleResponseData{}, invalidRoleResponse(
			"role response role_id %q does not match requested role_id %q",
			roleID,
			expectedRoleID,
		)
	}
	rawName, nameExists := role["name"]
	name, nameOK := rawName.(string)
	name = strings.TrimSpace(name)
	if requireName && !nameExists {
		return roleResponseData{}, invalidRoleResponse("role response field role.name must be a non-empty string")
	}
	if nameExists && (!nameOK || name == "") {
		return roleResponseData{}, invalidRoleResponse("role response field role.name must be a non-empty string")
	}
	rawDescription, descriptionExists := role["description"]
	description, descriptionOK := rawDescription.(string)
	if descriptionExists && !descriptionOK {
		return roleResponseData{}, invalidRoleResponse("role response field role.description must be a string")
	}
	return roleResponseData{RoleID: roleID, Name: name, Description: description}, nil
}

func invalidRoleResponse(message string, args ...interface{}) error {
	return errs.NewInternalError(errs.SubtypeInvalidResponse, message, args...).
		WithHint("retry the role read; do not treat a missing or malformed role as a successful result")
}

func invalidRoleDeleteResponse(message string, args ...interface{}) error {
	return errs.NewInternalError(errs.SubtypeInvalidResponse, message, args...).
		WithHint("do not claim deletion; verify the target role with +role-list --name <exact_name>")
}

func roleIntValue(value interface{}) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		i, err := strconv.Atoi(v.String())
		if err == nil {
			return i
		}
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(v))
		if err == nil {
			return i
		}
	}
	return 0
}

func renderRoleCreatePretty(w io.Writer, role roleResponseData) {
	fmt.Fprintf(w, "Created role %s\n", roleDisplayValue(role.RoleID))
}

func renderRoleGetPretty(w io.Writer, role roleResponseData) {
	renderRoleDetailPretty(w, role)
}

func renderRoleUpdatePretty(w io.Writer, role roleResponseData) {
	fmt.Fprintf(w, "Updated role %s\n", roleDisplayValue(role.RoleID))
}

func renderRoleDeletePretty(w io.Writer, roleID string) {
	fmt.Fprintf(w, "Deleted role %s\n", roleDisplayValue(roleID))
}

func renderRoleDetailPretty(w io.Writer, role roleResponseData) {
	fmt.Fprintf(w, "role_id: %s\n", roleDisplayValue(role.RoleID))
	fmt.Fprintf(w, "name: %s\n", roleDisplayValue(role.Name))
	fmt.Fprintf(w, "description: %s\n", roleDisplayValue(role.Description))
}

func renderRoleListPretty(w io.Writer, items []interface{}) {
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
			roleDisplayValue(common.GetString(role, "description")))
	}
	_ = tw.Flush()
}
