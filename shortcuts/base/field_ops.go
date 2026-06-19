// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

func dryRunFieldList(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	offset := runtime.Int("offset")
	if offset < 0 {
		offset = 0
	}
	limit := common.ParseIntBounded(runtime, "limit", 1, 200)
	return common.NewDryRunAPI().
		GET("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields").
		Params(map[string]interface{}{"offset": offset, "limit": limit}).
		Set("base_token", baseTokenOrRaw(runtime)).
		Set("table_id", baseTableID(runtime))
}

func dryRunFieldListBatch(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	offset := runtime.Int("offset")
	if offset < 0 {
		offset = 0
	}
	limit := common.ParseIntBounded(runtime, "limit", 1, 200)
	dry := common.NewDryRunAPI()
	for _, tableIDValue := range runtime.StrArray("table-id") {
		dry.GET(baseV3Path("bases", baseTokenOrRaw(runtime), "tables", tableIDValue, "fields")).
			Params(map[string]interface{}{"offset": offset, "limit": limit}).
			Set("base_token", baseTokenOrRaw(runtime)).
			Set("table_id", tableIDValue)
	}
	return dry
}

func dryRunFieldGet(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		GET("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields/:field_id").
		Set("base_token", baseTokenOrRaw(runtime)).
		Set("table_id", baseTableID(runtime)).
		Set("field_id", runtime.Str("field-id"))
}

func dryRunFieldCreate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	pc := newParseCtx(runtime)
	body, _ := parseJSONObject(pc, runtime.Str("json"), "json")
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields").
		Body(body).
		Set("base_token", baseTokenOrRaw(runtime)).
		Set("table_id", baseTableID(runtime))
}

func dryRunFieldUpdate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	pc := newParseCtx(runtime)
	body, _ := parseJSONObject(pc, runtime.Str("json"), "json")
	return common.NewDryRunAPI().
		PUT("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields/:field_id").
		Body(body).
		Set("base_token", baseTokenOrRaw(runtime)).
		Set("table_id", baseTableID(runtime)).
		Set("field_id", runtime.Str("field-id"))
}

func dryRunFieldDelete(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		DELETE("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields/:field_id").
		Set("base_token", baseTokenOrRaw(runtime)).
		Set("table_id", baseTableID(runtime)).
		Set("field_id", runtime.Str("field-id"))
}

func dryRunFieldSearchOptions(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	fieldRef := fieldSearchOptionsRef(runtime)
	params := map[string]interface{}{
		"offset": runtime.Int("offset"),
		"limit":  runtime.Int("limit"),
	}
	if params["limit"].(int) <= 0 {
		params["limit"] = 30
	}
	if keyword := strings.TrimSpace(fieldSearchOptionsKeyword(runtime)); keyword != "" {
		params["query"] = keyword
	}
	return common.NewDryRunAPI().
		GET(baseV3Path("bases", baseTokenOrRaw(runtime), "tables", baseTableID(runtime), "fields", fieldRef, "options")).
		Params(params).
		Set("base_token", baseTokenOrRaw(runtime)).
		Set("table_id", baseTableID(runtime)).
		Set("field_id", fieldRef)
}

func validateFieldJSON(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	pc := newParseCtx(runtime)
	return parseJSONObject(pc, runtime.Str("json"), "json")
}

func validateFormulaLookupGuideAck(runtime *common.RuntimeContext, command string, body map[string]interface{}) error {
	fieldType := strings.ToLower(strings.TrimSpace(common.GetString(body, "type")))
	if (fieldType == "formula" || fieldType == "lookup") && !runtime.Bool("i-have-read-guide") {
		guidePath := "skills/lark-base/references/formula-field-guide.md"
		if fieldType == "lookup" {
			guidePath = "skills/lark-base/references/lookup-field-guide.md"
		}
		return baseFlagErrorf("--i-have-read-guide is required for %s when --json.type is %q; read %s first, then retry with --i-have-read-guide", command, fieldType, guidePath)
	}
	return nil
}

func validateFieldCreate(runtime *common.RuntimeContext) error {
	body, err := validateFieldJSON(runtime)
	if err != nil {
		return err
	}
	return validateFormulaLookupGuideAck(runtime, "+field-create", body)
}

func validateFieldUpdate(runtime *common.RuntimeContext) error {
	body, err := validateFieldJSON(runtime)
	if err != nil {
		return err
	}
	return validateFormulaLookupGuideAck(runtime, "+field-update", body)
}

func executeFieldList(runtime *common.RuntimeContext) error {
	offset := runtime.Int("offset")
	if offset < 0 {
		offset = 0
	}
	limit := common.ParseIntBounded(runtime, "limit", 1, 200)
	baseToken := baseTokenOrRaw(runtime)
	tableID := strings.TrimSpace(baseTableID(runtime))
	if tableID == "" {
		return baseValidationErrorf("--table-id is required")
	}
	fields, total, err := listAllFields(runtime, baseToken, tableID, offset, limit)
	if err != nil {
		return err
	}
	if total == 0 {
		total = len(fields)
	}
	if runtime.Bool("compact") {
		fields = compactFields(fields)
	}
	runtime.Out(map[string]interface{}{"fields": fields, "total": total}, nil)
	return nil
}

