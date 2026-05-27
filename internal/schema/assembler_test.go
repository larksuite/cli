// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/registry"
)

// TestMain isolates registry-backed tests from any host ~/.lark-cli cache so
// the suite gives the same answer on every machine. Without this, a stale
// local remote_meta.json could surface methods that aren't in the embedded
// snapshot (or alter their data) depending on the contributor's environment.
//
// Note: os.Exit skips deferred functions, so cleanup is done explicitly
// after m.Run before exiting.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "schema-test-cfg-*")
	if err != nil {
		// Surface the failure rather than silently running against the host
		// cache — that defeats the whole purpose of this isolation.
		println("schema test setup: MkdirTemp failed:", err.Error())
		os.Exit(2)
	}
	os.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	os.Setenv("LARKSUITE_CLI_REMOTE_META", "off") // never touch network
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func TestKeyOrderIndex_ImReactionsList(t *testing.T) {
	// We only assert key-set membership, not absolute order — the upstream
	// meta_data API does not guarantee a stable JSON key sequence across
	// fetches, so hard-coding the order makes CI flaky. Order preservation
	// from input to output is tested separately in TestBuildInputSchema_*.
	order := lookupKeyOrder("im", []string{"reactions"}, "list")
	if order == nil {
		t.Fatal("expected key order for im.reactions.list, got nil")
	}
	wantParams := map[string]bool{
		"message_id": true, "reaction_type": true, "page_token": true,
		"page_size": true, "user_id_type": true,
	}
	if got, want := len(order.Parameters), len(wantParams); got != want {
		t.Errorf("parameters count = %d, want %d (got %v)", got, want, order.Parameters)
	}
	for _, k := range order.Parameters {
		if !wantParams[k] {
			t.Errorf("unexpected parameter key %q", k)
		}
	}
	// im.reactions.list 是 GET，没有 requestBody
	if len(order.RequestBody) != 0 {
		t.Errorf("expected empty RequestBody, got %v", order.RequestBody)
	}
}

