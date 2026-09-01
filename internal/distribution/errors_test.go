// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package distribution

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestClassifyError(t *testing.T) {
	for _, tt := range []struct {
		name      string
		err       error
		category  errs.Category
		subtype   errs.Subtype
		retryable bool
	}{
		{name: "timeout", err: context.DeadlineExceeded, category: errs.CategoryNetwork, subtype: errs.SubtypeNetworkTimeout, retryable: true},
		{name: "dns", err: &net.DNSError{Err: "lookup failed", Name: "dist.example"}, category: errs.CategoryNetwork, subtype: errs.SubtypeNetworkDNS, retryable: true},
		{name: "file IO", err: &os.PathError{Op: "mkdir", Path: "/tmp/config", Err: os.ErrPermission}, category: errs.CategoryInternal, subtype: errs.SubtypeFileIO},
		{name: "bad archive", err: errors.New("unsupported archive format"), category: errs.CategoryNetwork, subtype: errs.SubtypeNetworkProtocol},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyError("distribution failed", tt.err)
			problem, ok := errs.ProblemOf(got)
			if !ok || problem.Category != tt.category || problem.Subtype != tt.subtype || problem.Retryable != tt.retryable {
				t.Fatalf("problem = %#v, want category=%q subtype=%q retryable=%v", problem, tt.category, tt.subtype, tt.retryable)
			}
			if !errors.Is(got, tt.err) {
				t.Fatalf("cause %v was not preserved", tt.err)
			}
		})
	}
}
