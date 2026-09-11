// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestMailMessageCitationFromApplink(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CITATION", "1")
	f, stdout, _, reg := mailShortcutTestFactory(t)
	defer reg.Verify(t)

	const applink = "https://applink.feishu.cn/client/mail/message?thread=thread_1&message=msg_1"
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    mailboxPath("me", "messages", "msg_001") + "?format=full",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"message": map[string]interface{}{
					"message_id":        "msg_001",
					"thread_id":         "thread_001",
					"subject":           "Quarterly plan",
					"body_preview":      "Preview text",
					"internal_date":     "1700000000000",
					"body_plain_text":   "",
					"body_html":         "",
					"message_state":     1,
					"applink":           applink,
					"need_read_receipt": false,
				},
			},
		},
	})

	if err := runMountedMailShortcut(t, MailMessage, []string{
		"+message", "--message-id", "msg_001",
	}, f, stdout); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	citation := singleCitation(t, stdout)
	if citation["source_type"] != float64(12) {
		t.Fatalf("source_type = %v, want 12", citation["source_type"])
	}
	if citation["url"] != applink {
		t.Fatalf("url = %v, want %s", citation["url"], applink)
	}
	if citation["title"] != "Quarterly plan" || citation["snippet"] != "Preview text" {
		t.Fatalf("citation title/snippet mismatch: %#v", citation)
	}
	if citation["publish_time"] != "2023-11-14T22:13:20Z" {
		t.Fatalf("publish_time = %v", citation["publish_time"])
	}
}

func TestMailTriageCitationFromBatchGetApplink(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CITATION", "1")
	f, stdout, _, reg := mailShortcutTestFactory(t)
	defer reg.Verify(t)

	registerMailTriageListStub(reg, "me", []string{"msg_001", "msg_002"}, false, "")
	registerMailTriageBatchStub(reg, "me", []map[string]interface{}{
		mailTriageBatchMessageWithCitation("msg_001", "First", "https://applink.feishu.cn/client/mail/message?message=msg_001"),
		mailTriageBatchMessageWithCitation("msg_002", "Second", "https://applink.feishu.cn/client/mail/message?message=msg_002"),
	})

	if err := runMountedMailShortcut(t, MailTriage, []string{
		"+triage", "--format", "json", "--filter", `{"folder_id":"INBOX"}`,
	}, f, stdout); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	citations := envelopeCitations(t, stdout)
	if len(citations) != 2 {
		t.Fatalf("citations len = %d, want 2; stdout=%s", len(citations), stdout.String())
	}
	if citations[0]["title"] != "First" || citations[1]["title"] != "Second" {
		t.Fatalf("citation order/title mismatch: %#v", citations)
	}
	if citations[0]["snippet"] != "Preview msg_001" {
		t.Fatalf("snippet = %v", citations[0]["snippet"])
	}
}

func TestMailTriageSearchWithoutApplinkDropsCitation(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CITATION", "1")
	f, stdout, _, reg := mailShortcutTestFactory(t)
	defer reg.Verify(t)

	registerMailTriageSearchStub(reg, "me", []interface{}{
		mailTriageSearchItem("msg_search", "Search result"),
	}, false, "")

	if err := runMountedMailShortcut(t, MailTriage, []string{
		"+triage", "--format", "json", "--query", "keyword",
	}, f, stdout); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if citations := envelopeCitations(t, stdout); len(citations) != 0 {
		t.Fatalf("citations = %#v, want none", citations)
	}
}

func TestMailCitationDisabledAndTableOutputStayUnchanged(t *testing.T) {
	t.Run("env disabled", func(t *testing.T) {
		f, stdout, _, reg := mailShortcutTestFactory(t)
		defer reg.Verify(t)

		registerMailTriageListStub(reg, "me", []string{"msg_001"}, false, "")
		registerMailTriageBatchStub(reg, "me", []map[string]interface{}{
			mailTriageBatchMessageWithCitation("msg_001", "First", "https://applink.feishu.cn/client/mail/message?message=msg_001"),
		})

		if err := runMountedMailShortcut(t, MailTriage, []string{
			"+triage", "--format", "json", "--filter", `{"folder_id":"INBOX"}`,
		}, f, stdout); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(stdout.String(), `"citations"`) {
			t.Fatalf("stdout should not contain citations when env is disabled: %s", stdout.String())
		}
	})

	t.Run("table format", func(t *testing.T) {
		t.Setenv("LARKSUITE_CLI_CITATION", "1")
		f, stdout, _, reg := mailShortcutTestFactory(t)
		defer reg.Verify(t)

		registerMailTriageListStub(reg, "me", []string{"msg_001"}, false, "")
		registerMailTriageBatchStub(reg, "me", []map[string]interface{}{
			mailTriageBatchMessageWithCitation("msg_001", "First", "https://applink.feishu.cn/client/mail/message?message=msg_001"),
		})

		if err := runMountedMailShortcut(t, MailTriage, []string{
			"+triage", "--format", "table", "--filter", `{"folder_id":"INBOX"}`,
		}, f, stdout); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(stdout.String(), "citations") {
			t.Fatalf("table output should not contain citations: %s", stdout.String())
		}
	})
}

func envelopeCitations(t *testing.T, stdout *bytes.Buffer) []map[string]interface{} {
	t.Helper()
	var envelope struct {
		OK        bool          `json:"ok"`
		Citations []interface{} `json:"citations"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal stdout: %v; stdout=%s", err, stdout.String())
	}
	if !envelope.OK {
		t.Fatalf("expected ok envelope: %s", stdout.String())
	}
	out := make([]map[string]interface{}, 0, len(envelope.Citations))
	for i, item := range envelope.Citations {
		citation, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("citations[%d] type = %T", i, item)
		}
		out = append(out, citation)
	}
	return out
}

func singleCitation(t *testing.T, stdout *bytes.Buffer) map[string]interface{} {
	t.Helper()
	citations := envelopeCitations(t, stdout)
	if len(citations) != 1 {
		t.Fatalf("citations len = %d, want 1; stdout=%s", len(citations), stdout.String())
	}
	return citations[0]
}

func mailTriageBatchMessageWithCitation(messageID, subject, applink string) map[string]interface{} {
	msg := mailTriageBatchMessage(messageID, subject)
	msg["applink"] = applink
	msg["body_preview"] = "Preview " + messageID
	msg["internal_date"] = "1700000000000"
	return msg
}
