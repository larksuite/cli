// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/doc/internal/docxparse"
)

func TestDocsScriptDoesNotExposeRemovedCommandsOrFlags(t *testing.T) {
	foundCommand := false
	for _, flag := range DocsScript.Flags {
		switch flag.Name {
		case "strict", "output", "overwrite", "file-name":
			t.Fatalf("docs +script still exposes removed --%s flag", flag.Name)
		case "command":
			foundCommand = true
			for _, value := range flag.Enum {
				if value == "markdown-to-xml" || value == "create-temp-xml" {
					t.Fatalf("docs +script still exposes removed %s command", value)
				}
			}
		}
	}
	if !foundCommand {
		t.Fatal("docs +script does not expose --command")
	}
}

func TestDocsScriptPresentationDecisionFlagAcceptsFileAndStdin(t *testing.T) {
	for _, flag := range DocsScript.Flags {
		if flag.Name != "presentation-decision" {
			continue
		}
		if len(flag.Input) != 2 || flag.Input[0] != common.File || flag.Input[1] != common.Stdin {
			t.Fatalf("presentation-decision Input = %#v, want file and stdin", flag.Input)
		}
		for _, want := range []string{"genre_contract and adapter", `"none"`, "or null"} {
			if !strings.Contains(flag.Desc, want) {
				t.Fatalf("presentation-decision help = %q, want it to contain %q", flag.Desc, want)
			}
		}
		return
	}
	t.Fatal("docs +script does not expose --presentation-decision")
}

func TestDocsScriptParsesAndProfilesXML(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-test"))
	source := `<title>标题</title><p>一个苹果是 an apple。</p>`

	err := mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptParse,
		"--content", source,
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("execute docs +script: %v", err)
	}

	var envelope struct {
		OK   bool                       `json:"ok"`
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout)
	}
	if !envelope.OK {
		t.Fatalf("ok = false: %s", stdout)
	}
	if len(envelope.Data) != 1 || envelope.Data["profile"] == nil {
		t.Fatalf("data = %+v, want only profile", envelope.Data)
	}
	var profile docsScriptPublicProfile
	if err := json.Unmarshal(envelope.Data["profile"], &profile); err != nil {
		t.Fatalf("decode profile: %v", err)
	}
	var profileFields map[string]json.RawMessage
	if err := json.Unmarshal(envelope.Data["profile"], &profileFields); err != nil {
		t.Fatalf("decode profile fields: %v", err)
	}
	if len(profileFields) != 4 || profileFields["breakdown"] != nil {
		t.Fatalf("profile fields = %+v, want breakdown hidden", profileFields)
	}
	if profile.WordCount != 10 || profile.CharCount != 15 || profile.BlockCount != 2 {
		t.Fatalf("profile = %+v", profile)
	}
	if got := blockCount(profile.Blocks, "title"); got != 1 {
		t.Fatalf("title count = %d, want 1", got)
	}
	if got := blockCount(profile.Blocks, "p"); got != 1 {
		t.Fatalf("p count = %d, want 1", got)
	}
}

func TestDocsScriptReturnsPresentationDecisionWarnings(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-presentation-warnings"))
	decision := `{
		"audience": "reader",
		"reader_task": "understand the result and flow",
		"genre_contract": "none",
		"adapter": null,
		"presentation_mode": "rich",
		"word_count": {"min": 18, "max": 22},
		"visual_plan": {
			"reason": "visual explanation",
			"blocks": [
				{"type":"img","min_count":1,"purpose":"show the result"},
				{"type":"whiteboard","min_count":1,"purpose":"show the flow"},
				{"type":"html5-block","min_count":1,"purpose":"make the state explorable"}
			]
		}
	}`

	err := mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptParse,
		"--content", `<title>标题</title><p>一个苹果是 an apple。</p><img/>`,
		"--presentation-decision", decision,
		"--as", "bot",
	}, f, stdout)
	requireDocsScriptWarningPartialFailure(t, err)

	var envelope struct {
		OK   bool                  `json:"ok"`
		Data docsScriptParseResult `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout)
	}
	if envelope.OK {
		t.Fatalf("warning result reported ok:true: %s", stdout)
	}
	if len(envelope.Data.Warning) != 3 {
		t.Fatalf("warning = %#v, want word-count, whiteboard, and html warnings", envelope.Data.Warning)
	}
	for _, want := range []string{"expects range 18-22", "at least 1 whiteboard block", "at least 1 html5-block block"} {
		if !containsDocsScriptWarning(envelope.Data.Warning, want) {
			t.Errorf("warning = %#v, want substring %q", envelope.Data.Warning, want)
		}
	}
	if containsDocsScriptWarning(envelope.Data.Warning, "at least 1 img block") {
		t.Fatalf("warning = %#v, img block exists", envelope.Data.Warning)
	}
}

func TestDocsScriptPresentationDecisionPreflightsBlockedRemoteImage(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-resource-preflight"))
	decision := `{"audience":"reader","reader_task":"read the draft","genre_contract":"none","adapter":null,"presentation_mode":"normal","visual_plan":{"reason":"an image provides visual evidence","blocks":[{"type":"img","min_count":1,"purpose":"show visual evidence"}]}}`

	err := mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptParse,
		"--content", `<title>Draft</title><img href="http://127.0.0.1/image.png"/>`,
		"--presentation-decision", decision,
		"--as", "bot",
	}, f, stdout)
	requireDocsScriptWarningPartialFailure(t, err)

	var envelope struct {
		Data docsScriptParseResult `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout)
	}
	if len(envelope.Data.Warning) != 1 {
		t.Fatalf("warning = %#v, want one resource warning", envelope.Data.Warning)
	}
	warning := envelope.Data.Warning[0]
	for _, want := range []string{
		"resource preflight failed",
		"remote image #1 href is not allowed: local/internal host is not allowed",
		`use a public HTTP(S) image URL`,
	} {
		if !strings.Contains(warning, want) {
			t.Fatalf("warning = %q, want substring %q", warning, want)
		}
	}
}

