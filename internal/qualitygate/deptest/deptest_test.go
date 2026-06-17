// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deptest

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

var forbiddenRuntimeDeps = []*regexp.Regexp{
	regexp.MustCompile(`^github\.com/larksuite/cli/cmd($|/)`),
	regexp.MustCompile(`^github\.com/larksuite/cli/shortcuts($|/)`),
	regexp.MustCompile(`^github\.com/larksuite/cli/events($|/)`),
	regexp.MustCompile(`^github\.com/larksuite/cli/internal/cmdutil$`),
	regexp.MustCompile(`^github\.com/larksuite/cli/internal/registry$`),
	regexp.MustCompile(`^github\.com/larksuite/cli/internal/client$`),
	regexp.MustCompile(`^github\.com/larksuite/cli/internal/credential($|/)`),
	regexp.MustCompile(`^github\.com/larksuite/cli/extension/credential($|/)`),
	regexp.MustCompile(`^github\.com/larksuite/oapi-sdk-go/v3/service($|/)`),
	regexp.MustCompile(`^github\.com/spf13/cobra$`),
	regexp.MustCompile(`^github\.com/spf13/pflag$`),
}

func TestQualityGateCoreDoesNotDependOnCLIRuntime(t *testing.T) {
	root := repoRoot(t)
	packages := []string{
		"./internal/qualitygate/manifest",
		"./internal/qualitygate/facts",
		"./internal/qualitygate/rules",
		"./internal/qualitygate/semantic",
		"./internal/qualitygate/cmd/quality-gate",
		"./internal/qualitygate/cmd/semantic-review",
	}
	for _, pkg := range packages {
		t.Run(pkg, func(t *testing.T) {
			deps := goListDeps(t, root, false, pkg)
			deps = append(deps, goListDeps(t, root, true, pkg)...)
			for _, dep := range deps {
				for _, re := range forbiddenRuntimeDeps {
					if re.MatchString(dep) {
						t.Fatalf("%s must not depend on CLI runtime package %s", pkg, dep)
					}
				}
			}
		})
	}
}

func TestManifestExportIsTheOnlyRuntimeCollector(t *testing.T) {
	root := repoRoot(t)
	deps := goListDeps(t, root, false, "./internal/qualitygate/cmd/manifest-export")
	if !containsDep(deps, "github.com/larksuite/cli/cmd") {
		t.Fatal("manifest-export should be the explicit command-tree collector")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-parse --show-toplevel failed: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

func goListDeps(t *testing.T, root string, includeTests bool, pkg string) []string {
	t.Helper()
	args := []string{"list", "-deps"}
	if includeTests {
		args = append(args, "-test")
	}
	args = append(args, pkg)
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var deps []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			deps = append(deps, line)
		}
	}
	return deps
}

func containsDep(deps []string, want string) bool {
	for _, dep := range deps {
		if dep == want {
			return true
		}
	}
	return false
}
