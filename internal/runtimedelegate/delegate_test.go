// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package runtimedelegate

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCapabilities(t *testing.T) {
	var got map[string]any
	if err := json.Unmarshal([]byte(Capabilities("1.2.3")), &got); err != nil {
		t.Fatal(err)
	}
	if got["name"] != "lark-cli" || got["version"] != "1.2.3" || got["runtimeDelegateProtocol"] != float64(1) {
		t.Fatalf("unexpected capabilities: %#v", got)
	}
}

func TestDispatchAbsentBindingFallsThrough(t *testing.T) {
	t.Setenv(DescriptorEnv, "")
	if code, handled, err := Dispatch([]string{"lark-cli", "version"}, "test"); code != 0 || handled || err != nil {
		t.Fatalf("Dispatch = (%d, %v, %v)", code, handled, err)
	}
}

func TestDispatchFailsClosedForInvalidDescriptor(t *testing.T) {
	t.Setenv(DescriptorEnv, "relative.json")
	if code, handled, err := Dispatch([]string{"lark-cli"}, "test"); code != 1 || !handled || err == nil {
		t.Fatalf("Dispatch = (%d, %v, %v)", code, handled, err)
	}
}

func TestDispatchPreservesArgsCwdAndExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	if os.Getenv("GO_WANT_RUNTIME_DELEGATE_HELPER") == "1" {
		code, handled, err := Dispatch([]string{"lark-cli", "one", "two words"}, "test-version")
		if err != nil || !handled {
			t.Fatalf("Dispatch = (%d, %v, %v)", code, handled, err)
		}
		os.Exit(code)
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	delegate := filepath.Join(dir, "delegate")
	output := filepath.Join(dir, "out")
	script := "#!/bin/sh\nread line\nprintf '%s\\n' \"$PWD\" \"$@\" \"$line\" \"$LARK_CLI_RUNTIME_NATIVE_VERSION\" > \"$OUT\"\nprintf stdout-marker\nprintf stderr-marker >&2\nexit 23\n"
	if err := os.WriteFile(delegate, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	descriptorFile := filepath.Join(dir, "descriptor.json")
	raw, _ := json.Marshal(descriptor{ProtocolVersion: 1, BindingID: "binding-1", Delegate: delegate, DelegateArgs: []string{"prefix"}})
	if err := os.WriteFile(descriptorFile, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	working := filepath.Join(dir, "working")
	if err := os.Mkdir(working, 0o700); err != nil {
		t.Fatal(err)
	}
	working, err := filepath.EvalSymlinks(working)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=TestDispatchPreservesArgsCwdAndExit")
	command.Dir = working
	command.Env = append(os.Environ(), DescriptorEnv+"="+descriptorFile, ProtocolEnv+"=1", BoundEnv+"=", "OUT="+output, "GO_WANT_RUNTIME_DELEGATE_HELPER=1")
	command.Stdin = strings.NewReader("stdin-marker\n")
	var stdout, stderr strings.Builder
	command.Stdout, command.Stderr = &stdout, &stderr
	err = command.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 23 {
		t.Fatalf("run = %v", err)
	}
	if stdout.String() != "stdout-marker" || stderr.String() != "stderr-marker" {
		t.Fatalf("stdio = %q / %q", stdout.String(), stderr.String())
	}
	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != working+"\nprefix\none\ntwo words\nstdin-marker\ntest-version\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestMatchingBoundMarkerFallsThroughAndMismatchFails(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	delegate := filepath.Join(dir, "delegate")
	if err := os.WriteFile(delegate, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	descriptorFile := filepath.Join(dir, "descriptor.json")
	raw, _ := json.Marshal(descriptor{ProtocolVersion: 1, BindingID: "binding-1", Delegate: delegate})
	if err := os.WriteFile(descriptorFile, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(DescriptorEnv, descriptorFile)
	t.Setenv(ProtocolEnv, "1")
	t.Setenv(BoundEnv, "binding-1")
	if code, handled, err := Dispatch([]string{"lark-cli"}, "test"); code != 0 || handled || err != nil {
		t.Fatalf("matching = (%d,%v,%v)", code, handled, err)
	}
	t.Setenv(BoundEnv, "another")
	if code, handled, err := Dispatch([]string{"lark-cli"}, "test"); code != 1 || !handled || err == nil {
		t.Fatalf("mismatch = (%d,%v,%v)", code, handled, err)
	}
}

func TestDispatchRejectsGroupWritableDelegate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode semantics")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	delegate := filepath.Join(dir, "delegate")
	if err := os.WriteFile(delegate, []byte("#!/bin/sh\nexit 0\n"), 0o720); err != nil {
		t.Fatal(err)
	}
	descriptorFile := filepath.Join(dir, "descriptor.json")
	raw, _ := json.Marshal(descriptor{ProtocolVersion: 1, BindingID: "binding-1", Delegate: delegate})
	if err := os.WriteFile(descriptorFile, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(DescriptorEnv, descriptorFile)
	t.Setenv(ProtocolEnv, "1")
	if code, handled, err := Dispatch([]string{"lark-cli"}, "test"); code != 1 || !handled || err == nil {
		t.Fatalf("Dispatch = (%d,%v,%v)", code, handled, err)
	}
}
