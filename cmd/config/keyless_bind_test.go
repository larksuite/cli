// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/i18n"
	"github.com/larksuite/cli/internal/keysigner"
)

func TestConfigBindRun_OpenClawKeylessWritesProviderWithoutPath(t *testing.T) {
	saveWorkspace(t)
	clearAgentEnv(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	writeOpenClawKeylessConfig(t, "cli_keyless", "openclaw-lark")

	var gotProvider, gotKeyRef, gotClientID string
	var providerCommits int
	var providerCommitSawWorkspace bool
	replaceBindProbe(t, func(_ context.Context, _ *http.Client, _ core.LarkBrand, clientID string, _ keysigner.Signer, provider, keyRef string) (string, func() error, error) {
		gotClientID, gotProvider, gotKeyRef = clientID, provider, keyRef
		return "tat-ok", func() error {
			providerCommits++
			data, err := os.ReadFile(core.GetConfigPath())
			if err != nil {
				return err
			}
			providerCommitSawWorkspace = strings.Contains(string(data), "cli_keyless")
			return nil
		}, nil
	})

	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	if err := configBindRun(&BindOptions{Factory: f, Source: "openclaw", Identity: "bot-only"}); err != nil {
		t.Fatalf("configBindRun: %v", err)
	}
	if gotClientID != "cli_keyless" || gotProvider != core.KeylessProviderLarkSuite || gotKeyRef != "openclaw-lark" {
		t.Fatalf("probe route = client %q provider %q keyRef %q", gotClientID, gotProvider, gotKeyRef)
	}
	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatal(err)
	}
	app := multi.CurrentAppConfig("")
	if app == nil || app.AuthMethod != core.AuthMethodPrivateKeyJWT || app.KeyRef == nil ||
		app.KeyRef.Provider != core.KeylessProviderLarkSuite || app.KeyRef.ID != "openclaw-lark" || !app.AppSecret.IsZero() {
		t.Fatalf("persisted app = %#v", app)
	}
	if stdout.Len() == 0 {
		t.Fatal("bind did not emit success envelope")
	}
	if providerCommits != 1 {
		t.Fatalf("provider manifest commits = %d, want 1", providerCommits)
	}
	if !providerCommitSawWorkspace {
		t.Fatal("provider manifest committed before the workspace config became readable")
	}
}

func TestConfigBindRun_OpenClawOptionalSignerClosedLoop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a POSIX shebang; Windows resolution is compile-checked separately")
	}
	saveWorkspace(t)
	clearAgentEnv(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	signerPath := installOpenClawOptionalSigner(t)
	writeOpenClawKeylessConfig(t, "cli_keyless_optional", "openclaw-lark")

	f, _, _, registry := cmdutil.TestFactory(t, nil)
	registry.Register(&httpmock.Stub{
		Method: http.MethodPost,
		URL:    auth.PathOAuthTokenV2,
		Body:   map[string]any{"code": 0, "access_token": "tat-from-optional-signer"},
		BodyFilter: func(body []byte) bool {
			form, err := url.ParseQuery(string(body))
			return err == nil &&
				form.Get("client_id") == "cli_keyless_optional" &&
				form.Get("client_assertion_type") == "urn:ietf:params:oauth:client-assertion-type:jwt-bearer" &&
				form.Get("client_assertion") == "optional.jwt" &&
				!form.Has("client_secret")
		},
	})

	if err := configBindRun(&BindOptions{Factory: f, Source: "openclaw", Identity: "bot-only"}); err != nil {
		t.Fatalf("configBindRun: %v", err)
	}
	data, err := os.ReadFile(core.GetConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(data), "\n") || !strings.Contains(string(data), "\n  \"apps\": [") {
		t.Fatalf("config is not formatted JSON with a trailing newline:\n%s", data)
	}
	if strings.Contains(string(data), signerPath) {
		t.Fatalf("config persisted the discovered signer executable path:\n%s", data)
	}
	providerData, err := os.ReadFile(filepath.Join(core.GetBaseConfigDir(), "signing-providers.json"))
	if err != nil {
		t.Fatalf("read global signer manifest: %v", err)
	}
	if !strings.Contains(string(providerData), signerPath) {
		t.Fatalf("global signer manifest did not record the verified executable")
	}
	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatal(err)
	}
	app := multi.CurrentAppConfig("")
	if app == nil || app.AppId != "cli_keyless_optional" || app.KeyRef == nil ||
		app.KeyRef.Provider != core.KeylessProviderLarkSuite || app.KeyRef.ID != "openclaw-lark" || !app.AppSecret.IsZero() {
		t.Fatalf("resolved config = %#v", app)
	}
}

