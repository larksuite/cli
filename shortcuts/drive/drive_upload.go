// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	driveUploadParentTypeExplorer = "explorer"
	driveUploadParentTypeWiki     = "wiki"
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

func newDriveUploadSpec(runtime *common.RuntimeContext) driveUploadSpec {
	return driveUploadSpec{
		FilePath:    runtime.Str("file"),
		FileToken:   strings.TrimSpace(runtime.Str("file-token")),
		FolderToken: strings.TrimSpace(runtime.Str("folder-token")),
		WikiToken:   strings.TrimSpace(runtime.Str("wiki-token")),
		Name:        runtime.Str("name"),
	}
}

func (s driveUploadSpec) FileName() string {
	if s.Name != "" {
		return s.Name
	}
	return filepath.Base(s.FilePath)
}

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
	Scopes:      []string{"drive:file:upload"},
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
		"In bot mode, automatic full_access (可管理权限) grant only applies to newly uploaded files; overwrite via --file-token does not modify existing file permissions.",
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
			Desc("multipart/form-data upload (files > 20MB use chunked 3-step upload)").
			POST("/open-apis/drive/v1/files/upload_all").
			Body(body)
		if runtime.IsBot() && !isOverwrite {
			d.Desc("After file upload succeeds in bot mode, the CLI will also try to grant the current CLI user full_access (可管理权限) on the new file.")
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
			return common.WrapInputStatError(err)
		}
		fileSize := info.Size()

		fmt.Fprintf(runtime.IO().ErrOut, "Uploading: %s (%s) -> %s\n", fileName, common.FormatSize(fileSize), target.Label())

		var uploadResult driveUploadResult
		if fileSize > common.MaxDriveMediaUploadSinglePartSize {
			fmt.Fprintf(runtime.IO().ErrOut, "File exceeds 20MB, using multipart upload\n")
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
		// wiki-hosted files have no standalone /file/<token> URL — only the
		// wiki node URL, which the upload response doesn't carry. Skip the
		// fallback for parent_type=wiki rather than emit a link that 404s.
		if target.ParentType == driveUploadParentTypeExplorer {
			if u := common.BuildResourceURL(runtime.Config.Brand, "file", uploadResult.FileToken); u != "" {
				out["url"] = u
			}
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

func validateDriveUploadSpec(runtime *common.RuntimeContext, spec driveUploadSpec) error {
	if driveUploadFlagExplicitlyEmpty(runtime, "file-token") {
		return common.FlagErrorf("--file-token cannot be empty; omit --file-token for a new upload or pass an existing file token to overwrite")
	}
	if driveUploadFlagExplicitlyEmpty(runtime, "folder-token") {
		return common.FlagErrorf("--folder-token cannot be empty; omit --folder-token to upload into Drive root folder or pass a folder token")
	}
	if driveUploadFlagExplicitlyEmpty(runtime, "wiki-token") {
		return common.FlagErrorf("--wiki-token cannot be empty; omit --wiki-token to upload into Drive root folder or pass a wiki node token")
	}

	targets := 0
	if spec.FolderToken != "" {
		targets++
	}
	if spec.WikiToken != "" {
		targets++
	}
	if targets > 1 {
		return common.FlagErrorf("--folder-token and --wiki-token are mutually exclusive")
	}
	if spec.FolderToken != "" {
		if err := validate.ResourceName(spec.FolderToken, "--folder-token"); err != nil {
			return output.ErrValidation("%s", err)
		}
	}
	if spec.WikiToken != "" {
		if err := validate.ResourceName(spec.WikiToken, "--wiki-token"); err != nil {
			return output.ErrValidation("%s", err)
		}
	}
	if spec.FileToken != "" {
		if err := validate.ResourceName(spec.FileToken, "--file-token"); err != nil {
			return output.ErrValidation("%s", err)
		}
	}
	return nil
}

func driveUploadFlagExplicitlyEmpty(runtime *common.RuntimeContext, flagName string) bool {
	return runtime.Cmd != nil &&
		runtime.Cmd.Flags().Changed(flagName) &&
		strings.TrimSpace(runtime.Str(flagName)) == ""
}

func uploadFileToDrive(ctx context.Context, runtime *common.RuntimeContext, filePath, fileName string, target driveUploadTarget, fileSize int64, existingFileToken string) (driveUploadResult, error) {
	f, err := runtime.FileIO().Open(filePath)
	if err != nil {
		return driveUploadResult{}, common.WrapInputStatError(err)
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

	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodPost,
		ApiPath:    "/open-apis/drive/v1/files/upload_all",
		Body:       fd,
	}, larkcore.WithFileUpload())
	if err != nil {
		var exitErr *output.ExitError
		if errors.As(err, &exitErr) {
			return driveUploadResult{}, err
		}
		return driveUploadResult{}, output.ErrNetwork("upload failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(apiResp.RawBody, &result); err != nil {
		return driveUploadResult{}, output.Errorf(output.ExitAPI, "api_error", "upload failed: invalid response JSON: %v", err)
	}

	if larkCode := int(common.GetFloat(result, "code")); larkCode != 0 {
		msg, _ := result["msg"].(string)
		return driveUploadResult{}, output.ErrAPI(larkCode, fmt.Sprintf("upload failed: [%d] %s", larkCode, msg), result["error"])
	}

	data, _ := result["data"].(map[string]interface{})
	fileToken := common.GetString(data, "file_token")
	if fileToken == "" {
		return driveUploadResult{}, output.Errorf(output.ExitAPI, "api_error", "upload failed: no file_token returned")
	}
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
	prepareResult, err := runtime.CallAPI("POST", "/open-apis/drive/v1/files/upload_prepare", nil, prepareBody)
	if err != nil {
		return driveUploadResult{}, err
	}

	uploadID := common.GetString(prepareResult, "upload_id")
	blockSizeF := common.GetFloat(prepareResult, "block_size")
	blockNumF := common.GetFloat(prepareResult, "block_num")
	blockSize := int64(blockSizeF)
	blockNum := int(blockNumF)

	if uploadID == "" || blockSize <= 0 || blockNum <= 0 {
		return driveUploadResult{}, output.Errorf(output.ExitAPI, "api_error",
			"upload_prepare returned invalid data: upload_id=%q, block_size=%d, block_num=%d",
			uploadID, blockSize, blockNum)
	}

	fmt.Fprintf(runtime.IO().ErrOut, "Multipart upload: %s, block size %s, %d block(s)\n",
		common.FormatSize(fileSize), common.FormatSize(blockSize), blockNum)

	// Step 2: Upload parts
	for seq := 0; seq < blockNum; seq++ {
		offset := int64(seq) * blockSize
		partSize := blockSize
		if remaining := fileSize - offset; partSize > remaining {
			partSize = remaining
		}

		partFile, err := runtime.FileIO().Open(filePath)
		if err != nil {
			return driveUploadResult{}, common.WrapInputStatError(err)
		}

		fd := larkcore.NewFormdata()
		fd.AddField("upload_id", uploadID)
		fd.AddField("seq", fmt.Sprintf("%d", seq))
		fd.AddField("size", fmt.Sprintf("%d", partSize))
		fd.AddFile("file", io.NewSectionReader(partFile, offset, partSize))

		apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
			HttpMethod: http.MethodPost,
			ApiPath:    "/open-apis/drive/v1/files/upload_part",
			Body:       fd,
		}, larkcore.WithFileUpload())
		partFile.Close()
		if err != nil {
			var exitErr *output.ExitError
			if errors.As(err, &exitErr) {
				return driveUploadResult{}, err
			}
			return driveUploadResult{}, output.ErrNetwork("upload part %d/%d failed: %v", seq+1, blockNum, err)
		}

		var partResult map[string]interface{}
		if err := json.Unmarshal(apiResp.RawBody, &partResult); err != nil {
			return driveUploadResult{}, output.Errorf(output.ExitAPI, "api_error", "upload part %d/%d: invalid response JSON: %v", seq+1, blockNum, err)
		}
		if larkCode := int(common.GetFloat(partResult, "code")); larkCode != 0 {
			msg, _ := partResult["msg"].(string)
			return driveUploadResult{}, output.ErrAPI(larkCode, fmt.Sprintf("upload part %d/%d failed: [%d] %s", seq+1, blockNum, larkCode, msg), partResult["error"])
		}

		fmt.Fprintf(runtime.IO().ErrOut, "  Block %d/%d uploaded (%s)\n", seq+1, blockNum, common.FormatSize(partSize))
	}

	// Step 3: Finish
	finishBody := map[string]interface{}{
		"upload_id": uploadID,
		"block_num": blockNum,
	}
	finishResult, err := runtime.CallAPI("POST", "/open-apis/drive/v1/files/upload_finish", nil, finishBody)
	if err != nil {
		return driveUploadResult{}, err
	}

	fileToken := common.GetString(finishResult, "file_token")
	if fileToken == "" {
		return driveUploadResult{}, output.Errorf(output.ExitAPI, "api_error", "upload_finish succeeded but no file_token returned")
	}

	return driveUploadResult{
		FileToken: fileToken,
		Version:   driveUploadVersionFromData(finishResult),
	}, nil
}

func driveUploadVersionFromData(data map[string]interface{}) string {
	version := common.GetString(data, "version")
	if version == "" {
		version = common.GetString(data, "data_version")
	}
	return version
}
