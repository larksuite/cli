// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

// recordedFGRequest captures one outbound request for assertion.
type recordedFGRequest struct {
	method string
	path   string
	query  map[string][]string
	body   map[string]interface{}
}

// fgResponder maps a URL path suffix to a JSON response body.
type fgResponder func(path string, page int) (int, interface{})

// newFGCmd builds a cobra command carrying the shortcut's flags, applying the
// provided overrides.
func newFGCmd(t *testing.T, sc common.Shortcut, flags map[string]string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: sc.Command}
	for _, fl := range sc.Flags {
		switch fl.Type {
		case "bool":
			cmd.Flags().Bool(fl.Name, fl.Default == "true", fl.Desc)
		case "int":
			def := 0
			if fl.Default != "" {
				n, _ := strconv.Atoi(fl.Default)
				def = n
			}
			cmd.Flags().Int(fl.Name, def, fl.Desc)
		default:
			cmd.Flags().String(fl.Name, fl.Default, fl.Desc)
		}
	}
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	for name, val := range flags {
		if err := cmd.Flags().Set(name, val); err != nil {
			t.Fatalf("set flag %s=%s: %v", name, val, err)
		}
	}
	return cmd
}

// newFGRuntime wires a user-identity runtime with the shortcut's flags and an
// httpmock transport that records requests and replies via the responder.
func newFGRuntime(t *testing.T, sc common.Shortcut, flags map[string]string, recorded *[]recordedFGRequest, responder fgResponder) *common.RuntimeContext {
	t.Helper()
	pageByPath := map[string]int{}
	rt := shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		rec := recordedFGRequest{
			method: req.Method,
			path:   req.URL.Path,
			query:  req.URL.Query(),
		}
		if req.Body != nil {
			data, _ := io.ReadAll(req.Body)
			if len(data) > 0 {
				_ = json.Unmarshal(data, &rec.body)
			}
		}
		if recorded != nil {
			*recorded = append(*recorded, rec)
		}
		pageByPath[req.URL.Path]++
		status, body := 200, interface{}(map[string]interface{}{"code": 0, "data": map[string]interface{}{}})
		if responder != nil {
			status, body = responder(req.URL.Path, pageByPath[req.URL.Path])
		}
		return shortcutJSONResponse(status, body), nil
	})

	runtime := newUserShortcutRuntime(t, rt)
	runtime.Cmd = newFGCmd(t, sc, flags)
	runtime.Format = "json"
	return runtime
}

func wrapData(d map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"code": 0, "data": d}
}

func findFGRequest(reqs []recordedFGRequest, pathSuffix string) *recordedFGRequest {
	for i := range reqs {
		if strings.HasSuffix(reqs[i].path, pathSuffix) {
			return &reqs[i]
		}
	}
	return nil
}

func firstQueryValue(q map[string][]string, key string) string {
	if v := q[key]; len(v) > 0 {
		return v[0]
	}
	return ""
}

// dryRunJSON marshals a DryRunAPI to its wire shape so tests can assert against
// the public JSON (calls/extra are unexported on the struct).
func dryRunJSON(t *testing.T, d *common.DryRunAPI) map[string]interface{} {
	t.Helper()
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal dry-run: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal dry-run: %v", err)
	}
	return m
}

func dryRunCalls(t *testing.T, d *common.DryRunAPI) []map[string]interface{} {
	t.Helper()
	m := dryRunJSON(t, d)
	raw, _ := m["api"].([]interface{})
	calls := make([]map[string]interface{}, 0, len(raw))
	for _, c := range raw {
		cm, _ := c.(map[string]interface{})
		calls = append(calls, cm)
	}
	return calls
}

func countFGRequests(reqs []recordedFGRequest, pathSuffix string) int {
	n := 0
	for i := range reqs {
		if strings.HasSuffix(reqs[i].path, pathSuffix) {
			n++
		}
	}
	return n
}

// ── list-item: happy path with enrichment of items + deleted_items ──

