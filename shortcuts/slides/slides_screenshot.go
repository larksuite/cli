// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/util"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	defaultSlidesScreenshotDir = ".lark-slides/screenshots"
	maxSlidesPerScreenshot     = 10
)

var (
	unsafeScreenshotFileCharRegex = regexp.MustCompile(`[^A-Za-z0-9._-]+`)
	slideNumberAliasRegex         = regexp.MustCompile(`^[0-9]+$`)
)

// SlidesScreenshot fetches server-rendered slide screenshots and writes them to
// local files. The raw API returns Base64 image payloads; this shortcut keeps
// those payloads out of stdout so agents only see small file metadata.
var SlidesScreenshot = common.Shortcut{
	Service:     "slides",
	Command:     "+screenshot",
	Description: "Save up to 10 slide screenshots to local files without printing Base64 image data",
	Risk:        "read",
	Scopes:      []string{"slides:presentation:screenshot"},
	// wiki:node:read is required only when --presentation is a wiki URL.
	ConditionalScopes: []string{"wiki:node:read"},
	AuthTypes:         []string{"user", "bot"},
	Flags: []common.Flag{
		listModePresentationRefFlag(),
		{Name: "slide-id", Aliases: []string{"slide-ids", "slides"}, Type: "string_slice", Desc: "slide page identifier (repeat or comma-separated for multiple slides; max 10 pages per request)"},
		{Name: "slide-number", Aliases: []string{"slide-numbers"}, Type: "int_array", Desc: "slide page number (repeat for multiple slides; max 10 pages per request)"},
		{Name: "slide", Desc: "hidden alias routed to --slide-number for digits, otherwise --slide-id", Hidden: true},
		{Name: "content", Desc: "slide XML content to render directly instead of fetching existing slides", Input: []string{common.File, common.Stdin}},
		{Name: "output", Desc: "preferred relative output path for a single screenshot (extension optional; .png, .jpg, or .jpeg)"},
		{Name: "overwrite", Type: "bool", Desc: "overwrite an existing file selected by --output"},
		{Name: "output-dir", Default: defaultSlidesScreenshotDir, Desc: "relative directory for saved screenshots"},
		{Name: "output-name", Desc: "legacy file name stem for --content render output; prefer --output for new calls"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		renderMode := runtime.Changed("content")
		selectorCount := 1
		if runtime.Changed("overwrite") && !runtime.Changed("output") {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--overwrite requires --output").
				WithParam("--overwrite").
				WithHint("use --overwrite only with --output <file>")
		}
		if renderMode {
			if strings.TrimSpace(runtime.Str("content")) == "" {
				return slidesScreenshotFlagErrorf("--content cannot be empty")
			}
			if slidesScreenshotHasSelectorInput(runtime) {
				return slidesScreenshotContentSelectorConflictError(runtime)
			}
			if runtime.Changed("presentation") {
				return slidesScreenshotFlagErrorf("--presentation cannot be used with --content")
			}
		} else {
			ref, err := parsePresentationRef(slidesScreenshotPresentation(runtime))
			if err != nil {
				return err
			}
			slideIDs, slideNumbers, err := slidesScreenshotSelectors(runtime)
			if err != nil {
				return err
			}
			if ref.Kind == "wiki" {
				if err := runtime.EnsureScopes([]string{"wiki:node:read"}); err != nil {
					return err
				}
			}
			if len(slideIDs) == 0 && len(slideNumbers) == 0 {
				return slidesScreenshotMissingSelectorError()
			}
			selectorCount = len(slideIDs) + len(slideNumbers)
			if err := validateSlidesScreenshotSelectorLimit(selectorCount); err != nil {
				return err
			}
		}
		if runtime.Changed("output") {
			if runtime.Changed("output-dir") {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--output cannot be combined with --output-dir").WithParam("--output")
			}
			if runtime.Changed("output-name") {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--output cannot be combined with --output-name").WithParam("--output")
			}
			if selectorCount != 1 {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--output requires exactly one slide; use --output-dir for multiple screenshots").WithParam("--output")
			}
			if err := validateScreenshotOutputPath(runtime, runtime.Str("output")); err != nil {
				return err
			}
		} else {
			if !renderMode && runtime.Changed("output-name") {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--output-name is only supported with --content").
					WithParam("--output-name").
					WithHint("use --output <file> for one existing slide, or --output-dir for multiple slides")
			}
			if _, err := validateScreenshotOutputDir(runtime, runtime.Str("output-dir")); err != nil {
				return err
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		if runtime.Changed("content") {
			return dryRunRenderScreenshot(runtime)
		}
		ref, err := parsePresentationRef(slidesScreenshotPresentation(runtime))
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		slideIDs, slideNumbers, err := slidesScreenshotSelectors(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		if err := validateSlidesScreenshotSelectorLimit(len(slideIDs) + len(slideNumbers)); err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}

		presentationID := ref.Token
		dry := common.NewDryRunAPI()
		if ref.Kind == "wiki" {
			presentationID = "<resolved_slides_token>"
			dry.Desc("2-step orchestration: resolve wiki → fetch slide screenshot(s)").
				GET("/open-apis/wiki/v2/spaces/get_node").
				Desc("[1] Resolve wiki node to slides presentation").
				Params(map[string]interface{}{"token": ref.Token})
		} else {
			if outputPath := strings.TrimSpace(runtime.Str("output")); outputPath != "" {
				dry.Desc(fmt.Sprintf("Fetch one slide screenshot and save it as %s", outputPath))
			} else {
				dry.Desc(fmt.Sprintf("Fetch %d slide screenshot(s) and save files under %s", len(slideIDs)+len(slideNumbers), runtime.Str("output-dir")))
			}
		}
		body := map[string]interface{}{}
		if len(slideIDs) > 0 {
			body["slide_ids"] = slideIDs
		}
		if len(slideNumbers) > 0 {
			body["slide_numbers"] = slideNumbers
		}
		dry.POST(fmt.Sprintf(
			"/open-apis/slides_ai/v1/xml_presentations/%s/slide_images",
			validate.EncodePathSegment(presentationID),
		)).
			Body(body)
		return setSlidesScreenshotDryRunOutput(dry, runtime).Set("base64_output", "suppressed; decoded to local files during execution")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if runtime.Changed("content") {
			return executeRenderScreenshot(runtime)
		}
		ref, err := parsePresentationRef(slidesScreenshotPresentation(runtime))
		if err != nil {
			return err
		}
		presentationID, err := resolvePresentationID(runtime, ref)
		if err != nil {
			return err
		}

		slideIDs, slideNumbers, err := slidesScreenshotSelectors(runtime)
		if err != nil {
			return err
		}
		if len(slideIDs) == 0 && len(slideNumbers) == 0 {
			return slidesScreenshotMissingSelectorError()
		}
		if err := validateSlidesScreenshotSelectorLimit(len(slideIDs) + len(slideNumbers)); err != nil {
			return err
		}
		outputTarget, err := resolveSlidesScreenshotOutputTarget(runtime)
		if err != nil {
			return err
		}

		url := fmt.Sprintf(
			"/open-apis/slides_ai/v1/xml_presentations/%s/slide_images",
			validate.EncodePathSegment(presentationID),
		)
		query := larkcore.QueryParams{}
		body := map[string]interface{}{}
		if len(slideIDs) > 0 {
			body["slide_ids"] = slideIDs
		}
		if len(slideNumbers) > 0 {
			body["slide_numbers"] = slideNumbers
		}
		data, err := doSlidesScreenshotAPIJSONWithLogID(runtime, "POST", url, query, body)
		if err != nil {
			return enrichSlidesScreenshotSelectorError(err, slideNumbers)
		}

		saved, err := saveSlideScreenshots(runtime, data, outputTarget.safeOutputDir, presentationID, outputTarget.requested, outputTarget.overwrite)
		if err != nil {
			return err
		}
		result := map[string]interface{}{
			"xml_presentation_id": presentationID,
			"screenshots":         saved,
		}
		setSlidesScreenshotResultOutput(result, outputTarget, saved)
		runtime.Out(result, nil)
		return nil
	},
}

func dryRunRenderScreenshot(runtime *common.RuntimeContext) *common.DryRunAPI {
	content := runtime.Str("content")
	if strings.TrimSpace(content) == "" {
		return common.NewDryRunAPI().Set("error", "--content cannot be empty")
	}
	if slidesScreenshotHasSelectorInput(runtime) {
		return common.NewDryRunAPI().Set("error", "--content cannot be used with slide selectors")
	}
	if runtime.Changed("presentation") {
		return common.NewDryRunAPI().Set("error", "--presentation cannot be used with --content")
	}
	dry := common.NewDryRunAPI().Desc("Render slide XML content to a screenshot file")
	dry.POST("/open-apis/slides_ai/v1/slide_image/render").
		Body(map[string]interface{}{
			"content": fmt.Sprintf("<xml omitted; length=%d>", len(content)),
		})
	return setSlidesScreenshotDryRunOutput(dry, runtime).Set("base64_output", "suppressed; decoded to local file during execution")
}

func executeRenderScreenshot(runtime *common.RuntimeContext) error {
	content := runtime.Str("content")
	if strings.TrimSpace(content) == "" {
		return slidesScreenshotFlagErrorf("--content cannot be empty")
	}
	if slidesScreenshotHasSelectorInput(runtime) {
		return slidesScreenshotContentSelectorConflictError(runtime)
	}
	if runtime.Changed("presentation") {
		return slidesScreenshotFlagErrorf("--presentation cannot be used with --content")
	}
	outputTarget, err := resolveSlidesScreenshotOutputTarget(runtime)
	if err != nil {
		return err
	}

	data, err := doSlidesScreenshotAPIJSONWithLogID(runtime, "POST", "/open-apis/slides_ai/v1/slide_image/render", larkcore.QueryParams{}, map[string]interface{}{
		"content": content,
	})
	if err != nil {
		return err
	}
	saved, err := saveRenderedSlideScreenshot(runtime, data, outputTarget.safeOutputDir, runtime.Str("output-name"), outputTarget.requested, outputTarget.overwrite)
	if err != nil {
		return err
	}
	result := map[string]interface{}{
		"screenshots": saved,
	}
	setSlidesScreenshotResultOutput(result, outputTarget, saved)
	runtime.Out(result, nil)
	return nil
}

func setSlidesScreenshotDryRunOutput(dry *common.DryRunAPI, runtime *common.RuntimeContext) *common.DryRunAPI {
	if outputPath := strings.TrimSpace(runtime.Str("output")); outputPath != "" {
		return dry.Set("output", outputPath)
	}
	return dry.Set("output_dir", runtime.Str("output-dir"))
}

type slidesScreenshotOutputTarget struct {
	requested         string
	requestedResolved string
	outputDir         string
	safeOutputDir     string
	overwrite         bool
}

func resolveSlidesScreenshotOutputTarget(runtime *common.RuntimeContext) (slidesScreenshotOutputTarget, error) {
	target := slidesScreenshotOutputTarget{
		requested: strings.TrimSpace(runtime.Str("output")),
		outputDir: runtime.Str("output-dir"),
		overwrite: runtime.Bool("overwrite"),
	}
	if target.requested != "" {
		resolved, err := runtime.ResolveSavePath(target.requested)
		if err != nil {
			return target, errs.NewValidationError(errs.SubtypeInvalidArgument, "--output invalid: %v", err).
				WithParam("--output").
				WithCause(err)
		}
		target.requestedResolved = resolved
		return target, nil
	}
	safeOutputDir, err := ensureScreenshotOutputDir(runtime, target.outputDir)
	if err != nil {
		return target, err
	}
	target.safeOutputDir = safeOutputDir
	return target, nil
}

func setSlidesScreenshotResultOutput(result map[string]interface{}, target slidesScreenshotOutputTarget, saved []map[string]interface{}) {
	if target.requested != "" && len(saved) == 1 {
		actualPath, _ := saved[0]["path"].(string)
		result["output"] = actualPath
		if filepath.Clean(target.requestedResolved) != filepath.Clean(actualPath) {
			result["requested_output"] = target.requested
			result["output_adjusted"] = true
		}
		return
	}
	result["output_dir"] = target.outputDir
}

func slidesScreenshotPresentation(runtime *common.RuntimeContext) string {
	return runtime.Str("presentation")
}

func slidesScreenshotSelectors(runtime *common.RuntimeContext) ([]string, []int, error) {
	aliasSlide := strings.TrimSpace(runtime.Str("slide"))
	aliasSlideIsNumber := runtime.Changed("slide") && slideNumberAliasRegex.MatchString(aliasSlide)
	aliasSlideIsID := runtime.Changed("slide") && aliasSlide != "" && !aliasSlideIsNumber
	if runtime.Changed("slide") && aliasSlide == "" {
		return nil, nil, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"--slide cannot be empty",
		).WithParam("--slide").WithHint("use --slide-number or --slide-id to specify the selector type explicitly")
	}

	slideIDValues := append([]string(nil), runtime.StrSlice("slide-id")...)
	if runtime.Changed("slide-id") && len(normalizeSlideIDs(slideIDValues)) == 0 {
		return nil, nil, slidesScreenshotEmptySlideIDError()
	}
	if aliasSlideIsID {
		slideIDValues = append(slideIDValues, aliasSlide)
	}

	slideNumberValues := append([]int(nil), runtime.IntArray("slide-number")...)
	if aliasSlideIsNumber {
		n, err := strconv.Atoi(aliasSlide)
		if err != nil {
			return nil, nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--slide page number is outside the supported integer range").WithParam("--slide")
		}
		if n < 1 {
			return nil, nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--slide must be a positive page number").WithParam("--slide")
		}
		slideNumberValues = append(slideNumberValues, n)
	}

	slideNumbers, err := normalizeSlideNumbers(slideNumberValues)
	if err != nil {
		return nil, nil, err
	}
	slideIDs := normalizeSlideIDs(slideIDValues)
	if len(slideIDs) > 0 && len(slideNumbers) > 0 {
		return nil, nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "slide ID selectors and slide number selectors cannot be used together").
			WithParams(slidesScreenshotSelectorConflictParams(runtime, aliasSlideIsID, aliasSlideIsNumber)...).
			WithHint("choose either slide IDs or slide numbers for one screenshot request")
	}
	return slideIDs, slideNumbers, nil
}

func slidesScreenshotHasSelectorInput(runtime *common.RuntimeContext) bool {
	return len(slidesScreenshotSelectorInputParams(runtime, "")) > 0
}

func slidesScreenshotSelectorConflictParams(runtime *common.RuntimeContext, aliasSlideIsID, aliasSlideIsNumber bool) []errs.InvalidParam {
	params := make([]errs.InvalidParam, 0, 3)
	if len(normalizeSlideIDs(runtime.StrSlice("slide-id"))) > 0 {
		params = append(params, errs.InvalidParam{Name: "--slide-id", Reason: "selects by slide ID; cannot be combined with slide-number selectors"})
	}
	if aliasSlideIsID {
		params = append(params, errs.InvalidParam{Name: "--slide", Reason: "selects by slide ID; cannot be combined with slide-number selectors"})
	}
	if runtime.Changed("slide-number") {
		params = append(params, errs.InvalidParam{Name: "--slide-number", Reason: "selects by slide number; cannot be combined with slide-ID selectors"})
	}
	if aliasSlideIsNumber {
		params = append(params, errs.InvalidParam{Name: "--slide", Reason: "selects by slide number; cannot be combined with slide-ID selectors"})
	}
	return params
}

func slidesScreenshotSelectorInputParams(runtime *common.RuntimeContext, reason string) []errs.InvalidParam {
	params := make([]errs.InvalidParam, 0, 3)
	if len(normalizeSlideIDs(runtime.StrSlice("slide-id"))) > 0 {
		params = append(params, errs.InvalidParam{Name: "--slide-id", Reason: reason})
	}
	if runtime.Changed("slide-number") {
		params = append(params, errs.InvalidParam{Name: "--slide-number", Reason: reason})
	}
	if runtime.Changed("slide") {
		params = append(params, errs.InvalidParam{Name: "--slide", Reason: reason})
	}
	return params
}

func slidesScreenshotContentSelectorConflictError(runtime *common.RuntimeContext) error {
	params := []errs.InvalidParam{{Name: "--content", Reason: "cannot be combined with slide selectors"}}
	params = append(params, slidesScreenshotSelectorInputParams(runtime, "cannot be combined with --content")...)
	return errs.NewValidationError(errs.SubtypeInvalidArgument, "--content cannot be used with slide selectors").WithParams(params...)
}

func normalizeSlideIDs(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, v := range values {
		s := strings.TrimSpace(v)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func normalizeSlideNumbers(values []int) ([]int, error) {
	out := make([]int, 0, len(values))
	seen := map[int]struct{}{}
	for _, n := range values {
		if n < 1 {
			return nil, slidesScreenshotFlagErrorf("--slide-number must be a positive integer")
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out, nil
}

func validateSlidesScreenshotSelectorLimit(count int) error {
	if count > maxSlidesPerScreenshot {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "too many slide selectors: got %d, maximum is %d", count, maxSlidesPerScreenshot).
			WithHint("request at most 10 pages at a time")
	}
	return nil
}

func slidesScreenshotMissingSelectorError() error {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, "--slide-id or --slide-number is required").
		WithHint("specify up to 10 slides with --slide-id <slide_id> or --slide-number <number>; repeat the flag or use comma-separated values for multiple slides")
}

func slidesScreenshotEmptySlideIDError() error {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, "--slide-id cannot be empty").
		WithParam("--slide-id").
		WithHint("provide a non-empty slide ID or use --slide-number <number>")
}

func slidesScreenshotFlagErrorf(format string, args ...interface{}) error {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, format, args...)
}

func validateScreenshotOutputDir(runtime *common.RuntimeContext, outputDir string) (string, error) {
	if _, err := runtime.ResolveSavePath(filepath.Join(outputDir, "probe.png")); err != nil {
		return "", slidesScreenshotFlagErrorf("--output-dir invalid: %v", err)
	}
	return outputDir, nil
}

func validateScreenshotOutputPath(runtime *common.RuntimeContext, outputPath string) error {
	outputPath = strings.TrimSpace(outputPath)
	if outputPath == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--output cannot be empty").WithParam("--output")
	}
	if os.IsPathSeparator(outputPath[len(outputPath)-1]) {
		return screenshotOutputDirectoryError(outputPath)
	}
	ext := strings.ToLower(filepath.Ext(outputPath))
	if ext != "" && ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--output must have no extension or end with .png, .jpg, or .jpeg").WithParam("--output")
	}
	if _, err := runtime.ResolveSavePath(outputPath); err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--output invalid: %v", err).WithParam("--output").WithCause(err)
	}
	if info, err := runtime.FileIO().Stat(outputPath); err == nil {
		if info.IsDir() {
			return screenshotOutputDirectoryError(outputPath)
		}
	} else if !isScreenshotFileNotExist(err) {
		return errs.NewInternalError(errs.SubtypeFileIO, "inspect --output path %s: %v", outputPath, err).WithCause(err)
	}
	return nil
}

