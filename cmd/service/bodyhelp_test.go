// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/meta"
	"github.com/spf13/cobra"
)

// methodCmdWithBodyForTest registers a method that declares body fields and
// returns its command, so the lazy help rebuild can be exercised end to end.
func methodCmdWithBodyForTest(t *testing.T) *cobra.Command {
	t.Helper()
	parent := &cobra.Command{Use: "root"}
	svc := meta.ServiceFromMap(map[string]interface{}{
		"name":        "im",
		"description": "Messaging API",
		"servicePath": "/open-apis/im/v1",
		"resources": map[string]interface{}{
			"messages": map[string]interface{}{
				"methods": map[string]interface{}{
					"create": map[string]interface{}{
						"description": "发送消息",
						"httpMethod":  "POST",
						"requestBody": map[string]interface{}{
							"receive_id": map[string]interface{}{
								"type": "string", "required": true, "description": "消息接收者的 ID",
							},
							"content": map[string]interface{}{
								"type": "string", "required": true, "description": "消息内容",
							},
						},
					},
				},
			},
		},
	})
	registerService(parent, svc, &cmdutil.Factory{})

	cmd, _, err := parent.Find([]string{"im", "messages", "create"})
	if err != nil {
		t.Fatalf("method command not registered: %v", err)
	}
	return cmd
}

