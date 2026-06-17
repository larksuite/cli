// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/event/protocol"
)

func TestRejectionError_Rejected(t *testing.T) {
	ack := &protocol.HelloAck{Type: protocol.MsgTypeHelloAck, Rejected: true, RejectReason: "another consumer (pid 9) is already running"}
	err := rejectionError(ack, "im.message.receive_v1")
	if err == nil {
		t.Fatal("expected error for rejected ack")
	}
	prob, ok := errs.ProblemOf(err)
	if !ok || prob.Category != errs.CategoryValidation || prob.Subtype != errs.SubtypeFailedPrecondition {
		t.Errorf("problem = %v, want validation/failed_precondition; err=%q", prob, err.Error())
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Errorf("error = %q, want reject reason", err.Error())
	}
}

func TestRejectionError_NotRejected(t *testing.T) {
	if err := rejectionError(&protocol.HelloAck{Type: protocol.MsgTypeHelloAck}, "k"); err != nil {
		t.Errorf("expected nil for non-rejected ack, got %v", err)
	}
	if err := rejectionError(nil, "k"); err != nil {
		t.Errorf("expected nil for nil ack, got %v", err)
	}
}
