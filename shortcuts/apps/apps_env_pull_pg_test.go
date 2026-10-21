// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
)

// isolateManagedDBEnv removes the managed database keys from this test process
// environment so the shell-conflict check only sees what a test sets.
func isolateManagedDBEnv(t *testing.T) {
	t.Helper()
	for _, key := range envPullManagedKeys {
		if old, set := os.LookupEnv(key); set {
			t.Cleanup(func() { _ = os.Setenv(key, old) })
		} else {
			t.Cleanup(func() { _ = os.Unsetenv(key) })
		}
		_ = os.Unsetenv(key)
	}
}

func newEnvPullPGFactory(t *testing.T) (*cmdutil.Factory, *bytes.Buffer, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	isolateManagedDBEnv(t)
	cfg := &core.CliConfig{AppID: "test-app-envpull", AppSecret: "test-secret", Brand: core.BrandFeishu, UserOpenId: "ou_test"}
	factory, stdout, stderr, reg := cmdutil.TestFactory(t, cfg)
	return factory, stdout, stderr, reg
}

func envVarsResponse(entries ...map[string]any) map[string]any {
	vars := make([]any, 0, len(entries))
	for _, e := range entries {
		vars = append(vars, e)
	}
	return map[string]any{"code": 0, "data": map[string]any{"envVars": vars}}
}

func kv(key string, value any) map[string]any {
	return map[string]any{"key": key, "value": value}
}

const staleEnvLocal = `# user notes stay
MY_VAR="keep-me"
FORCE_FRAMEWORK_DISABLE_DATAPASS="false"
SUDA_DATABASE_URL="postgres://old-temp"
FORCE_DB_CONNECT_URL="postgres://old-direct"
FORCE_DB_BRANCH="dev"
MIAODA_CACHE_URL="redis://old"
`

func writeEnvLocal(t *testing.T, projectDir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(projectDir, ".env.local"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func runEnvPull(t *testing.T, factory *cmdutil.Factory, stdout *bytes.Buffer, projectDir string, extra ...string) error {
	t.Helper()
	args := append([]string{"+env-pull", "--app-id", "app_x", "--project-path", projectDir, "--as", "user"}, extra...)
	return runAppsShortcut(t, AppsEnvPull, args, factory, stdout)
}

func readEnvLocal(t *testing.T, projectDir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(projectDir, ".env.local"))
	if err != nil {
		t.Fatalf("read .env.local: %v", err)
	}
	return string(data)
}

func requireEnvLines(t *testing.T, got string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Fatalf(".env.local missing %s:\n%s", w, got)
		}
	}
}

// requireNoEnvPullFeatureState fails when env-pull output claims the app's
// database feature state: the env response cannot prove it, because an older
// app's custom variables can carry the same keys.
func requireNoEnvPullFeatureState(t *testing.T, data map[string]any) {
	t.Helper()
	for _, key := range []string{"pg_enabled", "config_revision", "feature_protocol"} {
		if _, present := data[key]; present {
			t.Fatalf("output must not carry %s: %v", key, data)
		}
	}
}

// The server explicitly disables the local database integration: the flag is
// "true" and both connection keys are sent empty. Stale platform connections
// in .env.local are cleared; user variables and Redis are kept.
func TestAppsEnvPullDB_ExplicitDisableClearsPlatformKeysAndKeepsUserAndRedis(t *testing.T) {
	factory, stdout, _, reg := newEnvPullPGFactory(t)
	projectDir := t.TempDir()
	writeEnvLocal(t, projectDir, staleEnvLocal)
	reg.Register(&httpmock.Stub{Method: "POST", URL: "/open-apis/spark/v1/apps/app_x/env_vars",
		Body: envVarsResponse(
			kv("FORCE_FRAMEWORK_DISABLE_DATAPASS", "true"),
			kv("SUDA_DATABASE_URL", ""),
			kv("FORCE_DB_CONNECT_URL", ""),
			kv("MIAODA_CACHE_URL", "redis://new"),
		)})

	if err := runEnvPull(t, factory, stdout, projectDir); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := readEnvLocal(t, projectDir)
	requireEnvLines(t, got,
		`FORCE_FRAMEWORK_DISABLE_DATAPASS="true"`,
		`SUDA_DATABASE_URL=""`,
		`FORCE_DB_CONNECT_URL=""`,
		`FORCE_DB_BRANCH=""`,
		`MY_VAR="keep-me"`,
		`MIAODA_CACHE_URL="redis://new"`,
		"# user notes stay",
	)
	if strings.Contains(got, "postgres://") {
		t.Fatalf("stale connection survived:\n%s", got)
	}
	data := decodeSuccessData(t, stdout)
	if data["restart_required"] != true {
		t.Fatalf("data = %v", data)
	}
	requireNoEnvPullFeatureState(t, data)
	if strings.Contains(stdout.String(), "redis://") || strings.Contains(stdout.String(), "postgres://") {
		t.Fatalf("stdout must not echo values: %s", stdout.String())
	}
}

