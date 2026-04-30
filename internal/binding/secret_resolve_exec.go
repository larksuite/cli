// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package binding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"
)

// execRequest is the JSON payload sent to exec provider's stdin.
type execRequest struct {
	ProtocolVersion int      `json:"protocolVersion"`
	Provider        string   `json:"provider"`
	IDs             []string `json:"ids"`
}

// execResponse is the JSON payload expected from exec provider's stdout.
type execResponse struct {
	ProtocolVersion int                     `json:"protocolVersion"`
	Values          map[string]interface{}  `json:"values"`
	Errors          map[string]execRefError `json:"errors,omitempty"`
}

// execRefError is an optional per-id error in exec provider response.
type execRefError struct {
	Message string `json:"message"`
}

// execRun bundles everything runExecCommand needs to spawn the child process.
// It is populated once by prepareExecRun and consumed exactly once by
// runExecCommand; keeping the two stages pure data + pure side effect makes
// each independently testable.
type execRun struct {
	Path    string        // absolute, already-audited path to the command
	Args    []string      // command arguments (from pc.Args)
	Env     []string      // minimal child env (passEnv + explicit env only)
	Request []byte        // JSON payload to feed on the child's stdin
	Timeout time.Duration // spawn deadline
	MaxOut  int           // hard cap on stdout size, enforced post-Run
}

// resolveExecRef handles {source:"exec"} SecretRef resolution. It audits the
// command path, runs the child under a timeout with a hard stdout cap, and
// extracts the secret from the JSON response. providerName is the caller-
// resolved effective alias (honours secrets.defaults.exec from openclaw.json).
func resolveExecRef(ref *SecretRef, providerName string, pc *ProviderConfig, getenv func(string) string) (string, error) {
	prep, err := prepareExecRun(ref, providerName, pc, getenv)
	if err != nil {
		return "", err
	}
	stdout, err := runExecCommand(prep)
	if err != nil {
		return "", err
	}
	return extractExecSecret(stdout, ref.ID, effectiveJSONOnly(pc))
}

// prepareExecRun audits the command path, marshals the JSON request,
// assembles the minimal child env, and resolves timeout / output limits.
// Never spawns a process — the returned execRun is pure data.
func prepareExecRun(ref *SecretRef, providerName string, pc *ProviderConfig, getenv func(string) string) (*execRun, error) {
	if pc.Command == "" {
		return nil, fmt.Errorf("exec provider command is empty")
	}

	securePath, err := AssertSecurePath(AuditParams{
		TargetPath:            pc.Command,
		Label:                 "exec provider command",
		TrustedDirs:           pc.TrustedDirs,
		AllowInsecurePath:     pc.AllowInsecurePath,
		AllowReadableByOthers: true, // exec commands are typically 755
		AllowSymlinkPath:      pc.AllowSymlinkCommand,
	})
	if err != nil {
		return nil, fmt.Errorf("exec provider security audit failed: %w", err)
	}

	reqJSON, err := marshalExecRequest(ref, providerName)
	if err != nil {
		return nil, err
	}

	timeoutMs, maxOut := effectiveExecLimits(pc)
	return &execRun{
		Path:    securePath,
		Args:    pc.Args,
		Env:     buildExecEnv(pc, getenv),
		Request: reqJSON,
		Timeout: time.Duration(timeoutMs) * time.Millisecond,
		MaxOut:  maxOut,
	}, nil
}

// marshalExecRequest encodes the JSON protocol request sent to the child.
// providerName is supplied by resolveSecretRef after consulting
// secrets.defaults.exec; an empty value falls back to DefaultProviderAlias
// so the function can still be reasoned about in isolation.
func marshalExecRequest(ref *SecretRef, providerName string) ([]byte, error) {
	if providerName == "" {
		providerName = DefaultProviderAlias
	}
	data, err := json.Marshal(execRequest{
		ProtocolVersion: 1,
		Provider:        providerName,
		IDs:             []string{ref.ID},
	})
	if err != nil {
		return nil, fmt.Errorf("exec provider: failed to marshal request: %w", err)
	}
	return data, nil
}

