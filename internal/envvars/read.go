// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package envvars

import (
	"os"
	"strings"
	"unicode"
)

const (
	agentNameMaxLen  = 128
	agentTraceMaxLen = 1024
	ttEnvMaxLen      = 256
)

func AgentName() string {
	return sanitizeSingleLine(os.Getenv(CliAgentName), agentNameMaxLen)
}

func AgentTrace() string {
	return sanitizeSingleLine(os.Getenv(CliAgentTrace), agentTraceMaxLen)
}

// TTEnv returns the sanitized value for the x-tt-env routing header
// (e.g. "ppe_whiteboard_mindnote"). Empty when unset or invalid.
func TTEnv() string {
	return sanitizeSingleLine(os.Getenv(CliTTEnv), ttEnvMaxLen)
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
