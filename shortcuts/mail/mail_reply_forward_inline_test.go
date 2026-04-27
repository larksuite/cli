// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/base64"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

// stubSourceMessageWithInlineImages registers HTTP stubs for a source message.
func stubSourceMessageWithInlineImages(reg *httpmock.Registry, bodyHTML string, allImages []map[string]interface{}) {
	// Profile
	reg.Register(&httpmock.Stub{
		URL: "/user_mailboxes/me/profile",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"primary_email_address": "me@example.com",
			},
		},
	})

	// Message get
	atts := allImages
	if atts == nil {
		atts = []map[string]interface{}{}
	}
	reg.Register(&httpmock.Stub{
		URL: "/user_mailboxes/me/messages/msg_001",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"message": map[string]interface{}{
					"message_id":      "msg_001",
					"thread_id":       "thread_001",
					"smtp_message_id": "<msg_001@example.com>",
					"subject":         "Original Subject",
					"head_from":       map[string]interface{}{"mail_address": "sender@example.com", "name": "Sender"},
					"to":              []map[string]interface{}{{"mail_address": "me@example.com", "name": "Me"}},
					"cc":              []interface{}{},
					"bcc":             []interface{}{},
					"body_html":       base64.URLEncoding.EncodeToString([]byte(bodyHTML)),
					"body_plain_text": base64.URLEncoding.EncodeToString([]byte("plain")),
					"internal_date":   "1704067200000",
					"attachments":     atts,
				},
			},
		},
	})

	// Download URLs
	if len(allImages) > 0 {
		downloadURLs := make([]map[string]interface{}, 0, len(allImages))
		for _, img := range allImages {
			id, _ := img["id"].(string)
			downloadURLs = append(downloadURLs, map[string]interface{}{
				"attachment_id": id,
				"download_url":  "https://storage.example.com/" + id,
			})
		}
		reg.Register(&httpmock.Stub{
			URL: "/attachments/download_url",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"download_urls": downloadURLs,
					"failed_ids":    []interface{}{},
				},
			},
		})
	}

	// Image downloads
	pngBytes := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	for _, img := range allImages {
		id, _ := img["id"].(string)
		reg.Register(&httpmock.Stub{
			URL:     "https://storage.example.com/" + id,
			RawBody: pngBytes,
		})
	}

	// Draft create
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/drafts",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"draft_id":  "draft_001",
				"reference": "https://www.feishu.cn/mail?draftId=draft_001",
			},
		},
	})
}

// ---------------------------------------------------------------------------
// +reply with source inline images
// ---------------------------------------------------------------------------

func TestReply_SourceInlineImagesPreserved(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)

	stubSourceMessageWithInlineImages(reg,
		`<p>Hello <img src="cid:banner_001" /></p>`,
		[]map[string]interface{}{
			{"id": "img_001", "filename": "banner.png", "is_inline": true, "cid": "banner_001", "content_type": "image/png"},
		},
	)

	err := runMountedMailShortcut(t, MailReply, []string{
		"+reply", "--message-id", "msg_001", "--body", "<p>Thanks!</p>",
	}, f, stdout)
	if err != nil {
		t.Fatalf("reply failed: %v", err)
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	if data["draft_id"] == nil || data["draft_id"] == "" {
		t.Fatal("expected draft_id in output")
	}
	if data["reference"] != "https://www.feishu.cn/mail?draftId=draft_001" {
		t.Fatalf("reference = %v", data["reference"])
	}
}

func TestReply_SourceOrphanCIDNotBlocked(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)

	// Source has TWO inline images, but body HTML only references one.
	// The unreferenced image should NOT be downloaded or cause an error.
	stubSourceMessageWithInlineImages(reg,
		`<p>Hello <img src="cid:used_001" /></p>`,
		[]map[string]interface{}{
			{"id": "img_001", "filename": "used.png", "is_inline": true, "cid": "used_001", "content_type": "image/png"},
			{"id": "img_002", "filename": "unused.png", "is_inline": true, "cid": "unused_002", "content_type": "image/png"},
		},
	)

	err := runMountedMailShortcut(t, MailReply, []string{
		"+reply", "--message-id", "msg_001", "--body", "<p>Reply</p>",
	}, f, stdout)
	if err != nil {
		t.Fatalf("reply should succeed even with unreferenced source CID, got: %v", err)
	}
}

