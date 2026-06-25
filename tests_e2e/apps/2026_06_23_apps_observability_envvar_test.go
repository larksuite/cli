// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const (
	appsE2EAppID    = "app_x"
	appsSecretValue = "super-secret-value-for-e2e"
)

func TestAppsObservabilityDryRunContract(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		method     string
		url        string
		assertBody func(*testing.T, string)
	}{
		{
			name: "log_list_request_shape",
			args: []string{
				"apps", "+log-list",
				"--app-id", appsE2EAppID,
				"--env", "online",
				"--level", "error",
				"--since", "2026-06-23T10:00:00Z",
				"--until", "2026-06-23T11:00:00Z",
				"--log-id", "LOG1",
				"--log-id", "LOG2",
				"--trace-id", "trace-1",
				"--keyword", "timeout",
				"--min-duration", "200",
				"--page-size", "50",
				"--page-token", "next-token",
			},
			method: "POST",
			url:    "/open-apis/spark/v1/apps/app_x/search_logs",
			assertBody: func(t *testing.T, stdout string) {
				assert.Equal(t, "runtime", gjson.Get(stdout, "api.0.body.app_env").String(), "stdout:\n%s", stdout)
				assert.Equal(t, "1782208800000000000", gjson.Get(stdout, "api.0.body.start_timestamp_ns").String(), "stdout:\n%s", stdout)
				assert.Equal(t, "1782212400000000000", gjson.Get(stdout, "api.0.body.end_timestamp_ns").String(), "stdout:\n%s", stdout)
				assert.Equal(t, int64(50), gjson.Get(stdout, "api.0.body.limit").Int(), "stdout:\n%s", stdout)
				assert.Equal(t, "next-token", gjson.Get(stdout, "api.0.body.page_token").String(), "stdout:\n%s", stdout)
				requireStringArray(t, stdout, "api.0.body.filter.levels", []string{"ERROR"})
				requireStringArray(t, stdout, "api.0.body.filter.log_ids", []string{"LOG1", "LOG2"})
				requireStringArray(t, stdout, "api.0.body.filter.trace_ids", []string{"trace-1"})
				assert.Equal(t, "timeout", gjson.Get(stdout, "api.0.body.filter.keyword").String(), "stdout:\n%s", stdout)
				assert.Equal(t, int64(200), gjson.Get(stdout, "api.0.body.filter.min_duration_ms").Int(), "stdout:\n%s", stdout)
			},
		},
		{
			name: "log_get_uses_search_logs_with_limit_one",
			args: []string{
				"apps", "+log-get",
				"--app-id", appsE2EAppID,
				"--env", "online",
				"--log-id", "LOG763372528845174288",
			},
			method: "POST",
			url:    "/open-apis/spark/v1/apps/app_x/search_logs",
			assertBody: func(t *testing.T, stdout string) {
				assert.Equal(t, "runtime", gjson.Get(stdout, "api.0.body.app_env").String(), "stdout:\n%s", stdout)
				assert.Equal(t, int64(1), gjson.Get(stdout, "api.0.body.limit").Int(), "stdout:\n%s", stdout)
				requireStringArray(t, stdout, "api.0.body.filter.log_ids", []string{"LOG763372528845174288"})
			},
		},
		{
			name: "trace_list_request_shape",
			args: []string{
				"apps", "+trace-list",
				"--app-id", appsE2EAppID,
				"--env", "online",
				"--trace-id", "trace-1",
				"--root-span", "api-gateway",
				"--user-id", "ou_user",
				"--since", "2026-06-23T10:00:00Z",
				"--until", "2026-06-23T11:00:00Z",
				"--page-size", "25",
				"--page-token", "next-token",
			},
			method: "POST",
			url:    "/open-apis/spark/v1/apps/app_x/search_traces",
			assertBody: func(t *testing.T, stdout string) {
				assert.Equal(t, "runtime", gjson.Get(stdout, "api.0.body.app_env").String(), "stdout:\n%s", stdout)
				assert.Equal(t, "1782208800000000000", gjson.Get(stdout, "api.0.body.start_timestamp_ns").String(), "stdout:\n%s", stdout)
				assert.Equal(t, "1782212400000000000", gjson.Get(stdout, "api.0.body.end_timestamp_ns").String(), "stdout:\n%s", stdout)
				assert.Equal(t, int64(25), gjson.Get(stdout, "api.0.body.limit").Int(), "stdout:\n%s", stdout)
				assert.Equal(t, "next-token", gjson.Get(stdout, "api.0.body.page_token").String(), "stdout:\n%s", stdout)
				requireStringArray(t, stdout, "api.0.body.filter.trace_ids", []string{"trace-1"})
				assert.Equal(t, "api-gateway", gjson.Get(stdout, "api.0.body.filter.keyword").String(), "stdout:\n%s", stdout)
				requireStringArray(t, stdout, "api.0.body.filter.user_ids", []string{"ou_user"})
			},
		},
		{
			name: "trace_get_request_shape",
			args: []string{
				"apps", "+trace-get",
				"--app-id", appsE2EAppID,
				"--env", "online",
				"--trace-id", "359d7ab1d9e222b43ee56619a55f937a",
			},
			method: "POST",
			url:    "/open-apis/spark/v1/apps/app_x/trace",
			assertBody: func(t *testing.T, stdout string) {
				assert.Equal(t, "runtime", gjson.Get(stdout, "api.0.body.app_env").String(), "stdout:\n%s", stdout)
				assert.Equal(t, "359d7ab1d9e222b43ee56619a55f937a", gjson.Get(stdout, "api.0.body.trace_id").String(), "stdout:\n%s", stdout)
			},
		},
		{
			name: "metric_query_request_shape",
			args: []string{
				"apps", "+metric-query",
				"--app-id", appsE2EAppID,
				"--env", "online",
				"--metric", "requests",
				"--series", "total",
				"--since", "2026-06-23T10:00:00Z",
				"--until", "2026-06-23T11:00:00Z",
				"--page", "/home",
				"--api", "/api/orders",
				"--down-sample", "1m",
			},
			method: "POST",
			url:    "/open-apis/spark/v1/apps/app_x/query_metrics_data",
			assertBody: func(t *testing.T, stdout string) {
				assert.False(t, gjson.Get(stdout, "api.0.body.app_env").Exists(), "metric OpenAPI body should not include app_env, stdout:\n%s", stdout)
				assert.Equal(t, "1782208800", gjson.Get(stdout, "api.0.body.start_timestamp").String(), "stdout:\n%s", stdout)
				assert.Equal(t, "1782212400", gjson.Get(stdout, "api.0.body.end_timestamp").String(), "stdout:\n%s", stdout)
				requireStringArray(t, stdout, "api.0.body.metric_names", []string{"client_api_request_count"})
				assert.Equal(t, "/home", gjson.Get(stdout, "api.0.body.filter.pages.0").String(), "stdout:\n%s", stdout)
				assert.Equal(t, "/api/orders", gjson.Get(stdout, "api.0.body.filter.apis.0").String(), "stdout:\n%s", stdout)
				assert.Equal(t, "1m", gjson.Get(stdout, "api.0.body.down_sample").String(), "stdout:\n%s", stdout)
				assert.True(t, gjson.Get(stdout, "api.0.body.need_pack_lack_point").Exists(), "stdout:\n%s", stdout)
				assert.False(t, gjson.Get(stdout, "api.0.body.need_pack_lack_point").Bool(), "stdout:\n%s", stdout)
			},
		},
		{
			name: "analytics_query_request_shape",
			args: []string{
				"apps", "+analytics-query",
				"--app-id", appsE2EAppID,
				"--env", "online",
				"--analytics", "users",
				"--series", "active-users",
				"--since", "2026-06-23T10:00:00Z",
				"--until", "2026-06-23T11:00:00Z",
				"--page", "/home",
				"--device-type", "desktop",
				"--granularity", "week",
			},
			method: "POST",
			url:    "/open-apis/spark/v1/apps/app_x/query_analytics_data",
			assertBody: func(t *testing.T, stdout string) {
				assert.False(t, gjson.Get(stdout, "api.0.body.app_env").Exists(), "analytics OpenAPI body should not include app_env, stdout:\n%s", stdout)
				assert.Equal(t, "1782208800000000000", gjson.Get(stdout, "api.0.body.start_timestamp_ns").String(), "stdout:\n%s", stdout)
				assert.Equal(t, "1782212400000000000", gjson.Get(stdout, "api.0.body.end_timestamp_ns").String(), "stdout:\n%s", stdout)
				requireStringArray(t, stdout, "api.0.body.metric_types", []string{"ACTIVE_USER"})
				assert.Equal(t, "WEEK", gjson.Get(stdout, "api.0.body.time_aggregation_unit").String(), "stdout:\n%s", stdout)
				assert.Equal(t, "/home", gjson.Get(stdout, "api.0.body.filter.page").String(), "stdout:\n%s", stdout)
				assert.Equal(t, "desktop", gjson.Get(stdout, "api.0.body.filter.device_types.0").String(), "stdout:\n%s", stdout)
				assert.True(t, gjson.Get(stdout, "api.0.body.need_pack_lack_point").Exists(), "stdout:\n%s", stdout)
				assert.False(t, gjson.Get(stdout, "api.0.body.need_pack_lack_point").Bool(), "stdout:\n%s", stdout)
				assert.False(t, gjson.Get(stdout, "api.0.body.group_by").Exists(), "group_by is intentionally unsupported for now, stdout:\n%s", stdout)
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result := runAppsDryRunCommand(t, ctx, tc.args, false)
			result.AssertExitCode(t, 0)
			assert.Equal(t, tc.method, gjson.Get(result.Stdout, "api.0.method").String(), "stdout:\n%s", result.Stdout)
			assert.Equal(t, tc.url, gjson.Get(result.Stdout, "api.0.url").String(), "stdout:\n%s", result.Stdout)
			tc.assertBody(t, result.Stdout)
		})
	}
}

