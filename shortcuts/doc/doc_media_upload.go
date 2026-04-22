// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

var DocMediaUpload = common.Shortcut{
	Service:     "docs",
	Command:     "+media-upload",
	Description: "Upload media file (image/attachment) to a document block",
	Risk:        "write",
	Scopes:      []string{"docs:document.media:upload"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "file", Desc: "local file path (files > 20MB use multipart upload automatically)", Required: true},
		{Name: "parent-type", Desc: "parent type: docx_image | docx_file | whiteboard", Required: true},
		{Name: "parent-node", Desc: "parent node ID (block_id for docx, board_token for whiteboard)", Required: true},
		{Name: "doc-id", Desc: "document ID (for drive_route_token)"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		filePath := runtime.Str("file")
		parentType := runtime.Str("parent-type")
		parentNode := runtime.Str("parent-node")
		docId := runtime.Str("doc-id")
		body := map[string]interface{}{
			"file_name":   filepath.Base(filePath),
			"parent_type": parentType,
			"parent_node": parentNode,
		}
		if docId != "" {
			body["extra"] = fmt.Sprintf(`{"drive_route_token":"%s"}`, docId)
		}
		dry := common.NewDryRunAPI()
		if docMediaShouldUseMultipart(runtime.FileIO(), filePath) {
			prepareBody := map[string]interface{}{
				"file_name":   filepath.Base(filePath),
				"parent_type": parentType,
				"parent_node": parentNode,
				"size":        "<file_size>",
			}
			if extra, ok := body["extra"]; ok {
				prepareBody["extra"] = extra
			}
			dry.Desc("chunked media upload (files > 20MB)").
				POST("/open-apis/drive/v1/medias/upload_prepare").
				Body(prepareBody).
				POST("/open-apis/drive/v1/medias/upload_part").
				Body(map[string]interface{}{
					"upload_id": "<upload_id>",
					"seq":       "<chunk_index>",
					"size":      "<chunk_size>",
					"file":      "<chunk_binary>",
				}).
				POST("/open-apis/drive/v1/medias/upload_finish").
				Body(map[string]interface{}{
					"upload_id": "<upload_id>",
					"block_num": "<block_num>",
				})
			return dry
		}

		body["file"] = "@" + filePath
		body["size"] = "<file_size>"
		return dry.Desc("multipart/form-data upload").
			POST("/open-apis/drive/v1/medias/upload_all").
			Body(body)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		filePath := runtime.Str("file")
		parentType := runtime.Str("parent-type")
		parentNode := runtime.Str("parent-node")
		docId := runtime.Str("doc-id")

		// Validate file
		stat, err := runtime.FileIO().Stat(filePath)
		if err != nil {
			return common.WrapInputStatError(err, "file not found")
		}
		if !stat.Mode().IsRegular() {
			return output.ErrValidation("file must be a regular file: %s", filePath)
		}

		fileName := filepath.Base(filePath)
		fmt.Fprintf(runtime.IO().ErrOut, "Uploading: %s (%d bytes)\n", fileName, stat.Size())
		if stat.Size() > common.MaxDriveMediaUploadSinglePartSize {
			fmt.Fprintf(runtime.IO().ErrOut, "File exceeds 20MB, using multipart upload\n")
		}

		fileToken, err := uploadDocMediaFile(runtime, UploadDocMediaFileConfig{
			FilePath:   filePath,
			FileName:   fileName,
			FileSize:   stat.Size(),
			ParentType: parentType,
			ParentNode: parentNode,
			DocID:      docId,
		})
		if err != nil {
			return err
		}

		runtime.Out(map[string]interface{}{
			"file_token": fileToken,
			"file_name":  fileName,
			"size":       stat.Size(),
		}, nil)
		return nil
	},
}

// UploadDocMediaFileConfig groups the inputs to uploadDocMediaFile so the
// call site names each value at call time, avoiding the "8 positional
// params of mostly string/int64" ambiguity and mirroring the config-struct
// style already used by DriveMediaUploadAllConfig /
// DriveMediaMultipartUploadConfig downstream.
//
// Exactly one of FilePath (on-disk source) or Reader (in-memory source for
// the clipboard flow) should be set. Leave Reader at its zero value (nil
// interface) when the caller only has FilePath — passing a typed-nil
// pointer like (*bytes.Reader)(nil) here would make Reader compare
// non-nil downstream and skip the FilePath open, so the field type is
// deliberately an interface and the clipboard caller builds it only when
// it actually has bytes.
type UploadDocMediaFileConfig struct {
	FilePath   string
	Reader     io.Reader
	FileName   string
	FileSize   int64
	ParentType string
	ParentNode string
	DocID      string
}

func uploadDocMediaFile(runtime *common.RuntimeContext, cfg UploadDocMediaFileConfig) (string, error) {
	var extra string
	if cfg.DocID != "" {
		var err error
		extra, err = buildDriveRouteExtra(cfg.DocID)
		if err != nil {
			return "", err
		}
	}

	// Doc media uploads share the generic Drive media transport. The doc-specific
	// routing only shows up in parent_type/parent_node and optional route extra.
	if cfg.FileSize <= common.MaxDriveMediaUploadSinglePartSize {
		return common.UploadDriveMediaAll(runtime, common.DriveMediaUploadAllConfig{
			FilePath:   cfg.FilePath,
			Reader:     cfg.Reader,
			FileName:   cfg.FileName,
			FileSize:   cfg.FileSize,
			ParentType: cfg.ParentType,
			ParentNode: &cfg.ParentNode,
			Extra:      extra,
		})
	}
	return common.UploadDriveMediaMultipart(runtime, common.DriveMediaMultipartUploadConfig{
		FilePath:   cfg.FilePath,
		Reader:     cfg.Reader,
		FileName:   cfg.FileName,
		FileSize:   cfg.FileSize,
		ParentType: cfg.ParentType,
		ParentNode: cfg.ParentNode,
		Extra:      extra,
	})
}

func docMediaShouldUseMultipart(fio fileio.FileIO, filePath string) bool {
	// Dry-run uses local stat as a best-effort planning hint. Execute re-validates
	// the file before choosing the actual upload path.
	info, err := fio.Stat(filePath)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular() && info.Size() > common.MaxDriveMediaUploadSinglePartSize
}
