// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/doc/internal/docxparse"
)

const (
	docsScriptParse                 = "parse"
	docsScriptInitDraft             = "init-draft"
	docsScriptDraftDirectoryPattern = "draft_*_folder"
	docsScriptDraftDirectoryPrefix  = "draft_"
	docsScriptDraftDirectorySuffix  = "_folder"
	docsScriptDraftXMLFileName      = "draft.xml"
	docsScriptDraftRandomHexLength  = 8
	docsScriptDecisionFile          = ".presentation-decision.json"
	docsScriptDraftTip              = "The workspace directory has been created successfully. draft_path points to a new XML file that does not exist yet. Create and write the file directly without reading it first."
	docsScriptListBlockType         = "list"
)

var DocsScript = common.Shortcut{
	Service:     "docs",
	Command:     "+script",
	Description: "Initialize a document draft workspace, or parse, preflight, and profile documents",
	Risk:        "read",
	AuthTypes:   []string{"user", "bot"},
	Scopes:      []string{},
	ConditionalScopes: []string{
		"docx:document:readonly",
	},
	Flags: []common.Flag{
		{
			Name:     "command",
			Desc:     "local document operation",
			Required: true,
			Enum:     []string{docsScriptInitDraft, docsScriptParse},
		},
		{
			Name:  "content",
			Desc:  "local XML or Markdown content for parse; use @relative-file or - for stdin; mutually exclusive with --doc",
			Input: []string{common.File, common.Stdin},
		},
		{
			Name: "doc",
			Desc: "online document URL or token for --command parse; mutually exclusive with --content",
		},
		{
			Name:  "presentation-decision",
			Desc:  "Presentation Decision JSON required by init-draft and saved as the draft profile baseline; genre_contract and adapter accept a short name, \"none\", or null; accepts inline JSON (recommended for init-draft), @relative-file, or - for stdin",
			Input: []string{common.File, common.Stdin},
		},
	},
	Tips: []string{
		"parse preflights draft resources when a Presentation Decision is loaded, repairs common malformed XML in memory, and returns the text and block profile",
	},
	Local:     docsScriptRunsLocally,
	PostMount: installDocsScriptHelp,
	Validate:  validateDocsScript,
	DryRun:    dryRunDocsScript,
	Execute:   executeDocsScript,
}

func docsScriptRunsLocally(cmd *cobra.Command) bool {
	doc, _ := cmd.Flags().GetString("doc")
	return strings.TrimSpace(doc) == ""
}

type docsScriptParseResult struct {
	Profile docsScriptPublicProfile `json:"profile"`
	Warning []string                `json:"warning,omitempty"`
}

// docsScriptPublicProfile is the stable shortcut response. The parser keeps
// the more detailed breakdown internally so it can be exposed later without
// changing the counting implementation.
type docsScriptPublicProfile struct {
	WordCount  int                    `json:"word_count"`
	CharCount  int                    `json:"char_count"`
	BlockCount int                    `json:"block_count"`
	Blocks     []docxparse.BlockShare `json:"blocks"`
}

type docsScriptPresentationDecision struct {
	Audience         string                           `json:"audience"`
	ReaderTask       string                           `json:"reader_task"`
	GenreContract    *string                          `json:"genre_contract"`
	Adapter          *string                          `json:"adapter"`
	PresentationMode string                           `json:"presentation_mode"`
	WordCount        *docsScriptPresentationWordCount `json:"word_count"`
	VisualPlan       docsScriptPresentationVisualPlan `json:"visual_plan"`
}

type docsScriptPresentationWordCount struct {
	Min *int `json:"min"`
	Max *int `json:"max"`
}

type docsScriptPresentationVisualPlan struct {
	Reason string                                   `json:"reason"`
	Blocks []docsScriptPresentationBlockRequirement `json:"blocks"`
}

type docsScriptPresentationBlockRequirement struct {
	Type     string `json:"type"`
	MinCount int    `json:"min_count"`
	Purpose  string `json:"purpose"`
}

type docsScriptDraftResult struct {
	Workspace string `json:"workspace"`
	DraftPath string `json:"draft_path"`
	Tip       string `json:"tip"`
}

