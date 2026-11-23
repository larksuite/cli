// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/larksuite/cli/internal/event"
)

// recordingBeanSource is the only recording source the vc.recording.* keys
// emit; events carrying any other source are silently filtered out.
const recordingBeanSource = "recording_bean"

// recordingBeanEventBody is the shared {"event": ...} body for
// recording_started and recording_ended, whose payloads carry identical fields.
type recordingBeanEventBody struct {
	UniqueKey string `json:"unique_key"`
	Source    string `json:"source"`
}

// decodeEventBody unmarshals the {"event": ...} envelope of raw and returns
// the decoded body; ok is false when the payload does not decode.
func decodeEventBody[T any](raw *event.RawEvent) (T, bool) {
	var envelope struct {
		Event T `json:"event"`
	}
	if err := json.Unmarshal(raw.Payload, &envelope); err != nil {
		var zero T
		return zero, false
	}
	return envelope.Event, true
}

// millisToLocalRFC3339 converts a unix-millisecond timestamp string to
// RFC3339 in the local timezone; empty or non-numeric input yields "".
func millisToLocalRFC3339(raw string) string {
	if raw == "" {
		return ""
	}
	millis, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return ""
	}
	return time.UnixMilli(millis).Local().Format(time.RFC3339)
}

// unixSecondsToLocalRFC3339 converts a unix-second timestamp string to
// RFC3339 in the local timezone; empty or non-numeric input yields "".
func unixSecondsToLocalRFC3339(raw string) string {
	if raw == "" {
		return ""
	}
	secs, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return ""
	}
	return time.Unix(secs, 0).Local().Format(time.RFC3339)
}
