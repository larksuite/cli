// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// ── +space-list ──────────────────────────────────────────────────────────────

func TestWikiShortcutsIncludesSpaceListNodeListNodeCopy(t *testing.T) {
	t.Parallel()

	commands := map[string]bool{}
	for _, s := range Shortcuts() {
		commands[s.Command] = true
	}
	for _, want := range []string{"+space-list", "+node-list", "+node-copy"} {
		if !commands[want] {
			t.Errorf("Shortcuts() missing %q", want)
		}
	}
}

// TestWikiListShortcutsDeclareNarrowScopes pins the per-endpoint scope
// choice. The framework's preflight does exact string matching, so a broad
// scope (e.g. wiki:wiki:readonly) would wrongly reject tokens carrying only
// the narrow per-API scope that the API actually accepts.
func TestWikiListShortcutsDeclareNarrowScopes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		shortcut common.Shortcut
		want     []string
	}{
		{"+space-list", WikiSpaceList, []string{"wiki:space:retrieve"}},
		{"+node-list", WikiNodeList, []string{"wiki:node:retrieve"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !reflect.DeepEqual(tc.shortcut.Scopes, tc.want) {
				t.Fatalf("%s scopes = %v, want %v", tc.name, tc.shortcut.Scopes, tc.want)
			}
		})
	}
}

func TestWikiSpaceListReturnsPaginatedSpaces(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"has_more": false,
				"items": []interface{}{
					map[string]interface{}{
						"space_id":   "space_1",
						"name":       "Engineering Wiki",
						"space_type": "team",
					},
					map[string]interface{}{
						"space_id":   "space_2",
						"name":       "Personal Library",
						"space_type": "my_library",
					},
				},
			},
			"msg": "success",
		},
	})

	err := mountAndRunWiki(t, WikiSpaceList, []string{"+space-list", "--as", "bot"}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Spaces    []map[string]interface{} `json:"spaces"`
			HasMore   bool                     `json:"has_more"`
			PageToken string                   `json:"page_token"`
		} `json:"data"`
		Meta struct {
			Count float64 `json:"count"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal stdout: %v", err)
	}
	if !envelope.OK {
		t.Fatalf("expected ok=true, got %s", stdout.String())
	}
	if envelope.Meta.Count != 2 {
		t.Fatalf("meta.count = %v, want 2", envelope.Meta.Count)
	}
	if envelope.Data.HasMore {
		t.Fatalf("has_more = true, want false on natural end")
	}
	if envelope.Data.Spaces[0]["name"] != "Engineering Wiki" {
		t.Fatalf("spaces[0].name = %v, want %q", envelope.Data.Spaces[0]["name"], "Engineering Wiki")
	}
}

// ── +node-list ───────────────────────────────────────────────────────────────

func TestWikiNodeListRequiresSpaceID(t *testing.T) {
	t.Parallel()

	factory, _, _, _ := cmdutil.TestFactory(t, wikiTestConfig())
	err := mountAndRunWiki(t, WikiNodeList, []string{"+node-list", "--as", "user"}, factory, nil)
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected required flag error, got %v", err)
	}
}

func TestWikiNodeListReturnsNodesForSpace(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/space_123/nodes",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"has_more": false,
				"items": []interface{}{
					map[string]interface{}{
						"space_id":          "space_123",
						"node_token":        "wik_node_1",
						"obj_token":         "docx_1",
						"obj_type":          "docx",
						"parent_node_token": "",
						"node_type":         "origin",
						"title":             "Getting Started",
						"has_child":         true,
					},
					map[string]interface{}{
						"space_id":          "space_123",
						"node_token":        "wik_node_2",
						"obj_token":         "docx_2",
						"obj_type":          "docx",
						"parent_node_token": "",
						"node_type":         "origin",
						"title":             "Architecture",
						"has_child":         false,
					},
				},
			},
			"msg": "success",
		},
	})

	err := mountAndRunWiki(t, WikiNodeList, []string{
		"+node-list", "--space-id", "space_123", "--as", "bot",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Nodes     []map[string]interface{} `json:"nodes"`
			HasMore   bool                     `json:"has_more"`
			PageToken string                   `json:"page_token"`
		} `json:"data"`
		Meta struct {
			Count float64 `json:"count"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal stdout: %v", err)
	}
	if !envelope.OK {
		t.Fatalf("expected ok=true, got %s", stdout.String())
	}
	if envelope.Meta.Count != 2 {
		t.Fatalf("meta.count = %v, want 2", envelope.Meta.Count)
	}
	if envelope.Data.Nodes[0]["title"] != "Getting Started" {
		t.Fatalf("nodes[0].title = %v, want %q", envelope.Data.Nodes[0]["title"], "Getting Started")
	}
	if envelope.Data.Nodes[0]["has_child"] != true {
		t.Fatalf("nodes[0].has_child = %v, want true", envelope.Data.Nodes[0]["has_child"])
	}
}

