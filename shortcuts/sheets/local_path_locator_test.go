// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/shortcuts/common"
)

// newLocalPathRuntime mounts the three locator flags the way withLocalPathLocator
// and flag-defs together do, so parseSpreadsheetRef sees the same flag set a real
// command gives it.
func newLocalPathRuntime(t *testing.T, url, token, localPath string) *common.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "sheets"}
	cmd.Flags().String("url", url, "")
	cmd.Flags().String("spreadsheet-token", token, "")
	cmd.Flags().String(localPathFlag, localPath, "")
	return common.TestNewRuntimeContext(cmd, testConfig(t))
}

// writeLocalWorkbook creates a file for the locator to name and returns its path.
// The bytes are never read — only the path is hashed — so the content is
// deliberately not a real workbook.
func writeLocalWorkbook(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("local workbook"), 0o600); err != nil {
		t.Fatalf("cannot write %s: %v", path, err)
	}
	return path
}

// registeredShortcut returns a shortcut as the CLI registers it, decorations
// included. Mounting the package global instead would test an undecorated
// command that no caller ever gets.
func registeredShortcut(t *testing.T, command string) common.Shortcut {
	t.Helper()
	for _, s := range Shortcuts() {
		if s.Command == command {
			return s
		}
	}
	t.Fatalf("no registered shortcut named %q", command)
	return common.Shortcut{}
}

// TestParseSpreadsheetRefLocalPath pins what --local-path resolves to: the token
// common.CreateLocalOfficeToken derives from the path, classified as a sheet ref
// so nothing downstream tries to resolve it further.
func TestParseSpreadsheetRefLocalPath(t *testing.T) {
	t.Parallel()
	path := writeLocalWorkbook(t, "report.xlsx")
	want, err := common.CreateLocalOfficeToken(path, common.LocalOfficeSheets)
	if err != nil {
		t.Fatalf("cannot derive the expected token: %v", err)
	}

	ref, err := parseSpreadsheetRef(newLocalPathRuntime(t, "", "", path))
	if err != nil {
		t.Fatalf("parseSpreadsheetRef returned error: %v", err)
	}
	if ref.Kind != spreadsheetRefSheet {
		t.Fatalf("ref.Kind = %q, want %q", ref.Kind, spreadsheetRefSheet)
	}
	if ref.Token != want {
		t.Fatalf("ref.Token = %q, want %q", ref.Token, want)
	}
	// The derived token has to keep reading as a local office token, since that
	// is what selects the office_sheet_file parent_type for an image upload.
	if !common.IsLocalOfficeToken(ref.Token) {
		t.Fatalf("IsLocalOfficeToken(%q) = false, want true", ref.Token)
	}
	if got := sheetMediaParentType(ref.Token); got != officeSheetFileParentType {
		t.Fatalf("sheetMediaParentType(%q) = %q, want %q", ref.Token, got, officeSheetFileParentType)
	}
}

// TestParseSpreadsheetRefLocalPathIsDeterministic pins the property the whole
// locator rests on: the same path names the same document on every call, so a
// caller can address a local file across commands without carrying a token.
func TestParseSpreadsheetRefLocalPathIsDeterministic(t *testing.T) {
	t.Parallel()
	path := writeLocalWorkbook(t, "report.xlsx")
	first, err := parseSpreadsheetRef(newLocalPathRuntime(t, "", "", path))
	if err != nil {
		t.Fatalf("first parseSpreadsheetRef returned error: %v", err)
	}
	second, err := parseSpreadsheetRef(newLocalPathRuntime(t, "", "", path))
	if err != nil {
		t.Fatalf("second parseSpreadsheetRef returned error: %v", err)
	}
	if first.Token != second.Token {
		t.Fatalf("same path gave two tokens: %q then %q", first.Token, second.Token)
	}

	// An uncleaned spelling of the same path resolves to the same document.
	// Two genuinely different spellings (relative vs absolute) do not, which is
	// the documented ceiling on localSpreadsheetToken.
	noisy := filepath.Join(filepath.Dir(path), ".", filepath.Base(path))
	same, err := parseSpreadsheetRef(newLocalPathRuntime(t, "", "", noisy))
	if err != nil {
		t.Fatalf("parseSpreadsheetRef on the uncleaned path returned error: %v", err)
	}
	if same.Token != first.Token {
		t.Fatalf("cleaning the path changed the token: %q vs %q", same.Token, first.Token)
	}
}

