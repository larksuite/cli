// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/spf13/cobra"
)

func TestValidateEnumFlags_ReturnsTypedValidation(t *testing.T) {
	rctx := newTestRuntime(map[string]string{"mode": "delete"})
	err := validateEnumFlags(rctx, []Flag{
		{Name: "mode", Enum: []string{"append", "overwrite"}},
	})
	assertValidationParam(t, err, "--mode")
}

func TestFrameworkFormatFlagPreservesUnknownAndCaseInsensitiveValues(t *testing.T) {
	for _, value := range []string{"unknown-format", "JSON", "TABLE", "NDJSON", "CSV"} {
		t.Run(value, func(t *testing.T) {
			config := &core.CliConfig{}
			factory, _, _, _ := cmdutil.TestFactory(t, config)
			parent := &cobra.Command{Use: "root"}
			var got string
			shortcut := Shortcut{
				Service:     "test",
				Command:     "+format",
				Description: "format fixture",
				AuthTypes:   []string{"user", "bot"},
				PostMount: func(cmd *cobra.Command) {
					AddOutputFormats(cmd, "concise")
				},
				Execute: func(_ context.Context, runtime *RuntimeContext) error {
					got = runtime.Format
					return nil
				},
			}
			shortcut.Mount(parent, factory)
			parent.SetArgs([]string{"+format", "--format", value})
			parent.SilenceErrors = true
			parent.SilenceUsage = true
			if err := parent.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if got != value {
				t.Fatalf("runtime format = %q, want verbatim %q", got, value)
			}
		})
	}
}

func TestHandleShortcutDryRunUnsupported_ReturnsTypedValidation(t *testing.T) {
	err := handleShortcutDryRun(nil, nil, &Shortcut{
		Service: "doc",
		Command: "fetch",
	})
	assertValidationParam(t, err, "--dry-run")
}
