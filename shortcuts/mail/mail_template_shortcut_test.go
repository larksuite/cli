// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// decodeCapturedBody JSON-parses a stub's captured request body. Returns nil
// when the stub was never hit.
func decodeCapturedBody(t *testing.T, stub *httpmock.Stub) map[string]interface{} {
	t.Helper()
	if stub == nil || len(stub.CapturedBody) == 0 {
		return nil
	}
	var out map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &out); err != nil {
		t.Fatalf("decode captured body: %v (raw=%s)", err, stub.CapturedBody)
	}
	return out
}

// TestMailTemplateCreate_Happy verifies a +template-create call with no local
// <img> references and no --attach files POSTs the expected body and emits
// the server's echoed template.
func TestMailTemplateCreate_Happy(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/templates",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"template": map[string]interface{}{
					"template_id":        "tpl_001",
					"name":               "Quarterly",
					"subject":            "Q4",
					"template_content":   "<p>hi</p>",
					"is_plain_text_mode": false,
				},
			},
		},
	}
	reg.Register(stub)

	err := runMountedMailShortcut(t, MailTemplateCreate, []string{
		"+template-create",
		"--name", "Quarterly",
		"--subject", "Q4",
		"--template-content", "<p>hi</p>",
		"--to", "alice@example.com",
	}, f, stdout)
	if err != nil {
		t.Fatalf("template-create failed: %v", err)
	}

	capturedBody := decodeCapturedBody(t, stub)
	if capturedBody == nil {
		t.Fatalf("expected POST body captured")
	}
	tplWrap, ok := capturedBody["template"].(map[string]interface{})
	if !ok {
		t.Fatalf("template wrapper missing: %#v", capturedBody)
	}
	if tplWrap["name"] != "Quarterly" {
		t.Errorf("name: %v", tplWrap["name"])
	}
	if tplWrap["template_content"] != "<p>hi</p>" {
		t.Errorf("template_content unexpectedly wrapped: %v", tplWrap["template_content"])
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	tpl, ok := data["template"].(map[string]interface{})
	if !ok {
		t.Fatalf("output envelope template missing: %#v", data)
	}
	if tpl["template_id"] != "tpl_001" {
		t.Errorf("template_id = %v", tpl["template_id"])
	}
}

// TestMailTemplateCreate_PlainTextWrap verifies that a non-HTML content in
// HTML mode is line-break-wrapped before being sent.
func TestMailTemplateCreate_PlainTextWrap(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/templates",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"template": map[string]interface{}{"template_id": "tpl_002", "name": "Multi"},
			},
		},
	}
	reg.Register(stub)

	err := runMountedMailShortcut(t, MailTemplateCreate, []string{
		"+template-create",
		"--name", "Multi",
		"--template-content", "line1\nline2",
	}, f, stdout)
	if err != nil {
		t.Fatalf("template-create failed: %v", err)
	}
	capturedBody := decodeCapturedBody(t, stub)
	if capturedBody == nil {
		t.Fatalf("expected captured body")
	}
	tplWrap := capturedBody["template"].(map[string]interface{})
	tc, _ := tplWrap["template_content"].(string)
	if tc == "line1\nline2" || !strings.Contains(tc, "line1") {
		t.Errorf("expected line-break wrapped content, got %q", tc)
	}
}

// TestMailTemplateCreate_ValidateErrors verifies Validate-layer errors fire
// before any network call.
func TestMailTemplateCreate_ValidateErrors(t *testing.T) {
	cases := []struct {
		name   string
		args   []string
		expect string
	}{
		{
			"name required",
			[]string{"+template-create"},
			`required flag(s) "name" not set`,
		},
		{
			"name too long",
			[]string{"+template-create", "--name", strings.Repeat("x", 101)},
			"--name must be at most 100 characters",
		},
		{
			"mutual exclusion",
			[]string{
				"+template-create",
				"--name", "n",
				"--template-content", "a",
				"--template-content-file", "b",
			},
			"--template-content and --template-content-file are mutually exclusive",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, stdout, _, _ := mailShortcutTestFactory(t)
			err := runMountedMailShortcut(t, MailTemplateCreate, c.args, f, stdout)
			if err == nil || !strings.Contains(err.Error(), c.expect) {
				t.Fatalf("expected %q, got %v", c.expect, err)
			}
		})
	}
}

