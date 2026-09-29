// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	extcs "github.com/larksuite/cli/extension/contentsafety"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestRecordReadsRejectDeprecatedMarkdownBeforeAPI(t *testing.T) {
	for _, shortcut := range []common.Shortcut{BaseRecordList, BaseRecordSearch, BaseRecordGet} {
		for _, output := range []bool{false, true} {
			t.Run(shortcut.Command+map[bool]string{false: "", true: " with output"}[output], func(t *testing.T) {
				factory, stdout, _ := newExecuteFactory(t)
				args := []string{shortcut.Command, "--base-token", "app_x", "--table-id", "tbl_x", "--format", "markdown"}
				if output {
					args = append(args, "--output", "records.ndjson")
				}
				err := runShortcut(t, shortcut, args, factory, stdout)
				problem, ok := errs.ProblemOf(err)
				var validation *errs.ValidationError
				if !ok || problem.Subtype != errs.SubtypeInvalidArgument || !errors.As(err, &validation) || validation.Param != "--format" || !strings.Contains(err.Error(), "deprecated format") || !strings.Contains(problem.Hint, "--format ndjson") {
					t.Fatalf("problem = %#v, err = %v", problem, err)
				}
				if stdout.Len() != 0 {
					t.Fatalf("unexpected stdout: %s", stdout.String())
				}
			})
		}
	}
}

func TestRecordGetDefaultsToNDJSONArtifact(t *testing.T) {
	dir := t.TempDir()
	withBaseWorkingDir(t, dir)
	factory, stdout, registry := newExecuteFactory(t)
	registry.Register(&httpmock.Stub{Method: "POST", URL: "/records/batch_get",
		Body: map[string]any{"code": 0, "data": recordMatrixPage(0, 1, false, "fld_name")}})
	if err := runShortcut(t, BaseRecordGet, []string{"+record-get", "--base-token", "app_x", "--table-id", "tbl_x", "--record-id", "rec_0000"}, factory, stdout); err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest["records_count"] != float64(1) {
		t.Fatalf("manifest = %#v", manifest)
	}
	raw, err := os.ReadFile(manifest["record_file"].(string))
	if err != nil {
		t.Fatal(err)
	}
	var row map[string]any
	if err := json.Unmarshal(raw, &row); err != nil {
		t.Fatal(err)
	}
	if row["record_id"] != "rec_0000" || row["Name"] != "Name 0" {
		t.Fatalf("row = %#v", row)
	}
}

type recordReadCSTestProvider struct{}

func (*recordReadCSTestProvider) Name() string { return "test" }
func (*recordReadCSTestProvider) Scan(_ context.Context, _ extcs.ScanRequest) (*extcs.Alert, error) {
	return &extcs.Alert{Provider: "test", MatchedRules: []string{"r1"}}, nil
}

func TestRecordReadNDJSONSafetyBlockDoesNotPublishFiles(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "block")
	extcs.Register(&recordReadCSTestProvider{})
	defer extcs.Register(nil)
	dir := t.TempDir()
	withBaseWorkingDir(t, dir)
	factory, stdout, registry := newExecuteFactory(t)
	registry.Register(&httpmock.Stub{Method: "GET", URL: "limit=1&offset=0", Body: map[string]any{"code": 0, "data": recordMatrixPage(0, 1, false, "fld_name")}})
	err := runShortcut(t, BaseRecordList, []string{"+record-list", "--base-token", "app_x", "--table-id", "tbl_x", "--limit", "1"}, factory, stdout)
	var safetyErr *errs.ContentSafetyError
	if !errors.As(err, &safetyErr) || len(safetyErr.Rules) != 1 || safetyErr.Rules[0] != "r1" {
		t.Fatalf("err=%v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("blocked export published files: %v", files)
	}
}
