// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package localfileio

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/larksuite/cli/internal/vfs"
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

// TestPolicy_LiteralTildeEntryCannotEscape covers the gap between the two
// readings of a "~/..." argument: validation expands it to the home
// directory, while a caller that keeps the original string opens whatever "~"
// names in the working directory. Both readings must be checked.
func TestPolicy_LiteralTildeEntryCannotEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("~ is not a home-directory shorthand on Windows")
	}
	dir := t.TempDir()
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)
	if err := os.Symlink("/etc", filepath.Join(dir, "~")); err != nil {
		t.Skipf("cannot create the probe symlink: %v", err)
	}

	if _, err := SafeInputPath("~/passwd"); err == nil {
		t.Error(`SafeInputPath("~/passwd") = nil error; want denylist rejection`)
	}
	if _, err := LocalInputPath("~/passwd"); err == nil {
		t.Error(`LocalInputPath("~/passwd") = nil error; want denylist rejection`)
	}
}

// TestPolicy_TildeStillReachesHomeFiles guards the other direction: closing
// the literal-~ gap must not cost the ~/files allow root for callers that pass
// the shorthand through argv, where no shell expands it.
func TestPolicy_TildeStillReachesHomeFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("~ is not a home-directory shorthand on Windows")
	}
	home, err := trustedHome()
	if err != nil {
		t.Skipf("no trusted home: %v", err)
	}
	if home == "/root" {
		t.Skip("~/files is a deny root when the home directory is /root")
	}
	dir := t.TempDir()
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	got, err := SafeOutputPath("~/files/tilde-probe.txt")
	if err != nil {
		t.Fatalf(`SafeOutputPath("~/files/tilde-probe.txt") error = %v, want nil`, err)
	}
	if want := filepath.Join(home, "files", "tilde-probe.txt"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestPolicy_ConfigDirFallbackIsDeniedWithoutHome covers the home-less
// container case: core.GetBaseConfigDir then keeps credentials in a bare
// ".lark-cli" under the working directory, which is otherwise an allow root.
func TestPolicy_ConfigDirFallbackIsDeniedWithoutHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the home lookup does not read the environment on Windows")
	}
	dir := t.TempDir()
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", "")
	t.Setenv("HOME", "")

	if _, err := SafeInputPath(filepath.Join(".lark-cli", "config.json")); err == nil {
		t.Fatal("the fallback CLI config directory was not denied")
	}
}

// TestPolicy_OutputTargetWithHardLinkIsRejected covers a write that stays
// inside the allowlist by name while sharing an inode with a file outside it:
// truncating the approved name in place would rewrite that outside file, and a
// hard link has no target for name resolution to follow.
func TestPolicy_OutputTargetWithHardLinkIsRejected(t *testing.T) {
	outside := t.TempDir()
	original := filepath.Join(outside, "secret.json")
	if err := os.WriteFile(original, []byte("ORIGINAL"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	work := t.TempDir()
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(work)
	if err := os.Link(original, filepath.Join(work, "target.png")); err != nil {
		t.Skipf("cannot create the hard-link probe: %v", err)
	}

	_, err := SafeOutputPath("target.png")
	if err == nil {
		t.Fatal(`SafeOutputPath("target.png") = nil error; want rejection for a multiply-linked target`)
	}
	if !strings.Contains(err.Error(), "hard links") {
		t.Errorf("error should cite the hard links, got: %v", err)
	}

	// A target with a single name stays writable, and reads are unaffected.
	if _, err := SafeOutputPath("fresh.png"); err != nil {
		t.Errorf(`SafeOutputPath("fresh.png") error = %v, want nil`, err)
	}
}

// denylistedAbsolutePath returns an absolute path inside a built-in deny root
// for the platform running the test. A Unix path literal cannot serve both:
// "/etc/passwd" is not absolute on Windows, so it would be read as a name
// relative to the working directory and land nowhere near a deny root. The
// credential directories under the account home are deny roots on every
// platform, which makes them the portable choice.
func denylistedAbsolutePath(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "windows" {
		return "/etc/passwd"
	}
	home, err := trustedHome()
	if err != nil {
		t.Skipf("no trusted home to derive a deny root from: %v", err)
	}
	return filepath.Join(home, ".ssh", "id_rsa")
}

// TestPolicy_RelativePathCannotLeaveCwdInsideAllowRoot covers the case an
// allow root wide enough to hold the working directory creates: with the
// process under /tmp, "../" reaches a sibling directory that the allowlist
// still reads as inside /tmp. /tmp is world-writable, so that sibling can
// belong to another user or another session.
func TestPolicy_RelativePathCannotLeaveCwdInsideAllowRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the /tmp allow root is Unix-only")
	}
	base, err := os.MkdirTemp("/tmp", "policy-cwd")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(base) })
	work := filepath.Join(base, "work")
	victim := filepath.Join(base, "victim")
	for _, d := range []string{work, victim} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(victim, "other-session.txt"), []byte("SECRET"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	if err := os.Chdir(work); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	for _, raw := range []string{"../victim/other-session.txt", ".."} {
		if _, err := SafeInputPath(raw); err == nil {
			t.Errorf("SafeInputPath(%q) = nil error; a relative path must not climb out of the working directory", raw)
		}
	}
	if _, err := SafeOutputPath("../victim/planted.txt"); err == nil {
		t.Error(`SafeOutputPath("../victim/planted.txt") = nil error; want the write refused`)
	}

	// The feature this branch exists for is untouched: a full path under an
	// allow root still works, and so does a relative path that stays put.
	if _, err := SafeOutputPath(filepath.Join(victim, "planted.txt")); err != nil {
		t.Errorf("an absolute path inside /tmp should stay writable, got: %v", err)
	}
	if _, err := SafeOutputPath("./inside.txt"); err != nil {
		t.Errorf("a relative path inside the working directory should stay writable, got: %v", err)
	}
}

