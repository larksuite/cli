// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docs

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"image"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/larksuite/cli/tests/cli_e2e/drive"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDocs_LocalResourcesWorkflowAsBot(t *testing.T) {
	// Bot-created documents are cleaned up with the test user because the
	// shared bot intentionally has no Drive delete scope.
	clie2e.SkipWithoutUserToken(t)
	testDocsLocalResourcesWorkflow(t, "bot")
}

func TestDocs_LocalResourcesWorkflowAsUser(t *testing.T) {
	clie2e.SkipWithoutUserToken(t)
	testDocsLocalResourcesWorkflow(t, "user")
}

func testDocsLocalResourcesWorkflow(t *testing.T, defaultAs string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	workDir := t.TempDir()
	createdSource := []byte("created source fixture\n")
	appendedNegativeSource := []byte("appended negative source fixture\n")
	appendedNonNumericSource := []byte("appended nonnumeric source fixture\n")
	replacedSource := []byte("block replaced source fixture\n")
	overwrittenSource := []byte("overwritten source fixture\n")
	writeLocalResourceFixture(t, workDir, "created.png", hundredByEightyPNG)
	writeLocalResourceFixture(t, workDir, "positioned.png", onePixelPNG)
	writeLocalResourceFixture(t, workDir, "created.txt", createdSource)
	writeLocalResourceFixture(t, workDir, "appended.png", onePixelPNG)
	writeLocalResourceFixture(t, workDir, "replaced.png", onePixelPNG)
	writeLocalResourceFixture(t, workDir, "replaced.txt", replacedSource)
	writeLocalResourceFixture(t, workDir, "appended-negative.txt", appendedNegativeSource)
	writeLocalResourceFixture(t, workDir, "appended-nonnumeric.txt", appendedNonNumericSource)
	writeLocalResourceFixture(t, workDir, "overwritten.png", onePixelPNG)
	writeLocalResourceFixture(t, workDir, "overwritten.txt", overwrittenSource)

	suffix := clie2e.GenerateSuffix()
	parentT := t
	folderToken := ""
	cleanupAs := defaultAs
	if defaultAs == "bot" {
		// Bot-created documents grant the current CLI user full access, while
		// the shared PPE bot intentionally lacks Drive delete scopes.
		cleanupAs = "user"
	} else {
		folderToken = drive.CreateDriveFolder(t, parentT, ctx, "lark-cli-e2e-local-resources-"+suffix, defaultAs, "")
	}
	var docToken string
	var roundTripDocToken string
	var roundTripContent string
	var appendedImageBlockID string
	var appendedFileBlockID string
	var appendedFileFigureBlockID string

	t.Run("create image and source", func(t *testing.T) {
		args := []string{
			"docs", "+create",
			"--title", "lark-cli local resources " + suffix,
			"--content", `<p>created resources</p><img path="@created.png" caption="created image" width="50"/><source path="@created.txt" name="created-report.txt" size="0"/>`,
		}
		if folderToken != "" {
			args = append(args, "--parent-token", folderToken)
		}
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      args,
			DefaultAs: defaultAs,
			WorkDir:   workDir,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assertBoundLocalResourceBlocks(t, result.Stdout, 1, 1)

		docToken = gjson.Get(result.Stdout, "data.document.document_id").String()
		require.NotEmpty(t, docToken, "stdout:\n%s", result.Stdout)
		parentT.Cleanup(func() {
			cleanupCtx, cleanupCancel := clie2e.CleanupContext()
			defer cleanupCancel()
			deleteResult, deleteErr := drive.DeleteDriveResourceAndVerify(cleanupCtx, docToken, "docx", cleanupAs)
			clie2e.ReportCleanupFailure(parentT, "delete doc "+docToken, deleteResult, deleteErr)
		})
	})

	t.Run("insert local image after fetched block id", func(t *testing.T) {
		require.NotEmpty(t, docToken, "document token should be created before update")
		initialXML, err := fetchDocsContent(ctx, docToken, "xml", "with-ids", defaultAs)
		require.NoError(t, err)
		anchorBlockID, err := docBlockIDByExactText(initialXML, "created resources")
		require.NoError(t, err)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+update",
				"--doc", docToken,
				"--command", "block_insert_after",
				"--block-id", anchorBlockID,
				"--content", `<img path="@positioned.png" caption="positioned image"/>`,
			},
			DefaultAs: defaultAs,
			WorkDir:   workDir,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assertBoundLocalResourceBlocks(t, result.Stdout, 1, 0)

		var positionedXML string
		require.Eventually(t, func() bool {
			positionedXML, err = fetchDocsContent(ctx, docToken, "xml", "with-ids", defaultAs)
			if err != nil {
				return false
			}
			anchorIndex := strings.Index(positionedXML, "created resources")
			positionedIndex := strings.Index(positionedXML, "positioned image")
			followingIndex := strings.Index(positionedXML, "created image")
			return anchorIndex >= 0 && positionedIndex > anchorIndex && followingIndex > positionedIndex
		}, 20*time.Second, 500*time.Millisecond, "positioned image was not persisted directly between the anchor and following block; XML:\n%s", positionedXML)
	})

	t.Run("append image and source", func(t *testing.T) {
		require.NotEmpty(t, docToken, "document token should be created before update")
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+update",
				"--doc", docToken,
				"--command", "append",
				"--content", `<p>appended resources</p><img path="@appended.png" caption="appended image" width="invalid" height="0"/><source path="@appended-negative.txt" name="appended-negative-report.txt" size="-2"/><source path="@appended-nonnumeric.txt" name="appended-nonnumeric-report.txt" size="invalid"/>`,
			},
			DefaultAs: defaultAs,
			WorkDir:   workDir,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assertBoundLocalResourceBlocks(t, result.Stdout, 1, 2)
		appendedImageBlockID = localResourceBlockID(t, result.Stdout, "image")
		appendedFileBlockID = localResourceBlockID(t, result.Stdout, "file")
		appendedFileFigureBlockID = localResourceParentBlockID(ctx, t, defaultAs, docToken, appendedFileBlockID)
	})

	t.Run("block replace with local image", func(t *testing.T) {
		require.NotEmpty(t, docToken, "document token should be created before update")
		require.NotEmpty(t, appendedImageBlockID, "appended image block should be available before replacement")
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+update",
				"--doc", docToken,
				"--command", "block_replace",
				"--block-id", appendedImageBlockID,
				"--content", `<img path="@replaced.png" caption="block replaced image"/>`,
			},
			DefaultAs: defaultAs,
			WorkDir:   workDir,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assertBoundLocalResourceBlocks(t, result.Stdout, 1, 0)
	})

	t.Run("block replace file child reports server reason", func(t *testing.T) {
		require.NotEmpty(t, docToken, "document token should be created before update")
		require.NotEmpty(t, appendedFileBlockID, "appended file block should be available before replacement")
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+update",
				"--doc", docToken,
				"--command", "block_replace",
				"--block-id", appendedFileBlockID,
				"--content", `<source path="@replaced.txt" name="invalid-child-target.txt"/>`,
			},
			DefaultAs: defaultAs,
			WorkDir:   workDir,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 1)
		result.AssertStdoutStatus(t, false)
		require.Equal(t, "failed", gjson.Get(result.Stdout, "data.result").String(), "stdout:\n%s", result.Stdout)
		require.NotEmpty(t, gjson.Get(result.Stdout, "data.warnings").Array(), "server failure reason should remain visible: %s", result.Stdout)
	})

	t.Run("block replace with local file", func(t *testing.T) {
		require.NotEmpty(t, docToken, "document token should be created before update")
		require.NotEmpty(t, appendedFileFigureBlockID, "appended file figure should be available before replacement")
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+update",
				"--doc", docToken,
				"--command", "block_replace",
				"--block-id", appendedFileFigureBlockID,
				"--content", `<source path="@replaced.txt" name="block-replaced-report.txt"/>`,
			},
			DefaultAs: defaultAs,
			WorkDir:   workDir,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assertBoundLocalResourceBlocks(t, result.Stdout, 0, 1)
	})

	t.Run("fetch verifies persisted resources", func(t *testing.T) {
		require.NotEmpty(t, docToken, "document token should be created before fetch")
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+fetch",
				"--doc", docToken,
				"--doc-format", "xml",
				"--detail", "full",
			},
			DefaultAs: defaultAs,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		content := gjson.Get(result.Stdout, "data.document.content").String()
		for _, want := range []string{
			"created image",
			"positioned image",
			"block replaced image",
			"created-report.txt",
			"block-replaced-report.txt",
			"appended-nonnumeric-report.txt",
		} {
			require.Contains(t, content, want, "fetched XML:\n%s", content)
		}
		require.NotContains(t, content, "@lcli_", "fetched XML leaked internal correlation marker")
		require.NotContains(t, content, "@created.", "fetched XML leaked create fixture path")
		require.NotContains(t, content, "@appended.", "fetched XML leaked append fixture path")
		require.NotContains(t, content, "appended image", "replaced image caption remained in fetched XML")
		require.NotContains(t, content, "appended-negative-report.txt", "replaced file name remained in fetched XML")
		assertFetchedImagePresentation(t, content, "created image", 100, 80, 0.5)
	})

	t.Run("fetch markdown preserves resource metadata", func(t *testing.T) {
		require.NotEmpty(t, docToken, "document token should be created before fetch")
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+fetch",
				"--doc", docToken,
				"--doc-format", "markdown",
				"--detail", "full",
			},
			DefaultAs: defaultAs,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		content := gjson.Get(result.Stdout, "data.document.content").String()
		for _, want := range []string{"![created image](", "![block replaced image]("} {
			require.Contains(t, content, want, "fetched Markdown:\n%s", content)
		}
		assertMarkdownSourceMetadata(t, content, "created-report.txt", len(createdSource))
		assertMarkdownSourceMetadata(t, content, "block-replaced-report.txt", len(replacedSource))
		assertMarkdownSourceMetadata(t, content, "appended-nonnumeric-report.txt", len(appendedNonNumericSource))
		require.NotContains(t, content, "@lcli_", "fetched Markdown leaked internal correlation marker")
		require.NotContains(t, content, "@created.", "fetched Markdown leaked create fixture path")
		require.NotContains(t, content, "@appended.", "fetched Markdown leaked append fixture path")

		roundTripContent = content
	})

	t.Run("create from exported markdown restores image captions", func(t *testing.T) {
		require.NotEmpty(t, roundTripContent, "Markdown content should be fetched before replay")
		args := []string{
			"docs", "+create",
			"--title", "lark-cli markdown replay " + suffix,
			"--doc-format", "markdown",
			"--content", "-",
		}
		if folderToken != "" {
			args = append(args, "--parent-token", folderToken)
		}
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      args,
			DefaultAs: defaultAs,
			Stdin:     []byte(roundTripContent),
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		roundTripDocToken = gjson.Get(result.Stdout, "data.document.document_id").String()
		require.NotEmpty(t, roundTripDocToken, "stdout:\n%s", result.Stdout)
		parentT.Cleanup(func() {
			cleanupCtx, cleanupCancel := clie2e.CleanupContext()
			defer cleanupCancel()
			deleteResult, deleteErr := drive.DeleteDriveResourceAndVerify(cleanupCtx, roundTripDocToken, "docx", cleanupAs)
			clie2e.ReportCleanupFailure(parentT, "delete markdown replay doc "+roundTripDocToken, deleteResult, deleteErr)
		})
	})

	t.Run("fetch markdown replay verifies captions and source metadata", func(t *testing.T) {
		require.NotEmpty(t, roundTripDocToken, "Markdown replay document should be created before fetch")
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+fetch",
				"--doc", roundTripDocToken,
				"--doc-format", "xml",
				"--detail", "full",
			},
			DefaultAs: defaultAs,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		content := gjson.Get(result.Stdout, "data.document.content").String()
		for _, want := range []string{
			`caption="created image`,
			`caption="block replaced image`,
		} {
			require.Contains(t, content, want, "replayed XML:\n%s", content)
		}
		for _, want := range []string{
			"created-report.txt",
			"block-replaced-report.txt",
			"appended-nonnumeric-report.txt",
		} {
			require.Contains(t, content, want, "replayed XML:\n%s", content)
		}
	})

	t.Run("overwrite image and source", func(t *testing.T) {
		require.NotEmpty(t, docToken, "document token should be created before overwrite")
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+update",
				"--doc", docToken,
				"--command", "overwrite",
				"--content", `<p>overwritten resources</p><img path="@overwritten.png" caption="overwritten image"/><source path="@overwritten.txt" name="overwritten-report.txt"/>`,
			},
			DefaultAs: defaultAs,
			WorkDir:   workDir,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assertBoundLocalResourceBlocks(t, result.Stdout, 1, 1)
	})

	t.Run("fetch verifies overwritten resources", func(t *testing.T) {
		require.NotEmpty(t, docToken, "document token should be created before fetch")
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+fetch",
				"--doc", docToken,
				"--doc-format", "xml",
				"--detail", "full",
			},
			DefaultAs: defaultAs,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		content := gjson.Get(result.Stdout, "data.document.content").String()
		for _, want := range []string{"overwritten resources", "overwritten image", "overwritten-report.txt"} {
			require.Contains(t, content, want, "fetched XML:\n%s", content)
		}
		for _, old := range []string{"created resources", "appended resources", "created-report.txt", "appended-negative-report.txt"} {
			require.NotContains(t, content, old, "overwrite preserved old content:\n%s", content)
		}
		require.NotContains(t, content, "@lcli_", "fetched XML leaked internal correlation marker")
		require.NotContains(t, content, "@overwritten.", "fetched XML leaked overwrite fixture path")
	})
}

