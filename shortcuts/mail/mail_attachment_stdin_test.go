package mail

import (
	"bytes"
	"context"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/mail/emlbuilder"
	"github.com/spf13/cobra"
	"strings"
	"testing"
)

func stdinAttachmentRuntime(name, kind, data string) *common.RuntimeContext {
	cmd := &cobra.Command{}
	cmd.Flags().String("attach-stdin-name", name, "")
	cmd.Flags().String("attach-stdin-type", kind, "")
	return &common.RuntimeContext{Cmd: cmd, Factory: &cmdutil.Factory{IOStreams: &cmdutil.IOStreams{In: strings.NewReader(data)}}}
}
func TestStdinAttachmentPDFBytesAndName(t *testing.T) {
	r := stdinAttachmentRuntime("Invoice 521,625.00.pdf", "application/pdf", "%PDF-1.4\x00\xff\n")
	a, err := readStdinAttachment(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 1 || a[0].Name != "Invoice 521,625.00.pdf" || !bytes.Equal(a[0].Data, []byte("%PDF-1.4\x00\xff\n")) {
		t.Fatalf("attachment=%+v", a)
	}
	b := a.addTo(emlbuilder.New().From("", "sender@example.com").To("", "recipient@example.com").TextBody([]byte("body")))
	raw, err := b.Build()
	if err != nil || !bytes.Contains(raw, []byte("application/octet-stream")) {
		t.Fatalf("EML err=%v", err)
	}
	if _, err := readStdinAttachment(r); err == nil {
		t.Fatal("cannot consume stdin twice")
	}
}
func TestStdinAttachmentRejectsUnsafeMetadata(t *testing.T) {
	for _, name := range []string{"../a.pdf", "/root/a.pdf", `C:\secret.pdf`, "a.pdf\r\nBcc: bad", "a.exe", ".", "a\x00.pdf"} {
		if err := validateStdinAttachmentFlags(stdinAttachmentRuntime(name, "application/pdf", "")); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
	for _, kind := range []string{"", "text/plain", "application/pdf\r\nX: bad"} {
		if err := validateStdinAttachmentFlags(stdinAttachmentRuntime("a.pdf", kind, "")); err == nil {
			t.Fatalf("accepted type %q", kind)
		}
	}
	if err := validateStdinAttachmentFlags(stdinAttachmentRuntime("", "application/pdf", "")); err == nil {
		t.Fatal("orphan type")
	}
}
func TestStdinAttachmentRejectsInvalidOrLargeBytes(t *testing.T) {
	for _, data := range []string{"", "not a PDF", "%PDF-" + strings.Repeat("x", int(maxStdinAttachmentBytes))} {
		if _, err := readStdinAttachment(stdinAttachmentRuntime("a.pdf", "application/pdf", data)); err == nil {
			t.Fatal("accepted empty, non-PDF, or oversized bytes")
		}
	}
}
func TestComposeStdinAttachmentMetadataValidatedAcrossPaths(t *testing.T) {
	for _, s := range []common.Shortcut{MailSend, MailDraftCreate, MailReply, MailReplyAll, MailForward} {
		t.Run(s.Command, func(t *testing.T) {
			r := stdinAttachmentRuntime("../a.pdf", "application/pdf", "")
			r.Cmd.Flags().StringArray("to", []string{"recipient@example.com"}, "")
			for k, v := range map[string]string{"body": "body", "subject": "subject", "message-id": "123"} {
				r.Cmd.Flags().String(k, v, "")
			}
			if err := s.Validate(context.Background(), r); err == nil {
				t.Fatal("unsafe stdin attachment metadata accepted")
			}
		})
	}
}