// TestPolicy_HomeCredentialFilesAreDenied covers the other half of the same
// exposure: the working directory is an allow root and running from the home
// directory is ordinary, so every credential store there is reachable by a
// relative name unless the denylist names it.
func TestPolicy_HomeCredentialFilesAreDenied(t *testing.T) {
	home, err := trustedHome()
	if err != nil {
		t.Skipf("no trusted home: %v", err)
	}
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	if err := os.Chdir(home); err != nil {
		t.Skipf("cannot chdir to home: %v", err)
	}

	for _, rel := range []string{
		".netrc", ".git-credentials", ".gitconfig",
		".kube/config", ".docker/config.json", ".npmrc",
		".config/gh/hosts.yml", ".zsh_history", ".bash_history",
		".ssh/id_rsa", ".aws/credentials",
	} {
		if _, err := SafeInputPath(rel); err == nil {
			t.Errorf("SafeInputPath(%q) from the home directory = nil error; want it denied", rel)
		}
	}

	// An ordinary file in the home directory is still readable.
	if _, err := SafeInputPath("some-ordinary-file.txt"); err != nil &&
		!strings.Contains(err.Error(), "cannot inspect path") {
		t.Errorf("an ordinary name in the home directory should not be denied, got: %v", err)
	}
}

// probeFS counts the paths a validation reads, so a test can assert that the
// credential directories were never touched. It wraps the real filesystem:
// what is under test is which paths get asked about, not how they answer.
//
// It sees this package's own calls, which is where the roots were resolved. It
// does not see inside the standard library, so it cannot rule out the per-entry
// lstat os.ReadDir falls back to on a filesystem that reports no dirent type —
// that condition is documented at listing() instead.
type probeFS struct {
	vfs.FS
	mu       sync.Mutex
	paths    []string
	readDirs []string
}

func (p *probeFS) record(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.paths = append(p.paths, name)
}

func (p *probeFS) Stat(name string) (fs.FileInfo, error)  { p.record(name); return p.FS.Stat(name) }
func (p *probeFS) Lstat(name string) (fs.FileInfo, error) { p.record(name); return p.FS.Lstat(name) }
func (p *probeFS) Open(name string) (*os.File, error)     { p.record(name); return p.FS.Open(name) }

func (p *probeFS) ReadDir(name string) ([]os.DirEntry, error) {
	p.mu.Lock()
	p.readDirs = append(p.readDirs, name)
	p.mu.Unlock()
	return p.FS.ReadDir(name)
}

// readDirCount reports how many times a directory was enumerated.
func (p *probeFS) readDirCount(dir string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, seen := range p.readDirs {
		if seen == dir {
			n++
		}
	}
	return n
}

// touched reports the recorded paths that name the given entry.
func (p *probeFS) touched(name string) []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var hits []string
	for _, path := range p.paths {
		if strings.Contains(strings.ToLower(path), name) {
			hits = append(hits, path)
		}
	}
	return hits
}

