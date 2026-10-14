// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"encoding/json"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestParseTaskTimeAbsoluteInputs(t *testing.T) {
	for _, tt := range []struct {
		input  string
		want   string
		allDay bool
	}{
		{"1775174400", "1775174400000", false},
		{"1775174400123", "1775174400123", false},
		{" 1775174400123 ", "1775174400123", false},
		{"1000000000000", "1000000000000", false},
		{"999999999999", "999999999999000", false},
		{"2026-04-03T00:00:00Z", "1775174400000", false},
		{"2026-04-03T08:00:00+08:00", "1775174400000", false},
		{"2026-04-03", strconv.FormatInt(time.Date(2026, 4, 3, 0, 0, 0, 0, time.Local).UnixMilli(), 10), true},
	} {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseTaskTime(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if got["timestamp"] != tt.want || got["is_all_day"] != tt.allDay {
				t.Fatalf("parseTaskTime(%q) = %v, want timestamp %s, all-day %v", tt.input, got, tt.want, tt.allDay)
			}
		})
	}
}

func TestTaskTimeInvalidNumericInputs(t *testing.T) {
	for _, input := range []string{"0", "-1775174400123", "+1775174400123", "01775174400123", "1775174400123junk", "1775174400123.5", "1.775174400123e12", "9223372036854775808"} {
		t.Run(input, func(t *testing.T) {
			if _, err := parseTaskTime(input); err == nil {
				t.Fatalf("accepted invalid time %q", input)
			}
		})
	}
}

func TestTaskTimeDateAndRelativeBoundaries(t *testing.T) {
	for _, hint := range []string{"start", "end"} {
		for _, input := range []string{"2026-04-03", "+2d", "-1w", "+1h", "-2m"} {
			t.Run(hint+"/"+input, func(t *testing.T) {
				before, err := parseTimeFlagSec(input, hint)
				if err != nil {
					t.Fatal(err)
				}
				got, err := parseTimeFlagMillis(input, hint)
				if err != nil {
					t.Fatal(err)
				}
				after, err := parseTimeFlagSec(input, hint)
				if err != nil {
					t.Fatal(err)
				}
				lo, _ := strconv.ParseInt(before, 10, 64)
				hi, _ := strconv.ParseInt(after, 10, 64)
				ms, _ := strconv.ParseInt(got, 10, 64)
				if ms < lo*1000 || ms > hi*1000 || ms%1000 != 0 {
					t.Fatalf("%q %s = %s ms, outside prior seconds behavior [%s, %s]", input, hint, got, before, after)
				}
			})
		}
	}
}

