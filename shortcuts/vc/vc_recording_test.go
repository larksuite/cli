// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// ---------------------------------------------------------------------------
// Unit tests: extractMinuteToken
// ---------------------------------------------------------------------------

func TestExtractMinuteToken(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"standard feishu URL", "https://meetings.feishu.cn/minutes/obcn37dxcftoc3656rgyejm7", "obcn37dxcftoc3656rgyejm7"},
		{"larksuite URL", "https://meetings.larksuite.com/minutes/obcn12345678", "obcn12345678"},
		{"trailing slash", "https://meetings.feishu.cn/minutes/obcntoken123/", "obcntoken123"},
		{"with query params", "https://meetings.feishu.cn/minutes/obcntoken123?from=share", "obcntoken123"},
		{"with fragment", "https://meetings.feishu.cn/minutes/obcntoken123#section", "obcntoken123"},
		{"empty URL", "", ""},
		{"no minutes path", "https://meetings.feishu.cn/other/path", ""},
		{"only domain", "https://meetings.feishu.cn", ""},
		{"minutes at end with no token", "https://meetings.feishu.cn/minutes", ""},
		{"minutes trailing slash only", "https://meetings.feishu.cn/minutes/", ""},
		{"invalid URL", "://invalid", ""},
		{"nested path after token", "https://meetings.feishu.cn/minutes/obcntoken123/extra/path", "obcntoken123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractMinuteToken(tt.url)
			if got != tt.want {
				t.Errorf("extractMinuteToken(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Validation tests
// ---------------------------------------------------------------------------

func TestRecording_Validation_ExactlyOne(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())

	// 没传任何 flag
	err := mountAndRun(t, VCRecording, []string{"+recording", "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for no flags")
	}

	// 两个 flag 都传了
	err = mountAndRun(t, VCRecording, []string{"+recording", "--meeting-ids", "m1", "--calendar-event-ids", "e1", "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for two flags")
	}
}

func TestRecording_BatchLimit_MeetingIDs(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	ids := make([]string, 51)
	for i := range ids {
		ids[i] = fmt.Sprintf("m%d", i)
	}
	err := mountAndRun(t, VCRecording, []string{"+recording", "--meeting-ids", strings.Join(ids, ","), "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected batch limit error")
	}
	if !strings.Contains(err.Error(), "too many IDs") {
		t.Errorf("expected 'too many IDs' error, got: %v", err)
	}
}

func TestRecording_BatchLimit_CalendarEventIDs(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	ids := make([]string, 51)
	for i := range ids {
		ids[i] = fmt.Sprintf("e%d", i)
	}
	err := mountAndRun(t, VCRecording, []string{"+recording", "--calendar-event-ids", strings.Join(ids, ","), "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected batch limit error")
	}
	if !strings.Contains(err.Error(), "too many IDs") {
		t.Errorf("expected 'too many IDs' error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// DryRun tests
// ---------------------------------------------------------------------------

func TestRecording_DryRun_MeetingIDs(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCRecording, []string{"+recording", "--meeting-ids", "m001", "--dry-run", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "recording") {
		t.Errorf("dry-run should show recording API, got: %s", out)
	}
	if !strings.Contains(out, "minute_token") {
		t.Errorf("dry-run should mention minute_token, got: %s", out)
	}
}

func TestRecording_DryRun_CalendarEventIDs(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCRecording, []string{"+recording", "--calendar-event-ids", "evt001", "--dry-run", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "mget_instance_relation_info") {
		t.Errorf("dry-run should show mget step, got: %s", out)
	}
	if !strings.Contains(out, "recording") {
		t.Errorf("dry-run should show recording step, got: %s", out)
	}
}

func TestRecording_DryRun_BatchIDs(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCRecording, []string{"+recording", "--meeting-ids", "m001,m002,m003", "--dry-run", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "m001") || !strings.Contains(out, "m002") || !strings.Contains(out, "m003") {
		t.Errorf("dry-run should list all meeting IDs, got: %s", out)
	}
}

// ---------------------------------------------------------------------------
// Unit tests: fetchRecordingByMeetingID via bot shortcut wrapper
// ---------------------------------------------------------------------------

func TestFetchRecordingByMeetingID_Success(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/meetings/m001/recording",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"recording": map[string]interface{}{
					"url":      "https://meetings.feishu.cn/minutes/obcntoken123",
					"duration": "30000",
				},
			},
		},
	})

	s := common.Shortcut{
		Service:   "test",
		Command:   "+fetch-recording",
		AuthTypes: []string{"bot"},
		Execute: func(_ context.Context, rctx *common.RuntimeContext) error {
			result := fetchRecordingByMeetingID(context.Background(), rctx, "m001")
			if result["error"] != nil {
				t.Errorf("unexpected error: %v", result["error"])
			}
			if result["minute_token"] != "obcntoken123" {
				t.Errorf("minute_token = %v, want obcntoken123", result["minute_token"])
			}
			if result["duration"] != "30000" {
				t.Errorf("duration = %v, want 30000", result["duration"])
			}
			if result["meeting_id"] != "m001" {
				t.Errorf("meeting_id = %v, want m001", result["meeting_id"])
			}
			return nil
		},
	}

	parent := &cobra.Command{Use: "vc"}
	s.Mount(parent, f)
	parent.SetArgs([]string{"+fetch-recording"})
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if err := parent.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFetchRecordingByMeetingID_NoRecording(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/meetings/m002/recording",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{},
		},
	})

	s := common.Shortcut{
		Service:   "test",
		Command:   "+fetch-no-recording",
		AuthTypes: []string{"bot"},
		Execute: func(_ context.Context, rctx *common.RuntimeContext) error {
			result := fetchRecordingByMeetingID(context.Background(), rctx, "m002")
			errMsg, _ := result["error"].(string)
			if errMsg == "" {
				t.Error("expected error for missing recording")
			}
			if !strings.Contains(errMsg, "no recording") {
				t.Errorf("error should mention no recording, got: %s", errMsg)
			}
			return nil
		},
	}

	parent := &cobra.Command{Use: "vc"}
	s.Mount(parent, f)
	parent.SetArgs([]string{"+fetch-no-recording"})
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if err := parent.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFetchRecordingByMeetingID_APIError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/meetings/m003/recording",
		Body: map[string]interface{}{
			"code": 121004, "msg": "data not found",
		},
	})

	s := common.Shortcut{
		Service:   "test",
		Command:   "+fetch-api-error",
		AuthTypes: []string{"bot"},
		Execute: func(_ context.Context, rctx *common.RuntimeContext) error {
			result := fetchRecordingByMeetingID(context.Background(), rctx, "m003")
			errMsg, _ := result["error"].(string)
			if errMsg == "" {
				t.Error("expected error for API failure")
			}
			if !strings.Contains(errMsg, "failed to query recording") {
				t.Errorf("error should mention query failure, got: %s", errMsg)
			}
			return nil
		},
	}

	parent := &cobra.Command{Use: "vc"}
	s.Mount(parent, f)
	parent.SetArgs([]string{"+fetch-api-error"})
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if err := parent.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFetchRecordingByMeetingID_URLWithoutMinuteToken(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/meetings/m004/recording",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"recording": map[string]interface{}{
					"url":      "https://example.com/some/other/path",
					"duration": "5000",
				},
			},
		},
	})

	s := common.Shortcut{
		Service:   "test",
		Command:   "+fetch-no-token",
		AuthTypes: []string{"bot"},
		Execute: func(_ context.Context, rctx *common.RuntimeContext) error {
			result := fetchRecordingByMeetingID(context.Background(), rctx, "m004")
			if result["error"] != nil {
				t.Errorf("should not error even without minute_token: %v", result["error"])
			}
			if _, exists := result["minute_token"]; exists {
				t.Error("should not have minute_token for non-standard URL")
			}
			if result["recording_url"] != "https://example.com/some/other/path" {
				t.Errorf("recording_url = %v, want the original URL", result["recording_url"])
			}
			return nil
		},
	}

	parent := &cobra.Command{Use: "vc"}
	s.Mount(parent, f)
	parent.SetArgs([]string{"+fetch-no-token"})
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if err := parent.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Unit tests: resolveMeetingIDsFromCalendarEvent
// ---------------------------------------------------------------------------