// installProbeFS routes the package's filesystem access through a recorder for
// the duration of the test.
func installProbeFS(t *testing.T) *probeFS {
	t.Helper()
	probe := &probeFS{FS: vfs.DefaultFS}
	vfs.DefaultFS = probe
	t.Cleanup(func() { vfs.DefaultFS = probe.FS })
	return probe
}

// fakeHome returns a home directory holding real (unlinked) credential
// directories, resolved so the test compares the same spelling the policy does.
func fakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	home, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	for _, name := range []string{".ssh", ".aws", ".config"} {
		if err := os.MkdirAll(filepath.Join(home, name), 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}
	return home
}

// TestPolicy_TargetOutsideHomeLeavesCredentialDirsUntouched is the regression
// test for #2726: validating a path that has nothing to do with the home
// directory must not stat ~/.ssh and its neighbours. Security tooling that
// guards those paths raises an alarm on every such access, and a download into
// the working directory has no reason to cause one.
func TestPolicy_TargetOutsideHomeLeavesCredentialDirsUntouched(t *testing.T) {
	home := fakeHome(t)
	outside := t.TempDir()
	outside, _ = filepath.EvalSymlinks(outside)
	target := filepath.Join(outside, "download.png")

	group := newHomeDenyGroup(home)
	probe := installProbeFS(t)
	roots := group.rootsToCheck(target, target, ancestors(target))

	if len(roots) != 0 {
		t.Errorf("resolved %d deny roots for a target outside the home directory; want none", len(roots))
	}
	if hits := probe.touched(".ssh"); len(hits) != 0 {
		t.Errorf("validation reached the SSH directory: %v", hits)
	}
	if hits := probe.touched(".aws"); len(hits) != 0 {
		t.Errorf("validation reached the AWS directory: %v", hits)
	}
}

// TestPolicy_TargetInsideHomeProbesOnlyItsOwnComponent covers the case Windows
// makes the common one: there the working directory almost always sits under
// C:\Users\<account>, so "inside the home directory" cannot by itself be the
// reason to stat every credential directory. Only the component the target is
// actually in may be resolved.
func TestPolicy_TargetInsideHomeProbesOnlyItsOwnComponent(t *testing.T) {
	home := fakeHome(t)
	target := filepath.Join(home, "projects", "report.txt")

	group := newHomeDenyGroup(home)
	probe := installProbeFS(t)
	roots := group.rootsToCheck(target, target, ancestors(target))

	if len(roots) != 0 {
		t.Errorf("resolved %d deny roots for a target in an ordinary home subdirectory; want none", len(roots))
	}
	if hits := probe.touched(".ssh"); len(hits) != 0 {
		t.Errorf("validation reached the SSH directory: %v", hits)
	}
}

// TestPolicy_TargetInsideDenyRootResolvesThatRoot pins the other half: a target
// that really is inside a credential directory selects that root — and only
// that one — so identity comparison still has the FileInfo it needs to see
// through alternate spellings of the name.
func TestPolicy_TargetInsideDenyRootResolvesThatRoot(t *testing.T) {
	home := fakeHome(t)
	target := filepath.Join(home, ".ssh", "id_rsa")

	group := newHomeDenyGroup(home)
	roots := group.rootsToCheck(target, target, ancestors(target))

	if len(roots) != 1 || roots[0].label != "~/.ssh" {
		t.Fatalf("selected %d roots %v for a target inside ~/.ssh; want exactly ~/.ssh", len(roots), labelsOf(roots))
	}
	if label, ok := identityLabel(ancestors(target), roots); !ok || label != "~/.ssh" {
		t.Errorf("identity comparison lost the ~/.ssh root: label=%q ok=%v", label, ok)
	}
}

func labelsOf(roots []policyEntry) []string {
	labels := make([]string, 0, len(roots))
	for _, e := range roots {
		labels = append(labels, e.label)
	}
	return labels
}

