// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// TestOutPartialFailure pins the batch / multi-status contract: stdout honors
// the selected format and still carries the full payload, while the returned
// error is the typed partial-failure exit signal.
func TestOutPartialFailure(t *testing.T) {
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
	f, stdout, _, _ := cmdutil.TestFactory(t, cfg)
	rt := TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+push"}, cfg, f, core.AsUser)
	rt.Format = "table"

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

	// 2) table output contains both successful and failed outcomes and does not
	// silently switch to a JSON envelope.
	got := stdout.String()
	for _, want := range []string{"a.txt", "uploaded", "b.txt", "failed", "boom"} {
		if !strings.Contains(got, want) {
			t.Fatalf("stdout missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"ok"`) {
		t.Fatalf("table output contains JSON envelope:\n%s", got)
	}
}
