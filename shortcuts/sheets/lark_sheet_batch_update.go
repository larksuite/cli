// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// ─── lark_sheet_batch_update ──────────────────────────────────────────
//
// One tool (batch_update), four shortcuts:
//
//   - +batch-update            user supplies a CLI-shape operations array
//                              [{shortcut, input}, ...]; CLI translates to
//                              MCP shape {tool_name, input(+operation)} via
//                              batchOpDispatch before invoking the tool
//                              (high-risk-write — anything in batchOpDispatch
//                              can be inside)
//   - +cells-batch-set-style   fan a single style across many ranges
//   - +dropdown-update         install/replace the same dropdown across
//                              many ranges in one atomic batch
//   - +dropdown-delete         clear data_validation across many ranges
//                              (high-risk-write)
//
// The tool's contract (post-translation):
//   { excel_id, operations: [{tool_name, input}, ...], continue_on_error? }
//
// continue_on_error defaults to false (fail-fast): execution stops at the
// first failing sub-op, but sub-ops already applied are NOT rolled back —
// the server reports "N succeeded, M failed" and the N stay in the sheet
// (verified against live batches; earlier docs wrongly promised a rollback,
// which made agents resend whole batches and double-apply the successes).
// CLI leaves the default in place for the fan-out shortcuts since they're
// idempotent stamps; only +batch-update lets callers flip it via
// --continue-on-error.

// BatchUpdate accepts a CLI-shape operations array (each item
// {shortcut, input}); on Validate / DryRun / Execute we translate each
// sub-op via batchOpDispatch (see batch_op_dispatch.go) into the MCP
// {tool_name, input(+operation)} form before calling the underlying
// batch_update tool.
var BatchUpdate = common.Shortcut{
	Service:           "sheets",
	Command:           "+batch-update",
	Description:       "Execute a batch of write shortcuts in one request; fail-fast on the first failing sub-op (already-applied sub-ops are NOT rolled back).",
	Risk:              "high-risk-write",
	Scopes:            []string{"sheets:spreadsheet:write_only"},
	ConditionalScopes: []string{"sheets:spreadsheet:read"},
	AuthTypes:         []string{"user", "bot"},
	HasFormat:         true,
	Flags:             flagsFor("+batch-update"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		// Run the full translation in Validate so shape errors surface before
		// DryRun / Execute. Translator is pure (no network), so re-running it
		// in DryRun / Execute below is fine.
		if _, err := buildBatchUpdatePlan(runtime, token); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		plan, _ := buildBatchUpdatePlan(runtime, token)
		dr := invokeToolDryRun(token, ToolKindWrite, "batch_update", plan.input)
		if batchContainsSemanticChartUpdate(runtime) {
			dr.Set("preflight", "execution reads each target chart snapshot before building its partial properties patch")
		}
		if len(plan.localFailures) > 0 {
			dr.Set("local_validation_failures", plan.localFailures)
		}
		if warnings := batchWarnings(runtime); len(warnings) > 0 {
			dr.Set("warning_message", strings.Join(warnings, "\n"))
		}
		return dr
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		plan, err := buildBatchUpdatePlan(runtime, token)
		if err != nil {
			return err
		}
		if err := prepareBatchChartUpdates(ctx, runtime, token, plan); err != nil {
			return err
		}
		for _, w := range batchWarnings(runtime) {
			fmt.Fprintln(runtime.IO().ErrOut, w)
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "batch_update", plan.input)
		if err != nil {
			return err
		}
		if len(plan.localFailures) > 0 {
			out = mergeBatchUpdatePartialOutput(out, plan)
		}
		runtime.Out(compactBatchChartCreateOutput(out), nil)
		return nil
	},
	Tips: []string{
		"high-risk-write: preview with --dry-run, get the user's explicit consent, then re-run with --yes appended — do not pass --yes before the user has confirmed (without it the call exits 10 asking for confirmation).",
		"Execution is fail-fast, NOT transactional: on \"N succeeded, M failed\" the succeeded sub-ops stay applied (no rollback) — fix the failure and resend ONLY the operations from the first failed index onward; resending the whole batch re-applies the succeeded ones. Pass --continue-on-error to keep going past failures instead.",
		"Each sub-op is {shortcut, input}. Do NOT pass input.operation (implied by shortcut name). Repeated input.excel_id / input.spreadsheet_token / input.url fields are ignored; the top-level locator wins.",
		"Chart operations are supported, but prefer +batch-chart-create / +batch-chart-update for chart-only work because their contracts and partial-failure recovery are simpler.",
	},
}

var chartCreateBatchDispatch = map[string]batchOpMapping{
	"+chart-create-basic": {"manage_chart_object", chartCreateBasicInput},
}

var chartUpdateBatchDispatch = map[string]batchOpMapping{
	"+chart-config-update": {"manage_chart_object", chartConfigUpdateInput},
	"+chart-data-update":   {"manage_chart_object", chartDataUpdateInput},
}