func screenshotOutputDirectoryError(outputPath string) error {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, "--output expects a file path, got directory %q", outputPath).
		WithParam("--output").
		WithHint("use --output-dir <directory> for directory output")
}

func ensureScreenshotOutputDir(runtime *common.RuntimeContext, outputDir string) (string, error) {
	return validateScreenshotOutputDir(runtime, outputDir)
}

func saveSlideScreenshots(runtime *common.RuntimeContext, data map[string]interface{}, outputDir string, presentationID string, outputPath string, overwrite bool) ([]map[string]interface{}, error) {
	items := common.GetSlice(data, "slide_images")
	if len(items) == 0 {
		return nil, slidesScreenshotAPIDataError(data, "slides screenshot returned no slide_images")
	}
	if outputPath != "" && len(items) != 1 {
		return nil, slidesScreenshotAPIDataError(data, "slides screenshot returned %d slide_images for single --output", len(items))
	}
	saved := make([]map[string]interface{}, 0, len(items))
	for i, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			return nil, slidesScreenshotAPIDataError(data, "slides screenshot returned invalid slide_images[%d]", i)
		}
		item, err := saveSlideScreenshotImage(runtime, m, outputDir, slideScreenshotListFileBase(presentationID, m, i), "", outputPath, overwrite)
		if err != nil {
			if isSlidesScreenshotPassthroughError(err) {
				return nil, err
			}
			return nil, slidesScreenshotAPIDataError(data, "slides screenshot returned invalid slide_images[%d]: %v", i, err)
		}
		saved = append(saved, item)
	}
	return saved, nil
}

