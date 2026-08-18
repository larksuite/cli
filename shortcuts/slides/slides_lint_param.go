// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import "github.com/larksuite/cli/shortcuts/common"

// The server-side XML lint switch, carried by every shortcut that writes slide
// content: +create, +add-slide, +update-slide and +replace-slide.
//
// The backend lints the page the write produces and refuses it when that page
// fails. Note "produces", not "is handed": +replace-slide submits fragments, and
// the subject of the lint is the assembled page, so a fragment that is valid on
// its own is still refused when it pushes a neighbour off the canvas. The CLI
// asks for the check on every call, so a caller who never touches the flag gets
// the checked path; --no-lint is the escape hatch for the case the lint is wrong
// about a page that must still ship.
//
// Sent explicitly in both directions rather than omitted when on: the parameter
// is newer than internal/registry/meta_data.json, so the server-side default is
// not something this CLI can read anywhere, and a request that states the value
// means the same thing before and after that default ever changes.
const noLintFlagName = "no-lint"

// lintXMLBodyKey is the wire name the switch binds to, and it travels in the
// request body rather than the query string.
//
// That is not a style choice. A query parameter has to be declared in the
// gateway's own api meta before it is bound to a field, and the published
// definition of these endpoints lists only xml_presentation_id, revision_id
// and idempotency_key. Until that publish happens the gateway drops an
// undeclared parameter on the floor: the request succeeds, the field arrives
// unset, and the server reads it as "lint not requested". Verified against a
// live backend — pages that asked to be linted were written unlinted, with no
// error anywhere to say so. Body fields ride along with the JSON already being
// sent and need no separate registration.
//
// snake_case matches every other body key these endpoints take: slide,
// before_slide_id, parts, comment.
//
// Worth knowing when this is next touched: a wrong name here still fails
// silently, for the same reason it did in the query. Tests and --dry-run only
// prove what the CLI sent, not what was read.
const lintXMLBodyKey = "lint_xml"

// noLintFlag is the shared flag definition, so the three commands cannot drift
// apart on the name or the wording.
func noLintFlag() common.Flag {
	return common.Flag{
		Name: noLintFlagName,
		Type: "bool",
		Desc: "submit the XML without the server-side lint; by default the server lints every page and rejects the write when it fails",
	}
}

// withLintXML stamps the switch onto a request body and returns it, so the body
// builders stay one-liners and dry-run and execute cannot disagree about the
// value. A nil map is allocated rather than rejected, so a caller with nothing
// else to send still gets a well-formed body.
func withLintXML(body map[string]interface{}, runtime *common.RuntimeContext) map[string]interface{} {
	if body == nil {
		body = map[string]interface{}{}
	}
	body[lintXMLBodyKey] = !runtime.Bool(noLintFlagName)
	return body
}
