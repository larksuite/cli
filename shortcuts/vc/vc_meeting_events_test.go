// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

func newMeetingEventsRuntime() *common.RuntimeContext {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("meeting-id", "", "")
	cmd.Flags().String("start", "", "")
	cmd.Flags().String("end", "", "")
	cmd.Flags().String("page-token", "", "")
	cmd.Flags().String("page-size", "", "")
	cmd.Flags().Bool("page-all", false, "")
	return common.TestNewRuntimeContext(cmd, defaultConfig())
}

func mustSetMeetingEventsFlag(t *testing.T, runtime *common.RuntimeContext, name, value string) {
	t.Helper()
	if err := runtime.Cmd.Flags().Set(name, value); err != nil {
		t.Fatalf("Flags().Set(%q, %q) error = %v", name, value, err)
	}
}

func meetingEventsStub(events []interface{}, hasMore bool, pageToken string) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    vcMeetingEventsAPIPath,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"total":      len(events),
				"has_more":   hasMore,
				"page_token": pageToken,
				"events":     events,
			},
		},
	}
}

func botInfoStub() *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/bot/v3/info",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"bot": map[string]interface{}{
				"open_id":  "bot_001",
				"app_name": "Demo Bot",
			},
		},
	}
}

func botInfoErrorStub() *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/bot/v3/info",
		Status: 500,
		Body: map[string]interface{}{
			"code": 99991663,
			"msg":  "bot info unavailable",
		},
	}
}

func participantJoinedEvent() map[string]interface{} {
	return map[string]interface{}{
		"event_id":   "event-1",
		"event_type": "participant_joined",
		"event_time": "2026-04-17T08:00:00Z",
		"payload": map[string]interface{}{
			"activity_event_type": "participant_joined",
			"meeting": map[string]interface{}{
				"id":         "7628568141510692381",
				"topic":      "项目例会",
				"meeting_no": "724939760",
				"start_time": "1776407700",
				"end_time":   "1776411300",
			},
			"participant_joined_items": []interface{}{
				map[string]interface{}{
					"participant": map[string]interface{}{
						"id":        "bot_001",
						"user_name": "Demo Bot",
						"user_type": 2,
						"user_role": 4,
					},
					"join_time": "2026-04-17T08:00:00Z",
				},
			},
		},
	}
}

func participantJoinedEventOngoing() map[string]interface{} {
	event := participantJoinedEvent()
	payload := common.GetMap(event, "payload")
	meeting := common.GetMap(payload, "meeting")
	meeting["start_time"] = "1776410100"
	meeting["end_time"] = "1776410100"
	return event
}

func participantLeftEventWithReason(leaveReason int) map[string]interface{} {
	return map[string]interface{}{
		"event_id":   "event-left",
		"event_type": "participant_left",
		"event_time": "2026-04-17T07:18:50Z",
		"payload": map[string]interface{}{
			"activity_event_type": "participant_left",
			"meeting": map[string]interface{}{
				"id":         "7628568141510692381",
				"topic":      "项目例会",
				"meeting_no": "724939760",
				"start_time": "1776410100",
				"end_time":   "1776410100",
			},
			"participant_left_items": []interface{}{
				map[string]interface{}{
					"participant": map[string]interface{}{
						"id":        "bot_001",
						"user_name": "Demo Bot",
						"user_type": 2,
						"user_role": 4,
					},
					"leave_time":   "1776410330000",
					"leave_reason": leaveReason,
				},
			},
		},
	}
}

func chatReceivedEvent() map[string]interface{} {
	return map[string]interface{}{
		"event_id":   "event-2",
		"event_type": "chat_received",
		"event_time": "2026-04-17T08:05:00Z",
		"payload": map[string]interface{}{
			"activity_event_type": "chat_received",
			"meeting": map[string]interface{}{
				"id":         "7628568141510692381",
				"topic":      "项目例会",
				"meeting_no": "724939760",
				"start_time": "1776407700",
				"end_time":   "1776411300",
			},
			"participant_joined_items":  []interface{}{},
			"participant_left_items":    []interface{}{},
			"transcript_received_items": []interface{}{},
			"magic_share_started_items": []interface{}{},
			"magic_share_ended_items":   []interface{}{},
			"chat_received_items": []interface{}{
				map[string]interface{}{
					"content":      "hello",
					"message_type": 1,
					"operator": map[string]interface{}{
						"id":        "u1",
						"user_name": "Alice",
					},
				},
			},
		},
	}
}

func multiChatReceivedEvent() map[string]interface{} {
	return map[string]interface{}{
		"event_id":   "event-3",
		"event_type": "chat_received",
		"event_time": "2026-04-17T08:06:00Z",
		"payload": map[string]interface{}{
			"activity_event_type": "chat_received",
			"meeting": map[string]interface{}{
				"id":         "7628568141510692381",
				"topic":      "项目例会",
				"meeting_no": "724939760",
				"start_time": "1776407700",
				"end_time":   "1776411300",
			},
			"chat_received_items": []interface{}{
				map[string]interface{}{
					"content":      "第一条\n第二行",
					"message_type": 1,
					"send_time":    "1776408061000",
					"operator": map[string]interface{}{
						"id":        "u1",
						"user_name": "Alice",
					},
				},
				map[string]interface{}{
					"content":      "第二条",
					"message_type": 1,
					"send_time":    "1776408062000",
					"operator": map[string]interface{}{
						"id":        "u1",
						"user_name": "Alice",
					},
				},
			},
		},
	}
}

func mixedChatAndReactionEvent() map[string]interface{} {
	return map[string]interface{}{
		"event_id":   "event-reaction",
		"event_type": "chat_received",
		"event_time": "2026-04-17T08:05:00Z",
		"payload": map[string]interface{}{
			"activity_event_type": "chat_received",
			"meeting": map[string]interface{}{
				"id":         "7628568141510692381",
				"topic":      "项目例会",
				"meeting_no": "724939760",
				"start_time": "1776407700",
				"end_time":   "1776411300",
			},
			"chat_received_items": []interface{}{
				map[string]interface{}{
					"content":      "hello",
					"message_type": 1,
					"send_time":    "1776408061000",
					"operator": map[string]interface{}{
						"id":        "u1",
						"user_name": "Alice",
					},
				},
				map[string]interface{}{
					"content":      "OK",
					"message_type": 3,
					"send_time":    "1776408062000",
					"operator": map[string]interface{}{
						"id":        "u1",
						"user_name": "Alice",
					},
				},
			},
		},
	}
}

func magicShareStartedEvent() map[string]interface{} {
	return map[string]interface{}{
		"event_id":   "event-4",
		"event_type": "magic_share_started",
		"event_time": "2026-04-17T08:07:00Z",
		"payload": map[string]interface{}{
			"activity_event_type": "magic_share_started",
			"meeting": map[string]interface{}{
				"id":         "7628568141510692381",
				"topic":      "项目例会",
				"meeting_no": "724939760",
				"start_time": "1776407700",
				"end_time":   "1776411300",
			},
			"magic_share_started_items": []interface{}{
				map[string]interface{}{
					"time": "1776408123000",
					"operator": map[string]interface{}{
						"id":        "u2",
						"user_name": "Bob",
					},
					"share_doc": map[string]interface{}{
						"title": "共享文档",
						"url":   "https://example.com/doc",
					},
				},
			},
		},
	}
}

func documentContextChangedEvent(items []interface{}) map[string]interface{} {
	return map[string]interface{}{
		"event_id":   "event-document-context",
		"event_type": "document_context_changed",
		"event_time": "2026-04-17T08:08:00Z",
		"payload": map[string]interface{}{
			"activity_event_type":            "document_context_changed",
			"document_context_changed_items": items,
		},
	}
}

func countdownChangedEvent() map[string]interface{} {
	return map[string]interface{}{
		"event_id":   "event-countdown",
		"event_type": "countdown_changed",
		"event_time": "2026-08-17T08:38:40Z",
		"payload": map[string]interface{}{
			"activity_event_type": "countdown_changed",
			"meeting": map[string]interface{}{
				"id":         "7674839513696374289",
				"topic":      "用户204901的视频会议",
				"meeting_no": "795201611",
				"start_time": "2026-08-17T08:25:54Z",
			},
			"countdown_items": []interface{}{
				map[string]interface{}{
					"action":                 "SET",
					"countdown_set_time":     "1786955920442",
					"end_time":               "1786956520442",
					"event_time":             "1786955920465",
					"need_play_audio_at_end": false,
					"seq_id":                 "4",
					"operator":               map[string]interface{}{"id": "ou_201d63482ba649f81a7c9c67bd523b40", "user_name": "用户204901", "user_role": 2, "user_type": 1},
				},
				map[string]interface{}{
					"action":                 "PROLONG",
					"countdown_set_time":     "1786955920442",
					"end_time":               "1786956580442",
					"event_time":             "1786955923183",
					"need_play_audio_at_end": false,
					"seq_id":                 "5",
					"operator":               map[string]interface{}{"id": "ou_201d63482ba649f81a7c9c67bd523b40", "user_name": "用户204901", "user_role": 2, "user_type": 1},
				},
				map[string]interface{}{
					"action":                 "END_IN_ADVANCE",
					"end_time":               "0",
					"event_time":             "1786955924725",
					"need_play_audio_at_end": false,
					"seq_id":                 "6",
					"operator":               map[string]interface{}{"id": "ou_201d63482ba649f81a7c9c67bd523b40", "user_name": "用户204901", "user_role": 2, "user_type": 1},
				},
				map[string]interface{}{
					"action":                 "CLOSE_WINDOW",
					"end_time":               "0",
					"event_time":             "1786955926444",
					"need_play_audio_at_end": false,
					"seq_id":                 "7",
					"operator":               map[string]interface{}{"id": "ou_201d63482ba649f81a7c9c67bd523b40", "user_name": "用户204901", "user_role": 2, "user_type": 1},
				},
				map[string]interface{}{
					"action":                 "ENDED",
					"end_time":               "0",
					"event_time":             "1786955928000",
					"need_play_audio_at_end": false,
					"seq_id":                 "8",
				},
				map[string]interface{}{
					"action":         "REMIND",
					"event_time":     "1786955929000",
					"remain_minutes": "5",
					"seq_id":         "9",
				},
			},
		},
	}
}

