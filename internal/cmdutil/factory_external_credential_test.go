// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package cmdutil

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"

	_ "github.com/larksuite/cli/extension/credential/env"
	exttransport "github.com/larksuite/cli/extension/transport"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/externalcredential"
	"github.com/larksuite/cli/internal/i18n"
	"github.com/larksuite/cli/internal/requestcontext"
	"github.com/larksuite/cli/internal/riskcontrol"
	"github.com/larksuite/cli/internal/runtimebootstrap"
	internaltransport "github.com/larksuite/cli/internal/transport"
)

func newFactoryFromRuntimeBootstrap(streams *IOStreams, inv InvocationContext) *Factory {
	startup := runtimebootstrap.Resolve(inv.Profile)
	return NewDefaultWithRuntimePlan(streams, inv, startup.ProfileConfig, startup.Plan, nil)
}

func writePlatformProxyConfiguration(t *testing.T, config *core.MultiAppConfig, appID string) {
	t.Helper()
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)
	systemPath := configDir + "/external-credential.json"
	t.Setenv(envvars.CliExternalCredentialConfig, systemPath)
	core.SetCurrentWorkspace(core.WorkspaceLocal)
	if err := core.SaveMultiAppConfig(config); err != nil {
		t.Fatal(err)
	}
	system := externalcredential.Config{
		Version: 1, Mode: externalcredential.ModePlatformProxy,
		RemoteEndpoint: "https://credentials.example",
		Applications:   []externalcredential.Application{{Brand: core.BrandFeishu, AppID: appID}},
	}
	data, err := json.Marshal(system)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(systemPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestFactoryInstallsExternalProxyTransport(t *testing.T) {
	t.Setenv(internaltransport.EnvNoProxy, "")
	previousTransport := http.DefaultTransport
	var observed *http.Request
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		observed = req.Clone(req.Context())
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       http.NoBody,
			Request:    req,
		}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = previousTransport })

	config := &core.MultiAppConfig{Apps: []core.AppConfig{{
		Name: "sandbox", AppId: "cli_test", Brand: core.BrandFeishu, Lang: i18n.LangJaJP, Users: []core.AppUser{},
	}}}
	writePlatformProxyConfiguration(t, config, "cli_test")
	factory := newFactoryFromRuntimeBootstrap(&IOStreams{In: nil, Out: io.Discard, ErrOut: io.Discard}, InvocationContext{})
	resolved, err := factory.Config()
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Lang != i18n.LangJaJP {
		t.Fatalf("resolved Lang = %q, want Profile Lang %q", resolved.Lang, i18n.LangJaJP)
	}
	client, err := factory.HttpClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx := requestcontext.WithIdentity(context.Background(), core.AsUser)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://open.feishu.cn/open-apis/test/v1/ping", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if observed == nil ||
		observed.URL.String() != "https://credentials.example/lark-cli/v1/openapi/open-apis/test/v1/ping" ||
		observed.Header.Get(externalcredential.HeaderAppID) != "cli_test" ||
		observed.Header.Get(externalcredential.HeaderIdentity) != "user" ||
		observed.Header.Get(riskcontrol.HeaderOSType) == "" {
		t.Fatalf("managed data-plane request = %#v", observed)
	}
}

func TestFactoryKeepsEnvironmentProviderWhenLegacyConfigIsMalformed(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)
	t.Setenv(envvars.CliAppID, "cli_env")
	t.Setenv(envvars.CliTenantAccessToken, "tenant-token")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv(envvars.CliUserAccessToken, "")
	core.SetCurrentWorkspace(core.WorkspaceLocal)
	if err := os.WriteFile(core.GetConfigPath(), []byte(`{"apps":[`), 0600); err != nil {
		t.Fatal(err)
	}

	factory := newFactoryFromRuntimeBootstrap(&IOStreams{In: nil, Out: io.Discard, ErrOut: io.Discard}, InvocationContext{})
	account, err := factory.Credential.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("ResolveAccount() error = %v", err)
	}
	if account.AppID != "cli_env" {
		t.Fatalf("account AppID = %q, want environment account", account.AppID)
	}
}

func TestFactoryKeepsEnvironmentProviderWhenLegacyConfigIsUnreadable(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)
	t.Setenv(envvars.CliAppID, "cli_env")
	t.Setenv(envvars.CliTenantAccessToken, "tenant-token")
	core.SetCurrentWorkspace(core.WorkspaceLocal)
	if err := os.Mkdir(core.GetConfigPath(), 0700); err != nil {
		t.Fatal(err)
	}

	factory := newFactoryFromRuntimeBootstrap(&IOStreams{In: nil, Out: io.Discard, ErrOut: io.Discard}, InvocationContext{})
	account, err := factory.Credential.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("ResolveAccount() error = %v", err)
	}
	if account.AppID != "cli_env" {
		t.Fatalf("account AppID = %q, want environment account", account.AppID)
	}
}