func TestFeedGroupListItemEnrichesBothLists(t *testing.T) {
	var reqs []recordedFGRequest
	runtime := newFGRuntime(t, ImFeedGroupListItem, map[string]string{"feed-group-id": "ofg_x"}, &reqs,
		func(path string, _ int) (int, interface{}) {
			switch {
			case strings.HasSuffix(path, "/list_item"):
				return 200, wrapData(map[string]interface{}{
					"items":         []interface{}{map[string]interface{}{"feed_id": "oc_abc", "feed_type": "chat", "update_time": "1767196800000"}},
					"deleted_items": []interface{}{map[string]interface{}{"feed_id": "oc_def", "feed_type": "chat", "update_time": "1767196800000"}},
					"page_token":    "",
					"has_more":      false,
				})
			case strings.HasSuffix(path, "/chats/batch_query"):
				return 200, wrapData(map[string]interface{}{"items": []interface{}{
					map[string]interface{}{"chat_id": "oc_abc", "name": "Release Team"},
					map[string]interface{}{"chat_id": "oc_def", "name": "Old Channel"},
				}})
			}
			return 200, wrapData(map[string]interface{}{})
		})

	if err := ImFeedGroupListItem.Execute(context.Background(), runtime); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	list := findFGRequest(reqs, "/list_item")
	if list == nil {
		t.Fatal("expected list_item request")
	}
	if list.method != http.MethodGet {
		t.Errorf("list_item method = %s, want GET", list.method)
	}
	if !strings.HasSuffix(list.path, "/open-apis/im/v1/groups/ofg_x/list_item") {
		t.Errorf("list_item path = %s", list.path)
	}
	if findFGRequest(reqs, "/chats/batch_query") == nil {
		t.Error("expected chats/batch_query enrichment request")
	}
}

// ── list-item: empty items skips enrichment ──

func TestFeedGroupListItemEmptySkipsEnrichment(t *testing.T) {
	var reqs []recordedFGRequest
	runtime := newFGRuntime(t, ImFeedGroupListItem, map[string]string{"feed-group-id": "ofg_x"}, &reqs,
		func(path string, _ int) (int, interface{}) {
			if strings.HasSuffix(path, "/list_item") {
				return 200, wrapData(map[string]interface{}{
					"items": []interface{}{}, "deleted_items": []interface{}{},
					"page_token": "", "has_more": false,
				})
			}
			return 200, wrapData(map[string]interface{}{})
		})
	if err := ImFeedGroupListItem.Execute(context.Background(), runtime); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if findFGRequest(reqs, "/chats/batch_query") != nil {
		t.Error("did not expect batch_query when there are no items")
	}
}

// ── list-item: page-all merges across 2 pages, empty deleted serializes as [] ──

func TestFeedGroupListItemPageAllMerges(t *testing.T) {
	var reqs []recordedFGRequest
	runtime := newFGRuntime(t, ImFeedGroupListItem, map[string]string{"feed-group-id": "ofg_x", "page-all": "true"}, &reqs,
		func(path string, page int) (int, interface{}) {
			if strings.HasSuffix(path, "/list_item") {
				if page == 1 {
					return 200, wrapData(map[string]interface{}{
						"items":         []interface{}{map[string]interface{}{"feed_id": "oc_a", "feed_type": "chat", "update_time": "1"}},
						"deleted_items": []interface{}{},
						"page_token":    "TKN", "has_more": true,
					})
				}
				return 200, wrapData(map[string]interface{}{
					"items":         []interface{}{map[string]interface{}{"feed_id": "oc_b", "feed_type": "chat", "update_time": "2"}},
					"deleted_items": []interface{}{},
					"page_token":    "", "has_more": false,
				})
			}
			if strings.HasSuffix(path, "/chats/batch_query") {
				return 200, wrapData(map[string]interface{}{"items": []interface{}{
					map[string]interface{}{"chat_id": "oc_a", "name": "A"},
					map[string]interface{}{"chat_id": "oc_b", "name": "B"},
				}})
			}
			return 200, wrapData(map[string]interface{}{})
		})
	if err := ImFeedGroupListItem.Execute(context.Background(), runtime); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if got := countFGRequests(reqs, "/list_item"); got != 2 {
		t.Errorf("expected 2 list_item requests, got %d", got)
	}
	// Second list_item page must carry the continuation token.
	var second *recordedFGRequest
	n := 0
	for i := range reqs {
		if strings.HasSuffix(reqs[i].path, "/list_item") {
			n++
			if n == 2 {
				second = &reqs[i]
			}
		}
	}
	if second == nil || firstQueryValue(second.query, "page_token") != "TKN" {
		t.Errorf("second page token = %q, want TKN", firstQueryValue(second.query, "page_token"))
	}
}

