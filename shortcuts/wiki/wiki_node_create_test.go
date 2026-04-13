// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

type fakeWikiNodeCreateCall struct {
	SpaceID string
	Spec    wikiNodeCreateSpec
}

type fakeWikiNodeCreateClient struct {
	spaces        map[string]*wikiSpaceRecord
	nodes         map[string]*wikiNodeRecord
	createNode    *wikiNodeRecord
	createErr     error
	getSpaceErr   error
	getNodeErr    error
	createInvoked []fakeWikiNodeCreateCall
}

func (fake *fakeWikiNodeCreateClient) GetNode(ctx context.Context, token string) (*wikiNodeRecord, error) {
	if fake.getNodeErr != nil {
		return nil, fake.getNodeErr
	}
	node, ok := fake.nodes[token]
	if !ok {
		return &wikiNodeRecord{}, nil
	}
	return node, nil
}

func (fake *fakeWikiNodeCreateClient) GetSpace(ctx context.Context, spaceID string) (*wikiSpaceRecord, error) {
	if fake.getSpaceErr != nil {
		return nil, fake.getSpaceErr
	}
	space, ok := fake.spaces[spaceID]
	if !ok {
		return &wikiSpaceRecord{}, nil
	}
	return space, nil
}

func (fake *fakeWikiNodeCreateClient) CreateNode(ctx context.Context, spaceID string, spec wikiNodeCreateSpec) (*wikiNodeRecord, error) {
	fake.createInvoked = append(fake.createInvoked, fakeWikiNodeCreateCall{
		SpaceID: spaceID,
		Spec:    spec,
	})
	if fake.createErr != nil {
		return nil, fake.createErr
	}
	if fake.createNode != nil {
		return fake.createNode, nil
	}
	return &wikiNodeRecord{SpaceID: spaceID, Title: spec.Title, NodeType: spec.NodeType, ObjType: spec.ObjType}, nil
}

var wikiTestConfigSeq atomic.Int64

func wikiTestConfig() *core.CliConfig {
	return &core.CliConfig{
		AppID:     fmt.Sprintf("wiki-test-app-%d", wikiTestConfigSeq.Add(1)),
		AppSecret: "test-secret",
		Brand:     core.BrandFeishu,
	}
}

