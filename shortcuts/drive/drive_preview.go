// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

var DrivePreview = common.Shortcut{
	Service:     "drive",
	Command:     "+preview",
	Description: "View or download Drive file content, or list and fetch available preview artifacts",
	Risk:        "read",
	Scopes:      []string{"drive:file:download"},
	// Entity lookup is best-effort; Wiki permission is only required when
	// an explicit Wiki input falls back to the legacy node lookup.
	ConditionalScopes: []string{driveMetadataReadScope, driveWikiNodeRetrieveScope},
	AuthTypes:         []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "file-token", Desc: "Drive file token"},
		{Name: "url", Desc: "Drive file URL or Wiki node URL wrapping an uploaded file"},
		{Name: "wiki-token", Desc: "Wiki node token wrapping an uploaded file"},
		{Name: "type", Desc: "preview type to download: pdf | html | text | image | source_file"},
		{Name: "version", Desc: "optional file version"},
		{Name: "list-only", Type: "bool", Desc: "list preview candidates without downloading"},
		{Name: "output", Desc: "local output path for downloaded preview"},
		{Name: "if-exists", Desc: "output conflict policy: error | overwrite | rename", Default: drivePreviewIfExistsError, Enum: []string{drivePreviewIfExistsError, drivePreviewIfExistsOverwrite, drivePreviewIfExistsRename}},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := normalizeDriveFileSource(runtime.Str("file-token"), runtime.Str("url"), runtime.Str("wiki-token"))
		if err != nil {
			return err
		}
		if err := validateDrivePreviewMode(runtime.Str("type"), runtime.Bool("list-only"), runtime.Str("output"), "type"); err != nil {
			return err
		}
		return validateDrivePreviewIfExists(runtime.Str("if-exists"))
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		source, err := normalizeDriveFileSource(runtime.Str("file-token"), runtime.Str("url"), runtime.Str("wiki-token"))
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		dry := common.NewDryRunAPI()
		fileToken, step := addDriveFileSourceDryRun(dry, source)
		version := strings.TrimSpace(runtime.Str("version"))
		requestedType := strings.TrimSpace(runtime.Str("type"))

		if requestedType == "source_file" {
			downloadParams := map[string]interface{}{
				"preview_type": drivePreviewTypeSourceFile,
			}
			if version != "" {
				downloadParams["version"] = version
			}
			return dry.
				GET("/open-apis/drive/v1/medias/:file_token/preview_download").
				Desc(fmt.Sprintf("[%d] Download the source file artifact", step)).
				Params(downloadParams).
				Set("file_token", fileToken).
				Set("mode", "download").
				Set("requested_type", requestedType).
				Set("selected_type", "source_file").
				Set("selected_type_code", drivePreviewTypeSourceFile).
				Set("output", runtime.Str("output"))
		}
		body := map[string]interface{}{}
		if version != "" {
			body["version"] = version
		}
		dry.
			POST("/open-apis/drive/v1/medias/:file_token/preview_result").
			Desc(fmt.Sprintf("[%d] Fetch preview candidates for a Drive file", step)).
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
			Desc(fmt.Sprintf("[%d] Download the requested preview after selecting a matching candidate from preview_result", step+1)).
			Params(downloadParams).
			Set("mode", "download").
			Set("requested_type", requestedType).
			Set("output", runtime.Str("output"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		source, err := normalizeDriveFileSource(runtime.Str("file-token"), runtime.Str("url"), runtime.Str("wiki-token"))
		if err != nil {
			return err
		}
		fileToken, wikiResolution, err := resolveDriveFileSource(ctx, runtime, source)
		if err != nil {
			return err
		}
		version := strings.TrimSpace(runtime.Str("version"))
		requestedType := strings.TrimSpace(runtime.Str("type"))
		outputPath := runtime.Str("output")
		ifExists := runtime.Str("if-exists")

		body := map[string]interface{}{}
		if version != "" {
			body["version"] = version
		}

		if requestedType == "source_file" {
			result, err := downloadDrivePreviewArtifact(ctx, runtime, fileToken, drivePreviewTypeSourceFile, version, outputPath, ifExists, drivePreviewFallbackExt("source_file"))
			if err != nil {
				return err
			}
			result["mode"] = "download"
			result["file_token"] = fileToken
			result["selected_type"] = "source_file"
			runtime.Out(annotateDriveFileWikiOutput(result, wikiResolution), nil)
			return nil
		}

		data, candidates, err := fetchDrivePreviewCandidates(runtime, fileToken, body)
		if err != nil {
			if runtime.Bool("list-only") {
				return withDrivePreviewSourceFileHint(err)
			}
			return err
		}
		if runtime.Bool("list-only") {
			runtime.Out(annotateDriveFileWikiOutput(buildDrivePreviewListOutput(fileToken, candidates), wikiResolution), nil)
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
		result, err := downloadDrivePreviewArtifact(ctx, runtime, fileToken, candidate.TypeCode, downloadVersion, outputPath, ifExists, drivePreviewFallbackExt(candidate.Type))
		if err != nil {
			return err
		}
		result["mode"] = "download"
		result["file_token"] = fileToken
		result["selected_type"] = candidate.Type
		runtime.Out(annotateDriveFileWikiOutput(result, wikiResolution), nil)
		return nil
	},
}
