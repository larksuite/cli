// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// runtimeForMailTemplateTest builds a minimal RuntimeContext with the declared
// flags of the given shortcut. Values is a map of flag name → value; bool flags
// accept "true"/"false" as strings.
func runtimeForMailTemplateTest(t *testing.T, sc common.Shortcut, values map[string]string) *common.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	for _, fl := range sc.Flags {
		switch fl.Type {
		case "bool":
			cmd.Flags().Bool(fl.Name, fl.Default == "true", "")
		case "int":
			cmd.Flags().Int(fl.Name, 0, "")
		default:
			cmd.Flags().String(fl.Name, fl.Default, "")
		}
	}
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("parse flags failed: %v", err)
	}
	for k, v := range values {
		if err := cmd.Flags().Set(k, v); err != nil {
			t.Fatalf("set --%s=%s failed: %v", k, v, err)
		}
	}
	return &common.RuntimeContext{Cmd: cmd}
}

// --- helper-level tests -----------------------------------------------------

func TestParseAddressListForAPI_ReturnsNilOnEmpty(t *testing.T) {
	if got := parseAddressListForAPI(""); got != nil {
		t.Fatalf("expected nil for empty input, got %#v", got)
	}
	if got := parseAddressListForAPI("   "); got != nil {
		t.Fatalf("expected nil for whitespace-only, got %#v", got)
	}
}

func TestParseAddressListForAPI_WithNamesAndPlainAddresses(t *testing.T) {
	got := parseAddressListForAPI(`Alice <alice@example.com>, bob@example.com`)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %#v", got)
	}
	if got[0]["mail_address"] != "alice@example.com" {
		t.Fatalf("entry[0].mail_address = %#v", got[0]["mail_address"])
	}
	if got[0]["name"] != "Alice" {
		t.Fatalf("entry[0].name = %#v", got[0]["name"])
	}
	if got[1]["mail_address"] != "bob@example.com" {
		t.Fatalf("entry[1].mail_address = %#v", got[1]["mail_address"])
	}
	if _, hasName := got[1]["name"]; hasName {
		t.Fatalf("entry[1] unexpected name: %#v", got[1])
	}
}

func TestExtractTemplateList_PrefersItems(t *testing.T) {
	data := map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{"template_id": "t1"},
		},
	}
	got := extractTemplateList(data)
	if len(got) != 1 {
		t.Fatalf("expected 1 item, got %#v", got)
	}
}

func TestExtractTemplateList_FallsBackToTemplatesKey(t *testing.T) {
	data := map[string]interface{}{
		"templates": []interface{}{map[string]interface{}{"template_id": "t1"}},
	}
	if got := extractTemplateList(data); len(got) != 1 {
		t.Fatalf("expected fallback to templates, got %#v", got)
	}
}

func TestExtractTemplateList_NilData(t *testing.T) {
	if got := extractTemplateList(nil); got != nil {
		t.Fatalf("expected nil, got %#v", got)
	}
}

func TestExtractTemplateObject_NestedAndFlat(t *testing.T) {
	nested := map[string]interface{}{
		"template": map[string]interface{}{"template_id": "t1", "name": "n"},
	}
	if got := extractTemplateObject(nested); got == nil || got["template_id"] != "t1" {
		t.Fatalf("nested extract failed: %#v", got)
	}

	flat := map[string]interface{}{"template_id": "t2", "name": "flat"}
	if got := extractTemplateObject(flat); got == nil || got["template_id"] != "t2" {
		t.Fatalf("flat extract failed: %#v", got)
	}

	if got := extractTemplateObject(map[string]interface{}{"other": "x"}); got != nil {
		t.Fatalf("expected nil for unknown shape, got %#v", got)
	}
}