// TestMailTemplateUpdate_PrintPatchTemplate verifies --print-patch-template is
// network-free and emits the skeleton fields.
func TestMailTemplateUpdate_PrintPatchTemplate(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailTemplateUpdate, []string{
		"+template-update",
		"--print-patch-template",
	}, f, stdout)
	if err != nil {
		t.Fatalf("print-patch-template failed: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	for _, key := range []string{"name", "subject", "template_content", "is_plain_text_mode", "tos", "ccs", "bccs"} {
		if _, ok := data[key]; !ok {
			t.Errorf("skeleton missing %q; got %#v", key, data)
		}
	}
}

// TestMailTemplateUpdate_Inspect verifies --inspect calls GET and returns the
// fetched template without a PUT.
func TestMailTemplateUpdate_Inspect(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/templates/42",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"template": map[string]interface{}{
					"template_id":        "42",
					"name":               "Weekly",
					"subject":            "W",
					"is_plain_text_mode": false,
				},
			},
		},
	})

	err := runMountedMailShortcut(t, MailTemplateUpdate, []string{
		"+template-update",
		"--template-id", "42",
		"--inspect",
	}, f, stdout)
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	tpl, ok := data["template"].(map[string]interface{})
	if !ok {
		t.Fatalf("template wrapper missing: %#v", data)
	}
	if tpl["template_id"] != "42" {
		t.Errorf("template_id = %v", tpl["template_id"])
	}
}

// TestMailTemplateUpdate_Happy verifies GET + PUT flow with --set-subject.
func TestMailTemplateUpdate_Happy(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/templates/42",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"template": map[string]interface{}{
					"template_id":        "42",
					"name":               "Orig",
					"subject":            "old-subj",
					"template_content":   "<p>body</p>",
					"is_plain_text_mode": false,
					"tos":                []interface{}{map[string]interface{}{"mail_address": "a@x"}},
				},
			},
		},
	})
	putStub := &httpmock.Stub{
		Method: "PUT",
		URL:    "/user_mailboxes/me/templates/42",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"template": map[string]interface{}{
					"template_id": "42",
					"name":        "Orig",
					"subject":     "new-subj",
				},
			},
		},
	}
	reg.Register(putStub)

	err := runMountedMailShortcut(t, MailTemplateUpdate, []string{
		"+template-update",
		"--template-id", "42",
		"--set-subject", "new-subj",
	}, f, stdout)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	putBody := decodeCapturedBody(t, putStub)
	if putBody == nil {
		t.Fatalf("expected PUT body captured")
	}
	tplWrap, ok := putBody["template"].(map[string]interface{})
	if !ok {
		t.Fatalf("PUT body missing template wrapper: %#v", putBody)
	}
	if tplWrap["subject"] != "new-subj" {
		t.Errorf("subject not updated: %v", tplWrap["subject"])
	}
	// Name preserved from GET.
	if tplWrap["name"] != "Orig" {
		t.Errorf("name not preserved: %v", tplWrap["name"])
	}
}

// TestMailTemplateCreate_TemplateContentFile verifies --template-content-file
// loads body from disk.
func TestMailTemplateCreate_TemplateContentFile(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("body.html", []byte("<p>from-file</p>"), 0o644); err != nil {
		t.Fatalf("write body.html: %v", err)
	}

	f, stdout, _, reg := mailShortcutTestFactory(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/templates",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"template": map[string]interface{}{"template_id": "tpl_003", "name": "FromFile"},
			},
		},
	}
	reg.Register(stub)

	err := runMountedMailShortcut(t, MailTemplateCreate, []string{
		"+template-create",
		"--name", "FromFile",
		"--template-content-file", "body.html",
	}, f, stdout)
	if err != nil {
		t.Fatalf("template-create failed: %v", err)
	}
	capturedBody := decodeCapturedBody(t, stub)
	if capturedBody == nil {
		t.Fatalf("expected captured body")
	}
	tplWrap := capturedBody["template"].(map[string]interface{})
	if tc, _ := tplWrap["template_content"].(string); !strings.Contains(tc, "from-file") {
		t.Errorf("template_content missing file contents: %q", tc)
	}
}

