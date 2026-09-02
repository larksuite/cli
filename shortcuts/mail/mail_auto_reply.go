// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/mail/filecheck"
	"golang.org/x/net/html"
)

const (
	maxAutoReplyContentHTMLRunes = 20000
	maxAutoReplyImageCount       = 250
	maxAutoReplyContentBytes     = 25 * 1024 * 1024
	minAutoReplyTimezoneOffset   = -12 * 60 * 60
	maxAutoReplyTimezoneOffset   = 14 * 60 * 60
)

var autoReplyDataImageRegexp = regexp.MustCompile(`(?i)<img\s(?:[^>]*?\s)?src\s*=\s*["'](data:image/[^"']+)["']`)

var MailAutoReply = common.Shortcut{
	Service:     "mail",
	Command:     "+auto-reply",
	Description: "Get mailbox auto-reply settings.",
	Risk:        "read",
	Scopes:      []string{"mail:user_mailbox.message:readonly"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Default: "me", Desc: "Mailbox address (default: me)."},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		mailboxID := resolveAutoReplyMailboxID(runtime)
		return common.NewDryRunAPI().
			Desc("Get mailbox auto-reply settings").
			GET(autoReplyPath(mailboxID))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		mailboxID := resolveAutoReplyMailboxID(runtime)
		data, err := runtime.CallAPITyped("GET", autoReplyPath(mailboxID), nil, nil)
		if err != nil {
			return mailDecorateProblemMessage(err, "get auto-reply failed")
		}
		return outputAutoReply(ctx, runtime, data, "")
	},
}

var MailAutoReplyModify = common.Shortcut{
	Service:     "mail",
	Command:     "+auto-reply-modify",
	Description: "Modify mailbox auto-reply settings by merging friendly flags into the current setting.",
	Risk:        "high-risk-write",
	Scopes:      []string{"mail:user_mailbox.message:readonly", "mail:user_mailbox.message:modify"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Default: "me", Desc: "Mailbox address (default: me)."},
		{Name: "enable", Type: "bool", Desc: "Turn auto-reply on."},
		{Name: "disable", Type: "bool", Desc: "Turn auto-reply off."},
		{Name: "content", Desc: "Auto-reply HTML content. Plain text is accepted and sent as-is. Supports @file, - stdin, local images, and data URI images.", Input: []string{common.File, common.Stdin}},
		{Name: "content-file", Desc: "Read auto-reply content from a file in the current directory. Local and data URI images are supported. Mutually exclusive with --content."},
		{Name: "start", Desc: "Start date as Unix timestamp or ISO 8601. Stored as the day's 00:00:00.000."},
		{Name: "end", Desc: "End date as Unix timestamp or ISO 8601. Stored as the day's 23:59:59.999."},
		{Name: "timezone", Desc: "UTC offset seconds for the auto-reply range, e.g. 28800 for UTC+8."},
		{Name: "internal-only", Type: "bool", Desc: "Only send auto-replies to tenant-internal senders."},
		{Name: "all", Type: "bool", Desc: "Send auto-replies to all senders, including external senders."},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		mailboxID := resolveAutoReplyMailboxID(runtime)
		patch, _ := buildAutoReplyPatch(ctx, runtime, false)
		return common.NewDryRunAPI().
			Desc("Modify mailbox auto-reply settings. Dry-run body shows only the options provided by flags because the live current value is unavailable.").
			GET(autoReplyPath(mailboxID)).
			PUT(autoReplyPath(mailboxID)).
			Body(patch)
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if runtime.Bool("enable") && runtime.Bool("disable") {
			return mailValidationError("--enable and --disable are mutually exclusive").
				WithParams(
					mailInvalidParam("--enable", "mutually exclusive with --disable"),
					mailInvalidParam("--disable", "mutually exclusive with --enable"),
				)
		}
		if runtime.Bool("internal-only") && runtime.Bool("all") {
			return mailValidationError("--internal-only and --all are mutually exclusive").
				WithParams(
					mailInvalidParam("--internal-only", "mutually exclusive with --all"),
					mailInvalidParam("--all", "mutually exclusive with --internal-only"),
				)
		}
		if runtime.Str("content") != "" && runtime.Str("content-file") != "" {
			return mailValidationError("--content and --content-file are mutually exclusive").
				WithParams(
					mailInvalidParam("--content", "mutually exclusive with --content-file"),
					mailInvalidParam("--content-file", "mutually exclusive with --content"),
				)
		}
		if !autoReplyHasModify(runtime) {
			return mailValidationError("no auto-reply changes provided")
		}
		_, err := buildAutoReplyPatch(ctx, runtime, false)
		return err
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		mailboxID := resolveAutoReplyMailboxID(runtime)
		preflightPatch, err := buildAutoReplyPatch(ctx, runtime, false)
		if err != nil {
			return err
		}
		current, err := fetchAutoReplySetting(ctx, runtime, mailboxID)
		if err != nil {
			return err
		}
		if err := validateAutoReplyFinal(preflightPatch, mergeAutoReplySetting(current, preflightPatch)); err != nil {
			return err
		}
		patch, err := buildAutoReplyPatch(ctx, runtime, true)
		if err != nil {
			return err
		}
		data, err := runtime.CallAPITyped("PUT", autoReplyPath(mailboxID), nil, patch)
		if err != nil {
			return mailDecorateProblemMessage(err, "modify auto-reply failed")
		}
		return outputAutoReply(ctx, runtime, data, "Auto-reply modified.")
	},
}

