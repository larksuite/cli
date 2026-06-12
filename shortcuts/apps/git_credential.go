// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/apps/gitcred"
	"github.com/larksuite/cli/shortcuts/common"
)

const gitCredentialIssuePath = apiBasePath + "/apps/:app_id/git_info"

// gitCredentialIssueHint is the actionable next-step attached to a failed
// Git-credential issuance. A 5xx is flagged retryable separately at the call site.
const gitCredentialIssueHint = "failed to issue the Git credential: verify --app-id is correct and you have developer access to this Miaoda app; a 5xx is a transient server error and is safe to retry"

var AppsGitCredentialInit = common.Shortcut{
	Service:     appsService,
	Command:     "+git-credential-init",
	Description: "Initialize Git credentials and a URL-scoped Git helper for a Miaoda app repository",
	Risk:        "write",
	Tips: []string{
		"Example: lark-cli apps +git-credential-init --app-id <app_id>",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app ID", Required: true},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if strings.TrimSpace(rctx.Str("app-id")) == "" {
			return appsValidationParamError("--app-id", "--app-id is required")
		}
		if err := validate.ResourceName(strings.TrimSpace(rctx.Str("app-id")), "--app-id"); err != nil {
			return appsValidationParamError("--app-id", "%v", err).WithCause(err)
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		return common.NewDryRunAPI().
			GET(gitCredentialIssuePath).
			Desc("Issue a Miaoda Git repository PAT").
			Set("mode", "api-plus-local-setup").
			Set("action", "initialize_local_git_credential").
			Set("app_id", appID).
			Set("metadata_file", appKeyPath(appID, gitcred.MetadataFilename)).
			Set("local_effects", []string{
				"save the issued PAT in the local system credential store",
				"write app-scoped git credential metadata",
				"configure a URL-scoped Git credential helper in global git config when possible",
			}).
			Params(gitCredentialIssueParams(appID))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		manager := newGitCredentialManager(appID, rctx.Factory.Keychain, runtimeIssuer{rctx: rctx})
		result, err := manager.Init(ctx, profileFromConfig(rctx.Config), appID)
		if err != nil {
			return gitCredentialLocalError("Initialize local Miaoda Git credential", err)
		}
		payload := map[string]interface{}{
			"app_id":         result.AppID,
			"repository_url": result.GitHTTPURL,
			"status":         initStatus(result),
		}
		if result.ConfigWarning != "" {
			payload["git_config_warning"] = result.ConfigWarning
		}
		rctx.OutFormat(payload, nil, func(w io.Writer) {
			title := "Git credential initialized"
			if result.Refreshed {
				title = "Git credential refreshed"
			}
			fmt.Fprintln(w, title)
			fmt.Fprintln(w)
			fmt.Fprintf(w, "App ID: %s\n", result.AppID)
			fmt.Fprintf(w, "Status: %s\n", initStatus(result))
			fmt.Fprintf(w, "Repository URL: %s\n", result.GitHTTPURL)
			if result.ConfigWarning != "" {
				fmt.Fprintln(w)
				fmt.Fprintln(w, "Git credential saved, but Git helper was not configured")
				fmt.Fprintf(w, "Reason: %s\n", result.ConfigWarning)
				fmt.Fprintf(w, "Next step: lark-cli apps +git-credential-init --app-id %s\n", result.AppID)
				return
			}
			fmt.Fprintln(w)
			fmt.Fprintln(w, "Next step:")
			fmt.Fprintf(w, "  git clone %s\n", result.GitHTTPURL)
		})
		return nil
	},
}

