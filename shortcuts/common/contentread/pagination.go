// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentread

import "strings"

// ApplyPagination enables pagination unless full was requested.
func ApplyPagination(req *Request, full bool, pageToken string, pageSize int) {
	if full {
		return
	}
	req.EnablePagination = true
	req.PageToken = strings.TrimSpace(pageToken)
	if pageSize > 0 {
		req.PageSize = int32(pageSize)
	}
}

// IsPageContinuation reports whether the caller supplied a page cursor.
// Continuations cannot fall back to an API that would restart at page one.
func IsPageContinuation(pageToken string) bool {
	return strings.TrimSpace(pageToken) != ""
}

// PaginationCursorHint reports a missing cursor without discarding readable data.
func PaginationCursorHint(hasMore bool, nextPageToken string) string {
	if hasMore && strings.TrimSpace(nextPageToken) == "" {
		return "expected next_page_token when has_more is true, but got empty; retry the read from the start"
	}
	return ""
}