func autoReplyPath(mailboxID string) string {
	return mailboxPath(mailboxID, "settings", "auto_reply")
}

func resolveAutoReplyMailboxID(runtime *common.RuntimeContext) string {
	if mailbox := strings.TrimSpace(runtime.Str("mailbox")); mailbox != "" {
		return mailbox
	}
	return "me"
}

func autoReplyHasModify(runtime *common.RuntimeContext) bool {
	for _, flag := range []string{
		"enable", "disable", "content", "content-file",
		"start", "end", "timezone", "internal-only", "all",
	} {
		if runtime.Changed(flag) {
			return true
		}
	}
	return false
}

func buildAutoReplyPatch(ctx context.Context, runtime *common.RuntimeContext, uploadLocalImages bool) (map[string]interface{}, error) {
	autoReply := map[string]interface{}{}
	if runtime.Bool("enable") {
		autoReply["enabled"] = true
	}
	if runtime.Bool("disable") {
		autoReply["enabled"] = false
	}
	content, err := resolveAutoReplyContent(runtime)
	if err != nil {
		return nil, err
	}
	if err := validateAutoReplyContentHTML(runtime, content); err != nil {
		return nil, err
	}
	contentChanged := runtime.Changed("content") || runtime.Changed("content-file")
	images := []map[string]interface{}{}
	if uploadLocalImages && contentChanged && content != "" {
		content, images, err = uploadAutoReplyLocalImages(ctx, runtime, content)
		if err != nil {
			return nil, err
		}
		if len(images) > 0 {
			autoReply["images"] = images
		}
	}
	if contentChanged {
		if uploadLocalImages || !autoReplyHasDataURIImage(content) {
			if err := validateAutoReplyContentLimits(content, images); err != nil {
				return nil, err
			}
		}
		autoReply["content_html"] = content
		autoReply["images"] = images
	}
	timezone := strings.TrimSpace(runtime.Str("timezone"))
	if err := validateAutoReplyTimezone(timezone); err != nil {
		return nil, err
	}
	if start := strings.TrimSpace(runtime.Str("start")); start != "" {
		ts, err := parseAutoReplyDateMillis("--start", start, timezone, false)
		if err != nil {
			return nil, err
		}
		autoReply["start_time"] = ts
	}
	if end := strings.TrimSpace(runtime.Str("end")); end != "" {
		ts, err := parseAutoReplyDateMillis("--end", end, timezone, true)
		if err != nil {
			return nil, err
		}
		autoReply["end_time"] = ts
	}
	if start, ok := autoReply["start_time"].(string); ok {
		if end, ok := autoReply["end_time"].(string); ok {
			startTS, _ := strconv.ParseInt(start, 10, 64)
			endTS, _ := strconv.ParseInt(end, 10, 64)
			if startTS > 0 && endTS > 0 && startTS >= endTS {
				return nil, mailValidationParamError("--end", "--end must be after --start")
			}
		}
	}
	if timezone != "" {
		autoReply["time_zone"] = timezone
	}
	if runtime.Bool("internal-only") {
		autoReply["only_send_to_tenant"] = true
	}
	if runtime.Bool("all") {
		autoReply["only_send_to_tenant"] = false
	}
	if len(autoReply) == 0 {
		return nil, mailValidationError("no auto-reply changes provided")
	}
	return autoReply, nil
}