func TestDocsScriptPresentationDecisionProbesRemoteImageDownload(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-resource-download-probe"))
	decision := `{"audience":"reader","reader_task":"read the draft","genre_contract":"none","adapter":null,"presentation_mode":"normal","visual_plan":{"reason":"an image provides visual evidence","blocks":[{"type":"img","min_count":1,"purpose":"show visual evidence"}]}}`

	originalDo := doRemoteDocImageRequest
	t.Cleanup(func() { doRemoteDocImageRequest = originalDo })
	requestMethod := ""
	requestRange := ""
	doRemoteDocImageRequest = func(_ remoteDocImageHTTPDoer, req *http.Request) (*http.Response, error) {
		requestMethod = req.Method
		requestRange = req.Header.Get("Range")
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     http.Header{"Content-Type": []string{"image/png"}},
			Body:       io.NopCloser(strings.NewReader("not found")),
		}, nil
	}

	err := mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptParse,
		"--content", `<title>Draft</title><img href="https://93.184.216.34/image.png"/>`,
		"--presentation-decision", decision,
	}, f, stdout)
	requireDocsScriptWarningPartialFailure(t, err)

	var envelope struct {
		Data docsScriptParseResult `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout)
	}
	if requestMethod != http.MethodGet || requestRange != "bytes=0-0" {
		t.Fatalf("remote image probe method = %q range = %q, want ranged GET", requestMethod, requestRange)
	}
	if !containsDocsScriptWarning(envelope.Data.Warning, "HTTP 404") {
		t.Fatalf("warning = %#v, want failed remote image availability probe", envelope.Data.Warning)
	}
}

func TestDocsScriptParseWithoutPresentationDecisionSkipsResourcePreflight(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-no-resource-preflight"))
	err := mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptParse,
		"--content", `<title>Draft</title><img href="http://127.0.0.1/image.png"/>`,
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("execute docs +script: %v", err)
	}

	var envelope struct {
		Data docsScriptParseResult `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout)
	}
	if len(envelope.Data.Warning) != 0 {
		t.Fatalf("warning = %#v, want no resource preflight without a Presentation Decision", envelope.Data.Warning)
	}
}

