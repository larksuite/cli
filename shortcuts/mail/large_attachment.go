// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/mail/emlbuilder"
)

// attachmentFile holds metadata about a local file to be attached.
type attachmentFile struct {
	Path     string // relative file path as provided by the user
	FileName string // basename
	Size     int64  // raw file size in bytes
}

// classifiedAttachments is the result of classifyAttachments.
type classifiedAttachments struct {
	Normal   []attachmentFile    // to be embedded in the EML
	Oversized []attachmentFile   // to be uploaded as large attachments
}

// largeAttachmentResult holds the upload result for a single large attachment.
type largeAttachmentResult struct {
	FileName  string
	FileSize  int64
	FileToken string
}

// estimateBase64EMLSize estimates the EML byte cost of embedding a raw file.
// base64 inflates 3 bytes → 4 chars, plus ~200 bytes for MIME part headers.
const base64MIMEOverhead = 200

func estimateBase64EMLSize(rawSize int64) int64 {
	return (rawSize*4+2)/3 + base64MIMEOverhead
}

// estimateEMLBaseSize estimates the EML size consumed by non-attachment content:
// headers (~2KB), body text/HTML, and inline images. Each component is
// accounted for with base64 encoding overhead where applicable.
//
// Parameters:
//   - bodySize: raw size of the text or HTML body in bytes
//   - inlineFilePaths: paths of inline image files (will be stat'd for size)
//   - extraBytes: any additional pre-computed EML bytes (e.g. downloaded
//     original attachments already loaded in memory for forward)
func estimateEMLBaseSize(fio fileio.FileIO, bodySize int64, inlineFilePaths []string, extraBytes int64) int64 {
	const headerOverhead = 2048 // generous estimate for all headers + MIME structure
	total := int64(headerOverhead) + estimateBase64EMLSize(bodySize) + extraBytes
	for _, p := range inlineFilePaths {
		if info, err := fio.Stat(p); err == nil {
			total += estimateBase64EMLSize(info.Size())
		}
	}
	return total
}

// classifyAttachments splits files into normal (embed in EML) and oversized
// (upload separately as large attachments).
//
// The decision is based on the estimated total EML size: headers + body +
// inline images + attachments, all base64-encoded. Files are processed in
// the user-specified order. Once a file would push the EML over MaxEMLSize,
// it and all subsequent files are classified as oversized.
func classifyAttachments(files []attachmentFile, emlBaseSize int64) classifiedAttachments {
	var result classifiedAttachments
	accumulated := emlBaseSize
	overflow := false

	for _, f := range files {
		if overflow {
			result.Oversized = append(result.Oversized, f)
			continue
		}
		cost := estimateBase64EMLSize(f.Size)
		if accumulated+cost > emlbuilder.MaxEMLSize {
			overflow = true
			result.Oversized = append(result.Oversized, f)
			continue
		}
		accumulated += cost
		result.Normal = append(result.Normal, f)
	}
	return result
}

// statAttachmentFiles stats each path and returns attachmentFile metadata.
func statAttachmentFiles(fio fileio.FileIO, paths []string) ([]attachmentFile, error) {
	files := make([]attachmentFile, 0, len(paths))
	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			continue
		}
		info, err := fio.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("failed to stat attachment %s: %w", p, err)
		}
		files = append(files, attachmentFile{
			Path:     p,
			FileName: filepath.Base(p),
			Size:     info.Size(),
		})
	}
	return files, nil
}

// uploadLargeAttachments uploads oversized files to the mail attachment storage
// via the medias/upload_* API with parent_type="email".
func uploadLargeAttachments(ctx context.Context, runtime *common.RuntimeContext, files []attachmentFile) ([]largeAttachmentResult, error) {
	if len(files) == 0 {
		return nil, nil
	}
	userOpenId := runtime.UserOpenId()
	if userOpenId == "" {
		return nil, fmt.Errorf("large attachment upload requires user identity (user open_id not available)")
	}

	results := make([]largeAttachmentResult, 0, len(files))
	for _, f := range files {
		fmt.Fprintf(runtime.IO().ErrOut, "Uploading large attachment: %s (%s)\n", f.FileName, common.FormatSize(f.Size))

		var (
			fileToken string
			err       error
		)
		if f.Size <= common.MaxDriveMediaUploadSinglePartSize {
			fileToken, err = common.UploadDriveMediaAll(runtime, common.DriveMediaUploadAllConfig{
				FilePath:   f.Path,
				FileName:   f.FileName,
				FileSize:   f.Size,
				ParentType: "email",
				ParentNode: &userOpenId,
			})
		} else {
			fileToken, err = common.UploadDriveMediaMultipart(runtime, common.DriveMediaMultipartUploadConfig{
				FilePath:   f.Path,
				FileName:   f.FileName,
				FileSize:   f.Size,
				ParentType: "email",
				ParentNode: userOpenId,
			})
		}
		if err != nil {
			return nil, fmt.Errorf("failed to upload large attachment %s: %w", f.FileName, err)
		}

		results = append(results, largeAttachmentResult{
			FileName:  f.FileName,
			FileSize:  f.Size,
			FileToken: fileToken,
		})
	}
	return results, nil
}

