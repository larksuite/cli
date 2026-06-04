// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
)

// TestHandleLoginScopeIssue_FailedJSON_PreservesScopeTriple asserts that the
// failed-login JSON branch (loginSucceeded == false, opts.JSON == true) wires
// requested + granted + missing scopes into the typed *PermissionError
// envelope. Consumers need the full triple to render actionable diagnostics,
// not just the missing set.
func TestHandleLoginScopeIssue_FailedJSON_PreservesScopeTriple(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	requested := []string{"docx:document", "im:message:send"}
	granted := []string{"docx:document"}
	missing := []string{"im:message:send"}

	err := handleLoginScopeIssue(
		&LoginOptions{JSON: true},
		getLoginMsg("en"),
		f,
		&loginScopeIssue{
			Message: "scope insufficient",
			Hint:    "re-login with --scope im:message:send",
			Summary: &loginScopeSummary{
				Requested: requested,
				Granted:   granted,
				Missing:   missing,
			},
		},
		"", // openId empty -> loginSucceeded = false
		"tester",
		nil, // no holder warning
	)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var permErr *errs.PermissionError
	if !errors.As(err, &permErr) {
		t.Fatalf("expected *errs.PermissionError, got %T: %v", err, err)
	}
	if !reflect.DeepEqual(permErr.RequestedScopes, requested) {
		t.Errorf("RequestedScopes = %v, want %v", permErr.RequestedScopes, requested)
	}
	if !reflect.DeepEqual(permErr.GrantedScopes, granted) {
		t.Errorf("GrantedScopes = %v, want %v", permErr.GrantedScopes, granted)
	}
	if !reflect.DeepEqual(permErr.MissingScopes, missing) {
		t.Errorf("MissingScopes = %v, want %v", permErr.MissingScopes, missing)
	}
}

// TestWriteLoginSuccess_JSONIncludesHolderMismatchWarning pins the JSON
// surface contract for S1: when the soft holder-mismatch advisory fires
// (operator did not pass --user / env, AppConfig.CurrentUser left over
// from a prior login disagrees with the freshly-authorized identity),
// the success payload must carry a structured `holder_mismatch_warning`
// field so non-human consumers (CI dashboards, agent runtimes, etc.) can
// branch on the active-user-stays semantics without parsing the stderr
// WARN line.
func TestWriteLoginSuccess_JSONIncludesHolderMismatchWarning(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)

	holderWarning := &holderMismatchWarning{
		HolderOpenId:   "ou_alice",
		HolderUserName: "Alice",
		FreshOpenId:    "ou_bob",
		FreshUserName:  "Bob",
		Message:        "[lark-cli] [WARN] auth login: ... active user stays Alice ...",
	}

	writeLoginSuccess(&LoginOptions{JSON: true}, getLoginMsg("en"), f, "ou_bob", "Bob", &loginScopeSummary{
		Granted: []string{"im:message:send"},
	}, holderWarning)

	var payload map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal(stdout) error = %v, stdout=%s", err, stdout.String())
	}

	raw, ok := payload["holder_mismatch_warning"]
	if !ok {
		t.Fatalf("payload missing holder_mismatch_warning field; full payload: %s", stdout.String())
	}
	got, ok := raw.(map[string]interface{})
	if !ok {
		t.Fatalf("holder_mismatch_warning is %T, want object", raw)
	}

	// Discriminator — symmetric with the existing scope `warning.type`
	// field so consumers can switch on a single key.
	if got["type"] != "holder_currentuser_mismatch" {
		t.Errorf("type = %v, want holder_currentuser_mismatch", got["type"])
	}
	// Typed identity fields — the whole point of S1 is letting consumers
	// key on these without regex'ing the message string.
	if got["holder_open_id"] != "ou_alice" {
		t.Errorf("holder_open_id = %v, want ou_alice", got["holder_open_id"])
	}
	if got["holder_user_name"] != "Alice" {
		t.Errorf("holder_user_name = %v, want Alice", got["holder_user_name"])
	}
	if got["fresh_open_id"] != "ou_bob" {
		t.Errorf("fresh_open_id = %v, want ou_bob", got["fresh_open_id"])
	}
	if got["fresh_user_name"] != "Bob" {
		t.Errorf("fresh_user_name = %v, want Bob", got["fresh_user_name"])
	}
	// Message stays for humans tailing 2>&1 of a JSON-mode invocation
	// against a TTY (rare but documented), and as a compatibility shim for
	// consumers that key on text already. Pin that the brand prefix
	// survives JSON encoding intact.
	msg, _ := got["message"].(string)
	if !strings.Contains(msg, "[lark-cli]") {
		t.Errorf("message lost brand prefix: %q", msg)
	}

	// Conversely: the missing-scope warning field MUST stay nil when only
	// a holder-mismatch fired. Crossing the two warnings here would let a
	// future regression silently emit `warning.type=missing_scope` when
	// the actual issue was the holder, breaking dashboards.
	if _, scopeWarn := payload["warning"]; scopeWarn {
		t.Errorf("payload should not have a `warning` field when only holder mismatch fired; got %v", payload["warning"])
	}
	// Sanity — the holder mismatch must not corrupt the rest of the success payload.
	if payload["event"] != "authorization_complete" {
		t.Errorf("event = %v, want authorization_complete", payload["event"])
	}
	if payload["user_open_id"] != "ou_bob" {
		t.Errorf("user_open_id = %v, want ou_bob (the freshly-authorized id, NOT the implied holder)", payload["user_open_id"])
	}
}

