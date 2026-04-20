// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/spf13/cobra"
)

// normalizeAtMentions fixes common AI mistakes in @mention tags.
var mentionFixRe = regexp.MustCompile(`<at\s+(id|open_id|user_id)=("?)([^"\s/>]+)"?\s*/?>`)
var threadIDRe = regexp.MustCompile(`^omt_`)
var messageIDRe = regexp.MustCompile(`^om_`)

func normalizeAtMentions(content string) string {
	return mentionFixRe.ReplaceAllString(content, `<at user_id="$3">`)
}

// buildMGetURL constructs the mget query URL for batch-fetching messages.
// Uses repeated params (?message_ids=x&message_ids=y) — RFC 6570 standard array
// encoding, shorter and more broadly compatible than indexed params ([0]=x).
func buildMGetURL(ids []string) string {
	parts := make([]string, 0, len(ids)+1)
	parts = append(parts, "card_msg_content_type=raw_card_content")
	for _, id := range ids {
		parts = append(parts, "message_ids="+url.QueryEscape(id))
	}
	return "/open-apis/im/v1/messages/mget?" + strings.Join(parts, "&")
}

func validateMessageID(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", output.ErrValidation("message ID cannot be empty")
	}
	if !strings.HasPrefix(input, "om_") {
		return "", output.ErrValidation("invalid message ID %q: must start with om_", input)
	}
	return input, nil
}

// buildMediaContentFromKey builds (msgType, contentJSON) for DryRun purposes from flag values.
// Local paths and URLs are represented with placeholder keys because DryRun does not upload media.
func buildMediaContentFromKey(text, imageKey, fileKey, videoKey, videoCoverKey, audioKey string) (msgType, content, desc string) {
	if text != "" {
		jsonBytes, _ := json.Marshal(map[string]string{"text": text})
		return "text", string(jsonBytes), ""
	}
	if videoKey != "" {
		coverKey := videoCoverKey
		if !isMediaKey(coverKey) {
			coverKey = "img_dryrun_upload"
		}
		fk := videoKey
		var d string
		if !isMediaKey(videoKey) {
			fk = "file_dryrun_upload"
			d = dryRunMediaUploadDesc("--video", videoKey)
		}
		if videoCoverKey != "" && !isMediaKey(videoCoverKey) {
			if d != "" {
				d += "; "
			}
			d += dryRunMediaUploadDesc("--video-cover", videoCoverKey)
		}
		jsonBytes, _ := json.Marshal(map[string]string{"file_key": fk, "image_key": coverKey})
		return "media", string(jsonBytes), d
	}
	if imageKey != "" {
		if !isMediaKey(imageKey) {
			jsonBytes, _ := json.Marshal(map[string]string{"image_key": "img_dryrun_upload"})
			return "image", string(jsonBytes), dryRunMediaUploadDesc("--image", imageKey)
		}
		jsonBytes, _ := json.Marshal(map[string]string{"image_key": imageKey})
		return "image", string(jsonBytes), ""
	}
	if fileKey != "" {
		if !isMediaKey(fileKey) {
			jsonBytes, _ := json.Marshal(map[string]string{"file_key": "file_dryrun_upload"})
			return "file", string(jsonBytes), dryRunMediaUploadDesc("--file", fileKey)
		}
		jsonBytes, _ := json.Marshal(map[string]string{"file_key": fileKey})
		return "file", string(jsonBytes), ""
	}
	if audioKey != "" {
		if !isMediaKey(audioKey) {
			jsonBytes, _ := json.Marshal(map[string]string{"file_key": "file_dryrun_upload"})
			return "audio", string(jsonBytes), dryRunMediaUploadDesc("--audio", audioKey)
		}
		jsonBytes, _ := json.Marshal(map[string]string{"file_key": audioKey})
		return "audio", string(jsonBytes), ""
	}
	return "", "", ""
}

// isURL returns true if the value looks like an http/https URL.
func isURL(value string) bool {
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
}

func dryRunMediaUploadDesc(flagName, value string) string {
	source := "local file"
	if isURL(value) {
		source = "URL"
	}
	return fmt.Sprintf("dry-run uses placeholder media keys for %s %s input; execution uploads it before sending", flagName, source)
}

// fileNameFromURL extracts a filename from a URL path, falling back to "download".
func fileNameFromURL(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil {
		if u.Scheme != "http" && u.Scheme != "https" {
			return "download"
		}
		base := path.Base(u.Path)
		if base != "" && base != "." && base != "/" {
			return base
		}
	}
	return "download"
}

func sanitizeURLForDisplay(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u == nil {
		return "[redacted-url]"
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return "[redacted-url]"
	}
	base := path.Base(u.Path)
	if base == "" || base == "." || base == "/" {
		base = "download"
	}
	return host + "/" + base
}

