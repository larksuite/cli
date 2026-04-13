// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

func validateDriveTaskResultScopes(ctx context.Context, runtime *common.RuntimeContext, scenario string) error {
	result, err := runtime.Factory.Credential.ResolveToken(ctx, credential.NewTokenSpec(runtime.As(), runtime.Config.AppID))
	if err == nil && result != nil && result.Scopes != "" {
		var required []string
		switch scenario {
		case "import", "export", "task_check":
			required = []string{"drive:drive.metadata:readonly"}
		case "wiki_move":
			required = []string{"wiki:node:move"}
		}

		return requireDriveScopes(result.Scopes, required)
	}

	return nil
}

func requireDriveScopes(storedScopes string, required []string) error {
	if len(required) == 0 {
		return nil
	}

	missing := missingDriveScopes(storedScopes, required)
	if len(missing) == 0 {
		return nil
	}

	return output.ErrWithHint(output.ExitAuth, "missing_scope",
		fmt.Sprintf("missing required scope(s): %s", strings.Join(missing, ", ")),
		fmt.Sprintf("run `lark-cli auth login --scope \"%s\"` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login.", strings.Join(missing, " ")))
}

func missingDriveScopes(storedScopes string, required []string) []string {
	granted := make(map[string]bool)
	for _, scope := range strings.Fields(storedScopes) {
		granted[scope] = true
	}

	missing := make([]string, 0, len(required))
	for _, scope := range required {
		if !granted[scope] {
			missing = append(missing, scope)
		}
	}
	return missing
}

type wikiMoveTaskResultStatus struct {
	Node      map[string]interface{}
	Status    int
	StatusMsg string
}

type wikiMoveTaskQueryStatus struct {
	TaskID      string
	MoveResults []wikiMoveTaskResultStatus
}

func (s wikiMoveTaskQueryStatus) Ready() bool {
	if len(s.MoveResults) == 0 {
		return false
	}
	for _, result := range s.MoveResults {
		if result.Status != 0 {
			return false
		}
	}
	return true
}

func (s wikiMoveTaskQueryStatus) Failed() bool {
	for _, result := range s.MoveResults {
		if result.Status < 0 {
			return true
		}
	}
	return false
}

func (s wikiMoveTaskQueryStatus) FirstResult() *wikiMoveTaskResultStatus {
	if len(s.MoveResults) == 0 {
		return nil
	}
	return &s.MoveResults[0]
}

func (s wikiMoveTaskQueryStatus) PrimaryStatusCode() int {
	if first := s.FirstResult(); first != nil {
		return first.Status
	}
	return 1
}

func (s wikiMoveTaskQueryStatus) PrimaryStatusLabel() string {
	if first := s.FirstResult(); first != nil {
		if msg := strings.TrimSpace(first.StatusMsg); msg != "" {
			return msg
		}
	}
	switch {
	case s.Ready():
		return "success"
	case s.Failed():
		return "failure"
	default:
		return "processing"
	}
}

func queryWikiMoveTask(runtime *common.RuntimeContext, taskID string) (map[string]interface{}, error) {
	status, err := getWikiMoveTaskStatus(runtime, taskID)
	if err != nil {
		return nil, err
	}

	out := map[string]interface{}{
		"scenario":   "wiki_move",
		"task_id":    status.TaskID,
		"ready":      status.Ready(),
		"failed":     status.Failed(),
		"status":     status.PrimaryStatusCode(),
		"status_msg": status.PrimaryStatusLabel(),
	}

	moveResults := make([]map[string]interface{}, 0, len(status.MoveResults))
	for _, result := range status.MoveResults {
		item := map[string]interface{}{
			"status":     result.Status,
			"status_msg": result.StatusMsg,
		}
		if result.Node != nil {
			item["node"] = result.Node
		}
		moveResults = append(moveResults, item)
	}
	if len(moveResults) > 0 {
		out["move_results"] = moveResults
	}

	if first := status.FirstResult(); first != nil {
		// Mirror the first moved node at the top level so follow-up commands can
		// reuse a stable field set without digging into move_results[0].node.
		if first.Node != nil {
			out["node"] = first.Node
			appendWikiMoveNodeFields(out, first.Node)
			if token := common.GetString(first.Node, "node_token"); token != "" {
				out["wiki_token"] = token
			}
		}
	}

	return out, nil
}

func getWikiMoveTaskStatus(runtime *common.RuntimeContext, taskID string) (wikiMoveTaskQueryStatus, error) {
	if err := validate.ResourceName(taskID, "--task-id"); err != nil {
		return wikiMoveTaskQueryStatus{}, output.ErrValidation("%s", err)
	}

	data, err := runtime.CallAPI(
		"GET",
		fmt.Sprintf("/open-apis/wiki/v2/tasks/%s", validate.EncodePathSegment(taskID)),
		map[string]interface{}{"task_type": "move"},
		nil,
	)
	if err != nil {
		return wikiMoveTaskQueryStatus{}, err
	}

	return parseWikiMoveTaskQueryStatus(taskID, common.GetMap(data, "task"))
}

func parseWikiMoveTaskQueryStatus(taskID string, task map[string]interface{}) (wikiMoveTaskQueryStatus, error) {
	if task == nil {
		return wikiMoveTaskQueryStatus{}, output.Errorf(output.ExitAPI, "api_error", "wiki task response missing task")
	}

	status := wikiMoveTaskQueryStatus{
		TaskID: common.GetString(task, "task_id"),
	}
	if status.TaskID == "" {
		status.TaskID = taskID
	}

	for _, item := range common.GetSlice(task, "move_result") {
		resultMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		status.MoveResults = append(status.MoveResults, wikiMoveTaskResultStatus{
			Node:      parseWikiMoveTaskNode(common.GetMap(resultMap, "node")),
			Status:    int(common.GetFloat(resultMap, "status")),
			StatusMsg: common.GetString(resultMap, "status_msg"),
		})
	}

	return status, nil
}

func parseWikiMoveTaskNode(node map[string]interface{}) map[string]interface{} {
	if node == nil {
		return nil
	}

	return map[string]interface{}{
		"space_id":          common.GetString(node, "space_id"),
		"node_token":        common.GetString(node, "node_token"),
		"obj_token":         common.GetString(node, "obj_token"),
		"obj_type":          common.GetString(node, "obj_type"),
		"parent_node_token": common.GetString(node, "parent_node_token"),
		"node_type":         common.GetString(node, "node_type"),
		"origin_node_token": common.GetString(node, "origin_node_token"),
		"title":             common.GetString(node, "title"),
		"has_child":         common.GetBool(node, "has_child"),
	}
}

func appendWikiMoveNodeFields(out, node map[string]interface{}) {
	if out == nil || node == nil {
		return
	}
	out["space_id"] = common.GetString(node, "space_id")
	out["node_token"] = common.GetString(node, "node_token")
	out["obj_token"] = common.GetString(node, "obj_token")
	out["obj_type"] = common.GetString(node, "obj_type")
	out["parent_node_token"] = common.GetString(node, "parent_node_token")
	out["node_type"] = common.GetString(node, "node_type")
	out["origin_node_token"] = common.GetString(node, "origin_node_token")
	out["title"] = common.GetString(node, "title")
	out["has_child"] = common.GetBool(node, "has_child")
}
