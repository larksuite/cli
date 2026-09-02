// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"math/big"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	localDocResourceBindBatchSize       = 20
	localDocResourceCleanupBatchSize    = 200
	localDocResourceBindAttempts        = 3
	localDocResourceUploadAttempts      = 3
	remoteDocImageDownloadAttempts      = 3
	remoteDocImageUploadConcurrency     = 10
	localDocResourceUploadInterval      = 220 * time.Millisecond
	localDocResourceBindInterval        = 350 * time.Millisecond
	localDocResourceVerifyInterval      = 220 * time.Millisecond
	localDocResourceUploadConflictCode  = 1061045
	localDocResourceUploadRateLimitCode = 99991400
	localDocImageMaxDisplayWidthPx      = 1020
	localDocImageScalePrecision         = 1000000
	remoteDocImageMaxBytes              = int64(20 * 1024 * 1024)
)

var waitLocalDocResourceRequest = func(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func localDocResourceRetryDelay(base time.Duration, attempt int) time.Duration {
	delay := base * time.Duration(1<<attempt)
	jitterLimit := delay / 4
	if jitterLimit <= 0 {
		return delay
	}
	jitter, err := rand.Int(rand.Reader, big.NewInt(int64(jitterLimit)+1))
	if err != nil {
		return delay
	}
	return delay + time.Duration(jitter.Int64())
}

type localDocResourceKind string

const (
	localDocResourceImage localDocResourceKind = "image"
	localDocResourceFile  localDocResourceKind = "file"
)

var localDocResourceMarkerPattern = regexp.MustCompile(`^@lcli_(?:img|file)_[0-9a-f]{32}$`)

func isReservedLocalDocResourceMarker(value string) bool {
	value = strings.TrimSpace(value)
	if localDocResourceMarkerPattern.MatchString(value) {
		return true
	}
	return strings.HasPrefix(value, "@lcli_img_") || strings.HasPrefix(value, "@lcli_file_")
}

type localDocResource struct {
	Occurrence                 int
	Kind                       localDocResourceKind
	Marker                     string
	Path                       string
	FileName                   string
	Size                       int64
	ImageWidth                 int
	ImageHeight                int
	ImageAlign                 string
	ImageScale                 float64
	HasScale                   bool
	RemoteURL                  string
	RequestedImagePresentation localDocImagePresentation
	Content                    []byte
}

type localDocImagePresentation struct {
	Width  string
	Height string
	Align  string
	Scale  string
}

type localDocResourceTag struct {
	Name        string
	Attrs       []html5BlockAttr
	SelfClosing bool
}

type localDocResourceOutcome struct {
	Resource        localDocResource
	BlockID         string
	Block           map[string]interface{}
	CleanupBlockIDs []string
	FileToken       string
	Status          string
	CleanupStatus   string
	Err             error
	ServerWarnings  []interface{}
	SafeToCleanup   bool
}

func prepareLocalDocResources(runtime *common.RuntimeContext, format, content string) (string, []localDocResource, error) {
	if !strings.Contains(content, "<img") && !strings.Contains(content, "<source") &&
		!(strings.TrimSpace(format) == "markdown" && strings.Contains(content, "![")) {
		return content, nil, nil
	}

	resources := make([]localDocResource, 0)
	markdownMode := strings.TrimSpace(format) == "markdown"
	rewriteSegment := func(segment string, localRefs map[string]struct{}) (string, error) {
		return rewriteLocalDocResourceSegment(runtime, segment, markdownMode, localRefs, &resources)
	}

	if !markdownMode {
		out, err := rewriteSegment(content, nil)
		return out, resources, err
	}

	content, inertSpans := protectMarkdownLocalDocResourceMarkup(content)
	localRefs := collectLocalMarkdownImageReferences(content)
	var (
		out        strings.Builder
		segment    strings.Builder
		rewriteErr error
		fenceChar  byte
		fenceLen   int
		indentCtx  markdownIndentContext
	)
	flush := func() {
		if segment.Len() == 0 {
			return
		}
		if rewriteErr != nil {
			out.WriteString(segment.String())
			segment.Reset()
			return
		}
		rewritten, err := rewriteSegment(segment.String(), localRefs)
		if err != nil {
			rewriteErr = err
			out.WriteString(segment.String())
		} else {
			out.WriteString(rewritten)
		}
		segment.Reset()
	}

	for _, line := range strings.SplitAfter(content, "\n") {
		indentedCode := false
		if fenceChar == 0 {
			indentedCode = indentCtx.isIndentedCodeLine(line)
		}
		char, run, isFence := markdownFence(line)
		if fenceChar == 0 && isFence {
			flush()
			fenceChar, fenceLen = char, run
			out.WriteString(line)
			continue
		}
		if fenceChar != 0 {
			out.WriteString(line)
			if isFence && char == fenceChar && run >= fenceLen && markdownFenceCloses(line, char, run) {
				fenceChar, fenceLen = 0, 0
			}
			continue
		}
		if indentedCode {
			flush()
			out.WriteString(line)
			continue
		}
		segment.WriteString(line)
	}
	flush()
	if rewriteErr != nil {
		return "", nil, rewriteErr
	}
	result := out.String()
	for _, inert := range inertSpans {
		result = strings.ReplaceAll(result, inert.Protected, inert.Original)
	}
	return result, resources, nil
}

type protectedLocalDocResourceSpan struct {
	Protected string
	Original  string
}

func protectMarkdownLocalDocResourceMarkup(content string) (string, []protectedLocalDocResourceSpan) {
	var out strings.Builder
	spans := make([]protectedLocalDocResourceSpan, 0)
	nextToken := 0
	for i := 0; i < len(content); {
		end := -1
		switch {
		case strings.HasPrefix(content[i:], "<!--"):
			if offset := strings.Index(content[i+4:], "-->"); offset >= 0 {
				end = i + 4 + offset + 3
			} else {
				end = len(content)
			}
		case strings.HasPrefix(content[i:], "<![CDATA["):
			if offset := strings.Index(content[i+9:], "]]>"); offset >= 0 {
				end = i + 9 + offset + 3
			} else {
				end = len(content)
			}
		default:
			if rawEnd, ok := findMarkdownRawHTMLInertEnd(content, i); ok {
				end = rawEnd
			}
		}
		if end < 0 {
			out.WriteByte(content[i])
			i++
			continue
		}

		original := content[i:end]
		protected := ""
		for {
			protected = fmt.Sprintf("\ue000lark_cli_inert_%d\ue001", nextToken)
			nextToken++
			if !strings.Contains(content, protected) {
				break
			}
		}
		protected += strings.Repeat("\n", strings.Count(original, "\n"))
		out.WriteString(protected)
		spans = append(spans, protectedLocalDocResourceSpan{Protected: protected, Original: original})
		i = end
	}
	return out.String(), spans
}

func rewriteLocalDocResourceSegment(runtime *common.RuntimeContext, segment string, markdownMode bool, localRefs map[string]struct{}, resources *[]localDocResource) (string, error) {
	var out strings.Builder
	for i := 0; i < len(segment); {
		if markdownMode {
			if end, ok := findMarkdownRawHTMLInertEnd(segment, i); ok {
				out.WriteString(segment[i:end])
				i = end
				continue
			}
		}
		if strings.HasPrefix(segment[i:], "<!--") {
			end := strings.Index(segment[i+4:], "-->")
			if end < 0 {
				out.WriteString(segment[i:])
				break
			}
			end += i + 7
			out.WriteString(segment[i:end])
			i = end
			continue
		}
		if strings.HasPrefix(segment[i:], "<![CDATA[") {
			end := strings.Index(segment[i+9:], "]]>")
			if end < 0 {
				out.WriteString(segment[i:])
				break
			}
			end += i + 12
			out.WriteString(segment[i:end])
			i = end
			continue
		}
		if markdownMode && segment[i] == '`' {
			run := countByteRun(segment, i, '`')
			end := findMatchingBacktickRun(segment, i+run, run)
			if end < 0 {
				out.WriteString(segment[i : i+run])
				i += run
				continue
			}
			out.WriteString(segment[i:end])
			i = end
			continue
		}

		if segment[i] == '<' && !(markdownMode && isEscapedMarkdownByte(segment, i)) {
			name := localResourceTagNameAt(segment, i)
			if name != "" {
				end := findXMLStartTagEnd(segment, i)
				if end < 0 {
					return "", common.ValidationErrorf("invalid <%s> local resource tag", name).WithParam(name)
				}
				raw := segment[i:end]
				rewritten, resource, changed, err := rewriteRawLocalResourceTag(runtime, raw, name, len(*resources)+1)
				if err != nil {
					return "", err
				}
				out.WriteString(rewritten)
				if changed {
					*resources = append(*resources, resource)
				}
				i = end
				continue
			}
		}

		if segment[i] == '!' && i+1 < len(segment) && segment[i+1] == '[' && !isEscapedMarkdownByte(segment, i) {
			image, ok := parseMarkdownImageAt(segment, i)
			if ok {
				if image.ReferenceLabel != "" {
					if _, local := localRefs[normalizeMarkdownReferenceLabel(image.ReferenceLabel)]; local {
						return "", common.ValidationErrorf("local Markdown reference-style images are not supported; use ![alt](@relative/path) instead").WithParam("content")
					}
					out.WriteString(segment[i:image.End])
					i = image.End
					continue
				}
				if strings.HasPrefix(image.Destination, "@") {
					resource, err := newLocalDocResource(runtime, localDocResourceImage, image.Destination, len(*resources)+1)
					if err != nil {
						return "", err
					}
					out.WriteString(`<img path="`)
					out.WriteString(escapeXMLAttr(resource.Marker))
					out.WriteByte('"')
					if strings.TrimSpace(image.Alt) != "" {
						out.WriteString(` caption="`)
						out.WriteString(escapeXMLAttr(unescapeMarkdownText(image.Alt)))
						out.WriteByte('"')
					}
					if strings.TrimSpace(image.Title) != "" {
						out.WriteString(` title="`)
						out.WriteString(escapeXMLAttr(image.Title))
						out.WriteByte('"')
					}
					out.WriteString("/>")
					*resources = append(*resources, resource)
					i = image.End
					continue
				}
				out.WriteString(segment[i:image.End])
				i = image.End
				continue
			}
		}

		out.WriteByte(segment[i])
		i++
	}
	return out.String(), nil
}

func rewriteRawLocalResourceTag(runtime *common.RuntimeContext, raw, name string, occurrence int) (string, localDocResource, bool, error) {
	tag, err := parseLocalDocResourceTag(raw, name)
	if err != nil {
		return "", localDocResource{}, false, common.ValidationErrorf("invalid <%s> local resource tag: %v", name, err).WithParam(name).WithCause(err)
	}
	pathValue, hasPath := tag.attr("path")
	if !hasPath {
		if name != "img" {
			return raw, localDocResource{}, false, nil
		}
		href, hasHref := tag.attr("href")
		if !hasHref {
			return raw, localDocResource{}, false, nil
		}
		conflicts := []string{"src", "token", "img_key", "img-key", "url"}
		for _, conflict := range conflicts {
			if tag.hasAttr(conflict) {
				return "", localDocResource{}, false, common.ValidationErrorf("<img> href cannot be combined with %s", conflict).WithParam("img")
			}
		}
		resource, err := newRemoteDocImageResource(href, occurrence)
		if err != nil {
			return "", localDocResource{}, false, err
		}
		tag.renameAttr("href", "path")
		tag.setAttr("path", resource.Marker)
		if _, hasCaption := tag.attr("caption"); !hasCaption {
			tag.renameAttr("alt", "caption")
		}
		resource.RequestedImagePresentation = captureLocalDocImagePresentation(tag)
		resource.captureImagePresentation(tag)
		for _, name := range []string{"width", "height", "align", "scale"} {
			tag.deleteAttr(name)
		}
		return tag.render(), resource, true, nil
	}

	conflicts := []string{"src", "href", "token", "img_key", "img-key", "url"}
	for _, conflict := range conflicts {
		if tag.hasAttr(conflict) {
			return "", localDocResource{}, false, common.ValidationErrorf("<%s> local path cannot be combined with %s", name, conflict).WithParam(name)
		}
	}

	kind := localDocResourceImage
	if name == "source" {
		kind = localDocResourceFile
	}
	resource, err := newLocalDocResource(runtime, kind, pathValue, occurrence)
	if err != nil {
		return "", localDocResource{}, false, err
	}
	tag.setAttr("path", resource.Marker)
	if kind == localDocResourceImage {
		if _, hasCaption := tag.attr("caption"); !hasCaption {
			tag.renameAttr("alt", "caption")
		}
		if err := normalizeLocalDocImagePresentation(runtime, &tag, &resource); err != nil {
			return "", localDocResource{}, false, localResourceValidationErrorWithCause(kind, occurrence, "file is not a supported BMP, GIF, JPEG, PNG, TIFF, or WebP image", err)
		}
		resource.captureImagePresentation(tag)
	} else if rawName, hasName := tag.attr("name"); hasName {
		fileName := strings.TrimSpace(rawName)
		if fileName == "" || fileName == "." || fileName == ".." || strings.ContainsAny(fileName, `/\\`) {
			return "", localDocResource{}, false, common.ValidationErrorf("<source> name must be a non-empty file name without path separators").WithParam("source")
		}
		resource.FileName = fileName
		tag.setAttr("name", fileName)
	}
	return tag.render(), resource, true, nil
}

func newRemoteDocImageResource(rawURL string, occurrence int) (localDocResource, error) {
	rawURL = strings.TrimSpace(rawURL)
	u, err := url.Parse(rawURL)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || strings.TrimSpace(u.Hostname()) == "" || u.User != nil {
		return localDocResource{}, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"remote image #%d href must be an absolute HTTP(S) URL without userinfo",
			occurrence,
		).WithParam("href")
	}
	marker, err := newLocalDocResourceMarker(localDocResourceImage)
	if err != nil {
		return localDocResource{}, errs.NewInternalError(errs.SubtypeUnknown, "failed to generate remote image marker").WithCause(err)
	}
	return localDocResource{
		Occurrence: occurrence,
		Kind:       localDocResourceImage,
		Marker:     marker,
		RemoteURL:  u.String(),
	}, nil
}

