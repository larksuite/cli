// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	_ "github.com/larksuite/cli/internal/vfs/localfileio"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func newCSVGuardRuntime(csvVal string) *common.RuntimeContext {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("csv", "", "")
	cmd.ParseFlags(nil)
	cmd.Flags().Set("csv", csvVal)
	return &common.RuntimeContext{Cmd: cmd}
}

// TestGuardCSVValueIsNotFilePath covers the existing-file tier: a bare --csv
// value naming a real file is a forgotten "@". The prescription names the fix
// with a <path> placeholder — the untrusted value must not be spliced into
// command-shaped text an agent would copy verbatim.
func TestGuardCSVValueIsNotFilePath(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	if err := os.WriteFile("data.csv", []byte("a,b\n1,2\n"), 0644); err != nil {
		t.Fatal(err)
	}

	err := guardCSVValueIsNotFilePath(newCSVGuardRuntime("data.csv"))
	ve := requireValidation(t, err, "existing file")
	if !strings.Contains(ve.Message, `"data.csv"`) {
		t.Errorf("message should name the offending value as data, got: %q", ve.Message)
	}
	if !strings.Contains(ve.Message, "--csv @<path>") {
		t.Errorf("message should prescribe the @ form via placeholder, got: %q", ve.Message)
	}
	if strings.Contains(ve.Message, "@data.csv") {
		t.Errorf("message must not splice the value into a command fragment, got: %q", ve.Message)
	}
	if ve.Param != "--csv" {
		t.Errorf("param = %q, want --csv", ve.Param)
	}
}

// TestGuardCSVValueIsNotFilePath_MissingButPathShaped covers the second tier.
// A path that doesn't resolve used to pass through and be written into the
// cell verbatim — a wrong value with a success exit code. The common source is
// an absolute path: `@` rejects those, so the caller drops the `@` and retries.
// Since the file can't be read from cwd, the prescription is stdin.
func TestGuardCSVValueIsNotFilePath_MissingButPathShaped(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	for _, v := range []string{
		"nope.csv",          // relative path from another working directory
		"./missing.csv",     // explicit relative prefix
		"../sibling/x.tsv",  // parent-relative
		"/tmp/nope.csv",     // absolute — the `@`-rejected case
		"~/data.tsv",        // home-relative
		"/var/tmp/export",   // no extension, but an unmistakable path prefix
		"C:/Users/me/a.csv", // windows-style, still ASCII path shape
	} {
		err := guardCSVValueIsNotFilePath(newCSVGuardRuntime(v))
		ve := requireValidation(t, err, "looks like a file path")
		if !strings.Contains(ve.Hint, "--csv @") || !strings.Contains(ve.Hint, "--csv - <") {
			t.Errorf("value %q: hint should offer both @file and stdin, got: %q", v, ve.Hint)
		}
		// The untrusted value must never appear inside the command-shaped
		// hint: "--csv - < $(id).csv" copied by an agent would expand in a
		// POSIX shell. The value is only named as quoted data in the message.
		if strings.Contains(ve.Hint, v) {
			t.Errorf("value %q: hint must not splice the raw value into a command fragment, got: %q", v, ve.Hint)
		}
		if !strings.Contains(ve.Message, v) {
			t.Errorf("value %q: message should still name the offending value, got: %q", v, ve.Message)
		}
	}
}

// TestGuardCSVValueIsNotFilePath_SkipsResolvedInput pins the origin rule that
// makes the shape heuristic safe: a value that arrived via @file / stdin is
// never inspected, however path-shaped its content — so the hint's promise
// that stdin writes such text verbatim actually holds, and a correct
// `--csv @file` invocation can't be re-rejected for its content.
func TestGuardCSVValueIsNotFilePath_SkipsResolvedInput(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	if err := os.WriteFile("data.csv", []byte("a,b\n1,2\n"), 0644); err != nil {
		t.Fatal(err)
	}

	for _, v := range []string{
		"nope.csv", // path-shaped, missing — rejected when inline
		"data.csv", // names an existing file — rejected when inline
	} {
		rctx := newCSVGuardRuntime(v)
		common.TestMarkInputResolved(rctx, "csv")
		if err := guardCSVValueIsNotFilePath(rctx); err != nil {
			t.Errorf("resolved value %q must skip the guard, got: %v", v, err)
		}
	}
}

