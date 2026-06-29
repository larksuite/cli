// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package publiccontent

import "testing"

func TestScanCommentAuditsPublishedCommentBodies(t *testing.T) {
	got := ScanComment("issue_comment", `The published comment included /tmp/harness`+`-agent/run and CCM`+`-Harness: stage-4`)
	rules := findingRules(got)
	if !rules["public_content_harness_metadata"] || !rules["public_content_ccm_harness_trailer"] {
		t.Fatalf("comment audit findings = %#v", got)
	}
	for _, item := range got {
		if item.File != "issue_comment" {
			t.Fatalf("comment finding file = %q, want issue_comment", item.File)
		}
	}
}
