package mail

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
)

func manifestRuntime(manifest, data string) *common.RuntimeContext {
	r := stdinAttachmentRuntime("", "", data)
	r.Cmd.Flags().String("attach-stdin-manifest", manifest, "")
	return r
}
func TestComposeManifestMetadataValidatedAcrossPaths(t *testing.T) {
	for _, shortcut := range []common.Shortcut{MailSend, MailDraftCreate, MailReply, MailReplyAll, MailForward} {
		t.Run(shortcut.Command, func(t *testing.T) {
			r := manifestRuntime(`[{"filename":"../a.pdf","size":5}]`, "")
			r.Cmd.Flags().StringArray("to", []string{"recipient@example.com"}, "")
			for key, value := range map[string]string{"body": "body", "subject": "subject", "message-id": "123"} {
				r.Cmd.Flags().String(key, value, "")
			}
			if err := shortcut.Validate(context.Background(), r); err == nil {
				t.Fatal("unsafe manifest accepted")
			}
		})
	}
}

func TestManifestRejectsInvalidMetadata(t *testing.T) {
	cases := []string{
		"[]", "null", `{}`, `[{"filename":"a.pdf"}]`, `[{"filename":"a.pdf","size":null}]`,
		`[{"filename":"a.pdf","size":-1}]`, `[{"filename":"a.pdf","size":4}]`, `[{"filename":"a.pdf","size":5.5}]`,
		`[{"filename":"a.pdf","size":"5"}]`, `[{"filename":"a.pdf","size":9223372036854775808}]`,
		`[{"filename":"a.pdf","size":10485761}]`, `[{"filename":"a.pdf","size":10485760},{"filename":"b.pdf","size":5}]`,
		`[{"filename":"a.pdf","size":5,"type":"text/plain"}]`, `[{"filename":"a.pdf","size":5}] null`,
		`[{"filename":"a.pdf","size":5,"size":6}]`, `[{"filename":"a.pdf","filename":"b.pdf","size":5}]`,
		`[{"Filename":"a.pdf","size":5}]`, `[{"filename":"a.pdf","Size":5}]`,
		`[{"filename":"a.pdf","size":5}]` + strings.Repeat(" ", 256*1024),
	}
	for _, name := range []string{"../a.pdf", `C:\a.pdf`, "/root/a.pdf", "a.pdf\r\nBcc: bad", "a.txt", " a.pdf", "a\x00.pdf", ".", "a\x7f.pdf"} {
		b, _ := json.Marshal([]map[string]interface{}{{"filename": name, "size": 5}})
		cases = append(cases, string(b))
	}
	cases = append(cases, `[{"filename":"a`+string([]byte{255})+`.pdf","size":5}]`)
	entries := make([]map[string]interface{}, MaxAttachmentCount+1)
	for i := range entries {
		entries[i] = map[string]interface{}{"filename": "a.pdf", "size": 5}
	}
	b, _ := json.Marshal(entries)
	cases = append(cases, string(b))
	for i, manifest := range cases {
		t.Run(string(rune('A'+i)), func(t *testing.T) {
			if err := validateStdinAttachmentFlags(manifestRuntime(manifest, "")); err == nil {
				t.Fatalf("accepted manifest %q", manifest)
			}
		})
	}
}
func TestManifestRejectsInvalidFraming(t *testing.T) {
	manifest := `[{"filename":"a.pdf","size":6},{"filename":"b.pdf","size":6}]`
	for _, data := range []string{"", "%PDF-a", "%PDF-a%PDF-", "%PDF-a%PDF-bX", "%PDF-a!PDF-b", "!PDF-a%PDF-b"} {
		if _, err := readStdinAttachment(manifestRuntime(manifest, data)); err == nil {
			t.Fatalf("accepted %q", data)
		}
	}
}
func TestManifestExclusiveWithSingleFileAndStdinConsumers(t *testing.T) {
	manifest := `[{"filename":"a.pdf","size":6}]`
	for _, flag := range []string{"attach-stdin-name", "attach-stdin-type"} {
		r := manifestRuntime(manifest, "%PDF-a")
		r.Cmd.Flags().Set(flag, "a.pdf")
		if err := validateStdinAttachmentFlags(r); err == nil {
			t.Fatalf("accepted %s", flag)
		}
	}
	r := manifestRuntime(manifest, "%PDF-a")
	if _, err := r.ReadStdinBytesOnce("--body", 100); err != nil {
		t.Fatal(err)
	}
	if _, err := readStdinAttachment(r); err == nil {
		t.Fatal("stdin reuse allowed")
	}
	r = manifestRuntime(manifest, "%PDF-a")
	if _, err := readStdinAttachment(r); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ReadStdinBytesOnce("--body", 100); err == nil {
		t.Fatal("stdin reuse allowed")
	}
}
func TestManifestBoundariesAndAccounting(t *testing.T) {
	entries := make([]map[string]interface{}, MaxAttachmentCount)
	for i := range entries {
		entries[i] = map[string]interface{}{"filename": "a.pdf", "size": 5}
	}
	b, _ := json.Marshal(entries)
	a, err := readStdinAttachment(manifestRuntime(string(b), strings.Repeat("%PDF-", MaxAttachmentCount)))
	if err != nil || a.count() != MaxAttachmentCount || a.emlBytes() != MaxAttachmentCount*(5*137/100+1024) {
		t.Fatalf("count=%d bytes=%d err=%v", a.count(), a.emlBytes(), err)
	}
	data := "%PDF-" + strings.Repeat("x", int(maxStdinAttachmentBytes)-5)
	a, err = readStdinAttachment(manifestRuntime(`[{"filename":"a.pdf","size":10485760}]`, data))
	if err != nil || a.count() != 1 || len(a[0].Data) != len(data) {
		t.Fatalf("max bytes err=%v", err)
	}
	if _, err := readStdinAttachment(manifestRuntime(`[{"filename":"a.pdf","size":10485760}]`, data+"x")); err == nil {
		t.Fatal("accepted more than 10 MiB")
	}
}
