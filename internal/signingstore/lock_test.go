// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package signingstore

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
)

func TestKeyStoreFileLockAcrossProcesses(t *testing.T) {
	const helperEnv = "LARK_KEY_STORE_HELPER"
	if os.Getenv(helperEnv) == "hold" {
		err := WithStoreLock(context.Background(), os.Getenv("LARK_KEY_STORE_LOCK_PATH"), func() error {
			fmt.Println("locked")
			_, err := io.Copy(io.Discard, os.Stdin)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return
	}
	lockPath := filepath.Join(t.TempDir(), "key_store.lock")
	t.Setenv("LARK_KEY_STORE_LOCK_PATH", lockPath)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestKeyStoreFileLockAcrossProcesses$")
	command.Env = append(os.Environ(), helperEnv+"=hold")
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}()
	if line, err := bufio.NewReader(stdout).ReadString('\n'); err != nil || line != "locked\n" {
		t.Fatalf("readiness: %q %v", line, err)
	}
	calls := 0
	operation := func() error { calls++; return nil }
	wait, stop := context.WithTimeout(ctx, 30*time.Millisecond)
	err = WithStoreLock(wait, lockPath, operation)
	stop()
	problem, ok := errs.ProblemOf(err)
	if !errors.Is(err, context.DeadlineExceeded) || !ok || problem.Subtype != errs.SubtypeStorage || !problem.Retryable || calls != 0 {
		t.Fatalf("timeout: %v", err)
	}
	wait, stop = context.WithCancel(ctx)
	timer := time.AfterFunc(30*time.Millisecond, stop)
	err = WithStoreLock(wait, lockPath, operation)
	timer.Stop()
	stop()
	if !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatalf("cancel: %v", err)
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = command.Wait()
	waited = true
	if err := WithStoreLock(ctx, lockPath, operation); err != nil || calls != 1 {
		t.Fatalf("after releasing lock: err=%v calls=%d", err, calls)
	}
}
