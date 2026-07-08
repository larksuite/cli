// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package application

import (
	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// matchCommandItem finds the item whose "command" equals name (exact match -
// the server enforces name uniqueness, so first hit is the only hit).
func matchCommandItem(items []interface{}, name string) (string, map[string]interface{}) {
	for _, it := range items {
		m, ok := it.(map[string]interface{})
		if !ok {
			continue
		}
		if m["command"] == name {
			id, _ := m["command_id"].(string)
			if id != "" {
				return id, m
			}
		}
	}
	return "", nil
}

// commandNotFoundError reports a resolution miss against the live list as an
// API-category not-found error (the name is a valid argument shape; the
// resource simply does not exist server-side - this is not a validation
// failure of caller input).
func commandNotFoundError(name string) error {
	return errs.NewAPIError(errs.SubtypeNotFound,
		"slash command %q not found in the current bound app", name).
		WithHint("run `lark-cli application +slash-command-list` to see registered commands")
}

// resolveCommandID resolves a command name to its command_id via the live
// list endpoint (in-memory only; never touches local files). Requires the
// read scope on the current identity.
func resolveCommandID(runtime *common.RuntimeContext, name string) (string, map[string]interface{}, error) {
	data, err := runtime.CallAPITyped("GET", slashCommandBasePath, nil, nil)
	if err != nil {
		return "", nil, err
	}
	items, _ := data["items"].([]interface{})
	id, item := matchCommandItem(items, name)
	if id == "" {
		return "", nil, commandNotFoundError(name)
	}
	return id, item, nil
}
