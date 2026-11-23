// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package catalog

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/larksuite/cli/internal/event/model"
	"github.com/larksuite/cli/internal/event/processing"
	"github.com/larksuite/cli/internal/event/schemas"
)

func compiledFixture(t *testing.T) *Snapshot {
	t.Helper()
	def := validDef()
	def.Params = []ParamDef{{Name: "mode", Type: ParamEnum, Description: "mode",
		Values: []ParamValue{{Value: "a", Desc: "first"}}}}
	def.Scopes = []string{"demo:read"}
	def.AuthTypes = []string{"user"}
	def.RequiredConsoleEvents = []string{"demo.thing.updated_v1"}
	snap, err := Compile([]KeyDefinition{def}, testStrategies)
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

// Mutating anything an accessor returns must not affect what the snapshot
// hands out next — otherwise one caller can silently rewrite the catalog for
// every other caller.
func TestSnapshot_IsImmutableFromOutside(t *testing.T) {
	snap := compiledFixture(t)
	entry, ok := snap.Resolve(validDef().Key)
	if !ok {
		t.Fatal("the compiled fixture does not contain its own key")
	}

	d := entry.Descriptor()
	d.Params[0].Name = "tampered"
	d.Params[0].Values[0].Value = "tampered"
	d.Scopes[0] = "tampered"
	d.AuthTypes[0] = "tampered"
	d.RequiredConsoleEvents[0] = "tampered"
	if fresh := entry.Descriptor(); fresh.Params[0].Name == "tampered" ||
		fresh.Params[0].Values[0].Value == "tampered" ||
		fresh.Scopes[0] == "tampered" ||
		fresh.AuthTypes[0] == "tampered" ||
		fresh.RequiredConsoleEvents[0] == "tampered" {
		t.Error("mutating a returned Descriptor leaked into the snapshot")
	}

	out := entry.Output()
	if len(out.SchemaJSON) > 0 {
		out.SchemaJSON[0] = '!'
		if fresh := entry.Output(); fresh.SchemaJSON[0] == '!' {
			t.Error("mutating a returned schema leaked into the snapshot")
		}
	}

	def := entry.Definition()
	def.Params[0].Name = "tampered"
	def.Scopes[0] = "tampered"
	if def.Schema.Custom == nil || len(def.Schema.Custom.Raw) == 0 {
		t.Fatal("the fixture no longer declares raw custom schema bytes; this check needs them")
	}
	def.Schema.Custom.Raw[0] = '!'
	if fresh := entry.Definition(); fresh.Params[0].Name == "tampered" ||
		fresh.Scopes[0] == "tampered" ||
		fresh.Schema.Custom.Raw[0] == '!' {
		t.Error("mutating a returned Definition leaked into the snapshot")
	}

	keys := snap.Keys()
	keys[0] = "tampered"
	if snap.Keys()[0] == "tampered" {
		t.Error("mutating the returned key list leaked into the snapshot")
	}
}

// FieldOverrides values carry a slice-typed member (FieldMeta.Enum), so a
// shallow map clone still shares the Enum backing arrays: writing through one
// copy would rewrite the catalog for everyone. Both directions must hold —
// a returned Definition and the original compile input are equally outside.
func TestSnapshot_FieldOverrideEnumIsNotShared(t *testing.T) {
	def := validDef()
	def.Schema.FieldOverrides = map[string]schemas.FieldMeta{
		"/id": {Description: "the id", Enum: []string{"a", "b"}},
	}
	snap, err := Compile([]KeyDefinition{def}, testStrategies)
	if err != nil {
		t.Fatal(err)
	}
	entry, _ := snap.Resolve(def.Key)

	got := entry.Definition()
	got.Schema.FieldOverrides["/id"].Enum[0] = "tampered-via-definition"
	if fresh := entry.Definition(); fresh.Schema.FieldOverrides["/id"].Enum[0] != "a" {
		t.Error("mutating a returned Definition's FieldOverrides Enum leaked into the snapshot")
	}

	def.Schema.FieldOverrides["/id"].Enum[1] = "tampered-via-input"
	if fresh := entry.Definition(); fresh.Schema.FieldOverrides["/id"].Enum[1] != "b" {
		t.Error("mutating the compile input's FieldOverrides Enum leaked into the snapshot")
	}
}

// The compiler deep-copies its input: mutating the declaration after Compile
// must not reach the snapshot either.
func TestSnapshot_DoesNotAliasCompileInput(t *testing.T) {
	def := validDef()
	def.Scopes = []string{"demo:read"}
	snap, err := Compile([]KeyDefinition{def}, testStrategies)
	if err != nil {
		t.Fatal(err)
	}
	def.Scopes[0] = "tampered"
	entry, _ := snap.Resolve(def.Key)
	if entry.Definition().Scopes[0] == "tampered" {
		t.Error("the snapshot aliases the caller's declaration")
	}
}

// keyDefinitionRouting states, for every KeyDefinition field, which projection
// carries it. A new field must be routed here (and actually projected) before
// it ships — this is the structural check the round-trip test cannot do,
// because an unprojected field is zero on both sides of a round trip.
var keyDefinitionRouting = map[string]string{
	"Key":                   "Descriptor",
	"DisplayName":           "Descriptor",
	"Description":           "Descriptor",
	"EventType":             "Descriptor",
	"Domain":                "Descriptor (derived; compat view keeps the raw declaration)",
	"SubscriptionType":      "Descriptor",
	"Params":                "Descriptor",
	"Schema":                "OutputContract (resolved schema + mode)",
	"NormalizeParams":       "RuntimeBinding",
	"Process":               "RuntimeBinding",
	"Match":                 "RuntimeBinding",
	"PreConsume":            "RuntimeBinding + Capability.Preparation",
	"Scopes":                "Descriptor",
	"AuthTypes":             "Descriptor",
	"RequiredConsoleEvents": "Descriptor",
	"BufferSize":            "Capability",
	"Workers":               "Capability",
	"SingleConsumer":        "Capability",
}

func TestProjection_EveryKeyDefinitionFieldIsRouted(t *testing.T) {
	typ := reflect.TypeFor[KeyDefinition]()
	if typ.NumField() == 0 {
		t.Fatal("KeyDefinition has no fields; the gate scanned nothing")
	}
	seen := map[string]bool{}
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		seen[name] = true
		if _, ok := keyDefinitionRouting[name]; !ok {
			t.Errorf("KeyDefinition.%s is not routed to any projection; route it in the compiler and record it here", name)
		}
	}
	for name := range keyDefinitionRouting {
		if !seen[name] {
			t.Errorf("routing entry %q is stale: KeyDefinition no longer has that field", name)
		}
	}
}

