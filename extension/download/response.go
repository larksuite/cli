// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/larksuite/cli/errs"
)

type classifiedBody struct {
	ctx    context.Context
	source io.ReadCloser
}

func newClassifiedBody(ctx context.Context, source io.ReadCloser) io.ReadCloser {
	return &classifiedBody{ctx: ctx, source: source}
}

func (r *classifiedBody) Read(p []byte) (int, error) {
	n, err := r.source.Read(p)
	if err == nil || err == io.EOF {
		return n, err
	}
	return n, classifyBodyReadError(r.ctx, err, err)
}

func (r *classifiedBody) Close() error { return r.source.Close() }

func classifyBodyReadError(ctx context.Context, readErr, cause error) error {
	subtype := errs.SubtypeNetworkTransport
	message := fmt.Sprintf("reading the resource failed: %s", readErr)
	var netErr net.Error
	switch {
	case errors.Is(readErr, context.DeadlineExceeded), errors.Is(ctx.Err(), context.DeadlineExceeded):
		subtype = errs.SubtypeNetworkTimeout
		message = "download request timed out"
	case errors.Is(readErr, context.Canceled), errors.Is(ctx.Err(), context.Canceled):
		message = "download request was canceled"
	default:
		if _, typed := errs.ProblemOf(readErr); typed {
			return readErr
		}
		if errors.As(readErr, &netErr) && netErr.Timeout() {
			subtype = errs.SubtypeNetworkTimeout
			message = "download request timed out"
		} else if errors.Is(readErr, io.ErrNoProgress) {
			message = "download response body made no progress"
		} else if errors.Is(readErr, io.ErrUnexpectedEOF) {
			message = "download response body ended unexpectedly"
		}
	}

	err := errs.NewNetworkError(subtype, "%s", message).WithCause(cause)
	if ctx.Err() == nil && !errors.Is(readErr, context.Canceled) {
		err.WithRetryable().WithHint("retry the download; discard any partial destination output")
	}
	return err
}

func validateResponseEncoding(resp *http.Response) error {
	if resp.Uncompressed {
		return protocolError("download response Content-Encoding was transparently decoded; want identity")
	}
	values := resp.Header.Values("Content-Encoding")
	if len(values) == 0 {
		return nil
	}
	encoding := strings.TrimSpace(strings.Join(values, ","))
	if strings.EqualFold(encoding, "identity") {
		return nil
	}
	return protocolError("download response uses unsupported Content-Encoding %q; want identity", encoding)
}
