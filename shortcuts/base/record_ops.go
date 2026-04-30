// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

const maxRecordSelectionCount = 200
const maxBatchGetSelectFieldCount = 100

type recordSelection struct {
	recordIDs    []string
	selectFields []string
	fromJSON     bool
}

type stringListNormalizeOptions struct {
	typeError     string
	emptyError    string
	itemName      string
	duplicateName string
	limitName     string
	max           int
	allowNil      bool
	allowEmpty    bool
}

func validateRecordSelection(runtime *common.RuntimeContext) error {
	_, err := resolveRecordSelection(runtime)
	return err
}

func resolveRecordSelection(runtime *common.RuntimeContext) (recordSelection, error) {
	recordIDs := runtime.StrArray("record-id")
	fieldIDs := runtime.StrArray("field-id")
	jsonRaw := strings.TrimSpace(runtime.Str("json"))
	if len(recordIDs) > 0 && jsonRaw != "" {
		return recordSelection{}, common.FlagErrorf("--record-id and --json are mutually exclusive")
	}
	if jsonRaw != "" {
		pc := newParseCtx(runtime)
		body, err := parseJSONObject(pc, jsonRaw, "json")
		if err != nil {
			return recordSelection{}, err
		}
		recordIDListValue, ok := body["record_id_list"]
		if !ok {
			return recordSelection{}, common.FlagErrorf(`--json must include "record_id_list" as a non-empty string array; %s`, jsonInputTip("json"))
		}
		recordIDItems, ok := recordIDListValue.([]interface{})
		if !ok {
			return recordSelection{}, common.FlagErrorf(`--json field "record_id_list" must be a string array; %s`, jsonInputTip("json"))
		}
		normalized, err := normalizeRecordIDs(recordIDItems)
		if err != nil {
			return recordSelection{}, err
		}
		selectFields, err := resolveRecordGetSelectFields(fieldIDs, body)
		if err != nil {
			return recordSelection{}, err
		}
		return recordSelection{
			recordIDs:    normalized,
			selectFields: selectFields,
			fromJSON:     true,
		}, nil
	}
	normalized, err := normalizeRecordIDs(recordIDs)
	if err != nil {
		return recordSelection{}, err
	}
	selectFields, err := resolveRecordGetSelectFields(fieldIDs, nil)
	if err != nil {
		return recordSelection{}, err
	}
	return recordSelection{
		recordIDs:    normalized,
		selectFields: selectFields,
	}, nil
}

func normalizeRecordIDs(values interface{}) ([]string, error) {
	return normalizeStringList(values, stringListNormalizeOptions{
		typeError:     "record selection must be a string array",
		emptyError:    `provide at least one --record-id, or use --json with "record_id_list"`,
		itemName:      "record selection item",
		duplicateName: "record id",
		limitName:     "record selection",
		max:           maxRecordSelectionCount,
	})
}