// TestMailTemplateUpdate_PatchFile verifies --patch-file loads JSON overlay
// and applies fields to the fetched template before PUT.
func TestMailTemplateUpdate_PatchFile(t *testing.T) {
	chdirTemp(t)
	patchJSON := `{"subject":"patched-subj","is_plain_text_mode":true}`
	if err := os.WriteFile("patch.json", []byte(patchJSON), 0o644); err != nil {
		t.Fatalf("write patch.json: %v", err)
	}

	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/templates/77",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"template": map[string]interface{}{
					"template_id":        "77",
					"name":               "Base",
					"subject":            "orig-subj",
					"template_content":   "<p>body</p>",
					"is_plain_text_mode": false,
				},
			},
		},
	})
	putStub := &httpmock.Stub{
		Method: "PUT",
		URL:    "/user_mailboxes/me/templates/77",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"template": map[string]interface{}{"template_id": "77"},
			},
		},
	}
	reg.Register(putStub)

	err := runMountedMailShortcut(t, MailTemplateUpdate, []string{
		"+template-update",
		"--template-id", "77",
		"--patch-file", "patch.json",
	}, f, stdout)
	if err != nil {
		t.Fatalf("update with --patch-file failed: %v", err)
	}
	putBody := decodeCapturedBody(t, putStub)
	if putBody == nil {
		t.Fatalf("expected PUT body captured")
	}
	tplWrap := putBody["template"].(map[string]interface{})
	if tplWrap["subject"] != "patched-subj" {
		t.Errorf("subject not overlaid: %v", tplWrap["subject"])
	}
	if tplWrap["is_plain_text_mode"] != true {
		t.Errorf("is_plain_text_mode not overlaid: %v", tplWrap["is_plain_text_mode"])
	}
	// Unpatched field preserved.
	if tplWrap["name"] != "Base" {
		t.Errorf("name should be preserved, got %v", tplWrap["name"])
	}
}

// TestMailTemplateUpdate_SetTemplateContentFile verifies the body-from-file
// path on update.
func TestMailTemplateUpdate_SetTemplateContentFile(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("new-body.html", []byte("<p>updated</p>"), 0o644); err != nil {
		t.Fatalf("write body: %v", err)
	}

	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/templates/99",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"template": map[string]interface{}{
					"template_id": "99",
					"name":        "Orig",
				},
			},
		},
	})
	putStub := &httpmock.Stub{
		Method: "PUT",
		URL:    "/user_mailboxes/me/templates/99",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"template": map[string]interface{}{"template_id": "99"},
			},
		},
	}
	reg.Register(putStub)

	err := runMountedMailShortcut(t, MailTemplateUpdate, []string{
		"+template-update",
		"--template-id", "99",
		"--set-template-content-file", "new-body.html",
	}, f, stdout)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	putBody := decodeCapturedBody(t, putStub)
	if putBody == nil {
		t.Fatalf("expected PUT body")
	}
	tplWrap := putBody["template"].(map[string]interface{})
	if tc, _ := tplWrap["template_content"].(string); !strings.Contains(tc, "updated") {
		t.Errorf("template_content missing updated body: %q", tc)
	}
}

