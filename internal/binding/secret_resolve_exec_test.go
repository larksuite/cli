// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package binding

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// writeExecHelper writes a small shell script that mimics an exec provider.
// The script reads stdin (the JSON request) and writes a JSON response to stdout.
func writeExecHelper(t *testing.T, dir, body string) string {
	t.Helper()
	p := filepath.Join(dir, "helper.sh")
	script := "#!/bin/sh\n" + body
	if err := os.WriteFile(p, []byte(script), 0o700); err != nil {
		t.Fatalf("write helper script: %v", err)
	}
	return p
}

func TestResolveExecRef_EmptyCommand(t *testing.T) {
	ref := &SecretRef{Source: "exec", ID: "MY_KEY"}
	pc := &ProviderConfig{Source: "exec", Command: ""}

	_, err := resolveExecRef(ref, "", pc, nil)
	if err == nil {
		t.Fatal("expected error for empty command, got nil")
	}
	want := "exec provider command is empty"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestResolveExecRef_CommandNotFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("path audit not applicable on Windows")
	}

	ref := &SecretRef{Source: "exec", ID: "MY_KEY"}
	pc := &ProviderConfig{
		Source:            "exec",
		Command:           "/nonexistent/command",
		AllowInsecurePath: true,
	}

	_, err := resolveExecRef(ref, "", pc, nil)
	if err == nil {
		t.Fatal("expected error for nonexistent command, got nil")
	}
}

func TestResolveExecRef_JSONResponse(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not applicable on Windows")
	}

	dir := t.TempDir()
	// Script reads stdin (ignores), writes valid JSON response
	helper := writeExecHelper(t, dir, `cat > /dev/null
printf '{"protocolVersion":1,"values":{"MY_KEY":"exec_secret_123"}}'
`)

	ref := &SecretRef{Source: "exec", Provider: "default", ID: "MY_KEY"}
	pc := &ProviderConfig{
		Source:            "exec",
		Command:           helper,
		AllowInsecurePath: true,
	}

	got, err := resolveExecRef(ref, "", pc, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "exec_secret_123" {
		t.Errorf("got %q, want %q", got, "exec_secret_123")
	}
}

func TestResolveExecRef_PerRefError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not applicable on Windows")
	}

	dir := t.TempDir()
	helper := writeExecHelper(t, dir, `cat > /dev/null
printf '{"protocolVersion":1,"values":{},"errors":{"MY_KEY":{"message":"secret not found"}}}'
`)

	ref := &SecretRef{Source: "exec", Provider: "default", ID: "MY_KEY"}
	pc := &ProviderConfig{
		Source:            "exec",
		Command:           helper,
		AllowInsecurePath: true,
	}

	_, err := resolveExecRef(ref, "", pc, nil)
	if err == nil {
		t.Fatal("expected error for per-ref error, got nil")
	}
	want := `exec provider failed for id "MY_KEY": secret not found`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestResolveExecRef_WrongProtocolVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not applicable on Windows")
	}

	dir := t.TempDir()
	helper := writeExecHelper(t, dir, `cat > /dev/null
printf '{"protocolVersion":99,"values":{"MY_KEY":"v"}}'
`)

	ref := &SecretRef{Source: "exec", Provider: "default", ID: "MY_KEY"}
	pc := &ProviderConfig{
		Source:            "exec",
		Command:           helper,
		AllowInsecurePath: true,
	}

	_, err := resolveExecRef(ref, "", pc, nil)
	if err == nil {
		t.Fatal("expected error for wrong protocol version, got nil")
	}
	want := "exec provider protocolVersion must be 1, got 99"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestResolveExecRef_MissingValues(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not applicable on Windows")
	}

	dir := t.TempDir()
	helper := writeExecHelper(t, dir, `cat > /dev/null
printf '{"protocolVersion":1}'
`)

	ref := &SecretRef{Source: "exec", Provider: "default", ID: "MY_KEY"}
	pc := &ProviderConfig{
		Source:            "exec",
		Command:           helper,
		AllowInsecurePath: true,
	}

	_, err := resolveExecRef(ref, "", pc, nil)
	if err == nil {
		t.Fatal("expected error for missing values, got nil")
	}
	want := "exec provider response missing 'values'"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestResolveExecRef_MissingID(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not applicable on Windows")
	}

	dir := t.TempDir()
	helper := writeExecHelper(t, dir, `cat > /dev/null
printf '{"protocolVersion":1,"values":{"OTHER":"val"}}'
`)

	ref := &SecretRef{Source: "exec", Provider: "default", ID: "MY_KEY"}
	pc := &ProviderConfig{
		Source:            "exec",
		Command:           helper,
		AllowInsecurePath: true,
	}

	_, err := resolveExecRef(ref, "", pc, nil)
	if err == nil {
		t.Fatal("expected error for missing ID, got nil")
	}
	want := `exec provider response missing id "MY_KEY"`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestResolveExecRef_EmptyStdout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not applicable on Windows")
	}

	dir := t.TempDir()
	helper := writeExecHelper(t, dir, `cat > /dev/null
`)

	ref := &SecretRef{Source: "exec", Provider: "default", ID: "MY_KEY"}
	pc := &ProviderConfig{
		Source:            "exec",
		Command:           helper,
		AllowInsecurePath: true,
	}

	_, err := resolveExecRef(ref, "", pc, nil)
	if err == nil {
		t.Fatal("expected error for empty stdout, got nil")
	}
	want := "exec provider returned empty stdout"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestResolveExecRef_InvalidJSON_JSONOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not applicable on Windows")
	}

	dir := t.TempDir()
	helper := writeExecHelper(t, dir, `cat > /dev/null
echo "not json"
`)

	ref := &SecretRef{Source: "exec", Provider: "default", ID: "MY_KEY"}
	pc := &ProviderConfig{
		Source:            "exec",
		Command:           helper,
		AllowInsecurePath: true,
		// JSONOnly defaults to true (nil)
	}

	_, err := resolveExecRef(ref, "", pc, nil)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestResolveExecRef_NonJSON_RawString(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not applicable on Windows")
	}

	dir := t.TempDir()
	helper := writeExecHelper(t, dir, `cat > /dev/null
echo "raw_secret_value"
`)

	jsonOnly := false
	ref := &SecretRef{Source: "exec", Provider: "default", ID: "MY_KEY"}
	pc := &ProviderConfig{
		Source:            "exec",
		Command:           helper,
		AllowInsecurePath: true,
		JSONOnly:          &jsonOnly,
	}

	got, err := resolveExecRef(ref, "", pc, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "raw_secret_value" {
		t.Errorf("got %q, want %q", got, "raw_secret_value")
	}
}

