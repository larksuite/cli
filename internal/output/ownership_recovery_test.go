// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"strings"
	"testing"
)

func checkOwnershipRecoveryHint(t *testing.T, hint string) {
	t.Helper()

	for _, part := range []string{
		"im +messages-resources-download",
		"--output <output_path>",
		"This resource belongs to another user",
		"download",
		"send",
		"image_key",
		"file_key",
	} {
		if !strings.Contains(hint, part) {
			t.Fatalf("hint %q missing %q", hint, part)
		}
	}
	if len(hint) > 360 {
		t.Fatalf("hint is too long: %d bytes", len(hint))
	}
	for _, noisy := range []string{
		"Step 1",
		"Step 2",
		"Step 3",
		"--message-id <message_id>",
		"--file-key <resource_key>",
		"--type <image|file>",
		"identity",
		"do not keep retrying alternative download methods",
		"POST /open-apis",
	} {
		if strings.Contains(hint, noisy) {
			t.Fatalf("hint %q should not contain noisy phrase %q", hint, noisy)
		}
	}
}

func TestBuildOwnershipRecoveryHint(t *testing.T) {
	checkOwnershipRecoveryHint(t, buildOwnershipRecoveryHint())
}

func TestErrAPI_OwnershipMismatch(t *testing.T) {
	upstreamMessage := "Bot or User is NOT the owner of the uat resource."
	err := ErrAPI(LarkErrOwnershipMismatch, upstreamMessage, map[string]any{"log_id": "test-log"})

	if err.Code != ExitAPI {
		t.Fatalf("exit code = %d, want %d", err.Code, ExitAPI)
	}
	if err.Detail == nil {
		t.Fatal("expected detail")
	}
	if err.Detail.Type != "ownership_mismatch" {
		t.Fatalf("type = %q, want %q", err.Detail.Type, "ownership_mismatch")
	}
	if got, want := err.Detail.Message, upstreamMessage; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
	checkOwnershipRecoveryHint(t, err.Detail.Hint)
	if err.Detail.Detail == nil {
		t.Fatal("expected upstream detail to be preserved")
	}
}
