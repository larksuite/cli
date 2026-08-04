// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/larksuite/cli/events"
	event "github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/adapter/localbus/protocol"
	"github.com/larksuite/cli/internal/event/catalog"
)

const legacyConfiguredAppID = "cli_configured_app"

// legacyFrame builds the event frame an older bus produced: it carries the
// facts that version knew about and nothing else. The absent fields are the
// whole reason the compatibility path exists.
func legacyFrame(payload json.RawMessage) *protocol.Event {
	return &protocol.Event{
		Type:       protocol.MsgTypeEvent,
		EventType:  "im.message.receive_v1",
		EventID:    "evt-legacy-1",
		SourceTime: "1700000000000",
		Seq:        1,
		Payload:    payload,
	}
}

func legacyEnvelope(header map[string]any) json.RawMessage {
	full := map[string]any{
		"event_id":    "evt-legacy-1",
		"event_type":  "im.message.receive_v1",
		"create_time": "1700000000000",
		"app_id":      legacyConfiguredAppID,
		"tenant_key":  "tenant-legacy",
	}
	for k, v := range header {
		full[k] = v
	}
	raw, _ := json.Marshal(map[string]any{"schema": "2.0", "header": full, "event": map[string]any{}})
	return raw
}

// The frame stays the authority for everything it carries; only the facts it
// cannot speak to come from the header.
func TestLegacyRestore_FillsOnlyWhatTheFrameLacks(t *testing.T) {
	raw := restoreCanonicalEvent(legacyFrame(legacyEnvelope(nil)), nil, true)
	if field := restoreLegacyMetadata(raw, legacyConfiguredAppID); field != "" {
		t.Fatalf("a well-formed legacy event must be deliverable, got conflict on %q", field)
	}

	if raw.AppID != legacyConfiguredAppID {
		t.Errorf("app_id = %q, want it restored from the header", raw.AppID)
	}
	if raw.TenantKey != "tenant-legacy" {
		t.Errorf("tenant_key = %q, want it restored from the header", raw.TenantKey)
	}
	// Frame-supplied facts must be untouched even though the header repeats them.
	if raw.EventID != "evt-legacy-1" || raw.EventType != "im.message.receive_v1" || raw.SourceTime != "1700000000000" {
		t.Errorf("the frame must stay authoritative for the facts it carries, got %+v", raw)
	}
	// The legacy frame has no observation clock and nothing reads one.
	if !raw.Timestamp.IsZero() {
		t.Errorf("Timestamp = %v, want the zero value on a legacy frame", raw.Timestamp)
	}
}

// The configured app is an independent source for app_id, so a header naming a
// different app is a forgery rather than a fact to restore.
func TestLegacyRestore_RefusesAnAppIDTheConsumerIsNotRunningAs(t *testing.T) {
	raw := restoreCanonicalEvent(legacyFrame(legacyEnvelope(map[string]any{"app_id": "cli_forged_app"})), nil, true)
	if field := restoreLegacyMetadata(raw, legacyConfiguredAppID); field != "app_id" {
		t.Errorf("a header naming another app must conflict on app_id, got %q", field)
	}
}

// Both restored facts are declared strings; a non-string assertion is refused
// rather than coerced, matching how arbitration treats a type flip.
func TestLegacyRestore_RefusesNonStringClaims(t *testing.T) {
	for _, field := range []string{"app_id", "tenant_key"} {
		t.Run(field, func(t *testing.T) {
			raw := restoreCanonicalEvent(legacyFrame(legacyEnvelope(map[string]any{field: 42})), nil, true)
			if got := restoreLegacyMetadata(raw, legacyConfiguredAppID); got != field {
				t.Errorf("a non-string %s claim must conflict, got %q", field, got)
			}
		})
	}
}

// An absent claim leaves the fact empty, which is what the old consumer
// rendered — restoring must not invent a value.
func TestLegacyRestore_AbsentClaimLeavesTheFactEmpty(t *testing.T) {
	payload := json.RawMessage(`{"schema":"2.0","header":{"event_id":"evt-legacy-1"},"event":{}}`)
	raw := restoreCanonicalEvent(legacyFrame(payload), nil, true)
	if field := restoreLegacyMetadata(raw, legacyConfiguredAppID); field != "" {
		t.Fatalf("an absent claim is not a conflict, got %q", field)
	}
	if raw.AppID != "" || raw.TenantKey != "" {
		t.Errorf("absent claims must stay empty, got app_id=%q tenant_key=%q", raw.AppID, raw.TenantKey)
	}
}

