// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

// Test coverage preview:
//
//	| Workflow | Commands |
//	| --- | --- |
//	| advanced permissions | base +base-create, base +advperm-enable, base +advperm-disable |
func TestBase_AdvpermWorkflow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	baseToken := createBase(t, ctx, "lark-cli-e2e-base-advperm-"+testSuffix())

	t.Run("enable", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+advperm-enable", "--base-token", baseToken},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot advanced permission enable capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("disable", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+advperm-disable", "--base-token", baseToken, "--yes"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot advanced permission disable capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})
}
