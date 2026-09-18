// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docs

import (
	"context"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDocsResourcePathsParseAndWriteDryRun(t *testing.T) {
	workDir := t.TempDir()
	draftDir := filepath.Join(workDir, "draft")
	require.NoError(t, os.Mkdir(draftDir, 0o700))
	writeLocalResourceFixture(t, draftDir, "image.png", onePixelPNG)
	writeLocalResourceFixture(t, draftDir, "attachment.txt", []byte("fixture attachment"))
	writeLocalResourceFixture(t, draftDir, "diagram.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100"><rect width="100" height="100"/></svg>`))
	writeLocalResourceFixture(t, draftDir, "widget.html", []byte(`<html><body>path fixture</body></html>`))
	writeLocalResourceFixture(t, draftDir, ".presentation-decision.json", []byte(`{"visual_plan":{"blocks":[{"type":"whiteboard","min_count":1}]}}`))
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	for _, mode := range []string{"xml directory fallback", "absolute"} {
		t.Run(mode, func(t *testing.T) {
			ref := func(name string) string {
				if mode == "absolute" {
					return "@" + html.EscapeString(filepath.Join(draftDir, name))
				}
				return "@./" + name
			}
			content := fmt.Sprintf(`<p>path fixture</p><img path="%s"/><source path="%s"/><whiteboard type="svg" path="%s"/><html5-block path="%s"/>`,
				ref("image.png"), ref("attachment.txt"), ref("diagram.svg"), ref("widget.html"))
			writeLocalResourceFixture(t, draftDir, "draft.xml", []byte(content))
			for _, operation := range []string{"parse", "create", "update"} {
				t.Run(operation, func(t *testing.T) {
					args := []string{"docs", "+script", "--command", "parse", "--content", "@./draft/draft.xml"}
					if operation == "create" {
						args = []string{"docs", "+create", "--content", "@./draft/draft.xml", "--dry-run"}
					}
					if operation == "update" {
						args = []string{"docs", "+update", "--doc", "doxcnResourcePathFixture", "--command", "append", "--content", "@./draft/draft.xml", "--dry-run"}
					}
					result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot", WorkDir: workDir, Env: docsScriptE2EEnv(t)})
					require.NoError(t, err)
					result.AssertExitCode(t, 0)
					result.AssertStdoutStatus(t, true)
					if operation == "parse" {
						require.Equal(t, "passed", gjson.Get(result.Stdout, "data.assessment.status").String())
						return
					}
					wantMethod := "POST"
					wantPath := "/open-apis/docs_ai/v1/documents"
					if operation == "update" {
						wantMethod = "PUT"
						wantPath += "/doxcnResourcePathFixture"
					}
					found := false
					for _, api := range gjson.Get(result.Stdout, "data.api").Array() {
						if api.Get("method").String() == wantMethod && api.Get("url").String() == wantPath {
							found = true
							body := api.Get("body")
							require.Contains(t, body.Get("content").String(), "<svg")
							require.Contains(t, body.Get("content").String(), `data-ref="html5_1"`)
							require.Contains(t, body.Get("reference_map.html5-block.html5_1.data").String(), "path fixture")
							break
						}
					}
					require.True(t, found, "missing document write request: %s", result.Stdout)
				})
			}
		})
	}
}