// Compatibility narrows arbitration, it does not switch it off: the facts the
// legacy frame carries itself are still checked against the header.
func TestLegacyRestore_StillArbitratesFrameSuppliedFacts(t *testing.T) {
	forgedEventID := legacyEnvelope(map[string]any{"event_id": "evt-forged"})
	raw := restoreCanonicalEvent(legacyFrame(forgedEventID), nil, true)
	if field := restoreLegacyMetadata(raw, legacyConfiguredAppID); field != "" {
		t.Fatalf("restore itself must not object here, got %q", field)
	}
	if got := checkCanonicalConflict(raw, true); got != "event_id" {
		t.Errorf("a forged event_id must still conflict on a legacy connection, got %q", got)
	}
}

// The accepted cost, recorded deliberately: tenant_key has no second source on
// a legacy connection, so a header claiming a different tenant is honoured.
// This is the one fact compatibility cannot protect, and it is why the mode is
// transitional.
func TestLegacyRestore_TenantKeyIsTheAcceptedCost(t *testing.T) {
	raw := restoreCanonicalEvent(legacyFrame(legacyEnvelope(map[string]any{"tenant_key": "tenant-other"})), nil, true)
	if field := restoreLegacyMetadata(raw, legacyConfiguredAppID); field != "" {
		t.Fatalf("tenant_key cannot be cross-checked, so it must not conflict, got %q", field)
	}
	if got := checkCanonicalConflict(raw, true); got != "" {
		t.Fatalf("a header-derived fact must not be arbitrated against the header it came from, got %q", got)
	}
	if raw.TenantKey != "tenant-other" {
		t.Errorf("tenant_key = %q, want the header's claim", raw.TenantKey)
	}
}

// Per-key matrix over the real catalog: every shipped key must have a defined
// answer for a legacy bus, and the answer must follow from its subscription
// shape rather than from a hand-maintained list.
//
// Keys that degrade are covered end to end by composition: this asserts the
// canonical event a legacy frame restores to is identical to the one a current
// frame produces, and TestProcessedOutputBaseline freezes Process output as a
// function of that canonical event. Identical input to a frozen function is
// what makes the rendered bytes identical.
func TestLegacyBus_EveryShippedKeyHasADefinedAnswer(t *testing.T) {
	snap, err := catalog.Compile(allShippedDefs(t), catalog.StrategyRefs{catalog.StrategyNone, catalog.StrategyLegacyPreConsume})
	if err != nil {
		t.Fatalf("compile catalog: %v", err)
	}
	legacyAck := &protocol.HelloAck{Type: protocol.MsgTypeHelloAck, BusVersion: "v1", FirstForKey: true}

	refused, degraded := 0, 0
	for _, def := range snap.Definitions() {
		t.Run(def.Key, func(t *testing.T) {
			mode, err := negotiateMetadataMode(legacyAck, def, legacyConfiguredAppID)

			if hasSubscriptionKeyParam(def) {
				refused++
				if err == nil {
					t.Fatal("a resource-scoped key must be refused on a legacy bus: its scope is hashed here and bare there, so the two would unsubscribe each other")
				}
				if mode.enabled {
					t.Error("a refused key must not also be put in compatibility mode")
				}
				return
			}

			degraded++
			if err != nil {
				t.Fatalf("a one-dimensional key must degrade rather than fail, got: %v", err)
			}
			if !mode.enabled {
				t.Fatal("a legacy ack must put the connection in compatibility mode")
			}

			// The restored canonical event must equal what a current frame
			// carries natively.
			payload := legacyEnvelope(map[string]any{"event_type": def.EventType})
			old := legacyFrame(payload)
			old.EventType = def.EventType
			restored := restoreCanonicalEvent(old, nil, true)
			if field := restoreLegacyMetadata(restored, legacyConfiguredAppID); field != "" {
				t.Fatalf("legacy restore rejected a well-formed event on %q", field)
			}

			current := legacyFrame(payload)
			current.EventType = def.EventType
			current.AppID = legacyConfiguredAppID
			current.TenantKey = "tenant-legacy"
			want := restoreCanonicalEvent(current, nil, true)

			if restored.EventID != want.EventID || restored.EventType != want.EventType ||
				restored.SourceTime != want.SourceTime || restored.AppID != want.AppID ||
				restored.TenantKey != want.TenantKey || !restored.Timestamp.Equal(want.Timestamp) ||
				!bytes.Equal(restored.Payload, want.Payload) {
				t.Errorf("legacy restore diverged from the current frame:\n got %+v\nwant %+v", restored, want)
			}
		})
	}
	if refused == 0 || degraded == 0 {
		t.Fatalf("the matrix must exercise both answers, got %d refused and %d degraded", refused, degraded)
	}
}

func allShippedDefs(t *testing.T) []event.KeyDefinition {
	t.Helper()
	defs := events.All()
	if len(defs) == 0 {
		t.Fatal("no shipped EventKeys found; the matrix scanned nothing")
	}
	return defs
}
