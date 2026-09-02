// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/fileevent"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	driveUploadParentTypeExplorer = "explorer"
	driveUploadParentTypeWiki     = "wiki"
)

const (
	driveUploadAllPath     = "/open-apis/drive/v1/files/upload_all"
	driveUploadPreparePath = "/open-apis/drive/v1/files/upload_prepare"
	driveUploadPartPath    = "/open-apis/drive/v1/files/upload_part"
	driveUploadFinishPath  = "/open-apis/drive/v1/files/upload_finish"
)

type driveUploadSpec struct {
	FilePath    string
	FileToken   string
	FolderToken string
	WikiToken   string
	Name        string
}

type driveUploadTarget struct {
	ParentType string
	ParentNode string
}

type driveUploadResult struct {
	FileToken string
	Version   string
}

// newDriveUploadSpec captures upload flags in the internal execution model.
func newDriveUploadSpec(runtime *common.RuntimeContext) driveUploadSpec {
	return driveUploadSpec{
		FilePath:    runtime.Str("file"),
		FileToken:   strings.TrimSpace(runtime.Str("file-token")),
		FolderToken: strings.TrimSpace(runtime.Str("folder-token")),
		WikiToken:   strings.TrimSpace(runtime.Str("wiki-token")),
		Name:        runtime.Str("name"),
	}
}

// FileName returns the explicit remote name or derives it from the local path.
func (s driveUploadSpec) FileName() string {
	if s.Name != "" {
		return s.Name
	}
	return filepath.Base(s.FilePath)
}

// Target returns the Drive or Wiki destination represented by the upload flags.
func (s driveUploadSpec) Target() driveUploadTarget {
	if s.WikiToken != "" {
		return driveUploadTarget{
			ParentType: driveUploadParentTypeWiki,
			ParentNode: s.WikiToken,
		}
	}
	return driveUploadTarget{
		ParentType: driveUploadParentTypeExplorer,
		ParentNode: s.FolderToken,
	}
}

// Label returns a human-readable description of the upload destination.
func (t driveUploadTarget) Label() string {
	switch t.ParentType {
	case driveUploadParentTypeWiki:
		return "wiki node " + common.MaskToken(t.ParentNode)
	case driveUploadParentTypeExplorer:
		if t.ParentNode == "" {
			return "Drive root folder"
		}
		return "folder " + common.MaskToken(t.ParentNode)
	default:
		return "target " + common.MaskToken(t.ParentNode)
	}
}

var DriveUpload = common.Shortcut{
	Service:     "drive",
	Command:     "+upload",
	Description: "Upload a local file to Drive",
	Risk:        "write",
	Scopes:      []string{"drive:file:upload", "drive:drive.metadata:readonly"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "file", Desc: "local file path (files > 20MB use multipart upload automatically)", Required: true},
		{Name: "file-token", Desc: "existing file token to overwrite in place"},
		{Name: "folder-token", Desc: "target folder token (default: root folder; mutually exclusive with --wiki-token)"},
		{Name: "wiki-token", Desc: "target wiki node token (uploads under that wiki node; mutually exclusive with --folder-token)"},
		{Name: "name", Desc: "uploaded file name (default: local file name)"},
	},
	Tips: []string{
		"Omit both --folder-token and --wiki-token to upload into the caller's Drive root folder.",
		"Use --wiki-token <wiki_node_token> to upload under a wiki node; the shortcut maps this to parent_type=wiki automatically.",
		"Pass --file-token <file_token> to overwrite an existing Drive file in place; the shortcut forwards file_token to the upload API.",
		"In bot mode, automatic full_access grant only applies to newly uploaded files; overwrite via --file-token does not modify existing file permissions.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateDriveUploadSpec(runtime, newDriveUploadSpec(runtime))
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec := newDriveUploadSpec(runtime)
		target := spec.Target()
		isOverwrite := spec.FileToken != ""
		body := map[string]interface{}{
			"file_name":   spec.FileName(),
			"parent_type": target.ParentType,
			"parent_node": target.ParentNode,
			"file":        "@" + spec.FilePath,
		}
		if spec.FileToken != "" {
			body["file_token"] = spec.FileToken
		}
		d := common.NewDryRunAPI().
			Desc("multipart/form-data upload (files > 20MB use chunked 3-step upload), then fetch the real Drive URL via metadata").
			POST(driveUploadAllPath).
			Body(body)
		fileevent.AppendUploadDryRun(d, runtime, fileevent.UploadMeta{
			APIPath:      driveUploadAllPath,
			ResourceType: "file",
			ParentType:   target.ParentType,
			FileToken:    "<file_token from upload response>",
		})
		d.POST("/open-apis/drive/v1/metas/batch_query").
			Desc("Fetch the uploaded file's real access URL").
			Body(map[string]interface{}{
				"request_docs": []map[string]interface{}{
					{
						"doc_token": "<file_token from upload response>",
						"doc_type":  "file",
					},
				},
				"with_url": true,
			})
		if runtime.IsBot() && !isOverwrite {
			d.Set("post_upload_note", "After file upload succeeds in bot mode, the CLI will also try to grant the current CLI user full_access on the new file.")
		}
		return d
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec := newDriveUploadSpec(runtime)
		isOverwrite := spec.FileToken != ""
		fileName := spec.FileName()
		target := spec.Target()

		info, err := runtime.FileIO().Stat(spec.FilePath)
		if err != nil {
			return driveInputStatError(err)
		}
		fileSize := info.Size()

		var uploadResult driveUploadResult
		if fileSize > common.MaxDriveMediaUploadSinglePartSize {
			uploadResult, err = uploadFileMultipart(ctx, runtime, spec.FilePath, fileName, target, fileSize, spec.FileToken)
		} else {
			uploadResult, err = uploadFileToDrive(ctx, runtime, spec.FilePath, fileName, target, fileSize, spec.FileToken)
		}
		if err != nil {
			return err
		}

		out := map[string]interface{}{
			"file_token": uploadResult.FileToken,
			"file_name":  fileName,
			"size":       fileSize,
		}
		if uploadResult.Version != "" {
			out["version"] = uploadResult.Version
		}
		if u, metaErr := common.FetchDriveMetaURL(runtime, uploadResult.FileToken, "file"); metaErr == nil && strings.TrimSpace(u) != "" {
			out["url"] = u
		} else if metaErr != nil {
			fmt.Fprintf(runtime.IO().ErrOut, "warning: uploaded file URL lookup failed: %v\n", metaErr)
		}
		if !isOverwrite {
			if grant := common.AutoGrantCurrentUserDrivePermission(runtime, uploadResult.FileToken, "file"); grant != nil {
				out["permission_grant"] = grant
			}
		}

		runtime.Out(out, nil)
		return nil
	},
}

