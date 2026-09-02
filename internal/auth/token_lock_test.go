// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/vfs"
)

type authFSStub struct {
	vfs.OsFs
	mkdirAll  func(string, fs.FileMode) error
	writeFile func(string, []byte, fs.FileMode) error
}

func (f authFSStub) MkdirAll(path string, perm fs.FileMode) error {
	if f.mkdirAll != nil {
		return f.mkdirAll(path, perm)
	}
	return f.OsFs.MkdirAll(path, perm)
}

func (f authFSStub) WriteFile(path string, data []byte, perm fs.FileMode) error {
	if f.writeFile != nil {
		return f.writeFile(path, data, perm)
	}
	return f.OsFs.WriteFile(path, data, perm)
}

func useAuthFSStub(t *testing.T, stub authFSStub) {
	t.Helper()
	previous := vfs.DefaultFS
	vfs.DefaultFS = stub
	t.Cleanup(func() { vfs.DefaultFS = previous })
}

func TestTokenStorageLockUsesWorkspaceSanitizedPath(t *testing.T) {
	setupStoredTokenTest(t)
	previous := core.CurrentWorkspace()
	core.SetCurrentWorkspace(core.WorkspaceOpenClaw)
	t.Cleanup(func() { core.SetCurrentWorkspace(previous) })

	got := tokenStorageLockPath("cli/test", "ou:test")
	if filepath.Dir(got) != filepath.Join(core.GetConfigDir(), "locks") {
		t.Fatalf("lock directory = %q, want workspace config lock directory", filepath.Dir(got))
	}
	if filepath.Base(got) != "refresh_cli_test_ou_test.lock" {
		t.Fatalf("lock filename = %q, want sanitized account identifiers", filepath.Base(got))
	}
}

func TestWithTokenStorageLockClassifiesDirectoryFailure(t *testing.T) {
	setupStoredTokenTest(t)
	sentinel := errors.New("permission denied")
	useAuthFSStub(t, authFSStub{
		mkdirAll: func(string, fs.FileMode) error { return sentinel },
	})
	called := false

	err := withTokenStorageLock("cli_lock_error", "ou_lock_error", func() error {
		called = true
		return nil
	})
	if called {
		t.Fatal("locked function ran after lock directory creation failed")
	}
	requireRefreshProblem(t, err, errs.CategoryInternal, errs.SubtypeFileIO, false)
	if !errors.Is(err, sentinel) {
		t.Fatalf("errors.Is(%v, sentinel) = false", err)
	}
}

const tokenStorageLockHelperEnv = "LARK_CLI_TOKEN_LOCK_TEST_HELPER"

func TestTokenStorageFileLockHelperProcess(t *testing.T) {
	if os.Getenv(tokenStorageLockHelperEnv) != "1" {
		return
	}
	err := withTokenStorageLock(
		os.Getenv("LARK_CLI_TOKEN_LOCK_TEST_APP_ID"),
		os.Getenv("LARK_CLI_TOKEN_LOCK_TEST_USER_ID"),
		func() error {
			if _, err := fmt.Fprintln(os.Stdout, "TOKEN_LOCKED"); err != nil {
				return err
			}
			var release [1]byte
			_, err := io.ReadFull(os.Stdin, release[:])
			return err
		},
	)
	if err != nil {
		t.Fatalf("child withTokenStorageLock() error = %v", err)
	}
}

func TestTokenStorageLockSerializesAcrossProcesses(t *testing.T) {
	setupStoredTokenTest(t)
	appID := "cli_cross_process_lock"
	userOpenID := "ou_cross_process_lock"
	command := exec.Command(os.Args[0], "-test.run=^TestTokenStorageFileLockHelperProcess$")
	command.Env = append(os.Environ(),
		tokenStorageLockHelperEnv+"=1",
		"LARK_CLI_TOKEN_LOCK_TEST_APP_ID="+appID,
		"LARK_CLI_TOKEN_LOCK_TEST_USER_ID="+userOpenID,
	)
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatalf("StdinPipe() error = %v", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe() error = %v", err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatalf("start helper process: %v", err)
	}
	waited := false
	t.Cleanup(func() {
		_, _ = io.WriteString(stdin, "x")
		_ = stdin.Close()
		if !waited && command.Process != nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	})

	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			ready <- scanner.Text()
			return
		}
		ready <- ""
	}()
	select {
	case line := <-ready:
		if line != "TOKEN_LOCKED" {
			t.Fatalf("helper readiness = %q, want TOKEN_LOCKED; stderr=%s", line, stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for helper lock; stderr=%s", stderr.String())
	}

	acquired := make(chan error, 1)
	go func() {
		acquired <- withTokenStorageLock(appID, userOpenID, func() error { return nil })
	}()
	select {
	case err := <-acquired:
		t.Fatalf("parent acquired lock while child held it: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	if _, err := io.WriteString(stdin, "x"); err != nil {
		t.Fatalf("release helper process: %v", err)
	}
	if err := stdin.Close(); err != nil {
		t.Fatalf("close helper stdin: %v", err)
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("helper process error = %v; stderr=%s", err, stderr.String())
	}
	waited = true
	select {
	case err := <-acquired:
		if err != nil {
			t.Fatalf("parent lock error after child release = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("parent did not acquire lock after child released it")
	}
}