type docsScriptFetchRequest struct {
	Format       string                      `json:"format"`
	ExtraParam   string                      `json:"extra_param"`
	ExportOption docsScriptFetchExportOption `json:"export_option"`
	Lang         string                      `json:"lang,omitempty"`
}

type docsScriptFetchExportOption struct {
	ExportBlockID       bool `json:"export_block_id"`
	ExportStyleAttrs    bool `json:"export_style_attrs"`
	ExportCiteExtraData bool `json:"export_cite_extra_data"`
}

type docsScriptFetchResponse struct {
	Document *struct {
		Content *string `json:"content"`
	} `json:"document"`
}

func installDocsScriptHelp(cmd *cobra.Command) {
	installDocsContentPathCapture(cmd)
	cmd.Example = `  lark-cli docs +script --command init-draft --presentation-decision '<JSON>'
  lark-cli docs +script --command parse --content "@./draft.xml"
  lark-cli docs +script --command parse --content "@./draft.md"
  lark-cli docs +script --command parse --doc "https://example.larksuite.com/docx/doxcn..."`
}

func validateDocsScript(_ context.Context, runtime *common.RuntimeContext) error {
	command := runtime.Str("command")
	content := strings.TrimSpace(runtime.Str("content"))
	doc := strings.TrimSpace(runtime.Str("doc"))
	presentationDecision := strings.TrimSpace(runtime.Str("presentation-decision"))
	if presentationDecision != "" {
		if command != docsScriptParse && command != docsScriptInitDraft {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--presentation-decision is only supported with --command init-draft or parse").WithParam("--presentation-decision")
		}
		if _, err := parseDocsScriptPresentationDecision(presentationDecision); err != nil {
			return err
		}
	}
	if command == docsScriptInitDraft {
		switch {
		case content != "":
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--content is not supported with --command %s", command).WithParam("--content")
		case doc != "":
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--doc is not supported with --command %s", command).WithParam("--doc")
		case presentationDecision == "":
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--presentation-decision is required with --command init-draft").WithParam("--presentation-decision")
		}
		return nil
	}
	if content == "" && doc == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "one of --content or --doc is required").WithParams(
			errs.InvalidParam{Name: "--content", Reason: "provide local document content"},
			errs.InvalidParam{Name: "--doc", Reason: "provide an online document URL or token"},
		)
	}
	if content != "" && doc != "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--content and --doc are mutually exclusive").WithParams(
			errs.InvalidParam{Name: "--content", Reason: "mutually exclusive with --doc"},
			errs.InvalidParam{Name: "--doc", Reason: "mutually exclusive with --content"},
		)
	}
	if doc != "" {
		if command != docsScriptParse {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--doc is only supported with --command parse").WithParam("--doc")
		}
		if _, err := parseDocumentRef(doc); err != nil {
			return err
		}
		if err := runtime.EnsureScopes([]string{"docx:document:readonly"}); err != nil {
			return err
		}
	}
	if command == docsScriptParse {
		if _, _, err := resolveDocsScriptPresentationDecision(runtime); err != nil {
			return err
		}
	}
	return nil
}

