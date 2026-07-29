// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package runtimedelegate implements the provider-neutral, environment-driven
// runtime delegation handshake used before normal command bootstrap.
package runtimedelegate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/larksuite/cli/internal/vfs"
)

const (
	ProtocolVersion     = 1
	DescriptorEnv       = "LARK_CLI_RUNTIME_DELEGATE"
	ProtocolEnv         = "LARK_CLI_RUNTIME_PROTOCOL"
	BoundEnv            = "LARK_CLI_RUNTIME_BOUND"
	NativeExecutableEnv = "LARK_CLI_RUNTIME_NATIVE_EXECUTABLE"
	NativeVersionEnv    = "LARK_CLI_RUNTIME_NATIVE_VERSION"
	CapabilityCommand   = "__runtime-delegate-capabilities"
)

type descriptor struct {
	ProtocolVersion int      `json:"protocolVersion"`
	BindingID       string   `json:"bindingId"`
	Delegate        string   `json:"delegate"`
	DelegateArgs    []string `json:"delegateArgs,omitempty"`
	Context         any      `json:"context,omitempty"`
}

type capability struct {
	Name                    string `json:"name"`
	Version                 string `json:"version"`
	RuntimeDelegateProtocol int    `json:"runtimeDelegateProtocol"`
}

// IsCapabilityRequest recognizes the side-effect-free setup handshake.
func IsCapabilityRequest(args []string) bool {
	return len(args) == 1 && args[0] == CapabilityCommand
}

// Capabilities returns a stable machine-readable capability document.
func Capabilities(version string) string {
	data, _ := json.Marshal(capability{Name: "lark-cli", Version: version, RuntimeDelegateProtocol: ProtocolVersion})
	return string(data)
}

// Dispatch delegates an opted-in invocation. handled is false only when the
// descriptor environment variable is absent. All malformed or unsafe binding
// state fails closed instead of falling through to normal command dispatch.
func Dispatch(argv []string, version string) (code int, handled bool, err error) {
	descriptorPath := strings.TrimSpace(os.Getenv(DescriptorEnv))
	if descriptorPath == "" {
		return 0, false, nil
	}
	if len(argv) == 0 {
		return 1, true, errors.New("missing argv[0]")
	}
	d, err := loadDescriptor(descriptorPath)
	if err != nil {
		return 1, true, err
	}
	protocol := fmt.Sprint(ProtocolVersion)
	if os.Getenv(ProtocolEnv) != protocol {
		return 1, true, fmt.Errorf("environment protocol must be %s", protocol)
	}
	if os.Getenv(BoundEnv) == d.BindingID {
		return 0, false, nil
	}
	if os.Getenv(BoundEnv) != "" {
		return 1, true, errors.New("binding marker does not match descriptor")
	}

	args := append(append([]string{}, d.DelegateArgs...), argv[1:]...)
	cwd, err := vfs.Getwd()
	if err != nil {
		return 1, true, fmt.Errorf("resolve working directory: %w", err)
	}
	native, err := os.Executable()
	if err != nil {
		return 1, true, fmt.Errorf("resolve native executable: %w", err)
	}
	native, err = filepath.EvalSymlinks(native)
	if err != nil {
		return 1, true, fmt.Errorf("canonicalize native executable: %w", err)
	}
	env := overriddenEnv(os.Environ(), map[string]string{NativeExecutableEnv: native, NativeVersionEnv: version})
	code, err = executeDelegate(d.Delegate, args, env, cwd)
	if err != nil {
		return 1, true, fmt.Errorf("execute delegate: %w", err)
	}
	return code, true, nil
}

func overriddenEnv(base []string, values map[string]string) []string {
	result := make([]string, 0, len(base)+len(values))
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		if _, replaced := values[key]; !replaced {
			result = append(result, entry)
		}
	}
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	return result
}

func loadDescriptor(file string) (descriptor, error) {
	var result descriptor
	if !filepath.IsAbs(file) || filepath.Clean(file) != file {
		return result, errors.New("descriptor path must be clean and absolute")
	}
	if err := validatePrivateDirectory(filepath.Dir(file)); err != nil {
		return result, err
	}
	if err := validatePrivateRegularFile(file, "descriptor"); err != nil {
		return result, err
	}
	raw, err := vfs.ReadFile(file)
	if err != nil {
		return result, fmt.Errorf("read descriptor: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, fmt.Errorf("decode descriptor: %w", err)
	}
	if result.ProtocolVersion != ProtocolVersion {
		return result, fmt.Errorf("descriptor protocol must be %d", ProtocolVersion)
	}
	if strings.TrimSpace(result.BindingID) == "" {
		return result, errors.New("descriptor bindingId is required")
	}
	if !filepath.IsAbs(result.Delegate) || filepath.Clean(result.Delegate) != result.Delegate {
		return result, errors.New("delegate path must be clean and absolute")
	}
	if err := validateExecutable(result.Delegate); err != nil {
		return result, err
	}
	for _, arg := range result.DelegateArgs {
		if strings.IndexByte(arg, 0) >= 0 {
			return result, errors.New("delegate argument contains NUL")
		}
	}
	return result, nil
}

func validatePrivateRegularFile(file, label string) error {
	info, err := vfs.Lstat(file)
	if err != nil {
		return fmt.Errorf("stat %s: %w", label, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s must be a regular non-symlink file", label)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s must not be accessible by group or others", label)
	}
	if !ownedByCurrentUser(file, info) {
		return fmt.Errorf("%s must be owned by the current user", label)
	}
	return nil
}

func validatePrivateDirectory(directory string) error {
	info, err := vfs.Lstat(directory)
	if err != nil {
		return fmt.Errorf("stat descriptor directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 || !ownedByCurrentUser(directory, info) {
		return errors.New("descriptor directory must be private, owned, and non-symlink")
	}
	return nil
}

func validateExecutable(file string) error {
	info, err := vfs.Lstat(file)
	if err != nil {
		return fmt.Errorf("stat delegate: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("delegate must be a regular non-symlink file")
	}
	if !ownedByCurrentUser(file, info) {
		return errors.New("delegate must be owned by the current user")
	}
	if info.Mode().Perm()&0o022 != 0 {
		return errors.New("delegate must not be writable by group or others")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return errors.New("delegate is not executable")
	}
	return nil
}