func TestBodyHelp_RendersSkeletonAndFacts(t *testing.T) {
	fields := []meta.Field{
		{Name: "receive_id", Type: "string", Required: true, Description: "消息接收者的 ID。Identity: tail"},
		{Name: "content", Type: "string", Required: true, Description: "消息内容"},
		{Name: "uuid", Type: "string", Description: "幂等 uuid"},
	}
	got := bodyHelp(fields)
	for _, want := range []string{
		"Request body (--data",
		`"receive_id"`,
		"required",
		"消息接收者的 ID", // first sentence only — the Identity tail must be cut
	} {
		if !strings.Contains(got, want) {
			t.Errorf("bodyHelp must contain %q, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Identity:") {
		t.Error("descriptions must be first-sentence only")
	}
	if !strings.Contains(got, "optional") {
		t.Error("a non-required field must be marked optional")
	}
}

func TestBodyHelp_EmptyForNoBody(t *testing.T) {
	if got := bodyHelp(nil); got != "" {
		t.Errorf("no body fields must render nothing, got %q", got)
	}
}

func TestBodyHelp_NestedFieldsOneLevel(t *testing.T) {
	fields := []meta.Field{{
		Name: "filter", Type: "object", Description: "过滤条件",
		Properties: map[string]meta.Field{
			"user_ids": {Name: "user_ids", Type: "array", Description: "用户 ID 列表"},
		},
	}}
	got := bodyHelp(fields)
	if !strings.Contains(got, "user_ids") {
		t.Errorf("nested fields must be listed, got:\n%s", got)
	}
}

// A second-level field with children of its own is where this contract goes
// quiet: the skeleton writes {} for it, which is what a genuinely empty object
// gets too, and no line below names what it holds. Reporting the count is what
// lets a caller tell the two apart — without it, "consult the schema only for
// deep nesting" cannot be acted on, because spotting the nesting is exactly
// what is missing.
func TestBodyHelp_MarksFieldsWhoseChildrenAreNotShown(t *testing.T) {
	fields := []meta.Field{{
		Name: "task", Type: "object", Description: "任务",
		Properties: map[string]meta.Field{
			"due": {
				Name: "due", Type: "object", Description: "截止时间",
				Properties: map[string]meta.Field{
					"timestamp":  {Name: "timestamp", Type: "string"},
					"is_all_day": {Name: "is_all_day", Type: "boolean"},
				},
			},
			"summary": {Name: "summary", Type: "string", Description: "标题"},
		},
	}}
	got := bodyHelp(fields)

	if !strings.Contains(got, "nested: 2 fields not shown") {
		t.Errorf("a truncated object must report what it hides, got:\n%s", got)
	}
	// summary has no children, and task's own children do get rendered, so
	// neither may claim to be hiding anything.
	for _, line := range strings.Split(got, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "summary ") && !strings.HasPrefix(trimmed, "task ") {
			continue
		}
		if strings.Contains(line, "nested:") {
			t.Errorf("field with nothing hidden must not be marked: %q", line)
		}
	}
}

// One hidden field reads "1 field", not "1 fields".
func TestBodyHelp_NestedCountIsSingularForOne(t *testing.T) {
	fields := []meta.Field{{
		Name: "wrapper", Type: "object",
		Properties: map[string]meta.Field{
			"rules": {
				Name: "rules", Type: "object",
				Properties: map[string]meta.Field{"only": {Name: "only", Type: "string"}},
			},
		},
	}}
	if got := bodyHelp(fields); !strings.Contains(got, "nested: 1 field not shown") {
		t.Errorf("a single hidden field must not read as plural, got:\n%s", got)
	}
}

// The skeleton is meant to be copied straight into --data, so it has to parse.
func TestBodyHelp_SkeletonIsValidJSON(t *testing.T) {
	fields := []meta.Field{
		{Name: "receive_id", Type: "string", Required: true},
		{Name: "count", Type: "int", Required: false},
		{Name: "enabled", Type: "boolean"},
		{Name: "items", Type: "array"},
		{Name: "filter", Type: "object", Properties: map[string]meta.Field{
			"user_ids": {Name: "user_ids", Type: "array"},
		}},
	}
	skeleton := bodySkeleton(fields)
	var into map[string]any
	if err := json.Unmarshal([]byte(skeleton), &into); err != nil {
		t.Fatalf("skeleton is not valid JSON: %v\n%s", err, skeleton)
	}
	for _, key := range []string{"receive_id", "count", "enabled", "items", "filter"} {
		if _, ok := into[key]; !ok {
			t.Errorf("skeleton missing field %q: %s", key, skeleton)
		}
	}
}

// Field names reach the skeleton from upstream metadata, so a name carrying a
// quote or control character must not be able to break the JSON shape.
func TestBodyHelp_SkeletonEscapesFieldNames(t *testing.T) {
	skeleton := bodySkeleton([]meta.Field{{Name: `we"ird` + "\n", Type: "string"}})
	var into map[string]any
	if err := json.Unmarshal([]byte(skeleton), &into); err != nil {
		t.Fatalf("skeleton with a quoted field name must stay valid JSON: %v\n%s", err, skeleton)
	}
}

// The facts line renders name, type and description from the same upstream
// document; cleaning only the description would leave the row forgeable through
// the other two.
func TestBodyHelp_SanitizesNameAndType(t *testing.T) {
	got := bodyHelp([]meta.Field{{
		Name: "na\x1b[31mme", Type: "str\x1b[0ming", Description: "desc",
	}})
	if strings.Contains(got, "\x1b") {
		t.Errorf("facts line must not carry escape sequences, got %q", got)
	}
	if !strings.Contains(got, "name") {
		t.Errorf("sanitized name must survive, got %q", got)
	}
}

// The skeleton and the facts line below it name the same fields, so they have to
// sanitize identically. json.Marshal alone escapes C0 and quotes but passes bidi
// controls and zero-width characters, which would make the two lines disagree.
func TestBodyHelp_SkeletonSanitizesInvisibleChars(t *testing.T) {
	for name, raw := range map[string]string{
		"bidi override":    "user_id‮",
		"zero width":       "user​id",
		"C1 control":       "userid",
		"arabic letter mk": "user؜id",
	} {
		got := bodyHelp([]meta.Field{{Name: raw, Type: "string"}})
		for _, bad := range []string{"‮", "​", "", "؜"} {
			if strings.Contains(got, bad) {
				t.Errorf("%s: output must not carry %q, got %q", name, bad, got)
			}
		}
	}
	// Two fields differing only by an invisible character must not render as two
	// identical skeleton keys — sanitizing collapses them, so the shape stays
	// honest about there being two distinct fields only if both are cleaned the
	// same way.
	same := bodySkeleton([]meta.Field{{Name: "user_id", Type: "string"}})
	withBidi := bodySkeleton([]meta.Field{{Name: "user_id‮", Type: "string"}})
	if same != withBidi {
		t.Errorf("skeleton must sanitize the key: %q vs %q", same, withBidi)
	}
}

// bodyHelp has to survive PrepareMethodHelp's lazy Long rebuild — that path
// recomposes from annotations, so a body section only written at build time
// would vanish the moment help is actually rendered.
func TestPrepareMethodHelp_KeepsBodyContract(t *testing.T) {
	cmd := methodCmdWithBodyForTest(t)
	if !overlayRenderer(t, nil, nil, nil).PrepareMethodHelp(cmd) {
		t.Fatal("PrepareMethodHelp must apply to a method command")
	}
	if !strings.Contains(cmd.Long, "Request body (--data") {
		t.Errorf("rebuilt Long must keep the request body contract, got:\n%s", cmd.Long)
	}
	if !strings.Contains(cmd.Long, "Full parameter schema") {
		t.Error("rebuilt Long must keep the schema pointer")
	}
}

// The metadata wraps an array's element schema in "properties", so an array of
// objects must render as [{…}]. Emitting ["<item>"] shows a string array, and a
// caller copying that builds a body the API rejects — with no local signal,
// since --dry-run does not validate body structure.
func TestBodyHelp_SkeletonRendersObjectArrays(t *testing.T) {
	fields := []meta.Field{{
		Name: "members", Type: "array", Description: "任务成员列表",
		Properties: map[string]meta.Field{
			"id":   {Name: "id", Type: "string", Required: true},
			"role": {Name: "role", Type: "string", Required: true},
		},
	}}
	skeleton := bodySkeleton(fields)
	if strings.Contains(skeleton, `["<item>"]`) {
		t.Errorf("an array of objects must not render as a string array: %s", skeleton)
	}
	for _, want := range []string{`"id"`, `"role"`} {
		if !strings.Contains(skeleton, want) {
			t.Errorf("skeleton must name the element field %s: %s", want, skeleton)
		}
	}
	// Still has to be copyable into --data.
	var into map[string][]map[string]any
	if err := json.Unmarshal([]byte(skeleton), &into); err != nil {
		t.Fatalf("object-array skeleton is not valid JSON: %v\n%s", err, skeleton)
	}
	if len(into["members"]) != 1 {
		t.Errorf("members must render exactly one sample element: %s", skeleton)
	}
}

// An array whose element shape the metadata does not describe keeps the neutral
// placeholder rather than inventing a structure.
func TestBodyHelp_SkeletonKeepsPlaceholderForUntypedArray(t *testing.T) {
	skeleton := bodySkeleton([]meta.Field{{Name: "tags", Type: "array"}})
	if !strings.Contains(skeleton, `["<item>"]`) {
		t.Errorf("an array with no element schema keeps the placeholder, got %s", skeleton)
	}
}

// Running out of nesting budget must not change what an element's *type* looks
// like. This is the shape that regressed once: the same field renders correctly
// at the top level but is nested one deeper in a sibling method (task create vs
// task patch, which wraps the body in "task"), and collapsing both stop
// conditions to ["<item>"] silently turned an object array into a string array
// only in the nested case.
func TestBodyHelp_NestedObjectArrayDoesNotClaimScalarElements(t *testing.T) {
	objArray := meta.Field{
		Name: "custom_fields", Type: "array",
		Properties: map[string]meta.Field{
			"guid": {Name: "guid", Type: "string"},
		},
	}
	nested := bodySkeleton([]meta.Field{{
		Name: "task", Type: "object",
		Properties: map[string]meta.Field{"custom_fields": objArray},
	}})
	if strings.Contains(nested, `"custom_fields": ["<item>"]`) {
		t.Errorf("a nested object array must not render as a string array: %s", nested)
	}
	if !strings.Contains(nested, `"custom_fields": [{}]`) {
		t.Errorf("past the nesting budget an object array degrades to [{}], got: %s", nested)
	}
	// A genuinely scalar array keeps the item placeholder at any depth.
	scalarNested := bodySkeleton([]meta.Field{{
		Name: "task", Type: "object",
		Properties: map[string]meta.Field{
			"tags": {Name: "tags", Type: "array"},
		},
	}})
	if !strings.Contains(scalarNested, `"tags": ["<item>"]`) {
		t.Errorf("a scalar array keeps the placeholder when nested, got: %s", scalarNested)
	}
	// Both shapes must still be copyable into --data.
	for _, s := range []string{nested, scalarNested} {
		var into map[string]any
		if err := json.Unmarshal([]byte(s), &into); err != nil {
			t.Fatalf("skeleton is not valid JSON: %v\n%s", err, s)
		}
	}
}

// A facts line has to be enough to construct the call with. Naming the field and
// its type is not: "参与人权限" does not say which four strings are accepted, and
// "智能体任务状态" hides the 1-4 range entirely — both were invisible in body help
// while the same facts already rendered on the param-flag side.
func TestBodyHelp_FactsCarryEnumAndBounds(t *testing.T) {
	fields := []meta.Field{
		{Name: "attendee_ability", Type: "string", Description: "参与人权限",
			Enum: []any{"none", "can_modify_event"},
			Options: []meta.Option{
				{Value: "none", Description: "无法编辑日程"},
				{Value: "can_modify_event", Description: "可以编辑日程"},
			}},
		{Name: "agent_task_status", Type: "integer", Description: "智能体任务状态", Min: "1", Max: "4"},
		{Name: "page_size", Type: "integer", Description: "每页数量", Max: "100"},
		{Name: "offset", Type: "integer", Description: "偏移量", Min: "0"},
		{Name: "summary", Type: "string", Description: "任务标题"},
	}
	got := bodyHelp(fields)
	for _, want := range []string{
		"enum: none=无法编辑日程|can_modify_event=可以编辑日程",
		"min: 1, max: 4",
		"max: 100",
		"min: 0",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("facts must contain %q, got:\n%s", want, got)
		}
	}
	// A field declaring neither gains nothing — the clauses are conditional.
	summary := factsLineFor(t, got, "summary")
	if strings.Contains(summary, "enum:") || strings.Contains(summary, "min:") {
		t.Errorf("a field with no enum/bounds must gain no clause, got %q", summary)
	}
}

