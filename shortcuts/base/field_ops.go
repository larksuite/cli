// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// Keep field writes sequential and use the lower bound of the documented
// 0.5-1s write-conflict guidance as the minimum interval between request starts.
// Request latency counts toward the interval, so successful calls do not incur
// an unconditional sleep.
var fieldCreateBatchDelay = 500 * time.Millisecond

func fieldCreateThrottleDelay(previousStartedAt, now time.Time) time.Duration {
	if previousStartedAt.IsZero() || fieldCreateBatchDelay <= 0 {
		return 0
	}
	wait := previousStartedAt.Add(fieldCreateBatchDelay).Sub(now)
	if wait > 0 {
		return wait
	}
	return 0
}

func dryRunFieldList(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	offset := runtime.Int("offset")
	if offset < 0 {
		offset = 0
	}
	limit := runtime.Int("limit")
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
	bodies, err := parseFieldCreateBodies(pc, runtime.Str("json"))
	if err != nil {
		return common.NewDryRunAPI().Desc(fmt.Sprintf("dry-run validation failed: %v", err))
	}
	dr := common.NewDryRunAPI().
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime))
	for _, body := range bodies {
		dr.POST("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields").Body(body)
	}
	return dr
}

func dryRunFieldUpdate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	pc := newParseCtx(runtime)
	body, err := parseJSONObject(pc, runtime.Str("json"), "json")
	if err != nil {
		return common.NewDryRunAPI().Desc(fmt.Sprintf("dry-run validation failed: %v", err))
	}
	return common.NewDryRunAPI().
		PUT("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields/:field_id").
		Body(body).
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime)).
		Set("field_id", runtime.Str("field-id"))
}

func dryRunFieldDelete(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		DELETE("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields/:field_id").
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime)).
		Set("field_id", runtime.Str("field-id"))
}

func dryRunFieldSearchOptions(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	limit := runtime.Int("limit")
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
		guidePath := "skills/lark-base/references/lark-base-field-formula.md"
		if fieldType == "lookup" {
			guidePath = "skills/lark-base/references/lark-base-field-lookup.md"
		}
		return baseFlagErrorf("--i-have-read-guide is required for %s when --json.type is %q; read %s first, then retry with --i-have-read-guide", command, fieldType, guidePath)
	}
	return nil
}

func validateFieldCreate(runtime *common.RuntimeContext) error {
	bodies, err := parseFieldCreateBodies(newParseCtx(runtime), runtime.Str("json"))
	if err != nil {
		return err
	}
	for _, body := range bodies {
		if err := validateFormulaLookupGuideAck(runtime, "+field-create", body); err != nil {
			return err
		}
	}
	return nil
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
	limit := runtime.Int("limit")
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
		return err
	}
	runtime.Out(map[string]interface{}{"field": data}, nil)
	return nil
}

func executeFieldCreate(runtime *common.RuntimeContext) error {
	bodies, err := parseFieldCreateBodies(newParseCtx(runtime), runtime.Str("json"))
	if err != nil {
		return err
	}
	fields := make([]interface{}, 0, len(bodies))
	var previousStartedAt time.Time
	for idx, body := range bodies {
		if wait := fieldCreateThrottleDelay(previousStartedAt, time.Now()); wait > 0 {
			time.Sleep(wait)
		}
		previousStartedAt = time.Now()
		data, err := baseV3Call(runtime, "POST", baseV3Path("bases", runtime.Str("base-token"), "tables", baseTableID(runtime), "fields"), nil, body)
		if err != nil {
			if len(fields) > 0 {
				return fieldCreatePartialFailure(runtime, bodies, fields, idx, err)
			}
			return err
		}
		fields = append(fields, data)
	}
	if len(fields) == 1 {
		runtime.Out(fieldCreateResult(map[string]interface{}{"field": fields[0], "created": true}, bodies[0]), nil)
		return nil
	}
	runtime.Out(fieldCreateBatchResult(map[string]interface{}{"fields": fields, "created": true, "total": len(fields)}, bodies), nil)
	return nil
}

