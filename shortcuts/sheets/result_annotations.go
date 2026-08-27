// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

// Result annotations: how a sheets shortcut reports something that is true of
// the call but is not part of the tool's own payload — an input it ignored, a
// semantic it emulated, a server-side state the request will produce.
//
// These used to be printed to stderr on the success path. That made every such
// call indistinguishable from a failure to any runner that treats non-empty
// stderr as an error (PowerShell's native-command handling, most agent
// harnesses), and it put decision-relevant facts outside the one artifact a
// caller actually parses. They now ride in the success payload instead:
// `warnings` for things the caller may need to act on, and named fields
// (`effective_operation`, `ignored_inputs`) for facts about the request.
//
// A deprecation steer is a fourth kind: it describes the CLI surface rather
// than the resource, so it gets its own `deprecation` key instead of being
// mixed into `warnings` (see annotateSheetsDeprecation).

// annotateSheetsResult attaches key/value to a tool result on its way to
// stdout.
//
// callTool returns whatever JSON the tool emitted: usually an object, but
// possibly null (empty output) or — for a tool that answers with an array or a
// scalar — a non-object. An annotation must never be silently dropped just
// because of the payload's shape, so a non-object result is wrapped as
// {"result": <original>, <key>: <value>} rather than discarded. Object results,
// which is every case in practice, are annotated in place and keep their shape.
func annotateSheetsResult(out interface{}, key string, value interface{}) interface{} {
	switch typed := out.(type) {
	case map[string]interface{}:
		typed[key] = value
		return typed
	case nil:
		return map[string]interface{}{key: value}
	default:
		return map[string]interface{}{"result": out, key: value}
	}
}

// appendSheetsWarnings adds one or more advisory messages to a tool result's
// `warnings` array. No warnings means the payload is returned untouched, so a
// clean call keeps its exact previous shape.
func appendSheetsWarnings(out interface{}, warnings []string) interface{} {
	if len(warnings) == 0 {
		return out
	}
	existing, _ := out.(map[string]interface{})
	if existing != nil {
		if prior, ok := existing["warnings"].([]string); ok {
			return annotateSheetsResult(out, "warnings", append(prior, warnings...))
		}
		if prior, ok := existing["warnings"].([]interface{}); ok {
			merged := make([]interface{}, 0, len(prior)+len(warnings))
			merged = append(merged, prior...)
			for _, w := range warnings {
				merged = append(merged, w)
			}
			return annotateSheetsResult(out, "warnings", merged)
		}
	}
	return annotateSheetsResult(out, "warnings", warnings)
}

// annotateSheetsDeprecation attaches a steer off a superseded command or flag
// spelling. An empty note leaves the payload untouched.
//
// It gets a dedicated `deprecation` key rather than joining `warnings`, which
// are about the request's effect on the sheet. The natural home is the
// envelope's meta — it is metadata about the CLI surface, not about the
// resource — but meta lives in the shared output package, and this change is
// scoped to the sheets domain. Moving it there is a one-line change once the
// shared envelope gains the field.
func annotateSheetsDeprecation(out interface{}, note string) interface{} {
	if note == "" {
		return out
	}
	return annotateSheetsResult(out, "deprecation", note)
}