var BatchChartCreate = common.Shortcut{
	Service:     "sheets",
	Command:     "+batch-chart-create",
	Description: "Create multiple independent basic charts through one batch request; valid charts continue when another chart fails.",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+batch-chart-create"),
	Tips: []string{
		"Each operation directly contains +chart-create-basic flags such as sheet_name, chart_type, and data_range; do not wrap it in shortcut/input. The legacy wrapped shape remains accepted for compatibility.",
		"--dry-run prints the translated internal MCP body (tool_name / operation / basic_chart) for inspection only; never copy that body back into --operations.",
		"Inspect succeeded, failed, and results after execution. Keep successful charts, retry only failed indexes, then call +chart-list once per affected sheet and repair mismatches with +batch-chart-update instead of deleting/recreating charts.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		_, err = buildChartBatchPlan(runtime, token, chartCreateBatchDispatch, "+batch-chart-create")
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		plan, _ := buildChartBatchPlan(runtime, token, chartCreateBatchDispatch, "+batch-chart-create")
		dryRun := invokeToolDryRun(token, ToolKindWrite, "batch_update", plan.input)
		if len(plan.localFailures) > 0 {
			dryRun.Set("local_validation_failures", plan.localFailures)
		}
		if warnings := batchIgnoredLocatorNotes(runtime, "+batch-chart-create"); len(warnings) > 0 {
			dryRun.Set("warning_message", strings.Join(warnings, "\n"))
		}
		return dryRun
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		plan, err := buildChartBatchPlan(runtime, token, chartCreateBatchDispatch, "+batch-chart-create")
		if err != nil {
			return err
		}
		for _, warning := range batchIgnoredLocatorNotes(runtime, "+batch-chart-create") {
			fmt.Fprintln(runtime.IO().ErrOut, warning)
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "batch_update", plan.input)
		if err != nil {
			return err
		}
		if len(plan.localFailures) > 0 {
			out = mergeBatchUpdatePartialOutput(out, plan)
		}
		runtime.Out(compactBatchChartCreateOutput(out), nil)
		return nil
	},
}

var BatchChartUpdate = common.Shortcut{
	Service:     "sheets",
	Command:     "+batch-chart-update",
	Description: "Update multiple independent chart configurations or data sources through one batch request.",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:read", "sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+batch-chart-update"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		_, err = buildChartBatchPlan(runtime, token, chartUpdateBatchDispatch, "+batch-chart-update")
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		plan, _ := buildChartBatchPlan(runtime, token, chartUpdateBatchDispatch, "+batch-chart-update")
		dryRun := invokeToolDryRun(token, ToolKindWrite, "batch_update", plan.input)
		dryRun.Set("preflight", "execution reads each target chart snapshot before building its partial properties patch")
		if len(plan.localFailures) > 0 {
			dryRun.Set("local_validation_failures", plan.localFailures)
		}
		if warnings := batchIgnoredLocatorNotes(runtime, "+batch-chart-update"); len(warnings) > 0 {
			dryRun.Set("warning_message", strings.Join(warnings, "\n"))
		}
		return dryRun
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		plan, err := buildChartBatchPlan(runtime, token, chartUpdateBatchDispatch, "+batch-chart-update")
		if err != nil {
			return err
		}
		if err := prepareBatchChartUpdates(ctx, runtime, token, plan); err != nil {
			return err
		}
		for _, warning := range batchIgnoredLocatorNotes(runtime, "+batch-chart-update") {
			fmt.Fprintln(runtime.IO().ErrOut, warning)
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "batch_update", plan.input)
		if err != nil {
			return err
		}
		if len(plan.localFailures) > 0 {
			out = mergeBatchUpdatePartialOutput(out, plan)
		}
		runtime.Out(out, nil)
		return nil
	},
}

func buildChartBatchPlan(
	runtime *common.RuntimeContext,
	token string,
	dispatch map[string]batchOpMapping,
	command string,
) (*batchUpdatePlan, error) {
	rawOps, err := parseBatchOperationsFlag(runtime)
	if err != nil {
		return nil, err
	}
	if command == "+batch-chart-create" {
		rawOps, err = normalizeChartCreateBatchOperations(rawOps)
		if err != nil {
			return nil, err
		}
	}
	if len(rawOps) == 0 {
		return nil, sheetsValidationForFlag("operations", "--operations must be a non-empty JSON array")
	}
	if len(rawOps) > maxBatchOperations {
		return nil, sheetsValidationForFlag(
			"operations",
			"--operations accepts at most %d entries; got %d",
			maxBatchOperations,
			len(rawOps),
		)
	}
	continueOnError := true
	if runtime.Changed("continue-on-error") {
		continueOnError = runtime.Bool("continue-on-error")
	}
	translated := make([]interface{}, 0, len(rawOps))
	originalIndexes := make([]int, 0, len(rawOps))
	failures := make([]batchOpTranslationFailure, 0)
	for index, raw := range rawOps {
		item, translateErr := translateBatchOpWithDispatch(raw, token, index, dispatch, command)
		if translateErr != nil {
			shortcut := ""
			if object, ok := raw.(map[string]interface{}); ok {
				shortcut, _ = object["shortcut"].(string)
			}
			failures = append(failures, batchOpTranslationFailure{
				Index:    index,
				Shortcut: shortcut,
				Err:      translateErr,
			})
			continue
		}
		translated = append(translated, item)
		originalIndexes = append(originalIndexes, index)
	}
	if !continueOnError {
		if err := batchOperationFailuresError(failures, len(rawOps)); err != nil {
			return nil, err
		}
	}
	if len(translated) == 0 {
		return nil, batchOperationFailuresError(failures, len(rawOps))
	}
	localFailures := make([]batchLocalValidationFailure, 0, len(failures))
	for _, failure := range failures {
		localFailures = append(localFailures, batchLocalValidationFailure{
			Index:    failure.Index,
			Shortcut: failure.Shortcut,
			Success:  false,
			Stage:    "cli_validation",
			Error:    failure.Err.Error(),
		})
	}
	plan := &batchUpdatePlan{
		input: map[string]interface{}{
			"excel_id":          token,
			"operations":        translated,
			"continue_on_error": continueOnError,
		},
		originalIndexes:      originalIndexes,
		normalizedOperations: normalizedBatchOperations(rawOps, originalIndexes),
		localFailures:        localFailures,
		total:                len(rawOps),
	}
	if err := rejectDuplicateBatchChartTargets(plan, nil); err != nil {
		return nil, err
	}
	return plan, nil
}

