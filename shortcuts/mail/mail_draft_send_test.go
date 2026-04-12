// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestMailSendWithScheduledTime(t *testing.T) {
	futureTS := time.Now().Unix() + 10*60 // 10 minutes from now

	tests := []struct {
		name         string
		sendTime     string
		wantSchedule bool
	}{
		{
			name:         "immediate send (no send-time)",
			sendTime:     "",
			wantSchedule: false,
		},
		{
			name:         "scheduled send with send-time",
			sendTime:     fmt.Sprintf("%d", futureTS),
			wantSchedule: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, stdout, _, reg := mailShortcutTestFactory(t)

			// Stub profile
			reg.Register(&httpmock.Stub{
				URL: "/user_mailboxes/me/profile",
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{
						"primary_email_address": "me@example.com",
					},
				},
			})

			// Stub draft create
			reg.Register(&httpmock.Stub{
				Method: "POST",
				URL:    "/user_mailboxes/me/drafts",
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{
						"draft_id": "draft_test_001",
					},
				},
			})

			// Stub draft send — capture request body to verify send_time
			sendStub := &httpmock.Stub{
				Method: "POST",
				URL:    "/user_mailboxes/me/drafts/draft_test_001/send",
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{
						"message_id": "msg_test_001",
						"thread_id":  "thread_test_001",
					},
				},
			}
			reg.Register(sendStub)

			args := []string{
				"+send",
				"--to", "recipient@example.com",
				"--subject", "Test Email",
				"--body", "Hello, world!",
				"--confirm-send",
			}
			if tc.sendTime != "" {
				args = append(args, "--send-time", tc.sendTime)
			}

			err := runMountedMailShortcut(t, MailSend, args, f, stdout)
			if err != nil {
				t.Fatalf("runMountedMailShortcut() error = %v", err)
			}

			// Verify the captured request body
			if tc.wantSchedule {
				if len(sendStub.CapturedBody) == 0 {
					t.Fatal("expected non-empty request body for scheduled send")
				}
				var reqBody map[string]interface{}
				if err := json.Unmarshal(sendStub.CapturedBody, &reqBody); err != nil {
					t.Fatalf("failed to unmarshal captured body: %v", err)
				}
				if _, ok := reqBody["send_time"]; !ok {
					t.Fatal("expected send_time in request body for scheduled send")
				}
				if reqBody["send_time"] != fmt.Sprintf("%d", futureTS) {
					t.Fatalf("send_time mismatch: want %d, got %v", futureTS, reqBody["send_time"])
				}
			} else {
				// Immediate send: body should be nil or empty
				if len(sendStub.CapturedBody) > 0 {
					var reqBody map[string]interface{}
					if err := json.Unmarshal(sendStub.CapturedBody, &reqBody); err == nil {
						if _, ok := reqBody["send_time"]; ok {
							t.Fatal("unexpected send_time in request body for immediate send")
						}
					}
				}
			}

			// Verify output
			data := decodeShortcutEnvelopeData(t, stdout)
			if data["message_id"] != "msg_test_001" {
				t.Fatalf("message_id mismatch: got %v", data["message_id"])
			}
			if tc.wantSchedule {
				if data["scheduled"] != true {
					t.Fatalf("expected scheduled=true in output, got %v", data["scheduled"])
				}
			}
		})
	}
}