// buildLargeAttachmentPreviewURL builds the download/preview URL for a large
// attachment token. The domain is derived from the CLI's configured endpoint
// (e.g. open.feishu.cn → www.feishu.cn).
func buildLargeAttachmentPreviewURL(brand core.LarkBrand, fileToken string) string {
	ep := core.ResolveEndpoints(brand)
	host := strings.TrimPrefix(ep.Open, "https://")
	host = strings.TrimPrefix(host, "http://")
	mainDomain := strings.TrimPrefix(host, "open.")
	return "https://www." + mainDomain + "/mail/page/attachment?token=" + url.QueryEscape(fileToken)
}

// buildLargeAttachmentHTML generates the HTML block for large attachments,
// matching the desktop client's exportLargeFileArea style.
//
// Reference: mail-editor/src/plugins/bigAttachment/export.ts
func buildLargeAttachmentHTML(brand core.LarkBrand, results []largeAttachmentResult) string {
	if len(results) == 0 {
		return ""
	}

	timestamp := fmt.Sprintf("%d", time.Now().UnixMilli())
	if len(timestamp) > 10 {
		timestamp = timestamp[:10]
	}

	// HTML template matches desktop's exportLargeFileArea (mail-editor/src/plugins/bigAttachment/export.ts).
	// Colors: primary-content-default = rgb(20, 86, 240), primary-fill-solid-02 = rgb(224, 233, 255).
	// Icon CDN: lf-larkemail.bytetos.com (CN) / sf16-sg.tiktokcdn.com (overseas).
	// Layout uses float (not flexbox) for email client compatibility.
	isOversea := brand == core.BrandLark
	iconCDN := "https://lf-larkemail.bytetos.com/obj/eden-cn/aultojhaah_npi_spht_ryhs/ljhwZthlaukjlkulzlp/"
	if isOversea {
		iconCDN = "https://sf16-sg.tiktokcdn.com/obj/eden-sg/aultojhaah_npi_spht_ryhs/ljhwZthlaukjlkulzlp/"
	}

	var items strings.Builder
	for _, att := range results {
		previewLink := buildLargeAttachmentPreviewURL(brand, att.FileToken)
		sizeText := common.FormatSize(att.FileSize)
		iconURL := iconCDN + fileTypeIcon(att.FileName)

		// Each item — matches desktop template structure exactly
		fmt.Fprintf(&items, `<div style="border-top: solid 1px #DEE0E3;padding: 12px;box-sizing: border-box;clear: both;overflow: hidden;display: flex;" id="lark-mail-large-file-item">`)
		fmt.Fprintf(&items, `<div style="float: left; margin-right: 8px; margin-top: 1px; margin-bottom: 1px;">`)
		fmt.Fprintf(&items, `<img src="%s" height="40" width="40" style="height: 40px;width: 40px;"/>`, htmlEscape(iconURL))
		fmt.Fprintf(&items, `</div>`)
		fmt.Fprintf(&items, `<div style="overflow: hidden;text-overflow: ellipsis;display: inline-block;width: 290px;float:left; margin-right: 10px;">`)
		fmt.Fprintf(&items, `<div style="white-space: nowrap; overflow: hidden; text-overflow: ellipsis;font-size: 14px;line-height: 22px;color: #1f2329">%s</div>`, htmlEscape(att.FileName))
		fmt.Fprintf(&items, `<div style="font-size: 12px; line-height: 20px; color: #8f959e; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;">`)
		fmt.Fprintf(&items, `<span style="color: #8f959e;vertical-align: middle;">%s</span>`, htmlEscape(sizeText))
		fmt.Fprintf(&items, `</div>`)
		fmt.Fprintf(&items, `</div>`)
		fmt.Fprintf(&items, `<a href="%s" data-mail-token="%s" style="margin: 10px; text-decoration: none; color: rgb(20, 86, 240); white-space: nowrap; cursor: pointer; line-height: 1.5; float: right; text-align: right; font-size: 14px;">Download</a>`,
			htmlEscape(previewLink), htmlEscape(att.FileToken))
		fmt.Fprintf(&items, `</div>`)
	}

	var buf strings.Builder
	fmt.Fprintf(&buf, `<div id="lark-mail-large-file-container-%s" style="border: 1px solid #DEE0E3; margin-bottom: 20px;max-width: 400px; min-width: 160px; border-radius: 8px;">`, timestamp)
	fmt.Fprintf(&buf, `<div style="font-weight: 500; font-size: 16px;line-height: 24px; padding: 8px 16px;background-color: rgb(224, 233, 255); border-top-left-radius: 8px;border-top-right-radius: 8px;">`)
	buf.WriteString("Attachments from Lark Mail")
	buf.WriteString(`</div>`)
	buf.WriteString(items.String())
	buf.WriteString(`</div>`)
	return buf.String()
}

