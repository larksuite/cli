// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func fgGroup(id string) map[string]interface{} {
	return map[string]interface{}{"group_id": id, "name": id, "type": "normal"}
}

// TestFeedGroupListPageAllMergesBothLists is the core regression for the
// +feed-group-list shortcut: a dual-list response (groups + deleted_groups) must
// have BOTH lists merged across pages — including active groups that appear only
// on a later page. This is what the raw `feed.groups list --page-all` gets wrong.
func TestFeedGroupListPageAllMergesBothLists(t *testing.T) {
	var reqs []recordedFGRequest
	runtime := newFGRuntime(t, ImFeedGroupList, map[string]string{"page-all": "true", "page-size": "5"}, &reqs,
		func(_ string, page int) (int, interface{}) {
			if page == 1 {
				// page 1 fills up with mostly deleted groups; the active groups
				// g1/g2 here plus one more (g3) on page 2.
				return 200, wrapData(map[string]interface{}{
					"groups":         []interface{}{fgGroup("g1"), fgGroup("g2")},
					"deleted_groups": []interface{}{fgGroup("d1"), fgGroup("d2"), fgGroup("d3")},
					"page_token":     "TKN", "has_more": true,
				})
			}
			return 200, wrapData(map[string]interface{}{
				"groups":         []interface{}{fgGroup("g3")},
				"deleted_groups": []interface{}{fgGroup("d4")},
				"page_token":     "", "has_more": false,
			})
		})

	if err := ImFeedGroupList.Execute(context.Background(), runtime); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if got := countFGRequests(reqs, "/groups"); got != 2 {
		t.Fatalf("expected 2 groups requests, got %d", got)
	}
	if got := firstQueryValue(reqs[1].query, "page_token"); got != "TKN" {
		t.Errorf("second page token = %q, want TKN", got)
	}

	out, _ := runtime.Factory.IOStreams.Out.(*bytes.Buffer)
	if out == nil {
		t.Fatal("stdout buffer missing")
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out.String())
	}
	data, _ := parsed["data"].(map[string]interface{})
	if got := len(data["groups"].([]interface{})); got != 3 {
		t.Errorf("merged groups = %d, want 3 (active list must include later pages)", got)
	}
	if got := len(data["deleted_groups"].([]interface{})); got != 4 {
		t.Errorf("merged deleted_groups = %d, want 4 (secondary list must also merge)", got)
	}
}

// TestFeedGroupListAlwaysSendsPageToken locks the fix for the groups endpoint's
// requirement that page_token be present even on the first page (HTTP 400
// "Missing required parameter: page_token" otherwise).
func TestFeedGroupListAlwaysSendsPageToken(t *testing.T) {
	var reqs []recordedFGRequest
	runtime := newFGRuntime(t, ImFeedGroupList, map[string]string{"page-size": "10"}, &reqs,
		func(_ string, _ int) (int, interface{}) {
			return 200, wrapData(map[string]interface{}{
				"groups": []interface{}{}, "deleted_groups": []interface{}{},
				"page_token": "", "has_more": false,
			})
		})

	if err := ImFeedGroupList.Execute(context.Background(), runtime); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	req := findFGRequest(reqs, "/groups")
	if req == nil {
		t.Fatal("no /groups request recorded")
	}
	if _, ok := req.query["page_token"]; !ok {
		t.Errorf("first request must carry page_token query param (empty = first page); query=%v", req.query)
	}
}

