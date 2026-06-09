// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package markdown

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const markdownSinglePartSizeLimit = common.MaxDriveMediaUploadSinglePartSize
const markdownEmptyContentError = "empty markdown content is not supported; cannot create or overwrite an empty file"

const (
	markdownUploadParentTypeExplorer = "explorer"
	markdownUploadParentTypeWiki     = "wiki"
	markdownUploadAllAction          = "upload markdown file failed"
	markdownUploadPrepareAction      = "initialize markdown multipart upload failed"
	markdownUploadFinishAction       = "finalize markdown multipart upload failed"
	markdownFetchNameAction          = "fetch existing markdown file name failed"
)

var markdownUploadRetryBackoffs = []time.Duration{
	200 * time.Millisecond,
	500 * time.Millisecond,
}

type markdownUploadSpec struct {
	FileToken   string
	FileName    string
	FolderToken string
	WikiToken   string
	FilePath    string
	Content     string
	ContentSet  bool
	FileSet     bool
}

type markdownUploadResult struct {
	FileToken string
	Version   string
}

type markdownMultipartSession struct {
	UploadID  string
	BlockSize int64
	BlockNum  int
}

type markdownUploadTarget struct {
	ParentType string
	ParentNode string
}

func (spec markdownUploadSpec) Target() markdownUploadTarget {
	if spec.WikiToken != "" {
		return markdownUploadTarget{
			ParentType: markdownUploadParentTypeWiki,
			ParentNode: spec.WikiToken,
		}
	}
	// An empty explorer parent node uploads to the user's Drive root folder.
	return markdownUploadTarget{
		ParentType: markdownUploadParentTypeExplorer,
		ParentNode: spec.FolderToken,
	}
}

func validateMarkdownSpec(runtime *common.RuntimeContext, spec markdownUploadSpec, requireName bool) error {
	switch {
	case spec.ContentSet && spec.FileSet:
		return common.FlagErrorf("--content and --file are mutually exclusive")
	case !spec.ContentSet && !spec.FileSet:
		return common.FlagErrorf("specify exactly one of --content or --file")
	}

	if markdownFlagExplicitlyEmpty(runtime, "folder-token") {
		return common.FlagErrorf("--folder-token cannot be empty; omit it to upload into Drive root folder")
	}
	if markdownFlagExplicitlyEmpty(runtime, "wiki-token") {
		return common.FlagErrorf("--wiki-token cannot be empty; provide a valid wiki node token or omit the flag entirely")
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

	if requireName && spec.ContentSet {
		if strings.TrimSpace(spec.FileName) == "" {
			return common.FlagErrorf("--name is required when using --content")
		}
		if err := validateMarkdownFileName(spec.FileName, "--name"); err != nil {
			return err
		}
	}

	if spec.FileSet {
		if strings.TrimSpace(spec.FilePath) == "" {
			return common.FlagErrorf("--file cannot be empty")
		}
		if _, err := validate.SafeInputPath(spec.FilePath); err != nil {
			return output.ErrValidation("unsafe file path: %s", err)
		}
		if err := validateMarkdownFileName(filepath.Base(spec.FilePath), "--file"); err != nil {
			return err
		}
	}

	if spec.FileName != "" {
		if err := validateMarkdownFileName(spec.FileName, "--name"); err != nil {
			return err
		}
	}

	return nil
}

func markdownFlagExplicitlyEmpty(runtime *common.RuntimeContext, flagName string) bool {
	return runtime.Changed(flagName) && strings.TrimSpace(runtime.Str(flagName)) == ""
}

func validateMarkdownFileName(name, flagName string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return common.FlagErrorf("%s cannot be empty", flagName)
	}
	if !strings.HasSuffix(strings.ToLower(trimmed), ".md") {
		return common.FlagErrorf("%s must end with .md", flagName)
	}
	return nil
}

func finalMarkdownFileName(spec markdownUploadSpec) string {
	if strings.TrimSpace(spec.FileName) != "" {
		return strings.TrimSpace(spec.FileName)
	}
	if strings.TrimSpace(spec.FilePath) == "" {
		return ""
	}
	return filepath.Base(spec.FilePath)
}

