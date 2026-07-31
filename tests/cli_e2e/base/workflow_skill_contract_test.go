// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"regexp"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestWorkflowSchemaMessageActionContractIsDelivered(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"skills", "read", "lark-base", "references/lark-base-workflow-schema.md"},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	for name, pattern := range map[string]string{
		"receiver required and non-empty":   `(?m)^\|\s*` + "`receiver`" + `\s*\|\s*是\s*\|\s*非空 ValueInfo\[\]`,
		"content required and non-empty":    `(?m)^\|\s*` + "`content`" + `\s*\|\s*是\s*\|\s*非空 TextRefItem\[\]`,
		"send_to_everyone optional boolean": `(?m)^\|\s*` + "`send_to_everyone`" + `\s*\|\s*否\s*\|\s*boolean`,
		"btn_list optional array":           `(?m)^\|\s*` + "`btn_list`" + `\s*\|\s*否\s*\|\s*ButtonConfig\[\]`,
	} {
		t.Run(name, func(t *testing.T) {
			require.Regexp(t, regexp.MustCompile(pattern), result.Stdout)
		})
	}
}
