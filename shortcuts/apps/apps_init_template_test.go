// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// --- pure-function tests ---

func TestAppDevTemplateForType(t *testing.T) {
	tests := []struct {
		name, appType, want string
	}{
		{"frontend", "frontend", "react-standard-webapp"},
		{"full_stack", "full_stack", "react-express-standard-fullstack"},
		{"html", "html", "html-standard-webapp"},
		{"unknown", "vue", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := appDevTemplateForType(tt.appType); got != tt.want {
				t.Errorf("appDevTemplateForType(%q) = %q, want %q", tt.appType, got, tt.want)
			}
		})
	}
}

func TestAppDevTemplatePackageName(t *testing.T) {
	if got := appDevTemplatePackageName("react-standard-webapp"); got != "@lark-apaas/coding-template-react-standard-webapp" {
		t.Errorf("package name = %q", got)
	}
}

func TestResolveAppDevDir(t *testing.T) {
	if got := resolveAppDevDir(""); got != "." {
		t.Errorf("default dir = %q, want . (in-place init)", got)
	}
	if got := resolveAppDevDir("./my-app"); got != "./my-app" {
		t.Errorf("explicit dir = %q", got)
	}
}

func TestAppDevProjectName(t *testing.T) {
	if got := appDevProjectName("./my-app"); got != "my-app" {
		t.Errorf("subdir project name = %q", got)
	}
	// In-place: "." resolves to the real directory name, not ".".
	if got := appDevProjectName("."); got == "." || got == "" {
		t.Errorf("in-place project name = %q, want the cwd base name", got)
	}
}

