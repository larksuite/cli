// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"encoding/json"
	"slices"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/adapter/localbus/protocol"
)

// legacyMetadataMode is the compatibility decision for one connection. It is
// resolved once from the handshake ack and never changes while that connection
// lives: a per-event fallback would let a forged header override canonical
// facts on a bus that does supply them, which is exactly what arbitration
// exists to prevent.
//
// TRANSITIONAL: the whole legacy path goes away when the bus protocol next
// bumps, at which point the capability check returns to selecting a restore
// strategy rather than tolerating a missing one.
type legacyMetadataMode struct {
	enabled bool
	// appID is the app this consumer is configured for. A legacy frame carries
	// no app_id, so the payload header is the only place to read it from — but
	// it is checked against this value instead of being trusted.
	appID string
}

// negotiateMetadataMode decides how this connection restores canonical facts.
//
// A bus that advertises canonical metadata needs nothing. An older bus is
// tolerated for keys whose subscription identity is one-dimensional, because
// their scope string is byte-identical across versions. Keys with a
// SubscriptionKey param are refused: their scope is hashed here and bare on
// the old bus, so old and new consumers of the same resource would each act as
// first and last for their own scope and unsubscribe one another — and the old
// bus can never grow a guard against that.
func negotiateMetadataMode(ack *protocol.HelloAck, def *event.KeyDefinition, appID string) (legacyMetadataMode, error) {
	if ack != nil && slices.Contains(ack.Capabilities, protocol.CapabilityCanonicalMetadataV1) {
		return legacyMetadataMode{}, nil
	}
	if hasSubscriptionKeyParam(def) {
		return legacyMetadataMode{}, capabilityError(def.Key)
	}
	return legacyMetadataMode{enabled: true, appID: appID}, nil
}

func hasSubscriptionKeyParam(def *event.KeyDefinition) bool {
	return slices.ContainsFunc(def.Params, func(p event.ParamDef) bool { return p.SubscriptionKey })
}

// restoreLegacyMetadata fills the canonical facts a legacy bus never sent,
// reading them from the payload header the old ingress parsed out of these
// same bytes. Facts the frame did carry are left alone: the frame stays the
// authority for everything it can speak to.
//
// It returns the name of the fact that cannot be honoured, or "" when the
// event may be delivered. Both facts are declared strings in the envelope, so
// a non-string assertion is refused rather than coerced; an absent claim
// leaves the fact empty, which is what the old consumer rendered too.
func restoreLegacyMetadata(ev *event.RawEvent, configuredAppID string) string {
	var claims payloadHeaderClaims
	if err := json.Unmarshal(ev.Payload, &claims); err != nil {
		// Nothing to restore from. The facts stay empty, exactly as an old
		// consumer would have left them.
		return ""
	}
	if ev.AppID == "" {
		claimed, ok := legacyHeaderString(claims, "app_id")
		if !ok {
			return "app_id"
		}
		// The configured app is an independent source for this one fact, so
		// the header does not get to name an app this consumer is not running
		// as — that is the forgery the arbiter would otherwise have caught.
		if claimed != "" && claimed != configuredAppID {
			return "app_id"
		}
		ev.AppID = claimed
	}
	if ev.TenantKey == "" {
		claimed, ok := legacyHeaderString(claims, "tenant_key")
		if !ok {
			return "tenant_key"
		}
		// Accepted cost of compatibility: nothing else on a legacy connection
		// knows the tenant, so this claim cannot be cross-checked.
		ev.TenantKey = claimed
	}
	return ""
}

// legacyHeaderString reads one header field as a string. ok is false only when
// the field is present and is not a string; an absent field and a JSON null
// both read as an empty string, which is how the envelope spells "not set".
func legacyHeaderString(claims payloadHeaderClaims, field string) (string, bool) {
	raw, asserted := claims.Header[field]
	if !asserted {
		return "", true
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return value, true
}

// capabilityError refuses a bus that cannot deliver full canonical metadata
// for a key whose subscription identity depends on it.
func capabilityError(eventKey string) error {
	return errs.NewValidationError(errs.SubtypeFailedPrecondition,
		"the running local event bus does not support %s, and %s subscribes per resource so it cannot fall back",
		protocol.CapabilityCanonicalMetadataV1, eventKey).
		WithHint("stop the consumers still attached to the old bus, run `lark-cli event stop` (add --force to override active consumers at the cost of dropping them), then retry `lark-cli event consume %s`", eventKey)
}

// legacyModeNotice is the one-time, per-connection line telling the operator
// which facts are being derived from the payload instead of the frame.
func legacyModeNotice(eventKey string) string {
	return "[event] legacy compatibility mode for " + eventKey +
		": the running bus predates " + protocol.CapabilityCanonicalMetadataV1 +
		", so app_id and tenant_key are read from the event payload; restart the bus (`lark-cli event stop`) once its consumers are done to leave this mode"
}