func normalizeChartCreateBatchOperations(rawOps []interface{}) ([]interface{}, error) {
	normalized := make([]interface{}, 0, len(rawOps))
	for index, raw := range rawOps {
		op, ok := raw.(map[string]interface{})
		if !ok {
			return nil, sheetsValidationForFlag("operations", "operations[%d] must be a JSON object", index)
		}
		_, hasShortcut := op["shortcut"]
		_, hasInput := op["input"]
		if hasShortcut && hasInput {
			normalized = append(normalized, op)
			continue
		}

		input := make(map[string]interface{}, len(op))
		for key, value := range op {
			if key != "shortcut" && key != "input" {
				input[key] = value
			}
		}
		if hasInput {
			rawInput := op["input"]
			var inputObject map[string]interface{}
			if rawInput != nil {
				inputObject, ok = rawInput.(map[string]interface{})
				if !ok {
					return nil, sheetsValidationForFlag("operations", "operations[%d]: 'input' must be a JSON object (got %T)", index, rawInput)
				}
			}
			input = inputObject
		}
		shortcut := "+chart-create-basic"
		if hasShortcut {
			value, ok := op["shortcut"].(string)
			if !ok || strings.TrimSpace(value) == "" {
				return nil, sheetsValidationForFlag("operations", "operations[%d]: 'shortcut' must be a non-empty string", index)
			}
			shortcut = value
		}
		normalized = append(normalized, map[string]interface{}{
			"shortcut": shortcut,
			"input":    input,
		})
	}
	return normalized, nil
}

func prepareBatchChartUpdates(
	ctx context.Context,
	runtime *common.RuntimeContext,
	token string,
	plan *batchUpdatePlan,
) error {
	if batchChartTargetsNeedSheetLookup(plan) {
		sheetIDsByName, _, err := listSheetIDsByName(ctx, runtime, token)
		if err != nil {
			return err
		}
		if err := rejectDuplicateBatchChartTargets(plan, sheetIDsByName); err != nil {
			return err
		}
	}
	translated, _ := plan.input["operations"].([]interface{})
	continueOnError, _ := plan.input["continue_on_error"].(bool)
	prepared := make([]interface{}, 0, len(translated))
	preparedIndexes := make([]int, 0, len(translated))
	preparedNormalized := make([]batchNormalizedOperation, 0, len(translated))
	for remoteIndex, rawIndex := range plan.originalIndexes {
		normalized := plan.normalizedOperations[remoteIndex]
		shortcut := normalized.shortcut
		if shortcut != "+chart-config-update" && shortcut != "+chart-data-update" {
			prepared = append(prepared, translated[remoteIndex])
			preparedIndexes = append(preparedIndexes, rawIndex)
			preparedNormalized = append(preparedNormalized, normalized)
			continue
		}
		fv := newMapFlagViewForCommand(shortcut, normalized.input)
		sheetID := strings.TrimSpace(fv.Str("sheet-id"))
		sheetName := strings.TrimSpace(fv.Str("sheet-name"))
		chartID := strings.TrimSpace(fv.Str("chart-id"))
		snapshot, err := fetchChartSnapshot(ctx, runtime, token, sheetID, sheetName, chartID)
		if err != nil {
			if !continueOnError {
				return err
			}
			plan.localFailures = append(plan.localFailures, batchLocalValidationFailure{
				Index:    rawIndex,
				Shortcut: shortcut,
				Success:  false,
				Stage:    "cli_preflight",
				Error:    err.Error(),
			})
			continue
		}
		var body map[string]interface{}
		switch shortcut {
		case "+chart-config-update":
			body, _, err = chartConfigUpdateInputFromSnapshot(fv, token, sheetID, sheetName, snapshot)
		case "+chart-data-update":
			body, _, _, _, err = chartDataUpdateInputFromSnapshot(fv, token, sheetID, sheetName, snapshot)
		}
		if err != nil {
			if !continueOnError {
				return err
			}
			plan.localFailures = append(plan.localFailures, batchLocalValidationFailure{
				Index:    rawIndex,
				Shortcut: shortcut,
				Success:  false,
				Stage:    "cli_preflight",
				Error:    err.Error(),
			})
			continue
		}
		item, _ := translated[remoteIndex].(map[string]interface{})
		item["input"] = body
		prepared = append(prepared, item)
		preparedIndexes = append(preparedIndexes, rawIndex)
		preparedNormalized = append(preparedNormalized, normalized)
	}
	if len(prepared) == 0 {
		return sheetsValidationForFlag("operations", "all chart updates failed CLI preflight; no write request was sent")
	}
	plan.input["operations"] = prepared
	plan.originalIndexes = preparedIndexes
	plan.normalizedOperations = preparedNormalized
	return nil
}

func batchContainsSemanticChartUpdate(runtime *common.RuntimeContext) bool {
	rawOps, err := parseBatchOperationsFlag(runtime)
	if err != nil {
		return false
	}
	for _, raw := range rawOps {
		op, _ := raw.(map[string]interface{})
		shortcut, _ := op["shortcut"].(string)
		if shortcut == "+chart-config-update" || shortcut == "+chart-data-update" {
			return true
		}
	}
	return false
}

func compactBatchChartCreateOutput(out interface{}) interface{} {
	root, ok := out.(map[string]interface{})
	if !ok {
		return out
	}
	results, _ := root["results"].([]interface{})
	for _, raw := range results {
		item, _ := raw.(map[string]interface{})
		data, _ := item["data"].(map[string]interface{})
		delete(data, "snapshot")
	}
	return root
}

type batchLocalValidationFailure struct {
	Index    int    `json:"index"`
	Shortcut string `json:"shortcut,omitempty"`
	Success  bool   `json:"success"`
	Stage    string `json:"stage"`
	Error    string `json:"error"`
}

type batchNormalizedOperation struct {
	shortcut string
	input    map[string]interface{}
}

type batchUpdatePlan struct {
	input                map[string]interface{}
	originalIndexes      []int
	normalizedOperations []batchNormalizedOperation
	localFailures        []batchLocalValidationFailure
	total                int
}

