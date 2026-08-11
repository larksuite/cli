// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const (
	dbSyncDryRunAppID    = "app_db_sync_e2e"
	dbSyncDryRunTaskID   = "streaming_1"
	dbSyncConfig         = `{"mode":"streaming","source":{"type":"base","base_url":"https://example.feishu.cn/base/mock","table":{"name":"Orders"}},"target":{"type":"postgresql","table":{"name":"orders_sync","action":"use_existing"}},"field_maps":[{"source_field":"Base 表记录 ID","target_field":"base_record_id","enabled":true}]}`
	dbSyncPreviewConfig  = `{"mode":"batch","source":{"type":"base","base_url":"https://example.feishu.cn/base/mock","table":{"name":"Orders"}},"target":{"type":"postgresql","table":{"name":"orders_preview","action":"create"}}}`
	dbSyncSingularConfig = `{"mode":"streaming","source":{"type":"base","base_url":"https://example.feishu.cn/base/mock","table":{"name":"Orders"}},"target":{"type":"postgresql","table":{"name":"orders_sync","action":"use_existing"}},"field_map":[{"source_field":"Base 表记录 ID","target_field":"base_record_id","enabled":true}]}`

	dbSyncUpdateNoBaseURLConfig = `{"mode":"streaming","source":{"type":"base","table":{"name":"数据表"}},"target":{"type":"postgresql","table":{"name":"orders_sync","action":"use_existing"}},"field_maps":[{"source_field":"Base 表记录 ID","target_field":"base_record_id","enabled":true}]}`

	// base_url without ?table= and empty source.table.name → create must reject locally.
	dbSyncNoTableNoNameConfig = `{"mode":"batch","source":{"type":"base","base_url":"https://example.feishu.cn/base/mock","table":{"name":""}},"target":{"type":"postgresql","table":{"name":"orders_preview","action":"create"}}}`

	// create-commit without field_maps: identifiable source (has table name), no field_maps →
	// CLI passes it through so the server auto-matches and creates the task.
	dbSyncNoFieldMapsCommitConfig = `{"mode":"batch","source":{"type":"base","base_url":"https://example.feishu.cn/base/mock","table":{"name":"Orders"}},"target":{"type":"postgresql","table":{"name":"orders_auto","action":"create"}}}`

	// field_maps present but not an array — a structural error rejected locally in
	// both preview and commit before the malformed shape reaches the backend.
	dbSyncInvalidFieldMapsConfig = `{"mode":"batch","source":{"type":"base","base_url":"https://example.feishu.cn/base/mock","table":{"name":"Orders"}},"target":{"type":"postgresql","table":{"name":"orders_preview","action":"create"}},"field_maps":{}}`
)

