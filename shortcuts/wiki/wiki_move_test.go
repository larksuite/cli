// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

type fakeWikiMoveNodeCall struct {
	SourceSpaceID string
	Spec          wikiMoveSpec
}

type fakeWikiDocsToWikiMoveCall struct {
	TargetSpaceID string
	Spec          wikiMoveSpec
}

type fakeWikiMoveClient struct {
	nodes map[string]*wikiNodeRecord

	getNodeErr  error
	moveNode    *wikiNodeRecord
	moveNodeErr error
	docsResp    *wikiMoveDocsResponse
	docsErr     error

	taskStatuses []wikiMoveTaskStatus
	taskErrs     []error

	getNodeCalls     []string
	moveNodeCalls    []fakeWikiMoveNodeCall
	docsToWikiCalls  []fakeWikiDocsToWikiMoveCall
	moveTaskCallArgs []string
}

func (fake *fakeWikiMoveClient) GetNode(ctx context.Context, token string) (*wikiNodeRecord, error) {
	fake.getNodeCalls = append(fake.getNodeCalls, token)
	if fake.getNodeErr != nil {
		return nil, fake.getNodeErr
	}
	if node, ok := fake.nodes[token]; ok {
		return node, nil
	}
	return &wikiNodeRecord{}, nil
}

func (fake *fakeWikiMoveClient) MoveNode(ctx context.Context, sourceSpaceID string, spec wikiMoveSpec) (*wikiNodeRecord, error) {
	fake.moveNodeCalls = append(fake.moveNodeCalls, fakeWikiMoveNodeCall{SourceSpaceID: sourceSpaceID, Spec: spec})
	if fake.moveNodeErr != nil {
		return nil, fake.moveNodeErr
	}
	if fake.moveNode != nil {
		return fake.moveNode, nil
	}
	return &wikiNodeRecord{SpaceID: sourceSpaceID, NodeToken: spec.NodeToken}, nil
}

func (fake *fakeWikiMoveClient) MoveDocsToWiki(ctx context.Context, targetSpaceID string, spec wikiMoveSpec) (*wikiMoveDocsResponse, error) {
	fake.docsToWikiCalls = append(fake.docsToWikiCalls, fakeWikiDocsToWikiMoveCall{TargetSpaceID: targetSpaceID, Spec: spec})
	if fake.docsErr != nil {
		return nil, fake.docsErr
	}
	if fake.docsResp != nil {
		return fake.docsResp, nil
	}
	return &wikiMoveDocsResponse{}, nil
}

func (fake *fakeWikiMoveClient) GetMoveTask(ctx context.Context, taskID string) (wikiMoveTaskStatus, error) {
	idx := len(fake.moveTaskCallArgs)
	fake.moveTaskCallArgs = append(fake.moveTaskCallArgs, taskID)
	if idx < len(fake.taskErrs) && fake.taskErrs[idx] != nil {
		return wikiMoveTaskStatus{TaskID: taskID}, fake.taskErrs[idx]
	}
	if idx < len(fake.taskStatuses) {
		status := fake.taskStatuses[idx]
		if status.TaskID == "" {
			status.TaskID = taskID
		}
		return status, nil
	}
	return wikiMoveTaskStatus{TaskID: taskID}, nil
}

type mockWikiMoveTokenResolver struct {
	token  string
	scopes string
	err    error
}

type wikiMoveAccountResolver struct {
	cfg *core.CliConfig
}

func (r *wikiMoveAccountResolver) ResolveAccount(ctx context.Context) (*credential.Account, error) {
	return credential.AccountFromCliConfig(r.cfg), nil
}

func (m *mockWikiMoveTokenResolver) ResolveToken(ctx context.Context, req credential.TokenSpec) (*credential.TokenResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	token := m.token
	if token == "" {
		token = "test-token"
	}
	return &credential.TokenResult{Token: token, Scopes: m.scopes}, nil
}

var wikiMovePollMu sync.Mutex

