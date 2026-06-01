// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errscontract

import (
	"strings"
	"testing"
)

// Tests for the four typed-error invariant rules:
//   - CheckNilSafeError
//   - CheckUnwrapSymmetry
//   - CheckBuilderImmutable
//   - CheckBuildAPIErrorArms
//
// Each rule gets a "rejects bad shape" + "accepts compliant shape" pair so
// future regressions reveal themselves immediately. Fixtures use the
// minimal canonical Problem-embedder shape and run through the public
// CheckXxx entry points — no internal helpers are exercised directly.

// =============================== CheckNilSafeError ===========================

func TestCheckNilSafeError_FlagsMissingOverride(t *testing.T) {
	// BadError embeds Problem by value but defines no own Error() —
	// the promoted Error() panics on a typed-nil interface holder.
	src := `package errs

type Problem struct{}
func (p *Problem) Error() string { return "" }

type BadError struct {
	Problem
}
`
	v := CheckNilSafeError("errs/types.go", src)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(v), v)
	}
	if v[0].Action != ActionReject {
		t.Errorf("action = %q, want REJECT", v[0].Action)
	}
	if !strings.Contains(v[0].Message, "BadError") {
		t.Errorf("message must name the type: %s", v[0].Message)
	}
}

func TestCheckNilSafeError_FlagsMissingNilGuard(t *testing.T) {
	// Error() exists but the first statement is not the nil-receiver guard.
	src := `package errs

type Problem struct{}
func (p *Problem) Error() string { return "" }

type BadError struct {
	Problem
}

func (e *BadError) Error() string {
	return e.Problem.Error()
}
`
	v := CheckNilSafeError("errs/types.go", src)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(v), v)
	}
	if !strings.Contains(v[0].Message, "nil-receiver guard") {
		t.Errorf("message should call out the missing nil guard: %s", v[0].Message)
	}
}

func TestCheckNilSafeError_AcceptsCompliantOverride(t *testing.T) {
	src := `package errs

type Problem struct{}
func (p *Problem) Error() string { return "" }

type GoodError struct {
	Problem
}

func (e *GoodError) Error() string {
	if e == nil {
		return ""
	}
	return e.Problem.Error()
}
`
	v := CheckNilSafeError("errs/types.go", src)
	if len(v) != 0 {
		t.Errorf("compliant nil-safe Error() must pass, got: %+v", v)
	}
}

func TestCheckNilSafeError_ScopedToErrsPackage(t *testing.T) {
	// Same violating fixture outside errs/ — must NOT fire (the typed
	// taxonomy is errs/-only).
	src := `package custom

type Problem struct{}
func (p *Problem) Error() string { return "" }

type BadError struct {
	Problem
}
`
	v := CheckNilSafeError("internal/custom/x.go", src)
	if len(v) != 0 {
		t.Errorf("CheckNilSafeError must scope to errs/ only, got: %+v", v)
	}
}

// ============================== CheckUnwrapSymmetry ==========================

func TestCheckUnwrapSymmetry_FlagsMissingUnwrap(t *testing.T) {
	src := `package errs

type Problem struct{}
func (p *Problem) Error() string { return "" }

type BadError struct {
	Problem
	Cause error
}
`
	v := CheckUnwrapSymmetry("errs/types.go", src)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(v), v)
	}
	if v[0].Action != ActionReject {
		t.Errorf("action = %q, want REJECT", v[0].Action)
	}
	if !strings.Contains(v[0].Message, "BadError") {
		t.Errorf("message must name the type: %s", v[0].Message)
	}
}

func TestCheckUnwrapSymmetry_FlagsUnwrapWithoutNilGuard(t *testing.T) {
	src := `package errs

type Problem struct{}
func (p *Problem) Error() string { return "" }

type BadError struct {
	Problem
	Cause error
}

func (e *BadError) Unwrap() error {
	return e.Cause
}
`
	v := CheckUnwrapSymmetry("errs/types.go", src)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(v), v)
	}
	if !strings.Contains(v[0].Message, "nil-receiver guard") {
		t.Errorf("message should call out the missing nil guard: %s", v[0].Message)
	}
}

