// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestReadSharedCardFullModesNormalizeMessageOutput(t *testing.T) {
	tests := []struct {
		format   string
		wantHTML bool
	}{
		{format: "full", wantHTML: true},
		{format: "plain_text_full", wantHTML: false},
	}
	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			f, stdout, _, reg := mailShortcutTestFactory(t)
			reg.Register(sharedCardLocatorStub([]string{"m1"}))
			reg.Register(&httpmock.Stub{
				Method: "POST",
				URL:    sharedCardDetailsPath,
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{"messages": []interface{}{map[string]interface{}{
						"message_id":      "m1",
						"body_plain_text": base64.URLEncoding.EncodeToString([]byte("plain body")),
						"body_html":       base64.URLEncoding.EncodeToString([]byte("<p>HTML body</p>")),
						"attachments": []interface{}{map[string]interface{}{
							"id": "a1", "filename": "note.txt", "attachment_type": 1,
						}},
					}}},
				},
			})

			err := runMountedMailShortcut(t, MailReadSharedCard, []string{
				"+read-shared-card", "--owner-id", "12345", "--card-id", "card_1", "--message-format", tt.format, "--format", "json",
			}, f, stdout)
			if err != nil {
				t.Fatalf("run shortcut: %v", err)
			}
			data := decodeShortcutEnvelopeData(t, stdout)
			message := data["messages"].([]interface{})[0].(map[string]interface{})
			if got := message["body_plain_text"]; got != "plain body" {
				t.Fatalf("body_plain_text = %#v, want decoded plain body", got)
			}
			if got, ok := message["body_html"]; tt.wantHTML {
				if !ok || got != "<p>HTML body</p>" {
					t.Fatalf("body_html = %#v, want decoded HTML", got)
				}
			} else if ok {
				t.Fatalf("plain_text_full unexpectedly contains body_html: %#v", got)
			}
			attachment := message["attachments"].([]interface{})[0].(map[string]interface{})
			if attachment["content_type"] != "text/plain; charset=utf-8" || attachment["is_inline"] != false {
				t.Fatalf("attachment = %#v, want normalized content_type and is_inline", attachment)
			}
		})
	}
}

func TestReadSharedCardIsRegistered(t *testing.T) {
	for _, shortcut := range Shortcuts() {
		if shortcut.Command == "+read-shared-card" {
			return
		}
	}
	t.Fatal("mail +read-shared-card is not registered")
}

func TestReadSharedCardRequiresOnlyReadScope(t *testing.T) {
	want := "mail:user_mailbox.message:readonly"
	if len(MailReadSharedCard.Scopes) != 1 || MailReadSharedCard.Scopes[0] != want {
		t.Fatalf("Scopes = %v, want [%s]", MailReadSharedCard.Scopes, want)
	}
}

func TestReadSharedCardIDOnlyUsesExistingLocator(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	locator := &httpmock.Stub{
		Method: "GET",
		URL:    sharedCardIDsPath,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"message_ids": []string{"m1", "m2", "m1"}},
		},
		OnMatch: func(req *http.Request) {
			if got := req.URL.Query().Get("owner_id"); got != "12345" {
				t.Errorf("owner_id = %q, want 12345", got)
			}
			if got := req.URL.Query().Get("card_id"); got != "card_1" {
				t.Errorf("card_id = %q, want card_1", got)
			}
			if got := req.URL.Query().Get("user_id_type"); got != "" {
				t.Errorf("user_id_type = %q, want omitted", got)
			}
		},
	}
	reg.Register(locator)

	err := runMountedMailShortcut(t, MailReadSharedCard, []string{
		"+read-shared-card", "--owner-id", "12345", "--card-id", "card_1", "--message-format", "id",
	}, f, stdout)
	if err != nil {
		t.Fatalf("run shortcut: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	if got := data["format"]; got != "id" {
		t.Fatalf("format = %#v, want id", got)
	}
	ids, _ := data["message_ids"].([]interface{})
	if len(ids) != 3 || ids[0] != "m1" || ids[1] != "m2" || ids[2] != "m1" {
		t.Fatalf("message_ids = %#v, want unchanged [m1 m2 m1]", data["message_ids"])
	}
}

