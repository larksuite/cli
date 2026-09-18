// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

type stickerConverter struct{}

func (stickerConverter) Convert(_ *ConvertContext) string { return "[Sticker]" }

type videoChatConverter struct{}

func (videoChatConverter) Convert(_ *ConvertContext) string { return "[Video call]" }

type shareChatConverter struct{}

func (shareChatConverter) Convert(ctx *ConvertContext) string {
	parsed, err := ParseJSONObject(ctx.RawContent)
	if err != nil {
		return invalidJSONPlaceholder("chat card")
	}
	if id, _ := parsed["chat_id"].(string); id != "" {
		return fmt.Sprintf("[Chat card: %s]", id)
	}
	return "[Chat card]"
}

// systemPlaceholderRe matches {word} tokens in system message templates.
var systemPlaceholderRe = regexp.MustCompile(`\{(\w+)}`)

type shareUserConverter struct{}

// Convert converts a share_chat message content JSON to human-readable string.
func (shareUserConverter) Convert(ctx *ConvertContext) string {
	parsed, err := ParseJSONObject(ctx.RawContent)
	if err != nil {
		return invalidJSONPlaceholder("user card")
	}
	if id, _ := parsed["user_id"].(string); id != "" {
		return fmt.Sprintf("[User card: %s]", id)
	}
	return "[User card]"
}

type locationConverter struct{}

func (locationConverter) Convert(ctx *ConvertContext) string {
	parsed, err := ParseJSONObject(ctx.RawContent)
	if err != nil {
		return invalidJSONPlaceholder("location")
	}
	if name, _ := parsed["name"].(string); name != "" {
		return fmt.Sprintf("[Location: %s]", name)
	}
	return "[Location]"
}

type folderConverter struct{}

func (folderConverter) Convert(ctx *ConvertContext) string {
	parsed, err := ParseJSONObject(ctx.RawContent)
	if err != nil {
		return invalidJSONPlaceholder("folder")
	}
	key, _ := parsed["file_key"].(string)
	if key == "" {
		return "[Folder]"
	}
	name, _ := parsed["file_name"].(string)

	// Fast path: caller pre-fetched this folder's expansion XML (see
	// PrefetchFolderChildren) keyed by message_id — reuse it, no GET.
	// "" as the cached value means "prefetch tried and failed": degrade to the
	// single-line output without an inline retry, so a page-wide failure (missing
	// scope, DLP deny, …) does not double every folder GET inside the render loop.
	if ctx.FolderChildren != nil && ctx.MessageID != "" {
		if xml, ok := ctx.FolderChildren[ctx.MessageID]; ok {
			if xml == "" {
				return fallbackFolderLine(key, name)
			}
			return xml
		}
	}
	// Inline path: expand one level via openapi folder-children (recursive=false).
	// Requires Runtime + MessageID; falls back to the old single-line output when
	// unavailable or on failure.
	if ctx.Runtime != nil && ctx.MessageID != "" {
		if tree := fetchFolderChildrenTree(ctx.Runtime, key, name, ctx.MessageID); tree != "" {
			return tree
		}
	}
	return fallbackFolderLine(key, name)
}

// fallbackFolderLine renders the pre-expansion single-line folder tag used when
// expansion data is unavailable (no runtime/message id, API failure, or a
// prefetch that already tried and failed).
func fallbackFolderLine(key, name string) string {
	if name != "" {
		return fmt.Sprintf(`<folder key="%s" name="%s"/>`, cardEscapeAttr(key), cardEscapeAttr(name))
	}
	return fmt.Sprintf(`<folder key="%s"/>`, cardEscapeAttr(key))
}

// folderWarnMu serializes stderr warnings from folder fetches: the same
// function runs both inline (single-threaded) and inside the concurrent
// PrefetchFolderChildren fan-out, so the ErrOut write must be mutex-guarded
// (bytes.Buffer-backed in tests, where a data race would fail -race CI).
var folderWarnMu sync.Mutex

func folderWarnf(runtime *common.RuntimeContext, format string, args ...interface{}) {
	folderWarnMu.Lock()
	defer folderWarnMu.Unlock()
	fmt.Fprintf(runtime.IO().ErrOut, format, args...)
}

