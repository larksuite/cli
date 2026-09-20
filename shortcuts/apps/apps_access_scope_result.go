// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"fmt"
	"io"
)

// accessScopeResultKey is the field the create endpoint uses to report what it
// did with access_scope. It describes that one call, not the app, so it sits
// next to the app object rather than inside it.
const accessScopeResultKey = "access_scope_result"

// Reason codes the server sends when the scope was not applied. Anything else
// is rendered verbatim, so the server can add codes without breaking clients.
const (
	accessScopeReasonPolicyDenied    = "policy_denied"
	accessScopeReasonDownstreamError = "downstream_error"
	accessScopeReasonUnsupportedType = "unsupported_app_type"
)

// accessScopeReasonText renders a reason and the next step the way a CLI user
// reads them. The server also returns a Message, but that text is localized for
// one brand while this CLI serves both, so the terminal copy is generated here
// and the server's own wording stays in the JSON envelope for callers that want
// it.
//
// The next step is never "run this command again": the scope is applied while
// the app is created, so a retry means creating a second app, and apps has no
// delete. Whatever is still wanted is done on the app's settings page.
func accessScopeReasonText(code string) (reason, next string) {
	switch code {
	case accessScopeReasonPolicyDenied:
		return "the tenant's sharing or classification policy refused it",
			"adjust it on the app's sharing settings page"
	case accessScopeReasonDownstreamError:
		return "the platform could not apply it", "set it on the app's sharing settings page"
	case accessScopeReasonUnsupportedType:
		return "the app type that was created does not support it", ""
	case "":
		return "the server did not say why", "set it on the app's sharing settings page"
	default:
		return "reason: " + code, "set it on the app's sharing settings page"
	}
}

// warnAccessScopeResult reports a requested access scope that the server did not
// apply. Creation succeeds either way — the app exists and is usable — so this
// is a warning on stderr, never an error: failing here would tell a caller the
// app was not created and invite a retry, and apps has no delete.
//
// requested is what this invocation asked for. It is the gate on purpose: the
// result only ever concerns a scope this call requested, so a run that passed no
// --access-scope ignores the field entirely and stays quiet.
func warnAccessScopeResult(w io.Writer, requested string, data map[string]interface{}) {
	if requested == "" || data == nil {
		return
	}
	raw, ok := data[accessScopeResultKey].(map[string]interface{})
	if !ok {
		return
	}
	if applied, _ := raw["applied"].(bool); applied {
		return
	}
	code, _ := raw["reason_code"].(string)
	reason, next := accessScopeReasonText(code)
	if next != "" {
		next = "; " + next
	}
	fmt.Fprintf(w,
		"warning: --access-scope %s was not applied: %s; the app is created and usable, and its visibility is unchanged%s. Do not create the app again to retry: the scope is only applied at creation and apps cannot be deleted\n",
		requested, reason, next)
}

// accessScopeResultOf lifts the raw result out of a create response so a caller
// that builds its own envelope (the bare-HTML publish) can pass it through
// unchanged. Returns nil when the server did not report one.
func accessScopeResultOf(data map[string]interface{}) map[string]interface{} {
	if data == nil {
		return nil
	}
	raw, _ := data[accessScopeResultKey].(map[string]interface{})
	return raw
}
