// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package command

// ValueShape is the closed set of JSON shapes accepted by command definitions.
type ValueShape interface{ valueShape() }

// StringShape describes a JSON string.
type StringShape struct {
	Enum      []string
	Format    string
	MinLength *int
	MaxLength *int
}

// BooleanShape describes a JSON boolean.
type BooleanShape struct{ Enum []bool }

// IntegerShape describes a JSON integer.
type IntegerShape struct {
	Enum    []int64
	Minimum *int64
	Maximum *int64
}

// NumberShape describes a JSON number.
type NumberShape struct {
	Enum    []float64
	Minimum *float64
	Maximum *float64
}

// NullShape describes JSON null.
type NullShape struct{}

// ConstShape describes one exact JSON value.
type ConstShape struct{ Value JSONValue }

// ArrayShape describes a JSON array.
type ArrayShape struct {
	Items    ValueShape
	MinItems *int
	MaxItems *int
}

// ObjectShape describes a JSON object.
type ObjectShape struct {
	Fields                    []ValueField
	AdditionalProperties      bool
	AdditionalPropertiesShape ValueShape
}

// ValueField describes one property of an ObjectShape.
type ValueField struct {
	Name        string
	Description string
	Required    bool
	Shape       ValueShape
}

// OneOfShape describes a value matching one of several shapes.
type OneOfShape struct{ Variants []ValueShape }

func (StringShape) valueShape()  {}
func (BooleanShape) valueShape() {}
func (IntegerShape) valueShape() {}
func (NumberShape) valueShape()  {}
func (NullShape) valueShape()    {}
func (ConstShape) valueShape()   {}
func (ArrayShape) valueShape()   {}
func (ObjectShape) valueShape()  {}
func (OneOfShape) valueShape()   {}

// DataDefinition supplements the schema inferred from Data.
type DataDefinition struct {
	Shape     ValueShape
	Overrides []DataField
}

// DataField overrides one output field selected by a JSON pointer.
type DataField struct {
	Path        string
	Description string
	Shape       ValueShape
}
