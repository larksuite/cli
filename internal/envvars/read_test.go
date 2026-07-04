// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package envvars

import (
	"strings"
	"testing"
)

func TestAgentName_EmptyWhenEnvUnset(t *testing.T) {
	t.Setenv(CliAgentName, "")
	if got := AgentName(); got != "" {
		t.Fatalf("AgentName() = %q, want empty when env unset", got)
	}
}

func TestAgentName_ReturnsCleanValue(t *testing.T) {
	t.Setenv(CliAgentName, "claude-code")
	if got := AgentName(); got != "claude-code" {
		t.Fatalf("AgentName() = %q, want %q", got, "claude-code")
	}
}

func TestAgentName_TrimsWhitespace(t *testing.T) {
	t.Setenv(CliAgentName, "  cursor  ")
	if got := AgentName(); got != "cursor" {
		t.Fatalf("AgentName() = %q, want %q (whitespace trimmed)", got, "cursor")
	}
}

func TestAgentName_RejectsCRLFInjection(t *testing.T) {
	t.Setenv(CliAgentName, "agent\r\nX-Evil: attack")
	if got := AgentName(); got != "" {
		t.Fatalf("AgentName() = %q, want empty for CR/LF value", got)
	}
}

func TestAgentName_RejectsControlChar(t *testing.T) {
	t.Setenv(CliAgentName, "agent\x01injected")
	if got := AgentName(); got != "" {
		t.Fatalf("AgentName() = %q, want empty for control char value", got)
	}
}

func TestAgentName_RejectsOverlongValue(t *testing.T) {
	longVal := strings.Repeat("a", agentNameMaxLen+1)
	t.Setenv(CliAgentName, longVal)
	if got := AgentName(); got != "" {
		t.Fatalf("AgentName() returned non-empty for %d-byte value (max %d)", len(longVal), agentNameMaxLen)
	}
}