func mountAndRunWiki(t *testing.T, shortcut common.Shortcut, args []string, factory *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	parent := &cobra.Command{Use: "wiki"}
	shortcut.Mount(parent, factory)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

func TestWikiShortcutsIncludeMoveAndNodeCreate(t *testing.T) {
	t.Parallel()

	shortcuts := Shortcuts()
	if len(shortcuts) != 2 {
		t.Fatalf("len(Shortcuts()) = %d, want 2", len(shortcuts))
	}
	if shortcuts[0].Command != "+move" {
		t.Fatalf("shortcuts[0].Command = %q, want %q", shortcuts[0].Command, "+move")
	}
	if shortcuts[1].Command != "+node-create" {
		t.Fatalf("shortcuts[1].Command = %q, want %q", shortcuts[1].Command, "+node-create")
	}
}

func TestValidateWikiNodeCreateSpecRejectsShortcutWithoutOriginNodeToken(t *testing.T) {
	t.Parallel()

	err := validateWikiNodeCreateSpec(wikiNodeCreateSpec{
		NodeType: wikiNodeTypeShortcut,
		ObjType:  "docx",
	}, core.AsUser)
	if err == nil || !strings.Contains(err.Error(), "--origin-node-token is required") {
		t.Fatalf("expected shortcut origin-token error, got %v", err)
	}
}

func TestValidateWikiNodeCreateSpecRejectsOriginTokenForOriginNode(t *testing.T) {
	t.Parallel()

	err := validateWikiNodeCreateSpec(wikiNodeCreateSpec{
		NodeType:        wikiNodeTypeOrigin,
		ObjType:         "docx",
		OriginNodeToken: "wik_origin",
	}, core.AsUser)
	if err == nil || !strings.Contains(err.Error(), "can only be used when --node-type=shortcut") {
		t.Fatalf("expected origin-node-token validation error, got %v", err)
	}
}

func TestValidateWikiNodeCreateSpecRejectsBotWithoutLocation(t *testing.T) {
	t.Parallel()

	err := validateWikiNodeCreateSpec(wikiNodeCreateSpec{
		NodeType: wikiNodeTypeOrigin,
		ObjType:  "docx",
	}, core.AsBot)
	if err == nil || !strings.Contains(err.Error(), "bot identity requires --space-id or --parent-node-token") {
		t.Fatalf("expected bot location validation error, got %v", err)
	}
}

func TestValidateWikiNodeCreateSpecRejectsBotMyLibrarySpaceID(t *testing.T) {
	t.Parallel()

	err := validateWikiNodeCreateSpec(wikiNodeCreateSpec{
		NodeType:        wikiNodeTypeOrigin,
		ObjType:         "docx",
		SpaceID:         wikiMyLibrarySpaceID,
		ParentNodeToken: "wik_parent",
	}, core.AsBot)
	if err == nil || !strings.Contains(err.Error(), "bot identity does not support --space-id my_library") {
		t.Fatalf("expected bot my_library validation error, got %v", err)
	}
}

func TestResolveWikiNodeCreateSpaceUsesParentNode(t *testing.T) {
	t.Parallel()

	client := &fakeWikiNodeCreateClient{
		nodes: map[string]*wikiNodeRecord{
			"wik_parent": {SpaceID: "space_parent"},
		},
	}

	resolved, err := resolveWikiNodeCreateSpace(context.Background(), client, core.AsUser, wikiNodeCreateSpec{
		NodeType:        wikiNodeTypeOrigin,
		ObjType:         "docx",
		ParentNodeToken: "wik_parent",
	})
	if err != nil {
		t.Fatalf("resolveWikiNodeCreateSpace() error = %v", err)
	}
	if resolved.SpaceID != "space_parent" {
		t.Fatalf("resolved space_id = %q, want %q", resolved.SpaceID, "space_parent")
	}
	if resolved.ResolvedBy != wikiResolvedByParentNode {
		t.Fatalf("resolved_by = %q, want %q", resolved.ResolvedBy, wikiResolvedByParentNode)
	}
}

func TestResolveWikiNodeCreateSpaceRejectsSpaceMismatch(t *testing.T) {
	t.Parallel()

	client := &fakeWikiNodeCreateClient{
		nodes: map[string]*wikiNodeRecord{
			"wik_parent": {SpaceID: "space_parent"},
		},
	}

	_, err := resolveWikiNodeCreateSpace(context.Background(), client, core.AsUser, wikiNodeCreateSpec{
		NodeType:        wikiNodeTypeOrigin,
		ObjType:         "docx",
		SpaceID:         "space_other",
		ParentNodeToken: "wik_parent",
	})
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected mismatch error, got %v", err)
	}
}

func TestResolveWikiNodeCreateSpaceUsesMyLibraryFallback(t *testing.T) {
	t.Parallel()

	client := &fakeWikiNodeCreateClient{
		spaces: map[string]*wikiSpaceRecord{
			wikiMyLibrarySpaceID: {SpaceID: "space_my_library", SpaceType: "my_library"},
		},
	}

	resolved, err := resolveWikiNodeCreateSpace(context.Background(), client, core.AsUser, wikiNodeCreateSpec{
		NodeType: wikiNodeTypeOrigin,
		ObjType:  "docx",
	})
	if err != nil {
		t.Fatalf("resolveWikiNodeCreateSpace() error = %v", err)
	}
	if resolved.SpaceID != "space_my_library" {
		t.Fatalf("resolved space_id = %q, want %q", resolved.SpaceID, "space_my_library")
	}
	if resolved.ResolvedBy != wikiResolvedByMyLibrary {
		t.Fatalf("resolved_by = %q, want %q", resolved.ResolvedBy, wikiResolvedByMyLibrary)
	}
}

