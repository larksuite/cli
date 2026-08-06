// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"strings"

	"github.com/larksuite/cli/errs"
)

// WikiNode is the structured result of unwrapping a wiki node token via wiki
// get_node. ObjType/ObjToken identify the underlying resource — the thing drive
// +fetch / +inspect actually reads; NodeToken/SpaceID/NodeType carry the wiki
// provenance recorded in resource.source. OriginNodeToken is set for shortcut
// nodes (node_type=shortcut) and points at the origin node.
type WikiNode struct {
	ObjType         string
	ObjToken        string
	NodeToken       string
	SpaceID         string
	NodeType        string
	OriginNodeToken string
}

// ResolveWikiNode unwraps a wiki node token into the underlying resource via
// GET /open-apis/wiki/v2/spaces/get_node (obj_type=wiki, the API default: treat
// the token as a node_token). It returns the node's obj_type/obj_token (what to
// actually fetch) plus the wiki provenance (node_token/space_id/node_type) for
// source metadata. drive +inspect and drive +fetch share this helper so wiki
// unwrapping lives in one place rather than inline copies.
//
// obj_type=wiki is the explicit default the API uses when a token is a
// node_token; passing it makes the intent unambiguous. Returns a typed
// InvalidResponse error when get_node succeeds but the node lacks
// obj_type/obj_token (incomplete data); transport/permission errors come back as
// the underlying CallAPITyped error for the caller to annotate.
func ResolveWikiNode(runtime *RuntimeContext, wikiToken string) (*WikiNode, error) {
	data, err := runtime.CallAPITyped("GET", "/open-apis/wiki/v2/spaces/get_node",
		map[string]interface{}{"token": wikiToken, "obj_type": "wiki"}, nil)
	if err != nil {
		return nil, err
	}
	node := GetMap(data, "node")
	objType := strings.TrimSpace(GetString(node, "obj_type"))
	objToken := strings.TrimSpace(GetString(node, "obj_token"))
	if objType == "" || objToken == "" {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"wiki get_node returned incomplete node data (obj_type=%q, obj_token=%q)", objType, objToken)
	}
	return &WikiNode{
		ObjType:         objType,
		ObjToken:        objToken,
		NodeToken:       strings.TrimSpace(GetString(node, "node_token")),
		SpaceID:         strings.TrimSpace(GetString(node, "space_id")),
		NodeType:        strings.TrimSpace(GetString(node, "node_type")),
		OriginNodeToken: strings.TrimSpace(GetString(node, "origin_node_token")),
	}, nil
}
