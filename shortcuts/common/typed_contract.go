// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"io"
	"reflect"

	"github.com/larksuite/cli/internal/citation"
)

type compiledCommand struct {
	metadata    CommandMetadata
	argsType    reflect.Type
	dataType    reflect.Type
	fields      []compiledInputField
	fieldByName map[string]int
	relations   []compiledRelation
	dataShape   ValueShape
	output      OutputDefinition
	contract    typedSchemaContract
	hooks       compiledHooks
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
	shape         ValueShape
	shapeExplicit bool
	defaultValue  InputDefault
	cli           CLIInput
}

type compiledRelation struct {
	kind     RelationKind
	fields   []int
	presence PresenceMode
	stage    RelationStage
}

type compiledResult struct {
	data    any
	outcome OutcomeKind
	meta    *ResultMeta
}

type compiledHooks struct {
	newArgs   func() any
	normalize func(context.Context, CommandContext, any) error
	validate  func(context.Context, CommandContext, any) error
	dryRun    func(context.Context, CommandContext, any) *DryRunAPI
	execute   func(context.Context, CommandContext, any) (compiledResult, error)
	renderers map[string]func(io.Writer, any) error

	buildCitation func(context.Context, CommandContext, any, any) []citation.Citation
}