func TestDocsScriptInitDraftPersistsDecisionForAutomaticParse(t *testing.T) {
	workDir := t.TempDir()
	withDocsWorkingDir(t, workDir)
	f, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-init-draft"))
	decision := `{"audience":"reader","reader_task":"understand the result and flow","genre_contract":"none","adapter":null,"presentation_mode":"rich","word_count":{"min":18,"max":22},"visual_plan":{"reason":"visual explanation","blocks":[{"type":"img","min_count":1,"purpose":"show the result"},{"type":"whiteboard","min_count":1,"purpose":"show the flow"},{"type":"html5-block","min_count":1,"purpose":"make the state explorable"}]}}`

	err := mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptInitDraft,
		"--presentation-decision", decision,
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("initialize draft: %v", err)
	}
	var initialized struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &initialized); err != nil {
		t.Fatalf("decode init output: %v\n%s", err, stdout)
	}
	if len(initialized.Data) != 3 || initialized.Data["workspace"] == nil || initialized.Data["draft_path"] == nil || initialized.Data["tip"] == nil {
		t.Fatalf("init data = %#v, want workspace, draft_path, and tip", initialized.Data)
	}
	var result docsScriptDraftResult
	rawResult, err := json.Marshal(initialized.Data)
	if err != nil {
		t.Fatalf("encode init data: %v", err)
	}
	if err := json.Unmarshal(rawResult, &result); err != nil {
		t.Fatalf("decode init data: %v", err)
	}
	draftPath := result.DraftPath
	if result.Tip != docsScriptDraftTip {
		t.Fatalf("tip = %q, want %q", result.Tip, docsScriptDraftTip)
	}
	if result.Workspace != filepath.Dir(draftPath) {
		t.Fatalf("workspace = %q, want parent of draft_path %q", result.Workspace, draftPath)
	}
	if filepath.IsAbs(result.Workspace) || filepath.Dir(result.Workspace) != "." {
		t.Fatalf("workspace = %q, want a generated relative directory", result.Workspace)
	}
	if filepath.IsAbs(draftPath) || filepath.Dir(draftPath) == "." {
		t.Fatalf("draft path = %q, want a relative path inside a generated workspace", draftPath)
	}
	if _, err := os.Stat(draftPath); !os.IsNotExist(err) {
		t.Fatalf("reserved draft XML path already exists: %q, err=%v", draftPath, err)
	}
	savedDecision, err := os.ReadFile(filepath.Join(filepath.Dir(draftPath), docsScriptDecisionFile))
	if err != nil {
		t.Fatalf("read saved decision: %v", err)
	}
	if string(savedDecision) != decision {
		t.Fatalf("saved decision = %q, want %q", savedDecision, decision)
	}
	if err := os.WriteFile(draftPath, []byte(`<title>标题</title><p>一个苹果是 an apple。</p><img/>`), 0o600); err != nil {
		t.Fatalf("write draft: %v", err)
	}

	stdout.Reset()
	err = mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptParse,
		"--content", "@./" + draftPath,
		"--as", "bot",
	}, f, stdout)
	requireDocsScriptWarningPartialFailure(t, err)
	var parsed struct {
		Data docsScriptParseResult `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		t.Fatalf("decode parse output: %v\n%s", err, stdout)
	}
	if len(parsed.Data.Warning) != 3 {
		t.Fatalf("warning = %#v, want persisted word-count, whiteboard, and html warnings", parsed.Data.Warning)
	}
}

func TestDocsScriptInitDraftRequiresPresentationDecision(t *testing.T) {
	workDir := t.TempDir()
	withDocsWorkingDir(t, workDir)
	f, _, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-init-draft-required"))
	err := mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptInitDraft,
		"--as", "bot",
	}, f, nil)
	assertValidationContract(t, err, errs.SubtypeInvalidArgument, "--presentation-decision")
	entries, readErr := os.ReadDir(workDir)
	if readErr != nil {
		t.Fatalf("read work directory: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("failed init created files: %#v", entries)
	}
}

func TestDocsScriptPresentationDecisionWordCountRangeIsInclusive(t *testing.T) {
	minimum, maximum := 9, 11
	decision := docsScriptPresentationDecision{WordCount: &docsScriptPresentationWordCount{Min: &minimum, Max: &maximum}}
	tests := []struct {
		wordCount int
		wantWarn  bool
	}{
		{wordCount: 8, wantWarn: true},
		{wordCount: 9, wantWarn: false},
		{wordCount: 10, wantWarn: false},
		{wordCount: 11, wantWarn: false},
		{wordCount: 12, wantWarn: true},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("word-count-%d", test.wordCount), func(t *testing.T) {
			warnings := docsScriptPresentationWarnings(docsScriptPublicProfile{WordCount: test.wordCount}, decision)
			if got := len(warnings) > 0; got != test.wantWarn {
				t.Fatalf("warnings = %#v, want warning=%v", warnings, test.wantWarn)
			}
		})
	}
}

func TestDocsScriptPresentationDecisionSupportsOneSidedWordCountBounds(t *testing.T) {
	tests := []struct {
		name      string
		wordCount int
		decision  string
		want      string
	}{
		{
			name:      "minimum",
			wordCount: 9,
			decision:  `{"audience":"reader","reader_task":"understand","genre_contract":"knowledge","adapter":null,"presentation_mode":"normal","word_count":{"min":10,"max":null},"visual_plan":{"reason":"plain text is sufficient","blocks":[]}}`,
			want:      "expects at least 10",
		},
		{
			name:      "maximum",
			wordCount: 21,
			decision:  `{"audience":"reader","reader_task":"understand","genre_contract":"knowledge","adapter":"wechat","presentation_mode":"normal","word_count":{"min":null,"max":20},"visual_plan":{"reason":"plain text is sufficient","blocks":[]}}`,
			want:      "expects at most 20",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision, err := parseDocsScriptPresentationDecision(test.decision)
			if err != nil {
				t.Fatalf("parse decision: %v", err)
			}
			warnings := docsScriptPresentationWarnings(docsScriptPublicProfile{WordCount: test.wordCount}, decision)
			if len(warnings) != 1 || !strings.Contains(warnings[0], test.want) {
				t.Fatalf("warnings = %#v, want one containing %q", warnings, test.want)
			}
		})
	}
}

func TestDocsScriptPresentationDecisionListCountsULAndOL(t *testing.T) {
	decision, err := parseDocsScriptPresentationDecision(`{
		"audience":"reader",
		"reader_task":"review recommendations",
		"genre_contract":"none",
		"adapter":null,
		"presentation_mode":"normal",
		"visual_plan":{
			"reason":"group recommendations",
			"blocks":[{"type":"list","min_count":2,"purpose":"separate equipment and food recommendations"}]
		}
	}`)
	if err != nil {
		t.Fatalf("parse list Presentation Decision: %v", err)
	}
	profile := docsScriptPublicProfile{Blocks: []docxparse.BlockShare{
		{Type: "ul", Count: 1},
		{Type: "ol", Count: 1},
	}}
	if warnings := docsScriptPresentationWarnings(profile, decision); len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want ul + ol to satisfy two list blocks", warnings)
	}

	decision.VisualPlan.Blocks[0].MinCount = 3
	warnings := docsScriptPresentationWarnings(profile, decision)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "at least 3 list block(s)") || !strings.Contains(warnings[0], "draft has 2") {
		t.Fatalf("warnings = %#v, want list count 2", warnings)
	}
}

func TestDocsScriptOmitsWarningWhenPresentationDecisionPasses(t *testing.T) {
	workDir := t.TempDir()
	withDocsWorkingDir(t, workDir)
	if err := os.WriteFile("flow.mmd", []byte("flowchart LR\nA --> B"), 0o600); err != nil {
		t.Fatalf("write whiteboard source: %v", err)
	}
	if err := os.WriteFile("widget.html", []byte("<html><body>status</body></html>"), 0o600); err != nil {
		t.Fatalf("write HTML source: %v", err)
	}
	f, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-presentation-pass"))
	decision := `{
		"audience": "reader",
		"reader_task": "understand the result and flow",
		"genre_contract": "none",
		"adapter": null,
		"presentation_mode": "rich",
		"word_count": {"min": 9, "max": 11},
		"visual_plan": {
			"reason": "visual explanation",
			"blocks": [
				{"type":"img","min_count":1,"purpose":"show the result"},
				{"type":"whiteboard","min_count":1,"purpose":"show the flow"},
				{"type":"html5-block","min_count":1,"purpose":"make the state explorable"}
			]
		}
	}`

	err := mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptParse,
		"--content", `<title>标题</title><p>一个苹果是 an apple。</p><img/><whiteboard type="mermaid" path="@flow.mmd"></whiteboard><html5-block path="@widget.html"></html5-block>`,
		"--presentation-decision", decision,
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("execute docs +script: %v", err)
	}

	var envelope struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout)
	}
	if envelope.Data["warning"] != nil {
		t.Fatalf("passing decision should omit data.warning: %s", stdout)
	}
}

func TestDocsScriptRejectsInvalidPresentationDecision(t *testing.T) {
	validPrefix := `{"audience":"reader","reader_task":"understand the document","genre_contract":"none","adapter":null,"presentation_mode":"normal",`
	tests := []struct {
		name     string
		decision string
	}{
		{name: "invalid json", decision: `{`},
		{name: "unknown field", decision: validPrefix + `"visual_plan":{"reason":"plain is enough","blocks":[],"image_enabled":true}}`},
		{name: "legacy fields", decision: `{"target":"reader","genre_contract":"none","adapter":"none","presentation_mode":"normal","hard_rule":"","word_count":20,"visual_plan":{"reason":"plain is enough","exception":"","blocks":[]}}`},
		{name: "integer word count", decision: `{"audience":"reader","reader_task":"understand","genre_contract":"none","adapter":null,"presentation_mode":"normal","word_count":20,"visual_plan":{"reason":"plain is enough","blocks":[]}}`},
		{name: "missing word count bound", decision: `{"audience":"reader","reader_task":"understand","genre_contract":"none","adapter":null,"presentation_mode":"normal","word_count":{"min":10},"visual_plan":{"reason":"plain is enough","blocks":[]}}`},
		{name: "empty word count range", decision: `{"audience":"reader","reader_task":"understand","genre_contract":"none","adapter":null,"presentation_mode":"normal","word_count":{"min":null,"max":null},"visual_plan":{"reason":"plain is enough","blocks":[]}}`},
		{name: "non-positive word count", decision: `{"audience":"reader","reader_task":"understand","genre_contract":"none","adapter":null,"presentation_mode":"normal","word_count":{"min":0,"max":10},"visual_plan":{"reason":"plain is enough","blocks":[]}}`},
		{name: "reversed word count range", decision: `{"audience":"reader","reader_task":"understand","genre_contract":"none","adapter":null,"presentation_mode":"normal","word_count":{"min":20,"max":10},"visual_plan":{"reason":"plain is enough","blocks":[]}}`},
		{name: "missing required field", decision: `{"reader_task":"understand","genre_contract":"none","adapter":null,"presentation_mode":"normal","visual_plan":{"reason":"plain is enough","blocks":[]}}`},
		{name: "missing adapter", decision: `{"audience":"reader","reader_task":"understand","genre_contract":"none","presentation_mode":"normal","visual_plan":{"reason":"plain is enough","blocks":[]}}`},
		{name: "empty genre contract", decision: `{"audience":"reader","reader_task":"understand","genre_contract":" ","adapter":null,"presentation_mode":"normal","visual_plan":{"reason":"plain is enough","blocks":[]}}`},
		{name: "empty adapter", decision: `{"audience":"reader","reader_task":"understand","genre_contract":"none","adapter":" ","presentation_mode":"normal","visual_plan":{"reason":"plain is enough","blocks":[]}}`},
		{name: "removed hard_rules field", decision: `{"audience":"reader","reader_task":"understand","genre_contract":"none","adapter":null,"presentation_mode":"normal","hard_rules":[],"visual_plan":{"reason":"plain is enough","blocks":[]}}`},
		{name: "invalid presentation mode", decision: `{"audience":"reader","reader_task":"understand","genre_contract":"none","adapter":null,"presentation_mode":"decorative","visual_plan":{"reason":"visual","blocks":[]}}`},
		{name: "null block plan", decision: validPrefix + `"visual_plan":{"reason":"visual","blocks":null}}`},
		{name: "unknown block type", decision: validPrefix + `"visual_plan":{"reason":"visual","blocks":[{"type":"future-widget","min_count":1,"purpose":"show the result"}]}}`},
		{name: "ordinary text block is not presentation block", decision: validPrefix + `"visual_plan":{"reason":"visual","blocks":[{"type":"p","min_count":2,"purpose":"fill the quota"}]}}`},
		{name: "non-positive block minimum", decision: validPrefix + `"visual_plan":{"reason":"visual","blocks":[{"type":"img","min_count":0,"purpose":"show the result"}]}}`},
		{name: "missing block purpose", decision: validPrefix + `"visual_plan":{"reason":"visual","blocks":[{"type":"img","min_count":1,"purpose":""}]}}`},
		{name: "duplicate block type", decision: validPrefix + `"visual_plan":{"reason":"visual","blocks":[{"type":"img","min_count":1,"purpose":"one"},{"type":"img","min_count":1,"purpose":"two"}]}}`},
		{name: "multiple values", decision: `{} {}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f, _, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-presentation-invalid"))
			err := mountAndRunDocs(t, DocsScript, []string{
				"+script",
				"--command", docsScriptParse,
				"--content", `<p>text</p>`,
				"--presentation-decision", test.decision,
				"--as", "bot",
			}, f, nil)
			assertValidationContract(t, err, errs.SubtypeInvalidArgument, "--presentation-decision")
		})
	}
}