func resolveRecordGetSelectFields(flagFields []string, body map[string]interface{}) ([]string, error) {
	fromFlags, err := normalizeRecordGetSelectFields(flagFields)
	if err != nil {
		return nil, err
	}
	if body == nil {
		return fromFlags, nil
	}
	rawJSONFields, ok := body["select_fields"]
	if !ok {
		return fromFlags, nil
	}
	if len(fromFlags) > 0 {
		return nil, common.FlagErrorf(`--field-id and --json field "select_fields" are mutually exclusive`)
	}
	items, ok := rawJSONFields.([]interface{})
	if !ok {
		return nil, common.FlagErrorf(`--json field "select_fields" must be a string array; %s`, jsonInputTip("json"))
	}
	if len(items) == 0 {
		return nil, common.FlagErrorf(`--json field "select_fields" must not be empty; %s`, jsonInputTip("json"))
	}
	normalized, err := normalizeRecordGetSelectFields(items)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

func normalizeRecordGetSelectFields(values interface{}) ([]string, error) {
	return normalizeStringList(values, stringListNormalizeOptions{
		typeError:     "field selection must be a string array",
		itemName:      "field selection item",
		duplicateName: "field id",
		limitName:     "field selection",
		max:           maxBatchGetSelectFieldCount,
		allowNil:      true,
		allowEmpty:    true,
	})
}

func normalizeStringList(values interface{}, opts stringListNormalizeOptions) ([]string, error) {
	var rawItems []interface{}
	switch typed := values.(type) {
	case nil:
		if opts.allowNil {
			return nil, nil
		}
		return nil, common.FlagErrorf(opts.typeError)
	case []interface{}:
		rawItems = typed
	case []string:
		rawItems = make([]interface{}, 0, len(typed))
		for _, item := range typed {
			rawItems = append(rawItems, item)
		}
	default:
		return nil, common.FlagErrorf(opts.typeError)
	}
	if len(rawItems) == 0 {
		if opts.allowEmpty {
			return nil, nil
		}
		return nil, common.FlagErrorf(opts.emptyError)
	}
	if opts.max > 0 && len(rawItems) > opts.max {
		return nil, common.FlagErrorf("%s exceeds maximum limit of %d (got %d)", opts.limitName, opts.max, len(rawItems))
	}
	seen := make(map[string]int, len(rawItems))
	result := make([]string, 0, len(rawItems))
	for index, value := range rawItems {
		item, ok := value.(string)
		if !ok {
			return nil, common.FlagErrorf("%s %d must be a string", opts.itemName, index+1)
		}
		item = strings.TrimSpace(item)
		if item == "" {
			return nil, common.FlagErrorf("%s %d must not be empty", opts.itemName, index+1)
		}
		if first, exists := seen[item]; exists {
			return nil, common.FlagErrorf("duplicate %s %q at positions %d and %d", opts.duplicateName, item, first, index+1)
		}
		seen[item] = index + 1
		result = append(result, item)
	}
	return result, nil
}

func recordGetBatchBody(selection recordSelection) map[string]interface{} {
	body := map[string]interface{}{
		"record_id_list": selection.recordIDs,
	}
	if len(selection.selectFields) > 0 {
		body["select_fields"] = selection.selectFields
	}
	return body
}

func dryRunRecordList(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	offset := runtime.Int("offset")
	if offset < 0 {
		offset = 0
	}
	limit := common.ParseIntBounded(runtime, "limit", 1, 200)
	params := url.Values{}
	params.Set("offset", strconv.Itoa(offset))
	params.Set("limit", strconv.Itoa(limit))
	for _, field := range recordListFields(runtime) {
		params.Add("field_id", field)
	}
	if viewID := runtime.Str("view-id"); viewID != "" {
		params.Set("view_id", viewID)
	}
	path := "/open-apis/base/v3/bases/:base_token/tables/:table_id/records?" + params.Encode()
	return common.NewDryRunAPI().
		GET(path).
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime))
}

func dryRunRecordGet(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	selection, err := resolveRecordSelection(runtime)
	if err != nil {
		return common.NewDryRunAPI()
	}
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/tables/:table_id/records/batch_get").
		Body(recordGetBatchBody(selection)).
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime))
}

func dryRunRecordSearch(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	pc := newParseCtx(runtime)
	body, _ := parseJSONObject(pc, runtime.Str("json"), "json")
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/tables/:table_id/records/search").
		Body(body).
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime))
}

func dryRunRecordUpsert(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	pc := newParseCtx(runtime)
	body, _ := parseJSONObject(pc, runtime.Str("json"), "json")
	if recordID := runtime.Str("record-id"); recordID != "" {
		return common.NewDryRunAPI().
			PATCH("/open-apis/base/v3/bases/:base_token/tables/:table_id/records/:record_id").
			Body(body).
			Set("base_token", runtime.Str("base-token")).
			Set("table_id", baseTableID(runtime)).
			Set("record_id", recordID)
	}
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/tables/:table_id/records").
		Body(body).
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime))
}

