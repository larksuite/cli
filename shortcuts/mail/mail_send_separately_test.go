// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

// --- unit: tri-state parsing ---

func TestParseSendSeparately(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "", false},
		{"true", "true", false},
		{"false", "false", false},
		{"TRUE", "true", false},
		{" False ", "false", false},
		{"yes", "", true},
		{"1", "", true},
	}
	for _, tc := range cases {
		got, err := parseSendSeparately(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseSendSeparately(%q) expected error, got %q", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseSendSeparately(%q) unexpected error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseSendSeparately(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// --- shared helpers ---

func registerComposeProfileStub(reg *httpmock.Registry) {
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/profile",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"primary_email_address": "me@example.com"},
		},
	})
}

// ssDraftEML builds a minimal draft EML fixture, optionally carrying the
// X-Cli-Send-Separately header.
func ssDraftEML(sendSeparatelyHeader string) string {
	eml := "From: me@example.com\r\n" +
		"To: alice@example.com\r\n" +
		"Subject: hello\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n"
	if sendSeparatelyHeader != "" {
		eml += sendSeparatelyHeader + "\r\n"
	}
	return eml + "\r\nworld\r\n"
}

func ssBase64URL(eml string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(eml))
}

// decodeCapturedDraftRaw decodes the {"raw": "<base64url-EML>"} request body
// captured on a drafts create/update stub back into the plain EML.
func decodeCapturedDraftRaw(t *testing.T, stub *httpmock.Stub) string {
	t.Helper()
	var reqBody map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &reqBody); err != nil {
		t.Fatalf("unmarshal captured request body: %v", err)
	}
	raw, _ := reqBody["raw"].(string)
	decoded, err := base64.URLEncoding.DecodeString(raw)
	if err != nil {
		decoded, err = base64.RawURLEncoding.DecodeString(raw)
		if err != nil {
			t.Fatalf("base64url decode raw: %v", err)
		}
	}
	return string(decoded)
}

func ssRegisterDraftCreateStub(reg *httpmock.Registry) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/drafts",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"draft_id": "draft_ss_001"},
		},
	}
	reg.Register(stub)
	return stub
}

// --- compose paths: header serialization (explicit false must not be dropped) ---

func TestDraftCreateSendSeparatelySerializedIntoEML(t *testing.T) {
	cases := []struct {
		name    string
		flag    []string
		wantHdr string
	}{
		{"explicit true", []string{"--send-separately", "true"}, "X-Cli-Send-Separately: true"},
		{"explicit false not dropped", []string{"--send-separately", "false"}, "X-Cli-Send-Separately: false"},
		{"unset omits header", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, stdout, _, reg := mailShortcutTestFactory(t)
			registerComposeProfileStub(reg)
			draftsStub := ssRegisterDraftCreateStub(reg)

			args := append([]string{
				"+draft-create",
				"--to", "alice@example.com",
				"--subject", "s",
				"--body", "hello",
			}, tc.flag...)
			if err := runMountedMailShortcut(t, MailDraftCreate, args, f, stdout); err != nil {
				t.Fatalf("draft create failed: %v", err)
			}

			eml := decodeCapturedDraftRaw(t, draftsStub)
			if tc.wantHdr == "" {
				if strings.Contains(eml, "X-Cli-Send-Separately") {
					t.Fatalf("expected no send-separately header in EML:\n%s", eml)
				}
			} else if !strings.Contains(eml, tc.wantHdr) {
				t.Fatalf("expected %q in EML:\n%s", tc.wantHdr, eml)
			}
		})
	}
}

func TestMailSendSendSeparatelyHeaderInDraft(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactoryWithSendScope(t)
	registerComposeProfileStub(reg)
	draftsStub := ssRegisterDraftCreateStub(reg)

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--to", "alice@example.com",
		"--subject", "s",
		"--body", "world",
		"--send-separately", "true",
	}, f, stdout)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	eml := decodeCapturedDraftRaw(t, draftsStub)
	if !strings.Contains(eml, "X-Cli-Send-Separately: true") {
		t.Fatalf("expected X-Cli-Send-Separately: true in EML:\n%s", eml)
	}
}

