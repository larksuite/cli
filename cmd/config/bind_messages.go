// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

// bindMsg holds all TUI text for config bind, supporting zh/en via --lang.
//
// Brand-aware strings use a %s slot where the UI-friendly product name
// should appear; callers pass brandDisplay(brand, lang) at that position.
// English templates use %[N]s positional indices when the natural English
// order puts brand before source.
type bindMsg struct {
	// Source selection.
	// SelectSourceDesc format: brand.
	SelectSource     string
	SelectSourceDesc string
	SourceOpenClaw   string // format: resolved config path.
	SourceHermes     string // format: resolved dotenv path.

	// Account selection (OpenClaw multi-account).
	// Format: source display name ("OpenClaw" | "Hermes"), brand.
	SelectAccount string

	// Conflict prompt.
	ConflictTitle     string
	ConflictDesc      string // format: workspace, appId, brand, configPath.
	ConflictForce     string
	ConflictCancel    string
	ConflictCancelled string

	// Post-bind agent-friendly message emitted in the stdout JSON envelope's
	// "message" field. Written as imperative instructions to the agent reading
	// the JSON — not as description for a human reader.
	// MessageBotOnly format: app_id, source display name, brand.
	// MessageUserDefault format: app_id, source display name, source display
	// name (second source ref anchors the "run in this chat" directive).
	// MessageUserDefault directs the Agent at the blocking single-call
	// `auth login --recommend` flow: the CLI streams verification_url to
	// stderr, which Agent runtimes (OpenClaw, Hermes) relay to the user in
	// real time, then blocks until the user authorizes in their own browser.
	// The Agent also needs an explicit "do not navigate the URL yourself"
	// guard — its own browser is sandboxed and cannot complete the user's
	// authorization.
	MessageBotOnly     string
	MessageUserDefault string

	// Identity preset (collapses strict-mode + default-as into one choice).
	// IdentityBotOnly/IdentityUserDefault are short, single-line labels for
	// the huh Select options. IdentityBotOnlyDesc / IdentityUserDefaultDesc
	// carry the longer explanation for each choice; tuiSelectIdentity
	// embeds the description under its label as a multi-line option value,
	// so huh renders the whole "label + indented description" block as one
	// picker row and styles it selected / unselected as a unit. Dynamic
	// DescriptionFunc was tried first but breaks here: a longer description
	// on hover pushes the field's initial viewport, clipping the selected
	// option row on terminals that fit the smaller description.
	// IdentityBotOnlyDesc format: brand.
	// IdentityUserDefaultDesc format: brand, brand.
	SelectIdentity          string
	IdentityBotOnly         string
	IdentityUserDefault     string
	IdentityBotOnlyDesc     string
	IdentityUserDefaultDesc string

	// Post-bind success notice printed to stderr once the workspace config
	// has been durably written. Rendered as two parts joined with "\n":
	//   BindSuccessHeader — format: source display name.
	//   BindSuccessNotice — caveat about one-time sync.
	// We intentionally do NOT emit a "replaced" suffix here (the TUI already
	// asked the user to confirm overwrite; flag mode carries `replaced:true`
	// in the stdout JSON envelope), and we do NOT emit an inline "next step"
	// line for user-default (stderr is the human channel; agents read the
	// MessageUserDefault field in the JSON envelope).
	BindSuccessHeader string
	BindSuccessNotice string

	// IdentityEscalationMessage / IdentityEscalationHint are returned when a
	// previous bind set the workspace to bot-only and a flag-mode (AI-driven)
	// caller tries to rebind with --identity user-default without --force.
	// The error asks the Agent to surface the risk to the user and re-run
	// with --force only after explicit user confirmation. TUI mode does not
	// hit this code path — tuiConflictPrompt + tuiSelectIdentity already
	// require in-flow human confirmation.
	IdentityEscalationMessage string
	IdentityEscalationHint    string
}