// TestParseSpreadsheetRefLocalPathRejects covers the inputs that must not reach
// the backend as a well-formed token.
//
// The missing-file case is the one that earns its keep. A token is derivable
// from any string, so without the existence check a mistyped path produces a
// perfectly valid token for a document that never existed, and the caller meets
// their own typo as a "not found" from the server.
func TestParseSpreadsheetRefLocalPathRejects(t *testing.T) {
	t.Parallel()
	existing := writeLocalWorkbook(t, "report.xlsx")

	cases := []struct {
		name      string
		url       string
		token     string
		localPath string
	}{
		{name: "no locator at all"},
		{name: "local path that does not exist", localPath: filepath.Join(t.TempDir(), "missing.xlsx")},
		{name: "local path naming a directory", localPath: t.TempDir()},
		{name: "local path with url", url: testURL, localPath: existing},
		{name: "local path with spreadsheet token", token: testToken, localPath: existing},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ref, err := parseSpreadsheetRef(newLocalPathRuntime(t, tc.url, tc.token, tc.localPath))
			if err == nil {
				t.Fatalf("want an error, got ref = %+v", ref)
			}
		})
	}
}

// TestLocalPathLocatorMounting pins which shortcuts carry the flag. It is keyed
// off --spreadsheet-token rather than a hand-written command list, so a shortcut
// added later inherits the locator without anyone remembering to.
func TestLocalPathLocatorMounting(t *testing.T) {
	t.Parallel()
	for _, s := range Shortcuts() {
		hasFlag := func(name string) bool {
			for _, f := range s.Flags {
				if f.Name == name {
					return true
				}
			}
			return false
		}
		wantLocalPath := hasFlag("spreadsheet-token")
		if got := hasFlag(localPathFlag); got != wantLocalPath {
			t.Errorf("%s: --%s mounted = %v, want %v (it follows --spreadsheet-token)",
				s.Command, localPathFlag, got, wantLocalPath)
		}
	}
}

// TestLocalPathLocatorLeavesCreateCommandsAlone names the two shortcuts that
// must never take the locator: both CREATE a spreadsheet, so there is no
// existing document for a path to address, and +workbook-import already spells
// its own local file --file.
func TestLocalPathLocatorLeavesCreateCommandsAlone(t *testing.T) {
	t.Parallel()
	for _, s := range Shortcuts() {
		if s.Command != "+workbook-create" && s.Command != "+workbook-import" {
			continue
		}
		for _, f := range s.Flags {
			if f.Name == localPathFlag {
				t.Errorf("%s must not carry --%s", s.Command, localPathFlag)
			}
		}
	}
}

// TestLocalPathLocatorPendingSpecRows is the counterpart to the skip in
// TestFlagsFor_EveryRegisteredCommandHasDefs, and it retires itself.
//
// The flag is mounted from Go while the sheet-skill-spec rows that generate
// flag-defs.json are still pending. The day those rows land this fails, and the
// fix is to delete withLocalPathLocator, this test, and that skip, leaving the
// flag to arrive through the generated set like every other one.
func TestLocalPathLocatorPendingSpecRows(t *testing.T) {
	t.Parallel()
	defs, err := loadFlagDefs()
	if err != nil {
		t.Fatal(err)
	}
	for command, spec := range defs {
		for _, df := range spec.Flags {
			if df.Name == localPathFlag {
				t.Fatalf("%s declares --%s in flag-defs.json: the spec rows have landed, so drop "+
					"withLocalPathLocator, this test, and the skip in "+
					"TestFlagsFor_EveryRegisteredCommandHasDefs", command, localPathFlag)
			}
		}
	}
}

// TestLocalPathReservedInsideBatchSubOp keeps a repeated locator harmless inside
// a +batch-update operation. The top-level locator is authoritative, so the
// sub-op copy is dropped with a warning rather than rejected as an unknown key —
// the same treatment url and spreadsheet_token already get.
func TestLocalPathReservedInsideBatchSubOp(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"local_path", "local-path"} {
		if !isReservedSubOpKey(key) {
			t.Errorf("isReservedSubOpKey(%q) = false, want true", key)
		}
	}
}

// TestExecute_LocalPathAddressesTheDerivedToken runs a real shortcut end to end
// on --local-path. The stub answers only the derived token's endpoint, so the
// test fails if any step between the flag and the request URL drops the
// derivation — which is the whole reason the locator exists.
func TestExecute_LocalPathAddressesTheDerivedToken(t *testing.T) {
	t.Parallel()
	path := writeLocalWorkbook(t, "quarterly.xlsx")
	token, err := common.CreateLocalOfficeToken(path, common.LocalOfficeSheets)
	if err != nil {
		t.Fatalf("cannot derive the expected token: %v", err)
	}

	// Mount the shortcut as registration does. The package globals are the
	// undecorated originals; --local-path is added by Shortcuts(), which is what
	// shortcuts/register.go registers.
	out, err := runShortcutWithStubs(t, registeredShortcut(t, "+sheet-list"), []string{"--" + localPathFlag, path},
		toolOutputStub(token, "read", workbookStructureOutput))
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}

	var envelope struct {
		OK   bool          `json:"ok"`
		Data []interface{} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("failed to decode envelope: %v\nraw=%s", err, out)
	}
	if !envelope.OK || len(envelope.Data) != 2 {
		t.Fatalf("envelope.ok=%v, data len=%d; out=%s", envelope.OK, len(envelope.Data), out)
	}
}

