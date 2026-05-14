// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/util"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

const (
	baseAttachmentUploadMaxFileSize int64 = 2 * 1024 * 1024 * 1024
	baseAttachmentParentType              = "bitable_file"
	baseAttachmentMaxBatchSize            = 50
	baseAttachmentGetMaxRecords           = 10
)

var BaseRecordUploadAttachment = common.Shortcut{
	Service:     "base",
	Command:     "+record-upload-attachment",
	Description: "Upload one or more local files and append the returned file_token values to a Base attachment cell",
	Risk:        "write",
	Scopes:      []string{"base:record:update", "base:field:read", "docs:document.media:upload"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		recordRefFlag(true),
		fieldRefFlag(true),
		{Name: "file", Type: "string_array", Desc: "local file path; repeat to append multiple attachments in one cell; max 50 files, max 2GB each; files > 20MB use multipart upload automatically", Required: true},
		{Name: "name", Desc: "deprecated; attachment names are derived from local file basenames", Hidden: true},
	},
	Tips: []string{
		`Example: lark-cli base +record-upload-attachment --base-token <base_token> --table-id <table_id> --record-id <record_id> --field-id <attachment_field_id> --file ./report.pdf`,
		`Repeat --file to append multiple attachments: --file ./report.pdf --file ./screenshot.png`,
		`Reuse returned file_token values for download/remove`,
	},
	DryRun: dryRunRecordUploadAttachment,
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateRecordUploadAttachment(runtime)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeRecordUploadAttachment(runtime)
	},
}

var BaseRecordDownloadAttachment = common.Shortcut{
	Service:     "base",
	Command:     "+record-download-attachment",
	Description: "Download Base record attachments by record-id, optionally filtering by file-token",
	Risk:        "read",
	Scopes:      []string{"base:record:read", "docs:document.media:download"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		recordRefFlag(true),
		{Name: "file-token", Type: "string_array", Desc: "attachment file_token returned by Base; repeat to download selected files; omit to download all attachments in the record", Required: false},
		{Name: "output", Desc: "local save path; with exactly one file token this may be a file path; with multiple or omitted file tokens this must be an existing directory", Required: true},
		{Name: "overwrite", Type: "bool", Desc: "overwrite existing output file"},
	},
	Tips: []string{
		`Example: lark-cli base +record-download-attachment --base-token <base_token> --table-id <table_id> --record-id <record_id> --file-token <file_token> --output ./downloads/`,
		`Omit --file-token to download every attachment in the record.`,
		`Base attachments should be downloaded with this command; other download commands may fail for Base attachment files.`,
		`With one --file-token, --output may be a file path or directory; with multiple or omitted --file-token values, --output must be an existing directory.`,
	},
	DryRun: dryRunRecordDownloadAttachment,
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateRecordDownloadAttachment(runtime)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeRecordDownloadAttachment(ctx, runtime)
	},
}

var BaseRecordRemoveAttachment = common.Shortcut{
	Service:     "base",
	Command:     "+record-remove-attachment",
	Description: "Remove one or more file_token values from a Base record attachment cell",
	Risk:        "high-risk-write",
	Scopes:      []string{"base:record:update", "base:field:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		recordRefFlag(true),
		fieldRefFlag(true),
		{Name: "file-token", Type: "string_array", Desc: "attachment file_token to remove from the target cell; repeat to remove multiple attachments; max 50 tokens", Required: true},
	},
	Tips: []string{
		`Example: lark-cli base +record-remove-attachment --base-token <base_token> --table-id <table_id> --record-id <record_id> --field-id <attachment_field_id> --file-token <file_token> --yes`,
		`Repeat --file-token to remove multiple attachments from the same cell in one call.`,
		`This is a high-risk write command and requires --yes.`,
	},
	DryRun: dryRunRecordRemoveAttachment,
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateRecordRemoveAttachment(runtime)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeRecordRemoveAttachment(runtime)
	},
}

