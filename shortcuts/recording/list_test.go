// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package recording

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/spf13/cobra"
)

func listCommand(t *testing.T) (*cobra.Command, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	f, out, _, reg := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu})
	root := &cobra.Command{Use: "recording", SilenceErrors: true, SilenceUsage: true}
	List.Mount(root, f)
	return root, out, reg
}

func TestTimeMillis(t *testing.T) {
	for _, tt := range []struct{ input, flag, want string }{
		{"2026-09-01T00:00:00.123+08:00", "start", "1788192000123"},
		{"1788192000", "start", "1788192000000"},
		{"2026-09-01T01:02:03.456", "start", strconv.FormatInt(time.Date(2026, 9, 1, 1, 2, 3, 456000000, time.Local).UnixMilli(), 10)},
		{"2026-09-01", "start", strconv.FormatInt(time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local).UnixMilli(), 10)},
		{"2026-09-01", "end", strconv.FormatInt(time.Date(2026, 9, 1, 23, 59, 59, 999000000, time.Local).UnixMilli(), 10)},
		{"", "end", ""},
	} {
		t.Run(tt.input+tt.flag, func(t *testing.T) {
			got, err := timeMillis(tt.input, tt.flag)
			if err != nil || got != tt.want {
				t.Fatalf("got %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestListValidation(t *testing.T) {
	for _, tt := range []struct {
		flags []string
		param string
	}{
		{[]string{"--start", "bad-date"}, "--start"},
		{[]string{"--end", "1969-12-31T23:59:59Z"}, "--end"},
		{[]string{"--start", "9223372036854775807"}, "--start"},
		{[]string{"--start", "2026-09-02", "--end", "2026-09-01"}, "--start"},
		{[]string{"--page-size", "0"}, "--page-size"},
		{[]string{"--page-size", "51"}, "--page-size"},
		{[]string{"--page-limit", "0"}, "--page-limit"},
		{[]string{"--page-delay", "-1"}, "--page-delay"},
		{[]string{"--as", "bot"}, "--as"},
	} {
		t.Run(strings.Join(tt.flags, " "), func(t *testing.T) {
			cmd, out, _ := listCommand(t)
			cmd.SetArgs(append([]string{"+list", "--as", "user", "--dry-run"}, tt.flags...))
			var ve *errs.ValidationError
			if err := cmd.Execute(); !errors.As(err, &ve) || ve.Param != tt.param {
				t.Fatalf("got %v, want validation for %s", err, tt.param)
			}
			if out.Len() != 0 {
				t.Fatal("invalid request emitted success")
			}
		})
	}
}

func TestListDryRun(t *testing.T) {
	cmd, out, _ := listCommand(t)
	cmd.SetArgs([]string{"+list", "--as", "user", "--start", "2026-09-01T00:00:00.123+08:00", "--end", "2026-09-01T00:00:00.123+08:00", "--page-token", " opaque+cursor= ", "--page-size", "2", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Data struct {
			API []cmdutil.DryRunAPICall `json:"api"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Data.API) != 1 {
		t.Fatalf("calls: %s", out)
	}
	call := got.Data.API[0]
	if call.Method != "GET" || call.URL != listPath {
		t.Fatalf("request: %+v", call)
	}
	for key, want := range map[string]interface{}{"page_size": "2", "page_token": " opaque+cursor= ", "start_time_ms": "1788192000123", "end_time_ms": "1788192000123"} {
		if call.Params[key] != want {
			t.Errorf("%s=%v, want %v", key, call.Params[key], want)
		}
	}
}

func TestListPagination(t *testing.T) {
	for _, capped := range []bool{false, true} {
		t.Run(strconv.FormatBool(capped), func(t *testing.T) {
			cmd, out, reg := listCommand(t)
			for i, token := range []string{"initial+cursor=", "second+cursor="} {
				reg.Register(&httpmock.Stub{Method: "GET", URL: listPath, OnMatch: func(req *http.Request) {
					for key, want := range map[string]string{"page_token": token, "page_size": "1", "start_time_ms": "1788192000123", "end_time_ms": "1788278400456"} {
						if got := req.URL.Query().Get(key); got != want {
							t.Errorf("page %d %s=%q, want %q", i, key, got, want)
						}
					}
				}, Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
					"recordings": []interface{}{map[string]interface{}{"recording_id": "100000000000000000" + strconv.Itoa(i), "start_time_ms": "1788192000123", "duration_ms": "61002", "artifacts": []interface{}{map[string]interface{}{"type": 1, "status": 2, "artifact_id": "1000000000000000003", "artifact_token": "minute_token"}, map[string]interface{}{"type": 2, "status": 1, "artifact_id": "1000000000000000004"}}}},
					"has_more":   i == 0 || capped, "page_token": []string{"second+cursor=", "third+cursor="}[i],
				}}})
			}
			cmd.SetArgs([]string{"+list", "--as", "user", "--page-all", "--page-limit", "2", "--page-delay", "0", "--page-size", "1", "--page-token", "initial+cursor=", "--start", "2026-09-01T00:00:00.123+08:00", "--end", "2026-09-02T00:00:00.456+08:00"})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			var got struct {
				Data listResult  `json:"data"`
				Meta output.Meta `json:"meta"`
			}
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if len(got.Data.Recordings) != 2 || got.Data.HasMore != capped || got.Data.PageToken != "third+cursor=" {
				t.Fatalf("bad result: %s", out)
			}
			if got.Meta.Pagination == nil || got.Meta.Pagination.Complete == capped || got.Meta.Pagination.Pages != 2 || got.Meta.Pagination.Items != 2 {
				t.Fatalf("bad pagination: %s", out)
			}
			for i, item := range got.Data.Recordings {
				if item.ID != "100000000000000000"+strconv.Itoa(i) || item.Start != "1788192000123" || len(item.Artifacts) != 2 || item.Artifacts[0].Token != "minute_token" || item.Artifacts[1].ID != "1000000000000000004" {
					t.Fatalf("lost fields: %+v", item)
				}
			}
		})
	}
}

func TestListSecondPageErrorDoesNotEmitSuccess(t *testing.T) {
	cmd, out, reg := listCommand(t)
	reg.Register(&httpmock.Stub{URL: listPath, Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"recordings": []interface{}{}, "has_more": true, "page_token": "next"}}})
	reg.Register(&httpmock.Stub{URL: listPath, Body: map[string]interface{}{"code": 99991679, "msg": "missing scope"}})
	cmd.SetArgs([]string{"+list", "--as", "user", "--page-all", "--page-delay", "0"})
	err := cmd.Execute()
	if _, ok := errs.ProblemOf(err); !ok {
		t.Fatalf("expected typed API error, got %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("partial failure emitted success: %s", out)
	}
}

func TestListMalformedResponse(t *testing.T) {
	cmd, _, reg := listCommand(t)
	reg.Register(&httpmock.Stub{URL: listPath, Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"recordings": []interface{}{map[string]interface{}{"recording_id": 100}}}}})
	cmd.SetArgs([]string{"+list", "--as", "user"})
	var ie *errs.InternalError
	if err := cmd.Execute(); !errors.As(err, &ie) || ie.Subtype != errs.SubtypeInvalidResponse || errors.Unwrap(err) == nil {
		t.Fatalf("expected typed decode error with cause, got %v", err)
	}
}

func TestListEmptyAndPretty(t *testing.T) {
	cmd, out, reg := listCommand(t)
	reg.Register(&httpmock.Stub{URL: listPath, Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"has_more": false}}})
	cmd.SetArgs([]string{"+list", "--as", "user"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"recordings": []`) {
		t.Fatalf("empty list must be an array: %s", out)
	}
	out.Reset()
	printRecordings(out, &listResult{Recordings: []recording{{ID: "recording-id", Start: "1788192000123", Duration: "61002", Artifacts: []artifact{{Type: 1, Status: 2, Token: "minute-token"}, {Type: 2, Status: 1, ID: "note-id"}, {Type: 99, Status: 99}}}}, HasMore: true, PageToken: "next"})
	for _, want := range []string{"recording-id", "1m 01.002s", "Minutes (ready): minute-token", "Note (generating): note-id", "unknown(99)"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("pretty output missing %q: %s", want, out)
		}
	}
}

// The shared emitter owns pagination presentation; opaque cursors remain raw in
// JSON but cannot emit terminal control sequences through pretty output.
func TestListPrettyMissingMinutesTokenAndPagination(t *testing.T) {
	const cursor = "next\x1b[2J\nforged"
	for _, format := range []string{"pretty", "json"} {
		t.Run(format, func(t *testing.T) {
			cmd, out, reg := listCommand(t)
			reg.Register(&httpmock.Stub{URL: listPath, Body: map[string]interface{}{
				"code": 0, "data": map[string]interface{}{
					"recordings": []interface{}{map[string]interface{}{
						"recording_id": "recording-id",
						"artifacts":    []interface{}{map[string]interface{}{"type": 1, "status": 1, "artifact_id": "minutes-artifact-id"}},
					}},
					"has_more": true, "page_token": cursor,
				},
			}})
			cmd.SetArgs([]string{"+list", "--as", "user", "--format", format})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if format == "json" {
				var got struct {
					Data listResult  `json:"data"`
					Meta output.Meta `json:"meta"`
				}
				if err := json.Unmarshal(out.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if got.Data.PageToken != cursor || got.Meta.Pagination.NextToken != cursor {
					t.Fatal("JSON cursor changed")
				}
				a := got.Data.Recordings[0].Artifacts[0]
				if a.ID != "minutes-artifact-id" || a.Token != "" {
					t.Fatalf("artifact identifiers changed: %+v", a)
				}
				return
			}
			for _, want := range []string{"Minutes (generating): minute_token unavailable (artifact_id=minutes-artifact-id)", "Pagination: incomplete", strconv.Quote(cursor)} {
				if !strings.Contains(out.String(), want) {
					t.Errorf("pretty output missing %q: %s", want, out)
				}
			}
			if strings.Contains(out.String(), "\x1b") || strings.Contains(out.String(), "\nforged") {
				t.Fatalf("unescaped cursor in pretty output: %q", out.String())
			}
			if strings.Count(out.String(), "Pagination:") != 1 || strings.Contains(out.String(), "page_token:") {
				t.Fatalf("duplicate pagination hints: %s", out)
			}
		})
	}
}
