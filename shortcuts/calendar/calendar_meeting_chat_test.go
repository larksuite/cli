// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

// ---------------------------------------------------------------------------
// calendar +meeting-chat-create
// ---------------------------------------------------------------------------

func TestMeetingChatCreate_DryRun(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, calDefaultConfig())
	err := calMountAndRun(t, CalendarMeetingChatCreate,
		[]string{"+meeting-chat-create", "--event-id", "evtA", "--dry-run", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "/open-apis/calendar/v4/calendars/primary/events/evtA/meeting_chat") {
		t.Errorf("dry-run should show the create meeting_chat path, got: %s", out)
	}
	if !strings.Contains(out, "POST") {
		t.Errorf("dry-run should show POST method, got: %s", out)
	}
}

func TestMeetingChatCreate_Success(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, calDefaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/primary/events/evtA/meeting_chat",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"meeting_chat_id": "oc_chat_1",
				"applink":         "https://applink.feishu.cn/client/chat/open?openChatId=oc_chat_1",
			},
		},
	})

	err := calMountAndRun(t, CalendarMeetingChatCreate,
		[]string{"+meeting-chat-create", "--event-id", "evtA", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("parse output: %v\nstdout=%s", err, stdout.String())
	}
	data, _ := resp["data"].(map[string]any)
	if data["meeting_chat_id"] != "oc_chat_1" {
		t.Errorf("meeting_chat_id = %v, want oc_chat_1", data["meeting_chat_id"])
	}
	if data["event_id_used"] != "evtA" {
		t.Errorf("event_id_used = %v, want evtA", data["event_id_used"])
	}
	if _, ok := data["applink"]; ok {
		t.Errorf("applink must not be emitted, got: %#v", data)
	}
	if _, ok := data["hint"]; ok {
		t.Errorf("non-recurring create should not emit a fallback hint, got: %#v", data)
	}
}

// An unmaterialised recurring instance ({uid}_{originalTime>0}) resolves to
// 193001 on the pre-probe GET, so the create call must fall back to the master
// {uid}_0 and flag the downgrade.
func TestMeetingChatCreate_UnmaterialisedInstance_FallsBackToMaster(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, calDefaultConfig())
	// Pre-probe GET on the instance -> 193001 not found.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/primary/events/evt_series_1742515200",
		Body: map[string]interface{}{
			"code": 193001, "msg": "event not found",
			"data": map[string]interface{}{},
		},
	})
	// Create on the master succeeds.
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/primary/events/evt_series_0/meeting_chat",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"meeting_chat_id": "oc_series",
				"applink":         "https://applink.feishu.cn/client/chat/open?openChatId=oc_series",
			},
		},
	})

	err := calMountAndRun(t, CalendarMeetingChatCreate,
		[]string{"+meeting-chat-create", "--event-id", "evt_series_1742515200", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("parse output: %v\nstdout=%s", err, stdout.String())
	}
	data, _ := resp["data"].(map[string]any)
	if data["event_id_used"] != "evt_series_0" {
		t.Errorf("event_id_used = %v, want evt_series_0 (master)", data["event_id_used"])
	}
	if hint, _ := data["hint"].(string); !strings.Contains(hint, "重复性日程") {
		t.Errorf("expected fallback hint mentioning recurring series, got: %v", data["hint"])
	}
}

func TestMeetingChatCreate_UnsupportedEvent_MapsToHint(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, calDefaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/primary/events/evt_bad/meeting_chat",
		Body: map[string]interface{}{
			"code": 195109, "msg": "event not support chat creation",
			"data": map[string]interface{}{},
		},
	})

	err := calMountAndRun(t, CalendarMeetingChatCreate,
		[]string{"+meeting-chat-create", "--event-id", "evt_bad", "--as", "user"}, f, stdout)
	if err == nil {
		t.Fatal("expected error for code 195109")
	}
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want *errs.APIError, got %T: %v", err, err)
	}
	if ae.Code != 195109 {
		t.Errorf("code = %d, want 195109 preserved", ae.Code)
	}
	if ae.Subtype != errs.SubtypeFailedPrecondition {
		t.Errorf("subtype = %q, want failed_precondition", ae.Subtype)
	}
	if !strings.Contains(ae.Hint, "PRIMARY calendar") || !strings.Contains(ae.Hint, "2 attendees") {
		t.Errorf("hint should describe the meeting-chat preconditions, got: %q", ae.Hint)
	}
}