func dryRunRecordUploadAttachment(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	files := runtime.StrArray("file")
	filePath := "<file>"
	fileName := "<local_file_name>"
	if len(files) > 0 {
		filePath = files[0]
		fileName = filepath.Base(filePath)
	}
	dry := common.NewDryRunAPI().
		Desc("3-step orchestration: validate attachment field → upload local file(s) to Base → append uploaded file token(s) to the attachment cell").
		GET("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields/:field_id").
		Desc("[1] Read target field and ensure it is an attachment field").
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime)).
		Set("field_id", runtime.Str("field-id"))
	if baseAttachmentShouldUseMultipart(runtime.FileIO(), filePath) {
		dry.POST("/open-apis/drive/v1/medias/upload_prepare").
			Desc("[2a] Initialize multipart attachment upload to the current Base").
			Body(map[string]interface{}{
				"file_name":   fileName,
				"parent_type": baseAttachmentParentType,
				"parent_node": runtime.Str("base-token"),
				"size":        "<file_size>",
			}).
			POST("/open-apis/drive/v1/medias/upload_part").
			Desc("[2b] Upload attachment parts (repeated for each large file)").
			Body(map[string]interface{}{
				"upload_id": "<upload_id>",
				"seq":       "<chunk_index>",
				"size":      "<chunk_size>",
				"file":      "<chunk_binary>",
			}).
			POST("/open-apis/drive/v1/medias/upload_finish").
			Desc("[2c] Finalize multipart attachment upload and get file token").
			Body(map[string]interface{}{
				"upload_id": "<upload_id>",
				"block_num": "<block_num>",
			})
	} else {
		dry.POST("/open-apis/drive/v1/medias/upload_all").
			Desc("[2] Upload local file(s) to the current Base as attachment media (multipart/form-data)").
			Body(map[string]interface{}{
				"file_name":   fileName,
				"parent_type": baseAttachmentParentType,
				"parent_node": runtime.Str("base-token"),
				"file":        "@" + filePath,
				"size":        "<file_size>",
			})
	}
	return dry.
		POST("/open-apis/base/v3/bases/:base_token/tables/:table_id/append_attachments").
		Desc("[3] Append uploaded file token(s) to the target attachment cell").
		Body(map[string]interface{}{
			"attachments": map[string]interface{}{
				runtime.Str("record-id"): map[string]interface{}{
					runtime.Str("field-id"): []interface{}{
						map[string]interface{}{
							"file_token":   "<uploaded_file_token>",
							"image_width":  "<image_width_if_image>",
							"image_height": "<image_height_if_image>",
						},
					},
				},
			},
		})
}

func dryRunRecordDownloadAttachment(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		Desc("2-step orchestration: read Base attachment metadata → download each requested attachment file").
		POST("/open-apis/base/v3/bases/:base_token/tables/:table_id/get_attachments").
		Desc("[1] Read attachment metadata for the record").
		Body(map[string]interface{}{"record_id_list": []string{runtime.Str("record-id")}}).
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime)).
		GET("/open-apis/drive/v1/medias/:file_token/download").
		Desc("[2] Download attachment media through the Base attachment flow").
		Set("file_token", "<file_token>").
		Set("output", runtime.Str("output")).
		Params(map[string]interface{}{"extra": "<extra_info_if_present>"})
}

func dryRunRecordRemoveAttachment(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	body := buildSingleCellAttachmentsBody(runtime.Str("record-id"), runtime.Str("field-id"), fileTokenPatchItems(runtime.StrArray("file-token")))
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/tables/:table_id/remove_attachments").
		Desc("Remove attachment file token(s) from the target attachment cell").
		Body(body).
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime))
}

func validateRecordUploadAttachment(runtime *common.RuntimeContext) error {
	if runtime.Changed("name") {
		return common.FlagErrorf("--name is no longer supported; uploaded attachment names are derived from local file basenames")
	}
	files, err := normalizeAttachmentFiles(runtime.StrArray("file"))
	if err != nil {
		return err
	}
	for _, path := range files {
		if _, err := validateAttachmentInputFile(runtime, path); err != nil {
			return err
		}
	}
	return nil
}

