// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"encoding/json"
	"testing"

	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/catalog"
)

// fillCanonicalFromHeader copies the payload envelope header metadata onto
// the RawEvent canonical fields. Process handlers read event_id and
// create_time from the RawEvent, which the consume pipeline fills from the
// envelope header before dispatch; tests that hand-build a RawEvent must
// mirror that so both views agree.
func fillCanonicalFromHeader(t *testing.T, raw *event.RawEvent) {
	t.Helper()
	var envelope struct {
		Header struct {
			EventID    string `json:"event_id"`
			EventType  string `json:"event_type"`
			CreateTime string `json:"create_time"`
		} `json:"header"`
	}
	if err := json.Unmarshal(raw.Payload, &envelope); err != nil {
		t.Fatalf("parse envelope header: %v", err)
	}
	raw.EventID = envelope.Header.EventID
	if envelope.Header.EventType != "" {
		raw.EventType = envelope.Header.EventType
	}
	raw.SourceTime = envelope.Header.CreateTime
}

// lookupCompiledDef compiles this domain's declarations and resolves one key,
// exactly as the runtime catalog would for a consumer.
func lookupCompiledDef(t *testing.T, key string) (*event.KeyDefinition, bool) {
	t.Helper()
	snap, err := catalog.Compile(Keys(), catalog.StrategyRefs{
		catalog.StrategyNone,
		catalog.StrategyLegacyPreConsume,
	})
	if err != nil {
		t.Fatalf("catalog.Compile(Keys()): %v", err)
	}
	entry, ok := snap.Resolve(key)
	if !ok {
		return nil, false
	}
	return entry.Definition(), true
}
