// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package keylesshelper invokes an external lark-keyless-signer-compatible
// helper configured by environment or config.json.
package keylesshelper

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/keysigner"
)

const helperOutputLimit = 1 << 20

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

type runner func(context.Context, []string, request) (response, error)

var run runner = runCommand

// Command is one resolved helper command. It pins the environment/config
// snapshot used by an operation so a concurrent atomic config replacement
// cannot switch signing backends between the configured check and execution.
type Command struct {
	argv []string
}

// ConfiguredFromEnvironment reports whether the current process explicitly
// supplied an external helper command. Config init uses this distinction
// because it removes a config-only command when the environment is absent.
func ConfiguredFromEnvironment() bool {
	return strings.TrimSpace(os.Getenv(envvars.CliKeylessSignerCmd)) != ""
}

// ValidateConfigured reports whether the configured helper command can be parsed.
func ValidateConfigured() error {
	helper, err := Resolve()
	if err != nil {
		return err
	}
	if helper == nil {
		return fmt.Errorf("%s is not set", envvars.CliKeylessSignerCmd)
	}
	return nil
}

// Resolve loads and validates the external helper once. A nil command and nil
// error means neither the environment nor config.json configured a helper.
// Missing config is treated as unconfigured; malformed or unreadable config
// fails closed with the config source and path preserved in the error.
func Resolve() (*Command, error) {
	raw, source, err := configuredCommand()
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, nil
	}
	argv, err := parseCommandFromSource(raw, source)
	if err != nil {
		return nil, err
	}
	return &Command{argv: argv}, nil
}

// Probe asks this resolved helper for its public key.
func (c *Command) Probe(ctx context.Context, keyRef string) error {
	resp, err := run(ctx, c.argv, request{
		Op:     "pubkey",
		KeyRef: defaultKeyRef(keyRef),
	})
	if err != nil {
		return err
	}
	return validateResponse(resp)
}

// SignAttestation asks the helper to mint a registration attestation JWT.
func SignAttestation(ctx context.Context, keyRef, nonce string) (string, error) {
	helper, err := Resolve()
	if err != nil {
		return "", err
	}
	if helper == nil {
		return "", fmt.Errorf("%s is not set", envvars.CliKeylessSignerCmd)
	}
	return helper.SignAttestation(ctx, keyRef, nonce)
}

// SignAttestation asks this resolved helper to mint a registration attestation JWT.
func (c *Command) SignAttestation(ctx context.Context, keyRef, nonce string) (string, error) {
	resp, err := run(ctx, c.argv, request{
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
	resp, err := run(ctx, c.argv, request{
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

func configuredCommand() (string, string, error) {
	if raw := strings.TrimSpace(os.Getenv(envvars.CliKeylessSignerCmd)); raw != "" {
		return raw, envvars.CliKeylessSignerCmd, nil
	}
	config, err := core.LoadMultiAppConfig()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", "config.json keylessSignerCmd", nil
		}
		return "", "config.json keylessSignerCmd", fmt.Errorf("load config.json keylessSignerCmd from %s: %w", core.GetConfigPath(), err)
	}
	if config == nil {
		return "", "config.json keylessSignerCmd", nil
	}
	return strings.TrimSpace(config.KeylessSignerCmd), "config.json keylessSignerCmd", nil
}

// parseCommand accepts either a direct helper path or a JSON argv array. The
// resulting argv is executed directly, never through a shell.
func parseCommand(raw string) ([]string, error) {
	return parseCommandFromSource(raw, envvars.CliKeylessSignerCmd)
}

func parseCommandFromSource(raw, source string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("%s is not set", source)
	}
	if strings.HasPrefix(raw, "[") {
		var argv []string
		if err := json.Unmarshal([]byte(raw), &argv); err != nil {
			return nil, fmt.Errorf("parse %s JSON argv: %w", source, err)
		}
		return validateArgv(argv, source)
	}
	return validateArgv([]string{raw}, source)
}

func validateArgv(argv []string, source string) ([]string, error) {
	if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
		return nil, fmt.Errorf("%s must name a helper binary", source)
	}
	for i, arg := range argv {
		if arg == "" {
			return nil, fmt.Errorf("%s argv[%d] is empty", source, i)
		}
	}
	return argv, nil
}

func runCommand(ctx context.Context, argv []string, req request) (response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return response{}, fmt.Errorf("marshal keyless helper request: %w", err)
	}
	body = append(body, '\n')

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stdin = bytes.NewReader(body)
	stdout := &limitedBuffer{limit: helperOutputLimit}
	stderr := &limitedBuffer{limit: helperOutputLimit}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	runErr := cmd.Run()
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

func helperRunError(runErr error, stderr string) error {
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
