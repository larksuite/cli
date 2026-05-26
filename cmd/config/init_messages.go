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

var initMsgFr = &initMsg{
	SelectAction:         "Sélectionner l'action",
	CreateNewApp:         "Configurer l'app en un clic (Recommandé)",
	ConfigExistingApp:    "Saisir les informations d'identification manuellement",
	Platform:             "Plateforme",
	SelectPlatform:       "Sélectionner la plateforme",
	Feishu:               "Feishu",
	ScanQRCode:           "\nScannez le QR code avec Feishu/Lark:\n\n",
	ScanOrOpenLink:       "\nOu ouvrez le lien ci-dessous dans votre navigateur:\n",
	WaitingForScan:       "Récupération des résultats de configuration...",
	OpenLinkNonTTY:       "\nOuvrez le lien ci-dessous pour configurer l'app:\n\n",
	WaitingForScanNonTTY: "En attente de la configuration de l'app...",
	DetectedLarkTenant:   "[lark-cli] Tenant Lark détecté, changement d'endpoint...",
	AppCreated:           "App configurée! App ID: %s",
	ConfigSaved:          "App configurée! App ID: %s",
}

var initMsgDe = &initMsg{
	SelectAction:         "Aktion auswählen",
	CreateNewApp:         "App mit einem Klick einrichten (Empfohlen)",
	ConfigExistingApp:    "App-Anmeldeinformationen manuell eingeben",
	Platform:             "Plattform",
	SelectPlatform:       "Plattform auswählen",
	Feishu:               "Feishu",
	ScanQRCode:           "\nScannen Sie den QR-Code mit Feishu/Lark:\n\n",
	ScanOrOpenLink:       "\nOder öffnen Sie den folgenden Link in Ihrem Browser:\n",
	WaitingForScan:       "Konfigurationsergebnisse werden abgerufen...",
	OpenLinkNonTTY:       "\nÖffnen Sie den folgenden Link, um die App zu konfigurieren:\n\n",
	WaitingForScanNonTTY: "Warte auf App-Konfiguration...",
	DetectedLarkTenant:   "[lark-cli] Lark-Mandant erkannt, Endpunkt wird gewechselt...",
	AppCreated:           "App konfiguriert! App ID: %s",
	ConfigSaved:          "App konfiguriert! App ID: %s",
}

var initMsgEs = &initMsg{
	SelectAction:         "Seleccionar acción",
	CreateNewApp:         "Configurar app con un clic (Recomendado)",
	ConfigExistingApp:    "Ingresar credenciales de app manualmente",
	Platform:             "Plataforma",
	SelectPlatform:       "Seleccionar plataforma",
	Feishu:               "Feishu",
	ScanQRCode:           "\nEscanee el código QR con Feishu/Lark:\n\n",
	ScanOrOpenLink:       "\nO abra el siguiente enlace en su navegador:\n",
	WaitingForScan:       "Obteniendo resultados de configuración...",
	OpenLinkNonTTY:       "\nAbra el siguiente enlace para configurar la app:\n\n",
	WaitingForScanNonTTY: "Esperando configuración de la app...",
	DetectedLarkTenant:   "[lark-cli] Detectado inquilino Lark, cambiando endpoint...",
	AppCreated:           "¡App configurada! App ID: %s",
	ConfigSaved:          "¡App configurada! App ID: %s",
}

var initMsgIt = &initMsg{
	SelectAction:         "Seleziona azione",
	CreateNewApp:         "Configura app con un click (Consigliato)",
	ConfigExistingApp:    "Inserisci le credenziali dell'app manualmente",
	Platform:             "Piattaforma",
	SelectPlatform:       "Seleziona piattaforma",
	Feishu:               "Feishu",
	ScanQRCode:           "\nScansiona il codice QR con Feishu/Lark:\n\n",
	ScanOrOpenLink:       "\nOppure apri il link qui sotto nel tuo browser:\n",
	WaitingForScan:       "Recupero dei risultati di configurazione...",
	OpenLinkNonTTY:       "\nApri il link qui sotto per configurare l'app:\n\n",
	WaitingForScanNonTTY: "In attesa della configurazione dell'app...",
	DetectedLarkTenant:   "[lark-cli] Rilevato tenant Lark, cambio endpoint...",
	AppCreated:           "App configurata! App ID: %s",
	ConfigSaved:          "App configurata! App ID: %s",
}

var initMsgRu = &initMsg{
	SelectAction:         "Выберите действие",
	CreateNewApp:         "Настроить приложение одним кликом (Рекомендуется)",
	ConfigExistingApp:    "Ввести учетные данные приложения вручную",
	Platform:             "Платформа",
	SelectPlatform:       "Выберите платформу",
	Feishu:               "Feishu",
	ScanQRCode:           "\nОтсканируйте QR-код с помощью Feishu/Lark:\n\n",
	ScanOrOpenLink:       "\nИли откройте ссылку ниже в браузере:\n",
	WaitingForScan:       "Получение результатов настройки...",
	OpenLinkNonTTY:       "\nОткройте ссылку ниже, чтобы настроить приложение:\n\n",
	WaitingForScanNonTTY: "Ожидание настройки приложения...",
	DetectedLarkTenant:   "[lark-cli] Обнаружен арендатор Lark, переключение endpoint...",
	AppCreated:           "Приложение настроено! App ID: %s",
	ConfigSaved:          "Приложение настроено! App ID: %s",
}