func validateRecordDownloadAttachment(runtime *common.RuntimeContext) error {
	tokens, err := normalizeOptionalDownloadAttachmentFileTokens(runtime.StrArray("file-token"))
	if err != nil {
		return err
	}
	if len(tokens) != 1 {
		info, statErr := runtime.FileIO().Stat(runtime.Str("output"))
		if statErr != nil || !info.IsDir() {
			return common.FlagErrorf("--output must be an existing directory when downloading multiple attachments or when --file-token is omitted")
		}
	}
	return nil
}

func validateRecordRemoveAttachment(runtime *common.RuntimeContext) error {
	_, err := normalizeAttachmentFileTokens(runtime.StrArray("file-token"))
	return err
}

func executeRecordUploadAttachment(runtime *common.RuntimeContext) error {
	files, err := normalizeAttachmentFiles(runtime.StrArray("file"))
	if err != nil {
		return err
	}

	field, err := fetchBaseField(runtime, runtime.Str("base-token"), baseTableID(runtime), runtime.Str("field-id"))
	if err != nil {
		return err
	}
	if normalized := normalizeFieldTypeName(fieldTypeName(field)); normalized != "attachment" {
		return output.ErrValidation("field %q is type %q, expected attachment", fieldName(field), normalized)
	}
	resolvedFieldID := fieldID(field)
	if resolvedFieldID == "" {
		resolvedFieldID = runtime.Str("field-id")
	}

	appendItems := make([]interface{}, 0, len(files))
	for _, filePath := range files {
		fileInfo, err := validateAttachmentInputFile(runtime, filePath)
		if err != nil {
			return err
		}
		fileName := filepath.Base(filePath)
		fmt.Fprintf(runtime.IO().ErrOut, "Uploading attachment: %s -> record %s field %s\n", fileName, runtime.Str("record-id"), fieldName(field))
		if fileInfo.Size() > common.MaxDriveMediaUploadSinglePartSize {
			fmt.Fprintf(runtime.IO().ErrOut, "File exceeds 20MB, using multipart upload\n")
		}
		attachment, err := uploadAttachmentToBase(runtime, filePath, fileName, runtime.Str("base-token"), fileInfo.Size())
		if err != nil {
			return err
		}
		appendItems = append(appendItems, attachmentAppendItem(attachment))
	}

	body := buildSingleCellAttachmentsBody(runtime.Str("record-id"), resolvedFieldID, appendItems)
	data, err := baseV3Call(runtime, "POST", baseV3Path("bases", runtime.Str("base-token"), "tables", baseTableID(runtime), "append_attachments"), nil, body)
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}

func executeRecordRemoveAttachment(runtime *common.RuntimeContext) error {
	tokens, err := normalizeAttachmentFileTokens(runtime.StrArray("file-token"))
	if err != nil {
		return err
	}
	field, err := fetchBaseField(runtime, runtime.Str("base-token"), baseTableID(runtime), runtime.Str("field-id"))
	if err != nil {
		return err
	}
	if normalized := normalizeFieldTypeName(fieldTypeName(field)); normalized != "attachment" {
		return output.ErrValidation("field %q is type %q, expected attachment", fieldName(field), normalized)
	}
	resolvedFieldID := fieldID(field)
	if resolvedFieldID == "" {
		resolvedFieldID = runtime.Str("field-id")
	}
	body := buildSingleCellAttachmentsBody(runtime.Str("record-id"), resolvedFieldID, fileTokenPatchItems(tokens))
	data, err := baseV3Call(runtime, "POST", baseV3Path("bases", runtime.Str("base-token"), "tables", baseTableID(runtime), "remove_attachments"), nil, body)
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}

