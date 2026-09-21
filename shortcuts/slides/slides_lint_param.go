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
// The server can be configured to require a page to have been linted once before
// it honours the flag, and a refusal counts as that lint. So the flag can need a
// prior submission of the same content to take effect — it never needs a
// successful one. What that changes is the wording of the refusal, not whether
// the escape hatch works: see noLintFirstLintHint.
//
// Sent explicitly in both directions rather than omitted when on: the parameter
// is newer than internal/registry/catalog/services/slides.json, so the
// server-side default is not something this CLI can read anywhere, and a request
// that states the value means the same thing before and after that default ever
// changes.
//
// The flag governs lint_xml only. enable_lint, described below, is sent as true
// on every call regardless — see there for what that costs.
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

// enableLintKey is a second lint parameter the backend asked for. It sits
// alongside lint_xml rather than replacing it — both are sent, and lint_xml
// keeps its existing meaning and value.
//
// It is always true and deliberately not configurable: no flag turns it off,
// and --no-lint does not reach it. That is the requested behaviour, and the
// price is worth stating plainly. --no-lint exists so a page the lint is wrong
// about can still ship; it works by sending lint_xml=false. If the backend ever
// honours enable_lint on its own, a caller who passed --no-lint still arrives
// carrying enable_lint=true, the page is linted anyway, and the escape hatch is
// gone. Anyone narrowing that gap later should make enable_lint follow the flag
// rather than add a second one.
//
// Where it travels differs by direction, because the two directions are shaped
// differently: the writers already send a JSON body, so it goes in the body next
// to lint_xml. +xml-get is a GET with no body, so there it goes in the query —
// and the gateway-drop hazard documented above applies in full. Neither GET the
// command issues declares enable_lint in its api meta today (see
// internal/registry/catalog/services/slides.json, which mirrors it), so until
// the server declares and publishes the parameter the value is expected to be
// dropped silently: the request succeeds and the field arrives unset. That is
// visible only from the backend; nothing on this side reports it.
const enableLintKey = "enable_lint"

// noLintFlag is the shared flag definition, so the three commands cannot drift
// apart on the name or the wording.
func noLintFlag() common.Flag {
	return common.Flag{
		Name: noLintFlagName,
		Type: "bool",
		Desc: "submit the XML without the server-side lint_xml check; by default the server lints every page and rejects the write when it fails. Note enable_lint is always sent as true and this flag does not turn it off, so a backend that acts on enable_lint alone still lints the page",
	}
}

// withLintParams stamps both lint parameters onto a request body and returns it,
// so the body builders stay one-liners and dry-run and execute cannot disagree
// about the values. Being the single place either key is written is what keeps
// the five writers from drifting apart. A nil map is allocated rather than
// rejected, so a caller with nothing else to send still gets a well-formed body.
func withLintParams(body map[string]interface{}, runtime *common.RuntimeContext) map[string]interface{} {
	if body == nil {
		body = map[string]interface{}{}
	}
	body[lintXMLBodyKey] = !runtime.Bool(noLintFlagName)
	body[enableLintKey] = true
	return body
}
