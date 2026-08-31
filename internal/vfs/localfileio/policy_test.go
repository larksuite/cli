// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package localfileio

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// TestPolicy_HomeEnvCannotMoveAllowRoot pins the built-in allowlist against
// $HOME: an invocation that controls the environment must not be able to point
// ~/files at a directory of its choosing.
// The assertion is made against the resolved roots rather than by validating a
// path under the fake home: on hosts whose temp directory lives in /tmp, that
// directory is an allow root already and the check would pass for the wrong
// reason.
func TestPolicy_HomeEnvCannotMoveAllowRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME is not the home-directory source on Windows")
	}
	fake := t.TempDir()
	fake, _ = filepath.EvalSymlinks(fake)
	t.Setenv("HOME", fake)

	for _, e := range allowRoots(t.TempDir()) {
		if strings.HasPrefix(e.resolved, fake) || strings.HasPrefix(e.literal, fake) {
			t.Fatalf("HOME moved allow root %q into the environment-supplied home %q", e.label, fake)
		}
	}
}

// TestPolicy_HomeEnvCannotHideDenyRoot pins the denylist against $HOME: the
// real credential directories must stay protected even when the environment
// claims the home directory is elsewhere.
func TestPolicy_HomeEnvCannotHideDenyRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME is not the home-directory source on Windows")
	}
	realHome, err := trustedHome()
	if err != nil {
		t.Skipf("no trusted home: %v", err)
	}
	t.Setenv("HOME", t.TempDir())

	_, err = SafeInputPath(filepath.Join(realHome, ".ssh", "id_rsa"))
	if err == nil {
		t.Fatal("HOME hid the ~/.ssh deny root; want rejection")
	}
	if !strings.Contains(err.Error(), "denylist") {
		t.Errorf("error should cite the denylist, got: %v", err)
	}
}

// TestPolicy_CaseVariantCannotBypassDenyRoot covers case-insensitive default
// filesystems (APFS, NTFS), where ~/.SSH and ~/.ssh are the same directory.
func TestPolicy_CaseVariantCannotBypassDenyRoot(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("default filesystem is case-sensitive on this platform")
	}
	home, err := trustedHome()
	if err != nil {
		t.Skipf("no trusted home: %v", err)
	}
	for _, variant := range []string{".SSH", ".GnUpG", ".AWS"} {
		t.Run(variant, func(t *testing.T) {
			if _, err := SafeInputPath(filepath.Join(home, variant, "credential")); err == nil {
				t.Fatalf("case variant %q bypassed its deny root; want rejection", variant)
			}
		})
	}
}