func TestAppsDBSyncDryRunRequestContracts(t *testing.T) {
	setAppsDryRunEnv(t)

	t.Run("create preview", func(t *testing.T) {
		result := runDBSyncDryRun(t, []string{
			"apps", "+db-sync-create",
			"--app-id", dbSyncDryRunAppID,
			"--config", dbSyncPreviewConfig,
			"--preview",
			"--dry-run",
		})

		assert.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/spark/v1/apps/app_db_sync_e2e/db/sync_create", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.params.env").Exists())
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.params.preview").Exists())
		assert.True(t, clie2e.DryRunGet(result.Stdout, "api.0.body.preview").Bool())
		assert.Equal(t, "batch", clie2e.DryRunGet(result.Stdout, "api.0.body.config.mode").String())
		assert.Equal(t, "base", clie2e.DryRunGet(result.Stdout, "api.0.body.config.source.type").String())
		assert.Equal(t, "postgresql", clie2e.DryRunGet(result.Stdout, "api.0.body.config.target.type").String())
	})

	t.Run("create commit", func(t *testing.T) {
		result := runDBSyncDryRun(t, []string{
			"apps", "+db-sync-create",
			"--app-id", dbSyncDryRunAppID,
			"--config", dbSyncConfig,
			"--environment", "dev",
			"--dry-run",
		})

		assert.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/spark/v1/apps/app_db_sync_e2e/db/sync_create", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.params.preview").Exists())
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.params.env").Exists())
		assert.Equal(t, "dev", clie2e.DryRunGet(result.Stdout, "api.0.body.env").String())
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.body.preview").Bool())
		assert.Equal(t, "streaming", clie2e.DryRunGet(result.Stdout, "api.0.body.config.mode").String())
		assert.Equal(t, "orders_sync", clie2e.DryRunGet(result.Stdout, "api.0.body.config.target.table.name").String())
		assert.Equal(t, "Base 表记录 ID", clie2e.DryRunGet(result.Stdout, "api.0.body.config.field_maps.0.source_field").String())
	})

	t.Run("create commit without field maps uses server auto-match", func(t *testing.T) {
		result := runDBSyncDryRun(t, []string{
			"apps", "+db-sync-create",
			"--app-id", dbSyncDryRunAppID,
			"--config", dbSyncNoFieldMapsCommitConfig,
			"--environment", "dev",
			"--dry-run",
		})

		assert.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.body.preview").Bool())
		assert.Equal(t, "dev", clie2e.DryRunGet(result.Stdout, "api.0.body.env").String())
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.body.config.field_maps").Exists(),
			"CLI must not inject field_maps when the caller delegates matching to the server")
	})

	t.Run("list with filters", func(t *testing.T) {
		result := runDBSyncDryRun(t, []string{
			"apps", "+db-sync-list",
			"--app-id", dbSyncDryRunAppID,
			"--mode", "streaming",
			"--status", "active",
			"--table", "orders_sync",
			"--page-size", "50",
			"--page-token", "cursor_1",
			"--environment", "online",
			"--dry-run",
		})

		assert.Equal(t, "GET", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/spark/v1/apps/app_db_sync_e2e/db/sync_list", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
		assert.Equal(t, "streaming", clie2e.DryRunGet(result.Stdout, "api.0.params.mode").String())
		assert.Equal(t, "active", clie2e.DryRunGet(result.Stdout, "api.0.params.status").String())
		assert.Equal(t, "orders_sync", clie2e.DryRunGet(result.Stdout, "api.0.params.table").String())
		assert.Equal(t, "50", clie2e.DryRunGet(result.Stdout, "api.0.params.page_size").String())
		assert.Equal(t, "cursor_1", clie2e.DryRunGet(result.Stdout, "api.0.params.page_token").String())
		assert.Equal(t, "online", clie2e.DryRunGet(result.Stdout, "api.0.params.env").String())
	})

	t.Run("get task", func(t *testing.T) {
		result := runDBSyncDryRun(t, []string{
			"apps", "+db-sync-get",
			"--app-id", dbSyncDryRunAppID,
			"--task-id", dbSyncDryRunTaskID,
			"--dry-run",
		})

		assert.Equal(t, "GET", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/spark/v1/apps/app_db_sync_e2e/db/sync_task", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
		assert.Equal(t, dbSyncDryRunTaskID, clie2e.DryRunGet(result.Stdout, "api.0.params.task_id").String())
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.params.env").Exists())
	})

	t.Run("enable task", func(t *testing.T) {
		result := runDBSyncDryRun(t, []string{"apps", "+db-sync-enable", "--app-id", dbSyncDryRunAppID, "--task-id", dbSyncDryRunTaskID, "--dry-run"})
		assert.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/spark/v1/apps/app_db_sync_e2e/db/sync_enable", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.params.task_id").Exists())
		assert.Equal(t, dbSyncDryRunTaskID, clie2e.DryRunGet(result.Stdout, "api.0.body.task_id").String())
	})

	t.Run("disable task", func(t *testing.T) {
		result := runDBSyncDryRun(t, []string{"apps", "+db-sync-disable", "--app-id", dbSyncDryRunAppID, "--task-id", dbSyncDryRunTaskID, "--dry-run"})
		assert.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/spark/v1/apps/app_db_sync_e2e/db/sync_disable", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.params.task_id").Exists())
		assert.Equal(t, dbSyncDryRunTaskID, clie2e.DryRunGet(result.Stdout, "api.0.body.task_id").String())
	})

	t.Run("update task", func(t *testing.T) {
		result := runDBSyncDryRun(t, []string{
			"apps", "+db-sync-update",
			"--app-id", dbSyncDryRunAppID,
			"--task-id", dbSyncDryRunTaskID,
			"--config", dbSyncConfig,
			"--environment", "dev",
			"--dry-run",
		})
		assert.Equal(t, "PUT", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/spark/v1/apps/app_db_sync_e2e/db/sync_update", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
		assert.Equal(t, dbSyncDryRunTaskID, clie2e.DryRunGet(result.Stdout, "api.0.body.task_id").String())
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.params.env").Exists())
		assert.Equal(t, "dev", clie2e.DryRunGet(result.Stdout, "api.0.body.env").String())
		assert.Equal(t, "streaming", clie2e.DryRunGet(result.Stdout, "api.0.body.config.mode").String())
		assert.Equal(t, "base_record_id", clie2e.DryRunGet(result.Stdout, "api.0.body.config.field_maps.0.target_field").String())
	})

	t.Run("delete task", func(t *testing.T) {
		result := runDBSyncDryRun(t, []string{"apps", "+db-sync-delete", "--app-id", dbSyncDryRunAppID, "--task-id", dbSyncDryRunTaskID, "--dry-run"})
		assert.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/spark/v1/apps/app_db_sync_e2e/db/sync_del", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.params.task_id").Exists())
		assert.Equal(t, dbSyncDryRunTaskID, clie2e.DryRunGet(result.Stdout, "api.0.body.task_id").String())
	})

	t.Run("update task without base_url passes through source verbatim", func(t *testing.T) {
		result := runDBSyncDryRun(t, []string{
			"apps", "+db-sync-update",
			"--app-id", dbSyncDryRunAppID,
			"--task-id", dbSyncDryRunTaskID,
			"--config", dbSyncUpdateNoBaseURLConfig,
			"--dry-run",
		})
		assert.Equal(t, "PUT", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/spark/v1/apps/app_db_sync_e2e/db/sync_update", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
		assert.Equal(t, dbSyncDryRunTaskID, clie2e.DryRunGet(result.Stdout, "api.0.body.task_id").String())
		// source 被透传
		assert.Equal(t, "base", clie2e.DryRunGet(result.Stdout, "api.0.body.config.source.type").String())
		assert.Equal(t, "数据表", clie2e.DryRunGet(result.Stdout, "api.0.body.config.source.table.name").String())
		// 关键契约：CLI 未静默注入 base_url 默认值
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.body.config.source.base_url").Exists(), "CLI must not inject a base_url when omitted on update")
	})
}

