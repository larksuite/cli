// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package gitcred

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestClassifyCredentialConfig(t *testing.T) {
	canonical := "!lark-cli apps git-credential-helper --app-id 'app_xxx'"
	writable := "file:/tmp/global"
	other := "file:/tmp/included"

	tests := []struct {
		name  string
		state credentialConfigState
		want  managedKind
	}{
		{
			name:  "absent",
			state: credentialConfigState{WritableOrigin: writable},
			want:  managedAbsent,
		},
		{
			name: "legacy without useHttpPath",
			state: credentialConfigState{
				WritableOrigin: writable,
				Helpers:        []configValue{{Origin: writable, Value: canonical}},
			},
			want: managedLegacy,
		},
		{
			name: "legacy with useHttpPath true",
			state: credentialConfigState{
				WritableOrigin: writable,
				Helpers:        []configValue{{Origin: writable, Value: canonical}},
				UseHTTPPath:    []configValue{{Origin: writable, Value: "TRUE"}},
			},
			want: managedLegacy,
		},
		{
			name: "current",
			state: credentialConfigState{
				WritableOrigin: writable,
				Helpers: []configValue{
					{Origin: writable, Value: ""},
					{Origin: writable, Value: canonical},
				},
				UseHTTPPath: []configValue{{Origin: writable, Value: "true"}},
			},
			want: managedCurrent,
		},
		{
			name: "third party helper",
			state: credentialConfigState{
				WritableOrigin: writable,
				Helpers:        []configValue{{Origin: writable, Value: "osxkeychain"}},
			},
			want: managedForeign,
		},
		{
			name: "mixed helper",
			state: credentialConfigState{
				WritableOrigin: writable,
				Helpers: []configValue{
					{Origin: writable, Value: canonical},
					{Origin: writable, Value: "osxkeychain"},
				},
			},
			want: managedMixed,
		},
		{
			name: "canonical helper with extra argument",
			state: credentialConfigState{
				WritableOrigin: writable,
				Helpers:        []configValue{{Origin: writable, Value: canonical + " --extra"}},
			},
			want: managedForeign,
		},
		{
			name: "different app id",
			state: credentialConfigState{
				WritableOrigin: writable,
				Helpers:        []configValue{{Origin: writable, Value: "!lark-cli apps git-credential-helper --app-id 'app_other'"}},
			},
			want: managedForeign,
		},
		{
			name: "useHttpPath false",
			state: credentialConfigState{
				WritableOrigin: writable,
				Helpers:        []configValue{{Origin: writable, Value: canonical}},
				UseHTTPPath:    []configValue{{Origin: writable, Value: "false"}},
			},
			want: managedNone,
		},
		{
			name: "multiple useHttpPath values",
			state: credentialConfigState{
				WritableOrigin: writable,
				Helpers:        []configValue{{Origin: writable, Value: canonical}},
				UseHTTPPath: []configValue{
					{Origin: writable, Value: "true"},
					{Origin: writable, Value: "true"},
				},
			},
			want: managedNone,
		},
		{
			name: "current without useHttpPath",
			state: credentialConfigState{
				WritableOrigin: writable,
				Helpers: []configValue{
					{Origin: writable, Value: ""},
					{Origin: writable, Value: canonical},
				},
			},
			want: managedPartial,
		},
		{
			name: "helper from include",
			state: credentialConfigState{
				WritableOrigin: writable,
				Helpers:        []configValue{{Origin: other, Value: canonical}},
			},
			want: managedNone,
		},
		{
			name: "useHttpPath from include",
			state: credentialConfigState{
				WritableOrigin: writable,
				Helpers:        []configValue{{Origin: writable, Value: canonical}},
				UseHTTPPath:    []configValue{{Origin: other, Value: "true"}},
			},
			want: managedNone,
		},
		{
			name: "helpers span origins",
			state: credentialConfigState{
				WritableOrigin: writable,
				Helpers: []configValue{
					{Origin: writable, Value: ""},
					{Origin: other, Value: canonical},
				},
				UseHTTPPath: []configValue{{Origin: writable, Value: "true"}},
			},
			want: managedNone,
		},
		{
			name: "only useHttpPath",
			state: credentialConfigState{
				WritableOrigin: writable,
				UseHTTPPath:    []configValue{{Origin: writable, Value: "true"}},
			},
			want: managedPartial,
		},
		{
			name: "reset marker without canonical helper",
			state: credentialConfigState{
				WritableOrigin: writable,
				Helpers:        []configValue{{Origin: writable, Value: ""}},
			},
			want: managedNone,
		},
		{
			name: "partial reset plus canonical with false useHttpPath is not owned",
			state: credentialConfigState{
				WritableOrigin: writable,
				Helpers: []configValue{
					{Origin: writable, Value: ""},
					{Origin: writable, Value: canonical},
				},
				UseHTTPPath: []configValue{{Origin: writable, Value: "false"}},
			},
			want: managedNone,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyManagedState(tc.state, canonical); got != tc.want {
				t.Fatalf("classifyManagedState() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseOriginValuesPreservesEmptyValue(t *testing.T) {
	raw := []byte("file:/tmp/global\x00\x00file:/tmp/global\x00!helper\x00")
	want := []configValue{
		{Origin: "file:/tmp/global", Value: ""},
		{Origin: "file:/tmp/global", Value: "!helper"},
	}

	got, err := parseOriginValues(raw)
	if err != nil {
		t.Fatalf("parseOriginValues() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseOriginValues() = %#v, want %#v", got, want)
	}
}

func TestParseOriginValuesRejectsMalformedOutput(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
	}{
		{name: "missing terminator", raw: []byte("file:/tmp/global\x00value")},
		{name: "odd token count", raw: []byte("file:/tmp/global\x00value\x00extra\x00")},
		{name: "empty origin", raw: []byte("\x00value\x00")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseOriginValues(tc.raw)
			assertProblemSubtype(t, err, errs.SubtypeExternalTool)
		})
	}
}

func TestWritableGlobalOrigin(t *testing.T) {
	tests := []struct {
		name       string
		globalEnv  string
		homeConfig bool
		xdgConfig  bool
		wantPath   func(home, xdg, global string) string
	}{
		{
			name:      "GIT_CONFIG_GLOBAL takes priority",
			globalEnv: "custom/global.config",
			wantPath: func(_, _, global string) string {
				return global
			},
		},
		{
			name:       "home config takes priority over XDG",
			homeConfig: true,
			xdgConfig:  true,
			wantPath: func(home, _, _ string) string {
				return filepath.Join(home, ".gitconfig")
			},
		},
		{
			name:      "existing XDG config is writable origin",
			xdgConfig: true,
			wantPath: func(_, xdg, _ string) string {
				return filepath.Join(xdg, "git", "config")
			},
		},
		{
			name: "home config is fallback",
			wantPath: func(home, _, _ string) string {
				return filepath.Join(home, ".gitconfig")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			home := filepath.Join(root, "home")
			xdg := filepath.Join(root, "xdg")
			global := ""
			if tc.globalEnv != "" {
				global = tc.globalEnv
			}
			if err := os.MkdirAll(home, 0o755); err != nil {
				t.Fatalf("create home: %v", err)
			}
			if tc.homeConfig {
				writeTestFile(t, filepath.Join(home, ".gitconfig"), nil)
			}
			if tc.xdgConfig {
				writeTestFile(t, filepath.Join(xdg, "git", "config"), nil)
			}
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("XDG_CONFIG_HOME", xdg)
			t.Setenv("GIT_CONFIG_GLOBAL", global)

			got, err := writableGlobalOrigin()
			if err != nil {
				t.Fatalf("writableGlobalOrigin() error = %v", err)
			}
			wantPath := filepath.Clean(tc.wantPath(home, xdg, global))
			if want := "file:" + wantPath; got != want {
				t.Fatalf("writableGlobalOrigin() = %q, want %q", got, want)
			}
		})
	}
}

func TestReadCredentialConfig(t *testing.T) {
	root := t.TempDir()
	globalPath := filepath.Join(root, "global.config")
	includePath := filepath.Join(root, "included.config")
	writeTestFile(t, includePath, []byte("[credential \"https://example.com/git/u/app.git\"]\n\thelper =\n\thelper = !included-helper\n"))
	writeTestFile(t, globalPath, []byte("[include]\n\tpath = "+includePath+"\n[credential \"https://example.com/git/u/app.git\"]\n\tuseHttpPath = true\n"))
	t.Setenv("GIT_CONFIG_GLOBAL", globalPath)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	got, err := readCredentialConfig(context.Background(), "https://example.com/git/u/app.git")
	if err != nil {
		t.Fatalf("readCredentialConfig() error = %v", err)
	}
	want := credentialConfigState{
		Helpers: []configValue{
			{Origin: "file:" + includePath, Value: ""},
			{Origin: "file:" + includePath, Value: "!included-helper"},
		},
		UseHTTPPath:    []configValue{{Origin: "file:" + globalPath, Value: "true"}},
		WritableOrigin: "file:" + globalPath,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("readCredentialConfig() = %#v, want %#v", got, want)
	}
}

func TestReadCredentialConfigTreatsExitOneAsMissing(t *testing.T) {
	globalPath := filepath.Join(t.TempDir(), "missing.config")
	t.Setenv("GIT_CONFIG_GLOBAL", globalPath)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	got, err := readCredentialConfig(context.Background(), "https://example.com/git/u/app.git")
	if err != nil {
		t.Fatalf("readCredentialConfig() error = %v", err)
	}
	want := credentialConfigState{WritableOrigin: "file:" + globalPath}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("readCredentialConfig() = %#v, want %#v", got, want)
	}
}

func TestReadCredentialConfigReturnsTypedExternalToolError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper is a POSIX shell script")
	}
	binDir := t.TempDir()
	writeTestFileMode(t, filepath.Join(binDir, "git"), []byte("#!/bin/sh\nexit 7\n"), 0o755)
	t.Setenv("PATH", binDir)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "global.config"))

	_, err := readCredentialConfig(context.Background(), "https://example.com/git/u/app.git")
	if err == nil {
		t.Fatal("readCredentialConfig() error = nil, want git failure")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Subtype != errs.SubtypeExternalTool {
		t.Fatalf("problem = %#v, ok = %v, want external_tool", problem, ok)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
		t.Fatalf("error cause = %v, want git exit code 7", err)
	}
}

