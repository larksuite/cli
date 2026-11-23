// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func assertHintContains(t *testing.T, sc common.Shortcut, args []string, stub *httpmock.Stub, want string) {
	t.Helper()
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(stub)
	err := runAppsShortcut(t, sc, args, factory, stdout)
	if err == nil {
		t.Fatalf("expected failure, got nil; stdout=%s", stdout.String())
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed errs.Problem, got %T: %v", err, err)
	}
	if !strings.Contains(p.Hint, want) {
		t.Fatalf("hint %q does not contain %q", p.Hint, want)
	}
}

func TestAppsSessionCreate_4xxFailureCarriesListHint(t *testing.T) {
	assertHintContains(t, AppsSessionCreate,
		[]string{"+session-create", "--app-id", "app_x", "--as", "user"},
		&httpmock.Stub{Method: "POST", URL: "/open-apis/spark/v1/apps/app_x/sessions",
			Status: http.StatusNotFound, Body: map[string]interface{}{"msg": "app not found"}},
		"apps +list")
}

func TestAppsSessionList_4xxFailureCarriesListHint(t *testing.T) {
	assertHintContains(t, AppsSessionList,
		[]string{"+session-list", "--app-id", "app_x", "--as", "user"},
		&httpmock.Stub{Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/sessions",
			Status: http.StatusForbidden, Body: map[string]interface{}{"msg": "permission denied"}},
		"apps +list")
}

func TestAppsUpdate_4xxFailureCarriesListHint(t *testing.T) {
	assertHintContains(t, AppsUpdate,
		[]string{"+update", "--app-id", "app_x", "--name", "n", "--as", "user"},
		&httpmock.Stub{Method: "PATCH", URL: "/open-apis/spark/v1/apps/app_x",
			Status: http.StatusNotFound, Body: map[string]interface{}{"msg": "app not found"}},
		"apps +list")
}

func TestAppsReleaseList_4xxFailureCarriesListHint(t *testing.T) {
	assertHintContains(t, AppsReleaseList,
		[]string{"+release-list", "--app-id", "app_x", "--as", "user"},
		&httpmock.Stub{Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/releases",
			Status: http.StatusForbidden, Body: map[string]interface{}{"msg": "permission denied"}},
		"apps +list")
}

func TestAppsSessionStop_4xxFailureCarriesSessionHint(t *testing.T) {
	assertHintContains(t, AppsSessionStop,
		[]string{"+session-stop", "--app-id", "app_x", "--session-id", "s1", "--turn-id", "t1", "--as", "user"},
		&httpmock.Stub{Method: "POST", URL: "/open-apis/spark/v1/apps/app_x/sessions/s1/stop",
			Status: http.StatusNotFound, Body: map[string]interface{}{"msg": "session not found"}},
		"+session-list")
}

func TestAppsCreate_4xxFailureCarriesTypeHint(t *testing.T) {
	assertHintContains(t, AppsCreate,
		[]string{"+create", "--name", "n", "--app-type", "html", "--as", "user"},
		&httpmock.Stub{Method: "POST", URL: "/open-apis/spark/v1/apps",
			Status: http.StatusForbidden, Body: map[string]interface{}{"msg": "permission denied"}},
		"full_stack")
}

func TestAppsDBEnvCreate_4xxFailureCarriesHint(t *testing.T) {
	assertHintContains(t, AppsDBEnvCreate,
		[]string{"+db-env-create", "--app-id", "app_x", "--environment", "dev", "--yes", "--as", "user"},
		&httpmock.Stub{Method: "POST", URL: "/open-apis/spark/v1/apps/app_x/db_dev_init",
			Status: http.StatusConflict, Body: map[string]interface{}{"msg": "already multi-env"}},
		"+db-table-list")
}

func TestAppsDBTableGet_4xxFailureCarriesHint(t *testing.T) {
	assertHintContains(t, AppsDBTableGet,
		[]string{"+db-table-get", "--app-id", "app_x", "--table", "users", "--as", "user"},
		&httpmock.Stub{Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/tables/users",
			Status: http.StatusNotFound, Body: map[string]interface{}{"msg": "table not found"}},
		"+db-table-list")
}