func withSingleWikiMovePoll(t *testing.T) {
	t.Helper()
	wikiMovePollMu.Lock()

	prevAttempts, prevInterval := wikiMovePollAttempts, wikiMovePollInterval
	wikiMovePollAttempts, wikiMovePollInterval = 1, 0
	t.Cleanup(func() {
		wikiMovePollAttempts, wikiMovePollInterval = prevAttempts, prevInterval
		wikiMovePollMu.Unlock()
	})
}

func newWikiMoveRuntimeWithScopes(t *testing.T, as core.Identity, scopes string) (*common.RuntimeContext, *bytes.Buffer) {
	t.Helper()

	cfg := wikiTestConfig()
	factory, _, stderr, _ := cmdutil.TestFactory(t, cfg)
	factory.Credential = credential.NewCredentialProvider(nil, nil, &mockWikiMoveTokenResolver{scopes: scopes}, nil)

	runtime := common.TestNewRuntimeContextWithIdentity(&cobra.Command{Use: "wiki +move"}, cfg, as)
	runtime.Factory = factory
	return runtime, stderr
}

func decodeWikiEnvelope(t *testing.T, stdout *bytes.Buffer) map[string]interface{} {
	t.Helper()

	var env struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal wiki envelope: %v\nstdout=%s", err, stdout.String())
	}
	if !env.OK {
		t.Fatalf("expected ok=true envelope, got stdout=%s", stdout.String())
	}
	return env.Data
}

func decodeWikiCapturedJSONBody(t *testing.T, stub *httpmock.Stub) map[string]interface{} {
	t.Helper()

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("unmarshal captured body: %v\nraw=%s", err, string(stub.CapturedBody))
	}
	return body
}

