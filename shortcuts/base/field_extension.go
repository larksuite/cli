// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

const (
	fieldExtensionReadScope        = "base:field:read"
	fieldExtensionUpdateScope      = "base:field:update"
	fieldExtensionUpdateCellsScope = "base:record:update"

	fieldExtensionIDBuiltinLLMCompletion = "builtin_llm_completion"
	fieldExtensionPromptTypeText         = "text"
	fieldExtensionPromptTypeFieldRef     = "field_ref"
)

type fieldExtensionClearRequest struct{}

type fieldExtensionUpdateRequest struct {
	ExtensionID string                         `json:"extension_id"`
	Inputs      *fieldExtensionCompletionInput `json:"inputs"`
}

type fieldExtensionCompletionInput struct {
	Prompt []fieldExtensionPromptSegment `json:"prompt"`
}

type fieldExtensionPromptSegment struct {
	Type  string  `json:"type"`
	Text  *string `json:"text,omitempty"`
	Field *string `json:"field,omitempty"`
}

var BaseFieldExtensionGet = common.Shortcut{
	Service:     "base",
	Command:     "+field-extension-get",
	Description: "Get a field extension configuration",
	Risk:        "read",
	Scopes:      []string{fieldExtensionReadScope},
	AuthTypes:   authTypes(),
	Flags:       []common.Flag{baseTokenFlag(true), tableRefFlag(true), fieldRefFlag(true)},
	Tips: []string{
		`Example: lark-cli base +field-extension-get --base-token <base_token> --table-id <table_id> --field-id <field_id>`,
		"Returns current_extension; null means the field has no recognizable extension configuration.",
	},
	DryRun: dryRunFieldExtensionGet,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeFieldExtensionGet(runtime)
	},
}

var BaseFieldExtensionUpdate = common.Shortcut{
	Service:     "base",
	Command:     "+field-extension-update",
	Description: "Install, update, or clear a field extension",
	Risk:        "high-risk-write",
	Scopes:      []string{fieldExtensionUpdateScope},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		fieldRefFlag(true),
		{Name: "json", Desc: `field extension JSON object; use {} to clear the extension`, Required: true},
	},
	Tips: []string{
		baseHighRiskYesTip,
		`Example update: lark-cli base +field-extension-update --base-token <base_token> --table-id <table_id> --field-id <field_id> --json '{"extension_id":"builtin_llm_completion","inputs":{"prompt":[{"type":"text","text":"Summarize "},{"type":"field_ref","field":"Description"}]}}' --yes`,
		`Example clear: lark-cli base +field-extension-update --base-token <base_token> --table-id <table_id> --field-id <field_id> --json '{}' --yes`,
		"Read lark-base-field-extension.md before constructing builtin_llm_completion prompt JSON.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateFieldExtensionUpdate(runtime)
	},
	DryRun: dryRunFieldExtensionUpdate,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeFieldExtensionUpdate(runtime)
	},
}

var BaseFieldExtensionUpdateCells = common.Shortcut{
	Service:     "base",
	Command:     "+field-extension-update-cells",
	Description: "Start a field extension cell update task",
	Risk:        "high-risk-write",
	Scopes:      []string{fieldExtensionUpdateCellsScope},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		fieldRefFlag(true),
		{Name: "type", Desc: "update range: column or row", Required: true, Enum: []string{"column", "row"}},
		viewRefFlag(false),
		{Name: "record-id", Type: "string_array", Desc: "record ID (repeatable); required when --type row"},
	},
	Tips: []string{
		baseHighRiskYesTip,
		`Example column: lark-cli base +field-extension-update-cells --base-token <base_token> --table-id <table_id> --field-id <field_id> --type column --view-id <view_id> --yes`,
		`Example row: lark-cli base +field-extension-update-cells --base-token <base_token> --table-id <table_id> --field-id <field_id> --type row --record-id <record_id_1> --record-id <record_id_2> --yes`,
		"Column updates may touch every visible record in the selected view; row updates must pass explicit record IDs.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateFieldExtensionUpdateCells(runtime)
	},
	DryRun: dryRunFieldExtensionUpdateCells,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeFieldExtensionUpdateCells(runtime)
	},
}

func fieldExtensionPath(runtime *common.RuntimeContext) string {
	return baseV3Path("bases", runtime.Str("base-token"), "tables", baseTableID(runtime), "fields", runtime.Str("field-id"), "field_extensions")
}

func dryRunFieldExtensionGet(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		GET("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields/:field_id/field_extensions").
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime)).
		Set("field_id", runtime.Str("field-id"))
}

func dryRunFieldExtensionUpdate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	body, err := parseFieldExtensionUpdateBody(runtime)
	if err != nil {
		return common.NewDryRunAPI().Desc(fmt.Sprintf("dry-run validation failed: %v", err))
	}
	return common.NewDryRunAPI().
		PUT("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields/:field_id/field_extensions").
		Body(body).
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime)).
		Set("field_id", runtime.Str("field-id"))
}

func dryRunFieldExtensionUpdateCells(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	body, err := fieldExtensionUpdateCellsBody(runtime)
	if err != nil {
		return common.NewDryRunAPI().Desc(fmt.Sprintf("dry-run validation failed: %v", err))
	}
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields/:field_id/field_extensions/update_cells").
		Body(body).
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime)).
		Set("field_id", runtime.Str("field-id"))
}

func validateFieldExtensionUpdate(runtime *common.RuntimeContext) error {
	_, err := parseFieldExtensionUpdateBody(runtime)
	return err
}

