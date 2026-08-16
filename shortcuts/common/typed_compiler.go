// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//nolint:forbidigo // Compiler diagnostics are registration-time programmer errors converted to contextual Define panics, not command-facing failures.
package common

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/larksuite/cli/internal/citation"
)

// Define compiles a Typed Shortcut definition. Invalid definitions are
// programmer errors and panic during registration; no partial legacy fallback
// is returned.
func Define[Args any, Data any](definition Definition[Args, Data]) Shortcut {
	compiled, err := compileDefinition(definition)
	if err != nil {
		service := strings.TrimSpace(definition.Metadata.Service)
		command := strings.TrimSpace(definition.Metadata.Command)
		if service == "" {
			service = "<missing-service>"
		}
		if command == "" {
			command = "<missing-command>"
		}
		panic(fmt.Sprintf("typed shortcut %s %s: %v", service, command, err))
	}
	shortcut := shortcutFromCompiled(compiled)
	if err := validateTypedFlagMountPlan(compiled, shortcut.PrintFlagSchema != nil, Risk(shortcut.Risk)); err != nil {
		panic(fmt.Sprintf("typed shortcut %s %s: %v", compiled.metadata.Service, compiled.metadata.Command, err))
	}
	return shortcut
}

func compileDefinition[Args any, Data any](definition Definition[Args, Data]) (*compiledCommand, error) {
	metadata := normalizeCommandMetadata(definition.Metadata)
	if err := validateCommandMetadata(metadata); err != nil {
		return nil, err
	}
	argsType := reflect.TypeFor[Args]()
	dataType := reflect.TypeFor[Data]()
	fields, fieldByName, err := compileInput(argsType, definition.Input)
	if err != nil {
		return nil, err
	}
	relations, err := compileRelations(definition.Input.Relations, fieldByName)
	if err != nil {
		return nil, err
	}
	if err := compileAuthorization(metadata.Authorization, fields, fieldByName); err != nil {
		return nil, err
	}
	dataShape, err := compileData(dataType, definition.Output.Data)
	if err != nil {
		return nil, err
	}
	if definition.Hooks.Execute == nil {
		return nil, fmt.Errorf("Hooks.Execute is required")
	}
	if err := validateOutput(definition.Output, dataShape); err != nil {
		return nil, err
	}
	if err := validateOutputHooks(definition.Output, rendererMarkers(definition.Hooks.Renderers)); err != nil {
		return nil, err
	}
	if err := validateTypedCitation(definition.Output.Citation, definition.Hooks.BuildCitation != nil, metadata.Risk, definition.Output.Mode); err != nil {
		return nil, err
	}
	command := &compiledCommand{
		metadata:    metadata,
		argsType:    argsType,
		dataType:    dataType,
		fields:      fields,
		fieldByName: fieldByName,
		relations:   relations,
		dataShape:   dataShape,
		output:      definition.Output,
		hooks:       adaptHooks(definition.Hooks),
	}
	command.contract = buildTypedSchemaContract(command)
	return command, nil
}

func normalizeCommandMetadata(metadata CommandMetadata) CommandMetadata {
	identities := make(map[Identity]IdentityAuthorization, len(metadata.Authorization.Identities))
	for identity, authorization := range metadata.Authorization.Identities {
		authorization.RequiredScopes = append([]string(nil), authorization.RequiredScopes...)
		authorization.ConditionalScopes = append([]ConditionalScope(nil), authorization.ConditionalScopes...)
		for i := range authorization.ConditionalScopes {
			conditional := &authorization.ConditionalScopes[i]
			conditional.Scopes = append([]string(nil), conditional.Scopes...)
			conditional.Params = append([]string(nil), conditional.Params...)
			if conditional.Requirement == "" {
				conditional.Requirement = ScopeRequired
			}
		}
		identities[identity] = authorization
	}
	metadata.Authorization.Identities = identities
	metadata.Authorization.IdentityOrder = append([]Identity(nil), metadata.Authorization.IdentityOrder...)
	metadata.Tips = append([]string(nil), metadata.Tips...)
	return metadata
}