// startURLDownload performs URL validation, creates an HTTP client, and sends a
// GET request. It returns the response (with Body still open) and the file
// extension inferred from the URL. The caller must close resp.Body.
func startURLDownload(ctx context.Context, runtime *common.RuntimeContext, rawURL string) (*http.Response, string, error) {
	if err := validate.ValidateDownloadSourceURL(ctx, rawURL); err != nil {
		return nil, "", fmt.Errorf("blocked URL: %w", err)
	}

	httpClient, err := runtime.Factory.HttpClient()
	if err != nil {
		return nil, "", fmt.Errorf("http client: %w", err)
	}
	httpClient = validate.NewDownloadHTTPClient(httpClient, validate.DownloadHTTPClientOptions{
		AllowHTTP: true,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("invalid URL: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("download failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, "", fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	ext := filepath.Ext(fileNameFromURL(rawURL))
	return resp, ext, nil
}

// downloadURLToReader returns a size-limited io.ReadCloser for the URL content
// and the file extension inferred from the URL. The caller must close the
// returned ReadCloser. No temp file is created and the content is not buffered.
func downloadURLToReader(ctx context.Context, runtime *common.RuntimeContext, rawURL string, maxSize int64) (io.ReadCloser, string, error) {
	resp, ext, err := startURLDownload(ctx, runtime, rawURL) //nolint:bodyclose // resp.Body is closed by the returned limitedReadCloser
	if err != nil {
		return nil, "", err
	}
	lr := &limitedReadCloser{
		r:      io.LimitReader(resp.Body, maxSize+1),
		closer: resp.Body,
		max:    maxSize,
	}
	return lr, ext, nil
}

// limitedReadCloser wraps a LimitReader and checks for size overflow on Close.
type limitedReadCloser struct {
	r      io.Reader
	closer io.Closer
	max    int64
	n      int64
}

func (l *limitedReadCloser) Read(p []byte) (int, error) {
	n, err := l.r.Read(p)
	l.n += int64(n)
	if l.n > l.max {
		return n, fmt.Errorf("download exceeds size limit (max %s)", common.FormatSize(l.max))
	}
	return n, err
}

func (l *limitedReadCloser) Close() error {
	return l.closer.Close()
}

// mediaKind distinguishes image uploads (image_key) from file uploads (file_key).
type mediaKind int

const (
	mediaKindImage mediaKind = iota // upload via image API, returns image_key
	mediaKindFile                   // upload via file API, returns file_key
)

// mediaSpec describes how to resolve and upload a single media input.
type mediaSpec struct {
	value        string    // raw input value (path, URL, or media key)
	flagName     string    // CLI flag name for log messages, e.g. "--image"
	mediaType    string    // human label for errors, e.g. "image"
	msgType      string    // IM message type, e.g. "image", "file", "audio"
	kind         mediaKind // image vs file upload
	maxSize      int64     // download size limit
	withDuration bool      // whether to parse audio/video duration
	resultKey    string    // JSON key for the upload result, e.g. "image_key"
}

// resolveMediaContent resolves text/media flags to (msgType, contentJSON) for Execute.
// For URL inputs, download failures fall back to sending the URL as a text link.
func resolveMediaContent(ctx context.Context, runtime *common.RuntimeContext, text, imageVal, fileVal, videoVal, videoCoverVal, audioVal string) (msgType, content string, err error) {
	if text != "" {
		jsonBytes, _ := json.Marshal(map[string]string{"text": text})
		return "text", string(jsonBytes), nil
	}

	// Video is special: it produces two keys (file_key + image_key for cover).
	if videoVal != "" {
		return resolveVideoContent(ctx, runtime, videoVal, videoCoverVal)
	}

	// All other media types follow a uniform pattern: single input → single key.
	specs := []mediaSpec{
		{value: imageVal, flagName: "--image", mediaType: "image", msgType: "image", kind: mediaKindImage, maxSize: maxImageUploadSize, resultKey: "image_key"},
		{value: fileVal, flagName: "--file", mediaType: "file", msgType: "file", kind: mediaKindFile, maxSize: maxFileUploadSize, resultKey: "file_key"},
		{value: audioVal, flagName: "--audio", mediaType: "audio", msgType: "audio", kind: mediaKindFile, maxSize: maxFileUploadSize, withDuration: true, resultKey: "file_key"},
	}

	for _, s := range specs {
		if s.value == "" {
			continue
		}
		key, resolveErr := resolveOneMedia(ctx, runtime, s)
		if resolveErr != nil {
			return mediaFallbackOrError(s.value, s.mediaType, resolveErr)
		}
		jsonBytes, _ := json.Marshal(map[string]string{s.resultKey: key})
		return s.msgType, string(jsonBytes), nil
	}
	return "", "", nil
}

// resolveOneMedia uploads a single media input (image, file, or audio) and
// returns the resulting key. It handles media keys, URLs, and local paths.
func resolveOneMedia(ctx context.Context, runtime *common.RuntimeContext, s mediaSpec) (string, error) {
	if isMediaKey(s.value) {
		return s.value, nil
	}

	if isURL(s.value) {
		return resolveURLMedia(ctx, runtime, s)
	}
	return resolveLocalMedia(ctx, runtime, s)
}

// resolveURLMedia downloads a URL and uploads it.
func resolveURLMedia(ctx context.Context, runtime *common.RuntimeContext, s mediaSpec) (string, error) {
	fmt.Fprintf(runtime.IO().ErrOut, "downloading %s: %s\n", s.flagName, sanitizeURLForDisplay(s.value))

	if s.kind == mediaKindImage {
		rc, _, err := downloadURLToReader(ctx, runtime, s.value, s.maxSize)
		if err != nil {
			return "", err
		}
		defer rc.Close()
		fmt.Fprintf(runtime.IO().ErrOut, "uploading %s\n", s.mediaType)
		return uploadImageFromReader(ctx, runtime, rc, "message")
	}

	// File-kind: buffer in memory for possible duration parsing.
	mb, err := newMediaBuffer(ctx, runtime, s.value, s.maxSize)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(runtime.IO().ErrOut, "uploading %s: %s\n", s.mediaType, mb.FileName())
	dur := ""
	if s.withDuration {
		dur = mb.Duration()
	}
	return uploadFileFromReader(ctx, runtime, mb.Reader(), mb.FileName(), mb.FileType(), dur)
}

// resolveLocalMedia uploads a local file.
func resolveLocalMedia(ctx context.Context, runtime *common.RuntimeContext, s mediaSpec) (string, error) {
	fmt.Fprintf(runtime.IO().ErrOut, "uploading %s: %s\n", s.mediaType, filepath.Base(s.value))

	if s.kind == mediaKindImage {
		return uploadImageToIM(ctx, runtime, s.value, "message")
	}

	ft := detectIMFileType(s.value)
	dur := ""
	if s.withDuration {
		dur = parseMediaDuration(runtime, s.value, ft)
	}
	return uploadFileToIM(ctx, runtime, s.value, ft, dur)
}

// resolveVideoContent handles the video case which needs both a file_key and
// a cover image_key.
func resolveVideoContent(ctx context.Context, runtime *common.RuntimeContext, videoVal, videoCoverVal string) (string, string, error) {
	videoSpec := mediaSpec{
		value: videoVal, flagName: "--video", mediaType: "video",
		kind: mediaKindFile, maxSize: maxFileUploadSize, withDuration: true, resultKey: "file_key",
	}
	fKey, err := resolveOneMedia(ctx, runtime, videoSpec)
	if err != nil {
		return mediaFallbackOrError(videoVal, "video", err)
	}

	coverSpec := mediaSpec{
		value: videoCoverVal, flagName: "--video-cover", mediaType: "cover image",
		kind: mediaKindImage, maxSize: maxImageUploadSize, resultKey: "image_key",
	}
	coverKey, err := resolveOneMedia(ctx, runtime, coverSpec)
	if err != nil {
		return "", "", fmt.Errorf("cover image upload failed: %w", err)
	}

	jsonBytes, _ := json.Marshal(map[string]string{"file_key": fKey, "image_key": coverKey})
	return "media", string(jsonBytes), nil
}

// mediaFallbackOrError returns a text fallback for URL inputs when upload fails,
// or a hard error for local file inputs.
func mediaFallbackOrError(originalValue, mediaType string, uploadErr error) (string, string, error) {
	if isURL(originalValue) {
		// Fallback: send URL as text link instead of failing.
		fallbackText := fmt.Sprintf("[%s upload failed, sending link] %s", mediaType, originalValue)
		jsonBytes, _ := json.Marshal(map[string]string{"text": fallbackText})
		return "text", string(jsonBytes), nil
	}
	return "", "", fmt.Errorf("%s upload failed: %w", mediaType, uploadErr)
}

// resolveP2PChatID resolves user open_id to P2P chat_id.
func resolveP2PChatID(runtime *common.RuntimeContext, openID string) (string, error) {
	if runtime.IsBot() {
		return "", output.Errorf(output.ExitValidation, "validation", "--user-id requires user identity (--as user); use --chat-id when calling with bot identity")
	}
	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodPost,
		ApiPath:    "/open-apis/im/v1/chat_p2p/batch_query",
		QueryParams: larkcore.QueryParams{
			"chatter_id_type": []string{"open_id"},
		},
		Body: map[string]interface{}{"chatter_ids": []string{openID}},
	})
	if err != nil {
		return "", err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(apiResp.RawBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse chat_p2p response: %w", err)
	}
	data, _ := result["data"].(map[string]interface{})

	chats, _ := data["p2p_chats"].([]interface{})
	for _, item := range chats {
		chat, _ := item.(map[string]interface{})
		chatID, _ := chat["chat_id"].(string)
		if chatID != "" {
			return chatID, nil
		}
	}

	return "", output.Errorf(output.ExitAPI, "not_found", "P2P chat not found for this user")
}

// resolveThreadID normalizes a message ID to its thread ID when possible.
func resolveThreadID(runtime *common.RuntimeContext, id string) (string, error) {
	if threadIDRe.MatchString(id) {
		return id, nil
	}
	if !messageIDRe.MatchString(id) {
		return "", output.Errorf(output.ExitValidation, "validation", "invalid thread ID format: must start with om_ or omt_")
	}

	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodGet,
		ApiPath:    "/open-apis/im/v1/messages/" + validate.EncodePathSegment(id),
	})
	if err != nil {
		return "", err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(apiResp.RawBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse message response: %w", err)
	}
	data, _ := result["data"].(map[string]interface{})

	items, _ := data["items"].([]interface{})
	for _, item := range items {
		msg, _ := item.(map[string]interface{})
		threadID, _ := msg["thread_id"].(string)
		if threadID != "" {
			return threadID, nil
		}
	}

	return "", output.Errorf(output.ExitAPI, "not_found", "thread ID not found for this message")
}

// parseOggOpusDuration parses the duration in milliseconds from an OGG/Opus
// buffer. Scans backward for the last OggS page header, reads the granule
// position, and divides by 48 000 (Opus standard sample rate).
// Returns 0 on any parse failure.
func parseOggOpusDuration(data []byte) int64 {
	offset := -1
	for i := len(data) - 4; i >= 0; i-- {
		if data[i] == 'O' && data[i+1] == 'g' && data[i+2] == 'g' && data[i+3] == 'S' {
			offset = i
			break
		}
	}
	if offset < 0 {
		return 0
	}
	granuleOffset := offset + 6
	if granuleOffset+8 > len(data) {
		return 0
	}
	lo := binary.LittleEndian.Uint32(data[granuleOffset:])
	hi := binary.LittleEndian.Uint32(data[granuleOffset+4:])
	granule := uint64(hi)<<32 | uint64(lo)
	if granule == 0 {
		return 0
	}
	return int64(math.Ceil(float64(granule)/48000.0)) * 1000
}

// parseMp4Duration parses the duration in milliseconds from an MP4 buffer.
// Locates the moov→mvhd box and reads timescale + duration fields.
// Returns 0 on any parse failure.
func parseMp4Duration(data []byte) int64 {
	moovStart, moovEnd := findMP4Box(data, 0, len(data), "moov")
	if moovStart < 0 {
		return 0
	}
	mvhdStart, mvhdEnd := findMP4Box(data, moovStart, moovEnd, "mvhd")
	if mvhdStart < 0 {
		return 0
	}
	return parseMvhdPayload(data[mvhdStart:mvhdEnd])
}

// parseMvhdPayload extracts duration in milliseconds from the raw mvhd box
// payload. Supports version 0 (32-bit fields) and version 1 (64-bit fields).
func parseMvhdPayload(data []byte) int64 {
	if len(data) < 1 {
		return 0
	}
	version := data[0]
	var timescale, duration uint64
	if version == 0 {
		if len(data) < 20 {
			return 0
		}
		timescale = uint64(binary.BigEndian.Uint32(data[12:]))
		duration = uint64(binary.BigEndian.Uint32(data[16:]))
	} else {
		if len(data) < 32 {
			return 0
		}
		timescale = uint64(binary.BigEndian.Uint32(data[20:]))
		duration = binary.BigEndian.Uint64(data[24:])
	}
	if timescale == 0 || duration == 0 {
		return 0
	}
	return int64(math.Round(float64(duration) / float64(timescale) * 1000))
}

// findMP4Box locates a box by its 4-char type within [start, end) of data.
// Returns (dataStart, dataEnd) after the box header, or (-1, -1) if not found.
func findMP4Box(data []byte, start, end int, boxType string) (int, int) {
	offset := start
	for offset+8 <= end {
		size := int(binary.BigEndian.Uint32(data[offset:]))
		typ := string(data[offset+4 : offset+8])
		var boxEnd, dataStart int
		switch {
		case size == 0:
			boxEnd = end
			dataStart = offset + 8
		case size == 1:
			if offset+16 > end {
				return -1, -1
			}
			boxEnd = int(binary.BigEndian.Uint64(data[offset+8:]))
			dataStart = offset + 16
		default:
			if size < 8 {
				return -1, -1
			}
			boxEnd = offset + size
			dataStart = offset + 8
		}
		if typ == boxType {
			if boxEnd > end {
				boxEnd = end
			}
			return dataStart, boxEnd
		}
		offset = boxEnd
	}
	return -1, -1
}

// parseMediaDuration opens a file and returns the duration string (in ms)
// for audio/video uploads. Only reads the minimal portion of the file needed
// for parsing (tail for OGG, box headers + moov for MP4).
// Returns "" if parsing fails or the file type is not audio/video.
func parseMediaDuration(runtime *common.RuntimeContext, filePath, fileType string) string {
	if fileType != "opus" && fileType != "mp4" {
		return ""
	}
	info, err := runtime.FileIO().Stat(filePath)
	if err != nil || info.Size() == 0 {
		return ""
	}
	f, err := runtime.FileIO().Open(filePath)
	if err != nil {
		return ""
	}

	var ms int64
	if fileType == "opus" {
		ms = readOggDuration(f, info.Size())
	} else {
		ms = readMp4Duration(f, info.Size())
	}
	if ms <= 0 {
		return ""
	}
	return strconv.FormatInt(ms, 10)
}

// mediaBuffer holds downloaded media content in memory, providing both random
// access (for duration parsing) and an io.Reader (for upload). It replaces temp
// files for URL-sourced media that needs seek-like access before upload.
type mediaBuffer struct {
	data []byte
	ext  string // file extension including leading dot, e.g. ".mp4"
	name string // original file name extracted from the source URL
}

// newMediaBuffer downloads URL content into memory via downloadURLToReader.
func newMediaBuffer(ctx context.Context, runtime *common.RuntimeContext, rawURL string, maxSize int64) (*mediaBuffer, error) {
	rc, ext, err := downloadURLToReader(ctx, runtime, rawURL, maxSize)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}
	return newMediaBufferFromBytes(data, ext, rawURL), nil
}

