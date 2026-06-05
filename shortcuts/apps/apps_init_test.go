// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
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
			c := findCall(f.calls, "npx", "-y")
			if c == nil || !containsAll(c, "-y", "--prefer-online", miaodaCLIPkg, "app", "init", "--template", "nestjs-react-fullstack", "--app-id", "app_x") {
				t.Errorf("app init not invoked with expected args: %v", f.calls)
			}
			if c != nil && containsAll(c, "--local") {
				t.Errorf("app init must NOT carry --local: %v", c)
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
	if c := findCallArg(f.calls, "npx", "app", "sync"); c == nil || !containsAll(c, "-y", "--prefer-online") {
		t.Error("app sync not invoked with --prefer-online")
	} else if containsAll(c, "--local") {
		t.Errorf("app sync must NOT carry --local: %v", c)
	}
	if c := findCallArg(f.calls, "npx", "skills", "sync"); c == nil || !containsAll(c, "-y", "--prefer-online", "--local") {
		t.Error("skills sync should run with --prefer-online and --local when steering dir absent")
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
		"git ls-files": {stdout: ""},
		"npx -y":       {stderr: "boom", err: errors.New("exit 1")},
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
	c := findCall(f.calls, "npx", "-y")
	if c == nil {
		t.Error("npx scaffold not invoked")
	} else if !containsAll(c, "-y", "--prefer-online", miaodaCLIPkg, "app", "init", "--template", defaultTemplate, "--app-id", "app_x") {
		t.Errorf("app init missing expected --template fallback args: %v", c)
	} else if containsAll(c, "--local") {
		t.Errorf("app init must NOT carry --local: %v", c)
	}
}

func TestAppsInit_AlreadyInitialized_ShortCircuit(t *testing.T) {
	dir := relCloneDir(t)
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".spark"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, metaRelPath), []byte(`{"app_id":"whatever"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &fakeCommandRunner{results: map[string]fakeCallResult{"env-pull": envPullOK(filepath.Join(abs, ".env.local"))}}
	withFakeRunner(t, f)
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsInit, []string{"+init", "--app-id", "app_x", "--dir", dir, "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	data := parseEnvelopeData(t, stdout)
	if data["scaffold"] != "already_initialized" {
		t.Errorf("scaffold=%v, want already_initialized", data["scaffold"])
	}
	// short-circuit must still skip clone/checkout/scaffold/commit ...
	for _, c := range f.calls {
		if containsAll(c, "git", "clone") || containsAll(c, "git", "checkout") || containsAll(c, "git", "status") {
			t.Errorf("short-circuit must not run git clone/checkout/status; got %v", f.calls)
		}
	}
	// ... but now refreshes local env exactly once.
	envPullCalls := 0
	for _, c := range f.calls {
		if containsAll(c, "+env-pull") {
			envPullCalls++
		}
	}
	if envPullCalls != 1 {
		t.Errorf("short-circuit must call +env-pull exactly once; got %d (%v)", envPullCalls, f.calls)
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
		"git ls-files":    {stdout: "src/x.ts\n"}, // non-empty repo -> app sync scaffold
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

// newAppsExecuteFactoryWithStderr mirrors newAppsExecuteFactory but also returns
// the stderr buffer, so tests can assert on the +init progress log lines that
// initLogf writes to IO().ErrOut.
func newAppsExecuteFactoryWithStderr(t *testing.T) (*cmdutil.Factory, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	cfg := &core.CliConfig{
		AppID:      "test-app-" + strings.ToLower(t.Name()),
		AppSecret:  "test-secret",
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_test",
	}
	factory, stdout, stderr, _ := cmdutil.TestFactory(t, cfg)
	return factory, stdout, stderr
}

func TestAppsInit_Req1_Wording(t *testing.T) {
	var tmpl *common.Flag
	for i := range AppsInit.Flags {
		if AppsInit.Flags[i].Name == "template" {
			tmpl = &AppsInit.Flags[i]
		}
	}
	if tmpl == nil {
		t.Fatal("--template flag missing")
	}
	if strings.Contains(strings.ToLower(tmpl.Desc), "scaffold") {
		t.Errorf("--template Desc still mentions scaffold: %q", tmpl.Desc)
	}
	if !strings.Contains(strings.ToLower(tmpl.Desc), "code-init") {
		t.Errorf("--template Desc should use code-init wording: %q", tmpl.Desc)
	}

	// The --dry-run output is a flat object (DryRunAPI marshals to top-level keys
	// description/scaffold/api/...), NOT wrapped in {"data":...}, so parse stdout
	// directly rather than via parseEnvelopeData.
	factory, stdout, _ := newAppsExecuteFactoryWithStderr(t)
	if err := runAppsShortcut(t, AppsInit, []string{"+init", "--app-id", "app_x", "--as", "user", "--dry-run"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &data); err != nil {
		t.Fatalf("decode dry-run output: %v (raw=%q)", err, stdout.String())
	}
	desc, _ := data["description"].(string)
	if strings.Contains(strings.ToLower(desc), "scaffold") {
		t.Errorf("dry-run description still mentions scaffold: %q", desc)
	}
	scaffold, ok := data["scaffold"].(string)
	if !ok {
		t.Error("dry-run must keep machine-contract key `scaffold`")
	} else if !strings.Contains(scaffold, "skills sync --local") {
		t.Errorf("dry-run scaffold string must show --local on skills sync: %q", scaffold)
	} else if strings.Contains(scaffold, "app init --template nestjs-react-fullstack --app-id app_x --local") ||
		strings.Contains(scaffold, "app sync --local") {
		t.Errorf("dry-run scaffold string must NOT show --local on app init / app sync: %q", scaffold)
	}

	f := &fakeCommandRunner{results: map[string]fakeCallResult{
		"credential-init": credInitOK("http://u:t@h/app_x.git"),
		"git clone":       {},
		"git checkout":    {},
		"git ls-files":    {stdout: ""},
		"git status":      {},
	}}
	withFakeRunner(t, f)
	factory2, stdout2, stderr2 := newAppsExecuteFactoryWithStderr(t)
	dir := relCloneDir(t)
	if err := runAppsShortcut(t, AppsInit, []string{"+init", "--app-id", "app_x", "--dir", dir, "--as", "user"}, factory2, stdout2); err != nil {
		t.Fatalf("run err=%v", err)
	}
	if strings.Contains(stderr2.String(), "Scaffolding") {
		t.Errorf("progress log still says Scaffolding: %q", stderr2.String())
	}
	if !strings.Contains(stderr2.String(), "Initializing app code") {
		t.Errorf("progress log should say 'Initializing app code': %q", stderr2.String())
	}
}

func TestClassifyPorcelain(t *testing.T) {
	cases := []struct {
		name, status            string
		wantAppCode, wantConfig bool
	}{
		{"empty", "", false, false},
		{"app code only", " M src/x.ts\n?? package.json\n", true, false},
		{"config only", "?? .spark/meta.json\n?? .agent/skills/steering/x.md\n", false, true},
		{"both", " M src/x.ts\n?? .spark/meta.json\n", true, true},
		{"rename to config", "R  old.txt -> .spark/meta.json\n", false, true},
		{"rename to app code", "R  .spark/old -> src/new.ts\n", true, false},
		{"quoted config path", "?? \".spark/with space.json\"\n", false, true},
		{"spark prefix lookalike not config", "?? .sparkrc\n", true, false},
		{"exact .spark dir", "?? .spark\n", false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotApp, gotCfg := classifyPorcelain(c.status)
			if (len(gotApp) > 0) != c.wantAppCode || (len(gotCfg) > 0) != c.wantConfig {
				t.Errorf("classifyPorcelain(%q) = (app=%v,cfg=%v), want app=%v cfg=%v",
					c.status, gotApp, gotCfg, c.wantAppCode, c.wantConfig)
			}
		})
	}
}

// commitMessages returns the -m messages of all recorded `git commit` calls.
func commitMessages(calls [][]string) []string {
	var msgs []string
	for _, c := range calls {
		if len(c) >= 3 && c[1] == "git" && c[2] == "commit" {
			for i := 3; i+1 < len(c); i++ {
				if c[i] == "-m" {
					msgs = append(msgs, c[i+1])
				}
			}
		}
	}
	return msgs
}

func TestAppsInit_EmptyRepo_TwoCommits(t *testing.T) {
	f := &fakeCommandRunner{results: map[string]fakeCallResult{
		"credential-init": credInitOK("http://u:t@h/app_x.git"),
		"git clone":       {},
		"git checkout":    {},
		"git ls-files":    {stdout: ""},
		"git status":      {stdout: " A src/app.ts\n A .spark/meta.json\n A .agent/skills/steering/x.md\n"},
	}}
	withFakeRunner(t, f)
	factory, stdout, _ := newAppsExecuteFactory(t)
	dir := relCloneDir(t)
	if err := runAppsShortcut(t, AppsInit, []string{"+init", "--app-id", "app_x", "--dir", dir, "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	msgs := commitMessages(f.calls)
	want := []string{"chore: initialize app project code", "chore: initialize miaoda app config"}
	if len(msgs) != 2 || msgs[0] != want[0] || msgs[1] != want[1] {
		t.Fatalf("commit messages = %v, want %v", msgs, want)
	}
	// The split's core invariant: each commit stages its own group's exact
	// porcelain paths (no :(exclude) magic, no explicitly-named ignored dirs —
	// see TestCommitAndPushIfDirty_RealGit_IgnoredAgentDir). The app-code commit
	// stages src/app.ts and not .spark/meta.json; the config commit, the reverse.
	appAdd := findCallArg(f.calls, "git", "add", "-A", "--", "src/app.ts")
	if appAdd == nil {
		t.Errorf("app-code git add missing src/app.ts; calls=%v", f.calls)
	} else if containsAll(appAdd, ".spark/meta.json") {
		t.Errorf("app-code commit must not stage config paths; got %v", appAdd)
	}
	cfgAdd := findCallArg(f.calls, "git", "add", "-A", "--", ".spark/meta.json")
	if cfgAdd == nil {
		t.Errorf("config git add missing .spark/meta.json; calls=%v", f.calls)
	} else if containsAll(cfgAdd, "src/app.ts") {
		t.Errorf("config commit must not stage app code; got %v", cfgAdd)
	}
	data := parseEnvelopeData(t, stdout)
	if data["committed"] != true || data["pushed"] != true {
		t.Errorf("committed/pushed = %v/%v, want true/true", data["committed"], data["pushed"])
	}
}

func TestAppsInit_EmptyRepo_AppCodeOnly_SingleCommit(t *testing.T) {
	f := &fakeCommandRunner{results: map[string]fakeCallResult{
		"credential-init": credInitOK("http://u:t@h/app_x.git"),
		"git clone":       {},
		"git checkout":    {},
		"git ls-files":    {stdout: ""},
		"git status":      {stdout: " A src/app.ts\n"},
	}}
	withFakeRunner(t, f)
	factory, stdout, _ := newAppsExecuteFactory(t)
	dir := relCloneDir(t)
	if err := runAppsShortcut(t, AppsInit, []string{"+init", "--app-id", "app_x", "--dir", dir, "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	msgs := commitMessages(f.calls)
	if len(msgs) != 1 || msgs[0] != "chore: initialize app project code" {
		t.Fatalf("commit messages = %v, want one app-code commit", msgs)
	}
}

func TestAppsInit_EmptyRepo_ConfigOnly_SingleCommit(t *testing.T) {
	f := &fakeCommandRunner{results: map[string]fakeCallResult{
		"credential-init": credInitOK("http://u:t@h/app_x.git"),
		"git clone":       {},
		"git checkout":    {},
		"git ls-files":    {stdout: ""},
		"git status":      {stdout: " A .spark/meta.json\n"},
	}}
	withFakeRunner(t, f)
	factory, stdout, _ := newAppsExecuteFactory(t)
	dir := relCloneDir(t)
	if err := runAppsShortcut(t, AppsInit, []string{"+init", "--app-id", "app_x", "--dir", dir, "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	msgs := commitMessages(f.calls)
	if len(msgs) != 1 || msgs[0] != "chore: initialize miaoda app config" {
		t.Fatalf("commit messages = %v, want one config commit", msgs)
	}
}

func TestAppsInit_NonEmpty_SingleInitCommit(t *testing.T) {
	f := &fakeCommandRunner{results: map[string]fakeCallResult{
		"credential-init": credInitOK("http://u:t@h/app_x.git"),
		"git clone":       {},
		"git checkout":    {},
		"git ls-files":    {stdout: "src/x.ts\n"},
		"git status":      {stdout: " M file.txt\n M .spark/meta.json\n"},
	}}
	withFakeRunner(t, f)
	factory, stdout, _ := newAppsExecuteFactory(t)
	dir := relCloneDir(t)
	if err := runAppsShortcut(t, AppsInit, []string{"+init", "--app-id", "app_x", "--dir", dir, "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	msgs := commitMessages(f.calls)
	if len(msgs) != 1 || msgs[0] != "chore: initialize miaoda app repository" {
		t.Fatalf("commit messages = %v, want one upgrade commit", msgs)
	}
	for _, c := range f.calls {
		if len(c) >= 3 && c[1] == "git" && c[2] == "commit" && !containsAll(c, "--no-verify") {
			t.Errorf("commit missing --no-verify: %v", c)
		}
	}
}

// gitMust runs a git command in dir with a real binary, failing the test on error.
func gitMust(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s failed: %v\n%s", args, dir, err, out)
	}
	return string(out)
}

// TestCommitAndPushIfDirty_RealGit_IgnoredAgentDir exercises the empty-repo
// commit split against a REAL git repo whose scaffold gitignores .agent. This
// reproduces the production failure where `git add -- .spark .agent` errored on
// the ignored .agent path; the fix stages the config remainder with ".".
func TestCommitAndPushIfDirty_RealGit_IgnoredAgentDir(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	// Bare remote so `git push origin sprint/default` succeeds.
	remote := t.TempDir()
	gitMust(t, remote, "init", "--bare", "-q", "--initial-branch", defaultInitBranch)

	dir := t.TempDir()
	gitMust(t, dir, "init", "-q", "--initial-branch", defaultInitBranch)
	gitMust(t, dir, "config", "user.email", "t@example.com")
	gitMust(t, dir, "config", "user.name", "Test")
	gitMust(t, dir, "remote", "add", "origin", remote)

	// Scaffold: app code + .spark config + an IGNORED .agent dir.
	mustWrite(t, filepath.Join(dir, ".gitignore"), ".agent\n")
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "src", "x.ts"), "export const x = 1\n")
	if err := os.MkdirAll(filepath.Join(dir, ".spark"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, ".spark", "meta.json"), `{"app_id":"app_x"}`)
	if err := os.MkdirAll(filepath.Join(dir, ".agent"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, ".agent", "skill.md"), "ignored\n")

	// Use the real exec runner (not the fake) so gitignore semantics apply.
	orig := initRunner
	initRunner = execCommandRunner{}
	t.Cleanup(func() { initRunner = orig })

	committed, pushed, err := commitAndPushIfDirty(context.Background(), dir, scaffoldKindInit)
	if err != nil {
		t.Fatalf("commitAndPushIfDirty returned error: %v", err)
	}
	if !committed || !pushed {
		t.Fatalf("committed=%v pushed=%v, want true/true", committed, pushed)
	}

	// Two commits, newest first: config then app code.
	subjects := strings.Split(strings.TrimSpace(gitMust(t, dir, "log", "--format=%s", "-2")), "\n")
	want := []string{commitMsgAppConfig, commitMsgAppCode}
	if len(subjects) != 2 || subjects[0] != want[0] || subjects[1] != want[1] {
		t.Fatalf("commit subjects = %v, want %v", subjects, want)
	}

	// .agent must NOT be tracked; .spark and src must be.
	tracked := gitMust(t, dir, "ls-files")
	if strings.Contains(tracked, ".agent") {
		t.Errorf("ignored .agent must not be committed; tracked=%q", tracked)
	}
	if !strings.Contains(tracked, ".spark/meta.json") {
		t.Errorf(".spark/meta.json should be committed; tracked=%q", tracked)
	}
	if !strings.Contains(tracked, "src/x.ts") {
		t.Errorf("src/x.ts should be committed; tracked=%q", tracked)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func envPullOK(envFile string) fakeCallResult {
	return fakeCallResult{stdout: `{"ok":true,"data":{"env_file":"` + envFile + `"}}`}
}

// testRuntimeForEnvPull builds a minimal RuntimeContext exposing the --as flag,
// which is all pullEnv reads.
func testRuntimeForEnvPull(t *testing.T, as string) *common.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "init"}
	cmd.Flags().String("as", as, "")
	return common.TestNewRuntimeContext(cmd, nil)
}

func TestPullEnv(t *testing.T) {
	// success: stdout envelope parsed; subprocess invoked with expected args
	f := &fakeCommandRunner{results: map[string]fakeCallResult{"env-pull": envPullOK("/abs/app_x/.env.local")}}
	withFakeRunner(t, f)
	rctx := testRuntimeForEnvPull(t, "")
	envFile, reason := pullEnv(context.Background(), rctx, "app_x", "/abs/app_x")
	if reason != "" || envFile != "/abs/app_x/.env.local" {
		t.Fatalf("success: envFile=%q reason=%q", envFile, reason)
	}
	// findCallArg matches c[1] against name; for self-invocations c[1] is the
	// test binary path (unknown at compile time), so search the args slice
	// directly for the expected ordered subsequence.
	var c []string
	for _, call := range f.calls {
		if findCallArg([][]string{call}, call[1], "apps", "+env-pull", "--app-id", "app_x", "--project-path", "/abs/app_x", "--format", "json") != nil {
			c = call
			break
		}
	}
	if c == nil {
		t.Errorf("+env-pull not invoked with expected args: %v", f.calls)
	}

	// failure: non-zero exit + stderr error envelope -> reason, env_file empty
	f2 := &fakeCommandRunner{results: map[string]fakeCallResult{"env-pull": {
		stderr: `{"ok":false,"error":{"type":"missing_scope","message":"need spark:app:read"}}`,
		err:    fmt.Errorf("exit status 2"),
	}}}
	withFakeRunner(t, f2)
	envFile2, reason2 := pullEnv(context.Background(), testRuntimeForEnvPull(t, ""), "app_x", "/abs/app_x")
	if envFile2 != "" || reason2 != "missing_scope: need spark:app:read" {
		t.Fatalf("failure: envFile=%q reason=%q", envFile2, reason2)
	}
}

// TestCommitAndPushIfDirty_RealGit_NonEmptyUpgrade pins down that the non-empty
// (upgrade) path is unaffected by the commit-split / exact-path changes: it must
// stay a SINGLE commit using `git add -A -- .`, which silently skips a gitignored
// .agent (no ignored-path error), with the upgrade subject.
func TestCommitAndPushIfDirty_RealGit_NonEmptyUpgrade(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	remote := t.TempDir()
	gitMust(t, remote, "init", "--bare", "-q", "--initial-branch", defaultInitBranch)

	dir := t.TempDir()
	gitMust(t, dir, "init", "-q", "--initial-branch", defaultInitBranch)
	gitMust(t, dir, "config", "user.email", "t@example.com")
	gitMust(t, dir, "config", "user.name", "Test")
	gitMust(t, dir, "remote", "add", "origin", remote)

	// Existing (non-empty) repo: a committed baseline with .agent already ignored.
	mustWrite(t, filepath.Join(dir, ".gitignore"), ".agent\n")
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "src", "old.ts"), "export const old = 0\n")
	gitMust(t, dir, "add", "-A")
	gitMust(t, dir, "commit", "-q", "-m", "baseline")
	baseline := strings.TrimSpace(gitMust(t, dir, "rev-parse", "HEAD"))

	// Simulate `app sync`: a modified app file, a patched .spark config, and an
	// IGNORED .agent dir produced by `skills sync`.
	mustWrite(t, filepath.Join(dir, "src", "old.ts"), "export const old = 1\n")
	if err := os.MkdirAll(filepath.Join(dir, ".spark"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, ".spark", "meta.json"), `{"app_id":"app_x"}`)
	if err := os.MkdirAll(filepath.Join(dir, ".agent"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, ".agent", "skill.md"), "ignored\n")

	orig := initRunner
	initRunner = execCommandRunner{}
	t.Cleanup(func() { initRunner = orig })

	committed, pushed, err := commitAndPushIfDirty(context.Background(), dir, scaffoldKindUpgrade)
	if err != nil {
		t.Fatalf("commitAndPushIfDirty returned error: %v", err)
	}
	if !committed || !pushed {
		t.Fatalf("committed=%v pushed=%v, want true/true", committed, pushed)
	}

	// Exactly ONE commit added, with the upgrade subject (not a split).
	added := strings.TrimSpace(gitMust(t, dir, "rev-list", "--count", baseline+"..HEAD"))
	if added != "1" {
		t.Fatalf("upgrade path added %s commits, want exactly 1 (no split)", added)
	}
	if subj := strings.TrimSpace(gitMust(t, dir, "log", "--format=%s", "-1")); subj != commitMsgUpgrade {
		t.Errorf("upgrade commit subject = %q, want %q", subj, commitMsgUpgrade)
	}

	// .agent stays ignored; the real changes are committed.
	tracked := gitMust(t, dir, "ls-files")
	if strings.Contains(tracked, ".agent") {
		t.Errorf("ignored .agent must not be committed; tracked=%q", tracked)
	}
	if !strings.Contains(tracked, ".spark/meta.json") {
		t.Errorf(".spark/meta.json should be committed; tracked=%q", tracked)
	}
}

func TestEnsureEmptyDir_RejectsNonDirAndNonEmpty(t *testing.T) {
	t.Run("non-existent is ok", func(t *testing.T) {
		if err := ensureEmptyDir(filepath.Join(t.TempDir(), "nope")); err != nil {
			t.Errorf("non-existent dir should be ok, got %v", err)
		}
	})
	t.Run("file is rejected", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "afile")
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := ensureEmptyDir(f); err == nil {
			t.Error("a regular file must be rejected")
		}
	})
	t.Run("non-empty dir is rejected", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "child"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := ensureEmptyDir(dir); err == nil {
			t.Error("a non-empty dir must be rejected")
		}
	})
	t.Run("empty dir is ok", func(t *testing.T) {
		if err := ensureEmptyDir(t.TempDir()); err != nil {
			t.Errorf("empty dir should be ok, got %v", err)
		}
	})
}

func TestParseEnvFileFromEnvelope(t *testing.T) {
	got, err := parseEnvFileFromEnvelope(`{"ok":true,"data":{"env_file":"/abs/app_x/.env.local"}}`)
	if err != nil || got != "/abs/app_x/.env.local" {
		t.Fatalf("got %q err %v", got, err)
	}
	for _, in := range []string{``, `not json`, `{"ok":false,"data":{}}`, `{"ok":true,"data":{}}`, `{"ok":true,"data":{"env_file":""}}`} {
		if _, err := parseEnvFileFromEnvelope(in); err == nil {
			t.Errorf("expected error for %q", in)
		}
	}
}

func TestParseEnvPullErrorEnvelope(t *testing.T) {
	cases := []struct{ in, want string }{
		{`{"ok":false,"error":{"type":"missing_scope","message":"need spark:app:read"}}`, "missing_scope: need spark:app:read"},
		{`{"ok":false,"error":{"message":"boom"}}`, "boom"},
		{`not json`, ""},
		{`{"ok":false,"error":{}}`, ""},
		{``, ""},
	}
	for _, c := range cases {
		if got := parseEnvPullErrorEnvelope(c.in); got != c.want {
			t.Errorf("parseEnvPullErrorEnvelope(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestEnsureMetaAppID_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".spark"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, metaRelPath), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureMetaAppID(dir, "app_x"); err == nil {
		t.Error("malformed meta.json must return a parse error")
	}
}

func TestIsEmptyRepo_GitError(t *testing.T) {
	f := &fakeCommandRunner{results: map[string]fakeCallResult{
		"git ls-files": {err: errors.New("fatal: not a git repository")},
	}}
	withFakeRunner(t, f)
	if _, err := isEmptyRepo(context.Background(), t.TempDir()); err == nil {
		t.Error("git ls-files failure must surface as an error")
	}
}

func TestRunScaffold_NonEmpty_SyncFailure(t *testing.T) {
	// Non-empty repo takes the `app sync` path; make that npx call fail.
	withFakeRunner(t, &fakeCommandRunner{results: map[string]fakeCallResult{
		"git ls-files": {stdout: "src/x.ts\n"},
		"npx -y":       {err: errors.New("sync boom")},
	}})
	if _, err := runScaffold(context.Background(), t.TempDir(), "app_x", "tpl"); err == nil {
		t.Error("npx app sync failure must surface as an error")
	}
}

func TestStageAndCommit_Errors(t *testing.T) {
	t.Run("git add fails", func(t *testing.T) {
		withFakeRunner(t, &fakeCommandRunner{results: map[string]fakeCallResult{
			"git add": {err: errors.New("boom")},
		}})
		if err := stageAndCommit(context.Background(), t.TempDir(), "msg", "."); err == nil {
			t.Error("git add failure must surface as an error")
		}
	})
	t.Run("git commit fails", func(t *testing.T) {
		withFakeRunner(t, &fakeCommandRunner{results: map[string]fakeCallResult{
			"git commit": {err: errors.New("boom")},
		}})
		if err := stageAndCommit(context.Background(), t.TempDir(), "msg", "."); err == nil {
			t.Error("git commit failure must surface as an error")
		}
	})
}

func TestCommitAndPushIfDirty_Branches(t *testing.T) {
	t.Run("clean tree is a no-op", func(t *testing.T) {
		withFakeRunner(t, &fakeCommandRunner{results: map[string]fakeCallResult{
			"git status": {stdout: "   "},
		}})
		committed, pushed, err := commitAndPushIfDirty(context.Background(), t.TempDir(), scaffoldKindUpgrade)
		if err != nil || committed || pushed {
			t.Errorf("clean tree: got committed=%v pushed=%v err=%v, want false,false,nil", committed, pushed, err)
		}
	})
	t.Run("status error", func(t *testing.T) {
		withFakeRunner(t, &fakeCommandRunner{results: map[string]fakeCallResult{
			"git status": {err: errors.New("boom")},
		}})
		if _, _, err := commitAndPushIfDirty(context.Background(), t.TempDir(), scaffoldKindUpgrade); err == nil {
			t.Error("git status failure must surface as an error")
		}
	})
	t.Run("upgrade path commits and pushes", func(t *testing.T) {
		withFakeRunner(t, &fakeCommandRunner{results: map[string]fakeCallResult{
			"git status": {stdout: " M src/app.ts\n"},
		}})
		committed, pushed, err := commitAndPushIfDirty(context.Background(), t.TempDir(), scaffoldKindUpgrade)
		if err != nil || !committed || !pushed {
			t.Errorf("dirty upgrade: got committed=%v pushed=%v err=%v, want true,true,nil", committed, pushed, err)
		}
	})
	t.Run("push failure", func(t *testing.T) {
		withFakeRunner(t, &fakeCommandRunner{results: map[string]fakeCallResult{
			"git status": {stdout: " M src/app.ts\n"},
			"git push":   {err: errors.New("rejected")},
		}})
		committed, pushed, err := commitAndPushIfDirty(context.Background(), t.TempDir(), scaffoldKindUpgrade)
		if err == nil || !committed || pushed {
			t.Errorf("push failure: got committed=%v pushed=%v err=%v, want true,false,err", committed, pushed, err)
		}
	})
}

func TestAppsInit_EnvPull_Success(t *testing.T) {
	f := &fakeCommandRunner{results: map[string]fakeCallResult{
		"credential-init": credInitOK("http://u:t@h/app_x.git"),
		"git clone":       {},
		"git checkout":    {},
		"git ls-files":    {stdout: ""},
		"git status":      {},
		"env-pull":        envPullOK("/abs/app_x/.env.local"),
	}}
	withFakeRunner(t, f)
	factory, stdout, _ := newAppsExecuteFactory(t)
	dir := relCloneDir(t)
	if err := runAppsShortcut(t, AppsInit, []string{"+init", "--app-id", "app_x", "--dir", dir, "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := parseEnvelopeData(t, stdout)
	if data["env_pulled"] != true {
		t.Errorf("env_pulled = %v, want true", data["env_pulled"])
	}
	if data["env_file"] != "/abs/app_x/.env.local" {
		t.Errorf("env_file = %v", data["env_file"])
	}
	// env-pull invoked with forwarded --as user and the expected flags
	var ep []string
	for _, c := range f.calls {
		if containsAll(c, "+env-pull") {
			ep = c
			break
		}
	}
	if ep == nil || !containsAll(ep, "--app-id", "app_x", "--project-path", "--as", "user", "--format", "json") {
		t.Errorf("+env-pull not invoked with expected args: %v", f.calls)
	}
}

func TestAppsInit_EnvPull_NonFatal(t *testing.T) {
	f := &fakeCommandRunner{results: map[string]fakeCallResult{
		"credential-init": credInitOK("http://u:t@h/app_x.git"),
		"git clone":       {},
		"git checkout":    {},
		"git ls-files":    {stdout: ""},
		"git status":      {},
		"env-pull": {
			stderr: `{"ok":false,"error":{"type":"missing_scope","message":"need spark:app:read"}}`,
			err:    fmt.Errorf("exit status 2"),
		},
	}}
	withFakeRunner(t, f)
	factory, stdout, _ := newAppsExecuteFactory(t)
	dir := relCloneDir(t)
	if err := runAppsShortcut(t, AppsInit, []string{"+init", "--app-id", "app_x", "--dir", dir, "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("env-pull failure must be non-fatal, got: %v", err)
	}
	data := parseEnvelopeData(t, stdout)
	if data["env_pulled"] != false {
		t.Errorf("env_pulled = %v, want false", data["env_pulled"])
	}
	if data["env_pull_error"] != "missing_scope: need spark:app:read" {
		t.Errorf("env_pull_error = %v", data["env_pull_error"])
	}
	if _, ok := data["env_file"]; ok {
		t.Errorf("env_file must be absent on failure: %v", data["env_file"])
	}
	msg, _ := data["message"].(string)
	if !strings.Contains(msg, "+env-pull --app-id app_x") {
		t.Errorf("message missing retry hint: %q", msg)
	}
	if strings.Contains(stdout.String(), "u:t@h") {
		t.Errorf("raw credential leaked: %s", stdout.String())
	}
}

func TestAppsInit_AlreadyInitialized_RunsEnvPull(t *testing.T) {
	dir := relCloneDir(t)
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(abs, ".spark"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(abs, metaRelPath), []byte(`{"app_id":"app_x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	envFile := filepath.Join(abs, ".env.local")
	f := &fakeCommandRunner{results: map[string]fakeCallResult{"env-pull": envPullOK(envFile)}}
	withFakeRunner(t, f)
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsInit, []string{"+init", "--app-id", "app_x", "--dir", dir, "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	called := false
	for _, c := range f.calls {
		if containsAll(c, "+env-pull") {
			called = true
		}
	}
	if !called {
		t.Errorf("already-initialized path must call +env-pull: %v", f.calls)
	}
	data := parseEnvelopeData(t, stdout)
	if data["scaffold"] != "already_initialized" {
		t.Errorf("scaffold=%v, want already_initialized", data["scaffold"])
	}
	if data["env_pulled"] != true {
		t.Errorf("env_pulled=%v, want true", data["env_pulled"])
	}
	if data["env_file"] != envFile {
		t.Errorf("env_file=%v, want %v", data["env_file"], envFile)
	}
	if data["committed"] != false || data["pushed"] != false {
		t.Errorf("committed/pushed must stay false: %v", data)
	}
}

func TestAppsInit_AlreadyInitialized_EnvPullFailure_NonFatal(t *testing.T) {
	dir := relCloneDir(t)
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(abs, ".spark"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(abs, metaRelPath), []byte(`{"app_id":"app_x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &fakeCommandRunner{results: map[string]fakeCallResult{
		"env-pull": {
			stderr: `{"ok":false,"error":{"type":"missing_scope","message":"need spark:app:read"}}`,
			err:    fmt.Errorf("exit status 2"),
		},
	}}
	withFakeRunner(t, f)
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsInit, []string{"+init", "--app-id", "app_x", "--dir", dir, "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("env-pull failure must be non-fatal, got: %v", err)
	}
	data := parseEnvelopeData(t, stdout)
	if data["scaffold"] != "already_initialized" {
		t.Errorf("scaffold=%v, want already_initialized", data["scaffold"])
	}
	if data["env_pulled"] != false {
		t.Errorf("env_pulled=%v, want false", data["env_pulled"])
	}
	if data["env_pull_error"] != "missing_scope: need spark:app:read" {
		t.Errorf("env_pull_error=%v", data["env_pull_error"])
	}
	if _, ok := data["env_file"]; ok {
		t.Errorf("env_file must be absent on failure: %v", data["env_file"])
	}
	msg, _ := data["message"].(string)
	if !strings.Contains(msg, "+env-pull --app-id app_x") {
		t.Errorf("message missing retry hint: %q", msg)
	}
}

func TestAppsInit_DryRun_DescribesEnvPull(t *testing.T) {
	f := &fakeCommandRunner{results: map[string]fakeCallResult{}}
	withFakeRunner(t, f)
	factory, stdout, _ := newAppsExecuteFactory(t)
	dir := relCloneDir(t)
	if err := runAppsShortcut(t, AppsInit, []string{"+init", "--app-id", "app_x", "--dir", dir, "--as", "user", "--dry-run"}, factory, stdout); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &m); err != nil {
		t.Fatalf("decode dry-run: %v (raw=%q)", err, stdout.String())
	}
	ep, _ := m["env_pull"].(string)
	if !strings.Contains(ep, "+env-pull") {
		t.Errorf("dry-run missing env_pull step: %v", m)
	}
	for _, c := range f.calls {
		if containsAll(c, "+env-pull") {
			t.Errorf("dry-run must not execute +env-pull: %v", f.calls)
		}
	}
}

func TestAppsInit_Description_IsAboutCode(t *testing.T) {
	if strings.Contains(strings.ToLower(AppsInit.Description), "local development repository") {
		t.Errorf("Description should describe initializing app code, not a local dev repo: %q", AppsInit.Description)
	}
	if !strings.Contains(strings.ToLower(AppsInit.Description), "code") {
		t.Errorf("Description should mention app code: %q", AppsInit.Description)
	}
}

// TestRunScaffold_SubprocessFailureIsExternalTool pins the typed
// classification of an external-tool failure: a failing git subprocess
// surfaces as internal/external_tool with the cause preserved.
func TestRunScaffold_SubprocessFailureIsExternalTool(t *testing.T) {
	cause := errors.New("exit status 128")
	f := &fakeCommandRunner{results: map[string]fakeCallResult{
		"git ls-files": {stderr: "fatal: not a git repository", err: cause},
	}}
	withFakeRunner(t, f)
	_, err := runScaffold(context.Background(), t.TempDir(), "app_x", "nestjs-react-fullstack")
	if err == nil {
		t.Fatalf("expected error from failing git subprocess")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T: %v", err, err)
	}
	if p.Category != errs.CategoryInternal || p.Subtype != errs.SubtypeExternalTool {
		t.Fatalf("classification = %s/%s, want internal/external_tool", p.Category, p.Subtype)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("cause chain not preserved: %v", err)
	}
}