// TestPolicy_LinkedCredentialRootIsStillResolved covers the case the skipping
// rule has to keep: a deny root that is a symlink out of the home directory can
// match a target that is outside it too, so it must still be resolved. The
// directory listing is what reveals the link — without opening it.
func TestPolicy_LinkedCredentialRootIsStillResolved(t *testing.T) {
	home := t.TempDir()
	home, _ = filepath.EvalSymlinks(home)
	elsewhere := t.TempDir()
	elsewhere, _ = filepath.EvalSymlinks(elsewhere)
	if err := os.Symlink(elsewhere, filepath.Join(home, ".ssh")); err != nil {
		t.Skipf("cannot create the probe symlink: %v", err)
	}
	target := filepath.Join(elsewhere, "id_rsa")

	group := newHomeDenyGroup(home)
	roots := group.rootsToCheck(target, target, ancestors(target))

	if label, ok := matchRoots(target, target, roots); !ok || label != "~/.ssh" {
		t.Errorf("a credential directory linked out of the home directory stopped matching: label=%q ok=%v", label, ok)
	}
}

// TestPolicy_HardLinkedCredentialFileIsDenied covers the alias a directory
// listing cannot show and symlink resolution cannot follow: a second name for
// a credential file. "~/report.txt" and "~/.npmrc" are then one file, and only
// identity comparison can say so — which means the roots that could be that
// file have to be resolved, however innocent the name the caller used.
func TestPolicy_HardLinkedCredentialFileIsDenied(t *testing.T) {
	home := fakeHome(t)
	npmrc := filepath.Join(home, ".npmrc")
	if err := os.WriteFile(npmrc, []byte("//registry.npmjs.org/:_authToken=probe"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	alias := filepath.Join(home, "report.txt")
	if err := os.Link(npmrc, alias); err != nil {
		t.Skipf("cannot create the probe hard link: %v", err)
	}

	group := newHomeDenyGroup(home)
	roots := group.rootsToCheck(alias, alias, ancestors(alias))

	label, ok := identityLabel(ancestors(alias), roots)
	if !ok || label != "~/.npmrc" {
		t.Errorf("a hard link to ~/.npmrc was not denied: label=%q ok=%v roots=%v", label, ok, labelsOf(roots))
	}
}

// TestPolicy_SinglyLinkedFileSkipsCredentialProbe pins the other side of the
// link-count rule: an ordinary file carries one name, cannot be a credential
// file under an alias, and so must not cause any of them to be resolved.
func TestPolicy_SinglyLinkedFileSkipsCredentialProbe(t *testing.T) {
	home := fakeHome(t)
	report := filepath.Join(home, "report.txt")
	if err := os.WriteFile(report, []byte("ordinary"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	group := newHomeDenyGroup(home)
	probe := installProbeFS(t)
	roots := group.rootsToCheck(report, report, ancestors(report))

	if len(roots) != 0 {
		t.Errorf("resolved %v for a singly-linked ordinary file; want none", labelsOf(roots))
	}
	if hits := probe.touched(".npmrc"); len(hits) != 0 {
		t.Errorf("validation reached the npm credential file: %v", hits)
	}
}

// TestPolicy_UnlistableHomeIsEnumeratedOnce pins the negative half of the
// listing cache. A home directory that allows access by name while refusing
// enumeration answers the classification question with an error, and that
// answer has to be kept: re-reading the directory for every name on every
// validation costs a syscall per name, and under access control it logs
// another denial each time.
func TestPolicy_UnlistableHomeIsEnumeratedOnce(t *testing.T) {
	home := fakeHome(t)
	npmrc := filepath.Join(home, ".npmrc")
	if err := os.WriteFile(npmrc, []byte("token"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	alias := filepath.Join(home, "report.txt")
	if err := os.Link(npmrc, alias); err != nil {
		t.Skipf("cannot create the probe hard link: %v", err)
	}
	if err := os.Chmod(home, 0o300); err != nil {
		t.Skipf("cannot drop read permission on the fixture home: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(home, 0o700) })
	if _, err := os.ReadDir(home); err == nil {
		t.Skip("the fixture home is still listable; the test needs an unreadable one")
	}

	group := newHomeDenyGroup(home)
	probe := installProbeFS(t)
	for range 2 {
		group.rootsToCheck(alias, alias, ancestors(alias))
	}

	if n := probe.readDirCount(home); n != 1 {
		t.Errorf("enumerated the home directory %d times across two validations; want 1", n)
	}
	// Fail-closed is not traded away for the caching: an unreadable directory
	// still resolves its roots, so the hard link stays denied.
	roots := group.rootsToCheck(alias, alias, ancestors(alias))
	if label, ok := identityLabel(ancestors(alias), roots); !ok || label != "~/.npmrc" {
		t.Errorf("hard link stopped being denied when the home could not be listed: label=%q ok=%v", label, ok)
	}
}
