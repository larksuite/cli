// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//nolint:forbidigo // Compiler diagnostics are build-time declaration errors wrapped by the command-set startup guard.
package common

import (
	"encoding/json"
	"fmt"
	"strings"
)

func compileRelations(definitions []typedRelation, fieldByName map[string]int) ([]compiledRelation, error) {
	result := make([]compiledRelation, 0, len(definitions))
	seen := make(map[string]struct{})
	for i, definition := range definitions {
		minimum := 2
		exact := 0
		switch definition.Kind {
		case typedRelationExactlyOne, typedRelationAtLeastOne, typedRelationCoOccur, typedRelationConflicts:
		case typedRelationRequires:
			exact = 2
		default:
			return nil, fmt.Errorf("Input.Relations[%d].Kind %q is invalid", i, definition.Kind)
		}
		if exact > 0 && len(definition.Params) != exact || exact == 0 && len(definition.Params) < minimum {
			return nil, fmt.Errorf("Input.Relations[%d] kind %s has invalid param count %d", i, definition.Kind, len(definition.Params))
		}
		if definition.Presence != typedPresenceExplicit && definition.Presence != typedPresenceNonZero {
			return nil, fmt.Errorf("Input.Relations[%d].Presence %q is invalid", i, definition.Presence)
		}
		if definition.Stage != typedStageSourcePreRun && definition.Stage != typedStageAfterPrepare {
			return nil, fmt.Errorf("Input.Relations[%d].Stage %q is invalid", i, definition.Stage)
		}
		compiled := compiledRelation{kind: definition.Kind, presence: definition.Presence, stage: definition.Stage}
		local := make(map[string]struct{})
		for _, name := range definition.Params {
			field, ok := fieldByName[name]
			if !ok {
				return nil, fmt.Errorf("Input.Relations[%d] references unknown param --%s", i, name)
			}
			if _, duplicate := local[name]; duplicate {
				return nil, fmt.Errorf("Input.Relations[%d] repeats param --%s", i, name)
			}
			local[name] = struct{}{}
			compiled.fields = append(compiled.fields, field)
		}
		keyBytes, _ := json.Marshal(definition)
		key := string(keyBytes)
		if _, duplicate := seen[key]; duplicate {
			return nil, fmt.Errorf("Input.Relations[%d] duplicates an earlier relation", i)
		}
		seen[key] = struct{}{}
		result = append(result, compiled)
	}
	return result, nil
}

func compileAuthorization(definition typedAuthorizationDefinition, fields []compiledInputField, fieldByName map[string]int) error {
	for identity, authorization := range definition.Identities {
		for i, conditional := range authorization.ConditionalScopes {
			path := fmt.Sprintf("Authorization.%s.ConditionalScopes[%d]", identity, i)
			if len(conditional.Params) > 0 && conditional.When == "" {
				return fmt.Errorf("%s.Params requires agent-readable When text", path)
			}
			seen := make(map[string]struct{}, len(conditional.Params))
			for j, param := range conditional.Params {
				if param == "" || param != strings.TrimSpace(param) {
					return fmt.Errorf("%s.Params[%d] must be a non-blank trimmed param", path, j)
				}
				fieldIndex, ok := fieldByName[param]
				if !ok {
					return fmt.Errorf("%s references unknown param --%s", path, param)
				}
				if fields[fieldIndex].cli.Hidden {
					return fmt.Errorf("%s references hidden param --%s; use a public canonical param", path, param)
				}
				if _, duplicate := seen[param]; duplicate {
					return fmt.Errorf("%s.Params contains duplicate param --%s", path, param)
				}
				seen[param] = struct{}{}
			}
		}
	}
	return nil
}

func validateOutput(definition typedOutputDefinition, dataShape typedValueShape) error {
	switch definition.Mode {
	case typedOutputGeneric, typedOutputFixedJSON:
	default:
		return fmt.Errorf("Output.Mode %q is invalid", definition.Mode)
	}
	return nil
}

func decodeJSONPointerSegment(segment string) (string, bool) {
	var builder strings.Builder
	for index := 0; index < len(segment); index++ {
		if segment[index] != '~' {
			builder.WriteByte(segment[index])
			continue
		}
		if index+1 >= len(segment) {
			return "", false
		}
		index++
		switch segment[index] {
		case '0':
			builder.WriteByte('~')
		case '1':
			builder.WriteByte('/')
		default:
			return "", false
		}
	}
	return builder.String(), true
}

func resolveShapePointer(shape typedValueShape, pointer string) (typedValueShape, error) {
	if pointer == "" {
		return shape, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, fmt.Errorf("must be an RFC 6901 JSON Pointer")
	}
	current := shape
	for _, encoded := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		name, valid := decodeJSONPointerSegment(encoded)
		if !valid {
			return nil, fmt.Errorf("segment %q has invalid RFC 6901 escaping", encoded)
		}
		var err error
		current, err = resolveShapeField(current, name)
		if err != nil {
			return nil, err
		}
	}
	return current, nil
}

func resolveShapeField(shape typedValueShape, name string) (typedValueShape, error) {
	switch value := shape.(type) {
	case typedObjectShape:
		for _, field := range value.Fields {
			if field.Name == name {
				return field.Shape, nil
			}
		}
		return nil, fmt.Errorf("field %q does not exist", name)
	case typedOneOfShape:
		var resolved []typedValueShape
		for _, variant := range value.Variants {
			if _, null := variant.(typedNullShape); null {
				continue
			}
			field, err := resolveShapeField(variant, name)
			if err != nil {
				return nil, err
			}
			resolved = append(resolved, field)
		}
		return combineResolvedShapes(resolved)
	default:
		return nil, fmt.Errorf("segment %q traverses non-object shape", name)
	}
}

func combineResolvedShapes(shapes []typedValueShape) (typedValueShape, error) {
	switch len(shapes) {
	case 0:
		return nil, fmt.Errorf("shape has no applicable variant")
	case 1:
		return shapes[0], nil
	default:
		return typedOneOfShape{Variants: shapes}, nil
	}
}

func shapeAsObject(shape typedValueShape) (typedObjectShape, bool) {
	if object, ok := shape.(typedObjectShape); ok {
		return object, true
	}
	if one, ok := shape.(typedOneOfShape); ok {
		var combined typedObjectShape
		found := false
		for _, variant := range one.Variants {
			if _, null := variant.(typedNullShape); null {
				continue
			}
			object, ok := shapeAsObject(variant)
			if !ok {
				return typedObjectShape{}, false
			}
			if !found {
				combined = object
				found = true
			} else if object.AdditionalProperties {
				combined.AdditionalProperties = true
			}
		}
		return combined, found
	}
	return typedObjectShape{}, false
}
func valueCompatibleWithShape(value any, shape typedValueShape) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("value is not JSON-encodable: %w", err)
	}
	normalized, err := decodeJSONValidationValue(encoded)
	if err != nil {
		return fmt.Errorf("value is not valid JSON: %w", err)
	}
	return validateJSONValueAgainstShape(normalized, shape, "value")
}
