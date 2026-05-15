// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// wikiNodeDeleteObjTypes is the set of obj_type values the delete-node API
// accepts. Unlike wikiNodeGetObjTypeEnum this includes "wiki" — for
// delete-node, obj_type="wiki" means the token is a wiki node_token, whereas
// the get_node API omits obj_type for node_tokens.
var wikiNodeDeleteObjTypes = []string{
	"wiki", "doc", "docx", "sheet", "bitable", "mindnote", "slides", "file",
}

var (
	wikiDeleteNodePollAttempts = 30
	wikiDeleteNodePollInterval = 2 * time.Second
)

// Lark wiki API error codes the delete-node API surfaces with actionable
// CLI workarounds. The full list is in the OpenAPI spec; we only special-case
// the codes whose remediation is non-obvious (UI approval, subtree size).
const (
	wikiDeleteNodeErrCodeApprovalRequired = 131011
	wikiDeleteNodeErrCodeSubtreeTooLarge  = 131003
)

// WikiNodeDelete deletes a wiki node (or pulls a cloud doc out of Wiki). The
// API mirrors +delete-space — synchronous on small deletes, async with a
// task_id for cascade deletes — so this shortcut shares the async-polling
// helper. Space ID is optional: when omitted, +node-delete first looks up the
// node via get_node to resolve the space ID so callers do not have to chain
// commands.
var WikiNodeDelete = common.Shortcut{
	Service:     "wiki",
	Command:     "+node-delete",
	Description: "Delete a wiki node, polling the async delete task when needed",
	Risk:        "high-risk-write",
	// API spec lists wiki:node:create as the only declared scope for the
	// delete endpoint. Naming is unfortunate, but the scope-preflight needs
	// the literal string.
	Scopes:    []string{"wiki:node:create"},
	AuthTypes: []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "node-token", Desc: "wiki node_token, cloud-doc obj_token, or a Lark URL embedding one of them", Required: true},
		// Not Required at the cobra level: URL inputs auto-infer obj_type
		// from the path, and the parser enforces explicit obj_type for raw
		// tokens. Forcing Cobra Required here breaks the URL ergonomic.
		{Name: "obj-type", Desc: "token kind; no default — pass explicitly when --node-token is a raw token (URL inputs auto-infer)", Enum: wikiNodeDeleteObjTypes},
		{Name: "space-id", Desc: "wiki space ID; auto-resolved via get_node when omitted"},
		{Name: "include-children", Type: "bool", Default: "true", Desc: "cascade delete the subtree (default); pass --include-children=false to lift direct children up to the parent"},
	},
	Tips: []string{
		"Deletion is irreversible; double-check --node-token and --obj-type before running.",
		"This is a high-risk-write command; pass --yes to confirm the deletion.",
		"--node-token accepts a raw token (wikcnXXX, docxXXX, ...) or a Lark URL like https://feishu.cn/wiki/<token> or https://feishu.cn/docx/<token>; URL paths also imply --obj-type.",
		"Run +node-get first to confirm space_id / obj_type when in doubt.",
		"Auto-resolving space_id (when --space-id is omitted) also calls get_node, which needs the wiki:node:retrieve scope; pass --space-id to skip that lookup if your token only carries wiki:node:create.",
		"Async deletes return a task_id; this command polls for a bounded window and then prints a follow-up drive +task_result command.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := readWikiNodeDeleteSpec(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec, err := readWikiNodeDeleteSpec(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return buildWikiNodeDeleteDryRun(spec)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec, err := readWikiNodeDeleteSpec(runtime)
		if err != nil {
			return err
		}

		out, err := runWikiNodeDelete(ctx, wikiNodeDeleteAPI{runtime: runtime}, runtime, spec)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

// wikiNodeDeleteSpec is the normalized input for the shortcut. Token / ObjType
// reconcile URL inputs with the explicit flags; SourceKind is purely for the
// dry-run description string.
type wikiNodeDeleteSpec struct {
	NodeToken       string
	ObjType         string
	SpaceID         string
	IncludeChildren bool
	SourceKind      string // "raw" | "url"
}

// RequestBody builds the JSON body for DELETE /spaces/{id}/nodes/{token}.
func (spec wikiNodeDeleteSpec) RequestBody() map[string]interface{} {
	return map[string]interface{}{
		"obj_type":         spec.ObjType,
		"include_children": spec.IncludeChildren,
	}
}

// wikiNodeDeleteClient isolates the network operations so business logic can
// be unit-tested without real HTTP calls. Mirrors wikiDeleteSpaceClient.
type wikiNodeDeleteClient interface {
	ResolveNode(ctx context.Context, token, objType string) (*wikiNodeRecord, error)
	DeleteNode(ctx context.Context, spaceID string, spec wikiNodeDeleteSpec) (string, error)
	GetDeleteNodeTask(ctx context.Context, taskID string) (wikiAsyncTaskStatus, error)
}

type wikiNodeDeleteAPI struct {
	runtime *common.RuntimeContext
}

func (api wikiNodeDeleteAPI) ResolveNode(ctx context.Context, token, objType string) (*wikiNodeRecord, error) {
	params := map[string]interface{}{"token": token}
	// get_node takes obj_type only when the token is an obj_token. For
	// wiki node_tokens the API rejects an obj_type kwarg, so omit it.
	if objType != "" && objType != "wiki" {
		params["obj_type"] = objType
	}
	data, err := api.runtime.CallAPI("GET", "/open-apis/wiki/v2/spaces/get_node", params, nil)
	if err != nil {
		return nil, err
	}
	return parseWikiNodeRecord(common.GetMap(data, "node"))
}

func (api wikiNodeDeleteAPI) DeleteNode(ctx context.Context, spaceID string, spec wikiNodeDeleteSpec) (string, error) {
	data, err := api.runtime.CallAPI(
		"DELETE",
		fmt.Sprintf(
			"/open-apis/wiki/v2/spaces/%s/nodes/%s",
			validate.EncodePathSegment(spaceID),
			validate.EncodePathSegment(spec.NodeToken),
		),
		nil,
		spec.RequestBody(),
	)
	if err != nil {
		return "", wrapWikiNodeDeleteAPIError(err)
	}
	return common.GetString(data, "task_id"), nil
}

func (api wikiNodeDeleteAPI) GetDeleteNodeTask(ctx context.Context, taskID string) (wikiAsyncTaskStatus, error) {
	data, err := api.runtime.CallAPI(
		"GET",
		fmt.Sprintf("/open-apis/wiki/v2/tasks/%s", validate.EncodePathSegment(taskID)),
		map[string]interface{}{"task_type": wikiAsyncTaskTypeDeleteNode},
		nil,
	)
	if err != nil {
		return wikiAsyncTaskStatus{}, err
	}
	return parseWikiAsyncTaskStatus(taskID, common.GetMap(data, "task"), wikiAsyncResultSimpleTask)
}

func readWikiNodeDeleteSpec(runtime *common.RuntimeContext) (wikiNodeDeleteSpec, error) {
	return parseWikiNodeDeleteSpec(
		runtime.Str("node-token"),
		runtime.Str("obj-type"),
		runtime.Str("space-id"),
		runtime.Bool("include-children"),
	)
}

// parseWikiNodeDeleteSpec normalizes the raw flag values: extracts a token
// from a URL when provided, reconciles URL-implied obj_type against the
// explicit flag, and validates that the resulting obj_type is one the delete
// API accepts.
func parseWikiNodeDeleteSpec(rawToken, rawObjType, rawSpaceID string, includeChildren bool) (wikiNodeDeleteSpec, error) {
	tokenInput := strings.TrimSpace(rawToken)
	if tokenInput == "" {
		return wikiNodeDeleteSpec{}, output.ErrValidation("--node-token is required")
	}

	spec := wikiNodeDeleteSpec{
		ObjType:         strings.ToLower(strings.TrimSpace(rawObjType)),
		SpaceID:         strings.TrimSpace(rawSpaceID),
		IncludeChildren: includeChildren,
	}

	if strings.Contains(tokenInput, "://") {
		u, err := url.Parse(tokenInput)
		if err != nil || u.Path == "" {
			return wikiNodeDeleteSpec{}, output.ErrValidation("--node-token URL is malformed: %q", tokenInput)
		}
		token, urlObjType, ok := tokenAndObjTypeFromWikiURL(u.Path)
		if !ok {
			return wikiNodeDeleteSpec{}, output.ErrValidation(
				"unsupported --node-token URL path %q: expected /wiki/, /docx/, /doc/, /sheets/, /base/, /mindnote/, /slides/, or /file/ followed by a token",
				u.Path,
			)
		}
		spec.NodeToken = token
		spec.SourceKind = "url"

		// /wiki/<token> implies node_token → obj_type=wiki for the delete API.
		// Cloud doc paths (/docx/, /sheets/, ...) already give us a concrete type.
		inferred := urlObjType
		if inferred == "" {
			inferred = "wiki"
		}
		switch {
		case spec.ObjType == "":
			spec.ObjType = inferred
		case spec.ObjType != inferred:
			return wikiNodeDeleteSpec{}, output.ErrValidation(
				"--obj-type %q does not match the obj_type %q implied by the URL path; pass only one",
				spec.ObjType, inferred,
			)
		}
	} else if strings.ContainsAny(tokenInput, "/?#") {
		return wikiNodeDeleteSpec{}, output.ErrValidation(
			"--node-token must be a raw token or a full URL; partial paths are not accepted: %q",
			tokenInput,
		)
	} else {
		spec.NodeToken = tokenInput
		spec.SourceKind = "raw"
	}

	if spec.ObjType == "" {
		return wikiNodeDeleteSpec{}, output.ErrValidation(
			"--obj-type is required (one of: %s)",
			strings.Join(wikiNodeDeleteObjTypes, ", "),
		)
	}
	if !isValidWikiDeleteObjType(spec.ObjType) {
		return wikiNodeDeleteSpec{}, output.ErrValidation(
			"--obj-type %q is not valid; pick one of: %s",
			spec.ObjType, strings.Join(wikiNodeDeleteObjTypes, ", "),
		)
	}
	if err := validateOptionalResourceName(spec.NodeToken, "--node-token"); err != nil {
		return wikiNodeDeleteSpec{}, err
	}
	if err := validateOptionalResourceName(spec.SpaceID, "--space-id"); err != nil {
		return wikiNodeDeleteSpec{}, err
	}
	return spec, nil
}

func isValidWikiDeleteObjType(v string) bool {
	for _, t := range wikiNodeDeleteObjTypes {
		if v == t {
			return true
		}
	}
	return false
}

func buildWikiNodeDeleteDryRun(spec wikiNodeDeleteSpec) *common.DryRunAPI {
	dry := common.NewDryRunAPI().Desc(
		"async-aware: delete wiki node -> poll wiki delete-node task when task_id is returned (auto-resolves space_id via get_node when --space-id is omitted)",
	)

	if spec.SpaceID == "" {
		params := map[string]interface{}{"token": spec.NodeToken}
		if spec.ObjType != "" && spec.ObjType != "wiki" {
			params["obj_type"] = spec.ObjType
		}
		dry.GET("/open-apis/wiki/v2/spaces/get_node").
			Desc("[1] Resolve space_id via get_node").
			Params(params)
		dry.DELETE(fmt.Sprintf(
			"/open-apis/wiki/v2/spaces/%s/nodes/%s",
			"<resolved_space_id>",
			validate.EncodePathSegment(spec.NodeToken),
		)).
			Desc("[2] Delete wiki node").
			Body(spec.RequestBody())
	} else {
		dry.DELETE(fmt.Sprintf(
			"/open-apis/wiki/v2/spaces/%s/nodes/%s",
			validate.EncodePathSegment(spec.SpaceID),
			validate.EncodePathSegment(spec.NodeToken),
		)).
			Desc("[1] Delete wiki node").
			Body(spec.RequestBody())
	}

	dry.GET("/open-apis/wiki/v2/tasks/:task_id").
		Desc("[N] Poll wiki delete-node task result when async").
		Set("task_id", "<task_id>").
		Params(map[string]interface{}{"task_type": wikiAsyncTaskTypeDeleteNode})

	return dry
}

func runWikiNodeDelete(ctx context.Context, client wikiNodeDeleteClient, runtime *common.RuntimeContext, spec wikiNodeDeleteSpec) (map[string]interface{}, error) {
	spaceID, err := resolveWikiNodeDeleteSpaceID(ctx, client, runtime, spec)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(runtime.IO().ErrOut, "Deleting wiki node %s in space %s (obj_type=%s, include_children=%t)...\n",
		common.MaskToken(spec.NodeToken), common.MaskToken(spaceID), spec.ObjType, spec.IncludeChildren)

	taskID, err := client.DeleteNode(ctx, spaceID, spec)
	if err != nil {
		return nil, err
	}

	out := map[string]interface{}{
		"space_id":         spaceID,
		"node_token":       spec.NodeToken,
		"obj_type":         spec.ObjType,
		"include_children": spec.IncludeChildren,
	}

	// Empty task_id means the delete completed synchronously. Match the
	// shape used by +delete-space so downstream scripts can read `status`
	// uniformly regardless of which branch fired.
	if taskID == "" {
		out["ready"] = true
		out["failed"] = false
		out["status"] = wikiAsyncStatusSuccess
		out["status_msg"] = wikiAsyncStatusSuccess
		return out, nil
	}

	fmt.Fprintf(runtime.IO().ErrOut, "Wiki node delete is async, polling task %s...\n", taskID)
	nextCommand := wikiDeleteNodeTaskResultCommand(taskID, runtime.As())
	status, ready, err := pollWikiAsyncTask(
		ctx, runtime, taskID, "delete-node",
		wikiDeleteNodePollAttempts, wikiDeleteNodePollInterval,
		func(ctx context.Context, id string) (wikiAsyncTaskStatus, error) {
			return client.GetDeleteNodeTask(ctx, id)
		},
		nextCommand,
	)
	if err != nil {
		return nil, err
	}

	out["task_id"] = taskID
	out["ready"] = ready
	out["failed"] = status.Failed()
	out["status"] = status.StatusCode()
	out["status_msg"] = status.StatusLabel()

	if !ready {
		fmt.Fprintf(runtime.IO().ErrOut, "Wiki delete-node task is still in progress. Continue with: %s\n", nextCommand)
		out["timed_out"] = true
		out["next_command"] = nextCommand
	}
	return out, nil
}

// resolveWikiNodeDeleteSpaceID returns the explicit space_id when the caller
// supplied one, otherwise resolves it via get_node. The latter saves callers
// from running +node-get first when they only have a node_token.
func resolveWikiNodeDeleteSpaceID(ctx context.Context, client wikiNodeDeleteClient, runtime *common.RuntimeContext, spec wikiNodeDeleteSpec) (string, error) {
	if spec.SpaceID != "" {
		return spec.SpaceID, nil
	}
	fmt.Fprintf(runtime.IO().ErrOut, "Resolving space_id via get_node for token %s...\n", common.MaskToken(spec.NodeToken))
	node, err := client.ResolveNode(ctx, spec.NodeToken, spec.ObjType)
	if err != nil {
		return "", err
	}
	spaceID, err := requireWikiNodeSpaceID(node)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(runtime.IO().ErrOut, "Resolved to space %s\n", common.MaskToken(spaceID))
	return spaceID, nil
}

func wikiDeleteNodeTaskResultCommand(taskID string, identity core.Identity) string {
	asFlag := string(identity)
	if asFlag == "" {
		asFlag = "user"
	}
	return fmt.Sprintf("lark-cli drive +task_result --scenario wiki_delete_node --task-id %s --as %s", taskID, asFlag)
}

// wrapWikiNodeDeleteAPIError attaches actionable hints to the two Lark error
// codes whose remediation lives outside the CLI:
//   - 131011: approval required (deletion gated by Wiki UI approval flow)
//   - 131003: subtree too large to cascade-delete (must split or use
//     include_children=false)
//
// Other codes pass through untouched so the generic error envelope still
// surfaces the original code+message.
func wrapWikiNodeDeleteAPIError(err error) error {
	if err == nil {
		return nil
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		return err
	}
	var hint string
	switch exitErr.Detail.Code {
	case wikiDeleteNodeErrCodeApprovalRequired:
		hint = "this wiki node has delete-approval enabled; ask the user to apply via the Wiki UI (CLI cannot bypass approval)"
	case wikiDeleteNodeErrCodeSubtreeTooLarge:
		hint = "the subtree is too large to cascade-delete in one call; pass --include-children=false to keep the children (they will be moved up to the parent), or delete sub-trees first"
	}
	if hint == "" {
		return err
	}
	if existing := strings.TrimSpace(exitErr.Detail.Hint); existing != "" {
		hint = existing + "\n" + hint
	}
	// ErrWithHint drops the upstream Detail.Code / Detail / Risk fields; build
	// the ExitError by hand so the Lark error code stays available to logs and
	// downstream pivots.
	return &output.ExitError{
		Code: exitErr.Code,
		Detail: &output.ErrDetail{
			Type:       exitErr.Detail.Type,
			Code:       exitErr.Detail.Code,
			Message:    exitErr.Detail.Message,
			Hint:       hint,
			ConsoleURL: exitErr.Detail.ConsoleURL,
			Risk:       exitErr.Detail.Risk,
			Detail:     exitErr.Detail.Detail,
		},
	}
}
