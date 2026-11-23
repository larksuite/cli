// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"github.com/larksuite/cli/extension/command"
	"github.com/larksuite/cli/internal/commandbridge"
)

// The public command model has exactly one owner: extension/command. These
// unexported aliases let the existing compiler and runner consume that model
// directly without publishing a second authoring contract from common.
type typedJSONValue = command.JSONValue
type typedCommandMetadata = command.CommandMetadata
type typedIdentity = command.Identity
type typedRisk = command.Risk
type typedAuthorizationDefinition = command.AuthorizationDefinition
type typedIdentityAuthorization = command.IdentityAuthorization
type typedConditionalScope = command.ConditionalScope
type typedScopeRequirement = command.ScopeRequirement
type typedInputDefinition = command.InputDefinition
type typedInputField = command.InputField
type typedInputDefault = command.InputDefault
type typedCLIInput = command.CLIInput
type typedFlagAlias = command.FlagAlias
type typedFlagAliasMode = command.FlagAliasMode
type typedAliasConflictPolicy = command.AliasConflictPolicy
type typedValueSource = command.ValueSource
type typedCLIEncoding = command.CLIEncoding
type typedRelation = command.Relation
type typedRelationKind = command.RelationKind
type typedPresenceMode = command.PresenceMode
type typedRelationStage = command.RelationStage
type typedRuntimeContext = commandbridge.RuntimeContext
type typedPaginationOptions = command.PaginationOptions

const (
	typedIdentityUser = command.IdentityUser
	typedIdentityBot  = command.IdentityBot

	typedRiskRead          = command.RiskRead
	typedRiskWrite         = command.RiskWrite
	typedRiskHighRiskWrite = command.RiskHighRiskWrite

	typedScopeRequired   = command.ScopeRequired
	typedScopeBestEffort = command.ScopeBestEffort

	typedAliasNormalize   = command.AliasNormalize
	typedAliasIndependent = command.AliasIndependent

	typedAliasCanonicalWins       = command.AliasCanonicalWins
	typedAliasErrorIfBoth         = command.AliasErrorIfBoth
	typedAliasTrimmedEqualOrError = command.AliasTrimmedEqualOrError

	typedSourceFlag  = command.SourceFlag
	typedSourceFile  = command.SourceFile
	typedSourceStdin = command.SourceStdin

	typedEncodingRepeated        = command.EncodingRepeated
	typedEncodingCommaOrRepeated = command.EncodingCommaOrRepeated
	typedEncodingJSON            = command.EncodingJSON

	typedRelationExactlyOne = command.RelationExactlyOne
	typedRelationAtLeastOne = command.RelationAtLeastOne
	typedRelationCoOccur    = command.RelationCoOccur
	typedRelationRequires   = command.RelationRequires
	typedRelationConflicts  = command.RelationConflicts

	typedPresenceExplicit = command.PresenceExplicit
	typedPresenceNonZero  = command.PresenceNonZero

	typedStageSourcePreRun = command.StageSourcePreRun
	typedStageAfterPrepare = command.StageAfterPrepare
)