// The round-trip half of the projection proof: a compiled entry's
// compatibility view equals the canonicalized input, hooks included.
func TestProjection_DefinitionRoundTripsCanonicalInput(t *testing.T) {
	var normalizeCalls, processCalls int
	def := validDef()
	def.DisplayName = "Demo thing updated"
	def.Description = "fires when the demo thing changes"
	def.Params = []ParamDef{{Name: "mode", Type: ParamEnum, Description: "m",
		Values: []ParamValue{{Value: "a", Desc: "first"}}, SubscriptionKey: true}}
	def.Scopes = []string{"demo:read"}
	def.AuthTypes = []string{"user", "bot"}
	def.RequiredConsoleEvents = []string{"demo.thing.updated_v1"}
	def.SingleConsumer = true
	def.NormalizeParams = func(context.Context, processing.APIClient, map[string]string) error {
		normalizeCalls++
		return nil
	}
	def.Process = func(context.Context, processing.APIClient, *model.Event, map[string]string) (json.RawMessage, error) {
		processCalls++
		return json.RawMessage(`{}`), nil
	}

	snap, err := Compile([]KeyDefinition{def}, testStrategies)
	if err != nil {
		t.Fatal(err)
	}
	entry, _ := snap.Resolve(def.Key)
	got := entry.Definition()
	want := Canonicalize(def)

	// Function values cannot be compared with DeepEqual; prove identity by
	// invocation, then blank them for the value comparison.
	if got.NormalizeParams == nil || got.Process == nil {
		t.Fatal("hooks were dropped by the projection")
	}
	_ = got.NormalizeParams(context.Background(), nil, nil)
	_, _ = got.Process(context.Background(), nil, nil, nil)
	if normalizeCalls != 1 || processCalls != 1 {
		t.Errorf("projected hooks are not the declared functions: normalize=%d process=%d", normalizeCalls, processCalls)
	}
	got.NormalizeParams, want.NormalizeParams = nil, nil
	got.Process, want.Process = nil, nil

	if !reflect.DeepEqual(*got, want) {
		t.Errorf("Definition() != Canonicalize(input)\n got: %+v\nwant: %+v", *got, want)
	}
}
