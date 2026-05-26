// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

// bindMsg holds all TUI text for config bind, supporting zh/en via --lang.
//
// Brand-aware strings use a %s slot where the UI-friendly product name
// should appear; callers pass brandDisplay(brand, lang) at that position.
// English templates use %[N]s positional indices when the natural English
// order puts brand before source.
type bindMsg struct {
	// Source selection.
	// SelectSourceDesc format: brand.
	SelectSource      string
	SelectSourceDesc  string
	SourceOpenClaw    string // format: resolved config path.
	SourceHermes      string // format: resolved dotenv path.
	SourceLarkChannel string // format: resolved config path.

	// Account selection (OpenClaw multi-account).
	// Format: source display name ("OpenClaw" | "Hermes"), brand.
	SelectAccount string

	// Conflict prompt.
	ConflictTitle     string
	ConflictDesc      string // format: workspace, appId, brand, configPath.
	ConflictForce     string
	ConflictCancel    string
	ConflictCancelled string

	// Post-bind agent-friendly message emitted in the stdout JSON envelope's
	// "message" field. Written as imperative instructions to the agent reading
	// the JSON — not as description for a human reader.
	// MessageBotOnly format: app_id, source display name, brand.
	// MessageUserDefault format: app_id, source display name, source display
	// name (second source ref anchors the "run in this chat" directive).
	// MessageUserDefault directs the Agent at the blocking single-call
	// `auth login --recommend` flow: the CLI streams verification_url to
	// stderr, which Agent runtimes (OpenClaw, Hermes) relay to the user in
	// real time, then blocks until the user authorizes in their own browser.
	// The Agent also needs an explicit "do not navigate the URL yourself"
	// guard — its own browser is sandboxed and cannot complete the user's
	// authorization.
	MessageBotOnly     string
	MessageUserDefault string

	// Identity preset (collapses strict-mode + default-as into one choice).
	// IdentityBotOnly/IdentityUserDefault are short, single-line labels for
	// the huh Select options. IdentityBotOnlyDesc / IdentityUserDefaultDesc
	// carry the longer explanation for each choice; tuiSelectIdentity
	// embeds the description under its label as a multi-line option value,
	// so huh renders the whole "label + indented description" block as one
	// picker row and styles it selected / unselected as a unit. Dynamic
	// DescriptionFunc was tried first but breaks here: a longer description
	// on hover pushes the field's initial viewport, clipping the selected
	// option row on terminals that fit the smaller description.
	// IdentityBotOnlyDesc format: brand.
	// IdentityUserDefaultDesc format: brand, brand.
	SelectIdentity          string
	IdentityBotOnly         string
	IdentityUserDefault     string
	IdentityBotOnlyDesc     string
	IdentityUserDefaultDesc string

	// Post-bind success notice printed to stderr once the workspace config
	// has been durably written. Rendered as two parts joined with "\n":
	//   BindSuccessHeader — format: source display name.
	//   BindSuccessNotice — caveat about one-time sync.
	// We intentionally do NOT emit a "replaced" suffix here (the TUI already
	// asked the user to confirm overwrite; flag mode carries `replaced:true`
	// in the stdout JSON envelope), and we do NOT emit an inline "next step"
	// line for user-default (stderr is the human channel; agents read the
	// MessageUserDefault field in the JSON envelope).
	BindSuccessHeader string
	BindSuccessNotice string

	// IdentityEscalationMessage / IdentityEscalationHint are returned when a
	// previous bind set the workspace to bot-only and a flag-mode (AI-driven)
	// caller tries to rebind with --identity user-default without --force.
	// The error asks the Agent to surface the risk to the user and re-run
	// with --force only after explicit user confirmation. TUI mode does not
	// hit this code path — tuiConflictPrompt + tuiSelectIdentity already
	// require in-flow human confirmation.
	IdentityEscalationMessage string
	IdentityEscalationHint    string

	// LangPreferenceSet is printed to stderr after a successful bind when the
	// user explicitly passed --lang. Format: language code. Not printed when
	// --lang was not explicit (i.e., the cobra default zh stayed in effect).
	LangPreferenceSet string
}

