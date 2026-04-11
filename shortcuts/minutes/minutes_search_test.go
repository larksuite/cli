// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

// newMinutesSearchTestCommand builds a command with the flags used by minutes search tests.
func newMinutesSearchTestCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("query", "", "")
	cmd.Flags().String("owner-ids", "", "")
	cmd.Flags().String("participant-ids", "", "")
	cmd.Flags().String("start", "", "")
	cmd.Flags().String("end", "", "")
	cmd.Flags().String("page-token", "", "")
	cmd.Flags().String("page-size", "15", "")
	return cmd
}

// configWithoutUserOpenID returns a test config without a resolvable user open_id.
func configWithoutUserOpenID() *core.CliConfig {
	cfg := defaultConfig()
	cfg.UserOpenId = ""
	return cfg
}

// TestMinutesSearchParseTimeRange verifies valid time inputs are normalized.
func TestMinutesSearchParseTimeRange(t *testing.T) {
	t.Parallel()

	cmd := newMinutesSearchTestCommand()
	_ = cmd.Flags().Set("start", "2026-03-24")
	_ = cmd.Flags().Set("end", "2026-03-25")

	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())
	start, end, err := parseTimeRange(runtime)
	if err != nil {
		t.Fatalf("parseTimeRange() unexpected error: %v", err)
	}
	if start == "" || end == "" {
		t.Fatalf("expected non-empty start/end, got %q %q", start, end)
	}
}

// TestMinutesSearchParseTimeRangeErrors verifies invalid time inputs return validation errors.
func TestMinutesSearchParseTimeRangeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		start       string
		end         string
		wantMessage string
	}{
		{name: "invalid start", start: "bad-start", wantMessage: "--start:"},
		{name: "invalid end", end: "bad-end", wantMessage: "--end:"},
		{name: "start after end", start: "2026-03-26", end: "2026-03-25", wantMessage: "is after --end"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd := newMinutesSearchTestCommand()
			if tt.start != "" {
				_ = cmd.Flags().Set("start", tt.start)
			}
			if tt.end != "" {
				_ = cmd.Flags().Set("end", tt.end)
			}

			_, _, err := parseTimeRange(common.TestNewRuntimeContext(cmd, defaultConfig()))
			if err == nil {
				t.Fatal("expected parseTimeRange error")
			}
			if !strings.Contains(err.Error(), tt.wantMessage) {
				t.Fatalf("error = %v, want %q", err, tt.wantMessage)
			}
		})
	}
}

// TestBuildMinutesSearchParams verifies request params and body fields are assembled correctly.
func TestBuildMinutesSearchParams(t *testing.T) {
	t.Parallel()

	cmd := newMinutesSearchTestCommand()
	_ = cmd.Flags().Set("query", "budget")
	_ = cmd.Flags().Set("owner-ids", "ou_owner,ou_owner_2")
	_ = cmd.Flags().Set("participant-ids", "ou_c")
	_ = cmd.Flags().Set("page-size", "5")
	_ = cmd.Flags().Set("page-token", "next_page")

	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())
	params := buildMinutesSearchParams(runtime)
	body, err := buildMinutesSearchBody(runtime, "2026-03-24T00:00:00Z", "2026-03-25T00:00:00Z")
	if err != nil {
		t.Fatalf("buildMinutesSearchBody() unexpected error: %v", err)
	}

	if got, _ := params["page_size"].(string); got != "5" {
		t.Fatalf("page_size = %q, want 5", got)
	}
	if got, _ := params["page_token"].(string); got != "next_page" {
		t.Fatalf("page_token = %q, want next_page", got)
	}
	if body["query"] != "budget" {
		t.Fatalf("body.query = %v, want budget", body["query"])
	}
	filter, _ := body["filter"].(map[string]interface{})
	if filter == nil {
		t.Fatalf("body.filter = nil, want filter object")
	}
	owners, _ := filter["owner_ids"].([]string)
	if len(owners) != 2 || owners[0] != "ou_owner" || owners[1] != "ou_owner_2" {
		t.Fatalf("owner_ids = %v, want [ou_owner ou_owner_2]", filter["owner_ids"])
	}
	participants, _ := filter["participant_ids"].([]string)
	if len(participants) != 1 || participants[0] != "ou_c" {
		t.Fatalf("participant_ids = %v, want [ou_c]", filter["participant_ids"])
	}
	createTime, _ := filter["create_time"].(map[string]interface{})
	if createTime == nil {
		t.Fatalf("create_time = nil, want time range")
	}
	if createTime["start_time"] != "2026-03-24T00:00:00Z" {
		t.Fatalf("start_time = %v", createTime["start_time"])
	}
	if createTime["end_time"] != "2026-03-25T00:00:00Z" {
		t.Fatalf("end_time = %v", createTime["end_time"])
	}
}

