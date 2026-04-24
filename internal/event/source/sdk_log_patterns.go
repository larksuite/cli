// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package source: SDK log pattern constants.
//
// These substrings are matched against larksuite/oapi-sdk-go/v3/ws log
// output to derive connection-lifecycle notifications. The SDK does not
// expose these states as a proper callback API (see upstream tracking in
// feishu.go), so we scrape log lines.
//
// This is inherently fragile: if the SDK localizes its logs, renames a
// pattern, or changes log levels, classification silently breaks. The
// sidecar test (sdk_log_patterns_test.go) feeds real observed log
// strings through the classifier so SDK drift is caught at CI time
// rather than in production.
//
// Note on shape: matching in tryNotify is HasPrefix, not Contains. SDK
// log lines are "<verb> to <url>[conn_id=...]", so "connected to " and
// "disconnected to " are mutually exclusive at the start of the string.
// A prior Contains-based switch accidentally matched "connected to ws"
// inside "disconnected to wss://...", misreporting every disconnect as
// a reconnect. Constants are stored with the trailing space preserved
// so downstream HasPrefix matches remain unambiguous.
//
// LAST VERIFIED: larksuite/oapi-sdk-go/v3 v3.5.3 on 2026-04-19.
// When bumping the SDK dependency, re-run the test; if it fails, update
// BOTH the patterns here AND the corresponding sample strings in the test.

package source

// DO NOT trim the trailing space on sdkLogConnected or sdkLogDisconnected —
// it is the HasPrefix disambiguator that prevents "disconnected to ..."
// from matching sdkLogConnected. The sdk_log_patterns_test.go has a
// HasSuffix pin that will fail if the space is ever removed.
const (
	// sdkLogReconnecting prefixes SDK lines like "trying to reconnect: 2".
	sdkLogReconnecting = "trying to reconnect"

	// sdkLogConnected prefixes SDK lines like "connected to wss://.../gw[conn_id=...]".
	// Trailing space is intentional: it prevents accidental overlap with
	// "disconnected to ..." under HasPrefix matching (see feishu.go).
	sdkLogConnected = "connected to "

	// sdkLogDisconnected prefixes SDK lines like "disconnected to wss://.../gw[conn_id=...]".
	// Trailing space is intentional (see sdkLogConnected).
	sdkLogDisconnected = "disconnected to "
)
