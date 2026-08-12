// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import "testing"

// A complete-set collection holds every page in memory before the workflow's
// writes run, so its bound is a host resource decision — deliberately tighter
// than the user-facing --page-limit maximum.
func TestCollectAllPolicyUsesWorkflowHardBound(t *testing.T) {
	policy, err := commandPagePolicy(CommandContext(nil), true)
	if err != nil {
		t.Fatal(err)
	}
	if policy.maxPages != collectAllHardPageBound {
		t.Fatalf("collect-all maxPages = %d, want %d", policy.maxPages, collectAllHardPageBound)
	}
	if collectAllHardPageBound >= pageLimitMaximum {
		t.Fatalf("workflow bound (%d) must stay below the user-facing --page-limit maximum (%d)",
			collectAllHardPageBound, pageLimitMaximum)
	}
	if collectAllHardPageBound != 100 {
		t.Fatalf("workflow bound = %d, want the Phase 0 value 100", collectAllHardPageBound)
	}
}
