// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errscontract

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fixtureRepo lays out a tiny repo on tmpfs that mimics the live layout enough
// for ScanRepo / CheckErrsContract to exercise. Each entry is path → content.
type fixtureRepo map[string]string

func writeFixture(t *testing.T, files fixtureRepo) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", full, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
	return root
}

func runGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	commandArgs := []string{
		"-c", "maintenance.autoDetach=false",
		"-c", "gc.autoDetach=false",
	}
	commandArgs = append(commandArgs, args...)
	cmd := exec.Command("git", commandArgs...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestRunGitDisablesDetachedMaintenance(t *testing.T) {
	for _, key := range []string{"maintenance.autoDetach", "gc.autoDetach"} {
		if got := runGit(t, t.TempDir(), "config", "--get", "--type=bool", key); got != "false" {
			t.Fatalf("%s = %q, want false", key, got)
		}
	}
}

func TestLoadSubtypeAllowlist_ExtractsTypedConstValues(t *testing.T) {
	root := writeFixture(t, fixtureRepo{
		"errs/subtypes.go": `package errs

type Subtype string

const (
	SubtypeMissingScope Subtype = "missing_scope"
	SubtypeRateLimit    Subtype = "rate_limit"
)

const (
	UnrelatedConst = "ignore_me" // not Subtype-typed
)
`,
	})
	got, err := LoadSubtypeAllowlist(filepath.Join(root, "errs", "subtypes.go"))
	if err != nil {
		t.Fatalf("LoadSubtypeAllowlist: %v", err)
	}
	want := map[string]struct{}{"missing_scope": {}, "rate_limit": {}}
	if len(got) != len(want) {
		t.Fatalf("size mismatch: got %d, want %d (%+v)", len(got), len(want), got)
	}
	for k := range want {
		if _, ok := got[k]; !ok {
			t.Errorf("missing %q in allowlist", k)
		}
	}
	if _, ok := got["ignore_me"]; ok {
		t.Errorf("untyped const leaked into allowlist")
	}
}

// TestLoadSubtypeAllowlists_WalksAllSubtypesFiles pins the multi-file load:
// constants from every errs/subtypes*.go must contribute to both the values
// allowlist and the declared-names set.
func TestLoadSubtypeAllowlists_WalksAllSubtypesFiles(t *testing.T) {
	root := writeFixture(t, fixtureRepo{
		"errs/subtypes.go": `package errs

type Subtype string

const (
	SubtypeMissingScope Subtype = "missing_scope"
)
`,
		"errs/subtypes_extra.go": `package errs

const (
	SubtypeExtraExample Subtype = "extra_example"
)
`,
	})
	values, names, err := LoadSubtypeAllowlists(filepath.Join(root, "errs"))
	if err != nil {
		t.Fatalf("LoadSubtypeAllowlists: %v", err)
	}
	for _, v := range []string{"missing_scope", "extra_example"} {
		if _, ok := values[v]; !ok {
			t.Errorf("values missing %q (across-file load broken)", v)
		}
	}
	for _, n := range []string{"SubtypeMissingScope", "SubtypeExtraExample"} {
		if _, ok := names[n]; !ok {
			t.Errorf("names missing %q (across-file load broken)", n)
		}
	}
}

func TestCheckErrsContract_FlagsMissingPredicateAndTest(t *testing.T) {
	root := writeFixture(t, fixtureRepo{
		"errs/types.go": `package errs

type Problem struct{}

type MissingError struct {
	Problem
}
`,
		"errs/predicates.go": `package errs
// IsMissing predicate intentionally absent
`,
		// No errs/*_test.go file → MissingError lacks test coverage.
		"internal/errclass/codemeta.go": `package errclass

type CodeMeta struct{}

var codeMeta = map[int]CodeMeta{1234: {}}
`,
	})
	v, err := CheckErrsContract(root)
	if err != nil {
		t.Fatalf("CheckErrsContract: %v", err)
	}
	var missingPredicate, missingTest int
	for _, vv := range v {
		switch {
		case strings.Contains(vv.Message, "no matching IsMissing predicate"):
			missingPredicate++
		case strings.Contains(vv.Message, "no test exercising it"):
			missingTest++
		}
		// Diagnostics emitted by CheckErrsContract must use repo-relative paths
		// (same convention as walker-side rules), not absolute filesystem paths
		// resolved via parser.ParseFile.
		if strings.Contains(vv.Message, "MissingError") && vv.File != "errs/types.go" {
			t.Errorf("violation File = %q, want repo-relative %q: %+v",
				vv.File, "errs/types.go", vv)
		}
	}
	if missingPredicate != 1 {
		t.Errorf("missing-predicate diagnostics = %d, want 1: %+v", missingPredicate, v)
	}
	if missingTest != 1 {
		t.Errorf("missing-test diagnostics = %d, want 1: %+v", missingTest, v)
	}
}

func TestCheckErrsContract_AcceptsCompleteContract(t *testing.T) {
	root := writeFixture(t, fixtureRepo{
		"errs/types.go": `package errs

type Problem struct{}

type FooError struct{ Problem }
`,
		"errs/predicates.go": `package errs

func IsFoo(err error) bool { return false }
`,
		"errs/foo_test.go": `package errs_test

import "testing"

func TestFooError(t *testing.T) { _ = FooError{} }
`,
		"internal/errclass/codemeta.go": `package errclass

type CodeMeta struct{}

var m = map[int]CodeMeta{42: {}}
`,
	})
	v, err := CheckErrsContract(root)
	if err != nil {
		t.Fatalf("CheckErrsContract: %v", err)
	}
	if len(v) != 0 {
		t.Errorf("complete contract should pass, got %d violations: %+v", len(v), v)
	}
}

func TestScanRepo_DetectsServiceRegistrarAndBadSubtype(t *testing.T) {
	root := writeFixture(t, fixtureRepo{
		"errs/types.go": `package errs

type Problem struct{}

type Subtype string

type FooError struct{ Problem }
`,
		"errs/predicates.go": `package errs

func IsFoo(err error) bool { return false }
`,
		"errs/foo_test.go": `package errs_test
import "testing"
func TestFooError(t *testing.T) { _ = FooError{} }
`,
		"errs/subtypes.go": `package errs

const (
	SubtypeKnown Subtype = "known"
)
`,
		"internal/errclass/codemeta.go": `package errclass

type CodeMeta struct{}

var m = map[int]CodeMeta{1: {}}
`,
		// Service file with a registrar AND a bad Subtype literal.
		"shortcuts/task/bad.go": `package task

func init() {
	mergeCodeMeta(nil, "task")
}

var _ = struct{ Subtype string }{Subtype: "not_known"}
`,
		// Test files are exempt from C/D/E (rule pre-filter).
		"shortcuts/task/bad_test.go": `package task
func placeholder() {}
`,
	})
	v, err := ScanRepo(root)
	if err != nil {
		t.Fatalf("ScanRepo: %v", err)
	}
	var sawRegistrar, sawBadSubtype bool
	for _, vv := range v {
		if vv.Rule == "no_registrar" && strings.Contains(vv.File, "shortcuts/task/bad.go") {
			sawRegistrar = true
		}
		if vv.Rule == "declared_subtype" && strings.Contains(vv.Message, "not_known") {
			sawBadSubtype = true
		}
	}
	if !sawRegistrar {
		t.Errorf("ScanRepo missed CheckNoRegistrar registrar; got %+v", v)
	}
	if !sawBadSubtype {
		t.Errorf("ScanRepo missed CheckDeclaredSubtype undeclared subtype; got %+v", v)
	}
}

func TestScanRepoWithOptionsLabelsAllowlistedCommandBoundaryError(t *testing.T) {
	cmdSrc := `package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func buildCmd() *cobra.Command {
	return &cobra.Command{RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("legacy user input")
	}}
}
`
	line := lineOf(cmdSrc, "legacy user input")
	addedAt := legacyCommandErrorCandidateDate(time.Now())
	root := writeFixture(t, fixtureRepo{
		"errs/types.go": `package errs

type Problem struct{}
type Subtype string
type FooError struct{ Problem }
`,
		"errs/predicates.go": `package errs

func IsFoo(err error) bool { return false }
`,
		"errs/foo_test.go": `package errs_test
import "testing"
func TestFooError(t *testing.T) { _ = FooError{} }
`,
		"errs/subtypes.go": `package errs

const (
	SubtypeKnown Subtype = "known"
)
`,
		"cmd/legacy.go": cmdSrc,
		"internal/qualitygate/config/allowlists/legacy-command-errors.txt": "cmd/legacy.go\t" +
			strings.TrimSpace(strconv.Itoa(line)) +
			"\tcli-owner\tlegacy command boundary bare error\t" + addedAt + "\n",
	})
	v, err := ScanRepoWithOptions(root, ScanOptions{})
	if err != nil {
		t.Fatalf("ScanRepoWithOptions: %v", err)
	}
	var sawLabel bool
	for _, vv := range v {
		if vv.Rule == "no_bare_command_error" {
			if vv.Action != ActionLabel {
				t.Fatalf("allowlisted boundary error should label, got %#v", vv)
			}
			sawLabel = true
		}
	}
	if !sawLabel {
		t.Fatalf("missing allowlisted boundary diagnostic: %#v", v)
	}
}

func TestScanRepoWithOptionsRejectsStaleCommandErrorAllowlistRows(t *testing.T) {
	addedAt := legacyCommandErrorCandidateDate(time.Now())
	root := writeFixture(t, fixtureRepo{
		"errs/types.go": `package errs

type Problem struct{}
type Subtype string
type FooError struct{ Problem }
`,
		"errs/predicates.go": `package errs

func IsFoo(err error) bool { return false }
`,
		"errs/foo_test.go": `package errs_test
import "testing"
func TestFooError(t *testing.T) { _ = FooError{} }
`,
		"errs/subtypes.go": `package errs

const (
	SubtypeKnown Subtype = "known"
)
`,
		"cmd/clean.go": `package cmd

import "github.com/spf13/cobra"

func buildCmd() *cobra.Command {
	return &cobra.Command{RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	}}
}
`,
		"internal/qualitygate/config/allowlists/legacy-command-errors.txt": "cmd/clean.go\t7\tcli-owner\tlegacy command boundary bare error\t" + addedAt + "\n",
	})
	v, err := ScanRepoWithOptions(root, ScanOptions{})
	if err != nil {
		t.Fatalf("ScanRepoWithOptions: %v", err)
	}
	for _, vv := range v {
		if vv.Rule == "legacy_command_error_allowlist" &&
			vv.Action == ActionReject &&
			vv.File == "internal/qualitygate/config/allowlists/legacy-command-errors.txt" &&
			vv.Line == 1 {
			return
		}
	}
	t.Fatalf("missing stale allowlist reject: %#v", v)
}

func TestScanRepoWithOptionsKeepsAllowlistedUnchangedCommandErrorInChangedScope(t *testing.T) {
	cmdSrc := `package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func buildCmd() *cobra.Command {
	return &cobra.Command{RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("legacy user input")
	}}
}
`
	line := lineOf(cmdSrc, "legacy user input")
	addedAt := legacyCommandErrorCandidateDate(time.Now())
	root := writeFixture(t, fixtureRepo{
		"errs/types.go": `package errs

type Problem struct{}
type Subtype string
type FooError struct{ Problem }
`,
		"errs/predicates.go": `package errs

func IsFoo(err error) bool { return false }
`,
		"errs/foo_test.go": `package errs_test
import "testing"
func TestFooError(t *testing.T) { _ = FooError{} }
`,
		"errs/subtypes.go": `package errs

const (
	SubtypeKnown Subtype = "known"
)
`,
		"cmd/legacy.go": cmdSrc,
		"README.md":     "base\n",
		"internal/qualitygate/config/allowlists/legacy-command-errors.txt": "cmd/legacy.go\t" +
			strings.TrimSpace(strconv.Itoa(line)) +
			"\tcli-owner\tlegacy command boundary bare error\t" + addedAt + "\n",
	})
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test User")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "base")
	base := runGit(t, root, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	runGit(t, root, "add", "README.md")
	runGit(t, root, "commit", "-m", "change docs")

	v, err := ScanRepoWithOptions(root, ScanOptions{ChangedFrom: base})
	if err != nil {
		t.Fatalf("ScanRepoWithOptions: %v", err)
	}
	var sawLabel bool
	for _, vv := range v {
		if vv.Rule == "legacy_command_error_allowlist" && vv.Action == ActionReject {
			t.Fatalf("allowlisted unchanged boundary must not be rejected as stale: %#v", vv)
		}
		if vv.Rule == "no_bare_command_error" {
			if vv.Action != ActionLabel {
				t.Fatalf("allowlisted unchanged boundary should remain LABEL, got %#v", vv)
			}
			sawLabel = true
		}
	}
	if !sawLabel {
		t.Fatalf("missing allowlisted unchanged boundary diagnostic: %#v", v)
	}
}

// TestScanRepo_EmitsAdvisoryWhenTypedScopeUnavailable pins Refinement 2:
// when a fixture LOOKS like a Go repo (has a go.mod) but typed loading
// cannot produce a usable errs.Subtype const set, ScanRepo emits a single
// ActionWarning advisory so reviewers know CheckDeclaredSubtype ran in a less-strict
// mode. ActionWarning is print-only — CI exit-code logic does not fail
// the run on it (proven by the lint main.go exit-code branch).
func TestScanRepo_EmitsAdvisoryWhenTypedScopeUnavailable(t *testing.T) {
	// Fixture: a Go-looking repo (has go.mod) but errs/ contains a
	// Subtype type with NO declared Subtype consts. LoadTypedScope will
	// initialize but errsSubtypeConsts stays empty → Enabled() returns
	// false under the tightened contract.
	root := writeFixture(t, fixtureRepo{
		"go.mod": "module example.com/fixture\n\ngo 1.23\n",
		"errs/types.go": `package errs

type Problem struct{}
type Subtype string
type FooError struct{ Problem }
`,
		"errs/predicates.go": `package errs
func IsFoo(err error) bool { return false }
`,
		"errs/foo_test.go": `package errs_test
import "testing"
func TestFooError(t *testing.T) { _ = FooError{} }
`,
		// subtypes.go is present so LoadSubtypeAllowlists succeeds, but the
		// const block is empty so no values/names are declared.
		"errs/subtypes.go": `package errs

const SubtypeKnown Subtype = "known"
`,
	})
	v, err := ScanRepo(root)
	if err != nil {
		t.Fatalf("ScanRepo: %v", err)
	}

	advisoryCount := 0
	for _, vv := range v {
		if vv.Rule == "declared_subtype" && vv.Action == ActionWarning &&
			strings.Contains(vv.Message, "typed resolution unavailable") {
			advisoryCount++
		}
	}
	if advisoryCount != 1 {
		t.Errorf("advisory count = %d, want exactly 1; got violations: %+v", advisoryCount, v)
	}
	// The advisory must NOT escalate to REJECT — ActionWarning is print-only.
	// (We don't assert rejectCount==0 in general since the fixture may emit
	// other rejections; we only assert the advisory itself is a WARNING.)
	for _, vv := range v {
		if vv.Action == ActionReject && strings.Contains(vv.Message, "typed resolution unavailable") {
			t.Errorf("advisory must be ActionWarning, not REJECT (would fail CI): %+v", vv)
		}
	}
}

// TestScanRepo_NoAdvisoryWithoutGoMod pins the scoping: fixtures that lack
// a go.mod (the common unit-test shape) must NOT emit the advisory, since
// the workspace is not a Go repo from the loader's perspective.
func TestScanRepo_NoAdvisoryWithoutGoMod(t *testing.T) {
	root := writeFixture(t, fixtureRepo{
		"errs/types.go": `package errs
type Problem struct{}
type Subtype string
type FooError struct{ Problem }
`,
		"errs/predicates.go": `package errs
func IsFoo(err error) bool { return false }
`,
		"errs/foo_test.go": `package errs_test
import "testing"
func TestFooError(t *testing.T) { _ = FooError{} }
`,
		"errs/subtypes.go": `package errs
const SubtypeKnown Subtype = "known"
`,
	})
	v, err := ScanRepo(root)
	if err != nil {
		t.Fatalf("ScanRepo: %v", err)
	}
	for _, vv := range v {
		if strings.Contains(vv.Message, "typed resolution unavailable") {
			t.Errorf("no go.mod present → advisory must not fire; got %+v", vv)
		}
	}
}

func TestScanRepo_LabelTriggerForAdHocSubtype(t *testing.T) {
	root := writeFixture(t, fixtureRepo{
		"errs/types.go": `package errs
type Problem struct{}
type Subtype string
type FooError struct{ Problem }
`,
		"errs/predicates.go": `package errs
func IsFoo(err error) bool { return false }
`,
		"errs/foo_test.go": `package errs_test
import "testing"
func TestFooError(t *testing.T) { _ = FooError{} }
`,
		"errs/subtypes.go": `package errs
const (
	SubtypeKnown Subtype = "known"
)
`,
		"internal/errclass/codemeta.go": `package errclass
type CodeMeta struct{}
var m = map[int]CodeMeta{}
`,
		"shortcuts/task/maybe.go": `package task
var _ = struct{ Subtype string }{Subtype: "ad_hoc_quota_breach"}
`,
	})
	v, err := ScanRepo(root)
	if err != nil {
		t.Fatalf("ScanRepo: %v", err)
	}
	var sawLabel bool
	for _, vv := range v {
		if vv.Action == ActionLabel &&
			strings.Contains(vv.Message, "needs-taxonomy-decision") {
			sawLabel = true
		}
		if vv.Action == ActionReject &&
			strings.Contains(vv.Message, "ad_hoc_quota_breach") {
			t.Errorf("ad_hoc_* must NOT be REJECTED (it's LABEL): %+v", vv)
		}
	}
	if !sawLabel {
		t.Errorf("ScanRepo missed CheckAdHocSubtype label trigger; got %+v", v)
	}
}
