// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBase_CoreWorkflow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	baseToken := createBase(t, ctx, uniqueName("lark-cli-e2e-base"))

	t.Run("get base", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+base-get", "--base-token", baseToken},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot base get capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.NotEmpty(t, gjson.Get(result.Stdout, "data.base.name").String(), "stdout:\n%s", result.Stdout)
	})

	t.Run("copy base", func(t *testing.T) {
		copiedToken := copyBase(t, ctx, baseToken, uniqueName("lark-cli-e2e-base-copy"))
		assert.NotEqual(t, baseToken, copiedToken)
	})
}
