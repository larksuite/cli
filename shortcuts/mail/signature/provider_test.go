// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package signature

import (
	"testing"
)

func TestDefaultSendID_Match(t *testing.T) {
	usages := []SignatureUsage{
		{EmailAddress: "user@example.com", SendMailSignatureID: "sig-send-1", ReplySignatureID: "sig-reply-1"},
	}
	if got := DefaultSendID(usages, "user@example.com"); got != "sig-send-1" {
		t.Fatalf("expected sig-send-1, got %q", got)
	}
}

func TestDefaultSendID_CaseInsensitive(t *testing.T) {
	usages := []SignatureUsage{
		{EmailAddress: "User@Example.COM", SendMailSignatureID: "sig-send-x"},
	}
	if got := DefaultSendID(usages, "user@example.com"); got != "sig-send-x" {
		t.Fatalf("expected case-insensitive match, got %q", got)
	}
}

func TestDefaultSendID_NoMatch(t *testing.T) {
	usages := []SignatureUsage{
		{EmailAddress: "other@example.com", SendMailSignatureID: "sig-other"},
	}
	if got := DefaultSendID(usages, "user@example.com"); got != "" {
		t.Fatalf("expected empty string for no match, got %q", got)
	}
}

func TestDefaultSendID_ZeroIDTreatedAsNone(t *testing.T) {
	usages := []SignatureUsage{
		{EmailAddress: "user@example.com", SendMailSignatureID: "0"},
	}
	if got := DefaultSendID(usages, "user@example.com"); got != "" {
		t.Fatalf("expected empty string for ID=0, got %q", got)
	}
}

func TestDefaultSendID_NilUsages(t *testing.T) {
	if got := DefaultSendID(nil, "user@example.com"); got != "" {
		t.Fatalf("expected empty string for nil usages, got %q", got)
	}
}

func TestDefaultReplyID_Match(t *testing.T) {
	usages := []SignatureUsage{
		{EmailAddress: "user@example.com", SendMailSignatureID: "sig-send-1", ReplySignatureID: "sig-reply-2"},
	}
	if got := DefaultReplyID(usages, "user@example.com"); got != "sig-reply-2" {
		t.Fatalf("expected sig-reply-2, got %q", got)
	}
}

func TestDefaultReplyID_CaseInsensitive(t *testing.T) {
	usages := []SignatureUsage{
		{EmailAddress: "User@Example.COM", ReplySignatureID: "sig-reply-x"},
	}
	if got := DefaultReplyID(usages, "user@example.com"); got != "sig-reply-x" {
		t.Fatalf("expected case-insensitive match, got %q", got)
	}
}

func TestDefaultReplyID_NoMatch(t *testing.T) {
	usages := []SignatureUsage{
		{EmailAddress: "other@example.com", ReplySignatureID: "sig-reply-other"},
	}
	if got := DefaultReplyID(usages, "user@example.com"); got != "" {
		t.Fatalf("expected empty string for no match, got %q", got)
	}
}

func TestDefaultReplyID_ZeroIDTreatedAsNone(t *testing.T) {
	usages := []SignatureUsage{
		{EmailAddress: "user@example.com", ReplySignatureID: "0"},
	}
	if got := DefaultReplyID(usages, "user@example.com"); got != "" {
		t.Fatalf("expected empty string for ID=0, got %q", got)
	}
}

func TestDefaultReplyID_NilUsages(t *testing.T) {
	if got := DefaultReplyID(nil, "user@example.com"); got != "" {
		t.Fatalf("expected empty string for nil usages, got %q", got)
	}
}
