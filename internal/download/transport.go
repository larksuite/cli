// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package download

import (
	"context"
	"net/http"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
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

// Get creates fresh SDK request state for every fetch.
func (o OAPI) Get(path string, options ...OAPIRequestOption) Transport {
	spec := oapiRequestSpec{
		pathParams:  larkcore.PathParams{},
		queryParams: larkcore.QueryParams{},
	}
	for _, option := range options {
		option(&spec)
	}
	return func(ctx context.Context, request Request) (*http.Response, error) {
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
