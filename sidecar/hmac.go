// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sidecar

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// BodySHA256 returns the hex-encoded SHA-256 digest of body.
// An empty or nil body produces the SHA-256 of the empty string.
func BodySHA256(body []byte) string {
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:])
}

// CanonicalRequest is the set of fields covered by the HMAC signature.
// Clients and servers must populate every field identically for verification
// to succeed; any field that is forwarded but *not* covered by this struct can
// be tampered with inside the MaxTimestampDrift replay window without
// invalidating the signature.
//
// Version must be set to a known protocol constant (ProtocolV1). It is the
// first field in the canonical string so that a future v2 with different
// structure cannot be confused for v1 output under the same key.
type CanonicalRequest struct {
	Version      string // e.g. ProtocolV1
	Method       string // e.g. "GET", "POST"
	Host         string // e.g. "open.feishu.cn"
	PathAndQuery string // e.g. "/open-apis/calendar/v4/events?page_size=50"
	BodySHA256   string // hex-encoded SHA-256 of the request body
	Timestamp    string // Unix epoch seconds string
	Identity     string // IdentityUser or IdentityBot
	AuthHeader   string // header the server should inject the real token into
}

// canonicalString joins the fields with newlines. Field order is part of the
// protocol contract — do not reorder without bumping Version.
func (c CanonicalRequest) canonicalString() string {
	return strings.Join([]string{
		c.Version,
		c.Method,
		c.Host,
		c.PathAndQuery,
		c.BodySHA256,
		c.Timestamp,
		c.Identity,
		c.AuthHeader,
	}, "\n")
}

// Sign computes the HMAC-SHA256 signature over the canonical request string.
func Sign(key []byte, req CanonicalRequest) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(req.canonicalString()))
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify checks that signature matches the HMAC-SHA256 of the canonical
// request and that the timestamp is within MaxTimestampDrift seconds of now.
// Returns nil on success.
func Verify(key []byte, req CanonicalRequest, signature string) error {
	ts, err := strconv.ParseInt(req.Timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp %q: %w", req.Timestamp, err)
	}
	drift := math.Abs(float64(time.Now().Unix() - ts))
	if drift > MaxTimestampDrift {
		return fmt.Errorf("timestamp drift %.0fs exceeds limit %ds", drift, MaxTimestampDrift)
	}
	expected := Sign(key, req)
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return fmt.Errorf("HMAC signature mismatch")
	}
	return nil
}

// Timestamp returns the current Unix epoch seconds as a string.
func Timestamp() string {
	return strconv.FormatInt(time.Now().Unix(), 10)
}
