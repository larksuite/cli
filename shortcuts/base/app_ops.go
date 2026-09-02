// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// ── Shared flags ─────────────────────────────────────────────────────

func workspaceTokenFlag(required bool) common.Flag {
	return common.Flag{Name: "workspace-token", Desc: "workspace token", Required: required}
}

func appTokenFlag(required bool) common.Flag {
	return common.Flag{Name: "app-token", Desc: "BaseApp token", Required: required}
}

func pageIDFlag(required bool) common.Flag {
	return common.Flag{Name: "page-id", Desc: "BaseApp page ID", Required: required}
}

func appBlockIDFlag(required bool) common.Flag {
	return common.Flag{Name: "block-id", Desc: "BaseApp page block ID", Required: required}
}

// ── Shared helpers ───────────────────────────────────────────────────

// entityTypeValues are the entity types a workspace can hold.
var entityTypeValues = []string{"base", "baseapp"}

// normalizeEntityType lowercases and validates the workspace entity type.
func normalizeEntityType(raw string) (string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return "", nil
	}
	for _, candidate := range entityTypeValues {
		if trimmed == candidate {
			return trimmed, nil
		}
	}
	return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--type 仅支持 base|baseapp，当前值: %s", raw).WithParam("--type")
}

// appBlockBody builds the public create/update request body for app page
// blocks. Layout, position, size and display settings are intentionally not
// exposed because they are outside the public protocol.
func appBlockBody(runtime *common.RuntimeContext, includeType bool) (map[string]interface{}, error) {
	body := map[string]interface{}{}
	if name := strings.TrimSpace(runtime.Str("name")); name != "" {
		body["name"] = name
	}
	if includeType {
		if blockType := strings.TrimSpace(runtime.Str("type")); blockType != "" {
			body["type"] = blockType
		}
		if strings.EqualFold(strings.TrimSpace(runtime.Str("type")), "list") {
			rawSubType := strings.TrimSpace(runtime.Str("sub-type"))
			if subType, ok := normalizeAppListSubType(rawSubType); ok && rawSubType != "" {
				body["sub_type"] = subType
			}
		}
	}
	if raw := strings.TrimSpace(runtime.Str("data-config")); raw != "" {
		pc := newParseCtx(runtime)
		parsed, err := parseJSONObject(pc, raw, "data-config")
		if err != nil {
			return nil, err
		}
		body["data_config"] = parsed
	}
	return body, nil
}

func pagingParams(runtime *common.RuntimeContext) map[string]interface{} {
	params := map[string]interface{}{"page_size": runtime.Int("page-size")}
	if pageToken := strings.TrimSpace(runtime.Str("page-token")); pageToken != "" {
		params["page_token"] = pageToken
	}
	return params
}

// ── Workspace: dry-run ───────────────────────────────────────────────

func dryRunWorkspaceCreate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/workspaces").
		Body(workspaceCreateBody(runtime))
}

func workspaceCreateBody(runtime *common.RuntimeContext) map[string]interface{} {
	return map[string]interface{}{"name": strings.TrimSpace(runtime.Str("name"))}
}

func dryRunWorkspaceEntityList(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	params := pagingParams(runtime)
	if entityType, err := normalizeEntityType(runtime.Str("type")); err == nil && entityType != "" {
		params["entity_type"] = entityType
	}
	return common.NewDryRunAPI().
		GET("/open-apis/base/v3/workspaces/:workspace_token/entities").
		Set("workspace_token", runtime.Str("workspace-token")).
		Params(params)
}

func workspaceMoveInBody(runtime *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{}
	if token := strings.TrimSpace(runtime.Str("entity-token")); token != "" {
		body["entity_token"] = token
	}
	return body
}

func dryRunWorkspaceMoveIn(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/workspaces/:workspace_token/move_in").
		Set("workspace_token", runtime.Str("workspace-token")).
		Body(workspaceMoveInBody(runtime))
}

// ── Workspace: execute ───────────────────────────────────────────────

func executeWorkspaceCreate(runtime *common.RuntimeContext) error {
	data, err := baseV3Call(runtime, "POST", baseV3Path("workspaces"), nil, workspaceCreateBody(runtime))
	if err != nil {
		return err
	}
	out := map[string]interface{}{"workspace": data, "created": true}
	if workspaceToken := strings.TrimSpace(common.GetString(data, "workspace_token")); workspaceToken != "" {
		out["workspace_token"] = workspaceToken
	}
	if workspaceURL := strings.TrimSpace(common.GetString(data, "url")); workspaceURL != "" {
		out["url"] = workspaceURL
	}
	runtime.Out(out, nil)
	return nil
}

