// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package markdown

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/sergi/go-diff/diffmatchpatch"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	markdownDiffModeRemoteVsRemote = "remote_vs_remote"
	markdownDiffModeRemoteVsLocal  = "remote_vs_local"
	markdownDiffMaxContentBytes    = 10 * 1024 * 1024
	markdownDiffTimeout            = 30 * time.Second
)

var markdownDiffVersionRe = regexp.MustCompile(`^\d{1,19}$`)

type markdownDiffSpec struct {
	FileToken    string
	FromVersion  string
	ToVersion    string
	FilePath     string
	ContextLines int
	Format       string
}

type markdownDiffHunk struct {
	Header   string `json:"header"`
	OldStart int    `json:"old_start"`
	OldLines int    `json:"old_lines"`
	NewStart int    `json:"new_start"`
	NewLines int    `json:"new_lines"`
}

type markdownDiffLineKind int

const (
	markdownDiffLineEqual markdownDiffLineKind = iota
	markdownDiffLineDelete
	markdownDiffLineInsert
)

type markdownDiffLineOp struct {
	Kind    markdownDiffLineKind
	Content string
}

type markdownDiffHunkRange struct {
	Start int
	End   int
}

func validateMarkdownDiffSpec(runtime *common.RuntimeContext, spec markdownDiffSpec) error {
	if err := validate.ResourceName(spec.FileToken, "--file-token"); err != nil {
		return output.ErrValidation("%s", err)
	}
	if spec.FromVersion != "" {
		if err := validateMarkdownDiffVersionValue(spec.FromVersion, "--from-version"); err != nil {
			return err
		}
	}
	if spec.ToVersion != "" {
		if err := validateMarkdownDiffVersionValue(spec.ToVersion, "--to-version"); err != nil {
			return err
		}
	}
	if spec.FilePath != "" {
		if _, err := validate.SafeInputPath(spec.FilePath); err != nil {
			return output.ErrValidation("unsafe file path: %s", err)
		}
		if err := validateMarkdownFileName(spec.FilePath, "--file"); err != nil {
			return err
		}
	}
	if spec.ContextLines < 0 {
		return output.ErrValidation("--context-lines must be >= 0")
	}
	if spec.Format != "" && spec.Format != "json" && spec.Format != "pretty" {
		return output.ErrValidation("markdown +diff only supports --format json or pretty")
	}
	if spec.FilePath == "" {
		if spec.FromVersion == "" && spec.ToVersion == "" {
			return common.FlagErrorf("specify --from-version, or both --from-version and --to-version, or use --file for remote vs local diff")
		}
		if spec.FromVersion == "" && spec.ToVersion != "" {
			return common.FlagErrorf("--to-version requires --from-version")
		}
		return nil
	}
	if spec.ToVersion != "" {
		return common.FlagErrorf("--to-version is not supported together with --file")
	}
	return nil
}

func validateMarkdownDiffVersionValue(value, flagName string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return output.ErrValidation("%s cannot be empty", flagName)
	}
	if !markdownDiffVersionRe.MatchString(value) {
		return output.ErrValidation("%s must be a numeric version string", flagName)
	}
	return nil
}

func markdownDiffMode(spec markdownDiffSpec) string {
	if spec.FilePath != "" {
		return markdownDiffModeRemoteVsLocal
	}
	return markdownDiffModeRemoteVsRemote
}

func markdownDiffDryRun(spec markdownDiffSpec) *common.DryRunAPI {
	dry := common.NewDryRunAPI().Desc("Download the requested Markdown content, compute a unified diff locally, and print the result without modifying the remote file")
	switch markdownDiffMode(spec) {
	case markdownDiffModeRemoteVsLocal:
		if spec.FromVersion != "" {
			dry.GET("/open-apis/drive/v1/files/:file_token/download").
				Desc("[1] Download the specified remote Markdown version").
				Set("file_token", spec.FileToken).
				Params(map[string]interface{}{"version": spec.FromVersion})
		} else {
			dry.GET("/open-apis/drive/v1/files/:file_token/download").
				Desc("[1] Download the latest remote Markdown version").
				Set("file_token", spec.FileToken)
		}
		dry.Set("local_file", spec.FilePath)
		dry.Set("mode", markdownDiffModeRemoteVsLocal)
	default:
		dry.GET("/open-apis/drive/v1/files/:file_token/download").
			Desc("[1] Download the base remote Markdown version").
			Set("file_token", spec.FileToken).
			Params(map[string]interface{}{"version": spec.FromVersion})
		if spec.ToVersion != "" {
			dry.GET("/open-apis/drive/v1/files/:file_token/download").
				Desc("[2] Download the target remote Markdown version").
				Set("file_token", spec.FileToken).
				Params(map[string]interface{}{"version": spec.ToVersion})
		} else {
			dry.GET("/open-apis/drive/v1/files/:file_token/download").
				Desc("[2] Download the latest remote Markdown version").
				Set("file_token", spec.FileToken)
		}
		dry.Set("mode", markdownDiffModeRemoteVsRemote)
	}
	dry.Set("context_lines", spec.ContextLines)
	return dry
}