// newMediaBufferFromBytes builds a mediaBuffer from already-downloaded bytes.
// Split out from newMediaBuffer so the URL-to-filename wiring is testable
// without going through the hardened download transport.
func newMediaBufferFromBytes(data []byte, ext, rawURL string) *mediaBuffer {
	return &mediaBuffer{data: data, ext: ext, name: fileNameFromURL(rawURL)}
}

// Reader returns a new io.Reader over the buffered data. Each call returns a
// fresh reader starting from the beginning, so the buffer can be read multiple
// times (once for duration parsing, once for upload).
func (b *mediaBuffer) Reader() io.Reader {
	return bytes.NewReader(b.data)
}

// FileName returns the original file name extracted from the source URL.
func (b *mediaBuffer) FileName() string {
	return b.name
}

// FileType returns the IM file type detected from the extension.
func (b *mediaBuffer) FileType() string {
	return detectIMFileType("file" + b.ext)
}

// Duration parses audio/video duration from the buffered data.
func (b *mediaBuffer) Duration() string {
	ft := b.FileType()
	if ft != "opus" && ft != "mp4" {
		return ""
	}
	if len(b.data) == 0 {
		return ""
	}
	var ms int64
	if ft == "opus" {
		ms = readOggDurationBytes(b.data)
	} else {
		ms = readMp4DurationBytes(b.data)
	}
	if ms <= 0 {
		return ""
	}
	return strconv.FormatInt(ms, 10)
}

