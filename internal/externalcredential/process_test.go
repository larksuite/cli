// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package externalcredential

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/i18n"
	"github.com/larksuite/cli/internal/vfs"
)

func TestCredentialProcessHelper(t *testing.T) {
	if os.Getenv("LARK_TEST_CREDENTIAL_HELPER") != "1" {
		return
	}
	request, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(2)
	}
	if capture := os.Getenv("LARK_TEST_CREDENTIAL_CAPTURE"); capture != "" {
		_ = os.WriteFile(capture, request, 0600)
	}
	if countPath := os.Getenv("LARK_TEST_CREDENTIAL_COUNT"); countPath != "" {
		file, openErr := os.OpenFile(countPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if openErr == nil {
			_, _ = file.WriteString("1\n")
			_ = file.Close()
		}
	}
	if capture := os.Getenv("LARK_TEST_CREDENTIAL_ENV_CAPTURE"); capture != "" {
		cwd, cwdErr := os.Getwd()
		if cwdErr != nil {
			os.Exit(2)
		}
		snapshot, marshalErr := json.Marshal(map[string]string{
			"cwd":                   cwd,
			"ordinary":              os.Getenv("UNRELATED_AMBIENT"),
			"path":                  os.Getenv("PATH"),
			"ld_preload":            os.Getenv("LD_PRELOAD"),
			"dyld_insert_libraries": os.Getenv("DYLD_INSERT_LIBRARIES"),
			"https_proxy":           os.Getenv("HTTPS_PROXY"),
			"no_proxy":              os.Getenv("NO_PROXY"),
			"ssl_cert_file":         os.Getenv("SSL_CERT_FILE"),
			"tmpdir":                os.Getenv("TMPDIR"),
			"lang":                  os.Getenv("LANG"),
			"systemroot":            os.Getenv("SYSTEMROOT"),
		})
		if marshalErr != nil || os.WriteFile(capture, snapshot, 0600) != nil {
			os.Exit(2)
		}
	}
	var req Request
	if json.Unmarshal(request, &req) != nil {
		os.Exit(2)
	}
	if os.Getenv("LARK_TEST_CREDENTIAL_SPAWN_PIPE_HOLDER") == "1" {
		executable, executableErr := os.Executable()
		if executableErr != nil {
			os.Exit(2)
		}
		child := exec.Command(executable, "-test.run=TestCredentialProcessPipeHolder")
		child.Env = append(os.Environ(), "LARK_TEST_CREDENTIAL_PIPE_HOLDER=1")
		child.Stdout = os.Stdout
		if child.Start() != nil {
			os.Exit(2)
		}
		time.Sleep(10 * time.Second)
		os.Exit(2)
	}
	if delay := os.Getenv("LARK_TEST_CREDENTIAL_DELAY"); delay != "" {
		duration, parseErr := time.ParseDuration(delay)
		if parseErr != nil {
			os.Exit(2)
		}
		time.Sleep(duration)
	}
	if raw := os.Getenv("LARK_TEST_CREDENTIAL_RESPONSE"); raw != "" {
		_, _ = io.WriteString(os.Stdout, raw)
		exitCode, _ := strconv.Atoi(os.Getenv("LARK_TEST_CREDENTIAL_EXIT_CODE"))
		os.Exit(exitCode)
	}
	expiresAt := time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)
	var out any
	switch req.Mode {
	case ModeCredentialProxy:
		out = map[string]any{"version": 1, "credential": map[string]any{"scheme": "bearer", "access_token": "proxy-short-lived", "expires_at": expiresAt}}
	case ModeDirect:
		tokenType := "uat"
		if req.Identity == "bot" {
			tokenType = "tat"
		}
		out = map[string]any{"version": 1, "credential": map[string]any{"token_type": tokenType, "access_token": "direct-token", "expires_at": expiresAt, "scopes": []string{"im:message"}}}
	default:
		os.Exit(2)
	}
	_ = json.NewEncoder(os.Stdout).Encode(out)
	os.Exit(0)
}