func TestWikiNodeListPassesParentNodeToken(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())

	stub := &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/space_123/nodes?page_size=50&parent_node_token=wik_parent",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"has_more": false,
				"items": []interface{}{
					map[string]interface{}{
						"space_id":          "space_123",
						"node_token":        "wik_child",
						"obj_token":         "docx_child",
						"obj_type":          "docx",
						"parent_node_token": "wik_parent",
						"node_type":         "origin",
						"title":             "Child Doc",
						"has_child":         false,
					},
				},
			},
			"msg": "success",
		},
	}
	reg.Register(stub)

	err := mountAndRunWiki(t, WikiNodeList, []string{
		"+node-list", "--space-id", "space_123", "--parent-node-token", "wik_parent", "--as", "bot",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	// Verify the correct node was returned (parent_node_token was passed correctly).
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Nodes []map[string]interface{} `json:"nodes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal stdout: %v", err)
	}
	if !envelope.OK {
		t.Fatalf("expected ok=true, got %s", stdout.String())
	}
	if len(envelope.Data.Nodes) != 1 {
		t.Fatalf("len(nodes) = %d, want 1", len(envelope.Data.Nodes))
	}
	if envelope.Data.Nodes[0]["parent_node_token"] != "wik_parent" {
		t.Fatalf("nodes[0].parent_node_token = %v, want %q", envelope.Data.Nodes[0]["parent_node_token"], "wik_parent")
	}
}

func TestWikiNodeListRejectsMyLibraryForBot(t *testing.T) {
	t.Parallel()

	factory, _, _, _ := cmdutil.TestFactory(t, wikiTestConfig())
	err := mountAndRunWiki(t, WikiNodeList, []string{
		"+node-list", "--space-id", "my_library", "--as", "bot",
	}, factory, nil)
	if err == nil || !strings.Contains(err.Error(), "bot identity does not support --space-id my_library") {
		t.Fatalf("expected my_library bot rejection, got %v", err)
	}
}

func TestWikiNodeListResolvesMyLibraryForUser(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())

	// Step 1: resolve my_library to the real space_id.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/my_library",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"space": map[string]interface{}{
					"space_id":   "space_personal_42",
					"name":       "My Library",
					"space_type": "my_library",
				},
			},
		},
	})
	// Step 2: list nodes in the resolved space.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/space_personal_42/nodes",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"has_more": false,
				"items": []interface{}{
					map[string]interface{}{
						"space_id":   "space_personal_42",
						"node_token": "wik_personal_1",
						"title":      "Personal Note",
					},
				},
			},
		},
	})

	err := mountAndRunWiki(t, WikiNodeList, []string{
		"+node-list", "--space-id", "my_library", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Nodes []map[string]interface{} `json:"nodes"`
		} `json:"data"`
		Meta struct {
			Count float64 `json:"count"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal stdout: %v", err)
	}
	if envelope.Meta.Count != 1 {
		t.Fatalf("meta.count = %v, want 1", envelope.Meta.Count)
	}
	if envelope.Data.Nodes[0]["space_id"] != "space_personal_42" {
		t.Fatalf("nodes[0].space_id = %v, want space_personal_42", envelope.Data.Nodes[0]["space_id"])
	}
}

// ── +node-copy ───────────────────────────────────────────────────────────────

