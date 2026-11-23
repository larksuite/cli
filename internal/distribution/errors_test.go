// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package distribution

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestInstallErrorPermissionHint(t *testing.T) {
	cause := &os.PathError{Op: "rename", Path: "/example/cli", Err: os.ErrPermission}
	got := installError("cannot install", cause)
	p := got.ProblemDetail()
	if p.Category != errs.CategoryInternal || p.Subtype != errs.SubtypeUnknown || !errors.Is(got, cause) {
		t.Fatalf("error contract changed: %+v", got)
	}
	if !strings.Contains(p.Hint, "write permissions") || strings.Contains(p.Hint, "--force") {
		t.Fatalf("permission hint = %q", p.Hint)
	}
}

func TestClassifyArtifactError(t *testing.T) {
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
			got := classifyArtifactError("extract", "skills", tt.err)
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
