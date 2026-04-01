// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func mailTestConfig() *core.CliConfig {
	return &core.CliConfig{
		AppID:      "test-app",
		AppSecret:  "test-secret",
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_testuser",
		UserName:   "Test User",
	}
}

func mailShortcutTestFactory(t *testing.T) (*cmdutil.Factory, *bytes.Buffer, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())

	cfg := mailTestConfig()
	token := &auth.StoredUAToken{
		UserOpenId:       cfg.UserOpenId,
		AppId:            cfg.AppID,
		AccessToken:      "test-user-access-token",
		RefreshToken:     "test-refresh-token",
		ExpiresAt:        time.Now().Add(1 * time.Hour).UnixMilli(),
		RefreshExpiresAt: time.Now().Add(24 * time.Hour).UnixMilli(),
		Scope:            "mail:user_mailbox.messages:write mail:user_mailbox.messages:read mail:user_mailbox.message:modify mail:user_mailbox.message:readonly mail:user_mailbox.message.address:read mail:user_mailbox.message.subject:read mail:user_mailbox.message.body:read mail:user_mailbox:readonly",
		GrantedAt:        time.Now().Add(-1 * time.Hour).UnixMilli(),
	}
	if err := auth.SetStoredToken(token); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}
	t.Cleanup(func() {
		_ = auth.RemoveStoredToken(cfg.AppID, cfg.UserOpenId)
	})

	return cmdutil.TestFactory(t, cfg)
}

func runMountedMailShortcut(t *testing.T, shortcut common.Shortcut, args []string, f *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	parent := &cobra.Command{Use: "test"}
	shortcut.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

func decodeShortcutEnvelopeData(t *testing.T, stdout *bytes.Buffer) map[string]interface{} {
	t.Helper()
	var envelope struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("Unmarshal(stdout) error = %v, stdout=%s", err, stdout.String())
	}
	if !envelope.OK {
		t.Fatalf("expected ok output, stdout=%s", stdout.String())
	}
	return envelope.Data
}

func encodeFixtureEMLForMailTest(raw string) string {
	return base64.URLEncoding.EncodeToString([]byte(raw))
}

// chdirTemp changes the working directory to a fresh temp directory and
// restores it when the test finishes. This allows SafeInputPath/SafeOutputPath
// to accept relative file paths created in the temp directory.
func chdirTemp(t *testing.T) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
}
