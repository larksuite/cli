// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/spf13/cobra"
)

func TestSanitizeURLForDisplay(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "normal file", input: "https://example.com/assets/image.png?x=1", want: "example.com/image.png"},
		{name: "root path falls back to download", input: "https://example.com/", want: "example.com/download"},
		{name: "invalid URL", input: "://bad", want: "[redacted-url]"},
		{name: "missing host", input: "/tmp/file", want: "[redacted-url]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeURLForDisplay(tt.input); got != tt.want {
				t.Fatalf("sanitizeURLForDisplay(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateMessageID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr string
	}{
		{name: "valid", input: " om_123 ", want: "om_123"},
		{name: "empty", input: " ", wantErr: "message ID cannot be empty"},
		{name: "invalid prefix", input: "omt_123", wantErr: "must start with om_"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateMessageID(tt.input)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("validateMessageID(%q) error = %v, want substring %q", tt.input, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateMessageID(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("validateMessageID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestReadDurationHelpersInvalid(t *testing.T) {
	f, err := os.CreateTemp("", "im-duration-invalid-*")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	if _, err := f.WriteString("not-a-valid-media-file"); err != nil {
		t.Fatalf("WriteString() error = %v", err)
	}

	info, err := f.Stat()
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}

	if got := readOggDuration(f, info.Size()); got != 0 {
		t.Fatalf("readOggDuration() = %d, want 0", got)
	}
	if got := readMp4Duration(f, info.Size()); got != 0 {
		t.Fatalf("readMp4Duration() = %d, want 0", got)
	}
}

func TestResolveMarkdownAsPost(t *testing.T) {
	got := resolveMarkdownAsPost(context.Background(), nil, "# Title\n## Subtitle\n\nbody")
	if !strings.Contains(got, `"tag":"md"`) {
		t.Fatalf("resolveMarkdownAsPost() = %q, want post payload", got)
	}
	if !strings.Contains(got, `#### Title`) || !strings.Contains(got, `##### Subtitle`) {
		t.Fatalf("resolveMarkdownAsPost() = %q, want optimized heading levels", got)
	}
	if strings.Contains(got, `<br>`) {
		t.Fatalf("resolveMarkdownAsPost() = %q, want no literal <br>", got)
	}
}

func TestValidateContentFlags(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		markdown   string
		content    string
		image      string
		file       string
		video      string
		videoCover string
		audio      string
		wantErr    []string
	}{
		{name: "multiple media", image: "img_x", file: "file_x", wantErr: []string{"mutually exclusive"}},
		{name: "multiple content", text: "hello", markdown: "# hi", wantErr: []string{"--text, --markdown, and --content cannot be specified together"}},
		{name: "content and media", text: "hello", image: "img_x", wantErr: []string{"--image/--file/--video/--audio cannot be used with --text, --markdown, or --content"}},
		{name: "none specified", wantErr: []string{"specify --content <json>"}},
		{name: "video without cover", video: "file_x", wantErr: []string{"--video-cover is required when using --video"}},
		{name: "video cover without video", videoCover: "img_x", wantErr: []string{"--video-cover can only be used with --video"}},
		{name: "valid text", text: "hello"},
		{name: "valid image", image: "img_x"},
		{name: "valid video with cover", video: "file_x", videoCover: "img_x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateContentFlags(tt.text, tt.markdown, tt.content, tt.image, tt.file, tt.video, tt.videoCover, tt.audio)
			if len(tt.wantErr) == 0 {
				if got != "" {
					t.Fatalf("validateContentFlags() = %q, want empty", got)
				}
				return
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(got, want) {
					t.Fatalf("validateContentFlags() = %q, want substring %q", got, want)
				}
			}
		})
	}
}

func TestValidateExplicitMsgType(t *testing.T) {
	t.Run("nil command", func(t *testing.T) {
		if got := validateExplicitMsgType(nil, "text", "hello", "", "", "", "", ""); got != "" {
			t.Fatalf("validateExplicitMsgType(nil) = %q, want empty", got)
		}
	})

	t.Run("flag not changed", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("msg-type", "", "")
		if got := validateExplicitMsgType(cmd, "text", "hello", "", "", "", "", ""); got != "" {
			t.Fatalf("validateExplicitMsgType() = %q, want empty", got)
		}
	})

	t.Run("matching type", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("msg-type", "", "")
		if err := cmd.Flags().Set("msg-type", "text"); err != nil {
			t.Fatalf("Flags().Set() error = %v", err)
		}
		if got := validateExplicitMsgType(cmd, "text", "hello", "", "", "", "", ""); got != "" {
			t.Fatalf("validateExplicitMsgType() = %q, want empty", got)
		}
	})

	t.Run("conflicting type", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("msg-type", "", "")
		if err := cmd.Flags().Set("msg-type", "text"); err != nil {
			t.Fatalf("Flags().Set() error = %v", err)
		}
		got := validateExplicitMsgType(cmd, "text", "", "# hi", "", "", "", "")
		if !strings.Contains(got, `conflicts with the inferred message type "post"`) {
			t.Fatalf("validateExplicitMsgType() = %q, want conflict message", got)
		}
	})
}