// --- invalid input is rejected before any side effect ---

func TestDraftCreateSendSeparatelyInvalidRejectedBeforeWrite(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	draftsStub := ssRegisterDraftCreateStub(reg)
	draftsStub.Optional = true

	err := runMountedMailShortcut(t, MailDraftCreate, []string{
		"+draft-create",
		"--to", "alice@example.com",
		"--subject", "s",
		"--body", "hello",
		"--send-separately", "yes",
	}, f, stdout)
	assertValidationError(t, err, "invalid --send-separately value")
	if len(draftsStub.CapturedBodies) != 0 {
		t.Fatalf("drafts.create must not be called when --send-separately is invalid")
	}
}

func TestDraftSendSendSeparatelyInvalidRejectedBeforeAnyCall(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	getStub := &httpmock.Stub{
		Method:   "GET",
		URL:      "/user_mailboxes/me/drafts/d1",
		Optional: true,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"draft_id": "d1", "raw": ssBase64URL(ssDraftEML(""))},
		},
	}
	reg.Register(getStub)

	err := runMountedMailShortcut(t, MailDraftSend, []string{
		"+draft-send",
		"--draft-id", "d1",
		"--send-separately", "maybe",
		"--yes",
	}, f, stdout)
	assertValidationError(t, err, "invalid --send-separately value")
	if len(getStub.CapturedBodies) != 0 {
		t.Fatalf("drafts.get must not be called when --send-separately is invalid")
	}
}

// --- +draft-edit: flag → patch op, JSON conflict, projection ---

func ssWritePatchFile(t *testing.T, ops string) {
	t.Helper()
	chdirTemp(t)
	if err := os.WriteFile("patch.json", []byte(ops), 0o644); err != nil {
		t.Fatalf("write patch file: %v", err)
	}
}

func TestDraftEditSendSeparatelyFlagPatchConflict(t *testing.T) {
	ssWritePatchFile(t, `{"ops":[{"op":"set_header","name":"X-Cli-Send-Separately","value":"true"}]}`)
	f, stdout, _, _ := mailShortcutTestFactory(t)

	err := runMountedMailShortcut(t, MailDraftEdit, []string{
		"+draft-edit",
		"--draft-id", "draft_ss_001",
		"--send-separately", "false",
		"--patch-file", "patch.json",
	}, f, stdout)
	assertValidationError(t, err, "conflicts with --patch-file")
}

func TestDraftEditSendSeparatelyFlagPatchSameValueAllowed(t *testing.T) {
	ssWritePatchFile(t, `{"ops":[{"op":"set_header","name":"X-Cli-Send-Separately","value":"true"}]}`)
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/drafts/draft_ss_001",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"draft_id": "draft_ss_001", "raw": ssBase64URL(ssDraftEML(""))},
		},
	})
	putStub := &httpmock.Stub{
		Method: "PUT",
		URL:    "/user_mailboxes/me/drafts/draft_ss_001",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"draft_id": "draft_ss_001"},
		},
	}
	reg.Register(putStub)

	err := runMountedMailShortcut(t, MailDraftEdit, []string{
		"+draft-edit",
		"--draft-id", "draft_ss_001",
		"--send-separately", "true",
		"--patch-file", "patch.json",
	}, f, stdout)
	if err != nil {
		t.Fatalf("same-value flag + patch should be accepted: %v", err)
	}

	eml := decodeCapturedDraftRaw(t, putStub)
	if got := strings.Count(eml, "X-Cli-Send-Separately"); got != 1 {
		t.Fatalf("expected exactly 1 send-separately header in updated EML, got %d:\n%s", got, eml)
	}
	if !strings.Contains(eml, "X-Cli-Send-Separately: true") {
		t.Fatalf("expected X-Cli-Send-Separately: true in updated EML:\n%s", eml)
	}
}

