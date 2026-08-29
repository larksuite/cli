// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

// --- pure-function tests ---

func TestAppDevBuildEnv(t *testing.T) {
	kvm := map[string]string{
		"upload_url":                 "https://tos/put",
		"tos_path":                   "x/y.zip",
		"MIAODA_CLIENT_BASE_PATH":    "/app/x",
		"MIAODA_RESOURCE_CDN_PREFIX": "https://lf.example",
		"miaoda_lowercase":           "must-not-inject",
		"NODE_OPTIONS":               "--require evil",
		"MIAODA_BAD=KEY":             "reject-equals",
		"MIAODA_BAD\nKEY":            "reject-newline",
		"MIAODA_BAD\rKEY":            "reject-cr",
	}
	env, keys := appDevBuildEnv(kvm)
	wantEnv := []string{
		"MIAODA_CLIENT_BASE_PATH=/app/x",
		"MIAODA_RESOURCE_CDN_PREFIX=https://lf.example",
	}
	wantKeys := []string{"MIAODA_CLIENT_BASE_PATH", "MIAODA_RESOURCE_CDN_PREFIX"}
	if !reflect.DeepEqual(env, wantEnv) || !reflect.DeepEqual(keys, wantKeys) {
		t.Errorf("appDevBuildEnv = (%v, %v), want (%v, %v)", env, keys, wantEnv, wantKeys)
	}
	if env, keys := appDevBuildEnv(nil); len(env) != 0 || len(keys) != 0 {
		t.Errorf("nil kvm should yield empty results, got (%v, %v)", env, keys)
	}
}

// --- artifact layout validation ---

// testAppDevCfg builds a resolved project config for validation tests.
// buildless mirrors a spark.json without build.command.
func testAppDevCfg(output, cdn string, buildless bool) *appDevProjectConfig {
	cfg := &appDevProjectConfig{BuildOutput: output, BuildOutputCDN: cdn}
	if !buildless {
		cfg.BuildCommand = []string{"npm", "run", "build"}
	}
	return cfg
}

// writeDistFiles creates files (relative to base) with parent dirs. A file
// named routes.json gets valid route-enumeration content so protocol
// validation passes by default; tests that need a broken one overwrite it
// afterwards.
func writeDistFiles(t *testing.T, base string, files []string) {
	t.Helper()
	for _, f := range files {
		p := filepath.Join(base, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		body := "x"
		if strings.HasSuffix(f, "routes.json") {
			body = `[{"path":"/","file":"index.html"}]`
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestValidateAppDevOutputs(t *testing.T) {
	tests := []struct {
		name      string
		files     []string
		buildless bool
		wantErr   string // "" = valid
	}{
		{"ok minimal", []string{"index.html", "routes.json"}, false, ""},
		{"ok non-index html", []string{"page.html", "routes.json"}, false, ""},
		{"ok extra files ride along", []string{"index.html", "routes.json", "assets/logo.png", "manifest.json"}, false, ""},
		{"ok buildless with routes", []string{"index.html", "routes.json"}, true, ""},
		{"ok buildless generates routes", []string{"index.html"}, true, ""},
		{"no html", []string{"routes.json"}, false, "no .html file"},
		{"no routes with build command", []string{"index.html"}, false, "routes.json is missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "dist", "output")
			writeDistFiles(t, out, tt.files)
			entries, gen, err := validateAppDevOutputs(permissiveFIO{}, testAppDevCfg(out, "", tt.buildless), false)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("want valid, got %v", err)
			}
			// Everything in the artifact directory is uploaded, normalized
			// under the fixed zip prefix.
			hasRoutes := false
			for _, e := range entries {
				if !strings.HasPrefix(e.ZipPath, "output/") {
					t.Errorf("zip path %q must be normalized under output/", e.ZipPath)
				}
				if e.ZipPath == "output/routes.json" {
					hasRoutes = true
				}
			}
			if len(entries) < len(tt.files) {
				t.Errorf("entries = %d, want at least %d (all files upload)", len(entries), len(tt.files))
			}
			if !hasRoutes {
				t.Error("payload must always carry output/routes.json (shipped or generated)")
			}
			wantGen := tt.buildless && !strings.Contains(strings.Join(tt.files, " "), "routes.json")
			if (gen >= 0) != wantGen {
				t.Errorf("generatedRoutes = %d, wantGenerated=%v", gen, wantGen)
			}
		})
	}
}

func TestValidateAppDevOutputs_CDNSplit(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "dist", "output")
	cdn := filepath.Join(root, "dist", "output_resource")
	writeDistFiles(t, out, []string{"index.html", "routes.json"})
	writeDistFiles(t, cdn, []string{"static/a.js"})
	entries, _, err := validateAppDevOutputs(permissiveFIO{}, testAppDevCfg(out, cdn, false), false)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, e := range entries {
		got[e.ZipPath] = true
	}
	for _, want := range []string{"output/index.html", "output/routes.json", "output_resource/static/a.js"} {
		if !got[want] {
			t.Errorf("missing normalized entry %q in %v", want, got)
		}
	}
	// A declared but missing CDN directory is a hard error, not silence.
	_, _, err = validateAppDevOutputs(permissiveFIO{}, testAppDevCfg(out, filepath.Join(root, "nope"), false), false)
	if err == nil || !strings.Contains(err.Error(), "CDN artifact directory") {
		t.Errorf("missing declared cdn dir must fail, got %v", err)
	}
}

func TestGenerateAppDevRoutes(t *testing.T) {
	b, n, err := generateAppDevRoutes([]string{"index.html", "foo/index.html", "bar.html", "dup.html", "dup/index.html"})
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Errorf("count = %d, want 4 (dup path deduped)", n)
	}
	var routes []map[string]string
	if err := json.Unmarshal(b, &routes); err != nil {
		t.Fatalf("generated routes.json not valid JSON: %v", err)
	}
	got := map[string]string{}
	for _, r := range routes {
		got[r["path"]] = r["file"]
	}
	if got["/"] != "index.html" || got["/foo"] != "foo/index.html" || got["/bar"] != "bar.html" {
		t.Errorf("routes = %v", got)
	}
	// The generated payload must pass the same schema check shipped files do.
	if err := validateAppDevRoutesJSON(b); err != nil {
		t.Errorf("generated routes.json fails schema: %v", err)
	}
}