func resolveMarkdownOverwriteFileName(runtime *common.RuntimeContext, spec markdownUploadSpec) (string, error) {
	fileName := strings.TrimSpace(spec.FileName)
	if fileName == "" && spec.FileSet {
		fileName = filepath.Base(spec.FilePath)
	}
	if fileName == "" {
		remoteName, err := fetchMarkdownFileName(runtime, spec.FileToken)
		if err != nil {
			return "", err
		}
		fileName = strings.TrimSpace(remoteName)
	}
	if fileName == "" {
		fileName = spec.FileToken + ".md"
	}
	return fileName, nil
}

func openMarkdownDownload(ctx context.Context, runtime *common.RuntimeContext, fileToken string) (*http.Response, error) {
	resp, err := runtime.DoAPIStream(ctx, &larkcore.ApiReq{
		HttpMethod: http.MethodGet,
		ApiPath:    fmt.Sprintf("/open-apis/drive/v1/files/%s/download", validate.EncodePathSegment(fileToken)),
	})
	if err != nil {
		return nil, wrapMarkdownDownloadError(err)
	}
	return resp, nil
}

func wrapMarkdownDownloadError(err error) error {
	// Preserve any already-classified error: legacy *output.ExitError or any
	// typed errs.* error. Only un-classified errors get wrapped as network.
	var exitErr *output.ExitError
	if errors.As(err, &exitErr) {
		return err
	}
	if _, ok := errs.ProblemOf(err); ok {
		return err
	}
	return output.ErrNetwork("download failed: %s", err)
}

func validateNonEmptyMarkdownSize(size int64) error {
	if size == 0 {
		return output.ErrValidation("%s", markdownEmptyContentError)
	}
	return nil
}

func markdownSourceSize(runtime *common.RuntimeContext, spec markdownUploadSpec) (int64, error) {
	var size int64
	if spec.ContentSet {
		size = int64(len(spec.Content))
	} else {
		if strings.TrimSpace(spec.FilePath) == "" {
			return 0, common.FlagErrorf("--file cannot be empty")
		}

		info, err := runtime.FileIO().Stat(spec.FilePath)
		if err != nil {
			return 0, common.WrapInputStatError(err)
		}
		size = info.Size()
	}
	if err := validateNonEmptyMarkdownSize(size); err != nil {
		return 0, err
	}
	return size, nil
}

func openMarkdownDownloadVersion(ctx context.Context, runtime *common.RuntimeContext, fileToken, version string) (*http.Response, string, error) {
	req := &larkcore.ApiReq{
		HttpMethod: http.MethodGet,
		ApiPath:    fmt.Sprintf("/open-apis/drive/v1/files/%s/download", validate.EncodePathSegment(fileToken)),
	}
	if strings.TrimSpace(version) != "" {
		req.QueryParams = larkcore.QueryParams{
			"version": []string{strings.TrimSpace(version)},
		}
	}

	resp, err := runtime.DoAPIStream(ctx, req)
	if err != nil {
		return nil, "", wrapMarkdownDownloadError(err)
	}
	return resp, fileNameFromDownloadHeader(resp.Header, fileToken+".md"), nil
}

func markdownDryRunFileField(spec markdownUploadSpec) string {
	if spec.FilePath != "" {
		return "@" + spec.FilePath
	}
	return "<markdown content>"
}

