// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/httpmock"
)

type mailCitationEnvelope struct {
	OK        bool            `json:"ok"`
	Data      json.RawMessage `json:"data"`
	Citations []string        `json:"citations"`
}

type mailTriageCitationOutput struct {
	Messages  []map[string]interface{} `json:"messages"`
	Citations []string                 `json:"citations"`
}

func decodeMailTriageCitationEnvelope(t *testing.T, stdout interface{ Bytes() []byte }) mailTriageCitationOutput {
	t.Helper()
	var envelope mailCitationEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v", err)
	}
	if !envelope.OK {
		t.Fatalf("expected citation envelope, got %s", stdout.Bytes())
	}
	var output mailTriageCitationOutput
	if err := json.Unmarshal(envelope.Data, &output); err != nil {
		t.Fatalf("json.Unmarshal(envelope.data) error = %v", err)
	}
	output.Citations = envelope.Citations
	return output
}

type mailCitationDocument struct {
	SourceType  citation.SourceType `xml:"source_type"`
	URL         string              `xml:"url"`
	Title       string              `xml:"title"`
	Snippet     string              `xml:"snippet"`
	PublishTime string              `xml:"publish_time"`
}

func decodeMailCitationDocument(t *testing.T, encoded string) mailCitationDocument {
	t.Helper()
	var document mailCitationDocument
	if err := xml.Unmarshal([]byte(encoded), &document); err != nil {
		t.Fatalf("xml.Unmarshal(citation) error = %v", err)
	}
	return document
}

func TestMailCitationBuildersMapFieldsAndTimes(t *testing.T) {
	nonSearch := mailMessageCitations(nil, map[string]interface{}{
		"applink":       "https://example.com/client/mail/message/1",
		"subject":       "Subject",
		"title":         "Ignored search title",
		"body_preview":  "Preview",
		"internal_date": json.Number("1700000000000"),
		"create_time":   "1700000001",
	})
	if len(nonSearch) != 1 {
		t.Fatalf("mailMessageCitations() = %#v", nonSearch)
	}
	if got := nonSearch[0]; got.SourceType != citation.SourceMail || got.URL != "https://example.com/client/mail/message/1" || got.Title != "Subject" || got.Snippet != "" || got.PublishTime != citation.Time("1700000000000") {
		t.Fatalf("non-search citation = %#v", got)
	}

	nonSearch = mailMessageCitations(nil, map[string]interface{}{
		"applink":         "https://example.com/client/mail/message/1-plain",
		"subject":         "Subject",
		"body_preview":    "",
		"body_plain_text": "Plain fallback",
	})
	if len(nonSearch) != 1 || nonSearch[0].Snippet != "" || nonSearch[0].Title != "Subject" || nonSearch[0].URL != "https://example.com/client/mail/message/1-plain" {
		t.Fatalf("non-search citation must clear snippet and preserve metadata = %#v", nonSearch)
	}

	runtime := runtimeForMailTriageTest(t, map[string]string{"query": "quarterly"})
	search := mailTriageCitations(runtime, map[string]interface{}{
		"messages": []interface{}{
			map[string]interface{}{
				"applink":         "https://example.com/client/mail/message/2",
				"title":           "Search title",
				"subject":         "Fallback subject",
				"body_plain_text": "Plain search body",
				"create_time":     "1700000001",
			},
			"wrong type",
		},
	})
	if len(search) != 1 {
		t.Fatalf("mailTriageCitations() = %#v", search)
	}
	if got := search[0]; got.URL != "https://example.com/client/mail/message/2" || got.Title != "Search title" || got.Snippet != "" || got.PublishTime != citation.Time("1700000001") {
		t.Fatalf("search citation = %#v", got)
	}

	search = mailTriageCitations(runtime, map[string]interface{}{
		"messages": []map[string]interface{}{{
			"applink":         "https://example.com/client/mail/message/2-plain",
			"title":           "Search title",
			"body_plain_text": "Plain search fallback",
			"create_time":     "1700000001",
		}},
	})
	if len(search) != 1 || search[0].Snippet != "" || search[0].Title != "Search title" || search[0].URL != "https://example.com/client/mail/message/2-plain" {
		t.Fatalf("search citation must clear snippet and preserve metadata = %#v", search)
	}

	search = mailTriageCitations(runtime, map[string]interface{}{
		"messages": []map[string]interface{}{{
			"applink":       "https://example.com/client/mail/message/3",
			"internal_date": "1700000000000",
			"create_time":   "1700000001",
		}},
	})
	if got := search[0].PublishTime; got != citation.Time("1700000000000") {
		t.Fatalf("internal_date must take priority, got %q", got)
	}

	search = mailTriageCitations(runtime, map[string]interface{}{
		"messages": []map[string]interface{}{{
			"applink":       "https://example.com/client/mail/message/4",
			"internal_date": true,
			"create_time":   "1700000001",
		}},
	})
	if got := search[0].PublishTime; got != "" {
		t.Fatalf("malformed present internal_date must not use create_time, got %q", got)
	}
}

