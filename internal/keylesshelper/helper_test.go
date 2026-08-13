// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keylesshelper

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const helperProcessMode = "GO_WANT_KEYLESS_PROVIDER_HELPER"

func TestRunCommandProtocol(t *testing.T) {
	t.Setenv(helperProcessMode, "reply")
	resp, err := runCommandConfigured(context.Background(), []string{os.Args[0], "-test.run=^TestHelperProcess$"}, request{
		Op: "sign-assertion", KeyRef: "key-1", ClientID: "cli_a", Audience: "aud",
	}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.ClientAssertion != "helper.jwt" {
		t.Fatalf("response = %#v", resp)
	}
}

func TestRunCommandTimeoutDoesNotLeakOutput(t *testing.T) {
	t.Setenv(helperProcessMode, "hang")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := runCommandConfigured(ctx, []string{os.Args[0], "-test.run=^TestHelperProcess$"}, request{Op: "pubkey", KeyRef: "key-1"}, "", nil)
	if !errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "secret.jwt") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunCommandRejectsOutputBeyondLimitAfterValidJSON(t *testing.T) {
	t.Setenv(helperProcessMode, "overflow")
	_, err := runCommandConfigured(context.Background(), []string{os.Args[0], "-test.run=^TestHelperProcess$"}, request{Op: "pubkey", KeyRef: "key-1"}, "", nil)
	if err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("error = %v, want output limit rejection", err)
	}
}

func TestRunCommandRejectsTrailingJSON(t *testing.T) {
	t.Setenv(helperProcessMode, "trailing")
	_, err := runCommandConfigured(context.Background(), []string{os.Args[0], "-test.run=^TestHelperProcess$"}, request{Op: "pubkey", KeyRef: "key-1"}, "", nil)
	if err == nil || !strings.Contains(err.Error(), "trailing JSON") {
		t.Fatalf("error = %v, want trailing JSON rejection", err)
	}
}

func TestValidateResponseDoesNotExposeHelperMessage(t *testing.T) {
	err := validateResponse(response{Error: &protocolError{Type: "signing_failed", Message: "secret.jwt"}})
	if err == nil || strings.Contains(err.Error(), "secret.jwt") {
		t.Fatalf("error = %v, want fixed redacted message", err)
	}
}

func TestProviderCommandRechecksDigestBeforeSpawn(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "signer")
	if err := os.WriteFile(binary, []byte("first"), 0700); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("first"))
	command, err := NewProviderCommand(binary, root, hex.EncodeToString(digest[:]))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, []byte("second"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := command.Probe(context.Background(), "key-1"); err == nil || !strings.Contains(err.Error(), "digest changed") {
		t.Fatalf("Probe error = %v", err)
	}
}

func TestProviderEnvironmentFiltersUntrustedVariables(t *testing.T) {
	t.Setenv("HOME", "/original")
	t.Setenv("HTTPS_PROXY", "http://untrusted.invalid")
	env := providerEnvironment()
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "HOME=/original") || strings.Contains(joined, "HTTPS_PROXY") {
		t.Fatalf("provider environment = %q", joined)
	}
}

func TestHelperProcess(t *testing.T) {
	switch os.Getenv(helperProcessMode) {
	case "reply":
		var req request
		if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
			os.Exit(2)
		}
		_ = json.NewEncoder(os.Stdout).Encode(response{
			OK: true, ClientAssertionType: "urn:ietf:params:oauth:client-assertion-type:jwt-bearer", ClientAssertion: "helper.jwt",
		})
		os.Exit(0)
	case "hang":
		_, _ = io.WriteString(os.Stdout, `{"ok":true,"client_assertion":"secret.jwt"`)
		for {
			time.Sleep(time.Hour)
		}
	case "overflow":
		_, _ = io.WriteString(os.Stdout, `{"ok":true}`)
		_, _ = io.WriteString(os.Stdout, strings.Repeat("x", helperOutputLimit+1))
		os.Exit(0)
	case "trailing":
		_, _ = io.WriteString(os.Stdout, `{"ok":true}{"ok":true}`)
		os.Exit(0)
	}
}
