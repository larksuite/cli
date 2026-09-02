// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package event is the compatibility facade over the event kernel packages:
// the value model (model), the declaration/compilation layer (catalog), the
// processing contracts (processing), and the dedup filter kept here. New code
// should import the owning package directly; the aliases keep the existing
// declaration and call sites compiling unchanged.
package event

import (
	"github.com/larksuite/cli/internal/event/catalog"
	"github.com/larksuite/cli/internal/event/model"
	"github.com/larksuite/cli/internal/event/processing"
)

const (
	DefaultBufferSize = catalog.DefaultBufferSize
	MaxBufferSize     = catalog.MaxBufferSize
)

// RawEvent is the canonical event fact carrier; see model.Event for the field
// contracts.
type RawEvent = model.Event

// APIClient is the narrow API surface handed to domain hooks; see
// processing.APIClient for the contract.
type APIClient = processing.APIClient

type (
	ParamType        = catalog.ParamType
	SubscriptionType = catalog.SubscriptionType
	ParamValue       = catalog.ParamValue
	ParamDef         = catalog.ParamDef
	ProcessFunc      = catalog.ProcessFunc
	SchemaDef        = catalog.SchemaDef
	SchemaSpec       = catalog.SchemaSpec
	KeyDefinition    = catalog.KeyDefinition
)

const (
	ParamString = catalog.ParamString
	ParamEnum   = catalog.ParamEnum
	ParamMulti  = catalog.ParamMulti
	ParamBool   = catalog.ParamBool
	ParamInt    = catalog.ParamInt

	SubTypeEvent    = catalog.SubTypeEvent
	SubTypeCallback = catalog.SubTypeCallback
)