func TestResolveMeetingIDs_TypeCoercion(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/cal_001/events/mget_instance_relation_info",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"instance_relation_infos": []interface{}{
					map[string]interface{}{
						"meeting_instance_ids": []interface{}{
							float64(12345678),
							"string_id",
							nil,
						},
					},
				},
			},
		},
	})

	s := common.Shortcut{
		Service:   "test",
		Command:   "+resolve-test",
		AuthTypes: []string{"bot"},
		Execute: func(_ context.Context, rctx *common.RuntimeContext) error {
			relInfo, err := resolveMeetingIDsFromCalendarEvent(rctx, "evt_001", "cal_001", false)
			if err != nil {
				return err
			}
			ids := relInfo.MeetingIDs
			if len(ids) != 2 {
				t.Errorf("expected 2 IDs (nil skipped), got %d: %v", len(ids), ids)
			}
			if len(ids) > 0 && ids[0] != "12345678" {
				t.Errorf("expected float64 coerced to string, got %q", ids[0])
			}
			if len(ids) > 1 && ids[1] != "string_id" {
				t.Errorf("expected string preserved, got %q", ids[1])
			}
			return nil
		},
	}

	parent := &cobra.Command{Use: "vc"}
	s.Mount(parent, f)
	parent.SetArgs([]string{"+resolve-test"})
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if err := parent.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveMeetingIDs_NoMeetings(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/cal_001/events/mget_instance_relation_info",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"instance_relation_infos": []interface{}{
					map[string]interface{}{
						"meeting_instance_ids": []interface{}{},
					},
				},
			},
		},
	})

	s := common.Shortcut{
		Service:   "test",
		Command:   "+resolve-no-meetings",
		AuthTypes: []string{"bot"},
		Execute: func(_ context.Context, rctx *common.RuntimeContext) error {
			_, err := resolveMeetingIDsFromCalendarEvent(rctx, "evt_001", "cal_001", false)
			if err == nil {
				t.Error("expected error for no meetings")
			}
			if !strings.Contains(err.Error(), "no associated video meeting") {
				t.Errorf("error should mention no meeting, got: %v", err)
			}
			return nil
		},
	}

	parent := &cobra.Command{Use: "vc"}
	s.Mount(parent, f)
	parent.SetArgs([]string{"+resolve-no-meetings"})
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if err := parent.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveMeetingIDs_NoRelationInfo(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/cal_001/events/mget_instance_relation_info",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"instance_relation_infos": []interface{}{},
			},
		},
	})

	s := common.Shortcut{
		Service:   "test",
		Command:   "+resolve-no-info",
		AuthTypes: []string{"bot"},
		Execute: func(_ context.Context, rctx *common.RuntimeContext) error {
			_, err := resolveMeetingIDsFromCalendarEvent(rctx, "evt_001", "cal_001", false)
			if err == nil {
				t.Error("expected error for no relation info")
			}
			if !strings.Contains(err.Error(), "no event relation info found") {
				t.Errorf("error should mention no info, got: %v", err)
			}
			return nil
		},
	}

	parent := &cobra.Command{Use: "vc"}
	s.Mount(parent, f)
	parent.SetArgs([]string{"+resolve-no-info"})
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if err := parent.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Integration tests: Execute path via bot wrapper
// ---------------------------------------------------------------------------

