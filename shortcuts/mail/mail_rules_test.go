// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestMailRuleParserGrammarAndEncode(t *testing.T) {
	cond, err := parseRuleConditionGrammar("subject:contains:Alpha通知", "--condition")
	if err != nil {
		t.Fatalf("parseRuleConditionGrammar() error = %v", err)
	}
	if cond.Field != "subject" || cond.Operator != "contains" || cond.Value != "Alpha通知" {
		t.Fatalf("condition = %+v", cond)
	}
	action, err := parseRuleActionGrammar("move_folder:folder_id=fld_123", "--action")
	if err != nil {
		t.Fatalf("parseRuleActionGrammar() error = %v", err)
	}
	if action.Kind != "move_folder" || action.Params["folder_id"] != "fld_123" {
		t.Fatalf("action = %+v", action)
	}
	spec := &mailRuleSpec{Version: mailRuleSpecVersion}
	spec.Mailbox.UserMailboxID = "me"
	spec.Rule.Name = "Alpha"
	spec.Rule.Enabled = true
	spec.Rule.Match = "all"
	spec.Rule.Conditions = []mailRuleCondition{cond}
	spec.Rule.Actions = []mailRuleAction{action}
	raw, err := encodeRuleSpec(spec)
	if err != nil {
		t.Fatalf("encodeRuleSpec() error = %v", err)
	}
	conditionItems := raw["condition"].(map[string]any)["items"].([]map[string]any)
	if got := conditionItems[0]["type"]; got != 6 {
		t.Fatalf("condition type = %v, want 6", got)
	}
	actionItems := raw["action"].(map[string]any)["items"].([]map[string]any)
	if got := actionItems[0]["type"]; got != 11 {
		t.Fatalf("action type = %v, want 11", got)
	}
	if got := actionItems[0]["folder_id"]; got != "fld_123" {
		t.Fatalf("folder_id = %v", got)
	}
}

func TestMailRuleParserRejectsUnknownAliasWithHint(t *testing.T) {
	_, err := parseRuleConditionGrammar("subjct:contains:x", "--condition")
	if err == nil {
		t.Fatal("expected validation error")
	}
	for _, want := range []string{`unknown rule condition field "subjct"`, `did you mean "subject"?`, "Accepted fields and aliases", "title"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error should include %q, got %v", want, err)
		}
	}
	_, err = parseRuleConditionGrammar("subject:contaiins:x", "--condition")
	if err == nil {
		t.Fatal("expected operator validation error")
	}
	if !strings.Contains(err.Error(), `did you mean "contains"?`) || !strings.Contains(err.Error(), "Accepted operators and aliases") {
		t.Fatalf("operator error should include suggestion and accepted aliases, got %v", err)
	}
	_, err = parseRuleActionGrammar("markread", "--action")
	if err == nil {
		t.Fatal("expected action validation error")
	}
	if !strings.Contains(err.Error(), `did you mean "mark_read"?`) || !strings.Contains(err.Error(), "Accepted actions and aliases") {
		t.Fatalf("action error should include suggestion and accepted aliases, got %v", err)
	}
	_, err = parseRuleActionGrammar("move_folder", "--action")
	if err == nil || !strings.Contains(err.Error(), "folder_id") {
		t.Fatalf("expected missing folder_id error, got %v", err)
	}
}

func TestDecodeMailRuleEnvelopePreservesUnknowns(t *testing.T) {
	raw := map[string]any{
		"rule_id":                  "rule_1",
		"name":                     "Alpha",
		"is_enable":                true,
		"ignore_the_rest_of_rules": false,
		"condition": map[string]any{
			"match_type": 1,
			"items": []interface{}{
				map[string]interface{}{"type": float64(6), "operator": float64(1), "input": "Alpha"},
				map[string]interface{}{"type": float64(999), "operator": float64(1), "input": "unknown"},
			},
		},
		"action": map[string]any{
			"items": []interface{}{
				map[string]interface{}{"type": float64(3)},
				map[string]interface{}{"type": float64(777)},
			},
		},
	}
	env := decodeMailRuleEnvelope(raw, "me")
	if env.SemanticSpec == nil || len(env.SemanticSpec.Rule.Conditions) != 1 || len(env.SemanticSpec.Rule.Actions) != 1 {
		t.Fatalf("semantic decode mismatch: %+v", env.SemanticSpec)
	}
	if len(env.Unknowns) != 2 {
		t.Fatalf("unknowns = %+v, want 2", env.Unknowns)
	}
	if !strings.Contains(env.Description, "无法识别") {
		t.Fatalf("description should mention unknown raw, got %q", env.Description)
	}
}

