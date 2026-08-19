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
	"github.com/larksuite/cli/internal/commandhost"
	internalpagination "github.com/larksuite/cli/internal/pagination"
	"github.com/spf13/pflag"
)

// Response is one scripted OpenAPI response.
type Response struct {
	data           any
	file           *fileResponse
	err            error
	expectedMethod string
	expectedPath   string
	expectedURL    string
}

type fileResponse struct {
	contentType string
	content     []byte
}

// Respond creates a successful scripted response containing an OpenAPI data object.
func Respond(data any) Response { return Response{data: data} }

// RespondFile creates a successful scripted file response.
func RespondFile(contentType string, content []byte) Response {
	return Response{file: &fileResponse{contentType: contentType, content: append([]byte(nil), content...)}}
}

// Fail creates a failed scripted response.
func Fail(err error) Response { return Response{err: err} }

// Recorder supplies a restricted CommandContext and records its observable activity.
type Recorder struct {
	testing testing.TB

	mu                 sync.Mutex
	responses          []Response
	requests           []command.RequestView
	urls               []string
	files              []RecordedFile
	operations         int
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

// ReplyFile appends an ordered successful file response with an expected
// request method and path.
func (r *Recorder) ReplyFile(method, path, contentType string, content []byte) *Recorder {
	r.testing.Helper()
	r.mu.Lock()
	r.responses = append(r.responses, Response{
		file:           &fileResponse{contentType: contentType, content: append([]byte(nil), content...)},
		expectedMethod: method,
		expectedPath:   path,
	})
	r.mu.Unlock()
	return r
}

// ReplyURL appends an ordered successful direct-URL file response.
func (r *Recorder) ReplyURL(rawURL, contentType string, content []byte) *Recorder {
	r.testing.Helper()
	r.mu.Lock()
	r.responses = append(r.responses, Response{
		file:        &fileResponse{contentType: contentType, content: append([]byte(nil), content...)},
		expectedURL: rawURL,
	})
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
		Download:        r.download,
		DownloadURL:     r.downloadURL,
		PreflightScopes: r.preflightScopes,
		CollectPages:    r.collectPages,
	})
}

// Execution is the inspected outcome of one business Execute hook.
type Execution[Data any] struct {
	Data Data
}

// RecordedFile is one scripted download committed by the test runtime.
type RecordedFile struct {
	Target    command.FileTarget
	Options   command.DownloadOptions
	SourceURL string
	Artifact  command.Artifact
	Content   []byte
}

// Execute runs Normalize, Validate, and Execute with the restricted test runtime.
// compileForTest runs the production compiler and returns the erased view every
// entry point drives. Sharing it is what keeps a harness entry from silently
// skipping the contract checks a real CLI mount performs.
func compileForTest[Args any, Data any](definition command.Definition[Args, Data]) (command.HostDefinition, error) {
	declaration := command.Define(definition)
	if err := commandhost.ValidateDeclaration(declaration); err != nil {
		return command.HostDefinition{}, err
	}
	return command.InspectCommand(declaration), nil
}