func fetchAutoReplySetting(ctx context.Context, runtime *common.RuntimeContext, mailboxID string) (map[string]interface{}, error) {
	data, err := runtime.CallAPITyped("GET", autoReplyPath(mailboxID), nil, nil)
	if err != nil {
		return nil, mailDecorateProblemMessage(err, "get current auto-reply failed")
	}
	if nested, ok := data["auto_reply"].(map[string]interface{}); ok {
		data = nested
	}
	return normalizeAutoReplyFields(data), nil
}

func mergeAutoReplySetting(current, patch map[string]interface{}) map[string]interface{} {
	merged := map[string]interface{}{}
	for k, v := range current {
		merged[k] = v
	}
	for k, v := range patch {
		merged[k] = v
	}
	return normalizeAutoReplyFields(merged)
}

func validateAutoReplyFinal(patch, autoReply map[string]interface{}) error {
	startSet := autoReplyHasKey(patch, "start_time")
	endSet := autoReplyHasKey(patch, "end_time")
	enabled := autoReplyBool(autoReply["enabled"])

	if enabled && strings.TrimSpace(autoReplyString(autoReply["content_html"])) == "" {
		return mailValidationParamError("--content", "content_html is required when enabled=true")
	}
	startTS, err := autoReplyInt64(autoReply["start_time"])
	if err != nil {
		return mailValidationParamError("--start", "start_time must be a Unix milliseconds timestamp")
	}
	endTS, err := autoReplyInt64(autoReply["end_time"])
	if err != nil {
		return mailValidationParamError("--end", "end_time must be a Unix milliseconds timestamp")
	}
	timezone := strings.TrimSpace(autoReplyString(autoReply["time_zone"]))
	if enabled && timezone == "" {
		return mailValidationParamError("--timezone", "time_zone is required when enabled=true")
	}
	loc := time.Local
	if timezone != "" {
		loadedLoc, err := autoReplyLocation(timezone, time.Local)
		if err != nil {
			return mailValidationParamError("--timezone", "invalid time_zone: %s", timezone)
		}
		loc = loadedLoc
	}
	now := time.Now().In(loc)
	currentDateStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).UnixMilli()
	if startTS > 0 && (enabled || startSet) && startTS < currentDateStart {
		return mailValidationParamError("--start", "start_time must be greater than or equal to current date")
	}
	if endTS > 0 && (enabled || endSet) && endTS < currentDateStart {
		return mailValidationParamError("--end", "end_time must be greater than or equal to current date")
	}
	if startTS > 0 && endTS > 0 && (enabled || startSet || endSet) && startTS >= endTS {
		return mailValidationParamError("--end", "end_time must be greater than start_time")
	}
	return nil
}

func autoReplyHasKey(values map[string]interface{}, key string) bool {
	_, ok := values[key]
	return ok
}

func autoReplyString(raw interface{}) string {
	switch v := raw.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatInt(int64(v), 10)
	default:
		return ""
	}
}