func TestChatReceivedSummary_MultipleItems(t *testing.T) {
	payload := map[string]interface{}{
		"chat_received_items": []interface{}{
			map[string]interface{}{"content": "hello"},
			map[string]interface{}{"content": "world"},
		},
	}

	got := chatReceivedSummary(payload)
	if got != "2 messages" {
		t.Fatalf("chatReceivedSummary() = %q, want %q", got, "2 messages")
	}
}

func TestChatReceivedSummary_MultipleItemsSameOperator(t *testing.T) {
	payload := map[string]interface{}{
		"chat_received_items": []interface{}{
			map[string]interface{}{"content": "hello", "operator": map[string]interface{}{"id": "u1", "user_name": "Alice"}},
			map[string]interface{}{"content": "world", "operator": map[string]interface{}{"id": "u1", "user_name": "Alice"}},
		},
	}

	got := chatReceivedSummary(payload)
	if got != "2 messages by Alice" {
		t.Fatalf("chatReceivedSummary() = %q, want %q", got, "2 messages by Alice")
	}
}

func TestChatReceivedSummary_MultipleItemsMultipleOperators(t *testing.T) {
	payload := map[string]interface{}{
		"chat_received_items": []interface{}{
			map[string]interface{}{"content": "hello", "operator": map[string]interface{}{"id": "u1", "user_name": "Alice"}},
			map[string]interface{}{"content": "world", "operator": map[string]interface{}{"id": "u2", "user_name": "Bob"}},
			map[string]interface{}{"content": "again", "operator": map[string]interface{}{"id": "u3", "user_name": "Carol"}},
		},
	}

	got := chatReceivedSummary(payload)
	if got != "3 messages by 3 users" {
		t.Fatalf("chatReceivedSummary() = %q, want %q", got, "3 messages by 3 users")
	}
}

func TestParticipantJoinedSummary_MultipleItems(t *testing.T) {
	payload := map[string]interface{}{
		"participant_joined_items": []interface{}{
			map[string]interface{}{"participant": map[string]interface{}{"id": "u1", "user_name": "User 1"}},
			map[string]interface{}{"participant": map[string]interface{}{"id": "u2", "user_name": "User 2"}},
		},
	}

	got := participantJoinedSummary(payload)
	if got != "2 participants joined" {
		t.Fatalf("participantJoinedSummary() = %q, want %q", got, "2 participants joined")
	}
}

