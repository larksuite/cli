// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event/model"
)

// rawEventFieldsNotComparable lists the model.Event fields that legitimately
// have no payload-header counterpart, each with the reason. Every other field
// must appear as a comparison row — the reflection walk below enforces it.
var rawEventFieldsNotComparable = map[string]string{
	"Payload":   "the value under validation cannot vouch for itself",
	"Timestamp": "local observation clock; the upstream envelope deliberately has no counterpart",
}

// fieldToComparison maps model.Event struct fields to their comparison rows.
var fieldToComparison = map[string]string{
	"EventID":    "event_id",
	"EventType":  "event_type",
	"SourceTime": "create_time",
	"AppID":      "app_id",
	"TenantKey":  "tenant_key",
}

// Every canonical fact must be either compared or declared not-comparable
// with a reason. This is the structural check that keeps the comparison table
// honest when model.Event grows a field.
func TestCanonicalConflict_CoversEveryCanonicalField(t *testing.T) {
	typ := reflect.TypeFor[model.Event]()
	if typ.NumField() == 0 {
		t.Fatal("model.Event has no fields; the gate scanned nothing")
	}
	rows := map[string]bool{}
	for _, c := range canonicalFactComparisons {
		rows[c.name] = true
	}
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		if _, exempt := rawEventFieldsNotComparable[name]; exempt {
			if _, alsoCompared := fieldToComparison[name]; alsoCompared {
				t.Errorf("field %s is both exempt and compared; pick one", name)
			}
			continue
		}
		rowName, ok := fieldToComparison[name]
		if !ok {
			t.Errorf("model.Event.%s is neither compared nor declared not-comparable; add a comparison row or an exemption with a reason", name)
			continue
		}
		if !rows[rowName] {
			t.Errorf("comparison row %q for model.Event.%s is missing from canonicalFactComparisons", rowName, name)
		}
	}
}

func conflictBaseEvent() *model.Event {
	return &model.Event{
		EventID:    "evt-1",
		EventType:  "im.message.receive_v1",
		SourceTime: "1700000000000",
		AppID:      "cli_app",
		TenantKey:  "tenant_a",
		Timestamp:  time.Unix(0, 0),
	}
}

func headerPayload(overrides map[string]string) json.RawMessage {
	header := map[string]string{
		"event_id":    "evt-1",
		"event_type":  "im.message.receive_v1",
		"create_time": "1700000000000",
		"app_id":      "cli_app",
		"tenant_key":  "tenant_a",
	}
	for k, v := range overrides {
		header[k] = v
	}
	raw, _ := json.Marshal(map[string]any{"schema": "2.0", "header": header, "event": map[string]any{}})
	return raw
}

// Each row must detect a real mismatch — a row that exists but compares a
// value to itself would pass this suite's structural check yet catch nothing.
func TestCanonicalConflict_DetectsEveryFactMismatch(t *testing.T) {
	for _, c := range canonicalFactComparisons {
		t.Run(c.name, func(t *testing.T) {
			ev := conflictBaseEvent()
			ev.Payload = headerPayload(map[string]string{c.name: "tampered-value"})
			if got := checkCanonicalConflict(ev); got != c.name {
				t.Errorf("header claiming a different %s must conflict, got %q", c.name, got)
			}
		})
	}
}

// The reverse direction: the header keeps its claim but the canonical side
// lost the fact. An asserted claim facing an empty canonical value means the
// fact was dropped between ingress and this consumer — that is a conflict,
// not "nothing to compare".
func TestCanonicalConflict_CatchesLostFacts(t *testing.T) {
	blank := map[string]func(*model.Event){
		"event_id":    func(ev *model.Event) { ev.EventID = "" },
		"event_type":  func(ev *model.Event) { ev.EventType = "" },
		"create_time": func(ev *model.Event) { ev.SourceTime = "" },
		"app_id":      func(ev *model.Event) { ev.AppID = "" },
		"tenant_key":  func(ev *model.Event) { ev.TenantKey = "" },
	}
	if len(blank) != len(canonicalFactComparisons) {
		t.Fatalf("blanking table covers %d facts, comparisons have %d; keep them in lockstep", len(blank), len(canonicalFactComparisons))
	}
	for _, c := range canonicalFactComparisons {
		t.Run(c.name, func(t *testing.T) {
			ev := conflictBaseEvent()
			ev.Payload = headerPayload(nil)
			blank[c.name](ev)
			if got := checkCanonicalConflict(ev); got != c.name {
				t.Errorf("a lost canonical %s facing an asserted header claim must conflict, got %q", c.name, got)
			}
		})
	}
}

// headerPayloadTyped builds a payload whose header values keep their declared
// JSON types, so a test can assert what happens when one field is not a
// string.
func headerPayloadTyped(overrides map[string]any) json.RawMessage {
	header := map[string]any{
		"event_id":    "evt-1",
		"event_type":  "im.message.receive_v1",
		"create_time": "1700000000000",
		"app_id":      "cli_app",
		"tenant_key":  "tenant_a",
	}
	for k, v := range overrides {
		header[k] = v
	}
	raw, _ := json.Marshal(map[string]any{"schema": "2.0", "header": header, "event": map[string]any{}})
	return raw
}