func autoReplyBool(raw interface{}) bool {
	v, _ := raw.(bool)
	return v
}

func autoReplyInt64(raw interface{}) (int64, error) {
	switch v := raw.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return 0, nil
		}
		return strconv.ParseInt(v, 10, 64)
	case int:
		return int64(v), nil
	case int64:
		return v, nil
	case float64:
		return int64(v), nil
	default:
		return 0, nil
	}
}

func resolveAutoReplyContent(runtime *common.RuntimeContext) (string, error) {
	if content := runtime.Str("content"); content != "" {
		return content, nil
	}
	path := runtime.Str("content-file")
	if path == "" {
		return "", nil
	}
	if err := validateAutoReplyContentFilePath(path); err != nil {
		return "", err
	}
	f, err := runtime.FileIO().Open(path)
	if err != nil {
		return "", mailValidationParamError("--content-file", "open --content-file %s: %v", path, err).WithCause(mailInputStatError(err))
	}
	defer f.Close()
	buf, err := io.ReadAll(f)
	if err != nil {
		return "", mailValidationParamError("--content-file", "read --content-file %s: %v", path, err).WithCause(err)
	}
	return string(buf), nil
}

func validateAutoReplyContentHTML(runtime *common.RuntimeContext, contentHTML string) error {
	if strings.TrimSpace(contentHTML) == "" {
		return nil
	}
	param := "--content"
	if runtime.Str("content") == "" && runtime.Str("content-file") != "" {
		param = "--content-file"
	}
	tokenizer := html.NewTokenizer(strings.NewReader(contentHTML))
	for {
		tokenType := tokenizer.Next()
		switch tokenType {
		case html.ErrorToken:
			if errors.Is(tokenizer.Err(), io.EOF) {
				return nil
			}
			return mailValidationParamError(param, "%s contains invalid html", param).WithCause(tokenizer.Err())
		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokenizer.Token()
			if isUnsafeAutoReplyHTMLTag(token.Data) {
				return mailValidationParamError(param, "%s contains unsafe html tag: %s", param, token.Data)
			}
			for _, attr := range token.Attr {
				if isUnsafeAutoReplyHTMLAttr(attr) {
					return mailValidationParamError(param, "%s contains unsafe html attribute: %s", param, attr.Key)
				}
			}
		}
	}
}

func validateAutoReplyContentLimits(contentHTML string, images []map[string]interface{}) error {
	if len([]rune(contentHTML)) > maxAutoReplyContentHTMLRunes {
		return mailFailedPreconditionError("auto-reply content_html exceeds %d characters", maxAutoReplyContentHTMLRunes)
	}
	totalBytes := int64(len([]byte(contentHTML)))
	if len(images) > maxAutoReplyImageCount {
		return mailFailedPreconditionError("auto-reply images count exceeds %d", maxAutoReplyImageCount)
	}
	for _, image := range images {
		fileSize, err := autoReplyImageFileSize(image["file_size"])
		if err != nil {
			return err
		}
		totalBytes += fileSize
	}
	if totalBytes > maxAutoReplyContentBytes {
		return mailFailedPreconditionError("auto-reply content size exceeds %d MB limit", maxAutoReplyContentBytes/(1024*1024))
	}
	return nil
}

func autoReplyImageFileSize(raw interface{}) (int64, error) {
	var size int64
	switch value := raw.(type) {
	case int64:
		size = value
	case int:
		size = int64(value)
	case float64:
		size = int64(value)
	default:
		return 0, mailFailedPreconditionError("auto-reply image file_size must be a number")
	}
	if size < 0 {
		return 0, mailFailedPreconditionError("auto-reply image file_size must be non-negative")
	}
	return size, nil
}