func TestAppsEnvPullDB_ExplicitDisablePrettyDescribesTheEnvironmentOnly(t *testing.T) {
	factory, stdout, _, reg := newEnvPullPGFactory(t)
	projectDir := t.TempDir()
	reg.Register(&httpmock.Stub{Method: "POST", URL: "/open-apis/spark/v1/apps/app_x/env_vars",
		Body: envVarsResponse(
			kv("FORCE_FRAMEWORK_DISABLE_DATAPASS", "true"),
			kv("SUDA_DATABASE_URL", ""),
			kv("FORCE_DB_CONNECT_URL", ""),
		)})

	if err := runEnvPull(t, factory, stdout, projectDir, "--format", "pretty"); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	out := stdout.String()
	for _, want := range []string{"Local database integration: disabled by the pulled environment", "Restart the whole local dev process", "verify one real database read/write"} {
		if !strings.Contains(out, want) {
			t.Fatalf("pretty output missing %q: %s", want, out)
		}
	}
	for _, claim := range []string{"not enabled for this app", "Database: enabled", "Development database detected"} {
		if strings.Contains(out, claim) {
			t.Fatalf("pretty output must not claim %q: %s", claim, out)
		}
	}
}

// A "true" flag without both connection keys present is not an explicit clear:
// a missing key must not delete the local value, so these responses keep the
// legacy merge (old-contract apps can carry the flag in their custom variables).
func TestAppsEnvPullDB_DisableWithAMissingConnectionKeyKeepsLocalValues(t *testing.T) {
	cases := map[string]struct {
		resp map[string]any
		want []string
	}{
		"FORCE_DB_CONNECT_URL missing": {
			resp: envVarsResponse(kv("FORCE_FRAMEWORK_DISABLE_DATAPASS", "true"), kv("SUDA_DATABASE_URL", "")),
			want: []string{`SUDA_DATABASE_URL=""`, `FORCE_DB_CONNECT_URL="postgres://old-direct"`},
		},
		"SUDA_DATABASE_URL missing": {
			resp: envVarsResponse(kv("FORCE_FRAMEWORK_DISABLE_DATAPASS", "true"), kv("FORCE_DB_CONNECT_URL", "")),
			want: []string{`SUDA_DATABASE_URL="postgres://old-temp"`, `FORCE_DB_CONNECT_URL=""`},
		},
		"both connection keys missing": {
			resp: envVarsResponse(kv("FORCE_FRAMEWORK_DISABLE_DATAPASS", "true")),
			want: []string{`SUDA_DATABASE_URL="postgres://old-temp"`, `FORCE_DB_CONNECT_URL="postgres://old-direct"`},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			factory, stdout, _, reg := newEnvPullPGFactory(t)
			projectDir := t.TempDir()
			writeEnvLocal(t, projectDir, staleEnvLocal)
			reg.Register(&httpmock.Stub{Method: "POST", URL: "/open-apis/spark/v1/apps/app_x/env_vars", Body: tc.resp})

			if err := runEnvPull(t, factory, stdout, projectDir); err != nil {
				t.Fatalf("execute err=%v", err)
			}
			got := readEnvLocal(t, projectDir)
			requireEnvLines(t, got, append([]string{`FORCE_FRAMEWORK_DISABLE_DATAPASS="true"`, `FORCE_DB_BRANCH="dev"`, `MY_VAR="keep-me"`, `MIAODA_CACHE_URL="redis://old"`}, tc.want...)...)
			data := decodeSuccessData(t, stdout)
			requireNoEnvPullFeatureState(t, data)
			for _, key := range []string{"restart_required", "shell_conflicts"} {
				if _, present := data[key]; present {
					t.Fatalf("legacy response must not gain %s: %v", key, data)
				}
			}
		})
	}
}