func TestMailCitationBuildersAreTypeSafeAndKeepEmptyURLCandidate(t *testing.T) {
	for _, value := range []interface{}{nil, "bad", map[string]interface{}{"messages": "bad"}} {
		if got := mailTriageCitations(nil, value); len(got) != 0 {
			t.Errorf("mailTriageCitations(%#v) = %#v", value, got)
		}
	}
	got := mailMessageCitations(nil, map[string]interface{}{
		"applink":       7,
		"subject":       []string{"bad"},
		"body_preview":  map[string]string{"bad": "type"},
		"internal_date": true,
	})
	if len(got) != 1 || got[0].URL != "" || got[0].Title != "" || got[0].Snippet != "" || got[0].PublishTime != "" {
		t.Fatalf("type-safe empty candidate = %#v", got)
	}
	if normalized := citation.Normalize(got); normalized != nil {
		t.Fatalf("empty URL must be silently filtered, got %#v", normalized)
	}
}

func TestMailMessageMountedCitationUsesFinalPayloadWithoutExtraRequest(t *testing.T) {
	t.Setenv(envvars.CliCitation, "1")
	factory, stdout, _, registry := mailShortcutTestFactory(t)
	registry.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/messages/msg_citation",
		OnMatch: func(req *http.Request) {
			if got := req.URL.Query().Get("include_applink"); got != "true" {
				t.Fatalf("include_applink query = %q, want true", got)
			}
		},
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"message": map[string]interface{}{
					"message_id":    "msg_citation",
					"applink":       "https://example.com/client/mail/message/1",
					"subject":       "Subject",
					"body_preview":  base64.RawURLEncoding.EncodeToString([]byte("Preview")),
					"internal_date": "1700000000000",
				},
			},
		},
	})
	if err := runMountedMailShortcut(t, MailMessage, []string{"+message", "--message-id", "msg_citation"}, factory, stdout); err != nil {
		t.Fatalf("MailMessage.Execute() error = %v", err)
	}
	registry.Verify(t)
	var envelope mailCitationEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v\n%s", err, stdout.String())
	}
	if !envelope.OK || len(envelope.Citations) != 1 {
		t.Fatalf("message envelope = %#v\n%s", envelope, stdout.String())
	}
	document := decodeMailCitationDocument(t, envelope.Citations[0])
	if document.SourceType != citation.SourceMail || document.URL != "https://example.com/client/mail/message/1" || document.Title != "Subject" || document.Snippet != "" || document.PublishTime != "2023-11-14T22:13:20Z" {
		t.Fatalf("message citation = %#v", document)
	}
}

func TestMailMessageDoesNotRequestApplinkWhenCitationDisabled(t *testing.T) {
	t.Setenv(envvars.CliCitation, "0")
	factory, stdout, stderr, registry := mailShortcutTestFactory(t)
	registry.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/messages/msg_no_citation",
		OnMatch: func(req *http.Request) {
			if got := req.URL.Query().Get("include_applink"); got != "" {
				t.Fatalf("include_applink query = %q, want empty", got)
			}
		},
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"message": map[string]interface{}{
					"message_id": "msg_no_citation",
					"subject":    "Subject",
				},
			},
		},
	})
	if err := runMountedMailShortcut(t, MailMessage, []string{"+message", "--message-id", "msg_no_citation"}, factory, stdout); err != nil {
		t.Fatalf("MailMessage.Execute() error = %v", err)
	}
	registry.Verify(t)
	var output map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v\n%s", err, stdout.String())
	}
	if _, ok := output["citations"]; ok {
		t.Fatalf("disabled citation emitted citations: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "confirm your email identity") {
		t.Fatalf("disabled citation must retain identity tip: %q", stderr.String())
	}
}

