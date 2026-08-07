// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/output"
)

// The Out* methods are wiring: each builds an output.EmitOptions and hands it to
// the Emitter, which does the formatting. This file pins the wiring — that the
// option each method promises in its name is the option the Emitter receives.
//
// It lives here rather than beside the Emitter's frozen golden fixtures in
// internal/output because the assertion is about this package's methods, and a
// test in internal/output would have to import shortcuts/common: the upward
// dependency the internal-no-upper rule forbids, which went unnoticed while the
// layering gate read production imports only.
//
// The distinction under test is visible in the bytes: Raw disables HTML escaping,
// so `<p>a&b</p>` either survives or comes back as <p>….
const wiringHTMLPayloadKey = "html"

const (
	wiringHTMLRaw     = `<p>a&b</p>`
	wiringHTMLEscaped = `\u003cp\u003ea\u0026b\u003c/p\u003e`
)

func wiringPayload() map[string]interface{} {
	return map[string]interface{}{wiringHTMLPayloadKey: wiringHTMLRaw}
}

func TestRuntimeContextOutEscapesHTML(t *testing.T) {
	rctx, stdout, _ := newJqTestContext("", "")

	rctx.Out(wiringPayload(), nil)

	if got := stdout.String(); !strings.Contains(got, wiringHTMLEscaped) {
		t.Fatalf("Out() stdout = %s, want the HTML escaped as %s", got, wiringHTMLEscaped)
	}
}

func TestRuntimeContextOutRawPreservesHTML(t *testing.T) {
	rctx, stdout, _ := newJqTestContext("", "")

	rctx.OutRaw(wiringPayload(), nil)

	got := stdout.String()
	if !strings.Contains(got, wiringHTMLRaw) {
		t.Fatalf("OutRaw() stdout = %s, want the HTML preserved as %s", got, wiringHTMLRaw)
	}
	if strings.Contains(got, wiringHTMLEscaped) {
		t.Fatalf("OutRaw() stdout = %s, want no escaped HTML — Raw was not passed through", got)
	}
}

// TestRuntimeContextOutFormatRawPreservesHTML covers the method the golden oracle
// was the only test to reach: OutFormatRaw has to set both Format and Raw, and
// dropping either one is invisible unless the payload contains HTML.
func TestRuntimeContextOutFormatRawPreservesHTML(t *testing.T) {
	rctx, stdout, _ := newJqTestContext("", "json")

	rctx.OutFormatRaw(wiringPayload(), nil, nil)

	got := stdout.String()
	if !strings.Contains(got, wiringHTMLRaw) {
		t.Fatalf("OutFormatRaw() stdout = %s, want the HTML preserved as %s", got, wiringHTMLRaw)
	}
	if strings.Contains(got, wiringHTMLEscaped) {
		t.Fatalf("OutFormatRaw() stdout = %s, want no escaped HTML — Raw was not passed through", got)
	}
}

// TestRuntimeContextOutFormatUsesRuntimeFormat pins that OutFormat reads
// ctx.Format rather than defaulting to the JSON envelope: with --format=pretty the
// renderer supplied by the caller owns stdout.
func TestRuntimeContextOutFormatUsesRuntimeFormat(t *testing.T) {
	rctx, stdout, _ := newJqTestContext("", "pretty")

	rctx.OutFormat(wiringPayload(), nil, func(w io.Writer) {
		fmt.Fprintln(w, "pretty:fixture")
	})

	got := stdout.String()
	if strings.TrimSpace(got) != "pretty:fixture" {
		t.Fatalf("OutFormat(format=pretty) stdout = %q, want the pretty renderer's output", got)
	}
}

// TestRuntimeContextOutCarriesMeta pins the last option the methods forward: a
// non-nil Meta has to reach the envelope, or a batch command silently loses the
// count and rollback hint it reported.
func TestRuntimeContextOutCarriesMeta(t *testing.T) {
	rctx, stdout, _ := newJqTestContext("", "")

	rctx.Out(wiringPayload(), &output.Meta{Count: 3, Rollback: "lark-cli undo"})

	var envelope struct {
		Meta *struct {
			Count    int    `json:"count"`
			Rollback string `json:"rollback"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("Out() stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if envelope.Meta == nil {
		t.Fatalf("Out() dropped meta from the envelope: %s", stdout.String())
	}
	if envelope.Meta.Count != 3 || envelope.Meta.Rollback != "lark-cli undo" {
		t.Fatalf("Out() meta = %+v, want count=3 rollback=%q", envelope.Meta, "lark-cli undo")
	}
}

// TestRuntimeContextOutCarriesNotice pins the last thing newEmitter forwards: the
// notice provider. Nothing else in this package would notice its removal — the
// envelope stays valid JSON without _notice, so a dropped assignment would only
// show up as users no longer being told their token is about to expire.
func TestRuntimeContextOutCarriesNotice(t *testing.T) {
	previous := output.PendingNotice
	t.Cleanup(func() { output.PendingNotice = previous })
	output.PendingNotice = func() map[string]interface{} {
		return map[string]interface{}{"warning": "token expires in 2 days"}
	}

	rctx, stdout, _ := newJqTestContext("", "")
	rctx.Out(wiringPayload(), nil)

	var envelope struct {
		Notice map[string]interface{} `json:"_notice"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("Out() stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if envelope.Notice["warning"] != "token expires in 2 days" {
		t.Fatalf("Out() _notice = %#v, want the pending notice — NoticeProvider is not reaching the Emitter", envelope.Notice)
	}
}
