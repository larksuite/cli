// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package binding

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/vfs"
)

// SingleValueFileRefID is the required ref.ID for singleValue file mode
// (aligned with OpenClaw ref-contract.ts SINGLE_VALUE_FILE_REF_ID).
const SingleValueFileRefID = "$SINGLE_VALUE"

// resolveFileRef handles {source:"file"} SecretRef resolution.
// Reads the file via assertSecurePath audit, then extracts the secret value
// based on the provider's mode (singleValue or json with JSON Pointer).
func resolveFileRef(ref *SecretRef, pc *ProviderConfig) (string, error) {
	if pc.Path == "" {
		return "", fmt.Errorf("file provider path is empty")
	}

	// Security audit on file path
	securePath, err := AssertSecurePath(AuditParams{
		TargetPath:            pc.Path,
		Label:                 "secrets.providers file path",
		TrustedDirs:           pc.TrustedDirs,
		AllowInsecurePath:     pc.AllowInsecurePath,
		AllowReadableByOthers: false, // file provider: strict by default
		AllowSymlinkPath:      false,
	})
	if err != nil {
		return "", fmt.Errorf("file provider security audit failed: %w", err)
	}

	// Read file content
	maxBytes := pc.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultFileMaxBytes
	}

	// Note: vfs.ReadFile loads the entire file. maxBytes is enforced post-read
	// because vfs does not expose a size-limited reader. For secret files this
	// is acceptable (default limit 1 MiB; secrets are typically < 1 KB).
	data, err := vfs.ReadFile(securePath)
	if err != nil {
		return "", fmt.Errorf("failed to read secret file %s: %w", securePath, err)
	}

	if len(data) > maxBytes {
		return "", fmt.Errorf("file provider exceeded maxBytes (%d)", maxBytes)
	}

	content := string(data)
	mode := pc.Mode
	if mode == "" {
		mode = "json" // default mode per OpenClaw
	}

	switch mode {
	case "singleValue":
		// OpenClaw requires ref.id == SINGLE_VALUE_FILE_REF_ID for singleValue mode
		if ref.ID != SingleValueFileRefID {
			return "", fmt.Errorf("singleValue file provider expects ref id %q, got %q",
				SingleValueFileRefID, ref.ID)
		}
		// Entire file content is the secret; trim trailing newline
		return strings.TrimRight(content, "\r\n"), nil

	case "json":
		// Parse as JSON, then navigate via JSON Pointer (ref.ID)
		var parsed interface{}
		if err := json.Unmarshal(data, &parsed); err != nil {
			return "", fmt.Errorf("file provider JSON parse error: %w", err)
		}

		value, err := ReadJSONPointer(parsed, ref.ID)
		if err != nil {
			return "", fmt.Errorf("file provider JSON Pointer %q: %w", ref.ID, err)
		}

		// Value must be a string
		strValue, ok := value.(string)
		if !ok {
			return "", fmt.Errorf("file provider JSON Pointer %q resolved to non-string value", ref.ID)
		}
		return strValue, nil

	default:
		return "", fmt.Errorf("unsupported file provider mode %q", mode)
	}
}
