// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	wikiNodeCopyMaxRetries     = 2
	wikiNodeCopyRetryBaseDelay = 250 * time.Millisecond
)

// WikiNodeCopy copies a wiki node into a target space or under a target parent node.
var WikiNodeCopy = common.Shortcut{
	Service:     "wiki",
	Command:     "+node-copy",
	Description: "Copy a wiki node to a target space or parent node",
	Risk:        "high-risk-write",
	Scopes:      []string{"wiki:node:copy"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "space-id", Desc: "source wiki space ID", Required: true},
		{Name: "node-token", Desc: "source node token to copy", Required: true},
		{Name: "target-space-id", Desc: "target wiki space ID; required if --target-parent-node-token is not set"},
		{Name: "target-parent-node-token", Desc: "target parent node token; required if --target-space-id is not set"},
		{Name: "title", Desc: "new title for the copied node; leave empty to keep the original title"},
	},
	Tips: []string{
		"At least one of --target-space-id or --target-parent-node-token must be provided.",
		"Omit --title to keep the original node title in the copy.",
		"This shortcut copies the current node only; descendant nodes are not copied.",
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
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "at least one of --target-space-id or --target-parent-node-token is required").
				WithParams(
					errs.InvalidParam{Name: "--target-space-id", Reason: "provide --target-space-id or --target-parent-node-token"},
					errs.InvalidParam{Name: "--target-parent-node-token", Reason: "provide --target-space-id or --target-parent-node-token"},
				)
		}
		if targetSpaceID != "" && targetParent != "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--target-space-id and --target-parent-node-token are mutually exclusive; provide only one").
				WithParams(
					errs.InvalidParam{Name: "--target-space-id", Reason: "mutually exclusive with --target-parent-node-token"},
					errs.InvalidParam{Name: "--target-parent-node-token", Reason: "mutually exclusive with --target-space-id"},
				)
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
			POST(fmt.Sprintf("/open-apis/wiki/v2/spaces/%s/nodes/%s/copy",
				validate.EncodePathSegment(spaceID),
				validate.EncodePathSegment(nodeToken))).
			Body(buildNodeCopyBody(runtime)).
			Set("space_id", spaceID).
			Set("node_token", nodeToken)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spaceID := strings.TrimSpace(runtime.Str("space-id"))
		nodeToken := strings.TrimSpace(runtime.Str("node-token"))

		fmt.Fprintf(runtime.IO().ErrOut, "Copying wiki node %s from space %s\n",
			common.MaskToken(nodeToken), common.MaskToken(spaceID))

		apiPath := fmt.Sprintf("/open-apis/wiki/v2/spaces/%s/nodes/%s/copy",
			validate.EncodePathSegment(spaceID),
			validate.EncodePathSegment(nodeToken))
		body := buildNodeCopyBody(runtime)
		data, err := runWikiNodeCopyWithRetry(ctx, runtime.IO().ErrOut, wikiNodeCopyRetryBaseDelay, func() (map[string]interface{}, error) {
			return runtime.CallAPITyped("POST", apiPath, nil, body)
		})
		if err != nil {
			return err
		}

		node, err := parseWikiNodeRecord(common.GetMap(data, "node"))
		if err != nil {
			return err
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Copied to node %s in space %s\n",
			common.MaskToken(node.NodeToken), common.MaskToken(node.SpaceID))
		out := wikiNodeCopyOutput(node)
		if u := wikiNodeURL(runtime.Config.Brand, node); u != "" {
			out["url"] = u
		}
		runtime.OutFormat(out, nil, func(w io.Writer) {
			renderWikiNodeCopyPretty(w, out)
		})
		return nil
	},
}

func runWikiNodeCopyWithRetry(ctx context.Context, errOut io.Writer, baseDelay time.Duration, call func() (map[string]interface{}, error)) (map[string]interface{}, error) {
	var lastErr error
	for attempt := 0; attempt <= wikiNodeCopyMaxRetries; attempt++ {
		if attempt > 0 {
			delay := baseDelay << uint(attempt-1)
			fmt.Fprintf(errOut, "Wiki node copy encountered lock contention, retrying (attempt %d/%d) in %v...\n", attempt, wikiNodeCopyMaxRetries, delay)
			select {
			case <-ctx.Done():
				return nil, wikiNodeCopyBackoffContextError(ctx.Err())
			case <-time.After(delay):
			}
		}

		data, err := call()
		if err == nil {
			return data, nil
		}
		lastErr = err
		if !isWikiNodeLockContention(err) {
			return nil, err
		}
	}
	return nil, wrapWikiNodeCopyRetryError(lastErr)
}

func wikiNodeCopyBackoffContextError(err error) error {
	subtype := errs.SubtypeNetworkTransport
	message := "wiki node copy retry was canceled during lock-contention backoff"
	if errors.Is(err, context.DeadlineExceeded) {
		subtype = errs.SubtypeNetworkTimeout
		message = "wiki node copy retry deadline exceeded during lock-contention backoff"
	}
	return errs.NewNetworkError(subtype, "%s", message).WithCause(err)
}

func wrapWikiNodeCopyRetryError(err error) error {
	if err == nil {
		return nil
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		return err
	}
	hint := fmt.Sprintf(
		"wiki node copy failed after %d retries due to lock contention; try again later or reduce concurrent writes under the same target parent",
		wikiNodeCopyMaxRetries,
	)
	if existing := strings.TrimSpace(problem.Hint); existing != "" {
		hint = existing + "\n" + hint
	}
	problem.Hint = hint
	return err
}

func renderWikiNodeCopyPretty(w io.Writer, out map[string]interface{}) {
	fmt.Fprintf(w, "Copied node:\n")
	fmt.Fprintf(w, "  title:             %s\n", valueOrDash(out["title"]))
	fmt.Fprintf(w, "  node_token:        %s\n", valueOrDash(out["node_token"]))
	fmt.Fprintf(w, "  space_id:          %s\n", valueOrDash(out["space_id"]))
	fmt.Fprintf(w, "  obj_type:          %s\n", valueOrDash(out["obj_type"]))
	fmt.Fprintf(w, "  obj_token:         %s\n", valueOrDash(out["obj_token"]))
	if parent, _ := out["parent_node_token"].(string); parent != "" {
		fmt.Fprintf(w, "  parent_node_token: %s\n", parent)
	}
	if url, _ := out["url"].(string); url != "" {
		fmt.Fprintf(w, "  url:               %s\n", url)
	}
}

func buildNodeCopyBody(runtime *common.RuntimeContext) map[string]interface{} {
	// Validate has already rejected the case where both --target-space-id and
	// --target-parent-node-token are set (mutually exclusive). It is safe to
	// inline both flags here; do not loosen that check without revisiting this
	// body builder, or the upstream API will see an ambiguous request shape.
	body := map[string]interface{}{}
	if v := strings.TrimSpace(runtime.Str("target-space-id")); v != "" {
		body["target_space_id"] = v
	}
	if v := strings.TrimSpace(runtime.Str("target-parent-node-token")); v != "" {
		body["target_parent_token"] = v
	}
	if v := strings.TrimSpace(runtime.Str("title")); v != "" {
		body["title"] = v
	}
	return body
}

func wikiNodeCopyOutput(node *wikiNodeRecord) map[string]interface{} {
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