func TestValidateAppDevOutputs_RoutesSchema(t *testing.T) {
	out := filepath.Join(t.TempDir(), "dist", "output")
	writeDistFiles(t, out, []string{"index.html", "routes.json"})
	set := func(body string) {
		os.WriteFile(filepath.Join(out, "routes.json"), []byte(body), 0o644)
	}
	check := func(body, wantErr string) {
		t.Helper()
		set(body)
		_, _, err := validateAppDevOutputs(permissiveFIO{}, testAppDevCfg(out, "", false), false)
		if wantErr == "" {
			if err != nil {
				t.Errorf("routes %q should be valid: %v", body, err)
			}
			return
		}
		if err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("routes %q: err = %v, want containing %q", body, err, wantErr)
		}
	}
	check("not-json", "not a valid route enumeration array")
	// Old fallback-only object form is no longer the schema.
	check(`{"version":1,"type":"t","fallback":"index.html"}`, "not a valid route enumeration array")
	check(`[{"path":""}]`, "invalid path")
	check(`[{"path":"orders"}]`, "invalid path")
	check(`[{"path":"/"},{"path":"/"}]`, "duplicate path")
	check(`[]`, "")                                                        // 纯静态站可为空数组
	check(`[{"path":"/orders/:id"}]`, "")                                  // 动态段合法
	check(`[{"path":"/","file":"index.html","name":"首页","future":1}]`, "") // 未识别字段忽略
}

func TestValidateAppDevOutputs_Missing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "dist", "output")
	_, _, err := validateAppDevOutputs(permissiveFIO{}, testAppDevCfg(missing, "", false), false)
	p := requireAppsProblem(t, err, errs.CategoryValidation)
	if p.Subtype != errs.SubtypeFailedPrecondition {
		t.Errorf("subtype = %q, want failed_precondition", p.Subtype)
	}
	if !strings.Contains(p.Hint, "--skip-build") {
		t.Errorf("hint = %q", p.Hint)
	}
	// Buildless projects get buildless-specific guidance, not a build hint.
	_, _, err = validateAppDevOutputs(permissiveFIO{}, testAppDevCfg(missing, "", true), false)
	p = requireAppsProblem(t, err, errs.CategoryValidation)
	if !strings.Contains(p.Hint, "no build.command") {
		t.Errorf("buildless hint = %q", p.Hint)
	}
}

func TestValidateAppDevOutputs_Sensitive(t *testing.T) {
	out := filepath.Join(t.TempDir(), "dist", "output")
	writeDistFiles(t, out, []string{"index.html", "routes.json", ".env"})
	_, _, err := validateAppDevOutputs(permissiveFIO{}, testAppDevCfg(out, "", false), false)
	if err == nil || !strings.Contains(err.Error(), "credential file") {
		t.Errorf("sensitive file must be rejected, got %v", err)
	}
	if _, _, err := validateAppDevOutputs(permissiveFIO{}, testAppDevCfg(out, "", false), true); err != nil {
		t.Errorf("allow-sensitive must waive the scan: %v", err)
	}
}

// --- zip packing ---

func TestBuildAppDevZip(t *testing.T) {
	root := t.TempDir()
	out, cdn := filepath.Join(root, "dist", "output"), filepath.Join(root, "dist", "output_resource")
	writeDistFiles(t, out, []string{"index.html", "routes.json"})
	writeDistFiles(t, cdn, []string{"a.js"})
	entries, _, err := validateAppDevOutputs(permissiveFIO{}, testAppDevCfg(out, cdn, false), false)
	if err != nil {
		t.Fatal(err)
	}
	zipball, err := buildAppDevZip(permissiveFIO{}, entries)
	if err != nil {
		t.Fatal(err)
	}
	if zipball.FileCount != 3 || zipball.Size != int64(len(zipball.Body)) {
		t.Errorf("FileCount=%d Size=%d len(Body)=%d", zipball.FileCount, zipball.Size, len(zipball.Body))
	}
	names := zipEntryNames(t, zipball.Body)
	want := map[string]bool{"output/index.html": true, "output/routes.json": true, "output_resource/a.js": true}
	if len(names) != len(want) {
		t.Fatalf("entries = %v", names)
	}
	for _, n := range names {
		if !want[n] {
			t.Errorf("unexpected zip entry %q (project dir names must be normalized away)", n)
		}
	}
}

