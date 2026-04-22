// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
	draftpkg "github.com/larksuite/cli/shortcuts/mail/draft"
	"github.com/larksuite/cli/shortcuts/mail/emlbuilder"
	"github.com/larksuite/cli/shortcuts/mail/filecheck"
)

// attachmentFile holds metadata about a local file to be attached.
type attachmentFile struct {
	Path        string // relative file path as provided by the user
	FileName    string // basename
	Size        int64  // raw file size in bytes
	SourceIndex int    // original index in the caller's list (e.g. patch op index)
	Data        []byte // in-memory content; when non-nil, used instead of Path for upload
}

// classifiedAttachments is the result of classifyAttachments.
type classifiedAttachments struct {
	Normal    []attachmentFile // to be embedded in the EML
	Oversized []attachmentFile // to be uploaded as large attachments
}

// largeAttachmentResult holds the upload result for a single large attachment.
type largeAttachmentResult struct {
	FileName  string
	FileSize  int64
	FileToken string
}

// MaxLargeAttachmentSize is the maximum allowed size for a single large
// attachment, aligned with the desktop client (3 GB).
const MaxLargeAttachmentSize = 3 * 1024 * 1024 * 1024 // 3 GB

// largeAttID is the JSON element inside the X-Lms-Large-Attachment-Ids header.
// The header name itself is defined as draftpkg.LargeAttachmentIDsHeader.
type largeAttID struct {
	ID string `json:"id"`
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

// statAttachmentFiles stats each path, checks blocked extensions, and returns
// attachmentFile metadata.
func statAttachmentFiles(fio fileio.FileIO, paths []string) ([]attachmentFile, error) {
	files := make([]attachmentFile, 0, len(paths))
	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			continue
		}
		name := filepath.Base(p)
		if err := filecheck.CheckBlockedExtension(name); err != nil {
			return nil, err
		}
		info, err := fio.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("failed to stat attachment %s: %w", p, err)
		}
		files = append(files, attachmentFile{
			Path:     p,
			FileName: name,
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
		if f.Data != nil {
			fileToken, err = common.UploadDriveMediaAll(runtime, common.DriveMediaUploadAllConfig{
				FileName:   f.FileName,
				FileSize:   f.Size,
				ParentType: "email",
				ParentNode: &userOpenId,
				Reader:     bytes.NewReader(f.Data),
			})
		} else if f.Size <= common.MaxDriveMediaUploadSinglePartSize {
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
// Large attachment HTML templates, matching desktop's exportLargeFileArea
// (mail-editor/src/plugins/bigAttachment/export.ts).
//
// IDs: container = "large-file-area-{9-digit-timestamp}", item = "large-file-item"
// Colors: title bg = rgb(224, 233, 255), link = rgb(20, 86, 240)
// Layout: float (not flexbox) for email client compatibility
const (
	// %s order: timestamp, title, items
	largeAttContainerTpl = `<div id="large-file-area-%s" style="border: 1px solid #DEE0E3; margin-bottom: 20px;max-width: 400px; min-width: 160px; border-radius: 8px;">` +
		`<div style="font-weight: 500; font-size: 16px;line-height: 24px; padding: 8px 16px;background-color: rgb(224, 233, 255); border-top-left-radius: 8px;border-top-right-radius: 8px;">%s</div>` +
		`%s` + // items
		`</div>`

	// %s order: icon URL, filename, file size, preview link, token, download text
	largeAttItemTpl = `<div style="border-top: solid 1px #DEE0E3;padding: 12px;box-sizing: border-box;clear: both;overflow: hidden;display: flex;" id="large-file-item">` +
		`<div style="float: left; margin-right: 8px; margin-top: 1px; margin-bottom: 1px;">` +
		`<img src="%s" height="40" width="40" style="height: 40px;width: 40px;"/>` + // icon URL
		`</div>` +
		`<div style="overflow: hidden;text-overflow: ellipsis;display: inline-block;width: 290px;float:left; margin-right: 10px;">` +
		`<div style="white-space: nowrap; overflow: hidden; text-overflow: ellipsis;font-size: 14px;line-height: 22px;color: #1f2329">%s</div>` + // filename
		`<div style="font-size: 12px; line-height: 20px; color: #8f959e; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;">` +
		`<span style="color: #8f959e;vertical-align: middle;">%s</span>` + // file size
		`</div>` +
		`</div>` +
		`<a href="%s" data-mail-token="%s" style="margin: 10px; text-decoration: none; color: rgb(20, 86, 240); white-space: nowrap; cursor: pointer; line-height: 1.5; float: right; text-align: right; font-size: 14px;">%s</a>` + // preview link, token, download text
		`</div>`

	iconCDNCN = "https://lf-larkemail.bytetos.com/obj/eden-cn/aultojhaah_npi_spht_ryhs/ljhwZthlaukjlkulzlp/"
	iconCDNEN = "https://sf16-sg.tiktokcdn.com/obj/eden-sg/aultojhaah_npi_spht_ryhs/ljhwZthlaukjlkulzlp/"
)

// brandDisplayName returns the product display name used in mail HTML
// text, aligning with the desktop client's APP_DISPLAY_NAME i18n
// substitution.
//
//   - BrandLark    → "Lark" (same in English and Chinese)
//   - BrandFeishu  → "飞书" for zh languages, "Feishu" for others
func brandDisplayName(brand core.LarkBrand, lang string) string {
	if brand == core.BrandLark {
		return "Lark"
	}
	if strings.HasPrefix(lang, "zh") {
		return "飞书"
	}
	return "Feishu"
}

func buildLargeAttachmentItems(brand core.LarkBrand, lang string, results []largeAttachmentResult) string {
	if len(results) == 0 {
		return ""
	}
	downloadText := "Download"
	if strings.HasPrefix(lang, "zh") {
		downloadText = "下载"
	}
	iconCDN := iconCDNCN
	if brand == core.BrandLark {
		iconCDN = iconCDNEN
	}
	var items strings.Builder
	for _, att := range results {
		fmt.Fprintf(&items, largeAttItemTpl,
			htmlEscape(iconCDN+fileTypeIcon(att.FileName)),
			htmlEscape(att.FileName),
			htmlEscape(common.FormatSize(att.FileSize)),
			htmlEscape(buildLargeAttachmentPreviewURL(brand, att.FileToken)),
			htmlEscape(att.FileToken),
			downloadText,
		)
	}
	return items.String()
}

func buildLargeAttachmentHTML(brand core.LarkBrand, lang string, results []largeAttachmentResult) string {
	if len(results) == 0 {
		return ""
	}
	appName := brandDisplayName(brand, lang)
	title := "Large file from " + appName + " Mail"
	if strings.HasPrefix(lang, "zh") {
		title = "来自" + appName + "邮箱的超大附件"
	}
	timestamp := fmt.Sprintf("%d", time.Now().UnixMilli())
	if len(timestamp) > 9 {
		timestamp = timestamp[:9]
	}
	return fmt.Sprintf(largeAttContainerTpl, timestamp, title, buildLargeAttachmentItems(brand, lang, results))
}

func buildLargeAttachmentPlainText(brand core.LarkBrand, lang string, results []largeAttachmentResult) string {
	if len(results) == 0 {
		return ""
	}

	appName := brandDisplayName(brand, lang)
	title := "Large file from " + appName + " Mail"
	downloadText := "Download"
	if strings.HasPrefix(lang, "zh") {
		title = "来自" + appName + "邮箱的超大附件"
		downloadText = "下载"
	}

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(title)
	sb.WriteString("\n")
	for i, att := range results {
		sb.WriteString(att.FileName)
		sb.WriteString("\n")
		sb.WriteString(common.FormatSize(att.FileSize))
		sb.WriteString("\n")
		sb.WriteString(downloadText + ": " + buildLargeAttachmentPreviewURL(brand, att.FileToken))
		if i < len(results)-1 {
			sb.WriteString("\n\n")
		} else {
			sb.WriteString("\n")
		}
	}
	return sb.String()
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
// handling across all mail compose shortcuts (draft-create, reply, forward, send).
//
// Parameters:
//   - htmlBody: the current HTML body string (for quote-aware insertion); empty for plain-text emails
//   - textBody: the current text body string; empty for HTML emails
//   - attachPaths: user-specified attachment file paths (from --attach flag)
//   - extraEMLBytes: EML bytes already accounted for
//   - extraAttachCount: number of attachments already added to bld
func processLargeAttachments(
	ctx context.Context,
	runtime *common.RuntimeContext,
	bld emlbuilder.Builder,
	htmlBody string,
	textBody string,
	attachPaths []string,
	extraEMLBytes int64,
	extraAttachCount int,
) (emlbuilder.Builder, error) {
	totalCount := extraAttachCount + len(attachPaths)
	if totalCount > MaxAttachmentCount {
		return bld, fmt.Errorf("attachment count %d exceeds the limit of %d", totalCount, MaxAttachmentCount)
	}

	files, err := statAttachmentFiles(runtime.FileIO(), attachPaths)
	if err != nil {
		return bld, err
	}

	for _, f := range files {
		if f.Size > MaxLargeAttachmentSize {
			return bld, fmt.Errorf("attachment %s (%.1f GB) exceeds the %.0f GB single file limit",
				f.FileName, float64(f.Size)/1024/1024/1024, float64(MaxLargeAttachmentSize)/1024/1024/1024)
		}
	}

	classified := classifyAttachments(files, extraEMLBytes)

	if len(classified.Oversized) == 0 {
		for _, f := range classified.Normal {
			bld = bld.AddFileAttachment(f.Path)
		}
		return bld, nil
	}

	if htmlBody == "" && textBody == "" {
		return bld, fmt.Errorf("large attachments require a body; " +
			"empty messages cannot include the download link")
	}

	if runtime.Config == nil || runtime.UserOpenId() == "" {
		var totalBytes int64
		for _, f := range files {
			totalBytes += f.Size
		}
		return bld, fmt.Errorf("total attachment size %.1f MB exceeds the 25 MB EML limit; "+
			"large attachment upload requires user identity (--as user)",
			float64(totalBytes)/1024/1024)
	}

	results, err := uploadLargeAttachments(ctx, runtime, classified.Oversized)
	if err != nil {
		return bld, err
	}

	if htmlBody != "" {
		largeHTML := buildLargeAttachmentHTML(runtime.Config.Brand, resolveLang(runtime), results)
		bld = bld.HTMLBody([]byte(draftpkg.InsertBeforeQuoteOrAppend(htmlBody, largeHTML)))
	} else {
		largeText := buildLargeAttachmentPlainText(runtime.Config.Brand, resolveLang(runtime), results)
		bld = bld.TextBody([]byte(textBody + largeText))
	}

	ids := make([]largeAttID, len(results))
	for i, r := range results {
		ids[i] = largeAttID{ID: r.FileToken}
	}
	idsJSON, err := json.Marshal(ids)
	if err != nil {
		return bld, fmt.Errorf("failed to encode large attachment IDs: %w", err)
	}
	bld = bld.Header(draftpkg.LargeAttachmentIDsHeader, base64.StdEncoding.EncodeToString(idsJSON))

	for _, f := range classified.Normal {
		bld = bld.AddFileAttachment(f.Path)
	}

	fmt.Fprintf(runtime.IO().ErrOut, "  %d normal attachment(s) embedded in EML\n", len(classified.Normal))
	fmt.Fprintf(runtime.IO().ErrOut, "  %d large attachment(s) uploaded (download links in body)\n", len(classified.Oversized))

	return bld, nil
}

// ensureLargeAttachmentCards checks whether the snapshot's HTML body is missing
// download cards for large attachments registered in the header. Drafts read
// back from the server may have their HTML cards stripped, even though the
// server-format X-Lark-Large-Attachment header still carries file_name and
// file_size metadata. This function uses that metadata to reconstruct only the
// missing cards/text and injects them into the body (HTML or plain text)
// without duplicating entries that are already present.
//
// Must be called BEFORE normalizeLargeAttachmentHeader, because that
// function converts the server-format header to CLI format and discards
// file_name/file_size.
func ensureLargeAttachmentCards(runtime *common.RuntimeContext, snapshot *draftpkg.DraftSnapshot) {
	summaries := draftpkg.ParseLargeAttachmentSummariesFromHeader(snapshot.Headers)
	if len(summaries) == 0 {
		return
	}

	brand := core.BrandFeishu
	if runtime.Config != nil {
		brand = runtime.Config.Brand
	}
	lang := "zh_cn"
	if runtime.Factory != nil {
		lang = resolveLang(runtime)
	}

	htmlPart := draftpkg.FindHTMLBodyPart(snapshot.Body)
	if htmlPart != nil {
		existingCards := draftpkg.ParseLargeAttachmentItemsFromHTML(string(htmlPart.Body))
		var missing []largeAttachmentResult
		for _, s := range summaries {
			if _, exists := existingCards[s.Token]; !exists {
				missing = append(missing, largeAttachmentResult{
					FileName:  s.FileName,
					FileSize:  s.SizeBytes,
					FileToken: s.Token,
				})
			}
		}
		if len(missing) == 0 {
			return
		}
		injectLargeAttachmentHTMLIntoSnapshot(snapshot, brand, lang, missing)
		return
	}

	textPart := draftpkg.FindTextBodyPart(snapshot.Body)
	if textPart != nil {
		bodyText := string(textPart.Body)
		var missing []largeAttachmentResult
		for _, s := range summaries {
			if !strings.Contains(bodyText, s.Token) {
				missing = append(missing, largeAttachmentResult{
					FileName:  s.FileName,
					FileSize:  s.SizeBytes,
					FileToken: s.Token,
				})
			}
		}
		if len(missing) == 0 {
			return
		}
		largeText := buildLargeAttachmentPlainText(brand, lang, missing)
		injectLargeAttachmentTextIntoSnapshot(snapshot, largeText)
	}
}

// preprocessLargeAttachmentsForDraftEdit scans a draft-edit patch for
// add_attachment ops, classifies the files (normal vs oversized based on
// the snapshot's current EML size), uploads oversized files, injects the
// large attachment HTML card into the snapshot's HTML body, and returns
// the patch with oversized ops removed (normal ops stay for draft.Apply).
func preprocessLargeAttachmentsForDraftEdit(
	ctx context.Context,
	runtime *common.RuntimeContext,
	snapshot *draftpkg.DraftSnapshot,
	patch draftpkg.Patch,
) (draftpkg.Patch, error) {
	// Reconstruct missing large attachment HTML cards from the server-format
	// header metadata. Must run before normalizeLargeAttachmentHeader which
	// discards file_name/file_size.
	ensureLargeAttachmentCards(runtime, snapshot)

	// Always normalize server-format headers to CLI format so every code
	// path below (and every early return) sends the format the server
	// recognizes on write.
	normalizeLargeAttachmentHeader(snapshot)

	// Collect add_attachment ops and their indices.
	type attachOp struct {
		index int
		path  string
	}
	var attachOps []attachOp
	for i, op := range patch.Ops {
		if op.Op == "add_attachment" {
			attachOps = append(attachOps, attachOp{index: i, path: op.Path})
		}
	}
	if len(attachOps) == 0 {
		return patch, nil
	}

	// Stat all attachment files.
	paths := make([]string, len(attachOps))
	for i, ao := range attachOps {
		paths[i] = ao.path
	}
	files, err := statAttachmentFiles(runtime.FileIO(), paths)
	if err != nil {
		return patch, err
	}
	for i := range files {
		files[i].SourceIndex = attachOps[i].index
	}

	// Check 3GB single file limit.
	for _, f := range files {
		if f.Size > MaxLargeAttachmentSize {
			return patch, fmt.Errorf("attachment %s (%.1f GB) exceeds the %.0f GB single file limit",
				f.FileName, float64(f.Size)/1024/1024/1024, float64(MaxLargeAttachmentSize)/1024/1024/1024)
		}
	}

	// Calculate the snapshot's current EML base size.
	emlBaseSize := snapshotEMLBaseSize(snapshot)

	// Classify files.
	classified := classifyAttachments(files, emlBaseSize)
	if len(classified.Oversized) == 0 {
		return patch, nil // all fit, let draft.Apply handle them
	}

	// Guard: large attachment requires at least some body part.
	hasHTML := draftpkg.FindHTMLBodyPart(snapshot.Body) != nil
	hasText := draftpkg.FindTextBodyPart(snapshot.Body) != nil
	if !hasHTML && !hasText {
		return patch, fmt.Errorf("large attachments require a body; " +
			"empty drafts cannot include the download link")
	}

	// Guard: need user identity for upload.
	if runtime.Config == nil || runtime.UserOpenId() == "" {
		var totalBytes int64
		for _, f := range files {
			totalBytes += f.Size
		}
		return patch, fmt.Errorf("total attachment size %.1f MB exceeds the 25 MB EML limit; "+
			"large attachment upload requires user identity (--as user)",
			float64(totalBytes)/1024/1024)
	}

	// Upload oversized files.
	results, err := uploadLargeAttachments(ctx, runtime, classified.Oversized)
	if err != nil {
		return patch, err
	}

	if hasHTML {
		injectLargeAttachmentHTMLIntoSnapshot(snapshot, runtime.Config.Brand, resolveLang(runtime), results)
	} else {
		largeText := buildLargeAttachmentPlainText(runtime.Config.Brand, resolveLang(runtime), results)
		injectLargeAttachmentTextIntoSnapshot(snapshot, largeText)
	}

	// Register large attachment tokens, merging with any existing IDs already
	// present in the snapshot (from a previous draft-create or draft-edit).
	// The server returns X-Lark-Large-Attachment on readback, so check both
	// header names.
	var existingIDs []largeAttID
	existingIdx := -1
	for i, h := range snapshot.Headers {
		if draftpkg.IsLargeAttachmentHeader(h.Name) {
			existingIdx = i
			if decoded, err := base64.StdEncoding.DecodeString(h.Value); err == nil {
				var raw []json.RawMessage
				if json.Unmarshal(decoded, &raw) == nil {
					for _, r := range raw {
						var entry struct {
							ID      string `json:"id"`
							FileKey string `json:"file_key"`
						}
						if json.Unmarshal(r, &entry) == nil {
							tok := entry.ID
							if tok == "" {
								tok = entry.FileKey
							}
							if tok != "" {
								existingIDs = append(existingIDs, largeAttID{ID: tok})
							}
						}
					}
				}
			}
			break
		}
	}
	merged := existingIDs
	for _, r := range results {
		merged = append(merged, largeAttID{ID: r.FileToken})
	}
	idsJSON, err := json.Marshal(merged)
	if err != nil {
		return patch, fmt.Errorf("failed to encode large attachment IDs: %w", err)
	}
	headerValue := base64.StdEncoding.EncodeToString(idsJSON)
	if existingIdx >= 0 {
		snapshot.Headers[existingIdx].Name = draftpkg.LargeAttachmentIDsHeader
		snapshot.Headers[existingIdx].Value = headerValue
	} else {
		snapshot.Headers = append(snapshot.Headers, draftpkg.Header{
			Name:  draftpkg.LargeAttachmentIDsHeader,
			Value: headerValue,
		})
	}

	// Remove oversized ops from the patch (keep normal ones for draft.Apply).
	oversizedIndices := make(map[int]bool, len(classified.Oversized))
	for _, f := range classified.Oversized {
		oversizedIndices[f.SourceIndex] = true
	}
	var filteredOps []draftpkg.PatchOp
	for i, op := range patch.Ops {
		if oversizedIndices[i] {
			continue // skip oversized, already uploaded
		}
		filteredOps = append(filteredOps, op)
	}
	patch.Ops = filteredOps

	fmt.Fprintf(runtime.IO().ErrOut, "  %d normal attachment(s) in patch\n", len(classified.Normal))
	fmt.Fprintf(runtime.IO().ErrOut, "  %d large attachment(s) uploaded (download links in body)\n", len(classified.Oversized))

	return patch, nil
}

// snapshotEMLBaseSize estimates the current EML size of a draft snapshot by
// summing all part bodies (base64 encoded) plus a header overhead.
func snapshotEMLBaseSize(snapshot *draftpkg.DraftSnapshot) int64 {
	const headerOverhead = 2048
	var total int64 = headerOverhead
	for _, p := range flattenSnapshotParts(snapshot.Body) {
		total += estimateBase64EMLSize(int64(len(p.Body)))
	}
	return total
}

// flattenSnapshotParts recursively collects all parts in the MIME tree.
func flattenSnapshotParts(root *draftpkg.Part) []*draftpkg.Part {
	if root == nil {
		return nil
	}
	out := []*draftpkg.Part{root}
	for _, child := range root.Children {
		out = append(out, flattenSnapshotParts(child)...)
	}
	return out
}

// injectLargeAttachmentHTMLIntoSnapshot adds large attachment items to the
// snapshot's HTML body. When the body already contains a large-file-area
// container, new items are appended inside that container (maintaining a
// single container, matching the desktop client). Otherwise a new
// container is created and inserted before the quote block (or appended).
func injectLargeAttachmentHTMLIntoSnapshot(snapshot *draftpkg.DraftSnapshot, brand core.LarkBrand, lang string, results []largeAttachmentResult) {
	if len(results) == 0 {
		return
	}
	htmlPart := draftpkg.FindHTMLBodyPart(snapshot.Body)
	if htmlPart == nil {
		if snapshot.Body != nil {
			return
		}
		snapshot.Body = &draftpkg.Part{
			MediaType: "text/html",
			Body:      []byte(buildLargeAttachmentHTML(brand, lang, results)),
			Dirty:     true,
		}
		return
	}

	currentHTML := string(htmlPart.Body)

	if draftpkg.HTMLContainsLargeAttachment(currentHTML) {
		itemsHTML := buildLargeAttachmentItems(brand, lang, results)
		before, card, after := draftpkg.SplitAtLargeAttachment(currentHTML)
		merged := card[:len(card)-len("</div>")] + itemsHTML + "</div>"
		htmlPart.Body = []byte(before + merged + after)
	} else {
		fullHTML := buildLargeAttachmentHTML(brand, lang, results)
		htmlPart.Body = []byte(draftpkg.InsertBeforeQuoteOrAppend(currentHTML, fullHTML))
	}
	htmlPart.Dirty = true
}

func injectLargeAttachmentTextIntoSnapshot(snapshot *draftpkg.DraftSnapshot, largeText string) {
	textPart := draftpkg.FindTextBodyPart(snapshot.Body)
	if textPart == nil {
		if snapshot.Body != nil {
			return
		}
		snapshot.Body = &draftpkg.Part{
			MediaType: "text/plain",
			Body:      []byte(largeText),
			Dirty:     true,
		}
		return
	}
	textPart.Body = append(textPart.Body, []byte(largeText)...)
	textPart.Dirty = true
}

// normalizeLargeAttachmentHeader converts server-format X-Lark-Large-Attachment
// headers to CLI-format X-Lms-Large-Attachment-Ids and removes all server-format
// headers. This ensures the PUT update always sends the format the server
// recognizes for write operations.
func normalizeLargeAttachmentHeader(snapshot *draftpkg.DraftSnapshot) {
	cliIdx := -1
	var serverIdxs []int
	seen := make(map[string]bool)
	var serverTokens []largeAttID

	for i, h := range snapshot.Headers {
		if !draftpkg.IsLargeAttachmentHeader(h.Name) {
			continue
		}
		if strings.EqualFold(h.Name, draftpkg.LargeAttachmentIDsHeader) {
			cliIdx = i
			continue
		}
		serverIdxs = append(serverIdxs, i)
		decoded, err := base64.StdEncoding.DecodeString(h.Value)
		if err != nil {
			continue
		}
		var raw []json.RawMessage
		if json.Unmarshal(decoded, &raw) != nil {
			continue
		}
		for _, r := range raw {
			var entry struct {
				ID      string `json:"id"`
				FileKey string `json:"file_key"`
			}
			if json.Unmarshal(r, &entry) == nil {
				tok := entry.ID
				if tok == "" {
					tok = entry.FileKey
				}
				if tok != "" && !seen[tok] {
					seen[tok] = true
					serverTokens = append(serverTokens, largeAttID{ID: tok})
				}
			}
		}
	}

	if len(serverIdxs) == 0 {
		return
	}

	// Remove server-format headers in reverse order to preserve indices.
	for j := len(serverIdxs) - 1; j >= 0; j-- {
		idx := serverIdxs[j]
		snapshot.Headers = append(snapshot.Headers[:idx], snapshot.Headers[idx+1:]...)
		if cliIdx > idx {
			cliIdx--
		}
	}

	// If a CLI-format header exists, it is authoritative — keep it as-is.
	if cliIdx >= 0 {
		return
	}

	// No CLI header — convert server tokens into one.
	if len(serverTokens) == 0 {
		return
	}
	idsJSON, err := json.Marshal(serverTokens)
	if err != nil {
		return
	}
	snapshot.Headers = append(snapshot.Headers, draftpkg.Header{
		Name:  draftpkg.LargeAttachmentIDsHeader,
		Value: base64.StdEncoding.EncodeToString(idsJSON),
	})
}