// TestBuildMinutesSearchParamsDefaultPageSize verifies the default page size is applied.
func TestBuildMinutesSearchParamsDefaultPageSize(t *testing.T) {
	t.Parallel()

	cmd := newMinutesSearchTestCommand()

	params := buildMinutesSearchParams(common.TestNewRuntimeContext(cmd, defaultConfig()))
	if got, _ := params["page_size"].(string); got != "15" {
		t.Fatalf("page_size = %q, want 15", got)
	}
}

// TestResolveUserIDs verifies me expansion, deduplication, and nil handling.
func TestResolveUserIDs(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "test"}
	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())

	got, err := resolveUserIDs("--owner-ids", []string{"me"}, runtime)
	if err != nil {
		t.Fatalf("resolveUserIDs([me]) unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != "ou_testuser" {
		t.Fatalf("resolveUserIDs([me]) = %v, want [ou_testuser]", got)
	}

	got, err = resolveUserIDs("--owner-ids", []string{"ou_other", "me", "Me"}, runtime)
	if err != nil {
		t.Fatalf("resolveUserIDs([ou_other, me, Me]) unexpected error: %v", err)
	}
	if len(got) != 2 || got[0] != "ou_other" || got[1] != "ou_testuser" {
		t.Fatalf("resolveUserIDs([ou_other, me, Me]) = %v, want [ou_other ou_testuser]", got)
	}

	got, err = resolveUserIDs("--owner-ids", nil, runtime)
	if err != nil {
		t.Fatalf("resolveUserIDs(nil) unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("resolveUserIDs(nil) = %v, want nil", got)
	}
}

// TestBuildTimeFilter verifies time filters are only populated for provided bounds.
func TestBuildTimeFilter(t *testing.T) {
	t.Parallel()

	if got := buildTimeFilter("", ""); got != nil {
		t.Fatalf("buildTimeFilter('', '') = %v, want nil", got)
	}
	if got := buildTimeFilter("2026-03-24T00:00:00Z", ""); got["start_time"] != "2026-03-24T00:00:00Z" {
		t.Fatalf("start_time = %v", got["start_time"])
	}
	if got := buildTimeFilter("", "2026-03-25T00:00:00Z"); got["end_time"] != "2026-03-25T00:00:00Z" {
		t.Fatalf("end_time = %v", got["end_time"])
	}
}

// TestMinutesSearchValidationMeOwnerID verifies owner-ids accepts me when open_id is available.
func TestMinutesSearchValidationMeOwnerID(t *testing.T) {
	cmd := newMinutesSearchTestCommand()
	_ = cmd.Flags().Set("owner-ids", "me")

	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())
	err := MinutesSearch.Validate(context.Background(), runtime)
	if err != nil {
		t.Fatalf("expected no error for --owner-ids me, got: %v", err)
	}
}