var bindMsgZh = &bindMsg{
	SelectSource:     "你想在哪个 Agent 中使用 lark-cli?",
	SelectSourceDesc: "从你选择的 Agent 中获取%s应用信息，并配置到 lark-cli 中",
	SourceOpenClaw:   "OpenClaw — 配置文件: %s",
	SourceHermes:     "Hermes — 配置文件: %s",

	SelectAccount: "检测到 %s 中已配置多个%s应用，请选择一个",

	ConflictTitle:     "检测到已有配置",
	ConflictDesc:      "%q 已配置 lark-cli:\n  App ID:  %s\n  品牌:    %s\n  配置文件: %s",
	ConflictForce:     "修改配置",
	ConflictCancel:    "保留当前配置",
	ConflictCancelled: "已保留当前配置",

	MessageBotOnly:     "已绑定应用 %s 到 %s，可立即以应用（bot）身份调用%s API，现在可以继续执行用户的请求。",
	MessageUserDefault: "已绑定应用 %s 到 %s。请接着在此 %s 对话中运行 `lark-cli auth login --recommend`。该命令会在 stderr 打出 verification_url 后阻塞等待用户授权；请将此链接原样发给用户在其浏览器中完成授权（不要自己调 browser_navigate 之类的工具打开，授权必须在用户的浏览器里完成），命令会在用户授权完成后自动返回。",

	SelectIdentity:      "你希望 AI 如何与你协作？",
	IdentityBotOnly:     "以机器人身份",
	IdentityUserDefault: "以你的身份",
	IdentityBotOnlyDesc: "AI 将在%s中以机器人的身份执行所有操作，适合作为团队助手，用于多人协作场景，如群聊问答、团队通知、公共文档维护。",
	IdentityUserDefaultDesc: "AI 将在%s中以你的名义执行所有操作，如读写文档、搜索消息、修改日程等，建议仅限个人使用。\n" +
		"⚠️  请勿将此机器人分享给他人或拉入群聊中使用，以免泄露你的%s数据。",

	BindSuccessHeader: "配置成功！lark-cli 已可在 %s 中使用。",
	BindSuccessNotice: "注意：这是一次性同步，后续 Agent 配置变更不会自动更新到 lark-cli。如需重新同步，请执行 `lark-cli config bind`",

	IdentityEscalationMessage: "你正在从应用身份切换到用户身份 —— 切换后 AI 将以你的名义在飞书中执行所有操作（读写文档、搜索消息、修改日程等）。⚠️ 请勿将此机器人分享给他人或拉入群聊中使用，以免泄露你的飞书数据。",
	IdentityEscalationHint:    "若用户确认切换，附加 --force 重新运行：`lark-cli config bind --identity user-default --force`",
}

var bindMsgEn = &bindMsg{
	SelectSource:     "Which Agent are you running?",
	SelectSourceDesc: "lark-cli will read your %s app credentials from the selected Agent and apply them automatically.",
	SourceOpenClaw:   "OpenClaw — config: %s",
	SourceHermes:     "Hermes — config: %s",

	// Args order (source, brand) matches the Chinese template; %[N]s lets the
	// English reading order differ while the caller passes args in one order.
	SelectAccount: "Multiple %[2]s apps configured in %[1]s — select one to continue.",

	ConflictTitle:     "Existing configuration found",
	ConflictDesc:      "lark-cli is already set up for %q:\n  App ID:  %s\n  Brand:   %s\n  Config:  %s",
	ConflictForce:     "Update config",
	ConflictCancel:    "Keep current config",
	ConflictCancelled: "Current config kept. No changes made.",

	MessageBotOnly:     "Bound app %s to %s. The %s app (bot) identity is ready — you can now continue with the user's request.",
	MessageUserDefault: "Bound app %s to %s. Next, in this %s chat, run `lark-cli auth login --recommend`. The command prints the verification URL to stderr and then blocks until the user authorizes it; relay the URL to the user so they can approve it in their own browser (do not call browser_navigate or any tool that opens a browser yourself — your browser is sandboxed and cannot complete the authorization). The command returns automatically once authorization completes.",

	SelectIdentity:      "How should the AI work with you?",
	IdentityBotOnly:     "As bot",
	IdentityUserDefault: "As you",
	IdentityBotOnlyDesc: "Works under its own identity in %s. Best for group chats, team notifications, and shared documents.",
	IdentityUserDefaultDesc: "Works under your identity in %s, managing docs, messages, calendar, and more on your behalf. Personal use only.\n" +
		"⚠️  Don't share this bot with others or add it to group chats. It has access to your personal %s data.",

	BindSuccessHeader: "All set! lark-cli is now ready to use in %s.",
	BindSuccessNotice: "Note: This is a one-time sync. To re-sync future changes, run `lark-cli config bind`",

	IdentityEscalationMessage: "you are switching from bot-only to user-default — the AI will then act under your Feishu identity for all operations (docs, messages, calendar, etc.). ⚠️ Don't share this bot with others or add it to group chats. It has access to your personal Feishu data.",
	IdentityEscalationHint:    "if the user confirms the switch, re-run with --force: `lark-cli config bind --identity user-default --force`",
}

func getBindMsg(lang string) *bindMsg {
	if lang == "en" {
		return bindMsgEn
	}
	return bindMsgZh
}

// brandDisplay returns the UI-friendly product name for the given brand
// identifier and display language. "lark" maps to "Lark" in both zh and en.
// "feishu" (or empty / unknown) maps to "飞书" in zh and "Feishu" in en —
// this is the safe default when the brand hasn't been resolved yet (for
// example, on the pre-binding source-selection screen).
func brandDisplay(brand, lang string) string {
	if brand == "lark" || brand == "Lark" || brand == "LARK" {
		return "Lark"
	}
	if lang == "en" {
		return "Feishu"
	}
	return "飞书"
}