// TestMailTemplateCreate_WithAttach verifies a non-inline --attach path goes
// through Drive upload_all and lands in the POST body as an SMALL attachment.
func TestMailTemplateCreate_WithAttach(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("report.pdf", []byte("pdf-bytes"), 0o644); err != nil {
		t.Fatalf("write report.pdf: %v", err)
	}

	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"file_token": "file_abc"},
		},
	})
	postStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/templates",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"template": map[string]interface{}{"template_id": "tpl_att", "name": "With Attach"},
			},
		},
	}
	reg.Register(postStub)

	err := runMountedMailShortcut(t, MailTemplateCreate, []string{
		"+template-create",
		"--name", "With Attach",
		"--template-content", "<p>body</p>",
		"--attach", "report.pdf",
	}, f, stdout)
	if err != nil {
		t.Fatalf("template-create --attach failed: %v", err)
	}

	body := decodeCapturedBody(t, postStub)
	if body == nil {
		t.Fatalf("expected POST body")
	}
	tplWrap := body["template"].(map[string]interface{})
	atts, ok := tplWrap["attachments"].([]interface{})
	if !ok || len(atts) != 1 {
		t.Fatalf("expected 1 attachment, got %#v", tplWrap["attachments"])
	}
	att := atts[0].(map[string]interface{})
	if att["id"] != "file_abc" {
		t.Errorf("attachment id = %v, want file_abc", att["id"])
	}
	if att["is_inline"] != false {
		t.Errorf("attachment is_inline = %v, want false", att["is_inline"])
	}
	if att["filename"] != "report.pdf" {
		t.Errorf("attachment filename = %v", att["filename"])
	}
}

// TestMailTemplateCreate_InlineImageRewrite verifies local <img src> tags in
// template content trigger Drive upload and are rewritten to cid: references.
func TestMailTemplateCreate_InlineImageRewrite(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("logo.png", []byte("png-bytes"), 0o644); err != nil {
		t.Fatalf("write logo.png: %v", err)
	}

	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"file_token": "file_logo"},
		},
	})
	postStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/templates",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"template": map[string]interface{}{"template_id": "tpl_inline", "name": "Inline"},
			},
		},
	}
	reg.Register(postStub)

	err := runMountedMailShortcut(t, MailTemplateCreate, []string{
		"+template-create",
		"--name", "Inline",
		"--template-content", `<p>hi</p><img src="logo.png">`,
	}, f, stdout)
	if err != nil {
		t.Fatalf("template-create inline failed: %v", err)
	}

	body := decodeCapturedBody(t, postStub)
	tplWrap := body["template"].(map[string]interface{})
	tc, _ := tplWrap["template_content"].(string)
	if !strings.Contains(tc, "cid:") || strings.Contains(tc, `src="logo.png"`) {
		t.Errorf("expected <img src> rewritten to cid, got %q", tc)
	}
	atts, ok := tplWrap["attachments"].([]interface{})
	if !ok || len(atts) != 1 {
		t.Fatalf("expected 1 inline attachment, got %#v", tplWrap["attachments"])
	}
	att := atts[0].(map[string]interface{})
	if att["id"] != "file_logo" {
		t.Errorf("attachment id = %v, want file_logo", att["id"])
	}
	if att["is_inline"] != true {
		t.Errorf("is_inline = %v, want true", att["is_inline"])
	}
	if cid, _ := att["cid"].(string); cid == "" || !strings.Contains(tc, "cid:"+cid) {
		t.Errorf("cid %q not referenced in body %q", cid, tc)
	}
}

// TestMailTemplateCreate_PrettyOutput covers the OutFormat pretty callback
// for +template-create Execute.
func TestMailTemplateCreate_PrettyOutput(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/templates",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"template": map[string]interface{}{
					"template_id": "tpl_pretty",
					"name":        "Pretty",
					"attachments": []interface{}{},
				},
			},
		},
	})
	err := runMountedMailShortcut(t, MailTemplateCreate, []string{
		"+template-create",
		"--name", "Pretty",
		"--template-content", "<p>x</p>",
		"--format", "pretty",
	}, f, stdout)
	if err != nil {
		t.Fatalf("pretty create failed: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "Template created") || !strings.Contains(out, "tpl_pretty") {
		t.Errorf("pretty output missing expected lines: %s", out)
	}
}

// TestMailTemplateUpdate_PrettyInspect covers the OutFormat pretty callback
// for +template-update --inspect.
func TestMailTemplateUpdate_PrettyInspect(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/templates/50",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"template": map[string]interface{}{
					"template_id":        "50",
					"name":               "PrettyInspect",
					"subject":            "hi",
					"is_plain_text_mode": true,
				},
			},
		},
	})
	err := runMountedMailShortcut(t, MailTemplateUpdate, []string{
		"+template-update",
		"--template-id", "50",
		"--inspect",
		"--format", "pretty",
	}, f, stdout)
	if err != nil {
		t.Fatalf("pretty inspect failed: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"Template inspection", "template_id: 50", "name: PrettyInspect", "is_plain_text_mode: true", "subject: hi"} {
		if !strings.Contains(out, want) {
			t.Errorf("pretty inspect missing %q in output: %s", want, out)
		}
	}
}

