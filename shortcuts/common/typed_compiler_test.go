// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"
)

type CompilerInlineArgs struct {
	Labels []string `flag:"labels" schema:"optional" cli:"encoding=repeated" doc:"labels to attach"`
}

type compilerPayload struct {
	Mode string `json:"mode" schema:"required;enum=fast|full" doc:"execution mode"`
}

type compilerCustomJSON struct {
	Value string `json:"value"`
}

func (value compilerCustomJSON) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{"value": value.Value})
}

type compilerArgs struct {
	Token              string          `flag:"token" schema:"required;minLength=1" doc:"target token"`
	Limit              Provided[int]   `flag:"limit" schema:"optional;default=20;minimum=1;maximum=100" doc:"maximum results"`
	Payload            compilerPayload `flag:"payload" schema:"optional;nonnullable" cli:"sources=flag|file|stdin;encoding=json" doc:"request payload"`
	CompilerInlineArgs `arg:"inline"`
	NormalizedToken    string `arg:"local"`
}

type compilerItem struct {
	ID    string `json:"id" schema:"required" doc:"item identity"`
	State string `json:"state" schema:"required;enum=ok|failed" doc:"item state"`
}

type compilerData struct {
	Title string         `json:"title" schema:"required" doc:"result title"`
	Items []compilerItem `json:"items" schema:"required;nonnullable" doc:"processed items"`
	Next  *string        `json:"next,omitempty" schema:"optional;nullable" doc:"next page token"`
}

func validCompilerDefinition() typedDefinition[compilerArgs, compilerData] {
	return typedDefinition[compilerArgs, compilerData]{
		Metadata: CommandMetadata{
			Service: "fixture", Command: "+compile", Description: "Compile a fixture", Risk: RiskWrite,
			Authorization: AuthorizationDefinition{Identities: map[Identity]IdentityAuthorization{
				IdentityUser: {
					RequiredScopes:    []string{"fixture:write"},
					ConditionalScopes: []ConditionalScope{{Scopes: []string{"fixture:read"}, When: "--payload selects the read path", Params: []string{"payload"}}},
				},
			}},
		},
		Input: InputDefinition{
			Fields:    []InputField{{Name: "token", CLI: CLIInput{Aliases: []FlagAlias{{Name: "legacy-token", Mode: AliasIndependent, Conflict: AliasTrimmedEqualOrError, Hidden: true}}}}},
			Relations: []Relation{{Kind: RelationRequires, Params: []string{"payload", "token"}, Presence: PresenceExplicit, Stage: StageSourcePreRun}},
		},
		Output: OutputDefinition{
			Outcomes: OutcomeDefinition{PartialFailure: &PartialFailureDefinition{
				ExitCode:    3,
				FailedItems: &FailedItemDefinition{ItemsPath: "/items", IdentityPaths: []string{"/id"}, StatePath: "/state", FailedValues: []JSONValue{"failed"}},
			}},
			Mode: OutputFixedJSON,
		},
		Hooks: typedHooks[compilerArgs, compilerData]{Execute: func(context.Context, CommandContext, *compilerArgs) (Result[compilerData], error) {
			return Success(compilerData{}), nil
		}},
	}
}