func TestBuildAppDevZip_InlineGeneratedRoutes(t *testing.T) {
	out := filepath.Join(t.TempDir(), "dist", "output")
	writeDistFiles(t, out, []string{"index.html"})
	entries, gen, err := validateAppDevOutputs(permissiveFIO{}, testAppDevCfg(out, "", true), false)
	if err != nil {
		t.Fatal(err)
	}
	if gen != 1 {
		t.Fatalf("generatedRoutes = %d, want 1", gen)
	}
	zipball, err := buildAppDevZip(permissiveFIO{}, entries)
	if err != nil {
		t.Fatal(err)
	}
	names := zipEntryNames(t, zipball.Body)
	found := false
	for _, n := range names {
		if n == "output/routes.json" {
			found = true
		}
	}
	if !found {
		t.Errorf("generated routes.json missing from zip: %v", names)
	}
}

func TestBuildAppDevZip_RawSizeCap(t *testing.T) {
	orig := maxAppDevPublishRawBytes
	maxAppDevPublishRawBytes = 1
	t.Cleanup(func() { maxAppDevPublishRawBytes = orig })
	out := filepath.Join(t.TempDir(), "dist", "output")
	writeDistFiles(t, out, []string{"index.html", "routes.json"})
	entries, _, err := validateAppDevOutputs(permissiveFIO{}, testAppDevCfg(out, "", false), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buildAppDevZip(permissiveFIO{}, entries); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("raw cap must reject, got %v", err)
	}
}

func TestBuildAppDevZip_ZipSizeCap(t *testing.T) {
	orig := maxAppDevPublishZipBytes
	maxAppDevPublishZipBytes = 1
	t.Cleanup(func() { maxAppDevPublishZipBytes = orig })
	out := filepath.Join(t.TempDir(), "dist", "output")
	writeDistFiles(t, out, []string{"index.html", "routes.json"})
	entries, _, err := validateAppDevOutputs(permissiveFIO{}, testAppDevCfg(out, "", false), false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = buildAppDevZip(permissiveFIO{}, entries)
	if err == nil || !strings.Contains(err.Error(), "packed zip size") {
		t.Errorf("zip cap must reject, got %v", err)
	}
	p, _ := errs.ProblemOf(err)
	if p == nil || !strings.Contains(p.Hint, "reduce the artifact directory contents") {
		t.Errorf("hint = %v", p)
	}
}

// --- shortcut orchestration ---

// fakeEnvRunner records the build invocation and optionally materializes dist
// as a side effect (simulating npm run build).
type fakeEnvRunner struct {
	called     bool
	dir, name  string
	args, env  []string
	stderr     string
	err        error
	sideEffect func()
}

func (f *fakeEnvRunner) RunEnv(ctx context.Context, dir string, extraEnv []string, name string, args ...string) (string, string, error) {
	f.called = true
	f.dir, f.name, f.args, f.env = dir, name, args, extraEnv
	if f.sideEffect != nil {
		f.sideEffect()
	}
	return "", f.stderr, f.err
}

func withFakeEnvRunner(t *testing.T, f *fakeEnvRunner) {
	t.Helper()
	orig := appDevRunner
	appDevRunner = f
	t.Cleanup(func() { appDevRunner = orig })
}

// chdirSparkProjectRoot creates a temp project root with spark.json and
// chdirs into it (the protocol-first path).
func chdirSparkProjectRoot(t *testing.T, miaodaJSON string) string {
	t.Helper()
	root := t.TempDir()
	if miaodaJSON != "" {
		if err := os.WriteFile(filepath.Join(root, sparkJSONRelPath), []byte(miaodaJSON), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })
	return root
}

// newTOSTLSServer starts a TLS server for the presigned PUT and swaps
// appDevNewTransferClient to trust its certificate.
func newTOSTLSServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)
	orig := appDevNewTransferClient
	appDevNewTransferClient = func() *http.Client { return srv.Client() }
	t.Cleanup(func() { appDevNewTransferClient = orig })
	return srv
}

func stubPreRelease(reg *httpmock.Registry, appID, uploadURL string, extraKVs map[string]string) {
	kvs := []interface{}{
		map[string]interface{}{"key": "artifact_url", "value": uploadURL},
	}
	for k, v := range extraKVs {
		kvs = append(kvs, map[string]interface{}{"key": k, "value": v})
	}
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/" + appID + "/pre_release",
		Body: map[string]interface{}{
			"code": float64(0),
			"data": map[string]interface{}{"kvs": kvs},
		},
	})
}

func stubReleaseGet(reg *httpmock.Registry, appID, releaseID string, respData map[string]interface{}) {
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/" + appID + "/releases/" + releaseID,
		Body: map[string]interface{}{
			"code": float64(0),
			"data": respData,
		},
	})
}

func stubReleases(reg *httpmock.Registry, appID string, respData map[string]interface{}) {
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/" + appID + "/releases",
		Body: map[string]interface{}{
			"code": float64(0),
			"data": respData,
		},
	})
}

func TestAppDevPublishValidate_NoMeta(t *testing.T) {
	chdirSparkProjectRoot(t, "")
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsDeploy, []string{"+deploy", "--as", "user"}, factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryValidation)
	if p.Subtype != errs.SubtypeFailedPrecondition || !strings.Contains(p.Message, "not a Miaoda app project") {
		t.Errorf("got %v", p)
	}
	if !strings.Contains(p.Message, "spark.json") {
		t.Errorf("message should name spark.json, got %q", p.Message)
	}
	if !strings.Contains(p.Hint, "+init-template") {
		t.Errorf("hint = %q", p.Hint)
	}
}

