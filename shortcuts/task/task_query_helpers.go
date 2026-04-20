// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func splitAndTrimCSV(input string) []string {
	parts := strings.Split(input, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseTimeRangeMillis(input string) (string, string, error) {
	if strings.TrimSpace(input) == "" {
		return "", "", nil
	}

	parts := strings.SplitN(input, ",", 2)
	startInput := strings.TrimSpace(parts[0])
	endInput := ""
	if len(parts) == 2 {
		endInput = strings.TrimSpace(parts[1])
	}

	var startMillis, endMillis string
	var startSecInt, endSecInt int64
	var hasStart, hasEnd bool
	if startInput != "" {
		startSec, err := parseTimeFlagSec(startInput, "start")
		if err != nil {
			return "", "", err
		}
		startSecInt, err = strconv.ParseInt(startSec, 10, 64)
		if err != nil {
			return "", "", fmt.Errorf("invalid start timestamp: %w", err)
		}
		hasStart = true
		startMillis = startSec + "000"
	}
	if endInput != "" {
		endSec, err := parseTimeFlagSec(endInput, "end")
		if err != nil {
			return "", "", err
		}
		endSecInt, err = strconv.ParseInt(endSec, 10, 64)
		if err != nil {
			return "", "", fmt.Errorf("invalid end timestamp: %w", err)
		}
		hasEnd = true
		endMillis = endSec + "000"
	}
	if hasStart && hasEnd && startSecInt > endSecInt {
		return "", "", fmt.Errorf("start time must be earlier than or equal to end time")
	}
	return startMillis, endMillis, nil
}

func parseTimeRangeRFC3339(input string) (string, string, error) {
	if strings.TrimSpace(input) == "" {
		return "", "", nil
	}

	parts := strings.SplitN(input, ",", 2)
	startInput := strings.TrimSpace(parts[0])
	endInput := ""
	if len(parts) == 2 {
		endInput = strings.TrimSpace(parts[1])
	}

	var startTime, endTime string
	var startSecInt, endSecInt int64
	var hasStart, hasEnd bool
	if startInput != "" {
		startSec, err := parseTimeFlagSec(startInput, "start")
		if err != nil {
			return "", "", err
		}
		startSecInt, err = strconv.ParseInt(startSec, 10, 64)
		if err != nil {
			return "", "", fmt.Errorf("invalid start timestamp: %w", err)
		}
		hasStart = true
		startTime = time.Unix(startSecInt, 0).Local().Format(time.RFC3339)
	}
	if endInput != "" {
		endSec, err := parseTimeFlagSec(endInput, "end")
		if err != nil {
			return "", "", err
		}
		endSecInt, err = strconv.ParseInt(endSec, 10, 64)
		if err != nil {
			return "", "", fmt.Errorf("invalid end timestamp: %w", err)
		}
		hasEnd = true
		endTime = time.Unix(endSecInt, 0).Local().Format(time.RFC3339)
	}
	if hasStart && hasEnd && startSecInt > endSecInt {
		return "", "", fmt.Errorf("start time must be earlier than or equal to end time")
	}
	return startTime, endTime, nil
}

func formatTaskDateTimeMillis(msStr string) string {
	if msStr == "" || msStr == "0" {
		return ""
	}
	ms, err := strconv.ParseInt(msStr, 10, 64)
	if err != nil {
		return ""
	}
	return time.UnixMilli(ms).Local().Format(time.DateTime)
}

func outputTaskSummary(task map[string]interface{}) map[string]interface{} {
	urlVal, _ := task["url"].(string)
	urlVal = truncateTaskURL(urlVal)

	out := map[string]interface{}{
		"guid":    task["guid"],
		"summary": task["summary"],
		"url":     urlVal,
	}
	if createdAt, _ := task["created_at"].(string); createdAt != "" {
		if created := formatTaskDateTimeMillis(createdAt); created != "" {
			out["created_at"] = created
		}
	}
	if completedAt, _ := task["completed_at"].(string); completedAt != "" {
		if completed := formatTaskDateTimeMillis(completedAt); completed != "" {
			out["completed_at"] = completed
		}
	}
	if updatedAt, _ := task["updated_at"].(string); updatedAt != "" {
		if updated := formatTaskDateTimeMillis(updatedAt); updated != "" {
			out["updated_at"] = updated
		}
	}
	if dueObj, ok := task["due"].(map[string]interface{}); ok {
		if tsStr, _ := dueObj["timestamp"].(string); tsStr != "" {
			if dueAt := formatTaskDateTimeMillis(tsStr); dueAt != "" {
				out["due_at"] = dueAt
			}
		}
	}
	return out
}

func outputRelatedTask(task map[string]interface{}) map[string]interface{} {
	urlVal, _ := task["url"].(string)
	urlVal = truncateTaskURL(urlVal)

	out := map[string]interface{}{
		"guid":          task["guid"],
		"summary":       task["summary"],
		"description":   task["description"],
		"status":        task["status"],
		"source":        task["source"],
		"mode":          task["mode"],
		"subtask_count": task["subtask_count"],
		"tasklists":     task["tasklists"],
		"url":           urlVal,
	}
	if creator, ok := task["creator"].(map[string]interface{}); ok {
		out["creator"] = creator
	}
	if members, ok := task["members"].([]interface{}); ok {
		out["members"] = members
	}
	if createdAt, _ := task["created_at"].(string); createdAt != "" {
		if created := formatTaskDateTimeMillis(createdAt); created != "" {
			out["created_at"] = created
		}
	}
	if completedAt, _ := task["completed_at"].(string); completedAt != "" {
		if completed := formatTaskDateTimeMillis(completedAt); completed != "" {
			out["completed_at"] = completed
		}
	}
	return out
}

func buildTimeRangeFilter(key, start, end string) map[string]interface{} {
	timeRange := map[string]interface{}{}
	if start != "" {
		timeRange["start_time"] = start
	}
	if end != "" {
		timeRange["end_time"] = end
	}
	if len(timeRange) == 0 {
		return nil
	}
	return map[string]interface{}{key: timeRange}
}

func mergeIntoFilter(dst map[string]interface{}, src map[string]interface{}) {
	for k, v := range src {
		dst[k] = v
	}
}

func requireSearchFilter(query string, filter map[string]interface{}, action string) error {
	if strings.TrimSpace(query) != "" {
		return nil
	}
	if len(filter) > 0 {
		return nil
	}
	return WrapTaskError(ErrCodeTaskInvalidParams, "query is empty and no filter is provided", action)
}

func renderRelatedTasksPretty(items []map[string]interface{}, hasMore bool, pageToken string) string {
	var b strings.Builder
	for i, item := range items {
		fmt.Fprintf(&b, "[%d] %v\n", i+1, item["summary"])
		fmt.Fprintf(&b, "    GUID: %v\n", item["guid"])
		if status, _ := item["status"].(string); status != "" {
			fmt.Fprintf(&b, "    Status: %s\n", status)
		}
		if created, _ := item["created_at"].(string); created != "" {
			fmt.Fprintf(&b, "    Created: %s\n", created)
		}
		if completed, _ := item["completed_at"].(string); completed != "" {
			fmt.Fprintf(&b, "    Completed: %s\n", completed)
		}
		if urlVal, _ := item["url"].(string); urlVal != "" {
			fmt.Fprintf(&b, "    URL: %s\n", urlVal)
		}
		b.WriteString("\n")
	}
	if hasMore && pageToken != "" {
		fmt.Fprintf(&b, "Next page token: %s\n", pageToken)
	}
	return b.String()
}
