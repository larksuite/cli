// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// wikiNodeDeleteObjTypes is the set of obj_type values the delete-node API
// accepts. For delete-node, obj_type="wiki" means the token is a wiki node_token.
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
// helper. It always resolves the target via node_by_token and infers the space
// ID when omitted, or validates the supplied space ID.
var WikiNodeDelete = common.Shortcut{
	Service:     "wiki",
	Command:     "+node-delete",
	Description: "Delete a wiki node, polling the async delete task when needed",
	Risk:        "high-risk-write",
	// Deletion requires wiki:node:create; the preceding node_by_token lookup
	// also requires wiki:node:retrieve, even when --space-id is provided.
	Scopes:    []string{"wiki:node:create", "wiki:node:retrieve"},
	AuthTypes: []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "node-token", Desc: "wiki node_token, cloud-doc obj_token, or a Lark URL embedding one of them", Required: true},
		// Not Required at the cobra level: URL inputs auto-infer obj_type
		// from the path, and the parser enforces explicit obj_type for raw
		// tokens. Forcing Cobra Required here breaks the URL ergonomic.
		{Name: "obj-type", Desc: "deletion token kind: wiki uses the resolved node_token, other types use obj_token; required for raw input (URL inputs auto-infer)", Enum: wikiNodeDeleteObjTypes},
		{Name: "space-id", Desc: "wiki space ID; auto-resolved via node_by_token when omitted"},
		{Name: "include-children", Type: "bool", Default: "true", Desc: "cascade delete the subtree (default); pass --include-children=false to lift direct children up to the parent"},
	},
	Tips: []string{
		"Deletion is irreversible; double-check --node-token and --obj-type before running.",
		"This is a high-risk-write command; pass --yes to confirm the deletion.",
		"--node-token accepts a raw token (wikcnXXX, docxXXX, ...) or a Lark URL like https://feishu.cn/wiki/<token> or https://feishu.cn/docx/<token>; URL paths also imply --obj-type.",
		"Run +node-get first to confirm space_id / obj_type when in doubt.",
		"Resolving the node also calls node_by_token and requires Wiki node read permission, including when --space-id is provided.",
		"Async deletes return a task_id; this command polls for a bounded window and then returns a follow-up drive +task_result command.",
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
	ResolveNode(ctx context.Context, token string) (*wikiNodeRecord, error)
	DeleteNode(ctx context.Context, spaceID string, spec wikiNodeDeleteSpec) (string, error)
	GetDeleteNodeTask(ctx context.Context, taskID string) (wikiAsyncTaskStatus, error)
}

type wikiNodeDeleteAPI struct {
	runtime *common.RuntimeContext
}

func (api wikiNodeDeleteAPI) ResolveNode(ctx context.Context, token string) (*wikiNodeRecord, error) {
	return lookupWikiNode(api.runtime, token)
}