func fieldCreatePartialFailure(runtime *common.RuntimeContext, bodies []map[string]interface{}, createdFields []interface{}, failedIndex int, err error) error {
	items := make([]map[string]interface{}, 0, len(bodies))
	for idx, field := range createdFields {
		items = append(items, map[string]interface{}{
			"index":  idx,
			"status": "created",
			"field":  fieldCreateOutputIdentity(field, bodies[idx]),
		})
	}

	presented := runtime.PresentError(err)
	failed := map[string]interface{}{
		"index":  failedIndex,
		"status": "failed",
		"field":  fieldCreateInputIdentity(bodies[failedIndex]),
		"error":  presented.Error(),
	}
	if problem, ok := errs.ProblemOf(presented); ok {
		failed["type"] = string(problem.Category)
		failed["subtype"] = string(problem.Subtype)
		failed["retryable"] = problem.Retryable
		if problem.Code != 0 {
			failed["code"] = problem.Code
		}
		if problem.Hint != "" {
			failed["hint"] = problem.Hint
		}
		if problem.LogID != "" {
			failed["log_id"] = problem.LogID
		}
		if problem.Troubleshooter != "" {
			failed["troubleshooter"] = problem.Troubleshooter
		}
	}
	for key, value := range fieldCreateTypedErrorExtensions(presented) {
		failed[key] = value
	}
	items = append(items, failed)

	for idx := failedIndex + 1; idx < len(bodies); idx++ {
		items = append(items, map[string]interface{}{
			"index":  idx,
			"status": "not_attempted",
			"field":  fieldCreateInputIdentity(bodies[idx]),
		})
	}

	result := fieldCreateBatchResult(map[string]interface{}{
		"summary": map[string]interface{}{
			"requested":     len(bodies),
			"attempted":     failedIndex + 1,
			"created":       len(createdFields),
			"failed":        1,
			"not_attempted": len(bodies) - failedIndex - 1,
		},
		"items": items,
		"hint":  "Some fields were already created and were not rolled back. Automatically retry a failed item unchanged only when retryable is true; otherwise follow its hint to authorize or correct the input before resubmitting it. Submit not_attempted items separately.",
	}, bodies[:len(createdFields)])
	result["next_step"] = "inspect_items"
	return runtime.OutPartialFailure(result, nil)
}

func fieldCreateTypedErrorExtensions(err error) map[string]interface{} {
	typed, ok := errs.UnwrapTypedError(err)
	if !ok {
		return nil
	}
	// Built-in typed errors expose JSON-safe extension fields. Keep this
	// projection best-effort so an encoding failure cannot replace the more
	// useful partial-success envelope.
	raw, marshalErr := json.Marshal(typed)
	if marshalErr != nil {
		return nil
	}
	var fields map[string]interface{}
	if unmarshalErr := json.Unmarshal(raw, &fields); unmarshalErr != nil {
		return nil
	}
	for _, key := range []string{"type", "subtype", "code", "message", "hint", "log_id", "troubleshooter", "retryable"} {
		delete(fields, key)
	}
	// These keys belong to the partial-failure ledger. Preserve colliding
	// extension values under a non-conflicting error_ alias instead of letting
	// an extension rewrite the submitted item identity or status.
	for _, key := range []string{"index", "status", "field", "error"} {
		value, exists := fields[key]
		if !exists {
			continue
		}
		alias := "error_" + key
		for {
			if _, conflict := fields[alias]; !conflict {
				break
			}
			alias = "error_" + alias
		}
		fields[alias] = value
		delete(fields, key)
	}
	return fields
}

func fieldCreateInputIdentity(body map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"name": body["name"],
		"type": body["type"],
	}
}

func fieldCreateOutputIdentity(field interface{}, submitted map[string]interface{}) map[string]interface{} {
	identity := fieldCreateInputIdentity(submitted)
	returned, ok := field.(map[string]interface{})
	if !ok {
		return identity
	}
	for _, key := range []string{"id", "name", "type"} {
		if value, exists := returned[key]; exists {
			identity[key] = value
		}
	}
	return identity
}

func parseFieldCreateBodies(pc *parseCtx, raw string) ([]map[string]interface{}, error) {
	bodies, err := parseObjectList(pc, raw, "json")
	if err != nil {
		return nil, err
	}
	if len(bodies) == 0 {
		return nil, baseFlagErrorf("--json must contain at least one field JSON object")
	}
	return bodies, nil
}

func executeFieldUpdate(runtime *common.RuntimeContext) error {
	pc := newParseCtx(runtime)
	baseToken := runtime.Str("base-token")
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
	runtime.Out(fieldUpdateResult(map[string]interface{}{"field": data, "updated": true}, body), nil)
	return nil
}