func saveRenderedSlideScreenshot(runtime *common.RuntimeContext, data map[string]interface{}, outputDir string, outputName string, outputPath string, overwrite bool) ([]map[string]interface{}, error) {
	item := common.GetMap(data, "slide_image")
	if item == nil {
		return nil, slidesScreenshotAPIDataError(data, "slides render screenshot returned no slide_image")
	}
	saved, err := saveSlideScreenshotImage(runtime, item, outputDir, outputName, "rendered-slide", outputPath, overwrite)
	if err != nil {
		if isSlidesScreenshotPassthroughError(err) {
			return nil, err
		}
		return nil, slidesScreenshotAPIDataError(data, "slides render screenshot returned invalid slide_image: %v", err)
	}
	return []map[string]interface{}{saved}, nil
}

func saveSlideScreenshotImage(runtime *common.RuntimeContext, item map[string]interface{}, outputDir string, outputName string, fallbackName string, outputPath string, overwrite bool) (map[string]interface{}, error) {
	slideID := strings.TrimSpace(common.GetString(item, "slide_id"))
	ext, label, err := slideScreenshotFormat(item)
	if err != nil {
		return nil, slidesScreenshotImageDataError(slideID, "%s", err)
	}
	encoded := strings.TrimSpace(common.GetString(item, "data"))
	if encoded == "" {
		return nil, slidesScreenshotImageDataError(slideID, "empty image data")
	}
	imageBytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, slidesScreenshotImageDataCauseError(slideID, err, "decode screenshot: %s", err)
	}
	var path string
	if outputPath != "" {
		path, err = writeScreenshotOutputPath(runtime, slideScreenshotOutputPathForFormat(outputPath, ext), imageBytes, overwrite)
	} else {
		fileBase := strings.TrimSpace(outputName)
		if fileBase == "" {
			fileBase = slideID
		}
		if fileBase == "" {
			fileBase = fallbackName
		}
		path, err = writeUniqueScreenshotFile(runtime, outputDir, fileBase, ext, imageBytes)
	}
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"slide_id":     slideID,
		"slide_number": slideScreenshotInt(item, "slide_number"),
		"format":       label,
		"path":         path,
		"size":         len(imageBytes),
	}, nil
}

