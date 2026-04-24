// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/event/protocol"
)

// seqGapDetector mirrors the inline gap-detection logic from consumeLoop's
// reader goroutine so tests can exercise it in isolation. If this logic
// changes in loop.go, update here too.
type seqGapDetector struct {
	lastSeq uint64
	errOut  io.Writer
	quiet   bool
}

func (d *seqGapDetector) observe(m *protocol.Event) {
	if d.lastSeq > 0 && m.Seq > 0 && m.Seq > d.lastSeq+1 {
		gap := m.Seq - d.lastSeq - 1
		if !d.quiet {
			fmt.Fprintf(d.errOut, "WARN: event seq gap %d->%d, missed %d events (dropped by bus backpressure)\n",
				d.lastSeq, m.Seq, gap)
		}
	}
	// CRITICAL: only advance forward to tolerate out-of-order arrivals from
	// concurrent Publishers winning sendMu races.
	if m.Seq > d.lastSeq {
		d.lastSeq = m.Seq
	}
}

func TestSeqGapDetectorNoWarningOnFirstEvent(t *testing.T) {
	var buf bytes.Buffer
	d := &seqGapDetector{errOut: &buf}
	d.observe(&protocol.Event{Seq: 5})
	if strings.Contains(buf.String(), "gap") {
		t.Errorf("unexpected gap warning on first event: %s", buf.String())
	}
}

func TestSeqGapDetectorNoWarningOnContiguous(t *testing.T) {
	var buf bytes.Buffer
	d := &seqGapDetector{errOut: &buf}
	for i := uint64(1); i <= 10; i++ {
		d.observe(&protocol.Event{Seq: i})
	}
	if buf.Len() > 0 {
		t.Errorf("unexpected output on contiguous seqs: %s", buf.String())
	}
}

func TestSeqGapDetectorWarnsOnActualGap(t *testing.T) {
	var buf bytes.Buffer
	d := &seqGapDetector{errOut: &buf}
	d.observe(&protocol.Event{Seq: 1})
	d.observe(&protocol.Event{Seq: 5}) // skipped 2, 3, 4
	out := buf.String()
	if !strings.Contains(out, "gap 1->5") {
		t.Errorf("expected 'gap 1->5' in output, got: %s", out)
	}
	if !strings.Contains(out, "missed 3 events") {
		t.Errorf("expected 'missed 3 events' in output, got: %s", out)
	}
}

// TestSeqGapDetectorHandlesOutOfOrderWithoutFalsePositive is the regression
// test for the bug where lastSeq went backwards. Under concurrent Publishers,
// msg Seq=6 may arrive before msg Seq=5 (B wins sendMu). The detector must
// NOT emit a false gap warning when Seq=7 arrives after that sequence.
func TestSeqGapDetectorHandlesOutOfOrderWithoutFalsePositive(t *testing.T) {
	var buf bytes.Buffer
	d := &seqGapDetector{errOut: &buf}
	d.observe(&protocol.Event{Seq: 6}) // arrives first
	d.observe(&protocol.Event{Seq: 5}) // arrives out-of-order
	d.observe(&protocol.Event{Seq: 7}) // next real event
	if buf.Len() > 0 {
		t.Errorf("unexpected warning for out-of-order (no actual gap): %s", buf.String())
	}
}

func TestSeqGapDetectorQuietMode(t *testing.T) {
	var buf bytes.Buffer
	d := &seqGapDetector{errOut: &buf, quiet: true}
	d.observe(&protocol.Event{Seq: 1})
	d.observe(&protocol.Event{Seq: 10}) // big gap
	if buf.Len() > 0 {
		t.Errorf("quiet mode should suppress warnings, got: %s", buf.String())
	}
}

func TestSeqGapDetectorZeroSeqIgnored(t *testing.T) {
	// Legacy events without Seq (old Bus version) have Seq=0. The detector
	// should neither warn nor advance lastSeq on these.
	var buf bytes.Buffer
	d := &seqGapDetector{errOut: &buf}
	d.observe(&protocol.Event{Seq: 5})
	d.observe(&protocol.Event{Seq: 0}) // legacy
	d.observe(&protocol.Event{Seq: 6}) // expected next
	if buf.Len() > 0 {
		t.Errorf("unexpected warning across legacy zero-seq event: %s", buf.String())
	}
	if d.lastSeq != 6 {
		t.Errorf("expected lastSeq=6 after legacy skip, got %d", d.lastSeq)
	}
}
