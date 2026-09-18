// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestWikiNodeDeleteDeclaredScopes(t *testing.T) {
	t.Parallel()
	want := []string{"wiki:node:create", "wiki:node:retrieve"}
	if !slices.Equal(WikiNodeDelete.Scopes, want) {
		t.Fatalf("WikiNodeDelete.Scopes = %v, want %v", WikiNodeDelete.Scopes, want)
	}
}

func TestWikiNodeDeleteScopePreflight(t *testing.T) {
	for _, identity := range []string{"user", "bot"} {
		for _, tt := range []struct {
			name, granted, missing string
		}{
			{"delete only", "wiki:node:create", "wiki:node:retrieve"},
			{"lookup only", "wiki:node:retrieve", "wiki:node:create"},
			{"both", "wiki:node:create wiki:node:retrieve", ""},
		} {
			t.Run(identity+"/"+tt.name, func(t *testing.T) {
				t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
				cfg := wikiTestConfig()
				factory, stdout, _, reg := cmdutil.TestFactory(t, cfg)
				factory.Credential = credential.NewCredentialProvider(nil, &wikiMoveAccountResolver{cfg: cfg}, &mockWikiMoveTokenResolver{scopes: tt.granted}, nil)
				if tt.missing == "" {
					reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/wiki/v2/spaces/node_by_token?token=wik_source", Body: map[string]any{
						"code": 0, "data": map[string]any{"node": map[string]any{"node_token": "wik_source", "space_id": "space_src"}},
					}})
					reg.Register(&httpmock.Stub{Method: "DELETE", URL: "/open-apis/wiki/v2/spaces/space_src/nodes/wik_source", Body: map[string]any{"code": 0, "data": map[string]any{}}})
				}
				err := mountAndRunWiki(t, WikiNodeDelete, []string{
					"+node-delete", "--node-token", "wik_source", "--obj-type", "wiki", "--space-id", "space_src", "--yes", "--as", identity,
				}, factory, stdout)
				if tt.missing == "" {
					if err != nil {
						t.Fatalf("both scopes should allow lookup and deletion: %v", err)
					}
					return
				}
				// No API stubs are registered: missing scopes must stop before
				// either lookup or deletion, even with an explicit space ID.
				var permission *errs.PermissionError
				if !errors.As(err, &permission) {
					t.Fatalf("error = %T %v, want PermissionError", err, err)
				}
				if permission.Category != errs.CategoryAuthorization || permission.Subtype != errs.SubtypeMissingScope || permission.Identity != identity {
					t.Fatalf("unexpected permission metadata: %+v", permission)
				}
				if !slices.Equal(permission.MissingScopes, []string{tt.missing}) {
					t.Fatalf("missing scopes = %v, want %s", permission.MissingScopes, tt.missing)
				}
				if stdout.Len() != 0 {
					t.Fatalf("unexpected success output: %s", stdout.String())
				}
			})
		}
	}
}

// ── parseWikiNodeDeleteSpec ─────────────────────────────────────────────────

func TestParseWikiNodeDeleteSpecAcceptsRawWikiToken(t *testing.T) {
	t.Parallel()

	spec, err := parseWikiNodeDeleteSpec("wikcnABC", "wiki", "", true)
	if err != nil {
		t.Fatalf("parseWikiNodeDeleteSpec() error = %v", err)
	}
	if spec.NodeToken != "wikcnABC" || spec.ObjType != "wiki" || spec.SourceKind != "raw" || !spec.IncludeChildren {
		t.Fatalf("spec = %+v", spec)
	}
	body := spec.RequestBody()
	if body["obj_type"] != "wiki" || body["include_children"] != true {
		t.Fatalf("RequestBody = %#v", body)
	}
}

func TestParseWikiNodeDeleteSpecRejectsMissingObjType(t *testing.T) {
	t.Parallel()

	_, err := parseWikiNodeDeleteSpec("wikcnABC", "", "", true)
	if err == nil || !strings.Contains(err.Error(), "--obj-type is required") {
		t.Fatalf("expected obj-type required error, got %v", err)
	}
}

func TestParseWikiNodeDeleteSpecRejectsInvalidObjType(t *testing.T) {
	t.Parallel()

	_, err := parseWikiNodeDeleteSpec("wikcnABC", "comment", "", true)
	if err == nil || !strings.Contains(err.Error(), "is not valid") {
		t.Fatalf("expected invalid obj-type error, got %v", err)
	}
}

