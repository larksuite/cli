// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package command

import (
	"net/http"
	"net/url"
	"path"
	"strings"
)

// Request is an opaque same-origin OpenAPI request description.
type Request struct {
	method      string
	path        string
	query       map[string]any
	body        any
	description string
}

// GET creates a GET OpenAPI request.
func GET(apiPath string) Request { return newRequest(http.MethodGet, apiPath) }

// POST creates a POST OpenAPI request.
func POST(apiPath string) Request { return newRequest(http.MethodPost, apiPath) }

// PUT creates a PUT OpenAPI request.
func PUT(apiPath string) Request { return newRequest(http.MethodPut, apiPath) }

// PATCH creates a PATCH OpenAPI request.
func PATCH(apiPath string) Request { return newRequest(http.MethodPatch, apiPath) }

// DELETE creates a DELETE OpenAPI request.
func DELETE(apiPath string) Request { return newRequest(http.MethodDelete, apiPath) }

// PathSegment escapes one user-provided value for use as a single OpenAPI
// path segment. Every variable concatenated into a request path must be
// wrapped with it, mirroring the host convention (internal/validate
// EncodePathSegment); an unescaped separator or dot sequence would otherwise
// change the request target.
func PathSegment(s string) string { return url.PathEscape(s) }

func newRequest(method, apiPath string) Request {
	return Request{method: method, path: apiPath, query: make(map[string]any)}
}

// Set adds or replaces one query parameter and returns a copied request.
func (r Request) Set(name string, value any) Request {
	r.query = cloneAnyMap(r.query)
	r.query[name] = cloneJSONValue(value)
	return r
}

// Params replaces all query parameters and returns a copied request.
func (r Request) Params(params map[string]any) Request {
	r.query = cloneAnyMap(params)
	return r
}

// Body sets the JSON request body and returns a copied request.
func (r Request) Body(body any) Request {
	r.body = cloneJSONValue(body)
	return r
}

// Desc adds a dry-run explanation and returns a copied request.
func (r Request) Desc(description string) Request {
	r.description = description
	return r
}

// RequestView is the immutable host and test projection of a Request.
type RequestView struct {
	Method      string
	Path        string
	Query       map[string]any
	Body        any
	Description string
}

// InspectRequest returns a copied projection for host adapters and tests.
func InspectRequest(request Request) RequestView {
	return RequestView{
		Method:      request.method,
		Path:        request.path,
		Query:       cloneAnyMap(request.query),
		Body:        cloneJSONValue(request.body),
		Description: request.description,
	}
}

func validateRequest(request Request) error {
	return ValidateRequestView(InspectRequest(request))
}

// ValidateRequestView checks the same-origin OpenAPI boundary for host adapters and tests.
func ValidateRequestView(request RequestView) error {
	if request.Method != http.MethodGet && request.Method != http.MethodPost && request.Method != http.MethodPut && request.Method != http.MethodPatch && request.Method != http.MethodDelete {
		return ValidationErrorf("unsupported OpenAPI request method %q", request.Method)
	}
	rawPath := strings.TrimSpace(request.Path)
	if rawPath == "" || rawPath != request.Path {
		return ValidationErrorf("OpenAPI request path must be a non-empty trimmed relative path")
	}
	parsed, err := url.Parse(rawPath)
	if err != nil {
		return ValidationErrorf("invalid OpenAPI request path %q: %v", rawPath, err)
	}
	if parsed.IsAbs() || parsed.Host != "" || parsed.Scheme != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ValidationErrorf("OpenAPI request path must be same-origin and contain no query or fragment: %q", rawPath)
	}
	if !strings.HasPrefix(parsed.Path, "/open-apis/") {
		return ValidationErrorf("OpenAPI request path must start with /open-apis/: %q", rawPath)
	}
	if cleaned := path.Clean(parsed.Path); cleaned != parsed.Path || strings.Contains(parsed.Path, "/../") {
		return ValidationErrorf("OpenAPI request path is not canonical: %q", rawPath)
	}
	for name := range request.Query {
		if strings.TrimSpace(name) == "" || name != strings.TrimSpace(name) {
			return ValidationErrorf("OpenAPI query parameter name %q is invalid", name)
		}
	}
	return nil
}

func cloneAnyMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}
	result := make(map[string]any, len(input))
	for key, value := range input {
		result[key] = cloneJSONValue(value)
	}
	return result
}
