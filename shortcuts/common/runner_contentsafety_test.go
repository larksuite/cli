// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"

	extcs "github.com/larksuite/cli/extension/contentsafety"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
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

	rctx, stdout, _ := newCSTestContext(t)
	rctx.Out(map[string]any{"msg": "hello"}, nil)

	if stdout.Len() > 0 {
		t.Error("block mode should not write data to stdout")
	}
	if rctx.outputErr == nil {
		t.Error("block mode should set outputErr")
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
