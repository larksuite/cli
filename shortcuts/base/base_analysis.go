// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

const fixedAnalysisOutputFormat = "json"

var BaseSchemaContextGet = common.Shortcut{
	Service:     "base",
	Command:     "+schema-context-get",
	Description: "Get the authorized Base schema context for analysis",
	Risk:        "read",
	Scopes:      []string{"base:table:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		{Name: "table-id", Type: "string_array", Desc: "table ID to include; repeat for multiple tables"},
		analysisLanguageFlag(),
		fixedAnalysisFormatFlag(),
	},
	Tips: []string{
		"The response is a JSON envelope whose data.content contains Markdown schema context.",
		"Omit --table-id to include all tables visible to the current identity.",
		"If the response is truncated, list tables and fields, then retry with only the required --table-id values.",
	},
	Validate: validateSchemaContextGet,
	DryRun:   dryRunSchemaContextGet,
	Execute:  executeSchemaContextGet,
}

var BaseDataSQLQuery = common.Shortcut{
	Service:     "base",
	Command:     "+data-sql-query",
	Description: "Run one read-only SQL query against a Base",
	Risk:        "read",
	Scopes:      []string{"base:record:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		{Name: "sql", Desc: "one read-only SELECT or WITH ... SELECT statement", Required: true, Input: []string{common.File, common.Stdin}},
		analysisLanguageFlag(),
		{Name: "timeout-ms", Type: "int", Desc: "requested SQL execution timeout in milliseconds; the service may enforce a higher minimum; non-positive values are omitted"},
		fixedAnalysisFormatFlag(),
	},
	Tips: []string{
		"Use --sql @query.sql for a file or --sql - to read SQL from stdin.",
		"Prefer explicit columns; SELECT * is supported when all visible fields are genuinely required.",
		"The service enforces SQL syntax and read-only execution.",
		"A truncated response keeps partial rows on stdout with ok=false and exits non-zero; do not use it as a complete result.",
	},
	Validate: validateDataSQLQuery,
	DryRun:   dryRunDataSQLQuery,
	Execute:  executeDataSQLQuery,
}

func analysisLanguageFlag() common.Flag {
	return common.Flag{Name: "language", Default: "zh", Desc: "response language", Enum: []string{"zh", "en", "ja"}}
}

// Declaring format ourselves prevents the framework from adding the public
// json/pretty/table/ndjson/csv surface. The empty Enum also prevents --json
// shorthand injection. Validate keeps even direct/legacy use fixed to JSON.
func fixedAnalysisFormatFlag() common.Flag {
	return common.Flag{Name: "format", Default: fixedAnalysisOutputFormat, Hidden: true, Desc: "fixed output format"}
}

func validateFixedAnalysisFormat(runtime *common.RuntimeContext) error {
	if runtime.Str("format") != fixedAnalysisOutputFormat {
		return baseFlagErrorf("--format is fixed to json for this command")
	}
	return nil
}

func validateSchemaContextGet(_ context.Context, runtime *common.RuntimeContext) error {
	if err := validateFixedAnalysisFormat(runtime); err != nil {
		return err
	}
	for _, tableID := range runtime.StrArray("table-id") {
		if strings.TrimSpace(tableID) == "" {
			return baseFlagErrorf("--table-id cannot be empty")
		}
	}
	return nil
}

func validateDataSQLQuery(_ context.Context, runtime *common.RuntimeContext) error {
	if err := validateFixedAnalysisFormat(runtime); err != nil {
		return err
	}
	return validateBaseSQLInput(runtime.Str("sql"))
}

func schemaContextRequest(runtime *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{
		"format":   "markdown",
		"language": runtime.Str("language"),
	}
	if tableIDs := runtime.StrArray("table-id"); len(tableIDs) > 0 {
		trimmed := make([]string, len(tableIDs))
		for i, tableID := range tableIDs {
			trimmed[i] = strings.TrimSpace(tableID)
		}
		body["table_ids"] = trimmed
	}
	return body
}