// Counter-test: a clean login (no holder mismatch) MUST NOT carry a
// holder_mismatch_warning key at all. JSON consumers branch on key
// existence — emitting `holder_mismatch_warning: null` would force them
// to also nil-check, and silent emission of an empty object would falsely
// trigger downstream alerting.
func TestWriteLoginSuccess_JSONOmitsHolderMismatchWarningOnCleanLogin(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)

	writeLoginSuccess(&LoginOptions{JSON: true}, getLoginMsg("en"), f, "ou_bob", "Bob", &loginScopeSummary{
		Granted: []string{"im:message:send"},
	}, nil)

	var payload map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal(stdout) error = %v, stdout=%s", err, stdout.String())
	}
	if _, ok := payload["holder_mismatch_warning"]; ok {
		t.Errorf("clean login must omit holder_mismatch_warning; got %v", payload["holder_mismatch_warning"])
	}
}

// Two-warning test: a single login that fires both the soft holder
// mismatch AND a missing-scope issue must carry BOTH structured fields
// independently. The fields share neither namespace nor lifecycle, so a
// consumer can react to one without the other.
func TestHandleLoginScopeIssue_JSONCarriesBothWarnings(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)

	holderWarning := &holderMismatchWarning{
		HolderOpenId:   "ou_alice",
		HolderUserName: "Alice",
		FreshOpenId:    "ou_bob",
		FreshUserName:  "Bob",
		Message:        "[lark-cli] [WARN] auth login: holder mismatch ...",
	}

	err := handleLoginScopeIssue(
		&LoginOptions{JSON: true},
		getLoginMsg("en"),
		f,
		&loginScopeIssue{
			Message: "scopes missing",
			Hint:    "re-login with --scope im:message:send",
			Summary: &loginScopeSummary{
				Requested: []string{"im:message:send", "docx:document"},
				Granted:   []string{"docx:document"},
				Missing:   []string{"im:message:send"},
			},
		},
		"ou_bob", // openId non-empty -> loginSucceeded = true (JSON path)
		"Bob",
		holderWarning,
	)
	// The scope-issue success-with-error path returns ErrBare so the
	// dispatcher can set the auth exit code without re-printing.
	if err == nil {
		t.Fatal("expected ErrBare auth-exit error from handleLoginScopeIssue")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal(stdout) error = %v, stdout=%s", err, stdout.String())
	}
	if _, ok := payload["warning"]; !ok {
		t.Errorf("payload missing scope warning; full payload: %s", stdout.String())
	}
	if _, ok := payload["holder_mismatch_warning"]; !ok {
		t.Errorf("payload missing holder_mismatch_warning; full payload: %s", stdout.String())
	}
	// The two warnings must not collide — different `type` discriminators.
	scopeWarn, _ := payload["warning"].(map[string]interface{})
	holderWarn, _ := payload["holder_mismatch_warning"].(map[string]interface{})
	if scopeWarn["type"] == holderWarn["type"] {
		t.Errorf("scope and holder warnings share the same type discriminator: %v", scopeWarn["type"])
	}
}