func TestBuildChatMessageListRequest(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"sort":       "asc",
			"page-size":  "80",
			"page-token": "next",
			"start":      "2026-03-01T00:00:00+08:00",
			"end":        "2026-03-02T23:59:59+08:00",
		}, nil)

		got, err := buildChatMessageListRequest(runtime, "oc_123")
		if err != nil {
			t.Fatalf("buildChatMessageListRequest() error = %v", err)
		}

		want := larkcore.QueryParams{
			"container_id_type":     {"chat"},
			"container_id":          {"oc_123"},
			"sort_type":             {"ByCreateTimeAsc"},
			"page_size":             {"50"},
			"card_msg_content_type": {"raw_card_content"},
			"start_time":            {"1772294400"},
			"end_time":              {"1772467199"},
			"page_token":            {"next"},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("buildChatMessageListRequest() = %#v, want %#v", got, want)
		}
	})

	t.Run("invalid start", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"start": "bad-time",
		}, nil)
		_, err := buildChatMessageListRequest(runtime, "oc_123")
		if err == nil || !strings.Contains(err.Error(), "--start") {
			t.Fatalf("buildChatMessageListRequest() error = %v, want start validation", err)
		}
	})

	t.Run("invalid end", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"end": "bad-time",
		}, nil)
		_, err := buildChatMessageListRequest(runtime, "oc_123")
		if err == nil || !strings.Contains(err.Error(), "--end") {
			t.Fatalf("buildChatMessageListRequest() error = %v, want end validation", err)
		}
	})
}

func TestResolveChatIDForMessagesList(t *testing.T) {
	t.Run("chat passthrough", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"chat-id": "oc_123",
		}, nil)
		got, err := resolveChatIDForMessagesList(runtime, false)
		if err != nil {
			t.Fatalf("resolveChatIDForMessagesList() error = %v", err)
		}
		if got != "oc_123" {
			t.Fatalf("resolveChatIDForMessagesList() = %q, want %q", got, "oc_123")
		}
	})

	t.Run("user dry run placeholder", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"user-id": "ou_123",
		}, nil)
		got, err := resolveChatIDForMessagesList(runtime, true)
		if err != nil {
			t.Fatalf("resolveChatIDForMessagesList() error = %v", err)
		}
		if got != "<resolved_chat_id>" {
			t.Fatalf("resolveChatIDForMessagesList() = %q, want placeholder", got)
		}
	})

	t.Run("user resolved through p2p lookup", func(t *testing.T) {
		runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case strings.Contains(req.URL.Path, "tenant_access_token"):
				return shortcutJSONResponse(200, map[string]interface{}{
					"code":                0,
					"tenant_access_token": "tenant-token",
					"expire":              7200,
				}), nil
			case strings.Contains(req.URL.Path, "/open-apis/im/v1/chat_p2p/batch_query"):
				return shortcutJSONResponse(200, map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{
						"p2p_chats": []interface{}{
							map[string]interface{}{"chat_id": "oc_resolved"},
						},
					},
				}), nil
			default:
				return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
			}
		}))
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("user-id", "", "")
		if err := cmd.Flags().Set("user-id", "ou_123"); err != nil {
			t.Fatalf("Flags().Set() error = %v", err)
		}
		runtime.Cmd = cmd

		got, err := resolveChatIDForMessagesList(runtime, false)
		if err != nil {
			t.Fatalf("resolveChatIDForMessagesList() error = %v", err)
		}
		if got != "oc_resolved" {
			t.Fatalf("resolveChatIDForMessagesList() = %q, want %q", got, "oc_resolved")
		}
	})
}