func TestDescribeAddressField(t *testing.T) {
	in := []interface{}{
		map[string]interface{}{"mail_address": "alice@example.com", "name": "Alice"},
		map[string]interface{}{"mail_address": "bob@example.com"},
	}
	got := describeAddressField(in)
	if got != "Alice <alice@example.com>, bob@example.com" {
		t.Fatalf("unexpected describe: %q", got)
	}
	if describeAddressField("alice@example.com") != "alice@example.com" {
		t.Fatal("plain string passthrough failed")
	}
	if describeAddressField(nil) != "" {
		t.Fatal("nil should render empty")
	}
}

func TestStringifyTemplateAddressList(t *testing.T) {
	list := []interface{}{
		map[string]interface{}{"mail_address": "a@example.com", "name": "A"},
		map[string]interface{}{"mail_address": "b@example.com"},
	}
	if got := stringifyTemplateAddressList(list); got != "A <a@example.com>, b@example.com" {
		t.Fatalf("unexpected output: %q", got)
	}
	if got := stringifyTemplateAddressList(nil); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if got := stringifyTemplateAddressList("passthrough@example.com"); got != "passthrough@example.com" {
		t.Fatalf("expected passthrough string, got %q", got)
	}
}

func TestCoalesceStr(t *testing.T) {
	if got := coalesceStr("", " ", "first"); got != "first" {
		t.Fatalf("expected 'first', got %q", got)
	}
	if got := coalesceStr("a", "b"); got != "a" {
		t.Fatalf("expected 'a', got %q", got)
	}
	if got := coalesceStr("", "  "); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

// --- buildTemplateCreateBody: TPL-CREATE-01 regression ---------------------

// TestBuildTemplateCreateBody_BodyFlagWritesToBodyHtml is the direct regression
// test for the verification report item TPL-CREATE-01: the --body flag MUST be
// written to the inner template object's body_html field.
func TestBuildTemplateCreateBody_BodyFlagWritesToBodyHtml(t *testing.T) {
	runtime := runtimeForMailTemplateTest(t, MailTemplateCreate, map[string]string{
		"name":    "regression",
		"subject": "hello",
		"body":    "<p>Hi Alice</p>",
	})
	body := buildTemplateCreateBody(runtime)
	got, ok := body["body_html"].(string)
	if !ok {
		t.Fatalf("body_html missing or not string in request body: %#v", body)
	}
	if got != "<p>Hi Alice</p>" {
		t.Fatalf("body_html = %q, want the exact --body value", got)
	}
	if body["name"] != "regression" {
		t.Fatalf("name mismatch: %#v", body["name"])
	}
	if body["subject"] != "hello" {
		t.Fatalf("subject mismatch: %#v", body["subject"])
	}
	if _, hasPlain := body["is_plain_text_mode"]; hasPlain {
		t.Fatalf("is_plain_text_mode should be absent when --plain-text is not set")
	}
}

func TestBuildTemplateCreateBody_OmitsEmptyFields(t *testing.T) {
	runtime := runtimeForMailTemplateTest(t, MailTemplateCreate, map[string]string{
		"name": "only-name",
	})
	body := buildTemplateCreateBody(runtime)
	if body["name"] != "only-name" {
		t.Fatalf("name mismatch: %#v", body)
	}
	for _, key := range []string{"subject", "body_html", "tos", "ccs", "bccs", "is_plain_text_mode"} {
		if _, ok := body[key]; ok {
			t.Fatalf("unexpected %q in minimal body: %#v", key, body)
		}
	}
}

func TestBuildTemplateCreateBody_IncludesRecipientsAndPlainText(t *testing.T) {
	runtime := runtimeForMailTemplateTest(t, MailTemplateCreate, map[string]string{
		"name":       "full",
		"to":         "alice@example.com",
		"cc":         "Bob <bob@example.com>",
		"bcc":        "carol@example.com",
		"plain-text": "true",
		"body":       "plaintext content",
	})
	body := buildTemplateCreateBody(runtime)
	if body["is_plain_text_mode"] != true {
		t.Fatalf("is_plain_text_mode = %#v, want true", body["is_plain_text_mode"])
	}
	if body["body_html"] != "plaintext content" {
		t.Fatalf("body_html should be the --body value even with --plain-text; got %#v", body["body_html"])
	}
	tos, ok := body["tos"].([]map[string]interface{})
	if !ok || len(tos) != 1 || tos[0]["mail_address"] != "alice@example.com" {
		t.Fatalf("tos mismatch: %#v", body["tos"])
	}
	ccs, ok := body["ccs"].([]map[string]interface{})
	if !ok || len(ccs) != 1 || ccs[0]["mail_address"] != "bob@example.com" || ccs[0]["name"] != "Bob" {
		t.Fatalf("ccs mismatch: %#v", body["ccs"])
	}
	bccs, ok := body["bccs"].([]map[string]interface{})
	if !ok || len(bccs) != 1 || bccs[0]["mail_address"] != "carol@example.com" {
		t.Fatalf("bccs mismatch: %#v", body["bccs"])
	}
}

// --- shortcut metadata sanity checks ---------------------------------------

func TestMailTemplateShortcutsMetadata(t *testing.T) {
	cases := []struct {
		s        common.Shortcut
		cmd      string
		risk     string
		mustScope []string
	}{
		{MailTemplateList, "+template-list", "read", []string{"mail:user_mailbox.message:readonly"}},
		{MailTemplateGet, "+template-get", "read", []string{"mail:user_mailbox.message:readonly"}},
		{MailTemplateCreate, "+template-create", "write", []string{"mail:user_mailbox.message:modify"}},
		{MailTemplateUpdate, "+template-update", "write", []string{"mail:user_mailbox.message:modify"}},
		{MailTemplateDelete, "+template-delete", "delete", []string{"mail:user_mailbox.message:modify"}},
		{MailTemplateSend, "+template-send", "write", []string{
			"mail:user_mailbox.message:readonly",
			"mail:user_mailbox.message:send",
			"mail:user_mailbox.message:modify",
		}},
	}
	for _, tc := range cases {
		if tc.s.Command != tc.cmd {
			t.Errorf("%s: Command = %q", tc.cmd, tc.s.Command)
		}
		if tc.s.Risk != tc.risk {
			t.Errorf("%s: Risk = %q, want %q", tc.cmd, tc.s.Risk, tc.risk)
		}
		if len(tc.s.AuthTypes) == 0 || tc.s.AuthTypes[0] != "user" {
			t.Errorf("%s: expected AuthTypes contains 'user', got %#v", tc.cmd, tc.s.AuthTypes)
		}
		for _, must := range tc.mustScope {
			if !containsString(tc.s.Scopes, must) {
				t.Errorf("%s: missing scope %q (got %#v)", tc.cmd, must, tc.s.Scopes)
			}
		}
	}
}

func TestMailTemplateShortcutsRegistered(t *testing.T) {
	set := make(map[string]bool)
	for _, sc := range Shortcuts() {
		set[sc.Command] = true
	}
	for _, want := range []string{
		"+template-list", "+template-get", "+template-create",
		"+template-update", "+template-delete", "+template-send",
	} {
		if !set[want] {
			t.Errorf("Shortcuts() missing %q (got %v)", want, set)
		}
	}
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// --- validation tests ------------------------------------------------------

func TestMailTemplateCreateValidate_NameRequired(t *testing.T) {
	runtime := runtimeForMailTemplateTest(t, MailTemplateCreate, map[string]string{})
	err := MailTemplateCreate.Validate(context.Background(), runtime)
	if err == nil {
		t.Fatal("expected validation error when --name is empty")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != output.ExitValidation {
		t.Fatalf("expected ExitValidation, got %T: %v", err, err)
	}
}

func TestMailTemplateCreateValidate_NameTooLong(t *testing.T) {
	long := strings.Repeat("x", 101)
	runtime := runtimeForMailTemplateTest(t, MailTemplateCreate, map[string]string{"name": long})
	err := MailTemplateCreate.Validate(context.Background(), runtime)
	if err == nil || !strings.Contains(err.Error(), "100") {
		t.Fatalf("expected 100-char limit error, got %v", err)
	}
}

func TestMailTemplateCreateValidate_UnicodeNameUnder100(t *testing.T) {
	// 100 runes of a 4-byte emoji fits the rune budget exactly.
	name := strings.Repeat("模", 100)
	runtime := runtimeForMailTemplateTest(t, MailTemplateCreate, map[string]string{"name": name})
	if err := MailTemplateCreate.Validate(context.Background(), runtime); err != nil {
		t.Fatalf("expected no error for 100-rune name, got %v", err)
	}
}

func TestMailTemplateGetValidate_RequiresTemplateID(t *testing.T) {
	runtime := runtimeForMailTemplateTest(t, MailTemplateGet, map[string]string{})
	if err := MailTemplateGet.Validate(context.Background(), runtime); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestMailTemplateUpdateValidate_RequiresTemplateID(t *testing.T) {
	runtime := runtimeForMailTemplateTest(t, MailTemplateUpdate, map[string]string{})
	if err := MailTemplateUpdate.Validate(context.Background(), runtime); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestMailTemplateDeleteValidate_RequiresTemplateID(t *testing.T) {
	runtime := runtimeForMailTemplateTest(t, MailTemplateDelete, map[string]string{})
	if err := MailTemplateDelete.Validate(context.Background(), runtime); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestMailTemplateSendValidate_RequiresTemplateID(t *testing.T) {
	runtime := runtimeForMailTemplateTest(t, MailTemplateSend, map[string]string{})
	if err := MailTemplateSend.Validate(context.Background(), runtime); err == nil {
		t.Fatal("expected validation error")
	}
}

// --- dry-run tests ---------------------------------------------------------

func TestMailTemplateListDryRun(t *testing.T) {
	runtime := runtimeForMailTemplateTest(t, MailTemplateList, map[string]string{})
	dry := MailTemplateList.DryRun(context.Background(), runtime)
	calls := dryRunAPIsForMailTemplateTest(t, dry)
	if len(calls) != 1 || calls[0].Method != "GET" {
		t.Fatalf("unexpected dry-run calls: %#v", calls)
	}
	if !strings.HasSuffix(calls[0].URL, "/user_mailboxes/me/templates") {
		t.Fatalf("unexpected URL: %s", calls[0].URL)
	}
}

func TestMailTemplateGetDryRun(t *testing.T) {
	runtime := runtimeForMailTemplateTest(t, MailTemplateGet, map[string]string{"template-id": "tpl_1"})
	dry := MailTemplateGet.DryRun(context.Background(), runtime)
	calls := dryRunAPIsForMailTemplateTest(t, dry)
	if len(calls) != 1 || calls[0].Method != "GET" {
		t.Fatalf("unexpected dry-run calls: %#v", calls)
	}
	if !strings.HasSuffix(calls[0].URL, "/templates/tpl_1") {
		t.Fatalf("unexpected URL: %s", calls[0].URL)
	}
}

func TestMailTemplateCreateDryRun(t *testing.T) {
	runtime := runtimeForMailTemplateTest(t, MailTemplateCreate, map[string]string{
		"name":    "n",
		"subject": "s",
		"body":    "<p>b</p>",
		"to":      "alice@example.com",
	})
	dry := MailTemplateCreate.DryRun(context.Background(), runtime)
	calls := dryRunAPIsForMailTemplateTest(t, dry)
	if len(calls) != 1 || calls[0].Method != "POST" {
		t.Fatalf("unexpected dry-run calls: %#v", calls)
	}
	// The HTTP body must wrap the template object per IDL api.body="template":
	//   {"template": {"name": "...", "body_html": "...", "tos": [...]}}
	wrap, _ := calls[0].Body.(map[string]interface{})
	if wrap == nil {
		t.Fatalf("expected body in dry-run, got %#v", calls[0].Body)
	}
	inner, ok := wrap["template"].(map[string]interface{})
	if !ok {
		t.Fatalf("dry-run body should be wrapped under \"template\"; got %#v", wrap)
	}
	if inner["body_html"] != "<p>b</p>" {
		t.Fatalf("body_html not in wrapped template: %#v", inner)
	}
	if inner["name"] != "n" || inner["subject"] != "s" {
		t.Fatalf("inner name/subject mismatch: %#v", inner)
	}
	// After json round-trip the array element type is []interface{} of map[string]interface{}.
	tos, ok := inner["tos"].([]interface{})
	if !ok || len(tos) != 1 {
		t.Fatalf("inner tos mismatch: %#v", inner["tos"])
	}
	firstTo, _ := tos[0].(map[string]interface{})
	if firstTo["mail_address"] != "alice@example.com" {
		t.Fatalf("inner tos[0].mail_address = %#v", firstTo["mail_address"])
	}
	if _, flat := wrap["body_html"]; flat {
		t.Fatalf("dry-run body must not be flat; got body_html at top level: %#v", wrap)
	}
}

func TestMailTemplateUpdateDryRun(t *testing.T) {
	runtime := runtimeForMailTemplateTest(t, MailTemplateUpdate, map[string]string{
		"template-id": "tpl_1",
	})
	dry := MailTemplateUpdate.DryRun(context.Background(), runtime)
	calls := dryRunAPIsForMailTemplateTest(t, dry)
	if len(calls) != 2 {
		t.Fatalf("expected 2 dry-run calls (GET then PUT), got %#v", calls)
	}
	if calls[0].Method != "GET" || calls[1].Method != "PUT" {
		t.Fatalf("unexpected dry-run sequence: %#v", calls)
	}
}

func TestMailTemplateDeleteDryRun(t *testing.T) {
	runtime := runtimeForMailTemplateTest(t, MailTemplateDelete, map[string]string{"template-id": "tpl_1"})
	dry := MailTemplateDelete.DryRun(context.Background(), runtime)
	calls := dryRunAPIsForMailTemplateTest(t, dry)
	if len(calls) != 1 || calls[0].Method != "DELETE" {
		t.Fatalf("unexpected dry-run calls: %#v", calls)
	}
}

func TestMailTemplateSendDryRun_DraftOnly(t *testing.T) {
	runtime := runtimeForMailTemplateTest(t, MailTemplateSend, map[string]string{"template-id": "tpl_1"})
	dry := MailTemplateSend.DryRun(context.Background(), runtime)
	calls := dryRunAPIsForMailTemplateTest(t, dry)
	if len(calls) != 3 {
		t.Fatalf("expected 3 dry-run calls for draft-only send, got %#v", calls)
	}
	if calls[0].Method != "GET" || calls[2].Method != "POST" {
		t.Fatalf("unexpected dry-run sequence: %#v", calls)
	}
}

func TestMailTemplateSendDryRun_ConfirmSend(t *testing.T) {
	runtime := runtimeForMailTemplateTest(t, MailTemplateSend, map[string]string{
		"template-id":  "tpl_1",
		"confirm-send": "true",
	})
	dry := MailTemplateSend.DryRun(context.Background(), runtime)
	calls := dryRunAPIsForMailTemplateTest(t, dry)
	if len(calls) != 4 {
		t.Fatalf("expected 4 dry-run calls with --confirm-send, got %#v", calls)
	}
	if calls[3].Method != "POST" || !strings.Contains(calls[3].URL, "/send") {
		t.Fatalf("last call should be POST .../send, got %#v", calls[3])
	}
}

// dryRunAPIsForMailTemplateTest marshals a DryRunAPI into its nested API list.
func dryRunAPIsForMailTemplateTest(t *testing.T, dry *common.DryRunAPI) []struct {
	Method string                 `json:"method"`
	URL    string                 `json:"url"`
	Params map[string]interface{} `json:"params,omitempty"`
	Body   interface{}            `json:"body"`
} {
	t.Helper()
	b, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry-run: %v", err)
	}
	var payload struct {
		API []struct {
			Method string                 `json:"method"`
			URL    string                 `json:"url"`
			Params map[string]interface{} `json:"params,omitempty"`
			Body   interface{}            `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		t.Fatalf("unmarshal dry-run: %v", err)
	}
	return payload.API
}

// --- mergeTemplateSendFields ----------------------------------------------

func TestMergeTemplateSendFields_UsesTemplateDefaultsWhenFlagsEmpty(t *testing.T) {
	runtime := runtimeForMailTemplateTest(t, MailTemplateSend, map[string]string{
		"template-id": "tpl_1",
	})
	tmpl := map[string]interface{}{
		"subject":            "Tmpl subject",
		"body_html":          "<p>Tmpl body</p>",
		"is_plain_text_mode": false,
		"to": []interface{}{
			map[string]interface{}{"mail_address": "alice@example.com", "name": "Alice"},
		},
		"cc": []interface{}{
			map[string]interface{}{"mail_address": "cc@example.com"},
		},
	}
	merged, err := mergeTemplateSendFields(runtime, tmpl)
	if err != nil {
		t.Fatalf("mergeTemplateSendFields: %v", err)
	}
	if merged.Subject != "Tmpl subject" {
		t.Errorf("subject = %q", merged.Subject)
	}
	if merged.Body != "<p>Tmpl body</p>" {
		t.Errorf("body = %q", merged.Body)
	}
	if merged.To != "Alice <alice@example.com>" {
		t.Errorf("to = %q", merged.To)
	}
	if merged.CC != "cc@example.com" {
		t.Errorf("cc = %q", merged.CC)
	}
	if merged.PlainText {
		t.Errorf("plain-text should be false")
	}
}

func TestMergeTemplateSendFields_FlagsOverrideTemplate(t *testing.T) {
	runtime := runtimeForMailTemplateTest(t, MailTemplateSend, map[string]string{
		"template-id": "tpl_1",
		"subject":     "Flag subject",
		"body":        "Flag body",
		"to":          "override@example.com",
	})
	tmpl := map[string]interface{}{
		"subject":   "Tmpl subject",
		"body_html": "<p>Tmpl body</p>",
		"to": []interface{}{
			map[string]interface{}{"mail_address": "alice@example.com"},
		},
	}
	merged, err := mergeTemplateSendFields(runtime, tmpl)
	if err != nil {
		t.Fatalf("mergeTemplateSendFields: %v", err)
	}
	if merged.Subject != "Flag subject" || merged.Body != "Flag body" || merged.To != "override@example.com" {
		t.Fatalf("flag override failed: %+v", merged)
	}
}

func TestMergeTemplateSendFields_ErrorsWhenNoRecipient(t *testing.T) {
	runtime := runtimeForMailTemplateTest(t, MailTemplateSend, map[string]string{
		"template-id": "tpl_1",
	})
	tmpl := map[string]interface{}{"subject": "s", "body_html": "b"}
	_, err := mergeTemplateSendFields(runtime, tmpl)
	if err == nil {
		t.Fatal("expected error when template has no default To and --to is empty")
	}
	if !strings.Contains(err.Error(), "no recipients") {
		t.Fatalf("expected 'no recipients' error, got: %v", err)
	}
}

// --- buildTemplateEML ------------------------------------------------------

func TestBuildTemplateEML_PlainTextBody(t *testing.T) {
	merged := templateSendFields{
		Subject:   "hi",
		Body:      "plain body",
		To:        "alice@example.com",
		PlainText: true,
	}
	raw, err := buildTemplateEML(merged, "sender@example.com")
	if err != nil {
		t.Fatalf("buildTemplateEML: %v", err)
	}
	eml := decodeBase64URL(raw)
	if !strings.Contains(eml, "alice@example.com") {
		t.Fatal("missing To address in EML")
	}
	if !strings.Contains(eml, "Subject: hi") {
		t.Fatal("missing Subject in EML")
	}
	if strings.Contains(eml, "Content-Type: text/html") {
		t.Fatal("plain-text body should not emit text/html part")
	}
}

func TestBuildTemplateEML_HTMLBody(t *testing.T) {
	merged := templateSendFields{
		Subject: "hi",
		Body:    "<p>hello</p>",
		To:      "alice@example.com",
	}
	raw, err := buildTemplateEML(merged, "sender@example.com")
	if err != nil {
		t.Fatalf("buildTemplateEML: %v", err)
	}
	eml := decodeBase64URL(raw)
	if !strings.Contains(eml, "text/html") {
		t.Fatal("expected HTML part in EML when body is HTML")
	}
}

// --- merge update flags ----------------------------------------------------

func TestMergeTemplateUpdateFlags_OverlaysNonEmptyFields(t *testing.T) {
	runtime := runtimeForMailTemplateTest(t, MailTemplateUpdate, map[string]string{
		"template-id": "tpl_1",
		"name":        "new-name",
		"body":        "<p>new</p>",
		"to":          "x@example.com",
	})
	tmpl := map[string]interface{}{
		"template_id": "tpl_1",
		"name":        "old",
		"subject":     "keep-subject",
		"body_html":   "<p>old</p>",
	}
	mergeTemplateUpdateFlags(runtime, tmpl)
	if tmpl["name"] != "new-name" {
		t.Errorf("name not overlayed: %#v", tmpl["name"])
	}
	if tmpl["subject"] != "keep-subject" {
		t.Errorf("subject should be preserved when flag is empty: %#v", tmpl["subject"])
	}
	if tmpl["body_html"] != "<p>new</p>" {
		t.Errorf("body_html not overlayed: %#v", tmpl["body_html"])
	}
	tos, ok := tmpl["tos"].([]map[string]interface{})
	if !ok || len(tos) != 1 || tos[0]["mail_address"] != "x@example.com" {
		t.Errorf("tos not overlayed: %#v", tmpl["tos"])
	}
}

// --- end-to-end httpmock tests ---------------------------------------------

func TestMailTemplateList_E2E(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/templates",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"template_id": "tpl_1",
						"name":        "周报模板",
						"subject":     "Weekly report",
						"create_time": "1700000000000",
					},
				},
			},
		},
	})
	err := runMountedMailShortcut(t, MailTemplateList, []string{"+template-list"}, f, stdout)
	if err != nil {
		t.Fatalf("runMountedMailShortcut: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	items, _ := data["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d: %s", len(items), stdout.String())
	}
	first := items[0].(map[string]interface{})
	if first["template_id"] != "tpl_1" {
		t.Errorf("template_id mismatch: %#v", first["template_id"])
	}
}

func TestMailTemplateCreate_E2E_WritesBodyHtml(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)

	createStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/templates",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"template": map[string]interface{}{
					"template_id": "tpl_new",
					"name":        "My Tpl",
				},
			},
		},
	}
	reg.Register(createStub)

	err := runMountedMailShortcut(t, MailTemplateCreate, []string{
		"+template-create",
		"--name", "My Tpl",
		"--subject", "hello",
		"--body", "<p>Body from CLI</p>",
		"--to", "alice@example.com",
	}, f, stdout)
	if err != nil {
		t.Fatalf("runMountedMailShortcut: %v", err)
	}

	var wrap map[string]interface{}
	if err := json.Unmarshal(createStub.CapturedBody, &wrap); err != nil {
		t.Fatalf("unmarshal captured body: %v (raw=%s)", err, string(createStub.CapturedBody))
	}
	// HTTP body must be wrapped per IDL api.body="template":
	//   {"template": {"name": "...", "tos": [{...}]}}
	captured, ok := wrap["template"].(map[string]interface{})
	if !ok {
		t.Fatalf("captured body is not wrapped under \"template\": %s", string(createStub.CapturedBody))
	}
	if got := captured["body_html"]; got != "<p>Body from CLI</p>" {
		t.Fatalf("captured body_html = %#v, want the --body value", got)
	}
	if captured["name"] != "My Tpl" {
		t.Errorf("captured name = %#v", captured["name"])
	}
	if captured["subject"] != "hello" {
		t.Errorf("captured subject = %#v", captured["subject"])
	}
	tosList, ok := captured["tos"].([]interface{})
	if !ok || len(tosList) == 0 {
		t.Fatalf("tos missing in captured template: %#v", captured["tos"])
	}
	first, _ := tosList[0].(map[string]interface{})
	if first["mail_address"] != "alice@example.com" {
		t.Errorf("tos[0].mail_address = %#v", first["mail_address"])
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	if data["template_id"] != "tpl_new" {
		t.Errorf("template_id mismatch: %#v", data["template_id"])
	}
}

func TestMailTemplateDelete_E2E(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/templates/tpl_1",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{},
		},
	})
	err := runMountedMailShortcut(t, MailTemplateDelete, []string{
		"+template-delete",
		"--template-id", "tpl_1",
	}, f, stdout)
	if err != nil {
		t.Fatalf("runMountedMailShortcut: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	if data["deleted"] != true {
		t.Fatalf("expected deleted=true, got %#v", data)
	}
	if data["template_id"] != "tpl_1" {
		t.Errorf("template_id mismatch: %#v", data["template_id"])
	}
}

func TestMailTemplateGet_E2E(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/templates/tpl_1",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"template": map[string]interface{}{
					"template_id": "tpl_1",
					"name":        "周报",
					"subject":     "Weekly",
					"body_html":   "<p>hi</p>",
				},
			},
		},
	})
	err := runMountedMailShortcut(t, MailTemplateGet, []string{
		"+template-get",
		"--template-id", "tpl_1",
	}, f, stdout)
	if err != nil {
		t.Fatalf("runMountedMailShortcut: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	if data["template_id"] != "tpl_1" {
		t.Fatalf("template_id mismatch: %#v", data)
	}
	if data["name"] != "周报" {
		t.Errorf("name mismatch: %#v", data["name"])
	}
}

func TestMailTemplateUpdate_E2E_MergesBeforePut(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	// GET returns existing template
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/templates/tpl_1",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"template": map[string]interface{}{
					"template_id": "tpl_1",
					"name":        "old-name",
					"subject":     "old-subject",
					"body_html":   "<p>old</p>",
				},
			},
		},
	})
	putStub := &httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/templates/tpl_1",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"template": map[string]interface{}{"template_id": "tpl_1", "name": "new-name"},
			},
		},
	}
	reg.Register(putStub)

	err := runMountedMailShortcut(t, MailTemplateUpdate, []string{
		"+template-update",
		"--template-id", "tpl_1",
		"--name", "new-name",
	}, f, stdout)
	if err != nil {
		t.Fatalf("runMountedMailShortcut: %v", err)
	}

	var wrap map[string]interface{}
	if err := json.Unmarshal(putStub.CapturedBody, &wrap); err != nil {
		t.Fatalf("unmarshal captured PUT body: %v", err)
	}
	// PUT body must be wrapped per IDL api.body="template".
	captured, ok := wrap["template"].(map[string]interface{})
	if !ok {
		t.Fatalf("captured PUT body is not wrapped under \"template\": %s", string(putStub.CapturedBody))
	}
	if captured["name"] != "new-name" {
		t.Errorf("PUT name = %#v, want new-name", captured["name"])
	}
	// subject should be preserved from the GET response (merge semantics)
	if captured["subject"] != "old-subject" {
		t.Errorf("PUT should preserve unspecified subject, got %#v", captured["subject"])
	}
	if captured["body_html"] != "<p>old</p>" {
		t.Errorf("PUT should preserve unspecified body_html, got %#v", captured["body_html"])
	}
}
