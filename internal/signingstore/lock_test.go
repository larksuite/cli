// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package signingstore

import (
	"bufio"
	"bytes"
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

// Native initialization is represented by an exclusive file creation. Reverting
// store locking makes competing child processes race on the shared file.
func processStoreOperation() error {
	if os.Getenv("LARK_KEY_STORE_HELPER") == "hold" {
		fmt.Println("locked")
		var b [1]byte
		if _, err := io.ReadFull(os.Stdin, b[:]); err != nil {
			return err
		}
	} else {
		path := filepath.Join(os.Getenv("HOME"), "native-keychain")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			time.Sleep(150 * time.Millisecond)
			f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	return nil
}

func TestKeyStoreFileLockAcrossProcesses(t *testing.T) {
	const helperEnv = "LARK_KEY_STORE_HELPER"
	if os.Getenv(helperEnv) != "" {
		if err := WithStoreLock(context.Background(), os.Getenv("LARK_KEY_STORE_LOCK_PATH"), processStoreOperation); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Setenv("HOME", t.TempDir())
	lockPath := filepath.Join(t.TempDir(), "key_store.lock")
	t.Setenv("LARK_KEY_STORE_LOCK_PATH", lockPath)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var commands []*exec.Cmd
	var outputs []*bytes.Buffer
	for range 6 {
		command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestKeyStoreFileLockAcrossProcesses$")
		command.Env = append(os.Environ(), helperEnv+"=compete")
		out := new(bytes.Buffer)
		command.Stdout, command.Stderr = out, out
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		commands = append(commands, command)
		outputs = append(outputs, out)
	}
	for i, command := range commands {
		if err := command.Wait(); err != nil {
			t.Fatalf("child: %v: %s", err, outputs[i])
		}
	}
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
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir()) // Another profile still shares the native keychain.
	calls := 0
	operation := func() error { calls++; return nil }
	wait, stop := context.WithTimeout(ctx, 30*time.Millisecond)
	err = WithStoreLock(wait, lockPath, operation)
	stop()
	problem, ok := errs.ProblemOf(err)
	if !errors.Is(err, context.DeadlineExceeded) || !ok || !problem.Retryable || calls != 0 {
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
	if err := WithStoreLock(ctx, lockPath, operation); err != nil {
		t.Fatal(err)
	}
}