func slideScreenshotOutputExtMatches(outputExt string, responseExt string) bool {
	outputExt = strings.ToLower(outputExt)
	if responseExt == "jpg" {
		return outputExt == ".jpg" || outputExt == ".jpeg"
	}
	return outputExt == "."+responseExt
}

func slideScreenshotOutputPathForFormat(outputPath string, responseExt string) string {
	outputExt := filepath.Ext(outputPath)
	if outputExt == "" {
		return outputPath + "." + responseExt
	}
	if slideScreenshotOutputExtMatches(outputExt, responseExt) {
		return outputPath
	}
	return strings.TrimSuffix(outputPath, outputExt) + "." + responseExt
}

func slideScreenshotListFileBase(presentationID string, item map[string]interface{}, index int) string {
	presentationID = strings.TrimSpace(presentationID)
	slideID := strings.TrimSpace(common.GetString(item, "slide_id"))
	slideNumber := slideScreenshotInt(item, "slide_number")
	if presentationID != "" {
		switch {
		case slideNumber > 0 && slideID != "":
			return fmt.Sprintf("%s_p%03d_%s", presentationID, slideNumber, slideID)
		case slideNumber > 0:
			return fmt.Sprintf("%s_p%03d", presentationID, slideNumber)
		case slideID != "":
			return fmt.Sprintf("%s_%s", presentationID, slideID)
		}
	}
	if slideID != "" {
		return slideID
	}
	if slideNumber := slideScreenshotInt(item, "slide_number"); slideNumber > 0 {
		return fmt.Sprintf("slide-%d", slideNumber)
	}
	return fmt.Sprintf("slide-%d", index+1)
}