func TestAppsObservabilityRejectsNonOnlineEnv(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{
			name: "log_list",
			args: []string{"apps", "+log-list", "--app-id", appsE2EAppID, "--env", "dev"},
		},
		{
			name: "log_get",
			args: []string{"apps", "+log-get", "--app-id", appsE2EAppID, "--env", "dev", "--log-id", "LOG763372528845174288"},
		},
		{
			name: "trace_list",
			args: []string{"apps", "+trace-list", "--app-id", appsE2EAppID, "--env", "dev"},
		},
		{
			name: "trace_get",
			args: []string{"apps", "+trace-get", "--app-id", appsE2EAppID, "--env", "dev", "--trace-id", "359d7ab1d9e222b43ee56619a55f937a"},
		},
		{
			name: "metric_query",
			args: []string{"apps", "+metric-query", "--app-id", appsE2EAppID, "--env", "dev", "--metric", "requests"},
		},
		{
			name: "analytics_query",
			args: []string{"apps", "+analytics-query", "--app-id", appsE2EAppID, "--env", "dev", "--analytics", "users"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result := runAppsDryRunCommand(t, ctx, tc.args, false)
			result.AssertExitCode(t, 2)
			raw := errorEnvelope(t, result)
			assert.Equal(t, "validation", gjson.Get(raw, "error.type").String(), "error envelope:\n%s", raw)
			assert.Equal(t, "invalid_argument", gjson.Get(raw, "error.subtype").String(), "error envelope:\n%s", raw)
			assert.Equal(t, "--env", gjson.Get(raw, "error.param").String(), "error envelope:\n%s", raw)
			assert.Contains(t, gjson.Get(raw, "error.message").String(), "observability commands only support online", "error envelope:\n%s", raw)
		})
	}
}

