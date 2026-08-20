// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
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

func TestMailRuleConditionJSONCoercesNonStringValue(t *testing.T) {
	conds, err := parseRuleConditionValue(nil, `{"field":"subject","operator":"contains","value":9007199254740993}`, "--condition")
	if err != nil {
		t.Fatalf("parseRuleConditionValue() error = %v", err)
	}
	if len(conds) != 1 {
		t.Fatalf("conditions = %+v, want one", conds)
	}
	if conds[0].Value != "9007199254740993" {
		t.Fatalf("condition value = %q", conds[0].Value)
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

func TestMailRuleListRejectsRepeatedPageToken(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	for i := 0; i < 2; i++ {
		reg.Register(
			&httpmock.Stub{
				Method: "GET",
				URL:    "open-apis/mail/v1/user_mailboxes/me/rules",
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{
						"rules":      []interface{}{},
						"has_more":   true,
						"page_token": "same_token",
					},
				},
			},
		)
	}

	err := runMountedMailShortcut(t, MailRuleList, []string{"+rule-list", "--format", "json"}, f, stdout)
	if err == nil {
		t.Fatal("expected repeated page token error")
	}
	if !strings.Contains(err.Error(), "repeated page_token") {
		t.Fatalf("error = %v", err)
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

func TestMailRuleUpdateRegistersNameFlag(t *testing.T) {
	flags := map[string]common.Flag{}
	for _, flag := range MailRuleUpdate.Flags {
		flags[flag.Name] = flag
	}

	name, ok := flags["name"]
	if !ok {
		t.Fatal("missing --name flag")
	}
	if name.Required {
		t.Fatal("--name must be optional for partial rule updates")
	}

	ruleID, ok := flags["rule-id"]
	if !ok {
		t.Fatal("missing --rule-id flag")
	}
	if !ruleID.Required {
		t.Fatal("--rule-id must remain required")
	}
}

func TestMailRuleUpdatePreservesRawFieldsAndUpdatesName(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	currentRule := map[string]interface{}{
		"rule_id":                  "rule_1",
		"name":                     "Alpha",
		"is_enable":                true,
		"ignore_the_rest_of_rules": false,
		"vendor_top":               "keep",
		"condition": map[string]interface{}{
			"match_type": 1,
			"items": []interface{}{
				map[string]interface{}{"type": 6, "operator": 1, "input": "Alpha", "vendor_condition_extra": "keep"},
			},
		},
		"action": map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"type": 3, "vendor_action_extra": "keep"},
			},
		},
	}
	reg.Register(
		&httpmock.Stub{
			Method: "GET",
			URL:    "open-apis/mail/v1/user_mailboxes/me/rules",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"rules": []interface{}{currentRule},
				},
			},
		},
	)
	put := &httpmock.Stub{
		Method: "PUT",
		URL:    "open-apis/mail/v1/user_mailboxes/me/rules/rule_1",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"rule": map[string]interface{}{
					"rule_id":   "rule_1",
					"name":      "Beta",
					"is_enable": true,
				},
			},
		},
	}
	reg.Register(put)

	err := runMountedMailShortcut(t, MailRuleUpdate, []string{"+rule-update", "--rule-id", "rule_1", "--name", "Beta", "--format", "json"}, f, stdout)
	if err != nil {
		t.Fatalf("run +rule-update error = %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(put.CapturedBody, &body); err != nil {
		t.Fatalf("Unmarshal(PUT body) error = %v, body=%s", err, string(put.CapturedBody))
	}
	if body["name"] != "Beta" {
		t.Fatalf("name = %v", body["name"])
	}
	if body["vendor_top"] != "keep" {
		t.Fatalf("vendor_top = %v", body["vendor_top"])
	}
	condition := body["condition"].(map[string]interface{})
	conditionItems := condition["items"].([]interface{})
	if got := conditionItems[0].(map[string]interface{})["vendor_condition_extra"]; got != "keep" {
		t.Fatalf("vendor_condition_extra = %v", got)
	}
	action := body["action"].(map[string]interface{})
	actionItems := action["items"].([]interface{})
	if got := actionItems[0].(map[string]interface{})["vendor_action_extra"]; got != "keep" {
		t.Fatalf("vendor_action_extra = %v", got)
	}
}

func TestMailRuleUpdateHelpListsNameFlag(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)

	err := runMountedMailShortcutWithCobraOutput(t, MailRuleUpdate, []string{"+rule-update", "-h"}, f, stdout)
	if err != nil {
		t.Fatalf("help returned error: %v", err)
	}

	if !strings.Contains(stdout.String(), "--name") {
		t.Fatalf("rule-update help missing --name flag\n%s", stdout.String())
	}
}
