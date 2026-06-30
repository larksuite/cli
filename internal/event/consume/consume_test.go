// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/protocol"
	"github.com/larksuite/cli/internal/event/transport"
)

// fakeRT is a minimal event.APIClient mock.
type fakeRT struct {
	err error
}

func (f *fakeRT) CallAPI(_ context.Context, _, _ string, _ interface{}) (json.RawMessage, error) {
	return nil, f.err
}

func TestNormalizeParams_ErrorIsWrappedWithEventKey(t *testing.T) {
	// Drives the real Run() path: NormalizeParams fails before EnsureBus, so no
	// bus is contacted, yet the production error-wrapping is exercised — if Run()
	// ever stops wrapping, this test fails.
	const key = "test.evt_normalize_fail"
	event.RegisterKey(event.KeyDefinition{
		Key:       key,
		EventType: key,
		Schema:    event.SchemaDef{Custom: &event.SchemaSpec{Raw: json.RawMessage(`{"type":"object"}`)}},
		NormalizeParams: func(_ context.Context, _ event.APIClient, _ map[string]string) error {
			return errors.New("simulated normalize failure")
		},
	})
	defer event.UnregisterKeyForTest(key)

	err := Run(context.Background(), transport.New(), "app", "", "", Options{
		EventKey: key,
		Runtime:  &fakeRT{},
		Quiet:    true,
	})
	if err == nil {
		t.Fatal("expected Run to fail when NormalizeParams errors")
	}
	if !strings.Contains(err.Error(), "normalize params for "+key+":") {
		t.Errorf("error not wrapped with EventKey prefix: %v", err)
	}
	if !strings.Contains(err.Error(), "simulated normalize failure") {
		t.Errorf("underlying error not propagated: %v", err)
	}
}

func TestValidateParamsRejectsUnknownPublicParam(t *testing.T) {
	const key = "test.evt_public_params"
	def := &event.KeyDefinition{
		Key: key,
		Params: []event.ParamDef{
			{Name: "mailbox"},
			{Name: "msg_format"},
		},
	}
	err := validateParams(def, map[string]string{
		"mailbox":  "me",
		"internal": "true",
	})
	if err == nil {
		t.Fatal("expected unknown param error")
	}
	if !strings.Contains(err.Error(), `unknown param "internal"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunRejectsInvalidEnumParamBeforePreConsume(t *testing.T) {
	const key = "test.evt_invalid_enum"
	var preConsumed atomic.Bool
	event.RegisterKey(event.KeyDefinition{
		Key:       key,
		EventType: key,
		Schema:    event.SchemaDef{Custom: &event.SchemaSpec{Raw: json.RawMessage(`{"type":"object"}`)}},
		Params: []event.ParamDef{{
			Name:    "msg_format",
			Type:    event.ParamEnum,
			Default: "metadata",
			Values: []event.ParamValue{
				{Value: "metadata", Desc: "Metadata only"},
				{Value: "minimal", Desc: "Minimal metadata"},
				{Value: "plain_text_full", Desc: "Plain text body"},
				{Value: "full", Desc: "Full payload"},
				{Value: "event", Desc: "Raw event"},
			},
		}},
		PreConsume: func(_ context.Context, _ event.APIClient, _ map[string]string) (func() error, error) {
			preConsumed.Store(true)
			return nil, nil
		},
	})
	defer event.UnregisterKeyForTest(key)

	params := map[string]string{"msg_format": "中文格式"}
	err := Run(context.Background(), transport.New(), "app", "", "", Options{
		EventKey: key,
		Params:   params,
		Runtime:  &fakeRT{},
		Out:      io.Discard,
		ErrOut:   io.Discard,
		Quiet:    true,
	})
	if err == nil {
		t.Fatal("expected invalid enum value error")
	}
	if preConsumed.Load() {
		t.Fatal("PreConsume should not run after invalid enum value")
	}
	for _, want := range []string{
		`param "msg_format"`,
		`中文格式`,
		"metadata, minimal, plain_text_full, full, event",
		"Run 'lark-cli event schema " + key + "'",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected %q in error, got: %v", want, err)
		}
	}
	if params["msg_format"] != "中文格式" {
		t.Fatalf("invalid value should not fall back to default, got %q", params["msg_format"])
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("ProblemOf() ok = false; error = %v", err)
	}
	if problem.Category != errs.CategoryValidation {
		t.Fatalf("category = %q, want %q", problem.Category, errs.CategoryValidation)
	}
	if problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("problem subtype = %q, want %q", problem.Subtype, errs.SubtypeInvalidArgument)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error type = %T, want *errs.ValidationError; error = %v", err, err)
	}
	if validationErr.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("subtype = %q, want %q", validationErr.Subtype, errs.SubtypeInvalidArgument)
	}
	if validationErr.Param != "msg_format" {
		t.Fatalf("param = %q, want msg_format", validationErr.Param)
	}
}

func TestDoHello_PassesSubscriptionIDToWire(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	// Server-side: read Hello, decode, assert SubscriptionID, send ack
	done := make(chan string, 1)
	go func() {
		br := bufio.NewReader(b)
		line, err := protocol.ReadFrame(br)
		if err != nil {
			done <- "READ_ERR:" + err.Error()
			return
		}
		msg, err := protocol.Decode(bytes.TrimRight(line, "\n"))
		if err != nil {
			done <- "DECODE_ERR:" + err.Error()
			return
		}
		if hello, ok := msg.(*protocol.Hello); ok {
			done <- hello.SubscriptionID
			// send ack so client can return
			ack := protocol.NewHelloAck("v1", true)
			_ = protocol.EncodeWithDeadline(b, ack, protocol.WriteTimeout)
		} else {
			done <- "WRONG_TYPE"
		}
	}()

	ack, _, err := doHello(a, "mail.x", []string{"mail.x"}, "mail.x:alice")
	if err != nil {
		t.Fatalf("doHello error: %v", err)
	}
	if ack == nil {
		t.Fatal("got nil ack")
	}
	got := <-done
	if got != "mail.x:alice" {
		t.Errorf("Hello.SubscriptionID on wire = %q, want %q", got, "mail.x:alice")
	}
}