func TestReply_WithAutoResolveLocalImage(t *testing.T) {
	chdirTemp(t)
	os.WriteFile("local.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644)

	f, stdout, _, reg := mailShortcutTestFactory(t)

	stubSourceMessageWithInlineImages(reg,
		`<p>Hello</p>`,
		nil,
	)

	err := runMountedMailShortcut(t, MailReply, []string{
		"+reply", "--message-id", "msg_001",
		"--body", `<p>See image: <img src="./local.png" /></p>`,
	}, f, stdout)
	if err != nil {
		t.Fatalf("reply with auto-resolved local image failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// +reply-all with source inline images
// ---------------------------------------------------------------------------

func TestReplyAll_SourceOrphanCIDNotBlocked(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)

	stubSourceMessageWithInlineImages(reg,
		`<p>Hello <img src="cid:used_001" /></p>`,
		[]map[string]interface{}{
			{"id": "img_001", "filename": "used.png", "is_inline": true, "cid": "used_001", "content_type": "image/png"},
			{"id": "img_002", "filename": "orphan.png", "is_inline": true, "cid": "orphan_002", "content_type": "image/png"},
		},
	)

	// reply-all also needs self-exclusion profile lookup
	reg.Register(&httpmock.Stub{
		URL: "/user_mailboxes/me/profile",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"primary_email_address": "me@example.com",
			},
		},
	})

	err := runMountedMailShortcut(t, MailReplyAll, []string{
		"+reply-all", "--message-id", "msg_001", "--body", "<p>Reply all</p>",
	}, f, stdout)
	if err != nil {
		t.Fatalf("reply-all should succeed with unreferenced source CID, got: %v", err)
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	if data["reference"] != "https://www.feishu.cn/mail?draftId=draft_001" {
		t.Fatalf("reference = %v", data["reference"])
	}
}

// ---------------------------------------------------------------------------
// +forward with source inline images
// ---------------------------------------------------------------------------

func TestForward_SourceOrphanCIDNotBlocked(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)

	stubSourceMessageWithInlineImages(reg,
		`<p>Hello <img src="cid:used_001" /></p>`,
		[]map[string]interface{}{
			{"id": "img_001", "filename": "used.png", "is_inline": true, "cid": "used_001", "content_type": "image/png"},
			{"id": "img_002", "filename": "orphan.png", "is_inline": true, "cid": "orphan_002", "content_type": "image/png"},
		},
	)

	err := runMountedMailShortcut(t, MailForward, []string{
		"+forward", "--message-id", "msg_001",
		"--to", "alice@example.com",
		"--body", "<p>FYI</p>",
	}, f, stdout)
	if err != nil {
		t.Fatalf("forward should succeed with unreferenced source CID, got: %v", err)
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	if data["reference"] != "https://www.feishu.cn/mail?draftId=draft_001" {
		t.Fatalf("reference = %v", data["reference"])
	}
}

func TestForward_WithAutoResolveLocalImage(t *testing.T) {
	chdirTemp(t)
	os.WriteFile("chart.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644)

	f, stdout, _, reg := mailShortcutTestFactory(t)

	stubSourceMessageWithInlineImages(reg,
		`<p>Original content</p>`,
		nil,
	)

	err := runMountedMailShortcut(t, MailForward, []string{
		"+forward", "--message-id", "msg_001",
		"--to", "alice@example.com",
		"--body", `<p>See chart: <img src="./chart.png" /></p>`,
	}, f, stdout)
	if err != nil {
		t.Fatalf("forward with auto-resolved local image failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// +reply body auto-resolve does NOT scan quoted content
// ---------------------------------------------------------------------------

func TestReply_QuotedContentNotAutoResolved(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)

	// Source message body has a relative <img src> — this should NOT be
	// auto-resolved because it's in the quoted portion, not the user body.
	stubSourceMessageWithInlineImages(reg,
		`<p>See <img src="./should-not-resolve.png" /></p>`,
		nil,
	)

	err := runMountedMailShortcut(t, MailReply, []string{
		"+reply", "--message-id", "msg_001",
		"--body", "<p>Got it</p>",
	}, f, stdout)
	// Should succeed — the ./should-not-resolve.png in quoted content is
	// NOT auto-resolved (file doesn't exist, would fail if scanned).
	if err != nil {
		if strings.Contains(err.Error(), "should-not-resolve") {
			t.Fatalf("auto-resolve incorrectly scanned quoted content: %v", err)
		}
		t.Fatalf("unexpected error: %v", err)
	}
}