func TestResolveAccountEnvironmentIsolationByMode(t *testing.T) {
	modes := []Mode{ModeDirect, ModeCredentialProxy, ModePlatformProxy}
	credentialEnvironment := credentialSourceEnvironmentNames()
	transportEnvironment := localTransportEnvironmentNames()
	allEnvironment := append(append([]string{}, credentialEnvironment...), transportEnvironment...)

	tests := make([]struct {
		name        string
		environment string
		credential  bool
		value       string
	}, 0, len(allEnvironment))
	for _, name := range credentialEnvironment {
		tests = append(tests, struct {
			name        string
			environment string
			credential  bool
			value       string
		}{
			name:        name,
			environment: name,
			credential:  true,
			value:       "configured",
		})
	}
	for _, item := range []struct {
		name  string
		value string
	}{
		{name: envvars.CliProxyEnable, value: "true"},
		{name: envvars.CliProxyAddress, value: "http://127.0.0.1:3128"},
		{name: envvars.CliCAPath, value: "/tmp/lark-cli-test-ca.pem"},
	} {
		tests = append(tests, struct {
			name        string
			environment string
			credential  bool
			value       string
		}{
			name:        item.name,
			environment: item.name,
			value:       item.value,
		})
	}

	for _, mode := range modes {
		for _, tt := range tests {
			t.Run(string(mode)+"/"+tt.name, func(t *testing.T) {
				for _, name := range allEnvironment {
					t.Setenv(name, "")
				}
				t.Setenv(tt.environment, tt.value)

				provider := NewProvider(&core.AppConfig{
					Name:      "sandbox",
					AppId:     "cli_test",
					Brand:     core.BrandFeishu,
					DefaultAs: core.AsUser,
				}, &Config{Version: 1, Mode: mode})
				account, err := provider.ResolveAccount(context.Background())
				wantReject := tt.credential || mode.IsProxy()
				if wantReject {
					if err == nil {
						t.Fatalf("ResolveAccount() accepted %s in %s mode", tt.environment, mode)
					}
					problem, ok := errs.ProblemOf(err)
					if !ok {
						t.Fatalf("ResolveAccount() error is not typed: %T %v", err, err)
					}
					if problem.Category != errs.CategoryConfig || problem.Subtype != errs.SubtypeInvalidConfig {
						t.Fatalf("ResolveAccount() problem = %#v, want config/invalid_config", problem)
					}
					wantHint := "remove the legacy credential or identity environment variable before using external-credential.json"
					if !tt.credential {
						wantHint = "remove the local proxy or CA environment variable when using a managed proxy mode"
					}
					if !strings.Contains(problem.Message, tt.environment) || problem.Hint != wantHint {
						t.Fatalf("ResolveAccount() problem = %#v, want environment %s and hint %q",
							problem, tt.environment, wantHint)
					}
					return
				}
				if err != nil {
					t.Fatalf("ResolveAccount() rejected direct-mode local transport variable %s: %v", tt.environment, err)
				}
				if account == nil || account.AppID != "cli_test" {
					t.Fatalf("ResolveAccount() account = %#v, want cli_test", account)
				}
			})
		}
	}
}

func TestDecodeResponseRejectsDuplicateProtocolFields(t *testing.T) {
	expiresAt := time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)
	tests := []string{
		`{"version":1,"Version":1,"credential":{"token_type":"uat","access_token":"token","expires_at":"` + expiresAt + `"}}`,
		`{"version":1,"credential":{"token_type":"uat","access_token":"token-a","Access_Token":"token-b","expires_at":"` + expiresAt + `"}}`,
	}
	for _, data := range tests {
		if _, err := decodeResponse([]byte(data)); err == nil {
			t.Fatalf("decodeResponse accepted duplicate protocol field: %s", data)
		}
	}
}

func TestCredentialProcessPipeHolder(t *testing.T) {
	if os.Getenv("LARK_TEST_CREDENTIAL_PIPE_HOLDER") != "1" {
		return
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := io.WriteString(os.Stdout, " "); err != nil {
			os.Exit(0)
		}
		time.Sleep(20 * time.Millisecond)
	}
	// This is only a backstop for a regressed parent that never closes the
	// inherited pipe. The normal WaitDelay path makes the write fail first.
	os.Exit(0)
}

