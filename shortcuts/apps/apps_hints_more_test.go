// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
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