// ── list-item: explicit page-token ignores page-all (single page) ──

func TestFeedGroupListItemPageTokenIgnoresPageAll(t *testing.T) {
	var reqs []recordedFGRequest
	runtime := newFGRuntime(t, ImFeedGroupListItem, map[string]string{
		"feed-group-id": "ofg_x", "page-all": "true", "page-token": "SOMETOKEN",
	}, &reqs, func(path string, _ int) (int, interface{}) {
		if strings.HasSuffix(path, "/list_item") {
			return 200, wrapData(map[string]interface{}{
				"items": []interface{}{}, "deleted_items": []interface{}{},
				"page_token": "NEXT", "has_more": true,
			})
		}
		return 200, wrapData(map[string]interface{}{})
	})
	if err := ImFeedGroupListItem.Execute(context.Background(), runtime); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if got := countFGRequests(reqs, "/list_item"); got != 1 {
		t.Errorf("expected 1 list_item request (page-token wins), got %d", got)
	}
	req := findFGRequest(reqs, "/list_item")
	if got := firstQueryValue(req.query, "page_token"); got != "SOMETOKEN" {
		t.Errorf("page_token query = %q, want SOMETOKEN", got)
	}
}

// ── query-item: builds correct body and enriches ──

func TestFeedGroupQueryItemBuildsBody(t *testing.T) {
	var reqs []recordedFGRequest
	runtime := newFGRuntime(t, ImFeedGroupQueryItem, map[string]string{
		"feed-group-id": "ofg_x", "feed-id": "oc_a,oc_b",
	}, &reqs, func(path string, _ int) (int, interface{}) {
		switch {
		case strings.HasSuffix(path, "/batch_query_item"):
			return 200, wrapData(map[string]interface{}{
				"items":         []interface{}{map[string]interface{}{"feed_id": "oc_a", "feed_type": "chat", "update_time": "1"}},
				"deleted_items": []interface{}{},
			})
		case strings.HasSuffix(path, "/chats/batch_query"):
			return 200, wrapData(map[string]interface{}{"items": []interface{}{
				map[string]interface{}{"chat_id": "oc_a", "name": "Team A"},
			}})
		}
		return 200, wrapData(map[string]interface{}{})
	})
	if err := ImFeedGroupQueryItem.Execute(context.Background(), runtime); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	req := findFGRequest(reqs, "/batch_query_item")
	if req == nil {
		t.Fatal("expected batch_query_item request")
	}
	if req.method != http.MethodPost {
		t.Errorf("method = %s, want POST", req.method)
	}
	if !strings.HasSuffix(req.path, "/open-apis/im/v1/groups/ofg_x/batch_query_item") {
		t.Errorf("path = %s", req.path)
	}
	items, ok := req.body["items"].([]interface{})
	if !ok || len(items) != 2 {
		t.Fatalf("body items = %#v, want 2 entries", req.body["items"])
	}
	first, _ := items[0].(map[string]interface{})
	if first["feed_id"] != "oc_a" || first["feed_type"] != "chat" {
		t.Errorf("first item = %#v", first)
	}
}

// ── table output: renders feed_id / chat_name / update_time + summary lines ──

