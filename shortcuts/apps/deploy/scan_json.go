// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deploy

import (
	"encoding/json"
	"strings"
)

// scanJSON collects the file paths written as values inside a JSON document --
// a web manifest naming its icons, a sketch describing its assets.
//
// Only values are examined; a key that looks like a path is a label, not a
// reference. Paths found here resolve against the payload root rather than
// against the JSON file, because the code that reads them fetches them from the
// document's location.
func scanJSON(raw []byte) ([]string, int, error) {
	var doc interface{}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, 0, &parseError{code: "invalid_json"}
	}
	var refs []string
	var walk func(interface{})
	walk = func(v interface{}) {
		switch t := v.(type) {
		case string:
			if !resourcePathRe.MatchString(strings.TrimSpace(t)) {
				return
			}
			if rewritten, ok := toDocumentRelative(t); ok {
				refs = append(refs, rewritten)
			}
		case []interface{}:
			for _, item := range t {
				walk(item)
			}
		case map[string]interface{}:
			for _, item := range t {
				walk(item)
			}
		}
	}
	walk(doc)
	return refs, 0, nil
}
