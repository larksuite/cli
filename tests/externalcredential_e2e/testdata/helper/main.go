// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Reference credential-process implementation used by the repository-local
// external credential protocol E2E. It intentionally has no configuration or
// ambient-environment dependency.
package main

import (
	"encoding/json"
	"flag"
	"os"
	"time"
)

type request struct {
	Version        int    `json:"version"`
	Mode           string `json:"mode"`
	CredentialType string `json:"credential_type"`
	AppID          string `json:"app_id"`
	Identity       string `json:"identity"`
	RemoteEndpoint string `json:"remote_endpoint"`
}

func main() {
	capture := flag.String("capture", "", "append non-secret protocol requests to this file")
	flag.Parse()

	var req request
	if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
		os.Exit(2)
	}
	if *capture != "" {
		file, err := os.OpenFile(*capture, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			os.Exit(2)
		}
		if err := json.NewEncoder(file).Encode(req); err != nil {
			_ = file.Close()
			os.Exit(2)
		}
		if err := file.Close(); err != nil {
			os.Exit(2)
		}
	}

	expiresAt := time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)
	var credential map[string]any
	switch req.Mode {
	case "direct":
		tokenType := "uat"
		if req.Identity == "bot" {
			tokenType = "tat"
		}
		credential = map[string]any{
			"token_type":   tokenType,
			"access_token": "direct-e2e-token",
			"expires_at":   expiresAt,
			"scopes":       []string{"im:message", "contact:user.base:readonly"},
		}
	case "credential_proxy":
		credential = map[string]any{
			"scheme":       "bearer",
			"access_token": "proxy-e2e-token",
			"expires_at":   expiresAt,
		}
	default:
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"version": 1,
			"error": map[string]any{
				"code":    "invalid_request",
				"message": "unsupported reference-helper mode",
			},
		})
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(map[string]any{
		"version":    1,
		"credential": credential,
	}); err != nil {
		os.Exit(2)
	}
}