func TestAppDevPublishValidate_NoAppID(t *testing.T) {
	chdirSparkProjectRoot(t, `{"stack":"react-standard-webapp","dev":{"port":5173}}`)
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsDeploy, []string{"+deploy", "--as", "user"}, factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryValidation)
	if !strings.Contains(p.Message, "no publish target") {
		t.Errorf("message = %q", p.Message)
	}
	// The guidance must lead to +create and the new --app-id flow (no manual
	// JSON editing).
	if !strings.Contains(p.Hint, "+create") || !strings.Contains(p.Hint, "+deploy --app-id") {
		t.Errorf("hint = %q", p.Hint)
	}
}

func TestAppDevPublishValidate_AppIDMismatch(t *testing.T) {
	chdirSparkProjectRoot(t, `{"app":{"id":"app_recorded"}}`)
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsDeploy,
		[]string{"+deploy", "--app-id", "app_other", "--as", "user"}, factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryValidation)
	if !strings.Contains(p.Message, "app_recorded") || !strings.Contains(p.Message, "app_other") {
		t.Errorf("message must name both ids, got %q", p.Message)
	}
	if !strings.Contains(p.Hint, "drop --app-id") {
		t.Errorf("hint = %q", p.Hint)
	}
}

func TestAppDevPublishExecute_FlagMatchesMeta(t *testing.T) {
	root := chdirSparkProjectRoot(t, `{"stack":"custom-webapp","dev":{"port":5173},"app":{"id":"app_x"}}`)
	writeDistFiles(t, filepath.Join(root, "dist"), []string{"output/index.html", "output/routes.json"})
	srv := newTOSTLSServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	factory, stdout, reg := newAppsExecuteFactory(t)
	stubPreRelease(reg, "app_x", srv.URL, nil)
	stubReleases(reg, "app_x", map[string]interface{}{"release_id": "rel_10", "status": "pending"})
	if err := runAppsShortcut(t, AppsDeploy,
		[]string{"+deploy", "--app-id", "app_x", "--skip-build", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("matching --app-id must publish fine: %v", err)
	}
}

func TestAppDevPublishValidate_BadAppID(t *testing.T) {
	chdirSparkProjectRoot(t, `{"app":{"id":"meta_token_x"}}`)
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsDeploy, []string{"+deploy", "--as", "user"}, factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryValidation)
	if !strings.Contains(p.Message, "spark.json app id") {
		t.Errorf("message should point at the config source, got %q", p.Message)
	}
	// This command has no --app-id flag; the error must not mention one.
	if strings.Contains(p.Message, "--app-id") || strings.Contains(p.Hint, "--app-id") {
		t.Errorf("error must not reference a nonexistent --app-id flag: %v", p)
	}
	if !strings.Contains(p.Hint, "+list") {
		t.Errorf("hint = %q", p.Hint)
	}
}

func TestAppDevPublishValidate_Declaration(t *testing.T) {
	// The hosting entry enforces the protocol's declaration-side MUSTs:
	// stack (with a supported hosting-shape suffix) and dev.port (the
	// platform relies on the local self-description endpoint after hosting).
	cases := []struct {
		name, sparkJSON, wantErr string
	}{
		{"missing stack", `{"dev":{"port":5173},"app":{"id":"app_x"}}`, "missing the required stack"},
		{"bad stack charset", `{"stack":"My Stack","dev":{"port":5173},"app":{"id":"app_x"}}`, "is invalid"},
		{"bad stack suffix", `{"stack":"react-standard","dev":{"port":5173},"app":{"id":"app_x"}}`, "must end with -webapp or -fullstack"},
		{"missing dev.port", `{"stack":"custom-webapp","app":{"id":"app_x"}}`, "missing the required dev.port"},
		{"port out of range", `{"stack":"custom-webapp","dev":{"port":70000},"app":{"id":"app_x"}}`, "out of range"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chdirSparkProjectRoot(t, tc.sparkJSON)
			factory, stdout, _ := newAppsExecuteFactory(t)
			err := runAppsShortcut(t, AppsDeploy, []string{"+deploy", "--as", "user"}, factory, stdout)
			p := requireAppsProblem(t, err, errs.CategoryValidation)
			if p.Subtype != errs.SubtypeFailedPrecondition || !strings.Contains(p.Message, tc.wantErr) {
				t.Errorf("got %v, want message containing %q", p, tc.wantErr)
			}
		})
	}
}

