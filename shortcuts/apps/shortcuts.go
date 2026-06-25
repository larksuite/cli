// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import "github.com/larksuite/cli/shortcuts/common"

// Shortcuts returns all apps domain shortcuts.
func Shortcuts() []common.Shortcut {
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
