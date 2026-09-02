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

	"github.com/larksuite/cli/extension/command"
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
	Token              string                `flag:"token" schema:"required;minLength=1" doc:"target token"`
	Limit              command.Provided[int] `flag:"limit" schema:"optional;default=20;minimum=1;maximum=100" doc:"maximum results"`
	Payload            compilerPayload       `flag:"payload" schema:"optional;nonnullable" cli:"sources=flag|file|stdin;encoding=json" doc:"request payload"`
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
		Metadata: typedCommandMetadata{
			Service: "fixture", Command: "+compile", Description: "Compile a fixture", Risk: typedRiskWrite,
			Authorization: typedAuthorizationDefinition{Identities: map[typedIdentity]typedIdentityAuthorization{
				typedIdentityUser: {
					RequiredScopes:    []string{"fixture:write"},
					ConditionalScopes: []typedConditionalScope{{Scopes: []string{"fixture:read"}, When: "--payload selects the read path", Params: []string{"payload"}}},
				},
			}},
		},
		Input: typedInputDefinition{
			Fields:    []typedInputField{{Name: "token", CLI: typedCLIInput{Aliases: []typedFlagAlias{{Name: "legacy-token", Mode: typedAliasIndependent, Conflict: typedAliasTrimmedEqualOrError, Hidden: true}}}}},
			Relations: []typedRelation{{Kind: typedRelationRequires, Params: []string{"payload", "token"}, Presence: typedPresenceExplicit, Stage: typedStageSourcePreRun}},
		},
		Output: typedOutputDefinition{Mode: typedOutputFixedJSON},
		Hooks: typedHooks[compilerArgs, compilerData]{Execute: func(context.Context, typedRuntimeContext, *compilerArgs) (typedResult[compilerData], error) {
			return typedSuccess(compilerData{}), nil
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
	conditional := shortcut.typed.metadata.Authorization.Identities[typedIdentityUser].ConditionalScopes[0]
	if conditional.Requirement != typedScopeRequired || conditional.When == "" || !equalStrings(conditional.Params, []string{"payload"}) {
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
	if payload.cli.Encoding != typedEncodingJSON || len(payload.cli.ValueSources) != 3 {
		t.Fatalf("compiled payload CLI = %#v", payload.cli)
	}
	if got := shortcut.typed.hooks.newArgs(); got == nil {
		t.Fatal("newArgs() = nil")
	}
	if _, ok := shortcut.typed.dataShape.(typedObjectShape); !ok {
		t.Fatalf("data shape = %T, want ObjectShape", shortcut.typed.dataShape)
	}
}

func TestValueShapeClosedSet(t *testing.T) {
	shapes := []typedValueShape{
		typedStringShape{},
		typedBooleanShape{},
		typedIntegerShape{},
		typedNumberShape{},
		typedNullShape{},
		typedConstShape{},
		typedArrayShape{},
		typedObjectShape{},
		typedOneOfShape{},
		anyJSONShape{},
	}
	for _, shape := range shapes {
		shape.typedValueShape()
	}
}

func TestCompiledTypedSchemaContract(t *testing.T) {
	definition := validCompilerDefinition()
	definition.Output.Meta = typedResultMetaDefinition{Pagination: true}
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
	if contract.Meta.EnvelopeVersion != "1.0" || contract.Meta.Risk != typedRiskWrite || !equalStrings(contract.Meta.AccessTokens, []string{"user"}) {
		t.Fatalf("contract metadata = %#v", contract.Meta)
	}
	conditional := contract.Meta.Authorization.Identities[typedIdentityUser].ConditionalScopes[0]
	if conditional.When != "--payload selects the read path" || conditional.Requirement != typedScopeRequired || !equalStrings(conditional.Params, []string{"payload"}) {
		t.Fatalf("conditional authorization contract = %#v", conditional)
	}
	if contract.Meta.Outcomes.PartialFailure.Supported {
		t.Fatalf("partial outcome = %#v, want unsupported", contract.Meta.Outcomes.PartialFailure)
	}
	if contract.Meta.ResultMeta == nil {
		t.Fatal("result_meta contract is nil")
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
	definition.Output.Mode = typedOutputGeneric
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
	definition.Output.Mode = typedOutputGeneric
	generic := defineTypedShortcut(definition).typed.contract.Meta.Formats
	if len(generic) != 4 || generic[0].Name != "json" || !reflect.DeepEqual(generic[0].SelectedBy, []string{"json", "pretty"}) {
		t.Fatalf("generic formats without pretty renderer = %#v", generic)
	}
}

func TestDefinePreservesCollectionDefaultForCobra(t *testing.T) {
	type args struct {
		Values []string `flag:"values" schema:"optional" cli:"encoding=repeated" doc:"values"`
	}
	type data struct {
		OK bool `json:"ok" schema:"required" doc:"success state"`
	}
	definition := typedDefinition[args, data]{
		Metadata: typedCommandMetadata{Service: "fixture", Command: "+collection-default", Description: "collection default", Risk: typedRiskRead, Authorization: typedAuthorizationDefinition{Identities: map[typedIdentity]typedIdentityAuthorization{typedIdentityUser: {}}}},
		Input:    typedInputDefinition{Fields: []typedInputField{{Name: "values", Default: typedInputDefault{Set: true, Value: []string{"a", "b"}}}}},
		Hooks: typedHooks[args, data]{Execute: func(context.Context, typedRuntimeContext, *args) (typedResult[data], error) {
			return typedSuccess(data{OK: true}), nil
		}},
	}
	shortcut := defineTypedShortcut(definition)
	if got := shortcut.Flags[0].Default; got != `["a","b"]` {
		t.Fatalf("legacy default = %q", got)
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
		Hooks: typedHooks[badArgs, data]{Execute: func(context.Context, typedRuntimeContext, *badArgs) (typedResult[data], error) {
			return typedSuccess(data{}), nil
		}},
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
		{"unknown risk", func(d *typedDefinition[compilerArgs, compilerData]) { d.Metadata.Risk = typedRisk("delete") }, "Metadata.Risk"},
		{"unknown relation param", func(d *typedDefinition[compilerArgs, compilerData]) { d.Input.Relations[0].Params[1] = "missing" }, "unknown param --missing"},
		{"unknown conditional param", func(d *typedDefinition[compilerArgs, compilerData]) {
			auth := d.Metadata.Authorization.Identities[typedIdentityUser]
			auth.ConditionalScopes[0].Params = []string{"missing"}
			d.Metadata.Authorization.Identities[typedIdentityUser] = auth
		}, "unknown param --missing"},
		{"scope both required and conditional", func(d *typedDefinition[compilerArgs, compilerData]) {
			auth := d.Metadata.Authorization.Identities[typedIdentityUser]
			auth.ConditionalScopes[0].Scopes = []string{"fixture:write"}
			d.Metadata.Authorization.Identities[typedIdentityUser] = auth
		}, "already always required"},
		{"invalid conditional requirement", func(d *typedDefinition[compilerArgs, compilerData]) {
			auth := d.Metadata.Authorization.Identities[typedIdentityUser]
			auth.ConditionalScopes[0].Requirement = typedScopeRequirement("sometimes")
			d.Metadata.Authorization.Identities[typedIdentityUser] = auth
		}, "Requirement \"sometimes\" is invalid"},
		{"conditional params without when", func(d *typedDefinition[compilerArgs, compilerData]) {
			auth := d.Metadata.Authorization.Identities[typedIdentityUser]
			auth.ConditionalScopes[0].When = ""
			d.Metadata.Authorization.Identities[typedIdentityUser] = auth
		}, "Params requires agent-readable When text"},
		{"hidden conditional param", func(d *typedDefinition[compilerArgs, compilerData]) {
			d.Input.Fields = append(d.Input.Fields, typedInputField{Name: "payload", CLI: typedCLIInput{Hidden: true}})
		}, "references hidden param --payload"},
		{"missing execute", func(d *typedDefinition[compilerArgs, compilerData]) { d.Hooks.Execute = nil }, "Hooks.Execute is required"},
		{"nil renderer", func(d *typedDefinition[compilerArgs, compilerData]) {
			d.Hooks.Renderers = map[string]typedRenderer[compilerData]{"pretty": nil}
		}, "Hooks.Renderers[\"pretty\"] is nil"},
		{"table renderer", func(d *typedDefinition[compilerArgs, compilerData]) {
			d.Output.Mode = typedOutputGeneric
			d.Hooks.Renderers = map[string]typedRenderer[compilerData]{"table": func(io.Writer, compilerData) error { return nil }}
		}, "custom renderers are only supported for pretty"},
		{"fixed JSON renderer", func(d *typedDefinition[compilerArgs, compilerData]) {
			d.Hooks.Renderers = map[string]typedRenderer[compilerData]{"pretty": func(io.Writer, compilerData) error { return nil }}
		}, "conflicts with Output.Mode \"fixed_json\""},
		{"invalid output mode", func(d *typedDefinition[compilerArgs, compilerData]) {
			d.Output.Mode = typedOutputMode("yaml")
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
	shape := command.ObjectShape{Fields: []command.ValueField{{Name: "value", Description: "custom value", Required: true, Shape: command.StringShape{}}}}
	definition := typedDefinition[args, data]{
		Metadata: typedCommandMetadata{Service: "fixture", Command: "+custom-json", Description: "custom JSON input", Risk: typedRiskRead, Authorization: typedAuthorizationDefinition{Identities: map[typedIdentity]typedIdentityAuthorization{typedIdentityUser: {}}}},
		Input:    typedInputDefinition{Fields: []typedInputField{{Name: "payload", Shape: shape}}},
		Hooks: typedHooks[args, data]{Execute: func(context.Context, typedRuntimeContext, *args) (typedResult[data], error) {
			return typedSuccess(data{OK: true}), nil
		}},
	}
	shortcut := defineTypedShortcut(definition)
	field := shortcut.typed.fields[shortcut.typed.fieldByName["payload"]]
	value, err := decodeCompiledValue(`{"value":"x"}`, field)
	if err != nil {
		t.Fatal(err)
	}
	if got := value.(compilerCustomJSON).Value; got != "x" {
		t.Fatalf("payload value = %q", got)
	}
}

func TestCompileDataAcceptsCompleteExplicitShapeForDynamicData(t *testing.T) {
	shape := command.ObjectShape{AdditionalProperties: true, AdditionalPropertiesShape: command.StringShape{}}
	compiled, err := compileData(reflect.TypeFor[map[string]any](), typedDataDefinition{Shape: shape})
	if err != nil {
		t.Fatal(err)
	}
	object, ok := compiled.(typedObjectShape)
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
	compiled, err := compileData(reflect.TypeFor[any](), typedDataDefinition{})
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
	if _, err := compileData(reflect.TypeFor[any](), typedDataDefinition{Overrides: []typedDataField{{Path: "/value"}}}); err == nil || !strings.Contains(err.Error(), "Overrides require struct Data") {
		t.Fatalf("override error = %v", err)
	}
}

func TestCompileDataOverrideRejectsInvalidJSONPointerEscaping(t *testing.T) {
	type data struct {
		Value string `json:"value" schema:"required" doc:"value"`
	}
	_, err := compileData(reflectType[data](), typedDataDefinition{Overrides: []typedDataField{{Path: "/value~2", Description: "override"}}})
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
	shape, err := compileData(reflectType[data](), typedDataDefinition{Overrides: []typedDataField{{Path: "/nested/value", Description: "overridden value"}}})
	if err != nil {
		t.Fatalf("compileData() error = %v", err)
	}
	nestedShape, err := resolveShapePointer(shape, "/nested/value")
	if err != nil {
		t.Fatal(err)
	}
	_ = nestedShape
	root := shape.(typedObjectShape)
	child := root.Fields[0].Shape.(typedObjectShape)
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
