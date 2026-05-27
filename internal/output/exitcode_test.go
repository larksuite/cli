// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"fmt"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestExitCodeForCategory(t *testing.T) {
	cases := []struct {
		name string
		cat  errs.Category
		want int
	}{
		{"validation", errs.CategoryValidation, 2},
		{"authentication", errs.CategoryAuthentication, 3},
		{"authorization", errs.CategoryAuthorization, 3},
		{"config", errs.CategoryConfig, 3},
		{"network", errs.CategoryNetwork, 4},
		{"api", errs.CategoryAPI, 1},
		{"policy", errs.CategoryPolicy, 6},
		{"internal", errs.CategoryInternal, 5},
		{"confirmation", errs.CategoryConfirmation, 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExitCodeForCategory(tc.cat); got != tc.want {
				t.Errorf("ExitCodeForCategory(%q) = %d, want %d", tc.cat, got, tc.want)
			}
		})
	}
}

func TestExitCodeForCategory_UnknownDefaults(t *testing.T) {
	if got := ExitCodeForCategory(errs.Category("not_a_real_category")); got != ExitInternal {
		t.Errorf("ExitCodeForCategory(unknown) = %d, want %d (ExitInternal)", got, ExitInternal)
	}
}

func TestExitCodeOf_Nil(t *testing.T) {
	if got := ExitCodeOf(nil); got != 0 {
		t.Errorf("ExitCodeOf(nil) = %d, want 0", got)
	}
}

func TestExitCodeOf_PermissionError(t *testing.T) {
	err := &errs.PermissionError{Problem: errs.Problem{Category: errs.CategoryAuthorization}}
	if got := ExitCodeOf(err); got != 3 {
		t.Errorf("ExitCodeOf(PermissionError) = %d, want 3", got)
	}
}

func TestExitCodeOf_APIError(t *testing.T) {
	err := &errs.APIError{Problem: errs.Problem{Category: errs.CategoryAPI}}
	if got := ExitCodeOf(err); got != 1 {
		t.Errorf("ExitCodeOf(APIError) = %d, want 1", got)
	}
}

func TestExitCodeOf_UntypedFallsBackToInternal(t *testing.T) {
	if got := ExitCodeOf(fmt.Errorf("plain")); got != 5 {
		t.Errorf("ExitCodeOf(plain) = %d, want 5 (untyped → CategoryInternal → ExitInternal)", got)
	}
}
