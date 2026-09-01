// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

const (
	wordRenderCreatePath       = "/open-apis/docs_ai/v1/word_render_tasks"
	wordRenderStatusPath       = "/open-apis/docs_ai/v1/word_render_tasks/:task_id"
	wordRenderMaxDOCXSizeBytes = int64(20 << 20)
	wordRenderMaxWaitSeconds   = 600
	wordRenderDefaultPoll      = 3 * time.Second
	wordRenderMinPoll          = 500 * time.Millisecond
	wordRenderMaxPoll          = 10 * time.Second

	wordRenderIfExistsError     = "error"
	wordRenderIfExistsOverwrite = "overwrite"
	wordRenderIfExistsRename    = "rename"
)

type wordRenderPDF struct {
	DownloadURL  string `json:"download_url,omitempty"`
	Size         int64  `json:"size,omitempty"`
	URLExpiresAt int64  `json:"url_expires_at,omitempty"`
}

type wordRenderHeading struct {
	HeadingIndex int32  `json:"heading_index"`
	Title        string `json:"title"`
	Level        int32  `json:"level"`
	PageIndex    *int32 `json:"page_index,omitempty"`
	PageNumber   *int32 `json:"page_number,omitempty"`
}

type wordRenderWarning struct {
	Code         string `json:"code"`
	Message      string `json:"message"`
	HeadingIndex *int32 `json:"heading_index,omitempty"`
}

type wordRenderFailure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type wordRenderTask struct {
	TaskID      string              `json:"task_id"`
	Status      string              `json:"status"`
	Stage       string              `json:"stage,omitempty"`
	PollAfterMs int64               `json:"poll_after_ms,omitempty"`
	PDF         *wordRenderPDF      `json:"pdf,omitempty"`
	PageCount   int32               `json:"page_count,omitempty"`
	Headings    []wordRenderHeading `json:"headings,omitempty"`
	Warnings    []wordRenderWarning `json:"warnings,omitempty"`
	Failure     *wordRenderFailure  `json:"failure,omitempty"`
	ExpiresAt   int64               `json:"expires_at,omitempty"`
}

var DocsRenderWord = common.Shortcut{
	Service:     "docs",
	Command:     "+render-word",
	Description: "Render a local DOCX file to PDF and download the result",
	Risk:        "write",
	Scopes:      []string{"docx:document:readonly"},
	AuthTypes:   []string{"user"},
	Flags: []common.Flag{
		{Name: "file", Desc: "local DOCX file", Required: true},
		{Name: "output", Desc: "PDF output path", Required: true},
		{Name: "if-exists", Desc: "output conflict policy: error | overwrite | rename", Default: wordRenderIfExistsError, Enum: []string{wordRenderIfExistsError, wordRenderIfExistsOverwrite, wordRenderIfExistsRename}},
		{Name: "wait-timeout-seconds", Type: "int", Desc: "maximum seconds to wait for rendering, range 0-600", Default: "600"},
		{Name: "idempotency-key", Desc: "optional idempotency key; generated when omitted"},
	},
	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		if err := validateWordRenderInputFile(runtime, runtime.Str("file")); err != nil {
			return err
		}
		if err := validateWordRenderOutput(runtime, runtime.Str("output"), runtime.Str("if-exists"), true); err != nil {
			return err
		}
		waitSeconds := runtime.Int("wait-timeout-seconds")
		if waitSeconds < 0 || waitSeconds > wordRenderMaxWaitSeconds {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"invalid --wait-timeout-seconds %d: must be between 0 and 600", waitSeconds).
				WithParam("--wait-timeout-seconds")
		}
		return validateWordRenderIdempotencyKey(runtime.Str("idempotency-key"))
	},
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		idempotencyKey := strings.TrimSpace(runtime.Str("idempotency-key"))
		if idempotencyKey == "" {
			idempotencyKey = "<generated>"
		}
		return common.NewDryRunAPI().
			POST(wordRenderCreatePath).
			Desc("Upload one DOCX file and create an asynchronous render task").
			Body(map[string]interface{}{
				"file":            "<contents of --file>",
				"idempotency_key": idempotencyKey,
			}).
			GET(wordRenderStatusPath).
			Desc("Poll the render task until it reaches a terminal state").
			GET("https://presigned.invalid/render.pdf").
			Desc("Download the PDF from the short-lived presigned URL without Authorization").
			File(cmdutil.DryRunFileIntent{
				Name:     runtime.Str("output"),
				IfExists: runtime.Str("if-exists"),
				Content:  "rendered PDF",
			})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		idempotencyKey := strings.TrimSpace(runtime.Str("idempotency-key"))
		if idempotencyKey == "" {
			var err error
			idempotencyKey, err = newWordRenderIdempotencyKey()
			if err != nil {
				return errs.NewInternalError(errs.SubtypeSDKError, "failed to generate idempotency key").WithCause(err)
			}
		}
		task, err := createWordRenderTask(ctx, runtime, runtime.Str("file"), idempotencyKey)
		if err != nil {
			return err
		}

		waitSeconds := runtime.Int("wait-timeout-seconds")
		if strings.EqualFold(task.Status, "processing") {
			if waitSeconds == 0 {
				runtime.Out(wordRenderResult(task, "", 0, true, runtime.Str("output")), nil)
				return nil
			}
			task, err = waitForWordRenderTask(ctx, runtime, task, time.Duration(waitSeconds)*time.Second)
			if err != nil {
				return err
			}
			if strings.EqualFold(task.Status, "processing") {
				runtime.Out(wordRenderResult(task, "", 0, true, runtime.Str("output")), nil)
				return nil
			}
		}

		if err := wordRenderTaskTerminalError(task); err != nil {
			return err
		}
		outputPath, downloadedSize, err := downloadWordRenderPDF(
			ctx,
			runtime,
			task,
			runtime.Str("output"),
			runtime.Str("if-exists"),
		)
		if err != nil {
			return err
		}
		runtime.Out(wordRenderResult(task, outputPath, downloadedSize, false, ""), nil)
		return nil
	},
}

