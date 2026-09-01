// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package command

import (
	"bytes"
	"context"
	"encoding/json"
	"time"
)

// CommandContext is an opaque, invocation-scoped set of safe host capabilities.
type CommandContext struct {
	identity        Identity
	dryRun          bool
	inputStage      bool
	callJSON        func(context.Context, Request) (map[string]any, error)
	download        func(context.Context, Request, FileTarget, DownloadOptions) (Artifact, error)
	downloadURL     func(context.Context, string, FileTarget, DownloadOptions) (Artifact, error)
	preflightScopes func(...string) error
	collectPages    func(context.Context, Request, bool) ([]map[string]any, HostPagination, error)
}

// PaginationOptions carries host-owned pagination controls to the public helpers.
// It is intended for host adapters and commandtest.
type PaginationOptions struct {
	All      bool
	MaxPages int
	Delay    time.Duration
}

// ContextOptions supplies safe callbacks when a host creates a CommandContext.
// It is intended for the lark-cli host adapter and commandtest.
type ContextOptions struct {
	Identity Identity
	DryRun   bool

	// InputStage marks a context serving Normalize or Validate. Those hooks
	// run before the high-risk confirmation gate, so the design gives them no
	// network: Validate is specified as parameter checking that issues no
	// request, and a command that reached out from there would produce remote
	// side effects the user was never asked to confirm.
	InputStage bool

	CallJSON        func(context.Context, Request) (map[string]any, error)
	Download        func(context.Context, Request, FileTarget, DownloadOptions) (Artifact, error)
	DownloadURL     func(context.Context, string, FileTarget, DownloadOptions) (Artifact, error)
	PreflightScopes func(...string) error
	CollectPages    func(context.Context, Request, bool) ([]map[string]any, HostPagination, error)
}

// NewCommandContext creates a restricted context from host callbacks.
func NewCommandContext(options ContextOptions) CommandContext {
	return CommandContext{
		identity:        options.Identity,
		dryRun:          options.DryRun,
		inputStage:      options.InputStage,
		callJSON:        options.CallJSON,
		download:        options.Download,
		downloadURL:     options.DownloadURL,
		preflightScopes: options.PreflightScopes,
		collectPages:    options.CollectPages,
	}
}

// Identity returns the selected execution identity.
func (c CommandContext) Identity() Identity { return c.identity }

// CallJSON executes one request and decodes its data object into T.
func CallJSON[T any](ctx context.Context, command CommandContext, request Request) (T, error) {
	var result T
	if err := validateRequest(request); err != nil {
		return result, err
	}
	if command.inputStage {
		return result, ValidationErrorf("network requests are unavailable in Normalize and Validate; move the call to Execute")
	}
	if command.dryRun {
		return result, ValidationErrorf("network requests are unavailable during dry-run")
	}
	if command.callJSON == nil {
		return result, InternalErrorf("command host does not provide OpenAPI requests")
	}
	data, err := command.callJSON(ctx, request)
	if err != nil {
		return result, err
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return result, InvalidResponseErrorf("encode OpenAPI response data: %v", err).WithCause(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		return result, InvalidResponseErrorf("decode OpenAPI response data: %v", err).WithCause(err)
	}
	return result, nil
}

// PreflightScopes checks declared conditional scopes before a branch starts side effects.
func PreflightScopes(command CommandContext, scopes ...string) error {
	if command.dryRun {
		return nil
	}
	if command.preflightScopes == nil {
		return InternalErrorf("command host does not provide conditional scope checks")
	}
	return command.preflightScopes(scopes...)
}
