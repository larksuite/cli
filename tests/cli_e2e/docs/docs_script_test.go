// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDocsScriptParseXMLFromFile(t *testing.T) {
	workDir := t.TempDir()
	input := `<title>标题</title><p>一个苹果是 an apple。</p>`
	if err := os.WriteFile(filepath.Join(workDir, "draft.xml"), []byte(input), 0o600); err != nil {
		t.Fatalf("write draft: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+script",
			"--command", "parse",
			"--content", "@draft.xml",
		},
		DefaultAs: "bot",
		WorkDir:   workDir,
		Env:       docsScriptE2EEnv(t),
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)
	require.Equal(t, "passed", gjson.Get(result.Stdout, "data.assessment.status").String())
	require.Equal(t, int64(10), gjson.Get(result.Stdout, "data.profile.word_count").Int())
	require.Equal(t, int64(15), gjson.Get(result.Stdout, "data.profile.char_count").Int())
	require.Equal(t, int64(2), gjson.Get(result.Stdout, "data.profile.block_count").Int())
	require.False(t, gjson.Get(result.Stdout, "data.input_format").Exists())
	require.False(t, gjson.Get(result.Stdout, "data.command").Exists())
	require.False(t, gjson.Get(result.Stdout, "data.profile.breakdown").Exists())
	require.False(t, gjson.Get(result.Stdout, "data.profile.compatibility").Exists())
}

func TestDocsScriptLocalParseWithAuthenticatedBot(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "local.xml"), []byte(`<p>local only</p>`), 0o600))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+script",
			"--command", "parse",
			"--content", "@local.xml",
		},
		DefaultAs: "bot",
		WorkDir:   workDir,
		Env:       docsScriptE2EEnv(t),
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)
	require.Equal(t, "passed", gjson.Get(result.Stdout, "data.assessment.status").String())
	require.Equal(t, int64(2), gjson.Get(result.Stdout, "data.profile.word_count").Int())
}

func TestDocsScriptPresentationDecisionListCountsULAndOL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	decision := `{"audience":"reader","reader_task":"review recommendations","genre_contract":"none","adapter":null,"presentation_mode":"normal","visual_plan":{"reason":"group recommendations","blocks":[{"type":"list","min_count":2,"purpose":"separate equipment and food recommendations"}]}}`
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+script",
			"--command", "parse",
			"--content", `<ul><li>equipment</li></ul><ol><li>food</li></ol>`,
			"--presentation-decision", decision,
		},
		DefaultAs: "bot",
		WorkDir:   t.TempDir(),
		Env:       docsScriptE2EEnv(t),
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)
	require.Equal(t, "passed", gjson.Get(result.Stdout, "data.assessment.status").String())
	require.False(t, gjson.Get(result.Stdout, "data.diagnostics").Exists())
	require.Equal(t, int64(1), gjson.Get(result.Stdout, `data.profile.blocks.#(type=="ul").count`).Int())
	require.Equal(t, int64(1), gjson.Get(result.Stdout, `data.profile.blocks.#(type=="ol").count`).Int())
}

func TestDocsScriptRemoteImagePreflightDryRunDeclaresNetwork(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	decision := `{"audience":"reader","reader_task":"read the draft","genre_contract":"none","adapter":null,"presentation_mode":"normal","visual_plan":{"reason":"an image provides visual evidence","blocks":[{"type":"img","min_count":1,"purpose":"show visual evidence"}]}}`
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+script",
			"--command", "parse",
			"--content", `<title>Draft</title><img href="https://93.184.216.34/image.png"/>`,
			"--presentation-decision", decision,
			"--dry-run",
		},
		WorkDir:   t.TempDir(),
		DefaultAs: "bot",
		Env:       docsScriptE2EEnv(t),
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	require.True(t, gjson.Get(result.Stdout, "data.network").Bool())
	require.Equal(t, int64(0), gjson.Get(result.Stdout, "data.api.#").Int())
}

