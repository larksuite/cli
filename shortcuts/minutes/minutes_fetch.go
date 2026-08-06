// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/note"
)

var minutesFetchIncludes = map[string]bool{"transcript": true, "note-doc": true}

// MinutesResult contains rendered content, metadata, and opt-in extras.
type MinutesResult struct {
	Content          string
	Title            string
	CreateTime       string
	NoteID           string
	NoteDocToken     string
	VerbatimDocToken string
	Warnings         []string
}

// FetchMinutesMarkdown renders metadata and artifacts as Markdown. Optional
// transcript and note-document extras degrade to warnings without losing the body.
func FetchMinutesMarkdown(ctx context.Context, runtime *common.RuntimeContext, token string, include map[string]bool) (*MinutesResult, error) {
	// 1. metadata: title / note_id / create_time
	metaData, err := runtime.DoAPIJSONTyped(http.MethodGet,
		fmt.Sprintf("/open-apis/minutes/v1/minutes/%s", validate.EncodePathSegment(token)), nil, nil)
	if err != nil {
		return nil, err
	}
	minute, _ := metaData["minute"].(map[string]any)
	if minute == nil {
		return nil, errs.NewAPIError(errs.SubtypeNotFound, "minute not found: %s", token)
	}
	title := common.GetString(minute, "title")
	noteID := common.GetString(minute, "note_id")
	createTime := common.FormatTime(minute["create_time"])

	// 2. AI artifacts: summary / chapters / todos / keywords (core content)
	art, err := runtime.DoAPIJSONTyped(http.MethodGet,
		fmt.Sprintf("/open-apis/minutes/v1/minutes/%s/artifacts", validate.EncodePathSegment(token)), nil, nil)
	if err != nil {
		return nil, err
	}
	content := renderMinutesMarkdown(
		title,
		common.GetString(art, "summary"),
		common.GetSlice(art, "minute_chapters"),
		common.GetSlice(art, "minute_todos"),
		common.GetSlice(art, "keywords"),
	)

	result := &MinutesResult{Content: content, Title: title, CreateTime: createTime}

	// 3. Optional artifacts never make the core body fail.
	if include["transcript"] {
		if transcript := strings.TrimSpace(common.GetString(art, "transcript")); transcript != "" {
			result.Content = appendSection(result.Content, "## 逐字稿", transcript)
		} else {
			result.Warnings = append(result.Warnings, "transcript omitted: the artifacts response did not include transcript content")
		}
	}
	if include["note-doc"] {
		result.NoteID = noteID
		if noteID == "" {
			result.Warnings = append(result.Warnings, "note documents omitted: the minute metadata did not include note_id")
		} else if err := runtime.EnsureScopes([]string{"vc:note:read"}); err != nil {
			result.Warnings = append(result.Warnings, optionalMinutesWarning(runtime, "note documents omitted", err))
		} else {
			detail, err := note.FetchDetail(ctx, runtime, noteID)
			if err != nil {
				result.Warnings = append(result.Warnings, optionalMinutesWarning(runtime, "note documents omitted", err))
			} else {
				result.NoteDocToken = detail.NoteDocToken
				result.VerbatimDocToken = detail.VerbatimDocToken
				if result.NoteDocToken == "" && result.VerbatimDocToken == "" {
					result.Warnings = append(result.Warnings, "note documents omitted: the note detail did not include document tokens")
				}
			}
		}
	}
	return result, nil
}

// optionalMinutesWarning keeps recovery details from a typed error when an
// opt-in Minutes artifact degrades to a warning. Warnings are strings in the
// public envelope, so include the structured hint and log ID when available.
func optionalMinutesWarning(runtime *common.RuntimeContext, prefix string, err error) string {
	err = runtime.PresentError(err)
	warning := fmt.Sprintf("%s: %v", prefix, err)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem == nil {
		return warning
	}
	var context []string
	if hint := strings.TrimSpace(problem.Hint); hint != "" && !strings.Contains(warning, hint) {
		context = append(context, "hint: "+hint)
	}
	if logID := strings.TrimSpace(problem.LogID); logID != "" {
		context = append(context, "log_id: "+logID)
	}
	if len(context) > 0 {
		warning += " (" + strings.Join(context, "; ") + ")"
	}
	return warning
}

