// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"encoding/json"

	"github.com/larksuite/cli/internal/event/model"
)

// payloadHeaderClaims holds the payload's header block with its values left
// undecoded. Decoding them one at a time is what keeps a single badly-typed
// field from discarding the claims beside it: encoding/json populates the
// fields it can and still reports an error, so a whole-header decode that
// bails on that error would throw away comparisons it had already resolved.
//
// It exists only for validation at the consume boundary — canonical metadata
// itself always comes from the ingress.
type payloadHeaderClaims struct {
	Header map[string]json.RawMessage `json:"header"`
}

// factComparison pairs one canonical fact with the payload-header field that
// can claim it. The comparisons live in a table, not an if-chain: adding a
// fact to model.Event means adding a row here (a reflection gate enforces it).
//
// name is both the row's identity and the header field it reads: the envelope
// spells these facts the same way the rows are named.
type factComparison struct {
	name      string
	canonical func(ev *model.Event) string
}

var canonicalFactComparisons = []factComparison{
	{name: "event_id", canonical: func(ev *model.Event) string { return ev.EventID }},
	{name: "event_type", canonical: func(ev *model.Event) string { return ev.EventType }},
	{name: "create_time", canonical: func(ev *model.Event) string { return ev.SourceTime }},
	{name: "app_id", canonical: func(ev *model.Event) string { return ev.AppID }},
	{name: "tenant_key", canonical: func(ev *model.Event) string { return ev.TenantKey }},
}

// checkCanonicalConflict returns the name of the first canonical fact the
// payload header contradicts, or "" when the event may be delivered.
//
// Arbitration is deliberately one-sided: a silent header claims nothing, but
// once the header asserts a fact it must match the canonical value —
// including when the canonical side is empty. An asserted header fact facing
// an empty canonical value means the fact was lost between the ingress and
// this consumer; that is a delivery defect, not "nothing to compare".
//
// The envelope declares every one of these facts as a string. A header that
// asserts one with a different JSON type states something this arbiter cannot
// compare, so it counts as a conflict rather than as silence — otherwise a
// single type flip would be enough to disable arbitration for the rest of the
// header. JSON null is the exception: it is how the envelope spells "absent",
// and it asserts nothing.
//
// A payload that is not a JSON object, or whose header is not one, claims no
// fact at all and is not re-classified here — malformed handling belongs to
// the processing layer.
func checkCanonicalConflict(ev *model.Event) string {
	var claims payloadHeaderClaims
	if err := json.Unmarshal(ev.Payload, &claims); err != nil {
		return ""
	}
	for _, c := range canonicalFactComparisons {
		raw, asserted := claims.Header[c.name]
		if !asserted {
			continue
		}
		var claimed string
		if err := json.Unmarshal(raw, &claimed); err != nil {
			return c.name
		}
		if claimed != "" && claimed != c.canonical(ev) {
			return c.name
		}
	}
	return ""
}
