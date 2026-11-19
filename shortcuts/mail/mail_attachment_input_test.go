package mail

import (
	"context"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
	"os"
	"strings"
	"testing"
)

func TestComposeAttachmentPreservesCommaFilename(t *testing.T) {
	for _, shortcut := range []common.Shortcut{MailSend, MailDraftCreate, MailReply, MailReplyAll, MailForward} {
		t.Run(shortcut.Command, func(t *testing.T) {
			chdirTemp(t)
			names := []string{"Example Vendor Campaign THB 521,625.00 Invoice.pdf", "second, report.pdf"}
			for _, name := range names {
				if err := os.WriteFile(name, []byte("%PDF-1.4\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			cmd := &cobra.Command{}
			cmd.Flags().StringArray("attach", names, "")
			cmd.Flags().StringArray("to", []string{"recipient@example.com"}, "")
			for k, v := range map[string]string{"subject": "subject", "body": "body", "message-id": "123"} {
				cmd.Flags().String(k, v, "")
			}
			runtime := &common.RuntimeContext{Cmd: cmd, Factory: &cmdutil.Factory{FileIOProvider: attachmentTestProvider{}}}
			if err := shortcut.Validate(context.Background(), runtime); err != nil {
				t.Fatalf("literal repeated --attach filenames must validate: %v", err)
			}
		})
	}
}

func TestComposeAttachmentDoesNotInterpretCommaAsList(t *testing.T) {
	chdirTemp(t)
	for _, name := range []string{"a.pdf", "b.pdf"} {
		if err := os.WriteFile(name, []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cmd := &cobra.Command{}
	cmd.Flags().StringArray("attach", []string{"a.pdf,b.pdf"}, "")
	cmd.Flags().String("body", "body", "")
	cmd.Flags().String("subject", "subject", "")
	cmd.Flags().StringArray("to", []string{"recipient@example.com"}, "")
	runtime := &common.RuntimeContext{Cmd: cmd, Factory: &cmdutil.Factory{FileIOProvider: attachmentTestProvider{}}}
	err := MailSend.Validate(context.Background(), runtime)
	if err == nil {
		t.Fatal("must not silently attach two different files for one literal path")
	}
	if !strings.Contains(err.Error(), "a.pdf,b.pdf") {
		t.Fatalf("error should identify exact path: %v", err)
	}
}

// An injected, test-only storage backend isolates parsing from host /root policy.
type attachmentTestProvider struct{}

func (attachmentTestProvider) Name() string                                { return "attachment-test" }
func (attachmentTestProvider) ResolveFileIO(context.Context) fileio.FileIO { return attachmentTestIO{} }

type attachmentTestIO struct{ fileio.FileIO }

func (attachmentTestIO) Stat(name string) (fileio.FileInfo, error) { return os.Stat(name) }

func TestAttachmentProtectedPathsRemainDenied(t *testing.T) {
	for _, path := range []string{"/root/never-open.pdf", "../../../root/never-open.pdf", "/proc/self/environ", "/etc/shadow"} {
		r := &common.RuntimeContext{}
		if _, err := statAttachmentFiles(r.FileIO(), []string{path}); err == nil {
			t.Fatalf("protected path accepted: %q", path)
		}
	}
}
