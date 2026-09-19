//go:build windows

// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keysigner

import (
	"errors"
	"syscall"
	"testing"
)

func TestCNGErrorClassificationUsesSecurityStatus(t *testing.T) {
	for _, tc := range []struct {
		name        string
		status      uint32
		unavailable bool
		notFound    bool
	}{
		{name: "unsupported", status: 0x80090029, unavailable: true},
		{name: "device not ready", status: 0x80090030, unavailable: true},
		{name: "TPM missing", status: 0x8028400F, unavailable: true},
		{name: "platform device not ready", status: 0x80290401, unavailable: true},
		{name: "key missing", status: nteNotFound, notFound: true},
		{name: "bad keyset", status: nteBadKeyset, notFound: true},
		{name: "access denied", status: 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := cngError("fixture", uintptr(tc.status))
			if !errors.Is(err, syscall.Errno(tc.status)) {
				t.Fatalf("lost SECURITY_STATUS 0x%08X: %v", tc.status, err)
			}
			if errors.Is(err, ErrUnavailable) != tc.unavailable {
				t.Fatalf("ErrUnavailable = %v, want %v", errors.Is(err, ErrUnavailable), tc.unavailable)
			}
			if isCNGNotFound(err) != tc.notFound {
				t.Fatalf("isCNGNotFound() = %v, want %v", isCNGNotFound(err), tc.notFound)
			}
		})
	}
}