func dryRunDocsScript(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	if runtime.Str("command") == docsScriptInitDraft {
		dry := common.NewDryRunAPI().
			Desc("Create a unique draft workspace, preserve its Presentation Decision, and reserve a new XML path without creating the XML; no API call is made").
			Set("command", docsScriptInitDraft).
			Set("directory_pattern", docsScriptDraftDirectoryPattern).
			Set("xml_file_name", docsScriptDraftXMLFileName).
			Set("creates_workspace", true).
			Set("creates_draft_file", false).
			Set("presentation_decision", true).
			Set("network", false)
		return dry
	}
	if doc := strings.TrimSpace(runtime.Str("doc")); doc != "" {
		ref, _ := parseDocumentRef(doc)
		dry := common.NewDryRunAPI().
			POST("/open-apis/docs_ai/v1/documents/:document_id/fetch").
			Desc("OpenAPI: fetch document for parsing and profiling").
			Body(docsScriptFetchBody(runtime)).
			Set("command", runtime.Str("command")).
			Set("document_id", ref.Token).
			Set("network", true)
		if strings.TrimSpace(runtime.Str("presentation-decision")) != "" {
			dry.Set("presentation_decision", true)
		}
		return dry
	}
	dry := common.NewDryRunAPI().
		Desc("Local LarkOpenCLI document parsing; no OpenAPI call is made").
		Set("command", runtime.Str("command")).
		Set("input_bytes", len(runtime.Str("content")))
	hasDecision := docsScriptHasPresentationDecision(runtime)
	network := hasDecision && docsScriptHasRemoteImagePreflight(runtime, runtime.Str("content"))
	dry.Set("network", network)
	if network {
		dry.Desc("Local LarkOpenCLI document parsing; remote image availability preflight makes ranged network requests but does not buffer image bodies")
	}
	if hasDecision {
		dry.Set("presentation_decision", true)
	}
	return dry
}

func executeDocsScript(_ context.Context, runtime *common.RuntimeContext) error {
	command := runtime.Str("command")
	content := runtime.Str("content")
	switch command {
	case docsScriptInitDraft:
		return initDocsScriptDraft(runtime)
	case docsScriptParse:
		inputParam := "--content"
		inputLabel := "--content"
		if strings.TrimSpace(runtime.Str("doc")) != "" {
			var err error
			content, err = fetchDocsScriptContent(runtime)
			if err != nil {
				return err
			}
			inputParam = "--doc"
			inputLabel = "fetched document content"
		}
		profile, err := docxparse.ParseAuto(content)
		if err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"could not parse %s as LarkOpenCLI XML or Markdown: %s", inputLabel, err).
				WithParam(inputParam).
				WithCause(err)
		}
		publicProfile := docsScriptPublicProfile{
			WordCount:  profile.WordCount,
			CharCount:  profile.CharCount,
			BlockCount: profile.BlockCount,
			Blocks:     profile.Blocks,
		}
		decision, hasDecision, err := resolveDocsScriptPresentationDecision(runtime)
		if err != nil {
			return err
		}
		var warnings []string
		if hasDecision {
			warnings = docsScriptPresentationWarnings(publicProfile, decision)
			warnings = append(warnings, docsScriptResourceWarnings(runtime, content)...)
		}
		result := docsScriptParseResult{
			Profile: publicProfile,
			Warning: warnings,
		}
		if len(warnings) > 0 {
			return runtime.OutPartialFailureRaw(result, nil)
		}
		runtime.OutFormatRaw(result, nil, nil)
		return nil
	default:
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"unsupported --command %q", command).
			WithParam("--command")
	}
}

func docsScriptResourceWarnings(runtime *common.RuntimeContext, content string) []string {
	input, err := prepareDocsV2WriteInputForFormat(runtime, string(docxparse.DetectFormat(content)), docsV2WriteInput{Content: content})
	if err != nil {
		return []string{docsScriptResourceWarning(err)}
	}

	warnings := make([]string, 0)
	for _, resource := range input.LocalResources {
		if resource.RemoteURL == "" {
			continue
		}
		if err := probeRemoteDocImageDownload(runtime, resource.RemoteURL, resource.Occurrence); err != nil {
			warnings = append(warnings, docsScriptResourceWarning(err))
		}
	}
	return warnings
}

func docsScriptHasPresentationDecision(runtime *common.RuntimeContext) bool {
	_, hasDecision, err := resolveDocsScriptPresentationDecision(runtime)
	return err == nil && hasDecision
}

func docsScriptHasRemoteImagePreflight(runtime *common.RuntimeContext, content string) bool {
	input, err := prepareDocsV2WriteInputForFormat(runtime, string(docxparse.DetectFormat(content)), docsV2WriteInput{Content: content})
	if err != nil {
		return false
	}
	for _, resource := range input.LocalResources {
		if resource.RemoteURL != "" {
			return true
		}
	}
	return false
}