func TestReadSharedCardDeduplicatesMessageIDsPreservingOrder(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(sharedCardLocatorStub([]string{"m2", "m1", "m2", "m3", "m1"}))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    sharedCardDetailsPath,
		BodyFilter: func(body []byte) bool {
			var request map[string]interface{}
			if json.Unmarshal(body, &request) != nil {
				return false
			}
			ids, ok := request["message_ids"].([]interface{})
			return ok && len(ids) == 3 && ids[0] == "m2" && ids[1] == "m1" && ids[2] == "m3"
		},
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"messages": []interface{}{
				map[string]interface{}{"message_id": "m2", "body_preview": "m2"},
				map[string]interface{}{"message_id": "m1", "body_preview": "m1"},
				map[string]interface{}{"message_id": "m3", "body_preview": "m3"},
			}},
		},
	})

	err := runMountedMailShortcut(t, MailReadSharedCard, []string{
		"+read-shared-card", "--owner-id", "12345", "--card-id", "card_1",
	}, f, stdout)
	if err != nil {
		t.Fatalf("run shortcut: %v", err)
	}
	messages := decodeShortcutEnvelopeData(t, stdout)["messages"].([]interface{})
	if len(messages) != 3 ||
		messages[0].(map[string]interface{})["message_id"] != "m2" ||
		messages[1].(map[string]interface{})["message_id"] != "m1" ||
		messages[2].(map[string]interface{})["message_id"] != "m3" {
		t.Fatalf("messages = %#v, want stable unique order [m2 m1 m3]", messages)
	}
}

func TestReadSharedCardSummaryUsesDedicatedDetailAPI(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(sharedCardLocatorStub([]string{"m1"}))
	bodyPreview := "Status update: all green."
	detail := &httpmock.Stub{
		Method: "POST",
		URL:    sharedCardDetailsPath,
		BodyFilter: func(body []byte) bool {
			var request map[string]interface{}
			return json.Unmarshal(body, &request) == nil && len(request["message_ids"].([]interface{})) == 1
		},
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"messages": []interface{}{map[string]interface{}{
				"message_id":    "m1",
				"body_preview":  bodyPreview,
				"subject":       "status report",
				"head_from":     map[string]interface{}{"name": "Alice"},
				"to":            []interface{}{map[string]interface{}{"name": "Bob"}},
				"internal_date": "1700000000000",
				"attachments":   []interface{}{},
			}}},
		},
		OnMatch: func(req *http.Request) {
			if got := req.URL.Query().Get("format"); got != "metadata" {
				t.Errorf("detail format = %q, want metadata", got)
			}
			if got := req.URL.Query().Get("owner_id"); got != "12345" {
				t.Errorf("detail owner_id = %q, want 12345", got)
			}
		},
	}
	reg.Register(detail)

	err := runMountedMailShortcut(t, MailReadSharedCard, []string{
		"+read-shared-card", "--owner-id", "12345", "--card-id", "card_1",
	}, f, stdout)
	if err != nil {
		t.Fatalf("run shortcut: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	messages := data["messages"].([]interface{})
	summary := messages[0].(map[string]interface{})
	if summary["message_id"] != "m1" || summary["body_preview"] != bodyPreview {
		t.Fatalf("summary = %#v", summary)
	}
	for _, field := range []string{"subject", "head_from", "to", "internal_date", "attachments", "body_html"} {
		if _, leaked := summary[field]; leaked {
			t.Fatalf("summary unexpectedly includes %s: %#v", field, summary)
		}
	}
}

func TestReadSharedCardBatchesMessageDetails(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	messageIDs := make([]string, 51)
	for i := range messageIDs {
		messageIDs[i] = fmt.Sprintf("m%02d", i)
	}
	reg.Register(sharedCardLocatorStub(messageIDs))
	for _, batch := range []struct {
		first string
		size  int
	}{
		{first: "m00", size: 50},
		{first: "m50", size: 1},
	} {
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    sharedCardDetailsPath,
			BodyFilter: func(body []byte) bool {
				var request map[string]interface{}
				if json.Unmarshal(body, &request) != nil {
					return false
				}
				ids, ok := request["message_ids"].([]interface{})
				return ok && len(ids) == batch.size && ids[0] == batch.first
			},
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"messages": []interface{}{map[string]interface{}{
					"message_id": batch.first, "body_preview": batch.first,
				}}},
			},
		})
	}

	err := runMountedMailShortcut(t, MailReadSharedCard, []string{
		"+read-shared-card", "--owner-id", "12345", "--card-id", "card_1",
	}, f, stdout)
	if err != nil {
		t.Fatalf("run shortcut: %v", err)
	}
	messages := decodeShortcutEnvelopeData(t, stdout)["messages"].([]interface{})
	if len(messages) != 2 || messages[0].(map[string]interface{})["message_id"] != "m00" || messages[1].(map[string]interface{})["message_id"] != "m50" {
		t.Fatalf("messages = %#v, want results from both detail batches", messages)
	}
}

