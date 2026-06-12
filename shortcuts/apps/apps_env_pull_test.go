// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
)

func assertValidationError(t *testing.T, err error, wantSubstr string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a validation error, got nil")
	}
	if !errs.IsValidation(err) && output.ExitCodeOf(err) != output.ExitValidation {
		t.Fatalf("expected validation error, got %T: %v", err, err)
	}
	if wantSubstr != "" && !strings.Contains(err.Error(), wantSubstr) {
		t.Fatalf("expected validation message containing %q, got %v", wantSubstr, err)
	}
}

func TestResolveEnvPullTarget_DefaultProjectPathUsesCWD(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() err=%v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldwd) })
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("Chdir() err=%v", err)
	}
	wantProject, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() after Chdir err=%v", err)
	}

	gotProject, gotFile, err := resolveEnvPullTarget("")
	if err != nil {
		t.Fatalf("resolveEnvPullTarget() err=%v", err)
	}
	if gotProject != wantProject {
		t.Fatalf("project path = %q, want %q", gotProject, wantProject)
	}
	wantFile := filepath.Join(wantProject, ".env.local")
	if gotFile != wantFile {
		t.Fatalf("env file = %q, want %q", gotFile, wantFile)
	}
}

func TestResolveEnvPullTarget_CustomProjectPath(t *testing.T) {
	root := t.TempDir()
	gotProject, gotFile, err := resolveEnvPullTarget(root)
	if err != nil {
		t.Fatalf("resolveEnvPullTarget() err=%v", err)
	}
	if gotProject != root {
		t.Fatalf("project path = %q, want %q", gotProject, root)
	}
	wantFile := filepath.Join(root, ".env.local")
	if gotFile != wantFile {
		t.Fatalf("env file = %q, want %q", gotFile, wantFile)
	}
}

func TestCheckEnvPullTargetRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	realFile := filepath.Join(dir, "real.env")
	if err := os.WriteFile(realFile, []byte("A = \"1\"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() err=%v", err)
	}
	link := filepath.Join(dir, ".env.local")
	if err := os.Symlink(realFile, link); err != nil {
		t.Fatalf("Symlink() err=%v", err)
	}

	err := checkEnvPullTarget(link)
	assertValidationError(t, err, "must be a regular file")
}

func TestCheckEnvPullTargetRejectsDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".env.local")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() err=%v", err)
	}

	err := checkEnvPullTarget(dir)
	assertValidationError(t, err, "must be a regular file")
}

func TestParseEnvPullAssignmentLine(t *testing.T) {
	tests := map[string]string{
		`FOO = "bar"`:    "FOO",
		`FOO=bar`:        "FOO",
		`FOO = bar`:      "FOO",
		`FOO='bar'`:      "FOO",
		`export FOO=bar`: "FOO",
		`FOO=`:           "FOO",
		`FOO=a=b=c`:      "FOO",
	}
	for line, want := range tests {
		key, ok := parseEnvPullAssignmentLine(line)
		if !ok {
			t.Fatalf("expected line to parse: %q", line)
		}
		if key != want {
			t.Fatalf("key for %q = %q, want %q", line, key, want)
		}
	}
}

func TestParseEnvPullAssignmentLineRejectsComment(t *testing.T) {
	if _, ok := parseEnvPullAssignmentLine("# FOO = \"bar\""); ok {
		t.Fatalf("commented line should not be treated as active assignment")
	}
}

func TestParseEnvPullAssignmentLineRejectsInvalidExport(t *testing.T) {
	if _, ok := parseEnvPullAssignmentLine("export =bar"); ok {
		t.Fatalf("invalid export line should not be treated as active assignment")
	}
}

func TestParseEnvPullAssignmentLineTreatsExportPrefixWithoutDelimiterAsKey(t *testing.T) {
	key, ok := parseEnvPullAssignmentLine("exportFOO=bar")
	if !ok {
		t.Fatalf("expected export-prefixed key to parse")
	}
	if key != "exportFOO" {
		t.Fatalf("key = %q, want exportFOO", key)
	}
}

func TestFormatEnvPullAssignmentEscapesQuotesAndBackslashes(t *testing.T) {
	got := formatEnvPullAssignment("TOKEN", `a"b\c`)
	want := `TOKEN="a\"b\\c"`
	if got != want {
		t.Fatalf("formatEnvPullAssignment() = %q, want %q", got, want)
	}
}

