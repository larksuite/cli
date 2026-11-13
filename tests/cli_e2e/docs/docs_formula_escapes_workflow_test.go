// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docs

import (
	"context"
	"encoding/xml"
	"io"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/larksuite/cli/tests/cli_e2e/drive"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestDocsFormulaEscapesWorkflowAsUser verifies persisted LaTeX after Markdown
// creation and append through the candidate CLI and the real document service.
func TestDocsFormulaEscapesWorkflowAsUser(t *testing.T) {
	clie2e.SkipWithoutUserToken(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	const identity = "user"
	folder := drive.CreateDriveFolder(t, t, ctx, "lark-cli-e2e-formulas-"+clie2e.GenerateSuffix(), identity, "")
	content := "$q \\| p$\n\n$$\na \\| b\n$$"
	doc := createDocWithRetry(t, t, ctx, folder, "Formula pipe regression", content, identity)

	assertPersisted := func(t *testing.T, expected ...string) {
		t.Helper()
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"docs", "+fetch", "--doc", doc, "--doc-format", "xml"},
			DefaultAs: identity,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		decoder := xml.NewDecoder(strings.NewReader(gjson.Get(result.Stdout, "data.document.content").String()))
		var formulas []string
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			if start, ok := token.(xml.StartElement); ok && start.Name.Local == "latex" {
				var formula string
				require.NoError(t, decoder.DecodeElement(&formula, &start))
				formulas = append(formulas, strings.TrimSpace(formula))
			}
		}
		for _, formula := range expected {
			require.Contains(t, formulas, formula, "persisted equation must retain its LaTeX pipe escape")
		}
	}

	t.Run("create preserves inline and block formulas", func(t *testing.T) {
		assertPersisted(t, `q \| p`, `a \| b`)
	})
	t.Run("append preserves new and existing formulas", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"docs", "+update", "--doc", doc, "--command", "append",
				"--doc-format", "markdown", "--content", `$x \| y$`},
			DefaultAs: identity,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assertPersisted(t, `q \| p`, `a \| b`, `x \| y`)
	})
}