func TestParseWikiNodeDeleteSpecRejectsEmptyToken(t *testing.T) {
	t.Parallel()

	_, err := parseWikiNodeDeleteSpec("   ", "wiki", "", true)
	if err == nil || !strings.Contains(err.Error(), "--node-token is required") {
		t.Fatalf("expected token required error, got %v", err)
	}
}

func TestParseWikiNodeDeleteSpecExtractsTokenFromWikiURL(t *testing.T) {
	t.Parallel()

	spec, err := parseWikiNodeDeleteSpec("https://feishu.cn/wiki/wikcnABC", "", "", true)
	if err != nil {
		t.Fatalf("parseWikiNodeDeleteSpec() error = %v", err)
	}
	if spec.NodeToken != "wikcnABC" || spec.ObjType != "wiki" || spec.SourceKind != "url" {
		t.Fatalf("spec = %+v, want url-extracted node_token + obj_type=wiki", spec)
	}
}

func TestParseWikiNodeDeleteSpecInfersObjTypeFromDocxURL(t *testing.T) {
	t.Parallel()

	spec, err := parseWikiNodeDeleteSpec("https://feishu.cn/docx/docxXYZ", "", "", false)
	if err != nil {
		t.Fatalf("parseWikiNodeDeleteSpec() error = %v", err)
	}
	if spec.NodeToken != "docxXYZ" || spec.ObjType != "docx" || spec.IncludeChildren {
		t.Fatalf("spec = %+v, want docxXYZ obj_type=docx include_children=false", spec)
	}
}

func TestParseWikiNodeDeleteSpecRejectsURLObjTypeMismatch(t *testing.T) {
	t.Parallel()

	_, err := parseWikiNodeDeleteSpec("https://feishu.cn/docx/docxXYZ", "wiki", "", true)
	if err == nil || !strings.Contains(err.Error(), "does not match the obj_type") {
		t.Fatalf("expected obj-type mismatch error, got %v", err)
	}
}

func TestParseWikiNodeDeleteSpecRejectsPartialPath(t *testing.T) {
	t.Parallel()

	_, err := parseWikiNodeDeleteSpec("/wiki/wikcnABC", "wiki", "", true)
	if err == nil || !strings.Contains(err.Error(), "partial paths are not accepted") {
		t.Fatalf("expected partial-path rejection, got %v", err)
	}
}

// ── DryRun ──────────────────────────────────────────────────────────────────

func TestBuildWikiNodeDeleteDryRunWithoutSpaceIDShowsResolve(t *testing.T) {
	t.Parallel()

	spec, err := parseWikiNodeDeleteSpec("https://feishu.cn/docx/docxXYZ", "", "", true)
	if err != nil {
		t.Fatalf("parseWikiNodeDeleteSpec() error = %v", err)
	}

	dry := buildWikiNodeDeleteDryRun(spec)
	got := decodeDryRunAPIs(t, dry)
	if len(got) != 3 {
		t.Fatalf("len(dry.api) = %d, want 3 (node_by_token, delete, task poll)", len(got))
	}
	if got[0].URL != "/open-apis/wiki/v2/spaces/node_by_token" {
		t.Fatalf("step[0].URL = %q, want node_by_token", got[0].URL)
	}
	if len(got[0].Params) != 1 || got[0].Params["token"] != "docxXYZ" {
		t.Fatalf("step[0].params = %#v", got[0].Params)
	}
	if got[1].URL != "/open-apis/wiki/v2/spaces/<resolved_space_id>/nodes/<resolved_obj_token>" {
		t.Fatalf("step[1].URL = %q, want delete with placeholder", got[1].URL)
	}
	if got[1].Body["obj_type"] != "docx" || got[1].Body["include_children"] != true {
		t.Fatalf("step[1].body = %#v", got[1].Body)
	}
	if got[2].Params["task_type"] != "delete_node" {
		t.Fatalf("step[2].params task_type = %#v, want delete_node", got[2].Params)
	}
}

