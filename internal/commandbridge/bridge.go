// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package commandbridge owns the internal, type-erased handoff between the
// public command authoring contract, its host adapter, and the existing
// shortcut runner.
package commandbridge

import (
	"context"
	"io"
	"reflect"

	"github.com/larksuite/cli/extension/command"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/core"
)

// Access seals the small exported handshake required by shortcuts/common.
// Packages outside this module cannot import an internal package, so these
// functions cannot become a second business-authoring surface.
type Access struct{}

// RuntimeContext is the restricted host capability set consumed by erased
// hooks. It is internal implementation ABI, not an authoring contract.
type RuntimeContext interface {
	Identity() command.Identity
	Config() core.CliConfig
	APIClient() (*client.APIClient, error)
	FileIO() fileio.FileIO
	InputResolvedFromSource(param string) bool
	ValidatePath(path string) error
	ResolveSavePath(path string) (string, error)
	Stderr() io.Writer
	StartSpinner(label string) func()
	PresentError(err error) error
	IsDryRun() bool
	PaginationOptions() (command.PaginationOptions, error)
	RequireConditionalScopes(scopes ...string) error
}

// Hooks is the type-erased hook set captured while Args and Data are known.
type Hooks struct {
	NewArgs   func() any
	Normalize func(context.Context, RuntimeContext, any) error
	Validate  func(context.Context, RuntimeContext, any) error
	DryRun    func(context.Context, RuntimeContext, any) (any, error)
	Execute   func(context.Context, RuntimeContext, any) (Result, error)
	Renderers map[string]func(io.Writer, any) error
}

// Result is the erased successful result returned to the shortcut runner.
type Result struct {
	Data       any
	Outcome    string
	Pagination *command.HostPagination
}

// Definition carries the one public declaration into the private compiler.
// Metadata, Input, and Output retain extension/command as their sole owner;
// the compiler lowers them into its private executable representation.
type Definition struct {
	Metadata   command.CommandMetadata
	Input      command.InputDefinition
	Output     command.OutputDefinition
	ArgsType   reflect.Type
	DataType   reflect.Type
	Hooks      Hooks
	PageOutput bool
}
