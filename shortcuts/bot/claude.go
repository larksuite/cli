package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ClaudeResponse represents the JSON output from claude CLI
type ClaudeResponse struct {
	Result     string `json:"result"`
	SessionID  string `json:"session_id"`
	Error      string `json:"error,omitempty"`
}

// ClaudeClient wraps interaction with Claude Code CLI
type ClaudeClient struct {
	workDir        string
	timeout        time.Duration
	maxRetries     int
	skipPermissions bool
}

// ClaudeClientConfig configures a new ClaudeClient
type ClaudeClientConfig struct {
	WorkDir         string        // Working directory for claude CLI
	Timeout         time.Duration // Command timeout (default: 5 minutes)
	MaxRetries      int           // Max retry attempts (default: 3)
	SkipPermissions bool          // Add --dangerously-skip-permissions flag
}

// NewClaudeClient creates a new Claude client
func NewClaudeClient(config ClaudeClientConfig) *ClaudeClient {
	if config.Timeout == 0 {
		config.Timeout = 5 * time.Minute
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	return &ClaudeClient{
		workDir:        config.WorkDir,
		timeout:        config.Timeout,
		maxRetries:     config.MaxRetries,
		skipPermissions: config.SkipPermissions,
	}
}

// ProcessMessage sends a message to Claude and returns the response
func (c *ClaudeClient) ProcessMessage(ctx context.Context, message string, sessionID string) (*ClaudeResponse, error) {
	var lastErr error

	for attempt := 0; attempt < c.maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff before retry
			waitTime := time.Duration(attempt) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(waitTime):
			}
		}

		resp, err := c.processMessageOnce(ctx, message, sessionID)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// Check if error is retryable
		if !c.isRetryableError(err) {
			break
		}
	}

	return nil, fmt.Errorf("failed after %d attempts: %w", c.maxRetries, lastErr)
}

// processMessageOnce executes a single claude CLI call
func (c *ClaudeClient) processMessageOnce(ctx context.Context, message string, sessionID string) (*ClaudeResponse, error) {
	// Build command arguments
	args := []string{
		"-p", message,
		"--output-format", "json",
		"--add-dir", c.workDir,
	}

	if c.skipPermissions {
		args = append(args, "--dangerously-skip-permissions")
	}

	if sessionID != "" {
		args = append(args, "--resume", sessionID)
	}

	// Create command with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "claude", args...)

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute command
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("claude CLI failed: %w (stderr: %s)", err, stderr.String())
	}

	// Parse JSON response
	var response ClaudeResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return nil, fmt.Errorf("failed to parse claude response: %w (output: %s)", err, stdout.String())
	}

	// Check for error in response
	if response.Error != "" {
		return nil, fmt.Errorf("claude returned error: %s", response.Error)
	}

	return &response, nil
}

// isRetryableError determines if an error is worth retrying
func (c *ClaudeClient) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := strings.ToLower(err.Error())

	// Retry on temporary/network errors
	retryablePatterns := []string{
		"timeout",
		"connection refused",
		"temporary failure",
		"resource temporarily unavailable",
		"context deadline exceeded",
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}

	return false
}

// ValidateClaudeCLI checks if claude CLI is available and working
func ValidateClaudeCLI(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "claude", "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("claude CLI not found or not working: %w (output: %s)", err, string(output))
	}

	version := strings.TrimSpace(string(output))
	if version == "" {
		return fmt.Errorf("claude CLI returned empty version")
	}

	return nil
}
