// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package events_test

import (
	"encoding/json"
	"testing"

	"github.com/larksuite/cli/events"
	"github.com/larksuite/cli/internal/event/catalog"
)

// This gate lives in the events package because the catalog package cannot
// import the declarations it compiles (that would be an import cycle). It is
// the acceptance half of the compiler's own rejection tests: the real catalog
// must compile — a compiler that rejects everything would also pass those.
func TestCompile_RealCatalogCompilesCleanly(t *testing.T) {
	snap, err := catalog.Compile(events.All(), catalog.StrategyRefs{
		catalog.StrategyNone,
		catalog.StrategyLegacyPreConsume,
	})
	if err != nil {
		t.Fatalf("the shipped declarations must compile: %v", err)
	}
	if snap.Len() == 0 {
		t.Fatal("the compiled catalog is empty; the gate proved nothing")
	}
	if snap.Len() != len(expectedKeys) {
		t.Fatalf("compiled %d keys, frozen baseline has %d", snap.Len(), len(expectedKeys))
	}
	for _, want := range expectedKeys {
		if _, ok := snap.Resolve(want); !ok {
			t.Errorf("baseline key missing from the compiled catalog: %s", want)
		}
	}
}

// Every shipped key must satisfy its compiled output contract: a resolvable
// non-empty schema, a jq root that matches the output mode, and normalized
// delivery values. Golden files pin a few representative keys byte-for-byte;
// this covers the whole catalog structurally.
func TestOutputContract_HoldsForEveryKey(t *testing.T) {
	snap, err := catalog.Compile(events.All(), catalog.StrategyRefs{
		catalog.StrategyNone,
		catalog.StrategyLegacyPreConsume,
	})
	if err != nil {
		t.Fatal(err)
	}

	checked := 0
	for _, entry := range snap.Entries() {
		checked++
		d := entry.Descriptor()
		out := entry.Output()

		var parsed map[string]json.RawMessage
		if err := json.Unmarshal(out.SchemaJSON, &parsed); err != nil || len(parsed) == 0 {
			t.Errorf("%s: resolved schema must be a non-empty JSON object (err=%v)", d.Key, err)
		}

		switch out.Mode {
		case catalog.OutputNative:
			if out.JQRootPath != ".event" {
				t.Errorf("%s: native keys deliver the V2 envelope; jq root must be .event, got %q", d.Key, out.JQRootPath)
			}
			if entry.Binding().Process != nil {
				t.Errorf("%s: native keys must not carry a processor", d.Key)
			}
		case catalog.OutputProcessed:
			if out.JQRootPath != "." {
				t.Errorf("%s: processed keys deliver a flat shape; jq root must be ., got %q", d.Key, out.JQRootPath)
			}
			if entry.Binding().Process == nil {
				t.Errorf("%s: processed keys must carry a processor", d.Key)
			}
		default:
			t.Errorf("%s: unknown output mode %q", d.Key, out.Mode)
		}

		cap := entry.Capability()
		if cap.BufferSize <= 0 || cap.BufferSize > catalog.MaxBufferSize || cap.Workers <= 0 {
			t.Errorf("%s: delivery values must be normalized, got buffer=%d workers=%d", d.Key, cap.BufferSize, cap.Workers)
		}
		if d.Domain == "" {
			t.Errorf("%s: descriptor domain must always be resolved", d.Key)
		}
	}
	if checked == 0 {
		t.Fatal("no entries were checked; the gate proved nothing")
	}
}
