// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package payload

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/larksuite/cli/errs"
)

func TestPayloadFailureReasons_PreserveTypedCauses(t *testing.T) {
	_, sensitive := Guard([]Candidate{{RelPath: "credentials", Size: 1}}, false, DefaultLimits())
	_, _, conflict := BuildManifest([]Candidate{{RelPath: "page.html"}, {RelPath: "index.html"}}, "page.html")
	for _, tc := range []struct {
		name       string
		err, cause error
		hint       string
		subtype    errs.Subtype
	}{
		{"Sensitive_Files", sensitive, ErrSensitiveFiles, "--allow-sensitive", errs.SubtypeInvalidArgument},
		{"Entry_Conflict", conflict, ErrEntryConflict, "--entry-file", errs.SubtypeFailedPrecondition},
		{"Dependency_Limit", limitError("too many dependencies"), ErrDependencyLimit, "--dir", errs.SubtypeFailedPrecondition},
	} {
		t.Run(tc.name, func(t *testing.T) {
			validation := requireValidation(t, tc.err, tc.subtype)
			assert.ErrorIs(t, tc.err, tc.cause)
			assert.Contains(t, validation.Hint, tc.hint)
		})
	}
}