func validateFieldExtensionUpdateCells(runtime *common.RuntimeContext) error {
	_, err := fieldExtensionUpdateCellsBody(runtime)
	return err
}

func parseFieldExtensionUpdateBody(runtime *common.RuntimeContext) (interface{}, error) {
	raw, err := loadJSONInput(newParseCtx(runtime), runtime.Str("json"), "json")
	if err != nil {
		return nil, err
	}

	var keys map[string]json.RawMessage
	if err := decodeFieldExtensionJSON(raw, &keys); err != nil {
		return nil, err
	}
	if keys == nil {
		return nil, baseFlagErrorf("--json must be a JSON object; %s", jsonInputTip("json"))
	}
	if len(keys) == 0 {
		return fieldExtensionClearRequest{}, nil
	}

	var req fieldExtensionUpdateRequest
	if err := decodeFieldExtensionJSON(raw, &req); err != nil {
		return nil, err
	}
	if err := validateFieldExtensionUpdateRequest(req); err != nil {
		return nil, err
	}
	return req, nil
}

func decodeFieldExtensionJSON(raw string, dst interface{}) error {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return formatJSONError("json", "field extension", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return baseFlagErrorf("--json must contain exactly one JSON object; %s", jsonInputTip("json"))
	}
	return nil
}

func validateFieldExtensionUpdateRequest(req fieldExtensionUpdateRequest) error {
	if req.ExtensionID == "" {
		return baseFlagErrorf("--json.extension_id must be %q, or use {} to clear the extension", fieldExtensionIDBuiltinLLMCompletion)
	}
	if req.ExtensionID != fieldExtensionIDBuiltinLLMCompletion {
		return baseFlagErrorf("--json.extension_id must be %q; got %q", fieldExtensionIDBuiltinLLMCompletion, req.ExtensionID)
	}
	if req.Inputs == nil {
		return baseFlagErrorf("--json.inputs is required for %s", fieldExtensionIDBuiltinLLMCompletion)
	}
	if len(req.Inputs.Prompt) == 0 {
		return baseFlagErrorf("--json.inputs.prompt must contain at least one prompt segment")
	}
	for i, segment := range req.Inputs.Prompt {
		if err := validateFieldExtensionPromptSegment(i, segment); err != nil {
			return err
		}
	}
	return nil
}

func validateFieldExtensionPromptSegment(index int, segment fieldExtensionPromptSegment) error {
	switch segment.Type {
	case fieldExtensionPromptTypeText:
		if segment.Text == nil {
			return baseFlagErrorf("--json.inputs.prompt[%d].text is required when type is text", index)
		}
		if segment.Field != nil {
			return baseFlagErrorf("--json.inputs.prompt[%d].field is only valid when type is field_ref", index)
		}
	case fieldExtensionPromptTypeFieldRef:
		if segment.Field == nil || *segment.Field == "" {
			return baseFlagErrorf("--json.inputs.prompt[%d].field is required when type is field_ref", index)
		}
		if segment.Text != nil {
			return baseFlagErrorf("--json.inputs.prompt[%d].text is only valid when type is text", index)
		}
	default:
		return baseFlagErrorf("--json.inputs.prompt[%d].type must be text or field_ref", index)
	}
	return nil
}

func fieldExtensionUpdateCellsBody(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	updateType := strings.TrimSpace(runtime.Str("type"))
	body := map[string]interface{}{
		"type": updateType,
	}
	viewID := strings.TrimSpace(runtime.Str("view-id"))
	recordIDs, err := fieldExtensionRecordIDs(runtime)
	if err != nil {
		return nil, err
	}
	switch updateType {
	case "column":
		if len(recordIDs) > 0 {
			return nil, baseFlagErrorf("--record-id is only valid when --type row")
		}
		if viewID != "" {
			body["view_id"] = viewID
		}
	case "row":
		if viewID != "" {
			return nil, baseFlagErrorf("--view-id is only valid when --type column")
		}
		if len(recordIDs) == 0 {
			return nil, baseFlagErrorf("--record-id is required when --type row")
		}
		body["record_ids"] = recordIDs
	default:
		return nil, baseFlagErrorf("--type must be column or row")
	}
	return body, nil
}

func fieldExtensionRecordIDs(runtime *common.RuntimeContext) ([]string, error) {
	raw := runtime.StrArray("record-id")
	return normalizeStringList(raw, stringListNormalizeOptions{
		typeError:     "record IDs must be a string array",
		itemName:      "record ID",
		duplicateName: "record id",
		limitName:     "record IDs",
		allowNil:      true,
		allowEmpty:    true,
	})
}

func executeFieldExtensionGet(runtime *common.RuntimeContext) error {
	data, err := baseV3Call(runtime, "GET", fieldExtensionPath(runtime), nil, nil)
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}

func executeFieldExtensionUpdate(runtime *common.RuntimeContext) error {
	body, err := parseFieldExtensionUpdateBody(runtime)
	if err != nil {
		return err
	}
	data, err := baseV3Call(runtime, "PUT", fieldExtensionPath(runtime), nil, body)
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}

func executeFieldExtensionUpdateCells(runtime *common.RuntimeContext) error {
	body, err := fieldExtensionUpdateCellsBody(runtime)
	if err != nil {
		return err
	}
	data, err := baseV3Call(runtime, "POST", fieldExtensionPath(runtime)+"/update_cells", nil, body)
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}