func TestCheckUnwrapSymmetry_AcceptsCompliantUnwrap(t *testing.T) {
	src := `package errs

type Problem struct{}
func (p *Problem) Error() string { return "" }

type GoodError struct {
	Problem
	Cause error
}

func (e *GoodError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
`
	v := CheckUnwrapSymmetry("errs/types.go", src)
	if len(v) != 0 {
		t.Errorf("compliant Unwrap() must pass, got: %+v", v)
	}
}

// ============================= CheckBuilderImmutable =========================

func TestCheckBuilderImmutable_FlagsBareSliceAssignment(t *testing.T) {
	src := `package errs

type Problem struct{}

type PermissionError struct {
	Problem
	MissingScopes []string
}

func (e *PermissionError) WithMissingScopes(scopes []string) *PermissionError {
	e.MissingScopes = scopes
	return e
}
`
	v := CheckBuilderImmutable("errs/types.go", src)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(v), v)
	}
	if v[0].Action != ActionReject {
		t.Errorf("action = %q, want REJECT", v[0].Action)
	}
	if !strings.Contains(v[0].Message, "WithMissingScopes") || !strings.Contains(v[0].Message, "MissingScopes") {
		t.Errorf("message should name the builder and field: %s", v[0].Message)
	}
}

func TestCheckBuilderImmutable_FlagsBareVariadicAssignment(t *testing.T) {
	// Variadic slice has the same aliasing hazard.
	src := `package errs

type Problem struct{}

type PermissionError struct {
	Problem
	MissingScopes []string
}

func (e *PermissionError) WithMissingScopes(scopes ...string) *PermissionError {
	e.MissingScopes = scopes
	return e
}
`
	v := CheckBuilderImmutable("errs/types.go", src)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(v), v)
	}
}

func TestCheckBuilderImmutable_FlagsBareMapAssignment(t *testing.T) {
	src := `package errs

type Problem struct{}

type APIError struct {
	Problem
	Detail map[string]any
}

func (e *APIError) WithDetail(detail map[string]any) *APIError {
	e.Detail = detail
	return e
}
`
	v := CheckBuilderImmutable("errs/types.go", src)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(v), v)
	}
	if !strings.Contains(v[0].Message, "WithDetail") {
		t.Errorf("message should name the builder: %s", v[0].Message)
	}
}

func TestCheckBuilderImmutable_AcceptsClonedSlice(t *testing.T) {
	src := `package errs

import "slices"

type Problem struct{}

type PermissionError struct {
	Problem
	MissingScopes []string
}

func (e *PermissionError) WithMissingScopes(scopes ...string) *PermissionError {
	e.MissingScopes = slices.Clone(scopes)
	return e
}
`
	v := CheckBuilderImmutable("errs/types.go", src)
	if len(v) != 0 {
		t.Errorf("slices.Clone wrap must pass, got: %+v", v)
	}
}

func TestCheckBuilderImmutable_AcceptsClonedMap(t *testing.T) {
	src := `package errs

import "maps"

type Problem struct{}

type APIError struct {
	Problem
	Detail map[string]any
}

func (e *APIError) WithDetail(detail map[string]any) *APIError {
	e.Detail = maps.Clone(detail)
	return e
}
`
	v := CheckBuilderImmutable("errs/types.go", src)
	if len(v) != 0 {
		t.Errorf("maps.Clone wrap must pass, got: %+v", v)
	}
}

func TestCheckBuilderImmutable_IgnoresScalarSetters(t *testing.T) {
	// Scalar / string setters are not reference-typed; the rule must not
	// false-positive on them.
	src := `package errs

type Problem struct{}

type ConfigError struct {
	Problem
	Field string
}

func (e *ConfigError) WithField(field string) *ConfigError {
	e.Field = field
	return e
}

func (e *ConfigError) WithCode(code int) *ConfigError {
	e.Code = code
	return e
}
`
	v := CheckBuilderImmutable("errs/types.go", src)
	if len(v) != 0 {
		t.Errorf("scalar setters must not fire, got: %+v", v)
	}
}

// ============================ CheckBuildAPIErrorArms =========================