var bindMsgZh = &bindMsg{
	SelectSource:      "你想在哪个 Agent 中使用 lark-cli?",
	SelectSourceDesc:  "从你选择的 Agent 中获取%s应用信息，并配置到 lark-cli 中",
	SourceOpenClaw:    "OpenClaw — 配置文件: %s",
	SourceHermes:      "Hermes — 配置文件: %s",
	SourceLarkChannel: "Lark Channel — 配置文件: %s",

	SelectAccount: "检测到 %s 中已配置多个%s应用，请选择一个",

	ConflictTitle:     "检测到已有配置",
	ConflictDesc:      "%q 已配置 lark-cli:\n  App ID:  %s\n  品牌:    %s\n  配置文件: %s",
	ConflictForce:     "修改配置",
	ConflictCancel:    "保留当前配置",
	ConflictCancelled: "已保留当前配置",

	MessageBotOnly:     "已绑定应用 %s 到 %s，可立即以应用（bot）身份调用%s API，现在可以继续执行用户的请求。",
	MessageUserDefault: "已绑定应用 %s 到 %s。请接着在此 %s 对话中运行 `lark-cli auth login --recommend`。该命令会在 stderr 打出 verification_url 后阻塞等待用户授权；请将此链接原样发给用户在其浏览器中完成授权（不要自己调 browser_navigate 之类的工具打开，授权必须在用户的浏览器里完成），命令会在用户授权完成后自动返回。",

	SelectIdentity:      "你希望 AI 如何与你协作？",
	IdentityBotOnly:     "以机器人身份",
	IdentityUserDefault: "以你的身份",
	IdentityBotOnlyDesc: "AI 将在%s中以机器人的身份执行所有操作，适合作为团队助手，用于多人协作场景，如群聊问答、团队通知、公共文档维护。",
	IdentityUserDefaultDesc: "AI 将在%s中以你的名义执行所有操作，如读写文档、搜索消息、修改日程等，建议仅限个人使用。\n" +
		"⚠️  请勿将此机器人分享给他人或拉入群聊中使用，以免泄露你的%s数据。",

	BindSuccessHeader: "配置成功！lark-cli 已可在 %s 中使用。",
	BindSuccessNotice: "注意：这是一次性同步，后续 Agent 配置变更不会自动更新到 lark-cli。如需重新同步，请执行 `lark-cli config bind`",

	IdentityEscalationMessage: "你正在从应用身份切换到用户身份 —— 切换后 AI 将以你的名义在飞书中执行所有操作（读写文档、搜索消息、修改日程等）。⚠️ 请勿将此机器人分享给他人或拉入群聊中使用，以免泄露你的飞书数据。",
	IdentityEscalationHint:    "若用户确认切换，附加 --force 重新运行：`lark-cli config bind --identity user-default --force`",

	LangPreferenceSet: "语言偏好已设置：%s",
}

var bindMsgEn = &bindMsg{
	SelectSource:      "Which Agent are you running?",
	SelectSourceDesc:  "lark-cli will read your %s app credentials from the selected Agent and apply them automatically.",
	SourceOpenClaw:    "OpenClaw — config: %s",
	SourceHermes:      "Hermes — config: %s",
	SourceLarkChannel: "Lark Channel — config: %s",

	// Args order (source, brand) matches the Chinese template; %[N]s lets the
	// English reading order differ while the caller passes args in one order.
	SelectAccount: "Multiple %[2]s apps configured in %[1]s — select one to continue.",

	ConflictTitle:     "Existing configuration found",
	ConflictDesc:      "lark-cli is already set up for %q:\n  App ID:  %s\n  Brand:   %s\n  Config:  %s",
	ConflictForce:     "Update config",
	ConflictCancel:    "Keep current config",
	ConflictCancelled: "Current config kept. No changes made.",

	MessageBotOnly:     "Bound app %s to %s. The %s app (bot) identity is ready — you can now continue with the user's request.",
	MessageUserDefault: "Bound app %s to %s. Next, in this %s chat, run `lark-cli auth login --recommend`. The command prints the verification URL to stderr and then blocks until the user authorizes it; relay the URL to the user so they can approve it in their own browser (do not call browser_navigate or any tool that opens a browser yourself — your browser is sandboxed and cannot complete the authorization). The command returns automatically once authorization completes.",

	SelectIdentity:      "How should the AI work with you?",
	IdentityBotOnly:     "As bot",
	IdentityUserDefault: "As you",
	IdentityBotOnlyDesc: "Works under its own identity in %s. Best for group chats, team notifications, and shared documents.",
	IdentityUserDefaultDesc: "Works under your identity in %s, managing docs, messages, calendar, and more on your behalf. Personal use only.\n" +
		"⚠️  Don't share this bot with others or add it to group chats. It has access to your personal %s data.",

	BindSuccessHeader: "All set! lark-cli is now ready to use in %s.",
	BindSuccessNotice: "Note: This is a one-time sync. To re-sync future changes, run `lark-cli config bind`",

	IdentityEscalationMessage: "you are switching from bot-only to user-default — the AI will then act under your Feishu identity for all operations (docs, messages, calendar, etc.). ⚠️ Don't share this bot with others or add it to group chats. It has access to your personal Feishu data.",
	IdentityEscalationHint:    "if the user confirms the switch, re-run with --force: `lark-cli config bind --identity user-default --force`",

	LangPreferenceSet: "Language preference set to: %s",
}

func getBindMsg(lang string) *bindMsg {
	switch lang {
	case "en":
		return bindMsgEn
	case "ja":
		return bindMsgJa
	case "ko":
		return bindMsgKo
	case "fr":
		return bindMsgFr
	case "de":
		return bindMsgDe
	case "es":
		return bindMsgEs
	case "it":
		return bindMsgIt
	case "ru":
		return bindMsgRu
	case "pt":
		return bindMsgPt
	case "th":
		return bindMsgTh
	case "vi":
		return bindMsgVi
	case "id":
		return bindMsgId
	case "ms":
		return bindMsgMs
	default:
		return bindMsgZh
	}
}

// brandDisplay returns the UI-friendly product name for the given brand
// identifier and display language. "lark" maps to "Lark" in both zh and en.
// "feishu" (or empty / unknown) maps to "飞书" in zh and "Feishu" in en —
// this is the safe default when the brand hasn't been resolved yet (for
// example, on the pre-binding source-selection screen).
func brandDisplay(brand, lang string) string {
	if brand == "lark" || brand == "Lark" || brand == "LARK" {
		return "Lark"
	}
	if lang == "en" {
		return "Feishu"
	}
	return "飞书"
}