func dryRunRecordBatchCreate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	pc := newParseCtx(runtime)
	body, _ := parseJSONObject(pc, runtime.Str("json"), "json")
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/tables/:table_id/records/batch_create").
		Body(body).
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime))
}

func dryRunRecordBatchUpdate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	pc := newParseCtx(runtime)
	body, _ := parseJSONObject(pc, runtime.Str("json"), "json")
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/tables/:table_id/records/batch_update").
		Body(body).
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime))
}

func dryRunRecordDelete(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	selection, err := resolveRecordSelection(runtime)
	if err != nil {
		return common.NewDryRunAPI()
	}
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/tables/:table_id/records/batch_delete").
		Body(map[string]interface{}{"record_id_list": selection.recordIDs}).
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime))
}

func dryRunRecordHistoryList(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	params := map[string]interface{}{
		"table_id":  baseTableID(runtime),
		"record_id": runtime.Str("record-id"),
		"page_size": runtime.Int("page-size"),
	}
	if value := runtime.Int("max-version"); value > 0 {
		params["max_version"] = value
	}
	return common.NewDryRunAPI().
		GET("/open-apis/base/v3/bases/:base_token/record_history").
		Params(params).
		Set("base_token", runtime.Str("base-token"))
}

func dryRunRecordShareBatch(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	recordIDs := deduplicateRecordIDs(runtime)
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/tables/:table_id/records/share_links/batch").
		Body(map[string]interface{}{"record_ids": recordIDs}).
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime))
}

const maxShareBatchSize = 100

func validateRecordShareBatch(runtime *common.RuntimeContext) error {
	recordIDs := deduplicateRecordIDs(runtime)
	if len(recordIDs) == 0 {
		return common.FlagErrorf("--record-ids is required and must not be empty")
	}
	if len(recordIDs) > maxShareBatchSize {
		return common.FlagErrorf("--record-ids exceeds maximum limit of %d (got %d)", maxShareBatchSize, len(recordIDs))
	}
	return nil
}

