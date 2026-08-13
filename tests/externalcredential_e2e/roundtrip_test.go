// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package externalcredential_e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/vfs"
)

const (
	e2eAppID       = "cli_external_e2e"
	e2eMinuteToken = "obcne2e"
	e2eMedia       = "external-credential-media"
	e2eDirectToken = "direct-e2e-token"
	e2eProxyToken  = "proxy-e2e-token"
)

type commandResult struct {
	exitCode int
	stdout   string
	stderr   string
}

type observedRequest struct {
	method         string
	path           string
	authorization  string
	appID          string
	identity       string
	proxyVersion   string
	originalTarget string
	requestID      string
}

type proxyHarness struct {
	server        *httptest.Server
	expectedAuth  string
	mu            sync.Mutex
	requests      []observedRequest
	handlerErrors []string
}

func newProxyHarness(t *testing.T, expectedAuth string) *proxyHarness {
	t.Helper()
	h := &proxyHarness{expectedAuth: expectedAuth}
	h.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.observe(r)
		switch r.URL.Path {
		case "/lark-cli/v1/openapi/open-apis/test/v1/ping":
			writeResponse(w, map[string]any{"code": 0, "data": map[string]any{"pong": true}})
		case "/lark-cli/v1/openapi/open-apis/minutes/v1/minutes/" + e2eMinuteToken + "/media":
			writeResponse(w, map[string]any{
				"code": 0,
				"data": map[string]any{
					"download_url": h.server.URL + "/lark-cli/v1/files/minute_e2e",
				},
			})
		case "/lark-cli/v1/files/minute_e2e":
			w.Header().Set("Content-Type", "audio/mpeg")
			w.Header().Set("Content-Disposition", `attachment; filename="recording.mp3"`)
			_, _ = w.Write([]byte(e2eMedia))
		default:
			http.Error(w, "unexpected proxy path", http.StatusNotFound)
		}
	}))
	t.Cleanup(h.server.Close)
	return h
}

func (h *proxyHarness) observe(r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	got := observedRequest{
		method:         r.Method,
		path:           r.URL.Path,
		authorization:  r.Header.Get("Authorization"),
		appID:          r.Header.Get("X-Lark-CLI-App-ID"),
		identity:       r.Header.Get("X-Lark-CLI-Identity"),
		proxyVersion:   r.Header.Get("X-Lark-Proxy-Version"),
		originalTarget: r.Header.Get("X-Lark-CLI-Original-Target"),
		requestID:      r.Header.Get("X-Lark-CLI-Request-ID"),
	}
	h.requests = append(h.requests, got)
	if got.authorization != h.expectedAuth {
		h.handlerErrors = append(h.handlerErrors,
			fmt.Sprintf("%s Authorization = %q, want %q", got.path, got.authorization, h.expectedAuth))
	}
	if got.appID != e2eAppID || got.identity != "user" || got.proxyVersion != "1" ||
		got.originalTarget == "" || got.requestID == "" {
		h.handlerErrors = append(h.handlerErrors,
			fmt.Sprintf("%s missing or invalid proxy metadata: %#v", got.path, got))
	}
}

func (h *proxyHarness) assertComplete(t *testing.T) {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.handlerErrors) != 0 {
		t.Fatalf("proxy contract failures: %s", strings.Join(h.handlerErrors, "; "))
	}
	wantPaths := []string{
		"/lark-cli/v1/openapi/open-apis/test/v1/ping",
		"/lark-cli/v1/openapi/open-apis/minutes/v1/minutes/" + e2eMinuteToken + "/media",
		"/lark-cli/v1/files/minute_e2e",
	}
	wantTargets := []string{
		"https://open.feishu.cn/open-apis/test/v1/ping",
		"https://open.feishu.cn/open-apis/minutes/v1/minutes/" + e2eMinuteToken + "/media",
		h.server.URL + "/lark-cli/v1/files/minute_e2e",
	}
	if len(h.requests) != len(wantPaths) {
		t.Fatalf("proxy saw %d requests, want %d: %#v", len(h.requests), len(wantPaths), h.requests)
	}
	for i, want := range wantPaths {
		if got := h.requests[i]; got.method != http.MethodGet || got.path != want ||
			got.originalTarget != wantTargets[i] {
			t.Fatalf("proxy request %d = %#v, want GET %q with original target %q",
				i, got, want, wantTargets[i])
		}
	}
}

