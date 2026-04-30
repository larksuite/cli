// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"strings"
	"testing"
)

// TestClassifyLarkError_DriveCreateShortcutConstraints verifies known Drive shortcut errors map to actionable hints.
func TestClassifyLarkError_DriveCreateShortcutConstraints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		code         int
		wantExitCode int
		wantType     string
		wantHint     string
	}{
		{
			name:         "resource contention",
			code:         LarkErrDriveResourceContention,
			wantExitCode: ExitAPI,
			wantType:     "conflict",
			wantHint:     "avoid concurrent duplicate requests",
		},
		{
			name:         "cross tenant unit",
			code:         LarkErrDriveCrossTenantUnit,
			wantExitCode: ExitAPI,
			wantType:     "cross_tenant_unit",
			wantHint:     "same tenant and region/unit",
		},
		{
			name:         "cross brand",
			code:         LarkErrDriveCrossBrand,
			wantExitCode: ExitAPI,
			wantType:     "cross_brand",
			wantHint:     "same brand environment",
		},
		{
			name:         "sheets float image invalid dims",
			code:         LarkErrSheetsFloatImageInvalidDims,
			wantExitCode: ExitAPI,
			wantType:     "invalid_params",
			wantHint:     "--width / --height / --offset-x / --offset-y",
		},
		{
			name:         "drive permission apply rate limit",
			code:         LarkErrDrivePermApplyRateLimit,
			wantExitCode: ExitAPI,
			wantType:     "rate_limit",
			wantHint:     "5 times per day",
		},
		{
			name:         "drive permission apply not applicable",
			code:         LarkErrDrivePermApplyNotApplicable,
			wantExitCode: ExitAPI,
			wantType:     "invalid_params",
			wantHint:     "does not accept a permission-apply request",
		},
		{
			name:         "ownership mismatch",
			code:         LarkErrOwnershipMismatch,
			wantExitCode: ExitAPI,
			wantType:     "ownership_mismatch",
			wantHint:     "messages-resources-download",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotExitCode, gotType, gotHint := ClassifyLarkError(tt.code, "raw msg")
			if gotExitCode != tt.wantExitCode {
				t.Fatalf("exitCode=%d, want %d", gotExitCode, tt.wantExitCode)
			}
			if gotType != tt.wantType {
				t.Fatalf("type=%q, want %q", gotType, tt.wantType)
			}
			if gotHint == "" {
				t.Fatal("expected non-empty hint")
			}
			if !strings.Contains(gotHint, tt.wantHint) {
				t.Fatalf("hint=%q, want substring %q", gotHint, tt.wantHint)
			}
		})
	}
}
