// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

func dryRunFieldList(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	offset := runtime.Int("offset")
	if offset < 0 {
		offset = 0
	}
	limit := getPaginationLimit(runtime)
	return common.NewDryRunAPI().
		GET("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields").
		Params(map[string]interface{}{"offset": offset, "limit": limit}).
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime))
}

func dryRunFieldGet(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		GET("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields/:field_id").
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime)).
		Set("field_id", runtime.Str("field-id"))
}

func dryRunFieldCreate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	pc := newParseCtx(runtime)
	body, _ := parseJSONObject(pc, runtime.Str("json"), "json")
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields").
		Body(body).
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime))
}

func dryRunFieldUpdate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	pc := newParseCtx(runtime)
	body, _ := parseJSONObject(pc, runtime.Str("json"), "json")
	normalized, reformatExisting, err := normalizeFieldUpdateBody(runtime, body)
	if err != nil || normalized == nil {
		normalized = body
		reformatExisting = false
	}
	dr := common.NewDryRunAPI().
		PUT("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields/:field_id").
		Body(normalized).
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime)).
		Set("field_id", runtime.Str("field-id"))
	if reformatExisting {
		if legacyBody, legacyErr := buildAutoNumberReformatBody(normalized, dryRunFieldName(runtime, normalized)); legacyErr == nil {
			dr.PUT("/open-apis/bitable/v1/apps/:base_token/tables/:table_id/fields/:field_id").Body(legacyBody)
		}
	}
	return dr
}

func dryRunFieldDelete(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		DELETE("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields/:field_id").
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime)).
		Set("field_id", runtime.Str("field-id"))
}

func dryRunFieldSearchOptions(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	limit := getPaginationLimit(runtime)
	params := map[string]interface{}{
		"offset": runtime.Int("offset"),
		"limit":  limit,
	}
	if keyword := strings.TrimSpace(runtime.Str("keyword")); keyword != "" {
		params["query"] = keyword
	}
	return common.NewDryRunAPI().
		GET("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields/:field_id/options").
		Params(params).
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime)).
		Set("field_id", runtime.Str("field-id"))
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
	normalized, _, err := normalizeFieldUpdateBody(runtime, body)
	if err != nil {
		return err
	}
	return validateFormulaLookupGuideAck(runtime, "+field-update", normalized)
}

func executeFieldList(runtime *common.RuntimeContext) error {
	offset := runtime.Int("offset")
	if offset < 0 {
		offset = 0
	}
	limit := getPaginationLimit(runtime)
	fields, total, err := listAllFields(runtime, runtime.Str("base-token"), baseTableID(runtime), offset, limit)
	if err != nil {
		return err
	}
	if total == 0 {
		total = len(fields)
	}
	runtime.Out(map[string]interface{}{"fields": fields, "total": total}, nil)
	return nil
}

func executeFieldGet(runtime *common.RuntimeContext) error {
	baseToken := runtime.Str("base-token")
	tableIDValue := baseTableID(runtime)
	fieldRef := runtime.Str("field-id")
	data, err := baseV3Call(runtime, "GET", baseV3Path("bases", baseToken, "tables", tableIDValue, "fields", fieldRef), nil, nil)
	if err != nil {
		return enrichFieldGetNotFoundHint(fieldRef, err)
	}
	runtime.Out(map[string]interface{}{"field": data}, nil)
	return nil
}

func enrichFieldGetNotFoundHint(fieldRef string, err error) error {
	fieldRef = strings.TrimSpace(fieldRef)
	if !strings.HasPrefix(fieldRef, "fld") {
		return err
	}

	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr == nil || exitErr.Detail == nil {
		return err
	}

	detail := exitErr.Detail
	if detail.Code != 800030201 && detail.Type != "not_found" && !strings.EqualFold(strings.TrimSpace(detail.Message), "not_found") {
		return err
	}

	detailMap, _ := detail.Detail.(map[string]interface{})
	if path := strings.TrimSpace(common.GetString(detailMap, "path")); path != "" && path != "/fields/:field_id" {
		return err
	}

	extraHint := "If this field ID came from a bidirectional link, it may be the linked table's auto-created reverse field rather than a field in the current table. Use the forward link field here, or switch to the linked table and run +field-list / +field-get there."
	if strings.Contains(detail.Hint, "auto-created reverse field") {
		return err
	}
	if strings.TrimSpace(detail.Hint) == "" {
		detail.Hint = extraHint
		return err
	}
	detail.Hint = strings.TrimSpace(detail.Hint + " " + extraHint)
	return err
}

