// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"errors"
	"fmt"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// updateExistingProfileWithoutSecret guards four blank-input scenarios. Each
// must surface as *ValidationError(SubtypeInvalidArgument) per RFC 6749 §5.2:
// SubtypeInvalidClient is reserved for IAM rejection of malformed credentials,
// not for missing user input.

func TestUpdateExistingProfileWithoutSecret_NilConfig_EmitsValidationError(t *testing.T) {
	err := updateExistingProfileWithoutSecret(nil, "", "cli_test", core.BrandFeishu, "en")
	assertValidationParam(t, err, "--app-secret")
}

func TestUpdateExistingProfileWithoutSecret_UnknownProfile_EmitsValidationError(t *testing.T) {
	existing := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			Name:      "default",
			AppId:     "app-default",
			AppSecret: core.PlainSecret("secret-default"),
			Brand:     core.BrandFeishu,
		}},
	}
	err := updateExistingProfileWithoutSecret(existing, "missing-profile", "cli_test", core.BrandFeishu, "en")
	assertValidationParam(t, err, "--app-secret")
}

func TestUpdateExistingProfileWithoutSecret_NoCurrentApp_EmitsValidationError(t *testing.T) {
	existing := &core.MultiAppConfig{
		CurrentApp: "missing",
		Apps: []core.AppConfig{{
			Name:      "default",
			AppId:     "app-default",
			AppSecret: core.PlainSecret("secret-default"),
			Brand:     core.BrandFeishu,
		}},
	}
	err := updateExistingProfileWithoutSecret(existing, "", "cli_test", core.BrandFeishu, "en")
	assertValidationParam(t, err, "--app-secret")
}

func TestUpdateExistingProfileWithoutSecret_AppIdMismatch_EmitsValidationError(t *testing.T) {
	existing := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			Name:      "default",
			AppId:     "app-default",
			AppSecret: core.PlainSecret("secret-default"),
			Brand:     core.BrandFeishu,
		}},
	}
	err := updateExistingProfileWithoutSecret(existing, "", "cli_different", core.BrandFeishu, "en")
	assertValidationParam(t, err, "--app-secret")
}

// wrapUpdateExistingProfileErr is the caller-side classifier for the error
// returned by updateExistingProfileWithoutSecret. It must preserve typed-error
// exit semantics: a typed ValidationError must keep ExitValidation rather than
// being downgraded to InternalError.

func TestWrapUpdateExistingProfileErr_NilPassesThrough(t *testing.T) {
	if got := wrapUpdateExistingProfileErr(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestWrapUpdateExistingProfileErr_TypedValidationErrorPreserved(t *testing.T) {
	in := errs.NewValidationError(errs.SubtypeInvalidArgument, "App Secret cannot be empty for new profile").
		WithParam("--app-secret")
	got := wrapUpdateExistingProfileErr(in)
	assertValidationParam(t, got, "--app-secret")
	// Exit code must remain ExitValidation (2), not ExitInternal (5).
	if code := output.ExitCodeOf(got); code != output.ExitValidation {
		t.Errorf("ExitCodeOf = %d, want %d (ExitValidation)", code, output.ExitValidation)
	}
	// Must NOT be wrapped as *InternalError.
	var intErr *errs.InternalError
	if errors.As(got, &intErr) {
		t.Errorf("typed ValidationError was downgraded to *InternalError: %v", got)
	}
}

func TestWrapUpdateExistingProfileErr_UntypedErrorBecomesInternal(t *testing.T) {
	in := fmt.Errorf("disk full")
	got := wrapUpdateExistingProfileErr(in)
	var intErr *errs.InternalError
	if !errors.As(got, &intErr) {
		t.Fatalf("expected *errs.InternalError, got %T: %v", got, got)
	}
	if intErr.Subtype != errs.SubtypeSDKError {
		t.Errorf("Subtype = %q, want %q", intErr.Subtype, errs.SubtypeSDKError)
	}
}

// assertValidationParam asserts err is *ValidationError with the given Param.
func assertValidationParam(t *testing.T, err error, wantParam string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var valErr *errs.ValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if valErr.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("Subtype = %q, want %q", valErr.Subtype, errs.SubtypeInvalidArgument)
	}
	if valErr.Param != wantParam {
		t.Errorf("Param = %q, want %q", valErr.Param, wantParam)
	}
}
