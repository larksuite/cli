// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
)

// Regression: decorateUserResolutionError previously gated the env-source
// hint on `strings.Contains(cfgErr.Message, "user")`. The substring match
// false-matched a profile-rung error whose hint contained "available
// users in this profile: ..." — a profile resolution failure on a typed
// $LARKSUITE_CLI_OPEN_ID got an "unset env or pass --user" suffix that
// pointed the operator at the wrong fix.
//
// Structural gate via core.ConfigError.Rung is robust to copy drift and
// to legitimate user-rung re-wordings.

func TestDecorateUserResolutionError_ProfileRung_NoEnvSuffix(t *testing.T) {
	// Profile rung error happens to mention "user" in its Hint copy.
	in := &core.ConfigError{
		Message: "profile \"ghost\" not found",
		Hint:    "available profiles: alpha, beta — pick one with --profile and run again",
		Rung:    core.RungProfile,
	}
	out := decorateUserResolutionError(in, "env")
	got, ok := out.(*core.ConfigError)
	if !ok {
		t.Fatalf("decorator must pass through; got %T", out)
	}
	if strings.Contains(got.Hint, envvars.CliOpenID) {
		t.Errorf("env suffix wrongly appended to profile-rung hint: %q", got.Hint)
	}
}

func TestDecorateUserResolutionError_UserRung_AppendsEnvSuffix(t *testing.T) {
	in := &core.ConfigError{
		Message: "user \"ou_alice\" not found in profile \"prod\"",
		Hint:    "available users in this profile: bob (ou_bob)",
		Rung:    core.RungUser,
	}
	out := decorateUserResolutionError(in, "env")
	got, ok := out.(*core.ConfigError)
	if !ok {
		t.Fatalf("expected *core.ConfigError; got %T", out)
	}
	if !strings.Contains(got.Hint, envvars.CliOpenID) {
		t.Errorf("user-rung env suffix missing; got hint=%q", got.Hint)
	}
}

func TestDecorateUserResolutionError_UserRung_FlagSource_NoDoubleHint(t *testing.T) {
	original := "available users in this profile: bob (ou_bob); --user already in copy"
	in := &core.ConfigError{
		Message: "user \"ou_alice\" not found in profile \"prod\"",
		Hint:    original,
		Rung:    core.RungUser,
	}
	out := decorateUserResolutionError(in, "flag")
	got, ok := out.(*core.ConfigError)
	if !ok {
		t.Fatalf("expected *core.ConfigError")
	}
	if got.Hint != original {
		t.Errorf("flag source must not modify hint; got %q, want %q", got.Hint, original)
	}
}

// Counter-test: a *core.ConfigError with no Rung tag (legacy) must NOT
// be decorated. Pre-fix the substring check would trigger on any "user"
// substring including documentation copy. Post-fix the structural gate
// only fires on RungUser.
func TestDecorateUserResolutionError_UntaggedConfigError_NoModify(t *testing.T) {
	original := "this happens to mention the word user in the hint"
	in := &core.ConfigError{
		Message: "some other failure",
		Hint:    original,
		Rung:    core.RungUnspecified,
	}
	out := decorateUserResolutionError(in, "env")
	got, ok := out.(*core.ConfigError)
	if !ok {
		t.Fatalf("expected *core.ConfigError")
	}
	if got.Hint != original {
		t.Errorf("untagged ConfigError must not be decorated; got %q", got.Hint)
	}
}

// Empty source short-circuits before any rung check.
func TestDecorateUserResolutionError_EmptySource_NoChange(t *testing.T) {
	in := &core.ConfigError{
		Hint: "h",
		Rung: core.RungUser,
	}
	out := decorateUserResolutionError(in, "")
	if out != in {
		t.Errorf("empty source must short-circuit identically; got new error")
	}
}