var bindMsgJa = &bindMsg{
	SelectSource:              "どの Agent で実行していますか?",
	SelectSourceDesc:          "選択した Agent から%sアプリ情報を取得し、lark-cli に自動設定します",
	SourceOpenClaw:            "OpenClaw — 設定ファイル: %s",
	SourceHermes:              "Hermes — 設定ファイル: %s",
	SourceLarkChannel:         "Lark Channel — 設定ファイル: %s",
	SelectAccount:             "%[1]s で複数の%[2]sアプリが検出 — 続行するものを選択",
	ConflictTitle:             "既存の設定が見つかりました",
	ConflictDesc:              "%q は既に lark-cli を設定:\n  App ID:  %s\n  ブランド:   %s\n  設定ファイル:  %s",
	ConflictForce:             "設定を更新",
	ConflictCancel:            "現在の設定を保持",
	ConflictCancelled:         "現在の設定を保持しました。変更はありません。",
	MessageBotOnly:            "アプリ %s を %s にバインド。%s アプリ (bot)  identity の準備が完了 — ユーザーのリクエストを続行できます。",
	MessageUserDefault:        "アプリ %s を %s にバインド。次に、この %s チャットで `lark-cli auth login --recommend` を実行。コマンドは検証 URL を stderr に出力し、ユーザーが承認するまでブロックします。",
	SelectIdentity:            "AI はどのように動作しますか?",
	IdentityBotOnly:           "Bot として",
	IdentityUserDefault:       "あなたとして",
	IdentityBotOnlyDesc:       "%s で独自の identity で動作。グループチャット、チーム通知、共有ドキュメントに最適。",
	IdentityUserDefaultDesc:   "%s であなたの identity で動作し、ドキュメント、メッセージ、カレンダーなどを管理。個人使用のみ。\n⚠️  このボットを他の人と共有したり、グループチャットに追加したりしないでください。あなたの個人 %s データにアクセスできます。",
	BindSuccessHeader:         "準備完了! lark-cli は %s で使用可能です。",
	BindSuccessNotice:         "注: これは一回限りの同期です。将来の変更を再同期するには、`lark-cli config bind` を実行してください。",
	IdentityEscalationMessage: "bot-only から user-default に切り替え中 — AI はあなたの Feishu identity ですべての操作を実行します (ドキュメント、メッセージ、カレンダーなど)。⚠️ このボットを他の人と共有したり、グループチャットに追加したりしないでください。あなたの個人 Feishu データにアクセスできます。",
	IdentityEscalationHint:    "ユーザーが切り替えを確認した場合、`--force` で再実行: `lark-cli config bind --identity user-default --force`",
}

var bindMsgKo = &bindMsg{
	SelectSource:              "어떤 Agent를 실행 중인가요?",
	SelectSourceDesc:          "선택한 Agent에서 %s 앱 인증 정보를 읽어 lark-cli에 자동 적용합니다",
	SourceOpenClaw:            "OpenClaw — 설정 파일: %s",
	SourceHermes:              "Hermes — 설정 파일: %s",
	SourceLarkChannel:         "Lark Channel — 설정 파일: %s",
	SelectAccount:             "%[1]s에서 여러 %[2]s 앱이 설정됨 — 계속할 것을 선택",
	ConflictTitle:             "기존 설정 발견",
	ConflictDesc:              "%q는 이미 lark-cli가 설정됨:\n  App ID:  %s\n  브랜드:   %s\n  설정 파일:  %s",
	ConflictForce:             "설정 업데이트",
	ConflictCancel:            "현재 설정 유지",
	ConflictCancelled:         "현재 설정을 유지했습니다. 변경 사항 없음。",
	MessageBotOnly:            "앱 %s를 %s에 바인드。%s 앱 (bot) identity 준비 완료 — 사용자 요청을 계속할 수 있습니다。",
	MessageUserDefault:        "앱 %s를 %s에 바인드。다음으로, 이 %s 채팅에서 `lark-cli auth login --recommend`를 실행합니다。",
	SelectIdentity:            "AI는 어떻게 작동하나요?",
	IdentityBotOnly:           "Bot으로",
	IdentityUserDefault:       "사용자로",
	IdentityBotOnlyDesc:       "%s에서 자체 identity로 작동。그룹 채팅, 팀 알림, 공유 문서에 가장 적합。",
	IdentityUserDefaultDesc:   "%s에서 사용자를 대신하여 작동하여 문서, 메시지, 일정 등을 관리합니다。개인용으로만 사용。\n⚠️  이 봇을 다른 사람과 공유하거나 그룹 채팅에 추가하지 마세요。사용자의 개인 %s 데이터에 접근할 수 있습니다。",
	BindSuccessHeader:         "준비 완료! lark-cli가 %s에서 사용 가능합니다。",
	BindSuccessNotice:         "참고: 이것은 일회성 동기화입니다。향후 변경 사항을 재동기화하려면 `lark-cli config bind`를 실행하세요。",
	IdentityEscalationMessage: "bot-only에서 user-default로 전환 중 — AI가 모든 작업을 귀하의 Feishu 신원으로 수행하게 됩니다。⚠️ 이 봇을 다른 사람과 공유하거나 그룹 채팅에 추가하지 마세요。",
	IdentityEscalationHint:    "사용자가 전환을 확인한 경우 `--force`로 재실행: `lark-cli config bind --identity user-default --force`",
}

