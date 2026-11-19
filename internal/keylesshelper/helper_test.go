// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keylesshelper

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/auth/jwt"
)

const helperProcessMode = "GO_WANT_KEYLESS_PROVIDER_HELPER"

func testClientAssertion() string {
	header := base64.RawURLEncoding.EncodeToString([]byte(
		`{"alg":"ES256","kid":"NzbLsXh8uDCcd-6MNwXF4W_7noWXFZAfHkxZsRGC9Xs","typ":"JWT"}`,
	))
	return header + ".e30.c2ln"
}

func runTestHelper(ctx context.Context, mode string, req request) (response, error) {
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestHelperProcess$")
	cmd.Env = append(providerEnvironment(), helperProcessMode+"="+mode)
	return runHelper(ctx, cmd, req)
}

func TestRunCommandProtocol(t *testing.T) {
	resp, err := runTestHelper(context.Background(), "reply", request{
		Op: "sign-assertion", KeyRef: "key-1", ClientID: "cli_a", Audience: "aud",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.ClientAssertion != testClientAssertion() {
		t.Fatalf("response = %#v", resp)
	}
}

func TestRunCommandTimeoutDoesNotLeakOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := runTestHelper(ctx, "hang", request{Op: "pubkey", KeyRef: "key-1"})
	if !errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "secret.jwt") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunCommandRejectsOutputBeyondLimitAfterValidJSON(t *testing.T) {
	_, err := runTestHelper(context.Background(), "overflow", request{Op: "pubkey", KeyRef: "key-1"})
	if err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("error = %v, want output limit rejection", err)
	}
}

func TestRunCommandRejectsTrailingJSON(t *testing.T) {
	_, err := runTestHelper(context.Background(), "trailing", request{Op: "pubkey", KeyRef: "key-1"})
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

func TestProviderCommandRequiresKeyRef(t *testing.T) {
	var command Command
	if err := command.Probe(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "key reference") {
		t.Fatalf("empty probe key: %v", err)
	}
	if _, _, err := command.SignClientAssertion(context.Background(), "", "cli_a", "aud"); err == nil || !strings.Contains(err.Error(), "key reference") {
		t.Fatalf("empty signing key: %v", err)
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
		var req map[string]string
		want := map[string]string{"op": "sign-assertion", "keyRef": "key-1", "clientId": "cli_a", "aud": "aud"}
		if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil || !maps.Equal(req, want) {
			os.Exit(2)
		}
		_ = json.NewEncoder(os.Stdout).Encode(response{
			OK: true, ClientAssertionType: jwt.ClientAssertionType, ClientAssertion: testClientAssertion(),
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
