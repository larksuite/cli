// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestDashboardShareGetCallsResourceEndpoint(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/share",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"enabled":      true,
				"access_scope": "tenant",
				"settings": map[string]interface{}{
					"show_source":          true,
					"enable_auto_analysis": true,
				},
			},
		},
	})

	err := runShortcut(t, BaseDashboardShareGet, []string{
		"+dashboard-share-get",
		"--base-token", "app_x",
		"--dashboard-id", "dsh_1",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("run shortcut: %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"access_scope": "tenant"`) {
		t.Fatalf("stdout=%s", got)
	}
	if got := stdout.String(); !strings.Contains(got, `"show_source": true`) {
		t.Fatalf("stdout=%s", got)
	}
	if got := stdout.String(); strings.Contains(got, `enable_auto_analysis`) {
		t.Fatalf("stdout exposes backend-gated auto analysis setting: %s", got)
	}
}

func TestDashboardShareUpdatePreservesExplicitFalse(t *testing.T) {
	tests := []struct {
		name string
		flag string
		want map[string]interface{}
	}{
		{
			name: "show source",
			flag: "--show-source=false",
			want: map[string]interface{}{"settings": map[string]interface{}{"show_source": false}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, stdout, reg := newExecuteFactory(t)
			stub := &httpmock.Stub{
				Method: "PATCH",
				URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/share",
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{"enabled": true},
				},
			}
			reg.Register(stub)

			err := runShortcut(t, BaseDashboardShareUpdate, []string{
				"+dashboard-share-update",
				"--base-token", "app_x",
				"--dashboard-id", "dsh_1",
				tt.flag,
			}, factory, stdout)
			if err != nil {
				t.Fatalf("run shortcut: %v", err)
			}
			if got := decodeCapturedJSONBody(t, stub); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("request body=%#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestDashboardShareUpdateHidesAutoAnalysisFromResponse(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/share",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"enabled": true,
				"settings": map[string]interface{}{
					"show_source":          false,
					"enable_auto_analysis": true,
				},
			},
		},
	}
	reg.Register(stub)

	err := runShortcut(t, BaseDashboardShareUpdate, []string{
		"+dashboard-share-update",
		"--base-token", "app_x",
		"--dashboard-id", "dsh_1",
		"--show-source=false",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("run shortcut: %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"show_source": false`) {
		t.Fatalf("stdout=%s", got)
	}
	if got := stdout.String(); strings.Contains(got, `enable_auto_analysis`) {
		t.Fatalf("stdout exposes backend-gated auto analysis setting: %s", got)
	}
}

func TestDashboardShareUpdateDoesNotExposeAutoAnalysisFlag(t *testing.T) {
	for _, flag := range BaseDashboardShareUpdate.Flags {
		if flag.Name == "enable-auto-analysis" {
			t.Fatal("dashboard share update exposes backend-gated --enable-auto-analysis")
		}
	}
}

func TestDashboardShareUpdateBuildsCommonFields(t *testing.T) {
	tests := []struct {
		name  string
		flags []string
		want  map[string]interface{}
	}{
		{name: "enabled", flags: []string{"--enabled=true"}, want: map[string]interface{}{"enabled": true}},
		{name: "access scope", flags: []string{"--access-scope", "invite"}, want: map[string]interface{}{"access_scope": "invite"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, stdout, reg := newExecuteFactory(t)
			stub := &httpmock.Stub{
				Method: "PATCH",
				URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/share",
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{"enabled": true},
				},
			}
			reg.Register(stub)

			args := []string{
				"+dashboard-share-update",
				"--base-token", "app_x",
				"--dashboard-id", "dsh_1",
			}
			args = append(args, tt.flags...)
			if err := runShortcut(t, BaseDashboardShareUpdate, args, factory, stdout); err != nil {
				t.Fatalf("run shortcut: %v", err)
			}
			if got := decodeCapturedJSONBody(t, stub); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("request body=%#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestDashboardShareUpdatePreservesExplicitFalseForEnabled(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/share",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"enabled": false},
		},
	}
	reg.Register(stub)

	err := runShortcut(t, BaseDashboardShareUpdate, []string{
		"+dashboard-share-update",
		"--base-token", "app_x",
		"--dashboard-id", "dsh_1",
		"--enabled=false",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("run shortcut: %v", err)
	}

	want := map[string]interface{}{"enabled": false}
	if got := decodeCapturedJSONBody(t, stub); !reflect.DeepEqual(got, want) {
		t.Fatalf("request body=%#v, want %#v", got, want)
	}
}

func TestFormShareGetCallsResourceEndpoint(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_1/forms/vew_1/share",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"enabled":      false,
				"access_scope": "tenant",
			},
		},
	})

	err := runShortcut(t, BaseFormShareGet, []string{
		"+form-share-get",
		"--base-token", "app_x",
		"--table-id", "tbl_1",
		"--form-id", "vew_1",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("run shortcut: %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"enabled": false`) {
		t.Fatalf("stdout=%s", got)
	}
}

func TestFormShareUpdateBuildsAccessScope(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_1/forms/vew_1/share",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"enabled": true},
		},
	}
	reg.Register(stub)

	err := runShortcut(t, BaseFormShareUpdate, []string{
		"+form-share-update",
		"--base-token", "app_x",
		"--table-id", "tbl_1",
		"--form-id", "vew_1",
		"--access-scope", "anyone",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("run shortcut: %v", err)
	}

	want := map[string]interface{}{"access_scope": "anyone"}
	if got := decodeCapturedJSONBody(t, stub); !reflect.DeepEqual(got, want) {
		t.Fatalf("request body=%#v, want %#v", got, want)
	}
}

func TestFormShareUpdateBuildsSingleSettingsField(t *testing.T) {
	tests := []struct {
		name string
		flag string
		want map[string]interface{}
	}{
		{
			name: "allow anonymous false",
			flag: "--allow-anonymous=false",
			want: map[string]interface{}{"settings": map[string]interface{}{"allow_anonymous": false}},
		},
		{
			name: "require login true",
			flag: "--require-login=true",
			want: map[string]interface{}{"settings": map[string]interface{}{"require_login": true}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, stdout, reg := newExecuteFactory(t)
			stub := &httpmock.Stub{
				Method: "PATCH",
				URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_1/forms/vew_1/share",
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{"enabled": true},
				},
			}
			reg.Register(stub)

			err := runShortcut(t, BaseFormShareUpdate, []string{
				"+form-share-update",
				"--base-token", "app_x",
				"--table-id", "tbl_1",
				"--form-id", "vew_1",
				tt.flag,
			}, factory, stdout)
			if err != nil {
				t.Fatalf("run shortcut: %v", err)
			}
			if got := decodeCapturedJSONBody(t, stub); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("request body=%#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestFormShareUpdatePreservesExplicitFalseForEnabled(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_1/forms/vew_1/share",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"enabled": false},
		},
	}
	reg.Register(stub)

	err := runShortcut(t, BaseFormShareUpdate, []string{
		"+form-share-update",
		"--base-token", "app_x",
		"--table-id", "tbl_1",
		"--form-id", "vew_1",
		"--enabled=false",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("run shortcut: %v", err)
	}

	want := map[string]interface{}{"enabled": false}
	if got := decodeCapturedJSONBody(t, stub); !reflect.DeepEqual(got, want) {
		t.Fatalf("request body=%#v, want %#v", got, want)
	}
}

func TestDashboardShareUpdateRejectsMultipleFields(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	err := runShortcut(t, BaseDashboardShareUpdate, []string{
		"+dashboard-share-update",
		"--base-token", "app_x",
		"--dashboard-id", "dsh_1",
		"--enabled=false",
		"--access-scope", "tenant",
		"--show-source=true",
	}, factory, stdout)

	assertInvalidArgumentValidation(t, err, "--enabled", []string{"--enabled", "--access-scope", "--show-source"}, "exactly one")
}

func TestFormShareUpdateRejectsMultipleSettings(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	err := runShortcut(t, BaseFormShareUpdate, []string{
		"+form-share-update",
		"--base-token", "app_x",
		"--table-id", "tbl_1",
		"--form-id", "vew_1",
		"--allow-anonymous=true",
		"--require-login=true",
	}, factory, stdout)

	assertInvalidArgumentValidation(t, err, "--allow-anonymous", []string{"--allow-anonymous", "--require-login"}, "exactly one")
}

func TestShareUpdateRejectsUnsupportedAccessScope(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	err := runShortcut(t, BaseDashboardShareUpdate, []string{
		"+dashboard-share-update",
		"--base-token", "app_x",
		"--dashboard-id", "dsh_1",
		"--access-scope", "off",
	}, factory, stdout)

	assertInvalidArgumentValidation(t, err, "--access-scope", nil, "allowed")
}

func TestShareUpdateRequiresExactlyOneChange(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	err := runShortcut(t, BaseFormShareUpdate, []string{
		"+form-share-update",
		"--base-token", "app_x",
		"--table-id", "tbl_1",
		"--form-id", "vew_1",
	}, factory, stdout)

	assertInvalidArgumentValidation(t, err, "", []string{}, "exactly one")
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T %v", err, err)
	}
	for _, flag := range []string{
		"--enabled",
		"--access-scope",
		"--allow-anonymous",
		"--require-login",
	} {
		if !strings.Contains(problem.Hint, flag) {
			t.Fatalf("hint=%q, want flag %q", problem.Hint, flag)
		}
	}
}
