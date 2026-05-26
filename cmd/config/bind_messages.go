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
	SelectSource      string
	SelectSourceDesc  string
	SourceOpenClaw    string // format: resolved config path.
	SourceHermes      string // format: resolved dotenv path.
	SourceLarkChannel string // format: resolved config path.

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
	SelectSource:      "你想在哪个 Agent 中使用 lark-cli?",
	SelectSourceDesc:  "从你选择的 Agent 中获取%s应用信息，并配置到 lark-cli 中",
	SourceOpenClaw:    "OpenClaw — 配置文件: %s",
	SourceHermes:      "Hermes — 配置文件: %s",
	SourceLarkChannel: "Lark Channel — 配置文件: %s",

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
	SelectSource:      "Which Agent are you running?",
	SelectSourceDesc:  "lark-cli will read your %s app credentials from the selected Agent and apply them automatically.",
	SourceOpenClaw:    "OpenClaw — config: %s",
	SourceHermes:      "Hermes — config: %s",
	SourceLarkChannel: "Lark Channel — config: %s",

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
	switch lang {
	case "en":
		return bindMsgEn
	case "ja":
		return bindMsgJa
	case "ko":
		return bindMsgKo
	case "fr", "de", "es", "it", "ru", "pt", "ar", "hi", "tr", "pl", "nl", "sv", "th", "vi", "id", "ms":
		return bindMsgEn
	default:
		return bindMsgZh
	}
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

var bindMsgJa = &bindMsg{
	SelectSource:              "どの Agent で実行していますか?",
	SelectSourceDesc:          "選択した Agent から%sアプリ情報を取得し、lark-cli に自動設定します",
	SourceOpenClaw:            "OpenClaw — 設定ファイル: %s",
	SourceHermes:              "Hermes — 設定ファイル: %s",
	SourceLarkChannel:         "Lark Channel — 設定ファイル: %s",
	SelectAccount:             "%[1]s で複数の%[2]sアプリが検出 — 続行するものを選択",
	ConflictTitle:             "既存の設定が見つかりました",
	ConflictDesc:              "%q は既に lark-cli を設定:\n  App ID:  %s\n  ブランド:   %s\n  設定ファイル:  %s",
	ConflictForce:             "設定を更新",
	ConflictCancel:            "現在の設定を保持",
	ConflictCancelled:         "現在の設定を保持しました。変更はありません。",
	MessageBotOnly:            "アプリ %s を %s にバインド。%s アプリ (bot)  identity の準備が完了 — ユーザーのリクエストを続行できます。",
	MessageUserDefault:        "アプリ %s を %s にバインド。次に、この %s チャットで `lark-cli auth login --recommend` を実行。コマンドは検証 URL を stderr に出力し、ユーザーが承認するまでブロックします。",
	SelectIdentity:            "AI はどのように動作しますか?",
	IdentityBotOnly:           "Bot として",
	IdentityUserDefault:       "あなたとして",
	IdentityBotOnlyDesc:       "%s で独自の identity で動作。グループチャット、チーム通知、共有ドキュメントに最適。",
	IdentityUserDefaultDesc:   "%s であなたの identity で動作し、ドキュメント、メッセージ、カレンダーなどを管理。個人使用のみ。\n⚠️  このボットを他の人と共有したり、グループチャットに追加したりしないでください。あなたの個人 %s データにアクセスできます。",
	BindSuccessHeader:         "準備完了! lark-cli は %s で使用可能です。",
	BindSuccessNotice:         "注: これは一回限りの同期です。将来の変更を再同期するには、`lark-cli config bind` を実行してください。",
	IdentityEscalationMessage: "bot-only から user-default に切り替え中 — AI はあなたの Feishu identity ですべての操作を実行します (ドキュメント、メッセージ、カレンダーなど)。⚠️ このボットを他の人と共有したり、グループチャットに追加したりしないでください。あなたの個人 Feishu データにアクセスできます。",
	IdentityEscalationHint:    "ユーザーが切り替えを確認した場合、`--force` で再実行: `lark-cli config bind --identity user-default --force`",
}

var bindMsgKo = &bindMsg{
	SelectSource:              "어떤 Agent를 실행 중인가요?",
	SelectSourceDesc:          "선택한 Agent에서 %s 앱 인증 정보를 읽어 lark-cli에 자동 적용합니다",
	SourceOpenClaw:            "OpenClaw — 설정 파일: %s",
	SourceHermes:              "Hermes — 설정 파일: %s",
	SourceLarkChannel:         "Lark Channel — 설정 파일: %s",
	SelectAccount:             "%[1]s에서 여러 %[2]s 앱이 설정됨 — 계속할 것을 선택",
	ConflictTitle:             "기존 설정 발견",
	ConflictDesc:              "%q는 이미 lark-cli가 설정됨:\n  App ID:  %s\n  브랜드:   %s\n  설정 파일:  %s",
	ConflictForce:             "설정 업데이트",
	ConflictCancel:            "현재 설정 유지",
	ConflictCancelled:         "현재 설정을 유지했습니다. 변경 사항 없음。",
	MessageBotOnly:            "앱 %s를 %s에 바인드。%s 앱 (bot) identity 준비 완료 — 사용자 요청을 계속할 수 있습니다。",
	MessageUserDefault:        "앱 %s를 %s에 바인드。다음으로, 이 %s 채팅에서 `lark-cli auth login --recommend`를 실행합니다。",
	SelectIdentity:            "AI는 어떻게 작동하나요?",
	IdentityBotOnly:           "Bot으로",
	IdentityUserDefault:       "사용자로",
	IdentityBotOnlyDesc:       "%s에서 자체 identity로 작동。그룹 채팅, 팀 알림, 공유 문서에 가장 적합。",
	IdentityUserDefaultDesc:   "%s에서 사용자를 대신하여 작동하여 문서, 메시지, 일정 등을 관리합니다。개인용으로만 사용。\n⚠️  이 봇을 다른 사람과 공유하거나 그룹 채팅에 추가하지 마세요。사용자의 개인 %s 데이터에 접근할 수 있습니다。",
	BindSuccessHeader:         "준비 완료! lark-cli가 %s에서 사용 가능합니다。",
	BindSuccessNotice:         "참고: 이것은 일회성 동기화입니다。향후 변경 사항을 재동기화하려면 `lark-cli config bind`를 실행하세요。",
	IdentityEscalationMessage: "bot-only에서 user-default로 전환 중 — AI가 모든 작업을 귀하의 Feishu 신원으로 수행하게 됩니다。⚠️ 이 봇을 다른 사람과 공유하거나 그룹 채팅에 추가하지 마세요。",
	IdentityEscalationHint:    "사용자가 전환을 확인한 경우 `--force`로 재실행: `lark-cli config bind --identity user-default --force`",
}