type remoteDocImageDownload struct {
	Content  []byte
	FileName string
	Width    int
	Height   int
}

var (
	downloadRemoteDocImage  = downloadRemoteDocImageContent
	doRemoteDocImageRequest = func(client remoteDocImageHTTPDoer, req *http.Request) (*http.Response, error) {
		return client.Do(req)
	}
)

type remoteDocImageHTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

func downloadRemoteDocImageContent(runtime *common.RuntimeContext, rawURL string, occurrence int) (remoteDocImageDownload, error) {
	if err := validateRemoteDocImageSource(runtime.Ctx(), rawURL, occurrence); err != nil {
		return remoteDocImageDownload{}, err
	}
	baseClient, err := runtime.Factory.ExternalHTTPClient()
	if err != nil {
		return remoteDocImageDownload{}, errs.NewInternalError(errs.SubtypeSDKError, "initialize remote image HTTP client: %v", err).WithCause(err)
	}
	client := validate.NewDownloadHTTPClient(baseClient, validate.DownloadHTTPClientOptions{AllowHTTP: true, MaxRedirects: 5})
	req, err := http.NewRequestWithContext(runtime.Ctx(), http.MethodGet, rawURL, nil) //nolint:forbidigo // guarded download of user-provided image URL
	if err != nil {
		return remoteDocImageDownload{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid remote image #%d href: %v", occurrence, err).WithParam("href").WithCause(err)
	}
	resp, err := doRemoteDocImageRequest(client, req)
	if err != nil {
		return remoteDocImageDownload{}, remoteDocImageNetworkError(err, occurrence, "request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		subtype := errs.SubtypeNetworkTransport
		retryable := false
		switch {
		case resp.StatusCode == http.StatusTooManyRequests:
			subtype = errs.SubtypeRateLimit
			retryable = true
		case resp.StatusCode >= 500:
			subtype = errs.SubtypeNetworkServer
			retryable = true
		}
		httpErr := errs.NewNetworkError(subtype, "download remote image #%d failed: HTTP %d", occurrence, resp.StatusCode).WithCode(resp.StatusCode)
		if retryable {
			httpErr.WithRetryable()
		}
		return remoteDocImageDownload{}, httpErr
	}
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return remoteDocImageDownload{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "remote image #%d response has an invalid Content-Type", occurrence).WithParam("href").WithCause(err)
	}
	ext, ok := docCoverAllowedContentTypes[strings.ToLower(mediaType)]
	if !ok {
		return remoteDocImageDownload{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "remote image #%d response Content-Type %q is not supported", occurrence, mediaType).WithParam("href")
	}
	if resp.ContentLength > remoteDocImageMaxBytes {
		return remoteDocImageDownload{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "remote image #%d exceeds 20MiB limit", occurrence).WithParam("href")
	}

	body := &remoteDocImageResponseReader{reader: resp.Body}
	content, err := io.ReadAll(io.LimitReader(body, remoteDocImageMaxBytes+1))
	if err != nil {
		if body.err != nil {
			return remoteDocImageDownload{}, remoteDocImageNetworkError(body.err, occurrence, "response body failed")
		}
		if contextErr := runtime.Ctx().Err(); contextErr != nil {
			return remoteDocImageDownload{}, remoteDocImageNetworkError(contextErr, occurrence, "was interrupted")
		}
		return remoteDocImageDownload{}, remoteDocImageNetworkError(err, occurrence, "response body failed")
	}
	if len(content) == 0 {
		return remoteDocImageDownload{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "remote image #%d response is empty", occurrence).WithParam("href")
	}
	if int64(len(content)) > remoteDocImageMaxBytes {
		return remoteDocImageDownload{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "remote image #%d exceeds 20MiB limit", occurrence).WithParam("href")
	}
	config, detectedFormat, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil {
		return remoteDocImageDownload{}, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"remote image #%d response body is not a valid %s image",
			occurrence,
			mediaType,
		).WithParam("href").WithCause(err)
	}
	expectedFormat := strings.TrimPrefix(strings.ToLower(mediaType), "image/")
	if expectedFormat == "jpeg" && detectedFormat == "jpg" {
		detectedFormat = "jpeg"
	}
	if detectedFormat != expectedFormat {
		return remoteDocImageDownload{}, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"remote image #%d response declared %s but contains %s image data",
			occurrence,
			mediaType,
			detectedFormat,
		).WithParam("href")
	}
	return remoteDocImageDownload{
		Content:  content,
		FileName: "image" + ext,
		Width:    config.Width,
		Height:   config.Height,
	}, nil
}

type remoteDocImageResponseReader struct {
	reader io.Reader
	err    error
}

func (r *remoteDocImageResponseReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		r.err = err
	}
	return n, err
}

func remoteDocImageNetworkError(err error, occurrence int, action string) error {
	if _, ok := errs.ProblemOf(err); ok {
		return err
	}
	subtype := errs.SubtypeNetworkTransport
	if errors.Is(err, context.DeadlineExceeded) {
		subtype = errs.SubtypeNetworkTimeout
	}
	networkErr := errs.NewNetworkError(subtype, "download remote image #%d %s", occurrence, action).WithCause(err)
	if !errors.Is(err, context.Canceled) {
		networkErr.WithRetryable()
	}
	return networkErr
}

func validateRemoteDocImageSource(ctx context.Context, rawURL string, occurrence int) error {
	if err := validate.ValidateDownloadSourceURL(ctx, rawURL); err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"remote image #%d href is not allowed: %v", occurrence, err).
			WithParam("href").
			WithHint("use a public HTTP(S) image URL, or save the image in the draft workspace and reference it with <img path=\"@relative/path\"/>").
			WithCause(err)
	}
	return nil
}

func validateRemoteDocImageSources(ctx context.Context, resources []localDocResource) error {
	for _, resource := range resources {
		if resource.RemoteURL == "" {
			continue
		}
		if err := validateRemoteDocImageSource(ctx, resource.RemoteURL, resource.Occurrence); err != nil {
			return err
		}
	}
	return nil
}

func probeRemoteDocImageDownload(runtime *common.RuntimeContext, rawURL string, occurrence int) error {
	if err := validateRemoteDocImageSource(runtime.Ctx(), rawURL, occurrence); err != nil {
		return err
	}
	baseClient, err := runtime.Factory.ExternalHTTPClient()
	if err != nil {
		return errs.NewInternalError(errs.SubtypeSDKError, "initialize remote image HTTP client: %v", err).WithCause(err)
	}
	client := validate.NewDownloadHTTPClient(baseClient, validate.DownloadHTTPClientOptions{AllowHTTP: true, MaxRedirects: 5})
	return probeRemoteDocImageRequest(runtime, client, rawURL, occurrence)
}

func probeRemoteDocImageRequest(runtime *common.RuntimeContext, client remoteDocImageHTTPDoer, rawURL string, occurrence int) error {
	req, err := http.NewRequestWithContext(runtime.Ctx(), http.MethodGet, rawURL, nil) //nolint:forbidigo // guarded probe of user-provided image URL
	if err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid remote image #%d href: %v", occurrence, err).WithParam("href").WithCause(err)
	}
	req.Header.Set("Range", "bytes=0-0")
	resp, err := doRemoteDocImageRequest(client, req)
	if err != nil {
		return remoteDocImageNetworkError(err, occurrence, "availability probe failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return remoteDocImageHTTPStatusError(occurrence, resp.StatusCode, "probe")
	}
	if resp.ContentLength > remoteDocImageMaxBytes {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "remote image #%d exceeds 20MiB limit", occurrence).WithParam("href")
	}
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		return nil
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "remote image #%d response has an invalid Content-Type", occurrence).WithParam("href").WithCause(err)
	}
	if _, ok := docCoverAllowedContentTypes[strings.ToLower(mediaType)]; !ok {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "remote image #%d response Content-Type %q is not supported", occurrence, mediaType).WithParam("href")
	}
	return nil
}

func remoteDocImageHTTPStatusError(occurrence, statusCode int, action string) error {
	subtype := errs.SubtypeNetworkTransport
	retryable := false
	switch {
	case statusCode == http.StatusTooManyRequests:
		subtype = errs.SubtypeRateLimit
		retryable = true
	case statusCode >= 500:
		subtype = errs.SubtypeNetworkServer
		retryable = true
	}
	httpErr := errs.NewNetworkError(subtype, "%s remote image #%d failed: HTTP %d", action, occurrence, statusCode).WithCode(statusCode)
	if retryable {
		httpErr.WithRetryable()
	}
	return httpErr
}

func applyRemoteDocImageDownload(resource *localDocResource, download remoteDocImageDownload) error {
	if len(download.Content) == 0 || download.Width <= 0 || download.Height <= 0 {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "remote image download is missing content or dimensions")
	}
	resource.FileName = download.FileName
	resource.Size = int64(len(download.Content))
	tag := resource.RequestedImagePresentation.tag()
	normalizeDocImagePresentation(&tag, resource, download.Width, download.Height)
	resource.ImageWidth = 0
	resource.ImageHeight = 0
	resource.ImageAlign = ""
	resource.ImageScale = 0
	resource.HasScale = false
	resource.captureImagePresentation(tag)
	return nil
}

func captureLocalDocImagePresentation(tag localDocResourceTag) localDocImagePresentation {
	presentation := localDocImagePresentation{}
	presentation.Width, _ = tag.attr("width")
	presentation.Height, _ = tag.attr("height")
	presentation.Align, _ = tag.attr("align")
	presentation.Scale, _ = tag.attr("scale")
	return presentation
}

func (p localDocImagePresentation) tag() localDocResourceTag {
	tag := localDocResourceTag{Name: "img", SelfClosing: true}
	for _, attr := range []struct {
		name  string
		value string
	}{
		{name: "width", value: p.Width},
		{name: "height", value: p.Height},
		{name: "align", value: p.Align},
		{name: "scale", value: p.Scale},
	} {
		if attr.value != "" {
			tag.setAttr(attr.name, attr.value)
		}
	}
	return tag
}

