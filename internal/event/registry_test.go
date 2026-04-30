// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func mustPanic(t *testing.T, substring string) {
	t.Helper()
	r := recover()
	if r == nil {
		t.Fatal("expected panic, got none")
	}
	msg, _ := r.(string)
	if msg == "" {
		if err, ok := r.(error); ok {
			msg = err.Error()
		} else {
			msg = fmt.Sprintf("%v", r)
		}
	}
	if !strings.Contains(msg, substring) {
		t.Errorf("panic %q does not contain %q", msg, substring)
	}
}

type emptyOut struct {
	A string `json:"a"`
}

func nativeSchema() SchemaDef {
	return SchemaDef{Native: &SchemaSpec{Type: reflect.TypeOf(emptyOut{})}}
}

func customSchema() SchemaDef {
	return SchemaDef{Custom: &SchemaSpec{Type: reflect.TypeOf(emptyOut{})}}
}

func customProcess() func(context.Context, APIClient, *RawEvent, map[string]string) (json.RawMessage, error) {
	return func(context.Context, APIClient, *RawEvent, map[string]string) (json.RawMessage, error) {
		return nil, nil
	}
}

func TestRegisterKey_NativeOnly(t *testing.T) {
	resetRegistry()
	RegisterKey(KeyDefinition{
		Key:       "t.native",
		EventType: "t.native",
		Schema:    nativeSchema(),
	})
	def, ok := Lookup("t.native")
	if !ok {
		t.Fatal("Lookup failed")
	}
	if def.Schema.Native == nil {
		t.Fatal("Native not stored")
	}
	if def.Process != nil {
		t.Error("Process should be nil for Native")
	}
}

func TestRegisterKey_CustomWithProcess(t *testing.T) {
	resetRegistry()
	RegisterKey(KeyDefinition{
		Key:       "t.custom",
		EventType: "t.custom",
		Schema:    customSchema(),
		Process:   customProcess(),
	})
	def, ok := Lookup("t.custom")
	if !ok {
		t.Fatal("Lookup failed")
	}
	if def.Schema.Custom == nil {
		t.Fatal("Custom not stored")
	}
	if def.Process == nil {
		t.Error("Process should be set")
	}
}

func TestRegisterKey_DuplicatePanics(t *testing.T) {
	resetRegistry()
	RegisterKey(KeyDefinition{Key: "t.dup", EventType: "t.dup", Schema: nativeSchema()})
	defer mustPanic(t, "duplicate EventKey")
	RegisterKey(KeyDefinition{Key: "t.dup", EventType: "t.dup", Schema: nativeSchema()})
}

func TestRegisterKey_EmptyEventTypePanics(t *testing.T) {
	resetRegistry()
	defer mustPanic(t, "EventType must not be empty")
	RegisterKey(KeyDefinition{Key: "t.no_type", Schema: nativeSchema()})
}

func TestRegisterKey_PanicsWhenBothSchemasSet(t *testing.T) {
	resetRegistry()
	defer mustPanic(t, "mutually exclusive")
	RegisterKey(KeyDefinition{
		Key:       "t.both",
		EventType: "t.both",
		Schema: SchemaDef{
			Native: &SchemaSpec{Type: reflect.TypeOf(emptyOut{})},
			Custom: &SchemaSpec{Type: reflect.TypeOf(emptyOut{})},
		},
	})
}

func TestRegisterKey_PanicsWhenNoSchemaSet(t *testing.T) {
	resetRegistry()
	defer mustPanic(t, "Schema requires either Native or Custom")
	RegisterKey(KeyDefinition{Key: "t.empty", EventType: "t.empty"})
}

func TestRegisterKey_PanicsWhenNativeWithProcess(t *testing.T) {
	resetRegistry()
	defer mustPanic(t, "Schema.Native forbids Process")
	RegisterKey(KeyDefinition{
		Key:       "t.badcombo",
		EventType: "t.badcombo",
		Schema:    nativeSchema(),
		Process:   customProcess(),
	})
}

func TestRegisterKey_PanicsWhenSpecHasBothTypeAndRaw(t *testing.T) {
	resetRegistry()
	defer mustPanic(t, "requires exactly one of Type or Raw")
	RegisterKey(KeyDefinition{
		Key:       "t.bothsrc",
		EventType: "t.bothsrc",
		Schema: SchemaDef{
			Custom: &SchemaSpec{Type: reflect.TypeOf(emptyOut{}), Raw: json.RawMessage(`{}`)},
		},
	})
}