func TestBuildWikiNodeDeleteDryRunWithSpaceIDResolvesToken(t *testing.T) {
	t.Parallel()

	spec, err := parseWikiNodeDeleteSpec("wikcnABC", "wiki", "7629741305993170448", false)
	if err != nil {
		t.Fatalf("parseWikiNodeDeleteSpec() error = %v", err)
	}

	dry := buildWikiNodeDeleteDryRun(spec)
	got := decodeDryRunAPIs(t, dry)
	if len(got) != 3 {
		t.Fatalf("len(dry.api) = %d, want 3 (lookup + delete + task poll) when --space-id supplied", len(got))
	}
	if got[1].Method != "DELETE" || got[1].URL != "/open-apis/wiki/v2/spaces/7629741305993170448/nodes/<resolved_node_token>" {
		t.Fatalf("step[0] = %+v", got[1])
	}
	if got[1].Body["include_children"] != false {
		t.Fatalf("body include_children = %#v", got[1].Body["include_children"])
	}
}

// ── runWikiNodeDelete unit ──────────────────────────────────────────────────

type fakeWikiNodeDeleteClient struct {
	resolveErr   error
	resolveNode  *wikiNodeRecord
	resolveCalls []string

	deleteErr    error
	deleteTaskID string
	deleteCalls  []struct {
		SpaceID string
		Spec    wikiNodeDeleteSpec
	}

	taskStatuses []wikiAsyncTaskStatus
	taskErrs     []error
	taskCallArgs []string
}

func (fake *fakeWikiNodeDeleteClient) ResolveNode(ctx context.Context, token string) (*wikiNodeRecord, error) {
	fake.resolveCalls = append(fake.resolveCalls, token)
	if fake.resolveErr != nil {
		return nil, fake.resolveErr
	}
	return fake.resolveNode, nil
}

func (fake *fakeWikiNodeDeleteClient) DeleteNode(ctx context.Context, spaceID string, spec wikiNodeDeleteSpec) (string, error) {
	fake.deleteCalls = append(fake.deleteCalls, struct {
		SpaceID string
		Spec    wikiNodeDeleteSpec
	}{SpaceID: spaceID, Spec: spec})
	if fake.deleteErr != nil {
		return "", fake.deleteErr
	}
	return fake.deleteTaskID, nil
}

func (fake *fakeWikiNodeDeleteClient) GetDeleteNodeTask(ctx context.Context, taskID string) (wikiAsyncTaskStatus, error) {
	idx := len(fake.taskCallArgs)
	fake.taskCallArgs = append(fake.taskCallArgs, taskID)
	if idx < len(fake.taskErrs) && fake.taskErrs[idx] != nil {
		return wikiAsyncTaskStatus{TaskID: taskID}, fake.taskErrs[idx]
	}
	if idx < len(fake.taskStatuses) {
		status := fake.taskStatuses[idx]
		if status.TaskID == "" {
			status.TaskID = taskID
		}
		return status, nil
	}
	return wikiAsyncTaskStatus{TaskID: taskID}, nil
}

var wikiDeleteNodePollMu sync.Mutex

func withSingleWikiDeleteNodePoll(t *testing.T) {
	t.Helper()
	wikiDeleteNodePollMu.Lock()

	prevAttempts, prevInterval := wikiDeleteNodePollAttempts, wikiDeleteNodePollInterval
	wikiDeleteNodePollAttempts, wikiDeleteNodePollInterval = 1, 0
	t.Cleanup(func() {
		wikiDeleteNodePollAttempts, wikiDeleteNodePollInterval = prevAttempts, prevInterval
		wikiDeleteNodePollMu.Unlock()
	})
}

func newWikiNodeDeleteRuntime(t *testing.T, as core.Identity) (*common.RuntimeContext, *bytes.Buffer) {
	t.Helper()

	cfg := wikiTestConfig()
	factory, _, stderr, _ := cmdutil.TestFactory(t, cfg)
	runtime := common.TestNewRuntimeContextWithIdentity(&cobra.Command{Use: "wiki +node-delete"}, cfg, as)
	runtime.Factory = factory
	return runtime, stderr
}