func TestMergeEnvPullFileContentPreservesCommentsAndMalformedLines(t *testing.T) {
	original := strings.Join([]string{
		"# FOO = \"old\"",
		"FOO=old",
		"BROKEN LINE",
		"KEEP = \"stay\"",
		"",
	}, "\n")

	merged, updated, created := mergeEnvPullFileContent(original, map[string]string{
		"FOO": "new",
		"BAR": "added",
	})

	if !strings.Contains(merged, "# FOO = \"old\"") {
		t.Fatalf("comment line must be preserved: %q", merged)
	}
	if !strings.Contains(merged, `FOO="new"`) {
		t.Fatalf("active key must be updated: %q", merged)
	}
	if !strings.Contains(merged, "BROKEN LINE") {
		t.Fatalf("malformed line must be preserved: %q", merged)
	}
	if !strings.Contains(merged, "KEEP = \"stay\"") {
		t.Fatalf("unrelated key must be preserved: %q", merged)
	}
	if !strings.Contains(merged, `BAR="added"`) {
		t.Fatalf("missing key must be appended: %q", merged)
	}
	if len(updated) != 1 || updated[0] != "FOO" {
		t.Fatalf("updated = %v, want [FOO]", updated)
	}
	if len(created) != 1 || created[0] != "BAR" {
		t.Fatalf("created = %v, want [BAR]", created)
	}
}

func TestMergeEnvPullFileContentUpdatesCommonAssignmentStylesWithoutDuplicateKeys(t *testing.T) {
	original := strings.Join([]string{
		`FOO=old`,
		`BAR = old`,
		`export BAZ=old`,
		`QUX='old'`,
		"",
	}, "\n")

	merged, updated, created := mergeEnvPullFileContent(original, map[string]string{
		"FOO": "new-foo",
		"BAR": "new-bar",
		"BAZ": "new-baz",
		"QUX": "new-qux",
	})

	for _, want := range []string{
		`FOO="new-foo"`,
		`BAR="new-bar"`,
		`BAZ="new-baz"`,
		`QUX="new-qux"`,
	} {
		if strings.Count(merged, want) != 1 {
			t.Fatalf("expected exactly one canonical assignment %q in %q", want, merged)
		}
	}
	for _, legacy := range []string{`FOO=old`, `BAR = old`, `export BAZ=old`, `QUX='old'`} {
		if strings.Contains(merged, legacy) {
			t.Fatalf("legacy assignment should be replaced, still found %q in %q", legacy, merged)
		}
	}
	if len(updated) != 4 {
		t.Fatalf("updated = %v, want 4 items", updated)
	}
	if len(created) != 0 {
		t.Fatalf("created = %v, want empty", created)
	}
}

func TestBuildEnvPullSuccessDataSuppressesEnvKeysAndValues(t *testing.T) {
	data := buildEnvPullSuccessData("app_x", "/repo/.env.local", envPullDatabaseInfo{Detected: true, ExpiresAtRaw: "1780389006", ExpiresAtText: "2026-06-02 16:30:06 CST"})

	if _, ok := data["updated"]; ok {
		t.Fatalf("success data must not expose updated key names: %v", data)
	}
	if _, ok := data["created"]; ok {
		t.Fatalf("success data must not expose created key names: %v", data)
	}
	if _, ok := data["project_path"]; ok {
		t.Fatalf("success data must not expose project_path: %v", data)
	}
	if _, ok := data["updated_count"]; ok {
		t.Fatalf("success data must not expose updated_count: %v", data)
	}
	if _, ok := data["created_count"]; ok {
		t.Fatalf("success data must not expose created_count: %v", data)
	}
	if got := data["app_id"]; got != "app_x" {
		t.Fatalf("app_id = %v, want app_x", got)
	}
	if got := data["env_file"]; got != "/repo/.env.local" {
		t.Fatalf("env_file = %v, want /repo/.env.local", got)
	}
	if got := data["database_url_expires_at"]; got != "1780389006" {
		t.Fatalf("database_url_expires_at = %v, want 1780389006", got)
	}
}

