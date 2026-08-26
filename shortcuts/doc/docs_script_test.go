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
		for _, want := range []string{
			"genre_contract and adapter",
			`"none"`,
			"or null",
			"unambiguous schema fields",
		} {
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
	if len(envelope.Data) != 2 || envelope.Data["profile"] == nil || envelope.Data["assessment"] == nil {
		t.Fatalf("data = %+v, want assessment and profile", envelope.Data)
	}
	var assessment docsScriptAssessment
	if err := json.Unmarshal(envelope.Data["assessment"], &assessment); err != nil {
		t.Fatalf("decode assessment: %v", err)
	}
	if assessment.Status != docsScriptAssessmentPassed {
		t.Fatalf("assessment = %+v for valid XML: %s", assessment, stdout)
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

func TestDocsScriptReturnsPresentationDecisionDiagnostics(t *testing.T) {
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
	if err != nil {
		t.Fatalf("execute docs +script: %v", err)
	}

	var envelope struct {
		OK   bool                  `json:"ok"`
		Data docsScriptParseResult `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout)
	}
	if !envelope.OK {
		t.Fatalf("diagnostic result reported ok:false: %s", stdout)
	}
	if envelope.Data.Assessment.Status != docsScriptAssessmentFailed {
		t.Fatalf("assessment = %+v with diagnostics, want failed: %s", envelope.Data.Assessment, stdout)
	}
	if len(envelope.Data.Diagnostics) != 3 {
		t.Fatalf("diagnostics = %#v, want word-count, whiteboard, and html diagnostics", envelope.Data.Diagnostics)
	}
	wordCount := envelope.Data.Diagnostics[0]
	if wordCount.Severity != docsScriptDiagnosticError || wordCount.Code != docsScriptCodeWordCountRange ||
		wordCount.Expected == nil || wordCount.Expected.Min == nil || *wordCount.Expected.Min != 18 ||
		wordCount.Expected.Max == nil || *wordCount.Expected.Max != 22 || wordCount.Actual == nil || *wordCount.Actual != 10 {
		t.Fatalf("word-count diagnostic = %#v", wordCount)
	}
	for index, wantType := range []string{"whiteboard", "html5-block"} {
		diagnostic := envelope.Data.Diagnostics[index+1]
		if diagnostic.Code != docsScriptCodeRequiredBlock || diagnostic.Expected == nil ||
			diagnostic.Expected.Type != wantType || diagnostic.Expected.MinCount != 1 ||
			diagnostic.Actual == nil || *diagnostic.Actual != 0 {
			t.Errorf("%s diagnostic = %#v", wantType, diagnostic)
		}
	}
	for _, diagnostic := range envelope.Data.Diagnostics {
		if diagnostic.Expected != nil && diagnostic.Expected.Type == "img" {
			t.Fatalf("diagnostics = %#v, img requirement is satisfied", envelope.Data.Diagnostics)
		}
	}
}

func TestDocsScriptPresentationDecisionPreflightsBlockedRemoteImage(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-resource-preflight"))
	decision := `{"audience":"reader","reader_task":"read the draft","genre_contract":"none","adapter":null,"presentation_mode":"normal","visual_plan":{"reason":"an image provides visual evidence","blocks":[{"type":"img","min_count":1,"purpose":"show visual evidence"}]}}`

	err := mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptParse,
		"--content", `<title>Draft</title><img href="http://127.0.0.1/one.png"/><img href="http://127.0.0.1/two.png"/>`,
		"--presentation-decision", decision,
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
	if envelope.Data.Assessment.Status != docsScriptAssessmentFailed {
		t.Fatalf("assessment = %+v, want failed", envelope.Data.Assessment)
	}
	if len(envelope.Data.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one deduplicated image diagnostic", envelope.Data.Diagnostics)
	}
	diagnostic := envelope.Data.Diagnostics[0]
	if diagnostic.Code != docsScriptCodeImageSource || diagnostic.Severity != docsScriptDiagnosticError ||
		len(diagnostic.ImageIndices) != 2 || diagnostic.ImageIndices[0] != 1 || diagnostic.ImageIndices[1] != 2 ||
		diagnostic.Msg != "local/internal host is not allowed" || !strings.Contains(diagnostic.Suggested, "Download the affected images") {
		t.Fatalf("image diagnostic = %#v", diagnostic)
	}
	if strings.Contains(diagnostic.Msg, "remote image #") || strings.Contains(stdout.String(), "resource preflight failed") {
		t.Fatalf("image diagnostic repeats legacy prefixes: %s", stdout)
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
	if err != nil {
		t.Fatalf("execute docs +script: %v", err)
	}

	var envelope struct {
		Data docsScriptParseResult `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout)
	}
	if requestMethod != http.MethodGet || requestRange != "bytes=0-0" {
		t.Fatalf("remote image probe method = %q range = %q, want ranged GET", requestMethod, requestRange)
	}
	if envelope.Data.Assessment.Status != docsScriptAssessmentFailed {
		t.Fatalf("assessment = %+v, want failed", envelope.Data.Assessment)
	}
	if len(envelope.Data.Diagnostics) != 1 || envelope.Data.Diagnostics[0].Code != docsScriptCodeImageUnavailable ||
		len(envelope.Data.Diagnostics[0].ImageIndices) != 1 || envelope.Data.Diagnostics[0].ImageIndices[0] != 1 ||
		envelope.Data.Diagnostics[0].Msg != "HTTP 404" {
		t.Fatalf("diagnostics = %#v, want image #1 HTTP 404", envelope.Data.Diagnostics)
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
	if envelope.Data.Assessment.Status != docsScriptAssessmentPassed || len(envelope.Data.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want passed with no resource preflight without a Presentation Decision", envelope.Data)
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
	if err != nil {
		t.Fatalf("execute docs +script: %v", err)
	}
	var parsed struct {
		Data docsScriptParseResult `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		t.Fatalf("decode parse output: %v\n%s", err, stdout)
	}
	if parsed.Data.Assessment.Status != docsScriptAssessmentFailed || len(parsed.Data.Diagnostics) != 3 {
		t.Fatalf("result = %#v, want failed assessment with persisted word-count, whiteboard, and html diagnostics", parsed.Data)
	}
}

func TestDocsScriptInitDraftNormalizesWindowsCommandShimQuotes(t *testing.T) {
	workDir := t.TempDir()
	withDocsWorkingDir(t, workDir)
	f, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-init-draft-shell-quotes"))
	decision := `{"audience":"普通读者","reader_task":"复现实验","genre_contract":null,"adapter":null,"presentation_mode":"normal","visual_plan":{"reason":"复现实验","blocks":[]}}`

	err := mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptInitDraft,
		"--presentation-decision", "'" + decision + "'",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("initialize draft with Windows command-shim quotes: %v", err)
	}

	var initialized struct {
		Data docsScriptDraftResult `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &initialized); err != nil {
		t.Fatalf("decode init output: %v\n%s", err, stdout)
	}
	savedDecision, err := os.ReadFile(filepath.Join(initialized.Data.Workspace, docsScriptDecisionFile))
	if err != nil {
		t.Fatalf("read saved decision: %v", err)
	}
	if got := string(savedDecision); got != decision {
		t.Fatalf("saved decision = %q, want normalized JSON %q", got, decision)
	}
}

func TestDocsScriptInitDraftRecoversPowerShellDequotedDecisionFromSchema(t *testing.T) {
	tests := []struct {
		name           string
		dequoted       string
		wantNormalized string
	}{
		{
			name:           "keys and values",
			dequoted:       `{audience:a,reader_task:b,genre_contract:null,adapter:null,presentation_mode:normal,word_count:{min:10,max:null},visual_plan:{reason:c,blocks:[{type:img,min_count:1,purpose:d}]}}`,
			wantNormalized: `{"audience":"a","reader_task":"b","genre_contract":null,"adapter":null,"presentation_mode":"normal","word_count":{"min":10,"max":null},"visual_plan":{"reason":"c","blocks":[{"type":"img","min_count":1,"purpose":"d"}]}}`,
		},
		{
			name:           "values only",
			dequoted:       `{"audience":a,"reader_task":b,"genre_contract":null,"adapter":null,"presentation_mode":normal,"visual_plan":{"reason":c,"blocks":[]}}`,
			wantNormalized: `{"audience":"a","reader_task":"b","genre_contract":null,"adapter":null,"presentation_mode":"normal","visual_plan":{"reason":"c","blocks":[]}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workDir := t.TempDir()
			withDocsWorkingDir(t, workDir)
			f, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-init-draft-dequoted-json"))

			err := mountAndRunDocs(t, DocsScript, []string{
				"+script",
				"--command", docsScriptInitDraft,
				"--presentation-decision", test.dequoted,
				"--as", "bot",
			}, f, stdout)
			if err != nil {
				t.Fatalf("initialize draft with PowerShell-dequoted JSON: %v", err)
			}

			var initialized struct {
				Data docsScriptDraftResult `json:"data"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &initialized); err != nil {
				t.Fatalf("decode init output: %v\n%s", err, stdout)
			}
			savedDecision, err := os.ReadFile(filepath.Join(initialized.Data.Workspace, docsScriptDecisionFile))
			if err != nil {
				t.Fatalf("read saved decision: %v", err)
			}
			if got := string(savedDecision); got != test.wantNormalized {
				t.Fatalf("saved decision = %q, want schema-normalized JSON %q", got, test.wantNormalized)
			}
		})
	}
}

func TestDocsScriptPowerShellDequotedRecoveryUsesOriginalValidation(t *testing.T) {
	workDir := t.TempDir()
	withDocsWorkingDir(t, workDir)
	f, _, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-dequoted-original-validation"))
	dequoted := `{audience:a,reader_task:b,genre_contract:null,adapter:null,presentation_mode:decorative,visual_plan:{reason:c,blocks:[]}}`

	err := mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptInitDraft,
		"--presentation-decision", dequoted,
		"--as", "bot",
	}, f, nil)
	assertValidationContract(t, err, errs.SubtypeInvalidArgument, "--presentation-decision")
	if !strings.Contains(err.Error(), "presentation_mode must be formal, normal, or rich") {
		t.Fatalf("error = %v, want original Presentation Decision validation", err)
	}
}