// ParseIncludes parses the --include CSV into a set, rejecting unknown values.
func ParseIncludes(raw string) (map[string]bool, error) {
	set := map[string]bool{}
	for _, v := range common.SplitCSV(raw) {
		if !minutesFetchIncludes[v] {
			return nil, common.ValidationErrorf("invalid --include value %q (allowed: transcript, note-doc)", v)
		}
		set[v] = true
	}
	return set, nil
}

// renderMinutesMarkdown assembles title, summary, chapters, todos, and keywords.
func renderMinutesMarkdown(title, summary string, chapters, todos, keywords []interface{}) string {
	var b strings.Builder
	if t := strings.TrimSpace(title); t != "" {
		b.WriteString("# ")
		b.WriteString(t)
	}
	body := b.String()
	body = appendSection(body, "## 总结", summary)
	body = appendSection(body, "## 章节", renderChapters(chapters))
	body = appendSection(body, "## 待办", renderTodos(todos))
	body = appendSection(body, "## 关键词", renderKeywords(keywords))
	return body
}

// appendSection appends `heading\n\n<body>` to base when body is non-empty,
// separating from existing content with a blank line.
func appendSection(base, heading, body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return base
	}
	var b strings.Builder
	b.WriteString(base)
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	b.WriteString(heading)
	b.WriteString("\n\n")
	b.WriteString(body)
	return b.String()
}

// renderChapters renders each chapter as `### title` + summary. Chapters are
// sorted by start timestamp only when every chapter carries one; if any chapter
// lacks a timestamp the API order is preserved (a 0 fallback would mis-order
// timestamp-less chapters ahead of timed ones).
func renderChapters(chapters []interface{}) string {
	maps := make([]map[string]interface{}, 0, len(chapters))
	for _, c := range chapters {
		if m, ok := c.(map[string]interface{}); ok {
			maps = append(maps, m)
		}
	}
	// Missing timestamps keep API order; treating them as zero would misorder chapters.
	allTimed := len(maps) > 0
	for _, m := range maps {
		if _, ok := chapterStartMs(m); !ok {
			allTimed = false
			break
		}
	}
	if allTimed && len(maps) > 1 {
		sort.SliceStable(maps, func(i, j int) bool {
			mi, _ := chapterStartMs(maps[i])
			mj, _ := chapterStartMs(maps[j])
			return mi < mj
		})
	}

	var parts []string
	for _, ch := range maps {
		title := strings.TrimSpace(common.GetString(ch, "title"))
		summary := strings.TrimSpace(common.GetString(ch, "summary_content"))
		var seg strings.Builder
		if title != "" {
			seg.WriteString("### ")
			seg.WriteString(title)
		}
		if summary != "" {
			if seg.Len() > 0 {
				seg.WriteString("\n\n")
			}
			seg.WriteString(summary)
		}
		if seg.Len() > 0 {
			parts = append(parts, seg.String())
		}
	}
	return strings.Join(parts, "\n\n")
}

// renderTodos renders todos as a bullet list, each collapsed to a single line.
func renderTodos(todos []interface{}) string {
	var lines []string
	common.EachMap(todos, func(td map[string]interface{}) {
		if content := strings.Join(strings.Fields(common.GetString(td, "content")), " "); content != "" {
			lines = append(lines, "- "+content)
		}
	})
	return strings.Join(lines, "\n")
}

// renderKeywords joins keyword strings with a Chinese enumeration comma.
func renderKeywords(keywords []interface{}) string {
	var kw []string
	for _, k := range keywords {
		if s, ok := k.(string); ok {
			if s = strings.TrimSpace(s); s != "" {
				kw = append(kw, s)
			}
		}
	}
	return strings.Join(kw, "、")
}

// chapterStartMs returns a chapter's start time in ms and whether a usable
// timestamp was found. It tries the candidate field names the artifacts API may
// use; start_ms arrives as a numeric *string* ("92000"), so strings are parsed
// too. ok=false lets renderChapters keep API order for a timestamp-less chapter
// instead of sorting it (as 0) ahead of every timed chapter.
func chapterStartMs(ch map[string]interface{}) (float64, bool) {
	for _, k := range []string{"start_ms", "start_time", "start", "timestamp", "begin_time"} {
		switch n := ch[k].(type) {
		case string:
			if f, err := strconv.ParseFloat(strings.TrimSpace(n), 64); err == nil {
				return f, true
			}
		case float64:
			return n, true
		case int:
			return float64(n), true
		case int64:
			return float64(n), true
		}
	}
	return 0, false
}
