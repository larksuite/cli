// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	wikiNodeListDefaultPageSize = 50
	wikiNodeListMaxPageSize     = 50
)

// WikiNodeList lists child nodes in a wiki space or under a parent node.
var WikiNodeList = common.Shortcut{
	Service:     "wiki",
	Command:     "+node-list",
	Description: "List wiki nodes in a space or under a parent node",
	Risk:        "read",
	// Same exact-match-scope reasoning as +space-list: declare the
	// narrowest scope the upstream API accepts so we don't false-reject
	// tokens that only carry wiki:node:retrieve.
	Scopes:    []string{"wiki:node:retrieve"},
	AuthTypes: []string{"user", "bot"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "space-id", Desc: "wiki space ID; use my_library for the personal document library, or +space-list to discover other space IDs", Required: true},
		{Name: "parent-node-token", Desc: "parent node token; if omitted, lists the root-level nodes of the space"},
		{Name: "page-size", Type: "int", Default: strconv.Itoa(wikiNodeListDefaultPageSize), Desc: fmt.Sprintf("page size, 1-%d", wikiNodeListMaxPageSize)},
		{Name: "page-token", Desc: "page token; implies single-page fetch (no auto-pagination)"},
		{Name: "page-all", Type: "bool", Desc: "automatically paginate through all pages (capped by --page-limit)"},
		{Name: "page-limit", Type: "int", Default: "10", Desc: "max pages to fetch with --page-all (default 10, 0 = unlimited)"},
	},
	Tips: []string{
		"Default fetches a single page; pass --page-all to walk every page (large knowledge bases can be huge — keep an eye on --page-limit).",
		"Use --parent-node-token to drill into a sub-directory.",
		"Run +space-list first to discover your space IDs, including the personal document library.",
		"--space-id my_library is a per-user alias and is only valid with --as user.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := readWikiNodeListSpec(runtime); err != nil {
			return err
		}
		return validateWikiListPagination(runtime, wikiNodeListMaxPageSize)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec, err := readWikiNodeListSpec(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		params := map[string]interface{}{"page_size": runtime.Int("page-size")}
		if spec.ParentNodeToken != "" {
			params["parent_node_token"] = spec.ParentNodeToken
		}
		if pt := strings.TrimSpace(runtime.Str("page-token")); pt != "" {
			params["page_token"] = pt
		}
		d := common.NewDryRunAPI()
		if wikiListShouldAutoPaginate(runtime) {
			d.Desc("Auto-paginates through all pages (capped by --page-limit when > 0)")
		}
		// When the caller passes my_library, +node-list must first resolve it
		// to the real per-user space_id before listing nodes, mirroring the
		// two-step orchestration used by +node-create.
		if spec.SpaceID == wikiMyLibrarySpaceID {
			return d.
				Desc("2-step orchestration: resolve my_library -> list nodes").
				GET("/open-apis/wiki/v2/spaces/my_library").
				Desc("[1] Resolve my_library space ID").
				GET(fmt.Sprintf("/open-apis/wiki/v2/spaces/%s/nodes", "<resolved_space_id>")).
				Desc("[2] List nodes").
				Params(params).
				Set("space_id", "<resolved_space_id>")
		}
		return d.
			GET(fmt.Sprintf("/open-apis/wiki/v2/spaces/%s/nodes", validate.EncodePathSegment(spec.SpaceID))).
			Params(params).
			Set("space_id", spec.SpaceID)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		warnIfConflictingPagingFlags(runtime)
		spec, err := readWikiNodeListSpec(runtime)
		if err != nil {
			return err
		}
		spaceID := spec.SpaceID

		// Resolve the my_library alias to the per-user real space_id before
		// listing, so the subsequent request hits a concrete space endpoint.
		if spaceID == wikiMyLibrarySpaceID {
			resolved, err := resolveMyLibrarySpaceID(runtime)
			if err != nil {
				return err
			}
			fmt.Fprintf(runtime.IO().ErrOut, "Resolved my_library to space %s\n", common.MaskToken(resolved))
			spaceID = resolved
		}

		nodes, hasMore, nextToken, err := fetchWikiNodes(runtime, spaceID, spec.ParentNodeToken)
		if err != nil {
			return err
		}
		fmt.Fprintf(runtime.IO().ErrOut, "Found %d node(s)\n", len(nodes))
		outData := map[string]interface{}{
			"nodes":      nodes,
			"has_more":   hasMore,
			"page_token": nextToken,
		}
		runtime.OutFormat(outData, &output.Meta{Count: len(nodes)}, func(w io.Writer) {
			renderWikiNodesPretty(w, nodes, hasMore, nextToken)
		})
		return nil
	},
}