func TestDocsScriptPresentationDecisionAllowsNoneOrNullRoutes(t *testing.T) {
	tests := []struct {
		name           string
		genreContract  string
		adapter        string
		wantGenreNil   bool
		wantAdapterNil bool
	}{
		{name: "both none", genreContract: `"none"`, adapter: `"none"`},
		{name: "both null", genreContract: `null`, adapter: `null`, wantGenreNil: true, wantAdapterNil: true},
		{name: "null genre", genreContract: `null`, adapter: `"none"`, wantGenreNil: true},
		{name: "null adapter", genreContract: `"none"`, adapter: `null`, wantAdapterNil: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision, err := parseDocsScriptPresentationDecision(fmt.Sprintf(
				`{"audience":"reader","reader_task":"understand","genre_contract":%s,"adapter":%s,"presentation_mode":"normal","visual_plan":{"reason":"plain is enough","blocks":[]}}`,
				test.genreContract,
				test.adapter,
			))
			if err != nil {
				t.Fatalf("parseDocsScriptPresentationDecision() error = %v", err)
			}
			if got := decision.GenreContract == nil; got != test.wantGenreNil {
				t.Fatalf("GenreContract nil = %v, want %v", got, test.wantGenreNil)
			}
			if got := decision.Adapter == nil; got != test.wantAdapterNil {
				t.Fatalf("Adapter nil = %v, want %v", got, test.wantAdapterNil)
			}
		})
	}
}

func TestDocsScriptPresentationDecisionRejectsNullWordCountWithOmitGuidance(t *testing.T) {
	_, err := parseDocsScriptPresentationDecision(`{"audience":"reader","reader_task":"understand","genre_contract":"none","adapter":null,"presentation_mode":"normal","word_count":null,"visual_plan":{"reason":"plain text is sufficient","blocks":[]}}`)
	assertValidationContract(t, err, errs.SubtypeInvalidArgument, "--presentation-decision")
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error does not expose a typed problem: %v", err)
	}
	if got, want := problem.Message, "--presentation-decision word_count must be omitted when no word-count requirement was requested"; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
	if got, want := problem.Hint, "remove the word_count field; include it only when the user requested a word-count bound"; got != want {
		t.Fatalf("hint = %q, want %q", got, want)
	}
}

