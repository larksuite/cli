// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"bytes"
	"testing"

	"github.com/larksuite/cli/internal/core"
)

// TestDebugfWhenEnabled verifies that when DebugEnabled=true,
// the message is output to stderr with [DEBUG] prefix.
func TestDebugfWhenEnabled(t *testing.T) {
	f, _, stderrBuf, _ := TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})
	f.DebugEnabled = true

	f.Debugf("test message")

	output := stderrBuf.String()
	if !contains(output, "[DEBUG]") {
		t.Errorf("output should contain [DEBUG] prefix, got: %q", output)
	}
	if !contains(output, "test message") {
		t.Errorf("output should contain message, got: %q", output)
	}
}

// TestDebugfWhenDisabled verifies that when DebugEnabled=false,
// nothing is output to stderr.
func TestDebugfWhenDisabled(t *testing.T) {
	f, _, stderrBuf, _ := TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})
	f.DebugEnabled = false

	f.Debugf("test message")

	output := stderrBuf.String()
	if output != "" {
		t.Errorf("output should be empty when debug disabled, got: %q", output)
	}
}

// TestDebugfWithNilIOStreams verifies that when IOStreams=nil,
// the method doesn't panic.
func TestDebugfWithNilIOStreams(t *testing.T) {
	f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})
	f.DebugEnabled = true
	f.IOStreams = nil

	// Should not panic
	f.Debugf("test message")
}

// TestDebugfWithNilFactory verifies that when Factory=nil,
// the method doesn't panic.
func TestDebugfWithNilFactory(t *testing.T) {
	var f *Factory

	// Should not panic
	f.Debugf("test message")
}

// TestDebugfFormat verifies that message formatting is correct.
func TestDebugfFormat(t *testing.T) {
	f, _, stderrBuf, _ := TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})
	f.DebugEnabled = true

	f.Debugf("test %s %d", "message", 42)

	output := stderrBuf.String()
	if !contains(output, "[DEBUG] test message 42") {
		t.Errorf("output should contain formatted message, got: %q", output)
	}
}

// TestDebugfWithNilErrOut verifies that when IOStreams.ErrOut=nil,
// the method doesn't panic.
func TestDebugfWithNilErrOut(t *testing.T) {
	f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})
	f.DebugEnabled = true
	f.IOStreams.ErrOut = nil

	// Should not panic
	f.Debugf("test message")
}

// contains is a helper function to check if a string contains a substring.
func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}
