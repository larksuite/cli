// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package publiccontent

import (
	"encoding/base64"
	"net/url"
	"strings"
)

func credentialValueHasStrongEvidence(key, value string) bool {
	normalized := strings.TrimRight(strings.TrimSpace(value), ",;")
	normalized = strings.TrimSpace(strings.Trim(normalized, `"'<>`))
	candidates := credentialEvidenceCandidates(unwrapCredentialValue(normalized))
	for _, candidate := range candidates {
		if providerCredentialIdentifier(candidate) {
			return true
		}
	}
	if isCredentialMetadataField(key) {
		return false
	}
	for _, candidate := range candidates {
		if highEntropyCredentialValue(strings.ToLower(candidate)) || base64PaddedCredentialValue(candidate) {
			return true
		}
	}
	return percentEncodedCredentialValue(strings.ToLower(candidates[0])) ||
		commandSubstitutionLooksCredentialLike(strings.ToLower(normalized))
}

func credentialEvidenceCandidates(value string) []string {
	candidates := []string{value}
	for range 3 {
		decoded, err := url.PathUnescape(value)
		if err != nil || decoded == value {
			break
		}
		candidates = append(candidates, decoded)
		value = decoded
	}
	return candidates
}

func isCredentialMetadataField(key string) bool {
	if isBenignTokenField(key) {
		return true
	}
	parts := credentialKeyParts(strings.ReplaceAll(strings.ToLower(key), "-", "_"))
	if len(parts) < 2 {
		return false
	}
	switch parts[len(parts)-1] {
	case "hash", "id", "kind", "marker", "prefix", "transport":
		return true
	default:
		return false
	}
}

func base64PaddedCredentialValue(value string) bool {
	if len(value) < 16 || !strings.HasSuffix(value, "=") {
		return false
	}
	if _, err := base64.StdEncoding.DecodeString(value); err != nil {
		return false
	}
	return shannonEntropy(strings.TrimRight(value, "=")) >= 3.5
}

func percentEncodedCredentialValue(value string) bool {
	if len(value) < 16 {
		return false
	}
	var escapes int
	for i := 0; i+2 < len(value); i++ {
		if value[i] == '%' && isHexByte(value[i+1]) && isHexByte(value[i+2]) {
			escapes++
			i += 2
		}
	}
	return escapes >= 2
}

func isHexByte(value byte) bool {
	return (value >= '0' && value <= '9') || (value >= 'a' && value <= 'f')
}
