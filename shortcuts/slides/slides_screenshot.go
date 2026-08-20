// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"io"
	"math"
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
	xdraw "golang.org/x/image/draw"
)

const (
	defaultSlidesScreenshotDir = ".lark-slides/screenshots"
	maxSlidesPerScreenshot     = 10
	maxSlidesPerOverview       = 20
	defaultOverviewColumns     = 4
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
	// wiki:node:read is required only for wiki URLs; slides:presentation:read
	// is required only by --overview to enumerate the current page order.
	ConditionalScopes: []string{"wiki:node:read", "slides:presentation:read"},
	AuthTypes:         []string{"user", "bot"},
	Flags: []common.Flag{
		listModePresentationRefFlag(),
		{Name: "slide-id", Aliases: []string{"slide-ids", "slides"}, Type: "string_slice", Desc: "slide page identifier (repeat or comma-separated for multiple slides; max 10 pages per request)"},
		{Name: "slide-number", Aliases: []string{"slide-numbers"}, Type: "int_array", Desc: "slide page number (repeat for multiple slides; max 10 pages per request)"},
		{Name: "slide", Desc: "hidden alias routed to --slide-number for digits, otherwise --slide-id", Hidden: true},
		{Name: "content", Desc: "slide XML content to render directly instead of fetching existing slides", Input: []string{common.File, common.Stdin}},
		{Name: "output", Desc: "preferred relative output path for a single screenshot (extension optional; .png, .jpg, or .jpeg)"},
		{Name: "output-dir", Default: defaultSlidesScreenshotDir, Desc: "relative directory for saved screenshots"},
		{Name: "output-name", Desc: "file name stem for --content render output"},
		{Name: "overview", Type: "bool", Desc: "render one indexed overview PNG containing up to 20 current slides; use --overview-page for additional slides; cannot be combined with slide selectors, --content, or --region"},
		{Name: "overview-page", Type: "int", Default: "1", Desc: "1-based overview page (up to 20 slides); continue with next_overview_page while has_next is true"},
		{Name: "region", Desc: "crop exactly one existing slide as x,y,width,height in the rendered screenshot pixels; cannot be combined with --overview"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		overview := runtime.Bool("overview")
		if runtime.Changed("region") && strings.TrimSpace(runtime.Str("region")) == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--region cannot be empty").WithParam("--region").WithHint("provide x,y,width,height in rendered screenshot pixels")
		}
		region, regionSet, err := parseSlidesScreenshotRegion(runtime.Str("region"))
		if err != nil {
			return err
		}
		if overview && regionSet {
			return slidesScreenshotFlagErrorf("--overview cannot be combined with --region")
		}
		if regionSet && runtime.Changed("content") {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--region cannot be combined with --content").WithParams(
				errs.InvalidParam{Name: "--region", Reason: "only crops an existing slide selected by --slide-id or --slide-number"},
				errs.InvalidParam{Name: "--content", Reason: "render mode does not select an existing slide"},
			)
		}
		if overview && (runtime.Changed("content") || slidesScreenshotHasSelectorInput(runtime)) {
			return slidesScreenshotFlagErrorf("--overview requires --presentation without --content or slide selectors")
		}
		if runtime.Changed("overview-page") && !overview {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--overview-page requires --overview").WithParam("--overview-page")
		}
		if overview && runtime.Int("overview-page") < 1 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--overview-page must be at least 1").WithParam("--overview-page")
		}
		renderMode := runtime.Changed("content")
		selectorCount := 1
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
			if !overview && len(slideIDs) == 0 && len(slideNumbers) == 0 {
				return slidesScreenshotMissingSelectorError()
			}
			selectorCount = len(slideIDs) + len(slideNumbers)
			if overview {
				// Overview is one synthesized local image even though it deliberately
				// has no page selector. Keep --output's single-file contract usable.
				selectorCount = 1
			}
			if regionSet && selectorCount != 1 {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--region requires exactly one slide selector").WithParam("--region").WithHint("use one --slide-id or one --slide-number")
			}
			_ = region
			if err := validateSlidesScreenshotSelectorLimit(selectorCount); err != nil {
				return err
			}
			// Overview reads the presentation XML before rendering. Keep this
			// conditional preflight in Validate so --dry-run reports the same
			// authorization requirement as execution, and a wiki URL cannot reach
			// its resolve call before a missing Slides read scope is surfaced.
			if overview {
				if err := runtime.EnsureScopes([]string{"slides:presentation:read"}); err != nil {
					return err
				}
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
			if overview {
				ext := strings.ToLower(filepath.Ext(runtime.Str("output")))
				if ext != "" && ext != ".png" {
					return errs.NewValidationError(errs.SubtypeInvalidArgument, "--overview output must be .png").WithParam("--output")
				}
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
		if runtime.Bool("overview") {
			if ref.Kind == "wiki" {
				presentationID = "<resolved_slides_token>"
				dry.GET("/open-apis/wiki/v2/spaces/get_node").
					Desc("Resolve the wiki node to its Slides presentation before reading the overview page").
					Params(map[string]interface{}{"token": ref.Token})
			}
			overviewURL := fmt.Sprintf("/open-apis/slides_ai/v1/xml_presentations/%s", validate.EncodePathSegment(presentationID))
			screenshotURL := overviewURL + "/slide_images"
			dry.GET(overviewURL).
				Desc(fmt.Sprintf("Fetch current presentation XML to select overview page %d (at most %d slides)", runtime.Int("overview-page"), maxSlidesPerOverview)).
				Params(map[string]interface{}{"revision_id": -1})
			dry.POST(screenshotURL).
				Desc("Render the first dynamic overview batch (slide IDs 1-10 of the selected overview page)").
				Body(map[string]interface{}{"slide_ids": []string{"<overview page slide IDs 1-10>"}})
			dry.POST(screenshotURL).
				Desc("Render the optional second dynamic overview batch (slide IDs 11-20; sent only when the selected page has more than 10 slides)").
				Body(map[string]interface{}{"slide_ids": []string{"<overview page slide IDs 11-20, if present>"}})
			return setSlidesScreenshotDryRunOutput(dry, runtime).Set("base64_output", "suppressed; decoded locally and composed into overview PNG during execution")
		}
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
		if runtime.Bool("overview") {
			return executeSlidesScreenshotOverview(runtime)
		}
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
		var region slidesScreenshotRegion
		if parsed, set, _ := parseSlidesScreenshotRegion(runtime.Str("region")); set {
			region = parsed
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

		var crop map[string]interface{}
		if _, set, _ := parseSlidesScreenshotRegion(runtime.Str("region")); set {
			crop, err = cropSlidesScreenshotResponse(data, region)
			if err != nil {
				return err
			}
		}
		saved, err := saveSlideScreenshots(runtime, data, outputTarget.safeOutputDir, presentationID, outputTarget.requested)
		if err != nil {
			return err
		}
		result := map[string]interface{}{
			"xml_presentation_id": presentationID,
			"screenshots":         saved,
		}
		if crop != nil {
			result["region"] = crop
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
	saved, err := saveRenderedSlideScreenshot(runtime, data, outputTarget.safeOutputDir, runtime.Str("output-name"), outputTarget.requested)
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
	if outputPath := runtime.Str("output"); outputPath != "" {
		return dry.Set("output", outputPath)
	}
	return dry.Set("output_dir", runtime.Str("output-dir"))
}

type slidesScreenshotOutputTarget struct {
	requested         string
	requestedResolved string
	outputDir         string
	safeOutputDir     string
}

func resolveSlidesScreenshotOutputTarget(runtime *common.RuntimeContext) (slidesScreenshotOutputTarget, error) {
	target := slidesScreenshotOutputTarget{
		requested: runtime.Str("output"),
		outputDir: runtime.Str("output-dir"),
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
	if outputPath == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--output cannot be empty").WithParam("--output")
	}
	if strings.TrimSpace(outputPath) != outputPath {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--output cannot have leading or trailing whitespace").
			WithParam("--output").
			WithHint("remove surrounding whitespace and retry")
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

func saveSlideScreenshots(runtime *common.RuntimeContext, data map[string]interface{}, outputDir string, presentationID string, outputPath string) ([]map[string]interface{}, error) {
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
		item, err := saveSlideScreenshotImage(runtime, m, outputDir, slideScreenshotListFileBase(presentationID, m, i), "", outputPath)
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

func saveRenderedSlideScreenshot(runtime *common.RuntimeContext, data map[string]interface{}, outputDir string, outputName string, outputPath string) ([]map[string]interface{}, error) {
	item := common.GetMap(data, "slide_image")
	if item == nil {
		return nil, slidesScreenshotAPIDataError(data, "slides render screenshot returned no slide_image")
	}
	saved, err := saveSlideScreenshotImage(runtime, item, outputDir, outputName, "rendered-slide", outputPath)
	if err != nil {
		if isSlidesScreenshotPassthroughError(err) {
			return nil, err
		}
		return nil, slidesScreenshotAPIDataError(data, "slides render screenshot returned invalid slide_image: %v", err)
	}
	return []map[string]interface{}{saved}, nil
}

func saveSlideScreenshotImage(runtime *common.RuntimeContext, item map[string]interface{}, outputDir string, outputName string, fallbackName string, outputPath string) (map[string]interface{}, error) {
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
	config, _, configErr := image.DecodeConfig(bytes.NewReader(imageBytes))
	var path string
	if outputPath != "" {
		path, err = writeUniqueScreenshotPath(runtime, slideScreenshotOutputPathForFormat(outputPath, ext), imageBytes)
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
	result := map[string]interface{}{
		"slide_id":     slideID,
		"slide_number": slideScreenshotInt(item, "slide_number"),
		"format":       label,
		"path":         path,
		"size":         len(imageBytes),
	}
	// Preserve the shortcut's established behavior for an opaque server payload
	// that is saved successfully but cannot be decoded locally. Real screenshot
	// images always carry image_size; omitting it here avoids changing the
	// historical save contract into a new decode failure.
	if configErr == nil {
		result["image_size"] = map[string]int{"width": config.Width, "height": config.Height}
	}
	return result, nil
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

type slidesScreenshotRegion struct{ X, Y, Width, Height int }

func parseSlidesScreenshotRegion(raw string) (slidesScreenshotRegion, bool, error) {
	if strings.TrimSpace(raw) == "" {
		return slidesScreenshotRegion{}, false, nil
	}
	parts := strings.Split(raw, ",")
	if len(parts) != 4 {
		return slidesScreenshotRegion{}, false, errs.NewValidationError(errs.SubtypeInvalidArgument, "--region must be x,y,width,height in rendered screenshot pixels").WithParam("--region")
	}
	vals := [4]int{}
	for i, part := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return slidesScreenshotRegion{}, false, errs.NewValidationError(errs.SubtypeInvalidArgument, "--region values must be integers").WithParam("--region")
		}
		vals[i] = n
	}
	r := slidesScreenshotRegion{vals[0], vals[1], vals[2], vals[3]}
	if r.X < 0 || r.Y < 0 || r.Width <= 0 || r.Height <= 0 {
		return slidesScreenshotRegion{}, false, errs.NewValidationError(errs.SubtypeInvalidArgument, "--region requires non-negative x,y and positive width,height").WithParam("--region")
	}
	return r, true, nil
}

func validateSlidesScreenshotRegionBounds(region slidesScreenshotRegion, imageSize image.Point) error {
	// Compare the remaining image size instead of adding X+Width so hostile
	// integer inputs cannot wrap before the bounds check.
	if region.X >= imageSize.X || region.Y >= imageSize.Y || region.Width > imageSize.X-region.X || region.Height > imageSize.Y-region.Y {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--region must stay within the %dx%d rendered screenshot", imageSize.X, imageSize.Y).WithParam("--region").WithHint(fmt.Sprintf("use x+width<=%d and y+height<=%d", imageSize.X, imageSize.Y))
	}
	return nil
}

func cropSlidesScreenshotResponse(data map[string]interface{}, region slidesScreenshotRegion) (map[string]interface{}, error) {
	items := common.GetSlice(data, "slide_images")
	if len(items) != 1 {
		return nil, slidesScreenshotAPIDataError(data, "--region requires exactly one rendered slide image")
	}
	item, ok := items[0].(map[string]interface{})
	if !ok {
		return nil, slidesScreenshotAPIDataError(data, "slides screenshot returned invalid slide_images[0]")
	}
	encoded := common.GetString(item, "data")
	b, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, slidesScreenshotImageDataCauseError(common.GetString(item, "slide_id"), err, "decode screenshot for --region: %s", err)
	}
	src, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, slidesScreenshotImageDataCauseError(common.GetString(item, "slide_id"), err, "decode screenshot image for --region: %s", err)
	}
	bounds := src.Bounds()
	if err := validateSlidesScreenshotRegionBounds(region, bounds.Size()); err != nil {
		return nil, err
	}
	dst := image.NewRGBA(image.Rect(0, 0, region.Width, region.Height))
	draw.Draw(dst, dst.Bounds(), src, image.Point{X: bounds.Min.X + region.X, Y: bounds.Min.Y + region.Y}, draw.Src)
	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return nil, errs.NewInternalError(errs.SubtypeFileIO, "encode --region screenshot: %v", err).WithCause(err)
	}
	item["data"] = base64.StdEncoding.EncodeToString(out.Bytes())
	item["format"] = 1
	return map[string]interface{}{
		"requested_pixel_rect": map[string]int{"x": region.X, "y": region.Y, "width": region.Width, "height": region.Height},
		"source_image_size":    map[string]int{"width": bounds.Dx(), "height": bounds.Dy()},
		"output_image_size":    map[string]int{"width": region.Width, "height": region.Height},
		"format":               "png",
	}, nil
}

func executeSlidesScreenshotOverview(runtime *common.RuntimeContext) error {
	ref, err := parsePresentationRef(slidesScreenshotPresentation(runtime))
	if err != nil {
		return err
	}
	if err := runtime.EnsureScopes([]string{"slides:presentation:read"}); err != nil {
		return err
	}
	presentationID, err := resolvePresentationID(runtime, ref)
	if err != nil {
		return err
	}
	data, err := runtime.CallAPITyped("GET", fmt.Sprintf("/open-apis/slides_ai/v1/xml_presentations/%s", validate.EncodePathSegment(presentationID)), map[string]interface{}{"revision_id": -1}, nil)
	if err != nil {
		return err
	}
	ids, err := slidesScreenshotOverviewIDs(data)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return errs.NewValidationError(errs.SubtypeFailedPrecondition, "--overview found no current slide IDs").WithHint("verify the presentation contains slides, then retry")
	}
	overviewPage := runtime.Int("overview-page")
	maxPage := (len(ids) + maxSlidesPerOverview - 1) / maxSlidesPerOverview
	if overviewPage > maxPage {
		return errs.NewValidationError(errs.SubtypeFailedPrecondition, "--overview-page %d is outside this %d-slide presentation", overviewPage, len(ids)).WithParam("--overview-page").WithHint(fmt.Sprintf("use a value between 1 and %d", maxPage))
	}
	start := (overviewPage - 1) * maxSlidesPerOverview
	end := start + maxSlidesPerOverview
	if end > len(ids) {
		end = len(ids)
	}
	pageIDs := ids[start:end]
	thumbs := make([]image.Image, 0, len(pageIDs))
	for batchStart := 0; batchStart < len(pageIDs); batchStart += maxSlidesPerScreenshot {
		batchEnd := batchStart + maxSlidesPerScreenshot
		if batchEnd > len(pageIDs) {
			batchEnd = len(pageIDs)
		}
		url := fmt.Sprintf("/open-apis/slides_ai/v1/xml_presentations/%s/slide_images", validate.EncodePathSegment(presentationID))
		pageData, err := doSlidesScreenshotAPIJSONWithLogID(runtime, "POST", url, larkcore.QueryParams{}, map[string]interface{}{"slide_ids": pageIDs[batchStart:batchEnd]})
		if err != nil {
			return err
		}
		batch, err := slidesScreenshotOverviewImages(pageData, pageIDs[batchStart:batchEnd])
		if err != nil {
			return err
		}
		thumbs = append(thumbs, batch...)
	}
	overview, cells := composeSlidesOverviewFromIndex(thumbs, defaultOverviewColumns, start+1)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, overview); err != nil {
		return errs.NewInternalError(errs.SubtypeFileIO, "encode overview PNG: %v", err).WithCause(err)
	}
	target, err := resolveSlidesScreenshotOutputTarget(runtime)
	if err != nil {
		return err
	}
	path := target.requested
	if path == "" {
		path = filepath.Join(target.safeOutputDir, safeScreenshotFileBase(presentationID)+"_overview.png")
	} else if filepath.Ext(path) == "" {
		path += ".png"
	}
	path, err = writeUniqueScreenshotPath(runtime, path, encoded.Bytes())
	if err != nil {
		return err
	}
	slides := make([]map[string]interface{}, len(pageIDs))
	for i, id := range pageIDs {
		cell := cells[i]
		index := start + i + 1
		slides[i] = map[string]interface{}{"index": index, "label": fmt.Sprintf("#%02d", index), "slide_id": id, "slide_number": index, "row": cell.row, "column": cell.column, "tile": overviewRectOutput(cell.tile), "thumbnail": overviewRectOutput(cell.thumbnail)}
	}
	overviewImageSize := map[string]int{"width": overview.Bounds().Dx(), "height": overview.Bounds().Dy()}
	overviewData := map[string]interface{}{"path": path, "format": "png", "size": encoded.Len(), "image_size": overviewImageSize, "columns": defaultOverviewColumns, "total_slides": len(ids), "overview_page": overviewPage, "page_size": maxSlidesPerOverview, "slide_range": map[string]int{"start": start + 1, "end": end}, "has_previous": overviewPage > 1, "has_next": end < len(ids), "slides": slides}
	if overviewPage > 1 {
		overviewData["previous_overview_page"] = overviewPage - 1
	}
	if end < len(ids) {
		overviewData["next_overview_page"] = overviewPage + 1
	}
	result := map[string]interface{}{"xml_presentation_id": presentationID, "overview": overviewData}
	// Overview is one synthesized local image. Mirror the normal screenshot
	// output locator at the root while retaining overview.path and its paging
	// metadata for overview-aware consumers.
	setSlidesScreenshotResultOutput(result, target, []map[string]interface{}{{"path": path}})
	runtime.Out(result, nil)
	return nil
}

// slidesScreenshotOverviewImages verifies that the API response is a bijection
// with requestedIDs, then returns its images in request order. The API's array
// order is deliberately not trusted: a mismatched thumbnail makes the overview
// index unsafe for an agent to use in a later per-slide review.
func slidesScreenshotOverviewImages(data map[string]interface{}, requestedIDs []string) ([]image.Image, error) {
	items := common.GetSlice(data, "slide_images")
	if len(items) != len(requestedIDs) {
		return nil, slidesScreenshotAPIDataError(data, "slides screenshot returned %d images for %d requested slide IDs", len(items), len(requestedIDs))
	}
	requested := make(map[string]struct{}, len(requestedIDs))
	for _, id := range requestedIDs {
		requested[id] = struct{}{}
	}
	byID := make(map[string]map[string]interface{}, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]interface{})
		if !ok {
			return nil, slidesScreenshotAPIDataError(data, "slides screenshot returned invalid slide image")
		}
		id := common.GetString(item, "slide_id")
		if _, ok := requested[id]; !ok || id == "" {
			return nil, slidesScreenshotAPIDataError(data, "slides screenshot returned unexpected slide_id %q", id)
		}
		if _, duplicate := byID[id]; duplicate {
			return nil, slidesScreenshotAPIDataError(data, "slides screenshot returned duplicate slide_id %q", id)
		}
		byID[id] = item
	}
	images := make([]image.Image, 0, len(requestedIDs))
	for _, id := range requestedIDs {
		item, ok := byID[id]
		if !ok {
			return nil, slidesScreenshotAPIDataError(data, "slides screenshot did not return requested slide_id %q", id)
		}
		b, err := base64.StdEncoding.DecodeString(common.GetString(item, "data"))
		if err != nil {
			return nil, slidesScreenshotImageDataCauseError(id, err, "decode screenshot for --overview: %s", err)
		}
		img, _, err := image.Decode(bytes.NewReader(b))
		if err != nil {
			return nil, slidesScreenshotImageDataCauseError(id, err, "decode screenshot image for --overview: %s", err)
		}
		images = append(images, img)
	}
	return images, nil
}

