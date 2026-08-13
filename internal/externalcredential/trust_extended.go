// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package externalcredential

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/vfs"
)

func validateTrustedConfiguration(configPath string, cfg *Config) error {
	if err := validateAdminControlledPath(configPath, false); err != nil {
		return trustError("system external credential configuration is not trusted: %v", err)
	}
	if cfg == nil || cfg.Program == nil {
		return nil
	}
	return verifyCredentialProgram(cfg.Program)
}

func verifyCredentialProgram(program *ProgramConfig) error {
	if program == nil {
		return trustError("external credential program is missing")
	}
	if err := validateAdminControlledPath(program.Executable, true); err != nil {
		return trustError("external credential program is not trusted: %v", err)
	}
	file, err := vfs.Open(program.Executable)
	if err != nil {
		return trustError("cannot open external credential program: %v", err)
	}
	defer file.Close()
	prefix := make([]byte, 2)
	n, err := io.ReadFull(file, prefix)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return trustError("cannot inspect external credential program: %v", err)
	}
	if n == 2 && string(prefix) == "#!" {
		return trustError("external credential program cannot be an interpreter script")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return trustError("cannot verify external credential program: %v", err)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return trustError("cannot verify external credential program: %v", err)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	expected := strings.TrimPrefix(program.SHA256, "sha256:")
	if !strings.EqualFold(actual, expected) {
		return trustError("external credential program SHA-256 does not match system configuration")
	}
	return nil
}

func trustError(format string, args ...any) error {
	return errs.NewConfigError(errs.SubtypeInvalidConfig, format, args...).
		WithHint("ask the system administrator to reinstall the external credential program and configuration")
}