func validateCommandMetadata(metadata CommandMetadata) error {
	if strings.TrimSpace(metadata.Service) == "" {
		return fmt.Errorf("Metadata.Service is required")
	}
	if strings.TrimSpace(metadata.Command) == "" {
		return fmt.Errorf("Metadata.Command is required")
	}
	if !strings.HasPrefix(metadata.Command, "+") {
		return fmt.Errorf("Metadata.Command %q must start with '+'", metadata.Command)
	}
	if strings.TrimSpace(metadata.Description) == "" {
		return fmt.Errorf("Metadata.Description is required")
	}
	for i, tip := range metadata.Tips {
		if strings.TrimSpace(tip) == "" {
			return fmt.Errorf("Metadata.Tips[%d] must not be blank", i)
		}
	}
	switch metadata.Risk {
	case RiskRead, RiskWrite, RiskHighRiskWrite:
	default:
		return fmt.Errorf("Metadata.Risk %q is invalid", metadata.Risk)
	}
	if len(metadata.Authorization.Identities) == 0 {
		return fmt.Errorf("Metadata.Authorization.Identities must declare at least one identity")
	}
	for identity, auth := range metadata.Authorization.Identities {
		if identity != IdentityUser && identity != IdentityBot {
			return fmt.Errorf("Metadata.Authorization identity %q is invalid", identity)
		}
		if err := validateScopeList(auth.RequiredScopes, fmt.Sprintf("Authorization.%s.RequiredScopes", identity)); err != nil {
			return err
		}
		requiredScopes := make(map[string]struct{}, len(auth.RequiredScopes))
		for _, scope := range auth.RequiredScopes {
			requiredScopes[scope] = struct{}{}
		}
		for i, conditional := range auth.ConditionalScopes {
			path := fmt.Sprintf("Authorization.%s.ConditionalScopes[%d]", identity, i)
			if err := validateScopeList(conditional.Scopes, path); err != nil {
				return err
			}
			if len(conditional.Scopes) == 0 {
				return fmt.Errorf("%s.Scopes is empty", path)
			}
			for _, scope := range conditional.Scopes {
				if _, alwaysRequired := requiredScopes[scope]; alwaysRequired {
					return fmt.Errorf("%s scope %q is already always required for identity %q", path, scope, identity)
				}
			}
			if conditional.When != strings.TrimSpace(conditional.When) {
				return fmt.Errorf("%s.When must be trimmed", path)
			}
			switch conditional.Requirement {
			case ScopeRequired, ScopeBestEffort:
			default:
				return fmt.Errorf("%s.Requirement %q is invalid", path, conditional.Requirement)
			}
		}
	}
	if len(metadata.Authorization.IdentityOrder) > 0 {
		if len(metadata.Authorization.IdentityOrder) != len(metadata.Authorization.Identities) {
			return fmt.Errorf("Metadata.Authorization.IdentityOrder must contain each declared identity exactly once")
		}
		seen := make(map[Identity]struct{}, len(metadata.Authorization.IdentityOrder))
		for _, identity := range metadata.Authorization.IdentityOrder {
			if _, ok := metadata.Authorization.Identities[identity]; !ok {
				return fmt.Errorf("Metadata.Authorization.IdentityOrder contains undeclared identity %q", identity)
			}
			if _, duplicate := seen[identity]; duplicate {
				return fmt.Errorf("Metadata.Authorization.IdentityOrder contains duplicate identity %q", identity)
			}
			seen[identity] = struct{}{}
		}
	}
	return nil
}

func validateScopeList(scopes []string, path string) error {
	seen := make(map[string]struct{}, len(scopes))
	for i, scope := range scopes {
		if strings.TrimSpace(scope) == "" || scope != strings.TrimSpace(scope) {
			return fmt.Errorf("%s[%d] must be a non-blank trimmed scope", path, i)
		}
		if _, ok := seen[scope]; ok {
			return fmt.Errorf("%s contains duplicate scope %q", path, scope)
		}
		seen[scope] = struct{}{}
	}
	return nil
}

