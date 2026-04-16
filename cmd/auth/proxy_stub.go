// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !authsidecar

package auth

import (
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

// NewCmdAuthProxy returns nil when the authsidecar build tag is not set.
func NewCmdAuthProxy(f *cmdutil.Factory) *cobra.Command {
	return nil
}
