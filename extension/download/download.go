// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package download provides validated full and ranged streams for extensions
// and built-in commands. It owns transfer bounds, retries, representation
// consistency, progress timeouts, and response-length checks; callers supply a
// Transport that owns authentication and source/URL policy, and they choose the
// destination that consumes Stream.Body.
package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
)

const (
	// DefaultPartSize keeps a 100 MiB object near 13 requests while capping replay at 8 MiB.
	DefaultPartSize = int64(8 * 1024 * 1024)
	// DefaultPartRetries tolerates three transient failures without prolonged retrying.
	DefaultPartRetries = 3
	// DefaultRetryDelay keeps three local backoffs below one second before jitter.
	DefaultRetryDelay = 100 * time.Millisecond
	// DefaultRetryWaitBudget caps cumulative local sleep for interactive callers.
	DefaultRetryWaitBudget = 3 * time.Second
	// DefaultIdleTimeout detects a dead connection within a minute without limiting slow progress.
	DefaultIdleTimeout = 60 * time.Second
)

// ByteRange is an inclusive HTTP byte range.
type ByteRange struct {
	Start int64
	End   int64

	_ struct{}
}

// HeaderValue returns the value for an HTTP Range header.
func (r ByteRange) HeaderValue() string {
	return fmt.Sprintf("bytes=%d-%d", r.Start, r.End)
}

// Request describes one full or ranged fetch.
type Request struct {
	Range   *ByteRange
	IfRange string

	_ struct{}
}

// Headers keeps byte offsets tied to the transferred representation.
func (r Request) Headers() http.Header {
	headers := make(http.Header, 3)
	headers.Set("Accept-Encoding", "identity")
	if r.Range != nil {
		headers.Set("Range", r.Range.HeaderValue())
	}
	if r.IfRange != "" {
		headers.Set("If-Range", r.IfRange)
	}
	return headers
}

// Transport performs one replay-safe fetch and binds ctx to the response body.
// Non-successful HTTP responses must be returned as typed errors with
// retryability decided at this boundary.
type Transport func(context.Context, Request) (*http.Response, error)

// Options controls multipart behavior. Zero values select production defaults.
type Options struct {
	// PartSize is the maximum byte count requested per range. Zero selects
	// DefaultPartSize.
	PartSize int64
	// MaxResponses bounds the total responses in one logical stream. Zero derives
	// a bound from the declared object size and PartSize.
	MaxResponses int
	// MaxPartRetries bounds retries per range. Zero selects the default.
	MaxPartRetries int
	// RetryDelay is the base exponential backoff. Zero selects the default.
	RetryDelay time.Duration
	// RetryWaitBudget bounds cumulative retry sleeps. Zero selects the default.
	RetryWaitBudget time.Duration
	// IdleTimeout bounds waiting for response headers or one body read. Time
	// between caller reads does not count. Zero selects the default.
	IdleTimeout time.Duration
	// DisableMultipart forces one full response.
	DisableMultipart bool

	_ struct{}
}

// Stream is one logical full or multipart response.
type Stream struct {
	// Body joins all validated parts and must be closed by the caller.
	Body io.ReadCloser
	// Header is a copy of the first successful response headers.
	Header http.Header
	// ContentLength is the validated total size, or -1 when unknown.
	ContentLength int64

	_ struct{}
}