// validateTypedCitation enforces that Output.Citation and Hooks.BuildCitation
// are declared together, that the typed path does not carry the legacy
// Build builder, that the command's output mode can actually carry a JSON
// envelope (citations ride the envelope; a mode whose format set drops JSON
// would compile a declaration nothing ever emits), and defers the rest to
// the shared citation contract.
func validateTypedCitation(def *CitationDefinition, hasHook bool, risk Risk, mode OutputMode) error {
	if def == nil && !hasHook {
		return nil
	}
	if def == nil || !hasHook {
		return fmt.Errorf("Output.Citation and Hooks.BuildCitation must be set together")
	}
	if def.Build != nil {
		return fmt.Errorf("Output.Citation.Build is legacy-only; use Hooks.BuildCitation")
	}
	if !outputModeIncludesJSON(mode) {
		return fmt.Errorf("citation requires an output mode whose format set includes JSON, got %q", mode)
	}
	return validateCitationDeclaration(def, string(risk))
}

// outputModeIncludesJSON reports whether mode's format set includes a JSON
// envelope. Both OutputGeneric (typedOutputFormats always lists "json" first,
// selectable by --format json and the default) and OutputFixedJSON (every
// successful invocation is a JSON envelope) satisfy this today, so this
// check is vacuously true for every mode validateOutput currently accepts.
// It is written as an explicit enumeration rather than a default-true
// fallback so a future OutputMode that drops JSON support fails this check
// at Define time instead of silently compiling a citation declaration that
// nothing will ever emit.
func outputModeIncludesJSON(mode OutputMode) bool {
	switch mode {
	case OutputGeneric, OutputFixedJSON:
		return true
	default:
		return false
	}
}

func adaptHooks[Args any, Data any](hooks Hooks[Args, Data]) compiledHooks {
	adapted := compiledHooks{newArgs: func() any { return new(Args) }}
	if hooks.Normalize != nil {
		adapted.normalize = func(ctx context.Context, cc CommandContext, args any) error {
			return hooks.Normalize(ctx, cc, args.(*Args))
		}
	}
	if hooks.Validate != nil {
		adapted.validate = func(ctx context.Context, cc CommandContext, args any) error {
			return hooks.Validate(ctx, cc, args.(*Args))
		}
	}
	if hooks.DryRun != nil {
		adapted.dryRun = func(ctx context.Context, cc CommandContext, args any) *DryRunAPI {
			return hooks.DryRun(ctx, cc, args.(*Args))
		}
	}
	adapted.execute = func(ctx context.Context, cc CommandContext, args any) (compiledResult, error) {
		result, err := hooks.Execute(ctx, cc, args.(*Args))
		return compiledResult{data: result.Data, outcome: result.Outcome, meta: result.Meta}, err
	}
	if hooks.BuildCitation != nil {
		adapted.buildCitation = func(ctx context.Context, cc CommandContext, args any, data any) []citation.Citation {
			return hooks.BuildCitation(ctx, cc, args.(*Args), data.(Data))
		}
	}
	if len(hooks.Renderers) > 0 {
		adapted.renderers = make(map[string]func(io.Writer, any) error, len(hooks.Renderers))
		for name, renderer := range hooks.Renderers {
			r := renderer
			adapted.renderers[name] = func(w io.Writer, data any) error { return r(w, data.(Data)) }
		}
	}
	return adapted
}