// The server exports a dev connection (e.g. after +feature-enable --feature database on
// a multi-environment app): the stale "true" flag in .env.local is replaced, and
// the CLI only asks for a restart — it never restarts anything itself and does
// not claim the database is reachable.
func TestAppsEnvPullDB_ExportedConnectionReplacesStaleDisableFlag(t *testing.T) {
	factory, stdout, _, reg := newEnvPullPGFactory(t)
	projectDir := t.TempDir()
	writeEnvLocal(t, projectDir, "FORCE_FRAMEWORK_DISABLE_DATAPASS=\"true\"\nSUDA_DATABASE_URL=\"\"\nFORCE_DB_BRANCH=\"\"\nMY_VAR=\"keep-me\"\n")
	reg.Register(&httpmock.Stub{Method: "POST", URL: "/open-apis/spark/v1/apps/app_x/env_vars",
		Body: envVarsResponse(
			kv("FORCE_FRAMEWORK_DISABLE_DATAPASS", "false"),
			map[string]any{"key": "SUDA_DATABASE_URL", "value": "postgres://new-dev", "extras": []any{map[string]any{"key": "expiresAt", "value": "1780389006"}}},
			kv("FORCE_DB_CONNECT_URL", "postgres://new-dev"),
			kv("FORCE_DB_BRANCH", "dev"),
		)})

	if err := runEnvPull(t, factory, stdout, projectDir, "--format", "pretty"); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := readEnvLocal(t, projectDir)
	requireEnvLines(t, got,
		`FORCE_FRAMEWORK_DISABLE_DATAPASS="false"`,
		`SUDA_DATABASE_URL="postgres://new-dev"`,
		`FORCE_DB_CONNECT_URL="postgres://new-dev"`,
		`FORCE_DB_BRANCH="dev"`,
		`MY_VAR="keep-me"`,
	)
	if strings.Count(got, "FORCE_FRAMEWORK_DISABLE_DATAPASS=") != 1 {
		t.Fatalf("duplicate disable flag:\n%s", got)
	}
	out := stdout.String()
	for _, want := range []string{"Local database integration: connection exported (branch dev)", "does not prove the database is reachable", "Restart the whole local dev process", "verify one real database read/write"} {
		if !strings.Contains(out, want) {
			t.Fatalf("pretty output missing %q: %s", want, out)
		}
	}
	if strings.Contains(out, "Database: enabled") {
		t.Fatalf("pretty output must not claim the app database is enabled: %s", out)
	}
}

func TestAppsEnvPullDB_ExportedConnectionJSONReportsRestartWithoutFeatureState(t *testing.T) {
	factory, stdout, _, reg := newEnvPullPGFactory(t)
	projectDir := t.TempDir()
	reg.Register(&httpmock.Stub{Method: "POST", URL: "/open-apis/spark/v1/apps/app_x/env_vars",
		Body: envVarsResponse(kv("FORCE_FRAMEWORK_DISABLE_DATAPASS", "false"), kv("SUDA_DATABASE_URL", "postgres://new-dev"))})

	if err := runEnvPull(t, factory, stdout, projectDir); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	data := decodeSuccessData(t, stdout)
	if data["restart_required"] != true {
		t.Fatalf("data = %v", data)
	}
	requireNoEnvPullFeatureState(t, data)
	if strings.Contains(stdout.String(), "postgres://") {
		t.Fatalf("stdout must not echo the connection: %s", stdout.String())
	}
}

// Responses that are neither explicit answer — every old-contract server, and
// old-contract apps whose custom variables carry the disable flag — keep the
// established behavior: the file is merged, the branch is pinned to dev,
// nothing is cleared, and no database fields are added.
func TestAppsEnvPullDB_OtherShapesKeepLegacyBehavior(t *testing.T) {
	cases := map[string]map[string]any{
		"no database keys":            envVarsResponse(kv("MIAODA_CACHE_URL", "redis://new")),
		"connection without the flag": envVarsResponse(kv("SUDA_DATABASE_URL", "postgres://expert")),
		"empty connection only":       envVarsResponse(kv("SUDA_DATABASE_URL", "")),
		"false without a connection":  envVarsResponse(kv("FORCE_FRAMEWORK_DISABLE_DATAPASS", "false")),
		"true with a connection":      envVarsResponse(kv("FORCE_FRAMEWORK_DISABLE_DATAPASS", "true"), kv("SUDA_DATABASE_URL", "postgres://expert"), kv("FORCE_DB_CONNECT_URL", "")),
		"non-string flag":             envVarsResponse(kv("FORCE_FRAMEWORK_DISABLE_DATAPASS", true)),
	}
	for name, resp := range cases {
		t.Run(name, func(t *testing.T) {
			factory, stdout, _, reg := newEnvPullPGFactory(t)
			projectDir := t.TempDir()
			writeEnvLocal(t, projectDir, "MY_VAR=\"keep-me\"\nFORCE_DB_CONNECT_URL=\"postgres://kept\"\n")
			reg.Register(&httpmock.Stub{Method: "POST", URL: "/open-apis/spark/v1/apps/app_x/env_vars", Body: resp})

			if err := runEnvPull(t, factory, stdout, projectDir); err != nil {
				t.Fatalf("execute err=%v", err)
			}
			got := readEnvLocal(t, projectDir)
			requireEnvLines(t, got, `FORCE_DB_BRANCH="dev"`, `MY_VAR="keep-me"`)
			if name != "true with a connection" {
				requireEnvLines(t, got, `FORCE_DB_CONNECT_URL="postgres://kept"`)
			}
			data := decodeSuccessData(t, stdout)
			requireNoEnvPullFeatureState(t, data)
			for _, key := range []string{"restart_required", "shell_conflicts"} {
				if _, present := data[key]; present {
					t.Fatalf("legacy response must not gain %s: %v", key, data)
				}
			}
		})
	}
}