func TestRunWikiNodeCreateCreatesNodeInResolvedSpace(t *testing.T) {
	t.Parallel()

	client := &fakeWikiNodeCreateClient{
		spaces: map[string]*wikiSpaceRecord{
			wikiMyLibrarySpaceID: {SpaceID: "space_my_library"},
		},
		createNode: &wikiNodeRecord{
			SpaceID:   "space_my_library",
			NodeToken: "wik_created",
			NodeType:  wikiNodeTypeOrigin,
			ObjType:   "docx",
			Title:     "Roadmap",
		},
	}

	spec := wikiNodeCreateSpec{
		NodeType: wikiNodeTypeOrigin,
		ObjType:  "docx",
		Title:    "Roadmap",
	}
	execution, err := runWikiNodeCreate(context.Background(), client, core.AsUser, spec)
	if err != nil {
		t.Fatalf("runWikiNodeCreate() error = %v", err)
	}
	if len(client.createInvoked) != 1 {
		t.Fatalf("create invoked %d times, want 1", len(client.createInvoked))
	}
	if client.createInvoked[0].SpaceID != "space_my_library" {
		t.Fatalf("create space_id = %q, want %q", client.createInvoked[0].SpaceID, "space_my_library")
	}
	if execution.Node.NodeToken != "wik_created" {
		t.Fatalf("created node token = %q, want %q", execution.Node.NodeToken, "wik_created")
	}
	if execution.ResolvedSpace.ResolvedBy != wikiResolvedByMyLibrary {
		t.Fatalf("resolved_by = %q, want %q", execution.ResolvedSpace.ResolvedBy, wikiResolvedByMyLibrary)
	}
}

func TestWikiNodeCreateDryRunShowsMyLibraryLookup(t *testing.T) {
	t.Parallel()

	got := wikiNodeCreateDryRunAPIsForTest(t, func(cmd *cobra.Command) {
		if err := cmd.Flags().Set("title", "My Node"); err != nil {
			t.Fatalf("set --title: %v", err)
		}
	})

	if len(got) != 2 {
		t.Fatalf("len(dryRun.api) = %d, want 2", len(got))
	}
	if got[0].URL != "/open-apis/wiki/v2/spaces/my_library" {
		t.Fatalf("first dry-run URL = %q, want my_library lookup", got[0].URL)
	}
	if got[1].URL != "/open-apis/wiki/v2/spaces/<resolved_space_id>/nodes" {
		t.Fatalf("second dry-run URL = %q, want placeholder create URL", got[1].URL)
	}
	if got[1].Body["title"] != "My Node" {
		t.Fatalf("dry-run create body = %#v", got[1].Body)
	}
}

func TestWikiNodeCreateDryRunUsesParentNodeWithoutMyLibraryLookup(t *testing.T) {
	t.Parallel()

	got := wikiNodeCreateDryRunAPIsForTest(t, func(cmd *cobra.Command) {
		if err := cmd.Flags().Set("title", "Child Node"); err != nil {
			t.Fatalf("set --title: %v", err)
		}
		if err := cmd.Flags().Set("parent-node-token", "wik_parent"); err != nil {
			t.Fatalf("set --parent-node-token: %v", err)
		}
	})

	if len(got) != 2 {
		t.Fatalf("len(dryRun.api) = %d, want 2", len(got))
	}
	if got[0].URL != "/open-apis/wiki/v2/spaces/get_node" {
		t.Fatalf("first dry-run URL = %q, want parent node lookup", got[0].URL)
	}
	if got[1].URL != "/open-apis/wiki/v2/spaces/<resolved_space_id>/nodes" {
		t.Fatalf("second dry-run URL = %q, want placeholder create URL", got[1].URL)
	}
}

func TestWikiNodeCreateDryRunKeepsExplicitSpaceIDWhenParentProvided(t *testing.T) {
	t.Parallel()

	got := wikiNodeCreateDryRunAPIsForTest(t, func(cmd *cobra.Command) {
		if err := cmd.Flags().Set("title", "Child Node"); err != nil {
			t.Fatalf("set --title: %v", err)
		}
		if err := cmd.Flags().Set("space-id", "space_123"); err != nil {
			t.Fatalf("set --space-id: %v", err)
		}
		if err := cmd.Flags().Set("parent-node-token", "wik_parent"); err != nil {
			t.Fatalf("set --parent-node-token: %v", err)
		}
	})

	if len(got) != 2 {
		t.Fatalf("len(dryRun.api) = %d, want 2", len(got))
	}
	if got[0].URL != "/open-apis/wiki/v2/spaces/get_node" {
		t.Fatalf("first dry-run URL = %q, want parent node lookup", got[0].URL)
	}
	if got[1].URL != "/open-apis/wiki/v2/spaces/space_123/nodes" {
		t.Fatalf("second dry-run URL = %q, want explicit space create URL", got[1].URL)
	}
}