// TestBuildToolBodyLocalPath pins the field's presence rule at the one function
// every sheet_ai body goes through. Omitted rather than empty when there is no
// local path: a request that could be sent before this existed must not change
// shape.
func TestBuildToolBodyLocalPath(t *testing.T) {
	t.Parallel()
	input := map[string]interface{}{"excel_id": "shtABC"}

	withPath, err := buildToolBody("/tmp/report.xlsx", "get_cell_ranges", input)
	if err != nil {
		t.Fatalf("buildToolBody returned error: %v", err)
	}
	if got := withPath["local_path"]; got != "/tmp/report.xlsx" {
		t.Fatalf("local_path = %v, want /tmp/report.xlsx", got)
	}
	if got := withPath["tool_name"]; got != "get_cell_ranges" {
		t.Fatalf("tool_name = %v, want get_cell_ranges", got)
	}

	withoutPath, err := buildToolBody("", "get_cell_ranges", input)
	if err != nil {
		t.Fatalf("buildToolBody returned error: %v", err)
	}
	if _, present := withoutPath["local_path"]; present {
		t.Fatalf("local_path present with no local path: %#v", withoutPath)
	}
}

// TestLocalPathForBodyMatchesTheHashedSeed keeps the reported path and the
// hashed one the same string. The field is only worth sending if the receiver
// can re-derive the token from it, which a differently-normalized spelling
// would quietly break.
func TestLocalPathForBodyMatchesTheHashedSeed(t *testing.T) {
	t.Parallel()
	path := writeLocalWorkbook(t, "report.xlsx")
	noisy := filepath.Join(filepath.Dir(path), ".", filepath.Base(path))
	runtime := newLocalPathRuntime(t, "", "", noisy)

	reported := localPathForBody(runtime)
	ref, err := parseSpreadsheetRef(runtime)
	if err != nil {
		t.Fatalf("parseSpreadsheetRef returned error: %v", err)
	}
	rederived, err := common.CreateLocalOfficeToken(reported, common.LocalOfficeSheets)
	if err != nil {
		t.Fatalf("cannot re-derive from the reported path: %v", err)
	}
	if rederived != ref.Token {
		t.Fatalf("token re-derived from local_path %q is %q, want %q", reported, rederived, ref.Token)
	}
}

// TestLocalPathForBodyEmptyWithoutTheFlag covers the locators that are not a
// local file, including a command that never mounts the flag at all.
func TestLocalPathForBodyEmptyWithoutTheFlag(t *testing.T) {
	t.Parallel()
	if got := localPathForBody(newLocalPathRuntime(t, testURL, "", "")); got != "" {
		t.Fatalf("localPathForBody with --url = %q, want empty", got)
	}
	if got := localPathForBody(newLocalPathRuntime(t, "", testToken, "")); got != "" {
		t.Fatalf("localPathForBody with --spreadsheet-token = %q, want empty", got)
	}
	// A command without the flag at all, as +workbook-create is.
	bare := &cobra.Command{Use: "sheets"}
	bare.Flags().String("title", "", "")
	if got := localPathForBody(common.TestNewRuntimeContext(bare, testConfig(t))); got != "" {
		t.Fatalf("localPathForBody on a command without the flag = %q, want empty", got)
	}
}

// TestDryRun_LocalPathMatchesTheExecutedBody is the reason localPath is a
// parameter of buildToolBody rather than something stamped on at the two wire
// moments. A preview that omitted the field would advertise a request the
// execute path does not send.
func TestDryRun_LocalPathMatchesTheExecutedBody(t *testing.T) {
	t.Parallel()
	path := writeLocalWorkbook(t, "quarterly.xlsx")

	out, err := runShortcut(t, registeredShortcut(t, "+sheet-list"),
		[]string{"--" + localPathFlag, path, "--dry-run"})
	if err != nil {
		t.Fatalf("dry run failed: %v\nout=%s", err, out)
	}
	var envelope struct {
		Data struct {
			API []struct {
				Body map[string]interface{} `json:"body"`
			} `json:"api"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("failed to decode envelope: %v\nraw=%s", err, out)
	}
	if len(envelope.Data.API) != 1 {
		t.Fatalf("api len = %d, want 1; out=%s", len(envelope.Data.API), out)
	}
	if got := envelope.Data.API[0].Body["local_path"]; got != path {
		t.Fatalf("dry-run body local_path = %v, want %q", got, path)
	}
}

// TestDryRun_NoLocalPathForRemoteLocators pins the other half: a spreadsheet
// located by URL carries no local_path at all.
func TestDryRun_NoLocalPathForRemoteLocators(t *testing.T) {
	t.Parallel()
	out, err := runShortcut(t, registeredShortcut(t, "+sheet-list"), []string{"--url", testURL, "--dry-run"})
	if err != nil {
		t.Fatalf("dry run failed: %v\nout=%s", err, out)
	}
	if strings.Contains(out, "local_path") {
		t.Fatalf("a --url dry run mentions local_path: %s", out)
	}
}
