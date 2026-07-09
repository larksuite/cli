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

// SlidesXMLGet fetches the full XML presentation content and writes it to a
// local file, keeping the terminal output small for large decks.
var SlidesXMLGet = common.Shortcut{
	Service:     "slides",
	Command:     "+xml-get",
	Description: "Fetch full presentation XML and save it to a local file",
	Risk:        "read",
	Scopes:      []string{"slides:presentation:read"},
	// wiki:node:read is required only when --presentation is a wiki URL.
	ConditionalScopes: []string{"wiki:node:read"},
	AuthTypes:         []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "presentation", Desc: "xml_presentation_id, slides URL, or wiki URL that resolves to slides", Required: true},
		{Name: "output", Desc: "local XML output path; existing file is overwritten", Required: true},
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
		if strings.TrimSpace(runtime.Str("output")) == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--output cannot be empty").WithParam("--output")
		}
		if _, err := runtime.ResolveSavePath(runtime.Str("output")); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--output invalid: %v", err).WithParam("--output").WithCause(err)
		}
		if runtime.Int("revision-id") < -1 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--revision-id must be -1 or a non-negative integer").WithParam("--revision-id")
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
			dry.Desc("Fetch full presentation XML and save it to a local file")
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
		return dry.Set("output", runtime.Str("output")).Set("stdout_content", "suppressed; XML content is saved to --output during execution")
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
		outputPath := runtime.Str("output")
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
