// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

const (
	dbEnvMigrateURL       = "/open-apis/spark/v1/apps/app_x/db/env_migrate"
	dbEnvMigrateStatusURL = "/open-apis/spark/v1/apps/app_x/db/env_migrate_status"
	dbRecoveryURL         = "/open-apis/spark/v1/apps/app_x/db/env_recovery"
	dbRecoveryDiffURL     = "/open-apis/spark/v1/apps/app_x/db/env_recovery_diff_status"
	dbRecoveryApplyURL    = "/open-apis/spark/v1/apps/app_x/db/env_recovery_apply_status"
	dbQuotaURL            = "/open-apis/spark/v1/apps/app_x/db/quota"
)

// ── env-diff ──

// TestAppsDBEnvDiff_DryRunBody 校验 dry-run 请求体：POST env_migrate 且 dry_run=true。
func TestAppsDBEnvDiff_DryRunBody(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBEnvDiff,
		[]string{"+db-env-diff", "--app-id", "app_x", "--dry-run", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env struct {
		API []struct {
			Method string                 `json:"method"`
			URL    string                 `json:"url"`
			Body   map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	_ = json.Unmarshal([]byte(stdout.String()), &env)
	a := env.API[0]
	if a.Method != "POST" || a.URL != dbEnvMigrateURL || a.Body["dry_run"] != true {
		t.Fatalf("dry-run = %s %s body=%v", a.Method, a.URL, a.Body)
	}
}

// TestAppsDBEnvDiff_SuccessRendersChanges 验证 pretty 输出渲染出 dev → online 变更摘要及 DDL 语句。
func TestAppsDBEnvDiff_SuccessRendersChanges(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: dbEnvMigrateURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"from": "dev", "to": "online",
			"changes": []interface{}{
				map[string]interface{}{"type": "ALTER_TABLE", "table": "orders", "statement": "ALTER TABLE orders ADD COLUMN note text"},
			},
		}},
	})
	if err := runAppsShortcut(t, AppsDBEnvDiff,
		[]string{"+db-env-diff", "--app-id", "app_x", "--format", "pretty", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "dev → online (1 changes)") || !strings.Contains(got, "ALTER TABLE orders ADD COLUMN note text") {
		t.Fatalf("pretty diff malformed:\n%s", got)
	}
}

// TestAppsDBEnvDiff_EmptyChanges 验证无变更时 pretty 输出"无待发布变更"提示。
func TestAppsDBEnvDiff_EmptyChanges(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: dbEnvMigrateURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"from": "dev", "to": "online", "changes": []interface{}{}}},
	})
	if err := runAppsShortcut(t, AppsDBEnvDiff,
		[]string{"+db-env-diff", "--app-id", "app_x", "--format", "pretty", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if !strings.Contains(stdout.String(), "No pending changes from dev to online.") {
		t.Fatalf("expected empty message, got: %s", stdout.String())
	}
}

// ── env-migrate ──

// TestAppsDBEnvMigrate_DryRunBody 校验 migrate 的 dry-run 请求体里 dry_run=false（真实迁移）。
func TestAppsDBEnvMigrate_DryRunBody(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBEnvMigrate,
		[]string{"+db-env-migrate", "--app-id", "app_x", "--dry-run", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env struct {
		API []struct {
			Body map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	_ = json.Unmarshal([]byte(stdout.String()), &env)
	if env.API[0].Body["dry_run"] != false {
		t.Fatalf("dry-run body=%v (want dry_run:false)", env.API[0].Body)
	}
}

// 异步：submit 返 task_id，status 立刻 applied → CLI 对外统一 migrated。
func TestAppsDBEnvMigrate_AsyncPollSuccess(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	// Reusable：Execute 现在会先打一次 dry_run 预览拿待发布数、再打 apply（对齐 miaoda-cli 的
	// diff-then-apply，兜底服务端 apply 少报 changes_applied 的情况），故同一 POST 端点被调用两次。
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: dbEnvMigrateURL, Reusable: true,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"from": "dev", "to": "online", "task_id": "t1"}},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: dbEnvMigrateStatusURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"task_id": "t1", "status": "applied", "changes_applied": 3}},
	})
	if err := runAppsShortcut(t, AppsDBEnvMigrate,
		[]string{"+db-env-migrate", "--app-id", "app_x", "--yes", "--format", "pretty", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "✓ Migrated dev → online (3 changes)") {
		t.Fatalf("pretty: %s", got)
	}
}