func TestDocsScriptInitializedDraftAutomaticallyValidatesPresentationDecision(t *testing.T) {
	workDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	decision := `{"audience":"reader","reader_task":"understand the result and flow","genre_contract":"none","adapter":null,"presentation_mode":"rich","word_count":{"min":18,"max":22},"visual_plan":{"reason":"visual explanation","blocks":[{"type":"img","min_count":1,"purpose":"show the result"},{"type":"whiteboard","min_count":1,"purpose":"show the flow"},{"type":"html5-block","min_count":1,"purpose":"make the state explorable"}]}}`
	initialized, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+script",
			"--command", "init-draft",
			"--presentation-decision", decision,
		},
		DefaultAs: "bot",
		WorkDir:   workDir,
		Env:       docsScriptE2EEnv(t),
	})
	require.NoError(t, err)
	initialized.AssertExitCode(t, 0)
	initialized.AssertStdoutStatus(t, true)
	draftPath := gjson.Get(initialized.Stdout, "data.draft_path").String()
	workspace := gjson.Get(initialized.Stdout, "data.workspace").String()
	require.NotEmpty(t, draftPath)
	require.Equal(t, filepath.Dir(draftPath), workspace)
	require.False(t, gjson.Get(initialized.Stdout, "data.draft_file_created").Exists())
	require.Equal(t, "The workspace directory has been created successfully. draft_path points to a new XML file that does not exist yet. Create and write the file directly without reading it first.", gjson.Get(initialized.Stdout, "data.tip").String())
	require.Len(t, gjson.Get(initialized.Stdout, "data").Map(), 3)
	_, statErr := os.Stat(filepath.Join(workDir, draftPath))
	require.True(t, os.IsNotExist(statErr), "reserved draft XML already exists: %v", statErr)
	require.NoError(t, os.WriteFile(
		filepath.Join(workDir, draftPath),
		[]byte(`<title>标题</title><p>一个苹果是 an apple。</p><img/>`),
		0o600,
	))

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+script",
			"--command", "parse",
			"--content", "@./" + draftPath,
		},
		DefaultAs: "bot",
		WorkDir:   workDir,
		Env:       docsScriptE2EEnv(t),
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)
	require.Equal(t, "failed", gjson.Get(result.Stdout, "data.assessment.status").String())
	require.Equal(t, int64(3), gjson.Get(result.Stdout, "data.diagnostics.#").Int())
	require.Equal(t, "word_count_out_of_range", gjson.Get(result.Stdout, "data.diagnostics.0.code").String())
	require.Equal(t, int64(18), gjson.Get(result.Stdout, "data.diagnostics.0.expected.min").Int())
	require.Equal(t, int64(22), gjson.Get(result.Stdout, "data.diagnostics.0.expected.max").Int())
	require.Equal(t, int64(10), gjson.Get(result.Stdout, "data.diagnostics.0.actual").Int())
	require.Equal(t, "required_block_missing", gjson.Get(result.Stdout, "data.diagnostics.1.code").String())
	require.Equal(t, "whiteboard", gjson.Get(result.Stdout, "data.diagnostics.1.expected.type").String())
	require.Equal(t, int64(1), gjson.Get(result.Stdout, "data.diagnostics.1.expected.min_count").Int())
	require.Equal(t, int64(0), gjson.Get(result.Stdout, "data.diagnostics.1.actual").Int())
	require.Equal(t, "html5-block", gjson.Get(result.Stdout, "data.diagnostics.2.expected.type").String())
}