func writeResponse(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func TestExternalCredentialProtocolRoundTrip(t *testing.T) {
	repo := repositoryRoot(t)
	binDir := t.TempDir()
	cliPath := filepath.Join(binDir, executableName("lark-cli-e2e"))
	helperPath := filepath.Join(binDir, executableName("credential-helper-e2e"))
	overlayPath := writeRootCAOverlay(t, repo)
	buildBinary(t, repo, cliPath, "-overlay", overlayPath, "-tags", "extended", ".")
	buildBinary(t, repo, helperPath, "./tests/externalcredential_e2e/testdata/helper")

	t.Run("direct helper scopes drive auth check", func(t *testing.T) {
		configDir, capture := writeExternalConfiguration(t, helperPath, "direct", "")
		result := runCLI(t, cliPath, configDir, "", "auth", "check", "--scope", "im:message", "--json")
		assertSuccess(t, result)
		var payload struct {
			OK      bool     `json:"ok"`
			Granted []string `json:"granted"`
			Missing []string `json:"missing"`
		}
		if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
			t.Fatalf("decode auth check output: %v\n%s", err, result.stdout)
		}
		if !payload.OK || len(payload.Granted) != 1 || payload.Granted[0] != "im:message" ||
			len(payload.Missing) != 0 {
			t.Fatalf("auth check output = %#v", payload)
		}
		assertNoCredentialLeak(t, result)
		assertHelperRequest(t, capture, "direct", "access_token", "user", "")
	})

	for _, tc := range []struct {
		mode         string
		expectedAuth string
	}{
		{mode: "credential_proxy", expectedAuth: "Bearer proxy-e2e-token"},
		{mode: "platform_proxy", expectedAuth: ""},
	} {
		t.Run(tc.mode+" API and opaque file", func(t *testing.T) {
			proxy := newProxyHarness(t, tc.expectedAuth)
			configDir, capture := writeExternalConfiguration(t, helperPath, tc.mode, proxy.server.URL)
			caPath := writeServerCA(t, proxy.server)

			scopeResult := runCLI(t, cliPath, configDir, caPath,
				"auth", "check", "--scope", "im:message", "--json")
			assertTypedScopeUnavailable(t, scopeResult)
			assertHelperNotInvoked(t, capture)

			apiResult := runCLI(t, cliPath, configDir, caPath,
				"api", "GET", "/open-apis/test/v1/ping", "--as", "user")
			assertSuccess(t, apiResult)
			var apiPayload struct {
				Code int `json:"code"`
				Data struct {
					Pong bool `json:"pong"`
				} `json:"data"`
			}
			if err := json.Unmarshal([]byte(apiResult.stdout), &apiPayload); err != nil {
				t.Fatalf("decode raw API response: %v\n%s", err, apiResult.stdout)
			}
			if apiPayload.Code != 0 || !apiPayload.Data.Pong {
				t.Fatalf("raw API response = %#v", apiPayload)
			}

			const outputName = "recording.mp3"
			outputPath := filepath.Join(configDir, outputName)
			fileResult := runCLI(t, cliPath, configDir, caPath,
				"minutes", "+download",
				"--minute-tokens", e2eMinuteToken,
				"--output", outputName,
				"--as", "user")
			assertSuccess(t, fileResult)
			data, err := vfs.ReadFile(outputPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != e2eMedia {
				t.Fatalf("downloaded media = %q, want %q", data, e2eMedia)
			}
			assertNoCredentialLeak(t, scopeResult, apiResult, fileResult)
			proxy.assertComplete(t)
			if tc.mode == "credential_proxy" {
				assertHelperRequest(t, capture, tc.mode, "proxy_access_token", "user", proxy.server.URL)
			} else {
				assertHelperNotInvoked(t, capture)
			}
		})
	}
}

func buildBinary(t *testing.T, repo, output string, args ...string) {
	t.Helper()
	buildArgs := append([]string{"build", "-o", output}, args...)
	goBinary := filepath.Join(runtime.GOROOT(), "bin", executableName("go"))
	cmd := exec.Command(goBinary, buildArgs...)
	cmd.Dir = repo
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", output, err, data)
	}
}