func executeRecordDownloadAttachment(ctx context.Context, runtime *common.RuntimeContext) error {
	tokens, err := normalizeOptionalDownloadAttachmentFileTokens(runtime.StrArray("file-token"))
	if err != nil {
		return err
	}
	attachments, err := fetchBaseAttachments(runtime, runtime.Str("base-token"), baseTableID(runtime), []string{runtime.Str("record-id")})
	if err != nil {
		return err
	}
	items, err := selectAttachmentDownloadItems(attachments, runtime.Str("record-id"), tokens)
	if err != nil {
		return err
	}
	targets, err := planAttachmentDownloadTargets(runtime, items, runtime.Str("output"), len(tokens) != 1 || len(items) > 1, runtime.Bool("overwrite"))
	if err != nil {
		return err
	}
	downloaded := make([]map[string]interface{}, 0, len(targets))
	for _, target := range targets {
		saved, err := downloadBaseAttachment(ctx, runtime, target.Item, target.TargetPath, runtime.Bool("overwrite"))
		if err != nil {
			failed := attachmentDownloadFailure(target, err)
			return attachmentDownloadProgressError(err, downloaded, []map[string]interface{}{failed})
		}
		downloaded = append(downloaded, saved)
	}
	runtime.Out(map[string]interface{}{"downloaded": downloaded}, nil)
	return nil
}

func validateAttachmentInputFile(runtime *common.RuntimeContext, filePath string) (fileio.FileInfo, error) {
	fio := runtime.FileIO()
	if fio == nil {
		return nil, output.ErrValidation("file operations require a FileIO provider")
	}
	fileInfo, err := fio.Stat(filePath)
	if err != nil {
		if errors.Is(err, fileio.ErrPathValidation) {
			return nil, output.ErrValidation("unsafe file path: %s", err)
		}
		return nil, output.ErrValidation("file not accessible: %s: %v", filePath, err)
	}
	if fileInfo.IsDir() {
		return nil, output.ErrValidation("file path is a directory: %s", filePath)
	}
	if fileInfo.Size() > baseAttachmentUploadMaxFileSize {
		return nil, output.ErrValidation("file %s exceeds 2GB limit", common.FormatSize(fileInfo.Size()))
	}
	return fileInfo, nil
}

func normalizeAttachmentFiles(files []string) ([]string, error) {
	return normalizeStringList(files, stringListNormalizeOptions{
		typeError:     "attachment files must be a string array",
		emptyError:    "provide at least one --file",
		itemName:      "attachment file",
		duplicateName: "attachment file",
		limitName:     "attachment file count",
		max:           baseAttachmentMaxBatchSize,
	})
}

func normalizeAttachmentFileTokens(tokens []string) ([]string, error) {
	return normalizeStringList(tokens, stringListNormalizeOptions{
		typeError:     "attachment file tokens must be a string array",
		emptyError:    "provide at least one --file-token",
		itemName:      "attachment file token",
		duplicateName: "attachment file token",
		limitName:     "attachment file token count",
		max:           baseAttachmentMaxBatchSize,
	})
}

func normalizeOptionalDownloadAttachmentFileTokens(tokens []string) ([]string, error) {
	if len(tokens) == 0 {
		return nil, nil
	}
	normalized := make([]string, 0, len(tokens))
	for index, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			return nil, common.FlagErrorf("attachment file token %d must not be empty", index+1)
		}
		normalized = append(normalized, token)
	}
	normalized = dedupeStringsPreserveOrder(normalized)
	if len(normalized) > baseAttachmentMaxBatchSize {
		return nil, common.FlagErrorf("attachment file token count exceeds maximum limit of %d (got %d)", baseAttachmentMaxBatchSize, len(normalized))
	}
	return normalized, nil
}

