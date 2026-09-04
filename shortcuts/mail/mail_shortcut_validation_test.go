// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// assertValidationError fails the test unless err carries the validation
// category with ExitValidation exit code and a message containing wantSubstr.
// Mail-produced validation errors should be typed; the exit-code fallback keeps
// shared framework validation gates covered without asserting their shape here.
func assertValidationError(t *testing.T, err error, wantSubstr string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a validation error, got nil")
	}
	code := output.ExitCodeOf(err)
	if !errs.IsValidation(err) && code != output.ExitValidation {
		t.Fatalf("expected a validation-category error, got %T: %v", err, err)
	}
	if code != output.ExitValidation {
		t.Errorf("expected exit code %d (ExitValidation), got %d", output.ExitValidation, code)
	}
	if wantSubstr != "" && !strings.Contains(err.Error(), wantSubstr) {
		t.Errorf("expected error message to contain %q, got: %v", wantSubstr, err.Error())
	}
}

func assertValidationParamError(t *testing.T, err error, wantParam, wantSubstr string) {
	t.Helper()
	assertValidationError(t, err, wantSubstr)

	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if validationErr.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("subtype = %q, want %q", validationErr.Subtype, errs.SubtypeInvalidArgument)
	}
	if validationErr.Param != wantParam {
		t.Errorf("param = %q, want %q", validationErr.Param, wantParam)
	}
}

// assertValidatePasses fails the test if err is a validation error; other
// errors (e.g. API call failures from missing tokens) are acceptable because
// we only care that the Validate callback passed.
func assertValidatePasses(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	if errs.IsValidation(err) || output.ExitCodeOf(err) == output.ExitValidation {
		t.Fatalf("Validate callback should have passed but returned validation error: %v", err)
	}
	// Non-validation errors (auth/API failures) are expected without HTTP mocks.
}

func TestRequiredBodyRejectsWhitespaceBodyFile(t *testing.T) {
	for _, tc := range []struct {
		name     string
		shortcut common.Shortcut
		args     []string
	}{
		{
			name:     "send",
			shortcut: MailSend,
			args: []string{
				"+send", "--as", "user", "--to", "alice@example.com",
				"--subject", "blank body-file", "--body-file", "blank.html",
			},
		},
		{
			name:     "draft-create",
			shortcut: MailDraftCreate,
			args: []string{
				"+draft-create", "--as", "user",
				"--subject", "blank body-file", "--body-file", "blank.html",
			},
		},
		{
			name:     "reply",
			shortcut: MailReply,
			args: []string{
				"+reply", "--as", "user", "--message-id", "msg_001",
				"--body-file", "blank.html",
			},
		},
		{
			name:     "reply-all",
			shortcut: MailReplyAll,
			args: []string{
				"+reply-all", "--as", "user", "--message-id", "msg_001",
				"--body-file", "blank.html",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chdirTemp(t)
			if err := os.WriteFile("blank.html", []byte("  \n\t"), 0o644); err != nil {
				t.Fatal(err)
			}
			f, stdout, _, _ := mailShortcutTestFactory(t)
			err := runMountedMailShortcut(t, tc.shortcut, tc.args, f, stdout)
			assertValidationError(t, err, "--body or --body-file is required")
		})
	}
}

// TC-1: +message --as bot --mailbox me → ErrValidation
func TestMailMessageBotMailboxMeReturnsValidationError(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailMessage, []string{
		"+message", "--as", "bot", "--mailbox", "me", "--message-id", "msg_xxx",
	}, f, stdout)
	assertValidationError(t, err, "does not support --mailbox me")
}

// TC-2: +message --as bot --mailbox explicit → Validate passes
func TestMailMessageBotExplicitMailboxPassesValidation(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailMessage, []string{
		"+message", "--as", "bot", "--mailbox", "alice@example.com", "--message-id", "msg_xxx",
	}, f, stdout)
	assertValidatePasses(t, err)
}

// TC-3: +message --as user --mailbox me → Validate passes
func TestMailMessageUserMailboxMePassesValidation(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailMessage, []string{
		"+message", "--as", "user", "--mailbox", "me", "--message-id", "msg_xxx",
	}, f, stdout)
	assertValidatePasses(t, err)
}

// TC-4: +messages --as bot (default mailbox=me) → ErrValidation
func TestMailMessagesBotDefaultMailboxMeReturnsValidationError(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailMessages, []string{
		"+messages", "--as", "bot", "--message-ids", validMessageIDForTest("biz-x"),
	}, f, stdout)
	assertValidationError(t, err, "does not support --mailbox me")
}