func TestDocsScriptInitializedDraftPreflightsBlockedRemoteImage(t *testing.T) {
	workDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	decision := `{"audience":"reader","reader_task":"read the draft","genre_contract":"none","adapter":null,"presentation_mode":"normal","visual_plan":{"reason":"an image provides visual evidence","blocks":[{"type":"img","min_count":1,"purpose":"show visual evidence"}]}}`

	initialized, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+script",
			"--command", "init-draft",
			"--presentation-decision", decision,
		},
		DefaultAs: "user",
		WorkDir:   workDir,
		Env:       docsScriptE2EEnv(t),
	})
	require.NoError(t, err)
	initialized.AssertExitCode(t, 0)
	draftPath := gjson.Get(initialized.Stdout, "data.draft_path").String()
	workspace := gjson.Get(initialized.Stdout, "data.workspace").String()
	require.NotEmpty(t, draftPath)
	require.Equal(t, filepath.Dir(draftPath), workspace)
	require.False(t, gjson.Get(initialized.Stdout, "data.draft_file_created").Exists())
	require.Equal(t, "The workspace directory has been created successfully. draft_path points to a new XML file that does not exist yet. Create and write the file directly without reading it first.", gjson.Get(initialized.Stdout, "data.tip").String())
	_, statErr := os.Stat(filepath.Join(workDir, draftPath))
	require.True(t, os.IsNotExist(statErr), "reserved draft XML already exists: %v", statErr)
	require.NoError(t, os.WriteFile(
		filepath.Join(workDir, draftPath),
		[]byte(`<title>Draft</title><img href="http://127.0.0.1/one.png"/><img href="http://127.0.0.1/two.png"/>`),
		0o600,
	))

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+script",
			"--command", "parse",
			"--content", "@./" + draftPath,
		},
		DefaultAs: "user",
		WorkDir:   workDir,
		Env:       docsScriptE2EEnv(t),
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)
	require.Equal(t, "failed", gjson.Get(result.Stdout, "data.assessment.status").String())
	require.Equal(t, int64(1), gjson.Get(result.Stdout, "data.diagnostics.#").Int())
	require.Equal(t, "remote_image_source_disallowed", gjson.Get(result.Stdout, "data.diagnostics.0.code").String())
	require.Equal(t, int64(2), gjson.Get(result.Stdout, "data.diagnostics.0.image_indices.#").Int())
	require.Equal(t, int64(1), gjson.Get(result.Stdout, "data.diagnostics.0.image_indices.0").Int())
	require.Equal(t, int64(2), gjson.Get(result.Stdout, "data.diagnostics.0.image_indices.1").Int())
	require.Equal(t, "local/internal host is not allowed", gjson.Get(result.Stdout, "data.diagnostics.0.msg").String())
	require.Contains(t, gjson.Get(result.Stdout, "data.diagnostics.0.suggested").String(), "Download the affected images")
	require.False(t, gjson.Get(result.Stdout, "data.warning").Exists())
}

func TestDocsScriptParseRepairsMalformedXMLForProfile(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+script",
			"--command", "parse",
			"--content", `<title>T</title><ul><li>one<li>two</ul><p>tail</p`,
		},
		DefaultAs: "bot",
		Env:       docsScriptE2EEnv(t),
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)
	require.Equal(t, int64(5), gjson.Get(result.Stdout, "data.profile.block_count").Int())
	require.Equal(t, int64(2), gjson.Get(result.Stdout, `data.profile.blocks.#(type=="li").count`).Int())
	require.False(t, gjson.Get(result.Stdout, "data.repairs").Exists())
	require.False(t, gjson.Get(result.Stdout, "data.diagnostics").Exists())
}

func TestDocsScriptStrictFlagIsRemoved(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+script",
			"--command", "parse",
			"--content", `<p>text</p>`,
			"--strict",
		},
		DefaultAs: "bot",
		Env:       docsScriptE2EEnv(t),
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String())
	require.Equal(t, "--strict", gjson.Get(result.Stderr, "error.params.0.name").String())
	require.Equal(t, "unknown flag", gjson.Get(result.Stderr, "error.params.0.reason").String())
}

func TestDocsScriptFileNameFlagIsRemoved(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+script",
			"--command", "init-draft",
			"--file-name", "draft.xml",
			"--presentation-decision", `{"audience":"reader","reader_task":"read the draft","genre_contract":"none","adapter":null,"presentation_mode":"normal","visual_plan":{"reason":"plain text is sufficient","blocks":[]}}`,
		},
		DefaultAs: "bot",
		Env:       docsScriptE2EEnv(t),
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String())
	require.Equal(t, "--file-name", gjson.Get(result.Stderr, "error.params.0.name").String())
	require.Equal(t, "unknown flag", gjson.Get(result.Stderr, "error.params.0.reason").String())
}

func TestDocsScriptMarkdownToXMLIsRemoved(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+script",
			"--command", "markdown-to-xml",
			"--content", "# title",
			"--dry-run",
		},
		DefaultAs: "bot",
		Env:       docsScriptE2EEnv(t),
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.Equal(t, "--command", gjson.Get(result.Stderr, "error.param").String())
	require.Contains(t, gjson.Get(result.Stderr, "error.message").String(), `invalid value "markdown-to-xml"`)
}