// TestMinutesSearchValidationMeRequiresResolvableUser verifies me fails without a resolvable open_id.
func TestMinutesSearchValidationMeRequiresResolvableUser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		flag string
	}{
		{name: "owner ids", flag: "owner-ids"},
		{name: "participant ids", flag: "participant-ids"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd := newMinutesSearchTestCommand()
			_ = cmd.Flags().Set(tt.flag, "me")

			runtime := common.TestNewRuntimeContext(cmd, configWithoutUserOpenID())
			err := MinutesSearch.Validate(context.Background(), runtime)
			if err == nil {
				t.Fatal("expected validation error for unresolved me")
			}
			if !strings.Contains(err.Error(), "resolvable open_id") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestBuildMinutesSearchFilterMeExpansion verifies me is expanded inside the request filter.
func TestBuildMinutesSearchFilterMeExpansion(t *testing.T) {
	t.Parallel()

	cmd := newMinutesSearchTestCommand()
	_ = cmd.Flags().Set("owner-ids", "me,ou_other")
	_ = cmd.Flags().Set("participant-ids", "me")

	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())
	body, err := buildMinutesSearchBody(runtime, "", "")
	if err != nil {
		t.Fatalf("buildMinutesSearchBody() unexpected error: %v", err)
	}

	filter, _ := body["filter"].(map[string]interface{})
	if filter == nil {
		t.Fatal("body.filter = nil, want filter object")
	}
	owners, _ := filter["owner_ids"].([]string)
	if len(owners) != 2 || owners[0] != "ou_testuser" || owners[1] != "ou_other" {
		t.Fatalf("owner_ids = %v, want [ou_testuser ou_other]", owners)
	}
	participants, _ := filter["participant_ids"].([]string)
	if len(participants) != 1 || participants[0] != "ou_testuser" {
		t.Fatalf("participant_ids = %v, want [ou_testuser]", participants)
	}
}

// TestMinuteSearchItems verifies items extraction from the search response payload.
func TestMinuteSearchItems(t *testing.T) {
	t.Parallel()

	items := minuteSearchItems(map[string]interface{}{
		"items": []interface{}{map[string]interface{}{"minute_token": "tok_1"}},
	})
	if len(items) != 1 {
		t.Fatalf("minuteSearchItems() len = %d, want 1", len(items))
	}

	if got := minuteSearchItems(map[string]interface{}{}); got != nil {
		t.Fatalf("minuteSearchItems() = %v, want nil", got)
	}
}

// TestMinutesSearchValidationNoFilter verifies at least one filter is required.
func TestMinutesSearchValidationNoFilter(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, MinutesSearch, []string{"+search", "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for empty filters")
	}
	if !strings.Contains(err.Error(), "specify at least one") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestMinutesSearchValidationInvalidParticipantID verifies participant IDs must be valid open_ids.
func TestMinutesSearchValidationInvalidParticipantID(t *testing.T) {
	cmd := newMinutesSearchTestCommand()
	_ = cmd.Flags().Set("participant-ids", "user_123")

	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())
	err := MinutesSearch.Validate(context.Background(), runtime)
	if err == nil {
		t.Fatal("expected invalid user ID error")
	}
}

// TestMinutesSearchValidationInvalidOwnerID verifies owner IDs must be valid open_ids.
func TestMinutesSearchValidationInvalidOwnerID(t *testing.T) {
	cmd := newMinutesSearchTestCommand()
	_ = cmd.Flags().Set("owner-ids", "user_123")

	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())
	err := MinutesSearch.Validate(context.Background(), runtime)
	if err == nil {
		t.Fatal("expected invalid owner ID error")
	}
}

// TestMinutesSearchValidationQueryTooLong verifies overly long queries are rejected.
func TestMinutesSearchValidationQueryTooLong(t *testing.T) {
	cmd := newMinutesSearchTestCommand()
	_ = cmd.Flags().Set("query", strings.Repeat("a", 51))

	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())
	err := MinutesSearch.Validate(context.Background(), runtime)
	if err == nil {
		t.Fatal("expected query length error")
	}
	if !strings.Contains(err.Error(), "length must be between 1 and 50 characters") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestMinutesSearchValidationMaxPageSize30 verifies the maximum allowed page size passes validation.
func TestMinutesSearchValidationMaxPageSize30(t *testing.T) {
	cmd := newMinutesSearchTestCommand()
	_ = cmd.Flags().Set("query", "budget")
	_ = cmd.Flags().Set("page-size", "30")

	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())
	err := MinutesSearch.Validate(context.Background(), runtime)
	if err != nil {
		t.Fatalf("expected no error for --page-size 30, got: %v", err)
	}
}

// TestMinutesSearchValidationPageSizeAboveMax verifies page sizes above the limit are rejected.
func TestMinutesSearchValidationPageSizeAboveMax(t *testing.T) {
	cmd := newMinutesSearchTestCommand()
	_ = cmd.Flags().Set("query", "budget")
	_ = cmd.Flags().Set("page-size", "31")

	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())
	err := MinutesSearch.Validate(context.Background(), runtime)
	if err == nil {
		t.Fatal("expected validation error for --page-size 31")
	}
	if !strings.Contains(err.Error(), "--page-size") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestMinutesSearchValidationTimeErrors verifies time parsing failures surface through validation.
func TestMinutesSearchValidationTimeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		start       string
		end         string
		wantMessage string
	}{
		{name: "invalid start", start: "bad-start", wantMessage: "--start:"},
		{name: "invalid end", end: "bad-end", wantMessage: "--end:"},
		{name: "start after end", start: "2026-03-26", end: "2026-03-25", wantMessage: "is after --end"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd := newMinutesSearchTestCommand()
			_ = cmd.Flags().Set("query", "budget")
			if tt.start != "" {
				_ = cmd.Flags().Set("start", tt.start)
			}
			if tt.end != "" {
				_ = cmd.Flags().Set("end", tt.end)
			}

			err := MinutesSearch.Validate(context.Background(), common.TestNewRuntimeContext(cmd, defaultConfig()))
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tt.wantMessage) {
				t.Fatalf("error = %v, want %q", err, tt.wantMessage)
			}
		})
	}
}