func fieldCreateResult(result map[string]interface{}, submitted map[string]interface{}) map[string]interface{} {
	readbackRecommended, reason := fieldWriteReadbackRecommendation(submitted, "create")
	return attachFieldReadbackRecommendation(result, readbackRecommended, reason)
}

// fieldCreateBatchResult attaches the same top-level readback contract to a
// multi-field create. It recommends +field-get when any submitted field is a
// computed/linked/generated (or unknown) type, so agents know when to verify
// server state without breaking the existing fields/total structure.
func fieldCreateBatchResult(result map[string]interface{}, submitted []map[string]interface{}) map[string]interface{} {
	recommend := false
	reason := "simple fields created successfully; next_step:done means stop: do not list or get fields unless the user explicitly requests readback or extra properties; if verification is required, filter +field-list with --jq"
	for _, body := range submitted {
		if rec, r := fieldWriteReadbackRecommendation(body, "create"); rec {
			recommend = true
			reason = r
			break
		}
	}
	return attachFieldReadbackRecommendation(result, recommend, reason)
}

func fieldUpdateResult(result map[string]interface{}, submitted map[string]interface{}) map[string]interface{} {
	returnedType := normalizeFieldType(fieldResultType(result["field"]))
	submittedType := normalizeFieldType(common.GetString(submitted, "type"))
	readbackRecommended, reason := fieldUpdateReadbackRecommendation(returnedType, submittedType)
	return attachFieldReadbackRecommendation(result, readbackRecommended, reason)
}

func fieldUpdateReadbackRecommendation(returnedType, submittedType string) (bool, string) {
	if returnedType != "" && submittedType != "" && returnedType != submittedType {
		return true, fmt.Sprintf("field update submitted type %q but the server returned type %q; run +field-get and verify record values before declaring completion", submittedType, returnedType)
	}

	fieldType := returnedType
	if fieldType == "" {
		fieldType = submittedType
	}
	if recommended, reason := fieldTypeReadbackRecommendation(fieldType, "update"); recommended {
		return true, reason + "; sample record values when generated, computed, or converted values are in scope"
	}
	return true, fmt.Sprintf("field update request succeeded for type %q, but +field-update cannot determine the previous type; run +field-get and sample record values if the type changed before declaring completion", fieldType)
}

func attachFieldReadbackRecommendation(result map[string]interface{}, readbackRecommended bool, reason string) map[string]interface{} {
	result["field_get_recommended"] = readbackRecommended
	result["verification_hint"] = reason
	if readbackRecommended {
		result["next_step"] = "field_get"
	} else {
		result["next_step"] = "done"
	}
	return result
}

func fieldWriteReadbackRecommendation(submitted map[string]interface{}, operation string) (bool, string) {
	fieldType := normalizeFieldType(common.GetString(submitted, "type"))
	return fieldTypeReadbackRecommendation(fieldType, operation)
}

func fieldTypeReadbackRecommendation(fieldType, operation string) (bool, string) {
	fieldType = normalizeFieldType(fieldType)
	switch fieldType {
	case "formula", "lookup", "auto_number", "link":
		return true, fmt.Sprintf("computed, linked, or generated field %s should be verified with +field-get before declaring completion", operation)
	case "text", "number", "select", "datetime", "checkbox", "user", "group_chat", "attachment", "location":
		return false, fmt.Sprintf("simple field %s succeeded; next_step:done means stop: do not list or get fields unless the user explicitly requests readback or extra properties; if verification is required, filter +field-list with --jq", operation)
	default:
		return true, "unknown or uncommon field type; run +field-get to avoid assuming the submitted JSON fully describes server state"
	}
}

func normalizeFieldType(fieldType string) string {
	return strings.ToLower(strings.TrimSpace(fieldType))
}

func fieldResultType(value interface{}) string {
	field, ok := value.(map[string]interface{})
	if !ok {
		return ""
	}
	if fieldType := strings.ToLower(strings.TrimSpace(common.GetString(field, "type"))); fieldType != "" {
		return fieldType
	}
	nested, ok := field["field"].(map[string]interface{})
	if !ok {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(common.GetString(nested, "type")))
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
	limit := runtime.Int("limit")
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