func TestMeetingEvents_Validation_InvalidMeetingID(t *testing.T) {
	runtime := newMeetingEventsRuntime()
	mustSetMeetingEventsFlag(t, runtime, "meeting-id", "not-a-number")

	err := VCMeetingEvents.Validate(context.Background(), runtime)
	if err == nil {
		t.Fatal("expected validation error for invalid meeting ID")
	}
	if !strings.Contains(err.Error(), "positive integer") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMeetingEvents_Validation_RejectsMeetingNumber(t *testing.T) {
	runtime := newMeetingEventsRuntime()
	mustSetMeetingEventsFlag(t, runtime, "meeting-id", "732067044")

	err := VCMeetingEvents.Validate(context.Background(), runtime)
	if err == nil {
		t.Fatal("expected validation error for 9-digit meeting number")
	}
	if !strings.Contains(err.Error(), "not a 9-digit meeting number") {
		t.Fatalf("unexpected error: %v", err)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Param != "--meeting-id" {
		t.Errorf("Param = %q, want %q", ve.Param, "--meeting-id")
	}
}

func TestMeetingEvents_Validation_InvalidTimeRange(t *testing.T) {
	runtime := newMeetingEventsRuntime()
	mustSetMeetingEventsFlag(t, runtime, "meeting-id", "7628568141510692381")
	mustSetMeetingEventsFlag(t, runtime, "start", "200")
	mustSetMeetingEventsFlag(t, runtime, "end", "100")

	err := VCMeetingEvents.Validate(context.Background(), runtime)
	if err == nil {
		t.Fatal("expected validation error for invalid time range")
	}
	if !strings.Contains(err.Error(), "after --end") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMeetingEvents_Validation_PageSizeBelowMinDoesNotError(t *testing.T) {
	runtime := newMeetingEventsRuntime()
	mustSetMeetingEventsFlag(t, runtime, "meeting-id", "7628568141510692381")
	mustSetMeetingEventsFlag(t, runtime, "page-size", "10")

	err := VCMeetingEvents.Validate(context.Background(), runtime)
	if err != nil {
		t.Fatalf("expected no validation error for page-size clamp, got: %v", err)
	}
}

func TestMeetingEvents_Validation_PageAllIgnoresInvalidPageSize(t *testing.T) {
	runtime := newMeetingEventsRuntime()
	mustSetMeetingEventsFlag(t, runtime, "meeting-id", "7628568141510692381")
	mustSetMeetingEventsFlag(t, runtime, "page-all", "true")
	mustSetMeetingEventsFlag(t, runtime, "page-size", "10")

	err := VCMeetingEvents.Validate(context.Background(), runtime)
	if err != nil {
		t.Fatalf("expected no validation error when page-all ignores page-size, got: %v", err)
	}
}

func TestMeetingEvents_UsesUserScopePreflightAndBotScopeHint(t *testing.T) {
	if got := VCMeetingEvents.ScopesForIdentity("user"); !reflect.DeepEqual(got, []string{meetingQueryUserScope}) {
		t.Fatalf("ScopesForIdentity(user) = %v, want %v", got, []string{meetingQueryUserScope})
	}
	if got := VCMeetingEvents.ScopesForIdentity("bot"); len(got) != 0 {
		t.Fatalf("ScopesForIdentity(bot) = %v, want no bot preflight scopes", got)
	}
	if got := VCMeetingEvents.DeclaredScopesForIdentity("user"); !reflect.DeepEqual(got, []string{meetingQueryUserScope}) {
		t.Fatalf("DeclaredScopesForIdentity(user) = %v, want %v", got, []string{meetingQueryUserScope})
	}
	if got := VCMeetingEvents.DeclaredScopesForIdentity("bot"); !reflect.DeepEqual(got, []string{meetingQueryBotScope}) {
		t.Fatalf("DeclaredScopesForIdentity(bot) = %v, want %v", got, []string{meetingQueryBotScope})
	}
}

func TestMeetingEvents_Validation_InvalidPageSizeReturnsFlagError(t *testing.T) {
	runtime := newMeetingEventsRuntime()
	mustSetMeetingEventsFlag(t, runtime, "meeting-id", "7628568141510692381")
	mustSetMeetingEventsFlag(t, runtime, "page-size", "foo")

	err := VCMeetingEvents.Validate(context.Background(), runtime)
	if err == nil {
		t.Fatal("expected validation error for non-integer page-size")
	}
	if !strings.Contains(err.Error(), "invalid --page-size") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildMeetingEventsParams(t *testing.T) {
	runtime := newMeetingEventsRuntime()
	mustSetMeetingEventsFlag(t, runtime, "meeting-id", "7628568141510692381")
	mustSetMeetingEventsFlag(t, runtime, "page-size", "40")
	mustSetMeetingEventsFlag(t, runtime, "page-token", "1710000000000000000")

	params, err := buildMeetingEventsParams(runtime, "1710000000", "1710003600")
	if err != nil {
		t.Fatalf("buildMeetingEventsParams() error = %v", err)
	}
	if got := params["meeting_id"]; got != "7628568141510692381" {
		t.Fatalf("meeting_id = %q, want %q", got, "7628568141510692381")
	}
	if got := params["page_size"]; got != "40" {
		t.Fatalf("page_size = %q, want %q", got, "40")
	}
	if got := params["page_token"]; got != "1710000000000000000" {
		t.Fatalf("page_token = %q, want %q", got, "1710000000000000000")
	}
	if got := params["start_time"]; got != "1710000000" {
		t.Fatalf("start_time = %q, want %q", got, "1710000000")
	}
	if got := params["end_time"]; got != "1710003600" {
		t.Fatalf("end_time = %q, want %q", got, "1710003600")
	}
}

func TestBuildMeetingEventsParams_PageSizeBelowMinClampsToMin(t *testing.T) {
	runtime := newMeetingEventsRuntime()
	mustSetMeetingEventsFlag(t, runtime, "meeting-id", "7628568141510692381")
	mustSetMeetingEventsFlag(t, runtime, "page-size", "10")

	params, err := buildMeetingEventsParams(runtime, "", "")
	if err != nil {
		t.Fatalf("buildMeetingEventsParams() error = %v", err)
	}
	if got := params["page_size"]; got != "20" {
		t.Fatalf("page_size = %q, want %q when below min", got, "20")
	}
}

func TestBuildMeetingEventsParams_PageSizeAboveMaxClampsToMax(t *testing.T) {
	runtime := newMeetingEventsRuntime()
	mustSetMeetingEventsFlag(t, runtime, "meeting-id", "7628568141510692381")
	mustSetMeetingEventsFlag(t, runtime, "page-size", "999")

	params, err := buildMeetingEventsParams(runtime, "", "")
	if err != nil {
		t.Fatalf("buildMeetingEventsParams() error = %v", err)
	}
	if got := params["page_size"]; got != "100" {
		t.Fatalf("page_size = %q, want %q when above max", got, "100")
	}
}

func TestBuildMeetingEventsParams_PageAllUsesMaxPageSize(t *testing.T) {
	runtime := newMeetingEventsRuntime()
	mustSetMeetingEventsFlag(t, runtime, "meeting-id", "7628568141510692381")
	mustSetMeetingEventsFlag(t, runtime, "page-all", "true")
	mustSetMeetingEventsFlag(t, runtime, "page-size", "50")

	params, err := buildMeetingEventsParams(runtime, "", "")
	if err != nil {
		t.Fatalf("buildMeetingEventsParams() error = %v", err)
	}
	if got := params["page_size"]; got != "100" {
		t.Fatalf("page_size = %q, want %q when page-all is set", got, "100")
	}
}

func TestMeetingEvents_DryRun(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCMeetingEvents, []string{
		"+meeting-events",
		"--meeting-id", "7628568141510692381",
		"--page-token", "1710000000000000000",
		"--page-size", "40",
		"--start", "1710000000",
		"--end", "1710003600",
		"--dry-run",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	for _, want := range []string{
		vcMeetingEventsAPIPath,
		`"meeting_id": "7628568141510692381"`,
		`"page_token": "1710000000000000000"`,
		`"page_size": "40"`,
		`"start_time": "1710000000"`,
		`"end_time": "1710003600"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q: %s", want, out)
		}
	}
}

func TestMeetingEvents_DryRun_PageAllUsesMaxLimit(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCMeetingEvents, []string{
		"+meeting-events",
		"--meeting-id", "7628568141510692381",
		"--page-all",
		"--dry-run",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout.String(), "Auto-paginates through all available pages") {
		t.Fatalf("dry-run output missing auto-pagination description: %s", stdout.String())
	}
}

func TestMeetingEvents_ExecuteJSON_PageAll(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(meetingEventsStub([]interface{}{participantJoinedEvent()}, true, "pt_2"))
	reg.Register(meetingEventsStub([]interface{}{participantJoinedEvent()}, false, ""))
	reg.Register(botInfoStub())

	err := mountAndRun(t, VCMeetingEvents, []string{
		"+meeting-events",
		"--meeting-id", "7628568141510692381",
		"--format", "json",
		"--page-all",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)

	var envelope map[string]interface{}
	if err := json.Unmarshal([]byte(stdout.String()), &envelope); err != nil {
		t.Fatalf("unmarshal stdout: %v: %s", err, stdout.String())
	}
	events := common.GetSlice(common.GetMap(envelope, "data"), "events")
	if got := len(events); got != 2 {
		t.Fatalf("events len = %d, want 2: %s", got, stdout.String())
	}
	for _, raw := range events {
		event, _ := raw.(map[string]interface{})
		if _, ok := event["summary"]; ok {
			t.Fatalf("event should not expose summary: %s", stdout.String())
		}
		if _, ok := event["raw"]; ok {
			t.Fatalf("event should not expose raw: %s", stdout.String())
		}
	}
	out := strings.ReplaceAll(stdout.String(), " ", "")
	out = strings.ReplaceAll(out, "\n", "")
	if !strings.Contains(out, `"has_more":false`) {
		t.Fatalf("expected final has_more=false: %s", stdout.String())
	}
}

func TestMeetingEvents_ExecuteJSON(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(meetingEventsStub([]interface{}{participantJoinedEvent()}, true, "1710000000000000000"))
	reg.Register(botInfoStub())

	err := mountAndRun(t, VCMeetingEvents, []string{
		"+meeting-events",
		"--meeting-id", "7628568141510692381",
		"--format", "json",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)

	out := strings.ReplaceAll(stdout.String(), " ", "")
	out = strings.ReplaceAll(out, "\n", "")
	for _, want := range []string{
		`"identity":{"id":"bot_001","name":"DemoBot","participant_type":"bot","label":"DemoBot[bot]"}`,
		`"role":"bot"`,
		`"event_type":"participant_joined"`,
		`"actors":[`,
		`"start_time":"2026-04-17T06:35:00Z"`,
		`"has_more":true`,
		`"page_token":"1710000000000000000"`,
		`"events":[`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("json output missing %q: %s", want, stdout.String())
		}
	}
	for _, unwanted := range []string{
		`"current_participants":`,
		`"is_self":`,
		`"summary":`,
		`"raw":`,
	} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("json output should not contain %q: %s", unwanted, stdout.String())
		}
	}
}

func TestMeetingEvents_Execute_NormalizesMeetingScopeError(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    vcMeetingEventsAPIPath,
		Status: 400,
		Body: map[string]interface{}{
			"code": output.LarkErrAppScopeNotEnabled,
			"msg":  "access denied",
			"error": map[string]interface{}{
				"permission_violations": []interface{}{
					map[string]interface{}{"subject": meetingQueryUserScope},
					map[string]interface{}{"subject": meetingQueryBotScope},
				},
			},
		},
	})

	err := mountAndRun(t, VCMeetingEvents, []string{
		"+meeting-events",
		"--meeting-id", "7628568141510692381",
		"--format", "json",
		"--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected permission error")
	}
	reg.Verify(t)

	var permissionErr *errs.PermissionError
	if !errors.As(err, &permissionErr) {
		t.Fatalf("error = %T %v, want *errs.PermissionError", err, err)
	}
	if permissionErr.Code != output.LarkErrAppScopeNotEnabled {
		t.Fatalf("Code = %d, want %d", permissionErr.Code, output.LarkErrAppScopeNotEnabled)
	}
	if permissionErr.Identity != "bot" {
		t.Fatalf("Identity = %q, want bot", permissionErr.Identity)
	}
	wantMessage := "access denied for bot identity; recommended scope: " + meetingQueryBotScope
	if permissionErr.Message != wantMessage {
		t.Fatalf("Message = %q, want %q", permissionErr.Message, wantMessage)
	}
	if !strings.Contains(permissionErr.Hint, meetingQueryBotScope) {
		t.Fatalf("Hint = %q, want bot scope %q", permissionErr.Hint, meetingQueryBotScope)
	}
	if len(permissionErr.MissingScopes) != 1 || permissionErr.MissingScopes[0] != meetingQueryBotScope {
		t.Fatalf("MissingScopes = %v, want only bot scope %q", permissionErr.MissingScopes, meetingQueryBotScope)
	}
	if permissionErr.ConsoleURL == "" {
		t.Fatal("ConsoleURL is empty, want identity-specific developer-console URL")
	}
	if strings.Contains(permissionErr.ConsoleURL, url.QueryEscape(meetingQueryUserScope)) || !strings.Contains(permissionErr.ConsoleURL, url.QueryEscape(meetingQueryBotScope)) {
		t.Fatalf("ConsoleURL = %q, want only bot scope", permissionErr.ConsoleURL)
	}
}

func TestMeetingEvents_ExecuteJSON_BotIdentityErrorDoesNotBlockEvents(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(meetingEventsStub([]interface{}{participantJoinedEvent()}, false, ""))
	reg.Register(botInfoErrorStub())

	err := mountAndRun(t, VCMeetingEvents, []string{
		"+meeting-events",
		"--meeting-id", "7628568141510692381",
		"--format", "json",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)

	out := strings.ReplaceAll(stdout.String(), " ", "")
	out = strings.ReplaceAll(out, "\n", "")
	for _, want := range []string{
		`"event_type":"participant_joined"`,
		`"identity":{"participant_type":"bot","label":"bot"}`,
		`"warnings":[`,
		`identityunavailable`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("json output missing %q: %s", want, stdout.String())
		}
	}
}

func TestMeetingEvents_ExecuteJSON_UserIdentitySkipsBotInfo(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(meetingEventsStub([]interface{}{participantJoinedEvent()}, false, ""))

	err := mountAndRun(t, VCMeetingEvents, []string{
		"+meeting-events",
		"--meeting-id", "7628568141510692381",
		"--format", "json",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)

	out := strings.ReplaceAll(stdout.String(), " ", "")
	out = strings.ReplaceAll(out, "\n", "")
	for _, want := range []string{
		`"identity":{"id":"ou_testuser","participant_type":"human","label":"ou_testuser[human]"}`,
		`"event_type":"participant_joined"`,
		`"has_more":false`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("user json output missing %q: %s", want, stdout.String())
		}
	}
}

func TestMeetingEvents_ExecuteJSON_OngoingMeetingOmitsEndTime(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(meetingEventsStub([]interface{}{participantJoinedEventOngoing()}, false, ""))
	reg.Register(botInfoStub())

	err := mountAndRun(t, VCMeetingEvents, []string{
		"+meeting-events",
		"--meeting-id", "7628568141510692381",
		"--format", "json",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)

	var envelope map[string]interface{}
	if err := json.Unmarshal([]byte(stdout.String()), &envelope); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, stdout.String())
	}
	data := common.GetMap(envelope, "data")
	meeting := common.GetMap(data, "meeting")
	if got := common.GetString(meeting, "status"); got != "ongoing" {
		t.Fatalf("meeting status = %q, want ongoing: %s", got, stdout.String())
	}
	if _, ok := meeting["end_time"]; ok {
		t.Fatalf("ongoing meeting should not expose dirty top-level end_time: %s", stdout.String())
	}
}

func TestBuildMeetingEventsOutput_MeetingEndedLeaveReasonOverridesDirtyMeetingEndTime(t *testing.T) {
	out := buildMeetingEventsOutput(map[string]interface{}{}, []interface{}{
		participantLeftEventWithReason(leaveReasonMeetingEnded),
	}, meetingEventsIdentity{})

	if got := out.Meeting.Status; got != "ended" {
		t.Fatalf("meeting status = %q, want ended", got)
	}
	if got := out.Meeting.EndTime; got != "2026-04-17T07:18:50Z" {
		t.Fatalf("meeting end_time = %q, want leave time", got)
	}
}

func TestBuildMeetingEventsOutput_NormalLeaveReasonDoesNotEndMeeting(t *testing.T) {
	out := buildMeetingEventsOutput(map[string]interface{}{}, []interface{}{
		participantLeftEventWithReason(leaveReasonUserLeft),
	}, meetingEventsIdentity{})

	if got := out.Meeting.Status; got != "ongoing" {
		t.Fatalf("meeting status = %q, want ongoing", got)
	}
	if got := out.Meeting.EndTime; got != "" {
		t.Fatalf("meeting end_time = %q, want empty", got)
	}
}

func TestRenderMeetingEventsPretty_MeetingEndedLeaveReasonOverridesDirtyMeetingEndTime(t *testing.T) {
	timeline := buildMeetingEventTimeline([]interface{}{
		participantLeftEventWithReason(leaveReasonMeetingEnded),
	})
	got := renderMeetingEventsPretty(timeline)

	if strings.Contains(got, "进行中") {
		t.Fatalf("pretty output should not show ongoing for meeting-ended leave reason: %s", got)
	}
	if !strings.Contains(got, "会议时间：2026-04-17 15:15:00 - 2026-04-17 15:18:50") {
		t.Fatalf("pretty output missing derived meeting end window: %s", got)
	}
}

func TestBuildMeetingEventsOutput_UsesLatestMeetingSnapshot(t *testing.T) {
	out := buildMeetingEventsOutput(map[string]interface{}{}, []interface{}{
		participantJoinedEventOngoing(),
		participantJoinedEvent(),
	}, meetingEventsIdentity{})

	if got := out.Meeting.Status; got != "ended" {
		t.Fatalf("meeting status = %q, want ended", got)
	}
	if got := out.Meeting.EndTime; got != "2026-04-17T07:35:00Z" {
		t.Fatalf("meeting end_time = %q, want latest ended snapshot", got)
	}
	if got := len(out.Events); got != 2 {
		t.Fatalf("events len = %d, want 2", got)
	}
}

func TestBuildMeetingEventsOutput_EmptyEventsHasUnknownMeetingStatus(t *testing.T) {
	out := buildMeetingEventsOutput(map[string]interface{}{}, nil, meetingEventsIdentity{})

	if got := out.Meeting.Status; got != "unknown" {
		t.Fatalf("meeting status = %q, want unknown", got)
	}
}

func TestMeetingEventsMeetingFromPayload_StartOnlyIsOngoing(t *testing.T) {
	got := meetingEventsMeetingFromPayload(map[string]interface{}{
		"id":         "m1",
		"start_time": "1776410100",
	})

	if got.Status != "ongoing" {
		t.Fatalf("meeting status = %q, want ongoing", got.Status)
	}
	if got.StartTime != "2026-04-17T07:15:00Z" {
		t.Fatalf("meeting start_time = %q, want normalized RFC3339", got.StartTime)
	}
	if got.EndTime != "" {
		t.Fatalf("meeting end_time = %q, want empty", got.EndTime)
	}
}

func TestMeetingEvents_ExecuteNDJSONIncludesMetadataRow(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(meetingEventsStub([]interface{}{participantJoinedEvent()}, true, "1710000000000000000"))
	reg.Register(botInfoStub())

	err := mountAndRun(t, VCMeetingEvents, []string{
		"+meeting-events",
		"--meeting-id", "7628568141510692381",
		"--format", "ndjson",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("ndjson lines = %d, want 2: %s", len(lines), stdout.String())
	}
	if !strings.Contains(lines[0], `"row_type":"event"`) || !strings.Contains(lines[0], `"event_type":"participant_joined"`) {
		t.Fatalf("first ndjson row should be event: %s", lines[0])
	}
	for _, unwanted := range []string{
		`"summary":`,
		`"raw":`,
	} {
		if strings.Contains(lines[0], unwanted) {
			t.Fatalf("event ndjson row should not contain %q: %s", unwanted, lines[0])
		}
	}
	for _, want := range []string{
		`"row_type":"metadata"`,
		`"has_more":true`,
		`"page_token":"1710000000000000000"`,
		`"identity":`,
	} {
		if !strings.Contains(lines[1], want) {
			t.Fatalf("metadata ndjson row missing %q: %s", want, lines[1])
		}
	}
}

func TestMeetingEventsEventRows_OmitsEmptyEventFields(t *testing.T) {
	rows := meetingEventsEventRows([]meetingEventsEvent{
		{EventType: "unknown_event"},
	}, nil)
	if len(rows) != 1 {
		t.Fatalf("rows len = %d, want 1", len(rows))
	}
	row, ok := rows[0].(map[string]interface{})
	if !ok {
		t.Fatalf("row type = %T, want map", rows[0])
	}
	for _, unwanted := range []string{"event_id", "event_time", "actors", "payload"} {
		if _, exists := row[unwanted]; exists {
			t.Fatalf("row should omit %q when empty: %#v", unwanted, row)
		}
	}
	if got := row["row_type"]; got != "event" {
		t.Fatalf("row_type = %v, want event", got)
	}
	if got := row["event_type"]; got != "unknown_event" {
		t.Fatalf("event_type = %v, want unknown_event", got)
	}
}

func TestMeetingEvents_ExecuteJSON_PrunesEmptySlices(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(meetingEventsStub([]interface{}{chatReceivedEvent()}, false, ""))
	reg.Register(botInfoStub())

	err := mountAndRun(t, VCMeetingEvents, []string{
		"+meeting-events",
		"--meeting-id", "7628568141510692381",
		"--format", "json",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)

	out := stdout.String()
	for _, unwanted := range []string{
		`"participant_joined_items": []`,
		`"participant_left_items": []`,
		`"transcript_received_items": []`,
		`"magic_share_started_items": []`,
		`"magic_share_ended_items": []`,
	} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("json output should not contain %q: %s", unwanted, out)
		}
	}
	if !strings.Contains(out, `"message_type": 1`) {
		t.Fatalf("json output should keep numeric fields: %s", out)
	}
}

func TestMeetingEvents_ExecuteJSON_PreservesReactionItems(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(meetingEventsStub([]interface{}{mixedChatAndReactionEvent()}, false, ""))
	reg.Register(botInfoStub())

	err := mountAndRun(t, VCMeetingEvents, []string{
		"+meeting-events",
		"--meeting-id", "7628568141510692381",
		"--format", "json",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)

	out := strings.ReplaceAll(stdout.String(), " ", "")
	out = strings.ReplaceAll(out, "\n", "")
	for _, want := range []string{
		`"event_type":"chat_received"`,
		`"chat_received_items":[`,
		`"content":"OK"`,
		`"message_type":3`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("json output missing %q: %s", want, stdout.String())
		}
	}
	if strings.Contains(out, `"im_post"`) {
		t.Fatalf("json output should not include IM post payload: %s", stdout.String())
	}
}

func TestMeetingEvents_ExecutePretty(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(meetingEventsStub([]interface{}{participantJoinedEventOngoing(), multiChatReceivedEvent(), magicShareStartedEvent()}, true, "1710000000000000000"))
	reg.Register(botInfoStub())

	err := mountAndRun(t, VCMeetingEvents, []string{
		"+meeting-events",
		"--meeting-id", "7628568141510692381",
		"--format", "pretty",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)

	out := stdout.String()
	for _, want := range []string{
		"当前身份：Demo Bot [bot]",
		"会议主题：项目例会",
		"会议时间：2026-04-17 15:15:00（进行中）",
		"Demo Bot(bot_001) 加入了会议",
		"Alice(u1): [text] 第一条\\n第二行",
		"Alice(u1): [text] 第二条",
		"Bob(u2) 开始共享「共享文档」",
		"URL: https://example.com/doc",
		"page_token: 1710000000000000000",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("pretty output missing %q: %s", want, out)
		}
	}
	if strings.Contains(out, "第二条\n\n[") {
		t.Fatalf("pretty output should not insert blank lines between event entries: %s", out)
	}
	if !strings.Contains(out, "第二条\n[") {
		t.Fatalf("pretty output should keep event entries contiguous: %s", out)
	}
}

func TestMeetingEvents_ExecutePretty_PrintsPageTokenWithoutHasMore(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(meetingEventsStub([]interface{}{participantJoinedEventOngoing()}, false, "pt_last"))
	reg.Register(botInfoStub())

	err := mountAndRun(t, VCMeetingEvents, []string{
		"+meeting-events",
		"--meeting-id", "7628568141510692381",
		"--format", "pretty",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)

	out := stdout.String()
	if !strings.Contains(out, "page_token: pt_last") {
		t.Fatalf("pretty output should print page_token even when has_more is false: %s", out)
	}
	if strings.Contains(out, "more available") {
		t.Fatalf("pretty output should not print more-available hint when has_more is false: %s", out)
	}
}

func TestMeetingEvents_ExecuteEmpty(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(meetingEventsStub(nil, false, ""))
	reg.Register(botInfoStub())

	err := mountAndRun(t, VCMeetingEvents, []string{
		"+meeting-events",
		"--meeting-id", "7628568141510692381",
		"--format", "pretty",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)

	if !strings.Contains(stdout.String(), "No meeting events.") {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestParseFlexibleTime(t *testing.T) {
	t.Run("unix seconds", func(t *testing.T) {
		got, ok := parseFlexibleTime("1776410100")
		if !ok {
			t.Fatal("parseFlexibleTime() ok = false, want true")
		}
		if want := time.Unix(1776410100, 0); !got.Equal(want) {
			t.Fatalf("parseFlexibleTime() = %v, want %v", got, want)
		}
	})

	t.Run("unix millis", func(t *testing.T) {
		got, ok := parseFlexibleTime("1776408061000")
		if !ok {
			t.Fatal("parseFlexibleTime() ok = false, want true")
		}
		if want := time.UnixMilli(1776408061000); !got.Equal(want) {
			t.Fatalf("parseFlexibleTime() = %v, want %v", got, want)
		}
	})

	t.Run("rfc3339", func(t *testing.T) {
		got, ok := parseFlexibleTime("2026-04-17T08:00:00Z")
		if !ok {
			t.Fatal("parseFlexibleTime() ok = false, want true")
		}
		if want, _ := time.Parse(time.RFC3339, "2026-04-17T08:00:00Z"); !got.Equal(want) {
			t.Fatalf("parseFlexibleTime() = %v, want %v", got, want)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		if _, ok := parseFlexibleTime("not-a-time"); ok {
			t.Fatal("parseFlexibleTime() ok = true, want false")
		}
	})
}

func TestFormatMeetingWindow(t *testing.T) {
	start := time.Unix(1776410100, 0)
	end := time.Unix(1776413700, 0)

	tests := []struct {
		name     string
		start    time.Time
		hasStart bool
		end      time.Time
		hasEnd   bool
		want     string
	}{
		{
			name:     "ongoing",
			start:    start,
			hasStart: true,
			end:      start,
			hasEnd:   true,
			want:     "2026-04-17 15:15:00（进行中）",
		},
		{
			name:     "finished range",
			start:    start,
			hasStart: true,
			end:      end,
			hasEnd:   true,
			want:     "2026-04-17 15:15:00 - 2026-04-17 16:15:00",
		},
		{
			name:     "only start",
			start:    start,
			hasStart: true,
			want:     "2026-04-17 15:15:00",
		},
		{
			name:   "only end",
			end:    end,
			hasEnd: true,
			want:   "2026-04-17 16:15:00",
		},
		{
			name: "empty",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatMeetingWindow(tt.start, tt.hasStart, tt.end, tt.hasEnd); got != tt.want {
				t.Fatalf("formatMeetingWindow() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatTimelineOffset(t *testing.T) {
	start := time.Unix(1776410100, 0)
	later := start.Add(90 * time.Second)
	earlier := start.Add(-5 * time.Minute)

	tests := []struct {
		name            string
		when            time.Time
		hasWhen         bool
		meetingStart    time.Time
		hasMeetingStart bool
		want            string
	}{
		{
			name:            "with meeting start",
			when:            later,
			hasWhen:         true,
			meetingStart:    start,
			hasMeetingStart: true,
			want:            "00:01:30",
		},
		{
			name:            "negative diff clamps to zero",
			when:            earlier,
			hasWhen:         true,
			meetingStart:    start,
			hasMeetingStart: true,
			want:            "00:00:00",
		},
		{
			name:    "without meeting start uses wall clock",
			when:    later,
			hasWhen: true,
			want:    "15:16:30",
		},
		{
			name: "missing when",
			want: "??:??:??",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatTimelineOffset(tt.when, tt.hasWhen, tt.meetingStart, tt.hasMeetingStart); got != tt.want {
				t.Fatalf("formatTimelineOffset() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFlattenQueryParams(t *testing.T) {
	params := map[string]interface{}{
		"one":  "1",
		"many": "2",
	}

	got := flattenQueryParams(params)
	want := map[string]interface{}{
		"one":  "1",
		"many": "2",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("flattenQueryParams() = %#v, want %#v", got, want)
	}
}

func TestFlattenQueryParams_NilOnEmpty(t *testing.T) {
	if got := flattenQueryParams(nil); got != nil {
		t.Fatalf("flattenQueryParams(nil) = %#v, want nil", got)
	}
	if got := flattenQueryParams(map[string]interface{}{}); got != nil {
		t.Fatalf("flattenQueryParams(empty) = %#v, want nil", got)
	}
}

func TestCompactMeetingPayload_DropsOnlyEmptySlices(t *testing.T) {
	got := compactMeetingPayload(map[string]interface{}{
		"empty_items": []interface{}{},
		"items":       []interface{}{"x"},
		"zero":        0,
		"text":        "ok",
	})

	if _, ok := got["empty_items"]; ok {
		t.Fatalf("compactMeetingPayload() should drop empty_items: %#v", got)
	}
	if !reflect.DeepEqual(got["items"], []interface{}{"x"}) {
		t.Fatalf("compactMeetingPayload() items = %#v, want %#v", got["items"], []interface{}{"x"})
	}
	if got["zero"] != 0 || got["text"] != "ok" {
		t.Fatalf("compactMeetingPayload() preserved fields mismatch: %#v", got)
	}
}

func TestCompactMeetingEvents_IgnoresNonMapsAndCompactsPayload(t *testing.T) {
	got := compactMeetingEvents([]interface{}{
		"skip-me",
		map[string]interface{}{
			"event_type": "chat_received",
			"payload": map[string]interface{}{
				"chat_received_items": []interface{}{"x"},
				"empty_items":         []interface{}{},
			},
		},
	})

	if len(got) != 1 {
		t.Fatalf("len(compactMeetingEvents()) = %d, want 1", len(got))
	}
	event, _ := got[0].(map[string]interface{})
	payload := common.GetMap(event, "payload")
	if _, ok := payload["empty_items"]; ok {
		t.Fatalf("compactMeetingEvents() should prune empty payload slices: %#v", payload)
	}
}

func TestCountdownChangedTimeline_KnownActions(t *testing.T) {
	event := countdownChangedEvent()
	got := meetingEventsEventFromPayload(event, meetingEventsIdentity{})
	if len(got.Actors) != 4 {
		t.Fatalf("actors = %#v, want the four operator-bearing countdown items", got.Actors)
	}
	if got.Actors[0].ID != "ou_201d63482ba649f81a7c9c67bd523b40" || got.Actors[0].Name != "用户204901" {
		t.Fatalf("first actor = %#v", got.Actors[0])
	}

	var sequence int
	entries := buildTimelineEntriesForEvent(event, &sequence)
	if len(entries) != 6 {
		t.Fatalf("timeline entries = %d, want 6: %#v", len(entries), entries)
	}
	want := []struct {
		subject     string
		description string
		details     []string
	}{
		{
			subject:     "用户204901(ou_201d63482ba649f81a7c9c67bd523b40)",
			description: "设置了倒计时",
			details:     []string{"时长：10分钟", "结束时间：2026-08-17 16:48:40", "seq_id：4"},
		},
		{
			subject:     "用户204901(ou_201d63482ba649f81a7c9c67bd523b40)",
			description: "延长了倒计时",
			details:     []string{"时长：11分钟", "结束时间：2026-08-17 16:49:40", "seq_id：5"},
		},
		{
			subject:     "用户204901(ou_201d63482ba649f81a7c9c67bd523b40)",
			description: "提前结束了倒计时",
			details:     []string{"seq_id：6"},
		},
		{
			subject:     "用户204901(ou_201d63482ba649f81a7c9c67bd523b40)",
			description: "关闭了倒计时窗口",
			details:     []string{"seq_id：7"},
		},
		{subject: "", description: "倒计时已结束", details: []string{"seq_id：8"}},
		{subject: "", description: "倒计时结束前提醒", details: []string{"剩余：5分钟", "seq_id：9"}},
	}
	for i := range want {
		if entries[i].subject != want[i].subject || entries[i].description != want[i].description {
			t.Fatalf("entry[%d] = %#v, want subject=%q description=%q", i, entries[i], want[i].subject, want[i].description)
		}
		if !entries[i].isAction {
			t.Fatalf("entry[%d] should use action formatting", i)
		}
		if !reflect.DeepEqual(entries[i].details, want[i].details) {
			t.Fatalf("entry[%d] details = %#v, want %#v", i, entries[i].details, want[i].details)
		}
		if !entries[i].hasWhen {
			t.Fatalf("entry[%d] should use item event_time", i)
		}
	}
}

func TestCountdownChangedPrettyDoesNotUseFallbackTopicSummary(t *testing.T) {
	timeline := buildMeetingEventTimeline([]interface{}{countdownChangedEvent()})
	out := renderMeetingEventsPretty(timeline)
	for _, want := range []string{
		"会议主题：用户204901的视频会议",
		"用户204901(ou_201d63482ba649f81a7c9c67bd523b40) 设置了倒计时",
		"时长：10分钟",
		"结束时间：2026-08-17 16:48:40",
		"用户204901(ou_201d63482ba649f81a7c9c67bd523b40) 延长了倒计时",
		"用户204901(ou_201d63482ba649f81a7c9c67bd523b40) 提前结束了倒计时",
		"用户204901(ou_201d63482ba649f81a7c9c67bd523b40) 关闭了倒计时窗口",
		"倒计时已结束",
		"倒计时结束前提醒",
		"剩余：5分钟",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("pretty output missing %q: %s", want, out)
		}
	}
	if strings.Contains(out, "countdown_changed: 用户204901的视频会议") {
		t.Fatalf("pretty output should not use fallback topic summary: %s", out)
	}
}

func TestDocumentContextChangedTimeline_KnownItems(t *testing.T) {
	tests := []struct {
		name            string
		item            map[string]interface{}
		wantActorID     string
		wantDescription string
		wantDetails     []string
	}{
		{
			name: "comment focus",
			item: map[string]interface{}{
				"time": "1776413281000",
				"operator": map[string]interface{}{
					"id":        "u-comment",
					"user_name": "Commenter",
					"user_type": 1,
					"user_role": 1,
				},
				"share_doc": map[string]interface{}{
					"title": "Design Doc",
					"url":   "https://example.com/docx/doc-token",
				},
				"comment_focus": map[string]interface{}{
					"comment_id": "comment-1",
					"focused":    true,
				},
			},
			wantActorID:     "u-comment",
			wantDescription: "聚焦评论 comment-1",
			wantDetails:     nil,
		},
		{
			name: "section location",
			item: map[string]interface{}{
				"time":     "1776413282000",
				"operator": map[string]interface{}{"id": "u-section", "user_name": "Navigator"},
				"section_location": map[string]interface{}{
					"parent_titles": []interface{}{" Roadmap ", "", "Phase 1"},
					"title":         " Milestone ",
					"level":         7,
				},
			},
			wantActorID:     "u-section",
			wantDescription: "定位到章节「Roadmap > Phase 1 > Milestone」",
			wantDetails:     []string{"层级：7"},
		},
		{
			name: "element preview",
			item: map[string]interface{}{
				"time":     "1776413283000",
				"operator": map[string]interface{}{"id": "u-preview", "user_name": "Viewer"},
				"element_preview": map[string]interface{}{
					"action":        "open",
					"element_type":  "image",
					"element_token": "image-token",
					"block_id":      "block-1",
				},
			},
			wantActorID:     "u-preview",
			wantDescription: "打开 image 预览 image-token",
			wantDetails:     []string{"block_id：block-1"},
		},
		{
			name: "comment focus without actor",
			item: map[string]interface{}{
				"comment_focus": map[string]interface{}{"comment_id": "comment-no-actor", "focused": true},
			},
			wantDescription: "聚焦评论 comment-no-actor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := documentContextChangedEvent([]interface{}{tt.item})
			got := meetingEventsEventFromPayload(event, meetingEventsIdentity{})
			if tt.wantActorID == "" {
				if len(got.Actors) != 0 {
					t.Fatalf("actors = %#v, want none", got.Actors)
				}
			} else if len(got.Actors) != 1 || got.Actors[0].ID != tt.wantActorID {
				t.Fatalf("actors = %#v, want actor %q", got.Actors, tt.wantActorID)
			}
			var sequence int
			entries := buildTimelineEntriesForEvent(event, &sequence)
			if len(entries) != 1 {
				t.Fatalf("timeline entries = %#v, want one", entries)
			}
			if entries[0].description != tt.wantDescription {
				t.Fatalf("timeline description = %q, want %q", entries[0].description, tt.wantDescription)
			}
			if !entries[0].isAction {
				t.Fatal("document context entry should keep its action formatting")
			}
			if !reflect.DeepEqual(entries[0].details, tt.wantDetails) {
				t.Fatalf("timeline details = %#v, want %#v", entries[0].details, tt.wantDetails)
			}
			if !entries[0].hasWhen {
				t.Fatal("timeline should use the item millisecond timestamp")
			}
		})
	}
}

func TestDocumentContextChangedTimeline_UsesExistingChronologicalOrder(t *testing.T) {
	event := documentContextChangedEvent([]interface{}{
		map[string]interface{}{
			"time":          "1776413400000",
			"comment_focus": map[string]interface{}{"comment_id": "first", "focused": true},
		},
		map[string]interface{}{
			"time":          "1776412800000",
			"comment_focus": map[string]interface{}{"comment_id": "second", "focused": true},
		},
	})
	early := map[string]interface{}{
		"event_type": "early_event", "event_time": "2026-04-17T08:07:00Z", "payload": map[string]interface{}{},
	}
	late := map[string]interface{}{
		"event_type": "late_event", "event_time": "2026-04-17T08:09:00Z", "payload": map[string]interface{}{},
	}
	timeline := buildMeetingEventTimeline([]interface{}{late, event, early})
	if len(timeline.entries) != 4 {
		t.Fatalf("timeline entries = %#v", timeline.entries)
	}
	want := []string{"聚焦评论 second", "early_event", "late_event", "聚焦评论 first"}
	for i := range want {
		if timeline.entries[i].description != want[i] {
			t.Fatalf("timeline order = %#v, want %#v", timeline.entries, want)
		}
	}
	if !timeline.entries[1].when.Before(timeline.entries[2].when) {
		t.Fatalf("timeline is not chronological: %#v", timeline.entries)
	}
}

func TestDocumentContextChangedTimeline_InvalidItemTimeFallsBackToEventTime(t *testing.T) {
	event := documentContextChangedEvent([]interface{}{
		map[string]interface{}{
			"time":          "not-a-time",
			"comment_focus": map[string]interface{}{"comment_id": "comment-1", "focused": true},
		},
	})
	var sequence int
	entries := buildTimelineEntriesForEvent(event, &sequence)
	if len(entries) != 1 {
		t.Fatalf("timeline entries = %#v", entries)
	}
	want, _ := time.Parse(time.RFC3339, "2026-04-17T08:08:00Z")
	if !entries[0].hasWhen || !entries[0].when.Equal(want) {
		t.Fatalf("timeline time = %v (has=%v), want event time %v", entries[0].when, entries[0].hasWhen, want)
	}
}

func TestDocumentContextChangedTimeline_MultipleItemsPreserveOrderAndSkipUnknown(t *testing.T) {
	event := documentContextChangedEvent([]interface{}{
		map[string]interface{}{
			"operator":      map[string]interface{}{"id": "u1", "user_name": "One"},
			"comment_focus": map[string]interface{}{"comment_id": "comment-1", "focused": false},
		},
		map[string]interface{}{
			"operator":         map[string]interface{}{"id": "u2", "user_name": "Two"},
			"section_location": map[string]interface{}{"parent_titles": []interface{}{"A"}, "title": "B", "level": 2},
		},
		map[string]interface{}{
			"operator":        map[string]interface{}{"id": "u3", "user_name": "Three"},
			"element_preview": map[string]interface{}{"action": "close", "element_type": "whiteboard", "element_token": "wb-1"},
		},
		map[string]interface{}{
			"operator":      map[string]interface{}{"id": "u4", "user_name": "Four"},
			"comment_focus": map[string]interface{}{"comment_id": "ambiguous", "focused": true},
			"element_preview": map[string]interface{}{
				"action": "open", "element_type": "image", "element_token": "ambiguous-image",
			},
		},
	})

	got := meetingEventsEventFromPayload(event, meetingEventsIdentity{})
	var actorIDs []string
	for _, actor := range got.Actors {
		actorIDs = append(actorIDs, actor.ID)
	}
	if want := []string{"u1", "u2", "u3", "u4"}; !reflect.DeepEqual(actorIDs, want) {
		t.Fatalf("actor order = %#v, want %#v", actorIDs, want)
	}

	var sequence int
	entries := buildTimelineEntriesForEvent(event, &sequence)
	if len(entries) != 3 {
		t.Fatalf("timeline entries = %d, want 3: %#v", len(entries), entries)
	}
	wantDescriptions := []string{
		"取消聚焦评论 comment-1",
		"定位到章节「A > B」",
		"关闭 whiteboard 预览 wb-1",
	}
	for i, want := range wantDescriptions {
		if entries[i].description != want {
			t.Fatalf("timeline[%d] description = %q, want %q", i, entries[i].description, want)
		}
	}
}

func TestCompactMeetingEvents_UsesSharedPayloadCompactionForDocumentContext(t *testing.T) {
	payload := map[string]interface{}{
		"activity_event_type": "document_context_changed",
		"document_context_changed_items": []interface{}{
			map[string]interface{}{
				"comment_focus":  map[string]interface{}{"comment_id": "comment-1", "focused": true},
				"unknown_nested": []interface{}{},
			},
		},
		"empty_unknown_items": []interface{}{},
		"future_field":        map[string]interface{}{"enabled": true},
	}
	want := map[string]interface{}{}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	delete(want, "empty_unknown_items")
	event := map[string]interface{}{
		"event_type": "document_context_changed",
		"payload":    payload,
	}

	compacted := compactMeetingEvents([]interface{}{event})
	gotEvent, _ := compacted[0].(map[string]interface{})
	if got := common.GetMap(gotEvent, "payload"); !reflect.DeepEqual(got, want) {
		t.Fatalf("document context payload changed:\ngot  %#v\nwant %#v", got, want)
	}
	projected := meetingEventsEventFromPayload(gotEvent, meetingEventsIdentity{})
	if !reflect.DeepEqual(projected.Payload, want) {
		t.Fatalf("projected payload changed:\ngot  %#v\nwant %#v", projected.Payload, want)
	}
}

func TestDocumentContextChangedTimeline_EmptyAndUnknownDoNotSynthesizeEntry(t *testing.T) {
	tests := []struct {
		name  string
		items []interface{}
	}{
		{name: "empty", items: []interface{}{}},
		{name: "unknown", items: []interface{}{map[string]interface{}{"future_context": map[string]interface{}{"id": "future-1"}}}},
		{name: "comment missing focused", items: []interface{}{map[string]interface{}{"comment_focus": map[string]interface{}{"comment_id": "comment-1"}}}},
		{name: "section missing path", items: []interface{}{map[string]interface{}{"section_location": map[string]interface{}{"level": 2}}}},
		{name: "preview missing type", items: []interface{}{map[string]interface{}{"element_preview": map[string]interface{}{"action": "open"}}}},
		{name: "preview unknown type", items: []interface{}{map[string]interface{}{"element_preview": map[string]interface{}{"action": "open", "element_type": "video"}}}},
		{name: "preview unknown action", items: []interface{}{map[string]interface{}{"element_preview": map[string]interface{}{"action": "zoom", "element_type": "image"}}}},
		{name: "non map", items: []interface{}{"bad-item"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := documentContextChangedEvent(tt.items)
			var sequence int
			entries := buildTimelineEntriesForEvent(event, &sequence)
			if len(entries) != 0 {
				t.Fatalf("timeline entries = %#v", entries)
			}
		})
	}
}

func TestDocumentContextChangedTimeline_MissingPayloadDoesNotSynthesizeEntry(t *testing.T) {
	event := map[string]interface{}{
		"event_id":   "event-without-payload",
		"event_type": "document_context_changed",
		"event_time": "2026-04-17T08:08:00Z",
	}
	got := meetingEventsEventFromPayload(event, meetingEventsIdentity{})
	if got.EventID != "event-without-payload" || got.Payload != nil {
		t.Fatalf("event = %#v", got)
	}
	var sequence int
	entries := buildTimelineEntriesForEvent(event, &sequence)
	if len(entries) != 0 {
		t.Fatalf("timeline entries = %#v", entries)
	}
}

func TestExistingMeetingEventEnvelope_OmitsDocumentContextFields(t *testing.T) {
	got := meetingEventsEventFromPayload(participantJoinedEvent(), meetingEventsIdentity{})
	row := meetingEventsEventRow(got)
	if _, ok := row["summary"]; ok {
		t.Fatalf("existing event row exposes summary: %#v", row)
	}
	if _, ok := row["section_path"]; ok {
		t.Fatalf("existing event row exposes section_path: %#v", row)
	}
	if _, ok := row["derived"]; ok {
		t.Fatalf("existing event row exposes an invented derived envelope: %#v", row)
	}
}

func TestVCMeetingEventsDocumentContextCLIBehavior(t *testing.T) {
	newEvent := func() map[string]interface{} {
		return documentContextChangedEvent([]interface{}{
			map[string]interface{}{
				"time":          "1776413281000",
				"operator":      map[string]interface{}{"id": "u1", "user_name": "Alice"},
				"share_id":      "share-session-1",
				"comment_focus": map[string]interface{}{"comment_id": "comment-1", "focused": true},
			},
			map[string]interface{}{
				"time":             "1776413282000",
				"operator":         map[string]interface{}{"id": "u2", "user_name": "Bob"},
				"share_id":         "share-session-1",
				"section_location": map[string]interface{}{"parent_titles": []interface{}{"Roadmap"}, "title": "Milestone", "level": 2},
			},
			map[string]interface{}{
				"time":            "1776413283000",
				"operator":        map[string]interface{}{"id": "u3", "user_name": "Carol"},
				"share_id":        "share-session-1",
				"element_preview": map[string]interface{}{"action": "open", "element_type": "whiteboard", "element_token": "wb-1"},
			},
		})
	}

	t.Run("json", func(t *testing.T) {
		f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
		event := newEvent()
		var wantPayload map[string]interface{}
		payloadJSON, err := json.Marshal(common.GetMap(event, "payload"))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(payloadJSON, &wantPayload); err != nil {
			t.Fatal(err)
		}
		reg.Register(meetingEventsStub([]interface{}{event}, false, ""))
		reg.Register(botInfoStub())
		err = mountAndRun(t, VCMeetingEvents, []string{
			"+meeting-events", "--meeting-id", "7628568141510692381", "--format", "json", "--as", "bot",
		}, f, stdout)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		reg.Verify(t)
		var envelope map[string]interface{}
		if err := json.Unmarshal([]byte(stdout.String()), &envelope); err != nil {
			t.Fatalf("unmarshal output: %v: %s", err, stdout.String())
		}
		gotEvent, _ := common.GetSlice(common.GetMap(envelope, "data"), "events")[0].(map[string]interface{})
		for _, key := range []string{"summary", "section_path", "derived"} {
			if _, exists := gotEvent[key]; exists {
				t.Fatalf("document context event exposes non-envelope field %q: %#v", key, gotEvent)
			}
		}
		if got := len(common.GetSlice(gotEvent, "actors")); got != 3 {
			t.Fatalf("actors = %d, want 3", got)
		}
		if got := common.GetMap(gotEvent, "payload"); !reflect.DeepEqual(got, wantPayload) {
			t.Fatalf("payload changed:\ngot  %#v\nwant %#v", got, wantPayload)
		}
	})

	t.Run("ndjson", func(t *testing.T) {
		f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
		reg.Register(meetingEventsStub([]interface{}{newEvent()}, false, ""))
		reg.Register(botInfoStub())
		err := mountAndRun(t, VCMeetingEvents, []string{
			"+meeting-events", "--meeting-id", "7628568141510692381", "--format", "ndjson", "--as", "bot",
		}, f, stdout)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		reg.Verify(t)
		lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
		if len(lines) != 2 {
			t.Fatalf("ndjson lines = %d, want event and metadata: %s", len(lines), stdout.String())
		}
		for _, want := range []string{
			`"event_type":"document_context_changed"`,
			`"document_context_changed_items"`,
		} {
			if !strings.Contains(lines[0], want) {
				t.Fatalf("ndjson event missing %q: %s", want, lines[0])
			}
		}
		var gotEvent map[string]interface{}
		if err := json.Unmarshal([]byte(lines[0]), &gotEvent); err != nil {
			t.Fatalf("decode ndjson event: %v", err)
		}
		for _, key := range []string{"summary", "section_path", "derived"} {
			if _, exists := gotEvent[key]; exists {
				t.Fatalf("document context ndjson exposes non-envelope field %q: %#v", key, gotEvent)
			}
		}
	})

	t.Run("pretty", func(t *testing.T) {
		f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
		reg.Register(meetingEventsStub([]interface{}{newEvent()}, false, ""))
		reg.Register(botInfoStub())
		err := mountAndRun(t, VCMeetingEvents, []string{
			"+meeting-events", "--meeting-id", "7628568141510692381", "--format", "pretty", "--as", "bot",
		}, f, stdout)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		reg.Verify(t)
		out := stdout.String()
		wantOrdered := []string{
			"Alice(u1) 聚焦评论 comment-1",
			"Bob(u2) 定位到章节「Roadmap > Milestone」",
			"Carol(u3) 打开 whiteboard 预览 wb-1",
		}
		last := -1
		for _, want := range wantOrdered {
			index := strings.Index(out, want)
			if index <= last {
				t.Fatalf("pretty output missing ordered %q: %s", want, out)
			}
			last = index
		}
	})
}

func TestVCShortcuts_RegistersMeetingAgentCommands(t *testing.T) {
	got := Shortcuts()
	var commands []string
	for _, shortcut := range got {
		commands = append(commands, shortcut.Command)
	}
	want := []string{"+search", "+notes", "+recording", "+detail", "+meeting-join", "+meeting-invite", "+meeting-end", "+meeting-leave", "+meeting-list-active", "+meeting-events", "+meeting-message-send", "+meeting-screenshot", "+meeting-countdown"}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("shortcut commands = %#v, want %#v", commands, want)
	}
}

func TestLeaveAction(t *testing.T) {
	tests := []struct {
		name string
		item map[string]interface{}
		want string
	}{
		{name: "meeting ended", item: map[string]interface{}{"leave_reason": leaveReasonMeetingEnded}, want: "因会议结束离开了会议"},
		{name: "kicked", item: map[string]interface{}{"leave_reason": leaveReasonKicked}, want: "被移出了会议"},
		{name: "default", item: map[string]interface{}{"leave_reason": leaveReasonUserLeft}, want: "离开了会议"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := leaveAction(tt.item); got != tt.want {
				t.Fatalf("leaveAction() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMeetingEventUserWithID(t *testing.T) {
	tests := []struct {
		name string
		user map[string]interface{}
		want string
	}{
		{name: "nil", want: ""},
		{name: "name and id", user: map[string]interface{}{"user_name": "Alice", "id": "u1"}, want: "Alice(u1)"},
		{name: "name only", user: map[string]interface{}{"user_name": "Alice"}, want: "Alice"},
		{name: "id only", user: map[string]interface{}{"id": "u1"}, want: "u1"},
		{name: "empty", user: map[string]interface{}{}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := meetingEventUserWithID(tt.user); got != tt.want {
				t.Fatalf("meetingEventUserWithID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMeetingEventsIdentityFromParticipant_UsesContractFields(t *testing.T) {
	got := meetingEventsIdentityFromParticipant(map[string]interface{}{
		"id":        "u1",
		"user_name": "Alice",
		"user_type": 1,
		"user_role": 2,
	}, meetingEventsIdentity{})

	if got.ParticipantType != "human" || got.Role != "host" {
		t.Fatalf("identity = %#v, want participant_type=human role=host", got)
	}
}

func TestMeetingEventsIdentityFromParticipant_UserRoleParticipant(t *testing.T) {
	got := meetingEventsIdentityFromParticipant(map[string]interface{}{
		"id":        "u1",
		"user_name": "Alice",
		"user_type": 1,
		"user_role": 1,
	}, meetingEventsIdentity{})

	if got.Role != "participant" {
		t.Fatalf("identity = %#v, want role=participant", got)
	}
}

func TestMeetingEventsIdentityFromParticipant_UserTypeApp(t *testing.T) {
	got := meetingEventsIdentityFromParticipant(map[string]interface{}{
		"id":        "ou_app",
		"user_name": "Demo Bot",
		"user_type": 10,
		"user_role": 1,
	}, meetingEventsIdentity{})

	if got.ParticipantType != "bot" {
		t.Fatalf("identity = %#v, want participant_type=bot", got)
	}
}

func TestMeetingEventsIdentityFromParticipant_UnknownUserType(t *testing.T) {
	got := meetingEventsIdentityFromParticipant(map[string]interface{}{
		"id":        "u_unknown",
		"user_name": "Unknown",
		"user_type": 0,
		"user_role": 1,
	}, meetingEventsIdentity{})

	if got.ParticipantType != "unknown" {
		t.Fatalf("identity = %#v, want participant_type=unknown", got)
	}
}

func TestMeetingEventsIdentityFromParticipant_IgnoresGenericTypeField(t *testing.T) {
	got := meetingEventsIdentityFromParticipant(map[string]interface{}{
		"id":        "u1",
		"user_name": "Alice",
		"type":      "bot",
	}, meetingEventsIdentity{})

	if got.ParticipantType != "human" {
		t.Fatalf("identity = %#v, generic type field should not drive participant_type", got)
	}
}

func TestMeetingEventSummary(t *testing.T) {
	tests := []struct {
		name  string
		event map[string]interface{}
		want  string
	}{
		{
			name: "participant joined count",
			event: map[string]interface{}{
				"event_type": "participant_joined",
				"payload": map[string]interface{}{
					"participant_joined_items": []interface{}{
						map[string]interface{}{},
						map[string]interface{}{},
					},
				},
			},
			want: "2 participants joined",
		},
		{
			name: "participant left with label",
			event: map[string]interface{}{
				"event_type": "participant_left",
				"payload": map[string]interface{}{
					"participant_left_items": []interface{}{
						map[string]interface{}{"participant": map[string]interface{}{"user_name": "Bob", "id": "u2"}},
					},
				},
			},
			want: "participant u2 (Bob) left",
		},
		{
			name: "fallback unknown event",
			event: map[string]interface{}{
				"event_type": "mystery_event",
				"payload":    map[string]interface{}{},
			},
			want: "mystery_event",
		},
		{
			name:  "countdown changed",
			event: countdownChangedEvent(),
			want:  "6 countdown changes: set, prolonged, ended in advance, window closed, ended, reminder",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := meetingEventSummary(tt.event); got != tt.want {
				t.Fatalf("meetingEventSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMeetingEventsEventFromPayloadUsesActivityEventTypeFallback(t *testing.T) {
	event := participantJoinedEvent()
	delete(event, "event_type")

	got := meetingEventsEventFromPayload(event, meetingEventsIdentity{})
	if got.EventType != "participant_joined" {
		t.Fatalf("EventType = %q, want participant_joined", got.EventType)
	}
	if len(got.Actors) != 1 {
		t.Fatalf("actors len = %d, want 1: %#v", len(got.Actors), got.Actors)
	}
	if got.Actors[0].ID != "bot_001" {
		t.Fatalf("actor id = %q, want bot_001", got.Actors[0].ID)
	}
}

func TestEscapePrettyText(t *testing.T) {
	got := escapePrettyText("line1\nline2\t\r" + string(rune(0x07)))
	want := `line1\nline2\t\r\u0007`
	if got != want {
		t.Fatalf("escapePrettyText() = %q, want %q", got, want)
	}
}

func TestNeedsColon(t *testing.T) {
	tests := []struct {
		description string
		want        bool
	}{
		{description: "发送了消息", want: false},
		{description: "加入了会议", want: false},
		{description: "离开了会议", want: false},
		{description: "开始共享「文档」", want: false},
		{description: "聚焦评论 comment-1", want: true},
		{description: "定位到章节「A > B」", want: true},
		{description: "打开 image 预览 token", want: true},
		{description: "[text] hello", want: true},
	}
	for _, tt := range tests {
		if got := needsColon(tt.description); got != tt.want {
			t.Fatalf("needsColon(%q) = %v, want %v", tt.description, got, tt.want)
		}
	}
}

func TestRenderMeetingEventsPretty_DoesNotInferActionFromTranscriptText(t *testing.T) {
	sequence := 0
	timeline := meetingTimeline{entries: []meetingTimelineEntry{
		newTimelineEntry(time.Time{}, false, &sequence, "Alice", "打开 image 预览不是操作事件", nil),
		newTimelineActionEntry(time.Time{}, false, &sequence, "Bob", "打开 image 预览 token", nil),
	}}

	got := renderMeetingEventsPretty(timeline)
	if !strings.Contains(got, "Alice: 打开 image 预览不是操作事件") {
		t.Fatalf("transcript-like text should retain colon: %s", got)
	}
	if !strings.Contains(got, "Bob 打开 image 预览 token") {
		t.Fatalf("document action should omit colon: %s", got)
	}
}

// ---------------------------------------------------------------------------
// Typed error lock assertions
// ---------------------------------------------------------------------------

func TestMeetingEvents_Validation_InvalidMeetingID_TypedError(t *testing.T) {
	runtime := newMeetingEventsRuntime()
	mustSetMeetingEventsFlag(t, runtime, "meeting-id", "not-a-number")

	err := VCMeetingEvents.Validate(context.Background(), runtime)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "positive integer") {
		t.Errorf("message mismatch: %v", err)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("Subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
	}
	if ve.Param != "--meeting-id" {
		t.Errorf("Param = %q, want %q", ve.Param, "--meeting-id")
	}
}

func TestMeetingEvents_Validation_InvalidPageSize_TypedError(t *testing.T) {
	runtime := newMeetingEventsRuntime()
	mustSetMeetingEventsFlag(t, runtime, "meeting-id", "7628568141510692381")
	mustSetMeetingEventsFlag(t, runtime, "page-size", "foo")

	err := VCMeetingEvents.Validate(context.Background(), runtime)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "invalid --page-size") {
		t.Errorf("message mismatch: %v", err)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("Subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
	}
	if ve.Param != "--page-size" {
		t.Errorf("Param = %q, want %q", ve.Param, "--page-size")
	}
}

func TestMeetingEvents_Validation_StartAfterEnd_TypedError(t *testing.T) {
	runtime := newMeetingEventsRuntime()
	mustSetMeetingEventsFlag(t, runtime, "meeting-id", "7628568141510692381")
	mustSetMeetingEventsFlag(t, runtime, "start", "200")
	mustSetMeetingEventsFlag(t, runtime, "end", "100")

	err := VCMeetingEvents.Validate(context.Background(), runtime)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "after --end") {
		t.Errorf("message mismatch: %v", err)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("Subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
	}
	if ve.Param != "--start" {
		t.Errorf("Param = %q, want %q", ve.Param, "--start")
	}
}
