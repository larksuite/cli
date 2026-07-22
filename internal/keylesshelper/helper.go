// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package keylesshelper invokes a signer generation that has already been
// resolved and verified by internal/keylessprovider.
package keylesshelper

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/keysigner"
	"github.com/larksuite/cli/internal/vfs"
)

const (
	helperOutputLimit      = 1 << 20
	helperStderrLimit      = 64 << 10
	helperExecutionTimeout = 10 * time.Second
)

type request struct {
	Op       string `json:"op"`
	KeyRef   string `json:"keyRef,omitempty"`
	Nonce    string `json:"nonce,omitempty"`
	ClientID string `json:"clientId,omitempty"`
	Audience string `json:"aud,omitempty"`
}

type response struct {
	OK                  bool           `json:"ok"`
	Error               *protocolError `json:"error,omitempty"`
	Attestation         string         `json:"attestation,omitempty"`
	ClientAssertionType string         `json:"client_assertion_type,omitempty"`
	ClientAssertion     string         `json:"client_assertion,omitempty"`
}

type protocolError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Command is one verified provider executable. It is intentionally impossible
// to construct from an app-config path, argv, or environment variable.
type Command struct {
	argv         []string
	providerCWD  string
	providerHome string
	providerSHA  string
}

// NewProviderCommand builds the fixed empty-argv command used by a verified
// provider executable. Provider execution never accepts argv from app config or
// the environment.
func NewProviderCommand(binaryPath, providerRoot, signerHome, expectedSHA256 string) (*Command, error) {
	if strings.TrimSpace(binaryPath) == "" || strings.TrimSpace(providerRoot) == "" {
		return nil, fmt.Errorf("provider binary path and root must be non-empty")
	}
	if len(expectedSHA256) != 64 {
		return nil, fmt.Errorf("provider binary digest must be a SHA-256 hex string")
	}
	return &Command{argv: []string{binaryPath}, providerCWD: providerRoot, providerHome: signerHome, providerSHA: expectedSHA256}, nil
}

// Probe asks this resolved helper for its public key.
func (c *Command) Probe(ctx context.Context, keyRef string) error {
	resp, err := c.execute(ctx, request{
		Op:     "pubkey",
		KeyRef: defaultKeyRef(keyRef),
	})
	if err != nil {
		return err
	}
	return validateResponse(resp)
}

// SignAttestation asks this resolved helper to mint a registration attestation JWT.
func (c *Command) SignAttestation(ctx context.Context, keyRef, nonce string) (string, error) {
	resp, err := c.execute(ctx, request{
		Op:     "sign-attestation",
		KeyRef: defaultKeyRef(keyRef),
		Nonce:  nonce,
	})
	if err != nil {
		return "", err
	}
	if err := validateResponse(resp); err != nil {
		return "", err
	}
	if resp.Attestation == "" {
		return "", fmt.Errorf("keyless helper returned empty attestation")
	}
	return resp.Attestation, nil
}

// SignClientAssertion asks this resolved helper to mint a token-endpoint client_assertion.
func (c *Command) SignClientAssertion(ctx context.Context, keyRef, clientID, audience string) (string, string, error) {
	resp, err := c.execute(ctx, request{
		Op:       "sign-assertion",
		KeyRef:   defaultKeyRef(keyRef),
		ClientID: clientID,
		Audience: audience,
	})
	if err != nil {
		return "", "", err
	}
	if err := validateResponse(resp); err != nil {
		return "", "", err
	}
	if resp.ClientAssertionType == "" {
		return "", "", fmt.Errorf("keyless helper returned empty client_assertion_type")
	}
	if resp.ClientAssertion == "" {
		return "", "", fmt.Errorf("keyless helper returned empty client_assertion")
	}
	return resp.ClientAssertionType, resp.ClientAssertion, nil
}

func (c *Command) execute(ctx context.Context, req request) (response, error) {
	if err := verifyProviderBinary(c.argv[0], c.providerSHA); err != nil {
		return response{}, err
	}
	return runCommandConfigured(ctx, c.argv, req, c.providerCWD, providerEnvironment(c.providerHome))
}

