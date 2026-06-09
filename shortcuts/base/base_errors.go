// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"errors"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/util"
)

func handleBaseAPIResult(result interface{}, err error, action string) (map[string]interface{}, error) {
	data, err := handleBaseAPIResultAny(result, err, action)
	if err != nil {
		return nil, err
	}
	dataMap, _ := data.(map[string]interface{})
	return dataMap, nil
}

// handleBaseAPIResultAny normalizes the Base v3 {code,msg,data} envelope used
// by shortcut APIs. Success returns data as-is; API failures become the CLI's
// structured ErrAPI, with server-provided message/hint promoted to the top level.
func handleBaseAPIResultAny(result interface{}, err error, action string) (interface{}, error) {
	if err != nil {
		return nil, baseAPIBoundaryError(err, action)
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok || resultMap == nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "%s: API returned a malformed response envelope", action)
	}
	if _, exists := resultMap["code"]; !exists {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "%s: API response is missing code", action)
	}
	code, numeric := util.ToFloat64(resultMap["code"])
	if !numeric {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "%s: API response code is not numeric", action)
	}
	if code == 0 {
		return resultMap["data"], nil
	}

	return nil, baseAPIErrorFromResult(resultMap, errclass.ClassifyContext{})
}

// baseFlagErrorf marks flag-usage failures; it shares baseValidationErrorf's
// typed envelope and exists so call sites read as flag rejections.
func baseFlagErrorf(format string, args ...any) error {
	return baseValidationErrorf(format, args...)
}

func baseValidationErrorf(format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	err := errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", msg)
	if params := flagParams(msg); len(params) > 0 {
		err = err.WithParam(params[0].Name).WithParams(params...)
	}
	if cause := firstErrorArg(args); cause != nil {
		err = err.WithCause(cause)
	}
	return err
}

func flagParams(msg string) []errs.InvalidParam {
	reason := msg
	seen := map[string]bool{}
	params := []errs.InvalidParam{}
	for start := strings.Index(msg, "--"); start >= 0; start = strings.Index(msg, "--") {
		end := start + 2
		for end < len(msg) {
			ch := msg[end]
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' {
				end++
				continue
			}
			break
		}
		if end > start+2 {
			name := msg[start:end]
			if !seen[name] {
				seen[name] = true
				params = append(params, errs.InvalidParam{Name: name, Reason: reason})
			}
		}
		msg = msg[end:]
	}
	return params
}

func firstErrorArg(args []any) error {
	for _, arg := range args {
		if err, ok := arg.(error); ok {
			return err
		}
	}
	return nil
}

// baseMissingFileIOError reports a broken runtime wiring: a command that needs
// local file access was constructed without a FileIO provider. The user cannot
// fix this by changing flags, so it classifies as internal, not validation.
func baseMissingFileIOError(format string, args ...any) error {
	return errs.NewInternalError(errs.SubtypeFileIO, format, args...)
}

func baseInputStatError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, fileio.ErrPathValidation) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe file path: %s", err).WithCause(err)
	}
	return errs.NewValidationError(errs.SubtypeInvalidArgument, "cannot read file: %s", err).WithCause(err)
}

func baseSaveError(err error) error {
	if err == nil {
		return nil
	}
	var me *fileio.MkdirError
	switch {
	case errors.Is(err, fileio.ErrPathValidation):
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe output path: %s", err).WithCause(err)
	case errors.As(err, &me):
		return errs.NewInternalError(errs.SubtypeFileIO, "cannot create parent directory: %s", err).WithCause(err)
	default:
		return errs.NewInternalError(errs.SubtypeFileIO, "cannot create file: %s", err).WithCause(err)
	}
}

func baseAPIBoundaryError(err error, action string) error {
	if _, ok := errs.ProblemOf(err); ok {
		return err
	}
	return errs.NewNetworkError(errs.SubtypeNetworkTransport, "%s: %s", action, err).WithCause(err)
}

func baseUploadAttachmentError(filePath string, err error) error {
	if p, ok := errs.ProblemOf(err); ok {
		p.Message = fmt.Sprintf("failed to upload attachment %s: %s", filePath, p.Message)
		return err
	}
	return errs.NewInternalError(errs.SubtypeSDKError, "failed to upload attachment %s: %s", filePath, err).WithCause(err)
}

func baseAPIErrorFromResult(resultMap map[string]interface{}, cc errclass.ClassifyContext) error {
	if resultMap == nil {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "API returned a malformed response envelope")
	}
	if msg := extractDataErrorMessage(resultMap); msg != "" {
		resultMap["msg"] = msg
	}
	hint := extractErrorHint(resultMap)
	if logID := extractBaseErrorLogID(resultMap); logID != "" {
		resultMap["log_id"] = logID
	}
	err := errclass.BuildAPIError(resultMap, cc)
	if err == nil {
		return nil
	}
	if p, ok := errs.ProblemOf(err); ok && hint != "" {
		p.Hint = hint
	}
	return err
}

func enrichBaseAPIErrorFromBody(err error, body []byte, cc errclass.ClassifyContext) error {
	if _, ok := errs.ProblemOf(err); !ok {
		return err
	}
	result, parseErr := decodeBaseV3Response(body)
	if parseErr != nil {
		return err
	}
	enriched := baseAPIErrorFromResult(result, cc)
	if enriched == nil {
		return err
	}
	src, _ := errs.ProblemOf(enriched)
	dst, _ := errs.ProblemOf(err)
	if src != nil && dst != nil {
		dst.Message = src.Message
		dst.Hint = src.Hint
		// A body without log_id must not erase a header-derived LogID
		// already carried by err.
		if src.LogID != "" {
			dst.LogID = src.LogID
		}
	}
	return err
}

func extractBaseErrorLogID(resultMap map[string]interface{}) string {
	for _, key := range []string{"log_id", "logid"} {
		if logID, _ := resultMap[key].(string); strings.TrimSpace(logID) != "" {
			return strings.TrimSpace(logID)
		}
	}
	if detail, ok := resultMap["error"].(map[string]interface{}); ok {
		for _, key := range []string{"log_id", "logid"} {
			if logID, _ := detail[key].(string); strings.TrimSpace(logID) != "" {
				return strings.TrimSpace(logID)
			}
		}
	}
	data, _ := resultMap["data"].(map[string]interface{})
	if detail, ok := data["error"].(map[string]interface{}); ok {
		for _, key := range []string{"log_id", "logid"} {
			if logID, _ := detail[key].(string); strings.TrimSpace(logID) != "" {
				return strings.TrimSpace(logID)
			}
		}
	}
	return ""
}

func extractErrorHint(resultMap map[string]interface{}) string {
	if detail, ok := resultMap["error"].(map[string]interface{}); ok {
		if hint := consumeStringField(detail, "hint"); hint != "" {
			return hint
		}
	}
	data, _ := resultMap["data"].(map[string]interface{})
	if detail, ok := data["error"].(map[string]interface{}); ok {
		if hint := consumeStringField(detail, "hint"); hint != "" {
			return hint
		}
	}
	return ""
}

func extractDataErrorMessage(resultMap map[string]interface{}) string {
	data, _ := resultMap["data"].(map[string]interface{})
	if detail, ok := data["error"].(map[string]interface{}); ok {
		if message := consumeStringField(detail, "message"); message != "" {
			return message
		}
	}
	return ""
}

func consumeStringField(src map[string]interface{}, key string) string {
	value, _ := src[key].(string)
	if _, exists := src[key]; exists {
		delete(src, key)
	}
	return strings.TrimSpace(value)
}