func TestDocsScriptPresentationDecisionQuoteRecoveryUsesOriginalSchema(t *testing.T) {
	workDir := t.TempDir()
	withDocsWorkingDir(t, workDir)
	f, _, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-presentation-quote-schema"))
	decision := `{"audience":"reader","reader_task":"understand","genre_contract":null,"adapter":null,"presentation_mode":"normal","visual_plan":{"reason":"plain text is sufficient","blocks":[]},"unexpected":true}`

	err := mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptInitDraft,
		"--presentation-decision", "'" + decision + "'",
		"--as", "bot",
	}, f, nil)
	assertValidationContract(t, err, errs.SubtypeInvalidArgument, "--presentation-decision")
	if !strings.Contains(err.Error(), `json: unknown field "unexpected"`) {
		t.Fatalf("error = %v, want recovered JSON to use the original strict schema", err)
	}
	entries, readErr := os.ReadDir(workDir)
	if readErr != nil {
		t.Fatalf("read work directory: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("failed quote recovery created files: %#v", entries)
	}
}

func TestDocsScriptPresentationDecisionFileRemainsStrictJSON(t *testing.T) {
	workDir := t.TempDir()
	withDocsWorkingDir(t, workDir)
	if err := os.WriteFile("decision.json", []byte(`{audience:reader,reader_task:understand}`), 0o600); err != nil {
		t.Fatalf("write decision: %v", err)
	}
	f, _, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-decision-file-strict-json"))

	err := mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptInitDraft,
		"--presentation-decision", "@./decision.json",
		"--as", "bot",
	}, f, nil)
	assertValidationContract(t, err, errs.SubtypeInvalidArgument, "--presentation-decision")
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error does not expose a typed problem: %v", err)
	}
	if problem.Hint != "" {
		t.Fatalf("hint = %q, want no shell-mangling guidance for strict @file JSON", problem.Hint)
	}
}