func normalizedBatchOperations(rawOps []interface{}, originalIndexes []int) []batchNormalizedOperation {
	operations := make([]batchNormalizedOperation, 0, len(originalIndexes))
	for _, index := range originalIndexes {
		raw, _ := rawOps[index].(map[string]interface{})
		shortcut, _ := raw["shortcut"].(string)
		input, _ := raw["input"].(map[string]interface{})
		operations = append(operations, batchNormalizedOperation{shortcut: shortcut, input: input})
	}
	return operations
}

func buildBatchUpdatePlan(runtime *common.RuntimeContext, token string) (*batchUpdatePlan, error) {
	rawOps, err := parseBatchOperationsFlag(runtime)
	if err != nil {
		return nil, err
	}
	continueOnError := batchContinueOnError(runtime)
	translated, originalIndexes, failures, err := collectBatchOperationTranslations(rawOps, token)
	if err != nil {
		return nil, err
	}
	if !continueOnError {
		if err := batchOperationFailuresError(failures, len(rawOps)); err != nil {
			return nil, err
		}
	}
	if len(translated) == 0 {
		return nil, batchOperationFailuresError(failures, len(rawOps))
	}
	input := map[string]interface{}{
		"excel_id":   token,
		"operations": translated,
	}
	if continueOnError {
		input["continue_on_error"] = true
	}
	localFailures := make([]batchLocalValidationFailure, 0, len(failures))
	for _, failure := range failures {
		localFailures = append(localFailures, batchLocalValidationFailure{
			Index:    failure.Index,
			Shortcut: failure.Shortcut,
			Success:  false,
			Stage:    "cli_validation",
			Error:    failure.Err.Error(),
		})
	}
	plan := &batchUpdatePlan{
		input:                input,
		originalIndexes:      originalIndexes,
		normalizedOperations: normalizedBatchOperations(rawOps, originalIndexes),
		localFailures:        localFailures,
		total:                len(rawOps),
	}
	if err := rejectDuplicateBatchChartTargets(plan, nil); err != nil {
		return nil, err
	}
	return plan, nil
}

func rejectDuplicateBatchChartTargets(plan *batchUpdatePlan, sheetIDsByName map[string]string) error {
	type seenTarget struct {
		index int
		label string
	}
	seen := make(map[string]seenTarget)
	for remoteIndex, operation := range plan.normalizedOperations {
		if operation.shortcut != "+chart-config-update" && operation.shortcut != "+chart-data-update" {
			continue
		}
		fv := newMapFlagViewForCommand(operation.shortcut, operation.input)
		chartID := strings.TrimSpace(fv.Str("chart-id"))
		sheetID := strings.TrimSpace(fv.Str("sheet-id"))
		sheetName := strings.TrimSpace(fv.Str("sheet-name"))
		if chartID == "" || (sheetID == "" && sheetName == "") {
			continue // The operation translator reports missing required fields.
		}
		selectorKey := "id:" + sheetID
		selectorLabel := "sheet-id " + strconv.Quote(sheetID)
		if sheetID == "" {
			selectorKey = "name:" + sheetName
			selectorLabel = "sheet-name " + strconv.Quote(sheetName)
			if resolvedID := sheetIDsByName[sheetName]; resolvedID != "" {
				selectorKey = "id:" + resolvedID
			}
		}
		key := selectorKey + "\x00" + chartID
		originalIndex := plan.originalIndexes[remoteIndex]
		if previous, ok := seen[key]; ok {
			return sheetsValidationForFlag(
				"operations",
				"operations[%d] and operations[%d] both target chart %q on %s; send duplicate chart targets in separate batch calls so each update reads the latest snapshot",
				previous.index,
				originalIndex,
				chartID,
				previous.label,
			)
		}
		seen[key] = seenTarget{index: originalIndex, label: selectorLabel}
	}
	return nil
}

func batchChartTargetsNeedSheetLookup(plan *batchUpdatePlan) bool {
	selectorKinds := make(map[string]uint8)
	for _, operation := range plan.normalizedOperations {
		if operation.shortcut != "+chart-config-update" && operation.shortcut != "+chart-data-update" {
			continue
		}
		fv := newMapFlagViewForCommand(operation.shortcut, operation.input)
		chartID := strings.TrimSpace(fv.Str("chart-id"))
		if chartID == "" {
			continue
		}
		if strings.TrimSpace(fv.Str("sheet-id")) != "" {
			selectorKinds[chartID] |= 1
		} else if strings.TrimSpace(fv.Str("sheet-name")) != "" {
			selectorKinds[chartID] |= 2
		}
		if selectorKinds[chartID] == 3 {
			return true
		}
	}
	return false
}

func batchContinueOnError(runtime *common.RuntimeContext) bool {
	if runtime.Changed("continue-on-error") {
		// An explicit false wins over the envelope.
		return runtime.Bool("continue-on-error")
	}
	if envelope, _ := parseJSONFlag(runtime, "operations"); envelope != nil {
		if m, ok := envelope.(map[string]interface{}); ok {
			if v, ok := m["continue_on_error"].(bool); ok {
				return v
			}
		}
	}
	return false
}