func TestDefineCompilesTypedContract(t *testing.T) {
	shortcut := defineTypedShortcut(validCompilerDefinition())
	if shortcut.typed == nil {
		t.Fatal("defineTypedShortcut() did not attach compiled contract")
	}
	if shortcut.Service != "fixture" || shortcut.Command != "+compile" || shortcut.Risk != "write" {
		t.Fatalf("Shortcut metadata = %#v", shortcut)
	}
	if got, want := shortcut.AuthTypes, []string{"user"}; !equalStrings(got, want) {
		t.Fatalf("AuthTypes = %v, want %v", got, want)
	}
	if got, want := shortcut.UserScopes, []string{"fixture:write"}; !equalStrings(got, want) {
		t.Fatalf("UserScopes = %v, want %v", got, want)
	}
	if got, want := shortcut.ConditionalUserScopes, []string{"fixture:read"}; !equalStrings(got, want) {
		t.Fatalf("ConditionalUserScopes = %v, want %v", got, want)
	}
	conditional := shortcut.typed.metadata.Authorization.Identities[IdentityUser].ConditionalScopes[0]
	if conditional.Requirement != ScopeRequired || conditional.When == "" || !equalStrings(conditional.Params, []string{"payload"}) {
		t.Fatalf("normalized conditional scope = %#v", conditional)
	}
	if got, want := len(shortcut.typed.fields), 4; got != want {
		t.Fatalf("compiled fields = %d, want %d", got, want)
	}
	limit := shortcut.typed.fields[shortcut.typed.fieldByName["limit"]]
	if !limit.provided || !limit.defaultValue.Set || limit.defaultValue.Value != float64(20) {
		t.Fatalf("compiled limit = %#v", limit)
	}
	payload := shortcut.typed.fields[shortcut.typed.fieldByName["payload"]]
	if payload.cli.Encoding != EncodingJSON || len(payload.cli.ValueSources) != 3 {
		t.Fatalf("compiled payload CLI = %#v", payload.cli)
	}
	if got := shortcut.typed.hooks.newArgs(); got == nil {
		t.Fatal("newArgs() = nil")
	}
	if _, ok := shortcut.typed.dataShape.(ObjectShape); !ok {
		t.Fatalf("data shape = %T, want ObjectShape", shortcut.typed.dataShape)
	}
}

func TestValueShapeClosedSet(t *testing.T) {
	shapes := []ValueShape{
		StringShape{},
		BooleanShape{},
		IntegerShape{},
		NumberShape{},
		NullShape{},
		ConstShape{},
		ArrayShape{},
		ObjectShape{},
		OneOfShape{},
		anyJSONShape{},
	}
	for _, shape := range shapes {
		shape.valueShape()
	}
}

func TestDefineClonesTipsAndRejectsBlankTips(t *testing.T) {
	definition := validCompilerDefinition()
	definition.Metadata.Tips = []string{"first tip", " second tip "}
	shortcut := defineTypedShortcut(definition)
	definition.Metadata.Tips[0] = "mutated"
	want := []string{"first tip", " second tip "}
	if got := shortcut.Tips; !reflect.DeepEqual(got, want) {
		t.Fatalf("Shortcut.Tips = %#v, want %#v", got, want)
	}
	if got := shortcut.typed.metadata.Tips; !reflect.DeepEqual(got, want) {
		t.Fatalf("compiled Metadata.Tips = %#v, want %#v", got, want)
	}

	definition = validCompilerDefinition()
	definition.Metadata.Tips = []string{" \t "}
	if _, err := compileDefinition(definition); err == nil || !strings.Contains(err.Error(), "Metadata.Tips[0]") {
		t.Fatalf("compileDefinition() error = %v, want blank Tip rejection", err)
	}
}

