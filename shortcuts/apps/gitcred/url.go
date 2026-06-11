// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package gitcred

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"path"
	"strings"

	"github.com/larksuite/cli/errs"
)

func NormalizeGitHTTPURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "git_http_url is empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid git_http_url %q: %s", raw, err).WithCause(err)
	}
	return normalizeParsedURL(u)
}

func NormalizeCredentialInput(input CredentialInput) (string, error) {
	protocol := strings.TrimSpace(input.Protocol)
	host := strings.TrimSpace(input.Host)
	if protocol == "" || host == "" {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "git credential input must include protocol and host")
	}
	u := &url.URL{
		Scheme: protocol,
		Host:   host,
		Path:   input.Path,
	}
	return normalizeParsedURL(u)
}

func normalizeParsedURL(u *url.URL) (string, error) {
	scheme := strings.ToLower(strings.TrimSpace(u.Scheme))
	if scheme != "http" && scheme != "https" {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "git credential only supports http/https URLs")
	}
	host := normalizeHost(scheme, u.Host)
	if host == "" {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "git_http_url host is empty")
	}
	cleanPath := cleanURLPath(u.EscapedPath())
	normalized := (&url.URL{Scheme: scheme, Host: host, Path: cleanPath}).String()
	if normalized != scheme+"://"+host+"/" {
		normalized = strings.TrimRight(normalized, "/")
	}
	return normalized, nil
}

func normalizeHost(scheme, host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return ""
	}
	name, port, err := net.SplitHostPort(host)
	if err == nil {
		if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
			return normalizeHostname(name)
		}
		return net.JoinHostPort(strings.ToLower(name), port)
	}
	return normalizeHostname(host)
}

func normalizeHostname(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		name := strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
		if ip := net.ParseIP(name); ip != nil && ip.To4() == nil {
			return joinHostWithoutPort(name)
		}
		return host
	}
	if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
		return joinHostWithoutPort(host)
	}
	return host
}

func joinHostWithoutPort(host string) string {
	joined := net.JoinHostPort(host, "")
	return strings.TrimSuffix(joined, ":")
}

func cleanURLPath(rawPath string) string {
	if rawPath == "" {
		return "/"
	}
	decoded, err := url.PathUnescape(rawPath)
	if err != nil {
		decoded = rawPath
	}
	if !strings.HasPrefix(decoded, "/") {
		decoded = "/" + decoded
	}
	return path.Clean(decoded)
}

func BuildPATRef(profile ProfileContext, appID string) string {
	seed := fmt.Sprintf("%s\x00%s", profile.UserOpenID, appID)
	sum := sha256.Sum256([]byte(seed))
	return "app-git-pat:" + hex.EncodeToString(sum[:16])
}
