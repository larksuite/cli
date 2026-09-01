// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package distribution

import (
	"errors"
	"os"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestClassifyError(t *testing.T) {
	for _, tt := range []struct {
		name     string
		err      error
		category errs.Category
		subtype  errs.Subtype
	}{
		{name: "file IO", err: &os.PathError{Op: "mkdir", Path: "/tmp/config", Err: os.ErrPermission}, category: errs.CategoryInternal, subtype: errs.SubtypeFileIO},
		{name: "bad archive", err: errors.New("unsupported archive format"), category: errs.CategoryNetwork, subtype: errs.SubtypeNetworkProtocol},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyError("distribution failed", tt.err)
			problem, ok := errs.ProblemOf(got)
			if !ok || problem.Category != tt.category || problem.Subtype != tt.subtype {
				t.Fatalf("problem = %#v, want category=%q subtype=%q", problem, tt.category, tt.subtype)
			}
			if !errors.Is(got, tt.err) {
				t.Fatalf("cause %v was not preserved", tt.err)
			}
		})
	}
}
