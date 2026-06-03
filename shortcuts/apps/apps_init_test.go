// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/shortcuts/common"
)

// testRuntimeWithDir builds a *common.RuntimeContext whose backing cobra command
// has string flags "dir" (=dirFlag) and "template" (=defaultTemplate) registered,
// mirroring how +init reads them at runtime via rctx.Str.
func testRuntimeWithDir(t *testing.T, dirFlag string) *common.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "init"}
	cmd.Flags().String("dir", dirFlag, "")
	cmd.Flags().String("template", defaultTemplate, "")
	return common.TestNewRuntimeContext(cmd, nil)
}

// testRuntimeWithTemplate builds a *common.RuntimeContext with "dir" and
// "template" string flags registered, mirroring +init's runtime flag set. The
// template flag is registered with an empty default (matching the real flag,
// which no longer carries Default: defaultTemplate); pass tpl="" to model an
// omitted --template and a non-empty tpl to model an explicit one.
func testRuntimeWithTemplate(t *testing.T, dirFlag, tpl string) *common.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "init"}
	cmd.Flags().String("dir", dirFlag, "")
	cmd.Flags().String("template", tpl, "")
	return common.TestNewRuntimeContext(cmd, nil)
}

func TestResolveTemplate(t *testing.T) {
	if got := resolveTemplate(testRuntimeWithTemplate(t, "", "foo"), "app_x"); got != "foo" {
		t.Errorf("explicit --template = %q, want foo", got)
	}
	if got := resolveTemplate(testRuntimeWithTemplate(t, "", ""), "app_x"); got != defaultTemplate {
		t.Errorf("omitted --template = %q, want fallback %q", got, defaultTemplate)
	}
	// Whitespace-only --template is treated as omitted -> fallback.
	if got := resolveTemplate(testRuntimeWithTemplate(t, "", "   "), "app_x"); got != defaultTemplate {
		t.Errorf("whitespace --template = %q, want fallback %q", got, defaultTemplate)
	}
}

func TestResolveTargetPath(t *testing.T) {
	got, err := resolveTargetPath(testRuntimeWithDir(t, ""), "app_x")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	want, _ := filepath.Abs(filepath.Join(".", "app_x"))
	if got != want {
		t.Errorf("default dir = %q, want %q", got, want)
	}
	abs := t.TempDir() + "/work"
	if got, err := resolveTargetPath(testRuntimeWithDir(t, abs), "app_x"); err != nil || got != filepath.Clean(abs) {
		t.Errorf("absolute --dir = %q, err=%v; want %q", got, err, filepath.Clean(abs))
	}
	for _, bad := range []string{"bad\tdir", "bad\ndir", "bad\x01dir", "a\rb"} {
		if _, err := resolveTargetPath(testRuntimeWithDir(t, bad), "app_x"); err == nil {
			t.Errorf("control char %q in --dir should be rejected", bad)
		}
	}
}

func TestEnsureEmptyDir_SymlinkRejected(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "real")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if err := ensureEmptyDir(link); err == nil {
		t.Error("symlink target must be rejected")
	}
}