func TestGlobalGitConfigWritesAndMigratesManagedStates(t *testing.T) {
	canonical := "!lark-cli apps git-credential-helper --app-id 'app_xxx'"
	url := "https://example.com/git/u/app.git"
	tests := []struct {
		name    string
		content string
	}{
		{name: "absent"},
		{name: "legacy", content: credentialConfigText(url, []string{canonical}, nil)},
		{name: "current", content: credentialConfigText(url, []string{"", canonical}, []string{"true"})},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			configureIsolatedGlobalGit(t, tc.content)
			cfg := GlobalGitConfig{}
			if err := cfg.SetHelper(context.Background(), url, "app_xxx"); err != nil {
				t.Fatalf("SetHelper() error = %v", err)
			}
			state, err := readCredentialConfig(context.Background(), url)
			if err != nil {
				t.Fatalf("readCredentialConfig() error = %v", err)
			}
			if got := classifyManagedState(state, canonical); got != managedCurrent {
				t.Fatalf("managed state = %v, want current; state=%#v", got, state)
			}
			if got := configValues(state.Helpers); !reflect.DeepEqual(got, []string{"", canonical}) {
				t.Fatalf("helper values = %#v, want reset + canonical", got)
			}
		})
	}
}

func TestGlobalGitConfigResetFirstHelperStopsGlobalStoreAndErase(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helpers are POSIX shell scripts")
	}
	configureIsolatedGlobalGit(t, "")
	root := t.TempDir()
	globalLog := filepath.Join(root, "global.log")
	scopedLog := filepath.Join(root, "scoped.log")
	globalHelper := filepath.Join(root, "global-helper")
	scopedHelper := filepath.Join(root, "scoped-helper")
	writeTestFileMode(t, globalHelper, []byte("#!/bin/sh\nprintf '%s\\n' \"$1\" >> \"$GLOBAL_HELPER_LOG\"\ncat >/dev/null\n"), 0o700)
	writeTestFileMode(t, scopedHelper, []byte("#!/bin/sh\nprintf '%s\\n' \"$1\" >> \"$SCOPED_HELPER_LOG\"\ncat >/dev/null\n"), 0o700)
	t.Setenv("GLOBAL_HELPER_LOG", globalLog)
	t.Setenv("SCOPED_HELPER_LOG", scopedLog)

	globalCommand := "!" + shellQuoteArg(globalHelper)
	if out, err := exec.Command("git", "config", "--global", "credential.helper", globalCommand).CombinedOutput(); err != nil {
		t.Fatalf("configure global helper: %v: %s", err, out)
	}
	url := "https://example.com/git/u/app.git"
	scopedCommand := "!" + shellQuoteArg(scopedHelper)
	if err := (GlobalGitConfig{HelperCommand: scopedCommand}).SetHelper(context.Background(), url, "app_xxx"); err != nil {
		t.Fatalf("SetHelper() error = %v", err)
	}

	credential := []byte("protocol=https\nhost=example.com\npath=git/u/app.git\nusername=x-access-token\npassword=test-only\n\n")
	for _, operation := range []string{"approve", "reject"} {
		cmd := exec.Command("git", "credential", operation)
		cmd.Stdin = bytes.NewReader(credential)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git credential %s: %v: %s", operation, err, out)
		}
	}
	if raw, err := os.ReadFile(scopedLog); err != nil || string(raw) != "store\nerase\n" {
		t.Fatalf("scoped helper log = %q, err = %v, want store and erase", raw, err)
	}
	if raw, err := os.ReadFile(globalLog); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("global helper unexpectedly ran: log=%q err=%v", raw, err)
	}
}

func TestGlobalGitConfigRollsBackWhenUseHTTPPathWriteFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test wrapper is a POSIX shell script")
	}
	url := "https://example.com/git/u/app.git"
	configureIsolatedGlobalGit(t, "")
	installGitConfigUseHTTPPathFailure(t, url, false, false)

	err := (GlobalGitConfig{}).SetHelper(context.Background(), url, "app_xxx")
	assertExternalToolExit(t, err, 23)
	state, readErr := readCredentialConfig(context.Background(), url)
	if readErr != nil {
		t.Fatalf("readCredentialConfig() error = %v", readErr)
	}
	if len(state.Helpers) != 0 || len(state.UseHTTPPath) != 0 {
		t.Fatalf("failed transaction was not rolled back: %#v", state)
	}
}

