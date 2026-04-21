// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestMailDraftEditOutputsReference(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)

	rawDraft := base64.RawURLEncoding.EncodeToString([]byte(
		"From: me@example.com\r\n" +
			"To: alice@example.com\r\n" +
			"Subject: Original subject\r\n" +
			"MIME-Version: 1.0\r\n" +
			"Content-Type: text/plain; charset=UTF-8\r\n" +
			"\r\n" +
			"hello\r\n",
	))

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/drafts/draft_001",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"draft_id": "draft_001",
				"raw":      rawDraft,
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/user_mailboxes/me/drafts/draft_001",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"draft_id":  "draft_001",
				"reference": "https://www.feishu.cn/mail?draftId=draft_001",
			},
		},
	})

	err := runMountedMailShortcut(t, MailDraftEdit, []string{
		"+draft-edit",
		"--draft-id", "draft_001",
		"--set-subject", "Updated subject",
	}, f, stdout)
	if err != nil {
		t.Fatalf("draft edit failed: %v", err)
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	if data["draft_id"] != "draft_001" {
		t.Fatalf("draft_id = %v", data["draft_id"])
	}
	if data["reference"] != "https://www.feishu.cn/mail?draftId=draft_001" {
		t.Fatalf("reference = %v", data["reference"])
	}
}

func TestMailDraftEditPrettyOutputsReference(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)

	rawDraft := base64.RawURLEncoding.EncodeToString([]byte(
		"From: me@example.com\r\n" +
			"To: alice@example.com\r\n" +
			"Subject: Original subject\r\n" +
			"MIME-Version: 1.0\r\n" +
			"Content-Type: text/plain; charset=UTF-8\r\n" +
			"\r\n" +
			"hello\r\n",
	))

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/drafts/draft_001",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"draft_id": "draft_001",
				"raw":      rawDraft,
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/user_mailboxes/me/drafts/draft_001",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"draft_id":  "draft_001",
				"reference": "https://www.feishu.cn/mail?draftId=draft_001",
			},
		},
	})

	err := runMountedMailShortcut(t, MailDraftEdit, []string{
		"+draft-edit",
		"--draft-id", "draft_001",
		"--set-subject", "Updated subject",
		"--format", "pretty",
	}, f, stdout)
	if err != nil {
		t.Fatalf("draft edit failed: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "Draft updated.") {
		t.Fatalf("expected pretty output header, got: %s", out)
	}
	if !strings.Contains(out, "draft_id: draft_001") {
		t.Fatalf("expected draft_id in pretty output, got: %s", out)
	}
	if !strings.Contains(out, "reference: https://www.feishu.cn/mail?draftId=draft_001") {
		t.Fatalf("expected reference in pretty output, got: %s", out)
	}
}
