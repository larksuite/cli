// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/vfs"
)

// executableTestFS mocks vfs for tests that still need vfs.Executable.
type executableTestFS struct {
	vfs.OsFs
	exe string
}

func (f executableTestFS) Executable() (string, error) { return f.exe, nil }

// lookPathMock patches execLookPath within VerifyBinary for controlled testing.
// Do not use t.Parallel() in tests that install this mock — it mutates a package-level var.
type lookPathMock struct {
	oldLookPath func(string) (string, error)
	result      string
	resultErr   error
}

func (m *lookPathMock) install(bin string) {
	m.oldLookPath = execLookPath
	execLookPath = func(name string) (string, error) {
		if name == bin {
			return m.result, m.resultErr
		}
		return m.oldLookPath(name)
	}
}

func (m *lookPathMock) restore() {
	execLookPath = m.oldLookPath
}

func TestResolveExe(t *testing.T) {
	u := New()
	p, err := u.resolveExe()
	if err != nil {
		t.Fatalf("resolveExe() error: %v", err)
	}
	if !filepath.IsAbs(p) {
		t.Errorf("expected absolute path, got: %s", p)
	}
}

func TestPrepareSelfReplace_ReturnsNoError(t *testing.T) {
	u := New()
	restore, err := u.PrepareSelfReplace()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	restore()
}

func TestCleanupStaleFiles_NoPanic(t *testing.T) {
	u := New()
	u.CleanupStaleFiles()
}

func TestVerifyBinaryLookPath(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell script")
	}

	dir := t.TempDir()
	bin := filepath.Join(dir, "lark-cli")
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo \"lark-cli version 2.1.0\"; exit 0; fi\nexit 12\n"
	if err := os.WriteFile(bin, []byte(script), 0755); err != nil {
		t.Fatalf("write test binary: %v", err)
	}

	mock := &lookPathMock{result: bin}
	mock.install("lark-cli")
	t.Cleanup(mock.restore)

	if err := New().VerifyBinary("2.1.0"); err != nil {
		t.Fatalf("VerifyBinary(2.1.0) error = %v, want nil", err)
	}

	if err := New().VerifyBinary("3.0.0"); err == nil {
		t.Fatal("VerifyBinary(mismatched) expected error, got nil")
	}

	// Regression: version must match exactly (not substring / prefix).
	if err := New().VerifyBinary("0.0"); err == nil {
		t.Fatal("VerifyBinary(substring-style mismatch) expected error, got nil")
	}
	if err := New().VerifyBinary("12.1.0"); err == nil {
		t.Fatal("VerifyBinary(prefix-style mismatch) expected error, got nil")
	}
}

func TestVerifyBinaryLookPathNotFound(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	mock := &lookPathMock{result: "", resultErr: fmt.Errorf("not found")}
	mock.install("lark-cli")
	t.Cleanup(mock.restore)

	oldFS := vfs.DefaultFS
	t.Cleanup(func() { vfs.DefaultFS = oldFS })
	// Without this, VerifyBinary would fall back to the real test binary, which
	// is not a lark-cli --version implementation.
	vfs.DefaultFS = executableTestFS{exe: filepath.Join(t.TempDir(), "missing-lark-cli")}

	if err := New().VerifyBinary("2.0.0"); err == nil {
		t.Fatal("VerifyBinary(not-found) expected error, got nil")
	}
}

func TestVerifyBinaryFallbackExecutableWhenNotOnPath(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell script")
	}

	dir := t.TempDir()
	bin := filepath.Join(dir, "lark-cli-abs")
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo \"lark-cli version 2.1.0\"; exit 0; fi\nexit 12\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write test binary: %v", err)
	}

	mock := &lookPathMock{result: "", resultErr: fmt.Errorf("not on PATH")}
	mock.install("lark-cli")
	t.Cleanup(mock.restore)

	oldFS := vfs.DefaultFS
	t.Cleanup(func() { vfs.DefaultFS = oldFS })
	vfs.DefaultFS = executableTestFS{exe: bin}

	if err := New().VerifyBinary("2.1.0"); err != nil {
		t.Fatalf("VerifyBinary(fallback executable) error = %v, want nil", err)
	}
}

