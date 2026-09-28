// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/httpmock"
)

const (
	citationTestMeetingID = "1000000000000000001"
	citationTestAppLink   = "https://applink.feishu.cn/client/vctab/open?source=chat&action=detail&meetingId=1000000000000000001"
)

type vcCitationEnvelope struct {
	Data      map[string]interface{} `json:"data"`
	Citations []string               `json:"citations"`
}

// vcCitationDoc decodes one XML <document> citation string so per-field
// assertions stay readable.
type vcCitationDoc struct {
	ReferenceID string              `xml:"reference_id,attr"`
	Title       string              `xml:"title"`
	SourceType  citation.SourceType `xml:"source_type"`
	URL         string              `xml:"url"`
}

func decodeVCCitationDoc(t *testing.T, s string) vcCitationDoc {
	t.Helper()
	// URL fields are emitted raw (consumer matches by exact string, no XML
	// unescape), so a bare & in a query string is expected; re-escape it to
	// make the document parseable for assertions.
	parseable := strings.ReplaceAll(s, "&", "&amp;")
	var d vcCitationDoc
	if err := xml.Unmarshal([]byte(parseable), &d); err != nil {
		t.Fatalf("xml.Unmarshal(%q) error = %v", s, err)
	}
	return d
}

func decodeVCCitationEnvelope(t *testing.T, stdout *bytes.Buffer) vcCitationEnvelope {
	t.Helper()
	var envelope vcCitationEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v\nstdout=%s", err, stdout.String())
	}
	return envelope
}

// meetingGetStubWithAppLink mirrors the meeting API once byteview-open-api
// returns app_link for every meeting status.
func meetingGetStubWithAppLink(meetingID, topic, appLink string) *httpmock.Stub {
	meeting := map[string]interface{}{"id": meetingID, "topic": topic}
	if appLink != "" {
		meeting["app_link"] = appLink
	}
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/meetings/" + meetingID,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"meeting": meeting},
		},
	}
}

func meetingRecordingStub(meetingID string) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/meetings/" + meetingID + "/recording",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"recording": map[string]interface{}{}},
		},
	}
}

// ---------------------------------------------------------------------------
// vc +detail
// ---------------------------------------------------------------------------

func TestVCDetailCitationUsesAppLinkTopicAndMeetingID(t *testing.T) {
	t.Setenv(envvars.CliCitation, "1")
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(meetingGetStubWithAppLink(citationTestMeetingID, "周会", citationTestAppLink))
	reg.Register(meetingRecordingStub(citationTestMeetingID))

	if err := mountAndRun(t, VCDetail,
		[]string{"+detail", "--meeting-ids", citationTestMeetingID, "--format", "json"}, f, stdout); err != nil {
		t.Fatalf("execute error = %v", err)
	}

	envelope := decodeVCCitationEnvelope(t, stdout)
	if len(envelope.Citations) != 1 {
		t.Fatalf("citations = %#v, want exactly 1", envelope.Citations)
	}
	doc := decodeVCCitationDoc(t, envelope.Citations[0])
	if doc.SourceType != citation.SourceMeeting {
		t.Errorf("source_type = %d, want %d", doc.SourceType, citation.SourceMeeting)
	}
	if doc.URL != citationTestAppLink {
		t.Errorf("url = %q, want the app_link the API returned", doc.URL)
	}
	if doc.Title != "周会" {
		t.Errorf("title = %q, want the meeting topic", doc.Title)
	}
	if doc.ReferenceID != doc.URL {
		t.Errorf("reference_id = %q, want it to mirror the url", doc.ReferenceID)
	}

	// The app_link reaches the builder through the unserialized struct field,
	// so it must never surface in data even when citations are on.
	meetings, _ := envelope.Data["meetings"].([]interface{})
	if len(meetings) != 1 {
		t.Fatalf("data.meetings = %#v, want 1 entry", envelope.Data["meetings"])
	}
	if first, _ := meetings[0].(map[string]interface{}); first["url"] != nil {
		t.Errorf("data.meetings[0].url = %v, want the url to stay off the wire", first["url"])
	}
}

func TestVCDetailCitationsSkipMeetingsWithoutAppLink(t *testing.T) {
	t.Setenv(envvars.CliCitation, "1")
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	// Before the byteview-open-api rollout the field is simply absent.
	reg.Register(meetingGetStubWithAppLink(citationTestMeetingID, "周会", ""))
	reg.Register(meetingRecordingStub(citationTestMeetingID))

	if err := mountAndRun(t, VCDetail,
		[]string{"+detail", "--meeting-ids", citationTestMeetingID, "--format", "json"}, f, stdout); err != nil {
		t.Fatalf("execute error = %v", err)
	}
	if got := decodeVCCitationEnvelope(t, stdout); got.Citations != nil {
		t.Errorf("citations = %#v, want none without an app_link", got.Citations)
	}
}

