// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"bytes"
	"testing"

	"github.com/larksuite/cli/internal/core"
)

// TestNewUATCallOptions validates the extraction of options from CLI config.
func TestNewUATCallOptions(t *testing.T) {
	cfg := &core.CliConfig{
		AppID:      "app123",
		AppSecret:  "secret",
		Brand:      core.BrandLark,
		UserOpenId: "ou_test",
	}
	errOut := &bytes.Buffer{}

	opts := NewUATCallOptions(cfg, errOut)

	if opts.AppId != "app123" {
		t.Errorf("AppId = %q, want app123", opts.AppId)
	}
	if opts.AppSecret != "secret" {
		t.Errorf("AppSecret = %q, want secret", opts.AppSecret)
	}
	if opts.Domain != core.BrandLark {
		t.Errorf("Domain = %q, want lark", opts.Domain)
	}
	if opts.UserOpenId != "ou_test" {
		t.Errorf("UserOpenId = %q, want ou_test", opts.UserOpenId)
	}
	if opts.ErrOut != errOut {
		t.Error("ErrOut not set correctly")
	}
}
