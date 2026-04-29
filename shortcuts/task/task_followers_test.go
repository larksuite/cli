// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestBuildFollowersBody(t *testing.T) {
	convey.Convey("Build with ids and token", t, func() {
		body := buildFollowersBody("u1, u2 , ", "token1")
		members := body["members"].([]map[string]interface{})
		convey.So(len(members), convey.ShouldEqual, 2)
		convey.So(body["client_token"], convey.ShouldEqual, "token1")
		convey.So(members[0]["role"], convey.ShouldEqual, "follower")
		convey.So(members[0]["type"], convey.ShouldEqual, "user")
	})

	convey.Convey("Build infers app followers", t, func() {
		body := buildFollowersBody("cli_bot_1", "")
		members := body["members"].([]map[string]interface{})
		convey.So(len(members), convey.ShouldEqual, 1)
		convey.So(members[0]["id"], convey.ShouldEqual, "cli_bot_1")
		convey.So(members[0]["role"], convey.ShouldEqual, "follower")
		convey.So(members[0]["type"], convey.ShouldEqual, "app")
	})

	convey.Convey("Build infers mixed follower types in one list", t, func() {
		body := buildFollowersBody("ou_user_1, cli_bot_1", "")
		members := body["members"].([]map[string]interface{})
		convey.So(len(members), convey.ShouldEqual, 2)
		convey.So(members[0]["type"], convey.ShouldEqual, "user")
		convey.So(members[1]["type"], convey.ShouldEqual, "app")
	})
}