// fetchFolderChildrenTree calls the folder-children openapi to expand one level and returns
// XML (folder child_count hint / file key+name). Returns "" on failure so callers fall back.
func fetchFolderChildrenTree(runtime *common.RuntimeContext, folderKey, folderName, messageID string) string {
	data, err := runtime.DoAPIJSONTyped(http.MethodGet,
		"/open-apis/im/v1/files/"+validate.EncodePathSegment(folderKey)+"/folder",
		larkcore.QueryParams{
			"srctype":   []string{"message"},
			"srcid":     []string{messageID},
			"recursive": []string{"false"},
		}, nil)
	// DoAPIJSONTyped unwraps the envelope and returns the response body's data
	// field; non-zero envelope codes already surface as err (BuildAPIError), so
	// no code re-check is needed here.
	if err != nil {
		folderWarnf(runtime, "warning: folder_fetch_failed: %s: %v\n", folderKey, err)
		return ""
	}
	if data == nil {
		folderWarnf(runtime, "warning: folder_fetch_failed: %s: empty data\n", folderKey)
		return ""
	}
	rawItems, _ := data["items"].([]interface{})
	allCount := numToInt64(data["all_count"])
	// One level only, capped at maxFolderChildren rendered items: files use <file key name/>;
	// sub-folders <folder key name child_count/> (no recursion; child_count hints deeper
	// levels). Root folder carries child_count (=all_count) + has_more when more children
	// remain than were rendered (cap reached or all_count > items returned). A genuinely
	// empty folder (items + all_count both 0) keeps child_count="0".
	const maxFolderChildren = 10
	hasMore := allCount > int64(len(rawItems)) // server may cap items below all_count
	shown := len(rawItems)
	if shown > maxFolderChildren {
		shown = maxFolderChildren
		hasMore = true // rendering cap reached
	}
	var b strings.Builder
	writeFolderOpen(&b, folderKey, folderName, allCount, shown == 0 && allCount == 0, hasMore, shown == 0)
	if shown == 0 {
		// No renderable children: self-closing tag (empty folder -> child_count="0";
		// server said all_count > 0 but returned no items -> child_count=all_count +
		// has_more). Self-closing avoids an odd empty open/close pair.
		return b.String()
	}
	for i := 0; i < shown; i++ {
		raw := rawItems[i]
		item, _ := raw.(map[string]interface{})
		k, _ := item["file_key"].(string)
		n, _ := item["name"].(string)
		isFolder, _ := item["is_folder"].(bool)
		if isFolder {
			fmt.Fprintf(&b, `<folder key="%s" name="%s"`, cardEscapeAttr(k), cardEscapeAttr(n))
			if cc := numToInt64(item["children_count"]); cc > 0 {
				fmt.Fprintf(&b, ` child_count="%d"`, cc)
			}
			b.WriteString(`/>`)
		} else {
			fmt.Fprintf(&b, `<file key="%s" name="%s"/>`, cardEscapeAttr(k), cardEscapeAttr(n))
		}
	}
	b.WriteString("</folder>")
	return b.String()
}

// writeFolderOpen writes the root <folder ...> opening tag: key + name attrs,
// then child_count (= allCount; forceChildZero renders an explicit "0" for a
// genuinely empty folder), then has_more, closing with "/>" when selfClose.
func writeFolderOpen(b *strings.Builder, folderKey, folderName string, childCount int64, forceChildZero, hasMore, selfClose bool) {
	b.WriteString(`<folder key="` + cardEscapeAttr(folderKey) + `"`)
	if folderName != "" {
		b.WriteString(` name="` + cardEscapeAttr(folderName) + `"`)
	}
	if forceChildZero || childCount > 0 {
		fmt.Fprintf(b, ` child_count="%d"`, childCount)
	}
	if hasMore {
		b.WriteString(` has_more="true"`)
	}
	if selfClose {
		b.WriteString(`/>`)
	} else {
		b.WriteString(`>`)
	}
}

