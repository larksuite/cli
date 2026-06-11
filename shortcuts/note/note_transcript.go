// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
//
// note +transcript — fetch the unified note transcript by a
// known note_id. The API is paginated; the CLI walks all pages internally,
// concatenates the content and saves the whole transcript to a local file.

package note

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/i18n"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	transcriptFormatMarkdown  = "markdown"
	transcriptFormatPlainText = "plain_text"

	logPrefix = "[note +transcript]"

	// maxTranscriptPages bounds the pagination loop so a misbehaving has_more
	// can never spin forever. transcriptPageSize reduces round trips; full
	// transcript correctness still depends on has_more/cursor pagination.
	maxTranscriptPages = 500
	transcriptPageSize = 200

	// pageDelay throttles successive page requests to stay gentle on the
	// downstream, matching the batch cadence used by `vc +notes`.
	pageDelay = 100 * time.Millisecond

	// noteArtifactSubdir is the default top-level directory for note-scoped
	// artifacts (parallel to the "minutes" layout used by minute artifacts).
	noteArtifactSubdir = "notes"
)

// NoteTranscript fetches the full unified transcript and saves it to a file.
var NoteTranscript = common.Shortcut{
	Service:     "note",
	Command:     "+transcript",
	Description: "Fetch the unified note transcript and save it to a file",
	Risk:        "read",
	Scopes:      []string{"vc:note:read"},
	AuthTypes:   []string{"user"},
	Flags: []common.Flag{
		{Name: "note-id", Desc: "note ID", Required: true},
		{Name: "transcript-format", Desc: "transcript content format", Default: transcriptFormatMarkdown, Enum: []string{transcriptFormatMarkdown, transcriptFormatPlainText}},
		{Name: "locale", Desc: "transcript locale, e.g. zh_cn, en_us, ja_jp (default follows profile language or brand)"},
		{Name: "output", Desc: "output file path (default: ./notes/{note_id}/unified_transcript.{md,txt})"},
		{Name: "overwrite", Type: "bool", Desc: "overwrite an existing output file"},
	},
	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		noteID := strings.TrimSpace(runtime.Str("note-id"))
		if noteID == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--note-id is required").WithParam("--note-id")
		}
		if err := validate.ResourceName(noteID, "--note-id"); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--note-id").WithCause(err)
		}
		if out := strings.TrimSpace(runtime.Str("output")); out != "" {
			if err := common.ValidateSafePathTyped(runtime.FileIO(), out); err != nil {
				return err
			}
		}
		return nil
	},
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		noteID := strings.TrimSpace(runtime.Str("note-id"))
		transcriptFormat := runtime.Str("transcript-format")
		locale := resolveTranscriptLocale(runtime)
		return common.NewDryRunAPI().
			GET(fmt.Sprintf("/open-apis/vc/v1/notes/%s", validate.EncodePathSegment(noteID))).
			Desc("[1] Check note_display_type and verbatim_doc_token before transcript fetch").
			GET(fmt.Sprintf("/open-apis/vc/v1/notes/%s/unified_note_transcript", validate.EncodePathSegment(noteID))).
			Desc("[2] Fetch unified note transcript pages; subsequent pages add cursor_id internally").
			Params(map[string]interface{}{
				"format":    transcriptFormat,
				"page_size": transcriptPageSize,
				"locale":    locale,
			}).
			Set("transcript_format", transcriptFormat).
			Set("locale", locale).
			Set("note", "CLI first checks note_display_type via note detail, then paginates internally (cursor_id) and saves the full unified transcript to a file")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		noteID := strings.TrimSpace(runtime.Str("note-id"))
		transcriptFormat := runtime.Str("transcript-format")
		locale := resolveTranscriptLocale(runtime)

		outPath := strings.TrimSpace(runtime.Str("output"))
		if outPath == "" {
			outPath = defaultTranscriptPath(noteID, transcriptFormat)
		}
		if !runtime.Bool("overwrite") {
			if _, statErr := runtime.FileIO().Stat(outPath); statErr == nil {
				precondition := errs.NewValidationError(errs.SubtypeFailedPrecondition, "output file already exists: %s", outPath).
					WithHint("Pass --overwrite to replace the existing file")
				if strings.TrimSpace(runtime.Str("output")) != "" {
					precondition = precondition.WithParam("--output")
				}
				return precondition
			}
		}

		if err := ensureUnifiedNote(ctx, runtime, noteID); err != nil {
			return err
		}

		content, err := fetchUnifiedTranscript(ctx, runtime, noteID, transcriptFormat, locale)
		if err != nil {
			return err
		}

		saved, err := runtime.FileIO().Save(outPath, fileio.SaveOptions{}, bytes.NewReader(content))
		if err != nil {
			return common.WrapSaveErrorTyped(err)
		}
		resolved, rerr := runtime.FileIO().ResolvePath(outPath)
		if rerr != nil || resolved == "" {
			resolved = outPath
		}

		runtime.OutFormat(map[string]any{
			"note_id":           noteID,
			"transcript_format": transcriptFormat,
			"transcript_file":   resolved,
			"size_bytes":        saved.Size(),
		}, nil, nil)
		return nil
	},
}