func TestValidateWikiMoveSpecRejectsInvalidCombinations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		spec    wikiMoveSpec
		wantErr string
	}{
		{
			name:    "node move rejects docs flags",
			spec:    wikiMoveSpec{NodeToken: "wik_node", ObjType: "sheet", TargetSpaceID: "space_dst"},
			wantErr: "cannot be combined",
		},
		{
			name:    "node move requires target",
			spec:    wikiMoveSpec{NodeToken: "wik_node"},
			wantErr: "cannot both be empty",
		},
		{
			name:    "source space requires node token",
			spec:    wikiMoveSpec{SourceSpaceID: "space_src", ObjType: "sheet", ObjToken: "sheet_token", TargetSpaceID: "space_dst"},
			wantErr: "can only be used with --node-token",
		},
		{
			name:    "docs to wiki requires obj type",
			spec:    wikiMoveSpec{ObjToken: "sheet_token", TargetSpaceID: "space_dst"},
			wantErr: "--obj-type is required",
		},
		{
			name:    "docs to wiki requires obj token",
			spec:    wikiMoveSpec{ObjType: "sheet", TargetSpaceID: "space_dst"},
			wantErr: "--obj-token is required",
		},
		{
			name:    "docs to wiki requires target space",
			spec:    wikiMoveSpec{ObjType: "sheet", ObjToken: "sheet_token"},
			wantErr: "--target-space-id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateWikiMoveSpec(tt.spec)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestValidateWikiMoveSpecAcceptsValidModes(t *testing.T) {
	t.Parallel()

	for _, spec := range []wikiMoveSpec{
		{NodeToken: "wik_node", TargetSpaceID: "space_dst"},
		{ObjType: "sheet", ObjToken: "sheet_token", TargetSpaceID: "space_dst", TargetParentToken: "wik_parent", Apply: true},
	} {
		if err := validateWikiMoveSpec(spec); err != nil {
			t.Fatalf("validateWikiMoveSpec(%+v) error = %v", spec, err)
		}
	}
}

func TestWikiMoveDeclaredScopes(t *testing.T) {
	t.Parallel()

	want := []string{"wiki:node:move", "wiki:node:read", "wiki:space:read"}
	if !reflect.DeepEqual(WikiMove.Scopes, want) {
		t.Fatalf("WikiMove.Scopes = %v, want %v", WikiMove.Scopes, want)
	}
}

func TestWikiMoveShortcutMissingDeclaredScope(t *testing.T) {
	cfg := wikiTestConfig()
	factory, stdout, _, _ := cmdutil.TestFactory(t, cfg)
	factory.Credential = credential.NewCredentialProvider(nil, &wikiMoveAccountResolver{cfg: cfg}, &mockWikiMoveTokenResolver{scopes: "wiki:node:read"}, nil)

	err := mountAndRunWiki(t, WikiMove, []string{
		"+move",
		"--node-token", "wik_node",
		"--target-space-id", "space_dst",
		"--as", "user",
	}, factory, stdout)
	if err == nil {
		t.Fatal("expected missing scope error, got nil")
	}
	if !strings.Contains(err.Error(), "missing required scope(s): wiki:node:move") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWikiMoveTaskStatusPendingAndFallbackLabels(t *testing.T) {
	t.Parallel()

	pending := wikiMoveTaskStatus{}
	if !pending.Pending() || pending.PrimaryStatusLabel() != "processing" {
		t.Fatalf("pending status = %+v", pending)
	}

	ready := wikiMoveTaskStatus{MoveResults: []wikiMoveTaskResult{{Status: 0}}}
	if !ready.Ready() || ready.PrimaryStatusLabel() != "success" {
		t.Fatalf("ready status = %+v", ready)
	}

	failed := wikiMoveTaskStatus{MoveResults: []wikiMoveTaskResult{{Status: -1}}}
	if !failed.Failed() || failed.PrimaryStatusLabel() != "failure" {
		t.Fatalf("failed status = %+v", failed)
	}
}

func TestWikiMoveTaskStatusPrimarySurfacesFailureOverEarlierSuccess(t *testing.T) {
	t.Parallel()

	status := wikiMoveTaskStatus{
		MoveResults: []wikiMoveTaskResult{
			{Status: 0, StatusMsg: "success"},
			{Status: -3, StatusMsg: "permission denied"},
			{Status: 1, StatusMsg: "processing"},
		},
	}
	if got := status.PrimaryStatusCode(); got != -3 {
		t.Fatalf("PrimaryStatusCode = %d, want -3", got)
	}
	if got := status.PrimaryStatusLabel(); got != "permission denied" {
		t.Fatalf("PrimaryStatusLabel = %q, want permission denied", got)
	}
	// FirstResult must keep its literal "first entry" semantics for callers
	// that flatten node fields from the first move_result.
	if first := status.FirstResult(); first == nil || first.StatusMsg != "success" {
		t.Fatalf("FirstResult = %+v, want first success entry", first)
	}
}

func TestWikiMoveTaskStatusPrimaryPrefersProcessingOverFirstSuccess(t *testing.T) {
	t.Parallel()

	status := wikiMoveTaskStatus{
		MoveResults: []wikiMoveTaskResult{
			{Status: 0, StatusMsg: "success"},
			{Status: 1, StatusMsg: "processing"},
		},
	}
	if got := status.PrimaryStatusCode(); got != 1 {
		t.Fatalf("PrimaryStatusCode = %d, want 1", got)
	}
	if got := status.PrimaryStatusLabel(); got != "processing" {
		t.Fatalf("PrimaryStatusLabel = %q, want processing", got)
	}
}

func TestWikiMoveValidateRejectsBotMyLibrary(t *testing.T) {
	cfg := wikiTestConfig()
	factory, stdout, _, _ := cmdutil.TestFactory(t, cfg)

	err := mountAndRunWiki(t, WikiMove, []string{
		"+move",
		"--obj-type", "docx",
		"--obj-token", "doccnXXX",
		"--target-space-id", "my_library",
		"--as", "bot",
	}, factory, stdout)
	if err == nil {
		t.Fatal("expected validation error for bot + my_library, got nil")
	}
	if !strings.Contains(err.Error(), "my_library") || !strings.Contains(err.Error(), "--as bot") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWikiMoveValidateAllowsUserMyLibrary(t *testing.T) {
	t.Parallel()

	// Bot guard must not affect user identity. We only assert the my_library
	// validation path doesn't trip; an empty obj-token still fails downstream
	// for unrelated reasons, so we check the error does not mention my_library.
	if err := validateWikiMoveSpec(wikiMoveSpec{
		ObjType:       "docx",
		ObjToken:      "doccnXXX",
		TargetSpaceID: "my_library",
	}); err != nil {
		t.Fatalf("validateWikiMoveSpec(user my_library) = %v, want nil", err)
	}
}

func TestWikiMoveDryRunNodeMoveIncludesResolutionSteps(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "wiki +move"}
	cmd.Flags().String("node-token", "", "")
	cmd.Flags().String("source-space-id", "", "")
	cmd.Flags().String("target-space-id", "", "")
	cmd.Flags().String("target-parent-token", "", "")
	cmd.Flags().String("obj-type", "", "")
	cmd.Flags().String("obj-token", "", "")
	cmd.Flags().Bool("apply", false, "")
	if err := cmd.Flags().Set("node-token", "wik_node"); err != nil {
		t.Fatalf("set --node-token: %v", err)
	}
	if err := cmd.Flags().Set("target-parent-token", "wik_parent"); err != nil {
		t.Fatalf("set --target-parent-token: %v", err)
	}

	runtime := common.TestNewRuntimeContext(cmd, nil)
	dry := WikiMove.DryRun(context.Background(), runtime)
	if dry == nil {
		t.Fatal("DryRun returned nil")
	}

	data, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry run: %v", err)
	}
	if !bytes.Contains(data, []byte(`"description":"3-step orchestration:`)) {
		t.Fatalf("dry run missing 3-step description: %s", string(data))
	}
	if !bytes.Contains(data, []byte(`"target_parent_token":"wik_parent"`)) {
		t.Fatalf("dry run missing target_parent_token body: %s", string(data))
	}
	if !bytes.Contains(data, []byte(`/open-apis/wiki/v2/spaces/\u003cresolved_source_space_id\u003e/nodes/wik_node/move`)) {
		t.Fatalf("dry run missing resolved source placeholder: %s", string(data))
	}
}

func TestWikiMoveDryRunDocsToWikiIncludesTaskPoll(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "wiki +move"}
	cmd.Flags().String("node-token", "", "")
	cmd.Flags().String("source-space-id", "", "")
	cmd.Flags().String("target-space-id", "", "")
	cmd.Flags().String("target-parent-token", "", "")
	cmd.Flags().String("obj-type", "", "")
	cmd.Flags().String("obj-token", "", "")
	cmd.Flags().Bool("apply", false, "")
	if err := cmd.Flags().Set("obj-type", "sheet"); err != nil {
		t.Fatalf("set --obj-type: %v", err)
	}
	if err := cmd.Flags().Set("obj-token", "sheet_token"); err != nil {
		t.Fatalf("set --obj-token: %v", err)
	}
	if err := cmd.Flags().Set("target-space-id", "space_dst"); err != nil {
		t.Fatalf("set --target-space-id: %v", err)
	}
	if err := cmd.Flags().Set("apply", "true"); err != nil {
		t.Fatalf("set --apply: %v", err)
	}

	runtime := common.TestNewRuntimeContext(cmd, nil)
	dry := WikiMove.DryRun(context.Background(), runtime)
	if dry == nil {
		t.Fatal("DryRun returned nil")
	}

	data, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry run: %v", err)
	}
	if !bytes.Contains(data, []byte(`"obj_type":"sheet"`)) || !bytes.Contains(data, []byte(`"apply":true`)) {
		t.Fatalf("dry run missing docs-to-wiki body: %s", string(data))
	}
	if !bytes.Contains(data, []byte(`"task_type":"move"`)) {
		t.Fatalf("dry run missing task polling params: %s", string(data))
	}
}

