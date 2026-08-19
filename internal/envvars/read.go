// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package envvars

import (
	"net/http"
	"os"
	"strings"
	"unicode"
)

const (
	agentNameMaxLen  = 128
	agentTraceMaxLen = 1024
)

func AgentName() string {
	return sanitizeSingleLine(os.Getenv(CliAgentName), agentNameMaxLen)
}

func AgentTrace() string {
	return sanitizeSingleLine(os.Getenv(CliAgentTrace), agentTraceMaxLen)
}

func ExtraHeaders() http.Header {
	headers := make(http.Header)
	for _, item := range strings.Split(os.Getenv(CliExtraHeaders), ";") {
		name, value, ok := strings.Cut(item, ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		value = sanitizeSingleLine(value, agentNameMaxLen)
		if name == "" || value == "" || !validHeaderName(name) {
			continue
		}
		headers.Set(name, value)
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
}

func sanitizeSingleLine(raw string, maxLen int) string {
	v := strings.TrimSpace(raw)
	if v == "" || len(v) > maxLen {
		return ""
	}
	for _, r := range v {
		if unicode.IsControl(r) {
			return ""
		}
	}
	return v
}

func validHeaderName(name string) bool {
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-':
		default:
			return false
		}
	}
	return true
}
