// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/shortcuts/common"
)

// parseCtx carries file I/O dependency for JSON/file parsing helpers.
type parseCtx struct {
	fio fileio.FileIO
}

func newParseCtx(runtime *common.RuntimeContext) *parseCtx {
	return &parseCtx{fio: runtime.FileIO()}
}

func baseTableID(runtime *common.RuntimeContext) string {
	return strings.TrimSpace(runtime.Str("table-id"))
}

func pageSizeLimitAliasFlag() common.Flag {
	return common.Flag{Name: "page-size", Type: "int", Default: "0", Desc: "hidden alias for --limit", Hidden: true}
}

func getPaginationLimit(runtime *common.RuntimeContext) int {
	if !runtime.Changed("limit") && runtime.Changed("page-size") {
		return runtime.Int("page-size")
	}
	return runtime.Int("limit")
}

func validateLimitPageSizeAlias(runtime *common.RuntimeContext) error {
	if runtime.Changed("limit") && runtime.Changed("page-size") {
		return common.ValidationErrorf("--limit and --page-size are mutually exclusive; use --limit").
			WithParam("--page-size")
	}
	return nil
}

func loadJSONInput(pc *parseCtx, raw string, flagName string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", baseFlagErrorf("--%s cannot be empty", flagName)
	}
	if !strings.HasPrefix(raw, "@") {
		return raw, nil
	}
	path := strings.TrimSpace(strings.TrimPrefix(raw, "@"))
	if path == "" {
		return "", baseFlagErrorf("--%s file path cannot be empty after @", flagName)
	}
	if pc.fio == nil {
		return "", baseMissingFileIOError("--%s @file inputs require a FileIO provider", flagName)
	}
	f, err := pc.fio.Open(path)
	if err != nil {
		var pathErr *fileio.PathValidationError
		if errors.As(err, &pathErr) {
			return "", baseFlagErrorf("--%s invalid JSON file path %q: %v", flagName, path, pathErr.Err)
		}
		return "", baseFlagErrorf("--%s cannot open JSON file %q: %v", flagName, path, err)
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return "", baseFlagErrorf("--%s cannot read JSON file %q: %v", flagName, path, err)
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return "", baseFlagErrorf("--%s JSON file %q is empty", flagName, path)
	}
	return content, nil
}

func jsonInputTip(flagName string) string {
	return fmt.Sprintf("tip: pass a valid JSON directly, or use --%s @file.json; for complex JSON/DSL, read the lark-base reference and match the documented shape", flagName)
}

func formatJSONError(flagName string, target string, err error) error {
	if syntaxErr, ok := err.(*json.SyntaxError); ok {
		return baseFlagErrorf("--%s invalid JSON %s near byte %d (%v); %s", flagName, target, syntaxErr.Offset, err, jsonInputTip(flagName))
	}
	if typeErr, ok := err.(*json.UnmarshalTypeError); ok {
		if typeErr.Field != "" {
			return baseFlagErrorf("--%s invalid JSON %s at field %q (%v); %s", flagName, target, typeErr.Field, err, jsonInputTip(flagName))
		}
		return baseFlagErrorf("--%s invalid JSON %s (%v); %s", flagName, target, err, jsonInputTip(flagName))
	}
	return baseFlagErrorf("--%s invalid JSON %s (%v); %s", flagName, target, err, jsonInputTip(flagName))
}

func baseAction(runtime *common.RuntimeContext, boolFlags []string, stringFlags []string) (string, error) {
	active := []string{}
	for _, name := range boolFlags {
		if runtime.Bool(name) {
			active = append(active, name)
		}
	}
	for _, name := range stringFlags {
		if strings.TrimSpace(runtime.Str(name)) != "" {
			active = append(active, name)
		}
	}
	if len(active) == 0 {
		return "", baseFlagErrorf("specify one action")
	}
	if len(active) > 1 {
		flags := make([]string, 0, len(active))
		for _, item := range active {
			flags = append(flags, "--"+item)
		}
		return "", baseFlagErrorf("actions are mutually exclusive: %s", strings.Join(flags, ", "))
	}
	return active[0], nil
}

func parseObjectList(pc *parseCtx, raw string, flagName string) ([]map[string]interface{}, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var err error
	raw, err = loadJSONInput(pc, raw, flagName)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(raw, "[") {
		arr, err := parseJSONArray(pc, raw, flagName)
		if err != nil {
			return nil, err
		}
		items := make([]map[string]interface{}, 0, len(arr))
		for idx, item := range arr {
			obj, ok := item.(map[string]interface{})
			if !ok {
				return nil, baseFlagErrorf("--%s item %d must be an object", flagName, idx+1)
			}
			items = append(items, obj)
		}
		return items, nil
	}
	obj, err := parseJSONObject(pc, raw, flagName)
	if err != nil {
		return nil, err
	}
	return []map[string]interface{}{obj}, nil
}

func parseJSONValue(pc *parseCtx, raw string, flagName string) (interface{}, error) {
	var err error
	raw, err = loadJSONInput(pc, raw, flagName)
	if err != nil {
		return nil, err
	}
	var value interface{}
	if err := common.ParseJSON([]byte(raw), &value); err != nil {
		return nil, formatJSONError(flagName, "value", err)
	}
	switch value.(type) {
	case map[string]interface{}, []interface{}:
		return value, nil
	default:
		return nil, baseFlagErrorf("--%s must be a JSON object or array", flagName)
	}
}
