// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package distribution

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	exttransport "github.com/larksuite/cli/extension/transport"
	internaltransport "github.com/larksuite/cli/internal/transport"
)

const testChecksum = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

type snapshotProvider struct {
	manifestURL string
	calls       int
}

func (*snapshotProvider) Name() string { return "snapshot-test" }
func (*snapshotProvider) ResolveInterceptor(context.Context) exttransport.Interceptor {
	return nil
}
func (p *snapshotProvider) ResolveManifestURL(context.Context) string {
	p.calls++
	return p.manifestURL
}

func validManifestJSON(version string) string {
	return fmt.Sprintf(`{"schema":1,"version":%q,"artifacts":{"skills":{"url":"https://dist.example/skills.tar.gz","checksum":%q},"test-os":{"url":"https://dist.example/cli.tar.gz","checksum":%q}}}`, version, testChecksum, testChecksum)
}

func TestCaptureSourceKeepsOneProviderResult(t *testing.T) {
	previous := exttransport.GetProvider()
	first := &snapshotProvider{manifestURL: "https://first.example/manifest.json"}
	second := &snapshotProvider{manifestURL: "https://second.example/manifest.json"}
	exttransport.Register(first)
	t.Cleanup(func() { exttransport.Register(previous) })

	ctx := CaptureSource(context.Background())
	exttransport.Register(second)
	got, err := ResolveSource(ctx)
	if err != nil || got.manifestURL != first.manifestURL || first.calls != 1 || second.calls != 0 {
		t.Fatalf("captured source = %#v, err = %v, calls = %d/%d", got, err, first.calls, second.calls)
	}
}

func TestDistributionClientUsesSharedBuiltInTransport(t *testing.T) {
	previousClient := DefaultClient
	DefaultClient = nil
	t.Cleanup(func() { DefaultClient = previousClient })

	if got, want := httpClient().Transport, internaltransport.Shared(); got != want {
		t.Fatalf("distribution transport = %T, want shared transport %T", got, want)
	}
}

func TestDistributionRedirectPolicyRejectsDowngradeAndLimitsHops(t *testing.T) {
	httpsRequest, _ := http.NewRequest(http.MethodGet, "https://dist.example/manifest.json", nil)
	httpRequest, _ := http.NewRequest(http.MethodGet, "http://dist.example/manifest.json", nil)
	if err := distributionRedirectPolicy(httpRequest, []*http.Request{httpsRequest}); err == nil {
		t.Fatal("HTTPS to HTTP redirect was allowed")
	}
	if err := distributionRedirectPolicy(httpsRequest, make([]*http.Request, 10)); err == nil {
		t.Fatal("eleventh redirect was allowed")
	}
}

func TestValidateDistributionURLAcceptsHTTPAndHTTPS(t *testing.T) {
	for _, raw := range []string{"http://dist.example/manifest.json", "https://dist.example/manifest.json"} {
		if err := validateDistributionURL(raw); err != nil {
			t.Fatalf("validateDistributionURL(%q) = %v", raw, err)
		}
	}
	for _, raw := range []string{"file:///tmp/manifest.json", "dist.example/manifest.json"} {
		if err := validateDistributionURL(raw); err == nil {
			t.Fatalf("validateDistributionURL(%q) succeeded", raw)
		}
	}
}

func TestParseManifestAcceptsOpaqueTarget(t *testing.T) {
	manifest, err := parseManifest([]byte(validManifestJSON("release-channel-7")), "test-os")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Version != "release-channel-7" {
		t.Fatalf("version = %q", manifest.Version)
	}
}

func TestFetchManifestAppliesManifestDeadline(t *testing.T) {
	previousClient := DefaultClient
	DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if _, ok := req.Context().Deadline(); !ok {
			t.Fatal("manifest request has no deadline")
		}
		body := strings.Replace(validManifestJSON("target"), `"test-os":`, fmt.Sprintf("%q:", CurrentPlatformKey()), 1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})}
	t.Cleanup(func() { DefaultClient = previousClient })
	if _, err := (Source{manifestURL: "https://dist.example/manifest.json"}).FetchManifest(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestParseManifestAcceptsHTTPArtifacts(t *testing.T) {
	input := strings.ReplaceAll(validManifestJSON("1"), "https://", "http://")
	if _, err := parseManifest([]byte(input), "test-os"); err != nil {
		t.Fatal(err)
	}
}

func TestParseManifestIgnoresArtifactsForOtherPlatforms(t *testing.T) {
	input := strings.Replace(validManifestJSON("1"), `"test-os":`, `"other-os":{"url":"not a URL","checksum":"bad"},"test-os":`, 1)
	if _, err := parseManifest([]byte(input), "test-os"); err != nil {
		t.Fatal(err)
	}
}

func TestParseManifestAllowsExtensionFields(t *testing.T) {
	input := strings.Replace(
		validManifestJSON("1"),
		`"schema":1`,
		`"schema":1,"environment":"customer-a"`,
		1,
	)
	input = strings.Replace(
		input,
		`"url":"https://dist.example/skills.tar.gz"`,
		`"url":"https://dist.example/skills.tar.gz","channel":"stable"`,
		1,
	)
	if _, err := parseManifest([]byte(input), "test-os"); err != nil {
		t.Fatal(err)
	}
}

func TestParseManifestRejectsInvalidContracts(t *testing.T) {
	tests := []struct{ name, input, contains string }{
		{"schema", strings.Replace(validManifestJSON("1"), `"schema":1`, `"schema":2`, 1), "unsupported schema"},
		{"missing version", strings.Replace(validManifestJSON("1"), `"version":"1",`, "", 1), "version must be"},
		{"unsupported scheme", strings.Replace(validManifestJSON("1"), "https://dist.example/skills", "file:///tmp/skills", 1), "HTTP or HTTPS"},
		{"checksum", strings.Replace(validManifestJSON("1"), testChecksum, "sha256:ABC", 1), "checksum"},
		{"missing skills", strings.Replace(validManifestJSON("1"), `"skills"`, `"other"`, 1), "missing required"},
		{"missing platform", strings.Replace(validManifestJSON("1"), `"test-os"`, `"other-os"`, 1), "missing required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseManifest([]byte(tt.input), "test-os")
			if err == nil || !strings.Contains(err.Error(), tt.contains) {
				t.Fatalf("err = %v, want containing %q", err, tt.contains)
			}
		})
	}
}
