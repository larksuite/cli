// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package command

// OutputDefinition declares result formats.
type OutputDefinition struct {
	Data DataDefinition
	Meta ResultMetaDefinition
	Mode OutputMode

	DisableHTMLEscaping bool
}

// ResultMetaDefinition declares standard metadata a command may return.
//
// Only Pagination is declarable here. A count field would be unproducible: the
// opaque Result carries data, outcome and pagination, and exposes no way to set
// a count, so declaring one would make schema advertise a field the runtime can
// never emit.
type ResultMetaDefinition struct {
	Pagination bool
}

// OutputMode selects the framework output behavior.
type OutputMode string

const (
	// OutputGeneric uses the selected framework formatter.
	OutputGeneric OutputMode = ""
	// OutputFixedJSON always emits the standard JSON envelope.
	OutputFixedJSON OutputMode = "fixed_json"
)

type outcomeKind string

// outcomeSuccess is the only outcome a business command declares. It doubles as
// the marker that Execute produced a Result at all, which is how the host tells
// a returned result apart from the zero value accompanying an error.
const outcomeSuccess outcomeKind = "success"

// Result is an opaque command result created with Success.
type Result[Data any] struct {
	data       Data
	outcome    outcomeKind
	pagination *paginationMeta
}

// Success creates a complete successful result.
func Success[Data any](data Data) Result[Data] {
	return resultWithOutcome(data, outcomeSuccess)
}

func resultWithOutcome[Data any](data Data, outcome outcomeKind) Result[Data] {
	result := Result[Data]{data: data, outcome: outcome}
	if provider, ok := any(data).(interface{ commandPagination() *paginationMeta }); ok {
		result.pagination = clonePaginationMeta(provider.commandPagination())
	}
	return result
}
