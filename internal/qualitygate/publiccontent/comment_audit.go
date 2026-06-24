// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package publiccontent

func ScanComment(kind, body string) []Finding {
	if kind == "" {
		kind = "comment"
	}
	return scanText(kind, "comment", body, false)
}
