// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestNormalizeAtMentions(t *testing.T) {
	input := `<at id=ou_alpha/> hi <at open_id="ou_beta"> and <at user_id=ou_gamma /> and <at email="x@example.com"/>`
	got := normalizeAtMentions(input)
	want := `<at user_id="ou_alpha"> hi <at user_id="ou_beta"> and <at user_id="ou_gamma"> and <at email="x@example.com"/>`
	if got != want {
		t.Fatalf("normalizeAtMentions() = %q, want %q", got, want)
	}
}

func TestDetectIMFileType(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "opus", path: "voice.opus", want: "opus"},
		{name: "ogg", path: "voice.ogg", want: "opus"},
		{name: "video uppercase", path: "movie.MP4", want: "mp4"},
		{name: "document", path: "report.docx", want: "doc"},
		{name: "sheet", path: "data.csv", want: "xls"},
		{name: "slides", path: "deck.ppt", want: "ppt"},
		{name: "pdf", path: "paper.pdf", want: "pdf"},
		{name: "default", path: "archive.zip", want: "stream"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectIMFileType(tt.path); got != tt.want {
				t.Fatalf("detectIMFileType(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestValidateAudioMessageInput(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "empty", value: ""},
		{name: "existing file key", value: "file_abc"},
		{name: "opus file", value: "./voice.opus"},
		{name: "ogg opus file", value: "./voice.ogg"},
		{name: "uppercase opus", value: "./VOICE.OPUS"},
		{name: "mp3 local file", value: "./voice.mp3", wantErr: true},
		{name: "wav local file", value: "./voice.wav", wantErr: true},
		{name: "extensionless local path", value: "./voice"},
		{name: "opus url", value: "https://example.com/voice.opus?download=1"},
		{name: "ogg url", value: "https://example.com/voice.ogg?download=1"},
		{name: "mp3 url", value: "https://example.com/voice.mp3?download=1", wantErr: true},
		{name: "extensionless url", value: "https://example.com/download?id=1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAudioMessageInput("--audio", tt.value)
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "--audio supports only Opus audio files") {
					t.Fatalf("validateAudioMessageInput(%q) error = %v", tt.value, err)
				}
				p, ok := errs.ProblemOf(err)
				if !ok {
					t.Fatalf("validateAudioMessageInput(%q) error is not typed: %v", tt.value, err)
				}
				if p.Category != errs.CategoryValidation || p.Subtype != errs.SubtypeInvalidArgument {
					t.Fatalf("ProblemOf(%q) = category %q subtype %q", tt.value, p.Category, p.Subtype)
				}
				validationErr, ok := err.(*errs.ValidationError)
				if !ok || validationErr.Param != "--audio" {
					t.Fatalf("validateAudioMessageInput(%q) param = %q, want --audio", tt.value, validationErr.Param)
				}
				if !strings.Contains(p.Hint, "use --file") || !strings.Contains(p.Hint, "ffmpeg") {
					t.Fatalf("validateAudioMessageInput(%q) hint = %q, want --file and ffmpeg guidance", tt.value, p.Hint)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateAudioMessageInput(%q) unexpected error = %v", tt.value, err)
			}
		})
	}
}

// TestSplitCSV covers the shared helper that replaced the three identical functions
func TestSplitCSV(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "normal", input: "ou_a,ou_b,ou_c", want: []string{"ou_a", "ou_b", "ou_c"}},
		{name: "spaces around values", input: " ou_a, ,ou_b ,, ou_c ", want: []string{"ou_a", "ou_b", "ou_c"}},
		{name: "single value", input: "om_xxx", want: []string{"om_xxx"}},
		{name: "empty string", input: "", want: nil},
		{name: "only commas and spaces", input: " , , ", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := common.SplitCSV(tt.input); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("common.SplitCSV(%q) = %#v, want %#v", tt.input, got, tt.want)
			}
		})
	}
}

func TestSplitAndTrim(t *testing.T) {
	got := common.SplitCSV(" ou_a, ,ou_b ,, ou_c ")
	want := []string{"ou_a", "ou_b", "ou_c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("common.SplitCSV() = %#v, want %#v", got, want)
	}
}