var DocsRenderWordStatus = common.Shortcut{
	Service:     "docs",
	Command:     "+render-word-status",
	Description: "Get a Word render task and optionally download its PDF",
	Risk:        "read",
	Scopes:      []string{"docx:document:readonly"},
	AuthTypes:   []string{"user"},
	Flags: []common.Flag{
		{Name: "task-id", Desc: "task_id returned by docs +render-word", Required: true},
		{Name: "output", Desc: "optional PDF output path"},
		{Name: "if-exists", Desc: "output conflict policy: error | overwrite | rename", Default: wordRenderIfExistsError, Enum: []string{wordRenderIfExistsError, wordRenderIfExistsOverwrite, wordRenderIfExistsRename}},
	},
	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		if err := validateWordRenderTaskID(runtime.Str("task-id")); err != nil {
			return err
		}
		return validateWordRenderOutput(runtime, runtime.Str("output"), runtime.Str("if-exists"), false)
	},
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		taskID := strings.TrimSpace(runtime.Str("task-id"))
		plan := common.NewDryRunAPI().
			GET(wordRenderStatusPath).
			Set("task_id", taskID).
			Desc("Get the render task once")
		if strings.TrimSpace(runtime.Str("output")) != "" {
			plan.GET("https://presigned.invalid/render.pdf").
				Desc("Download the PDF from the short-lived presigned URL without Authorization").
				File(cmdutil.DryRunFileIntent{
					Name:     runtime.Str("output"),
					IfExists: runtime.Str("if-exists"),
					Content:  "rendered PDF",
				})
		}
		return plan
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		task, err := getWordRenderTask(ctx, runtime, runtime.Str("task-id"))
		if err != nil {
			return err
		}
		if strings.EqualFold(task.Status, "processing") {
			runtime.Out(wordRenderResult(task, "", 0, false, ""), nil)
			return nil
		}
		if err := wordRenderTaskTerminalError(task); err != nil {
			return err
		}
		output := strings.TrimSpace(runtime.Str("output"))
		if output == "" {
			runtime.Out(wordRenderResult(task, "", 0, false, ""), nil)
			return nil
		}
		outputPath, downloadedSize, err := downloadWordRenderPDF(ctx, runtime, task, output, runtime.Str("if-exists"))
		if err != nil {
			return err
		}
		runtime.Out(wordRenderResult(task, outputPath, downloadedSize, false, ""), nil)
		return nil
	},
}

