// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
)

// runHermesBindWithIdentity boots a Hermes-shaped fake env, runs `config bind`
// with the given identity preset in flag (non-TUI) mode, and returns captured
// stderr. Hermes is the simplest source to fake (single .env file).
func runHermesBindWithIdentity(t *testing.T, identity string) string {
	t.Helper()
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	hermesHome := t.TempDir()
	t.Setenv("HERMES_HOME", hermesHome)
	envContent := "FEISHU_APP_ID=cli_hermes_abc\nFEISHU_APP_SECRET=hermes_secret_123\nFEISHU_DOMAIN=lark\n"
	if err := os.WriteFile(filepath.Join(hermesHome, ".env"), []byte(envContent), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	f, _, stderr, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{
		Factory:  f,
		Source:   "hermes",
		Identity: identity,
		Lang:     "zh",
	})
	if err != nil {
		t.Fatalf("bind failed: %v", err)
	}
	return stderr.String()
}

// TestConfigBindRun_UserDefaultIdentity_WarnsAboutImpersonation covers the
// gap that previously slipped through: a fresh flag-mode bind landing on
// user-default. warnIdentityEscalation requires a previous bot lock to fire,
// and IdentityUserDefaultDesc only renders in TUI selection — so without
// noticeUserDefaultRisk the user/AI never see the impersonation risk on a
// first-time user-default bind.
func TestConfigBindRun_UserDefaultIdentity_WarnsAboutImpersonation(t *testing.T) {
	out := runHermesBindWithIdentity(t, "user-default")
	if !strings.Contains(out, bindMsgZh.IdentityEscalationMessage) {
		t.Errorf("user-default bind must surface IdentityEscalationMessage; got: %s", out)
	}
}

func TestConfigBindRun_BotOnlyIdentity_NoImpersonationWarning(t *testing.T) {
	out := runHermesBindWithIdentity(t, "bot-only")
	if strings.Contains(out, bindMsgZh.IdentityEscalationMessage) {
		t.Errorf("bot-only bind must NOT warn about impersonation; got: %s", out)
	}
}