func executeFieldCreate(runtime *common.RuntimeContext) error {
	pc := newParseCtx(runtime)
	body, err := parseJSONObject(pc, runtime.Str("json"), "json")
	if err != nil {
		return err
	}
	data, err := baseV3Call(runtime, "POST", baseV3Path("bases", runtime.Str("base-token"), "tables", baseTableID(runtime), "fields"), nil, body)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"field": data, "created": true}, nil)
	return nil
}

func executeFieldUpdate(runtime *common.RuntimeContext) error {
	pc := newParseCtx(runtime)
	baseToken := runtime.Str("base-token")
	tableIDValue := baseTableID(runtime)
	body, err := parseJSONObject(pc, runtime.Str("json"), "json")
	if err != nil {
		return err
	}
	normalized, reformatExisting, err := normalizeFieldUpdateBody(runtime, body)
	if err != nil {
		return err
	}
	fieldRef := runtime.Str("field-id")
	data, err := baseV3Call(runtime, "PUT", baseV3Path("bases", baseToken, "tables", tableIDValue, "fields", fieldRef), nil, normalized)
	if err != nil {
		if !reformatExisting && isFieldUpdateNoopError(err) {
			fieldData, readErr := baseV3Call(runtime, "GET", baseV3Path("bases", baseToken, "tables", tableIDValue, "fields", fieldRef), nil, nil)
			if readErr != nil || !fieldUpdateSubsetMatches(fieldData, normalized) {
				return err
			}
			runtime.Out(map[string]interface{}{"field": fieldData, "updated": false, "noop": true}, nil)
			return nil
		}
		return err
	}
	if !reformatExisting {
		runtime.Out(map[string]interface{}{"field": data, "updated": true}, nil)
		return nil
	}

	tableIDResolved, err := resolveLegacyFieldUpdateTableID(runtime, baseToken, tableIDValue)
	if err != nil {
		return err
	}
	fieldIDResolved, fieldNameResolved, err := resolveLegacyFieldUpdateField(runtime, baseToken, tableIDResolved, fieldRef)
	if err != nil {
		return err
	}
	legacyBody, err := buildAutoNumberReformatBody(normalized, fieldNameResolved)
	if err != nil {
		return err
	}
	if _, err := bitableV1Call(runtime, "PUT", bitableV1Path("apps", baseToken, "tables", tableIDResolved, "fields", fieldIDResolved), nil, legacyBody); err != nil {
		return err
	}
	fieldData, err := baseV3Call(runtime, "GET", baseV3Path("bases", baseToken, "tables", tableIDResolved, "fields", fieldIDResolved), nil, nil)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"field": fieldData, "updated": true, "reformatted_existing_records": true}, nil)
	return nil
}

func isFieldUpdateNoopError(err error) bool {
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr == nil || exitErr.Detail == nil {
		return false
	}
	if exitErr.Detail.Code != 800070003 {
		return false
	}
	msg := strings.TrimSpace(exitErr.Detail.Message)
	return msg == "" || strings.EqualFold(msg, "no operation produced")
}

func fieldUpdateSubsetMatches(actual map[string]interface{}, desired map[string]interface{}) bool {
	return fieldUpdateValueMatches(actual, desired)
}

func fieldUpdateValueMatches(actual interface{}, desired interface{}) bool {
	switch want := desired.(type) {
	case map[string]interface{}:
		got, ok := actual.(map[string]interface{})
		if !ok {
			return false
		}
		for key, wantValue := range want {
			gotValue, exists := got[key]
			if !exists || !fieldUpdateValueMatches(gotValue, wantValue) {
				return false
			}
		}
		return true
	case []interface{}:
		got, ok := actual.([]interface{})
		if !ok || len(got) != len(want) {
			return false
		}
		for i := range want {
			if !fieldUpdateValueMatches(got[i], want[i]) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(actual, desired)
	}
}

func executeFieldDelete(runtime *common.RuntimeContext) error {
	baseToken := runtime.Str("base-token")
	tableIDValue := baseTableID(runtime)
	fieldRef := runtime.Str("field-id")
	_, err := baseV3Call(runtime, "DELETE", baseV3Path("bases", baseToken, "tables", tableIDValue, "fields", fieldRef), nil, nil)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"deleted": true, "field_id": fieldRef, "field_name": fieldRef}, nil)
	return nil
}