func TestCompiledTypedSchemaContract(t *testing.T) {
	definition := validCompilerDefinition()
	definition.Output.Meta = ResultMetaDefinition{Count: true, Pagination: true}
	contract := defineTypedShortcut(definition).typed.contract
	if contract.Name != "fixture +compile" || contract.InputSchema.Type != "object" || contract.OutputSchema.Type != "object" {
		t.Fatalf("contract identity or root shapes = %#v", contract)
	}
	if contract.InputSchema.Required == nil || !equalStrings(*contract.InputSchema.Required, []string{"token"}) {
		t.Fatalf("required inputs = %#v", contract.InputSchema.Required)
	}
	if len(contract.InputSchema.Properties) != 4 {
		t.Fatalf("input properties = %#v", contract.InputSchema.Properties)
	}
	token := contract.InputSchema.Properties["token"]
	if token.Flag != "--token" || token.Aliases == nil || len(*token.Aliases) != 1 || (*token.Aliases)[0].Flag != "--legacy-token" {
		t.Fatalf("token contract = %#v", token)
	}
	if contract.Meta.EnvelopeVersion != "1.0" || contract.Meta.Risk != RiskWrite || !equalStrings(contract.Meta.AccessTokens, []string{"user"}) {
		t.Fatalf("contract metadata = %#v", contract.Meta)
	}
	conditional := contract.Meta.Authorization.Identities[IdentityUser].ConditionalScopes[0]
	if conditional.When != "--payload selects the read path" || conditional.Requirement != ScopeRequired || !equalStrings(conditional.Params, []string{"payload"}) {
		t.Fatalf("conditional authorization contract = %#v", conditional)
	}
	if !contract.Meta.Outcomes.PartialFailure.Supported || contract.Meta.Outcomes.PartialFailure.ExitCode != 3 {
		t.Fatalf("partial outcome = %#v", contract.Meta.Outcomes.PartialFailure)
	}
	if contract.Meta.ResultMeta == nil {
		t.Fatal("result_meta contract is nil")
	}
	count := contract.Meta.ResultMeta.Properties["count"]
	if count.Type != "integer" || count.Minimum == nil || *count.Minimum != 0 {
		t.Fatalf("result meta count = %#v", count)
	}
	pagination := contract.Meta.ResultMeta.Properties["pagination"]
	if pagination.Type != "object" || pagination.Required == nil || !equalStrings(*pagination.Required, []string{"complete", "pages", "items"}) {
		t.Fatalf("result meta pagination = %#v", pagination)
	}
	if pages := pagination.Properties["pages"]; pages.Minimum == nil || *pages.Minimum != 1 {
		t.Fatalf("pagination pages = %#v", pages)
	}
	if items := pagination.Properties["items"]; items.Minimum == nil || *items.Minimum != 0 {
		t.Fatalf("pagination items = %#v", items)
	}
	if token := pagination.Properties["next_token"]; token.Type != "string" {
		t.Fatalf("pagination next_token = %#v", token)
	}
}

func TestCompileOutputAcceptsResultLevelPartial(t *testing.T) {
	definition := validCompilerDefinition()
	definition.Output.Outcomes.PartialFailure.FailedItems = nil
	shortcut := defineTypedShortcut(definition)
	partial := shortcut.typed.contract.Meta.Outcomes.PartialFailure
	if !partial.Supported || partial.ExitCode != 3 || partial.FailedItems != nil {
		t.Fatalf("result-level partial contract = %#v", partial)
	}
}

func TestCompiledSchemaRecordsJSONHTMLEscapingPolicy(t *testing.T) {
	definition := validCompilerDefinition()
	contract := defineTypedShortcut(definition).typed.contract
	if got := contract.Meta.Formats[0].EscapeHTML; got == nil || !*got {
		t.Fatalf("default JSON escape_html = %#v, want true", got)
	}

	definition.Output.DisableHTMLEscaping = true
	contract = defineTypedShortcut(definition).typed.contract
	if got := contract.Meta.Formats[0].EscapeHTML; got == nil || *got {
		t.Fatalf("unescaped JSON escape_html = %#v, want false", got)
	}
}

func TestCompileOutputDerivesExecutableGenericFormats(t *testing.T) {
	definition := validCompilerDefinition()
	definition.Output.Mode = OutputGeneric
	definition.Hooks.Renderers = map[string]typedRenderer[compilerData]{"pretty": func(io.Writer, compilerData) error { return nil }}
	command, err := compileDefinition(definition)
	if err != nil {
		t.Fatal(err)
	}
	formats := command.contract.Meta.Formats
	var names []string
	for _, format := range formats {
		names = append(names, format.Name)
		if !reflect.DeepEqual(format.SelectedBy, []string{format.Name}) {
			t.Fatalf("format %q selected_by = %v", format.Name, format.SelectedBy)
		}
	}
	if want := []string{"json", "pretty", "table", "ndjson", "csv"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("format names = %v, want %v", names, want)
	}
}

func TestCompileOutputRecordsCompatibilityFallbacks(t *testing.T) {
	fixed := defineTypedShortcut(validCompilerDefinition()).typed.contract.Meta.Formats
	if len(fixed) != 1 || fixed[0].Name != "json" || !reflect.DeepEqual(fixed[0].SelectedBy, []string{"json", "pretty", "table", "ndjson", "csv"}) {
		t.Fatalf("fixed JSON formats = %#v", fixed)
	}

	definition := validCompilerDefinition()
	definition.Output.Mode = OutputGeneric
	generic := defineTypedShortcut(definition).typed.contract.Meta.Formats
	if len(generic) != 4 || generic[0].Name != "json" || !reflect.DeepEqual(generic[0].SelectedBy, []string{"json", "pretty"}) {
		t.Fatalf("generic formats without pretty renderer = %#v", generic)
	}
}

