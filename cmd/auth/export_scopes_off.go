// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !scopeexport

package auth

import (
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

// registerExportScopes is a no-op in normal builds. The scope-export command is
// a build-time tool gated behind the `scopeexport` tag and must not compile into
// released user binaries. See export_scopes.go for the real registration.
func registerExportScopes(_ *cobra.Command, _ *cmdutil.Factory) {}
