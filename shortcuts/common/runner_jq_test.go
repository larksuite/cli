// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// newJqTestContext creates a RuntimeContext wired for jq testing.
func newJqTestContext(jqExpr, format string) (*RuntimeContext, *bytes.Buffer, *bytes.Buffer) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("jq", "", "")
	cmd.Flags().String("format", "json", "")
	cmd.Flags().String("as", "bot", "")
	cmd.ParseFlags(nil)
	if jqExpr != "" {
		cmd.Flags().Set("jq", jqExpr)
	}
	if format != "" {
		cmd.Flags().Set("format", format)
	}

	rctx := &RuntimeContext{
		ctx:        context.Background(),
		Config:     &core.CliConfig{Brand: core.BrandFeishu},
		Cmd:        cmd,
		Format:     format,
		JqExpr:     jqExpr,
		resolvedAs: core.AsBot,
		Factory: &cmdutil.Factory{
			IOStreams: &cmdutil.IOStreams{Out: stdout, ErrOut: stderr},
		},
	}
	return rctx, stdout, stderr
}

func TestRuntimeContext_Out_WithJq(t *testing.T) {
	rctx, stdout, _ := newJqTestContext(".data.name", "")

	rctx.Out(map[string]interface{}{
		"name": "Alice",
		"age":  30,
	}, nil)

	out := stdout.String()
	if !strings.Contains(out, "Alice") {
		t.Errorf("expected jq-filtered 'Alice', got: %s", out)
	}
	if strings.Contains(out, "age") {
		t.Errorf("expected jq to filter out 'age', got: %s", out)
	}
}

func TestRuntimeContext_Out_WithJq_Identity(t *testing.T) {
	rctx, stdout, _ := newJqTestContext(".ok", "")

	rctx.Out(map[string]interface{}{"key": "value"}, nil)

	out := strings.TrimSpace(stdout.String())
	if out != "true" {
		t.Errorf("expected 'true' for .ok, got: %s", out)
	}
}

func TestRuntimeContext_OutFormat_WithJq_OverridesFormat(t *testing.T) {
	rctx, stdout, _ := newJqTestContext(".data.items", "pretty")

	items := []interface{}{"a", "b", "c"}
	rctx.OutFormat(map[string]interface{}{
		"items": items,
	}, nil, func(w io.Writer) {
		t.Error("prettyFn should not be called when jq is set")
	})

	out := stdout.String()
	if !strings.Contains(out, "a") || !strings.Contains(out, "b") {
		t.Errorf("expected jq-filtered items, got: %s", out)
	}
}

func TestRuntimeContext_Out_WithJq_InvalidExpr_WritesStderr(t *testing.T) {
	rctx, _, stderr := newJqTestContext(".foo | invalid_func_xyz", "")

	rctx.Out(map[string]interface{}{"foo": "bar"}, nil)

	if !strings.Contains(stderr.String(), "error") {
		t.Errorf("expected error on stderr for runtime jq error, got: %s", stderr.String())
	}
}

type testResolvedFileIO struct{}

func (testResolvedFileIO) Open(string) (fileio.File, error)        { return nil, nil }
func (testResolvedFileIO) Stat(string) (fileio.FileInfo, error)    { return nil, nil }
func (testResolvedFileIO) ResolvePath(path string) (string, error) { return path, nil }
func (testResolvedFileIO) Save(string, fileio.SaveOptions, io.Reader) (fileio.SaveResult, error) {
	return nil, nil
}

type capturingFileIOProvider struct {
	gotCtx context.Context
	fileIO fileio.FileIO
}

func (p *capturingFileIOProvider) Name() string { return "capture" }

func (p *capturingFileIOProvider) ResolveFileIO(ctx context.Context) fileio.FileIO {
	p.gotCtx = ctx
	return p.fileIO
}

func TestRuntimeContext_FileIO_UsesExecutionContext(t *testing.T) {
	execCtx := context.WithValue(context.Background(), "key", "value")
	resolved := testResolvedFileIO{}
	provider := &capturingFileIOProvider{fileIO: resolved}

	rctx := &RuntimeContext{
		ctx: execCtx,
		Factory: &cmdutil.Factory{
			FileIOProvider: provider,
		},
	}

	got := rctx.FileIO()
	if got != resolved {
		t.Fatalf("FileIO() returned %T, want %T", got, resolved)
	}
	if provider.gotCtx != execCtx {
		t.Fatal("ResolveFileIO() did not receive the runtime execution context")
	}
}

func newTestShortcutCmd(s *Shortcut, f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{Use: "test-shortcut"}
	cmd.SetContext(context.Background())
	registerShortcutFlags(cmd, f, s)
	return cmd
}

func newTestFactory() *cmdutil.Factory {
	return &cmdutil.Factory{
		Config: func() (*core.CliConfig, error) {
			return &core.CliConfig{
				AppID: "test", AppSecret: "test", Brand: core.BrandFeishu,
			}, nil
		},
		LarkClient: func() (*lark.Client, error) {
			return lark.NewClient("test", "test"), nil
		},
		IOStreams:      &cmdutil.IOStreams{Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}},
		FileIOProvider: fileio.GetProvider(),
	}
}

