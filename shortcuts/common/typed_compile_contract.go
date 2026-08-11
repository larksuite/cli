// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//nolint:forbidigo // Compiler diagnostics are registration-time programmer errors consumed by Define's panic boundary, not command-facing failures.
package common

import (
	"encoding/json"
	"fmt"
	"strings"
)

func compileRelations(definitions []Relation, fieldByName map[string]int) ([]compiledRelation, error) {
	result := make([]compiledRelation, 0, len(definitions))
	seen := make(map[string]struct{})
	for i, definition := range definitions {
		minimum := 2
		exact := 0
		switch definition.Kind {
		case RelationExactlyOne, RelationAtLeastOne, RelationCoOccur, RelationConflicts:
		case RelationRequires:
			exact = 2
		default:
			return nil, fmt.Errorf("Input.Relations[%d].Kind %q is invalid", i, definition.Kind)
		}
		if exact > 0 && len(definition.Params) != exact || exact == 0 && len(definition.Params) < minimum {
			return nil, fmt.Errorf("Input.Relations[%d] kind %s has invalid param count %d", i, definition.Kind, len(definition.Params))
		}
		if definition.Presence != PresenceExplicit && definition.Presence != PresenceNonZero {
			return nil, fmt.Errorf("Input.Relations[%d].Presence %q is invalid", i, definition.Presence)
		}
		if definition.Stage != StageSourcePreRun && definition.Stage != StageAfterPrepare {
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

func compileAuthorization(definition AuthorizationDefinition, fields []compiledInputField, fieldByName map[string]int) error {
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

func validateOutput(definition OutputDefinition, dataShape ValueShape) error {
	switch definition.Mode {
	case OutputGeneric, OutputFixedJSON:
	default:
		return fmt.Errorf("Output.Mode %q is invalid", definition.Mode)
	}
	partial := definition.Outcomes.PartialFailure
	if partial != nil {
		if partial.ExitCode <= 0 {
			return fmt.Errorf("Output.Outcomes.PartialFailure.ExitCode must be non-zero")
		}
		if failed := partial.FailedItems; failed != nil {
			itemsShape, err := resolveShapePointer(dataShape, failed.ItemsPath)
			if err != nil {
				return fmt.Errorf("Output partial failed items path %q: %w", failed.ItemsPath, err)
			}
			array, ok := unwrapArray(itemsShape)
			if !ok {
				return fmt.Errorf("Output partial failed items path %q must identify an array", failed.ItemsPath)
			}
			for _, path := range failed.IdentityPaths {
				if _, err := resolveShapePointer(array.Items, path); err != nil {
					return fmt.Errorf("Output partial identity path %q: %w", path, err)
				}
			}
			if failed.AllItems {
				if failed.StatePath != "" || len(failed.FailedValues) > 0 {
					return fmt.Errorf("Output partial FailedItems.AllItems conflicts with StatePath/FailedValues")
				}
			} else {
				if failed.StatePath == "" || len(failed.FailedValues) == 0 {
					return fmt.Errorf("Output partial FailedItems requires AllItems or StatePath with FailedValues")
				}
				stateShape, err := resolveShapePointer(array.Items, failed.StatePath)
				if err != nil {
					return fmt.Errorf("Output partial state path %q: %w", failed.StatePath, err)
				}
				for i, value := range failed.FailedValues {
					if err := valueCompatibleWithShape(value, stateShape); err != nil {
						return fmt.Errorf("Output partial FailedValues[%d]: %w", i, err)
					}
				}
			}
		}
	}
	artifactNames := make(map[string]struct{})
	for i, artifact := range definition.Artifacts {
		if strings.TrimSpace(artifact.Name) == "" {
			return fmt.Errorf("Output.Artifacts[%d].Name is required", i)
		}
		if _, duplicate := artifactNames[artifact.Name]; duplicate {
			return fmt.Errorf("Output.Artifacts contains duplicate name %q", artifact.Name)
		}
		artifactNames[artifact.Name] = struct{}{}
		if artifact.PathField == "" {
			return fmt.Errorf("Output.Artifacts[%d].PathField is required", i)
		}
		items, err := resolveShapePointer(dataShape, artifact.ItemsPath)
		if err != nil {
			return fmt.Errorf("Output.Artifacts[%d].ItemsPath %q: %w", i, artifact.ItemsPath, err)
		}
		itemShape := items
		if array, ok := unwrapArray(items); ok {
			itemShape = array.Items
		}
		for label, path := range map[string]string{"PathField": artifact.PathField, "MediaTypeField": artifact.MediaTypeField, "SizeField": artifact.SizeField} {
			if path == "" {
				continue
			}
			resolved, err := resolveShapePointer(itemShape, path)
			if err != nil {
				return fmt.Errorf("Output.Artifacts[%d].%s %q: %w", i, label, path, err)
			}
			switch label {
			case "PathField", "MediaTypeField":
				if !shapeHasType(resolved, "string") {
					return fmt.Errorf("Output.Artifacts[%d].%s %q must identify a string", i, label, path)
				}
			case "SizeField":
				if !shapeHasType(resolved, "integer") {
					return fmt.Errorf("Output.Artifacts[%d].SizeField %q must identify an integer", i, path)
				}
			}
		}
	}
	return nil
}

func resolveShapePointer(shape ValueShape, pointer string) (ValueShape, error) {
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
		object, ok := shapeAsObject(current)
		if !ok {
			return nil, fmt.Errorf("segment %q traverses non-object shape", name)
		}
		found := false
		for _, field := range object.Fields {
			if field.Name == name {
				current = field.Shape
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("field %q does not exist", name)
		}
	}
	return current, nil
}

func shapeAsObject(shape ValueShape) (ObjectShape, bool) {
	if object, ok := shape.(ObjectShape); ok {
		return object, true
	}
	if one, ok := shape.(OneOfShape); ok {
		for _, variant := range one.Variants {
			if object, ok := variant.(ObjectShape); ok {
				return object, true
			}
		}
	}
	return ObjectShape{}, false
}
func unwrapArray(shape ValueShape) (ArrayShape, bool) {
	if array, ok := shape.(ArrayShape); ok {
		return array, true
	}
	if one, ok := shape.(OneOfShape); ok {
		for _, variant := range one.Variants {
			if array, ok := variant.(ArrayShape); ok {
				return array, true
			}
		}
	}
	return ArrayShape{}, false
}
func shapeHasType(shape ValueShape, want string) bool {
	switch value := shape.(type) {
	case StringShape:
		return want == "string"
	case IntegerShape:
		return want == "integer"
	case NumberShape:
		return want == "number"
	case BooleanShape:
		return want == "boolean"
	case ArrayShape:
		return want == "array"
	case ObjectShape:
		return want == "object"
	case OneOfShape:
		for _, variant := range value.Variants {
			if shapeHasType(variant, want) {
				return true
			}
		}
	}
	return false
}

func valueCompatibleWithShape(value any, shape ValueShape) error {
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