// Enum values and their meanings are upstream text reaching a consumer's
// context, exactly like the name/type/description already sanitized on this
// line. Rendering them raw would let one field's row redraw or reorder the
// listing around it.
func TestBodyHelp_FactsSanitizeEnumText(t *testing.T) {
	got := bodyHelp([]meta.Field{{
		Name: "mode", Type: "string", Description: "模式",
		Enum: []any{"a‮b"},
		Options: []meta.Option{
			{Value: "a‮b", Description: "\x1b[31mred\x1b[0m​ tail"},
		},
	}})
	for _, bad := range []string{"\x1b", "‮", "​"} {
		if strings.Contains(got, bad) {
			t.Errorf("enum clause must not echo %q, got:\n%s", bad, got)
		}
	}
	if !strings.Contains(got, "enum: ") {
		t.Errorf("sanitizing must not drop the clause itself, got:\n%s", got)
	}
	// An optional field's allowed value never reaches the skeleton, so there is
	// nothing there to leak — this half holds structurally, not by cleaning.
	if !strings.Contains(got, `{"mode": "<string>"}`) {
		t.Errorf("an optional enum must not reach the skeleton, got:\n%s", got)
	}
}

// The body line keeps FirstSentence, not the param side's sanitizeFieldDesc:
// the latter cuts at `；`/`;`, which would truncate a description that spells its
// alternatives out inline — mode's own values live after the semicolon.
func TestBodyHelp_FactsKeepDescriptionPastSemicolon(t *testing.T) {
	got := bodyHelp([]meta.Field{
		{Name: "mode", Type: "integer", Description: "任务完成模式, 1 - 会签任务; 2 - 或签任务", Min: "1", Max: "2"},
	})
	if !strings.Contains(got, "2 - 或签任务") {
		t.Errorf("description must survive past the semicolon, got:\n%s", got)
	}
}