func TestResolveWikiNodeMoveSpacesUsesSourceAndTargetLookups(t *testing.T) {
	t.Parallel()

	client := &fakeWikiMoveClient{
		nodes: map[string]*wikiNodeRecord{
			"wik_node":   {SpaceID: "space_src"},
			"wik_parent": {SpaceID: "space_dst"},
		},
	}

	sourceSpaceID, targetSpaceID, err := resolveWikiNodeMoveSpaces(context.Background(), client, wikiMoveSpec{
		NodeToken:         "wik_node",
		TargetParentToken: "wik_parent",
	})
	if err != nil {
		t.Fatalf("resolveWikiNodeMoveSpaces() error = %v", err)
	}
	if sourceSpaceID != "space_src" || targetSpaceID != "space_dst" {
		t.Fatalf("resolved spaces = (%q, %q), want (%q, %q)", sourceSpaceID, targetSpaceID, "space_src", "space_dst")
	}
	if strings.Join(client.getNodeCalls, ",") != "wik_node,wik_parent" {
		t.Fatalf("getNodeCalls = %v, want source and target-parent lookups", client.getNodeCalls)
	}
}

func TestResolveWikiNodeMoveSpacesRejectsTargetSpaceMismatch(t *testing.T) {
	t.Parallel()

	client := &fakeWikiMoveClient{
		nodes: map[string]*wikiNodeRecord{
			"wik_parent": {SpaceID: "space_parent"},
		},
	}

	_, _, err := resolveWikiNodeMoveSpaces(context.Background(), client, wikiMoveSpec{
		NodeToken:         "wik_node",
		SourceSpaceID:     "space_src",
		TargetSpaceID:     "space_other",
		TargetParentToken: "wik_parent",
	})
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected mismatch error, got %v", err)
	}
}

