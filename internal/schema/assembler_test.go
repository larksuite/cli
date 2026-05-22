// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"reflect"
	"testing"
)

func TestKeyOrderIndex_ImReactionsList(t *testing.T) {
	// im.reactions.list 在 meta_data.json 里 parameters 顺序是固定的：
	// message_id (path), reaction_type, page_token, page_size, user_id_type (all query)
	order := lookupKeyOrder("im", []string{"reactions"}, "list")
	if order == nil {
		t.Fatal("expected key order for im.reactions.list, got nil")
	}
	wantParameters := []string{"message_id", "reaction_type", "page_token", "page_size", "user_id_type"}
	if !reflect.DeepEqual(order.Parameters, wantParameters) {
		t.Errorf("parameters order:\ngot:  %v\nwant: %v", order.Parameters, wantParameters)
	}
	// im.reactions.list 没有 requestBody（GET 方法）
	if len(order.RequestBody) != 0 {
		t.Errorf("expected empty RequestBody, got %v", order.RequestBody)
	}
}

func TestKeyOrderIndex_ImImagesCreate(t *testing.T) {
	// im.images.create 在 meta_data.json 里 requestBody 顺序是：image_type, image
	order := lookupKeyOrder("im", []string{"images"}, "create")
	if order == nil {
		t.Fatal("expected key order for im.images.create, got nil")
	}
	wantBody := []string{"image_type", "image"}
	if !reflect.DeepEqual(order.RequestBody, wantBody) {
		t.Errorf("requestBody order:\ngot:  %v\nwant: %v", order.RequestBody, wantBody)
	}
}

func TestKeyOrderIndex_UnknownPath(t *testing.T) {
	// 远端缓存的命令（不在 embedded 内）查不到 key order，返回 nil 走字母序兜底
	order := lookupKeyOrder("nonexistent_service", []string{"foo"}, "bar")
	if order != nil {
		t.Errorf("expected nil for unknown path, got %+v", order)
	}
}

func TestConvertProperty_BasicTypes(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		wantType string
	}{
		{"string", map[string]interface{}{"type": "string"}, "string"},
		{"integer", map[string]interface{}{"type": "integer"}, "integer"},
		{"boolean", map[string]interface{}{"type": "boolean"}, "boolean"},
		{"number", map[string]interface{}{"type": "number"}, "number"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertProperty(tt.input, "")
			if got.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", got.Type, tt.wantType)
			}
		})
	}
}

func TestConvertProperty_FileBinary(t *testing.T) {
	input := map[string]interface{}{"type": "file", "description": "upload"}
	got := convertProperty(input, "")
	if got.Type != "string" {
		t.Errorf("Type = %q, want \"string\"", got.Type)
	}
	if got.Format != "binary" {
		t.Errorf("Format = %q, want \"binary\"", got.Format)
	}
}

func TestConvertProperty_OptionsToEnum(t *testing.T) {
	input := map[string]interface{}{
		"type": "string",
		"options": []interface{}{
			map[string]interface{}{"value": "banana"},
			map[string]interface{}{"value": "apple"},
			map[string]interface{}{"value": "banana"}, // duplicate
		},
	}
	got := convertProperty(input, "")
	want := []string{"apple", "banana"} // sorted + deduped
	if !reflect.DeepEqual(got.Enum, want) {
		t.Errorf("Enum = %v, want %v", got.Enum, want)
	}
}

func TestConvertProperty_EnumPassThrough(t *testing.T) {
	input := map[string]interface{}{
		"type": "string",
		"enum": []interface{}{"x", "y"},
	}
	got := convertProperty(input, "")
	want := []string{"x", "y"} // pass through, no sort
	if !reflect.DeepEqual(got.Enum, want) {
		t.Errorf("Enum = %v, want %v", got.Enum, want)
	}
}

func TestConvertProperty_MinMaxParsing(t *testing.T) {
	input := map[string]interface{}{"type": "integer", "min": "10", "max": "50"}
	got := convertProperty(input, "")
	if got.Minimum == nil || *got.Minimum != 10.0 {
		t.Errorf("Minimum = %v, want 10", got.Minimum)
	}
	if got.Maximum == nil || *got.Maximum != 50.0 {
		t.Errorf("Maximum = %v, want 50", got.Maximum)
	}
}

func TestConvertProperty_MinMaxInvalid(t *testing.T) {
	input := map[string]interface{}{"type": "integer", "min": "not_a_number"}
	got := convertProperty(input, "")
	if got.Minimum != nil {
		t.Errorf("Minimum = %v, want nil for unparseable min", got.Minimum)
	}
}

func TestConvertProperty_ArrayWithProperties(t *testing.T) {
	// meta_data quirk: array element schema is in "properties" not "items"
	input := map[string]interface{}{
		"type": "array",
		"properties": map[string]interface{}{
			"id":   map[string]interface{}{"type": "string"},
			"name": map[string]interface{}{"type": "string"},
		},
	}
	got := convertProperty(input, "")
	if got.Type != "array" {
		t.Fatalf("Type = %q, want \"array\"", got.Type)
	}
	if got.Items == nil {
		t.Fatal("Items is nil, want non-nil")
	}
	if got.Items.Type != "object" {
		t.Errorf("Items.Type = %q, want \"object\"", got.Items.Type)
	}
	if got.Items.Properties == nil || len(got.Items.Properties.Map) != 2 {
		t.Errorf("Items.Properties did not contain both id and name")
	}
	if got.Properties != nil {
		t.Error("array Property must not have top-level Properties after unfold")
	}
}

func TestConvertProperty_ObjectWithProperties(t *testing.T) {
	input := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"x": map[string]interface{}{"type": "string"},
		},
	}
	got := convertProperty(input, "")
	if got.Type != "object" {
		t.Errorf("Type = %q, want \"object\"", got.Type)
	}
	if got.Properties == nil || got.Properties.Map["x"].Type != "string" {
		t.Errorf("nested Properties not preserved")
	}
}

func TestConvertProperty_InferObjectFromProperties(t *testing.T) {
	input := map[string]interface{}{
		"properties": map[string]interface{}{
			"y": map[string]interface{}{"type": "string"},
		},
	}
	got := convertProperty(input, "")
	if got.Type != "object" {
		t.Errorf("Type = %q, want \"object\" (inferred)", got.Type)
	}
}

func TestConvertProperty_DropsRefAndAnnotations(t *testing.T) {
	input := map[string]interface{}{
		"type":        "string",
		"ref":         "operator",
		"annotations": []interface{}{"readOnly"},
		"enumName":    "FooEnum",
	}
	got := convertProperty(input, "")
	// 这些字段直接被丢弃；Property 结构里也没存这些字段，断言只有 type 设置即可
	if got.Type != "string" {
		t.Errorf("Type = %q", got.Type)
	}
}

func TestConvertProperty_DescriptionDefaultExample(t *testing.T) {
	input := map[string]interface{}{
		"type":        "string",
		"description": "hello\nworld",
		"default":     "",
		"example":     "ex",
	}
	got := convertProperty(input, "")
	if got.Description != "hello\nworld" {
		t.Errorf("Description not preserved verbatim")
	}
	if got.Default != "" {
		t.Errorf("Default = %v, want empty string (preserved)", got.Default)
	}
	if got.Example != "ex" {
		t.Errorf("Example = %v, want \"ex\"", got.Example)
	}
}
