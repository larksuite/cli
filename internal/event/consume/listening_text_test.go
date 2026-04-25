// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"bytes"
	"testing"
	"time"
)

func TestListeningText_TTY(t *testing.T) {
	got := listeningText(Options{EventKey: "im.message.receive_v1", IsTTY: true})
	want := "[event] listening for events (key=im.message.receive_v1), ctrl+c to stop"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestListeningText_NonTTY_Default(t *testing.T) {
	got := listeningText(Options{EventKey: "im.message.receive_v1", IsTTY: false})
	want := "[event] listening for events (key=im.message.receive_v1); send SIGTERM or close stdin to stop"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestListeningText_NonTTY_MaxEvents(t *testing.T) {
	got := listeningText(Options{EventKey: "im.message.receive_v1", IsTTY: false, MaxEvents: 1})
	want := "[event] listening for events (key=im.message.receive_v1); will exit after 1 event(s)"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestListeningText_NonTTY_Timeout(t *testing.T) {
	got := listeningText(Options{EventKey: "im.message.receive_v1", IsTTY: false, Timeout: 30 * time.Second})
	want := "[event] listening for events (key=im.message.receive_v1); will exit after 30s timeout"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestListeningText_NonTTY_MaxEventsAndTimeout(t *testing.T) {
	got := listeningText(Options{EventKey: "im.message.receive_v1", IsTTY: false, MaxEvents: 1, Timeout: 30 * time.Second})
	want := "[event] listening for events (key=im.message.receive_v1); will exit after 1 event(s) or 30s timeout"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// AI-facing contract: must name "kill -9" + "cleanup" so agents parsing stderr are steered away from SIGKILL.
func TestStopHintText_Content(t *testing.T) {
	got := stopHintText()
	mustContain := []string{"SIGTERM", "kill -9", "cleanup"}
	for _, s := range mustContain {
		if !bytes.Contains([]byte(got), []byte(s)) {
			t.Errorf("stopHintText missing %q; got %q", s, got)
		}
	}
}

func TestReadyMarker_EmittedAfterListening(t *testing.T) {
	var buf bytes.Buffer
	writeReadyMarker(&buf, Options{EventKey: "im.message.receive_v1"})

	got := buf.String()
	want := "[event] ready event_key=im.message.receive_v1\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestReadyMarker_SuppressedWhenQuiet(t *testing.T) {
	var buf bytes.Buffer
	writeReadyMarker(&buf, Options{EventKey: "im.message.receive_v1", Quiet: true})

	if buf.Len() != 0 {
		t.Errorf("Quiet=true must suppress ready marker; got %q", buf.String())
	}
}
