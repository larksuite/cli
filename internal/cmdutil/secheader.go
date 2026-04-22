// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"context"
	"net/http"
	"reflect"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/extension/fileio"
	exttransport "github.com/larksuite/cli/extension/transport"
	"github.com/larksuite/cli/internal/build"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

const (
	HeaderSource      = "X-Cli-Source"
	HeaderVersion     = "X-Cli-Version"
	HeaderBuild       = "X-Cli-Build"
	HeaderShortcut    = "X-Cli-Shortcut"
	HeaderExecutionId = "X-Cli-Execution-Id"

	SourceValue = "lark-cli"

	HeaderUserAgent = "User-Agent"

	// BuildKindOfficial / BuildKindExtended / BuildKindUnknown are the values
	// reported in the X-Cli-Build header; see DetectBuildKind for semantics.
	BuildKindOfficial = "official"
	BuildKindExtended = "extended"
	BuildKindUnknown  = "unknown"

	officialModulePath = "github.com/larksuite/cli"
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
	h.Set(HeaderBuild, DetectBuildKind())
	h.Set(HeaderUserAgent, UserAgentValue())
	return h
}

var (
	buildKindOnce sync.Once
	buildKindVal  string
)

// DetectBuildKind reports whether this binary is the official CLI, an
// extended/repackaged build, or unknown. The result is cached via sync.Once
// so it is computed only on the first call.
//
// IMPORTANT: must NOT be called from any package init(). Go's init ordering
// follows the import graph; ISV providers registered via blank import may not
// have run yet, which would misclassify an extended build as official. Call
// only when handling an actual request (e.g. from BaseSecurityHeaders).
func DetectBuildKind() string {
	buildKindOnce.Do(func() {
		buildKindVal = computeBuildKind()
	})
	return buildKindVal
}

// computeBuildKind performs the actual detection without any caching.
// Exposed for tests. Gathers runtime/global inputs and delegates the pure
// branching logic to classifyBuild so that logic can be unit-tested without
// mutating process-wide provider registries.
func computeBuildKind() string {
	info, ok := debug.ReadBuildInfo()
	mainPath := ""
	if ok {
		mainPath = info.Main.Path
	}

	credProviders := credential.Providers()
	creds := make([]any, len(credProviders))
	for i, p := range credProviders {
		creds[i] = p
	}

	var tp any
	if p := exttransport.GetProvider(); p != nil {
		tp = p
	}
	var fp any
	if p := fileio.GetProvider(); p != nil {
		fp = p
	}
	return classifyBuild(mainPath, ok, creds, tp, fp)
}

// classifyBuild is the pure classification logic used by computeBuildKind.
// Callers supply concrete values so every branch is reachable from tests
// without touching debug.ReadBuildInfo or the extension registries.
//
// Priority order mirrors the design doc:
//  1. no build info → unknown
//  2. main module path not the official one → extended (ISV wrapper)
//  3. any non-builtin provider (credential / transport / fileio) → extended
//  4. otherwise → official
func classifyBuild(mainPath string, haveBuildInfo bool, credProviders []any, transportProvider, fileioProvider any) string {
	if !haveBuildInfo {
		return BuildKindUnknown
	}
	if mainPath != "" && mainPath != officialModulePath {
		return BuildKindExtended
	}
	for _, p := range credProviders {
		if !isBuiltinProvider(p) {
			return BuildKindExtended
		}
	}
	if transportProvider != nil && !isBuiltinProvider(transportProvider) {
		return BuildKindExtended
	}
	if fileioProvider != nil && !isBuiltinProvider(fileioProvider) {
		return BuildKindExtended
	}
	return BuildKindOfficial
}

// isBuiltinProvider reports whether p is declared under the official module
// path. Third-party providers live under their own module and fail this check.
// Using reflect.PkgPath makes this robust against Name() spoofing since
// package paths are fixed at compile time.
func isBuiltinProvider(p any) bool {
	if p == nil {
		return false
	}
	t := reflect.TypeOf(p)
	if t == nil {
		return false
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	pkg := t.PkgPath()
	return pkg == officialModulePath || strings.HasPrefix(pkg, officialModulePath+"/")
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