func TestAppsEnvVarDryRunAndSafety(t *testing.T) {
	t.Run("env_pull_uses_dev_post_body_contract", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)
		projectDir := filepath.Join(t.TempDir(), "demo")

		result := runAppsDryRunCommand(t, ctx, []string{
			"apps", "+env-pull",
			"--app-id", appsE2EAppID,
			"--project-path", projectDir,
		}, false)
		result.AssertExitCode(t, 0)
		assert.Equal(t, "POST", gjson.Get(result.Stdout, "api.0.method").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, "/open-apis/spark/v1/apps/app_x/env_vars", gjson.Get(result.Stdout, "api.0.url").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, "dev", gjson.Get(result.Stdout, "api.0.body.env").String(), "stdout:\n%s", result.Stdout)
		assert.False(t, gjson.Get(result.Stdout, "api.0.params.include_values").Exists(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, filepath.Join(projectDir, ".env.local"), gjson.Get(result.Stdout, "env_file").String(), "stdout:\n%s", result.Stdout)
		assert.False(t, gjson.Get(result.Stdout, "env_keys").Exists(), "env-pull dry-run must not expose key list, stdout:\n%s", result.Stdout)
		assert.NotContains(t, result.Stdout, appsSecretValue, "env-pull dry-run must not leak env values, stdout:\n%s", result.Stdout)
	})

	t.Run("envvar_list_defaults_to_dev_without_values", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result := runAppsDryRunCommand(t, ctx, []string{
			"apps", "+envvar-list",
			"--app-id", appsE2EAppID,
		}, false)
		result.AssertExitCode(t, 0)
		assert.Equal(t, "POST", gjson.Get(result.Stdout, "api.0.method").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, "/open-apis/spark/v1/apps/app_x/env_vars", gjson.Get(result.Stdout, "api.0.url").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, "dev", gjson.Get(result.Stdout, "api.0.body.env").String(), "stdout:\n%s", result.Stdout)
		assert.False(t, gjson.Get(result.Stdout, "api.0.params.include_values").Exists(), "stdout:\n%s", result.Stdout)
		assert.False(t, gjson.Get(result.Stdout, "api.0.body.value").Exists(), "list dry-run must not send values, stdout:\n%s", result.Stdout)
	})

	t.Run("envvar_set_dev_post_redacts_value", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result := runAppsDryRunCommand(t, ctx, []string{
			"apps", "+envvar-set",
			"--app-id", appsE2EAppID,
			"--env", "dev",
			"--key", "API_HOST",
			"--value", appsSecretValue,
		}, false)
		result.AssertExitCode(t, 0)
		assert.Equal(t, "POST", gjson.Get(result.Stdout, "api.0.method").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, "/open-apis/spark/v1/apps/app_x/create_or_update_env_var", gjson.Get(result.Stdout, "api.0.url").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, "dev", gjson.Get(result.Stdout, "api.0.body.env").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, "API_HOST", gjson.Get(result.Stdout, "api.0.body.key").String(), "stdout:\n%s", result.Stdout)
		assert.NotContains(t, result.Stdout, appsSecretValue, "envvar-set dry-run must not leak raw value in stdout:\n%s", result.Stdout)
		assert.NotContains(t, result.Stderr, appsSecretValue, "envvar-set dry-run must not leak raw value in stderr:\n%s", result.Stderr)
	})

	t.Run("envvar_set_online_dry_run_does_not_require_yes", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result := runAppsDryRunCommand(t, ctx, []string{
			"apps", "+envvar-set",
			"--app-id", appsE2EAppID,
			"--env", "online",
			"--key", "API_HOST",
			"--value", appsSecretValue,
		}, false)
		result.AssertExitCode(t, 0)
		assert.Equal(t, "POST", gjson.Get(result.Stdout, "api.0.method").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, "/open-apis/spark/v1/apps/app_x/create_or_update_env_var", gjson.Get(result.Stdout, "api.0.url").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, "online", gjson.Get(result.Stdout, "api.0.body.env").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, "API_HOST", gjson.Get(result.Stdout, "api.0.body.key").String(), "stdout:\n%s", result.Stdout)
		assert.NotContains(t, result.Stdout, appsSecretValue, "online dry-run must not leak raw value in stdout:\n%s", result.Stdout)
	})

	t.Run("envvar_set_online_requires_yes_without_dry_run", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result := runAppsCommand(t, ctx, []string{
			"apps", "+envvar-set",
			"--app-id", appsE2EAppID,
			"--env", "online",
			"--key", "API_HOST",
			"--value", appsSecretValue,
		}, false)
		result.AssertExitCode(t, 10)
		raw := errorEnvelope(t, result)
		assert.Equal(t, "confirmation", gjson.Get(raw, "error.type").String(), "error envelope:\n%s", raw)
		assert.Equal(t, "confirmation_required", gjson.Get(raw, "error.subtype").String(), "error envelope:\n%s", raw)
		assert.Contains(t, gjson.Get(raw, "error.hint").String(), "add --yes to confirm", "error envelope:\n%s", raw)
		assert.NotContains(t, raw, appsSecretValue, "confirmation error must not leak raw value:\n%s", raw)
	})

	t.Run("envvar_delete_dry_run_body", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result := runAppsDryRunCommand(t, ctx, []string{
			"apps", "+envvar-delete",
			"--app-id", appsE2EAppID,
			"--env", "dev",
			"--key", "API_HOST",
			"--key", "API_TOKEN",
		}, true)
		result.AssertExitCode(t, 0)
		assert.Equal(t, "POST", gjson.Get(result.Stdout, "api.0.method").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, "/open-apis/spark/v1/apps/app_x/delete_env_vars", gjson.Get(result.Stdout, "api.0.url").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, "dev", gjson.Get(result.Stdout, "api.0.body.env").String(), "stdout:\n%s", result.Stdout)
		requireStringArray(t, result.Stdout, "api.0.body.keys", []string{"API_HOST", "API_TOKEN"})
		assert.False(t, gjson.Get(result.Stdout, "api.0.body.value").Exists(), "delete body must not contain values, stdout:\n%s", result.Stdout)
	})

	t.Run("envvar_delete_requires_yes_without_dry_run", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result := runAppsCommand(t, ctx, []string{
			"apps", "+envvar-delete",
			"--app-id", appsE2EAppID,
			"--env", "dev",
			"--key", "API_HOST",
		}, false)
		result.AssertExitCode(t, 10)
		raw := errorEnvelope(t, result)
		assert.Equal(t, "confirmation", gjson.Get(raw, "error.type").String(), "error envelope:\n%s", raw)
		assert.Equal(t, "confirmation_required", gjson.Get(raw, "error.subtype").String(), "error envelope:\n%s", raw)
	})
}

