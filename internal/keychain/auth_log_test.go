package keychain

import (
	"path/filepath"
	"testing"
)

func TestAuthLogDir_UsesValidatedLogDirEnv(t *testing.T) {
	base := t.TempDir()
	base, _ = filepath.EvalSymlinks(base)
	t.Setenv("LARKSUITE_CLI_LOG_DIR", filepath.Join(base, "logs", "..", "auth"))
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", "")

	got := authLogDir()
	want := filepath.Join(base, "auth")
	if got != want {
		t.Fatalf("authLogDir() = %q, want %q", got, want)
	}
}

func TestAuthLogDir_InvalidLogDirFallsBackToConfigDir(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_LOG_DIR", "relative-logs")
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	got := authLogDir()
	want := filepath.Join(configDir, "logs")
	if got != want {
		t.Fatalf("authLogDir() = %q, want %q", got, want)
	}
}