var AppsGitCredentialRemove = common.Shortcut{
	Service:     appsService,
	Command:     "+git-credential-remove",
	Description: "Remove local Git credentials and the URL-scoped Git helper for a Miaoda app repository",
	Risk:        "write",
	Tips: []string{
		"Example: lark-cli apps +git-credential-remove --app-id <app_id>",
	},
	Scopes:    []string{},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app ID", Required: true},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if strings.TrimSpace(rctx.Str("app-id")) == "" {
			return appsValidationParamError("--app-id", "--app-id is required")
		}
		if err := validate.ResourceName(strings.TrimSpace(rctx.Str("app-id")), "--app-id"); err != nil {
			return appsValidationParamError("--app-id", "%v", err).WithCause(err)
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		return common.NewDryRunAPI().
			Desc("Preview local Git credential cleanup (no API call; would clean up local-only state).").
			Set("mode", "local-cleanup-only").
			Set("action", "remove_local_git_credential").
			Set("app_id", appID).
			Set("metadata_file", appKeyPath(appID, gitcred.MetadataFilename)).
			Set("effects", []string{
				"read app-scoped git credential metadata",
				"remove the saved PAT from the local system credential store",
				"remove the app-scoped Git helper from global git config when present",
				"delete the local metadata record after cleanup succeeds",
			})
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		manager := newGitCredentialManager(appID, rctx.Factory.Keychain, nil)
		result, err := manager.Remove(ctx, profileFromConfig(rctx.Config), appID)
		if err != nil {
			return gitCredentialLocalError("Remove local Miaoda Git credential", err)
		}
		payload := map[string]interface{}{
			"app_id":  result.AppID,
			"removed": result.Removed,
		}
		if result.ConfigWarning != "" {
			payload["git_config_warning"] = result.ConfigWarning
		}
		rctx.OutFormat(payload, nil, func(w io.Writer) {
			if !result.Removed {
				fmt.Fprintln(w, "No local Git credential found")
				return
			}
			fmt.Fprintln(w, "Git credential removed")
			fmt.Fprintln(w)
			fmt.Fprintf(w, "App ID: %s\n", result.AppID)
			if len(result.Records) > 0 {
				fmt.Fprintf(w, "Repository URL: %s\n", result.Records[0].GitHTTPURL)
			}
			fmt.Fprintln(w, "Status: removed")
			if result.ConfigWarning != "" {
				fmt.Fprintln(w)
				fmt.Fprintln(w, "Git config cleanup warning")
				fmt.Fprintf(w, "Reason: %s\n", result.ConfigWarning)
			}
		})
		return nil
	},
}

var AppsGitCredentialList = common.Shortcut{
	Service:     appsService,
	Command:     "+git-credential-list",
	Description: "List local Git credentials for Miaoda app repositories",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +git-credential-list",
	},
	Scopes:    []string{},
	AuthTypes: []string{"user"},
	HasFormat: true,
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			Desc("Preview local Git credential listing (no API call, read-only local state).").
			Set("mode", "local-read-only").
			Set("action", "list_local_git_credentials").
			Set("storage_root", filepath.Join(core.GetConfigDir(), storageRoot)).
			Set("reads", []string{
				"scan app-scoped git credential metadata under the CLI config directory",
				"derive per-app repository URLs and local credential status from local metadata",
			})
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		records, err := listGitCredentialRecords(rctx.Factory.Keychain, time.Now)
		if err != nil {
			return gitCredentialLocalError("List local Miaoda Git credentials", err)
		}
		payload := map[string]interface{}{
			"count":       len(records),
			"credentials": gitCredentialListPayload(records),
		}
		rctx.OutFormat(payload, nil, func(w io.Writer) {
			if len(records) == 0 {
				fmt.Fprintln(w, "No Git credentials initialized")
				fmt.Fprintln(w)
				fmt.Fprintln(w, "Next step: lark-cli apps +git-credential-init --app-id <app_id>")
				return
			}
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "App ID\tRepository URL\tStatus")
			for _, record := range records {
				fmt.Fprintf(tw, "%s\t%s\t%s\n", record.AppID, record.GitHTTPURL, gitCredentialDisplayStatus(record.Status))
			}
			_ = tw.Flush()
			fmt.Fprintln(w)
			fmt.Fprintln(w, "Profile switches do not remove old URL-scoped Git helpers automatically.")
			fmt.Fprintln(w, "Cleanup: lark-cli apps +git-credential-remove --app-id <app_id>")
		})
		return nil
	},
}

// InstallOnApps attaches hidden, apps-domain commands that are not regular
// shortcuts. git-credential-helper must speak Git's stdin/stdout protocol
// directly, so it intentionally does not use the shortcut JSON envelope.
func InstallOnApps(parent *cobra.Command, f *cmdutil.Factory) {
	parent.AddCommand(newGitCredentialHelperCommand(f))
}

func newGitCredentialHelperCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "git-credential-helper get|store|erase",
		Short:  "Git credential helper for Miaoda app repositories",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			appID, _ := cmd.Flags().GetString("app-id")
			return runGitCredentialHelper(cmd.Context(), f, strings.TrimSpace(appID), args[0])
		},
	}
	cmd.Flags().String("app-id", "", "Miaoda app ID")
	_ = cmd.Flags().MarkHidden("app-id")
	return cmd
}

type runtimeIssuer struct {
	rctx *common.RuntimeContext
}

func (i runtimeIssuer) Issue(ctx context.Context, appID string, profile gitcred.ProfileContext) (*gitcred.IssuedCredential, error) {
	resp, err := i.rctx.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodGet,
		ApiPath:    issuePath(appID),
	})
	data, err := parseIssueCredentialData(resp, err, i.rctx.APIClassifyContext())
	if err != nil {
		return nil, err
	}
	return issuedFromData(appID, data)
}

type factoryIssuer struct {
	f *cmdutil.Factory
}