var bindMsgFr = &bindMsg{
	SelectSource:              "Quel Agent utilisez-vous?",
	SelectSourceDesc:          "lark-cli lira vos informations d'identification %s depuis l'Agent sélectionné et les appliquera automatiquement.",
	SourceOpenClaw:            "OpenClaw — config: %s",
	SourceHermes:              "Hermes — config: %s",
	SourceLarkChannel:         "Lark Channel — config: %s",
	SelectAccount:             "Plusieurs apps %[2]s configurées dans %[1]s — sélectionnez-en une pour continuer.",
	ConflictTitle:             "Configuration existante trouvée",
	ConflictDesc:              "lark-cli est déjà configuré pour %q:\n  App ID:  %s\n  Marque:   %s\n  Config:  %s",
	ConflictForce:             "Mettre à jour la config",
	ConflictCancel:            "Garder la config actuelle",
	ConflictCancelled:         "Config actuelle conservée. Aucune modification.",
	MessageBotOnly:            "App %s liée à %s. L'identity %s (bot) est prête — vous pouvez maintenant continuer avec la requête de l'utilisateur.",
	MessageUserDefault:        "App %s liée à %s. Ensuite, dans ce chat %s, exécutez `lark-cli auth login --recommend`. La commande affiche l'URL de vérification sur stderr et attend que l'utilisateur l'autorise.",
	SelectIdentity:            "Comment l'AI doit-elle travailler avec vous?",
	IdentityBotOnly:           "En tant que bot",
	IdentityUserDefault:       "En tant que vous",
	IdentityBotOnlyDesc:       "Fonctionne sous sa propre identity dans %s. Idéal pour les chats de groupe, notifications d'équipe et documents partagés.",
	IdentityUserDefaultDesc:   "Fonctionne sous votre identity dans %s, gérant documents, messages, calendrier, etc. Uniquement pour usage personnel.\n⚠️  Ne partagez pas ce bot avec d'autres ou ne l'ajoutez pas aux chats de groupe. Il a accès à vos données %s personnelles.",
	BindSuccessHeader:         "Prêt! lark-cli est maintenant utilisable dans %s.",
	BindSuccessNotice:         "Note: C'est une synchronisation unique. Pour re-synchroniser, exécutez `lark-cli config bind`",
	IdentityEscalationMessage: "Vous passez de bot-only à user-default — l'AI agira alors sous votre identity Feishu pour toutes les opérations. ⚠️ Ne partagez pas ce bot avec d'autres.",
	IdentityEscalationHint:    "Si l'utilisateur confirme, re-exécutez avec --force: `lark-cli config bind --identity user-default --force`",
}

var bindMsgDe = &bindMsg{
	SelectSource:              "Welchen Agent verwenden Sie?",
	SelectSourceDesc:          "lark-cli liest Ihre %s-App-Anmeldeinformationen aus dem ausgewählten Agent und wendet sie automatisch an.",
	SourceOpenClaw:            "OpenClaw — Konfig: %s",
	SourceHermes:              "Hermes — Konfig: %s",
	SourceLarkChannel:         "Lark Channel — Konfig: %s",
	SelectAccount:             "Mehrere %[2]s-Apps in %[1]s konfiguriert — wählen Sie eine zum Fortfahren.",
	ConflictTitle:             "Bestehende Konfiguration gefunden",
	ConflictDesc:              "lark-cli ist bereits für %q eingerichtet:\n  App ID:  %s\n  Marke:   %s\n  Konfig:  %s",
	ConflictForce:             "Konfig aktualisieren",
	ConflictCancel:            "Aktuelle Konfig behalten",
	ConflictCancelled:         "Aktuelle Konfig beibehalten. Keine Änderungen.",
	MessageBotOnly:            "App %s an %s gebunden. Die %s-App (Bot)-Identity ist bereit — Sie können nun mit der Anfrage des Benutzers fortfahren.",
	MessageUserDefault:        "App %s an %s gebunden. Führen Sie als Nächstes in diesem %s-Chat `lark-cli auth login --recommend` aus.",
	SelectIdentity:            "Wie soll die AI mit Ihnen arbeiten?",
	IdentityBotOnly:           "Als Bot",
	IdentityUserDefault:       "Als Sie",
	IdentityBotOnlyDesc:       "Funktioniert unter eigener Identity in %s. Am besten für Gruppenchats, Team-Benachrichtigungen und gemeinsame Dokumente.",
	IdentityUserDefaultDesc:   "Funktioniert unter Ihrer Identity in %s und verwaltet Dokumente, Nachrichten, Kalender usw. Nur für persönliche Nutzung.\n⚠️  Teilen Sie diesen Bot nicht mit anderen oder fügen Sie ihn zu Gruppenchats hinzu. Er hat Zugriff auf Ihre persönlichen %s-Daten.",
	BindSuccessHeader:         "Fertig! lark-cli ist jetzt in %s einsatzbereit.",
	BindSuccessNotice:         "Hinweis: Dies ist eine einmalige Synchronisation. Für Re-Sync: `lark-cli config bind`",
	IdentityEscalationMessage: "Sie wechseln von Bot-only zu User-default — die AI wird dann unter Ihrer Feishu-Identity agieren. ⚠️ Teilen Sie diesen Bot nicht.",
	IdentityEscalationHint:    "Wenn der Benutzer bestätigt, mit --force erneut ausführen: `lark-cli config bind --identity user-default --force`",
}