func TestAppsObservabilityLiveFixtureOutputs(t *testing.T) {
	appID := os.Getenv("LARK_CLI_E2E_APPS_OBSERVABILITY_APP_ID")
	logID := os.Getenv("LARK_CLI_E2E_APPS_LOG_ID")
	traceID := os.Getenv("LARK_CLI_E2E_APPS_TRACE_ID")
	if appID == "" || logID == "" || traceID == "" {
		t.Skip("FIXTURE: Set LARK_CLI_E2E_APPS_OBSERVABILITY_APP_ID, LARK_CLI_E2E_APPS_LOG_ID, and LARK_CLI_E2E_APPS_TRACE_ID to an online app with visible log, trace, metric, and analytics data")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	t.Run("log_get_returns_fixture_log", func(t *testing.T) {
		result := runAppsLiveCommand(t, ctx, []string{
			"apps", "+log-get",
			"--app-id", appID,
			"--env", "online",
			"--log-id", logID,
		}, false)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, logID, gjson.Get(result.Stdout, "data.log_id").String(), "stdout:\n%s", result.Stdout)
	})

	t.Run("trace_get_returns_fixture_trace", func(t *testing.T) {
		result := runAppsLiveCommand(t, ctx, []string{
			"apps", "+trace-get",
			"--app-id", appID,
			"--env", "online",
			"--trace-id", traceID,
		}, false)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, traceID, gjson.Get(result.Stdout, "data.trace_id").String(), "stdout:\n%s", result.Stdout)
		require.NotEmpty(t, gjson.Get(result.Stdout, "data.spans").Array(), "trace should include spans, stdout:\n%s", result.Stdout)
	})

	t.Run("metric_query_returns_request_series", func(t *testing.T) {
		result := runAppsLiveCommand(t, ctx, []string{
			"apps", "+metric-query",
			"--app-id", appID,
			"--env", "online",
			"--metric", "requests",
			"--series", "total",
		}, false)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		items := gjson.Get(result.Stdout, "data.items").Array()
		require.NotEmpty(t, items, "fixture app should have request metric points, stdout:\n%s", result.Stdout)
		assert.True(t, items[0].Get("values.total").Exists(), "request metric should expose total values, stdout:\n%s", result.Stdout)
	})

	t.Run("analytics_query_returns_active_users", func(t *testing.T) {
		result := runAppsLiveCommand(t, ctx, []string{
			"apps", "+analytics-query",
			"--app-id", appID,
			"--env", "online",
			"--analytics", "users",
			"--series", "active-users",
		}, false)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		items := gjson.Get(result.Stdout, "data.items").Array()
		require.NotEmpty(t, items, "fixture app should have analytics points, stdout:\n%s", result.Stdout)
		assert.True(t, items[0].Get("values.active-users").Exists(), "analytics should expose active-users values, stdout:\n%s", result.Stdout)
	})
}