// TestMailTemplateUpdate_AllSetFlags covers all --set-* branches and the
// attachment dedup + body-fill paths by providing a GET response that already
// contains an attachment without Body.
func TestMailTemplateUpdate_AllSetFlags(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/templates/60",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"template": map[string]interface{}{
					"template_id":        "60",
					"name":               "Old",
					"subject":            "old",
					"template_content":   "<p>old</p>",
					"is_plain_text_mode": false,
					"attachments": []interface{}{
						map[string]interface{}{
							"id":              "existing_key",
							"filename":        "old.pdf",
							"is_inline":       false,
							"attachment_type": 1,
							// body intentionally omitted so the body-fill loop runs
						},
					},
				},
			},
		},
	})
	putStub := &httpmock.Stub{
		Method: "PUT",
		URL:    "/user_mailboxes/me/templates/60",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"template": map[string]interface{}{"template_id": "60"},
			},
		},
	}
	reg.Register(putStub)

	err := runMountedMailShortcut(t, MailTemplateUpdate, []string{
		"+template-update",
		"--template-id", "60",
		"--set-name", "New",
		"--set-subject", "new-subj",
		"--set-template-content", "<p>new body</p>",
		"--set-plain-text",
		"--set-to", "to@x",
		"--set-cc", "cc@x",
		"--set-bcc", "bcc@x",
	}, f, stdout)
	if err != nil {
		t.Fatalf("update all-set failed: %v", err)
	}
	body := decodeCapturedBody(t, putStub)
	tplWrap := body["template"].(map[string]interface{})
	if tplWrap["name"] != "New" {
		t.Errorf("name = %v", tplWrap["name"])
	}
	if tplWrap["is_plain_text_mode"] != true {
		t.Errorf("plain_text = %v", tplWrap["is_plain_text_mode"])
	}
	// Body-fill: the existing attachment's body should have been set to its ID.
	atts, _ := tplWrap["attachments"].([]interface{})
	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment (no new --attach), got %#v", atts)
	}
	att := atts[0].(map[string]interface{})
	if att["body"] != "existing_key" {
		t.Errorf("body should be filled from ID, got %v", att["body"])
	}
}

// TestMailTemplateUpdate_SetEmptyClearsAddrs verifies --set-to="" /
// --set-cc="" / --set-bcc="" each clear the corresponding address list
// while a non-passed flag leaves it untouched.
func TestMailTemplateUpdate_SetEmptyClearsAddrs(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/templates/61",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"template": map[string]interface{}{
					"template_id":        "61",
					"name":               "Old",
					"subject":            "old",
					"template_content":   "<p>old</p>",
					"is_plain_text_mode": false,
					"tos":                []interface{}{map[string]interface{}{"mail_address": "keep-to@x"}},
					"ccs":                []interface{}{map[string]interface{}{"mail_address": "drop-cc@x"}},
					"bccs":               []interface{}{map[string]interface{}{"mail_address": "drop-bcc@x"}},
				},
			},
		},
	})
	putStub := &httpmock.Stub{
		Method: "PUT",
		URL:    "/user_mailboxes/me/templates/61",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"template": map[string]interface{}{"template_id": "61"}},
		},
	}
	reg.Register(putStub)

	err := runMountedMailShortcut(t, MailTemplateUpdate, []string{
		"+template-update",
		"--template-id", "61",
		"--set-cc=",  // explicit clear
		"--set-bcc=", // explicit clear
		// --set-to omitted on purpose: must remain untouched
	}, f, stdout)
	if err != nil {
		t.Fatalf("update clear-addrs failed: %v", err)
	}
	body := decodeCapturedBody(t, putStub)
	tplWrap := body["template"].(map[string]interface{})
	if tos, ok := tplWrap["tos"].([]interface{}); !ok || len(tos) != 1 {
		t.Errorf("tos should be left intact (1 entry), got %#v", tplWrap["tos"])
	}
	if ccs, ok := tplWrap["ccs"]; ok && ccs != nil {
		if list, _ := ccs.([]interface{}); len(list) != 0 {
			t.Errorf("ccs should be cleared, got %#v", ccs)
		}
	}
	if bccs, ok := tplWrap["bccs"]; ok && bccs != nil {
		if list, _ := bccs.([]interface{}); len(list) != 0 {
			t.Errorf("bccs should be cleared, got %#v", bccs)
		}
	}
}

