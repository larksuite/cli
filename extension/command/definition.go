// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package command defines the public contract for build-time command extensions.
//
// Business command authors use Definition, Define, the CommandContext helpers,
// and high-level effects such as Download. The Host* types, InspectCommand,
// InspectDomain and CloneSets are the
// erased read side that lark-cli's host adapter and commandtest consume. They
// stay exported because a Command holds its declaration unexported and Go gives
// a sibling package no way to reach it; business commands never call them.
package command

import (
	"context"
	"io"
)

// JSONValue is a value representable by JSON encoding.
type JSONValue = any

// Definition declares one typed command extension.
type Definition[Args any, Data any] struct {
	Metadata CommandMetadata
	Input    InputDefinition
	Output   OutputDefinition
	Hooks    Hooks[Args, Data]
}

// CommandMetadata describes the command name, help, risk, and authorization.
type CommandMetadata struct {
	Service       DomainName
	Command       string
	Description   string
	Risk          Risk
	Hidden        bool
	Authorization AuthorizationDefinition
}

// Identity selects a supported Lark identity.
type Identity string

// Risk classifies the side effects of a command.
type Risk string

const (
	// IdentityUser executes with a user access token.
	IdentityUser Identity = "user"
	// IdentityBot executes with a tenant access token.
	IdentityBot Identity = "bot"

	// RiskRead declares a read-only command.
	RiskRead Risk = "read"
	// RiskWrite declares a command that changes remote state.
	RiskWrite Risk = "write"
	// RiskHighRiskWrite declares a command that requires explicit confirmation.
	RiskHighRiskWrite Risk = "high-risk-write"
)

// AuthorizationDefinition declares supported identities and their scopes.
type AuthorizationDefinition struct {
	Identities    map[Identity]IdentityAuthorization
	IdentityOrder []Identity
}

// IdentityAuthorization declares required and conditional scopes for one identity.
type IdentityAuthorization struct {
	RequiredScopes    []string           `json:"required_scopes"`
	ConditionalScopes []ConditionalScope `json:"conditional_scopes"`
}

// ConditionalScope describes scopes required by only some execution branches.
type ConditionalScope struct {
	Scopes      []string         `json:"scopes"`
	When        string           `json:"when,omitempty"`
	Params      []string         `json:"params,omitempty"`
	Requirement ScopeRequirement `json:"requirement"`
}

// ScopeRequirement defines whether a conditional scope is mandatory.
type ScopeRequirement string

const (
	// ScopeRequired fails the selected execution branch when the scope is absent.
	ScopeRequired ScopeRequirement = "required"
	// ScopeBestEffort allows the primary operation to continue without the scope.
	ScopeBestEffort ScopeRequirement = "best_effort"
)

// InputDefinition supplements tags on Args with aliases, sources, and relations.
type InputDefinition struct {
	Fields    []InputField
	Relations []Relation
}

// InputField supplements one field declared by a flag tag.
type InputField struct {
	Name        string
	Description string
	Shape       ValueShape
	Default     InputDefault
	CLI         CLIInput
}

// InputDefault distinguishes an omitted default from a JSON zero value.
type InputDefault struct {
	Set   bool
	Value JSONValue
}

// CLIInput controls aliases, accepted value sources, encoding, and help visibility.
type CLIInput struct {
	Aliases      []FlagAlias
	ValueSources []ValueSource
	Encoding     CLIEncoding
	Hidden       bool
	Deprecated   string
}

// FlagAlias declares a compatibility spelling for a flag.
type FlagAlias struct {
	Name       string
	Mode       FlagAliasMode
	Conflict   AliasConflictPolicy
	Hidden     bool
	Deprecated bool
}

// FlagAliasMode defines whether an alias normalizes into the canonical flag.
type FlagAliasMode string

// AliasConflictPolicy defines how canonical and alias values interact.
type AliasConflictPolicy string

const (
	// AliasNormalize maps the alias value to the canonical field.
	AliasNormalize FlagAliasMode = "normalize"
	// AliasIndependent keeps the alias as a separate compatibility input.
	AliasIndependent FlagAliasMode = "independent"

	// AliasCanonicalWins prefers the canonical flag when both spellings appear.
	AliasCanonicalWins AliasConflictPolicy = "canonical_wins"
	// AliasErrorIfBoth rejects simultaneous canonical and alias values.
	AliasErrorIfBoth AliasConflictPolicy = "error_if_both"
	// AliasTrimmedEqualOrError accepts both spellings only when trimmed values match.
	AliasTrimmedEqualOrError AliasConflictPolicy = "trimmed_equal_or_error"
)

// ValueSource identifies where a CLI input value may come from.
type ValueSource string

