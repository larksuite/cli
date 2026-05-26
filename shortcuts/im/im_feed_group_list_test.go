// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"encoding/json"
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
