// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var DrivePreview = common.Shortcut{
	Service:     "drive",
	Command:     "+preview",
	Description: "List or download available preview artifacts for a Drive file",
	Risk:        "read",
	Scopes:      []string{"drive:file:download"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "file-token", Desc: "Drive file token", Required: true},
		{Name: "type", Desc: "preview type to download: pdf | html | text | image | source"},
		{Name: "version", Desc: "optional file version"},
		{Name: "list-only", Type: "bool", Desc: "list preview candidates without downloading"},
		{Name: "output", Desc: "local output path for downloaded preview"},
		{Name: "if-exists", Desc: "output conflict policy: error | overwrite | rename", Default: drivePreviewIfExistsError, Enum: []string{drivePreviewIfExistsError, drivePreviewIfExistsOverwrite, drivePreviewIfExistsRename}},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validate.ResourceName(runtime.Str("file-token"), "--file-token"); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--file-token")
		}
		if err := validateDrivePreviewMode(runtime.Str("type"), runtime.Bool("list-only"), runtime.Str("output"), "type"); err != nil {
			return err
		}
		return validateDrivePreviewIfExists(runtime.Str("if-exists"))
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		fileToken := runtime.Str("file-token")
		version := strings.TrimSpace(runtime.Str("version"))
		body := map[string]interface{}{}
		if version != "" {
			body["version"] = version
		}
		dry := common.NewDryRunAPI().
			POST("/open-apis/drive/v1/medias/:file_token/preview_result").
			Desc("[1] Fetch preview candidates for a Drive file").
			Set("file_token", fileToken)
		if len(body) > 0 {
			dry.Body(body)
		}
		if runtime.Bool("list-only") {
			return dry.Set("mode", "list")
		}
		downloadParams := map[string]interface{}{
			"preview_type": "<selected type_code from preview_result>",
		}
		if version != "" {
			downloadParams["version"] = version
		} else {
			downloadParams["version"] = "<resolved version from preview_result>"
		}
		return dry.
			GET("/open-apis/drive/v1/medias/:file_token/preview_download").
			Desc("[2] Download the requested preview after selecting a matching candidate from preview_result").
			Params(downloadParams).
			Set("mode", "download").
			Set("requested_type", runtime.Str("type")).
			Set("output", runtime.Str("output"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		fileToken := runtime.Str("file-token")
		version := strings.TrimSpace(runtime.Str("version"))
		requestedType := strings.TrimSpace(runtime.Str("type"))
		outputPath := runtime.Str("output")
		ifExists := runtime.Str("if-exists")

		body := map[string]interface{}{}
		if version != "" {
			body["version"] = version
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Fetching preview candidates: %s\n", common.MaskToken(fileToken))
		data, candidates, err := fetchDrivePreviewCandidates(runtime, fileToken, body)
		if err != nil {
			return err
		}
		if runtime.Bool("list-only") {
			runtime.Out(buildDrivePreviewListOutput(fileToken, candidates), nil)
			return nil
		}

		candidate, ok := selectDrivePreviewCandidate(candidates, requestedType)
		if !ok {
			return wrapDrivePreviewUnavailable(fileToken, requestedType, candidates, "")
		}
		if !candidate.Downloadable {
			return wrapDrivePreviewNotReady(fileToken, requestedType, candidate)
		}

		downloadVersion := version
		if downloadVersion == "" {
			downloadVersion = versionString(data["version"])
		}
		fmt.Fprintf(runtime.IO().ErrOut, "Downloading preview %s for file %s\n", candidate.Type, common.MaskToken(fileToken))
		result, err := downloadDrivePreviewArtifact(ctx, runtime, fileToken, candidate.TypeCode, downloadVersion, outputPath, ifExists, drivePreviewFallbackExt(candidate.Type))
		if err != nil {
			return err
		}
		result["mode"] = "download"
		result["file_token"] = fileToken
		result["selected_type"] = candidate.Type
		runtime.Out(result, nil)
		return nil
	},
}