func TestWikiNodeCreateDryRunShowsMyLibraryLookupWhenExplicitAndParentProvided(t *testing.T) {
	t.Parallel()

	got := wikiNodeCreateDryRunAPIsForTest(t, func(cmd *cobra.Command) {
		if err := cmd.Flags().Set("title", "Child Node"); err != nil {
			t.Fatalf("set --title: %v", err)
		}
		if err := cmd.Flags().Set("space-id", wikiMyLibrarySpaceID); err != nil {
			t.Fatalf("set --space-id: %v", err)
		}
		if err := cmd.Flags().Set("parent-node-token", "wik_parent"); err != nil {
			t.Fatalf("set --parent-node-token: %v", err)
		}
	})

	if len(got) != 3 {
		t.Fatalf("len(dryRun.api) = %d, want 3", len(got))
	}
	if got[0].URL != "/open-apis/wiki/v2/spaces/my_library" {
		t.Fatalf("first dry-run URL = %q, want my_library lookup", got[0].URL)
	}
	if got[1].URL != "/open-apis/wiki/v2/spaces/get_node" {
		t.Fatalf("second dry-run URL = %q, want parent node lookup", got[1].URL)
	}
	if got[2].URL != "/open-apis/wiki/v2/spaces/<resolved_space_id>/nodes" {
		t.Fatalf("third dry-run URL = %q, want placeholder create URL", got[2].URL)
	}
}

func wikiNodeCreateDryRunAPIsForTest(t *testing.T, setFlags func(*cobra.Command)) []struct {
	Method string                 `json:"method"`
	URL    string                 `json:"url"`
	Body   map[string]interface{} `json:"body"`
} {
	t.Helper()

	cmd := &cobra.Command{Use: "wiki +node-create"}
	cmd.Flags().String("space-id", "", "")
	cmd.Flags().String("parent-node-token", "", "")
	cmd.Flags().String("title", "", "")
	cmd.Flags().String("node-type", wikiNodeTypeOrigin, "")
	cmd.Flags().String("obj-type", "docx", "")
	cmd.Flags().String("origin-node-token", "", "")
	if setFlags != nil {
		setFlags(cmd)
	}

	runtime := common.TestNewRuntimeContext(cmd, nil)
	dry := WikiNodeCreate.DryRun(context.Background(), runtime)
	if dry == nil {
		t.Fatal("DryRun returned nil")
	}

	data, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry run: %v", err)
	}

	var got struct {
		API []struct {
			Method string                 `json:"method"`
			URL    string                 `json:"url"`
			Body   map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal dry run json: %v", err)
	}

	return got.API
}

func TestWikiNodeCreateMountedExecuteWithExplicitSpaceID(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, stderr, reg := cmdutil.TestFactory(t, wikiTestConfig())

	createStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/wiki/v2/spaces/space_123/nodes",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"space_id":          "space_123",
					"node_token":        "wik_created",
					"obj_token":         "docx_created",
					"obj_type":          "docx",
					"parent_node_token": "",
					"node_type":         "origin",
					"origin_node_token": "",
					"title":             "Wiki Node",
					"has_child":         false,
				},
			},
			"msg": "success",
		},
	}
	reg.Register(createStub)

	err := mountAndRunWiki(t, WikiNodeCreate, []string{
		"+node-create",
		"--space-id", "space_123",
		"--title", "Wiki Node",
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
		t.Fatalf("expected ok=true, got stdout=%s", stdout.String())
	}
	if envelope.Data["resolved_by"] != wikiResolvedByExplicitSpaceID {
		t.Fatalf("resolved_by = %#v, want %q", envelope.Data["resolved_by"], wikiResolvedByExplicitSpaceID)
	}
	if envelope.Data["node_token"] != "wik_created" {
		t.Fatalf("node_token = %#v, want %q", envelope.Data["node_token"], "wik_created")
	}

	var captured map[string]interface{}
	if err := json.Unmarshal(createStub.CapturedBody, &captured); err != nil {
		t.Fatalf("unmarshal captured request body: %v", err)
	}
	if captured["node_type"] != wikiNodeTypeOrigin {
		t.Fatalf("captured node_type = %#v, want %q", captured["node_type"], wikiNodeTypeOrigin)
	}
	if captured["obj_type"] != "docx" {
		t.Fatalf("captured obj_type = %#v, want %q", captured["obj_type"], "docx")
	}
	if captured["title"] != "Wiki Node" {
		t.Fatalf("captured title = %#v, want %q", captured["title"], "Wiki Node")
	}
	if got := stderr.String(); !strings.Contains(got, "Created wiki node in space space_123 via explicit_space_id.") {
		t.Fatalf("stderr = %q, want completed creation message", got)
	}
}