func TestBuildMediaContentFromKey(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		image      string
		file       string
		video      string
		videoCover string
		audio      string
		wantTyp    string
		wantSub    string
		wantDesc   string
	}{
		{name: "text", text: "hello", wantTyp: "text", wantSub: `"text":"hello"`},
		{name: "image", image: "img_123", wantTyp: "image", wantSub: `"image_key":"img_123"`},
		{name: "file", file: "file_123", wantTyp: "file", wantSub: `"file_key":"file_123"`},
		{name: "video", video: "file_456", videoCover: "img_cover_456", wantTyp: "media", wantSub: `"file_key":"file_456","image_key":"img_cover_456"`},
		{name: "video with cover", video: "file_456", videoCover: "img_cover_123", wantTyp: "media", wantSub: `"file_key":"file_456","image_key":"img_cover_123"`},
		{name: "audio", audio: "file_789", wantTyp: "audio", wantSub: `"file_key":"file_789"`},
		{name: "image url", image: "https://example.com/a.png", wantTyp: "image", wantSub: `"image_key":"img_dryrun_upload"`, wantDesc: "placeholder media keys"},
		{name: "file local path", file: "./report.pdf", wantTyp: "file", wantSub: `"file_key":"file_dryrun_upload"`, wantDesc: "placeholder media keys"},
		{name: "empty", wantTyp: "", wantSub: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTyp, gotContent, gotDesc := buildMediaContentFromKey(tt.text, tt.image, tt.file, tt.video, tt.videoCover, tt.audio)
			if gotTyp != tt.wantTyp {
				t.Fatalf("buildMediaContentFromKey() type = %q, want %q", gotTyp, tt.wantTyp)
			}
			if tt.wantDesc == "" {
				if gotDesc != "" {
					t.Fatalf("buildMediaContentFromKey() desc = %q, want empty", gotDesc)
				}
			} else if !strings.Contains(gotDesc, tt.wantDesc) {
				t.Fatalf("buildMediaContentFromKey() desc = %q, want substring %q", gotDesc, tt.wantDesc)
			}
			if tt.wantSub == "" {
				if gotContent != "" {
					t.Fatalf("buildMediaContentFromKey() content = %q, want empty", gotContent)
				}
				return
			}
			if !strings.Contains(gotContent, tt.wantSub) {
				t.Fatalf("buildMediaContentFromKey() content = %q, want substring %q", gotContent, tt.wantSub)
			}
		})
	}
}

// TestWrapMarkdownAsPostForDryRun covers markdown-to-post wrapping used by dry runs.
func TestWrapMarkdownAsPostForDryRun(t *testing.T) {
	content, desc := wrapMarkdownAsPostForDryRun("hello ![alt](https://example.com/a.png)")
	if !strings.Contains(content, `![alt](img_dryrun_1)`) {
		t.Fatalf("wrapMarkdownAsPostForDryRun() content = %q, want placeholder img key", content)
	}
	if !strings.Contains(desc, "placeholder image keys") {
		t.Fatalf("wrapMarkdownAsPostForDryRun() desc = %q, want placeholder note", desc)
	}
}

// TestParseAttachmentFlag covers bare key parsing and empty-value rejection.
func TestParseAttachmentFlag(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantKey string
		wantErr bool
	}{
		{name: "key only", value: "file_123", wantKey: "file_123"},
		{name: "whitespace trimmed", value: "  file_123  ", wantKey: "file_123"},
		{name: "empty", value: "   ", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAttachmentFlag(tt.value, "--attachment")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseAttachmentFlag(%q) expected error, got %#v", tt.value, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseAttachmentFlag(%q) unexpected error: %v", tt.value, err)
			}
			if got.Key != tt.wantKey {
				t.Fatalf("parseAttachmentFlag(%q) = %+v, want key=%q", tt.value, got, tt.wantKey)
			}
		})
	}
}

// TestParseAttachments parses repeated values in order.

func TestParseAttachments(t *testing.T) {
	got, err := parseAttachments([]string{"file_1", "file_2"}, "--attachment")
	if err != nil {
		t.Fatalf("parseAttachments unexpected error: %v", err)
	}
	if len(got) != 2 || got[0].Key != "file_1" || got[1].Key != "file_2" {
		t.Fatalf("parseAttachments() = %+v, want 2 items with keys set", got)
	}
	// TestMergeAttachmentsIntoPostContent merges files into post content top-level, preserving existing files.
}

