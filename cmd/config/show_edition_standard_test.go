// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !extended

package config

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/runtimeplan"
)

func TestStandardConfigShowPreservesLocalConfigPath(t *testing.T) {
	f := newConfigFactoryWithExternalProvider(t)

	err := configShowRun(&ConfigShowOptions{Factory: f})
	problem, ok := errs.ProblemOf(err)
	if !ok ||
		problem.Category != errs.CategoryConfig ||
		problem.Subtype != errs.SubtypeNotConfigured {
		t.Fatalf("error = %#v, want established config/not_configured result", err)
	}
}

func TestStandardConfigShowReturnsTypedRuntimeStartupError(t *testing.T) {
	startupErr := errs.NewValidationError(
		errs.SubtypeFailedPrecondition,
		"system external credential configuration requires the lark-cli Extended edition",
	).WithHint("install lark-cli Extended or ask the administrator to remove external-credential.json")
	f, stdout, _, _ := cmdutil.TestFactoryWithRuntimePlan(
		t,
		nil,
		runtimeplan.Failed(startupErr, runtimeplan.MetadataEmbeddedOnly),
	)

	err := configShowRun(&ConfigShowOptions{Factory: f})
	if !errors.Is(err, startupErr) {
		t.Fatalf("config show error = %v, want original startup error", err)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok ||
		problem.Category != errs.CategoryValidation ||
		problem.Subtype != errs.SubtypeFailedPrecondition ||
		problem.Message != "system external credential configuration requires the lark-cli Extended edition" {
		t.Fatalf("config show problem = %#v, want typed Extended-required startup failure", problem)
	}
	if stdout.Len() != 0 {
		t.Fatalf("config show wrote local Profile after bootstrap failure: %s", stdout.String())
	}
}