// buildExecEnv assembles the child's environment: only variables listed in
// pc.PassEnv (and non-empty in the parent) plus pc.Env entries. The child
// never inherits the full parent env — always set cmd.Env explicitly.
func buildExecEnv(pc *ProviderConfig, getenv func(string) string) []string {
	env := make([]string, 0, len(pc.PassEnv)+len(pc.Env))
	for _, key := range pc.PassEnv {
		if val := getenv(key); val != "" {
			env = append(env, key+"="+val)
		}
	}
	for key, val := range pc.Env {
		env = append(env, key+"="+val)
	}
	return env
}

// effectiveExecLimits returns (timeoutMs, maxOutputBytes), falling back to
// package defaults for any non-positive value. The exec provider uses its
// own NoOutputTimeoutMs field (pc.TimeoutMs is the file-provider field and
// should not be consulted here); the value is applied as the overall
// deadline for the child process.
func effectiveExecLimits(pc *ProviderConfig) (timeoutMs, maxOutputBytes int) {
	timeoutMs = pc.NoOutputTimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = DefaultExecTimeoutMs
	}
	maxOutputBytes = pc.MaxOutputBytes
	if maxOutputBytes <= 0 {
		maxOutputBytes = DefaultExecMaxOutputBytes
	}
	return timeoutMs, maxOutputBytes
}

// effectiveJSONOnly returns pc.JSONOnly or its documented default (true).
func effectiveJSONOnly(pc *ProviderConfig) bool {
	if pc.JSONOnly != nil {
		return *pc.JSONOnly
	}
	return true
}

// runExecCommand spawns the child per prep, feeds prep.Request on stdin, and
// returns trimmed stdout on success. Failure modes:
//   - timeout → typed error with the configured limit
//   - non-zero exit → wrapped *exec.ExitError
//   - stdout exceeds prep.MaxOut → typed error (size enforced post-Run)
//   - empty trimmed stdout → typed error
func runExecCommand(prep *execRun) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), prep.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, prep.Path, prep.Args...)
	cmd.Dir = filepath.Dir(prep.Path)
	cmd.Env = prep.Env // always set — leaving nil would inherit the parent env
	cmd.Stdin = bytes.NewReader(prep.Request)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("exec provider timed out after %dms", int(prep.Timeout/time.Millisecond))
		}
		return nil, fmt.Errorf("exec provider exited with error: %w", err)
	}

	if stdout.Len() > prep.MaxOut {
		return nil, fmt.Errorf("exec provider output exceeded maxOutputBytes (%d)", prep.MaxOut)
	}

	trimmed := bytes.TrimSpace(stdout.Bytes())
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("exec provider returned empty stdout")
	}
	return trimmed, nil
}

// extractExecSecret parses stdout as a JSON execResponse and returns the
// string value at refID. When jsonOnly is false and the response is not valid
// JSON (or the value is not a string), it falls back to the raw stdout or the
// JSON encoding of the value respectively — mirroring OpenClaw's resolve.ts.
func extractExecSecret(stdout []byte, refID string, jsonOnly bool) (string, error) {
	var resp execResponse
	if err := json.Unmarshal(stdout, &resp); err != nil {
		if !jsonOnly {
			return string(stdout), nil
		}
		return "", fmt.Errorf("exec provider returned invalid JSON: %w", err)
	}

	if resp.ProtocolVersion != 1 {
		return "", fmt.Errorf("exec provider protocolVersion must be 1, got %d", resp.ProtocolVersion)
	}

	if refErr, ok := resp.Errors[refID]; ok {
		msg := refErr.Message
		if msg == "" {
			msg = "unknown error"
		}
		return "", fmt.Errorf("exec provider failed for id %q: %s", refID, msg)
	}

	if resp.Values == nil {
		return "", fmt.Errorf("exec provider response missing 'values'")
	}
	value, ok := resp.Values[refID]
	if !ok {
		return "", fmt.Errorf("exec provider response missing id %q", refID)
	}

	if str, ok := value.(string); ok {
		return str, nil
	}
	if !jsonOnly {
		data, err := json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("exec provider value for id %q is not JSON-serializable: %w", refID, err)
		}
		return string(data), nil
	}
	return "", fmt.Errorf("exec provider value for id %q is not a string", refID)
}
