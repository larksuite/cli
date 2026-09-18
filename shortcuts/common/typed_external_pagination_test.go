// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"testing"

	internalpagination "github.com/larksuite/cli/internal/pagination"
)

// A complete-set collection holds every page in memory before the workflow's
// writes run, so its bound is a host resource decision — deliberately tighter
// than the user-facing --page-limit maximum.
func TestCollectAllPolicyUsesWorkflowHardBound(t *testing.T) {
	policy, err := commandPagePolicy(typedRuntimeContext(nil), true)
	if err != nil {
		t.Fatal(err)
	}
	if policy.maxPages != internalpagination.CollectAllHardPageBound {
		t.Fatalf("collect-all maxPages = %d, want %d", policy.maxPages, internalpagination.CollectAllHardPageBound)
	}
	if internalpagination.CollectAllHardPageBound >= pageLimitMaximum {
		t.Fatalf("workflow bound (%d) must stay below the user-facing --page-limit maximum (%d)",
			internalpagination.CollectAllHardPageBound, pageLimitMaximum)
	}
	if internalpagination.CollectAllHardPageBound != 100 {
		t.Fatalf("workflow bound = %d, want the Phase 0 value 100", internalpagination.CollectAllHardPageBound)
	}
}
