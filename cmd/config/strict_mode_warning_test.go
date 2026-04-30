// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

// runStrictMode is a small helper that runs `config strict-mode <args...>` and
// returns the captured stderr — that's where success-path messages and the
// new user-identity warning land.
func runStrictMode(t *testing.T, args ...string) string {
	t.Helper()
	f, _, stderr, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test-app", AppSecret: "secret"})
	cmd := NewCmdConfigStrictMode(f)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("strict-mode %v failed: %v", args, err)
	}
	return stderr.String()
}

// expandsUserIdentity covers the only two transitions where AI gains the
// ability to act under the user's identity, and asserts the warning fires.
// Reuses bind_messages.go's IdentityEscalationMessage as the canonical text
// so all three call sites (bind upgrade, fresh user-default bind, strict-mode
// relax) stay phrased identically.
func TestStrictMode_BotToUser_WarnsAboutIdentityRisk(t *testing.T) {
	setupStrictModeTestConfig(t)
	runStrictMode(t, "bot")

	out := runStrictMode(t, "user")
	if !strings.Contains(out, bindMsgZh.IdentityEscalationMessage) {
		t.Errorf("bot→user transition must surface IdentityEscalationMessage; got: %s", out)
	}
}

func TestStrictMode_BotToOff_WarnsAboutIdentityRisk(t *testing.T) {
	setupStrictModeTestConfig(t)
	runStrictMode(t, "bot")

	out := runStrictMode(t, "off")
	if !strings.Contains(out, bindMsgZh.IdentityEscalationMessage) {
		t.Errorf("bot→off transition must surface IdentityEscalationMessage; got: %s", out)
	}
}

// narrowingDoesNotWarn covers the cases that revoke or keep user-identity
// scope — those should stay quiet, otherwise AI will spam users with risk
// text on every restrictive change.
func TestStrictMode_UserToBot_NoWarning(t *testing.T) {
	setupStrictModeTestConfig(t)
	runStrictMode(t, "user")

	out := runStrictMode(t, "bot")
	if strings.Contains(out, bindMsgZh.IdentityEscalationMessage) {
		t.Errorf("user→bot is a narrowing change; must not warn. got: %s", out)
	}
}

func TestStrictMode_OffToBot_NoWarning(t *testing.T) {
	setupStrictModeTestConfig(t)
	// Default starts at off; explicitly set bot — narrowing.
	out := runStrictMode(t, "bot")
	if strings.Contains(out, bindMsgZh.IdentityEscalationMessage) {
		t.Errorf("off→bot is a narrowing change; must not warn. got: %s", out)
	}
}

func TestStrictMode_OffToUser_NoWarning(t *testing.T) {
	// Off already permits user-identity, so off→user is not a NEW grant
	// even though it forces user identity. Don't warn.
	setupStrictModeTestConfig(t)
	out := runStrictMode(t, "user")
	if strings.Contains(out, bindMsgZh.IdentityEscalationMessage) {
		t.Errorf("off→user does not newly permit user identity; must not warn. got: %s", out)
	}
}

// --- --global path: comparison must use multi.StrictMode, not profile's
// effective mode. The previous (buggy) version used resolveStrictModeStatus
// here too, leading to both false positives (current profile has explicit
// override unaffected by --global → still warned) and false negatives
// (current profile has explicit override that masks an actual bot → off
// global broadening for OTHER inheriting profiles → didn't warn).

func TestStrictMode_GlobalBotToUser_Warns(t *testing.T) {
	setupStrictModeTestConfig(t)
	runStrictMode(t, "bot", "--global")

	out := runStrictMode(t, "user", "--global")
	if !strings.Contains(out, bindMsgZh.IdentityEscalationMessage) {
		t.Errorf("global bot→user must warn (broadens user-identity for inheriting profiles); got: %s", out)
	}
}

func TestStrictMode_GlobalBotToOff_Warns(t *testing.T) {
	setupStrictModeTestConfig(t)
	runStrictMode(t, "bot", "--global")

	out := runStrictMode(t, "off", "--global")
	if !strings.Contains(out, bindMsgZh.IdentityEscalationMessage) {
		t.Errorf("global bot→off must warn (newly permits user identity in inheriting profiles); got: %s", out)
	}
}

// FalsePositive: current profile has explicit "bot" override, global goes
// off → user. The current profile is unaffected (still bot via override),
// and off→user at the global level is not a new grant either. Must not warn.
func TestStrictMode_GlobalOffToUser_WithProfileBotOverride_NoWarning(t *testing.T) {
	setupStrictModeTestConfig(t)
	runStrictMode(t, "bot")             // profile-level explicit bot
	runStrictMode(t, "off", "--global") // global = off

	out := runStrictMode(t, "user", "--global")
	if strings.Contains(out, bindMsgZh.IdentityEscalationMessage) {
		t.Errorf("global off→user with profile-bot-override must not warn (profile unaffected, global wasn't bot); got: %s", out)
	}
}

// FalseNegative: global = bot, current profile has explicit "off" override.
// Running --global off broadens OTHER inheriting profiles (bot → off). The
// current profile doesn't change effective mode, but the policy still expanded
// user-identity, so warning must fire. The pre-fix logic compared via the
// current profile's effective mode and missed this case.
func TestStrictMode_GlobalBotToOff_WithProfileOffOverride_Warns(t *testing.T) {
	setupStrictModeTestConfig(t)
	runStrictMode(t, "bot", "--global") // global = bot
	runStrictMode(t, "off")             // profile-level explicit off (already shows the warning at profile scope)

	out := runStrictMode(t, "off", "--global")
	if !strings.Contains(out, bindMsgZh.IdentityEscalationMessage) {
		t.Errorf("global bot→off must warn even when current profile has explicit off (other profiles inherit and newly permit user identity); got: %s", out)
	}
}