func TestCredentialProcessBareEnvironmentHelper(t *testing.T) {
	child := false
	for _, arg := range os.Args {
		if arg == "-test.run=^TestCredentialProcessBareEnvironmentHelper$" {
			child = true
			break
		}
	}
	if !child {
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		os.Exit(2)
	}
	if err := json.NewEncoder(os.Stdout).Encode(map[string]string{
		"cwd":        cwd,
		"path":       os.Getenv("PATH"),
		"ld_preload": os.Getenv("LD_PRELOAD"),
		"dyld":       os.Getenv("DYLD_INSERT_LIBRARIES"),
		"systemroot": os.Getenv("SYSTEMROOT"),
	}); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func helperProgram(t *testing.T, timeout int) *ProgramConfig {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	t.Setenv("LARK_TEST_CREDENTIAL_HELPER", "1")
	t.Setenv(envvars.CliExternalCredentialConfig, t.TempDir()+"/external-credential.json")
	return &ProgramConfig{
		Executable: executable, Arguments: []string{"-test.run=TestCredentialProcessHelper"},
		SHA256: "sha256:" + fmt.Sprintf("%x", sum), ProtocolVersion: 1, TimeoutSeconds: timeout,
	}
}

func testExternalConfig(t *testing.T, mode Mode, timeout int) *Config {
	t.Helper()
	cfg := &Config{Version: 1, Mode: mode, Program: helperProgram(t, timeout)}
	if mode.IsProxy() {
		cfg.RemoteEndpoint = "https://credentials.example"
	}
	return cfg
}

func newTestProvider(t *testing.T, app *core.AppConfig, config *Config) *Provider {
	t.Helper()
	provider := NewProvider(app, config)
	provider.processEnvironment = func(env []string) ([]string, error) {
		out, err := credentialProcessEnvironment(env)
		if err != nil {
			return nil, err
		}
		for _, item := range env {
			name, _, ok := strings.Cut(item, "=")
			if ok && strings.HasPrefix(strings.ToUpper(name), "LARK_TEST_CREDENTIAL_") {
				out = append(out, item)
			}
		}
		return out, nil
	}
	return provider
}

func assertCachedCredentialHasExpiry(t *testing.T, provider *Provider, key cacheKey) {
	t.Helper()
	provider.mu.Lock()
	entry, ok := provider.cache[key]
	provider.mu.Unlock()
	if !ok || entry.expiresAt.IsZero() {
		t.Fatalf("cached credential %v = %#v, want an internally tracked expiry", key, entry)
	}
}

func TestProviderSkipsUserInfoEnrichment(t *testing.T) {
	app := &core.AppConfig{
		Name:  "sandbox",
		AppId: "cli_test",
		Brand: core.BrandFeishu,
		Lang:  i18n.LangJaJP,
	}
	config := &Config{
		Version:        1,
		Mode:           ModePlatformProxy,
		RemoteEndpoint: "https://credentials.example",
	}
	provider := newTestProvider(t, app, config)
	httpClientCalls := 0
	credentials := credential.NewCredentialProvider(
		[]extcred.Provider{provider},
		nil,
		nil,
		func() (*http.Client, error) {
			httpClientCalls++
			return nil, errors.New("unexpected user-info enrichment")
		},
	)

	account, err := credentials.ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if account.AppID != app.AppId {
		t.Fatalf("account AppID = %q, want %q", account.AppID, app.AppId)
	}
	if account.Lang != app.Lang {
		t.Fatalf("account Lang = %q, want Profile Lang %q", account.Lang, app.Lang)
	}
	if httpClientCalls != 0 {
		t.Fatalf("user-info HTTP client calls = %d, want 0", httpClientCalls)
	}
}

func TestSelectProfileRemoteMetadataPolicy(t *testing.T) {
	for _, tc := range []struct {
		name        string
		mode        Mode
		wantDisable bool
	}{
		{name: "platform proxy", mode: ModePlatformProxy, wantDisable: true},
		{name: "credential proxy", mode: ModeCredentialProxy, wantDisable: true},
		{name: "direct", mode: ModeDirect, wantDisable: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configDir := t.TempDir()
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)
			core.SetCurrentWorkspace(core.WorkspaceLocal)

			app := core.AppConfig{
				Name: "sandbox", AppId: "cli_test", Brand: core.BrandFeishu, Users: []core.AppUser{},
			}
			if err := core.SaveMultiAppConfig(&core.MultiAppConfig{
				CurrentApp: "sandbox",
				Apps:       []core.AppConfig{app},
			}); err != nil {
				t.Fatal(err)
			}

			systemPath := configDir + "/external-credential.json"
			t.Setenv(envvars.CliExternalCredentialConfig, systemPath)
			cfg := &Config{
				Version:      1,
				Mode:         tc.mode,
				Applications: []Application{{Brand: core.BrandFeishu, AppID: app.AppId}},
			}
			if tc.mode.IsProxy() {
				cfg.RemoteEndpoint = "https://credentials.example"
			}
			if tc.mode != ModePlatformProxy {
				cfg.Program = helperProgram(t, 5)
				// helperProgram installs its own development system path.
				systemPath = os.Getenv(envvars.CliExternalCredentialConfig)
			}
			data, err := json.Marshal(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if err := vfs.WriteFile(systemPath, data, 0o600); err != nil {
				t.Fatal(err)
			}

			selection, err := SelectProfile("")
			if err != nil {
				t.Fatal(err)
			}
			if selection.DisableRemoteMeta != tc.wantDisable {
				t.Fatalf("DisableRemoteMeta = %v, want %v", selection.DisableRemoteMeta, tc.wantDisable)
			}
		})
	}
}