func (i factoryIssuer) Issue(ctx context.Context, appID string, profile gitcred.ProfileContext) (*gitcred.IssuedCredential, error) {
	cfg, err := i.f.Config()
	if err != nil {
		return nil, err
	}
	if cfg.UserOpenId == "" {
		return nil, errs.NewAuthenticationError(errs.SubtypeTokenMissing, "not logged in").
			WithHint("run `lark-cli auth login --scope \"spark:app:read\"`")
	}
	ac, err := i.f.NewAPIClientWithConfig(cfg)
	if err != nil {
		return nil, err
	}
	req := &larkcore.ApiReq{
		HttpMethod: http.MethodGet,
		ApiPath:    issuePath(appID),
	}
	resp, err := ac.DoSDKRequest(ctx, req, core.AsUser)
	data, err := parseIssueCredentialData(resp, err, errclass.ClassifyContext{
		Brand:    string(cfg.Brand),
		AppID:    cfg.AppID,
		Identity: string(core.AsUser),
	})
	if err != nil {
		return nil, err
	}
	return issuedFromData(appID, data)
}

func runGitCredentialHelper(ctx context.Context, f *cmdutil.Factory, appID, action string) error {
	if f == nil || f.IOStreams == nil {
		return nil
	}
	if appID == "" {
		fmt.Fprintln(f.IOStreams.ErrOut, "Git credential unavailable: missing app_id; rerun lark-cli apps +git-credential-init --app-id <app_id>")
		return nil
	}
	manager := newGitCredentialManager(appID, f.Keychain, factoryIssuer{f: f})
	switch action {
	case "get":
		input, err := gitcred.ParseCredentialInput(f.IOStreams.In)
		if err != nil {
			fmt.Fprintf(f.IOStreams.ErrOut, "Git credential unavailable: %s\n", err)
			return nil
		}
		cfg, err := f.Config()
		if err != nil {
			fmt.Fprintf(f.IOStreams.ErrOut, "Git credential unavailable: %s\n", err)
			return nil
		}
		return manager.Get(ctx, input, profileFromConfig(cfg), f.IOStreams.Out, f.IOStreams.ErrOut)
	case "store":
		return manager.StoreCredential(f.IOStreams.In)
	case "erase":
		return manager.Erase(f.IOStreams.In)
	default:
		fmt.Fprintf(f.IOStreams.ErrOut, "unsupported git credential action %q\n", action)
		return nil
	}
}

func newGitCredentialManager(appID string, kc keychain.KeychainAccess, issuer gitcred.Issuer) *gitcred.Manager {
	storage := gitCredentialAppStorage{}
	return gitcred.NewManager(gitcred.NewAppStore(appID, storage), gitcred.NewSecretStore(kc), gitcred.GlobalGitConfig{}, issuer)
}

func listGitCredentialRecords(kc keychain.KeychainAccess, now func() time.Time) ([]gitcred.ListRecord, error) {
	storage := gitCredentialAppStorage{}
	appIDs, err := storage.ListAppIDs()
	if err != nil {
		return nil, err
	}
	records := make([]gitcred.ListRecord, 0, len(appIDs))
	for _, appID := range appIDs {
		manager := newGitCredentialManager(appID, kc, nil)
		manager.Now = now
		result, err := manager.List()
		if err != nil {
			return nil, err
		}
		records = append(records, result.Records...)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].AppID == records[j].AppID {
			return records[i].GitHTTPURL < records[j].GitHTTPURL
		}
		return records[i].AppID < records[j].AppID
	})
	return records, nil
}

func gitCredentialListPayload(records []gitcred.ListRecord) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(records))
	for _, record := range records {
		out = append(out, map[string]interface{}{
			"app_id":         record.AppID,
			"repository_url": record.GitHTTPURL,
			"status":         gitCredentialDisplayStatus(record.Status),
		})
	}
	return out
}

func gitCredentialDisplayStatus(status string) string {
	if status == gitcred.ListStatusExpired {
		return "refresh_required"
	}
	return status
}

func profileFromConfig(cfg *core.CliConfig) gitcred.ProfileContext {
	if cfg == nil {
		return gitcred.ProfileContext{}
	}
	return gitcred.ProfileContext{
		Profile:      cfg.ProfileName,
		ProfileAppID: cfg.AppID,
		UserOpenID:   cfg.UserOpenId,
	}
}

func issuePath(appID string) string {
	return strings.Replace(gitCredentialIssuePath, ":app_id", url.PathEscape(strings.TrimSpace(appID)), 1)
}

func gitCredentialIssueParams(appID string) map[string]interface{} {
	return map[string]interface{}{"app_id": strings.TrimSpace(appID)}
}

func initStatus(result *gitcred.InitResult) string {
	if result != nil && result.Refreshed {
		return "refreshed"
	}
	return "initialized"
}