func isUnsafeAutoReplyHTMLTag(tag string) bool {
	switch strings.ToLower(strings.TrimSpace(tag)) {
	case "script", "style", "iframe", "object", "embed", "form", "input", "button", "textarea", "select", "option", "meta", "link", "base", "svg", "math":
		return true
	default:
		return false
	}
}

func isUnsafeAutoReplyHTMLAttr(attr html.Attribute) bool {
	key := strings.ToLower(strings.TrimSpace(attr.Key))
	if key == "" {
		return false
	}
	if strings.HasPrefix(key, "on") {
		return true
	}
	switch key {
	case "src", "href", "action", "formaction", "xlink:href", "poster", "background":
		return isUnsafeAutoReplyURL(attr.Val)
	case "style":
		return isUnsafeAutoReplyStyle(attr.Val)
	case "srcdoc", "srcset":
		return true
	default:
		return false
	}
}

func isUnsafeAutoReplyURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	compact := strings.Map(func(r rune) rune {
		if r <= ' ' || r == 0x7f {
			return -1
		}
		return r
	}, raw)
	lowerCompact := strings.ToLower(compact)
	if strings.HasPrefix(lowerCompact, "javascript:") || strings.HasPrefix(lowerCompact, "vbscript:") {
		return true
	}
	parsed, err := url.Parse(compact)
	if err != nil {
		return true
	}
	scheme := strings.ToLower(parsed.Scheme)
	switch scheme {
	case "", "http", "https", "mailto", "cid":
		return false
	case "data":
		mediaType := strings.ToLower(strings.TrimSpace(strings.SplitN(parsed.Opaque, ",", 2)[0]))
		return !strings.HasPrefix(mediaType, "image/")
	default:
		return true
	}
}

func isUnsafeAutoReplyStyle(style string) bool {
	lower := strings.ToLower(style)
	return strings.Contains(lower, "expression") ||
		strings.Contains(lower, "behavior:") ||
		strings.Contains(lower, "-moz-binding") ||
		strings.Contains(lower, "url(")
}

func validateAutoReplyTimezone(timezone string) error {
	if timezone == "" {
		return nil
	}
	if _, err := parseAutoReplyTimezoneOffset(timezone); err != nil {
		return err
	}
	return nil
}

func parseAutoReplyTimezoneOffset(timezone string) (int, error) {
	offset, err := strconv.Atoi(strings.TrimSpace(timezone))
	if err != nil {
		return 0, mailValidationParamError("--timezone", "--timezone must be UTC offset seconds, e.g. 28800 for UTC+8").WithCause(err)
	}
	if offset < minAutoReplyTimezoneOffset || offset > maxAutoReplyTimezoneOffset {
		return 0, mailValidationParamError("--timezone", "--timezone must be between %d and %d seconds", minAutoReplyTimezoneOffset, maxAutoReplyTimezoneOffset)
	}
	return offset, nil
}

func autoReplyOffsetLocation(offset int) *time.Location {
	return time.FixedZone(autoReplyOffsetName(offset), offset)
}

func autoReplyOffsetName(offset int) string {
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	return fmt.Sprintf("UTC%s%02d:%02d", sign, offset/3600, offset%3600/60)
}

func validateAutoReplyContentFilePath(path string) error {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "." || clean == ".." || filepath.IsAbs(clean) || filepath.Base(clean) != clean {
		return mailValidationParamError("--content-file", "--content-file must be a file in the current directory")
	}
	return nil
}

func autoReplyHasDataURIImage(content string) bool {
	return autoReplyDataImageRegexp.MatchString(content)
}