func TestGlobalGitConfigDoesNotRollbackOverExternalChange(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test wrapper is a POSIX shell script")
	}
	url := "https://example.com/git/u/app.git"
	configureIsolatedGlobalGit(t, "")
	installGitConfigUseHTTPPathFailure(t, url, true, false)

	err := (GlobalGitConfig{}).SetHelper(context.Background(), url, "app_xxx")
	assertExternalToolExit(t, err, 23)
	state, readErr := readCredentialConfig(context.Background(), url)
	if readErr != nil {
		t.Fatalf("readCredentialConfig() error = %v", readErr)
	}
	want := []string{"", "!lark-cli apps git-credential-helper --app-id 'app_xxx'", "!external-change"}
	if got := configValues(state.Helpers); !reflect.DeepEqual(got, want) {
		t.Fatalf("helpers after external change = %#v, want preserved %#v", got, want)
	}
}

func TestGlobalGitConfigRollbackFailurePreservesFirstTypedCauseAndStaticHint(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test wrapper is a POSIX shell script")
	}
	url := "https://example.com/git/u/app.git"
	configureIsolatedGlobalGit(t, "")
	installGitConfigUseHTTPPathFailure(t, url, false, true)

	err := (GlobalGitConfig{}).SetHelper(context.Background(), url, "app_xxx")
	assertExternalToolExit(t, err, 23)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Hint != rollbackFailureHint {
		t.Fatalf("problem = %#v, ok = %v, want static rollback hint", problem, ok)
	}
	if strings.Contains(err.Error(), "sensitive-rollback-detail") {
		t.Fatalf("rollback error leaked command detail: %q", err.Error())
	}
}

func TestGlobalGitConfigRefusesNonOwnedStates(t *testing.T) {
	canonical := "!lark-cli apps git-credential-helper --app-id 'app_xxx'"
	url := "https://example.com/git/u/app.git"
	tests := []struct {
		name    string
		content string
	}{
		{name: "third party", content: credentialConfigText(url, []string{"osxkeychain"}, nil)},
		{name: "mixed", content: credentialConfigText(url, []string{canonical, "osxkeychain"}, nil)},
		{name: "useHttpPath false", content: credentialConfigText(url, []string{canonical}, []string{"false"})},
		{name: "different app id", content: credentialConfigText(url, []string{"!lark-cli apps git-credential-helper --app-id 'app_other'"}, nil)},
		{name: "extra helper argument", content: credentialConfigText(url, []string{canonical + " --extra"}, nil)},
		{name: "command injection suffix", content: credentialConfigText(url, []string{canonical + " && /bin/false"}, nil)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := configureIsolatedGlobalGit(t, tc.content)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read config before SetHelper: %v", err)
			}
			err = (GlobalGitConfig{}).SetHelper(context.Background(), url, "app_xxx")
			assertProblemSubtype(t, err, errs.SubtypeFailedPrecondition)
			after, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("read config after SetHelper: %v", readErr)
			}
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("non-owned config changed:\nbefore=%s\nafter=%s", before, after)
			}
		})
	}
}

func TestGlobalGitConfigRefusesManagedStateFromIncludedConfig(t *testing.T) {
	url := "https://example.com/git/u/app.git"
	canonical := "!lark-cli apps git-credential-helper --app-id 'app_xxx'"
	root := t.TempDir()
	globalPath := filepath.Join(root, "global.config")
	includePath := filepath.Join(root, "included.config")
	includeContent := credentialConfigText(url, []string{"", canonical}, []string{"true"})
	writeTestFile(t, includePath, []byte(includeContent))
	writeTestFile(t, globalPath, []byte(fmt.Sprintf("[include]\n\tpath = %s\n", includePath)))
	t.Setenv("GIT_CONFIG_GLOBAL", globalPath)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	beforeGlobal, err := os.ReadFile(globalPath)
	if err != nil {
		t.Fatalf("read global config: %v", err)
	}
	beforeInclude, err := os.ReadFile(includePath)
	if err != nil {
		t.Fatalf("read included config: %v", err)
	}
	err = (GlobalGitConfig{}).SetHelper(context.Background(), url, "app_xxx")
	assertProblemSubtype(t, err, errs.SubtypeFailedPrecondition)
	afterGlobal, _ := os.ReadFile(globalPath)
	afterInclude, _ := os.ReadFile(includePath)
	if !bytes.Equal(afterGlobal, beforeGlobal) || !bytes.Equal(afterInclude, beforeInclude) {
		t.Fatalf("cross-origin config changed:\nglobal=%s\ninclude=%s", afterGlobal, afterInclude)
	}
}

func TestGlobalGitConfigUnsetOnlyExactManagedState(t *testing.T) {
	canonical := "!lark-cli apps git-credential-helper --app-id 'app_xxx'"
	url := "https://example.com/git/u/app.git"
	tests := []struct {
		name        string
		content     string
		wantHelpers []string
		wantUsePath []string
	}{
		{name: "legacy", content: credentialConfigText(url, []string{canonical}, nil)},
		{name: "legacy with useHttpPath", content: credentialConfigText(url, []string{canonical}, []string{"true"})},
		{name: "current", content: credentialConfigText(url, []string{"", canonical}, []string{"true"})},
		{name: "different app", content: credentialConfigText(url, []string{"!lark-cli apps git-credential-helper --app-id 'app_other'"}, []string{"true"}), wantHelpers: []string{"!lark-cli apps git-credential-helper --app-id 'app_other'"}, wantUsePath: []string{"true"}},
		{name: "only useHttpPath", content: credentialConfigText(url, nil, []string{"true"})},
		{name: "managed helper with false useHttpPath", content: credentialConfigText(url, []string{canonical}, []string{"false"}), wantHelpers: []string{canonical}, wantUsePath: []string{"false"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			configureIsolatedGlobalGit(t, tc.content)
			if err := (GlobalGitConfig{}).UnsetHelper(context.Background(), url, "app_xxx"); err != nil {
				t.Fatalf("UnsetHelper() error = %v", err)
			}
			state, err := readCredentialConfig(context.Background(), url)
			if err != nil {
				t.Fatalf("readCredentialConfig() error = %v", err)
			}
			if got := configValues(state.Helpers); !reflect.DeepEqual(got, tc.wantHelpers) {
				t.Fatalf("helper values = %#v, want %#v", got, tc.wantHelpers)
			}
			if got := configValues(state.UseHTTPPath); !reflect.DeepEqual(got, tc.wantUsePath) {
				t.Fatalf("useHttpPath values = %#v, want %#v", got, tc.wantUsePath)
			}
		})
	}
}