func slideScreenshotFormat(item map[string]interface{}) (string, string, error) {
	format := slideScreenshotInt(item, "format")
	switch format {
	case 1:
		return "png", "png", nil
	case 2:
		return "jpg", "jpeg", nil
	default:
		return "", "", errs.NewAPIError(errs.SubtypeInvalidResponse, "unsupported screenshot format %d", format)
	}
}

func slidesScreenshotImageDataError(slideID string, format string, args ...interface{}) error {
	msg := fmt.Sprintf(format, args...)
	if slideID != "" {
		msg = fmt.Sprintf("%s for slide %s", msg, slideID)
	}
	return errs.NewAPIError(errs.SubtypeInvalidResponse, "%s", msg)
}

func slidesScreenshotImageDataCauseError(slideID string, cause error, format string, args ...interface{}) error {
	msg := fmt.Sprintf(format, args...)
	if slideID != "" {
		msg = fmt.Sprintf("%s for slide %s", msg, slideID)
	}
	return errs.NewAPIError(errs.SubtypeInvalidResponse, "%s", msg).WithCause(cause)
}

func slideScreenshotInt(item map[string]interface{}, key string) int {
	n, ok := util.ToFloat64(item[key])
	if !ok {
		return 0
	}
	return int(n)
}

func doSlidesScreenshotAPIJSONWithLogID(runtime *common.RuntimeContext, method string, apiPath string, query larkcore.QueryParams, body interface{}) (map[string]interface{}, error) {
	req := &larkcore.ApiReq{
		HttpMethod:  method,
		ApiPath:     apiPath,
		QueryParams: query,
	}
	if body != nil {
		req.Body = body
	}
	resp, err := runtime.DoAPI(req)
	if err != nil {
		return nil, errs.WrapInternal(err)
	}
	data, err := runtime.ClassifyAPIResponse(resp)
	if err != nil {
		return data, err
	}
	if data == nil {
		data = map[string]interface{}{}
	}
	if logID := strings.TrimSpace(resp.Header.Get("x-tt-logid")); logID != "" {
		data["log_id"] = logID
	}
	return data, nil
}