func TestAppsEnvVarLiveWorkflow(t *testing.T) {
	appID := os.Getenv("LARK_CLI_E2E_APPS_ENVVAR_APP_ID")
	if appID == "" {
		t.Skip("FIXTURE: Set LARK_CLI_E2E_APPS_ENVVAR_APP_ID to an app where the user identity may create, list, and delete online env vars")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := strings.NewReplacer("-", "_").Replace(clie2e.GenerateSuffix())
	key := "LARK_CLI_E2E_" + suffix
	value := "secret-value-" + suffix
	created := false

	t.Cleanup(func() {
		if !created {
			return
		}
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()
		deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args: []string{
				"apps", "+envvar-delete",
				"--app-id", appID,
				"--env", "online",
				"--key", key,
			},
			DefaultAs: "user",
			Env:       appsNoNoticeEnv(),
			Yes:       true,
		})
		clie2e.ReportCleanupFailure(t, "delete apps envvar "+key, deleteResult, deleteErr)
	})

	t.Run("set_online_redacts_value", func(t *testing.T) {
		result := runAppsLiveCommand(t, ctx, []string{
			"apps", "+envvar-set",
			"--app-id", appID,
			"--env", "online",
			"--key", key,
			"--value", value,
		}, true)
		if result.ExitCode == 0 {
			created = true
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, key, gjson.Get(result.Stdout, "data.key").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, "online", gjson.Get(result.Stdout, "data.env").String(), "stdout:\n%s", result.Stdout)
		assert.Contains(t, []string{"set", "created", "updated"}, gjson.Get(result.Stdout, "data.action").String(), "stdout:\n%s", result.Stdout)
		assert.NotContains(t, result.Stdout, value, "set output must not leak raw value, stdout:\n%s", result.Stdout)
		assert.NotContains(t, result.Stderr, value, "set output must not leak raw value, stderr:\n%s", result.Stderr)
	})

	t.Run("list_include_values_observes_created_key", func(t *testing.T) {
		result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args: []string{
				"apps", "+envvar-list",
				"--app-id", appID,
				"--env", "online",
				"--include-values",
			},
			DefaultAs: "user",
			Env:       appsNoNoticeEnv(),
		}, clie2e.RetryOptions{
			ShouldRetry: func(result *clie2e.Result) bool {
				return result == nil || result.ExitCode != 0 || !envVarKeyExists(result.Stdout, key)
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		item, found := envVarItem(result.Stdout, key)
		require.True(t, found, "list should include created key %q, stdout:\n%s", key, result.Stdout)
		assert.Equal(t, "online", item.Get("env").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, value, item.Get("value").String(), "include-values should expose the explicitly requested test value, stdout:\n%s", result.Stdout)
	})

	t.Run("delete_removes_key", func(t *testing.T) {
		deleteResult := runAppsLiveCommand(t, ctx, []string{
			"apps", "+envvar-delete",
			"--app-id", appID,
			"--env", "online",
			"--key", key,
		}, true)
		if deleteResult.ExitCode == 0 {
			created = false
		}
		deleteResult.AssertExitCode(t, 0)
		deleteResult.AssertStdoutStatus(t, true)
		requireStringArray(t, deleteResult.Stdout, "data.deleted_keys", []string{key})
		assert.Equal(t, "online", gjson.Get(deleteResult.Stdout, "data.env").String(), "stdout:\n%s", deleteResult.Stdout)

		listResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args: []string{
				"apps", "+envvar-list",
				"--app-id", appID,
				"--env", "online",
				"--include-values",
			},
			DefaultAs: "user",
			Env:       appsNoNoticeEnv(),
		}, clie2e.RetryOptions{
			ShouldRetry: func(result *clie2e.Result) bool {
				return result == nil || result.ExitCode != 0 || envVarKeyExists(result.Stdout, key)
			},
		})
		require.NoError(t, err)
		listResult.AssertExitCode(t, 0)
		listResult.AssertStdoutStatus(t, true)
		assert.False(t, envVarKeyExists(listResult.Stdout, key), "deleted key should be absent, stdout:\n%s", listResult.Stdout)
	})
}

