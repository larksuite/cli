package mail

import (
	"bytes"
	"encoding/base64"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
)

// Every request is intercepted by httpmock; these tests never contact an account.
func TestComposeStdinAttachmentMIMEAcrossPaths(t *testing.T) {
	for _, shortcut := range []common.Shortcut{MailSend, MailDraftCreate, MailReply, MailReplyAll, MailForward} {
		t.Run(shortcut.Command, func(t *testing.T) {
			f, out, _, reg := mailShortcutTestFactory(t)
			if shortcut.Command == "+send" || shortcut.Command == "+draft-create" {
				registerMailboxProfileMock(reg)
			} else {
				stubSourceMessageHTML(reg, "<p>Original</p>")
			}
			stub := &httpmock.Stub{Method: "POST", URL: "/user_mailboxes/me/drafts", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"draft_id": "test-draft"}}}
			reg.Register(stub)
			name := "Example Vendor Campaign THB 521,625.00 Invoice.pdf"
			data := []byte("%PDF-1.4\x00\xff\n")
			args := []string{shortcut.Command, "--body", "body", "--no-signature", "--attach-stdin-name", name, "--attach-stdin-type", "application/pdf"}
			if shortcut.Command == "+send" || shortcut.Command == "+draft-create" {
				args = append(args, "--to", "recipient@example.com", "--subject", "subject")
			} else {
				args = append(args, "--message-id", "msg_w1")
				if shortcut.Command == "+forward" {
					args = append(args, "--to", "recipient@example.com")
				}
			}
			f.IOStreams.In = bytes.NewReader(data)
			if err := runMountedMailShortcut(t, shortcut, args, f, out); err != nil {
				t.Fatal(err)
			}
			raw := mustDecodeRawEMLFromStub(t, stub)
			msg, err := mail.ReadMessage(strings.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			_, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
			if err != nil {
				t.Fatal(err)
			}
			reader := multipart.NewReader(msg.Body, params["boundary"])
			var names []string
			for {
				part, err := reader.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				_, disposition, _ := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
				if disposition["filename"] == "" {
					continue
				}
				names = append(names, disposition["filename"])
				got, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, part))
				if err != nil || !bytes.Equal(got, data) {
					t.Fatalf("payload=%q err=%v", got, err)
				}
			}
			if len(names) != 1 || names[0] != name {
				t.Fatalf("MIME names=%q", names)
			}
		})
	}
}
