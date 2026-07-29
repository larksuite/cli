//go:build windows

package runtimedelegate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsExecuteDelegateReturnsChildExitAndPreservesCwd(t *testing.T) {
	if os.Getenv("GO_WANT_WINDOWS_EXECUTE_HELPER") == "1" {
		if err := os.WriteFile(os.Getenv("WINDOWS_EXECUTE_MARKER"), []byte(mustGetwd(t)), 0o600); err != nil {
			t.Fatal(err)
		}
		os.Exit(17)
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")
	env := append(os.Environ(), "GO_WANT_WINDOWS_EXECUTE_HELPER=1", "WINDOWS_EXECUTE_MARKER="+marker)
	code, err := executeDelegate(os.Args[0], []string{"-test.run=TestWindowsExecuteDelegateReturnsChildExitAndPreservesCwd"}, env, dir)
	if err != nil || code != 17 {
		t.Fatalf("executeDelegate = (%d,%v)", code, err)
	}
	bytes, err := os.ReadFile(marker)
	if err != nil || string(bytes) != dir {
		t.Fatalf("marker = %q, %v", bytes, err)
	}
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	value, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return value
}
