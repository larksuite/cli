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

	"github.com/larksuite/cli/shortcuts/common"
)

func decodePostContentForTest(t *testing.T, raw string) []interface{} {
	t.Helper()

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, raw=%s", err, raw)
	}
	locale, _ := payload["zh_cn"].(map[string]interface{})
	content, _ := locale["content"].([]interface{})
	if content == nil {
		t.Fatalf("post content missing: %#v", payload)
	}
	return content
}

func decodePostParagraphForTest(t *testing.T, raw string, idx int) map[string]interface{} {
	t.Helper()

	content := decodePostContentForTest(t, raw)
	if idx >= len(content) {
		t.Fatalf("paragraph index %d out of range, len=%d, raw=%s", idx, len(content), raw)
	}
	paragraph, _ := content[idx].([]interface{})
	if len(paragraph) != 1 {
		t.Fatalf("paragraph %d = %#v, want single node", idx, paragraph)
	}
	node, _ := paragraph[0].(map[string]interface{})
	return node
}

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

func TestWrapMarkdownAsPostForDryRun(t *testing.T) {
	content, desc := wrapMarkdownAsPostForDryRun("hello ![alt](https://example.com/a.png)")
	if !strings.Contains(content, `![alt](img_dryrun_1)`) {
		t.Fatalf("wrapMarkdownAsPostForDryRun() content = %q, want placeholder img key", content)
	}
	if !strings.Contains(desc, "placeholder image keys") {
		t.Fatalf("wrapMarkdownAsPostForDryRun() desc = %q, want placeholder note", desc)
	}
}

func TestWrapMarkdownAsPostForDryRun_SegmentedBlankLines(t *testing.T) {
	content, _ := wrapMarkdownAsPostForDryRun("hello\n\n![alt](https://example.com/a.png)")
	if !strings.Contains(content, `![alt](img_dryrun_1)`) {
		t.Fatalf("wrapMarkdownAsPostForDryRun(segmented) content = %q, want placeholder img key", content)
	}
	if !strings.Contains(content, `"tag":"text"`) {
		t.Fatalf("wrapMarkdownAsPostForDryRun(segmented) content = %q, want blank-line text paragraph", content)
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
	content := decodePostContentForTest(t, got)
	if len(content) != 1 {
		t.Fatalf("wrapMarkdownAsPost() content len = %d, want 1", len(content))
	}
	node := decodePostParagraphForTest(t, got, 0)
	if node["tag"] != "md" {
		t.Fatalf("wrapMarkdownAsPost() tag = %#v, want md", node["tag"])
	}
	if node["text"] != "hello **world**" {
		t.Fatalf("wrapMarkdownAsPost() text = %#v, want %q", node["text"], "hello **world**")
	}
}

func TestShouldUseSegmentedPost(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		want     bool
	}{
		{name: "single newline", markdown: "a\nb", want: false},
		{name: "blank line", markdown: "a\n\nb", want: true},
		{name: "blank line with spaces", markdown: "a\n  \nb", want: true},
		{name: "multiple blank lines", markdown: "a\n \n \n b", want: true},
		{name: "blank lines inside code block only", markdown: "```go\n\n\nfmt.Println(1)\n```\nnext", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldUseSegmentedPost(tt.markdown); got != tt.want {
				t.Fatalf("shouldUseSegmentedPost(%q) = %v, want %v", tt.markdown, got, tt.want)
			}
		})
	}
}

func TestWrapMarkdownAsPost_SegmentedBlankLines(t *testing.T) {
	got := wrapMarkdownAsPost("a\n\nb")
	content := decodePostContentForTest(t, got)
	if len(content) != 3 {
		t.Fatalf("wrapMarkdownAsPost(a\\n\\nb) content len = %d, want 3", len(content))
	}

	first := decodePostParagraphForTest(t, got, 0)
	if first["tag"] != "md" || first["text"] != "a" {
		t.Fatalf("first paragraph = %#v, want md/a", first)
	}

	second := decodePostParagraphForTest(t, got, 1)
	if second["tag"] != "text" || second["text"] != postBlankLinePlaceholder {
		t.Fatalf("second paragraph = %#v, want blank text placeholder", second)
	}

	third := decodePostParagraphForTest(t, got, 2)
	if third["tag"] != "md" || third["text"] != "b" {
		t.Fatalf("third paragraph = %#v, want md/b", third)
	}
}

