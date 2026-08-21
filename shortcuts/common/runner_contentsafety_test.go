// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	extcs "github.com/larksuite/cli/extension/contentsafety"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/imcontract"
	"github.com/larksuite/cli/internal/output"
)

type csTestProvider struct {
	alert *extcs.Alert
}

func (p *csTestProvider) Name() string { return "test" }
func (p *csTestProvider) Scan(_ context.Context, _ extcs.ScanRequest) (*extcs.Alert, error) {
	return p.alert, nil
}

func newCSTestContext(t *testing.T) (*RuntimeContext, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	parentCmd := &cobra.Command{Use: "lark-cli"}
	cmd := &cobra.Command{Use: "test"}
	parentCmd.AddCommand(cmd)
	rctx := &RuntimeContext{
		ctx:        context.Background(),
		Config:     &core.CliConfig{Brand: core.BrandFeishu},
		Cmd:        cmd,
		resolvedAs: core.AsBot,
		Factory: &cmdutil.Factory{
			IOStreams: &cmdutil.IOStreams{Out: stdout, ErrOut: stderr},
		},
	}
	return rctx, stdout, stderr
}

func TestOut_ContentSafetyWarn(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "warn")

	alert := &extcs.Alert{Provider: "test", MatchedRules: []string{"r1"}}
	extcs.Register(&csTestProvider{alert: alert})
	defer extcs.Register(nil)

	rctx, stdout, _ := newCSTestContext(t)
	rctx.Out(map[string]any{"msg": "hello"}, nil)

	var env output.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env.ContentSafetyAlert == nil {
		t.Error("expected _content_safety_alert in envelope")
	}
}

func TestOut_ContentSafetyBlock(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "block")

	alert := &extcs.Alert{Provider: "test", MatchedRules: []string{"r1"}}
	extcs.Register(&csTestProvider{alert: alert})
	defer extcs.Register(nil)

	rctx, stdout, stderr := newCSTestContext(t)
	rctx.Out(map[string]any{"msg": "hello"}, nil)

	if stdout.Len() > 0 {
		t.Error("block mode should not write data to stdout")
	}
	if rctx.outputErr == nil {
		t.Error("block mode should set outputErr")
	}
	if stderr.Len() != 0 {
		t.Fatalf("block mode stderr = %q, want empty", stderr.String())
	}
	var safetyErr *errs.ContentSafetyError
	if !errors.As(rctx.outputErr, &safetyErr) {
		t.Fatalf("block mode output error = %T, want *errs.ContentSafetyError", rctx.outputErr)
	}
	if got := output.ExitCodeOf(rctx.outputErr); got != output.ExitContentSafety {
		t.Fatalf("block mode exit code = %d, want %d", got, output.ExitContentSafety)
	}
}

func TestIMContractWriteContentSafetyBlockKeepsAllowlistedCompletion(t *testing.T) {
	const secret = "SECRET_MARKER"
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "block")

	alert := &extcs.Alert{Provider: "test", MatchedRules: []string{secret}}
	extcs.Register(&csTestProvider{alert: alert})
	defer extcs.Register(nil)

	rctx, stdout, stderr := newCSTestContext(t)
	rctx.Format = "pretty"
	contract, _ := imcontract.Lookup("im chat.moderation update")
	rctx.contractSession = imcontract.NewSession(contract)
	prettyCalled := false

	rctx.OutFormat(map[string]any{"subject": secret}, nil, func(io.Writer) {
		prettyCalled = true
	})

	if prettyCalled {
		t.Fatal("blocked pretty presentation ran after the write completed")
	}
	if output.ExitCodeOf(rctx.outputErr) != output.ExitContentSafety {
		t.Fatalf("output error = %T %v", rctx.outputErr, rctx.outputErr)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if bytes.Contains(stdout.Bytes(), []byte(secret)) || strings.Contains(rctx.outputErr.Error(), secret) {
		t.Fatalf("blocked payload or scanner detail leaked: stdout=%q err=%v", stdout.String(), rctx.outputErr)
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("fallback is not JSON: %v\n%s", err, stdout.String())
	}
	if len(env) != 3 || env["ok"] != false {
		t.Fatalf("fallback = %#v", env)
	}
	data, _ := env["data"].(map[string]any)
	completion, _ := data["completion"].(map[string]any)
	if len(data) != 1 || completion["status"] != "accepted_unverified" ||
		completion["final_state_verified"] != false || completion["retry_scope"] != "none" {
		t.Fatalf("completion = %#v", completion)
	}
	problem, _ := env["error"].(map[string]any)
	if problem["type"] != "policy" || problem["subtype"] != "content_safety" ||
		problem["message"] != "Output blocked after the IM write completed" {
		t.Fatalf("error = %#v", problem)
	}
	if _, exists := env["presentation"]; exists {
		t.Fatalf("fallback introduced presentation: %#v", env)
	}
}

func TestOut_ContentSafetyOff(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")

	rctx, stdout, _ := newCSTestContext(t)
	rctx.Out(map[string]any{"msg": "hello"}, nil)

	var env output.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.ContentSafetyAlert != nil {
		t.Error("mode=off should not produce alert")
	}
}