func TestReadSharedCardAttachmentURLsAreOptInAndGrouped(t *testing.T) {
	f, stdout, stderr, reg := mailShortcutTestFactory(t)
	reg.Register(sharedCardLocatorStub([]string{"m1", "m2"}))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    sharedCardDetailsPath,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"messages": []interface{}{
				map[string]interface{}{
					"message_id":  "m1",
					"attachments": []interface{}{map[string]interface{}{"id": "a2"}, map[string]interface{}{"id": "a1"}, map[string]interface{}{"id": "a2"}},
				},
				map[string]interface{}{"message_id": "m2", "attachments": []interface{}{}},
			}},
		},
	})
	attachment := &httpmock.Stub{
		Method: "POST",
		URL:    sharedCardAttachmentURLsPath,
		BodyFilter: func(body []byte) bool {
			return strings.Contains(string(body), `"attachment_ids":["a2","a1"]`)
		},
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"download_urls": []interface{}{map[string]interface{}{"attachment_id": "a2", "download_url": "https://signed.example/a2?secret=do-not-log"}},
				"failed_ids":    []string{"a1"},
			},
		},
		OnMatch: func(req *http.Request) {
			if got := req.URL.Query().Get("message_id"); got != "m1" {
				t.Errorf("message_id = %q, want m1", got)
			}
			if got := req.URL.Query().Get("owner_id"); got != "12345" {
				t.Errorf("attachment owner_id = %q, want 12345", got)
			}
		},
	}
	reg.Register(attachment)

	err := runMountedMailShortcut(t, MailReadSharedCard, []string{
		"+read-shared-card", "--owner-id", "12345", "--card-id", "card_1", "--message-format", "full", "--include-attachment-download-urls",
	}, f, stdout)
	if err != nil {
		t.Fatalf("run shortcut: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	messages := data["messages"].([]interface{})
	first := messages[0].(map[string]interface{})
	second := messages[1].(map[string]interface{})
	if len(first["attachment_download_urls"].([]interface{})) != 1 || len(first["attachment_failed_ids"].([]interface{})) != 1 {
		t.Fatalf("first message attachment result = %#v", first)
	}
	if len(second["attachment_download_urls"].([]interface{})) != 0 || len(second["attachment_failed_ids"].([]interface{})) != 0 {
		t.Fatalf("no-attachment message result = %#v", second)
	}
	if strings.Contains(stderr.String(), "signed.example") || strings.Contains(stderr.String(), "secret=do-not-log") {
		t.Fatalf("stderr leaked signed URL: %s", stderr.String())
	}
}

func TestReadSharedCardBatchesAttachmentURLs(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(sharedCardLocatorStub([]string{"m1"}))
	attachments := make([]interface{}, 101)
	for i := range attachments {
		attachments[i] = map[string]interface{}{"id": fmt.Sprintf("a%03d", i)}
	}
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    sharedCardDetailsPath,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"messages": []interface{}{map[string]interface{}{
				"message_id": "m1", "attachments": attachments,
			}}},
		},
	})
	for _, batch := range []struct {
		first string
		size  int
	}{
		{first: "a000", size: 100},
		{first: "a100", size: 1},
	} {
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    sharedCardAttachmentURLsPath,
			BodyFilter: func(body []byte) bool {
				var request map[string]interface{}
				if json.Unmarshal(body, &request) != nil {
					return false
				}
				ids, ok := request["attachment_ids"].([]interface{})
				return ok && len(ids) == batch.size && ids[0] == batch.first
			},
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"download_urls": []interface{}{map[string]interface{}{"attachment_id": batch.first, "download_url": "https://signed.example/" + batch.first}},
					"failed_ids":    []interface{}{},
				},
			},
		})
	}

	err := runMountedMailShortcut(t, MailReadSharedCard, []string{
		"+read-shared-card", "--owner-id", "12345", "--card-id", "card_1", "--message-format", "full", "--include-attachment-download-urls",
	}, f, stdout)
	if err != nil {
		t.Fatalf("run shortcut: %v", err)
	}
	message := decodeShortcutEnvelopeData(t, stdout)["messages"].([]interface{})[0].(map[string]interface{})
	if got := len(message["attachment_download_urls"].([]interface{})); got != 2 {
		t.Fatalf("download URL count = %d, want 2 batches combined", got)
	}
}