func TestRunWikiNodeMoveReturnsResolvedMetadata(t *testing.T) {
	t.Parallel()

	client := &fakeWikiMoveClient{
		nodes: map[string]*wikiNodeRecord{
			"wik_node":   {SpaceID: "space_src"},
			"wik_parent": {SpaceID: "space_dst"},
		},
		moveNode: &wikiNodeRecord{
			SpaceID:         "space_dst",
			NodeToken:       "wik_moved",
			ObjToken:        "sheet_token",
			ObjType:         "sheet",
			ParentNodeToken: "wik_parent",
			NodeType:        wikiNodeTypeOrigin,
			Title:           "Roadmap",
		},
	}

	out, err := runWikiNodeMove(context.Background(), client, wikiMoveSpec{
		NodeToken:         "wik_node",
		TargetParentToken: "wik_parent",
	})
	if err != nil {
		t.Fatalf("runWikiNodeMove() error = %v", err)
	}
	if len(client.moveNodeCalls) != 1 {
		t.Fatalf("MoveNode called %d times, want 1", len(client.moveNodeCalls))
	}
	if client.moveNodeCalls[0].SourceSpaceID != "space_src" {
		t.Fatalf("source space = %q, want %q", client.moveNodeCalls[0].SourceSpaceID, "space_src")
	}
	if out["mode"] != wikiMoveModeNode || out["source_space_id"] != "space_src" || out["target_space_id"] != "space_dst" {
		t.Fatalf("unexpected node move output: %#v", out)
	}
	if out["node_token"] != "wik_moved" || out["title"] != "Roadmap" {
		t.Fatalf("node fields not propagated: %#v", out)
	}
}

func TestRunWikiMoveDispatchesByMode(t *testing.T) {
	t.Parallel()

	runtime, _ := newWikiMoveRuntimeWithScopes(t, core.AsUser, "")
	client := &fakeWikiMoveClient{
		docsResp: &wikiMoveDocsResponse{WikiToken: "wik_ready"},
		moveNode: &wikiNodeRecord{SpaceID: "space_dst", NodeToken: "wik_node"},
	}

	nodeOut, err := runWikiMove(context.Background(), client, runtime, wikiMoveSpec{
		NodeToken:     "wik_node",
		SourceSpaceID: "space_src",
		TargetSpaceID: "space_dst",
	})
	if err != nil {
		t.Fatalf("runWikiMove(node) error = %v", err)
	}
	if nodeOut["mode"] != wikiMoveModeNode {
		t.Fatalf("node mode output = %#v", nodeOut)
	}

	docsOut, err := runWikiMove(context.Background(), client, runtime, wikiMoveSpec{
		ObjType:       "sheet",
		ObjToken:      "sheet_token",
		TargetSpaceID: "space_dst",
	})
	if err != nil {
		t.Fatalf("runWikiMove(docs_to_wiki) error = %v", err)
	}
	if docsOut["mode"] != wikiMoveModeDocsToWiki {
		t.Fatalf("docs-to-wiki output = %#v", docsOut)
	}
}