func TestAppDevPublishExecute_MissingIndexHTMLWarns(t *testing.T) {
	// A payload without index.html publishes (warning only, per the protocol
	// owner's call) — the platform's SPA fallback depends on it, so the
	// warning must be loud but non-blocking.
	root := chdirSparkProjectRoot(t, `{"stack":"custom-webapp","dev":{"port":5173},"app":{"id":"app_x"}}`)
	writeDistFiles(t, filepath.Join(root, "dist"), []string{"output/page.html", "output/routes.json"})
	srv := newTOSTLSServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	factory, stdout, reg := newAppsExecuteFactory(t)
	stubPreRelease(reg, "app_x", srv.URL, nil)
	stubReleases(reg, "app_x", map[string]interface{}{"release_id": "rel_60", "status": "finished", "online_url": "https://x/app/app_x"})
	if err := runAppsShortcut(t, AppsDeploy, []string{"+deploy", "--skip-build", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("missing index.html must not block the deploy: %v", err)
	}
}

func TestAppDevPublishValidate_SensitiveGatesDryRun(t *testing.T) {
	root := chdirSparkProjectRoot(t, `{"stack":"custom-webapp","dev":{"port":5173},"app":{"id":"app_x"}}`)
	writeDistFiles(t, filepath.Join(root, "dist"),
		[]string{"output/index.html", "output/routes.json", "output/.env"})
	factory, stdout, _ := newAppsExecuteFactory(t)
	// Sensitive hits are the one exception to dry-run's exit-0 convention:
	// Validate rejects before the DryRun branch runs.
	err := runAppsShortcut(t, AppsDeploy,
		[]string{"+deploy", "--skip-build", "--as", "user", "--dry-run"}, factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryValidation)
	if !strings.Contains(p.Message, "publish payload contains") || !strings.Contains(p.Message, "credential file") {
		t.Errorf("message = %q", p.Message)
	}
	// This command has no --path flag; the error must not mention one.
	if strings.Contains(p.Message, "--path") {
		t.Errorf("error must not reference a nonexistent --path flag: %q", p.Message)
	}
	// --allow-sensitive waives the gate and dry-run goes back to exit 0.
	if err := runAppsShortcut(t, AppsDeploy,
		[]string{"+deploy", "--skip-build", "--allow-sensitive", "--as", "user", "--dry-run"}, factory, stdout); err != nil {
		t.Errorf("allow-sensitive dry-run should pass: %v", err)
	}
}

func TestAppDevPublishValidate_SkipBuildNoDist(t *testing.T) {
	chdirSparkProjectRoot(t, `{"stack":"custom-webapp","dev":{"port":5173},"app":{"id":"app_x"},"build":{"command":["npm","run","build"]}}`)
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsDeploy, []string{"+deploy", "--skip-build", "--as", "user"}, factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryValidation)
	if p.Subtype != errs.SubtypeFailedPrecondition || !strings.Contains(p.Message, "--skip-build is set but the artifact directory dist/output does not exist") {
		t.Errorf("got %v", p)
	}
}

func TestAppDevPublishValidate_BuildlessNoDist(t *testing.T) {
	// No build.command declared in spark.json (buildless): the artifact
	// directory must already exist.
	chdirSparkProjectRoot(t, `{"stack":"custom-webapp","dev":{"port":5173},"app":{"id":"app_x"}}`)
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsDeploy, []string{"+deploy", "--as", "user"}, factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryValidation)
	if p.Subtype != errs.SubtypeFailedPrecondition || !strings.Contains(p.Message, "artifact directory dist/output does not exist") {
		t.Errorf("got %v", p)
	}
	if !strings.Contains(p.Hint, "no build.command") {
		t.Errorf("hint = %q", p.Hint)
	}
}