// numToInt64 converts a JSON numeric value to int64 (json.Number is what the JSON
// decoder produces under dec.UseNumber(); other numeric kinds are tolerated).
func numToInt64(v interface{}) int64 {
	switch n := v.(type) {
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return i
		}
	case float64:
		return int64(n)
	case float32:
		return int64(n)
	case int:
		return int64(n)
	case int64:
		return n
	}
	return 0
}

// folderPrefetchConcurrency bounds the folder-children fan-out. A message-list
// page rarely carries more than a handful of folder messages, and each folder
// expansion is a single ~1 RTT GET, so 8 concurrent workers comfortably drain
// one page without stampeding the gateway; the result cache also makes later
// renders of the same message free. Serial fallbacks still apply when a caller
// skips prefetching.
const folderPrefetchConcurrency = 8

// folderCacheKeySep separates message_id from folder file_key in the prefetch
// cache for post-message folder attachments (a post may carry several folders,
// so message_id alone is not a unique cache key). Folder messages use the bare
// message_id as their key.
const folderCacheKeySep = "\x00"

// PrefetchFolderChildren scans rawItems for folder messages (msg_type == folder,
// keyed by message_id) and for folder attachments inside post messages (keyed
// by message_id + folderCacheKeySep + folder key), concurrently expanding each
// one level (bounded by folderPrefetchConcurrency) and returning the expansion
// XML. Callers thread the map through FormatMessageItemWithFolderPrefetchOpts
// so the per-item folder/post converters reuse cached XML instead of issuing N
// serial GETs inside the FormatMessageItem loop.
//
// A "" value means the prefetch already tried and failed: converters degrade to
// the single-line tag without an inline retry (so page-wide failures — missing
// scope, DLP deny — do not double every folder GET). A single-entry fast path
// avoids goroutine overhead.
func PrefetchFolderChildren(runtime *common.RuntimeContext, rawItems []interface{}) map[string]string {
	if runtime == nil || len(rawItems) == 0 {
		return nil
	}
	type folderRef struct{ cacheKey, id, key, name string }
	var refs []folderRef
	for _, item := range rawItems {
		m, _ := item.(map[string]interface{})
		if m == nil {
			continue
		}
		mt, _ := m["msg_type"].(string)
		if mt == "folder" {
			// Standalone folder message: content carries one file_key; the whole
			// expansion is keyed by message_id.
			id, _ := m["message_id"].(string)
			if id == "" {
				continue
			}
			key, name := "", ""
			if body, ok := m["body"].(map[string]interface{}); ok {
				if c, ok := body["content"].(string); ok {
					if parsed, err := ParseJSONObject(c); err == nil {
						key, _ = parsed["file_key"].(string)
						name, _ = parsed["file_name"].(string)
					}
				}
			}
			if key == "" {
				continue
			}
			refs = append(refs, folderRef{cacheKey: id, id: id, key: key, name: name})
			continue
		}
		if mt == "post" {
			// Post message: folder attachments live in the attachment zone
			// ("files": [{file_key, file_name, is_folder}]). Each folder is a
			// separate expansion keyed by message_id + folder_key.
			id, _ := m["message_id"].(string)
			if id == "" {
				continue
			}
			if body, ok := m["body"].(map[string]interface{}); ok {
				if c, ok := body["content"].(string); ok {
					if parsed, err := ParseJSONObject(c); err == nil {
						if rawFiles, ok := parsed["files"].([]interface{}); ok {
							for _, raw := range rawFiles {
								f, _ := raw.(map[string]interface{})
								if f == nil {
									continue
								}
								if isF, _ := f["is_folder"].(bool); !isF {
									continue
								}
								key, _ := f["file_key"].(string)
								name, _ := f["file_name"].(string)
								if key == "" {
									continue
								}
								refs = append(refs, folderRef{cacheKey: id + folderCacheKeySep + key, id: id, key: key, name: name})
							}
						}
					}
				}
			}
		}
	}
	if len(refs) == 0 {
		return nil
	}

	result := make(map[string]string, len(refs))
	if len(refs) == 1 {
		// Single-entry fast path; "" records "tried and failed" so the converter
		// skips an inline retry (systemic failures would otherwise double GETs).
		result[refs[0].cacheKey] = fetchFolderChildrenTree(runtime, refs[0].key, refs[0].name, refs[0].id)
		return result
	}

	var mu sync.Mutex
	sem := make(chan struct{}, folderPrefetchConcurrency)
	var wg sync.WaitGroup
	for _, ref := range refs {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			xml := fetchFolderChildrenTree(runtime, ref.key, ref.name, ref.id)
			mu.Lock()
			result[ref.cacheKey] = xml // "" = tried and failed: converter degrades, no retry
			mu.Unlock()
		}()
	}
	wg.Wait()
	return result
}