func TestRunWikiNodeDeleteResolvesSpaceWhenMissing(t *testing.T) {
	t.Parallel()

	runtime, _ := newWikiNodeDeleteRuntime(t, core.AsUser)
	client := &fakeWikiNodeDeleteClient{
		resolveNode: &wikiNodeRecord{SpaceID: "space_resolved", NodeToken: "wikcnABC"},
	}

	out, err := runWikiNodeDelete(context.Background(), client, runtime, wikiNodeDeleteSpec{
		NodeToken:       "wikcnABC",
		ObjType:         "wiki",
		IncludeChildren: true,
	})
	if err != nil {
		t.Fatalf("runWikiNodeDelete() error = %v", err)
	}
	if len(client.resolveCalls) != 1 || client.resolveCalls[0] != "wikcnABC" {
		t.Fatalf("resolve calls = %v", client.resolveCalls)
	}
	if len(client.deleteCalls) != 1 || client.deleteCalls[0].SpaceID != "space_resolved" {
		t.Fatalf("delete calls = %+v", client.deleteCalls)
	}
	if out["space_id"] != "space_resolved" || out["ready"] != true || out["status"] != "success" {
		t.Fatalf("sync output = %#v", out)
	}
}

func TestRunWikiNodeDeleteResolvesTokenWhenSpaceProvided(t *testing.T) {
	t.Parallel()

	runtime, _ := newWikiNodeDeleteRuntime(t, core.AsUser)
	client := &fakeWikiNodeDeleteClient{resolveNode: &wikiNodeRecord{SpaceID: "space_explicit", NodeToken: "wikcnABC"}}

	_, err := runWikiNodeDelete(context.Background(), client, runtime, wikiNodeDeleteSpec{
		NodeToken: "wikcnABC", ObjType: "wiki", SpaceID: "space_explicit",
	})
	if err != nil {
		t.Fatalf("runWikiNodeDelete() error = %v", err)
	}
	if len(client.resolveCalls) != 1 {
		t.Fatalf("expected token lookup when --space-id supplied, got %v", client.resolveCalls)
	}
	if client.deleteCalls[0].SpaceID != "space_explicit" {
		t.Fatalf("delete used wrong space: %+v", client.deleteCalls)
	}
}

func TestRunWikiNodeDeleteAsyncReadyShape(t *testing.T) {
	withSingleWikiDeleteNodePoll(t)

	runtime, stderr := newWikiNodeDeleteRuntime(t, core.AsUser)
	client := &fakeWikiNodeDeleteClient{
		resolveNode:  &wikiNodeRecord{SpaceID: "space_123", NodeToken: "wikcnABC"},
		deleteTaskID: "task_async_node",
		taskStatuses: []wikiAsyncTaskStatus{{Status: "success"}},
	}

	out, err := runWikiNodeDelete(context.Background(), client, runtime, wikiNodeDeleteSpec{
		NodeToken: "wikcnABC", ObjType: "wiki", SpaceID: "space_123", IncludeChildren: true,
	})
	if err != nil {
		t.Fatalf("runWikiNodeDelete() error = %v", err)
	}
	if out["task_id"] != "task_async_node" || out["ready"] != true || out["failed"] != false {
		t.Fatalf("async-ready output = %#v", out)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want no async progress output", stderr.String())
	}
}

func TestRunWikiNodeDeleteAsyncTimeoutReturnsNextCommand(t *testing.T) {
	withSingleWikiDeleteNodePoll(t)

	runtime, _ := newWikiNodeDeleteRuntime(t, core.AsUser)
	client := &fakeWikiNodeDeleteClient{
		resolveNode:  &wikiNodeRecord{SpaceID: "space_123", NodeToken: "wikcnABC"},
		deleteTaskID: "task_async_node",
		taskStatuses: []wikiAsyncTaskStatus{{Status: "processing"}},
	}

	out, err := runWikiNodeDelete(context.Background(), client, runtime, wikiNodeDeleteSpec{
		NodeToken: "wikcnABC", ObjType: "wiki", SpaceID: "space_123",
	})
	if err != nil {
		t.Fatalf("runWikiNodeDelete() error = %v", err)
	}
	wantNext := wikiDeleteNodeTaskResultCommand("task_async_node", core.AsUser)
	if out["ready"] != false || out["timed_out"] != true || out["next_command"] != wantNext {
		t.Fatalf("timeout output = %#v", out)
	}
	if !strings.Contains(wantNext, "wiki_delete_node") {
		t.Fatalf("next command should scope wiki_delete_node, got %q", wantNext)
	}
}