// TestMergeAttachmentsIntoPostContent merges files into post content top-level, preserving existing files.
func TestMergeAttachmentsIntoPostContent(t *testing.T) {
	items := []attachmentItem{{Key: "file_1"}, {Key: "file_2"}}
	merged, err := mergeAttachmentsIntoPostContent(`{"zh_cn":{"content":[[{"tag":"text","text":"hi"}]]}}`, items)
	if err != nil {
		t.Fatalf("mergeAttachmentsIntoPostContent unexpected error: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(merged), &parsed); err != nil {
		t.Fatalf("merged content is not valid JSON: %v\n%s", err, merged)
	}
	files, ok := parsed["files"].([]interface{})
	if !ok || len(files) != 2 {
		t.Fatalf("merged files = %#v, want 2 entries", parsed["files"])
	}
	// 不信任客户端 name：files 元素只应有 key，不应带 name 字段
	for _, f := range files {
		m, _ := f.(map[string]interface{})
		if _, hasName := m["name"]; hasName {
			t.Fatalf("merged files entry must not carry client name, got %#v", m)
		}
	}
	if _, ok := parsed["zh_cn"]; !ok {
		t.Fatalf("merged content lost the post body: %s", merged)
	}

	// Pre-existing files in --content are preserved, and --attachment appends.
	merged2, err := mergeAttachmentsIntoPostContent(`{"files":[{"key":"existing"}]}`, items)
	if err != nil {
		t.Fatalf("mergeAttachmentsIntoPostContent(pre) unexpected error: %v", err)
	}
	var parsed2 map[string]interface{}
	_ = json.Unmarshal([]byte(merged2), &parsed2)
	files2, _ := parsed2["files"].([]interface{})
	if len(files2) != 3 {
		t.Fatalf("merged files with pre-existing = %d entries, want 3", len(files2))
	}

	// JSON "null" content must not panic: treat it as empty and attach files.
	merged3, err := mergeAttachmentsIntoPostContent("null", items)
	if err != nil {
		t.Fatalf("mergeAttachmentsIntoPostContent(null) unexpected error: %v", err)
	}
	var parsed3 map[string]interface{}
	_ = json.Unmarshal([]byte(merged3), &parsed3)
	files3, _ := parsed3["files"].([]interface{})
	if len(files3) != 2 {
		t.Fatalf("merged files with null content = %d entries, want 2", len(files3))
	}

	// Duplicate keys are deduplicated: a key already in --content is not
	// appended again by --attachment, and repeated --attachment values collapse.
	merged4, err := mergeAttachmentsIntoPostContent(`{"files":[{"key":"file_1"}]}`, items)
	if err != nil {
		t.Fatalf("mergeAttachmentsIntoPostContent(dedup) unexpected error: %v", err)
	}
	var parsed4 map[string]interface{}
	_ = json.Unmarshal([]byte(merged4), &parsed4)
	files4, _ := parsed4["files"].([]interface{})
	if len(files4) != 2 {
		t.Fatalf("merged files with duplicate = %d entries, want 2 (file_1, file_2)", len(files4))
	}
}

// TestValidateAttachmentFlags covers key prefix and post-only constraints.
// Attachments imply post: without an explicit --msg-type the type is inferred
// as post; only an explicit incompatible --msg-type conflicts.
func TestValidateAttachmentFlags(t *testing.T) {
	tests := []struct {
		name            string
		values          []string
		msgType         string
		markdown        string
		msgTypeExplicit bool
		wantErr         bool
		wantSub         errs.Subtype // expected validation subtype when wantErr
		wantFlag        string       // expected --flag in the typed error
	}{
		{name: "post with markdown", values: []string{"file_1"}, msgType: "text", markdown: "# hi", wantErr: false},
		{name: "post via explicit msg-type", values: []string{"file_1"}, msgType: "post", markdown: "", msgTypeExplicit: true, wantErr: false},
		{name: "implicit msg-type infers post", values: []string{"file_1"}, msgType: "text", markdown: "", msgTypeExplicit: false, wantErr: false},
		{name: "explicit non-post rejected", values: []string{"file_1"}, msgType: "text", markdown: "", msgTypeExplicit: true, wantErr: true, wantSub: errs.SubtypeInvalidArgument, wantFlag: "--attachment"},
		{name: "non-file key rejected", values: []string{"img_1"}, msgType: "post", markdown: "", wantErr: true, wantSub: errs.SubtypeInvalidArgument, wantFlag: "--attachment"},
		{name: "empty ok", values: nil, msgType: "text", markdown: "", wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateAttachmentFlags(tt.values, tt.msgType, tt.markdown, "--attachment", tt.msgTypeExplicit, "")
			if tt.wantErr && err == nil {
				t.Fatalf("validateAttachmentFlags() expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("validateAttachmentFlags() unexpected error: %v", err)
			}
			if tt.wantErr {
				var ve *errs.ValidationError
				if !errors.As(err, &ve) {
					t.Fatalf("validateAttachmentFlags() error = %T, want *errs.ValidationError", err)
				}
				if tt.wantSub != "" && ve.Subtype != tt.wantSub {
					t.Fatalf("validateAttachmentFlags() subtype = %q, want %q", ve.Subtype, tt.wantSub)
				}
				if tt.wantFlag != "" && ve.Param != tt.wantFlag {
					t.Fatalf("validateAttachmentFlags() param = %q, want %q", ve.Param, tt.wantFlag)
				}
			}
		})
	}
}

func TestResolveMediaContentWithoutUploads(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		image      string
		file       string
		video      string
		videoCover string
		audio      string
		wantTyp    string
		wantSub    string
	}{
		{name: "text", text: "hello", wantTyp: "text", wantSub: `"text":"hello"`},
		{name: "image key", image: "img_123", wantTyp: "image", wantSub: `"image_key":"img_123"`},
		{name: "file key", file: "file_123", wantTyp: "file", wantSub: `"file_key":"file_123"`},
		{name: "video key", video: "file_456", videoCover: "img_cover_456", wantTyp: "media", wantSub: `"file_key":"file_456","image_key":"img_cover_456"`},
		{name: "audio key", audio: "file_789", wantTyp: "audio", wantSub: `"file_key":"file_789"`},
		{name: "empty", wantTyp: "", wantSub: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTyp, gotContent, err := resolveMediaContent(context.Background(), nil, tt.text, tt.image, tt.file, tt.video, tt.videoCover, tt.audio)
			if err != nil {
				t.Fatalf("resolveMediaContent() error = %v", err)
			}
			if gotTyp != tt.wantTyp {
				t.Fatalf("resolveMediaContent() type = %q, want %q", gotTyp, tt.wantTyp)
			}
			if tt.wantSub == "" {
				if gotContent != "" {
					t.Fatalf("resolveMediaContent() content = %q, want empty", gotContent)
				}
				return
			}
			if !strings.Contains(gotContent, tt.wantSub) {
				t.Fatalf("resolveMediaContent() content = %q, want substring %q", gotContent, tt.wantSub)
			}
		})
	}
}