func TestWikiNodeCopyRequiresTargetSpaceOrParent(t *testing.T) {
	t.Parallel()

	factory, _, _, _ := cmdutil.TestFactory(t, wikiTestConfig())
	err := mountAndRunWiki(t, WikiNodeCopy, []string{
		"+node-copy", "--space-id", "space_123", "--node-token", "wik_src", "--as", "bot",
	}, factory, nil)
	if err == nil || !strings.Contains(err.Error(), "--target-space-id or --target-parent-node-token") {
		t.Fatalf("expected target validation error, got %v", err)
	}
}

func TestWikiNodeCopyRejectsBothTargetFlags(t *testing.T) {
	t.Parallel()

	factory, _, _, _ := cmdutil.TestFactory(t, wikiTestConfig())
	err := mountAndRunWiki(t, WikiNodeCopy, []string{
		"+node-copy", "--space-id", "space_123", "--node-token", "wik_src",
		"--target-space-id", "space_dst", "--target-parent-node-token", "wik_parent",
		"--as", "bot",
	}, factory, nil)
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutually exclusive error, got %v", err)
	}
}

// TestWikiNodeCopyDeclaredHighRiskWrite pins down the high-risk-write
// contract: invocation without --yes must fail with a confirmation_required
// error and must NOT issue the underlying API call. The aligned upstream
// schema flags this API as `danger: true`, and the shortcut now matches that
// risk classification.
func TestWikiNodeCopyDeclaredHighRiskWrite(t *testing.T) {
	t.Parallel()

	if WikiNodeCopy.Risk != "high-risk-write" {
		t.Fatalf("WikiNodeCopy.Risk = %q, want %q", WikiNodeCopy.Risk, "high-risk-write")
	}

	factory, _, _, _ := cmdutil.TestFactory(t, wikiTestConfig())
	// No HTTP stub registered — if the gate leaks, the request fires and
	// httpmock errors with "no stub for POST ..." instead of the expected
	// confirmation_required error, making the regression obvious.
	err := mountAndRunWiki(t, WikiNodeCopy, []string{
		"+node-copy",
		"--space-id", "space_src",
		"--node-token", "wik_src",
		"--target-space-id", "space_dst",
		"--as", "bot",
	}, factory, nil)
	if err == nil || !strings.Contains(err.Error(), "requires confirmation") {
		t.Fatalf("expected confirmation_required error, got %v", err)
	}
}

func TestWikiNodeCopyCopiesNodeToTargetSpace(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, stderr, reg := cmdutil.TestFactory(t, wikiTestConfig())

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/wiki/v2/spaces/space_src/nodes/wik_src/copy",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"space_id":          "space_dst",
					"node_token":        "wik_copied",
					"obj_token":         "docx_copied",
					"obj_type":          "docx",
					"parent_node_token": "",
					"node_type":         "origin",
					"title":             "Architecture (Copy)",
					"has_child":         false,
				},
			},
			"msg": "success",
		},
	}
	reg.Register(stub)

	err := mountAndRunWiki(t, WikiNodeCopy, []string{
		"+node-copy",
		"--space-id", "space_src",
		"--node-token", "wik_src",
		"--target-space-id", "space_dst",
		"--title", "Architecture (Copy)",
		"--yes",
		"--as", "bot",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	var envelope struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal stdout: %v", err)
	}
	if !envelope.OK {
		t.Fatalf("expected ok=true, got %s", stdout.String())
	}
	if envelope.Data["node_token"] != "wik_copied" {
		t.Fatalf("node_token = %v, want %q", envelope.Data["node_token"], "wik_copied")
	}
	if envelope.Data["space_id"] != "space_dst" {
		t.Fatalf("space_id = %v, want %q", envelope.Data["space_id"], "space_dst")
	}

	var captured map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &captured); err != nil {
		t.Fatalf("unmarshal captured body: %v", err)
	}
	if captured["target_space_id"] != "space_dst" {
		t.Fatalf("captured target_space_id = %v, want %q", captured["target_space_id"], "space_dst")
	}
	if captured["title"] != "Architecture (Copy)" {
		t.Fatalf("captured title = %v, want %q", captured["title"], "Architecture (Copy)")
	}
	if got := stderr.String(); !strings.Contains(got, "Copying wiki node") {
		t.Fatalf("stderr = %q, want copy message", got)
	}
}

