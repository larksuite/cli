// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"io"
	"reflect"
)

type compiledCommand struct {
	metadata    typedCommandMetadata
	argsType    reflect.Type
	dataType    reflect.Type
	fields      []compiledInputField
	fieldByName map[string]int
	relations   []compiledRelation
	dataShape   typedValueShape
	output      typedOutputDefinition
	contract    typedSchemaContract
	hooks       compiledHooks
	pageOutput  bool
}

type compiledInputField struct {
	name          string
	goName        string
	index         []int
	valueIndex    []int
	valueType     reflect.Type
	provided      bool
	required      bool
	nullable      *bool
	description   string
	shape         typedValueShape
	shapeExplicit bool
	defaultValue  typedInputDefault
	cli           typedCLIInput
}

type compiledRelation struct {
	kind     typedRelationKind
	fields   []int
	presence typedPresenceMode
	stage    typedRelationStage
}

type compiledResult struct {
	data    any
	outcome typedOutcomeKind
	meta    *typedResultMeta
}

type compiledHooks struct {
	newArgs   func() any
	normalize func(context.Context, typedRuntimeContext, any) error
	validate  func(context.Context, typedRuntimeContext, any) error
	dryRun    func(context.Context, typedRuntimeContext, any) (*DryRunAPI, error)
	execute   func(context.Context, typedRuntimeContext, any) (compiledResult, error)
	renderers map[string]func(io.Writer, any) error
}
