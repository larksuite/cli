// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

type mockDriveTaskResultTokenResolver struct {
	token  string
	scopes string
	err    error
}

func (m *mockDriveTaskResultTokenResolver) ResolveToken(ctx context.Context, req credential.TokenSpec) (*credential.TokenResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	token := m.token
	if token == "" {
		token = "test-token"
	}
	return &credential.TokenResult{Token: token, Scopes: m.scopes}, nil
}

func newDriveTaskResultRuntimeWithScopes(t *testing.T, as core.Identity, scopes string) *common.RuntimeContext {
	t.Helper()

	cfg := driveTestConfig()
	factory, _, _, _ := cmdutil.TestFactory(t, cfg)
	factory.Credential = credential.NewCredentialProvider(nil, nil, &mockDriveTaskResultTokenResolver{scopes: scopes}, nil)

	runtime := common.TestNewRuntimeContextWithIdentity(&cobra.Command{Use: "drive +task_result"}, cfg, as)
	runtime.Factory = factory
	return runtime
}

func TestDriveTaskResultDryRunWikiMoveIncludesTaskTypeParam(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "drive +task_result"}
	cmd.Flags().String("scenario", "", "")
	cmd.Flags().String("ticket", "", "")
	cmd.Flags().String("task-id", "", "")
	cmd.Flags().String("file-token", "", "")
	if err := cmd.Flags().Set("scenario", "wiki_move"); err != nil {
		t.Fatalf("set --scenario: %v", err)
	}
	if err := cmd.Flags().Set("task-id", "task_123"); err != nil {
		t.Fatalf("set --task-id: %v", err)
	}

	runtime := common.TestNewRuntimeContext(cmd, nil)
	dry := DriveTaskResult.DryRun(context.Background(), runtime)
	if dry == nil {
		t.Fatal("DryRun returned nil")
	}

	data, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry run: %v", err)
	}

	var got struct {
		API []struct {
			Params map[string]interface{} `json:"params"`
		} `json:"api"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal dry run json: %v", err)
	}
	if len(got.API) != 1 {
		t.Fatalf("expected 1 API call, got %d", len(got.API))
	}
	if got.API[0].Params["task_type"] != "move" {
		t.Fatalf("wiki move params = %#v, want task_type=move", got.API[0].Params)
	}
}

func TestDriveTaskResultWikiMoveIncludesFlattenedNodeFields(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
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

	err := mountAndRunDrive(t, DriveTaskResult, []string{
		"+task_result",
		"--scenario", "wiki_move",
		"--task-id", "task_123",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeDriveEnvelope(t, stdout)
	if data["scenario"] != "wiki_move" || data["task_id"] != "task_123" {
		t.Fatalf("unexpected wiki_move envelope: %#v", data)
	}
	if data["ready"] != true || data["failed"] != false || data["wiki_token"] != "wik_done" {
		t.Fatalf("unexpected readiness fields: %#v", data)
	}
	if data["title"] != "Roadmap" || data["obj_type"] != "sheet" || data["space_id"] != "space_dst" {
		t.Fatalf("flattened node fields missing: %#v", data)
	}
	moveResults, ok := data["move_results"].([]interface{})
	if !ok || len(moveResults) != 1 {
		t.Fatalf("move_results = %#v, want one result", data["move_results"])
	}
}

func TestValidateDriveTaskResultScopesWikiMoveRequiresWikiScope(t *testing.T) {
	t.Parallel()

	runtime := newDriveTaskResultRuntimeWithScopes(t, core.AsUser, "drive:drive.metadata:readonly")
	err := validateDriveTaskResultScopes(context.Background(), runtime, "wiki_move")
	if err == nil || !strings.Contains(err.Error(), "missing required scope(s): wiki:node:move") {
		t.Fatalf("expected missing wiki scope error, got %v", err)
	}
}

func TestValidateDriveTaskResultScopesWikiMoveAcceptsWikiScope(t *testing.T) {
	t.Parallel()

	runtime := newDriveTaskResultRuntimeWithScopes(t, core.AsUser, "wiki:node:move")
	err := validateDriveTaskResultScopes(context.Background(), runtime, "wiki_move")
	if err != nil {
		t.Fatalf("validateDriveTaskResultScopes() error = %v", err)
	}
}

func TestValidateDriveTaskResultScopesDriveScenariosRequireDriveScope(t *testing.T) {
	t.Parallel()

	runtime := newDriveTaskResultRuntimeWithScopes(t, core.AsUser, "wiki:node:move")
	err := validateDriveTaskResultScopes(context.Background(), runtime, "import")
	if err == nil || !strings.Contains(err.Error(), "missing required scope(s): drive:drive.metadata:readonly") {
		t.Fatalf("expected missing drive scope error, got %v", err)
	}
}

func TestParseWikiMoveTaskQueryStatusFallbackTaskIDAndNode(t *testing.T) {
	t.Parallel()

	status, err := parseWikiMoveTaskQueryStatus("task_fallback", map[string]interface{}{
		"move_result": []interface{}{
			map[string]interface{}{
				"status":     0,
				"status_msg": "success",
				"node": map[string]interface{}{
					"space_id":   "space_dst",
					"node_token": "wik_done",
					"obj_token":  "sheet_token",
					"obj_type":   "sheet",
					"title":      "Roadmap",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("parseWikiMoveTaskQueryStatus() error = %v", err)
	}
	if status.TaskID != "task_fallback" || !status.Ready() || status.PrimaryStatusLabel() != "success" {
		t.Fatalf("unexpected parsed status: %+v", status)
	}
	if first := status.FirstResult(); first == nil || first.Node == nil || first.Node["node_token"] != "wik_done" {
		t.Fatalf("parsed node = %+v", first)
	}
}

func TestParseWikiMoveTaskQueryStatusRejectsMissingTask(t *testing.T) {
	t.Parallel()

	_, err := parseWikiMoveTaskQueryStatus("task_123", nil)
	if err == nil || !strings.Contains(err.Error(), "missing task") {
		t.Fatalf("expected missing task error, got %v", err)
	}
}