func TestAppsDBTableList_4xxFailureCarriesHint(t *testing.T) {
	assertHintContains(t, AppsDBTableList,
		[]string{"+db-table-list", "--app-id", "app_x", "--environment", "dev", "--as", "user"},
		&httpmock.Stub{Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/tables",
			Status: http.StatusNotFound, Body: map[string]interface{}{"msg": "dev env not found"}},
		"+db-env-create")
}

// withAppsHint must only fill an EMPTY hint; an upstream-provided hint wins.
func TestWithAppsHint_DoesNotOverrideUpstreamHint(t *testing.T) {
	upstream := &errs.Problem{Message: "boom", Hint: "upstream specific hint"}
	got := withAppsHint(upstream, appIDListHint)
	p, ok := errs.ProblemOf(got)
	if !ok {
		t.Fatalf("expected typed problem, got %T", got)
	}
	if p.Hint != "upstream specific hint" {
		t.Fatalf("upstream hint was overridden: %q", p.Hint)
	}
}

// withAppsHint fills the hint when empty and leaves Message untouched.
func TestWithAppsHint_FillsEmptyHintKeepsMessage(t *testing.T) {
	p0 := &errs.Problem{Message: "boom"}
	got := withAppsHint(p0, appIDListHint)
	p, _ := errs.ProblemOf(got)
	if p.Hint != appIDListHint {
		t.Fatalf("hint not filled: %q", p.Hint)
	}
	if p.Message != "boom" {
		t.Fatalf("message mutated: %q", p.Message)
	}
}

// A failed_precondition failure must NOT inherit the command's request-shaped
// hint: by classification the request is valid, so "fix your request" advice is
// wrong. Regression guard for 221800 "miaoda UAT not activated", which used to be
// answered with the +db-execute table/column hint.
func TestWithAppsHint_WithholdsHintForFailedPrecondition(t *testing.T) {
	in := errs.NewValidationError(errs.SubtypeFailedPrecondition, "miaoda UAT not activated").WithCode(221800)
	out := withAppsHint(in, "verify table/column names with `lark-cli apps +db-table-get`")
	p, ok := errs.ProblemOf(out)
	if !ok {
		t.Fatalf("returned error is not typed: %T", out)
	}
	if p.Hint != "" {
		t.Fatalf("hint = %q, want empty: a request-shaped hint cannot explain a precondition failure", p.Hint)
	}
	// Only the hint is withheld — classification, code and message must survive so
	// the envelope still tells the caller what happened.
	if p.Category != errs.CategoryValidation || p.Subtype != errs.SubtypeFailedPrecondition ||
		p.Code != 221800 || p.Message != "miaoda UAT not activated" {
		t.Fatalf("classification mutated: category=%q subtype=%q code=%d msg=%q", p.Category, p.Subtype, p.Code, p.Message)
	}
}

// The gate has to hold on the REAL classification path, not just on a
// hand-built Problem. Every other gate test constructs
// errs.NewValidationError(SubtypeFailedPrecondition, ...) directly, which feeds
// the gate the input it wants and passes whether or not 221800 is registered in
// internal/errclass — a false negative that let an unregistered 221800 ship with
// this gate believed to be working. Drive it through BuildAPIError so the
// registration is part of what is asserted.
func TestWithAppsHint_WithholdsHintOnRealClassificationPath(t *testing.T) {
	err := errclass.BuildAPIError(map[string]any{
		"code": 221800,
		"msg":  "miaoda UAT not activated",
	}, errclass.ClassifyContext{Identity: "user"})

	p, ok := errs.ProblemOf(withAppsHint(err, "verify table/column names with `lark-cli apps +db-table-get`"))
	if !ok {
		t.Fatalf("BuildAPIError did not produce a typed problem: %#v", err)
	}
	// Registration is the precondition for the gate; assert it here so a missing
	// codemeta entry fails as a hint bug, which is how it is felt in practice.
	if p.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("subtype = %q, want failed_precondition: 221800 must be registered in internal/errclass for the gate to fire", p.Subtype)
	}
	if p.Hint != "" {
		t.Fatalf("hint = %q, want empty: the request-shaped hint must be withheld", p.Hint)
	}
}