func (r *localDocResource) captureImagePresentation(tag localDocResourceTag) {
	if raw, ok := tag.attr("width"); ok {
		if value, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && value > 0 {
			r.ImageWidth = value
		}
	}
	if raw, ok := tag.attr("height"); ok {
		if value, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && value > 0 {
			r.ImageHeight = value
		}
	}
	if raw, ok := tag.attr("align"); ok {
		align := strings.ToLower(strings.TrimSpace(raw))
		if _, supported := alignMap[align]; supported {
			r.ImageAlign = align
		}
	}
	if raw, ok := tag.attr("scale"); ok {
		if value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil && value > 0 {
			r.ImageScale = value
			r.HasScale = true
		}
	}
}

// normalizeLocalDocImagePresentation mirrors the SDK's network-image
// normalization before the local path is replaced by an opaque marker. The
// stored width/height are always the source image's intrinsic pixel
// dimensions; model-provided display dimensions are converted to scale.
func normalizeLocalDocImagePresentation(runtime *common.RuntimeContext, tag *localDocResourceTag, resource *localDocResource) error {
	nativeWidth, nativeHeight, err := detectImageDimensionsFromPath(runtime.FileIO(), resource.Path)
	if err != nil || nativeWidth <= 0 || nativeHeight <= 0 {
		if err != nil {
			return err
		}
		return invalidLocalDocImageDimensionsError()
	}
	normalizeDocImagePresentation(tag, resource, nativeWidth, nativeHeight)
	return nil
}

func normalizeDocImagePresentation(tag *localDocResourceTag, resource *localDocResource, nativeWidth, nativeHeight int) {
	modelScale, hasModelScale := positiveLocalDocImageFloatAttr(*tag, "scale")
	modelWidth, hasModelWidth := positiveLocalDocImageDisplaySizeAttr(*tag, "width", nativeWidth)
	modelHeight, hasModelHeight := positiveLocalDocImageDisplaySizeAttr(*tag, "height", nativeHeight)

	tag.setAttr("width", strconv.Itoa(nativeWidth))
	tag.setAttr("height", strconv.Itoa(nativeHeight))

	scale, hasScale := resolveLocalDocImageScale(
		nativeWidth,
		nativeHeight,
		modelScale,
		hasModelScale,
		modelWidth,
		hasModelWidth,
		modelHeight,
		hasModelHeight,
	)
	if !hasScale {
		tag.deleteAttr("scale")
		return
	}
	tag.setAttr("scale", strconv.FormatFloat(scale, 'f', 6, 64))
}

func resolveLocalDocImageScale(nativeWidth, nativeHeight int, modelScale float64, hasModelScale bool, modelWidth float64, hasModelWidth bool, modelHeight float64, hasModelHeight bool) (float64, bool) {
	var scale float64
	switch {
	case hasModelScale:
		scale = modelScale
	case hasModelWidth:
		scale = modelWidth / float64(nativeWidth)
	case hasModelHeight:
		scale = modelHeight / float64(nativeHeight)
	case nativeWidth >= localDocImageMaxDisplayWidthPx:
		scale = 1
	default:
		return 0, false
	}
	scale = floorLocalDocImageScalePrecision(scale)
	return capLocalDocImageScaleBelowPageWidth(nativeWidth, scale), true
}

func positiveLocalDocImageFloatAttr(tag localDocResourceTag, name string) (float64, bool) {
	value, ok := tag.attr(name)
	if !ok {
		return 0, false
	}
	return positiveLocalDocImageFloat(strings.TrimSpace(value))
}

func positiveLocalDocImageDisplaySizeAttr(tag localDocResourceTag, name string, nativeSize int) (float64, bool) {
	value, ok := tag.attr(name)
	if !ok {
		return 0, false
	}
	value = strings.TrimSpace(value)
	if strings.HasSuffix(value, "%") {
		percentage, valid := positiveLocalDocImageFloat(strings.TrimSpace(strings.TrimSuffix(value, "%")))
		if !valid {
			return 0, false
		}
		return float64(nativeSize) * percentage / 100, true
	}
	return positiveLocalDocImageFloat(value)
}

func positiveLocalDocImageFloat(value string) (float64, bool) {
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed <= 0 || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, false
	}
	return parsed, true
}

func floorLocalDocImageScalePrecision(scale float64) float64 {
	floored := math.Floor(scale*localDocImageScalePrecision) / localDocImageScalePrecision
	if floored <= 0 {
		return scale
	}
	return floored
}

func capLocalDocImageScaleBelowPageWidth(nativeWidth int, scale float64) float64 {
	maxScale := float64(localDocImageMaxDisplayWidthPx) / float64(nativeWidth)
	if scale < maxScale {
		return scale
	}
	capped := math.Floor(maxScale*localDocImageScalePrecision) / localDocImageScalePrecision
	if capped >= maxScale {
		capped -= 1.0 / localDocImageScalePrecision
	}
	if capped <= 0 {
		return math.Nextafter(maxScale, 0)
	}
	return capped
}

func newLocalDocResource(runtime *common.RuntimeContext, kind localDocResourceKind, pathValue string, occurrence int) (localDocResource, error) {
	pathValue = strings.TrimSpace(pathValue)
	if !strings.HasPrefix(pathValue, "@") {
		return localDocResource{}, localResourceValidationError(kind, occurrence, "path must start with @")
	}
	if isReservedLocalDocResourceMarker(pathValue) {
		return localDocResource{}, localResourceValidationError(kind, occurrence, "path uses a reserved lark-cli marker")
	}
	relPath := strings.TrimSpace(strings.TrimPrefix(pathValue, "@"))
	clean := filepath.Clean(relPath)
	if relPath == "" || filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return localDocResource{}, localResourceValidationError(kind, occurrence, "path must be a relative file inside the current working directory")
	}

	info, err := runtime.FileIO().Stat(clean)
	if err != nil {
		return localDocResource{}, localResourceValidationErrorWithCause(kind, occurrence, "file does not exist or its path is unsafe", err)
	}
	if !info.Mode().IsRegular() {
		return localDocResource{}, localResourceValidationError(kind, occurrence, "path must point to a regular file")
	}
	if info.Size() <= 0 {
		return localDocResource{}, localResourceValidationError(kind, occurrence, "file must not be empty")
	}
	file, err := runtime.FileIO().Open(clean)
	if err != nil {
		return localDocResource{}, localResourceValidationErrorWithCause(kind, occurrence, "file is not readable", err)
	}
	if err := file.Close(); err != nil {
		return localDocResource{}, localResourceValidationErrorWithCause(kind, occurrence, "file could not be closed after validation", err)
	}
	var imageWidth, imageHeight int
	if kind == localDocResourceImage {
		imageWidth, imageHeight, _, err = detectImageConfigFromPath(runtime.FileIO(), clean)
		if err != nil || imageWidth <= 0 || imageHeight <= 0 {
			if err == nil {
				err = invalidLocalDocImageDimensionsError()
			}
			return localDocResource{}, localResourceValidationErrorWithCause(kind, occurrence, "file is not a supported BMP, GIF, JPEG, PNG, TIFF, or WebP image", err)
		}
	}

	marker, err := newLocalDocResourceMarker(kind)
	if err != nil {
		return localDocResource{}, errs.NewInternalError(errs.SubtypeUnknown, "failed to generate local resource marker").WithCause(err)
	}
	return localDocResource{
		Occurrence:  occurrence,
		Kind:        kind,
		Marker:      marker,
		Path:        clean,
		FileName:    filepath.Base(clean),
		Size:        info.Size(),
		ImageWidth:  imageWidth,
		ImageHeight: imageHeight,
	}, nil
}

func invalidLocalDocImageDimensionsError() error {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, "decoded image has invalid dimensions")
}

func newLocalDocResourceMarker(kind localDocResourceKind) (string, error) {
	raw := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", err
	}
	prefix := "@lcli_img_"
	if kind == localDocResourceFile {
		prefix = "@lcli_file_"
	}
	return prefix + hex.EncodeToString(raw), nil
}

func localResourceValidationError(kind localDocResourceKind, occurrence int, reason string) error {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, "local %s #%d: %s", kind, occurrence, reason).WithParam("path")
}

func localResourceValidationErrorWithCause(kind localDocResourceKind, occurrence int, reason string, cause error) error {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, "local %s #%d: %s", kind, occurrence, reason).WithParam("path").WithCause(cause)
}

func validateLocalDocResourceUpdateCommand(command string, resources []localDocResource) error {
	if len(resources) == 0 || command == "append" || command == "block_insert_after" || command == "block_replace" || command == "overwrite" {
		return nil
	}
	return errs.NewValidationError(
		errs.SubtypeInvalidArgument,
		"local images and files are only supported with --command append, block_insert_after, block_replace, or overwrite",
	).WithParams(
		errs.InvalidParam{Name: "--command", Reason: "use append, block_insert_after, block_replace, or overwrite for local resources"},
		errs.InvalidParam{Name: "--content", Reason: "contains local image or file input"},
	)
}

func finalizeLocalDocResources(runtime *common.RuntimeContext, documentKey string, data map[string]interface{}, resources []localDocResource) error {
	if len(resources) == 0 {
		return nil
	}
	if data == nil {
		data = map[string]interface{}{}
	}
	outcomes := correlateLocalDocResources(data, resources)
	if strings.TrimSpace(documentKey) == "" {
		for _, outcome := range outcomes {
			outcome.Status = "correlation_failed"
			outcome.CleanupStatus = "skipped"
			outcome.Err = errs.NewInternalError(errs.SubtypeInvalidResponse, "document response is missing document_id")
		}
		appendLocalDocResourceFailures(data, outcomes)
		scrubLocalDocResourceResponseMarkers(data)
		return runtime.OutPartialFailure(data, nil)
	}

	uploadLocalDocResources(runtime, documentKey, outcomes)
	lastRevision := localDocResourceRevisionFromDocsAI(data)
	revisionKnown := lastRevision != nil
	bindRevision, bindRevisionKnown := bindLocalDocResources(runtime, documentKey, outcomes)
	if bindRevision != nil {
		lastRevision = bindRevision
		revisionKnown = true
	} else if !bindRevisionKnown {
		lastRevision = nil
		revisionKnown = false
	}
	cleanupRevision, cleanupRevisionKnown := cleanupLocalDocResourcePlaceholders(runtime, documentKey, outcomes, lastRevision)
	if cleanupRevision != nil {
		lastRevision = cleanupRevision
		revisionKnown = true
	} else if !cleanupRevisionKnown {
		lastRevision = nil
		revisionKnown = false
	}
	if revisionKnown && lastRevision != nil {
		setLocalDocResourceRevision(data, lastRevision)
	} else if !revisionKnown {
		clearLocalDocResourceRevision(data)
	}

	failed := false
	for _, outcome := range outcomes {
		if outcome.Status == "bound" {
			if outcome.Block != nil {
				outcome.Block["block_token"] = outcome.FileToken
			}
			continue
		}
		failed = true
		if outcome.Block != nil {
			delete(outcome.Block, "block_token")
		}
	}
	scrubLocalDocResourceResponseMarkers(data)
	if !failed {
		return nil
	}
	appendLocalDocResourceFailures(data, outcomes)
	return runtime.OutPartialFailure(data, nil)
}

func appendLocalDocResourceFailures(data map[string]interface{}, outcomes []*localDocResourceOutcome) {
	failures := make([]interface{}, 0)
	for _, outcome := range outcomes {
		if outcome == nil || outcome.Status == "bound" {
			continue
		}
		failure := map[string]interface{}{
			"occurrence":     outcome.Resource.Occurrence,
			"kind":           outcome.Resource.Kind,
			"status":         outcome.Status,
			"cleanup_status": outcome.CleanupStatus,
		}
		if problem, ok := errs.ProblemOf(outcome.Err); ok {
			detail := map[string]interface{}{
				"type":    problem.Category,
				"subtype": problem.Subtype,
			}
			if problem.Code != 0 {
				detail["code"] = problem.Code
			}
			if problem.Retryable {
				detail["retryable"] = true
			}
			failure["error"] = detail
		}
		if len(outcome.ServerWarnings) > 0 {
			failure["server_warnings"] = outcome.ServerWarnings
		}
		failures = append(failures, failure)
	}
	if len(failures) > 0 {
		data["local_resource_failures"] = failures
	}
}

