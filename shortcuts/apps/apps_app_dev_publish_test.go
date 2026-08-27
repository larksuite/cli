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

func TestEnsureMetaOnlineURL(t *testing.T) {
	dir := t.TempDir()
	// Missing meta.json: best-effort no-op.
	if err := ensureMetaOnlineURL(dir, "https://x/app/app_x"); err != nil {
		t.Errorf("missing meta must not error: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".spark"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, metaRelPath), []byte(`{"app_id":"app_x","stack":"s"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureMetaOnlineURL(dir, "https://x/app/app_x"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, metaRelPath))
	if err != nil {
		t.Fatal(err)
	}
	var meta map[string]interface{}
	if err := json.Unmarshal(b, &meta); err != nil {
		t.Fatal(err)
	}
	if meta["online_url"] != "https://x/app/app_x" || meta["app_id"] != "app_x" || meta["stack"] != "s" {
		t.Errorf("meta after backfill = %v", meta)
	}
}

// --- dist layout validation ---

// writeDistFiles creates files (relative to base) with parent dirs. A file
// named routes.json gets valid v1 schema content so protocol validation
// passes by default; tests that need a broken one overwrite it afterwards.
func writeDistFiles(t *testing.T, base string, files []string) {
	t.Helper()
	for _, f := range files {
		p := filepath.Join(base, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		body := "x"
		if strings.HasSuffix(f, "routes.json") {
			body = `{"version":1,"type":"test-stack","fallback":"index.html"}`
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestValidateAppDevDist(t *testing.T) {
	tests := []struct {
		name    string
		files   []string
		wantErr string // "" = valid
	}{
		{"ok full", []string{"output/index.html", "output/routes.json", "output_resource/index.js"}, ""},
		{"ok no resource", []string{"output/index.html", "output/routes.json"}, ""},
		{"ok non-index html", []string{"output/page.html", "output/routes.json"}, ""},
		{"ok capabilities dir", []string{"output/index.html", "output/routes.json", "output_capabilities/cap.json"}, ""},
		{"no output dir", []string{"stray.txt"}, "top-level entr"},
		{"no html", []string{"output/routes.json"}, "no .html file"},
		{"no routes", []string{"output/index.html"}, "routes.json"},
		{"extra top-level dir", []string{"output/index.html", "output/routes.json", "extra/x.js"}, "outside the artifact-hosting layout"},
		{"extra top-level file", []string{"output/index.html", "output/routes.json", "notes.md"}, "outside the artifact-hosting layout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dist := filepath.Join(t.TempDir(), "dist")
			writeDistFiles(t, dist, tt.files)
			_, err := validateAppDevDist(permissiveFIO{}, dist, false)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("want valid, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateAppDevDist_RoutesSchema(t *testing.T) {
	dist := filepath.Join(t.TempDir(), "dist")
	writeDistFiles(t, dist, []string{"output/index.html", "output/routes.json"})
	// Broken JSON.
	os.WriteFile(filepath.Join(dist, "output", "routes.json"), []byte("not-json"), 0o644)
	if _, err := validateAppDevDist(permissiveFIO{}, dist, false); err == nil || !strings.Contains(err.Error(), "not valid JSON") {
		t.Errorf("broken routes.json must be rejected, got %v", err)
	}
	// Missing required fields.
	os.WriteFile(filepath.Join(dist, "output", "routes.json"), []byte(`{"version":1}`), 0o644)
	if _, err := validateAppDevDist(permissiveFIO{}, dist, false); err == nil || !strings.Contains(err.Error(), "missing required fields") {
		t.Errorf("incomplete routes.json must be rejected, got %v", err)
	}
	// Unknown fields ignored (forward compatible).
	os.WriteFile(filepath.Join(dist, "output", "routes.json"), []byte(`{"version":1,"type":"t","fallback":"index.html","future":true}`), 0o644)
	if _, err := validateAppDevDist(permissiveFIO{}, dist, false); err != nil {
		t.Errorf("unknown fields must be ignored: %v", err)
	}
}

func TestValidateAppDevDist_Missing(t *testing.T) {
	_, err := validateAppDevDist(permissiveFIO{}, filepath.Join(t.TempDir(), "dist"), false)
	p := requireAppsProblem(t, err, errs.CategoryValidation)
	if p.Subtype != errs.SubtypeFailedPrecondition {
		t.Errorf("subtype = %q, want failed_precondition", p.Subtype)
	}
	if !strings.Contains(p.Hint, "--skip-build") {
		t.Errorf("hint = %q", p.Hint)
	}
}

func TestValidateAppDevDist_Sensitive(t *testing.T) {
	dist := filepath.Join(t.TempDir(), "dist")
	writeDistFiles(t, dist, []string{"output/index.html", "output/routes.json", "output/.env"})
	_, err := validateAppDevDist(permissiveFIO{}, dist, false)
	if err == nil || !strings.Contains(err.Error(), "credential file") {
		t.Errorf("sensitive file must be rejected, got %v", err)
	}
	if _, err := validateAppDevDist(permissiveFIO{}, dist, true); err != nil {
		t.Errorf("allow-sensitive must waive the scan: %v", err)
	}
}

// --- zip packing ---

func TestBuildAppDevZip(t *testing.T) {
	dist := filepath.Join(t.TempDir(), "dist")
	writeDistFiles(t, dist, []string{"output/index.html", "output/routes.json", "output_resource/a.js"})
	candidates, err := validateAppDevDist(permissiveFIO{}, dist, false)
	if err != nil {
		t.Fatal(err)
	}
	zipball, err := buildAppDevZip(permissiveFIO{}, candidates)
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
			t.Errorf("unexpected zip entry %q (dist prefix must be stripped)", n)
		}
	}
}

func TestBuildAppDevZip_RawSizeCap(t *testing.T) {
	orig := maxAppDevPublishRawBytes
	maxAppDevPublishRawBytes = 1
	t.Cleanup(func() { maxAppDevPublishRawBytes = orig })
	dist := filepath.Join(t.TempDir(), "dist")
	writeDistFiles(t, dist, []string{"output/index.html", "output/routes.json"})
	candidates, err := validateAppDevDist(permissiveFIO{}, dist, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buildAppDevZip(permissiveFIO{}, candidates); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("raw cap must reject, got %v", err)
	}
}

func TestBuildAppDevZip_ZipSizeCap(t *testing.T) {
	orig := maxAppDevPublishZipBytes
	maxAppDevPublishZipBytes = 1
	t.Cleanup(func() { maxAppDevPublishZipBytes = orig })
	dist := filepath.Join(t.TempDir(), "dist")
	writeDistFiles(t, dist, []string{"output/index.html", "output/routes.json"})
	candidates, err := validateAppDevDist(permissiveFIO{}, dist, false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = buildAppDevZip(permissiveFIO{}, candidates)
	if err == nil || !strings.Contains(err.Error(), "packed zip size") {
		t.Errorf("zip cap must reject, got %v", err)
	}
	p, _ := errs.ProblemOf(err)
	if p == nil || !strings.Contains(p.Hint, "reduce dist contents") {
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

// chdirMiaodaProjectRoot creates a temp project root with miaoda.json and
// chdirs into it (the protocol-first path).
func chdirMiaodaProjectRoot(t *testing.T, miaodaJSON string) string {
	t.Helper()
	root := t.TempDir()
	if miaodaJSON != "" {
		if err := os.WriteFile(filepath.Join(root, miaodaJSONRelPath), []byte(miaodaJSON), 0o644); err != nil {
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

// chdirProjectRoot creates a temp project root with .spark/meta.json and
// chdirs into it for the test (legacy fallback path).
func chdirProjectRoot(t *testing.T, metaJSON string) string {
	t.Helper()
	root := t.TempDir()
	if metaJSON != "" {
		if err := os.MkdirAll(filepath.Join(root, ".spark"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, metaRelPath), []byte(metaJSON), 0o644); err != nil {
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
		map[string]interface{}{"key": "upload_url", "value": uploadURL},
		map[string]interface{}{"key": "tos_path", "value": "bucket/pkg.zip"},
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
	chdirProjectRoot(t, "")
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsAppDevPublish, []string{"+app-dev-publish", "--as", "user"}, factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryValidation)
	if p.Subtype != errs.SubtypeFailedPrecondition || !strings.Contains(p.Message, "not a Miaoda app project") {
		t.Errorf("got %v", p)
	}
	if !strings.Contains(p.Message, "miaoda.json") {
		t.Errorf("message should name miaoda.json, got %q", p.Message)
	}
	if !strings.Contains(p.Hint, "+app-dev-init-template") {
		t.Errorf("hint = %q", p.Hint)
	}
}

func TestAppDevPublishValidate_NoAppID(t *testing.T) {
	chdirProjectRoot(t, `{"stack":"react-standard-webapp"}`)
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsAppDevPublish, []string{"+app-dev-publish", "--as", "user"}, factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryValidation)
	if !strings.Contains(p.Message, "no publish target") {
		t.Errorf("message = %q", p.Message)
	}
	// The guidance must lead to +create and the new --app-id flow (no manual
	// JSON editing).
	if !strings.Contains(p.Hint, "+create") || !strings.Contains(p.Hint, "+app-dev-publish --app-id") {
		t.Errorf("hint = %q", p.Hint)
	}
}

func TestAppDevPublishValidate_AppIDMismatch(t *testing.T) {
	chdirProjectRoot(t, `{"app_id":"app_recorded"}`)
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsAppDevPublish,
		[]string{"+app-dev-publish", "--app-id", "app_other", "--as", "user"}, factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryValidation)
	if !strings.Contains(p.Message, "app_recorded") || !strings.Contains(p.Message, "app_other") {
		t.Errorf("message must name both ids, got %q", p.Message)
	}
	if !strings.Contains(p.Hint, "drop --app-id") {
		t.Errorf("hint = %q", p.Hint)
	}
}

func TestAppDevPublishExecute_FlagAppIDBackfill(t *testing.T) {
	root := chdirProjectRoot(t, `{"stack":"react-standard-webapp"}`) // no app_id
	writeDistFiles(t, filepath.Join(root, appDevDistDir), []string{"output/index.html", "output/routes.json"})
	srv := newTOSTLSServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	factory, stdout, reg := newAppsExecuteFactory(t)
	stubPreRelease(reg, "app_flag1", srv.URL, nil)
	stubReleases(reg, "app_flag1", map[string]interface{}{"release_id": "rel_9", "status": "pending"})
	if err := runAppsShortcut(t, AppsAppDevPublish,
		[]string{"+app-dev-publish", "--app-id", "app_flag1", "--skip-build", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	// app_id persisted on success, other fields preserved.
	b, _ := os.ReadFile(filepath.Join(root, metaRelPath))
	var meta map[string]interface{}
	_ = json.Unmarshal(b, &meta)
	if meta["app_id"] != "app_flag1" || meta["stack"] != "react-standard-webapp" {
		t.Errorf("meta after publish = %v", meta)
	}
}

func TestAppDevPublishExecute_FlagMatchesMeta(t *testing.T) {
	root := chdirProjectRoot(t, `{"app_id":"app_x"}`)
	writeDistFiles(t, filepath.Join(root, appDevDistDir), []string{"output/index.html", "output/routes.json"})
	srv := newTOSTLSServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	factory, stdout, reg := newAppsExecuteFactory(t)
	stubPreRelease(reg, "app_x", srv.URL, nil)
	stubReleases(reg, "app_x", map[string]interface{}{"release_id": "rel_10", "status": "pending"})
	if err := runAppsShortcut(t, AppsAppDevPublish,
		[]string{"+app-dev-publish", "--app-id", "app_x", "--skip-build", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("matching --app-id must publish fine: %v", err)
	}
}

func TestAppDevPublishValidate_BadAppID(t *testing.T) {
	chdirProjectRoot(t, `{"app_id":"meta_token_x"}`)
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsAppDevPublish, []string{"+app-dev-publish", "--as", "user"}, factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryValidation)
	if !strings.Contains(p.Message, ".spark/meta.json app id") {
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

func TestAppDevPublishValidate_SensitiveGatesDryRun(t *testing.T) {
	root := chdirProjectRoot(t, `{"app_id":"app_x"}`)
	writeDistFiles(t, filepath.Join(root, appDevDistDir),
		[]string{"output/index.html", "output/routes.json", "output_resource/.env"})
	factory, stdout, _ := newAppsExecuteFactory(t)
	// Sensitive hits are the one exception to dry-run's exit-0 convention:
	// Validate rejects before the DryRun branch runs.
	err := runAppsShortcut(t, AppsAppDevPublish,
		[]string{"+app-dev-publish", "--skip-build", "--as", "user", "--dry-run"}, factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryValidation)
	if !strings.Contains(p.Message, "dist contains") || !strings.Contains(p.Message, "credential file") {
		t.Errorf("message = %q", p.Message)
	}
	// This command has no --path flag; the error must not mention one.
	if strings.Contains(p.Message, "--path") {
		t.Errorf("error must not reference a nonexistent --path flag: %q", p.Message)
	}
	// --allow-sensitive waives the gate and dry-run goes back to exit 0.
	if err := runAppsShortcut(t, AppsAppDevPublish,
		[]string{"+app-dev-publish", "--skip-build", "--allow-sensitive", "--as", "user", "--dry-run"}, factory, stdout); err != nil {
		t.Errorf("allow-sensitive dry-run should pass: %v", err)
	}
}

func TestAppDevPublishValidate_SkipBuildNoDist(t *testing.T) {
	chdirProjectRoot(t, `{"app_id":"app_x"}`)
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsAppDevPublish, []string{"+app-dev-publish", "--skip-build", "--as", "user"}, factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryValidation)
	if p.Subtype != errs.SubtypeFailedPrecondition || !strings.Contains(p.Message, "build output directory dist does not exist") {
		t.Errorf("got %v", p)
	}
}

func TestAppDevPublishExecute_SyncSuccess(t *testing.T) {
	root := chdirProjectRoot(t, `{"app_id":"app_x","stack":"react-standard-webapp"}`)
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
		writeDistFiles(t, filepath.Join(root, appDevDistDir), []string{"output/index.html", "output/routes.json", "output_resource/a.js"})
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
	if err := runAppsShortcut(t, AppsAppDevPublish, []string{"+app-dev-publish", "--as", "user"}, factory, stdout); err != nil {
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
	// meta.json backfill.
	b, _ := os.ReadFile(filepath.Join(root, metaRelPath))
	var meta map[string]interface{}
	_ = json.Unmarshal(b, &meta)
	if meta["online_url"] != "https://x.feishuapp.cn/app/app_x" || meta["app_id"] != "app_x" {
		t.Errorf("meta after publish = %v", meta)
	}
}

func TestAppDevPublishExecute_AsyncSuccess(t *testing.T) {
	root := chdirProjectRoot(t, `{"app_id":"app_x"}`)
	writeDistFiles(t, filepath.Join(root, appDevDistDir), []string{"output/index.html", "output/routes.json"})
	srv := newTOSTLSServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	factory, stdout, reg := newAppsExecuteFactory(t)
	stubPreRelease(reg, "app_x", srv.URL, nil)
	stubReleases(reg, "app_x", map[string]interface{}{"release_id": "rel_2", "status": "pending"})
	if err := runAppsShortcut(t, AppsAppDevPublish, []string{"+app-dev-publish", "--skip-build", "--as", "user"}, factory, stdout); err != nil {
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
	// No online_url -> no backfill.
	b, _ := os.ReadFile(filepath.Join(root, metaRelPath))
	if strings.Contains(string(b), "online_url") {
		t.Errorf("meta must not gain online_url on async publish: %s", b)
	}
}

func TestAppDevPublishExecute_BuildFails(t *testing.T) {
	chdirProjectRoot(t, `{"app_id":"app_x"}`)
	srv := newTOSTLSServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	f := &fakeEnvRunner{stderr: "TS2304: boom", err: errors.New("exit 1")}
	withFakeEnvRunner(t, f)
	factory, stdout, reg := newAppsExecuteFactory(t)
	stubPreRelease(reg, "app_x", srv.URL, nil)
	err := runAppsShortcut(t, AppsAppDevPublish, []string{"+app-dev-publish", "--as", "user"}, factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryInternal)
	if !strings.Contains(p.Message, `build command "npm run build" failed`) || !strings.Contains(p.Message, "TS2304") {
		t.Errorf("message = %q", p.Message)
	}
	if !strings.Contains(p.Hint, "--skip-build") {
		t.Errorf("hint = %q", p.Hint)
	}
}

func TestAppDevPublishExecute_PreReleaseMissingKVs(t *testing.T) {
	root := chdirProjectRoot(t, `{"app_id":"app_x"}`)
	writeDistFiles(t, filepath.Join(root, appDevDistDir), []string{"output/index.html", "output/routes.json"})
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/pre_release",
		Body: map[string]interface{}{
			"code": float64(0),
			"data": map[string]interface{}{"kvs": []interface{}{}},
		},
	})
	err := runAppsShortcut(t, AppsAppDevPublish, []string{"+app-dev-publish", "--skip-build", "--as", "user"}, factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryInternal)
	if !strings.Contains(p.Message, "missing upload_url or tos_path") {
		t.Errorf("message = %q", p.Message)
	}
}

func TestAppDevPublishExecute_NonHTTPSUploadURL(t *testing.T) {
	root := chdirProjectRoot(t, `{"app_id":"app_x"}`)
	writeDistFiles(t, filepath.Join(root, appDevDistDir), []string{"output/index.html", "output/routes.json"})
	factory, stdout, reg := newAppsExecuteFactory(t)
	stubPreRelease(reg, "app_x", "http://insecure.example/put", nil)
	err := runAppsShortcut(t, AppsAppDevPublish, []string{"+app-dev-publish", "--skip-build", "--as", "user"}, factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryInternal)
	if !strings.Contains(p.Message, "not https") {
		t.Errorf("message = %q", p.Message)
	}
}

func TestAppDevPublishExecute_TOS5xx(t *testing.T) {
	root := chdirProjectRoot(t, `{"app_id":"app_x"}`)
	writeDistFiles(t, filepath.Join(root, appDevDistDir), []string{"output/index.html", "output/routes.json"})
	srv := newTOSTLSServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) })
	factory, stdout, reg := newAppsExecuteFactory(t)
	stubPreRelease(reg, "app_x", srv.URL, nil)
	err := runAppsShortcut(t, AppsAppDevPublish, []string{"+app-dev-publish", "--skip-build", "--as", "user"}, factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryNetwork)
	if !p.Retryable {
		t.Error("5xx upload failure must be retryable")
	}
}

func TestAppDevPublishDryRun(t *testing.T) {
	root := chdirProjectRoot(t, `{"app_id":"app_x"}`)
	writeDistFiles(t, filepath.Join(root, appDevDistDir), []string{"output/index.html"})
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsAppDevPublish, []string{"+app-dev-publish", "--skip-build", "--as", "user", "--dry-run"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	data, err := decodeDryRunDataMap(stdout.Bytes())
	if err != nil {
		t.Fatalf("decode dry-run: %v (raw=%q)", err, stdout.String())
	}
	if data["app_id"] != "app_x" {
		t.Errorf("app_id = %v", data["app_id"])
	}
	if verr, _ := data["dist_validation_error"].(string); !strings.Contains(verr, "routes.json") {
		t.Errorf("dist_validation_error = %v (routes.json missing should surface)", data["dist_validation_error"])
	}
	buildCmd, _ := data["build_command"].(string)
	if !strings.Contains(buildCmd, "MIAODA_*") {
		t.Errorf("build_command = %q", buildCmd)
	}
}

func TestAppDevPublishExecute_MiaodaProtocol(t *testing.T) {
	// miaoda.json declares a custom build command and output dir; the app
	// section is replaced wholesale on success.
	root := chdirMiaodaProjectRoot(t, `{
  "stack": "custom-webapp",
  "build": { "command": ["make", "site"], "output": "public" },
  "app": { "id": "app_x" }
}`)
	srv := newTOSTLSServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	f := &fakeEnvRunner{sideEffect: func() {
		writeDistFiles(t, filepath.Join(root, "public"), []string{"output/index.html", "output/routes.json"})
		routes := `{"version":1,"type":"custom-webapp","fallback":"index.html"}`
		os.WriteFile(filepath.Join(root, "public", "output", "routes.json"), []byte(routes), 0o644)
	}}
	withFakeEnvRunner(t, f)
	factory, stdout, reg := newAppsExecuteFactory(t)
	stubPreRelease(reg, "app_x", srv.URL, nil)
	stubReleases(reg, "app_x", map[string]interface{}{
		"release_id": "rel_20", "status": "finished",
		"online_url": "https://x/app/app_x",
	})
	if err := runAppsShortcut(t, AppsAppDevPublish, []string{"+app-dev-publish", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	// Declared build command executed (not npm run build).
	if f.name != "make" || len(f.args) != 1 || f.args[0] != "site" {
		t.Errorf("build call = %v %v, want make site", f.name, f.args)
	}
	// App section replaced wholesale with id+url; declarations preserved.
	b, _ := os.ReadFile(filepath.Join(root, miaodaJSONRelPath))
	var doc map[string]interface{}
	_ = json.Unmarshal(b, &doc)
	app, _ := doc["app"].(map[string]interface{})
	if app == nil || app["id"] != "app_x" || app["url"] != "https://x/app/app_x" {
		t.Errorf("app section = %v", doc["app"])
	}
	if doc["stack"] != "custom-webapp" || doc["build"] == nil {
		t.Errorf("declaration fields must be preserved: %v", doc)
	}
}

func TestAppDevPublishExecute_MiaodaFlagBackfill(t *testing.T) {
	// No recorded app id in miaoda.json: --app-id publishes and the app
	// section is written on success (async: no url yet).
	root := chdirMiaodaProjectRoot(t, `{"stack":"react-standard-webapp"}`)
	writeDistFiles(t, filepath.Join(root, appDevDefaultBuildOutput), []string{"output/index.html", "output/routes.json"})
	srv := newTOSTLSServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	factory, stdout, reg := newAppsExecuteFactory(t)
	stubPreRelease(reg, "app_new1", srv.URL, nil)
	stubReleases(reg, "app_new1", map[string]interface{}{"release_id": "rel_21", "status": "pending"})
	if err := runAppsShortcut(t, AppsAppDevPublish,
		[]string{"+app-dev-publish", "--app-id", "app_new1", "--skip-build", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(root, miaodaJSONRelPath))
	var doc map[string]interface{}
	_ = json.Unmarshal(b, &doc)
	app, _ := doc["app"].(map[string]interface{})
	if app == nil || app["id"] != "app_new1" {
		t.Errorf("app section = %v", doc["app"])
	}
	if _, has := app["url"]; has {
		t.Error("async publish must not write app.url")
	}
}

func TestAppDevPublishValidate_MiaodaMismatch(t *testing.T) {
	chdirMiaodaProjectRoot(t, `{"app": {"id": "app_recorded"}}`)
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsAppDevPublish,
		[]string{"+app-dev-publish", "--app-id", "app_other", "--as", "user"}, factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryValidation)
	if !strings.Contains(p.Message, "miaoda.json") || !strings.Contains(p.Message, "app_recorded") {
		t.Errorf("message = %q", p.Message)
	}
}

func TestAppsAppDevPublish_Declaration(t *testing.T) {
	if AppsAppDevPublish.Command != "+app-dev-publish" {
		t.Errorf("Command = %q", AppsAppDevPublish.Command)
	}
	if AppsAppDevPublish.Risk != "write" {
		t.Errorf("Risk = %q", AppsAppDevPublish.Risk)
	}
	if !AppsAppDevPublish.HasFormat {
		t.Error("HasFormat = false")
	}
	if len(AppsAppDevPublish.Scopes) != 2 {
		t.Errorf("Scopes = %v", AppsAppDevPublish.Scopes)
	}
}
