// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
//
// minutes +detail — query minute details with selective artifact flags

package minutes

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const minutesDetailLogPrefix = "[minutes +detail]"

// Error codes from the minutes API.
const (
	minutesDetailProcessingCode       = 2091003
	minutesDetailNoReadPermissionCode = 2091005
	minutesDetailWaitTimeoutDefault   = 300
	minutesDetailWaitIntervalDefault  = 15
)

var validMinuteTokenDetail = regexp.MustCompile(`^[a-z0-9]+$`)

var scopesDetailMinuteTokens = []string{
	"minutes:minutes.basic:read",
	"minutes:minutes.artifacts:read",
}

// minuteDetailItem represents a single minute detail result.
type minuteDetailItem struct {
	MinuteToken string         `json:"minute_token"`
	Status      string         `json:"status,omitempty"`
	Title       string         `json:"title"`
	NoteID      string         `json:"note_id"`
	Artifacts   map[string]any `json:"artifacts,omitempty"`
	Retryable   bool           `json:"retryable,omitempty"`
	Error       string         `json:"error,omitempty"`
	Hint        string         `json:"hint,omitempty"`
	NextCommand string         `json:"next_command,omitempty"`
}

// fetchMinuteDetail queries a single minute's metadata and selected artifacts.
func fetchMinuteDetail(ctx context.Context, runtime *common.RuntimeContext, minuteToken string) *minuteDetailItem {
	artifactFlags := requestedMinutesDetailArtifactFlags(runtime)
	waitReady := runtime.Bool("wait-ready")
	waitTimeout, waitInterval := minutesDetailWaitConfig(runtime)

	data, err := callMinutesDetailAPIUntilReady(ctx, runtime, waitReady, waitTimeout, waitInterval, func() (map[string]interface{}, error) {
		return runtime.CallAPITyped(http.MethodGet,
			fmt.Sprintf("/open-apis/minutes/v1/minutes/%s", validate.EncodePathSegment(minuteToken)), nil, nil)
	})
	if err != nil {
		result := &minuteDetailItem{MinuteToken: minuteToken}
		if isMinutesDetailProcessingError(err) {
			markMinutesDetailProcessing(result, minuteToken, artifactFlags, "minute metadata is still being generated")
		} else if p, ok := errs.ProblemOf(err); ok && p.Code == minutesDetailNoReadPermissionCode {
			result.Error = fmt.Sprintf("No read permission for minute %s. Ask the minute owner for minute file read permission", minuteToken)
		} else {
			result.Error = fmt.Sprintf("failed to query minute: %v", err)
		}
		return result
	}

	minute, _ := data["minute"].(map[string]any)
	if minute == nil {
		return &minuteDetailItem{MinuteToken: minuteToken, Error: "minute not found"}
	}

	result := &minuteDetailItem{MinuteToken: minuteToken}
	if v, ok := minute["title"].(string); ok && v != "" {
		result.Title = v
	}
	if v, ok := minute["note_id"].(string); ok && v != "" {
		result.NoteID = v
	}

	// Fetch artifacts selectively based on flags
	needSummary := runtime.Bool("summary")
	needTodo := runtime.Bool("todo")
	needChapter := runtime.Bool("chapter")
	needTranscript := runtime.Bool("transcript")
	needKeyword := runtime.Bool("keyword")

	if needSummary || needTodo || needChapter || needTranscript || needKeyword {
		artData, err := callMinutesDetailAPIUntilReady(ctx, runtime, waitReady, waitTimeout, waitInterval, func() (map[string]interface{}, error) {
			return runtime.CallAPITyped(http.MethodGet,
				fmt.Sprintf("/open-apis/minutes/v1/minutes/%s/artifacts", validate.EncodePathSegment(minuteToken)), nil, nil)
		})
		if err != nil {
			if isMinutesDetailProcessingError(err) {
				markMinutesDetailProcessing(result, minuteToken, artifactFlags, "minute artifacts are still being generated")
			} else {
				result.Error = fmt.Sprintf("failed to query minute artifacts: %v", err)
			}
		} else {
			artifacts := make(map[string]any)
			if needSummary {
				if v, ok := artData["summary"].(string); ok && v != "" {
					artifacts["summary"] = v
				} else {
					artifacts["summary"] = ""
				}
			}
			if needTodo {
				if v, ok := artData["minute_todos"].([]any); ok && len(v) > 0 {
					artifacts["todos"] = v
				} else {
					artifacts["todos"] = []any{}
				}
			}
			if needChapter {
				if v, ok := artData["minute_chapters"].([]any); ok && len(v) > 0 {
					artifacts["chapters"] = v
				} else {
					artifacts["chapters"] = []any{}
				}
			}
			if needKeyword {
				if v, ok := artData["keywords"].([]any); ok && len(v) > 0 {
					artifacts["keywords"] = v
				} else {
					artifacts["keywords"] = []any{}
				}
			}
			if needTranscript {
				if v, ok := artData["transcript"].(string); ok && v != "" {
					if path := saveDetailTranscript(runtime, minuteToken, result.Title, []byte(v)); path != "" {
						artifacts["transcript_file"] = path
					} else {
						artifacts["transcript_file"] = ""
					}
				} else {
					artifacts["transcript_file"] = ""
				}
			}
			result.Artifacts = artifacts
		}
	}

	return result
}