func uploadAutoReplyLocalImages(ctx context.Context, runtime *common.RuntimeContext, content string) (string, []map[string]interface{}, error) {
	imgs := parseLocalImgs(content)
	type uploadedImage struct {
		cid, fileKey string
		size         int64
	}
	uploaded := make(map[string]uploadedImage, len(imgs))
	images := make([]map[string]interface{}, 0, len(imgs))
	for _, img := range imgs {
		item, ok := uploaded[img.Path]
		if !ok {
			buf, err := readAutoReplyImage(runtime, img.Path)
			if err != nil {
				return "", nil, err
			}
			_, err = filecheck.CheckInlineImageFormat(filepath.Base(img.Path), buf)
			if err != nil {
				return "", nil, mailValidationParamError("--content", "inline image %s: %v", img.Path, err).WithCause(err)
			}
			fileKey, size, err := uploadAutoReplyImageToDrive(runtime, img.Path)
			if err != nil {
				return "", nil, err
			}
			cid, err := generateAutoReplyCID()
			if err != nil {
				return "", nil, err
			}
			item = uploadedImage{cid: cid, fileKey: fileKey, size: size}
			uploaded[img.Path] = item
			image := map[string]interface{}{
				"cid": item.cid, "image_name": filepath.Base(img.Path), "file_key": item.fileKey,
				"file_size": item.size,
			}
			images = append(images, image)
		}
		content = replaceImgSrcOnce(content, img.RawSrc, "cid:"+item.cid)
	}
	return uploadAutoReplyDataImages(runtime, content, images)
}

func uploadAutoReplyDataImages(runtime *common.RuntimeContext, content string, images []map[string]interface{}) (string, []map[string]interface{}, error) {
	for i, m := range autoReplyDataImageRegexp.FindAllStringSubmatch(content, -1) {
		rawSrc := m[1]
		head, payload, ok := strings.Cut(rawSrc, ",")
		if !ok || !strings.Contains(strings.ToLower(head), ";base64") {
			return "", nil, mailValidationParamError("--content", "inline data image must be a base64 data URI")
		}
		mediaType := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.Split(head, ";")[0], "data:")))
		ext, ok := map[string]string{"image/jpeg": "jpg", "image/png": "png", "image/gif": "gif", "image/webp": "webp"}[mediaType]
		if !ok {
			return "", nil, mailValidationParamError("--content", "inline data image media type %q is not supported", mediaType)
		}
		buf, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(payload), ""))
		if err != nil {
			return "", nil, mailValidationParamError("--content", "inline data image contains invalid base64").WithCause(err)
		}
		name := fmt.Sprintf("auto-reply-image-%d.%s", i+1, ext)
		mimeType, err := filecheck.CheckInlineImageFormat(name, buf)
		if err != nil {
			return "", nil, mailValidationParamError("--content", "inline data image: %v", err).WithCause(err)
		}
		if mimeType != mediaType {
			return "", nil, mailValidationParamError("--content", "inline data image declares %s but content is %s", mediaType, mimeType)
		}
		fileKey, size, err := uploadAutoReplyImageBytes(runtime, name, buf)
		if err != nil {
			return "", nil, err
		}
		cid, err := generateAutoReplyCID()
		if err != nil {
			return "", nil, err
		}
		content = replaceImgSrcOnce(content, rawSrc, "cid:"+cid)
		image := map[string]interface{}{
			"cid": cid, "image_name": name, "file_key": fileKey,
			"file_size": size,
		}
		images = append(images, image)
	}
	return content, images, nil
}

func generateAutoReplyCID() (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", errs.NewInternalError(errs.SubtypeSDKError, "failed to generate CID: %v", err).WithCause(err)
	}
	return id.String(), nil
}