func scrubLocalDocResourceResponseMarkers(data map[string]interface{}) {
	doc, _ := data["document"].(map[string]interface{})
	for _, raw := range common.GetSlice(doc, "new_blocks") {
		block, _ := raw.(map[string]interface{})
		if block == nil {
			continue
		}
		marker := strings.TrimSpace(common.GetString(block, "block_token"))
		if isReservedLocalDocResourceMarker(marker) {
			delete(block, "block_token")
		}
	}
}

func correlateLocalDocResources(data map[string]interface{}, resources []localDocResource) []*localDocResourceOutcome {
	outcomes := make([]*localDocResourceOutcome, len(resources))
	byMarker := make(map[string][]map[string]interface{}, len(resources))
	expectedMarkers := make(map[string]struct{}, len(resources))
	for _, resource := range resources {
		expectedMarkers[resource.Marker] = struct{}{}
	}
	unknownBlocks := make([]map[string]interface{}, 0)
	doc, _ := data["document"].(map[string]interface{})
	for _, raw := range common.GetSlice(doc, "new_blocks") {
		block, _ := raw.(map[string]interface{})
		if block == nil {
			continue
		}
		marker := strings.TrimSpace(common.GetString(block, "block_token"))
		if marker == "" {
			continue
		}
		if _, expected := expectedMarkers[marker]; expected {
			byMarker[marker] = append(byMarker[marker], block)
		} else if isReservedLocalDocResourceMarker(marker) {
			unknownBlocks = append(unknownBlocks, block)
		}
	}

	for i, resource := range resources {
		outcome := &localDocResourceOutcome{
			Resource:      resource,
			Status:        "pending",
			CleanupStatus: "not_needed",
		}
		matches := byMarker[resource.Marker]
		if len(matches) != 1 {
			outcome.Status = "correlation_failed"
			outcome.Err = errs.NewInternalError(errs.SubtypeInvalidResponse, "SDK returned %d blocks for local %s #%d; expected exactly one", len(matches), resource.Kind, resource.Occurrence)
			allBlocksMatchKind := true
			for _, block := range matches {
				if !localDocResourceBlockMatchesKind(block, resource.Kind) {
					allBlocksMatchKind = false
				}
				if id := strings.TrimSpace(common.GetString(block, "block_id")); id != "" {
					outcome.CleanupBlockIDs = append(outcome.CleanupBlockIDs, id)
				}
			}
			outcome.SafeToCleanup = len(outcome.CleanupBlockIDs) > 0 && allBlocksMatchKind
			if len(matches) == 0 || !allBlocksMatchKind {
				outcome.CleanupStatus = "skipped_ambiguous"
			}
			outcomes[i] = outcome
			continue
		}

		block := matches[0]
		blockID := strings.TrimSpace(common.GetString(block, "block_id"))
		blockType := strings.TrimSpace(common.GetString(block, "block_type"))
		outcome.Block = block
		outcome.BlockID = blockID
		blockMatchesKind := localDocResourceBlockMatchesKind(block, resource.Kind)
		if blockID != "" && blockMatchesKind {
			outcome.CleanupBlockIDs = []string{blockID}
			outcome.SafeToCleanup = true
		}
		if blockID == "" || !blockMatchesKind {
			outcome.Status = "correlation_failed"
			outcome.Err = errs.NewInternalError(errs.SubtypeInvalidResponse, "SDK returned an invalid block for local %s #%d", resource.Kind, resource.Occurrence)
			if blockID != "" && blockType != string(resource.Kind) {
				outcome.CleanupStatus = "skipped_ambiguous"
			}
		}
		outcomes[i] = outcome
	}
	if len(unknownBlocks) > 0 {
		for _, outcome := range outcomes {
			outcome.Status = "correlation_failed"
			outcome.CleanupStatus = "skipped_ambiguous"
			outcome.SafeToCleanup = false
			outcome.Err = errs.NewInternalError(errs.SubtypeInvalidResponse, "SDK returned unrecognized local resource markers; no local uploads were started")
		}
		return outcomes
	}
	byBlockID := make(map[string][]*localDocResourceOutcome, len(outcomes))
	for _, outcome := range outcomes {
		if outcome.BlockID != "" {
			byBlockID[outcome.BlockID] = append(byBlockID[outcome.BlockID], outcome)
		}
	}
	for blockID, conflicts := range byBlockID {
		if len(conflicts) < 2 {
			continue
		}
		blockIDSafeToCleanup := true
		for _, outcome := range conflicts {
			if !outcome.SafeToCleanup {
				blockIDSafeToCleanup = false
				break
			}
		}
		for i, outcome := range conflicts {
			outcome.Status = "correlation_failed"
			outcome.Err = errs.NewInternalError(errs.SubtypeInvalidResponse, "SDK correlated %d local resources to the same block_id", len(conflicts))
			if i == 0 && blockIDSafeToCleanup {
				found := false
				for _, cleanupBlockID := range outcome.CleanupBlockIDs {
					if cleanupBlockID == blockID {
						found = true
						break
					}
				}
				if !found {
					outcome.CleanupBlockIDs = append(outcome.CleanupBlockIDs, blockID)
				}
				continue
			}
			outcome.CleanupBlockIDs = nil
			outcome.SafeToCleanup = false
			if blockIDSafeToCleanup {
				outcome.CleanupStatus = "skipped_duplicate"
			} else {
				outcome.CleanupStatus = "skipped_ambiguous"
			}
		}
	}
	return outcomes
}

func uploadLocalDocResources(runtime *common.RuntimeContext, documentKey string, outcomes []*localDocResourceOutcome) {
	remoteOutcomes := make([]*localDocResourceOutcome, 0)
	localOutcomes := make([]*localDocResourceOutcome, 0, len(outcomes))
	for _, outcome := range outcomes {
		if outcome.Status != "pending" {
			continue
		}
		if outcome.Resource.RemoteURL != "" {
			remoteOutcomes = append(remoteOutcomes, outcome)
			continue
		}
		localOutcomes = append(localOutcomes, outcome)
	}
	uploadRemoteDocImages(runtime, documentKey, remoteOutcomes)
	uploadLocalDocResourcesSerially(runtime, documentKey, localOutcomes)
}

func uploadRemoteDocImages(runtime *common.RuntimeContext, documentKey string, outcomes []*localDocResourceOutcome) {
	if len(outcomes) == 0 {
		return
	}
	// Resolve credentials before fan-out so lazy credential-source selection
	// completes on the caller goroutine. Upload requests still resolve the
	// current token through the standard API path.
	if _, err := runtime.AccessToken(); err != nil {
		for _, outcome := range outcomes {
			markLocalDocResourceUploadFailed(outcome, err)
		}
		return
	}
	workerCount := min(remoteDocImageUploadConcurrency, len(outcomes))
	jobs := make(chan *localDocResourceOutcome)
	done := make(chan struct{}, workerCount)
	for range workerCount {
		go func() {
			defer func() { done <- struct{}{} }()
			for outcome := range jobs {
				uploadLocalDocResource(runtime, documentKey, outcome)
			}
		}()
	}
	for _, outcome := range outcomes {
		jobs <- outcome
	}
	close(jobs)
	for range workerCount {
		<-done
	}
}

func uploadLocalDocResourcesSerially(runtime *common.RuntimeContext, documentKey string, outcomes []*localDocResourceOutcome) {
	started := false
	for _, outcome := range outcomes {
		if started {
			if err := waitLocalDocResourceRequest(runtime.Ctx(), localDocResourceUploadInterval); err != nil {
				markLocalDocResourceUploadFailed(outcome, err)
				continue
			}
		}
		started = true
		uploadLocalDocResource(runtime, documentKey, outcome)
	}
}

func uploadLocalDocResource(runtime *common.RuntimeContext, documentKey string, outcome *localDocResourceOutcome) {
	var content []byte
	if outcome.Resource.RemoteURL != "" {
		outcome.Resource.Content = nil
		download, err := downloadRemoteDocImageWithRetry(runtime, outcome.Resource.RemoteURL, outcome.Resource.Occurrence)
		if err != nil {
			markLocalDocResourceUploadFailed(outcome, err)
			return
		}
		if err := applyRemoteDocImageDownload(&outcome.Resource, download); err != nil {
			markLocalDocResourceUploadFailed(outcome, err)
			return
		}
		content = download.Content
		outcome.Resource.Content = nil
		defer func() {
			content = nil
			outcome.Resource.Content = nil
		}()
	} else if len(outcome.Resource.Content) > 0 {
		content = outcome.Resource.Content
	}

	var uploadErr error
	for attempt := 0; attempt < localDocResourceUploadAttempts; attempt++ {
		upload := UploadDocMediaFileConfig{
			FilePath:   outcome.Resource.Path,
			FileName:   outcome.Resource.FileName,
			FileSize:   outcome.Resource.Size,
			ParentType: parentTypeForMediaType(string(outcome.Resource.Kind)),
			ParentNode: outcome.BlockID,
			DocID:      documentKey,
		}
		if len(content) > 0 {
			upload.Reader = bytes.NewReader(content)
		}
		token, err := uploadDocMediaFile(runtime, upload)
		if err == nil {
			outcome.FileToken = token
			outcome.Status = "uploaded"
			return
		}
		uploadErr = err
		if !isRetryableLocalDocResourceUpload(err) || attempt+1 >= localDocResourceUploadAttempts {
			break
		}
		delay := localDocResourceRetryDelay(localDocResourceUploadInterval, attempt)
		if waitErr := waitLocalDocResourceRequest(runtime.Ctx(), delay); waitErr != nil {
			uploadErr = errors.Join(uploadErr, waitErr)
			break
		}
	}
	markLocalDocResourceUploadFailed(outcome, uploadErr)
}

func downloadRemoteDocImageWithRetry(runtime *common.RuntimeContext, rawURL string, occurrence int) (remoteDocImageDownload, error) {
	var downloadErr error
	for attempt := 0; attempt < remoteDocImageDownloadAttempts; attempt++ {
		download, err := downloadRemoteDocImage(runtime, rawURL, occurrence)
		if err == nil {
			return download, nil
		}
		downloadErr = err
		if !isRetryableLocalDocResourceNetwork(err) || attempt+1 >= remoteDocImageDownloadAttempts {
			break
		}
		delay := localDocResourceRetryDelay(localDocResourceUploadInterval, attempt)
		if waitErr := waitLocalDocResourceRequest(runtime.Ctx(), delay); waitErr != nil {
			downloadErr = errors.Join(downloadErr, waitErr)
			break
		}
	}
	return remoteDocImageDownload{}, downloadErr
}

func markLocalDocResourceUploadFailed(outcome *localDocResourceOutcome, err error) {
	outcome.Status = "upload_failed"
	outcome.CleanupStatus = "pending"
	outcome.Err = err
}

func isRetryableLocalDocResourceUpload(err error) bool {
	if isRetryableLocalDocResourceNetwork(err) {
		return true
	}
	problem, ok := errs.ProblemOf(err)
	return ok && (problem.Code == localDocResourceUploadConflictCode || problem.Code == localDocResourceUploadRateLimitCode)
}

func isRetryableLocalDocResourceNetwork(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errs.IsRetryable(err) {
		return true
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		return false
	}
	if problem.Category != errs.CategoryNetwork {
		return false
	}
	return problem.Subtype == errs.SubtypeNetworkTransport ||
		problem.Subtype == errs.SubtypeNetworkTimeout ||
		problem.Subtype == errs.SubtypeNetworkServer ||
		problem.Subtype == errs.SubtypeRateLimit ||
		problem.Code == http.StatusTooManyRequests ||
		problem.Code >= http.StatusInternalServerError
}

