// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/vfs/localfileio"
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

func TestValidateRecipientFlagOccurrences(t *testing.T) {
	for _, flagName := range []string{"to", "cc", "bcc"} {
		t.Run(flagName+" rejects malformed later occurrence", func(t *testing.T) {
			secret := "TOP-SECRET-RECIPIENT"
			err := validateRecipientFlagOccurrences(flagName, []string{
				`"Doe, John" <john@example.com>,jane@example.com`,
				`"Broken, Recipient" <` + secret,
			})
			if err == nil {
				t.Fatal("expected malformed second occurrence to fail")
			}
			for _, want := range []string{"--" + flagName, "occurrence 2", "value redacted"} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error = %q, want %q", err, want)
				}
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("error must not echo recipient value, got %q", err)
			}
			var ve *errs.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("error type = %T, want *errs.ValidationError", err)
			}
			if ve.Param != "--"+flagName {
				t.Fatalf("validation param = %q, want --%s", ve.Param, flagName)
			}
		})

		t.Run(flagName+" preserves compatible address lists", func(t *testing.T) {
			err := validateRecipientFlagOccurrences(flagName, []string{
				`"Doe, John" <john@example.com>,jane@example.com`,
				`测试用户 <unicode@example.com>`,
				``,
			})
			if err != nil {
				t.Fatalf("compatible recipient list rejected: %v", err)
			}
		})
	}
}

func TestMailRepeatableRecipientValidationPrecedesSideEffects(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	shortcuts := []struct {
		name     string
		shortcut common.Shortcut
		extra    []string
	}{
		{name: "send", shortcut: MailSend, extra: []string{"--subject", "subject", "--body", "<p>body</p>"}},
		{name: "draft-create", shortcut: MailDraftCreate, extra: []string{"--subject", "subject", "--body", "<p>body</p>"}},
		{name: "reply", shortcut: MailReply, extra: []string{"--message-id", "message", "--body", "<p>body</p>"}},
		{name: "reply-all", shortcut: MailReplyAll, extra: []string{"--message-id", "message", "--body", "<p>body</p>"}},
		{name: "forward", shortcut: MailForward, extra: []string{"--message-id", "message", "--body", "<p>body</p>"}},
		{name: "template-create", shortcut: MailTemplateCreate, extra: []string{"--name", "name", "--template-content", "<p>body</p>"}},
	}

	for _, shortcut := range shortcuts {
		for _, flagName := range []string{"to", "cc", "bcc"} {
			t.Run(shortcut.name+"/"+flagName, func(t *testing.T) {
				secret := "TOP-SECRET-RECIPIENT"
				args := append([]string{shortcut.shortcut.Command}, shortcut.extra...)
				if flagName != "to" && (shortcut.name == "send" || shortcut.name == "forward") {
					args = append(args, "--to", "required@example.com")
				}
				args = append(args,
					"--"+flagName, `"Valid, Recipient" <valid@example.com>`,
					"--"+flagName, `"Broken, Recipient" <`+secret,
				)
				err := runMountedMailShortcut(t, shortcut.shortcut, args, f, stdout)
				if err == nil {
					t.Fatal("expected recipient validation failure before command side effects")
				}
				for _, want := range []string{"--" + flagName, "occurrence 2", "value redacted"} {
					if !strings.Contains(err.Error(), want) {
						t.Fatalf("error = %q, want %q", err, want)
					}
				}
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("error must not echo recipient value, got %q", err)
				}
			})
		}
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

func TestValidateAttachmentFlagValuesReportsSafeOccurrence(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("existing.txt", []byte("ok"), 0o600); err != nil {
		t.Fatalf("write existing attachment: %v", err)
	}

	secret := "TOP-SECRET-ATTACHMENT-NAME.txt"
	err := validateAttachmentFlagValues(&localfileio.LocalFileIO{}, []string{
		"existing.txt",
		"missing.txt," + secret,
	})
	if err == nil {
		t.Fatal("expected invalid second occurrence to fail")
	}
	for _, want := range []string{"--attach", "occurrence 2", "2 path(s)", "values redacted"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "missing.txt") || strings.Contains(err.Error(), secret) {
		t.Fatalf("error must not echo attachment values, got %q", err)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error type = %T, want *errs.ValidationError", err)
	}
	if ve.Param != "--attach" {
		t.Fatalf("validation param = %q, want --attach", ve.Param)
	}
}

func TestMailRepeatableAttachmentValidationPrecedesSideEffects(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("existing.txt", []byte("ok"), 0o600); err != nil {
		t.Fatalf("write existing attachment: %v", err)
	}

	f, stdout, _, _ := mailShortcutTestFactory(t)
	secret := "TOP-SECRET-OCCURRENCE.txt"
	for _, tc := range []struct {
		name     string
		shortcut common.Shortcut
		extra    []string
	}{
		{name: "send", shortcut: MailSend, extra: []string{"--to", "a@example.com", "--subject", "subject", "--body", "<p>body</p>"}},
		{name: "draft-create", shortcut: MailDraftCreate, extra: []string{"--subject", "subject", "--body", "<p>body</p>"}},
		{name: "reply", shortcut: MailReply, extra: []string{"--message-id", "message", "--body", "<p>body</p>"}},
		{name: "reply-all", shortcut: MailReplyAll, extra: []string{"--message-id", "message", "--body", "<p>body</p>"}},
		{name: "forward", shortcut: MailForward, extra: []string{"--message-id", "message", "--to", "a@example.com", "--body", "<p>body</p>"}},
		{name: "template-create", shortcut: MailTemplateCreate, extra: []string{"--name", "name", "--template-content", "<p>body</p>"}},
		{name: "template-update", shortcut: MailTemplateUpdate, extra: []string{"--template-id", "1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{tc.shortcut.Command}, tc.extra...)
			args = append(args,
				"--attach", "existing.txt",
				"--attach", secret,
			)
			err := runMountedMailShortcut(t, tc.shortcut, args, f, stdout)
			if err == nil {
				t.Fatal("expected attachment preflight failure")
			}
			for _, want := range []string{"--attach", "occurrence 2", "1 path(s)", "values redacted"} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error = %q, want %q", err, want)
				}
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("error must not echo attachment value, got %q", err)
			}
		})
	}
}

