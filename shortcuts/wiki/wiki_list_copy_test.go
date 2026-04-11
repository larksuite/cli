// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
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
			Spaces []map[string]interface{} `json:"spaces"`
			Total  float64                  `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal stdout: %v", err)
	}
	if !envelope.OK {
		t.Fatalf("expected ok=true, got %s", stdout.String())
	}
	if envelope.Data.Total != 2 {
		t.Fatalf("total = %v, want 2", envelope.Data.Total)
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
			Nodes []map[string]interface{} `json:"nodes"`
			Total float64                  `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal stdout: %v", err)
	}
	if !envelope.OK {
		t.Fatalf("expected ok=true, got %s", stdout.String())
	}
	if envelope.Data.Total != 2 {
		t.Fatalf("total = %v, want 2", envelope.Data.Total)
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

// ── +node-move ───────────────────────────────────────────────────────────────

func TestWikiNodeMoveRequiresTargetSpaceOrParent(t *testing.T) {
	t.Parallel()

	factory, _, _, _ := cmdutil.TestFactory(t, wikiTestConfig())
	err := mountAndRunWiki(t, WikiNodeMove, []string{
		"+node-move", "--space-id", "space_123", "--node-token", "wik_src", "--as", "bot",
	}, factory, nil)
	if err == nil || !strings.Contains(err.Error(), "--target-space-id or --target-parent-node-token") {
		t.Fatalf("expected target validation error, got %v", err)
	}
}

func TestWikiNodeMoveMovesNodeToTargetSpace(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, stderr, reg := cmdutil.TestFactory(t, wikiTestConfig())

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/wiki/v2/spaces/space_src/nodes/wik_src/move",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"space_id":          "space_dst",
					"node_token":        "wik_src",
					"obj_token":         "docx_src",
					"obj_type":          "docx",
					"parent_node_token": "",
					"node_type":         "origin",
					"title":             "Architecture",
					"has_child":         false,
				},
			},
			"msg": "success",
		},
	}
	reg.Register(stub)

	err := mountAndRunWiki(t, WikiNodeMove, []string{
		"+node-move",
		"--space-id", "space_src",
		"--node-token", "wik_src",
		"--target-space-id", "space_dst",
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
	if got := stderr.String(); !strings.Contains(got, "Moving wiki node") {
		t.Fatalf("stderr = %q, want move message", got)
	}
}

func TestWikiNodeMoveMovesNodeToTargetParent(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/wiki/v2/spaces/space_src/nodes/wik_src/move",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"space_id":          "space_src",
					"node_token":        "wik_src",
					"obj_token":         "docx_src",
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

	err := mountAndRunWiki(t, WikiNodeMove, []string{
		"+node-move",
		"--space-id", "space_src",
		"--node-token", "wik_src",
		"--target-parent-node-token", "wik_parent_dst",
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
	if _, hasSpaceID := captured["target_space_id"]; hasSpaceID {
		t.Fatalf("target_space_id should not be in body when not provided, got %v", captured)
	}
}
