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
	OpenURL        string
	WaitingAuth    string
	AuthSuccess    string
	LoginSuccess   string
	GrantedScopes  string
	BrowserOpened  string

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

	OpenURL:        "在浏览器中打开以下链接进行认证:\n\n",
	WaitingAuth:    "等待用户授权...",
	AuthSuccess:    "授权成功，正在获取用户信息...",
	LoginSuccess:   "登录成功! 用户: %s (%s)",
	GrantedScopes:  "  已授权 scopes: %s\n",
	BrowserOpened:  "已在浏览器中打开，如未跳转请手动复制上方链接",

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

	OpenURL:        "Open this URL in your browser to authenticate:\n\n",
	WaitingAuth:    "Waiting for user authorization...",
	AuthSuccess:    "Authorization successful, fetching user info...",
	LoginSuccess:   "Login successful! User: %s (%s)",
	GrantedScopes:  "  Granted scopes: %s\n",
	BrowserOpened:  "Opened in browser. If it didn't work, copy the URL above.",

	HintHeader:  "Please specify the scopes to authorize:\n",
	HintCommon1: "  --recommend                     authorize recommended scopes",
	HintCommon2: "  --domain all                    authorize all known domain scopes",
	HintCommon3: "  --domain calendar,task          authorize calendar and task scopes",
	HintCommon4: "  --domain calendar --recommend   authorize calendar recommended scopes",
	HintFooter:  "  lark-cli auth login --help",
}

// getLoginMsg returns the localized login message strings for the given language.
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