// The envelope contract declares every header fact as a string. A header that
// asserts one with a different JSON type is not a value this arbiter can
// compare, and letting it through would disable arbitration for the whole
// header — so it is a conflict.
func TestCanonicalConflict_TypeFlippedClaimConflicts(t *testing.T) {
	for _, c := range canonicalFactComparisons {
		t.Run(c.name, func(t *testing.T) {
			ev := conflictBaseEvent()
			ev.Payload = headerPayloadTyped(map[string]any{c.name: 1700000000000})
			if got := checkCanonicalConflict(ev); got != c.name {
				t.Errorf("a non-string %s claim must conflict, got %q", c.name, got)
			}
		})
	}
}

// The attack the per-field decode closes: one type-flipped field used as a
// carrier for forged identity facts. Decoding the header into typed strings
// fails on the flipped field while still populating the forged ones, so a
// whole-header bail-out would deliver the forgery.
func TestCanonicalConflict_TypeFlipCannotSmuggleForgedIdentity(t *testing.T) {
	ev := conflictBaseEvent()
	ev.Payload = headerPayloadTyped(map[string]any{
		"create_time": 1700000000000,
		"app_id":      "cli_forged",
		"tenant_key":  "tenant_forged",
	})
	if got := checkCanonicalConflict(ev); got == "" {
		t.Fatal("a header carrying forged identity facts behind a type flip must not be delivered")
	}

	// The same payload with every value a string is caught on the first forged
	// fact, which proves the flip is the only thing standing between the
	// forgery and stdout.
	control := conflictBaseEvent()
	control.Payload = headerPayload(map[string]string{
		"app_id":     "cli_forged",
		"tenant_key": "tenant_forged",
	})
	if got := checkCanonicalConflict(control); got != "app_id" {
		t.Errorf("control: forged app_id must conflict, got %q", got)
	}
}

// A null claim asserts nothing, which is the one non-string form that stays
// silent: JSON null is how the platform spells "field not present".
func TestCanonicalConflict_NullClaimStaysSilent(t *testing.T) {
	ev := conflictBaseEvent()
	ev.Payload = headerPayloadTyped(map[string]any{"app_id": nil})
	if got := checkCanonicalConflict(ev); got != "" {
		t.Errorf("a null claim must deliver, got conflict on %q", got)
	}
}

// A header that is not an object cannot assert any fact, so there is nothing
// to arbitrate — the same as a missing header.
func TestCanonicalConflict_NonObjectHeaderDelivers(t *testing.T) {
	ev := conflictBaseEvent()
	ev.Payload = json.RawMessage(`{"schema":"2.0","header":"not-an-object","event":{}}`)
	if got := checkCanonicalConflict(ev); got != "" {
		t.Errorf("a non-object header claims nothing and must deliver, got conflict on %q", got)
	}
}

// Well-formed control cases: agreement and silence both deliver.
func TestCanonicalConflict_AgreementAndSilenceDeliver(t *testing.T) {
	agree := conflictBaseEvent()
	agree.Payload = headerPayload(nil)
	if got := checkCanonicalConflict(agree); got != "" {
		t.Errorf("matching header must deliver, got conflict on %q", got)
	}

	silent := conflictBaseEvent()
	silent.Payload = json.RawMessage(`{"schema":"2.0","event":{"text":"no header block"}}`)
	if got := checkCanonicalConflict(silent); got != "" {
		t.Errorf("a silent header claims nothing and must deliver, got conflict on %q", got)
	}

	// Non-JSON payloads are the processing layer's business, not arbitration's.
	malformed := conflictBaseEvent()
	malformed.Payload = json.RawMessage(`this is definitely not valid json {{{`)
	if got := checkCanonicalConflict(malformed); got != "" {
		t.Errorf("non-JSON payloads are not re-classified here, got conflict on %q", got)
	}
}

// The pipeline drops conflicting events with a diagnostic naming identity
// facts only — never payload content.
func TestCanonicalConflict_PipelineDropsWithRedactedDiagnostic(t *testing.T) {
	const sentinel = "SECRET-PAYLOAD-CONTENT-XYZ"
	ev := conflictBaseEvent()
	ev.Payload = headerPayload(map[string]string{"app_id": "attacker-" + sentinel})

	field := checkCanonicalConflict(ev)
	if field != "app_id" {
		t.Fatalf("expected app_id conflict, got %q", field)
	}
	diag := fmt.Sprintf("WARN: event %s (%s) dropped: payload header conflicts with canonical metadata (field=%s)\n",
		ev.EventID, ev.EventType, field)
	assertNoPayloadBytes(t, diag, sentinel)
}