func TestValidateAppDevDir(t *testing.T) {
	for _, ok := range []string{"", "my-app", "./my-app", "a/b"} {
		if err := validateAppDevDir(ok); err != nil {
			t.Errorf("%q should be valid: %v", ok, err)
		}
	}
	for _, bad := range []string{"/abs", "../x", "a/../../b"} {
		if err := validateAppDevDir(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestEnsureAppDevDirUsable(t *testing.T) {
	dir := t.TempDir()
	if err := ensureAppDevDirUsable(filepath.Join(dir, "missing")); err != nil {
		t.Errorf("missing dir should be usable: %v", err)
	}
	empty := filepath.Join(dir, "empty")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ensureAppDevDirUsable(empty); err != nil {
		t.Errorf("empty dir should be usable: %v", err)
	}
	nonEmpty := filepath.Join(dir, "full")
	if err := os.Mkdir(nonEmpty, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nonEmpty, "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := ensureAppDevDirUsable(nonEmpty)
	if err == nil {
		t.Fatal("non-empty dir must be rejected")
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Subtype != errs.SubtypeFailedPrecondition {
		t.Errorf("want failed_precondition, got %v", err)
	}
}

// --- template tgz test fixture ---

type tgzEntry struct {
	name     string
	body     string
	typeflag byte
	linkname string
}

// buildTemplateTgz assembles an npm-style template tarball in memory.
func buildTemplateTgz(t *testing.T, entries []tgzEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		tf := e.typeflag
		if tf == 0 {
			tf = tar.TypeReg
		}
		hdr := &tar.Header{Name: e.name, Mode: 0o644, Size: int64(len(e.body)), Typeflag: tf, Linkname: e.linkname}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if tf == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func defaultTemplateEntries() []tgzEntry {
	return []tgzEntry{
		{name: "package/package.json", body: `{"name":"@lark-apaas/coding-template-react-standard-webapp","version":"1.2.3","miaodaTemplate":{"archType":2}}`},
		{name: "package/template/index.html", body: "<title>{{projectName}}</title>"},
		{name: "package/template/README.md", body: "# {{projectName}}"},
		{name: "package/template/src/App.tsx", body: "export default 1"},
		{name: "package/template/_gitignore", body: "node_modules\n"},
		{name: "package/template/_npmrc", body: "registry=x\n"},
		{name: "package/README.md", body: "pkg readme, not extracted"},
	}
}

// withFakeRegistry starts a TLS registry server that serves metadata + tarball
// for pkg, and points appDevRegistryBase / appDevNewTransferClient at it.
func withFakeRegistry(t *testing.T, pkg string, tgz []byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/"+pkg, func(w http.ResponseWriter, _ *http.Request) {
		meta := map[string]interface{}{
			"dist-tags": map[string]string{"latest": "1.2.3", "alpha": "2.0.0-alpha.1"},
			"versions": map[string]interface{}{
				"1.2.3": map[string]interface{}{
					"dist": map[string]string{"tarball": srv.URL + "/tarball.tgz"},
				},
				"2.0.0-alpha.1": map[string]interface{}{
					"dist": map[string]string{"tarball": srv.URL + "/tarball.tgz"},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(meta)
	})
	mux.HandleFunc("/tarball.tgz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tgz)
	})
	srv = httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	origRegs, origClient := appDevRegistries, appDevNewTransferClient
	appDevRegistries = []string{srv.URL}
	appDevNewTransferClient = func() *http.Client { return srv.Client() }
	t.Cleanup(func() { appDevRegistries, appDevNewTransferClient = origRegs, origClient })
	return srv
}

func TestFetchAppDevTemplate_PinnedVersionAndTag(t *testing.T) {
	pkg := "@lark-apaas/coding-template-react-standard-webapp"
	withFakeRegistry(t, pkg, buildTemplateTgz(t, defaultTemplateEntries()))
	// dist-tag resolution.
	v, _, err := fetchAppDevTemplate(context.Background(), pkg, "alpha", nil, nil)
	if err != nil || v != "2.0.0-alpha.1" {
		t.Errorf("dist-tag pin: v=%q err=%v", v, err)
	}
	// Exact version resolution.
	v, _, err = fetchAppDevTemplate(context.Background(), pkg, "1.2.3", nil, nil)
	if err != nil || v != "1.2.3" {
		t.Errorf("exact pin: v=%q err=%v", v, err)
	}
	// Unknown version: actionable error listing dist-tags.
	_, _, err = fetchAppDevTemplate(context.Background(), pkg, "9.9.9", nil, nil)
	if err == nil || !strings.Contains(err.Error(), `no version or dist-tag "9.9.9"`) {
		t.Errorf("unknown pin: err=%v", err)
	}
}

// --- render tests ---

func TestRenderAppDevTemplate(t *testing.T) {
	dir := t.TempDir()
	rendered, err := renderAppDevTemplate(dir, "my-app", buildTemplateTgz(t, defaultTemplateEntries()))
	if err != nil {
		t.Fatal(err)
	}
	if rendered.Files != 5 {
		t.Errorf("Files = %d, want 5 (template subtree only)", rendered.Files)
	}
	// Placeholder replaced.
	b, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	if string(b) != "<title>my-app</title>" {
		t.Errorf("index.html = %q", b)
	}
	// Renames applied.
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); err != nil {
		t.Error("_gitignore must be renamed to .gitignore")
	}
	if _, err := os.Stat(filepath.Join(dir, ".npmrc")); err != nil {
		t.Error("_npmrc must be renamed to .npmrc")
	}
	if _, err := os.Stat(filepath.Join(dir, "_gitignore")); !os.IsNotExist(err) {
		t.Error("_gitignore placeholder must not remain")
	}
	// Non-template pkg files not extracted.
	if _, err := os.Stat(filepath.Join(dir, "package")); !os.IsNotExist(err) {
		t.Error("files outside package/template/ must not be extracted")
	}
	// Nested file extracted.
	if _, err := os.Stat(filepath.Join(dir, "src", "App.tsx")); err != nil {
		t.Error("nested template file missing")
	}
}

func TestRenderAppDevTemplate_RejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	tgz := buildTemplateTgz(t, []tgzEntry{
		{name: "package/template/../../evil.txt", body: "x"},
	})
	if _, err := renderAppDevTemplate(dir, "p", tgz); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Errorf("traversal entry must be rejected, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "evil.txt")); !os.IsNotExist(err) {
		t.Error("traversal file must not be written")
	}
}

func TestRenderAppDevTemplate_SkipsSymlinks(t *testing.T) {
	dir := t.TempDir()
	tgz := buildTemplateTgz(t, []tgzEntry{
		{name: "package/template/link", typeflag: tar.TypeSymlink, linkname: "/etc/passwd"},
		{name: "package/template/index.html", body: "ok"},
	})
	rendered, err := renderAppDevTemplate(dir, "p", tgz)
	if err != nil {
		t.Fatal(err)
	}
	if rendered.Files != 1 {
		t.Errorf("Files = %d, want 1 (symlink skipped)", rendered.Files)
	}
	if _, err := os.Lstat(filepath.Join(dir, "link")); !os.IsNotExist(err) {
		t.Error("symlink must not be materialized")
	}
}

func TestRenderAppDevTemplate_ExtractCap(t *testing.T) {
	orig := appDevMaxTemplateExtractBytes
	appDevMaxTemplateExtractBytes = 4
	t.Cleanup(func() { appDevMaxTemplateExtractBytes = orig })
	tgz := buildTemplateTgz(t, []tgzEntry{
		{name: "package/template/big.txt", body: "0123456789"},
	})
	if _, err := renderAppDevTemplate(t.TempDir(), "p", tgz); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("extract cap must reject, got %v", err)
	}
}

func TestRenderAppDevTemplate_FileCountCap(t *testing.T) {
	orig := appDevMaxTemplateFiles
	appDevMaxTemplateFiles = 1
	t.Cleanup(func() { appDevMaxTemplateFiles = orig })
	tgz := buildTemplateTgz(t, []tgzEntry{
		{name: "package/template/a.txt", body: "a"},
		{name: "package/template/b.txt", body: "b"},
	})
	if _, err := renderAppDevTemplate(t.TempDir(), "p", tgz); err == nil || !strings.Contains(err.Error(), "more than") {
		t.Errorf("file count cap must reject, got %v", err)
	}
}

func TestWriteMiaodaScaffoldFields(t *testing.T) {
	dir := t.TempDir()
	// Fresh project: stack + version stamped.
	if err := writeMiaodaScaffoldFields(dir, "react-standard-webapp", "1.2.3"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, miaodaJSONRelPath))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["stack"] != "react-standard-webapp" || doc["version"] != "1.2.3" {
		t.Errorf("doc = %v", doc)
	}
	// Seed-shipped declarations are preserved; seed stack wins; version is
	// re-stamped with the rendered package version.
	seed := `{"stack":"seed-stack","version":"0.0.1","build":{"command":["make","dist"],"output":"out"},"dev":{"port":5173}}`
	if err := os.WriteFile(filepath.Join(dir, miaodaJSONRelPath), []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeMiaodaScaffoldFields(dir, "react-standard-webapp", "2.0.0"); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(filepath.Join(dir, miaodaJSONRelPath))
	doc = map[string]interface{}{}
	_ = json.Unmarshal(b, &doc)
	if doc["stack"] != "seed-stack" {
		t.Errorf("seed stack must not be overwritten, got %v", doc["stack"])
	}
	if doc["version"] != "2.0.0" {
		t.Errorf("version must be re-stamped, got %v", doc["version"])
	}
	if doc["build"] == nil || doc["dev"] == nil {
		t.Errorf("seed declarations must be preserved: %v", doc)
	}
}

// --- fetch tests ---

func TestFetchAppDevTemplateMeta_RejectsNonHTTPSTarball(t *testing.T) {
	pkg := "@lark-apaas/coding-template-react-standard-webapp"
	mux := http.NewServeMux()
	mux.HandleFunc("/"+pkg, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"dist-tags":{"latest":"1.0.0"},"versions":{"1.0.0":{"dist":{"tarball":"http://insecure.example/t.tgz"}}}}`))
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	origRegs, origClient := appDevRegistries, appDevNewTransferClient
	appDevRegistries = []string{srv.URL}
	appDevNewTransferClient = func() *http.Client { return srv.Client() }
	t.Cleanup(func() { appDevRegistries, appDevNewTransferClient = origRegs, origClient })

	_, _, err := fetchAppDevTemplateMeta(context.Background(), srv.URL, pkg, "")
	if err == nil || !strings.Contains(err.Error(), "not https") {
		t.Errorf("non-https tarball must be rejected, got %v", err)
	}
}

func TestFetchAppDevTemplateMeta_RejectsCrossHostTarball(t *testing.T) {
	pkg := "@lark-apaas/coding-template-react-standard-webapp"
	mux := http.NewServeMux()
	mux.HandleFunc("/"+pkg, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"dist-tags":{"latest":"1.0.0"},"versions":{"1.0.0":{"dist":{"tarball":"https://evil.example/t.tgz"}}}}`))
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	origRegs, origClient := appDevRegistries, appDevNewTransferClient
	appDevRegistries = []string{srv.URL}
	appDevNewTransferClient = func() *http.Client { return srv.Client() }
	t.Cleanup(func() { appDevRegistries, appDevNewTransferClient = origRegs, origClient })

	_, _, err := fetchAppDevTemplateMeta(context.Background(), srv.URL, pkg, "")
	if err == nil || !strings.Contains(err.Error(), "differs from registry host") {
		t.Errorf("cross-host tarball must be rejected, got %v", err)
	}
}

func TestRenderAppDevTemplate_RejectsBackslashEntry(t *testing.T) {
	tgz := buildTemplateTgz(t, []tgzEntry{
		{name: `package/template/..\evil.txt`, body: "x"},
	})
	if _, err := renderAppDevTemplate(t.TempDir(), "p", tgz); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Errorf("backslash entry must be rejected, got %v", err)
	}
}

