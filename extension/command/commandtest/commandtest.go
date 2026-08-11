// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package commandtest supplies an isolated runtime for business command tests.
package commandtest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/larksuite/cli/extension/command"
)

// Response is one scripted OpenAPI response.
type Response struct {
	data any
	err  error
}

// Respond creates a successful scripted response containing an OpenAPI data object.
func Respond(data any) Response { return Response{data: data} }

// Fail creates a failed scripted response.
func Fail(err error) Response { return Response{err: err} }

// Recorder supplies a restricted CommandContext and records its observable activity.
type Recorder struct {
	testing testing.TB

	mu                 sync.Mutex
	responses          []Response
	requests           []command.RequestView
	scopeChecks        [][]string
	scopeError         error
	pagination         command.PaginationOptions
	cancelAfterRequest int
	cancel             context.CancelFunc
}

// New creates a Recorder with an ordered response script.
func New(testing testing.TB, responses ...Response) *Recorder {
	testing.Helper()
	return &Recorder{
		testing:    testing,
		responses:  append([]Response(nil), responses...),
		pagination: command.PaginationOptions{MaxPages: 1},
	}
}

// CommandContext returns a restricted public command context.
func (r *Recorder) CommandContext(identity command.Identity) command.CommandContext {
	return r.commandContext(identity, false)
}

// DryRunContext returns an offline context for invoking a DryRun hook.
func (r *Recorder) DryRunContext(identity command.Identity) command.CommandContext {
	return r.commandContext(identity, true)
}

func (r *Recorder) commandContext(identity command.Identity, dryRun bool) command.CommandContext {
	return command.NewCommandContext(command.ContextOptions{
		Identity:          identity,
		DryRun:            dryRun,
		CallJSON:          r.callJSON,
		PreflightScopes:   r.preflightScopes,
		PaginationOptions: r.paginationOptions,
	})
}

// Execution is the inspected outcome of one business Execute hook.
type Execution[Data any] struct {
	Data    Data
	Partial bool
}

// Execute runs Normalize, Validate, and Execute with the restricted test runtime.
func Execute[Args any, Data any](ctx context.Context, recorder *Recorder, identity command.Identity, definition command.Definition[Args, Data], args *Args) (Execution[Data], error) {
	var execution Execution[Data]
	declaration := command.InspectCommand(command.Define(definition))
	commandContext := recorder.CommandContext(identity)
	if declaration.Hooks.Normalize != nil {
		if err := declaration.Hooks.Normalize(ctx, commandContext, args); err != nil {
			return execution, err
		}
	}
	if declaration.Hooks.Validate != nil {
		if err := declaration.Hooks.Validate(ctx, commandContext, args); err != nil {
			return execution, err
		}
	}
	if declaration.Hooks.Execute == nil {
		return execution, errors.New("business command has no Execute hook")
	}
	result, err := declaration.Hooks.Execute(ctx, commandContext, args)
	if err != nil {
		return execution, err
	}
	data, ok := result.Data.(Data)
	if !ok {
		return execution, fmt.Errorf("business Execute returned %T, expected %T", result.Data, execution.Data)
	}
	return Execution[Data]{Data: data, Partial: result.Outcome == "partial"}, nil
}

// Preview runs Normalize, Validate, and DryRun with an offline test context.
func Preview[Args any, Data any](ctx context.Context, recorder *Recorder, identity command.Identity, definition command.Definition[Args, Data], args *Args) (*command.DryRun, error) {
	declaration := command.InspectCommand(command.Define(definition))
	commandContext := recorder.DryRunContext(identity)
	if declaration.Hooks.Normalize != nil {
		if err := declaration.Hooks.Normalize(ctx, commandContext, args); err != nil {
			return nil, err
		}
	}
	if declaration.Hooks.Validate != nil {
		if err := declaration.Hooks.Validate(ctx, commandContext, args); err != nil {
			return nil, err
		}
	}
	if declaration.Hooks.DryRun == nil {
		return nil, errors.New("business command has no DryRun hook")
	}
	return declaration.Hooks.DryRun(ctx, commandContext, args), nil
}

// ExecutionContext creates a cancellable context observed by scripted requests and pagination waits.
func (r *Recorder) ExecutionContext(parent context.Context) context.Context {
	ctx, cancel := context.WithCancel(parent)
	r.mu.Lock()
	if r.cancel != nil {
		r.cancel()
	}
	r.cancel = cancel
	r.mu.Unlock()
	r.testing.Cleanup(cancel)
	return ctx
}

// CancelAfterRequest injects cancellation immediately after the numbered request succeeds.
func (r *Recorder) CancelAfterRequest(number int) {
	r.testing.Helper()
	if number < 1 {
		r.testing.Fatalf("request number must be positive: %d", number)
	}
	r.mu.Lock()
	r.cancelAfterRequest = number
	r.mu.Unlock()
}