func TestBuildMessagesSearchRequest(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"query":                   "hello",
			"chat-id":                 "oc_1,oc_2",
			"sender":                  "ou_1,ou_2",
			"mention":                 "ou_3",
			"include-attachment-type": "image",
			"chat-type":               "group",
			"sender-type":             "user",
			"exclude-sender-type":     "bot",
			"start":                   "2026-03-01T00:00:00+08:00",
			"end":                     "2026-03-02T23:59:59+08:00",
			"page-size":               "80",
			"page-token":              "next-token",
		}, map[string]bool{
			"at-all": true,
		})

		got, err := buildMessagesSearchRequest(runtime)
		if err != nil {
			t.Fatalf("buildMessagesSearchRequest() error = %v", err)
		}

		want := &messagesSearchRequest{
			params: map[string][]string{
				"page_size":  []string{"50"},
				"page_token": []string{"next-token"},
			},
			body: map[string]interface{}{
				"query": "hello",
				"filter": map[string]interface{}{
					"time_range": map[string]interface{}{
						"start_time": "2026-03-01T00:00:00+08:00",
						"end_time":   "2026-03-02T23:59:59+08:00",
					},
					"chat_ids":                 []string{"oc_1", "oc_2"},
					"from_ids":                 []string{"ou_1", "ou_2"},
					"include_attachment_types": []string{"image"},
					"from_types":               []string{"user"},
					"exclude_from_types":       []string{"bot"},
					"chat_type":                "group",
				},
			},
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("buildMessagesSearchRequest() = %#v, want %#v", got, want)
		}
	})

	t.Run("start later than end", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"start": "2026-03-03T00:00:00+08:00",
			"end":   "2026-03-02T00:00:00+08:00",
		}, nil)
		_, err := buildMessagesSearchRequest(runtime)
		if err == nil || !strings.Contains(err.Error(), "--start cannot be later than --end") {
			t.Fatalf("buildMessagesSearchRequest() error = %v", err)
		}
	})

	t.Run("invalid sender id", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"sender": "bad_sender",
		}, nil)
		_, err := buildMessagesSearchRequest(runtime)
		if err == nil || !strings.Contains(err.Error(), "invalid user ID format") {
			t.Fatalf("buildMessagesSearchRequest() error = %v", err)
		}
	})
}

func TestBuildSearchChatBodyAdditionalBranches(t *testing.T) {
	runtime := newTestRuntimeContext(t, map[string]string{
		"query":        "team-alpha",
		"search-types": "private,external",
		"member-ids":   "ou_1,ou_2",
		"sort-by":      "member_count",
		"page-size":    "0",
		"page-token":   "next-page",
	}, map[string]bool{
		"is-manager":             true,
		"disable-search-by-user": true,
	})

	got := buildSearchChatBody(runtime)
	want := map[string]interface{}{
		"query": `"team-alpha"`,
		"filter": map[string]interface{}{
			"search_types":           []string{"private", "external"},
			"member_ids":             []string{"ou_1", "ou_2"},
			"is_manager":             true,
			"disable_search_by_user": true,
		},
		"sorter": "member_count",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildSearchChatBody() = %#v, want %#v", got, want)
	}
}

