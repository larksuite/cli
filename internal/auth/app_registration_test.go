// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

// Test_BuildVerificationURL verifies that tracking parameters are correctly appended.
func Test_BuildVerificationURL(t *testing.T) {
	t.Run("URL不含问号则添加?分隔符", func(t *testing.T) {
		result := BuildVerificationURL("https://example.com/verify", "1.0.0")
		convey.Convey("should add ? separator", t, func() {
			convey.So(result, convey.ShouldContainSubstring, "?lpv=1.0.0")
			convey.So(result, convey.ShouldContainSubstring, "&ocv=1.0.0")
			convey.So(result, convey.ShouldContainSubstring, "&from=cli")
			convey.So(result, convey.ShouldStartWith, "https://example.com/verify?")
		})
	})

	t.Run("URL已含问号则添加&分隔符", func(t *testing.T) {
		result := BuildVerificationURL("https://example.com/verify?code=abc", "2.0.0")
		convey.Convey("should add & separator", t, func() {
			convey.So(result, convey.ShouldContainSubstring, "&lpv=2.0.0")
			convey.So(result, convey.ShouldContainSubstring, "&ocv=2.0.0")
			convey.So(result, convey.ShouldContainSubstring, "&from=cli")
			convey.So(result, convey.ShouldNotContainSubstring, "?lpv=")
		})
	})
}
