// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
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
		{"unknown", "html", ""},
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

func TestAppDevInitArgs(t *testing.T) {
	got := appDevInitArgs("react-standard-webapp")
	want := []string{
		"-y", "--prefer-online", "--registry", npmRegistry, miaodaCLIPkg,
		"app", "init", "--template", "react-standard-webapp", "--skip-install",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("appDevInitArgs = %v, want %v", got, want)
	}
}

func TestResolveAppDevDir(t *testing.T) {
	if got := resolveAppDevDir("", "react-standard-webapp"); got != filepath.Join(".", "react-standard-webapp") {
		t.Errorf("default dir = %q", got)
	}
	if got := resolveAppDevDir("./my-app", "react-standard-webapp"); got != "./my-app" {
		t.Errorf("explicit dir = %q", got)
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
	if !strings.Contains(p.Message, "already exists and is not empty") {
		t.Errorf("message = %q", p.Message)
	}
}

func TestReadMetaStack(t *testing.T) {
	dir := t.TempDir()
	if s, ok, err := readMetaStack(dir); s != "" || ok || err != nil {
		t.Errorf("missing meta: got (%q,%v,%v)", s, ok, err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".spark"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, metaRelPath), []byte(`{"stack":"react-standard-webapp","version":"1.0.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s, ok, err := readMetaStack(dir)
	if err != nil || !ok || s != "react-standard-webapp" {
		t.Errorf("got (%q,%v,%v)", s, ok, err)
	}
}

// --- declaration & validate tests ---

func TestAppsAppDevInitTemplate_Declaration(t *testing.T) {
	if AppsAppDevInitTemplate.Command != "+app-dev-init-template" {
		t.Errorf("Command = %q", AppsAppDevInitTemplate.Command)
	}
	if AppsAppDevInitTemplate.Service != appsService {
		t.Errorf("Service = %q", AppsAppDevInitTemplate.Service)
	}
	if AppsAppDevInitTemplate.Risk != "write" {
		t.Errorf("Risk = %q, want write", AppsAppDevInitTemplate.Risk)
	}
	if !AppsAppDevInitTemplate.HasFormat {
		t.Error("HasFormat = false, want true")
	}
	if AppsAppDevInitTemplate.Scopes == nil {
		t.Error("Scopes must be non-nil (no remote API => empty slice)")
	}
}

// testRuntimeAppDevInit builds a RuntimeContext with the type/dir flags
// registered, mirroring how the shortcut reads them via rctx.Str.
func testRuntimeAppDevInit(t *testing.T, appType, dir string) *common.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "+app-dev-init-template"}
	cmd.Flags().String("type", appType, "")
	cmd.Flags().String("dir", dir, "")
	return common.TestNewRuntimeContext(cmd, nil)
}

func TestAppDevInitAppValidate(t *testing.T) {
	tests := []struct {
		name, appType, dir, wantErr string
	}{
		{"missing type", "", "", "--type is required"},
		{"abs dir", "frontend", "/abs", "--dir"},
		{"dotdot dir", "frontend", "../x", "--dir"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := AppsAppDevInitTemplate.Validate(context.Background(), testRuntimeAppDevInit(t, tt.appType, tt.dir))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestAppDevInitAppValidate_NpxMissing(t *testing.T) {
	orig := appDevLookPath
	appDevLookPath = func(string) (string, error) { return "", errors.New("not found") }
	t.Cleanup(func() { appDevLookPath = orig })
	err := AppsAppDevInitTemplate.Validate(context.Background(), testRuntimeAppDevInit(t, "frontend", ""))
	p := requireAppsProblem(t, err, errs.CategoryValidation)
	if p.Subtype != errs.SubtypeFailedPrecondition {
		t.Errorf("subtype = %q, want failed_precondition", p.Subtype)
	}
	if !strings.Contains(p.Hint, "Node.js") {
		t.Errorf("hint = %q, want Node.js install guidance", p.Hint)
	}
}

// --- execute tests (framework runner + fake commandRunner) ---

// relAppDevDir returns a relative, cwd-contained, not-yet-existing directory
// suitable for --dir (mirrors relCloneDir).
func relAppDevDir(t *testing.T) string {
	t.Helper()
	rel := "app-dev-" + strings.ReplaceAll(t.Name(), "/", "_")
	t.Cleanup(func() { os.RemoveAll(rel) })
	return rel
}

func TestAppDevInitAppExecute_DelegatesNpx(t *testing.T) {
	f := &fakeCommandRunner{}
	withFakeRunner(t, f)
	factory, stdout, _ := newAppsExecuteFactory(t)
	dir := relAppDevDir(t)
	if err := runAppsShortcut(t, AppsAppDevInitTemplate,
		[]string{"+app-dev-init-template", "--type", "frontend", "--dir", dir, "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	c := findCall(f.calls, "npx", "-y")
	if c == nil {
		t.Fatalf("npx not invoked: %v", f.calls)
	}
	if !containsAll(c, "-y", "--prefer-online", "--registry", npmRegistry, miaodaCLIPkg,
		"app", "init", "--template", "react-standard-webapp", "--skip-install") {
		t.Errorf("npx args = %v", c)
	}
	if containsAll(c, "--app-id") {
		t.Errorf("app init must NOT carry --app-id in artifact-hosting mode: %v", c)
	}
	if c[0] != dir {
		t.Errorf("npx cwd = %q, want %q", c[0], dir)
	}
	data := parseEnvelopeData(t, stdout)
	if data["dir"] != dir || data["template"] != "react-standard-webapp" {
		t.Errorf("data = %v", data)
	}
	if data["stack"] != "react-standard-webapp" {
		t.Errorf("stack fallback = %v, want template name", data["stack"])
	}
	steps, _ := data["next_steps"].([]interface{})
	if len(steps) != 3 {
		t.Errorf("next_steps = %v, want 3 entries", data["next_steps"])
	}
}

func TestAppDevInitAppExecute_FullStackTemplate(t *testing.T) {
	f := &fakeCommandRunner{}
	withFakeRunner(t, f)
	factory, stdout, _ := newAppsExecuteFactory(t)
	dir := relAppDevDir(t)
	if err := runAppsShortcut(t, AppsAppDevInitTemplate,
		[]string{"+app-dev-init-template", "--type", "full_stack", "--dir", dir, "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if c := findCall(f.calls, "npx", "-y"); c == nil || !containsAll(c, "--template", "react-express-standard-fullstack") {
		t.Errorf("full_stack template not passed: %v", f.calls)
	}
}

func TestAppDevInitAppExecute_MetaStackEcho(t *testing.T) {
	dir := relAppDevDir(t)
	f := &fakeCommandRunner{results: map[string]fakeCallResult{}}
	// Simulate the template producing .spark/meta.json during scaffold.
	f.results["npx -y"] = fakeCallResult{}
	withFakeRunner(t, f)
	factory, stdout, _ := newAppsExecuteFactory(t)
	// Pre-create meta.json via a side channel: the fake runner records but
	// does not write files, so write it before Execute reads it back — the
	// dir must stay empty for ensureAppDevDirUsable, so use a wrapper runner.
	wrapped := &metaWritingRunner{inner: f, dir: dir, stack: "custom-stack"}
	initRunner = wrapped
	if err := runAppsShortcut(t, AppsAppDevInitTemplate,
		[]string{"+app-dev-init-template", "--type", "frontend", "--dir", dir, "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	data := parseEnvelopeData(t, stdout)
	if data["stack"] != "custom-stack" {
		t.Errorf("stack = %v, want custom-stack (from meta.json)", data["stack"])
	}
}

// metaWritingRunner simulates miaoda-cli writing .spark/meta.json into the
// scaffold dir as a side effect of app init.
type metaWritingRunner struct {
	inner *fakeCommandRunner
	dir   string
	stack string
}

func (m *metaWritingRunner) Run(ctx context.Context, dir, name string, args ...string) (string, string, error) {
	if err := os.MkdirAll(filepath.Join(m.dir, ".spark"), 0o755); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(filepath.Join(m.dir, metaRelPath), []byte(`{"stack":"`+m.stack+`"}`), 0o644); err != nil {
		return "", "", err
	}
	return m.inner.Run(ctx, dir, name, args...)
}

func TestAppDevInitAppExecute_NpxFails(t *testing.T) {
	f := &fakeCommandRunner{results: map[string]fakeCallResult{
		"npx -y": {stderr: "boom", err: errors.New("exit 1")},
	}}
	withFakeRunner(t, f)
	factory, stdout, _ := newAppsExecuteFactory(t)
	dir := relAppDevDir(t)
	err := runAppsShortcut(t, AppsAppDevInitTemplate,
		[]string{"+app-dev-init-template", "--type", "frontend", "--dir", dir, "--as", "user"}, factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryInternal)
	if !strings.Contains(p.Message, "npx app init failed") || !strings.Contains(p.Message, "boom") {
		t.Errorf("message = %q", p.Message)
	}
}

func TestAppDevInitAppExecute_DirNotEmpty(t *testing.T) {
	dir := relAppDevDir(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &fakeCommandRunner{}
	withFakeRunner(t, f)
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsAppDevInitTemplate,
		[]string{"+app-dev-init-template", "--type", "frontend", "--dir", dir, "--as", "user"}, factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryValidation)
	if p.Subtype != errs.SubtypeFailedPrecondition {
		t.Errorf("subtype = %q", p.Subtype)
	}
	if len(f.calls) != 0 {
		t.Errorf("npx must not run when dir is not empty: %v", f.calls)
	}
}

func TestAppDevInitAppDryRun(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsAppDevInitTemplate,
		[]string{"+app-dev-init-template", "--type", "frontend", "--as", "user", "--dry-run"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	data, err := decodeDryRunDataMap(stdout.Bytes())
	if err != nil {
		t.Fatalf("decode dry-run: %v (raw=%q)", err, stdout.String())
	}
	cmdLine, _ := data["command"].(string)
	if !strings.Contains(cmdLine, "app init --template react-standard-webapp --skip-install") {
		t.Errorf("command = %q", cmdLine)
	}
	if data["remote_side_effects"] != "none (local scaffold via npx)" {
		t.Errorf("remote_side_effects = %v", data["remote_side_effects"])
	}
	if data["target_dir"] != filepath.Join(".", "react-standard-webapp") {
		t.Errorf("target_dir = %v", data["target_dir"])
	}
	if data["target_dir_state"] != "ok (absent or empty)" {
		t.Errorf("target_dir_state = %v", data["target_dir_state"])
	}
}

func TestAppDevInitAppDryRun_DirNotEmptySurfaced(t *testing.T) {
	dir := relAppDevDir(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsAppDevInitTemplate,
		[]string{"+app-dev-init-template", "--type", "frontend", "--dir", dir, "--as", "user", "--dry-run"}, factory, stdout); err != nil {
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