// TestAppsDBEnvMigrate_PollFailedSurfacesError 验证轮询到 failed 时返回 API/server_error 类型错误，携带服务端 message 与恢复 hint。
func TestAppsDBEnvMigrate_PollFailedSurfacesError(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	// Reusable：Execute 现在会先打一次 dry_run 预览拿待发布数、再打 apply（对齐 miaoda-cli 的
	// diff-then-apply，兜底服务端 apply 少报 changes_applied 的情况），故同一 POST 端点被调用两次。
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: dbEnvMigrateURL, Reusable: true,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"from": "dev", "to": "online", "task_id": "t1"}},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: dbEnvMigrateStatusURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"task_id": "t1", "status": "failed", "error_message": "lock timeout"}},
	})
	err := runAppsShortcut(t, AppsDBEnvMigrate,
		[]string{"+db-env-migrate", "--app-id", "app_x", "--yes", "--as", "user"}, factory, stdout)
	p, ok := errs.ProblemOf(err)
	if !ok || p.Category != errs.CategoryAPI || p.Subtype != errs.SubtypeServerError {
		t.Fatalf("got %T %v, want API/server_error typed error", err, err)
	}
	if !strings.Contains(p.Message, "lock timeout") {
		t.Fatalf("Message = %q, want it to contain 'lock timeout'", p.Message)
	}
	if !strings.Contains(p.Hint, "+db-env-diff") {
		t.Fatalf("Hint = %q, want the db-env-migrate recovery hint", p.Hint)
	}
}

// TestAppsDBEnvMigrate_RequiresConfirmation 验证 high-risk-write 无 --yes 时被确认门拦截。
func TestAppsDBEnvMigrate_RequiresConfirmation(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	// high-risk-write 无 --yes → 应被确认门拦截（非 0 退出）。
	if err := runAppsShortcut(t, AppsDBEnvMigrate,
		[]string{"+db-env-migrate", "--app-id", "app_x", "--as", "user"}, factory, stdout); err == nil {
		t.Fatalf("expected confirmation gate without --yes")
	}
}

// ── recovery-diff ──

// TestAppsDBRecoveryDiff_RequiresTarget 验证缺少 --target 时报必填错误。
func TestAppsDBRecoveryDiff_RequiresTarget(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBRecoveryDiff,
		[]string{"+db-recovery-diff", "--app-id", "app_x", "--as", "user"}, factory, stdout); err == nil {
		t.Fatalf("expected required --target error")
	}
}

