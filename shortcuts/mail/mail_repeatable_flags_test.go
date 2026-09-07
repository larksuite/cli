// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestNormalizeRepeatedRecipientFlagsPreservesDisplayNameCommas(t *testing.T) {
	got := normalizeRecipientFlagValues([]string{
		`a@x,b@y`,
		`Alice <alice@example.com>, Bob <bob@example.com>`,
		`"ACME, Inc." <billing@example.com>`,
		`alice@example.com,bob@example.com`,
	})

	if !strings.Contains(got, `"ACME, Inc." <billing@example.com>`) {
		t.Fatalf("quoted display-name comma should stay one recipient, got %q", got)
	}
	boxes := ParseMailboxList(got)
	if len(boxes) != 7 {
		t.Fatalf("recipient count = %d, want 7; normalized=%q", len(boxes), got)
	}
	want := []Mailbox{
		{Email: "a@x"},
		{Email: "b@y"},
		{Name: "Alice", Email: "alice@example.com"},
		{Name: "Bob", Email: "bob@example.com"},
		{Name: "ACME, Inc.", Email: "billing@example.com"},
		{Email: "alice@example.com"},
		{Email: "bob@example.com"},
	}
	if !reflect.DeepEqual(boxes, want) {
		t.Fatalf("recipients = %#v, want %#v; normalized=%q", boxes, want, got)
	}
}

func TestNormalizeRecipientFlagsPreservesUnicodeDisplayNameSemantics(t *testing.T) {
	got := normalizeRecipientFlagValues([]string{`测试用户 <unicode@example.com>`})
	if strings.Contains(got, "=?UTF-8?") {
		t.Fatalf("normalization must not RFC2047-encode display names, got %q", got)
	}
	if got != `测试用户 <unicode@example.com>` {
		t.Fatalf("normalized recipient = %q, want raw display-name semantics", got)
	}
}

func TestMailboxLongUnicodeDisplayNameSplitsEncodedWords(t *testing.T) {
	name := strings.Repeat("测试用户", 20)
	got := (Mailbox{Name: name, Email: "long@example.com"}).String()
	if strings.Contains(got, name) {
		t.Fatalf("display name should be encoded, got %q", got)
	}
	encodedWords := 0
	for _, field := range strings.Fields(got) {
		if !strings.HasPrefix(field, "=?UTF-8?b?") && !strings.HasPrefix(field, "=?UTF-8?B?") {
			continue
		}
		encodedWords++
		if len(field) > 75 {
			t.Fatalf("encoded word length = %d, want <= 75: %q", len(field), field)
		}
	}
	if encodedWords < 2 {
		t.Fatalf("expected long display name to be split into multiple encoded words, got %q", got)
	}
}

func TestNormalizeRepeatedCommaFlagsPreservesOrder(t *testing.T) {
	got := normalizeCommaListFlagValues([]string{
		"./a.pdf",
		" ./b.pdf,./c.pdf ",
		"",
		"./d.pdf",
	})
	want := []string{"./a.pdf", "./b.pdf", "./c.pdf", "./d.pdf"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("paths = %#v, want %#v", got, want)
	}
}

func TestNormalizeRepeatedInlineFlagsAppendsObjectAndArrayValues(t *testing.T) {
	raw, err := normalizeInlineFlagValues([]string{
		`{"cid":"hero","file_path":"./hero.png"}`,
		`[{"cid":"logo","file_path":"./logo.png"},{"cid":"qr","file_path":"./qr.png"}]`,
	})
	if err != nil {
		t.Fatalf("normalizeInlineFlagValues() error = %v", err)
	}
	specs, err := parseInlineSpecs(raw)
	if err != nil {
		t.Fatalf("parseInlineSpecs(%q) error = %v", raw, err)
	}
	want := []InlineSpec{
		{CID: "hero", FilePath: "./hero.png"},
		{CID: "logo", FilePath: "./logo.png"},
		{CID: "qr", FilePath: "./qr.png"},
	}
	if !reflect.DeepEqual(specs, want) {
		t.Fatalf("inline specs = %#v, want %#v", specs, want)
	}
}

