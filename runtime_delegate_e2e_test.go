// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !windows

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestMainProcessRuntimeDelegateE2E(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "lark-cli")
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	binary, err := filepath.EvalSymlinks(binary)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("bootstrap stdio cwd argv and ordinary exit", func(t *testing.T) {
		working := filepath.Join(root, "working")
		if err := os.Mkdir(working, 0o700); err != nil {
			t.Fatal(err)
		}
		working, err = filepath.EvalSymlinks(working)
		if err != nil {
			t.Fatal(err)
		}
		delegate := filepath.Join(root, "delegate")
		script := `#!/bin/sh
read line
printf 'out:%s:%s:%s:%s\n' "$PWD" "$1" "$line" "$LARK_CLI_RUNTIME_NATIVE_VERSION"
printf 'err-marker\n' >&2
test "$LARK_CLI_RUNTIME_NATIVE_EXECUTABLE" = "$EXPECTED_NATIVE" || exit 91
exit 29
`
		if err := os.WriteFile(delegate, []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
		descriptor := filepath.Join(root, "descriptor.json")
		writeDescriptor(t, descriptor, delegate)
		command := exec.Command(binary, "definitely-not-a-cobra-command", "two words")
		command.Dir = working
		command.Env = append(os.Environ(), "LARK_CLI_RUNTIME_DELEGATE="+descriptor, "LARK_CLI_RUNTIME_PROTOCOL=1", "EXPECTED_NATIVE="+binary)
		command.Stdin = bytes.NewBufferString("stdin-marker\n")
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		err := command.Run()
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 29 {
			t.Fatalf("exit = %v", err)
		}
		if stdout.String() != "out:"+working+":definitely-not-a-cobra-command:stdin-marker:DEV\n" {
			t.Fatalf("stdout = %q", stdout.String())
		}
		if stderr.String() != "err-marker\n" {
			t.Fatalf("stderr = %q", stderr.String())
		}
	})

	t.Run("signal terminates the replaced process", func(t *testing.T) {
		delegate := filepath.Join(root, "signal-delegate")
		marker := filepath.Join(root, "started")
		if err := os.WriteFile(delegate, []byte("#!/bin/sh\nprintf started > \"$STARTED\"\nexec sleep 30\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		descriptor := filepath.Join(root, "signal-descriptor.json")
		writeDescriptor(t, descriptor, delegate)
		command := exec.Command(binary, "ignored")
		command.Env = append(os.Environ(), "LARK_CLI_RUNTIME_DELEGATE="+descriptor, "LARK_CLI_RUNTIME_PROTOCOL=1", "STARTED="+marker)
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(5 * time.Second)
		for {
			if _, err := os.Stat(marker); err == nil {
				break
			}
			if time.Now().After(deadline) {
				_ = command.Process.Kill()
				t.Fatal("delegate startup marker timed out")
			}
			time.Sleep(10 * time.Millisecond)
		}
		if err := command.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		err := command.Wait()
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("wait = %v", err)
		}
		status, ok := exitErr.Sys().(syscall.WaitStatus)
		if !ok || status.Signal() != syscall.SIGTERM {
			t.Fatalf("status = %#v", exitErr.Sys())
		}
	})
}

func writeDescriptor(t *testing.T, file, delegate string) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"protocolVersion": 1, "bindingId": "e2e-binding", "delegate": delegate})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}