func TestDocsScriptCreateTempXMLIsRemoved(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+script",
			"--command", "create-temp-xml",
			"--dry-run",
		},
		DefaultAs: "bot",
		Env:       docsScriptE2EEnv(t),
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.Equal(t, "--command", gjson.Get(result.Stderr, "error.param").String())
	require.Contains(t, gjson.Get(result.Stderr, "error.message").String(), `invalid value "create-temp-xml"`)
}

func TestDocsScriptParseAcceptsLocalImagePath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+script",
			"--command", "parse",
			"--content", `<title>Local image</title><img path="@diagram.png" caption="diagram"/>`,
		},
		DefaultAs: "bot",
		Env:       docsScriptE2EEnv(t),
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)
	require.Equal(t, int64(2), gjson.Get(result.Stdout, "data.profile.block_count").Int())
	require.Equal(t, int64(1), gjson.Get(result.Stdout, `data.profile.blocks.#(type=="img").count`).Int())
}

func TestDocsScriptParseDoesNotSupportLegacyQAImage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+script",
			"--command", "parse",
			"--content", `<qa_image><image_key=img_v3_abc w=320 h=200></qa_image>`,
		},
		DefaultAs: "bot",
		Env:       docsScriptE2EEnv(t),
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)
	require.False(t, gjson.Get(result.Stdout, `data.profile.blocks.#(type=="img").count`).Exists())
}

func TestDocsScriptParseAcceptsServerSDKAttributeAmpersand(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	input := `<block_insert><parameter><block_id>-1</block_id><content><img href="https://picsum.photos/320/200?seed=lark-cli&raw=1"/></content></parameter></block_insert>`
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"docs", "+script", "--command", "parse", "--content", input},
		DefaultAs: "bot",
		Env:       docsScriptE2EEnv(t),
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)
	require.Equal(t, int64(1), gjson.Get(result.Stdout, "data.profile.block_count").Int())
}

func TestDocsScriptParseRejectsMarkdownFromFile(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "draft.md"), []byte("# 标题\n\n- item"), 0o600))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+script",
			"--command", "parse",
			"--content", "@draft.md",
		},
		DefaultAs: "bot",
		WorkDir:   workDir,
		Env:       docsScriptE2EEnv(t),
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.False(t, gjson.Get(result.Stderr, "ok").Bool())
	require.Equal(t, "--content", gjson.Get(result.Stderr, "error.param").String())
	require.Contains(t, gjson.Get(result.Stderr, "error.message").String(), "as LarkOpenCLI XML")
}

func TestDocsScriptInitDraftCreatesUniqueWorkspacesWithoutXML(t *testing.T) {
	workDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	decision := `{"audience":"reader","reader_task":"read the draft","genre_contract":"none","adapter":null,"presentation_mode":"normal","visual_plan":{"reason":"plain text is sufficient","blocks":[]}}`

	const count = 8
	type outcome struct {
		result *clie2e.Result
		err    error
	}
	outcomes := make(chan outcome, count)
	for i := 0; i < count; i++ {
		go func() {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{
					"docs", "+script", "--command", "init-draft",
					"--presentation-decision", decision,
				},
				DefaultAs: "bot",
				WorkDir:   workDir,
				Env:       docsScriptE2EEnv(t),
			})
			outcomes <- outcome{result: result, err: err}
		}()
	}

	seen := make(map[string]struct{}, count)
	for i := 0; i < count; i++ {
		outcome := <-outcomes
		require.NoError(t, outcome.err)
		require.NotNil(t, outcome.result)
		outcome.result.AssertExitCode(t, 0)
		outcome.result.AssertStdoutStatus(t, true)

		path := gjson.Get(outcome.result.Stdout, "data.draft_path").String()
		directory := filepath.Dir(path)
		workspace := gjson.Get(outcome.result.Stdout, "data.workspace").String()
		randomPart := strings.TrimSuffix(strings.TrimPrefix(directory, "draft_"), "_folder")
		require.Equal(t, directory, workspace)
		require.False(t, gjson.Get(outcome.result.Stdout, "data.draft_file_created").Exists())
		require.Equal(t, "The workspace directory has been created successfully. draft_path points to a new XML file that does not exist yet. Create and write the file directly without reading it first.", gjson.Get(outcome.result.Stdout, "data.tip").String())
		require.Equal(t, "draft.xml", filepath.Base(path))
		require.Equal(t, filepath.Base(directory), directory)
		require.True(t, strings.HasPrefix(directory, "draft_"), "path: %q", path)
		require.True(t, strings.HasSuffix(directory, "_folder"), "path: %q", path)
		require.Len(t, randomPart, 8)
		_, duplicate := seen[path]
		require.False(t, duplicate, "duplicate draft path: %q", path)
		seen[path] = struct{}{}
		_, statErr := os.Stat(filepath.Join(workDir, path))
		require.True(t, os.IsNotExist(statErr), "reserved draft XML already exists: %q, err=%v", path, statErr)
		savedDecision, err := os.ReadFile(filepath.Join(workDir, directory, ".presentation-decision.json"))
		require.NoError(t, err)
		require.Equal(t, decision, string(savedDecision))
	}
	require.Len(t, seen, count)
}

