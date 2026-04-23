// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestShortcutsRegistration(t *testing.T) {
	convey.Convey("Shortcuts() returns all commands", t, func() {
		list := Shortcuts()
		convey.So(len(list), convey.ShouldBeGreaterThan, 0)
	})
}
