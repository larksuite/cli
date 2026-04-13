// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/util"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	// ErrCodeOkrInvalidParams is returned when request parameters are invalid.
	ErrCodeOkrInvalidParams = 1001001
	// ErrCodeOkrPermDenied is returned when the user has no permission.
	ErrCodeOkrPermDenied = 1001002
	// ErrCodeOkrUserNotFound is returned when the user is not found.
	ErrCodeOkrUserNotFound = 1001003
	// ErrCodeOkrNotFound is returned when OKR data is not found.
	ErrCodeOkrNotFound = 1001004
	// ErrCodeOkrDuplicatePeriod is returned when a period date conflicts.
	ErrCodeOkrDuplicatePeriod = 1001008
	// ErrCodeOkrEditionRequired is returned when a higher edition is required.
	ErrCodeOkrEditionRequired = 1000403
	// ErrCodeOkrSystemException is returned for system exceptions.
	ErrCodeOkrSystemException = 1009998
	// ErrCodeOkrInternalError is returned for unknown internal errors.
	ErrCodeOkrInternalError = 1009999
)

// OkrErrorInfo maps Lark error codes to standardized error info.
type OkrErrorInfo struct {
	Type     string
	Message  string
	Hint     string
	ExitCode int
}

var okrErrorMap = map[int]OkrErrorInfo{
	ErrCodeOkrInvalidParams:   {"validation_error", "Invalid request parameters", "Please check required fields and parameter values.", output.ExitValidation},
	ErrCodeOkrPermDenied:      {"permission_error", "Permission denied", "Please check if the calling identity has the necessary OKR permissions. Run: lark-cli auth login --domain okr", output.ExitAPI},
	ErrCodeOkrUserNotFound:    {"not_found", "User not found", "Please verify the user ID is correct.", output.ExitAPI},
	ErrCodeOkrNotFound:        {"not_found", "OKR data not found", "Please verify the OKR, objective, or key result ID is correct.", output.ExitAPI},
	ErrCodeOkrDuplicatePeriod: {"conflict", "Duplicate period or date conflict", "A period with overlapping dates already exists.", output.ExitAPI},
	ErrCodeOkrEditionRequired: {"permission_error", "Business edition or above required", "This operation requires Feishu Business edition or above.", output.ExitAPI},
	ErrCodeOkrSystemException: {"api_error", "System exception", "Please try again. If the error persists, contact support.", output.ExitAPI},
	ErrCodeOkrInternalError:   {"api_error", "Internal server error", "Please try again. If the error persists, contact support.", output.ExitAPI},
}

// WrapOkrError wraps a Lark API error into a standardized ExitError based on OKR-specific rules.
func WrapOkrError(larkCode int, rawMsg string, action string) error {
	info, ok := okrErrorMap[larkCode]
	if !ok {
		exitCode, errType, hint := output.ClassifyLarkError(larkCode, rawMsg)

		genericMsg := "API error"
		switch errType {
		case "permission":
			genericMsg = "Permission denied"
		case "auth":
			genericMsg = "Authentication failed"
		case "config":
			genericMsg = "Configuration error"
		case "rate_limit":
			genericMsg = "Rate limit exceeded"
		}

		displayMsg := fmt.Sprintf("%s: %s [%d] (Details: %s)", action, genericMsg, larkCode, rawMsg)
		return &output.ExitError{
			Code: exitCode,
			Detail: &output.ErrDetail{
				Type:    errType,
				Code:    larkCode,
				Message: displayMsg,
				Hint:    hint,
			},
		}
	}

	return &output.ExitError{
		Code: info.ExitCode,
		Detail: &output.ErrDetail{
			Type:    info.Type,
			Code:    larkCode,
			Message: fmt.Sprintf("%s: %s (Details: %s)", action, info.Message, rawMsg),
			Hint:    info.Hint,
		},
	}
}

// HandleOkrApiResult checks for network/API errors and returns the "data" field.
func HandleOkrApiResult(result interface{}, err error, action string) (map[string]interface{}, error) {
	if err != nil {
		return nil, err
	}

	resultMap, _ := result.(map[string]interface{})
	codeVal, hasCode := resultMap["code"]
	if !hasCode {
		data, err := common.HandleApiResult(result, err, action)
		return data, err
	}

	code, _ := util.ToFloat64(codeVal)
	larkCode := int(code)
	if larkCode != 0 {
		rawMsg, _ := resultMap["msg"].(string)
		return nil, WrapOkrError(larkCode, rawMsg, action)
	}

	data, _ := resultMap["data"].(map[string]interface{})
	return data, nil
}

// findCurrentPeriod returns the first period whose time range covers now and status is normal (0).
func findCurrentPeriod(periods []interface{}) map[string]interface{} {
	now := time.Now().UnixMilli()
	for _, p := range periods {
		period, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		status, _ := util.ToFloat64(period["status"])
		if int(status) != 0 {
			continue
		}
		startStr, _ := period["period_start_time"].(string)
		endStr, _ := period["period_end_time"].(string)
		start, _ := strconv.ParseInt(startStr, 10, 64)
		end, _ := strconv.ParseInt(endStr, 10, 64)
		if start <= now && now <= end {
			return period
		}
	}
	return nil
}

// splitIDs splits a comma-separated string into a slice, trimming whitespace.
func splitIDs(s string) []string {
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// formatProgressPercent formats a progress_rate object into a display string.
func formatProgressPercent(progressRate map[string]interface{}) string {
	percent, _ := util.ToFloat64(progressRate["percent"])
	statusStr, _ := progressRate["status"].(string)

	var statusLabel string
	switch statusStr {
	case "0":
		statusLabel = "normal"
	case "1":
		statusLabel = "at risk"
	case "2":
		statusLabel = "delayed"
	default:
		statusLabel = ""
	}

	if statusLabel != "" {
		return fmt.Sprintf("%.0f%% (%s)", percent, statusLabel)
	}
	return fmt.Sprintf("%.0f%%", percent)
}

// formatTimestampMs formats a millisecond timestamp string to local time.
func formatTimestampMs(tsStr string) string {
	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil || ts == 0 {
		return ""
	}
	return time.UnixMilli(ts).Local().Format("2006-01-02")
}

// resolveTargetType converts a human-readable target type to the API numeric value.
// Accepts: "objective" or "2" → "2", "key_result" or "3" → "3".
func resolveTargetType(input string) (string, error) {
	switch input {
	case "objective", "2":
		return "2", nil
	case "key_result", "3":
		return "3", nil
	default:
		return "", fmt.Errorf("invalid --target-type %q: must be objective, key_result, 2, or 3", input)
	}
}

// wrapPlainTextContent converts a plain text string into Lark rich text format for progress records.
func wrapPlainTextContent(text string) map[string]interface{} {
	return map[string]interface{}{
		"blocks": []interface{}{
			map[string]interface{}{
				"type": "paragraph",
				"paragraph": map[string]interface{}{
					"elements": []interface{}{
						map[string]interface{}{
							"type": "textRun",
							"textRun": map[string]interface{}{
								"text": text,
							},
						},
					},
				},
			},
		},
	}
}