func markdownUploadDryRun(spec markdownUploadSpec, fileSize int64, multipart bool) *common.DryRunAPI {
	fileName := finalMarkdownFileName(spec)
	target := spec.Target()

	if !multipart {
		body := map[string]interface{}{
			"file_name":   fileName,
			"parent_type": target.ParentType,
			"parent_node": target.ParentNode,
			"size":        fileSize,
			"file":        markdownDryRunFileField(spec),
		}
		if spec.FileToken != "" {
			body["file_token"] = spec.FileToken
		}

		desc := "multipart/form-data upload"
		if spec.FileToken != "" {
			desc = "multipart/form-data overwrite upload"
		}

		return common.NewDryRunAPI().
			Desc(desc).
			POST("/open-apis/drive/v1/files/upload_all").
			Body(body)
	}

	prepareBody := map[string]interface{}{
		"file_name":   fileName,
		"parent_type": target.ParentType,
		"parent_node": target.ParentNode,
		"size":        fileSize,
	}
	if spec.FileToken != "" {
		prepareBody["file_token"] = spec.FileToken
	}

	desc := "3-step multipart upload"
	if spec.FileToken != "" {
		desc = "3-step multipart overwrite upload"
	}

	return common.NewDryRunAPI().
		Desc(desc).
		POST("/open-apis/drive/v1/files/upload_prepare").
		Desc("[1] Initialize multipart upload").
		Body(prepareBody).
		POST("/open-apis/drive/v1/files/upload_part").
		Desc("[2] Upload file parts (repeated)").
		Body(map[string]interface{}{
			"upload_id": "<upload_id>",
			"seq":       "<chunk_index>",
			"size":      "<chunk_size>",
			"file":      "<chunk_binary>",
		}).
		POST("/open-apis/drive/v1/files/upload_finish").
		Desc("[3] Finalize upload and get file_token/version").
		Body(map[string]interface{}{
			"upload_id": "<upload_id>",
			"block_num": "<block_num>",
		})
}

func markdownOverwriteDryRun(spec markdownUploadSpec, fileSize int64, multipart bool) *common.DryRunAPI {
	fileName := strings.TrimSpace(spec.FileName)
	target := spec.Target()
	if fileName == "" && spec.FileSet {
		fileName = finalMarkdownFileName(spec)
	}
	if fileName != "" {
		spec.FileName = fileName
		return markdownUploadDryRun(spec, fileSize, multipart)
	}

	dry := common.NewDryRunAPI().Desc("Fetch the existing file name, then overwrite the file content")
	dry.POST("/open-apis/drive/v1/metas/batch_query").
		Desc("[1] Read current file metadata to preserve the existing file name").
		Body(map[string]interface{}{
			"request_docs": []map[string]interface{}{
				{
					"doc_token": spec.FileToken,
					"doc_type":  "file",
				},
			},
		})

	spec.FileName = "<existing_remote_name_or_" + spec.FileToken + ".md>"
	if !multipart {
		dry.POST("/open-apis/drive/v1/files/upload_all").
			Desc("[2] Overwrite file contents with multipart/form-data upload").
			Body(map[string]interface{}{
				"file_name":   spec.FileName,
				"parent_type": target.ParentType,
				"parent_node": target.ParentNode,
				"size":        fileSize,
				"file":        markdownDryRunFileField(spec),
				"file_token":  spec.FileToken,
			})
		return dry
	}

	dry.POST("/open-apis/drive/v1/files/upload_prepare").
		Desc("[2] Initialize multipart overwrite upload").
		Body(map[string]interface{}{
			"file_name":   spec.FileName,
			"parent_type": target.ParentType,
			"parent_node": target.ParentNode,
			"size":        fileSize,
			"file_token":  spec.FileToken,
		}).
		POST("/open-apis/drive/v1/files/upload_part").
		Desc("[3] Upload file parts (repeated)").
		Body(map[string]interface{}{
			"upload_id": "<upload_id>",
			"seq":       "<chunk_index>",
			"size":      "<chunk_size>",
			"file":      "<chunk_binary>",
		}).
		POST("/open-apis/drive/v1/files/upload_finish").
		Desc("[4] Finalize upload and get file_token/version").
		Body(map[string]interface{}{
			"upload_id": "<upload_id>",
			"block_num": "<block_num>",
		})
	return dry
}

