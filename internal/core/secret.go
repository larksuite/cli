// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"encoding/json"
	"fmt"
)

// ---------------------------------------------------------------------------
// SecretRef — external secret reference
// ---------------------------------------------------------------------------

// SecretRef references a secret stored externally.
type SecretRef struct {
	Source   string `json:"source"`             // "file" | "keychain" | "encrypted_file"
	Provider string `json:"provider,omitempty"` // optional, reserved
	ID       string `json:"id"`                 // env var name / file path / command / keychain key
}

// ---------------------------------------------------------------------------
// SecretInput — union type: plain string or SecretRef
// ---------------------------------------------------------------------------

// SecretInput represents a secret value: either a plain string or a SecretRef object.
type SecretInput struct {
	Plain string     // non-empty for plain string values
	Ref   *SecretRef // non-nil for SecretRef values
}

// PlainSecret creates a SecretInput from a plain string.
func PlainSecret(s string) SecretInput {
	return SecretInput{Plain: s}
}

// IsZero returns true if the SecretInput has no value.
func (s SecretInput) IsZero() bool {
	return s.Plain == "" && s.Ref == nil
}

// IsSecretRef returns true if this is a SecretRef object (env/file/keychain).
func (s SecretInput) IsSecretRef() bool {
	return s.Ref != nil
}

// IsPlain returns true if this is a plain text string (not a SecretRef).
func (s SecretInput) IsPlain() bool {
	return s.Ref == nil
}

// MarshalJSON serializes SecretInput: plain string → JSON string, SecretRef → JSON object.
func (s SecretInput) MarshalJSON() ([]byte, error) {
	if s.Ref != nil {
		return json.Marshal(s.Ref)
	}
	return json.Marshal(s.Plain)
}

// UnmarshalJSON deserializes SecretInput from either a JSON string or a SecretRef object.
func (s *SecretInput) UnmarshalJSON(data []byte) error {
	// Try string first
	var plain string
	if err := json.Unmarshal(data, &plain); err == nil {
		s.Plain = plain
		s.Ref = nil
		return nil
	}
	// Try SecretRef object
	var ref SecretRef
	if err := json.Unmarshal(data, &ref); err == nil && isValidSource(ref.Source) && ref.ID != "" {
		s.Ref = &ref
		s.Plain = ""
		return nil
	}
	return fmt.Errorf("appSecret must be a string or {source, id} object")
}

// ValidSecretSources is the set of recognized SecretRef sources.
var ValidSecretSources = map[string]bool{
	"file": true, "keychain": true, "encrypted_file": true,
}

func isValidSource(source string) bool {
	return ValidSecretSources[source]
}
