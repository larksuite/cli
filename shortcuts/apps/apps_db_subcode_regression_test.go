// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"strconv"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestAppsDBPreconditionSubcodes_CommandRecovery(t *testing.T) {
	tests := []struct {
		name       string
		shortcut   common.Shortcut
		method     string
		url        string
		args       []string
		code       int
		message    string
		hintHas    []string
		hintReject []string
	}{
		{
			name:     "db table list single-env dev branch",
			shortcut: AppsDBTableList,
			method:   "GET",
			url:      "/open-apis/spark/v1/apps/app_x/tables",
			args:     []string{"+db-table-list", "--app-id", "app_x", "--environment", "dev", "--as", "user"},
			code:     400002476,
			message:  "k_dl_1600000：Multi-environment database is not initialized for this app. Invalid DB Branch：dev",
			hintHas:  []string{"single-env", "--environment online", "+db-env-create"},
			hintReject: []string{
				"verify --app-id",
			},
		},
		{
			name:     "db audit list audit disabled",
			shortcut: AppsDBAuditList,
			method:   "GET",
			url:      "/open-apis/spark/v1/apps/app_x/db/audit_list",
			args:     []string{"+db-audit-list", "--app-id", "app_x", "--table", "orders", "--as", "user"},
			code:     400002476,
			message:  "k_dl_4000028：audit not enabled for table orders",
			hintHas:  []string{"+db-audit-enable", "--table <table>", "+db-audit-status"},
			hintReject: []string{
				"verify --app-id",
			},
		},
		{
			name:     "db env diff multi-env not initialized",
			shortcut: AppsDBEnvDiff,
			method:   "POST",
			url:      "/open-apis/spark/v1/apps/app_x/db/env_migrate",
			args:     []string{"+db-env-diff", "--app-id", "app_x", "--as", "user"},
			code:     400002476,
			message:  "k_dl_4000027：Multi-env is not initialized",
			hintHas:  []string{"not initialized", "no diff or migration", "+db-env-create"},
			hintReject: []string{
				"pending dev changes",
			},
		},
		{
			name:     "db env diff creative mode unsupported",
			shortcut: AppsDBEnvDiff,
			method:   "POST",
			url:      "/open-apis/spark/v1/apps/app_x/db/env_migrate",
			args:     []string{"+db-env-diff", "--app-id", "app_x", "--as", "user"},
			code:     400002476,
			message:  "k_dl_4000022：this command is only available for expert mode applications",
			hintHas:  []string{"full_stack", "creative-mode", "+db-env-diff", "+db-env-migrate"},
			hintReject: []string{
				"pending dev changes",
			},
		},
		{
			name:     "cache clear single-env dev branch",
			shortcut: AppsCacheClear,
			method:   "POST",
			url:      "/open-apis/spark/v1/apps/app_x/cache/clear",
			args:     []string{"+cache-clear", "--app-id", "app_x", "--environment", "dev", "--yes", "--as", "user"},
			code:     400002476,
			message:  "k_dl_1600000：Invalid DB Branch：dev",
			hintHas:  []string{"single-env", "--environment online"},
			hintReject: []string{
				"verify --app-id",
			},
		},
		{
			name:     "cache get single-env dev branch",
			shortcut: AppsCacheGet,
			method:   "GET",
			url:      "/open-apis/spark/v1/apps/app_x/cache",
			args:     []string{"+cache-get", "--app-id", "app_x", "--environment", "dev", "--key", "k:1", "--as", "user"},
			code:     400002476,
			message:  "k_dl_1600000：Invalid DB Branch：dev",
			hintHas:  []string{"single-env", "--environment online"},
			hintReject: []string{
				"verify --app-id",
			},
		},
		{
			name:     "cache delete single-env dev branch",
			shortcut: AppsCacheDelete,
			method:   "DELETE",
			url:      "/open-apis/spark/v1/apps/app_x/cache",
			args:     []string{"+cache-delete", "--app-id", "app_x", "--environment", "dev", "--key", "k:1", "--as", "user"},
			code:     400002476,
			message:  "k_dl_1600000：Invalid DB Branch：dev",
			hintHas:  []string{"single-env", "--environment online"},
			hintReject: []string{
				"verify --app-id",
			},
		},
		{
			name:     "db sync create single-env dev branch",
			shortcut: AppsDBSyncCreate,
			method:   "POST",
			url:      dbSyncURL,
			args:     []string{"+db-sync-create", "--app-id", "app_x", "--environment", "dev", "--config", dbSyncCommitConfig, "--yes", "--as", "user"},
			code:     400002476,
			message:  "k_dl_1600000：Invalid DB Branch：dev",
			hintHas:  []string{"single-env", "--environment online"},
			hintReject: []string{
				"after fixing the config",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, stdout, reg := newAppsExecuteFactory(t)
			reg.Register(&httpmock.Stub{
				Method: tt.method,
				URL:    tt.url,
				Body:   map[string]interface{}{"code": tt.code, "msg": tt.message},
			})

			err := runAppsShortcut(t, tt.shortcut, tt.args, factory, stdout)
			if err == nil {
				t.Fatal("expected database precondition error")
			}
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("error = %T %v, want typed problem", err, err)
			}
			if p.Category != errs.CategoryValidation || p.Subtype != errs.SubtypeFailedPrecondition {
				t.Fatalf("category/subtype = %s/%s, want validation/failed_precondition", p.Category, p.Subtype)
			}
			if got := output.ExitCodeOf(err); got != output.ExitValidation {
				t.Fatalf("exit code = %d, want %d", got, output.ExitValidation)
			}
			for _, want := range tt.hintHas {
				if !strings.Contains(p.Hint, want) {
					t.Errorf("hint = %q, want it to contain %q", p.Hint, want)
				}
			}
			for _, rejected := range tt.hintReject {
				if strings.Contains(p.Hint, rejected) {
					t.Errorf("hint = %q, must not contain stale guidance %q", p.Hint, rejected)
				}
			}
			if strings.Contains(p.Message, dbSubcodePrefix) {
				t.Errorf("message = %q, internal subcode prefix must be stripped", p.Message)
			}
		})
	}
}

func TestAppsDBSyncCreate_OnlineDDLSubcodeOuterCodes(t *testing.T) {
	for _, code := range []int{400002476, 500002776} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			factory, stdout, reg := newAppsExecuteFactory(t)
			reg.Register(&httpmock.Stub{
				Method: "POST",
				URL:    dbSyncURL,
				Body: map[string]interface{}{
					"code": code,
					"msg":  "k_dl_4000001：forbid ddl/dcl operation in online env",
				},
			})

			err := runAppsShortcut(t, AppsDBSyncCreate, []string{
				"+db-sync-create", "--app-id", "app_x", "--config", dbSyncCommitConfig, "--yes", "--as", "user",
			}, factory, stdout)
			if err == nil {
				t.Fatal("expected online DDL precondition error")
			}
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("error = %T %v, want typed problem", err, err)
			}
			if p.Category != errs.CategoryValidation || p.Subtype != errs.SubtypeFailedPrecondition {
				t.Fatalf("category/subtype = %s/%s, want validation/failed_precondition", p.Category, p.Subtype)
			}
			if got := output.ExitCodeOf(err); got != output.ExitValidation {
				t.Fatalf("exit code = %d, want %d", got, output.ExitValidation)
			}
			if p.Hint != dbSyncOnlineDDLHint {
				t.Fatalf("hint = %q, want %q", p.Hint, dbSyncOnlineDDLHint)
			}
			if p.Code != code {
				t.Fatalf("code = %d, want server code %d preserved", p.Code, code)
			}
		})
	}
}