func writeRootCAOverlay(t *testing.T, repo string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "overlay.json")
	writeJSON(t, path, map[string]any{
		"Replace": map[string]string{
			filepath.Join(repo, "externalcredential_e2e_rootca.go"): filepath.Join(
				repo, "tests", "externalcredential_e2e", "testdata", "rootca", "main.go",
			),
		},
	})
	return path
}

func writeExternalConfiguration(t *testing.T, helperPath, mode, endpoint string) (string, string) {
	t.Helper()
	configDir := t.TempDir()
	systemPath := filepath.Join(configDir, "external-credential.json")
	capturePath := filepath.Join(configDir, "helper-requests.ndjson")
	writeJSON(t, filepath.Join(configDir, "config.json"), map[string]any{
		"currentApp": "sandbox",
		"apps": []any{map[string]any{
			"name":      "sandbox",
			"appId":     e2eAppID,
			"brand":     "feishu",
			"defaultAs": "user",
			"users":     []any{},
		}},
	})

	system := map[string]any{
		"version":      1,
		"mode":         mode,
		"applications": []any{map[string]any{"brand": "feishu", "appId": e2eAppID}},
	}
	if endpoint != "" {
		system["remoteEndpoint"] = endpoint
	}
	if mode != "platform_proxy" {
		helper, err := vfs.ReadFile(helperPath)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(helper)
		system["program"] = map[string]any{
			"executable":      helperPath,
			"arguments":       []string{"--capture", capturePath},
			"sha256":          "sha256:" + hex.EncodeToString(sum[:]),
			"protocolVersion": 1,
			"timeoutSeconds":  5,
		}
	}
	writeJSON(t, systemPath, system)
	return configDir, capturePath
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := vfs.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeServerCA(t *testing.T, server *httptest.Server) string {
	t.Helper()
	cert, err := x509.ParseCertificate(server.Certificate().Raw)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "proxy-ca.pem")
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	if err := vfs.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func runCLI(t *testing.T, binary, configDir, caPath string, args ...string) commandResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = configDir
	cmd.Env = cleanCLIEnvironment(os.Environ(), configDir, caPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("run lark-cli: %v", err)
		}
	}
	return commandResult{exitCode: exitCode, stdout: stdout.String(), stderr: stderr.String()}
}

func cleanCLIEnvironment(base []string, configDir, caPath string) []string {
	env := make([]string, 0, len(base)+11)
	for _, item := range base {
		name, _, _ := strings.Cut(item, "=")
		upper := strings.ToUpper(name)
		if strings.HasPrefix(upper, "LARKSUITE_CLI_") ||
			strings.HasPrefix(upper, "LARK_CLI_") ||
			upper == "GODEBUG" ||
			upper == "SSL_CERT_FILE" ||
			upper == "SSL_CERT_DIR" ||
			upper == "HTTP_PROXY" ||
			upper == "HTTPS_PROXY" ||
			upper == "ALL_PROXY" ||
			upper == "NO_PROXY" {
			continue
		}
		env = append(env, item)
	}
	env = append(env,
		"LARKSUITE_CLI_CONFIG_DIR="+configDir,
		"LARKSUITE_CLI_EXTERNAL_CREDENTIAL_CONFIG="+filepath.Join(configDir, "external-credential.json"),
		"LARKSUITE_CLI_REMOTE_META=off",
		"LARKSUITE_CLI_NO_UPDATE_NOTIFIER=1",
		"LARKSUITE_CLI_NO_SKILLS_NOTIFIER=1",
		"LARKSUITE_CLI_E2E_CA_PATH="+caPath,
		"GODEBUG=x509usefallbackroots=1",
		"HTTP_PROXY=",
		"HTTPS_PROXY=",
		"ALL_PROXY=",
		"NO_PROXY=",
	)
	return env
}