func uploadMarkdownContent(runtime *common.RuntimeContext, spec markdownUploadSpec, payload []byte) (markdownUploadResult, error) {
	fileName := finalMarkdownFileName(spec)
	fileSize := int64(len(payload))
	if fileSize > markdownSinglePartSizeLimit {
		return uploadMarkdownFileMultipart(runtime, spec, fileName, fileSize, func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(payload)), nil
		})
	}
	return uploadMarkdownFileAll(runtime, spec, fileName, fileSize, func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(payload)), nil
	})
}

func uploadMarkdownLocalFile(runtime *common.RuntimeContext, spec markdownUploadSpec, fileSize int64) (markdownUploadResult, error) {
	fileName := finalMarkdownFileName(spec)
	if fileSize > markdownSinglePartSizeLimit {
		return uploadMarkdownFileMultipart(runtime, spec, fileName, fileSize, func() (io.ReadCloser, error) {
			return runtime.FileIO().Open(spec.FilePath)
		})
	}
	return uploadMarkdownFileAll(runtime, spec, fileName, fileSize, func() (io.ReadCloser, error) {
		return runtime.FileIO().Open(spec.FilePath)
	})
}

func uploadMarkdownFileAll(runtime *common.RuntimeContext, spec markdownUploadSpec, fileName string, fileSize int64, openReader func() (io.ReadCloser, error)) (markdownUploadResult, error) {
	target := spec.Target()
	return withMarkdownUploadRetryResult(runtime, markdownUploadAllAction, func() (markdownUploadResult, error) {
		fileReader, err := openReader()
		if err != nil {
			return markdownUploadResult{}, common.WrapInputStatErrorTyped(err)
		}
		defer fileReader.Close()

		fd := larkcore.NewFormdata()
		fd.AddField("file_name", fileName)
		fd.AddField("parent_type", target.ParentType)
		fd.AddField("parent_node", target.ParentNode)
		fd.AddField("size", fmt.Sprintf("%d", fileSize))
		if spec.FileToken != "" {
			fd.AddField("file_token", spec.FileToken)
		}
		fd.AddFile("file", fileReader)

		apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
			HttpMethod: http.MethodPost,
			ApiPath:    "/open-apis/drive/v1/files/upload_all",
			Body:       fd,
		}, larkcore.WithFileUpload())
		if err != nil {
			return markdownUploadResult{}, markdownUploadProblem(client.WrapDoAPIError(err), markdownUploadAllAction)
		}

		data, err := runtime.ClassifyAPIResponse(apiResp)
		if err != nil {
			return markdownUploadResult{}, markdownUploadProblem(err, markdownUploadAllAction)
		}
		result, err := parseMarkdownUploadResult(data, spec.FileToken != "")
		if err != nil {
			return markdownUploadResult{}, markdownUploadProblem(err, markdownUploadAllAction)
		}
		return result, nil
	})
}

func uploadMarkdownFileMultipart(runtime *common.RuntimeContext, spec markdownUploadSpec, fileName string, fileSize int64, openReader func() (io.ReadCloser, error)) (markdownUploadResult, error) {
	target := spec.Target()
	prepareBody := map[string]interface{}{
		"file_name":   fileName,
		"parent_type": target.ParentType,
		"parent_node": target.ParentNode,
		"size":        fileSize,
	}
	if spec.FileToken != "" {
		prepareBody["file_token"] = spec.FileToken
	}

	prepareResult, err := withMarkdownUploadRetryData(runtime, markdownUploadPrepareAction, func() (map[string]interface{}, error) {
		data, err := runtime.CallAPITyped("POST", "/open-apis/drive/v1/files/upload_prepare", nil, prepareBody)
		if err != nil {
			return nil, markdownUploadProblem(err, markdownUploadPrepareAction)
		}
		return data, nil
	})
	if err != nil {
		return markdownUploadResult{}, err
	}

	session, err := parseMarkdownMultipartSession(prepareResult)
	if err != nil {
		return markdownUploadResult{}, markdownUploadProblem(err, markdownUploadPrepareAction)
	}

	fmt.Fprintf(runtime.IO().ErrOut, "Multipart upload initialized: %d chunks x %s\n", session.BlockNum, common.FormatSize(session.BlockSize))

	fileReader, err := openReader()
	if err != nil {
		return markdownUploadResult{}, common.WrapInputStatErrorTyped(err)
	}
	defer fileReader.Close()

	if err := uploadMarkdownMultipartParts(runtime, fileReader, fileSize, session); err != nil {
		return markdownUploadResult{}, err
	}

	finishResult, err := withMarkdownUploadRetryData(runtime, markdownUploadFinishAction, func() (map[string]interface{}, error) {
		data, err := runtime.CallAPITyped("POST", "/open-apis/drive/v1/files/upload_finish", nil, map[string]interface{}{
			"upload_id": session.UploadID,
			"block_num": session.BlockNum,
		})
		if err != nil {
			return nil, markdownUploadProblem(err, markdownUploadFinishAction)
		}
		return data, nil
	})
	if err != nil {
		return markdownUploadResult{}, err
	}

	result, err := parseMarkdownUploadResult(finishResult, spec.FileToken != "")
	if err != nil {
		return markdownUploadResult{}, markdownUploadProblem(err, markdownUploadFinishAction)
	}
	return result, nil
}

