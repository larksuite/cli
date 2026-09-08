// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
)

func TestDriveAppendErrorPreservesFileIOClassification(t *testing.T) {
	pathErr := &fileio.PathValidationError{Err: errors.New("unsafe path")}
	for name, input := range map[string]error{
		"path":  pathErr,
		"mkdir": &fileio.MkdirError{Err: errors.New("permission denied")},
		"write": &fileio.WriteError{Err: errors.New("disk full")},
	} {
		t.Run(name, func(t *testing.T) {
			got := driveAppendError(input)
			problem, ok := errs.ProblemOf(got)
			if !ok {
				t.Fatalf("driveAppendError() = %T %v, want typed error", got, got)
			}
			if problem.Subtype != errs.SubtypeFileIO && !errors.Is(got, fileio.ErrPathValidation) {
				t.Fatalf("problem subtype = %q, want fileio (or path validation)", problem.Subtype)
			}
		})
	}
}

func TestDriveAppendErrorPreservesAlreadyTypedError(t *testing.T) {
	input := errs.NewNetworkError(errs.SubtypeNetworkServer, "upstream failed")
	if got := driveAppendError(input); got != input {
		t.Fatalf("driveAppendError() = %v, want the original typed error", got)
	}
}
