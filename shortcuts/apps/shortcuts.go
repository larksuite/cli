// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import "github.com/larksuite/cli/shortcuts/common"

// Shortcuts returns all apps domain shortcuts.
func Shortcuts() []common.Shortcut {
	envSet := withExtraTips(AppsEnvVarSet, "Example: lark-cli apps +env-set --app-id <app_id> --environment online --key FOO --value <value> --yes")
	envDelete := withExtraTips(AppsEnvVarDelete, "Tip: +env-delete is high-risk-write; only pass --yes after explicit confirmation.")

	return []common.Shortcut{
		AppsCreate,
		AppsUpdate,
		AppsList,
		AppsAccessScopeSet,
		AppsAccessScopeGet,
		AppsHTMLPublish,
		AppsInit,
		AppsReleaseCreate,
		AppsReleaseList,
		AppsReleaseGet,
		AppsEnvPull,
		withExtraTips(AppsLogList, "Tip: logs are online-only; keep --environment omitted or set --environment online."),
		withExtraTips(AppsLogGet, "Tip: logs are online-only; keep --environment omitted or set --environment online."),
		withExtraTips(AppsTraceList, "Tip: traces are online-only; keep --environment omitted or set --environment online."),
		withExtraTips(AppsTraceGet, "Tip: traces are online-only; keep --environment omitted or set --environment online."),
		withExtraTips(AppsMetricList, "Tip: metrics are online-only; keep --environment omitted or set --environment online."),
		withExtraTips(AppsAnalyticsList, "Tip: analytics are online-only; keep --environment omitted or set --environment online."),
		AppsEnvVarList,
		envSet,
		envDelete,
		AppsDBTableList,
		AppsDBTableGet,
		AppsDBExecute,
		AppsDBEnvCreate,
		AppsDBDataImport,
		AppsDBDataExport,
		AppsDBChangelogList,
		AppsDBAuditStatus,
		AppsDBAuditEnable,
		AppsDBAuditDisable,
		AppsDBAuditList,
		AppsDBEnvDiff,
		AppsDBEnvMigrate,
		AppsDBRecoveryDiff,
		AppsDBRecoveryApply,
		AppsDBQuotaGet,
		AppsFileList,
		AppsFileGet,
		AppsFileSign,
		AppsFileDownload,
		AppsFileUpload,
		AppsFileDelete,
		AppsFileQuotaGet,
		AppsGitCredentialInit,
		AppsGitCredentialList,
		AppsGitCredentialRemove,
		AppsSessionCreate,
		AppsSessionList,
		AppsSessionGet,
		AppsSessionStop,
		AppsSessionMessagesList,
		AppsChat,
		AppsPluginInstall,
		AppsPluginUninstall,
		AppsPluginList,
		// open API key management
		AppsOpenAPIKeyList,
		AppsOpenAPIKeyGet,
		AppsOpenAPIKeyCreate,
		AppsOpenAPIKeyUpdate,
		AppsOpenAPIKeyEnable,
		AppsOpenAPIKeyDisable,
		AppsOpenAPIKeyDelete,
		AppsOpenAPIKeyReset,
	}
}

func withExtraTips(sc common.Shortcut, tips ...string) common.Shortcut {
	sc.Tips = append(append([]string{}, sc.Tips...), tips...)
	return sc
}