func TestTaskTimeRangeMilliseconds(t *testing.T) {
	for _, tt := range []struct{ input, start, end string }{
		{"1775174400123,1775174400456", "1775174400123", "1775174400456"},
		{"1775174400,1775174400456", "1775174400000", "1775174400456"},
		{"1775174400123,", "1775174400123", ""},
		{",1775174400456", "", "1775174400456"},
	} {
		t.Run(tt.input, func(t *testing.T) {
			start, end, err := parseTimeRangeMillis(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if start != tt.start || end != tt.end {
				t.Fatalf("got %s,%s, want %s,%s", start, end, tt.start, tt.end)
			}
		})
	}
	for _, parse := range []func(string) (string, string, error){parseTimeRangeMillis, parseTimeRangeRFC3339} {
		for _, input := range []string{"1775174400456,1775174400123", "1775174400123,1775174400"} {
			_, _, err := parse(input)
			if p, ok := errs.ProblemOf(err); !ok || p.Subtype != errs.SubtypeInvalidArgument {
				t.Errorf("reversed range %q: expected invalid_argument, got %v", input, err)
			}
		}
	}
}

func TestTaskMillisecondsDryRun(t *testing.T) {
	for _, s := range []common.Shortcut{CreateTask, UpdateTask} {
		t.Run(s.Command, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, stdout, _, _ := taskShortcutTestFactory(t)
			args := []string{s.Command, "--summary", "timestamp regression", "--due", "1775174400123", "--dry-run", "--as", "user", "--format", "json"}
			method, endpoint := "POST", "/open-apis/task/v2/tasks"
			if s.Command == "+update" {
				args = append(args, "--task-id", "task-guid-1")
				method, endpoint = "PATCH", endpoint+"/task-guid-1"
			}
			if err := runMountedTaskShortcut(t, s, args, f, stdout); err != nil {
				t.Fatal(err)
			}
			var preview struct {
				Data struct {
					API []struct {
						Method string                 `json:"method"`
						URL    string                 `json:"url"`
						Params map[string]interface{} `json:"params"`
						Body   map[string]interface{} `json:"body"`
					} `json:"api"`
				} `json:"data"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &preview); err != nil {
				t.Fatal(err)
			}
			if len(preview.Data.API) != 1 {
				t.Fatalf("unexpected preview: %s", stdout.String())
			}
			call := preview.Data.API[0]
			if call.Method != method || call.URL != endpoint || call.Params["user_id_type"] != "open_id" {
				t.Fatalf("unexpected request: %+v", call)
			}
			body := call.Body
			if s.Command == "+update" {
				body = body["task"].(map[string]interface{})
			}
			if got := body["due"].(map[string]interface{})["timestamp"]; got != "1775174400123" {
				t.Errorf("millisecond due timestamp = %s; want 1775174400123", got)
			}
		})
	}
}

func TestTaskMillisecondsSearch(t *testing.T) {
	got, end, err := parseTimeRangeRFC3339("1775174400123,1775174400456")
	if err != nil {
		t.Fatal(err)
	}
	want := time.UnixMilli(1775174400123).Local().Format(time.RFC3339Nano)
	if got != want {
		t.Errorf("search start_time = %s; want %s", got, want)
	}
	if wantEnd := time.UnixMilli(1775174400456).Local().Format(time.RFC3339Nano); end != wantEnd {
		t.Fatalf("search end_time = %s, want %s", end, wantEnd)
	}
}

func TestTaskMillisecondsWriteRequests(t *testing.T) {
	for _, s := range []common.Shortcut{CreateTask, UpdateTask} {
		t.Run(s.Command, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, stdout, _, reg := taskShortcutTestFactory(t)
			args := []string{s.Command, "--summary", "timestamp regression", "--due", "1775174400123", "--as", "user", "--format", "json"}
			method, endpoint := "POST", "/open-apis/task/v2/tasks"
			if s.Command == "+update" {
				args = append(args, "--task-id", "task-guid-1")
				method, endpoint = "PATCH", endpoint+"/task-guid-1"
			}
			stub := &httpmock.Stub{Method: method, URL: endpoint, Body: map[string]interface{}{
				"code": 0, "msg": "success", "data": map[string]interface{}{"task": fullTaskOutputFixture()},
			}}
			reg.Register(stub)
			if err := runMountedTaskShortcut(t, s, args, f, stdout); err != nil {
				t.Fatal(err)
			}
			var body map[string]interface{}
			if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
				t.Fatal(err)
			}
			if s.Command == "+update" {
				body = body["task"].(map[string]interface{})
			}
			if got := body["due"].(map[string]interface{})["timestamp"]; got != "1775174400123" {
				t.Fatalf("HTTP request timestamp = %v, want 1775174400123", got)
			}
		})
	}
}

func TestTaskSearchMillisecondsDryRun(t *testing.T) {
	for _, tt := range []struct {
		shortcut               common.Shortcut
		flag, filter, endpoint string
	}{
		{SearchTask, "due", "due_time", "/open-apis/task/v2/tasks/search"},
		{SearchTasklist, "create-time", "create_time", "/open-apis/task/v2/tasklists/search"},
	} {
		t.Run(tt.shortcut.Command, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, stdout, _, _ := taskShortcutTestFactory(t)
			err := runMountedTaskShortcut(t, tt.shortcut, []string{tt.shortcut.Command, "--" + tt.flag, "1775174400123,1775174400456", "--dry-run", "--as", "user", "--format", "json"}, f, stdout)
			if err != nil {
				t.Fatal(err)
			}
			var preview struct {
				Data struct {
					API []struct {
						Method string `json:"method"`
						URL    string `json:"url"`
						Body   struct {
							Filter map[string]struct {
								Start string `json:"start_time"`
								End   string `json:"end_time"`
							} `json:"filter"`
						} `json:"body"`
					} `json:"api"`
				} `json:"data"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &preview); err != nil {
				t.Fatal(err)
			}
			if len(preview.Data.API) != 1 {
				t.Fatalf("unexpected preview: %s", stdout.String())
			}
			call := preview.Data.API[0]
			if call.Method != "POST" || call.URL != tt.endpoint {
				t.Fatalf("unexpected request: %+v", call)
			}
			filter := call.Body.Filter[tt.filter]
			for _, boundary := range []struct {
				input string
				want  int64
			}{{filter.Start, 1775174400123}, {filter.End, 1775174400456}} {
				got, err := time.Parse(time.RFC3339Nano, boundary.input)
				if err != nil {
					t.Fatal(err)
				}
				if got.UnixMilli() != boundary.want {
					t.Errorf("search time = %s, want %d ms", boundary.input, boundary.want)
				}
			}
		})
	}
}

func TestTaskMillisecondsFilter(t *testing.T) {
	for _, tt := range []struct {
		flag, input string
		want        []string
	}{
		{"due-start", "1775174400", []string{"before", "equal", "after"}},
		{"due-start", "1775174400123", []string{"equal", "after"}},
		{"due-end", "1775174400123", []string{"before", "equal"}},
		{"created_at", "1775174400123", []string{"equal", "after"}},
	} {
		t.Run(tt.flag+"/"+tt.input, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, stdout, _, reg := taskShortcutTestFactory(t)
			items := make([]interface{}, 0, 3)
			for i, id := range []string{"before", "equal", "after"} {
				ts := strconv.FormatInt(1775174400122+int64(i), 10)
				items = append(items, map[string]interface{}{"guid": id, "summary": id, "created_at": ts, "due": map[string]interface{}{"timestamp": ts}})
			}
			reg.Register(&httpmock.Stub{
				Method: "GET", URL: "/open-apis/task/v2/tasks",
				Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{
					"has_more": false,
					"items":    items,
				}},
			})
			err := runMountedTaskShortcut(t, GetMyTasks, []string{"+get-my-tasks", "--as", "user", "--format", "json", "--" + tt.flag, tt.input}, f, stdout)
			if err != nil {
				t.Fatal(err)
			}
			var result struct {
				Data struct {
					Items []struct {
						GUID string `json:"guid"`
					} `json:"items"`
				} `json:"data"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, item := range result.Data.Items {
				got = append(got, item.GUID)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("filtered IDs = %v, want %v; output: %s", got, tt.want, stdout.String())
			}
		})
	}
}