func dataSQLRequest(runtime *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{
		"sql":      strings.TrimSpace(runtime.Str("sql")),
		"language": runtime.Str("language"),
	}
	if timeoutMS := runtime.Int("timeout-ms"); timeoutMS > 0 {
		body["timeout_ms"] = timeoutMS
	}
	return body
}

func dryRunSchemaContextGet(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/data/context").
		Body(schemaContextRequest(runtime)).
		Set("base_token", runtime.Str("base-token"))
}

func dryRunDataSQLQuery(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/data/sql").
		Body(dataSQLRequest(runtime)).
		Set("base_token", runtime.Str("base-token"))
}

func executeSchemaContextGet(_ context.Context, runtime *common.RuntimeContext) error {
	payload, err := callAnalysisAPI(runtime, "context", schemaContextRequest(runtime))
	if err != nil {
		return err
	}
	data, partial, err := validateSchemaContextPayload(payload)
	if err != nil {
		return err
	}
	if partial {
		return runtime.OutPartialFailure(data, nil)
	}
	runtime.Out(data, nil)
	return runtime.OutputError()
}

func executeDataSQLQuery(_ context.Context, runtime *common.RuntimeContext) error {
	payload, err := callAnalysisAPI(runtime, "sql", dataSQLRequest(runtime))
	if err != nil {
		return err
	}
	data, partial, err := validateSQLQueryPayload(payload)
	if err != nil {
		return err
	}
	if partial {
		return runtime.OutPartialFailure(data, nil)
	}
	runtime.Out(data, nil)
	return runtime.OutputError()
}

func callAnalysisAPI(runtime *common.RuntimeContext, endpoint string, body map[string]interface{}) (map[string]interface{}, error) {
	result, err := baseV3CallAny(runtime, "POST", baseV3Path("bases", runtime.Str("base-token"), "data", endpoint), nil, body)
	if err != nil {
		return nil, err
	}
	payload, ok := result.(map[string]interface{})
	if !ok || payload == nil {
		return nil, invalidAnalysisResponse("data must be an object")
	}
	return payload, nil
}

func analysisBusinessData(payload map[string]interface{}) (map[string]interface{}, bool, string, error) {
	var response analysisResponsePayload
	if err := decodeAnalysisShape(payload, &response); err != nil {
		return nil, false, "", invalidAnalysisResponse("payload fields have invalid types")
	}
	if response.Success == nil {
		return nil, false, "", invalidAnalysisResponse("success must be a boolean")
	}
	errorMessage := ""
	if !*response.Success {
		if response.ErrorMessage == nil || strings.TrimSpace(*response.ErrorMessage) == "" {
			return nil, false, "", invalidAnalysisResponse("error_msg must be a non-empty string when success is false")
		}
		errorMessage = strings.TrimSpace(*response.ErrorMessage)
	}
	if len(response.Data) == 0 || string(response.Data) == "null" {
		if *response.Success {
			return nil, false, "", invalidAnalysisResponse("data.data must be an object when success is true")
		}
		return nil, false, errorMessage, nil
	}
	var data map[string]interface{}
	if err := decodeAnalysisShape(response.Data, &data); err != nil || data == nil {
		return nil, false, "", invalidAnalysisResponse("data.data must be an object")
	}
	return data, *response.Success, errorMessage, nil
}

func validateSchemaContextPayload(payload map[string]interface{}) (map[string]interface{}, bool, error) {
	data, success, errorMessage, err := analysisBusinessData(payload)
	if err != nil {
		return nil, false, err
	}
	partial, err := analysisPartialFailure(data, success, errorMessage)
	if err != nil {
		return nil, false, err
	}
	if !success && !partial {
		return nil, false, errs.NewAPIError(errs.SubtypeUnknown, "%s", errorMessage)
	}
	if data == nil {
		return nil, false, invalidAnalysisResponse("data.data must be an object")
	}
	var result schemaContextResult
	if err := decodeAnalysisShape(data, &result); err != nil {
		return nil, false, invalidAnalysisResponse("data.data context fields have invalid types")
	}
	if result.Format == nil || *result.Format != "markdown" {
		return nil, false, invalidAnalysisResponse("data.data.format must be markdown")
	}
	if result.Content == nil {
		return nil, false, invalidAnalysisResponse("data.data.content must be a string")
	}
	if result.Truncated == nil {
		return nil, false, invalidAnalysisResponse("data.data.truncated must be a boolean")
	}
	if success && *result.Truncated {
		return nil, false, invalidAnalysisResponse("success=true conflicts with data.data.truncated=true")
	}
	if partial {
		data["error_msg"] = errorMessage
	}
	return data, partial, nil
}

