// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"github.com/charmbracelet/huh"

	"github.com/larksuite/cli/internal/cmdutil"
)

type initMsg struct {
	SelectAction      string
	CreateNewApp      string
	ConfigExistingApp string
	Platform          string
	SelectPlatform    string
	Feishu            string
	// TTY (interactive) variants
	ScanQRCode     string // header shown above QR code
	ScanOrOpenLink string // post-QR alt link prompt ("or open...")
	WaitingForScan string // active polling indicator
	// Non-TTY (AI / non-interactive) variants — preserve original copy
	OpenLinkNonTTY       string // primary link prompt
	WaitingForScanNonTTY string // passive waiting indicator
	DetectedLarkTenant   string
	AppCreated           string
	ConfigSaved          string

	// LangPreferenceSet is printed to stderr after a successful init when the
	// user explicitly passed --lang. Format: language code.
	LangPreferenceSet string
}

var initMsgZh = &initMsg{
	SelectAction:         "选择操作",
	CreateNewApp:         "一键配置应用 (推荐) ",
	ConfigExistingApp:    "手动输入应用凭证",
	Platform:             "平台",
	SelectPlatform:       "选择平台",
	Feishu:               "飞书",
	ScanQRCode:           "\n使用飞书 / Lark 扫码配置应用：\n\n",
	ScanOrOpenLink:       "\n或打开以下链接完成配置：\n",
	WaitingForScan:       "正在获取你的应用配置结果...",
	OpenLinkNonTTY:       "\n打开以下链接配置应用:\n\n",
	WaitingForScanNonTTY: "等待配置应用...",
	DetectedLarkTenant:   "[lark-cli] 检测到 Lark 租户，切换端点重试...",
	AppCreated:           "应用配置成功! App ID: %s",
	ConfigSaved:          "应用配置成功! App ID: %s",
	LangPreferenceSet:    "语言偏好已设置：%s",
}

var initMsgEn = &initMsg{
	SelectAction:         "Select action",
	CreateNewApp:         "Set up your app with one click (Recommended)",
	ConfigExistingApp:    "Enter app credentials yourself",
	Platform:             "Platform",
	SelectPlatform:       "Select platform",
	Feishu:               "Feishu",
	ScanQRCode:           "\nScan the QR code with Feishu/Lark:\n\n",
	ScanOrOpenLink:       "\nOr open the link below in your browser:\n",
	WaitingForScan:       "Fetching configuration results...",
	OpenLinkNonTTY:       "\nOpen the link below to configure app:\n\n",
	WaitingForScanNonTTY: "Waiting for app configuration...",
	DetectedLarkTenant:   "[lark-cli] Detected Lark tenant, switching endpoint...",
	AppCreated:           "App configured! App ID: %s",
	ConfigSaved:          "App configured! App ID: %s",
	LangPreferenceSet:    "Language preference set to: %s",
}

// getInitMsg returns the initMsg for the TUI display language. Same
// contract as getBindMsg: input is opts.UILang ("zh" or "en"); anything
// else collapses to zh.
func getInitMsg(lang string) *initMsg {
	if lang == "en" {
		return initMsgEn
	}
	return initMsgZh
}

// promptLangSelection shows an interactive 2-option language picker
// (中文 / English) and returns the chosen lang code ("zh" or "en"). The
// picker is the sole writer of opts.UILang. Callers also propagate the
// result into opts.Lang so the persisted preference matches what the
// user just picked.
func promptLangSelection() (string, error) {
	lang := "zh"
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Language / 语言").
				Options(
					huh.NewOption("中文", "zh"),
					huh.NewOption("English", "en"),
				).
				Value(&lang),
		),
	).WithTheme(cmdutil.ThemeFeishu())

	if err := form.Run(); err != nil {
		return "", err
	}
	return lang, nil
}
