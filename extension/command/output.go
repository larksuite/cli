// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package command

// OutputDefinition declares result formats and partial outcomes.
type OutputDefinition struct {
	Data     DataDefinition
	Outcomes OutcomeDefinition
	Meta     ResultMetaDefinition
	Mode     OutputMode

	DisableHTMLEscaping bool
}

// ResultMetaDefinition declares standard metadata a command may return.
type ResultMetaDefinition struct {
	Count      bool
	Pagination bool
}

// OutcomeDefinition declares optional non-success outcomes.
type OutcomeDefinition struct {
	PartialFailure *PartialFailureDefinition
}

// PartialFailureDefinition declares the exit code and optional failed-item receipt.
type PartialFailureDefinition struct {
	ExitCode    int
	FailedItems *FailedItemDefinition
}

// FailedItemDefinition identifies failed records in a partial result.
type FailedItemDefinition struct {
	ItemsPath     string      `json:"items_path"`
	IdentityPaths []string    `json:"identity_paths"`
	AllItems      bool        `json:"all_items,omitempty"`
	StatePath     string      `json:"state_path,omitempty"`
	FailedValues  []JSONValue `json:"failed_values,omitempty"`
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

const (
	outcomeSuccess outcomeKind = "success"
	outcomePartial outcomeKind = "partial"
)

// Result is an opaque command result created with Success or Partial.
type Result[Data any] struct {
	data       Data
	outcome    outcomeKind
	pagination *paginationMeta
}

// Success creates a complete successful result.
func Success[Data any](data Data) Result[Data] {
	return resultWithOutcome(data, outcomeSuccess)
}

// Partial creates a partial result whose completed operations remain in Data.
func Partial[Data any](data Data) Result[Data] {
	return resultWithOutcome(data, outcomePartial)
}

func resultWithOutcome[Data any](data Data, outcome outcomeKind) Result[Data] {
	result := Result[Data]{data: data, outcome: outcome}
	if provider, ok := any(data).(interface{ commandPagination() *paginationMeta }); ok {
		result.pagination = clonePaginationMeta(provider.commandPagination())
	}
	return result
}