// TestGlobalGitConfigMigratesWithNonCanonicalGlobalPath pins the fix for
// non-canonical GIT_CONFIG_GLOBAL values. Git echoes the path verbatim in
// --show-origin (e.g. "file:/tmp/x/./sub//global.config" for a path with
// embedded "./" or "//"), so the writable origin this package derives must
// normalize to the same cleaned form or a legacy managed helper is
// misclassified as non-owned and migration is refused.
func TestGlobalGitConfigMigratesWithNonCanonicalGlobalPath(t *testing.T) {
	canonical := "!lark-cli apps git-credential-helper --app-id 'app_xxx'"
	url := "https://example.com/git/u/app.git"
	dir := t.TempDir()
	cleanPath := filepath.Join(dir, "sub", "global.config")
	writeTestFile(t, cleanPath, []byte(credentialConfigText(url, []string{canonical}, nil)))
	// Point GIT_CONFIG_GLOBAL at the same file via a non-canonical path so git
	// echoes the uncleaned form in --show-origin.
	nonCanonical := filepath.Join(dir, "sub") + string(filepath.Separator) + "." + string(filepath.Separator) + "global.config"
	t.Setenv("GIT_CONFIG_GLOBAL", nonCanonical)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	state, err := readCredentialConfig(context.Background(), url)
	if err != nil {
		t.Fatalf("readCredentialConfig() error = %v", err)
	}
	if got := classifyManagedState(state, canonical); got != managedLegacy {
		t.Fatalf("managed state = %v, want legacy; state=%#v", got, state)
	}
	if err := (GlobalGitConfig{}).SetHelper(context.Background(), url, "app_xxx"); err != nil {
		t.Fatalf("SetHelper() error = %v", err)
	}
	after, err := readCredentialConfig(context.Background(), url)
	if err != nil {
		t.Fatalf("readCredentialConfig() error = %v", err)
	}
	if got := classifyManagedState(after, canonical); got != managedCurrent {
		t.Fatalf("managed state after migration = %v, want current; state=%#v", got, after)
	}
}

// TestGlobalGitConfigResetFirstHelperStopsGlobalFill proves, through real
// `git credential fill`, that after the empty-helper reset the earlier global
// helper is never asked to return credentials for the scoped URL — closing the
// durable-plumbing gap where only approve/reject (store/erase) were covered.
func TestGlobalGitConfigResetFirstHelperStopsGlobalFill(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helpers are POSIX shell scripts")
	}
	configureIsolatedGlobalGit(t, "")
	root := t.TempDir()
	globalLog := filepath.Join(root, "global.log")
	scopedLog := filepath.Join(root, "scoped.log")
	globalHelper := filepath.Join(root, "global-helper")
	scopedHelper := filepath.Join(root, "scoped-helper")
	// The global helper returns a full credential on `get`; if the reset did
	// not isolate it, fill would short-circuit here and never reach the scoped
	// helper.
	writeTestFileMode(t, globalHelper, []byte("#!/bin/sh\nprintf '%s\\n' \"$1\" >> \"$GLOBAL_HELPER_LOG\"\ncat >/dev/null\nif [ \"$1\" = get ]; then printf 'username=global-user\\npassword=global-pass\\n'; fi\n"), 0o700)
	writeTestFileMode(t, scopedHelper, []byte("#!/bin/sh\nprintf '%s\\n' \"$1\" >> \"$SCOPED_HELPER_LOG\"\ncat >/dev/null\nif [ \"$1\" = get ]; then printf 'username=scoped-user\\npassword=scoped-pass\\n'; fi\n"), 0o700)
	t.Setenv("GLOBAL_HELPER_LOG", globalLog)
	t.Setenv("SCOPED_HELPER_LOG", scopedLog)

	if out, err := exec.Command("git", "config", "--global", "credential.helper", "!"+shellQuoteArg(globalHelper)).CombinedOutput(); err != nil {
		t.Fatalf("configure global helper: %v: %s", err, out)
	}
	url := "https://example.com/git/u/app.git"
	if err := (GlobalGitConfig{HelperCommand: "!" + shellQuoteArg(scopedHelper)}).SetHelper(context.Background(), url, "app_xxx"); err != nil {
		t.Fatalf("SetHelper() error = %v", err)
	}

	cmd := exec.Command("git", "credential", "fill")
	cmd.Stdin = strings.NewReader("protocol=https\nhost=example.com\npath=git/u/app.git\n\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git credential fill: %v: %s", err, out)
	}
	if creds := parseCredentialFields(string(out)); creds["username"] != "scoped-user" || creds["password"] != "scoped-pass" {
		t.Fatalf("fill returned %v, want scoped helper credential", creds)
	}
	if raw, err := os.ReadFile(scopedLog); err != nil || string(raw) != "get\n" {
		t.Fatalf("scoped helper log = %q, err = %v, want single get", raw, err)
	}
	if raw, err := os.ReadFile(globalLog); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("global helper unexpectedly ran during fill: log=%q err=%v", raw, err)
	}
}

func readLogLines(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

// TestGlobalGitConfigDefaultHelperShellQuotesAppID proves at execution level
// that a shell-metacharacter-laden appID (permitted by validate.ResourceName,
// which only blocks ?#% / control chars / traversal) is passed to the helper
// as a single --app-id argument and does not execute an injected command. This
// restores execution-level shell-quoting coverage for the default helper.
func TestGlobalGitConfigDefaultHelperShellQuotesAppID(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("helper resolution relies on a POSIX shell")
	}
	configureIsolatedGlobalGit(t, "")
	root := t.TempDir()
	argLog := filepath.Join(root, "args.log")
	sentinel := filepath.Join(root, "pwned")
	// Injecting a real lark-cli is out of scope; instead point the helper
	// command at a fake `lark-cli` on PATH that records its literal arguments.
	binDir := filepath.Join(root, "bin")
	fakeCLI := filepath.Join(binDir, "lark-cli")
	writeTestFileMode(t, fakeCLI, []byte("#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\" >> \"$ARG_LOG\"; done\ncat >/dev/null\nfor a in \"$@\"; do if [ \"$a\" = get ]; then printf 'username=fake-user\\npassword=fake-pass\\n'; fi; done\n"), 0o700)
	t.Setenv("ARG_LOG", argLog)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	appID := "app_xxx; touch " + sentinel
	url := "https://example.com/git/u/app.git"
	if err := (GlobalGitConfig{}).SetHelper(context.Background(), url, appID); err != nil {
		t.Fatalf("SetHelper() error = %v", err)
	}
	cmd := exec.Command("git", "credential", "fill")
	cmd.Stdin = strings.NewReader("protocol=https\nhost=example.com\npath=git/u/app.git\n\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git credential fill: %v: %s", err, out)
	}
	if _, err := os.Stat(sentinel); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("injected command executed: sentinel exists (err=%v)", err)
	}
	args := readLogLines(t, argLog)
	// The fake CLI receives: apps git-credential-helper --app-id <appID> get
	foundArg := false
	for i, a := range args {
		if a == "--app-id" {
			if i+1 >= len(args) || args[i+1] != appID {
				t.Fatalf("--app-id argument = %q, want single literal %q; args=%#v", args[i+1:], appID, args)
			}
			foundArg = true
		}
	}
	if !foundArg {
		t.Fatalf("helper did not receive --app-id argument; args=%#v", args)
	}
}

func parseCredentialFields(output string) map[string]string {
	fields := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if key, value, ok := strings.Cut(line, "="); ok {
			fields[key] = value
		}
	}
	return fields
}

func configureIsolatedGlobalGit(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "global.config")
	writeTestFile(t, path, []byte(content))
	t.Setenv("GIT_CONFIG_GLOBAL", path)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	return path
}

func credentialConfigText(url string, helpers, useHTTPPath []string) string {
	if len(helpers) == 0 && len(useHTTPPath) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("[credential \"")
	b.WriteString(url)
	b.WriteString("\"]\n")
	for _, helper := range helpers {
		b.WriteString("\thelper = ")
		b.WriteString(helper)
		b.WriteByte('\n')
	}
	for _, value := range useHTTPPath {
		b.WriteString("\tuseHttpPath = ")
		b.WriteString(value)
		b.WriteByte('\n')
	}
	return b.String()
}

func configValues(values []configValue) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, len(values))
	for i := range values {
		result[i] = values[i].Value
	}
	return result
}

