// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package client

import (
	"net/http"
	"time"
)

// Option configures API request behavior for DoStream (and future DoSDKRequest).
type Option func(*requestConfig)

type requestConfig struct {
	timeout time.Duration
	headers http.Header
}

// WithTimeout sets a request-level timeout that overrides the client default.
func WithTimeout(d time.Duration) Option {
	return func(c *requestConfig) {
		c.timeout = d
	}
}

// WithHeaders adds extra HTTP headers to the request.
func WithHeaders(h http.Header) Option {
	return func(c *requestConfig) {
		if c.headers == nil {
			c.headers = make(http.Header)
		}
		for k, vs := range h {
			for _, v := range vs {
				c.headers.Add(k, v)
			}
		}
	}
}

func buildConfig(opts []Option) requestConfig {
	var cfg requestConfig
	for _, o := range opts {
		o(&cfg)
	}
	return cfg
}
