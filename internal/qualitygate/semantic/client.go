// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package semantic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/qualitygate/facts"
)

type Client struct {
	BaseURL         string
	APIKey          string
	Model           string
	Timeout         time.Duration
	MaxTokens       int
	MaxRequestBytes int
	AllowedModels   map[string]bool
	HTTPClient      *http.Client
}

type chatRequest struct {
	Model          string    `json:"model"`
	Temperature    float64   `json:"temperature"`
	MaxTokens      int       `json:"max_tokens"`
	ResponseFormat any       `json:"response_format,omitempty"`
	Messages       []Message `json:"messages"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

const (
	defaultReviewTimeout         = 90 * time.Second
	maxReviewTimeout             = 5 * time.Minute
	maxModelErrorBodyBytes       = 512
	maxReviewAttempts            = 3
	defaultMaxReviewRequestBytes = 64 * 1024
)

var ErrReviewerRequestTooLarge = errors.New("semantic review request too large")

func FromEnvWithConfig(cfg ModelConfig) (Client, bool, error) {
	key := os.Getenv("ARK_API_KEY")
	model := os.Getenv("ARK_MODEL")
	if key == "" || model == "" {
		return Client{}, false, nil
	}
	base := os.Getenv("ARK_BASE_URL")
	if base == "" {
		base = defaultBaseURL
	}
	if !IsTrustedBaseURL(base, cfg) {
		return Client{}, false, fmt.Errorf("%w: base URL %q is not allowed", ErrReviewerConfiguration, base)
	}
	if !cfg.AllowsModel(model) {
		return Client{}, false, fmt.Errorf("%w: model %q is not allowed", ErrReviewerConfiguration, model)
	}
	allowed := make(map[string]bool, len(cfg.Allowed))
	for _, item := range cfg.Allowed {
		allowed[item] = true
	}
	normalizedBase, err := normalizeBaseURL(base)
	if err != nil {
		return Client{}, false, fmt.Errorf("%w: %v", ErrReviewerConfiguration, err)
	}
	timeout, err := timeoutFromEnv()
	if err != nil {
		return Client{}, false, fmt.Errorf("%w: %v", ErrReviewerConfiguration, err)
	}
	return Client{
		BaseURL:       normalizedBase,
		APIKey:        key,
		Model:         model,
		Timeout:       timeout,
		MaxTokens:     2048,
		AllowedModels: allowed,
	}, true, nil
}

func (c Client) Review(ctx context.Context, f facts.Facts) (Review, error) {
	timeout := c.Timeout
	if timeout == 0 {
		timeout = defaultReviewTimeout
	}
	maxTokens := c.MaxTokens
	if maxTokens == 0 {
		maxTokens = 2048
	}
	maxRequestBytes := c.MaxRequestBytes
	if maxRequestBytes == 0 {
		maxRequestBytes = defaultMaxReviewRequestBytes
	}
	if len(c.AllowedModels) > 0 && !c.AllowedModels[c.Model] {
		return Review{}, fmt.Errorf("%w: model %q is not allowed", ErrReviewerConfiguration, c.Model)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client := c.HTTPClient
	if client == nil {
		client = secureHTTPClient()
	}
	var lastErr error
	responseFormats := responseFormatsForBaseURL(c.BaseURL)
	for formatIndex, responseFormat := range responseFormats {
		reqBody := chatRequest{
			Model:          c.Model,
			Temperature:    0,
			MaxTokens:      maxTokens,
			ResponseFormat: responseFormat,
			Messages:       BuildPrompt(f),
		}
		body, err := json.Marshal(reqBody)
		if err != nil {
			return Review{}, err
		}
		if len(body) > maxRequestBytes {
			return Review{}, modelRequestTooLargeError(c.BaseURL, c.Model, responseFormat, len(body), maxRequestBytes)
		}
		for attempt := 0; attempt < maxReviewAttempts; attempt++ {
			if attempt > 0 {
				if err := sleepForRetry(ctx, attempt); err != nil {
					return Review{}, modelRetryError(c.BaseURL, c.Model, responseFormat, attempt, timeout, err)
				}
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
			if err != nil {
				return Review{}, err
			}
			req.Header.Set("Authorization", "Bearer "+c.APIKey)
			req.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				lastErr = modelRequestError(c.BaseURL, c.Model, responseFormat, attempt, err)
				if attempt == maxReviewAttempts-1 {
					return Review{}, lastErr
				}
				continue
			}
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				review, err := decodeChatReview(resp.Body)
				_ = resp.Body.Close()
				if err == nil {
					return review, nil
				}
				lastErr = modelDecodeError(c.BaseURL, c.Model, responseFormat, attempt, err)
				if attempt == maxReviewAttempts-1 {
					return Review{}, lastErr
				}
				continue
			}
			statusCode := resp.StatusCode
			lastErr = modelStatusError(c.BaseURL, c.Model, responseFormat, attempt, resp)
			_ = resp.Body.Close()
			if statusCode == http.StatusBadRequest && formatIndex < len(responseFormats)-1 {
				break
			}
			if !retryableStatus(statusCode) {
				return Review{}, lastErr
			}
		}
	}
	return Review{}, lastErr
}

func secureHTTPClient() *http.Client {
	return &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) == 0 {
			return nil
		}
		if requestOrigin(req.URL) != requestOrigin(via[0].URL) {
			return http.ErrUseLastResponse
		}
		return nil
	}}
}

func timeoutFromEnv() (time.Duration, error) {
	raw := os.Getenv("ARK_TIMEOUT_SECONDS")
	if raw == "" {
		return defaultReviewTimeout, nil
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return 0, fmt.Errorf("ARK_TIMEOUT_SECONDS must be a positive integer")
	}
	timeout := time.Duration(seconds) * time.Second
	if timeout > maxReviewTimeout {
		return 0, fmt.Errorf("ARK_TIMEOUT_SECONDS must be at most %d", int(maxReviewTimeout/time.Second))
	}
	return timeout, nil
}

func responseFormatsForBaseURL(baseURL string) []any {
	if preferUnconstrainedResponseFormat(baseURL) {
		return []any{nil, jsonSchemaResponseFormat(), jsonObjectResponseFormat()}
	}
	return []any{jsonSchemaResponseFormat(), jsonObjectResponseFormat(), nil}
}

func preferUnconstrainedResponseFormat(baseURL string) bool {
	u, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	return strings.HasSuffix(u.Path, "/api/plan/v3")
}

func requestOrigin(u *url.URL) string {
	return u.Scheme + "://" + strings.ToLower(u.Host)
}

func decodeChatReview(r io.Reader) (Review, error) {
	var response chatResponse
	if err := json.NewDecoder(io.LimitReader(r, maxModelResponseBytes)).Decode(&response); err != nil {
		return Review{}, err
	}
	if len(response.Choices) == 0 {
		return Review{}, fmt.Errorf("model response has no choices")
	}
	content := strings.TrimSpace(response.Choices[0].Message.Content)
	if content == "" {
		return Review{}, fmt.Errorf("model response content is empty")
	}
	return DecodeModelReview(strings.NewReader(content))
}

func modelRequestError(baseURL, model string, responseFormat any, attempt int, err error) error {
	return fmt.Errorf("model request failed (%s): %w", modelRequestContext(baseURL, model, responseFormat, attempt), err)
}

func modelDecodeError(baseURL, model string, responseFormat any, attempt int, err error) error {
	return fmt.Errorf("model response decode failed (%s): %w", modelRequestContext(baseURL, model, responseFormat, attempt), err)
}

func modelRetryError(baseURL, model string, responseFormat any, attempt int, timeout time.Duration, err error) error {
	return fmt.Errorf("model retry stopped (%s timeout=%s): %w", modelRequestContext(baseURL, model, responseFormat, attempt), timeout, err)
}

func modelRequestTooLargeError(baseURL, model string, responseFormat any, size, limit int) error {
	return fmt.Errorf("%w (%s bytes=%d limit=%d)", ErrReviewerRequestTooLarge, modelRequestContext(baseURL, model, responseFormat, 0), size, limit)
}

func modelStatusError(baseURL, model string, responseFormat any, attempt int, resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, maxModelErrorBodyBytes))
	body := strings.Join(strings.Fields(string(data)), " ")
	if body == "" {
		return fmt.Errorf("model endpoint returned HTTP %d (%s)", resp.StatusCode, modelRequestContext(baseURL, model, responseFormat, attempt))
	}
	return fmt.Errorf("model endpoint returned HTTP %d (%s): %s", resp.StatusCode, modelRequestContext(baseURL, model, responseFormat, attempt), body)
}

func modelRequestContext(baseURL, model string, responseFormat any, attempt int) string {
	return fmt.Sprintf("endpoint=%s/chat/completions model=%s response_format=%s attempt=%d/%d",
		baseURL,
		model,
		responseFormatName(responseFormat),
		attempt+1,
		maxReviewAttempts,
	)
}

func responseFormatName(responseFormat any) string {
	if responseFormat == nil {
		return "none"
	}
	data, err := json.Marshal(responseFormat)
	if err != nil {
		return "unknown"
	}
	var typed struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &typed); err != nil {
		return "unknown"
	}
	if typed.Type == "" {
		return "unknown"
	}
	return typed.Type
}

func retryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

func sleepForRetry(ctx context.Context, attempt int) error {
	delay := time.Duration(10*(1<<(attempt-1))) * time.Millisecond
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func jsonSchemaResponseFormat() map[string]any {
	return map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   "quality_gate_semantic_review",
			"strict": true,
			"schema": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"verdict", "findings"},
				"properties": map[string]any{
					"verdict": map[string]any{
						"type": "string",
						"enum": []string{"pass", "warn"},
					},
					"findings": map[string]any{
						"type":     "array",
						"maxItems": 20,
						"items": map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"category", "severity", "evidence", "message", "suggested_action"},
							"properties": map[string]any{
								"category": map[string]any{
									"type": "string",
									"enum": []string{"error_hint", "default_output", "naming", "skill_quality", "public_content_leakage"},
								},
								"severity": map[string]any{
									"type": "string",
									"enum": []string{"minor", "major", "critical"},
								},
								"evidence": map[string]any{
									"type":     "array",
									"minItems": 1,
									"maxItems": 20,
									"items": map[string]any{
										"type":      "string",
										"maxLength": 100,
									},
								},
								"message": map[string]any{
									"type":      "string",
									"maxLength": 500,
								},
								"suggested_action": map[string]any{
									"type":      "string",
									"maxLength": 500,
								},
							},
						},
					},
				},
			},
		},
	}
}

func jsonObjectResponseFormat() map[string]string {
	return map[string]string{"type": "json_object"}
}
