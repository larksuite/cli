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
	AuthSuccess        string
	LoginSuccess       string
	ScopeMismatch      string
	ScopeHint          string
	ScopeHintShort     string
	RequestedScopes    string
	NewlyGrantedScopes string
	MissingScopes      string
	NoScopes           string

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
	AuthSuccess:        "授权成功，正在获取用户信息...",
	LoginSuccess:       "登录成功! 用户: %s (%s)",
	ScopeMismatch:      "授权完成，但以下请求 scopes 未被授予: %s",
	ScopeHint:          "以上未授权 scopes 是用户本次授权的最终结果，请不要持续重试。可执行 `lark-cli auth status` 查看当前账号实际已授权 scopes，执行 `lark-cli auth scopes` 查看应用已启用 scopes；如果仍需这些权限，请检查应用在飞书开发者后台是否已启用对应 scopes，并确认用户在授权页已同意相关权限。",
	ScopeHintShort:     "以上未授权 scopes 是用户本次授权的最终结果，请不要持续重试。可执行 `lark-cli auth status` 查看当前账号实际已授权 scopes，执行 `lark-cli auth scopes` 查看应用已启用 scopes。",
	RequestedScopes:    "  本次请求 scopes: %s\n",
	NewlyGrantedScopes: "  本次新增 scopes: %s\n",
	MissingScopes:      "  未授权 scopes: %s\n",
	NoScopes:           "（空）",

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
	AuthSuccess:        "Authorization successful, fetching user info...",
	LoginSuccess:       "Login successful! User: %s (%s)",
	ScopeMismatch:      "authorization completed, but these requested scopes were not granted: %s",
	ScopeHint:          "These missing scopes are the final outcome of this authorization and should not be retried continuously. Run `lark-cli auth status` to inspect the scopes currently granted to this account, and run `lark-cli auth scopes` to inspect the scopes enabled for the app. If these scopes are still required, verify the app configuration in the developer console and confirm that the user approved them on the authorization page.",
	ScopeHintShort:     "These missing scopes are the final outcome of this authorization and should not be retried continuously. Run `lark-cli auth status` to inspect the scopes currently granted to this account, and run `lark-cli auth scopes` to inspect the scopes enabled for the app.",
	RequestedScopes:    "  Requested scopes: %s\n",
	NewlyGrantedScopes: "  Newly granted scopes: %s\n",
	MissingScopes:      "  Missing scopes: %s\n",
	NoScopes:           "(none)",

	HintHeader:  "Please specify the scopes to authorize:\n",
	HintCommon1: "  --recommend                     authorize recommended scopes",
	HintCommon2: "  --domain all                    authorize all known domain scopes",
	HintCommon3: "  --domain calendar,task          authorize calendar and task scopes",
	HintCommon4: "  --domain calendar --recommend   authorize calendar recommended scopes",
	HintFooter:  "  lark-cli auth login --help",
}

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
	return []string{"base", "contact", "docs"}
}