var markdownSourceTagPattern = regexp.MustCompile(`(?s)<source\b[^>]*>`)

func assertMarkdownSourceMetadata(t *testing.T, content, wantName string, wantSize int) {
	t.Helper()
	wantNameAttr := fmt.Sprintf(`name="%s"`, wantName)
	for _, tag := range markdownSourceTagPattern.FindAllString(content, -1) {
		if !strings.Contains(tag, wantNameAttr) {
			continue
		}
		require.Contains(t, tag, fmt.Sprintf(`size="%d"`, wantSize), "source tag in fetched Markdown:\n%s", tag)
		return
	}
	require.Failf(t, "source metadata not found", "fetched Markdown has no source tag with %s:\n%s", wantNameAttr, content)
}

func assertBoundLocalResourceBlocks(t *testing.T, stdout string, wantImages, wantFiles int) {
	t.Helper()
	counts := map[string]int{"image": 0, "file": 0}
	blockIDs := make(map[string]struct{}, wantImages+wantFiles)
	for _, block := range gjson.Get(stdout, "data.document.new_blocks").Array() {
		blockType := block.Get("block_type").String()
		if _, tracked := counts[blockType]; !tracked {
			continue
		}
		counts[blockType]++
		blockID := block.Get("block_id").String()
		require.NotEmpty(t, blockID, "%s block has no block_id: %s", blockType, block.Raw)
		require.NotContains(t, blockIDs, blockID, "multiple local resources reused block_id %s: %s", blockID, stdout)
		blockIDs[blockID] = struct{}{}
		token := block.Get("block_token").String()
		require.NotEmpty(t, token, "%s block has no bound token: %s", blockType, block.Raw)
		require.False(t, strings.HasPrefix(token, "@lcli_"), "%s block leaked marker: %s", blockType, block.Raw)
	}
	require.Equal(t, wantImages, counts["image"], "image blocks in stdout:\n%s", stdout)
	require.Equal(t, wantFiles, counts["file"], "file blocks in stdout:\n%s", stdout)
}