// readOggDurationBytes parses OGG duration from the tail of in-memory data.
func readOggDurationBytes(data []byte) int64 {
	const maxTail = 65536
	buf := data
	if len(buf) > maxTail {
		buf = buf[len(buf)-maxTail:]
	}
	return parseOggOpusDuration(buf)
}

// readMp4DurationBytes walks top-level MP4 boxes in memory to find moov/mvhd duration.
func readMp4DurationBytes(data []byte) int64 {
	fileSize := int64(len(data))
	var offset int64
	for offset+8 <= fileSize {
		size := int64(binary.BigEndian.Uint32(data[offset : offset+4]))
		typ := string(data[offset+4 : offset+8])

		var boxEnd, dataStart int64
		switch {
		case size == 0:
			boxEnd = fileSize
			dataStart = offset + 8
		case size == 1:
			if offset+16 > fileSize {
				return 0
			}
			boxEnd = int64(binary.BigEndian.Uint64(data[offset+8 : offset+16]))
			dataStart = offset + 16
		case size < 8:
			return 0
		default:
			boxEnd = offset + size
			dataStart = offset + 8
		}

		if typ == "moov" {
			moovLen := boxEnd - dataStart
			if moovLen <= 0 || moovLen > 10<<20 || dataStart+moovLen > fileSize {
				return 0
			}
			moov := data[dataStart : dataStart+moovLen]
			mvhdStart, mvhdEnd := findMP4Box(moov, 0, len(moov), "mvhd")
			if mvhdStart < 0 {
				return 0
			}
			return parseMvhdPayload(moov[mvhdStart:mvhdEnd])
		}
		offset = boxEnd
	}
	return 0
}