func TestIsAlreadyInitialized(t *testing.T) {
	dir := t.TempDir()
	if isAlreadyInitialized(dir) {
		t.Error("empty dir must not be already-initialized")
	}
	if err := os.MkdirAll(filepath.Join(dir, ".spark"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".spark", "meta.json"), []byte(`{"app_id":"app_y"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if !isAlreadyInitialized(dir) {
		t.Error("dir with .spark/meta.json must be already-initialized (regardless of app_id)")
	}
}

func TestAppsInit_Declaration(t *testing.T) {
	if AppsInit.Command != "+init" {
		t.Errorf("Command = %q, want +init", AppsInit.Command)
	}
	if AppsInit.Service != appsService {
		t.Errorf("Service = %q, want %q", AppsInit.Service, appsService)
	}
	if AppsInit.Risk != "write" {
		t.Errorf("Risk = %q, want write", AppsInit.Risk)
	}
	if !AppsInit.HasFormat {
		t.Error("HasFormat = false, want true")
	}
}

func TestDefaultCloneDir(t *testing.T) {
	got := defaultCloneDir("app_xyz")
	if got != filepath.Join(".", "app_xyz") {
		t.Errorf("defaultCloneDir = %q, want ./app_xyz", got)
	}
}

// --- pure-function tests ---

func TestParseRepoURL(t *testing.T) {
	url, err := parseRepoURLFromEnvelope(`{"ok":true,"data":{"repository_url":"http://u:t@h/app_x.git"}}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "http://u:t@h/app_x.git" {
		t.Errorf("got %q", url)
	}
}

func TestParseRepoURL_Errors(t *testing.T) {
	for _, in := range []string{`not json`, `{"ok":false,"data":{}}`, `{"ok":true,"data":{}}`, `{"ok":true,"data":{"repository_url":""}}`} {
		if _, err := parseRepoURLFromEnvelope(in); err == nil {
			t.Errorf("expected error for %q", in)
		}
	}
}

func TestValidateRepoURLScheme(t *testing.T) {
	for _, ok := range []string{"http://h/r.git", "https://h/r.git"} {
		if err := validateRepoURLScheme(ok); err != nil {
			t.Errorf("%q should be valid: %v", ok, err)
		}
	}
	for _, bad := range []string{"ext::sh -c id", "file:///etc/passwd", "ssh://h/r", "-oProxyCommand=x", "git@h:r"} {
		if err := validateRepoURLScheme(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

// --- orchestration test helpers ---

func withFakeRunner(t *testing.T, f *fakeCommandRunner) {
	t.Helper()
	orig := initRunner
	initRunner = f
	t.Cleanup(func() { initRunner = orig })
}

func credInitOK(repoURL string) fakeCallResult {
	return fakeCallResult{stdout: `{"ok":true,"data":{"repository_url":"` + repoURL + `"}}`}
}

// relCloneDir returns a relative, cwd-contained, not-yet-existing directory
// name suitable for --dir. SafeInputPath rejects absolute paths (so
// t.TempDir() cannot be used directly) and requires the path stay under cwd.
// The fake runner never creates the dir, so ensureEmptyDir sees a missing path
// and passes. Cleanup removes it in case anything materializes it.
func relCloneDir(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	rel := "init-clone-" + strings.ReplaceAll(t.Name(), "/", "_")
	t.Cleanup(func() { os.RemoveAll(filepath.Join(cwd, rel)) })
	return rel
}

// parseEnvelopeData parses the JSON envelope's data object from stdout.
func parseEnvelopeData(t *testing.T, stdout *bytes.Buffer) map[string]interface{} {
	t.Helper()
	var env struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v (raw=%q)", err, stdout.String())
	}
	return env.Data
}

// findCall returns the recorded call whose name (element[1]) and first arg
// (element[2]) match, or nil if none.
func findCall(calls [][]string, name, firstArg string) []string {
	for _, c := range calls {
		if len(c) >= 3 && c[1] == name && c[2] == firstArg {
			return c
		}
	}
	return nil
}

// findCallArg returns the first recorded call whose name (element[1]) matches
// and whose args contain the given ordered subsequence anywhere after the name.
func findCallArg(calls [][]string, name string, wantArgs ...string) []string {
	for _, c := range calls {
		if len(c) < 2 || c[1] != name {
			continue
		}
		args := c[2:]
		i := 0
		for _, a := range args {
			if i < len(wantArgs) && a == wantArgs[i] {
				i++
			}
		}
		if i == len(wantArgs) {
			return c
		}
	}
	return nil
}

func containsAll(call []string, subs ...string) bool {
	set := map[string]bool{}
	for _, c := range call {
		set[c] = true
	}
	for _, s := range subs {
		if !set[s] {
			return false
		}
	}
	return true
}

// --- orchestration tests ---

func TestRunScaffold_EmptyRepo(t *testing.T) {
	// Both a truly empty tree and a tree carrying only the seed README.md count
	// as empty and must take the `app init` path.
	for _, ls := range []string{"", "README.md\n"} {
		t.Run("ls="+ls, func(t *testing.T) {
			f := &fakeCommandRunner{results: map[string]fakeCallResult{"git ls-files": {stdout: ls}}}
			withFakeRunner(t, f)
			kind, err := runScaffold(context.Background(), t.TempDir(), "app_x", "nestjs-react-fullstack")
			if err != nil || kind != "init" {
				t.Fatalf("ls=%q kind=%q err=%v, want init", ls, kind, err)
			}
			c := findCall(f.calls, "npx", miaodaCLIPkg)
			if c == nil || !containsAll(c, "app", "init", "--template", "nestjs-react-fullstack", "--app-id", "app_x") {
				t.Errorf("app init not invoked with expected args: %v", f.calls)
			}
		})
	}
}

func TestRunScaffold_NonEmpty_SyncsWhenNoSteering(t *testing.T) {
	dir := t.TempDir() // no steering dir, no meta.json
	f := &fakeCommandRunner{results: map[string]fakeCallResult{"git ls-files": {stdout: "src/x.ts\n"}}}
	withFakeRunner(t, f)
	kind, err := runScaffold(context.Background(), dir, "app_x", "nestjs-react-fullstack")
	if err != nil || kind != "upgrade" {
		t.Fatalf("kind=%q err=%v, want upgrade", kind, err)
	}
	if findCallArg(f.calls, "npx", "app", "upgrade") == nil {
		t.Error("app upgrade not invoked")
	}
	if findCallArg(f.calls, "npx", "skills", "sync") == nil {
		t.Error("skills sync should run when steering dir absent")
	}
}

func TestRunScaffold_NonEmpty_SkipsSyncWhenSteeringExists(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, steeringRelPath), 0o755)
	f := &fakeCommandRunner{results: map[string]fakeCallResult{"git ls-files": {stdout: "src/x.ts\n"}}}
	withFakeRunner(t, f)
	if _, err := runScaffold(context.Background(), dir, "app_x", "nestjs-react-fullstack"); err != nil {
		t.Fatal(err)
	}
	if findCallArg(f.calls, "npx", "skills", "sync") != nil {
		t.Error("skills sync must be skipped when steering dir exists")
	}
}

func TestRunScaffold_AppInitFailure(t *testing.T) {
	f := &fakeCommandRunner{results: map[string]fakeCallResult{
		"git ls-files":        {stdout: ""},
		"npx " + miaodaCLIPkg: {stderr: "boom", err: errors.New("exit 1")},
	}}
	withFakeRunner(t, f)
	if _, err := runScaffold(context.Background(), t.TempDir(), "app_x", "nestjs-react-fullstack"); err == nil {
		t.Error("app init failure must propagate")
	}
}

func TestAppsInit_EmptyRepo_EndToEnd(t *testing.T) {
	f := &fakeCommandRunner{results: map[string]fakeCallResult{
		"credential-init": credInitOK("http://u:t@h/app_x.git"),
		"git clone":       {},
		"git checkout":    {},
		"git ls-files":    {stdout: ""},                // empty repo -> app init
		"git status":      {stdout: " M src/app.ts\n"}, // scaffold produced changes
	}}
	withFakeRunner(t, f)
	factory, stdout, _ := newAppsExecuteFactory(t)
	dir := relCloneDir(t)
	if err := runAppsShortcut(t, AppsInit, []string{"+init", "--app-id", "app_x", "--dir", dir, "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	data := parseEnvelopeData(t, stdout)
	if data["scaffold"] != "init" {
		t.Errorf("scaffold=%v, want init", data["scaffold"])
	}
	if data["committed"] != true || data["pushed"] != true {
		t.Errorf("committed/pushed = %v/%v, want true/true", data["committed"], data["pushed"])
	}
	if _, ok := data["npx_skipped"]; ok {
		t.Error("npx_skipped must be removed")
	}
	// --template is omitted here, so resolveTemplate falls back to
	// defaultTemplate and `app init` must still receive --template nestjs-react-fullstack.
	c := findCall(f.calls, "npx", miaodaCLIPkg)
	if c == nil {
		t.Error("npx scaffold not invoked")
	} else if !containsAll(c, "app", "init", "--template", defaultTemplate, "--app-id", "app_x") {
		t.Errorf("app init missing expected --template fallback args: %v", c)
	}
}

func TestAppsInit_AlreadyInitialized_ShortCircuit(t *testing.T) {
	dir := relCloneDir(t)
	if err := os.MkdirAll(filepath.Join(dir, ".spark"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, metaRelPath), []byte(`{"app_id":"whatever"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &fakeCommandRunner{results: map[string]fakeCallResult{}}
	withFakeRunner(t, f)
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsInit, []string{"+init", "--app-id", "app_x", "--dir", dir, "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	data := parseEnvelopeData(t, stdout)
	if data["scaffold"] != "already_initialized" {
		t.Errorf("scaffold=%v, want already_initialized", data["scaffold"])
	}
	if len(f.calls) != 0 {
		t.Errorf("no runner calls expected on short-circuit; got %v", f.calls)
	}
}

func TestAppsInit_HappyPathCleanTree(t *testing.T) {
	f := &fakeCommandRunner{results: map[string]fakeCallResult{
		"credential-init": credInitOK("http://u:t@h/app_x.git"),
		"git clone":       {},
		"git checkout":    {},
		"git ls-files":    {stdout: ""}, // empty repo -> app init scaffold
		"git status":      {},           // clean tree after scaffold -> no commit/push
	}}
	withFakeRunner(t, f)
	factory, stdout, _ := newAppsExecuteFactory(t)
	dir := relCloneDir(t)

	err := runAppsShortcut(t, AppsInit, []string{"+init", "--app-id", "app_x", "--dir", dir, "--as", "user"}, factory, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := parseEnvelopeData(t, stdout)
	if data["committed"] != false {
		t.Errorf("committed = %v, want false", data["committed"])
	}
	if data["pushed"] != false {
		t.Errorf("pushed = %v, want false", data["pushed"])
	}
	if data["scaffold"] != "init" {
		t.Errorf("scaffold = %v, want init", data["scaffold"])
	}
	if _, ok := data["npx_skipped"]; ok {
		t.Error("npx_skipped must be removed")
	}
	if data["repository_url"] != "http://***@h/app_x.git" {
		t.Errorf("repository_url = %v, want redacted http://***@h/app_x.git", data["repository_url"])
	}
	clone := findCall(f.calls, "git", "clone")
	if clone == nil {
		t.Fatalf("git clone not recorded; calls=%v", f.calls)
	}
	// clone == [dir, "git", "clone", "--", repoURL, dir]; "--" must precede the URL.
	found := false
	for i := 0; i+1 < len(clone); i++ {
		if clone[i] == "--" && strings.HasPrefix(clone[i+1], "http") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("git clone args missing \"--\" immediately before URL: %v", clone)
	}
}

func TestAppsInit_DirtyTreeCommitPush(t *testing.T) {
	f := &fakeCommandRunner{results: map[string]fakeCallResult{
		"credential-init": credInitOK("http://u:t@h/app_x.git"),
		"git clone":       {},
		"git checkout":    {},
		"git ls-files":    {stdout: "src/x.ts\n"}, // non-empty repo -> app upgrade scaffold
		"git status":      {stdout: " M file.txt"},
	}}
	withFakeRunner(t, f)
	factory, stdout, _ := newAppsExecuteFactory(t)
	dir := relCloneDir(t)

	err := runAppsShortcut(t, AppsInit, []string{"+init", "--app-id", "app_x", "--dir", dir, "--as", "user"}, factory, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if findCall(f.calls, "git", "add") == nil {
		t.Errorf("git add not recorded; calls=%v", f.calls)
	}
	if commit := findCall(f.calls, "git", "commit"); commit == nil {
		t.Errorf("git commit not recorded; calls=%v", f.calls)
	} else if !containsAll(commit, "--no-verify") {
		t.Errorf("git commit missing --no-verify; got %v", commit)
	}
	if findCall(f.calls, "git", "push") == nil {
		t.Errorf("git push not recorded; calls=%v", f.calls)
	}
	data := parseEnvelopeData(t, stdout)
	if data["committed"] != true {
		t.Errorf("committed = %v, want true", data["committed"])
	}
	if data["pushed"] != true {
		t.Errorf("pushed = %v, want true", data["pushed"])
	}
	if data["scaffold"] != "upgrade" {
		t.Errorf("scaffold = %v, want upgrade", data["scaffold"])
	}
}

func TestAppsInit_CredentialInitFailure(t *testing.T) {
	f := &fakeCommandRunner{results: map[string]fakeCallResult{
		"credential-init": {stderr: "boom", err: errors.New("exit 1")},
	}}
	withFakeRunner(t, f)
	factory, stdout, _ := newAppsExecuteFactory(t)
	dir := relCloneDir(t)

	err := runAppsShortcut(t, AppsInit, []string{"+init", "--app-id", "app_x", "--dir", dir, "--as", "user"}, factory, stdout)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if strings.Contains(err.Error(), ":t@") {
		t.Errorf("error leaks token: %v", err)
	}
}

func TestAppsInit_BadRepoURLScheme(t *testing.T) {
	f := &fakeCommandRunner{results: map[string]fakeCallResult{
		"credential-init": credInitOK("ext::sh -c id"),
	}}
	withFakeRunner(t, f)
	factory, stdout, _ := newAppsExecuteFactory(t)
	dir := relCloneDir(t)

	err := runAppsShortcut(t, AppsInit, []string{"+init", "--app-id", "app_x", "--dir", dir, "--as", "user"}, factory, stdout)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if findCall(f.calls, "git", "clone") != nil {
		t.Errorf("git clone should not be recorded for bad scheme; calls=%v", f.calls)
	}
}

func TestAppsInit_CloneFailure(t *testing.T) {
	f := &fakeCommandRunner{results: map[string]fakeCallResult{
		"credential-init": credInitOK("http://u:t@h/r.git"),
		"git clone":       {stderr: "fatal: unable to access 'http://u:t@h/r.git'", err: errors.New("exit 128")},
	}}
	withFakeRunner(t, f)
	factory, stdout, _ := newAppsExecuteFactory(t)
	dir := relCloneDir(t)

	err := runAppsShortcut(t, AppsInit, []string{"+init", "--app-id", "app_x", "--dir", dir, "--as", "user"}, factory, stdout)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if strings.Contains(err.Error(), "u:t@") {
		t.Errorf("error leaks credentials: %v", err)
	}
	if !strings.Contains(err.Error(), "***") {
		t.Errorf("error should be redacted with ***: %v", err)
	}
}

func TestAppsInit_PushFailure(t *testing.T) {
	f := &fakeCommandRunner{results: map[string]fakeCallResult{
		"credential-init": credInitOK("http://u:t@h/app_x.git"),
		"git clone":       {},
		"git checkout":    {},
		"git ls-files":    {stdout: ""},
		"git status":      {stdout: " M file.txt"},
		"git push":        {err: errors.New("exit 1")},
	}}
	withFakeRunner(t, f)
	factory, stdout, _ := newAppsExecuteFactory(t)
	dir := relCloneDir(t)

	err := runAppsShortcut(t, AppsInit, []string{"+init", "--app-id", "app_x", "--dir", dir, "--as", "user"}, factory, stdout)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestAppsInit_DirNonEmpty(t *testing.T) {
	f := &fakeCommandRunner{results: map[string]fakeCallResult{
		"credential-init": credInitOK("http://u:t@h/app_x.git"),
	}}
	withFakeRunner(t, f)
	factory, stdout, _ := newAppsExecuteFactory(t)

	// Create a non-empty directory under cwd (SafeInputPath requires relative,
	// cwd-contained paths), then pass it as --dir.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	nonEmpty, err := os.MkdirTemp(cwd, "init-nonempty-")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(nonEmpty) })
	if err := os.WriteFile(filepath.Join(nonEmpty, "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	err = runAppsShortcut(t, AppsInit, []string{"+init", "--app-id", "app_x", "--dir", filepath.Base(nonEmpty), "--as", "user"}, factory, stdout)
	if err == nil {
		t.Fatalf("expected validation error, got nil")
	}
	if len(f.calls) != 0 {
		t.Errorf("no runner calls expected before dir rejection; calls=%v", f.calls)
	}
}

func TestAppsInit_AsPassthrough(t *testing.T) {
	f := &fakeCommandRunner{results: map[string]fakeCallResult{
		"credential-init": credInitOK("http://u:t@h/app_x.git"),
		"git clone":       {},
		"git checkout":    {},
		"git ls-files":    {stdout: ""},
		"git status":      {},
	}}
	withFakeRunner(t, f)
	factory, stdout, _ := newAppsExecuteFactory(t)
	dir := relCloneDir(t)

	// AppsInit.AuthTypes is ["user"], so the framework rejects --as bot. Use
	// --as user and assert it is forwarded to the self-invoked credential-init.
	err := runAppsShortcut(t, AppsInit, []string{"+init", "--app-id", "app_x", "--dir", dir, "--as", "user"}, factory, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var cred []string
	for _, c := range f.calls {
		if len(c) >= 3 && c[2] == "apps" {
			cred = c
			break
		}
	}
	if cred == nil {
		t.Fatalf("credential-init call not recorded; calls=%v", f.calls)
	}
	hasAs, hasUser := false, false
	for _, a := range cred {
		if a == "--as" {
			hasAs = true
		}
		if a == "user" {
			hasUser = true
		}
	}
	if !hasAs || !hasUser {
		t.Errorf("credential-init args missing --as user: %v", cred)
	}
}

func TestEnsureMetaAppID(t *testing.T) {
	// no meta.json -> no-op, must not create
	dir := t.TempDir()
	if err := ensureMetaAppID(dir, "app_x"); err != nil {
		t.Fatalf("missing meta should be no-op: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, metaRelPath)); !os.IsNotExist(err) {
		t.Error("must not create meta.json when absent")
	}
	// exists without app_id -> add, preserve other fields
	dir2 := t.TempDir()
	os.MkdirAll(filepath.Join(dir2, ".spark"), 0o755)
	os.WriteFile(filepath.Join(dir2, metaRelPath), []byte(`{"name":"keep"}`), 0o644)
	if err := ensureMetaAppID(dir2, "app_x"); err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	b, _ := os.ReadFile(filepath.Join(dir2, metaRelPath))
	json.Unmarshal(b, &m)
	if m["app_id"] != "app_x" || m["name"] != "keep" {
		t.Errorf("merge failed: %v", m)
	}
	// exists with app_id -> untouched
	dir3 := t.TempDir()
	os.MkdirAll(filepath.Join(dir3, ".spark"), 0o755)
	os.WriteFile(filepath.Join(dir3, metaRelPath), []byte(`{"app_id":"orig"}`), 0o644)
	if err := ensureMetaAppID(dir3, "app_x"); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(filepath.Join(dir3, metaRelPath))
	m = nil
	json.Unmarshal(b, &m)
	if m["app_id"] != "orig" {
		t.Errorf("existing app_id overwritten: %v", m)
	}
}

func TestHasSteeringSkills(t *testing.T) {
	dir := t.TempDir()
	if hasSteeringSkills(dir) {
		t.Error("absent steering dir -> false")
	}
	os.MkdirAll(filepath.Join(dir, steeringRelPath), 0o755)
	if !hasSteeringSkills(dir) {
		t.Error("present steering dir -> true")
	}
}

func TestIsEmptyRepo(t *testing.T) {
	cases := []struct {
		name, ls string
		want     bool
	}{
		{"zero files", "", true},
		{"only README.md", "README.md\n", true},
		{"README + business file", "README.md\nsrc/x.ts\n", false},
		{"business file only", "src/x.ts\n", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeCommandRunner{results: map[string]fakeCallResult{"git ls-files": {stdout: c.ls}}}
			withFakeRunner(t, f)
			got, err := isEmptyRepo(context.Background(), t.TempDir())
			if err != nil || got != c.want {
				t.Errorf("ls=%q -> empty=%v err=%v, want %v", c.ls, got, err, c.want)
			}
		})
	}
}
