// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whiteboard

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	defaultParseImageResultTimeout  = 20 * time.Minute
	defaultParseImageResultInterval = 10 * time.Second
)

var wbParseImageExts = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".webp": true,
}

const (
	parseImageModeMini       = "mini"
	parseImageModeFlash      = "flash"
	parseImageModeAgentic    = "agentic"
	parseImageModeAgenticMax = "agentic_max"
)

func wbParseImageURL(token string) string {
	return fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/parse_image", url.PathEscape(token))
}

func wbParseImageDryRunURL(token string) string {
	return fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/parse_image", common.MaskToken(url.PathEscape(token)))
}

func wbParseImageResultURL(token, taskID string) string {
	return fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/parse_image/%s", url.PathEscape(token), url.PathEscape(taskID))
}

func wbParseImageResultDryRunURL(token, taskID string) string {
	return fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/parse_image/%s", common.MaskToken(url.PathEscape(token)), url.PathEscape(taskID))
}

func validateParseImageFilePath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--image is required").WithParam("--image")
	}
	ext := strings.ToLower(filepath.Ext(path))
	if !wbParseImageExts[ext] {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--image must be an image (supported: PNG, JPG, JPEG, GIF, WEBP), got %q", ext).
			WithParam("--image")
	}
	return nil
}

func normalizeParseImageIdempotentToken(raw string) (string, error) {
	token := strings.TrimSpace(raw)
	if err := common.RejectDangerousCharsTyped("--idempotent-token", token); err != nil {
		return "", err
	}
	if token != "" && len(token) < 10 {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--idempotent-token must be at least 10 characters long.").
			WithParam("--idempotent-token")
	}
	if token == "" {
		token = uuid.NewString()
	}
	return token, nil
}

func normalizeParseImageMode(raw string) (string, error) {
	mode := strings.TrimSpace(raw)
	switch mode {
	case "", parseImageModeMini, parseImageModeFlash, parseImageModeAgentic, parseImageModeAgenticMax:
		return mode, nil
	default:
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--mode must be one of mini, flash, agentic, agentic_max").
			WithParam("--mode")
	}
}

func validateParseImageTaskID(taskID string) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--task-id is required").WithParam("--task-id")
	}
	parsed, err := strconv.ParseInt(taskID, 10, 64)
	if err != nil || parsed <= 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--task-id must be a positive integer").
			WithParam("--task-id")
	}
	return nil
}

func parsePositiveDuration(raw, flag string) (time.Duration, error) {
	d, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || d <= 0 {
		return 0, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s must be a positive duration such as 10s or 20m", flag).
			WithParam(flag)
	}
	return d, nil
}

func parseImageInputError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, fileio.ErrPathValidation) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe --image path: %s", err).
			WithParam("--image").
			WithCause(err)
	}
	return errs.NewValidationError(errs.SubtypeInvalidArgument, "cannot read --image: %s", err).
		WithParam("--image").
		WithCause(err)
}
