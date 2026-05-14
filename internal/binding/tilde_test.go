// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package binding

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// setFakeOSHome controls osHome's env-chain inputs (HOME and USERPROFILE)
// in one call so tests stay deterministic across platforms. osHome reads
// HOME first, then USERPROFILE, then user.Current(); setting only one of
// the two leaves the test sensitive to whichever the runner happens to
// have populated. Passing dir == "" disables both env entries so tests
// can exercise the user.Current() fallback or no-home edge cases.
func setFakeOSHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

// isolateRuntimeWrites parks the process cwd in a fresh TempDir for the
// test's duration. Tests that set HOME to a sentinel literal trigger Go
// runtime side effects — most visibly the telemetry subsystem, which
// calls os.UserConfigDir() (= "$HOME/Library/Application Support" on
// darwin) and happily writes through a relative result like
// "undefined/Library/...". Without isolation those files land in the
// package or repo dir and get accidentally staged. Chdir'ing into a
// TempDir routes the noise into a path testing.T auto-cleans.
func isolateRuntimeWrites(t *testing.T) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(orig)
	})
}

// TestOpenClawHome covers the openClawHome resolution table: empty /
// sentinel OPENCLAW_HOME falls back to the OS home, explicit absolute
// values are used verbatim (with whitespace trimmed), and tilde-prefixed
// values recurse through the OS home.
func TestOpenClawHome(t *testing.T) {
	homeDir := t.TempDir()
	explicit := t.TempDir()
	setFakeOSHome(t, homeDir)

	tests := []struct {
		name        string
		openclawEnv string
		want        string
	}{
		{"unset falls back to OS home", "", homeDir},
		{"undefined literal treated as unset", "undefined", homeDir},
		{"null literal treated as unset", "null", homeDir},
		{"whitespace-only treated as unset", "   ", homeDir},
		{"explicit absolute path used verbatim", explicit, explicit},
		{"explicit absolute path is trimmed", "  " + explicit + "  ", explicit},
		{"bare tilde resolves to OS home", "~", homeDir},
		{"tilde-prefixed value recurses through OS home", "~/custom", filepath.Join(homeDir, "custom")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OPENCLAW_HOME", tc.openclawEnv)
			got := openClawHome()
			if got != tc.want {
				t.Errorf("openClawHome() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestOpenClawHome_RelativeIsAbsolutized confirms a relative
// OPENCLAW_HOME is resolved against the process cwd, mirroring Node's
// path.resolve behaviour in OpenClaw.
func TestOpenClawHome_RelativeIsAbsolutized(t *testing.T) {
	t.Setenv("OPENCLAW_HOME", filepath.FromSlash("relative/dir"))
	got := openClawHome()

	if !filepath.IsAbs(got) {
		t.Fatalf("openClawHome() = %q, want absolute path", got)
	}
	wantSuffix := filepath.FromSlash("relative/dir")
	if !strings.HasSuffix(got, wantSuffix) {
		t.Errorf("openClawHome() = %q, want suffix %q", got, wantSuffix)
	}
}

// TestOpenClawHome_FallsBackToUserDatabase pins osHome's final fallback
// to the OS user database when HOME and USERPROFILE are both unset,
// matching Node's os.homedir() (which uses getpwuid). Cwd-independent
// and user-bound, so it does not conflict with the "no cwd fallback"
// rule documented on osHome.
func TestOpenClawHome_FallsBackToUserDatabase(t *testing.T) {
	u, err := user.Current()
	if err != nil || u.HomeDir == "" {
		t.Skip("os/user.Current() unavailable on this runner")
	}
	setFakeOSHome(t, "")
	t.Setenv("OPENCLAW_HOME", "")
	got := openClawHome()
	if got != u.HomeDir {
		t.Errorf("openClawHome() = %q, want %q (account home from user.Current)", got, u.HomeDir)
	}
}

// TestOpenClawHome_TildeOpenClawHomeUsesUserDatabaseFallback pins that
// a tilde-form OPENCLAW_HOME ("~/custom") expands against the
// user-database fallback when HOME and USERPROFILE are both unset.
// Without the user.Current() step in osHome this would have failed
// (returning "") and dropped the bind back to the audit's
// "path must be absolute" error.
func TestOpenClawHome_TildeOpenClawHomeUsesUserDatabaseFallback(t *testing.T) {
	u, err := user.Current()
	if err != nil || u.HomeDir == "" {
		t.Skip("os/user.Current() unavailable on this runner")
	}
	setFakeOSHome(t, "")
	t.Setenv("OPENCLAW_HOME", "~/custom")
	got := openClawHome()
	want := filepath.Join(u.HomeDir, "custom")
	if got != want {
		t.Errorf("openClawHome() = %q, want %q", got, want)
	}
}

// TestExpandTildePath covers the full input grid for expandTildePath:
// bare tilde, tilde-slash, tilde + suffix, nested suffix, plain absolute
// and relative literals, and the intentionally-unchanged forms (~user,
// ~foo) that OpenClaw does not expand either.
func TestExpandTildePath(t *testing.T) {
	fakeHome := t.TempDir()
	absFixture := filepath.Join(fakeHome, "abs.json")
	setFakeOSHome(t, fakeHome)
	t.Setenv("OPENCLAW_HOME", "")

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"bare tilde", "~", fakeHome},
		{"tilde slash", "~/", fakeHome},
		{"tilde with file", "~/secret.json", filepath.Join(fakeHome, "secret.json")},
		{"tilde with nested path", "~/.openclaw/secret.json", filepath.Join(fakeHome, ".openclaw/secret.json")},
		{"absolute unchanged", absFixture, absFixture},
		{"relative unchanged", "foo/bar", "foo/bar"},
		{"dot relative unchanged", "../foo", "../foo"},
		{"tilde user form unchanged", "~root/foo", "~root/foo"},
		{"tilde without separator unchanged", "~foo", "~foo"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := expandTildePath(tc.in)
			if got != tc.want {
				t.Errorf("expandTildePath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestExpandTildePath_RespectsOpenClawHome verifies that with
// OPENCLAW_HOME set, tilde expansion uses that custom home rather than
// the OS home — the integration-level invariant that closes the
// internal inconsistency CodeX's first review flagged.
func TestExpandTildePath_RespectsOpenClawHome(t *testing.T) {
	homeDir := t.TempDir()
	clawHome := t.TempDir()
	setFakeOSHome(t, homeDir)
	t.Setenv("OPENCLAW_HOME", clawHome)

	got := expandTildePath("~/secret.json")
	want := filepath.Join(clawHome, "secret.json")
	if got != want {
		t.Errorf("expandTildePath(%q) = %q, want %q (should use OPENCLAW_HOME)", "~/secret.json", got, want)
	}
	if got == filepath.Join(homeDir, "secret.json") {
		t.Errorf("expandTildePath unexpectedly used OS home %q instead of OPENCLAW_HOME %q", homeDir, clawHome)
	}
}

// TestExpandTildePath_FallsBackToUserDatabase is the end-to-end
// equivalent of TestOpenClawHome_FallsBackToUserDatabase: with HOME and
// USERPROFILE both unset, expandTildePath still resolves `~/foo` via
// osHome's user.Current() step. Matches Node os.homedir() and keeps
// OpenClaw-authored configs working in minimal-env shells.
func TestExpandTildePath_FallsBackToUserDatabase(t *testing.T) {
	u, err := user.Current()
	if err != nil || u.HomeDir == "" {
		t.Skip("os/user.Current() unavailable on this runner")
	}
	setFakeOSHome(t, "")
	t.Setenv("OPENCLAW_HOME", "")
	got := expandTildePath("~/foo")
	want := filepath.Join(u.HomeDir, "foo")
	if got != want {
		t.Errorf("expandTildePath(~/foo) = %q, want %q", got, want)
	}
}

// TestOpenClawHome_OSHomeNormalization pins OpenClaw's sentinel
// normalisation on the env chain: the literals "undefined" / "null" /
// blank-or-whitespace are all treated as unset, so a JS-flavoured
// accidentally-stringified env value (e.g. `HOME=undefined` from a
// shell wrapper) doesn't end up as a literal directory component when
// the user authored `~/secret`. Combined with the user.Current()
// fallback further down (see TestOpenClawHome_FallsBackToUserDatabase),
// the contract is: a malformed HOME falls through to USERPROFILE first,
// and only if that's also unset/sentinel do we go to the user database.
func TestOpenClawHome_OSHomeNormalization(t *testing.T) {
	isolateRuntimeWrites(t)
	userProfileDir := t.TempDir()
	homeWinsDir := t.TempDir()

	tests := []struct {
		name        string
		home        string
		userProfile string
		want        string
	}{
		{"HOME=undefined falls through to USERPROFILE", "undefined", userProfileDir, userProfileDir},
		{"HOME=null falls through to USERPROFILE", "null", userProfileDir, userProfileDir},
		{"HOME=whitespace falls through to USERPROFILE", "   ", userProfileDir, userProfileDir},
		{"HOME wins over USERPROFILE when both are valid", homeWinsDir, userProfileDir, homeWinsDir},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", tc.home)
			t.Setenv("USERPROFILE", tc.userProfile)
			t.Setenv("OPENCLAW_HOME", "")
			if got := openClawHome(); got != tc.want {
				t.Errorf("openClawHome() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestOpenClawHome_SentinelHOMEFallsToUserDatabaseNotCwd pins the
// deliberate hybrid documented on osHome: with HOME a sentinel literal
// and USERPROFILE unset, OpenClaw would fall back to process.cwd();
// this implementation falls to the OS user database instead. The
// account home is both safer (cwd-independent) and more useful (it is
// where the user originally authored `~/...` against), so we prefer it
// over either OpenClaw's cwd fallback or a strict reject.
func TestOpenClawHome_SentinelHOMEFallsToUserDatabaseNotCwd(t *testing.T) {
	isolateRuntimeWrites(t)
	u, err := user.Current()
	if err != nil || u.HomeDir == "" {
		t.Skip("os/user.Current() unavailable on this runner")
	}
	t.Setenv("HOME", "undefined")
	t.Setenv("USERPROFILE", "")
	t.Setenv("OPENCLAW_HOME", "")
	got := openClawHome()
	if got != u.HomeDir {
		t.Errorf("openClawHome() = %q, want %q (account home, not cwd)", got, u.HomeDir)
	}
}

// TestExpandTildePath_BackslashPreservedOnPOSIX pins that `~\secret.json`
// expands by replacing only the `~` byte, leaving the backslash literally
// as part of the filename — matching OpenClaw's regex-replace semantics
// (`/^~(?=$|[\\/])/`) rather than going through filepath.Join (which would
// drop the backslash on POSIX). On Windows backslash is a real separator,
// so the literal-byte invariant doesn't apply.
func TestExpandTildePath_BackslashPreservedOnPOSIX(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("backslash is a path separator on Windows; invariant only applies on POSIX")
	}
	fakeHome := t.TempDir()
	setFakeOSHome(t, fakeHome)
	t.Setenv("OPENCLAW_HOME", "")

	got := expandTildePath(`~\secret.json`)
	want := fakeHome + `\secret.json`
	if got != want {
		t.Errorf("expandTildePath(%q) = %q, want %q (backslash should be preserved as filename byte)", `~\secret.json`, got, want)
	}
}
