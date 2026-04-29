// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var driveVersionNumberRe = regexp.MustCompile(`^\d{1,19}$`)

type driveVersionHistorySpec struct {
	FileToken string
	Limit     int
	Cursor    string
}

func validateDriveVersionValue(value, flagName string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return output.ErrValidation("%s cannot be empty", flagName)
	}
	if !driveVersionNumberRe.MatchString(value) {
		return output.ErrValidation("%s must be a numeric version string", flagName)
	}
	return nil
}

func validateDriveVersionHistorySpec(spec driveVersionHistorySpec) error {
	if err := validate.ResourceName(spec.FileToken, "--file-token"); err != nil {
		return output.ErrValidation("%s", err)
	}
	if spec.Limit < 1 || spec.Limit > 200 {
		return output.ErrValidation("invalid --limit %d: must be between 1 and 200", spec.Limit)
	}
	if spec.Cursor != "" {
		if err := validateDriveVersionValue(spec.Cursor, "--cursor"); err != nil {
			return err
		}
	}
	return nil
}

func driveVersionHistoryParams(spec driveVersionHistorySpec) map[string]interface{} {
	params := map[string]interface{}{
		"only_tag":  true,
		"page_size": spec.Limit,
	}
	if spec.Cursor != "" {
		params["last_edit_time"] = spec.Cursor
	}
	return params
}

func driveVersionActionTypeLabel(raw int) string {
	switch raw {
	case 1:
		return "upload"
	case 2:
		return "rename"
	case 3:
		return "delete_version"
	case 4:
		return "revert"
	default:
		return fmt.Sprintf("type_%d", raw)
	}
}

func transformDriveVersionHistory(items []interface{}) []map[string]interface{} {
	versions := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		version := common.GetString(m, "version")
		if version == "" {
			continue
		}
		versions = append(versions, map[string]interface{}{
			"version":     version,
			"name":        common.GetString(m, "name"),
			"edited_at":   common.GetString(m, "edit_time"),
			"edited_by":   common.GetString(m, "edit_user_id"),
			"size_bytes":  int64(common.GetFloat(m, "size")),
			"action_type": driveVersionActionTypeLabel(int(common.GetFloat(m, "type"))),
			"tag":         int(common.GetFloat(m, "tag")),
		})
	}
	return versions
}

func nextDriveVersionCursor(items []interface{}, hasMore bool) string {
	if !hasMore || len(items) == 0 {
		return ""
	}
	last, _ := items[len(items)-1].(map[string]interface{})
	return common.GetString(last, "edit_time")
}