// Open probes range support and returns one validated stream.
func Open(ctx context.Context, source Source, opts Options) (*Stream, error) {
	opts = opts.withDefaults()
	if err := validateOptions(source, opts); err != nil {
		return nil, err
	}
	retryWait := newRetryWaitBudget(opts.RetryWaitBudget)
	if opts.DisableMultipart {
		return openFull(ctx, source.transport, opts, retryWait)
	}

	firstPart := ByteRange{Start: 0, End: opts.PartSize - 1}
	resp, err := fetchWithRetry(ctx, source.transport, Request{Range: &firstPart}, opts, retryWait)
	if err != nil {
		if !rangeProbeRejected(err) {
			return nil, err
		}
		return openFull(ctx, source.transport, opts, retryWait)
	}
	if resp == nil || resp.Body == nil {
		return nil, errs.NewNetworkError(errs.SubtypeNetworkProtocol, "range fetch returned an empty response")
	}

	switch resp.StatusCode {
	case http.StatusOK:
		return singleStream(ctx, resp)
	case http.StatusPartialContent:
		return openPartial(ctx, source, opts, retryWait, firstPart, resp)
	case http.StatusBadRequest, http.StatusRequestedRangeNotSatisfiable:
		resp.Body.Close()
		return openFull(ctx, source.transport, opts, retryWait)
	default:
		resp.Body.Close()
		return nil, unexpectedStatus(resp.StatusCode)
	}
}

func openFull(ctx context.Context, fetch Transport, opts Options, retryWait *retryWaitBudget) (*Stream, error) {
	resp, err := fetchWithRetry(ctx, fetch, Request{}, opts, retryWait)
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Body == nil {
		return nil, protocolError("full-response fallback returned an empty response")
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, protocolError("full-response fallback returned HTTP %d, want 200", resp.StatusCode)
	}
	return singleStream(ctx, resp)
}

func rangeProbeRejected(err error) bool {
	problem, ok := errs.ProblemOf(err)
	return ok && (problem.Code == http.StatusBadRequest || problem.Code == http.StatusRequestedRangeNotSatisfiable)
}

func fetchWithRetry(ctx context.Context, fetch Transport, request Request, opts Options, retryWait *retryWaitBudget) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= opts.MaxPartRetries; attempt++ {
		resp, err := fetchWithIdleTimeout(ctx, fetch, request, opts.IdleTimeout)
		if err == nil {
			return resp, nil
		}
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, downloadContextError(ctx.Err())
		}
		if attempt == opts.MaxPartRetries || !downloadRetryable(ctx, err) {
			return nil, err
		}
		if waitErr := waitForRetry(ctx, retryWait, opts.RetryDelay, attempt, err); waitErr != nil {
			return nil, waitErr
		}
	}
	return nil, lastErr
}

func downloadContextError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return errs.NewNetworkError(errs.SubtypeNetworkTimeout, "download timed out").WithCause(err)
	}
	return errs.NewNetworkError(errs.SubtypeNetworkTransport, "download canceled").WithCause(err)
}

func downloadRetryable(ctx context.Context, err error) bool {
	return err != nil && ctx.Err() == nil && errs.IsRetryable(err)
}

func waitForRetry(ctx context.Context, retryWait *retryWaitBudget, base time.Duration, attempt int, retryErr error) error {
	delay := addRetryJitter(retryDelay(base, attempt, retryErr))
	return retryWait.wait(ctx, delay, retryErr)
}

type retryWaitBudget struct {
	remaining time.Duration
}

func newRetryWaitBudget(limit time.Duration) *retryWaitBudget {
	return &retryWaitBudget{remaining: limit}
}

func (b *retryWaitBudget) wait(ctx context.Context, delay time.Duration, retryErr error) error {
	if delay <= 0 {
		return nil
	}
	if b == nil || delay > b.remaining {
		return retryErr
	}
	b.remaining -= delay
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return downloadContextError(ctx.Err())
	case <-timer.C:
		return nil
	}
}

func retryDelay(base time.Duration, attempt int, retryErr error) time.Duration {
	local := exponentialDelay(base, attempt)
	if upstream, ok := errs.RetryAfter(retryErr); ok && upstream > local {
		return upstream
	}
	return local
}

