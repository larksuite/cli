// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	extcs "github.com/larksuite/cli/extension/contentsafety"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

type recordMarkdownCSTestProvider struct {
	alert *extcs.Alert
}

func (p *recordMarkdownCSTestProvider) Name() string { return "test" }
func (p *recordMarkdownCSTestProvider) Scan(_ context.Context, _ extcs.ScanRequest) (*extcs.Alert, error) {
	return p.alert, nil
}

func newRecordMarkdownTestRuntime(stdout, stderr *bytes.Buffer) *common.RuntimeContext {
	parentCmd := &cobra.Command{Use: "lark-cli"}
	baseCmd := &cobra.Command{Use: "base"}
	cmd := &cobra.Command{Use: "+record-list"}
	cmd.Flags().String("format", "markdown", "")
	parentCmd.AddCommand(baseCmd)
	baseCmd.AddCommand(cmd)
	return &common.RuntimeContext{
		Config:  &core.CliConfig{Brand: core.BrandFeishu},
		Cmd:     cmd,
		Factory: &cmdutil.Factory{IOStreams: &cmdutil.IOStreams{Out: stdout, ErrOut: stderr}},
	}
}

func TestRenderRecordMarkdownEmptyResult(t *testing.T) {
	got, err := renderRecordMarkdown(map[string]interface{}{
		"fields":         []interface{}{"Name", "Age"},
		"record_id_list": []interface{}{},
		"data":           []interface{}{},
		"has_more":       false,
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	for _, want := range []string{
		"| _record_id | Name | Age |",
		"Meta: count=0; has_more=false",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestRenderRecordMarkdownEscapesTableCells(t *testing.T) {
	got, err := renderRecordMarkdown(map[string]interface{}{
		"fields":         []interface{}{"Name|Label", "Note"},
		"record_id_list": []interface{}{"rec_1"},
		"data":           []interface{}{[]interface{}{"A|B", "line1\nline2"}},
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	for _, want := range []string{
		"| _record_id | Name\\|Label | Note |",
		"| rec_1 | A\\|B | line1<br>line2 |",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestRenderRecordMarkdownTruncatesIgnoredFields(t *testing.T) {
	ignored := make([]interface{}, maxRecordMarkdownIgnoredFields+2)
	for i := range ignored {
		ignored[i] = fmt.Sprintf("Field%d", i+1)
	}
	got, err := renderRecordMarkdown(map[string]interface{}{
		"fields":         []interface{}{"Name"},
		"record_id_list": []interface{}{"rec_1"},
		"data":           []interface{}{[]interface{}{"Alice"}},
		"ignored_fields": ignored,
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(got, fmt.Sprintf("ignored_fields=%d", len(ignored))) ||
		!strings.Contains(got, fmt.Sprintf("...(%d total)", len(ignored))) ||
		strings.Contains(got, "Field22") {
		t.Fatalf("ignored field truncation mismatch:\n%s", got)
	}
}

func TestOutputRecordMarkdownContentSafetyWarnKeepsStdoutClean(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "warn")
	extcs.Register(&recordMarkdownCSTestProvider{
		alert: &extcs.Alert{Provider: "test", MatchedRules: []string{"r1"}},
	})
	defer extcs.Register(nil)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := outputRecordMarkdown(newRecordMarkdownTestRuntime(stdout, stderr), map[string]interface{}{
		"fields":         []interface{}{"Name"},
		"record_id_list": []interface{}{"rec_1"},
		"data":           []interface{}{[]interface{}{"Alice"}},
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "| rec_1 | Alice |") || strings.Contains(got, "content safety") {
		t.Fatalf("stdout should contain only markdown data, got:\n%s", got)
	}
	if got := stderr.String(); !strings.Contains(got, "warning: content safety alert") {
		t.Fatalf("stderr missing content safety warning:\n%s", got)
	}
}

func TestOutputRecordMarkdownContentSafetyBlockDoesNotWriteStdout(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "block")
	extcs.Register(&recordMarkdownCSTestProvider{
		alert: &extcs.Alert{Provider: "test", MatchedRules: []string{"r1"}},
	})
	defer extcs.Register(nil)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := outputRecordMarkdown(newRecordMarkdownTestRuntime(stdout, stderr), map[string]interface{}{
		"fields":         []interface{}{"Name"},
		"record_id_list": []interface{}{"rec_1"},
		"data":           []interface{}{[]interface{}{"Alice"}},
	})
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != output.ExitContentSafety {
		t.Fatalf("err=%v, want content safety exit error", err)
	}
	if stdout.Len() > 0 {
		t.Fatalf("block mode should not write stdout, got:\n%s", stdout.String())
	}
	if stderr.Len() > 0 {
		t.Fatalf("block mode should not write warning to stderr, got:\n%s", stderr.String())
	}
}

func TestOutputRecordMarkdownFallsBackToJSONWhenRenderFails(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := outputRecordMarkdown(newRecordMarkdownTestRuntime(stdout, stderr), map[string]interface{}{
		"records": map[string]interface{}{
			"schema": []interface{}{"Name"},
			"rows":   []interface{}{[]interface{}{"Alice"}},
		},
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(stdout.String(), "markdown render failed") {
		t.Fatalf("stdout should not contain fallback warning:\n%s", stdout.String())
	}
	var env output.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout should be JSON fallback, got err=%v stdout=%s", err, stdout.String())
	}
	if !env.OK || !strings.Contains(stdout.String(), `"records"`) {
		t.Fatalf("stdout missing JSON fallback data:\n%s", stdout.String())
	}
	if got := stderr.String(); !strings.Contains(got, "warning: record markdown render failed, falling back to json") {
		t.Fatalf("stderr missing fallback warning:\n%s", got)
	}
}

func TestOutputRecordMarkdownDefaultFormatAllowsJqJSONFallback(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runtime := newRecordMarkdownTestRuntime(stdout, stderr)
	runtime.JqExpr = ".data.record_id_list[0]"
	err := outputRecordMarkdown(runtime, map[string]interface{}{
		"fields":         []interface{}{"Name"},
		"record_id_list": []interface{}{"rec_1"},
		"data":           []interface{}{[]interface{}{"Alice"}},
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "rec_1" {
		t.Fatalf("stdout jq fallback mismatch: %q", got)
	}
	if stderr.Len() > 0 {
		t.Fatalf("stderr should be empty, got:\n%s", stderr.String())
	}
}

func TestOutputRecordMarkdownExplicitFormatRejectsJq(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runtime := newRecordMarkdownTestRuntime(stdout, stderr)
	runtime.JqExpr = ".data"
	if err := runtime.Cmd.Flags().Set("format", "markdown"); err != nil {
		t.Fatalf("set format: %v", err)
	}
	err := outputRecordMarkdown(runtime, map[string]interface{}{
		"fields":         []interface{}{"Name"},
		"record_id_list": []interface{}{"rec_1"},
		"data":           []interface{}{[]interface{}{"Alice"}},
	})
	if err == nil || !strings.Contains(err.Error(), "--jq and --format markdown are mutually exclusive") {
		t.Fatalf("err=%v, want jq markdown conflict", err)
	}
	if stdout.Len() > 0 {
		t.Fatalf("stdout should be empty, got:\n%s", stdout.String())
	}
}