func TestAppDevPublishExecute_SyncSuccess(t *testing.T) {
	root := chdirSparkProjectRoot(t, `{
  "stack": "react-standard-webapp",
  "dev": { "port": 5173 },
  "build": { "command": ["npm", "run", "build"], "output": "dist/output" },
  "app": { "id": "app_x" }
}`)
	var uploaded []byte
	var contentType string
	srv := newTOSTLSServer(t, func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		uploaded = buf
		w.WriteHeader(200)
	})
	f := &fakeEnvRunner{sideEffect: func() {
		writeDistFiles(t, filepath.Join(root, "dist", "output"), []string{"index.html", "routes.json", "a.js"})
	}}
	withFakeEnvRunner(t, f)
	factory, stdout, reg := newAppsExecuteFactory(t)
	stubPreRelease(reg, "app_x", srv.URL, map[string]string{
		"MIAODA_CLIENT_BASE_PATH": "/app/app_x",
		"NODE_OPTIONS":            "--require evil",
	})
	stubReleases(reg, "app_x", map[string]interface{}{
		"release_id": "rel_1", "status": "finished",
		"online_url": "https://x.feishuapp.cn/app/app_x",
	})
	if err := runAppsShortcut(t, AppsDeploy, []string{"+deploy", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	// Build invocation contract.
	if !f.called || f.name != "npm" || !reflect.DeepEqual(f.args, []string{"run", "build"}) {
		t.Errorf("build call = %v %v (called=%v)", f.name, f.args, f.called)
	}
	if !reflect.DeepEqual(f.env, []string{"MIAODA_CLIENT_BASE_PATH=/app/app_x"}) {
		t.Errorf("injected env = %v (NODE_OPTIONS must be filtered)", f.env)
	}
	// Upload contract.
	if contentType != "application/zip" {
		t.Errorf("Content-Type = %q", contentType)
	}
	if len(uploaded) == 0 {
		t.Error("zip body not uploaded")
	}
	// Output contract.
	data := parseEnvelopeData(t, stdout)
	if data["online_url"] != "https://x.feishuapp.cn/app/app_x" || data["release_id"] != "rel_1" {
		t.Errorf("data = %v", data)
	}
	if data["built"] != true {
		t.Errorf("built = %v", data["built"])
	}
	if _, hasPoll := data["poll_hint"]; hasPoll {
		t.Error("sync success must not carry poll_hint")
	}
	// spark.json app-section writeback.
	b, _ := os.ReadFile(filepath.Join(root, sparkJSONRelPath))
	var doc map[string]interface{}
	_ = json.Unmarshal(b, &doc)
	app, _ := doc["app"].(map[string]interface{})
	if app == nil || app["id"] != "app_x" || app["online_url"] != "https://x.feishuapp.cn/app/app_x" {
		t.Errorf("app section after publish = %v", doc["app"])
	}
}

func TestAppDevPublishExecute_BuildlessSync(t *testing.T) {
	// spark.json without build.command: buildless — no build runs,
	// dist/output is packed as-is, the app section gains the url.
	root := chdirSparkProjectRoot(t, `{"stack":"custom-webapp","dev":{"port":5173},"app":{"id":"app_x"}}`)
	writeDistFiles(t, filepath.Join(root, "dist"), []string{"output/index.html", "output/routes.json"})
	f := &fakeEnvRunner{}
	withFakeEnvRunner(t, f)
	srv := newTOSTLSServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	factory, stdout, reg := newAppsExecuteFactory(t)
	stubPreRelease(reg, "app_x", srv.URL, nil)
	stubReleases(reg, "app_x", map[string]interface{}{
		"release_id": "rel_30", "status": "finished",
		"online_url": "https://x/app/app_x",
	})
	if err := runAppsShortcut(t, AppsDeploy, []string{"+deploy", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if f.called {
		t.Error("buildless project must never invoke a build command")
	}
	data := parseEnvelopeData(t, stdout)
	if data["built"] != false {
		t.Errorf("built = %v, want false for buildless", data["built"])
	}
	b, _ := os.ReadFile(filepath.Join(root, sparkJSONRelPath))
	var doc map[string]interface{}
	_ = json.Unmarshal(b, &doc)
	app, _ := doc["app"].(map[string]interface{})
	if app == nil || app["online_url"] != "https://x/app/app_x" {
		t.Errorf("app section after publish = %v", doc["app"])
	}
}

func TestAppDevPublishExecute_AsyncSuccess(t *testing.T) {
	root := chdirSparkProjectRoot(t, `{"stack":"custom-webapp","dev":{"port":5173},"app":{"id":"app_x"}}`)
	writeDistFiles(t, filepath.Join(root, "dist"), []string{"output/index.html", "output/routes.json"})
	srv := newTOSTLSServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	factory, stdout, reg := newAppsExecuteFactory(t)
	stubPreRelease(reg, "app_x", srv.URL, nil)
	stubReleases(reg, "app_x", map[string]interface{}{"release_id": "rel_2", "status": "pending"})
	// An in-flight release returns immediately with the poll hint — the
	// command never blocks on polling (agent runtimes own the wait).
	if err := runAppsShortcut(t, AppsDeploy, []string{"+deploy", "--skip-build", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	data := parseEnvelopeData(t, stdout)
	if data["built"] != false {
		t.Errorf("built = %v, want false with --skip-build", data["built"])
	}
	hint, _ := data["poll_hint"].(string)
	if !strings.Contains(hint, "+release-get --app-id app_x --release-id rel_2") {
		t.Errorf("poll_hint = %q", hint)
	}
	if _, has := data["online_url"]; has {
		t.Error("async must not carry online_url")
	}
	// No online_url -> the app section carries no url key.
	b, _ := os.ReadFile(filepath.Join(root, sparkJSONRelPath))
	if strings.Contains(string(b), "\"online_url\"") {
		t.Errorf("spark.json must not gain app.online_url on a still-publishing release: %s", b)
	}
}

func TestAppDevPublishExecute_FinishedWithoutURL(t *testing.T) {
	// The create response may report finished without online_url; the
	// command must fetch the release once to recover the url instead of
	// returning an empty one with a misleading poll hint.
	root := chdirSparkProjectRoot(t, `{"stack":"custom-webapp","dev":{"port":5173},"app":{"id":"app_x"}}`)
	writeDistFiles(t, filepath.Join(root, appDevDefaultBuildOutput), []string{"index.html", "routes.json"})
	srv := newTOSTLSServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	factory, stdout, reg := newAppsExecuteFactory(t)
	stubPreRelease(reg, "app_x", srv.URL, nil)
	stubReleases(reg, "app_x", map[string]interface{}{"release_id": "rel_51", "status": "finished"})
	stubReleaseGet(reg, "app_x", "rel_51", map[string]interface{}{
		"release_id": "rel_51", "status": "finished",
		"online_url": "https://x/app/app_x",
	})
	if err := runAppsShortcut(t, AppsDeploy, []string{"+deploy", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	data := parseEnvelopeData(t, stdout)
	if data["online_url"] != "https://x/app/app_x" {
		t.Errorf("online_url must be recovered via release-get, got %v", data["online_url"])
	}
	if _, has := data["poll_hint"]; has {
		t.Error("recovered finish must not carry poll_hint")
	}
}

func TestAppDevPublishExecute_CreateReportsFailed(t *testing.T) {
	// A create response that already reports failed is a failed publish:
	// exit non-zero with the error_logs (fetched once) summarized.
	root := chdirSparkProjectRoot(t, `{"stack":"custom-webapp","dev":{"port":5173},"app":{"id":"app_x"}}`)
	writeDistFiles(t, filepath.Join(root, "dist"), []string{"output/index.html", "output/routes.json"})
	srv := newTOSTLSServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	factory, stdout, reg := newAppsExecuteFactory(t)
	stubPreRelease(reg, "app_x", srv.URL, nil)
	stubReleases(reg, "app_x", map[string]interface{}{"release_id": "rel_41", "status": "failed"})
	stubReleaseGet(reg, "app_x", "rel_41", map[string]interface{}{
		"release_id": "rel_41", "status": "failed",
		"error_logs": []interface{}{
			map[string]interface{}{"step": "build", "error_log": "formula output is empty"},
		},
	})
	err := runAppsShortcut(t, AppsDeploy, []string{"+deploy", "--skip-build", "--as", "user"}, factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryInternal)
	if !strings.Contains(p.Message, "release rel_41 failed") || !strings.Contains(p.Message, "[build] formula output is empty") {
		t.Errorf("message = %q", p.Message)
	}
	if !strings.Contains(p.Hint, "+release-get --app-id app_x --release-id rel_41") {
		t.Errorf("hint = %q", p.Hint)
	}
}

func TestSummarizeReleaseErrorLogs(t *testing.T) {
	if got := summarizeReleaseErrorLogs(nil); got != "" {
		t.Errorf("nil logs = %q", got)
	}
	logs := []interface{}{
		map[string]interface{}{"step": "build", "error_log": "a"},
		map[string]interface{}{"error_log": "b"},
		"garbage",
	}
	if got := summarizeReleaseErrorLogs(logs); got != "[build] a; b" {
		t.Errorf("summary = %q", got)
	}
}

func TestAppDevPublishExecute_BuildFails(t *testing.T) {
	chdirSparkProjectRoot(t, `{"stack":"custom-webapp","dev":{"port":5173},"app":{"id":"app_x"},"build":{"command":["npm","run","build"]}}`)
	srv := newTOSTLSServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	f := &fakeEnvRunner{stderr: "TS2304: boom", err: errors.New("exit 1")}
	withFakeEnvRunner(t, f)
	factory, stdout, reg := newAppsExecuteFactory(t)
	stubPreRelease(reg, "app_x", srv.URL, nil)
	err := runAppsShortcut(t, AppsDeploy, []string{"+deploy", "--as", "user"}, factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryInternal)
	if !strings.Contains(p.Message, `build command "npm run build" failed`) || !strings.Contains(p.Message, "TS2304") {
		t.Errorf("message = %q", p.Message)
	}
	if !strings.Contains(p.Hint, "--skip-build") {
		t.Errorf("hint = %q", p.Hint)
	}
}

func TestAppDevPublishExecute_PreReleaseMissingKVs(t *testing.T) {
	root := chdirSparkProjectRoot(t, `{"stack":"custom-webapp","dev":{"port":5173},"app":{"id":"app_x"}}`)
	writeDistFiles(t, filepath.Join(root, "dist"), []string{"output/index.html", "output/routes.json"})
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/pre_release",
		Body: map[string]interface{}{
			"code": float64(0),
			"data": map[string]interface{}{"kvs": []interface{}{}},
		},
	})
	err := runAppsShortcut(t, AppsDeploy, []string{"+deploy", "--skip-build", "--as", "user"}, factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryInternal)
	if !strings.Contains(p.Message, "missing artifact_url") {
		t.Errorf("message = %q", p.Message)
	}
}

func TestAppDevPublishExecute_NonHTTPSUploadURL(t *testing.T) {
	root := chdirSparkProjectRoot(t, `{"stack":"custom-webapp","dev":{"port":5173},"app":{"id":"app_x"}}`)
	writeDistFiles(t, filepath.Join(root, "dist"), []string{"output/index.html", "output/routes.json"})
	factory, stdout, reg := newAppsExecuteFactory(t)
	stubPreRelease(reg, "app_x", "http://insecure.example/put", nil)
	err := runAppsShortcut(t, AppsDeploy, []string{"+deploy", "--skip-build", "--as", "user"}, factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryInternal)
	if !strings.Contains(p.Message, "not https") {
		t.Errorf("message = %q", p.Message)
	}
}

func TestAppDevPublishExecute_TOS5xx(t *testing.T) {
	root := chdirSparkProjectRoot(t, `{"stack":"custom-webapp","dev":{"port":5173},"app":{"id":"app_x"}}`)
	writeDistFiles(t, filepath.Join(root, "dist"), []string{"output/index.html", "output/routes.json"})
	srv := newTOSTLSServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) })
	factory, stdout, reg := newAppsExecuteFactory(t)
	stubPreRelease(reg, "app_x", srv.URL, nil)
	err := runAppsShortcut(t, AppsDeploy, []string{"+deploy", "--skip-build", "--as", "user"}, factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryNetwork)
	if !p.Retryable {
		t.Error("5xx upload failure must be retryable")
	}
}

func TestAppDevPublishDryRun(t *testing.T) {
	root := chdirSparkProjectRoot(t, `{"stack":"custom-webapp","dev":{"port":5173},"app":{"id":"app_x"},"build":{"command":["npm","run","build"],"output":"dist/output"}}`)
	writeDistFiles(t, filepath.Join(root, "dist", "output"), []string{"index.html"})
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDeploy, []string{"+deploy", "--skip-build", "--as", "user", "--dry-run"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	data, err := decodeDryRunDataMap(stdout.Bytes())
	if err != nil {
		t.Fatalf("decode dry-run: %v (raw=%q)", err, stdout.String())
	}
	if data["app_id"] != "app_x" {
		t.Errorf("app_id = %v", data["app_id"])
	}
	// A declared build.command is expected to produce routes.json — its
	// absence surfaces as a validation error (never CLI-generated here).
	if verr, _ := data["output_validation_error"].(string); !strings.Contains(verr, "routes.json") {
		t.Errorf("output_validation_error = %v (routes.json missing should surface)", data["output_validation_error"])
	}
	buildCmd, _ := data["build_command"].(string)
	if !strings.Contains(buildCmd, "MIAODA_*") {
		t.Errorf("build_command = %q", buildCmd)
	}
}

func TestAppDevPublishDryRun_Buildless(t *testing.T) {
	root := chdirSparkProjectRoot(t, `{"stack":"custom-webapp","dev":{"port":5173},"app":{"id":"app_x"}}`)
	writeDistFiles(t, filepath.Join(root, "dist"), []string{"output/index.html"})
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDeploy, []string{"+deploy", "--as", "user", "--dry-run"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	data, err := decodeDryRunDataMap(stdout.Bytes())
	if err != nil {
		t.Fatalf("decode dry-run: %v (raw=%q)", err, stdout.String())
	}
	buildCmd, _ := data["build_command"].(string)
	if !strings.Contains(buildCmd, "buildless") {
		t.Errorf("build_command = %q, want buildless note", buildCmd)
	}
	// Missing routes.json is fine for buildless — the CLI generates it.
	if verr, has := data["output_validation_error"]; has {
		t.Errorf("output_validation_error = %v, want none", verr)
	}
	routes, _ := data["routes_json"].(string)
	if !strings.Contains(routes, "generated") {
		t.Errorf("routes_json = %q, want generation note", routes)
	}
	cdn, _ := data["build_output_cdn"].(string)
	if !strings.Contains(cdn, "not declared") {
		t.Errorf("build_output_cdn = %q", cdn)
	}
}

func TestAppDevPublishExecute_MiaodaProtocol(t *testing.T) {
	// spark.json declares a custom build command and output dir; the app
	// section is replaced wholesale on success.
	root := chdirSparkProjectRoot(t, `{
  "stack": "custom-webapp",
  "dev": { "port": 5173 },
  "build": { "command": ["make", "site"], "output": "public" },
  "app": { "id": "app_x" }
}`)
	srv := newTOSTLSServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	// build.output points straight at the same-origin artifact directory.
	f := &fakeEnvRunner{sideEffect: func() {
		writeDistFiles(t, filepath.Join(root, "public"), []string{"index.html", "routes.json"})
	}}
	withFakeEnvRunner(t, f)
	factory, stdout, reg := newAppsExecuteFactory(t)
	stubPreRelease(reg, "app_x", srv.URL, nil)
	stubReleases(reg, "app_x", map[string]interface{}{
		"release_id": "rel_20", "status": "finished",
		"online_url": "https://x/app/app_x",
	})
	if err := runAppsShortcut(t, AppsDeploy, []string{"+deploy", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	// Declared build command executed (not npm run build).
	if f.name != "make" || len(f.args) != 1 || f.args[0] != "site" {
		t.Errorf("build call = %v %v, want make site", f.name, f.args)
	}
	// App section replaced wholesale with id+url; declarations preserved.
	b, _ := os.ReadFile(filepath.Join(root, sparkJSONRelPath))
	var doc map[string]interface{}
	_ = json.Unmarshal(b, &doc)
	app, _ := doc["app"].(map[string]interface{})
	if app == nil || app["id"] != "app_x" || app["online_url"] != "https://x/app/app_x" {
		t.Errorf("app section = %v", doc["app"])
	}
	if doc["stack"] != "custom-webapp" || doc["build"] == nil {
		t.Errorf("declaration fields must be preserved: %v", doc)
	}
}

func TestAppDevPublishExecute_MiaodaFlagBackfill(t *testing.T) {
	// No recorded app id in spark.json: --app-id publishes and the app
	// section is written on success (async: no url yet).
	root := chdirSparkProjectRoot(t, `{"stack":"react-standard-webapp","dev":{"port":5173}}`)
	writeDistFiles(t, filepath.Join(root, appDevDefaultBuildOutput), []string{"index.html", "routes.json"})
	srv := newTOSTLSServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	factory, stdout, reg := newAppsExecuteFactory(t)
	stubPreRelease(reg, "app_new1", srv.URL, nil)
	stubReleases(reg, "app_new1", map[string]interface{}{"release_id": "rel_21", "status": "pending"})
	if err := runAppsShortcut(t, AppsDeploy,
		[]string{"+deploy", "--app-id", "app_new1", "--skip-build", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(root, sparkJSONRelPath))
	var doc map[string]interface{}
	_ = json.Unmarshal(b, &doc)
	app, _ := doc["app"].(map[string]interface{})
	if app == nil || app["id"] != "app_new1" {
		t.Errorf("app section = %v", doc["app"])
	}
	if _, has := app["online_url"]; has {
		t.Error("async publish must not write app.url")
	}
}

func TestAppDevPublishValidate_MiaodaMismatch(t *testing.T) {
	chdirSparkProjectRoot(t, `{"app": {"id": "app_recorded"}}`)
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsDeploy,
		[]string{"+deploy", "--app-id", "app_other", "--as", "user"}, factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryValidation)
	if !strings.Contains(p.Message, "spark.json") || !strings.Contains(p.Message, "app_recorded") {
		t.Errorf("message = %q", p.Message)
	}
}

func TestAppsDeploy_Declaration(t *testing.T) {
	if AppsDeploy.Command != "+deploy" {
		t.Errorf("Command = %q", AppsDeploy.Command)
	}
	if AppsDeploy.Risk != "write" {
		t.Errorf("Risk = %q", AppsDeploy.Risk)
	}
	if !AppsDeploy.HasFormat {
		t.Error("HasFormat = false")
	}
	if len(AppsDeploy.Scopes) != 2 {
		t.Errorf("Scopes = %v", AppsDeploy.Scopes)
	}
}