func dedupeStringsPreserveOrder(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func baseAttachmentShouldUseMultipart(fio fileio.FileIO, filePath string) bool {
	if fio == nil {
		return false
	}
	info, err := fio.Stat(filePath)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular() && info.Size() > common.MaxDriveMediaUploadSinglePartSize
}

func fetchBaseField(runtime *common.RuntimeContext, baseToken, tableIDValue, fieldRef string) (map[string]interface{}, error) {
	return baseV3Call(runtime, "GET", baseV3Path("bases", baseToken, "tables", tableIDValue, "fields", fieldRef), nil, nil)
}

func fetchBaseAttachments(runtime *common.RuntimeContext, baseToken, tableIDValue string, recordIDs []string) (map[string]interface{}, error) {
	if len(recordIDs) == 0 {
		return nil, output.ErrValidation("provide at least one record id")
	}
	if len(recordIDs) > baseAttachmentGetMaxRecords {
		return nil, output.ErrValidation("get attachments record selection exceeds maximum limit of %d (got %d)", baseAttachmentGetMaxRecords, len(recordIDs))
	}
	data, err := baseV3Call(runtime, "POST", baseV3Path("bases", baseToken, "tables", tableIDValue, "get_attachments"), nil, map[string]interface{}{
		"record_id_list": recordIDs,
	})
	if err != nil {
		return nil, err
	}
	attachments, _ := data["attachments"].(map[string]interface{})
	if attachments == nil {
		return map[string]interface{}{}, nil
	}
	return attachments, nil
}

func uploadAttachmentToBase(runtime *common.RuntimeContext, filePath, fileName, baseToken string, fileSize int64) (map[string]interface{}, error) {
	mimeType, err := detectAttachmentMIMEType(runtime.FileIO(), filePath, fileName)
	if err != nil {
		return nil, err
	}

	parentNode := baseToken
	var (
		fileToken string
	)
	if fileSize <= common.MaxDriveMediaUploadSinglePartSize {
		fileToken, err = common.UploadDriveMediaAll(runtime, common.DriveMediaUploadAllConfig{
			FilePath:   filePath,
			FileName:   fileName,
			FileSize:   fileSize,
			ParentType: baseAttachmentParentType,
			ParentNode: &parentNode,
		})
	} else {
		fileToken, err = common.UploadDriveMediaMultipart(runtime, common.DriveMediaMultipartUploadConfig{
			FilePath:   filePath,
			FileName:   fileName,
			FileSize:   fileSize,
			ParentType: baseAttachmentParentType,
			ParentNode: parentNode,
		})
	}
	if err != nil {
		return nil, err
	}

	attachment := map[string]interface{}{
		"file_token": fileToken,
		"name":       fileName,
		"mime_type":  mimeType,
		"size":       fileSize,
	}
	if width, height, ok := detectAttachmentImageDimensions(runtime.FileIO(), filePath, mimeType); ok {
		attachment["image_width"] = width
		attachment["image_height"] = height
	} else if strings.HasPrefix(mimeType, "image/") {
		fmt.Fprintf(runtime.IO().ErrOut, "Warning: image dimensions unavailable for %s; attachment may display as square\n", fileName)
	}
	return attachment, nil
}

func attachmentAppendItem(attachment map[string]interface{}) map[string]interface{} {
	item := map[string]interface{}{
		"file_token": attachment["file_token"],
	}
	if width, ok := attachment["image_width"]; ok && !util.IsNil(width) {
		item["image_width"] = width
	}
	if height, ok := attachment["image_height"]; ok && !util.IsNil(height) {
		item["image_height"] = height
	}
	return item
}

func fileTokenPatchItems(tokens []string) []interface{} {
	items := make([]interface{}, 0, len(tokens))
	for _, token := range tokens {
		items = append(items, map[string]interface{}{"file_token": token})
	}
	return items
}

func buildSingleCellAttachmentsBody(recordID, fieldID string, items []interface{}) map[string]interface{} {
	return map[string]interface{}{
		"attachments": map[string]interface{}{
			recordID: map[string]interface{}{
				fieldID: items,
			},
		},
	}
}

func detectAttachmentMIMEType(fio fileio.FileIO, filePath, fileName string) (string, error) {
	if byExt := strings.TrimSpace(mime.TypeByExtension(strings.ToLower(filepath.Ext(fileName)))); byExt != "" {
		return stripMIMEParams(byExt), nil
	}
	if byExt := strings.TrimSpace(mime.TypeByExtension(strings.ToLower(filepath.Ext(filePath)))); byExt != "" {
		return stripMIMEParams(byExt), nil
	}

	f, err := fio.Open(filePath)
	if err != nil {
		return "", common.WrapInputStatError(err)
	}
	defer f.Close()

	buf := make([]byte, 512)
	n, readErr := f.Read(buf)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return "", output.ErrValidation("cannot read file: %s", readErr)
	}
	return detectAttachmentMIMEFromContent(buf[:n]), nil
}

