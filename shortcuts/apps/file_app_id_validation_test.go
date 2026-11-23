// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

// fileAppIDCommands are the file commands whose --app-id addresses an app by its
// real id only. Listing them here rather than ranging over Shortcuts() is
// deliberate: the point of the test is that this set does not silently shrink
// when a command is edited.
var fileAppIDCommands = []string{
	"+file-list", "+file-get", "+file-upload", "+file-download",
	"+file-delete", "+file-sign", "+file-quota-get",
}

// TestRequireFileAppID_RejectsNonAppIdentifiers pins the client-side gate for the
// storage commands.
//
// Before it existed, a meta token or a page token was forwarded verbatim and the
// backend answered "user need admin or developer permission" — so a caller who
// had simply pasted the wrong identifier was told to go request access they
// already had. The value of catching it here is the wording: the error names the
// argument, and the hint carries the command that converts the token into the
// app_id.
func TestRequireFileAppID_RejectsNonAppIdentifiers(t *testing.T) {
	// Every shape observed being pasted into --app-id in place of an app id.
	for _, raw := range []string{
		"mtk_1234567890abcdef",      // meta token
		"doccnAbCdEfGhIjKlMnOpQr",   // doc token from a /page/<token>/ link
		"7685678125455560209",       // a numeric id (release id, record id, …)
		"bascnXyZ123",               // bitable token
		"  doccnAbCdEfGhIjKlMnOpQr", // leading blanks must not smuggle it through
	} {
		t.Run(strings.TrimSpace(raw), func(t *testing.T) {
			_, err := requireFileAppID(raw)
			if err == nil {
				t.Fatalf("requireFileAppID(%q) = nil, want a validation error", raw)
			}
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("error is not a typed problem: %v", err)
			}
			if p.Category != errs.CategoryValidation || p.Subtype != errs.SubtypeInvalidArgument {
				t.Fatalf("category/subtype = %s/%s, want validation/invalid_argument", p.Category, p.Subtype)
			}
			if got := validationParamOf(t, err); got != "--app-id" {
				t.Errorf("param = %q, want --app-id", got)
			}
			// The recovery command is the whole point; a bare "invalid" would leave
			// the caller holding a token with nothing to do about it.
			if !strings.Contains(p.Hint, "+get") {
				t.Errorf("hint must name the command that resolves the token, got %q", p.Hint)
			}
		})
	}
}

// TestRequireFileAppID_AcceptsRealAppIDs guards the other direction: the gate must
// not start rejecting the ids these commands are for.
func TestRequireFileAppID_AcceptsRealAppIDs(t *testing.T) {
	for _, raw := range []string{"app_17dzdjh51jt", "  app_17dzdjh51jt  "} {
		got, err := requireFileAppID(raw)
		if err != nil {
			t.Fatalf("requireFileAppID(%q) = %v, want it accepted", raw, err)
		}
		if got != strings.TrimSpace(raw) {
			t.Errorf("requireFileAppID(%q) = %q, want the trimmed id", raw, got)
		}
	}
}

// TestRequireFileAppID_MissingStaysRequired keeps the empty case on its own
// message: "required" and "malformed" are different fixes.
func TestRequireFileAppID_MissingStaysRequired(t *testing.T) {
	for _, raw := range []string{"", "   "} {
		_, err := requireFileAppID(raw)
		if err == nil {
			t.Fatalf("requireFileAppID(%q) = nil, want a validation error", raw)
		}
		p, _ := errs.ProblemOf(err)
		if !strings.Contains(p.Message, "required") {
			t.Errorf("message = %q, want it to say the flag is required", p.Message)
		}
	}
}

// TestFileCommands_ValidateRejectsNonAppID exercises the gate through each
// command's own Validate hook, which is what actually runs. A helper that
// validates correctly but is wired into only some of the commands would pass the
// tests above and still ship the defect.
func TestFileCommands_ValidateRejectsNonAppID(t *testing.T) {
	byCommand := map[string]bool{}
	for _, sc := range Shortcuts() {
		byCommand[sc.Command] = true
	}
	for _, name := range fileAppIDCommands {
		if !byCommand[name] {
			t.Fatalf("%s is no longer registered; update fileAppIDCommands", name)
		}
	}

	for _, sc := range Shortcuts() {
		want := false
		for _, name := range fileAppIDCommands {
			if sc.Command == name {
				want = true
				break
			}
		}
		if !want || sc.Validate == nil {
			continue
		}
		t.Run(sc.Command, func(t *testing.T) {
			// Other required flags are filled so the run reaches the --app-id check
			// instead of stopping at an unrelated "required" error.
			values := map[string]string{"app-id": "doccnAbCdEfGhIjKlMnOpQr"}
			for _, f := range sc.Flags {
				if f.Required && f.Name != "app-id" {
					values[f.Name] = "placeholder"
				}
			}
			rctx := newAppsMemberRuntime(t, sc, values)
			err := sc.Validate(context.Background(), rctx)
			if err == nil {
				t.Fatalf("%s accepted a page token as --app-id", sc.Command)
			}
			if got := validationParamOf(t, err); got != "--app-id" {
				t.Fatalf("%s: error points at %q, want --app-id: %v", sc.Command, got, err)
			}
		})
	}
}

// validationParamOf extracts the offending flag name from a typed validation
// error. It lives on *errs.ValidationError rather than on the shared Problem, so
// the assertion has to unwrap rather than read ProblemOf.
func validationParamOf(t *testing.T, err error) string {
	t.Helper()
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error is not a *errs.ValidationError: %v", err)
	}
	return ve.Param
}
