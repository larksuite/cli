// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import "github.com/larksuite/cli/internal/output"

type OutputDefinition struct {
	Data      DataDefinition
	Outcomes  OutcomeDefinition
	Artifacts []ArtifactDefinition
	Meta      ResultMetaDefinition
	Mode      OutputMode

	// DisableHTMLEscaping preserves literal <, >, and & characters in JSON
	// envelopes and jq JSON output. It does not enable bare stdout output or
	// bypass content-safety scanning.
	DisableHTMLEscaping bool

	// Citation declares this read command's citation capability. Only
	// SourceTypes is honored on the typed path; the builder lives in
	// Hooks.BuildCitation so it can see typed *Args and Data.
	Citation *CitationDefinition
}

// ResultMetaDefinition declares which standard envelope metadata a command may
// return. It is deliberately narrower than output.Meta: rollback and arbitrary
// metadata are not part of the Typed Result contract.
type ResultMetaDefinition struct {
	Count      bool
	Pagination bool
}

// ResultPaginationMeta reuses the standard output envelope pagination contract.
// The name avoids colliding with the existing PaginationMeta response helper.
type ResultPaginationMeta = output.PaginationMeta

// ResultMeta carries optional standard envelope metadata for one Result.
// Count is a pointer so the runner can distinguish an omitted count from an
// explicitly supplied zero while preserving output.Meta's existing JSON rules.
type ResultMeta struct {
	Count      *int
	Pagination *ResultPaginationMeta
}

type Result[Data any] struct {
	Data    Data
	Outcome OutcomeKind
	Meta    *ResultMeta
}

type OutcomeKind string

const (
	OutcomeSuccess OutcomeKind = "success"
	OutcomePartial OutcomeKind = "partial"
)

func Success[Data any](data Data) Result[Data] {
	return Result[Data]{Data: data, Outcome: OutcomeSuccess}
}

func Partial[Data any](data Data) Result[Data] {
	return Result[Data]{Data: data, Outcome: OutcomePartial}
}

// WithMeta attaches standard envelope metadata to a Result.
func (result Result[Data]) WithMeta(meta ResultMeta) Result[Data] {
	result.Meta = &meta
	return result
}

// CountMeta constructs count metadata while preserving an explicit zero.
func CountMeta(count int) ResultMeta {
	return ResultMeta{Count: &count}
}

// PaginationResultMeta constructs pagination metadata.
func PaginationResultMeta(pagination *ResultPaginationMeta) ResultMeta {
	return ResultMeta{Pagination: pagination}
}

type OutcomeDefinition struct{ PartialFailure *PartialFailureDefinition }
type PartialFailureDefinition struct {
	ExitCode int
	// FailedItems declares an item-ledger receipt. Leave it nil for a
	// result-level partial failure whose recovery state lives directly in Data.
	FailedItems *FailedItemDefinition
}
type FailedItemDefinition struct {
	ItemsPath     string      `json:"items_path"`
	IdentityPaths []string    `json:"identity_paths"`
	AllItems      bool        `json:"all_items,omitempty"`
	StatePath     string      `json:"state_path,omitempty"`
	FailedValues  []JSONValue `json:"failed_values,omitempty"`
}

// ArtifactDefinition identifies file receipts in Data. It does not write,
// stat, or enforce overwrite policy for the referenced files.
type ArtifactDefinition struct {
	Name      string `json:"name"`
	ItemsPath string `json:"items_path"`
	// Optional allows ItemsPath to be absent or null when an invocation
	// legitimately produces no file. Any present receipt is still validated.
	Optional       bool   `json:"optional,omitempty"`
	PathField      string `json:"path_field"`
	MediaTypeField string `json:"media_type_field,omitempty"`
	SizeField      string `json:"size_field,omitempty"`
}

// OutputMode selects one of the output paths the Typed runner actually
// executes. Generic delegates record formats to the framework formatter and
// uses an optional pretty renderer. FixedJSON preserves Legacy Out/OutRaw
// behavior: --format remains accepted, but successful output is always a JSON
// envelope.
type OutputMode string

const (
	OutputGeneric   OutputMode = ""
	OutputFixedJSON OutputMode = "fixed_json"
)
