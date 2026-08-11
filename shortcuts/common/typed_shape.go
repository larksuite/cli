// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

// ValueShape is the closed set of JSON shapes accepted by Typed Shortcut.
type ValueShape interface{ valueShape() }

type StringShape struct {
	Enum      []string
	Format    string
	MinLength *int
	MaxLength *int
}
type BooleanShape struct{ Enum []bool }
type IntegerShape struct {
	Enum    []int64
	Minimum *int64
	Maximum *int64
}
type NumberShape struct {
	Enum    []float64
	Minimum *float64
	Maximum *float64
}
type NullShape struct{}
type ConstShape struct{ Value JSONValue }
type ArrayShape struct {
	Items    ValueShape
	MinItems *int
	MaxItems *int
}
type ObjectShape struct {
	Fields                    []ValueField
	AdditionalProperties      bool
	AdditionalPropertiesShape ValueShape
}
type ValueField struct {
	Name        string
	Description string
	Required    bool
	Shape       ValueShape
}
type OneOfShape struct{ Variants []ValueShape }

// anyJSONShape is inferred only for Data=any. It is intentionally unexported:
// arbitrary JSON is a migration escape hatch for an established standard
// envelope that already forwarded every JSON value, not a general input shape.
type anyJSONShape struct{}

func (StringShape) valueShape()  {}
func (BooleanShape) valueShape() {}
func (IntegerShape) valueShape() {}
func (NumberShape) valueShape()  {}
func (NullShape) valueShape()    {}
func (ConstShape) valueShape()   {}
func (ArrayShape) valueShape()   {}
func (ObjectShape) valueShape()  {}
func (OneOfShape) valueShape()   {}
func (anyJSONShape) valueShape() {}

type DataDefinition struct {
	Shape     ValueShape
	Overrides []DataField
}

type DataField struct {
	Path        string
	Description string
	Shape       ValueShape
}
