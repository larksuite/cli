// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// ToolKind selects the One-OpenAPI endpoint and its rate-limit bucket.
//
//   - ToolKindRead  → POST .../tools/invoke_read   (scope sheets:spreadsheet:read,       10 qps)
//   - ToolKindWrite → POST .../tools/invoke_write  (scope sheets:spreadsheet:write_only,  5 qps)
type ToolKind string

const (
	ToolKindRead  ToolKind = "read"
	ToolKindWrite ToolKind = "write"
)

// toolInvokePath returns the full One-OpenAPI invoke path for the given
// spreadsheet token + tool kind. Network-free, safe in DryRun.
func toolInvokePath(token string, kind ToolKind) string {
	suffix := "invoke_read"
	if kind == ToolKindWrite {
		suffix = "invoke_write"
	}
	return fmt.Sprintf("/open-apis/sheet_ai/v2/spreadsheets/%s/tools/%s",
		validate.EncodePathSegment(token), suffix)
}

// buildToolBody constructs the One-OpenAPI request body for a tool invocation.
// `input` is serialized to a JSON string per the API contract; callers pass
// a typed Go map and never need to handle JSON encoding themselves.
func buildToolBody(toolName string, input map[string]interface{}) (map[string]interface{}, error) {
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeSDKError, "encode tool input: %v", err).WithCause(err)
	}
	return map[string]interface{}{
		"tool_name": toolName,
		"input":     string(inputJSON),
	}, nil
}

// callTool invokes a sheet-ai tool via the One-OpenAPI endpoint and decodes
// the JSON-string `output` field into a generic Go value (typically
// map[string]interface{}). When the tool returns an empty `output`, callTool
// returns nil with no error.
//
// kind must match the tool's read/write classification — passing a read tool
// to invoke_write (or vice versa) results in a 403 from the gateway.
func callTool(
	ctx context.Context,
	runtime *common.RuntimeContext,
	token string,
	kind ToolKind,
	toolName string,
	input map[string]interface{},
) (interface{}, error) {
	body, err := buildToolBody(toolName, input)
	if err != nil {
		return nil, err
	}

	data, err := runtime.CallAPITyped("POST", toolInvokePath(token, kind), nil, body)
	if err != nil {
		// A classified business error (non-zero API code) carries the tool's
		// own code and raw msg. Rewrite the typed error in place: the Message
		// gains tool context and flattenToolErrorMsg unwraps batch_update's
		// double-escaped failures payload. The classified Subtype passes
		// through untouched — rate_limit / invalid_parameters / not_found are
		// facts an agent routes on, and stamping them server_error would
		// misread them as backend faults. Only SubtypeUnknown (codes absent
		// from the code table, i.e. most of sheet-ai's own code space) is
		// pinned to SubtypeServerError, preserving callTool's long-standing
		// envelope for those. Mutating in place keeps the classifier's
		// log_id / hint / retryable, which a rebuilt error would drop.
		// Transport, HTTP-status, and auth errors are already correctly typed
		// by CallAPITyped, so they pass through untouched.
		if p, ok := errs.ProblemOf(err); ok && p.Category == errs.CategoryAPI {
			// The recovery prescription depends on the execution mode the batch
			// was sent with; non-batch tools simply lack the key (false).
			continueOnError, _ := input["continue_on_error"].(bool)
			flat := flattenToolErrorMsg(p.Message, continueOnError, callerAuthoredOperations(runtime.Command()))
			if p.Subtype == errs.SubtypeUnknown {
				p.Subtype = errs.SubtypeServerError
			}
			p.Message = fmt.Sprintf("tool %q failed: [%d] %s", toolName, p.Code, flat)
		}
		return nil, err
	}
	rawOutput, _ := data["output"].(string)
	if rawOutput == "" {
		return nil, nil
	}

	var out interface{}
	if err := json.Unmarshal([]byte(rawOutput), &out); err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"tool %q returned invalid JSON output: %v", toolName, err).WithCause(err)
	}
	return out, nil
}

