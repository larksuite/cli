// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/larksuite/cli/errs"
	larkauth "github.com/larksuite/cli/internal/auth"
)

func assertRegistrationProblem(t *testing.T, got, cause error, category errs.Category, subtype errs.Subtype) *errs.Problem {
	t.Helper()
	p, ok := errs.ProblemOf(got)
	if !ok {
		t.Fatalf("error %T is not typed: %v", got, got)
	}
	if p.Category != category || p.Subtype != subtype {
		t.Errorf("problem = (%q, %q), want (%q, %q)", p.Category, p.Subtype, category, subtype)
	}
	if !errors.Is(got, cause) {
		t.Errorf("error %v does not preserve cause %v", got, cause)
	}
	return p
}

func TestClassifyRegistrationBeginError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		category errs.Category
		subtype  errs.Subtype
	}{
		{"cancelled", context.Canceled, errs.CategoryAuthentication, errs.SubtypeUnknown},
		{"deadline", context.DeadlineExceeded, errs.CategoryNetwork, errs.SubtypeNetworkTimeout},
		{"transport", &net.DNSError{Err: "lookup failed", Name: "accounts.example"}, errs.CategoryNetwork, errs.SubtypeNetworkTransport},
		{"response", errors.New("response not JSON"), errs.CategoryAPI, errs.SubtypeUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertRegistrationProblem(t, classifyRegistrationBeginError(tt.err), tt.err, tt.category, tt.subtype)
		})
	}
}

func TestClassifyRegistrationError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		subtype errs.Subtype
		hint    bool
	}{
		{"denied", larkauth.ErrRegistrationDenied, errs.SubtypeUnknown, true},
		{"expired", larkauth.ErrRegistrationExpired, errs.SubtypeTokenExpired, true},
		{"timed-out", larkauth.ErrRegistrationTimedOut, errs.SubtypeTokenExpired, true},
		{"cancelled", context.Canceled, errs.SubtypeUnknown, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := assertRegistrationProblem(t, classifyRegistrationError(tt.err), tt.err, errs.CategoryAuthentication, tt.subtype)
			if (p.Hint != "") != tt.hint {
				t.Errorf("hint = %q, want non-empty=%v", p.Hint, tt.hint)
			}
		})
	}
}
