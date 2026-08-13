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
	"io"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/larksuite/cli/extension/command"
	internalpagination "github.com/larksuite/cli/internal/pagination"
	"github.com/spf13/pflag"
)

// Response is one scripted OpenAPI response.
type Response struct {
	data           any
	err            error
	expectedMethod string
	expectedPath   string
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

// ReplyJSON appends an ordered successful response with an expected request method and path.
func (r *Recorder) ReplyJSON(method, path string, data any) *Recorder {
	r.testing.Helper()
	r.mu.Lock()
	r.responses = append(r.responses, Response{data: data, expectedMethod: method, expectedPath: path})
	r.mu.Unlock()
	return r
}

// CommandContext returns a restricted public command context.
func (r *Recorder) CommandContext(identity command.Identity) command.CommandContext {
	return r.commandContext(identity, false)
}

// DryRunContext returns the network-free context the host gives a DryRun hook.
func (r *Recorder) DryRunContext(identity command.Identity) command.CommandContext {
	return r.commandContext(identity, true)
}

// InputStageContext returns the network-free context the host gives Normalize
// and Validate. Tests that drive those hooks directly must use it, or a command
// that calls the API before the high-risk confirmation gate passes its tests
// and fails only in production.
func (r *Recorder) InputStageContext(identity command.Identity) command.CommandContext {
	return command.NewCommandContext(command.ContextOptions{
		Identity:        identity,
		InputStage:      true,
		PreflightScopes: r.preflightScopes,
	})
}

func (r *Recorder) commandContext(identity command.Identity, dryRun bool) command.CommandContext {
	return command.NewCommandContext(command.ContextOptions{
		Identity:        identity,
		DryRun:          dryRun,
		CallJSON:        r.callJSON,
		PreflightScopes: r.preflightScopes,
		CollectPages:    r.collectPages,
	})
}

// Execution is the inspected outcome of one business Execute hook.
type Execution[Data any] struct {
	Data Data
}

// Execute runs Normalize, Validate, and Execute with the restricted test runtime.
func Execute[Args any, Data any](ctx context.Context, recorder *Recorder, identity command.Identity, definition command.Definition[Args, Data], args *Args) (Execution[Data], error) {
	var execution Execution[Data]
	declaration := command.InspectCommand(command.Define(definition))
	commandContext := recorder.CommandContext(identity)
	inputContext := recorder.InputStageContext(identity)
	if declaration.Hooks.Normalize != nil {
		if err := declaration.Hooks.Normalize(ctx, inputContext, args); err != nil {
			return execution, err
		}
	}
	if declaration.Hooks.Validate != nil {
		if err := declaration.Hooks.Validate(ctx, inputContext, args); err != nil {
			return execution, err
		}
	}
	if declaration.Hooks.Execute == nil {
		return execution, errors.New("business command has no Execute hook")
	}
	result, err := declaration.Hooks.Execute(ctx, commandContext, args)
	if err != nil {
		if result.Outcome != "" || result.Pagination != nil {
			return execution, command.InternalErrorf("business Execute returned both Result and error").WithCause(err)
		}
		return execution, err
	}
	data, ok := result.Data.(Data)
	if !ok {
		return execution, fmt.Errorf("business Execute returned %T, expected %T", result.Data, execution.Data)
	}
	return Execution[Data]{Data: data}, nil
}

// RunWithFlags executes a page-returning command with the framework's standard pagination flags.
func RunWithFlags[Args any, Data any](ctx context.Context, recorder *Recorder, identity command.Identity, definition command.Definition[Args, Data], args *Args, flags ...string) (Execution[Data], error) {
	if !command.InspectCommand(command.Define(definition)).PageOutput {
		return Execution[Data]{}, command.ValidationErrorf("framework pagination flags require a Page output")
	}
	options, err := parsePaginationFlags(flags)
	if err != nil {
		return Execution[Data]{}, err
	}
	restore := recorder.replacePagination(options)
	defer restore()
	return Execute(ctx, recorder, identity, definition, args)
}

func parsePaginationFlags(arguments []string) (command.PaginationOptions, error) {
	flags := pflag.NewFlagSet("commandtest pagination", pflag.ContinueOnError)
	flags.SetOutput(io.Discard)
	pageAll := flags.Bool("page-all", false, "")
	pageLimit := flags.Int("page-limit", 10, "")
	pageDelay := flags.Int("page-delay", 200, "")
	if err := flags.Parse(arguments); err != nil {
		return command.PaginationOptions{}, command.ValidationErrorf("parse framework pagination flags: %v", err).WithCause(err)
	}
	if flags.NArg() != 0 {
		return command.PaginationOptions{}, command.ValidationErrorf("unexpected framework pagination argument %q", flags.Arg(0))
	}
	return command.PaginationOptions{
		All:      *pageAll,
		MaxPages: *pageLimit,
		Delay:    time.Duration(*pageDelay) * time.Millisecond,
	}, nil
}

// Preview runs Normalize, Validate, and DryRun with a network-free test context.
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

func (r *Recorder) replacePagination(options command.PaginationOptions) func() {
	r.mu.Lock()
	previous := r.pagination
	r.pagination = options
	r.mu.Unlock()
	return func() {
		r.mu.Lock()
		r.pagination = previous
		r.mu.Unlock()
	}
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
	cloned, err := cloneRequestViews(r.requests)
	if err != nil {
		r.testing.Errorf("clone recorded requests: %v", err)
	}
	return cloned
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
	cloned, cloneErr := cloneRequestView(view)
	if cloneErr != nil {
		r.testing.Errorf("clone executed request: %v", cloneErr)
	}
	r.mu.Lock()
	r.requests = append(r.requests, cloned)
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
	if response.expectedMethod != "" && response.expectedMethod != view.Method {
		return nil, fmt.Errorf("request %d method = %q, expected %q", requestNumber, view.Method, response.expectedMethod)
	}
	if response.expectedPath != "" && response.expectedPath != view.Path {
		return nil, fmt.Errorf("request %d path = %q, expected %q", requestNumber, view.Path, response.expectedPath)
	}

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

func (r *Recorder) collectPages(ctx context.Context, request command.Request, all bool) ([]map[string]any, command.HostPagination, error) {
	r.mu.Lock()
	options := r.pagination
	r.mu.Unlock()
	if all {
		// Same bound as the production host adapter (commandPagePolicy), so a
		// complete-set collection that passes here cannot fail only in production.
		options = command.PaginationOptions{All: true, MaxPages: internalpagination.CollectAllHardPageBound}
	} else if !options.All {
		options.MaxPages = 1
	}
	if options.MaxPages < 1 || options.MaxPages > 1000 {
		return nil, command.HostPagination{}, command.ValidationErrorf("pagination page limit must be between 1 and 1000")
	}
	if options.Delay < 0 || options.Delay > time.Minute {
		return nil, command.HostPagination{}, command.ValidationErrorf("pagination delay must be between 0 and 60000 milliseconds")
	}

	var pages []map[string]any
	state, err := internalpagination.Walk(ctx, internalpagination.Options{
		InitialToken: requestPageToken(command.InspectRequest(request).Query),
		MaxPages:     options.MaxPages,
		Delay:        options.Delay,
		Fetch: func(ctx context.Context, _ int, token string) (bool, string, error) {
			pageRequest := request
			if token != "" {
				pageRequest = pageRequest.Set("page_token", token)
			}
			data, err := r.callJSON(ctx, pageRequest)
			if err != nil {
				return false, "", err
			}
			pages = append(pages, data)
			hasMore, _ := data["has_more"].(bool)
			nextToken, _ := data["page_token"].(string)
			if nextToken == "" {
				nextToken, _ = data["next_page_token"].(string)
			}
			return hasMore, nextToken, nil
		},
	})
	pagination := command.HostPagination{Complete: state.Complete, Pages: state.Pages, NextToken: state.NextToken}
	if err == nil {
		return pages, pagination, nil
	}
	var cursorErr *internalpagination.CursorError
	if errors.As(err, &cursorErr) {
		if cursorErr.Kind == internalpagination.CursorMissing {
			return pages, pagination, command.InvalidResponseErrorf("pagination page %d reports has_more=true without a page token", cursorErr.Page)
		}
		return pages, pagination, command.InvalidResponseErrorf("pagination page %d repeated page token %q", cursorErr.Page, cursorErr.Token)
	}
	var waitErr *internalpagination.WaitError
	if errors.As(err, &waitErr) {
		return pages, pagination, command.PaginationInterruptedError(waitErr.Err)
	}
	return pages, pagination, err
}

func requestPageToken(query map[string]any) string {
	switch value := query["page_token"].(type) {
	case string:
		return value
	case []string:
		if len(value) > 0 {
			return value[0]
		}
	case []any:
		if len(value) > 0 {
			return fmt.Sprint(value[0])
		}
	}
	return ""
}

func (r *Recorder) preflightScopes(scopes ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scopeChecks = append(r.scopeChecks, append([]string(nil), scopes...))
	return r.scopeError
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

func cloneRequestViews(requests []command.RequestView) ([]command.RequestView, error) {
	cloned := make([]command.RequestView, len(requests))
	for index, request := range requests {
		value, err := cloneRequestView(request)
		if err != nil {
			return cloned, fmt.Errorf("request %d: %w", index+1, err)
		}
		cloned[index] = value
	}
	return cloned, nil
}

func cloneRequestView(request command.RequestView) (command.RequestView, error) {
	encoded, err := json.Marshal(request)
	if err != nil {
		return request, fmt.Errorf("encode recorded request: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var cloned command.RequestView
	if err := decoder.Decode(&cloned); err != nil {
		return request, fmt.Errorf("decode recorded request: %w", err)
	}
	return cloned, nil
}