func TestReadSharedCardRejectsMissingAndInvalidArguments(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing owner", args: []string{"+read-shared-card", "--card-id", "card_1"}, want: "owner-id"},
		{name: "missing card", args: []string{"+read-shared-card", "--owner-id", "12345"}, want: "card-id"},
		{name: "invalid message format", args: []string{"+read-shared-card", "--owner-id", "12345", "--card-id", "card_1", "--message-format", "raw"}, want: "allowed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, _ := mailShortcutTestFactory(t)
			err := runMountedMailShortcut(t, MailReadSharedCard, tt.args, f, stdout)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestReadSharedCardPreservesAPIErrorMetadata(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     sharedCardIDsPath,
		Headers: http.Header{"X-Tt-Logid": []string{"trace-card-denied"}},
		Body:    map[string]interface{}{"code": 15180002, "msg": "permission denied"},
	})
	err := runMountedMailShortcut(t, MailReadSharedCard, []string{
		"+read-shared-card", "--owner-id", "12345", "--card-id", "card_denied",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected API error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error = %T %v, want typed problem", err, err)
	}
	if problem.Code != 15180002 || problem.Message != "permission denied" || problem.LogID != "trace-card-denied" {
		t.Fatalf("problem = %#v, want server code/message/log_id preserved", problem)
	}
}

func TestReadSharedCardRejectsMalformedLocatorResponse(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    sharedCardIDsPath,
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
	})
	err := runMountedMailShortcut(t, MailReadSharedCard, []string{
		"+read-shared-card", "--owner-id", "12345", "--card-id", "card_1",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "missing message_ids") {
		t.Fatalf("error = %v, want invalid response for missing message_ids", err)
	}
}

func TestReadSharedCardRejectsAttachmentWithoutMessageID(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(sharedCardLocatorStub([]string{"m1"}))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    sharedCardDetailsPath,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"messages": []interface{}{map[string]interface{}{
				"attachments": []interface{}{map[string]interface{}{"id": "a1"}},
			}}},
		},
	})
	err := runMountedMailShortcut(t, MailReadSharedCard, []string{
		"+read-shared-card", "--owner-id", "12345", "--card-id", "card_1", "--message-format", "full", "--include-attachment-download-urls",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "no message_id") {
		t.Fatalf("error = %v, want invalid response for missing message_id", err)
	}
}

func sharedCardLocatorStub(messageIDs []string) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    sharedCardIDsPath,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"message_ids": messageIDs},
		},
	}
}
