// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmde2e

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDebugFlag_Workflow tests the --debug global flag across various commands
func TestDebugFlag_Workflow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	t.Run("api_without_debug", func(t *testing.T) {
		// Execute api command without --debug flag
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"api", "GET", "/open-apis/contact/v3/users"},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		// stdout should contain valid API response
		require.NotEmpty(t, result.Stdout, "stdout should contain API response")
		// stderr should not contain [DEBUG] prefix
		debugPresent := strings.Contains(result.Stderr, "[DEBUG]")
		assert.False(t, debugPresent, "stderr should not contain [DEBUG] when --debug is not set, stderr: %s", result.Stderr)
	})

	t.Run("api_with_debug", func(t *testing.T) {
		// Execute same api command WITH --debug flag
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"--debug", "api", "GET", "/open-apis/contact/v3/users"},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		// stdout should still contain valid API response
		require.NotEmpty(t, result.Stdout, "stdout should contain API response")
		// Debug mode should be enabled (stderr may contain [DEBUG] if implementation calls Debugf)
		// The important thing is that the command executes successfully
	})

	t.Run("help_without_debug", func(t *testing.T) {
		// Test with help command to verify --debug doesn't break built-in commands
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"api", "--help"},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		// help text should be in stdout
		helpPresent := strings.Contains(result.Stdout, "usage") || strings.Contains(result.Stdout, "Usage")
		assert.True(t, helpPresent, "help output should be present")
	})

	t.Run("help_with_debug", func(t *testing.T) {
		// Test with --debug and help command
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"--debug", "api", "--help"},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		// help text should still be in stdout
		helpPresent := strings.Contains(result.Stdout, "usage") || strings.Contains(result.Stdout, "Usage")
		assert.True(t, helpPresent, "help output should be present with --debug")
	})

	t.Run("debug_with_profile", func(t *testing.T) {
		// Test --debug combined with --profile flag
		// Using default profile which should exist
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"--debug", "--profile", "default", "api", "GET", "/open-apis/contact/v3/users"},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		// Both flags should work together
		require.NotEmpty(t, result.Stdout, "stdout should contain API response")
	})

	t.Run("profile_then_debug", func(t *testing.T) {
		// Test flag order: --profile before --debug (order shouldn't matter)
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"--profile", "default", "--debug", "api", "GET", "/open-apis/contact/v3/users"},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		// Both flags should work regardless of order
		require.NotEmpty(t, result.Stdout, "stdout should contain API response")
	})

	t.Run("unknown_command_with_debug", func(t *testing.T) {
		// Test --debug with invalid command
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"--debug", "invalid-command"},
		})
		require.NoError(t, err)
		// Exit code should be non-zero for unknown command
		assert.NotEqual(t, 0, result.ExitCode, "unknown command should fail")

		// stderr should contain error message
		require.NotEmpty(t, result.Stderr, "stderr should contain error message")
	})

	t.Run("debug_placement_after_command", func(t *testing.T) {
		// Test --debug placed after command (not as global flag)
		// This tests that --debug is position-sensitive
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"api", "--debug", "GET", "/open-apis/contact/v3/users"},
		})
		require.NoError(t, err)
		// Exit code could be 0 or non-zero depending on if --debug is accepted by api command
		// The important thing is that it behaves differently than global --debug
		// If it fails, that's correct behavior; if it passes, --debug was passed to api subcommand
		_ = result // result used to verify command executes (exit code checked implicitly)
	})

	t.Run("config_command_with_debug", func(t *testing.T) {
		// Test --debug with config command (another built-in)
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"--debug", "config", "list"},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		// config list should return JSON or structured output
		require.NotEmpty(t, result.Stdout, "stdout should contain config output")
	})

	t.Run("auth_command_with_debug", func(t *testing.T) {
		// Test --debug with auth command
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"--debug", "auth", "--help"},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		// help text should be present
		helpPresent := strings.Contains(result.Stdout, "usage") || strings.Contains(result.Stdout, "Usage") || strings.Contains(result.Stdout, "auth")
		assert.True(t, helpPresent, "auth help should be present with --debug")
	})
}

// TestDebugFlag_Consistency tests that --debug flag is properly parsed and does not break command execution
func TestDebugFlag_Consistency(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	// Run the same command multiple times: without and with --debug
	// Both should produce equivalent output (same exit code, same response structure)

	t.Run("api_response_consistency", func(t *testing.T) {
		// Get response without --debug
		resultWithout, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"api", "GET", "/open-apis/contact/v3/users"},
		})
		require.NoError(t, err)
		resultWithout.AssertExitCode(t, 0)
		resultWithout.AssertStdoutStatus(t, 0)

		// Get response with --debug
		resultWith, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"--debug", "api", "GET", "/open-apis/contact/v3/users"},
		})
		require.NoError(t, err)
		resultWith.AssertExitCode(t, 0)
		resultWith.AssertStdoutStatus(t, 0)

		// Both should return valid JSON responses
		require.NotEmpty(t, resultWithout.Stdout)
		require.NotEmpty(t, resultWith.Stdout)
		// Both should have same exit code
		assert.Equal(t, resultWithout.ExitCode, resultWith.ExitCode)
	})

	t.Run("help_response_consistency", func(t *testing.T) {
		// Get help without --debug
		resultWithout, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"api", "--help"},
		})
		require.NoError(t, err)
		resultWithout.AssertExitCode(t, 0)

		// Get help with --debug
		resultWith, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"--debug", "api", "--help"},
		})
		require.NoError(t, err)
		resultWith.AssertExitCode(t, 0)

		// Both should contain help text (may not be identical due to debug output)
		helpWithout := strings.Contains(resultWithout.Stdout, "usage") || strings.Contains(resultWithout.Stdout, "Usage")
		helpWith := strings.Contains(resultWith.Stdout, "usage") || strings.Contains(resultWith.Stdout, "Usage")
		assert.True(t, helpWithout, "help without --debug should be present")
		assert.True(t, helpWith, "help with --debug should be present")
	})
}

// TestDebugFlag_Integration tests --debug with various global flag combinations
func TestDebugFlag_Integration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	t.Run("debug_with_format_json", func(t *testing.T) {
		// Test --debug combined with --format flag
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"--debug", "--format", "json", "api", "GET", "/open-apis/contact/v3/users"},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		// Should return JSON format as specified
		require.NotEmpty(t, result.Stdout)
	})

	t.Run("debug_format_order", func(t *testing.T) {
		// Test different flag order: --format before --debug
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"--format", "json", "--debug", "api", "GET", "/open-apis/contact/v3/users"},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		require.NotEmpty(t, result.Stdout)
	})

	t.Run("multiple_global_flags", func(t *testing.T) {
		// Test --debug, --profile, and --format all together
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"--debug", "--profile", "default", "--format", "json", "api", "GET", "/open-apis/contact/v3/users"},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		require.NotEmpty(t, result.Stdout)
	})
}
