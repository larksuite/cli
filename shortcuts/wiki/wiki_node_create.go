// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	wikiNodeTypeOrigin   = "origin"
	wikiNodeTypeShortcut = "shortcut"
	wikiMyLibrarySpaceID = "my_library"

	wikiResolvedByExplicitSpaceID = "explicit_space_id"
	wikiResolvedByParentNode      = "parent_node_token"
	wikiResolvedByMyLibrary       = "my_library"
)

var wikiObjectTypes = []string{
	"sheet",
	"mindnote",
	"bitable",
	"docx",
	"slides",
}

// WikiNodeCreate wraps wiki node creation with shortcut-specific ergonomics:
// it can infer the target space from the parent node or the caller's personal
// document library instead of forcing users to pass a numeric space ID first.
var WikiNodeCreate = common.Shortcut{
	Service:     "wiki",
	Command:     "+node-create",
	Description: "Create a wiki node with automatic space resolution",
	Risk:        "write",
	Scopes:      []string{"wiki:node:create", "wiki:node:read", "wiki:space:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "space-id", Desc: "target wiki space ID; use my_library for the personal document library"},
		{Name: "parent-node-token", Desc: "parent wiki node token; if set, the new node is created under that parent"},
		{Name: "title", Desc: "node title"},
		{Name: "node-type", Default: wikiNodeTypeOrigin, Desc: "node type", Enum: []string{wikiNodeTypeOrigin, wikiNodeTypeShortcut}},
		{Name: "obj-type", Default: "docx", Desc: "target object type", Enum: wikiObjectTypes},
		{Name: "origin-node-token", Desc: "source node token when --node-type=shortcut"},
	},
	Tips: []string{
		"If --space-id and --parent-node-token are both omitted, user identity falls back to my_library.",
		"Use --node-type shortcut --origin-node-token <token> to create a shortcut node.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateWikiNodeCreateSpec(readWikiNodeCreateSpec(runtime), runtime.As())
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		dry := buildWikiNodeCreateDryRun(readWikiNodeCreateSpec(runtime))
		if runtime.IsBot() {
			dry.Desc("After wiki node creation succeeds in bot mode, the CLI will also try to grant the current CLI user full_access (可管理权限) on the new wiki node.")
		}
		return dry
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec := readWikiNodeCreateSpec(runtime)

		fmt.Fprintf(runtime.IO().ErrOut, "Creating wiki node...\n")
		execution, err := runWikiNodeCreate(ctx, wikiNodeCreateAPI{runtime: runtime}, runtime.As(), spec)
		if err != nil {
			return err
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Created wiki node in space %s via %s.\n", execution.ResolvedSpace.SpaceID, execution.ResolvedSpace.ResolvedBy)
		runtime.Out(augmentWikiNodeCreateOutput(runtime, execution), nil)
		return nil
	},
}

// wikiNodeCreateSpec is the normalized CLI input for the shortcut.
type wikiNodeCreateSpec struct {
	SpaceID         string
	ParentNodeToken string
	Title           string
	NodeType        string
	ObjType         string
	OriginNodeToken string
}

// RequestBody converts the normalized shortcut input into the OpenAPI payload.
func (spec wikiNodeCreateSpec) RequestBody() map[string]interface{} {
	body := map[string]interface{}{
		"node_type": spec.NodeType,
		"obj_type":  spec.ObjType,
	}
	if spec.Title != "" {
		body["title"] = spec.Title
	}
	if spec.ParentNodeToken != "" {
		body["parent_node_token"] = spec.ParentNodeToken
	}
	if spec.OriginNodeToken != "" {
		body["origin_node_token"] = spec.OriginNodeToken
	}
	return body
}

// wikiNodeRecord contains the response fields used by the shortcut.
type wikiNodeRecord struct {
	SpaceID         string
	NodeToken       string
	ObjToken        string
	ObjType         string
	ParentNodeToken string
	NodeType        string
	OriginNodeToken string
	Title           string
	HasChild        bool
}

// wikiSpaceRecord contains the response fields used when resolving spaces.
type wikiSpaceRecord struct {
	SpaceID     string
	Name        string
	SpaceType   string
	Visibility  string
	OpenSharing string
}

// wikiResolvedSpace captures both the final numeric space ID and how it was
// derived. Keeping the provenance separate makes the command output easier to
// understand and keeps the resolution logic testable.
type wikiResolvedSpace struct {
	SpaceID    string
	ResolvedBy string
	ParentNode *wikiNodeRecord
}

type wikiNodeCreateExecution struct {
	Node          *wikiNodeRecord
	ResolvedSpace wikiResolvedSpace
}

// wikiNodeCreateClient isolates the network operations so the resolution logic
// can be unit-tested without real HTTP calls.
type wikiNodeCreateClient interface {
	GetNode(ctx context.Context, token string) (*wikiNodeRecord, error)
	GetSpace(ctx context.Context, spaceID string) (*wikiSpaceRecord, error)
	CreateNode(ctx context.Context, spaceID string, spec wikiNodeCreateSpec) (*wikiNodeRecord, error)
}

type wikiNodeCreateAPI struct {
	runtime *common.RuntimeContext
}

func (api wikiNodeCreateAPI) GetNode(ctx context.Context, token string) (*wikiNodeRecord, error) {
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

func (api wikiNodeCreateAPI) GetSpace(ctx context.Context, spaceID string) (*wikiSpaceRecord, error) {
	data, err := api.runtime.CallAPI(
		"GET",
		fmt.Sprintf("/open-apis/wiki/v2/spaces/%s", validate.EncodePathSegment(spaceID)),
		nil,
		nil,
	)
	if err != nil {
		return nil, err
	}
	return parseWikiSpaceRecord(common.GetMap(data, "space"))
}

func (api wikiNodeCreateAPI) CreateNode(ctx context.Context, spaceID string, spec wikiNodeCreateSpec) (*wikiNodeRecord, error) {
	data, err := api.runtime.CallAPI(
		"POST",
		fmt.Sprintf("/open-apis/wiki/v2/spaces/%s/nodes", validate.EncodePathSegment(spaceID)),
		nil,
		spec.RequestBody(),
	)
	if err != nil {
		return nil, err
	}
	return parseWikiNodeRecord(common.GetMap(data, "node"))
}

func readWikiNodeCreateSpec(runtime *common.RuntimeContext) wikiNodeCreateSpec {
	return wikiNodeCreateSpec{
		SpaceID:         strings.TrimSpace(runtime.Str("space-id")),
		ParentNodeToken: strings.TrimSpace(runtime.Str("parent-node-token")),
		Title:           strings.TrimSpace(runtime.Str("title")),
		NodeType:        strings.ToLower(strings.TrimSpace(runtime.Str("node-type"))),
		ObjType:         strings.ToLower(strings.TrimSpace(runtime.Str("obj-type"))),
		OriginNodeToken: strings.TrimSpace(runtime.Str("origin-node-token")),
	}
}

func validateWikiNodeCreateSpec(spec wikiNodeCreateSpec, identity core.Identity) error {
	if err := validateOptionalResourceName(spec.SpaceID, "--space-id"); err != nil {
		return err
	}
	if err := validateOptionalResourceName(spec.ParentNodeToken, "--parent-node-token"); err != nil {
		return err
	}
	if err := validateOptionalResourceName(spec.OriginNodeToken, "--origin-node-token"); err != nil {
		return err
	}

	if spec.NodeType == wikiNodeTypeShortcut && spec.OriginNodeToken == "" {
		return output.ErrValidation("--origin-node-token is required when --node-type=shortcut")
	}
	if spec.NodeType != wikiNodeTypeShortcut && spec.OriginNodeToken != "" {
		return output.ErrValidation("--origin-node-token can only be used when --node-type=shortcut")
	}

	// Bot identity has no meaningful "personal document library" target, so
	// my_library must be rejected explicitly instead of deferring to API-time
	// resolution errors.
	if identity.IsBot() && spec.SpaceID == wikiMyLibrarySpaceID {
		return output.ErrValidation("bot identity does not support --space-id my_library; use an explicit --space-id or --parent-node-token")
	}
	// Bot identity also cannot fall back implicitly, so it requires an explicit
	// target or a parent it can resolve from.
	if identity.IsBot() && spec.SpaceID == "" && spec.ParentNodeToken == "" {
		return output.ErrValidation("bot identity requires --space-id or --parent-node-token")
	}

	return nil
}

func buildWikiNodeCreateDryRun(spec wikiNodeCreateSpec) *common.DryRunAPI {
	dry := common.NewDryRunAPI()
	step := 1

	switch {
	case needsMyLibraryLookup(spec) && spec.ParentNodeToken != "":
		dry.Desc("3-step orchestration: resolve my_library -> resolve parent node -> create wiki node")
	case needsMyLibraryLookup(spec):
		dry.Desc("2-step orchestration: resolve my_library -> create wiki node")
	case spec.ParentNodeToken != "":
		dry.Desc("2-step orchestration: resolve parent node -> create wiki node")
	default:
		dry.Desc("1-step request: create wiki node")
	}

	if needsMyLibraryLookup(spec) {
		dry.GET("/open-apis/wiki/v2/spaces/my_library").
			Desc(fmt.Sprintf("[%d] Resolve my_library space ID", step))
		step++
	}

	if spec.ParentNodeToken != "" {
		dry.GET("/open-apis/wiki/v2/spaces/get_node").
			Desc(fmt.Sprintf("[%d] Resolve parent node space", step)).
			Params(map[string]interface{}{"token": spec.ParentNodeToken})
		step++
	}

	dry.POST(fmt.Sprintf("/open-apis/wiki/v2/spaces/%s/nodes", dryRunWikiNodeCreateSpaceID(spec))).
		Desc(fmt.Sprintf("[%d] Create wiki node", step)).
		Body(spec.RequestBody())

	return dry
}

func dryRunWikiNodeCreateSpaceID(spec wikiNodeCreateSpec) string {
	if spec.SpaceID != "" && spec.SpaceID != wikiMyLibrarySpaceID {
		return spec.SpaceID
	}
	return "<resolved_space_id>"
}

func needsMyLibraryLookup(spec wikiNodeCreateSpec) bool {
	if spec.ParentNodeToken != "" && spec.SpaceID == "" {
		return false
	}
	return spec.SpaceID == "" || spec.SpaceID == wikiMyLibrarySpaceID
}

func runWikiNodeCreate(ctx context.Context, client wikiNodeCreateClient, identity core.Identity, spec wikiNodeCreateSpec) (*wikiNodeCreateExecution, error) {
	resolvedSpace, err := resolveWikiNodeCreateSpace(ctx, client, identity, spec)
	if err != nil {
		return nil, err
	}

	node, err := client.CreateNode(ctx, resolvedSpace.SpaceID, spec)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, output.Errorf(output.ExitAPI, "api_error", "wiki node create returned no node")
	}

	return &wikiNodeCreateExecution{
		Node:          node,
		ResolvedSpace: resolvedSpace,
	}, nil
}

// resolveWikiNodeCreateSpace applies the shortcut's precedence rules:
// explicit space ID wins, then parent-node inference, then my_library fallback.
func resolveWikiNodeCreateSpace(ctx context.Context, client wikiNodeCreateClient, identity core.Identity, spec wikiNodeCreateSpec) (wikiResolvedSpace, error) {
	if spec.SpaceID != "" {
		return resolveWikiNodeCreateSpaceFromExplicitSpace(ctx, client, spec)
	}
	if spec.ParentNodeToken != "" {
		return resolveWikiNodeCreateSpaceFromParentNode(ctx, client, spec.ParentNodeToken)
	}
	if identity.IsBot() {
		return wikiResolvedSpace{}, output.ErrValidation("bot identity requires --space-id or --parent-node-token")
	}
	return resolveWikiNodeCreateSpaceFromMyLibrary(ctx, client)
}

func resolveWikiNodeCreateSpaceFromExplicitSpace(ctx context.Context, client wikiNodeCreateClient, spec wikiNodeCreateSpec) (wikiResolvedSpace, error) {
	resolved := wikiResolvedSpace{
		SpaceID:    spec.SpaceID,
		ResolvedBy: wikiResolvedByExplicitSpaceID,
	}

	if spec.SpaceID == wikiMyLibrarySpaceID {
		space, err := client.GetSpace(ctx, wikiMyLibrarySpaceID)
		if err != nil {
			return wikiResolvedSpace{}, err
		}
		spaceID, err := requireWikiSpaceID(space)
		if err != nil {
			return wikiResolvedSpace{}, err
		}
		resolved.SpaceID = spaceID
		resolved.ResolvedBy = wikiResolvedByMyLibrary
	}

	if spec.ParentNodeToken == "" {
		return resolved, nil
	}

	parent, err := client.GetNode(ctx, spec.ParentNodeToken)
	if err != nil {
		return wikiResolvedSpace{}, err
	}
	parentSpaceID, err := requireWikiNodeSpaceID(parent)
	if err != nil {
		return wikiResolvedSpace{}, err
	}
	if parentSpaceID != resolved.SpaceID {
		return wikiResolvedSpace{}, output.ErrValidation(
			"--space-id %q does not match parent node space %q (resolved space: %q)",
			spec.SpaceID,
			parentSpaceID,
			resolved.SpaceID,
		)
	}

	resolved.ParentNode = parent
	return resolved, nil
}

func resolveWikiNodeCreateSpaceFromParentNode(ctx context.Context, client wikiNodeCreateClient, parentNodeToken string) (wikiResolvedSpace, error) {
	parent, err := client.GetNode(ctx, parentNodeToken)
	if err != nil {
		return wikiResolvedSpace{}, err
	}
	spaceID, err := requireWikiNodeSpaceID(parent)
	if err != nil {
		return wikiResolvedSpace{}, err
	}

	return wikiResolvedSpace{
		SpaceID:    spaceID,
		ResolvedBy: wikiResolvedByParentNode,
		ParentNode: parent,
	}, nil
}

func resolveWikiNodeCreateSpaceFromMyLibrary(ctx context.Context, client wikiNodeCreateClient) (wikiResolvedSpace, error) {
	space, err := client.GetSpace(ctx, wikiMyLibrarySpaceID)
	if err != nil {
		return wikiResolvedSpace{}, err
	}
	spaceID, err := requireWikiSpaceID(space)
	if err != nil {
		return wikiResolvedSpace{}, err
	}

	return wikiResolvedSpace{
		SpaceID:    spaceID,
		ResolvedBy: wikiResolvedByMyLibrary,
	}, nil
}

func requireWikiNodeSpaceID(node *wikiNodeRecord) (string, error) {
	if node != nil && node.SpaceID != "" {
		return node.SpaceID, nil
	}
	return "", output.Errorf(output.ExitAPI, "api_error", "wiki node lookup returned no space_id")
}

func requireWikiSpaceID(space *wikiSpaceRecord) (string, error) {
	if space != nil && space.SpaceID != "" {
		return space.SpaceID, nil
	}
	return "", output.ErrValidation("personal document library was not found, please specify --space-id")
}

// resolveMyLibrarySpaceID calls GET /wiki/v2/spaces/my_library and returns
// the per-user real space_id. Shared by shortcuts that accept the my_library
// alias (e.g. +node-create, +node-list) so the behavior stays consistent.
func resolveMyLibrarySpaceID(runtime *common.RuntimeContext) (string, error) {
	data, err := runtime.CallAPI(
		"GET",
		fmt.Sprintf("/open-apis/wiki/v2/spaces/%s", validate.EncodePathSegment(wikiMyLibrarySpaceID)),
		nil, nil,
	)
	if err != nil {
		return "", err
	}
	space, err := parseWikiSpaceRecord(common.GetMap(data, "space"))
	if err != nil {
		return "", err
	}
	return requireWikiSpaceID(space)
}

func validateOptionalResourceName(value, flagName string) error {
	if value == "" {
		return nil
	}
	if err := validate.ResourceName(value, flagName); err != nil {
		return output.ErrValidation("%s", err)
	}
	return nil
}

func parseWikiNodeRecord(node map[string]interface{}) (*wikiNodeRecord, error) {
	if node == nil {
		return nil, output.Errorf(output.ExitAPI, "api_error", "wiki node response missing node")
	}
	return &wikiNodeRecord{
		SpaceID:         common.GetString(node, "space_id"),
		NodeToken:       common.GetString(node, "node_token"),
		ObjToken:        common.GetString(node, "obj_token"),
		ObjType:         common.GetString(node, "obj_type"),
		ParentNodeToken: common.GetString(node, "parent_node_token"),
		NodeType:        common.GetString(node, "node_type"),
		OriginNodeToken: common.GetString(node, "origin_node_token"),
		Title:           common.GetString(node, "title"),
		HasChild:        common.GetBool(node, "has_child"),
	}, nil
}

func parseWikiSpaceRecord(space map[string]interface{}) (*wikiSpaceRecord, error) {
	if space == nil {
		return nil, output.Errorf(output.ExitAPI, "api_error", "wiki space response missing space")
	}
	return &wikiSpaceRecord{
		SpaceID:     common.GetString(space, "space_id"),
		Name:        common.GetString(space, "name"),
		SpaceType:   common.GetString(space, "space_type"),
		Visibility:  common.GetString(space, "visibility"),
		OpenSharing: common.GetString(space, "open_sharing"),
	}, nil
}

func wikiNodeCreateOutput(execution *wikiNodeCreateExecution) map[string]interface{} {
	node := execution.Node
	return map[string]interface{}{
		"resolved_space_id": execution.ResolvedSpace.SpaceID,
		"resolved_by":       execution.ResolvedSpace.ResolvedBy,
		"space_id":          node.SpaceID,
		"node_token":        node.NodeToken,
		"obj_token":         node.ObjToken,
		"obj_type":          node.ObjType,
		"node_type":         node.NodeType,
		"title":             node.Title,
		"parent_node_token": node.ParentNodeToken,
		"origin_node_token": node.OriginNodeToken,
		"has_child":         node.HasChild,
	}
}

func augmentWikiNodeCreateOutput(runtime *common.RuntimeContext, execution *wikiNodeCreateExecution) map[string]interface{} {
	if execution == nil || execution.Node == nil {
		return map[string]interface{}{}
	}

	out := wikiNodeCreateOutput(execution)
	if grant := common.AutoGrantCurrentUserDrivePermission(runtime, execution.Node.NodeToken, "wiki"); grant != nil {
		out["permission_grant"] = grant
	}
	if u := common.BuildResourceURL(runtime.Config.Brand, "wiki", execution.Node.NodeToken); u != "" {
		out["url"] = u
	}
	return out
}