func assertProblemSubtype(t *testing.T, err error, subtype errs.Subtype) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want subtype %s", subtype)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Subtype != subtype {
		t.Fatalf("problem = %#v, ok = %v, want subtype %s", problem, ok, subtype)
	}
}

func assertExternalToolExit(t *testing.T, err error, exitCode int) {
	t.Helper()
	assertProblemSubtype(t, err, errs.SubtypeExternalTool)
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != exitCode {
		t.Fatalf("error cause = %v, want git exit code %d", err, exitCode)
	}
}

func installGitConfigUseHTTPPathFailure(t *testing.T, url string, externalChange, rollbackFailure bool) {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("find real git: %v", err)
	}
	binDir := t.TempDir()
	gitPath := filepath.Join(binDir, "git")
	script := `#!/bin/sh
if [ "$1" = "config" ] && [ "$2" = "--global" ] && [ "$3" = "$GIT_TEST_USE_PATH_KEY" ] && [ "$4" = "true" ]; then
  if [ "$GIT_TEST_EXTERNAL_CHANGE" = "1" ]; then
    "$GIT_TEST_REAL_GIT" config --global --add "$GIT_TEST_HELPER_KEY" '!external-change'
  fi
  exit 23
fi
if [ "$GIT_TEST_ROLLBACK_FAILURE" = "1" ] && [ "$1" = "config" ] && [ "$2" = "--global" ] && [ "$3" = "--unset-all" ] && [ "$4" = "$GIT_TEST_HELPER_KEY" ]; then
  if "$GIT_TEST_REAL_GIT" config --global --get-all "$GIT_TEST_HELPER_KEY" >/dev/null 2>&1; then
    echo 'sensitive-rollback-detail' >&2
    exit 24
  fi
fi
exec "$GIT_TEST_REAL_GIT" "$@"
`
	writeTestFileMode(t, gitPath, []byte(script), 0o700)
	t.Setenv("GIT_TEST_REAL_GIT", realGit)
	t.Setenv("GIT_TEST_HELPER_KEY", gitCredentialKey(url, "helper"))
	t.Setenv("GIT_TEST_USE_PATH_KEY", gitCredentialKey(url, "useHttpPath"))
	t.Setenv("GIT_TEST_EXTERNAL_CHANGE", fmt.Sprint(boolToInt(externalChange)))
	t.Setenv("GIT_TEST_ROLLBACK_FAILURE", fmt.Sprint(boolToInt(rollbackFailure)))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func writeTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	writeTestFileMode(t, path, data, 0o600)
}

func writeTestFileMode(t *testing.T, path string, data []byte, mode fs.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent directory: %v", err)
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestGlobalGitConfigResetDefeatsLaterGlobalHelperOnFill proves through real
// `git credential fill` (and store/erase) that when a generic
// credential.helper is configured AFTER the URL-scoped lark-cli section — the
// case an empty-helper reset alone does NOT isolate, because git applies
// helpers in parse order — SetHelper repositions the lark-cli section so the
// reset defeats the later helper. Covers both a textually-later [credential]
// section and one sourced from a later [include].
func TestGlobalGitConfigResetDefeatsLaterGlobalHelperOnFill(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helpers are POSIX shell scripts")
	}
	url := "https://example.com/git/u/app.git"
	scopedSection := credentialConfigText(url, []string{"", "SCOPED_CMD"}, []string{"true"})

	tests := []struct {
		name    string
		content func(globalCmd, includePath string) string
	}{
		{
			name: "later generic section",
			content: func(globalCmd, _ string) string {
				return scopedSection + "[credential]\n\thelper = " + globalCmd + "\n"
			},
		},
		{
			name: "later include",
			content: func(_, includePath string) string {
				return scopedSection + "[include]\n\tpath = " + includePath + "\n"
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			globalLog := filepath.Join(root, "global.log")
			scopedLog := filepath.Join(root, "scoped.log")
			globalHelper := filepath.Join(root, "global-helper")
			scopedHelper := filepath.Join(root, "scoped-helper")
			writeTestFileMode(t, globalHelper, []byte("#!/bin/sh\nprintf '%s\\n' \"$1\" >> \"$GLOBAL_HELPER_LOG\"\ncat >/dev/null\nif [ \"$1\" = get ]; then printf 'username=global-user\\npassword=global-pass\\n'; fi\n"), 0o700)
			writeTestFileMode(t, scopedHelper, []byte("#!/bin/sh\nprintf '%s\\n' \"$1\" >> \"$SCOPED_HELPER_LOG\"\ncat >/dev/null\nif [ \"$1\" = get ]; then printf 'username=scoped-user\\npassword=scoped-pass\\n'; fi\n"), 0o700)
			t.Setenv("GLOBAL_HELPER_LOG", globalLog)
			t.Setenv("SCOPED_HELPER_LOG", scopedLog)

			globalCmd := "!" + shellQuoteArg(globalHelper)
			scopedCmd := "!" + shellQuoteArg(scopedHelper)
			includePath := filepath.Join(root, "included.config")
			writeTestFile(t, includePath, []byte("[credential]\n\thelper = "+globalCmd+"\n"))

			content := strings.ReplaceAll(tc.content(globalCmd, includePath), "SCOPED_CMD", scopedCmd)
			configureIsolatedGlobalGit(t, content)

			if err := (GlobalGitConfig{HelperCommand: scopedCmd}).SetHelper(context.Background(), url, "app_xxx"); err != nil {
				t.Fatalf("SetHelper() error = %v", err)
			}

			// fill: the scoped helper answers; the later global helper must not run.
			cmd := exec.Command("git", "credential", "fill")
			cmd.Stdin = strings.NewReader("protocol=https\nhost=example.com\npath=git/u/app.git\n\n")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("git credential fill: %v: %s", err, out)
			}
			if creds := parseCredentialFields(string(out)); creds["username"] != "scoped-user" || creds["password"] != "scoped-pass" {
				t.Fatalf("fill returned %v, want scoped helper credential", creds)
			}

			// store/erase invoke every helper in the list; the later global helper
			// must not be reached for this URL.
			credential := []byte("protocol=https\nhost=example.com\npath=git/u/app.git\nusername=x-access-token\npassword=test-only\n\n")
			for _, operation := range []string{"approve", "reject"} {
				op := exec.Command("git", "credential", operation)
				op.Stdin = bytes.NewReader(credential)
				if o, err := op.CombinedOutput(); err != nil {
					t.Fatalf("git credential %s: %v: %s", operation, err, o)
				}
			}
			if raw, err := os.ReadFile(globalLog); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("global helper unexpectedly ran: log=%q err=%v", raw, err)
			}
			if raw := readLogLines(t, scopedLog); !reflect.DeepEqual(raw, []string{"get", "store", "erase"}) {
				t.Fatalf("scoped helper log = %#v, want get/store/erase", raw)
			}
		})
	}
}

