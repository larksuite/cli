// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// WikiNodeMove moves a wiki node to a target space or under a target parent node.
var WikiNodeMove = common.Shortcut{
	Service:     "wiki",
	Command:     "+node-move",
	Description: "Move a wiki node to a target space or parent node",
	Risk:        "write",
	Scopes:      []string{"wiki:node:move"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "space-id", Desc: "source wiki space ID", Required: true},
		{Name: "node-token", Desc: "node token to move", Required: true},
		{Name: "target-space-id", Desc: "target wiki space ID; required if --target-parent-node-token is not set"},
		{Name: "target-parent-node-token", Desc: "target parent node token; required if --target-space-id is not set"},
	},
	Tips: []string{
		"At least one of --target-space-id or --target-parent-node-token must be provided.",
		"Moving is recursive — the entire subtree under the node is moved together.",
		"Unlike copy, move removes the node from its original location.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateOptionalResourceName(strings.TrimSpace(runtime.Str("space-id")), "--space-id"); err != nil {
			return err
		}
		if err := validateOptionalResourceName(strings.TrimSpace(runtime.Str("node-token")), "--node-token"); err != nil {
			return err
		}
		targetSpaceID := strings.TrimSpace(runtime.Str("target-space-id"))
		targetParent := strings.TrimSpace(runtime.Str("target-parent-node-token"))
		if targetSpaceID == "" && targetParent == "" {
			return output.ErrValidation("at least one of --target-space-id or --target-parent-node-token is required")
		}
		if err := validateOptionalResourceName(targetSpaceID, "--target-space-id"); err != nil {
			return err
		}
		return validateOptionalResourceName(targetParent, "--target-parent-node-token")
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spaceID := strings.TrimSpace(runtime.Str("space-id"))
		nodeToken := strings.TrimSpace(runtime.Str("node-token"))
		return common.NewDryRunAPI().
			POST(fmt.Sprintf("/open-apis/wiki/v2/spaces/%s/nodes/%s/move",
				validate.EncodePathSegment(spaceID),
				validate.EncodePathSegment(nodeToken))).
			Body(buildNodeMoveBody(runtime)).
			Set("space_id", spaceID).
			Set("node_token", nodeToken)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spaceID := strings.TrimSpace(runtime.Str("space-id"))
		nodeToken := strings.TrimSpace(runtime.Str("node-token"))

		fmt.Fprintf(runtime.IO().ErrOut, "Moving wiki node %s from space %s\n",
			common.MaskToken(nodeToken), common.MaskToken(spaceID))

		data, err := runtime.CallAPI("POST",
			fmt.Sprintf("/open-apis/wiki/v2/spaces/%s/nodes/%s/move",
				validate.EncodePathSegment(spaceID),
				validate.EncodePathSegment(nodeToken)),
			nil, buildNodeMoveBody(runtime))
		if err != nil {
			return err
		}

		node, err := parseWikiNodeRecord(common.GetMap(data, "node"))
		if err != nil {
			return err
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Moved to node %s in space %s\n",
			common.MaskToken(node.NodeToken), common.MaskToken(node.SpaceID))
		runtime.Out(wikiNodeMoveOutput(node), nil)
		return nil
	},
}

func buildNodeMoveBody(runtime *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{}
	if v := strings.TrimSpace(runtime.Str("target-space-id")); v != "" {
		body["target_space_id"] = v
	}
	if v := strings.TrimSpace(runtime.Str("target-parent-node-token")); v != "" {
		body["target_parent_token"] = v
	}
	return body
}

func wikiNodeMoveOutput(node *wikiNodeRecord) map[string]interface{} {
	return map[string]interface{}{
		"space_id":          node.SpaceID,
		"node_token":        node.NodeToken,
		"obj_token":         node.ObjToken,
		"obj_type":          node.ObjType,
		"node_type":         node.NodeType,
		"title":             node.Title,
		"parent_node_token": node.ParentNodeToken,
		"has_child":         node.HasChild,
	}
}
