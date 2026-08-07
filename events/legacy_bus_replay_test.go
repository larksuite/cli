// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package events_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	eventlib "github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/consume"
	"github.com/larksuite/cli/internal/event/testutil"
)

// replayAppID must match the app_id the baseline fixtures put in their header:
// the compatibility path checks the header's claim against the app the
// consumer is configured for, so a mismatch here would be a dropped event
// rather than a rendered one.
const replayAppID = "cli-baseline-app"

// A consumer attached to a bus that predates canonical metadata must still
// print exactly what it printed before. This drives the real pipeline — bus
// handshake, frame decode, compatibility restore, arbitration, Match, Process,
// sink — off a byte-for-byte reproduction of the old wire format, and compares
// stdout against the frozen baseline.
//
// Asserting on the restored canonical event instead would be weaker in a way
// that matters: the legacy frame has no observation clock, so its canonical
// event legitimately differs from a current one, and any future Process that
// reads a field the two disagree on would keep both a field-level test and the
// output baseline green while the bytes diverged. Comparing the output closes
// that gap for every field at once, including fields nothing reads today.
func TestLegacyBusReplay_RendersTheFrozenOutput(t *testing.T) {
	want := readFrozenBaseline(t)

	for _, def := range compileRealCatalog(t).Definitions() {
		if def.Process == nil {
			continue
		}
		fx, ok := baselineFixtures[def.Key]
		if !ok {
			t.Fatalf("Processed EventKey %q has no baseline fixture", def.Key)
		}
		expected, ok := want[def.Key]
		if !ok {
			t.Fatalf("baseline snapshot has no entry for %q", def.Key)
		}

		t.Run(def.Key, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

			payload := buildBaselineEnvelope(t, def.EventType, fx)
			frame := testutil.LegacyEventFrame(def.EventType, baselineEventID, baselineCreateTime, 1, payload)
			tr := testutil.NewBusStub(testutil.LegacyAck, frame).Listen(t, replayAppID)

			// The context bounds a regression: if the consumer stopped exiting
			// on the event bound it would otherwise block until the package
			// timeout and report that instead of the real failure.
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			var out bytes.Buffer
			err := consume.Run(ctx, tr, replayAppID, "", "", consume.Options{
				EventKey:  def.Key,
				Def:       def,
				Runtime:   &baselineAPIClient{t: t},
				Out:       &out,
				ErrOut:    io.Discard,
				Quiet:     true,
				MaxEvents: 1,
				// The declaration's own preparation would reach for the
				// subscription API, which has nothing to do with what gets
				// printed. Injecting a no-op keeps this test about output and
				// exercises the preparation seam the command uses in
				// production.
				Prepare: func(context.Context) (func() error, error) { return nil, nil },
			})
			if err != nil {
				t.Fatalf("consuming a legacy frame must succeed, got: %v", err)
			}

			assertSameJSONBytes(t, out.Bytes(), expected)
		})
	}
}

// The one key that cannot fall back must refuse, on the same real path: its
// subscription identity is hashed here and bare on an older bus, so old and
// new consumers of the same board would unsubscribe one another.
func TestLegacyBusReplay_RefusesTheResourceScopedKey(t *testing.T) {
	var refused *eventlib.KeyDefinition
	for _, def := range compileRealCatalog(t).Definitions() {
		for _, p := range def.Params {
			if p.SubscriptionKey {
				refused = def
			}
		}
	}
	if refused == nil {
		t.Skip("no shipped key carries a SubscriptionKey param")
	}

	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	tr := testutil.NewBusStub(testutil.LegacyAck).Listen(t, replayAppID)

	// Short deadline on purpose: a regression that degrades instead of refusing
	// would enter the consume loop, and without this the failure would surface
	// as a package timeout pointing at the wrong thing.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	params := map[string]string{}
	for _, p := range refused.Params {
		if p.Required {
			params[p.Name] = "resource-1"
		}
	}

	err := consume.Run(ctx, tr, replayAppID, "", "", consume.Options{
		EventKey: refused.Key,
		Def:      refused,
		Params:   params,
		Runtime:  &baselineAPIClient{t: t},
		Out:      io.Discard,
		ErrOut:   io.Discard,
		Quiet:    true,
	})
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("%s must refuse a legacy bus with a failed_precondition, got: %v", refused.Key, err)
	}
}

func readFrozenBaseline(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(baselineSnapshotPath)
	if err != nil {
		t.Fatalf("read frozen baseline %s: %v", baselineSnapshotPath, err)
	}
	var snapshot map[string]json.RawMessage
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatalf("decode frozen baseline: %v", err)
	}
	return snapshot
}

// assertSameJSONBytes compares rendered stdout with a baseline entry after
// normalizing insignificant whitespace only: the stream is newline-delimited
// and the snapshot is stored indented, but every value inside must match.
func assertSameJSONBytes(t *testing.T, got, want json.RawMessage) {
	t.Helper()
	gotCompact, wantCompact := new(bytes.Buffer), new(bytes.Buffer)
	if err := json.Compact(gotCompact, bytes.TrimSpace(got)); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nraw: %s", err, got)
	}
	if err := json.Compact(wantCompact, want); err != nil {
		t.Fatalf("baseline entry is not valid JSON: %v", err)
	}
	if !bytes.Equal(gotCompact.Bytes(), wantCompact.Bytes()) {
		t.Errorf("legacy replay diverged from the frozen output:\n got %s\nwant %s", gotCompact, wantCompact)
	}
}
