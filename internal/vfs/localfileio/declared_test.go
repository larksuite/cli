// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package localfileio

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// unusedDeclaredRoot returns an absolute directory that no built-in allow root
// contains and that does not exist on disk, so a test can prove a path is
// rejected before a declaration and accepted after it without touching the
// filesystem. /opt (the current volume's root on Windows) is outside cwd,
// /tmp, ~/files, and the agent state directories on every platform; the
// t.TempDir() alternative lives under /tmp on Linux and would pass for the
// wrong reason.
func unusedDeclaredRoot(t *testing.T) string {
	t.Helper()
	name := fmt.Sprintf("lark-cli-declared-test-%d-%s", os.Getpid(), strings.ReplaceAll(t.Name(), "/", "_"))
	if runtime.GOOS == "windows" {
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatalf("getwd: %v", err)
		}
		return filepath.Join(filepath.VolumeName(cwd)+`\`, name)
	}
	return filepath.Join("/opt", name)
}

// resetDeclared clears the process-wide declaration after the test so the
// widening cannot leak into later tests of the same binary.
func resetDeclared(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { declaredAllowRoots.Store(nil) })
}

func declare(t *testing.T, owner string, roots ...string) {
	t.Helper()
	if err := ApplyDeclaredAllowRoots([]PluginAllowRoots{{PluginName: owner, Roots: roots}}); err != nil {
		t.Fatalf("ApplyDeclaredAllowRoots(%q, %v): %v", owner, roots, err)
	}
}

func hasLiteralRoot(roots []policyEntry, literal string) bool {
	for _, e := range roots {
		if e.literal == filepath.Clean(literal) {
			return true
		}
	}
	return false
}

// A declared root widens both tiers of the strict validation, and validation
// still never touches the disk: the root need not exist, matching ~/files.
func TestApplyDeclaredAllowRoots_WidensStrictTier(t *testing.T) {
	resetDeclared(t)
	root := unusedDeclaredRoot(t)
	target := filepath.Join(root, "exports", "report.xlsx")

	// GIVEN: without a declaration the target is outside every allow root
	if _, err := SafeOutputPath(target); err == nil {
		t.Fatalf("%q was accepted before any declaration; the fixture is not outside the built-in roots", target)
	}

	// WHEN: a plugin declares the root
	declare(t, "acme", root)

	// THEN: both tiers accept paths under it, resolved through the declared root
	for name, validate := range map[string]func(string) (string, error){
		"--output": SafeOutputPath,
		"--file":   SafeInputPath,
	} {
		got, err := validate(target)
		if err != nil {
			t.Fatalf("%s %q after declaration: %v", name, target, err)
		}
		if !strings.HasSuffix(got, filepath.Join("exports", "report.xlsx")) {
			t.Errorf("%s resolved to %q, want a path ending in exports/report.xlsx", name, got)
		}
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Errorf("validation must not create the declared root; stat err = %v", err)
	}
}

// A relative argument has to stay inside the working directory even when a
// declared root is wide enough to contain cwd: naming a full path is a
// deliberate act the allowlist judges, climbing out with ".." is not.
func TestApplyDeclaredAllowRoots_RelativeArgumentStaysInCwd(t *testing.T) {
	resetDeclared(t)
	dir := t.TempDir()
	dir, _ = filepath.EvalSymlinks(dir)
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	parent := filepath.Dir(dir)
	declare(t, "acme", parent)

	if _, err := SafeOutputPath(filepath.Join("..", "sibling.txt")); err == nil {
		t.Fatal("a relative path climbed out of cwd into the declared root; checkRelativeStaysInCwd must still apply")
	}
	if _, err := SafeOutputPath(filepath.Join(parent, "sibling.txt")); err != nil {
		t.Fatalf("the same location named absolutely must pass through the declared root: %v", err)
	}
}

// Deny wins over declared roots exactly as it wins over cwd: even the widest
// possible declaration cannot reach a credential directory.
func TestApplyDeclaredAllowRoots_DenyStillWins(t *testing.T) {
	resetDeclared(t)
	home, err := trustedHome()
	if err != nil {
		t.Skipf("no trusted home: %v", err)
	}
	root := "/"
	if runtime.GOOS == "windows" {
		root = filepath.VolumeName(home) + `\`
	}
	declare(t, "acme", root)

	for _, protected := range []string{
		filepath.Join(home, ".ssh", "id_rsa"),
		filepath.Join(home, ".lark-cli", "config.json"),
	} {
		_, err := SafeInputPath(protected)
		if err == nil {
			t.Fatalf("declared root %q reached %q; deny must win over declared roots", root, protected)
		}
		if !strings.Contains(err.Error(), "denylist") {
			t.Errorf("rejection of %q should cite the denylist, got: %v", protected, err)
		}
	}
}

// A root inside a deny root can never be reached, so declaring one is a
// configuration error reported at startup, not a silently dead entry.
func TestApplyDeclaredAllowRoots_RejectsRootInsideDenyRoot(t *testing.T) {
	resetDeclared(t)
	home, err := trustedHome()
	if err != nil {
		t.Skipf("no trusted home: %v", err)
	}
	inside := filepath.Join(home, ".ssh", "exports")

	err = ApplyDeclaredAllowRoots([]PluginAllowRoots{{PluginName: "acme", Roots: []string{inside}}})
	if err == nil {
		t.Fatal("a root inside ~/.ssh was accepted; it can never be reached because deny wins")
	}
	if !strings.Contains(err.Error(), "denylist") || !strings.Contains(err.Error(), `plugin "acme"`) {
		t.Errorf("error should name the plugin and the denylist, got: %v", err)
	}
	if declaredAllowRoots.Load() != nil {
		t.Error("a rejected declaration must leave no roots installed")
	}
}

func TestApplyDeclaredAllowRoots_RejectsMalformedRoots(t *testing.T) {
	resetDeclared(t)
	cases := []struct{ name, root string }{
		{"relative", "exports"},
		{"dot relative", "./exports"},
		{"parent relative", "../exports"},
		{"empty", ""},
		{"blank", "   "},
		{"control character", "/opt/ex\x00ports"},
		{"bare home", "~"},
		{"bare home with slash", "~/"},
	}
	if runtime.GOOS == "windows" {
		cases = append(cases, struct{ name, root string }{"foreign absolute", "/opt/exports"})
	} else {
		cases = append(cases, struct{ name, root string }{"foreign absolute", `C:\exports`})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ApplyDeclaredAllowRoots([]PluginAllowRoots{{PluginName: "acme", Roots: []string{tc.root}}})
			if err == nil {
				t.Fatalf("root %q was accepted, want rejection", tc.root)
			}
			if !strings.Contains(err.Error(), `plugin "acme"`) {
				t.Errorf("error should name the plugin, got: %v", err)
			}
			if declaredAllowRoots.Load() != nil {
				t.Error("a rejected declaration must leave no roots installed")
			}
		})
	}
}

// A "~/" root is anchored to the account's home the way ~/files is: an
// invocation that controls $HOME cannot move it, and messages keep the
// declared spelling rather than the expanded account path.
func TestApplyDeclaredAllowRoots_TildeExpandsAgainstTrustedHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME is not the home-directory source on Windows")
	}
	resetDeclared(t)
	home, err := trustedHome()
	if err != nil {
		t.Skipf("no trusted home: %v", err)
	}
	if home == "/root" {
		t.Skip("the home directory is the /root deny root; a ~/ declaration is rejected there by design")
	}
	fake := t.TempDir()
	fake, _ = filepath.EvalSymlinks(fake)
	t.Setenv("HOME", fake)

	declare(t, "acme", "~/lark-cli-declared-test")

	d := declaredAllowRoots.Load()
	if d == nil || len(*d) != 1 {
		t.Fatalf("declared snapshot = %+v, want one entry", d)
	}
	e := (*d)[0]
	if strings.HasPrefix(e.literal, fake) || strings.HasPrefix(e.resolved, fake) {
		t.Fatalf("HOME moved the declared root into the environment-supplied home %q", fake)
	}
	if want := filepath.Join(home, "lark-cli-declared-test"); e.literal != want {
		t.Errorf("literal = %q, want %q (expanded against the account's home)", e.literal, want)
	}
	if e.label != "~/lark-cli-declared-test" {
		t.Errorf("label = %q, want the root as declared", e.label)
	}
}

func TestApplyDeclaredAllowRoots_MultipleOwnersRejected(t *testing.T) {
	resetDeclared(t)
	root := unusedDeclaredRoot(t)

	err := ApplyDeclaredAllowRoots([]PluginAllowRoots{
		{PluginName: "acme", Roots: []string{root}},
		{PluginName: "globex", Roots: []string{filepath.Join(root, "other")}},
	})
	if !errors.Is(err, ErrMultipleAllowPathsOwners) {
		t.Fatalf("err = %v, want ErrMultipleAllowPathsOwners", err)
	}
	for _, owner := range []string{"acme", "globex"} {
		if !strings.Contains(err.Error(), owner) {
			t.Errorf("error should name owner %q, got: %v", owner, err)
		}
	}
	if declaredAllowRoots.Load() != nil {
		t.Error("a rejected declaration must leave no roots installed")
	}
}

// A root declared twice is an authoring error rather than a silent no-op.
func TestApplyDeclaredAllowRoots_DuplicateRootRejected(t *testing.T) {
	resetDeclared(t)
	a := unusedDeclaredRoot(t)

	err := ApplyDeclaredAllowRoots([]PluginAllowRoots{{PluginName: "acme", Roots: []string{a, a + string(filepath.Separator)}}})
	if err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Fatalf("duplicate root: err = %v, want a 'declared more than once' rejection", err)
	}
	if declaredAllowRoots.Load() != nil {
		t.Error("a rejected declaration must leave no roots installed")
	}
}

// Each apply replaces the snapshot and an empty apply clears it, so a host
// that builds its command tree more than once per process always runs the
// declaration of its latest build.
func TestApplyDeclaredAllowRoots_ReplacesAndClears(t *testing.T) {
	resetDeclared(t)
	a := unusedDeclaredRoot(t)
	b := a + "-b"
	cwd := t.TempDir()

	declare(t, "acme", a)
	if roots := allowRoots(cwd); !hasLiteralRoot(roots, a) || hasLiteralRoot(roots, b) {
		t.Fatalf("after declaring a: roots = %+v", roots)
	}
	declare(t, "globex", b)
	if roots := allowRoots(cwd); hasLiteralRoot(roots, a) || !hasLiteralRoot(roots, b) {
		t.Fatalf("after declaring b: roots = %+v (a must be replaced)", roots)
	}
	if err := ApplyDeclaredAllowRoots(nil); err != nil {
		t.Fatalf("clearing: %v", err)
	}
	if roots := allowRoots(cwd); hasLiteralRoot(roots, a) || hasLiteralRoot(roots, b) {
		t.Fatalf("after clearing: roots = %+v", roots)
	}
}

// The rejection message lists the declared roots as the plugin wrote them, in
// the same flat comma-separated list as the built-in roots, so every item is a
// spelling the caller can paste. Which plugin declared them is operator
// information (config plugins show) and stays out of the message.
func TestAllowRootsLabel_ListsDeclaredRootsAsWritten(t *testing.T) {
	resetDeclared(t)
	home, err := trustedHome()
	if err != nil {
		t.Skipf("no trusted home: %v", err)
	}
	if home == "/root" {
		t.Skip("the home directory is the /root deny root; a ~/ declaration is rejected there by design")
	}
	root := unusedDeclaredRoot(t)
	declare(t, "acme", root, "~/lark-cli-declared-test")

	names := []string{"cwd"}
	for _, e := range fixedAllowRoots() {
		names = append(names, e.label)
	}
	want := strings.Join(append(names, root, "~/lark-cli-declared-test"), ", ")
	if got := allowRootsLabel(); got != want {
		t.Errorf("allowRootsLabel() = %q, want %q", got, want)
	}
	if expanded := filepath.Join(home, "lark-cli-declared-test"); strings.Contains(allowRootsLabel(), expanded) {
		t.Errorf("label echoes the expanded home path %q; it must keep the declared spelling", expanded)
	}

	_, err = SafeOutputPath(filepath.Join(root+"-outside", "out.txt"))
	if err == nil {
		t.Fatal("a path outside every root must be rejected")
	}
	if !strings.Contains(err.Error(), "~/lark-cli-declared-test") || strings.Contains(err.Error(), "plugin") {
		t.Errorf("rejection should list the declared roots without plugin attribution, got: %v", err)
	}
}

// The rejection carries the roots and nothing else for --output; the stdin
// escape is offered only where it exists, on the input side. Every extra
// sentence is paid for by the agent on every failed call.
func TestCheckAllow_MessageIsRootsOnlyPlusStdinEscapeForInputs(t *testing.T) {
	resetDeclared(t)
	outside := filepath.Join(unusedDeclaredRoot(t), "out.txt")

	_, err := SafeOutputPath(outside)
	if err == nil {
		t.Fatal("output path outside every root must be rejected")
	}
	want := fmt.Sprintf("--output %q is outside the built-in allowlist; allowed roots: %s", outside, allowRootsLabel())
	if err.Error() != want {
		t.Errorf("--output rejection:\n got  %q\n want %q", err.Error(), want)
	}

	_, err = SafeInputPath(outside)
	if err == nil {
		t.Fatal("input path outside every root must be rejected")
	}
	want = fmt.Sprintf("--file %q is outside the built-in allowlist; allowed roots: %s (or pass '-' to read from stdin where the flag supports it)", outside, allowRootsLabel())
	if err.Error() != want {
		t.Errorf("--file rejection:\n got  %q\n want %q", err.Error(), want)
	}
}

// The built-in wording is public documentation quoted by help text and skill
// references; pin it so the generated list stays byte-for-byte what it was.
func TestAllowRootsLabel_BuiltInWording(t *testing.T) {
	resetDeclared(t)
	if _, err := trustedHome(); err != nil {
		t.Skipf("no trusted home, ~/files is absent: %v", err)
	}
	names := []string{"cwd", "/tmp", "~/files"}
	if runtime.GOOS == "windows" {
		names[1] = "~/AppData/Local/Temp"
	}
	for _, name := range agentStateDirNames {
		names = append(names, "~/"+name)
	}
	want := strings.Join(names, ", ")
	if got := allowRootsLabel(); got != want {
		t.Errorf("allowRootsLabel() = %q, want %q", got, want)
	}
}