func TestVerifyBinaryEmptyOutput(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell script")
	}

	dir := t.TempDir()
	bin := filepath.Join(dir, "lark-cli")
	script := "#!/bin/sh\necho\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0755); err != nil {
		t.Fatalf("write test binary: %v", err)
	}

	mock := &lookPathMock{result: bin}
	mock.install("lark-cli")
	t.Cleanup(mock.restore)

	if err := New().VerifyBinary("2.0.0"); err == nil {
		t.Fatal("VerifyBinary(empty output) expected error, got nil")
	}
}

func TestSkillsCommandsUseExpectedArgs(t *testing.T) {
	tests := []struct {
		name string
		run  func(*Updater) *NpmResult
		want string
	}{
		{
			name: "list official primary",
			run: func(u *Updater) *NpmResult {
				return u.runSkillsListOfficial("https://open.feishu.cn")
			},
			want: "-y skills add https://open.feishu.cn --list",
		},
		{
			name: "list global",
			run: func(u *Updater) *NpmResult {
				return u.runSkillsListGlobal()
			},
			want: "-y skills ls -g",
		},
		{
			name: "list global json",
			run: func(u *Updater) *NpmResult {
				return u.ListGlobalSkillsJSON()
			},
			want: "-y skills ls -g --json",
		},
		{
			name: "install skill primary",
			run: func(u *Updater) *NpmResult {
				return u.runSkillsInstall("https://open.feishu.cn", []string{"lark-mail"})
			},
			want: "-y skills add https://open.feishu.cn -s lark-mail -g -y",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("uses a POSIX shell script")
			}
			dir := t.TempDir()
			script := filepath.Join(dir, "npx")
			logPath := filepath.Join(dir, "npx.log")
			if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \""+logPath+"\"\nexit 0\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

			result := tt.run(New())
			if result.Err != nil {
				t.Fatalf("command err = %v, want nil", result.Err)
			}
			raw, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(string(raw)) != tt.want {
				t.Fatalf("args = %q, want %q", strings.TrimSpace(string(raw)), tt.want)
			}
		})
	}
}

func TestListOfficialSkillsIndexSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"skills":[{"name":"lark-calendar"}]}`)
	}))
	defer server.Close()

	oldURL := officialSkillsIndexURL
	officialSkillsIndexURL = server.URL
	t.Cleanup(func() { officialSkillsIndexURL = oldURL })

	result := New().ListOfficialSkillsIndex()
	if result.Err != nil {
		t.Fatalf("ListOfficialSkillsIndex() err = %v, want nil", result.Err)
	}
	if got := result.Stdout.String(); !strings.Contains(got, "lark-calendar") {
		t.Fatalf("ListOfficialSkillsIndex() stdout = %q, want skill JSON", got)
	}
}

func TestListOfficialSkillsIndexHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	oldURL := officialSkillsIndexURL
	officialSkillsIndexURL = server.URL
	t.Cleanup(func() { officialSkillsIndexURL = oldURL })

	result := New().ListOfficialSkillsIndex()
	if result.Err == nil || !strings.Contains(result.Err.Error(), "HTTP 404") {
		t.Fatalf("ListOfficialSkillsIndex() err = %v, want HTTP 404", result.Err)
	}
}

func TestListOfficialSkillsIndexBodyTooLarge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, strings.Repeat("x", skillsIndexMaxBodySize+1))
	}))
	defer server.Close()

	oldURL := officialSkillsIndexURL
	officialSkillsIndexURL = server.URL
	t.Cleanup(func() { officialSkillsIndexURL = oldURL })

	result := New().ListOfficialSkillsIndex()
	if result.Err == nil || !strings.Contains(result.Err.Error(), "exceeds") {
		t.Fatalf("ListOfficialSkillsIndex() err = %v, want exceeds", result.Err)
	}
	if result.Stdout.Len() != 0 {
		t.Fatalf("ListOfficialSkillsIndex() stdout len = %d, want 0", result.Stdout.Len())
	}
}

func TestListOfficialSkillsIndexTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		fmt.Fprint(w, `{"skills":[{"name":"lark-calendar"}]}`)
	}))
	defer server.Close()

	oldURL := officialSkillsIndexURL
	oldTimeout := skillsIndexFetchTimeout
	officialSkillsIndexURL = server.URL
	skillsIndexFetchTimeout = 50 * time.Millisecond
	t.Cleanup(func() {
		officialSkillsIndexURL = oldURL
		skillsIndexFetchTimeout = oldTimeout
	})

	result := New().ListOfficialSkillsIndex()
	var netErr net.Error
	if result.Err == nil || (!errors.Is(result.Err, context.DeadlineExceeded) && !(errors.As(result.Err, &netErr) && netErr.Timeout())) {
		t.Fatalf("ListOfficialSkillsIndex() err = %v, want timeout error", result.Err)
	}
}

func TestListOfficialSkillsIndexRejectsNonHTTPSRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.com/skills.json", http.StatusFound)
	}))
	defer server.Close()

	oldURL := officialSkillsIndexURL
	officialSkillsIndexURL = server.URL
	t.Cleanup(func() { officialSkillsIndexURL = oldURL })

	result := New().ListOfficialSkillsIndex()
	if result.Err == nil || !strings.Contains(result.Err.Error(), "non-HTTPS") {
		t.Fatalf("ListOfficialSkillsIndex() err = %v, want non-HTTPS redirect", result.Err)
	}
}

func TestListOfficialSkillsIndexUsesOverride(t *testing.T) {
	result := (&Updater{SkillsIndexFetchOverride: func() *NpmResult {
		r := &NpmResult{}
		r.Stdout.WriteString(`{"skills":[{"name":"override-skill"}]}`)
		return r
	}}).ListOfficialSkillsIndex()
	if result.Err != nil {
		t.Fatalf("ListOfficialSkillsIndex() err = %v, want nil", result.Err)
	}
	if !strings.Contains(result.Stdout.String(), "override-skill") {
		t.Fatalf("ListOfficialSkillsIndex() stdout = %q, want override result", result.Stdout.String())
	}
}

func TestListOfficialSkillsFallsBack(t *testing.T) {
	called := []string{}
	updater := &Updater{
		SkillsCommandOverride: func(args ...string) *NpmResult {
			called = append(called, strings.Join(args, " "))
			r := &NpmResult{}
			if strings.Contains(strings.Join(args, " "), "https://open.feishu.cn") {
				r.Err = fmt.Errorf("primary failed")
				return r
			}
			r.Stdout.WriteString("lark-calendar\n")
			return r
		},
	}

	result := updater.ListOfficialSkills()
	if result.Err != nil {
		t.Fatalf("ListOfficialSkills() err = %v, want nil", result.Err)
	}
	if len(called) != 2 {
		t.Fatalf("called %d commands, want 2: %#v", len(called), called)
	}
	if !strings.Contains(called[1], "larksuite/cli --list") {
		t.Fatalf("fallback call = %q, want larksuite/cli --list", called[1])
	}
}

func TestContainsPnpmMarker(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		// Classic virtual-store layout (.pnpm segment).
		{"/Users/x/Library/pnpm/global/5/node_modules/.pnpm/@larksuite+cli@1.0.44/node_modules/@larksuite/cli/bin/lark-cli", true},
		{`C:\Users\x\AppData\Local\pnpm\global\5\node_modules\.pnpm\@larksuite+cli@1.0.44\node_modules\@larksuite\cli\bin\lark-cli.exe`, true},
		// Global content-addressable store layout (pnpm 11): resolved path runs
		// through the pnpm home store, a "pnpm" segment with no ".pnpm".
		{"/Users/x/Library/pnpm/store/v11/links/@larksuite/cli/1.0.59/abc123/node_modules/@larksuite/cli/bin/lark-cli", true},
		{"/home/x/.local/share/pnpm/store/v10/@larksuite/cli/node_modules/@larksuite/cli/bin/lark-cli", true},
		{`C:\Users\x\AppData\Local\pnpm\store\v11\links\@larksuite\cli\node_modules\@larksuite\cli\bin\lark-cli.exe`, true},
		// npm and non-package installs — no pnpm/.pnpm segment.
		{"/usr/local/lib/node_modules/@larksuite/cli/bin/lark-cli", false},
		{"/usr/local/bin/lark-cli", false},
		// Substrings that must NOT match: segment must be exactly .pnpm, or
		// "pnpm" immediately followed by "store".
		{"/opt/homebrew/.pnpmfoo/node_modules/@larksuite/cli/bin/lark-cli", false},
		{"/opt/pnpmfoo/node_modules/@larksuite/cli/bin/lark-cli", false},
		// A bare "pnpm" directory NOT followed by "store" (e.g. an npm install
		// living under a dir named pnpm) must not be misclassified as pnpm.
		{"/opt/pnpm/lib/node_modules/@larksuite/cli/bin/lark-cli", false},
	}
	for _, c := range cases {
		if got := containsPnpmMarker(c.path); got != c.want {
			t.Errorf("containsPnpmMarker(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestDetectInstallMethod_Pnpm(t *testing.T) {
	u := &Updater{DetectOverride: nil}
	u.DetectOverride = func() DetectResult {
		// Exercise the real classification by feeding a resolved path via a small shim.
		return detectFromResolved("/x/node_modules/.pnpm/@larksuite+cli@1.0.44/node_modules/@larksuite/cli/bin/lark-cli", true, true)
	}
	got := u.DetectInstallMethod()
	if got.Method != InstallPnpm {
		t.Errorf("Method = %v, want InstallPnpm", got.Method)
	}
	if !got.PnpmAvailable {
		t.Errorf("PnpmAvailable = false, want true")
	}
}

func TestDetectInstallMethod_NpmVsManual(t *testing.T) {
	if m := detectFromResolved("/usr/local/lib/node_modules/@larksuite/cli/bin/lark-cli", true, false).Method; m != InstallNpm {
		t.Errorf("npm path Method = %v, want InstallNpm", m)
	}
	if m := detectFromResolved("/usr/local/bin/lark-cli", false, false).Method; m != InstallManual {
		t.Errorf("manual path Method = %v, want InstallManual", m)
	}
}

func TestCanAutoUpdate_Pnpm(t *testing.T) {
	if !(DetectResult{Method: InstallPnpm, PnpmAvailable: true}).CanAutoUpdate() {
		t.Error("pnpm available should CanAutoUpdate")
	}
	if (DetectResult{Method: InstallPnpm, PnpmAvailable: false}).CanAutoUpdate() {
		t.Error("pnpm unavailable should not CanAutoUpdate")
	}
}

func TestManualReason_Pnpm(t *testing.T) {
	if got := (DetectResult{Method: InstallPnpm, NpmAvailable: false, PnpmAvailable: false}).ManualReason(); got != "installed via pnpm, but pnpm is not available in PATH" {
		t.Errorf("pnpm reason = %q", got)
	}
	if got := (DetectResult{Method: InstallManual}).ManualReason(); got != "not installed via npm or pnpm" {
		t.Errorf("manual reason = %q", got)
	}
}

func TestRunPnpmInstall_Override(t *testing.T) {
	u := &Updater{PnpmInstallOverride: func(version string) *NpmResult {
		r := &NpmResult{}
		r.Stdout.WriteString("added @larksuite/cli@" + version)
		return r
	}}
	got := u.RunPnpmInstall("2.0.0")
	if got.Err != nil {
		t.Fatalf("unexpected err: %v", got.Err)
	}
	if !strings.Contains(got.CombinedOutput(), "2.0.0") {
		t.Errorf("output = %q, want version echoed", got.CombinedOutput())
	}
}

func TestRunPnpmInstall_Error(t *testing.T) {
	wantErr := errors.New("boom")
	u := &Updater{PnpmInstallOverride: func(string) *NpmResult { return &NpmResult{Err: wantErr} }}
	if got := u.RunPnpmInstall("2.0.0"); !errors.Is(got.Err, wantErr) {
		t.Errorf("err = %v, want %v", got.Err, wantErr)
	}
}

func TestSkillsInvocation(t *testing.T) {
	addArgs := []string{"-y", "skills", "add", "https://open.feishu.cn", "-g", "-y"}
	cases := []struct {
		name          string
		method        InstallMethod
		pnpmAvailable bool
		args          []string
		wantLauncher  string
		wantRest      []string
	}{
		{"pnpm install + pnpm available → pnpm dlx, drop leading -y", InstallPnpm, true, addArgs,
			"pnpm", []string{"dlx", "skills", "add", "https://open.feishu.cn", "-g", "-y"}},
		{"pnpm install but pnpm unavailable → npx unchanged", InstallPnpm, false, addArgs,
			"npx", addArgs},
		{"npm install → npx unchanged", InstallNpm, false, addArgs,
			"npx", addArgs},
		{"manual install → npx unchanged", InstallManual, false, []string{"-y", "skills", "ls", "-g"},
			"npx", []string{"-y", "skills", "ls", "-g"}},
		{"pnpm without a leading -y → prepend dlx only", InstallPnpm, true, []string{"skills", "ls", "-g"},
			"pnpm", []string{"dlx", "skills", "ls", "-g"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotLauncher, gotRest := skillsInvocation(c.method, c.pnpmAvailable, c.args)
			if gotLauncher != c.wantLauncher {
				t.Errorf("launcher = %q, want %q", gotLauncher, c.wantLauncher)
			}
			if strings.Join(gotRest, " ") != strings.Join(c.wantRest, " ") {
				t.Errorf("rest = %v, want %v", gotRest, c.wantRest)
			}
		})
	}
}

// TestDetectInstallMethod_Caches locks the fix for the post-update re-detection
// hazard: DetectInstallMethod must return the first (pre-update) detection on
// subsequent calls, so the skills launcher chosen after the binary is replaced
// stays consistent with what was detected — and reported — before the update.
func TestDetectInstallMethod_Caches(t *testing.T) {
	u := New()
	cached := DetectResult{Method: InstallPnpm, PnpmAvailable: true, ResolvedPath: "/x/pnpm/store/v11/links/@larksuite/cli/1.0.0/node_modules/@larksuite/cli/bin/lark-cli"}
	u.detectCache = &cached
	got := u.DetectInstallMethod()
	if got.Method != InstallPnpm || !got.PnpmAvailable {
		t.Errorf("expected cached pnpm result to be returned, got %+v", got)
	}
}

func TestSkillsBrandHosts(t *testing.T) {
	cases := []struct {
		brand      core.LarkBrand
		wantIndex  string
		wantSource string
	}{
		{core.BrandFeishu, "https://open.feishu.cn/.well-known/skills/index.json", "https://open.feishu.cn"},
		{core.BrandLark, "https://open.larksuite.com/.well-known/skills/index.json", "https://open.larksuite.com"},
	}
	for _, c := range cases {
		u := &Updater{Brand: c.brand}
		if got := u.skillsIndexURL(); got != c.wantIndex {
			t.Errorf("brand %q: skillsIndexURL = %q, want %q", c.brand, got, c.wantIndex)
		}
		if got := u.skillsSource(); got != c.wantSource {
			t.Errorf("brand %q: skillsSource = %q, want %q", c.brand, got, c.wantSource)
		}
	}
}