func bindLocalDocResources(runtime *common.RuntimeContext, documentKey string, outcomes []*localDocResourceOutcome) (interface{}, bool) {
	ready := make([]*localDocResourceOutcome, 0, len(outcomes))
	for _, outcome := range outcomes {
		if outcome.Status == "uploaded" {
			ready = append(ready, outcome)
		}
	}
	var lastRevision interface{}
	revisionKnown := true
	for start := 0; start < len(ready); start += localDocResourceBindBatchSize {
		end := min(start+localDocResourceBindBatchSize, len(ready))
		chunk := ready[start:end]
		if start > 0 {
			if err := waitLocalDocResourceRequest(runtime.Ctx(), localDocResourceBindInterval); err != nil {
				markLocalDocResourceBindFailed(ready[start:], err)
				break
			}
		}
		revision, chunkRevisionKnown := bindLocalDocResourceChunk(runtime, documentKey, chunk)
		if revision != nil {
			lastRevision = revision
			revisionKnown = true
		} else if !chunkRevisionKnown {
			lastRevision = nil
			revisionKnown = false
		}
	}
	return lastRevision, revisionKnown
}

func bindLocalDocResourceChunk(runtime *common.RuntimeContext, documentKey string, chunk []*localDocResourceOutcome) (interface{}, bool) {
	clientToken := uuid.NewString()
	body := buildLocalDocResourceBatchUpdate(chunk)
	url := fmt.Sprintf("/open-apis/docx/v1/documents/%s/blocks/batch_update", validate.EncodePathSegment(documentKey))
	var lastErr error
	for attempt := 0; attempt < localDocResourceBindAttempts; attempt++ {
		data, err := runtime.CallAPITyped("PATCH", url, map[string]interface{}{"client_token": clientToken}, body)
		if err == nil {
			for _, outcome := range chunk {
				outcome.Status = "bound"
				outcome.CleanupStatus = "not_needed"
				outcome.SafeToCleanup = false
			}
			revision := localDocResourceRevisionFromBatch(data)
			return revision, revision != nil
		}
		lastErr = err
		allBound, hasConflict, hasUnknown := verifyLocalDocResourceChunk(runtime, documentKey, chunk, err)
		if allBound {
			return nil, false
		}
		if hasConflict || hasUnknown || !errs.IsRetryable(err) {
			break
		}
		if attempt+1 < localDocResourceBindAttempts {
			delay := localDocResourceBindInterval * time.Duration(1<<attempt)
			if waitErr := waitLocalDocResourceRequest(runtime.Ctx(), delay); waitErr != nil {
				lastErr = errors.Join(lastErr, waitErr)
				break
			}
		}
	}

	markLocalDocResourceBindFailed(chunk, lastErr)
	for _, outcome := range chunk {
		if outcome.Status == "bound" || outcome.Status == "bind_conflict" || outcome.Status == "bind_ambiguous" {
			return nil, false
		}
	}
	return nil, true
}

func markLocalDocResourceBindFailed(chunk []*localDocResourceOutcome, bindErr error) {
	for _, outcome := range chunk {
		if outcome.Status == "bound" || outcome.Status == "bind_conflict" || outcome.Status == "bind_ambiguous" {
			continue
		}
		outcome.Status = "bind_failed"
		outcome.CleanupStatus = "pending"
		outcome.SafeToCleanup = true
		outcome.Err = bindErr
	}
}

func verifyLocalDocResourceChunk(runtime *common.RuntimeContext, documentKey string, chunk []*localDocResourceOutcome, bindErr error) (allBound, hasConflict, hasUnknown bool) {
	allBound = true
	for i, outcome := range chunk {
		if i > 0 {
			if err := waitLocalDocResourceRequest(runtime.Ctx(), localDocResourceVerifyInterval); err != nil {
				outcome.Status = "bind_ambiguous"
				outcome.CleanupStatus = "skipped_ambiguous"
				outcome.SafeToCleanup = false
				outcome.Err = errors.Join(bindErr, err)
				allBound = false
				hasUnknown = true
				continue
			}
		}
		data, err := runtime.CallAPITyped("GET", fmt.Sprintf("/open-apis/docx/v1/documents/%s/blocks/%s", validate.EncodePathSegment(documentKey), validate.EncodePathSegment(outcome.BlockID)), nil, nil)
		if err != nil {
			outcome.Status = "bind_ambiguous"
			outcome.CleanupStatus = "skipped_ambiguous"
			outcome.SafeToCleanup = false
			outcome.Err = errors.Join(bindErr, err)
			allBound = false
			hasUnknown = true
			continue
		}
		actualToken := localDocResourceBlockToken(common.GetMap(data, "block"), outcome.Resource.Kind)
		switch actualToken {
		case outcome.FileToken:
			outcome.Status = "bound"
			outcome.CleanupStatus = "not_needed"
			outcome.SafeToCleanup = false
		case "":
			outcome.Status = "uploaded"
			outcome.SafeToCleanup = true
			allBound = false
		default:
			outcome.Status = "bind_conflict"
			outcome.CleanupStatus = "skipped_conflict"
			outcome.SafeToCleanup = false
			outcome.Err = errs.NewInternalError(errs.SubtypeInvalidResponse, "local %s #%d block token changed unexpectedly; placeholder was preserved", outcome.Resource.Kind, outcome.Resource.Occurrence)
			allBound = false
			hasConflict = true
		}
	}
	return allBound, hasConflict, hasUnknown
}

func buildLocalDocResourceBatchUpdate(chunk []*localDocResourceOutcome) map[string]interface{} {
	requests := make([]interface{}, 0, len(chunk))
	for _, outcome := range chunk {
		request := map[string]interface{}{"block_id": outcome.BlockID}
		if outcome.Resource.Kind == localDocResourceFile {
			request["replace_file"] = map[string]interface{}{"token": outcome.FileToken}
		} else {
			replaceImage := map[string]interface{}{"token": outcome.FileToken}
			if outcome.Resource.ImageWidth > 0 {
				replaceImage["width"] = outcome.Resource.ImageWidth
			}
			if outcome.Resource.ImageHeight > 0 {
				replaceImage["height"] = outcome.Resource.ImageHeight
			}
			if align, ok := alignMap[outcome.Resource.ImageAlign]; ok {
				replaceImage["align"] = align
			}
			if outcome.Resource.HasScale {
				replaceImage["scale"] = outcome.Resource.ImageScale
			}
			request["replace_image"] = replaceImage
		}
		requests = append(requests, request)
	}
	return map[string]interface{}{"requests": requests}
}

func cleanupLocalDocResourcePlaceholders(runtime *common.RuntimeContext, documentKey string, outcomes []*localDocResourceOutcome, baseRevision interface{}) (interface{}, bool) {
	type cleanupTarget struct {
		BlockID string
		Owner   *localDocResourceOutcome
	}
	targets := make([]cleanupTarget, 0)
	seenBlockIDs := make(map[string]struct{})
	for _, outcome := range outcomes {
		if outcome.Status == "bound" || !outcome.SafeToCleanup {
			continue
		}
		for _, blockID := range outcome.CleanupBlockIDs {
			blockID = strings.TrimSpace(blockID)
			if blockID == "" {
				continue
			}
			if _, duplicate := seenBlockIDs[blockID]; duplicate {
				continue
			}
			seenBlockIDs[blockID] = struct{}{}
			targets = append(targets, cleanupTarget{BlockID: blockID, Owner: outcome})
		}
		if len(outcome.CleanupBlockIDs) == 0 && outcome.CleanupStatus == "not_needed" {
			outcome.CleanupStatus = "skipped"
		}
	}
	if len(targets) == 0 {
		return nil, true
	}
	baseRevision = normalizeLocalDocResourceRevision(baseRevision)
	if baseRevision == nil {
		err := errs.NewInternalError(errs.SubtypeInvalidResponse, "document response is missing a revision; local resource placeholders were preserved")
		for _, target := range targets {
			target.Owner.CleanupStatus = "skipped_ambiguous"
			target.Owner.SafeToCleanup = false
			appendLocalDocResourceOutcomeError(target.Owner, err)
		}
		return nil, false
	}

	verifiedTargets := make([]cleanupTarget, 0, len(targets))
	blockedOwners := make(map[*localDocResourceOutcome]struct{})
	for i, target := range targets {
		if i > 0 {
			if err := waitLocalDocResourceRequest(runtime.Ctx(), localDocResourceVerifyInterval); err != nil {
				target.Owner.CleanupStatus = "skipped_ambiguous"
				target.Owner.SafeToCleanup = false
				blockedOwners[target.Owner] = struct{}{}
				appendLocalDocResourceOutcomeError(target.Owner, err)
				continue
			}
		}
		data, err := runtime.CallAPITyped("GET", fmt.Sprintf("/open-apis/docx/v1/documents/%s/blocks/%s", validate.EncodePathSegment(documentKey), validate.EncodePathSegment(target.BlockID)), nil, nil)
		if err != nil {
			target.Owner.Status = "bind_ambiguous"
			target.Owner.CleanupStatus = "skipped_ambiguous"
			target.Owner.SafeToCleanup = false
			blockedOwners[target.Owner] = struct{}{}
			appendLocalDocResourceOutcomeError(target.Owner, err)
			continue
		}
		block := common.GetMap(data, "block")
		if !localDocResourceBlockMatchesKind(block, target.Owner.Resource.Kind) {
			blockedOwners[target.Owner] = struct{}{}
			target.Owner.Status = "bind_ambiguous"
			target.Owner.CleanupStatus = "skipped_ambiguous"
			target.Owner.SafeToCleanup = false
			appendLocalDocResourceOutcomeError(target.Owner, errs.NewInternalError(
				errs.SubtypeInvalidResponse,
				"local %s #%d block type could not be verified before cleanup; placeholder was preserved",
				target.Owner.Resource.Kind,
				target.Owner.Resource.Occurrence,
			))
			continue
		}

		actualTokens := localDocResourceBlockTokens(block)
		if len(actualTokens) == 0 {
			if target.Owner.Resource.Kind == localDocResourceFile {
				// DocX represents a file as a source child inside a figure
				// block. Deleting the source ID is accepted by docs_ai but
				// leaves an empty <figure><source/></figure> behind, so remove
				// the owning figure after the tokenless child is verified.
				parentID := strings.TrimSpace(common.GetString(block, "parent_id"))
				if parentID == "" || parentID == target.BlockID || parentID == documentKey {
					blockedOwners[target.Owner] = struct{}{}
					target.Owner.Status = "bind_ambiguous"
					target.Owner.CleanupStatus = "skipped_ambiguous"
					target.Owner.SafeToCleanup = false
					appendLocalDocResourceOutcomeError(target.Owner, errs.NewInternalError(
						errs.SubtypeInvalidResponse,
						"local file #%d figure parent could not be verified before cleanup; placeholder was preserved",
						target.Owner.Resource.Occurrence,
					))
					continue
				}
				parentData, err := runtime.CallAPITyped("GET", fmt.Sprintf("/open-apis/docx/v1/documents/%s/blocks/%s", validate.EncodePathSegment(documentKey), validate.EncodePathSegment(parentID)), nil, nil)
				if err != nil {
					blockedOwners[target.Owner] = struct{}{}
					target.Owner.Status = "bind_ambiguous"
					target.Owner.CleanupStatus = "skipped_ambiguous"
					target.Owner.SafeToCleanup = false
					appendLocalDocResourceOutcomeError(target.Owner, err)
					continue
				}
				parentBlock := common.GetMap(parentData, "block")
				if !localDocResourceIsSoleFileFigure(parentBlock, parentID, target.BlockID) {
					blockedOwners[target.Owner] = struct{}{}
					target.Owner.Status = "bind_ambiguous"
					target.Owner.CleanupStatus = "skipped_ambiguous"
					target.Owner.SafeToCleanup = false
					appendLocalDocResourceOutcomeError(target.Owner, errs.NewInternalError(
						errs.SubtypeInvalidResponse,
						"local file #%d parent is not a sole-source figure; placeholder was preserved",
						target.Owner.Resource.Occurrence,
					))
					continue
				}
				target.BlockID = parentID
			}
			verifiedTargets = append(verifiedTargets, target)
			continue
		}

		blockedOwners[target.Owner] = struct{}{}
		target.Owner.SafeToCleanup = false
		if target.Owner.FileToken != "" && len(actualTokens) == 1 && actualTokens[0] == target.Owner.FileToken {
			target.Owner.Status = "bound"
			target.Owner.CleanupStatus = "not_needed"
			if target.Owner.Block != nil {
				target.Owner.Block["block_token"] = target.Owner.FileToken
			}
			continue
		}
		target.Owner.Status = "bind_conflict"
		target.Owner.CleanupStatus = "skipped_conflict"
		appendLocalDocResourceOutcomeError(target.Owner, errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"local %s #%d block token changed before cleanup; placeholder was preserved",
			target.Owner.Resource.Kind,
			target.Owner.Resource.Occurrence,
		))
	}
	verifiedByBlockID := make(map[string]cleanupTarget, len(verifiedTargets))
	for _, target := range verifiedTargets {
		if previous, duplicate := verifiedByBlockID[target.BlockID]; duplicate {
			blockedOwners[previous.Owner] = struct{}{}
			blockedOwners[target.Owner] = struct{}{}
			for _, owner := range []*localDocResourceOutcome{previous.Owner, target.Owner} {
				owner.Status = "bind_ambiguous"
				owner.CleanupStatus = "skipped_ambiguous"
				owner.SafeToCleanup = false
				appendLocalDocResourceOutcomeError(owner, errs.NewInternalError(
					errs.SubtypeInvalidResponse,
					"multiple local resources resolved to cleanup block %s; placeholders were preserved",
					target.BlockID,
				))
			}
			continue
		}
		verifiedByBlockID[target.BlockID] = target
	}
	deleteTargets := verifiedTargets[:0]
	for _, target := range verifiedTargets {
		if _, blocked := blockedOwners[target.Owner]; !blocked {
			deleteTargets = append(deleteTargets, target)
		}
	}
	if len(deleteTargets) == 0 {
		return nil, false
	}

	var lastRevision interface{}
	currentRevision := baseRevision
	for start := 0; start < len(deleteTargets); start += localDocResourceCleanupBatchSize {
		end := min(start+localDocResourceCleanupBatchSize, len(deleteTargets))
		chunk := deleteTargets[start:end]
		ids := make([]string, 0, len(chunk))
		for _, target := range chunk {
			ids = append(ids, target.BlockID)
		}
		body := map[string]interface{}{
			"format":      "xml",
			"command":     "block_delete",
			"block_id":    strings.Join(ids, ","),
			"revision_id": currentRevision,
		}
		injectDocsScene(runtime, body)
		data, err := doDocAPI(runtime, "PUT", fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s", validate.EncodePathSegment(documentKey)), body)
		if err != nil {
			for _, target := range chunk {
				target.Owner.CleanupStatus = "failed"
				target.Owner.SafeToCleanup = false
				appendLocalDocResourceOutcomeError(target.Owner, err)
			}
			for _, target := range deleteTargets[end:] {
				target.Owner.CleanupStatus = "skipped_ambiguous"
				target.Owner.SafeToCleanup = false
				appendLocalDocResourceOutcomeError(target.Owner, errs.NewInternalError(errs.SubtypeInvalidResponse, "cleanup revision could not be confirmed; placeholder was preserved"))
			}
			return nil, false
		}
		if docsAPIOperationFailed(data) {
			serviceErr := errs.NewAPIError(errs.SubtypeUnknown, "local resource placeholder cleanup returned result=failed")
			warnings := common.GetSlice(data, "warnings")
			for _, target := range chunk {
				target.Owner.CleanupStatus = "failed"
				target.Owner.SafeToCleanup = false
				target.Owner.ServerWarnings = append(target.Owner.ServerWarnings, warnings...)
				appendLocalDocResourceOutcomeError(target.Owner, serviceErr)
			}
			for _, target := range deleteTargets[end:] {
				target.Owner.CleanupStatus = "skipped_ambiguous"
				target.Owner.SafeToCleanup = false
				appendLocalDocResourceOutcomeError(target.Owner, errs.NewInternalError(errs.SubtypeInvalidResponse, "cleanup revision could not be confirmed; placeholder was preserved"))
			}
			return nil, false
		}
		for _, target := range chunk {
			target.Owner.CleanupStatus = "succeeded"
			target.Owner.SafeToCleanup = false
		}
		revision := localDocResourceRevisionFromDocsAI(data)
		if revision == nil {
			for _, target := range deleteTargets[end:] {
				target.Owner.CleanupStatus = "skipped_ambiguous"
				target.Owner.SafeToCleanup = false
				appendLocalDocResourceOutcomeError(target.Owner, errs.NewInternalError(errs.SubtypeInvalidResponse, "cleanup response is missing a revision; placeholder was preserved"))
			}
			return nil, false
		}
		lastRevision = revision
		currentRevision = revision
	}
	return lastRevision, true
}