func slidesScreenshotIDsFromXML(content string) ([]string, error) {
	d := xml.NewDecoder(strings.NewReader(content))
	var ids []string
	seenRoot := false
	for {
		tok, err := d.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				if !seenRoot {
					return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "presentation XML for --overview has no root element")
				}
				break
			}
			return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "parse presentation XML for --overview: %v", err).WithCause(err)
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if !seenRoot {
			seenRoot = true
			if start.Name.Local != "presentation" {
				return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "presentation XML for --overview has root element %q, want presentation", start.Name.Local)
			}
			continue
		}
		if start.Name.Local != "slide" {
			continue
		}
		for _, a := range start.Attr {
			if a.Name.Local == "id" && strings.TrimSpace(a.Value) != "" {
				ids = append(ids, a.Value)
				break
			}
		}
	}
	return ids, nil
}

func slidesScreenshotOverviewIDs(data map[string]interface{}) ([]string, error) {
	presentation, ok := data["xml_presentation"].(map[string]interface{})
	if !ok {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "xml_presentation response for --overview must be an object")
	}
	content, ok := presentation["content"].(string)
	if !ok || strings.TrimSpace(content) == "" {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "xml_presentation.content response for --overview must be a non-empty string")
	}
	return slidesScreenshotIDsFromXML(content)
}