// fileTypeIcon returns the CDN icon filename for a given attachment filename,
// matching desktop's AttachmentIconPath (mail-editor/src/plugins/bigAttachment/utils.ts).
func fileTypeIcon(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if len(ext) > 0 {
		ext = ext[1:] // strip leading dot
	}
	switch ext {
	case "doc", "docx":
		return "icon_file_doc.png"
	case "pdf":
		return "icon_file_pdf.png"
	case "ppt", "pptx":
		return "icon_file_ppt.png"
	case "xls", "xlsx":
		return "icon_file_excel.png"
	case "zip", "rar", "7z", "tar", "gz":
		return "icon_file_zip.png"
	case "png", "jpg", "jpeg", "gif", "bmp", "webp", "svg", "ico", "tiff":
		return "icon_file_image.png"
	case "mp4", "avi", "mov", "mkv", "wmv", "flv":
		return "icon_file_video.png"
	case "mp3", "wav", "flac", "aac", "ogg", "wma":
		return "icon_file_audio.png"
	case "txt":
		return "icon_file_doc.png"
	case "eml":
		return "icon_file_eml.png"
	case "apk":
		return "icon_file_android.png"
	case "psd":
		return "icon_file_ps.png"
	case "ai":
		return "icon_file_ai.png"
	case "sketch":
		return "icon_file_sketch.png"
	case "key", "keynote":
		return "icon_file_keynote.png"
	case "numbers":
		return "icon_file_numbers.png"
	case "pages":
		return "icon_file_pages.png"
	default:
		return "icon_file_unknow.png"
	}
}

// processLargeAttachments is the unified entry point for large attachment
// handling across all mail compose shortcuts (draft-create, reply, forward).
//
// It replaces the previous pattern of:
//   checkAttachmentSizeLimit → AddFileAttachment loop
//
// with:
//   processLargeAttachments → add normal via AddFileAttachment + inject HTML for oversized
//
// Parameters:
//   - runtime: shortcut runtime context
//   - bld: the EML builder (with body and inline images already set)
//   - attachPaths: user-specified attachment file paths (from --attach flag)
//   - extraEMLBytes: EML bytes already accounted for (e.g. downloaded original
//     attachments in forward, estimated body+header size). Callers should
//     pass the sum of base64-encoded sizes of any content already added to bld.
//   - extraAttachCount: number of attachments already added to bld
//
// Returns the updated builder with normal attachments embedded and large
// attachment HTML injected into the body.
func processLargeAttachments(
	ctx context.Context,
	runtime *common.RuntimeContext,
	bld emlbuilder.Builder,
	attachPaths []string,
	extraEMLBytes int64,
	extraAttachCount int,
) (emlbuilder.Builder, error) {
	// Count check (total attachments must not exceed limit)
	totalCount := extraAttachCount + len(attachPaths)
	if totalCount > MaxAttachmentCount {
		return bld, fmt.Errorf("attachment count %d exceeds the limit of %d", totalCount, MaxAttachmentCount)
	}

	files, err := statAttachmentFiles(runtime.FileIO(), attachPaths)
	if err != nil {
		return bld, err
	}

	classified := classifyAttachments(files, extraEMLBytes)

	if len(classified.Oversized) == 0 {
		// All files fit in EML — use the normal path
		for _, f := range classified.Normal {
			bld = bld.AddFileAttachment(f.Path)
		}
		return bld, nil
	}

	// Guard: large attachment upload requires user identity. When unavailable
	// (e.g. bot identity), fall back to the traditional size-limit error so
	// callers get a clear, actionable message.
	if runtime.Config == nil || runtime.UserOpenId() == "" {
		var totalBytes int64
		for _, f := range files {
			totalBytes += f.Size
		}
		return bld, fmt.Errorf("total attachment size %.1f MB exceeds the 25 MB EML limit; "+
			"large attachment upload requires user identity (--as user)",
			float64(totalBytes)/1024/1024)
	}

	// Upload oversized files
	results, err := uploadLargeAttachments(ctx, runtime, classified.Oversized)
	if err != nil {
		return bld, err
	}

	// Generate and inject the large attachment HTML block
	html := buildLargeAttachmentHTML(runtime.Config.Brand, results)
	bld = bld.LargeAttachmentHTML(html)

	// Register large attachment tokens via X-Lms-Large-Attachment-Ids header,
	// so the mail server associates them with this draft.
	type largeAttID struct {
		ID string `json:"id"`
	}
	ids := make([]largeAttID, len(results))
	for i, r := range results {
		ids[i] = largeAttID{ID: r.FileToken}
	}
	idsJSON, err := json.Marshal(ids)
	if err != nil {
		return bld, fmt.Errorf("failed to encode large attachment IDs: %w", err)
	}
	bld = bld.Header("X-Lms-Large-Attachment-Ids", base64.StdEncoding.EncodeToString(idsJSON))

	// Embed normal files
	for _, f := range classified.Normal {
		bld = bld.AddFileAttachment(f.Path)
	}

	// Print summary
	fmt.Fprintf(runtime.IO().ErrOut, "  %d normal attachment(s) embedded in EML\n", len(classified.Normal))
	fmt.Fprintf(runtime.IO().ErrOut, "  %d large attachment(s) uploaded (download links in body)\n", len(classified.Oversized))

	return bld, nil
}
