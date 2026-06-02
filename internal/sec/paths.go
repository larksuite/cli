// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package sec manages the first-time bootstrap install of the lark-sec-cli
// sidecar from lark-cli's side: download the artifact, lay it out on disk,
// record what version landed. Runtime lifecycle (start / stop / status) is
// handled by shelling out to lark-sec-cli's own `service enable / disable /
// status` commands, so we don't need pid files / env capture / log tees here.
// Updates after install are lark-sec-cli's responsibility, not lark-cli's.
package sec

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

const (
	// envInstallDirOverride lets tests and power users redirect the entire sec
	// tree (install + data) to a single root. When set, install_dir is <root>
	// and data_dir is <root>/data — no platform-conventional lookup happens.
	envInstallDirOverride = "LARKSUITE_CLI_SEC_DIR"
)

// BinaryName returns the executable basename inside the sec-cli artifact for
// the current platform:
//
//	darwin  → libLarkEntCli.dylib
//	linux   → liblarkentcli.so
//	windows → lark_enterprise_cli.exe
//
// The .dylib/.so extensions on POSIX are convention only — those files are
// normal Mach-O / ELF executables, not loadable libraries.
func BinaryName() string {
	switch runtime.GOOS {
	case "darwin":
		return "libLarkEntCli.dylib"
	case "windows":
		return "lark_enterprise_cli.exe"
	default:
		return "liblarkentcli.so"
	}
}

// Paths exposes the filesystem layout for the sec sidecar. All methods return
// absolute paths; nothing on disk is created — callers must call Ensure().
type Paths struct {
	install string
	data    string
}

// DefaultPaths returns Paths rooted at the platform-conventional user data dir,
// or at $LARKSUITE_CLI_SEC_DIR when set.
func DefaultPaths() (*Paths, error) {
	if root := os.Getenv(envInstallDirOverride); root != "" {
		return &Paths{install: root, data: filepath.Join(root, "data")}, nil
	}
	install, data, err := platformDirs()
	if err != nil {
		return nil, err
	}
	return &Paths{install: install, data: data}, nil
}

// platformDirs returns (install_dir, data_dir) for the current OS, applying
// per-platform conventions:
//
//	macOS    install = data = ~/Library/Application Support/lark-cli/sec
//	Linux    install = $XDG_DATA_HOME/lark-cli/sec      (fallback ~/.local/share/...)
//	         data    = $XDG_STATE_HOME/lark-cli/sec     (fallback ~/.local/state/...)
//	Windows  install = data = %LOCALAPPDATA%\lark-cli\sec
//
// Linux splits install/data along XDG lines; macOS and Windows colocate them
// because their conventions don't distinguish "share" from "state" at the
// per-user level.
func platformDirs() (install, data string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	switch runtime.GOOS {
	case "darwin":
		base := filepath.Join(home, "Library", "Application Support", "lark-cli", "sec")
		return base, filepath.Join(base, "data"), nil
	case "windows":
		appData := os.Getenv("LOCALAPPDATA")
		if appData == "" {
			return "", "", errors.New("LOCALAPPDATA is not set")
		}
		base := filepath.Join(appData, "lark-cli", "sec")
		return base, filepath.Join(base, "data"), nil
	case "linux":
		dataHome := os.Getenv("XDG_DATA_HOME")
		if dataHome == "" {
			dataHome = filepath.Join(home, ".local", "share")
		}
		stateHome := os.Getenv("XDG_STATE_HOME")
		if stateHome == "" {
			stateHome = filepath.Join(home, ".local", "state")
		}
		return filepath.Join(dataHome, "lark-cli", "sec"),
			filepath.Join(stateHome, "lark-cli", "sec"),
			nil
	default:
		base := filepath.Join(home, ".lark-cli", "sec")
		return base, filepath.Join(base, "data"), nil
	}
}

// Ensure creates the directories the installer writes into.
func (p *Paths) Ensure() error {
	for _, d := range []string{p.install, p.data, p.VersionsDir()} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
	}
	return nil
}

// InstallDir is the root for binaries and version trees.
func (p *Paths) InstallDir() string { return p.install }

// DataDir is the root for state.json (and anything else lark-cli persists
// about the install — currently just state.json).
func (p *Paths) DataDir() string { return p.data }

// VersionsDir stores each unpacked release: versions/<version>/<files>.
func (p *Paths) VersionsDir() string { return filepath.Join(p.install, "versions") }

// VersionDir is the unpack target for a specific version string.
func (p *Paths) VersionDir(version string) string {
	return filepath.Join(p.VersionsDir(), version)
}

// CurrentLink points to the active version (symlink on POSIX, plain copy on Windows).
func (p *Paths) CurrentLink() string { return filepath.Join(p.install, "current") }

// BinaryPath is the active sec-cli executable, addressed through the
// `current` symlink so it stays valid across version swaps.
func (p *Paths) BinaryPath() string {
	return filepath.Join(p.CurrentLink(), BinaryName())
}

// StateFile records what version is installed and where its binary lives.
func (p *Paths) StateFile() string { return filepath.Join(p.data, "state.json") }