func downloadMarkdownContent(ctx context.Context, runtime *common.RuntimeContext, fileToken, version string) (string, string, error) {
	resp, fileName, err := openMarkdownDownloadVersion(ctx, runtime, fileToken, version)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	payload, err := readMarkdownDiffPayload(resp.Body, "remote Markdown content")
	if err != nil {
		return "", "", wrapMarkdownDownloadError(err)
	}
	return fileName, string(payload), nil
}

func readMarkdownLocalFile(runtime *common.RuntimeContext, filePath string) (string, error) {
	f, err := runtime.FileIO().Open(filePath)
	if err != nil {
		return "", common.WrapInputStatError(err)
	}
	defer f.Close()

	payload, err := readMarkdownDiffPayload(f, "local Markdown file")
	if err != nil {
		var exitErr *output.ExitError
		if errors.As(err, &exitErr) {
			return "", err
		}
		return "", output.ErrValidation("cannot read file: %s", err)
	}
	return string(payload), nil
}

func readMarkdownDiffPayload(r io.Reader, source string) ([]byte, error) {
	payload, err := io.ReadAll(io.LimitReader(r, markdownDiffMaxContentBytes+1))
	if err != nil {
		return nil, err
	}
	if len(payload) > markdownDiffMaxContentBytes {
		return nil, output.ErrValidation("%s exceeds %s markdown +diff content limit", source, common.FormatSize(markdownDiffMaxContentBytes))
	}
	return payload, nil
}

func splitMarkdownDiffLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.SplitAfter(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func markdownDiffLineOps(fromContent, toContent string) []markdownDiffLineOp {
	dmp := diffmatchpatch.New()
	dmp.DiffTimeout = markdownDiffTimeout
	before, after, lineArray := dmp.DiffLinesToRunes(fromContent, toContent)
	diffs := dmp.DiffMainRunes(before, after, false)
	// Keep the diff line-based. Running cleanup after hydrating real text
	// would re-split replacements into word-level edits.
	diffs = dmp.DiffCharsToLines(diffs, lineArray)

	ops := make([]markdownDiffLineOp, 0, len(diffs))
	for _, diff := range diffs {
		lines := splitMarkdownDiffLines(diff.Text)
		for _, line := range lines {
			switch diff.Type {
			case diffmatchpatch.DiffDelete:
				ops = append(ops, markdownDiffLineOp{Kind: markdownDiffLineDelete, Content: line})
			case diffmatchpatch.DiffInsert:
				ops = append(ops, markdownDiffLineOp{Kind: markdownDiffLineInsert, Content: line})
			default:
				ops = append(ops, markdownDiffLineOp{Kind: markdownDiffLineEqual, Content: line})
			}
		}
	}
	return ops
}

func markdownDiffSummary(ops []markdownDiffLineOp) (bool, int, int) {
	added := 0
	deleted := 0
	changed := false
	for _, op := range ops {
		switch op.Kind {
		case markdownDiffLineDelete:
			changed = true
			deleted++
		case markdownDiffLineInsert:
			changed = true
			added++
		}
	}
	return changed, added, deleted
}

func markdownDiffHunkRanges(ops []markdownDiffLineOp, contextLines int) []markdownDiffHunkRange {
	if len(ops) == 0 {
		return nil
	}
	changedLines := make([]int, 0)
	for i, op := range ops {
		if op.Kind != markdownDiffLineEqual {
			changedLines = append(changedLines, i)
		}
	}
	if len(changedLines) == 0 {
		return nil
	}

	ranges := make([]markdownDiffHunkRange, 0, len(changedLines))
	current := markdownDiffHunkRange{
		Start: max(0, changedLines[0]-contextLines),
		End:   min(len(ops), changedLines[0]+contextLines+1),
	}
	for _, idx := range changedLines[1:] {
		next := markdownDiffHunkRange{
			Start: max(0, idx-contextLines),
			End:   min(len(ops), idx+contextLines+1),
		}
		if next.Start <= current.End {
			if next.End > current.End {
				current.End = next.End
			}
			continue
		}
		ranges = append(ranges, current)
		current = next
	}
	ranges = append(ranges, current)
	return ranges
}

func markdownDiffHunkAt(ops []markdownDiffLineOp, r markdownDiffHunkRange) markdownDiffHunk {
	oldBefore := 0
	newBefore := 0
	for _, op := range ops[:r.Start] {
		if op.Kind != markdownDiffLineInsert {
			oldBefore++
		}
		if op.Kind != markdownDiffLineDelete {
			newBefore++
		}
	}

	oldLines := 0
	newLines := 0
	for _, op := range ops[r.Start:r.End] {
		if op.Kind != markdownDiffLineInsert {
			oldLines++
		}
		if op.Kind != markdownDiffLineDelete {
			newLines++
		}
	}

	oldStart := oldBefore + 1
	newStart := newBefore + 1
	if oldLines == 0 {
		oldStart = oldBefore
	}
	if newLines == 0 {
		newStart = newBefore
	}

	return markdownDiffHunk{
		Header:   fmt.Sprintf("@@ -%d,%d +%d,%d @@", oldStart, oldLines, newStart, newLines),
		OldStart: oldStart,
		OldLines: oldLines,
		NewStart: newStart,
		NewLines: newLines,
	}
}

func buildMarkdownUnifiedDiff(fromLabel, toLabel string, ops []markdownDiffLineOp, ranges []markdownDiffHunkRange) string {
	if len(ranges) == 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "--- %s\n", fromLabel)
	fmt.Fprintf(&b, "+++ %s\n", toLabel)
	for _, r := range ranges {
		hunk := markdownDiffHunkAt(ops, r)
		b.WriteString(hunk.Header)
		b.WriteByte('\n')
		for _, op := range ops[r.Start:r.End] {
			prefix := ' '
			switch op.Kind {
			case markdownDiffLineDelete:
				prefix = '-'
			case markdownDiffLineInsert:
				prefix = '+'
			}
			b.WriteByte(byte(prefix))
			b.WriteString(op.Content)
			if !strings.HasSuffix(op.Content, "\n") {
				b.WriteByte('\n')
				b.WriteString(`\ No newline at end of file`)
				b.WriteByte('\n')
			}
		}
	}
	return b.String()
}

func summarizeMarkdownDiff(fromLabel, toLabel, fromContent, toContent string, contextLines int) (string, bool, int, int, []markdownDiffHunk) {
	ops := markdownDiffLineOps(fromContent, toContent)
	changed, added, deleted := markdownDiffSummary(ops)
	ranges := markdownDiffHunkRanges(ops, contextLines)
	hunks := make([]markdownDiffHunk, 0, len(ranges))
	for _, r := range ranges {
		hunks = append(hunks, markdownDiffHunkAt(ops, r))
	}
	return buildMarkdownUnifiedDiff(fromLabel, toLabel, ops, ranges), changed, added, deleted, hunks
}

func colorizeUnifiedDiff(diffText string) string {
	if diffText == "" {
		return ""
	}
	lines := strings.SplitAfter(diffText, "\n")
	var b strings.Builder
	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\n")
		suffix := ""
		if strings.HasSuffix(line, "\n") {
			suffix = "\n"
		}
		switch {
		case strings.HasPrefix(trimmed, "@@"):
			b.WriteString(output.Cyan)
			b.WriteString(trimmed)
			b.WriteString(output.Reset)
		case strings.HasPrefix(trimmed, "+++"), strings.HasPrefix(trimmed, "---"):
			b.WriteString(output.Bold)
			b.WriteString(trimmed)
			b.WriteString(output.Reset)
		case strings.HasPrefix(trimmed, "+") && !strings.HasPrefix(trimmed, "+++"):
			b.WriteString(output.Green)
			b.WriteString(trimmed)
			b.WriteString(output.Reset)
		case strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, "---"):
			b.WriteString(output.Red)
			b.WriteString(trimmed)
			b.WriteString(output.Reset)
		default:
			b.WriteString(trimmed)
		}
		b.WriteString(suffix)
	}
	return b.String()
}

func prettyPrintMarkdownDiff(w io.Writer, data map[string]interface{}) {
	if !common.GetBool(data, "changed") {
		io.WriteString(w, "No differences.\n")
		return
	}
	io.WriteString(w, colorizeUnifiedDiff(common.GetString(data, "diff")))
}