func docsScriptResourceWarning(err error) string {
	message := "resource preflight failed: " + err.Error()
	if problem, ok := errs.ProblemOf(err); ok && strings.TrimSpace(problem.Hint) != "" {
		message += "; hint: " + problem.Hint
	}
	return message
}

func resolveDocsScriptPresentationDecision(runtime *common.RuntimeContext) (docsScriptPresentationDecision, bool, error) {
	if rawDecision := strings.TrimSpace(runtime.Str("presentation-decision")); rawDecision != "" {
		decision, err := parseDocsScriptPresentationDecision(rawDecision)
		return decision, err == nil, err
	}
	contentPath, ok := runtime.Cmd.Annotations[docsContentPathAnnotation]
	if !ok {
		return docsScriptPresentationDecision{}, false, nil
	}
	rawDecision, err := cmdutil.ReadInputFile(runtime.FileIO(), filepath.Join(filepath.Dir(contentPath), docsScriptDecisionFile))
	if errors.Is(err, fs.ErrNotExist) {
		return docsScriptPresentationDecision{}, false, nil
	}
	if err != nil {
		return docsScriptPresentationDecision{}, false, errs.NewInternalError(errs.SubtypeFileIO,
			"read saved Presentation Decision: %s", err).
			WithCause(err)
	}
	decision, err := parseDocsScriptPresentationDecision(string(rawDecision))
	if err != nil {
		return docsScriptPresentationDecision{}, false, errs.NewInternalError(errs.SubtypeUnknown,
			"saved Presentation Decision is invalid").
			WithCause(err)
	}
	return decision, true, nil
}

