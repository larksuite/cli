// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deploy

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
)

// A hint that does not fit its cause sends the caller round a loop: they do what
// it says, hit the identical error, and conclude the tool is broken. This has
// happened twice on this path, so the pairing is pinned rather than left to
// review.
func TestInvalidReferenceHintMatchesItsCause(t *testing.T) {
	cases := map[string]struct {
		ref  string
		hint string
	}{
		// Only a reference climbing out of the payload is fixed by moving a
		// file, and even then the reference itself has to change with it.
		"escapes the payload": {"../shared/app.css", hintReferenceEscapes},
		// The rest are malformed text: no amount of moving files helps.
		"file scheme":   {"file:///etc/hosts", hintFixReferenceText},
		"windows drive": {`c:\boot.css`, hintFixReferenceText},
		"backslash":     {`sub\win.css`, hintFixReferenceText},
		"bad percent":   {"a%ZZb.css", hintFixReferenceText},
		"decoded colon": {"a%3Ab.css", hintFixReferenceText},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, err := resolveReference("index.html", c.ref)
			if err == nil {
				t.Fatalf("expected %q to be rejected", c.ref)
			}
			var ve *errs.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("expected a validation error, got %T", err)
			}
			if ve.Hint != c.hint {
				t.Errorf("hint does not fit the cause\n got %q\nwant %q", ve.Hint, c.hint)
			}
		})
	}
	if hintReferenceEscapes == hintFixReferenceText {
		t.Fatal("the two hints must stay distinct; one advises moving files, the other editing text")
	}
}