func TestRunShortcut_JqAndFormatConflict(t *testing.T) {
	s := &Shortcut{
		Service:   "test",
		Command:   "test-shortcut",
		AuthTypes: []string{"bot"},
		HasFormat: true,
		Execute: func(ctx context.Context, rctx *RuntimeContext) error {
			return nil
		},
	}
	cmd := newTestShortcutCmd(s, newTestFactory())
	cmd.Flags().Set("jq", ".data")
	cmd.Flags().Set("format", "table")
	cmd.Flags().Set("as", "bot")

	err := runShortcut(cmd, newTestFactory(), s, true)
	if err == nil {
		t.Fatal("expected error for --jq + --format table conflict")
	}
	requireValidation(t, err, "mutually exclusive")
}

func TestRunShortcut_JqInvalidExpression(t *testing.T) {
	s := &Shortcut{
		Service:   "test",
		Command:   "test-shortcut",
		AuthTypes: []string{"bot"},
		Execute: func(ctx context.Context, rctx *RuntimeContext) error {
			return nil
		},
	}
	cmd := newTestShortcutCmd(s, newTestFactory())
	cmd.Flags().Set("jq", "invalid[")
	cmd.Flags().Set("as", "bot")

	err := runShortcut(cmd, newTestFactory(), s, true)
	if err == nil {
		t.Fatal("expected error for invalid jq expression")
	}
	requireValidation(t, err, "invalid jq expression")
}

func TestRunShortcut_JqRuntimeError_PropagatesError(t *testing.T) {
	s := &Shortcut{
		Service:   "test",
		Command:   "test-shortcut",
		AuthTypes: []string{"bot"},
		Execute: func(ctx context.Context, rctx *RuntimeContext) error {
			rctx.Out(map[string]interface{}{"foo": "bar"}, nil)
			return nil
		},
	}
	cmd := newTestShortcutCmd(s, newTestFactory())
	cmd.Flags().Set("jq", ".foo | invalid_func_xyz")
	cmd.Flags().Set("as", "bot")

	err := runShortcut(cmd, newTestFactory(), s, true)
	if err == nil {
		t.Fatal("expected error from jq runtime failure to propagate")
	}
}

func TestRunShortcut_DryRunJSONUsesEnvelope(t *testing.T) {
	s := &Shortcut{
		Service:   "test",
		Command:   "test-shortcut",
		AuthTypes: []string{"bot"},
		DryRun: func(ctx context.Context, rctx *RuntimeContext) *cmdutil.DryRunAPI {
			return cmdutil.NewDryRunAPI().GET("/open-apis/test")
		},
		Execute: func(ctx context.Context, rctx *RuntimeContext) error {
			t.Fatal("Execute should not run in dry-run")
			return nil
		},
	}
	f := newTestFactory()
	cmd := newTestShortcutCmd(s, f)
	cmd.Flags().Set("dry-run", "true")
	cmd.Flags().Set("as", "bot")

	if err := runShortcut(cmd, f, s, false); err != nil {
		t.Fatalf("runShortcut() error = %v", err)
	}
	stdout := f.IOStreams.Out.(*bytes.Buffer)
	var env map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("dry-run stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if env["ok"] != true || env["identity"] != "bot" || env["dry_run"] != true {
		t.Fatalf("unexpected dry-run envelope: %#v", env)
	}
	data := env["data"].(map[string]interface{})
	api := data["api"].([]interface{})
	call := api[0].(map[string]interface{})
	if call["url"] != "/open-apis/test" {
		t.Fatalf("api[0] = %#v", call)
	}
	dctx, ok := data["context"].(map[string]interface{})
	if !ok || dctx["app_id"] != "test" {
		t.Fatalf("runner must inject data.context like the service/api paths, got: %#v", data["context"])
	}
}

func TestRunShortcut_DryRunWithJq(t *testing.T) {
	s := &Shortcut{
		Service:   "test",
		Command:   "test-shortcut",
		AuthTypes: []string{"bot"},
		DryRun: func(ctx context.Context, rctx *RuntimeContext) *cmdutil.DryRunAPI {
			return cmdutil.NewDryRunAPI().GET("/open-apis/test")
		},
		Execute: func(ctx context.Context, rctx *RuntimeContext) error {
			t.Fatal("Execute should not run in dry-run")
			return nil
		},
	}
	f := newTestFactory()
	cmd := newTestShortcutCmd(s, f)
	cmd.Flags().Set("dry-run", "true")
	cmd.Flags().Set("jq", ".dry_run")
	cmd.Flags().Set("as", "bot")

	if err := runShortcut(cmd, f, s, false); err != nil {
		t.Fatalf("runShortcut() error = %v", err)
	}
	stdout := f.IOStreams.Out.(*bytes.Buffer)
	if got := strings.TrimSpace(stdout.String()); got != "true" {
		t.Fatalf("jq output = %q, want true", got)
	}
}

func TestRuntimeContext_Out_WithoutJq_NormalOutput(t *testing.T) {
	rctx, stdout, _ := newJqTestContext("", "")

	rctx.Out(map[string]interface{}{"key": "value"}, &output.Meta{Count: 1})

	out := stdout.String()
	if !strings.Contains(out, `"ok"`) || !strings.Contains(out, `"key"`) {
		t.Errorf("expected normal JSON envelope, got: %s", out)
	}
}