func shortcutFromCompiled(compiled *compiledCommand) Shortcut {
	metadata := compiled.metadata
	shortcut := Shortcut{
		Service:     metadata.Service,
		Command:     metadata.Command,
		Description: metadata.Description,
		Risk:        string(metadata.Risk),
		Hidden:      metadata.Hidden,
		Tips:        append([]string(nil), metadata.Tips...),
		typed:       compiled,
	}
	identities := make([]string, 0, len(metadata.Authorization.Identities))
	identityOrder := metadata.Authorization.IdentityOrder
	if len(identityOrder) == 0 {
		identityOrder = []Identity{IdentityUser, IdentityBot}
	}
	for _, identity := range identityOrder {
		if auth, ok := metadata.Authorization.Identities[identity]; ok {
			identities = append(identities, string(identity))
			scopes := append([]string(nil), auth.RequiredScopes...)
			conditional := flattenConditionalScopes(auth.ConditionalScopes)
			switch identity {
			case IdentityUser:
				shortcut.UserScopes = scopes
				shortcut.ConditionalUserScopes = conditional
			case IdentityBot:
				shortcut.BotScopes = scopes
				shortcut.ConditionalBotScopes = conditional
			}
		}
	}
	shortcut.AuthTypes = identities
	shortcut.Flags = legacyFlagsFromCompiled(compiled.fields)
	shortcut.PrintFlagSchema = typedFlagSchemaPrinter(compiled)
	return shortcut
}

func flattenConditionalScopes(definitions []ConditionalScope) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, definition := range definitions {
		for _, scope := range definition.Scopes {
			if _, ok := seen[scope]; ok {
				continue
			}
			seen[scope] = struct{}{}
			result = append(result, scope)
		}
	}
	return result
}

func legacyFlagsFromCompiled(fields []compiledInputField) []Flag {
	flags := make([]Flag, 0, len(fields))
	for _, field := range fields {
		flag := Flag{
			Name:     field.name,
			Type:     legacyFlagType(field),
			Desc:     field.description,
			Required: field.required,
			Hidden:   field.cli.Hidden,
			Input:    legacyInputSources(field.cli.ValueSources),
		}
		if field.defaultValue.Set {
			if encoded, err := json.Marshal(field.defaultValue.Value); err == nil && (field.valueType.Kind() == reflect.Slice || field.valueType.Kind() == reflect.Array) {
				flag.Default = string(encoded)
			} else {
				flag.Default = fmt.Sprint(field.defaultValue.Value)
			}
		}
		for _, alias := range field.cli.Aliases {
			if alias.Mode == AliasNormalize {
				flag.Aliases = append(flag.Aliases, alias.Name)
			}
		}
		if stringShape, ok := field.shape.(StringShape); ok {
			flag.Enum = append([]string(nil), stringShape.Enum...)
		}
		if hasIndependentAlias(field.cli.Aliases) {
			flag.Required = false
		}
		flags = append(flags, flag)
		for _, alias := range field.cli.Aliases {
			if alias.Mode != AliasIndependent {
				continue
			}
			aliasFlag := flag
			aliasFlag.Name = alias.Name
			aliasFlag.Aliases = nil
			aliasFlag.Default = ""
			aliasFlag.Required = false
			aliasFlag.Hidden = alias.Hidden
			aliasFlag.Desc = fmt.Sprintf("Compatibility alias for --%s", field.name)
			flags = append(flags, aliasFlag)
		}
	}
	return flags
}

func hasIndependentAlias(aliases []FlagAlias) bool {
	for _, alias := range aliases {
		if alias.Mode == AliasIndependent {
			return true
		}
	}
	return false
}

func legacyFlagType(field compiledInputField) string {
	t := field.valueType
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Bool:
		return "bool"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "int"
	case reflect.Float32, reflect.Float64:
		return "float64"
	case reflect.Slice, reflect.Array:
		switch field.cli.Encoding {
		case EncodingRepeated:
			if t.Elem().Kind() == reflect.String {
				return "string_array"
			}
		case EncodingCommaOrRepeated:
			if isIntegerKind(t.Elem().Kind()) {
				return "int_array"
			}
			return "string_slice"
		}
	}
	return "string"
}

func legacyInputSources(sources []ValueSource) []string {
	var result []string
	for _, source := range sources {
		switch source {
		case SourceFile:
			result = append(result, File)
		case SourceStdin:
			result = append(result, Stdin)
		}
	}
	return result
}
