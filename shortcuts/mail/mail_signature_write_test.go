// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestMailSignatureWriteShortcutMetadata(t *testing.T) {
	for _, s := range []struct {
		name      string
		shortcut  common.Shortcut
		flagNames []string
	}{
		{
			name:      "create",
			shortcut:  MailSignatureCreate,
			flagNames: []string{"mailbox", "name", "content", "content-file", "device"},
		},
		{
			name:      "update",
			shortcut:  MailSignatureUpdate,
			flagNames: []string{"mailbox", "signature-id", "name", "content", "content-file", "device"},
		},
		{
			name:      "delete",
			shortcut:  MailSignatureDelete,
			flagNames: []string{"mailbox", "signature-id"},
		},
	} {
		t.Run(s.name, func(t *testing.T) {
			if !strings.HasPrefix(s.shortcut.Command, "+signature-") {
				t.Fatalf("command = %q", s.shortcut.Command)
			}
			if s.shortcut.Risk != "write" {
				t.Fatalf("risk = %q, want write", s.shortcut.Risk)
			}
			flags := map[string]common.Flag{}
			for _, flag := range s.shortcut.Flags {
				flags[flag.Name] = flag
			}
			for _, name := range s.flagNames {
				if _, ok := flags[name]; !ok {
					t.Fatalf("missing flag %q in %#v", name, s.shortcut.Flags)
				}
			}
		})
	}
	if got := MailSignatureCreate.Scopes; len(got) != 2 || got[0] != "mail:user_mailbox.message:modify" || got[1] != "mail:user_mailbox:readonly" {
		t.Fatalf("create scopes = %#v", got)
	}
	if got := MailSignatureCreate.AuthTypes; len(got) != 2 || got[0] != "user" || got[1] != "bot" {
		t.Fatalf("create auth = %#v", got)
	}
	if !MailSignatureCreate.HasFormat || !MailSignatureUpdate.HasFormat || !MailSignatureDelete.HasFormat {
		t.Fatal("signature write shortcuts must support --format")
	}
}

func TestMailSignatureCreatePostsUSERSignature(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/settings/signatures",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"signature": map[string]interface{}{
					"id":               "sig_001",
					"name":             "Work",
					"signature_type":   "USER",
					"signature_device": "MOBILE",
					"content":          "<p>Regards</p>",
				},
			},
		},
	}
	reg.Register(stub)

	err := runMountedMailShortcut(t, MailSignatureCreate, []string{
		"+signature-create",
		"--name", "Work",
		"--content", "<p>Regards</p>",
		"--device", "MOBILE",
	}, f, stdout)
	if err != nil {
		t.Fatalf("signature-create failed: %v", err)
	}

	body := decodeCapturedBody(t, stub)
	sig := body["signature"].(map[string]interface{})
	if sig["signature_type"] != "USER" || sig["signature_device"] != "MOBILE" {
		t.Fatalf("signature type/device = %#v", sig)
	}
	if sig["content"] != "<p>Regards</p>" {
		t.Fatalf("content = %v", sig["content"])
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	out := data["signature"].(map[string]interface{})
	if out["id"] != "sig_001" {
		t.Fatalf("output signature id = %v", out["id"])
	}
}

func TestMailSignatureUpdateFullReplaceWarningAndBody(t *testing.T) {
	f, stdout, stderr, reg := mailShortcutTestFactory(t)
	stub := &httpmock.Stub{
		Method: "PUT",
		URL:    "/user_mailboxes/shared@example.com/settings/signatures/sig_002",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"signature": map[string]interface{}{
					"id":               "sig_002",
					"name":             "Updated",
					"signature_type":   "USER",
					"signature_device": "PC",
				},
			},
		},
	}
	reg.Register(stub)

	err := runMountedMailShortcut(t, MailSignatureUpdate, []string{
		"+signature-update",
		"--mailbox", "shared@example.com",
		"--signature-id", "sig_002",
		"--name", "Updated",
	}, f, stdout)
	if err != nil {
		t.Fatalf("signature-update failed: %v", err)
	}
	if !strings.Contains(stderr.String(), "full-replace") {
		t.Fatalf("stderr missing full-replace warning: %s", stderr.String())
	}
	body := decodeCapturedBody(t, stub)
	sig := body["signature"].(map[string]interface{})
	if sig["id"] != "sig_002" || sig["name"] != "Updated" || sig["signature_device"] != "PC" {
		t.Fatalf("signature payload = %#v", sig)
	}
	if sig["content"] != "" {
		t.Fatalf("empty content should be sent explicitly for full replace, got %#v", sig)
	}
}

func TestMailSignatureDeleteCallsDelete(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/user_mailboxes/me/settings/signatures/sig_003",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
	})

	err := runMountedMailShortcut(t, MailSignatureDelete, []string{
		"+signature-delete",
		"--signature-id", "sig_003",
	}, f, stdout)
	if err != nil {
		t.Fatalf("signature-delete failed: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	if data["deleted"] != true || data["signature_id"] != "sig_003" {
		t.Fatalf("delete output = %#v", data)
	}
}

func TestMailSignatureWriteValidateErrors(t *testing.T) {
	cases := []struct {
		name     string
		shortcut any
		args     []string
		want     string
	}{
		{
			name:     "create content mutex",
			shortcut: MailSignatureCreate,
			args:     []string{"+signature-create", "--name", "n", "--content", "a", "--content-file", "b"},
			want:     "--content and --content-file are mutually exclusive",
		},
		{
			name:     "create invalid device",
			shortcut: MailSignatureCreate,
			args:     []string{"+signature-create", "--name", "n", "--device", "phone"},
			want:     "--device must be PC or MOBILE",
		},
		{
			name:     "update signature id required",
			shortcut: MailSignatureUpdate,
			args:     []string{"+signature-update", "--name", "n"},
			want:     `required flag(s) "signature-id" not set`,
		},
		{
			name:     "delete signature id required",
			shortcut: MailSignatureDelete,
			args:     []string{"+signature-delete"},
			want:     `required flag(s) "signature-id" not set`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, stdout, _, _ := mailShortcutTestFactory(t)
			var err error
			switch sc := tc.shortcut.(type) {
			case common.Shortcut:
				err = runMountedMailShortcut(t, sc, tc.args, f, stdout)
			default:
				t.Fatalf("unsupported shortcut type %T", tc.shortcut)
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

func TestDecorateSignatureWriteErrorAddsHints(t *testing.T) {
	err := errs.NewAPIError(errs.SubtypeUnknown, "duplicate").WithCode(signatureErrNameDuplicate)
	got := decorateSignatureWriteError(err, "create signature failed")
	if got != err {
		t.Fatalf("typed error should be decorated in place")
	}
	p, ok := errs.ProblemOf(got)
	if !ok {
		t.Fatalf("expected typed problem, got %T", got)
	}
	if !strings.Contains(p.Message, "create signature failed") || !strings.Contains(p.Hint, "different signature name") {
		t.Fatalf("decorated problem = %+v", p)
	}

	plain := errors.New("sdk down")
	got = decorateSignatureWriteError(plain, "delete signature failed")
	if _, ok := errs.ProblemOf(got); !ok {
		t.Fatalf("plain error should be upgraded to typed problem, got %T", got)
	}
	if !errors.Is(got, plain) {
		t.Fatalf("plain cause not preserved: %v", got)
	}
}