func uploadAutoReplyImageToDrive(runtime *common.RuntimeContext, path string) (fileKey string, size int64, err error) {
	info, err := runtime.FileIO().Stat(path)
	if err != nil {
		return "", 0, mailInputStatError(err)
	}
	size = info.Size()
	if size > MaxLargeAttachmentSize {
		return "", size, mailFailedPreconditionError("auto-reply image %s (%.1f GB) exceeds the %.0f GB single file limit",
			filepath.Base(path), float64(size)/1024/1024/1024, float64(MaxLargeAttachmentSize)/1024/1024/1024)
	}
	name := filepath.Base(path)
	if err := filecheck.CheckBlockedExtension(name); err != nil {
		return "", size, mailValidationError("%v", err).WithCause(err)
	}
	userOpenId := runtime.UserOpenId()
	if userOpenId == "" {
		return "", size, mailFailedPreconditionError("auto-reply image upload requires user identity (--as user)")
	}
	if size <= common.MaxDriveMediaUploadSinglePartSize {
		fileKey, err = common.UploadDriveMediaAllTyped(runtime, common.DriveMediaUploadAllConfig{
			FilePath:   path,
			FileName:   name,
			FileSize:   size,
			ParentType: "email",
			ParentNode: &userOpenId,
		})
	} else {
		fileKey, err = common.UploadDriveMediaMultipartTyped(runtime, common.DriveMediaMultipartUploadConfig{
			FilePath:   path,
			FileName:   name,
			FileSize:   size,
			ParentType: "email",
			ParentNode: userOpenId,
		})
	}
	if err != nil {
		return "", size, mailDecorateProblemMessage(err, "upload auto-reply image %s to Drive failed", name)
	}
	return fileKey, size, nil
}

func uploadAutoReplyImageBytes(runtime *common.RuntimeContext, name string, buf []byte) (fileKey string, size int64, err error) {
	size = int64(len(buf))
	if size > MaxLargeAttachmentSize {
		return "", size, mailFailedPreconditionError("auto-reply image %s (%.1f GB) exceeds the %.0f GB single file limit",
			name, float64(size)/1024/1024/1024, float64(MaxLargeAttachmentSize)/1024/1024/1024)
	}
	userOpenId := runtime.UserOpenId()
	if userOpenId == "" {
		return "", size, mailFailedPreconditionError("auto-reply image upload requires user identity (--as user)")
	}
	if size <= common.MaxDriveMediaUploadSinglePartSize {
		fileKey, err = common.UploadDriveMediaAllTyped(runtime, common.DriveMediaUploadAllConfig{
			FileName:   name,
			FileSize:   size,
			ParentType: "email",
			ParentNode: &userOpenId,
			Reader:     bytes.NewReader(buf),
		})
	} else {
		fileKey, err = common.UploadDriveMediaMultipartTyped(runtime, common.DriveMediaMultipartUploadConfig{
			FileName:   name,
			FileSize:   size,
			ParentType: "email",
			ParentNode: userOpenId,
			Reader:     bytes.NewReader(buf),
		})
	}
	if err != nil {
		return "", size, mailDecorateProblemMessage(err, "upload auto-reply image %s to Drive failed", name)
	}
	return fileKey, size, nil
}

func readAutoReplyImage(runtime *common.RuntimeContext, path string) ([]byte, error) {
	f, err := runtime.FileIO().Open(path)
	if err != nil {
		return nil, mailValidationParamError("--content", "open inline image %s: %v", path, err).WithCause(mailInputStatError(err))
	}
	defer f.Close()
	buf, err := io.ReadAll(f)
	if err != nil {
		return nil, mailValidationParamError("--content", "read inline image %s: %v", path, err).WithCause(err)
	}
	return buf, nil
}

func parseAutoReplyDateMillis(flag, raw, timezone string, endOfDay bool) (string, error) {
	if value, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if value == 0 {
			return "0", nil
		}
		t := time.Unix(value, 0)
		if value >= 1_000_000_000_000 {
			t = time.UnixMilli(value)
		}
		loc, err := autoReplyLocation(timezone, t.Location())
		if err != nil {
			return "", err
		}
		return strconv.FormatInt(autoReplyDayBoundary(t.In(loc), endOfDay).UnixMilli(), 10), nil
	}

	t, err := parseAutoReplyISODate(raw, timezone)
	if err != nil {
		return "", mailValidationParamError(flag, "%s must be Unix seconds or ISO 8601, got %q", flag, raw).WithCause(err)
	}
	return strconv.FormatInt(autoReplyDayBoundary(t, endOfDay).UnixMilli(), 10), nil
}