func TestDocsScriptDryRunIsLocal(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+script",
			"--command", "parse",
			"--content", `<p>text</p>`,
			"--dry-run",
		},
		DefaultAs: "bot",
		Env:       docsScriptE2EEnv(t),
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	require.Equal(t, int64(0), gjson.Get(result.Stdout, "data.api.#").Int())
	require.False(t, gjson.Get(result.Stdout, "data.network").Bool())
	require.False(t, gjson.Get(result.Stdout, "data.strict").Exists())
	require.Equal(t, "parse", gjson.Get(result.Stdout, "data.command").String())
	require.False(t, gjson.Get(result.Stdout, "data.presentation_decision").Exists())
}

func TestDocsScriptInitDraftDryRunDoesNotWrite(t *testing.T) {
	workDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+script",
			"--command", "init-draft",
			"--presentation-decision", `{"audience":"reader","reader_task":"understand the topic","genre_contract":"none","adapter":null,"presentation_mode":"rich","visual_plan":{"reason":"compatibility check for ordinary lists","blocks":[{"type":"list","min_count":1,"purpose":"group related items"}]}}`,
			"--dry-run",
		},
		DefaultAs: "bot",
		WorkDir:   workDir,
		Env:       docsScriptE2EEnv(t),
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	require.Equal(t, int64(0), gjson.Get(result.Stdout, "data.api.#").Int())
	require.False(t, gjson.Get(result.Stdout, "data.network").Bool())
	require.True(t, gjson.Get(result.Stdout, "data.creates_workspace").Bool())
	require.False(t, gjson.Get(result.Stdout, "data.creates_draft_file").Bool())
	require.Equal(t, "init-draft", gjson.Get(result.Stdout, "data.command").String())
	require.Equal(t, "draft_*_folder", gjson.Get(result.Stdout, "data.directory_pattern").String())
	require.Equal(t, "draft.xml", gjson.Get(result.Stdout, "data.xml_file_name").String())
	require.False(t, gjson.Get(result.Stdout, "data.file_name").Exists())
	require.True(t, gjson.Get(result.Stdout, "data.presentation_decision").Bool())
	entries, err := os.ReadDir(workDir)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestDocsScriptInitDraftAcceptsWindowsCommandShimQuotes(t *testing.T) {
	workDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	decision := `{"audience":"reader","reader_task":"understand the topic","genre_contract":"none","adapter":null,"presentation_mode":"normal","visual_plan":{"reason":"plain text is sufficient","blocks":[]}}`
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+script",
			"--command", "init-draft",
			"--presentation-decision", "'" + decision + "'",
			"--dry-run",
		},
		DefaultAs: "bot",
		WorkDir:   workDir,
		Env:       docsScriptE2EEnv(t),
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	require.Equal(t, "init-draft", gjson.Get(result.Stdout, "data.command").String())
	require.True(t, gjson.Get(result.Stdout, "data.presentation_decision").Bool())
	entries, err := os.ReadDir(workDir)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestDocsScriptRecoversPowerShellDequotedPresentationDecision(t *testing.T) {
	for _, decision := range []string{
		`{audience:a,reader_task:b,genre_contract:null,adapter:null,presentation_mode:normal,visual_plan:{reason:c,blocks:[]}}`,
		`{"audience":a,"reader_task":b,"genre_contract":null,"adapter":null,"presentation_mode":normal,"visual_plan":{"reason":c,"blocks":[]}}`,
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+script",
				"--command", "init-draft",
				"--presentation-decision", decision,
				"--dry-run",
			},
			DefaultAs: "bot",
			WorkDir:   t.TempDir(),
			Env:       docsScriptE2EEnv(t),
		})
		cancel()
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.Equal(t, "init-draft", gjson.Get(result.Stdout, "data.command").String())
		require.True(t, gjson.Get(result.Stdout, "data.presentation_decision").Bool())
	}
}