// TestAppsDBRecoveryDiff_DryRunNormalizesTarget 验证 dry-run 走 POST env_recovery 且 --target 被归一化为 RFC3339 UTC。
func TestAppsDBRecoveryDiff_DryRunNormalizesTarget(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBRecoveryDiff,
		[]string{"+db-recovery-diff", "--app-id", "app_x", "--target", "2026-04-15", "--dry-run", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env struct {
		API []struct {
			Method string                 `json:"method"`
			URL    string                 `json:"url"`
			Body   map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	_ = json.Unmarshal([]byte(stdout.String()), &env)
	a := env.API[0]
	if a.Method != "POST" || a.URL != dbRecoveryURL || a.Body["dry_run"] != true {
		t.Fatalf("dry-run = %s %s body=%v", a.Method, a.URL, a.Body)
	}
	if s, _ := a.Body["target"].(string); !strings.HasSuffix(s, "Z") {
		t.Fatalf("target not normalized to RFC3339 UTC: %v", a.Body["target"])
	}
}

// TestAppsDBRecoveryDiff_SuccessRendersChanges 验证 preview 成功后 pretty 渲染受影响表数、行增删与预估耗时。
func TestAppsDBRecoveryDiff_SuccessRendersChanges(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: dbRecoveryURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"preview_request_id": "p1"}},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: dbRecoveryDiffURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"preview_status": "success", "tables_affected": 2, "estimated_seconds": 12,
			"changes": []interface{}{
				map[string]interface{}{"table": "orders", "inserted": 5, "deleted": 2},
				map[string]interface{}{"table": "carts", "action": "restore_table"},
			},
		}},
	})
	if err := runAppsShortcut(t, AppsDBRecoveryDiff,
		[]string{"+db-recovery-diff", "--app-id", "app_x", "--target", "2h", "--format", "pretty", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	for _, want := range []string{"tables affected: 2", "orders: +5 rows, -2 rows", "carts: table will be restored", "estimated time: ~12s"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
}

// TestAppsDBRecoveryDiff_PreviewFailed 验证 preview_status=failed 时返回 API/server_error，携带 message 与 PITR window hint。
func TestAppsDBRecoveryDiff_PreviewFailed(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: dbRecoveryURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"preview_request_id": "p1"}},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: dbRecoveryDiffURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"preview_status": "failed", "error_message": "snapshot expired"}},
	})
	err := runAppsShortcut(t, AppsDBRecoveryDiff,
		[]string{"+db-recovery-diff", "--app-id", "app_x", "--target", "2h", "--as", "user"}, factory, stdout)
	p, ok := errs.ProblemOf(err)
	if !ok || p.Category != errs.CategoryAPI || p.Subtype != errs.SubtypeServerError {
		t.Fatalf("got %T %v, want API/server_error typed error", err, err)
	}
	if !strings.Contains(p.Message, "snapshot expired") {
		t.Fatalf("Message = %q, want it to contain 'snapshot expired'", p.Message)
	}
	if !strings.Contains(p.Hint, "PITR window") {
		t.Fatalf("Hint = %q, want the db-recovery recovery hint", p.Hint)
	}
}

// ── recovery-apply ──

// TestAppsDBRecoveryApply_NoChangesShortCircuits 验证 status=no_changes 时短路输出"已是该状态"，不再轮询。
func TestAppsDBRecoveryApply_NoChangesShortCircuits(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: dbRecoveryURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"status": "no_changes"}},
	})
	if err := runAppsShortcut(t, AppsDBRecoveryApply,
		[]string{"+db-recovery-apply", "--app-id", "app_x", "--target", "2h", "--yes", "--format", "pretty", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if !strings.Contains(stdout.String(), "No changes — database is already at this state.") {
		t.Fatalf("expected no-changes short-circuit, got: %s", stdout.String())
	}
}

// TestAppsDBRecoveryApply_AsyncPollSuccess 验证 running → 轮询 success 后 pretty 输出恢复完成及耗时。
func TestAppsDBRecoveryApply_AsyncPollSuccess(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: dbRecoveryURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"status": "running"}},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: dbRecoveryApplyURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"status": "success", "restore_time_sec": 8}},
	})
	if err := runAppsShortcut(t, AppsDBRecoveryApply,
		[]string{"+db-recovery-apply", "--app-id", "app_x", "--target", "2h", "--yes", "--format", "pretty", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if !strings.Contains(stdout.String(), "✓ Database restored to") || !strings.Contains(stdout.String(), "(8s elapsed)") {
		t.Fatalf("pretty: %s", stdout.String())
	}
}

// TestAppsDBRecoveryApply_RequiresConfirmation 验证无 --yes 时被确认门拦截。
func TestAppsDBRecoveryApply_RequiresConfirmation(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBRecoveryApply,
		[]string{"+db-recovery-apply", "--app-id", "app_x", "--target", "2h", "--as", "user"}, factory, stdout); err == nil {
		t.Fatalf("expected confirmation gate without --yes")
	}
}

// ── quota-get ──