func executeWorkspaceEntityList(runtime *common.RuntimeContext) error {
	params := pagingParams(runtime)
	entityType, err := normalizeEntityType(runtime.Str("type"))
	if err != nil {
		return err
	}
	if entityType != "" {
		params["entity_type"] = entityType
	}
	data, err := baseV3Call(runtime, "GET", baseV3Path("workspaces", runtime.Str("workspace-token"), "entities"), params, nil)
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}

func executeWorkspaceMoveIn(runtime *common.RuntimeContext) error {
	data, err := baseV3Call(runtime, "POST", baseV3Path("workspaces", runtime.Str("workspace-token"), "move_in"), nil, workspaceMoveInBody(runtime))
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}

// ── BaseApp: dry-run ─────────────────────────────────────────────────

func baseappCreateBodyWithWorkspace(runtime *common.RuntimeContext, workspaceToken string) map[string]interface{} {
	body := map[string]interface{}{"name": strings.TrimSpace(runtime.Str("name"))}
	if workspaceToken != "" {
		body["workspace_token"] = workspaceToken
	}
	if themeStyle := strings.TrimSpace(runtime.Str("theme-style")); themeStyle != "" {
		body["theme"] = map[string]interface{}{"theme_style": themeStyle}
	}
	return body
}

func dryRunBaseappCreate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/base_apps").
		Body(baseappCreateBodyWithWorkspace(runtime, strings.TrimSpace(runtime.Str("workspace-token"))))
}

func dryRunBaseappGet(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		GET("/open-apis/base/v3/base_apps/:app_token").
		Set("app_token", runtime.Str("app-token"))
}

// ── BaseApp: execute ─────────────────────────────────────────────────

func executeBaseappCreate(runtime *common.RuntimeContext) error {
	workspaceToken := strings.TrimSpace(runtime.Str("workspace-token"))
	app, err := baseV3Call(runtime, "POST", baseV3Path("base_apps"), nil, baseappCreateBodyWithWorkspace(runtime, workspaceToken))
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"app": app, "created": true, "workspace_token": workspaceToken}, nil)
	return nil
}

func executeBaseappGet(runtime *common.RuntimeContext) error {
	data, err := baseV3Call(runtime, "GET", baseV3Path("base_apps", runtime.Str("app-token")), nil, nil)
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}

// ── Page: dry-run ────────────────────────────────────────────────────

func dryRunBaseappPageList(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		GET("/open-apis/base/v3/base_apps/:app_token/pages").
		Set("app_token", runtime.Str("app-token")).
		Params(pagingParams(runtime))
}

func dryRunBaseappPageGet(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		GET("/open-apis/base/v3/base_apps/:app_token/pages/:page_id").
		Set("app_token", runtime.Str("app-token")).
		Set("page_id", runtime.Str("page-id"))
}

func baseappPageCreateBody(runtime *common.RuntimeContext) map[string]interface{} {
	return map[string]interface{}{"name": strings.TrimSpace(runtime.Str("name"))}
}

func dryRunBaseappPageCreate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/base_apps/:app_token/pages").
		Set("app_token", runtime.Str("app-token")).
		Body(baseappPageCreateBody(runtime))
}

func dryRunBaseappPageRename(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		PATCH("/open-apis/base/v3/base_apps/:app_token/pages/:page_id").
		Set("app_token", runtime.Str("app-token")).
		Set("page_id", runtime.Str("page-id")).
		Body(map[string]interface{}{"name": strings.TrimSpace(runtime.Str("name"))})
}

func dryRunBaseappPageDelete(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		DELETE("/open-apis/base/v3/base_apps/:app_token/pages/:page_id").
		Set("app_token", runtime.Str("app-token")).
		Set("page_id", runtime.Str("page-id"))
}

// ── Page: execute ────────────────────────────────────────────────────

func executeBaseappPageList(runtime *common.RuntimeContext) error {
	data, err := baseV3Call(runtime, "GET", baseV3Path("base_apps", runtime.Str("app-token"), "pages"), pagingParams(runtime), nil)
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}

