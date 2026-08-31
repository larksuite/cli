// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/httpmock"
)

const (
	calCitationMeetingID = "1000000000000000001"
	calCitationAppLink   = "https://applink.feishu.cn/client/vctab/open?source=chat&action=detail&meetingId=1000000000000000001"
	calCitationEventID   = "abc_123"
)

type calCitationEnvelope struct {
	Data      map[string]interface{} `json:"data"`
	Citations []string               `json:"citations"`
}

// calCitationDoc decodes one XML <document> citation string so per-field
// assertions stay readable.
type calCitationDoc struct {
	ReferenceID string              `xml:"reference_id,attr"`
	Title       string              `xml:"title"`
	SourceType  citation.SourceType `xml:"source_type"`
	URL         string              `xml:"url"`
}

func decodeCalCitationDoc(t *testing.T, s string) calCitationDoc {
	t.Helper()
	// URL fields are emitted raw (consumer matches by exact string); re-escape
	// bare & so the document parses for assertions.
	parseable := strings.ReplaceAll(s, "&", "&amp;")
	var d calCitationDoc
	if err := xml.Unmarshal([]byte(parseable), &d); err != nil {
		t.Fatalf("xml.Unmarshal(%q) error = %v", s, err)
	}
	return d
}

func decodeCalCitationEnvelope(t *testing.T, stdout *bytes.Buffer) calCitationEnvelope {
	t.Helper()
	var envelope calCitationEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v\nstdout=%s", err, stdout.String())
	}
	return envelope
}

// mgetInstanceRelationStubWithURLs mirrors the calendar API once facade_oapi
// returns meeting_urls index-aligned with meeting_instance_ids.
func mgetInstanceRelationStubWithURLs(calendarID, instanceID string, meetingIDs, meetingURLs []string) *httpmock.Stub {
	info := map[string]interface{}{"instance_id": instanceID}
	ids := make([]interface{}, len(meetingIDs))
	for i, id := range meetingIDs {
		ids[i] = id
	}
	info["meeting_instance_ids"] = ids
	if meetingURLs != nil {
		urls := make([]interface{}, len(meetingURLs))
		for i, u := range meetingURLs {
			urls[i] = u
		}
		info["meeting_urls"] = urls
	}
	return &httpmock.Stub{
		Method: "POST",
		URL:    fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/mget_instance_relation_info", calendarID),
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"instance_relation_infos": []interface{}{info}},
		},
	}
}

func calMeetingGetStub(meetingID, topic string) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/meetings/" + meetingID,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"meeting": map[string]interface{}{"id": meetingID, "topic": topic},
			},
		},
	}
}

func TestCalendarMeetingCitationUsesURLTopicAndMeetingID(t *testing.T) {
	t.Setenv(envvars.CliCitation, "1")
	f, stdout, _, reg := cmdutil.TestFactory(t, calDefaultConfig())
	reg.Register(mgetInstanceRelationStubWithURLs("primary", calCitationEventID,
		[]string{calCitationMeetingID}, []string{calCitationAppLink}))
	reg.Register(calMeetingGetStub(calCitationMeetingID, "周会"))

	if err := calMountAndRun(t, CalendarMeeting,
		[]string{"+meeting", "--event-ids", calCitationEventID, "--format", "json", "--as", "user"}, f, stdout); err != nil {
		t.Fatalf("execute error = %v", err)
	}

	envelope := decodeCalCitationEnvelope(t, stdout)
	if len(envelope.Citations) != 1 {
		t.Fatalf("citations = %#v, want exactly 1", envelope.Citations)
	}
	doc := decodeCalCitationDoc(t, envelope.Citations[0])
	if doc.SourceType != citation.SourceMeeting {
		t.Errorf("source_type = %d, want %d", doc.SourceType, citation.SourceMeeting)
	}
	if doc.URL != calCitationAppLink {
		t.Errorf("url = %q, want the meeting_urls entry", doc.URL)
	}
	if doc.Title != "周会" {
		t.Errorf("title = %q, want the topic resolved from the meeting API", doc.Title)
	}
	if doc.ReferenceID != doc.URL {
		t.Errorf("reference_id = %q, want it to mirror the url", doc.ReferenceID)
	}

	// url and topic reach the builder through unserialized struct fields, so
	// they must never surface in data even when citations are on.
	meetings, _ := envelope.Data["meetings"].([]interface{})
	if len(meetings) != 1 {
		t.Fatalf("data.meetings = %#v, want 1 entry", envelope.Data["meetings"])
	}
	first, _ := meetings[0].(map[string]interface{})
	if first["url"] != nil || first["topic"] != nil {
		t.Errorf("data.meetings[0] = %#v, want url/topic to stay off the wire", first)
	}
}

func TestCalendarMeetingCitationsSkipEventsWithoutURL(t *testing.T) {
	t.Setenv(envvars.CliCitation, "1")
	f, stdout, _, reg := cmdutil.TestFactory(t, calDefaultConfig())
	// Before the facade_oapi rollout the field is simply absent.
	reg.Register(mgetInstanceRelationStubWithURLs("primary", calCitationEventID,
		[]string{calCitationMeetingID}, nil))

	if err := calMountAndRun(t, CalendarMeeting,
		[]string{"+meeting", "--event-ids", calCitationEventID, "--format", "json", "--as", "user"}, f, stdout); err != nil {
		t.Fatalf("execute error = %v", err)
	}
	if got := decodeCalCitationEnvelope(t, stdout); got.Citations != nil {
		t.Errorf("citations = %#v, want none without a meeting url", got.Citations)
	}
}

func TestCalendarMeetingSkipsTopicLookupWhenGateIsOff(t *testing.T) {
	t.Setenv(envvars.CliCitation, "0")
	f, stdout, stderr, reg := cmdutil.TestFactory(t, calDefaultConfig())
	reg.Register(mgetInstanceRelationStubWithURLs("primary", calCitationEventID,
		[]string{calCitationMeetingID}, []string{calCitationAppLink}))
	// No meeting.get stub is registered, so an attempted lookup would fail and
	// log to stderr. Asserting that silence is what proves the gate skipped it.
	if err := calMountAndRun(t, CalendarMeeting,
		[]string{"+meeting", "--event-ids", calCitationEventID, "--format", "json", "--as", "user"}, f, stdout); err != nil {
		t.Fatalf("execute error = %v", err)
	}
	if strings.Contains(stderr.String(), "topic lookup") {
		t.Errorf("stderr = %q, want no topic lookup with the gate off", stderr.String())
	}
	envelope := decodeCalCitationEnvelope(t, stdout)
	if envelope.Citations != nil {
		t.Errorf("citations = %#v, want none with the gate off", envelope.Citations)
	}
	// url and topic only exist to feed the builder, so with the gate off the
	// payload must look exactly as it did before this command emitted
	// citations.
	meetings, _ := envelope.Data["meetings"].([]interface{})
	if len(meetings) != 1 {
		t.Fatalf("data.meetings = %#v, want 1 entry", envelope.Data["meetings"])
	}
	first, _ := meetings[0].(map[string]interface{})
	if first["url"] != nil || first["topic"] != nil {
		t.Errorf("data.meetings[0] = %#v, want no url/topic with the gate off", first)
	}
}
