// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
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
	// Reader, when non-nil, is used as the upload source instead of opening
	// FilePath. Callers must set FileName and FileSize explicitly. The reader
	// is NOT closed by UploadDriveMediaAllTyped; the caller owns its lifetime.
	// Used by the clipboard path in docs +media-insert.
	Reader io.Reader
}

type DriveMediaMultipartUploadConfig struct {
	FilePath   string
	FileName   string
	FileSize   int64
	ParentType string
	ParentNode string
	Extra      string
	// Reader mirrors DriveMediaUploadAllConfig.Reader for chunked uploads.
	Reader io.Reader
}

// UploadDriveMediaAllTyped uploads a file in a single request: file-open
// failures surface as typed validation errors, transport failures as typed
// network errors, and API failures are classified via ClassifyAPIResponse so
// subtype / code / log_id survive on the error.
func UploadDriveMediaAllTyped(runtime *RuntimeContext, cfg DriveMediaUploadAllConfig) (string, error) {
	var fileReader io.Reader
	if cfg.Reader != nil {
		fileReader = cfg.Reader
	} else {
		f, err := runtime.FileIO().Open(cfg.FilePath)
		if err != nil {
			return "", WrapInputStatErrorTyped(err)
		}
		defer f.Close()
		fileReader = f
	}

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
	fd.AddFile("file", fileReader)

	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodPost,
		ApiPath:    "/open-apis/drive/v1/medias/upload_all",
		Body:       fd,
	}, larkcore.WithFileUpload())
	if err != nil {
		return "", prefixDriveMediaUploadProblem(client.WrapDoAPIError(err), driveMediaUploadAllAction)
	}

	data, err := runtime.ClassifyAPIResponse(apiResp)
	if err != nil {
		return "", prefixDriveMediaUploadProblem(err, driveMediaUploadAllAction)
	}
	return extractDriveMediaUploadFileTokenTyped(data, driveMediaUploadAllAction)
}

// UploadDriveMediaMultipartTyped uploads a file in server-planned chunks:
// prepare/finish failures come back typed from CallAPITyped, malformed session
// plans surface as invalid-response internal errors, and per-part
// transport/API failures are classified the same way as
// UploadDriveMediaAllTyped.
func UploadDriveMediaMultipartTyped(runtime *RuntimeContext, cfg DriveMediaMultipartUploadConfig) (string, error) {
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

	data, err := runtime.CallAPITyped("POST", "/open-apis/drive/v1/medias/upload_prepare", nil, prepareBody)
	if err != nil {
		return "", err
	}

	session, err := parseDriveMediaMultipartUploadSessionTyped(data)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(runtime.IO().ErrOut, "Multipart upload initialized: %d chunks x %s\n", session.BlockNum, FormatSize(session.BlockSize))

	if err = uploadDriveMediaMultipartPartsTyped(runtime, cfg, session); err != nil {
		return "", err
	}

	return finishDriveMediaMultipartUploadTyped(runtime, session.UploadID, session.BlockNum)
}

// prefixDriveMediaUploadProblem prepends the upload action to a typed error's
// message so callers see which upload step failed. Non-typed errors are
// returned unchanged.
func prefixDriveMediaUploadProblem(err error, action string) error {
	if p, ok := errs.ProblemOf(err); ok {
		p.Message = action + ": " + p.Message
	}
	return err
}

// parseDriveMediaMultipartUploadSessionTyped validates the upload_prepare
// session plan, reporting a malformed plan as a typed invalid-response
// internal error.
func parseDriveMediaMultipartUploadSessionTyped(data map[string]interface{}) (DriveMediaMultipartUploadSession, error) {
	// The backend chooses both chunk size and chunk count. Validate them once so
	// the streaming loop can follow the returned plan without re-checking shape.
	session := DriveMediaMultipartUploadSession{
		UploadID:  GetString(data, "upload_id"),
		BlockSize: int64(GetFloat(data, "block_size")),
		BlockNum:  int(GetFloat(data, "block_num")),
	}
	if session.UploadID == "" {
		return DriveMediaMultipartUploadSession{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "upload prepare failed: no upload_id returned")
	}
	if session.BlockSize <= 0 {
		return DriveMediaMultipartUploadSession{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "upload prepare failed: invalid block_size returned")
	}
	if session.BlockNum <= 0 {
		return DriveMediaMultipartUploadSession{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "upload prepare failed: invalid block_num returned")
	}
	return session, nil
}

// extractDriveMediaUploadFileTokenTyped reads the file_token from a successful
// upload response, reporting a missing file_token as a typed invalid-response
// internal error.
func extractDriveMediaUploadFileTokenTyped(data map[string]interface{}, action string) (string, error) {
	fileToken := GetString(data, "file_token")
	if fileToken == "" {
		return "", errs.NewInternalError(errs.SubtypeInvalidResponse, "%s: no file_token returned", action)
	}
	return fileToken, nil
}

// uploadDriveMediaMultipartPartsTyped streams the file in server-planned
// chunks, with typed errors for file-open, file-read, and per-part upload
// failures.
func uploadDriveMediaMultipartPartsTyped(runtime *RuntimeContext, cfg DriveMediaMultipartUploadConfig, session DriveMediaMultipartUploadSession) error {
	var r io.Reader
	if cfg.Reader != nil {
		r = cfg.Reader
	} else {
		f, err := runtime.FileIO().Open(cfg.FilePath)
		if err != nil {
			return WrapInputStatErrorTyped(err)
		}
		defer f.Close()
		r = f
	}

	maxInt := int64(^uint(0) >> 1)
	bufferSize := session.BlockSize
	if bufferSize <= 0 || bufferSize > maxInt {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "upload prepare failed: invalid block_size returned")
	}
	buffer := make([]byte, int(bufferSize))
	remaining := cfg.FileSize
	// Follow the server-declared block plan exactly; upload_finish expects the
	// same block count returned by upload_prepare.
	for seq := 0; seq < session.BlockNum; seq++ {
		chunkSize := session.BlockSize
		if remaining > 0 && chunkSize > remaining {
			chunkSize = remaining
		}

		n, readErr := io.ReadFull(r, buffer[:int(chunkSize)])
		if readErr != nil {
			return WrapInputStatErrorTyped(readErr)
		}

		if err := uploadDriveMediaMultipartPartTyped(runtime, session.UploadID, seq, buffer[:n]); err != nil {
			return err
		}
		fmt.Fprintf(runtime.IO().ErrOut, "  Block %d/%d uploaded (%s)\n", seq+1, session.BlockNum, FormatSize(int64(n)))
		remaining -= int64(n)
	}

	return nil
}

func uploadDriveMediaMultipartPartTyped(runtime *RuntimeContext, uploadID string, seq int, chunk []byte) error {
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
		return prefixDriveMediaUploadProblem(client.WrapDoAPIError(err), driveMediaUploadPartAction)
	}

	if _, err := runtime.ClassifyAPIResponse(apiResp); err != nil {
		return prefixDriveMediaUploadProblem(err, driveMediaUploadPartAction)
	}
	return nil
}

func finishDriveMediaMultipartUploadTyped(runtime *RuntimeContext, uploadID string, blockNum int) (string, error) {
	data, err := runtime.CallAPITyped("POST", "/open-apis/drive/v1/medias/upload_finish", nil, map[string]interface{}{
		"upload_id": uploadID,
		"block_num": blockNum,
	})
	if err != nil {
		return "", err
	}
	return extractDriveMediaUploadFileTokenTyped(data, driveMediaUploadFinishAction)
}
