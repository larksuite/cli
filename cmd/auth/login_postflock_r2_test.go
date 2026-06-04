// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
)

// Regression for the second post-flock-load wrap site:
// syncLoginUserToProfile reloads under the flock and previously
// collapsed any error into errs.NewInternalError(SubtypeStorage, ...).
// If config.json was rewritten by a newer lark-cli between the
// pre-login resolve and the post-flock reload, the operator would
// see a generic "load config" storage error instead of the
// upgrade-required hint, while their access token had ALREADY been
// written to the keychain — leaving them in a state where the next
// command would hit R2 again, only via a different code path.
//
// Pin core.PassThroughOrNotConfigured(err) here so the
// *core.ConfigError + Hint reach the dispatcher.
func TestSyncLoginUserToProfile_R2ForwardIncompat_PassesUpgradeHint(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	t.Setenv("HOME", t.TempDir())

	// Seed a future-schema config — syncLoginUserToProfile reads it
	// post-flock, so the R2 envelope must come back from THIS path.
	future := []byte(`{"schemaVersion":99,"apps":[{"appId":"cli_x","appSecret":"s","brand":"feishu","users":[]}]}` + "\n")
	if err := os.WriteFile(filepath.Join(dir, "config.json"), future, 0600); err != nil {
		t.Fatalf("seed config.json: %v", err)
	}

	root := larkauth.NewLocalRoot(core.GetConfigDir())
	var errOut bytes.Buffer
	err := syncLoginUserToProfile(
		root,
		"target", "cli_x",
		"ou_alice", "uni_alice", "Alice",
		"contact:user.email:readonly",
		time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
		&errOut,
	)
	if err == nil {
		t.Fatal("syncLoginUserToProfile must surface R2 upgrade error from a future schema, got nil")
	}
	// The OUTER envelope must be *core.ConfigError so the dispatcher's
	// PromoteConfigError step routes it to SubtypeInvalidConfig with the
	// upgrade hint. Pre-fix the outer was *errs.InternalError(SubtypeStorage)
	// — errors.As would still walk through WithCause and find the inner
	// ConfigError, so the assertion has to be on the concrete top type, not
	// on errors.As reachability.
	if _, ok := err.(*core.ConfigError); !ok {
		t.Fatalf("expected outer *core.ConfigError so dispatcher routes the R2 hint; got %T: %v", err, err)
	}
	var cfgErr *core.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *core.ConfigError preserved by PassThroughOrNotConfigured, got %T: %v", err, err)
	}
	if !strings.Contains(cfgErr.Message, "newer lark-cli") {
		t.Errorf("R2 message lost; got %q", cfgErr.Message)
	}
	if !strings.Contains(cfgErr.Hint, "upgrade lark-cli") {
		t.Errorf("R2 upgrade hint lost; got %q", cfgErr.Hint)
	}
}
