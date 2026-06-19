// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/event"
)

// failDialTransport refuses every dial so EnsureBus falls through to the
// remote-connection check without a local bus.
type failDialTransport struct{}

func (failDialTransport) Listen(string) (net.Listener, error) { return nil, errors.New("no listen") }
func (failDialTransport) Dial(string) (net.Conn, error)       { return nil, errors.New("refused") }
func (failDialTransport) Address(string) string               { return "guard-test-addr" }
func (failDialTransport) Cleanup(string)                      {}

// remoteBusyAPIClient reports active remote WebSocket connections.
type remoteBusyAPIClient struct{ count int }

func (c remoteBusyAPIClient) CallAPI(context.Context, string, string, interface{}) (json.RawMessage, error) {
	return json.RawMessage(`{"code":0,"msg":"ok","data":{"online_instance_cnt":` +
		strconv.Itoa(c.count) + `}}`), nil
}

func TestEnsureBus_RemoteBusAlreadyConnectedIsFailedPrecondition(t *testing.T) {
	conn, err := EnsureBus(context.Background(), failDialTransport{},
		"cli_guard_test", "", "", remoteBusyAPIClient{count: 2}, io.Discard)
	if conn != nil {
		t.Fatal("expected nil conn when a remote bus is already connected")
	}
	if err == nil {
		t.Fatal("expected single-bus guard error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Subtype != errs.SubtypeFailedPrecondition {
		t.Errorf("subtype = %s, want %s", ve.Subtype, errs.SubtypeFailedPrecondition)
	}
	wantHints := []string{
		"remote event connection",
		"`lark-cli event status` and `lark-cli event stop` only inspect local buses",
		"stop the owner host/process",
		"wait for the platform connection timeout",
	}
	for _, want := range wantHints {
		if !strings.Contains(ve.Hint, want) {
			t.Errorf("hint missing %q\ngot: %q", want, ve.Hint)
		}
	}
}

func TestRun_UnknownEventKeyIsTypedValidation(t *testing.T) {
	err := Run(context.Background(), failDialTransport{}, "cli_x", "", "", Options{
		EventKey: "bogus.run.key",
		ErrOut:   io.Discard,
	})
	if err == nil {
		t.Fatal("expected unknown EventKey error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("subtype = %s, want %s", ve.Subtype, errs.SubtypeInvalidArgument)
	}
	if !strings.Contains(ve.Hint, "event list") {
		t.Errorf("hint should point at `event list`, got: %q", ve.Hint)
	}
}

func TestRun_InvalidJQFailsBeforeAnySideEffect(t *testing.T) {
	event.RegisterKey(event.KeyDefinition{
		Key:       "consume.runtest.jq",
		EventType: "consume.runtest.jq_v1",
		Schema:    event.SchemaDef{Custom: &event.SchemaSpec{Raw: json.RawMessage(`{}`)}},
	})
	err := Run(context.Background(), failDialTransport{}, "cli_x", "", "", Options{
		EventKey: "consume.runtest.jq",
		JQExpr:   "[invalid{{{",
		ErrOut:   io.Discard,
	})
	if err == nil {
		t.Fatal("expected jq validation error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Param != "--jq" {
		t.Errorf("param = %q, want %q", ve.Param, "--jq")
	}
}