func verifyProviderBinary(path, expectedSHA string) error {
	info, err := vfs.Lstat(path)
	if err != nil {
		return fmt.Errorf("recheck provider signer: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() <= 0 || info.Size() > 512<<20 {
		return fmt.Errorf("provider signer changed before execution")
	}
	f, err := vfs.Open(path)
	if err != nil {
		return fmt.Errorf("reopen provider signer: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, 512<<20+1))
	if err != nil || n != info.Size() {
		return fmt.Errorf("rehash provider signer: file changed while reading")
	}
	if hex.EncodeToString(h.Sum(nil)) != expectedSHA {
		return fmt.Errorf("provider signer digest changed before execution")
	}
	return nil
}

func validateResponse(resp response) error {
	if resp.Error != nil {
		return fmt.Errorf("keyless helper %s: %s", resp.Error.Type, resp.Error.Message)
	}
	if !resp.OK {
		return fmt.Errorf("keyless helper returned ok=false")
	}
	return nil
}

func defaultKeyRef(keyRef string) string {
	if keyRef != "" {
		return keyRef
	}
	return keysigner.DefaultKeyLabel
}

func runCommand(ctx context.Context, argv []string, req request) (response, error) {
	return runCommandConfigured(ctx, argv, req, "", nil)
}

func runCommandConfigured(ctx context.Context, argv []string, req request, cwd string, env []string) (response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return response{}, fmt.Errorf("marshal keyless helper request: %w", err)
	}
	body = append(body, '\n')

	helperCtx, cancel := withExecutionTimeout(ctx)
	defer cancel()

	// CommandContext's default cancellation kills the helper process. This is
	// important for unattended agent calls: a signer blocked on platform UI must
	// not hold the caller indefinitely.
	cmd := exec.CommandContext(helperCtx, argv[0], argv[1:]...)
	if cwd != "" {
		cmd.Dir = cwd
		cmd.Env = env
	}
	cmd.Stdin = bytes.NewReader(body)
	stdout := &limitedBuffer{limit: helperOutputLimit}
	stderr := &limitedBuffer{limit: helperStderrLimit}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	runErr := cmd.Run()
	if err := helperCtx.Err(); err != nil {
		// Never parse or include helper output on cancellation. A partially written
		// response may contain a client assertion or other credential material.
		if errors.Is(err, context.DeadlineExceeded) {
			return response{}, fmt.Errorf("keyless helper timed out: %w", context.DeadlineExceeded)
		}
		return response{}, fmt.Errorf("keyless helper canceled: %w", err)
	}
	var resp response
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		if runErr != nil {
			return response{}, helperRunError(runErr, stderr.String())
		}
		return response{}, fmt.Errorf("keyless helper produced invalid JSON: %w", err)
	}
	if runErr != nil && resp.Error == nil {
		return response{}, helperRunError(runErr, stderr.String())
	}
	return resp, nil
}

func providerEnvironment(homeOverride string) []string {
	// Signer implementations use OS facilities and must not inherit language
	// runtime/proxy/library injection variables. HOME/TMPDIR/SystemRoot are the
	// minimal cross-platform values currently needed by supported backends.
	keep := map[string]bool{"HOME": true, "TMPDIR": true, "TEMP": true, "TMP": true, "SystemRoot": true, "WINDIR": true}
	var env []string
	for _, entry := range os.Environ() {
		name := entry
		if idx := strings.IndexByte(entry, '='); idx >= 0 {
			name = entry[:idx]
		}
		if keep[name] && !(name == "HOME" && homeOverride != "") {
			env = append(env, entry)
		}
	}
	if homeOverride != "" {
		env = append(env, "HOME="+homeOverride)
	}
	return env
}

func withExecutionTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, helperExecutionTimeout)
}

func helperRunError(runErr error, stderr string) error {
	if errors.Is(runErr, os.ErrNotExist) {
		return fmt.Errorf("keyless helper executable no longer exists; repair or reinstall the OpenClaw Feishu plugin: %w", runErr)
	}
	if strings.TrimSpace(stderr) != "" {
		return fmt.Errorf("keyless helper failed: %w (stderr omitted)", runErr)
	}
	return fmt.Errorf("keyless helper failed: %w", runErr)
}

type limitedBuffer struct {
	buf   bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		return len(p), nil
	}
	remaining := b.limit - b.buf.Len()
	if remaining > 0 {
		if len(p) < remaining {
			remaining = len(p)
		}
		_, _ = b.buf.Write(p[:remaining])
	}
	return len(p), nil
}

func (b *limitedBuffer) Bytes() []byte {
	return b.buf.Bytes()
}

func (b *limitedBuffer) String() string {
	return b.buf.String()
}
