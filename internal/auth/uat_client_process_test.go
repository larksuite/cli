// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build darwin || linux

package auth

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/core"
)

func TestRefreshRotatesOnceAcrossConfigIsolatedProcesses(t *testing.T) {
	if os.Getenv("LARK_CLI_REFRESH_PROCESS_HELPER") != "" {
		runRefreshProcessHelper(t)
		return
	}

	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "token.json")
	now := time.Now()
	initial := refreshTestToken("expired-access", "single-use-refresh", now.Add(-time.Minute), now.Add(24*time.Hour))
	writeProcessToken(t, storePath, initial)

	var refreshCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		refreshCalls.Add(1)
		time.Sleep(150 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 0, "access_token": "winner-access", "refresh_token": "winner-refresh",
			"expires_in": 3600, "refresh_token_expires_in": 604800,
		})
	}))
	defer server.Close()

	firstResult := filepath.Join(tempDir, "first-result")
	secondResult := filepath.Join(tempDir, "second-result")
	first := refreshProcessCommand(t, filepath.Join(tempDir, "config-a"), tempDir, storePath, firstResult, server.URL)
	second := refreshProcessCommand(t, filepath.Join(tempDir, "config-b"), tempDir, storePath, secondResult, server.URL)
	if err := first.Start(); err != nil {
		t.Fatal(err)
	}
	if err := second.Start(); err != nil {
		_ = first.Process.Kill()
		t.Fatal(err)
	}
	if err := first.Wait(); err != nil {
		t.Fatalf("AC6: first refresh process failed: %v", err)
	}
	if err := second.Wait(); err != nil {
		t.Fatalf("AC6: second refresh process failed: %v", err)
	}

	if refreshCalls.Load() != 1 {
		t.Fatalf("AC6: refresh endpoint calls = %d, want 1", refreshCalls.Load())
	}
	for _, resultPath := range []string{firstResult, secondResult} {
		result, err := os.ReadFile(resultPath)
		if err != nil || string(result) != "winner-access" {
			t.Fatalf("AC6: process result = %q, err=%v", result, err)
		}
	}
	stored := readProcessToken(t, storePath)
	if stored.RefreshToken != "winner-refresh" {
		t.Fatalf("AC6: persisted refresh token = %q, want winner", stored.RefreshToken)
	}
}

func refreshProcessCommand(t *testing.T, configDir, homeDir, storePath, resultPath, serverURL string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestRefreshRotatesOnceAcrossConfigIsolatedProcesses$")
	env := make([]string, 0, len(os.Environ())+6)
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, "HOME=") || strings.HasPrefix(value, "LARKSUITE_CLI_CONFIG_DIR=") ||
			strings.HasPrefix(value, "LARKSUITE_CLI_DATA_DIR=") || strings.HasPrefix(value, "LARK_CLI_REFRESH_PROCESS_") {
			continue
		}
		env = append(env, value)
	}
	cmd.Env = append(env, "HOME="+homeDir, "LARKSUITE_CLI_CONFIG_DIR="+configDir,
		"LARK_CLI_REFRESH_PROCESS_HELPER=1", "LARK_CLI_REFRESH_PROCESS_STORE="+storePath,
		"LARK_CLI_REFRESH_PROCESS_RESULT="+resultPath, "LARK_CLI_REFRESH_PROCESS_SERVER="+serverURL)
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &bytes.Buffer{}
	return cmd
}

func runRefreshProcessHelper(t *testing.T) {
	storePath := os.Getenv("LARK_CLI_REFRESH_PROCESS_STORE")
	loadStoredUAToken = func(string, string) (*StoredUAToken, error) {
		data, err := os.ReadFile(storePath)
		if err != nil {
			return nil, err
		}
		var token StoredUAToken
		return &token, json.Unmarshal(data, &token)
	}
	persistStoredUAToken = func(token *StoredUAToken) error {
		data, err := json.Marshal(token)
		if err != nil {
			return err
		}
		return os.WriteFile(storePath, data, 0600)
	}
	target, err := url.Parse(os.Getenv("LARK_CLI_REFRESH_PROCESS_SERVER"))
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: uatRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		clone := request.Clone(request.Context())
		clone.URL.Scheme = target.Scheme
		clone.URL.Host = target.Host
		return http.DefaultTransport.RoundTrip(clone)
	})}
	token, err := GetValidAccessToken(client, UATCallOptions{
		UserOpenId: "ou_user", AppId: "cli_test", AppSecret: "secret", Domain: core.BrandFeishu, ErrOut: io.Discard,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("LARK_CLI_REFRESH_PROCESS_RESULT"), []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
}

func writeProcessToken(t *testing.T, path string, token *StoredUAToken) {
	t.Helper()
	data, err := json.Marshal(token)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func readProcessToken(t *testing.T, path string) *StoredUAToken {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var token StoredUAToken
	if err := json.Unmarshal(data, &token); err != nil {
		t.Fatal(err)
	}
	return &token
}
