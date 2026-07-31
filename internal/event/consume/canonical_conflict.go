// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"encoding/json"

	"github.com/larksuite/cli/internal/event/model"
)

// payloadHeaderClaims is the subset of the payload's header block that can
// claim canonical facts. It exists only for validation at the consume
// boundary — canonical metadata itself always comes from the ingress.
type payloadHeaderClaims struct {
	Header struct {
		EventID    string `json:"event_id"`
		EventType  string `json:"event_type"`
		CreateTime string `json:"create_time"`
		AppID      string `json:"app_id"`
		TenantKey  string `json:"tenant_key"`
	} `json:"header"`
}

// factComparison pairs one canonical fact with the payload-header field that
// can claim it. The comparisons live in a table, not an if-chain: adding a
// fact to model.Event means adding a row here (a reflection gate enforces it).
type factComparison struct {
	name      string
	header    func(c *payloadHeaderClaims) string
	canonical func(ev *model.Event) string
}

var canonicalFactComparisons = []factComparison{
	{
		name:      "event_id",
		header:    func(c *payloadHeaderClaims) string { return c.Header.EventID },
		canonical: func(ev *model.Event) string { return ev.EventID },
	},
	{
		name:      "event_type",
		header:    func(c *payloadHeaderClaims) string { return c.Header.EventType },
		canonical: func(ev *model.Event) string { return ev.EventType },
	},
	{
		name:      "create_time",
		header:    func(c *payloadHeaderClaims) string { return c.Header.CreateTime },
		canonical: func(ev *model.Event) string { return ev.SourceTime },
	},
	{
		name:      "app_id",
		header:    func(c *payloadHeaderClaims) string { return c.Header.AppID },
		canonical: func(ev *model.Event) string { return ev.AppID },
	},
	{
		name:      "tenant_key",
		header:    func(c *payloadHeaderClaims) string { return c.Header.TenantKey },
		canonical: func(ev *model.Event) string { return ev.TenantKey },
	},
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
// A payload that is not a JSON object is not re-classified here — malformed
// handling belongs to the processing layer.
func checkCanonicalConflict(ev *model.Event) string {
	var claims payloadHeaderClaims
	if err := json.Unmarshal(ev.Payload, &claims); err != nil {
		return ""
	}
	for _, c := range canonicalFactComparisons {
		if claimed := c.header(&claims); claimed != "" && claimed != c.canonical(ev) {
			return c.name
		}
	}
	return ""
}