func appendLocalDocResourceOutcomeError(outcome *localDocResourceOutcome, err error) {
	if err == nil {
		return
	}
	if outcome.Err == nil {
		outcome.Err = err
		return
	}
	outcome.Err = errors.Join(outcome.Err, err)
}

func localDocResourceBlockToken(block map[string]interface{}, kind localDocResourceKind) string {
	if block == nil {
		return ""
	}
	if token := strings.TrimSpace(common.GetString(block, "token")); token != "" {
		return token
	}
	return strings.TrimSpace(common.GetString(common.GetMap(block, string(kind)), "token"))
}

func localDocResourceBlockMatchesKind(block map[string]interface{}, kind localDocResourceKind) bool {
	if block == nil {
		return false
	}
	rawType, ok := block["block_type"]
	if !ok {
		return false
	}
	if blockType, ok := rawType.(string); ok {
		return strings.TrimSpace(blockType) == string(kind)
	}
	blockType, ok := normalizeLocalDocResourceRevision(rawType).(int64)
	return ok && blockType == int64(blockTypeForMediaType(string(kind)))
}

func localDocResourceIsSoleFileFigure(block map[string]interface{}, expectedBlockID, childBlockID string) bool {
	if block == nil || strings.TrimSpace(common.GetString(block, "block_id")) != expectedBlockID {
		return false
	}
	rawType, ok := block["block_type"]
	if !ok {
		return false
	}
	figureType := false
	if blockType, ok := rawType.(string); ok {
		switch strings.ToLower(strings.TrimSpace(blockType)) {
		case "figure", "view":
			figureType = true
		}
	} else if blockType, ok := normalizeLocalDocResourceRevision(rawType).(int64); ok {
		figureType = blockType == 33
	}
	if !figureType {
		return false
	}
	children := common.GetSlice(block, "children")
	if len(children) != 1 {
		return false
	}
	child, ok := children[0].(string)
	return ok && strings.TrimSpace(child) == childBlockID
}

func localDocResourceBlockTokens(block map[string]interface{}) []string {
	if block == nil {
		return nil
	}
	tokens := make([]string, 0, 3)
	seen := make(map[string]struct{}, 3)
	for _, token := range []string{
		common.GetString(block, "token"),
		common.GetString(common.GetMap(block, string(localDocResourceImage)), "token"),
		common.GetString(common.GetMap(block, string(localDocResourceFile)), "token"),
	} {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if _, duplicate := seen[token]; duplicate {
			continue
		}
		seen[token] = struct{}{}
		tokens = append(tokens, token)
	}
	return tokens
}

func localDocResourceRevisionFromBatch(data map[string]interface{}) interface{} {
	if data == nil {
		return nil
	}
	return normalizeLocalDocResourceRevision(data["document_revision_id"])
}

func localDocResourceRevisionFromDocsAI(data map[string]interface{}) interface{} {
	doc, _ := data["document"].(map[string]interface{})
	if doc == nil {
		return nil
	}
	return normalizeLocalDocResourceRevision(doc["revision_id"])
}

func normalizeLocalDocResourceRevision(value interface{}) interface{} {
	var revision int64
	switch number := value.(type) {
	case int:
		revision = int64(number)
	case int8:
		revision = int64(number)
	case int16:
		revision = int64(number)
	case int32:
		revision = int64(number)
	case int64:
		revision = number
	case uint:
		if uint64(number) > math.MaxInt64 {
			return nil
		}
		revision = int64(number)
	case uint8:
		revision = int64(number)
	case uint16:
		revision = int64(number)
	case uint32:
		revision = int64(number)
	case uint64:
		if number > math.MaxInt64 {
			return nil
		}
		revision = int64(number)
	case float32:
		value := float64(number)
		if value > math.MaxInt64 || value < 0 || math.Trunc(value) != value {
			return nil
		}
		revision = int64(value)
	case float64:
		if number > math.MaxInt64 || number < 0 || math.Trunc(number) != number {
			return nil
		}
		revision = int64(number)
	case json.Number:
		parsed, err := strconv.ParseInt(number.String(), 10, 64)
		if err != nil {
			return nil
		}
		revision = parsed
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(number), 10, 64)
		if err != nil {
			return nil
		}
		revision = parsed
	default:
		return nil
	}
	if revision < 0 {
		return nil
	}
	return revision
}

func setLocalDocResourceRevision(data map[string]interface{}, revision interface{}) {
	doc, _ := data["document"].(map[string]interface{})
	if doc != nil {
		if revision = normalizeLocalDocResourceRevision(revision); revision != nil {
			doc["revision_id"] = revision
		}
	}
}

func clearLocalDocResourceRevision(data map[string]interface{}) {
	doc, _ := data["document"].(map[string]interface{})
	if doc != nil {
		delete(doc, "revision_id")
	}
}