func TestParseOggOpusDuration(t *testing.T) {
	// Granule = 480000 samples at 48kHz → 10s → 10000ms
	page := make([]byte, 27)
	copy(page[0:4], "OggS")
	page[5] = 4 // last page flag
	// granule position at offset 6 (LE uint64 = 480000)
	page[6] = 0x00
	page[7] = 0x53
	page[8] = 0x07

	if got := parseOggOpusDuration(page); got != 10000 {
		t.Fatalf("parseOggOpusDuration() = %d, want 10000", got)
	}
	if got := parseOggOpusDuration(nil); got != 0 {
		t.Fatalf("parseOggOpusDuration(nil) = %d, want 0", got)
	}
	if got := parseOggOpusDuration([]byte("not ogg")); got != 0 {
		t.Fatalf("parseOggOpusDuration(invalid) = %d, want 0", got)
	}
}

// buildMvhdBox creates a minimal mvhd box with the given version, timescale, and duration.
func buildMvhdBox(version byte, timescale uint32, dur uint64) []byte {
	var payload []byte
	if version == 0 {
		payload = make([]byte, 20)
		payload[0] = 0
		binary.BigEndian.PutUint32(payload[12:], timescale)
		binary.BigEndian.PutUint32(payload[16:], uint32(dur))
	} else {
		payload = make([]byte, 32)
		payload[0] = 1
		binary.BigEndian.PutUint32(payload[20:], timescale)
		binary.BigEndian.PutUint64(payload[24:], dur)
	}
	box := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(box[0:4], uint32(len(box)))
	copy(box[4:8], "mvhd")
	copy(box[8:], payload)
	return box
}

// wrapInMoov wraps inner box data in a moov box.
func wrapInMoov(inner []byte) []byte {
	moov := make([]byte, 8+len(inner))
	binary.BigEndian.PutUint32(moov[0:4], uint32(len(moov)))
	copy(moov[4:8], "moov")
	copy(moov[8:], inner)
	return moov
}

func TestParseMp4Duration(t *testing.T) {
	t.Run("version 0", func(t *testing.T) {
		// timescale=1000, duration=5000 → 5000ms
		data := wrapInMoov(buildMvhdBox(0, 1000, 5000))
		if got := parseMp4Duration(data); got != 5000 {
			t.Fatalf("parseMp4Duration(v0) = %d, want 5000", got)
		}
	})

	t.Run("version 1", func(t *testing.T) {
		// timescale=44100, duration=441000 → 10000ms
		data := wrapInMoov(buildMvhdBox(1, 44100, 441000))
		if got := parseMp4Duration(data); got != 10000 {
			t.Fatalf("parseMp4Duration(v1) = %d, want 10000", got)
		}
	})

	t.Run("nil", func(t *testing.T) {
		if got := parseMp4Duration(nil); got != 0 {
			t.Fatalf("parseMp4Duration(nil) = %d, want 0", got)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		if got := parseMp4Duration([]byte("not mp4")); got != 0 {
			t.Fatalf("parseMp4Duration(invalid) = %d, want 0", got)
		}
	})
}

func TestParseMediaDuration(t *testing.T) {
	rt := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("unexpected")
	}))
	if got := parseMediaDuration(rt, "test.pdf", "pdf"); got != "" {
		t.Fatalf("parseMediaDuration(pdf) = %q, want empty", got)
	}
	if got := parseMediaDuration(rt, "nonexistent.opus", "opus"); got != "" {
		t.Fatalf("parseMediaDuration(missing) = %q, want empty", got)
	}
}

func TestOptimizeMarkdownStyle(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "heading downgrade H1 and H2",
			input: "# Title\n## Section\ntext",
			want:  "#### Title\n\n##### Section\ntext",
		},
		{
			name:  "no downgrade when no H1-H3",
			input: "#### Already H4\ntext",
			want:  "#### Already H4\ntext",
		},
		{
			name:  "code block protected",
			input: "# Title\n```\n# not a heading\n```\ntext",
			want:  "#### Title\n```\n# not a heading\n```\ntext",
		},
		{
			name:  "table spacing",
			input: "text\n| A | B |\n| - | - |\n| 1 | 2 |\nafter",
			want:  "text\n\n| A | B |\n| - | - |\n| 1 | 2 |\n\nafter",
		},
		{
			name:  "table spacing keeps heading separation",
			input: "# Title\n| A | B |\n| - | - |\n| 1 | 2 |\n## Next",
			want:  "#### Title\n\n| A | B |\n| - | - |\n| 1 | 2 |\n\n##### Next",
		},
		{
			name:  "excess blank lines compressed",
			input: "a\n\n\n\nb",
			want:  "a\n\nb",
		},
		{
			name:  "strip invalid image keep img_key",
			input: "![alt](img_abc123) ![bad](https://example.com/x.png)",
			want:  "![alt](img_abc123) ",
		},
		{
			name:  "empty input",
			input: "",
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := optimizeMarkdownStyle(tt.input)
			if got != tt.want {
				t.Errorf("optimizeMarkdownStyle():\n got: %q\nwant: %q", got, tt.want)
			}
		})
	}
}

