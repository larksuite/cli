// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package gitcred

import "time"

const (
	CurrentCredentialVersion = 1
	MetadataFilename         = "git.json"

	// KeychainService intentionally reuses the CLI-wide internal keychain
	// service, so Git PAT .enc files stay under Application Support/lark-cli.
	KeychainService = "lark-cli"

	StatusPending   = "pending"
	StatusConfirmed = "confirmed"

	ListStatusValid         = "valid"
	ListStatusExpired       = "expired"
	ListStatusInvalidated   = "invalidated"
	ListStatusMissingSecret = "missing_secret"
	ListStatusIncomplete    = "incomplete"

	refreshBeforeExpiry = 10 * time.Minute
	eraseCooldown       = 5 * time.Minute
)

// CredentialFile is the app-scoped non-secret metadata persisted under the
// app storage directory.
type CredentialFile struct {
	Version int `json:"version"`
	CredentialRecord
}

// CredentialRecord points to the keychain-stored PAT without storing the PAT
// plaintext in metadata.
type CredentialRecord struct {
	AppID         string `json:"app_id"`
	GitHTTPURL    string `json:"git_http_url"`
	Profile       string `json:"profile"`
	ProfileAppID  string `json:"profile_app_id"`
	UserOpenID    string `json:"user_open_id"`
	Username      string `json:"username"`
	PATRef        string `json:"pat_ref"`
	Status        string `json:"status"`
	ExpiresAt     int64  `json:"expires_at"`
	UpdatedAt     int64  `json:"updated_at"`
	LastEraseAt   int64  `json:"last_erase_at,omitempty"`
	InvalidatedAt int64  `json:"invalidated_at,omitempty"`
}

type IssuedCredential struct {
	AppID      string
	GitHTTPURL string
	Username   string
	PAT        string
	ExpiresAt  int64
}

type InitResult struct {
	AppID         string
	GitHTTPURL    string
	Refreshed     bool
	ConfigWarning string
}

type RemoveResult struct {
	AppID         string
	Removed       bool
	Records       []CredentialRecord
	ConfigWarning string
}

type ListResult struct {
	Records []ListRecord
}

type ListRecord struct {
	AppID         string
	GitHTTPURL    string
	Status        string
	ExpiresAt     int64
	UpdatedAt     int64
	Profile       string
	ProfileAppID  string
	UserOpenID    string
	Expired       bool
	InvalidatedAt int64
}

type CredentialInput struct {
	Protocol string
	Host     string
	Path     string
}

type ProfileContext struct {
	Profile      string
	ProfileAppID string
	UserOpenID   string
}
