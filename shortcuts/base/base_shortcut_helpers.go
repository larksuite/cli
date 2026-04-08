// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/shortcuts/common"
)

func baseTableID(runtime *common.RuntimeContext) string {
	return strings.TrimSpace(runtime.Str("table-id"))
}

func loadJSONInput(fio fileio.FileIO, raw string, flagName string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", common.FlagErrorf("--%s cannot be empty", flagName)
	}
	if !strings.HasPrefix(raw, "@") {
		return raw, nil
	}
	path := strings.TrimSpace(strings.TrimPrefix(raw, "@"))
	if path == "" {
		return "", common.FlagErrorf("--%s file path cannot be empty after @", flagName)
	}
	f, err := fio.Open(path)
	if err != nil {
		return "", common.FlagErrorf("--%s invalid JSON file path %q: %v", flagName, path, err)
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return "", common.FlagErrorf("--%s cannot read JSON file %q: %v", flagName, path, err)
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return "", common.FlagErrorf("--%s JSON file %q is empty", flagName, path)
	}
	return content, nil
}

func jsonInputTip(flagName string) string {
	return fmt.Sprintf("tip: pass a JSON object/array directly, or use --%s @path/to/file.json", flagName)
}

func formatJSONError(flagName string, target string, err error) error {
	if syntaxErr, ok := err.(*json.SyntaxError); ok {
		return common.FlagErrorf("--%s invalid JSON %s near byte %d (%v); %s", flagName, target, syntaxErr.Offset, err, jsonInputTip(flagName))
	}
	if typeErr, ok := err.(*json.UnmarshalTypeError); ok {
		if typeErr.Field != "" {
			return common.FlagErrorf("--%s invalid JSON %s at field %q (%v); %s", flagName, target, typeErr.Field, err, jsonInputTip(flagName))
		}
		return common.FlagErrorf("--%s invalid JSON %s (%v); %s", flagName, target, err, jsonInputTip(flagName))
	}
	return common.FlagErrorf("--%s invalid JSON %s (%v); %s", flagName, target, err, jsonInputTip(flagName))
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
		return "", common.FlagErrorf("specify one action")
	}
	if len(active) > 1 {
		flags := make([]string, 0, len(active))
		for _, item := range active {
			flags = append(flags, "--"+item)
		}
		return "", common.FlagErrorf("actions are mutually exclusive: %s", strings.Join(flags, ", "))
	}
	return active[0], nil
}

func parseObjectList(fio fileio.FileIO, raw string, flagName string) ([]map[string]interface{}, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var err error
	raw, err = loadJSONInput(fio, raw, flagName)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(raw, "[") {
		arr, err := parseJSONArray(fio, raw, flagName)
		if err != nil {
			return nil, err
		}
		items := make([]map[string]interface{}, 0, len(arr))
		for idx, item := range arr {
			obj, ok := item.(map[string]interface{})
			if !ok {
				return nil, common.FlagErrorf("--%s item %d must be an object", flagName, idx+1)
			}
			items = append(items, obj)
		}
		return items, nil
	}
	obj, err := parseJSONObject(fio, raw, flagName)
	if err != nil {
		return nil, err
	}
	return []map[string]interface{}{obj}, nil
}

func parseJSONValue(fio fileio.FileIO, raw string, flagName string) (interface{}, error) {
	var err error
	raw, err = loadJSONInput(fio, raw, flagName)
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
		return nil, common.FlagErrorf("--%s must be a JSON object or array", flagName)
	}
}