// validateDriveUploadSpec validates local input and mutually exclusive upload targets.
func validateDriveUploadSpec(runtime *common.RuntimeContext, spec driveUploadSpec) error {
	if driveUploadFlagExplicitlyEmpty(runtime, "file-token") {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--file-token cannot be empty; omit --file-token for a new upload or pass an existing file token to overwrite").WithParam("--file-token")
	}
	if driveUploadFlagExplicitlyEmpty(runtime, "folder-token") {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--folder-token cannot be empty; omit --folder-token to upload into Drive root folder or pass a folder token").WithParam("--folder-token")
	}
	if driveUploadFlagExplicitlyEmpty(runtime, "wiki-token") {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--wiki-token cannot be empty; omit --wiki-token to upload into Drive root folder or pass a wiki node token").WithParam("--wiki-token")
	}

	targets := 0
	if spec.FolderToken != "" {
		targets++
	}
	if spec.WikiToken != "" {
		targets++
	}
	if targets > 1 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--folder-token and --wiki-token are mutually exclusive")
	}
	if spec.FolderToken != "" {
		if err := validate.ResourceName(spec.FolderToken, "--folder-token"); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--folder-token")
		}
	}
	if spec.WikiToken != "" {
		if err := validate.ResourceName(spec.WikiToken, "--wiki-token"); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--wiki-token")
		}
	}
	if spec.FileToken != "" {
		if err := validate.ResourceName(spec.FileToken, "--file-token"); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--file-token")
		}
	}
	return nil
}

// driveUploadFlagExplicitlyEmpty reports whether a target flag was supplied with an empty value.
func driveUploadFlagExplicitlyEmpty(runtime *common.RuntimeContext, flagName string) bool {
	return runtime.Cmd != nil &&
		runtime.Cmd.Flags().Changed(flagName) &&
		strings.TrimSpace(runtime.Str(flagName)) == ""
}

// uploadFileToDrive selects the appropriate upload mode and reports the resulting file event.
func uploadFileToDrive(ctx context.Context, runtime *common.RuntimeContext, filePath, fileName string, target driveUploadTarget, fileSize int64, existingFileToken string) (driveUploadResult, error) {
	f, err := runtime.FileIO().Open(filePath)
	if err != nil {
		return driveUploadResult{}, driveInputStatError(err)
	}
	defer f.Close()

	// Build SDK Formdata
	fd := larkcore.NewFormdata()
	fd.AddField("file_name", fileName)
	fd.AddField("parent_type", target.ParentType)
	fd.AddField("parent_node", target.ParentNode)
	fd.AddField("size", fmt.Sprintf("%d", fileSize))
	if existingFileToken != "" {
		fd.AddField("file_token", existingFileToken)
	}
	fd.AddFile("file", f)

	meta := fileevent.UploadMeta{
		APIPath:      driveUploadAllPath,
		ResourceType: "file",
		ParentType:   target.ParentType,
	}

	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodPost,
		ApiPath:    driveUploadAllPath,
		Body:       fd,
	}, larkcore.WithFileUpload())
	if err != nil {
		if errs.IsTyped(err) {
			return driveUploadResult{}, fileevent.ReportUploadError(runtime, err, meta)
		}
		return driveUploadResult{}, fileevent.ReportUploadError(runtime, wrapDriveNetworkErr(err, "upload failed: %v", err), meta)
	}

	data, err := runtime.ClassifyAPIResponse(apiResp)
	if err != nil {
		return driveUploadResult{}, fileevent.ReportUploadError(runtime, err, meta)
	}
	fileToken := common.GetString(data, "file_token")
	if fileToken == "" {
		return driveUploadResult{}, fileevent.ReportUploadError(runtime, errs.NewInternalError(errs.SubtypeInvalidResponse, "upload failed: no file_token returned"), meta)
	}
	meta.FileToken = fileToken
	fileevent.ReportUpload(runtime, meta)
	return driveUploadResult{
		FileToken: fileToken,
		Version:   driveUploadVersionFromData(data),
	}, nil
}