func executeBaseappPageGet(runtime *common.RuntimeContext) error {
	data, err := baseV3Call(runtime, "GET", baseV3Path("base_apps", runtime.Str("app-token"), "pages", runtime.Str("page-id")), nil, nil)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"page": data}, nil)
	return nil
}

func executeBaseappPageCreate(runtime *common.RuntimeContext) error {
	if err := ensureUniqueAppPageName(runtime, strings.TrimSpace(runtime.Str("name")), ""); err != nil {
		return err
	}
	data, err := baseV3Call(runtime, "POST", baseV3Path("base_apps", runtime.Str("app-token"), "pages"), nil, baseappPageCreateBody(runtime))
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"page": data, "created": true}, nil)
	return nil
}

func executeBaseappPageRename(runtime *common.RuntimeContext) error {
	if err := ensureUniqueAppPageName(runtime, strings.TrimSpace(runtime.Str("name")), runtime.Str("page-id")); err != nil {
		return err
	}
	body := map[string]interface{}{"name": strings.TrimSpace(runtime.Str("name"))}
	data, err := baseV3Call(runtime, "PATCH", baseV3Path("base_apps", runtime.Str("app-token"), "pages", runtime.Str("page-id")), nil, body)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"page": data, "updated": true}, nil)
	return nil
}

func ensureUniqueAppPageName(runtime *common.RuntimeContext, name, excludePageID string) error {
	pageToken := ""
	for {
		params := map[string]interface{}{"page_size": 100}
		if pageToken != "" {
			params["page_token"] = pageToken
		}
		data, err := baseV3Call(runtime, "GET", baseV3Path("base_apps", runtime.Str("app-token"), "pages"), params, nil)
		if err != nil {
			return err
		}
		for _, page := range appPageItems(data) {
			pageID := firstNonEmpty(common.GetString(page, "page_id"), common.GetString(page, "id"))
			if pageID == strings.TrimSpace(excludePageID) {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(common.GetString(page, "name")), name) {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "同一应用内 Page 名称必须唯一，已存在名为 %q 的页面", name).WithParam("--name")
			}
		}
		hasMore, _ := data["has_more"].(bool)
		pageToken = firstNonEmpty(common.GetString(data, "page_token"), common.GetString(data, "next_page_token"))
		if !hasMore || pageToken == "" {
			return nil
		}
	}
}

func appPageItems(data map[string]interface{}) []map[string]interface{} {
	for _, key := range []string{"items", "pages"} {
		raw, ok := data[key].([]interface{})
		if !ok {
			continue
		}
		items := make([]map[string]interface{}, 0, len(raw))
		for _, item := range raw {
			if page, ok := item.(map[string]interface{}); ok {
				items = append(items, page)
			}
		}
		return items
	}
	return nil
}

func executeBaseappPageDelete(runtime *common.RuntimeContext) error {
	_, err := baseV3Call(runtime, "DELETE", baseV3Path("base_apps", runtime.Str("app-token"), "pages", runtime.Str("page-id")), nil, nil)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"deleted": true, "page_id": runtime.Str("page-id")}, nil)
	return nil
}

// ── App block: dry-run ───────────────────────────────────────────────

func dryRunAppBlockList(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		GET("/open-apis/base/v3/base_apps/:app_token/pages/:page_id/blocks").
		Set("app_token", runtime.Str("app-token")).
		Set("page_id", runtime.Str("page-id")).
		Params(pagingParams(runtime))
}

func dryRunAppBlockGet(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		GET("/open-apis/base/v3/base_apps/:app_token/pages/:page_id/blocks/:block_id").
		Set("app_token", runtime.Str("app-token")).
		Set("page_id", runtime.Str("page-id")).
		Set("block_id", runtime.Str("block-id"))
}

func dryRunAppBlockCreate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	body, err := appBlockBody(runtime, true)
	if err != nil {
		body = map[string]interface{}{}
	}
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/base_apps/:app_token/pages/:page_id/blocks").
		Set("app_token", runtime.Str("app-token")).
		Set("page_id", runtime.Str("page-id")).
		Body(body)
}

func dryRunAppBlockUpdate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	body, err := appBlockBody(runtime, false)
	if err != nil {
		body = map[string]interface{}{}
	}
	return common.NewDryRunAPI().
		PATCH("/open-apis/base/v3/base_apps/:app_token/pages/:page_id/blocks/:block_id").
		Set("app_token", runtime.Str("app-token")).
		Set("page_id", runtime.Str("page-id")).
		Set("block_id", runtime.Str("block-id")).
		Body(body)
}