// TestGuardCSVValueIsNotFilePath_PassesThrough pins what must still reach the
// sheet untouched. The prose cases are why the guard checks a narrow shape
// instead of "contains a filename": an earlier name-shape heuristic rejected
// them. "N/A" and "README.md" pin the two narrowing rules — a slash alone is
// not a path, and a filename alone is not a CSV path.
func TestGuardCSVValueIsNotFilePath_PassesThrough(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	for _, v := range []string{
		"改完记得更新config.json",           // CJK prose ending in a filename
		"remember to update data.csv", // prose mentioning a file
		"a,b\n1,2",                    // multi-cell CSV
		"hello world",
		"N/A",             // slash, but no CSV extension and no path prefix
		"README.md",       // filename shape, not a CSV one
		"report 2026.csv", // has a space: content, not a path
		"",
	} {
		if err := guardCSVValueIsNotFilePath(newCSVGuardRuntime(v)); err != nil {
			t.Errorf("content %q must pass through, got: %v", v, err)
		}
	}
}

// newCSVFileAliasRuntime is newCSVGuardRuntime plus the record chainFlagAliases
// leaves when the value was typed as --file.
func newCSVFileAliasRuntime(csvVal string) *common.RuntimeContext {
	rctx := newCSVGuardRuntime(csvVal)
	rctx.Cmd.Annotations = map[string]string{aliasSourceAnnotation("csv"): "file"}
	return rctx
}