func TestRunWikiNodeDeleteAsyncFailureSurfacesReason(t *testing.T) {
	withSingleWikiDeleteNodePoll(t)

	runtime, _ := newWikiNodeDeleteRuntime(t, core.AsUser)
	client := &fakeWikiNodeDeleteClient{
		resolveNode:  &wikiNodeRecord{SpaceID: "space_123", NodeToken: "wikcnABC"},
		deleteTaskID: "task_async_node",
		taskStatuses: []wikiAsyncTaskStatus{{Status: "failure", StatusMsg: "permission denied"}},
	}

	_, err := runWikiNodeDelete(context.Background(), client, runtime, wikiNodeDeleteSpec{
		NodeToken: "wikcnABC", ObjType: "wiki", SpaceID: "space_123",
	})
	if err == nil || !strings.Contains(err.Error(), "delete-node task task_async_node failed: permission denied") {
		t.Fatalf("expected async failure error, got %v", err)
	}
}

// ── error code hint mapping ─────────────────────────────────────────────────

func TestWrapWikiNodeDeleteAPIErrorAddsApprovalHint(t *testing.T) {
	t.Parallel()

	in := errclass.BuildAPIError(
		map[string]any{"code": float64(wikiDeleteNodeErrCodeApprovalRequired), "msg": "node requires delete approval"},
		errclass.ClassifyContext{},
	)
	got := wrapWikiNodeDeleteAPIError(in)
	p, ok := errs.ProblemOf(got)
	if !ok {
		t.Fatalf("expected a typed errs.* error, got %T %v", got, got)
	}
	if !strings.Contains(p.Hint, "delete-approval enabled") || !strings.Contains(p.Hint, "Wiki UI") {
		t.Fatalf("hint = %q, want approval guidance", p.Hint)
	}
	// Original code/message must be preserved so logs and dashboards still
	// pivot on the upstream error code.
	if p.Code != wikiDeleteNodeErrCodeApprovalRequired {
		t.Fatalf("hint wrapper lost the original code: %d", p.Code)
	}
	if p.Message != "node requires delete approval" {
		t.Fatalf("message changed unexpectedly: %q", p.Message)
	}
}

func TestWrapWikiNodeDeleteAPIErrorAddsSubtreeHint(t *testing.T) {
	t.Parallel()

	in := errclass.BuildAPIError(
		map[string]any{"code": float64(wikiDeleteNodeErrCodeSubtreeTooLarge), "msg": "subtree too large"},
		errclass.ClassifyContext{},
	)
	got := wrapWikiNodeDeleteAPIError(in)
	p, ok := errs.ProblemOf(got)
	if !ok {
		t.Fatalf("expected a typed errs.* error, got %T %v", got, got)
	}
	if !strings.Contains(p.Hint, "--include-children=false") {
		t.Fatalf("hint = %q, want subtree-too-large guidance", p.Hint)
	}
}

func TestWrapWikiNodeDeleteAPIErrorPassesThroughUnknownCodes(t *testing.T) {
	t.Parallel()

	in := errclass.BuildAPIError(
		map[string]any{"code": float64(131005), "msg": "node not found"},
		errclass.ClassifyContext{},
	)
	got := wrapWikiNodeDeleteAPIError(in)
	if got != in {
		t.Fatalf("unknown code should pass through unchanged; got %#v", got)
	}
}

func TestWrapWikiNodeDeleteAPIErrorIgnoresNonExit(t *testing.T) {
	t.Parallel()

	in := errors.New("transport boom")
	if got := wrapWikiNodeDeleteAPIError(in); got != in {
		t.Fatalf("non-ExitError should pass through, got %T %v", got, got)
	}
	if got := wrapWikiNodeDeleteAPIError(nil); got != nil {
		t.Fatalf("nil should pass through, got %v", got)
	}
}

// ── Mounted execute (httpmock) ──────────────────────────────────────────────

func TestWikiNodeDeleteExecuteRequiresYesConfirmation(t *testing.T) {
	factory, stdout, _, _ := cmdutil.TestFactory(t, wikiTestConfig())

	err := mountAndRunWiki(t, WikiNodeDelete, []string{
		"+node-delete",
		"--node-token", "wikcnABC",
		"--obj-type", "wiki",
		"--space-id", "space_123",
		"--as", "user",
	}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "requires confirmation") {
		t.Fatalf("expected high-risk confirmation error, got %v", err)
	}
}