var bindMsgEs = &bindMsg{
	SelectSource:              "¿Qué Agent está ejecutando?",
	SelectSourceDesc:          "lark-cli leerá las credenciales de su app %s desde el Agent seleccionado y las aplicará automáticamente.",
	SourceOpenClaw:            "OpenClaw — config: %s",
	SourceHermes:              "Hermes — config: %s",
	SourceLarkChannel:         "Lark Channel — config: %s",
	SelectAccount:             "Múltiples apps %[2]s configuradas en %[1]s — seleccione una para continuar.",
	ConflictTitle:             "Configuración existente encontrada",
	ConflictDesc:              "lark-cli ya está configurado para %q:\n  App ID:  %s\n  Marca:   %s\n  Config:  %s",
	ConflictForce:             "Actualizar config",
	ConflictCancel:            "Mantener config actual",
	ConflictCancelled:         "Config actual mantenida. Sin cambios.",
	MessageBotOnly:            "App %s vinculada a %s. La identity %s (bot) está lista — ahora puede continuar con la solicitud del usuario.",
	MessageUserDefault:        "App %s vinculada a %s. A continuación, en este chat %s, ejecute `lark-cli auth login --recommend`.",
	SelectIdentity:            "¿Cómo debería trabajar la AI con usted?",
	IdentityBotOnly:           "Como bot",
	IdentityUserDefault:       "Como usted",
	IdentityBotOnlyDesc:       "Funciona bajo su propia identity en %s. Mejor para chats grupales, notificaciones de equipo y documentos compartidos.",
	IdentityUserDefaultDesc:   "Funciona bajo su identity en %s, gestionando documentos, mensajes, calendario, etc. Solo uso personal.\n⚠️  No comparta este bot con otros ni lo agregue a chats grupales. Tiene acceso a sus datos %s personales.",
	BindSuccessHeader:         "¡Listo! lark-cli ahora está listo para usar en %s.",
	BindSuccessNotice:         "Nota: Esta es una sincronización única. Para re-sincronizar: `lark-cli config bind`",
	IdentityEscalationMessage: "Está cambiando de bot-only a user-default — la AI actuará bajo su identity Feishu. ⚠️ No comparta este bot.",
	IdentityEscalationHint:    "Si el usuario confirma, vuelva a ejecutar con --force: `lark-cli config bind --identity user-default --force`",
}

var bindMsgIt = &bindMsg{
	SelectSource:              "Quale Agent stai eseguendo?",
	SelectSourceDesc:          "lark-cli leggerà le credenziali della tua app %s dall'Agent selezionato e le applicherà automaticamente.",
	SourceOpenClaw:            "OpenClaw — config: %s",
	SourceHermes:              "Hermes — config: %s",
	SourceLarkChannel:         "Lark Channel — config: %s",
	SelectAccount:             "Più app %[2]s configurate in %[1]s — selezionane una per continuare.",
	ConflictTitle:             "Configurazione esistente trovata",
	ConflictDesc:              "lark-cli è già configurato per %q:\n  App ID:  %s\n  Marchio:   %s\n  Config:  %s",
	ConflictForce:             "Aggiorna config",
	ConflictCancel:            "Mantieni config attuale",
	ConflictCancelled:         "Config attuale mantenuta. Nessuna modifica.",
	MessageBotOnly:            "App %s collegata a %s. L'identity %s (bot) è pronta — ora puoi continuare con la richiesta dell'utente.",
	MessageUserDefault:        "App %s collegata a %s. Successivamente, in questa chat %s, esegui `lark-cli auth login --recommend`.",
	SelectIdentity:            "Come dovrebbe lavorare l'AI con te?",
	IdentityBotOnly:           "Come bot",
	IdentityUserDefault:       "Come te",
	IdentityBotOnlyDesc:       "Funziona con la propria identity in %s. Ideale per chat di gruppo, notifiche di squadra e documenti condivisi.",
	IdentityUserDefaultDesc:   "Funziona con la tua identity in %s, gestendo documenti, messaggi, calendario, ecc. Solo uso personale.\n⚠️  Non condividere questo bot con altri o aggiungerlo a chat di gruppo. Ha accesso ai tuoi dati %s personali.",
	BindSuccessHeader:         "Tutto pronto! lark-cli è ora pronto per l'uso in %s.",
	BindSuccessNotice:         "Nota: Questa è una sincronizzazione una tantum. Per re-sincronizzare: `lark-cli config bind`",
	IdentityEscalationMessage: "Stai passando da bot-only a user-default — l'AI agirà con la tua identity Feishu. ⚠️ Non condividere questo bot.",
	IdentityEscalationHint:    "Se l'utente conferma, riesegui con --force: `lark-cli config bind --identity user-default --force`",
}

