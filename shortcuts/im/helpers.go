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

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/spf13/cobra"
)

// normalizeAtMentions fixes common AI mistakes in @mention tags.
var mentionFixRe = regexp.MustCompile(`<at\s+(id|open_id|user_id)=("?)([^"\s/>]+)"?\s*/?>`)
var threadIDRe = regexp.MustCompile(`^omt_`)
var messageIDRe = regexp.MustCompile(`^om_`)

func flagMessageID(rt *common.RuntimeContext) (string, error) {
	id := strings.TrimSpace(rt.Str("message-id"))
	if id == "" {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--message-id is required").WithParam("--message-id")
	}
	if strings.HasPrefix(id, "omt_") {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument,
			"invalid message ID %q: omt_ prefix is a thread ID, not a message ID; flag operations require om_ message IDs", id).WithParam("--message-id")
	}
	return validateMessageID(id)
}

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
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "message ID cannot be empty").WithParam("--message-id")
	}
	if !strings.HasPrefix(input, "om_") {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid message ID %q: must start with om_", input).WithParam("--message-id")
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
func startURLDownload(ctx context.Context, runtime *common.RuntimeContext, rawURL, param string) (*http.Response, string, error) {
	if err := validate.ValidateDownloadSourceURL(ctx, rawURL); err != nil {
		return nil, "", errs.NewValidationError(errs.SubtypeInvalidArgument, "blocked URL: %v", err).
			WithParam(param).
			WithCause(err)
	}

	httpClient, err := runtime.Factory.HttpClient()
	if err != nil {
		return nil, "", errs.NewInternalError(errs.SubtypeSDKError, "http client: %v", err).WithCause(err)
	}
	httpClient = validate.NewDownloadHTTPClient(httpClient, validate.DownloadHTTPClientOptions{
		AllowHTTP: true,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid URL: %v", err).
			WithParam(param).
			WithCause(err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", wrapIMNetworkErr(err, "download failed")
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, "", errs.NewNetworkError(errs.SubtypeNetworkTransport, "download failed: HTTP %d", resp.StatusCode)
	}

	ext := filepath.Ext(fileNameFromURL(rawURL))
	return resp, ext, nil
}

// downloadURLToReader returns a size-limited io.ReadCloser for the URL content
// and the file extension inferred from the URL. The caller must close the
// returned ReadCloser. No temp file is created and the content is not buffered.
func downloadURLToReader(ctx context.Context, runtime *common.RuntimeContext, rawURL string, maxSize int64, param string) (io.ReadCloser, string, error) {
	resp, ext, err := startURLDownload(ctx, runtime, rawURL, param) //nolint:bodyclose // resp.Body is closed by the returned limitedReadCloser
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
		return n, fmt.Errorf("download exceeds size limit (max %s)", common.FormatSize(l.max)) //nolint:forbidigo // io.Reader.Read contract returns a plain error; classified by the download caller
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
		rc, _, err := downloadURLToReader(ctx, runtime, s.value, s.maxSize, s.flagName)
		if err != nil {
			return "", err
		}
		defer rc.Close()
		fmt.Fprintf(runtime.IO().ErrOut, "uploading %s\n", s.mediaType)
		return uploadImageFromReader(ctx, runtime, rc, "message")
	}

	// File-kind: buffer in memory for possible duration parsing.
	mb, err := newMediaBuffer(ctx, runtime, s.value, s.maxSize, s.flagName)
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
		return uploadImageToIM(ctx, runtime, s.value, "message", s.flagName)
	}

	ft := detectIMFileType(s.value)
	dur := ""
	if s.withDuration {
		dur = parseMediaDuration(runtime, s.value, ft)
	}
	return uploadFileToIM(ctx, runtime, s.value, ft, dur, s.flagName)
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
		return "", "", wrapIMNetworkErr(err, "cover image upload failed")
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
	return "", "", wrapIMNetworkErr(uploadErr, "%s upload failed", mediaType)
}

// resolveP2PChatID resolves user open_id to P2P chat_id.
func resolveP2PChatID(runtime *common.RuntimeContext, openID string) (string, error) {
	if runtime.IsBot() {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--user-id requires user identity (--as user); use --chat-id when calling with bot identity").WithParam("--user-id")
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
	data, err := runtime.ClassifyAPIResponse(apiResp)
	if err != nil {
		return "", err
	}

	chats, _ := data["p2p_chats"].([]interface{})
	for _, item := range chats {
		chat, _ := item.(map[string]interface{})
		chatID, _ := chat["chat_id"].(string)
		if chatID != "" {
			return chatID, nil
		}
	}

	return "", errs.NewAPIError(errs.SubtypeNotFound, "P2P chat not found for this user")
}

// resolveThreadID normalizes a message ID to its thread ID when possible.
func resolveThreadID(runtime *common.RuntimeContext, id string) (string, error) {
	if threadIDRe.MatchString(id) {
		return id, nil
	}
	if !messageIDRe.MatchString(id) {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid thread ID format: must start with om_ or omt_").WithParam("--thread")
	}

	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodGet,
		ApiPath:    "/open-apis/im/v1/messages/" + validate.EncodePathSegment(id),
	})
	if err != nil {
		return "", err
	}
	data, err := runtime.ClassifyAPIResponse(apiResp)
	if err != nil {
		return "", err
	}

	items, _ := data["items"].([]interface{})
	for _, item := range items {
		msg, _ := item.(map[string]interface{})
		threadID, _ := msg["thread_id"].(string)
		if threadID != "" {
			return threadID, nil
		}
	}

	return "", errs.NewAPIError(errs.SubtypeNotFound, "thread ID not found for this message")
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
			// 64-bit "largesize" is the whole box length including its 16-byte
			// header, so the box ends at offset+largesize (mirroring the
			// offset+size used for 32-bit boxes below). Reject sizes that do not
			// fit the search window; this also rejects values that would
			// overflow int and drive boxEnd negative (CWE-190), which would
			// otherwise index data out of range and panic.
			largesize := binary.BigEndian.Uint64(data[offset+8:])
			if largesize < 16 || largesize > uint64(end-offset) {
				return -1, -1
			}
			boxEnd = offset + int(largesize)
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
func newMediaBuffer(ctx context.Context, runtime *common.RuntimeContext, rawURL string, maxSize int64, param string) (*mediaBuffer, error) {
	rc, ext, err := downloadURLToReader(ctx, runtime, rawURL, maxSize, param)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, wrapIMNetworkErr(err, "download failed")
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
			// 64-bit "largesize" is the whole box length including its 16-byte
			// header, so the box ends at offset+largesize (mirroring offset+size
			// for 32-bit boxes). Reject sizes that do not fit the file; this also
			// rejects values that would overflow int64 and drive boxEnd negative
			// (CWE-190), which would otherwise index data out of range and panic.
			largesize := binary.BigEndian.Uint64(data[offset+8 : offset+16])
			if largesize < 16 || largesize > uint64(fileSize-offset) {
				return 0
			}
			boxEnd = offset + int64(largesize)
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
			// 64-bit "largesize" is the whole box length including its 16-byte
			// header, so the box ends at offset+largesize (mirroring offset+size
			// for 32-bit boxes). Reject sizes that do not fit the file; this also
			// rejects values that would overflow int64 and drive boxEnd negative
			// (CWE-190).
			largesize := binary.BigEndian.Uint64(hdr[8:16])
			if largesize < 16 || largesize > uint64(fileSize-offset) {
				return 0
			}
			boxEnd = offset + int64(largesize)
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
	reH2toH6     = regexp.MustCompile(`(?m)^#{2,6} (.+)$`)
	reH1         = regexp.MustCompile(`(?m)^# (.+)$`)
	reHasH1toH3  = regexp.MustCompile(`(?m)^#{1,3} `)
	reConsecH    = regexp.MustCompile(`(?m)^(#{4,5} .+)\n{1,2}(#{4,5} )`)
	reTableNoGap = regexp.MustCompile(`(?m)^([^|\n].*)\n(\|.+\|)`)
	reTableAfter = regexp.MustCompile(`(?m)((?:^\|.+\|[^\S\n]*\n?)+)`)
	reExcessNL   = regexp.MustCompile(`\n{3,}`)
	reInvalidImg = regexp.MustCompile(`!\[[^\]]*\]\(([^)\s]+)\)`)
	reCodeBlock  = regexp.MustCompile("```[\\s\\S]*?```")
)

func optimizeMarkdownStyle(text string) string {
	const mark = "___CB_"
	var codeBlocks []string
	r := reCodeBlock.ReplaceAllStringFunc(text, func(m string) string {
		idx := len(codeBlocks)
		codeBlocks = append(codeBlocks, m)
		return fmt.Sprintf("%s%d___", mark, idx)
	})

	// Only downgrade when original text has H1~H3; order matters (H2~H6 first).
	if reHasH1toH3.MatchString(text) {
		r = reH2toH6.ReplaceAllString(r, "##### $1")
		r = reH1.ReplaceAllString(r, "#### $1")
	}

	r = reConsecH.ReplaceAllString(r, "$1\n\n$2")

	r = reTableNoGap.ReplaceAllString(r, "$1\n\n$2")
	r = reTableAfter.ReplaceAllString(r, "$1\n")

	for i, block := range codeBlocks {
		r = strings.Replace(r, fmt.Sprintf("%s%d___", mark, i), block, 1)
	}

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

// wrapMarkdownAsPost wraps markdown text into Feishu post format JSON (no network).
// Used by DryRun. Output: {"zh_cn":{"content":[[{"tag":"md","text":"..."}]]}}
func wrapMarkdownAsPost(markdown string) string {
	optimized := optimizeMarkdownStyle(markdown)
	inner, _ := json.Marshal(optimized)
	return `{"zh_cn":{"content":[[{"tag":"md","text":` + string(inner) + `}]]}}`
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
	optimized := optimizeMarkdownStyle(resolved)
	inner, _ := json.Marshal(optimized)
	return `{"zh_cn":{"content":[[{"tag":"md","text":` + string(inner) + `}]]}}`
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

		rc, _, err := downloadURLToReader(ctx, runtime, imgURL, maxImageUploadSize, "--markdown")
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

func uploadImageToIM(ctx context.Context, runtime *common.RuntimeContext, filePath, imageType, param string) (string, error) {
	if info, err := runtime.FileIO().Stat(filePath); err == nil && info.Size() > maxImageUploadSize {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "image size %s exceeds limit (max 5MB)", common.FormatSize(info.Size())).WithParam(param)
	}

	f, err := runtime.FileIO().Open(filePath)
	if err != nil {
		return "", withIMValidationParam(common.WrapInputStatErrorTyped(err), param)
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

	data, err := runtime.ClassifyAPIResponse(apiResp)
	if err != nil {
		return "", err
	}
	imageKey, _ := data["image_key"].(string)
	if imageKey == "" {
		return "", errs.NewInternalError(errs.SubtypeInvalidResponse, "image_key missing from a successful upload response")
	}
	return imageKey, nil
}

func uploadFileToIM(ctx context.Context, runtime *common.RuntimeContext, filePath, fileType, duration, param string) (string, error) {
	if info, err := runtime.FileIO().Stat(filePath); err == nil && info.Size() > maxFileUploadSize {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "file size %s exceeds limit (max 100MB)", common.FormatSize(info.Size())).WithParam(param)
	}

	f, err := runtime.FileIO().Open(filePath)
	if err != nil {
		return "", withIMValidationParam(common.WrapInputStatErrorTyped(err), param)
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

	data, err := runtime.ClassifyAPIResponse(apiResp)
	if err != nil {
		return "", err
	}
	fileKey, _ := data["file_key"].(string)
	if fileKey == "" {
		return "", errs.NewInternalError(errs.SubtypeInvalidResponse, "file_key missing from a successful upload response")
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

	data, err := runtime.ClassifyAPIResponse(apiResp)
	if err != nil {
		return "", err
	}
	imageKey, _ := data["image_key"].(string)
	if imageKey == "" {
		return "", errs.NewInternalError(errs.SubtypeInvalidResponse, "image_key missing from a successful upload response")
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

	data, err := runtime.ClassifyAPIResponse(apiResp)
	if err != nil {
		return "", err
	}
	fileKey, _ := data["file_key"].(string)
	if fileKey == "" {
		return "", errs.NewInternalError(errs.SubtypeInvalidResponse, "file_key missing from a successful upload response")
	}
	return fileKey, nil
}

// FlagType enumerates the kind of bookmark.
// Aligned with server-side constants: Unknown=0, Feed=1, Message=2.
type FlagType int

const (
	FlagTypeUnknown FlagType = 0
	FlagTypeFeed    FlagType = 1
	FlagTypeMessage FlagType = 2
)

// ItemType enumerates the kind of thing being bookmarked.
// Server-side constants (only the types used by IM flags):
//
//	default=0, thread=4, msg_thread=11.
//
// Note on the two thread-shaped item types:
//   - ItemTypeThread (4)     — thread inside a topic-style chat
//   - ItemTypeMsgThread (11) — thread inside a regular chat
type ItemType int

const (
	ItemTypeDefault   ItemType = 0
	ItemTypeThread    ItemType = 4  // thread in a topic-style chat
	ItemTypeMsgThread ItemType = 11 // thread in a regular chat
)

const (
	flagWriteScope = "im:feed.flag:write"
	flagReadScope  = "im:feed.flag:read"
)

var (
	flagWriteLookupScopes = append([]string{flagWriteScope}, flagLookupScopes...)
	flagMessageReadScopes = []string{
		"im:message.group_msg:get_as_user",
		"im:message.p2p_msg:get_as_user",
	}
	flagLookupScopes = []string{
		"im:message.group_msg:get_as_user",
		"im:message.p2p_msg:get_as_user",
		"im:chat:read",
	}
)

func checkFlagRequiredScopes(ctx context.Context, rt *common.RuntimeContext, required []string) error {
	if len(required) == 0 {
		return nil
	}
	result, err := rt.Factory.Credential.ResolveToken(ctx, credential.NewTokenSpec(rt.As(), rt.Config.AppID))
	if err != nil {
		return errs.NewAuthenticationError(errs.SubtypeTokenMissing, "cannot verify required scope(s): %v", err).
			WithHint("%s", flagScopeLoginHint(required)).
			WithCause(err)
	}
	if result == nil || result.Scopes == "" {
		fmt.Fprintf(rt.IO().ErrOut,
			"warning: cannot verify required scope(s) because token scope metadata is unavailable; API may fail if missing: %s\n",
			strings.Join(required, " "))
		return nil
	}
	if missing := auth.MissingScopes(result.Scopes, required); len(missing) > 0 {
		return errs.NewPermissionError(errs.SubtypeMissingScope, "missing required scope(s): %s", strings.Join(missing, ", ")).
			WithMissingScopes(missing...).
			WithHint("%s", flagScopeLoginHint(missing))
	}
	return nil
}

func flagScopeLoginHint(scopes []string) string {
	return fmt.Sprintf("run `lark-cli auth login --scope \"%s\"` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login.", strings.Join(scopes, " "))
}

// flagItem is one entry in the flags API body. The server expects numeric
// enums serialized as strings.
type flagItem struct {
	ItemID   string `json:"item_id"`
	ItemType string `json:"item_type"`
	FlagType string `json:"flag_type"`
}

// parseItemID inspects an om_ prefix and returns a best-guess
// (itemType, flagType) pair. Used when the user omits the explicit enums.
//   - om_xxx  → (default, message)
func parseItemID(id string) (ItemType, FlagType, error) {
	id = strings.TrimSpace(id)
	switch {
	case strings.HasPrefix(id, "om_"):
		return ItemTypeDefault, FlagTypeMessage, nil
	case id == "":
		return 0, 0, errs.NewValidationError(errs.SubtypeInvalidArgument, "--message-id cannot be empty").WithParam("--message-id")
	default:
		return 0, 0, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"cannot infer item type from id %q: expected om_ (message) prefix; "+
				"pass --item-type and --flag-type explicitly if you are using a different id format", id).WithParam("--message-id")
	}
}

// parseItemType converts a user-facing string to the server enum.
func parseItemType(s string) (ItemType, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "default":
		return ItemTypeDefault, nil
	case "thread":
		return ItemTypeThread, nil
	case "msg_thread":
		return ItemTypeMsgThread, nil
	}
	return 0, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --item-type %q: expected one of default|thread|msg_thread", s).WithParam("--item-type")
}

// parseFlagType converts a user-facing string to the server enum.
func parseFlagType(s string) (FlagType, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "message":
		return FlagTypeMessage, nil
	case "feed":
		return FlagTypeFeed, nil
	}
	return 0, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --flag-type %q: expected one of message|feed", s).WithParam("--flag-type")
}

// isValidCombo checks if the (ItemType, FlagType) pair is accepted by the server.
// Note: (ItemType, FlagType) is shorthand for (item_type, flag_type) — the two
// enum fields that determine which layer the flag operates on.
//
// Valid combinations are:
//   - (default, message)  — regular chat message (message-layer flag)
//   - (thread, feed)      — thread as feed-layer flag (topic-style chat)
//   - (msg_thread, feed)  — message-thread as feed-layer flag (regular chat)
func isValidCombo(it ItemType, ft FlagType) bool {
	return (it == ItemTypeDefault && ft == FlagTypeMessage) ||
		(it == ItemTypeThread && ft == FlagTypeFeed) ||
		(it == ItemTypeMsgThread && ft == FlagTypeFeed)
}

// parseItemTypeFromRaw parses a stringified numeric item_type back to ItemType.
// Used when re-parsing the serialized enum for combo-validity checks.
// Note: Unknown values return ItemTypeDefault (0). This is safe because:
//  1. This function only parses values we serialized ourselves via newFlagItem
//  2. Unknown server values would fail combo validation or be rejected by the server
func parseItemTypeFromRaw(s string) ItemType {
	switch s {
	case "0":
		return ItemTypeDefault
	case "4":
		return ItemTypeThread
	case "11":
		return ItemTypeMsgThread
	}
	return ItemTypeDefault
}

// parseFlagTypeFromRaw parses a stringified numeric flag_type back to FlagType.
// Used when re-parsing the serialized enum for combo-validity checks.
func parseFlagTypeFromRaw(s string) FlagType {
	switch s {
	case "1":
		return FlagTypeFeed
	case "2":
		return FlagTypeMessage
	}
	return FlagTypeUnknown
}

// newFlagItem builds a payload entry with numeric-stringified enums.
func newFlagItem(itemID string, it ItemType, ft FlagType) flagItem {
	return flagItem{
		ItemID:   itemID,
		ItemType: fmt.Sprintf("%d", int(it)),
		FlagType: fmt.Sprintf("%d", int(ft)),
	}
}

// getMessageChatID queries the message API to get the chat_id.
// Used by flag-create to determine the chat type for feed-layer flags.
func getMessageChatID(rt *common.RuntimeContext, messageID string) (string, error) {
	data, err := rt.DoAPIJSONTyped("GET", "/open-apis/im/v1/messages/"+validate.EncodePathSegment(messageID), nil, nil)
	if err != nil {
		return "", err
	}

	items, ok := data["items"].([]any)
	if !ok || len(items) == 0 {
		return "", errs.NewAPIError(errs.SubtypeNotFound, "message not found")
	}

	msg, ok := items[0].(map[string]any)
	if !ok {
		return "", errs.NewInternalError(errs.SubtypeInvalidResponse, "unexpected message format in API response")
	}

	chatID, ok := msg["chat_id"].(string)
	if !ok {
		return "", errs.NewInternalError(errs.SubtypeInvalidResponse, "message response missing chat_id field")
	}
	return chatID, nil
}

// resolveThreadFeedItemType determines the correct feed-layer ItemType for a thread
// by querying the chat API for chat_mode.
//   - topic-style chat → ItemTypeThread
//   - regular chat    → ItemTypeMsgThread
//
// Returns an error if the chat query fails, since guessing the wrong item_type
// can cause silent failures in flag operations.
func resolveThreadFeedItemType(rt *common.RuntimeContext, chatID string) (ItemType, error) {
	data, err := rt.DoAPIJSONTyped("GET", "/open-apis/im/v1/chats/"+validate.EncodePathSegment(chatID), nil, nil)
	if err != nil {
		return ItemTypeDefault, wrapIMNetworkErr(err, "failed to query chat_mode for chat %s", chatID)
	}

	// DoAPIJSONTyped returns envelope.Data, so chat_mode is at the top level
	chatMode, _ := data["chat_mode"].(string)
	if chatMode == "topic" {
		return ItemTypeThread, nil
	}
	return ItemTypeMsgThread, nil
}

// ShortcutType enumerates the OpenAPI feed-shortcut types.
// Currently the server only opens CHAT (1) externally; other internal values
// (DOC, OPENAPP, etc.) are not yet whitelisted on the OAPI gateway.
type ShortcutType int

const (
	ShortcutTypeUnknown ShortcutType = 0
	ShortcutTypeChat    ShortcutType = 1
)

const (
	feedShortcutBatchLimit = 10
	feedShortcutWriteScope = "im:feed.shortcut:write"
	feedShortcutReadScope  = "im:feed.shortcut:read"
)

// shortcutItem is one entry in the feed_shortcuts API body.
type shortcutItem struct {
	FeedCardID string `json:"feed_card_id"`
	Type       int    `json:"type"`
}

// collectChatIDs reads --chat-id values (repeatable + comma-split) and
// returns deduped, validated oc_ IDs. The server batch limit is 10.
func collectChatIDs(rt *common.RuntimeContext) ([]string, error) {
	raw := rt.StrSlice("chat-id")
	if len(raw) == 0 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--chat-id is required (oc_xxx); repeat the flag or pass comma-separated values").WithParam("--chat-id")
	}

	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if !strings.HasPrefix(v, "oc_") {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"invalid --chat-id %q: must be an open_chat_id starting with oc_", v).WithParam("--chat-id")
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--chat-id is required (oc_xxx)").WithParam("--chat-id")
	}
	if len(out) > feedShortcutBatchLimit {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"too many --chat-id values (%d); the server accepts up to %d per request",
			len(out), feedShortcutBatchLimit).WithParam("--chat-id")
	}
	return out, nil
}

