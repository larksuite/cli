// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package catalog

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/event/model"
	"github.com/larksuite/cli/internal/event/processing"
	"github.com/larksuite/cli/internal/event/schemas"
)

var testStrategies = StrategyRefs{StrategyNone, StrategyLegacyPreConsume}

func validDef() KeyDefinition {
	return KeyDefinition{
		Key:       "demo.thing.updated_v1",
		EventType: "demo.thing.updated_v1",
		Schema:    SchemaDef{Custom: &SchemaSpec{Raw: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}}}`)}},
		Process: func(context.Context, processing.APIClient, *model.Event, map[string]string) (json.RawMessage, error) {
			return json.RawMessage(`{}`), nil
		},
	}
}

// The rejection side: every contract violation must fail the compile, and a
// failed compile must never hand back a live snapshot.
func TestCompile_RejectsContractViolations(t *testing.T) {
	cases := map[string]struct {
		mutate  func(*KeyDefinition)
		wantMsg string
	}{
		"empty event type": {
			func(d *KeyDefinition) { d.EventType = "" },
			"EventType must not be empty",
		},
		"bad subscription type": {
			func(d *KeyDefinition) { d.SubscriptionType = "webhook" },
			"SubscriptionType must be",
		},
		"native and custom together": {
			func(d *KeyDefinition) {
				d.Schema.Native = &SchemaSpec{Raw: json.RawMessage(`{"type":"object","properties":{"x":{}}}`)}
			},
			"mutually exclusive",
		},
		"neither native nor custom": {
			func(d *KeyDefinition) { d.Schema = SchemaDef{} },
			"requires either Native or Custom",
		},
		"native with process": {
			func(d *KeyDefinition) {
				d.Schema = SchemaDef{Native: &SchemaSpec{Raw: json.RawMessage(`{"type":"object","properties":{"x":{}}}`)}}
				d.Process = func(context.Context, processing.APIClient, *model.Event, map[string]string) (json.RawMessage, error) {
					return nil, nil
				}
			},
			"forbids Process",
		},
		"spec with both type and raw": {
			func(d *KeyDefinition) {
				d.Schema.Custom.Type = jsonObjectType()
			},
			"exactly one of Type or Raw",
		},
		"enum param without values": {
			func(d *KeyDefinition) {
				d.Params = []ParamDef{{Name: "mode", Type: ParamEnum, Description: "mode"}}
			},
			"requires Values",
		},
		"enum value without desc": {
			func(d *KeyDefinition) {
				d.Params = []ParamDef{{Name: "mode", Type: ParamEnum, Description: "mode", Values: []ParamValue{{Value: "a"}}}}
			},
			"requires non-empty Desc",
		},
		"unknown param type": {
			func(d *KeyDefinition) {
				d.Params = []ParamDef{{Name: "x", Type: "float", Description: "x"}}
			},
			"unknown type",
		},
		"bad auth type": {
			func(d *KeyDefinition) { d.AuthTypes = []string{"tenant"} },
			`must be "user" or "bot"`,
		},
		"explicit domain mismatch": {
			func(d *KeyDefinition) { d.Domain = "gadget" },
			"does not match the key's first segment",
		},
		"custom schema without process": {
			func(d *KeyDefinition) { d.Process = nil },
			"Schema.Custom requires Process",
		},
		"raw schema with garbage bytes": {
			func(d *KeyDefinition) { d.Schema.Custom.Raw = json.RawMessage(`this is {{{ not json`) },
			"is not a JSON object",
		},
		"placeholder object schema": {
			func(d *KeyDefinition) { d.Schema.Custom.Raw = json.RawMessage(`{}`) },
			"empty placeholder",
		},
		"placeholder null schema": {
			func(d *KeyDefinition) { d.Schema.Custom.Raw = json.RawMessage(`null`) },
			"empty placeholder",
		},
		"orphan field override": {
			func(d *KeyDefinition) {
				d.Schema.FieldOverrides = map[string]schemas.FieldMeta{
					"/no/such/path": {Description: "dangling"},
				}
			},
			"paths the schema does not have",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			def := validDef()
			tc.mutate(&def)
			snap, err := Compile([]KeyDefinition{def}, testStrategies)
			if err == nil {
				t.Fatalf("compile must reject this declaration")
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("error should mention %q, got: %v", tc.wantMsg, err)
			}
			if snap != nil {
				t.Error("a failed compile must never produce a live snapshot")
			}
		})
	}
}

func TestCompile_RejectsDuplicateKeys(t *testing.T) {
	snap, err := Compile([]KeyDefinition{validDef(), validDef()}, testStrategies)
	if err == nil || !strings.Contains(err.Error(), "duplicate EventKey") {
		t.Fatalf("want duplicate-key rejection, got err=%v", err)
	}
	if snap != nil {
		t.Error("a failed compile must never produce a live snapshot")
	}
}

func TestCompile_RejectsUnknownStrategy(t *testing.T) {
	def := validDef()
	def.PreConsume = func(context.Context, processing.APIClient, map[string]string) (func() error, error) {
		return nil, nil
	}
	// A strategy set without legacy_preconsume cannot host a PreConsume key.
	snap, err := Compile([]KeyDefinition{def}, StrategyRefs{StrategyNone})
	if err == nil || !strings.Contains(err.Error(), "strategy") {
		t.Fatalf("want strategy rejection, got err=%v", err)
	}
	if snap != nil {
		t.Error("a failed compile must never produce a live snapshot")
	}
}

// The acceptance side: a compile that rejects everything would be just as
// broken as one that accepts everything.
func TestCompile_AcceptsWellFormedDeclarations(t *testing.T) {
	withPrep := validDef()
	withPrep.Key = "demo.other.created_v1"
	withPrep.EventType = withPrep.Key
	withPrep.PreConsume = func(context.Context, processing.APIClient, map[string]string) (func() error, error) {
		return nil, nil
	}

	snap, err := Compile([]KeyDefinition{validDef(), withPrep}, testStrategies)
	if err != nil {
		t.Fatalf("well-formed declarations must compile: %v", err)
	}
	if snap.Len() != 2 {
		t.Fatalf("compiled %d keys, want 2", snap.Len())
	}

	plain, _ := snap.Resolve("demo.thing.updated_v1")
	if got := plain.Capability().Preparation; got != StrategyNone {
		t.Errorf("key without PreConsume must project strategy %q, got %q", StrategyNone, got)
	}
	prepared, _ := snap.Resolve("demo.other.created_v1")
	if got := prepared.Capability().Preparation; got != StrategyLegacyPreConsume {
		t.Errorf("key with PreConsume must project strategy %q, got %q", StrategyLegacyPreConsume, got)
	}
}

func TestCompile_CanonicalizesDefaults(t *testing.T) {
	def := validDef()
	def.BufferSize = 5000 // above the cap
	snap, err := Compile([]KeyDefinition{def}, testStrategies)
	if err != nil {
		t.Fatal(err)
	}
	entry, _ := snap.Resolve(def.Key)
	got := entry.Definition()
	if got.SubscriptionType != SubTypeEvent {
		t.Errorf("empty SubscriptionType must canonicalize to %q, got %q", SubTypeEvent, got.SubscriptionType)
	}
	if got.BufferSize != MaxBufferSize {
		t.Errorf("BufferSize must clamp to %d, got %d", MaxBufferSize, got.BufferSize)
	}
	if got.Workers != 1 {
		t.Errorf("Workers must default to 1, got %d", got.Workers)
	}
	cap := entry.Capability()
	if cap.BufferSize != MaxBufferSize || cap.Workers != 1 {
		t.Errorf("capability must carry canonicalized delivery values, got %+v", cap)
	}
}

func TestCompile_ProjectsOutputContract(t *testing.T) {
	custom := validDef()
	native := KeyDefinition{
		Key:       "demo.native.updated_v1",
		EventType: "demo.native.updated_v1",
		Schema:    SchemaDef{Native: &SchemaSpec{Raw: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}}}`)}},
	}
	snap, err := Compile([]KeyDefinition{custom, native}, testStrategies)
	if err != nil {
		t.Fatal(err)
	}

	c, _ := snap.Resolve(custom.Key)
	if out := c.Output(); out.Mode != OutputProcessed || out.JQRootPath != "." || len(out.SchemaJSON) == 0 {
		t.Errorf("custom key contract wrong: %+v", out)
	}
	n, _ := snap.Resolve(native.Key)
	if out := n.Output(); out.Mode != OutputNative || out.JQRootPath != ".event" || len(out.SchemaJSON) == 0 {
		t.Errorf("native key contract wrong: %+v", out)
	}
	// Native schemas are delivered inside the V2 envelope; the resolved
	// schema must describe the envelope, not the bare body.
	if !strings.Contains(string(n.Output().SchemaJSON), `"header"`) {
		t.Error("native schema must be wrapped in the V2 envelope shape")
	}
}

func jsonObjectType() reflect.Type {
	return reflect.TypeOf(struct {
		ID string `json:"id"`
	}{})
}