// mergeBatchUpdatePartialOutput restores original operation indexes after the
// CLI omitted locally invalid operations from the server request, then appends
// the local failures to the same result list. The command exits successfully
// when the server preserved at least one valid operation, but the output still
// makes every failed operation explicit.
func mergeBatchUpdatePartialOutput(out interface{}, plan *batchUpdatePlan) interface{} {
	merged := map[string]interface{}{}
	if remote, ok := out.(map[string]interface{}); ok {
		for key, value := range remote {
			merged[key] = value
		}
	} else if out != nil {
		merged["tool_output"] = out
	}

	results := make([]interface{}, 0, plan.total)
	if remoteResults, ok := merged["results"].([]interface{}); ok {
		for _, raw := range remoteResults {
			item, ok := raw.(map[string]interface{})
			if !ok {
				results = append(results, raw)
				continue
			}
			copied := make(map[string]interface{}, len(item))
			for key, value := range item {
				copied[key] = value
			}
			if remoteIndex, ok := batchResultIndex(copied["index"]); ok &&
				remoteIndex >= 0 && remoteIndex < len(plan.originalIndexes) {
				copied["index"] = plan.originalIndexes[remoteIndex]
			}
			results = append(results, copied)
		}
	}
	for _, failure := range plan.localFailures {
		results = append(results, map[string]interface{}{
			"index":    failure.Index,
			"shortcut": failure.Shortcut,
			"success":  false,
			"stage":    failure.Stage,
			"error":    failure.Error,
		})
	}
	sort.SliceStable(results, func(i, j int) bool {
		left, leftOK := batchResultItemIndex(results[i])
		right, rightOK := batchResultItemIndex(results[j])
		return leftOK && rightOK && left < right
	})

	succeeded := batchResultCount(merged["succeeded"])
	remoteFailed := batchResultCount(merged["failed"])
	failed := remoteFailed + len(plan.localFailures)
	merged["total"] = plan.total
	merged["succeeded"] = succeeded
	merged["failed"] = failed
	merged["results"] = results
	merged["local_validation_failures"] = plan.localFailures
	merged["message"] = fmt.Sprintf("batch_update: %d succeeded, %d failed", succeeded, failed)
	return merged
}

func batchResultIndex(value interface{}) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), typed == float64(int(typed))
	default:
		return 0, false
	}
}

func batchResultItemIndex(value interface{}) (int, bool) {
	item, ok := value.(map[string]interface{})
	if !ok {
		return 0, false
	}
	return batchResultIndex(item["index"])
}

func batchResultCount(value interface{}) int {
	count, _ := batchResultIndex(value)
	return count
}

// batchNeedsDimInsertBeforeStyleWarning reports whether any +dim-insert sub-op
// requests --inherit-style before at the first row/column, where the
// preceding-side style cannot be copied (no preceding row/column exists).
// batchWarnings collects the advisory notes a batch surfaces before it runs,
// in one place so DryRun and Execute cannot drift apart on which ones they
// report.
func batchWarnings(runtime *common.RuntimeContext) []string {
	var out []string
	out = append(out, batchIgnoredLocatorNotes(runtime, "+batch-update")...)
	if batchNeedsDimInsertBeforeStyleWarning(runtime) {
		out = append(out, dimInsertBeforeStyleWarning)
	}
	out = append(out, batchCollidingDimFreezeNotes(runtime)...)
	return append(out, batchLegacyDimFreezeNotes(runtime)...)
}

// batchIgnoredLocatorNotes makes the translator's intentional locator rewrite
// visible. Without this note, a caller can pass a different spreadsheet in a
// sub-op and mistake the ignored value for the actual target.
func batchIgnoredLocatorNotes(runtime *common.RuntimeContext, command string) []string {
	rawOps, err := parseBatchOperationsFlag(runtime)
	if err != nil {
		return nil // a malformed --operations is the translator's to report.
	}
	if command == "+batch-chart-create" {
		rawOps, err = normalizeChartCreateBatchOperations(rawOps)
		if err != nil {
			return nil // a malformed --operations is the translator's to report.
		}
	}
	var notes []string
	for i, raw := range rawOps {
		op, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		input, _ := op["input"].(map[string]interface{})
		if input == nil {
			continue
		}
		var ignored []string
		for userKey := range input {
			if isReservedSubOpKey(userKey) {
				ignored = append(ignored, userKey)
			}
		}
		if len(ignored) == 0 {
			continue
		}
		sort.Strings(ignored)
		shortcut, _ := op["shortcut"].(string)
		notes = append(notes, fmt.Sprintf(
			"warning: operations[%d] (%s) ignored input locator keys %s; the top-level %s --url/--spreadsheet-token locator is authoritative",
			i, shortcut, strings.Join(ignored, ", "), command))
	}
	return notes
}

// batchCollidingDimFreezeNotes reports +dim-freeze sub-ops that target the SAME
// sheet more than once. Freeze is full-state replacement, so each of them
// discards the previous one and only the last survives — both still report
// success, which is exactly why the mistake goes unnoticed. The CLI has already
// walked the whole ops array by this point, so it can name the survivor and the
// single sub-op that holds everything the caller clearly meant to hold.
//
// A batch cannot read current state, and +styles-put (the other combined-freeze
// carrier) is not batchable, so folding into ONE sub-op is the only fix — hence
// a note rather than a suggestion to reorder.
func batchCollidingDimFreezeNotes(runtime *common.RuntimeContext) []string {
	rawOps, err := parseBatchOperationsFlag(runtime)
	if err != nil {
		return nil // a malformed --operations is the translator's to report.
	}
	type freezeOp struct {
		index      int
		rows, cols int
	}
	// Keyed by the sub-op's sheet selector: freezes on different sheets are
	// independent. Order of first appearance keeps the notes deterministic.
	bySheet := map[string][]freezeOp{}
	var order []string
	for i, raw := range rawOps {
		op, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if sc, _ := op["shortcut"].(string); sc != "+dim-freeze" {
			continue
		}
		input, _ := op["input"].(map[string]interface{})
		if input == nil {
			continue
		}
		fv := newMapFlagViewForCommand("+dim-freeze", input)
		rows, cols, ok := dimFreezeAxes(fv)
		if !ok {
			continue // an unusable sub-op is the translator's to report.
		}
		key := strings.TrimSpace(fv.Str("sheet-id")) + "\x00" + strings.TrimSpace(fv.Str("sheet-name"))
		if _, seen := bySheet[key]; !seen {
			order = append(order, key)
		}
		bySheet[key] = append(bySheet[key], freezeOp{index: i, rows: rows, cols: cols})
	}

	var notes []string
	for _, key := range order {
		ops := bySheet[key]
		if len(ops) < 2 {
			continue
		}
		indexes := make([]string, 0, len(ops))
		// The combined state is what the caller almost certainly meant: keep the
		// last positive value named for each axis. An axis nobody ever freezes
		// stays 0, so a deliberate "unfreeze everything" batch still renders as
		// --rows 0 --cols 0 rather than inventing a freeze.
		combinedRows, combinedCols := 0, 0
		for _, op := range ops {
			indexes = append(indexes, fmt.Sprintf("operations[%d]", op.index))
			if op.rows > 0 {
				combinedRows = op.rows
			}
			if op.cols > 0 {
				combinedCols = op.cols
			}
		}
		last := ops[len(ops)-1]
		notes = append(notes, fmt.Sprintf(
			"warning: %s are all +dim-freeze on the same sheet — freeze replaces the WHOLE state, so each one discards the previous and only %s survives (ending at %s). They all report success. Replace them with ONE sub-op: %s",
			strings.Join(indexes, ", "),
			indexes[len(indexes)-1],
			dimFreezeSpelling(last.rows, last.cols),
			dimFreezeSpelling(combinedRows, combinedCols)))
	}
	return notes
}