func detectAttachmentImageDimensions(fio fileio.FileIO, filePath string, mimeType string) (int, int, bool) {
	if fio == nil || !strings.HasPrefix(mimeType, "image/") {
		return 0, 0, false
	}
	f, err := fio.Open(filePath)
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0, false
	}
	return cfg.Width, cfg.Height, true
}

type baseAttachmentDownloadItem struct {
	RecordID   string
	FieldID    string
	FileToken  string
	Name       string
	Size       interface{}
	ExtraInfo  string
	MimeType   string
	RawPayload map[string]interface{}
}

type baseAttachmentDownloadTarget struct {
	Item         baseAttachmentDownloadItem
	TargetPath   string
	ResolvedPath string
}

func selectAttachmentDownloadItems(attachments map[string]interface{}, recordID string, tokens []string) ([]baseAttachmentDownloadItem, error) {
	recordRaw, ok := attachments[recordID]
	if !ok {
		return nil, output.ErrValidation("record %q has no attachment metadata; verify the record-id", recordID)
	}
	fields, ok := recordRaw.(map[string]interface{})
	if !ok {
		return nil, output.ErrValidation("record %q attachment metadata has unexpected type %T", recordID, recordRaw)
	}
	byToken := map[string]baseAttachmentDownloadItem{}
	fieldIDs := make([]string, 0, len(fields))
	for currentFieldID := range fields {
		fieldIDs = append(fieldIDs, currentFieldID)
	}
	sort.Strings(fieldIDs)
	for _, currentFieldID := range fieldIDs {
		rawList := fields[currentFieldID]
		items, ok := rawList.([]interface{})
		if !ok {
			return nil, output.ErrValidation("record %q field %q attachment metadata has unexpected type %T", recordID, currentFieldID, rawList)
		}
		for _, rawItem := range items {
			item, ok := rawItem.(map[string]interface{})
			if !ok {
				return nil, output.ErrValidation("record %q field %q contains unexpected attachment item type %T", recordID, currentFieldID, rawItem)
			}
			fileToken, _ := item["file_token"].(string)
			if fileToken == "" {
				continue
			}
			if _, exists := byToken[fileToken]; exists {
				continue
			}
			name, _ := item["name"].(string)
			extraInfo, _ := item["extra_info"].(string)
			mimeType, _ := item["mime_type"].(string)
			byToken[fileToken] = baseAttachmentDownloadItem{
				RecordID:   recordID,
				FieldID:    currentFieldID,
				FileToken:  fileToken,
				Name:       name,
				Size:       item["size"],
				ExtraInfo:  extraInfo,
				MimeType:   mimeType,
				RawPayload: item,
			}
		}
	}
	result := make([]baseAttachmentDownloadItem, 0, len(tokens))
	if len(tokens) == 0 {
		for _, item := range byToken {
			result = append(result, item)
		}
		if len(result) == 0 {
			return nil, output.ErrValidation("record %q has no attachments to download", recordID)
		}
		sort.SliceStable(result, func(i, j int) bool {
			leftName := strings.ToLower(baseAttachmentDownloadName(result[i]))
			rightName := strings.ToLower(baseAttachmentDownloadName(result[j]))
			if leftName != rightName {
				return leftName < rightName
			}
			return result[i].FileToken < result[j].FileToken
		})
		return result, nil
	}
	for _, token := range tokens {
		item, ok := byToken[token]
		if !ok {
			return nil, output.ErrValidation("attachment file_token %q not found in record %q; verify the record-id/file-token pair", token, recordID)
		}
		result = append(result, item)
	}
	return result, nil
}

