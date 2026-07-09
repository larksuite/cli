// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

// TestAppsEnvPull_4xxFailureCarriesListHint verifies that a 4xx failure from the
// env_vars endpoint surfaces an actionable hint pointing at `lark-cli apps +list`.
func TestAppsEnvPull_4xxFailureCarriesListHint(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/env_vars",
		Status: http.StatusForbidden,
		Body:   map[string]interface{}{"msg": "permission denied"},
		OnMatch: func(req *http.Request) {
			assertEnvPullBody(t, req)
		},
	})

	err := runAppsShortcut(t, AppsEnvPull,
		[]string{"+env-pull", "--app-id", "app_x", "--project-path", t.TempDir(), "--as", "user"},
		factory, stdout)
	if err == nil {
		t.Fatalf("expected failure, got nil; stdout=%s", stdout.String())
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed errs.Problem, got %T: %v", err, err)
	}
	if !strings.Contains(p.Hint, "apps +list") {
		t.Fatalf("hint missing `apps +list`: %q", p.Hint)
	}
}

// TestAppsAccessScopeGet_4xxFailureCarriesListHint verifies the access-scope-get
// 4xx failure points at `lark-cli apps +list`.
func TestAppsAccessScopeGet_4xxFailureCarriesListHint(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/access-scope",
		Status: http.StatusNotFound,
		Body:   map[string]interface{}{"msg": "app not found"},
	})

	err := runAppsShortcut(t, AppsAccessScopeGet,
		[]string{"+access-scope-get", "--app-id", "app_x", "--as", "user"},
		factory, stdout)
	if err == nil {
		t.Fatalf("expected failure, got nil; stdout=%s", stdout.String())
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed errs.Problem, got %T: %v", err, err)
	}
	if !strings.Contains(p.Hint, "apps +list") {
		t.Fatalf("hint missing `apps +list`: %q", p.Hint)
	}
}

// TestAppsAccessScopeSet_4xxFailureCarriesScopeGetHint verifies the
// access-scope-set 4xx failure points at `+access-scope-get`.
func TestAppsAccessScopeSet_4xxFailureCarriesScopeGetHint(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/spark/v1/apps/app_x/access-scope",
		Status: http.StatusBadRequest,
		Body:   map[string]interface{}{"msg": "invalid target id"},
	})

	err := runAppsShortcut(t, AppsAccessScopeSet,
		[]string{"+access-scope-set", "--app-id", "app_x", "--scope", "tenant", "--as", "user"},
		factory, stdout)
	if err == nil {
		t.Fatalf("expected failure, got nil; stdout=%s", stdout.String())
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed errs.Problem, got %T: %v", err, err)
	}
	if !strings.Contains(p.Hint, "+access-scope-get") {
		t.Fatalf("hint missing `+access-scope-get`: %q", p.Hint)
	}
}
