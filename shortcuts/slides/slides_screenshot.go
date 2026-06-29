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
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/util"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const defaultSlidesScreenshotDir = ".lark-slides/screenshots"

var unsafeScreenshotFileCharRegex = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// SlidesScreenshot fetches server-rendered slide screenshots and writes them to
// local files. The raw API returns Base64 image payloads; this shortcut keeps
// those payloads out of stdout so agents only see small file metadata.
var SlidesScreenshot = common.Shortcut{
	Service:     "slides",
	Command:     "+screenshot",
	Description: "Save slide screenshots to local files without printing Base64 image data",
	Risk:        "read",
	Scopes:      []string{"slides:presentation:screenshot"},
	// wiki:node:read is required only when --presentation is a wiki URL.
	ConditionalScopes: []string{"wiki:node:read"},
	AuthTypes:         []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "presentation", Desc: "xml_presentation_id, slides URL, or wiki URL that resolves to slides; list mode only"},
		{Name: "slide-id", Type: "string_array", Desc: "slide page identifier (repeat for multiple slides)"},
		{Name: "slide-number", Type: "int_array", Desc: "slide page number (repeat for multiple slides)"},
		{Name: "content", Desc: "slide XML content to render directly instead of fetching existing slides", Input: []string{common.File, common.Stdin}},
		{Name: "output-dir", Default: defaultSlidesScreenshotDir, Desc: "relative directory for saved screenshots"},
		{Name: "output-name", Desc: "file name stem for --content render output"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		renderMode := runtime.Changed("content")
		if renderMode {
			if strings.TrimSpace(runtime.Str("content")) == "" {
				return slidesScreenshotFlagErrorf("--content cannot be empty")
			}
			if len(normalizeSlideIDs(runtime.StrArray("slide-id"))) > 0 || len(runtime.IntArray("slide-number")) > 0 {
				return slidesScreenshotFlagErrorf("--content cannot be used with --slide-id or --slide-number")
			}
			if runtime.Changed("presentation") {
				return slidesScreenshotFlagErrorf("--presentation cannot be used with --content")
			}
		} else {
			ref, err := parsePresentationRef(runtime.Str("presentation"))
			if err != nil {
				return err
			}
			if ref.Kind == "wiki" {
				if err := runtime.EnsureScopes([]string{"wiki:node:read"}); err != nil {
					return err
				}
			}
			if _, err := normalizeSlideNumbers(runtime.IntArray("slide-number")); err != nil {
				return err
			}
			if !hasSlideScreenshotSelector(runtime) {
				return slidesScreenshotFlagErrorf("--slide-id or --slide-number is required")
			}
		}
		if _, err := validateScreenshotOutputDir(runtime, runtime.Str("output-dir")); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		if runtime.Changed("content") {
			return dryRunRenderScreenshot(runtime)
		}
		ref, err := parsePresentationRef(runtime.Str("presentation"))
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		slideIDs := normalizeSlideIDs(runtime.StrArray("slide-id"))
		slideNumbers, err := normalizeSlideNumbers(runtime.IntArray("slide-number"))
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		if len(slideIDs) == 0 && len(slideNumbers) == 0 {
			return common.NewDryRunAPI().Set("error", "--slide-id or --slide-number is required")
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
			dry.Desc(fmt.Sprintf("Fetch %d slide screenshot(s) and save files under %s", len(slideIDs)+len(slideNumbers), runtime.Str("output-dir")))
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
		return dry.Set("output_dir", runtime.Str("output-dir")).Set("base64_output", "suppressed; decoded to local files during execution")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if runtime.Changed("content") {
			return executeRenderScreenshot(runtime)
		}
		ref, err := parsePresentationRef(runtime.Str("presentation"))
		if err != nil {
			return err
		}
		presentationID, err := resolvePresentationID(runtime, ref)
		if err != nil {
			return err
		}

		slideIDs := normalizeSlideIDs(runtime.StrArray("slide-id"))
		slideNumbers, err := normalizeSlideNumbers(runtime.IntArray("slide-number"))
		if err != nil {
			return err
		}
		if len(slideIDs) == 0 && len(slideNumbers) == 0 {
			return slidesScreenshotFlagErrorf("--slide-id or --slide-number is required")
		}
		outputDir := runtime.Str("output-dir")
		safeOutputDir, err := ensureScreenshotOutputDir(runtime, outputDir)
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

		saved, err := saveSlideScreenshots(runtime, data, safeOutputDir, presentationID)
		if err != nil {
			return err
		}
		runtime.Out(map[string]interface{}{
			"xml_presentation_id": presentationID,
			"output_dir":          outputDir,
			"screenshots":         saved,
		}, nil)
		return nil
	},
}

func dryRunRenderScreenshot(runtime *common.RuntimeContext) *common.DryRunAPI {
	content := runtime.Str("content")
	if strings.TrimSpace(content) == "" {
		return common.NewDryRunAPI().Set("error", "--content cannot be empty")
	}
	if len(normalizeSlideIDs(runtime.StrArray("slide-id"))) > 0 || len(runtime.IntArray("slide-number")) > 0 {
		return common.NewDryRunAPI().Set("error", "--content cannot be used with --slide-id or --slide-number")
	}
	if runtime.Changed("presentation") {
		return common.NewDryRunAPI().Set("error", "--presentation cannot be used with --content")
	}
	dry := common.NewDryRunAPI().Desc("Render slide XML content to a screenshot file")
	dry.POST("/open-apis/slides_ai/v1/slide_image/render").
		Body(map[string]interface{}{
			"content": fmt.Sprintf("<xml omitted; length=%d>", len(content)),
		})
	return dry.Set("output_dir", runtime.Str("output-dir")).Set("base64_output", "suppressed; decoded to local file during execution")
}