func TestRunWikiDocsToWikiMoveSyncReady(t *testing.T) {
	t.Parallel()

	runtime, _ := newWikiMoveRuntimeWithScopes(t, core.AsUser, "")
	client := &fakeWikiMoveClient{
		docsResp: &wikiMoveDocsResponse{WikiToken: "wik_ready"},
	}

	out, err := runWikiDocsToWikiMove(context.Background(), client, runtime, wikiMoveSpec{
		ObjType:       "sheet",
		ObjToken:      "sheet_token",
		TargetSpaceID: "space_dst",
	})
	if err != nil {
		t.Fatalf("runWikiDocsToWikiMove() error = %v", err)
	}
	if out["ready"] != true || out["failed"] != false {
		t.Fatalf("expected ready sync result, got %#v", out)
	}
	if out["wiki_token"] != "wik_ready" || out["node_token"] != "wik_ready" {
		t.Fatalf("wiki token fields = %#v", out)
	}
	if len(client.docsToWikiCalls) != 1 || client.docsToWikiCalls[0].TargetSpaceID != "space_dst" {
		t.Fatalf("unexpected docs-to-wiki calls: %#v", client.docsToWikiCalls)
	}
}

func TestRunWikiDocsToWikiMoveApplied(t *testing.T) {
	t.Parallel()

	runtime, _ := newWikiMoveRuntimeWithScopes(t, core.AsUser, "")
	client := &fakeWikiMoveClient{
		docsResp: &wikiMoveDocsResponse{Applied: true},
	}

	out, err := runWikiDocsToWikiMove(context.Background(), client, runtime, wikiMoveSpec{
		ObjType:       "sheet",
		ObjToken:      "sheet_token",
		TargetSpaceID: "space_dst",
	})
	if err != nil {
		t.Fatalf("runWikiDocsToWikiMove() error = %v", err)
	}
	if out["applied"] != true || out["ready"] != false || out["failed"] != false {
		t.Fatalf("expected applied response, got %#v", out)
	}
	if out["status_msg"] != "move request submitted for approval" {
		t.Fatalf("status_msg = %#v", out["status_msg"])
	}
}

func TestRunWikiDocsToWikiMoveAsyncReady(t *testing.T) {
	withSingleWikiMovePoll(t)

	runtime, stderr := newWikiMoveRuntimeWithScopes(t, core.AsUser, "")
	client := &fakeWikiMoveClient{
		docsResp: &wikiMoveDocsResponse{TaskID: "task_123"},
		taskStatuses: []wikiMoveTaskStatus{{
			MoveResults: []wikiMoveTaskResult{{
				Status:    0,
				StatusMsg: "success",
				Node: &wikiNodeRecord{
					SpaceID:   "space_dst",
					NodeToken: "wik_done",
					ObjToken:  "sheet_token",
					ObjType:   "sheet",
					NodeType:  wikiNodeTypeOrigin,
					Title:     "Roadmap",
				},
			}},
		}},
	}

	out, err := runWikiDocsToWikiMove(context.Background(), client, runtime, wikiMoveSpec{
		ObjType:       "sheet",
		ObjToken:      "sheet_token",
		TargetSpaceID: "space_dst",
	})
	if err != nil {
		t.Fatalf("runWikiDocsToWikiMove() error = %v", err)
	}
	if out["task_id"] != "task_123" || out["ready"] != true || out["failed"] != false {
		t.Fatalf("unexpected async-ready output: %#v", out)
	}
	if out["wiki_token"] != "wik_done" || out["title"] != "Roadmap" || out["status_msg"] != "success" {
		t.Fatalf("async-ready output missing flattened fields: %#v", out)
	}
	if !strings.Contains(stderr.String(), "Docs-to-wiki move is async") || !strings.Contains(stderr.String(), "completed successfully") {
		t.Fatalf("stderr = %q, want async progress logs", stderr.String())
	}
}