func TestFetchAppDevTemplateMeta_404(t *testing.T) {
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	origRegs, origClient := appDevRegistries, appDevNewTransferClient
	appDevRegistries = []string{srv.URL}
	appDevNewTransferClient = func() *http.Client { return srv.Client() }
	t.Cleanup(func() { appDevRegistries, appDevNewTransferClient = origRegs, origClient })

	_, _, err := fetchAppDevTemplateMeta(context.Background(), srv.URL, "@lark-apaas/coding-template-x", "")
	p := requireAppsProblem(t, err, errs.CategoryNetwork)
	if !strings.Contains(p.Hint, "not be published") {
		t.Errorf("404 hint = %q", p.Hint)
	}
}

// --- registry fallback tests ---

// newFailingThenOKRegistries starts two TLS servers: the first responds with
// failStatus for everything, the second serves pkg + tarball normally, and
// wires appDevRegistries = [failing, ok].
func newFailingThenOKRegistries(t *testing.T, pkg string, tgz []byte, failStatus int) {
	t.Helper()
	failing := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(failStatus)
	}))
	t.Cleanup(failing.Close)
	mux := http.NewServeMux()
	var okSrv *httptest.Server
	mux.HandleFunc("/"+pkg, func(w http.ResponseWriter, _ *http.Request) {
		meta := map[string]interface{}{
			"dist-tags": map[string]string{"latest": "1.2.3"},
			"versions": map[string]interface{}{
				"1.2.3": map[string]interface{}{
					"dist": map[string]string{"tarball": okSrv.URL + "/tarball.tgz"},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(meta)
	})
	mux.HandleFunc("/tarball.tgz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(tgz) })
	okSrv = httptest.NewTLSServer(mux)
	t.Cleanup(okSrv.Close)
	origRegs, origClient := appDevRegistries, appDevNewTransferClient
	appDevRegistries = []string{failing.URL, okSrv.URL}
	appDevNewTransferClient = func() *http.Client { return okSrv.Client() }
	t.Cleanup(func() { appDevRegistries, appDevNewTransferClient = origRegs, origClient })
}

func TestFetchAppDevTemplate_FallbackOn5xx(t *testing.T) {
	pkg := "@lark-apaas/coding-template-react-standard-webapp"
	newFailingThenOKRegistries(t, pkg, buildTemplateTgz(t, defaultTemplateEntries()), 503)
	var notes []string
	version, tgz, err := fetchAppDevTemplate(context.Background(), pkg, "", nil, func(n string) { notes = append(notes, n) })
	if err != nil {
		t.Fatalf("fallback should succeed: %v", err)
	}
	if version != "1.2.3" || len(tgz) == 0 {
		t.Errorf("version=%q len=%d", version, len(tgz))
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "falling back to") {
		t.Errorf("fallback note = %v", notes)
	}
}

func TestFetchAppDevTemplate_FallbackOn404(t *testing.T) {
	// A freshly published package may not have synced to the mirror yet —
	// 404 on the primary must also fall through to the official registry.
	pkg := "@lark-apaas/coding-template-react-standard-webapp"
	newFailingThenOKRegistries(t, pkg, buildTemplateTgz(t, defaultTemplateEntries()), 404)
	version, _, err := fetchAppDevTemplate(context.Background(), pkg, "", nil, nil)
	if err != nil || version != "1.2.3" {
		t.Errorf("404 fallback: version=%q err=%v", version, err)
	}
}

func TestFetchAppDevTemplate_AllRegistriesFail(t *testing.T) {
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	origRegs, origClient := appDevRegistries, appDevNewTransferClient
	appDevRegistries = []string{srv.URL, srv.URL}
	appDevNewTransferClient = func() *http.Client { return srv.Client() }
	t.Cleanup(func() { appDevRegistries, appDevNewTransferClient = origRegs, origClient })

	_, _, err := fetchAppDevTemplate(context.Background(), "@lark-apaas/coding-template-x", "", nil, nil)
	if err == nil {
		t.Fatal("all-fail must error")
	}
	p, _ := errs.ProblemOf(err)
	if p == nil || !strings.Contains(p.Hint, "not be published") {
		t.Errorf("hint = %v", p)
	}
}

func TestRenderAppDevTemplate_SkippedEntryBombCap(t *testing.T) {
	// A huge entry OUTSIDE package/template/ is skipped by the walk, but its
	// decompressed bytes still stream through the counter and must trip the
	// cap (gzip-bomb defense for skipped entries).
	orig := appDevMaxTemplateExtractBytes
	appDevMaxTemplateExtractBytes = 64
	t.Cleanup(func() { appDevMaxTemplateExtractBytes = orig })
	tgz := buildTemplateTgz(t, []tgzEntry{
		{name: "package/ignored-bomb.bin", body: strings.Repeat("0", 4096)},
		{name: "package/template/index.html", body: "ok"},
	})
	if _, err := renderAppDevTemplate(t.TempDir(), "p", tgz); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("skipped-entry bomb must trip the cap, got %v", err)
	}
}

// --- declaration & validate tests ---

func TestAppsInitTemplate_Declaration(t *testing.T) {
	if AppsInitTemplate.Command != "+init-template" {
		t.Errorf("Command = %q", AppsInitTemplate.Command)
	}
	if AppsInitTemplate.Service != appsService {
		t.Errorf("Service = %q", AppsInitTemplate.Service)
	}
	if AppsInitTemplate.Risk != "write" {
		t.Errorf("Risk = %q, want write", AppsInitTemplate.Risk)
	}
	if !AppsInitTemplate.HasFormat {
		t.Error("HasFormat = false, want true")
	}
	if AppsInitTemplate.Scopes == nil {
		t.Error("Scopes must be non-nil (no Lark API => empty slice)")
	}
}

func testRuntimeAppDevInit(t *testing.T, appType, dir string) *common.RuntimeContext {
	t.Helper()
	return testRuntimeAppDevInitTpl(t, appType, "", dir)
}

func testRuntimeAppDevInitTpl(t *testing.T, appType, template, dir string) *common.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "+init-template"}
	cmd.Flags().String("type", appType, "")
	cmd.Flags().String("template", template, "")
	cmd.Flags().String("template-version", "", "")
	cmd.Flags().String("registry", "", "")
	cmd.Flags().String("dir", dir, "")
	return common.TestNewRuntimeContext(cmd, nil)
}