// TC-5: +messages --as bot --mailbox explicit → Validate passes
func TestMailMessagesBotExplicitMailboxPassesValidation(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailMessages, []string{
		"+messages", "--as", "bot", "--mailbox", "alice@example.com", "--message-ids", validMessageIDForTest("biz-x"),
	}, f, stdout)
	assertValidatePasses(t, err)
}

// TC-6: +thread --as bot (default mailbox=me) → ErrValidation
func TestMailThreadBotDefaultMailboxMeReturnsValidationError(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailThread, []string{
		"+thread", "--as", "bot", "--thread-id", "thread_xxx",
	}, f, stdout)
	assertValidationError(t, err, "does not support --mailbox me")
}

// TC-7: +thread --as bot --mailbox explicit → Validate passes
func TestMailThreadBotExplicitMailboxPassesValidation(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailThread, []string{
		"+thread", "--as", "bot", "--mailbox", "alice@example.com", "--thread-id", "thread_xxx",
	}, f, stdout)
	assertValidatePasses(t, err)
}

// TC-8: +triage --as bot (default mailbox=me) → ErrValidation
func TestMailTriageBotDefaultMailboxMeReturnsValidationError(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailTriage, []string{
		"+triage", "--as", "bot",
	}, f, stdout)
	assertValidationError(t, err, "does not support --mailbox me")
}

// TC-9: +triage --as bot --mailbox explicit → Validate passes
func TestMailTriageBotExplicitMailboxPassesValidation(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailTriage, []string{
		"+triage", "--as", "bot", "--mailbox", "alice@example.com",
	}, f, stdout)
	assertValidatePasses(t, err)
}

func TestValidateUserMailboxID(t *testing.T) {
	for _, mailboxID := range []string{
		"me",
		"shared@example.com",
		"first.last+tag@example.co.uk",
	} {
		t.Run("valid/"+mailboxID, func(t *testing.T) {
			if err := validateUserMailboxID("--mailbox", mailboxID); err != nil {
				t.Fatalf("expected nil error, got: %v", err)
			}
		})
	}

	for _, mailboxID := range []string{
		"",
		"Alice <alice@example.com>",
		"alice@example.com,bob@example.com",
		"user/foo",
		"alice@",
		"alice@example.com\nx",
		" ",
		" me ",
		"shared@example.com ",
	} {
		t.Run("invalid/"+strings.ReplaceAll(mailboxID, "\n", "\\n"), func(t *testing.T) {
			err := validateUserMailboxID("--mailbox", mailboxID)
			assertValidationParamError(t, err, "--mailbox", `--mailbox must be "me" or a valid email address`)
		})
	}
}

func TestMailMessageInvalidMailboxValidationStopsBeforeHTTP(t *testing.T) {
	for _, mailboxID := range []string{"user/foo", " ", " me ", "shared@example.com "} {
		t.Run(strings.ReplaceAll(mailboxID, " ", "_"), func(t *testing.T) {
			f, stdout, _, _ := mailShortcutTestFactory(t)
			err := runMountedMailShortcut(t, MailMessage, []string{
				"+message", "--as", "user", "--mailbox", mailboxID, "--message-id", "msg_xxx",
			}, f, stdout)
			assertValidationParamError(t, err, "--mailbox", `--mailbox must be "me" or a valid email address`)
			if stdout.Len() != 0 {
				t.Fatalf("expected no command output before HTTP, got: %s", stdout.String())
			}
		})
	}
}

func TestMailTriageInvalidMailboxValidationStopsDryRun(t *testing.T) {
	for _, mailboxID := range []string{"user/foo", " ", " me ", "shared@example.com "} {
		t.Run(strings.ReplaceAll(mailboxID, " ", "_"), func(t *testing.T) {
			f, stdout, _, _ := mailShortcutTestFactory(t)
			err := runMountedMailShortcut(t, MailTriage, []string{
				"+triage", "--as", "user", "--mailbox", mailboxID, "--dry-run",
			}, f, stdout)
			assertValidationParamError(t, err, "--mailbox", `--mailbox must be "me" or a valid email address`)
			if strings.Contains(stdout.String(), "/open-apis/mail/v1/user_mailboxes/") {
				t.Fatalf("dry-run API output should not be generated, got: %s", stdout.String())
			}
		})
	}
}