// TestGlobalGitConfigFailsClosedWhenLaterHelperCannotBeDefeated verifies the
// fail-closed guarantee: if a generic credential.helper is applied AFTER the
// lark-cli helper even after repositioning (i.e. it lives somewhere the writable
// file cannot move past), SetHelper must return FailedPrecondition and restore
// the prior configuration rather than leave a written-but-ineffective reset.
//
// Repositioning always wins within a single --global file, so to exercise the
// residual un-defeatable case deterministically we wrap git to inject a phantom
// later generic helper into every `--list` (the parse-order oracle), which no
// on-disk reposition can remove. The real config file is otherwise unmodified,
// so we can assert the lark-cli section was rolled back to its pre-write state.
func TestGlobalGitConfigFailsClosedWhenLaterHelperCannotBeDefeated(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test wrapper is a POSIX shell script")
	}
	url := "https://example.com/git/u/app.git"
	globalPath := configureIsolatedGlobalGit(t, "")
	installPhantomLaterHelper(t)

	err := (GlobalGitConfig{}).SetHelper(context.Background(), url, "app_xxx")
	assertProblemSubtype(t, err, errs.SubtypeFailedPrecondition)
	if !strings.Contains(err.Error(), "applied after") {
		t.Fatalf("error = %v, want message mentioning a helper applied after the reset", err)
	}

	// The phantom exists only in --list output, so on-disk state must be rolled
	// back to no lark-cli-owned credential configuration for the URL.
	state, readErr := readCredentialConfig(context.Background(), url)
	if readErr != nil {
		t.Fatalf("readCredentialConfig() error = %v", readErr)
	}
	if got := classifyManagedState(state, "!lark-cli apps git-credential-helper --app-id 'app_xxx'"); got != managedAbsent {
		t.Fatalf("state after fail-closed = %v (helpers=%#v), want absent", got, state.Helpers)
	}
	raw, err := os.ReadFile(globalPath)
	if err != nil {
		t.Fatalf("read config file: %v", err)
	}
	if strings.Contains(string(raw), "lark-cli") {
		t.Fatalf("lark-cli helper not rolled back from config file:\n%s", raw)
	}
}

// installPhantomLaterHelper wraps git so that `git config ... -z --list` always
// reports an extra generic credential.helper AFTER the real entries, modeling a
// later-parsing helper that repositioning the writable file cannot defeat. All
// other git invocations pass through unchanged.
func installPhantomLaterHelper(t *testing.T) {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("find real git: %v", err)
	}
	binDir := t.TempDir()
	gitPath := filepath.Join(binDir, "git")
	script := `#!/bin/sh
if [ "$1" = "config" ] && [ "$2" = "--global" ] && [ "$3" = "--includes" ] && [ "$4" = "--show-origin" ] && [ "$5" = "-z" ] && [ "$6" = "--list" ]; then
  "$GIT_TEST_REAL_GIT" "$@" || exit $?
  printf 'file:/phantom\0credential.helper\n!phantom-later\0'
  exit 0
fi
exec "$GIT_TEST_REAL_GIT" "$@"
`
	writeTestFileMode(t, gitPath, []byte(script), 0o700)
	t.Setenv("GIT_TEST_REAL_GIT", realGit)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestGlobalGitConfigDetectsConcurrentHelperInsertion models a non-lark-cli
// writer that inserts a foreign helper into the URL-scoped list during the
// write window (after the reset unset, before readback). lockGlobalConfig
// serializes lark-cli writers against each other but cannot stop an unrelated
// process, so this must be caught by readback and fail closed with the foreign
// value preserved — never silently overwritten.
func TestGlobalGitConfigDetectsConcurrentHelperInsertion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test wrapper is a POSIX shell script")
	}
	url := "https://example.com/git/u/app.git"
	configureIsolatedGlobalGit(t, "")
	installGitConfigUseHTTPPathFailure(t, url, true, false)

	err := (GlobalGitConfig{}).SetHelper(context.Background(), url, "app_xxx")
	assertExternalToolExit(t, err, 23)

	state, readErr := readCredentialConfig(context.Background(), url)
	if readErr != nil {
		t.Fatalf("readCredentialConfig() error = %v", readErr)
	}
	foundForeign := false
	for _, h := range state.Helpers {
		if h.Value == "!external-change" {
			foundForeign = true
		}
	}
	if !foundForeign {
		t.Fatalf("concurrently inserted foreign helper was lost: %#v", state.Helpers)
	}
}

// TestGlobalGitConfigPreservesForeignHelperInsertedBeforeReset models the
// narrower race the previous whole-key --unset-all could not survive: a
// non-lark-cli process inserts a foreign helper into the URL-scoped list AFTER
// the ownership read but BEFORE lark-cli begins rewriting the helper list. A
// whole-key --unset-all would delete that foreign value and the canonical
// re-add would leave a readback matching `known`, so the third party's write
// would vanish and SetHelper would (wrongly) report success. Because the
// rewrite now deletes only the snapshot's own values, the foreign helper
// survives, readback diverges from `known`, and SetHelper fails closed with the
// foreign value preserved.
func TestGlobalGitConfigPreservesForeignHelperInsertedBeforeReset(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test wrapper is a POSIX shell script")
	}
	url := "https://example.com/git/u/app.git"
	configureIsolatedGlobalGit(t, "")
	installForeignHelperBeforeFirstHelperAdd(t, url)

	err := (GlobalGitConfig{}).SetHelper(context.Background(), url, "app_xxx")
	assertProblemSubtype(t, err, errs.SubtypeFailedPrecondition)

	state, readErr := readCredentialConfig(context.Background(), url)
	if readErr != nil {
		t.Fatalf("readCredentialConfig() error = %v", readErr)
	}
	foundForeign := false
	for _, h := range state.Helpers {
		if h.Value == "!external-race" {
			foundForeign = true
		}
	}
	if !foundForeign {
		t.Fatalf("foreign helper inserted before the reset was silently deleted: %#v", state.Helpers)
	}
}

// installForeignHelperBeforeFirstHelperAdd wraps git so that the FIRST time
// lark-cli runs `git config --global --add <helperKey> ""` (the empty reset
// marker that begins the helper rewrite), a foreign helper is first inserted
// into the same list via real git. This deterministically reproduces a
// concurrent non-lark writer landing in the window between the ownership read
// and the helper rewrite. All other git invocations pass through unchanged.
func installForeignHelperBeforeFirstHelperAdd(t *testing.T, url string) {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("find real git: %v", err)
	}
	binDir := t.TempDir()
	gitPath := filepath.Join(binDir, "git")
	script := `#!/bin/sh
if [ "$1" = "config" ] && [ "$2" = "--global" ] && [ "$3" = "--add" ] && [ "$4" = "$GIT_TEST_HELPER_KEY" ] && [ -z "$5" ]; then
  if [ ! -f "$GIT_TEST_FLAG" ]; then
    : > "$GIT_TEST_FLAG"
    "$GIT_TEST_REAL_GIT" config --global --add "$GIT_TEST_HELPER_KEY" '!external-race'
  fi
fi
exec "$GIT_TEST_REAL_GIT" "$@"
`
	writeTestFileMode(t, gitPath, []byte(script), 0o700)
	t.Setenv("GIT_TEST_REAL_GIT", realGit)
	t.Setenv("GIT_TEST_HELPER_KEY", gitCredentialKey(url, "helper"))
	t.Setenv("GIT_TEST_FLAG", filepath.Join(binDir, "flag"))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestLockGlobalConfigSerializesWriters runs concurrent SetHelper calls for the