// SetPagination supplies host-owned pagination settings.
func (r *Recorder) SetPagination(options command.PaginationOptions) {
	r.mu.Lock()
	r.pagination = options
	r.mu.Unlock()
}

// SetScopeError makes every subsequent scope preflight return err after recording it.
func (r *Recorder) SetScopeError(err error) {
	r.mu.Lock()
	r.scopeError = err
	r.mu.Unlock()
}

// Requests returns copied requests in execution order.
func (r *Recorder) Requests() []command.RequestView {
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneRequestViews(r.requests)
}

// ScopeChecks returns copied scope preflights in execution order.
func (r *Recorder) ScopeChecks() [][]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	checks := make([][]string, len(r.scopeChecks))
	for index, scopes := range r.scopeChecks {
		checks[index] = append([]string(nil), scopes...)
	}
	return checks
}

// AssertScriptConsumed verifies that every scripted response was used.
func (r *Recorder) AssertScriptConsumed() {
	r.testing.Helper()
	r.mu.Lock()
	remaining := len(r.responses)
	r.mu.Unlock()
	if remaining != 0 {
		r.testing.Errorf("unused scripted responses: %d", remaining)
	}
}

// AssertDryRunMatches verifies method, path, query, body, count, and order.
func (r *Recorder) AssertDryRunMatches(dryRun *command.DryRun) {
	r.testing.Helper()
	claimed := command.InspectDryRun(dryRun).Requests
	actual := r.Requests()
	if len(claimed) != len(actual) {
		r.testing.Errorf("dry-run request count = %d, executed request count = %d", len(claimed), len(actual))
		return
	}
	for index := range claimed {
		claimedValue, err := comparableRequest(claimed[index])
		if err != nil {
			r.testing.Errorf("dry-run request %d: %v", index+1, err)
			continue
		}
		actualValue, err := comparableRequest(actual[index])
		if err != nil {
			r.testing.Errorf("executed request %d: %v", index+1, err)
			continue
		}
		if !reflect.DeepEqual(claimedValue, actualValue) {
			r.testing.Errorf("dry-run request %d differs from executed request\nclaimed: %s\nexecuted: %s", index+1, claimedValue, actualValue)
		}
	}
}

func (r *Recorder) callJSON(ctx context.Context, request command.Request) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	view := command.InspectRequest(request)
	r.mu.Lock()
	r.requests = append(r.requests, cloneRequestView(view))
	requestNumber := len(r.requests)
	if len(r.responses) == 0 {
		r.mu.Unlock()
		return nil, fmt.Errorf("request %d has no scripted response", requestNumber)
	}
	response := r.responses[0]
	r.responses = r.responses[1:]
	cancel := r.cancel
	shouldCancel := r.cancelAfterRequest == requestNumber
	r.mu.Unlock()

	if response.err != nil {
		return nil, response.err
	}
	data, err := responseDataObject(response.data)
	if err != nil {
		return nil, fmt.Errorf("scripted response %d: %w", requestNumber, err)
	}
	if shouldCancel && cancel != nil {
		cancel()
	}
	return data, nil
}

func (r *Recorder) preflightScopes(scopes ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scopeChecks = append(r.scopeChecks, append([]string(nil), scopes...))
	return r.scopeError
}

func (r *Recorder) paginationOptions() (command.PaginationOptions, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pagination, nil
}

func responseDataObject(value any) (map[string]any, error) {
	if value == nil {
		return map[string]any{}, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode data object: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var data map[string]any
	if err := decoder.Decode(&data); err != nil {
		return nil, fmt.Errorf("decode data object: %w", err)
	}
	if data == nil {
		return nil, errors.New("response data must be a JSON object")
	}
	return data, nil
}

func comparableRequest(request command.RequestView) (string, error) {
	value := struct {
		Method string         `json:"method"`
		Path   string         `json:"path"`
		Query  map[string]any `json:"query"`
		Body   any            `json:"body,omitempty"`
	}{Method: request.Method, Path: request.Path, Query: request.Query, Body: request.Body}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode comparable request: %w", err)
	}
	return string(encoded), nil
}

func cloneRequestViews(requests []command.RequestView) []command.RequestView {
	cloned := make([]command.RequestView, len(requests))
	for index, request := range requests {
		cloned[index] = cloneRequestView(request)
	}
	return cloned
}

func cloneRequestView(request command.RequestView) command.RequestView {
	encoded, err := json.Marshal(request)
	if err != nil {
		return request
	}
	var cloned command.RequestView
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return request
	}
	return cloned
}
