// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"github.com/larksuite/cli/internal/i18n"
	"github.com/larksuite/cli/internal/recovery"
)

type loginMsg struct {
	// Non-interactive prompts (login.go)
	OpenURL          string
	WaitingAuth      string
	AgentTimeoutHint func(recovery.RenderContext) string
	AuthSuccess      string
	LoginSuccess     string
	ScopeMismatch    string
	ScopeHint        string
	GrantedScopes    string
	NotGrantedScopes string
	AuthDetails      string
	ScopeSeparator   string
	NoScopes         string
}

var loginMsgZh = &loginMsg{
	OpenURL:          "在浏览器中打开以下链接进行认证:\n\n",
	WaitingAuth:      "等待用户授权...",
	AgentTimeoutHint: renderAgentTimeoutHintZh,
	AuthSuccess:      "已收到授权确认，正在获取用户信息并校验授权结果...",
	LoginSuccess:     "登录成功! 用户: %s (%s)",
	ScopeMismatch:    "授权结果异常: 以下请求 scopes 未被授予: %s",
	ScopeHint:        "以上结果是本次授权请求用户最终确认后的结果，请勿持续重试；Scopes 未授予的原因是多样的，如 scope 被禁用；具体原因已通过授权页提示用户。可执行 `lark-cli auth status` 查看账号当前已授予的全部 scopes；",
	GrantedScopes:    "本次已成功授权：",
	NotGrantedScopes: "以下是本次未授予的权限：",
	AuthDetails:      "本次授权结果详情：",
	ScopeSeparator:   "、",
	NoScopes:         "（空）",
}

var loginMsgEn = &loginMsg{
	OpenURL:          "Open this URL in your browser to authenticate:\n\n",
	WaitingAuth:      "Waiting for user authorization...",
	AgentTimeoutHint: renderAgentTimeoutHintEn,
	AuthSuccess:      "Authorization confirmed, fetching user info and validating granted scopes...",
	LoginSuccess:     "Authorization successful! User: %s (%s)",
	ScopeMismatch:    "authorization result is abnormal: these requested scopes were not granted: %s",
	ScopeHint:        "The result above is the user's final confirmation for this authorization request. Do not retry continuously. Scopes may be not granted for various reasons, such as a scope being disabled. The specific reason has already been shown to the user on the authorization page. Run `lark-cli auth status` to inspect all scopes currently granted to the account.",
	GrantedScopes:    "- Successfully authorized in this request:",
	NotGrantedScopes: "- Scopes not granted in this request:",
	AuthDetails:      "- Authorization details:",
	ScopeSeparator:   ", ",
	NoScopes:         "(none)",
}

func renderAgentTimeoutHintZh(context recovery.RenderContext) string {
	profileInstruction := ""
	if context.Profile != "" {
		profileInstruction = "，并使用 " + context.InlineAuthLoginCommand("") + " 保留显式 --profile"
	}
	return "[AI agent] 此命令最长阻塞约 10 分钟，等待用户在浏览器内完成授权。请确保 runner 的 timeout >= 600s。若你的 harness 或 agent 工具只会把最终回复发给用户，请沿用原请求的 --scope、--domain 或 --recommend 选择以及任何 --exclude 值" +
		profileInstruction +
		"，并附加 --no-wait --json，拿到 device_code 和 verification_url；把 verification_url 作为本轮最终消息发给用户并结束本轮；等用户回复已完成授权后，再在后续步骤运行 \"" +
		context.AuthLoginCommand("--device-code <code>") +
		"\" 续上轮询。**不要在同一轮里展示 URL 后立刻阻塞执行 --device-code**，也不要短 timeout 反复重试；每次重启会作废上一轮的 device code，导致用户授权链接失效。**必须生成二维码并展示**: 你必须调用 lark-cli auth qrcode 将 verification_url 转为二维码，这是必须步骤，不要跳过。优先生成 PNG 二维码（--output）；仅当用户明确要求时才使用 ASCII（--ascii）。**关键：生成后必须在回复中展示图片**，仅生成文件不算完成，须用 image 标签或内联图片等方式将图片包含在回复中。**展示顺序**: 先输出 URL，再将二维码图片置于 URL 下方完整展示。**URL 输出规则**: 将 verification_url 视为不可修改的 opaque string，不要做任何修改（包括 URL 编码/解码、添加空格或标点）。"
}

func renderAgentTimeoutHintEn(context recovery.RenderContext) string {
	return "[AI agent] This command blocks for up to ~10 minutes while waiting for the user to authorize in their browser. Make sure your runner's timeout is >= 600s. If your harness or agent tool only delivers final turn messages, rerun " +
		context.InlineAuthLoginCommand("") +
		" with the same --scope, --domain, or --recommend selection and any --exclude values, plus --no-wait --json, to get device_code and verification_url; present verification_url to the user exactly as the final message of this turn, then end the turn; after the user replies that they authorized, run \"" +
		context.AuthLoginCommand("--device-code <code>") +
		"\" in a later step to resume polling. **Do NOT show the URL and then immediately block on --device-code in the same turn**, and do not retry with a short timeout; each restart invalidates the previous device code and makes the earlier authorization URL useless.**MUST generate QR code AND display it:** You MUST call lark-cli auth qrcode to convert verification_url into a QR code. This is a required step, do NOT skip it. Prefer PNG QR code (--output); use ASCII (--ascii) only when the user explicitly requests it.**CRITICAL: You MUST include the QR image in your response.** Generating the file alone is NOT enough—use image tags, inline images, or file attachments to display it.**Display order:** Output the URL first, then place the QR code image below the URL.**URL Output Rules:** Treat verification_url as an opaque string that cannot be modified. Do NOT URL-encode/decode or add spaces/punctuation."
}

func noWaitAgentHint(context recovery.RenderContext) string {
	return "**MUST generate QR code AND display it:** You MUST call lark-cli auth qrcode to convert verification_url into a QR code. This is a required step, do NOT skip it. Prefer PNG QR code (--output); use ASCII (--ascii) only when the user explicitly requests it." +
		"**CRITICAL: You MUST include the QR image in your response.** Generating the file alone is NOT enough—use image tags, inline images, or file attachments to display it." +
		"**Display order:** Output the URL first, then place the QR code image below the URL." +
		"**URL Output Rules:** Treat verification_url as an opaque string that cannot be modified. Do NOT URL-encode/decode or add spaces/punctuation." +
		"For agent harnesses that only deliver final turn messages, make the QR code image (or URL) the final message of the turn and return control to the user; do not block on --device-code in the same turn. **Before ending the turn, tell the user to come back and notify you after completing authorization.**" +
		"**After the user confirms authorization:** YOU must execute " + context.InlineAuthLoginCommand("--device-code <device_code>") + " yourself." +
		"**Do NOT cache verification_url or device_code for future use.** When authorization is needed again, rerun " + context.InlineAuthLoginCommand("") + " with the same `--scope`, `--domain`, or `--recommend` selection and any `--exclude` values, plus `--no-wait --json` to get a fresh link."
}

// getLoginMsg returns the login message bundle for the given language.
func getLoginMsg(lang i18n.Lang) *loginMsg {
	if lang.IsEnglish() {
		return loginMsgEn
	}
	return loginMsgZh
}
