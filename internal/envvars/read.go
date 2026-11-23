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
)

var agentNameEnvHints = [...]struct{ env, name string }{
	{"CLAUDECODE", "claudecode"},
	{"WORKBUDDY_APP_NAME", "workbuddy"},
	{"QODERWORK_EXECUTION_ENV", "qwenwork"},
	{"HERMES_HOME", "hermes"},
	{"OPENCLAW_SHELL", "openclaw"},
	{"CODEX_SHELL", "codex"},
	{"CODEX_VERSION", "codex"},
	{"CODEX_PERMISSION_PROFILE", "codex"},
}

// AgentName resolves explicit or environment-provided agent metadata.
func AgentName() string {
	if raw := os.Getenv(CliAgentName); raw != "" {
		return sanitizeSingleLine(raw, agentNameMaxLen)
	}
	for _, hint := range agentNameEnvHints {
		if os.Getenv(hint.env) != "" {
			return hint.name
		}
	}
	if ideType := os.Getenv("CLIENT_INFO_IDE_TYPE"); ideType != "" {
		return sanitizeSingleLine(ideType, agentNameMaxLen)
	}
	return ""
}

func AgentTrace() string {
	return sanitizeSingleLine(os.Getenv(CliAgentTrace), agentTraceMaxLen)
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