func TestConfigBindRun_OpenClawKeylessProbeFailureDoesNotWrite(t *testing.T) {
	saveWorkspace(t)
	clearAgentEnv(t)
	base := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", base)
	writeOpenClawKeylessConfig(t, "cli_wrong_key", "openclaw-lark")
	replaceBindProbe(t, func(context.Context, *http.Client, core.LarkBrand, string, keysigner.Signer, string, string) (string, func() error, error) {
		return "", nil, errs.NewConfigError(errs.SubtypeInvalidClient, "public key is not bound")
	})

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	if err := configBindRun(&BindOptions{Factory: f, Source: "openclaw", Identity: "bot-only"}); err == nil {
		t.Fatal("expected probe error")
	}
	if _, err := os.Stat(filepath.Join(base, "openclaw", "config.json")); !os.IsNotExist(err) {
		t.Fatalf("config must not be written; stat error = %v", err)
	}
}

func TestConfigBindRun_OpenClawKeylessMissingProviderCommitFailsClosed(t *testing.T) {
	saveWorkspace(t)
	clearAgentEnv(t)
	base := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", base)
	writeOpenClawKeylessConfig(t, "cli_missing_commit", "openclaw-lark")
	replaceBindProbe(t, func(context.Context, *http.Client, core.LarkBrand, string, keysigner.Signer, string, string) (string, func() error, error) {
		return "tat-ok", nil, nil
	})

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "openclaw", Identity: "bot-only"})
	if err == nil || !strings.Contains(err.Error(), "did not produce a provider manifest commit") {
		t.Fatalf("configBindRun error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(base, "openclaw", "config.json")); !os.IsNotExist(statErr) {
		t.Fatalf("config must not be written; stat error = %v", statErr)
	}
}