func enrichSlidesScreenshotSelectorError(err error, slideNumbers []int) error {
	if len(slideNumbers) == 0 {
		return err
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		return err
	}
	if p.Hint == "" {
		p.Hint = "slide_numbers was rejected by the server; verify the page number exists in this presentation, or retry with --slide-id."
	}
	return err
}

func slidesScreenshotAPIDataError(data map[string]interface{}, format string, args ...interface{}) error {
	msg := fmt.Sprintf(format, args...)
	err := errs.NewAPIError(errs.SubtypeInvalidResponse, "%s; raw_data=%v", msg, summarizeScreenshotAPIData(data))
	if logID := strings.TrimSpace(common.GetString(data, "log_id")); logID != "" {
		err = err.WithLogID(logID)
	}
	return err
}

func isSlidesScreenshotPassthroughError(err error) bool {
	_, ok := errs.ProblemOf(err)
	return ok
}

func summarizeScreenshotAPIData(v interface{}) interface{} {
	switch x := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(x))
		for k, val := range x {
			out[k] = summarizeScreenshotAPIData(val)
		}
		return out
	case []interface{}:
		out := make([]interface{}, 0, len(x))
		for i, val := range x {
			if i >= 20 {
				out = append(out, fmt.Sprintf("<omitted %d more items>", len(x)-i))
				break
			}
			out = append(out, summarizeScreenshotAPIData(val))
		}
		return out
	case string:
		if len(x) > 512 {
			return fmt.Sprintf("<omitted string length=%d prefix=%q>", len(x), x[:64])
		}
		return x
	default:
		return x
	}
}

