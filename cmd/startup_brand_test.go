// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/registry"
)

const startupBrandHelperEnv = "GO_TEST_STARTUP_BRAND_HELPER"

var _ = flag.String("startup-brand-helper", "", "internal startup brand test helper nonce")

func isStartupBrandHelper() bool {
	return startupBrandHelperEnabled(os.Getenv(startupBrandHelperEnv), startupBrandHelperNonce(os.Args))
}

func startupBrandHelperEnabled(envNonce, argNonce string) bool {
	return envNonce != "" && envNonce == argNonce
}

func startupBrandHelperNonce(args []string) string {
	const prefix = "-startup-brand-helper="
	for _, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			return strings.TrimPrefix(arg, prefix)
		}
	}
	return ""
}

func TestResolveStartupBrand_Precedence(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", tmp)
	t.Setenv("LARKSUITE_CLI_BRAND", "")
	os.Unsetenv("LARKSUITE_CLI_BRAND")

	// No config at all → default brand.
	if got := ResolveStartupBrand(""); got != core.BrandFeishu {
		t.Errorf("empty state brand = %q, want feishu", got)
	}

	// Raw config supplies the active profile's brand — no keychain involved.
	raw := `{"currentApp":"feishu-app","apps":[` +
		`{"name":"feishu-app","appId":"cli_f","appSecret":"test-secret","brand":"feishu","users":[]},` +
		`{"name":"lark-prof","appId":"cli_l","appSecret":"test-secret","brand":"LARK","users":[]}]}`
	if err := os.WriteFile(filepath.Join(tmp, "config.json"), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if got := ResolveStartupBrand(""); got != core.BrandFeishu {
		t.Errorf("default profile brand = %q, want feishu", got)
	}
	if got := ResolveStartupBrand("lark-prof"); got != core.BrandLark {
		t.Errorf("lark profile brand = %q, want lark (normalized)", got)
	}

	// Environment wins over the config file.
	t.Setenv("LARKSUITE_CLI_BRAND", "lark")
	if got := ResolveStartupBrand(""); got != core.BrandLark {
		t.Errorf("env brand = %q, want lark", got)
	}
}

// TestStartupBrandReachesRegistry_RealStartupOrder proves the fix for the
// production startup sequence: building the command tree locks the registry's
// sync.Once, so the brand must be injected before the first catalog access.
// It runs in a subprocess because the registry is process-global.
func TestStartupBrandReachesRegistry_RealStartupOrder(t *testing.T) {
	if isStartupBrandHelper() {
		// Helper: replicate Execute()'s build wiring with a lark config.
		buildInternal(
			context.Background(), cmdutil.InvocationContext{},
			WithIO(strings.NewReader(""), os.Stdout, os.Stderr),
			WithStartupBrand(ResolveStartupBrand("")),
		)
		fmt.Printf("CONFIGURED_BRAND=%s\n", registry.ConfiguredBrand())
		os.Exit(0)
	}

	tmp := t.TempDir()
	raw := `{"apps":[{"appId":"cli_l","appSecret":"test-secret","brand":"lark","users":[]}]}`
	if err := os.WriteFile(filepath.Join(tmp, "config.json"), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}

	nonce := uuid.NewString()
	t.Setenv(startupBrandHelperEnv, nonce)
	cmd := exec.Command(os.Args[0], "-test.run", "TestStartupBrandReachesRegistry_RealStartupOrder")
	cmd.Args = append(cmd.Args, "-startup-brand-helper="+nonce)
	cmd.Env = append(os.Environ(),
		"LARKSUITE_CLI_CONFIG_DIR="+tmp,
		"LARKSUITE_CLI_REMOTE_META=off", // no network during the subprocess build
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "CONFIGURED_BRAND=lark") {
		t.Errorf("registry brand after real startup order = %s, want lark", out)
	}
}

func TestStartupBrandHelperRequiresMatchingCommandNonce(t *testing.T) {
	for _, tt := range []struct {
		name     string
		envNonce string
		argNonce string
		want     bool
	}{
		{name: "neither set"},
		{name: "ambient environment only", envNonce: "ambient"},
		{name: "command argument only", argNonce: "command"},
		{name: "mismatch", envNonce: "ambient", argNonce: "command"},
		{name: "matching", envNonce: "nonce", argNonce: "nonce", want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := startupBrandHelperEnabled(tt.envNonce, tt.argNonce); got != tt.want {
				t.Fatalf("startupBrandHelperEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStartupBrandHelperNonce(t *testing.T) {
	if got := startupBrandHelperNonce([]string{"test", "-test.run", "brand"}); got != "" {
		t.Fatalf("startupBrandHelperNonce() = %q, want empty", got)
	}
	if got := startupBrandHelperNonce([]string{"test", "-startup-brand-helper=nonce"}); got != "nonce" {
		t.Fatalf("startupBrandHelperNonce() = %q, want nonce", got)
	}
}