func TestMailTriageCitationDeclarationIsMailOnly(t *testing.T) {
	for name, shortcut := range map[string]struct {
		SourceTypes []citation.SourceType
	}{
		"message": {SourceTypes: MailMessage.Citation.SourceTypes},
		"triage":  {SourceTypes: MailTriage.Citation.SourceTypes},
	} {
		if len(shortcut.SourceTypes) != 1 || shortcut.SourceTypes[0] != citation.SourceMail {
			t.Errorf("%s SourceTypes = %#v", name, shortcut.SourceTypes)
		}
	}
}

func TestMailTriageDefaultFormatUsesCitationEnvelope(t *testing.T) {
	t.Setenv(envvars.CliCitation, "1")
	factory, stdout, stderr, registry := mailShortcutTestFactory(t)
	registerMailTriageListStub(registry, "me", []string{"msg_list"}, false, "")
	registerMailTriageBatchStub(registry, "me", []map[string]interface{}{
		{
			"message_id":    "msg_list",
			"applink":       "https://example.com/client/mail/message/list",
			"subject":       "List subject",
			"body_preview":  base64.RawURLEncoding.EncodeToString([]byte("List preview")),
			"internal_date": "1700000000000",
		},
	}, true)
	if err := runMountedMailShortcut(t, MailTriage, []string{"+triage", "--max", "1"}, factory, stdout); err != nil {
		t.Fatalf("MailTriage.Execute() error = %v", err)
	}
	registry.Verify(t)
	output := decodeMailTriageCitationEnvelope(t, stdout)
	if len(output.Messages) != 1 || output.Messages[0]["subject"] != "List subject" || len(output.Citations) != 1 {
		t.Fatalf("triage list output = %#v\n%s", output, stdout.String())
	}
	document := decodeMailCitationDocument(t, output.Citations[0])
	if document.SourceType != citation.SourceMail || document.URL != "https://example.com/client/mail/message/list" || document.Title != "List subject" || document.Snippet != "" || document.PublishTime != citation.Time("1700000000000") {
		t.Fatalf("triage list citation = %#v", document)
	}
	if strings.Contains(stderr.String(), "confirm your email identity") {
		t.Fatalf("configured citation env must suppress identity tip: %q", stderr.String())
	}
}

func TestMailTriageExplicitDataFormatUsesCitationEnvelope(t *testing.T) {
	t.Setenv(envvars.CliCitation, "1")
	factory, stdout, _, registry := mailShortcutTestFactory(t)
	registerMailTriageListStub(registry, "me", []string{"msg_data"}, false, "")
	registerMailTriageBatchStub(registry, "me", []map[string]interface{}{
		{
			"message_id": "msg_data",
			"applink":    "https://example.com/client/mail/message/data",
			"subject":    "Data subject",
		},
	}, true)
	if err := runMountedMailShortcut(t, MailTriage, []string{"+triage", "--format", "data", "--max", "1"}, factory, stdout); err != nil {
		t.Fatalf("MailTriage.Execute() error = %v", err)
	}
	registry.Verify(t)
	output := decodeMailTriageCitationEnvelope(t, stdout)
	if len(output.Messages) != 1 || output.Messages[0]["subject"] != "Data subject" || len(output.Citations) != 1 {
		t.Fatalf("triage data output = %#v\\n%s", output, stdout.String())
	}
}

func TestMailTriageExplicitTableFormatDoesNotEmitCitations(t *testing.T) {
	t.Setenv(envvars.CliCitation, "1")
	factory, stdout, stderr, registry := mailShortcutTestFactory(t)
	registerMailTriageListStub(registry, "me", []string{"msg_table"}, false, "")
	registerMailTriageBatchStub(registry, "me", []map[string]interface{}{
		{
			"message_id": "msg_table",
			"applink":    "https://example.com/client/mail/message/table",
			"subject":    "Table subject",
		},
	}, true)
	if err := runMountedMailShortcut(t, MailTriage, []string{"+triage", "--format", "table", "--max", "1"}, factory, stdout); err != nil {
		t.Fatalf("MailTriage.Execute() error = %v", err)
	}
	registry.Verify(t)
	if got := stdout.String(); strings.Contains(got, `"citations"`) || !strings.Contains(got, "Table subject") {
		t.Fatalf("explicit table output = %q", got)
	}
	if strings.Contains(stderr.String(), "confirm your email identity") {
		t.Fatalf("configured citation env must suppress identity tip: %q", stderr.String())
	}
}