func deduplicateRecordIDs(runtime *common.RuntimeContext) []string {
	raw := runtime.StrSlice("record-ids")
	seen := make(map[string]bool, len(raw))
	result := make([]string, 0, len(raw))
	for _, id := range raw {
		if id != "" && !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result
}

func executeRecordShareBatch(runtime *common.RuntimeContext) error {
	recordIDs := deduplicateRecordIDs(runtime)
	body := map[string]interface{}{
		"record_ids": recordIDs,
	}
	data, err := baseV3Call(runtime, "POST",
		baseV3Path("bases", runtime.Str("base-token"), "tables", baseTableID(runtime), "records", "share_links", "batch"),
		nil, body)
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}

func validateRecordJSON(runtime *common.RuntimeContext) error {
	pc := newParseCtx(runtime)
	_, err := parseJSONObject(pc, runtime.Str("json"), "json")
	return err
}

func recordListFields(runtime *common.RuntimeContext) []string {
	return runtime.StrArray("field-id")
}

func executeRecordList(runtime *common.RuntimeContext) error {
	if err := validateRecordReadFormat(runtime); err != nil {
		return err
	}
	offset := runtime.Int("offset")
	if offset < 0 {
		offset = 0
	}
	limit := common.ParseIntBounded(runtime, "limit", 1, 200)
	params := map[string]interface{}{"offset": offset, "limit": limit}
	fields := recordListFields(runtime)
	if len(fields) > 0 {
		params["field_id"] = fields
	}
	if viewID := runtime.Str("view-id"); viewID != "" {
		params["view_id"] = viewID
	}
	data, err := baseV3Call(runtime, "GET", baseV3Path("bases", runtime.Str("base-token"), "tables", baseTableID(runtime), "records"), params, nil)
	if err != nil {
		return err
	}
	if runtime.Str("format") == "markdown" {
		return outputRecordMarkdown(runtime, data)
	}
	runtime.Out(data, nil)
	return nil
}

func executeRecordGet(runtime *common.RuntimeContext) error {
	if err := validateRecordReadFormat(runtime); err != nil {
		return err
	}
	selection, err := resolveRecordSelection(runtime)
	if err != nil {
		return err
	}
	result, err := baseV3Raw(runtime, "POST", baseV3Path("bases", runtime.Str("base-token"), "tables", baseTableID(runtime), "records", "batch_get"), nil, recordGetBatchBody(selection))
	data, err := handleBaseAPIResult(result, err, "batch get records")
	if err != nil {
		return err
	}
	if runtime.Str("format") == "markdown" {
		return outputRecordGetMarkdown(runtime, data)
	}
	runtime.Out(data, nil)
	return nil
}

func executeRecordSearch(runtime *common.RuntimeContext) error {
	pc := newParseCtx(runtime)
	body, err := parseJSONObject(pc, runtime.Str("json"), "json")
	if err != nil {
		return err
	}
	data, err := baseV3Call(runtime, "POST", baseV3Path("bases", runtime.Str("base-token"), "tables", baseTableID(runtime), "records", "search"), nil, body)
	if err != nil {
		return err
	}
	if runtime.Str("format") == "markdown" {
		return outputRecordMarkdown(runtime, data)
	}
	runtime.Out(data, nil)
	return nil
}

func executeRecordUpsert(runtime *common.RuntimeContext) error {
	pc := newParseCtx(runtime)
	body, err := parseJSONObject(pc, runtime.Str("json"), "json")
	if err != nil {
		return err
	}
	baseToken := runtime.Str("base-token")
	tableIDValue := baseTableID(runtime)
	if recordID := runtime.Str("record-id"); recordID != "" {
		data, err := baseV3Call(runtime, "PATCH", baseV3Path("bases", baseToken, "tables", tableIDValue, "records", recordID), nil, body)
		if err != nil {
			return err
		}
		runtime.Out(map[string]interface{}{"record": data, "updated": true}, nil)
		return nil
	}
	data, err := baseV3Call(runtime, "POST", baseV3Path("bases", baseToken, "tables", tableIDValue, "records"), nil, body)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"record": data, "created": true}, nil)
	return nil
}

func executeRecordBatchCreate(runtime *common.RuntimeContext) error {
	pc := newParseCtx(runtime)
	body, err := parseJSONObject(pc, runtime.Str("json"), "json")
	if err != nil {
		return err
	}
	result, err := baseV3Raw(runtime, "POST", baseV3Path("bases", runtime.Str("base-token"), "tables", baseTableID(runtime), "records", "batch_create"), nil, body)
	data, err := handleBaseAPIResult(result, err, "batch create records")
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}

func executeRecordBatchUpdate(runtime *common.RuntimeContext) error {
	pc := newParseCtx(runtime)
	body, err := parseJSONObject(pc, runtime.Str("json"), "json")
	if err != nil {
		return err
	}
	result, err := baseV3Raw(runtime, "POST", baseV3Path("bases", runtime.Str("base-token"), "tables", baseTableID(runtime), "records", "batch_update"), nil, body)
	data, err := handleBaseAPIResult(result, err, "batch update records")
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}

func executeRecordDelete(runtime *common.RuntimeContext) error {
	selection, err := resolveRecordSelection(runtime)
	if err != nil {
		return err
	}
	result, err := baseV3Raw(runtime, "POST", baseV3Path("bases", runtime.Str("base-token"), "tables", baseTableID(runtime), "records", "batch_delete"), nil, map[string]interface{}{
		"record_id_list": selection.recordIDs,
	})
	data, err := handleBaseAPIResult(result, err, "batch delete records")
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}