func TestDocsScriptRichPresentationDecisionAllowsAnyBlockCount(t *testing.T) {
	tests := []struct {
		name     string
		decision string
	}{
		{
			name:     "no planned blocks",
			decision: `{"audience":"reader","reader_task":"understand the topic","genre_contract":"knowledge","adapter":null,"presentation_mode":"rich","visual_plan":{"reason":"the scan found no presentation block that would improve the reader task","blocks":[]}}`,
		},
		{
			name:     "one planned block",
			decision: `{"audience":"reader","reader_task":"understand the relationship","genre_contract":"knowledge","adapter":null,"presentation_mode":"rich","visual_plan":{"reason":"one diagram is sufficient","blocks":[{"type":"whiteboard","min_count":1,"purpose":"show the relationship"}]}}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseDocsScriptPresentationDecision(test.decision); err != nil {
				t.Fatalf("parseDocsScriptPresentationDecision() error: %v", err)
			}
		})
	}
}

func TestDocsScriptPresentationDecisionRejectsRemovedHardRulesField(t *testing.T) {
	_, err := parseDocsScriptPresentationDecision(`{
		"audience":"executives",
		"reader_task":"approve or reject the proposal",
		"genre_contract":"formal-doc",
		"adapter":null,
		"presentation_mode":"formal",
		"hard_rules":["do not use callout blocks","include a risks section"],
		"visual_plan":{
			"reason":"a restrained structure best supports formal approval",
			"blocks":[]
		}
	}`)
	assertValidationContract(t, err, errs.SubtypeInvalidArgument, "--presentation-decision")
	problem, ok := errs.ProblemOf(err)
	if !ok || !strings.Contains(problem.Message, `unknown field "hard_rules"`) {
		t.Fatalf("error = %v, want removed hard_rules field to be rejected", err)
	}
}

func TestDocsScriptPresentationBlockPlanUsesProfileCatalogGenerically(t *testing.T) {
	decision, err := parseDocsScriptPresentationDecision(`{
		"audience":"reviewers",
		"reader_task":"compare both cohorts",
		"genre_contract":"report",
		"adapter":null,
		"presentation_mode":"normal",
		"visual_plan":{
			"reason":"compare exact fields",
			"blocks":[{"type":"table","min_count":2,"purpose":"compare both cohorts"}]
		}
	}`)
	if err != nil {
		t.Fatalf("parseDocsScriptPresentationDecision() error: %v", err)
	}
	warnings := docsScriptPresentationWarnings(docsScriptPublicProfile{Blocks: []docxparse.BlockShare{{Type: "table", Count: 1}}}, decision)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "at least 2 table block") || !strings.Contains(warnings[0], "draft has 1") {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func TestDocsScriptRejectsMarkdownContent(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-reject-markdown"))

	err := mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptParse,
		"--content", "# 标题\n\n- item",
		"--as", "bot",
	}, f, stdout)
	problem, ok := errs.ProblemOf(err)
	var validationErr *errs.ValidationError
	if !ok || problem.Subtype != errs.SubtypeInvalidArgument ||
		!errors.As(err, &validationErr) || validationErr.Param != "--content" {
		t.Fatalf("error = %T %v, problem = %#v, validation = %#v", err, err, problem, validationErr)
	}
}

func TestDocsScriptParsesOnlineDocumentFromToken(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-online-token"))
	registerDocsAIStub(reg, "POST", "/open-apis/docs_ai/v1/documents/doxcnScriptToken/fetch", map[string]interface{}{
		"document": map[string]interface{}{
			"document_id": "doxcnScriptToken",
			"content":     `<title>在线文档</title><p>Hello world</p>`,
		},
	})

	err := mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptParse,
		"--doc", "doxcnScriptToken",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("execute docs +script with token: %v", err)
	}

	var envelope struct {
		Data docsScriptParseResult `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout)
	}
	if envelope.Data.Profile.BlockCount != 2 {
		t.Fatalf("profile = %+v, want 2 blocks", envelope.Data.Profile)
	}
	if got := blockCount(envelope.Data.Profile.Blocks, "title"); got != 1 {
		t.Fatalf("title count = %d, want 1", got)
	}
	if got := blockCount(envelope.Data.Profile.Blocks, "p"); got != 1 {
		t.Fatalf("p count = %d, want 1", got)
	}
}