func TestMailTriageDisabledCitationKeepsDefaultTable(t *testing.T) {
	for _, value := range []string{"0", ""} {
		t.Run("value="+value, func(t *testing.T) {
			t.Setenv(envvars.CliCitation, value)
			factory, stdout, stderr, registry := mailShortcutTestFactory(t)
			registerMailTriageListStub(registry, "me", nil, false, "")
			if err := runMountedMailShortcut(t, MailTriage, []string{"+triage", "--max", "1"}, factory, stdout); err != nil {
				t.Fatalf("MailTriage.Execute() error = %v", err)
			}
			registry.Verify(t)
			var output map[string]interface{}
			if err := json.Unmarshal(stdout.Bytes(), &output); err == nil {
				t.Fatalf("citation env value %q must preserve default table output: %s", value, stdout.String())
			}
			if !strings.Contains(stderr.String(), "confirm your email identity") {
				t.Fatalf("disabled citation must retain identity tip: %q", stderr.String())
			}
		})
	}
}

func TestMailTriageSearchCitationUsesExistingPayloadWithoutBatchGet(t *testing.T) {
	t.Setenv(envvars.CliCitation, "1")
	factory, stdout, _, registry := mailShortcutTestFactory(t)
	registerMailTriageSearchStub(registry, "me", []interface{}{
		map[string]interface{}{
			"meta_data": map[string]interface{}{
				"message_biz_id":  "msg_search",
				"app_link":        "https://example.com/client/mail/message/search",
				"title":           "Search title",
				"body_plain_text": base64.RawURLEncoding.EncodeToString([]byte("Plain search body")),
				"create_time":     "1700000001",
			},
		},
	}, false, "", true)
	if err := runMountedMailShortcut(t, MailTriage, []string{"+triage", "--format", "json", "--max", "1", "--query", "quarterly"}, factory, stdout); err != nil {
		t.Fatalf("MailTriage.Execute() error = %v", err)
	}
	registry.Verify(t)
	output := decodeMailTriageCitationEnvelope(t, stdout)
	if len(output.Messages) != 1 || len(output.Citations) != 1 {
		t.Fatalf("triage search output = %#v\n%s", output, stdout.String())
	}
	document := decodeMailCitationDocument(t, output.Citations[0])
	if document.SourceType != citation.SourceMail || document.URL != "https://example.com/client/mail/message/search" || document.Title != "Search title" || document.Snippet != "" || document.PublishTime != citation.Time("1700000001") {
		t.Fatalf("triage search citation = %#v", document)
	}
}

func TestMailTriageSearchCitationDropsResultWithoutApplink(t *testing.T) {
	t.Setenv(envvars.CliCitation, "1")
	factory, stdout, _, registry := mailShortcutTestFactory(t)
	registerMailTriageSearchStub(registry, "me", []interface{}{
		map[string]interface{}{
			"meta_data": map[string]interface{}{
				"message_biz_id":  "msg_search_no_link",
				"title":           "Search title",
				"body_plain_text": base64.RawURLEncoding.EncodeToString([]byte("Plain search body")),
				"create_time":     "1700000001",
			},
		},
	}, false, "", true)
	if err := runMountedMailShortcut(t, MailTriage, []string{"+triage", "--format", "json", "--max", "1", "--query", "quarterly"}, factory, stdout); err != nil {
		t.Fatalf("MailTriage.Execute() error = %v", err)
	}
	registry.Verify(t)
	output := decodeMailTriageCitationEnvelope(t, stdout)
	if len(output.Messages) != 1 || len(output.Citations) != 0 {
		t.Fatalf("triage search output with missing applink = %#v\n%s", output, stdout.String())
	}
}

func TestMailTriageCitationDisabledKeepsLegacyJSONShape(t *testing.T) {
	t.Setenv(envvars.CliCitation, "0")
	factory, stdout, _, registry := mailShortcutTestFactory(t)
	registerMailTriageListStub(registry, "me", nil, false, "")
	if err := runMountedMailShortcut(t, MailTriage, []string{"+triage", "--format", "json", "--max", "1"}, factory, stdout); err != nil {
		t.Fatalf("MailTriage.Execute() error = %v", err)
	}
	registry.Verify(t)
	var output map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v", err)
	}
	if _, ok := output["ok"]; ok {
		t.Fatalf("disabled citation must preserve raw triage JSON: %s", stdout.String())
	}
	if _, ok := output["citations"]; ok {
		t.Fatalf("disabled citation emitted citations: %s", stdout.String())
	}
}
