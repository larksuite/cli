// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package externalcredential

import (
	"net/url"
	"strings"

	"github.com/larksuite/cli/errs"
)

// ValidateFileURL rejects raw presigned URLs in proxy mode. It is part of the
// Extended managed file plane; Standard only checks for the system
// configuration sentinel and never parses or routes managed file handles.
func ValidateFileURL(rawURL string, cfg *Config) error {
	if cfg == nil || !cfg.Mode.IsProxy() {
		return nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid file URL returned by external credential proxy").WithCause(err)
	}
	endpoint, _ := url.Parse(cfg.RemoteEndpoint)
	if !isProxyFileURL(u, endpoint) {
		return errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"proxy mode requires an opaque file URL on the configured external credential platform")
	}
	return nil
}

func isProxyFileURL(u, endpoint *url.URL) bool {
	if u == nil || endpoint == nil || u.Scheme != endpoint.Scheme || !strings.EqualFold(u.Host, endpoint.Host) || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	// File handles are protocol identifiers, not paths. Reject alternate
	// percent-encoded spellings so the CLI and proxy classify the exact same
	// request target.
	if u.RawPath != "" || u.EscapedPath() != u.Path {
		return false
	}
	handle := strings.TrimPrefix(u.Path, "/lark-cli/v1/files/")
	if handle == u.Path || handle == "" || handle == "." || handle == ".." || len(handle) > 512 || strings.Contains(handle, "/") {
		return false
	}
	for _, r := range handle {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("-_.~", r) {
			continue
		}
		return false
	}
	return true
}