func TestRegisterKey_PanicsWhenSpecHasNeitherTypeNorRaw(t *testing.T) {
	resetRegistry()
	defer mustPanic(t, "requires exactly one of Type or Raw")
	RegisterKey(KeyDefinition{
		Key:       "t.nosrc",
		EventType: "t.nosrc",
		Schema: SchemaDef{
			Custom: &SchemaSpec{},
		},
	})
}

func TestRegisterKey_ParamMultiRequiresValues(t *testing.T) {
	resetRegistry()
	defer mustPanic(t, "requires Values")
	RegisterKey(KeyDefinition{
		Key:       "t.paramnovalues",
		EventType: "t.paramnovalues",
		Schema:    nativeSchema(),
		Params:    []ParamDef{{Name: "fields", Type: ParamMulti}},
	})
}

func TestRegisterKey_ParamEnumRequiresValues(t *testing.T) {
	resetRegistry()
	defer mustPanic(t, "requires Values")
	RegisterKey(KeyDefinition{
		Key:       "t.enumnovalues",
		EventType: "t.enumnovalues",
		Schema:    nativeSchema(),
		Params:    []ParamDef{{Name: "mode", Type: ParamEnum}},
	})
}

func TestRegisterKey_ParamValueRequiresDesc(t *testing.T) {
	resetRegistry()
	defer mustPanic(t, "requires non-empty Desc")
	RegisterKey(KeyDefinition{
		Key:       "t.paramdesc",
		EventType: "t.paramdesc",
		Schema:    nativeSchema(),
		Params: []ParamDef{{
			Name:   "f",
			Type:   ParamEnum,
			Values: []ParamValue{{Value: "x"}},
		}},
	})
}

func TestRegisterKey_UnknownParamType(t *testing.T) {
	resetRegistry()
	defer mustPanic(t, "unknown type")
	RegisterKey(KeyDefinition{
		Key:       "t.badtype",
		EventType: "t.badtype",
		Schema:    nativeSchema(),
		Params:    []ParamDef{{Name: "x", Type: ParamType("wtf")}},
	})
}

func TestRegisterKey_InvalidAuthTypesPanics(t *testing.T) {
	resetRegistry()
	defer mustPanic(t, "AuthTypes elements must be")
	RegisterKey(KeyDefinition{
		Key:       "t.badauth",
		EventType: "t.badauth",
		Schema:    nativeSchema(),
		AuthTypes: []string{"invalid"},
	})
}

func TestRegisterKey_ValidAuthTypes(t *testing.T) {
	resetRegistry()
	RegisterKey(KeyDefinition{Key: "u.e", EventType: "u.e", Schema: nativeSchema(), AuthTypes: []string{"user"}})
	RegisterKey(KeyDefinition{Key: "b.e", EventType: "b.e", Schema: nativeSchema(), AuthTypes: []string{"bot"}})
	RegisterKey(KeyDefinition{Key: "ub.e", EventType: "ub.e", Schema: nativeSchema(), AuthTypes: []string{"bot", "user"}})
	RegisterKey(KeyDefinition{Key: "na.e", EventType: "na.e", Schema: nativeSchema()})
}

func TestListAll_SortedByKey(t *testing.T) {
	resetRegistry()
	RegisterKey(KeyDefinition{Key: "z.event", EventType: "z", Schema: nativeSchema()})
	RegisterKey(KeyDefinition{Key: "a.event", EventType: "a", Schema: nativeSchema()})
	RegisterKey(KeyDefinition{Key: "m.event", EventType: "m", Schema: nativeSchema()})
	all := ListAll()
	if len(all) != 3 || all[0].Key != "a.event" || all[1].Key != "m.event" || all[2].Key != "z.event" {
		t.Errorf("keys not sorted: %v", []string{all[0].Key, all[1].Key, all[2].Key})
	}
}

func TestBufferSize_Clamped(t *testing.T) {
	resetRegistry()
	RegisterKey(KeyDefinition{
		Key: "big", EventType: "big", Schema: nativeSchema(),
		BufferSize: 5000,
	})
	def, _ := Lookup("big")
	if def.BufferSize != MaxBufferSize {
		t.Errorf("BufferSize = %d, want %d", def.BufferSize, MaxBufferSize)
	}
}
