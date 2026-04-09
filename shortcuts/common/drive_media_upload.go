// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/output"
)

const MaxDriveMediaUploadSinglePartSize int64 = 20 * 1024 * 1024 // 20MB

const (
	driveMediaUploadAllAction    = "upload media failed"
	driveMediaUploadPartAction   = "upload media part failed"
	driveMediaUploadFinishAction = "upload media finish failed"
)

type DriveMediaMultipartUploadSession struct {
	UploadID  string
	BlockSize int64
	BlockNum  int
}

type DriveMediaUploadAllConfig struct {
	FilePath   string
	FileName   string
	FileSize   int64
	ParentType string
	ParentNode *string
	Extra      string
}

type DriveMediaMultipartUploadConfig struct {
	FilePath   string
	FileName   string
	FileSize   int64
	ParentType string
	ParentNode string
	Extra      string
}

func UploadDriveMediaAll(runtime *RuntimeContext, cfg DriveMediaUploadAllConfig) (string, error) {
	safeFilePath, err := validate.SafeInputPath(cfg.FilePath)
	if err != nil {
		return "", output.ErrValidation("invalid file path: %s", err)
	}
	f, err := vfs.Open(safeFilePath)
	if err != nil {
		return "", output.ErrValidation("cannot read file: %s", err)
	}
	defer f.Close()

	fd := larkcore.NewFormdata()
	fd.AddField("file_name", cfg.FileName)
	fd.AddField("parent_type", cfg.ParentType)
	fd.AddField("size", fmt.Sprintf("%d", cfg.FileSize))
	if cfg.ParentNode != nil {
		fd.AddField("parent_node", *cfg.ParentNode)
	}
	if cfg.Extra != "" {
		fd.AddField("extra", cfg.Extra)
	}
	fd.AddFile("file", f)

	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodPost,
		ApiPath:    "/open-apis/drive/v1/medias/upload_all",
		Body:       fd,
	}, larkcore.WithFileUpload())
	if err != nil {
		return "", WrapDriveMediaUploadRequestError(err, driveMediaUploadAllAction)
	}

	data, err := ParseDriveMediaUploadResponse(apiResp, driveMediaUploadAllAction)
	if err != nil {
		return "", err
	}
	return ExtractDriveMediaUploadFileToken(data, driveMediaUploadAllAction)
}

func UploadDriveMediaMultipart(runtime *RuntimeContext, cfg DriveMediaMultipartUploadConfig) (string, error) {
	// upload_prepare expects parent_node to be present even when the caller wants
	// the service default/root behavior, so multipart callers pass an explicit
	// string instead of relying on field omission like upload_all does.
	prepareBody := map[string]interface{}{
		"file_name":   cfg.FileName,
		"parent_type": cfg.ParentType,
		"parent_node": cfg.ParentNode,
		"size":        cfg.FileSize,
	}
	if cfg.Extra != "" {
		prepareBody["extra"] = cfg.Extra
	}

	data, err := runtime.CallAPI("POST", "/open-apis/drive/v1/medias/upload_prepare", nil, prepareBody)
	if err != nil {
		return "", err
	}

	session, err := ParseDriveMediaMultipartUploadSession(data)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(runtime.IO().ErrOut, "Multipart upload initialized: %d chunks x %s\n", session.BlockNum, FormatSize(session.BlockSize))

	if err = uploadDriveMediaMultipartParts(runtime, cfg.FilePath, cfg.FileSize, session); err != nil {
		return "", err
	}

	return finishDriveMediaMultipartUpload(runtime, session.UploadID, session.BlockNum)
}

func ParseDriveMediaMultipartUploadSession(data map[string]interface{}) (DriveMediaMultipartUploadSession, error) {
	// The backend chooses both chunk size and chunk count. Validate them once so
	// the streaming loop can follow the returned plan without re-checking shape.
	session := DriveMediaMultipartUploadSession{
		UploadID:  GetString(data, "upload_id"),
		BlockSize: int64(GetFloat(data, "block_size")),
		BlockNum:  int(GetFloat(data, "block_num")),
	}
	if session.UploadID == "" {
		return DriveMediaMultipartUploadSession{}, output.Errorf(output.ExitAPI, "api_error", "upload prepare failed: no upload_id returned")
	}
	if session.BlockSize <= 0 {
		return DriveMediaMultipartUploadSession{}, output.Errorf(output.ExitAPI, "api_error", "upload prepare failed: invalid block_size returned")
	}
	if session.BlockNum <= 0 {
		return DriveMediaMultipartUploadSession{}, output.Errorf(output.ExitAPI, "api_error", "upload prepare failed: invalid block_num returned")
	}
	return session, nil
}