// TestMinutesSearchDryRun verifies dry-run output includes the expected API request details.
func TestMinutesSearchDryRun(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, MinutesSearch, []string{"+search", "--query", "budget", "--owner-ids", "ou_owner,ou_owner_2", "--dry-run", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "/open-apis/minutes/v1/minutes/search") {
		t.Fatalf("dry-run should show API path, got: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "\"method\": \"POST\"") {
		t.Fatalf("dry-run should use POST, got: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "\"query\": \"budget\"") {
		t.Fatalf("dry-run should show query in body, got: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "\"owner_ids\": [") || !strings.Contains(stdout.String(), "\"ou_owner\"") {
		t.Fatalf("dry-run should show owner_ids in filter, got: %s", stdout.String())
	}
}

// TestMinutesSearchExecuteRendersRowsAndMoreHint verifies pretty output renders rows and pagination hints.
func TestMinutesSearchExecuteRendersRowsAndMoreHint(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	searchStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/minutes/v1/minutes/search",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"token":        "minute_1",
						"display_info": "周会摘要",
						"meta_data": map[string]interface{}{
							"description": "周会纪要",
							"app_link":    "https://meetings.feishu.cn/minutes/obcn123",
							"avatar":      "https://p3-lark-file.byteimg.com/img/xxxx.jpg",
						},
					},
				},
				"total":      1,
				"has_more":   true,
				"page_token": "next_token",
			},
		},
	}
	reg.Register(searchStub)

	err := mountAndRun(t, MinutesSearch, []string{"+search", "--query", "budget", "--owner-ids", "me", "--format", "pretty", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)

	var body map[string]interface{}
	if err := json.Unmarshal(searchStub.CapturedBody, &body); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	if body["query"] != "budget" {
		t.Fatalf("request query = %v, want budget", body["query"])
	}
	filter, _ := body["filter"].(map[string]interface{})
	if filter == nil {
		t.Fatalf("request filter = %v, want object", body["filter"])
	}
	owners, _ := filter["owner_ids"].([]interface{})
	if len(owners) != 1 || owners[0] != "ou_testuser" {
		t.Fatalf("request owner_ids = %v, want [ou_testuser]", filter["owner_ids"])
	}

	out := stdout.String()
	for _, want := range []string{"minute_1", "周会摘要", "周会纪要", "https://meetings.feishu.cn/minutes/obcn123", "https://p3-lark-file.byteimg.com/img/xxxx.jpg", "next_token", "more available"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q, got: %s", want, out)
		}
	}
}

// TestMinutesSearchExecuteNoMinutes verifies empty results render the no-data message.
func TestMinutesSearchExecuteNoMinutes(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/minutes/v1/minutes/search",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items":      []interface{}{},
				"total":      0,
				"has_more":   false,
				"page_token": "",
			},
		},
	})

	err := mountAndRun(t, MinutesSearch, []string{"+search", "--query", "budget", "--format", "pretty", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)
	if !strings.Contains(stdout.String(), "No minutes.") {
		t.Fatalf("expected no minutes message, got: %s", stdout.String())
	}
}

// TestMinutesSearchExecuteShowsPaginationHintForTableFormat verifies table output includes pagination hints.
func TestMinutesSearchExecuteShowsPaginationHintForTableFormat(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/minutes/v1/minutes/search",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"token":        "minute_1",
						"display_info": "周会摘要",
						"meta_data": map[string]interface{}{
							"description": "周会纪要",
							"app_link":    "https://meetings.feishu.cn/minutes/obcn123",
							"avatar":      "https://p3-lark-file.byteimg.com/img/xxxx.jpg",
						},
					},
				},
				"total":      1,
				"has_more":   true,
				"page_token": "next_token",
			},
		},
	})

	err := mountAndRun(t, MinutesSearch, []string{"+search", "--query", "budget", "--format", "table", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)

	out := stdout.String()
	if !strings.Contains(out, "next_token") || !strings.Contains(out, "more available") {
		t.Fatalf("expected pagination hint in table output, got: %s", out)
	}
}

