// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
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
					"message_type": 3,
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
					"message_type": 3,
					"send_time":    "1776408061000",
					"operator": map[string]interface{}{
						"id":        "u1",
						"user_name": "Alice",
					},
				},
				map[string]interface{}{
					"content":      "第二条",
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
	if got := params["meeting_id"][0]; got != "7628568141510692381" {
		t.Fatalf("meeting_id = %q, want %q", got, "7628568141510692381")
	}
	if got := params["page_size"][0]; got != "40" {
		t.Fatalf("page_size = %q, want %q", got, "40")
	}
	if got := params["page_token"][0]; got != "1710000000000000000" {
		t.Fatalf("page_token = %q, want %q", got, "1710000000000000000")
	}
	if got := params["start_time"][0]; got != "1710000000" {
		t.Fatalf("start_time = %q, want %q", got, "1710000000")
	}
	if got := params["end_time"][0]; got != "1710003600" {
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
	if got := params["page_size"][0]; got != "20" {
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
	if got := params["page_size"][0]; got != "100" {
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
	if got := params["page_size"][0]; got != "100" {
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
		"--as", "user",
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
		"--as", "user",
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

	err := mountAndRun(t, VCMeetingEvents, []string{
		"+meeting-events",
		"--meeting-id", "7628568141510692381",
		"--format", "json",
		"--page-all",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)

	out := strings.ReplaceAll(stdout.String(), " ", "")
	out = strings.ReplaceAll(out, "\n", "")
	if count := strings.Count(out, `"event_type":"participant_joined"`); count != 2 {
		t.Fatalf("expected 2 aggregated events, got %d: %s", count, stdout.String())
	}
	if !strings.Contains(out, `"has_more":false`) {
		t.Fatalf("expected final has_more=false: %s", stdout.String())
	}
}

func TestMeetingEvents_ExecuteJSON(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(meetingEventsStub([]interface{}{participantJoinedEvent()}, true, "1710000000000000000"))

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
		`"event_type":"participant_joined"`,
		`"has_more":true`,
		`"page_token":"1710000000000000000"`,
		`"events":[`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("json output missing %q: %s", want, stdout.String())
		}
	}
}

func TestMeetingEvents_ExecuteJSON_PrunesEmptySlices(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(meetingEventsStub([]interface{}{chatReceivedEvent()}, false, ""))

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
	if !strings.Contains(out, `"message_type": 3`) {
		t.Fatalf("json output should keep numeric fields: %s", out)
	}
}

func TestMeetingEvents_ExecutePretty(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(meetingEventsStub([]interface{}{participantJoinedEventOngoing(), multiChatReceivedEvent(), magicShareStartedEvent()}, true, "1710000000000000000"))

	err := mountAndRun(t, VCMeetingEvents, []string{
		"+meeting-events",
		"--meeting-id", "7628568141510692381",
		"--format", "pretty",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)

	out := stdout.String()
	for _, want := range []string{
		"会议主题：项目例会",
		"会议时间：2026-04-17 15:15:00（进行中）",
		"Demo Bot(bot_001) 加入了会议",
		"Alice(u1): [reaction] 第一条\\n第二行",
		"Alice(u1): [reaction] 第二条",
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

	err := mountAndRun(t, VCMeetingEvents, []string{
		"+meeting-events",
		"--meeting-id", "7628568141510692381",
		"--format", "pretty",
		"--as", "user",
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

	err := mountAndRun(t, VCMeetingEvents, []string{
		"+meeting-events",
		"--meeting-id", "7628568141510692381",
		"--format", "pretty",
		"--as", "user",
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
	params := larkcore.QueryParams{
		"one":   []string{"1"},
		"many":  []string{"2", "3"},
		"empty": []string{},
	}

	got := flattenQueryParams(params)
	want := map[string]interface{}{
		"one":  "1",
		"many": []string{"2", "3"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("flattenQueryParams() = %#v, want %#v", got, want)
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

func TestVCShortcuts_RegistersMeetingAgentCommands(t *testing.T) {
	got := Shortcuts()
	var commands []string
	for _, shortcut := range got {
		commands = append(commands, shortcut.Command)
	}
	want := []string{"+search", "+notes", "+recording", "+meeting-join", "+meeting-leave", "+meeting-events"}
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
		{name: "meeting ended", item: map[string]interface{}{"leave_reason": 2}, want: "因会议结束离开了会议"},
		{name: "kicked", item: map[string]interface{}{"leave_reason": 3}, want: "被移出了会议"},
		{name: "default", item: map[string]interface{}{"leave_reason": 1}, want: "离开了会议"},
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := meetingEventSummary(tt.event); got != tt.want {
				t.Fatalf("meetingEventSummary() = %q, want %q", got, tt.want)
			}
		})
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
		{description: "[text] hello", want: true},
	}
	for _, tt := range tests {
		if got := needsColon(tt.description); got != tt.want {
			t.Fatalf("needsColon(%q) = %v, want %v", tt.description, got, tt.want)
		}
	}
}
