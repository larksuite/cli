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

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
	"github.com/larksuite/cli/shortcuts/common"
)

var DriveUpload = common.Shortcut{
	Service:     "drive",
	Command:     "+upload",
	Description: "Upload a local file to Drive",
	Risk:        "write",
	Scopes:      []string{"drive:file:upload"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "file", Desc: "local file path (files > 20MB use multipart upload automatically)", Required: true},
		{Name: "folder-token", Desc: "target folder token (default: root)"},
		{Name: "name", Desc: "uploaded file name (default: local file name)"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		filePath := runtime.Str("file")
		folderToken := runtime.Str("folder-token")
		name := runtime.Str("name")
		fileName := name
		if fileName == "" {
			fileName = filepath.Base(filePath)
		}
		return common.NewDryRunAPI().
			Desc("multipart/form-data upload (files > 20MB use chunked 3-step upload)").
			POST("/open-apis/drive/v1/files/upload_all").
			Body(map[string]interface{}{
				"file_name":   fileName,
				"parent_type": "explorer",
				"parent_node": folderToken,
				"file":        "@" + filePath,
			})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		filePath := runtime.Str("file")
		folderToken := runtime.Str("folder-token")
		name := runtime.Str("name")

		safeFilePath, err := validate.SafeInputPath(filePath)
		if err != nil {
			return output.ErrValidation("unsafe file path: %s", err)
		}
		filePath = safeFilePath

		fileName := name
		if fileName == "" {
			fileName = filepath.Base(filePath)
		}

		info, err := vfs.Stat(filePath)
		if err != nil {
			return output.ErrValidation("cannot read file: %s", err)
		}
		fileSize := info.Size()

		fmt.Fprintf(runtime.IO().ErrOut, "Uploading: %s (%s)\n", fileName, common.FormatSize(fileSize))

		var fileToken string
		if fileSize > common.MaxDriveMediaUploadSinglePartSize {
			fmt.Fprintf(runtime.IO().ErrOut, "File exceeds 20MB, using multipart upload\n")
			fileToken, err = uploadFileMultipart(ctx, runtime, filePath, fileName, folderToken, fileSize)
		} else {
			fileToken, err = uploadFileToDrive(ctx, runtime, filePath, fileName, folderToken, fileSize)
		}
		if err != nil {
			return err
		}

		runtime.Out(map[string]interface{}{
			"file_token": fileToken,
			"file_name":  fileName,
			"size":       fileSize,
		}, nil)
		return nil
	},
}

func uploadFileToDrive(ctx context.Context, runtime *common.RuntimeContext, filePath, fileName, folderToken string, fileSize int64) (string, error) {
	f, err := vfs.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// Build SDK Formdata
	fd := larkcore.NewFormdata()
	fd.AddField("file_name", fileName)
	fd.AddField("parent_type", "explorer")
	fd.AddField("parent_node", folderToken)
	fd.AddField("size", fmt.Sprintf("%d", fileSize))
	fd.AddFile("file", f)

	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodPost,
		ApiPath:    "/open-apis/drive/v1/files/upload_all",
		Body:       fd,
	}, larkcore.WithFileUpload())
	if err != nil {
		var exitErr *output.ExitError
		if errors.As(err, &exitErr) {
			return "", err
		}
		return "", output.ErrNetwork("upload failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(apiResp.RawBody, &result); err != nil {
		return "", output.Errorf(output.ExitAPI, "api_error", "upload failed: invalid response JSON: %v", err)
	}

	if larkCode := int(common.GetFloat(result, "code")); larkCode != 0 {
		msg, _ := result["msg"].(string)
		return "", output.ErrAPI(larkCode, fmt.Sprintf("upload failed: [%d] %s", larkCode, msg), result["error"])
	}

	data, _ := result["data"].(map[string]interface{})
	fileToken, _ := data["file_token"].(string)
	if fileToken == "" {
		return "", output.Errorf(output.ExitAPI, "api_error", "upload failed: no file_token returned")
	}
	return fileToken, nil
}

// uploadFileMultipart uploads a large file using the three-step multipart API:
// 1. upload_prepare — get upload_id, block_size, block_num
// 2. upload_part   — upload each block sequentially
// 3. upload_finish — finalize and get file_token
func uploadFileMultipart(_ context.Context, runtime *common.RuntimeContext, filePath, fileName, folderToken string, fileSize int64) (string, error) {
	// Step 1: Prepare
	prepareBody := map[string]interface{}{
		"file_name":   fileName,
		"parent_type": "explorer",
		"parent_node": folderToken,
		"size":        fileSize,
	}
	prepareResult, err := runtime.CallAPI("POST", "/open-apis/drive/v1/files/upload_prepare", nil, prepareBody)
	if err != nil {
		return "", err
	}

	uploadID := common.GetString(prepareResult, "upload_id")
	blockSizeF := common.GetFloat(prepareResult, "block_size")
	blockNumF := common.GetFloat(prepareResult, "block_num")
	blockSize := int64(blockSizeF)
	blockNum := int(blockNumF)

	if uploadID == "" || blockSize <= 0 || blockNum <= 0 {
		return "", output.Errorf(output.ExitAPI, "api_error",
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

		partFile, err := vfs.Open(filePath)
		if err != nil {
			return "", output.ErrValidation("cannot open file: %v", err)
		}
		if _, err := partFile.Seek(offset, io.SeekStart); err != nil {
			partFile.Close()
			return "", output.Errorf(output.ExitInternal, "internal_error", "seek to block %d failed: %v", seq, err)
		}

		fd := larkcore.NewFormdata()
		fd.AddField("upload_id", uploadID)
		fd.AddField("seq", fmt.Sprintf("%d", seq))
		fd.AddField("size", fmt.Sprintf("%d", partSize))
		fd.AddFile("file", io.LimitReader(partFile, partSize))

		apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
			HttpMethod: http.MethodPost,
			ApiPath:    "/open-apis/drive/v1/files/upload_part",
			Body:       fd,
		}, larkcore.WithFileUpload())
		partFile.Close()
		if err != nil {
			var exitErr *output.ExitError
			if errors.As(err, &exitErr) {
				return "", err
			}
			return "", output.ErrNetwork("upload part %d/%d failed: %v", seq+1, blockNum, err)
		}

		var partResult map[string]interface{}
		if err := json.Unmarshal(apiResp.RawBody, &partResult); err != nil {
			return "", output.Errorf(output.ExitAPI, "api_error", "upload part %d/%d: invalid response JSON: %v", seq+1, blockNum, err)
		}
		if larkCode := int(common.GetFloat(partResult, "code")); larkCode != 0 {
			msg, _ := partResult["msg"].(string)
			return "", output.ErrAPI(larkCode, fmt.Sprintf("upload part %d/%d failed: [%d] %s", seq+1, blockNum, larkCode, msg), partResult["error"])
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
		return "", err
	}

	fileToken := common.GetString(finishResult, "file_token")
	if fileToken == "" {
		return "", output.Errorf(output.ExitAPI, "api_error", "upload_finish succeeded but no file_token returned")
	}

	return fileToken, nil
}
