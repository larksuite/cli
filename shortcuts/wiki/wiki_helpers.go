// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
)

// wikiNodeURL returns the user-facing link for a wiki node. The create/copy
// OpenAPI responses carry a real `url` (undocumented in the server-docs schema
// but present in practice); prefer it so the CLI surfaces the canonical link.
// Fall back to BuildResourceURL synthesis only when the response omits it.
//
// Shared by +node-create and +node-copy, hence kept here rather than in either
// command's file.
func wikiNodeURL(brand core.LarkBrand, node *wikiNodeRecord) string {
	if node == nil {
		return ""
	}
	if u := strings.TrimSpace(node.URL); u != "" {
		return u
	}
	return common.BuildResourceURL(brand, "wiki", node.NodeToken)
}

func appendWikiProblemHint(err error, hint string) error {
	if strings.TrimSpace(hint) == "" {
		return err
	}
	if p, ok := errs.ProblemOf(err); ok {
		if strings.TrimSpace(p.Hint) != "" {
			p.Hint = p.Hint + "\n" + hint
		} else {
			p.Hint = hint
		}
	}
	return err
}

// wikiPermissionDeniedHint provides stable recovery for read-path 131006
// (node-get / node-list). The service uses one public code for both node and
// space ACL failures, while the upstream message is informational and
// normalized by the shared classifier. Keep the command hint accurate without
// branching on that unstable text.
func wikiPermissionDeniedHint() string {
	return "The current user or app/bot identity lacks access to the target wiki space or node. This is resource access, not app scope authorization. Do not retry the same request, reauthorize, or switch identity as trial and error; ask the resource owner or wiki administrator to grant read access, or use an accessible resource."
}

// wikiCopyPermissionDeniedHint recovers copy-path 131006. Official copy
// requires container edit on the source or destination parent; a space-root
// target can also require wiki space membership or administrator permission.
func wikiCopyPermissionDeniedHint() string {
	return "The current user or app/bot identity lacks Wiki container permission for this copy. This is resource access, not app scope authorization. Do not retry the same request, reauthorize, or switch identity as trial and error; ask the resource owner or wiki administrator to grant container edit permission on the relevant source or destination parent node. Copying to a space root can also require wiki space membership or administrator permission. Use an accessible resource if that access cannot be granted."
}

// wikiNodeMovePermissionDeniedHint recovers node-move 131006. Official move
// requires edit permission on the node plus container edit on the source and
// destination parents; a space-root target can also require wiki space
// membership or administrator permission.
func wikiNodeMovePermissionDeniedHint() string {
	return "The current user or app/bot identity lacks permission to move this Wiki node. Official node move requires edit permission on the node plus container edit permission on the source and destination parent nodes. Moving to a space root can also require wiki space membership or administrator permission. This is resource access, not app scope authorization. Do not retry the same request, reauthorize, or switch identity as trial and error. Use an accessible resource if that access cannot be granted."
}

// wikiDocsToWikiPermissionDeniedHint recovers docs-to-wiki write 131006.
// Official guidance can mean the source Drive document lacks manage
// permission or its parent folder lacks edit permission; the destination
// Wiki parent still needs container edit, and a space-root target can
// require wiki space membership or administrator permission.
func wikiDocsToWikiPermissionDeniedHint() string {
	return "The current user or app/bot identity lacks permission to move this Drive document into Wiki. Official docs-to-wiki 131006 can mean the source document lacks manage permission or its parent folder lacks edit permission; the destination Wiki parent needs container edit permission, and a space-root target can require wiki space membership or administrator permission. This is resource access, not app scope authorization. Do not retry the same request, reauthorize, or switch identity as trial and error."
}

// wikiTaskPermissionDeniedHint recovers task-get 131006. Official lookup
// can mean only the task creator can query status, the destination Wiki
// space or node is inaccessible, or the source Drive document lacks manage
// permission and its parent folder lacks edit permission.
func wikiTaskPermissionDeniedHint() string {
	return "The current user or app/bot identity cannot read this Wiki move task. Official task lookup 131006 can mean only the task creator can query status, the destination Wiki space or node is inaccessible, or the source Drive document lacks manage permission and its parent folder lacks edit permission. This is resource access, not app scope authorization. Do not retry the same status lookup, reauthorize, or switch identity as trial and error."
}

func annotateWikiPermissionDenied(err error) error {
	return annotateWikiPermissionDeniedWith(err, wikiPermissionDeniedHint())
}

func annotateWikiCopyPermissionDenied(err error) error {
	return annotateWikiPermissionDeniedWith(err, wikiCopyPermissionDeniedHint())
}

func annotateWikiNodeMovePermissionDenied(err error) error {
	return annotateWikiPermissionDeniedWith(err, wikiNodeMovePermissionDeniedHint())
}

func annotateWikiDocsToWikiPermissionDenied(err error) error {
	return annotateWikiPermissionDeniedWith(err, wikiDocsToWikiPermissionDeniedHint())
}

func annotateWikiTaskPermissionDenied(err error) error {
	return annotateWikiPermissionDeniedWith(err, wikiTaskPermissionDeniedHint())
}

func isWikiPermissionDenied(err error) bool {
	p, ok := errs.ProblemOf(err)
	return ok && p != nil && p.Code == 131006
}

// annotateWikiPermissionDeniedWith marks wiki 131006 as a terminal
// resource-access failure and attaches the given recovery hint. Other errors
// pass through.
func annotateWikiPermissionDeniedWith(err error, hint string) error {
	p, ok := errs.ProblemOf(err)
	if !ok || p == nil || p.Code != 131006 {
		return err
	}
	p.Retryable = false
	if strings.TrimSpace(hint) == "" || strings.Contains(p.Hint, hint) {
		return err
	}
	return appendWikiProblemHint(err, hint)
}