// uploadFileMultipart uploads a large file using the three-step multipart API:
// 1. upload_prepare — get upload_id, block_size, block_num
// 2. upload_part   — upload each block sequentially
// 3. upload_finish — finalize and get file_token/version
func uploadFileMultipart(_ context.Context, runtime *common.RuntimeContext, filePath, fileName string, target driveUploadTarget, fileSize int64, existingFileToken string) (driveUploadResult, error) {
	// Step 1: Prepare
	prepareBody := map[string]interface{}{
		"file_name":   fileName,
		"parent_type": target.ParentType,
		"parent_node": target.ParentNode,
		"size":        fileSize,
	}
	if existingFileToken != "" {
		prepareBody["file_token"] = existingFileToken
	}

	meta := fileevent.UploadMeta{
		APIPath:      driveUploadPreparePath,
		ResourceType: "file",
		ParentType:   target.ParentType,
	}

	prepareResult, err := runtime.CallAPITyped("POST", driveUploadPreparePath, nil, prepareBody)
	if err != nil {
		return driveUploadResult{}, fileevent.ReportUploadError(runtime, err, meta)
	}

	uploadID := common.GetString(prepareResult, "upload_id")
	blockSizeF := common.GetFloat(prepareResult, "block_size")
	blockNumF := common.GetFloat(prepareResult, "block_num")
	blockSize := int64(blockSizeF)
	blockNum := int(blockNumF)

	if uploadID == "" || blockSize <= 0 || blockNum <= 0 {
		return driveUploadResult{}, fileevent.ReportUploadError(runtime, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"upload_prepare returned invalid data: upload_id=%q, block_size=%d, block_num=%d",
			uploadID, blockSize, blockNum), meta)
	}
	// Step 2: Upload parts
	meta.APIPath = driveUploadPartPath
	for seq := 0; seq < blockNum; seq++ {
		offset := int64(seq) * blockSize
		partSize := blockSize
		if remaining := fileSize - offset; partSize > remaining {
			partSize = remaining
		}

		partFile, err := runtime.FileIO().Open(filePath)
		if err != nil {
			return driveUploadResult{}, fileevent.ReportUploadError(runtime, driveInputStatError(err), meta)
		}

		fd := larkcore.NewFormdata()
		fd.AddField("upload_id", uploadID)
		fd.AddField("seq", fmt.Sprintf("%d", seq))
		fd.AddField("size", fmt.Sprintf("%d", partSize))
		fd.AddFile("file", io.NewSectionReader(partFile, offset, partSize))

		apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
			HttpMethod: http.MethodPost,
			ApiPath:    driveUploadPartPath,
			Body:       fd,
		}, larkcore.WithFileUpload())
		partFile.Close()
		if err != nil {
			if errs.IsTyped(err) {
				return driveUploadResult{}, fileevent.ReportUploadError(runtime, err, meta)
			}
			return driveUploadResult{}, fileevent.ReportUploadError(runtime, wrapDriveNetworkErr(err, "upload part %d/%d failed: %v", seq+1, blockNum, err), meta)
		}

		if _, err := runtime.ClassifyAPIResponse(apiResp); err != nil {
			return driveUploadResult{}, fileevent.ReportUploadError(runtime, err, meta)
		}

	}

	// Step 3: Finish
	meta.APIPath = driveUploadFinishPath
	finishBody := map[string]interface{}{
		"upload_id": uploadID,
		"block_num": blockNum,
	}
	finishResult, err := runtime.CallAPITyped("POST", driveUploadFinishPath, nil, finishBody)
	if err != nil {
		return driveUploadResult{}, fileevent.ReportUploadError(runtime, err, meta)
	}

	fileToken := common.GetString(finishResult, "file_token")
	if fileToken == "" {
		return driveUploadResult{}, fileevent.ReportUploadError(runtime, errs.NewInternalError(errs.SubtypeInvalidResponse, "upload_finish succeeded but no file_token returned"), meta)
	}

	meta.FileToken = fileToken
	fileevent.ReportUpload(runtime, meta)
	return driveUploadResult{
		FileToken: fileToken,
		Version:   driveUploadVersionFromData(finishResult),
	}, nil
}

// driveUploadVersionFromData extracts the upload version from supported response aliases.
func driveUploadVersionFromData(data map[string]interface{}) string {
	version := common.GetString(data, "version")
	if version == "" {
		version = common.GetString(data, "data_version")
	}
	return version
}