// TestMinutesSearchExecuteJSONCountUsesRenderedRows verifies JSON metadata counts rendered rows only.
func TestMinutesSearchExecuteJSONCountUsesRenderedRows(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/minutes/v1/minutes/search",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					nil,
					map[string]interface{}{
						"token":        "minute_1",
						"display_info": "周会摘要",
						"meta_data": map[string]interface{}{
							"description": "周会纪要",
						},
					},
				},
				"total":      2,
				"has_more":   false,
				"page_token": "",
			},
		},
	})

	err := mountAndRun(t, MinutesSearch, []string{"+search", "--query", "budget", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)

	var envelope struct {
		Meta struct {
			Count int `json:"count"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to parse output: %v\nraw: %s", err, stdout.String())
	}
	if envelope.Meta.Count != 1 {
		t.Fatalf("meta.count = %d, want 1", envelope.Meta.Count)
	}
}

// TestMinuteSearchFieldExtractors verifies field extractors read populated metadata correctly.
func TestMinuteSearchFieldExtractors(t *testing.T) {
	t.Parallel()

	item := map[string]interface{}{
		"token":        "minute_1",
		"display_info": "<h>周会</h>摘要",
		"meta_data": map[string]interface{}{
			"description": "周会纪要",
			"app_link":    "https://meetings.feishu.cn/minutes/obcn123",
			"avatar":      "https://p3-lark-file.byteimg.com/img/xxxx.jpg",
		},
	}

	if got := minuteSearchToken(item); got != "minute_1" {
		t.Fatalf("minuteSearchToken() = %q, want minute_1", got)
	}
	if got := minuteSearchDisplayInfo(item); got != "<h>周会</h>摘要" {
		t.Fatalf("minuteSearchDisplayInfo() = %q", got)
	}
	if got := minuteSearchDescription(item); got != "周会纪要" {
		t.Fatalf("minuteSearchDescription() = %q, want 周会纪要", got)
	}
	if got := minuteSearchAppLink(item); got != "https://meetings.feishu.cn/minutes/obcn123" {
		t.Fatalf("minuteSearchAppLink() = %q", got)
	}
	if got := minuteSearchAvatar(item); got != "https://p3-lark-file.byteimg.com/img/xxxx.jpg" {
		t.Fatalf("minuteSearchAvatar() = %q", got)
	}
}

// TestMinuteSearchFieldExtractorsFallbacks verifies extractors keep working for alternate sample data.
func TestMinuteSearchFieldExtractorsFallbacks(t *testing.T) {
	t.Parallel()

	item := map[string]interface{}{
		"token":        "minute_2",
		"display_info": "回退摘要",
		"meta_data": map[string]interface{}{
			"description": "回退纪要",
			"app_link":    "https://meetings.feishu.cn/minutes/fallback",
			"avatar":      "https://p3-lark-file.byteimg.com/img/fallback.jpg",
		},
	}

	if got := minuteSearchToken(item); got != "minute_2" {
		t.Fatalf("minuteSearchToken() = %q, want minute_2", got)
	}
	if got := minuteSearchDescription(item); got != "回退纪要" {
		t.Fatalf("minuteSearchDescription() = %q, want 回退纪要", got)
	}
	if got := minuteSearchAppLink(item); got != "https://meetings.feishu.cn/minutes/fallback" {
		t.Fatalf("minuteSearchAppLink() = %q", got)
	}
	if got := minuteSearchAvatar(item); got != "https://p3-lark-file.byteimg.com/img/fallback.jpg" {
		t.Fatalf("minuteSearchAvatar() = %q", got)
	}
}

// TestMinuteSearchFieldExtractorsMissingMetaData verifies extractors fall back to empty values without metadata.
func TestMinuteSearchFieldExtractorsMissingMetaData(t *testing.T) {
	t.Parallel()

	item := map[string]interface{}{
		"token":        "minute_3",
		"display_info": "无元信息摘要",
	}

	if got := minuteSearchToken(item); got != "minute_3" {
		t.Fatalf("minuteSearchToken() = %q, want minute_3", got)
	}
	if got := minuteSearchDescription(item); got != "" {
		t.Fatalf("minuteSearchDescription() = %q, want empty", got)
	}
	if got := minuteSearchAppLink(item); got != "" {
		t.Fatalf("minuteSearchAppLink() = %q, want empty", got)
	}
	if got := minuteSearchAvatar(item); got != "" {
		t.Fatalf("minuteSearchAvatar() = %q, want empty", got)
	}
}
