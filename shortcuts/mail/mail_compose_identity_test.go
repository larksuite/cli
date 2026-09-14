// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestResolveComposeIdentityExplicitPrecedence(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("from", "", "")
	cmd.Flags().String("mailbox", "", "")
	_ = cmd.Flags().Set("from", "from@example.com")
	_ = cmd.Flags().Set("mailbox", "mailbox@example.com")

	got := resolveComposeIdentity(&common.RuntimeContext{Cmd: cmd}, "mailbox@example.com", composeScenarioNew, nil, nil)
	if got.Email != "from@example.com" {
		t.Fatalf("resolveComposeIdentity() email = %q, want explicit --from", got.Email)
	}
}

func TestUniqueDefaultComposeIdentityRequiresOneNonEmptyMarker(t *testing.T) {
	tests := []struct {
		name       string
		identities []composeIdentity
		wantEmail  string
		wantOK     bool
	}{
		{name: "one", identities: []composeIdentity{{Email: "first@example.com"}, {Email: "default@example.com", IsDefault: true}}, wantEmail: "default@example.com", wantOK: true},
		{name: "none", identities: []composeIdentity{{Email: "first@example.com"}}},
		{name: "multiple", identities: []composeIdentity{{Email: "one@example.com", IsDefault: true}, {Email: "two@example.com", IsDefault: true}}},
		{name: "empty default", identities: []composeIdentity{{Email: " ", IsDefault: true}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := uniqueDefaultComposeIdentity(tt.identities)
			if ok != tt.wantOK || got.Email != tt.wantEmail {
				t.Fatalf("uniqueDefaultComposeIdentity() = (%q, %v), want (%q, %v)", got.Email, ok, tt.wantEmail, tt.wantOK)
			}
		})
	}
}

func TestFirstMatchingComposeIdentityUsesToBeforeCCAndNormalizes(t *testing.T) {
	identities := []composeIdentity{
		{Email: "Alias@Example.com", Name: "Alias"},
		{Email: "cc@example.com", Name: "CC"},
	}
	got, ok := firstMatchingComposeIdentity(
		[]string{"other@example.com", " alias@example.COM "},
		[]string{"cc@example.com"},
		identities,
		composeIdentity{Email: "primary@example.com", Name: "Primary"},
	)
	if !ok || got.Email != "Alias@Example.com" || got.Name != "Alias" {
		t.Fatalf("firstMatchingComposeIdentity() = %#v, %v", got, ok)
	}
}

func TestMailSendUsesDefaultSendAsAddressAndName(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/settings/send_as",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"sendable_addresses": []interface{}{
					map[string]interface{}{"email_address": "first@example.com", "name": "First"},
					map[string]interface{}{"email_address": "alias@example.com", "name": "Default Alias", "is_default": true},
				},
			},
		},
	})
	registerMailboxProfileMock(reg)
	draftStub := registerDraftCreateCapture(reg)

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send", "--to", "recipient@example.com", "--subject", "subject", "--body", "body", "--no-signature",
	}, f, stdout)
	if err != nil {
		t.Fatalf("MailSend.Execute() error = %v", err)
	}
	eml := mustDecodeRawEMLFromStub(t, draftStub)
	if !strings.Contains(eml, "From: Default Alias <alias@example.com>") {
		t.Fatalf("default send-as identity not used in EML:\n%s", eml)
	}
}

func TestMailSendDoesNotUseFirstAddressWithoutTrustedDefault(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/settings/send_as",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"sendable_addresses": []interface{}{
					map[string]interface{}{"email_address": "first@example.com", "name": "First"},
					map[string]interface{}{"email_address": "other@example.com", "name": "Other"},
				},
			},
		},
	})
	registerMailboxProfileMock(reg)
	draftStub := registerDraftCreateCapture(reg)

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send", "--to", "recipient@example.com", "--subject", "subject", "--body", "body", "--no-signature",
	}, f, stdout)
	if err != nil {
		t.Fatalf("MailSend.Execute() error = %v", err)
	}
	eml := mustDecodeRawEMLFromStub(t, draftStub)
	if strings.Contains(eml, "first@example.com") || !strings.Contains(eml, "sender@example.com") {
		t.Fatalf("untrusted first send-as address was selected:\n%s", eml)
	}
}

func TestMailSendSettingsFailureUsesLegacyPrimaryAddress(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	registerMailboxProfileMock(reg)
	draftStub := registerDraftCreateCapture(reg)

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send", "--to", "recipient@example.com", "--subject", "subject", "--body", "body", "--no-signature",
	}, f, stdout)
	if err != nil {
		t.Fatalf("MailSend.Execute() error = %v", err)
	}
	eml := mustDecodeRawEMLFromStub(t, draftStub)
	if !strings.Contains(eml, "From: <sender@example.com>") {
		t.Fatalf("settings failure did not preserve the legacy primary sender:\n%s", eml)
	}
}

func TestMailReplyUsesFirstMatchingOriginalRecipientIdentity(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stubSourceMessageHTML(reg, `<p>Original</p>`)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/settings/send_as",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"sendable_addresses": []interface{}{
					map[string]interface{}{"email_address": "default@example.com", "name": "Default", "is_default": true},
					map[string]interface{}{"email_address": "me@example.com", "name": "Original Recipient"},
				},
			},
		},
	})
	draftStub := registerDraftCreateCapture(reg)

	err := runMountedMailShortcut(t, MailReply, []string{
		"+reply", "--message-id", "msg_w1", "--body", "reply", "--no-signature",
	}, f, stdout)
	if err != nil {
		t.Fatalf("MailReply.Execute() error = %v", err)
	}
	eml := mustDecodeRawEMLFromStub(t, draftStub)
	if !strings.Contains(eml, "From: Original Recipient <me@example.com>") {
		t.Fatalf("original recipient identity not used in reply EML:\n%s", eml)
	}
}

func registerDraftCreateCapture(reg *httpmock.Registry) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/drafts",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"draft_id": "d_identity"},
		},
	}
	reg.Register(stub)
	return stub
}