func localResourceBlockID(t *testing.T, stdout, blockType string) string {
	t.Helper()
	for _, block := range gjson.Get(stdout, "data.document.new_blocks").Array() {
		if block.Get("block_type").String() == blockType {
			blockID := block.Get("block_id").String()
			require.NotEmpty(t, blockID, "%s block has no block_id: %s", blockType, block.Raw)
			return blockID
		}
	}
	require.Failf(t, "local resource block not found", "stdout has no %s block:\n%s", blockType, stdout)
	return ""
}

func localResourceParentBlockID(ctx context.Context, t *testing.T, defaultAs, docToken, childBlockID string) string {
	t.Helper()
	endpoint := fmt.Sprintf(
		"/open-apis/docx/v1/documents/%s/blocks/%s",
		url.PathEscape(docToken),
		url.PathEscape(childBlockID),
	)
	result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      []string{"api", "get", endpoint, "--params", `{"document_revision_id":-1}`},
		DefaultAs: defaultAs,
	}, clie2e.RetryOptions{
		Attempts:     5,
		InitialDelay: 250 * time.Millisecond,
		MaxDelay:     time.Second,
		ShouldRetry: func(result *clie2e.Result) bool {
			return result != nil && result.ExitCode != 0
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)
	parentID := gjson.Get(result.Stdout, "data.block.parent_id").String()
	require.NotEmpty(t, parentID, "file block %s has no figure parent: %s", childBlockID, result.Stdout)
	require.NotEqual(t, childBlockID, parentID, "file replacement must target the enclosing figure")
	return parentID
}