func TestRunWikiDocsToWikiMoveAsyncTimeoutReturnsNextCommand(t *testing.T) {
	withSingleWikiMovePoll(t)

	runtime, stderr := newWikiMoveRuntimeWithScopes(t, core.AsUser, "")
	client := &fakeWikiMoveClient{
		docsResp: &wikiMoveDocsResponse{TaskID: "task_123"},
		taskStatuses: []wikiMoveTaskStatus{{
			MoveResults: []wikiMoveTaskResult{{Status: 1, StatusMsg: "processing"}},
		}},
	}

	out, err := runWikiDocsToWikiMove(context.Background(), client, runtime, wikiMoveSpec{
		ObjType:       "sheet",
		ObjToken:      "sheet_token",
		TargetSpaceID: "space_dst",
	})
	if err != nil {
		t.Fatalf("runWikiDocsToWikiMove() error = %v", err)
	}
	if out["ready"] != false || out["timed_out"] != true || out["next_command"] != wikiMoveTaskResultCommand("task_123", core.AsUser) {
		t.Fatalf("expected timeout response, got %#v", out)
	}
	if out["status_msg"] != "processing" {
		t.Fatalf("status_msg = %#v, want processing", out["status_msg"])
	}
	if !strings.Contains(stderr.String(), "Continue with") {
		t.Fatalf("stderr = %q, want continuation hint", stderr.String())
	}
}

func TestRunWikiDocsToWikiMoveAsyncFailureReturnsStructuredError(t *testing.T) {
	withSingleWikiMovePoll(t)

	runtime, _ := newWikiMoveRuntimeWithScopes(t, core.AsUser, "")
	client := &fakeWikiMoveClient{
		docsResp: &wikiMoveDocsResponse{TaskID: "task_123"},
		taskStatuses: []wikiMoveTaskStatus{{
			MoveResults: []wikiMoveTaskResult{{Status: -1, StatusMsg: "approval rejected"}},
		}},
	}

	_, err := runWikiDocsToWikiMove(context.Background(), client, runtime, wikiMoveSpec{
		ObjType:       "sheet",
		ObjToken:      "sheet_token",
		TargetSpaceID: "space_dst",
	})
	if err == nil || !strings.Contains(err.Error(), "wiki move task failed: approval rejected") {
		t.Fatalf("expected async failure error, got %v", err)
	}
}