// readOggDuration reads the tail of an OGG file (up to 64 KB) and parses duration.
func readOggDuration(f fileio.File, fileSize int64) int64 {
	const maxTail = 65536
	readSize := fileSize
	if readSize > maxTail {
		readSize = maxTail
	}
	buf := make([]byte, readSize)
	if _, err := f.ReadAt(buf, fileSize-readSize); err != nil {
		return 0
	}
	return parseOggOpusDuration(buf)
}

// readMp4Duration walks top-level MP4 boxes via file seeks to find moov,
// then reads only the moov content to locate mvhd and extract the duration.
func readMp4Duration(f fileio.File, fileSize int64) int64 {
	hdr := make([]byte, 16)
	var offset int64
	for offset+8 <= fileSize {
		if _, err := f.ReadAt(hdr[:8], offset); err != nil {
			return 0
		}
		size := int64(binary.BigEndian.Uint32(hdr[0:4]))
		typ := string(hdr[4:8])

		var boxEnd, dataStart int64
		switch {
		case size == 0:
			boxEnd = fileSize
			dataStart = offset + 8
		case size == 1:
			if _, err := f.ReadAt(hdr[8:16], offset+8); err != nil {
				return 0
			}
			boxEnd = int64(binary.BigEndian.Uint64(hdr[8:16]))
			dataStart = offset + 16
		case size < 8:
			return 0
		default:
			boxEnd = offset + size
			dataStart = offset + 8
		}

		if typ == "moov" {
			moovLen := boxEnd - dataStart
			if moovLen <= 0 || moovLen > 10<<20 {
				return 0
			}
			moov := make([]byte, moovLen)
			if _, err := f.ReadAt(moov, dataStart); err != nil {
				return 0
			}
			mvhdStart, mvhdEnd := findMP4Box(moov, 0, len(moov), "mvhd")
			if mvhdStart < 0 {
				return 0
			}
			return parseMvhdPayload(moov[mvhdStart:mvhdEnd])
		}
		offset = boxEnd
	}
	return 0
}

// optimizeMarkdownStyle optimizes markdown text for Feishu post rendering.
// Ported from an internal markdown-style implementation.
//
// Steps:
//  1. Extract code blocks with placeholders to protect them
//  2. Downgrade headings: H1 → H4, H2~H6 → H5 (only when H1~H3 present)
//  3. Normalize spacing between consecutive headings and tables with blank lines
//  4. Restore code blocks
//  5. Compress excess blank lines
//  6. Strip invalid image references (keep only img_xxx keys)
var (
	reH2toH6             = regexp.MustCompile(`(?m)^#{2,6} (.+)$`)
	reH1                 = regexp.MustCompile(`(?m)^# (.+)$`)
	reHasH1toH3          = regexp.MustCompile(`(?m)^#{1,3} `)
	reConsecH            = regexp.MustCompile(`(?m)^(#{4,5} .+)\n{1,2}(#{4,5} )`)
	reTableNoGap         = regexp.MustCompile(`(?m)^([^|\n].*)\n(\|.+\|)`)
	reTableAfter         = regexp.MustCompile(`(?m)((?:^\|.+\|[^\S\n]*\n?)+)`)
	reExcessNL           = regexp.MustCompile(`\n{3,}`)
	reInvalidImg         = regexp.MustCompile(`!\[[^\]]*\]\(([^)\s]+)\)`)
	reCodeBlock          = regexp.MustCompile("```[\\s\\S]*?```")
	reBlankLineSeparator = regexp.MustCompile(`\n(?:[ \t]*\n)+`)
)