func planAttachmentDownloadTargets(runtime *common.RuntimeContext, items []baseAttachmentDownloadItem, outputPath string, outputIsDir bool, overwrite bool) ([]baseAttachmentDownloadTarget, error) {
	names := downloadTargetNames(items, outputIsDir || outputPathLooksDirectory(runtime, outputPath))
	targets := make([]baseAttachmentDownloadTarget, 0, len(items))
	seen := map[string]baseAttachmentDownloadItem{}
	for _, item := range items {
		targetName := names[item.FileToken]
		targetPath := outputPath
		if targetName != "" {
			targetPath = filepath.Join(outputPath, targetName)
		}
		resolved, err := runtime.ResolveSavePath(targetPath)
		if err != nil {
			return nil, output.ErrValidation("unsafe output path: %s", err)
		}
		if previous, exists := seen[resolved]; exists {
			return nil, output.ErrValidation("multiple attachments resolve to the same output path %q (%s and %s); download them separately or choose a different directory", resolved, previous.FileToken, item.FileToken)
		}
		seen[resolved] = item
		if !overwrite {
			if _, statErr := runtime.FileIO().Stat(targetPath); statErr == nil {
				return nil, output.ErrValidation("output file already exists: %s (use --overwrite to replace)", targetPath)
			}
		}
		targets = append(targets, baseAttachmentDownloadTarget{
			Item:         item,
			TargetPath:   targetPath,
			ResolvedPath: resolved,
		})
	}
	return targets, nil
}

func downloadTargetNames(items []baseAttachmentDownloadItem, outputIsDir bool) map[string]string {
	if !outputIsDir {
		return nil
	}
	nameCounts := make(map[string]int, len(items))
	for _, item := range items {
		nameCounts[baseAttachmentDownloadName(item)]++
	}
	names := make(map[string]string, len(items))
	for _, item := range items {
		name := baseAttachmentDownloadName(item)
		if nameCounts[name] > 1 {
			name = attachmentNameWithTokenSuffix(name, item.FileToken)
		}
		names[item.FileToken] = name
	}
	return names
}

func baseAttachmentDownloadName(item baseAttachmentDownloadItem) string {
	name := filepath.Base(strings.TrimSpace(item.Name))
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = item.FileToken
	}
	return name
}

func attachmentNameWithTokenSuffix(name, fileToken string) string {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	if stem == "" {
		stem = name
	}
	return stem + "_" + safeAttachmentFileTokenSuffix(fileToken) + ext
}

