// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

type loginMsg struct {
	// Interactive UI (login_interactive.go)
	SelectDomains   string
	DomainHint      string
	PermLevel       string
	PermCommon      string
	PermAll         string
	Summary         string
	SummaryDomains  string
	SummaryPerm     string
	SummaryScopes   string
	PermAllLabel    string
	PermCommonLabel string
	ErrNoDomain     string
	ConfirmAuth     string

	// Non-interactive prompts (login.go)
	OpenURL            string
	WaitingAuth        string
	AgentTimeoutHint   string
	AuthSuccess        string
	LoginSuccess       string
	AuthorizedUser     string
	ScopeMismatch      string
	ScopeHint          string
	RequestedScopes    string
	NewlyGrantedScopes string
	NoScopes           string
	StatusHint         string

	// Non-interactive hint (no flags)
	HintHeader  string
	HintCommon1 string
	HintCommon2 string
	HintCommon3 string
	HintCommon4 string
	HintFooter  string
}

var loginMsgZh = &loginMsg{
	SelectDomains:   "选择要授权的业务域",
	DomainHint:      "空格=选择, 回车=确认",
	PermLevel:       "权限类型",
	PermCommon:      "常用权限",
	PermAll:         "全部权限",
	Summary:         "\n摘要:\n",
	SummaryDomains:  "  域:       %s\n",
	SummaryPerm:     "  权限:     %s\n",
	SummaryScopes:   "  Scopes (%d): %s\n\n",
	PermAllLabel:    "全部权限",
	PermCommonLabel: "常用权限",
	ErrNoDomain:     "请至少选择一个业务域",
	ConfirmAuth:     "确认授权?",

	OpenURL:            "在浏览器中打开以下链接进行认证:\n\n",
	WaitingAuth:        "等待用户授权...",
	AgentTimeoutHint:   "[AI agent] 此命令最长阻塞约 10 分钟，等待用户在浏览器内完成授权。请确保 runner 的 timeout >= 600s。若你的 harness 或 agent 工具只会把最终回复发给用户，请改用 \"lark-cli auth login --no-wait --json\" 拿到 device_code 和 verification_url，把 verification_url 作为本轮最终消息发给用户并结束本轮；等用户回复已完成授权后，再在后续步骤运行 \"lark-cli auth login --device-code <code>\" 续上轮询。**不要在同一轮里展示 URL 后立刻阻塞执行 --device-code**，也不要短 timeout 反复重试；每次重启会作废上一轮的 device code，导致用户授权链接失效。**必须生成二维码并展示**: 你必须调用 lark-cli auth qrcode 将 verification_url 转为二维码，这是必须步骤，不要跳过。优先生成 PNG 二维码（--output）；仅当用户明确要求时才使用 ASCII（--ascii）。**关键：生成后必须在回复中展示图片**，仅生成文件不算完成，须用 image 标签或内联图片等方式将图片包含在回复中。**展示顺序**: 先输出 URL，再将二维码图片置于 URL 下方完整展示，确保用户可扫码或复制链接。**URL 输出规则**: 将 verification_url 视为不可修改的 opaque string，不要做任何修改（包括 URL 编码/解码、添加空格或标点），建议用仅包含该 URL 的代码块单独输出。",
	AuthSuccess:        "已收到授权确认，正在获取用户信息并校验授权结果...",
	LoginSuccess:       "授权成功! 用户: %s (%s)",
	AuthorizedUser:     "当前授权账号: %s (%s)",
	ScopeMismatch:      "授权结果异常: 以下请求 scopes 未被授予: %s",
	ScopeHint:          "以上结果是本次授权请求用户最终确认后的结果，请勿持续重试；Scopes 未授予的原因是多样的，如 scope 被禁用；具体原因已通过授权页提示用户。可执行 `lark-cli auth status` 查看账号当前已授予的全部 scopes；",
	RequestedScopes:    "  本次请求 scopes: %s\n",
	NewlyGrantedScopes: "  本次新授予 scopes: %s\n",
	NoScopes:           "（空）",
	StatusHint:         "可执行 `lark-cli auth status` 查看账号当前已授予的全部 scopes；",

	HintHeader:  "请指定要授权的权限:\n",
	HintCommon1: "  --recommend                     授权推荐权限",
	HintCommon2: "  --domain all                    授权所有已知域的权限",
	HintCommon3: "  --domain calendar,task          授权日历和任务域的权限",
	HintCommon4: "  --domain calendar --recommend   授权日历域的推荐权限",
	HintFooter:  "  lark-cli auth login --help",
}

