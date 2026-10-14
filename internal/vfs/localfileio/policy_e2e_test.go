// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package localfileio

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// The policy is built once per process from the account's home directory, so a
// test that wants a home of its own needs a process of its own. These run the
// public entry points in a child with HOME pointed at a fixture, which is the
// only way to exercise the real call path, the process-level caches and the
// order one validation leaves behind for the next.

const (
	fixtureHomeEnv = "LARK_CLI_POLICY_FIXTURE_HOME"
	fixtureCaseEnv = "LARK_CLI_POLICY_FIXTURE_CASE"
)

// TestPolicyFixtureHelperProcess is the child half. It reports one line per
// validation ("allowed" / "denied: <reason>") and one "touched" line per
// credential path the policy read, which the parent asserts on.
func TestPolicyFixtureHelperProcess(t *testing.T) {
	home := os.Getenv(fixtureHomeEnv)
	if home == "" {
		t.Skip("helper process for TestPolicy_PublicEntryPointsUnderFixtureHome")
	}
	probe := installProbeFS(t)
	runFixtureCase(t, home, os.Getenv(fixtureCaseEnv))
	for _, name := range []string{".ssh", ".npmrc", ".aws", ".gitconfig"} {
		if len(probe.touched(name)) != 0 {
			fmt.Printf("touched %s\n", name)
		}
	}
}

// runFixtureCase performs one named sequence of validations against the fixture
// home. Cases that validate an ordinary file first are the ones that matter
// most: they leave the process caches warm, which is the state a later
// credential access has to survive.
func runFixtureCase(t *testing.T, home, name string) {
	t.Helper()
	work := filepath.Join(home, "work")
	if err := os.Chdir(work); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	switch name {
	case "ordinary-output":
		report(SafeOutputPath(filepath.Join(work, "out.txt")))
		report(SafeOutputPath("out.txt"))
	case "deny-named-directory":
		report(SafeInputPath(filepath.Join(home, ".ssh", "id_rsa")))
	case "deny-relative-from-home":
		if err := os.Chdir(home); err != nil {
			t.Fatalf("Chdir: %v", err)
		}
		report(SafeInputPath(filepath.Join(".ssh", "id_rsa")))
	case "deny-hard-link-after-ordinary":
		report(SafeOutputPath("out.txt"))
		report(LocalInputPath(filepath.Join(home, "report.txt")))
	case "deny-symlinked-root-after-ordinary":
		report(SafeOutputPath("out.txt"))
		report(LocalInputPath(filepath.Join(home, "dotfiles", ".gitconfig")))
	case "rename-between-validations":
		report(SafeOutputPath("out.txt"))
		if err := os.Rename(filepath.Join(home, ".ssh"), filepath.Join(home, "archive")); err != nil {
			t.Fatalf("Rename: %v", err)
		}
		report(LocalInputPath(filepath.Join(home, "archive", "id_rsa")))
	default:
		t.Fatalf("unknown fixture case %q", name)
	}
}

func report(_ string, err error) {
	if err == nil {
		fmt.Println("allowed")
		return
	}
	fmt.Printf("denied: %s\n", strings.ReplaceAll(err.Error(), "\n", " "))
}