func assertSuccess(t *testing.T, result commandResult) {
	t.Helper()
	if result.exitCode != 0 {
		t.Fatalf("lark-cli exit = %d; stdout=%s stderr=%s", result.exitCode, result.stdout, result.stderr)
	}
}

func assertTypedScopeUnavailable(t *testing.T, result commandResult) {
	t.Helper()
	if result.exitCode == 0 {
		t.Fatalf("proxy auth check unexpectedly succeeded: stdout=%s stderr=%s", result.stdout, result.stderr)
	}
	if strings.TrimSpace(result.stdout) != "" {
		t.Fatalf("proxy auth check wrote data to stdout: %s", result.stdout)
	}
	var payload struct {
		OK    bool `json:"ok"`
		Error struct {
			Type    string `json:"type"`
			Subtype string `json:"subtype"`
			Param   string `json:"param"`
			Hint    string `json:"hint"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(result.stderr), &payload); err != nil {
		t.Fatalf("decode proxy auth-check error: %v\n%s", err, result.stderr)
	}
	if payload.OK || payload.Error.Type != "validation" ||
		payload.Error.Subtype != "failed_precondition" ||
		payload.Error.Param != "" || payload.Error.Hint == "" {
		t.Fatalf("proxy auth-check error = %#v, want typed validation/failed_precondition with hint and no param", payload)
	}
}

func assertNoCredentialLeak(t *testing.T, results ...commandResult) {
	t.Helper()
	for i, result := range results {
		combined := result.stdout + result.stderr
		for _, secret := range []string{e2eDirectToken, e2eProxyToken} {
			if strings.Contains(combined, secret) {
				t.Fatalf("command result %d exposed credential %q: stdout=%s stderr=%s",
					i, secret, result.stdout, result.stderr)
			}
		}
	}
}

func assertHelperRequest(t *testing.T, capture, mode, credentialType, identity, remoteEndpoint string) {
	t.Helper()
	data, err := vfs.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte{'\n'})
	if len(lines) == 0 {
		t.Fatal("credential helper was not invoked")
	}
	for i, line := range lines {
		var req struct {
			Version        int    `json:"version"`
			Mode           string `json:"mode"`
			CredentialType string `json:"credential_type"`
			AppID          string `json:"app_id"`
			Identity       string `json:"identity"`
			RemoteEndpoint string `json:"remote_endpoint"`
		}
		if err := json.Unmarshal(line, &req); err != nil {
			t.Fatalf("decode helper request %d: %v", i, err)
		}
		if req.Version != 1 || req.Mode != mode || req.CredentialType != credentialType ||
			req.AppID != e2eAppID || req.Identity != identity || req.RemoteEndpoint != remoteEndpoint {
			t.Fatalf("helper request %d = %#v", i, req)
		}
	}
}

func assertHelperNotInvoked(t *testing.T, capture string) {
	t.Helper()
	data, err := vfs.ReadFile(capture)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		t.Fatalf("inspect helper capture: %v", err)
	}
	if len(bytes.TrimSpace(data)) != 0 {
		t.Fatalf("platform_proxy unexpectedly invoked helper: %s", data)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	data, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(data))
}

func executableName(base string) string {
	if filepath.Ext(os.Args[0]) == ".exe" {
		return base + ".exe"
	}
	return base
}
