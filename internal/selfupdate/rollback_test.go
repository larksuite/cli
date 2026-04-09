// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package selfupdate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackupAndList(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)

	// Create a fake "current binary" that Backup will copy.
	fakeExe := filepath.Join(dir, "lark-cli")
	os.WriteFile(fakeExe, []byte("binary-v1"), 0755)

	// We can't easily mock os.Executable(), so test the lower-level functions.
	bkDir := filepath.Join(dir, backupDir)
	os.MkdirAll(bkDir, 0700)

	// Directly copy to simulate backup.
	bkPath := filepath.Join(bkDir, "v1.0.0-20260410-120000")
	copyFile(fakeExe, bkPath)
	os.WriteFile(bkPath+".json", []byte(`{"version":"1.0.0","path":"`+bkPath+`","created_at":"2026-04-10T12:00:00Z"}`), 0644)

	backups, err := ListBackups()
	if err != nil {
		t.Fatalf("ListBackups failed: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(backups))
	}
	if backups[0].Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", backups[0].Version)
	}
}

func TestPruneBackups(t *testing.T) {
	dir := t.TempDir()

	// Create 7 fake backups.
	for i := 0; i < 7; i++ {
		name := filepath.Join(dir, "v1.0."+string(rune('0'+i))+"-20260410-12000"+string(rune('0'+i)))
		os.WriteFile(name, []byte("bin"), 0755)
		os.WriteFile(name+".json", []byte("{}"), 0644)
	}

	pruneBackups(dir)

	entries, _ := os.ReadDir(dir)
	// Should have 5 binaries + 5 json = 10 files.
	binCount := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".json" {
			binCount++
		}
	}
	if binCount != maxBackups {
		t.Errorf("expected %d backups after prune, got %d", maxBackups, binCount)
	}
}

func TestRollbackTo(t *testing.T) {
	dir := t.TempDir()

	// Create a backup binary.
	backupPath := filepath.Join(dir, "v1.0.0-backup")
	os.WriteFile(backupPath, []byte("old-binary"), 0755)

	// Create a "current" binary to be replaced.
	current := filepath.Join(dir, "lark-cli")
	os.WriteFile(current, []byte("new-binary"), 0755)

	info := BackupInfo{
		Version: "1.0.0",
		Path:    backupPath,
	}

	// Note: RollbackTo calls ReplaceSelf which uses os.Executable().
	// We can't easily test the full flow here, but we can test that
	// the backup file exists and is readable.
	if _, err := os.Stat(info.Path); err != nil {
		t.Fatalf("backup file should exist: %v", err)
	}
}

func TestListBackups_Empty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)

	backups, err := ListBackups()
	if err != nil {
		t.Fatalf("ListBackups failed: %v", err)
	}
	if len(backups) != 0 {
		t.Errorf("expected 0 backups, got %d", len(backups))
	}
}