// TestResolveCSVPathFromFileAlias covers the value-side half of the file → csv
// alias: --file names a path, so a value naming a readable file is read like
// `--csv @<path>` instead of being written into the sheet as literal text.
func TestResolveCSVPathFromFileAlias(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	if err := os.WriteFile("data.csv", []byte("a,b\n1,2\n"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("reads the named file", func(t *testing.T) {
		rctx := newCSVFileAliasRuntime("./data.csv")
		if err := resolveCSVPathFromFileAlias(rctx); err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if got, _ := rctx.Cmd.Flags().GetString("csv"); got != "a,b\n1,2\n" {
			t.Errorf("--csv = %q, want the file contents", got)
		}
	})

	t.Run("the contents are marked source-resolved", func(t *testing.T) {
		// Contents read from a file may legitimately look like anything,
		// including a path — the shape guards downstream must see the same bit
		// they would for --csv @<path>, or a one-cell CSV holding "report.csv"
		// is rejected as a caller who forgot the @.
		if err := os.WriteFile("pathshaped.csv", []byte("report.csv\n"), 0644); err != nil {
			t.Fatal(err)
		}
		rctx := newCSVFileAliasRuntime("./pathshaped.csv")
		if err := resolveCSVPathFromFileAlias(rctx); err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if !rctx.InputResolvedFromSource("csv") {
			t.Error("the rewritten value must be marked as read from a source")
		}
		if err := guardCSVValueIsNotFilePath(rctx); err != nil {
			t.Errorf("path-shaped file contents must survive the guard, got: %v", err)
		}
	})

	t.Run("an out-of-tree path is rejected toward stdin", func(t *testing.T) {
		err := resolveCSVPathFromFileAlias(newCSVFileAliasRuntime("/tmp/data.csv"))
		ve := requireValidation(t, err, "relative path")
		if ve.Param != "--file" {
			t.Errorf("param = %q, want the flag the caller actually typed", ve.Param)
		}
		if ve.Cause == nil {
			t.Error("the underlying path error should be preserved as Cause")
		}
		if !strings.Contains(ve.Hint, "--csv - <") {
			t.Errorf("hint should offer stdin for a file outside the tree, got: %q", ve.Hint)
		}
	})

	t.Run("inline CSV text still passes through", func(t *testing.T) {
		// --file holding literal CSV was accepted before this rule existed;
		// naming nothing readable, it stays the --csv guard's business.
		rctx := newCSVFileAliasRuntime("a,b\n1,2")
		if err := resolveCSVPathFromFileAlias(rctx); err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if got, _ := rctx.Cmd.Flags().GetString("csv"); got != "a,b\n1,2" {
			t.Errorf("--csv = %q, want the value untouched", got)
		}
		if rctx.InputResolvedFromSource("csv") {
			t.Error("a value that was not read from a file must not be marked resolved")
		}
	})

	t.Run("a value typed as --csv is not re-read as a path", func(t *testing.T) {
		// Without the alias record the rule must not fire: --csv promises text,
		// and its own guard (not this one) answers a path passed to it.
		rctx := newCSVGuardRuntime("./data.csv")
		if err := resolveCSVPathFromFileAlias(rctx); err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if got, _ := rctx.Cmd.Flags().GetString("csv"); got != "./data.csv" {
			t.Errorf("--csv = %q, want --csv left to its guard", got)
		}
	})

	t.Run("@file and stdin values are already contents", func(t *testing.T) {
		rctx := newCSVFileAliasRuntime("./data.csv")
		common.TestMarkInputResolved(rctx, "csv")
		if err := resolveCSVPathFromFileAlias(rctx); err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if got, _ := rctx.Cmd.Flags().GetString("csv"); got != "./data.csv" {
			t.Errorf("--csv = %q, want a resolved value left alone", got)
		}
	})
}

// TestResolveCSVPathFromFileAlias_UnreadablePaths pins that every unreadable
// path is answered under --file, the flag the caller typed. Handing these to
// the --csv guard instead named the wrong flag and, for a file that exists but
// cannot be opened, prescribed "pass it with @" — advice that routes through
// this very reader and fails identically.
func TestResolveCSVPathFromFileAlias_UnreadablePaths(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	t.Run("a path-shaped value that names nothing", func(t *testing.T) {
		err := resolveCSVPathFromFileAlias(newCSVFileAliasRuntime("./typo.csv"))
		ve := requireValidation(t, err, "names no file under the current directory")
		if ve.Param != "--file" {
			t.Errorf("param = %q, want --file", ve.Param)
		}
		if ve.Cause == nil {
			t.Error("the underlying read error should be preserved as Cause")
		}
	})

	t.Run("a file that exists but cannot be read", func(t *testing.T) {
		if err := os.WriteFile("noread.csv", []byte("a,b\n"), 0o000); err != nil {
			t.Fatal(err)
		}
		if _, err := os.ReadFile("noread.csv"); err == nil {
			t.Skip("running with rights that ignore file modes")
		}
		err := resolveCSVPathFromFileAlias(newCSVFileAliasRuntime("./noread.csv"))
		ve := requireValidation(t, err, "cannot read file")
		if ve.Param != "--file" {
			t.Errorf("param = %q, want --file", ve.Param)
		}
		if ve.Cause == nil {
			t.Error("the underlying read error should be preserved as Cause")
		}
		if strings.Contains(ve.Hint, "@") {
			t.Errorf("@file shares this reader, so it cannot be the fix; hint was %q", ve.Hint)
		}
	})

	t.Run("a directory", func(t *testing.T) {
		if err := os.Mkdir("adir", 0o755); err != nil {
			t.Fatal(err)
		}
		err := resolveCSVPathFromFileAlias(newCSVFileAliasRuntime("./adir"))
		ve := requireValidation(t, err, "cannot read file")
		if ve.Param != "--file" {
			t.Errorf("param = %q, want --file", ve.Param)
		}
		if ve.Cause == nil {
			t.Error("the underlying read error should be preserved as Cause")
		}
	})
}
