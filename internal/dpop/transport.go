// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package dpop

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/larksuite/cli/errs"
)

// Transport signs access-token requests at the final request boundary. Place it
// inside retry transports so each attempt receives a fresh proof. Callers own
// destination policy and must clear DPoP headers and context whenever they
// remove Authorization; this transport does not restrict request origins.
type Transport struct {
	Base http.RoundTripper
}

func (t *Transport) BaseRoundTripper() http.RoundTripper {
	if t == nil || t.Base == nil {
		return http.DefaultTransport
	}
	return t.Base
}

func (t *Transport) WithBaseRoundTripper(base http.RoundTripper) http.RoundTripper {
	return &Transport{Base: base}
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	request := req.Clone(req.Context())
	request.Header = req.Header.Clone()
	if key, ok := tokenEndpointKeyFromContext(request.Context()); ok {
		proof, err := key.SignProofContext(request.Context(), request.Method, request.URL.String())
		if err != nil {
			closeRequestBody(request)
			return nil, errs.NewAuthenticationError(errs.SubtypeDPoPProofFailed,
				"failed to generate Token Endpoint DPoP proof: %v", err).
				WithCause(err).
				WithHint("%s", KeyStoreUnavailableHint)
		}
		request.Header.Set(ProofHeader, proof)
		return roundTripWithoutReplay(base, request)
	}
	binding, bound := BindingFromContext(request.Context())
	if !bound {
		return base.RoundTrip(request)
	}

	authorization := strings.TrimSpace(request.Header.Get("Authorization"))
	parts := strings.SplitN(authorization, " ", 2)
	if len(parts) != 2 || (parts[0] != "Bearer" && parts[0] != Scheme) || parts[1] != binding.Token {
		closeRequestBody(request)
		return nil, errs.NewAuthenticationError(errs.SubtypeDPoPBindingMismatch,
			"DPoP token binding does not match the request authorization").
			WithHint("re-authorize the profile to replace the inconsistent token binding")
	}
	proof, err := binding.SignProofContext(request.Context(), request.Method, request.URL.String())
	if err != nil {
		closeRequestBody(request)
		return nil, errs.NewAuthenticationError(errs.SubtypeDPoPProofFailed,
			"failed to generate DPoP proof: %v", err).
			WithCause(err).
			WithHint("%s", KeyAccessUnavailableHint)
	}
	request.Header.Set("Authorization", fmt.Sprintf("%s %s", Scheme, binding.Token))
	request.Header.Set(ProofHeader, proof)
	return roundTripWithoutReplay(base, request)
}

func closeRequestBody(request *http.Request) {
	if request.Body != nil {
		_ = request.Body.Close()
	}
}

func roundTripWithoutReplay(base http.RoundTripper, request *http.Request) (*http.Response, error) {
	// A signed request cannot be replayed by net/http: its proof is single-use.
	// Even bodyless requests need a non-replayable body to suppress transparent
	// HTTP/1 and HTTP/2 retries. The caller's original body and GetBody remain
	// intact so retries above this transport can obtain a fresh proof.
	request.GetBody = nil
	if request.Body == nil || request.Body == http.NoBody {
		request.Body = io.NopCloser(http.NoBody)
	}
	return base.RoundTrip(request)
}