func gitCredentialLocalError(action string, err error) error {
	if err == nil {
		return nil
	}
	// Typed errors pass through unchanged; everything the apps domain and the
	// shared runtime produce is typed, so there is no legacy envelope to forward.
	if _, ok := errs.UnwrapTypedError(err); ok {
		return err
	}
	return &errs.ConfigError{Problem: errs.Problem{
		Category: errs.CategoryConfig,
		Subtype:  errs.SubtypeInvalidConfig,
		Message:  fmt.Sprintf("%s: %s", action, err),
		Hint:     "retry the command; if the local Git credential state is damaged, rerun `lark-cli apps +git-credential-init --app-id <app_id>` or remove the app credential again",
	}, Cause: err}
}

func issuedFromData(appID string, data map[string]interface{}) (*gitcred.IssuedCredential, error) {
	source := data
	for _, key := range []string{"credential", "git_credential", "gitInfo", "git_info"} {
		if nested, ok := data[key].(map[string]interface{}); ok {
			source = nested
			break
		}
	}
	issued := &gitcred.IssuedCredential{
		AppID:      firstString(source, "app_id", appID),
		GitHTTPURL: firstString(source, "gitURL", "GitURL", "GitUrl", "gitUrl", "git_url", "git_http_url", "repository_url"),
		Username:   firstString(source, "username"),
		PAT:        firstString(source, "token", "Token", "pat", "password"),
		ExpiresAt:  firstInt64(source, "expiredTime", "ExpiredTime", "expired_time", "expires_at"),
	}
	if issued.AppID == "" {
		issued.AppID = appID
	}
	if issued.GitHTTPURL == "" {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "Issue Miaoda Git credential: response missing gitURL")
	}
	if issued.PAT == "" {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "Issue Miaoda Git credential: response missing token")
	}
	return issued, nil
}

// parseIssueCredentialData turns the git-credential issue response into the
// credential data map. A standard Lark envelope (top-level "code") and any
// HTTP error status route through the shared response classifier, so generic
// codes (missing scope, app not authorized) and 5xx statuses keep their
// canonical category/subtype/retryable classification. The endpoint's
// non-standard success shapes — direct git info or a BaseResp wrapper — are
// handled locally.
func parseIssueCredentialData(resp *larkcore.ApiResp, err error, cc errclass.ClassifyContext) (map[string]any, error) {
	if err != nil {
		return nil, client.WrapDoAPIError(err)
	}
	detail := logIDDetail(resp)
	if resp == nil || len(resp.RawBody) == 0 {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"Issue Miaoda Git credential: empty response body")
	}
	var result map[string]any
	jsonErr := json.Unmarshal(resp.RawBody, &result)
	_, hasCode := result["code"]
	if jsonErr != nil || hasCode || resp.StatusCode >= http.StatusBadRequest {
		data, cerr := common.ClassifyAPIResponseWith(resp, cc)
		if cerr != nil {
			return nil, withAppsHint(cerr, gitCredentialIssueHint)
		}
		if data != nil {
			result = data
		}
		// data == nil: a code==0 envelope whose fields sit beside "code" instead
		// of under "data" — keep the locally-unmarshalled top-level object.
	} else if err := checkGitInfoBaseResp(result, logIDString(resp)); err != nil {
		return nil, err
	}
	if detail != nil {
		if result == nil {
			result = map[string]any{}
		}
		for k, v := range detail {
			result[k] = v
		}
	}
	return result, nil
}

func checkGitInfoBaseResp(result map[string]any, logID string) error {
	for _, key := range []string{"BaseResp", "baseResp", "base_resp"} {
		baseResp, ok := result[key].(map[string]any)
		if !ok {
			continue
		}
		code := firstInt64(baseResp, "StatusCode", "statusCode", "status_code")
		if code == 0 {
			return nil
		}
		message := firstString(baseResp, "StatusMessage", "statusMessage", "status_message")
		if message == "" {
			message = "Git credential API returned non-zero BaseResp status"
		}
		baseErr := errs.NewAPIError(errs.SubtypeUnknown, "Issue Miaoda Git credential: %s", message).WithCode(int(code))
		if logID != "" {
			baseErr = baseErr.WithLogID(logID)
		}
		return baseErr
	}
	return nil
}

func logIDDetail(resp *larkcore.ApiResp) map[string]any {
	logID := logIDString(resp)
	if logID == "" {
		return nil
	}
	return map[string]any{"log_id": logID}
}

func logIDString(resp *larkcore.ApiResp) string {
	if resp == nil {
		return ""
	}
	return resp.Header.Get("x-tt-logid")
}

func firstString(data map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if v, ok := data[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func firstInt64(data map[string]interface{}, keys ...string) int64 {
	for _, key := range keys {
		switch v := data[key].(type) {
		case int64:
			return v
		case int:
			return int64(v)
		case float64:
			return int64(v)
		case json.Number:
			n, _ := v.Int64()
			return n
		case string:
			n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			return n
		}
	}
	return 0
}