// The skeleton is copied verbatim and --dry-run does not validate the body, so a
// placeholder must not assert a value the API rejects. `0` did: it is out of
// range wherever min > 0, and outright illegal where the only allowed value is
// something else.
func TestBodyHelp_SkeletonPlaceholdersAreLegalValues(t *testing.T) {
	cases := []struct {
		name  string
		field meta.Field
		want  string
	}{
		// Required: the caller cannot omit it, so an allowed value is a head start.
		{"required string enum takes its first allowed value",
			meta.Field{Name: "obj_type", Type: "string", Required: true, Enum: []any{"doc", "sheet"}},
			`{"obj_type": "doc"}`},
		{"required numeric enum likewise",
			meta.Field{Name: "to_entity_type", Type: "integer", Required: true, Enum: []any{"2"}},
			`{"to_entity_type": 2}`},
		{"required integer takes a floor that puts 0 out of range",
			meta.Field{Name: "event_type", Type: "integer", Required: true, Min: "1", Max: "1"},
			`{"event_type": 1}`},
		// Optional: the caller never asked to set it, so the skeleton must not read
		// as though they chose the value. The facts line carries what is allowed.
		{"optional string enum keeps its type marker",
			meta.Field{Name: "share_entity", Type: "string", Enum: []any{"anyone", "same_tenant"}},
			`{"share_entity": "<string>"}`},
		{"optional numeric enum keeps zero",
			meta.Field{Name: "approval_method", Type: "integer", Enum: []any{"1", "2", "3"}},
			`{"approval_method": 0}`},
		// The one place the two rules conflict: 0 is out of range here, and rule two
		// still wins. A rejection beats a silent write the caller did not ask for,
		// and the facts line states the range either way.
		{"optional integer keeps zero even below its floor",
			meta.Field{Name: "agent_task_status", Type: "integer", Min: "1", Max: "4"},
			`{"agent_task_status": 0}`},
		{"no enum and no floor keeps zero",
			meta.Field{Name: "relative_fire_minute", Type: "integer"},
			`{"relative_fire_minute": 0}`},
		// A floor only displaces 0 when 0 is below it. Reaching for the floor
		// regardless turns okr indicators patch's -99999999999 into the suggested
		// value for a field whose upstream example is plain 0 — legal, and absurd.
		{"a required field's negative floor leaves zero alone",
			meta.Field{Name: "current_value", Type: "number", Required: true, Min: "-99999999999", Max: "99999999999"},
			`{"current_value": 0}`},
		{"a required string's length bound is not a value",
			meta.Field{Name: "summary", Type: "string", Required: true, Min: "1"},
			`{"summary": "<string>"}`},
		{"a string with no enum keeps its type marker",
			meta.Field{Name: "summary", Type: "string", Min: "1"},
			`{"summary": "<string>"}`},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := bodySkeleton([]meta.Field{tt.field})
			if got != tt.want {
				t.Errorf("skeleton = %s, want %s", got, tt.want)
			}
			var into map[string]any
			if err := json.Unmarshal([]byte(got), &into); err != nil {
				t.Fatalf("skeleton is not valid JSON: %v\n%s", err, got)
			}
		})
	}
}

