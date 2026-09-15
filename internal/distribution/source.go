// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package distribution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"

	"github.com/larksuite/cli/errs"
	exttransport "github.com/larksuite/cli/extension/transport"
)

// Source describes the active update source. The zero value is the npm
// registry flow; a non-zero Source selects manifest-based distribution.
type Source struct{ manifestURL string }

// SourceSnapshot freezes source resolution for one command invocation.
type SourceSnapshot struct {
	source Source
	err    errs.TypedError
}

type sourceSnapshotKey struct{}

// CaptureSource resolves the provider once and stores the result in ctx.
func CaptureSource(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Value(sourceSnapshotKey{}).(SourceSnapshot); ok {
		return ctx
	}
	source, err := resolveSource(ctx)
	return context.WithValue(ctx, sourceSnapshotKey{}, SourceSnapshot{source: source, err: err})
}

// ResolveSource reads the optional distribution manifest URL from the
// registered transport provider. Providers that do not implement
// exttransport.DistributionProvider, or return an empty URL, yield the zero
// (npm) Source.
func ResolveSource(ctx context.Context) (Source, errs.TypedError) {
	if ctx != nil {
		if snapshot, ok := ctx.Value(sourceSnapshotKey{}).(SourceSnapshot); ok {
			return snapshot.source, snapshot.err
		}
	}
	return resolveSource(ctx)
}

func resolveSource(ctx context.Context) (Source, errs.TypedError) {
	if ctx == nil {
		ctx = context.Background()
	}
	configured, ok := exttransport.GetProvider().(exttransport.DistributionProvider)
	if !ok {
		return Source{}, nil
	}
	raw := strings.TrimSpace(configured.ResolveManifestURL(ctx))
	if raw == "" {
		return Source{}, nil
	}
	if err := validateDistributionURL(raw); err != nil {
		return Source{}, errs.NewConfigError(
			errs.SubtypeInvalidConfig,
			"invalid distribution manifest URL: %v", err,
		).WithCause(err)
	}
	return Source{manifestURL: raw}, nil
}

// ManifestMode reports whether this source is a manifest distribution.
func (s Source) ManifestMode() bool { return s.manifestURL != "" }

// Identity is a stable fingerprint of the source used to attribute persisted
// state without storing the URL. The npm source has the empty identity, which
// also matches state files written before source tracking existed.
func (s Source) Identity() string {
	if s.manifestURL == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(s.manifestURL))
	return "manifest:" + hex.EncodeToString(sum[:])
}

// validateDistributionURL accepts absolute HTTP/HTTPS URLs. Plain HTTP is
// intended for trusted distribution networks only: a manifest served over HTTP
// can be replaced together with its checksums, so the provider owns transport
// integrity when it does not use HTTPS.
func validateDistributionURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("must be a valid URL")
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("must be an absolute HTTP or HTTPS URL")
	}
	if parsed.User != nil {
		return fmt.Errorf("must not contain user information")
	}
	if parsed.Fragment != "" {
		return fmt.Errorf("must not contain a fragment")
	}
	return nil
}