func executeFieldListBatch(runtime *common.RuntimeContext) error {
	offset := runtime.Int("offset")
	if offset < 0 {
		offset = 0
	}
	limit := common.ParseIntBounded(runtime, "limit", 1, 200)
	baseToken := baseTokenOrRaw(runtime)
	rawRefs := runtime.StrArray("table-id")
	if len(rawRefs) == 0 {
		return baseValidationErrorf("--table-id is required")
	}
	tableIDs := make([]string, 0, len(rawRefs))
	for _, raw := range rawRefs {
		ref := strings.TrimSpace(raw)
		if ref == "" {
			return baseValidationErrorf("--table-id must not be empty")
		}
		tableIDs = append(tableIDs, ref)
	}
	results := make([]map[string]interface{}, 0, len(tableIDs))
	for _, tableID := range tableIDs {
		fields, total, err := listAllFields(runtime, baseToken, tableID, offset, limit)
		if err != nil {
			return err
		}
		if total == 0 {
			total = len(fields)
		}
		if runtime.Bool("compact") {
			fields = compactFields(fields)
		}
		results = append(results, map[string]interface{}{
			"table_id": tableID,
			"fields":   fields,
			"total":    total,
		})
	}
	runtime.Out(map[string]interface{}{"tables": results, "total": len(results)}, nil)
	return nil
}

// compactFields projects each field to the keys an agent needs for selection
// (id / name / type / style, plus select option names), dropping formula
// expressions and lookup internals that bloat agent context. Opt-in via
// `--compact`; the default output keeps full field objects.
func compactFields(fields []map[string]interface{}) []map[string]interface{} {
	keep := []string{"id", "name", "type", "is_primary", "ui_type", "description", "style"}
	out := make([]map[string]interface{}, 0, len(fields))
	for _, f := range fields {
		c := map[string]interface{}{}
		for _, k := range keep {
			if v, ok := f[k]; ok {
				c[k] = v
			}
		}
		if opts, ok := f["options"].([]interface{}); ok && len(opts) > 0 {
			names := make([]interface{}, 0, len(opts))
			for _, o := range opts {
				if om, ok := o.(map[string]interface{}); ok {
					if name, ok := om["name"]; ok {
						names = append(names, name)
						continue
					}
				}
				names = append(names, o)
			}
			c["options"] = names
		}
		out = append(out, c)
	}
	return out
}

func executeFieldGet(runtime *common.RuntimeContext) error {
	baseToken := baseTokenOrRaw(runtime)
	tableIDValue := baseTableID(runtime)
	fieldRef := runtime.Str("field-id")
	data, err := baseV3Call(runtime, "GET", baseV3Path("bases", baseToken, "tables", tableIDValue, "fields", fieldRef), nil, nil)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"field": data}, nil)
	return nil
}

func executeFieldCreate(runtime *common.RuntimeContext) error {
	pc := newParseCtx(runtime)
	body, err := parseJSONObject(pc, runtime.Str("json"), "json")
	if err != nil {
		return err
	}
	data, err := baseV3Call(runtime, "POST", baseV3Path("bases", baseTokenOrRaw(runtime), "tables", baseTableID(runtime), "fields"), nil, body)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"field": data, "created": true}, nil)
	return nil
}

func executeFieldUpdate(runtime *common.RuntimeContext) error {
	pc := newParseCtx(runtime)
	baseToken := baseTokenOrRaw(runtime)
	tableIDValue := baseTableID(runtime)
	body, err := parseJSONObject(pc, runtime.Str("json"), "json")
	if err != nil {
		return err
	}
	fieldRef := runtime.Str("field-id")
	data, err := baseV3Call(runtime, "PUT", baseV3Path("bases", baseToken, "tables", tableIDValue, "fields", fieldRef), nil, body)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"field": data, "updated": true}, nil)
	return nil
}

func executeFieldDelete(runtime *common.RuntimeContext) error {
	baseToken := baseTokenOrRaw(runtime)
	tableIDValue := baseTableID(runtime)
	fieldRef := runtime.Str("field-id")
	_, err := baseV3Call(runtime, "DELETE", baseV3Path("bases", baseToken, "tables", tableIDValue, "fields", fieldRef), nil, nil)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"deleted": true, "field_id": fieldRef, "field_name": fieldRef}, nil)
	return nil
}

func fieldSearchOptionsRef(runtime *common.RuntimeContext) string {
	fieldRef := runtime.Str("field-id")
	if strings.TrimSpace(fieldRef) == "" {
		fieldRef = runtime.Str("field-name")
	}
	return fieldRef
}

func fieldSearchOptionsKeyword(runtime *common.RuntimeContext) string {
	if keyword := strings.TrimSpace(runtime.Str("keyword")); keyword != "" {
		return keyword
	}
	return strings.TrimSpace(runtime.Str("query"))
}

func executeFieldSearchOptions(runtime *common.RuntimeContext) error {
	baseToken := baseTokenOrRaw(runtime)
	tableIDValue := baseTableID(runtime)
	fieldRef := fieldSearchOptionsRef(runtime)
	params := map[string]interface{}{
		"offset": runtime.Int("offset"),
		"limit":  runtime.Int("limit"),
	}
	if params["limit"].(int) <= 0 {
		params["limit"] = 30
	}
	if keyword := strings.TrimSpace(fieldSearchOptionsKeyword(runtime)); keyword != "" {
		params["query"] = keyword
	}
	data, err := baseV3Call(runtime, "GET", baseV3Path("bases", baseToken, "tables", tableIDValue, "fields", fieldRef, "options"), params, nil)
	if err != nil {
		return err
	}
	options, _ := data["options"].([]interface{})
	total := toInt(data["total"])
	if total == 0 {
		total = len(options)
	}
	out := map[string]interface{}{
		"keyword": fieldSearchOptionsKeyword(runtime),
		"options": options,
		"total":   total,
	}
	if v := strings.TrimSpace(runtime.Str("field-id")); v != "" {
		out["field_id"] = v
	}
	if v := strings.TrimSpace(runtime.Str("field-name")); v != "" {
		out["field_name"] = v
	}
	runtime.Out(out, nil)
	return nil
}
