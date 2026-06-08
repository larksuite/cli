// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
)

func TestMailFileIOErrorTyped(t *testing.T) {
	cause := errors.New("disk read failed")

	err := mailFileIOError("load %s: %v", cause, "body.html", cause)

	var internalErr *errs.InternalError
	if !errors.As(err, &internalErr) {
		t.Fatalf("expected internal error, got %T", err)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("cause not preserved: %v", err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T", err)
	}
	if p.Subtype != errs.SubtypeFileIO {
		t.Fatalf("subtype = %q, want %q", p.Subtype, errs.SubtypeFileIO)
	}
	if !strings.Contains(p.Message, "body.html") || !strings.Contains(p.Message, "disk read failed") {
		t.Fatalf("message missing context: %q", p.Message)
	}
}

func TestMailFileIOErrorDoesNotAppendCauseAsFormatArg(t *testing.T) {
	cause := errors.New("mkdir denied")

	err := mailFileIOError("cannot create output directory %q", cause, "out")

	if !errors.Is(err, cause) {
		t.Fatalf("cause not preserved: %v", err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T", err)
	}
	if strings.Contains(p.Message, "%!(") {
		t.Fatalf("message contains fmt extra marker: %q", p.Message)
	}
	if strings.Contains(p.Message, "mkdir denied") {
		t.Fatalf("cause should not be implicitly appended to message: %q", p.Message)
	}
}

func TestMailInputStatErrorTyped(t *testing.T) {
	if err := mailInputStatError(nil); err != nil {
		t.Fatalf("nil input should stay nil, got %v", err)
	}

	pathErr := fileio.ErrPathValidation
	err := mailInputStatError(pathErr)
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %T", err)
	}
	if !errors.Is(err, pathErr) {
		t.Fatalf("cause not preserved: %v", err)
	}
	if !strings.Contains(err.Error(), "unsafe file path") {
		t.Fatalf("unexpected path validation message: %v", err)
	}

	statErr := errors.New("permission denied")
	err = mailInputStatError(statErr)
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %T", err)
	}
	if !errors.Is(err, statErr) {
		t.Fatalf("stat cause not preserved: %v", err)
	}
	if !strings.Contains(err.Error(), "cannot read file") {
		t.Fatalf("unexpected stat message: %v", err)
	}
}

func TestMailDecorateProblemMessageTypedAndPlain(t *testing.T) {
	if err := mailDecorateProblemMessage(nil, "fetch profile"); err != nil {
		t.Fatalf("nil input should stay nil, got %v", err)
	}

	typedErr := errs.NewAPIError(errs.SubtypeRateLimit, "too many requests")
	err := mailDecorateProblemMessage(typedErr, "fetch %s", "profile")
	if err != typedErr {
		t.Fatalf("typed error should be decorated in place")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T", err)
	}
	if p.Message != "fetch profile: too many requests" {
		t.Fatalf("message = %q", p.Message)
	}

	blankPrefixErr := errs.NewAPIError(errs.SubtypeUnknown, "unchanged")
	err = mailDecorateProblemMessage(blankPrefixErr, "  ")
	p, ok = errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T", err)
	}
	if p.Message != "unchanged" {
		t.Fatalf("blank prefix should not change message, got %q", p.Message)
	}

	plainCause := errors.New("sdk failed")
	err = mailDecorateProblemMessage(plainCause, "fetch mailbox")
	var internalErr *errs.InternalError
	if !errors.As(err, &internalErr) {
		t.Fatalf("plain error should be upgraded to internal SDK error, got %T", err)
	}
	if !errors.Is(err, plainCause) {
		t.Fatalf("cause not preserved: %v", err)
	}
	p, ok = errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T", err)
	}
	if p.Subtype != errs.SubtypeSDKError || !strings.Contains(p.Message, "fetch mailbox: sdk failed") {
		t.Fatalf("unexpected problem: %+v", p)
	}
}

func TestMailAppendProblemHintTypedAndPlain(t *testing.T) {
	if err := mailAppendProblemHint(nil, "retry later"); err != nil {
		t.Fatalf("nil input should stay nil, got %v", err)
	}

	withoutHint := errs.NewAPIError(errs.SubtypeUnknown, "failed")
	err := mailAppendProblemHint(withoutHint, "retry later")
	if err != withoutHint {
		t.Fatalf("typed error should be updated in place")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T", err)
	}
	if p.Hint != "retry later" {
		t.Fatalf("hint = %q", p.Hint)
	}

	withHint := errs.NewAPIError(errs.SubtypeUnknown, "failed").WithHint("check scope")
	err = mailAppendProblemHint(withHint, "retry later")
	p, ok = errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T", err)
	}
	if p.Hint != "check scope; retry later" {
		t.Fatalf("hint = %q", p.Hint)
	}

	plainCause := errors.New("legacy api failed")
	err = mailAppendProblemHint(plainCause, "retry later")
	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("plain error should be upgraded to API error, got %T", err)
	}
	if !errors.Is(err, plainCause) {
		t.Fatalf("cause not preserved: %v", err)
	}
	p, ok = errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T", err)
	}
	if p.Hint != "retry later" || p.Subtype != errs.SubtypeUnknown {
		t.Fatalf("unexpected problem: %+v", p)
	}
}

