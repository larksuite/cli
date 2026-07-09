// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestWithAppsHint(t *testing.T) {
	t.Run("nil error stays nil", func(t *testing.T) {
		if got := withAppsHint(nil, "do x"); got != nil {
			t.Fatalf("withAppsHint(nil) = %v, want nil", got)
		}
	})

	t.Run("empty hint gets filled, classification preserved", func(t *testing.T) {
		in := errs.NewAPIError(errs.SubtypeNotFound, "boom").WithCode(404)
		out := withAppsHint(in, "run +release-list")
		p, ok := errs.ProblemOf(out)
		if !ok {
			t.Fatalf("returned error is not typed: %T", out)
		}
		if p.Hint != "run +release-list" {
			t.Errorf("Hint = %q, want %q", p.Hint, "run +release-list")
		}
		if p.Subtype != errs.SubtypeNotFound || p.Code != 404 || p.Message != "boom" {
			t.Errorf("subtype/code/message mutated: subtype=%q code=%d msg=%q", p.Subtype, p.Code, p.Message)
		}
	})

	t.Run("existing hint is preserved, not clobbered", func(t *testing.T) {
		in := errs.NewAPIError(errs.SubtypeUnknown, "boom").WithHint("original hint")
		out := withAppsHint(in, "new hint")
		p, _ := errs.ProblemOf(out)
		if p.Hint != "original hint" {
			t.Errorf("Hint = %q, want preserved %q", p.Hint, "original hint")
		}
	})

	t.Run("blank-whitespace hint is treated as empty and filled", func(t *testing.T) {
		in := errs.NewAPIError(errs.SubtypeUnknown, "boom").WithHint("   ")
		out := withAppsHint(in, "filled hint")
		p, _ := errs.ProblemOf(out)
		if p.Hint != "filled hint" {
			t.Errorf("Hint = %q, want %q", p.Hint, "filled hint")
		}
	})

	t.Run("untyped error returned unchanged, no panic", func(t *testing.T) {
		in := errors.New("plain")
		out := withAppsHint(in, "ignored")
		if out == nil || out.Error() != "plain" {
			t.Fatalf("withAppsHint(plain) = %v, want unchanged plain error", out)
		}
	})
}