func TestResolveExecRef_NonStringValue_JSONOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not applicable on Windows")
	}

	dir := t.TempDir()
	helper := writeExecHelper(t, dir, `cat > /dev/null
printf '{"protocolVersion":1,"values":{"MY_KEY":42}}'
`)

	ref := &SecretRef{Source: "exec", Provider: "default", ID: "MY_KEY"}
	pc := &ProviderConfig{
		Source:            "exec",
		Command:           helper,
		AllowInsecurePath: true,
	}

	_, err := resolveExecRef(ref, "", pc, nil)
	if err == nil {
		t.Fatal("expected error for non-string value with jsonOnly=true, got nil")
	}
	want := `exec provider value for id "MY_KEY" is not a string`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestResolveExecRef_NonStringValue_NoJSONOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not applicable on Windows")
	}

	dir := t.TempDir()
	helper := writeExecHelper(t, dir, `cat > /dev/null
printf '{"protocolVersion":1,"values":{"MY_KEY":42}}'
`)

	jsonOnly := false
	ref := &SecretRef{Source: "exec", Provider: "default", ID: "MY_KEY"}
	pc := &ProviderConfig{
		Source:            "exec",
		Command:           helper,
		AllowInsecurePath: true,
		JSONOnly:          &jsonOnly,
	}

	got, err := resolveExecRef(ref, "", pc, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "42" {
		t.Errorf("got %q, want %q", got, "42")
	}
}

func TestResolveExecRef_CommandExitError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not applicable on Windows")
	}

	dir := t.TempDir()
	helper := writeExecHelper(t, dir, `exit 1
`)

	ref := &SecretRef{Source: "exec", Provider: "default", ID: "MY_KEY"}
	pc := &ProviderConfig{
		Source:            "exec",
		Command:           helper,
		AllowInsecurePath: true,
	}

	_, err := resolveExecRef(ref, "", pc, nil)
	if err == nil {
		t.Fatal("expected error for command exit error, got nil")
	}
}

func TestResolveExecRef_PassEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not applicable on Windows")
	}

	dir := t.TempDir()
	// Script uses TEST_SECRET env to produce value
	helper := writeExecHelper(t, dir, `cat > /dev/null
printf '{"protocolVersion":1,"values":{"MY_KEY":"%s"}}' "$TEST_SECRET"
`)

	ref := &SecretRef{Source: "exec", Provider: "default", ID: "MY_KEY"}
	pc := &ProviderConfig{
		Source:            "exec",
		Command:           helper,
		AllowInsecurePath: true,
		PassEnv:           []string{"TEST_SECRET"},
	}

	getenv := func(key string) string {
		if key == "TEST_SECRET" {
			return "passed_env_value"
		}
		return ""
	}

	got, err := resolveExecRef(ref, "", pc, getenv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "passed_env_value" {
		t.Errorf("got %q, want %q", got, "passed_env_value")
	}
}

func TestResolveExecRef_ExplicitEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not applicable on Windows")
	}

	dir := t.TempDir()
	helper := writeExecHelper(t, dir, `cat > /dev/null
printf '{"protocolVersion":1,"values":{"MY_KEY":"%s"}}' "$CUSTOM_VAR"
`)

	ref := &SecretRef{Source: "exec", Provider: "default", ID: "MY_KEY"}
	pc := &ProviderConfig{
		Source:            "exec",
		Command:           helper,
		AllowInsecurePath: true,
		Env:               map[string]string{"CUSTOM_VAR": "explicit_value"},
	}

	got, err := resolveExecRef(ref, "", pc, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "explicit_value" {
		t.Errorf("got %q, want %q", got, "explicit_value")
	}
}

func TestResolveExecRef_OutputExceedsMax(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not applicable on Windows")
	}

	dir := t.TempDir()
	// Script outputs more than maxOutputBytes
	helper := writeExecHelper(t, dir, `cat > /dev/null
python3 -c "print('x' * 200)"
`)

	ref := &SecretRef{Source: "exec", Provider: "default", ID: "MY_KEY"}
	pc := &ProviderConfig{
		Source:            "exec",
		Command:           helper,
		AllowInsecurePath: true,
		MaxOutputBytes:    10,
	}

	_, err := resolveExecRef(ref, "", pc, nil)
	if err == nil {
		t.Fatal("expected error for output exceeding maxOutputBytes, got nil")
	}
	want := fmt.Sprintf("exec provider output exceeded maxOutputBytes (%d)", 10)
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}