func parseDocsScriptPresentationDecision(raw string) (docsScriptPresentationDecision, error) {
	var decision docsScriptPresentationDecision
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decision); err != nil {
		return docsScriptPresentationDecision{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--presentation-decision must be a valid Presentation Decision JSON object: %s", err).
			WithParam("--presentation-decision").
			WithCause(err)
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return docsScriptPresentationDecision{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--presentation-decision must contain exactly one JSON object; multiple JSON values were provided").
				WithParam("--presentation-decision")
		}
		return docsScriptPresentationDecision{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--presentation-decision must contain exactly one JSON object: %s", err).
			WithParam("--presentation-decision").
			WithCause(err)
	}
	var rawFields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &rawFields); err != nil {
		return docsScriptPresentationDecision{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--presentation-decision must be a valid Presentation Decision JSON object: %s", err).
			WithParam("--presentation-decision").
			WithCause(err)
	}
	for _, field := range []string{
		"audience", "reader_task", "genre_contract", "adapter", "presentation_mode",
		"visual_plan",
	} {
		if _, ok := rawFields[field]; !ok {
			return docsScriptPresentationDecision{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--presentation-decision %s is required", field).
				WithParam("--presentation-decision")
		}
	}
	if rawWordCountValue, hasWordCount := rawFields["word_count"]; hasWordCount {
		if strings.TrimSpace(string(rawWordCountValue)) == "null" {
			return docsScriptPresentationDecision{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--presentation-decision word_count must be omitted when no word-count requirement was requested").
				WithParam("--presentation-decision").
				WithHint("remove the word_count field; include it only when the user requested a word-count bound")
		}
		if decision.WordCount == nil {
			return docsScriptPresentationDecision{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--presentation-decision word_count must be an object containing min and max").
				WithParam("--presentation-decision")
		}
		var rawWordCountFields map[string]json.RawMessage
		if err := json.Unmarshal(rawWordCountValue, &rawWordCountFields); err != nil {
			return docsScriptPresentationDecision{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--presentation-decision word_count must be an object containing min and max").
				WithParam("--presentation-decision").
				WithCause(err)
		}
		for _, field := range []string{"min", "max"} {
			if _, ok := rawWordCountFields[field]; !ok {
				return docsScriptPresentationDecision{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
					"--presentation-decision word_count.%s is required; use null when that bound is unspecified", field).
					WithParam("--presentation-decision")
			}
		}
		switch {
		case decision.WordCount.Min == nil && decision.WordCount.Max == nil:
			return docsScriptPresentationDecision{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--presentation-decision word_count must set at least one of min or max; omit word_count when no word count was requested").
				WithParam("--presentation-decision")
		case decision.WordCount.Min != nil && *decision.WordCount.Min <= 0:
			return docsScriptPresentationDecision{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--presentation-decision word_count.min must be a positive integer when provided").
				WithParam("--presentation-decision")
		case decision.WordCount.Max != nil && *decision.WordCount.Max <= 0:
			return docsScriptPresentationDecision{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--presentation-decision word_count.max must be a positive integer when provided").
				WithParam("--presentation-decision")
		case decision.WordCount.Min != nil && decision.WordCount.Max != nil && *decision.WordCount.Min > *decision.WordCount.Max:
			return docsScriptPresentationDecision{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--presentation-decision word_count.min must not exceed word_count.max").
				WithParam("--presentation-decision")
		}
	}
	decision.Audience = strings.TrimSpace(decision.Audience)
	decision.ReaderTask = strings.TrimSpace(decision.ReaderTask)
	var err error
	decision.GenreContract, err = normalizeDocsScriptOptionalRoute("genre_contract", decision.GenreContract)
	if err != nil {
		return docsScriptPresentationDecision{}, err
	}
	decision.Adapter, err = normalizeDocsScriptOptionalRoute("adapter", decision.Adapter)
	if err != nil {
		return docsScriptPresentationDecision{}, err
	}
	decision.PresentationMode = strings.TrimSpace(decision.PresentationMode)
	decision.VisualPlan.Reason = strings.TrimSpace(decision.VisualPlan.Reason)
	requiredStrings := []struct {
		field string
		value string
	}{
		{"audience", decision.Audience},
		{"reader_task", decision.ReaderTask},
		{"presentation_mode", decision.PresentationMode},
		{"visual_plan.reason", decision.VisualPlan.Reason},
	}
	for _, required := range requiredStrings {
		if required.value == "" {
			return docsScriptPresentationDecision{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--presentation-decision %s must not be empty", required.field).
				WithParam("--presentation-decision")
		}
	}
	var rawVisualPlan map[string]json.RawMessage
	if err := json.Unmarshal(rawFields["visual_plan"], &rawVisualPlan); err != nil {
		return docsScriptPresentationDecision{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--presentation-decision visual_plan must be an object containing reason and blocks").
			WithParam("--presentation-decision").
			WithCause(err)
	}
	for _, field := range []string{"reason", "blocks"} {
		if _, ok := rawVisualPlan[field]; !ok {
			return docsScriptPresentationDecision{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--presentation-decision visual_plan.%s is required", field).
				WithParam("--presentation-decision")
		}
	}
	if decision.VisualPlan.Blocks == nil {
		return docsScriptPresentationDecision{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--presentation-decision visual_plan.blocks must be an array; use [] when no presentation blocks are planned").
			WithParam("--presentation-decision")
	}
	switch decision.PresentationMode {
	case "formal", "normal", "rich":
	default:
		return docsScriptPresentationDecision{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--presentation-decision presentation_mode must be formal, normal, or rich").
			WithParam("--presentation-decision")
	}
	seenBlockTypes := make(map[string]struct{}, len(decision.VisualPlan.Blocks))
	for i := range decision.VisualPlan.Blocks {
		requirement := &decision.VisualPlan.Blocks[i]
		requirement.Type = strings.TrimSpace(requirement.Type)
		requirement.Purpose = strings.TrimSpace(requirement.Purpose)
		if requirement.Type != docsScriptListBlockType && !docxparse.IsPresentationBlockType(requirement.Type) {
			return docsScriptPresentationDecision{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--presentation-decision visual_plan.blocks[%d].type %q is not a presentation block type", i, requirement.Type).
				WithParam("--presentation-decision")
		}
		if _, exists := seenBlockTypes[requirement.Type]; exists {
			return docsScriptPresentationDecision{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--presentation-decision visual_plan.blocks contains duplicate type %q; combine it into one minimum", requirement.Type).
				WithParam("--presentation-decision")
		}
		seenBlockTypes[requirement.Type] = struct{}{}
		if requirement.MinCount <= 0 {
			return docsScriptPresentationDecision{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--presentation-decision visual_plan.blocks[%d].min_count must be positive", i).
				WithParam("--presentation-decision")
		}
		if requirement.Purpose == "" {
			return docsScriptPresentationDecision{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--presentation-decision visual_plan.blocks[%d].purpose must not be empty", i).
				WithParam("--presentation-decision")
		}
	}
	return decision, nil
}

func normalizeDocsScriptOptionalRoute(field string, value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	normalized := strings.TrimSpace(*value)
	if normalized == "" {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--presentation-decision %s must be a non-empty short name, \"none\", or null", field).
			WithParam("--presentation-decision")
	}
	return &normalized, nil
}

func docsScriptPresentationWarnings(profile docsScriptPublicProfile, decision docsScriptPresentationDecision) []string {
	var warnings []string
	if decision.WordCount != nil {
		belowMinimum := decision.WordCount.Min != nil && profile.WordCount < *decision.WordCount.Min
		aboveMaximum := decision.WordCount.Max != nil && profile.WordCount > *decision.WordCount.Max
		if belowMinimum || aboveMaximum {
			expectation := ""
			switch {
			case decision.WordCount.Min != nil && decision.WordCount.Max != nil:
				expectation = fmt.Sprintf("range %d-%d", *decision.WordCount.Min, *decision.WordCount.Max)
			case decision.WordCount.Min != nil:
				expectation = fmt.Sprintf("at least %d", *decision.WordCount.Min)
			default:
				expectation = fmt.Sprintf("at most %d", *decision.WordCount.Max)
			}
			warnings = append(warnings, fmt.Sprintf(
				"word_count is %d; Presentation Decision expects %s",
				profile.WordCount, expectation,
			))
		}
	}
	for _, required := range decision.VisualPlan.Blocks {
		actual := docsScriptBlockCount(profile.Blocks, required.Type)
		if actual < required.MinCount {
			warnings = append(warnings, fmt.Sprintf(
				"visual_plan requires at least %d %s block(s) for %s, but the draft has %d",
				required.MinCount, required.Type, required.Purpose, actual,
			))
		}
	}
	return warnings
}

func docsScriptBlockCount(blocks []docxparse.BlockShare, blockType string) int {
	if blockType == docsScriptListBlockType {
		return docsScriptBlockCount(blocks, "ul") + docsScriptBlockCount(blocks, "ol")
	}
	for _, block := range blocks {
		if block.Type == blockType {
			return block.Count
		}
	}
	return 0
}

func initDocsScriptDraft(runtime *common.RuntimeContext) error {
	path, err := newDocsScriptWorkspacePath()
	if err != nil {
		return errs.NewInternalError(errs.SubtypeUnknown, "generate unique draft workspace path").WithCause(err)
	}
	resolvedPath, err := runtime.ResolveSavePath(path)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeFileIO,
			"resolve reserved draft XML path %s: %s", path, err).
			WithCause(err)
	}
	rawDecision := strings.TrimSpace(runtime.Str("presentation-decision"))
	decisionPath := filepath.Join(filepath.Dir(path), docsScriptDecisionFile)
	if _, err := runtime.FileIO().Save(decisionPath, fileio.SaveOptions{
		ContentType:   "application/json",
		ContentLength: int64(len(rawDecision)),
	}, strings.NewReader(rawDecision)); err != nil {
		return failDocsScriptWorkspace(path, resolvedPath, common.WrapSaveErrorTyped(err))
	}
	runtime.Out(docsScriptDraftResult{
		Workspace: filepath.Dir(path),
		DraftPath: path,
		Tip:       docsScriptDraftTip,
	}, nil)
	if err := runtime.OutputError(); err != nil {
		return failDocsScriptWorkspace(path, resolvedPath, err)
	}
	return nil
}

func failDocsScriptWorkspace(path, resolvedPath string, cause error) error {
	if cleanupErr := removeDocsScriptWorkspace(path, resolvedPath); cleanupErr != nil {
		return errors.Join(cause, cleanupErr)
	}
	return cause
}

func removeDocsScriptWorkspace(path, resolvedPath string) error {
	cleanPath := filepath.Clean(path)
	if !isDocsScriptWorkspacePath(cleanPath, true) {
		return errs.NewInternalError(errs.SubtypeFileIO,
			"refusing to remove unexpected draft workspace path %s", filepath.Dir(cleanPath))
	}
	cleanResolvedPath := filepath.Clean(resolvedPath)
	if !filepath.IsAbs(cleanResolvedPath) ||
		filepath.Base(filepath.Dir(cleanResolvedPath)) != filepath.Base(filepath.Dir(cleanPath)) ||
		!isDocsScriptWorkspacePath(cleanResolvedPath, false) {
		return errs.NewInternalError(errs.SubtypeFileIO,
			"refusing to remove unexpected resolved draft workspace path %s", filepath.Dir(cleanResolvedPath))
	}
	resolvedDirectory := filepath.Dir(cleanResolvedPath)
	if err := os.RemoveAll(resolvedDirectory); err != nil { //nolint:forbidigo // FileIO has no remove operation; resolvedDirectory is strictly validated above.
		return errs.NewInternalError(errs.SubtypeFileIO,
			"remove failed draft workspace %s: %s", resolvedDirectory, err).
			WithCause(err)
	}
	return nil
}

func isDocsScriptWorkspacePath(path string, requireRelativeParent bool) bool {
	directory := filepath.Dir(path)
	directoryName := filepath.Base(directory)
	randomPart := strings.TrimSuffix(strings.TrimPrefix(directoryName, docsScriptDraftDirectoryPrefix), docsScriptDraftDirectorySuffix)
	_, randomErr := hex.DecodeString(randomPart)
	return (!requireRelativeParent || filepath.Dir(directory) == ".") &&
		filepath.Base(path) == docsScriptDraftXMLFileName &&
		strings.HasPrefix(directoryName, docsScriptDraftDirectoryPrefix) && strings.HasSuffix(directoryName, docsScriptDraftDirectorySuffix) &&
		len(randomPart) == docsScriptDraftRandomHexLength && randomErr == nil
}

func newDocsScriptWorkspacePath() (string, error) {
	random := make([]byte, docsScriptDraftRandomHexLength/2)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	directory := docsScriptDraftDirectoryPrefix + hex.EncodeToString(random) + docsScriptDraftDirectorySuffix
	return filepath.Join(directory, docsScriptDraftXMLFileName), nil
}

func docsScriptFetchBody(runtime *common.RuntimeContext) docsScriptFetchRequest {
	body := docsScriptFetchRequest{
		Format:     "xml",
		ExtraParam: docsFetchExtraParam,
		ExportOption: docsScriptFetchExportOption{
			ExportBlockID:       false,
			ExportStyleAttrs:    false,
			ExportCiteExtraData: false,
		},
	}
	if lang := resolveFetchLang(runtime); lang != "" {
		body.Lang = lang
	}
	return body
}

func fetchDocsScriptContent(runtime *common.RuntimeContext) (string, error) {
	ref, _ := parseDocumentRef(runtime.Str("doc"))
	apiPath := fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s/fetch", ref.Token)
	data, err := runtime.CallAPITyped("POST", apiPath, nil, docsScriptFetchBody(runtime))
	if err != nil {
		return "", err
	}
	response, err := projectDocsScriptFetchResponse(data)
	if err != nil {
		return "", err
	}
	if response.Document == nil {
		return "", errs.NewInternalError(errs.SubtypeInvalidResponse,
			"document fetch response for --doc is missing document")
	}
	if response.Document.Content == nil {
		return "", errs.NewInternalError(errs.SubtypeInvalidResponse,
			"document fetch response for --doc is missing document.content")
	}
	return *response.Document.Content, nil
}

func projectDocsScriptFetchResponse(data map[string]interface{}) (docsScriptFetchResponse, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return docsScriptFetchResponse{}, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"encode document fetch response: %v", err).WithCause(err)
	}
	var response docsScriptFetchResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return docsScriptFetchResponse{}, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"decode document fetch response: %v", err).WithCause(err)
	}
	return response, nil
}