func safeScreenshotFileBase(base string) string {
	name := unsafeScreenshotFileCharRegex.ReplaceAllString(base, "_")
	name = strings.Trim(name, "._-")
	if name == "" {
		name = "slide"
	}
	return name
}

func writeUniqueScreenshotFile(runtime *common.RuntimeContext, outputDir string, fileBase string, ext string, imageBytes []byte) (string, error) {
	base := safeScreenshotFileBase(fileBase)
	return writeUniqueScreenshotPath(runtime, filepath.Join(outputDir, base+"."+ext), imageBytes)
}

func writeScreenshotOutputPath(runtime *common.RuntimeContext, outputPath string, imageBytes []byte, overwrite bool) (string, error) {
	if info, err := runtime.FileIO().Stat(outputPath); err == nil {
		if info.IsDir() {
			return "", screenshotOutputDirectoryError(outputPath)
		}
		if !overwrite {
			return "", errs.NewValidationError(errs.SubtypeFailedPrecondition, "output file already exists: %s", outputPath).
				WithParam("--output").
				WithHint("add --overwrite to replace the existing file")
		}
	} else if !isScreenshotFileNotExist(err) {
		return "", errs.NewInternalError(errs.SubtypeFileIO, "inspect screenshot output %s: %v", outputPath, err).WithCause(err)
	}
	if _, err := runtime.FileIO().Save(outputPath, fileio.SaveOptions{}, bytes.NewReader(imageBytes)); err != nil {
		return "", common.WrapSaveErrorTyped(err)
	}
	resolvedPath, err := runtime.ResolveSavePath(outputPath)
	if err != nil {
		return "", errs.NewInternalError(errs.SubtypeFileIO, "resolve saved screenshot path %s: %v", outputPath, err).WithCause(err)
	}
	return resolvedPath, nil
}

