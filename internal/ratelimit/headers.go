// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package ratelimit parses server-provided retry pacing without coupling
// retry loops to one gateway's header names.
package ratelimit

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// HeaderLimit is the Lark OpenAPI gateway's request-window quota.
	HeaderLimit = "X-Ogw-Ratelimit-Limit"
	// HeaderReset is the Lark OpenAPI gateway's remaining recovery time in
	// seconds. It takes precedence over the standard Retry-After fallback.
	HeaderReset = "X-Ogw-Ratelimit-Reset"
)

// Info is the transport-neutral rate-limit signal extracted from one response.
type Info struct {
	RetryAfter time.Duration
	Limit      int64
}

// ParseHeaders reads Lark gateway pacing first, then the standard Retry-After
// delta-seconds or HTTP-date form. Invalid and non-positive values are ignored.
func ParseHeaders(header http.Header, now time.Time) Info {
	info := Info{Limit: positiveInt64(header.Get(HeaderLimit))}
	if delay, ok := deltaSeconds(header.Get(HeaderReset)); ok {
		info.RetryAfter = delay
		return info
	}
	standard := ParseStandardHeaders(header, now)
	info.RetryAfter = standard.RetryAfter
	return info
}

// ParseStandardHeaders reads the standard Retry-After delta-seconds or
// HTTP-date form.
func ParseStandardHeaders(header http.Header, now time.Time) Info {
	var info Info
	if delay, ok := deltaSeconds(header.Get("Retry-After")); ok {
		info.RetryAfter = delay
		return info
	}
	if retryAt, err := http.ParseTime(strings.TrimSpace(header.Get("Retry-After"))); err == nil {
		if delay := retryAt.Sub(now); delay > 0 {
			info.RetryAfter = delay
		}
	}
	return info
}

// RetryAfterSeconds rounds a positive delay up so serialized retry metadata
// never asks a caller to retry before the server-provided recovery instant.
func (i Info) RetryAfterSeconds() int {
	if i.RetryAfter <= 0 {
		return 0
	}
	seconds := i.RetryAfter / time.Second
	if i.RetryAfter%time.Second != 0 {
		seconds++
	}
	maxInt := int64(^uint(0) >> 1)
	if int64(seconds) > maxInt {
		return int(maxInt)
	}
	return int(seconds)
}

func deltaSeconds(value string) (time.Duration, bool) {
	seconds, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || seconds <= 0 {
		return 0, false
	}
	if seconds > math.MaxInt64/int64(time.Second) {
		return time.Duration(math.MaxInt64), true
	}
	return time.Duration(seconds) * time.Second, true
}

func positiveInt64(value string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}