func TestMailRuleListShortcutDecodesResponse(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(
		&httpmock.Stub{
			Method: "GET",
			URL:    "open-apis/mail/v1/user_mailboxes/me/rules",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"rules": []interface{}{
						map[string]interface{}{
							"rule_id":   "rule_1",
							"name":      "Alpha通知",
							"is_enable": true,
							"condition": map[string]interface{}{
								"match_type": 1,
								"items": []interface{}{
									map[string]interface{}{"type": 6, "operator": 1, "input": "Alpha"},
								},
							},
							"action": map[string]interface{}{
								"items": []interface{}{map[string]interface{}{"type": 3}},
							},
						},
					},
				},
			},
		},
	)
	if err := runMountedMailShortcut(t, MailRuleList, []string{"+rule-list", "--format", "json"}, f, stdout); err != nil {
		t.Fatalf("run +rule-list error = %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	rules, ok := data["rules"].([]interface{})
	if !ok || len(rules) != 1 {
		b, _ := json.Marshal(data)
		t.Fatalf("rules output mismatch: %s", b)
	}
	rule := rules[0].(map[string]interface{})
	if !strings.Contains(rule["description"].(string), "主题包含") {
		t.Fatalf("description = %v", rule["description"])
	}
}

func TestMailRuleUpdateAcceptsNameDryRun(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	if err := runMountedMailShortcut(t, MailRuleUpdate, []string{
		"+rule-update",
		"--rule-id", "rule_1",
		"--name", "Renamed",
		"--dry-run",
		"--format", "json",
	}, f, stdout); err != nil {
		t.Fatalf("run +rule-update --name --dry-run error = %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	rawPatch, ok := data["raw_request_patch"].(map[string]interface{})
	if !ok {
		t.Fatalf("raw_request_patch missing: %#v", data)
	}
	if got := rawPatch["name"]; got != "Renamed" {
		t.Fatalf("raw_request_patch.name = %v, want Renamed", got)
	}
}

func TestMailRuleUpdateNamePreservesCollectionsAndOutputsDiff(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	currentRule := map[string]interface{}{
		"rule_id":                  "rule_1",
		"name":                     "Old Name",
		"is_enable":                true,
		"ignore_the_rest_of_rules": false,
		"condition": map[string]interface{}{
			"match_type": 1,
			"items": []interface{}{
				map[string]interface{}{"type": 6, "operator": 1, "input": "Alpha"},
			},
		},
		"action": map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"type": 3},
			},
		},
	}
	updatedRule := map[string]interface{}{}
	for k, v := range currentRule {
		updatedRule[k] = v
	}
	updatedRule["name"] = "Renamed"

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "open-apis/mail/v1/user_mailboxes/me/rules",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"rules": []interface{}{currentRule},
			},
		},
	})
	putStub := &httpmock.Stub{
		Method: "PUT",
		URL:    "open-apis/mail/v1/user_mailboxes/me/rules/rule_1",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"rule": updatedRule,
			},
		},
	}
	reg.Register(putStub)

	if err := runMountedMailShortcut(t, MailRuleUpdate, []string{
		"+rule-update",
		"--rule-id", "rule_1",
		"--name", "Renamed",
		"--format", "json",
	}, f, stdout); err != nil {
		t.Fatalf("run +rule-update --name error = %v", err)
	}
	reg.Verify(t)

	var putBody map[string]interface{}
	if err := json.Unmarshal(putStub.CapturedBody, &putBody); err != nil {
		t.Fatalf("unmarshal PUT body: %v", err)
	}
	if got := putBody["name"]; got != "Renamed" {
		t.Fatalf("PUT body name = %v, want Renamed", got)
	}
	assertJSONEqual(t, putBody["condition"], currentRule["condition"], "condition")
	assertJSONEqual(t, putBody["action"], currentRule["action"], "action")

	data := decodeShortcutEnvelopeData(t, stdout)
	diff, ok := data["diff"].([]interface{})
	if !ok || len(diff) != 1 {
		t.Fatalf("diff = %#v, want one name entry", data["diff"])
	}
	entry := diff[0].(map[string]interface{})
	if entry["field"] != "name" || entry["before"] != "Old Name" || entry["after"] != "Renamed" {
		t.Fatalf("diff entry = %#v, want name Old Name -> Renamed", entry)
	}
}

