// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

// wikiNodeGetURLObjTypes maps a Lark URL path prefix (slash-bounded) to the
// obj_type the wiki get_node API expects when the token is an obj_token.
// /wiki/ is handled separately because node_tokens take no obj_type.
//
// INVARIANT: the prefixes must be mutually exclusive (no prefix may be a
// prefix of another). tokenAndObjTypeFromWikiURL ranges this map, and Go map
// iteration order is randomized — overlapping prefixes would make the match
// non-deterministic. The trailing slash keeps them disjoint today (e.g.
// "/docx/" does not start with "/doc/"); preserve that when adding entries.
var wikiNodeGetURLObjTypes = map[string]string{
	"/docx/":     "docx",
	"/doc/":      "doc",
	"/sheets/":   "sheet",
	"/base/":     "bitable",
	"/mindnote/": "mindnote",
	"/slides/":   "slides",
	"/file/":     "file",
}

// wikiNodeGetObjTypeEnum is the union of obj_types accepted by the upstream
// API. It is a superset of the create / move enums so this shortcut can look
// up legacy `doc` nodes too.
var wikiNodeGetObjTypeEnum = []string{
	"doc", "docx", "sheet", "bitable", "mindnote", "slides", "file",
}

// WikiNodeGet wraps wiki.spaces.get_node so callers can resolve a node by
// node_token, obj_token, or a Lark URL without hand-rolling a
// `wiki spaces get_node --params ...` invocation. The shortcut prints a
// formatted view of the node (title / obj_type / obj_token / parent /
// creator / updated_at) and is intended as the "what am I about to
// touch?" step before +move / +node-copy / +delete-space.
var WikiNodeGet = common.Shortcut{
	Service:     "wiki",
	Command:     "+node-get",
	Description: "Get wiki node details by node_token, obj_token, or Lark URL",
	Risk:        "read",
	Scopes:      []string{"wiki:node:retrieve"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		// --node-token is the canonical flag, matching sibling wiki commands
		// (+node-delete / +node-copy / +move). --token is the original name
		// and is kept as a hidden deprecated alias for backward compatibility;
		// MarkDeprecated (registered in PostMount) prints a stderr warning
		// when --token is used.
		{Name: "node-token", Desc: "wiki node_token, obj_token, or a Lark URL embedding one of them"},
		{Name: "token", Desc: "DEPRECATED: use --node-token", Hidden: true},
		{Name: "obj-type", Desc: "obj_type when --node-token is an obj_token; auto-inferred from URL path when omitted", Enum: wikiNodeGetObjTypeEnum},
		{Name: "space-id", Desc: "optional: assert the resolved node lives in this space"},
	},
	Tips: []string{
		"--node-token accepts a raw wiki node_token, obj_token, or a Lark URL like https://feishu.cn/wiki/<token> or https://feishu.cn/docx/<token>.",
		"For raw obj_tokens, pass --obj-type so the API knows how to resolve them; URL inputs infer it from the path.",
		"Pair with +move / +node-copy / +delete-space to confirm space_id, obj_type, and parent before mutating.",
		"--token is the deprecated original name and still works for backward compatibility; new scripts should use --node-token.",
	},
	PostMount: func(cmd *cobra.Command) {
		// cobra's MarkDeprecated prints "Flag --token has been deprecated, use --node-token instead"
		// to stderr on use, and hides the flag from --help (matching the Hidden: true marker above).
		_ = cmd.Flags().MarkDeprecated("token", "use --node-token instead")
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := readWikiNodeGetSpec(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec, err := readWikiNodeGetSpec(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return buildWikiNodeGetDryRun(spec)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec, err := readWikiNodeGetSpec(runtime)
		if err != nil {
			return err
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Fetching wiki node %s...\n", common.MaskToken(spec.Token))

		data, err := runtime.CallAPITyped("GET", "/open-apis/wiki/v2/spaces/get_node", spec.RequestParams(), nil)
		if err != nil {
			return err
		}
		raw := common.GetMap(data, "node")
		node, err := parseWikiNodeRecord(raw)
		if err != nil {
			return err
		}

		if spec.SpaceID != "" && node.SpaceID != "" && spec.SpaceID != node.SpaceID {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--space-id %q does not match the resolved node space %q (node_token=%s)",
				spec.SpaceID, node.SpaceID, node.NodeToken,
			).WithParam("--space-id")
		}
		if spec.SpaceID != "" && node.SpaceID == "" {
			// The cross-check was requested but get_node returned no space_id,
			// so it silently passed. Surface that the assertion was a no-op
			// rather than letting the caller assume it was verified.
			fmt.Fprintf(runtime.IO().ErrOut,
				"Warning: --space-id %q could not be verified; the resolved node carries no space_id.\n",
				spec.SpaceID)
		}

		out := wikiNodeGetOutput(node, raw)
		runtime.OutFormat(out, nil, func(w io.Writer) {
			renderWikiNodeGetPretty(w, out)
		})
		return nil
	},
}

// wikiNodeGetSpec is the normalized input for the shortcut.
type wikiNodeGetSpec struct {
	// Token is the resolved token (after URL extraction) to send to the API.
	Token string
	// ObjType is the resolved obj_type. Empty for node_tokens (the API does
	// not need obj_type for `wik`-prefixed tokens).
	ObjType string
	// SpaceID is an optional cross-check; when set, the response space_id must match.
	SpaceID string
	// SourceKind records how Token was derived for the dry-run description:
	// "url-wiki", "url-obj", "raw-node", "raw-obj".
	SourceKind string
}

// RequestParams returns the query params for GET /wiki/v2/spaces/get_node.
func (spec wikiNodeGetSpec) RequestParams() map[string]interface{} {
	params := map[string]interface{}{"token": spec.Token}
	if spec.ObjType != "" {
		params["obj_type"] = spec.ObjType
	}
	return params
}

func readWikiNodeGetSpec(runtime *common.RuntimeContext) (wikiNodeGetSpec, error) {
	rawToken, err := resolveWikiNodeGetRawToken(
		runtime.Str("node-token"),
		runtime.Str("token"),
	)
	if err != nil {
		return wikiNodeGetSpec{}, err
	}
	return parseWikiNodeGetSpec(
		rawToken,
		runtime.Str("obj-type"),
		runtime.Str("space-id"),
	)
}

// resolveWikiNodeGetRawToken picks between the canonical --node-token and the
// deprecated --token alias. Both empty is fine (parseWikiNodeGetSpec will
// surface the required-flag error). Both set with different values is rejected
// upfront so callers fix the obvious bug rather than silently picking one.
func resolveWikiNodeGetRawToken(nodeToken, legacyToken string) (string, error) {
	canonical := strings.TrimSpace(nodeToken)
	legacy := strings.TrimSpace(legacyToken)
	switch {
	case canonical != "" && legacy != "" && canonical != legacy:
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--node-token and --token are both set with different values; pass --node-token only (--token is deprecated)").WithParam("--token")
	case canonical != "":
		return nodeToken, nil
	default:
		return legacyToken, nil
	}
}

// parseWikiNodeGetSpec normalizes the raw flag values: extracts a token from a
// URL when needed, picks the obj_type (URL path > explicit flag > none for
// node_tokens), and validates the token shape.
func parseWikiNodeGetSpec(rawToken, rawObjType, rawSpaceID string) (wikiNodeGetSpec, error) {
	tokenInput := strings.TrimSpace(rawToken)
	if tokenInput == "" {
		return wikiNodeGetSpec{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "--node-token is required").WithParam("--node-token")
	}

	spec := wikiNodeGetSpec{
		ObjType: strings.ToLower(strings.TrimSpace(rawObjType)),
		SpaceID: strings.TrimSpace(rawSpaceID),
	}

	if strings.Contains(tokenInput, "://") {
		u, err := url.Parse(tokenInput)
		if err != nil || u.Path == "" {
			return wikiNodeGetSpec{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "--node-token URL is malformed: %q", tokenInput).WithParam("--node-token")
		}
		token, urlObjType, ok := tokenAndObjTypeFromWikiURL(u.Path)
		if !ok {
			return wikiNodeGetSpec{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"unsupported --node-token URL path %q: expected /wiki/, /docx/, /doc/, /sheets/, /base/, /mindnote/, /slides/, or /file/ followed by a token",
				u.Path,
			).WithParam("--node-token")
		}
		spec.Token = token
		if urlObjType == "" {
			spec.SourceKind = "url-wiki"
		} else {
			spec.SourceKind = "url-obj"
		}
		switch {
		case spec.ObjType == "" && urlObjType != "":
			spec.ObjType = urlObjType
		case spec.ObjType != "" && urlObjType != "" && spec.ObjType != urlObjType:
			return wikiNodeGetSpec{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--obj-type %q does not match the obj_type %q implied by the URL path; pass only one",
				spec.ObjType, urlObjType,
			).WithParam("--obj-type")
		}
	} else if strings.ContainsAny(tokenInput, "/?#") {
		return wikiNodeGetSpec{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--node-token must be a raw token or a full URL; partial paths are not accepted: %q",
			tokenInput,
		).WithParam("--node-token")
	} else {
		spec.Token = tokenInput
		if spec.ObjType == "" {
			spec.SourceKind = "raw-node"
		} else {
			spec.SourceKind = "raw-obj"
		}
	}

	if err := validateOptionalResourceName(spec.Token, "--node-token"); err != nil {
		return wikiNodeGetSpec{}, err
	}
	if err := validateOptionalResourceName(spec.SpaceID, "--space-id"); err != nil {
		return wikiNodeGetSpec{}, err
	}
	return spec, nil
}

// tokenAndObjTypeFromWikiURL extracts the token and inferred obj_type from a
// Lark URL path. The wiki path returns an empty obj_type because node_tokens
// don't need one.
func tokenAndObjTypeFromWikiURL(path string) (token, objType string, ok bool) {
	if t, found := wikiPathSegmentAfter(path, "/wiki/"); found {
		return t, "", true
	}
	for prefix, ot := range wikiNodeGetURLObjTypes {
		if t, found := wikiPathSegmentAfter(path, prefix); found {
			return t, ot, true
		}
	}
	return "", "", false
}

// wikiPathSegmentAfter returns the first path segment after prefix, or ("",
// false) when path doesn't start with prefix or the segment is empty.
func wikiPathSegmentAfter(path, prefix string) (string, bool) {
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	rest := path[len(prefix):]
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		rest = rest[:i]
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", false
	}
	return rest, true
}

func buildWikiNodeGetDryRun(spec wikiNodeGetSpec) *common.DryRunAPI {
	dry := common.NewDryRunAPI()
	switch spec.SourceKind {
	case "url-wiki":
		dry.Desc("Resolve wiki node from /wiki/ URL")
	case "url-obj":
		dry.Desc("Resolve wiki node from Lark document URL (obj_type inferred from path)")
	case "raw-node":
		dry.Desc("Look up wiki node by node_token")
	case "raw-obj":
		dry.Desc("Look up wiki node by obj_token")
	}
	return dry.GET("/open-apis/wiki/v2/spaces/get_node").Params(spec.RequestParams())
}

// wikiNodeGetOutput shapes the structured output. It carries the formatted
// values (title/obj_type/obj_token/parent_node_token/creator/updated_at)
// the user asked for, plus enough raw fields (node_type, has_child, owner,
// timestamps) that callers can pipe into +move / +node-copy without rerunning
// get_node.
//
// No synthesized `url` is emitted: get_node returns none, and a
// BuildResourceURL fallback (www.feishu.cn/wiki/<node_token>) is a
// non-canonical link that misleads in a read/confirm command. Sibling read
// shortcuts (+node-list, +node-copy) likewise omit it; node_token/obj_token
// are the precise identifiers.
func wikiNodeGetOutput(node *wikiNodeRecord, raw map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{
		"space_id":          node.SpaceID,
		"node_token":        node.NodeToken,
		"obj_token":         node.ObjToken,
		"obj_type":          node.ObjType,
		"node_type":         node.NodeType,
		"parent_node_token": node.ParentNodeToken,
		"origin_node_token": node.OriginNodeToken,
		"title":             node.Title,
		"has_child":         node.HasChild,
	}

	creator := strings.TrimSpace(common.GetString(raw, "node_creator"))
	if creator == "" {
		creator = strings.TrimSpace(common.GetString(raw, "creator"))
	}
	out["creator"] = creator
	out["owner"] = common.GetString(raw, "owner")

	objEditRaw := common.GetString(raw, "obj_edit_time")
	out["obj_edit_time"] = objEditRaw
	out["obj_create_time"] = common.GetString(raw, "obj_create_time")
	out["node_create_time"] = common.GetString(raw, "node_create_time")
	out["updated_at"] = formatWikiTimestamp(objEditRaw)

	return out
}

// formatWikiTimestamp turns a Lark unix-seconds string (the format used by
// wiki.spaces.get_node) into a UTC RFC3339 string. UTC (not the host's local
// zone) keeps the output stable regardless of where the CLI runs. Returns ""
// when the input is empty or not numeric so the pretty renderer falls back
// to "-".
func formatWikiTimestamp(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	secs, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return ""
	}
	return time.Unix(secs, 0).UTC().Format(time.RFC3339)
}

