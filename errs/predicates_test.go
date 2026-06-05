// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errs_test

import (
	"fmt"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "api error with retryable=true",
			err:  &errs.APIError{Problem: errs.Problem{Category: errs.CategoryAPI, Retryable: true}},
			want: true,
		},
		{
			name: "api error with retryable=false (zero)",
			err:  &errs.APIError{Problem: errs.Problem{Category: errs.CategoryAPI}},
			want: false,
		},
		{
			name: "plain error",
			err:  fmt.Errorf("plain"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := errs.IsRetryable(tt.err); got != tt.want {
				t.Errorf("IsRetryable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsAuthTypedOnly(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "errs.AuthenticationError",
			err:  &errs.AuthenticationError{Problem: errs.Problem{Category: errs.CategoryAuthentication}},
			want: true,
		},
		{
			name: "errs.ConfigError",
			err:  &errs.ConfigError{Problem: errs.Problem{Category: errs.CategoryConfig}},
			want: false,
		},
		{
			name: "plain error",
			err:  fmt.Errorf("plain"),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := errs.IsAuthentication(tt.err); got != tt.want {
				t.Errorf("IsAuthentication(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsConfigTypedOnly(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "errs.ConfigError",
			err:  &errs.ConfigError{Problem: errs.Problem{Category: errs.CategoryConfig}},
			want: true,
		},
		{
			name: "errs.AuthenticationError",
			err:  &errs.AuthenticationError{Problem: errs.Problem{Category: errs.CategoryAuthentication}},
			want: false,
		},
		{
			name: "plain error",
			err:  fmt.Errorf("plain"),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := errs.IsConfig(tt.err); got != tt.want {
				t.Errorf("IsConfig(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestCategoryOf(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want errs.Category
	}{
		{
			name: "typed validation error",
			err:  &errs.ValidationError{Problem: errs.Problem{Category: errs.CategoryValidation}},
			want: errs.CategoryValidation,
		},
		{
			name: "typed permission error",
			err:  &errs.PermissionError{Problem: errs.Problem{Category: errs.CategoryAuthorization}},
			want: errs.CategoryAuthorization,
		},
		{
			name: "typed config error",
			err:  &errs.ConfigError{Problem: errs.Problem{Category: errs.CategoryConfig}},
			want: errs.CategoryConfig,
		},
		{
			name: "typed auth error",
			err:  &errs.AuthenticationError{Problem: errs.Problem{Category: errs.CategoryAuthentication}},
			want: errs.CategoryAuthentication,
		},
		{
			name: "plain error falls back to internal",
			err:  fmt.Errorf("plain"),
			want: errs.CategoryInternal,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := errs.CategoryOf(tt.err); got != tt.want {
				t.Errorf("CategoryOf(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

// TestProblemOf_NilProblemReturnsFalse pins that a problemCarrier whose
// ProblemDetail() returns nil does NOT satisfy ProblemOf — otherwise
// CategoryOf / IsRetryable and other downstream readers would dereference
// nil and panic. *Problem(nil) is a directly constructable trigger: its
// ProblemDetail method `return p` is nil-safe and yields nil.
func TestProblemOf_NilProblemReturnsFalse(t *testing.T) {
	var nilP *errs.Problem
	var err error = nilP // *Problem implements error via Error() (nil-receiver safe)

	p, ok := errs.ProblemOf(err)
	if ok {
		t.Fatalf("ProblemOf(*Problem(nil)) = (%v, true); want (nil, false)", p)
	}
	if p != nil {
		t.Errorf("ProblemOf(*Problem(nil)).p = %v; want nil", p)
	}

	// Downstream readers must not panic on the same input.
	if cat := errs.CategoryOf(err); cat != errs.CategoryInternal {
		t.Errorf("CategoryOf(*Problem(nil)) = %q, want fallback %q", cat, errs.CategoryInternal)
	}
	if retryable := errs.IsRetryable(err); retryable {
		t.Errorf("IsRetryable(*Problem(nil)) = true; want false")
	}
}

func TestTypedPredicates(t *testing.T) {
	cases := []struct {
		name string
		err  error
		pred func(error) bool
		want bool
	}{
		{"IsValidation+", &errs.ValidationError{}, errs.IsValidation, true},
		{"IsValidation-", &errs.APIError{}, errs.IsValidation, false},
		{"IsPermission+", &errs.PermissionError{}, errs.IsPermission, true},
		{"IsPermission-", &errs.APIError{}, errs.IsPermission, false},
		{"IsNetwork+", &errs.NetworkError{}, errs.IsNetwork, true},
		{"IsAPI+", &errs.APIError{}, errs.IsAPI, true},
		{"IsSecurityPolicy+", &errs.SecurityPolicyError{}, errs.IsSecurityPolicy, true},
		{"IsContentSafety+", &errs.ContentSafetyError{}, errs.IsContentSafety, true},
		{"IsInternal+", &errs.InternalError{}, errs.IsInternal, true},
		{"IsConfirmationRequired+", &errs.ConfirmationRequiredError{}, errs.IsConfirmationRequired, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.pred(tc.err); got != tc.want {
				t.Errorf("%s: predicate = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}