// batchLegacyDimFreezeNotes steers +dim-freeze sub-ops still written in the
// deprecated --dimension/--count form (see DEPRECATED(phase-2) on
// dimFreezeLegacyNote). The standalone command prints that note from its own
// DryRun/Execute, which a sub-op never reaches — yet the batch is where the
// legacy form does the most damage: freeze is full-state replacement, so two
// per-axis sub-ops both report success while only the last axis stays frozen,
// and +styles-put (the other way to set both axes) is not batchable. The
// wording comes from the shared helper, so it cannot drift from the standalone
// one.
func batchLegacyDimFreezeNotes(runtime *common.RuntimeContext) []string {
	rawOps, err := parseBatchOperationsFlag(runtime)
	if err != nil {
		return nil // a malformed --operations is the translator's to report.
	}
	var notes []string
	for i, raw := range rawOps {
		op, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if sc, _ := op["shortcut"].(string); sc != "+dim-freeze" {
			continue
		}
		input, _ := op["input"].(map[string]interface{})
		if input == nil {
			continue
		}
		if note := dimFreezeLegacyNote(newMapFlagViewForCommand("+dim-freeze", input)); note != "" {
			notes = append(notes, fmt.Sprintf("operations[%d] (+dim-freeze): %s", i, note))
		}
	}
	return notes
}

func batchNeedsDimInsertBeforeStyleWarning(runtime *common.RuntimeContext) bool {
	rawOps, err := parseBatchOperationsFlag(runtime)
	if err != nil {
		return false
	}
	for _, raw := range rawOps {
		op, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		sc, _ := op["shortcut"].(string)
		if sc != "+dim-insert" {
			continue
		}
		input, _ := op["input"].(map[string]interface{})
		isBefore := false
		for _, key := range []string{"inherit-style", "inherit_style", "inheritStyle"} {
			if v, _ := input[key].(string); strings.EqualFold(v, "before") {
				isBefore = true
				break
			}
		}
		if !isBefore {
			continue
		}
		posRaw, hasPos := input["position"]
		if !hasPos {
			continue
		}
		// Warn only at the first row/column (idx 0).
		if _, idx, err := parseA1Position(strings.TrimSpace(fmt.Sprintf("%v", posRaw))); err == nil && idx == 0 {
			return true
		}
	}
	return false
}

// parseBatchOperationsFlag accepts --operations as either a JSON array (the
// operations list directly) or an envelope object { operations, continue_on_error }
// for back-compat with the legacy --data shape. Returns the operations array.
func parseBatchOperationsFlag(runtime *common.RuntimeContext) ([]interface{}, error) {
	v, err := parseJSONFlag(runtime, "operations")
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, sheetsValidationForFlag("operations", "--operations is required")
	}
	if arr, ok := v.([]interface{}); ok {
		return arr, nil
	}
	if m, ok := v.(map[string]interface{}); ok {
		if ops, ok := m["operations"].([]interface{}); ok {
			return ops, nil
		}
	}
	return nil, sheetsValidationForFlag("operations", "--operations must be a JSON array (or { operations: [...] } envelope)")
}