func TestMailTriageValidMailboxDryRunStillGeneratesAPI(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailTriage, []string{
		"+triage", "--as", "user", "--mailbox", "shared@example.com", "--dry-run",
	}, f, stdout)
	if err != nil {
		t.Fatalf("expected valid mailbox dry-run to pass, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "/open-apis/mail/v1/user_mailboxes/shared@example.com/messages") {
		t.Fatalf("expected dry-run API output for explicit email mailbox, got: %s", stdout.String())
	}
}

// --- message_ids validation tests (S2) ---

func validMessageIDForTest(s string) string {
	return base64.URLEncoding.EncodeToString([]byte(s))
}

func rawMessageIDForTest(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

func TestValidateMessageIDsAcceptsValidIDs(t *testing.T) {
	_, err := validateMessageIDs(validMessageIDForTest("biz-001") + "," + validMessageIDForTest("biz-002"))
	if err != nil {
		t.Fatalf("expected nil error for valid IDs, got: %v", err)
	}
}

func TestValidateMessageIDsAcceptsRawBase64URLIDs(t *testing.T) {
	_, err := validateMessageIDs(rawMessageIDForTest("biz-raw-001"))
	if err != nil {
		t.Fatalf("expected nil error for raw base64url ID, got: %v", err)
	}
}

func TestValidateMessageIDsRejectsEmpty(t *testing.T) {
	_, err := validateMessageIDs("")
	assertValidationError(t, err, "--message-ids is required")
	_, err = validateMessageIDs("   ")
	assertValidationError(t, err, "--message-ids is required")
}

func TestValidateMessageIDsAcceptsMoreThanSingleBackendBatch(t *testing.T) {
	ids := make([]string, 21)
	for i := range ids {
		ids[i] = validMessageIDForTest(string(rune('a' + i)))
	}
	_, err := validateMessageIDs(strings.Join(ids, ","))
	if err != nil {
		t.Fatalf("expected nil error for more than one backend batch, got: %v", err)
	}
}

func TestValidateMessageIDsRejectsEmptyEntry(t *testing.T) {
	_, err := validateMessageIDs(validMessageIDForTest("biz-1") + ",," + validMessageIDForTest("biz-2"))
	assertValidationError(t, err, "entry 2 is empty")
}

func TestValidateMessageIDsRejectsLeadingOrTrailingWhitespace(t *testing.T) {
	id1 := validMessageIDForTest("biz-1")
	id2 := validMessageIDForTest("biz-2")
	_, err := validateMessageIDs(id1 + ", " + id2)
	assertValidationError(t, err, "must not contain leading or trailing whitespace")
	_, err = validateMessageIDs(" " + id1 + "," + id2)
	assertValidationError(t, err, "must not contain leading or trailing whitespace")
}

func TestValidateMessageIDsRejectsDuplicateIDs(t *testing.T) {
	id := validMessageIDForTest("biz-1")
	_, err := validateMessageIDs(id + "," + id)
	assertValidationError(t, err, "duplicate message ID is not allowed")
}

func TestValidateMessageIDsRejectsJSONLikeInput(t *testing.T) {
	_, err := validateMessageIDs(`["id1","id2"]`)
	assertValidationError(t, err, "expected a base64url")
}

func TestValidateMessageIDsRejectsColonJoinedInput(t *testing.T) {
	_, err := validateMessageIDs("id1:id2")
	assertValidationError(t, err, "expected a base64url")
}

func TestValidateMessageIDsRejectsNumericPrimaryID(t *testing.T) {
	_, err := validateMessageIDs("123456789")
	assertValidationError(t, err, "numeric primary IDs are not supported")
}

func TestValidateMessageIDsAcceptsExactlyTwenty(t *testing.T) {
	ids := make([]string, 20)
	for i := range ids {
		ids[i] = validMessageIDForTest(string(rune('A' + i)))
	}
	_, err := validateMessageIDs(strings.Join(ids, ","))
	if err != nil {
		t.Fatalf("expected nil error for exactly 20 IDs, got: %v", err)
	}
}

func TestValidateMessageIDRejectsInvalidBase64(t *testing.T) {
	_, err := validateMessageIDs("msg 1")
	assertValidationError(t, err, "expected a base64url")
	_, err = validateMessageIDs("not-base64!")
	assertValidationError(t, err, "expected a base64url")
}
