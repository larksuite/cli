// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/util"
	"github.com/larksuite/cli/shortcuts/common"
)

var relativeTimeRe = regexp.MustCompile(`^([+-])(\d+)([dwmh])$`)

func isRelativeTime(s string) bool {
	return relativeTimeRe.MatchString(s)
}

func parseRelativeTime(s string) (time.Time, error) {
	matches := relativeTimeRe.FindStringSubmatch(s)
	if len(matches) == 0 {
		return time.Time{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid relative time format: %s", s)
	}

	sign := matches[1]
	amountStr := matches[2]
	unit := matches[3]

	amount, err := strconv.Atoi(amountStr)
	if err != nil {
		return time.Time{}, err
	}

	if sign == "-" {
		amount = -amount
	}

	now := time.Now()
	switch unit {
	case "d":
		return now.AddDate(0, 0, amount), nil
	case "w":
		return now.AddDate(0, 0, amount*7), nil
	case "m":
		return now.Add(time.Duration(amount) * time.Minute), nil
	case "h":
		return now.Add(time.Duration(amount) * time.Hour), nil
	}
	panic(fmt.Sprintf("unreachable: relativeTimeRe matched unexpected unit %q", unit))
}

const (
	// ErrCodeTaskInvalidParams is returned when request parameters are invalid.
	ErrCodeTaskInvalidParams = 1470400
	// ErrCodeTaskPermissionDenied is returned when the user has no permission.
	ErrCodeTaskPermissionDenied = 1470403
	// ErrCodeTaskNotFound is returned when the resource is not found.
	ErrCodeTaskNotFound = 1470404
	// ErrCodeTaskConflict is returned when concurrent call conflict.
	ErrCodeTaskConflict = 1470422
	// ErrCodeTaskInternalError is returned when server error occurs.
	ErrCodeTaskInternalError = 1470500
	// ErrCodeTaskAssigneeLimit is returned when assignee limit exceeded.
	ErrCodeTaskAssigneeLimit = 1470610
	// ErrCodeTaskFollowerLimit is returned when follower limit exceeded.
	ErrCodeTaskFollowerLimit = 1470611
	// ErrCodeTasklistMemberLimit is returned when tasklist member limit exceeded.
	ErrCodeTasklistMemberLimit = 1470612
	// ErrCodeTaskReminderExists is returned when reminder already exists.
	ErrCodeTaskReminderExists = 1470613
)

// taskAPIHints carries the task-specific recovery hint for each known Lark API
// code, layered onto the typed error after errclass.BuildAPIError classifies
// it. errclass.APIHint only covers context-free subtypes (e.g. conflict); these
// hints carry the resource context APIHint intentionally leaves to the caller.
// Authorization (1470403) is omitted: BuildAPIError already attaches the
// canonical permission hint.
var taskAPIHints = map[int]string{
	ErrCodeTaskInvalidParams:   "Please check required fields, field lengths, or parameter logic (e.g., reminders require a due date).",
	ErrCodeTaskNotFound:        "Please verify if the task, tasklist, or group ID is correct and has not been deleted.",
	ErrCodeTaskConflict:        "Avoid making concurrent API calls using the same client_token.",
	ErrCodeTaskInternalError:   "Please try again. If the error persists, check the content validity or contact support.",
	ErrCodeTaskAssigneeLimit:   "The current task has reached the maximum number of assignees.",
	ErrCodeTaskFollowerLimit:   "The current task has reached the maximum number of followers.",
	ErrCodeTasklistMemberLimit: "The current tasklist has reached the maximum number of members.",
	ErrCodeTaskReminderExists:  "The task already has a reminder set. Remove the existing reminder before adding a new one.",
}

func callTaskAPITyped(runtime *common.RuntimeContext, method, url string, params map[string]interface{}, body interface{}) (map[string]interface{}, error) {
	data, err := runtime.CallAPITyped(method, url, params, body)
	return data, applyTaskAPIHint(err)
}

func applyTaskAPIHint(err error) error {
	if err == nil {
		return nil
	}
	if p, ok := errs.ProblemOf(err); ok {
		if hint := taskAPIHints[p.Code]; hint != "" {
			p.Hint = hint
		}
	}
	return err
}

// HandleTaskApiResult interprets a parsed Lark API response. A non-zero code is
// classified into a typed errs.* error by errclass.BuildAPIError — Category,
// Subtype, Code, and log_id are sourced from internal/errclass/codemeta_task.go
// — with the task-specific recovery hint (taskAPIHints) layered on top.
func HandleTaskApiResult(result interface{}, err error, action string) (map[string]interface{}, error) {
	return handleTaskAPIResult(result, err, action, errclass.ClassifyContext{})
}

func HandleTaskApiResultWithContext(result interface{}, err error, action string, cc errclass.ClassifyContext) (map[string]interface{}, error) {
	return handleTaskAPIResult(result, err, action, cc)
}

func handleTaskAPIResult(result interface{}, err error, action string, cc errclass.ClassifyContext) (map[string]interface{}, error) {
	if err != nil {
		return nil, err
	}

	resultMap, _ := result.(map[string]interface{})
	codeVal, hasCode := resultMap["code"]
	if !hasCode {
		// A Lark response always carries a top-level code; its absence (with no
		// transport error) means a malformed or unexpected body.
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "%s: unexpected response (missing code field)", action)
	}

	code, ok := util.ToFloat64(codeVal)
	if !ok {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "%s: malformed response (non-numeric code %v)", action, codeVal)
	}
	larkCode := int(code)
	if larkCode != 0 {
		typedErr := errclass.BuildAPIError(resultMap, cc)
		return nil, applyTaskAPIHint(typedErr)
	}

	data, _ := resultMap["data"].(map[string]interface{})
	return data, nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// truncateTaskURL removes extra query parameters from task applink, keeping only guid.
func truncateTaskURL(u string) string {
	if u == "" {
		return ""
	}
	if idx := strings.Index(u, "&"); idx != -1 {
		return u[:idx]
	}
	return u
}

// parseTimeFlagSec parses a time flag that can be absolute (ISO 8601, timestamp) or relative (+/- Nd/w/m/h).
// It returns the Unix seconds string.
func parseTimeFlagSec(input string, hint string) (string, error) {
	if isRelativeTime(input) {
		t, err := parseRelativeTime(input)
		if err != nil {
			return "", err
		}
		// Snap to day if unit is days or weeks
		if strings.HasSuffix(input, "d") || strings.HasSuffix(input, "w") {
			if hint == "end" {
				t = time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, t.Location())
			} else {
				t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
			}
		}
		return fmt.Sprintf("%d", t.Unix()), nil
	}
	return common.ParseTime(input, hint)
}
