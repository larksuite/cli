// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errcompat_test

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/errcompat"
)

// R2 forward-incompat schema must promote to SubtypeInvalidConfig so the
// dispatcher does not push AI agents toward `config init` (which would
// overwrite fields a newer binary populated).
func TestPromoteConfigError_R2_ClassifiesAsInvalidConfig(t *testing.T) {
	cases := []struct {
		name    string
		message string
	}{
		{"newer_binary_phrase", "config.json was written by a newer lark-cli (schemaVersion 99 > supported 1)"},
		{"schemaversion_phrase", "schemaVersion 5 > supported 1"},
		{"failed_to_load_wrap", "failed to load config: invalid config format: unexpected EOF"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &core.ConfigError{Type: "config", Code: 3, Message: tc.message, Hint: "upgrade lark-cli"}
			got := errcompat.PromoteConfigError(cfg)

			var ce *errs.ConfigError
			if !errors.As(got, &ce) {
				t.Fatalf("expected *errs.ConfigError, got %T", got)
			}
			if ce.Subtype != errs.SubtypeInvalidConfig {
				t.Errorf("subtype = %v, want SubtypeInvalidConfig (R2/parse must NOT route to NotConfigured)", ce.Subtype)
			}
			if ce.Hint != "upgrade lark-cli" {
				t.Errorf("hint dropped during promotion: %q", ce.Hint)
			}
		})
	}
}