func TestAppsEnvPull_DryRunUsesPostAndResolvedEnvFile(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	projectDir := t.TempDir()

	if err := runAppsShortcut(t, AppsEnvPull,
		[]string{"+env-pull", "--app-id", "app_x", "--project-path", projectDir, "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, `"method": "POST"`) {
		t.Fatalf("dry-run must use POST: %s", got)
	}
	if !strings.Contains(got, `/open-apis/spark/v1/apps/app_x/env_vars`) {
		t.Fatalf("dry-run missing endpoint: %s", got)
	}
	if !strings.Contains(got, filepath.Join(projectDir, ".env.local")) {
		t.Fatalf("dry-run must include resolved env file path: %s", got)
	}
}

func TestAppsEnvPull_PrettyOutput_WithDatabaseLine(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	projectDir := t.TempDir()
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/env_vars",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"env_vars": []interface{}{
					map[string]interface{}{"key": "SUDA_DATABASE_URL", "value": "postgres://db", "extras": []interface{}{map[string]interface{}{"key": "expiresAt", "value": "1780389006"}}},
					map[string]interface{}{"key": "APP_ID", "value": "app_x"},
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsEnvPull,
		[]string{"+env-pull", "--app-id", "app_x", "--project-path", projectDir, "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, "App detected: app_x") {
		t.Fatalf("missing app summary: %q", got)
	}
	if !strings.Contains(got, "Development database detected") {
		t.Fatalf("missing database line: %q", got)
	}
	if !strings.Contains(got, "✓ Local environment written to "+filepath.Join(projectDir, ".env.local")) {
		t.Fatalf("missing env file write line in pretty output: %q", got)
	}
	wantExpiry := time.Unix(1780389006, 0).Local().Format("2006-01-02 15:04:05 MST")
	if !strings.Contains(got, "\n\nDATABASE_URL is valid until "+wantExpiry+".\n") {
		t.Fatalf("missing blank-line separated expiry block: %q", got)
	}
	if !strings.Contains(got, "Run `lark-cli apps +env-pull --app-id <app_id>` again to refresh it.") {
		t.Fatalf("missing refresh hint line: %q", got)
	}
	if strings.Contains(got, "postgres://db") {
		t.Fatalf("pretty output must not print env values: %q", got)
	}
}

func TestAppsEnvPull_JSONOutput_UsesSummaryFieldsOnly(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	projectDir := t.TempDir()
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/env_vars",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"env_vars": []interface{}{
					map[string]interface{}{"key": "AAA", "value": "value-a"},
					map[string]interface{}{"key": "SUDA_DATABASE_URL", "value": "postgres://db", "extras": []interface{}{map[string]interface{}{"key": "expiresAt", "value": "1780389006"}}},
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsEnvPull,
		[]string{"+env-pull", "--app-id", "app_x", "--project-path", projectDir, "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, `"app_id": "app_x"`) {
		t.Fatalf("json output must expose app_id: %s", got)
	}
	if !strings.Contains(got, `"env_file": "`+filepath.Join(projectDir, ".env.local")+`"`) {
		t.Fatalf("json output must expose env_file: %s", got)
	}
	if !strings.Contains(got, `"database_url_expires_at": "1780389006"`) {
		t.Fatalf("json output must expose raw database_url_expires_at: %s", got)
	}
	if strings.Contains(got, `"project_path"`) {
		t.Fatalf("json output must not expose project_path: %s", got)
	}
	if strings.Contains(got, `"updated_count"`) || strings.Contains(got, `"created_count"`) {
		t.Fatalf("json output must not expose count fields: %s", got)
	}
	if strings.Contains(got, `"AAA"`) || strings.Contains(got, `"value-a"`) || strings.Contains(got, `"postgres://db"`) {
		t.Fatalf("json output must not expose env keys or env values: %s", got)
	}
}

func TestAppsEnvPull_MalformedPayloadSkipsInvalidEntries(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	projectDir := t.TempDir()
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/env_vars",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"env_vars": []interface{}{"bad"},
			},
		},
	})

	err := runAppsShortcut(t, AppsEnvPull,
		[]string{"+env-pull", "--app-id", "app_x", "--project-path", projectDir, "--format", "pretty", "--as", "user"},
		factory, stdout)
	if err != nil {
		t.Fatalf("malformed entries should be skipped, not fail; err=%v", err)
	}
}

func TestAppsEnvPull_TargetSymlinkIsRejectedBeforeAPI(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	projectDir := t.TempDir()
	linkTarget := filepath.Join(projectDir, "real.env")
	if err := os.WriteFile(linkTarget, []byte("KEEP = \"1\"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() err=%v", err)
	}
	if err := os.Symlink(linkTarget, filepath.Join(projectDir, ".env.local")); err != nil {
		t.Fatalf("Symlink() err=%v", err)
	}

	err := runAppsShortcut(t, AppsEnvPull,
		[]string{"+env-pull", "--app-id", "app_x", "--project-path", projectDir, "--as", "user"},
		factory, stdout)
	assertValidationError(t, err, "must be a regular file")
}