func validateWordRenderInputFile(runtime *common.RuntimeContext, rawPath string) error {
	filePath := strings.TrimSpace(rawPath)
	if filePath == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--file is required").WithParam("--file")
	}
	if !strings.EqualFold(filepath.Ext(filePath), ".docx") {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--file must point to a .docx file").WithParam("--file")
	}
	if runtime.FileIO() == nil {
		return errs.NewInternalError(errs.SubtypeFileIO, "file I/O is unavailable")
	}
	info, err := runtime.FileIO().Stat(filePath)
	if err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "cannot access --file: %s", err).
			WithParam("--file").
			WithCause(err)
	}
	if !info.Mode().IsRegular() {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--file must be a regular file").WithParam("--file")
	}
	if info.Size() <= 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--file is empty").WithParam("--file")
	}
	if info.Size() > wordRenderMaxDOCXSizeBytes {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--file exceeds the 20 MiB limit (size is %d bytes)", info.Size()).WithParam("--file")
	}
	return nil
}

func validateWordRenderOutput(runtime *common.RuntimeContext, rawOutput, ifExists string, required bool) error {
	output := strings.TrimSpace(rawOutput)
	if output == "" {
		if required {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--output is required").WithParam("--output")
		}
		if strings.TrimSpace(ifExists) != "" && strings.TrimSpace(ifExists) != wordRenderIfExistsError {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--if-exists requires --output").WithParam("--if-exists")
		}
		return nil
	}
	if _, err := runtime.ResolveSavePath(output); err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe --output path: %s", err).
			WithParam("--output").
			WithCause(err)
	}
	return validateWordRenderIfExists(ifExists)
}

func validateWordRenderIfExists(value string) error {
	switch strings.TrimSpace(value) {
	case "", wordRenderIfExistsError, wordRenderIfExistsOverwrite, wordRenderIfExistsRename:
		return nil
	default:
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"invalid --if-exists %q: allowed values are error, overwrite, rename", value).WithParam("--if-exists")
	}
}

func validateWordRenderIdempotencyKey(value string) error {
	if value == "" {
		return nil
	}
	if strings.TrimSpace(value) != value || len(value) > 128 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--idempotency-key must be 1-128 characters without surrounding whitespace").WithParam("--idempotency-key")
	}
	return nil
}

func validateWordRenderTaskID(value string) error {
	taskID := strings.TrimSpace(value)
	if taskID == "" || len(taskID) > 128 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--task-id is required and must be at most 128 characters").WithParam("--task-id")
	}
	if err := validate.ResourceName(taskID, "--task-id"); err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --task-id: %s", err).WithParam("--task-id").WithCause(err)
	}
	return nil
}

func createWordRenderTask(
	ctx context.Context,
	runtime *common.RuntimeContext,
	filePath string,
	idempotencyKey string,
) (wordRenderTask, error) {
	file, err := runtime.FileIO().Open(strings.TrimSpace(filePath))
	if err != nil {
		return wordRenderTask{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "cannot open --file: %s", err).
			WithParam("--file").
			WithCause(err)
	}
	defer file.Close()

	form := larkcore.NewFormdata()
	form.AddFile("file", file)
	form.AddField("idempotency_key", idempotencyKey)
	response, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodPost,
		ApiPath:    wordRenderCreatePath,
		Body:       form,
	}, larkcore.WithFileUpload())
	if err != nil {
		return wordRenderTask{}, client.WrapDoAPIError(err)
	}
	data, err := runtime.ClassifyAPIResponse(response)
	if err != nil {
		return wordRenderTask{}, err
	}
	return parseWordRenderTask(data)
}

func getWordRenderTask(ctx context.Context, runtime *common.RuntimeContext, rawTaskID string) (wordRenderTask, error) {
	taskID := strings.TrimSpace(rawTaskID)
	data, err := runtime.CallAPITyped(
		http.MethodGet,
		strings.Replace(wordRenderStatusPath, ":task_id", validate.EncodePathSegment(taskID), 1),
		nil,
		nil,
	)
	if err != nil {
		return wordRenderTask{}, err
	}
	return parseWordRenderTask(data)
}

