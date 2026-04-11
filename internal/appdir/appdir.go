// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package appdir

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

const appName = "lark-cli"

// ConfigDir returns the CLI config directory.
func ConfigDir() string {
	if dir, ok := validatedEnvDir("LARKSUITE_CLI_CONFIG_DIR"); ok {
		return dir
	}
	if dir, ok := xdgAppDir("XDG_CONFIG_HOME"); ok {
		return dir
	}
	if dir, ok := legacyConfigDir(); ok {
		return dir
	}
	return defaultConfigDir()
}

// ConfigPath returns the CLI config file path.
func ConfigPath() string {
	return filepath.Join(ConfigDir(), "config.json")
}

// CacheDir returns the CLI cache directory.
func CacheDir() string {
	if dir, ok := validatedEnvDir("LARKSUITE_CLI_CACHE_DIR"); ok {
		return dir
	}
	if dir, ok := xdgAppDir("XDG_CACHE_HOME"); ok {
		return dir
	}
	if dir, ok := legacyConfigDir(); ok {
		return filepath.Join(dir, "cache")
	}
	return defaultCacheDir()
}

// StateDir returns the CLI state directory.
func StateDir() string {
	if dir, ok := validatedEnvDir("LARKSUITE_CLI_STATE_DIR"); ok {
		return dir
	}
	if dir, ok := xdgAppDir("XDG_STATE_HOME"); ok {
		return dir
	}
	if dir, ok := legacyConfigDir(); ok {
		return dir
	}
	return defaultStateDir()
}

// LogDir returns the CLI log directory.
func LogDir() string {
	if dir, ok := validatedEnvDir("LARKSUITE_CLI_LOG_DIR"); ok {
		return dir
	}
	return filepath.Join(StateDir(), "logs")
}

// DataDir returns the directory used for service-specific stored data.
// It returns an error if service contains path separators, traversal sequences,
// or other unsafe characters.
func DataDir(service string) (string, error) {
	if err := validate.SafeServiceName(service); err != nil {
		return "", fmt.Errorf("appdir.DataDir: invalid service name: %w", err)
	}
	if dir, ok := validatedEnvDir("LARKSUITE_CLI_DATA_DIR"); ok {
		return filepath.Join(dir, service), nil
	}
	if dir, ok := xdgDataDir(service); ok {
		return dir, nil
	}
	if dir, ok := legacyDataDir(service); ok {
		return dir, nil
	}
	return defaultDataDir(service), nil
}

func validatedEnvDir(envName string) (string, bool) {
	value := os.Getenv(envName)
	if value == "" {
		return "", false
	}
	dir, err := validate.SafeEnvDirPath(value, envName)
	if err != nil {
		fmt.Fprintf(log.Writer(), "warning: %s=%q is invalid (%v), using default\n", envName, value, err)
		return "", false
	}
	return dir, true
}

func xdgAppDir(envName string) (string, bool) {
	base, ok := validatedEnvDir(envName)
	if !ok {
		return "", false
	}
	return filepath.Join(base, appName), true
}

func xdgDataDir(service string) (string, bool) {
	base, ok := validatedEnvDir("XDG_DATA_HOME")
	if !ok {
		return "", false
	}
	return filepath.Join(base, appName, service), true
}

func legacyConfigDir() (string, bool) {
	dir := filepath.Join(homeDir(), ".lark-cli")
	return dir, dirExists(dir)
}

func legacyDataDir(service string) (string, bool) {
	var dir string
	switch runtime.GOOS {
	case "darwin":
		dir = filepath.Join(homeDir(), "Library", "Application Support", service)
	default:
		return "", false
	}
	return dir, dirExists(dir)
}

func defaultConfigDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(homeDir(), ".lark-cli")
	}
	return filepath.Join(homeDir(), ".config", appName)
}

func defaultCacheDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(ConfigDir(), "cache")
	}
	return filepath.Join(homeDir(), ".cache", appName)
}

func defaultStateDir() string {
	if runtime.GOOS == "windows" {
		return ConfigDir()
	}
	return filepath.Join(homeDir(), ".local", "state", appName)
}

func defaultDataDir(service string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(homeDir(), ".lark-cli", "data", service)
	}
	return filepath.Join(homeDir(), ".local", "share", appName, service)
}

func dirExists(path string) bool {
	info, err := vfs.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func homeDir() string {
	home, err := vfs.UserHomeDir()
	if err != nil || home == "" {
		fmt.Fprintf(log.Writer(), "warning: unable to determine home directory: %v\n", err)
	}
	return home
}