type calendarEventConverter struct{}

// Convert converts a share_calendar_event message content JSON to human-readable string.
// Includes open_calendar_id and open_event_id as XML attributes so agents can look up the event.
func (calendarEventConverter) Convert(ctx *ConvertContext) string {
	parsed, err := ParseJSONObject(ctx.RawContent)
	if err != nil {
		return invalidJSONPlaceholder("calendar")
	}
	calendarID, _ := parsed["open_calendar_id"].(string)
	eventID, _ := parsed["open_event_id"].(string)
	var attrs string
	if calendarID != "" {
		attrs += fmt.Sprintf(` open_calendar_id="%s"`, cardEscapeAttr(calendarID))
	}
	if eventID != "" {
		attrs += fmt.Sprintf(` open_event_id="%s"`, cardEscapeAttr(eventID))
	}
	attrs += calendarShareTokenAttr(parsed)
	return formatCalendarContent(parsed, "calendar_share", attrs)
}

type calendarInviteConverter struct{}

// Convert converts a calendar message content JSON to human-readable string.
func (calendarInviteConverter) Convert(ctx *ConvertContext) string {
	parsed, err := ParseJSONObject(ctx.RawContent)
	if err != nil {
		return invalidJSONPlaceholder("calendar")
	}
	return formatCalendarContent(parsed, "calendar_invite", "")
}

type generalCalendarConverter struct{}

func (generalCalendarConverter) Convert(ctx *ConvertContext) string {
	parsed, err := ParseJSONObject(ctx.RawContent)
	if err != nil {
		return invalidJSONPlaceholder("calendar")
	}
	return formatCalendarContent(parsed, "calendar", calendarShareTokenAttr(parsed))
}

func calendarShareTokenAttr(parsed map[string]interface{}) string {
	shareToken, _ := parsed["share_token"].(string)
	if shareToken == "" {
		return ""
	}
	return fmt.Sprintf(` share_token="%s"`, cardEscapeAttr(shareToken))
}

// formatCalendarContent builds a human-readable string from a calendar JSON object.
// Expected fields: summary (string), start_time (epoch string), end_time (epoch string).
// extraAttrs is an optional string of XML attributes (e.g. ` open_event_id="xxx"`) appended to the opening tag.
func formatCalendarContent(parsed map[string]interface{}, tag, extraAttrs string) string {
	summary, _ := parsed["summary"].(string)
	startTime, _ := parsed["start_time"].(string)
	endTime, _ := parsed["end_time"].(string)

	var inner []string
	if summary != "" {
		inner = append(inner, summary)
	}

	start := formatTimestamp(startTime)
	end := formatTimestamp(endTime)
	if start != "" && end != "" {
		inner = append(inner, start+" ~ "+end)
	} else if start != "" {
		inner = append(inner, start)
	}

	body := strings.Join(inner, "\n")
	if body == "" {
		body = tag
	}
	return fmt.Sprintf("<%s%s>\n%s\n</%s>", tag, extraAttrs, xmlEscapeBody(body), tag)
}

type voteConverter struct{}