func runAppsDryRunCommand(t *testing.T, ctx context.Context, args []string, yes bool) *clie2e.Result {
	t.Helper()
	dryRunArgs := append([]string{}, args...)
	dryRunArgs = append(dryRunArgs, "--dry-run")
	return runAppsCommandWithEnv(t, ctx, dryRunArgs, yes, appsDryRunEnv())
}

func runAppsCommand(t *testing.T, ctx context.Context, args []string, yes bool) *clie2e.Result {
	t.Helper()
	return runAppsCommandWithEnv(t, ctx, args, yes, appsDryRunEnv())
}

func runAppsLiveCommand(t *testing.T, ctx context.Context, args []string, yes bool) *clie2e.Result {
	t.Helper()
	return runAppsCommandWithEnv(t, ctx, args, yes, appsNoNoticeEnv())
}

func runAppsCommandWithEnv(t *testing.T, ctx context.Context, args []string, yes bool, env map[string]string) *clie2e.Result {
	t.Helper()
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      args,
		DefaultAs: "user",
		Env:       env,
		Yes:       yes,
	})
	require.NoError(t, err)
	return result
}

func appsDryRunEnv() map[string]string {
	env := appsNoNoticeEnv()
	env["LARKSUITE_CLI_APP_ID"] = "cli-e2e-app-id"
	env["LARKSUITE_CLI_APP_SECRET"] = "cli-e2e-app-secret"
	env["LARKSUITE_CLI_BRAND"] = "feishu"
	return env
}