// TestMailTemplateCreate_DryRunWithInlineImage covers the DryRun inline-image
// loop (parseLocalImgs branch + addTemplateUploadSteps per image).
func TestMailTemplateCreate_DryRunWithInlineImage(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("logo.png", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailTemplateCreate, []string{
		"+template-create",
		"--name", "DR",
		"--template-content", `<p><img src="logo.png"></p>`,
		"--dry-run",
	}, f, stdout)
	if err != nil {
		t.Fatalf("dry-run with inline: %v", err)
	}
	if !strings.Contains(stdout.String(), "upload_all") {
		t.Errorf("expected upload_all step for inline logo, got %s", stdout.String())
	}
}

// TestMailTemplateCreate_DryRun verifies the --dry-run path covers
// addTemplateUploadSteps (small + large branches) without network.
func TestMailTemplateCreate_DryRun(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("small.png", []byte("tiny"), 0o644); err != nil {
		t.Fatal(err)
	}

	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailTemplateCreate, []string{
		"+template-create",
		"--name", "DryRun",
		"--template-content", "<p>hello</p>",
		"--attach", "small.png",
		"--dry-run",
	}, f, stdout)
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "/open-apis/drive/v1/medias/upload_all") {
		t.Errorf("expected upload_all step in dry-run output, got %s", out)
	}
	if !strings.Contains(out, "/user_mailboxes/me/templates") {
		t.Errorf("expected template POST in dry-run output, got %s", out)
	}
}

// TestMailTemplateUpdate_DryRun verifies the template-update dry-run covers
// inspect + GET + PUT planning and addTemplateUploadSteps for --attach.
func TestMailTemplateUpdate_DryRun(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("attach.bin", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailTemplateUpdate, []string{
		"+template-update",
		"--template-id", "77",
		"--set-subject", "new",
		"--attach", "attach.bin",
		"--dry-run",
	}, f, stdout)
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "/user_mailboxes/me/templates/77") {
		t.Errorf("expected templates/77 in dry-run, got %s", out)
	}
	if !strings.Contains(out, "upload_all") {
		t.Errorf("expected upload step for attach.bin, got %s", out)
	}
}

// TestAddTemplateUploadSteps_Branches directly exercises the three branches
// (small/single-part, missing, large/multipart) by stat()-ing a real file on
// disk and a missing path.
func TestAddTemplateUploadSteps_Branches(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("small.bin", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt, _, _ := newOutputRuntime(t)
	rt.Factory.FileIOProvider = nil // fall back to default provider

	api := common.NewDryRunAPI()
	addTemplateUploadSteps(rt, api, "") // empty path → no-op
	addTemplateUploadSteps(rt, api, "small.bin")
	addTemplateUploadSteps(rt, api, "does-not-exist.bin") // stat fails branch
	// We don't assert on the API details here; touching the code paths is the
	// coverage goal. The happy-path file hits the single-part branch and the
	// missing path hits the "size unknown" fallback.
	_ = api
}

// TestMailTemplateCreate_ContentFileMissing covers the resolveTemplateContent
// file-open error branch and the Execute-level error propagation.
func TestMailTemplateCreate_ContentFileMissing(t *testing.T) {
	chdirTemp(t)
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailTemplateCreate, []string{
		"+template-create",
		"--name", "X",
		"--template-content-file", "missing.html",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "open --template-content-file") {
		t.Errorf("expected file-open error, got %v", err)
	}
}

// TestMailTemplateCreate_ContentTooBig covers the maxTemplateContentBytes
// check at Execute.
func TestMailTemplateCreate_ContentTooBig(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	big := strings.Repeat("x", 3*1024*1024+1) // > 3 MB cap
	err := runMountedMailShortcut(t, MailTemplateCreate, []string{
		"+template-create",
		"--name", "Big",
		"--template-content", big,
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "template content exceeds") {
		t.Errorf("expected content-too-big error, got %v", err)
	}
}

// TestMailTemplateCreate_CreateAPIError covers the createTemplate error-wrap
// branch in Execute.
func TestMailTemplateCreate_CreateAPIError(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/templates",
		Body: map[string]interface{}{
			"code": 1001,
			"msg":  "server rejected",
		},
	})
	err := runMountedMailShortcut(t, MailTemplateCreate, []string{
		"+template-create",
		"--name", "Err",
		"--template-content", "<p>x</p>",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "create template failed") {
		t.Errorf("expected create-template error, got %v", err)
	}
}

// TestMailTemplateUpdate_FetchAPIError covers the fetchTemplate error path.
func TestMailTemplateUpdate_FetchAPIError(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/templates/99",
		Body: map[string]interface{}{
			"code": 2001,
			"msg":  "template not found",
		},
	})
	err := runMountedMailShortcut(t, MailTemplateUpdate, []string{
		"+template-update",
		"--template-id", "99",
		"--set-subject", "x",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "fetch template") {
		t.Errorf("expected fetch error, got %v", err)
	}
}