func isMinutesDetailProcessingError(err error) bool {
	if p, ok := errs.ProblemOf(err); ok && p.Code == minutesDetailProcessingCode {
		return true
	}
	return false
}

func minutesDetailWaitConfig(runtime *common.RuntimeContext) (time.Duration, time.Duration) {
	timeoutSeconds, intervalSeconds := normalizeMinutesDetailWaitSeconds(runtime.Int("wait-timeout-seconds"), runtime.Int("wait-interval-seconds"))
	return time.Duration(timeoutSeconds) * time.Second, time.Duration(intervalSeconds) * time.Second
}

func normalizeMinutesDetailWaitSeconds(timeoutSeconds, intervalSeconds int) (int, int) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = minutesDetailWaitTimeoutDefault
	}
	if intervalSeconds <= 0 {
		intervalSeconds = minutesDetailWaitIntervalDefault
	}
	return timeoutSeconds, intervalSeconds
}

func callMinutesDetailAPIUntilReady(ctx context.Context, runtime *common.RuntimeContext, waitReady bool, timeout, interval time.Duration, call func() (map[string]interface{}, error)) (map[string]interface{}, error) {
	deadline := time.Now().Add(timeout)
	for {
		data, err := call()
		if err == nil || !waitReady || !isMinutesDetailProcessingError(err) {
			return data, err
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		remaining := time.Until(deadline)
		if remaining <= 0 || interval > remaining {
			return nil, err
		}
		fmt.Fprintf(runtime.IO().ErrOut, "%s minute is still processing; retrying in %s\n", minutesDetailLogPrefix, interval)
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func requestedMinutesDetailArtifactFlags(runtime *common.RuntimeContext) []string {
	var flags []string
	for _, flag := range []string{"summary", "todo", "chapter", "keyword", "transcript"} {
		if runtime.Bool(flag) {
			flags = append(flags, "--"+flag)
		}
	}
	return flags
}

func markMinutesDetailProcessing(result *minuteDetailItem, minuteToken string, artifactFlags []string, reason string) {
	result.Status = "processing"
	result.Retryable = true
	result.Error = reason
	result.Hint = "The minute is still being generated. Retry later, or rerun the next_command to wait until it is ready."
	result.NextCommand = minutesDetailNextCommand(minuteToken, artifactFlags)
}

func minutesDetailNextCommand(minuteToken string, artifactFlags []string) string {
	parts := []string{"lark-cli", "minutes", "+detail", "--minute-tokens", minuteToken}
	parts = append(parts, artifactFlags...)
	parts = append(parts, "--wait-ready")
	return strings.Join(parts, " ")
}

// saveDetailTranscript persists transcript bytes to the canonical artifact path.
// With --output-dir, transcripts land under <output-dir>/artifact-<title>-<token>/
// to mirror the legacy `vc +notes` layout. Otherwise falls back to the default
// ./minutes/<token>/ shared with `minutes +download`.
func saveDetailTranscript(runtime *common.RuntimeContext, minuteToken, title string, content []byte) string {
	errOut := runtime.IO().ErrOut
	var dirName string
	if outDir := runtime.Str("output-dir"); outDir != "" {
		dirName = filepath.Join(outDir, sanitizeDetailDirName(title, minuteToken))
	} else {
		dirName = common.DefaultMinuteArtifactDir(minuteToken)
	}
	transcriptPath := filepath.Join(dirName, common.DefaultTranscriptFileName)

	if !runtime.Bool("overwrite") {
		if _, statErr := runtime.FileIO().Stat(transcriptPath); statErr == nil {
			fmt.Fprintf(errOut, "%s transcript already exists: %s (use --overwrite to replace)\n", minutesDetailLogPrefix, transcriptPath)
			return transcriptPath
		}
	}

	fmt.Fprintf(errOut, "%s writing transcript: %s\n", minutesDetailLogPrefix, transcriptPath)
	if _, err := runtime.FileIO().Save(transcriptPath, fileio.SaveOptions{}, bytes.NewReader(content)); err != nil {
		fmt.Fprintf(errOut, "%s failed to write transcript: %v\n", minutesDetailLogPrefix, err)
		return ""
	}
	return transcriptPath
}

// sanitizeDetailDirName generates a filesystem-safe directory name using title
// and minuteToken for uniqueness. Mirrors the layout produced by `vc +notes`
// so both shortcuts write artifacts to identical paths under --output-dir.
func sanitizeDetailDirName(title, minuteToken string) string {
	const maxLen = 200
	replacer := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_", "?", "_",
		"\"", "_", "<", "_", ">", "_", "|", "_",
		"\n", "_", "\r", "_", "\t", "_", "\x00", "_",
	)
	safe := replacer.Replace(strings.TrimSpace(title))
	safe = strings.Trim(safe, ".")
	if len(safe) > maxLen {
		safe = safe[:maxLen]
	}
	if safe == "" {
		return fmt.Sprintf("artifact-%s", minuteToken)
	}
	return fmt.Sprintf("artifact-%s-%s", safe, minuteToken)
}

// MinutesDetail queries minute details with selective artifact flags.
var MinutesDetail = common.Shortcut{
	Service:     "minutes",
	Command:     "+detail",
	Description: "Query minute details with selective artifact flags (summary, todo, chapter, transcript, keyword)",
	Risk:        "read",
	Scopes:      []string{"minutes:minutes.basic:read", "minutes:minutes.artifacts:read"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "minute-tokens", Desc: "minute tokens, comma-separated for batch", Required: true},
		{Name: "summary", Type: "bool", Desc: "include summary"},
		{Name: "todo", Type: "bool", Desc: "include todos"},
		{Name: "chapter", Type: "bool", Desc: "include chapters"},
		{Name: "transcript", Type: "bool", Desc: "include transcript (saved to file)"},
		{Name: "keyword", Type: "bool", Desc: "include keywords"},
		{Name: "output-dir", Desc: "output directory for transcript files (default: ./minutes/{minute_token}/)"},
		{Name: "overwrite", Type: "bool", Desc: "overwrite existing transcript files"},
		{Name: "wait-ready", Type: "bool", Desc: "wait until minute metadata/artifacts are ready", Hidden: true},
		{Name: "wait-timeout-seconds", Type: "int", Default: "300", Desc: "maximum seconds to wait for readiness", Hidden: true},
		{Name: "wait-interval-seconds", Type: "int", Default: "15", Desc: "seconds between readiness checks", Hidden: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		tokens := common.SplitCSV(runtime.Str("minute-tokens"))
		const maxBatchSize = 50
		if len(tokens) > maxBatchSize {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--minute-tokens: too many tokens (%d), maximum is %d", len(tokens), maxBatchSize).WithParam("--minute-tokens")
		}
		for _, token := range tokens {
			if !validMinuteTokenDetail.MatchString(token) {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid minute token %q: must contain only lowercase alphanumeric characters", token).WithParam("--minute-tokens")
			}
		}
		if outDir := runtime.Str("output-dir"); outDir != "" {
			if err := common.ValidateSafePathTyped(runtime.FileIO(), outDir); err != nil {
				return err
			}
		}
		// dynamic scope check
		result, err := runtime.Factory.Credential.ResolveToken(ctx, credential.NewTokenSpec(runtime.As(), runtime.Config.AppID))
		if err == nil && result != nil && result.Scopes != "" {
			if missing := auth.MissingScopes(result.Scopes, scopesDetailMinuteTokens); len(missing) > 0 {
				return errs.NewPermissionError(errs.SubtypeMissingScope,
					"missing required scope(s): %s", strings.Join(missing, ", ")).
					WithHint("run `lark-cli auth login --scope %q` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login.", strings.Join(missing, " ")).
					WithMissingScopes(missing...).
					WithIdentity(string(runtime.As()))
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		tokens := runtime.Str("minute-tokens")
		d := common.NewDryRunAPI().
			GET("/open-apis/minutes/v1/minutes/{minute_token}").
			Set("minute_tokens", common.SplitCSV(tokens))

		if runtime.Bool("summary") || runtime.Bool("todo") || runtime.Bool("chapter") || runtime.Bool("transcript") || runtime.Bool("keyword") {
			d.GET("/open-apis/minutes/v1/minutes/{minute_token}/artifacts")
		}
		return d
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		errOut := runtime.IO().ErrOut
		minuteTokens := common.SplitCSV(runtime.Str("minute-tokens"))
		results := make([]*minuteDetailItem, 0, len(minuteTokens))

		const batchDelay = 100 * time.Millisecond
		fmt.Fprintf(errOut, "%s querying %d minute_token(s)\n", minutesDetailLogPrefix, len(minuteTokens))
		for i, token := range minuteTokens {
			if err := ctx.Err(); err != nil {
				return err
			}
			if i > 0 {
				time.Sleep(batchDelay)
			}
			fmt.Fprintf(errOut, "%s querying minute_token=%s ...\n", minutesDetailLogPrefix, token)
			results = append(results, fetchMinuteDetail(ctx, runtime, token))
		}

		successCount := 0
		for _, r := range results {
			if r.Error == "" {
				successCount++
			}
		}
		fmt.Fprintf(errOut, "%s done: %d total, %d succeeded, %d failed\n", minutesDetailLogPrefix, len(results), successCount, len(results)-successCount)

		if successCount == 0 && len(results) > 0 {
			return runtime.OutPartialFailure(map[string]any{"minutes": results}, &output.Meta{Count: len(results)})
		}

		outData := map[string]any{"minutes": results}
		runtime.OutFormat(outData, &output.Meta{Count: len(results)}, func(w io.Writer) {
			if len(results) == 0 {
				fmt.Fprintln(w, "No minutes.")
				return
			}
			var rows []map[string]interface{}
			for _, r := range results {
				row := map[string]interface{}{"minute_token": r.MinuteToken}
				if r.Error != "" {
					if r.Status == "processing" {
						row["status"] = "PROCESSING"
					} else {
						row["status"] = "FAIL"
					}
					row["error"] = r.Error
					if r.NextCommand != "" {
						row["next_command"] = r.NextCommand
					}
				} else {
					row["status"] = "OK"
					row["title"] = r.Title
					row["note_id"] = r.NoteID
					if len(r.Artifacts) > 0 {
						var parts []string
						if _, ok := r.Artifacts["summary"]; ok {
							parts = append(parts, "summary")
						}
						if _, ok := r.Artifacts["todos"]; ok {
							parts = append(parts, "todo")
						}
						if _, ok := r.Artifacts["chapters"]; ok {
							parts = append(parts, "chapter")
						}
						if _, ok := r.Artifacts["keywords"]; ok {
							parts = append(parts, "keyword")
						}
						if _, ok := r.Artifacts["transcript_file"]; ok {
							parts = append(parts, "transcript")
						}
						if len(parts) > 0 {
							row["artifacts"] = strings.Join(parts, ", ")
						}
					}
				}
				rows = append(rows, row)
			}
			output.PrintTable(w, rows)
			fmt.Fprintf(w, "\n%d minute(s), %d succeeded, %d failed\n", len(results), successCount, len(results)-successCount)
		})
		return nil
	},
}