var bindMsgRu = &bindMsg{
	SelectSource:              "Какой Agent вы используете?",
	SelectSourceDesc:          "lark-cli прочитает учетные данные вашего приложения %s из выбранного Agent и применит их автоматически.",
	SourceOpenClaw:            "OpenClaw — конфиг: %s",
	SourceHermes:              "Hermes — конфиг: %s",
	SourceLarkChannel:         "Lark Channel — конфиг: %s",
	SelectAccount:             "Несколько приложений %[2]s настроены в %[1]s — выберите одно для продолжения.",
	ConflictTitle:             "Найдена существующая конфигурация",
	ConflictDesc:              "lark-cli уже настроен для %q:\n  App ID:  %s\n  Бренд:   %s\n  Конфиг:  %s",
	ConflictForce:             "Обновить конфиг",
	ConflictCancel:            "Сохранить текущий конфиг",
	ConflictCancelled:         "Текущий конфиг сохранен. Изменений нет.",
	MessageBotOnly:            "Приложение %s привязано к %s. Identity %s (бот) готова — теперь вы можете продолжить запрос пользователя.",
	MessageUserDefault:        "Приложение %s привязано к %s. Далее, в этом чате %s, выполните `lark-cli auth login --recommend`.",
	SelectIdentity:            "Как AI должен работать с вами?",
	IdentityBotOnly:           "Как бот",
	IdentityUserDefault:       "Как вы",
	IdentityBotOnlyDesc:       "Работает под собственной identity в %s. Лучше всего подходит для групповых чатов, уведомлений команды и общих документов.",
	IdentityUserDefaultDesc:   "Работает под вашей identity в %s, управляя документами, сообщениями, календарем и т.д. Только для личного использования.\n⚠️  Не делитесь этим ботом с другими и не добавляйте его в групповые чаты. У него есть доступ к вашим личным данным %s.",
	BindSuccessHeader:         "Готово! lark-cli теперь готов к использованию в %s.",
	BindSuccessNotice:         "Примечание: Это одноразовая синхронизация. Для повторной синхронизации: `lark-cli config bind`",
	IdentityEscalationMessage: "Вы переключаетесь с bot-only на user-default — AI будет действовать под вашей identity Feishu. ⚠️ Не делитесь этим ботом.",
	IdentityEscalationHint:    "Если пользователь подтвердил, повторите с --force: `lark-cli config bind --identity user-default --force`",
}

var bindMsgPt = &bindMsg{
	SelectSource:              "Qual Agent você está executando?",
	SelectSourceDesc:          "lark-cli lerá as credenciais do seu app %s do Agent selecionado e as aplicará automaticamente.",
	SourceOpenClaw:            "OpenClaw — config: %s",
	SourceHermes:              "Hermes — config: %s",
	SourceLarkChannel:         "Lark Channel — config: %s",
	SelectAccount:             "Múltiplos apps %[2]s configurados em %[1]s — selecione um para continuar.",
	ConflictTitle:             "Configuração existente encontrada",
	ConflictDesc:              "lark-cli já está configurado para %q:\n  App ID:  %s\n  Marca:   %s\n  Config:  %s",
	ConflictForce:             "Atualizar config",
	ConflictCancel:            "Manter config atual",
	ConflictCancelled:         "Config atual mantida. Sem alterações.",
	MessageBotOnly:            "App %s vinculado a %s. A identity %s (bot) está pronta — agora você pode continuar com a solicitação do usuário.",
	MessageUserDefault:        "App %s vinculado a %s. Em seguida, neste chat %s, execute `lark-cli auth login --recommend`.",
	SelectIdentity:            "Como a AI deve trabalhar com você?",
	IdentityBotOnly:           "Como bot",
	IdentityUserDefault:       "Como você",
	IdentityBotOnlyDesc:       "Funciona sob sua própria identity em %s. Melhor para chats em grupo, notificações de equipe e documentos compartilhados.",
	IdentityUserDefaultDesc:   "Funciona sob sua identity em %s, gerenciando documentos, mensagens, calendário, etc. Apenas para uso pessoal.\n⚠️  Não compartilhe este bot com outros ou o adicione a chats em grupo. Ele tem acesso aos seus dados %s pessoais.",
	BindSuccessHeader:         "Pronto! lark-cli agora está pronto para uso em %s.",
	BindSuccessNotice:         "Nota: Esta é uma sincronização única. Para re-sincronizar: `lark-cli config bind`",
	IdentityEscalationMessage: "Você está mudando de bot-only para user-default — a AI agirá sob sua identity Feishu. ⚠️ Não compartilhe este bot.",
	IdentityEscalationHint:    "Se o usuário confirmar, execute novamente com --force: `lark-cli config bind --identity user-default --force`",
}

