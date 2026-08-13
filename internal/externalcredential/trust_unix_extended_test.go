// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended && !windows

package externalcredential

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/vfs"
)

func TestVerifyCredentialProgramRejectsInterpreterScript(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envvars.CliExternalCredentialConfig, filepath.Join(dir, "external-credential.json"))
	path := filepath.Join(dir, "credential-helper")
	data := []byte("#!/bin/sh\nprintf '{}'\n")
	if err := vfs.WriteFile(path, data, 0o700); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	err := verifyCredentialProgram(&ProgramConfig{
		Executable: path,
		SHA256:     "sha256:" + fmt.Sprintf("%x", sum),
	})
	if err == nil {
		t.Fatal("expected interpreter script to be rejected")
	}
}

func TestVerifyCredentialProgramRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envvars.CliExternalCredentialConfig, filepath.Join(dir, "external-credential.json"))
	target := filepath.Join(dir, "credential-helper.bin")
	link := filepath.Join(dir, "credential-helper")
	data := []byte("native-placeholder")
	if err := vfs.WriteFile(target, data, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	err := verifyCredentialProgram(&ProgramConfig{
		Executable: link,
		SHA256:     "sha256:" + fmt.Sprintf("%x", sum),
	})
	if err == nil {
		t.Fatal("expected symlink executable to be rejected")
	}
}

func TestValidateAdminControlledPathChecksNonCanonicalLeaf(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envvars.CliExternalCredentialConfig, filepath.Join(dir, "external-credential.json"))
	path := filepath.Join(dir, "credential-helper")
	if err := vfs.WriteFile(path, []byte("native-placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	nonCanonical := filepath.Join(dir, "unused", "..", filepath.Base(path))

	err := validateAdminControlledPath(nonCanonical, true)
	if err == nil || !strings.Contains(err.Error(), "not executable") {
		t.Fatalf("error = %v, want non-canonical leaf executable check", err)
	}
}

func TestValidateAdminControlledPathEnforcesAdminBoundary(t *testing.T) {
	const (
		target = "/opt/lark-cli/credential-helper"
		parent = "/opt/lark-cli"
		opt    = "/opt"
		root   = "/"
	)
	secure := map[string]os.FileInfo{
		target: fakeUnixFileInfo{name: "credential-helper", mode: 0o755, uid: 0},
		parent: fakeUnixFileInfo{name: "lark-cli", mode: os.ModeDir | 0o755, uid: 0},
		opt:    fakeUnixFileInfo{name: "opt", mode: os.ModeDir | 0o755, uid: 0},
		root:   fakeUnixFileInfo{name: "/", mode: os.ModeDir | 0o755, uid: 0},
	}
	lstat := func(files map[string]os.FileInfo) func(string) (os.FileInfo, error) {
		return func(path string) (os.FileInfo, error) {
			info, ok := files[path]
			if !ok {
				return nil, os.ErrNotExist
			}
			return info, nil
		}
	}
	clone := func() map[string]os.FileInfo {
		files := make(map[string]os.FileInfo, len(secure))
		for path, info := range secure {
			files[path] = info
		}
		return files
	}

	t.Run("accepts root-owned non-writable path", func(t *testing.T) {
		if err := validateAdminControlledPathWith(target, true, false, lstat(secure)); err != nil {
			t.Fatalf("validateAdminControlledPathWith() error = %v", err)
		}
	})

	t.Run("rejects non-root-owned leaf", func(t *testing.T) {
		files := clone()
		files[target] = fakeUnixFileInfo{name: "credential-helper", mode: 0o755, uid: 1000}
		err := validateAdminControlledPathWith(target, true, false, lstat(files))
		if err == nil || !strings.Contains(err.Error(), "not owned by root") {
			t.Fatalf("error = %v, want root ownership rejection", err)
		}
	})

	t.Run("rejects group-writable leaf", func(t *testing.T) {
		files := clone()
		files[target] = fakeUnixFileInfo{name: "credential-helper", mode: 0o775, uid: 0}
		err := validateAdminControlledPathWith(target, true, false, lstat(files))
		if err == nil || !strings.Contains(err.Error(), "writable by group or other users") {
			t.Fatalf("error = %v, want writable leaf rejection", err)
		}
	})

	t.Run("rejects untrusted ancestor", func(t *testing.T) {
		files := clone()
		files[parent] = fakeUnixFileInfo{name: "lark-cli", mode: os.ModeDir | 0o777, uid: 0}
		err := validateAdminControlledPathWith(target, true, false, lstat(files))
		if err == nil || !strings.Contains(err.Error(), parent+" is writable by group or other users") {
			t.Fatalf("error = %v, want ancestor rejection", err)
		}
	})
}

// TestNativeAdminControlledPath exercises the production lstat/syscall path on
// the current runner. The table-driven tests above pin the Unix policy. The
// positive native helper check runs on macOS, whose hosted runner exposes the
// production root-owned path contract; hosted Linux runners may use remapped
// ownership for every system directory, so they retain the native rejection
// check without pretending to provide a production-trusted fixture.
func TestNativeAdminControlledPath(t *testing.T) {
	if runtime.GOOS == "darwin" {
		const systemExecutable = "/usr/bin/true"
		if err := validateAdminControlledPath(systemExecutable, true); err != nil {
			t.Fatalf("trusted system executable %s was rejected: %v", systemExecutable, err)
		}
	}

	untrusted := filepath.Join(t.TempDir(), "credential-helper")
	if err := vfs.WriteFile(untrusted, []byte("native-placeholder"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := validateAdminControlledPath(untrusted, true); err == nil {
		t.Fatalf("caller-controlled executable %s was accepted", untrusted)
	}
}

type fakeUnixFileInfo struct {
	name string
	mode os.FileMode
	uid  uint32
}

func (f fakeUnixFileInfo) Name() string       { return f.name }
func (f fakeUnixFileInfo) Size() int64        { return 0 }
func (f fakeUnixFileInfo) Mode() os.FileMode  { return f.mode }
func (f fakeUnixFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeUnixFileInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fakeUnixFileInfo) Sys() any           { return &syscall.Stat_t{Uid: f.uid} }