func TestFeedGroupListItemTableOutput(t *testing.T) {
	runtime := newFGRuntime(t, ImFeedGroupListItem, map[string]string{"feed-group-id": "ofg_x"}, nil,
		func(path string, _ int) (int, interface{}) {
			switch {
			case strings.HasSuffix(path, "/list_item"):
				return 200, wrapData(map[string]interface{}{
					"items":         []interface{}{map[string]interface{}{"feed_id": "oc_abc", "feed_type": "chat", "update_time": "1767196800000"}},
					"deleted_items": []interface{}{map[string]interface{}{"feed_id": "oc_def", "feed_type": "chat", "update_time": "1767196800000"}},
					"page_token":    "TKN", "has_more": true,
				})
			case strings.HasSuffix(path, "/chats/batch_query"):
				return 200, wrapData(map[string]interface{}{"items": []interface{}{
					map[string]interface{}{"chat_id": "oc_abc", "name": "Release Team"},
				}})
			}
			return 200, wrapData(map[string]interface{}{})
		})
	runtime.Format = "pretty"

	if err := ImFeedGroupListItem.Execute(context.Background(), runtime); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	out, _ := runtime.Factory.IOStreams.Out.(*bytes.Buffer)
	if out == nil {
		t.Fatal("stdout buffer missing")
	}
	got := out.String()
	for _, want := range []string{"feed_id", "chat_name", "update_time", "oc_abc", "Release Team", "1 item(s)", "more available", "(1 deleted)"} {
		if !strings.Contains(got, want) {
			t.Errorf("table output missing %q; got:\n%s", want, got)
		}
	}
	// update_time must be rendered human-readable (RFC3339), not as raw Unix millis.
	if strings.Contains(got, "1767196800000") {
		t.Errorf("table output should not contain raw millis timestamp; got:\n%s", got)
	}
	wantTime := time.UnixMilli(1767196800000).Local().Format(time.RFC3339)
	if !strings.Contains(got, wantTime) {
		t.Errorf("table output should contain formatted update_time %q; got:\n%s", wantTime, got)
	}
}

// ── enrichment graceful degradation: unresolved feed_id keeps no chat_name ──

func TestEnrichFeedGroupItemsGracefulDegradation(t *testing.T) {
	runtime := newFGRuntime(t, ImFeedGroupQueryItem, map[string]string{
		"feed-group-id": "ofg_x", "feed-id": "oc_known",
	}, nil, func(path string, _ int) (int, interface{}) {
		if strings.HasSuffix(path, "/chats/batch_query") {
			// Only oc_known resolves; oc_gone is absent.
			return 200, wrapData(map[string]interface{}{"items": []interface{}{
				map[string]interface{}{"chat_id": "oc_known", "name": "Known"},
			}})
		}
		return 200, wrapData(map[string]interface{}{})
	})
	data := map[string]any{
		"items": []any{
			map[string]any{"feed_id": "oc_known", "feed_type": "chat"},
			map[string]any{"feed_id": "oc_gone", "feed_type": "chat"},
		},
		"deleted_items": []any{},
	}
	enrichFeedGroupItemsChatName(runtime, data)
	items := data["items"].([]any)
	known := items[0].(map[string]any)
	gone := items[1].(map[string]any)
	if known["chat_name"] != "Known" {
		t.Errorf("oc_known chat_name = %v, want Known", known["chat_name"])
	}
	if _, present := gone["chat_name"]; present {
		t.Errorf("oc_gone should not have chat_name, got %v", gone["chat_name"])
	}
}

// ── validation errors ──

