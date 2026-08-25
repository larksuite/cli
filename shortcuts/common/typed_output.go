// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"github.com/larksuite/cli/extension/command"
	"github.com/larksuite/cli/internal/output"
)

type typedOutputDefinition = command.OutputDefinition
type typedResultMetaDefinition = command.ResultMetaDefinition
type typedOutputMode = command.OutputMode
type typedResultPaginationMeta = output.PaginationMeta

// typedResultMeta is the runner-owned receipt projected from the opaque public
// Result. V1 can produce pagination only; count, partial outcomes, and artifact
// receipt declarations were unreachable from extension/command and are not
// carried by the private runtime model.
type typedResultMeta struct {
	Pagination *typedResultPaginationMeta
}

type typedOutcomeKind string

const (
	typedOutcomeSuccess  typedOutcomeKind = "success"
	typedOutputGeneric   typedOutputMode  = command.OutputGeneric
	typedOutputFixedJSON typedOutputMode  = command.OutputFixedJSON
)