func TestProviderDirectProtocol(t *testing.T) {
	capture := t.TempDir() + "/request.json"
	t.Setenv("LARK_TEST_CREDENTIAL_CAPTURE", capture)
	app := &core.AppConfig{
		Name: "sandbox", AppId: "cli_test", Brand: core.BrandFeishu, Users: []core.AppUser{},
	}
	provider := newTestProvider(t, app, testExternalConfig(t, ModeDirect, 5))
	token, err := provider.ResolveToken(context.Background(), extcred.TokenSpec{Type: extcred.TokenTypeUAT, AppID: app.AppId})
	if err != nil {
		t.Fatal(err)
	}
	if token.Value != "direct-token" || token.Scopes != "im:message" || token.Source != provider.Name() {
		t.Fatalf("token = %#v", token)
	}
	assertCachedCredentialHasExpiry(t, provider, cacheKey{
		credentialType: credentialAccessToken,
		identity:       string(core.AsUser),
	})
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		t.Fatal(err)
	}
	if req.CredentialType != credentialAccessToken || req.Identity != "user" || req.RemoteEndpoint != "" {
		t.Fatalf("request = %#v", req)
	}
}

func TestProviderProxyProtocol(t *testing.T) {
	capture := t.TempDir() + "/request.json"
	t.Setenv("LARK_TEST_CREDENTIAL_CAPTURE", capture)
	app := &core.AppConfig{
		AppId: "cli_test", Brand: core.BrandLark, Users: []core.AppUser{},
	}
	provider := newTestProvider(t, app, testExternalConfig(t, ModeCredentialProxy, 5))
	token, err := provider.ResolveToken(context.Background(), extcred.TokenSpec{Type: extcred.TokenTypeTAT, AppID: app.AppId})
	if err != nil {
		t.Fatal(err)
	}
	if token.Value != proxyBotPlaceholder || token.Scopes != "" || token.Source != provider.Name() {
		t.Fatalf("SDK placeholder = %#v", token)
	}
	proxyToken, err := provider.ResolveProxyCredential(context.Background(), core.AsBot)
	if err != nil {
		t.Fatal(err)
	}
	if proxyToken != "proxy-short-lived" {
		t.Fatalf("proxy token = %q", proxyToken)
	}
	assertCachedCredentialHasExpiry(t, provider, cacheKey{
		credentialType: credentialProxyToken,
		identity:       string(core.AsBot),
	})
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		t.Fatal(err)
	}
	if req.Mode != ModeCredentialProxy ||
		req.CredentialType != credentialProxyToken ||
		req.Identity != "bot" ||
		req.RemoteEndpoint != "https://credentials.example" {
		t.Fatalf("request = %#v", req)
	}
}