func TestResolveToLocalPath(t *testing.T) {
	t.Run("media key returns empty path", func(t *testing.T) {
		got, cleanup, err := resolveToLocalPath(context.Background(), nil, "--image", "img_123")
		if err != nil {
			t.Fatalf("resolveToLocalPath() error = %v", err)
		}
		defer cleanup()
		if got != "" {
			t.Fatalf("resolveToLocalPath() = %q, want empty path", got)
		}
	})

	t.Run("local path passthrough", func(t *testing.T) {
		got, cleanup, err := resolveToLocalPath(context.Background(), nil, "--file", "report.pdf")
		if err != nil {
			t.Fatalf("resolveToLocalPath() error = %v", err)
		}
		defer cleanup()
		if got != "report.pdf" {
			t.Fatalf("resolveToLocalPath() = %q, want %q", got, "report.pdf")
		}
	})
}

func TestParseMediaDurationSuccess(t *testing.T) {
	t.Run("mp4", func(t *testing.T) {
		f, err := os.CreateTemp("", "im-duration-*.mp4")
		if err != nil {
			t.Fatalf("CreateTemp() error = %v", err)
		}
		defer os.Remove(f.Name())
		defer f.Close()

		if _, err := f.Write(wrapInMoov(buildMvhdBox(0, 1000, 5000))); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		if got := parseMediaDuration(f.Name(), "mp4"); got != "5000" {
			t.Fatalf("parseMediaDuration(mp4) = %q, want %q", got, "5000")
		}
	})

	t.Run("opus", func(t *testing.T) {
		f, err := os.CreateTemp("", "im-duration-*.ogg")
		if err != nil {
			t.Fatalf("CreateTemp() error = %v", err)
		}
		defer os.Remove(f.Name())
		defer f.Close()

		page := make([]byte, 27)
		copy(page[0:4], "OggS")
		page[5] = 4
		page[6] = 0x00
		page[7] = 0x53
		page[8] = 0x07
		if _, err := f.Write(page); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		if got := parseMediaDuration(f.Name(), "opus"); got != "10000" {
			t.Fatalf("parseMediaDuration(opus) = %q, want %q", got, "10000")
		}
	})
}

func TestResolveMediaContentURLFallback(t *testing.T) {
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
	}))

	tests := []struct {
		name       string
		image      string
		file       string
		video      string
		videoCover string
		audio      string
		wantType   string
		wantText   string
	}{
		{name: "image URL fallback", image: "http://127.0.0.1/image.png", wantType: "text", wantText: "[image upload failed, sending link] http://127.0.0.1/image.png"},
		{name: "file URL fallback", file: "http://127.0.0.1/report.pdf", wantType: "text", wantText: "[file upload failed, sending link] http://127.0.0.1/report.pdf"},
		{name: "video URL fallback", video: "http://127.0.0.1/video.mp4", videoCover: "img_cover_x", wantType: "text", wantText: "[video upload failed, sending link] http://127.0.0.1/video.mp4"},
		{name: "audio URL fallback", audio: "http://127.0.0.1/audio.ogg", wantType: "text", wantText: "[audio upload failed, sending link] http://127.0.0.1/audio.ogg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotContent, err := resolveMediaContent(context.Background(), runtime, "", tt.image, tt.file, tt.video, tt.videoCover, tt.audio)
			if err != nil {
				t.Fatalf("resolveMediaContent() error = %v", err)
			}
			if gotType != tt.wantType {
				t.Fatalf("resolveMediaContent() type = %q, want %q", gotType, tt.wantType)
			}
			if !strings.Contains(gotContent, tt.wantText) {
				t.Fatalf("resolveMediaContent() content = %q, want substring %q", gotContent, tt.wantText)
			}
		})
	}
}
