// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !extended

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/vfs"
)

func TestStandardDoctorReportsEditionSentinelWithoutLocalProfile(t *testing.T) {
	clearWorkspaceSignals(t)
	clearCredentialSignals(t)
	configDir := t.TempDir()
	systemPath := filepath.Join(configDir, "external-credential.json")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)
	t.Setenv(envvars.CliExternalCredentialConfig, systemPath)
	t.Setenv("LARKSUITE_CLI_REMOTE_META", "off")
	t.Setenv("LARKSUITE_CLI_NO_UPDATE_NOTIFIER", "1")
	t.Setenv("LARKSUITE_CLI_NO_SKILLS_NOTIFIER", "1")
	if err := vfs.WriteFile(systemPath, []byte("sentinel-only"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	root := Build(
		context.Background(),
		cmdutil.InvocationContext{},
		WithIO(strings.NewReader(""), &stdout, &stderr),
		WithoutPlugins(),
		WithoutServiceCommands(),
	)
	root.SetArgs([]string{"doctor", "--offline"})
	if err := root.ExecuteContext(context.Background()); err == nil {
		t.Fatal("doctor returned nil, want failed diagnostic result")
	}

	var report struct {
		Checks []struct {
			Name    string `json:"name"`
			Status  string `json:"status"`
			Message string `json:"message"`
			Hint    string `json:"hint"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode doctor output: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	for _, check := range report.Checks {
		if check.Name != "credential_source" {
			continue
		}
		if check.Status != "fail" ||
			check.Message != "system external credential configuration requires the lark-cli Extended edition" ||
			!strings.Contains(check.Hint, "install lark-cli Extended") {
			t.Fatalf("credential_source check = %#v", check)
		}
		if strings.Contains(stdout.String(), "config init") {
			t.Fatalf("doctor suggested local credential bootstrap for an edition sentinel: %s", stdout.String())
		}
		return
	}
	t.Fatalf("doctor did not report the edition sentinel: %s", stdout.String())
}