func TestNormalizeRepeatedInlineFlagsAllowsDuplicateCIDForCompatibility(t *testing.T) {
	raw, err := normalizeInlineFlagValues([]string{
		`{"cid":"logo","file_path":"./a.png"}`,
		`[{"cid":"<logo>","file_path":"./b.png"}]`,
	})
	if err != nil {
		t.Fatalf("expected duplicate cid compatibility, got %v", err)
	}
	specs, err := parseInlineSpecs(raw)
	if err != nil {
		t.Fatalf("parseInlineSpecs(%q) error = %v", raw, err)
	}
	if len(specs) != 2 {
		t.Fatalf("inline specs = %#v, want 2 entries", specs)
	}
	want := []InlineSpec{
		{CID: "logo", FilePath: "./a.png"},
		{CID: "logo", FilePath: "./b.png"},
	}
	if !reflect.DeepEqual(specs, want) {
		t.Fatalf("inline specs = %#v, want %#v", specs, want)
	}
}

func TestParseInlineSpecsNullIsEmptyForCompatibility(t *testing.T) {
	specs, err := parseInlineSpecs(`null`)
	if err != nil {
		t.Fatalf("parseInlineSpecs(null) error = %v", err)
	}
	if len(specs) != 0 {
		t.Fatalf("parseInlineSpecs(null) = %#v, want empty", specs)
	}
}

func TestParseInlineSpecsRejectsNonObjectArrayJSON(t *testing.T) {
	for _, raw := range []string{`"value"`, `42`, `true`} {
		t.Run(raw, func(t *testing.T) {
			_, err := parseInlineSpecs(raw)
			if err == nil || !strings.Contains(err.Error(), "JSON object or array") {
				t.Fatalf("expected JSON object/array error, got %v", err)
			}
			assertInlineValidationError(t, err)
		})
	}
}

func assertInlineValidationError(t *testing.T, err error) {
	t.Helper()
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error type = %T, want *errs.ValidationError", err)
	}
	if ve.Category != errs.CategoryValidation {
		t.Fatalf("validation category = %q, want %q", ve.Category, errs.CategoryValidation)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("validation subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
	}
	if ve.Param != "--inline" {
		t.Fatalf("validation param = %q, want --inline", ve.Param)
	}
}

func TestMailRepeatableFlagTypes(t *testing.T) {
	f, _, _, _ := mailShortcutTestFactory(t)
	for _, tc := range []struct {
		name         string
		shortcut     common.Shortcut
		stringArrays []string
		strings      []string
	}{
		{
			name:         "send",
			shortcut:     MailSend,
			stringArrays: []string{"to", "cc", "bcc", "attach", "inline"},
		},
		{
			name:         "draft-create",
			shortcut:     MailDraftCreate,
			stringArrays: []string{"to", "cc", "bcc", "attach", "inline"},
		},
		{
			name:         "reply",
			shortcut:     MailReply,
			stringArrays: []string{"to", "cc", "bcc", "attach", "inline"},
		},
		{
			name:         "reply-all",
			shortcut:     MailReplyAll,
			stringArrays: []string{"to", "cc", "bcc", "remove", "attach", "inline"},
		},
		{
			name:         "forward",
			shortcut:     MailForward,
			stringArrays: []string{"to", "cc", "bcc", "attach", "inline"},
		},
		{
			name:         "template-create",
			shortcut:     MailTemplateCreate,
			stringArrays: []string{"to", "cc", "bcc", "attach", "inline"},
		},
		{
			name:         "template-update",
			shortcut:     MailTemplateUpdate,
			stringArrays: []string{"attach", "inline"},
			strings:      []string{"set-to", "set-cc", "set-bcc"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := mountedShortcutCommand(t, f, tc.shortcut)
			for _, name := range tc.stringArrays {
				assertFlagType(t, cmd, name, "stringArray")
			}
			for _, name := range tc.strings {
				assertFlagType(t, cmd, name, "string")
			}
		})
	}
}

func mountedShortcutCommand(t *testing.T, f *cmdutil.Factory, shortcut common.Shortcut) *cobra.Command {
	t.Helper()
	parent := &cobra.Command{Use: "test"}
	shortcut.Mount(parent, f)
	for _, cmd := range parent.Commands() {
		if cmd.Name() == shortcut.Command {
			return cmd
		}
	}
	t.Fatalf("mounted command %q not found", shortcut.Command)
	return nil
}

func assertFlagType(t *testing.T, cmd *cobra.Command, name, want string) {
	t.Helper()
	flag := cmd.Flags().Lookup(name)
	if flag == nil {
		t.Fatalf("%s: flag %q not found", cmd.Name(), name)
	}
	if got := flag.Value.Type(); got != want {
		t.Fatalf("%s --%s type = %q, want %q", cmd.Name(), name, got, want)
	}
}