var bindMsgTh = &bindMsg{
	SelectSource:              "คุณกำลังรัน Agent ใด?",
	SelectSourceDesc:          "lark-cli จะอ่านข้อมูลประจำตัวแอป %s ของคุณจาก Agent ที่เลือกและนำไปใช้โดยอัตโนมัติ",
	SourceOpenClaw:            "OpenClaw — การกำหนดค่า: %s",
	SourceHermes:              "Hermes — การกำหนดค่า: %s",
	SourceLarkChannel:         "Lark Channel — การกำหนดค่า: %s",
	SelectAccount:             "มีแอป %[2]s หลายตัวที่กำหนดค่าไว้ใน %[1]s — เลือกหนึ่งตัวเพื่อดำเนินการต่อ",
	ConflictTitle:             "พบการกำหนดค่าที่มีอยู่",
	ConflictDesc:              "lark-cli ได้รับการตั้งค่าสำหรับ %q แล้ว:\n  App ID:  %s\n  แบรนด์:   %s\n  การกำหนดค่า:  %s",
	ConflictForce:             "อัปเดตการกำหนดค่า",
	ConflictCancel:            "เก็บการกำหนดค่าปัจจุบัน",
	ConflictCancelled:         "เก็บการกำหนดค่าปัจจุบัน ไม่มีการเปลี่ยนแปลง",
	MessageBotOnly:            "ผูกแอป %s กับ %s แล้ว identity %s (บอท) พร้อมใช้งาน — ตอนนี้คุณสามารถดำเนินการตามคำขอของผู้ใช้ต่อได้",
	MessageUserDefault:        "ผูกแอป %s กับ %s แล้ว จากนั้น ในแชท %s นี้ ให้รัน `lark-cli auth login --recommend`",
	SelectIdentity:            "AI ควรทำงานกับคุณอย่างไร?",
	IdentityBotOnly:           "ในฐานะบอท",
	IdentityUserDefault:       "ในฐานะคุณ",
	IdentityBotOnlyDesc:       "ทำงานภายใต้ identity ของตนเองใน %s เหมาะสำหรับแชทกลุ่ม การแจ้งเตือนทีม และเอกสารที่ใช้ร่วมกัน",
	IdentityUserDefaultDesc:   "ทำงานภายใต้ identity ของคุณใน %s และจัดการเอกสาร ข้อความ ปฏิทิน ฯลฯ สำหรับใช้ส่วนบุคคลเท่านั้น\n⚠️  อย่าแชร์บอทนี้กับผู้อื่นหรือเพิ่มเข้าไปในแชทกลุ่ม บอทมีสิทธิ์เข้าถึงข้อมูล %s ส่วนบุคคลของคุณ",
	BindSuccessHeader:         "พร้อมใช้งาน! lark-cli พร้อมใช้งานใน %s แล้ว",
	BindSuccessNotice:         "หมายเหตุ: นี่คือการซิงค์ครั้งเดียว หากต้องการซิงค์ใหม่: `lark-cli config bind`",
	IdentityEscalationMessage: "คุณกำลังเปลี่ยนจาก bot-only เป็น user-default — AI จะทำงานภายใต้ identity Feishu ของคุณ ⚠️ อย่าแชร์บอทนี้",
	IdentityEscalationHint:    "หากผู้ใช้ยืนยัน ให้รันอีกครั้งด้วย --force: `lark-cli config bind --identity user-default --force`",
}

var bindMsgVi = &bindMsg{
	SelectSource:              "Bạn đang chạy Agent nào?",
	SelectSourceDesc:          "lark-cli sẽ đọc thông tin xác thực ứng dụng %s của bạn từ Agent đã chọn và tự động áp dụng chúng.",
	SourceOpenClaw:            "OpenClaw — cấu hình: %s",
	SourceHermes:              "Hermes — cấu hình: %s",
	SourceLarkChannel:         "Lark Channel — cấu hình: %s",
	SelectAccount:             "Nhiều ứng dụng %[2]s được cấu hình trong %[1]s — chọn một để tiếp tục.",
	ConflictTitle:             "Đã tìm thấy cấu hình hiện có",
	ConflictDesc:              "lark-cli đã được thiết lập cho %q:\n  App ID:  %s\n  Thương hiệu:   %s\n  Cấu hình:  %s",
	ConflictForce:             "Cập nhật cấu hình",
	ConflictCancel:            "Giữ cấu hình hiện tại",
	ConflictCancelled:         "Đã giữ cấu hình hiện tại. Không có thay đổi.",
	MessageBotOnly:            "Đã liên kết ứng dụng %s với %s. identity %s (bot) đã sẵn sàng — bây giờ bạn có thể tiếp tục với yêu cầu của người dùng.",
	MessageUserDefault:        "Đã liên kết ứng dụng %s với %s. Tiếp theo, trong cuộc trò chuyện %s này, hãy chạy `lark-cli auth login --recommend`.",
	SelectIdentity:            "AI nên làm việc với bạn như thế nào?",
	IdentityBotOnly:           "Với tư cách bot",
	IdentityUserDefault:       "Với tư cách bạn",
	IdentityBotOnlyDesc:       "Hoạt động dưới identity của riêng mình trong %s. Tốt nhất cho trò chuyện nhóm, thông báo nhóm và tài liệu được chia sẻ.",
	IdentityUserDefaultDesc:   "Hoạt động dưới identity của bạn trong %s, quản lý tài liệu, tin nhắn, lịch, v.v. Chỉ dành cho sử dụng cá nhân.\n⚠️  Đừng chia sẻ bot này với người khác hoặc thêm vào trò chuyện nhóm. Nó có quyền truy cập vào dữ liệu %s cá nhân của bạn.",
	BindSuccessHeader:         "Sẵn sàng! lark-cli hiện đã sẵn sàng để sử dụng trong %s.",
	BindSuccessNotice:         "Lưu ý: Đây là đồng bộ hóa một lần. Để đồng bộ lại: `lark-cli config bind`",
	IdentityEscalationMessage: "Bạn đang chuyển từ bot-only sang user-default — AI sau đó sẽ hoạt động dưới identity Feishu của bạn. ⚠️ Đừng chia sẻ bot này.",
	IdentityEscalationHint:    "Nếu người dùng xác nhận, chạy lại với --force: `lark-cli config bind --identity user-default --force`",
}