func TestDraftEditSendSeparatelyOnlySetsHeader(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/drafts/draft_ss_001",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"draft_id": "draft_ss_001", "raw": ssBase64URL(ssDraftEML(""))},
		},
	})
	putStub := &httpmock.Stub{
		Method: "PUT",
		URL:    "/user_mailboxes/me/drafts/draft_ss_001",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"draft_id": "draft_ss_001"},
		},
	}
	reg.Register(putStub)

	err := runMountedMailShortcut(t, MailDraftEdit, []string{
		"+draft-edit",
		"--draft-id", "draft_ss_001",
		"--send-separately", "false",
	}, f, stdout)
	if err != nil {
		t.Fatalf("draft edit failed: %v", err)
	}

	eml := decodeCapturedDraftRaw(t, putStub)
	if !strings.Contains(eml, "X-Cli-Send-Separately: false") {
		t.Fatalf("expected X-Cli-Send-Separately: false in updated EML:\n%s", eml)
	}
}

func TestDraftEditInspectSendSeparatelyProjection(t *testing.T) {
	cases := []struct {
		name      string
		emlHeader string
		want      string
	}{
		// Absent header must project "unknown", never a fabricated false.
		{"absent header projects unknown", "", "unknown"},
		{"header true projects true", "X-Cli-Send-Separately: true", "true"},
		{"header false projects false", "X-Cli-Send-Separately: false", "false"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, stdout, _, reg := mailShortcutTestFactory(t)
			reg.Register(&httpmock.Stub{
				Method: "GET",
				URL:    "/user_mailboxes/me/drafts/draft_ss_001",
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{
						"draft_id": "draft_ss_001",
						"raw":      ssBase64URL(ssDraftEML(tc.emlHeader)),
					},
				},
			})

			err := runMountedMailShortcut(t, MailDraftEdit, []string{
				"+draft-edit",
				"--draft-id", "draft_ss_001",
				"--inspect",
			}, f, stdout)
			if err != nil {
				t.Fatalf("inspect failed: %v", err)
			}

			data := decodeShortcutEnvelopeData(t, stdout)
			projection, ok := data["projection"].(map[string]interface{})
			if !ok {
				t.Fatalf("projection missing from output: %#v", data)
			}
			if got := projection["send_separately"]; got != tc.want {
				t.Fatalf("send_separately = %v, want %q (a missing header must not be fabricated as false)", got, tc.want)
			}
		})
	}
}

// 更新内容不改状态: an unrelated edit must preserve an existing
// send-separately header (update keeps the stored value when the flag is unset).
func TestDraftEditSetSubjectKeepsSendSeparately(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/drafts/draft_ss_001",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"draft_id": "draft_ss_001",
				"raw":      ssBase64URL(ssDraftEML("X-Cli-Send-Separately: true")),
			},
		},
	})
	putStub := &httpmock.Stub{
		Method: "PUT",
		URL:    "/user_mailboxes/me/drafts/draft_ss_001",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"draft_id": "draft_ss_001"},
		},
	}
	reg.Register(putStub)

	err := runMountedMailShortcut(t, MailDraftEdit, []string{
		"+draft-edit",
		"--draft-id", "draft_ss_001",
		"--set-subject", "Updated subject",
	}, f, stdout)
	if err != nil {
		t.Fatalf("draft edit failed: %v", err)
	}

	eml := decodeCapturedDraftRaw(t, putStub)
	if !strings.Contains(eml, "X-Cli-Send-Separately: true") {
		t.Fatalf("unrelated edit must preserve the stored send-separately header:\n%s", eml)
	}
	if !strings.Contains(eml, "Updated subject") {
		t.Fatalf("expected updated subject in EML:\n%s", eml)
	}
}