func TestAppsDBSyncValidationErrors(t *testing.T) {
	setAppsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"apps", "+db-sync-create", "--app-id", dbSyncDryRunAppID, "--config", dbSyncSingularConfig, "--dry-run"},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	assert.Empty(t, strings.TrimSpace(result.Stdout), "validation failures must not emit success data on stdout")
	assert.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
	assert.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
	assert.Equal(t, "--config", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
	message := dbSyncValidateErrorMessage(result)
	assert.Contains(t, message, "field_maps")
	assert.Contains(t, message, "--config")
}

func TestAppsDBSyncPreviewRejectsNonArrayFieldMaps(t *testing.T) {
	setAppsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"apps", "+db-sync-create", "--app-id", dbSyncDryRunAppID, "--config", dbSyncInvalidFieldMapsConfig, "--preview", "--dry-run"},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	assert.Empty(t, strings.TrimSpace(result.Stdout), "preview must reject a malformed field_maps locally, not forward it")
	assert.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
	assert.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
	assert.Equal(t, "--config", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
	assert.Contains(t, dbSyncValidateErrorMessage(result), "field_maps must be an array")
}

func TestAppsDBSyncCreateRequiresSourceTable(t *testing.T) {
	setAppsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"apps", "+db-sync-create", "--app-id", dbSyncDryRunAppID, "--config", dbSyncNoTableNoNameConfig, "--preview", "--dry-run"},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	assert.Empty(t, strings.TrimSpace(result.Stdout), "validation failures must not emit success data on stdout")
	assert.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
	assert.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
	message := dbSyncValidateErrorMessage(result)
	assert.Contains(t, message, "source.table.name")
	assert.Equal(t, "--config", dbSyncValidateErrorParam(result))
	assert.Contains(t, dbSyncValidateErrorHint(result), "base +table-list")
}

func runDBSyncDryRun(t *testing.T, args []string) *clie2e.Result {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      args,
		DefaultAs: "user",
	})
	require.NoError(t, err)
	require.Equal(t, 0, result.ExitCode, "stdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
	result.AssertStdoutStatus(t, true)
	return result
}

func dbSyncValidateErrorMessage(r *clie2e.Result) string {
	if msg := gjson.Get(r.Stdout, "error.message").String(); msg != "" {
		return msg
	}
	if msg := gjson.Get(r.Stderr, "error.message").String(); msg != "" {
		return msg
	}
	return strings.TrimSpace(r.Stderr)
}

func dbSyncValidateErrorParam(r *clie2e.Result) string {
	if v := gjson.Get(r.Stdout, "error.param").String(); v != "" {
		return v
	}
	return gjson.Get(r.Stderr, "error.param").String()
}

func dbSyncValidateErrorHint(r *clie2e.Result) string {
	if v := gjson.Get(r.Stdout, "error.hint").String(); v != "" {
		return v
	}
	return gjson.Get(r.Stderr, "error.hint").String()
}