func parseMarkdownMultipartSession(data map[string]interface{}) (markdownMultipartSession, error) {
	session := markdownMultipartSession{
		UploadID:  common.GetString(data, "upload_id"),
		BlockSize: int64(common.GetFloat(data, "block_size")),
		BlockNum:  int(common.GetFloat(data, "block_num")),
	}
	if session.UploadID == "" || session.BlockSize <= 0 || session.BlockNum <= 0 {
		return markdownMultipartSession{}, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"upload_prepare returned invalid data: upload_id=%q, block_size=%d, block_num=%d",
			session.UploadID, session.BlockSize, session.BlockNum)
	}
	return session, nil
}

func uploadMarkdownMultipartParts(runtime *common.RuntimeContext, fileReader io.Reader, payloadSize int64, session markdownMultipartSession) error {
	expectedBlocks := int((payloadSize + session.BlockSize - 1) / session.BlockSize)
	if session.BlockNum != expectedBlocks {
		return errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"upload_prepare returned inconsistent chunk plan: block_size=%d, block_num=%d, expected_block_num=%d, payload_size=%d",
			session.BlockSize,
			session.BlockNum,
			expectedBlocks,
			payloadSize,
		)
	}

	maxInt := int64(^uint(0) >> 1)
	if session.BlockSize > maxInt {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "upload prepare failed: invalid block_size returned")
	}

	buffer := make([]byte, int(session.BlockSize))
	remaining := payloadSize

	for seq := 0; seq < session.BlockNum; seq++ {
		chunkSize := session.BlockSize
		if remaining > 0 && chunkSize > remaining {
			chunkSize = remaining
		}

		n, readErr := io.ReadFull(fileReader, buffer[:int(chunkSize)])
		if readErr != nil {
			return output.ErrValidation("cannot read file: %s", readErr)
		}

		fd := larkcore.NewFormdata()
		fd.AddField("upload_id", session.UploadID)
		fd.AddField("seq", fmt.Sprintf("%d", seq))
		fd.AddField("size", fmt.Sprintf("%d", n))
		action := fmt.Sprintf("upload markdown file part %d/%d failed", seq+1, session.BlockNum)
		if err := withMarkdownUploadRetryVoid(runtime, action, func() error {
			fd := larkcore.NewFormdata()
			fd.AddField("upload_id", session.UploadID)
			fd.AddField("seq", fmt.Sprintf("%d", seq))
			fd.AddField("size", fmt.Sprintf("%d", n))
			fd.AddFile("file", bytes.NewReader(buffer[:n]))

			apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
				HttpMethod: http.MethodPost,
				ApiPath:    "/open-apis/drive/v1/files/upload_part",
				Body:       fd,
			}, larkcore.WithFileUpload())
			if err != nil {
				return markdownUploadProblem(client.WrapDoAPIError(err), action)
			}
			if _, err := runtime.ClassifyAPIResponse(apiResp); err != nil {
				return markdownUploadProblem(err, action)
			}
			return nil
		}); err != nil {
			return err
		}

		fmt.Fprintf(runtime.IO().ErrOut, "  Block %d/%d uploaded (%s)\n", seq+1, session.BlockNum, common.FormatSize(int64(n)))
		remaining -= int64(n)
	}
	if remaining != 0 {
		return errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"upload_prepare returned inconsistent chunk plan: %d bytes remain after %d blocks",
			remaining,
			session.BlockNum,
		)
	}

	return nil
}