func TestDocsScriptParsesOnlineDocumentFromURL(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-online-url"))
	stub := registerDocsAIStub(reg, "POST", "/open-apis/docs_ai/v1/documents/wikcnScriptURL/fetch", map[string]interface{}{
		"document": map[string]interface{}{
			"document_id": "doxcnResolvedScriptURL",
			"content":     `<p>从 Wiki URL 读取</p>`,
		},
	})

	err := mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptParse,
		"--doc", "https://example.larksuite.com/wiki/wikcnScriptURL",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("execute docs +script with URL: %v", err)
	}
	if stub.CapturedBody == nil {
		t.Fatal("online parse did not call the document fetch API")
	}

	var envelope struct {
		Data docsScriptParseResult `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout)
	}
	if envelope.Data.Profile.BlockCount != 1 || blockCount(envelope.Data.Profile.Blocks, "p") != 1 {
		t.Fatalf("profile = %+v, want one paragraph", envelope.Data.Profile)
	}
}

func TestDocsScriptRejectsContentAndDocTogether(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-input-conflict"))
	err := mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptParse,
		"--content", `<p>local</p>`,
		"--doc", "doxcnScriptConflict",
		"--as", "bot",
	}, f, nil)
	assertValidationContract(t, err, errs.SubtypeInvalidArgument, "", "--content", "--doc")
}

func TestDocsScriptInitDraftCreatesUniqueWorkspacesWithoutXML(t *testing.T) {
	workDir := t.TempDir()
	withDocsWorkingDir(t, workDir)
	f, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-unique-drafts"))
	decision := `{"audience":"reader","reader_task":"read the draft","genre_contract":"none","adapter":null,"presentation_mode":"normal","visual_plan":{"reason":"plain text is sufficient","blocks":[]}}`

	initialize := func() docsScriptDraftResult {
		t.Helper()
		stdout.Reset()
		err := mountAndRunDocs(t, DocsScript, []string{
			"+script",
			"--command", docsScriptInitDraft,
			"--presentation-decision", decision,
			"--as", "bot",
		}, f, stdout)
		if err != nil {
			t.Fatalf("execute docs +script: %v", err)
		}
		var envelope struct {
			Data docsScriptDraftResult `json:"data"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("decode stdout: %v\n%s", err, stdout)
		}
		return envelope.Data
	}

	first := initialize()
	second := initialize()
	if first.DraftPath == second.DraftPath {
		t.Fatalf("draft paths are identical: %q", first.DraftPath)
	}
	for _, got := range []docsScriptDraftResult{first, second} {
		directory := filepath.Dir(got.DraftPath)
		randomPart := strings.TrimSuffix(strings.TrimPrefix(directory, docsScriptDraftDirectoryPrefix), docsScriptDraftDirectorySuffix)
		if got.Workspace != directory || got.Tip != docsScriptDraftTip || filepath.Base(got.DraftPath) != docsScriptDraftXMLFileName || filepath.Base(directory) != directory ||
			!strings.HasPrefix(directory, docsScriptDraftDirectoryPrefix) || !strings.HasSuffix(directory, docsScriptDraftDirectorySuffix) {
			t.Fatalf("result = %+v, want draft_<random>_folder/draft.xml with an uncreated draft", got)
		}
		if len(randomPart) != docsScriptDraftRandomHexLength {
			t.Fatalf("workspace random suffix = %q, want %d hex characters", randomPart, docsScriptDraftRandomHexLength)
		}
		if _, err := os.Stat(got.DraftPath); !os.IsNotExist(err) {
			t.Fatalf("reserved draft XML path already exists: %q, err=%v", got.DraftPath, err)
		}
		savedDecision, err := os.ReadFile(filepath.Join(directory, docsScriptDecisionFile))
		if err != nil {
			t.Fatalf("read saved decision for %q: %v", got.DraftPath, err)
		}
		if string(savedDecision) != decision {
			t.Fatalf("saved decision = %q, want %q", savedDecision, decision)
		}
	}
}

type docsScriptFailingFileIO struct {
	fileio.FileIO
	failSaveAt int
	saveCalls  int
}

func (f *docsScriptFailingFileIO) Save(path string, opts fileio.SaveOptions, body io.Reader) (fileio.SaveResult, error) {
	f.saveCalls++
	if f.saveCalls == f.failSaveAt {
		return nil, errors.New("injected draft workspace save failure")
	}
	return f.FileIO.Save(path, opts, body)
}

type docsScriptFileIOProvider struct {
	fileIO fileio.FileIO
}

func (p docsScriptFileIOProvider) Name() string { return "docs-script-test" }

func (p docsScriptFileIOProvider) ResolveFileIO(context.Context) fileio.FileIO { return p.fileIO }

type docsScriptErrorWriter struct{}

func (docsScriptErrorWriter) Write([]byte) (int, error) {
	return 0, errors.New("injected output failure")
}