// End-to-end on the M1+S1 contract: when verifyHolder constructs the
// warning, it sanitizes the human-readable Message but leaves the typed
// HolderUserName / FreshUserName fields raw (per the dual-channel
// contract — stderr/Message gets escape-stripped, JSON typed fields
// keep raw bytes for consumers to escape themselves).
//
// authorizationCompletePayload then copies those struct fields verbatim
// into the JSON map. The unit test on verifyHolder pins the struct
// shape, and TestWriteLoginSuccess_JSONIncludesHolderMismatchWarning
// pins the JSON envelope, but neither asserts the COMPOSITION end-to-
// end. A future regression that called SanitizeForTerminal on the
// typed JSON fields (or stopped sanitizing the Message) would slip
// through both unit tests independently. This test pins the seam.
//
// We construct the holderMismatchWarning with the same shape verifyHolder
// would emit — Message is the post-sanitize string (no \x1b/\x07/\r),
// HolderUserName / FreshUserName carry raw escape bytes (what app.Users[]
// would have stored from a poisoned IdP response), open_ids untouched —
// then call writeLoginSuccess in JSON mode and assert the resulting
// payload preserves both invariants in transit.
func TestWriteLoginSuccess_JSON_SanitizesMessageButPreservesTypedFields(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)

	const rawHolderName = "Al\x1b[31mice\x1b[0m"
	const rawFreshName = "Bo\x07b"

	holderWarning := &holderMismatchWarning{
		HolderOpenId:   "ou_alice",
		HolderUserName: rawHolderName, // raw bytes — exactly what verifyHolder stores
		FreshOpenId:    "ou_bob",
		FreshUserName:  rawFreshName,
		// Message is the sanitized text — escape-stripped, with names
		// embedded only via the cleaned formatHolderLabel output.
		Message: "[lark-cli] [WARN] auth login: the active profile's currentUser is Alice (ou_alice) but the device you authorized was Bob (ou_bob). active user stays Alice (ou_alice). Run `lark-cli auth users use ou_bob` to switch the active user.",
	}

	if err := writeLoginSuccess(&LoginOptions{JSON: true}, getLoginMsg("en"), f, "ou_bob", "Bob", &loginScopeSummary{
		Granted: []string{"im:message:send"},
	}, holderWarning); err != nil {
		t.Fatalf("writeLoginSuccess error: %v", err)
	}

	// json.Decoder over the raw bytes — Unmarshal into map[string]interface{}
	// would also work but we want to exercise the same path a JSON consumer
	// piping `auth login --json` would hit.
	var payload map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal: %v\nstdout: %q", err, stdout.String())
	}

	got, ok := payload["holder_mismatch_warning"].(map[string]interface{})
	if !ok {
		t.Fatalf("holder_mismatch_warning is %T, want map[string]interface{}: %v", payload["holder_mismatch_warning"], payload)
	}

	// (a) The JSON `message` field is the sanitized stderr line — must
	// have NO escape bytes. This is the regression class where a future
	// refactor copies the wrong field, or accidentally rebuilds Message
	// from raw typed fields.
	msg, _ := got["message"].(string)
	for _, banned := range []string{"\x1b", "[31m", "[0m", "\x07"} {
		if strings.Contains(msg, banned) {
			t.Errorf("JSON message field leaked escape %q: %q", banned, msg)
		}
	}
	if !strings.Contains(msg, "[lark-cli]") {
		t.Errorf("JSON message must keep brand prefix: %q", msg)
	}

	// (b) The JSON typed fields preserve RAW bytes byte-for-byte. JSON's
	// own escaping (e.g.  for \x1b) is a wire-format transformation
	// that Unmarshal reverses — so after Unmarshal the Go string has the
	// raw escape byte back. Asserting equality (not just substring)
	// proves no sanitizer leaked into the typed-field path.
	if got["holder_user_name"] != rawHolderName {
		t.Errorf("holder_user_name lost raw bytes: got %q (% x), want %q (% x)",
			got["holder_user_name"], []byte(fmt.Sprint(got["holder_user_name"])),
			rawHolderName, []byte(rawHolderName))
	}
	if got["fresh_user_name"] != rawFreshName {
		t.Errorf("fresh_user_name lost raw bytes: got %q (% x), want %q (% x)",
			got["fresh_user_name"], []byte(fmt.Sprint(got["fresh_user_name"])),
			rawFreshName, []byte(rawFreshName))
	}
	// open_ids stay verbatim by contract — IdP-validated upstream, no
	// sanitization at this seam.
	if got["holder_open_id"] != "ou_alice" || got["fresh_open_id"] != "ou_bob" {
		t.Errorf("open_id fields drifted: %v / %v", got["holder_open_id"], got["fresh_open_id"])
	}
}
