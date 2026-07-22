// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// SlidesXMLGet fetches the full XML presentation content. When --output is
// provided it writes to a local file; otherwise it returns the XML in the
// standard JSON envelope. Use --slide-id or --slide-number to fetch one page,
// and use --raw for direct XML stdout.
var SlidesXMLGet = common.Shortcut{
	Service:     "slides",
	Command:     "+xml-get",
	Description: "Fetch presentation XML or one slide XML",
	Risk:        "read",
	Scopes:      []string{"slides:presentation:read"},
	// wiki:node:read is required only when --presentation is a wiki URL.
	ConditionalScopes: []string{"wiki:node:read"},
	AuthTypes:         []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "presentation", Desc: "xml_presentation_id, slides URL, or wiki URL that resolves to slides", Required: true},
		{Name: "output", Desc: "local XML output path; must be a relative path within the current directory; existing file is overwritten; omit to return XML in the JSON envelope"},
		{Name: "raw", Type: "bool", Desc: "print raw XML to stdout instead of the JSON envelope; incompatible with --output and --jq"},
		{Name: "slide-id", Desc: "slide page identifier; omit both slide selectors to fetch full presentation XML"},
		{Name: "slide-number", Type: "int", Desc: "1-based slide page number; omit both slide selectors to fetch full presentation XML"},
		{Name: "revision-id", Type: "int", Default: "-1", Desc: "presentation revision_id; -1 means latest"},
		{Name: "remove-attr-id", Type: "bool", Desc: "remove XML id attributes in the returned content; useful for read-only inspection, not precise block editing"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		ref, err := parsePresentationRef(runtime.Str("presentation"))
		if err != nil {
			return err
		}
		if revisionID := runtime.Int("revision-id"); revisionID < -1 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--revision-id must be -1 or a non-negative integer").WithParam("--revision-id")
		}
		if ref.Kind == "wiki" {
			if err := runtime.EnsureScopes([]string{"wiki:node:read"}); err != nil {
				return err
			}
		}
		if err := validateSlidesXMLGetSelector(runtime); err != nil {
			return err
		}
		outputPath := strings.TrimSpace(runtime.Str("output"))
		if outputPath != "" {
			if _, err := runtime.ResolveSavePath(outputPath); err != nil {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--output invalid: %v", err).WithParam("--output").WithCause(err)
			}
		}
		if runtime.Bool("raw") {
			if outputPath != "" {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--raw cannot be used with --output").WithParam("--raw")
			}
			if runtime.JqExpr != "" {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--raw cannot be used with --jq").WithParam("--raw")
			}
			if runtime.Changed("format") && runtime.Format != "json" {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--raw cannot be used with --format %s", runtime.Format).WithParam("--raw")
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		ref, err := parsePresentationRef(runtime.Str("presentation"))
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		presentationID := ref.Token
		dry := common.NewDryRunAPI()
		if ref.Kind == "wiki" {
			presentationID = "<resolved_slides_token>"
			dry.Desc("2-step orchestration: resolve wiki → fetch presentation XML").
				GET("/open-apis/wiki/v2/spaces/get_node").
				Desc("[1] Resolve wiki node to slides presentation").
				Params(map[string]interface{}{"token": ref.Token})
		} else {
			dry.Desc("Fetch presentation XML")
		}
		params := map[string]interface{}{
			"revision_id": runtime.Int("revision-id"),
		}
		slideID := strings.TrimSpace(runtime.Str("slide-id"))
		slideNumber := runtime.Int("slide-number")
		if slideID != "" {
			params["slide_id"] = slideID
		}
		if slideNumber > 0 {
			params["slide_number"] = slideNumber
		}
		if slideID == "" && slideNumber == 0 && runtime.Bool("remove-attr-id") {
			params["remove_attr_id"] = true
		}
		path := fmt.Sprintf("/open-apis/slides_ai/v1/xml_presentations/%s", validate.EncodePathSegment(presentationID))
		if slideID != "" || slideNumber > 0 {
			path += "/slide"
		}
		dry.GET(path).Params(params)
		if outputPath := strings.TrimSpace(runtime.Str("output")); outputPath != "" {
			return dry.Set("output", outputPath).Set("stdout_content", "suppressed; XML content is saved to --output during execution")
		}
		if runtime.Bool("raw") {
			return dry.Set("output", "<stdout>").Set("stdout_content", "raw XML content is printed to stdout during execution")
		}
		return dry.Set("output", "<stdout>").Set("stdout_content", "JSON envelope with XML content is printed to stdout during execution")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		ref, err := parsePresentationRef(runtime.Str("presentation"))
		if err != nil {
			return err
		}
		presentationID, err := resolvePresentationID(runtime, ref)
		if err != nil {
			return err
		}

		if err := validateSlidesXMLGetSelector(runtime); err != nil {
			return err
		}
		params := map[string]interface{}{
			"revision_id": runtime.Int("revision-id"),
		}

		slideID := strings.TrimSpace(runtime.Str("slide-id"))
		slideNumber := runtime.Int("slide-number")
		content, out, err := fetchSlidesXMLGetContent(runtime, presentationID, params, slideID, slideNumber)
		if err != nil {
			return err
		}
		outputPath := strings.TrimSpace(runtime.Str("output"))
		return outputSlidesXMLGetContent(runtime, content, outputPath, out)
	},
}

