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
}

func getInitMsg(lang string) *initMsg {
	switch lang {
	case "en":
		return initMsgEn
	case "ja":
		return initMsgJa
	case "ko":
		return initMsgKo
	case "fr":
		return initMsgFr
	case "de":
		return initMsgDe
	case "es":
		return initMsgEs
	case "it":
		return initMsgIt
	case "ru":
		return initMsgRu
	case "pt":
		return initMsgPt
	case "ar":
		return initMsgAr
	case "hi":
		return initMsgHi
	case "tr":
		return initMsgTr
	case "pl":
		return initMsgPl
	case "nl":
		return initMsgNl
	case "sv":
		return initMsgSv
	case "th":
		return initMsgTh
	case "vi":
		return initMsgVi
	case "id":
		return initMsgId
	case "ms":
		return initMsgMs
	default:
		return initMsgZh
	}
}

var initMsgJa = &initMsg{
	SelectAction:         "操作を選択",
	CreateNewApp:         "ワンクリックでアプリを設定 (推奨)",
	ConfigExistingApp:    "アプリの認証情報を手動入力",
	Platform:             "プラットフォーム",
	SelectPlatform:       "プラットフォームを選択",
	Feishu:               "Feishu",
	ScanQRCode:           "\nFeishu/LarkでQRコードをスキャン:\n\n",
	ScanOrOpenLink:       "\nまたは以下のリンクをブラウザで開く:\n",
	WaitingForScan:       "設定結果を取得中...",
	OpenLinkNonTTY:       "\n以下のリンクを開いてアプリを設定:\n\n",
	WaitingForScanNonTTY: "アプリ設定を待機中...",
	DetectedLarkTenant:   "[lark-cli] Larkテナントを検出、エンドポイントを切り替え...",
	AppCreated:           "アプリ設定完了! App ID: %s",
	ConfigSaved:          "アプリ設定完了! App ID: %s",
}

var initMsgKo = &initMsg{
	SelectAction:         "작업 선택",
	CreateNewApp:         "원클릭 앱 설정 (권장)",
	ConfigExistingApp:    "앱 인증 정보 직접 입력",
	Platform:             "플랫폼",
	SelectPlatform:       "플랫폼 선택",
	Feishu:               "Feishu",
	ScanQRCode:           "\nFeishu/Lark으로 QR 코드 스캔:\n\n",
	ScanOrOpenLink:       "\n또는 아래 링크를 브라우저에서 열기:\n",
	WaitingForScan:       "설정 결과 가져오는 중...",
	OpenLinkNonTTY:       "\n아래 링크를 열어 앱 설정:\n\n",
	WaitingForScanNonTTY: "앱 설정 대기 중...",
	DetectedLarkTenant:   "[lark-cli] Lark 테넌트 감지, 엔드포인트 전환...",
	AppCreated:           "앱 설정 완료! App ID: %s",
	ConfigSaved:          "앱 설정 완료! App ID: %s",
}

// Placeholder variables for remaining languages - will be implemented in subsequent commits
var (
	initMsgFr = initMsgEn // French
	initMsgDe = initMsgEn // German
	initMsgEs = initMsgEn // Spanish
	initMsgIt = initMsgEn // Italian
	initMsgRu = initMsgEn // Russian
	initMsgPt = initMsgEn // Portuguese
	initMsgAr = initMsgEn // Arabic
	initMsgHi = initMsgEn // Hindi
	initMsgTr = initMsgEn // Turkish
	initMsgPl = initMsgEn // Polish
	initMsgNl = initMsgEn // Dutch
	initMsgSv = initMsgEn // Swedish
	initMsgTh = initMsgEn // Thai
	initMsgVi = initMsgEn // Vietnamese
	initMsgId = initMsgEn // Indonesian
	initMsgMs = initMsgEn // Malay
)

// TODO: Add proper translations for remaining 16 languages (fr, de, es, it, ru, pt, ar, hi, tr, pl, nl, sv, th, vi, id, ms)
// For Phase 1, we implement ja and ko as proof of concept
// The remaining languages will be added in subsequent commits

// promptLangSelection shows an interactive language picker and returns the chosen lang code.
// savedLang is used as the pre-selected default (from existing config).
func promptLangSelection(savedLang string) (string, error) {
	lang := i18n.NormalizeLang(savedLang)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Language / 语言 / 言語 / 언어").
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
					huh.NewOption("العربية", "ar"),
					huh.NewOption("हिन्दी", "hi"),
					huh.NewOption("Türkçe", "tr"),
					huh.NewOption("Polski", "pl"),
					huh.NewOption("Nederlands", "nl"),
					huh.NewOption("Svenska", "sv"),
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