// buildShortcutItems converts chat IDs to API payload entries (type=CHAT).
func buildShortcutItems(ids []string) []shortcutItem {
	items := make([]shortcutItem, 0, len(ids))
	for _, id := range ids {
		items = append(items, shortcutItem{FeedCardID: id, Type: int(ShortcutTypeChat)})
	}
	return items
}

// shortcutFailedReasonString converts the numeric failed-reason enum returned
// by the server into a human-readable label. Used to enrich the response
// when the API reports per-item failures.
func shortcutFailedReasonString(reason int) string {
	switch reason {
	case 0:
		return "unknown"
	case 1:
		return "no_permission"
	case 2:
		return "invalid_item"
	case 3:
		return "has_pending_delete"
	case 4:
		return "type_not_support"
	case 5:
		return "internal_error"
	}
	return "unknown"
}

// chatBatchQueryScope is the scope required by im.chats.batch_query, which
// the CHAT detail resolver depends on. Surfaced as a conditional scope on
// +feed-shortcut-list so the framework's scope diagnostics know about it.
const chatBatchQueryScope = "im:chat:read"

// chatBatchQuerySize matches the server-side limit on /im/v1/chats/batch_query.
const chatBatchQuerySize = 50

// shortcutTypeFromValue parses the type field as returned by the v2
// feed_shortcuts API. JSON numbers come back as float64 after generic
// unmarshal; we also tolerate the int form for forward-compat.
func shortcutTypeFromValue(v any) ShortcutType {
	switch n := v.(type) {
	case float64:
		return ShortcutType(int(n))
	case int:
		return ShortcutType(n)
	case json.Number:
		i, err := n.Int64()
		if err == nil {
			return ShortcutType(i)
		}
	}
	return ShortcutTypeUnknown
}