// flattenToolErrorMsg unwraps the nested-escaped-JSON error payload some
// sheet-ai tools put in msg — batch_update in particular wraps its result as
// {"error":"{\"message\":\"batch_update: N succeeded, M failed\",
// \"failures\":[…]}","errorType":…,"data":{…}} — into one readable line
// naming each failed operation. Eval traces show agents (and even the eval
// aggregator) failing to extract the real cause from the double-escaped
// form. Anything that doesn't match the nested shape passes through
// untouched.
//
// continueOnError is the execution mode the batch was sent with: it decides
// the recovery prescription, because a single listed failure only implies
// "nothing after it ran" under fail-fast.
//
// callerAuthoredOps says whether the operations array the server indexes into
// is the one the CALLER wrote. Only +batch-update's --operations is; every
// other batch_update user (+styles-put, +cells-set --writes, +dim-delete
// --ranges, the fan-out stampers) synthesizes the array client-side, and
// +styles-put coalesces while +dim-delete deliberately re-sorts descending —
// so "operations[3]" there names nothing the caller can find, and
// "resend operations[3:]" is not a command they can issue. Those callers get
// the per-op detail (still the best available description of what failed) plus
// a generic no-rollback warning, never an index-based resend instruction.
func flattenToolErrorMsg(msg string, continueOnError, callerAuthoredOps bool) string {
	trimmed := strings.TrimSpace(msg)
	if !strings.HasPrefix(trimmed, "{") {
		return msg
	}
	var outer struct {
		Error string `json:"error"`
	}
	if json.Unmarshal([]byte(trimmed), &outer) != nil || strings.TrimSpace(outer.Error) == "" {
		return msg
	}
	inner := strings.TrimSpace(outer.Error)
	var detail struct {
		Message  string `json:"message"`
		Failures []struct {
			Index    int    `json:"index"`
			ToolName string `json:"tool_name"`
			Error    string `json:"error"`
		} `json:"failures"`
	}
	if strings.HasPrefix(inner, "{") && json.Unmarshal([]byte(inner), &detail) == nil && detail.Message != "" {
		if len(detail.Failures) == 0 {
			return detail.Message
		}
		parts := make([]string, 0, len(detail.Failures))
		firstFailed := detail.Failures[0].Index
		for _, f := range detail.Failures {
			parts = append(parts, fmt.Sprintf("operations[%d] (%s): %s", f.Index, f.ToolName, f.Error))
			if f.Index < firstFailed {
				firstFailed = f.Index
			}
		}
		out := detail.Message + " — " + strings.Join(parts, "; ")
		// Partial failure is NOT rolled back server-side: the succeeded sub-ops
		// stay applied. Spell out the recovery so agents don't resend the whole
		// batch and double-apply the successes (observed in eval traces). Only
		// under fail-fast does a single failure mean nothing after it ran —
		// resend from that index. Under continue-on-error the later operations
		// already executed, so even a single listed failure must be resent
		// alone; prescribing the tail there would double-apply the successes.
		if strings.Contains(detail.Message, "succeeded") &&
			!strings.Contains(detail.Message, " 0 succeeded") {
			switch {
			case !callerAuthoredOps:
				// Client-side expansion: the indexes above are internal, so
				// prescribe a read-back instead of an un-issuable resend.
				out += "; note: this command expands into the operations above client-side, so their indexes are not something you can resend directly. Succeeded operations stay applied (no rollback) — read the affected area back (+sheet-info / +cells-get), then re-issue only the part that did not land"
			case !continueOnError && len(detail.Failures) == 1:
				out += fmt.Sprintf("; note: succeeded operations stay applied (no rollback) — fix the failure and resend only operations[%d:] onward, do not resend the whole batch", firstFailed)
			default:
				out += "; note: succeeded operations stay applied (no rollback) — fix and resend only the failed operations listed above, do not resend the whole batch"
			}
		}
		return out
	}
	return inner
}

// callerAuthoredOperations reports whether `command` is the one shortcut whose
// batch_update operations array the caller wrote by hand. Everything else
// synthesizes it, so server-reported operation indexes are internal detail
// there (see flattenToolErrorMsg).
func callerAuthoredOperations(command string) bool { return command == "+batch-update" }

// invokeToolDryRun renders the One-OpenAPI request the shortcut would send.
// The wire-format body (with input serialized to a JSON string) is preserved
// for fidelity, and a decoded tool_input map is surfaced alongside so humans
// don't have to mentally unmarshal the string field.
func invokeToolDryRun(
	token string,
	kind ToolKind,
	toolName string,
	input map[string]interface{},
) *common.DryRunAPI {
	wireBody, _ := buildToolBody(toolName, input)
	return common.NewDryRunAPI().
		POST(toolInvokePath(token, kind)).
		Body(wireBody).
		Set("spreadsheet_token", token).
		Set("tool_name", toolName).
		Set("tool_input", input)
}