func executeFieldSearchOptions(runtime *common.RuntimeContext) error {
	baseToken := runtime.Str("base-token")
	tableIDValue := baseTableID(runtime)
	fieldRef := runtime.Str("field-id")
	limit := getPaginationLimit(runtime)
	params := map[string]interface{}{
		"offset": runtime.Int("offset"),
		"limit":  limit,
	}
	if keyword := strings.TrimSpace(runtime.Str("keyword")); keyword != "" {
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
	runtime.Out(map[string]interface{}{
		"field_id":   fieldRef,
		"field_name": fieldRef,
		"keyword":    strings.TrimSpace(runtime.Str("keyword")),
		"options":    options,
		"total":      total,
	}, nil)
	return nil
}

func normalizeFieldUpdateBody(runtime *common.RuntimeContext, body map[string]interface{}) (map[string]interface{}, bool, error) {
	if body == nil {
		return nil, false, nil
	}
	normalized := cloneMap(body)
	reformatExisting := runtime.Bool("reformat-existing-records")
	isAutoNumber := false

	switch strings.ToLower(strings.TrimSpace(common.GetString(normalized, "type"))) {
	case "autonumber", "auto-number", "auto_number":
		normalized["type"] = "auto_number"
		isAutoNumber = true
	}
	if toInt(normalized["type"]) == 1005 {
		normalized["type"] = "auto_number"
		isAutoNumber = true
	}

	if fieldName := strings.TrimSpace(common.GetString(normalized, "field_name")); fieldName != "" && strings.TrimSpace(common.GetString(normalized, "name")) == "" {
		normalized["name"] = fieldName
	}
	delete(normalized, "field_name")

	style, _ := normalized["style"].(map[string]interface{})
	if style != nil {
		style = cloneMap(style)
		if flag, ok := style["reformat_existing_records"].(bool); ok {
			reformatExisting = reformatExisting || flag
			delete(style, "reformat_existing_records")
		}
		if len(style) == 0 {
			delete(normalized, "style")
		} else {
			normalized["style"] = style
		}
	}

	if property, ok := normalized["property"].(map[string]interface{}); ok && property != nil {
		if autoSerial, ok := property["auto_serial"].(map[string]interface{}); ok && autoSerial != nil {
			isAutoNumber = true
			normalized["type"] = "auto_number"
			if flag, ok := autoSerial["reformat_existing_records"].(bool); ok {
				reformatExisting = reformatExisting || flag
			}
			if style == nil {
				style = map[string]interface{}{}
			}
			if _, exists := style["rules"]; !exists {
				if options, ok := autoSerial["options"].([]interface{}); ok && len(options) > 0 {
					rules, err := legacyAutoNumberOptionsToRules(options)
					if err != nil {
						return nil, false, err
					}
					style["rules"] = rules
				}
			}
			if len(style) == 0 {
				delete(normalized, "style")
			} else {
				normalized["style"] = style
			}
			delete(normalized, "property")
		}
	}

	if reformatExisting && !isAutoNumber {
		currentType := strings.TrimSpace(common.GetString(normalized, "type"))
		if currentType == "" {
			currentType = strings.TrimSpace(fmt.Sprintf("%v", body["type"]))
		}
		if currentType == "" || currentType == "<nil>" {
			currentType = "unset"
		}
		return nil, false, common.FlagErrorf("--reformat-existing-records is only supported when --json.type is %q; current type is %q", "auto_number", currentType)
	}

	return normalized, reformatExisting, nil
}

func legacyAutoNumberOptionsToRules(options []interface{}) ([]interface{}, error) {
	rules := make([]interface{}, 0, len(options))
	for i, item := range options {
		option, ok := item.(map[string]interface{})
		if !ok {
			return nil, common.FlagErrorf("auto_number legacy option %d must be an object", i+1)
		}
		switch strings.TrimSpace(common.GetString(option, "type")) {
		case "fixed_text":
			rules = append(rules, map[string]interface{}{"type": "text", "text": common.GetString(option, "value")})
		case "created_time":
			dateFormat := common.GetString(option, "value")
			if dateFormat == "" {
				dateFormat = "yyyyMMdd"
			}
			rules = append(rules, map[string]interface{}{"type": "created_time", "date_format": dateFormat})
		case "system_number":
			length := toInt(option["value"])
			if length <= 0 {
				length = 3
			}
			rules = append(rules, map[string]interface{}{"type": "incremental_number", "length": length})
		default:
			return nil, common.FlagErrorf("unsupported auto_number legacy option type %q; use fixed_text, created_time, or system_number", common.GetString(option, "type"))
		}
	}
	return rules, nil
}

func autoNumberRulesToLegacyOptions(rules []interface{}) ([]interface{}, error) {
	options := make([]interface{}, 0, len(rules))
	for i, item := range rules {
		rule, ok := item.(map[string]interface{})
		if !ok {
			return nil, common.FlagErrorf("auto_number style.rules[%d] must be an object", i)
		}
		switch strings.TrimSpace(common.GetString(rule, "type")) {
		case "text":
			options = append(options, map[string]interface{}{"type": "fixed_text", "value": common.GetString(rule, "text")})
		case "created_time":
			dateFormat := common.GetString(rule, "date_format")
			if dateFormat == "" {
				dateFormat = "yyyyMMdd"
			}
			options = append(options, map[string]interface{}{"type": "created_time", "value": dateFormat})
		case "incremental_number":
			length := toInt(rule["length"])
			if length <= 0 {
				length = 3
			}
			options = append(options, map[string]interface{}{"type": "system_number", "value": strconv.Itoa(length)})
		default:
			return nil, common.FlagErrorf("unsupported auto_number rule type %q; use text, created_time, or incremental_number", common.GetString(rule, "type"))
		}
	}
	return options, nil
}

func autoNumberRulesFromBody(body map[string]interface{}) ([]interface{}, error) {
	if style, ok := body["style"].(map[string]interface{}); ok && style != nil {
		if rawRules, exists := style["rules"]; exists {
			rules, ok := rawRules.([]interface{})
			if !ok {
				return nil, common.FlagErrorf("auto_number style.rules must be a JSON array when using --reformat-existing-records")
			}
			cloned, _ := cloneValue(rules).([]interface{})
			return cloned, nil
		}
	}
	spec, err := resolveFieldTypeSpec("auto_number")
	if err != nil {
		return nil, err
	}
	style, _ := spec.Extra["style"].(map[string]interface{})
	rules, _ := style["rules"].([]interface{})
	if len(rules) == 0 {
		return nil, common.FlagErrorf("default auto_number style rules are unavailable")
	}
	cloned, _ := cloneValue(rules).([]interface{})
	return cloned, nil
}

func buildAutoNumberReformatBody(body map[string]interface{}, fieldName string) (map[string]interface{}, error) {
	fieldName = strings.TrimSpace(fieldName)
	if fieldName == "" {
		return nil, common.FlagErrorf("auto_number reformat needs a field name; include --json.name or read the current field first with +field-get")
	}
	rules, err := autoNumberRulesFromBody(body)
	if err != nil {
		return nil, err
	}
	options, err := autoNumberRulesToLegacyOptions(rules)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"field_name": fieldName,
		"type":       1005,
		"property": map[string]interface{}{
			"auto_serial": map[string]interface{}{
				"type":                      "custom",
				"reformat_existing_records": true,
				"options":                   options,
			},
		},
	}, nil
}