func TestMeetingChatCreate_Validation_MissingEventID(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, calDefaultConfig())
	err := calMountAndRun(t, CalendarMeetingChatCreate,
		[]string{"+meeting-chat-create", "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for missing --event-id")
	}
}

// ---------------------------------------------------------------------------
// calendar +meeting-chat-get
// ---------------------------------------------------------------------------

func TestMeetingChatGet_DryRun(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, calDefaultConfig())
	err := calMountAndRun(t, CalendarMeetingChatGet,
		[]string{"+meeting-chat-get", "--event-id", "evtA", "--dry-run", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "/open-apis/calendar/v4/calendars/primary/events/mget_meeting_chat") {
		t.Errorf("dry-run should show the mget_meeting_chat path, got: %s", out)
	}
	if !strings.Contains(out, "evtA") {
		t.Errorf("dry-run should carry the event id in the body, got: %s", out)
	}
}

func TestMeetingChatGet_Hit(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, calDefaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/primary/events/mget_meeting_chat",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"meeting_chats": []interface{}{
					map[string]interface{}{
						"event_id":          "evtA",
						"meeting_chat_id":   "oc_chat_1",
						"applink":           "https://applink.feishu.cn/client/chat/open?openChatId=oc_chat_1",
						"meeting_chat_type": "meeting_chat",
					},
				},
			},
		},
	})

	err := calMountAndRun(t, CalendarMeetingChatGet,
		[]string{"+meeting-chat-get", "--event-id", "evtA", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("parse output: %v\nstdout=%s", err, stdout.String())
	}
	data, _ := resp["data"].(map[string]any)
	if data["found"] != true {
		t.Errorf("found = %v, want true", data["found"])
	}
	if data["meeting_chat_id"] != "oc_chat_1" {
		t.Errorf("meeting_chat_id = %v, want oc_chat_1", data["meeting_chat_id"])
	}
	if _, ok := data["meeting_chat_type"]; ok {
		t.Errorf("meeting_chat_type must not be emitted, got: %#v", data)
	}
	if _, ok := data["applink"]; ok {
		t.Errorf("applink must not be emitted, got: %#v", data)
	}
}

func TestMeetingChatGet_Miss_SurfacesFailMsg(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, calDefaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/primary/events/mget_meeting_chat",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"failed_event_ids": []interface{}{
					map[string]interface{}{
						"event_id": "evtA",
						"fail_msg": "meeting chat not created",
					},
				},
			},
		},
	})

	err := calMountAndRun(t, CalendarMeetingChatGet,
		[]string{"+meeting-chat-get", "--event-id", "evtA", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("parse output: %v\nstdout=%s", err, stdout.String())
	}
	data, _ := resp["data"].(map[string]any)
	if data["found"] != false {
		t.Errorf("found = %v, want false", data["found"])
	}
	if data["fail_msg"] != "meeting chat not created" {
		t.Errorf("fail_msg = %v, want the server fail_msg verbatim", data["fail_msg"])
	}
}

func TestMeetingChatGet_UnmaterialisedInstance_FallsBackToMaster(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, calDefaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/primary/events/evt_series_1742515200",
		Body: map[string]interface{}{
			"code": 193001, "msg": "event not found",
			"data": map[string]interface{}{},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/primary/events/mget_meeting_chat",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"meeting_chats": []interface{}{
					map[string]interface{}{
						"event_id":          "evt_series_0",
						"meeting_chat_id":   "oc_series",
						"applink":           "https://applink.feishu.cn/client/chat/open?openChatId=oc_series",
						"meeting_chat_type": "meeting_chat",
					},
				},
			},
		},
	})

	err := calMountAndRun(t, CalendarMeetingChatGet,
		[]string{"+meeting-chat-get", "--event-id", "evt_series_1742515200", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("parse output: %v\nstdout=%s", err, stdout.String())
	}
	data, _ := resp["data"].(map[string]any)
	if data["event_id"] != "evt_series_0" {
		t.Errorf("event_id = %v, want evt_series_0 (master)", data["event_id"])
	}
	if data["found"] != true {
		t.Errorf("found = %v, want true", data["found"])
	}
	if hint, _ := data["hint"].(string); !strings.Contains(hint, "重复性日程") {
		t.Errorf("expected fallback hint mentioning recurring series, got: %v", data["hint"])
	}
}