func TestCommitBinding_ProviderManifestFailureRestoresWorkspace(t *testing.T) {
	saveWorkspace(t)
	base := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", base)
	core.SetCurrentWorkspace(core.WorkspaceOpenClaw)
	configPath := core.GetConfigPath()
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		t.Fatal(err)
	}
	previous := []byte("{\n  \"current_app\": \"old\",\n  \"apps\": [{\"name\": \"old\", \"app_id\": \"cli_old\", \"app_secret\": \"keep\"}]\n}\n")
	if err := os.WriteFile(configPath, previous, 0600); err != nil {
		t.Fatal(err)
	}

	f, stdout, stderr, _ := cmdutil.TestFactory(t, nil)
	commitCalls := 0
	result := &BindResult{
		AppConfig: &core.AppConfig{AppId: "cli_new", Brand: core.BrandFeishu},
		commitProviderManifest: func() error {
			commitCalls++
			return errors.New("manifest write failed")
		},
	}
	err := commitBinding(&BindOptions{Factory: f, Identity: "bot-only"}, result, previous, "openclaw", configPath)
	if err == nil || !strings.Contains(err.Error(), "workspace config restored") {
		t.Fatalf("commitBinding error = %v", err)
	}
	if commitCalls != 1 {
		t.Fatalf("provider manifest commits = %d, want 1", commitCalls)
	}
	got, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(previous) {
		t.Fatalf("workspace was not restored:\n%s", got)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("failed bind emitted success output: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestCommitBinding_ProviderManifestFailureRemovesNewWorkspace(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	core.SetCurrentWorkspace(core.WorkspaceOpenClaw)
	configPath := core.GetConfigPath()
	f, stdout, stderr, _ := cmdutil.TestFactory(t, nil)
	result := &BindResult{
		AppConfig: &core.AppConfig{AppId: "cli_new", Brand: core.BrandFeishu},
		commitProviderManifest: func() error {
			return errors.New("manifest write failed")
		},
	}
	err := commitBinding(&BindOptions{Factory: f, Identity: "bot-only"}, result, nil, "openclaw", configPath)
	if err == nil || !strings.Contains(err.Error(), "workspace config restored") {
		t.Fatalf("commitBinding error = %v", err)
	}
	if _, statErr := os.Stat(configPath); !os.IsNotExist(statErr) {
		t.Fatalf("new workspace config was not removed; stat error = %v", statErr)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("failed bind emitted success output: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestCommitBinding_WorkspaceWriteFailureDoesNotCommitProvider(t *testing.T) {
	saveWorkspace(t)
	base := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", base)
	core.SetCurrentWorkspace(core.WorkspaceOpenClaw)
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	providerCommitted := false
	result := &BindResult{
		AppConfig: &core.AppConfig{AppId: "cli_new", Brand: core.BrandFeishu},
		commitProviderManifest: func() error {
			providerCommitted = true
			return nil
		},
	}
	configPath := filepath.Join(base, "missing-parent", "config.json")
	if err := commitBinding(&BindOptions{Factory: f, Identity: "bot-only"}, result, nil, "openclaw", configPath); err == nil {
		t.Fatal("expected workspace write failure")
	}
	if providerCommitted {
		t.Fatal("provider manifest was committed before the workspace config write succeeded")
	}
}

func TestCommitBinding_ConcurrentWorkspaceChangeFailsBeforeProviderCommit(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	core.SetCurrentWorkspace(core.WorkspaceOpenClaw)
	configPath := core.GetConfigPath()
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		t.Fatal(err)
	}
	previous := []byte(`{"apps":[{"app_id":"cli_old","app_secret":"old"}]}`)
	concurrent := []byte(`{"apps":[{"app_id":"cli_other","app_secret":"newer"}]}`)
	if err := os.WriteFile(configPath, concurrent, 0600); err != nil {
		t.Fatal(err)
	}
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	providerCommitted := false
	result := &BindResult{
		AppConfig: &core.AppConfig{AppId: "cli_new", Brand: core.BrandFeishu},
		commitProviderManifest: func() error {
			providerCommitted = true
			return nil
		},
	}
	err := commitBinding(&BindOptions{Factory: f, Identity: "bot-only"}, result, previous, "openclaw", configPath)
	if err == nil || !strings.Contains(err.Error(), "changed while the bind was being validated") {
		t.Fatalf("commitBinding error = %v", err)
	}
	if providerCommitted {
		t.Fatal("provider manifest was committed after a concurrent workspace change")
	}
	got, readErr := os.ReadFile(configPath)
	if readErr != nil || string(got) != string(concurrent) {
		t.Fatalf("concurrent workspace was overwritten: %q, %v", got, readErr)
	}
}

func TestCommitBinding_ProviderFailureDoesNotOverwriteConcurrentWriter(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	core.SetCurrentWorkspace(core.WorkspaceOpenClaw)
	configPath := core.GetConfigPath()
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		t.Fatal(err)
	}
	previous := []byte(`{"apps":[{"app_id":"cli_old","app_secret":"old"}]}`)
	concurrent := []byte(`{"apps":[{"app_id":"cli_other","app_secret":"newer"}]}`)
	if err := os.WriteFile(configPath, previous, 0600); err != nil {
		t.Fatal(err)
	}
	f, stdout, stderr, _ := cmdutil.TestFactory(t, nil)
	result := &BindResult{
		AppConfig: &core.AppConfig{AppId: "cli_new", Brand: core.BrandFeishu},
		commitProviderManifest: func() error {
			if err := os.WriteFile(configPath, concurrent, 0600); err != nil {
				return err
			}
			return errors.New("manifest write failed")
		},
	}
	err := commitBinding(&BindOptions{Factory: f, Identity: "bot-only"}, result, previous, "openclaw", configPath)
	if err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("commitBinding error = %v", err)
	}
	got, readErr := os.ReadFile(configPath)
	if readErr != nil || string(got) != string(concurrent) {
		t.Fatalf("concurrent workspace was overwritten: %q, %v", got, readErr)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("failed bind emitted success output: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestMergeBoundApp_UpsertsAndActivatesWithoutClobberingSiblings(t *testing.T) {
	lang := "en_us"
	previous := &core.MultiAppConfig{
		StrictMode: core.StrictModeUser,
		CurrentApp: "other",
		Apps: []core.AppConfig{
			{Name: "bound", AppId: "cli_target", Brand: core.BrandLark, Lang: coreLang(lang), Users: []core.AppUser{{UserOpenId: "ou_1", UserName: "alice"}}},
			{Name: "other", AppId: "cli_other", AppSecret: core.PlainSecret("keep"), Brand: core.BrandFeishu, Users: []core.AppUser{}},
		},
	}
	beforeSibling := previous.Apps[1]
	data := mustJSON(t, previous)
	incoming := &core.AppConfig{AppId: "cli_target", Brand: core.BrandFeishu, AuthMethod: core.AuthMethodPrivateKeyJWT,
		KeyRef: &core.SecretRef{Source: core.SecretSourceTEE, Provider: core.KeylessProviderLarkSuite, ID: "openclaw-lark"}}

	got, err := mergeBoundApp(incoming, data, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Apps) != 2 || got.CurrentApp != "bound" || got.PreviousApp != "other" || got.StrictMode != previous.StrictMode {
		t.Fatalf("merged root = %#v", got)
	}
	if !reflect.DeepEqual(got.Apps[1], beforeSibling) {
		t.Fatalf("sibling changed: got %#v want %#v", got.Apps[1], beforeSibling)
	}
	if got.Apps[0].Name != "bound" || got.Apps[0].Lang != coreLang(lang) || !reflect.DeepEqual(got.Apps[0].Users, previous.Apps[0].Users) {
		t.Fatalf("target-owned fields were lost: %#v", got.Apps[0])
	}
}

func writeOpenClawKeylessConfig(t *testing.T, appID, keyRef string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "openclaw.json")
	data := []byte(`{"channels":{"feishu":{"appId":"` + appID + `","authMethod":"private_key_jwt","keyRef":"` + keyRef + `","domain":"feishu"}}}`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENCLAW_CONFIG_PATH", path)
}

func replaceBindProbe(t *testing.T, fn func(context.Context, *http.Client, core.LarkBrand, string, keysigner.Signer, string, string) (string, func() error, error)) {
	t.Helper()
	previous := fetchTATForBind
	fetchTATForBind = fn
	t.Cleanup(func() { fetchTATForBind = previous })
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func coreLang(value string) i18n.Lang { return i18n.Lang(value) }

func installOpenClawOptionalSigner(t *testing.T) string {
	t.Helper()
	// This closure specifically exercises the no-inspect compatibility path;
	// keylessprovider tests separately cover authoritative managed-project
	// discovery from `openclaw plugins inspect`.
	t.Setenv("PATH", "")
	type signerPackage struct {
		name, npmOS, npmCPU, binary string
	}
	packages := map[string]signerPackage{
		"darwin/arm64": {"@larksuite/lark-keyless-signer-darwin-arm64", "darwin", "arm64", "lark-keyless-signer"},
		"darwin/amd64": {"@larksuite/lark-keyless-signer-darwin-x64", "darwin", "x64", "lark-keyless-signer"},
		"linux/arm64":  {"@larksuite/lark-keyless-signer-linux-arm64", "linux", "arm64", "lark-keyless-signer"},
		"linux/amd64":  {"@larksuite/lark-keyless-signer-linux-x64", "linux", "x64", "lark-keyless-signer"},
	}
	spec, ok := packages[runtime.GOOS+"/"+runtime.GOARCH]
	if !ok {
		t.Skipf("no optional signer package for %s/%s", runtime.GOOS, runtime.GOARCH)
		return ""
	}

	stateDir := filepath.Join(t.TempDir(), "openclaw state")
	packageDir := filepath.Join(
		stateDir, "extensions", "openclaw-lark", "node_modules", "@larksuite", strings.TrimPrefix(spec.name, "@larksuite/"),
	)
	binDir := filepath.Join(packageDir, "bin")
	if err := os.MkdirAll(binDir, 0700); err != nil {
		t.Fatal(err)
	}
	packageJSON, err := json.MarshalIndent(map[string]any{
		"name": spec.name, "version": "1.2.3", "os": []string{spec.npmOS}, "cpu": []string{spec.npmCPU},
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageDir, "package.json"), append(packageJSON, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n" +
		"IFS= read -r request\n" +
		"printf '%s\\n' '{\"ok\":true,\"client_assertion_type\":\"urn:ietf:params:oauth:client-assertion-type:jwt-bearer\",\"client_assertion\":\"optional.jwt\"}'\n"
	signerPath := filepath.Join(binDir, spec.binary)
	if err := os.WriteFile(signerPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENCLAW_STATE_DIR", stateDir)
	t.Setenv("PATH", "")
	return signerPath
}