func dryRunFieldName(runtime *common.RuntimeContext, body map[string]interface{}) string {
	if body != nil {
		if name := strings.TrimSpace(common.GetString(body, "name")); name != "" {
			return name
		}
	}
	fieldRef := strings.TrimSpace(runtime.Str("field-id"))
	if fieldRef != "" && !strings.HasPrefix(fieldRef, "fld") {
		return fieldRef
	}
	return ""
}

func resolveLegacyFieldUpdateTableID(runtime *common.RuntimeContext, baseToken string, tableRef string) (string, error) {
	if strings.HasPrefix(strings.TrimSpace(tableRef), "tbl") {
		return tableRef, nil
	}
	table, err := baseV3Call(runtime, "GET", baseV3Path("bases", baseToken, "tables", tableRef), nil, nil)
	if err != nil {
		return "", err
	}
	if tableID := strings.TrimSpace(common.GetString(table, "id")); tableID != "" {
		return tableID, nil
	}
	if tableID := strings.TrimSpace(common.GetString(table, "table_id")); tableID != "" {
		return tableID, nil
	}
	return "", output.ErrWithHint(output.ExitAPI, "api_error", fmt.Sprintf("resolved table %q but the response did not include a table id", tableRef), "Retry with the table ID from +table-list or +table-get.")
}

func resolveLegacyFieldUpdateField(runtime *common.RuntimeContext, baseToken string, tableRef string, fieldRef string) (string, string, error) {
	field, err := baseV3Call(runtime, "GET", baseV3Path("bases", baseToken, "tables", tableRef, "fields", fieldRef), nil, nil)
	if err != nil {
		return "", "", err
	}
	fieldID := strings.TrimSpace(common.GetString(field, "id"))
	if fieldID == "" {
		fieldID = strings.TrimSpace(common.GetString(field, "field_id"))
	}
	fieldName := strings.TrimSpace(common.GetString(field, "name"))
	if fieldName == "" {
		fieldName = strings.TrimSpace(common.GetString(field, "field_name"))
	}
	if fieldID == "" {
		return "", "", output.ErrWithHint(output.ExitAPI, "api_error", fmt.Sprintf("resolved field %q but the response did not include a field id", fieldRef), "Retry with the field ID from +field-list or +field-get.")
	}
	return fieldID, fieldName, nil
}
