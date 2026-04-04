// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"context"
	"net/http"

	"github.com/larksuite/cli/internal/build"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

const (
	HeaderSource      = "X-Cli-Source"
	HeaderVersion     = "X-Cli-Version"
	HeaderShortcut    = "X-Cli-Shortcut"
	HeaderExecutionId = "X-Cli-Execution-Id"

	SourceValue = "lark-cli"

	HeaderUserAgent = "User-Agent"
)

// UserAgentValue returns the User-Agent value: "lark-cli/{version}".
func UserAgentValue() string {
	return SourceValue + "/" + build.Version
}

// BaseSecurityHeaders returns headers that every request must carry.
func BaseSecurityHeaders() http.Header {
	h := make(http.Header)
	h.Set(HeaderSource, SourceValue)
	h.Set(HeaderVersion, build.Version)
	h.Set(HeaderUserAgent, UserAgentValue())
	return h
}

// ── Context utilities ──

type ctxKey string

const (
	ctxShortcutName ctxKey = "lark:shortcut-name"
	ctxExecutionId  ctxKey = "lark:execution-id"
)

// ContextWithShortcut injects shortcut name and execution ID into the context.
func ContextWithShortcut(ctx context.Context, name, executionId string) context.Context {
	ctx = context.WithValue(ctx, ctxShortcutName, name)
	ctx = context.WithValue(ctx, ctxExecutionId, executionId)
	return ctx
}

// ShortcutNameFromContext extracts the shortcut name from the context.
func ShortcutNameFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(ctxShortcutName).(string)
	return v, ok && v != ""
}

// ExecutionIdFromContext extracts the execution ID from the context.
func ExecutionIdFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(ctxExecutionId).(string)
	return v, ok && v != ""
}

// ShortcutHeaderOpts extracts Shortcut info from the context and returns a
// RequestOptionFunc that injects the corresponding headers into SDK requests.
// Returns nil if the context has no Shortcut info.
func ShortcutHeaderOpts(ctx context.Context) larkcore.RequestOptionFunc {
	h := ShortcutHeaders(ctx)
	if h == nil {
		return nil
	}
	return larkcore.WithHeaders(h)
}

// ShortcutHeaders extracts Shortcut info from the context and returns
// the corresponding HTTP headers. Returns nil if the context has no Shortcut info.
func ShortcutHeaders(ctx context.Context) http.Header {
	name, ok := ShortcutNameFromContext(ctx)
	if !ok {
		return nil
	}
	h := make(http.Header)
	h.Set(HeaderShortcut, name)
	if eid, ok := ExecutionIdFromContext(ctx); ok {
		h.Set(HeaderExecutionId, eid)
	}
	return h
}
