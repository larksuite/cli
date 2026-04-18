// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

// bindMsg holds all TUI text for config bind, supporting zh/en via --lang.
type bindMsg struct {
	// Source selection
	SelectSource     string
	SelectSourceDesc string
	SourceOpenClaw   string // format string: %s = resolved config path
	SourceHermes     string // format string: %s = resolved dotenv path

	// Account selection (OpenClaw multi-account)
	SelectAccount string

	// Conflict prompt
	ConflictTitle     string
	ConflictDesc      string // format: workspace, appId, brand, configPath
	ConflictForce     string
	ConflictCancel    string
	ConflictCancelled string

	// Security disclaimer
	SecurityTitle string
	SecurityDesc  string // format: source, configPath, source

	// Strict mode / default-as
	SelectStrictMode     string
	SelectStrictModeDesc string
	StrictModeOff        string
	StrictModeBot        string
	StrictModeUser       string

	SelectDefaultAs     string
	SelectDefaultAsDesc string
	DefaultAsAuto       string
	DefaultAsUser       string
	DefaultAsBot        string

	// Success / error
	BindSuccess string
}

var bindMsgZh = &bindMsg{
	SelectSource:     "选择绑定来源",
	SelectSourceDesc: "将哪个 Agent 的飞书凭据同步到 lark-cli？",
	SourceOpenClaw:   "OpenClaw（从 %s 读取）",
	SourceHermes:     "Hermes（从 %s 读取）",

	SelectAccount: "检测到多个飞书账号，请选择一个绑定",

	ConflictTitle:     "检测到已有绑定",
	ConflictDesc:      "工作区 %q 已绑定:\n  App ID:  %s\n  Brand:   %s\n  配置文件: %s",
	ConflictForce:     "重新绑定（覆盖当前配置）",
	ConflictCancel:    "取消（保持当前配置不变）",
	ConflictCancelled: "绑定已取消，未做任何修改",

	SecurityTitle: "安全提示",
	SecurityDesc: "绑定操作将:\n" +
		"  • 从 %s 同步凭据到 lark-cli 的 OS 钥匙串\n" +
		"  • 创建工作区配置文件: %s\n" +
		"  • Agent 侧后续修改不会自动同步到 lark-cli\n" +
		"  • 如需更新请重新执行: lark-cli config bind --source %s --force",

	SelectStrictMode:     "选择严格模式",
	SelectStrictModeDesc: "限制 API 调用时使用的身份类型",
	StrictModeOff:        "off — 不限制（推荐）",
	StrictModeBot:        "bot — 仅允许应用身份，隐藏用户相关命令",
	StrictModeUser:       "user — 仅允许用户身份，隐藏应用相关命令",

	SelectDefaultAs:     "选择默认身份",
	SelectDefaultAsDesc: "未显式指定身份时的默认调用方式",
	DefaultAsAuto:       "auto — 自动推断（默认）",
	DefaultAsUser:       "user — 默认以用户身份调用",
	DefaultAsBot:        "bot — 默认以应用身份调用",

	BindSuccess: "绑定成功",
}

var bindMsgEn = &bindMsg{
	SelectSource:     "Select Agent source to bind",
	SelectSourceDesc: "Which Agent's Feishu credentials should lark-cli use?",
	SourceOpenClaw:   "OpenClaw (read from %s)",
	SourceHermes:     "Hermes (read from %s)",

	SelectAccount: "Multiple Feishu accounts found — select one to bind",

	ConflictTitle:     "Existing binding detected",
	ConflictDesc:      "Workspace %q already bound:\n  App ID:  %s\n  Brand:   %s\n  Config:  %s",
	ConflictForce:     "Force — re-bind with fresh credentials",
	ConflictCancel:    "Cancel — keep existing binding",
	ConflictCancelled: "bind cancelled by user, no changes made",

	SecurityTitle: "Security notice",
	SecurityDesc: "Binding will:\n" +
		"  • Sync credentials from %s to lark-cli's OS keychain\n" +
		"  • Create workspace config at %s\n" +
		"  • Agent-side config changes will NOT auto-sync back\n" +
		"  • To update, re-run: lark-cli config bind --source %s --force",

	SelectStrictMode:     "Strict mode",
	SelectStrictModeDesc: "Restrict which identity type can be used for API calls",
	StrictModeOff:        "off — no restriction (recommended)",
	StrictModeBot:        "bot — only app identity allowed, user commands hidden",
	StrictModeUser:       "user — only user identity allowed, app commands hidden",

	SelectDefaultAs:     "Default identity",
	SelectDefaultAsDesc: "Identity to use when not explicitly specified",
	DefaultAsAuto:       "auto — infer automatically (default)",
	DefaultAsUser:       "user — default to user identity",
	DefaultAsBot:        "bot — default to app identity",

	BindSuccess: "Bind successful",
}

func getBindMsg(lang string) *bindMsg {
	if lang == "en" {
		return bindMsgEn
	}
	return bindMsgZh
}