const (
	// SourceFlag accepts a literal flag value.
	SourceFlag ValueSource = "flag"
	// SourceFile accepts an @path value and substitutes the file content. The
	// path goes through the invocation's FileIO provider, so it stays relative
	// to the working directory; @@ passes a literal leading @.
	SourceFile ValueSource = "file"
	// SourceStdin accepts a single dash and reads standard input. A process has
	// one stdin, so at most one flag per invocation may use it -- declare
	// SourceFile alongside it to keep the remaining values passable.
	SourceStdin ValueSource = "stdin"
)

// CLIEncoding defines how repeated or structured values are parsed.
type CLIEncoding string

const (
	// EncodingRepeated accepts repeated flag occurrences.
	EncodingRepeated CLIEncoding = "repeated"
	// EncodingCommaOrRepeated accepts comma-separated or repeated values.
	EncodingCommaOrRepeated CLIEncoding = "comma_or_repeated"
	// EncodingJSON accepts a JSON-encoded value.
	EncodingJSON CLIEncoding = "json"
)

// Provided preserves whether the caller explicitly supplied a value.
type Provided[T any] struct {
	Value T
	Set   bool
}

// Relation declares a presence relationship among input fields.
type Relation struct {
	Kind     RelationKind  `json:"kind"`
	Params   []string      `json:"params"`
	Presence PresenceMode  `json:"presence"`
	Stage    RelationStage `json:"stage"`
}

// RelationKind identifies the relationship among fields.
type RelationKind string

// PresenceMode defines how a field counts as present.
type PresenceMode string

// RelationStage selects when a relation is checked.
type RelationStage string

const (
	// RelationExactlyOne requires exactly one field.
	RelationExactlyOne RelationKind = "exactly_one"
	// RelationAtLeastOne requires one or more fields.
	RelationAtLeastOne RelationKind = "at_least_one"
	// RelationCoOccur requires all named fields to appear together.
	RelationCoOccur RelationKind = "co_occur"
	// RelationRequires makes the first field require the remaining fields.
	RelationRequires RelationKind = "requires"
	// RelationConflicts rejects fields used together.
	RelationConflicts RelationKind = "conflicts"

	// PresenceExplicit counts only values supplied by the caller.
	PresenceExplicit PresenceMode = "explicit"
	// PresenceNonZero counts non-zero values after normalization.
	PresenceNonZero PresenceMode = "non_zero"

	// StageSourcePreRun checks source presence before hooks run.
	StageSourcePreRun RelationStage = "source_pre_run"
	// StageAfterPrepare checks prepared values after Normalize.
	StageAfterPrepare RelationStage = "after_prepare"
)

// Hooks contains the optional preparation hooks and required Execute hook.
type Hooks[Args any, Data any] struct {
	// Normalize folds legacy input spellings into current semantics. It runs
	// first and only when a legacy form exists.
	//
	// Normalize and Validate both run before the high-risk confirmation gate,
	// so their CommandContext carries no network: CallJSON and CollectPages
	// refuse there. A check that needs the API belongs in Execute, after the
	// user has confirmed.
	Normalize func(context.Context, CommandContext, *Args) error

	// Validate checks format, range and field combinations. Requirements the
	// schema tag already states are enforced by the framework; this hook is for
	// the rules a tag cannot express. It issues no request -- see Normalize.
	Validate func(context.Context, CommandContext, *Args) error

	// DryRun returns the requests the command would send, which the framework
	// prints instead of executing. Validate has already run, so the requests
	// follow from Args alone and the hook reports nothing back.
	DryRun func(context.Context, CommandContext, *Args) *DryRun

	// Execute carries the business logic and returns Success. It is
	// the only hook that may call the API, and it must not write to stdout --
	// the framework owns the envelope, format and exit code.
	Execute func(context.Context, CommandContext, *Args) (Result[Data], error)

	// PrettyRenderer customizes --format pretty. It is a single hook rather than
	// a map keyed by format name because pretty is the only format a business
	// command may render itself: JSON, table, CSV and NDJSON are produced by the
	// framework formatters, and a map would let a command declare an entry the
	// compiler can only reject.
	PrettyRenderer Renderer[Data]
}

// Renderer renders one successful result in a supported custom format.
type Renderer[Data any] func(io.Writer, Data) error

// prettyFormatName is the only format a business command may render itself. The
// host hook set is keyed by format name, so PrettyRenderer is projected under
// this key.
const prettyFormatName = "pretty"

// Command is an immutable typed command declaration returned by Define.
type Command struct {
	definition hostDefinition
}

// Define captures a typed command declaration for later host compilation.
func Define[Args any, Data any](definition Definition[Args, Data]) Command {
	return newCommand(definition)
}
