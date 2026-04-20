// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"errors"
	"fmt"

	"github.com/larksuite/cli/internal/output"
)

const (
	errCodeInvalidParamsWithDetail = 190014
)

// getErrorDetailValue extracts the first detail value from the output.ErrDetail.
// It assumes Detail is a map containing a "details" array of objects with "value" string fields.
// For example: {"details": [{"value": "error message 1"}, {"value": "error message 2"}]}
// Returns an empty string if the structure doesn't match or the array is empty.
func getErrorDetailValue(e *output.ErrDetail) string {
	if e == nil || e.Detail == nil {
		return ""
	}

	errMap, ok := e.Detail.(map[string]interface{})
	if !ok {
		return ""
	}

	details, ok := errMap["details"].([]interface{})
	if !ok || len(details) == 0 {
		return ""
	}

	detailObj, ok := details[0].(map[string]interface{})
	if !ok {
		return ""
	}

	val, _ := detailObj["value"].(string)
	return val
}

// wrapPredefinedError wraps an error into *output.ExitError if it matches predefined error codes.
// Currently handles error code 190014 (invalid params with detail), extracting the detail value into the message.
// If the error is nil or doesn't match predefined codes, returns the original error.
func wrapPredefinedError(err error) error {
	if err == nil {
		return nil
	}

	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		return err
	}

	if exitErr.Detail.Code == errCodeInvalidParamsWithDetail {
		if val := getErrorDetailValue(exitErr.Detail); val != "" {
			fullMsg := fmt.Sprintf("%s: %s", exitErr.Detail.Message, val)
			return output.ErrAPI(exitErr.Detail.Code, fullMsg, exitErr.Detail.Detail)
		}
	}

	return err
}
