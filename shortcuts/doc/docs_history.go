// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

type docsHistoryListSpec struct {
	Doc       documentRef
	PageSize  int
	PageToken string
}

type docsHistoryRevertSpec struct {
	Doc              documentRef
	HistoryVersionID string
	WaitTimeoutMs    int
}

type docsHistoryRevertStatusSpec struct {
	Doc    documentRef
	TaskID string
}

func parseDocsHistoryDocRef(raw, shortcut string) (documentRef, error) {
	ref, err := parseDocumentRef(raw)
	if err != nil {
		return documentRef{}, err
	}
	if ref.Kind == "doc" {
		return documentRef{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "docs %s only supports docx documents; use a docx token/URL or a wiki URL that resolves to docx", shortcut).WithParam("--doc")
	}
	return ref, nil
}

func validateDocsHistoryPageSize(pageSize int) error {
	if pageSize < 1 || pageSize > 20 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --page-size %d: must be between 1 and 20", pageSize).WithParam("--page-size")
	}
	return nil
}

func validateDocsHistoryVersionID(historyVersionID string) error {
	version, err := strconv.ParseInt(strings.TrimSpace(historyVersionID), 10, 64)
	if err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--history-version-id must be a positive integer string returned by docs +history-list").WithParam("--history-version-id").WithCause(err)
	}
	if version <= 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--history-version-id must be a positive integer string returned by docs +history-list").WithParam("--history-version-id")
	}
	return nil
}

func validateDocsHistoryWaitTimeout(timeoutMs int) error {
	if timeoutMs < 0 || timeoutMs > 30000 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --wait-timeout-ms %d: must be between 0 and 30000", timeoutMs).WithParam("--wait-timeout-ms")
	}
	return nil
}

func docsHistoryListParams(spec docsHistoryListSpec) map[string]interface{} {
	params := map[string]interface{}{
		"page_size": spec.PageSize,
	}
	if spec.PageToken != "" {
		params["page_token"] = spec.PageToken
	}
	return params
}

func docsHistoryRevertBody(spec docsHistoryRevertSpec) map[string]interface{} {
	return map[string]interface{}{
		"history_version_id": spec.HistoryVersionID,
		"wait_timeout_ms":    spec.WaitTimeoutMs,
	}
}

func docsHistoryStatusParams(spec docsHistoryRevertStatusSpec) map[string]interface{} {
	return map[string]interface{}{
		"task_id": spec.TaskID,
	}
}

func docsHistoryAPIPath(docToken, suffix string) string {
	return fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s/%s", validate.EncodePathSegment(docToken), suffix)
}

