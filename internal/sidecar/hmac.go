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
	"time"
)

// BodySHA256 returns the hex-encoded SHA-256 digest of body.
// An empty or nil body produces the SHA-256 of the empty string.
func BodySHA256(body []byte) string {
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:])
}

// Sign computes the HMAC-SHA256 signature over the canonical request string.
//
// Signing material (newline-separated):
//
//	method          e.g. "GET", "POST"
//	host            e.g. "open.feishu.cn"
//	pathAndQuery    e.g. "/open-apis/calendar/v4/events?page_size=50"
//	bodySHA256      hex-encoded SHA-256 of the request body
//	timestamp       Unix epoch seconds string
func Sign(key []byte, method, host, pathAndQuery, bodySHA256, timestamp string) string {
	payload := method + "\n" + host + "\n" + pathAndQuery + "\n" + bodySHA256 + "\n" + timestamp
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify checks that signature matches the HMAC-SHA256 of the canonical
// request string, and that the timestamp is within MaxTimestampDrift seconds
// of now. Returns nil on success.
func Verify(key []byte, method, host, pathAndQuery, bodySHA256, timestamp, signature string) error {
	// Validate timestamp
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp %q: %w", timestamp, err)
	}
	drift := math.Abs(float64(time.Now().Unix() - ts))
	if drift > MaxTimestampDrift {
		return fmt.Errorf("timestamp drift %.0fs exceeds limit %ds", drift, MaxTimestampDrift)
	}

	// Verify HMAC
	expected := Sign(key, method, host, pathAndQuery, bodySHA256, timestamp)
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return fmt.Errorf("HMAC signature mismatch")
	}
	return nil
}

// Timestamp returns the current Unix epoch seconds as a string.
func Timestamp() string {
	return strconv.FormatInt(time.Now().Unix(), 10)
}