const (
	markdownCodeBlockPlaceholder = "___CB_"
	postBlankLinePlaceholder     = "\u200B"
)

type markdownPart struct {
	text         string
	newlineCount int
	isSeparator  bool
}

func protectMarkdownCodeBlocks(text string) (string, []string) {
	var codeBlocks []string
	protected := reCodeBlock.ReplaceAllStringFunc(text, func(m string) string {
		idx := len(codeBlocks)
		codeBlocks = append(codeBlocks, m)
		return fmt.Sprintf("%s%d___", markdownCodeBlockPlaceholder, idx)
	})
	return protected, codeBlocks
}

func restoreMarkdownCodeBlocks(text string, codeBlocks []string) string {
	restored := text
	for i, block := range codeBlocks {
		restored = strings.Replace(restored, fmt.Sprintf("%s%d___", markdownCodeBlockPlaceholder, i), block, 1)
	}
	return restored
}

func optimizeMarkdownStyle(text string) string {
	r, codeBlocks := protectMarkdownCodeBlocks(text)

	// Only downgrade when original text has H1~H3; order matters (H2~H6 first).
	if reHasH1toH3.MatchString(text) {
		r = reH2toH6.ReplaceAllString(r, "##### $1")
		r = reH1.ReplaceAllString(r, "#### $1")
	}

	r = reConsecH.ReplaceAllString(r, "$1\n\n$2")

	r = reTableNoGap.ReplaceAllString(r, "$1\n\n$2")
	r = reTableAfter.ReplaceAllString(r, "$1\n")

	r = restoreMarkdownCodeBlocks(r, codeBlocks)

	r = reExcessNL.ReplaceAllString(r, "\n\n")

	if strings.Contains(r, "![") {
		r = reInvalidImg.ReplaceAllStringFunc(r, func(m string) string {
			// Extract the URL from ![alt](URL) — it starts after "(" and ends before ")"
			start := strings.LastIndex(m, "(")
			end := strings.LastIndex(m, ")")
			if start >= 0 && end > start && strings.HasPrefix(m[start+1:end], "img_") {
				return m
			}
			return ""
		})
	}

	return r
}

func shouldUseSegmentedPost(markdown string) bool {
	protected, _ := protectMarkdownCodeBlocks(markdown)
	return reBlankLineSeparator.MatchString(protected)
}

func splitMarkdownByBlankLines(markdown string) []markdownPart {
	protected, codeBlocks := protectMarkdownCodeBlocks(markdown)
	locs := reBlankLineSeparator.FindAllStringIndex(protected, -1)
	if len(locs) == 0 {
		return []markdownPart{{text: markdown}}
	}

	parts := make([]markdownPart, 0, len(locs)*2+1)
	last := 0
	for _, loc := range locs {
		if loc[0] > last {
			content := restoreMarkdownCodeBlocks(protected[last:loc[0]], codeBlocks)
			if content != "" {
				parts = append(parts, markdownPart{text: content})
			}
		}
		separator := protected[loc[0]:loc[1]]
		parts = append(parts, markdownPart{
			isSeparator:  true,
			newlineCount: strings.Count(separator, "\n"),
		})
		last = loc[1]
	}

	if last < len(protected) {
		content := restoreMarkdownCodeBlocks(protected[last:], codeBlocks)
		if content != "" {
			parts = append(parts, markdownPart{text: content})
		}
	}

	if len(parts) == 0 {
		return []markdownPart{{text: markdown}}
	}
	return parts
}

func marshalMarkdownPostContent(content [][]map[string]interface{}) string {
	payload := map[string]interface{}{
		"zh_cn": map[string]interface{}{
			"content": content,
		},
	}
	data, _ := json.Marshal(payload)
	return string(data)
}

func buildSingleMDPost(markdown string) string {
	return marshalMarkdownPostContent([][]map[string]interface{}{
		{{
			"tag":  "md",
			"text": optimizeMarkdownStyle(markdown),
		}},
	})
}

func buildSegmentedPost(markdown string) string {
	parts := splitMarkdownByBlankLines(markdown)
	content := make([][]map[string]interface{}, 0, len(parts))
	for _, part := range parts {
		if part.isSeparator {
			for i := 1; i < part.newlineCount; i++ {
				content = append(content, []map[string]interface{}{{
					"tag":  "text",
					"text": postBlankLinePlaceholder,
				}})
			}
			continue
		}
		if part.text == "" {
			continue
		}
		optimized := strings.Trim(optimizeMarkdownStyle(part.text), "\n")
		if optimized == "" {
			continue
		}
		content = append(content, []map[string]interface{}{{
			"tag":  "md",
			"text": optimized,
		}})
	}
	if len(content) == 0 {
		return buildSingleMDPost(markdown)
	}
	return marshalMarkdownPostContent(content)
}

