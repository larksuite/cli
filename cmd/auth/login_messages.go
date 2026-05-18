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
	AgentNoJSONHint    string
	AuthSuccess        string
	LoginSuccess       string
	AuthorizedUser     string
	ScopeMismatch      string
	ScopeHint          string
	RequestedScopes    string
	NewlyGrantedScopes string
	NoScopes           string
	StatusHint         string
	// QR code messages
	ScanQRCode           string // header shown above QR code (TTY)
	ScanOrOpenLink       string // post-QR alt link prompt ("or open...") (TTY)
	OpenLinkNonTTY       string // primary link prompt (non-TTY/AI)
	WaitingForScanNonTTY string // passive waiting indicator (non-TTY/AI)
	QRCodeDisplayHint    string // hint for displaying qr_code_base64

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

	OpenURL:              "在浏览器中打开以下链接进行认证:\n\n",
	WaitingAuth:          "等待用户授权...",
	AgentTimeoutHint:     "[AI agent] 此命令最长阻塞约 10 分钟，等待用户在浏览器内完成授权。请确保 runner 的 timeout >= 600s。若你的 harness 只会把最终回复发给用户，请改用 `lark-cli auth login --no-wait --json` 拿到 device_code、qr_code_base64 和 verification_url，把二维码图片（解码 qr_code_base64）和 verification_url 作为本轮最终消息发给用户并结束本轮；等用户回复已完成授权后，再在后续步骤运行 `lark-cli auth login --device-code <code>` 续上轮询。**不要在同一轮里展示 URL 后立刻阻塞执行 --device-code**，也不要短 timeout 反复重试；每次重启会作废上一轮的 device code，导致用户授权链接失效。若你的 agent 无法显示图片，只展示 verification_url 即可。向用户展示授权链接时，必须逐字原样转发 CLI 返回的 URL，把它视为不可修改的 opaque string；不要做 URL 编码或解码，不要补 `%20`、空格或标点，不要改写成 Markdown 链接，建议用只包含该 URL 的代码块单独输出。",
	AgentNoJSONHint:      "[AI agent] 如需获取二维码图片，请添加 --json 或使用 `lark-cli auth login --no-wait --json` 获取 qr_code_base64 字段。",
	AuthSuccess:          "已收到授权确认，正在获取用户信息并校验授权结果...",
	LoginSuccess:         "授权成功! 用户: %s (%s)",
	AuthorizedUser:       "当前授权账号: %s (%s)",
	ScopeMismatch:        "授权结果异常: 以下请求 scopes 未被授予: %s",
	ScopeHint:            "以上结果是本次授权请求用户最终确认后的结果，请勿持续重试；Scopes 未授予的原因是多样的，如 scope 被禁用；具体原因已通过授权页提示用户。可执行 `lark-cli auth status` 查看账号当前已授予的全部 scopes；",
	RequestedScopes:      "  本次请求 scopes: %s\n",
	NewlyGrantedScopes:   "  本次新授予 scopes: %s\n",
	NoScopes:             "（空）",
	StatusHint:           "可执行 `lark-cli auth status` 查看账号当前已授予的全部 scopes；",
	ScanQRCode:           "\n使用飞书 / Lark 扫码授权：\n\n",
	ScanOrOpenLink:       "\n或打开以下链接完成授权：\n",
	OpenLinkNonTTY:       "\n打开以下链接授权:\n\n",
	WaitingForScanNonTTY: "等待授权...",
	QRCodeDisplayHint:    "[AI Agent] 请尽可能展示 qr_code_base64 图片！qr_code_base64 包含 base64 编码的 PNG 图片，解码后以图片形式显示（256x256 像素）便于用户扫码授权。如果 agent 无法显示图片，可以展示 qr_code_ascii（ASCII 二维码），并且展示 verification_url 让用户可以手动打开。",

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

	OpenURL:              "Open this URL in your browser to authenticate:\n\n",
	WaitingAuth:          "Waiting for user authorization...",
	AgentTimeoutHint:     "[AI agent] This command blocks for up to ~10 minutes while waiting for the user to authorize in their browser. Make sure your runner's timeout is >= 600s. If your harness only delivers final turn messages, use `lark-cli auth login --no-wait --json` to get device_code, qr_code_base64 and verification_url, present the QR code image (decode qr_code_base64) and verification_url to the user as the final message of this turn, then end the turn; after the user replies that they authorized, run `lark-cli auth login --device-code <code>` in a later step to resume polling. **Do NOT show the URL and then immediately block on --device-code in the same turn**, and do not retry with a short timeout; each restart invalidates the previous device code and makes the earlier authorization URL useless. If your agent cannot display images, show verification_url only. When showing the authorization URL to the user, copy the CLI-returned URL exactly as-is and treat it as an opaque string. Do not URL-encode or decode it, do not add `%20`, spaces, or punctuation, do not rewrite it as Markdown link text, and prefer a fenced code block containing only the raw URL.",
	AgentNoJSONHint:      "[AI agent] To get QR code image, add --json or use `lark-cli auth login --no-wait --json` to get the qr_code_base64 field.",
	AuthSuccess:          "Authorization confirmed, fetching user info and validating granted scopes...",
	LoginSuccess:         "Authorization successful! User: %s (%s)",
	AuthorizedUser:       "Authorized account: %s (%s)",
	ScopeMismatch:        "authorization result is abnormal: these requested scopes were not granted: %s",
	ScopeHint:            "The result above is the user's final confirmation for this authorization request. Do not retry continuously. Scopes may be not granted for various reasons, such as a scope being disabled. The specific reason has already been shown to the user on the authorization page. Run `lark-cli auth status` to inspect all scopes currently granted to the account.",
	RequestedScopes:      "  Requested scopes: %s\n",
	NewlyGrantedScopes:   "  Newly granted scopes: %s\n",
	NoScopes:             "(none)",
	StatusHint:           "Run `lark-cli auth status` to inspect all scopes currently granted to the account.",
	ScanQRCode:           "\nScan the QR code with Feishu/Lark to authorize:\n\n",
	ScanOrOpenLink:       "\nOr open the link below to complete authorization:\n",
	OpenLinkNonTTY:       "\nOpen the link below to authorize:\n\n",
	WaitingForScanNonTTY: "Waiting for authorization...",
	QRCodeDisplayHint:    "[AI Agent] Please display qr_code_base64 image if at all possible! qr_code_base64 contains a base64-encoded PNG image. Decode it and display as an image (256x256 pixels) for easy QR code scanning. If your agent cannot display images, you can show qr_code_ascii (ASCII QR code) and display verification_url for users to open manually.",

	HintHeader:  "Please specify the scopes to authorize:\n",
	HintCommon1: "  --recommend                     authorize recommended scopes",
	HintCommon2: "  --domain all                    authorize all known domain scopes",
	HintCommon3: "  --domain calendar,task          authorize calendar and task scopes",
	HintCommon4: "  --domain calendar --recommend   authorize calendar recommended scopes",
	HintFooter:  "  lark-cli auth login --help",
}

// getLoginMsg returns the login message bundle for the specified language.
// Supports "zh" for Chinese and "en" for English. Defaults to Chinese.
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
	return []string{"base", "contact", "docs", "markdown"}
}
