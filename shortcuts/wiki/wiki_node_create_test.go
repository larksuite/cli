// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// wikiTestLockContentionErr builds the typed API error that runtime.CallAPITyped
// produces for a wiki write-lock-contention response (code 131009), so the
// retry path's errs.ProblemOf(...).Code check sees the same shape in tests as
// in production.
func wikiTestLockContentionErr() error {
	return errclass.BuildAPIError(
		map[string]any{"code": float64(output.LarkErrWikiLockContention), "msg": "lock contention"},
		errclass.ClassifyContext{},
	)
}

type fakeWikiNodeCreateCall struct {
	SpaceID string
	Spec    wikiNodeCreateSpec
}

type fakeWikiNodeCreateClient struct {
	spaces        map[string]*wikiSpaceRecord
	nodes         map[string]*wikiNodeRecord
	createNode    *wikiNodeRecord
	returnNilNode bool
	createErr     error
	createErrs    []error // consumed in order; takes precedence over createErr
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
	if len(fake.createErrs) > 0 {
		err := fake.createErrs[0]
		fake.createErrs = fake.createErrs[1:]
		return nil, err
	}
	if fake.createErr != nil {
		return nil, fake.createErr
	}
	if fake.returnNilNode {
		return nil, nil
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

func wikiPermissionTestConfig(userOpenID string) *core.CliConfig {
	return &core.CliConfig{
		AppID:      fmt.Sprintf("wiki-permission-test-app-%d", wikiTestConfigSeq.Add(1)),
		AppSecret:  "test-secret",
		Brand:      core.BrandFeishu,
		UserOpenId: userOpenID,
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

// requireWikiValidationParams asserts err carries a *errs.ValidationError of
// category validation / subtype invalid_argument and that its named flags
// (single Param or structured Params) cover every wanted flag, locking the
// typed contract and input-recovery fields so callers and agents can re-fill
// the right flags without parsing the prose message.
func requireWikiValidationParams(t *testing.T, err error, wantNames ...string) {
	t.Helper()
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T (%v)", err, err)
	}
	if ve.Category != errs.CategoryValidation || ve.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected validation/invalid_argument, got %s/%s", ve.Category, ve.Subtype)
	}
	have := make(map[string]bool, len(ve.Params)+1)
	if ve.Param != "" {
		have[ve.Param] = true
	}
	for _, p := range ve.Params {
		have[p.Name] = true
	}
	for _, name := range wantNames {
		if !have[name] {
			t.Fatalf("flag %q not found; Param=%q Params=%+v", name, ve.Param, ve.Params)
		}
	}
}

func TestRequireWikiSpaceIDTreatsEmptyAsInvalidResponse(t *testing.T) {
	t.Parallel()

	_, err := requireWikiSpaceID(&wikiSpaceRecord{})
	p, ok := errs.ProblemOf(err)
	if !ok || p.Category != errs.CategoryInternal || p.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("expected internal/invalid_response, got %v", err)
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
	requireWikiValidationParams(t, err, "--space-id", "--parent-node-token")
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

func TestResolveWikiNodeCreateSpaceRejectsBotWithoutLocation(t *testing.T) {
	t.Parallel()

	_, err := resolveWikiNodeCreateSpace(context.Background(), &fakeWikiNodeCreateClient{}, core.AsBot, wikiNodeCreateSpec{
		NodeType: wikiNodeTypeOrigin,
		ObjType:  "docx",
	})
	if err == nil || !strings.Contains(err.Error(), "bot identity requires --space-id or --parent-node-token") {
		t.Fatalf("expected bot location validation error, got %v", err)
	}
	requireWikiValidationParams(t, err, "--space-id", "--parent-node-token")
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
	execution, err := runWikiNodeCreate(context.Background(), client, core.AsUser, spec, io.Discard)
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

func TestRunWikiNodeCreateRejectsNilCreatedNode(t *testing.T) {
	t.Parallel()

	client := &fakeWikiNodeCreateClient{
		spaces: map[string]*wikiSpaceRecord{
			wikiMyLibrarySpaceID: {SpaceID: "space_my_library", SpaceType: "my_library"},
		},
		returnNilNode: true,
	}

	_, err := runWikiNodeCreate(context.Background(), client, core.AsUser, wikiNodeCreateSpec{
		NodeType: wikiNodeTypeOrigin,
		ObjType:  "docx",
		Title:    "Roadmap",
	}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "wiki node create returned no node") {
		t.Fatalf("expected missing node error, got %v", err)
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
					"url":               "https://abc.feishu.cn/wiki/wik_created_real",
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
	if got, want := envelope.Data["url"], "https://abc.feishu.cn/wiki/wik_created_real"; got != want {
		t.Fatalf("url = %#v, want %q (response url must win over synthesized fallback)", got, want)
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

func TestWikiNodeCreateBotAutoGrantSuccess(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiPermissionTestConfig("ou_current_user"))

	createStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/wiki/v2/spaces/space_123/nodes",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"space_id":   "space_123",
					"node_token": "wik_created",
					"obj_token":  "docx_created",
					"obj_type":   "docx",
					"node_type":  "origin",
					"title":      "Wiki Node",
					"has_child":  false,
				},
			},
			"msg": "success",
		},
	}
	reg.Register(createStub)

	permStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/wik_created/members",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
		},
	}
	reg.Register(permStub)

	err := mountAndRunWiki(t, WikiNodeCreate, []string{
		"+node-create",
		"--space-id", "space_123",
		"--title", "Wiki Node",
		"--as", "bot",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	data := decodeWikiEnvelope(t, stdout)
	grant, _ := data["permission_grant"].(map[string]interface{})
	if grant["status"] != common.PermissionGrantGranted {
		t.Fatalf("permission_grant.status = %#v, want %q", grant["status"], common.PermissionGrantGranted)
	}
	if grant["user_open_id"] != "ou_current_user" {
		t.Fatalf("permission_grant.user_open_id = %#v, want %q", grant["user_open_id"], "ou_current_user")
	}
	if grant["message"] != "Granted the current CLI user full_access on the new wiki node." {
		t.Fatalf("permission_grant.message = %#v", grant["message"])
	}

	var body map[string]interface{}
	if err := json.Unmarshal(permStub.CapturedBody, &body); err != nil {
		t.Fatalf("unmarshal permission body: %v", err)
	}
	if body["member_type"] != "openid" || body["member_id"] != "ou_current_user" || body["perm"] != "full_access" || body["type"] != "user" {
		t.Fatalf("unexpected permission request body: %#v", body)
	}
	if body["perm_type"] != "container" {
		t.Fatalf("perm_type = %#v, want %q", body["perm_type"], "container")
	}
}

func TestWikiNodeCreateBotAutoGrantSkippedNoUser(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiPermissionTestConfig(""))

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/wiki/v2/spaces/space_123/nodes",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"space_id":   "space_123",
					"node_token": "wik_skipped",
					"obj_token":  "docx_skipped",
					"obj_type":   "docx",
					"node_type":  "origin",
					"title":      "Wiki Skipped",
					"has_child":  false,
				},
			},
			"msg": "success",
		},
	})

	err := mountAndRunWiki(t, WikiNodeCreate, []string{
		"+node-create",
		"--space-id", "space_123",
		"--title", "Wiki Skipped",
		"--as", "bot",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	data := decodeWikiEnvelope(t, stdout)
	grant, _ := data["permission_grant"].(map[string]interface{})
	if grant["status"] != common.PermissionGrantSkipped {
		t.Fatalf("permission_grant.status = %#v, want %q", grant["status"], common.PermissionGrantSkipped)
	}
	if hint, ok := grant["hint"].(string); !ok || !strings.Contains(hint, "auth login") {
		t.Fatalf("hint = %#v, want string containing 'auth login'", grant["hint"])
	}
}

func TestWikiNodeCreateBotAutoGrantFailed(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiPermissionTestConfig("ou_current_user"))

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/wiki/v2/spaces/space_123/nodes",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"space_id":   "space_123",
					"node_token": "wik_grant_fail",
					"obj_token":  "docx_grant_fail",
					"obj_type":   "docx",
					"node_type":  "origin",
					"title":      "Wiki Fail",
					"has_child":  false,
				},
			},
			"msg": "success",
		},
	})

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/wik_grant_fail/members",
		Body: map[string]interface{}{
			"code": 230001,
			"msg":  "no permission",
		},
	})

	err := mountAndRunWiki(t, WikiNodeCreate, []string{
		"+node-create",
		"--space-id", "space_123",
		"--title", "Wiki Fail",
		"--as", "bot",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	data := decodeWikiEnvelope(t, stdout)
	grant, _ := data["permission_grant"].(map[string]interface{})
	if grant["status"] != common.PermissionGrantFailed {
		t.Fatalf("permission_grant.status = %#v, want %q", grant["status"], common.PermissionGrantFailed)
	}
	if hint, ok := grant["hint"].(string); !ok || !strings.Contains(hint, "Retry later") {
		t.Fatalf("hint = %#v, want string containing 'Retry later'", grant["hint"])
	}
}

func TestWikiNodeCreateUserSkipsPermissionGrantAugmentation(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiPermissionTestConfig("ou_current_user"))

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/wiki/v2/spaces/space_123/nodes",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"space_id":   "space_123",
					"node_token": "wik_created",
					"obj_token":  "docx_created",
					"obj_type":   "docx",
					"node_type":  "origin",
					"title":      "Wiki Node",
					"has_child":  false,
				},
			},
			"msg": "success",
		},
	})

	err := mountAndRunWiki(t, WikiNodeCreate, []string{
		"+node-create",
		"--space-id", "space_123",
		"--title", "Wiki Node",
		"--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	data := decodeWikiEnvelope(t, stdout)
	if _, ok := data["permission_grant"]; ok {
		t.Fatalf("did not expect permission_grant in user mode output: %#v", data)
	}
}

func TestAugmentWikiNodeCreateOutputReturnsEmptyMapForNilInput(t *testing.T) {
	t.Parallel()

	if got := augmentWikiNodeCreateOutput(nil, nil); len(got) != 0 {
		t.Fatalf("augmentWikiNodeCreateOutput(nil, nil) = %#v, want empty map", got)
	}

	if got := augmentWikiNodeCreateOutput(nil, &wikiNodeCreateExecution{}); len(got) != 0 {
		t.Fatalf("augmentWikiNodeCreateOutput(nil, empty execution) = %#v, want empty map", got)
	}
}

func TestWikiNodeURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		node *wikiNodeRecord
		want string
	}{
		{
			name: "prefers response url over synthesized fallback",
			node: &wikiNodeRecord{NodeToken: "wik_token", URL: "https://abc.feishu.cn/wiki/wik_real"},
			want: "https://abc.feishu.cn/wiki/wik_real",
		},
		{
			name: "falls back to synthesized url when response omits it",
			node: &wikiNodeRecord{NodeToken: "wik_token"},
			want: "https://www.feishu.cn/wiki/wik_token",
		},
		{
			name: "blank response url is treated as absent",
			node: &wikiNodeRecord{NodeToken: "wik_token", URL: "   "},
			want: "https://www.feishu.cn/wiki/wik_token",
		},
		{
			name: "nil node yields empty string",
			node: nil,
			want: "",
		},
		{
			name: "no token and no url yields empty string",
			node: &wikiNodeRecord{},
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := wikiNodeURL(core.BrandFeishu, tc.node); got != tc.want {
				t.Fatalf("wikiNodeURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRunWikiNodeCreateRetriesOnLockContention(t *testing.T) {
	t.Parallel()

	lockErr := wikiTestLockContentionErr()

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
		createErrs: []error{lockErr, lockErr}, // fail twice, then succeed
	}

	var stderr bytes.Buffer
	spec := wikiNodeCreateSpec{
		NodeType: wikiNodeTypeOrigin,
		ObjType:  "docx",
		Title:    "Roadmap",
	}
	execution, err := runWikiNodeCreate(context.Background(), client, core.AsUser, spec, &stderr)
	if err != nil {
		t.Fatalf("runWikiNodeCreate() error = %v", err)
	}
	if len(client.createInvoked) != 3 {
		t.Fatalf("create invoked %d times, want 3", len(client.createInvoked))
	}
	if execution.Node.NodeToken != "wik_created" {
		t.Fatalf("node token = %q, want %q", execution.Node.NodeToken, "wik_created")
	}
	if !strings.Contains(stderr.String(), "lock contention") {
		t.Fatalf("stderr = %q, want lock contention log", stderr.String())
	}
	if !strings.Contains(stderr.String(), "retrying (attempt 1/") {
		t.Fatalf("stderr = %q, want attempt 1 log", stderr.String())
	}
	if !strings.Contains(stderr.String(), "retrying (attempt 2/") {
		t.Fatalf("stderr = %q, want attempt 2 log", stderr.String())
	}
}

func TestRunWikiNodeCreateRetriesExhausted(t *testing.T) {
	t.Parallel()

	lockErr := wikiTestLockContentionErr()

	client := &fakeWikiNodeCreateClient{
		spaces: map[string]*wikiSpaceRecord{
			wikiMyLibrarySpaceID: {SpaceID: "space_my_library"},
		},
		createErrs: []error{lockErr, lockErr, lockErr}, // all 3 attempts fail
	}

	var stderr bytes.Buffer
	spec := wikiNodeCreateSpec{
		NodeType: wikiNodeTypeOrigin,
		ObjType:  "docx",
		Title:    "Roadmap",
	}
	_, err := runWikiNodeCreate(context.Background(), client, core.AsUser, spec, &stderr)
	if err == nil {
		t.Fatalf("expected error after retries exhausted")
	}
	if len(client.createInvoked) != 3 {
		t.Fatalf("create invoked %d times, want 3", len(client.createInvoked))
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected a typed errs.* error, got %T: %v", err, err)
	}
	if p.Code != output.LarkErrWikiLockContention {
		t.Fatalf("error code = %d, want %d", p.Code, output.LarkErrWikiLockContention)
	}
	if !strings.Contains(p.Hint, "failed after 2 retries") {
		t.Fatalf("hint = %q, want retry exhaustion message", p.Hint)
	}
	if !strings.Contains(p.Hint, "lock contention") {
		t.Fatalf("hint = %q, want original classification hint preserved", p.Hint)
	}
}

func TestRunWikiNodeCreateNoRetryOnNonContentionError(t *testing.T) {
	t.Parallel()

	// A typed API error for a different code (rate limit, not lock contention),
	// mirroring what runtime.CallAPITyped produces.
	otherErr := errclass.BuildAPIError(
		map[string]any{"code": float64(output.LarkErrRateLimit), "msg": "rate limit"},
		errclass.ClassifyContext{},
	)

	client := &fakeWikiNodeCreateClient{
		spaces: map[string]*wikiSpaceRecord{
			wikiMyLibrarySpaceID: {SpaceID: "space_my_library"},
		},
		createErrs: []error{otherErr},
	}

	var stderr bytes.Buffer
	spec := wikiNodeCreateSpec{
		NodeType: wikiNodeTypeOrigin,
		ObjType:  "docx",
		Title:    "Roadmap",
	}
	_, err := runWikiNodeCreate(context.Background(), client, core.AsUser, spec, &stderr)
	if err == nil {
		t.Fatalf("expected error")
	}
	if len(client.createInvoked) != 1 {
		t.Fatalf("create invoked %d times, want 1 (no retry)", len(client.createInvoked))
	}
	if strings.Contains(stderr.String(), "retrying") {
		t.Fatalf("stderr = %q, should not contain retry log for non-contention error", stderr.String())
	}
}

func TestRunWikiNodeCreateRetriesOnFirstLockThenSucceeds(t *testing.T) {
	t.Parallel()

	lockErr := wikiTestLockContentionErr()

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
		createErrs: []error{lockErr}, // fail once, then succeed
	}

	var stderr bytes.Buffer
	spec := wikiNodeCreateSpec{
		NodeType: wikiNodeTypeOrigin,
		ObjType:  "docx",
		Title:    "Roadmap",
	}
	execution, err := runWikiNodeCreate(context.Background(), client, core.AsUser, spec, &stderr)
	if err != nil {
		t.Fatalf("runWikiNodeCreate() error = %v", err)
	}
	if len(client.createInvoked) != 2 {
		t.Fatalf("create invoked %d times, want 2", len(client.createInvoked))
	}
	if execution.Node.NodeToken != "wik_created" {
		t.Fatalf("node token = %q, want %q", execution.Node.NodeToken, "wik_created")
	}
	if !strings.Contains(stderr.String(), "retrying (attempt 1/") {
		t.Fatalf("stderr = %q, want attempt 1 log", stderr.String())
	}
	if strings.Contains(stderr.String(), "retrying (attempt 2/") {
		t.Fatalf("stderr = %q, should not contain attempt 2 log", stderr.String())
	}
}

func TestRunWikiNodeCreateRetryContextCancelled(t *testing.T) {
	t.Parallel()

	lockErr := wikiTestLockContentionErr()

	client := &fakeWikiNodeCreateClient{
		spaces: map[string]*wikiSpaceRecord{
			wikiMyLibrarySpaceID: {SpaceID: "space_my_library"},
		},
		createErrs: []error{lockErr, lockErr, lockErr}, // always fail
	}

	var stderr bytes.Buffer
	spec := wikiNodeCreateSpec{
		NodeType: wikiNodeTypeOrigin,
		ObjType:  "docx",
		Title:    "Roadmap",
	}

	// Pre-cancel the context so the retry loop's select picks up
	// ctx.Done() immediately during the first backoff wait.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := runWikiNodeCreate(ctx, client, core.AsUser, spec, &stderr)
	if err == nil {
		t.Fatalf("expected error due to context cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	// The initial attempt runs (context is checked only during backoff
	// wait), but no retries should complete.
	if len(client.createInvoked) != 1 {
		t.Fatalf("create invoked %d times, want 1 (no retries after cancel)", len(client.createInvoked))
	}
}

func TestRunWikiNodeCreateNoRetryOnSuccess(t *testing.T) {
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

	var stderr bytes.Buffer
	spec := wikiNodeCreateSpec{
		NodeType: wikiNodeTypeOrigin,
		ObjType:  "docx",
		Title:    "Roadmap",
	}
	execution, err := runWikiNodeCreate(context.Background(), client, core.AsUser, spec, &stderr)
	if err != nil {
		t.Fatalf("runWikiNodeCreate() error = %v", err)
	}
	if len(client.createInvoked) != 1 {
		t.Fatalf("create invoked %d times, want 1", len(client.createInvoked))
	}
	if execution.Node.NodeToken != "wik_created" {
		t.Fatalf("node token = %q, want %q", execution.Node.NodeToken, "wik_created")
	}
	if strings.Contains(stderr.String(), "retrying") {
		t.Fatalf("stderr = %q, should not contain retry log on success", stderr.String())
	}
}