func TestWrapMarkdownAsPost(t *testing.T) {
	got := wrapMarkdownAsPost("hello **world**")
	// Should produce valid JSON with post structure
	if !strings.Contains(got, `"tag":"md"`) {
		t.Fatalf("wrapMarkdownAsPost() missing md tag: %s", got)
	}
	if !strings.Contains(got, `"zh_cn"`) {
		t.Fatalf("wrapMarkdownAsPost() missing zh_cn: %s", got)
	}
	if !strings.Contains(got, "hello **world**") {
		t.Fatalf("wrapMarkdownAsPost() missing content: %s", got)
	}
}

func TestIsURL(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"https://example.com/photo.jpg", true},
		{"http://example.com/file.pdf", true},
		{"img_abc123", false},
		{"file_abc123", false},
		{"./local/file.jpg", false},
		{"/absolute/path.png", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isURL(tt.input); got != tt.want {
			t.Errorf("isURL(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestFileNameFromURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://example.com/photos/cat.jpg", "cat.jpg"},
		{"https://example.com/", "download"},
		{"https://example.com", "download"},
		{"https://example.com/path/file.pdf?token=abc", "file.pdf"},
		{"not a url", "download"},
	}
	for _, tt := range tests {
		if got := fileNameFromURL(tt.input); got != tt.want {
			t.Errorf("fileNameFromURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMediaFallbackOrError(t *testing.T) {
	testErr := errors.New("upload failed")

	// URL input: should fallback to text
	mt, content, err := mediaFallbackOrError("https://example.com/photo.jpg", "image", testErr)
	if err != nil {
		t.Fatalf("mediaFallbackOrError(URL) returned error: %v", err)
	}
	if mt != "text" {
		t.Fatalf("mediaFallbackOrError(URL) mt = %q, want text", mt)
	}
	if !strings.Contains(content, "https://example.com/photo.jpg") {
		t.Fatalf("mediaFallbackOrError(URL) content missing URL: %s", content)
	}

	// Local file input: should return hard error
	_, _, err = mediaFallbackOrError("./local.jpg", "image", testErr)
	if err == nil {
		t.Fatal("mediaFallbackOrError(local) should return error")
	}
}

func TestResolveMarkdownImageURLs_NoImages(t *testing.T) {
	input := "just text, no images"
	got := resolveMarkdownImageURLs(context.Background(), nil, input)
	if got != input {
		t.Fatalf("resolveMarkdownImageURLs(no images) changed text: %q", got)
	}
}

func TestNormalizeChatSearchQuery(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain query", input: "project", want: "project"},
		{name: "hyphenated query gets quoted", input: "team-alpha", want: `"team-alpha"`},
		{name: "fully quoted query is normalized", input: `"team-alpha"`, want: `"team-alpha"`},
		{name: "partially quoted query is re-quoted as whole string", input: `"team-alpha`, want: `"\"team-alpha"`},
		{name: "embedded quote is escaped", input: `team-"alpha"`, want: `"team-\"alpha\""`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeChatSearchQuery(tt.input); got != tt.want {
				t.Fatalf("normalizeChatSearchQuery(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeDownloadOutputPath(t *testing.T) {
	tests := []struct {
		name       string
		fileKey    string
		outputPath string
		want       string
		wantErr    string
	}{
		{name: "default to file key", fileKey: "file_123", want: "file_123"},
		{name: "clean relative path", fileKey: "file_123", outputPath: " nested/../out.bin ", want: "out.bin"},
		{name: "empty key", fileKey: " ", wantErr: "file-key cannot be empty"},
		{name: "separator in key", fileKey: "dir/file", wantErr: "file-key cannot contain path separators"},
		// Where the path points is the built-in policy's call, made by the
		// ResolveSavePath both call sites run next; this function only settles
		// what the caller named. An absolute path reaching that policy is the
		// whole point — refusing it here is what failed an agent's first call.
		{name: "absolute path passes through", fileKey: "file_123", outputPath: "/tmp/out.bin", want: "/tmp/out.bin"},
		{name: "parent-relative path passes through", fileKey: "file_123", outputPath: "../out.bin", want: "../out.bin"},
		{name: "empty path after clean", fileKey: "file_123", outputPath: " . ", wantErr: "path cannot be empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeDownloadOutputPath(tt.fileKey, tt.outputPath)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("normalizeDownloadOutputPath() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeDownloadOutputPath() unexpected error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("normalizeDownloadOutputPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDownloadIMResourceToPathHTTPClientError(t *testing.T) {
	// DoAPIStream now goes through APIClient, which requires a fully constructed Factory.
	// When HttpClient returns an error, NewAPIClient fails, and getAPIClient propagates it.
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("http client unavailable")
	}))

	_, _, err := downloadIMResourceToPath(context.Background(), runtime, "om_123", "img_123", "image", "out.bin", true)
	if err == nil || !strings.Contains(err.Error(), "http client unavailable") {
		t.Fatalf("downloadIMResourceToPath() error = %v", err)
	}
}

func TestParseContentDispositionFilename(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{name: "empty header", header: "", want: ""},
		{name: "no filename param", header: "attachment", want: ""},
		{name: "plain filename", header: `attachment; filename="report.xlsx"`, want: "report.xlsx"},
		{name: "unquoted filename", header: `attachment; filename=report.xlsx`, want: "report.xlsx"},
		{name: "RFC 5987 UTF-8 encoded", header: `attachment; filename*=UTF-8''%E5%AD%A3%E5%BA%A6%E6%8A%A5%E5%91%8A.xlsx`, want: "季度报告.xlsx"},
		{name: "RFC 5987 takes priority over plain", header: `attachment; filename="fallback.xlsx"; filename*=UTF-8''%E5%AD%A3%E5%BA%A6%E6%8A%A5%E5%91%8A.xlsx`, want: "季度报告.xlsx"},
		{name: "path traversal stripped", header: `attachment; filename="../../etc/passwd"`, want: "passwd"},
		{name: "windows path stripped", header: `attachment; filename="C:\\Windows\\evil.exe"`, want: "evil.exe"},
		{name: "control char rejected", header: "attachment; filename=\"evil\x01file.txt\"", want: ""},
		{name: "malformed header", header: "not/valid/mime; ===", want: ""},
		{name: "whitespace trimmed", header: `attachment; filename="  report.pdf  "`, want: "report.pdf"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseContentDispositionFilename(tt.header); got != tt.want {
				t.Fatalf("parseContentDispositionFilename(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}

func TestResolveIMResourceDownloadPath(t *testing.T) {
	tests := []struct {
		name               string
		safePath           string
		contentType        string
		contentDisposition string
		preserveBasename   bool
		want               string
	}{
		// safePath already has extension: always return as-is
		{name: "user path with ext, no CD", safePath: "out.xlsx", contentType: "application/pdf", preserveBasename: true, want: "out.xlsx"},
		{name: "user path with ext, CD present", safePath: "out.xlsx", contentDisposition: `attachment; filename="server.pdf"`, preserveBasename: true, want: "out.xlsx"},
		// No --output: use CD filename when present
		{name: "default path, CD filename", safePath: "file_xxx", contentDisposition: `attachment; filename="季度报告.xlsx"`, want: "季度报告.xlsx"},
		{name: "default path, CD RFC5987", safePath: "file_xxx", contentDisposition: `attachment; filename*=UTF-8''%E5%AD%A3%E5%BA%A6%E6%8A%A5%E5%91%8A.xlsx`, want: "季度报告.xlsx"},
		{name: "default path, no CD, MIME ext", safePath: "file_xxx", contentType: "application/pdf", want: "file_xxx.pdf"},
		{name: "default path, no CD, unknown MIME", safePath: "file_xxx", contentType: "application/x-unknown", want: "file_xxx"},
		{name: "default path, CD with dir component", safePath: "downloads/file_xxx", contentDisposition: `attachment; filename="report.xlsx"`, want: "downloads/report.xlsx"},
		// User --output without extension: use CD filename's extension
		{name: "user path no ext, CD with ext", safePath: "myfile", contentDisposition: `attachment; filename="server.pdf"`, preserveBasename: true, want: "myfile.pdf"},
		{name: "user path no ext, CD no ext, MIME ext", safePath: "myfile", contentDisposition: `attachment; filename="noext"`, contentType: "image/png", preserveBasename: true, want: "myfile.png"},
		{name: "user path no ext, no CD, MIME ext", safePath: "myfile", contentType: "image/jpeg", preserveBasename: true, want: "myfile.jpg"},
		// Batch --download-resources (preserveBasename=true): the file_key basename
		// is kept and only the extension borrowed, so two resources whose servers
		// return the SAME Content-Disposition filename still resolve to distinct
		// paths instead of clobbering each other.
		{name: "batch key A, shared CD filename", safePath: "lark-im-resources/file_aaa", contentDisposition: `attachment; filename="download.bin"`, preserveBasename: true, want: "lark-im-resources/file_aaa.bin"},
		{name: "batch key B, shared CD filename", safePath: "lark-im-resources/file_bbb", contentDisposition: `attachment; filename="download.bin"`, preserveBasename: true, want: "lark-im-resources/file_bbb.bin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveIMResourceDownloadPath(tt.safePath, tt.contentType, tt.contentDisposition, tt.preserveBasename)
			if got != tt.want {
				t.Fatalf("resolveIMResourceDownloadPath() = %q, want %q", got, tt.want)
			}
			// TestShortcuts verifies all IM shortcuts are registered and match expected commands.
		})
	}
}

// TestShortcuts verifies all IM shortcuts are registered and match expected commands.
func TestShortcuts(t *testing.T) {
	var commands []string
	for _, shortcut := range Shortcuts() {
		commands = append(commands, shortcut.Command)
	}

	want := []string{
		"+chat-create",
		"+chat-list",
		"+chat-members-list",
		"+chat-messages-list",
		"+chat-search",
		"+chat-update",
		"+message-read-users",
		"+messages-edit",
		"+messages-mget",
		"+messages-read-status",
		"+messages-reply",
		"+messages-resources-download",
		"+messages-search",
		"+messages-send",
		"+threads-messages-list",
		"+flag-create",
		"+flag-cancel",
		"+flag-list",
		"+feed-shortcut-create",
		"+feed-shortcut-remove",
		"+feed-shortcut-list",
		"+feed-group-list",
		"+feed-group-list-item",
		"+feed-group-query-item",
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("Shortcuts() commands = %#v, want %#v", commands, want)
	}
}

// TestSenderDisplay covers the human-readable sender column: a resolved name wins,
// otherwise the sender id is shown (AC3 fallback), and a system/senderless message
// with neither yields an empty string (no name is normal, not an error).
func TestSenderDisplay(t *testing.T) {
	cases := []struct {
		name   string
		sender map[string]interface{}
		want   string
	}{
		{"name wins", map[string]interface{}{"name": "Bot Alpha", "id": "cli_bot"}, "Bot Alpha"},
		{"id fallback when no name (AC3)", map[string]interface{}{"id": "cli_bot", "open_bot_id": "ou_bot"}, "cli_bot"},
		{"user id fallback", map[string]interface{}{"id": "ou_user"}, "ou_user"},
		{"empty name falls back to id", map[string]interface{}{"name": "", "id": "cli_x"}, "cli_x"},
		{"no name no id (system)", map[string]interface{}{"sender_type": "system"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := senderDisplay(c.sender); got != c.want {
				t.Fatalf("senderDisplay(%#v) = %q, want %q", c.sender, got, c.want)
			}
		})
	}
}

// newSliceRuntimeContext builds a RuntimeContext supporting string and
// string-slice flags, for validating send/reply with repeatable --attachment.
func newSliceRuntimeContext(stringFlags map[string]string, sliceFlags map[string][]string) *common.RuntimeContext {
	cmd := &cobra.Command{Use: "test"}
	for name := range stringFlags {
		cmd.Flags().String(name, "", "")
	}
	for name := range sliceFlags {
		cmd.Flags().StringSlice(name, nil, "")
	}
	for name, val := range stringFlags {
		_ = cmd.Flags().Set(name, val)
	}
	for name, vals := range sliceFlags {
		_ = cmd.Flags().Set(name, strings.Join(vals, ","))
	}
	return &common.RuntimeContext{Cmd: cmd}
}

// TestSendReplyAttachmentDoesNotBypassContentValidation is the P1 regression
// guard: --attachment must NOT swallow mutual-exclusion / media-integrity
// errors when another content source is present (a conflicting --text or
// --markdown was previously silently dropped).
func TestSendReplyAttachmentDoesNotBypassContentValidation(t *testing.T) {
	t.Run("send text+markdown+attachment rejected", func(t *testing.T) {
		rt := newSliceRuntimeContext(map[string]string{
			"chat-id": "oc_123", "text": "conflict", "markdown": "# wins",
		}, map[string][]string{"attachment": {"file_1"}})
		err := ImMessagesSend.Validate(context.Background(), rt)
		if err == nil || (!strings.Contains(err.Error(), "cannot be specified together") && !strings.Contains(err.Error(), "cannot be combined with --text")) {
			t.Fatalf("ImMessagesSend.Validate() = %v, want text/markdown conflict", err)
		}
	})

	t.Run("send markdown+image+attachment rejected", func(t *testing.T) {
		rt := newSliceRuntimeContext(map[string]string{
			"chat-id": "oc_123", "markdown": "# wins", "image": "img_1",
		}, map[string][]string{"attachment": {"file_1"}})
		err := ImMessagesSend.Validate(context.Background(), rt)
		if err == nil || !strings.Contains(err.Error(), "cannot be used with") {
			t.Fatalf("ImMessagesSend.Validate() = %v, want media/content conflict", err)
		}
	})

	t.Run("send attachment-only post allowed", func(t *testing.T) {
		rt := newSliceRuntimeContext(map[string]string{
			"chat-id": "oc_123", "msg-type": "post",
		}, map[string][]string{"attachment": {"file_1"}})
		if err := ImMessagesSend.Validate(context.Background(), rt); err != nil {
			t.Fatalf("ImMessagesSend.Validate() unexpected error = %v (attachment-only post must be allowed)", err)
		}
	})

	t.Run("reply markdown+image+attachment rejected", func(t *testing.T) {
		rt := newSliceRuntimeContext(map[string]string{
			"message-id": "om_1", "markdown": "# wins", "image": "img_1",
		}, map[string][]string{"attachment": {"file_1"}})
		err := ImMessagesReply.Validate(context.Background(), rt)
		if err == nil || !strings.Contains(err.Error(), "cannot be used with") {
			t.Fatalf("ImMessagesReply.Validate() = %v, want media/content conflict", err)
		}
	})

	t.Run("reply attachment-only post allowed", func(t *testing.T) {
		rt := newSliceRuntimeContext(map[string]string{
			"message-id": "om_1", "msg-type": "post",
		}, map[string][]string{"attachment": {"file_1"}})
		if err := ImMessagesReply.Validate(context.Background(), rt); err != nil {
			t.Fatalf("ImMessagesReply.Validate() unexpected error = %v (attachment-only reply must be allowed)", err)
		}
	})
}

// TestReplaceAttachmentsIntoPostContent verifies the edit "set" semantic: the
// flag value becomes the FINAL files list, discarding --content's files and
// never duplicating keys.
func TestReplaceAttachmentsIntoPostContent(t *testing.T) {
	t.Run("replaces content files, does not merge", func(t *testing.T) {
		got, err := replaceAttachmentsIntoPostContent(`{"zh_cn":{"content":[]},"files":[{"key":"file_old"}]}`, []attachmentItem{{Key: "file_new"}})
		if err != nil {
			t.Fatalf("replaceAttachmentsIntoPostContent() error = %v", err)
		}
		var parsed map[string]interface{}
		_ = json.Unmarshal([]byte(got), &parsed)
		files, _ := parsed["files"].([]interface{})
		if len(files) != 1 || files[0].(map[string]interface{})["key"] != "file_new" {
			t.Fatalf("replace files = %#v, want only [file_new]", parsed["files"])
		}
		if _, ok := parsed["zh_cn"]; !ok {
			t.Fatalf("replace lost the post body: %s", got)
		}
	})

	t.Run("no duplicate when same key in content and flag", func(t *testing.T) {
		got, err := replaceAttachmentsIntoPostContent(`{"zh_cn":{"content":[]},"files":[{"key":"file_old"}]}`, []attachmentItem{{Key: "file_old"}})
		if err != nil {
			t.Fatalf("replaceAttachmentsIntoPostContent() error = %v", err)
		}
		var parsed map[string]interface{}
		_ = json.Unmarshal([]byte(got), &parsed)
		files, _ := parsed["files"].([]interface{})
		if len(files) != 1 {
			t.Fatalf("replace files = %#v, want single file_old (no duplicate)", parsed["files"])
		}
	})

	t.Run("multiple flag keys replace in order", func(t *testing.T) {
		got, err := replaceAttachmentsIntoPostContent(`{"zh_cn":{"content":[]},"files":[{"key":"file_old"}]}`, []attachmentItem{{Key: "a"}, {Key: "b"}})
		if err != nil {
			t.Fatalf("replaceAttachmentsIntoPostContent() error = %v", err)
		}
		var parsed map[string]interface{}
		_ = json.Unmarshal([]byte(got), &parsed)
		files, _ := parsed["files"].([]interface{})
		if len(files) != 2 || files[0].(map[string]interface{})["key"] != "a" || files[1].(map[string]interface{})["key"] != "b" {
			t.Fatalf("replace files = %#v, want [a, b]", parsed["files"])
		}
	})
}

// TestAttachmentFlagsMutuallyExclusiveWithContentFiles verifies the A-plan
// contract: --content that already declares a files array cannot be combined
// with attachment flags (--attachment / --set-attachments / --clear-attachments).
func TestAttachmentFlagsMutuallyExclusiveWithContentFiles(t *testing.T) {
	contentWithFiles := `{"zh_cn":{"content":[]},"files":[{"key":"file_old"}]}`
	contentNoFiles := `{"zh_cn":{"content":[]}}`

	t.Run("send content-with-files + attachment rejected", func(t *testing.T) {
		rt := newSliceRuntimeContext(map[string]string{
			"chat-id": "oc_123", "msg-type": "post", "content": contentWithFiles,
		}, map[string][]string{"attachment": {"file_new"}})
		err := ImMessagesSend.Validate(context.Background(), rt)
		if err == nil || !strings.Contains(err.Error(), "files array") {
			t.Fatalf("ImMessagesSend.Validate() = %v, want files-array conflict", err)
		}
	})

	t.Run("send content-no-files + attachment allowed", func(t *testing.T) {
		rt := newSliceRuntimeContext(map[string]string{
			"chat-id": "oc_123", "msg-type": "post", "content": contentNoFiles,
		}, map[string][]string{"attachment": {"file_new"}})
		if err := ImMessagesSend.Validate(context.Background(), rt); err != nil {
			t.Fatalf("ImMessagesSend.Validate() unexpected error = %v", err)
		}
	})

	t.Run("edit content-with-files + set-attachments rejected", func(t *testing.T) {
		rt := newEditTestRuntimeContext(map[string]string{
			"message-id": "om_1", "msg-type": "post", "content": contentWithFiles,
		}, map[string][]string{"set-attachments": {"file_new"}})
		err := ImMessagesEdit.Validate(context.Background(), rt)
		if err == nil || !strings.Contains(err.Error(), "files array") {
			t.Fatalf("ImMessagesEdit.Validate() = %v, want files-array conflict", err)
		}
	})

	t.Run("edit content-with-files + clear-attachments rejected", func(t *testing.T) {
		rt := newEditTestRuntimeContext(map[string]string{
			"message-id": "om_1", "msg-type": "post", "content": contentWithFiles, "clear-attachments": "true",
		}, nil)
		err := ImMessagesEdit.Validate(context.Background(), rt)
		if err == nil || !strings.Contains(err.Error(), "files array") {
			t.Fatalf("ImMessagesEdit.Validate() = %v, want files-array conflict", err)
		}
	})

	t.Run("edit content-no-files + set-attachments allowed", func(t *testing.T) {
		rt := newEditTestRuntimeContext(map[string]string{
			"message-id": "om_1", "msg-type": "post", "content": contentNoFiles,
		}, map[string][]string{"set-attachments": {"file_new"}})
		if err := ImMessagesEdit.Validate(context.Background(), rt); err != nil {
			t.Fatalf("ImMessagesEdit.Validate() unexpected error = %v", err)
		}
	})
}
