// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/base64"
	"io"
	netmail "net/mail"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestMailDraftCreate_LongASCIIBodyPreservedInRequest(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	chdirTemp(t)
	registerMailboxProfileMock(reg)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/drafts",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"draft_id": "d_test"},
		},
	}
	reg.Register(stub)
	// Put the old 998-byte split inside an HTML attribute as well as text.
	body := "<div>" + strings.Repeat("word ", 195) +
		`<span style="font-family:Arial;margin-top:0">` +
		strings.Repeat("word ", 300) + "</span></div>"
	if err := runMountedMailShortcut(t, MailDraftCreate, []string{
		"+draft-create", "--to", "alice@example.com", "--subject", "Long body",
		"--body", body, "--no-signature",
	}, f, stdout); err != nil {
		t.Fatal(err)
	}
	if decodedSinglePartBodyForTest(t, mustDecodeRawEMLFromStub(t, stub)) != body {
		t.Fatal("drafts.create request changed the HTML body")
	}
}

func decodedSinglePartBodyForTest(t *testing.T, eml string) string {
	t.Helper()
	message, err := netmail.ReadMessage(strings.NewReader(eml))
	if err != nil {
		t.Fatal(err)
	}
	reader := message.Body
	if message.Header.Get("Content-Transfer-Encoding") == "base64" {
		reader = base64.NewDecoder(base64.StdEncoding, reader)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(decoded)
}