func validateSQLQueryPayload(payload map[string]interface{}) (map[string]interface{}, bool, error) {
	data, success, errorMessage, err := analysisBusinessData(payload)
	if err != nil {
		return nil, false, err
	}
	partial, err := analysisPartialFailure(data, success, errorMessage)
	if err != nil {
		return nil, false, err
	}
	if !success && !partial {
		return nil, false, errs.NewAPIError(errs.SubtypeUnknown, "%s", errorMessage)
	}
	if data == nil {
		return nil, false, invalidAnalysisResponse("data.data must be an object")
	}
	var result sqlQueryResult
	if err := decodeAnalysisShape(data, &result); err != nil {
		return nil, false, invalidAnalysisResponse("data.data SQL fields have invalid types")
	}
	if result.Columns == nil {
		return nil, false, invalidAnalysisResponse("data.data.columns must be an array")
	}
	if result.Rows == nil {
		return nil, false, invalidAnalysisResponse("data.data.rows must be an array")
	}
	for _, row := range *result.Rows {
		if row == nil {
			return nil, false, invalidAnalysisResponse("data.data.rows must contain only objects")
		}
	}
	if result.RowCount == nil || !isNonNegativeInteger(*result.RowCount) {
		return nil, false, invalidAnalysisResponse("data.data.row_count must be a non-negative integer")
	}
	if result.Truncated == nil {
		return nil, false, invalidAnalysisResponse("data.data.truncated must be a boolean")
	}
	if success && *result.Truncated {
		return nil, false, invalidAnalysisResponse("success=true conflicts with data.data.truncated=true")
	}
	if _, exists := data["feedback"]; exists && result.Feedback == nil {
		return nil, false, invalidAnalysisResponse("data.data.feedback must be a string when present")
	}
	if partial {
		data["error_msg"] = errorMessage
	}
	return data, partial, nil
}

func analysisPartialFailure(data map[string]interface{}, success bool, errorMessage string) (bool, error) {
	if success {
		return false, nil
	}
	if data == nil {
		return false, nil
	}
	truncated, exists := data["truncated"]
	if !exists {
		return false, nil
	}
	value, ok := truncated.(bool)
	if !ok {
		return false, invalidAnalysisResponse("data.data.truncated must be a boolean")
	}
	if !value {
		return false, nil
	}
	if errorMessage == "" {
		return false, invalidAnalysisResponse("error_msg must be a non-empty string when success is false")
	}
	return true, nil
}

type analysisResponsePayload struct {
	Success      *bool           `json:"success"`
	Data         json.RawMessage `json:"data"`
	ErrorMessage *string         `json:"error_msg"`
}

type schemaContextResult struct {
	Format    *string `json:"format"`
	Content   *string `json:"content"`
	Truncated *bool   `json:"truncated"`
}

type sqlQueryResult struct {
	Columns   *[]string                 `json:"columns"`
	Rows      *[]map[string]interface{} `json:"rows"`
	RowCount  *json.Number              `json:"row_count"`
	Truncated *bool                     `json:"truncated"`
	Feedback  *string                   `json:"feedback"`
}

func decodeAnalysisShape(source interface{}, target interface{}) error {
	raw, err := json.Marshal(source)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return decoder.Decode(target)
}

func invalidAnalysisResponse(reason string) error {
	return errs.NewInternalError(errs.SubtypeInvalidResponse, "analysis API returned an invalid response: %s", reason)
}

func isNonNegativeInteger(number json.Number) bool {
	parsed, err := strconv.ParseInt(number.String(), 10, 64)
	return err == nil && parsed >= 0
}