func TestDocsScriptAmbiguousMangledPresentationDecisionSuggestsFileInput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+script",
			"--command", "init-draft",
			"--presentation-decision", `{audience:reader,reviewer,reader_task:understand}`,
			"--dry-run",
		},
		DefaultAs: "bot",
		WorkDir:   t.TempDir(),
		Env:       docsScriptE2EEnv(t),
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.Empty(t, result.Stdout)
	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String())
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String())
	require.Contains(t, gjson.Get(result.Stderr, "error.message").String(), "--presentation-decision must be a valid Presentation Decision JSON object")
	require.Equal(t, "--presentation-decision", gjson.Get(result.Stderr, "error.param").String())
	require.Equal(t, "restore the original JSON quotes; if shell quote loss made a string ambiguous, save the original JSON as UTF-8 and pass --presentation-decision \"@./decision.json\"", gjson.Get(result.Stderr, "error.hint").String())
}

func TestDocsScriptInitDraftRejectsNullWordCountWithOmitGuidance(t *testing.T) {
	workDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+script",
			"--command", "init-draft",
			"--presentation-decision", `{"audience":"reader","reader_task":"understand the topic","genre_contract":"none","adapter":null,"presentation_mode":"normal","word_count":null,"visual_plan":{"reason":"plain text is sufficient","blocks":[]}}`,
			"--dry-run",
		},
		DefaultAs: "bot",
		WorkDir:   workDir,
		Env:       docsScriptE2EEnv(t),
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.Equal(t, "--presentation-decision", gjson.Get(result.Stderr, "error.param").String())
	require.Contains(t, gjson.Get(result.Stderr, "error.message").String(), "word_count must be omitted")
	require.Contains(t, gjson.Get(result.Stderr, "error.hint").String(), "remove the word_count field")
	entries, readErr := os.ReadDir(workDir)
	require.NoError(t, readErr)
	require.Empty(t, entries)
}

func TestDocsScriptOnlineDryRunFetchesXML(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+script",
			"--command", "parse",
			"--doc", "https://example.larksuite.com/docx/doxcnScriptDryRun",
			"--dry-run",
		},
		DefaultAs: "bot",
		Env:       docsScriptE2EEnv(t),
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	require.Equal(t, int64(1), gjson.Get(result.Stdout, "data.api.#").Int())
	require.Equal(t, "POST", gjson.Get(result.Stdout, "data.api.0.method").String())
	require.Equal(t, "/open-apis/docs_ai/v1/documents/doxcnScriptDryRun/fetch", gjson.Get(result.Stdout, "data.api.0.url").String())
	require.Equal(t, "xml", gjson.Get(result.Stdout, "data.api.0.body.format").String())
	require.True(t, gjson.Get(result.Stdout, "data.network").Bool())
	require.False(t, gjson.Get(result.Stdout, "data.strict").Exists())
	require.Equal(t, "parse", gjson.Get(result.Stdout, "data.command").String())
}

func docsScriptE2EEnv(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{
		"LARKSUITE_CLI_APP_ID":     "docs-script-e2e",
		"LARKSUITE_CLI_APP_SECRET": "secret",
		"LARKSUITE_CLI_BRAND":      "feishu",
		"LARKSUITE_CLI_CONFIG_DIR": t.TempDir(),
	}
}