// botExec runs a function within a bot shortcut context, reusing the httpmock registry.
func botExec(t *testing.T, name string, f *cmdutil.Factory, fn func(context.Context, *common.RuntimeContext) error) error {
	t.Helper()
	warmTokenCache(t)
	s := common.Shortcut{
		Service:   "test",
		Command:   "+" + name,
		AuthTypes: []string{"bot"},
		HasFormat: true,
		Execute:   fn,
	}
	parent := &cobra.Command{Use: "vc"}
	s.Mount(parent, f)
	parent.SetArgs([]string{"+" + name, "--format", "json"})
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	return parent.Execute()
}

func TestRecording_Execute_MeetingIDs_PartialFailure(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	// m001 succeeds, m002 fails (API error)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/meetings/m001/recording",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"recording": map[string]interface{}{
					"url":      "https://meetings.feishu.cn/minutes/obcnpartial1",
					"duration": "10000",
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/meetings/m002/recording",
		Body:   map[string]interface{}{"code": 121004, "msg": "data not found"},
	})

	err := botExec(t, "partial-fail", f, func(ctx context.Context, rctx *common.RuntimeContext) error {
		r1 := fetchRecordingByMeetingID(ctx, rctx, "m001")
		r2 := fetchRecordingByMeetingID(ctx, rctx, "m002")

		if r1["error"] != nil {
			t.Errorf("m001 should succeed, got error: %v", r1["error"])
		}
		if r1["minute_token"] != "obcnpartial1" {
			t.Errorf("m001 minute_token = %v, want obcnpartial1", r1["minute_token"])
		}
		if r2["error"] == nil {
			t.Error("m002 should fail")
		}

		// verify counting logic
		results := []any{r1, r2}
		successCount := 0
		for _, r := range results {
			m, _ := r.(map[string]any)
			if m["error"] == nil {
				successCount++
			}
		}
		if successCount != 1 {
			t.Errorf("expected 1 success, got %d", successCount)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecording_Execute_CalendarPath_ResolveAndFetch(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/cal_001/events/mget_instance_relation_info",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"instance_relation_infos": []interface{}{
					map[string]interface{}{
						"meeting_instance_ids": []interface{}{"m001"},
					},
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/meetings/m001/recording",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"recording": map[string]interface{}{
					"url":      "https://meetings.feishu.cn/minutes/obcnfromcal",
					"duration": "60000",
				},
			},
		},
	})

	err := botExec(t, "cal-resolve", f, func(ctx context.Context, rctx *common.RuntimeContext) error {
		relInfo, resolveErr := resolveMeetingIDsFromCalendarEvent(rctx, "evt_001", "cal_001", false)
		if resolveErr != nil {
			t.Fatalf("resolve failed: %v", resolveErr)
		}
		if len(relInfo.MeetingIDs) != 1 || relInfo.MeetingIDs[0] != "m001" {
			t.Fatalf("expected [m001], got %v", relInfo.MeetingIDs)
		}

		result := fetchRecordingByMeetingID(ctx, rctx, relInfo.MeetingIDs[0])
		if result["error"] != nil {
			t.Errorf("fetch should succeed, got: %v", result["error"])
		}
		if result["minute_token"] != "obcnfromcal" {
			t.Errorf("minute_token = %v, want obcnfromcal", result["minute_token"])
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecording_Execute_CalendarPath_MultiMeetingFallback(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	// calendar resolve returns two meetings: m001 (no recording) and m002 (has recording)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/cal_001/events/mget_instance_relation_info",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"instance_relation_infos": []interface{}{
					map[string]interface{}{
						"meeting_instance_ids": []interface{}{"m001", "m002"},
					},
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/meetings/m001/recording",
		Body:   map[string]interface{}{"code": 121004, "msg": "data not found"},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/meetings/m002/recording",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"recording": map[string]interface{}{
					"url":      "https://meetings.feishu.cn/minutes/obcnfallback",
					"duration": "45000",
				},
			},
		},
	})

	err := botExec(t, "cal-fallback", f, func(ctx context.Context, rctx *common.RuntimeContext) error {
		relInfo, resolveErr := resolveMeetingIDsFromCalendarEvent(rctx, "evt_001", "cal_001", false)
		if resolveErr != nil {
			t.Fatalf("resolve failed: %v", resolveErr)
		}
		if len(relInfo.MeetingIDs) != 2 {
			t.Fatalf("expected 2 meeting IDs, got %d", len(relInfo.MeetingIDs))
		}

		// simulate fallback: try each until success
		var found bool
		for _, meetingID := range relInfo.MeetingIDs {
			result := fetchRecordingByMeetingID(ctx, rctx, meetingID)
			if result["error"] == nil {
				if result["minute_token"] != "obcnfallback" {
					t.Errorf("minute_token = %v, want obcnfallback", result["minute_token"])
				}
				found = true
				break
			}
		}
		if !found {
			t.Error("expected fallback to succeed on m002")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecording_Execute_AllFailed_ErrorMessage(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/meetings/m001/recording",
		Body:   map[string]interface{}{"code": 121004, "msg": "data not found"},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/meetings/m002/recording",
		Body:   map[string]interface{}{"code": 121005, "msg": "no permission"},
	})

	err := botExec(t, "all-fail", f, func(ctx context.Context, rctx *common.RuntimeContext) error {
		r1 := fetchRecordingByMeetingID(ctx, rctx, "m001")
		r2 := fetchRecordingByMeetingID(ctx, rctx, "m002")

		if r1["error"] == nil || r2["error"] == nil {
			t.Error("both should fail")
		}
		e1, _ := r1["error"].(string)
		e2, _ := r2["error"].(string)
		if !strings.Contains(e1, "data not found") {
			t.Errorf("m001 error should contain API message, got: %s", e1)
		}
		if !strings.Contains(e2, "no permission") {
			t.Errorf("m002 error should contain API message, got: %s", e2)
		}
		if r1["meeting_id"] != "m001" {
			t.Errorf("error result should preserve meeting_id, got: %v", r1["meeting_id"])
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecording_Execute_EmptyURL(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/meetings/m001/recording",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"recording": map[string]interface{}{
					"url":      "",
					"duration": "1000",
				},
			},
		},
	})

	err := botExec(t, "empty-url", f, func(ctx context.Context, rctx *common.RuntimeContext) error {
		result := fetchRecordingByMeetingID(ctx, rctx, "m001")
		if result["error"] != nil {
			t.Errorf("empty URL should not cause error: %v", result["error"])
		}
		if _, exists := result["minute_token"]; exists {
			t.Error("empty URL should not produce minute_token")
		}
		if _, exists := result["recording_url"]; exists {
			t.Error("empty URL should not produce recording_url")
		}
		if result["duration"] != "1000" {
			t.Errorf("duration should be preserved, got: %v", result["duration"])
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecording_Execute_RecordingGenerating(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/meetings/m001/recording",
		Body:   map[string]interface{}{"code": 124002, "msg": "recording generating"},
	})

	err := botExec(t, "generating", f, func(ctx context.Context, rctx *common.RuntimeContext) error {
		result := fetchRecordingByMeetingID(ctx, rctx, "m001")
		errMsg, _ := result["error"].(string)
		if errMsg == "" {
			t.Error("should return error for generating recording")
		}
		if !strings.Contains(errMsg, "recording generating") {
			t.Errorf("error should mention recording generating, got: %s", errMsg)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
