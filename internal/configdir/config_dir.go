// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package configdir

import (
	"fmt"
	"os"
	"path/filepath"
)

// Get returns the CLI config directory.
func Get() string {
	if dir := os.Getenv("LARKSUITE_CLI_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		fmt.Fprintf(os.Stderr, "warning: unable to determine home directory: %v\n", err)
	}
	return filepath.Join(home, ".lark-cli")
}