// same app ID against the same global config file. lockGlobalConfig must
// serialize the read-modify-write so the writes do not interleave: the final
// config is exactly one valid managedCurrent state ([reset, canonical] + one
// useHttpPath), never duplicated or torn helper values.
func TestLockGlobalConfigSerializesWriters(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cross-process file lock behavior is validated on POSIX")
	}
	url := "https://example.com/git/u/app.git"
	canonical := "!lark-cli apps git-credential-helper --app-id 'app_xxx'"
	configureIsolatedGlobalGit(t, "")

	const writers = 4
	var wg sync.WaitGroup
	errCh := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- (GlobalGitConfig{}).SetHelper(context.Background(), url, "app_xxx")
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent SetHelper() error = %v", err)
		}
	}

	state, err := readCredentialConfig(context.Background(), url)
	if err != nil {
		t.Fatalf("readCredentialConfig() error = %v", err)
	}
	if got := configValues(state.Helpers); !reflect.DeepEqual(got, []string{"", canonical}) {
		t.Fatalf("final helpers = %#v, want exactly [reset, canonical] (no torn/duplicated writes)", got)
	}
	if got := configValues(state.UseHTTPPath); !reflect.DeepEqual(got, []string{"true"}) {
		t.Fatalf("final useHttpPath = %#v, want exactly [true]", got)
	}
	if got := classifyManagedState(state, canonical); got != managedCurrent {
		t.Fatalf("final managed state = %v, want current; state=%#v", got, state)
	}
}

// TestUnsetHelperUseHTTPPathDeleteFailureLeavesRecoverableState proves the new
// delete order (useHttpPath first, then helper) keeps the residue recoverable
// when the helper delete fails: the leftover helper (without useHttpPath) still
// classifies as a lark-cli-owned state, so a later SetHelper can re-normalize
// it — unlike the old order, which could leave a useHttpPath-only orphan that
// SetHelper refuses.
func TestUnsetHelperUseHTTPPathDeleteFailureLeavesRecoverableState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test wrapper is a POSIX shell script")
	}
	url := "https://example.com/git/u/app.git"
	canonical := "!lark-cli apps git-credential-helper --app-id 'app_xxx'"
	configureIsolatedGlobalGit(t, credentialConfigText(url, []string{"", canonical}, []string{"true"}))
	installGitConfigHelperUnsetFailure(t, url, 29)

	err := (GlobalGitConfig{}).UnsetHelper(context.Background(), url, "app_xxx")
	assertExternalToolExit(t, err, 29)

	state, readErr := readCredentialConfig(context.Background(), url)
	if readErr != nil {
		t.Fatalf("readCredentialConfig() error = %v", readErr)
	}
	// The helper delete failed; useHttpPath was deleted first and then restored
	// best-effort. Whatever the exact residue, it must be a lark-cli-owned
	// (recoverable) state — never a useHttpPath-only orphan (managedNone) that a
	// later SetHelper would refuse to re-init.
	kind := classifyManagedState(state, canonical)
	if kind != managedLegacy && kind != managedCurrent && kind != managedPartial {
		t.Fatalf("residue classified %v, want a recoverable lark-cli-owned state; state=%#v", kind, state)
	}
	if len(state.Helpers) == 0 {
		t.Fatalf("helper delete failure lost the helper list, leaving an unrecoverable residue; state=%#v", state)
	}
	// A recoverable residue is one SetHelper's ownership gate accepts; assert
	// that directly rather than re-running SetHelper under the injected fault
	// (whose whole-key helper unset SetHelper's own reset would also trip).
	if !setHelperAcceptsState(kind) {
		t.Fatalf("SetHelper would refuse to re-init residue classified %v; state=%#v", kind, state)
	}
}

// TestUnsetHelperMixedOwnedRemovesOnlyLarkValues proves that when the URL-scoped
// helper list mixes the lark-cli values with a foreign helper, UnsetHelper
// removes only the lark-cli values (the empty reset and the canonical helper)
// and the lark-cli useHttpPath, leaves the foreign helper intact, and returns a
// non-nil warning so callers surface that a foreign helper remains.
func TestUnsetHelperMixedOwnedRemovesOnlyLarkValues(t *testing.T) {
	url := "https://example.com/git/u/app.git"
	canonical := "!lark-cli apps git-credential-helper --app-id 'app_xxx'"
	configureIsolatedGlobalGit(t, credentialConfigText(url, []string{"", canonical, "osxkeychain"}, []string{"true"}))

	err := (GlobalGitConfig{}).UnsetHelper(context.Background(), url, "app_xxx")
	assertProblemSubtype(t, err, errs.SubtypeFailedPrecondition)

	state, readErr := readCredentialConfig(context.Background(), url)
	if readErr != nil {
		t.Fatalf("readCredentialConfig() error = %v", readErr)
	}
	if got := configValues(state.Helpers); !reflect.DeepEqual(got, []string{"osxkeychain"}) {
		t.Fatalf("helpers after unset = %#v, want only the foreign helper", got)
	}
	if len(state.UseHTTPPath) != 0 {
		t.Fatalf("lark-cli useHttpPath not removed: %#v", state.UseHTTPPath)
	}
}

// installGitConfigHelperUnsetFailure wraps git so the URL-scoped helper
// `--unset-all` (the whole-key form, with no value pattern) fails with exitCode,
// while all other git invocations — including the useHttpPath unset — pass
// through. Used to exercise UnsetHelper's helper-delete failure path.
func installGitConfigHelperUnsetFailure(t *testing.T, url string, exitCode int) {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("find real git: %v", err)
	}
	binDir := t.TempDir()
	gitPath := filepath.Join(binDir, "git")
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "config" ] && [ "$2" = "--global" ] && [ "$3" = "--unset-all" ] && [ "$4" = "$GIT_TEST_HELPER_KEY" ] && [ -z "$5" ]; then
  exit %d