// TestPolicy_PreservesSurroundingSpaceInFilenames guards against trimming a
// caller's filename: a trailing space is legal, and silently dropping it would
// address a different file than the one named.
func TestPolicy_PreservesSurroundingSpaceInFilenames(t *testing.T) {
	dir := t.TempDir()
	dir, _ = filepath.EvalSymlinks(dir)
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	got, err := SafeOutputPath("report .txt")
	if err != nil {
		t.Fatalf("SafeOutputPath error = %v", err)
	}
	if want := filepath.Join(dir, "report .txt"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestPolicy_UnicodeFoldingCannotBypassDenyRoot covers filesystems that fold
// case beyond simple lowercasing: APFS maps U+017F (ſ) to "s", so ".ſſh" opens
// "~/.ssh" while the two strings compare as different. Containment must be
// decided by file identity, not by name.
func TestPolicy_UnicodeFoldingCannotBypassDenyRoot(t *testing.T) {
	home, err := trustedHome()
	if err != nil {
		t.Skipf("no trusted home: %v", err)
	}
	probe := filepath.Join(home, ".ssh")
	if _, err := os.Stat(probe); err != nil {
		t.Skipf("~/.ssh not present on this host: %v", err)
	}
	// Only meaningful where the filesystem actually folds the variant onto the
	// real directory; elsewhere the variant is a genuinely different path.
	variant := filepath.Join(home, ".\u017f\u017fh")
	if _, err := os.Stat(variant); err != nil {
		t.Skipf("filesystem does not case-fold U+017F: %v", err)
	}

	for _, tier := range []struct {
		name string
		call func(string) (string, error)
	}{
		{"strict input", SafeInputPath},
		{"strict output", SafeOutputPath},
		{"relaxed input", LocalInputPath},
	} {
		t.Run(tier.name, func(t *testing.T) {
			if _, err := tier.call(filepath.Join(variant, "id_rsa")); err == nil {
				t.Fatal("case-folded spelling reached ~/.ssh; want denylist rejection")
			}
		})
	}
}

// TestPolicy_ForeignAbsolutePathIsNotTreatedAsRelative pins a shape that would
// otherwise be re-read as a filename and joined to the working directory,
// landing inside an allow root.
func TestPolicy_ForeignAbsolutePathIsNotTreatedAsRelative(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("these shapes are native on Windows")
	}
	dir := t.TempDir()
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	for _, p := range []string{`C:\Users\agent\secret.txt`, "C:/Users/agent/secret.txt", `\Users\agent\secret.txt`} {
		if _, err := SafeInputPath(p); err == nil {
			t.Errorf("SafeInputPath(%q) = nil error; want rejection", p)
		}
	}
}

// TestPolicy_DenyRootWinsOverHomeFilesForRoot pins the accepted trade-off: when
// the home directory is itself a deny root (running as root, where it is
// /root), ~/files loses to the denylist and only cwd and /tmp remain. Written
// as a test so the behaviour cannot be "fixed" back into a carve-out without a
// deliberate decision.
func TestPolicy_DenyRootWinsOverHomeFilesForRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("/root is not a Windows path")
	}
	home, err := trustedHome()
	if err != nil {
		t.Skipf("no trusted home: %v", err)
	}
	if home != "/root" {
		t.Skip("only meaningful when the account's home is the /root deny root")
	}

	if _, err := SafeOutputPath("/root/files/out.txt"); err == nil {
		t.Fatal("~/files was allowed inside the /root deny root; deny must win")
	}
}

// TestPolicy_ConcurrentValidationDoesNotShareDenyRootStorage pins the fix for
// a data race: the per-call config-dir deny root used to be appended onto the
// process-cached slice, writing into its spare capacity. Download fan-out
// validates paths from several goroutines at once, so run it under -race.
func TestPolicy_ConcurrentValidationDoesNotShareDenyRootStorage(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", "relcfg")

	var wg sync.WaitGroup
	for i := range 32 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _ = SafeInputPath(fmt.Sprintf("file-%d.txt", i))
			_, _ = SafeOutputPath(filepath.Join("relcfg", "config.json"))
		}(i)
	}
	wg.Wait()

	// The config directory must still be denied after the concurrent run.
	if _, err := SafeInputPath(filepath.Join("relcfg", "config.json")); err == nil {
		t.Fatal("relative LARKSUITE_CLI_CONFIG_DIR was not denied")
	}
}

// TestPolicy_ForeignAbsoluteShapeCannotEscapeViaCwdSymlink covers a path that
// is absolute only under Windows rules: on Unix the OS reads it as a relative
// name, so a cwd-local symlink spelled like the prefix would carry it out of
// the allowlist unless the denylist is applied to that real interpretation.
func TestPolicy_ForeignAbsoluteShapeCannotEscapeViaCwdSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shape is native on Windows")
	}
	dir := t.TempDir()
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)
	// Each probe is a cwd-local symlink named exactly like the prefix the OS
	// will read as the first path component.
	for _, name := range []string{"C:", " "} {
		if err := os.Symlink("/", filepath.Join(dir, name)); err != nil {
			t.Skipf("cannot create the probe symlink %q: %v", name, err)
		}
	}

	for _, p := range []string{"C:/etc/passwd", " /etc/passwd"} {
		t.Run(p, func(t *testing.T) {
			if _, err := SafeInputPath(p); err == nil {
				t.Errorf("SafeInputPath(%q) = nil error; want rejection", p)
			}
			// The relaxed tier keeps the spelling, but the denylist must still
			// see through it to the location the OS would open.
			if _, err := LocalInputPath(p); err == nil {
				t.Errorf("LocalInputPath(%q) = nil error; want denylist rejection", p)
			}
		})
	}
}