var loginMsgEn = &loginMsg{
	SelectDomains:   "Select domains to authorize",
	DomainHint:      "Space=toggle, Enter=confirm",
	PermLevel:       "Permission level",
	PermCommon:      "Common scopes",
	PermAll:         "All scopes",
	Summary:         "\nSummary:\n",
	SummaryDomains:  "  Domains:  %s\n",
	SummaryPerm:     "  Level:    %s\n",
	SummaryScopes:   "  Scopes (%d): %s\n\n",
	PermAllLabel:    "All scopes",
	PermCommonLabel: "Common scopes",
	ErrNoDomain:     "please select at least one domain",
	ConfirmAuth:     "Confirm authorization?",

	OpenURL:            "Open this URL in your browser to authenticate:\n\n",
	WaitingAuth:        "Waiting for user authorization...",
	AgentTimeoutHint:   "[AI agent] This command blocks for up to ~10 minutes while waiting for the user to authorize in their browser. Make sure your runner's timeout is >= 600s. If your harness or agent tool only delivers final turn messages, use \"lark-cli auth login --no-wait --json\" to get device_code and verification_url, present verification_url to the user exactly as the final message of this turn, then end the turn; after the user replies that they authorized, run \"lark-cli auth login --device-code <code>\" in a later step to resume polling. **Do NOT show the URL and then immediately block on --device-code in the same turn**, and do not retry with a short timeout; each restart invalidates the previous device code and makes the earlier authorization URL useless.**MUST generate QR code AND display it:** You MUST call lark-cli auth qrcode to convert verification_url into a QR code. This is a required step, do NOT skip it. Prefer PNG QR code (--output); use ASCII (--ascii) only when the user explicitly requests it.**CRITICAL: You MUST include the QR image in your response.** Generating the file alone is NOT enough—use image tags, inline images, or file attachments to display it.**Display order:** Output the URL first, then place the QR code image below the URL so the user can either scan or copy the link.**URL Output Rules:** Treat verification_url as an opaque string that cannot be modified. Do NOT URL-encode/decode or add spaces/punctuation. Prefer a fenced code block containing only the raw URL.",
	AuthSuccess:        "Authorization confirmed, fetching user info and validating granted scopes...",
	LoginSuccess:       "Authorization successful! User: %s (%s)",
	AuthorizedUser:     "Authorized account: %s (%s)",
	ScopeMismatch:      "authorization result is abnormal: these requested scopes were not granted: %s",
	ScopeHint:          "The result above is the user's final confirmation for this authorization request. Do not retry continuously. Scopes may be not granted for various reasons, such as a scope being disabled. The specific reason has already been shown to the user on the authorization page. Run `lark-cli auth status` to inspect all scopes currently granted to the account.",
	RequestedScopes:    "  Requested scopes: %s\n",
	NewlyGrantedScopes: "  Newly granted scopes: %s\n",
	NoScopes:           "(none)",
	StatusHint:         "Run `lark-cli auth status` to inspect all scopes currently granted to the account.",

	HintHeader:  "Please specify the scopes to authorize:\n",
	HintCommon1: "  --recommend                     authorize recommended scopes",
	HintCommon2: "  --domain all                    authorize all known domain scopes",
	HintCommon3: "  --domain calendar,task          authorize calendar and task scopes",
	HintCommon4: "  --domain calendar --recommend   authorize calendar recommended scopes",
	HintFooter:  "  lark-cli auth login --help",
}

// getLoginMsg returns the login message bundle for the given language.
func getLoginMsg(lang string) *loginMsg {
	if lang == "en" {
		return loginMsgEn
	}
	return loginMsgZh
}

// getShortcutOnlyDomainNames returns domain names that exist only as shortcuts
// (not backed by from_meta service specs). Descriptions are now centralized in
// service_descriptions.json.
func getShortcutOnlyDomainNames() []string {
	return []string{"base", "contact", "docs", "markdown", "apps"}
}