func exponentialDelay(base time.Duration, attempt int) time.Duration {
	if base <= 0 {
		return 0
	}
	shift := min(max(attempt, 0), 4)
	factor := int64(1 << uint(shift))
	if base > time.Duration(math.MaxInt64/factor) {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(int64(base) * factor)
}

// addRetryJitter never shortens an upstream retry delay.
func addRetryJitter(delay time.Duration) time.Duration {
	if delay <= 0 {
		return 0
	}
	window := min(delay/10, 250*time.Millisecond)
	if window <= 0 {
		return delay
	}
	extra := time.Duration(rand.Int64N(int64(window) + 1))
	if delay > time.Duration(math.MaxInt64)-extra {
		return time.Duration(math.MaxInt64)
	}
	return delay + extra
}

func (o Options) withDefaults() Options {
	if o.PartSize == 0 {
		o.PartSize = DefaultPartSize
	}
	if o.MaxPartRetries == 0 {
		o.MaxPartRetries = DefaultPartRetries
	}
	if o.RetryDelay == 0 {
		o.RetryDelay = DefaultRetryDelay
	}
	if o.RetryWaitBudget == 0 {
		o.RetryWaitBudget = DefaultRetryWaitBudget
	}
	if o.IdleTimeout == 0 {
		o.IdleTimeout = DefaultIdleTimeout
	}
	return o
}

func validateOptions(source Source, opts Options) error {
	if source.transport == nil {
		return errs.NewInternalError(errs.SubtypeUnknown, "download requires a configured transport")
	}
	if source.representation != Immutable && source.representation != Mutable {
		return errs.NewInternalError(errs.SubtypeUnknown, "download requires an explicit representation contract")
	}
	if opts.PartSize <= 0 {
		return errs.NewInternalError(errs.SubtypeUnknown, "download part size must be positive, got %d", opts.PartSize)
	}
	if opts.MaxResponses < 0 {
		return errs.NewInternalError(errs.SubtypeUnknown, "download max responses cannot be negative, got %d", opts.MaxResponses)
	}
	if opts.MaxPartRetries < 0 {
		return errs.NewInternalError(errs.SubtypeUnknown, "download max part retries cannot be negative, got %d", opts.MaxPartRetries)
	}
	if opts.RetryDelay < 0 {
		return errs.NewInternalError(errs.SubtypeUnknown, "download retry delay cannot be negative, got %s", opts.RetryDelay)
	}
	if opts.RetryWaitBudget < 0 {
		return errs.NewInternalError(errs.SubtypeUnknown, "download retry wait budget cannot be negative, got %s", opts.RetryWaitBudget)
	}
	if opts.IdleTimeout < 0 {
		return errs.NewInternalError(errs.SubtypeUnknown, "download idle timeout cannot be negative, got %s", opts.IdleTimeout)
	}
	return nil
}

func singleStream(ctx context.Context, resp *http.Response) (*Stream, error) {
	if err := validateResponseEncoding(resp); err != nil {
		resp.Body.Close()
		return nil, err
	}
	contentLength := resp.ContentLength
	if contentLength < 0 || (contentLength == 0 && resp.Header.Get("Content-Length") == "") {
		contentLength = -1
	}
	return &Stream{
		Body:          newExactLengthReader(newClassifiedBody(ctx, resp.Body), contentLength),
		Header:        resp.Header.Clone(),
		ContentLength: contentLength,
	}, nil
}

func openPartial(ctx context.Context, source Source, opts Options, retryWait *retryWaitBudget, requested ByteRange, resp *http.Response) (*Stream, error) {
	if err := validateResponseEncoding(resp); err != nil {
		resp.Body.Close()
		return nil, err
	}
	first, err := parseContentRange(resp.Header.Get("Content-Range"))
	if err != nil {
		resp.Body.Close()
		return nil, protocolError("invalid Content-Range header on range response: %s", err)
	}
	if first.start != 0 {
		resp.Body.Close()
		return nil, protocolError("range response is %s, want it to start at byte 0", first)
	}
	if first.end > requested.End {
		resp.Body.Close()
		return nil, protocolError("range response is %s, outside requested %s", first, requested.HeaderValue())
	}

	session := newRepresentationSession(source, first, resp.Header)
	completeInFirstResponse := first.end == first.total-1
	if !completeInFirstResponse && !session.multipartAllowed() {
		resp.Body.Close()
		return openFull(ctx, source.transport, opts, retryWait)
	}

	limit := opts.MaxResponses
	if limit == 0 {
		limit = responseLimit(first.total, opts.PartSize)
	}
	body := &sequentialPartReader{
		ctx:            ctx,
		session:        session,
		opts:           opts,
		current:        resp.Body,
		chunkSize:      opts.PartSize,
		chunkWant:      first.length(),
		responses:      1,
		maxResponses:   limit,
		maxPartRetries: opts.MaxPartRetries,
		retryDelay:     opts.RetryDelay,
		retryWait:      retryWait,
	}
	return &Stream{
		Body:          newExactLengthReader(body, first.total),
		Header:        resp.Header.Clone(),
		ContentLength: first.total,
	}, nil
}

type sequentialPartReader struct {
	ctx            context.Context
	session        *representationSession
	opts           Options
	current        io.ReadCloser
	chunkSize      int64
	chunkWant      int64
	chunkRead      int64
	responses      int
	maxResponses   int
	nextOffset     int64
	maxPartRetries int
	partRetries    int
	retryDelay     time.Duration
	retryWait      *retryWaitBudget
	retryErr       error
	retryPending   bool
	closed         bool
}

func (r *sequentialPartReader) Read(p []byte) (int, error) {
	if r.closed {
		return 0, io.ErrClosedPipe
	}
	for {
		if r.current != nil {
			n, err := r.current.Read(p)
			r.chunkRead += int64(n)
			r.nextOffset += int64(n)
			if r.chunkRead > r.chunkWant {
				return 0, r.closeAfter(protocolError("range response delivered more than the %d bytes its framing declared", r.chunkWant))
			}

			switch err {
			case nil:
				return n, nil
			case io.EOF:
				closeErr := r.current.Close()
				r.current = nil
				if r.chunkRead != r.chunkWant {
					failure := r.readFailure(io.ErrUnexpectedEOF, closeErr)
					if r.scheduleBodyRetry(failure) {
						if n > 0 {
							return n, nil
						}
						continue
					}
					return n, failure
				}
				// Closing cannot invalidate a complete framed response.
				r.partRetries = 0
				if r.nextOffset == r.session.totalSize {
					if n > 0 {
						return n, nil
					}
					return 0, io.EOF
				}
				r.chunkRead = 0
				r.chunkWant = 0
				if n > 0 {
					return n, nil
				}
			default:
				closeErr := r.current.Close()
				r.current = nil
				failure := r.readFailure(err, closeErr)
				if r.scheduleBodyRetry(failure) {
					if n > 0 {
						return n, nil
					}
					continue
				}
				return n, failure
			}
		}

		if r.nextOffset >= r.session.totalSize {
			if r.nextOffset == r.session.totalSize {
				return 0, io.EOF
			}
			return 0, protocolError("file size mismatch: expected %d, got %d", r.session.totalSize, r.nextOffset)
		}
		if r.responses >= r.maxResponses {
			return 0, protocolError(
				"server split the resource into more than %d range responses; giving up after %d of %d bytes",
				r.maxResponses, r.nextOffset, r.session.totalSize,
			)
		}
		if r.retryPending {
			if err := r.waitBeforeRetry(); err != nil {
				return 0, err
			}
			r.retryPending = false
			r.retryErr = nil
		}

		if err := r.openNext(); err != nil {
			return 0, err
		}
	}
}

func (r *sequentialPartReader) scheduleBodyRetry(failure error) bool {
	if !r.canRetryBody(failure) || r.partRetries >= r.maxPartRetries {
		return false
	}
	r.retryErr = failure
	r.partRetries++
	r.retryPending = true
	r.chunkRead = 0
	r.chunkWant = 0
	return true
}

func (r *sequentialPartReader) openNext() error {
	end := r.session.totalSize - 1
	if remaining := r.session.totalSize - r.nextOffset; r.chunkSize < remaining {
		end = r.nextOffset + r.chunkSize - 1
	}
	requested := ByteRange{Start: r.nextOffset, End: end}
	resp, err := fetchWithRetry(r.ctx, r.session.transport, r.session.request(requested), r.opts, r.retryWait)
	if err != nil {
		return err
	}
	if resp == nil || resp.Body == nil {
		return protocolError("range fetch returned an empty response")
	}

	r.responses++
	if resp.StatusCode == http.StatusOK {
		return r.continueFullResponse(resp)
	}
	if resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		return unexpectedStatus(resp.StatusCode)
	}
	if err := validateResponseEncoding(resp); err != nil {
		resp.Body.Close()
		return err
	}

	got, parseErr := parseContentRange(resp.Header.Get("Content-Range"))
	if parseErr != nil {
		resp.Body.Close()
		return protocolError("invalid Content-Range header on range response: %s", parseErr)
	}
	if validateErr := r.session.validatePartial(resp.Header, got, requested); validateErr != nil {
		resp.Body.Close()
		return validateErr
	}
	if got.end < r.nextOffset {
		resp.Body.Close()
		return protocolError("range response is %s and makes no progress past byte %d", got, r.nextOffset)
	}

	r.current = resp.Body
	r.chunkWant = got.length()
	r.chunkRead = 0
	return nil
}