// TestFeedGroupListValidation checks flag validation surfaces clear errors.
func TestFeedGroupListValidation(t *testing.T) {
	cases := []struct {
		name  string
		flags map[string]string
		want  string
	}{
		{"page-size too small", map[string]string{"page-size": "0"}, "--page-size"},
		{"page-size too large", map[string]string{"page-size": "51"}, "--page-size"},
		{"page-limit too large", map[string]string{"page-limit": "1001"}, "--page-limit"},
		{"bad start-time", map[string]string{"start-time": "notnum"}, "--start-time"},
		{"bad end-time", map[string]string{"end-time": "notnum"}, "--end-time"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runtime := newFGRuntime(t, ImFeedGroupList, tc.flags, nil, nil)
			err := ImFeedGroupList.Validate(context.Background(), runtime)
			if err == nil {
				t.Fatalf("expected validation error containing %q, got nil", tc.want)
			}
			if !bytes.Contains([]byte(err.Error()), []byte(tc.want)) {
				t.Errorf("error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

// TestFeedGroupListSinglePageTableOutput covers the non-page-all Execute path:
// time-window params must reach the query, and the pretty table must render
// group_id / name / type with the summary, pagination hint and deleted count.
func TestFeedGroupListSinglePageTableOutput(t *testing.T) {
	var reqs []recordedFGRequest
	runtime := newFGRuntime(t, ImFeedGroupList, map[string]string{
		"start-time": "100", "end-time": "200",
	}, &reqs, func(_ string, _ int) (int, interface{}) {
		return 200, wrapData(map[string]interface{}{
			"groups":         []interface{}{fgGroup("g1"), fgGroup("g2")},
			"deleted_groups": []interface{}{fgGroup("d1")},
			"page_token":     "TKN", "has_more": true,
		})
	})
	runtime.Format = "pretty"

	if err := ImFeedGroupList.Execute(context.Background(), runtime); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := countFGRequests(reqs, "/groups"); got != 1 {
		t.Fatalf("expected 1 groups request, got %d", got)
	}
	if got := firstQueryValue(reqs[0].query, "start_time"); got != "100" {
		t.Errorf("start_time query = %q, want 100", got)
	}
	if got := firstQueryValue(reqs[0].query, "end_time"); got != "200" {
		t.Errorf("end_time query = %q, want 200", got)
	}

	out, _ := runtime.Factory.IOStreams.Out.(*bytes.Buffer)
	if out == nil {
		t.Fatal("stdout buffer missing")
	}
	got := out.String()
	for _, want := range []string{"group_id", "g1", "g2", "2 group(s)", "more available", "(1 deleted)"} {
		if !strings.Contains(got, want) {
			t.Errorf("table output missing %q; got:\n%s", want, got)
		}
	}
}

// TestFeedGroupListDryRun locks the dry-run shape: GET /groups with page_size,
// page_token (always present) and the optional time-window params.
func TestFeedGroupListDryRun(t *testing.T) {
	runtime := newFGRuntime(t, ImFeedGroupList, map[string]string{
		"page-size": "10", "page-token": "TKN", "start-time": "100", "end-time": "200",
	}, nil, nil)
	d := ImFeedGroupList.DryRun(context.Background(), runtime)
	calls := dryRunCalls(t, d)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0]["method"] != "GET" {
		t.Errorf("method = %v, want GET", calls[0]["method"])
	}
	if url, _ := calls[0]["url"].(string); !strings.HasSuffix(url, "/open-apis/im/v1/groups") {
		t.Errorf("url = %s", url)
	}
	params, _ := calls[0]["params"].(map[string]interface{})
	for key, want := range map[string]string{
		"page_size": "10", "page_token": "TKN", "start_time": "100", "end_time": "200",
	} {
		if params[key] != want {
			t.Errorf("params %s = %v, want %s", key, params[key], want)
		}
	}
}

func TestFeedGroupListDryRunValidationError(t *testing.T) {
	runtime := newFGRuntime(t, ImFeedGroupList, map[string]string{"page-size": "0"}, nil, nil)
	d := ImFeedGroupList.DryRun(context.Background(), runtime)
	m := dryRunJSON(t, d)
	errMsg, _ := m["error"].(string)
	if !strings.Contains(errMsg, "--page-size") {
		t.Errorf("dry-run error = %q, want --page-size validation message", errMsg)
	}
}

// TestFeedGroupListPageAllStopsOnRepeatedToken locks the infinite-loop guard:
// when the server keeps returning the same page_token with has_more=true,
// pagination must stop after the repeat and warn on stderr. Also exercises the
// defensive page-limit clamping (Execute is called directly, bypassing Validate).
func TestFeedGroupListPageAllStopsOnRepeatedToken(t *testing.T) {
	for _, tc := range []struct {
		name      string
		pageLimit string
	}{
		{"limit clamped up from 0", "0"},
		{"limit clamped down from 1001", "1001"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var reqs []recordedFGRequest
			runtime := newFGRuntime(t, ImFeedGroupList, map[string]string{
				"page-all": "true", "page-limit": tc.pageLimit, "start-time": "100", "end-time": "200",
			}, &reqs, func(_ string, _ int) (int, interface{}) {
				return 200, wrapData(map[string]interface{}{
					"groups":         []interface{}{fgGroup("g1")},
					"deleted_groups": []interface{}{},
					"page_token":     "SAME", "has_more": true,
				})
			})
			runtime.Format = "pretty" // exercise the page-all table-render path too
			if err := ImFeedGroupList.Execute(context.Background(), runtime); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if got := countFGRequests(reqs, "/groups"); got != 2 {
				t.Errorf("expected 2 requests (stop on repeated token), got %d", got)
			}
			errOut, _ := runtime.Factory.IOStreams.ErrOut.(*bytes.Buffer)
			if !strings.Contains(errOut.String(), "page_token did not change") {
				t.Errorf("stderr missing loop warning; got:\n%s", errOut.String())
			}
		})
	}
}

// TestFeedGroupListAPIError checks both Execute paths surface API errors.
func TestFeedGroupListAPIError(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags map[string]string
	}{
		{"single page", map[string]string{}},
		{"page-all", map[string]string{"page-all": "true"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime := newFGRuntime(t, ImFeedGroupList, tc.flags, nil,
				func(_ string, _ int) (int, interface{}) {
					return 200, map[string]interface{}{"code": 99999, "msg": "boom"}
				})
			if err := ImFeedGroupList.Execute(context.Background(), runtime); err == nil {
				t.Fatal("expected API error, got nil")
			}
		})
	}
}