func (api wikiNodeDeleteAPI) DeleteNode(ctx context.Context, spaceID string, spec wikiNodeDeleteSpec) (string, error) {
	data, err := api.runtime.CallAPITyped(
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
	data, err := api.runtime.CallAPITyped(
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
		return wikiNodeDeleteSpec{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "--node-token is required").WithParam("--node-token")
	}

	spec := wikiNodeDeleteSpec{
		ObjType:         strings.ToLower(strings.TrimSpace(rawObjType)),
		SpaceID:         strings.TrimSpace(rawSpaceID),
		IncludeChildren: includeChildren,
	}

	if strings.Contains(tokenInput, "://") {
		u, err := url.Parse(tokenInput)
		if err != nil || u.Path == "" {
			return wikiNodeDeleteSpec{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "--node-token URL is malformed: %q", tokenInput).WithParam("--node-token")
		}
		token, urlObjType, ok := tokenAndObjTypeFromWikiURL(u.Path)
		if !ok {
			return wikiNodeDeleteSpec{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"unsupported --node-token URL path %q: expected /wiki/, /docx/, /doc/, /sheets/, /base/, /mindnote/, /slides/, or /file/ followed by a token",
				u.Path,
			).WithParam("--node-token")
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
			return wikiNodeDeleteSpec{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--obj-type %q does not match the obj_type %q implied by the URL path; pass only one",
				spec.ObjType, inferred,
			).WithParam("--obj-type")
		}
	} else if strings.ContainsAny(tokenInput, "/?#") {
		return wikiNodeDeleteSpec{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--node-token must be a raw token or a full URL; partial paths are not accepted: %q",
			tokenInput,
		).WithParam("--node-token")
	} else {
		spec.NodeToken = tokenInput
		spec.SourceKind = "raw"
	}

	if spec.ObjType == "" {
		return wikiNodeDeleteSpec{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--obj-type is required (one of: %s)",
			strings.Join(wikiNodeDeleteObjTypes, ", "),
		).WithParam("--obj-type")
	}
	if !isValidWikiDeleteObjType(spec.ObjType) {
		return wikiNodeDeleteSpec{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--obj-type %q is not valid; pick one of: %s",
			spec.ObjType, strings.Join(wikiNodeDeleteObjTypes, ", "),
		).WithParam("--obj-type")
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
		"async-aware: always resolve the target via node_by_token (requires Wiki node read access, including with --space-id) -> delete wiki node -> poll wiki delete-node task when task_id is returned",
	)

	dry.GET("/open-apis/wiki/v2/spaces/node_by_token").
		Desc("[1] Resolve node and space").
		Params(map[string]interface{}{"token": spec.NodeToken})
	spaceID := "<resolved_space_id>"
	if spec.SpaceID != "" {
		spaceID = validate.EncodePathSegment(spec.SpaceID)
	}
	token := "<resolved_node_token>"
	if spec.ObjType != "wiki" {
		token = "<resolved_obj_token>"
	}
	dry.DELETE(fmt.Sprintf("/open-apis/wiki/v2/spaces/%s/nodes/%s", spaceID, token)).
		Desc("[2] Delete wiki node").
		Body(spec.RequestBody())

	dry.GET("/open-apis/wiki/v2/tasks/:task_id").
		Desc("[N] Poll wiki delete-node task result when async").
		Set("task_id", "<task_id>").
		Params(map[string]interface{}{"task_type": wikiAsyncTaskTypeDeleteNode})

	return dry
}

func runWikiNodeDelete(ctx context.Context, client wikiNodeDeleteClient, runtime *common.RuntimeContext, spec wikiNodeDeleteSpec) (map[string]interface{}, error) {
	spaceID, err := resolveWikiNodeDeleteSpaceID(ctx, client, &spec)
	if err != nil {
		return nil, err
	}

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
		out["timed_out"] = true
		out["next_command"] = nextCommand
	}
	return out, nil
}

// resolveWikiNodeDeleteSpaceID resolves the mutation token and checks any explicit space.
func resolveWikiNodeDeleteSpaceID(ctx context.Context, client wikiNodeDeleteClient, spec *wikiNodeDeleteSpec) (string, error) {
	node, err := client.ResolveNode(ctx, spec.NodeToken)
	if err != nil {
		return "", err
	}
	spaceID, err := requireWikiNodeSpaceID(node)
	if err != nil {
		return "", err
	}
	if spec.SpaceID != "" && spec.SpaceID != spaceID {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--space-id does not match the resolved node space").WithParam("--space-id")
	}
	if spec.ObjType == "wiki" {
		if node.NodeToken == "" {
			return "", errs.NewInternalError(errs.SubtypeInvalidResponse, "wiki node lookup returned no node_token")
		}
		spec.NodeToken = node.NodeToken
	} else {
		// A shortcut's obj_token belongs to its origin document. Converting it
		// would change the deletion target from the shortcut to that document.
		if node.NodeType == wikiNodeTypeShortcut {
			return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "deleting a Wiki shortcut requires --obj-type wiki").
				WithParam("--obj-type").WithHint("Use --obj-type wiki to delete the shortcut itself.")
		}
		if node.ObjToken == "" || node.ObjType == "" {
			return "", errs.NewInternalError(errs.SubtypeInvalidResponse, "wiki node lookup returned no obj_token or obj_type")
		}
		if spec.ObjType != node.ObjType {
			return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--obj-type does not match the resolved document type").WithParam("--obj-type")
		}
		spec.NodeToken = node.ObjToken
	}
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
	p, ok := errs.ProblemOf(err)
	if !ok {
		return err
	}
	var hint string
	switch p.Code {
	case wikiDeleteNodeErrCodeApprovalRequired:
		hint = "this wiki node has delete-approval enabled; ask the user to apply via the Wiki UI (CLI cannot bypass approval)"
	case wikiDeleteNodeErrCodeSubtreeTooLarge:
		hint = "the subtree is too large to cascade-delete in one call; pass --include-children=false to keep the children (they will be moved up to the parent), or delete sub-trees first"
	}
	if hint == "" {
		return err
	}
	// Append the hint in place so the typed error keeps its category / subtype /
	// code / log_id (per ERROR_CONTRACT.md "propagate typed errors unchanged").
	if existing := strings.TrimSpace(p.Hint); existing != "" {
		hint = existing + "\n" + hint
	}
	p.Hint = hint
	return err
}