func TestResolveAppDevRegistries(t *testing.T) {
	rctxWith := func(registry string) *common.RuntimeContext {
		cmd := &cobra.Command{Use: "+init-template"}
		cmd.Flags().String("registry", registry, "")
		return common.TestNewRuntimeContext(cmd, nil)
	}
	// Unset: nil selects the built-in fallback chain.
	regs, err := resolveAppDevRegistries(rctxWith(""))
	if err != nil || regs != nil {
		t.Errorf("unset = (%v, %v), want (nil, nil)", regs, err)
	}
	// Explicit https URL: single entry, trailing slash trimmed.
	regs, err = resolveAppDevRegistries(rctxWith("https://bnpm.example.com/"))
	if err != nil || len(regs) != 1 || regs[0] != "https://bnpm.example.com" {
		t.Errorf("explicit = (%v, %v)", regs, err)
	}
	// http and bare hosts are rejected.
	for _, bad := range []string{"http://registry.npmjs.org", "registry.npmjs.org", "ftp://x"} {
		if _, err := resolveAppDevRegistries(rctxWith(bad)); err == nil || !strings.Contains(err.Error(), "https") {
			t.Errorf("registry %q must be rejected with an https hint, got %v", bad, err)
		}
	}
}

func TestFetchAppDevTemplate_ExplicitRegistry(t *testing.T) {
	pkg := "@lark-apaas/coding-template-react-standard-webapp"
	srv := withFakeRegistry(t, pkg, buildTemplateTgz(t, defaultTemplateEntries()))
	// An explicit registry pointing at the fake server works.
	v, _, err := fetchAppDevTemplate(context.Background(), pkg, "", []string{srv.URL}, nil)
	if err != nil || v != "1.2.3" {
		t.Errorf("explicit registry fetch = (%q, %v)", v, err)
	}
	// An explicit dead registry must fail deterministically — never fall
	// back to the built-in chain (which points at the working fake here).
	var notes []string
	_, _, err = fetchAppDevTemplate(context.Background(), pkg, "", []string{"https://127.0.0.1:1"},
		func(n string) { notes = append(notes, n) })
	if err == nil {
		t.Fatal("dead explicit registry must fail, not fall back")
	}
	if len(notes) != 0 {
		t.Errorf("no fallback notes expected for a single explicit registry, got %v", notes)
	}
}