func TestAppsEnvPullDB_EmptyConnectionIsNotReportedAsDatabase(t *testing.T) {
	factory, stdout, _, reg := newEnvPullPGFactory(t)
	projectDir := t.TempDir()
	reg.Register(&httpmock.Stub{Method: "POST", URL: "/open-apis/spark/v1/apps/app_x/env_vars",
		Body: envVarsResponse(kv("SUDA_DATABASE_URL", ""))})
	if err := runEnvPull(t, factory, stdout, projectDir, "--format", "pretty"); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if out := stdout.String(); strings.Contains(out, "Development database detected") || strings.Contains(out, "Restart the whole") {
		t.Fatalf("an empty connection must not be reported as a database: %s", out)
	}
}

// A single-environment app has no dev database: the server answers with an
// error, which passes through unchanged and leaves .env.local untouched. An
// enabled database switch alone is never treated as local readiness.
func TestAppsEnvPullDB_SingleEnvironmentErrorLeavesFileUntouched(t *testing.T) {
	factory, stdout, _, reg := newEnvPullPGFactory(t)
	projectDir := t.TempDir()
	writeEnvLocal(t, projectDir, staleEnvLocal)
	reg.Register(&httpmock.Stub{Method: "POST", URL: "/open-apis/spark/v1/apps/app_x/env_vars",
		Body: map[string]any{"code": 500001, "msg": "database dev environment not ready, next_action=db_env_create"}})

	err := runEnvPull(t, factory, stdout, projectDir)
	if err == nil {
		t.Fatal("expected the server error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok || !strings.Contains(p.Message, "database dev environment not ready") {
		t.Fatalf("server error must pass through: %T %v", err, err)
	}
	if got := readEnvLocal(t, projectDir); got != staleEnvLocal {
		t.Fatalf("a failed pull must not touch .env.local:\n%s", got)
	}
	if stdout.Len() != 0 {
		t.Fatalf("no success output expected: %s", stdout.String())
	}
}

func TestAppsEnvPullDB_ShellConflictIsReportedWithoutValues(t *testing.T) {
	factory, stdout, stderr, reg := newEnvPullPGFactory(t)
	t.Setenv("SUDA_DATABASE_URL", "postgres://shell-stale")
	t.Setenv("FORCE_FRAMEWORK_DISABLE_DATAPASS", "true") // matches the pulled value: no conflict
	projectDir := t.TempDir()
	reg.Register(&httpmock.Stub{Method: "POST", URL: "/open-apis/spark/v1/apps/app_x/env_vars",
		Body: envVarsResponse(kv("FORCE_FRAMEWORK_DISABLE_DATAPASS", "true"), kv("SUDA_DATABASE_URL", ""), kv("FORCE_DB_CONNECT_URL", ""))})

	if err := runEnvPull(t, factory, stdout, projectDir); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	data := decodeSuccessData(t, stdout)
	if !reflect.DeepEqual(data["shell_conflicts"], []any{"SUDA_DATABASE_URL"}) {
		t.Fatalf("shell_conflicts = %v", data["shell_conflicts"])
	}
	warn := stderr.String()
	if !strings.Contains(warn, "unset SUDA_DATABASE_URL") || !strings.Contains(warn, "restart the local dev process") {
		t.Fatalf("stderr must tell the user to clear the shell value and restart: %q", warn)
	}
	if strings.Contains(warn+stdout.String(), "shell-stale") {
		t.Fatal("conflict report must not print the shell value")
	}
}