func TestDocsScriptRemovesOnlyCurrentWorkspaceOnFinalFailure(t *testing.T) {
	workDir := t.TempDir()
	withDocsWorkingDir(t, workDir)
	f, _, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-cleanup-failed-workspace"))
	baseFileIO := f.ResolveFileIO(context.Background())
	f.FileIOProvider = docsScriptFileIOProvider{fileIO: &docsScriptFailingFileIO{
		FileIO:     baseFileIO,
		failSaveAt: 1,
	}}
	decision := `{"audience":"reader","reader_task":"read the draft","genre_contract":"none","adapter":null,"presentation_mode":"normal","visual_plan":{"reason":"plain text is sufficient","blocks":[]}}`

	err := mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptInitDraft,
		"--presentation-decision", decision,
	}, f, nil)
	if err == nil {
		t.Fatal("execute docs +script succeeded, want injected save failure")
	}
	entries, readErr := os.ReadDir(workDir)
	if readErr != nil {
		t.Fatalf("read work directory: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("failed initialization left workspace entries: %+v", entries)
	}

	f.IOStreams.Out = docsScriptErrorWriter{}
	err = mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptInitDraft,
		"--presentation-decision", decision,
	}, f, nil)
	if err == nil {
		t.Fatal("execute docs +script succeeded, want injected output failure")
	}
	entries, readErr = os.ReadDir(workDir)
	if readErr != nil {
		t.Fatalf("read work directory after output failure: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("output failure left workspace entries: %+v", entries)
	}

	if err := os.MkdirAll("unrelated_folder", 0o700); err != nil {
		t.Fatalf("create unrelated directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join("unrelated_folder", docsScriptDraftXMLFileName), []byte("keep"), 0o600); err != nil {
		t.Fatalf("create unrelated file: %v", err)
	}
	unrelatedPath := filepath.Join("unrelated_folder", docsScriptDraftXMLFileName)
	unrelatedResolvedPath, err := filepath.Abs(unrelatedPath)
	if err != nil {
		t.Fatalf("resolve unrelated path: %v", err)
	}
	if err := removeDocsScriptWorkspace(unrelatedPath, unrelatedResolvedPath); err == nil {
		t.Fatal("removeDocsScriptWorkspace accepted an unrelated directory")
	}
	if _, err := os.Stat(filepath.Join("unrelated_folder", docsScriptDraftXMLFileName)); err != nil {
		t.Fatalf("unrelated file was removed: %v", err)
	}

	mappedRoot := t.TempDir()
	mappedDirectoryName := docsScriptDraftDirectoryPrefix + strings.Repeat("a", docsScriptDraftRandomHexLength) + docsScriptDraftDirectorySuffix
	mappedResolvedPath := filepath.Join(mappedRoot, mappedDirectoryName, docsScriptDraftXMLFileName)
	if err := os.MkdirAll(filepath.Dir(mappedResolvedPath), 0o700); err != nil {
		t.Fatalf("create mapped workspace: %v", err)
	}
	if err := os.WriteFile(mappedResolvedPath, []byte("draft"), 0o600); err != nil {
		t.Fatalf("create mapped draft: %v", err)
	}
	mappedPath := filepath.Join(mappedDirectoryName, docsScriptDraftXMLFileName)
	if err := removeDocsScriptWorkspace(mappedPath, mappedResolvedPath); err != nil {
		t.Fatalf("remove mapped workspace: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(mappedResolvedPath)); !os.IsNotExist(err) {
		t.Fatalf("mapped workspace still exists or stat failed unexpectedly: %v", err)
	}

	otherDirectoryName := docsScriptDraftDirectoryPrefix + strings.Repeat("b", docsScriptDraftRandomHexLength) + docsScriptDraftDirectorySuffix
	otherResolvedPath := filepath.Join(mappedRoot, otherDirectoryName, docsScriptDraftXMLFileName)
	if err := os.MkdirAll(filepath.Dir(otherResolvedPath), 0o700); err != nil {
		t.Fatalf("create other mapped workspace: %v", err)
	}
	if err := os.WriteFile(otherResolvedPath, []byte("keep"), 0o600); err != nil {
		t.Fatalf("create other mapped draft: %v", err)
	}
	if err := removeDocsScriptWorkspace(mappedPath, otherResolvedPath); err == nil {
		t.Fatal("removeDocsScriptWorkspace accepted a different random workspace")
	}
	if _, err := os.Stat(otherResolvedPath); err != nil {
		t.Fatalf("different random workspace was removed: %v", err)
	}
}

func TestDocsScriptInitDraftRejectsContentAndDoc(t *testing.T) {
	decision := `{"audience":"reader","reader_task":"read the draft","genre_contract":"none","adapter":null,"presentation_mode":"normal","visual_plan":{"reason":"plain text is sufficient","blocks":[]}}`
	tests := []struct {
		name  string
		args  []string
		param string
	}{
		{name: "content", args: []string{"--content", "<p>text</p>"}, param: "--content"},
		{name: "doc", args: []string{"--doc", "doxcnScriptTemp"}, param: "--doc"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f, _, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-init-draft-flags"))
			args := []string{"+script", "--command", docsScriptInitDraft, "--presentation-decision", decision, "--as", "bot"}
			args = append(args, test.args...)
			err := mountAndRunDocs(t, DocsScript, args, f, nil)
			if err == nil {
				t.Fatalf("expected %s validation error", test.param)
			}
			problem, ok := errs.ProblemOf(err)
			var validationErr *errs.ValidationError
			if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument ||
				!errors.As(err, &validationErr) || validationErr.Param != test.param {
				t.Fatalf("problem = %+v, validation = %+v, ok=%v", problem, validationErr, ok)
			}
		})
	}
}

func TestDocsScriptDryRunHasNoAPICall(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-dry-run"))

	err := mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptParse,
		"--content", `<p>text</p>`,
		"--dry-run",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("execute docs +script dry-run: %v", err)
	}
	var got struct {
		API     []any  `json:"api"`
		Command string `json:"command"`
		Network bool   `json:"network"`
	}
	decodeDocsScriptDryRun(t, stdout, &got)
	if len(got.API) != 0 || got.Command != docsScriptParse || got.Network {
		t.Fatalf("dry-run output = %+v", got)
	}
	var envelope struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode dry-run envelope: %v", err)
	}
	if _, exists := envelope.Data["strict"]; exists {
		t.Fatalf("dry-run output still exposes removed strict mode: %s", stdout)
	}
}

func TestDocsScriptRemoteImagePreflightDryRunDeclaresNetwork(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-remote-preflight-dry-run"))
	decision := `{"audience":"reader","reader_task":"read the draft","genre_contract":"none","adapter":null,"presentation_mode":"normal","visual_plan":{"reason":"an image provides visual evidence","blocks":[{"type":"img","min_count":1,"purpose":"show visual evidence"}]}}`

	err := mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptParse,
		"--content", `<title>Draft</title><img href="https://93.184.216.34/image.png"/>`,
		"--presentation-decision", decision,
		"--dry-run",
	}, f, stdout)
	if err != nil {
		t.Fatalf("execute docs +script dry-run: %v", err)
	}
	var got struct {
		Command string `json:"command"`
		Network bool   `json:"network"`
	}
	decodeDocsScriptDryRun(t, stdout, &got)
	if got.Command != docsScriptParse || !got.Network {
		t.Fatalf("dry-run output = %+v, want network=true for remote image preflight", got)
	}
}