func TestAppDevInitTemplateValidate(t *testing.T) {
	tests := []struct {
		name, appType, dir, wantErr string
	}{
		{"missing type and template", "", "", "--type or --template is required"},
		{"abs dir", "frontend", "/abs", "--dir"},
		{"dotdot dir", "frontend", "../x", "--dir"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := AppsInitTemplate.Validate(context.Background(), testRuntimeAppDevInit(t, tt.appType, tt.dir))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestResolveAppDevTemplate(t *testing.T) {
	// template-first: explicit --template wins over --type.
	tpl, err := resolveAppDevTemplate(testRuntimeAppDevInitTpl(t, "frontend", "vite-react", ""))
	if err != nil || tpl != "vite-react" {
		t.Errorf("template-first: got (%q, %v)", tpl, err)
	}
	// --type mapping still works without --template.
	tpl, err = resolveAppDevTemplate(testRuntimeAppDevInitTpl(t, "full_stack", "", ""))
	if err != nil || tpl != "react-express-standard-fullstack" {
		t.Errorf("type mapping: got (%q, %v)", tpl, err)
	}
	// Unsafe template names are rejected (they splice into URL/package name).
	for _, bad := range []string{"../evil", "a/b", "@scope/x", "UPPER", "-lead", "x y"} {
		if _, err := resolveAppDevTemplate(testRuntimeAppDevInitTpl(t, "", bad, "")); err == nil {
			t.Errorf("template %q should be rejected", bad)
		}
	}
}

// --- execute tests (framework runner + fake registry) ---

// relAppDevDir returns a relative, cwd-contained, not-yet-existing directory
// suitable for --dir (mirrors relCloneDir).
func relAppDevDir(t *testing.T) string {
	t.Helper()
	rel := "app-dev-" + strings.ReplaceAll(t.Name(), "/", "_")
	t.Cleanup(func() { os.RemoveAll(rel) })
	return rel
}

func TestAppDevInitTemplateExecute_RendersFromRegistry(t *testing.T) {
	pkg := "@lark-apaas/coding-template-react-standard-webapp"
	withFakeRegistry(t, pkg, buildTemplateTgz(t, defaultTemplateEntries()))
	factory, stdout, _ := newAppsExecuteFactory(t)
	dir := relAppDevDir(t)
	if err := runAppsShortcut(t, AppsInitTemplate,
		[]string{"+init-template", "--type", "frontend", "--dir", dir, "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	data := parseEnvelopeData(t, stdout)
	if data["dir"] != dir || data["template"] != "react-standard-webapp" || data["version"] != "1.2.3" {
		t.Errorf("data = %v", data)
	}
	if data["files"] != float64(5) {
		t.Errorf("files = %v", data["files"])
	}
	// Rendered content on disk.
	b, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil || !strings.Contains(string(b), dir) {
		t.Errorf("index.html placeholder = %q err=%v (projectName is dir basename)", b, err)
	}
	// miaoda.json written by lark-cli (protocol §3).
	mb, err := os.ReadFile(filepath.Join(dir, miaodaJSONRelPath))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]interface{}
	_ = json.Unmarshal(mb, &doc)
	if doc["stack"] != "react-standard-webapp" || doc["version"] != "1.2.3" {
		t.Errorf("miaoda.json = %v", doc)
	}
	steps, _ := data["next_steps"].([]interface{})
	if len(steps) != 3 {
		t.Errorf("next_steps = %v", data["next_steps"])
	}
}

func TestAppDevInitTemplateExecute_FullStackPackage(t *testing.T) {
	pkg := "@lark-apaas/coding-template-react-express-standard-fullstack"
	withFakeRegistry(t, pkg, buildTemplateTgz(t, []tgzEntry{
		{name: "package/package.json", body: `{"miaodaTemplate":{"archType":1}}`},
		{name: "package/template/index.html", body: "fs"},
	}))
	factory, stdout, _ := newAppsExecuteFactory(t)
	dir := relAppDevDir(t)
	if err := runAppsShortcut(t, AppsInitTemplate,
		[]string{"+init-template", "--type", "full_stack", "--dir", dir, "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	data := parseEnvelopeData(t, stdout)
	if data["template"] != "react-express-standard-fullstack" {
		t.Errorf("template = %v", data["template"])
	}
}

func TestAppDevInitTemplateExecute_ExplicitTemplate(t *testing.T) {
	pkg := "@lark-apaas/coding-template-vite-react"
	withFakeRegistry(t, pkg, buildTemplateTgz(t, []tgzEntry{
		{name: "package/package.json", body: `{"miaodaTemplate":{"archType":2}}`},
		{name: "package/template/index.html", body: "tpl"},
	}))
	factory, stdout, _ := newAppsExecuteFactory(t)
	dir := relAppDevDir(t)
	if err := runAppsShortcut(t, AppsInitTemplate,
		[]string{"+init-template", "--template", "vite-react", "--dir", dir, "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	data := parseEnvelopeData(t, stdout)
	if data["template"] != "vite-react" || data["stack"] != "vite-react" {
		t.Errorf("data = %v", data)
	}
}

func TestAppDevInitTemplateExecute_RegistryDown(t *testing.T) {
	pkg := "@lark-apaas/coding-template-react-standard-webapp"
	mux := http.NewServeMux()
	mux.HandleFunc("/"+pkg, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) })
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	origRegs, origClient := appDevRegistries, appDevNewTransferClient
	appDevRegistries = []string{srv.URL}
	appDevNewTransferClient = func() *http.Client { return srv.Client() }
	t.Cleanup(func() { appDevRegistries, appDevNewTransferClient = origRegs, origClient })

	factory, stdout, _ := newAppsExecuteFactory(t)
	dir := relAppDevDir(t)
	err := runAppsShortcut(t, AppsInitTemplate,
		[]string{"+init-template", "--type", "frontend", "--dir", dir, "--as", "user"}, factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryNetwork)
	if !p.Retryable {
		t.Error("registry 5xx must be retryable")
	}
	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Error("target dir must not be created when the fetch fails")
	}
}

func TestAppDevInitTemplateExecute_DirNotEmpty(t *testing.T) {
	dir := relAppDevDir(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsInitTemplate,
		[]string{"+init-template", "--type", "frontend", "--dir", dir, "--as", "user"}, factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryValidation)
	if p.Subtype != errs.SubtypeFailedPrecondition {
		t.Errorf("subtype = %q", p.Subtype)
	}
}

func TestAppDevInitTemplateDryRun(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsInitTemplate,
		[]string{"+init-template", "--type", "frontend", "--as", "user", "--dry-run"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	data, err := decodeDryRunDataMap(stdout.Bytes())
	if err != nil {
		t.Fatalf("decode dry-run: %v (raw=%q)", err, stdout.String())
	}
	if data["template_package"] != "@lark-apaas/coding-template-react-standard-webapp" {
		t.Errorf("template_package = %v", data["template_package"])
	}
	if data["remote_side_effects"] != "read-only npm registry download, no Lark API" {
		t.Errorf("remote_side_effects = %v", data["remote_side_effects"])
	}
	if data["target_dir"] != "." {
		t.Errorf("target_dir = %v, want . (in-place default)", data["target_dir"])
	}
	// The test cwd (package dir) is non-empty, so the in-place default must
	// surface as not usable in dry-run.
	state, _ := data["target_dir_state"].(string)
	if !strings.Contains(state, "not usable") || !strings.Contains(state, "current directory is not empty") {
		t.Errorf("target_dir_state = %q", state)
	}
}

func TestAppDevInitTemplateDryRun_DirNotEmptySurfaced(t *testing.T) {
	dir := relAppDevDir(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsInitTemplate,
		[]string{"+init-template", "--type", "frontend", "--dir", dir, "--as", "user", "--dry-run"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	data, err := decodeDryRunDataMap(stdout.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	state, _ := data["target_dir_state"].(string)
	if !strings.Contains(state, "not usable") {
		t.Errorf("target_dir_state = %q, want non-empty dir surfaced", state)
	}
}
