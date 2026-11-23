// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"encoding/json"
	"testing"

	event "github.com/larksuite/cli/internal/event"
)

// A key whose server-side subscription is parameter-scoped must derive
// distinct subscription identities per parameter value: consumers of
// different resources own independent setup/cleanup lifecycles, while
// consumers of the same resource share one.
func TestSubscriptionKeyParam_SplitsScopes(t *testing.T) {
	def := &event.KeyDefinition{
		Key:       "test.evt_scoped",
		EventType: "test.evt_scoped",
		Schema:    event.SchemaDef{Native: &event.SchemaSpec{Raw: json.RawMessage(`{"type":"object"}`)}},
		Params: []event.ParamDef{
			{Name: "resource_id", Type: event.ParamString, Required: true, SubscriptionKey: true},
		},
	}

	a1 := ComputeSubscriptionID(def, map[string]string{"resource_id": "board-A"})
	a2 := ComputeSubscriptionID(def, map[string]string{"resource_id": "board-A"})
	b := ComputeSubscriptionID(def, map[string]string{"resource_id": "board-B"})

	if a1 != a2 {
		t.Errorf("same resource must share one scope: %q vs %q", a1, a2)
	}
	if a1 == b {
		t.Error("different resources must not share a subscription scope")
	}
	if a1 == def.Key || b == def.Key {
		t.Error("scoped identities must not degenerate to the bare key")
	}
}
