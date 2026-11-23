// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

// A dry run in a degraded environment (the test factory has no reachable
// platform, so every weak read-only check comes back unanswered) still exits
// zero with a structured decision that honestly says "unknown" — and performs
// none of its declared write effects.
func TestDryRun_DegradedEnvironmentStaysHonestAndSideEffectFree(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "cli_test"})
	snap := compileCatalog()

	tmp := t.TempDir()
	prevWD, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevWD) })

	cmd := NewCmdConsume(f, snap)
	cmd.SetArgs([]string{"im.message.receive_v1", "--as", "bot", "--dry-run", "--output-dir", "events-out"})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	if err := cmd.Execute(); err != nil {
		t.Fatalf("dry-run must not fail on unusable credentials, got: %v", err)
	}

	var envelope struct {
		OK     bool `json:"ok"`
		DryRun bool `json:"dry_run"`
		Data   struct {
			Decision struct {
				Status        string `json:"status"`
				Preconditions []struct {
					Name   string `json:"name"`
					Status string `json:"status"`
				} `json:"preconditions"`
				WouldWrite []string `json:"would_write"`
			} `json:"decision"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("stdout is not a decision envelope: %v\n%s", err, stdout.String())
	}
	if !envelope.OK || !envelope.DryRun {
		t.Errorf("want ok=true dry_run=true, got: %s", stdout.String())
	}
	if envelope.Data.Decision.Status != "unknown" {
		t.Errorf("unanswerable weak checks must render unknown, not fake readiness; got status %q", envelope.Data.Decision.Status)
	}
	names := map[string]string{}
	for _, p := range envelope.Data.Decision.Preconditions {
		names[p.Name] = p.Status
	}
	if names["credentials_available"] == "" || names["console_event_published"] == "" || names["scopes_granted"] == "" {
		t.Errorf("preconditions must name every check, got: %v", names)
	}

	// The declared write side effects must stay declarations: the requested
	// output dir must not exist after a dry run.
	if _, err := os.Stat(filepath.Join(tmp, "events-out")); !os.IsNotExist(err) {
		t.Error("dry-run created the output directory; the preview performed a side effect")
	}
}