var initMsgPt = &initMsg{
	SelectAction:         "Selecionar ação",
	CreateNewApp:         "Configurar app com um clique (Recomendado)",
	ConfigExistingApp:    "Inserir credenciais do app manualmente",
	Platform:             "Plataforma",
	SelectPlatform:       "Selecionar plataforma",
	Feishu:               "Feishu",
	ScanQRCode:           "\nEscaneie o código QR com Feishu/Lark:\n\n",
	ScanOrOpenLink:       "\nOu abra o link abaixo no seu navegador:\n",
	WaitingForScan:       "Obtendo resultados da configuração...",
	OpenLinkNonTTY:       "\nAbra o link abaixo para configurar o app:\n\n",
	WaitingForScanNonTTY: "Aguardando configuração do app...",
	DetectedLarkTenant:   "[lark-cli] Tenant Lark detectado, mudando endpoint...",
	AppCreated:           "App configurado! App ID: %s",
	ConfigSaved:          "App configurado! App ID: %s",
}

var initMsgTh = &initMsg{
	SelectAction:         "เลือกการดำเนินการ",
	CreateNewApp:         "ตั้งค่าแอปด้วยคลิกเดียว (แนะนำ)",
	ConfigExistingApp:    "ป้อนข้อมูลประจำตัวแอปด้วยตนเอง",
	Platform:             "แพลตฟอร์ม",
	SelectPlatform:       "เลือกแพลตฟอร์ม",
	Feishu:               "Feishu",
	ScanQRCode:           "\nสแกนรหัส QR ด้วย Feishu/Lark:\n\n",
	ScanOrOpenLink:       "\nหรือเปิดลิงก์ด้านล่างในเบราว์เซอร์ของคุณ:\n",
	WaitingForScan:       "กำลังดึงผลลัพธ์การกำหนดค่า...",
	OpenLinkNonTTY:       "\nเปิดลิงก์ด้านล่างเพื่อกำหนดค่าแอป:\n\n",
	WaitingForScanNonTTY: "กำลังรอการกำหนดค่าแอป...",
	DetectedLarkTenant:   "[lark-cli] ตรวจพบผู้เช่า Lark กำลังสลับ endpoint...",
	AppCreated:           "กำหนดค่าแอปสำเร็จ! App ID: %s",
	ConfigSaved:          "กำหนดค่าแอปสำเร็จ! App ID: %s",
}

var initMsgVi = &initMsg{
	SelectAction:         "Chọn hành động",
	CreateNewApp:         "Thiết lập ứng dụng bằng một cú nhấp chuột (Khuyến nghị)",
	ConfigExistingApp:    "Nhập thông tin xác thực ứng dụng thủ công",
	Platform:             "Nền tảng",
	SelectPlatform:       "Chọn nền tảng",
	Feishu:               "Feishu",
	ScanQRCode:           "\nQuét mã QR bằng Feishu/Lark:\n\n",
	ScanOrOpenLink:       "\nHoặc mở liên kết bên dưới trong trình duyệt của bạn:\n",
	WaitingForScan:       "Đang lấy kết quả cấu hình...",
	OpenLinkNonTTY:       "\nMở liên kết bên dưới để cấu hình ứng dụng:\n\n",
	WaitingForScanNonTTY: "Đang chờ cấu hình ứng dụng...",
	DetectedLarkTenant:   "[lark-cli] Đã phát hiện người thuê Lark, chuyển đổi endpoint...",
	AppCreated:           "Ứng dụng đã được cấu hình! App ID: %s",
	ConfigSaved:          "Ứng dụng đã được cấu hình! App ID: %s",
}

var initMsgId = &initMsg{
	SelectAction:         "Pilih tindakan",
	CreateNewApp:         "Siapkan aplikasi dengan satu klik (Direkomendasikan)",
	ConfigExistingApp:    "Masukkan kredensial aplikasi secara manual",
	Platform:             "Platform",
	SelectPlatform:       "Pilih platform",
	Feishu:               "Feishu",
	ScanQRCode:           "\nPindai kode QR dengan Feishu/Lark:\n\n",
	ScanOrOpenLink:       "\nAtau buka tautan di bawah ini di browser Anda:\n",
	WaitingForScan:       "Mengambil hasil konfigurasi...",
	OpenLinkNonTTY:       "\nBuka tautan di bawah ini untuk mengkonfigurasi aplikasi:\n\n",
	WaitingForScanNonTTY: "Menunggu konfigurasi aplikasi...",
	DetectedLarkTenant:   "[lark-cli] Tenant Lark terdeteksi, mengalihkan endpoint...",
	AppCreated:           "Aplikasi dikonfigurasi! App ID: %s",
	ConfigSaved:          "Aplikasi dikonfigurasi! App ID: %s",
}

var initMsgMs = &initMsg{
	SelectAction:         "Pilih tindakan",
	CreateNewApp:         "Sediakan aplikasi dengan satu klik (Disyorkan)",
	ConfigExistingApp:    "Masukkan kelayakan aplikasi secara manual",
	Platform:             "Platform",
	SelectPlatform:       "Pilih platform",
	Feishu:               "Feishu",
	ScanQRCode:           "\nImbas kod QR dengan Feishu/Lark:\n\n",
	ScanOrOpenLink:       "\nAtau buka pautan di bawah dalam pelayar anda:\n",
	WaitingForScan:       "Mengambil hasil konfigurasi...",
	OpenLinkNonTTY:       "\nBuka pautan di bawah untuk mengkonfigurasi aplikasi:\n\n",
	WaitingForScanNonTTY: "Menunggu konfigurasi aplikasi...",
	DetectedLarkTenant:   "[lark-cli] Penyewa Lark dikesan, menukar endpoint...",
	AppCreated:           "Aplikasi dikonfigurasi! App ID: %s",
	ConfigSaved:          "Aplikasi dikonfigurasi! App ID: %s",
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