func TestWrapMarkdownAsPost_SegmentedMultipleBlankLines(t *testing.T) {
	got := wrapMarkdownAsPost("a\n\n\nb")
	content := decodePostContentForTest(t, got)
	if len(content) != 4 {
		t.Fatalf("wrapMarkdownAsPost(a\\n\\n\\nb) content len = %d, want 4", len(content))
	}

	for i := 1; i <= 2; i++ {
		node := decodePostParagraphForTest(t, got, i)
		if node["tag"] != "text" || node["text"] != postBlankLinePlaceholder {
			t.Fatalf("blank paragraph %d = %#v, want blank text placeholder", i, node)
		}
	}
}

func TestWrapMarkdownAsPost_SegmentedBlankLinesWithSpaces(t *testing.T) {
	got := wrapMarkdownAsPost("a\n  \nb")
	content := decodePostContentForTest(t, got)
	if len(content) != 3 {
		t.Fatalf("wrapMarkdownAsPost(a\\n  \\nb) content len = %d, want 3", len(content))
	}
	node := decodePostParagraphForTest(t, got, 1)
	if node["tag"] != "text" || node["text"] != postBlankLinePlaceholder {
		t.Fatalf("middle paragraph = %#v, want blank text placeholder", node)
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
		{name: "absolute path", fileKey: "file_123", outputPath: "/tmp/out.bin", wantErr: "absolute paths are not allowed"},
		{name: "parent escape", fileKey: "file_123", outputPath: "../out.bin", wantErr: "path cannot escape the current working directory"},
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

	_, _, err := downloadIMResourceToPath(context.Background(), runtime, "om_123", "img_123", "image", "out.bin")
	if err == nil || !strings.Contains(err.Error(), "http client unavailable") {
		t.Fatalf("downloadIMResourceToPath() error = %v", err)
	}
}

func TestParseTotalSize(t *testing.T) {
	tests := []struct {
		name         string
		contentRange string
		want         int64
		wantErr      string
	}{
		{name: "normal", contentRange: "bytes 0-131071/104857600", want: 104857600},
		{name: "single probe chunk", contentRange: "bytes 0-131071/131072", want: 131072},
		{name: "single small chunk", contentRange: "bytes 0-15/16", want: 16},
		{name: "empty", contentRange: "", wantErr: "content-range is empty"},
		{name: "invalid prefix", contentRange: "items 0-15/16", wantErr: `unsupported content-range: "items 0-15/16"`},
		{name: "missing total", contentRange: "bytes 0-15/", wantErr: `unsupported content-range: "bytes 0-15/"`},
		{name: "wildcard", contentRange: "bytes */16", wantErr: `unsupported content-range: "bytes */16"`},
		{name: "unknown total size", contentRange: "bytes 0-99/*", wantErr: `unknown total size in content-range: "bytes 0-99/*"`},
		{name: "invalid total", contentRange: "bytes 0-15/not-a-number", wantErr: "parse total size:"},
		{name: "zero total size", contentRange: "bytes 0-0/0", wantErr: "invalid total size: 0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTotalSize(tt.contentRange)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseTotalSize() error = %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseTotalSize() unexpected error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseTotalSize() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestShortcuts(t *testing.T) {
	var commands []string
	for _, shortcut := range Shortcuts() {
		commands = append(commands, shortcut.Command)
	}

	want := []string{
		"+chat-create",
		"+chat-messages-list",
		"+chat-search",
		"+chat-update",
		"+messages-mget",
		"+messages-reply",
		"+messages-resources-download",
		"+messages-search",
		"+messages-send",
		"+threads-messages-list",
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("Shortcuts() commands = %#v, want %#v", commands, want)
	}
}
