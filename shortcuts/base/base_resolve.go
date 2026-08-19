// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	baseURLResolveHintGeneric = "Provide a /base/, /app/, /wiki/, or /record/ URL, or use base +title-resolve --title if you only know the Base title."
	baseTitleResolveHint      = "choose one candidate, then use +base-block-list to list tables, dashboards, workflows, and other Base blocks"
	nextStepBaseBlockList     = "use +base-block-list to list tables, dashboards, workflows, and other Base blocks"
	nextStepRecordList        = "use +record-list to list records in the resolved table"
	nextStepBaseApp           = "use +app-get with app_token; there is no +app-list, so list apps in a workspace with +workspace-entity-list --type baseapp"
	titleResolveQueryMaxLen   = 30
)

var BaseURLResolve = common.Shortcut{
	Service:     "base",
	Command:     "+url-resolve",
	Description: "Resolve a Base or BaseApp URL into usable coordinates",
	Risk:        "read",
	Scopes:      []string{},
	ConditionalScopes: []string{
		"base:block:read",
		"base:field:read",
		"base:record:read",
		"wiki:node:retrieve",
	},
	AuthTypes: authTypes(),
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "url", Aliases: []string{"query"}, Desc: "Base/BaseApp/Wiki/record-share URL to resolve"},
	},
	Tips: []string{
		`Example: lark-cli base +url-resolve --url "https://example.larkoffice.com/base/<base_token>?table=<table_id>&view=<view_id>"`,
		`BaseApp example: lark-cli base +url-resolve --url "https://example.larkoffice.com/app/<app_token>?pre_pathname=/base/workspace/<workspace_token>&pageId=<page_id>"`,
		"Only URLs are accepted. For Base titles or keywords, use +title-resolve --title.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := readURLResolveInput(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		raw, err := readURLResolveInput(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		parsed, err := parseResolveURL(raw)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		switch classifyBaseURL(parsed) {
		case "base_url":
			baseToken := firstPathSegmentAfter(parsed.Path, "/base/")
			if selectedBlockID := strings.TrimSpace(parsed.Query().Get("table")); selectedBlockID != "" {
				return common.NewDryRunAPI().
					POST("/open-apis/base/v3/bases/:base_token/blocks/list").
					Body(map[string]interface{}{}).
					Set("base_token", baseToken).
					Set("selected_block_id", selectedBlockID)
			}
			return common.NewDryRunAPI().Set("url", raw).Set("resolution", "local")
		case "wiki_url":
			dry := common.NewDryRunAPI()
			selectedBlockID := strings.TrimSpace(parsed.Query().Get("table"))
			if selectedBlockID == "" {
				return dry.
					GET("/open-apis/wiki/v2/spaces/get_node").
					Params(map[string]interface{}{"token": firstPathSegmentAfter(parsed.Path, "/wiki/")})
			}
			dry.Desc("2-step: resolve the Wiki node to a Base, then identify the selected Base block")
			dry.GET("/open-apis/wiki/v2/spaces/get_node").
				Desc("[1] Resolve the Wiki node to its underlying Base").
				Params(map[string]interface{}{"token": firstPathSegmentAfter(parsed.Path, "/wiki/")})
			dry.POST("/open-apis/base/v3/bases/:base_token/blocks/list").
				Desc("[2] List Base blocks and match selected_block_id").
				Body(map[string]interface{}{})
			return dry.
				Set("base_token", "<obj_token from step 1>").
				Set("selected_block_id", selectedBlockID)
		case "record_share_url":
			return common.NewDryRunAPI().
				GET("/open-apis/base/v3/record_share/:record_share_token/meta").
				Set("record_share_token", firstPathSegmentAfter(parsed.Path, "/record/"))
		default:
			return common.NewDryRunAPI().Set("url", raw).Set("resolution", "local")
		}
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeBaseURLResolve(runtime)
	},
}

