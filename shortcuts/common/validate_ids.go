// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"strings"

	"github.com/larksuite/cli/internal/output"
)

// ValidateChatIDTyped checks if a chat ID has valid format (oc_ prefix).
// Also extracts token from URL if provided. param names the flag being
// validated (e.g. "--chat-ids") and is recorded on the typed error.
func ValidateChatIDTyped(param, input string) (string, error) {
	chatID, msg := normalizeChatID(input)
	if msg != "" {
		return "", ValidationErrorf("%s", msg).WithParam(param)
	}
	return chatID, nil
}

func normalizeChatID(input string) (string, string) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "chat ID cannot be empty"
	}
	// Extract from URL if present
	if strings.Contains(input, "feishu.cn") || strings.Contains(input, "larksuite.com") {
		// Extract oc_xxx from URL
		parts := strings.Split(input, "/")
		for _, part := range parts {
			if strings.HasPrefix(part, "oc_") {
				input = part
				break
			}
		}
	}
	if !strings.HasPrefix(input, "oc_") {
		return "", "invalid chat ID format, should start with 'oc_' (e.g., oc_abc123)"
	}
	return input, ""
}

// ValidateUserID checks if a user ID has valid format (ou_ prefix).
//
// Deprecated: use ValidateUserIDTyped for typed error envelopes.
func ValidateUserID(input string) (string, error) {
	userID, msg := normalizeUserID(input)
	if msg != "" {
		return "", output.ErrValidation("%s", msg)
	}
	return userID, nil
}

// ValidateUserIDTyped checks if a user ID has valid format (ou_ prefix).
// param names the flag being validated (e.g. "--creator-ids") and is
// recorded on the typed error.
func ValidateUserIDTyped(param, input string) (string, error) {
	userID, msg := normalizeUserID(input)
	if msg != "" {
		return "", ValidationErrorf("%s", msg).WithParam(param)
	}
	return userID, nil
}

func normalizeUserID(input string) (string, string) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "user ID cannot be empty"
	}
	if !strings.HasPrefix(input, "ou_") {
		return "", "invalid user ID format, should start with 'ou_' (e.g., ou_abc123)"
	}
	return input, ""
}