func TestWikiNodeCopyCopiesNodeToTargetParent(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/wiki/v2/spaces/space_src/nodes/wik_src/copy",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"space_id":          "space_src",
					"node_token":        "wik_copied2",
					"obj_token":         "docx_copied2",
					"obj_type":          "docx",
					"parent_node_token": "wik_parent_dst",
					"node_type":         "origin",
					"title":             "Architecture",
					"has_child":         false,
				},
			},
			"msg": "success",
		},
	}
	reg.Register(stub)

	err := mountAndRunWiki(t, WikiNodeCopy, []string{
		"+node-copy",
		"--space-id", "space_src",
		"--node-token", "wik_src",
		"--target-parent-node-token", "wik_parent_dst",
		"--yes",
		"--as", "bot",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	var captured map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &captured); err != nil {
		t.Fatalf("unmarshal captured body: %v", err)
	}
	if captured["target_parent_token"] != "wik_parent_dst" {
		t.Fatalf("captured target_parent_token = %v, want %q", captured["target_parent_token"], "wik_parent_dst")
	}
	if _, hasTitle := captured["title"]; hasTitle {
		t.Fatalf("title should not be in body when --title not provided, got %v", captured)
	}
}

// ── +space-list / +node-list pagination & format ─────────────────────────────

func TestWikiSpaceListRejectsInvalidPageSize(t *testing.T) {
	t.Parallel()

	factory, _, _, _ := cmdutil.TestFactory(t, wikiTestConfig())
	err := mountAndRunWiki(t, WikiSpaceList, []string{
		"+space-list", "--page-size", "0", "--as", "bot",
	}, factory, nil)
	if err == nil || !strings.Contains(err.Error(), "--page-size must be between 1 and 50") {
		t.Fatalf("expected page-size validation error, got %v", err)
	}
}

func TestWikiSpaceListRejectsNegativePageLimit(t *testing.T) {
	t.Parallel()

	factory, _, _, _ := cmdutil.TestFactory(t, wikiTestConfig())
	err := mountAndRunWiki(t, WikiSpaceList, []string{
		"+space-list", "--page-limit", "-1", "--as", "bot",
	}, factory, nil)
	if err == nil || !strings.Contains(err.Error(), "--page-limit must be a non-negative integer") {
		t.Fatalf("expected page-limit validation error, got %v", err)
	}
}

