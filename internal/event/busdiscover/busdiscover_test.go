// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package busdiscover

import "testing"

func TestParseAppIDFromCmdline(t *testing.T) {
	for _, tt := range []struct {
		name    string
		cmdline string
		want    string
	}{
		{
			name:    "unix full path bus",
			cmdline: "/Users/bytedance/go/src/github/cli/lark-cli event _bus --profile cli_XXXXXXXXXXXXXXXX --domain https://open.feishu.cn",
			want:    "cli_XXXXXXXXXXXXXXXX",
		},
		{
			name:    "windows quoted exe bus",
			cmdline: `"C:\Program Files\lark-cli\lark-cli.exe" event _bus --profile cli_XXXXXXXXXXXXXXXX --domain https://open.larksuite.com`,
			want:    "cli_XXXXXXXXXXXXXXXX",
		},
		{
			name:    "no profile flag",
			cmdline: "/usr/local/bin/lark-cli event _bus --domain https://open.feishu.cn",
			want:    "",
		},
		{
			name:    "not a bus process",
			cmdline: "/usr/local/bin/lark-cli event consume im.message.receive_v1",
			want:    "",
		},
		{
			name:    "completely unrelated",
			cmdline: "/Applications/Visual Studio Code.app/Contents/MacOS/Electron --type=renderer",
			want:    "",
		},
		{
			name:    "lark-cli but not event",
			cmdline: "/usr/local/bin/lark-cli auth login",
			want:    "",
		},
		{
			name:    "profile arg with trailing space and more flags",
			cmdline: "lark-cli event _bus --profile cli_xyz123 --verbose",
			want:    "cli_xyz123",
		},
		{
			name:    "empty cmdline",
			cmdline: "",
			want:    "",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := parseAppIDFromCmdline(tt.cmdline)
			if got != tt.want {
				t.Errorf("parseAppIDFromCmdline(%q) = %q, want %q", tt.cmdline, got, tt.want)
			}
		})
	}
}