// queryChatBatch fetches one im.chats.batch_query page (at most
// chatBatchQuerySize ids) and merges the full chat objects into dst keyed by
// chat_id. Shared by feed-shortcut detail enrichment and message-search chat
// context lookup, which apply their own per-chunk error policies.
func queryChatBatch(rt *common.RuntimeContext, batch []string, dst map[string]map[string]any) error {
	res, err := rt.DoAPIJSONTyped(http.MethodPost, "/open-apis/im/v1/chats/batch_query",
		larkcore.QueryParams{"user_id_type": []string{"open_id"}},
		map[string]any{"chat_ids": batch})
	if err != nil {
		return err
	}
	items, _ := res["items"].([]any)
	for _, ci := range items {
		cm, _ := ci.(map[string]any)
		if cm == nil {
			continue
		}
		if id := asString(cm["chat_id"]); id != "" {
			dst[id] = cm
		}
	}
	return nil
}

// resolveChatDetail batch-fetches the full chat object via
// im.chats.batch_query (50 ids per request — server limit) and returns the
// objects keyed by chat_id, verbatim, so the caller can decide which fields
// to surface. The server's `name` field is empty for p2p chats (client UI
// shows the partner's display name there), but the full object still carries
// `chat_mode`, `p2p_target_id`, `description`, etc., so callers can render
// p2p entries however they want.
func resolveChatDetail(rt *common.RuntimeContext, ids []string) (map[string]map[string]any, error) {
	out := map[string]map[string]any{}
	if len(ids) == 0 {
		return out, nil
	}
	if err := checkFlagRequiredScopes(rt.Ctx(), rt, []string{chatBatchQueryScope}); err != nil {
		return nil, err
	}
	for _, batch := range chunkStrings(ids, chatBatchQuerySize) {
		if err := queryChatBatch(rt, batch, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// enrichFeedShortcutDetail walks the list response and attaches the full chat
// object under `detail` for CHAT-type entries — the only type the OpenAPI
// gateway exposes today. Mutates data in place.
//
// Failures are returned to the caller so it can decide whether to hard-fail
// the command or downgrade to a warning. Listing the shortcuts succeeds even
// if enrichment is unavailable (missing scope, network error, etc.).
func enrichFeedShortcutDetail(rt *common.RuntimeContext, data map[string]any) error {
	items, _ := data["shortcuts"].([]any)
	if len(items) == 0 {
		return nil
	}

	seen := map[string]struct{}{}
	ids := make([]string, 0, len(items))
	for _, it := range items {
		m, _ := it.(map[string]any)
		if m == nil || shortcutTypeFromValue(m["type"]) != ShortcutTypeChat {
			continue
		}
		id := asString(m["feed_card_id"])
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}

	details, err := resolveChatDetail(rt, ids)
	if err != nil {
		return err
	}

	// Missing items (server didn't return one for an id we asked about) are
	// left untouched, so the presence of `detail` signals a successful lookup.
	for _, it := range items {
		m, _ := it.(map[string]any)
		if m == nil || shortcutTypeFromValue(m["type"]) != ShortcutTypeChat {
			continue
		}
		if info, ok := details[asString(m["feed_card_id"])]; ok {
			m["detail"] = info
		}
	}
	return nil
}

// annotateFailedShortcuts walks the API response and attaches a
// reason_label string next to each numeric reason. Mutates data in place.
func annotateFailedShortcuts(data map[string]any) {
	items, ok := data["failed_shortcuts"].([]any)
	if !ok {
		return
	}
	for _, it := range items {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		// reason is serialized as a JSON number → float64 after generic unmarshal.
		switch r := m["reason"].(type) {
		case float64:
			m["reason_label"] = shortcutFailedReasonString(int(r))
		case int:
			m["reason_label"] = shortcutFailedReasonString(r)
		case json.Number:
			i, err := r.Int64()
			if err == nil {
				m["reason_label"] = shortcutFailedReasonString(int(i))
			}
		}
	}
}

// emitFeedShortcutWriteResult preserves the server payload while adding a
// batch ledger. A feed-shortcut write can return HTTP/API success with
// failed_shortcuts populated; callers still need a complete account of which
// requested entries succeeded and which failed.
func emitFeedShortcutWriteResult(rt *common.RuntimeContext, requested []shortcutItem, data map[string]any) error {
	// A fully-successful write can come back as code:0 with data:null, in
	// which case DoAPIJSON hands us a nil map; the caller is still owed a
	// ledger, so start from an empty object instead of panicking on write.
	if data == nil {
		data = map[string]any{}
	}
	annotateFailedShortcuts(data)
	addFeedShortcutWriteLedger(data, requested)
	if hasFailedShortcuts(data) {
		return rt.OutPartialFailure(data, nil)
	}
	rt.Out(data, nil)
	return nil
}

func addFeedShortcutWriteLedger(data map[string]any, requested []shortcutItem) {
	failed := failedShortcutItems(data)
	// Failed entries are matched back to requested items by feed_card_id
	// alone: every requested item is CHAT-type, so the id is the identity,
	// and a failed echo with a missing or zero type still excludes its item
	// from the success list.
	failedIDs := map[string]struct{}{}
	for _, it := range failed {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		shortcut, _ := m["shortcut"].(map[string]any)
		if shortcut == nil {
			continue
		}
		if id := asString(shortcut["feed_card_id"]); id != "" {
			failedIDs[id] = struct{}{}
		}
	}

	succeeded := make([]shortcutItem, 0, len(requested))
	for _, it := range requested {
		if _, isFailed := failedIDs[it.FeedCardID]; isFailed {
			continue
		}
		succeeded = append(succeeded, it)
	}

	// Counts are derived from the requested-item accounting alone so the
	// success+failure==total invariant holds even if the server echoes a
	// failed entry twice or reports one we never asked about;
	// failed_shortcuts still carries the raw server report.
	data["total"] = len(requested)
	data["success_count"] = len(succeeded)
	data["failure_count"] = len(requested) - len(succeeded)
	data["succeeded_shortcuts"] = succeeded
}

func hasFailedShortcuts(data map[string]any) bool {
	return len(failedShortcutItems(data)) > 0
}

func failedShortcutItems(data map[string]any) []any {
	items, _ := data["failed_shortcuts"].([]any)
	return items
}
