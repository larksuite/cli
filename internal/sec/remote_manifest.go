// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sec

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/core"
)

// secCliManifestPath is the OAPI endpoint that returns the per-platform
// download URLs for lark-sec-cli, gated by tenant_access_token.
const secCliManifestPath = "/open-apis/security_plugin/v1/sec_cli/manifest"

// xTTEnvEnv, when set, injects an x-tt-env header on the manifest request.
// Used for BOE / sub-environment routing (e.g. value "boe_tns_api"). Unset
// in prod — the gateway treats absence as "no override". This is the only
// debug-routing knob in this file; brand/domain switching itself is handled
// at the network layer via the lark-env.sh Whistle pattern in the
// lark-cli maintainer doc.
const xTTEnvEnv = "LARKSUITE_CLI_X_TT_ENV"

// RemoteManifest is the payload returned by GET /open-apis/security_plugin/v1/sec_cli/manifest
// for a single (region, platform, arch) combination. The server returns only
// the download URLs; version metadata is parsed from the URL itself (see
// versionFromURL).
type RemoteManifest struct {
	URLs []string `json:"urls"`
}

// FetchRemoteManifest calls the OAPI manifest endpoint with TAT (bot) auth
// and returns the typed payload for the given region/platform/arch. When the
// LARKSUITE_CLI_X_TT_ENV env var is set, its value is sent as an x-tt-env
// request header for sub-environment routing.
//
// Errors are returned as-is — there is no fallback to the embedded
// bootstrap manifest. Callers that need offline behavior must handle that
// explicitly.
func FetchRemoteManifest(
	ctx context.Context,
	ac *client.APIClient,
	region, platform, arch string,
	verbose io.Writer,
) (*RemoteManifest, error) {
	req := &larkcore.ApiReq{
		HttpMethod: "GET",
		ApiPath:    secCliManifestPath,
		QueryParams: larkcore.QueryParams{
			"region":   []string{region},
			"platform": []string{platform},
			"arch":     []string{arch},
		},
	}
	tracef(verbose, "GET %s?region=%s&platform=%s&arch=%s as=bot", secCliManifestPath, region, platform, arch)

	var extraOpts []larkcore.RequestOptionFunc
	if v := os.Getenv(xTTEnvEnv); v != "" {
		h := http.Header{}
		h.Set("x-tt-env", v)
		extraOpts = append(extraOpts, larkcore.WithHeaders(h))
		tracef(verbose, "injecting header x-tt-env=%s (from %s)", v, xTTEnvEnv)
	}

	resp, err := ac.DoSDKRequest(ctx, req, core.AsBot, extraOpts...)
	if err != nil {
		return nil, fmt.Errorf("sec_cli manifest request: %w", err)
	}
	tracef(verbose, "response status=%d body-len=%d body=%q", resp.StatusCode, len(resp.RawBody), string(resp.RawBody))

	var env struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data *RemoteManifest `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &env); err != nil {
		// Print body unconditionally on decode failure — a malformed response is
		// the most common case where the caller needs to see exactly what arrived.
		fmt.Fprintf(os.Stderr, "[sec_cli manifest] decode failed; status=%d len=%d body=%q\n", resp.StatusCode, len(resp.RawBody), string(resp.RawBody))
		return nil, fmt.Errorf("sec_cli manifest decode: %w", err)
	}
	if env.Code != 0 {
		return nil, fmt.Errorf("sec_cli manifest error %d: %s", env.Code, env.Msg)
	}
	if env.Data == nil || len(env.Data.URLs) == 0 {
		return nil, fmt.Errorf("sec_cli manifest: no urls for region=%s platform=%s arch=%s", region, platform, arch)
	}
	return env.Data, nil
}

// versionFromURL extracts the release version from a download URL of the form
//   .../releases/<version>/<pipeline-id>/<platform-arch>/<archive>.zip
// The server-side manifest does not return version as a discrete field;
// state.json's Version needs *something* to disambiguate concurrent installs
// in versions/<version>/, so we parse it out here.
var releaseVersionRE = regexp.MustCompile(`/releases/([^/]+)/`)

func versionFromURL(u string) (string, error) {
	m := releaseVersionRE.FindStringSubmatch(u)
	if len(m) < 2 || m[1] == "" {
		return "", fmt.Errorf("could not parse release version from URL %q", u)
	}
	return m[1], nil
}
