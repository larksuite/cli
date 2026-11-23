// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
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
	if _, ok := actionItems[0]["folder_id"]; ok {
		t.Fatalf("move_folder OAPI body must not send folder_id: %v", actionItems[0])
	}
	if got := actionItems[0]["input"]; got != "fld_123" {
		t.Fatalf("input = %v, want fld_123", got)
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
	_, err = parseRuleActionGrammar("forward:email=dev@example.com", "--action")
	if err == nil {
		t.Fatal("expected unsupported forward action error")
	}
	accepted := acceptedAliasList(mailRuleActions)
	for _, unsupported := range []string{"forward", "add_user_label", "share_to_chat"} {
		if strings.Contains(accepted, unsupported) {
			t.Fatalf("unsupported action %q should not be listed in accepted aliases: %s", unsupported, accepted)
		}
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

func TestDecodeMailRuleEnvelopeMarksKnownItemExtraFieldsUnknown(t *testing.T) {
	raw := map[string]any{
		"rule_id":   "rule_1",
		"name":      "Alpha",
		"is_enable": true,
		"condition": map[string]any{
			"match_type":             1,
			"tenant_condition_extra": "keep",
			"items": []interface{}{
				map[string]interface{}{"type": float64(6), "operator": float64(1), "input": "Alpha", "case_sensitive": true},
			},
		},
		"action": map[string]any{
			"tenant_action_extra": "keep",
			"items": []interface{}{
				map[string]interface{}{"type": float64(11), "input": "fld_1", "params": map[string]interface{}{"input": "fld_1", "vendor": "keep"}, "vendor_action_extra": "keep"},
			},
		},
	}
	env := decodeMailRuleEnvelope(raw, "me")
	unknownPaths := map[string]bool{}
	for _, item := range env.Unknowns {
		unknownPaths[item.Path] = true
	}
	for _, want := range []string{
		"condition.tenant_condition_extra",
		"condition.items[0].case_sensitive",
		"action.tenant_action_extra",
		"action.items[0].params.vendor",
		"action.items[0].vendor_action_extra",
	} {
		if !unknownPaths[want] {
			t.Fatalf("missing unknown path %s in %+v", want, env.Unknowns)
		}
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

func TestMailRuleListIgnoresPaginationFields(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	list := &httpmock.Stub{
		Method: "GET",
		URL:    "open-apis/mail/v1/user_mailboxes/me/rules",
		OnMatch: func(req *http.Request) {
			if req.URL.RawQuery != "" {
				t.Fatalf("list request query = %q, want empty", req.URL.RawQuery)
			}
		},
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"rules": []interface{}{
					mailRuleTestRawRule("rule_1", "Alpha"),
				},
				"has_more":   true,
				"page_token": "ignored",
			},
		},
	}
	reg.Register(list)

	if err := runMountedMailShortcut(t, MailRuleList, []string{"+rule-list", "--format", "json"}, f, stdout); err != nil {
		t.Fatalf("run +rule-list error = %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	if data["total"] != float64(1) {
		t.Fatalf("total = %v, want 1", data["total"])
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

func TestMailRuleWriteRisks(t *testing.T) {
	for _, tc := range []struct {
		shortcut common.Shortcut
		risk     string
	}{
		{MailRuleCreate, "high-risk-write"},
		{MailRuleUpdate, "high-risk-write"},
		{MailRuleEnable, "write"},
		{MailRuleDisable, "write"},
		{MailRuleReorder, "write"},
		{MailRuleDelete, "high-risk-write"},
	} {
		if tc.shortcut.Risk != tc.risk {
			t.Fatalf("%s Risk = %q, want %q", tc.shortcut.Command, tc.shortcut.Risk, tc.risk)
		}
		for _, flag := range tc.shortcut.Flags {
			if flag.Name == "yes" {
				t.Fatalf("%s must rely on the standard risk framework instead of declaring a custom --yes flag", tc.shortcut.Command)
			}
		}
	}
}

func TestMailRuleScopes(t *testing.T) {
	for _, tc := range []struct {
		shortcut common.Shortcut
		scopes   []string
	}{
		{MailRuleList, []string{"mail:user_mailbox.rule:read"}},
		{MailRuleGet, []string{"mail:user_mailbox.rule:read"}},
		{MailRuleCreate, []string{"mail:user_mailbox.rule:write"}},
		{MailRuleUpdate, []string{"mail:user_mailbox.rule:write"}},
		{MailRuleDelete, []string{"mail:user_mailbox.rule:write"}},
		{MailRuleEnable, []string{"mail:user_mailbox.rule:write"}},
		{MailRuleDisable, []string{"mail:user_mailbox.rule:write"}},
		{MailRuleReorder, []string{"mail:user_mailbox.rule:write"}},
	} {
		if !reflect.DeepEqual(tc.shortcut.Scopes, tc.scopes) {
			t.Fatalf("%s Scopes = %v, want %v", tc.shortcut.Command, tc.shortcut.Scopes, tc.scopes)
		}
	}
}

func TestMailRuleCreateAndUpdateRequireStandardYesConfirmation(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		f, stdout, _, reg := mailShortcutTestFactory(t)
		post := &httpmock.Stub{
			Method:   "POST",
			URL:      "open-apis/mail/v1/user_mailboxes/me/rules",
			Optional: true,
			Body:     map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
		}
		reg.Register(post)

		err := runMountedMailShortcut(t, MailRuleCreate, []string{"+rule-create", "--name", "Alpha", "--condition", "subject:contains:Alpha", "--action", "mark_read", "--format", "json"}, f, stdout)
		var confirmErr *errs.ConfirmationRequiredError
		if !errors.As(err, &confirmErr) {
			t.Fatalf("expected confirmation required error, got %T: %v", err, err)
		}
		if len(post.CapturedBodies) != 0 {
			t.Fatalf("POST should not be sent before --yes, captured %d request(s)", len(post.CapturedBodies))
		}
	})

	t.Run("update", func(t *testing.T) {
		f, stdout, _, reg := mailShortcutTestFactory(t)
		list := mailRuleListStub(mailRuleTestRawRule("rule_1", "Alpha"))
		list.Optional = true
		reg.Register(list)
		put := &httpmock.Stub{
			Method:   "PUT",
			URL:      "open-apis/mail/v1/user_mailboxes/me/rules/rule_1",
			Optional: true,
			Body:     map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
		}
		reg.Register(put)

		err := runMountedMailShortcut(t, MailRuleUpdate, []string{"+rule-update", "--rule-id", "rule_1", "--name", "Beta", "--format", "json"}, f, stdout)
		var confirmErr *errs.ConfirmationRequiredError
		if !errors.As(err, &confirmErr) {
			t.Fatalf("expected confirmation required error, got %T: %v", err, err)
		}
		if len(list.CapturedBodies) != 0 || len(put.CapturedBodies) != 0 {
			t.Fatalf("GET/PUT should not be sent before --yes, captured GET=%d PUT=%d", len(list.CapturedBodies), len(put.CapturedBodies))
		}
	})
}

func TestMailRuleShortcutsAllowBotButRequireExplicitMailbox(t *testing.T) {
	for _, shortcut := range []common.Shortcut{MailRuleList, MailRuleGet, MailRuleCreate, MailRuleUpdate, MailRuleDelete, MailRuleEnable, MailRuleDisable, MailRuleReorder} {
		hasBot := false
		for _, authType := range shortcut.AuthTypes {
			if authType == "bot" {
				hasBot = true
			}
		}
		if !hasBot {
			t.Fatalf("%s AuthTypes = %v, want bot support", shortcut.Command, shortcut.AuthTypes)
		}
	}

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("user-mailbox-id", "me", "")
	botDefault := common.TestNewRuntimeContextWithIdentity(cmd, mailTestConfig(), core.AsBot)
	if err := validateMailRuleMailbox(nil, botDefault); err == nil {
		t.Fatal("expected bot default mailbox validation error")
	}

	cmd = &cobra.Command{Use: "test"}
	cmd.Flags().String("user-mailbox-id", "me", "")
	if err := cmd.Flags().Set("user-mailbox-id", "user@example.com"); err != nil {
		t.Fatal(err)
	}
	botExplicit := common.TestNewRuntimeContextWithIdentity(cmd, mailTestConfig(), core.AsBot)
	if err := validateMailRuleMailbox(nil, botExplicit); err != nil {
		t.Fatalf("bot explicit mailbox validation error = %v", err)
	}

	userDefault := common.TestNewRuntimeContextWithIdentity(cmd, mailTestConfig(), core.AsUser)
	if err := validateMailRuleMailbox(nil, userDefault); err != nil {
		t.Fatalf("user mailbox validation error = %v", err)
	}
}

func TestMailRuleDeleteRequiresStandardYesConfirmation(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(mailRuleListStub(mailRuleTestRawRule("rule_1", "Alpha")))
	del := &httpmock.Stub{
		Method:   "DELETE",
		URL:      "open-apis/mail/v1/user_mailboxes/me/rules/rule_1",
		Optional: true,
		Body:     map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
	}
	reg.Register(del)
	err := runMountedMailShortcut(t, MailRuleDelete, []string{"+rule-delete", "--rule-id", "rule_1", "--format", "json"}, f, stdout)
	if err == nil {
		t.Fatal("expected confirmation required error")
	}
	var confirmErr *errs.ConfirmationRequiredError
	if !errors.As(err, &confirmErr) {
		t.Fatalf("expected confirmation required error, got %T: %v", err, err)
	}
	if !strings.Contains(confirmErr.Action, "mail +rule-delete") || !strings.Contains(confirmErr.Action, "Alpha") {
		t.Fatalf("confirmation action should include target summary, got %q", confirmErr.Action)
	}
	if len(del.CapturedBodies) != 0 {
		t.Fatalf("DELETE should not be sent before --yes, captured %d request(s)", len(del.CapturedBodies))
	}
}

func TestMailRuleUpdateUsesRequestBodyWhitelistAndUpdatesName(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	currentRule := map[string]interface{}{
		"rule_id":                  "rule_1",
		"id":                       "response_id",
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
	updatedRule := cloneMailRuleRawMap(t, currentRule)
	updatedRule["name"] = "Beta"
	reg.Register(mailRuleListStub(updatedRule))

	err := runMountedMailShortcut(t, MailRuleUpdate, []string{"+rule-update", "--rule-id", "rule_1", "--name", "Beta", "--yes", "--format", "json"}, f, stdout)
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
	for _, forbidden := range []string{"vendor_top", "rule_id", "id"} {
		if _, ok := body[forbidden]; ok {
			t.Fatalf("PUT body must not include %s: %v", forbidden, body)
		}
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

func TestMailRuleUpdatePreservesConditionItemExtrasWhenChangingMatch(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	currentRule := map[string]interface{}{
		"rule_id":                  "rule_1",
		"name":                     "Alpha",
		"is_enable":                true,
		"ignore_the_rest_of_rules": false,
		"condition": map[string]interface{}{
			"match_type": 1,
			"items": []interface{}{
				map[string]interface{}{"type": 6, "operator": 1, "input": "Alpha", "vendor_condition_extra": "keep"},
				map[string]interface{}{"type": 999, "operator": 1, "input": "unknown"},
			},
			"vendor_condition_top": "keep-top",
		},
		"action": map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"type": 3},
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
					"name":      "Alpha",
					"is_enable": true,
				},
			},
		},
	}
	reg.Register(put)
	updatedRule := cloneMailRuleRawMap(t, currentRule)
	updatedRule["condition"].(map[string]interface{})["match_type"] = 2
	reg.Register(mailRuleListStub(updatedRule))

	err := runMountedMailShortcut(t, MailRuleUpdate, []string{"+rule-update", "--rule-id", "rule_1", "--match", "any", "--yes", "--format", "json"}, f, stdout)
	if err != nil {
		t.Fatalf("run +rule-update error = %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(put.CapturedBody, &body); err != nil {
		t.Fatalf("Unmarshal(PUT body) error = %v, body=%s", err, string(put.CapturedBody))
	}
	condition := body["condition"].(map[string]interface{})
	if got := condition["match_type"]; got != float64(2) {
		t.Fatalf("match_type = %v, want 2", got)
	}
	if got := condition["vendor_condition_top"]; got != "keep-top" {
		t.Fatalf("vendor_condition_top = %v", got)
	}
	conditionItems := condition["items"].([]interface{})
	if len(conditionItems) != 2 {
		t.Fatalf("condition items = %+v, want original 2 items preserved", conditionItems)
	}
	if got := conditionItems[0].(map[string]interface{})["vendor_condition_extra"]; got != "keep" {
		t.Fatalf("vendor_condition_extra = %v", got)
	}
	if got := conditionItems[1].(map[string]interface{})["type"]; got != float64(999) {
		t.Fatalf("unknown condition item type = %v, want preserved 999", got)
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

func TestMailRuleCreateParsesJSONConditionsAndActions(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	post := &httpmock.Stub{
		Method: "POST",
		URL:    "open-apis/mail/v1/user_mailboxes/user@example.com/rules",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"rule": map[string]interface{}{
					"rule_id":   "rule_json",
					"name":      "JSON Rule",
					"is_enable": false,
				},
			},
		},
	}
	reg.Register(post)

	err := runMountedMailShortcut(t, MailRuleCreate, []string{
		"+rule-create",
		"--user-mailbox-id", "user@example.com",
		"--name", "JSON Rule",
		"--match", "any",
		"--disable",
		"--stop-after-match",
		"--conditions", `[{"field":"subject","op":"contains","value":"Alpha"},{"field":"has_attachment"}]`,
		"--actions", `[{"kind":"mark_read"},{"kind":"move_folder","folder_id":"fld_json"}]`,
		"--yes",
		"--format", "json",
	}, f, stdout)
	if err != nil {
		t.Fatalf("run +rule-create error = %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(post.CapturedBody, &body); err != nil {
		t.Fatalf("Unmarshal(POST body) error = %v, body=%s", err, string(post.CapturedBody))
	}
	if body["name"] != "JSON Rule" || body["is_enable"] != false || body["ignore_the_rest_of_rules"] != true {
		t.Fatalf("top-level body fields mismatch: %v", body)
	}
	condition := body["condition"].(map[string]interface{})
	if got := condition["match_type"]; got != float64(2) {
		t.Fatalf("match_type = %v, want 2", got)
	}
	conditionItems := condition["items"].([]interface{})
	if len(conditionItems) != 2 {
		t.Fatalf("condition items = %+v, want 2", conditionItems)
	}
	if got := conditionItems[1].(map[string]interface{})["type"]; got != float64(16) {
		t.Fatalf("has_attachment type = %v, want 16", got)
	}
	actionItems := body["action"].(map[string]interface{})["items"].([]interface{})
	if len(actionItems) != 2 {
		t.Fatalf("action items = %+v, want 2", actionItems)
	}
	if got := actionItems[0].(map[string]interface{})["type"]; got != float64(3) {
		t.Fatalf("mark_read type = %v, want 3", got)
	}
	if got := actionItems[1].(map[string]interface{})["input"]; got != "fld_json" {
		t.Fatalf("move_folder input = %v", got)
	}
}

func TestMailRuleCreateReadsConditionsAndActionsFromFiles(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("conditions.json", []byte(`[{"field":"body","operator":"contains","value":"hello"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("actions.json", []byte(`[{"kind":"mark_read"}]`), 0o600); err != nil {
		t.Fatal(err)
	}

	f, stdout, _, reg := mailShortcutTestFactory(t)
	post := &httpmock.Stub{
		Method: "POST",
		URL:    "open-apis/mail/v1/user_mailboxes/me/rules",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
	}
	reg.Register(post)

	err := runMountedMailShortcut(t, MailRuleCreate, []string{
		"+rule-create",
		"--name", "File Rule",
		"--conditions", "@conditions.json",
		"--actions", "@actions.json",
		"--yes",
		"--format", "json",
	}, f, stdout)
	if err != nil {
		t.Fatalf("run +rule-create from files error = %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(post.CapturedBody, &body); err != nil {
		t.Fatalf("Unmarshal(POST body) error = %v, body=%s", err, string(post.CapturedBody))
	}
	conditionItems := body["condition"].(map[string]interface{})["items"].([]interface{})
	if got := conditionItems[0].(map[string]interface{})["type"]; got != float64(7) {
		t.Fatalf("body condition type = %v, want 7", got)
	}
	actionItems := body["action"].(map[string]interface{})["items"].([]interface{})
	if got := actionItems[0].(map[string]interface{})["type"]; got != float64(3) {
		t.Fatalf("mark_read type = %v, want 3", got)
	}
}

func TestMailRuleCreateRejectsOversizedRuleInputFile(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("conditions.json", bytes.Repeat([]byte(" "), mailRuleInputFileMaxBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}

	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailRuleCreate, []string{
		"+rule-create",
		"--name", "Oversized",
		"--conditions", "@conditions.json",
		"--action", "mark_read",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected oversized file error")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v", err)
	}
}

func TestMailRuleCreateValidationErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing name",
			args: []string{"+rule-create", "--condition", "subject:contains:Alpha", "--action", "mark_read"},
			want: "--name is required",
		},
		{
			name: "missing condition",
			args: []string{"+rule-create", "--name", "Alpha", "--action", "mark_read"},
			want: "at least one --condition",
		},
		{
			name: "missing action",
			args: []string{"+rule-create", "--name", "Alpha", "--condition", "subject:contains:Alpha"},
			want: "at least one --action",
		},
		{
			name: "invalid match",
			args: []string{"+rule-create", "--name", "Alpha", "--match", "maybe", "--condition", "subject:contains:Alpha", "--action", "mark_read"},
			want: "allowed: all, any",
		},
		{
			name: "conflicting enable flags",
			args: []string{"+rule-create", "--name", "Alpha", "--enable", "--disable", "--condition", "subject:contains:Alpha", "--action", "mark_read"},
			want: "mutually exclusive",
		},
		{
			name: "conflicting stop flags",
			args: []string{"+rule-create", "--name", "Alpha", "--stop-after-match", "--continue-after-match", "--condition", "subject:contains:Alpha", "--action", "mark_read"},
			want: "mutually exclusive",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, stdout, _, _ := mailShortcutTestFactory(t)
			err := runMountedMailShortcut(t, MailRuleCreate, tc.args, f, stdout)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestMailRuleParserRejectsTrailingJSONValues(t *testing.T) {
	if _, err := parseRuleConditionValue(nil, `[{"field":"subject","operator":"contains","value":"Alpha"}] {"field":"body"}`, "--conditions"); err == nil {
		t.Fatal("expected trailing condition JSON error")
	}
	if _, err := parseRuleActionValue(nil, `[{"kind":"mark_read"}] {"kind":"archive"}`, "--actions"); err == nil {
		t.Fatal("expected trailing action JSON error")
	}
	if _, err := parseRuleActionGrammar(`move_folder:json={"folder_id":"fld_1"} {"folder_id":"fld_2"}`, "--action"); err == nil {
		t.Fatal("expected trailing action params JSON error")
	}
}

func TestMailRuleParserRejectsOversizedCollections(t *testing.T) {
	conditions := make([]any, mailRuleCollectionMax+1)
	for i := range conditions {
		conditions[i] = map[string]any{"field": "subject", "operator": "contains", "value": "Alpha"}
	}
	if _, err := parseRuleConditionJSON(conditions, "--conditions"); err == nil {
		t.Fatal("expected condition count error")
	}

	actions := make([]any, mailRuleCollectionMax+1)
	for i := range actions {
		actions[i] = map[string]any{"kind": "mark_read"}
	}
	if _, err := parseRuleActionJSON(actions, "--actions"); err == nil {
		t.Fatal("expected action count error")
	}
}

func TestMailRuleParserRejectsAggregateInputLimits(t *testing.T) {
	largeValue := strings.Repeat("a", mailRuleInputFileMaxBytes/2+1)
	largeCondition := `[{"field":"subject","operator":"contains","value":"` + largeValue + `"}]`
	if _, err := parseRuleConditions(nil, []string{largeCondition, largeCondition}, ""); err == nil {
		t.Fatal("expected aggregate condition byte limit error")
	}

	repeated := make([]string, mailRuleCollectionMax+1)
	for i := range repeated {
		repeated[i] = "subject:contains:Alpha"
	}
	if _, err := parseRuleConditions(nil, repeated, ""); err == nil {
		t.Fatal("expected aggregate condition count limit error")
	}
}

func TestMailRuleDryRunShortcuts(t *testing.T) {
	for _, tc := range []struct {
		name     string
		shortcut common.Shortcut
		args     []string
	}{
		{
			name:     "list",
			shortcut: MailRuleList,
			args:     []string{"+rule-list", "--dry-run"},
		},
		{
			name:     "get",
			shortcut: MailRuleGet,
			args:     []string{"+rule-get", "--rule-id", "rule_1", "--dry-run"},
		},
		{
			name:     "create",
			shortcut: MailRuleCreate,
			args:     []string{"+rule-create", "--name", "Alpha", "--condition", "subject:contains:Alpha", "--action", "mark_read", "--dry-run"},
		},
		{
			name:     "update",
			shortcut: MailRuleUpdate,
			args:     []string{"+rule-update", "--rule-id", "rule_1", "--name", "Beta", "--dry-run"},
		},
		{
			name:     "delete",
			shortcut: MailRuleDelete,
			args:     []string{"+rule-delete", "--rule-id", "rule_1", "--dry-run"},
		},
		{
			name:     "enable",
			shortcut: MailRuleEnable,
			args:     []string{"+rule-enable", "--rule-id", "rule_1", "--dry-run"},
		},
		{
			name:     "reorder",
			shortcut: MailRuleReorder,
			args:     []string{"+rule-reorder", "--rule-ids", "a,b", "--dry-run"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, stdout, _, _ := mailShortcutTestFactory(t)
			if err := runMountedMailShortcut(t, tc.shortcut, tc.args, f, stdout); err != nil {
				t.Fatalf("dry-run error = %v", err)
			}
		})
	}
}

func TestMailRuleUpdateRejectsEmptyUpdate(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailRuleUpdate, []string{"+rule-update", "--rule-id", "rule_1", "--format", "json"}, f, stdout)
	if err == nil {
		t.Fatal("expected empty update error")
	}
	if !strings.Contains(err.Error(), "at least one update field") {
		t.Fatalf("error = %v", err)
	}
}

func TestMailRuleUpdateNoopsWhenRequestedStateAlreadyMatches(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(mailRuleListStub(mailRuleTestRawRule("rule_1", "Alpha")))
	put := &httpmock.Stub{
		Method:   "PUT",
		URL:      "open-apis/mail/v1/user_mailboxes/me/rules/rule_1",
		Optional: true,
		Body:     map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
	}
	reg.Register(put)

	if err := runMountedMailShortcut(t, MailRuleUpdate, []string{"+rule-update", "--rule-id", "rule_1", "--name", "Alpha", "--yes", "--format", "json"}, f, stdout); err != nil {
		t.Fatalf("run +rule-update error = %v", err)
	}
	if len(put.CapturedBodies) != 0 {
		t.Fatalf("PUT should not be sent for no-op update, captured %d request(s)", len(put.CapturedBodies))
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	if data["no_op"] != true {
		t.Fatalf("no_op = %v, want true", data["no_op"])
	}
}

func TestMailRuleGetShortcutReturnsMatchAndMissingRule(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(mailRuleListStub(mailRuleTestRawRule("rule_1", "Alpha")))
	if err := runMountedMailShortcut(t, MailRuleGet, []string{"+rule-get", "--rule-id", "rule_1", "--format", "json"}, f, stdout); err != nil {
		t.Fatalf("run +rule-get error = %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	if data["rule_id"] != "rule_1" {
		t.Fatalf("rule = %+v", data)
	}

	reg.Register(mailRuleListStub(mailRuleTestRawRule("rule_2", "Beta")))
	err := runMountedMailShortcut(t, MailRuleGet, []string{"+rule-get", "--rule-id", "rule_missing", "--format", "json"}, f, stdout)
	if err == nil {
		t.Fatal("expected missing rule error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %v", err)
	}
}

func TestMailRuleParserErrorBranches(t *testing.T) {
	if conds, err := parseRuleConditionValue(nil, "", "--condition"); err != nil || conds != nil {
		t.Fatalf("empty condition = %+v, %v", conds, err)
	}
	if _, err := parseRuleConditionValue(nil, `{`, "--condition"); err == nil {
		t.Fatal("expected invalid condition JSON error")
	}
	if _, err := parseRuleConditionJSON([]any{"bad"}, "--condition"); err == nil {
		t.Fatal("expected invalid condition array item error")
	}
	if _, err := parseRuleConditionJSON("bad", "--condition"); err == nil {
		t.Fatal("expected invalid condition object error")
	}
	if _, err := parseRuleConditionValue(nil, "subject:contains:", "--condition"); err == nil {
		t.Fatal("expected empty condition value error")
	}
	if _, err := parseRuleConditionValue(nil, "has_attachment:contains:x", "--condition"); err == nil {
		t.Fatal("expected boolean condition value error")
	}
	if _, err := parseRuleConditionValue(nil, "subject:empty:x", "--condition"); err == nil {
		t.Fatal("expected no-value operator error")
	}

	if actions, err := parseRuleActionValue(nil, "", "--action"); err != nil || actions != nil {
		t.Fatalf("empty action = %+v, %v", actions, err)
	}
	if _, err := parseRuleActionValue(nil, `{`, "--action"); err == nil {
		t.Fatal("expected invalid action JSON error")
	}
	if _, err := parseRuleActionJSON([]any{"bad"}, "--action"); err == nil {
		t.Fatal("expected invalid action array item error")
	}
	if _, err := parseRuleActionJSON("bad", "--action"); err == nil {
		t.Fatal("expected invalid action object error")
	}
	if _, err := parseRuleActionGrammar(`move_folder:json={`, "--action"); err == nil {
		t.Fatal("expected invalid action params JSON error")
	}
	if _, err := parseRuleActionGrammar("forward:email", "--action"); err == nil {
		t.Fatal("expected invalid action grammar error")
	}
	if _, err := parseRuleActionGrammar("mark_read:foo=bar", "--action"); err == nil {
		t.Fatal("expected parameterless action error")
	}
}

func TestMailRuleActionValueParsesJSONObjectsAndGrammarJSONParams(t *testing.T) {
	actions, err := parseRuleActionValue(nil, `{"kind":"move_folder","params":{"folder_id":"fld_1"}}`, "--action")
	if err != nil {
		t.Fatalf("parseRuleActionValue(object) error = %v", err)
	}
	if len(actions) != 1 || actions[0].Kind != "move_folder" || actions[0].Params["folder_id"] != "fld_1" {
		t.Fatalf("actions = %+v", actions)
	}

	actions, err = parseRuleActionValue(nil, `move_folder:json={"folder_id":"fld_2"}`, "--action")
	if err != nil {
		t.Fatalf("parseRuleActionValue(grammar json) error = %v", err)
	}
	if len(actions) != 1 || actions[0].Kind != "move_folder" || actions[0].Params["folder_id"] != "fld_2" {
		t.Fatalf("actions = %+v", actions)
	}
}

func TestMailRuleActionParamsCannotOverrideControlledFields(t *testing.T) {
	for _, raw := range []string{
		`move_folder:json={"folder_id":"fld_1","input":"hidden"}`,
		`move_folder:json={"folder_id":"fld_1","type":2}`,
		`{"kind":"move_folder","folder_id":"fld_1","input":"hidden"}`,
		`{"kind":"move_folder","params":{"folder_id":"fld_1","type":2}}`,
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := parseRuleActionValue(nil, raw, "--action"); err == nil {
				t.Fatal("expected controlled field validation error")
			}
		})
	}
}

func TestMailRuleRejectsUnsupportedOAPIActions(t *testing.T) {
	for _, raw := range []string{
		`{"kind":"add_user_label","params":{"label_id":"lbl_1"}}`,
		`{"kind":"forward","email":"dev@example.com"}`,
		`{"kind":"share_to_chat","params":{"chat_id":"oc_123"}}`,
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := parseRuleActionValue(nil, raw, "--action")
			if err == nil {
				t.Fatal("expected unsupported action error")
			}
			accepted := acceptedAliasList(mailRuleActions)
			for _, unsupported := range []string{"add_user_label", "forward", "share_to_chat"} {
				if strings.Contains(accepted, unsupported) {
					t.Fatalf("unsupported action %q should not be listed in accepted aliases: %s", unsupported, accepted)
				}
			}
		})
	}
}

func TestMailRuleToggleAndDeleteShortcutsPreserveRawBody(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	currentRule := mailRuleTestRawRule("rule_1", "Alpha")
	reg.Register(mailRuleListStub(currentRule))
	put := &httpmock.Stub{
		Method: "PUT",
		URL:    "open-apis/mail/v1/user_mailboxes/me/rules/rule_1",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
	}
	reg.Register(put)

	if err := runMountedMailShortcut(t, MailRuleDisable, []string{"+rule-disable", "--rule-id", "rule_1", "--format", "json"}, f, stdout); err != nil {
		t.Fatalf("run +rule-disable error = %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(put.CapturedBody, &body); err != nil {
		t.Fatalf("Unmarshal(PUT body) error = %v, body=%s", err, string(put.CapturedBody))
	}
	if body["is_enable"] != false {
		t.Fatalf("toggle body is_enable = %v, want false", body["is_enable"])
	}
	for _, forbidden := range []string{"vendor_top", "rule_id", "id"} {
		if _, ok := body[forbidden]; ok {
			t.Fatalf("toggle PUT body must not include %s: %v", forbidden, body)
		}
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	after := data["after"].(map[string]interface{})
	afterRaw := after["raw"].(map[string]interface{})
	if afterRaw["is_enable"] != false {
		t.Fatalf("after.raw is_enable = %v, want false", afterRaw["is_enable"])
	}

	reg.Register(mailRuleListStub(currentRule))
	del := &httpmock.Stub{
		Method: "DELETE",
		URL:    "open-apis/mail/v1/user_mailboxes/me/rules/rule_1",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
	}
	reg.Register(del)
	if err := runMountedMailShortcut(t, MailRuleDelete, []string{"+rule-delete", "--rule-id", "rule_1", "--yes", "--format", "json"}, f, stdout); err != nil {
		t.Fatalf("run +rule-delete error = %v", err)
	}
	data = decodeShortcutEnvelopeData(t, stdout)
	if data["deleted"] != true {
		t.Fatalf("deleted = %v, want true", data["deleted"])
	}
}

func TestMailRuleToggleNoopsWhenAlreadyAtTargetState(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(mailRuleListStub(mailRuleTestRawRule("rule_1", "Alpha")))
	put := &httpmock.Stub{
		Method:   "PUT",
		URL:      "open-apis/mail/v1/user_mailboxes/me/rules/rule_1",
		Optional: true,
		Body:     map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
	}
	reg.Register(put)

	if err := runMountedMailShortcut(t, MailRuleEnable, []string{"+rule-enable", "--rule-id", "rule_1", "--format", "json"}, f, stdout); err != nil {
		t.Fatalf("run +rule-enable error = %v", err)
	}
	if len(put.CapturedBodies) != 0 {
		t.Fatalf("PUT should not be sent for no-op enable, captured %d request(s)", len(put.CapturedBodies))
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	if data["no_op"] != true {
		t.Fatalf("no_op = %v, want true", data["no_op"])
	}
}

func TestMailRuleListFilterUsesTextTable(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(mailRuleListStub(
		mailRuleTestRawRule("rule_1", "Alpha"),
		mailRuleTestRawRule("rule_2", "Beta"),
	))

	if err := runMountedMailShortcut(t, MailRuleList, []string{"+rule-list", "--name-contains", "beta"}, f, stdout); err != nil {
		t.Fatalf("run +rule-list error = %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "Beta") {
		t.Fatalf("filtered table should include Beta, got\n%s", out)
	}
	if strings.Contains(out, "Alpha") {
		t.Fatalf("filtered table should exclude Alpha, got\n%s", out)
	}
}

func TestMailRuleReorderShortcutPostsFullAndMoveOrders(t *testing.T) {
	t.Run("full order", func(t *testing.T) {
		f, stdout, _, reg := mailShortcutTestFactory(t)
		reg.Register(mailRuleListStub(
			mailRuleTestRawRule("a", "A"),
			mailRuleTestRawRule("b", "B"),
			mailRuleTestRawRule("c", "C"),
		))
		post := &httpmock.Stub{
			Method: "POST",
			URL:    "open-apis/mail/v1/user_mailboxes/me/rules/reorder",
			Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
		}
		reg.Register(post)

		if err := runMountedMailShortcut(t, MailRuleReorder, []string{"+rule-reorder", "--rule-ids", "c,b,a", "--format", "json"}, f, stdout); err != nil {
			t.Fatalf("run +rule-reorder full error = %v", err)
		}
		assertRuleIDsBody(t, post.CapturedBody, "c,b,a")
	})

	t.Run("move to bottom", func(t *testing.T) {
		f, stdout, _, reg := mailShortcutTestFactory(t)
		reg.Register(mailRuleListStub(
			mailRuleTestRawRule("a", "A"),
			mailRuleTestRawRule("b", "B"),
			mailRuleTestRawRule("c", "C"),
		))
		post := &httpmock.Stub{
			Method: "POST",
			URL:    "open-apis/mail/v1/user_mailboxes/me/rules/reorder",
			Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
		}
		reg.Register(post)

		if err := runMountedMailShortcut(t, MailRuleReorder, []string{"+rule-reorder", "--move-rule-id", "a", "--to-bottom", "--format", "json"}, f, stdout); err != nil {
			t.Fatalf("run +rule-reorder move error = %v", err)
		}
		assertRuleIDsBody(t, post.CapturedBody, "b,c,a")
	})

	t.Run("move to top", func(t *testing.T) {
		f, stdout, _, reg := mailShortcutTestFactory(t)
		reg.Register(mailRuleListStub(
			mailRuleTestRawRule("a", "A"),
			mailRuleTestRawRule("b", "B"),
			mailRuleTestRawRule("c", "C"),
		))
		post := &httpmock.Stub{
			Method: "POST",
			URL:    "open-apis/mail/v1/user_mailboxes/me/rules/reorder",
			Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
		}
		reg.Register(post)

		if err := runMountedMailShortcut(t, MailRuleReorder, []string{"+rule-reorder", "--move-rule-id", "c", "--to-top", "--format", "json"}, f, stdout); err != nil {
			t.Fatalf("run +rule-reorder to-top error = %v", err)
		}
		assertRuleIDsBody(t, post.CapturedBody, "c,a,b")
	})

	t.Run("move after target", func(t *testing.T) {
		f, stdout, _, reg := mailShortcutTestFactory(t)
		reg.Register(mailRuleListStub(
			mailRuleTestRawRule("a", "A"),
			mailRuleTestRawRule("b", "B"),
			mailRuleTestRawRule("c", "C"),
		))
		post := &httpmock.Stub{
			Method: "POST",
			URL:    "open-apis/mail/v1/user_mailboxes/me/rules/reorder",
			Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
		}
		reg.Register(post)

		if err := runMountedMailShortcut(t, MailRuleReorder, []string{"+rule-reorder", "--move-rule-id", "a", "--after-rule-id", "c", "--format", "json"}, f, stdout); err != nil {
			t.Fatalf("run +rule-reorder after error = %v", err)
		}
		assertRuleIDsBody(t, post.CapturedBody, "b,c,a")
	})
}

func TestMailRuleUpdateReplacesUnknownConditionCollection(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	currentRule := mailRuleTestRawRule("rule_1", "Alpha")
	currentRule["condition"].(map[string]interface{})["items"] = []interface{}{
		map[string]interface{}{"type": 999, "operator": 1, "input": "unknown"},
	}
	reg.Register(mailRuleListStub(currentRule))
	put := &httpmock.Stub{
		Method: "PUT",
		URL:    "open-apis/mail/v1/user_mailboxes/me/rules/rule_1",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
	}
	reg.Register(put)
	updatedRule := cloneMailRuleRawMap(t, currentRule)
	updatedRule["condition"].(map[string]interface{})["items"] = []interface{}{
		map[string]interface{}{"type": 6, "operator": 1, "input": "Beta"},
	}
	reg.Register(mailRuleListStub(updatedRule))

	err := runMountedMailShortcut(t, MailRuleUpdate, []string{"+rule-update", "--rule-id", "rule_1", "--condition", "subject:contains:Beta", "--yes", "--format", "json"}, f, stdout)
	if err != nil {
		t.Fatalf("run +rule-update error = %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(put.CapturedBody, &body); err != nil {
		t.Fatalf("Unmarshal(PUT body) error = %v, body=%s", err, string(put.CapturedBody))
	}
	items := body["condition"].(map[string]interface{})["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("condition items = %+v, want replacement", items)
	}
	item := items[0].(map[string]interface{})
	if item["type"] != float64(6) || item["input"] != "Beta" {
		t.Fatalf("condition replacement item = %+v", item)
	}
}

func TestMailRuleUpdateReplacesConditionItemsAndPreservesMatchType(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	currentRule := mailRuleTestRawRule("rule_1", "Alpha")
	currentRule["condition"].(map[string]interface{})["match_type"] = 2
	currentRule["condition"].(map[string]interface{})["tenant_condition_extra"] = "keep"
	reg.Register(mailRuleListStub(currentRule))
	put := &httpmock.Stub{
		Method: "PUT",
		URL:    "open-apis/mail/v1/user_mailboxes/me/rules/rule_1",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
	}
	reg.Register(put)
	updatedRule := cloneMailRuleRawMap(t, currentRule)
	updatedRule["condition"].(map[string]interface{})["items"] = []interface{}{
		map[string]interface{}{"type": 6, "operator": 1, "input": "Beta"},
	}
	reg.Register(mailRuleListStub(updatedRule))

	err := runMountedMailShortcut(t, MailRuleUpdate, []string{"+rule-update", "--rule-id", "rule_1", "--condition", "subject:contains:Beta", "--yes", "--format", "json"}, f, stdout)
	if err != nil {
		t.Fatalf("run +rule-update error = %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(put.CapturedBody, &body); err != nil {
		t.Fatalf("Unmarshal(PUT body) error = %v, body=%s", err, string(put.CapturedBody))
	}
	condition := body["condition"].(map[string]interface{})
	if condition["match_type"] != float64(2) {
		t.Fatalf("match_type = %v, want preserved 2", condition["match_type"])
	}
	if condition["tenant_condition_extra"] != "keep" {
		t.Fatalf("tenant_condition_extra = %v, want preserved", condition["tenant_condition_extra"])
	}
}

func TestMailRuleUpdateRejectsExplicitEmptyCollections(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "empty conditions", args: []string{"+rule-update", "--rule-id", "rule_1", "--conditions", "[]", "--format", "json"}, want: "at least one --condition"},
		{name: "empty actions", args: []string{"+rule-update", "--rule-id", "rule_1", "--actions", "[]", "--format", "json"}, want: "at least one --action"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, stdout, _, _ := mailShortcutTestFactory(t)
			err := runMountedMailShortcut(t, MailRuleUpdate, tc.args, f, stdout)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestMailRuleUpdateReplacesKnownConditionsAndActions(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	currentRule := mailRuleTestRawRule("rule_1", "Alpha")
	reg.Register(mailRuleListStub(currentRule))
	put := &httpmock.Stub{
		Method: "PUT",
		URL:    "open-apis/mail/v1/user_mailboxes/me/rules/rule_1",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
	}
	reg.Register(put)
	updatedRule := cloneMailRuleRawMap(t, currentRule)
	updatedRule["condition"].(map[string]interface{})["items"] = []interface{}{
		map[string]interface{}{"type": 6, "operator": 1, "input": "Beta"},
	}
	updatedRule["action"].(map[string]interface{})["items"] = []interface{}{
		map[string]interface{}{"type": 11, "input": "fld_2"},
	}
	reg.Register(mailRuleListStub(updatedRule))

	err := runMountedMailShortcut(t, MailRuleUpdate, []string{
		"+rule-update",
		"--rule-id", "rule_1",
		"--condition", "subject:contains:Beta",
		"--action", "move_folder:folder_id=fld_2",
		"--yes",
		"--format", "json",
	}, f, stdout)
	if err != nil {
		t.Fatalf("run +rule-update error = %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(put.CapturedBody, &body); err != nil {
		t.Fatalf("Unmarshal(PUT body) error = %v, body=%s", err, string(put.CapturedBody))
	}
	conditionItems := body["condition"].(map[string]interface{})["items"].([]interface{})
	if got := conditionItems[0].(map[string]interface{})["input"]; got != "Beta" {
		t.Fatalf("condition input = %v, want Beta", got)
	}
	actionItems := body["action"].(map[string]interface{})["items"].([]interface{})
	if _, ok := actionItems[0].(map[string]interface{})["folder_id"]; ok {
		t.Fatalf("move_folder OAPI body must not send folder_id: %v", actionItems[0])
	}
	if got := actionItems[0].(map[string]interface{})["input"]; got != "fld_2" {
		t.Fatalf("input = %v, want fld_2", got)
	}
}

func TestMailRuleDecodeHandlesMalformedRawItems(t *testing.T) {
	conditions, unknowns := decodeRuleConditions([]interface{}{
		"bad-condition",
		map[string]interface{}{"operator": 1},
		map[string]interface{}{"type": 6},
		map[string]interface{}{"type": 6, "operator": 999},
		map[string]interface{}{"type": 16},
	}, nil)
	if len(conditions) != 1 || conditions[0].Field != "has_attachment" {
		t.Fatalf("conditions = %+v", conditions)
	}
	if len(unknowns) != 4 {
		t.Fatalf("condition unknowns = %+v, want 4", unknowns)
	}

	actions, unknowns := decodeRuleActions([]interface{}{
		"bad-action",
		map[string]interface{}{},
		map[string]interface{}{"type": 999},
		map[string]interface{}{"type": 11, "params": map[string]interface{}{"input": "nested_fld"}},
	}, nil)
	if len(actions) != 1 || actions[0].Kind != "move_folder" || actions[0].Params["folder_id"] != "nested_fld" {
		t.Fatalf("actions = %+v", actions)
	}
	if len(unknowns) != 3 {
		t.Fatalf("action unknowns = %+v, want 3", unknowns)
	}
}

func TestMailRuleExtractAndRenderHelpers(t *testing.T) {
	rule := mailRuleTestRawRule("rule_1", "Alpha")
	if got := extractRuleItems(map[string]interface{}{"items": []interface{}{rule}}); len(got) != 1 {
		t.Fatalf("items extraction = %+v", got)
	}
	if got := extractRuleItems(map[string]interface{}{"rule": rule}); len(got) != 1 {
		t.Fatalf("rule extraction = %+v", got)
	}
	if got := firstRuleObject(map[string]interface{}{"name": "Direct"}); got["name"] != "Direct" {
		t.Fatalf("firstRuleObject direct = %+v", got)
	}
	if got := firstRuleObject(map[string]interface{}{}); len(got) != 0 {
		t.Fatalf("firstRuleObject empty = %+v", got)
	}

	envs := []mailRuleEnvelope{decodeMailRuleEnvelope(rule, "me")}
	var out strings.Builder
	printMailRuleTable(&out, envs)
	if !strings.Contains(out.String(), "rule_1") || !strings.Contains(out.String(), "Alpha") {
		t.Fatalf("table output missing rule data\n%s", out.String())
	}

	raw := map[string]any{
		"id":                       123,
		"rule_name":                "Fallback",
		"enabled":                  "false",
		"ignore_the_rest_of_rules": float64(1),
		"sequence":                 9,
		"condition": map[string]interface{}{
			"match_type": 99,
			"items":      []interface{}{},
		},
	}
	env := decodeMailRuleEnvelope(raw, "me")
	if env.RuleID != "123" || env.Name != "Fallback" || env.Enabled || !env.SemanticSpec.Rule.StopAfterMatch || env.Order != 9 {
		t.Fatalf("fallback decode mismatch: %+v", env)
	}
	if env.SemanticSpec.Rule.Match != "" {
		t.Fatalf("unknown match type should not decode as comparable match, got %q", env.SemanticSpec.Rule.Match)
	}
	if len(env.Unknowns) != 1 || env.Unknowns[0].Path != "condition.match_type" {
		t.Fatalf("unknown match type = %+v", env.Unknowns)
	}
}

func TestMailRuleScalarHelpersCoverFallbacks(t *testing.T) {
	for _, tc := range []struct {
		raw  any
		want int
		ok   bool
	}{
		{int64(7), 7, true},
		{json.Number("8"), 8, true},
		{"9", 9, true},
		{"bad", 0, false},
		{true, 0, false},
	} {
		got, ok := intValue(tc.raw)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("intValue(%v) = %d,%v want %d,%v", tc.raw, got, ok, tc.want, tc.ok)
		}
	}
	if !boolValueDefault("true", false) {
		t.Fatal("boolValueDefault should parse true string")
	}
	if boolValueDefault("not-bool", false) {
		t.Fatal("boolValueDefault should use fallback for invalid string")
	}
	if !boolValueDefault(float64(1), false) {
		t.Fatal("boolValueDefault should treat non-zero float as true")
	}
	if got := mailRuleFirstString(map[string]interface{}{"id": 123}, "missing", "id"); got != "123" {
		t.Fatalf("mailRuleFirstString numeric = %q", got)
	}
	if firstPresent(map[string]any{}, "missing") != nil {
		t.Fatal("firstPresent should return nil for missing keys")
	}
	if params := decodeActionParams("move_folder", map[string]interface{}{}); params != nil {
		t.Fatalf("empty action params = %+v, want nil", params)
	}
	if got := describeConditions("any", []mailRuleCondition{{Field: "has_attachment"}, {Field: "subject", Operator: "contains"}}); !strings.Contains(got, "或") || !strings.Contains(got, "主题包含") {
		t.Fatalf("describeConditions = %q", got)
	}
	if got := describeConditions("all", nil); got != "满足未知条件" {
		t.Fatalf("empty conditions description = %q", got)
	}
	if got := describeActions(nil); got != "执行未知动作" {
		t.Fatalf("empty actions description = %q", got)
	}
}

func TestMailRuleOrderValidationErrors(t *testing.T) {
	if err := validateFullRuleOrder([]string{"a"}, []string{"a", "b"}); err == nil {
		t.Fatal("expected length mismatch error")
	}
	if err := validateFullRuleOrder([]string{"a", "a"}, []string{"a", "b"}); err == nil {
		t.Fatal("expected duplicate mismatch error")
	}
	if _, err := insertRelative([]string{"a", "b"}, "c", "", true); err == nil {
		t.Fatal("expected missing target error")
	}
	if _, err := insertRelative([]string{"a", "b"}, "c", "z", true); err == nil {
		t.Fatal("expected unknown target error")
	}

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "no mode",
			args: []string{"+rule-reorder"},
			want: "exactly one",
		},
		{
			name: "full with target",
			args: []string{"+rule-reorder", "--rule-ids", "a,b", "--before-rule-id", "a"},
			want: "cannot be combined",
		},
		{
			name: "move with no target",
			args: []string{"+rule-reorder", "--move-rule-id", "a"},
			want: "move mode requires exactly one",
		},
		{
			name: "move missing rule",
			args: []string{"+rule-reorder", "--move-rule-id", "z", "--to-top"},
			want: "is not in current rule order",
		},
		{
			name: "full mismatch",
			args: []string{"+rule-reorder", "--rule-ids", "a,z"},
			want: "mismatch",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, stdout, _, reg := mailShortcutTestFactory(t)
			if strings.Contains(tc.want, "current rule order") || strings.Contains(tc.want, "mismatch") {
				reg.Register(mailRuleListStub(mailRuleTestRawRule("a", "A"), mailRuleTestRawRule("b", "B")))
			}
			err := runMountedMailShortcut(t, MailRuleReorder, append(tc.args, "--format", "json"), f, stdout)
			if err == nil {
				t.Fatal("expected reorder error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func mailRuleTestRawRule(ruleID, name string) map[string]interface{} {
	return map[string]interface{}{
		"rule_id":                  ruleID,
		"name":                     name,
		"is_enable":                true,
		"ignore_the_rest_of_rules": false,
		"vendor_top":               "keep",
		"condition": map[string]interface{}{
			"match_type": 1,
			"items": []interface{}{
				map[string]interface{}{"type": 6, "operator": 1, "input": name},
			},
		},
		"action": map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"type": 3},
			},
		},
	}
}

func cloneMailRuleRawMap(t testing.TB, in map[string]interface{}) map[string]interface{} {
	t.Helper()
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal(rule) error = %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("Unmarshal(rule) error = %v", err)
	}
	return out
}

func mailRuleListStub(rules ...map[string]interface{}) *httpmock.Stub {
	items := make([]interface{}, 0, len(rules))
	for _, rule := range rules {
		items = append(items, rule)
	}
	return &httpmock.Stub{
		Method: "GET",
		URL:    "open-apis/mail/v1/user_mailboxes/me/rules",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"rules": items,
			},
		},
	}
}

func assertRuleIDsBody(t *testing.T, raw []byte, want string) {
	t.Helper()
	var body map[string]interface{}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("Unmarshal(reorder body) error = %v, body=%s", err, string(raw))
	}
	var got []string
	for _, item := range body["rule_ids"].([]interface{}) {
		got = append(got, item.(string))
	}
	if strings.Join(got, ",") != want {
		t.Fatalf("rule_ids = %v, want %s", got, want)
	}
}
