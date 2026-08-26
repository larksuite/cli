// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
)

func TestValidateDocsWriteContentEncodingAcceptsUTF8(t *testing.T) {
	t.Parallel()

	for _, content := range []string{
		"",
		"<title>项目计划</title><p>这是中文正文。</p>",
		"# 项目计划\n\n这是中文正文。",
		"<p>Unicode replacement character: �</p>",
		"<p>literal NUL follows:\x00</p>",
	} {
		if err := validateDocsWriteContentEncoding(content); err != nil {
			t.Fatalf("validateDocsWriteContentEncoding(%q) error = %v", content, err)
		}
	}
}

func TestValidateDocsWriteContentEncodingRejectsInvalidUTF8(t *testing.T) {
	t.Parallel()

	content := string([]byte{0xc4, 0xe3, 0xba, 0xc3}) // CP936 encoding of “你好”.
	err := validateDocsWriteContentEncoding(content)
	assertDocsContentEncodingError(t, err, "must be valid UTF-8")
}

func TestDocsCreateRejectsInvalidUTF8FromStdin(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	f.IOStreams.In = bytes.NewReader([]byte{0xc4, 0xe3, 0xba, 0xc3})

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create",
		"--content", "-",
		"--dry-run",
		"--as", "user",
	})
	assertDocsContentEncodingError(t, err, "must be valid UTF-8")
}

func TestDocsUpdateRejectsInvalidUTF8Content(t *testing.T) {
	t.Parallel()

	runtime := newUpdateShortcutTestRuntime(t, "", map[string]string{
		"content": string([]byte{0xc4, 0xe3, 0xba, 0xc3}),
	})
	err := validateUpdateV2(context.Background(), runtime)
	assertDocsContentEncodingError(t, err, "must be valid UTF-8")
}

func assertDocsContentEncodingError(t *testing.T, err error, wantMessage string) {
	t.Helper()

	assertValidationContract(t, err, errs.SubtypeInvalidArgument, "--content")
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T, want *errs.ValidationError", err)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error does not expose a typed problem: %v", err)
	}
	if !strings.Contains(problem.Message, wantMessage) {
		t.Fatalf("message = %q, want substring %q", problem.Message, wantMessage)
	}
}