func TestCheckBuildAPIErrorArms_FlagsMissingCategory(t *testing.T) {
	// Switch is missing the CategoryConfirmation arm.
	src := `package errclass

import "github.com/larksuite/cli/errs"

type ClassifyContext struct{}

func BuildAPIError(resp map[string]any, cc ClassifyContext) error {
	var cat errs.Category
	switch cat {
	case errs.CategoryValidation:
		return &errs.ValidationError{}
	case errs.CategoryAuthentication:
		return &errs.AuthenticationError{}
	case errs.CategoryAuthorization:
		return &errs.PermissionError{}
	case errs.CategoryConfig:
		return &errs.ConfigError{}
	case errs.CategoryNetwork:
		return &errs.NetworkError{}
	case errs.CategoryAPI:
		return &errs.APIError{}
	case errs.CategoryPolicy:
		return &errs.SecurityPolicyError{}
	case errs.CategoryInternal:
		return &errs.InternalError{}
	default:
		return &errs.InternalError{}
	}
}
`
	v := CheckBuildAPIErrorArms("internal/errclass/classify.go", src)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation (missing CategoryConfirmation), got %d: %+v", len(v), v)
	}
	if !strings.Contains(v[0].Message, "CategoryConfirmation") {
		t.Errorf("message should name the missing Category: %s", v[0].Message)
	}
}

func TestCheckBuildAPIErrorArms_FlagsMissingDefault(t *testing.T) {
	src := `package errclass

import "github.com/larksuite/cli/errs"

type ClassifyContext struct{}

func BuildAPIError(resp map[string]any, cc ClassifyContext) error {
	var cat errs.Category
	switch cat {
	case errs.CategoryValidation:
		return &errs.ValidationError{}
	case errs.CategoryAuthentication:
		return &errs.AuthenticationError{}
	case errs.CategoryAuthorization:
		return &errs.PermissionError{}
	case errs.CategoryConfig:
		return &errs.ConfigError{}
	case errs.CategoryNetwork:
		return &errs.NetworkError{}
	case errs.CategoryAPI:
		return &errs.APIError{}
	case errs.CategoryPolicy:
		return &errs.SecurityPolicyError{}
	case errs.CategoryInternal:
		return &errs.InternalError{}
	case errs.CategoryConfirmation:
		return &errs.ConfirmationRequiredError{}
	}
	return nil
}
`
	v := CheckBuildAPIErrorArms("internal/errclass/classify.go", src)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation (missing default arm), got %d: %+v", len(v), v)
	}
	if !strings.Contains(v[0].Message, "no default arm") {
		t.Errorf("message should call out the missing default: %s", v[0].Message)
	}
}

func TestCheckBuildAPIErrorArms_FlagsNilReturningDefault(t *testing.T) {
	src := `package errclass

import "github.com/larksuite/cli/errs"

type ClassifyContext struct{}

func BuildAPIError(resp map[string]any, cc ClassifyContext) error {
	var cat errs.Category
	switch cat {
	case errs.CategoryValidation:
		return &errs.ValidationError{}
	case errs.CategoryAuthentication:
		return &errs.AuthenticationError{}
	case errs.CategoryAuthorization:
		return &errs.PermissionError{}
	case errs.CategoryConfig:
		return &errs.ConfigError{}
	case errs.CategoryNetwork:
		return &errs.NetworkError{}
	case errs.CategoryAPI:
		return &errs.APIError{}
	case errs.CategoryPolicy:
		return &errs.SecurityPolicyError{}
	case errs.CategoryInternal:
		return &errs.InternalError{}
	case errs.CategoryConfirmation:
		return &errs.ConfirmationRequiredError{}
	default:
		return nil
	}
}
`
	v := CheckBuildAPIErrorArms("internal/errclass/classify.go", src)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation (default returns nil), got %d: %+v", len(v), v)
	}
	if !strings.Contains(v[0].Message, "default arm") {
		t.Errorf("message should call out the default arm: %s", v[0].Message)
	}
}