func TestRepeatableFlagNormalizationMatchesLegacySingleOccurrence(t *testing.T) {
	if got, want := normalizeRecipientFlagValues([]string{"a@x", "b@y"}), normalizeRecipientFlagValues([]string{"a@x,b@y"}); got != want {
		t.Fatalf("repeated recipients = %q, legacy single occurrence = %q", got, want)
	}
	if got, want := normalizeCommaFlagValues([]string{"./a.pdf", "./b.pdf"}), normalizeCommaFlagValues([]string{"./a.pdf,./b.pdf"}); got != want {
		t.Fatalf("repeated attachments = %q, legacy single occurrence = %q", got, want)
	}
	repeated, err := normalizeInlineFlagValues([]string{
		`{"cid":"hero","file_path":"./hero.png"}`,
		`{"cid":"logo","file_path":"./logo.png"}`,
	})
	if err != nil {
		t.Fatalf("normalize repeated inline values: %v", err)
	}
	legacy, err := normalizeInlineFlagValues([]string{
		`[{"cid":"hero","file_path":"./hero.png"},{"cid":"logo","file_path":"./logo.png"}]`,
	})
	if err != nil {
		t.Fatalf("normalize legacy inline array: %v", err)
	}
	if repeated != legacy {
		t.Fatalf("repeated inline = %q, legacy array = %q", repeated, legacy)
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

func TestParseInlineSpecsRejectsNull(t *testing.T) {
	_, err := parseInlineSpecs(`null`)
	if err == nil || !strings.Contains(err.Error(), "JSON object or array") {
		t.Fatalf("expected JSON object/array error, got %v", err)
	}
	assertInlineValidationError(t, err)
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

func TestNormalizeRepeatedInlineFlagsReportsSafeOneBasedOccurrence(t *testing.T) {
	secret := "TOP-SECRET-INLINE-PAYLOAD"
	_, err := normalizeInlineFlagValues([]string{
		`{"cid":"hero","file_path":"./hero.png"}`,
		`{"cid":"` + secret + `","file_path":`,
	})
	if err == nil {
		t.Fatal("expected invalid second occurrence to fail")
	}
	if !strings.Contains(err.Error(), "occurrence 2") {
		t.Fatalf("error = %q, want one-based occurrence index", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error must not echo inline input, got %q", err)
	}
	assertInlineValidationError(t, err)
}

func TestNormalizeRepeatedInlineFlagsRejectsExplicitEmptyOccurrence(t *testing.T) {
	_, err := normalizeInlineFlagValues([]string{
		`{"cid":"hero","file_path":"./hero.png"}`,
		"   ",
	})
	if err == nil || !strings.Contains(err.Error(), "occurrence 2") {
		t.Fatalf("expected empty second occurrence error, got %v", err)
	}
	assertInlineValidationError(t, err)
}

func TestMailRepeatableInlineValidationPrecedesSideEffects(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	for _, tc := range []struct {
		name     string
		shortcut common.Shortcut
		extra    []string
	}{
		{name: "send", shortcut: MailSend, extra: []string{"--to", "a@example.com", "--subject", "subject", "--body", "<p>body</p>"}},
		{name: "draft-create", shortcut: MailDraftCreate, extra: []string{"--subject", "subject", "--body", "<p>body</p>"}},
		{name: "reply", shortcut: MailReply, extra: []string{"--message-id", "message", "--body", "<p>body</p>"}},
		{name: "reply-all", shortcut: MailReplyAll, extra: []string{"--message-id", "message", "--body", "<p>body</p>"}},
		{name: "forward", shortcut: MailForward, extra: []string{"--message-id", "message", "--to", "a@example.com", "--body", "<p>body</p>"}},
		{name: "template-create", shortcut: MailTemplateCreate, extra: []string{"--name", "name", "--template-content", "<p>body</p>"}},
		{name: "template-update", shortcut: MailTemplateUpdate, extra: []string{"--template-id", "1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{tc.shortcut.Command}, tc.extra...)
			args = append(args,
				"--inline", `{"cid":"hero","file_path":"./hero.png"}`,
				"--inline", `{"cid":"broken","file_path":`,
			)
			err := runMountedMailShortcut(t, tc.shortcut, args, f, stdout)
			if err == nil || !strings.Contains(err.Error(), "occurrence 2") {
				t.Fatalf("expected inline validation before command side effects, got %v", err)
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
