// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/apps/deploy"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestIsHTMLDeployMode(t *testing.T) {
	if !isHTMLDeployMode("a.html", "", "") || !isHTMLDeployMode("", "./site", "") {
		t.Error("either payload flag should select html deploy mode")
	}
	// --entry-file cannot stand alone, but it must still select this mode:
	// otherwise a lone --entry-file falls through to the project mode and the
	// user is told to scaffold a project instead of to add --dir.
	if !isHTMLDeployMode("", "", "page.html") {
		t.Error("--entry-file alone must select html deploy mode so its own error can surface")
	}
	if isHTMLDeployMode("", "", "") {
		t.Error("no payload flag should keep the existing project mode")
	}
}

func TestValidateHTMLDeployFlags(t *testing.T) {
	cases := []struct {
		name                 string
		filePath, dir, entry string
		skipBuild, noVerify  bool
		wantErr              string
	}{
		{name: "两者同时给", filePath: "a.html", dir: "./site", wantErr: "mutually exclusive"},
		{name: "entry-file 无 dir", filePath: "a.html", entry: "p.html", wantErr: "--entry-file"},
		{name: "skip-build 误用", filePath: "a.html", skipBuild: true, wantErr: "--skip-build"},
		{name: "no-verify 误用", dir: "./site", noVerify: true, wantErr: "--no-verify"},
		{name: "file-path 非 html", filePath: "a.txt", wantErr: "--file-path"},
		{name: "entry-file 带路径分隔符", dir: "./site", entry: "sub/p.html", wantErr: "--entry-file"},
		{name: "合法单文件", filePath: "a.html"},
		{name: "合法目录带入口", dir: "./site", entry: "p.html"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateHTMLDeployFlags(tc.filePath, tc.dir, tc.entry, tc.skipBuild, tc.noVerify)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("got %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

// htmlDeployRuntime builds a RuntimeContext carrying only the bare-HTML flags,
// so validateHTMLDeploy can be exercised without the spark.json project setup.
func htmlDeployRuntime(t *testing.T, filePath, dir, entry string, allowSensitive bool) *common.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "+deploy"}
	cmd.Flags().String("file-path", filePath, "")
	cmd.Flags().String("dir", dir, "")
	cmd.Flags().String("entry-file", entry, "")
	cmd.Flags().Bool("allow-sensitive", allowSensitive, "")
	cmd.Flags().Bool("skip-build", false, "")
	cmd.Flags().Bool("no-verify", false, "")
	return common.TestNewRuntimeContext(cmd, nil)
}

// chdirHTMLPayload writes files (relative names -> content) under a temp dir and
// chdirs into it, since the publish flags only accept cwd-relative paths.
func chdirHTMLPayload(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	return root
}

func TestValidateHTMLDeploy(t *testing.T) {
	t.Run("单文件通过", func(t *testing.T) {
		chdirHTMLPayload(t, map[string]string{"report.html": "<h1>hi</h1>"})
		if err := validateHTMLDeploy(htmlDeployRuntime(t, "report.html", "", "", false)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("目录默认入口通过", func(t *testing.T) {
		chdirHTMLPayload(t, map[string]string{
			"site/index.html":     "<h1>hi</h1>",
			"site/assets/app.css": "body{}",
		})
		if err := validateHTMLDeploy(htmlDeployRuntime(t, "", "site", "", false)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("目录缺入口报错", func(t *testing.T) {
		chdirHTMLPayload(t, map[string]string{"site/page.html": "<h1>hi</h1>"})
		err := validateHTMLDeploy(htmlDeployRuntime(t, "", "site", "", false))
		if err == nil || !strings.Contains(err.Error(), "no entry file") {
			t.Fatalf("got %v, want a missing-entry error", err)
		}
	})
	t.Run("凭证文件拦截", func(t *testing.T) {
		chdirHTMLPayload(t, map[string]string{
			"site/index.html": "<h1>hi</h1>",
			"site/.env":       "TOKEN=x",
		})
		err := validateHTMLDeploy(htmlDeployRuntime(t, "", "site", "", false))
		if err == nil || !strings.Contains(err.Error(), "credential file") {
			t.Fatalf("got %v, want the credential scan to reject the payload", err)
		}
	})
	t.Run("allow-sensitive 放行", func(t *testing.T) {
		chdirHTMLPayload(t, map[string]string{
			"site/index.html": "<h1>hi</h1>",
			"site/.env":       "TOKEN=x",
		})
		if err := validateHTMLDeploy(htmlDeployRuntime(t, "", "site", "", true)); err != nil {
			t.Fatalf("--allow-sensitive should waive the scan: %v", err)
		}
	})
	t.Run("flag 组合先于文件系统检查", func(t *testing.T) {
		chdirHTMLPayload(t, nil)
		err := validateHTMLDeploy(htmlDeployRuntime(t, "a.html", "site", "", false))
		if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
			t.Fatalf("got %v, want the flag conflict reported before any stat", err)
		}
	})
}

func TestHTMLAppIDFromFlagSkipsLookup(t *testing.T) {
	src, id := htmlAppIDFromFlag("app_1abc")
	if src != htmlAppIDSourceFlag || id != "app_1abc" {
		t.Errorf("got (%v, %q), want (--app-id, app_1abc)", src, id)
	}
	src, id = htmlAppIDFromFlag("  ")
	if src != htmlAppIDSourceLookup || id != "" {
		t.Errorf("got (%v, %q), want (lookup, \"\")", src, id)
	}
}

func TestParseHasHTMLAppCreated(t *testing.T) {
	exists, id := parseHasHTMLAppCreated(map[string]interface{}{
		"exists": true, "app_id": "app_x", "app_url": "https://ignored", "ccm_token": "ignored",
	})
	if !exists || id != "app_x" {
		t.Errorf("got (%v, %q), want (true, app_x)", exists, id)
	}
	if exists, id := parseHasHTMLAppCreated(map[string]interface{}{"exists": false}); exists || id != "" {
		t.Errorf("got (%v, %q), want (false, \"\")", exists, id)
	}
	if exists, id := parseHasHTMLAppCreated(map[string]interface{}{"exists": true}); !exists || id != "" {
		t.Errorf("got (%v, %q), want (true, \"\") when app_id is absent", exists, id)
	}
}

// TestHTMLAppIDLookupPath pins the idempotency endpoint and the third source,
// which the create fallback in the Execute step reports.
func TestHTMLAppIDLookupPath(t *testing.T) {
	if hasHTMLAppCreatedPath != "/open-apis/spark/v1/apps/has_html_app_created" {
		t.Errorf("unexpected lookup path %q", hasHTMLAppCreatedPath)
	}
	if htmlAppIDSourceCreate != "+create" {
		t.Errorf("unexpected create source %q", htmlAppIDSourceCreate)
	}
}

// TestHTMLDeployPlanFields covers every field Execute reads off the resolved
// plan, so a missing one shows up here rather than at the call site.
func TestHTMLDeployPlanFields(t *testing.T) {
	plan := htmlDeployPlan{
		AbsEntry:    "/Users/me/site/index.html",
		EntryRel:    "index.html",
		AppID:       "app_x",
		AppIDSource: htmlAppIDSourceLookup,
		Entries:     []deploy.PackEntry{{ZipPath: "output/index.html", Size: 11}},
		FileCount:   1,
		TotalBytes:  11,
		ZipPaths:    []string{"output/index.html"},
		RouteCount:  1,
		ContentHash: "abc",
		Waived:      []string{".env"},
	}
	if plan.AbsEntry == "" || plan.EntryRel == "" || plan.AppID == "" {
		t.Error("entry and app id must survive into the plan")
	}
	if plan.AppIDSource != htmlAppIDSourceLookup {
		t.Errorf("unexpected app id source %q", plan.AppIDSource)
	}
	if len(plan.Entries) != 1 || plan.Entries[0].ZipPath != "output/index.html" {
		t.Errorf("unexpected entries %+v", plan.Entries)
	}
	if plan.FileCount != 1 || plan.TotalBytes != 11 || plan.RouteCount != 1 {
		t.Errorf("unexpected counters %+v", plan)
	}
	if len(plan.ZipPaths) != 1 || plan.ContentHash != "abc" {
		t.Errorf("unexpected zip paths or hash %+v", plan)
	}
	if len(plan.Waived) != 1 || plan.Waived[0] != ".env" {
		t.Errorf("waived credential files must stay on the plan for the stderr notice, got %v", plan.Waived)
	}
}

// TestHTMLDeployDryRunExposesOutboundPath pins the security-review release
// condition: the dry-run must echo the absolute path that actually leaves the
// machine, not just the relative entry name.
func TestHTMLDeployDryRunExposesOutboundPath(t *testing.T) {
	dry := common.NewDryRunAPI()
	fillHTMLDeployDryRun(dry, htmlDeployPlan{
		AbsEntry:    "/Users/me/work/report.html",
		EntryRel:    "report.html",
		AppIDSource: htmlAppIDSourceLookup,
		FileCount:   2,
		TotalBytes:  40,
		ZipPaths:    []string{"output/index.html", "output/a.css"},
		RouteCount:  1,
		ContentHash: "abc",
	})
	raw, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry-run: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal dry-run: %v", err)
	}
	if out["idempotent_key"] != "/Users/me/work/report.html" {
		t.Errorf("dry-run must show the absolute path sent as idempotent_key, got %v", out["idempotent_key"])
	}
	if out["file_path"] != "/Users/me/work/report.html" {
		t.Errorf("dry-run must show the absolute path sent as file_path, got %v", out["file_path"])
	}
	if out["content_hash"] != "abc" {
		t.Errorf("content_hash missing from dry-run: %v", out)
	}
	if out["app_id_source"] != string(htmlAppIDSourceLookup) {
		t.Errorf("app_id_source = %v", out["app_id_source"])
	}
}

func TestToAppDevEntries(t *testing.T) {
	got := toAppDevEntries([]deploy.PackEntry{
		{ZipPath: "output/index.html", AbsPath: "site/index.html", Size: 11},
		{ZipPath: "output/routes.json", Content: []byte("[]"), Size: 2},
	})
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if got[0].ZipPath != "output/index.html" || got[0].AbsPath != "site/index.html" || got[0].Size != 11 {
		t.Errorf("disk entry = %+v", got[0])
	}
	if got[1].AbsPath != "" || string(got[1].Content) != "[]" {
		t.Errorf("generated entry = %+v", got[1])
	}
}

func TestResolveHTMLDeployPlan(t *testing.T) {
	t.Run("单文件入口改名并生成路由", func(t *testing.T) {
		root := chdirHTMLPayload(t, map[string]string{"report.html": "<h1>hi</h1>"})
		plan, err := resolveHTMLDeployPlan(htmlDeployRuntime(t, "report.html", "", "", false))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := filepath.Join(resolvedRoot(t, root), "report.html"); plan.AbsEntry != want {
			t.Errorf("AbsEntry = %q, want %q", plan.AbsEntry, want)
		}
		if plan.EntryRel != "report.html" {
			t.Errorf("EntryRel = %q", plan.EntryRel)
		}
		wantPaths := []string{"output/index.html", "output/routes.json"}
		if !reflect.DeepEqual(plan.ZipPaths, wantPaths) {
			t.Errorf("ZipPaths = %v, want %v", plan.ZipPaths, wantPaths)
		}
		if plan.FileCount != 2 || plan.RouteCount != 1 {
			t.Errorf("FileCount/RouteCount = %d/%d", plan.FileCount, plan.RouteCount)
		}
		if plan.TotalBytes != int64(len("<h1>hi</h1>")) {
			t.Errorf("TotalBytes = %d (raw payload bytes only, routes.json excluded)", plan.TotalBytes)
		}
		// Single-file payload: the fingerprint is the raw file's sha256.
		sum := sha256.Sum256([]byte("<h1>hi</h1>"))
		if plan.ContentHash != hex.EncodeToString(sum[:]) {
			t.Errorf("ContentHash = %q, want the raw sha256", plan.ContentHash)
		}
	})
	t.Run("目录入口改名且其余文件保留路径", func(t *testing.T) {
		root := chdirHTMLPayload(t, map[string]string{
			"site/page.html":      "<h1>hi</h1>",
			"site/assets/app.css": "body{}",
		})
		plan, err := resolveHTMLDeployPlan(htmlDeployRuntime(t, "", "site", "page.html", false))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := filepath.Join(resolvedRoot(t, root), "site", "page.html"); plan.AbsEntry != want {
			t.Errorf("AbsEntry = %q, want %q", plan.AbsEntry, want)
		}
		got := append([]string(nil), plan.ZipPaths...)
		sort.Strings(got)
		want := []string{"output/assets/app.css", "output/index.html", "output/routes.json"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ZipPaths = %v, want %v", got, want)
		}
		if len(plan.ContentHash) != 64 {
			t.Errorf("ContentHash = %q, want a 64-char digest", plan.ContentHash)
		}
	})
	t.Run("凭证放行时记录 waived", func(t *testing.T) {
		chdirHTMLPayload(t, map[string]string{
			"site/index.html": "<h1>hi</h1>",
			"site/.env":       "TOKEN=x",
		})
		plan, err := resolveHTMLDeployPlan(htmlDeployRuntime(t, "", "site", "", true))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(plan.Waived) != 1 || plan.Waived[0] != ".env" {
			t.Errorf("Waived = %v", plan.Waived)
		}
	})
}

// resolvedRoot mirrors the symlink resolution the collector applies, so the
// expected absolute path matches on macOS where /var is a symlink.
func resolvedRoot(t *testing.T, root string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("eval symlinks: %v", err)
	}
	return resolved
}

func stubHasHTMLAppCreated(reg *httpmock.Registry, data map[string]interface{}) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/has_html_app_created",
		Body:   map[string]interface{}{"code": float64(0), "data": data},
	}
	reg.Register(stub)
	return stub
}

func TestHTMLDeployExecute_LookupHit(t *testing.T) {
	root := chdirHTMLPayload(t, map[string]string{
		"site/index.html": "<h1>hi</h1>",
		"site/app.css":    "body{}",
	})
	var uploaded []byte
	srv := newTOSTLSServer(t, func(w http.ResponseWriter, r *http.Request) {
		uploaded, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
	})
	factory, stdout, reg := newAppsExecuteFactory(t)
	lookup := stubHasHTMLAppCreated(reg, map[string]interface{}{"exists": true, "app_id": "app_x"})
	stubPreRelease(reg, "app_x", srv.URL, nil)
	release := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/releases",
		Body: map[string]interface{}{"code": float64(0), "data": map[string]interface{}{
			"release_id": "rel_1", "status": "finished", "online_url": "https://x/app/app_x",
		}},
	}
	reg.Register(release)

	if err := runAppsShortcut(t, AppsDeploy, []string{"+deploy", "--dir", "site", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("unexpected: %v", err)
	}

	// The idempotency key must be the entry's absolute path.
	var lookupBody map[string]interface{}
	if err := json.Unmarshal(lookup.CapturedBody, &lookupBody); err != nil {
		t.Fatalf("decode lookup body: %v", err)
	}
	wantKey := filepath.Join(resolvedRoot(t, root), "site", "index.html")
	if lookupBody["idempotent_key"] != wantKey {
		t.Errorf("idempotent_key = %v, want %q", lookupBody["idempotent_key"], wantKey)
	}
	if len(uploaded) == 0 {
		t.Error("zip body not uploaded")
	}
	// The release carries the content fingerprint. extra is a JSON *string* on
	// the wire: an object form still gets a 200 back, so only an assertion on
	// the encoding catches a regression here.
	var releaseBody struct {
		Extra string `json:"extra"`
	}
	if err := json.Unmarshal(release.CapturedBody, &releaseBody); err != nil {
		t.Fatalf("decode release body (extra must be a JSON string, not an object): %v", err)
	}
	var extra map[string]string
	if err := json.Unmarshal([]byte(releaseBody.Extra), &extra); err != nil {
		t.Fatalf("extra is not a JSON document: %q (%v)", releaseBody.Extra, err)
	}
	if len(extra["hash_tag"]) != 64 {
		t.Errorf("extra.hash_tag = %q, want a 64-char digest", extra["hash_tag"])
	}
	data := parseEnvelopeData(t, stdout)
	if data["app_id"] != "app_x" || data["release_id"] != "rel_1" || data["online_url"] != "https://x/app/app_x" {
		t.Errorf("data = %v", data)
	}
	if data["built"] != false {
		t.Errorf("built = %v, want false on the bare HTML path", data["built"])
	}
	if data["file_count"] != float64(3) {
		t.Errorf("file_count = %v, want 3 (index.html + app.css + routes.json)", data["file_count"])
	}
	// The bare HTML path must never create a spark.json in the payload dir.
	if _, err := os.Stat(filepath.Join(root, sparkJSONRelPath)); !os.IsNotExist(err) {
		t.Errorf("bare HTML publish must not write %s (stat err = %v)", sparkJSONRelPath, err)
	}
}

func TestHTMLDeployExecute_CreateFallback(t *testing.T) {
	root := chdirHTMLPayload(t, map[string]string{"report.html": "<h1>hi</h1>"})
	srv := newTOSTLSServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	factory, stdout, reg := newAppsExecuteFactory(t)
	stubHasHTMLAppCreated(reg, map[string]interface{}{"exists": false})
	create := &httpmock.Stub{
		Method:     "POST",
		URL:        "/open-apis/spark/v1/apps",
		BodyFilter: func(b []byte) bool { return bytes.Contains(b, []byte(`"app_type"`)) },
		Body: map[string]interface{}{"code": float64(0), "data": map[string]interface{}{
			"app": map[string]interface{}{"app_id": "app_new"},
		}},
	}
	reg.Register(create)
	stubPreRelease(reg, "app_new", srv.URL, nil)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_new/releases",
		Body: map[string]interface{}{"code": float64(0), "data": map[string]interface{}{
			"release_id": "rel_2", "status": "publishing",
		}},
	})

	if err := runAppsShortcut(t, AppsDeploy, []string{"+deploy", "--file-path", "report.html", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(create.CapturedBody, &body); err != nil {
		t.Fatalf("decode create body: %v", err)
	}
	if body["app_type"] != "html" {
		t.Errorf("app_type = %v, want html", body["app_type"])
	}
	if body["name"] != "report" {
		t.Errorf("name = %v, want the entry base name", body["name"])
	}
	// Creation is what registers the idempotency key, so the field name has to
	// match what the lookup queries by — file_path would be silently ignored.
	wantPath := filepath.Join(resolvedRoot(t, root), "report.html")
	if body["idempotent_key"] != wantPath {
		t.Errorf("idempotent_key = %v, want %q", body["idempotent_key"], wantPath)
	}
	if _, stale := body["file_path"]; stale {
		t.Error("file_path must not be sent: the server registers the key under idempotent_key")
	}
	data := parseEnvelopeData(t, stdout)
	if data["app_id"] != "app_new" || data["release_id"] != "rel_2" {
		t.Errorf("data = %v", data)
	}
	if _, ok := data["poll_hint"]; !ok {
		t.Errorf("an in-flight release must carry poll_hint: %v", data)
	}
}

func TestHTMLDeployExecute_AppIDFlagSkipsLookup(t *testing.T) {
	chdirHTMLPayload(t, map[string]string{"report.html": "<h1>hi</h1>"})
	srv := newTOSTLSServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	factory, stdout, reg := newAppsExecuteFactory(t)
	lookup := &httpmock.Stub{
		Method:   "POST",
		URL:      "/open-apis/spark/v1/apps/has_html_app_created",
		Optional: true,
		Body:     map[string]interface{}{"code": float64(0), "data": map[string]interface{}{"exists": false}},
	}
	reg.Register(lookup)
	stubPreRelease(reg, "app_flag", srv.URL, nil)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_flag/releases",
		Body: map[string]interface{}{"code": float64(0), "data": map[string]interface{}{
			"release_id": "rel_3", "status": "finished", "online_url": "https://x/app/app_flag",
		}},
	})

	args := []string{"+deploy", "--file-path", "report.html", "--app-id", "app_flag", "--as", "user"}
	if err := runAppsShortcut(t, AppsDeploy, args, factory, stdout); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(lookup.CapturedBodies) != 0 {
		t.Errorf("--app-id must skip the idempotency lookup, got %d call(s)", len(lookup.CapturedBodies))
	}
}

func TestHTMLDeployDryRun_NoWrites(t *testing.T) {
	root := chdirHTMLPayload(t, map[string]string{"report.html": "<h1>hi</h1>"})
	factory, stdout, reg := newAppsExecuteFactory(t)
	args := []string{"+deploy", "--file-path", "report.html", "--as", "user", "--dry-run"}
	if err := runAppsShortcut(t, AppsDeploy, args, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	reg.Verify(t) // no stub registered: a dry-run must issue no request at all
	data, err := decodeDryRunDataMap(stdout.Bytes())
	if err != nil {
		t.Fatalf("decode dry-run output: %v (raw=%q)", err, stdout.String())
	}
	wantPath := filepath.Join(resolvedRoot(t, root), "report.html")
	if data["idempotent_key"] != wantPath {
		t.Errorf("idempotent_key = %v, want %q", data["idempotent_key"], wantPath)
	}
	// Without --app-id the target is only decided at run time. The preview must
	// say so, and must name the app that would be created — apps has no
	// +delete, so an unexpected create cannot be undone.
	src, _ := data["app_id_source"].(string)
	if !strings.Contains(src, string(htmlAppIDSourceLookup)) ||
		!strings.Contains(src, string(htmlAppIDSourceCreate)) {
		t.Errorf("app_id_source = %v, want both the lookup and the create fallback named", data["app_id_source"])
	}
	if data["app_name_if_created"] != "report" {
		t.Errorf("app_name_if_created = %v, want \"report\"", data["app_name_if_created"])
	}
	if data["entry_file"] != "report.html" {
		t.Errorf("entry_file = %v", data["entry_file"])
	}
	if _, ok := data["content_hash"].(string); !ok {
		t.Errorf("content_hash missing: %v", data)
	}
}

func TestAdoptOrGenerateRoutesNoDuplicateEntry(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "routes.json"),
		[]byte(`[{"path":"/","file":"index.html"},{"path":"/about","file":"about.html"}]`), 0o600); err != nil {
		t.Fatalf("write routes.json: %v", err)
	}
	entries := []deploy.PackEntry{
		{ZipPath: "output/index.html", AbsPath: filepath.Join(root, "index.html")},
		{ZipPath: "output/routes.json", AbsPath: filepath.Join(root, "routes.json")},
	}
	count, err := adoptOrGenerateRoutes(htmlDeployTestFIO{}, &entries, []string{"index.html"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("route count = %d, want 2 (from the payload's own routes.json)", count)
	}
	seen := 0
	for _, e := range entries {
		if e.ZipPath == "output/routes.json" {
			seen++
		}
	}
	// 同名条目出现两次时，服务端解压保留哪一份是未定义的。
	if seen != 1 {
		t.Errorf("output/routes.json appears %d times in the zip manifest, want exactly 1", seen)
	}
}

// htmlDeployTestFIO lets the bare-HTML unit tests read absolute t.TempDir
// paths; production code goes through LocalFileIO, which is cwd-bounded.
// Defined here rather than reused from the +html-publish test files so these
// tests survive that command's planned removal.
type htmlDeployTestFIO struct{}

func (htmlDeployTestFIO) Open(name string) (fileio.File, error)     { return os.Open(name) }
func (htmlDeployTestFIO) Stat(name string) (fileio.FileInfo, error) { return os.Stat(name) }
func (htmlDeployTestFIO) ResolvePath(p string) (string, error)      { return p, nil }
func (htmlDeployTestFIO) Save(string, fileio.SaveOptions, io.Reader) (fileio.SaveResult, error) {
	panic("Save not used in bare-HTML deploy unit tests")
}

// A publish that could not collect everything the pages reference still
// succeeds, so the only signal a machine caller gets is what the envelope says.
// Leaving it on stderr alone hands an agent an unqualified success for a page
// that renders broken -- the exact failure this path exists to prevent.
func TestHTMLDeployResultCarriesSkippedReferences(t *testing.T) {
	plan := htmlDeployPlan{
		AppID: "app_1",
		SkippedDeps: []deploy.Skip{
			{Ref: "assets/logo.png", From: "index.html", Why: "the file does not exist", Kind: deploy.SkipMissing},
		},
	}
	data := htmlDeployResult(plan, 3, 1024, "rel_1", "finished")

	lines, ok := data["dependencies_skipped"].([]string)
	if !ok || len(lines) != 1 {
		t.Fatalf("envelope must carry the skipped references, got %#v", data["dependencies_skipped"])
	}
	if !strings.Contains(lines[0], "assets/logo.png") || !strings.Contains(lines[0], "index.html") {
		t.Errorf("the line should name the file and the page referencing it: %q", lines[0])
	}

	clean := htmlDeployResult(htmlDeployPlan{AppID: "app_1"}, 3, 1024, "rel_1", "finished")
	if _, present := clean["dependencies_skipped"]; present {
		t.Errorf("a publish with nothing skipped must not carry an empty field: %#v", clean)
	}
}

// pre_release is where a publish first learns whether the app accepts an
// uploaded artifact. Answering that failure with "check your --app-id" sends
// someone whose app publishes from git looking for a permissions problem they
// do not have, so the alternative route has to be named.
func TestPreReleaseFailureNamesTheGitRoute(t *testing.T) {
	if !strings.Contains(preReleaseHint, "+release-create") {
		t.Errorf("the hint must name the command that does work: %q", preReleaseHint)
	}
	if !strings.Contains(preReleaseHint, "--app-id") {
		t.Errorf("an unreachable app id is still a live possibility and must stay named: %q", preReleaseHint)
	}
	// Naming a cause we cannot tell apart is how a hint sends people in
	// circles; the failure looks identical either way.
	for _, claim := range []string{"app type", "not supported", "unsupported"} {
		if strings.Contains(strings.ToLower(preReleaseHint), claim) {
			t.Errorf("the hint must not assert a cause the response cannot distinguish (%q): %q", claim, preReleaseHint)
		}
	}
}

// An app that publishes from git answers pre_release with 200 and an empty kv
// set. That is a correct answer about the app, not a malformed response, and
// reporting it as an internal error sent callers to retry or to file a server
// bug when the fix was to use a different command. A populated set missing only
// the upload key is a different thing: nobody can act on that.
func TestNoArtifactUploadErrorSeparatesAppFromServerFault(t *testing.T) {
	t.Run("empty kvs is a statement about the app", func(t *testing.T) {
		err := noArtifactUploadError("app_x", 0, "artifact_url")
		problem := requireAppsValidationProblem(t, err)
		if !strings.Contains(problem.Message, "does not publish by uploading an artifact") {
			t.Errorf("message should describe the app, not the response: %q", problem.Message)
		}
		if !strings.Contains(problem.Hint, "+release-create") {
			t.Errorf("the hint must name the command that does work: %q", problem.Hint)
		}
	})

	t.Run("values present but no upload key stays a server fault", func(t *testing.T) {
		err := noArtifactUploadError("app_x", 3, "artifact_url")
		var p *errs.Problem
		if errors.As(err, &p) && p.Category == errs.CategoryValidation {
			t.Errorf("a response nobody can act on must not be reported as the caller's precondition: %+v", p)
		}
		if !strings.Contains(err.Error(), "no artifact_url") {
			t.Errorf("the message should say what the response was missing: %v", err)
		}
	})
}
