// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/keychain"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/httpmock"
)

// TestVerifyUserToken_TransportError verifies handling of underlying transport errors.
func TestVerifyUserToken_TransportError(t *testing.T) {
	reg := &httpmock.Registry{}
	// Register no stubs — any request will fail with "no stub" error
	sdk := lark.NewClient("test-app", "test-secret",
		lark.WithLogLevel(larkcore.LogLevelError),
		lark.WithHttpClient(httpmock.NewClient(reg)),
	)

	err := VerifyUserToken(context.Background(), sdk, "test-token")
	if err == nil {
		t.Fatal("expected error from transport failure, got nil")
	}
}

// TestVerifyUserToken validates normal and error response paths of the user token validation.
func TestVerifyUserToken(t *testing.T) {
	tests := []struct {
		name      string
		body      interface{}
		wantErr   bool
		errSubstr string
		wantLog   bool
	}{
		{
			name:    "success",
			body:    map[string]interface{}{"code": 0, "msg": "ok"},
			wantErr: false,
			wantLog: true,
		},
		{
			name:      "token invalid",
			body:      map[string]interface{}{"code": 99991668, "msg": "invalid token"},
			wantErr:   true,
			errSubstr: "[99991668]",
			wantLog:   true,
		},
		{
			name:      "non-JSON response",
			body:      "not json",
			wantErr:   true,
			errSubstr: "invalid character",
			wantLog:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			t.Cleanup(func() { reg.Verify(t) })

			reg.Register(&httpmock.Stub{
				Method: "GET",
				URL:    PathUserInfoV1,
				Body:   tt.body,
				Headers: http.Header{
					"Content-Type": []string{"application/json"},
					"X-Tt-Logid":   []string{"verify-log-id"},
				},
			})

			sdk := lark.NewClient("test-app", "test-secret",
				lark.WithLogLevel(larkcore.LogLevelError),
				lark.WithHttpClient(httpmock.NewClient(reg)),
			)

			var buf bytes.Buffer
			restore := keychain.SetAuthLogHooksForTest(log.New(&buf, "", 0), func() time.Time {
				return time.Date(2026, 4, 2, 3, 4, 5, 0, time.UTC)
			}, func() []string {
				return []string{"lark-cli", "auth", "status"}
			})
			t.Cleanup(restore)

			err := VerifyUserToken(context.Background(), sdk, "test-token")
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errSubstr)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
			got := buf.String()
			if tt.wantLog {
				if !strings.Contains(got, "path="+PathUserInfoV1) {
					t.Fatalf("expected path in log, got %q", got)
				}
				if !strings.Contains(got, "status=200") {
					t.Fatalf("expected status=200 in log, got %q", got)
				}
				if !strings.Contains(got, "x-tt-logid=verify-log-id") {
					t.Fatalf("expected x-tt-logid in log, got %q", got)
				}
				if !strings.Contains(got, "cmdline=lark-cli auth status") {
					t.Fatalf("expected cmdline in log, got %q", got)
				}
			} else if got != "" {
				t.Fatalf("expected no log output, got %q", got)
			}
		})
	}
}
