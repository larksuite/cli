// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSlidesPresentationAliasesDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	aliases := []string{
		"presentation-id",
		"presentation-token",
		"token",
		"presentation_id",
		"xml-presentation-id",
		"url",
	}
	for _, alias := range aliases {
		t.Run(alias, func(t *testing.T) {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{
					"slides", "+xml-get",
					"--" + alias, "presAliasDryRun",
					"--dry-run",
				},
				DefaultAs: "bot",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			require.Equal(t, "GET", gjson.Get(result.Stdout, "data.api.0.method").String(), result.Stdout)
			require.Equal(t,
				"/open-apis/slides_ai/v1/xml_presentations/presAliasDryRun",
				gjson.Get(result.Stdout, "data.api.0.url").String(),
				result.Stdout,
			)
		})
	}
}