func TestWikiSpaceListAutoPaginatesAcrossPages(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())

	// Page 1: has_more=true, page_token set. Loop must continue.
	page1 := &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"has_more":   true,
				"page_token": "tok_page2",
				"items": []interface{}{
					map[string]interface{}{"space_id": "sp_1", "name": "First"},
				},
			},
		},
	}
	// Page 2: must receive page_token=tok_page2 in query. Captured to verify.
	var page2Query string
	page2 := &httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/wiki/v2/spaces",
		OnMatch: func(req *http.Request) { page2Query = req.URL.RawQuery },
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"has_more":   false,
				"page_token": "",
				"items": []interface{}{
					map[string]interface{}{"space_id": "sp_2", "name": "Second"},
				},
			},
		},
	}
	reg.Register(page1)
	reg.Register(page2)

	err := mountAndRunWiki(t, WikiSpaceList, []string{"+space-list", "--page-all", "--as", "bot"}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	var envelope struct {
		Data struct {
			Spaces    []map[string]interface{} `json:"spaces"`
			HasMore   bool                     `json:"has_more"`
			PageToken string                   `json:"page_token"`
		} `json:"data"`
		Meta struct {
			Count float64 `json:"count"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal stdout: %v", err)
	}
	if envelope.Meta.Count != 2 || len(envelope.Data.Spaces) != 2 {
		t.Fatalf("merged spaces = %d / count=%v, want 2 / 2", len(envelope.Data.Spaces), envelope.Meta.Count)
	}
	if envelope.Data.HasMore || envelope.Data.PageToken != "" {
		t.Fatalf("natural end should clear has_more/page_token, got has_more=%v page_token=%q", envelope.Data.HasMore, envelope.Data.PageToken)
	}
	q, _ := url.ParseQuery(page2Query)
	if q.Get("page_token") != "tok_page2" {
		t.Fatalf("page2 page_token = %q, want tok_page2", q.Get("page_token"))
	}
}

func TestWikiSpaceListPageLimitTruncatesAndExposesNextCursor(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())

	// Only stub page 1; with --page-limit=1, the loop must stop BEFORE
	// requesting page 2 — and surface has_more/page_token so the caller can resume.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"has_more":   true,
				"page_token": "tok_next",
				"items": []interface{}{
					map[string]interface{}{"space_id": "sp_only", "name": "First"},
				},
			},
		},
	})

	err := mountAndRunWiki(t, WikiSpaceList, []string{
		"+space-list", "--page-all", "--page-limit", "1", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	var envelope struct {
		Data struct {
			Spaces    []map[string]interface{} `json:"spaces"`
			HasMore   bool                     `json:"has_more"`
			PageToken string                   `json:"page_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal stdout: %v", err)
	}
	if len(envelope.Data.Spaces) != 1 {
		t.Fatalf("spaces = %d, want 1 (capped)", len(envelope.Data.Spaces))
	}
	if !envelope.Data.HasMore || envelope.Data.PageToken != "tok_next" {
		t.Fatalf("truncated state = has_more=%v page_token=%q, want true / tok_next", envelope.Data.HasMore, envelope.Data.PageToken)
	}
}

func TestWikiSpaceListExplicitPageTokenStopsAfterOnePage(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())

	// Stub a page where has_more=true; auto-pagination should NOT trigger
	// because the caller supplied an explicit --page-token cursor.
	var capturedQuery string
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/wiki/v2/spaces",
		OnMatch: func(req *http.Request) { capturedQuery = req.URL.RawQuery },
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"has_more":   true,
				"page_token": "tok_next",
				"items":      []interface{}{map[string]interface{}{"space_id": "sp_x"}},
			},
		},
	})

	err := mountAndRunWiki(t, WikiSpaceList, []string{
		"+space-list", "--page-token", "tok_input", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	q, _ := url.ParseQuery(capturedQuery)
	if q.Get("page_token") != "tok_input" {
		t.Fatalf("captured page_token = %q, want tok_input", q.Get("page_token"))
	}
}

func TestWikiSpaceListPrettyFormatRendersFields(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"has_more": false,
				"items": []interface{}{
					map[string]interface{}{
						"space_id":     "sp_1",
						"name":         "Engineering",
						"description":  "team docs",
						"space_type":   "team",
						"visibility":   "public",
						"open_sharing": "open",
					},
				},
			},
		},
	})

	err := mountAndRunWiki(t, WikiSpaceList, []string{
		"+space-list", "--format", "pretty", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	out := stdout.String()
	for _, want := range []string{
		"Engineering",
		"space_id:     sp_1",
		"space_type:   team",
		"visibility:   public",
		"description:  team docs",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("pretty output missing %q, got:\n%s", want, out)
		}
	}
}