var bindMsgId = &bindMsg{
	SelectSource:              "Agent mana yang Anda jalankan?",
	SelectSourceDesc:          "lark-cli akan membaca kredensial aplikasi %s Anda dari Agent yang dipilih dan menerapkannya secara otomatis.",
	SourceOpenClaw:            "OpenClaw — konfig: %s",
	SourceHermes:              "Hermes — konfig: %s",
	SourceLarkChannel:         "Lark Channel — konfig: %s",
	SelectAccount:             "Beberapa aplikasi %[2]s dikonfigurasi di %[1]s — pilih satu untuk melanjutkan.",
	ConflictTitle:             "Konfigurasi yang ada ditemukan",
	ConflictDesc:              "lark-cli sudah diatur untuk %q:\n  App ID:  %s\n  Merek:   %s\n  Konfig:  %s",
	ConflictForce:             "Perbarui konfig",
	ConflictCancel:            "Pertahankan konfig saat ini",
	ConflictCancelled:         "Konfig saat ini dipertahankan. Tidak ada perubahan.",
	MessageBotOnly:            "Aplikasi %s terikat ke %s. identity %s (bot) siap — sekarang Anda dapat melanjutkan dengan permintaan pengguna.",
	MessageUserDefault:        "Aplikasi %s terikat ke %s. Selanjutnya, dalam obrolan %s ini, jalankan `lark-cli auth login --recommend`.",
	SelectIdentity:            "Bagaimana AI harus bekerja dengan Anda?",
	IdentityBotOnly:           "Sebagai bot",
	IdentityUserDefault:       "Sebagai Anda",
	IdentityBotOnlyDesc:       "Bekerja di bawah identity sendiri di %s. Terbaik untuk obrolan grup, notifikasi tim, dan dokumen bersama.",
	IdentityUserDefaultDesc:   "Bekerja di bawah identity Anda di %s, mengelola dokumen, pesan, kalender, dll. Hanya untuk penggunaan pribadi.\n⚠️  Jangan bagikan bot ini dengan orang lain atau tambahkan ke obrolan grup. Bot memiliki akses ke data %s pribadi Anda.",
	BindSuccessHeader:         "Siap! lark-cli sekarang siap digunakan di %s.",
	BindSuccessNotice:         "Catatan: Ini adalah sinkronisasi satu kali. Untuk menyinkronkan ulang: `lark-cli config bind`",
	IdentityEscalationMessage: "Anda beralih dari bot-only ke user-default — AI kemudian akan bertindak di bawah identity Feishu Anda. ⚠️ Jangan bagikan bot ini.",
	IdentityEscalationHint:    "Jika pengguna mengonfirmasi, jalankan ulang dengan --force: `lark-cli config bind --identity user-default --force`",
}

var bindMsgMs = &bindMsg{
	SelectSource:              "Agent mana yang anda jalankan?",
	SelectSourceDesc:          "lark-cli akan membaca kelayakan aplikasi %s anda daripada Agent yang dipilih dan menggunakannya secara automatik.",
	SourceOpenClaw:            "OpenClaw — konfig: %s",
	SourceHermes:              "Hermes — konfig: %s",
	SourceLarkChannel:         "Lark Channel — konfig: %s",
	SelectAccount:             "Beberapa aplikasi %[2]s dikonfigurasi dalam %[1]s — pilih satu untuk meneruskan.",
	ConflictTitle:             "Konfigurasi sedia ada ditemui",
	ConflictDesc:              "lark-cli sudah disediakan untuk %q:\n  App ID:  %s\n  Jenama:   %s\n  Konfig:  %s",
	ConflictForce:             "Kemas kini konfig",
	ConflictCancel:            "Kekalkan konfig semasa",
	ConflictCancelled:         "Konfig semasa dikekalkan. Tiada perubahan.",
	MessageBotOnly:            "Aplikasi %s diikat ke %s. identity %s (bot) sedia — sekarang anda boleh meneruskan dengan permintaan pengguna.",
	MessageUserDefault:        "Aplikasi %s diikat ke %s. Seterusnya, dalam sembang %s ini, jalankan `lark-cli auth login --recommend`.",
	SelectIdentity:            "Bagaimana AI harus bekerja dengan anda?",
	IdentityBotOnly:           "Sebagai bot",
	IdentityUserDefault:       "Sebagai anda",
	IdentityBotOnlyDesc:       "Bekerja di bawah identity sendiri dalam %s. Terbaik untuk sembang berkumpulan, pemberitahuan pasukan, dan dokumen dikongsi.",
	IdentityUserDefaultDesc:   "Bekerja di bawah identity anda dalam %s, mengurus dokumen, mesej, kalendar, dll. Hanya untuk kegunaan peribadi.\n⚠️  Jangan kongsi bot ini dengan orang lain atau tambahkan ke sembang berkumpulan. Ia mempunyai akses kepada data %s peribadi anda.",
	BindSuccessHeader:         "Sedia! lark-cli kini sedia untuk digunakan dalam %s.",
	BindSuccessNotice:         "Nota: Ini adalah penyegerakan sekali. Untuk menyerak semula: `lark-cli config bind`",
	IdentityEscalationMessage: "Anda beralih dari bot-only ke user-default — AI kemudian akan bertindak di bawah identity Feishu anda. ⚠️ Jangan kongsi bot ini.",
	IdentityEscalationHint:    "Jika pengguna mengesahkan, jalankan semula dengan --force: `lark-cli config bind --identity user-default --force`",
}