// ── App block: execute ───────────────────────────────────────────────

func executeAppBlockList(runtime *common.RuntimeContext) error {
	data, err := baseV3Call(runtime, "GET", baseV3Path("base_apps", runtime.Str("app-token"), "pages", runtime.Str("page-id"), "blocks"), pagingParams(runtime), nil)
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}

func executeAppBlockGet(runtime *common.RuntimeContext) error {
	data, err := baseV3Call(runtime, "GET", baseV3Path("base_apps", runtime.Str("app-token"), "pages", runtime.Str("page-id"), "blocks", runtime.Str("block-id")), nil, nil)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"block": data}, nil)
	return nil
}

func executeAppBlockCreate(runtime *common.RuntimeContext) error {
	if err := ensureUniqueAppBlockName(runtime, strings.TrimSpace(runtime.Str("name")), ""); err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(runtime.Str("type")), "list") {
		if err := validateListBaseWorkspace(runtime); err != nil {
			return err
		}
	}
	body, err := appBlockBody(runtime, true)
	if err != nil {
		return err
	}
	data, err := baseV3Call(runtime, "POST", baseV3Path("base_apps", runtime.Str("app-token"), "pages", runtime.Str("page-id"), "blocks"), nil, body)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"block": data, "created": true}, nil)
	return nil
}

func ensureUniqueAppBlockName(runtime *common.RuntimeContext, name, excludeBlockID string) error {
	pageToken := ""
	for {
		params := map[string]interface{}{"page_size": 100}
		if pageToken != "" {
			params["page_token"] = pageToken
		}
		data, err := baseV3Call(runtime, "GET", baseV3Path(
			"base_apps", runtime.Str("app-token"),
			"pages", runtime.Str("page-id"),
			"blocks",
		), params, nil)
		if err != nil {
			return err
		}
		for _, block := range appBlockItems(data) {
			blockID := firstNonEmpty(common.GetString(block, "block_id"), common.GetString(block, "id"), common.GetString(block, "widget_id"))
			if excludeBlockID != "" && strings.TrimSpace(blockID) == excludeBlockID {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(common.GetString(block, "name")), name) {
				return errs.NewValidationError(
					errs.SubtypeInvalidArgument,
					"同一 Page 内组件名称必须唯一，已存在名为 %q 的组件",
					name,
				).WithParam("--name")
			}
		}
		hasMore, _ := data["has_more"].(bool)
		pageToken = firstNonEmpty(common.GetString(data, "page_token"), common.GetString(data, "next_page_token"))
		if !hasMore || pageToken == "" {
			return nil
		}
	}
}

func appBlockItems(data map[string]interface{}) []map[string]interface{} {
	for _, key := range []string{"items", "blocks", "widgets"} {
		raw, ok := data[key].([]interface{})
		if !ok {
			continue
		}
		items := make([]map[string]interface{}, 0, len(raw))
		for _, item := range raw {
			if block, ok := item.(map[string]interface{}); ok {
				items = append(items, block)
			}
		}
		return items
	}
	return nil
}

func validateListBaseWorkspace(runtime *common.RuntimeContext) error {
	raw := strings.TrimSpace(runtime.Str("data-config"))
	if raw == "" {
		return nil
	}
	cfg, err := parseJSONObject(newParseCtx(runtime), raw, "data-config")
	if err != nil {
		return err
	}
	baseToken := strings.TrimSpace(common.GetString(cfg, "base_token"))
	if baseToken == "" {
		return nil
	}
	app, err := baseV3Call(runtime, "GET", baseV3Path("base_apps", runtime.Str("app-token")), nil, nil)
	if err != nil {
		return err
	}
	if appRefContainsBase(app["ref"], baseToken) {
		return nil
	}
	workspaceToken := firstNonEmpty(
		strings.TrimSpace(common.GetString(app, "workspace_token")),
		strings.TrimSpace(common.GetString(app, "workspace_id")),
	)
	if workspaceToken == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "无法确认列表 Base 与 App 是否位于同一 Workspace：App 响应缺少 workspace_token").WithParam("--data-config")
	}
	pageToken := ""
	for {
		params := map[string]interface{}{"page_size": 100, "entity_type": "base"}
		if pageToken != "" {
			params["page_token"] = pageToken
		}
		entities, err := baseV3Call(runtime, "GET", baseV3Path("workspaces", workspaceToken, "entities"), params, nil)
		if err != nil {
			return err
		}
		if workspaceContainsBase(entities, baseToken) {
			return nil
		}
		hasMore, _ := entities["has_more"].(bool)
		pageToken = firstNonEmpty(common.GetString(entities, "page_token"), common.GetString(entities, "next_page_token"))
		if !hasMore || pageToken == "" {
			break
		}
	}
	return errs.NewValidationError(errs.SubtypeInvalidArgument, "列表组件只能选择 App 所在 Workspace 内的一个 Base；%s 不在当前 Workspace", baseToken).WithParam("--data-config")
}

