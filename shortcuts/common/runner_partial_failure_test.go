// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// TestOutPartialFailure pins the batch / multi-status contract: the result
// rides on stdout as an ok:false envelope (carrying the full payload), and the
// returned error is the typed partial-failure exit signal (ExitAPI), distinct
// from ErrBare (the silent-exit signal).
func TestOutPartialFailure(t *testing.T) {
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
	f, stdout, _, _ := cmdutil.TestFactory(t, cfg)
	rt := TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+push"}, cfg, f, core.AsUser)

	payload := map[string]interface{}{
		"summary": map[string]interface{}{"uploaded": 1, "failed": 1},
		"items": []map[string]interface{}{
			{"rel_path": "a.txt", "action": "uploaded"},
			{"rel_path": "b.txt", "action": "failed", "error": "boom"},
		},
	}

	err := rt.OutPartialFailure(payload, nil)

	// 1) typed partial-failure exit signal
	var pfErr *output.PartialFailureError
	if !errors.As(err, &pfErr) {
		t.Fatalf("expected *output.PartialFailureError, got %T: %v", err, err)
	}
	if pfErr.Code != output.ExitAPI {
		t.Errorf("exit code = %d, want %d (ExitAPI)", pfErr.Code, output.ExitAPI)
	}

	// 2) stdout envelope reports ok:false but still carries the full payload
	// (both the succeeded and failed items) — consistent with a success Out().
	var env struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal stdout envelope: %v\nstdout: %s", err, stdout.String())
	}
	if env.OK {
		t.Errorf("ok must be false on partial failure, got ok:true\nstdout: %s", stdout.String())
	}
	items, _ := env.Data["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("both succeeded and failed items must ride on stdout, got %d items\nstdout: %s", len(items), stdout.String())
	}
}
