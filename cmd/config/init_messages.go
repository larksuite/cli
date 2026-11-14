// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"errors"

	"github.com/charmbracelet/huh"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/i18n"
	"github.com/larksuite/cli/internal/output"
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

// getInitMsg picks the zh/en TUI bundle. zh_cn — and values expressing no
// preference — render Chinese; every other supported locale renders English.
func getInitMsg(lang i18n.Lang) *initMsg {
	if lang.UsesEnglishUI() {
		return initMsgEn
	}
	return initMsgZh
}

// pickerCanExpress reports whether the 中文/English picker can represent l
// without losing it.
//
// Only a recognized locale outside zh_cn/en_us cannot be: the picker has no
// option for it, so a bare Enter would silently rewrite it. Those runs skip the
// picker and keep what --lang set.
//
// Everything else can be. Unset, mis-cased and unrecognized values express no
// usable preference — the same reading UsesEnglishUI applies to them — so there
// is nothing to destroy, and skipping the picker would strand the user with no
// interactive way to set one. A short code ("zh") maps onto the option that
// writes its canonical form, so picking it loses nothing either.
func pickerCanExpress(l i18n.Lang) bool {
	canonical, ok := i18n.Parse(string(l))
	if !ok {
		return true
	}
	return canonical == i18n.LangZhCN || canonical == i18n.LangEnUS
}

// promptLangSelection shows the 中文/English picker and returns the chosen
// locale. current pre-selects the option already in effect, so a user who
// already expressed a preference keeps it by pressing Enter.
func promptLangSelection(current i18n.Lang) (i18n.Lang, error) {
	lang := i18n.LangZhCN
	if current.UsesEnglishUI() {
		lang = i18n.LangEnUS
	}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[i18n.Lang]().
				Title("Language / 语言").
				Options(
					huh.NewOption("中文", i18n.LangZhCN),
					huh.NewOption("English", i18n.LangEnUS),
				).
				Value(&lang),
		),
	).WithTheme(cmdutil.ThemeFeishu())

	if err := form.Run(); err != nil {
		return "", err
	}
	return lang, nil
}

// langSelectionError maps a promptLangSelection failure to its exit surface:
// user abort exits bare with code 1; any other failure is internal.
func langSelectionError(err error) error {
	if errors.Is(err, huh.ErrUserAborted) {
		return output.ErrBare(1)
	}
	return errs.NewInternalError(errs.SubtypeUnknown, "language selection failed: %v", err).WithCause(err)
}
