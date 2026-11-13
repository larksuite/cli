package mail

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// Exercise all mounted compose paths without contacting an account or sending.
func TestComposeManifestFourPDFsAcrossPaths(t *testing.T) {
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
			names := []string{"Invoice 521,625.00.pdf", "Receipt.pdf", "คำขอ.PDF", "Payment.pdf"}
			payloads := [][]byte{[]byte("%PDF-1.4\x00\xff\n"), []byte("%PDF-2.0\nsecond"), []byte("%PDF-1.7\nthird\x00"), []byte("%PDF-1.5\nfourth")}
			metadata := []map[string]interface{}{}
			var stream bytes.Buffer
			for i, name := range names {
				metadata = append(metadata, map[string]interface{}{"filename": name, "size": len(payloads[i])})
				stream.Write(payloads[i])
			}
			manifest, _ := json.Marshal(metadata)
			args := []string{shortcut.Command, "--body", "body", "--no-signature", "--attach-stdin-manifest", string(manifest)}
			if shortcut.Command == "+send" || shortcut.Command == "+draft-create" {
				args = append(args, "--to", "recipient@example.com", "--subject", "subject")
			} else {
				args = append(args, "--message-id", "msg_w1")
				if shortcut.Command == "+forward" {
					args = append(args, "--to", "recipient@example.com")
				}
			}
			f.IOStreams.In = &stream
			if err := runMountedMailShortcut(t, shortcut, args, f, out); err != nil {
				t.Fatal(err)
			}
			msg, err := mail.ReadMessage(strings.NewReader(mustDecodeRawEMLFromStub(t, stub)))
			if err != nil {
				t.Fatal(err)
			}
			_, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
			if err != nil {
				t.Fatal(err)
			}
			reader := multipart.NewReader(msg.Body, params["boundary"])
			count := 0
			for {
				part, err := reader.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				disposition, dp, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
				if dp["filename"] == "" {
					continue
				}
				decodedName, decodeErr := new(mime.WordDecoder).DecodeHeader(dp["filename"])
				if err != nil || decodeErr != nil || count >= len(names) || decodedName != names[count] || disposition != "attachment" {
					t.Fatalf("part %d disposition=%q params=%v err=%v", count, disposition, dp, err)
				}
				kind, _, err := mime.ParseMediaType(part.Header.Get("Content-Type"))
				if err != nil || kind != "application/octet-stream" {
					t.Fatalf("content type %q err=%v", kind, err)
				}
				got, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, part))
				if err != nil || !bytes.Equal(got, payloads[count]) {
					t.Fatalf("part %d bytes=%q err=%v", count, got, err)
				}
				count++
			}
			if count != 4 {
				t.Fatalf("attachment count=%d", count)
			}
		})
	}
}