// TestPolicy_PublicEntryPointsUnderFixtureHome drives SafeOutputPath,
// SafeInputPath and LocalInputPath against a home directory built for the test.
func TestPolicy_PublicEntryPointsUnderFixtureHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME is not the home-directory source on Windows")
	}
	home := buildFixtureHome(t)

	for _, tc := range []struct {
		name         string
		wantVerdicts []string
		wantReason   string
		// wantTouched is the exact set of credential names the validation may
		// read, asserted only where the case pins it.
		wantTouched []string
	}{
		{
			name:         "ordinary-output",
			wantVerdicts: []string{"allowed", "allowed"},
			// ~/.gitconfig is a symlink in the fixture, and a deny root that can
			// point anywhere has to be resolved whatever the target is. The plain
			// ones — ~/.ssh, ~/.npmrc, ~/.aws — must stay untouched, which is the
			// whole point of #2726.
			wantTouched: []string{".gitconfig"},
		},
		{
			// An accepted limit, recorded so that changing it is a decision
			// rather than an accident. Resolving the roots up front used to pin
			// their identities for the life of the process, which incidentally
			// kept matching a credential directory that was renamed afterwards.
			// Pinning an identity means stat-ing the path to learn it, which is
			// exactly what #2726 asks the CLI to stop doing, so the two cannot
			// both hold. Across processes — every ordinary CLI invocation — the
			// pinning never applied in the first place: a fresh process reads
			// the renamed tree and allows it either way.
			name:         "rename-between-validations",
			wantVerdicts: []string{"allowed", "allowed"},
		},
		{
			name:         "deny-named-directory",
			wantVerdicts: []string{"denied"},
			wantReason:   "~/.ssh",
		},
		{
			name:         "deny-relative-from-home",
			wantVerdicts: []string{"denied"},
			wantReason:   "~/.ssh",
		},
		{
			name:         "deny-hard-link-after-ordinary",
			wantVerdicts: []string{"allowed", "denied"},
			wantReason:   "~/.npmrc",
		},
		{
			name:         "deny-symlinked-root-after-ordinary",
			wantVerdicts: []string{"allowed", "denied"},
			wantReason:   "~/.gitconfig",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			verdicts, touched := runFixtureChild(t, home, tc.name)
			assertVerdicts(t, verdicts, tc.wantVerdicts, tc.wantReason)
			if tc.wantTouched != nil && !slices.Equal(touched, tc.wantTouched) {
				t.Errorf("validation read %v of the credential paths, want exactly %v", touched, tc.wantTouched)
			}
		})
	}
}

func assertVerdicts(t *testing.T, got, want []string, reason string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d verdicts %v, want %d", len(got), got, len(want))
	}
	for i, verdict := range got {
		if !strings.HasPrefix(verdict, want[i]) {
			t.Fatalf("validation %d: got %q, want %q", i+1, verdict, want[i])
		}
	}
	last := got[len(got)-1]
	if reason == "" {
		return
	}
	if !strings.Contains(last, reason) || !strings.Contains(last, "denylist") {
		t.Errorf("rejection should cite %s and the denylist, got: %s", reason, last)
	}
}

// buildFixtureHome lays out a home directory holding one of every shape the
// policy has to tell apart: a credential directory, a credential file with a
// second hard link under an innocent name, a credential file that is a symlink
// into a dotfiles directory, and an ordinary working directory.
func buildFixtureHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	home, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	for _, dir := range []string{"work", "dotfiles", ".ssh"} {
		if err := os.MkdirAll(filepath.Join(home, dir), 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}
	write := func(path, content string) {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	write(filepath.Join(home, ".ssh", "id_rsa"), "PRIVATE KEY")
	write(filepath.Join(home, ".npmrc"), "//registry.npmjs.org/:_authToken=fixture")
	write(filepath.Join(home, "dotfiles", ".gitconfig"), "[user]\n\tname = fixture")
	if err := os.Link(filepath.Join(home, ".npmrc"), filepath.Join(home, "report.txt")); err != nil {
		t.Skipf("cannot create the probe hard link: %v", err)
	}
	if err := os.Symlink(filepath.Join(home, "dotfiles", ".gitconfig"), filepath.Join(home, ".gitconfig")); err != nil {
		t.Skipf("cannot create the probe symlink: %v", err)
	}
	return home
}

// runFixtureChild runs one case and splits the child's report into the
// validation verdicts and the credential paths it read.
func runFixtureChild(t *testing.T, home, name string) (verdicts, touched []string) {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestPolicyFixtureHelperProcess$")
	command.Env = append(os.Environ(),
		"HOME="+home,
		fixtureHomeEnv+"="+home,
		fixtureCaseEnv+"="+name,
	)
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("helper process failed: %v\n%s", err, out)
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case line == "allowed" || strings.HasPrefix(line, "denied: "):
			verdicts = append(verdicts, line)
		case strings.HasPrefix(line, "touched "):
			touched = append(touched, strings.TrimPrefix(line, "touched "))
		}
	}
	return verdicts, touched
}