type overviewCell struct {
	row, column     int
	tile, thumbnail image.Rectangle
}

func overviewRectOutput(r image.Rectangle) map[string]int {
	return map[string]int{"x": r.Min.X, "y": r.Min.Y, "width": r.Dx(), "height": r.Dy()}
}

func composeSlidesOverview(images []image.Image, columns int) (*image.RGBA, []overviewCell) {
	return composeSlidesOverviewFromIndex(images, columns, 1)
}

func composeSlidesOverviewFromIndex(images []image.Image, columns int, firstIndex int) (*image.RGBA, []overviewCell) {
	if columns < 1 {
		columns = defaultOverviewColumns
	}
	const thumbW, thumbH, headerH, pad = 320, 180, 24, 16
	rows := int(math.Ceil(float64(len(images)) / float64(columns)))
	tileH := headerH + thumbH
	out := image.NewRGBA(image.Rect(0, 0, columns*thumbW+(columns+1)*pad, rows*tileH+(rows+1)*pad))
	draw.Draw(out, out.Bounds(), &image.Uniform{C: color.RGBA{R: 246, G: 247, B: 249, A: 255}}, image.Point{}, draw.Src)
	cells := make([]overviewCell, len(images))
	for i, src := range images {
		r, c := i/columns, i%columns
		x, y := pad+c*(thumbW+pad), pad+r*(tileH+pad)
		tile := image.Rect(x, y, x+thumbW, y+tileH)
		thumbnail := image.Rect(x, y+headerH, x+thumbW, y+tileH)
		draw.Draw(out, image.Rect(x, y, x+thumbW, y+headerH), &image.Uniform{C: color.RGBA{R: 49, G: 50, B: 53, A: 255}}, image.Point{}, draw.Src)
		xdraw.CatmullRom.Scale(out, thumbnail, src, src.Bounds(), draw.Over, nil)
		drawOverviewThumbnailBorder(out, thumbnail)
		drawOverviewIndex(out, image.Pt(x+9, y+5), firstIndex+i)
		cells[i] = overviewCell{row: r, column: c, tile: tile, thumbnail: thumbnail}
	}
	return out, cells
}

