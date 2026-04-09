// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package selfupdate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/larksuite/cli/internal/core"
)

const (
	backupDir  = "backups"
	maxBackups = 5
)

// BackupInfo stores metadata about a backup.
type BackupInfo struct {
	Version   string `json:"version"`
	Path      string `json:"path"`
	CreatedAt string `json:"created_at"`
}

// Backup creates a backup of the current binary before upgrading.
// Returns the backup info, or nil if backup is not possible (e.g., dev builds).
func Backup(version string) (*BackupInfo, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("cannot determine current binary: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve symlinks: %w", err)
	}

	dir := backupBaseDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("cannot create backup directory: %w", err)
	}

	ts := time.Now().Format("20060102-150405")
	name := fmt.Sprintf("v%s-%s", version, ts)
	backupPath := filepath.Join(dir, name)

	if err := copyFile(exe, backupPath); err != nil {
		return nil, fmt.Errorf("backup failed: %w", err)
	}
	os.Chmod(backupPath, 0755)

	info := &BackupInfo{
		Version:   version,
		Path:      backupPath,
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	// Save metadata.
	metaPath := backupPath + ".json"
	data, _ := json.Marshal(info)
	os.WriteFile(metaPath, data, 0644)

	// Prune old backups.
	pruneBackups(dir)

	return info, nil
}

// ListBackups returns all available backups, newest first.
func ListBackups() ([]BackupInfo, error) {
	dir := backupBaseDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var backups []BackupInfo
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) == ".json" {
			continue
		}
		metaPath := filepath.Join(dir, e.Name()+".json")
		data, err := os.ReadFile(metaPath)
		if err != nil {
			// No metadata — create a minimal entry from filename.
			backups = append(backups, BackupInfo{
				Version: e.Name(),
				Path:    filepath.Join(dir, e.Name()),
			})
			continue
		}
		var info BackupInfo
		if err := json.Unmarshal(data, &info); err != nil {
			continue
		}
		backups = append(backups, info)
	}

	// Sort newest first.
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt > backups[j].CreatedAt
	})
	return backups, nil
}

// Rollback restores the most recent backup.
func Rollback() (*BackupInfo, error) {
	backups, err := ListBackups()
	if err != nil {
		return nil, err
	}
	if len(backups) == 0 {
		return nil, fmt.Errorf("no backups available")
	}
	return RollbackTo(backups[0])
}

// RollbackTo restores a specific backup.
func RollbackTo(info BackupInfo) (*BackupInfo, error) {
	if _, err := os.Stat(info.Path); err != nil {
		return nil, fmt.Errorf("backup file not found: %s", info.Path)
	}
	if err := ReplaceSelf(info.Path); err != nil {
		return nil, fmt.Errorf("rollback failed: %w", err)
	}
	return &info, nil
}

func backupBaseDir() string {
	return filepath.Join(core.GetConfigDir(), backupDir)
}

func pruneBackups(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	// Collect binary files (not .json metadata).
	var binaries []os.DirEntry
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) != ".json" {
			binaries = append(binaries, e)
		}
	}

	if len(binaries) <= maxBackups {
		return
	}

	// Sort by name (contains timestamp) — oldest first.
	sort.Slice(binaries, func(i, j int) bool {
		return binaries[i].Name() < binaries[j].Name()
	})

	// Remove oldest.
	for _, e := range binaries[:len(binaries)-maxBackups] {
		p := filepath.Join(dir, e.Name())
		os.Remove(p)
		os.Remove(p + ".json")
	}
}