// TestAppsDBQuotaGet_WithQuotaPretty 验证已对接配额时 pretty 渲染存储用量、百分比及 tables/views 数。
func TestAppsDBQuotaGet_WithQuotaPretty(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: dbQuotaURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"storage_used_bytes": 1048576, "storage_quota_bytes": 10485760, "usage_percent": 10.0,
			"tables": 4, "views": 1,
		}},
	})
	if err := runAppsShortcut(t, AppsDBQuotaGet,
		[]string{"+db-quota-get", "--app-id", "app_x", "--format", "pretty", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	for _, want := range []string{"Storage", "(10.0%)", "Tables", "4", "Views", "1"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
}

// 配额未对接（storage_quota_bytes=0）→ json 删 quota/usage_percent，仅留已用量与 tables/views。
// TestAppsDBQuotaGet_DryRunOmitsEnvWhenUnset 验证不传 --environment 时 quota-get 的 dry-run
// query 不带 env 键（交服务端按应用形态自动选分支）。
func TestAppsDBQuotaGet_DryRunOmitsEnvWhenUnset(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBQuotaGet,
		[]string{"+db-quota-get", "--app-id", "app_x", "--dry-run", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env struct {
		API []struct {
			Method string                 `json:"method"`
			URL    string                 `json:"url"`
			Params map[string]interface{} `json:"params"`
		} `json:"api"`
	}
	_ = json.Unmarshal([]byte(stdout.String()), &env)
	a := env.API[0]
	if a.Method != "GET" || a.URL != dbQuotaURL {
		t.Fatalf("dry-run = %s %s", a.Method, a.URL)
	}
	if _, ok := a.Params["env"]; ok {
		t.Fatalf("no --environment → env key must be omitted, got params=%v", a.Params)
	}
}

func TestAppsDBQuotaGet_NoQuotaOmitsFields(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: dbQuotaURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"storage_used_bytes": 2048, "storage_quota_bytes": 0, "tables": 2, "views": 0,
		}},
	})
	if err := runAppsShortcut(t, AppsDBQuotaGet,
		[]string{"+db-quota-get", "--app-id", "app_x", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	if strings.Contains(got, "storage_quota_bytes") || strings.Contains(got, "usage_percent") {
		t.Fatalf("quota fields should be omitted when not provisioned:\n%s", got)
	}
	if !strings.Contains(got, "storage_used_bytes") || !strings.Contains(got, "\"tables\"") {
		t.Fatalf("expected used + tables retained:\n%s", got)
	}
}

// TestProjectDbQuota_WhitelistsFields 验证 projectDbQuota 白名单投影：只保留 used/tables/views（及配额已对接时的
// quota/usage_percent），后端额外字段不透传。
func TestProjectDbQuota_WhitelistsFields(t *testing.T) {
	out := projectDbQuota(map[string]interface{}{
		"storage_used_bytes": 2048, "storage_quota_bytes": float64(0), "usage_percent": float64(0),
		"tables": 2, "views": 1, "tenant_key": "leak", "internal_shard": "s1",
	})
	if _, ok := out["storage_quota_bytes"]; ok {
		t.Errorf("zero quota should be omitted: %v", out)
	}
	if out["storage_used_bytes"] != 2048 || out["tables"] != 2 || out["views"] != 1 {
		t.Errorf("whitelisted fields should be kept: %v", out)
	}
	for _, leaked := range []string{"tenant_key", "internal_shard"} {
		if _, ok := out[leaked]; ok {
			t.Errorf("non-whitelisted field %q must be dropped: %v", leaked, out)
		}
	}

	out2 := projectDbQuota(map[string]interface{}{"storage_used_bytes": 2048, "storage_quota_bytes": float64(4096), "usage_percent": float64(50), "tables": 2})
	if _, ok := out2["storage_quota_bytes"]; !ok {
		t.Errorf("non-zero quota should be kept: %v", out2)
	}
	if _, ok := out2["usage_percent"]; !ok {
		t.Errorf("usage_percent should be kept when quota>0: %v", out2)
	}
}