func TestVCDetailCitationsAreOmittedWhenGateIsOff(t *testing.T) {
	t.Setenv(envvars.CliCitation, "0")
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(meetingGetStubWithAppLink(citationTestMeetingID, "周会", citationTestAppLink))
	reg.Register(meetingRecordingStub(citationTestMeetingID))

	if err := mountAndRun(t, VCDetail,
		[]string{"+detail", "--meeting-ids", citationTestMeetingID, "--format", "json"}, f, stdout); err != nil {
		t.Fatalf("execute error = %v", err)
	}
	envelope := decodeVCCitationEnvelope(t, stdout)
	if envelope.Citations != nil {
		t.Errorf("citations = %#v, want none with the gate off", envelope.Citations)
	}
	// url only exists to feed the builder, so with the gate off the payload
	// must look exactly as it did before this command emitted citations.
	meetings, _ := envelope.Data["meetings"].([]interface{})
	if len(meetings) != 1 {
		t.Fatalf("data.meetings = %#v, want 1 entry", envelope.Data["meetings"])
	}
	if first, _ := meetings[0].(map[string]interface{}); first["url"] != nil {
		t.Errorf("data.meetings[0].url = %v, want absent with the gate off", first["url"])
	}
}

// ---------------------------------------------------------------------------
// vc +search
// ---------------------------------------------------------------------------

func meetingSearchStub(meetingID, appLink string) *httpmock.Stub {
	meta := map[string]interface{}{"description": "2026-08-25 10:00"}
	if appLink != "" {
		meta["app_link"] = appLink
	}
	return &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/vc/v1/meetings/search",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"id":           meetingID,
						"display_info": "周会",
						"meta_data":    meta,
					},
				},
				"has_more": false,
			},
		},
	}
}

func TestVCSearchCitationResolvesTopicThroughMeetingGet(t *testing.T) {
	t.Setenv(envvars.CliCitation, "1")
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(meetingSearchStub(citationTestMeetingID, citationTestAppLink))
	reg.Register(meetingGetStubWithAppLink(citationTestMeetingID, "周会", citationTestAppLink))

	if err := mountAndRun(t, VCSearch,
		[]string{"+search", "--query", "周会", "--format", "json", "--as", "user"}, f, stdout); err != nil {
		t.Fatalf("execute error = %v", err)
	}

	envelope := decodeVCCitationEnvelope(t, stdout)
	if len(envelope.Citations) != 1 {
		t.Fatalf("citations = %#v, want exactly 1", envelope.Citations)
	}
	doc := decodeVCCitationDoc(t, envelope.Citations[0])
	if doc.SourceType != citation.SourceMeeting {
		t.Errorf("source_type = %d, want %d", doc.SourceType, citation.SourceMeeting)
	}
	if doc.URL != citationTestAppLink {
		t.Errorf("url = %q, want meta_data.app_link", doc.URL)
	}
	if doc.Title != "周会" {
		t.Errorf("title = %q, want the topic resolved from the meeting API", doc.Title)
	}
	if doc.ReferenceID != doc.URL {
		t.Errorf("reference_id = %q, want it to mirror the url", doc.ReferenceID)
	}

	// The topic reaches the builder beside the payload, not inside it, so the
	// items must stay exactly as the search API returned them.
	items, _ := envelope.Data["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("data.items = %#v, want 1 entry", envelope.Data["items"])
	}
	if first, _ := items[0].(map[string]interface{}); first["topic"] != nil {
		t.Errorf("data.items[0].topic = %v, want the topic to stay off the wire", first["topic"])
	}
}

func TestVCSearchSkipsTopicLookupWhenGateIsOff(t *testing.T) {
	t.Setenv(envvars.CliCitation, "0")
	f, stdout, stderr, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(meetingSearchStub(citationTestMeetingID, citationTestAppLink))
	// No meeting.get stub is registered, so an attempted lookup would fail and
	// log to stderr. Asserting that silence is what proves the gate skipped it.
	if err := mountAndRun(t, VCSearch,
		[]string{"+search", "--query", "周会", "--format", "json", "--as", "user"}, f, stdout); err != nil {
		t.Fatalf("execute error = %v", err)
	}
	if strings.Contains(stderr.String(), "topic lookup") {
		t.Errorf("stderr = %q, want no topic lookup with the gate off", stderr.String())
	}

	envelope := decodeVCCitationEnvelope(t, stdout)
	if envelope.Citations != nil {
		t.Errorf("citations = %#v, want none with the gate off", envelope.Citations)
	}
	items, _ := envelope.Data["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("data.items = %#v, want 1 entry", envelope.Data["items"])
	}
	if first, _ := items[0].(map[string]interface{}); first["topic"] != nil {
		t.Errorf("data.items[0].topic = %v, want absent with the gate off", first["topic"])
	}
}

func TestVCSearchCitationsSkipItemsWithoutAppLink(t *testing.T) {
	t.Setenv(envvars.CliCitation, "1")
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(meetingSearchStub(citationTestMeetingID, ""))

	if err := mountAndRun(t, VCSearch,
		[]string{"+search", "--query", "周会", "--format", "json", "--as", "user"}, f, stdout); err != nil {
		t.Fatalf("execute error = %v", err)
	}
	if got := decodeVCCitationEnvelope(t, stdout); got.Citations != nil {
		t.Errorf("citations = %#v, want none without an app_link", got.Citations)
	}
}
