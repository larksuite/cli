// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//nolint:forbidigo // Compiler diagnostics are build-time declaration errors wrapped by the command-set startup guard.
package common

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

func compileDefinitionParts(
	metadata typedCommandMetadata,
	input typedInputDefinition,
	output typedOutputDefinition,
	argsType reflect.Type,
	dataType reflect.Type,
	hooks compiledHooks,
	renderers map[string]rendererMarker,
	pageOutput bool,
) (*compiledCommand, error) {
	metadata = normalizeCommandMetadata(metadata)
	if err := validateCommandMetadata(metadata); err != nil {
		return nil, err
	}
	fields, fieldByName, err := compileInput(argsType, input)
	if err != nil {
		return nil, err
	}
	relations, err := compileRelations(input.Relations, fieldByName)
	if err != nil {
		return nil, err
	}
	if err := compileAuthorization(metadata.Authorization, fields, fieldByName); err != nil {
		return nil, err
	}
	dataShape, err := compileData(dataType, output.Data)
	if err != nil {
		return nil, err
	}
	if hooks.execute == nil {
		return nil, fmt.Errorf("Hooks.Execute is required")
	}
	if err := validateOutput(output, dataShape); err != nil {
		return nil, err
	}
	if err := validateOutputHooks(output, renderers); err != nil {
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
		output:      output,
		hooks:       hooks,
		pageOutput:  pageOutput,
	}
	command.contract = buildTypedSchemaContract(command)
	return command, nil
}

func normalizeCommandMetadata(metadata typedCommandMetadata) typedCommandMetadata {
	identities := make(map[typedIdentity]typedIdentityAuthorization, len(metadata.Authorization.Identities))
	for identity, authorization := range metadata.Authorization.Identities {
		authorization.RequiredScopes = append([]string(nil), authorization.RequiredScopes...)
		authorization.ConditionalScopes = append([]typedConditionalScope(nil), authorization.ConditionalScopes...)
		for i := range authorization.ConditionalScopes {
			conditional := &authorization.ConditionalScopes[i]
			conditional.Scopes = append([]string(nil), conditional.Scopes...)
			conditional.Params = append([]string(nil), conditional.Params...)
			if conditional.Requirement == "" {
				conditional.Requirement = typedScopeRequired
			}
		}
		identities[identity] = authorization
	}
	metadata.Authorization.Identities = identities
	metadata.Authorization.IdentityOrder = append([]typedIdentity(nil), metadata.Authorization.IdentityOrder...)
	return metadata
}

func validateCommandMetadata(metadata typedCommandMetadata) error {
	service := strings.TrimSpace(string(metadata.Service))
	if service == "" {
		return fmt.Errorf("Metadata.Service is required")
	}
	if service != string(metadata.Service) || strings.ContainsAny(service, " \t\r\n/") {
		return fmt.Errorf("Metadata.Service %q must be one trimmed command segment", metadata.Service)
	}
	command := strings.TrimSpace(metadata.Command)
	if command == "" {
		return fmt.Errorf("Metadata.Command is required")
	}
	if command != metadata.Command || strings.ContainsAny(command, " \t\r\n/") {
		return fmt.Errorf("Metadata.Command %q must be one trimmed command segment", metadata.Command)
	}
	if !strings.HasPrefix(command, "+") {
		return fmt.Errorf("Metadata.Command %q must start with '+'", metadata.Command)
	}
	if command == "+" {
		return fmt.Errorf("Metadata.Command must contain a name after '+'")
	}
	if strings.TrimSpace(metadata.Description) == "" {
		return fmt.Errorf("Metadata.Description is required")
	}
	switch metadata.Risk {
	case typedRiskRead, typedRiskWrite, typedRiskHighRiskWrite:
	default:
		return fmt.Errorf("Metadata.Risk %q is invalid", metadata.Risk)
	}
	if len(metadata.Authorization.Identities) == 0 {
		return fmt.Errorf("Metadata.Authorization.Identities must declare at least one identity")
	}
	for identity, auth := range metadata.Authorization.Identities {
		if identity != typedIdentityUser && identity != typedIdentityBot {
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
			case typedScopeRequired, typedScopeBestEffort:
			default:
				return fmt.Errorf("%s.Requirement %q is invalid", path, conditional.Requirement)
			}
		}
	}
	if len(metadata.Authorization.IdentityOrder) > 0 {
		if len(metadata.Authorization.IdentityOrder) != len(metadata.Authorization.Identities) {
			return fmt.Errorf("Metadata.Authorization.IdentityOrder must contain each declared identity exactly once")
		}
		seen := make(map[typedIdentity]struct{}, len(metadata.Authorization.IdentityOrder))
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

func shortcutFromCompiled(compiled *compiledCommand) Shortcut {
	metadata := compiled.metadata
	shortcut := Shortcut{
		Service:     string(metadata.Service),
		Command:     metadata.Command,
		Description: metadata.Description,
		Risk:        string(metadata.Risk),
		Hidden:      metadata.Hidden,
		typed:       compiled,
	}
	identities := make([]string, 0, len(metadata.Authorization.Identities))
	identityOrder := metadata.Authorization.IdentityOrder
	if len(identityOrder) == 0 {
		identityOrder = []typedIdentity{typedIdentityUser, typedIdentityBot}
	}
	for _, identity := range identityOrder {
		if auth, ok := metadata.Authorization.Identities[identity]; ok {
			identities = append(identities, string(identity))
			scopes := append([]string(nil), auth.RequiredScopes...)
			conditional := flattenConditionalScopes(auth.ConditionalScopes)
			switch identity {
			case typedIdentityUser:
				shortcut.UserScopes = scopes
				shortcut.ConditionalUserScopes = conditional
			case typedIdentityBot:
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

func flattenConditionalScopes(definitions []typedConditionalScope) []string {
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
			if alias.Mode == typedAliasNormalize {
				flag.Aliases = append(flag.Aliases, alias.Name)
			}
		}
		if stringShape, ok := field.shape.(typedStringShape); ok {
			flag.Enum = append([]string(nil), stringShape.Enum...)
		}
		if hasIndependentAlias(field.cli.Aliases) {
			flag.Required = false
		}
		flags = append(flags, flag)
		for _, alias := range field.cli.Aliases {
			if alias.Mode != typedAliasIndependent {
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

func hasIndependentAlias(aliases []typedFlagAlias) bool {
	for _, alias := range aliases {
		if alias.Mode == typedAliasIndependent {
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
		case typedEncodingRepeated:
			if t.Elem().Kind() == reflect.String {
				return "string_array"
			}
		case typedEncodingCommaOrRepeated:
			if isIntegerKind(t.Elem().Kind()) {
				return "int_array"
			}
			return "string_slice"
		}
	}
	return "string"
}

func legacyInputSources(sources []typedValueSource) []string {
	var result []string
	for _, source := range sources {
		switch source {
		case typedSourceFile:
			result = append(result, File)
		case typedSourceStdin:
			result = append(result, Stdin)
		}
	}
	return result
}