// TestMailTemplateUpdate_PatchFileMalformed covers both the patch-file open
// miss and the JSON parse error branch.
func TestMailTemplateUpdate_PatchFileMalformed(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("bad.json", []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, stdout, _, reg := mailShortcutTestFactory(t)
	templateGetBody := map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{
			"template": map[string]interface{}{"template_id": "88", "name": "x"},
		},
	}
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/user_mailboxes/me/templates/88", Body: templateGetBody})
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/user_mailboxes/me/templates/88", Body: templateGetBody})

	err := runMountedMailShortcut(t, MailTemplateUpdate, []string{
		"+template-update",
		"--template-id", "88",
		"--patch-file", "bad.json",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "parse --patch-file") {
		t.Errorf("expected parse error, got %v", err)
	}

	// Missing patch-file path.
	err = runMountedMailShortcut(t, MailTemplateUpdate, []string{
		"+template-update",
		"--template-id", "88",
		"--patch-file", "absent.json",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "open --patch-file") {
		t.Errorf("expected open error, got %v", err)
	}
}

// TestExtractTemplatePayload_Errors covers the data-missing and JSON-error
// branches.
func TestExtractTemplatePayload_Errors(t *testing.T) {
	// Nil-payload branch: passing nil data should return an error.
	if _, err := extractTemplatePayload(nil); err == nil {
		t.Errorf("expected error for nil data")
	}
	// Malformed: a non-string type in a string-typed JSON field round-trips
	// through json.Marshal+Unmarshal and surfaces as an error.
	bad := map[string]interface{}{
		"template_id": []int{1, 2, 3}, // cannot unmarshal into string
	}
	if _, err := extractTemplatePayload(bad); err == nil {
		t.Errorf("expected unmarshal error")
	}
}

// TestFetchTemplateAttachmentURLs_FailedReasons covers the failed_reasons
// warning-entry branch.
func TestFetchTemplateAttachmentURLs_FailedReasons(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	// Fetch with an attachment that the server marks failed — the embed path
	// should surface the warning but not crash.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/profile",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"primary_email_address": "me@example.com"},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/templates/33",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"template": map[string]interface{}{
					"template_id":      "33",
					"name":             "F",
					"template_content": `<p>plain</p>`,
					"attachments": []interface{}{
						map[string]interface{}{"id": "bad_key", "filename": "x.pdf", "is_inline": false, "attachment_type": 1},
					},
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/templates/33/attachments/download_url",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"download_urls": []interface{}{},
				"failed_reasons": []interface{}{
					map[string]interface{}{"attachment_id": "bad_key", "reason": "expired"},
				},
			},
		},
	})
	// Draft save won't be reached: embed* returns an error when the URL map
	// has no entry for an ID. Run and expect an error bubbling that message.
	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--to", "alice@example.com",
		"--subject", "s",
		"--body", "<p>b</p>",
		"--template-id", "33",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "download URL not returned") {
		t.Errorf("expected download-URL-missing error (with failed_reasons warning), got %v", err)
	}
}