func writeUniqueScreenshotPath(runtime *common.RuntimeContext, outputPath string, imageBytes []byte) (string, error) {
	ext := filepath.Ext(outputPath)
	base := strings.TrimSuffix(outputPath, ext)
	for i := 0; i < 1000; i++ {
		candidate := outputPath
		if i > 0 {
			candidate = fmt.Sprintf("%s_%d%s", base, i+1, ext)
		}
		if _, err := runtime.FileIO().Stat(candidate); err == nil {
			continue
		} else if !isScreenshotFileNotExist(err) {
			return "", errs.NewInternalError(errs.SubtypeFileIO, "write screenshot %s: %v", candidate, err).WithCause(err)
		}
		if _, err := runtime.FileIO().Save(candidate, fileio.SaveOptions{}, bytes.NewReader(imageBytes)); err != nil {
			return "", common.WrapSaveErrorTyped(err)
		}
		resolvedPath, err := runtime.ResolveSavePath(candidate)
		if err != nil {
			return "", errs.NewInternalError(errs.SubtypeFileIO, "resolve saved screenshot path %s: %v", candidate, err).WithCause(err)
		}
		return resolvedPath, nil
	}
	return "", errs.NewInternalError(errs.SubtypeFileIO, "write screenshot %s: too many duplicate file names", outputPath)
}

func isScreenshotFileNotExist(err error) bool {
	return os.IsNotExist(err)
}