func drawOverviewThumbnailBorder(dst draw.Image, thumbnail image.Rectangle) {
	border := &image.Uniform{C: color.RGBA{R: 205, G: 208, B: 213, A: 255}}
	draw.Draw(dst, image.Rect(thumbnail.Min.X, thumbnail.Min.Y, thumbnail.Max.X, thumbnail.Min.Y+1), border, image.Point{}, draw.Src)
	draw.Draw(dst, image.Rect(thumbnail.Min.X, thumbnail.Max.Y-1, thumbnail.Max.X, thumbnail.Max.Y), border, image.Point{}, draw.Src)
	draw.Draw(dst, image.Rect(thumbnail.Min.X, thumbnail.Min.Y, thumbnail.Min.X+1, thumbnail.Max.Y), border, image.Point{}, draw.Src)
	draw.Draw(dst, image.Rect(thumbnail.Max.X-1, thumbnail.Min.Y, thumbnail.Max.X, thumbnail.Max.Y), border, image.Point{}, draw.Src)
}

var overviewDigitPixels = map[rune][]string{
	'#': {"01010", "11111", "01010", "11111", "01010"},
	'0': {"01110", "10001", "10011", "10101", "11001", "10001", "01110"},
	'1': {"00100", "01100", "00100", "00100", "00100", "00100", "01110"},
	'2': {"01110", "10001", "00001", "00010", "00100", "01000", "11111"},
	'3': {"11110", "00001", "00001", "01110", "00001", "00001", "11110"},
	'4': {"00010", "00110", "01010", "10010", "11111", "00010", "00010"},
	'5': {"11111", "10000", "11110", "00001", "00001", "10001", "01110"},
	'6': {"00110", "01000", "10000", "11110", "10001", "10001", "01110"},
	'7': {"11111", "00001", "00010", "00100", "01000", "01000", "01000"},
	'8': {"01110", "10001", "10001", "01110", "10001", "10001", "01110"},
	'9': {"01110", "10001", "10001", "01111", "00001", "00010", "11100"},
}

func drawOverviewIndex(dst draw.Image, at image.Point, index int) {
	const scale, gap = 2, 3
	x := at.X
	for _, ch := range fmt.Sprintf("#%02d", index) {
		glyph := overviewDigitPixels[ch]
		for y, row := range glyph {
			for col, on := range row {
				if on == '1' {
					draw.Draw(dst, image.Rect(x+col*scale, at.Y+y*scale, x+(col+1)*scale, at.Y+(y+1)*scale), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
				}
			}
		}
		x += (len(glyph[0]) + gap) * scale
	}
}