func TestReadEnvPullFile_MissingFileReturnsEmpty(t *testing.T) {
	got, err := readEnvPullFile(filepath.Join(t.TempDir(), "missing.env"))
	if err != nil {
		t.Fatalf("readEnvPullFile() err=%v", err)
	}
	if got != "" {
		t.Fatalf("readEnvPullFile() = %q, want empty string", got)
	}
}

func TestAppsEnvPull_WritesCanonicalEnvFile(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	projectDir := t.TempDir()
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/env_vars",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"env_vars": map[string]interface{}{
					"AAA": "new",
					"BBB": `quote"and\\slash`,
				},
			},
		},
	})
	if err := os.WriteFile(filepath.Join(projectDir, ".env.local"), []byte(strings.Join([]string{
		"# AAA = \"commented\"",
		"AAA=old",
		"KEEP = \"stay\"",
		"BROKEN LINE",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile() err=%v", err)
	}

	if err := runAppsShortcut(t, AppsEnvPull,
		[]string{"+env-pull", "--app-id", "app_x", "--project-path", projectDir, "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}

	data, err := os.ReadFile(filepath.Join(projectDir, ".env.local"))
	if err != nil {
		t.Fatalf("ReadFile() err=%v", err)
	}
	got := string(data)
	if !strings.Contains(got, "# AAA = \"commented\"") {
		t.Fatalf("comment must be preserved: %q", got)
	}
	if !strings.Contains(got, `AAA="new"`) {
		t.Fatalf("active value must be updated: %q", got)
	}
	if !strings.Contains(got, `BBB="quote\"and\\\\slash"`) {
		t.Fatalf("new key must be appended canonically: %q", got)
	}
	if !strings.Contains(got, "KEEP = \"stay\"") {
		t.Fatalf("unrelated key must be preserved: %q", got)
	}
	if !strings.Contains(got, "BROKEN LINE") {
		t.Fatalf("malformed line must be preserved: %q", got)
	}
}

func TestAppsEnvPull_DryRunDoesNotWriteFile(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	projectDir := t.TempDir()
	target := filepath.Join(projectDir, ".env.local")

	if err := runAppsShortcut(t, AppsEnvPull,
		[]string{"+env-pull", "--app-id", "app_x", "--project-path", projectDir, "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry-run must not create target file, stat err=%v", err)
	}
}

func TestAppsEnvPull_JSONOutputOmitsDatabaseLineText(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	projectDir := t.TempDir()
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/env_vars",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"env_vars": map[string]interface{}{
					"SUDA_DATABASE_URL": "short-lived-db-token",
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsEnvPull,
		[]string{"+env-pull", "--app-id", "app_x", "--project-path", projectDir, "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if strings.Contains(stdout.String(), "Development database detected") {
		t.Fatalf("json output must not include pretty text: %s", stdout.String())
	}
}

func TestAppsEnvPull_ValidationRequiresAppID(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsEnvPull,
		[]string{"+env-pull", "--project-path", t.TempDir(), "--as", "user"},
		factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "app-id") {
		t.Fatalf("expected missing app-id error, got %v", err)
	}
}

func TestAppsEnvPull_ExecuteUsesNestedDataEnvVars(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	projectDir := t.TempDir()
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/env_vars",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"env_vars": map[string]interface{}{
					"AAA": "value-a",
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsEnvPull,
		[]string{"+env-pull", "--app-id", "app_x", "--project-path", projectDir, "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	data, err := os.ReadFile(filepath.Join(projectDir, ".env.local"))
	if err != nil {
		t.Fatalf("ReadFile() err=%v", err)
	}
	if !strings.Contains(string(data), `AAA="value-a"`) {
		t.Fatalf("expected nested data env vars to be written, got %q", string(data))
	}
}

func TestAppsEnvPull_ExecuteUsesArrayEnvVars(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	projectDir := t.TempDir()
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/env_vars",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"env_vars": []interface{}{
					map[string]interface{}{"key": "AAA", "value": "value-a"},
					map[string]interface{}{"key": "SUDA_DATABASE_URL", "value": "postgres://db", "extras": []interface{}{map[string]interface{}{"key": "expiresAt", "value": "1780389006"}}},
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsEnvPull,
		[]string{"+env-pull", "--app-id", "app_x", "--project-path", projectDir, "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	data, err := os.ReadFile(filepath.Join(projectDir, ".env.local"))
	if err != nil {
		t.Fatalf("ReadFile() err=%v", err)
	}
	gotFile := string(data)
	if !strings.Contains(gotFile, `AAA="value-a"`) {
		t.Fatalf("expected array env_vars entry to be written, got %q", gotFile)
	}
	if !strings.Contains(gotFile, `SUDA_DATABASE_URL="postgres://db"`) {
		t.Fatalf("expected SUDA_DATABASE_URL array entry to be written, got %q", gotFile)
	}
	gotOut := stdout.String()
	if !strings.Contains(gotOut, "Development database detected") {
		t.Fatalf("expected database line in pretty output, got %q", gotOut)
	}
	wantExpiry := time.Unix(1780389006, 0).Local().Format("2006-01-02 15:04:05 MST")
	if !strings.Contains(gotOut, "DATABASE_URL is valid until "+wantExpiry+".") {
		t.Fatalf("expected expiry line in pretty output, got %q", gotOut)
	}
	if strings.Contains(gotOut, "expiresAt") {
		t.Fatalf("extras metadata must not leak to output, got %q", gotOut)
	}
}

func TestAppsEnvPull_JSONOutputCanBeDecoded(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	projectDir := t.TempDir()
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/env_vars",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"env_vars": map[string]interface{}{
					"AAA": "value-a",
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsEnvPull,
		[]string{"+env-pull", "--app-id", "app_x", "--project-path", projectDir, "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}

	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			AppID                string `json:"app_id"`
			EnvFile              string `json:"env_file"`
			DatabaseURLExpiresAt string `json:"database_url_expires_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("json.Unmarshal() err=%v; stdout=%s", err, stdout.String())
	}
	if !envelope.OK {
		t.Fatalf("expected ok=true envelope, got %+v", envelope)
	}
	if envelope.Data.AppID != "app_x" {
		t.Fatalf("app_id = %q, want app_x", envelope.Data.AppID)
	}
	if envelope.Data.EnvFile != filepath.Join(projectDir, ".env.local") {
		t.Fatalf("env_file = %q, want %q", envelope.Data.EnvFile, filepath.Join(projectDir, ".env.local"))
	}
	if envelope.Data.DatabaseURLExpiresAt != "" {
		t.Fatalf("database_url_expires_at = %q, want empty for payload without SUDA_DATABASE_URL extras", envelope.Data.DatabaseURLExpiresAt)
	}
}

func TestAppsEnvPull_PrettyOutputWithoutDatabaseLine(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	projectDir := t.TempDir()
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/env_vars",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"env_vars": map[string]interface{}{
					"AAA": "value-a",
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsEnvPull,
		[]string{"+env-pull", "--app-id", "app_x", "--project-path", projectDir, "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if strings.Contains(stdout.String(), "Development database detected") {
		t.Fatalf("unexpected database line in pretty output: %q", stdout.String())
	}
}

func TestMergeEnvPullFileContentEmptyEnvVarsPreservesOriginalNewline(t *testing.T) {
	original := "KEEP = \"stay\""
	merged, updated, created := mergeEnvPullFileContent(original, map[string]string{})
	if merged != "KEEP = \"stay\"\n" {
		t.Fatalf("merged = %q, want trailing newline preserved", merged)
	}
	if len(updated) != 0 || len(created) != 0 {
		t.Fatalf("updated=%v created=%v, want both empty", updated, created)
	}
}

func TestParseEnvPullAssignmentLineRejectsInvalidKey(t *testing.T) {
	if _, ok := parseEnvPullAssignmentLine("FOO BAR=baz"); ok {
		t.Fatalf("assignment with whitespace in key should not be treated as active assignment")
	}
}

func TestResolveEnvPullTargetCleansCustomPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "demo")
	input := filepath.Join(root, ".", "sub", "..")
	gotProject, gotFile, err := resolveEnvPullTarget(input)
	if err != nil {
		t.Fatalf("resolveEnvPullTarget() err=%v", err)
	}
	wantProject := filepath.Clean(input)
	if gotProject != wantProject {
		t.Fatalf("project path = %q, want %q", gotProject, wantProject)
	}
	if gotFile != filepath.Join(wantProject, ".env.local") {
		t.Fatalf("env file = %q, want %q", gotFile, filepath.Join(wantProject, ".env.local"))
	}
}

func TestAppsEnvPull_DatabaseExtrasWithoutExpiresAtDoesNotFail(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	projectDir := t.TempDir()
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/env_vars",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"env_vars": []interface{}{
					map[string]interface{}{"key": "SUDA_DATABASE_URL", "value": "postgres://db", "extras": []interface{}{map[string]interface{}{"key": "notExpiresAt", "value": "1780389006"}}},
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsEnvPull,
		[]string{"+env-pull", "--app-id", "app_x", "--project-path", projectDir, "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, "Development database detected") {
		t.Fatalf("expected database detection line, got %q", got)
	}
	if strings.Contains(got, "DATABASE_URL is valid until") {
		t.Fatalf("did not expect expiry line when expiresAt is absent, got %q", got)
	}
}

func TestWriteEnvPullPretty(t *testing.T) {
	var buf bytes.Buffer
	writeEnvPullPretty(&buf, "app_x", "/repo/.env.local", envPullDatabaseInfo{Detected: true, ExpiresAtText: "2026-06-02 16:30:06 CST"}, nil)
	got := buf.String()
	if !strings.Contains(got, "App detected: app_x") {
		t.Fatalf("missing app line: %q", got)
	}
	if !strings.Contains(got, "Development database detected") {
		t.Fatalf("missing database line: %q", got)
	}
	if !strings.Contains(got, "✓ Local environment written to /repo/.env.local") {
		t.Fatalf("missing env file write line: %q", got)
	}
	if !strings.Contains(got, "\n\nDATABASE_URL is valid until 2026-06-02 16:30:06 CST.\n") {
		t.Fatalf("missing blank-line separated expiry block: %q", got)
	}
	if strings.Contains(got, "Skipped") {
		t.Fatalf("no skipped warning when skippedKeys is nil: %q", got)
	}
	if !strings.Contains(got, "Run `lark-cli apps +env-pull --app-id <app_id>` again to refresh it.") {
		t.Fatalf("missing refresh hint line: %q", got)
	}
}

func TestWriteEnvPullPretty_SkippedKeys(t *testing.T) {
	var buf bytes.Buffer
	writeEnvPullPretty(&buf, "app_x", "/repo/.env.local", envPullDatabaseInfo{}, []string{"bad key", "=eq"})
	got := buf.String()
	if !strings.Contains(got, "⚠ Skipped 2 invalid key(s): bad key, =eq") {
		t.Fatalf("missing skipped keys warning: %q", got)
	}
}

func TestExtractEnvPullVars_SkipsInvalidKeys(t *testing.T) {
	data := map[string]interface{}{
		"env_vars": map[string]interface{}{
			"VALID_KEY":      "ok",
			"also_valid_123": "ok2",
			"has space":      "skip1",
			"has\nnewline":   "skip2",
			"=starts-eq":     "skip3",
			"":               "skip4",
			"has=equals":     "skip5",
		},
	}
	got, _, skipped, err := extractEnvPullVars(data)
	if err != nil {
		t.Fatalf("extractEnvPullVars() err=%v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 valid keys, got %d: %v", len(got), got)
	}
	if len(skipped) != 5 {
		t.Fatalf("expected 5 skipped keys, got %d: %v", len(skipped), skipped)
	}
	if got["VALID_KEY"] != "ok" {
		t.Fatalf("VALID_KEY = %q, want ok", got["VALID_KEY"])
	}
	if got["also_valid_123"] != "ok2" {
		t.Fatalf("also_valid_123 = %q, want ok2", got["also_valid_123"])
	}
}

func TestExtractEnvPullVars_ArraySkipsInvalidKeys(t *testing.T) {
	data := map[string]interface{}{
		"env_vars": []interface{}{
			map[string]interface{}{"key": "GOOD_KEY", "value": "val1"},
			map[string]interface{}{"key": "bad key", "value": "val2"},
		},
	}
	got, _, skipped, err := extractEnvPullVars(data)
	if err != nil {
		t.Fatalf("extractEnvPullVars() err=%v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 valid key, got %d: %v", len(got), got)
	}
	if len(skipped) != 1 || skipped[0] != "bad key" {
		t.Fatalf("expected 1 skipped key 'bad key', got %v", skipped)
	}
	if got["GOOD_KEY"] != "val1" {
		t.Fatalf("GOOD_KEY = %q, want val1", got["GOOD_KEY"])
	}
}

func TestAppsEnvPull_InjectsForceDBBranchWhenAbsent(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	projectDir := t.TempDir()
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/env_vars",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"env_vars": map[string]interface{}{
					"AAA": "value-a",
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsEnvPull,
		[]string{"+env-pull", "--app-id", "app_x", "--project-path", projectDir, "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	data, err := os.ReadFile(filepath.Join(projectDir, ".env.local"))
	if err != nil {
		t.Fatalf("ReadFile() err=%v", err)
	}
	got := string(data)
	if !strings.Contains(got, `FORCE_DB_BRANCH="dev"`) {
		t.Fatalf("expected FORCE_DB_BRANCH to be injected, got %q", got)
	}
	if !strings.Contains(got, `AAA="value-a"`) {
		t.Fatalf("expected upstream env vars to remain, got %q", got)
	}
}

func TestAppsEnvPull_InjectsForceDBBranchAlongsideArrayEnvVars(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	projectDir := t.TempDir()
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/env_vars",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"env_vars": []interface{}{
					map[string]interface{}{"key": "AAA", "value": "value-a"},
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsEnvPull,
		[]string{"+env-pull", "--app-id", "app_x", "--project-path", projectDir, "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	data, err := os.ReadFile(filepath.Join(projectDir, ".env.local"))
	if err != nil {
		t.Fatalf("ReadFile() err=%v", err)
	}
	if !strings.Contains(string(data), `FORCE_DB_BRANCH="dev"`) {
		t.Fatalf("expected FORCE_DB_BRANCH to be injected for array env_vars, got %q", string(data))
	}
}

func TestAppsEnvPull_ForceDBBranchOverwritesExistingLocalValue(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	projectDir := t.TempDir()
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/env_vars",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"env_vars": map[string]interface{}{
					"AAA": "value-a",
				},
			},
		},
	})
	if err := os.WriteFile(filepath.Join(projectDir, ".env.local"), []byte(strings.Join([]string{
		`FORCE_DB_BRANCH="prod"`,
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile() err=%v", err)
	}

	if err := runAppsShortcut(t, AppsEnvPull,
		[]string{"+env-pull", "--app-id", "app_x", "--project-path", projectDir, "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	data, err := os.ReadFile(filepath.Join(projectDir, ".env.local"))
	if err != nil {
		t.Fatalf("ReadFile() err=%v", err)
	}
	got := string(data)
	if !strings.Contains(got, `FORCE_DB_BRANCH="dev"`) {
		t.Fatalf("expected FORCE_DB_BRANCH to be overwritten with dev, got %q", got)
	}
	if strings.Contains(got, `FORCE_DB_BRANCH="prod"`) {
		t.Fatalf("expected stale FORCE_DB_BRANCH=\"prod\" to be replaced, got %q", got)
	}
	if strings.Count(got, "FORCE_DB_BRANCH=") != 1 {
		t.Fatalf("expected exactly one FORCE_DB_BRANCH assignment, got %q", got)
	}
}

func TestAppsEnvPull_ForceDBBranchInjectedEvenWhenUpstreamReturnsEmptyMap(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	projectDir := t.TempDir()
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/env_vars",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"env_vars": map[string]interface{}{},
			},
		},
	})

	if err := runAppsShortcut(t, AppsEnvPull,
		[]string{"+env-pull", "--app-id", "app_x", "--project-path", projectDir, "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	data, err := os.ReadFile(filepath.Join(projectDir, ".env.local"))
	if err != nil {
		t.Fatalf("ReadFile() err=%v", err)
	}
	if !strings.Contains(string(data), `FORCE_DB_BRANCH="dev"`) {
		t.Fatalf("expected FORCE_DB_BRANCH to be injected even with empty upstream map, got %q", string(data))
	}
}

func TestEnsureTrailingNewline_Cases(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"abc", "abc\n"},
		{"abc\n", "abc\n"},
	}
	for _, c := range cases {
		if got := ensureTrailingNewline(c.in); got != c.want {
			t.Errorf("ensureTrailingNewline(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestExtractEnvPullDatabaseExpiry_Cases(t *testing.T) {
	t.Run("not a slice", func(t *testing.T) {
		raw, text := extractEnvPullDatabaseExpiry("nope")
		if raw != "" || text != "" {
			t.Errorf("got %q,%q want empty", raw, text)
		}
	})
	t.Run("no expiresAt key", func(t *testing.T) {
		raw, text := extractEnvPullDatabaseExpiry([]interface{}{
			map[string]interface{}{"key": "other", "value": "1"},
		})
		if raw != "" || text != "" {
			t.Errorf("got %q,%q want empty", raw, text)
		}
	})
	t.Run("non-map element skipped", func(t *testing.T) {
		raw, text := extractEnvPullDatabaseExpiry([]interface{}{"not-a-map"})
		if raw != "" || text != "" {
			t.Errorf("got %q,%q want empty", raw, text)
		}
	})
	t.Run("string timestamp", func(t *testing.T) {
		ts := int64(1700000000)
		raw, text := extractEnvPullDatabaseExpiry([]interface{}{
			map[string]interface{}{"key": "expiresAt", "value": "1700000000"},
		})
		want := time.Unix(ts, 0).Local().Format("2006-01-02 15:04:05 MST")
		if raw != "1700000000" || text != want {
			t.Errorf("got %q,%q want 1700000000,%q", raw, text, want)
		}
	})
	t.Run("float timestamp", func(t *testing.T) {
		ts := int64(1700000000)
		raw, text := extractEnvPullDatabaseExpiry([]interface{}{
			map[string]interface{}{"key": "expiresAt", "value": float64(ts)},
		})
		want := time.Unix(ts, 0).Local().Format("2006-01-02 15:04:05 MST")
		if raw != "1700000000" || text != want {
			t.Errorf("got %q,%q want 1700000000,%q", raw, text, want)
		}
	})
	t.Run("invalid string timestamp", func(t *testing.T) {
		raw, text := extractEnvPullDatabaseExpiry([]interface{}{
			map[string]interface{}{"key": "expiresAt", "value": "notanumber"},
		})
		if raw != "" || text != "" {
			t.Errorf("got %q,%q want empty", raw, text)
		}
	})
}

func TestExtractEnvPullVars_EdgeCases(t *testing.T) {
	t.Run("missing env_vars", func(t *testing.T) {
		_, _, _, err := extractEnvPullVars(map[string]interface{}{})
		if err == nil {
			t.Fatal("expected error for missing env_vars")
		}
	})
	t.Run("nested under data", func(t *testing.T) {
		vars, _, _, err := extractEnvPullVars(map[string]interface{}{
			"data": map[string]interface{}{
				"env_vars": map[string]interface{}{"FOO": "bar"},
			},
		})
		if err != nil || vars["FOO"] != "bar" {
			t.Fatalf("got vars=%v err=%v", vars, err)
		}
	})
	t.Run("array form skips non-string value", func(t *testing.T) {
		vars, _, _, err := extractEnvPullVars(map[string]interface{}{
			"env_vars": []interface{}{
				map[string]interface{}{"key": "K1", "value": "v1"},
				map[string]interface{}{"key": "K2", "value": 5},
			},
		})
		if err != nil {
			t.Fatalf("err=%v", err)
		}
		if vars["K1"] != "v1" {
			t.Errorf("K1 missing: %v", vars)
		}
		if _, ok := vars["K2"]; ok {
			t.Errorf("K2 should be skipped (non-string value)")
		}
	})
}

func TestResolveEnvPullTarget_RejectsControlChars(t *testing.T) {
	if _, _, err := resolveEnvPullTarget("bad\x01path"); err == nil {
		t.Error("control char in --project-path must be rejected")
	}
}

func TestReadEnvPullFile_ReadErrorOnDirectory(t *testing.T) {
	// Reading a directory as a file is a non-ENOENT error path.
	if _, err := readEnvPullFile(t.TempDir()); err == nil {
		t.Error("reading a directory as env file must surface an error")
	}
}

func TestEnsureEnvPullParentDir_MkdirError(t *testing.T) {
	// A file occupying the would-be parent component makes MkdirAll fail.
	base := t.TempDir()
	blocker := filepath.Join(base, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureEnvPullParentDir(filepath.Join(blocker, "child", ".env.local")); err == nil {
		t.Error("MkdirAll over a file component must surface an error")
	}
}

// TestExtractEnvPullVars_MissingEnvVarsIsInternalInvalidResponse pins that a
// response without a usable env_vars field classifies as
// internal/invalid_response — a broken upstream payload, not a flag problem
// the agent should retry with different arguments.
func TestExtractEnvPullVars_MissingEnvVarsIsInternalInvalidResponse(t *testing.T) {
	for name, data := range map[string]map[string]interface{}{
		"missing":    {},
		"wrong type": {"env_vars": "not-an-object"},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, _, err := extractEnvPullVars(data)
			if err == nil {
				t.Fatalf("expected error for %s env_vars", name)
			}
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("expected typed problem, got %T: %v", err, err)
			}
			if p.Category != errs.CategoryInternal || p.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("classification = %s/%s, want internal/invalid_response", p.Category, p.Subtype)
			}
		})
	}
}
