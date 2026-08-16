// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"io"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/core"
)

// JSONValue is a value representable by JSON encoding.
type JSONValue = any

// Definition is the single source of truth for a Typed Shortcut.
// See TYPED_SHORTCUTS.md for the framework contract and migration guide.
type Definition[Args any, Data any] struct {
	Metadata CommandMetadata
	Input    InputDefinition
	Output   OutputDefinition
	Hooks    Hooks[Args, Data]
}

type CommandMetadata struct {
	Service       string
	Command       string
	Description   string
	Risk          Risk
	Hidden        bool
	Tips          []string
	Authorization AuthorizationDefinition
}

type Identity string
type Risk string

const (
	IdentityUser Identity = "user"
	IdentityBot  Identity = "bot"

	RiskRead          Risk = "read"
	RiskWrite         Risk = "write"
	RiskHighRiskWrite Risk = "high-risk-write"
)

type AuthorizationDefinition struct {
	Identities    map[Identity]IdentityAuthorization
	IdentityOrder []Identity // optional CLI compatibility order; must contain each declared identity exactly once
}

type IdentityAuthorization struct {
	RequiredScopes    []string           `json:"required_scopes"`
	ConditionalScopes []ConditionalScope `json:"conditional_scopes"`
}

type ConditionalScope struct {
	Scopes      []string         `json:"scopes"`
	When        string           `json:"when,omitempty"`
	Params      []string         `json:"params,omitempty"`
	Requirement ScopeRequirement `json:"requirement"`
}

type ScopeRequirement string

const (
	ScopeRequired   ScopeRequirement = "required"
	ScopeBestEffort ScopeRequirement = "best_effort"
)

type InputDefinition struct {
	Fields    []InputField
	Relations []Relation
}

type InputField struct {
	Name        string
	Description string
	Shape       ValueShape
	Default     InputDefault
	CLI         CLIInput
}

type InputDefault struct {
	Set   bool
	Value JSONValue
}

type CLIInput struct {
	Aliases      []FlagAlias
	ValueSources []ValueSource
	Encoding     CLIEncoding
	Hidden       bool   // compatibility-only primary flags omitted from default Help
	Deprecated   string // optional Cobra deprecation message for a primary flag
}

type FlagAlias struct {
	Name       string
	Mode       FlagAliasMode
	Conflict   AliasConflictPolicy
	Hidden     bool
	Deprecated bool
}

type FlagAliasMode string
type AliasConflictPolicy string

const (
	AliasNormalize   FlagAliasMode = "normalize"
	AliasIndependent FlagAliasMode = "independent"

	AliasCanonicalWins       AliasConflictPolicy = "canonical_wins"
	AliasErrorIfBoth         AliasConflictPolicy = "error_if_both"
	AliasTrimmedEqualOrError AliasConflictPolicy = "trimmed_equal_or_error"
)

type ValueSource string

const (
	SourceFlag  ValueSource = "flag"
	SourceFile  ValueSource = "file"
	SourceStdin ValueSource = "stdin"
)

type CLIEncoding string

const (
	EncodingRepeated        CLIEncoding = "repeated"
	EncodingCommaOrRepeated CLIEncoding = "comma_or_repeated"
	EncodingJSON            CLIEncoding = "json"
)

// Provided preserves whether the caller explicitly supplied a value.
type Provided[T any] struct {
	Value T
	Set   bool
}

type Relation struct {
	Kind     RelationKind  `json:"kind"`
	Params   []string      `json:"params"`
	Presence PresenceMode  `json:"presence"`
	Stage    RelationStage `json:"stage"`
}

type RelationKind string
type PresenceMode string
type RelationStage string

const (
	RelationExactlyOne RelationKind = "exactly_one"
	RelationAtLeastOne RelationKind = "at_least_one"
	RelationCoOccur    RelationKind = "co_occur"
	RelationRequires   RelationKind = "requires"
	RelationConflicts  RelationKind = "conflicts"

	PresenceExplicit PresenceMode = "explicit"
	PresenceNonZero  PresenceMode = "non_zero"

	StageSourcePreRun RelationStage = "source_pre_run"
	StageAfterPrepare RelationStage = "after_prepare"
)

type Hooks[Args any, Data any] struct {
	Normalize func(context.Context, CommandContext, *Args) error
	Validate  func(context.Context, CommandContext, *Args) error
	DryRun    func(context.Context, CommandContext, *Args) *DryRunAPI
	Execute   func(context.Context, CommandContext, *Args) (Result[Data], error)
	Renderers map[string]Renderer[Data]

	// BuildCitation builds this result's citations from the final output and
	// the bound args. It runs only when the gate is on and the result goes
	// through an envelope; it must not call any API and must not fail.
	BuildCitation func(context.Context, CommandContext, *Args, Data) []citation.Citation
}

type Renderer[Data any] func(io.Writer, Data) error

// CommandContext exposes only runtime capabilities available to Typed hooks.
type CommandContext interface {
	Identity() Identity
	Config() core.CliConfig
	APIClient() (*client.APIClient, error)
	FileIO() fileio.FileIO
	InputResolvedFromSource(param string) bool
	ValidatePath(path string) error
	ResolveSavePath(path string) (string, error)
	Stderr() io.Writer
	StartSpinner(label string) func()
	PresentError(err error) error

	// RequireConditionalScopes checks scopes that the Definition declares as
	// path-dependent for the selected identity. Domain code calls it only after
	// it has determined that the path requiring those scopes will execute.
	RequireConditionalScopes(scopes ...string) error
}
