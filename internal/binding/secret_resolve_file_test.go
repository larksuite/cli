// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package binding

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveFileRef_SingleValue(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(p, []byte("my_secret\n"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	ref := &SecretRef{Source: "file", ID: SingleValueFileRefID}
	pc := &ProviderConfig{
		Source:            "file",
		Path:              p,
		Mode:              "singleValue",
		AllowInsecurePath: true,
	}

	got, err := resolveFileRef(ref, pc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "my_secret" {
		t.Errorf("got %q, want %q", got, "my_secret")
	}
}

func TestResolveFileRef_SingleValue_WrongRefID(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(p, []byte("my_secret\n"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	ref := &SecretRef{Source: "file", ID: "WRONG_ID"}
	pc := &ProviderConfig{
		Source:            "file",
		Path:              p,
		Mode:              "singleValue",
		AllowInsecurePath: true,
	}

	_, err := resolveFileRef(ref, pc)
	if err == nil {
		t.Fatal("expected error for wrong ref ID, got nil")
	}
	want := `singleValue file provider expects ref id "$SINGLE_VALUE", got "WRONG_ID"`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestResolveFileRef_JSONMode(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "secrets.json")
	content := `{"providers":{"feishu":{"key":"secret123"}}}`
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	ref := &SecretRef{Source: "file", ID: "/providers/feishu/key"}
	pc := &ProviderConfig{
		Source:            "file",
		Path:              p,
		Mode:              "json",
		AllowInsecurePath: true,
	}

	got, err := resolveFileRef(ref, pc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "secret123" {
		t.Errorf("got %q, want %q", got, "secret123")
	}
}

func TestResolveFileRef_JSONMode_MissingPointer(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "secrets.json")
	content := `{"providers":{"feishu":{"key":"secret123"}}}`
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	ref := &SecretRef{Source: "file", ID: "/providers/nonexistent/key"}
	pc := &ProviderConfig{
		Source:            "file",
		Path:              p,
		Mode:              "json",
		AllowInsecurePath: true,
	}

	_, err := resolveFileRef(ref, pc)
	if err == nil {
		t.Fatal("expected error for missing JSON pointer, got nil")
	}
	want := `file provider JSON Pointer "/providers/nonexistent/key": json pointer "/providers/nonexistent/key": key "nonexistent" not found`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestResolveFileRef_FileNotFound(t *testing.T) {
	nonexistent := filepath.Join(t.TempDir(), "no_such_file.txt")
	ref := &SecretRef{Source: "file", ID: SingleValueFileRefID}
	pc := &ProviderConfig{
		Source:            "file",
		Path:              nonexistent,
		Mode:              "singleValue",
		AllowInsecurePath: true,
	}

	_, err := resolveFileRef(ref, pc)
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestResolveFileRef_EmptyProviderPath(t *testing.T) {
	ref := &SecretRef{Source: "file", ID: SingleValueFileRefID}
	pc := &ProviderConfig{Source: "file", Path: "", Mode: "singleValue", AllowInsecurePath: true}
	_, err := resolveFileRef(ref, pc)
	if err == nil {
		t.Fatal("expected error for empty provider path, got nil")
	}
	want := "file provider path is empty"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestResolveFileRef_JSONMode_NonStringValue(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "secrets.json")
	if err := os.WriteFile(p, []byte(`{"count":42}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	ref := &SecretRef{Source: "file", ID: "/count"}
	pc := &ProviderConfig{Source: "file", Path: p, Mode: "json", AllowInsecurePath: true}
	_, err := resolveFileRef(ref, pc)
	if err == nil {
		t.Fatal("expected error for non-string JSON value, got nil")
	}
	want := `file provider JSON Pointer "/count" resolved to non-string value`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestResolveFileRef_UnsupportedMode(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(p, []byte("data"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	ref := &SecretRef{Source: "file", ID: SingleValueFileRefID}
	pc := &ProviderConfig{Source: "file", Path: p, Mode: "yaml", AllowInsecurePath: true}
	_, err := resolveFileRef(ref, pc)
	if err == nil {
		t.Fatal("expected error for unsupported mode, got nil")
	}
	want := `unsupported file provider mode "yaml"`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestResolveFileRef_DefaultMode_IsJSON(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "secrets.json")
	if err := os.WriteFile(p, []byte(`{"key":"value123"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	ref := &SecretRef{Source: "file", ID: "/key"}
	pc := &ProviderConfig{Source: "file", Path: p, Mode: "", AllowInsecurePath: true}
	got, err := resolveFileRef(ref, pc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "value123" {
		t.Errorf("got %q, want %q", got, "value123")
	}
}

func TestResolveFileRef_JSONMode_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(p, []byte("not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	ref := &SecretRef{Source: "file", ID: "/key"}
	pc := &ProviderConfig{Source: "file", Path: p, Mode: "json", AllowInsecurePath: true}
	_, err := resolveFileRef(ref, pc)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestResolveFileRef_ExceedsMaxBytes(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "big.txt")
	if err := os.WriteFile(p, []byte("this content is longer than 5 bytes"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	ref := &SecretRef{Source: "file", ID: SingleValueFileRefID}
	pc := &ProviderConfig{
		Source:            "file",
		Path:              p,
		Mode:              "singleValue",
		MaxBytes:          5,
		AllowInsecurePath: true,
	}

	_, err := resolveFileRef(ref, pc)
	if err == nil {
		t.Fatal("expected error for file exceeding maxBytes, got nil")
	}
	want := "file provider exceeded maxBytes (5)"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

// TestResolveFileRef_TildePath_SingleValue is the end-to-end smoke test
// for the fix: a singleValue file provider with a ~/-relative path
// resolves correctly through resolveFileRef. Before this PR the audit
// would reject the path as "must be absolute".
func TestResolveFileRef_TildePath_SingleValue(t *testing.T) {
	dir := t.TempDir()
	setFakeOSHome(t, dir)
	t.Setenv("OPENCLAW_HOME", "")

	p := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(p, []byte("tilde_secret\n"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	ref := &SecretRef{Source: "file", ID: SingleValueFileRefID}
	pc := &ProviderConfig{
		Source:            "file",
		Path:              "~/secret.txt",
		Mode:              "singleValue",
		AllowInsecurePath: true,
	}

	got, err := resolveFileRef(ref, pc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "tilde_secret" {
		t.Errorf("got %q, want %q", got, "tilde_secret")
	}
}

// TestResolveFileRef_RelativePath_StillRejected guards the absolute-path
// audit: cwd-relative input must still be rejected even though tilde was
// loosened. Catches regressions if expandTildePath is ever widened to
// also expand "./..." (which would weaken the audit's invariant).
func TestResolveFileRef_RelativePath_StillRejected(t *testing.T) {
	ref := &SecretRef{Source: "file", ID: SingleValueFileRefID}
	pc := &ProviderConfig{
		Source:            "file",
		Path:              "relative/secret.txt",
		Mode:              "singleValue",
		AllowInsecurePath: true,
	}

	_, err := resolveFileRef(ref, pc)
	if err == nil {
		t.Fatal("expected error for relative path, got nil")
	}
	wantSub := "path must be absolute"
	if !strings.Contains(err.Error(), wantSub) {
		t.Errorf("error = %q, want substring %q", err.Error(), wantSub)
	}
}

// TestResolveFileRef_TildePath_JSONMode verifies the tilde-expansion
// path works for json mode (where ref id is a JSON pointer) as well as
// singleValue mode — the mechanism is mode-agnostic.
func TestResolveFileRef_TildePath_JSONMode(t *testing.T) {
	dir := t.TempDir()
	setFakeOSHome(t, dir)
	t.Setenv("OPENCLAW_HOME", "")

	p := filepath.Join(dir, "secrets.json")
	content := `{"providers":{"feishu":{"key":"json_via_tilde"}}}`
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	ref := &SecretRef{Source: "file", ID: "/providers/feishu/key"}
	pc := &ProviderConfig{
		Source:            "file",
		Path:              "~/secrets.json",
		Mode:              "json",
		AllowInsecurePath: true,
	}

	got, err := resolveFileRef(ref, pc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "json_via_tilde" {
		t.Errorf("got %q, want %q", got, "json_via_tilde")
	}
}
