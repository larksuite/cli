// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package publiccontent

func ScanComment(kind, body string) []Finding {
	return ScanCommentAtPath(kind, "", body)
}

func ScanCommentAtPath(kind, path, body string) []Finding {
	if kind == "" {
		kind = "comment"
	}
	if path == "" {
		path = kind
	}
	return scanText(path, "comment", body, isDetectorRuleFile(path))
}
