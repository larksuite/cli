// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestWarnDeprecatedV1SuggestsSkillUpdate(t *testing.T) {
	for _, shortcut := range []string{"+create", "+fetch", "+update"} {
		t.Run(shortcut, func(t *testing.T) {
			f, _, stderr, _ := cmdutil.TestFactory(t, &core.CliConfig{})
			warnDeprecatedV1(&common.RuntimeContext{Factory: f}, shortcut)

			got := stderr.String()
			for _, want := range []string{
				"[deprecated] docs " + shortcut + " is using the v1 API.",
				"Check the installed lark-doc skill first",
				"if it is not the v2 skill, run `lark-cli update` to upgrade skills",
			} {
				if !strings.Contains(got, want) {
					t.Fatalf("warning missing %q:\n%s", want, got)
				}
			}
			if strings.Contains(got, "will be removed in a future release") {
				t.Fatalf("warning should not include removal-only guidance:\n%s", got)
			}
		})
	}
}
