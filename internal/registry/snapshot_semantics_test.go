// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registry

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
	"testing"
)

// TestCatalogSnapshotPinsReviewedSemantics guards the field-level corrections
// made to the snapshot after the 2026-09-10 reviews. The registry's English
// source carried descriptions that inverted a boolean, borrowed another
// field's text or invented a condition; the corrected text follows the live
// Chinese metadata. A re-sync that brings the old text back must fail here so
// the source is fixed before the snapshot is refreshed.
func TestCatalogSnapshotPinsReviewedSemantics(t *testing.T) {
	type pin struct {
		service  string
		path     []string
		contains []string
		excludes []string
	}
	pins := []pin{
		{ // F1: true means notify, not the opposite
			service:  "drive",
			path:     []string{"resources", "permission.members", "methods", "transfer_owner", "parameters", "need_notification", "description"},
			contains: []string{"true = notify"},
			excludes: []string{"not notified"},
		},
		{ // F7: folder grants are conditional for bots, not unsupported
			service:  "drive",
			path:     []string{"resources", "permission.members", "methods", "create", "parameters", "type", "options", "6", "description"},
			contains: []string{"manage permission"},
			excludes: []string{"not supported"},
		},
		{ // F8: topic 2 is the handled queue, not an approval result
			service:  "approval",
			path:     []string{"resources", "tasks", "methods", "query", "parameters", "topic", "options", "1", "description"},
			contains: []string{"Handled approvals"},
		},
		{ // F6: the upper bound defaults to the last task
			service:  "task",
			path:     []string{"resources", "sections", "methods", "tasks", "parameters", "created_to", "description"},
			contains: []string{"last task"},
			excludes: []string{"first task"},
		},
		{ // F6: omitting completed disables the filter
			service:  "task",
			path:     []string{"resources", "tasks", "methods", "list", "parameters", "completed", "description"},
			contains: []string{"omitting it disables"},
			excludes: []string{"Fill in means"},
		},
		{ // F12: update_fields lists every machine key
			service:  "task",
			path:     []string{"resources", "tasks", "methods", "patch", "requestBody", "update_fields", "description"},
			contains: []string{"- summary:", "- mode:", "- is_milestone:", "- custom_fields:"},
		},
		{ // F13: the cross-field constraint on start/due is stated
			service:  "task",
			path:     []string{"resources", "tasks", "methods", "create", "requestBody", "start", "properties", "timestamp", "description"},
			contains: []string{"start time must be <= the due time", "is_all_day settings"},
		},
		{ // F14: an objective list is not an error code
			service:  "okr",
			path:     []string{"resources", "cycle.objectives", "methods", "list", "responseBody", "items", "description"},
			contains: []string{"Objective list"},
			excludes: []string{"Error code"},
		},
		{ // F10: response fields carry their own enum text
			service:  "im",
			path:     []string{"resources", "chats", "methods", "create", "responseBody", "group_message_type", "description"},
			contains: []string{"`chat`", "`thread`"},
			excludes: []string{"only_owner"},
		},
		{
			service:  "im",
			path:     []string{"resources", "chats", "methods", "create", "responseBody", "chat_mode", "description"},
			contains: []string{"`group`"},
			excludes: []string{"`thread`"},
		},
		{ // F9: patch schemas replace the whole list
			service:  "calendar",
			path:     []string{"resources", "events", "methods", "patch", "requestBody", "schemas", "description"},
			contains: []string{"覆盖更新", "[]"},
		},
		{ // F9: app calendars need allow_attendees_start = true
			service:  "calendar",
			path:     []string{"resources", "events", "methods", "create", "requestBody", "vchat", "properties", "meeting_settings", "properties", "allow_attendees_start", "description"},
			contains: []string{"必须为 true"},
		},
		{ // F15: the find range syntax is spelled out
			service:  "sheets",
			path:     []string{"resources", "spreadsheet.sheets", "methods", "find", "requestBody", "find_condition", "properties", "range", "description"},
			contains: []string{"<sheetId>!<start>:<end>"},
		},
		{ // F11: examples are exact enum literals
			service:  "mail",
			path:     []string{"resources", "user_mailbox.messages", "methods", "get", "responseBody", "message", "properties", "security_level", "properties", "risk_banner_level", "example"},
			contains: []string{"WARNING"},
		},
		{
			service:  "wiki",
			path:     []string{"resources", "spaces", "methods", "get", "responseBody", "space", "properties", "open_sharing", "example"},
			contains: []string{"open"},
			excludes: []string{"Open"},
		},
		{
			service:  "wiki",
			path:     []string{"resources", "members", "methods", "delete", "requestBody", "member_role", "example"},
			contains: []string{"admin"},
			excludes: []string{"Admin"},
		},
	}
	for _, p := range pins {
		got := stringAt(t, loadShardDoc(t, p.service), p.path)
		for _, want := range p.contains {
			if !strings.Contains(got, want) {
				t.Errorf("%s %s: %q must contain %q", p.service, strings.Join(p.path, "/"), got, want)
			}
		}
		for _, bad := range p.excludes {
			if strings.Contains(got, bad) {
				t.Errorf("%s %s: %q must not contain %q", p.service, strings.Join(p.path, "/"), got, bad)
			}
		}
	}
}

