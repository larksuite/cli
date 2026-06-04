// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var DriveCover = common.Shortcut{
	Service:     "drive",
	Command:     "+cover",
	Description: "List or download stable cover presets for a Drive file",
	Risk:        "read",
	Scopes:      []string{"drive:file:download"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "file-token", Desc: "Drive file token", Required: true},
		{Name: "spec", Desc: "cover preset: default | icon | grid | small | middle | big | square"},
		{Name: "version", Desc: "optional file version"},
		{Name: "list-only", Type: "bool", Desc: "list built-in cover specs without downloading"},
		{Name: "output", Desc: "local output path for downloaded cover"},
		{Name: "if-exists", Desc: "output conflict policy: error | overwrite | rename", Default: drivePreviewIfExistsError, Enum: []string{drivePreviewIfExistsError, drivePreviewIfExistsOverwrite, drivePreviewIfExistsRename}},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validate.ResourceName(runtime.Str("file-token"), "--file-token"); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--file-token")
		}
		if err := validateDrivePreviewMode(runtime.Str("spec"), runtime.Bool("list-only"), runtime.Str("output"), "spec"); err != nil {
			return err
		}
		if err := validateDrivePreviewIfExists(runtime.Str("if-exists")); err != nil {
			return err
		}
		if spec := strings.TrimSpace(runtime.Str("spec")); spec != "" {
			if _, ok := findDriveCoverSpec(spec); !ok {
				return wrapDriveCoverUnavailable(spec)
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		fileToken := runtime.Str("file-token")
		if runtime.Bool("list-only") {
			return common.NewDryRunAPI().
				Desc("List built-in cover specs (no API call)").
				Set("mode", "list").
				Set("file_token", fileToken).
				Set("candidates", buildDriveCoverListOutput(fileToken)["candidates"])
		}

		spec, _ := findDriveCoverSpec(runtime.Str("spec"))
		params := buildDriveCoverDownloadParams(strings.TrimSpace(runtime.Str("version")), spec)
		dry := common.NewDryRunAPI().
			GET("/open-apis/drive/v1/medias/:file_token/preview_download").
			Desc("Download selected cover preset directly via preview_download").
			Params(params).
			Set("file_token", fileToken).
			Set("selected_spec", spec.Name).
			Set("output", runtime.Str("output"))
		return dry
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		fileToken := runtime.Str("file-token")
		version := strings.TrimSpace(runtime.Str("version"))
		requestedSpec := strings.TrimSpace(runtime.Str("spec"))
		outputPath := runtime.Str("output")
		ifExists := runtime.Str("if-exists")

		if runtime.Bool("list-only") {
			runtime.Out(buildDriveCoverListOutput(fileToken), nil)
			return nil
		}

		spec, ok := findDriveCoverSpec(requestedSpec)
		if !ok {
			return wrapDriveCoverUnavailable(requestedSpec)
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Downloading cover %s for file %s\n", spec.Name, common.MaskToken(fileToken))
		result, err := downloadDrivePreviewArtifactWithParams(ctx, runtime, fileToken, buildDriveCoverDownloadParams(version, spec), outputPath, ifExists, spec.FallbackExt)
		if err != nil {
			return wrapDriveCoverDownloadError(err, spec.Name)
		}
		result["mode"] = "download"
		result["file_token"] = fileToken
		result["selected_spec"] = spec.Name
		runtime.Out(result, nil)
		return nil
	},
}

// wrapDriveCoverDownloadError reclassifies preview_download HTTP 404 responses
// on the +cover path as a failed precondition on --spec, because the Drive
// shortcut contract documents 404 as "this file has no artifact for that cover
// preset" rather than a transient transport failure.
func wrapDriveCoverDownloadError(err error, requestedSpec string) error {
	if err == nil {
		return nil
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Code != http.StatusNotFound {
		return err
	}
	hint := fmt.Sprintf(
		"This may mean no artifact exists for --spec %q, or that the file token/version is invalid. Verify the inputs, or rerun with `lark-cli drive +cover --file-token <file-token> --list-only`. Available cover specs: %s",
		requestedSpec,
		strings.Join(availableDriveCoverSpecs(), ", "),
	)
	return errs.NewValidationError(
		errs.SubtypeFailedPrecondition,
		"preview_download returned HTTP 404 for --spec %q",
		requestedSpec,
	).WithParam("--spec").WithCode(problem.Code).WithLogID(problem.LogID).WithHint(hint).WithCause(err)
}