func buildMarkdownPostContent(markdown string) string {
	if shouldUseSegmentedPost(markdown) {
		return buildSegmentedPost(markdown)
	}
	return buildSingleMDPost(markdown)
}

// wrapMarkdownAsPost wraps markdown text into Feishu post format JSON (no network).
// Used by DryRun. Output may include md/text paragraphs when blank-line separators are present.
func wrapMarkdownAsPost(markdown string) string {
	return buildMarkdownPostContent(markdown)
}

var reMarkdownImage = regexp.MustCompile(`!\[[^\]]*\]\((https?://[^)\s]+)\)`)

// wrapMarkdownAsPostForDryRun rewrites remote markdown images to placeholder img_ keys
// so the preview matches the shape of the real request body.
func wrapMarkdownAsPostForDryRun(markdown string) (content, desc string) {
	imageIndex := 0
	rewritten := reMarkdownImage.ReplaceAllStringFunc(markdown, func(m string) string {
		imageIndex++
		sub := reMarkdownImage.FindStringSubmatch(m)
		altStart := strings.Index(m, "[")
		altEnd := strings.Index(m, "]")
		alt := ""
		if altStart >= 0 && altEnd > altStart {
			alt = m[altStart+1 : altEnd]
		}
		if len(sub) < 2 {
			return fmt.Sprintf("![%s](img_dryrun_%d)", alt, imageIndex)
		}
		return fmt.Sprintf("![%s](img_dryrun_%d)", alt, imageIndex)
	})

	desc = ""
	if imageIndex > 0 {
		desc = "dry-run uses placeholder image keys for markdown image URLs; execution downloads and uploads them before sending"
	}
	return wrapMarkdownAsPost(rewritten), desc
}

// resolveMarkdownAsPost resolves image URLs in markdown, applies style optimization,
// and wraps as post format JSON. Used by Execute (makes network calls).
func resolveMarkdownAsPost(ctx context.Context, runtime *common.RuntimeContext, markdown string) string {
	resolved := resolveMarkdownImageURLs(ctx, runtime, markdown)
	return buildMarkdownPostContent(resolved)
}

// resolveMarkdownImageURLs finds ![alt](https://...) in markdown, downloads each URL,
// uploads as image, and replaces with ![alt](img_xxx). Failed uploads are stripped.
func resolveMarkdownImageURLs(ctx context.Context, runtime *common.RuntimeContext, markdown string) string {
	if !strings.Contains(markdown, "![") {
		return markdown
	}
	return reMarkdownImage.ReplaceAllStringFunc(markdown, func(m string) string {
		sub := reMarkdownImage.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		imgURL := sub[1]

		rc, _, err := downloadURLToReader(ctx, runtime, imgURL, maxImageUploadSize)
		if err != nil {
			fmt.Fprintf(runtime.IO().ErrOut, "warning: failed to download image %s: %v\n", sanitizeURLForDisplay(imgURL), err)
			return ""
		}
		defer rc.Close()

		fmt.Fprintf(runtime.IO().ErrOut, "uploading image from URL: %s\n", sanitizeURLForDisplay(imgURL))
		imgKey, err := uploadImageFromReader(ctx, runtime, rc, "message")
		if err != nil {
			fmt.Fprintf(runtime.IO().ErrOut, "warning: failed to upload image %s: %v\n", sanitizeURLForDisplay(imgURL), err)
			return ""
		}

		// Reconstruct ![alt](img_xxx)
		altStart := strings.Index(m, "[")
		altEnd := strings.Index(m, "]")
		alt := ""
		if altStart >= 0 && altEnd > altStart {
			alt = m[altStart+1 : altEnd]
		}
		return fmt.Sprintf("![%s](%s)", alt, imgKey)
	})
}

// validateContentFlags checks mutual exclusion between content flags (text/markdown/content)
// and media flags (image/file/video/audio). Returns an error string or "".
func validateContentFlags(text, markdown, content, imageKey, fileKey, videoKey, videoCoverKey, audioKey string) string {
	mediaCount := 0
	if imageKey != "" {
		mediaCount++
	}
	if fileKey != "" {
		mediaCount++
	}
	if videoKey != "" {
		mediaCount++
	}
	if audioKey != "" {
		mediaCount++
	}
	if mediaCount > 1 {
		return "--image, --file, --video, --audio are mutually exclusive"
	}
	if videoCoverKey != "" && videoKey == "" {
		return "--video-cover can only be used with --video"
	}
	if videoKey != "" && videoCoverKey == "" {
		return "--video-cover is required when using --video (serves as the video cover)"
	}

	contentFlags := 0
	if text != "" {
		contentFlags++
	}
	if markdown != "" {
		contentFlags++
	}
	if content != "" {
		contentFlags++
	}
	if contentFlags > 1 {
		return "--text, --markdown, and --content cannot be specified together"
	}
	if mediaCount > 0 && contentFlags > 0 {
		return "--image/--file/--video/--audio cannot be used with --text, --markdown, or --content"
	}
	if contentFlags == 0 && mediaCount == 0 {
		return "specify --content <json>, --text <plain text>, --markdown <markdown text>, or a media flag (--image/--file/--video/--audio)"
	}
	return ""
}

