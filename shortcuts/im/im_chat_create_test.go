// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

const chatCreateMissingIdempotencyKeyHint = "Generate one UUID with a library or tool (max 50 chars), then pass its literal value; reuse it with unchanged parameters for retries within 10 hours."

func newChatCreateRuntime(t *testing.T, idempotencyKey string, rt http.RoundTripper) *common.RuntimeContext {
	t.Helper()

	runtime := newBotShortcutRuntime(t, rt)
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("name", "", "")
	cmd.Flags().String("description", "", "")
	cmd.Flags().String("users", "", "")
	cmd.Flags().String("bots", "", "")
	cmd.Flags().String("owner", "", "")
	cmd.Flags().String("type", "private", "")
	cmd.Flags().String("chat-mode", "group", "")
	cmd.Flags().String("idempotency-key", "", "")
	cmd.Flags().Bool("set-bot-manager", false, "")
	if err := cmd.Flags().Set("name", "Project Room"); err != nil {
		t.Fatalf("Flags().Set(name) error = %v", err)
	}
	if err := cmd.Flags().Set("idempotency-key", idempotencyKey); err != nil {
		t.Fatalf("Flags().Set(idempotency-key) error = %v", err)
	}
	runtime.Cmd = cmd
	return runtime
}

func TestChatCreateIdempotencyKeyValidation(t *testing.T) {
	t.Run("flag is registered by shortcut metadata", func(t *testing.T) {
		for _, flag := range ImChatCreate.Flags {
			if flag.Name == "idempotency-key" {
				return
			}
		}
		t.Fatal("ImChatCreate.Flags does not contain idempotency-key")
	})

	t.Run("missing", func(t *testing.T) {
		assertChatCreateMissingIdempotencyKey(t, "")
	})

	t.Run("blank", func(t *testing.T) {
		assertChatCreateMissingIdempotencyKey(t, " \t\n ")
	})

	t.Run("long", func(t *testing.T) {
		var requestCount atomic.Int32
		runtime := newChatCreateRuntime(t, strings.Repeat("界", 51), shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestCount.Add(1)
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}))

		err := ImChatCreate.Validate(context.Background(), runtime)
		var validationErr *errs.ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("ImChatCreate.Validate() error type = %T, want *errs.ValidationError", err)
		}
		if validationErr.Category != errs.CategoryValidation ||
			validationErr.Subtype != errs.SubtypeInvalidArgument ||
			validationErr.Message != "--idempotency-key exceeds the maximum of 50 characters (got 51)" ||
			validationErr.Param != "--idempotency-key" {
			t.Fatalf("ImChatCreate.Validate() error = %#v", validationErr)
		}
		if requestCount.Load() != 0 {
			t.Fatalf("request count = %d, want 0", requestCount.Load())
		}
	})

	t.Run("valid 50 rune literal", func(t *testing.T) {
		runtime := newChatCreateRuntime(t, strings.Repeat("界", 50), shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}))
		if err := ImChatCreate.Validate(context.Background(), runtime); err != nil {
			t.Fatalf("ImChatCreate.Validate() error = %v", err)
		}
	})
}

func assertChatCreateMissingIdempotencyKey(t *testing.T, key string) {
	t.Helper()

	var requestCount atomic.Int32
	runtime := newChatCreateRuntime(t, key, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		requestCount.Add(1)
		return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
	}))

	err := ImChatCreate.Validate(context.Background(), runtime)
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("ImChatCreate.Validate() error type = %T, want *errs.ValidationError", err)
	}
	if validationErr.Category != errs.CategoryValidation ||
		validationErr.Subtype != errs.SubtypeInvalidArgument ||
		validationErr.Message != "--idempotency-key is required for idempotent retries that prevent duplicate groups" ||
		validationErr.Param != "--idempotency-key" ||
		validationErr.Hint != chatCreateMissingIdempotencyKeyHint {
		t.Fatalf("ImChatCreate.Validate() error = %#v", validationErr)
	}
	if requestCount.Load() != 0 {
		t.Fatalf("request count = %d, want 0", requestCount.Load())
	}
}

func TestChatCreateDryRunUsesOriginalIdempotencyKeyAsUUIDQueryOnly(t *testing.T) {
	const key = " job-create-001 "
	runtime := newChatCreateRuntime(t, key, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
	}))

	raw, err := json.Marshal(ImChatCreate.DryRun(context.Background(), runtime))
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var preview struct {
		API []struct {
			Params map[string]interface{} `json:"params"`
			Body   map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(raw, &preview); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(preview.API) != 1 {
		t.Fatalf("dry-run API calls = %d, want 1", len(preview.API))
	}
	if got := preview.API[0].Params["uuid"]; got != key {
		t.Fatalf("dry-run uuid = %#v, want %#v", got, key)
	}
	if _, ok := preview.API[0].Body["uuid"]; ok {
		t.Fatalf("dry-run body contains uuid: %#v", preview.API[0].Body)
	}
}

func TestChatCreateExecuteUsesOriginalIdempotencyKeyAsUUIDQueryOnly(t *testing.T) {
	const key = " job-create-002 "
	var createRequests atomic.Int32
	runtime := newChatCreateRuntime(t, key, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/open-apis/im/v1/chats":
			createRequests.Add(1)
			if got := req.URL.Query().Get("uuid"); got != key {
				t.Errorf("create query uuid = %#v, want %#v", got, key)
			}
			rawBody, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("io.ReadAll() error = %v", err)
			}
			var body map[string]interface{}
			if err := json.Unmarshal(rawBody, &body); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if _, ok := body["uuid"]; ok {
				t.Errorf("create body contains uuid: %#v", body)
			}
			return shortcutJSONResponse(http.StatusOK, map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"chat_id":   "oc_created",
					"name":      "Project Room",
					"chat_type": "private",
					"owner_id":  "ou_owner",
					"external":  false,
				},
			}), nil
		case "/open-apis/im/v1/chats/oc_created/link":
			return shortcutJSONResponse(http.StatusOK, map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"share_link": "https://example.invalid/chat"},
			}), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	if err := ImChatCreate.Validate(context.Background(), runtime); err != nil {
		t.Fatalf("ImChatCreate.Validate() error = %v", err)
	}
	if err := ImChatCreate.Execute(context.Background(), runtime); err != nil {
		t.Fatalf("ImChatCreate.Execute() error = %v", err)
	}
	if createRequests.Load() != 1 {
		t.Fatalf("create request count = %d, want 1", createRequests.Load())
	}
}