var DriveVersionHistory = common.Shortcut{
	Service:     "drive",
	Command:     "+version-history",
	Description: "List the version history of a Drive file",
	Risk:        "read",
	Scopes:      []string{"drive:file:download"},
	AuthTypes:   []string{"bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "file-token", Desc: "target file token", Required: true},
		{Name: "limit", Desc: "max versions to return (1-200)", Type: "int", Default: "20"},
		{Name: "cursor", Desc: "pagination cursor from the previous page's next_cursor"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateDriveVersionHistorySpec(driveVersionHistorySpec{
			FileToken: strings.TrimSpace(runtime.Str("file-token")),
			Limit:     runtime.Int("limit"),
			Cursor:    strings.TrimSpace(runtime.Str("cursor")),
		})
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec := driveVersionHistorySpec{
			FileToken: strings.TrimSpace(runtime.Str("file-token")),
			Limit:     runtime.Int("limit"),
			Cursor:    strings.TrimSpace(runtime.Str("cursor")),
		}
		return common.NewDryRunAPI().
			Desc("Query version history with only_tag=true and optional pagination cursor").
			GET("/open-apis/drive/v1/files/:file_token/history").
			Set("file_token", spec.FileToken).
			Params(driveVersionHistoryParams(spec))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec := driveVersionHistorySpec{
			FileToken: strings.TrimSpace(runtime.Str("file-token")),
			Limit:     runtime.Int("limit"),
			Cursor:    strings.TrimSpace(runtime.Str("cursor")),
		}

		data, err := runtime.CallAPI(
			http.MethodGet,
			fmt.Sprintf("/open-apis/drive/v1/files/%s/history", validate.EncodePathSegment(spec.FileToken)),
			driveVersionHistoryParams(spec),
			nil,
		)
		if err != nil {
			return err
		}

		items := common.GetSlice(data, "items")
		hasMore := common.GetBool(data, "has_more")
		out := map[string]interface{}{
			"versions": transformDriveVersionHistory(items),
			"has_more": hasMore,
		}
		if nextCursor := nextDriveVersionCursor(items, hasMore); nextCursor != "" {
			out["next_cursor"] = nextCursor
		}

		runtime.OutFormat(out, nil, nil)
		return nil
	},
}

type driveVersionGetSpec struct {
	FileToken string
	Version   string
	Output    string
	Overwrite bool
}

func validateDriveVersionGetSpec(runtime *common.RuntimeContext, spec driveVersionGetSpec) error {
	if err := validate.ResourceName(spec.FileToken, "--file-token"); err != nil {
		return output.ErrValidation("%s", err)
	}
	if err := validateDriveVersionValue(spec.Version, "--version"); err != nil {
		return err
	}
	if spec.Output == "" {
		return nil
	}
	if _, err := validate.SafeOutputPath(spec.Output); err != nil {
		return output.ErrValidation("unsafe output path: %s", err)
	}
	if _, statErr := runtime.FileIO().Stat(spec.Output); statErr == nil && !spec.Overwrite {
		return output.ErrValidation("output file already exists: %s (use --overwrite to replace)", spec.Output)
	}
	return nil
}

var DriveVersionGet = common.Shortcut{
	Service:     "drive",
	Command:     "+version-get",
	Description: "Download a specific version of a Drive file",
	Risk:        "read",
	Scopes:      []string{"drive:file:download"},
	AuthTypes:   []string{"bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "file-token", Desc: "target file token", Required: true},
		{Name: "version", Desc: "target version number", Required: true},
		{Name: "output", Desc: "local save path; omit to use the same default behavior as drive +download"},
		{Name: "overwrite", Type: "bool", Desc: "overwrite existing output file"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateDriveVersionGetSpec(runtime, driveVersionGetSpec{
			FileToken: strings.TrimSpace(runtime.Str("file-token")),
			Version:   strings.TrimSpace(runtime.Str("version")),
			Output:    strings.TrimSpace(runtime.Str("output")),
			Overwrite: runtime.Bool("overwrite"),
		})
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec := driveVersionGetSpec{
			FileToken: strings.TrimSpace(runtime.Str("file-token")),
			Version:   strings.TrimSpace(runtime.Str("version")),
			Output:    strings.TrimSpace(runtime.Str("output")),
		}
		outputPath := spec.Output
		if outputPath == "" {
			outputPath = spec.FileToken
		}
		return common.NewDryRunAPI().
			Desc("Download a specific file version to local storage").
			GET("/open-apis/drive/v1/files/:file_token/download").
			Set("file_token", spec.FileToken).
			Set("output", outputPath).
			Params(map[string]interface{}{"version": spec.Version})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec := driveVersionGetSpec{
			FileToken: strings.TrimSpace(runtime.Str("file-token")),
			Version:   strings.TrimSpace(runtime.Str("version")),
			Output:    strings.TrimSpace(runtime.Str("output")),
			Overwrite: runtime.Bool("overwrite"),
		}

		outputPath := spec.Output
		if outputPath == "" {
			outputPath = spec.FileToken
		}

		if _, resolveErr := runtime.ResolveSavePath(outputPath); resolveErr != nil {
			return output.ErrValidation("unsafe output path: %s", resolveErr)
		}
		if _, statErr := runtime.FileIO().Stat(outputPath); statErr == nil && !spec.Overwrite {
			return output.ErrValidation("output file already exists: %s (use --overwrite to replace)", outputPath)
		}

		resp, err := runtime.DoAPIStream(ctx, &larkcore.ApiReq{
			HttpMethod: http.MethodGet,
			ApiPath:    fmt.Sprintf("/open-apis/drive/v1/files/%s/download", validate.EncodePathSegment(spec.FileToken)),
			QueryParams: larkcore.QueryParams{
				"version": []string{spec.Version},
			},
		})
		if err != nil {
			return output.ErrNetwork("download failed: %s", err)
		}
		defer resp.Body.Close()

		result, err := runtime.FileIO().Save(outputPath, fileio.SaveOptions{
			ContentType:   resp.Header.Get("Content-Type"),
			ContentLength: resp.ContentLength,
		}, resp.Body)
		if err != nil {
			return common.WrapSaveErrorByCategory(err, "io")
		}

		savedPath, _ := runtime.ResolveSavePath(outputPath)
		if savedPath == "" {
			savedPath = outputPath
		}
		out := map[string]interface{}{
			"file_token": spec.FileToken,
			"version":    spec.Version,
			"file_name":  filepath.Base(savedPath),
			"saved_path": savedPath,
			"size_bytes": result.Size(),
		}
		runtime.OutFormat(out, nil, nil)
		return nil
	},
}

type driveVersionMutationSpec struct {
	FileToken string
	Version   string
}

func validateDriveVersionMutationSpec(spec driveVersionMutationSpec) error {
	if err := validate.ResourceName(spec.FileToken, "--file-token"); err != nil {
		return output.ErrValidation("%s", err)
	}
	return validateDriveVersionValue(spec.Version, "--version")
}

var DriveVersionRevert = common.Shortcut{
	Service:     "drive",
	Command:     "+version-revert",
	Description: "Revert a Drive file to a specific historical version",
	Risk:        "write",
	Scopes:      []string{"drive:file:upload"},
	AuthTypes:   []string{"bot"},
	Flags: []common.Flag{
		{Name: "file-token", Desc: "target file token", Required: true},
		{Name: "version", Desc: "version number to revert to", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateDriveVersionMutationSpec(driveVersionMutationSpec{
			FileToken: strings.TrimSpace(runtime.Str("file-token")),
			Version:   strings.TrimSpace(runtime.Str("version")),
		})
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec := driveVersionMutationSpec{
			FileToken: strings.TrimSpace(runtime.Str("file-token")),
			Version:   strings.TrimSpace(runtime.Str("version")),
		}
		return common.NewDryRunAPI().
			Desc("Revert the current file to a specified historical version").
			POST("/open-apis/drive/v1/files/:file_token/revert").
			Set("file_token", spec.FileToken).
			Body(map[string]interface{}{"version": spec.Version})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec := driveVersionMutationSpec{
			FileToken: strings.TrimSpace(runtime.Str("file-token")),
			Version:   strings.TrimSpace(runtime.Str("version")),
		}
		if _, err := runtime.CallAPI(
			http.MethodPost,
			fmt.Sprintf("/open-apis/drive/v1/files/%s/revert", validate.EncodePathSegment(spec.FileToken)),
			nil,
			map[string]interface{}{"version": spec.Version},
		); err != nil {
			return err
		}

		runtime.Out(map[string]interface{}{}, nil)
		return nil
	},
}

var DriveVersionDelete = common.Shortcut{
	Service:     "drive",
	Command:     "+version-delete",
	Description: "Delete a specific historical version of a Drive file",
	Risk:        "write",
	Scopes:      []string{"drive:file:upload"},
	AuthTypes:   []string{"bot"},
	Flags: []common.Flag{
		{Name: "file-token", Desc: "target file token", Required: true},
		{Name: "version", Desc: "version number to delete", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateDriveVersionMutationSpec(driveVersionMutationSpec{
			FileToken: strings.TrimSpace(runtime.Str("file-token")),
			Version:   strings.TrimSpace(runtime.Str("version")),
		})
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec := driveVersionMutationSpec{
			FileToken: strings.TrimSpace(runtime.Str("file-token")),
			Version:   strings.TrimSpace(runtime.Str("version")),
		}
		return common.NewDryRunAPI().
			Desc("Permanently delete a historical file version").
			POST("/open-apis/drive/v1/files/:file_token/version_del").
			Set("file_token", spec.FileToken).
			Body(map[string]interface{}{"version": spec.Version})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec := driveVersionMutationSpec{
			FileToken: strings.TrimSpace(runtime.Str("file-token")),
			Version:   strings.TrimSpace(runtime.Str("version")),
		}
		if _, err := runtime.CallAPI(
			http.MethodPost,
			fmt.Sprintf("/open-apis/drive/v1/files/%s/version_del", validate.EncodePathSegment(spec.FileToken)),
			nil,
			map[string]interface{}{"version": spec.Version},
		); err != nil {
			return err
		}

		runtime.Out(map[string]interface{}{}, nil)
		return nil
	},
}
