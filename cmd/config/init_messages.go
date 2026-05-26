// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"github.com/charmbracelet/huh"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/i18n"
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

// promptLangSelection shows an interactive language picker and returns the chosen lang code.
// savedLang is used as the pre-selected default (from existing config).
func promptLangSelection(savedLang string) (string, error) {
	lang := i18n.NormalizeLang(savedLang)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Language / 语言").
				Options(
					huh.NewOption("中文", "zh"),
					huh.NewOption("English", "en"),
					huh.NewOption("日本語", "ja"),
					huh.NewOption("한국어", "ko"),
					huh.NewOption("Français", "fr"),
					huh.NewOption("Deutsch", "de"),
					huh.NewOption("Español", "es"),
					huh.NewOption("Italiano", "it"),
					huh.NewOption("Русский", "ru"),
					huh.NewOption("Português", "pt"),
					huh.NewOption("ไทย", "th"),
					huh.NewOption("Tiếng Việt", "vi"),
					huh.NewOption("Bahasa Indonesia", "id"),
					huh.NewOption("Bahasa Melayu", "ms"),
				).
				Value(&lang),
		),
	).WithTheme(cmdutil.ThemeFeishu())

	if err := form.Run(); err != nil {
		return "", err
	}
	return lang, nil
}
