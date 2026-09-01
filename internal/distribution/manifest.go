// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package distribution owns fixed-schema manifest loading and verified
// artifact installation for wrapper distributions.
package distribution

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"runtime"
	"time"

	"github.com/larksuite/cli/errs"
	internaltransport "github.com/larksuite/cli/internal/transport"
)

const (
	manifestSchema  = 1
	manifestMaxBody = 256 << 10
	fetchTimeout    = 15 * time.Second
	SkillsKey       = "skills"
)

var checksumPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// Artifact identifies one downloadable resource.
type Artifact struct {
	URL      string `json:"url"`
	Checksum string `json:"checksum"`
}

// Manifest is schema 1 of the distribution protocol. Unknown JSON fields are
// ignored so producers can attach metadata without breaking older CLIs; the
// required fields and artifacts are still validated before use.
type Manifest struct {
	Schema    int                 `json:"schema"`
	Version   string              `json:"version"`
	Artifacts map[string]Artifact `json:"artifacts"`

	sourceIdentity string
}

// ManifestSourceIdentity identifies one manifest without persisting its URL.
func ManifestSourceIdentity(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return "manifest:" + hex.EncodeToString(sum[:])
}

// DefaultClient overrides the manifest/artifact client in tests. Production
// uses a standalone net/http client so distribution URLs bypass extensions.
var DefaultClient *http.Client

func httpClient() *http.Client {
	if DefaultClient != nil {
		return DefaultClient
	}
	return &http.Client{
		// Distribution URLs bypass extension hooks, but they still use the CLI's
		// built-in proxy, custom CA, and fail-closed transport policy.
		Transport: internaltransport.Shared(),
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("distribution URL redirected to an unsupported scheme")
			}
			return nil
		},
	}
}

// PlatformKey returns the manifest artifact key for a platform.
func PlatformKey(goos, goarch string) string { return goos + "-" + goarch }

// CurrentPlatformKey returns the artifact key for this binary.
func CurrentPlatformKey() string { return PlatformKey(runtime.GOOS, runtime.GOARCH) }

// FetchManifest synchronously loads and validates the configured manifest.
// Failures are classified at this owner boundary before they reach commands,
// background checks, or diagnostics.
func FetchManifest(ctx context.Context, manifestURL string) (*Manifest, errs.TypedError) {
	manifest, err := fetchManifest(ctx, manifestURL)
	if err != nil {
		return nil, classifyError("failed to load distribution manifest", err)
	}
	return manifest, nil
}

func fetchManifest(ctx context.Context, manifestURL string) (*Manifest, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create manifest request: %w", err)
	}
	resp, err := httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch distribution manifest: %w", redactRequestError(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, newHTTPStatusError("fetch distribution manifest", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, manifestMaxBody+1))
	if err != nil {
		return nil, fmt.Errorf("read distribution manifest: %w", err)
	}
	if len(body) > manifestMaxBody {
		return nil, fmt.Errorf("distribution manifest exceeds %d bytes", manifestMaxBody)
	}
	manifest, err := parseManifest(body, CurrentPlatformKey())
	if err != nil {
		return nil, err
	}
	manifest.sourceIdentity = ManifestSourceIdentity(manifestURL)
	return manifest, nil
}

type httpStatusError struct {
	operation  string
	statusCode int
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("%s: HTTP %d", e.operation, e.statusCode)
}

func newHTTPStatusError(operation string, statusCode int) error {
	return &httpStatusError{operation: operation, statusCode: statusCode}
}

func httpStatusCode(err error) (int, bool) {
	var statusErr *httpStatusError
	if !errors.As(err, &statusErr) {
		return 0, false
	}
	return statusErr.statusCode, true
}

func redactRequestError(err error) error {
	var requestErr *url.Error
	if errors.As(err, &requestErr) {
		return fmt.Errorf("%s request failed: %w", requestErr.Op, requestErr.Err)
	}
	return err
}

func parseManifest(data []byte, platformKey string) (*Manifest, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("invalid distribution manifest: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, fmt.Errorf("invalid distribution manifest: %w", err)
	}
	if manifest.Schema != manifestSchema {
		return nil, fmt.Errorf("unsupported distribution manifest schema %d", manifest.Schema)
	}
	if manifest.Version == "" {
		return nil, fmt.Errorf("distribution manifest version must be a non-empty opaque string")
	}
	if manifest.Artifacts == nil {
		return nil, fmt.Errorf("distribution manifest artifacts are required")
	}
	for _, required := range []string{SkillsKey, platformKey} {
		artifact, ok := manifest.Artifacts[required]
		if !ok {
			return nil, fmt.Errorf("distribution manifest is missing required artifact %q", required)
		}
		if err := validateArtifact(required, artifact); err != nil {
			return nil, err
		}
	}
	return &manifest, nil
}

func validateArtifact(key string, artifact Artifact) error {
	if err := validateDistributionURL(artifact.URL); err != nil {
		return fmt.Errorf("distribution artifact %q has invalid URL: %w", key, err)
	}
	if !checksumPattern.MatchString(artifact.Checksum) {
		return fmt.Errorf("distribution artifact %q has invalid checksum", key)
	}
	return nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}