func appendLocalDocResourcesDryRun(dry *common.DryRunAPI, documentKey string, resources []localDocResource) *common.DryRunAPI {
	if len(resources) == 0 {
		return dry
	}
	encodedDocumentKey := validate.EncodePathSegment(documentKey)
	routeExtra, _ := buildDriveRouteExtra(documentKey)
	for _, resource := range resources {
		parentType := parentTypeForMediaType(string(resource.Kind))
		body := map[string]interface{}{
			"file_name":   fmt.Sprintf("<local_%s_%d_filename>", resource.Kind, resource.Occurrence),
			"parent_type": parentType,
			"parent_node": fmt.Sprintf("<local_%s_%d_block_id>", resource.Kind, resource.Occurrence),
			"size":        resource.Size,
			"extra":       routeExtra,
		}
		if resource.Size > common.MaxDriveMediaUploadSinglePartSize {
			dry.POST("/open-apis/drive/v1/medias/upload_prepare").
				Desc(fmt.Sprintf("Upload local %s #%d: initialize multipart upload", resource.Kind, resource.Occurrence)).
				Body(body).
				POST("/open-apis/drive/v1/medias/upload_part").
				Desc(fmt.Sprintf("Upload local %s #%d: upload parts (repeated)", resource.Kind, resource.Occurrence)).
				Body(map[string]interface{}{"upload_id": "<upload_id>", "seq": "<chunk_index>", "size": "<chunk_size>", "file": "<local_resource_binary>"}).
				POST("/open-apis/drive/v1/medias/upload_finish").
				Desc(fmt.Sprintf("Upload local %s #%d: finish multipart upload", resource.Kind, resource.Occurrence)).
				Body(map[string]interface{}{"upload_id": "<upload_id>", "block_num": "<block_num>"})
		} else {
			body["file"] = "<local_resource_binary>"
			dry.POST("/open-apis/drive/v1/medias/upload_all").
				Desc(fmt.Sprintf("Upload local %s #%d", resource.Kind, resource.Occurrence)).
				Body(body)
		}
	}
	for start := 0; start < len(resources); start += localDocResourceBindBatchSize {
		end := min(start+localDocResourceBindBatchSize, len(resources))
		requests := make([]interface{}, 0, end-start)
		for _, resource := range resources[start:end] {
			request := map[string]interface{}{"block_id": fmt.Sprintf("<local_%s_%d_block_id>", resource.Kind, resource.Occurrence)}
			if resource.Kind == localDocResourceFile {
				request["replace_file"] = map[string]interface{}{"token": fmt.Sprintf("<uploaded_file_token_%d>", resource.Occurrence)}
			} else {
				replaceImage := map[string]interface{}{"token": fmt.Sprintf("<uploaded_file_token_%d>", resource.Occurrence)}
				if resource.ImageWidth > 0 {
					replaceImage["width"] = resource.ImageWidth
				}
				if resource.ImageHeight > 0 {
					replaceImage["height"] = resource.ImageHeight
				}
				if align, ok := alignMap[resource.ImageAlign]; ok {
					replaceImage["align"] = align
				}
				if resource.HasScale {
					replaceImage["scale"] = resource.ImageScale
				}
				request["replace_image"] = replaceImage
			}
			requests = append(requests, request)
		}
		dry.PATCH(fmt.Sprintf("/open-apis/docx/v1/documents/%s/blocks/batch_update", encodedDocumentKey)).
			Desc(fmt.Sprintf("Bind uploaded local resources (batch %d, max %d requests)", start/localDocResourceBindBatchSize+1, localDocResourceBindBatchSize)).
			Params(map[string]interface{}{"client_token": fmt.Sprintf("<stable_client_token_%d>", start/localDocResourceBindBatchSize+1)}).
			Body(map[string]interface{}{"requests": requests})
	}
	dry.GET(fmt.Sprintf("/open-apis/docx/v1/documents/%s/blocks/%s", encodedDocumentKey, validate.EncodePathSegment("<local_resource_block_id>"))).
		Desc("Conditional: verify block token after an ambiguous bind response or immediately before cleanup")
	dry.PUT(fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s", encodedDocumentKey)).
		Desc("Conditional: delete placeholders whose upload or bind failed; successful resources are preserved").
		Body(map[string]interface{}{"format": "xml", "command": "block_delete", "block_id": "<failed_placeholder_block_ids>", "revision_id": "<known_revision_id>"})
	return dry
}

func appendRemoteDocImageDownloadsDryRun(dry *common.DryRunAPI, resources []localDocResource) *common.DryRunAPI {
	for _, resource := range resources {
		if resource.RemoteURL == "" {
			continue
		}
		dry.GET(redactRemoteDocImageURL(resource.RemoteURL)).
			Desc(fmt.Sprintf("Download remote image #%d in a bounded concurrent upload worker after the document write succeeds (userinfo, query, and fragment are redacted)", resource.Occurrence))
	}
	return dry
}

func redactRemoteDocImageURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed == nil {
		return "<invalid-remote-image-url>"
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	return parsed.String()
}

func (t localDocResourceTag) attr(name string) (string, bool) {
	for _, attr := range t.Attrs {
		if attr.Name == name {
			return attr.Value, true
		}
	}
	return "", false
}

func (t localDocResourceTag) hasAttr(name string) bool {
	_, ok := t.attr(name)
	return ok
}

func (t *localDocResourceTag) setAttr(name, value string) {
	for i := range t.Attrs {
		if t.Attrs[i].Name == name {
			t.Attrs[i].Value = value
			return
		}
	}
	t.Attrs = append(t.Attrs, html5BlockAttr{Name: name, Value: value})
}

func (t *localDocResourceTag) deleteAttr(name string) {
	attrs := t.Attrs[:0]
	for _, attr := range t.Attrs {
		if attr.Name != name {
			attrs = append(attrs, attr)
		}
	}
	t.Attrs = attrs
}

func (t *localDocResourceTag) renameAttr(oldName, newName string) bool {
	if t.hasAttr(newName) {
		return false
	}
	for i := range t.Attrs {
		if t.Attrs[i].Name == oldName {
			t.Attrs[i].Name = newName
			return true
		}
	}
	return false
}

type markdownIndentContext struct {
	quoteDepth         int
	listContentIndents []int
}

func (c *markdownIndentContext) isIndentedCodeLine(line string) bool {
	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	content, quoteDepth := stripMarkdownBlockQuotePrefixes(line)
	if quoteDepth != c.quoteDepth {
		c.quoteDepth = quoteDepth
		c.listContentIndents = nil
	}
	if strings.TrimSpace(content) == "" {
		return false
	}

	indent, offset := markdownLeadingIndent(content)
	for len(c.listContentIndents) > 0 && indent < c.listContentIndents[len(c.listContentIndents)-1] {
		c.listContentIndents = c.listContentIndents[:len(c.listContentIndents)-1]
	}
	if indent <= 3 && isMarkdownThematicBreak(content[offset:]) {
		return false
	}
	if markerIndent, ok := markdownListItemContentIndent(content[offset:], indent); ok && c.enterListItem(indent, markerIndent) {
		return false
	}

	containerIndent := 0
	if len(c.listContentIndents) > 0 {
		containerIndent = c.listContentIndents[len(c.listContentIndents)-1]
	}
	return indent >= containerIndent+4
}

func (c *markdownIndentContext) enterListItem(indent, contentIndent int) bool {
	for len(c.listContentIndents) > 0 {
		parentIndent := c.listContentIndents[len(c.listContentIndents)-1]
		if indent >= parentIndent && indent <= parentIndent+3 {
			c.listContentIndents = append(c.listContentIndents, contentIndent)
			return true
		}
		if indent < parentIndent {
			c.listContentIndents = c.listContentIndents[:len(c.listContentIndents)-1]
			continue
		}
		return false
	}
	if indent > 3 {
		return false
	}
	c.listContentIndents = append(c.listContentIndents[:0], contentIndent)
	return true
}

func stripMarkdownBlockQuotePrefixes(line string) (string, int) {
	depth := 0
	for {
		indent := 0
		for indent < len(line) && indent < 3 && line[indent] == ' ' {
			indent++
		}
		if indent >= len(line) || line[indent] != '>' {
			return line, depth
		}
		line = line[indent+1:]
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			line = line[1:]
		}
		depth++
	}
}

func markdownLeadingIndent(line string) (columns, offset int) {
	for offset < len(line) {
		switch line[offset] {
		case ' ':
			columns++
		case '\t':
			columns += 4 - columns%4
		default:
			return columns, offset
		}
		offset++
	}
	return columns, offset
}

func markdownListItemContentIndent(line string, markerColumn int) (int, bool) {
	markerWidth := 0
	if len(line) > 0 && (line[0] == '-' || line[0] == '+' || line[0] == '*') {
		markerWidth = 1
	} else {
		for markerWidth < len(line) && markerWidth < 9 && line[markerWidth] >= '0' && line[markerWidth] <= '9' {
			markerWidth++
		}
		if markerWidth == 0 || markerWidth >= len(line) || (line[markerWidth] != '.' && line[markerWidth] != ')') {
			return 0, false
		}
		markerWidth++
	}
	if markerWidth == len(line) {
		return markerColumn + markerWidth + 1, true
	}
	if line[markerWidth] != ' ' && line[markerWidth] != '\t' {
		return 0, false
	}

	paddingColumns := 0
	column := markerColumn + markerWidth
	for i := markerWidth; i < len(line); i++ {
		switch line[i] {
		case ' ':
			column++
			paddingColumns++
		case '\t':
			width := 4 - column%4
			column += width
			paddingColumns += width
		default:
			if paddingColumns > 4 {
				paddingColumns = 1
			}
			return markerColumn + markerWidth + paddingColumns, true
		}
	}
	if paddingColumns > 4 {
		paddingColumns = 1
	}
	return markerColumn + markerWidth + paddingColumns, true
}

func isMarkdownThematicBreak(line string) bool {
	marker := byte(0)
	count := 0
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case ' ', '\t':
			continue
		case '*', '-', '_':
			if marker == 0 {
				marker = line[i]
			} else if line[i] != marker {
				return false
			}
			count++
		default:
			return false
		}
	}
	return count >= 3
}

func findMarkdownRawHTMLInertEnd(content string, start int) (int, bool) {
	if start < 0 || start >= len(content) || content[start] != '<' || start+2 >= len(content) {
		return 0, false
	}
	for _, name := range []string{"pre", "script", "style", "textarea"} {
		nameEnd := start + 1 + len(name)
		if nameEnd >= len(content) || !strings.EqualFold(content[start+1:nameEnd], name) || !isLocalDocResourceHTMLTagBoundary(content[nameEnd]) {
			continue
		}
		startTagEnd := findXMLStartTagEnd(content, start)
		if startTagEnd < 0 {
			return len(content), true
		}
		if strings.HasSuffix(strings.TrimSpace(content[start:startTagEnd]), "/>") {
			return startTagEnd, true
		}

		searchFrom := startTagEnd
		lowerRest := strings.ToLower(content[searchFrom:])
		needle := "</" + name
		for {
			offset := strings.Index(lowerRest, needle)
			if offset < 0 {
				return len(content), true
			}
			closeStart := searchFrom + offset
			boundary := closeStart + len(needle)
			if boundary < len(content) && isLocalDocResourceHTMLTagBoundary(content[boundary]) {
				closeEnd := findXMLStartTagEnd(content, closeStart)
				if closeEnd < 0 {
					return len(content), true
				}
				return closeEnd, true
			}
			searchFrom = boundary
			lowerRest = strings.ToLower(content[searchFrom:])
		}
	}
	return 0, false
}

func isLocalDocResourceHTMLTagBoundary(value byte) bool {
	return value == '>' || value == '/' || value == ' ' || value == '\t' || value == '\r' || value == '\n'
}

func (t localDocResourceTag) render() string {
	var out strings.Builder
	out.WriteByte('<')
	out.WriteString(t.Name)
	for _, attr := range t.Attrs {
		out.WriteByte(' ')
		out.WriteString(attr.Name)
		out.WriteString(`="`)
		out.WriteString(escapeXMLAttr(attr.Value))
		out.WriteByte('"')
	}
	if t.SelfClosing {
		out.WriteString("/>")
	} else {
		out.WriteByte('>')
	}
	return out.String()
}

func parseLocalDocResourceTag(raw, expected string) (localDocResourceTag, error) {
	if expected == "img" {
		raw = escapeBareXMLAmpersandsInTagAttr(raw, "href")
	}
	decoder := xml.NewDecoder(strings.NewReader(raw))
	for {
		token, err := decoder.Token()
		if err != nil {
			return localDocResourceTag{}, err
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local != expected {
			return localDocResourceTag{}, fmt.Errorf("expected <%s>, got <%s>", expected, start.Name.Local) //nolint:forbidigo // caller wraps with typed validation error.
		}
		attrs := make([]html5BlockAttr, 0, len(start.Attr))
		seenAttrs := make(map[string]struct{}, len(start.Attr))
		for _, attr := range start.Attr {
			attrName := strings.ToLower(strings.TrimSpace(attr.Name.Local))
			if _, exists := seenAttrs[attrName]; exists {
				return localDocResourceTag{}, fmt.Errorf("duplicate attribute %q", attrName) //nolint:forbidigo // caller wraps with typed validation error.
			}
			seenAttrs[attrName] = struct{}{}
			attrs = append(attrs, html5BlockAttr{Name: attr.Name.Local, Value: attr.Value})
		}
		return localDocResourceTag{Name: expected, Attrs: attrs, SelfClosing: strings.HasSuffix(strings.TrimSpace(raw), "/>")}, nil
	}
}

func escapeBareXMLAmpersandsInTagAttr(raw, targetAttr string) string {
	for i := 1; i < len(raw); {
		for i < len(raw) && isXMLSpace(raw[i]) {
			i++
		}
		nameStart := i
		for i < len(raw) && isLocalDocResourceAttrNameByte(raw[i]) {
			i++
		}
		if nameStart == i {
			i++
			continue
		}
		name := raw[nameStart:i]
		for i < len(raw) && isXMLSpace(raw[i]) {
			i++
		}
		if i >= len(raw) || raw[i] != '=' {
			continue
		}
		i++
		for i < len(raw) && isXMLSpace(raw[i]) {
			i++
		}
		if i >= len(raw) || (raw[i] != '"' && raw[i] != '\'') {
			continue
		}
		quote := raw[i]
		valueStart := i + 1
		valueEnd := strings.IndexByte(raw[valueStart:], quote)
		if valueEnd < 0 {
			return raw
		}
		valueEnd += valueStart
		if strings.EqualFold(name, targetAttr) {
			value := escapeBareXMLAmpersands(raw[valueStart:valueEnd])
			return raw[:valueStart] + value + raw[valueEnd:]
		}
		i = valueEnd + 1
	}
	return raw
}

func isXMLSpace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\r' || value == '\n'
}

func isLocalDocResourceAttrNameByte(value byte) bool {
	return value > ' ' && value != '=' && value != '/' && value != '>'
}