func TestCheckBuildAPIErrorArms_AcceptsCompliantSwitch(t *testing.T) {
	src := `package errclass

import "github.com/larksuite/cli/errs"

type ClassifyContext struct{}

func BuildAPIError(resp map[string]any, cc ClassifyContext) error {
	var cat errs.Category
	switch cat {
	case errs.CategoryValidation:
		return &errs.ValidationError{}
	case errs.CategoryAuthentication:
		return &errs.AuthenticationError{}
	case errs.CategoryAuthorization:
		return &errs.PermissionError{}
	case errs.CategoryConfig:
		return &errs.ConfigError{}
	case errs.CategoryNetwork:
		return &errs.NetworkError{}
	case errs.CategoryAPI:
		return &errs.APIError{}
	case errs.CategoryPolicy:
		return &errs.SecurityPolicyError{}
	case errs.CategoryInternal:
		return &errs.InternalError{}
	case errs.CategoryConfirmation:
		return &errs.ConfirmationRequiredError{}
	default:
		return &errs.InternalError{}
	}
}
`
	v := CheckBuildAPIErrorArms("internal/errclass/classify.go", src)
	if len(v) != 0 {
		t.Errorf("compliant switch must pass, got: %+v", v)
	}
}

func TestCheckBuildAPIErrorArms_RejectsWrongCategoryDefault(t *testing.T) {
	src := `package errclass

import "github.com/larksuite/cli/errs"

type ClassifyContext struct{}

func BuildAPIError(resp map[string]any, cc ClassifyContext) error {
	var cat errs.Category
	switch cat {
	case errs.CategoryValidation:
		return &errs.ValidationError{}
	case errs.CategoryAuthentication:
		return &errs.AuthenticationError{}
	case errs.CategoryAuthorization:
		return &errs.PermissionError{}
	case errs.CategoryConfig:
		return &errs.ConfigError{}
	case errs.CategoryNetwork:
		return &errs.NetworkError{}
	case errs.CategoryAPI:
		return &errs.APIError{}
	case errs.CategoryPolicy:
		return &errs.SecurityPolicyError{}
	case errs.CategoryInternal:
		return &errs.InternalError{}
	case errs.CategoryConfirmation:
		return &errs.ConfirmationRequiredError{}
	default:
		return &errs.APIError{}
	}
}
`
	v := CheckBuildAPIErrorArms("internal/errclass/classify.go", src)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation (wrong-type default), got %d: %+v", len(v), v)
	}
	if !strings.Contains(v[0].Message, "InternalError") {
		t.Errorf("violation must call out InternalError requirement: %s", v[0].Message)
	}
}

func TestCheckBuildAPIErrorArms_AcceptsNewInternalErrorConstructor(t *testing.T) {
	src := `package errclass

import "github.com/larksuite/cli/errs"

type ClassifyContext struct{}

func BuildAPIError(resp map[string]any, cc ClassifyContext) error {
	var cat errs.Category
	switch cat {
	case errs.CategoryValidation:
		return &errs.ValidationError{}
	case errs.CategoryAuthentication:
		return &errs.AuthenticationError{}
	case errs.CategoryAuthorization:
		return &errs.PermissionError{}
	case errs.CategoryConfig:
		return &errs.ConfigError{}
	case errs.CategoryNetwork:
		return &errs.NetworkError{}
	case errs.CategoryAPI:
		return &errs.APIError{}
	case errs.CategoryPolicy:
		return &errs.SecurityPolicyError{}
	case errs.CategoryInternal:
		return &errs.InternalError{}
	case errs.CategoryConfirmation:
		return &errs.ConfirmationRequiredError{}
	default:
		return errs.NewInternalError(errs.SubtypeSDKError, "unknown category")
	}
}
`
	v := CheckBuildAPIErrorArms("internal/errclass/classify.go", src)
	if len(v) != 0 {
		t.Errorf("constructor form must be accepted, got: %+v", v)
	}
}

func TestCheckBuildAPIErrorArms_ScopedToClassifyFile(t *testing.T) {
	// Identical violating shape outside the canonical path — must NOT fire.
	src := `package custom

import "github.com/larksuite/cli/errs"

func BuildAPIError(resp map[string]any) error {
	var cat errs.Category
	switch cat {
	case errs.CategoryValidation:
		return nil
	}
	return nil
}
`
	v := CheckBuildAPIErrorArms("internal/foo/other.go", src)
	if len(v) != 0 {
		t.Errorf("rule must scope to internal/errclass/classify.go, got: %+v", v)
	}
}