func safeAttachmentFileTokenSuffix(fileToken string) string {
	var b strings.Builder
	for _, r := range fileToken {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	suffix := strings.Trim(b.String(), "_")
	if suffix == "" {
		return "file"
	}
	return suffix
}

func downloadBaseAttachment(ctx context.Context, runtime *common.RuntimeContext, item baseAttachmentDownloadItem, targetPath string, overwrite bool) (map[string]interface{}, error) {
	if _, err := runtime.ResolveSavePath(targetPath); err != nil {
		return nil, output.ErrValidation("unsafe output path: %s", err)
	}

	query := larkcore.QueryParams{}
	if item.ExtraInfo != "" {
		query.Set("extra", item.ExtraInfo)
	}
	resp, err := runtime.DoAPIStream(ctx, &larkcore.ApiReq{
		HttpMethod:  http.MethodGet,
		ApiPath:     fmt.Sprintf("/open-apis/drive/v1/medias/%s/download", validate.EncodePathSegment(item.FileToken)),
		QueryParams: query,
	})
	if err != nil {
		return nil, output.ErrNetwork("download failed: %v", err)
	}
	defer resp.Body.Close()

	if !overwrite {
		if _, statErr := runtime.FileIO().Stat(targetPath); statErr == nil {
			return nil, output.ErrValidation("output file already exists: %s (use --overwrite to replace)", targetPath)
		}
	}
	result, err := runtime.FileIO().Save(targetPath, fileio.SaveOptions{
		ContentType:   resp.Header.Get("Content-Type"),
		ContentLength: resp.ContentLength,
	}, resp.Body)
	if err != nil {
		return nil, common.WrapSaveErrorByCategory(err, "io")
	}
	savedPath, _ := runtime.ResolveSavePath(targetPath)
	if savedPath == "" {
		savedPath = targetPath
	}
	return map[string]interface{}{
		"record_id":    item.RecordID,
		"field_id":     item.FieldID,
		"file_token":   item.FileToken,
		"name":         item.Name,
		"size":         item.Size,
		"saved_path":   savedPath,
		"size_bytes":   result.Size(),
		"content_type": resp.Header.Get("Content-Type"),
	}, nil
}

func attachmentDownloadFailure(target baseAttachmentDownloadTarget, err error) map[string]interface{} {
	return map[string]interface{}{
		"record_id":     target.Item.RecordID,
		"field_id":      target.Item.FieldID,
		"file_token":    target.Item.FileToken,
		"name":          target.Item.Name,
		"target_path":   target.TargetPath,
		"resolved_path": target.ResolvedPath,
		"error":         err.Error(),
	}
}

func attachmentDownloadProgressError(err error, downloaded []map[string]interface{}, failed []map[string]interface{}) error {
	msg := fmt.Sprintf("download failed after %d attachment(s) succeeded and %d failed: %v", len(downloaded), len(failed), err)
	var exitErr *output.ExitError
	if errors.As(err, &exitErr) && exitErr.Detail != nil {
		return &output.ExitError{
			Code: exitErr.Code,
			Detail: &output.ErrDetail{
				Type:    exitErr.Detail.Type,
				Code:    exitErr.Detail.Code,
				Message: msg,
				Hint:    "Some files may already have been saved. Inspect error.detail.downloaded before retrying, or rerun with --overwrite if the failed target now exists.",
				Detail: map[string]interface{}{
					"downloaded": downloaded,
					"failed":     failed,
				},
			},
			Err: err,
		}
	}
	return &output.ExitError{
		Code: output.ExitInternal,
		Detail: &output.ErrDetail{
			Type:    "io",
			Message: msg,
			Hint:    "Some files may already have been saved. Inspect error.detail.downloaded before retrying, or rerun with --overwrite if the failed target now exists.",
			Detail: map[string]interface{}{
				"downloaded": downloaded,
				"failed":     failed,
			},
		},
		Err: err,
	}
}

func outputPathLooksDirectory(runtime *common.RuntimeContext, outputPath string) bool {
	if strings.HasSuffix(outputPath, "/") || strings.HasSuffix(outputPath, string(filepath.Separator)) {
		return true
	}
	info, err := runtime.FileIO().Stat(outputPath)
	return err == nil && info.IsDir()
}

func stripMIMEParams(value string) string {
	if i := strings.IndexByte(value, ';'); i != -1 {
		value = value[:i]
	}
	return strings.TrimSpace(value)
}

func detectAttachmentMIMEFromContent(content []byte) string {
	if len(content) == 0 {
		return "application/octet-stream"
	}
	if bytes.HasPrefix(content, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		return "image/png"
	}
	if bytes.HasPrefix(content, []byte{0xff, 0xd8, 0xff}) {
		return "image/jpeg"
	}
	if bytes.HasPrefix(content, []byte("GIF87a")) || bytes.HasPrefix(content, []byte("GIF89a")) {
		return "image/gif"
	}
	if len(content) >= 12 && bytes.Equal(content[:4], []byte("RIFF")) && bytes.Equal(content[8:12], []byte("WEBP")) {
		return "image/webp"
	}
	if bytes.HasPrefix(content, []byte("%PDF-")) {
		return "application/pdf"
	}
	if looksLikeText(content) {
		return "text/plain"
	}
	return "application/octet-stream"
}

func looksLikeText(content []byte) bool {
	if !utf8.Valid(content) {
		return false
	}
	for _, r := range string(content) {
		if r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}