func parseMarkdownUploadResult(data map[string]interface{}, requireVersion bool) (markdownUploadResult, error) {
	result := markdownUploadResult{
		FileToken: common.GetString(data, "file_token"),
		Version:   common.GetString(data, "version"),
	}
	if result.Version == "" {
		result.Version = common.GetString(data, "data_version")
	}
	if result.FileToken == "" {
		return markdownUploadResult{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "upload failed: no file_token returned")
	}
	if requireVersion && result.Version == "" {
		return markdownUploadResult{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "overwrite failed: no version returned")
	}
	return result, nil
}

func fetchMarkdownFileName(runtime *common.RuntimeContext, fileToken string) (string, error) {
	data, err := withMarkdownUploadRetryData(runtime, markdownFetchNameAction, func() (map[string]interface{}, error) {
		data, err := runtime.CallAPITyped(
			"POST",
			"/open-apis/drive/v1/metas/batch_query",
			nil,
			map[string]interface{}{
				"request_docs": []map[string]interface{}{
					{
						"doc_token": fileToken,
						"doc_type":  "file",
					},
				},
			},
		)
		if err != nil {
			return nil, markdownUploadProblem(err, markdownFetchNameAction)
		}
		return data, nil
	})
	if err != nil {
		return "", err
	}

	metas := common.GetSlice(data, "metas")
	if len(metas) == 0 {
		return "", nil
	}
	meta, _ := metas[0].(map[string]interface{})
	return common.GetString(meta, "title"), nil
}

func withMarkdownUploadRetryResult(runtime *common.RuntimeContext, action string, fn func() (markdownUploadResult, error)) (markdownUploadResult, error) {
	var zero markdownUploadResult
	for attempt := 0; ; attempt++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}
		if !markdownUploadShouldRetry(err) || attempt >= len(markdownUploadRetryBackoffs) {
			return zero, markdownUploadRetryExhausted(err, action, attempt)
		}
		fmt.Fprintf(runtime.IO().ErrOut, "%s; retrying (attempt %d/%d)\n", err.Error(), attempt+1, len(markdownUploadRetryBackoffs))
		time.Sleep(markdownUploadRetryBackoffs[attempt])
	}
}

func withMarkdownUploadRetryData(runtime *common.RuntimeContext, action string, fn func() (map[string]interface{}, error)) (map[string]interface{}, error) {
	for attempt := 0; ; attempt++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}
		if !markdownUploadShouldRetry(err) || attempt >= len(markdownUploadRetryBackoffs) {
			return nil, markdownUploadRetryExhausted(err, action, attempt)
		}
		fmt.Fprintf(runtime.IO().ErrOut, "%s; retrying (attempt %d/%d)\n", err.Error(), attempt+1, len(markdownUploadRetryBackoffs))
		time.Sleep(markdownUploadRetryBackoffs[attempt])
	}
}

func withMarkdownUploadRetryVoid(runtime *common.RuntimeContext, action string, fn func() error) error {
	for attempt := 0; ; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		if !markdownUploadShouldRetry(err) || attempt >= len(markdownUploadRetryBackoffs) {
			return markdownUploadRetryExhausted(err, action, attempt)
		}
		fmt.Fprintf(runtime.IO().ErrOut, "%s; retrying (attempt %d/%d)\n", err.Error(), attempt+1, len(markdownUploadRetryBackoffs))
		time.Sleep(markdownUploadRetryBackoffs[attempt])
	}
}