// CellsBatchSetStyle stamps one style block across many sheet-prefixed
// ranges atomically. --ranges is a JSON array of sheet-prefixed A1
// strings; the style is composed from the same flat flags as
// +cells-set-style. CLI fans each range into a separate set_cell_range
// op inside one batch_update.
var CellsBatchSetStyle = common.Shortcut{
	Service:     "sheets",
	Command:     "+cells-batch-set-style",
	Description: "Apply one style block to many sheet-prefixed ranges in one batch request (fail-fast, no rollback).",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+cells-batch-set-style"),
	Tips: []string{
		"DEPRECATED: superseded by +styles-put, whose one spec also covers merges, row/col sizes and freeze — prefer it for new work.",
		`Example: lark-cli sheets +cells-batch-set-style --url <URL> --ranges '["Sheet1!A1:B2","汇总!C1:C9"]' --font-weight bold`,
		"Every range carries its sheet-NAME prefix (Sheet1!A1:B2, not a sheet_id) — there is no --sheet-id / --sheet-name flag here.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if _, err := validateDropdownRanges(runtime); err != nil {
			return err
		}
		if err := requireAnyStyleFlag(runtime); err != nil {
			return err
		}
		if _, err := borderStylesFromFlag(runtime); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		input, _ := cellsBatchSetStyleInput(runtime, token)
		return invokeToolDryRun(token, ToolKindWrite, "batch_update", input)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		input, err := cellsBatchSetStyleInput(runtime, token)
		if err != nil {
			return err
		}
		// DEPRECATED(phase-2): +cells-batch-set-style — replaced by +styles-put.
		// Phase 1 (here): the command keeps working and is already retired from
		// the skill docs via bundle.json doc_hidden_shortcuts in
		// sheet-skill-spec; steer new usage to the superset in-band.
		// Phase 2 removal: drop the shortcut from spec-tables + its
		// doc_hidden_shortcuts entry, then this command and its input builder.
		fmt.Fprintln(runtime.IO().ErrOut,
			"note: +cells-batch-set-style is superseded by +styles-put (one spec covers styles + merges + row/col sizes + freeze); prefer +styles-put for new work")
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "batch_update", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

func cellsBatchSetStyleInput(runtime *common.RuntimeContext, token string) (map[string]interface{}, error) {
	ranges, err := validateDropdownRanges(runtime)
	if err != nil {
		return nil, err
	}
	cellStyle := buildCellStyleFromFlags(runtime)
	borderStyles, err := borderStylesFromFlag(runtime)
	if err != nil {
		return nil, err
	}
	prototype := map[string]interface{}{}
	if len(cellStyle) > 0 {
		prototype["cell_styles"] = cellStyle
	}
	if borderStyles != nil {
		prototype["border_styles"] = borderStyles
	}
	ops := make([]interface{}, 0, len(ranges))
	var totalCells int64
	for _, rng := range ranges {
		sheet, sub, err := splitSheetPrefixedRange(rng)
		if err != nil {
			return nil, err
		}
		rows, cols, err := rangeDimensions(sub)
		if err != nil {
			return nil, sheetsValidationForFlag("range", "range %q: %v", rng, err)
		}
		if err := checkStampMatrixBudget("ranges", rng, rows, cols); err != nil {
			return nil, err
		}
		totalCells += int64(rows) * int64(cols)
		if err := checkBatchStampBudget("ranges", totalCells); err != nil {
			return nil, err
		}
		cells := fillCellsMatrix(rows, cols, prototype)
		ops = append(ops, map[string]interface{}{
			"tool_name": "set_cell_range",
			"input": map[string]interface{}{
				"excel_id":   token,
				"sheet_name": sheet,
				"range":      sub,
				"cells":      cells,
			},
		})
	}
	return map[string]interface{}{
		"excel_id":   token,
		"operations": ops,
	}, nil
}

// CellsBatchClear clears content / formats / both across many sheet-prefixed
// ranges in one atomic batch. --ranges is a JSON array of sheet-prefixed A1
// strings; --scope reuses the +cells-clear vocabulary (content / formats /
// all). CLI fans each range into a separate clear_cell_range op inside one
// batch_update. high-risk-write because clear is irreversible.
var CellsBatchClear = common.Shortcut{
	Service:     "sheets",
	Command:     "+cells-batch-clear",
	Description: "Clear content/formats across many sheet-prefixed ranges in one batch request (irreversible; fail-fast, no rollback).",
	Risk:        "high-risk-write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+cells-batch-clear"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if _, err := validateDropdownRanges(runtime); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		input, _ := cellsBatchClearInput(runtime, token)
		return invokeToolDryRun(token, ToolKindWrite, "batch_update", input)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		input, err := cellsBatchClearInput(runtime, token)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "batch_update", input)
		if err != nil {
			return annotateEmbeddedBlockClearErr(err)
		}
		runtime.Out(out, nil)
		return nil
	},
	Tips: []string{
		"high-risk-write — always preview with --dry-run; clear is not undoable.",
		"Every --ranges item must carry a sheet prefix (e.g. \"Sheet1!A1:A10\"); all ranges are cleared with the same --scope.",
		"Can't delete an embedded pivot/chart by clearing cells — remove the object itself with +pivot-delete / +chart-delete.",
	},
}

func cellsBatchClearInput(runtime *common.RuntimeContext, token string) (map[string]interface{}, error) {
	ranges, err := validateDropdownRanges(runtime)
	if err != nil {
		return nil, err
	}
	clearType := normalizeClearType(runtime.Str("scope"))
	ops := make([]interface{}, 0, len(ranges))
	for _, rng := range ranges {
		sheet, sub, err := splitSheetPrefixedRange(rng)
		if err != nil {
			return nil, err
		}
		ops = append(ops, map[string]interface{}{
			"tool_name": "clear_cell_range",
			"input": map[string]interface{}{
				"excel_id":   token,
				"sheet_name": sheet,
				"range":      sub,
				"clear_type": clearType,
			},
		})
	}
	return map[string]interface{}{
		"excel_id":   token,
		"operations": ops,
	}, nil
}

// DropdownUpdate installs/replaces a single dropdown on many ranges in one
// atomic batch. Sheet ids come from the per-range sheet prefix.
var DropdownUpdate = common.Shortcut{
	Service:     "sheets",
	Command:     "+dropdown-update",
	Description: "Install or replace one dropdown across many sheet-prefixed ranges in one batch request (fail-fast, no rollback).",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+dropdown-update"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if _, err := validateDropdownRanges(runtime); err != nil {
			return err
		}
		if _, err := validateDropdownSourceOrOptions(runtime); err != nil {
			return err
		}
		warnDropdownSourceRangeHighlight(runtime)
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		input, _ := dropdownBatchInput(runtime, token, false)
		return invokeToolDryRun(token, ToolKindWrite, "batch_update", input)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		input, err := dropdownBatchInput(runtime, token, false)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "batch_update", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