func TestDefinePreservesCollectionDefaultForCobraAndMapBinder(t *testing.T) {
	type args struct {
		Values []string `flag:"values" schema:"optional" cli:"encoding=repeated" doc:"values"`
	}
	type data struct {
		OK bool `json:"ok" schema:"required" doc:"success state"`
	}
	definition := typedDefinition[args, data]{
		Metadata: CommandMetadata{Service: "fixture", Command: "+collection-default", Description: "collection default", Risk: RiskRead, Authorization: AuthorizationDefinition{Identities: map[Identity]IdentityAuthorization{IdentityUser: {}}}},
		Input:    InputDefinition{Fields: []InputField{{Name: "values", Default: InputDefault{Set: true, Value: []string{"a", "b"}}}}},
		Hooks: typedHooks[args, data]{Execute: func(context.Context, CommandContext, *args) (Result[data], error) {
			return Success(data{OK: true}), nil
		}},
	}
	shortcut := defineTypedShortcut(definition)
	if got := shortcut.Flags[0].Default; got != `["a","b"]` {
		t.Fatalf("legacy default = %q", got)
	}
	bound, err := bindTypedMap(shortcut.typed, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := bound.value.(*args).Values; !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("bound default = %#v", got)
	}
}

func TestDefinePanicIncludesCommandAndFieldContext(t *testing.T) {
	type badArgs struct {
		Token string `flag:"token" schema:"required"`
	}
	type data struct {
		OK bool `json:"ok" schema:"required" doc:"success state"`
	}
	definition := typedDefinition[badArgs, data]{
		Metadata: validCompilerDefinition().Metadata,
		Hooks:    typedHooks[badArgs, data]{Execute: func(context.Context, CommandContext, *badArgs) (Result[data], error) { return Success(data{}), nil }},
	}
	defer func() {
		panicValue := recover()
		if panicValue == nil {
			t.Fatal("defineTypedShortcut() did not panic")
		}
		message := panicValue.(string)
		for _, want := range []string{"typed shortcut fixture +compile", "Args field Token", "--token", "description is required"} {
			if !strings.Contains(message, want) {
				t.Fatalf("panic %q does not contain %q", message, want)
			}
		}
	}()
	_ = defineTypedShortcut(definition)
}