func Execute[Args any, Data any](ctx context.Context, recorder *Recorder, identity command.Identity, definition command.Definition[Args, Data], args *Args) (Execution[Data], error) {
	var execution Execution[Data]
	declaration, err := compileForTest(definition)
	if err != nil {
		return execution, err
	}
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
	// The same protocol the host adapter enforces. Without it a zero-value
	// Result passes here -- generic erasure leaves a correctly typed zero Data
	// behind, so the type assertion below succeeds and the test reports success
	// for a command every real invocation would fail.
	if err := command.ValidateHostResult(declaration, result); err != nil {
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
	declaration, err := compileForTest(definition)
	if err != nil {
		return Execution[Data]{}, err
	}
	if !declaration.PageOutput {
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
	declaration, err := compileForTest(definition)
	if err != nil {
		return nil, err
	}
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

// Files returns copied file downloads in execution order.
func (r *Recorder) Files() []RecordedFile {
	r.mu.Lock()
	defer r.mu.Unlock()
	files := make([]RecordedFile, len(r.files))
	for index, file := range r.files {
		files[index] = file
		files[index].Content = append([]byte(nil), file.Content...)
	}
	return files
}

// URLs returns copied direct download URLs in execution order.
func (r *Recorder) URLs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.urls...)
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
	claimedFiles := command.InspectDryRun(dryRun).Files
	actualFiles := r.Files()
	if len(claimedFiles) != len(actualFiles) {
		r.testing.Errorf("dry-run file count = %d, executed download count = %d", len(claimedFiles), len(actualFiles))
		return
	}
	for index := range claimedFiles {
		if claimedFiles[index].Name != actualFiles[index].Target.Name || claimedFiles[index].IfExists != actualFiles[index].Target.IfExists {
			r.testing.Errorf("dry-run file %d target = %#v, executed target = %#v", index+1, claimedFiles[index], actualFiles[index].Target)
		}
	}
}

func (r *Recorder) callJSON(ctx context.Context, request command.Request) (map[string]any, error) {
	response, requestNumber, finish, err := r.nextResponse(ctx, request)
	if err != nil {
		return nil, err
	}
	if response.err != nil {
		return nil, response.err
	}
	if response.file != nil {
		return nil, fmt.Errorf("scripted response %d is a file, not a JSON data object", requestNumber)
	}
	data, err := responseDataObject(response.data)
	if err != nil {
		return nil, fmt.Errorf("scripted response %d: %w", requestNumber, err)
	}
	finish()
	return data, nil
}

func (r *Recorder) download(ctx context.Context, request command.Request, target command.FileTarget, options command.DownloadOptions) (command.Artifact, error) {
	response, requestNumber, finish, err := r.nextResponse(ctx, request)
	if err != nil {
		return command.Artifact{}, err
	}
	if response.err != nil {
		return command.Artifact{}, response.err
	}
	if response.file == nil {
		return command.Artifact{}, fmt.Errorf("scripted response %d is JSON, not a file", requestNumber)
	}
	artifact := command.Artifact{
		Name: target.Name, Location: target.Name,
		Size: int64(len(response.file.content)), ContentType: response.file.contentType,
	}
	r.mu.Lock()
	r.files = append(r.files, RecordedFile{
		Target: target, Options: options, Artifact: artifact, Content: append([]byte(nil), response.file.content...),
	})
	r.mu.Unlock()
	finish()
	return artifact, nil
}

func (r *Recorder) downloadURL(ctx context.Context, rawURL string, target command.FileTarget, options command.DownloadOptions) (command.Artifact, error) {
	response, requestNumber, finish, err := r.nextURLResponse(ctx, rawURL)
	if err != nil {
		return command.Artifact{}, err
	}
	if response.err != nil {
		return command.Artifact{}, response.err
	}
	if response.file == nil {
		return command.Artifact{}, fmt.Errorf("scripted response %d is JSON, not a file", requestNumber)
	}
	artifact := command.Artifact{
		Name: target.Name, Location: target.Name,
		Size: int64(len(response.file.content)), ContentType: response.file.contentType,
	}
	r.mu.Lock()
	r.files = append(r.files, RecordedFile{
		Target: target, Options: options, SourceURL: rawURL,
		Artifact: artifact, Content: append([]byte(nil), response.file.content...),
	})
	r.mu.Unlock()
	finish()
	return artifact, nil
}

func (r *Recorder) nextResponse(ctx context.Context, request command.Request) (Response, int, func(), error) {
	if err := ctx.Err(); err != nil {
		return Response{}, 0, nil, err
	}
	view := command.InspectRequest(request)
	cloned, cloneErr := cloneRequestView(view)
	if cloneErr != nil {
		r.testing.Errorf("clone executed request: %v", cloneErr)
	}
	r.mu.Lock()
	r.requests = append(r.requests, cloned)
	r.operations++
	requestNumber := r.operations
	if len(r.responses) == 0 {
		r.mu.Unlock()
		return Response{}, 0, nil, fmt.Errorf("request %d has no scripted response", requestNumber)
	}
	response := r.responses[0]
	r.responses = r.responses[1:]
	cancel := r.cancel
	shouldCancel := r.cancelAfterRequest == requestNumber
	r.mu.Unlock()
	if response.expectedMethod != "" && response.expectedMethod != view.Method {
		return Response{}, 0, nil, fmt.Errorf("request %d method = %q, expected %q", requestNumber, view.Method, response.expectedMethod)
	}
	if response.expectedPath != "" && response.expectedPath != view.Path {
		return Response{}, 0, nil, fmt.Errorf("request %d path = %q, expected %q", requestNumber, view.Path, response.expectedPath)
	}
	finish := func() {
		if shouldCancel && cancel != nil {
			cancel()
		}
	}
	return response, requestNumber, finish, nil
}

func (r *Recorder) nextURLResponse(ctx context.Context, rawURL string) (Response, int, func(), error) {
	if err := ctx.Err(); err != nil {
		return Response{}, 0, nil, err
	}
	r.mu.Lock()
	r.urls = append(r.urls, rawURL)
	r.operations++
	requestNumber := r.operations
	if len(r.responses) == 0 {
		r.mu.Unlock()
		return Response{}, 0, nil, fmt.Errorf("request %d has no scripted response", requestNumber)
	}
	response := r.responses[0]
	r.responses = r.responses[1:]
	cancel := r.cancel
	shouldCancel := r.cancelAfterRequest == requestNumber
	r.mu.Unlock()
	if response.expectedURL != "" && response.expectedURL != rawURL {
		return Response{}, 0, nil, fmt.Errorf("request %d URL = %q, expected %q", requestNumber, rawURL, response.expectedURL)
	}
	finish := func() {
		if shouldCancel && cancel != nil {
			cancel()
		}
	}
	return response, requestNumber, finish, nil
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