func TestKeyOrderIndex_ImImagesCreate(t *testing.T) {
	// Membership-only assertion; see comment on TestKeyOrderIndex_ImReactionsList.
	order := lookupKeyOrder("im", []string{"images"}, "create")
	if order == nil {
		t.Fatal("expected key order for im.images.create, got nil")
	}
	wantBody := map[string]bool{"image_type": true, "image": true}
	if got, want := len(order.RequestBody), len(wantBody); got != want {
		t.Errorf("requestBody count = %d, want %d (got %v)", got, want, order.RequestBody)
	}
	for _, k := range order.RequestBody {
		if !wantBody[k] {
			t.Errorf("unexpected requestBody key %q", k)
		}
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
	// string enums preserve source order (deduped), matching the `enum`
	// branch. Numeric/boolean enums would still be sorted by value.
	want := []interface{}{"banana", "apple"}
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
	want := []interface{}{"x", "y"} // pass through, no sort
	if !reflect.DeepEqual(got.Enum, want) {
		t.Errorf("Enum = %v, want %v", got.Enum, want)
	}
}

func TestConvertProperty_EnumIntegerCoerce(t *testing.T) {
	input := map[string]interface{}{
		"type": "integer",
		"options": []interface{}{
			map[string]interface{}{"value": "10"},
			map[string]interface{}{"value": "1"},
			map[string]interface{}{"value": "2"},
		},
	}
	got := convertProperty(input, "")
	want := []interface{}{int64(1), int64(2), int64(10)} // typed + numerically sorted
	if !reflect.DeepEqual(got.Enum, want) {
		t.Errorf("Enum = %v, want %v", got.Enum, want)
	}
}

func TestConvertProperty_ListTypeFallback(t *testing.T) {
	input := map[string]interface{}{
		"type":        "list",
		"description": "ids",
	}
	got := convertProperty(input, "")
	if got.Type != "array" {
		t.Errorf("Type = %q, want %q", got.Type, "array")
	}
	if got.Items == nil {
		t.Fatalf("Items = nil, want non-nil (any-schema fallback)")
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
	// top-level required: ["params"] because message_id is a required path param
	if !reflect.DeepEqual(is.Required, []string{"params"}) {
		t.Errorf("Required = %v, want [params]", is.Required)
	}
	// top-level properties only contains "params" (no body fields, no high-risk-write)
	if !reflect.DeepEqual(is.Properties.Order, []string{"params"}) {
		t.Errorf("top-level properties order = %v, want [params]", is.Properties.Order)
	}
	// params sub-object: required + property order
	params := is.Properties.Map["params"]
	if params.Type != "object" {
		t.Errorf("params.Type = %q, want \"object\"", params.Type)
	}
	if !reflect.DeepEqual(params.Required, []string{"message_id"}) {
		t.Errorf("params.Required = %v, want [message_id]", params.Required)
	}
	if !reflect.DeepEqual(params.Properties.Order, mko.Parameters) {
		t.Errorf("params.properties order = %v, want (from key index) %v",
			params.Properties.Order, mko.Parameters)
	}
}

func TestBuildInputSchema_ImagesCreate_FileAndBody(t *testing.T) {
	method := loadMethodFromRegistry(t, "im", []string{"images"}, "create")
	currentMethodOrder = lookupKeyOrder("im", []string{"images"}, "create")
	defer func() { currentMethodOrder = nil }()

	is := buildInputSchema(method)

	// top-level required: ["data", "file"] — image_type body required + image file required
	if !reflect.DeepEqual(is.Required, []string{"data", "file"}) {
		t.Errorf("Required = %v, want [data, file]", is.Required)
	}
	// top-level properties: data (for non-file body) + file (for binary upload)
	if !reflect.DeepEqual(is.Properties.Order, []string{"data", "file"}) {
		t.Errorf("top-level properties order = %v, want [data, file]", is.Properties.Order)
	}
	// data sub-object carries only non-file body fields (image_type)
	data := is.Properties.Map["data"]
	if !reflect.DeepEqual(data.Required, []string{"image_type"}) {
		t.Errorf("data.Required = %v, want [image_type]", data.Required)
	}
	if !reflect.DeepEqual(data.Properties.Order, []string{"image_type"}) {
		t.Errorf("data.properties order = %v, want [image_type]", data.Properties.Order)
	}
	if it := data.Properties.Map["image_type"]; !reflect.DeepEqual(it.Enum, []interface{}{"message", "avatar"}) {
		t.Errorf("image_type unexpected: %+v", it)
	}
	if _, isFile := data.Properties.Map["image"]; isFile {
		t.Errorf("image (file field) should NOT appear in data sub-object")
	}

	// file sub-object carries the binary upload field
	file := is.Properties.Map["file"]
	if file.Type != "object" {
		t.Errorf("file.Type = %q, want \"object\"", file.Type)
	}
	if !reflect.DeepEqual(file.Required, []string{"image"}) {
		t.Errorf("file.Required = %v, want [image]", file.Required)
	}
	if !reflect.DeepEqual(file.Properties.Order, []string{"image"}) {
		t.Errorf("file.properties order = %v, want [image]", file.Properties.Order)
	}
	img := file.Properties.Map["image"]
	if img.Type != "string" {
		t.Errorf("image.Type = %q, want \"string\"", img.Type)
	}
	if img.Format != "binary" {
		t.Errorf("image.Format = %q, want \"binary\"", img.Format)
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

	// yes lives at inputSchema.properties.yes (sibling of params/data)
	yes, ok := is.Properties.Map["yes"]
	if !ok {
		t.Fatal("expected top-level `yes` property in high-risk-write envelope, not found")
	}
	if yes.Type != "boolean" {
		t.Errorf("yes.Type = %q, want \"boolean\"", yes.Type)
	}
	if v, _ := yes.Default.(bool); v != false {
		t.Errorf("yes.Default = %v, want false", yes.Default)
	}
	// yes must NOT be in top-level required
	for _, r := range is.Required {
		if r == "yes" {
			t.Errorf("`yes` should not appear in top-level required")
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

func TestBuildOutputSchema_ReactionsList(t *testing.T) {
	method := loadMethodFromRegistry(t, "im", []string{"reactions"}, "list")
	mko := lookupKeyOrder("im", []string{"reactions"}, "list")
	currentMethodOrder = mko
	defer func() { currentMethodOrder = nil }()

	os := buildOutputSchema(method)

	if os.Type != "object" {
		t.Errorf("Type = %q, want \"object\"", os.Type)
	}
	// Top-level response: has_more, page_token, items
	if _, ok := os.Properties.Map["items"]; !ok {
		t.Fatal("items not found in outputSchema")
	}
	items := os.Properties.Map["items"]
	if items.Type != "array" {
		t.Errorf("items.Type = %q, want \"array\"", items.Type)
	}
	if items.Items == nil {
		t.Fatal("items.Items is nil (array unfold failed)")
	}
	if items.Items.Type != "object" {
		t.Errorf("items.Items.Type = %q, want \"object\"", items.Items.Type)
	}
}

func TestConvertAccessTokens(t *testing.T) {
	tests := []struct {
		name  string
		input []interface{}
		want  []string
	}{
		{"tenant only", []interface{}{"tenant"}, []string{"bot"}},
		{"user only", []interface{}{"user"}, []string{"user"}},
		{"tenant then user", []interface{}{"tenant", "user"}, []string{"bot", "user"}},
		{"user then tenant", []interface{}{"user", "tenant"}, []string{"bot", "user"}},
		{"deduped", []interface{}{"tenant", "tenant", "user"}, []string{"bot", "user"}},
		{"empty", []interface{}{}, []string{}},
		{"nil", nil, []string{}},
		{"unknown skipped", []interface{}{"user", "admin"}, []string{"user"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertAccessTokens(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildMeta_FullFields(t *testing.T) {
	// Synthesized method to avoid runtime variance from remote-cache overlay
	// (which strips `risk` from merged services). All other field semantics
	// match the real im.images.create entry in meta_data.json.
	method := map[string]interface{}{
		"risk":   "write",
		"danger": true,
		"scopes": []interface{}{
			"im:resource:upload",
			"im:resource",
		},
		"accessTokens": []interface{}{"tenant"},
		"docUrl":       "https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/reference/im-v1/image/create",
	}
	m := buildMeta(method)

	if m.EnvelopeVersion != "1.0" {
		t.Errorf("EnvelopeVersion = %q", m.EnvelopeVersion)
	}
	if m.Risk != "write" {
		t.Errorf("Risk = %q, want \"write\"", m.Risk)
	}
	if !m.Danger {
		t.Errorf("Danger = false, want true")
	}
	if !reflect.DeepEqual(m.AccessTokens, []string{"bot"}) {
		t.Errorf("AccessTokens = %v, want [bot]", m.AccessTokens)
	}
	if m.DocURL == "" {
		t.Errorf("DocURL should be present for im.images.create")
	}
	if !reflect.DeepEqual(m.Scopes, []string{"im:resource:upload", "im:resource"}) {
		t.Errorf("Scopes = %v, want [im:resource:upload, im:resource] (meta_data natural order)", m.Scopes)
	}
	if m.RequiredScopes == nil {
		t.Errorf("RequiredScopes should be empty slice, not nil")
	}
	if len(m.RequiredScopes) != 0 {
		t.Errorf("RequiredScopes should be empty for this method, got %v", m.RequiredScopes)
	}
	if m.Affordance != nil {
		t.Errorf("Affordance must be nil when method has no affordance field, got %+v", m.Affordance)
	}
}

func TestBuildMeta_MissingRiskDefaultsToRead(t *testing.T) {
	method := map[string]interface{}{
		"scopes":       []interface{}{"x"},
		"accessTokens": []interface{}{"user"},
		// no risk field
	}
	m := buildMeta(method)
	if m.Risk != "read" {
		t.Errorf("Risk = %q, want \"read\" (default for missing risk)", m.Risk)
	}
}

func TestBuildMeta_RequiredScopesPresent(t *testing.T) {
	method := loadMethodFromRegistry(t, "mail", []string{"user_mailbox", "messages"}, "get")
	m := buildMeta(method)
	if len(m.RequiredScopes) == 0 {
		t.Errorf("RequiredScopes should be non-empty for mail.user_mailbox.messages.get")
	}
}

func TestParseAffordance_NilOrEmpty(t *testing.T) {
	cases := []struct {
		name string
		raw  interface{}
	}{
		{"nil", nil},
		{"empty object", map[string]interface{}{}},
		{"all-five-empty-arrays", map[string]interface{}{
			"use_when":        []interface{}{},
			"do_not_use_when": []interface{}{},
			"prerequisites":   []interface{}{},
			"examples":        []interface{}{},
			"related":         []interface{}{},
		}},
		{"malformed (string)", "not an object"},
		{"malformed (number)", 42},
		{"malformed (nested type mismatch)", map[string]interface{}{
			"examples": "should be a list, not a string",
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseAffordance(c.raw); got != nil {
				t.Errorf("parseAffordance(%v) = %+v, want nil", c.raw, got)
			}
		})
	}
}

func TestParseAffordance_FullPopulated(t *testing.T) {
	raw := map[string]interface{}{
		"use_when":        []interface{}{"需要拿到当前用户的主日历 ID"},
		"do_not_use_when": []interface{}{"已知具体某一个非主日历的 calendar_id"},
		"prerequisites":   []interface{}{"user 身份登录"},
		"examples": []interface{}{
			map[string]interface{}{"description": "获取主日历", "command": "lark-cli calendar calendars primary"},
		},
		"related": []interface{}{"calendars.list"},
	}
	a := parseAffordance(raw)
	if a == nil {
		t.Fatal("parseAffordance returned nil, want populated")
	}
	if len(a.UseWhen) != 1 || a.UseWhen[0] != "需要拿到当前用户的主日历 ID" {
		t.Errorf("UseWhen = %v", a.UseWhen)
	}
	if len(a.Examples) != 1 || a.Examples[0].Description != "获取主日历" ||
		a.Examples[0].Command != "lark-cli calendar calendars primary" {
		t.Errorf("Examples = %+v", a.Examples)
	}
	if len(a.Related) != 1 || a.Related[0] != "calendars.list" {
		t.Errorf("Related = %v", a.Related)
	}
}

func TestBuildMeta_AffordanceFromMethod(t *testing.T) {
	method := map[string]interface{}{
		"scopes":       []interface{}{"x"},
		"accessTokens": []interface{}{"user"},
		"risk":         "read",
		"affordance": map[string]interface{}{
			"use_when": []interface{}{"trigger"},
		},
	}
	m := buildMeta(method)
	if m.Affordance == nil {
		t.Fatal("Affordance should be populated from method[\"affordance\"]")
	}
	if len(m.Affordance.UseWhen) != 1 || m.Affordance.UseWhen[0] != "trigger" {
		t.Errorf("UseWhen = %v", m.Affordance.UseWhen)
	}
}

func TestBuildMeta_MissingDocURLOmitted(t *testing.T) {
	method := map[string]interface{}{
		"scopes":       []interface{}{"x"},
		"accessTokens": []interface{}{"user"},
		"risk":         "read",
		// no docUrl
	}
	m := buildMeta(method)
	if m.DocURL != "" {
		t.Errorf("DocURL = %q, want empty (will be omitempty)", m.DocURL)
	}
	// Verify JSON serialization omits doc_url
	b, _ := json.Marshal(m)
	if strings.Contains(string(b), "doc_url") {
		t.Errorf("doc_url should be omitted from JSON, got: %s", b)
	}
}

func TestBuildOutputSchema_EmptyResponseBody(t *testing.T) {
	// 装配器对空 responseBody 应生成 properties = {} （不 nil）
	method := map[string]interface{}{}
	currentMethodOrder = nil
	os := buildOutputSchema(method)
	if os.Type != "object" {
		t.Errorf("Type = %q, want \"object\"", os.Type)
	}
	if os.Properties == nil {
		t.Fatal("Properties is nil, want empty OrderedProps")
	}
	if len(os.Properties.Order) != 0 {
		t.Errorf("Properties.Order should be empty, got %v", os.Properties.Order)
	}
}

func TestAssembleEnvelope_ReactionsList_FullStructure(t *testing.T) {
	method := loadMethodFromRegistry(t, "im", []string{"reactions"}, "list")
	env := AssembleEnvelope("im", []string{"reactions"}, "list", method)

	if env.Name != "im reactions list" {
		t.Errorf("Name = %q, want \"im reactions list\"", env.Name)
	}
	if env.Description == "" {
		t.Errorf("Description should not be empty for im.reactions.list")
	}
	if env.InputSchema == nil || env.OutputSchema == nil || env.Meta == nil {
		t.Fatal("InputSchema/OutputSchema/Meta must all be non-nil")
	}
	if env.Meta.EnvelopeVersion != "1.0" {
		t.Errorf("Meta.EnvelopeVersion = %q", env.Meta.EnvelopeVersion)
	}
}

func TestAssembleEnvelope_NestedResource_NameJoinedWithSpaces(t *testing.T) {
	// im.chat.members.create — resource path is one element "chat.members" with
	// an internal dot. Substituted from plan's `bots` because remote-cache
	// overlay strips `bots` from the loaded method map on this environment;
	// the assertion is about name joining, not method specifics.
	method := loadMethodFromRegistry(t, "im", []string{"chat.members"}, "create")
	env := AssembleEnvelope("im", []string{"chat.members"}, "create", method)
	// chat.members resourcePath stays as one element in the slice with a dot;
	// name should split it to "im chat.members create" — we keep the dot as-is
	// inside the resource segment to round-trip with completion logic.
	if env.Name != "im chat.members create" {
		t.Errorf("Name = %q, want \"im chat.members create\"", env.Name)
	}
}

func TestAssembleEnvelope_JSONIsStable(t *testing.T) {
	// Assemble twice; JSON output must be byte-identical (determinism).
	method := loadMethodFromRegistry(t, "im", []string{"reactions"}, "list")
	a := AssembleEnvelope("im", []string{"reactions"}, "list", method)
	b := AssembleEnvelope("im", []string{"reactions"}, "list", method)
	ja, _ := json.MarshalIndent(a, "", "  ")
	jb, _ := json.MarshalIndent(b, "", "  ")
	if string(ja) != string(jb) {
		t.Errorf("envelope assembly is non-deterministic:\nfirst:\n%s\nsecond:\n%s", ja, jb)
	}
}

func TestAssembleService_Im(t *testing.T) {
	spec := registry.LoadFromMeta("im")
	envs := AssembleService("im", spec, nil)
	if len(envs) == 0 {
		t.Fatal("expected non-empty envelopes for service im")
	}
	// Every envelope.Name starts with "im "
	for _, e := range envs {
		if !strings.HasPrefix(e.Name, "im ") {
			t.Errorf("envelope name %q does not start with \"im \"", e.Name)
		}
	}
	// Sorted by name
	for i := 1; i < len(envs); i++ {
		if envs[i-1].Name > envs[i].Name {
			t.Errorf("envelopes not sorted by name at idx %d: %q > %q", i, envs[i-1].Name, envs[i].Name)
		}
	}
}

func TestAssembleService_FilterByAccessToken(t *testing.T) {
	spec := registry.LoadFromMeta("im")
	// Filter to bot-only (--as bot, which corresponds to "tenant")
	envs := AssembleService("im", spec, func(method map[string]interface{}) bool {
		tokens, _ := method["accessTokens"].([]interface{})
		for _, t := range tokens {
			if s, _ := t.(string); s == "tenant" {
				return true
			}
		}
		return false
	})
	// Every envelope's _meta.access_tokens must contain "bot"
	for _, e := range envs {
		found := false
		for _, t := range e.Meta.AccessTokens {
			if t == "bot" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("envelope %q does not declare bot access", e.Name)
		}
	}
}

func TestAssembleAll_AtLeast193(t *testing.T) {
	envs := AssembleAll(nil)
	// Envelope assembly is overlay-independent (Task 17b): AssembleAll walks the
	// embedded meta_data.json directly, so the count is stable across machines.
	if len(envs) < 193 {
		t.Errorf("AssembleAll returned %d envelopes, expected >= 193", len(envs))
	}
	// Spot check: im reactions list should be present
	found := false
	for _, e := range envs {
		if e.Name == "im reactions list" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("im reactions list not found in AssembleAll output")
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