func appRefContainsBase(raw interface{}, baseToken string) bool {
	switch refs := raw.(type) {
	case map[string]interface{}:
		_, ok := refs[baseToken]
		return ok
	case map[string][]string:
		_, ok := refs[baseToken]
		return ok
	default:
		return false
	}
}

func workspaceContainsBase(data map[string]interface{}, baseToken string) bool {
	for _, key := range []string{"items", "entities"} {
		items, _ := data[key].([]interface{})
		for _, raw := range items {
			entity, _ := raw.(map[string]interface{})
			token := firstNonEmpty(common.GetString(entity, "token"), common.GetString(entity, "entity_token"))
			if strings.TrimSpace(token) == baseToken {
				return true
			}
		}
	}
	return false
}

func executeAppBlockUpdate(runtime *common.RuntimeContext) error {
	blockID := strings.TrimSpace(runtime.Str("block-id"))
	if name := strings.TrimSpace(runtime.Str("name")); name != "" {
		if err := ensureUniqueAppBlockName(runtime, name, blockID); err != nil {
			return err
		}
	}
	if strings.TrimSpace(runtime.Str("data-config")) != "" {
		current, err := baseV3Call(runtime, "GET", baseV3Path("base_apps", runtime.Str("app-token"), "pages", runtime.Str("page-id"), "blocks", blockID), nil, nil)
		if err != nil {
			return err
		}
		if err := validateAppBlockUpdateForCurrentBlock(runtime, current); err != nil {
			return err
		}
		blockType := firstNonEmpty(common.GetString(current, "type"), common.GetString(current, "block_type"))
		if strings.EqualFold(strings.TrimSpace(blockType), "list") {
			if err := validateListBaseWorkspace(runtime); err != nil {
				return err
			}
		}
	}
	body, err := appBlockBody(runtime, false)
	if err != nil {
		return err
	}
	data, err := baseV3Call(runtime, "PATCH", baseV3Path("base_apps", runtime.Str("app-token"), "pages", runtime.Str("page-id"), "blocks", blockID), nil, body)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"block": data, "updated": true}, nil)
	return nil
}

func validateAppBlockUpdateForCurrentBlock(runtime *common.RuntimeContext, current map[string]interface{}) error {
	if runtime.Bool("no-validate") {
		return nil
	}
	patch, err := parseJSONObject(newParseCtx(runtime), strings.TrimSpace(runtime.Str("data-config")), "data-config")
	if err != nil {
		return err
	}
	currentConfig := common.GetMap(current, "data_config")
	merged := cloneMap(currentConfig)
	if merged == nil {
		merged = map[string]interface{}{}
	}
	for key, value := range patch {
		merged[key] = value
	}

	blockType := strings.TrimSpace(firstNonEmpty(common.GetString(current, "type"), common.GetString(current, "block_type")))
	var problems []string
	switch {
	case strings.EqualFold(blockType, "list"):
		subType, ok := normalizeAppListSubType(firstNonEmpty(common.GetString(current, "sub_type"), common.GetString(current, "subType")))
		if !ok {
			return errs.NewValidationError(errs.SubtypeFailedPrecondition, "当前列表组件 sub_type 不受 CLI 支持: %s", subType).WithParam("--data-config")
		}
		problems = validateAppListDataConfig(subType, merged)
	case isTextBlockType(blockType):
		problems = validateAppBlockDataConfig(blockType, merged)
	case isChartBlockType(blockType):
		problems = validateAppBlockDataConfig(blockType, normalizeAppChartDataConfig(merged))
	default:
		return errs.NewValidationError(errs.SubtypeFailedPrecondition, "当前组件类型 %q 不支持通过 CLI 更新 data_config", blockType).WithParam("--data-config")
	}
	return formatDataConfigErrors(problems)
}
