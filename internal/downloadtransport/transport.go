// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package downloadtransport adapts host-owned OAPI and URL clients to the
// public extension/download transport contract.
package downloadtransport

import (
	"context"
	"io"
	"net/http"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	extdownload "github.com/larksuite/cli/extension/download"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/ratelimit"
)

// APIStreamFunc is the OAPI streaming capability used by Transport.
type APIStreamFunc func(context.Context, *larkcore.ApiReq, ...client.Option) (*http.Response, error)

// OAPI builds authenticated download transports.
type OAPI struct {
	doStream APIStreamFunc
}

// OAPIRequestOption configures one declarative OAPI endpoint.
type OAPIRequestOption func(*oapiRequestSpec)

type oapiRequestSpec struct {
	pathParams  larkcore.PathParams
	queryParams larkcore.QueryParams
}

// NewOAPI creates an authenticated transport factory.
func NewOAPI(doStream APIStreamFunc) OAPI {
	return OAPI{doStream: doStream}
}

// PathParam binds one SDK path placeholder.
func PathParam(name, value string) OAPIRequestOption {
	return func(spec *oapiRequestSpec) {
		spec.pathParams[name] = value
	}
}

// Query binds one OAPI query parameter.
func Query(name, value string) OAPIRequestOption {
	return func(spec *oapiRequestSpec) {
		spec.queryParams.Set(name, value)
	}
}

// QueryIf binds a query parameter when value is non-empty.
func QueryIf(name, value string) OAPIRequestOption {
	return func(spec *oapiRequestSpec) {
		if value != "" {
			spec.queryParams.Set(name, value)
		}
	}
}

// Get creates fresh SDK request state for every fetch.
func (o OAPI) Get(path string, options ...OAPIRequestOption) extdownload.Transport {
	spec := oapiRequestSpec{
		pathParams:  larkcore.PathParams{},
		queryParams: larkcore.QueryParams{},
	}
	for _, option := range options {
		option(&spec)
	}
	return func(ctx context.Context, request extdownload.Request) (*http.Response, error) {
		if o.doStream == nil {
			return nil, errs.NewInternalError(errs.SubtypeUnknown, "OAPI download transport is not configured")
		}
		req := &larkcore.ApiReq{
			HttpMethod:  http.MethodGet,
			ApiPath:     path,
			PathParams:  clonePathParams(spec.pathParams),
			QueryParams: cloneQueryParams(spec.queryParams),
		}
		return o.doStream(ctx, req, client.WithHeaders(request.Headers()), client.WithReplaySafe())
	}
}

func clonePathParams(params larkcore.PathParams) larkcore.PathParams {
	cloned := make(larkcore.PathParams, len(params))
	for name, value := range params {
		cloned[name] = value
	}
	return cloned
}

func cloneQueryParams(params larkcore.QueryParams) larkcore.QueryParams {
	cloned := make(larkcore.QueryParams, len(params))
	for name, values := range params {
		cloned[name] = append([]string(nil), values...)
	}
	return cloned
}

// URL adapts a caller-validated URL and HTTP client to a public download
// Transport. The caller owns source and redirect validation;
// extension/download.Open owns transfer timeouts.
func URL(httpClient *http.Client, rawURL string) extdownload.Transport {
	var downloadClient *http.Client
	if httpClient != nil {
		downloadClient = &http.Client{
			Transport:     httpClient.Transport,
			CheckRedirect: httpClient.CheckRedirect,
			Jar:           httpClient.Jar,
		}
	}
	return func(ctx context.Context, request extdownload.Request) (*http.Response, error) {
		if downloadClient == nil {
			return nil, errs.NewInternalError(errs.SubtypeUnknown, "download URL transport requires an HTTP client")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, errs.NewNetworkError(errs.SubtypeNetworkTransport, "invalid download URL").WithCause(err)
		}
		req.Header = request.Headers()

		resp, err := downloadClient.Do(req)
		if err != nil {
			hasResponse := resp != nil
			if resp != nil && resp.Body != nil {
				resp.Body.Close()
			}
			if _, typed := errs.ProblemOf(err); typed {
				return nil, err
			}
			if hasResponse {
				return nil, errs.NewNetworkError(errs.SubtypeNetworkTransport, "download redirect failed").WithCause(err)
			}
			return nil, client.WrapReplaySafeTransportError(ctx, err, "download failed")
		}
		if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			return resp, nil
		}
		return nil, urlResponseError(resp)
	}
}

func urlResponseError(resp *http.Response) error {
	if resp.Body != nil {
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	}

	subtype := errs.SubtypeNetworkTransport
	if resp.StatusCode == http.StatusRequestTimeout {
		subtype = errs.SubtypeNetworkTimeout
	} else if resp.StatusCode >= http.StatusInternalServerError {
		subtype = errs.SubtypeNetworkServer
	}
	networkErr := errs.NewNetworkError(subtype, "download failed: HTTP %d", resp.StatusCode).WithCode(resp.StatusCode)
	if resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
		networkErr.WithRetryable()
		if retry := ratelimit.ParseStandardHeaders(resp.Header, time.Now()).RetryAfterSeconds(); retry > 0 {
			networkErr.WithRetryAfterSeconds(retry)
		}
	}
	return networkErr
}
