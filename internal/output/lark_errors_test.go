// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"testing"
)

func TestMailSendErrorConstantsUseServiceScopedCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  int
		want int
	}{
		{name: "mailbox not found", got: LarkErrMailboxNotFound, want: 1234013},
		{name: "user daily send quota", got: LarkErrMailSendQuotaUser, want: 1236007},
		{name: "user external recipient quota", got: LarkErrMailSendQuotaUserExt, want: 1236008},
		{name: "tenant external recipient quota", got: LarkErrMailSendQuotaTenantExt, want: 1236009},
		{name: "mail quota", got: LarkErrMailQuota, want: 1236010},
		{name: "tenant storage limit", got: LarkErrTenantStorageLimit, want: 1236013},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.got != tt.want {
				t.Fatalf("code=%d, want %d", tt.got, tt.want)
			}
		})
	}
}