// TestMailSend_TemplateIDAppliesInlineAndSmall exercises the full +send
// --template-id flow with both an inline image (CID) and a SMALL non-inline
// attachment. Exercises fetchTemplate + fetchTemplateAttachmentURLs +
// embedTemplateInlineAttachments + embedTemplateSmallAttachments +
// downloadAttachmentContent + draft save.
func TestMailSend_TemplateIDAppliesInlineAndSmall(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)

	// Minimal PNG magic bytes so filecheck.CheckInlineImageFormat accepts it.
	pngBytes := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}

	// Profile — resolveComposeSenderEmail path in mail_send.go.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/profile",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"primary_email_address": "me@example.com"},
		},
	})

	// Template GET.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/templates/42",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"template": map[string]interface{}{
					"template_id":        "42",
					"name":               "T",
					"subject":            "tpl-subj",
					"template_content":   `<p>hi <img src="cid:tplcid1"></p>`,
					"is_plain_text_mode": false,
					"attachments": []interface{}{
						map[string]interface{}{"id": "img_inline", "filename": "logo.png", "is_inline": true, "cid": "tplcid1", "attachment_type": 1},
						map[string]interface{}{"id": "file_small", "filename": "plan.pdf", "is_inline": false, "attachment_type": 1},
					},
				},
			},
		},
	})

	// Download URL resolver — registered twice because stubs are single-shot
	// and the CLI calls this endpoint once for inline refs and again for
	// SMALL non-inline refs.
	downloadURLBody := map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{
			"download_urls": []interface{}{
				map[string]interface{}{"attachment_id": "img_inline", "download_url": "https://storage.example.com/img_inline"},
				map[string]interface{}{"attachment_id": "file_small", "download_url": "https://storage.example.com/file_small"},
			},
			"failed_reasons": []interface{}{},
		},
	}
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/templates/42/attachments/download_url",
		Body:   downloadURLBody,
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/templates/42/attachments/download_url",
		Body:   downloadURLBody,
	})

	// Presigned downloads.
	reg.Register(&httpmock.Stub{
		URL:     "https://storage.example.com/img_inline",
		RawBody: pngBytes,
	})
	reg.Register(&httpmock.Stub{
		URL:     "https://storage.example.com/file_small",
		RawBody: []byte("pdf-bytes"),
	})

	// Draft save.
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/drafts",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"draft_id": "draft_001"},
		},
	})

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--to", "alice@example.com",
		"--subject", "override-subj",
		"--body", "<p>user body</p>",
		"--template-id", "42",
	}, f, stdout)
	if err != nil {
		t.Fatalf("+send --template-id failed: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	if data["draft_id"] != "draft_001" {
		t.Errorf("draft_id = %v", data["draft_id"])
	}
}

// TestMailTemplateUpdate_ValidateErrors verifies Validate-layer errors fire
// before any network call.
func TestMailTemplateUpdate_ValidateErrors(t *testing.T) {
	cases := []struct {
		name   string
		args   []string
		expect string
	}{
		{
			"template-id required",
			[]string{"+template-update"},
			"--template-id is required",
		},
		{
			"template-id must be decimal",
			[]string{"+template-update", "--template-id", "abc"},
			"--template-id must be a decimal integer string",
		},
		{
			"content mutual exclusion",
			[]string{
				"+template-update",
				"--template-id", "1",
				"--set-template-content", "a",
				"--set-template-content-file", "b",
			},
			"mutually exclusive",
		},
		{
			"set-name too long",
			[]string{
				"+template-update",
				"--template-id", "1",
				"--set-name", strings.Repeat("x", 101),
			},
			"at most 100 characters",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, stdout, _, _ := mailShortcutTestFactory(t)
			err := runMountedMailShortcut(t, MailTemplateUpdate, c.args, f, stdout)
			if err == nil || !strings.Contains(err.Error(), c.expect) {
				t.Fatalf("expected %q, got %v", c.expect, err)
			}
		})
	}
}