func validateExplicitMsgType(cmd *cobra.Command, msgType, text, markdown, imageKey, fileKey, videoKey, audioKey string) string {
	if cmd == nil || !cmd.Flags().Changed("msg-type") {
		return ""
	}

	var inferred string
	switch {
	case text != "":
		inferred = "text"
	case markdown != "":
		inferred = "post"
	case imageKey != "":
		inferred = "image"
	case fileKey != "":
		inferred = "file"
	case videoKey != "":
		inferred = "media"
	case audioKey != "":
		inferred = "audio"
	}
	if inferred == "" || msgType == inferred {
		return ""
	}
	return fmt.Sprintf("--msg-type %q conflicts with the inferred message type %q from the selected content flag", msgType, inferred)
}

func detectIMFileType(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".opus", ".ogg":
		return "opus"
	case ".mp4", ".mov", ".avi", ".mkv", ".webm":
		return "mp4"
	case ".pdf":
		return "pdf"
	case ".doc", ".docx":
		return "doc"
	case ".xls", ".xlsx", ".csv":
		return "xls"
	case ".ppt", ".pptx":
		return "ppt"
	default:
		return "stream"
	}
}

const maxImageUploadSize = 5 * 1024 * 1024  // 5MB — Lark API limit for images
const maxFileUploadSize = 100 * 1024 * 1024 // 100MB — Lark API limit for files

func uploadImageToIM(ctx context.Context, runtime *common.RuntimeContext, filePath, imageType string) (string, error) {
	if info, err := runtime.FileIO().Stat(filePath); err == nil && info.Size() > maxImageUploadSize {
		return "", fmt.Errorf("image size %s exceeds limit (max 5MB)", common.FormatSize(info.Size()))
	}

	f, err := runtime.FileIO().Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	fd := larkcore.NewFormdata()
	fd.AddField("image_type", imageType)
	fd.AddFile("image", f)

	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodPost,
		ApiPath:    "/open-apis/im/v1/images",
		Body:       fd,
	}, larkcore.WithFileUpload())
	if err != nil {
		return "", err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(apiResp.RawBody, &result); err != nil {
		return "", fmt.Errorf("parse error: %w", err)
	}

	data, _ := result["data"].(map[string]interface{})
	imageKey, _ := data["image_key"].(string)
	if imageKey == "" {
		return "", fmt.Errorf("image_key not found in response (code: %v, msg: %v)", result["code"], result["msg"])
	}
	return imageKey, nil
}

func uploadFileToIM(ctx context.Context, runtime *common.RuntimeContext, filePath, fileType, duration string) (string, error) {
	if info, err := runtime.FileIO().Stat(filePath); err == nil && info.Size() > maxFileUploadSize {
		return "", fmt.Errorf("file size %s exceeds limit (max 100MB)", common.FormatSize(info.Size()))
	}

	f, err := runtime.FileIO().Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	fd := larkcore.NewFormdata()
	fd.AddField("file_type", fileType)
	fd.AddField("file_name", filepath.Base(filePath))
	if duration != "" {
		fd.AddField("duration", duration)
	}
	fd.AddFile("file", f)

	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodPost,
		ApiPath:    "/open-apis/im/v1/files",
		Body:       fd,
	}, larkcore.WithFileUpload())
	if err != nil {
		return "", err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(apiResp.RawBody, &result); err != nil {
		return "", fmt.Errorf("parse error: %w", err)
	}

	data, _ := result["data"].(map[string]interface{})
	fileKey, _ := data["file_key"].(string)
	if fileKey == "" {
		return "", fmt.Errorf("file_key not found in response (code: %v, msg: %v)", result["code"], result["msg"])
	}
	return fileKey, nil
}

// uploadImageFromReader uploads an image from an io.Reader (no local file needed).
func uploadImageFromReader(ctx context.Context, runtime *common.RuntimeContext, r io.Reader, imageType string) (string, error) {
	fd := larkcore.NewFormdata()
	fd.AddField("image_type", imageType)
	fd.AddFile("image", r)

	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodPost,
		ApiPath:    "/open-apis/im/v1/images",
		Body:       fd,
	}, larkcore.WithFileUpload())
	if err != nil {
		return "", err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(apiResp.RawBody, &result); err != nil {
		return "", fmt.Errorf("parse error: %w", err)
	}

	data, _ := result["data"].(map[string]interface{})
	imageKey, _ := data["image_key"].(string)
	if imageKey == "" {
		return "", fmt.Errorf("image_key not found in response (code: %v, msg: %v)", result["code"], result["msg"])
	}
	return imageKey, nil
}

// uploadFileFromReader uploads a file from an io.Reader (no local file needed).
func uploadFileFromReader(ctx context.Context, runtime *common.RuntimeContext, r io.Reader, fileName, fileType, duration string) (string, error) {
	fd := larkcore.NewFormdata()
	fd.AddField("file_type", fileType)
	fd.AddField("file_name", fileName)
	if duration != "" {
		fd.AddField("duration", duration)
	}
	fd.AddFile("file", r)

	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodPost,
		ApiPath:    "/open-apis/im/v1/files",
		Body:       fd,
	}, larkcore.WithFileUpload())
	if err != nil {
		return "", err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(apiResp.RawBody, &result); err != nil {
		return "", fmt.Errorf("parse error: %w", err)
	}

	data, _ := result["data"].(map[string]interface{})
	fileKey, _ := data["file_key"].(string)
	if fileKey == "" {
		return "", fmt.Errorf("file_key not found in response (code: %v, msg: %v)", result["code"], result["msg"])
	}
	return fileKey, nil
}