// continueFullResponse uses a peer's full response after it stops honoring Range.
func (r *sequentialPartReader) continueFullResponse(resp *http.Response) error {
	if err := validateResponseEncoding(resp); err != nil {
		resp.Body.Close()
		return err
	}
	if validateErr := r.session.validateFull(resp); validateErr != nil {
		resp.Body.Close()
		return validateErr
	}
	body := newClassifiedBody(r.ctx, resp.Body)
	if _, err := io.CopyN(io.Discard, body, r.nextOffset); err != nil {
		body.Close()
		if _, typed := errs.ProblemOf(err); typed {
			return err
		}
		return protocolError("full-response continuation ended before byte %d: %s", r.nextOffset, err).WithCause(err)
	}
	r.current = body
	r.chunkWant = r.session.totalSize - r.nextOffset
	r.chunkRead = 0
	return nil
}

func (r *sequentialPartReader) canRetryBody(err error) bool {
	return r.session.multipartAllowed() && downloadRetryable(r.ctx, err)
}

func (r *sequentialPartReader) readFailure(readErr, closeErr error) error {
	if r.ctx.Err() != nil || errors.Is(readErr, context.Canceled) || errors.Is(readErr, context.DeadlineExceeded) {
		return classifyBodyReadError(r.ctx, readErr, readErr)
	}
	if _, typed := errs.ProblemOf(readErr); typed {
		return readErr
	}
	cause := readErr
	if closeErr != nil {
		cause = errors.Join(readErr, closeErr)
	}
	if errors.Is(readErr, io.ErrUnexpectedEOF) {
		return errs.NewNetworkError(errs.SubtypeNetworkTransport,
			"range response body ended after %d of the %d bytes its framing declared", r.chunkRead, r.chunkWant).
			WithRetryable().
			WithHint("retry the download; discard any partial destination output").
			WithCause(cause)
	}
	return classifyBodyReadError(r.ctx, readErr, cause)
}

