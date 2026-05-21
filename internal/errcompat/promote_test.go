// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errcompat_test

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/errcompat"
)

// TestPromoteConfigError_Stage1Passthrough pins the stage-1 passthrough
// behaviour: every input *core.ConfigError flows out unchanged so the
// dispatcher's legacy envelope path emits the same wire shape as pre-PR.
// Per-domain typed migration will replace this in stage 2+.
func TestPromoteConfigError_Stage1Passthrough(t *testing.T) {
	for _, cfgType := range []string{"config", "auth", "openclaw", ""} {
		t.Run(cfgType, func(t *testing.T) {
			src := &core.ConfigError{Code: 3, Type: cfgType, Message: "msg", Hint: "hint"}
			out := errcompat.PromoteConfigError(src)
			var got *core.ConfigError
			if !errors.As(out, &got) || got != src {
				t.Fatalf("Type=%q: expected passthrough of original *core.ConfigError, got %T (%v)", cfgType, out, out)
			}
		})
	}
}

// TestPromoteConfigError_NilInputReturnsNil pins that PromoteConfigError on a
// nil input returns nil rather than panicking on the (cfgErr.Type) access.
func TestPromoteConfigError_NilInputReturnsNil(t *testing.T) {
	if got := errcompat.PromoteConfigError(nil); got != nil {
		t.Errorf("PromoteConfigError(nil) = %v, want nil", got)
	}
}