func TestProviderPlatformProxyUsesNonSecretPlaceholders(t *testing.T) {
	app := &core.AppConfig{
		AppId: "cli_test", Brand: core.BrandFeishu, Users: []core.AppUser{},
	}
	provider := newTestProvider(t, app, &Config{
		Version: 1, Mode: ModePlatformProxy,
		RemoteEndpoint: "https://credentials.example",
	})
	user, err := provider.ResolveToken(context.Background(), extcred.TokenSpec{Type: extcred.TokenTypeUAT, AppID: app.AppId})
	if err != nil {
		t.Fatal(err)
	}
	bot, err := provider.ResolveToken(context.Background(), extcred.TokenSpec{Type: extcred.TokenTypeTAT, AppID: app.AppId})
	if err != nil {
		t.Fatal(err)
	}
	if user.Value != proxyUserPlaceholder || bot.Value != proxyBotPlaceholder {
		t.Fatalf("placeholders: user=%q bot=%q", user.Value, bot.Value)
	}
	if _, err := provider.ResolveProxyCredential(context.Background(), core.AsUser); err == nil {
		t.Fatal("platform_proxy must not issue a proxy bearer")
	}
}

func TestProviderRejectsProgramDigestMismatch(t *testing.T) {
	cfg := testExternalConfig(t, ModeDirect, 5)
	cfg.Program.SHA256 = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	app := &core.AppConfig{AppId: "cli_test", Brand: core.BrandFeishu}
	_, err := newTestProvider(t, app, cfg).ResolveToken(context.Background(), extcred.TokenSpec{Type: extcred.TokenTypeUAT, AppID: app.AppId})
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryConfig || problem.Subtype != errs.SubtypeInvalidConfig {
		t.Fatalf("digest mismatch error = %#v, ok=%v", problem, ok)
	}
}

func TestProviderRejectsMismatchedAppID(t *testing.T) {
	app := &core.AppConfig{
		AppId: "cli_expected", Brand: core.BrandFeishu, Users: []core.AppUser{},
	}
	_, err := newTestProvider(t, app, testExternalConfig(t, ModeDirect, 5)).ResolveToken(context.Background(), extcred.TokenSpec{Type: extcred.TokenTypeUAT, AppID: "cli_other"})
	if err == nil {
		t.Fatal("expected mismatched app ID to be rejected")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("problem = %#v, ok = %v", problem, ok)
	}
}

func TestProviderRejectsLegacyCredentialEnvironment(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_USER_ACCESS_TOKEN", "must-not-be-used")
	app := &core.AppConfig{
		AppId: "cli_test", Brand: core.BrandFeishu, Users: []core.AppUser{},
	}
	_, err := newTestProvider(t, app, testExternalConfig(t, ModeDirect, 5)).ResolveAccount(context.Background())
	if err == nil {
		t.Fatal("expected legacy environment credential to be rejected")
	}
}