func renderWikiNodeGetPretty(w io.Writer, out map[string]interface{}) {
	fmt.Fprintln(w, "Wiki node:")
	fmt.Fprintf(w, "  title:             %s\n", valueOrDash(out["title"]))
	fmt.Fprintf(w, "  obj_type:          %s\n", valueOrDash(out["obj_type"]))
	fmt.Fprintf(w, "  obj_token:         %s\n", valueOrDash(out["obj_token"]))
	fmt.Fprintf(w, "  node_token:        %s\n", valueOrDash(out["node_token"]))
	fmt.Fprintf(w, "  space_id:          %s\n", valueOrDash(out["space_id"]))
	fmt.Fprintf(w, "  parent_node_token: %s\n", valueOrDash(out["parent_node_token"]))
	fmt.Fprintf(w, "  node_type:         %s\n", valueOrDash(out["node_type"]))
	if origin, _ := out["origin_node_token"].(string); origin != "" {
		fmt.Fprintf(w, "  origin_node_token: %s\n", origin)
	}
	hasChild, _ := out["has_child"].(bool)
	fmt.Fprintf(w, "  has_child:         %t\n", hasChild)
	fmt.Fprintf(w, "  creator:           %s\n", valueOrDash(out["creator"]))
	if owner, _ := out["owner"].(string); owner != "" {
		fmt.Fprintf(w, "  owner:             %s\n", owner)
	}
	fmt.Fprintf(w, "  updated_at:        %s\n", valueOrDash(out["updated_at"]))
}
