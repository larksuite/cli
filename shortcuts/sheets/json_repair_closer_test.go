// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"testing"
)

func decodeErrFor(t *testing.T, raw string) error {
	t.Helper()
	var v interface{}
	err := json.Unmarshal([]byte(raw), &v)
	if err == nil {
		t.Fatalf("fixture parses; it cannot exercise the repair: %s", raw)
	}
	return err
}

// `invalid character '}' after array element` is 24,559 calls in the
// 09-11..15 catalog. At that position JSON allows only `,` or `]`.
func TestRepairMistypedCloser_FixesASingleWrongBracket(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"array closed with a brace", `[[{"value":"a"}]}`, `[[{"value":"a"}]]`},
		{"writes array closed with a brace", `[{"range":"A1","cells":[[{"value":1}]]}}`, `[{"range":"A1","cells":[[{"value":1}]]}]`},
		{"object closed with a bracket", `{"a":1]`, `{"a":1}`},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := repairMistypedCloser(tt.raw, decodeErrFor(t, tt.raw))
			if !ok {
				t.Fatalf("not repaired: %s", tt.raw)
			}
			if got != tt.want {
				t.Errorf("repaired = %s, want %s", got, tt.want)
			}
		})
	}
}

// The truncation this module refuses on principle: repairing it means guessing
// where the caller's data ended, and a guess that lands writes a wrong table
// silently instead of failing loudly.
func TestRepairMistypedCloser_RefusesEverythingThatNeedsAGuess(t *testing.T) {
	for _, raw := range []string{
		`[[{"value":"a"`,       // cut short mid-object
		`[[{"value":"a"}]`,     // one closer genuinely missing at the end
		`{"a":[1,2},{"b":3}]}`, // structure is wrong, not one keystroke
		`[1,2,,3]`,             // not a bracket problem at all
		`{"a":1,}`,             // trailing comma: repairLooseJSON's job
		`nonsense`,             // not JSON
	} {
		var v interface{}
		err := json.Unmarshal([]byte(raw), &v)
		if err == nil {
			continue // already valid; nothing to refuse
		}
		if got, ok := repairMistypedCloser(raw, err); ok {
			t.Errorf("repaired a shape that needs a guess: %s -> %s", raw, got)
		}
	}
}

// A non-syntax error carries no offset to act on.
func TestRepairMistypedCloser_IgnoresNonSyntaxErrors(t *testing.T) {
	var target []int
	err := json.Unmarshal([]byte(`{"a":1}`), &target) // UnmarshalTypeError
	if err == nil {
		t.Fatal("fixture did not produce a type error")
	}
	if _, ok := repairMistypedCloser(`{"a":1}`, err); ok {
		t.Error("acted on a non-syntax error")
	}
}

// Strict input is never touched: the repair only ever runs after the decoder
// has already refused the payload, and this pins that it changes nothing when
// handed a document that parses.
func TestRepairMistypedCloser_LeavesValidJSONAlone(t *testing.T) {
	var v interface{}
	raw := `[[{"value":"a"}]]`
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("fixture should parse: %v", err)
	}
	if _, ok := repairMistypedCloser(raw, nil); ok {
		t.Error("repaired a document that already parses")
	}
}
