// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/envelope"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
)

// TestDispatchErrorGoldenParity asserts the public envelope.DispatchError
// triple is byte-identical to what the real root dispatcher
// (handleRootError) writes to stderr, with the same exit code, for every
// error class (spec G1-G6). Comparison happens at the dispatch boundary:
// need_user_authorization hint folding runs before dispatch and is not part
// of the public contract, so cases here must not depend on it.
func TestDispatchErrorGoldenParity(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	cases := []struct {
		name string
		err  error
	}{
		{"G1_typed_validation", errs.NewValidationError(errs.SubtypeInvalidArgument, "missing --id")},
		{"G2_typed_extension_fields", &errs.PermissionError{
			Problem: errs.Problem{
				Category: errs.CategoryAuthorization,
				Subtype:  errs.SubtypePermissionDenied,
				Code:     99991679,
				Message:  "missing required scopes",
				Hint:     "re-auth with the listed scopes",
			},
			MissingScopes: []string{"im:message", "docs:doc"},
			Identity:      "user",
		}},
		{"G3_confirmation_required", errs.NewConfirmationRequiredError(
			"high-risk-write", "drive +delete", "drive +delete requires confirmation")},
		{"G4_partial_failure", output.PartialFailure(1)},
		{"G5_cobra_usage", fmt.Errorf(`required flag(s) "values" not set`)},
		{"G6_leaked_untyped", errors.New("boom")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, _, _, _ := cmdutil.TestFactory(t, nil)
			errOut := &bytes.Buffer{}
			f.IOStreams.ErrOut = errOut

			realExit := handleRootError(f, tc.err)
			env, code, has := envelope.DispatchError(tc.err, string(f.ResolvedIdentity))

			if code != realExit {
				t.Errorf("exit code: public %d, real dispatcher %d", code, realExit)
			}
			if has != (errOut.Len() > 0) {
				t.Errorf("hasEnvelope=%v but real stderr len=%d", has, errOut.Len())
			}
			if !bytes.Equal(env, errOut.Bytes()) {
				t.Errorf("envelope bytes differ\npublic:  %s\nreal:    %s", env, errOut.Bytes())
			}
		})
	}
}
