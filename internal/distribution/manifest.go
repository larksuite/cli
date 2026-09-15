// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package distribution owns fixed-schema manifest loading and verified
// artifact installation for wrapper distributions.
package distribution

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"runtime"
	"sync"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/download"
	"github.com/larksuite/cli/internal/downloadtransport"
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

// DefaultClient overrides the manifest/artifact client in tests. Production
// uses a standalone net/http client so distribution URLs bypass extensions.
var DefaultClient *http.Client

var defaultClientOnce = sync.OnceValue(func() *http.Client {
	return &http.Client{
		// Distribution URLs bypass extension hooks, but they still use the CLI's
		// built-in proxy, custom CA, and fail-closed transport policy.
		Transport:     internaltransport.Shared(),
		CheckRedirect: distributionRedirectPolicy,
	}
})

func distributionRedirectPolicy(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("stopped after 10 redirects")
	}
	if req == nil || (req.URL.Scheme != "http" && req.URL.Scheme != "https") {
		return fmt.Errorf("distribution URL redirected to an unsupported scheme")
	}
	if len(via) > 0 && via[len(via)-1].URL.Scheme == "https" && req.URL.Scheme == "http" {
		return fmt.Errorf("distribution URL redirected from HTTPS to HTTP")
	}
	return nil
}

func httpClient() *http.Client {
	if DefaultClient != nil {
		return DefaultClient
	}
	return defaultClientOnce()
}

// PlatformKey returns the manifest artifact key for a platform.
func PlatformKey(goos, goarch string) string { return goos + "-" + goarch }

// CurrentPlatformKey returns the artifact key for this binary.
func CurrentPlatformKey() string { return PlatformKey(runtime.GOOS, runtime.GOARCH) }

// FetchManifest synchronously loads and validates the source's manifest.
// Fetch failures are network errors; a fetched body that fails validation is
// an invalid response.
func (s Source) FetchManifest(ctx context.Context) (*Manifest, errs.TypedError) {
	body, err := fetchManifestBody(ctx, s.manifestURL)
	if err != nil {
		return nil, errs.NewNetworkError(errs.SubtypeNetworkTransport, "failed to fetch distribution manifest: %s", err).
			WithCause(err)
	}
	manifest, err := parseManifest(body, CurrentPlatformKey())
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "invalid distribution manifest: %s", err).
			WithCause(err)
	}
	manifest.sourceIdentity = s.Identity()
	return manifest, nil
}

func fetchManifestBody(ctx context.Context, manifestURL string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	resp, err := downloadtransport.URL(httpClient(), manifestURL)(ctx, download.Request{})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, manifestMaxBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) > manifestMaxBody {
		return nil, fmt.Errorf("distribution manifest exceeds %d bytes", manifestMaxBody)
	}
	return body, nil
}

func parseManifest(data []byte, platformKey string) (*Manifest, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		return nil, err
	}
	if manifest.Schema != manifestSchema {
		return nil, fmt.Errorf("unsupported schema %d", manifest.Schema)
	}
	if manifest.Version == "" {
		return nil, fmt.Errorf("version must be a non-empty opaque string")
	}
	if manifest.Artifacts == nil {
		return nil, fmt.Errorf("artifacts are required")
	}
	for _, required := range []string{SkillsKey, platformKey} {
		artifact, ok := manifest.Artifacts[required]
		if !ok {
			return nil, fmt.Errorf("missing required artifact %q", required)
		}
		if err := validateDistributionURL(artifact.URL); err != nil {
			return nil, fmt.Errorf("artifact %q has invalid URL: %w", required, err)
		}
		if !checksumPattern.MatchString(artifact.Checksum) {
			return nil, fmt.Errorf("artifact %q has invalid checksum", required)
		}
	}
	return &manifest, nil
}