func TestValidateBodyFileMutexTypedErrors(t *testing.T) {
	err := validateBodyFileMutex("<p>Hello</p>", "body.html", func(string) error { return nil })
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %T", err)
	}
	if len(validationErr.Params) != 2 {
		t.Fatalf("params = %#v, want two conflicting params", validationErr.Params)
	}
	if validationErr.Params[0].Name != "--body" || validationErr.Params[1].Name != "--body-file" {
		t.Fatalf("unexpected params: %#v", validationErr.Params)
	}

	pathErr := errors.New("outside cwd")
	err = validateBodyFileMutex("", "body.html", func(string) error { return pathErr })
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %T", err)
	}
	if validationErr.Param != "--body-file" {
		t.Fatalf("param = %q, want --body-file", validationErr.Param)
	}
	if !errors.Is(err, pathErr) {
		t.Fatalf("cause not preserved: %v", err)
	}
}

func TestReadBodyFileTypedErrors(t *testing.T) {
	openErr := errors.New("missing")
	_, err := readBodyFile(bodyFileTestIO{
		open: func(string) (fileio.File, error) { return nil, openErr },
	}, "missing.html")
	requireBodyFileValidationError(t, err, openErr)
	if !strings.Contains(err.Error(), "open --body-file missing.html") {
		t.Fatalf("unexpected open message: %v", err)
	}

	readErr := errors.New("read broken")
	_, err = readBodyFile(bodyFileTestIO{
		open: func(string) (fileio.File, error) {
			return &bodyFileTestFile{readErr: readErr}, nil
		},
	}, "body.html")
	requireBodyFileValidationError(t, err, readErr)
	if !strings.Contains(err.Error(), "read --body-file body.html") {
		t.Fatalf("unexpected read message: %v", err)
	}

	_, err = readBodyFile(bodyFileTestIO{
		open: func(string) (fileio.File, error) {
			return &bodyFileTestFile{remaining: maxBodyFileSize + 1}, nil
		},
	}, "huge.html")
	requireBodyFileValidationError(t, err, nil)
	if !strings.Contains(err.Error(), "file exceeds 32 MB limit") {
		t.Fatalf("unexpected size message: %v", err)
	}
}

func requireBodyFileValidationError(t *testing.T, err error, cause error) {
	t.Helper()
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %T (%v)", err, err)
	}
	if validationErr.Param != "--body-file" {
		t.Fatalf("param = %q, want --body-file", validationErr.Param)
	}
	if cause != nil && !errors.Is(err, cause) {
		t.Fatalf("cause %v not preserved in %v", cause, err)
	}
}

type bodyFileTestIO struct {
	open func(string) (fileio.File, error)
}

func (fio bodyFileTestIO) Open(name string) (fileio.File, error) {
	return fio.open(name)
}

func (fio bodyFileTestIO) Stat(string) (fileio.FileInfo, error) {
	return nil, errors.New("unused")
}

func (fio bodyFileTestIO) ResolvePath(path string) (string, error) {
	return path, nil
}

func (fio bodyFileTestIO) Save(string, fileio.SaveOptions, io.Reader) (fileio.SaveResult, error) {
	return nil, errors.New("unused")
}

type bodyFileTestFile struct {
	readErr   error
	remaining int
}

func (f *bodyFileTestFile) Read(p []byte) (int, error) {
	if f.readErr != nil {
		return 0, f.readErr
	}
	if f.remaining <= 0 {
		return 0, io.EOF
	}
	n := len(p)
	if n > f.remaining {
		n = f.remaining
	}
	for i := range p[:n] {
		p[i] = 'x'
	}
	f.remaining -= n
	return n, nil
}

func (f *bodyFileTestFile) ReadAt([]byte, int64) (int, error) {
	return 0, errors.New("unused")
}

func (f *bodyFileTestFile) Close() error {
	return nil
}

var _ fileio.FileIO = bodyFileTestIO{}
var _ fileio.File = (*bodyFileTestFile)(nil)