type wikiNodeListSpec struct {
	SpaceID         string
	ParentNodeToken string
}

func readWikiNodeListSpec(runtime *common.RuntimeContext) (wikiNodeListSpec, error) {
	spaceID := strings.TrimSpace(runtime.Str("space-id"))
	// my_library is a per-user personal-library alias; it has no meaning
	// for a tenant_access_token (--as bot), so reject early with a clear
	// hint instead of deferring to API-time errors. Matches the contract
	// used by +node-create and +move.
	if runtime.As().IsBot() && spaceID == wikiMyLibrarySpaceID {
		return wikiNodeListSpec{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "bot identity does not support --space-id my_library; use an explicit numeric --space-id").WithParam("--space-id")
	}
	if err := validateWikiNodeListSpaceID(spaceID); err != nil {
		return wikiNodeListSpec{}, err
	}

	parentNodeToken, err := normalizeWikiNodeListParentToken(strings.TrimSpace(runtime.Str("parent-node-token")))
	if err != nil {
		return wikiNodeListSpec{}, err
	}
	return wikiNodeListSpec{SpaceID: spaceID, ParentNodeToken: parentNodeToken}, nil
}

func validateWikiNodeListSpaceID(spaceID string) error {
	if spaceID == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--space-id is required").WithParam("--space-id")
	}
	if spaceID == wikiMyLibrarySpaceID {
		return nil
	}
	if strings.Contains(spaceID, "://") || strings.ContainsAny(spaceID, "/?#") {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--space-id must be a numeric wiki space_id, not a URL or path",
		).WithParam("--space-id").WithHint("Run `lark-cli wiki +space-list --as user` to discover space IDs.")
	}
	if !isDecimalWikiSpaceID(spaceID) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--space-id must be a numeric wiki space_id; do not pass a wiki node token, document token, or title",
		).WithParam("--space-id").WithHint("Run `lark-cli wiki +space-list --as user` to list accessible wiki spaces, then pass the numeric `space_id`.")
	}
	if err := validateOptionalResourceName(spaceID, "--space-id"); err != nil {
		return err
	}
	return nil
}

func isDecimalWikiSpaceID(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func normalizeWikiNodeListParentToken(parentNodeToken string) (string, error) {
	if parentNodeToken == "" {
		return "", nil
	}
	if strings.Contains(parentNodeToken, "://") {
		ref, ok := common.ParseResourceURL(parentNodeToken)
		if !ok {
			return "", errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--parent-node-token URL is unsupported",
			).WithParam("--parent-node-token").WithHint("Pass a raw wiki node token from `wiki +node-get` or `wiki +node-list`.")
		}
		if ref.Type != "wiki" {
			return "", errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--parent-node-token must identify a wiki node; got a %s URL",
				ref.Type,
			).WithParam("--parent-node-token").WithHint("Resolve the document URL with `lark-cli wiki +node-get --node-token <url>` and use its `node_token`.")
		}
		parentNodeToken = ref.Token
	}
	if strings.ContainsAny(parentNodeToken, "/?#") {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--parent-node-token must be a raw wiki node token, not a partial URL or path",
		).WithParam("--parent-node-token")
	}
	if err := validateOptionalResourceName(parentNodeToken, "--parent-node-token"); err != nil {
		return "", err
	}
	return parentNodeToken, nil
}

func fetchWikiNodes(runtime *common.RuntimeContext, spaceID, parentNodeToken string) ([]map[string]interface{}, bool, string, error) {
	pageSize := runtime.Int("page-size")
	startToken := strings.TrimSpace(runtime.Str("page-token"))
	auto := wikiListShouldAutoPaginate(runtime)
	pageLimit := runtime.Int("page-limit")

	apiPath := fmt.Sprintf("/open-apis/wiki/v2/spaces/%s/nodes", validate.EncodePathSegment(spaceID))

	// Non-nil empty slice keeps json output stable as `[]` instead of `null`.
	var (
		nodes         = make([]map[string]interface{}, 0)
		pageToken     = startToken
		lastHasMore   bool
		lastPageToken string
	)
	for page := 0; ; page++ {
		params := map[string]interface{}{"page_size": pageSize}
		if parentNodeToken != "" {
			params["parent_node_token"] = parentNodeToken
		}
		if pageToken != "" {
			params["page_token"] = pageToken
		}
		data, err := runtime.CallAPITyped("GET", apiPath, params, nil)
		if err != nil {
			return nil, false, "", wikiNodeListProblem(err, runtime)
		}
		items, _ := data["items"].([]interface{})
		for _, item := range items {
			if m, ok := item.(map[string]interface{}); ok {
				nodes = append(nodes, wikiNodeListItem(m))
			}
		}
		lastHasMore, _ = data["has_more"].(bool)
		lastPageToken, _ = data["page_token"].(string)
		if !auto {
			break
		}
		if !lastHasMore || lastPageToken == "" {
			break
		}
		if pageLimit > 0 && page+1 >= pageLimit {
			break
		}
		pageToken = lastPageToken
	}
	return nodes, lastHasMore, lastPageToken, nil
}