fi
exec "$GIT_TEST_REAL_GIT" "$@"
`, exitCode)
	writeTestFileMode(t, gitPath, []byte(script), 0o700)
	t.Setenv("GIT_TEST_REAL_GIT", realGit)
	t.Setenv("GIT_TEST_HELPER_KEY", gitCredentialKey(url, "helper"))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestGlobalGitConfigResetDefeatsLaterHelperForUppercaseURLPath guards against
// lowercasing the URL subsection when matching credential.helper entries in the
// parse-order oracle. NormalizeGitHTTPURL preserves the URL path case, and git
// stores/prints the subsection verbatim, so a URL with an uppercase path
// segment must still be recognized as the lark-cli helper. If readHelperFillOrder
// lowercased the scoped key, the lark helper would not be found in the order,
// SetHelper would wrongly hit the fail-closed path, and this real fill would
// return the later global helper's credential instead of the scoped one.
func TestGlobalGitConfigResetDefeatsLaterHelperForUppercaseURLPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helpers are POSIX shell scripts")
	}
	url := "https://example.com/git/u/App.git"
	root := t.TempDir()
	globalLog := filepath.Join(root, "global.log")
	scopedLog := filepath.Join(root, "scoped.log")
	globalHelper := filepath.Join(root, "global-helper")
	scopedHelper := filepath.Join(root, "scoped-helper")
	writeTestFileMode(t, globalHelper, []byte("#!/bin/sh\nprintf '%s\\n' \"$1\" >> \"$GLOBAL_HELPER_LOG\"\ncat >/dev/null\nif [ \"$1\" = get ]; then printf 'username=global-user\\npassword=global-pass\\n'; fi\n"), 0o700)
	writeTestFileMode(t, scopedHelper, []byte("#!/bin/sh\nprintf '%s\\n' \"$1\" >> \"$SCOPED_HELPER_LOG\"\ncat >/dev/null\nif [ \"$1\" = get ]; then printf 'username=scoped-user\\npassword=scoped-pass\\n'; fi\n"), 0o700)
	t.Setenv("GLOBAL_HELPER_LOG", globalLog)
	t.Setenv("SCOPED_HELPER_LOG", scopedLog)

	globalCmd := "!" + shellQuoteArg(globalHelper)
	scopedCmd := "!" + shellQuoteArg(scopedHelper)
	// lark-cli section first, generic helper AFTER it: only correct-case matching
	// + repositioning isolates the scoped helper.
	content := credentialConfigText(url, []string{"", scopedCmd}, []string{"true"}) +
		"[credential]\n\thelper = " + globalCmd + "\n"
	configureIsolatedGlobalGit(t, content)

	if err := (GlobalGitConfig{HelperCommand: scopedCmd}).SetHelper(context.Background(), url, "app_xxx"); err != nil {
		t.Fatalf("SetHelper() error = %v", err)
	}

	cmd := exec.Command("git", "credential", "fill")
	cmd.Stdin = strings.NewReader("protocol=https\nhost=example.com\npath=git/u/App.git\n\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git credential fill: %v: %s", err, out)
	}
	if creds := parseCredentialFields(string(out)); creds["username"] != "scoped-user" || creds["password"] != "scoped-pass" {
		t.Fatalf("fill returned %v, want scoped helper credential for uppercase-path URL", creds)
	}
	if raw, err := os.ReadFile(globalLog); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("global helper unexpectedly ran during fill: log=%q err=%v", raw, err)
	}
}

// TestGlobalGitConfigRefusesToRepositionSectionWithExtraKeys verifies that when
// the lark-cli section must be repositioned (a later generic helper exists) but
// the URL-scoped section also holds an untracked key (e.g. username), SetHelper
// refuses with a typed FailedPrecondition error instead of destroying that key
// via --remove-section, and leaves the extra key intact.
func TestGlobalGitConfigRefusesToRepositionSectionWithExtraKeys(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helpers are POSIX shell scripts")
	}
	url := "https://example.com/git/u/app.git"
	scopedCmd := "!scoped-cmd"
	globalCmd := "!global-cmd"
	// lark-cli reset+canonical+useHttpPath, PLUS an untracked username key, and a
	// later generic helper that forces repositioning.
	content := "[credential \"" + url + "\"]\n" +
		"\thelper = \n" +
		"\thelper = " + scopedCmd + "\n" +
		"\tuseHttpPath = true\n" +
		"\tusername = alice\n" +
		"[credential]\n\thelper = " + globalCmd + "\n"
	configureIsolatedGlobalGit(t, content)

	err := (GlobalGitConfig{HelperCommand: scopedCmd}).SetHelper(context.Background(), url, "app_xxx")
	assertProblemSubtype(t, err, errs.SubtypeFailedPrecondition)

	// The untracked username key must survive.
	got, err := exec.Command("git", "config", "--global", "--get", gitCredentialKey(url, "username")).Output()
	if err != nil {
		t.Fatalf("username key was discarded: %v", err)
	}
	if strings.TrimSpace(string(got)) != "alice" {
		t.Fatalf("username = %q, want preserved alice", got)
	}
}

// TestGlobalGitConfigRepositionsWhenUntrackedKeyLivesOnlyInIncludedConfig proves
// the untracked-key scan is scoped to the writable file (--no-includes): when
// the only "extra" URL-scoped key (username) lives in an INCLUDED config —
// which --remove-section never edits — repositioning must proceed and succeed,
// not fail closed. The included username must survive untouched.
func TestGlobalGitConfigRepositionsWhenUntrackedKeyLivesOnlyInIncludedConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helpers are POSIX shell scripts")
	}
	url := "https://example.com/git/u/app.git"
	scopedCmd := "!scoped-cmd"
	globalCmd := "!global-cmd"
	root := t.TempDir()
	includePath := filepath.Join(root, "included.config")
	// The included config holds the untracked username AND a later generic helper
	// that forces repositioning of the writable lark-cli section.
	writeTestFile(t, includePath, []byte(
		"[credential \""+url+"\"]\n\tusername = alice\n"+
			"[credential]\n\thelper = "+globalCmd+"\n"))
	// The writable file holds only the lark-cli-tracked keys, then the include.
	content := "[credential \"" + url + "\"]\n" +
		"\thelper = \n" +
		"\thelper = " + scopedCmd + "\n" +
		"\tuseHttpPath = true\n" +
		"[include]\n\tpath = " + includePath + "\n"
	globalPath := configureIsolatedGlobalGit(t, content)

	if err := (GlobalGitConfig{HelperCommand: scopedCmd}).SetHelper(context.Background(), url, "app_xxx"); err != nil {
		t.Fatalf("SetHelper() error = %v, want success (untracked key is include-only)", err)
	}

	// The included username must be untouched (it lives outside the writable file).
	afterInclude, err := os.ReadFile(includePath)
	if err != nil {
		t.Fatalf("read included config: %v", err)
	}
	if !strings.Contains(string(afterInclude), "username = alice") {
		t.Fatalf("included username was modified:\n%s", afterInclude)
	}

	// The writable file must not have grown a username key from the include.
	afterGlobal, err := os.ReadFile(globalPath)
	if err != nil {
		t.Fatalf("read global config: %v", err)
	}
	if strings.Contains(string(afterGlobal), "username") {
		t.Fatalf("writable file unexpectedly gained a username key:\n%s", afterGlobal)
	}
}

// TestGlobalConfigLockNameResolvesRelativeOrigin proves the global-config lock
// name is derived from an absolute path, so a relative GIT_CONFIG_GLOBAL maps to
// the same lock regardless of the caller's working directory. Without absolute
// resolution the name would embed the relative path verbatim and collide with a
// different config file that happens to share that relative name from another
// cwd.
func TestGlobalConfigLockNameResolvesRelativeOrigin(t *testing.T) {
	rel := globalConfigLockName("file:relative/global.config")
	abs, err := filepath.Abs("relative/global.config")
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	want := "apps_git_credential_globalcfg_" + safeLockNameChars.ReplaceAllString(abs, "_") + ".lock"
	if rel != want {
		t.Fatalf("lock name = %q, want absolute-resolved %q", rel, want)
	}
	// An already-absolute origin must resolve to the same name (idempotent).
	if got := globalConfigLockName("file:" + abs); got != want {
		t.Fatalf("absolute origin lock name = %q, want %q", got, want)
	}
}

// TestLockGlobalConfigWrapsAcquireFailureAsTypedError proves a lock-acquisition
// failure (not just the mkdir failure) is returned as a typed InternalError with
// SubtypeStorage, so SetHelper/UnsetHelper surface consistent metadata. It forces
// the failure by pre-creating the lock file path as a directory, which makes the
// underlying O_CREATE|O_RDWR open fail with a non-retryable error.
func TestLockGlobalConfigWrapsAcquireFailureAsTypedError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("relies on POSIX open-a-directory-for-write failing")
	}
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	origin := "file:" + filepath.Join(t.TempDir(), "global.config")
	lockPath := filepath.Join(configDir, "locks", filepath.Base(globalConfigLockName(origin)))
	if err := os.MkdirAll(lockPath, 0o700); err != nil {
		t.Fatalf("pre-create lock path as directory: %v", err)
	}

	unlock, err := lockGlobalConfig(origin)
	if err == nil {
		unlock()
		t.Fatal("lockGlobalConfig() succeeded, want a typed acquisition failure")
	}
	assertProblemSubtype(t, err, errs.SubtypeStorage)
}