// The skeleton's one hard promise is that it parses, and that promise must not
// rest on upstream data happening to be benign. Every input below is reachable
// through the metadata contract: min/max and enum values arrive as strings, and
// meta.coerceLiteral reaches "number" through strconv.ParseFloat, which accepts
// "Inf" and "NaN" — strconv would render those `+Inf`, which is not JSON.
func TestBodyHelp_SkeletonRejectsUnrenderablePlaceholders(t *testing.T) {
	cases := []struct {
		name, want string
		field      meta.Field
	}{
		{"non-finite floor is not a JSON number", `{"t": 0}`,
			meta.Field{Name: "t", Type: "integer", Required: true, Min: "Inf"}},
		{"NaN floor likewise", `{"t": 0}`,
			meta.Field{Name: "t", Type: "number", Required: true, Min: "NaN"}},
		{"a non-finite allowed value is refused the same way", `{"t": 0}`,
			meta.Field{Name: "t", Type: "number", Required: true, Enum: []any{"Inf"}}},
		// The text guard, reachable through a required string enum: cleaning the
		// value would emit something the API no longer accepts, so it is refused
		// and the field falls back to its marker.
		{"an allowed value carrying a bidi override is refused", `{"t": "<string>"}`,
			meta.Field{Name: "t", Type: "string", Required: true, Enum: []any{"a‮b"}}},
		// meta.coerceLiteral passes a string-typed field's allowed values through
		// untouched, so a composite can arrive where a scalar belongs. Rendering it
		// would replace the field's shape instead of filling it in.
		{"an object allowed value cannot stand in for a string", `{"t": "<string>"}`,
			meta.Field{Name: "t", Type: "string", Required: true, Enum: []any{map[string]any{"k": "v"}}}},
		{"a list allowed value likewise", `{"t": "<string>"}`,
			meta.Field{Name: "t", Type: "string", Required: true, Enum: []any{[]any{"v"}}}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := bodySkeleton([]meta.Field{tt.field})
			if got != tt.want {
				t.Errorf("skeleton = %s, want %s", got, tt.want)
			}
			if !json.Valid([]byte(got)) {
				t.Errorf("skeleton must always parse, got %s", got)
			}
		})
	}
}

// An enum cannot stand in for a shape: substituting a scalar would tell the
// caller to send a string where the API wants an object or a list. Objects and
// arrays return before the scalar rules are reached, so this holds structurally —
// which is the point. It fails the moment someone hoists the EnumOptions lookup
// above the type switch, where it was and where it does reach both shapes. Both
// fields are required, which is the case that would fill a scalar in.
func TestBodyHelp_SkeletonKeepsShapeOverEnum(t *testing.T) {
	got := bodySkeleton([]meta.Field{
		{Name: "setting", Type: "object", Required: true, Enum: []any{"x"},
			Properties: map[string]meta.Field{"id": {Name: "id", Type: "string"}}},
		{Name: "tags", Type: "array", Required: true, Enum: []any{"y"}},
	})
	if !strings.Contains(got, `"setting": {"id": "<string>"}`) {
		t.Errorf("an object must keep its shape, got %s", got)
	}
	if !strings.Contains(got, `"tags": ["<item>"]`) {
		t.Errorf("an array must keep its shape, got %s", got)
	}
}

// factsLineFor returns the Fields line for a top-level field name.
func factsLineFor(t *testing.T, help, name string) string {
	t.Helper()
	for _, l := range strings.Split(help, "\n") {
		if strings.HasPrefix(l, "    "+name+"  (") {
			return l
		}
	}
	t.Fatalf("no facts line for %q in:\n%s", name, help)
	return ""
}