func TestWikiNodeDeleteExecuteSync(t *testing.T) {
	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())

	reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/wiki/v2/spaces/node_by_token", Body: map[string]interface{}{
		"code": 0, "data": map[string]interface{}{"node": map[string]interface{}{"space_id": "space_123", "node_token": "wikcnABC"}},
	}})
	deleteStub := &httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/wiki/v2/spaces/space_123/nodes/wikcnABC",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
			"msg":  "success",
		},
	}
	reg.Register(deleteStub)

	err := mountAndRunWiki(t, WikiNodeDelete, []string{
		"+node-delete",
		"--node-token", "wikcnABC",
		"--obj-type", "wiki",
		"--space-id", "space_123",
		"--yes",
		"--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	data := decodeWikiEnvelope(t, stdout)
	if data["ready"] != true || data["failed"] != false || data["space_id"] != "space_123" {
		t.Fatalf("sync output = %#v", data)
	}
	if data["obj_type"] != "wiki" || data["include_children"] != true {
		t.Fatalf("obj_type/include_children = %#v / %#v", data["obj_type"], data["include_children"])
	}

	var captured map[string]interface{}
	if err := json.Unmarshal(deleteStub.CapturedBody, &captured); err != nil {
		t.Fatalf("unmarshal captured body: %v", err)
	}
	if captured["obj_type"] != "wiki" || captured["include_children"] != true {
		t.Fatalf("captured DELETE body = %#v", captured)
	}
}

func TestWikiNodeDeleteExecuteResolvesSpaceIDFromURL(t *testing.T) {
	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())

	resolveStub := &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/node_by_token",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"space_id":   "space_resolved",
					"node_token": "wikcnABC",
					"obj_token":  "docxXYZ",
					"obj_type":   "docx",
				},
			},
		},
	}
	var resolveQuery string
	resolveStub.OnMatch = func(req *http.Request) { resolveQuery = req.URL.RawQuery }
	reg.Register(resolveStub)

	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/wiki/v2/spaces/space_resolved/nodes/docxXYZ",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
		},
	})

	err := mountAndRunWiki(t, WikiNodeDelete, []string{
		"+node-delete",
		"--node-token", "https://feishu.cn/docx/docxXYZ",
		"--yes",
		"--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	if resolveQuery != "token=docxXYZ" {
		t.Fatalf("resolve query = %q, want token only", resolveQuery)
	}
	data := decodeWikiEnvelope(t, stdout)
	if data["space_id"] != "space_resolved" || data["obj_type"] != "docx" {
		t.Fatalf("output = %#v", data)
	}
}

func TestWikiNodeDeleteExecuteAsyncSuccess(t *testing.T) {
	withSingleWikiDeleteNodePoll(t)

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/wiki/v2/spaces/node_by_token", Body: map[string]interface{}{
		"code": 0, "data": map[string]interface{}{"node": map[string]interface{}{"space_id": "space_123", "node_token": "wikcnABC"}},
	}})
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/wiki/v2/spaces/space_123/nodes/wikcnABC",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"task_id": "task_async_node"},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/tasks/task_async_node",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"task": map[string]interface{}{
					// Gateway returns delete-node status under the generic
					// simple_task_result key (NOT delete_node_result).
					"simple_task_result": map[string]interface{}{
						"status": "success",
					},
				},
			},
		},
	})

	err := mountAndRunWiki(t, WikiNodeDelete, []string{
		"+node-delete",
		"--node-token", "wikcnABC",
		"--obj-type", "wiki",
		"--space-id", "space_123",
		"--yes",
		"--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}
	data := decodeWikiEnvelope(t, stdout)
	if data["task_id"] != "task_async_node" || data["ready"] != true || data["failed"] != false {
		t.Fatalf("async-success output = %#v", data)
	}
}

// ── helpers ─────────────────────────────────────────────────────────────────

type dryRunStep struct {
	Method string                 `json:"method"`
	URL    string                 `json:"url"`
	Body   map[string]interface{} `json:"body"`
	Params map[string]interface{} `json:"params"`
}

func decodeDryRunAPIs(t *testing.T, dry *common.DryRunAPI) []dryRunStep {
	t.Helper()

	data, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry run: %v", err)
	}
	var got struct {
		API []dryRunStep `json:"api"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal dry run: %v", err)
	}
	return got.API
}