func parseWordRenderTask(data map[string]interface{}) (wordRenderTask, error) {
	rawTask := common.GetMap(data, "task")
	if rawTask == nil {
		rawTask = common.GetMap(data, "Task")
	}
	if rawTask == nil {
		return wordRenderTask{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "Word render API returned no task")
	}
	raw, err := json.Marshal(rawTask)
	if err != nil {
		return wordRenderTask{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "failed to decode Word render task").WithCause(err)
	}
	var task wordRenderTask
	if err := json.Unmarshal(raw, &task); err != nil {
		return wordRenderTask{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "failed to decode Word render task").WithCause(err)
	}
	if strings.TrimSpace(task.TaskID) == "" || strings.TrimSpace(task.Status) == "" {
		return wordRenderTask{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "Word render API returned an incomplete task")
	}
	return task, nil
}

func waitForWordRenderTask(
	ctx context.Context,
	runtime *common.RuntimeContext,
	task wordRenderTask,
	timeout time.Duration,
) (wordRenderTask, error) {
	deadline := time.Now().Add(timeout)
	for strings.EqualFold(task.Status, "processing") {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return task, nil
		}
		wait := clampWordRenderPoll(task.PollAfterMs)
		if wait > remaining {
			wait = remaining
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return wordRenderTask{}, ctx.Err()
		case <-timer.C:
		}
		if time.Until(deadline) <= 0 {
			return task, nil
		}
		var err error
		task, err = getWordRenderTask(ctx, runtime, task.TaskID)
		if err != nil {
			return wordRenderTask{}, err
		}
	}
	return task, nil
}

func clampWordRenderPoll(pollAfterMillis int64) time.Duration {
	if pollAfterMillis <= 0 {
		return wordRenderDefaultPoll
	}
	duration := time.Duration(pollAfterMillis) * time.Millisecond
	if duration < wordRenderMinPoll {
		return wordRenderMinPoll
	}
	if duration > wordRenderMaxPoll {
		return wordRenderMaxPoll
	}
	return duration
}

func wordRenderTaskTerminalError(task wordRenderTask) error {
	switch strings.ToLower(strings.TrimSpace(task.Status)) {
	case "succeeded":
		if task.PDF == nil || strings.TrimSpace(task.PDF.DownloadURL) == "" {
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "succeeded Word render task has no PDF download URL")
		}
		return nil
	case "failed":
		code := "drive_preview_failed"
		message := "Word render task failed"
		if task.Failure != nil {
			if strings.TrimSpace(task.Failure.Code) != "" {
				code = strings.TrimSpace(task.Failure.Code)
			}
			if strings.TrimSpace(task.Failure.Message) != "" {
				message = strings.TrimSpace(task.Failure.Message)
			}
		}
		return errs.NewAPIError(errs.SubtypeServerError, "%s (failure_code=%s)", message, code).
			WithHint("retry docs +render-word with a new --idempotency-key; if it persists, contact support")
	case "expired":
		return errs.NewAPIError(errs.SubtypeNotFound, "Word render task expired").
			WithHint("run docs +render-word again to create a new task")
	case "processing":
		return nil
	default:
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "Word render API returned unknown task status %q", task.Status)
	}
}

func wordRenderResult(task wordRenderTask, outputPath string, downloadedSize int64, timedOut bool, nextOutput string) map[string]interface{} {
	raw, _ := json.Marshal(task)
	result := make(map[string]interface{})
	_ = json.Unmarshal(raw, &result)
	if outputPath != "" {
		result["output_path"] = outputPath
		result["downloaded_size"] = downloadedSize
	}
	if timedOut {
		result["timed_out"] = true
		result["next_command"] = fmt.Sprintf(
			"lark-cli docs +render-word-status --task-id %s --output %s",
			shellQuoteWordRender(task.TaskID),
			shellQuoteWordRender(nextOutput),
		)
	}
	return result
}

func shellQuoteWordRender(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func newWordRenderIdempotencyKey() (string, error) {
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	return "lark-cli-" + base64.RawURLEncoding.EncodeToString(randomBytes), nil
}