func markdownUploadShouldRetry(err error) bool {
	p, ok := errs.ProblemOf(err)
	if !ok || p == nil {
		return false
	}
	return p.Retryable || p.Category == errs.CategoryNetwork
}

func markdownUploadRetryExhausted(err error, action string, retries int) error {
	if retries <= 0 {
		return err
	}
	return appendMarkdownProblemHint(err, fmt.Sprintf("%s remained retryable after %d attempts; retry later if the upstream service is throttling or temporarily unavailable", action, retries+1))
}

func markdownUploadProblem(err error, action string) error {
	if p, ok := errs.ProblemOf(err); ok {
		p.Message = action + ": " + p.Message
		switch p.Code {
		case 99991672, 99991679:
			appendMarkdownProblemHint(err, "The current token or identity lacks the required document upload scope/capability. Grant the document upload scope or use a token with the appropriate permissions, then retry.")
		case 10071:
			appendMarkdownProblemHint(err, "The target document has reached its version limit. Clean up old versions or create a new file before retrying.")
		case 90003087:
			appendMarkdownProblemHint(err, "The current tenant or user may not have document capabilities enabled. Ask an administrator to verify document-module access.")
		case 1061003, 1061044:
			appendMarkdownProblemHint(err, "Check whether the target folder or wiki node still exists, and verify the token you passed to the command.")
		case 1061004, 1062501:
			appendMarkdownProblemHint(err, "Check whether the current identity has write access to the target folder or wiki node.")
		}
	}
	return err
}

func appendMarkdownProblemHint(err error, hint string) error {
	if strings.TrimSpace(hint) == "" {
		return err
	}
	if p, ok := errs.ProblemOf(err); ok {
		if strings.TrimSpace(p.Hint) != "" {
			p.Hint = p.Hint + "\n" + hint
		} else {
			p.Hint = hint
		}
	}
	return err
}

func prettyPrintMarkdownWrite(w io.Writer, data map[string]interface{}) {
	fmt.Fprintf(w, "file_token: %s\n", common.GetString(data, "file_token"))
	fmt.Fprintf(w, "file_name: %s\n", common.GetString(data, "file_name"))
	if url := common.GetString(data, "url"); url != "" {
		fmt.Fprintf(w, "url: %s\n", url)
	}
	version := common.GetString(data, "version")
	if version == "" {
		version = common.GetString(data, "data_version")
	}
	if version != "" {
		fmt.Fprintf(w, "version: %s\n", version)
	}
	fmt.Fprintf(w, "size_bytes: %d\n", int64(common.GetFloat(data, "size_bytes")))
	if grant := common.GetMap(data, "permission_grant"); grant != nil {
		fmt.Fprintf(w, "permission_grant.status: %s\n", common.GetString(grant, "status"))
		fmt.Fprintf(w, "permission_grant.perm: %s\n", common.GetString(grant, "perm"))
	}
}

func prettyPrintMarkdownSavedFile(w io.Writer, data map[string]interface{}) {
	fmt.Fprintf(w, "file_token: %s\n", common.GetString(data, "file_token"))
	fmt.Fprintf(w, "file_name: %s\n", common.GetString(data, "file_name"))
	fmt.Fprintf(w, "saved_path: %s\n", common.GetString(data, "saved_path"))
	fmt.Fprintf(w, "size_bytes: %d\n", int64(common.GetFloat(data, "size_bytes")))
}

func prettyPrintMarkdownContent(w io.Writer, data map[string]interface{}) {
	fmt.Fprint(w, common.GetString(data, "content"))
}

func fileNameFromDownloadHeader(header http.Header, fallback string) string {
	name := fallback
	if header != nil {
		if headerName := larkcore.FileNameByHeader(header); strings.TrimSpace(headerName) != "" {
			name = headerName
		}
	}
	name = strings.ReplaceAll(strings.TrimSpace(name), "\\", "/")
	name = path.Base(name)
	if name == "" || name == "." || name == ".." {
		return fallback
	}
	return name
}