func WrapDriveMediaUploadRequestError(err error, action string) error {
	var exitErr *output.ExitError
	if errors.As(err, &exitErr) {
		return err
	}
	return output.ErrNetwork("%s: %v", action, err)
}

func ParseDriveMediaUploadResponse(apiResp *larkcore.ApiResp, action string) (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := json.Unmarshal(apiResp.RawBody, &result); err != nil {
		return nil, output.Errorf(output.ExitAPI, "api_error", "%s: invalid response JSON: %v", action, err)
	}

	if larkCode := int(GetFloat(result, "code")); larkCode != 0 {
		msg, _ := result["msg"].(string)
		return nil, output.ErrAPI(larkCode, fmt.Sprintf("%s: [%d] %s", action, larkCode, msg), result["error"])
	}

	data, _ := result["data"].(map[string]interface{})
	return data, nil
}

func ExtractDriveMediaUploadFileToken(data map[string]interface{}, action string) (string, error) {
	fileToken := GetString(data, "file_token")
	if fileToken == "" {
		return "", output.Errorf(output.ExitAPI, "api_error", "%s: no file_token returned", action)
	}
	return fileToken, nil
}

func uploadDriveMediaMultipartParts(runtime *RuntimeContext, filePath string, fileSize int64, session DriveMediaMultipartUploadSession) error {
	safeFilePath, err := validate.SafeInputPath(filePath)
	if err != nil {
		return output.ErrValidation("invalid file path: %s", err)
	}
	f, err := vfs.Open(safeFilePath)
	if err != nil {
		return output.ErrValidation("cannot read file: %s", err)
	}
	defer f.Close()

	maxInt := int64(^uint(0) >> 1)
	bufferSize := session.BlockSize
	if bufferSize <= 0 || bufferSize > maxInt {
		return output.Errorf(output.ExitAPI, "api_error", "upload prepare failed: invalid block_size returned")
	}
	buffer := make([]byte, int(bufferSize))
	remaining := fileSize
	// Follow the server-declared block plan exactly; upload_finish expects the
	// same block count returned by upload_prepare.
	for seq := 0; seq < session.BlockNum; seq++ {
		chunkSize := session.BlockSize
		if remaining > 0 && chunkSize > remaining {
			chunkSize = remaining
		}

		n, readErr := io.ReadFull(f, buffer[:int(chunkSize)])
		if readErr != nil {
			return output.ErrValidation("cannot read file: %s", readErr)
		}

		if err = uploadDriveMediaMultipartPart(runtime, session.UploadID, seq, buffer[:n]); err != nil {
			return err
		}
		fmt.Fprintf(runtime.IO().ErrOut, "  Block %d/%d uploaded (%s)\n", seq+1, session.BlockNum, FormatSize(int64(n)))
		remaining -= int64(n)
	}

	return nil
}

func uploadDriveMediaMultipartPart(runtime *RuntimeContext, uploadID string, seq int, chunk []byte) error {
	fd := larkcore.NewFormdata()
	fd.AddField("upload_id", uploadID)
	fd.AddField("seq", fmt.Sprintf("%d", seq))
	fd.AddField("size", fmt.Sprintf("%d", len(chunk)))
	fd.AddFile("file", bytes.NewReader(chunk))

	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodPost,
		ApiPath:    "/open-apis/drive/v1/medias/upload_part",
		Body:       fd,
	}, larkcore.WithFileUpload())
	if err != nil {
		return WrapDriveMediaUploadRequestError(err, driveMediaUploadPartAction)
	}

	_, err = ParseDriveMediaUploadResponse(apiResp, driveMediaUploadPartAction)
	return err
}

func finishDriveMediaMultipartUpload(runtime *RuntimeContext, uploadID string, blockNum int) (string, error) {
	data, err := runtime.CallAPI("POST", "/open-apis/drive/v1/medias/upload_finish", nil, map[string]interface{}{
		"upload_id": uploadID,
		"block_num": blockNum,
	})
	if err != nil {
		return "", err
	}
	return ExtractDriveMediaUploadFileToken(data, driveMediaUploadFinishAction)
}
