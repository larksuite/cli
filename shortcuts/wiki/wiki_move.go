// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var (
	wikiMovePollAttempts = 30
	wikiMovePollInterval = 2 * time.Second
)

const (
	wikiMoveModeNode       = "node"
	wikiMoveModeDocsToWiki = "docs_to_wiki"
)

var wikiMoveObjectTypes = []string{
	"doc",
	"sheet",
	"bitable",
	"mindnote",
	"docx",
	"file",
	"slides",
}

// WikiMove moves an existing wiki node inside Wiki or migrates a Drive
// document into Wiki with bounded polling for async task completion.
var WikiMove = common.Shortcut{
	Service:     "wiki",
	Command:     "+move",
	Description: "Move a wiki node, or move a Drive document into Wiki",
	Risk:        "write",
	Scopes:      []string{"wiki:node:move", "wiki:node:read", "wiki:space:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "node-token", Desc: "wiki node token to move inside Wiki"},
		{Name: "source-space-id", Desc: "source wiki space ID for --node-token; if omitted, it is resolved from the node token"},
		{Name: "target-space-id", Desc: "target wiki space ID; required for docs-to-wiki, optional for node move when --target-parent-token is set"},
		{Name: "target-parent-token", Desc: "target parent wiki node token; if omitted for docs-to-wiki, the document is moved to the target space root"},
		{Name: "obj-type", Desc: "Drive document type for docs-to-wiki mode", Enum: wikiMoveObjectTypes},
		{Name: "obj-token", Desc: "Drive document token for docs-to-wiki mode"},
		{Name: "apply", Type: "bool", Desc: "submit a move request when the caller lacks permission to move the document immediately"},
	},
	Tips: []string{
		"Use --node-token to move an existing wiki node inside or across wiki spaces.",
		"Use --obj-type and --obj-token to move a Drive document into Wiki.",
		"If docs-to-wiki returns a long-running task, this command polls for a bounded window and then prints a follow-up drive +task_result command.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec := readWikiMoveSpec(runtime)
		// `my_library` is a per-user personal-library alias; it has no meaning
		// for a tenant_access_token (--as bot), so reject early with a clear
		// hint instead of letting the API return a confusing error.
		if runtime.As().IsBot() && spec.TargetSpaceID == wikiMyLibrarySpaceID {
			return output.ErrValidation("--target-space-id my_library is a per-user personal library alias and cannot be used with --as bot; resolve it to a real space_id first via `lark-cli wiki spaces get --params '{\"space_id\":\"my_library\"}' --as user`")
		}
		return validateWikiMoveSpec(spec)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return buildWikiMoveDryRun(readWikiMoveSpec(runtime))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec := readWikiMoveSpec(runtime)
		fmt.Fprintf(runtime.IO().ErrOut, "Running wiki move (%s)...\n", spec.Mode())

		out, err := runWikiMove(ctx, wikiMoveAPI{runtime: runtime}, runtime, spec)
		if err != nil {
			return err
		}

		runtime.Out(out, nil)
		return nil
	},
}

type wikiMoveSpec struct {
	NodeToken         string
	SourceSpaceID     string
	TargetSpaceID     string
	TargetParentToken string
	ObjType           string
	ObjToken          string
	Apply             bool
}

func (spec wikiMoveSpec) Mode() string {
	if spec.NodeToken != "" {
		return wikiMoveModeNode
	}
	return wikiMoveModeDocsToWiki
}

func (spec wikiMoveSpec) NodeMoveBody() map[string]interface{} {
	body := map[string]interface{}{}
	if spec.TargetParentToken != "" {
		body["target_parent_token"] = spec.TargetParentToken
	}
	if spec.TargetSpaceID != "" {
		body["target_space_id"] = spec.TargetSpaceID
	}
	return body
}

