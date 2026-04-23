// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/vfs"
)

// noopKeychain is a zero-side-effect KeychainAccess for exercising
// WithKeychain without touching the platform keychain.
type noopKeychain struct{}

func (noopKeychain) Get(service, account string) (string, error) { return "", nil }
func (noopKeychain) Set(service, account, value string) error    { return nil }
func (noopKeychain) Remove(service, account string) error        { return nil }

// TestBuild_ExternalAPI asserts the library surface that external consumers
// (e.g. cli-server) depend on: Build composes a root command from an
// InvocationContext plus BuildOptions (WithIO, WithKeychain, HideProfile),
// and SetDefaultFS swaps the global VFS. This test is the contract guard.
func TestBuild_ExternalAPI(t *testing.T) {
	// Exercise SetDefaultFS both directions. Passing nil restores the OS FS.
	SetDefaultFS(vfs.OsFs{})
	SetDefaultFS(nil)

	var in, out, errOut bytes.Buffer
	rootCmd := Build(
		context.Background(),
		cmdutil.InvocationContext{},
		WithIO(&in, &out, &errOut),
		WithKeychain(noopKeychain{}),
		HideProfile(true),
	)

	if rootCmd == nil {
		t.Fatal("Build returned nil root command")
	}
	if rootCmd.Use != "lark-cli" {
		t.Errorf("rootCmd.Use = %q, want %q", rootCmd.Use, "lark-cli")
	}
	if len(rootCmd.Commands()) == 0 {
		t.Error("Build produced a root command with no subcommands")
	}
}

// TestBuild_NoOptions guards against regression of the nil-streams panic:
// calling Build without WithIO must fall back to SystemIO rather than
// deref nil at rootCmd.SetIn/Out/Err.
func TestBuild_NoOptions(t *testing.T) {
	rootCmd := Build(context.Background(), cmdutil.InvocationContext{})
	if rootCmd == nil {
		t.Fatal("Build returned nil root command")
	}
	if rootCmd.Use != "lark-cli" {
		t.Errorf("rootCmd.Use = %q, want %q", rootCmd.Use, "lark-cli")
	}
}