var DocsHistoryList = common.Shortcut{
	Service:     "docs",
	Command:     "+history-list",
	Description: "List Lark document history versions",
	Risk:        "read",
	Scopes:      []string{"docx:document:readonly"},
	AuthTypes:   []string{"user", "bot"},
	PostMount:   installDocsShortcutHelp("+history-list"),
	Flags: []common.Flag{
		{Name: "doc", Desc: "document URL or token", Required: true},
		{Name: "page-size", Type: "int", Default: "20", Desc: "history entries to return, range 1-20"},
		{Name: "page-token", Desc: "pagination token from the previous page's page_token"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := parseDocsHistoryDocRef(runtime.Str("doc"), "+history-list"); err != nil {
			return err
		}
		return validateDocsHistoryPageSize(runtime.Int("page-size"))
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		ref, _ := parseDocsHistoryDocRef(runtime.Str("doc"), "+history-list")
		spec := docsHistoryListSpec{
			Doc:       ref,
			PageSize:  runtime.Int("page-size"),
			PageToken: strings.TrimSpace(runtime.Str("page-token")),
		}
		return common.NewDryRunAPI().
			Desc("OpenAPI: list document history versions").
			GET("/open-apis/docs_ai/v1/documents/:document_id/histories").
			Set("document_id", spec.Doc.Token).
			Params(docsHistoryListParams(spec))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		ref, _ := parseDocsHistoryDocRef(runtime.Str("doc"), "+history-list")
		spec := docsHistoryListSpec{
			Doc:       ref,
			PageSize:  runtime.Int("page-size"),
			PageToken: strings.TrimSpace(runtime.Str("page-token")),
		}

		data, err := runtime.CallAPITyped(
			http.MethodGet,
			docsHistoryAPIPath(spec.Doc.Token, "histories"),
			docsHistoryListParams(spec),
			nil,
		)
		if err != nil {
			return err
		}
		runtime.OutRaw(data, nil)
		return nil
	},
}

var DocsHistoryRevert = common.Shortcut{
	Service:     "docs",
	Command:     "+history-revert",
	Description: "Revert a Lark document to a historical version",
	Risk:        "write",
	Scopes:      []string{"docx:document:write_only", "docx:document:readonly"},
	AuthTypes:   []string{"user", "bot"},
	PostMount:   installDocsShortcutHelp("+history-revert"),
	Flags: []common.Flag{
		{Name: "doc", Desc: "document URL or token", Required: true},
		{Name: "history-version-id", Desc: "history_version_id from docs +history-list to revert to", Required: true},
		{Name: "wait-timeout-ms", Type: "int", Default: "30000", Desc: "milliseconds to wait for revert completion before returning, range 0-30000"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := parseDocsHistoryDocRef(runtime.Str("doc"), "+history-revert"); err != nil {
			return err
		}
		if err := validateDocsHistoryVersionID(runtime.Str("history-version-id")); err != nil {
			return err
		}
		return validateDocsHistoryWaitTimeout(runtime.Int("wait-timeout-ms"))
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		ref, _ := parseDocsHistoryDocRef(runtime.Str("doc"), "+history-revert")
		spec := docsHistoryRevertSpec{
			Doc:              ref,
			HistoryVersionID: strings.TrimSpace(runtime.Str("history-version-id")),
			WaitTimeoutMs:    runtime.Int("wait-timeout-ms"),
		}
		return common.NewDryRunAPI().
			Desc("OpenAPI: revert document history").
			POST("/open-apis/docs_ai/v1/documents/:document_id/history/revert").
			Set("document_id", spec.Doc.Token).
			Body(docsHistoryRevertBody(spec))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		ref, _ := parseDocsHistoryDocRef(runtime.Str("doc"), "+history-revert")
		spec := docsHistoryRevertSpec{
			Doc:              ref,
			HistoryVersionID: strings.TrimSpace(runtime.Str("history-version-id")),
			WaitTimeoutMs:    runtime.Int("wait-timeout-ms"),
		}

		data, err := runtime.CallAPITyped(
			http.MethodPost,
			docsHistoryAPIPath(spec.Doc.Token, "history/revert"),
			nil,
			docsHistoryRevertBody(spec),
		)
		if err != nil {
			return err
		}
		runtime.OutRaw(data, nil)
		return nil
	},
}

var DocsHistoryRevertStatus = common.Shortcut{
	Service:     "docs",
	Command:     "+history-revert-status",
	Description: "Get Lark document history revert task status",
	Risk:        "read",
	Scopes:      []string{"docx:document:readonly"},
	AuthTypes:   []string{"user", "bot"},
	PostMount:   installDocsShortcutHelp("+history-revert-status"),
	Flags: []common.Flag{
		{Name: "doc", Desc: "document URL or token", Required: true},
		{Name: "task-id", Desc: "task_id returned by docs +history-revert", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := parseDocsHistoryDocRef(runtime.Str("doc"), "+history-revert-status"); err != nil {
			return err
		}
		if strings.TrimSpace(runtime.Str("task-id")) == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--task-id is required").WithParam("--task-id")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		ref, _ := parseDocsHistoryDocRef(runtime.Str("doc"), "+history-revert-status")
		spec := docsHistoryRevertStatusSpec{
			Doc:    ref,
			TaskID: strings.TrimSpace(runtime.Str("task-id")),
		}
		return common.NewDryRunAPI().
			Desc("OpenAPI: get document history revert status").
			GET("/open-apis/docs_ai/v1/documents/:document_id/history/revert_status").
			Set("document_id", spec.Doc.Token).
			Params(docsHistoryStatusParams(spec))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		ref, _ := parseDocsHistoryDocRef(runtime.Str("doc"), "+history-revert-status")
		spec := docsHistoryRevertStatusSpec{
			Doc:    ref,
			TaskID: strings.TrimSpace(runtime.Str("task-id")),
		}

		data, err := runtime.CallAPITyped(
			http.MethodGet,
			docsHistoryAPIPath(spec.Doc.Token, "history/revert_status"),
			docsHistoryStatusParams(spec),
			nil,
		)
		if err != nil {
			return err
		}
		runtime.OutRaw(data, nil)
		return nil
	},
}