func executeRenderScreenshot(runtime *common.RuntimeContext) error {
	content := runtime.Str("content")
	if strings.TrimSpace(content) == "" {
		return slidesScreenshotFlagErrorf("--content cannot be empty")
	}
	if len(normalizeSlideIDs(runtime.StrArray("slide-id"))) > 0 || len(runtime.IntArray("slide-number")) > 0 {
		return slidesScreenshotFlagErrorf("--content cannot be used with --slide-id or --slide-number")
	}
	if runtime.Changed("presentation") {
		return slidesScreenshotFlagErrorf("--presentation cannot be used with --content")
	}
	outputDir := runtime.Str("output-dir")
	safeOutputDir, err := ensureScreenshotOutputDir(runtime, outputDir)
	if err != nil {
		return err
	}

	data, err := doSlidesScreenshotAPIJSONWithLogID(runtime, "POST", "/open-apis/slides_ai/v1/slide_image/render", larkcore.QueryParams{}, map[string]interface{}{
		"content": content,
	})
	if err != nil {
		return err
	}
	saved, err := saveRenderedSlideScreenshot(runtime, data, safeOutputDir, runtime.Str("output-name"))
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{
		"output_dir":  outputDir,
		"screenshots": saved,
	}, nil)
	return nil
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

func hasSlideScreenshotSelector(runtime *common.RuntimeContext) bool {
	return len(normalizeSlideIDs(runtime.StrArray("slide-id"))) > 0 || len(runtime.IntArray("slide-number")) > 0
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

func ensureScreenshotOutputDir(runtime *common.RuntimeContext, outputDir string) (string, error) {
	return validateScreenshotOutputDir(runtime, outputDir)
}

func saveSlideScreenshots(runtime *common.RuntimeContext, data map[string]interface{}, outputDir string, presentationID string) ([]map[string]interface{}, error) {
	items := common.GetSlice(data, "slide_images")
	if len(items) == 0 {
		return nil, slidesScreenshotAPIDataError(data, "slides screenshot returned no slide_images")
	}
	saved := make([]map[string]interface{}, 0, len(items))
	for i, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			return nil, slidesScreenshotAPIDataError(data, "slides screenshot returned invalid slide_images[%d]", i)
		}
		item, err := saveSlideScreenshotImage(runtime, m, outputDir, slideScreenshotListFileBase(presentationID, m, i), "")
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

func saveRenderedSlideScreenshot(runtime *common.RuntimeContext, data map[string]interface{}, outputDir string, outputName string) ([]map[string]interface{}, error) {
	item := common.GetMap(data, "slide_image")
	if item == nil {
		return nil, slidesScreenshotAPIDataError(data, "slides render screenshot returned no slide_image")
	}
	saved, err := saveSlideScreenshotImage(runtime, item, outputDir, outputName, "rendered-slide")
	if err != nil {
		if isSlidesScreenshotPassthroughError(err) {
			return nil, err
		}
		return nil, slidesScreenshotAPIDataError(data, "slides render screenshot returned invalid slide_image: %v", err)
	}
	return []map[string]interface{}{saved}, nil
}

func saveSlideScreenshotImage(runtime *common.RuntimeContext, item map[string]interface{}, outputDir string, outputName string, fallbackName string) (map[string]interface{}, error) {
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
	fileBase := strings.TrimSpace(outputName)
	if fileBase == "" {
		fileBase = slideID
	}
	if fileBase == "" {
		fileBase = fallbackName
	}
	path, err := writeUniqueScreenshotFile(runtime, outputDir, fileBase, ext, imageBytes)
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
	for i := 0; i < 1000; i++ {
		candidateBase := base
		if i > 0 {
			candidateBase = fmt.Sprintf("%s_%d", base, i+1)
		}
		path := filepath.Join(outputDir, candidateBase+"."+ext)
		if _, err := runtime.FileIO().Stat(path); err == nil {
			continue
		} else if !isScreenshotFileNotExist(err) {
			return "", errs.NewInternalError(errs.SubtypeFileIO, "write screenshot %s: %v", path, err).WithCause(err)
		}
		if _, err := runtime.FileIO().Save(path, fileio.SaveOptions{}, bytes.NewReader(imageBytes)); err != nil {
			return "", common.WrapSaveErrorTyped(err)
		}
		resolvedPath, err := runtime.ResolveSavePath(path)
		if err != nil {
			return "", errs.NewInternalError(errs.SubtypeFileIO, "resolve saved screenshot path %s: %v", path, err).WithCause(err)
		}
		return resolvedPath, nil
	}
	path := filepath.Join(outputDir, base+"."+ext)
	return "", errs.NewInternalError(errs.SubtypeFileIO, "write screenshot %s: too many duplicate file names", path)
}

func isScreenshotFileNotExist(err error) bool {
	return os.IsNotExist(err)
}
