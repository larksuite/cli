// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keychain"
)

// Regression: a config.json written by a newer lark-cli must surface its
// upgrade hint through the credential layer; the previous `if err != nil {
// return nil, core.NotConfiguredError() }` lost the Hint and routed AI
// agents toward `config init`, which would overwrite fields the newer
// binary populated.
func TestResolveAccount_R2ForwardSchema_PassesThroughHint(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	future := []byte(`{"schemaVersion":99,"apps":[{"appId":"cli_x","appSecret":"s","brand":"feishu","users":[]}]}` + "\n")
	if err := os.WriteFile(filepath.Join(dir, "config.json"), future, 0600); err != nil {
		t.Fatal(err)
	}

	p := NewDefaultAccountProvider(func() keychain.KeychainAccess { return stubKC{} }, "", "", "")
	_, err := p.ResolveAccount(context.Background())
	if err == nil {
		t.Fatal("expected R2 error from ResolveAccount, got nil")
	}
	var cfgErr *core.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *core.ConfigError, got %T: %v", err, err)
	}
	if !strings.Contains(cfgErr.Message, "newer lark-cli") {
		t.Errorf("R2 message lost; got %q", cfgErr.Message)
	}
	if !strings.Contains(cfgErr.Hint, "upgrade lark-cli") {
		t.Errorf("R2 upgrade hint lost; got %q", cfgErr.Hint)
	}
}
