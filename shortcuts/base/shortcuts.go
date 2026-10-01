// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import "github.com/larksuite/cli/shortcuts/common"

// Shortcuts returns all base shortcuts.
func Shortcuts() []common.Shortcut {
	return withAppTokenAlias([]common.Shortcut{
		BaseURLResolve,
		BaseTitleResolve,
		BaseBaseBlockList,
		BaseBaseBlockCreate,
		BaseBaseBlockMove,
		BaseBaseBlockRename,
		BaseBaseBlockDelete,
		BaseTableList,
		BaseTableGet,
		BaseTableCreate,
		BaseTableUpdate,
		BaseTableDelete,
		BaseTableCopy,
		BaseTableCopyStatus,
		BaseFieldList,
		BaseFieldGet,
		BaseFieldCreate,
		BaseFieldUpdate,
		BaseFieldDelete,
		BaseFieldSearchOptions,
		BaseFieldExtensionGet,
		BaseFieldExtensionUpdate,
		BaseFieldExtensionUpdateCells,
		BaseViewList,
		BaseViewGet,
		BaseViewCreate,
		BaseViewDelete,
		BaseViewGetFilter,
		BaseViewSetFilter,
		BaseViewGetVisibleFields,
		BaseViewSetVisibleFields,
		BaseViewGetGroup,
		BaseViewSetGroup,
		BaseViewGetSort,
		BaseViewSetSort,
		BaseViewGetTimebar,
		BaseViewSetTimebar,
		BaseViewGetCard,
		BaseViewSetCard,
		BaseViewRename,
		BaseRecordList,
		BaseRecordSearch,
		BaseRecordGet,
		BaseRecordUpsert,
		BaseRecordBatchCreate,
		BaseRecordBatchUpdate,
		BaseRecordShareLinkCreate,
		BaseRecordUploadAttachment,
		BaseRecordDownloadAttachment,
		BaseRecordRemoveAttachment,
		BaseRecordDelete,
		BaseRecordHistoryList,
		BaseBaseGet,
		BaseBaseCopy,
		BaseBaseCreate,
		BaseTemplateCategories,
		BaseTemplateList,
		BaseTemplateSearch,
		BaseRoleCreate,
		BaseRoleDelete,
		BaseRoleUpdate,
		BaseRoleList,
		BaseRoleGet,
		BaseAdvpermEnable,
		BaseAdvpermDisable,
		BaseWorkflowList,
		BaseWorkflowGet,
		BaseWorkflowCreate,
		BaseWorkflowUpdate,
		BaseWorkflowEnable,
		BaseWorkflowDisable,
		BaseButtonRuleBind,
		BaseButtonRuleGet,
		BaseButtonRuleUnbind,
		BaseDataQuery,
		BaseFormCreate,
		BaseFormDelete,
		BaseFormsList,
		BaseFormUpdate,
		BaseFormGet,
		BaseFormDetail,
		BaseFormQuestionsCreate,
		BaseFormQuestionsDelete,
		BaseFormQuestionsUpdate,
		BaseFormQuestionsList,
		BaseFormSubmit,
		BaseFormShareGet,
		BaseFormShareUpdate,
		BaseDashboardList,
		BaseDashboardGet,
		BaseDashboardShareGet,
		BaseDashboardShareUpdate,
		BaseDashboardCreate,
		BaseDashboardUpdate,
		BaseDashboardDelete,
		BaseDashboardArrange,
		BaseDashboardBlockList,
		BaseDashboardBlockGet,
		BaseDashboardBlockGetData,
		BaseDashboardBlockCreate,
		BaseDashboardBlockUpdate,
		BaseDashboardBlockDelete,
		BaseWorkspaceCreate,
		BaseWorkspaceEntityList,
		BaseWorkspaceMoveIn,
		BaseAppCreate,
		BaseAppGet,
		BaseAppPageList,
		BaseAppPageGet,
		BaseAppPageCreate,
		BaseAppPageRename,
		BaseAppPageDelete,
		BaseAppBlockList,
		BaseAppBlockGet,
		BaseAppBlockGetData,
		BaseAppBlockCreate,
		BaseAppBlockUpdate,
	})
}

// withAppTokenAlias attaches "app-token" as a parse-time alias to every
// "base-token" flag. Lark's Bitable API names the resource app_token in its
// URL path (/open-apis/bitable/v1/apps/{app_token}/...), so users reading the
// vendor docs reasonably type --app-token. The alias is scoped to this domain
// on purpose: "base-token" flags exist only here, so the synonym cannot
// misroute another domain's flag. Aliases are hidden from human help and
// exported in machine metadata by the shortcut framework.
func withAppTokenAlias(shortcuts []common.Shortcut) []common.Shortcut {
	for i := range shortcuts {
		// Skip shortcuts that already own an --app-token flag (e.g. BaseApp
		// operations), so the alias does not collide with a real flag.
		hasAppToken := false
		for _, fl := range shortcuts[i].Flags {
			if fl.Name == "app-token" {
				hasAppToken = true
				break
			}
		}
		if hasAppToken {
			continue
		}
		for j := range shortcuts[i].Flags {
			fl := &shortcuts[i].Flags[j]
			if fl.Name != "base-token" {
				continue
			}
			has := false
			for _, a := range fl.Aliases {
				if a == "app-token" {
					has = true
					break
				}
			}
			if !has {
				fl.Aliases = append(fl.Aliases, "app-token")
			}
		}
	}
	return shortcuts
}
