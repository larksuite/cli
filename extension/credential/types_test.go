// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import "testing"

func TestIdentitySupport_Has(t *testing.T) {
	if !SupportsAll.Has(SupportsUser) {
		t.Error("SupportsAll should have SupportsUser")
	}
	if !SupportsAll.Has(SupportsBot) {
		t.Error("SupportsAll should have SupportsBot")
	}
	if SupportsUser.Has(SupportsBot) {
		t.Error("SupportsUser should not have SupportsBot")
	}
}

func TestIdentitySupport_UserOnly(t *testing.T) {
	if !SupportsUser.UserOnly() {
		t.Error("SupportsUser.UserOnly() should be true")
	}
	if SupportsAll.UserOnly() {
		t.Error("SupportsAll.UserOnly() should be false")
	}
	if IdentitySupport(0).UserOnly() {
		t.Error("zero value UserOnly() should be false")
	}
}

func TestIdentitySupport_BotOnly(t *testing.T) {
	if !SupportsBot.BotOnly() {
		t.Error("SupportsBot.BotOnly() should be true")
	}
	if SupportsAll.BotOnly() {
		t.Error("SupportsAll.BotOnly() should be false")
	}
}