var MarkdownDiff = common.Shortcut{
	Service:     "markdown",
	Command:     "+diff",
	Description: "Compare remote Markdown versions or compare remote Markdown against a local file",
	Risk:        "read",
	Scopes:      []string{"drive:file:download"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "file-token", Desc: "target Markdown file token", Required: true},
		{Name: "from-version", Desc: "base remote version; when --to-version is omitted, compare this version to the latest remote version"},
		{Name: "to-version", Desc: "target remote version; requires --from-version"},
		{Name: "file", Desc: "local .md file path to compare against the remote content"},
		{Name: "context-lines", Desc: "number of unchanged context lines to include around each diff hunk", Type: "int", Default: "3"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateMarkdownDiffSpec(runtime, markdownDiffSpec{
			FileToken:    strings.TrimSpace(runtime.Str("file-token")),
			FromVersion:  strings.TrimSpace(runtime.Str("from-version")),
			ToVersion:    strings.TrimSpace(runtime.Str("to-version")),
			FilePath:     strings.TrimSpace(runtime.Str("file")),
			ContextLines: runtime.Int("context-lines"),
			Format:       runtime.Format,
		})
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return markdownDiffDryRun(markdownDiffSpec{
			FileToken:    strings.TrimSpace(runtime.Str("file-token")),
			FromVersion:  strings.TrimSpace(runtime.Str("from-version")),
			ToVersion:    strings.TrimSpace(runtime.Str("to-version")),
			FilePath:     strings.TrimSpace(runtime.Str("file")),
			ContextLines: runtime.Int("context-lines"),
		})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec := markdownDiffSpec{
			FileToken:    strings.TrimSpace(runtime.Str("file-token")),
			FromVersion:  strings.TrimSpace(runtime.Str("from-version")),
			ToVersion:    strings.TrimSpace(runtime.Str("to-version")),
			FilePath:     strings.TrimSpace(runtime.Str("file")),
			ContextLines: runtime.Int("context-lines"),
		}

		var (
			fromLabel   string
			toLabel     string
			fromContent string
			toContent   string
			err         error
		)

		switch markdownDiffMode(spec) {
		case markdownDiffModeRemoteVsLocal:
			fromLabel = "a/" + spec.FileToken
			if spec.FromVersion != "" {
				fromLabel += "@version:" + spec.FromVersion
			} else {
				fromLabel += "@latest"
			}
			_, fromContent, err = downloadMarkdownContent(ctx, runtime, spec.FileToken, spec.FromVersion)
			if err != nil {
				return err
			}

			toLabel = "b/" + spec.FilePath
			toContent, err = readMarkdownLocalFile(runtime, spec.FilePath)
			if err != nil {
				return err
			}
		default:
			fromLabel = "a/" + spec.FileToken + "@version:" + spec.FromVersion
			_, fromContent, err = downloadMarkdownContent(ctx, runtime, spec.FileToken, spec.FromVersion)
			if err != nil {
				return err
			}

			if spec.ToVersion != "" {
				toLabel = "b/" + spec.FileToken + "@version:" + spec.ToVersion
				_, toContent, err = downloadMarkdownContent(ctx, runtime, spec.FileToken, spec.ToVersion)
			} else {
				toLabel = "b/" + spec.FileToken + "@latest"
				_, toContent, err = downloadMarkdownContent(ctx, runtime, spec.FileToken, "")
			}
			if err != nil {
				return err
			}
		}

		diffText, changed, addedLines, deletedLines, hunks := summarizeMarkdownDiff(fromLabel, toLabel, fromContent, toContent, spec.ContextLines)

		out := map[string]interface{}{
			"changed":       changed,
			"mode":          markdownDiffMode(spec),
			"file_token":    spec.FileToken,
			"from_version":  spec.FromVersion,
			"to_version":    spec.ToVersion,
			"from_label":    fromLabel,
			"to_label":      toLabel,
			"added_lines":   addedLines,
			"deleted_lines": deletedLines,
			"context_lines": spec.ContextLines,
			"hunks":         hunks,
			"diff":          diffText,
		}
		if spec.FilePath != "" {
			out["local_file"] = spec.FilePath
		}

		runtime.OutFormatRaw(out, nil, func(w io.Writer) {
			prettyPrintMarkdownDiff(w, out)
		})
		return nil
	},
}