// TestCatalogSnapshotEntityIDsCarryNoIdentityTypeCondition guards the twelve
// OKR entity-ID parameters: a cycle, objective or key result ID is not typed by
// user_id_type or department_id_type. The real user_id parameter keeps its
// condition and serves as the positive control.
func TestCatalogSnapshotEntityIDsCarryNoIdentityTypeCondition(t *testing.T) {
	doc := loadShardDoc(t, "okr")
	resources := doc["resources"].(map[string]any)
	entityParams := 0
	for rname, r := range resources {
		methods := r.(map[string]any)["methods"].(map[string]any)
		for mname, m := range methods {
			params, _ := m.(map[string]any)["parameters"].(map[string]any)
			for pname, p := range params {
				desc, _ := p.(map[string]any)["description"].(string)
				switch pname {
				case "cycle_id", "objective_id", "key_result_id":
					entityParams++
					if strings.Contains(desc, "must match") {
						t.Errorf("okr %s.%s %s: entity ID carries an identity-type condition: %q", rname, mname, pname, desc)
					}
				}
			}
		}
	}
	if entityParams < 12 {
		t.Fatalf("expected at least 12 OKR entity ID parameters, found %d", entityParams)
	}
	userID := stringAt(t, doc, []string{"resources", "cycles", "methods", "list", "parameters", "user_id", "description"})
	if !strings.Contains(userID, "must match") {
		t.Fatalf("okr cycles.list user_id lost its user_id_type condition: %q", userID)
	}
}

// TestCatalogSnapshotHasMoreDescribesPagination guards every has_more field:
// the English source described the pagination flag as "the response body has
// more parameters" across five domains.
func TestCatalogSnapshotHasMoreDescribesPagination(t *testing.T) {
	services, err := fs.Sub(embeddedCatalogFS, "catalog/services")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := fs.ReadDir(services, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		data, err := fs.ReadFile(services, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		var doc any
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatalf("%s: %v", entry.Name(), err)
		}
		walkDescriptions(doc, entry.Name(), func(pointer, text string) {
			if text == "Whether the response body has more parameters" {
				t.Errorf("%s: pagination flag described as a parameter count", pointer)
			}
		})
	}
}

func loadShardDoc(t testing.TB, service string) map[string]any {
	t.Helper()
	data, err := fs.ReadFile(embeddedCatalogFS, "catalog/services/"+service+".json")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("%s: %v", service, err)
	}
	return doc
}

// stringAt follows path through maps and arrays (numeric segments index arrays)
// and returns the string it lands on.
func stringAt(t testing.TB, doc any, path []string) string {
	t.Helper()
	node := doc
	for i, seg := range path {
		switch n := node.(type) {
		case map[string]any:
			next, ok := n[seg]
			if !ok {
				t.Fatalf("path %s: no key %q", strings.Join(path[:i+1], "/"), seg)
			}
			node = next
		case []any:
			var idx int
			if _, err := fmt.Sscanf(seg, "%d", &idx); err != nil || idx < 0 || idx >= len(n) {
				t.Fatalf("path %s: bad array index %q", strings.Join(path[:i+1], "/"), seg)
			}
			node = n[idx]
		default:
			t.Fatalf("path %s: cannot descend into %T", strings.Join(path[:i+1], "/"), node)
		}
	}
	s, ok := node.(string)
	if !ok {
		t.Fatalf("path %s: %T is not a string", strings.Join(path, "/"), node)
	}
	return s
}