func TestFeedGroupValidationErrors(t *testing.T) {
	cases := []struct {
		name  string
		sc    common.Shortcut
		flags map[string]string
		want  string
	}{
		{"list missing feed-group-id", ImFeedGroupListItem, map[string]string{}, "--feed-group-id is required"},
		{"list bad page-size", ImFeedGroupListItem, map[string]string{"feed-group-id": "ofg_x", "page-size": "0"}, "--page-size must be an integer between 1 and 50"},
		{"list bad page-limit", ImFeedGroupListItem, map[string]string{"feed-group-id": "ofg_x", "page-limit": "2000"}, "--page-limit must be an integer between 1 and 1000"},
		{"query missing feed-group-id", ImFeedGroupQueryItem, map[string]string{"feed-id": "oc_a"}, "--feed-group-id is required"},
		{"query missing feed-id", ImFeedGroupQueryItem, map[string]string{"feed-group-id": "ofg_x"}, "--feed-id is required (comma-separated chat IDs)"},
		{"query blank feed-id tokens", ImFeedGroupQueryItem, map[string]string{"feed-group-id": "ofg_x", "feed-id": ", ,"}, "--feed-id is required (comma-separated chat IDs)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runtime := newFGRuntime(t, tc.sc, tc.flags, nil, nil)
			err := tc.sc.Validate(context.Background(), runtime)
			if err == nil {
				t.Fatalf("expected validation error %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want contains %q", err.Error(), tc.want)
			}
		})
	}
}

// ── dry-run shapes ──

func TestFeedGroupListItemDryRun(t *testing.T) {
	runtime := newFGRuntime(t, ImFeedGroupListItem, map[string]string{
		"feed-group-id": "ofg_x", "page-size": "10", "start-time": "100",
	}, nil, nil)
	d := ImFeedGroupListItem.DryRun(context.Background(), runtime)
	calls := dryRunCalls(t, d)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0]["method"] != "GET" {
		t.Errorf("method = %v, want GET", calls[0]["method"])
	}
	if url, _ := calls[0]["url"].(string); !strings.HasSuffix(url, "/groups/ofg_x/list_item") {
		t.Errorf("url = %s", url)
	}
	params, _ := calls[0]["params"].(map[string]interface{})
	if params["page_size"] != "10" {
		t.Errorf("params page_size = %v, want 10", params["page_size"])
	}
	if params["start_time"] != "100" {
		t.Errorf("params start_time = %v, want 100", params["start_time"])
	}
	if desc, _ := calls[0]["desc"].(string); !strings.Contains(desc, "im:chat:read") {
		t.Errorf("desc = %q, want chat_name enrichment note", desc)
	}
}

func TestFeedGroupListItemDryRunValidationError(t *testing.T) {
	runtime := newFGRuntime(t, ImFeedGroupListItem, map[string]string{}, nil, nil)
	d := ImFeedGroupListItem.DryRun(context.Background(), runtime)
	m := dryRunJSON(t, d)
	errMsg, _ := m["error"].(string)
	if errMsg == "" {
		t.Fatalf("expected error in dry-run output, got %#v", m)
	}
	if !strings.Contains(errMsg, "--feed-group-id is required") {
		t.Errorf("error = %v", errMsg)
	}
}

func TestFeedGroupQueryItemDryRun(t *testing.T) {
	runtime := newFGRuntime(t, ImFeedGroupQueryItem, map[string]string{
		"feed-group-id": "ofg_x", "feed-id": "oc_a,oc_b",
	}, nil, nil)
	d := ImFeedGroupQueryItem.DryRun(context.Background(), runtime)
	calls := dryRunCalls(t, d)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0]["method"] != "POST" {
		t.Errorf("method = %v, want POST", calls[0]["method"])
	}
	if url, _ := calls[0]["url"].(string); !strings.HasSuffix(url, "/groups/ofg_x/batch_query_item") {
		t.Errorf("url = %s", url)
	}
	body, _ := calls[0]["body"].(map[string]interface{})
	items, _ := body["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("dry-run body items = %#v, want 2", body["items"])
	}
}

func TestFeedGroupQueryItemDryRunValidationError(t *testing.T) {
	runtime := newFGRuntime(t, ImFeedGroupQueryItem, map[string]string{"feed-group-id": "ofg_x"}, nil, nil)
	d := ImFeedGroupQueryItem.DryRun(context.Background(), runtime)
	m := dryRunJSON(t, d)
	if errMsg, _ := m["error"].(string); errMsg == "" {
		t.Fatalf("expected error in dry-run output, got %#v", m)
	}
}