func TestProviderCoalescesConcurrentCredentialRequests(t *testing.T) {
	countPath := t.TempDir() + "/count"
	t.Setenv("LARK_TEST_CREDENTIAL_COUNT", countPath)
	app := &core.AppConfig{
		AppId: "cli_test", Brand: core.BrandFeishu, Users: []core.AppUser{},
	}
	provider := newTestProvider(t, app, testExternalConfig(t, ModeCredentialProxy, 5))
	start := make(chan struct{})
	errsCh := make(chan error, 16)
	for i := 0; i < cap(errsCh); i++ {
		go func() {
			<-start
			_, err := provider.ResolveProxyCredential(context.Background(), core.AsUser)
			errsCh <- err
		}()
	}
	close(start)
	for i := 0; i < cap(errsCh); i++ {
		if err := <-errsCh; err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(countPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(strings.Fields(string(data))); got != 1 {
		t.Fatalf("credential process executions = %d, want 1", got)
	}
}

func TestDecodeResponseRejectsUnknownFields(t *testing.T) {
	_, err := decodeResponse([]byte(`{"version":1,"credential":{"scheme":"bearer","access_token":"x","expires_at":"2099-01-01T00:00:00Z","uat":"leak"}}`))
	if err == nil {
		t.Fatal("expected closed response schema to reject unknown credential field")
	}
}

func TestProviderValidatesFailureEnvelopeBeforeClassifyingError(t *testing.T) {
	tests := []struct {
		name     string
		response string
	}{
		{
			name:     "unsupported version",
			response: `{"version":2,"error":{"code":"access_denied","message":"denied"}}`,
		},
		{
			name:     "credential and error",
			response: `{"version":1,"credential":{"token_type":"uat","access_token":"token","expires_at":"2099-01-01T00:00:00Z"},"error":{"code":"access_denied","message":"denied"}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LARK_TEST_CREDENTIAL_RESPONSE", tt.response)
			t.Setenv("LARK_TEST_CREDENTIAL_EXIT_CODE", "7")
			app := &core.AppConfig{
				AppId: "cli_test", Brand: core.BrandFeishu, Users: []core.AppUser{},
			}
			_, err := newTestProvider(t, app, testExternalConfig(t, ModeDirect, 5)).ResolveToken(context.Background(), extcred.TokenSpec{Type: extcred.TokenTypeUAT, AppID: app.AppId})
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("problem = %#v, ok = %v, err = %v", problem, ok, err)
			}
		})
	}
}

func TestProviderExternalToolFailureExplainsIsolatedEnvironment(t *testing.T) {
	t.Setenv("LARK_TEST_CREDENTIAL_RESPONSE", "not-json")
	t.Setenv("LARK_TEST_CREDENTIAL_EXIT_CODE", "7")
	app := &core.AppConfig{
		AppId: "cli_test", Brand: core.BrandFeishu, Users: []core.AppUser{},
	}
	_, err := newTestProvider(t, app, testExternalConfig(t, ModeDirect, 5)).ResolveToken(
		context.Background(),
		extcred.TokenSpec{Type: extcred.TokenTypeUAT, AppID: app.AppId},
	)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeExternalTool {
		t.Fatalf("problem = %#v, ok = %v, err = %v", problem, ok, err)
	}
	if !strings.Contains(problem.Hint, "no caller environment or PATH") {
		t.Fatalf("hint = %q, want isolated-environment recovery guidance", problem.Hint)
	}
}

func TestValidateResponseLimitsOnlyProxyCredentialLifetime(t *testing.T) {
	now := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	expiresAt := now.Add(maxProxyCredentialTTL + time.Minute).Format(time.RFC3339)

	proxy := &response{Version: protocolVersion, Credential: &wireCredential{
		Scheme: "bearer", AccessToken: "proxy-token", ExpiresAt: expiresAt,
	}}
	if _, _, err := validateResponse(proxy, ModeCredentialProxy, now); err == nil {
		t.Fatal("expected an overlong proxy credential to be rejected")
	}

	direct := &response{Version: protocolVersion, Credential: &wireCredential{
		TokenType: "uat", AccessToken: "user-token", ExpiresAt: expiresAt,
	}}
	if _, _, err := validateResponse(direct, ModeDirect, now); err != nil {
		t.Fatalf("direct credential should not use proxy TTL limit: %v", err)
	}
}

func TestCredentialProcessEnvironmentUsesExplicitAllowlist(t *testing.T) {
	env := []string{
		"SystemRoot=C:\\caller-controlled",
		"TMPDIR=/var/tmp/helper",
		"LANG=C.UTF-8",
		"https_proxy=https://proxy.example",
		"NO_PROXY=localhost",
		"SSL_CERT_FILE=/etc/ssl/custom.pem",
		"PATH=/untrusted/bin",
		"LD_PRELOAD=/untrusted/loader.so",
		"DYLD_INSERT_LIBRARIES=/untrusted/loader.dylib",
		"UNRELATED_AMBIENT=drop-me",
		"LARKSUITE_CLI_APP_SECRET=uppercase-secret",
		"larksuite_cli_user_access_token=lowercase-token",
	}
	got, err := credentialProcessEnvironment(env)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("credentialProcessEnvironment() returned nil, which would inherit the caller environment")
	}
	if runtime.GOOS != "windows" {
		if len(got) != 0 {
			t.Fatalf("credentialProcessEnvironment() = %#v, want no inherited environment", got)
		}
		return
	}
	for _, item := range got {
		name, value, ok := strings.Cut(item, "=")
		if !ok || (name != "SYSTEMROOT" && name != "WINDIR") {
			t.Fatalf("unexpected trusted Windows environment item %q", item)
		}
		if strings.EqualFold(value, `C:\caller-controlled`) {
			t.Fatalf("trusted Windows environment reused caller value: %q", item)
		}
	}
}

func TestCredentialProcessCommandRejectsImplicitEnvironment(t *testing.T) {
	program := &ProgramConfig{Executable: os.Args[0]}
	cmd, err := newCredentialProcessCommand(context.Background(), program, nil, nil)
	if err == nil || cmd != nil {
		t.Fatalf("newCredentialProcessCommand() = (%v, %v), want nil command and error", cmd, err)
	}
}

func TestCredentialProcessCommandRunsWithIsolatedEnvironment(t *testing.T) {
	t.Setenv("PATH", "/caller/path")
	t.Setenv("LD_PRELOAD", "/caller/loader.so")
	t.Setenv("DYLD_INSERT_LIBRARIES", "/caller/loader.dylib")

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	environment, err := credentialProcessEnvironment(os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	program := &ProgramConfig{
		Executable: executable,
		Arguments:  []string{"-test.run=^TestCredentialProcessBareEnvironmentHelper$"},
	}
	cmd, err := newCredentialProcessCommand(context.Background(), program, nil, environment)
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["path"] != "" || got["ld_preload"] != "" || got["dyld"] != "" {
		t.Fatalf("helper inherited caller environment: %#v", got)
	}
	if runtime.GOOS == "windows" && got["systemroot"] == "" {
		t.Fatal("Windows helper is missing trusted SYSTEMROOT")
	}
	wantCWD := filepath.Dir(filepath.Clean(executable))
	gotCWDInfo, gotCWDErr := os.Stat(got["cwd"])
	wantCWDInfo, wantCWDErr := os.Stat(wantCWD)
	if gotCWDErr != nil || wantCWDErr != nil || !os.SameFile(gotCWDInfo, wantCWDInfo) {
		t.Fatalf("cwd = %q, want filesystem-equivalent path %q (got err %v, want err %v)", got["cwd"], wantCWD, gotCWDErr, wantCWDErr)
	}
}

func TestProviderFailsClosedWhenIsolatedEnvironmentIsUnavailable(t *testing.T) {
	app := &core.AppConfig{
		AppId: "cli_test", Brand: core.BrandFeishu, Users: []core.AppUser{},
	}
	provider := NewProvider(app, testExternalConfig(t, ModeDirect, 5))
	cause := errors.New("trusted runtime lookup failed")
	provider.processEnvironment = func([]string) ([]string, error) {
		return nil, cause
	}
	_, err := provider.ResolveToken(
		context.Background(),
		extcred.TokenSpec{Type: extcred.TokenTypeUAT, AppID: app.AppId},
	)
	if !errors.Is(err, cause) {
		t.Fatalf("err = %v, want cause %v", err, cause)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeExternalTool {
		t.Fatalf("problem = %#v, ok = %v, err = %v", problem, ok, err)
	}
}

func TestCredentialProcessRunsWithIsolatedEnvironmentAndTrustedWorkingDirectory(t *testing.T) {
	capture := filepath.Join(t.TempDir(), "environment.json")
	t.Setenv("LARK_TEST_CREDENTIAL_ENV_CAPTURE", capture)
	t.Setenv("UNRELATED_AMBIENT", "drop-me")
	t.Setenv("PATH", "/untrusted/bin")
	t.Setenv("LD_PRELOAD", "/untrusted/loader.so")
	t.Setenv("DYLD_INSERT_LIBRARIES", "/untrusted/loader.dylib")
	t.Setenv("HTTPS_PROXY", "https://proxy.example")
	t.Setenv("NO_PROXY", "localhost")
	t.Setenv("SSL_CERT_FILE", "/etc/ssl/custom.pem")
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("LANG", "C.UTF-8")
	t.Setenv("SYSTEMROOT", "C:\\caller-controlled")

	app := &core.AppConfig{
		AppId: "cli_test", Brand: core.BrandFeishu, Users: []core.AppUser{},
	}
	config := testExternalConfig(t, ModeDirect, 5)
	if _, err := newTestProvider(t, app, config).ResolveToken(
		context.Background(),
		extcred.TokenSpec{Type: extcred.TokenTypeUAT, AppID: app.AppId},
	); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"ordinary",
		"path",
		"ld_preload",
		"dyld_insert_libraries",
		"https_proxy",
		"no_proxy",
		"ssl_cert_file",
		"tmpdir",
		"lang",
	} {
		if got[name] != "" {
			t.Errorf("%s = %q, want empty", name, got[name])
		}
	}
	if runtime.GOOS == "windows" {
		if got["systemroot"] == "" || strings.EqualFold(got["systemroot"], `C:\caller-controlled`) {
			t.Errorf("systemroot = %q, want independently resolved Windows directory", got["systemroot"])
		}
	} else if got["systemroot"] != "" {
		t.Errorf("systemroot = %q, want empty", got["systemroot"])
	}
	wantCWD := filepath.Dir(filepath.Clean(config.Program.Executable))
	gotCWDInfo, gotCWDErr := os.Stat(got["cwd"])
	wantCWDInfo, wantCWDErr := os.Stat(wantCWD)
	if gotCWDErr != nil || wantCWDErr != nil || !os.SameFile(gotCWDInfo, wantCWDInfo) {
		t.Errorf("cwd = %q, want filesystem-equivalent path %q (got err %v, want err %v)", got["cwd"], wantCWD, gotCWDErr, wantCWDErr)
	}
}

func TestCredentialProcessCallerCancellationIsNotSourceFailure(t *testing.T) {
	t.Setenv("LARK_TEST_CREDENTIAL_DELAY", "10s")
	app := &core.AppConfig{
		AppId: "cli_test", Brand: core.BrandFeishu, Users: []core.AppUser{},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := newTestProvider(t, app, testExternalConfig(t, ModeDirect, 5)).ResolveToken(ctx, extcred.TokenSpec{Type: extcred.TokenTypeUAT, AppID: app.AppId})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if problem, ok := errs.ProblemOf(err); ok {
		t.Fatalf("caller cancellation was reclassified as %#v", problem)
	}
}

func TestCredentialProcessCallerDeadlineIsNotConfiguredTimeout(t *testing.T) {
	t.Setenv("LARK_TEST_CREDENTIAL_DELAY", "10s")
	app := &core.AppConfig{
		AppId: "cli_test", Brand: core.BrandFeishu, Users: []core.AppUser{},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := newTestProvider(t, app, testExternalConfig(t, ModeDirect, 5)).ResolveToken(ctx, extcred.TokenSpec{Type: extcred.TokenTypeUAT, AppID: app.AppId})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if problem, ok := errs.ProblemOf(err); ok {
		t.Fatalf("caller deadline was reclassified as configured process timeout: %#v", problem)
	}
}

func TestCredentialProcessTimeoutClosesInheritedPipes(t *testing.T) {
	t.Setenv("LARK_TEST_CREDENTIAL_SPAWN_PIPE_HOLDER", "1")
	app := &core.AppConfig{
		AppId: "cli_test", Brand: core.BrandFeishu, Users: []core.AppUser{},
	}
	started := time.Now()
	_, err := newTestProvider(t, app, testExternalConfig(t, ModeDirect, 1)).ResolveToken(context.Background(), extcred.TokenSpec{Type: extcred.TokenTypeUAT, AppID: app.AppId})
	elapsed := time.Since(started)
	problem, ok := errs.ProblemOf(err)
	metadata, _ := errs.DiagnosticMetadataOf(err)
	if !ok || problem.Category != errs.CategoryNetwork || problem.Subtype != errs.SubtypeCredentialSourceUnavailable || !problem.Retryable || metadata.Origin != "credential_process" {
		t.Fatalf("problem = %#v, metadata = %#v, ok = %v, err = %v", problem, metadata, ok, err)
	}
	if elapsed >= 4*time.Second {
		t.Fatalf("credential process timeout took %s; inherited output pipe was not bounded", elapsed)
	}
}