func (r *sequentialPartReader) waitBeforeRetry() error {
	delay := addRetryJitter(exponentialDelay(r.retryDelay, r.partRetries-1))
	return r.retryWait.wait(r.ctx, delay, r.retryErr)
}

func (r *sequentialPartReader) closeAfter(primary *errs.NetworkError) error {
	if r.current == nil {
		return primary
	}
	closeErr := r.current.Close()
	r.current = nil
	if closeErr != nil {
		return primary.WithCause(closeErr)
	}
	return primary
}

func (r *sequentialPartReader) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	if r.current == nil {
		return nil
	}
	err := r.current.Close()
	r.current = nil
	return err
}

func unexpectedStatus(status int) error {
	return protocolError("transport returned unexpected HTTP status %d", status).WithCode(status)
}

func protocolError(format string, args ...any) *errs.NetworkError {
	return errs.NewNetworkError(errs.SubtypeNetworkProtocol, format, args...)
}

func representationChanged(format string, args ...any) *errs.NetworkError {
	return errs.NewNetworkError(errs.SubtypeNetworkRepresentationChanged, format, args...).
		WithRetryable().
		WithHint("run the command again to download the current version")
}

// responseLimit bounds amplification while allowing smaller server ranges.
func responseLimit(totalSize, chunkSize int64) int {
	expected := (totalSize-1)/chunkSize + 1
	const (
		floor      = int64(64)
		multiplier = int64(4)
	)
	maxInt := int64(^uint(0) >> 1)
	if expected > maxInt/multiplier {
		return int(maxInt)
	}
	limit := expected * multiplier
	if limit < floor {
		limit = floor
	}
	return int(limit)
}

