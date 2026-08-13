// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"context"
	"encoding/json"

	"github.com/larksuite/cli/errs"
)

// Runtime is the only thing a verb hook touches for I/O. It is the agent
// analogue of the event/shortcut runtime: the framework has already resolved and
// PINNED the calling identity (user|bot) inside it, so a hook never sees a raw
// *client.APIClient, never resolves a token, and cannot bypass scope preflight.
// The concrete implementation lives in cmd/agent (like event's consumeRuntime in
// cmd/event), which is why internal/agent no longer needs to depend on
// internal/client — the sole reason the old Deps struct existed.
type Runtime interface {
	// AgentID is the agent this call addresses (parsed from the ref by the
	// framework). A catalog hook may ignore it; an instance template reads it to
	// know which runtime agent it serves. Request data, not plumbing.
	AgentID() string

	// IsBot reports the resolved identity kind for the rare hook that must branch
	// on it. Identity resolution itself stays hidden.
	IsBot() bool

	// Params returns this call's validated business parameters (a copy — a hook
	// cannot corrupt framework state). Contract, guaranteed on every verb
	// command path BEFORE a handler runs: the current operation's Required keys
	// are present and non-empty; keys with a Default are present (backfilled);
	// every value passed Type/Enum/Min-Max validation — handlers read directly,
	// no re-validation, no nil-checking. The card Describe path and a ListAgents
	// call without ListParams declarations see an empty map. Prefer the typed
	// accessors (BindParams[T] / ParamInt / ParamBool) over raw map lookups.
	Params() map[string]string

	// CallAPI issues one JSON OAPI request under the pinned identity and returns
	// the raw "data" object (the response envelope's data field, already unwrapped
	// and error-checked) or a typed errs.* error — a hook never does envelope
	// unwrapping, identity threading, or error classification. Hooks do not use
	// this directly; they call the typed Call[T] helper below, which decodes the
	// raw bytes into a struct. query values are strings (page_token, *_id_type, …).
	CallAPI(ctx context.Context, method, path string, query map[string]string, body any) (json.RawMessage, error)

	// CallMultipart is the file-upload seam: it reproduces the multipart form
	// upload a real provider would otherwise hand-write (larkcore.NewFormdata +
	// WithFileUpload), but centralized and identity-opaque. The framework
	// SafeInputPath-validates and opens each FilePart.Path, builds the multipart
	// body, pins the identity, and returns the raw "data" object (decode it with
	// the typed CallUpload[T] helper below). This is what makes the FileInput
	// capability actually deliverable — without it a provider declaring
	// FileInput=true but with only a JSON client would silently drop SendInput.Files.
	CallMultipart(ctx context.Context, method, path string, fields map[string]string, files []FilePart) (json.RawMessage, error)
}

// Call issues a JSON OAPI request under rt's pinned identity and decodes the
// response "data" object into T. This is the typed entry point a verb hook uses
// instead of poking at a map[string]any: declare the response struct you expect
// and let the framework unmarshal and classify errors. For a genuinely dynamic
// shape, use Call[map[string]any]. A response with no "data" (e.g. a pure write)
// yields the zero value of T and a nil error.
//
//	type chat struct{ SessionID, AgentChatID string }
//	c, err := agent.Call[chat](ctx, rt, "POST", path, nil, body)
func Call[T any](ctx context.Context, rt Runtime, method, path string, query map[string]string, body any) (T, error) {
	raw, err := rt.CallAPI(ctx, method, path, query, body)
	if err != nil {
		var zero T
		return zero, err
	}
	return decodeData[T](method, path, raw)
}

// CallUpload is the multipart (file-upload) variant of Call: it uploads files
// and decodes the response "data" object into T.
func CallUpload[T any](ctx context.Context, rt Runtime, method, path string, fields map[string]string, files []FilePart) (T, error) {
	raw, err := rt.CallMultipart(ctx, method, path, fields, files)
	if err != nil {
		var zero T
		return zero, err
	}
	return decodeData[T](method, path, raw)
}

// decodeData unmarshals a raw "data" object into T, classifying a decode failure
// as a typed invalid_response error (consistent with the runtime's own error
// handling). Empty raw ⇒ zero value, nil error.
func decodeData[T any](method, path string, raw json.RawMessage) (T, error) {
	var out T
	if len(raw) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"decode data for %s %s: %v", method, path, err).WithCause(err)
	}
	return out, nil
}

// FilePart is one file to upload. Path comes straight from SendInput.Files and is
// SafeInputPath-validated by the runtime (the security check stays framework-
// owned, not re-implemented per provider).
type FilePart struct {
	Field string // multipart field name, e.g. "file"
	Path  string // local path (framework validates + opens)
}