var BaseTitleResolve = common.Shortcut{
	Service:     "base",
	Command:     "+title-resolve",
	Description: "Resolve a Base title or keyword through Drive search",
	Risk:        "read",
	Scopes:      []string{"search:docs:read"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "title", Aliases: []string{"query", "url"}, Desc: "Base title keyword to search via Drive (30 characters or fewer)"},
	},
	Tips: []string{
		`Example: lark-cli base +title-resolve --title "Sales pipeline"`,
		"Pass a short keyword from the Base title, 30 characters or fewer. Use +url-resolve for URLs.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := readTitleResolveQuery(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		query, err := readTitleResolveQuery(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return common.NewDryRunAPI().
			POST("/open-apis/search/v2/doc_wiki/search").
			Body(buildTitleResolveSearchBody(query))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeBaseTitleResolve(runtime)
	},
}

func readURLResolveInput(runtime *common.RuntimeContext) (string, error) {
	value := strings.TrimSpace(runtime.Str("url"))
	if value == "" {
		return "", baseFlagErrorf("specify --url")
	}
	return value, nil
}

func readTitleResolveQuery(runtime *common.RuntimeContext) (string, error) {
	pickedValue := strings.TrimSpace(runtime.Str("title"))
	if pickedValue == "" {
		return "", baseFlagErrorf("specify --title")
	}
	if len([]rune(pickedValue)) > titleResolveQueryMaxLen {
		return "", resolveValidationError(
			fmt.Sprintf("base +title-resolve title keyword must be %d characters or fewer.", titleResolveQueryMaxLen),
			"Use a shorter keyword from the Base title, or provide a /base/ URL and use base +url-resolve.",
		)
	}
	return pickedValue, nil
}

func executeBaseURLResolve(runtime *common.RuntimeContext) error {
	raw, err := readURLResolveInput(runtime)
	if err != nil {
		return err
	}
	parsed, err := parseResolveURL(raw)
	if err != nil {
		return err
	}

	switch classifyBaseURL(parsed) {
	case "baseapp_url":
		runtime.OutFormat(resolveBaseAppURL(parsed), nil, nil)
		return nil
	case "base_url":
		out := resolveBaseURL(parsed)
		enrichBaseResolveHint(runtime, out, resolveBaseURLSelection(parsed))
		runtime.OutFormat(out, nil, nil)
		return nil
	case "wiki_url":
		out, err := resolveWikiBaseURL(runtime, parsed)
		if err != nil {
			return err
		}
		selection := resolveBaseURLSelection(parsed)
		applyBaseURLSelection(out, selection)
		enrichBaseResolveHint(runtime, out, selection)
		runtime.OutFormat(out, nil, nil)
		return nil
	case "record_share_url":
		out, err := resolveRecordShareURL(runtime, parsed)
		if err != nil {
			return err
		}
		runtime.OutFormat(out, nil, nil)
		return nil
	case "form_share_url":
		runtime.OutFormat(resolveFormShareURL(parsed), nil, nil)
		return nil
	case "view_share_url":
		return resolveValidationError(
			"This is a Base view share URL. CLI does not support resolving Base view share URLs.",
			"Open it in the browser, or provide the URL of the Base itself, such as its Wiki URL or Base URL.",
		)
	case "dashboard_share_url":
		return resolveValidationError(
			"This is a Base dashboard share URL. CLI does not support resolving Base dashboard share URLs.",
			"Open it in the browser, or provide the URL of the Base itself, such as its Wiki URL or Base URL.",
		)
	case "workspace_url":
		return resolveValidationError(
			"This is a Base workspace URL. CLI does not support resolving Base workspace URLs.",
			"Open it in the browser, or provide the URL of the Base itself, such as its Wiki URL or Base URL.",
		)
	case "add_record_url":
		return resolveValidationError(
			"This is a Base add-record URL. CLI does not support resolving Base add-record URLs.",
			"Open it in the browser, or provide the URL of the Base itself, such as its Wiki URL or Base URL.",
		)
	default:
		return resolveValidationError("This URL is not a supported Base URL pattern.", baseURLResolveHintGeneric)
	}
}

func parseResolveURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, resolveValidationError("base +url-resolve only accepts full URLs.", "For a Base title or keyword, use base +title-resolve --title.")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, resolveValidationError("base +url-resolve only accepts HTTP or HTTPS URLs.", baseURLResolveHintGeneric)
	}
	return parsed, nil
}

func classifyBaseURL(u *url.URL) string {
	path := normalizeResolvePath(u.Path)
	switch {
	case pathSegmentExists(path, "/app/"):
		return "baseapp_url"
	case pathSegmentExists(path, "/base/workspace/"):
		return "workspace_url"
	case pathSegmentExists(path, "/base/add/"):
		return "add_record_url"
	case pathSegmentExists(path, "/base/"):
		return "base_url"
	case pathSegmentExists(path, "/wiki/"):
		return "wiki_url"
	case pathSegmentExists(path, "/record/"):
		return "record_share_url"
	case pathSegmentExists(path, "/share/base/form/"):
		return "form_share_url"
	case pathSegmentExists(path, "/share/base/view/"):
		return "view_share_url"
	case pathSegmentExists(path, "/share/base/dashboard/"):
		return "dashboard_share_url"
	default:
		return ""
	}
}

func resolveBaseAppURL(u *url.URL) map[string]interface{} {
	query := u.Query()
	out := map[string]interface{}{
		"input_type":    "baseapp_url",
		"resource_type": "baseapp",
		"app_token":     firstPathSegmentAfter(u.Path, "/app/"),
		"hint": map[string]interface{}{
			"next_step": nextStepBaseApp,
		},
	}
	if pageID := strings.TrimSpace(query.Get("pageId")); pageID != "" {
		out["page_id"] = pageID
	}
	if workspaceToken := firstPathSegmentAfter(query.Get("pre_pathname"), "/base/workspace/"); workspaceToken != "" {
		out["workspace_token"] = workspaceToken
	}
	return out
}

func resolveBaseURL(u *url.URL) map[string]interface{} {
	out := map[string]interface{}{
		"input_type":    "base_url",
		"resource_type": "bitable",
		"base_token":    firstPathSegmentAfter(u.Path, "/base/"),
	}
	applyBaseURLSelection(out, resolveBaseURLSelection(u))
	return out
}

type baseURLSelection struct {
	blockID  string
	viewID   string
	recordID string
}

func resolveBaseURLSelection(u *url.URL) baseURLSelection {
	query := u.Query()
	return baseURLSelection{
		blockID:  strings.TrimSpace(query.Get("table")),
		viewID:   strings.TrimSpace(query.Get("view")),
		recordID: strings.TrimSpace(query.Get("record")),
	}
}

func applyBaseURLSelection(out map[string]interface{}, selection baseURLSelection) {
	if selection.blockID != "" {
		// The Base web UI historically uses the query key "table" for the
		// currently selected top-level block. Its value can identify a table,
		// dashboard, workflow, or another block type. Keep it neutral until the
		// block directory confirms the resource type.
		out["block_id"] = selection.blockID
		out["selection_source"] = "url_query"
	}
}

func applyResolvedTableSelection(out map[string]interface{}, selection baseURLSelection) {
	if selection.viewID != "" {
		out["view_id"] = selection.viewID
	}
	if selection.recordID != "" {
		out["record_id"] = selection.recordID
	}
}

func resolveWikiBaseURL(runtime *common.RuntimeContext, u *url.URL) (map[string]interface{}, error) {
	token := firstPathSegmentAfter(u.Path, "/wiki/")
	data, err := runtime.CallAPITyped("GET", "/open-apis/wiki/v2/spaces/get_node", map[string]interface{}{"token": token}, nil)
	if err != nil {
		return nil, err
	}
	node := common.GetMap(data, "node")
	objType := strings.TrimSpace(common.GetString(node, "obj_type"))
	if objType != "bitable" {
		return nil, resolveValidationError(
			fmt.Sprintf("This Wiki URL resolves to %s, not Base.", valueOrUnknown(objType)),
			"Use the corresponding skill for that resource, or provide a Base URL.",
		)
	}
	baseToken := strings.TrimSpace(common.GetString(node, "obj_token"))
	if baseToken == "" {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "wiki node response is missing obj_token")
	}
	return map[string]interface{}{
		"input_type":      "wiki_url",
		"resource_type":   "bitable",
		"wiki_node_token": token,
		"base_token":      baseToken,
		"title":           common.GetString(node, "title"),
		"hint":            resolveHint("", nil),
	}, nil
}