// --- +draft-send: save-then-send override ---

func ssRegisterDraftGetStub(reg *httpmock.Registry, draftID, eml string) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/drafts/" + draftID,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"draft_id": draftID, "raw": ssBase64URL(eml)},
		},
	}
	reg.Register(stub)
	return stub
}

func TestDraftSendSendSeparatelyOverridesSaveThenSend(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	getStub := ssRegisterDraftGetStub(reg, "d1", ssDraftEML(""))
	putStub := &httpmock.Stub{
		Method: "PUT",
		URL:    "/user_mailboxes/me/drafts/d1",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"draft_id": "d1"},
		},
	}
	reg.Register(putStub)
	stubDraftSend(reg, "d1", map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{"message_id": "msg_ss_1"},
	})

	err := runMountedMailShortcut(t, MailDraftSend, []string{
		"+draft-send",
		"--draft-id", "d1",
		"--send-separately", "true",
		"--yes",
	}, f, stdout)
	if err != nil {
		t.Fatalf("draft send failed: %v", err)
	}
	reg.Verify(t) // GET + PUT + POST all consumed

	eml := decodeCapturedDraftRaw(t, putStub)
	if !strings.Contains(eml, "X-Cli-Send-Separately: true") {
		t.Fatalf("expected override header in saved draft EML:\n%s", eml)
	}
	if len(getStub.CapturedBodies) == 0 && getStub.CapturedBody == nil {
		t.Fatalf("expected the draft to be fetched before the override save")
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	sent, ok := data["sent"].([]interface{})
	if !ok || len(sent) != 1 {
		t.Fatalf("sent[] missing or wrong size: %#v", data)
	}
	entry, _ := sent[0].(map[string]interface{})
	if entry["message_id"] != "msg_ss_1" {
		t.Fatalf("message_id = %v, want msg_ss_1", entry["message_id"])
	}
}

func TestDraftSendSendSeparatelySaveFailureSkipsSend(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	ssRegisterDraftGetStub(reg, "d1", ssDraftEML(""))
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/user_mailboxes/me/drafts/d1",
		Body: map[string]interface{}{
			// Non-fatal recoverable code (mirrors the existing draft-send
			// failure fixtures) → per-draft failure, batch continues.
			"code": 230001,
			"msg":  "update draft failed",
		},
	})
	sendStub := stubDraftSend(reg, "d1", map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{"message_id": "msg_ss_1"},
	})
	sendStub.Optional = true

	err := runMountedMailShortcut(t, MailDraftSend, []string{
		"+draft-send",
		"--draft-id", "d1",
		"--send-separately", "true",
		"--yes",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected a partial-failure error when the save step fails")
	}
	if len(sendStub.CapturedBodies) != 0 {
		t.Fatalf("a draft whose save failed must NOT be sent (send was called %d times)", len(sendStub.CapturedBodies))
	}
}

func TestDraftSendWithoutFlagSendsWithoutExtraCalls(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	getStub := ssRegisterDraftGetStub(reg, "d1", ssDraftEML(""))
	getStub.Optional = true
	putStub := &httpmock.Stub{
		Method:   "PUT",
		URL:      "/user_mailboxes/me/drafts/d1",
		Optional: true,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"draft_id": "d1"},
		},
	}
	reg.Register(putStub)
	stubDraftSend(reg, "d1", map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{"message_id": "msg_ss_1"},
	})

	err := runMountedMailShortcut(t, MailDraftSend, []string{
		"+draft-send",
		"--draft-id", "d1",
		"--yes",
	}, f, stdout)
	if err != nil {
		t.Fatalf("draft send failed: %v", err)
	}
	if len(getStub.CapturedBodies) != 0 || len(putStub.CapturedBodies) != 0 {
		t.Fatalf("without --send-separately the draft must be sent as-is (no get/update calls)")
	}
}