func (voteConverter) Convert(ctx *ConvertContext) string {
	parsed, err := ParseJSONObject(ctx.RawContent)
	if err != nil {
		return invalidJSONPlaceholder("vote")
	}
	topic, _ := parsed["topic"].(string)

	var inner []string
	if topic != "" {
		inner = append(inner, topic)
	}
	if opts, ok := parsed["options"].([]interface{}); ok {
		for _, o := range opts {
			if s, ok := o.(string); ok && s != "" {
				inner = append(inner, "• "+s)
			}
		}
	}
	// status: 0 = open, non-zero = closed (based on internal VoteStatus enum)
	if status, ok := parsed["status"].(float64); ok && status != 0 {
		inner = append(inner, "(Closed)")
	}

	body := strings.Join(inner, "\n")
	if body == "" {
		body = "vote"
	}
	return fmt.Sprintf("<vote>\n%s\n</vote>", xmlEscapeBody(body))
}

type hongbaoConverter struct{}

func (hongbaoConverter) Convert(ctx *ConvertContext) string {
	parsed, err := ParseJSONObject(ctx.RawContent)
	if err != nil {
		return invalidJSONPlaceholder("hongbao")
	}
	if text, _ := parsed["text"].(string); text != "" {
		return fmt.Sprintf(`<hongbao text=%q/>`, text)
	}
	return "<hongbao/>"
}

type todoConverter struct{}

func (todoConverter) Convert(ctx *ConvertContext) string {
	parsed, err := ParseJSONObject(ctx.RawContent)
	if err != nil {
		return invalidJSONPlaceholder("todo")
	}

	taskID, _ := parsed["task_id"].(string)
	var taskAttr string
	if taskID != "" {
		taskAttr = fmt.Sprintf(` task_id="%s"`, cardEscapeAttr(taskID))
	}

	var inner []string
	if summary, ok := parsed["summary"].(map[string]interface{}); ok {
		if title, _ := summary["title"].(string); title != "" {
			inner = append(inner, title)
		}
		if blocks, ok := summary["content"].([]interface{}); ok {
			if text := extractPostBlocksText(blocks); text != "" {
				inner = append(inner, text)
			}
		}
	}
	if dueTime, _ := parsed["due_time"].(string); dueTime != "" {
		if formatted := formatTimestamp(dueTime); formatted != "" {
			inner = append(inner, "Due: "+formatted)
		}
	}

	body := strings.Join(inner, "\n")
	if body == "" {
		body = "todo"
	}
	return fmt.Sprintf("<todo%s>\n%s\n</todo>", taskAttr, xmlEscapeBody(body))
}

type systemConverter struct{}

func (systemConverter) Convert(ctx *ConvertContext) string {
	parsed, err := ParseJSONObject(ctx.RawContent)
	if err != nil {
		return invalidJSONPlaceholder("system message")
	}

	tmpl, _ := parsed["template"].(string)
	if tmpl == "" {
		return "[System message]"
	}

	content := tmpl

	if fromUsers, ok := parsed["from_user"].([]interface{}); ok {
		var names []string
		for _, u := range fromUsers {
			if s, ok := u.(string); ok && s != "" {
				names = append(names, s)
			}
		}
		content = strings.ReplaceAll(content, "{from_user}", strings.Join(names, ", "))
	} else {
		content = strings.ReplaceAll(content, "{from_user}", "")
	}

	if toChatters, ok := parsed["to_chatters"].([]interface{}); ok {
		var names []string
		for _, u := range toChatters {
			if s, ok := u.(string); ok && s != "" {
				names = append(names, s)
			}
		}
		content = strings.ReplaceAll(content, "{to_chatters}", strings.Join(names, ", "))
	} else {
		content = strings.ReplaceAll(content, "{to_chatters}", "")
	}

	if divider, ok := parsed["divider_text"].(map[string]interface{}); ok {
		text, _ := divider["text"].(string)
		content = strings.ReplaceAll(content, "{divider_text}", text)
	} else {
		content = strings.ReplaceAll(content, "{divider_text}", "")
	}

	// Generic pass: replace any remaining {key} placeholders with matching
	// string-typed fields in the JSON (e.g. {name}, {operator}).
	content = systemPlaceholderRe.ReplaceAllStringFunc(content, func(match string) string {
		key := match[1 : len(match)-1]
		if val, _ := parsed[key].(string); val != "" {
			return val
		}
		return match // preserve unknown placeholders intact
	})

	return strings.TrimSpace(content)
}