func resolveRecordShareURL(runtime *common.RuntimeContext, u *url.URL) (map[string]interface{}, error) {
	shareToken := firstPathSegmentAfter(u.Path, "/record/")
	data, err := baseV3Call(runtime, "GET", baseV3Path("record_share", shareToken, "meta"), nil, nil)
	if err != nil {
		return nil, err
	}
	out := map[string]interface{}{
		"input_type":         "record_share_url",
		"resource_type":      "bitable",
		"record_share_token": firstNonEmpty(common.GetString(data, "record_share_token"), shareToken),
		"base_token":         common.GetString(data, "base_token"),
		"table_id":           common.GetString(data, "table_id"),
		"record_id":          common.GetString(data, "record_id"),
	}
	enrichRecordShareResolveHint(runtime, out)
	return out, nil
}

func resolveFormShareURL(u *url.URL) map[string]interface{} {
	return map[string]interface{}{
		"input_type":    "form_share_url",
		"resource_type": "bitable_form",
		"share_token":   firstPathSegmentAfter(u.Path, "/share/base/form/"),
		"hint": map[string]interface{}{
			"next_step": "use +form-detail to inspect the form, or use +form-submit to submit a response",
		},
	}
}

func executeBaseTitleResolve(runtime *common.RuntimeContext) error {
	query, err := readTitleResolveQuery(runtime)
	if err != nil {
		return err
	}
	data, err := runtime.CallAPITyped("POST", "/open-apis/search/v2/doc_wiki/search", nil, buildTitleResolveSearchBody(query))
	if err != nil {
		return err
	}
	candidates := normalizeTitleResolveCandidates(common.GetSlice(data, "res_units"))
	switch len(candidates) {
	case 0:
		return resolveValidationError(
			"No Base matched this title or keyword.",
			"Try a more specific Base title, or provide a /base/ URL and use base +url-resolve.",
		)
	case 1:
		out := map[string]interface{}{
			"input_type":    "title_query",
			"resource_type": "bitable",
			"title":         candidates[0]["title"],
			"base_token":    candidates[0]["base_token"],
			"url":           candidates[0]["url"],
			"owner_name":    candidates[0]["owner_name"],
			"update_time":   candidates[0]["update_time"],
			"hint":          resolveHint("", nil),
		}
		runtime.OutFormat(out, nil, nil)
		return nil
	default:
		runtime.OutFormat(map[string]interface{}{
			"input_type":    "title_query",
			"resource_type": "bitable",
			"candidates":    candidates,
			"hint": map[string]interface{}{
				"next_step": baseTitleResolveHint,
			},
		}, nil, nil)
		return nil
	}
}

