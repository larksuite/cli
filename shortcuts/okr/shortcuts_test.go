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
		commands := make([]string, 0, len(list))
		for _, shortcut := range list {
			commands = append(commands, shortcut.Command)
		}
		convey.So(commands, convey.ShouldContain, "+create")
		convey.So(commands, convey.ShouldContain, "+batch-create")
		convey.So(commands, convey.ShouldContain, "+patch")
	})
}
