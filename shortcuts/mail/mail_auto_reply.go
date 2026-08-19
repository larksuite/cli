// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/mail/filecheck"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"golang.org/x/net/html"
)

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
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.message:readonly", "mail:user_mailbox.message:modify"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Default: "me", Desc: "Mailbox address (default: me)."},
		{Name: "enable", Type: "bool", Desc: "Turn auto-reply on."},
		{Name: "disable", Type: "bool", Desc: "Turn auto-reply off."},
		{Name: "content", Desc: "Auto-reply HTML content. Plain text is accepted and sent as-is. Supports @file and - stdin. Local <img src=\"./file.png\"> references are uploaded and rewritten to cid: references.", Input: []string{common.File, common.Stdin}},
		{Name: "content-file", Desc: "Read auto-reply content from a file in the current directory. Mutually exclusive with --content."},
		{Name: "start", Desc: "Start date as Unix timestamp or ISO 8601. Stored as the day's 00:00:00.000."},
		{Name: "end", Desc: "End date as Unix timestamp or ISO 8601. Stored as the day's 23:59:59.999."},
		{Name: "timezone", Desc: "Time zone for the auto-reply range, e.g. Asia/Shanghai. Defaults to the start time zone when it can be inferred."},
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
		patch, err := buildAutoReplyPatch(ctx, runtime, true)
		if err != nil {
			return err
		}
		current, err := runtime.CallAPITyped("GET", autoReplyPath(mailboxID), nil, nil)
		if err != nil {
			return mailDecorateProblemMessage(err, "get auto-reply failed")
		}
		merged := mergeAutoReply(current, patch)
		data, err := runtime.CallAPITyped("PUT", autoReplyPath(mailboxID), nil, merged)
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
	if uploadLocalImages && content != "" {
		var images []map[string]interface{}
		content, images, err = uploadAutoReplyLocalImages(ctx, runtime, content)
		if err != nil {
			return nil, err
		}
		if len(images) > 0 {
			autoReply["images"] = images
		}
	}
	if content != "" {
		if int64(len(content)) > maxTemplateContentBytes {
			return nil, mailFailedPreconditionError("auto-reply content exceeds %d MB (got %.1f MB)",
				maxTemplateContentBytes/(1024*1024), float64(len(content))/1024/1024)
		}
		autoReply["content_html"] = content
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
			if startTS >= endTS {
				return nil, mailValidationParamError("--end", "--end must be after --start")
			}
		}
	}
	if timezone != "" {
		autoReply["time_zone"] = timezone
	} else if inferred := inferAutoReplyTimezone(runtime.Str("start")); inferred != "" {
		autoReply["time_zone"] = inferred
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
	if _, err := time.LoadLocation(timezone); err != nil {
		return mailValidationParamError("--timezone", "invalid --timezone %q", timezone).WithCause(err)
	}
	return nil
}

func validateAutoReplyContentFilePath(path string) error {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "." || clean == ".." || filepath.IsAbs(clean) || filepath.Base(clean) != clean {
		return mailValidationParamError("--content-file", "--content-file must be a file in the current directory")
	}
	return nil
}

func uploadAutoReplyLocalImages(ctx context.Context, runtime *common.RuntimeContext, content string) (string, []map[string]interface{}, error) {
	imgs := parseLocalImgs(content)
	type uploadedImage struct {
		cid, fileKey, mimeType string
		size                   int64
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
			mimeType, err := filecheck.CheckInlineImageFormat(filepath.Base(img.Path), buf)
			if err != nil {
				return "", nil, mailValidationParamError("--content", "inline image %s: %v", img.Path, err).WithCause(err)
			}
			fileKey, size, err := uploadToDriveForTemplate(ctx, runtime, img.Path)
			if err != nil {
				return "", nil, err
			}
			cid, err := generateTemplateCID()
			if err != nil {
				return "", nil, err
			}
			item = uploadedImage{cid: cid, fileKey: fileKey, mimeType: mimeType, size: size}
			uploaded[img.Path] = item
			images = append(images, map[string]interface{}{
				"cid": item.cid, "image_name": filepath.Base(img.Path), "file_key": item.fileKey,
				"file_size": item.size, "content_type": item.mimeType,
			})
		}
		content = replaceImgSrcOnce(content, img.RawSrc, "cid:"+item.cid)
	}
	return content, images, nil
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
		loc, err := time.LoadLocation(timezone)
		if err != nil {
			return time.Time{}, mailValidationParamError("--timezone", "invalid --timezone %q", timezone).WithCause(err)
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
		loc, err := time.LoadLocation(timezone)
		if err != nil {
			return time.Time{}, mailValidationParamError("--timezone", "invalid --timezone %q", timezone).WithCause(err)
		}
		t = t.In(loc)
	}
	return t, nil
}

func autoReplyLocation(timezone string, fallback *time.Location) (*time.Location, error) {
	if timezone == "" {
		return fallback, nil
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

func inferAutoReplyTimezone(rawStart string) string {
	if rawStart == "" {
		return ""
	}
	t, err := parseISO8601(rawStart)
	if err != nil {
		return ""
	}
	if name := t.Location().String(); name != "" && name != "UTC" && name != "Local" {
		return name
	}
	return ""
}

func mergeAutoReply(current map[string]interface{}, patch map[string]interface{}) map[string]interface{} {
	merged := map[string]interface{}{}
	if nested, ok := current["auto_reply"].(map[string]interface{}); ok {
		for k, v := range nested {
			merged[k] = v
		}
	} else {
		for k, v := range current {
			merged[k] = v
		}
	}
	merged = normalizeAutoReplyFields(merged)
	for k, v := range patch {
		merged[k] = v
	}
	delete(merged, "content_summary")
	return merged
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
	autoReply["images"] = hydrateAutoReplyImages(ctx, runtime, autoReply["images"])
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

func hydrateAutoReplyImages(ctx context.Context, runtime *common.RuntimeContext, raw interface{}) []interface{} {
	items, _ := raw.([]interface{})
	result := make([]interface{}, 0, len(items))
	for _, rawItem := range items {
		image, ok := rawItem.(map[string]interface{})
		if !ok {
			continue
		}
		projected := make(map[string]interface{}, len(image)+1)
		for key, value := range image {
			if key != "file_key" && key != "download_url" {
				projected[key] = value
			}
		}
		fileKey, _ := image["file_key"].(string)
		if fileKey == "" {
			projected["error"] = "image file_key is missing"
			result = append(result, projected)
			continue
		}
		resp, err := runtime.DoAPIStream(ctx, &larkcore.ApiReq{
			HttpMethod: "GET",
			ApiPath:    fmt.Sprintf("/open-apis/drive/v1/medias/%s/download", validate.EncodePathSegment(fileKey)),
		})
		if err != nil {
			projected["error"] = fmt.Sprintf("download image failed: %v", err)
			result = append(result, projected)
			continue
		}
		buf, readErr := io.ReadAll(io.LimitReader(resp.Body, int64(MaxAttachmentDownloadBytes)+1))
		resp.Body.Close()
		if readErr != nil {
			projected["error"] = fmt.Sprintf("read image failed: %v", readErr)
		} else if len(buf) > MaxAttachmentDownloadBytes {
			projected["error"] = fmt.Sprintf("image exceeds %d MB download limit", MaxAttachmentDownloadBytes/1024/1024)
		} else {
			projected["data"] = base64.StdEncoding.EncodeToString(buf)
			if _, ok := projected["content_type"]; !ok {
				projected["content_type"] = resp.Header.Get("Content-Type")
			}
		}
		result = append(result, projected)
	}
	return result
}