func enrichBaseResolveHint(runtime *common.RuntimeContext, out map[string]interface{}, selection baseURLSelection) {
	baseToken := strings.TrimSpace(common.GetString(out, "base_token"))
	selectedBlockID := strings.TrimSpace(common.GetString(out, "block_id"))
	if baseToken == "" || selectedBlockID == "" {
		out["hint"] = resolveHint("", nil)
		return
	}

	if block, found, err := resolveSelectedBaseBlock(runtime, baseToken, selectedBlockID); err == nil && found {
		out["block_type"] = block.Type
		if block.Name != "" {
			out["block_name"] = block.Name
		}
		switch block.Type {
		case "table":
			applyResolvedTableSelection(out, selection)
			enrichResolvedTable(runtime, out, baseToken, selectedBlockID)
		case "dashboard":
			out["dashboard_id"] = selectedBlockID
			out["hint"] = map[string]interface{}{
				"next_step": "this dashboard is only the block currently selected by the URL; if the user names a different dashboard than block_name, use +dashboard-list and match that name first, otherwise use +dashboard-get to inspect this dashboard",
			}
		case "workflow":
			out["workflow_id"] = selectedBlockID
			out["hint"] = map[string]interface{}{
				"next_step": "use +workflow-get to inspect the resolved workflow",
			}
		case "folder":
			out["hint"] = map[string]interface{}{
				"next_step": fmt.Sprintf("use +base-block-list --base-token %s --parent-id %s to list this folder's direct children", baseToken, selectedBlockID),
			}
		case "docx":
			if block.DocxToken != "" {
				out["docx_token"] = block.DocxToken
				out["hint"] = map[string]interface{}{
					"next_step": fmt.Sprintf("use docs +fetch --doc %s to read this document", block.DocxToken),
				}
			} else {
				out["hint"] = map[string]interface{}{
					"next_step": "use +base-block-list --type docx and match block_id to retrieve this document's docx_token",
				}
			}
		default:
			out["hint"] = resolveUnknownBlockHint()
		}
		return
	}

	out["hint"] = resolveUnknownBlockHint()
}

type resolvedBaseBlock struct {
	ID        string
	Type      string
	Name      string
	DocxToken string
}

func resolveSelectedBaseBlock(runtime *common.RuntimeContext, baseToken, selectedBlockID string) (resolvedBaseBlock, bool, error) {
	data, err := baseV3Call(runtime, "POST", baseV3Path("bases", baseToken, "blocks", "list"), nil, map[string]interface{}{})
	if err != nil {
		return resolvedBaseBlock{}, false, err
	}
	for _, item := range common.GetSlice(data, "blocks") {
		row, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		block := resolvedBaseBlock{
			ID:        strings.TrimSpace(common.GetString(row, "id")),
			Type:      strings.TrimSpace(common.GetString(row, "type")),
			Name:      strings.TrimSpace(common.GetString(row, "name")),
			DocxToken: strings.TrimSpace(common.GetString(row, "docx_token")),
		}
		if block.ID == selectedBlockID {
			return block, true, nil
		}
	}
	return resolvedBaseBlock{}, false, nil
}

func enrichResolvedTable(runtime *common.RuntimeContext, out map[string]interface{}, baseToken, tableID string) {
	out["table_id"] = tableID
	fields, total, err := listAllFields(runtime, baseToken, tableID, 0, 100)
	if err != nil {
		out["hint"] = resolveHint(tableID, nil)
		return
	}
	out["hint"] = resolveHint(tableID, map[string]interface{}{"fields": map[string]interface{}{"fields": fields, "total": total}})
}

func resolveUnknownBlockHint() map[string]interface{} {
	return map[string]interface{}{
		"next_step": "use +base-block-list and match block_id to determine whether this is a table, dashboard, workflow, folder, or docx block",
	}
}

func enrichRecordShareResolveHint(runtime *common.RuntimeContext, out map[string]interface{}) {
	baseToken := strings.TrimSpace(common.GetString(out, "base_token"))
	tableID := strings.TrimSpace(common.GetString(out, "table_id"))
	recordID := strings.TrimSpace(common.GetString(out, "record_id"))
	hint := map[string]interface{}{}
	if baseToken != "" && tableID != "" && recordID != "" {
		if record, err := getResolveRecord(runtime, baseToken, tableID, recordID); err == nil {
			hint["record_data"] = formatResolvedRecordData(record)
		}
	}
	if baseToken != "" && tableID != "" {
		if fields, total, err := listAllFields(runtime, baseToken, tableID, 0, 100); err == nil {
			hint["fields"] = map[string]interface{}{"fields": fields, "total": total}
		}
	}
	out["hint"] = resolveHint(tableID, hint)
	common.GetMap(out, "hint")["next_step"] = recordShareNextStep(baseToken, tableID, recordID)
}

func getResolveRecord(runtime *common.RuntimeContext, baseToken, tableID, recordID string) (map[string]interface{}, error) {
	body := map[string]interface{}{"record_id_list": []string{recordID}}
	result, err := baseV3Raw(runtime, "POST", baseV3Path("bases", baseToken, "tables", tableID, "records", "batch_get"), nil, body)
	return handleBaseAPIResult(result, err, "batch get records")
}

