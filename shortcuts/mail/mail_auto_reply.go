// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
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
		{Name: "from", Default: "me", Desc: "Mailbox address (default: me)."},
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
		return outputAutoReply(runtime, data, "")
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
		{Name: "from", Default: "me", Desc: "Mailbox address (default: me)."},
		{Name: "enable", Type: "bool", Desc: "Turn auto-reply on."},
		{Name: "disable", Type: "bool", Desc: "Turn auto-reply off."},
		{Name: "content", Desc: "Auto-reply HTML content. Plain text is accepted and sent as-is. Supports @file and - stdin.", Input: []string{common.File, common.Stdin}},
		{Name: "content-file", Desc: "Read auto-reply content from a file path. Mutually exclusive with --content."},
		{Name: "summary", Desc: "Plain-text content summary. Defaults to a preview generated from --content/--content-file."},
		{Name: "start", Desc: "Start date as Unix timestamp or ISO 8601. Stored as the day's 00:00:00.000."},
		{Name: "end", Desc: "End date as Unix timestamp or ISO 8601. Stored as the day's 23:59:59.999."},
		{Name: "timezone", Desc: "Time zone for the auto-reply range, e.g. Asia/Shanghai. Defaults to the start time zone when it can be inferred."},
		{Name: "internal-only", Type: "bool", Desc: "Only send auto-replies to tenant-internal senders."},
		{Name: "external", Type: "bool", Desc: "Send auto-replies to external senders too."},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		mailboxID := resolveAutoReplyMailboxID(runtime)
		patch, _ := buildAutoReplyPatch(runtime)
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
		if runtime.Bool("internal-only") && runtime.Bool("external") {
			return mailValidationError("--internal-only and --external are mutually exclusive").
				WithParams(
					mailInvalidParam("--internal-only", "mutually exclusive with --external"),
					mailInvalidParam("--external", "mutually exclusive with --internal-only"),
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
		_, err := buildAutoReplyPatch(runtime)
		return err
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		mailboxID := resolveAutoReplyMailboxID(runtime)
		patch, err := buildAutoReplyPatch(runtime)
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
		return outputAutoReply(runtime, data, "Auto-reply modified.")
	},
}

func autoReplyPath(mailboxID string) string {
	return mailboxPath(mailboxID, "settings", "auto_reply")
}

func resolveAutoReplyMailboxID(runtime *common.RuntimeContext) string {
	if from := strings.TrimSpace(runtime.Str("from")); from != "" {
		return from
	}
	return "me"
}

func autoReplyHasModify(runtime *common.RuntimeContext) bool {
	for _, flag := range []string{
		"enable", "disable", "content", "content-file", "summary",
		"start", "end", "timezone", "internal-only", "external",
	} {
		if runtime.Changed(flag) {
			return true
		}
	}
	return false
}

func buildAutoReplyPatch(runtime *common.RuntimeContext) (map[string]interface{}, error) {
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
	if content != "" {
		autoReply["content_html"] = content
		if strings.TrimSpace(runtime.Str("summary")) == "" {
			autoReply["content_summary"] = contentPreview(content, 200, resolveLang(runtime))
		}
	}
	if summary := strings.TrimSpace(runtime.Str("summary")); summary != "" {
		autoReply["content_summary"] = summary
	}
	timezone := strings.TrimSpace(runtime.Str("timezone"))
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
	if runtime.Bool("external") {
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

func outputAutoReply(runtime *common.RuntimeContext, data map[string]interface{}, message string) error {
	autoReply := data
	if nested, ok := data["auto_reply"].(map[string]interface{}); ok {
		autoReply = nested
	}
	autoReply = normalizeAutoReplyFields(autoReply)
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