func TestWikiNodeListDefaultIsSinglePage(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())

	// Only one stub registered; if the default tried to auto-paginate, the
	// loop would attempt a 2nd request and httpmock would error. So this
	// test pins down the "default = single page" contract.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/space_123/nodes",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"has_more":   true,
				"page_token": "tok_next",
				"items": []interface{}{
					map[string]interface{}{"space_id": "space_123", "node_token": "wik_1", "title": "First"},
				},
			},
		},
	})

	err := mountAndRunWiki(t, WikiNodeList, []string{
		"+node-list", "--space-id", "space_123", "--as", "bot",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	var envelope struct {
		Data struct {
			Nodes     []map[string]interface{} `json:"nodes"`
			HasMore   bool                     `json:"has_more"`
			PageToken string                   `json:"page_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal stdout: %v", err)
	}
	if len(envelope.Data.Nodes) != 1 {
		t.Fatalf("nodes = %d, want 1 (single page default)", len(envelope.Data.Nodes))
	}
	if !envelope.Data.HasMore || envelope.Data.PageToken != "tok_next" {
		t.Fatalf("single-page default should surface upstream cursor, got has_more=%v page_token=%q", envelope.Data.HasMore, envelope.Data.PageToken)
	}
}

func TestWikiNodeListPrettyFormatRendersFields(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/space_123/nodes",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"has_more": false,
				"items": []interface{}{
					map[string]interface{}{
						"space_id":   "space_123",
						"node_token": "wik_1",
						"obj_type":   "docx",
						"obj_token":  "docx_1",
						"title":      "Getting Started",
						"has_child":  true,
					},
				},
			},
		},
	})

	err := mountAndRunWiki(t, WikiNodeList, []string{
		"+node-list", "--space-id", "space_123", "--format", "pretty", "--as", "bot",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	out := stdout.String()
	for _, want := range []string{
		"Getting Started",
		"node_token: wik_1",
		"obj_type:   docx",
		"has_child:  true",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("pretty output missing %q, got:\n%s", want, out)
		}
	}
}

// ── QA-driven fixes: empty slice + has_more hint + node-copy format ──

func TestWikiSpaceListEmptyResultReturnsEmptySliceNotNull(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"has_more":   false,
				"page_token": "",
				"items":      []interface{}{},
			},
		},
	})

	err := mountAndRunWiki(t, WikiSpaceList, []string{"+space-list", "--as", "bot"}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	// Substring assertion is the only reliable way to distinguish [] from null
	// in serialised JSON — unmarshalling both back into a Go slice would
	// collapse the distinction.
	if !strings.Contains(stdout.String(), `"spaces": []`) {
		t.Fatalf("expected spaces to be empty array [], got:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), `"spaces": null`) {
		t.Fatalf("spaces serialised as null — JSON consumers expect []:\n%s", stdout.String())
	}

	var envelope struct {
		Meta struct {
			Count float64 `json:"count"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal stdout: %v", err)
	}
	if envelope.Meta.Count != 0 {
		t.Fatalf("meta.count = %v, want 0", envelope.Meta.Count)
	}
}

func TestWikiSpaceListPrettyHintsWhenEmptyButHasMore(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"has_more":   true,
				"page_token": "tok_more",
				"items":      []interface{}{},
			},
		},
	})

	err := mountAndRunWiki(t, WikiSpaceList, []string{"+space-list", "--format", "pretty", "--as", "bot"}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	out := stdout.String()
	// When the bot's first page is filtered out by upstream permissions, the
	// blanket "No wiki spaces found." used to mislead users into thinking they
	// had no access at all. Pretty mode must now distinguish that case.
	if strings.Contains(out, "No wiki spaces found.") {
		t.Fatalf("pretty output should not flatly claim 'No wiki spaces found.' when has_more=true; got:\n%s", out)
	}
	for _, want := range []string{
		"Current page is empty but the server reports more pages.",
		"tok_more",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("pretty output missing %q, got:\n%s", want, out)
		}
	}
}

func TestWikiNodeCopyHasFormatPrettyRendersNode(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/wiki/v2/spaces/space_src/nodes/wik_src/copy",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"space_id":          "space_dst",
					"node_token":        "wik_copied",
					"obj_token":         "docx_copied",
					"obj_type":          "docx",
					"parent_node_token": "wik_parent",
					"node_type":         "origin",
					"title":             "Architecture (Copy)",
				},
			},
		},
	})

	err := mountAndRunWiki(t, WikiNodeCopy, []string{
		"+node-copy",
		"--space-id", "space_src",
		"--node-token", "wik_src",
		"--target-space-id", "space_dst",
		"--title", "Architecture (Copy)",
		"--format", "pretty",
		"--yes",
		"--as", "bot",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	out := stdout.String()
	for _, want := range []string{
		"Copied node:",
		"title:             Architecture (Copy)",
		"node_token:        wik_copied",
		"space_id:          space_dst",
		"parent_node_token: wik_parent",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("pretty output missing %q, got:\n%s", want, out)
		}
	}
}