func (spec wikiMoveSpec) DocsToWikiBody() map[string]interface{} {
	body := map[string]interface{}{
		"obj_type":  spec.ObjType,
		"obj_token": spec.ObjToken,
	}
	if spec.TargetParentToken != "" {
		body["parent_wiki_token"] = spec.TargetParentToken
	}
	if spec.Apply {
		body["apply"] = true
	}
	return body
}

type wikiMoveTaskResult struct {
	Node      *wikiNodeRecord
	Status    int
	StatusMsg string
}

type wikiMoveTaskStatus struct {
	TaskID      string
	MoveResults []wikiMoveTaskResult
}

func (s wikiMoveTaskStatus) Ready() bool {
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

func (s wikiMoveTaskStatus) Failed() bool {
	for _, result := range s.MoveResults {
		if result.Status < 0 {
			return true
		}
	}
	return false
}

func (s wikiMoveTaskStatus) Pending() bool {
	return !s.Ready() && !s.Failed()
}

func (s wikiMoveTaskStatus) FirstResult() *wikiMoveTaskResult {
	if len(s.MoveResults) == 0 {
		return nil
	}
	return &s.MoveResults[0]
}

// primaryResult picks the most informative move_result for top-level status
// surfacing: prefer a failing entry so multi-doc tasks don't mask failures
// behind an earlier success, then a still-processing entry, and finally fall
// back to the first entry.
func (s wikiMoveTaskStatus) primaryResult() *wikiMoveTaskResult {
	for i := range s.MoveResults {
		if s.MoveResults[i].Status < 0 {
			return &s.MoveResults[i]
		}
	}
	for i := range s.MoveResults {
		if s.MoveResults[i].Status > 0 {
			return &s.MoveResults[i]
		}
	}
	return s.FirstResult()
}

func (s wikiMoveTaskStatus) PrimaryStatusCode() int {
	if r := s.primaryResult(); r != nil {
		return r.Status
	}
	return 1
}

func (s wikiMoveTaskStatus) PrimaryStatusLabel() string {
	if r := s.primaryResult(); r != nil {
		if msg := strings.TrimSpace(r.StatusMsg); msg != "" {
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

type wikiMoveDocsResponse struct {
	WikiToken string
	TaskID    string
	Applied   bool
}

type wikiMoveClient interface {
	GetNode(ctx context.Context, token string) (*wikiNodeRecord, error)
	MoveNode(ctx context.Context, sourceSpaceID string, spec wikiMoveSpec) (*wikiNodeRecord, error)
	MoveDocsToWiki(ctx context.Context, targetSpaceID string, spec wikiMoveSpec) (*wikiMoveDocsResponse, error)
	GetMoveTask(ctx context.Context, taskID string) (wikiMoveTaskStatus, error)
}

type wikiMoveAPI struct {
	runtime *common.RuntimeContext
}

func (api wikiMoveAPI) GetNode(ctx context.Context, token string) (*wikiNodeRecord, error) {
	data, err := api.runtime.CallAPI(
		"GET",
		"/open-apis/wiki/v2/spaces/get_node",
		map[string]interface{}{"token": token},
		nil,
	)
	if err != nil {
		return nil, err
	}
	return parseWikiNodeRecord(common.GetMap(data, "node"))
}

func (api wikiMoveAPI) MoveNode(ctx context.Context, sourceSpaceID string, spec wikiMoveSpec) (*wikiNodeRecord, error) {
	data, err := api.runtime.CallAPI(
		"POST",
		fmt.Sprintf(
			"/open-apis/wiki/v2/spaces/%s/nodes/%s/move",
			validate.EncodePathSegment(sourceSpaceID),
			validate.EncodePathSegment(spec.NodeToken),
		),
		nil,
		spec.NodeMoveBody(),
	)
	if err != nil {
		return nil, err
	}
	return parseWikiNodeRecord(common.GetMap(data, "node"))
}

func (api wikiMoveAPI) MoveDocsToWiki(ctx context.Context, targetSpaceID string, spec wikiMoveSpec) (*wikiMoveDocsResponse, error) {
	data, err := api.runtime.CallAPI(
		"POST",
		fmt.Sprintf(
			"/open-apis/wiki/v2/spaces/%s/nodes/move_docs_to_wiki",
			validate.EncodePathSegment(targetSpaceID),
		),
		nil,
		spec.DocsToWikiBody(),
	)
	if err != nil {
		return nil, err
	}

	return &wikiMoveDocsResponse{
		WikiToken: common.GetString(data, "wiki_token"),
		TaskID:    common.GetString(data, "task_id"),
		Applied:   common.GetBool(data, "applied"),
	}, nil
}

func (api wikiMoveAPI) GetMoveTask(ctx context.Context, taskID string) (wikiMoveTaskStatus, error) {
	data, err := api.runtime.CallAPI(
		"GET",
		fmt.Sprintf("/open-apis/wiki/v2/tasks/%s", validate.EncodePathSegment(taskID)),
		map[string]interface{}{"task_type": "move"},
		nil,
	)
	if err != nil {
		return wikiMoveTaskStatus{}, err
	}
	return parseWikiMoveTaskStatus(taskID, common.GetMap(data, "task"))
}

func readWikiMoveSpec(runtime *common.RuntimeContext) wikiMoveSpec {
	return wikiMoveSpec{
		NodeToken:         strings.TrimSpace(runtime.Str("node-token")),
		SourceSpaceID:     strings.TrimSpace(runtime.Str("source-space-id")),
		TargetSpaceID:     strings.TrimSpace(runtime.Str("target-space-id")),
		TargetParentToken: strings.TrimSpace(runtime.Str("target-parent-token")),
		ObjType:           strings.ToLower(strings.TrimSpace(runtime.Str("obj-type"))),
		ObjToken:          strings.TrimSpace(runtime.Str("obj-token")),
		Apply:             runtime.Bool("apply"),
	}
}

func validateWikiMoveSpec(spec wikiMoveSpec) error {
	if err := validateOptionalResourceName(spec.NodeToken, "--node-token"); err != nil {
		return err
	}
	if err := validateOptionalResourceName(spec.SourceSpaceID, "--source-space-id"); err != nil {
		return err
	}
	if err := validateOptionalResourceName(spec.TargetSpaceID, "--target-space-id"); err != nil {
		return err
	}
	if err := validateOptionalResourceName(spec.TargetParentToken, "--target-parent-token"); err != nil {
		return err
	}
	if err := validateOptionalResourceName(spec.ObjToken, "--obj-token"); err != nil {
		return err
	}

	if spec.NodeToken != "" {
		if spec.ObjType != "" || spec.ObjToken != "" || spec.Apply {
			return output.ErrValidation("--node-token cannot be combined with --obj-type, --obj-token, or --apply")
		}
		if spec.TargetParentToken == "" && spec.TargetSpaceID == "" {
			return output.ErrValidation("--target-parent-token and --target-space-id cannot both be empty for wiki node move")
		}
		return nil
	}

	if spec.SourceSpaceID != "" {
		return output.ErrValidation("--source-space-id can only be used with --node-token")
	}
	if spec.ObjType == "" && spec.ObjToken == "" && !spec.Apply {
		return output.ErrValidation("provide --node-token for wiki node move, or provide --obj-type and --obj-token for docs-to-wiki move")
	}
	if spec.ObjType == "" {
		return output.ErrValidation("--obj-type is required for docs-to-wiki move")
	}
	if spec.ObjToken == "" {
		return output.ErrValidation("--obj-token is required for docs-to-wiki move")
	}
	if spec.TargetSpaceID == "" {
		return output.ErrValidation("--target-space-id is required for docs-to-wiki move")
	}

	return nil
}

func buildWikiMoveDryRun(spec wikiMoveSpec) *common.DryRunAPI {
	dry := common.NewDryRunAPI()
	switch spec.Mode() {
	case wikiMoveModeNode:
		step := 1
		switch {
		case spec.SourceSpaceID == "" && spec.TargetParentToken != "":
			dry.Desc("3-step orchestration: resolve source node -> resolve target parent -> move wiki node")
		case spec.SourceSpaceID == "":
			dry.Desc("2-step orchestration: resolve source node -> move wiki node")
		case spec.TargetParentToken != "":
			dry.Desc("2-step orchestration: resolve target parent -> move wiki node")
		default:
			dry.Desc("1-step request: move wiki node")
		}

		if spec.SourceSpaceID == "" {
			dry.GET("/open-apis/wiki/v2/spaces/get_node").
				Desc(fmt.Sprintf("[%d] Resolve source space from node token", step)).
				Params(map[string]interface{}{"token": spec.NodeToken})
			step++
		}
		if spec.TargetParentToken != "" {
			dry.GET("/open-apis/wiki/v2/spaces/get_node").
				Desc(fmt.Sprintf("[%d] Resolve target parent node", step)).
				Params(map[string]interface{}{"token": spec.TargetParentToken})
			step++
		}

		dry.POST(fmt.Sprintf(
			"/open-apis/wiki/v2/spaces/%s/nodes/%s/move",
			dryRunWikiMoveSourceSpaceID(spec),
			validate.EncodePathSegment(spec.NodeToken),
		)).
			Desc(fmt.Sprintf("[%d] Move wiki node", step)).
			Body(spec.NodeMoveBody())
	case wikiMoveModeDocsToWiki:
		dry.Desc("2-step orchestration: move Drive document into Wiki -> poll wiki task result when task_id is returned")
		dry.POST(fmt.Sprintf(
			"/open-apis/wiki/v2/spaces/%s/nodes/move_docs_to_wiki",
			dryRunWikiMoveTargetSpaceID(spec),
		)).
			Desc("[1] Move Drive document into Wiki").
			Body(spec.DocsToWikiBody())
		dry.GET("/open-apis/wiki/v2/tasks/:task_id").
			Desc("[2] Poll wiki move task result when async").
			Set("task_id", "<task_id>").
			Params(map[string]interface{}{"task_type": "move"})
	default:
		dry.Set("error", "unknown wiki move mode")
	}
	return dry
}

func dryRunWikiMoveSourceSpaceID(spec wikiMoveSpec) string {
	if spec.SourceSpaceID != "" {
		return validate.EncodePathSegment(spec.SourceSpaceID)
	}
	return "<resolved_source_space_id>"
}

func dryRunWikiMoveTargetSpaceID(spec wikiMoveSpec) string {
	if spec.TargetSpaceID != "" {
		return validate.EncodePathSegment(spec.TargetSpaceID)
	}
	return "<target_space_id>"
}

func runWikiMove(ctx context.Context, client wikiMoveClient, runtime *common.RuntimeContext, spec wikiMoveSpec) (map[string]interface{}, error) {
	switch spec.Mode() {
	case wikiMoveModeNode:
		return runWikiNodeMove(ctx, client, spec)
	case wikiMoveModeDocsToWiki:
		return runWikiDocsToWikiMove(ctx, client, runtime, spec)
	default:
		return nil, output.ErrValidation("unknown wiki move mode")
	}
}

func runWikiNodeMove(ctx context.Context, client wikiMoveClient, spec wikiMoveSpec) (map[string]interface{}, error) {
	sourceSpaceID, targetSpaceID, err := resolveWikiNodeMoveSpaces(ctx, client, spec)
	if err != nil {
		return nil, err
	}

	node, err := client.MoveNode(ctx, sourceSpaceID, spec)
	if err != nil {
		return nil, err
	}

	out := map[string]interface{}{
		"mode":            wikiMoveModeNode,
		"source_space_id": sourceSpaceID,
		"target_space_id": targetSpaceID,
	}
	appendWikiNodeOutput(out, node)
	return out, nil
}

func resolveWikiNodeMoveSpaces(ctx context.Context, client wikiMoveClient, spec wikiMoveSpec) (string, string, error) {
	// Node move requests may start from just a node token and/or a target parent.
	// Resolve both ends up front so we can fail on space mismatches before sending
	// the mutation request.
	sourceSpaceID := spec.SourceSpaceID
	if sourceSpaceID == "" {
		sourceNode, err := client.GetNode(ctx, spec.NodeToken)
		if err != nil {
			return "", "", err
		}
		sourceSpaceID, err = requireWikiNodeSpaceID(sourceNode)
		if err != nil {
			return "", "", err
		}
	}

	targetSpaceID := spec.TargetSpaceID
	if spec.TargetParentToken != "" {
		targetParent, err := client.GetNode(ctx, spec.TargetParentToken)
		if err != nil {
			return "", "", err
		}
		parentSpaceID, err := requireWikiNodeSpaceID(targetParent)
		if err != nil {
			return "", "", err
		}
		if targetSpaceID == "" {
			targetSpaceID = parentSpaceID
		} else if targetSpaceID != parentSpaceID {
			return "", "", output.ErrValidation(
				"--target-space-id %q does not match target parent node space %q",
				spec.TargetSpaceID,
				parentSpaceID,
			)
		}
	}

	if targetSpaceID == "" {
		targetSpaceID = sourceSpaceID
	}

	return sourceSpaceID, targetSpaceID, nil
}

func runWikiDocsToWikiMove(ctx context.Context, client wikiMoveClient, runtime *common.RuntimeContext, spec wikiMoveSpec) (map[string]interface{}, error) {
	response, err := client.MoveDocsToWiki(ctx, spec.TargetSpaceID, spec)
	if err != nil {
		return nil, err
	}

	out := map[string]interface{}{
		"mode":                wikiMoveModeDocsToWiki,
		"obj_type":            spec.ObjType,
		"obj_token":           spec.ObjToken,
		"target_space_id":     spec.TargetSpaceID,
		"target_parent_token": spec.TargetParentToken,
	}

	// move_docs_to_wiki has three success-shaped responses: immediate completion,
	// approval-request submission, or an async task that must be polled.
	switch {
	case response.WikiToken != "":
		out["ready"] = true
		out["failed"] = false
		out["wiki_token"] = response.WikiToken
		out["node_token"] = response.WikiToken
		return out, nil
	case response.Applied:
		out["ready"] = false
		out["failed"] = false
		out["applied"] = true
		out["status_msg"] = "move request submitted for approval"
		return out, nil
	case response.TaskID != "":
		fmt.Fprintf(runtime.IO().ErrOut, "Docs-to-wiki move is async, polling task %s...\n", response.TaskID)
		status, ready, err := pollWikiMoveTask(ctx, client, runtime, response.TaskID)
		if err != nil {
			return nil, err
		}

		out["task_id"] = response.TaskID
		out["ready"] = ready
		out["failed"] = status.Failed()
		out["status"] = status.PrimaryStatusCode()
		out["status_msg"] = status.PrimaryStatusLabel()
		if first := status.FirstResult(); first != nil {
			appendWikiNodeOutput(out, first.Node)
			if first.Node != nil && first.Node.NodeToken != "" {
				out["wiki_token"] = first.Node.NodeToken
			}
		}
		if !ready {
			nextCommand := wikiMoveTaskResultCommand(response.TaskID, runtime.As())
			fmt.Fprintf(runtime.IO().ErrOut, "Wiki move task is still in progress. Continue with: %s\n", nextCommand)
			out["timed_out"] = true
			out["next_command"] = nextCommand
		}
		return out, nil
	default:
		return nil, output.Errorf(output.ExitAPI, "api_error", "move_docs_to_wiki returned neither wiki_token, task_id, nor applied result")
	}
}

func wikiMoveTaskResultCommand(taskID string, identity core.Identity) string {
	asFlag := string(identity)
	if asFlag == "" {
		asFlag = "user"
	}
	return fmt.Sprintf("lark-cli drive +task_result --scenario wiki_move --task-id %s --as %s", taskID, asFlag)
}

func pollWikiMoveTask(ctx context.Context, client wikiMoveClient, runtime *common.RuntimeContext, taskID string) (wikiMoveTaskStatus, bool, error) {
	lastStatus := wikiMoveTaskStatus{TaskID: taskID}
	var lastErr error
	hadSuccessfulPoll := false

	// The move request itself already succeeded. Treat poll failures as transient
	// until every attempt fails, then return a resume hint instead of discarding
	// the task identifier.
	for attempt := 1; attempt <= wikiMovePollAttempts; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return lastStatus, false, ctx.Err()
			case <-time.After(wikiMovePollInterval):
			}
		}

		status, err := client.GetMoveTask(ctx, taskID)
		if err != nil {
			lastErr = err
			fmt.Fprintf(runtime.IO().ErrOut, "Wiki move status attempt %d/%d failed: %v\n", attempt, wikiMovePollAttempts, err)
			continue
		}
		lastStatus = status
		hadSuccessfulPoll = true

		if status.Ready() {
			fmt.Fprintf(runtime.IO().ErrOut, "Wiki move task completed successfully.\n")
			return status, true, nil
		}
		if status.Failed() {
			return status, false, output.Errorf(output.ExitAPI, "api_error", "wiki move task failed: %s", status.PrimaryStatusLabel())
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Wiki move status %d/%d: %s\n", attempt, wikiMovePollAttempts, status.PrimaryStatusLabel())
	}

	if !hadSuccessfulPoll && lastErr != nil {
		nextCommand := wikiMoveTaskResultCommand(taskID, runtime.As())
		hint := fmt.Sprintf(
			"the wiki move task was created but every status poll failed (task_id=%s)\nretry status lookup with: %s",
			taskID,
			nextCommand,
		)
		var exitErr *output.ExitError
		if errors.As(lastErr, &exitErr) && exitErr.Detail != nil {
			if strings.TrimSpace(exitErr.Detail.Hint) != "" {
				hint = exitErr.Detail.Hint + "\n" + hint
			}
			return lastStatus, false, output.ErrWithHint(exitErr.Code, exitErr.Detail.Type, exitErr.Detail.Message, hint)
		}
		return lastStatus, false, output.ErrWithHint(output.ExitAPI, "api_error", lastErr.Error(), hint)
	}

	return lastStatus, false, nil
}

func parseWikiMoveTaskStatus(taskID string, task map[string]interface{}) (wikiMoveTaskStatus, error) {
	if task == nil {
		return wikiMoveTaskStatus{}, output.Errorf(output.ExitAPI, "api_error", "wiki task response missing task")
	}

	status := wikiMoveTaskStatus{
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

		var node *wikiNodeRecord
		if nodeMap := common.GetMap(resultMap, "node"); nodeMap != nil {
			parsedNode, err := parseWikiNodeRecord(nodeMap)
			if err != nil {
				return wikiMoveTaskStatus{}, err
			}
			node = parsedNode
		}

		status.MoveResults = append(status.MoveResults, wikiMoveTaskResult{
			Node:      node,
			Status:    int(common.GetFloat(resultMap, "status")),
			StatusMsg: common.GetString(resultMap, "status_msg"),
		})
	}

	return status, nil
}

func appendWikiNodeOutput(out map[string]interface{}, node *wikiNodeRecord) {
	if out == nil || node == nil {
		return
	}
	out["space_id"] = node.SpaceID
	out["node_token"] = node.NodeToken
	out["obj_token"] = node.ObjToken
	out["obj_type"] = node.ObjType
	out["parent_node_token"] = node.ParentNodeToken
	out["node_type"] = node.NodeType
	out["origin_node_token"] = node.OriginNodeToken
	out["title"] = node.Title
	out["has_child"] = node.HasChild
}
