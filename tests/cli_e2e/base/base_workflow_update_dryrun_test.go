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

func TestBaseWorkflowUpdateDryRunReadsJSONFromStdin(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+workflow-update",
			"--base-token", "app_example",
			"--workflow-id", "wkf_example",
			"--json", "-",
			"--dry-run",
		},
		Stdin:     []byte(`{"title":"Updated workflow","status":"OFF","steps":[]}` + "\n"),
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, "PUT", clie2e.DryRunGet(out, "api.0.method").String(), out)
	require.Equal(t, "/open-apis/base/v3/bases/app_example/workflows/wkf_example", clie2e.DryRunGet(out, "api.0.url").String(), out)
	require.Equal(t, "Updated workflow", clie2e.DryRunGet(out, "api.0.body.title").String(), out)
	require.Equal(t, "OFF", clie2e.DryRunGet(out, "api.0.body.status").String(), out)
	require.True(t, clie2e.DryRunGet(out, "api.0.body.steps").IsArray(), out)
}