func ensureUnifiedNote(ctx context.Context, runtime *common.RuntimeContext, noteID string) error {
	detail, err := FetchDetail(ctx, runtime, noteID)
	if err != nil {
		return mapNoteError(err)
	}
	if detail.DisplayType != "unified" {
		if detail.VerbatimDocToken != "" {
			return errs.NewValidationError(errs.SubtypeFailedPrecondition, "note %s is not a unified note (note_display_type=%s, verbatim_doc_token=%s)", noteID, detail.DisplayType, detail.VerbatimDocToken).
				WithHint("Use docs +fetch --api-version v2 --doc %s for normal note transcripts", detail.VerbatimDocToken)
		}
		return errs.NewValidationError(errs.SubtypeFailedPrecondition, "note %s is not a unified note (note_display_type=%s, verbatim_doc_token=)", noteID, detail.DisplayType).
			WithHint("Use note +detail to inspect document tokens")
	}
	return nil
}

// fetchUnifiedTranscript walks every page of the unified transcript and returns
// the concatenated content. Any page error fails the whole call: a partial
// transcript is misleading, so we prefer an explicit error over silent loss.
func fetchUnifiedTranscript(ctx context.Context, runtime *common.RuntimeContext, noteID, transcriptFormat, locale string) ([]byte, error) {
	errOut := runtime.IO().ErrOut
	apiPath := fmt.Sprintf("/open-apis/vc/v1/notes/%s/unified_note_transcript", validate.EncodePathSegment(noteID))

	var buf bytes.Buffer
	var cursor string
	seenCursors := map[string]bool{}
	for page := 1; ; page++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if page > maxTranscriptPages {
			return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "transcript exceeded %d pages; aborting to avoid an unbounded loop", maxTranscriptPages)
		}

		query := larkcore.QueryParams{
			"format":    []string{transcriptFormat},
			"locale":    []string{locale},
			"page_size": []string{strconv.Itoa(transcriptPageSize)},
		}
		if cursor != "" {
			query["cursor_id"] = []string{cursor}
		}
		data, err := runtime.DoAPIJSONTyped(http.MethodGet, apiPath, query, nil)
		if err != nil {
			return nil, mapNoteError(err)
		}

		if transcript, _ := data["transcript"].(map[string]any); transcript != nil {
			if chunk, _ := transcript[transcriptFormat].(string); chunk != "" {
				buf.WriteString(chunk)
			}
		}

		hasMore, _ := data["has_more"].(bool)
		if !hasMore {
			break
		}
		next, ok := parseLooseCursorID(data["next_cursor_id"])
		if !ok || next == cursor || seenCursors[next] {
			fmt.Fprintf(errOut, "%s has_more set but cursor did not advance at page %d\n", logPrefix, page)
			return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "transcript pagination cursor did not advance at page %d; aborting to avoid saving a partial transcript", page)
		}
		seenCursors[cursor] = true
		cursor = next
		timer := time.NewTimer(pageDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}

	if buf.Len() == 0 {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "transcript is empty for note %s in %s format; aborting to avoid saving an empty transcript", noteID, transcriptFormat)
	}
	return buf.Bytes(), nil
}

// defaultTranscriptPath builds the default save path for a note transcript.
func defaultTranscriptPath(noteID, transcriptFormat string) string {
	name := "unified_transcript.md"
	if transcriptFormat == transcriptFormatPlainText {
		name = "unified_transcript.txt"
	}
	return filepath.Join(noteArtifactSubdir, noteID, name)
}

func resolveTranscriptLocale(runtime *common.RuntimeContext) string {
	if explicit := strings.TrimSpace(runtime.Str("locale")); explicit != "" {
		return explicit
	}
	if lang := runtime.Lang(); lang != "" {
		return string(lang)
	}
	if runtime.Config.Brand == core.BrandLark {
		return string(i18n.LangEnUS)
	}
	return string(i18n.LangZhCN)
}