// contentRange is a parsed `Content-Range: bytes start-end/total` header.
type contentRange struct {
	start int64
	end   int64
	total int64
}

func (cr contentRange) String() string {
	return fmt.Sprintf("bytes %d-%d/%d", cr.start, cr.end, cr.total)
}

func (cr contentRange) length() int64 {
	return cr.end - cr.start + 1
}

func parseContentRange(header string) (contentRange, error) {
	header = strings.TrimSpace(header)
	if header == "" {
		return contentRange{}, fmt.Errorf("content-range is empty")
	}
	unit, spec, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(unit, "bytes") {
		return contentRange{}, fmt.Errorf("unsupported content-range: %q", header)
	}
	parts := strings.SplitN(strings.TrimSpace(spec), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || parts[0] == "*" {
		return contentRange{}, fmt.Errorf("unsupported content-range: %q", header)
	}
	if parts[1] == "*" {
		return contentRange{}, fmt.Errorf("unknown total size in content-range: %q", header)
	}
	bounds := strings.SplitN(parts[0], "-", 2)
	if len(bounds) != 2 || bounds[0] == "" || bounds[1] == "" {
		return contentRange{}, fmt.Errorf("unsupported content-range: %q", header)
	}
	start, err := strconv.ParseInt(bounds[0], 10, 64)
	if err != nil {
		return contentRange{}, fmt.Errorf("parse range start: %w", err)
	}
	end, err := strconv.ParseInt(bounds[1], 10, 64)
	if err != nil {
		return contentRange{}, fmt.Errorf("parse range end: %w", err)
	}
	total, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return contentRange{}, fmt.Errorf("parse total size: %w", err)
	}
	if total <= 0 {
		return contentRange{}, fmt.Errorf("invalid total size: %d", total)
	}
	if start < 0 || end < 0 {
		return contentRange{}, fmt.Errorf("invalid negative content range: %d-%d", start, end)
	}
	if start > end {
		return contentRange{}, fmt.Errorf("invalid content range: start %d is after end %d", start, end)
	}
	if end >= total {
		return contentRange{}, fmt.Errorf("invalid content range: end %d is outside total %d", end, total)
	}
	return contentRange{start: start, end: end, total: total}, nil
}

// strongETag returns a validator suitable for If-Range.
func strongETag(header http.Header) (string, bool) {
	values := header.Values("ETag")
	if len(values) != 1 {
		return "", false
	}
	tag := strings.TrimSpace(values[0])
	if len(tag) < 2 || tag[0] != '"' || tag[len(tag)-1] != '"' {
		return "", false
	}
	for i := 1; i < len(tag)-1; i++ {
		if c := tag[i]; c != 0x21 && !(c >= 0x23 && c <= 0x7E) && c < 0x80 {
			return "", false
		}
	}
	return tag, true
}
