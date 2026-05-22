// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/registry"
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

func TestBuildInputSchema_ReactionsList(t *testing.T) {
	method := loadMethodFromRegistry(t, "im", []string{"reactions"}, "list")
	mko := lookupKeyOrder("im", []string{"reactions"}, "list")
	currentMethodOrder = mko
	defer func() { currentMethodOrder = nil }()

	is := buildInputSchema(method)

	if is.Type != "object" {
		t.Errorf("Type = %q, want \"object\"", is.Type)
	}
	// required is alphabetical
	if !reflect.DeepEqual(is.Required, []string{"message_id"}) {
		t.Errorf("Required = %v, want [message_id]", is.Required)
	}
	// properties preserves meta_data order: message_id, reaction_type, page_token, page_size, user_id_type
	wantOrder := []string{"message_id", "reaction_type", "page_token", "page_size", "user_id_type"}
	if !reflect.DeepEqual(is.Properties.Order, wantOrder) {
		t.Errorf("properties order = %v, want %v", is.Properties.Order, wantOrder)
	}
	// message_id has x-in: path
	if is.Properties.Map["message_id"].XIn != "path" {
		t.Errorf("message_id.XIn = %q, want \"path\"", is.Properties.Map["message_id"].XIn)
	}
	// reaction_type has x-in: query
	if is.Properties.Map["reaction_type"].XIn != "query" {
		t.Errorf("reaction_type.XIn = %q, want \"query\"", is.Properties.Map["reaction_type"].XIn)
	}
}

func TestBuildInputSchema_ImagesCreate_FileAndBody(t *testing.T) {
	method := loadMethodFromRegistry(t, "im", []string{"images"}, "create")
	mko := lookupKeyOrder("im", []string{"images"}, "create")
	currentMethodOrder = mko
	defer func() { currentMethodOrder = nil }()

	is := buildInputSchema(method)

	// required is alphabetical: image, image_type
	if !reflect.DeepEqual(is.Required, []string{"image", "image_type"}) {
		t.Errorf("Required = %v, want [image, image_type]", is.Required)
	}
	// properties preserves meta_data order: image_type, image
	wantOrder := []string{"image_type", "image"}
	if !reflect.DeepEqual(is.Properties.Order, wantOrder) {
		t.Errorf("properties order = %v, want %v", is.Properties.Order, wantOrder)
	}
	// image field: string + binary + body
	img := is.Properties.Map["image"]
	if img.Type != "string" {
		t.Errorf("image.Type = %q, want \"string\"", img.Type)
	}
	if img.Format != "binary" {
		t.Errorf("image.Format = %q, want \"binary\"", img.Format)
	}
	if img.XIn != "body" {
		t.Errorf("image.XIn = %q, want \"body\"", img.XIn)
	}
	// image_type: enum present, body
	if it := is.Properties.Map["image_type"]; it.XIn != "body" || !reflect.DeepEqual(it.Enum, []string{"message", "avatar"}) {
		t.Errorf("image_type unexpected: %+v", it)
	}
}

func TestBuildInputSchema_HighRiskWriteInjectsYes(t *testing.T) {
	// Synthesized method to avoid registry-overlay variance (remote cache may
	// strip `risk` field); buildInputSchema only cares about the method map.
	method := map[string]interface{}{
		"risk": "high-risk-write",
		"parameters": map[string]interface{}{
			"message_id": map[string]interface{}{
				"type":     "string",
				"location": "path",
				"required": true,
			},
		},
	}
	currentMethodOrder = nil
	defer func() { currentMethodOrder = nil }()

	is := buildInputSchema(method)

	yes, ok := is.Properties.Map["yes"]
	if !ok {
		t.Fatal("expected `yes` property in high-risk-write envelope, not found")
	}
	if yes.Type != "boolean" {
		t.Errorf("yes.Type = %q, want \"boolean\"", yes.Type)
	}
	if v, _ := yes.Default.(bool); v != false {
		t.Errorf("yes.Default = %v, want false", yes.Default)
	}
	// yes must NOT be in required
	for _, r := range is.Required {
		if r == "yes" {
			t.Errorf("`yes` should not appear in required")
		}
	}
	// yes is appended to properties.Order
	last := is.Properties.Order[len(is.Properties.Order)-1]
	if last != "yes" {
		t.Errorf("`yes` should be last in properties.Order, got: %v", is.Properties.Order)
	}
}

func TestBuildInputSchema_NoYesForReadRisk(t *testing.T) {
	method := loadMethodFromRegistry(t, "im", []string{"reactions"}, "list")
	mko := lookupKeyOrder("im", []string{"reactions"}, "list")
	currentMethodOrder = mko
	defer func() { currentMethodOrder = nil }()

	is := buildInputSchema(method)
	if _, ok := is.Properties.Map["yes"]; ok {
		t.Errorf("`yes` must not be injected for risk=read")
	}
}

// loadMethodFromRegistry is a test helper that pulls one method's spec from the
// real embedded meta_data.json via the registry package.
func loadMethodFromRegistry(t *testing.T, service string, resourcePath []string, methodName string) map[string]interface{} {
	t.Helper()
	spec := registry.LoadFromMeta(service)
	if spec == nil {
		t.Fatalf("service %q not found in registry", service)
	}
	resources, _ := spec["resources"].(map[string]interface{})
	resKey := strings.Join(resourcePath, ".")
	res, ok := resources[resKey].(map[string]interface{})
	if !ok {
		t.Fatalf("resource %q.%s not found", service, resKey)
	}
	methods, _ := res["methods"].(map[string]interface{})
	m, ok := methods[methodName].(map[string]interface{})
	if !ok {
		t.Fatalf("method %q.%s.%s not found", service, resKey, methodName)
	}
	return m
}
