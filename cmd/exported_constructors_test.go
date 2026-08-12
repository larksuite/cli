// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"testing"

	"github.com/larksuite/cli/cmd/auth"
	"github.com/larksuite/cli/cmd/schema"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/spf13/cobra"
)

// Constructors that existed before the command-extension work stay callable at
// their established signatures. Taking a shortcut snapshot is a new entry
// point, not a replacement: distributions outside this module already build
// command trees through these, and dropping one breaks them at compile time
// with no deprecation window. NewCmdAuthWithRecovery counts even though
// *recovery.Projector is internal -- an outside caller cannot name the type but
// can still pass nil for it.
//
// Written as assignments rather than calls so the assertion is the signature
// itself: a later parameter addition fails here before it reaches anyone
// downstream.
func TestPreExistingExportedConstructorsKeepTheirSignatures(t *testing.T) {
	var (
		_ func(*cmdutil.Factory) *cobra.Command                                    = auth.NewCmdAuth
		_ func(*cmdutil.Factory, *recovery.Projector) *cobra.Command               = auth.NewCmdAuthWithRecovery
		_ func(*cmdutil.Factory, func(*auth.LoginOptions) error) *cobra.Command    = auth.NewCmdAuthLogin
		_ func(*cmdutil.Factory, func(*schema.SchemaOptions) error) *cobra.Command = schema.NewCmdSchema
	)
	var _ func(*cmdutil.Factory, schema.CommandVisibility, func(*schema.SchemaOptions) error) *cobra.Command = schema.NewCmdSchemaWithVisibility
}