func TestFactoryRejectsMisspelledExternalCredentialInsteadOfFallingBack(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)
	t.Setenv(envvars.CliAppID, "cli_env")
	t.Setenv(envvars.CliTenantAccessToken, "tenant-token")
	core.SetCurrentWorkspace(core.WorkspaceLocal)
	config := `{"apps":[{"appId":"cli_external","brand":"feishu","users":[],"externalCredentials":{"mode":"proxy"}}]}`
	if err := os.WriteFile(core.GetConfigPath(), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}

	factory := newFactoryFromRuntimeBootstrap(&IOStreams{In: nil, Out: io.Discard, ErrOut: io.Discard}, InvocationContext{})
	if _, err := factory.Credential.ResolveAccount(context.Background()); err == nil {
		t.Fatal("expected misspelled external credential profile to fail closed")
	}
}

func TestFactoryRejectsNullExternalCredentialInsteadOfFallingBack(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)
	t.Setenv(envvars.CliAppID, "cli_env")
	t.Setenv(envvars.CliTenantAccessToken, "tenant-token")
	core.SetCurrentWorkspace(core.WorkspaceLocal)
	config := `{"apps":[{"appId":"cli_external","brand":"feishu","users":[],"externalCredential":null}]}`
	if err := os.WriteFile(core.GetConfigPath(), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}

	factory := newFactoryFromRuntimeBootstrap(&IOStreams{In: nil, Out: io.Discard, ErrOut: io.Discard}, InvocationContext{})
	if _, err := factory.Credential.ResolveAccount(context.Background()); err == nil {
		t.Fatal("expected null external credential profile to fail closed")
	}
}

func TestFactoryRejectsMissingSelectedExternalCredentialProfile(t *testing.T) {
	t.Setenv(envvars.CliAppID, "cli_env")
	t.Setenv(envvars.CliTenantAccessToken, "tenant-token")
	config := &core.MultiAppConfig{
		CurrentApp: "missing",
		Apps: []core.AppConfig{{
			Name: "sandbox", AppId: "cli_external", Brand: core.BrandFeishu, Users: []core.AppUser{},
		}},
	}
	writePlatformProxyConfiguration(t, config, "cli_external")

	factory := newFactoryFromRuntimeBootstrap(&IOStreams{In: nil, Out: io.Discard, ErrOut: io.Discard}, InvocationContext{})
	if _, err := factory.Credential.ResolveAccount(context.Background()); err == nil {
		t.Fatal("expected missing selected external credential profile to fail closed")
	}
}

func TestFactoryUsesOneProfileSnapshotForCredentialAndTransport(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)
	systemPath := configDir + "/external-credential.json"
	t.Setenv(envvars.CliExternalCredentialConfig, systemPath)
	for _, name := range []string{
		envvars.CliAppID, envvars.CliAppSecret, envvars.CliUserAccessToken,
		envvars.CliTenantAccessToken, envvars.CliAuthProxy, envvars.CliProxyKey,
		envvars.CliProxyEnable, envvars.CliProxyAddress, envvars.CliCAPath,
	} {
		t.Setenv(name, "")
	}
	core.SetCurrentWorkspace(core.WorkspaceLocal)
	initial := &core.MultiAppConfig{Apps: []core.AppConfig{{
		Name: "initial", AppId: "cli_initial", AppSecret: core.PlainSecret("initial-secret"),
		Brand: core.BrandFeishu, Users: []core.AppUser{},
	}}}
	if err := core.SaveMultiAppConfig(initial); err != nil {
		t.Fatal(err)
	}

	factory := newFactoryFromRuntimeBootstrap(&IOStreams{In: nil, Out: io.Discard, ErrOut: io.Discard}, InvocationContext{})
	replacement := &core.MultiAppConfig{Apps: []core.AppConfig{{
		Name: "replacement", AppId: "cli_proxy", Brand: core.BrandFeishu, Users: []core.AppUser{},
	}}}
	if err := core.SaveMultiAppConfig(replacement); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(systemPath, []byte(`{"version":1,"mode":"platform_proxy","remoteEndpoint":"https://credentials.example","applications":[{"brand":"feishu","appId":"cli_proxy"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	account, err := factory.Credential.ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if account.AppID != "cli_initial" {
		t.Fatalf("account AppID = %q, want immutable snapshot cli_initial", account.AppID)
	}
	client, err := factory.HttpClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := client.Transport.(*externalcredential.Transport); ok {
		t.Fatalf("transport = %T, should match initial non-proxy profile", client.Transport)
	}
}

func TestFactoryRejectsProxyModeWithTransportExtension(t *testing.T) {
	exttransport.Register(&stubTransportProvider{})
	t.Cleanup(func() { exttransport.Register(nil) })
	config := &core.MultiAppConfig{Apps: []core.AppConfig{{
		AppId: "cli_test", Brand: core.BrandFeishu, Users: []core.AppUser{},
	}}}
	writePlatformProxyConfiguration(t, config, "cli_test")

	factory := newFactoryFromRuntimeBootstrap(&IOStreams{Out: io.Discard, ErrOut: io.Discard}, InvocationContext{})
	if _, err := factory.HttpClient(); err == nil {
		t.Fatal("expected proxy mode and transport extension to be rejected")
	}
}