func validateSlidesXMLGetSelector(runtime *common.RuntimeContext) error {
	slideID := strings.TrimSpace(runtime.Str("slide-id"))
	slideNumber := runtime.Int("slide-number")
	if runtime.Changed("slide-id") && slideID == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--slide-id cannot be empty").WithParam("--slide-id")
	}
	if slideID != "" && slideNumber > 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--slide-id cannot be used with --slide-number").WithParam("--slide-id")
	}
	if runtime.Changed("slide-number") && slideNumber < 1 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--slide-number must be a positive integer").WithParam("--slide-number")
	}
	if (slideID != "" || slideNumber > 0) && runtime.Bool("remove-attr-id") {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--remove-attr-id is only supported when fetching full presentation XML").WithParam("--remove-attr-id")
	}
	return nil
}

func fetchSlidesXMLGetContent(runtime *common.RuntimeContext, presentationID string, params map[string]interface{}, slideID string, slideNumber int) (string, map[string]interface{}, error) {
	if slideID != "" || slideNumber > 0 {
		if slideID != "" {
			params["slide_id"] = slideID
		}
		if slideNumber > 0 {
			params["slide_number"] = slideNumber
		}
		data, err := runtime.CallAPITyped(
			"GET",
			fmt.Sprintf("/open-apis/slides_ai/v1/xml_presentations/%s/slide", validate.EncodePathSegment(presentationID)),
			params,
			nil,
		)
		if err != nil {
			return "", nil, err
		}
		slide := common.GetMap(data, "slide")
		content := common.GetString(slide, "content")
		if content == "" {
			return "", nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "slides xml get returned empty slide.content")
		}
		slideOut := map[string]interface{}{
			"content": content,
		}
		actualSlideID := common.GetString(slide, "slide_id")
		if actualSlideID == "" {
			actualSlideID = slideID
		}
		if actualSlideID != "" {
			slideOut["slide_id"] = actualSlideID
		}
		if slideNumber > 0 {
			slideOut["slide_number"] = slideNumber
		}
		out := map[string]interface{}{
			"xml_presentation_id": presentationID,
			"scope":               "slide",
			"slide":               slideOut,
		}
		if actualSlideID != "" {
			out["slide_id"] = actualSlideID
		}
		if slideNumber > 0 {
			out["slide_number"] = slideNumber
		}
		if revisionID := common.GetFloat(data, "revision_id"); revisionID > 0 {
			out["revision_id"] = int(revisionID)
			slideOut["revision_id"] = int(revisionID)
		}
		return content, out, nil
	}

	if runtime.Bool("remove-attr-id") {
		params["remove_attr_id"] = true
	}
	data, err := runtime.CallAPITyped(
		"GET",
		fmt.Sprintf("/open-apis/slides_ai/v1/xml_presentations/%s", validate.EncodePathSegment(presentationID)),
		params,
		nil,
	)
	if err != nil {
		return "", nil, err
	}

	presentation := common.GetMap(data, "xml_presentation")
	content := common.GetString(presentation, "content")
	if content == "" {
		return "", nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "slides xml get returned empty xml_presentation.content")
	}
	presentationOut := map[string]interface{}{
		"content": content,
	}
	out := map[string]interface{}{
		"xml_presentation_id": presentationID,
		"scope":               "presentation",
		"xml_presentation":    presentationOut,
	}
	if revisionID := common.GetFloat(presentation, "revision_id"); revisionID > 0 {
		out["revision_id"] = int(revisionID)
		presentationOut["revision_id"] = int(revisionID)
	}
	if runtime.Bool("remove-attr-id") {
		out["remove_attr_id"] = true
	}
	return content, out, nil
}

func outputSlidesXMLGetContent(runtime *common.RuntimeContext, content string, outputPath string, out map[string]interface{}) error {
	if outputPath == "" {
		if !runtime.Bool("raw") {
			runtime.OutFormatRaw(out, nil, nil)
			return nil
		}
		if _, err := fmt.Fprint(runtime.IO().Out, content); err != nil {
			return errs.NewInternalError(errs.SubtypeFileIO, "write XML content to stdout: %v", err).WithCause(err)
		}
		return nil
	}

	result, err := runtime.FileIO().Save(outputPath, fileio.SaveOptions{
		ContentType:   "application/xml",
		ContentLength: int64(len(content)),
	}, bytes.NewReader([]byte(content)))
	if err != nil {
		return common.WrapSaveErrorTyped(err)
	}
	resolvedPath, err := runtime.ResolveSavePath(outputPath)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeFileIO, "resolve saved XML path %s: %v", outputPath, err).WithCause(err)
	}

	fileOut := map[string]interface{}{
		"xml_presentation_id": out["xml_presentation_id"],
		"scope":               out["scope"],
		"path":                resolvedPath,
		"size":                result.Size(),
		"content_saved":       true,
	}
	for _, key := range []string{"revision_id", "remove_attr_id", "slide_id", "slide_number"} {
		if value, ok := out[key]; ok {
			fileOut[key] = value
		}
	}
	runtime.Out(fileOut, nil)
	return nil
}