// The gate declines to invent a hint; it must never drop one the upstream sent,
// nor touch the classification it came with.
func TestWithAppsHint_KeepsUpstreamHintOnFailedPrecondition(t *testing.T) {
	cause := errors.New("upstream cause")
	in := errs.NewValidationError(errs.SubtypeFailedPrecondition, "not activated").
		WithCode(221800).WithHint("activate Miaoda first").WithCause(cause)
	out := withAppsHint(in, "verify --app-id")
	p, ok := errs.ProblemOf(out)
	if !ok {
		t.Fatalf("returned error is not typed: %T", out)
	}
	if p.Hint != "activate Miaoda first" {
		t.Fatalf("hint = %q, want the upstream hint preserved", p.Hint)
	}
	if p.Category != errs.CategoryValidation || p.Subtype != errs.SubtypeFailedPrecondition || p.Code != 221800 {
		t.Fatalf("classification mutated: category=%q subtype=%q code=%d", p.Category, p.Subtype, p.Code)
	}
	if !errors.Is(out, cause) {
		t.Fatalf("cause chain dropped: %v", out)
	}
}

// The no-database recovery flow is itself a failed_precondition (400002465), and its
// override runs before the gate — so it must keep rewriting message and forcing its
// own accurate hint. Guards the ordering inside withAppsHint.
func TestWithAppsHint_NoDatabaseOverrideOutranksTheGate(t *testing.T) {
	cause := errors.New("upstream cause")
	in := errs.NewValidationError(errs.SubtypeFailedPrecondition, "workspace has no db branch").
		WithCode(appNoDatabaseCode).WithCause(cause)
	out := withAppsHint(in, "verify table/column names")
	p, ok := errs.ProblemOf(out)
	if !ok {
		t.Fatalf("returned error is not typed: %T", out)
	}
	if p.Message != appNoDatabaseMessage {
		t.Fatalf("message = %q, want the no-database rewrite %q", p.Message, appNoDatabaseMessage)
	}
	if p.Hint != appNoDatabaseHint {
		t.Fatalf("hint = %q, want the cloud-dev recovery hint", p.Hint)
	}
	// The override rewrites Message/Hint only — classification and cause stay put.
	if p.Category != errs.CategoryValidation || p.Subtype != errs.SubtypeFailedPrecondition || p.Code != appNoDatabaseCode {
		t.Fatalf("classification mutated: category=%q subtype=%q code=%d", p.Category, p.Subtype, p.Code)
	}
	if !errors.Is(out, cause) {
		t.Fatalf("cause chain dropped: %v", out)
	}
}

// Everything that is not a precondition keeps inheriting the command hint —
// including api/unknown, where most unclassified Spark codes still land, and the
// two classes other tests in this package assert on purpose (caller standing,
// transient upstream). Narrowing the gate further would fail here.
func TestWithAppsHint_FillsHintForOtherClasses(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      error
		category errs.Category
		subtype  errs.Subtype
		code     int
	}{
		{"api/not_found", errs.NewAPIError(errs.SubtypeNotFound, "table does not exist").WithCode(400002469),
			errs.CategoryAPI, errs.SubtypeNotFound, 400002469},
		{"api/unknown unclassified business code", errs.NewAPIError(errs.SubtypeUnknown, "boom").WithCode(999999),
			errs.CategoryAPI, errs.SubtypeUnknown, 999999},
		{"api/server_error", errs.NewAPIError(errs.SubtypeServerError, "upstream busy").WithCode(503),
			errs.CategoryAPI, errs.SubtypeServerError, 503},
		{"authentication/token_invalid", errs.NewAuthenticationError(errs.SubtypeTokenInvalid, "permission denied").WithCode(99991663),
			errs.CategoryAuthentication, errs.SubtypeTokenInvalid, 99991663},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := withAppsHint(tc.err, appIDListHint)
			p, ok := errs.ProblemOf(out)
			if !ok {
				t.Fatalf("returned error is not typed: %T", out)
			}
			if p.Hint != appIDListHint {
				t.Fatalf("hint = %q, want %q", p.Hint, appIDListHint)
			}
			// Filling the hint must not reclassify the failure.
			if p.Category != tc.category || p.Subtype != tc.subtype || p.Code != tc.code {
				t.Fatalf("classification mutated: category=%q subtype=%q code=%d", p.Category, p.Subtype, p.Code)
			}
		})
	}
}
