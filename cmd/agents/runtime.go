// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"context"
	"encoding/json"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	iagents "github.com/larksuite/cli/internal/agents"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

// cmdRuntime is the concrete iagents.Runtime: it routes provider hook calls
// through the shared client.APIClient under a pinned identity (mirrors event's
// consumeRuntime in cmd/event/runtime.go). Provider code never sees the client,
// the identity resolution, or the response-envelope unwrap — that is exactly the
// plumbing the old Deps struct leaked.
type cmdRuntime struct {
	client  *client.APIClient
	as      core.Identity
	agentID string
	params  map[string]string // validated business params (defaults backfilled)
}

func (r *cmdRuntime) AgentID() string { return r.agentID }
func (r *cmdRuntime) IsBot() bool     { return r.as == core.AsBot }

// Params returns a copy of the validated business parameters, so a hook cannot
// corrupt framework state (see the Runtime.Params contract in internal/agent).
func (r *cmdRuntime) Params() map[string]string {
	out := make(map[string]string, len(r.params))
	for k, v := range r.params {
		out[k] = v
	}
	return out
}

func (r *cmdRuntime) CallAPI(ctx context.Context, method, path string, query map[string]string, body any) (json.RawMessage, error) {
	var params map[string]interface{}
	if len(query) > 0 {
		params = make(map[string]interface{}, len(query))
		for k, v := range query {
			params[k] = v
		}
	}
	return r.do(ctx, client.RawApiRequest{Method: method, URL: path, Params: params, Data: body, As: r.as})
}

func (r *cmdRuntime) CallMultipart(ctx context.Context, method, path string, fields map[string]string, files []iagents.FilePart) (json.RawMessage, error) {
	fd := larkcore.NewFormdata()
	for k, v := range fields {
		fd.AddField(k, v)
	}
	for _, fp := range files {
		// SafeInputPath is the framework-owned security check (no path traversal /
		// outside CWD); a provider must never re-implement it.
		resolved, err := validate.SafeInputPath(fp.Path)
		if err != nil {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--file: %v", err).
				WithParam("--file").WithCause(err)
		}
		f, err := vfs.Open(resolved)
		if err != nil {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--file: cannot open %s: %v", fp.Path, err).
				WithParam("--file").WithCause(err)
		}
		// Closed when CallMultipart returns, i.e. after do()'s request has read the
		// body — deferring in the loop keeps every file open for the request.
		defer f.Close()
		fd.AddFile(fp.Field, f)
	}
	return r.do(ctx, client.RawApiRequest{
		Method: method, URL: path, Data: fd, As: r.as,
		ExtraOpts: []larkcore.RequestOptionFunc{larkcore.WithFileUpload()},
	})
}

// do is the shared DoAPI → ParseJSONResponse → CheckResponse → unwrap-"data"
// path. It returns the "data" sub-object as raw JSON (the typed Call[T]/
// CallUpload[T] helpers decode it). Identity is sealed in r.as and never handed
// out; any non-typed transport error is classified here so hooks only ever see
// typed errs.* values.
func (r *cmdRuntime) do(ctx context.Context, req client.RawApiRequest) (json.RawMessage, error) {
	resp, err := r.client.DoAPI(ctx, req)
	if err != nil {
		if _, ok := errs.ProblemOf(err); ok {
			return nil, err
		}
		return nil, errs.NewNetworkError(errs.SubtypeNetworkTransport, "api %s %s: %s", req.Method, req.URL, err).WithCause(err)
	}
	result, err := client.ParseJSONResponse(resp)
	if err != nil {
		if _, ok := errs.ProblemOf(err); ok {
			return nil, err
		}
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "api %s %s: %s", req.Method, req.URL, err).WithCause(err)
	}
	if top, ok := result.(map[string]interface{}); ok {
		topLogID, _ := top["log_id"].(string)
		var nestedLogID string
		if errBlock, ok := top["error"].(map[string]interface{}); ok {
			nestedLogID, _ = errBlock["log_id"].(string)
		}
		for _, candidate := range []string{
			topLogID,
			nestedLogID,
			resp.Header.Get(larkcore.HttpHeaderKeyLogId),
			resp.Header.Get(larkcore.HttpHeaderKeyRequestId),
		} {
			if logID := strings.TrimSpace(candidate); logID != "" {
				// CheckResponse classifies errors from the top-level envelope, so
				// always promote the selected ID there in its normalized form.
				top["log_id"] = logID
				break
			}
		}
	}
	if apiErr := r.client.CheckResponse(result, r.as); apiErr != nil {
		return nil, apiErr
	}
	top, _ := result.(map[string]interface{})
	dataVal, ok := top["data"]
	if !ok || dataVal == nil {
		return nil, nil // no "data" (e.g. a pure write) — callers get the zero value
	}
	raw, err := json.Marshal(dataVal)
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "api %s %s: re-encode data: %s", req.Method, req.URL, err).WithCause(err)
	}
	return raw, nil
}