func TestDocsScriptDryRunRejectsInvalidSavedPresentationDecision(t *testing.T) {
	workDir := t.TempDir()
	withDocsWorkingDir(t, workDir)
	workspace := filepath.Join("draft_workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatalf("create draft workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "draft.xml"), []byte(`<p>draft</p>`), 0o600); err != nil {
		t.Fatalf("write draft XML: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, docsScriptDecisionFile), []byte(`{"invalid":true}`), 0o600); err != nil {
		t.Fatalf("write invalid saved Presentation Decision: %v", err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-invalid-sidecar-dry-run"))
	err := mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptParse,
		"--content", "@" + filepath.Join(workspace, "draft.xml"),
		"--dry-run",
	}, f, stdout)
	if err == nil {
		t.Fatal("execute docs +script dry-run succeeded with an invalid saved Presentation Decision")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Subtype != errs.SubtypeUnknown {
		t.Fatalf("error = %T %v, problem = %#v; want typed invalid saved decision error", err, err, problem)
	}
}

func TestDocsScriptInitDraftDryRunDoesNotWrite(t *testing.T) {
	workDir := t.TempDir()
	withDocsWorkingDir(t, workDir)
	f, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-init-draft-dry-run"))

	err := mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptInitDraft,
		"--presentation-decision", `{"audience":"reader","reader_task":"understand key states","genre_contract":"none","adapter":null,"presentation_mode":"rich","visual_plan":{"reason":"visual explanation","blocks":[{"type":"img","min_count":2,"purpose":"show key states"}]}}`,
		"--dry-run",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("execute init-draft dry-run: %v", err)
	}
	var got struct {
		API                  []any  `json:"api"`
		Command              string `json:"command"`
		PresentationDecision bool   `json:"presentation_decision"`
		CreatesWorkspace     bool   `json:"creates_workspace"`
		CreatesDraftFile     bool   `json:"creates_draft_file"`
		DirectoryPattern     string `json:"directory_pattern"`
		XMLFileName          string `json:"xml_file_name"`
		Network              bool   `json:"network"`
	}
	decodeDocsScriptDryRun(t, stdout, &got)
	if len(got.API) != 0 || got.Command != docsScriptInitDraft || !got.PresentationDecision ||
		!got.CreatesWorkspace || got.CreatesDraftFile || got.DirectoryPattern != docsScriptDraftDirectoryPattern ||
		got.XMLFileName != docsScriptDraftXMLFileName || got.Network {
		t.Fatalf("dry-run output = %+v", got)
	}
	entries, err := os.ReadDir(workDir)
	if err != nil {
		t.Fatalf("read work directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("dry-run created files: %+v", entries)
	}
}

func TestDocsScriptOnlineDryRunShowsFetchAPICall(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-online-dry-run"))

	err := mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptParse,
		"--doc", "https://example.larksuite.com/docx/doxcnScriptDryRun",
		"--dry-run",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("execute online docs +script dry-run: %v", err)
	}
	var got struct {
		API []struct {
			Method string                 `json:"method"`
			URL    string                 `json:"url"`
			Body   map[string]interface{} `json:"body"`
		} `json:"api"`
		Command    string `json:"command"`
		DocumentID string `json:"document_id"`
		Network    bool   `json:"network"`
	}
	decodeDocsScriptDryRun(t, stdout, &got)
	if len(got.API) != 1 || got.API[0].Method != "POST" ||
		got.API[0].URL != "/open-apis/docs_ai/v1/documents/doxcnScriptDryRun/fetch" {
		t.Fatalf("dry-run API = %+v", got.API)
	}
	if got.API[0].Body["format"] != "xml" {
		t.Fatalf("dry-run body = %+v, want XML fetch", got.API[0].Body)
	}
	if got.Command != docsScriptParse || got.DocumentID != "doxcnScriptDryRun" || !got.Network {
		t.Fatalf("dry-run output = %+v", got)
	}
}

func decodeDocsScriptDryRun(t *testing.T, stdout *bytes.Buffer, dst interface{}) {
	t.Helper()
	var envelope struct {
		OK     bool            `json:"ok"`
		DryRun bool            `json:"dry_run"`
		Data   json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode dry-run envelope: %v\n%s", err, stdout)
	}
	if !envelope.OK || !envelope.DryRun {
		t.Fatalf("unexpected dry-run envelope: %s", stdout)
	}
	if err := json.Unmarshal(envelope.Data, dst); err != nil {
		t.Fatalf("decode dry-run data: %v\n%s", err, stdout)
	}
}

func TestDocsScriptReturnsTypedParseError(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-error"))

	err := mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptParse,
		"--content", `<!DOCTYPE document><p>text</p>`,
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected parse error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("problem = %+v, ok=%v", problem, ok)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Param != "--content" {
		t.Fatalf("error = %#v, want --content metadata", err)
	}
}

func TestDocsScriptProfilesCompatibleMalformedXML(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-malformed"))

	err := mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptParse,
		"--content", `<title>T</title><ul><li>one<li>two</ul><p>tail</p`,
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("execute compatible parse: %v", err)
	}

	var envelope struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout)
	}
	if len(envelope.Data) != 1 || envelope.Data["profile"] == nil {
		t.Fatalf("data = %+v, want only profile", envelope.Data)
	}
	var profile docsScriptPublicProfile
	if err := json.Unmarshal(envelope.Data["profile"], &profile); err != nil {
		t.Fatalf("decode profile: %v", err)
	}
	if profile.BlockCount != 5 || blockCount(profile.Blocks, "li") != 2 || blockCount(profile.Blocks, "p") != 1 {
		t.Fatalf("profile = %+v, want title, ul, two li, and p", profile)
	}
}

func TestDocsScriptAcceptsLocalImagePath(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-local-image"))

	err := mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptParse,
		"--content", `<title>Local image</title><img path="@diagram.png" caption="diagram"/>`,
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("execute parse: %v", err)
	}

	var envelope struct {
		Data struct {
			Profile docsScriptPublicProfile `json:"profile"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout)
	}
	if envelope.Data.Profile.BlockCount != 2 || blockCount(envelope.Data.Profile.Blocks, "img") != 1 {
		t.Fatalf("profile = %+v, want one title and one img block", envelope.Data.Profile)
	}
}

func TestDocsScriptHelpExamplesAreCrossShellSafe(t *testing.T) {
	cmd := &cobra.Command{Short: "local document parser"}
	installDocsScriptHelp(cmd)
	if strings.Contains(cmd.Example, "cat ") {
		t.Fatalf("help examples require a platform-specific command: %q", cmd.Example)
	}
	if strings.Contains(cmd.Example, "--content @") {
		t.Fatalf("help examples contain an unquoted @file argument: %q", cmd.Example)
	}
	for _, want := range []string{`--command init-draft --presentation-decision`, `--content "@./draft.xml"`, `--content "@./draft.md"`} {
		if !strings.Contains(cmd.Example, want) {
			t.Errorf("help examples missing %q: %q", want, cmd.Example)
		}
	}
}

func blockCount(blocks []docxparse.BlockShare, typ string) int {
	for _, block := range blocks {
		if block.Type == typ {
			return block.Count
		}
	}
	return 0
}

func containsDocsScriptWarning(warnings []string, substring string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, substring) {
			return true
		}
	}
	return false
}

func requireDocsScriptWarningPartialFailure(t *testing.T, err error) {
	t.Helper()
	var partialFailure *output.PartialFailureError
	if !errors.As(err, &partialFailure) {
		t.Fatalf("error = %T %v, want warning partial failure", err, err)
	}
	if partialFailure.Code != output.ExitAPI {
		t.Fatalf("partial failure exit code = %d, want %d", partialFailure.Code, output.ExitAPI)
	}
}
