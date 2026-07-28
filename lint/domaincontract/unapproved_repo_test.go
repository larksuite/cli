// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package domaincontract

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/lint/lintapi"
)

func gitTestCommand(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func setupDomainDiffRepo(t *testing.T, target string) (root, base string) {
	t.Helper()
	root = t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/domainfixture\n\ngo 1.23.0\n")
	writeFile(t, root, publicDomainsPath, "# public\npublic.example.com\n")
	writeFile(t, root, fixtureDomainsPath, "# fixtures\nfixture.example.com\n")
	writeFile(t, root, "policy_refs.go", "package sample\n\nvar APIHost = \"public.example.com\"\n")
	writeFile(t, root, "policy_refs_test.go", "package sample\n\nvar FixtureHost = \"fixture.example.com\"\n")
	writeFile(t, root, "target.go", target)

	gitTestCommand(t, root, "init", "-q")
	gitTestCommand(t, root, "config", "user.name", "Domain Contract Test")
	gitTestCommand(t, root, "config", "user.email", "domain-contract@example.com")
	gitTestCommand(t, root, "add", ".")
	gitTestCommand(t, root, "-c", "commit.gpgsign=false", "commit", "-qm", "base")
	return root, gitTestCommand(t, root, "rev-parse", "HEAD")
}

func commitDomainDiff(t *testing.T, root, message string) {
	t.Helper()
	gitTestCommand(t, root, "add", "-A")
	gitTestCommand(t, root, "-c", "commit.gpgsign=false", "commit", "-qm", message)
}

func violationsForRule(vs []lintapi.Violation, rule string) []lintapi.Violation {
	var out []lintapi.Violation
	for _, v := range vs {
		if v.Rule == rule {
			out = append(out, v)
		}
	}
	return out
}

func scanDomainDiff(t *testing.T, root, base string) []lintapi.Violation {
	t.Helper()
	vs, err := ScanRepoWithOptions(root, ScanOptions{ChangedFrom: base})
	if err != nil {
		t.Fatal(err)
	}
	return vs
}

func TestUnapprovedDomainDiffContract(t *testing.T) {
	t.Run("new PR 1975 case", func(t *testing.T) {
		root, base := setupDomainDiffRepo(t, "package sample\n\nvar unrelated = 1\n")
		writeFile(t, root, "target.go",
			"package sample\n\nvar unrelated = 1\nvar APIHost = \"internal-api-drive-stream.larkoffice.com\"\n")
		commitDomainDiff(t, root, "add internal host")

		got := violationsForRule(scanDomainDiff(t, root, base), unapprovedDomainRule)
		if len(got) != 1 || !strings.Contains(got[0].Message, "internal-api-drive-stream.larkoffice.com") {
			t.Fatalf("violations = %+v, want PR 1975 hostname", got)
		}
	})

	t.Run("hostname field in nested Go module", func(t *testing.T) {
		root, base := setupDomainDiffRepo(t, "package sample\n\nvar unrelated = 1\n")
		writeFile(t, root, "nested/go.mod", "module example.com/nested\n\ngo 1.23.0\n")
		writeFile(t, root, "nested/target.go",
			"package nested\n\ntype Config struct{ Host string }\n\n"+
				"var config = Config{Host: \"private.corp.internal\"}\n")
		commitDomainDiff(t, root, "add nested module hostname")

		all := scanDomainDiff(t, root, base)
		got := violationsForRule(all, unapprovedDomainRule)
		if len(got) != 1 || filepath.ToSlash(got[0].File) != "nested/target.go" ||
			!strings.Contains(got[0].Message, "private.corp.internal") {
			t.Fatalf("violations = %+v, want nested-module hostname rejection", got)
		}
		if incomplete := violationsForRule(all, incompleteDomainRule); len(incomplete) != 0 {
			t.Fatalf("nested module must have complete type information: %+v", incomplete)
		}
	})

	t.Run("changed excluded field reports incomplete scan", func(t *testing.T) {
		root, base := setupDomainDiffRepo(t, "package sample\n\nvar unrelated = 1\n")
		writeFile(t, root, "excluded.go",
			"//go:build domaincontract_never && !domaincontract_never\n\npackage sample\n\n"+
				"type Config struct{ Host string }\n\n"+
				"var config = Config{Host: \"private.corp.internal\"}\n")
		commitDomainDiff(t, root, "add excluded hostname field")

		all := scanDomainDiff(t, root, base)
		got := violationsForRule(all, incompleteDomainRule)
		if len(got) != 1 || filepath.Base(got[0].File) != "excluded.go" || got[0].Line != 7 {
			t.Fatalf("violations = %+v, want changed field scan-incomplete at line 7", got)
		}
		if unapproved := violationsForRule(all, unapprovedDomainRule); len(unapproved) != 0 {
			t.Fatalf("untyped field must not produce an unverified hostname finding: %+v", unapproved)
		}
	})

	t.Run("changed excluded selector reports incomplete scan", func(t *testing.T) {
		root, base := setupDomainDiffRepo(t, "package sample\n\nvar unrelated = 1\n")
		writeFile(t, root, "excluded.go",
			"//go:build domaincontract_never && !domaincontract_never\n\npackage sample\n\n"+
				"type Config struct{ Host string }\n\n"+
				"func configure(config *Config) { config.Host = \"private.corp.internal\" }\n")
		commitDomainDiff(t, root, "add excluded hostname selector")

		got := violationsForRule(scanDomainDiff(t, root, base), incompleteDomainRule)
		if len(got) != 1 || filepath.Base(got[0].File) != "excluded.go" || got[0].Line != 7 {
			t.Fatalf("violations = %+v, want changed selector scan-incomplete at line 7", got)
		}
	})

	t.Run("changed excluded named slice reports incomplete scan", func(t *testing.T) {
		root, base := setupDomainDiffRepo(t, "package sample\n\nvar unrelated = 1\n")
		writeFile(t, root, "excluded.go",
			"//go:build domaincontract_never && !domaincontract_never\n\npackage sample\n\n"+
				"type HostList []string\n\n"+
				"var AllowedHosts = HostList{\n\t\"attacker.zip\",\n}\n")
		commitDomainDiff(t, root, "add excluded hostname slice")

		all := scanDomainDiff(t, root, base)
		got := violationsForRule(all, incompleteDomainRule)
		if len(got) != 1 || filepath.Base(got[0].File) != "excluded.go" || got[0].Line != 8 {
			t.Fatalf("violations = %+v, want named-slice scan-incomplete at line 8", got)
		}
		if unapproved := violationsForRule(all, unapprovedDomainRule); len(unapproved) != 0 {
			t.Fatalf("untyped named slice must not produce an unverified hostname finding: %+v", unapproved)
		}
	})

	t.Run("changed excluded named map reports incomplete scan", func(t *testing.T) {
		root, base := setupDomainDiffRepo(t, "package sample\n\nvar unrelated = 1\n")
		writeFile(t, root, "excluded.go",
			"//go:build domaincontract_never && !domaincontract_never\n\npackage sample\n\n"+
				"type HostSet map[string]struct{}\n\n"+
				"var AllowedHosts = HostSet{\n\t\"attacker.zip\": {},\n}\n")
		commitDomainDiff(t, root, "add excluded hostname map")

		all := scanDomainDiff(t, root, base)
		got := violationsForRule(all, incompleteDomainRule)
		if len(got) != 1 || filepath.Base(got[0].File) != "excluded.go" || got[0].Line != 8 {
			t.Fatalf("violations = %+v, want named-map scan-incomplete at line 8", got)
		}
		if unapproved := violationsForRule(all, unapprovedDomainRule); len(unapproved) != 0 {
			t.Fatalf("untyped named map must not produce an unverified hostname finding: %+v", unapproved)
		}
	})

	t.Run("changed excluded unrelated code stays allowed", func(t *testing.T) {
		root, base := setupDomainDiffRepo(t, "package sample\n\nvar unrelated = 1\n")
		writeFile(t, root, "excluded.go",
			"//go:build domaincontract_never && !domaincontract_never\n\npackage sample\n\nvar unrelated = 2\n")
		commitDomainDiff(t, root, "add excluded unrelated code")

		if got := violationsForRule(scanDomainDiff(t, root, base), incompleteDomainRule); len(got) != 0 {
			t.Fatalf("unrelated excluded code must not require hostname type information: %+v", got)
		}
	})

	t.Run("new element in existing collection", func(t *testing.T) {
		root, base := setupDomainDiffRepo(t,
			"package sample\n\nvar ExtraHosts = []string{\n\t\"public.example.com\",\n}\n")
		writeFile(t, root, "target.go",
			"package sample\n\nvar ExtraHosts = []string{\n\t\"public.example.com\",\n\t\"attacker.zip\",\n}\n")
		commitDomainDiff(t, root, "add collection host")

		got := violationsForRule(scanDomainDiff(t, root, base), unapprovedDomainRule)
		if len(got) != 1 || !strings.Contains(got[0].Message, "attacker.zip") {
			t.Fatalf("violations = %+v, want attacker.zip", got)
		}
		if got[0].Line != 5 {
			t.Fatalf("violation line = %d, want 5", got[0].Line)
		}
	})

	t.Run("multiline expression changed segment", func(t *testing.T) {
		root, base := setupDomainDiffRepo(t,
			"package sample\n\nvar ExtraHost = \"private.corp.\" +\n\t\"example.com\"\n")
		writeFile(t, root, "target.go",
			"package sample\n\nvar ExtraHost = \"private.corp.\" +\n\t\"internal\"\n")
		commitDomainDiff(t, root, "change concatenated host")

		got := violationsForRule(scanDomainDiff(t, root, base), unapprovedDomainRule)
		if len(got) != 1 || !strings.Contains(got[0].Message, "private.corp.internal") {
			t.Fatalf("violations = %+v, want private.corp.internal", got)
		}
		if got[0].Line != 4 {
			t.Fatalf("violation line = %d, want changed line 4", got[0].Line)
		}
	})

	t.Run("unrelated change beside historical hostname", func(t *testing.T) {
		root, base := setupDomainDiffRepo(t,
			"package sample\n\nvar HistoricalHost = \"historical.private.internal\"\n")
		writeFile(t, root, "target.go",
			"package sample\n\nvar HistoricalHost = \"historical.private.internal\"\nvar unrelated = 1\n")
		commitDomainDiff(t, root, "add unrelated value")

		if got := violationsForRule(scanDomainDiff(t, root, base), unapprovedDomainRule); len(got) != 0 {
			t.Fatalf("unexpected historical-domain violation: %+v", got)
		}
	})

	t.Run("historical hostname expression changed", func(t *testing.T) {
		root, base := setupDomainDiffRepo(t,
			"package sample\n\nvar HistoricalHost = \"historical.private.internal\"\n")
		writeFile(t, root, "target.go",
			"package sample\n\nvar HistoricalHost = \"replacement.private.internal\"\n")
		commitDomainDiff(t, root, "change historical host")

		got := violationsForRule(scanDomainDiff(t, root, base), unapprovedDomainRule)
		if len(got) != 1 || !strings.Contains(got[0].Message, "replacement.private.internal") {
			t.Fatalf("violations = %+v, want replacement.private.internal", got)
		}
	})

	t.Run("new assignment references existing constant", func(t *testing.T) {
		root, base := setupDomainDiffRepo(t,
			"package sample\n\nconst existingConst = \"private.corp.internal\"\n")
		writeFile(t, root, "target.go",
			"package sample\n\nconst existingConst = \"private.corp.internal\"\nvar APIHost = existingConst\n")
		commitDomainDiff(t, root, "use existing hostname constant")

		got := violationsForRule(scanDomainDiff(t, root, base), unapprovedDomainRule)
		if len(got) != 1 || !strings.Contains(got[0].Message, "private.corp.internal") {
			t.Fatalf("violations = %+v, want private.corp.internal", got)
		}
		if got[0].Line != 4 {
			t.Fatalf("violation line = %d, want 4", got[0].Line)
		}
	})

	t.Run("allowlisted hostname", func(t *testing.T) {
		root, base := setupDomainDiffRepo(t, "package sample\n\nvar unrelated = 1\n")
		writeFile(t, root, "target.go",
			"package sample\n\nvar unrelated = 1\nvar BackupHost = \"public.example.com\"\n")
		commitDomainDiff(t, root, "add public host")

		if got := violationsForRule(scanDomainDiff(t, root, base), unapprovedDomainRule); len(got) != 0 {
			t.Fatalf("unexpected public-domain violation: %+v", got)
		}
	})

	t.Run("reserved example URL", func(t *testing.T) {
		root, base := setupDomainDiffRepo(t, "package sample\n\nvar unrelated = 1\n")
		writeFile(t, root, "target.go",
			"package sample\n\nvar unrelated = 1\nfunc fakeValue() string { return \"https://example.test/resource\" }\n")
		commitDomainDiff(t, root, "add safe example URL")

		if got := violationsForRule(scanDomainDiff(t, root, base), unapprovedDomainRule); len(got) != 0 {
			t.Fatalf("unexpected reserved-example violation: %+v", got)
		}
	})

	t.Run("historical type gap suppresses unused policy diagnostics", func(t *testing.T) {
		root, _ := setupDomainDiffRepo(t, "package sample\n\nvar unrelated = 1\n")
		writeFile(t, root, publicDomainsPath,
			"# public\nplatform.example.com\npublic.example.com\n")
		writeFile(t, root, "excluded.go",
			"//go:build domaincontract_never && !domaincontract_never\n\npackage sample\n\n"+
				"type Config struct{ Host string }\n\n"+
				"var config = Config{Host: \"platform.example.com\"}\n")
		commitDomainDiff(t, root, "add historical platform hostname")
		base := gitTestCommand(t, root, "rev-parse", "HEAD")

		writeFile(t, root, "target.go", "package sample\n\nvar unrelated = 2\n")
		commitDomainDiff(t, root, "change unrelated code")

		all := scanDomainDiff(t, root, base)
		if got := violationsForRule(all, incompleteDomainRule); len(got) != 0 {
			t.Fatalf("historical type gap must not be attributed to this change: %+v", got)
		}
		if got := violationsForRule(all, unusedDomainRule); len(got) != 0 {
			t.Fatalf("incomplete inventory must not produce unused-policy diagnostics: %+v", got)
		}
	})

	t.Run("allowlist does not approve subdomains", func(t *testing.T) {
		root, base := setupDomainDiffRepo(t, "package sample\n\nvar unrelated = 1\n")
		writeFile(t, root, "target.go",
			"package sample\n\nvar unrelated = 1\nvar BackupHost = \"evil.public.example.com\"\n")
		commitDomainDiff(t, root, "add unapproved public subdomain")

		got := violationsForRule(scanDomainDiff(t, root, base), unapprovedDomainRule)
		if len(got) != 1 || !strings.Contains(got[0].Message, "evil.public.example.com") {
			t.Fatalf("violations = %+v, want evil.public.example.com", got)
		}
	})

	t.Run("multi assignment pairs names and values", func(t *testing.T) {
		root, base := setupDomainDiffRepo(t, "package sample\n\nvar unrelated = 1\n")
		writeFile(t, root, publicDomainsPath,
			"# public\nopen.larksuite.com\npublic.example.com\n")
		writeFile(t, root, "target.go",
			"package sample\n\nvar unrelated = 1\nvar APIHost, BackupHost = \"open.larksuite.com\", \"attacker.zip\"\n")
		commitDomainDiff(t, root, "add multiple hosts")

		got := violationsForRule(scanDomainDiff(t, root, base), unapprovedDomainRule)
		if len(got) != 1 || !strings.Contains(got[0].Message, "attacker.zip") {
			t.Fatalf("violations = %+v, want only attacker.zip", got)
		}
	})

	t.Run("IDN hostname is rejected", func(t *testing.T) {
		root, base := setupDomainDiffRepo(t, "package sample\n\nvar unrelated = 1\n")
		writeFile(t, root, "target.go",
			"package sample\n\nvar unrelated = 1\nvar BackupHost = \"例子.公司.cn\"\n")
		commitDomainDiff(t, root, "add IDN hostname")

		got := violationsForRule(scanDomainDiff(t, root, base), unapprovedDomainRule)
		if len(got) != 1 || !strings.Contains(got[0].Message, "例子.公司.cn") {
			t.Fatalf("violations = %+v, want IDN hostname", got)
		}
	})

	t.Run("fixture limited to test files", func(t *testing.T) {
		root, base := setupDomainDiffRepo(t, "package sample\n\nvar unrelated = 1\n")
		writeFile(t, root, "target.go",
			"package sample\n\nvar unrelated = 1\nvar ProductionHost = \"fixture.example.com\"\n")
		commitDomainDiff(t, root, "use fixture in production")

		got := violationsForRule(scanDomainDiff(t, root, base), unapprovedDomainRule)
		if len(got) != 1 || !strings.Contains(got[0].Message, "fixture.example.com") {
			t.Fatalf("violations = %+v, want production fixture rejection", got)
		}
		if !strings.Contains(got[0].Suggestion, "fixture-only hostname") ||
			strings.Contains(got[0].Suggestion, "public allowlist") {
			t.Fatalf("suggestion = %q, want fixture-scope guidance", got[0].Suggestion)
		}
	})

	t.Run("fixture accepted in test file", func(t *testing.T) {
		root, base := setupDomainDiffRepo(t, "package sample\n\nvar unrelated = 1\n")
		writeFile(t, root, "new_target_test.go",
			"package sample\n\nvar BackupHost = \"fixture.example.com\"\n")
		commitDomainDiff(t, root, "use fixture in test")

		if got := violationsForRule(scanDomainDiff(t, root, base), unapprovedDomainRule); len(got) != 0 {
			t.Fatalf("unexpected fixture-domain violation: %+v", got)
		}
	})

	t.Run("fixture allowlist does not approve subdomains", func(t *testing.T) {
		root, base := setupDomainDiffRepo(t, "package sample\n\nvar unrelated = 1\n")
		writeFile(t, root, "new_target_test.go",
			"package sample\n\nvar BackupHost = \"evil.fixture.example.com\"\n")
		commitDomainDiff(t, root, "use unapproved fixture subdomain")

		got := violationsForRule(scanDomainDiff(t, root, base), unapprovedDomainRule)
		if len(got) != 1 || !strings.Contains(got[0].Message, "evil.fixture.example.com") {
			t.Fatalf("violations = %+v, want exact fixture match", got)
		}
	})

	t.Run("fixture rejected in skills", func(t *testing.T) {
		root, base := setupDomainDiffRepo(t, "package sample\n\nvar unrelated = 1\n")
		writeFile(t, root, "skills/example/example_test.go",
			"package example\n\nvar BackupHost = \"fixture.example.com\"\n")
		commitDomainDiff(t, root, "use fixture in skill")

		got := violationsForRule(scanDomainDiff(t, root, base), unapprovedDomainRule)
		if len(got) != 1 || !strings.Contains(got[0].Message, "fixture.example.com") {
			t.Fatalf("violations = %+v, want skill fixture rejection", got)
		}
	})

	t.Run("pure rename", func(t *testing.T) {
		root, base := setupDomainDiffRepo(t,
			"package sample\n\nvar HistoricalHost = \"historical.private.internal\"\n")
		gitTestCommand(t, root, "mv", "target.go", "renamed.go")
		commitDomainDiff(t, root, "rename file")

		if got := violationsForRule(scanDomainDiff(t, root, base), unapprovedDomainRule); len(got) != 0 {
			t.Fatalf("unexpected rename violation: %+v", got)
		}
	})
}

func TestUnapprovedDomainPolicyAndFailurePaths(t *testing.T) {
	t.Run("unused policy entry", func(t *testing.T) {
		root, base := setupDomainDiffRepo(t, "package sample\n\nvar unrelated = 1\n")
		writeFile(t, root, publicDomainsPath,
			"# public\npublic.example.com\nunused.example.com\n")
		commitDomainDiff(t, root, "add unused policy")

		got := violationsForRule(scanDomainDiff(t, root, base), unusedDomainRule)
		if len(got) != 1 || !strings.Contains(got[0].Message, "unused.example.com") {
			t.Fatalf("violations = %+v, want unused.example.com", got)
		}
	})

	t.Run("public entry used only by fixture", func(t *testing.T) {
		root, base := setupDomainDiffRepo(t, "package sample\n\nvar unrelated = 1\n")
		writeFile(t, root, publicDomainsPath,
			"# public\npublic.example.com\ntest-only.example.com\n")
		writeFile(t, root, "public_only_test.go",
			"package sample\n\nvar BackupHost = \"test-only.example.com\"\n")
		commitDomainDiff(t, root, "add test-only public policy")

		got := violationsForRule(scanDomainDiff(t, root, base), unusedDomainRule)
		if len(got) != 1 || !strings.Contains(got[0].Message, "test-only.example.com") {
			t.Fatalf("violations = %+v, want test-only.example.com", got)
		}
	})

	t.Run("changed Go parse failure", func(t *testing.T) {
		root, base := setupDomainDiffRepo(t, "package sample\n\nvar unrelated = 1\n")
		writeFile(t, root, "target.go", "package sample\n\nfunc broken(\n")
		commitDomainDiff(t, root, "break source")

		all := scanDomainDiff(t, root, base)
		got := violationsForRule(all, incompleteDomainRule)
		if len(got) != 1 || filepath.Base(got[0].File) != "target.go" {
			t.Fatalf("violations = %+v, want target.go scan-incomplete", got)
		}
		if unused := violationsForRule(all, unusedDomainRule); len(unused) != 0 {
			t.Fatalf("parse failure must not produce unreliable unused-policy diagnostics: %+v", unused)
		}
	})

	t.Run("repository type loading failure", func(t *testing.T) {
		root, base := setupDomainDiffRepo(t, "package sample\n\nvar unrelated = 1\n")
		writeFile(t, root, "go.mod", "module example.com/domainfixture\n\ngo 1.23.0\n\n"+
			"require example.com/missing v0.0.0\n\nreplace example.com/missing => ./missing\n")
		writeFile(t, root, "target.go",
			"package sample\n\nimport _ \"example.com/missing\"\n\n"+
				"type Config struct{ Host string }\nvar config = Config{Host: \"malicious.corp.internal\"}\n")
		commitDomainDiff(t, root, "break type loading")

		all := scanDomainDiff(t, root, base)
		got := violationsForRule(all, incompleteDomainRule)
		if len(got) != 1 || filepath.Base(got[0].File) != "go.mod" {
			t.Fatalf("violations = %+v, want go.mod scan-incomplete", got)
		}
		if !strings.Contains(got[0].Message, "load Go type information") {
			t.Fatalf("message = %q, want type-loading failure", got[0].Message)
		}
		if unused := violationsForRule(all, unusedDomainRule); len(unused) != 0 {
			t.Fatalf("type-loading failure must not produce unreliable unused-policy diagnostics: %+v", unused)
		}
	})
}
