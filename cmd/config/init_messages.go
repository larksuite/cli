// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"github.com/charmbracelet/huh"

	"github.com/larksuite/cli/internal/cmdutil"
)

type initMsg struct {
	SelectAction       string
	CreateNewApp       string
	ConfigExistingApp  string
	Platform           string
	SelectPlatform     string
	Feishu             string
	ScanOrOpenLink     string
	WaitingForScan     string
	DetectedLarkTenant string
	AppCreated         string
	ConfigSaved        string
	BrowserOpened      string
}

var initMsgZh = &initMsg{
	SelectAction:       "选择操作",
	CreateNewApp:       "一键配置应用 (推荐) ",
	ConfigExistingApp:  "手动输入应用凭证",
	Platform:           "平台",
	SelectPlatform:     "选择平台",
	Feishu:             "飞书",
	ScanOrOpenLink:     "\n打开以下链接配置应用:\n\n",
	WaitingForScan:     "等待配置应用...",
	DetectedLarkTenant: "[lark-cli] 检测到 Lark 租户，切换端点重试...",
	AppCreated:         "应用配置成功! App ID: %s",
	ConfigSaved:        "应用配置成功! App ID: %s",
	BrowserOpened:      "已在浏览器中打开，如未跳转请手动复制上方链接",
}

var initMsgEn = &initMsg{
	SelectAction:       "Select action",
	CreateNewApp:       "Set up your app with one click (Recommended)",
	ConfigExistingApp:  "Enter app credentials yourself",
	Platform:           "Platform",
	SelectPlatform:     "Select platform",
	Feishu:             "Feishu",
	ScanOrOpenLink:     "\nOpen the link below to configure app:\n\n",
	WaitingForScan:     "Waiting for app configuration...",
	DetectedLarkTenant: "[lark-cli] Detected Lark tenant, switching endpoint...",
	AppCreated:         "App configured! App ID: %s",
	ConfigSaved:        "App configured! App ID: %s",
	BrowserOpened:      "Opened in browser. If it didn't work, copy the URL above.",
}

// getInitMsg returns the localized init message strings for the given language.
func getInitMsg(lang string) *initMsg {
	if lang == "en" {
		return initMsgEn
	}
	return initMsgZh
}

// promptLangSelection shows an interactive language picker and returns the chosen lang code.
// savedLang is used as the pre-selected default (from existing config).
func promptLangSelection(savedLang string) (string, error) {
	lang := savedLang
	if lang != "en" {
		lang = "zh"
	}
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