func TestCompileDefinitionRejectsInvalidContracts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*typedDefinition[compilerArgs, compilerData])
		want   string
	}{
		{"missing service", func(d *typedDefinition[compilerArgs, compilerData]) { d.Metadata.Service = "" }, "Metadata.Service is required"},
		{"unknown risk", func(d *typedDefinition[compilerArgs, compilerData]) { d.Metadata.Risk = Risk("delete") }, "Metadata.Risk"},
		{"unknown relation param", func(d *typedDefinition[compilerArgs, compilerData]) { d.Input.Relations[0].Params[1] = "missing" }, "unknown param --missing"},
		{"unknown conditional param", func(d *typedDefinition[compilerArgs, compilerData]) {
			auth := d.Metadata.Authorization.Identities[IdentityUser]
			auth.ConditionalScopes[0].Params = []string{"missing"}
			d.Metadata.Authorization.Identities[IdentityUser] = auth
		}, "unknown param --missing"},
		{"scope both required and conditional", func(d *typedDefinition[compilerArgs, compilerData]) {
			auth := d.Metadata.Authorization.Identities[IdentityUser]
			auth.ConditionalScopes[0].Scopes = []string{"fixture:write"}
			d.Metadata.Authorization.Identities[IdentityUser] = auth
		}, "already always required"},
		{"invalid conditional requirement", func(d *typedDefinition[compilerArgs, compilerData]) {
			auth := d.Metadata.Authorization.Identities[IdentityUser]
			auth.ConditionalScopes[0].Requirement = ScopeRequirement("sometimes")
			d.Metadata.Authorization.Identities[IdentityUser] = auth
		}, "Requirement \"sometimes\" is invalid"},
		{"conditional params without when", func(d *typedDefinition[compilerArgs, compilerData]) {
			auth := d.Metadata.Authorization.Identities[IdentityUser]
			auth.ConditionalScopes[0].When = ""
			d.Metadata.Authorization.Identities[IdentityUser] = auth
		}, "Params requires agent-readable When text"},
		{"hidden conditional param", func(d *typedDefinition[compilerArgs, compilerData]) {
			d.Input.Fields = append(d.Input.Fields, InputField{Name: "payload", CLI: CLIInput{Hidden: true}})
		}, "references hidden param --payload"},
		{"missing execute", func(d *typedDefinition[compilerArgs, compilerData]) { d.Hooks.Execute = nil }, "Hooks.Execute is required"},
		{"invalid partial path", func(d *typedDefinition[compilerArgs, compilerData]) {
			d.Output.Outcomes.PartialFailure.FailedItems.ItemsPath = "/missing"
		}, "field \"missing\" does not exist"},
		{"invalid pointer escaping", func(d *typedDefinition[compilerArgs, compilerData]) {
			d.Output.Outcomes.PartialFailure.FailedItems.ItemsPath = "/items/~2"
		}, "invalid RFC 6901 escaping"},
		{"all-items state conflict", func(d *typedDefinition[compilerArgs, compilerData]) {
			d.Output.Outcomes.PartialFailure.FailedItems.AllItems = true
		}, "AllItems conflicts"},
		{"missing failure discriminator", func(d *typedDefinition[compilerArgs, compilerData]) {
			d.Output.Outcomes.PartialFailure.FailedItems.StatePath = ""
			d.Output.Outcomes.PartialFailure.FailedItems.FailedValues = nil
		}, "requires AllItems"},
		{"failure discriminator outside state enum", func(d *typedDefinition[compilerArgs, compilerData]) {
			d.Output.Outcomes.PartialFailure.FailedItems.FailedValues = []JSONValue{"unknown"}
		}, "must be one of: ok, failed"},
		{"artifact path field required", func(d *typedDefinition[compilerArgs, compilerData]) {
			d.Output.Artifacts = []ArtifactDefinition{{Name: "items", ItemsPath: "/items"}}
		}, "PathField is required"},
		{"nil renderer", func(d *typedDefinition[compilerArgs, compilerData]) {
			d.Hooks.Renderers = map[string]typedRenderer[compilerData]{"pretty": nil}
		}, "Hooks.Renderers[\"pretty\"] is nil"},
		{"table renderer", func(d *typedDefinition[compilerArgs, compilerData]) {
			d.Output.Mode = OutputGeneric
			d.Hooks.Renderers = map[string]typedRenderer[compilerData]{"table": func(io.Writer, compilerData) error { return nil }}
		}, "custom renderers are only supported for pretty"},
		{"fixed JSON renderer", func(d *typedDefinition[compilerArgs, compilerData]) {
			d.Hooks.Renderers = map[string]typedRenderer[compilerData]{"pretty": func(io.Writer, compilerData) error { return nil }}
		}, "conflicts with Output.Mode \"fixed_json\""},
		{"invalid output mode", func(d *typedDefinition[compilerArgs, compilerData]) {
			d.Output.Mode = OutputMode("yaml")
		}, "Output.Mode \"yaml\" is invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			definition := validCompilerDefinition()
			tt.mutate(&definition)
			_, err := compileDefinition(definition)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("compileDefinition() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestCompileInputAcceptsExplicitShapeForCustomJSONType(t *testing.T) {
	type args struct {
		Payload compilerCustomJSON `flag:"payload" schema:"optional" cli:"encoding=json" doc:"custom payload"`
	}
	type data struct {
		OK bool `json:"ok" schema:"required" doc:"success state"`
	}
	shape := ObjectShape{Fields: []ValueField{{Name: "value", Description: "custom value", Required: true, Shape: StringShape{}}}}
	definition := typedDefinition[args, data]{
		Metadata: CommandMetadata{Service: "fixture", Command: "+custom-json", Description: "custom JSON input", Risk: RiskRead, Authorization: AuthorizationDefinition{Identities: map[Identity]IdentityAuthorization{IdentityUser: {}}}},
		Input:    InputDefinition{Fields: []InputField{{Name: "payload", Shape: shape}}},
		Hooks: typedHooks[args, data]{Execute: func(context.Context, CommandContext, *args) (Result[data], error) {
			return Success(data{OK: true}), nil
		}},
	}
	shortcut := defineTypedShortcut(definition)
	bound, err := bindTypedMap(shortcut.typed, map[string]any{"payload": map[string]any{"value": "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := bound.value.(*args).Payload.Value; got != "x" {
		t.Fatalf("payload value = %q", got)
	}
}

func TestCompileDataAcceptsCompleteExplicitShapeForDynamicData(t *testing.T) {
	shape := ObjectShape{AdditionalProperties: true, AdditionalPropertiesShape: StringShape{}}
	compiled, err := compileData(reflect.TypeFor[map[string]any](), DataDefinition{Shape: shape})
	if err != nil {
		t.Fatal(err)
	}
	object, ok := compiled.(ObjectShape)
	if !ok || !object.AdditionalProperties || object.AdditionalPropertiesShape == nil {
		t.Fatalf("compiled shape = %#v", compiled)
	}
	node := schemaNodeFromShape(compiled)
	if node.AdditionalProperties == nil {
		t.Fatal("schema additionalProperties missing")
	}
	if _, ok := (*node.AdditionalProperties).(typedSchemaNode); !ok {
		t.Fatalf("additionalProperties = %#v", *node.AdditionalProperties)
	}
}

func TestCompileDataAcceptsAnyForLegacyJSONPassthrough(t *testing.T) {
	compiled, err := compileData(reflect.TypeFor[any](), DataDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := compiled.(anyJSONShape); !ok {
		t.Fatalf("compiled shape = %T, want anyJSONShape", compiled)
	}
	if node := schemaNodeFromShape(compiled); !reflect.DeepEqual(node, typedSchemaNode{}) {
		t.Fatalf("schema node = %#v, want unconstrained JSON schema", node)
	}
	for _, value := range []any{nil, "text", true, float64(7), []any{"x", float64(1)}, map[string]any{"nested": []any{false, nil}}} {
		if err := valueCompatibleWithShape(value, compiled); err != nil {
			t.Fatalf("value %#v rejected: %v", value, err)
		}
	}
	if _, err := compileData(reflect.TypeFor[any](), DataDefinition{Overrides: []DataField{{Path: "/value"}}}); err == nil || !strings.Contains(err.Error(), "Overrides require struct Data") {
		t.Fatalf("override error = %v", err)
	}
}

func TestCompileDataOverrideRejectsInvalidJSONPointerEscaping(t *testing.T) {
	type data struct {
		Value string `json:"value" schema:"required" doc:"value"`
	}
	_, err := compileData(reflectType[data](), DataDefinition{Overrides: []DataField{{Path: "/value~2", Description: "override"}}})
	if err == nil || !strings.Contains(err.Error(), "invalid RFC 6901 escaping") {
		t.Fatalf("error = %v", err)
	}
}

func TestCompileDataOverrideMutatesNestedShape(t *testing.T) {
	type nested struct {
		Value string `json:"value" schema:"required"`
	}
	type data struct {
		Nested nested `json:"nested" schema:"required" doc:"nested result"`
	}
	shape, err := compileData(reflectType[data](), DataDefinition{Overrides: []DataField{{Path: "/nested/value", Description: "overridden value"}}})
	if err != nil {
		t.Fatalf("compileData() error = %v", err)
	}
	nestedShape, err := resolveShapePointer(shape, "/nested/value")
	if err != nil {
		t.Fatal(err)
	}
	_ = nestedShape
	root := shape.(ObjectShape)
	child := root.Fields[0].Shape.(ObjectShape)
	if got := child.Fields[0].Description; got != "overridden value" {
		t.Fatalf("nested description = %q", got)
	}
}

func reflectType[T any]() reflect.Type { return reflect.TypeFor[T]() }

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