func escapeBareXMLAmpersands(value string) string {
	var out strings.Builder
	for i := 0; i < len(value); {
		if value[i] != '&' {
			out.WriteByte(value[i])
			i++
			continue
		}
		if entityLen := validXMLCharacterEntityLength(value[i:]); entityLen > 0 {
			out.WriteString(value[i : i+entityLen])
			i += entityLen
			continue
		}
		out.WriteString("&amp;")
		i++
	}
	return out.String()
}

func validXMLCharacterEntityLength(value string) int {
	for _, entity := range []string{"&amp;", "&lt;", "&gt;", "&quot;", "&apos;"} {
		if strings.HasPrefix(value, entity) {
			return len(entity)
		}
	}
	if !strings.HasPrefix(value, "&#") {
		return 0
	}
	i := 2
	isDigit := func(value byte) bool { return value >= '0' && value <= '9' }
	if i < len(value) && (value[i] == 'x' || value[i] == 'X') {
		i++
		isDigit = func(value byte) bool {
			return (value >= '0' && value <= '9') || (value >= 'a' && value <= 'f') || (value >= 'A' && value <= 'F')
		}
	}
	digitsStart := i
	for i < len(value) && isDigit(value[i]) {
		i++
	}
	if i == digitsStart || i >= len(value) || value[i] != ';' {
		return 0
	}
	return i + 1
}

func localResourceTagNameAt(content string, index int) string {
	for _, name := range []string{"img", "source"} {
		prefix := "<" + name
		if !strings.HasPrefix(content[index:], prefix) {
			continue
		}
		next := index + len(prefix)
		if next >= len(content) || content[next] == '>' || content[next] == '/' || content[next] == ' ' || content[next] == '\t' || content[next] == '\r' || content[next] == '\n' {
			return name
		}
	}
	return ""
}

func findXMLStartTagEnd(content string, start int) int {
	var quote byte
	for i := start + 1; i < len(content); i++ {
		if quote != 0 {
			if content[i] == quote {
				quote = 0
			}
			continue
		}
		switch content[i] {
		case '\'', '"':
			quote = content[i]
		case '>':
			return i + 1
		}
	}
	return -1
}

type parsedMarkdownImage struct {
	Alt            string
	Destination    string
	Title          string
	ReferenceLabel string
	End            int
}

func parseMarkdownImageAt(content string, start int) (parsedMarkdownImage, bool) {
	altEnd := findMarkdownClosingBracket(content, start+2)
	if altEnd < 0 {
		return parsedMarkdownImage{}, false
	}
	alt := content[start+2 : altEnd]
	next := altEnd + 1
	if next < len(content) && content[next] == '(' {
		destination, title, end, ok := parseMarkdownImageDestination(content, next)
		if !ok {
			return parsedMarkdownImage{}, false
		}
		return parsedMarkdownImage{Alt: alt, Destination: destination, Title: title, End: end}, true
	}
	if next < len(content) && content[next] == '[' {
		labelEnd := findMarkdownClosingBracket(content, next+1)
		if labelEnd < 0 {
			return parsedMarkdownImage{}, false
		}
		label := content[next+1 : labelEnd]
		if label == "" {
			label = alt
		}
		return parsedMarkdownImage{Alt: alt, ReferenceLabel: label, End: labelEnd + 1}, true
	}
	return parsedMarkdownImage{Alt: alt, ReferenceLabel: alt, End: altEnd + 1}, true
}

func parseMarkdownImageDestination(content string, open int) (string, string, int, bool) {
	i := open + 1
	for i < len(content) && (content[i] == ' ' || content[i] == '\t') {
		i++
	}
	if i >= len(content) {
		return "", "", 0, false
	}
	var destination string
	if content[i] == '<' {
		start := i + 1
		i++
		for i < len(content) {
			if content[i] == '>' && !isEscapedMarkdownByte(content, i) {
				destination = content[start:i]
				i++
				break
			}
			i++
		}
		if destination == "" {
			return "", "", 0, false
		}
	} else {
		start := i
		depth := 0
		for i < len(content) {
			if isEscapedMarkdownByte(content, i) {
				i++
				continue
			}
			switch content[i] {
			case '(':
				depth++
			case ')':
				if depth == 0 {
					destination = content[start:i]
					return unescapeMarkdownText(strings.TrimSpace(destination)), "", i + 1, destination != ""
				}
				depth--
			case ' ', '\t', '\r', '\n':
				if depth == 0 {
					destination = content[start:i]
					goto findClose
				}
			}
			i++
		}
		return "", "", 0, false
	}

findClose:
	for i < len(content) && isMarkdownSpace(content[i]) {
		i++
	}
	if i >= len(content) {
		return "", "", 0, false
	}
	if content[i] == ')' {
		return unescapeMarkdownText(strings.TrimSpace(destination)), "", i + 1, destination != ""
	}

	opener := content[i]
	closer := opener
	if opener == '(' {
		closer = ')'
	} else if opener != '\'' && opener != '"' {
		return "", "", 0, false
	}
	titleStart := i + 1
	i = titleStart
	for i < len(content) {
		if content[i] == closer && !isEscapedMarkdownByte(content, i) {
			title := unescapeMarkdownText(content[titleStart:i])
			i++
			for i < len(content) && isMarkdownSpace(content[i]) {
				i++
			}
			if i >= len(content) || content[i] != ')' {
				return "", "", 0, false
			}
			return unescapeMarkdownText(strings.TrimSpace(destination)), title, i + 1, destination != ""
		}
		i++
	}
	return "", "", 0, false
}

func isMarkdownSpace(char byte) bool {
	return char == ' ' || char == '\t' || char == '\r' || char == '\n'
}

func collectLocalMarkdownImageReferences(content string) map[string]struct{} {
	refs := map[string]struct{}{}
	var fenceChar byte
	var fenceLen int
	var indentCtx markdownIndentContext
	for _, line := range strings.Split(maskLocalDocResourceMarkupInertContexts(content), "\n") {
		indentedCode := false
		if fenceChar == 0 {
			indentedCode = indentCtx.isIndentedCodeLine(line)
		}
		char, run, isFence := markdownFence(line)
		if fenceChar == 0 && isFence {
			fenceChar, fenceLen = char, run
			continue
		}
		if fenceChar != 0 {
			if isFence && char == fenceChar && run >= fenceLen && markdownFenceCloses(line, char, run) {
				fenceChar, fenceLen = 0, 0
			}
			continue
		}
		if indentedCode {
			continue
		}
		label, destination, ok := parseMarkdownReferenceDefinition(line)
		if ok && strings.HasPrefix(destination, "@") {
			refs[normalizeMarkdownReferenceLabel(label)] = struct{}{}
		}
	}
	return refs
}

func maskLocalDocResourceMarkupInertContexts(content string) string {
	var out strings.Builder
	for i := 0; i < len(content); {
		if end, ok := findMarkdownRawHTMLInertEnd(content, i); ok {
			writeLocalDocResourceMaskedSpan(&out, content[i:end])
			i = end
			continue
		}
		terminator := ""
		prefixLen := 0
		switch {
		case strings.HasPrefix(content[i:], "<!--"):
			terminator = "-->"
			prefixLen = 4
		case strings.HasPrefix(content[i:], "<![CDATA["):
			terminator = "]]>"
			prefixLen = 9
		}
		if terminator == "" {
			out.WriteByte(content[i])
			i++
			continue
		}
		end := strings.Index(content[i+prefixLen:], terminator)
		if end < 0 {
			end = len(content)
		} else {
			end += i + prefixLen + len(terminator)
		}
		writeLocalDocResourceMaskedSpan(&out, content[i:end])
		i = end
	}
	return out.String()
}

func writeLocalDocResourceMaskedSpan(out *strings.Builder, content string) {
	for _, char := range content {
		if char == '\n' || char == '\r' {
			out.WriteRune(char)
		} else {
			out.WriteByte(' ')
		}
	}
}

func parseMarkdownReferenceDefinition(line string) (string, string, bool) {
	trimmed := strings.TrimLeft(line, " \t")
	if len(line)-len(trimmed) > 3 || !strings.HasPrefix(trimmed, "[") {
		return "", "", false
	}
	end := findMarkdownClosingBracket(trimmed, 1)
	if end < 0 || end+1 >= len(trimmed) || trimmed[end+1] != ':' {
		return "", "", false
	}
	rest := strings.TrimSpace(trimmed[end+2:])
	if strings.HasPrefix(rest, "<") {
		close := strings.Index(rest, ">")
		if close < 0 {
			return "", "", false
		}
		return trimmed[1:end], rest[1:close], true
	}
	if field := strings.Fields(rest); len(field) > 0 {
		return trimmed[1:end], unescapeMarkdownText(field[0]), true
	}
	return "", "", false
}

func normalizeMarkdownReferenceLabel(label string) string {
	return strings.ToLower(strings.Join(strings.Fields(unescapeMarkdownText(label)), " "))
}

func unescapeMarkdownText(value string) string {
	var out strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' && i+1 < len(value) && isMarkdownEscapablePunctuation(value[i+1]) {
			i++
		}
		out.WriteByte(value[i])
	}
	return out.String()
}

func isMarkdownEscapablePunctuation(value byte) bool {
	return (value >= '!' && value <= '/') || (value >= ':' && value <= '@') ||
		(value >= '[' && value <= '`') || (value >= '{' && value <= '~')
}

func findMarkdownClosingBracket(content string, start int) int {
	depth := 0
	for i := start; i < len(content); i++ {
		if isEscapedMarkdownByte(content, i) {
			continue
		}
		switch content[i] {
		case '[':
			depth++
		case ']':
			if depth == 0 {
				return i
			}
			depth--
		}
	}
	return -1
}

func isEscapedMarkdownByte(content string, index int) bool {
	backslashes := 0
	for i := index - 1; i >= 0 && content[i] == '\\'; i-- {
		backslashes++
	}
	return backslashes%2 == 1
}

func countByteRun(content string, start int, value byte) int {
	i := start
	for i < len(content) && content[i] == value {
		i++
	}
	return i - start
}

func findMatchingBacktickRun(content string, start, run int) int {
	for i := start; i < len(content); {
		if content[i] != '`' {
			i++
			continue
		}
		got := countByteRun(content, i, '`')
		if got == run {
			return i + got
		}
		i += got
	}
	return -1
}

func markdownFence(line string) (byte, int, bool) {
	trimmed, ok := markdownFenceCandidate(line)
	if !ok || len(trimmed) < 3 {
		return 0, 0, false
	}
	char := trimmed[0]
	if char != '`' && char != '~' {
		return 0, 0, false
	}
	run := countByteRun(trimmed, 0, char)
	return char, run, run >= 3
}

func markdownFenceCloses(line string, char byte, run int) bool {
	trimmed, ok := markdownFenceCandidate(line)
	return ok && len(trimmed) >= run && strings.TrimSpace(trimmed[run:]) == "" && trimmed[0] == char
}

func markdownFenceCandidate(line string) (string, bool) {
	rest := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	for {
		indent := 0
		for indent < len(rest) && rest[indent] == ' ' {
			indent++
		}
		if indent > 3 || (indent < len(rest) && rest[indent] == '\t') {
			return "", false
		}
		rest = rest[indent:]
		if strings.HasPrefix(rest, ">") {
			rest = rest[1:]
			if strings.HasPrefix(rest, " ") || strings.HasPrefix(rest, "\t") {
				rest = rest[1:]
			}
			continue
		}
		if prefixLen := markdownListMarkerPrefixLen(rest); prefixLen > 0 {
			rest = rest[prefixLen:]
			continue
		}
		return rest, true
	}
}

func markdownListMarkerPrefixLen(line string) int {
	if len(line) >= 2 && (line[0] == '-' || line[0] == '+' || line[0] == '*') && (line[1] == ' ' || line[1] == '\t') {
		return 2
	}
	digits := 0
	for digits < len(line) && digits < 9 && line[digits] >= '0' && line[digits] <= '9' {
		digits++
	}
	if digits == 0 || digits+1 >= len(line) || (line[digits] != '.' && line[digits] != ')') || (line[digits+1] != ' ' && line[digits+1] != '\t') {
		return 0
	}
	return digits + 2
}
