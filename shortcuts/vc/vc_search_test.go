// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
)

// ---------------------------------------------------------------------------
// Unit tests for pure functions
// ---------------------------------------------------------------------------

func TestUniqueIDs(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{"nil", nil, nil},
		{"empty", []string{}, nil},
		{"no dups", []string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{"with dups", []string{"a", "b", "a", "c", "b"}, []string{"a", "b", "c"}},
		{"single", []string{"x"}, []string{"x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := uniqueIDs(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("uniqueIDs(%v) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("uniqueIDs(%v)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseTimeRange(t *testing.T) {
	cfg := &core.CliConfig{AppID: "test", AppSecret: "s", Brand: core.BrandFeishu}
	tests := []struct {
		name    string
		start   string
		end     string
		wantErr bool
	}{
		{"both empty", "", "", false},
		{"valid start", "2026-03-24T00:00+08:00", "", false},
		{"valid end", "", "2026-03-24T23:59+08:00", false},
		{"both valid", "2026-03-24T00:00+08:00", "2026-03-24T23:59+08:00", false},
		{"invalid start", "not-a-date", "", true},
		{"invalid end", "", "not-a-date", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().String("start", "", "")
			cmd.Flags().String("end", "", "")
			if tt.start != "" {
				cmd.Flags().Set("start", tt.start)
			}
			if tt.end != "" {
				cmd.Flags().Set("end", tt.end)
			}
			runtime := common.TestNewRuntimeContext(cmd, cfg)
			_, _, err := parseTimeRange(runtime)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseTimeRange() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBuildSearchBody(t *testing.T) {
	cfg := &core.CliConfig{AppID: "test", AppSecret: "s", Brand: core.BrandFeishu}
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("query", "", "")
	cmd.Flags().String("start", "", "")
	cmd.Flags().String("end", "", "")
	cmd.Flags().String("organizer-ids", "", "")
	cmd.Flags().String("participant-ids", "", "")
	cmd.Flags().String("room-ids", "", "")
	cmd.Flags().Set("query", "weekly")
	cmd.Flags().Set("organizer-ids", "ou_a,ou_b")
	cmd.Flags().Set("room-ids", "room1")

	runtime := common.TestNewRuntimeContext(cmd, cfg)
	body := buildSearchBody(runtime, "2026-03-20T00:00:00Z", "2026-03-24T00:00:00Z")

	if body["query"] != "weekly" {
		t.Errorf("query = %v, want 'weekly'", body["query"])
	}
	filter, ok := body["meeting_filter"].(map[string]interface{})
	if !ok {
		t.Fatal("meeting_filter should be present")
	}
	if filter["organizer_ids"] == nil {
		t.Error("organizer_ids should be present")
	}
	if filter["open_room_ids"] == nil {
		t.Error("open_room_ids should be present")
	}
	if filter["start_time"] == nil {
		t.Error("start_time should be present")
	}
}

func TestBuildSearchBody_Empty(t *testing.T) {
	cfg := &core.CliConfig{AppID: "test", AppSecret: "s", Brand: core.BrandFeishu}
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("query", "", "")
	cmd.Flags().String("start", "", "")
	cmd.Flags().String("end", "", "")
	cmd.Flags().String("organizer-ids", "", "")
	cmd.Flags().String("participant-ids", "", "")
	cmd.Flags().String("room-ids", "", "")

	runtime := common.TestNewRuntimeContext(cmd, cfg)
	body := buildSearchBody(runtime, "", "")

	if len(body) != 0 {
		t.Errorf("expected empty body, got %v", body)
	}
}

func TestBuildTimeFilter(t *testing.T) {
	if got := buildTimeFilter("", ""); got != nil {
		t.Errorf("expected nil for empty times, got %v", got)
	}
	got := buildTimeFilter("2026-03-24T00:00Z", "2026-03-25T00:00Z")
	if got["start_time"] != "2026-03-24T00:00Z" || got["end_time"] != "2026-03-25T00:00Z" {
		t.Errorf("unexpected time filter: %v", got)
	}
	got = buildTimeFilter("2026-03-24T00:00Z", "")
	if got["start_time"] != "2026-03-24T00:00Z" || got["end_time"] != nil {
		t.Errorf("expected only start_time: %v", got)
	}
}

func TestBuildMeetingFilter_Empty(t *testing.T) {
	if got := buildMeetingFilter(nil, nil, nil, nil); got != nil {
		t.Errorf("expected nil for all empty, got %v", got)
	}
}

func TestBuildMeetingFilter_WithData(t *testing.T) {
	got := buildMeetingFilter([]string{"ou_a"}, nil, []string{"r1"}, nil)
	if got["participant_ids"] == nil {
		t.Error("participant_ids should be present")
	}
	if got["open_room_ids"] == nil {
		t.Error("open_room_ids should be present")
	}
	if got["organizer_ids"] != nil {
		t.Error("organizer_ids should not be present")
	}
}

func TestMeetingSearchDisplayInfo(t *testing.T) {
	if got := meetingSearchDisplayInfo(map[string]interface{}{"display_info": "Weekly Sync"}); got != "Weekly Sync" {
		t.Fatalf("meetingSearchDisplayInfo() = %q, want %q", got, "Weekly Sync")
	}
	if got := meetingSearchDisplayInfo(map[string]interface{}{}); got != "" {
		t.Fatalf("meetingSearchDisplayInfo() = %q, want empty string", got)
	}
}

func TestMeetingSearchDescription(t *testing.T) {
	if got := meetingSearchDescription(map[string]interface{}{
		"meta_data": map[string]interface{}{"description": "2026-03-24 15:00"},
	}); got != "2026-03-24 15:00" {
		t.Fatalf("meetingSearchDescription() = %q, want %q", got, "2026-03-24 15:00")
	}
	if got := meetingSearchDescription(map[string]interface{}{}); got != "" {
		t.Fatalf("meetingSearchDescription() = %q, want empty string", got)
	}
}

func TestSearch_Validation_NoFilter(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCSearch, []string{"+search", "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for no filter")
	}
	if !strings.Contains(err.Error(), "specify at least one") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSearch_Validation_QueryTooLong(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("query", "", "")
	cmd.Flags().String("start", "", "")
	cmd.Flags().String("end", "", "")
	cmd.Flags().String("organizer-ids", "", "")
	cmd.Flags().String("participant-ids", "", "")
	cmd.Flags().String("room-ids", "", "")
	cmd.Flags().String("page-size", "", "")
	longQuery := strings.Repeat("a", 51)
	_ = cmd.Flags().Set("query", longQuery)

	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())
	err := VCSearch.Validate(context.Background(), runtime)
	if err == nil {
		t.Fatal("expected validation error for overlong query")
	}
	if !strings.Contains(err.Error(), "length must be between 1 and 50 characters") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestSearch_Validation_InvalidPageSize(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("query", "", "")
	cmd.Flags().String("start", "", "")
	cmd.Flags().String("end", "", "")
	cmd.Flags().String("organizer-ids", "", "")
	cmd.Flags().String("participant-ids", "", "")
	cmd.Flags().String("room-ids", "", "")
	cmd.Flags().String("page-size", "", "")
	_ = cmd.Flags().Set("query", "weekly")
	_ = cmd.Flags().Set("page-size", "31")

	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())
	err := VCSearch.Validate(context.Background(), runtime)
	if err == nil {
		t.Fatal("expected validation error for invalid page-size")
	}
	if !strings.Contains(err.Error(), "must be between 1 and 30") {
		t.Fatalf("unexpected error message: %v", err)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("Subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
	}
	if ve.Param != "--page-size" {
		t.Fatalf("Param = %q, want --page-size", ve.Param)
	}
}

func TestSearch_DryRun(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCSearch, []string{"+search", "--query", "test", "--dry-run", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "/open-apis/vc/v1/meetings/search") {
		t.Errorf("dry-run should show API path, got: %s", stdout.String())
	}
}

func TestSearch_InvalidTimeRange(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCSearch, []string{"+search", "--start", "bad-time", "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected error for invalid time")
	}
}

// ---------------------------------------------------------------------------
// Typed error envelope assertions (errs migration lock-in)
// ---------------------------------------------------------------------------

func TestParseTimeRange_InvalidStart_TypedError(t *testing.T) {
	cfg := &core.CliConfig{AppID: "test", AppSecret: "s", Brand: core.BrandFeishu}
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("start", "", "")
	cmd.Flags().String("end", "", "")
	_ = cmd.Flags().Set("start", "not-a-date")
	runtime := common.TestNewRuntimeContext(cmd, cfg)

	_, _, err := parseTimeRange(runtime)
	if err == nil {
		t.Fatal("expected error for invalid --start")
	}
	if !strings.Contains(err.Error(), "--start:") {
		t.Errorf("message should contain '--start:', got: %v", err)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("Subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
	}
	if ve.Param != "--start" {
		t.Errorf("Param = %q, want \"--start\"", ve.Param)
	}
}

func TestParseTimeRange_InvalidEnd_TypedError(t *testing.T) {
	cfg := &core.CliConfig{AppID: "test", AppSecret: "s", Brand: core.BrandFeishu}
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("start", "", "")
	cmd.Flags().String("end", "", "")
	_ = cmd.Flags().Set("end", "not-a-date")
	runtime := common.TestNewRuntimeContext(cmd, cfg)

	_, _, err := parseTimeRange(runtime)
	if err == nil {
		t.Fatal("expected error for invalid --end")
	}
	if !strings.Contains(err.Error(), "--end:") {
		t.Errorf("message should contain '--end:', got: %v", err)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("Subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
	}
	if ve.Param != "--end" {
		t.Errorf("Param = %q, want \"--end\"", ve.Param)
	}
}

func TestParseTimeRange_StartAfterEnd_TypedError(t *testing.T) {
	cfg := &core.CliConfig{AppID: "test", AppSecret: "s", Brand: core.BrandFeishu}
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("start", "", "")
	cmd.Flags().String("end", "", "")
	_ = cmd.Flags().Set("start", "2026-03-25T00:00+08:00")
	_ = cmd.Flags().Set("end", "2026-03-24T00:00+08:00")
	runtime := common.TestNewRuntimeContext(cmd, cfg)

	_, _, err := parseTimeRange(runtime)
	if err == nil {
		t.Fatal("expected error for start after end")
	}
	if !strings.Contains(err.Error(), "--start") || !strings.Contains(err.Error(), "--end") {
		t.Errorf("message should mention --start and --end, got: %v", err)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("Subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
	}
	if ve.Param != "--start" {
		t.Errorf("Param = %q, want \"--start\"", ve.Param)
	}
}

func TestSearch_Validation_QueryTooLong_TypedError(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("query", "", "")
	cmd.Flags().String("start", "", "")
	cmd.Flags().String("end", "", "")
	cmd.Flags().String("organizer-ids", "", "")
	cmd.Flags().String("participant-ids", "", "")
	cmd.Flags().String("room-ids", "", "")
	cmd.Flags().String("page-size", "", "")
	_ = cmd.Flags().Set("query", strings.Repeat("x", 51))

	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())
	err := VCSearch.Validate(context.Background(), runtime)
	if err == nil {
		t.Fatal("expected validation error for overlong query")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("Subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
	}
	if ve.Param != "--query" {
		t.Errorf("Param = %q, want \"--query\"", ve.Param)
	}
}

func TestSearch_Validation_NoFilter_TypedError(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCSearch, []string{"+search", "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for no filter")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("Subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
	}
}

func TestBuildSearchParams(t *testing.T) {
	cfg := &core.CliConfig{AppID: "test", AppSecret: "s", Brand: core.BrandFeishu}

	t.Run("defaults", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("page-token", "", "")
		cmd.Flags().String("page-size", "", "")
		runtime := common.TestNewRuntimeContext(cmd, cfg)
		params := buildSearchParams(runtime)
		if params["page_size"] != "15" {
			t.Errorf("page_size = %v, want \"15\"", params["page_size"])
		}
		if _, ok := params["page_token"]; ok {
			t.Error("page_token should be absent when not set")
		}
	})

	t.Run("custom page-size and page-token", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("page-token", "", "")
		cmd.Flags().String("page-size", "", "")
		_ = cmd.Flags().Set("page-size", "20")
		_ = cmd.Flags().Set("page-token", "tok123")
		runtime := common.TestNewRuntimeContext(cmd, cfg)
		params := buildSearchParams(runtime)
		if params["page_size"] != "20" {
			t.Errorf("page_size = %v, want \"20\"", params["page_size"])
		}
		if params["page_token"] != "tok123" {
			t.Errorf("page_token = %v, want \"tok123\"", params["page_token"])
		}
	})

	t.Run("values are scalars not slices", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("page-token", "", "")
		cmd.Flags().String("page-size", "", "")
		_ = cmd.Flags().Set("page-size", "10")
		_ = cmd.Flags().Set("page-token", "p")
		runtime := common.TestNewRuntimeContext(cmd, cfg)
		params := buildSearchParams(runtime)
		if _, isSlice := params["page_size"].([]string); isSlice {
			t.Error("page_size must be a scalar string, not []string")
		}
		if _, isSlice := params["page_token"].([]string); isSlice {
			t.Error("page_token must be a scalar string, not []string")
		}
	})
}