func formatResolvedRecordData(record map[string]interface{}) map[string]interface{} {
	fieldIDs := common.GetSlice(record, "field_id_list")
	fieldNames := common.GetSlice(record, "fields")
	rows := common.GetSlice(record, "data")

	data := map[string]interface{}{}
	if len(rows) > 0 {
		if values, ok := rows[0].([]interface{}); ok {
			for i, value := range values {
				data[resolvedRecordFieldKey(fieldIDs, fieldNames, i)] = value
			}
		}
	}
	return data
}

func resolvedRecordFieldKey(fieldIDs, fieldNames []interface{}, index int) string {
	if index < len(fieldIDs) {
		if fieldID := strings.TrimSpace(fmt.Sprintf("%v", fieldIDs[index])); fieldID != "" {
			return fieldID
		}
	}
	if index < len(fieldNames) {
		if fieldName := strings.TrimSpace(fmt.Sprintf("%v", fieldNames[index])); fieldName != "" {
			return fieldName
		}
	}
	return fmt.Sprintf("field_%d", index+1)
}

func recordShareNextStep(baseToken, tableID, recordID string) string {
	return fmt.Sprintf(`use +record-batch-update --base-token %s --table-id %s --json '{"update_records":{"%s":{"<field_id>":<CellValue>}}}' to update this record`, baseToken, tableID, recordID)
}

func resolveHint(tableID string, extra map[string]interface{}) map[string]interface{} {
	hint := map[string]interface{}{}
	for key, value := range extra {
		hint[key] = value
	}
	if strings.TrimSpace(tableID) != "" {
		hint["next_step"] = nextStepRecordList
	} else {
		hint["next_step"] = nextStepBaseBlockList
	}
	return hint
}

func buildTitleResolveSearchBody(query string) map[string]interface{} {
	filter := map[string]interface{}{"doc_types": []string{"BITABLE"}}
	return map[string]interface{}{
		"query":       query,
		"page_size":   5,
		"doc_filter":  filter,
		"wiki_filter": filter,
	}
}

func normalizeTitleResolveCandidates(items []interface{}) []map[string]interface{} {
	candidates := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		row, _ := item.(map[string]interface{})
		meta, _ := row["result_meta"].(map[string]interface{})
		if row == nil || meta == nil || strings.ToUpper(common.GetString(meta, "doc_types")) != "BITABLE" {
			continue
		}
		token := strings.TrimSpace(common.GetString(meta, "token"))
		if token == "" {
			continue
		}
		title := stripSearchHighlight(common.GetString(row, "title_highlighted"))
		if title == "" {
			title = strings.TrimSpace(common.GetString(row, "title"))
		}
		candidates = append(candidates, map[string]interface{}{
			"title":       title,
			"base_token":  token,
			"url":         common.GetString(meta, "url"),
			"owner_name":  common.GetString(meta, "owner_name"),
			"update_time": common.GetString(meta, "update_time_iso"),
		})
	}
	return candidates
}

var searchHighlightTagRe = regexp.MustCompile(`</?h>`)

func stripSearchHighlight(s string) string {
	return strings.TrimSpace(searchHighlightTagRe.ReplaceAllString(s, ""))
}

func resolveValidationError(message, hint string) error {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", message).WithHint("%s", hint)
}

func normalizeResolvePath(path string) string {
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func pathSegmentExists(path, prefix string) bool {
	return firstPathSegmentAfter(path, prefix) != ""
}

func firstPathSegmentAfter(path, prefix string) string {
	path = normalizeResolvePath(path)
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := path[len(prefix):]
	if idx := strings.IndexByte(rest, '/'); idx >= 0 {
		rest = rest[:idx]
	}
	return strings.TrimSpace(rest)
}

func valueOrUnknown(s string) string {
	if strings.TrimSpace(s) == "" {
		return "an unknown resource type"
	}
	return s
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
