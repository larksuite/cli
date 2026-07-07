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
// standard JSON envelope. Use --raw for direct XML stdout.
var SlidesXMLGet = common.Shortcut{
	Service:     "slides",
	Command:     "+xml-get",
	Description: "Fetch full presentation XML",
	Risk:        "read",
	Scopes:      []string{"slides:presentation:read"},
	// wiki:node:read is required only when --presentation is a wiki URL.
	ConditionalScopes: []string{"wiki:node:read"},
	AuthTypes:         []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "presentation", Desc: "xml_presentation_id, slides URL, or wiki URL that resolves to slides", Required: true},
		{Name: "output", Desc: "local XML output path; existing file is overwritten; omit to return XML in the JSON envelope"},
		{Name: "raw", Type: "bool", Desc: "print raw XML to stdout instead of the JSON envelope; incompatible with --output and --jq"},
		{Name: "revision-id", Type: "int", Default: "-1", Desc: "presentation revision_id; -1 means latest"},
		{Name: "remove-attr-id", Type: "bool", Desc: "remove XML id attributes in the returned content; useful for read-only inspection, not precise block editing"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		ref, err := parsePresentationRef(runtime.Str("presentation"))
		if err != nil {
			return err
		}
		if ref.Kind == "wiki" {
			if err := runtime.EnsureScopes([]string{"wiki:node:read"}); err != nil {
				return err
			}
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
			dry.Desc("2-step orchestration: resolve wiki → fetch full presentation XML").
				GET("/open-apis/wiki/v2/spaces/get_node").
				Desc("[1] Resolve wiki node to slides presentation").
				Params(map[string]interface{}{"token": ref.Token})
		} else {
			dry.Desc("Fetch full presentation XML")
		}
		params := map[string]interface{}{
			"revision_id": runtime.Int("revision-id"),
		}
		if runtime.Bool("remove-attr-id") {
			params["remove_attr_id"] = true
		}
		dry.GET(fmt.Sprintf(
			"/open-apis/slides_ai/v1/xml_presentations/%s",
			validate.EncodePathSegment(presentationID),
		)).
			Params(params)
		if outputPath := strings.TrimSpace(runtime.Str("output")); outputPath != "" {
			return dry.Set("output", outputPath).Set("stdout_content", "suppressed; XML content is saved to --output during execution")
		}
		if runtime.Bool("raw") {
			return dry.Set("output", "<stdout>").Set("stdout_content", "raw XML content is printed to stdout during execution")
		}
		return dry.Set("output", "<stdout>").Set("stdout_content", "JSON envelope with xml_presentation.content is printed to stdout during execution")
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

		params := map[string]interface{}{
			"revision_id": runtime.Int("revision-id"),
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
			return err
		}

		presentation := common.GetMap(data, "xml_presentation")
		content := common.GetString(presentation, "content")
		if content == "" {
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "slides xml get returned empty xml_presentation.content")
		}
		outputPath := strings.TrimSpace(runtime.Str("output"))
		if outputPath == "" {
			presentationOut := map[string]interface{}{
				"content": content,
			}
			out := map[string]interface{}{
				"xml_presentation_id": presentationID,
				"xml_presentation":    presentationOut,
			}
			if revisionID := common.GetFloat(presentation, "revision_id"); revisionID > 0 {
				out["revision_id"] = int(revisionID)
				presentationOut["revision_id"] = int(revisionID)
			}
			if runtime.Bool("remove-attr-id") {
				out["remove_attr_id"] = true
			}
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

		out := map[string]interface{}{
			"xml_presentation_id": presentationID,
			"path":                resolvedPath,
			"size":                result.Size(),
			"content_saved":       true,
		}
		if revisionID := common.GetFloat(presentation, "revision_id"); revisionID > 0 {
			out["revision_id"] = int(revisionID)
		}
		if runtime.Bool("remove-attr-id") {
			out["remove_attr_id"] = true
		}
		runtime.Out(out, nil)
		return nil
	},
}