func TestMailRuleUpdateReplacesActionsAndPreservesConditions(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	currentRule := map[string]interface{}{
		"rule_id":                  "rule_1",
		"name":                     "Old Name",
		"is_enable":                true,
		"ignore_the_rest_of_rules": false,
		"condition": map[string]interface{}{
			"match_type": 1,
			"items": []interface{}{
				map[string]interface{}{"type": 6, "operator": 1, "input": "Alpha"},
			},
		},
		"action": map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"type": 11, "folder_id": "fld_old"},
			},
		},
	}
	updatedRule := map[string]interface{}{}
	for k, v := range currentRule {
		updatedRule[k] = v
	}
	updatedRule["action"] = map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{"type": 3},
			map[string]interface{}{"type": 9},
		},
	}

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "open-apis/mail/v1/user_mailboxes/me/rules",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"rules": []interface{}{currentRule},
			},
		},
	})
	putStub := &httpmock.Stub{
		Method: "PUT",
		URL:    "open-apis/mail/v1/user_mailboxes/me/rules/rule_1",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"rule": updatedRule,
			},
		},
	}
	reg.Register(putStub)

	if err := runMountedMailShortcut(t, MailRuleUpdate, []string{
		"+rule-update",
		"--rule-id", "rule_1",
		"--action", "mark_read",
		"--action", "star",
		"--format", "json",
	}, f, stdout); err != nil {
		t.Fatalf("run +rule-update --action error = %v", err)
	}
	reg.Verify(t)

	var putBody map[string]interface{}
	if err := json.Unmarshal(putStub.CapturedBody, &putBody); err != nil {
		t.Fatalf("unmarshal PUT body: %v", err)
	}
	assertJSONEqual(t, putBody["condition"], currentRule["condition"], "condition")
	action := putBody["action"].(map[string]interface{})
	items := action["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("action items = %#v, want two replacements", items)
	}
	if got := items[0].(map[string]interface{})["type"]; got != float64(3) {
		t.Fatalf("first action type = %v, want 3", got)
	}
	if got := items[1].(map[string]interface{})["type"]; got != float64(9) {
		t.Fatalf("second action type = %v, want 9", got)
	}
}

func assertJSONEqual(t *testing.T, got, want interface{}, label string) {
	t.Helper()
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal got %s: %v", label, err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal want %s: %v", label, err)
	}
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("%s = %s, want %s", label, gotJSON, wantJSON)
	}
}

func TestRuleReorderComputesMoveTarget(t *testing.T) {
	current := []mailRuleEnvelope{{RuleID: "a"}, {RuleID: "b"}, {RuleID: "c"}}
	f, _, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailRuleReorder, []string{"+rule-reorder", "--move-rule-id", "c", "--before-rule-id", "a", "--dry-run"}, f, nil)
	if err != nil {
		t.Fatalf("dry-run should validate move flags: %v", err)
	}
	order, err := insertRelative(removeString(envelopeRuleIDs(current), "c"), "c", "a", false)
	if err != nil {
		t.Fatalf("insertRelative() error = %v", err)
	}
	if got := strings.Join(order, ","); got != "c,a,b" {
		t.Fatalf("order = %s", got)
	}
}

func TestMailRuleReorderRejectsDuplicateIDsBeforeDryRunAndExecute(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "dry-run", args: []string{"+rule-reorder", "--rule-ids", "a,a", "--dry-run", "--format", "json"}},
		{name: "execute", args: []string{"+rule-reorder", "--rule-ids", "a,a", "--format", "json"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, _ := mailShortcutTestFactory(t)
			err := runMountedMailShortcut(t, MailRuleReorder, tt.args, f, stdout)
			if err == nil {
				t.Fatal("duplicate rule IDs should be rejected")
			}
			if !strings.Contains(err.Error(), "duplicate a") {
				t.Fatalf("error = %q, want duplicate rule ID detail", err)
			}
		})
	}
}