func parseAutoReplyISODate(raw, timezone string) (time.Time, error) {
	if timezone != "" && !autoReplyHasExplicitZone(raw) {
		loc, err := autoReplyLocation(timezone, time.Local)
		if err != nil {
			return time.Time{}, err
		}
		for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02"} {
			if t, err := time.ParseInLocation(layout, raw, loc); err == nil {
				return t, nil
			}
		}
	}
	t, err := parseISO8601(raw)
	if err != nil {
		return time.Time{}, err
	}
	if timezone != "" {
		loc, err := autoReplyLocation(timezone, time.Local)
		if err != nil {
			return time.Time{}, err
		}
		t = t.In(loc)
	}
	return t, nil
}

func autoReplyLocation(timezone string, fallback *time.Location) (*time.Location, error) {
	if timezone == "" {
		return fallback, nil
	}
	if offset, err := parseAutoReplyTimezoneOffset(timezone); err == nil {
		return autoReplyOffsetLocation(offset), nil
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, mailValidationParamError("--timezone", "invalid --timezone %q", timezone).WithCause(err)
	}
	return loc, nil
}

func autoReplyDayBoundary(t time.Time, endOfDay bool) time.Time {
	start := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	if endOfDay {
		return start.AddDate(0, 0, 1).Add(-time.Millisecond)
	}
	return start
}

func autoReplyHasExplicitZone(raw string) bool {
	if strings.HasSuffix(raw, "Z") {
		return true
	}
	tPos := strings.Index(raw, "T")
	if tPos < 0 {
		return false
	}
	return strings.ContainsAny(raw[tPos+1:], "+-")
}

func normalizeAutoReplyFields(in map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range in {
		out[k] = v
	}
	moveAutoReplyField(out, "enable", "enabled")
	moveAutoReplyField(out, "content", "content_html")
	moveAutoReplyField(out, "timezone", "time_zone")
	moveAutoReplyField(out, "only_send_inner_sender", "only_send_to_tenant")
	return out
}

func moveAutoReplyField(values map[string]interface{}, oldKey, newKey string) {
	if v, ok := values[oldKey]; ok {
		if _, exists := values[newKey]; !exists {
			values[newKey] = v
		}
		delete(values, oldKey)
	}
}

func outputAutoReply(ctx context.Context, runtime *common.RuntimeContext, data map[string]interface{}, message string) error {
	autoReply := data
	if nested, ok := data["auto_reply"].(map[string]interface{}); ok {
		autoReply = nested
	}
	autoReply = normalizeAutoReplyFields(autoReply)
	if content, ok := autoReply["content_html"]; ok {
		autoReply["content"] = content
		delete(autoReply, "content_html")
	}
	runtime.OutFormat(
		map[string]interface{}{"auto_reply": autoReply},
		&output.Meta{Count: 1},
		func(w io.Writer) {
			if message != "" {
				fmt.Fprintln(w, message)
			}
			if enabled, ok := autoReply["enabled"].(bool); ok {
				fmt.Fprintf(w, "enabled: %v\n", enabled)
			}
			if summary, ok := autoReply["content_summary"].(string); ok && summary != "" {
				fmt.Fprintf(w, "summary: %s\n", summary)
			}
			if start, ok := autoReply["start_time"].(string); ok && start != "" {
				fmt.Fprintf(w, "start_time: %s\n", start)
			}
			if end, ok := autoReply["end_time"].(string); ok && end != "" {
				fmt.Fprintf(w, "end_time: %s\n", end)
			}
			if timezone, ok := autoReply["time_zone"].(string); ok && timezone != "" {
				fmt.Fprintf(w, "timezone: %s\n", timezone)
			}
			if innerOnly, ok := autoReply["only_send_to_tenant"].(bool); ok {
				fmt.Fprintf(w, "internal_only: %v\n", innerOnly)
			}
		},
	)
	return nil
}