// DropdownDelete clears data_validation across many ranges atomically.
var DropdownDelete = common.Shortcut{
	Service:     "sheets",
	Command:     "+dropdown-delete",
	Description: "Clear dropdowns from many sheet-prefixed ranges in one batch request (irreversible; fail-fast, no rollback).",
	Risk:        "high-risk-write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+dropdown-delete"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		// validateDropdownRanges enforces the shared maxBatchRanges cap.
		if _, err := validateDropdownRanges(runtime); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		input, _ := dropdownBatchInput(runtime, token, true)
		return invokeToolDryRun(token, ToolKindWrite, "batch_update", input)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		input, err := dropdownBatchInput(runtime, token, true)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "batch_update", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

// dropdownBatchInput builds the batch_update payload for both
// +dropdown-update (clear=false, data_validation populated) and
// +dropdown-delete (clear=true, data_validation: null).
func dropdownBatchInput(runtime *common.RuntimeContext, token string, clear bool) (map[string]interface{}, error) {
	ranges, err := validateDropdownRanges(runtime)
	if err != nil {
		return nil, err
	}
	var prototype map[string]interface{}
	if clear {
		prototype = map[string]interface{}{"data_validation": nil}
	} else {
		validation, err := buildDropdownValidation(runtime)
		if err != nil {
			return nil, err
		}
		prototype = map[string]interface{}{"data_validation": validation}
	}
	ops := make([]interface{}, 0, len(ranges))
	var totalCells int64
	for _, rng := range ranges {
		sheet, sub, err := splitSheetPrefixedRange(rng)
		if err != nil {
			return nil, err
		}
		rows, cols, err := rangeDimensions(sub)
		if err != nil {
			return nil, sheetsValidationForFlag("range", "range %q: %v", rng, err)
		}
		if err := checkStampMatrixBudget("ranges", rng, rows, cols); err != nil {
			return nil, err
		}
		totalCells += int64(rows) * int64(cols)
		if err := checkBatchStampBudget("ranges", totalCells); err != nil {
			return nil, err
		}
		cells := fillCellsMatrix(rows, cols, prototype)
		ops = append(ops, map[string]interface{}{
			"tool_name": "set_cell_range",
			"input": map[string]interface{}{
				"excel_id":   token,
				"sheet_name": sheet,
				"range":      sub,
				"cells":      cells,
			},
		})
	}
	return map[string]interface{}{
		"excel_id":   token,
		"operations": ops,
	}, nil
}

// ─── helpers resurrected from B3 (used here + future skills) ──────────

// maxBatchRanges caps how many ranges a fan-out batch (+cells-batch-set-style /
// +cells-batch-clear / +dropdown-update / +dropdown-delete) may carry, bounding
// the number of ops materialized into one batch_update.
const maxBatchRanges = 100

// checkBatchStampBudget rejects a fan-out batch whose ranges materialize more
// than maxStampMatrixCells cells in aggregate. A batch builds every range's
// cells matrix up front, so the SUM across ranges is the real peak-memory bound
// — the per-range checkStampMatrixBudget alone can't stop many ranges from
// summing past it. totalCells is int64 to stay overflow-safe.
func checkBatchStampBudget(flagName string, totalCells int64) error {
	if totalCells > maxStampMatrixCells {
		return sheetsValidationForFlag(flagName,
			"the request expands to %d cells total, over the %d-cell safety cap; reduce the number or size of ranges",
			totalCells, maxStampMatrixCells)
	}
	return nil
}

// validateDropdownRanges parses --ranges, requires every entry to carry a
// sheet prefix, and returns the parsed list.
func validateDropdownRanges(runtime *common.RuntimeContext) ([]string, error) {
	raw, err := requireJSONArray(runtime, "ranges")
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(raw))
	for i, v := range raw {
		s, ok := v.(string)
		if !ok {
			return nil, sheetsValidationForFlag("ranges", "--ranges[%d] must be a string", i)
		}
		s = strings.TrimSpace(s)
		// scanSheetQualifier rather than a literal "!" scan: the separator has
		// four equal spellings, so a full-width "工作表1！A1:B2" does carry a
		// prefix and must not be told it doesn't.
		if _, _, ok := scanSheetQualifier(s); !ok {
			return nil, sheetsValidationForFlag("ranges", "--ranges[%d] (%q) must include a sheet prefix", i, s)
		}
		// Validate the sheet!range shape up front so malformed entries like
		// "!A1" (no sheet), "Sheet1!" (no range) or "Sheet1!bad" (bad ref) fail
		// here at Validate instead of slipping through to DryRun/Execute.
		_, sub, err := splitSheetPrefixedRange(s)
		if err != nil {
			return nil, sheetsValidationForFlag("ranges", "--ranges[%d]: %v", i, err)
		}
		if _, _, err := rangeDimensions(sub); err != nil {
			return nil, sheetsValidationForFlag("ranges", "--ranges[%d] (%q): %v", i, s, err)
		}
		out = append(out, s)
	}
	if len(out) > maxBatchRanges {
		return nil, sheetsValidationForFlag("ranges", "--ranges accepts at most %d entries; got %d", maxBatchRanges, len(out))
	}
	return out, nil
}

// splitSheetPrefixedRange splits "sheet1!A2:A100" into ("sheet1", "A2:A100").
//
// The grammar is splitRangeSheetPrefix's, so every --ranges item parses the way
// the same prefix does in --range: the full-width and backslash-escaped
// separators count, and a quoted name is unwrapped. The sheet returned here
// goes straight into a sub-op's "sheet_name", so keeping the quotes would ship
// a name the backend cannot find ('My Sheet' instead of My Sheet).
//
// err distinguishes only the malformed cases (empty side); "no prefix at all"
// is the caller's own check, which names the flag and the item index.
func splitSheetPrefixedRange(rng string) (sheet, sub string, err error) {
	sheet, sub, ok := splitRangeSheetPrefix(rng)
	if !ok {
		return "", "", sheetsValidationForFlag("range", "range %q must use sheet!range form", rng)
	}
	return sheet, sub, nil
}