func TestDocsScriptPresentationDecisionFileAcceptsUTF8BOM(t *testing.T) {
	workDir := t.TempDir()
	withDocsWorkingDir(t, workDir)
	decision := `{"audience":"普通读者","reader_task":"复现实验","genre_contract":null,"adapter":null,"presentation_mode":"normal","visual_plan":{"reason":"复现实验","blocks":[]}}`
	if err := os.WriteFile("decision.json", []byte("\uFEFF"+decision), 0o600); err != nil {
		t.Fatalf("write decision: %v", err)
	}
	f, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-decision-file-bom"))

	err := mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptInitDraft,
		"--presentation-decision", "@./decision.json",
		"--dry-run",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("dry-run init with BOM-prefixed decision file: %v", err)
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
			diagnostics := docsScriptPresentationDiagnostics(docsScriptPublicProfile{WordCount: test.wordCount}, decision)
			if got := len(diagnostics) > 0; got != test.wantWarn {
				t.Fatalf("diagnostics = %#v, want diagnostic=%v", diagnostics, test.wantWarn)
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
			want:      "Increase word_count to at least 10.",
		},
		{
			name:      "maximum",
			wordCount: 21,
			decision:  `{"audience":"reader","reader_task":"understand","genre_contract":"knowledge","adapter":"wechat","presentation_mode":"normal","word_count":{"min":null,"max":20},"visual_plan":{"reason":"plain text is sufficient","blocks":[]}}`,
			want:      "Reduce word_count to at most 20.",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision, err := parseDocsScriptPresentationDecision(test.decision)
			if err != nil {
				t.Fatalf("parse decision: %v", err)
			}
			diagnostics := docsScriptPresentationDiagnostics(docsScriptPublicProfile{WordCount: test.wordCount}, decision)
			if len(diagnostics) != 1 || diagnostics[0].Code != docsScriptCodeWordCountRange || diagnostics[0].Suggested != test.want {
				t.Fatalf("diagnostics = %#v, want one with suggestion %q", diagnostics, test.want)
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
	if diagnostics := docsScriptPresentationDiagnostics(profile, decision); len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want ul + ol to satisfy two list blocks", diagnostics)
	}

	decision.VisualPlan.Blocks[0].MinCount = 3
	diagnostics := docsScriptPresentationDiagnostics(profile, decision)
	if len(diagnostics) != 1 || diagnostics[0].Code != docsScriptCodeRequiredBlock ||
		diagnostics[0].Expected == nil || diagnostics[0].Expected.Type != docsScriptListBlockType ||
		diagnostics[0].Expected.MinCount != 3 || diagnostics[0].Actual == nil || *diagnostics[0].Actual != 2 {
		t.Fatalf("diagnostics = %#v, want list min_count 3 and actual 2", diagnostics)
	}
}

func TestDocsScriptOmitsDiagnosticsWhenPresentationDecisionPasses(t *testing.T) {
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
	if envelope.Data["diagnostics"] != nil {
		t.Fatalf("passing decision should omit data.diagnostics: %s", stdout)
	}
	var assessment docsScriptAssessment
	if err := json.Unmarshal(envelope.Data["assessment"], &assessment); err != nil || assessment.Status != docsScriptAssessmentPassed {
		t.Fatalf("passing decision data.assessment = %s, decode error = %v", envelope.Data["assessment"], err)
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

func TestDocsScriptPresentationDecisionMangledInlineJSONSuggestsFileInput(t *testing.T) {
	workDir := t.TempDir()
	withDocsWorkingDir(t, workDir)
	f, _, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-presentation-shell-mangled"))
	err := mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptInitDraft,
		"--presentation-decision", `{audience:reader,reviewer,reader_task:understand}`,
		"--as", "bot",
	}, f, nil)
	assertValidationContract(t, err, errs.SubtypeInvalidArgument, "--presentation-decision")
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error does not expose a typed problem: %v", err)
	}
	if got := problem.Hint; got != docsScriptDecisionShellHint {
		t.Fatalf("hint = %q, want %q", got, docsScriptDecisionShellHint)
	}
	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Fatalf("error = %T (%v), want preserved *json.SyntaxError cause", err, err)
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
	diagnostics := docsScriptPresentationDiagnostics(docsScriptPublicProfile{Blocks: []docxparse.BlockShare{{Type: "table", Count: 1}}}, decision)
	if len(diagnostics) != 1 || diagnostics[0].Code != docsScriptCodeRequiredBlock ||
		diagnostics[0].Expected == nil || diagnostics[0].Expected.Type != "table" || diagnostics[0].Expected.MinCount != 2 ||
		diagnostics[0].Actual == nil || *diagnostics[0].Actual != 1 {
		t.Fatalf("diagnostics = %#v", diagnostics)
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
	fileio.WorkspaceFileIO
	failSaveAt  int
	saveCalls   int
	removeCalls []string
}

func (f *docsScriptFailingFileIO) Save(path string, opts fileio.SaveOptions, body io.Reader) (fileio.SaveResult, error) {
	f.saveCalls++
	if f.saveCalls == f.failSaveAt {
		return nil, errors.New("injected draft workspace save failure")
	}
	return f.WorkspaceFileIO.Save(path, opts, body)
}

func (f *docsScriptFailingFileIO) RemoveWorkspaceEntry(path string) error {
	f.removeCalls = append(f.removeCalls, path)
	return f.WorkspaceFileIO.RemoveWorkspaceEntry(path)
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
	baseFileIO, ok := f.ResolveFileIO(context.Background()).(fileio.WorkspaceFileIO)
	if !ok {
		t.Fatalf("default FileIO does not implement fileio.WorkspaceFileIO")
	}
	failingFileIO := &docsScriptFailingFileIO{
		WorkspaceFileIO: baseFileIO,
		failSaveAt:      1,
	}
	f.FileIOProvider = docsScriptFileIOProvider{fileIO: failingFileIO}
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
	removeCallsBefore := len(failingFileIO.removeCalls)
	if err := (docsScriptWorkspace{
		path:   unrelatedPath,
		fileIO: failingFileIO,
	}).remove(); err == nil {
		t.Fatal("workspace cleanup accepted an unrelated directory")
	}
	if len(failingFileIO.removeCalls) != removeCallsBefore {
		t.Fatalf("unsafe workspace reached FileIO removal: %v", failingFileIO.removeCalls[removeCallsBefore:])
	}
	if _, err := os.Stat(filepath.Join("unrelated_folder", docsScriptDraftXMLFileName)); err != nil {
		t.Fatalf("unrelated file was removed: %v", err)
	}
	if len(failingFileIO.removeCalls) != 4 {
		t.Fatalf("workspace FileIO removal calls = %v, want two entries for each failed initialization", failingFileIO.removeCalls)
	}
	for i := 0; i < len(failingFileIO.removeCalls); i += 2 {
		decisionPath := failingFileIO.removeCalls[i]
		directory := failingFileIO.removeCalls[i+1]
		if filepath.Base(decisionPath) != docsScriptDecisionFile || filepath.Dir(decisionPath) != directory {
			t.Fatalf("workspace cleanup pair = %q, %q", decisionPath, directory)
		}
		if !isDocsScriptWorkspacePath(filepath.Join(directory, docsScriptDraftXMLFileName)) {
			t.Fatalf("workspace cleanup used unexpected directory %q", directory)
		}
	}
}

type docsScriptBasicFileIO struct {
	fileio.FileIO
}

func TestDocsScriptInitDraftRequiresWorkspaceFileIO(t *testing.T) {
	workDir := t.TempDir()
	withDocsWorkingDir(t, workDir)
	f, _, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-script-workspace-fileio"))
	f.FileIOProvider = docsScriptFileIOProvider{fileIO: docsScriptBasicFileIO{
		FileIO: f.ResolveFileIO(context.Background()),
	}}
	decision := `{"audience":"reader","reader_task":"read the draft","genre_contract":"none","adapter":null,"presentation_mode":"normal","visual_plan":{"reason":"plain text is sufficient","blocks":[]}}`

	err := mountAndRunDocs(t, DocsScript, []string{
		"+script",
		"--command", docsScriptInitDraft,
		"--presentation-decision", decision,
	}, f, nil)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("problem = %+v, ok=%v, want validation/failed_precondition", problem, ok)
	}
	entries, readErr := os.ReadDir(workDir)
	if readErr != nil {
		t.Fatalf("read work directory: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("unsupported FileIO created workspace entries: %+v", entries)
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
	if len(envelope.Data) != 2 || envelope.Data["profile"] == nil || envelope.Data["assessment"] == nil {
		t.Fatalf("data = %+v, want assessment and profile", envelope.Data)
	}
	var assessment docsScriptAssessment
	if err := json.Unmarshal(envelope.Data["assessment"], &assessment); err != nil || assessment.Status != docsScriptAssessmentPassed {
		t.Fatalf("compatible XML data.assessment = %s, decode error = %v", envelope.Data["assessment"], err)
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
	for _, want := range []string{`--command init-draft --presentation-decision`, `--content "@./draft.xml"`} {
		if !strings.Contains(cmd.Example, want) {
			t.Errorf("help examples missing %q: %q", want, cmd.Example)
		}
	}
	if strings.Contains(cmd.Example, `@./draft.md`) {
		t.Fatalf("help examples contain unsupported Markdown input: %q", cmd.Example)
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