func writeLocalResourceFixture(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, data, 0o600))
}

func assertFetchedImagePresentation(t *testing.T, content, caption string, width, height int, scale float64) {
	t.Helper()
	for _, tag := range regexp.MustCompile(`(?s)<img\b[^>]*>`).FindAllString(content, -1) {
		captionMatch := regexp.MustCompile(`\bcaption="([^"]*)"`).FindStringSubmatch(tag)
		if len(captionMatch) != 2 || strings.TrimSpace(html.UnescapeString(captionMatch[1])) != caption {
			continue
		}
		require.Contains(t, tag, fmt.Sprintf(`width="%d"`, width), "image tag in fetched XML:\n%s", tag)
		require.Contains(t, tag, fmt.Sprintf(`height="%d"`, height), "image tag in fetched XML:\n%s", tag)
		scaleMatch := regexp.MustCompile(`\bscale="([^"]+)"`).FindStringSubmatch(tag)
		require.Len(t, scaleMatch, 2, "image tag has no scale: %s", tag)
		var gotScale float64
		_, err := fmt.Sscanf(scaleMatch[1], "%f", &gotScale)
		require.NoError(t, err, "parse image scale from %s", tag)
		require.InDelta(t, scale, gotScale, 0.000001, "image tag in fetched XML:\n%s", tag)
		return
	}
	require.Failf(t, "image presentation not found", "fetched XML has no image with caption %q:\n%s", caption, content)
}

func encodePNGFixture(width, height int) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		panic(fmt.Sprintf("encode embedded %dx%d PNG fixture: %v", width, height, err))
	}
	return buf.Bytes()
}

var (
	onePixelPNG        = encodePNGFixture(1, 1)
	hundredByEightyPNG = encodePNGFixture(100, 80)
)