func TestWikiMoveExecuteNodeShortcut(t *testing.T) {
	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{"space_id": "space_src"},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{"space_id": "space_dst"},
			},
		},
	})
	moveStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/wiki/v2/spaces/space_src/nodes/wik_node/move",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"space_id":          "space_dst",
					"node_token":        "wik_moved",
					"obj_token":         "sheet_token",
					"obj_type":          "sheet",
					"parent_node_token": "wik_parent",
					"node_type":         "origin",
					"title":             "Roadmap",
				},
			},
		},
	}
	reg.Register(moveStub)

	err := mountAndRunWiki(t, WikiMove, []string{
		"+move",
		"--node-token", "wik_node",
		"--target-parent-token", "wik_parent",
		"--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	data := decodeWikiEnvelope(t, stdout)
	if data["mode"] != wikiMoveModeNode || data["source_space_id"] != "space_src" || data["target_space_id"] != "space_dst" {
		t.Fatalf("unexpected node shortcut output: %#v", data)
	}
	body := decodeWikiCapturedJSONBody(t, moveStub)
	if body["target_parent_token"] != "wik_parent" {
		t.Fatalf("move body = %#v, want target_parent_token", body)
	}
}

func TestWikiMoveExecuteDocsToWikiShortcutAsyncSuccess(t *testing.T) {
	withSingleWikiMovePoll(t)

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())
	docsStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/wiki/v2/spaces/space_dst/nodes/move_docs_to_wiki",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"task_id": "task_123",
			},
		},
	}
	reg.Register(docsStub)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/tasks/task_123",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"task": map[string]interface{}{
					"task_id": "task_123",
					"move_result": []interface{}{
						map[string]interface{}{
							"status":     0,
							"status_msg": "success",
							"node": map[string]interface{}{
								"space_id":   "space_dst",
								"node_token": "wik_done",
								"obj_token":  "sheet_token",
								"obj_type":   "sheet",
								"node_type":  "origin",
								"title":      "Roadmap",
							},
						},
					},
				},
			},
		},
	})

	err := mountAndRunWiki(t, WikiMove, []string{
		"+move",
		"--obj-type", "sheet",
		"--obj-token", "sheet_token",
		"--target-space-id", "space_dst",
		"--apply",
		"--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	data := decodeWikiEnvelope(t, stdout)
	if data["mode"] != wikiMoveModeDocsToWiki || data["ready"] != true || data["wiki_token"] != "wik_done" {
		t.Fatalf("unexpected docs-to-wiki shortcut output: %#v", data)
	}
	body := decodeWikiCapturedJSONBody(t, docsStub)
	if body["obj_type"] != "sheet" || body["obj_token"] != "sheet_token" || body["apply"] != true {
		t.Fatalf("docs-to-wiki body = %#v", body)
	}
}

func TestPollWikiMoveTaskWrapsRepeatedPollFailuresWithHint(t *testing.T) {
	withSingleWikiMovePoll(t)

	runtime, stderr := newWikiMoveRuntimeWithScopes(t, core.AsUser, "")
	client := &fakeWikiMoveClient{
		taskErrs: []error{output.ErrWithHint(output.ExitAPI, "api_error", "poll failed", "retry original")},
	}

	status, ready, err := pollWikiMoveTask(context.Background(), client, runtime, "task_123")
	if err == nil {
		t.Fatal("expected pollWikiMoveTask() error, got nil")
	}
	if ready {
		t.Fatal("expected ready=false when every poll fails")
	}
	if status.TaskID != "task_123" {
		t.Fatalf("status.TaskID = %q, want %q", status.TaskID, "task_123")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		t.Fatalf("expected structured exit error, got %T %v", err, err)
	}
	if !strings.Contains(exitErr.Detail.Hint, "retry original") || !strings.Contains(exitErr.Detail.Hint, wikiMoveTaskResultCommand("task_123", core.AsUser)) {
		t.Fatalf("hint = %q, want original hint and resume command", exitErr.Detail.Hint)
	}
	if !strings.Contains(stderr.String(), "Wiki move status attempt 1/1 failed") {
		t.Fatalf("stderr = %q, want poll failure log", stderr.String())
	}
}

func TestParseWikiMoveTaskStatusFallbackTaskIDAndNode(t *testing.T) {
	t.Parallel()

	status, err := parseWikiMoveTaskStatus("task_fallback", map[string]interface{}{
		"move_result": []interface{}{
			map[string]interface{}{
				"status":     0,
				"status_msg": "success",
				"node": map[string]interface{}{
					"space_id":   "space_dst",
					"node_token": "wik_done",
					"obj_token":  "sheet_token",
					"obj_type":   "sheet",
					"node_type":  wikiNodeTypeOrigin,
					"title":      "Roadmap",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("parseWikiMoveTaskStatus() error = %v", err)
	}
	if status.TaskID != "task_fallback" {
		t.Fatalf("TaskID = %q, want %q", status.TaskID, "task_fallback")
	}
	if !status.Ready() || status.PrimaryStatusLabel() != "success" {
		t.Fatalf("unexpected parsed status: %+v", status)
	}
	if first := status.FirstResult(); first == nil || first.Node == nil || first.Node.NodeToken != "wik_done" {
		t.Fatalf("parsed node = %+v", first)
	}
}

func TestParseWikiMoveTaskStatusRejectsMissingTask(t *testing.T) {
	t.Parallel()

	_, err := parseWikiMoveTaskStatus("task_123", nil)
	if err == nil || !strings.Contains(err.Error(), "missing task") {
		t.Fatalf("expected missing task error, got %v", err)
	}
}