func appsNoNoticeEnv() map[string]string {
	return map[string]string{
		"LARKSUITE_CLI_NO_UPDATE_NOTIFIER": "1",
		"LARKSUITE_CLI_NO_SKILLS_NOTIFIER": "1",
	}
}

func requireStringArray(t *testing.T, stdout string, path string, want []string) {
	t.Helper()
	got := gjson.Get(stdout, path).Array()
	require.Len(t, got, len(want), "path %s should contain %d items, stdout:\n%s", path, len(want), stdout)
	for i, value := range want {
		assert.Equal(t, value, got[i].String(), "path %s[%d], stdout:\n%s", path, i, stdout)
	}
}

func errorEnvelope(t *testing.T, result *clie2e.Result) string {
	t.Helper()
	raw := strings.TrimSpace(result.Stdout)
	if raw == "" {
		raw = strings.TrimSpace(result.Stderr)
	}
	require.NotEmpty(t, raw, "expected structured error output, stdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
	return raw
}

func envVarKeyExists(stdout string, key string) bool {
	_, found := envVarItem(stdout, key)
	return found
}

func envVarItem(stdout string, key string) (gjson.Result, bool) {
	for _, item := range gjson.Get(stdout, "data.items").Array() {
		if item.Get("key").String() == key {
			return item, true
		}
	}
	return gjson.Result{}, false
}
