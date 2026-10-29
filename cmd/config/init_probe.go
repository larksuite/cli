// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/keysigner"
)

// probeTimeout is the total wall-clock budget for the credential probe step
// (covering both TAT acquisition and the subsequent probe request).
const probeTimeout = 3 * time.Second

// runProbe runs a best-effort credential validation after config init has
// persisted the App ID and App Secret. Typed token-endpoint failures propagate;
// ambiguous transport, parse, timeout, and transient server failures stay silent.
//
// The function performs up to two HTTP calls in series, bounded by
// probeTimeout:
//
//  1. A TAT request using the just-saved credentials. The credential helper
//     returns typed OAuth, policy, and rate-limit failures using the same
//     classification as normal token resolution. errs.IsTyped is the discriminator
//     between errors to propagate and ambiguous failures to ignore.
//
//  2. If TAT succeeded, a POST to the probe endpoint is fired. Typed policy
//     errors are propagated; every other outcome remains best-effort and is
//     ignored.
func runProbe(parent context.Context, factory *cmdutil.Factory, appID, appSecret string, brand core.LarkBrand) error {
	if factory == nil {
		return nil
	}
	httpClient, err := factory.HttpClient()
	if err != nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(parent, probeTimeout)
	defer cancel()

	result, err := credential.FetchTAT(ctx, httpClient, brand, auth.ClientAuth{AppID: appID, AppSecret: appSecret}, core.DPoPModeDisabled)
	if err != nil {
		// Preserve token resolution's typed failures; ambiguous failures stay silent.
		if errs.IsTyped(err) {
			return err
		}
		return nil
	}
	if result.StatusMessage != "" && factory.IOStreams != nil && factory.IOStreams.ErrOut != nil {
		fmt.Fprintf(factory.IOStreams.ErrOut, "[lark-cli] tat-client: %s\n", result.StatusMessage)
	}

	// TAT succeeded — fire the probe call. Only typed policy errors propagate.
	url := core.ResolveEndpoints(brand).Open + "/open-apis/application/v6/larksuite_cli_app/probe"
	body := []byte(fmt.Sprintf(`{"from":"lark-cli/%s"}`, build.Version))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+result.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		if problem, ok := errs.ProblemOf(err); ok && problem.Category == errs.CategoryPolicy {
			return err
		}
		return nil
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// runProbePrivateKeyJWTLocalKeyPair does a best-effort key-binding validation: it signs a
// client_assertion with the configured key (file or generated) and mints a token.
// A typed error (a deterministic server rejection — e.g. the key
// is not bound to this app) is propagated so `config init` exits non-zero with
// the canonical envelope; untyped errors (transport / HTTP / parse / timeout)
// are swallowed (return nil). The mint itself is the probe — no second call.
func runProbePrivateKeyJWTLocalKeyPair(parent context.Context, factory *cmdutil.Factory, brand core.LarkBrand, clientID string, signer keysigner.Signer, keyLabel string) error {
	if factory == nil {
		return nil
	}
	if signer == nil {
		return errs.NewConfigError(errs.SubtypeInvalidConfig,
			"private-key JWT probe requires a key signer")
	}
	httpClient, err := factory.HttpClient()
	if err != nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(parent, probeTimeout)
	defer cancel()

	if _, err := credential.FetchTATWithAssertion(ctx, httpClient, brand, clientID, signer, keyLabel); err != nil {
		// Preserve token resolution's typed failures; ambiguous failures stay silent.
		if errs.IsTyped(err) {
			return err
		}
		return nil
	}
	return nil
}
