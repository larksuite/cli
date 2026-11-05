// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package html

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/shortcuts/html/payload"
)

func TestPublishRecoveryError_PreservesErrorAndCause(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cause error
	}{
		{"Sensitive_Files", payload.ErrSensitiveFiles},
		{"Entry_Conflict", payload.ErrEntryConflict},
		{"Dependency_Limit", payload.ErrDependencyLimit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cause := fmt.Errorf("payload check: %w", tc.cause)
			original := errs.NewValidationError(errs.SubtypeFailedPrecondition, "payload rejected").
				WithParam("--file-path").WithCause(cause).WithHint("apps-only advice")
			before := *original
			wrapped := fmt.Errorf("validation: %w", original)
			actual := publishRecoveryError(wrapped)
			assert.Same(t, wrapped, actual)
			assert.ErrorIs(t, actual, cause)
			assert.ErrorIs(t, actual, tc.cause)
			var validation *errs.ValidationError
			require.ErrorAs(t, actual, &validation)
			assert.NotEmpty(t, validation.Hint)
			assert.NotEqual(t, before.Hint, validation.Hint)
			before.Hint = validation.Hint
			assert.Equal(t, before, *validation)
		})
	}
}

func TestPublishRecoveryError_PreservesUnrelatedFailures(t *testing.T) {
	original := errs.NewValidationError(errs.SubtypeFailedPrecondition, "file is unreadable").
		WithCause(fs.ErrPermission).WithHint("check file permissions")
	assert.Same(t, original, publishRecoveryError(original))
	assert.Equal(t, "check file permissions", original.Hint)
	assert.ErrorIs(t, original, fs.ErrPermission)
	assert.Same(t, fs.ErrNotExist, publishRecoveryError(fs.ErrNotExist))
	assert.NoError(t, publishRecoveryError(nil))
}

func TestPublish_SkipRecoveryHints(t *testing.T) {
	for _, tc := range []struct {
		name, script, advice string
		kind                 payload.SkipKind
	}{
		{"Dynamic_Reference", "fetch(runtimePath);", "use literal local paths", payload.SkipDynamic},
		{"Unparsed_Script", "const = ;", "fix the file", payload.SkipUnparsed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmdutil.TestChdir(t, t.TempDir())
			writeFile(t, "page.html", []byte(`<script src="app.js"></script>`))
			writeFile(t, "app.js", []byte(tc.script))
			out, err := runHTML(t, Publish, []string{"+publish", "--file-path", "page.html", "--dry-run"}, nil)
			require.NoError(t, err)
			var response struct {
				Data struct {
					Skipped   []payload.Skip `json:"skipped_dependencies"`
					FileCount int            `json:"file_count"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal([]byte(out), &response))
			assert.Equal(t, 2, response.Data.FileCount)
			require.Len(t, response.Data.Skipped, 1)
			skip := response.Data.Skipped[0]
			assert.Equal(t, tc.kind, skip.Kind)
			assert.Equal(t, "app.js", skip.Ref)
			assert.Equal(t, "app.js", skip.From)
			assert.NotEmpty(t, skip.Why)
			assert.Contains(t, skip.Advice, tc.advice)
			assert.NotContains(t, skip.Advice, "--dir")
		})
	}
}
