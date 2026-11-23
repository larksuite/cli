// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//nolint:forbidigo // Shape-lowering errors are intermediate build diagnostics wrapped by the command-set startup guard.
package common

import (
	"fmt"

	"github.com/larksuite/cli/extension/command"
)

// typedValueShape is the normalized private schema IR. Public authoring shapes
// are lowered into it once; runtime validation and schema rendering never read
// the public AST directly.
type typedValueShape interface{ typedValueShape() }

type typedStringShape struct {
	Enum      []string
	Format    string
	MinLength *int
	MaxLength *int
}
type typedBooleanShape struct{ Enum []bool }
type typedIntegerShape struct {
	Enum    []int64
	Minimum *int64
	Maximum *int64
}
type typedNumberShape struct {
	Enum    []float64
	Minimum *float64
	Maximum *float64
}
type typedNullShape struct{}
type typedConstShape struct{ Value typedJSONValue }
type typedArrayShape struct {
	Items    typedValueShape
	MinItems *int
	MaxItems *int
}
type typedObjectShape struct {
	Fields                    []typedValueField
	AdditionalProperties      bool
	AdditionalPropertiesShape typedValueShape
}
type typedValueField struct {
	Name        string
	Description string
	Required    bool
	Shape       typedValueShape
}
type typedOneOfShape struct{ Variants []typedValueShape }

// anyJSONShape is inferred only for Data=any and interface-valued fields.
type anyJSONShape struct{}

func (typedStringShape) typedValueShape()  {}
func (typedBooleanShape) typedValueShape() {}
func (typedIntegerShape) typedValueShape() {}
func (typedNumberShape) typedValueShape()  {}
func (typedNullShape) typedValueShape()    {}
func (typedConstShape) typedValueShape()   {}
func (typedArrayShape) typedValueShape()   {}
func (typedObjectShape) typedValueShape()  {}
func (typedOneOfShape) typedValueShape()   {}
func (anyJSONShape) typedValueShape()      {}

type typedDataDefinition = command.DataDefinition
type typedDataField = command.DataField

func lowerAuthoringShape(shape command.ValueShape) (typedValueShape, error) {
	switch value := shape.(type) {
	case nil:
		return nil, nil
	case command.StringShape:
		return typedStringShape{
			Enum: append([]string(nil), value.Enum...), Format: value.Format,
			MinLength: cloneScalarPointer(value.MinLength), MaxLength: cloneScalarPointer(value.MaxLength),
		}, nil
	case command.BooleanShape:
		return typedBooleanShape{Enum: append([]bool(nil), value.Enum...)}, nil
	case command.IntegerShape:
		return typedIntegerShape{
			Enum:    append([]int64(nil), value.Enum...),
			Minimum: cloneScalarPointer(value.Minimum), Maximum: cloneScalarPointer(value.Maximum),
		}, nil
	case command.NumberShape:
		return typedNumberShape{
			Enum:    append([]float64(nil), value.Enum...),
			Minimum: cloneScalarPointer(value.Minimum), Maximum: cloneScalarPointer(value.Maximum),
		}, nil
	case command.NullShape:
		return typedNullShape{}, nil
	case command.ConstShape:
		return typedConstShape{Value: cloneJSONValue(value.Value)}, nil
	case command.ArrayShape:
		items, err := lowerAuthoringShape(value.Items)
		if err != nil {
			return nil, err
		}
		return typedArrayShape{
			Items: items, MinItems: cloneScalarPointer(value.MinItems), MaxItems: cloneScalarPointer(value.MaxItems),
		}, nil
	case command.ObjectShape:
		fields := make([]typedValueField, len(value.Fields))
		for index, field := range value.Fields {
			fieldShape, err := lowerAuthoringShape(field.Shape)
			if err != nil {
				return nil, fmt.Errorf("field %q: %w", field.Name, err)
			}
			fields[index] = typedValueField{
				Name: field.Name, Description: field.Description, Required: field.Required, Shape: fieldShape,
			}
		}
		additional, err := lowerAuthoringShape(value.AdditionalPropertiesShape)
		if err != nil {
			return nil, err
		}
		return typedObjectShape{
			Fields: fields, AdditionalProperties: value.AdditionalProperties,
			AdditionalPropertiesShape: additional,
		}, nil
	case command.OneOfShape:
		variants := make([]typedValueShape, len(value.Variants))
		for index, variant := range value.Variants {
			lowered, err := lowerAuthoringShape(variant)
			if err != nil {
				return nil, fmt.Errorf("variant %d: %w", index, err)
			}
			variants[index] = lowered
		}
		return typedOneOfShape{Variants: variants}, nil
	default:
		return nil, fmt.Errorf("unsupported public shape %T", shape)
	}
}