func wikiNodeListProblem(err error, runtime *common.RuntimeContext) error {
	p, ok := errs.ProblemOf(err)
	if !ok {
		return err
	}
	switch p.Code {
	case 131002:
		msg := strings.ToLower(p.Message)
		switch {
		case strings.Contains(msg, "page_token"):
			appendWikiProblemHint(err, "The page token is invalid or stale. Use only the `page_token` returned by the immediately preceding `wiki +node-list` response, or omit --page-token and start over.")
		case strings.Contains(msg, "space_id"):
			appendWikiProblemHint(err, "The --space-id value must be the numeric wiki space_id from `wiki +space-list`; do not pass a wiki URL, node token, document token, or title.")
		default:
			appendWikiProblemHint(err, "Check the wiki +node-list flags. Fix the parameter before retrying; this is not a transient error.")
		}
	case 131005:
		appendWikiProblemHint(err, "The target wiki space or parent node was not found. Re-discover the space with `wiki +space-list` and the parent with `wiki +node-list`/`wiki +node-get`; do not retry the same stale token.")
	case 131006:
		if runtime != nil && runtime.As().IsBot() {
			appendWikiProblemHint(err, "The bot/app identity cannot read this wiki space or node. Grant the app the required wiki scope and ensure the app or bot has access to the target knowledge space.")
		} else {
			appendWikiProblemHint(err, "The current user cannot read this wiki space or node. Switch to a user with access or ask the space owner to grant read permission.")
		}
	case 99991400:
		appendWikiProblemHint(err, "Rate limited by the wiki API. Stop immediate retries and retry later with exponential backoff or a smaller --page-limit.")
	}
	return err
}

func wikiNodeListItem(m map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"space_id":          common.GetString(m, "space_id"),
		"node_token":        common.GetString(m, "node_token"),
		"obj_token":         common.GetString(m, "obj_token"),
		"obj_type":          common.GetString(m, "obj_type"),
		"parent_node_token": common.GetString(m, "parent_node_token"),
		"node_type":         common.GetString(m, "node_type"),
		"title":             common.GetString(m, "title"),
		"has_child":         common.GetBool(m, "has_child"),
	}
}

func renderWikiNodesPretty(w io.Writer, nodes []map[string]interface{}, hasMore bool, pageToken string) {
	if len(nodes) == 0 {
		if hasMore && pageToken != "" {
			fmt.Fprintln(w, "Current page is empty but the server reports more pages.")
			fmt.Fprintln(w, "Pass --page-all to walk every page, or --page-token to resume from the cursor below:")
			fmt.Fprintf(w, "  next page_token: %s\n", pageToken)
			return
		}
		fmt.Fprintln(w, "No wiki nodes found.")
		return
	}
	for i, n := range nodes {
		fmt.Fprintf(w, "[%d] %s\n", i+1, valueOrDash(n["title"]))
		fmt.Fprintf(w, "    node_token: %s\n", valueOrDash(n["node_token"]))
		fmt.Fprintf(w, "    obj_type:   %s\n", valueOrDash(n["obj_type"]))
		fmt.Fprintf(w, "    obj_token:  %s\n", valueOrDash(n["obj_token"]))
		hasChild, _ := n["has_child"].(bool)
		fmt.Fprintf(w, "    has_child:  %t\n", hasChild)
		if parent, _ := n["parent_node_token"].(string); parent != "" {
			fmt.Fprintf(w, "    parent:     %s\n", parent)
		}
		fmt.Fprintln(w)
	}
	if hasMore && pageToken != "" {
		fmt.Fprintf(w, "Next page token: %s\n", pageToken)
	}
}
