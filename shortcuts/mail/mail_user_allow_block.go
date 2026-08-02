// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/cmdmeta"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
)

const (
	userAllowBlockDefaultPageSize = 20
	userAllowBlockMaxPageSize     = 100

	userAllowBlockScopeRead   = "mail:user_mailbox.message:readonly"
	userAllowBlockScopeModify = "mail:user_mailbox.message:modify"
)

type userAllowBlockOptions struct {
	Factory   *cmdutil.Factory
	Command   *cobra.Command
	Ctx       context.Context
	MailboxID string
	Kind      string
	PageSize  int
	Cursor    string
	Query     string
	Records   []string
	Allow     bool
	Block     bool
	Config    *core.CliConfig
	Format    string
	JqExpr    string
	DryRun    bool
	As        core.Identity
}

func installUserAllowBlockCommands(svc *cobra.Command, f *cmdutil.Factory) {
	if svc == nil || f == nil || findChildCommand(svc, "user-allow-block") != nil {
		return
	}
	root := &cobra.Command{
		Use:   "user-allow-block",
		Short: "Manage user mail allow/block senders",
		Long: strings.TrimSpace(`Manage the current user's mail allow/block sender lists.

Use list/search/get for read-only inspection and add/delete for updates. Records can be either full email addresses or domains. The command wraps mail.user_mailbox.allow_senders and mail.user_mailbox.blocked_senders OpenAPI methods.`),
	}
	cmdmeta.SetSource(root, cmdmeta.SourceShortcut, false)
	cmdmeta.SetAffordanceRef(root, "mail", "user-allow-block")
	cmdutil.SetSupportedIdentities(root, []string{"user"})
	cmdutil.SetRisk(root, cmdutil.RiskRead)
	addUserAllowBlockChildren(root, f)
	svc.AddCommand(root)
}

func addUserAllowBlockChildren(root *cobra.Command, f *cmdutil.Factory) {
	root.AddCommand(
		newUserAllowBlockListCmd(f),
		newUserAllowBlockSearchCmd(f),
		newUserAllowBlockAddCmd(f),
		newUserAllowBlockDeleteCmd(f),
		newUserAllowBlockGetCmd(f),
	)
}

func newUserAllowBlockListCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &userAllowBlockOptions{Factory: f, Kind: "all", PageSize: userAllowBlockDefaultPageSize}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List allow/block senders",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Command = cmd
			opts.Ctx = cmd.Context()
			return runUserAllowBlockRead(opts)
		},
	}
	addUserAllowBlockCommonFlags(cmd, opts)
	addUserAllowBlockTypeFlag(cmd, opts, true)
	addUserAllowBlockPageFlags(cmd, opts)
	finalizeUserAllowBlockCommand(cmd, cmdutil.RiskRead)
	return cmd
}

func newUserAllowBlockSearchCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &userAllowBlockOptions{Factory: f, Kind: "all", PageSize: userAllowBlockDefaultPageSize}
	cmd := &cobra.Command{
		Use:   "search <keyword>",
		Short: "Search allow/block senders",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return mailValidationParamError("keyword", "search requires exactly one keyword")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Command = cmd
			opts.Ctx = cmd.Context()
			opts.Query = strings.TrimSpace(args[0])
			return runUserAllowBlockRead(opts)
		},
	}
	addUserAllowBlockCommonFlags(cmd, opts)
	addUserAllowBlockTypeFlag(cmd, opts, true)
	addUserAllowBlockPageFlags(cmd, opts)
	finalizeUserAllowBlockCommand(cmd, cmdutil.RiskRead)
	return cmd
}

func newUserAllowBlockGetCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &userAllowBlockOptions{Factory: f, PageSize: 100}
	cmd := &cobra.Command{
		Use:   "get <address-or-domain>",
		Short: "Get one allow/block sender",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return mailValidationParamError("record", "get requires exactly one address or domain")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Command = cmd
			opts.Ctx = cmd.Context()
			opts.Query = strings.TrimSpace(args[0])
			opts.PageSize = userAllowBlockMaxPageSize
			return runUserAllowBlockGet(opts)
		},
	}
	addUserAllowBlockCommonFlags(cmd, opts)
	cmd.Flags().BoolVar(&opts.Allow, "allow", false, "Look up the allow list")
	cmd.Flags().BoolVar(&opts.Block, "block", false, "Look up the block list")
	finalizeUserAllowBlockCommand(cmd, cmdutil.RiskRead)
	return cmd
}

func newUserAllowBlockAddCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &userAllowBlockOptions{Factory: f}
	cmd := &cobra.Command{
		Use:   "add <address-or-domain> [address-or-domain...]",
		Short: "Add senders to the allow or block list",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return mailValidationParamError("record", "add requires at least one address or domain")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Command = cmd
			opts.Ctx = cmd.Context()
			opts.Records = args
			return runUserAllowBlockWrite(opts, "add")
		},
	}
	addUserAllowBlockCommonFlags(cmd, opts)
	cmd.Flags().BoolVar(&opts.Allow, "allow", false, "Add to the allow list")
	cmd.Flags().BoolVar(&opts.Block, "block", false, "Add to the block list")
	finalizeUserAllowBlockCommand(cmd, cmdutil.RiskWrite)
	return cmd
}

func newUserAllowBlockDeleteCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &userAllowBlockOptions{Factory: f}
	cmd := &cobra.Command{
		Use:     "delete <address-or-domain> [address-or-domain...]",
		Aliases: []string{"remove"},
		Short:   "Delete senders from the allow or block list",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return mailValidationParamError("record", "delete requires at least one address or domain")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Command = cmd
			opts.Ctx = cmd.Context()
			opts.Records = args
			return runUserAllowBlockWrite(opts, "delete")
		},
	}
	addUserAllowBlockCommonFlags(cmd, opts)
	cmd.Flags().BoolVar(&opts.Allow, "allow", false, "Delete from the allow list")
	cmd.Flags().BoolVar(&opts.Block, "block", false, "Delete from the block list")
	finalizeUserAllowBlockCommand(cmd, cmdutil.RiskWrite)
	return cmd
}

func addUserAllowBlockCommonFlags(cmd *cobra.Command, opts *userAllowBlockOptions) {
	cmd.Flags().StringVar(&opts.MailboxID, "mailbox", "me", "User mailbox ID or email address (default: me)")
	cmd.Flags().StringVar(&opts.Format, "format", "json", "output format: json|ndjson|table|csv")
	cmd.Flags().Bool("json", false, "shorthand for --format json")
	cmd.Flags().StringVarP(&opts.JqExpr, "jq", "q", "", "jq expression to filter JSON output")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "print request without executing")
	cmdutil.AddShortcutIdentityFlag(context.Background(), cmd, opts.Factory, []string{"user"})
}

func addUserAllowBlockTypeFlag(cmd *cobra.Command, opts *userAllowBlockOptions, all bool) {
	enum := []string{"allow", "block"}
	desc := "List type: allow or block"
	if all {
		enum = append(enum, "all")
		desc = "List type: allow, block, or all"
	}
	cmd.Flags().StringVar(&opts.Kind, "type", opts.Kind, desc)
	_ = cmd.RegisterFlagCompletionFunc("type", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return enum, cobra.ShellCompDirectiveNoFileComp
	})
}

func addUserAllowBlockPageFlags(cmd *cobra.Command, opts *userAllowBlockOptions) {
	cmd.Flags().IntVar(&opts.PageSize, "page-size", userAllowBlockDefaultPageSize, "Page size (1-100)")
	cmd.Flags().StringVar(&opts.Cursor, "cursor", "", "Page cursor from a previous response")
}

func finalizeUserAllowBlockCommand(cmd *cobra.Command, risk string) {
	cmdutil.SetSupportedIdentities(cmd, []string{"user"})
	cmdutil.SetRisk(cmd, risk)
	cmdmeta.SetSource(cmd, cmdmeta.SourceShortcut, false)
	cmdmeta.SetAffordanceRef(cmd, "mail", "user-allow-block."+cmd.Name())
	cmd.SilenceUsage = true
}

func runUserAllowBlockRead(opts *userAllowBlockOptions) error {
	if err := opts.prepare(userAllowBlockScopeRead); err != nil {
		return err
	}
	if opts.Query != "" && strings.TrimSpace(opts.Query) == "" {
		return mailValidationParamError("keyword", "keyword must not be empty")
	}
	if err := validateUserAllowBlockType(opts.Kind, true); err != nil {
		return err
	}
	if opts.PageSize <= 0 || opts.PageSize > userAllowBlockMaxPageSize {
		return mailValidationParamError("--page-size", "--page-size must be between 1 and 100")
	}

	if opts.Kind == "all" {
		return runUserAllowBlockCombinedSearch(opts)
	}
	return opts.dispatch(userAllowBlockResource(opts.Kind), "list", "GET", nil, userAllowBlockListParams(opts))
}

func runUserAllowBlockGet(opts *userAllowBlockOptions) error {
	if err := opts.prepare(userAllowBlockScopeRead); err != nil {
		return err
	}
	record, err := normalizeUserAllowBlockRecord(opts.Query, "record")
	if err != nil {
		return err
	}
	kinds, err := opts.selectedKindsForGet()
	if err != nil {
		return err
	}
	result := map[string]interface{}{
		"record": record,
		"found":  false,
	}
	for _, kind := range kinds {
		params := map[string]interface{}{
			"keyword":   record,
			"page_size": userAllowBlockMaxPageSize,
		}
		data, err := opts.call(userAllowBlockResource(kind), "list", "GET", nil, params)
		if err != nil {
			return mailDecorateAllowBlockProblem(err)
		}
		if match := findUserAllowBlockRecord(data, record); match != nil {
			result["found"] = true
			result["type"] = kind
			result["item"] = match
			break
		}
	}
	return opts.emit(result)
}

func runUserAllowBlockWrite(opts *userAllowBlockOptions, action string) error {
	scope := userAllowBlockScopeModify
	if err := opts.prepare(scope); err != nil {
		return err
	}
	kind, err := opts.selectedKindForWrite()
	if err != nil {
		return err
	}
	records, err := normalizeUserAllowBlockRecords(opts.Records)
	if err != nil {
		return err
	}
	if len(records) > userAllowBlockMaxPageSize {
		return mailValidationParamError("record", "%s accepts at most 100 records per request (got %d)", action, len(records))
	}

	resource := userAllowBlockResource(kind)
	method := "batch_create"
	body := map[string]interface{}{"items": userAllowBlockItems(records)}
	if action == "delete" {
		method = "batch_remove"
		body = map[string]interface{}{"senders": records}
	}
	data, err := opts.call(resource, method, "POST", body, nil)
	if err != nil {
		return mailDecorateAllowBlockProblem(err)
	}
	out := map[string]interface{}{
		"mailbox_id": opts.mailboxID(),
		"type":       kind,
		"action":     action,
		"records":    records,
		"total":      len(records),
		"result":     data,
	}
	return opts.emit(out)
}

func runUserAllowBlockCombinedSearch(opts *userAllowBlockOptions) error {
	allowData, err := opts.call("allow_senders", "list", "GET", nil, userAllowBlockListParams(opts))
	if err != nil {
		return mailDecorateAllowBlockProblem(err)
	}
	blockData, err := opts.call("blocked_senders", "list", "GET", nil, userAllowBlockListParams(opts))
	if err != nil {
		return mailDecorateAllowBlockProblem(err)
	}
	out := map[string]interface{}{
		"mailbox_id":       opts.mailboxID(),
		"type":             "all",
		"keyword":          opts.Query,
		"items":            mergeUserAllowBlockItems(allowData, blockData),
		"allow_senders":    extractUserAllowBlockItems(allowData),
		"blocked_senders":  extractUserAllowBlockItems(blockData),
		"allow_page_token": strVal(allowData["page_token"]),
		"block_page_token": strVal(blockData["page_token"]),
		"allow_has_more":   userAllowBlockBoolVal(allowData["has_more"]),
		"block_has_more":   userAllowBlockBoolVal(blockData["has_more"]),
	}
	return opts.emit(out)
}

func (opts *userAllowBlockOptions) prepare(scopes ...string) error {
	if opts.Factory == nil {
		return mailInvalidResponseError("mail user-allow-block command is missing factory")
	}
	cfg, err := opts.Factory.Config()
	if err != nil {
		return err
	}
	opts.Config = cfg
	asFlag, _ := opts.Command.Flags().GetString("as")
	opts.As = core.Identity(asFlag)
	opts.As = opts.Factory.ResolveAs(opts.Ctx, opts.Command, opts.As)
	if err := opts.Factory.CheckStrictMode(opts.Ctx, opts.As); err != nil {
		return err
	}
	if err := opts.Factory.CheckIdentity(opts.As, []string{"user"}); err != nil {
		return err
	}
	if opts.As.IsBot() {
		return mailValidationParamError("--as", "mail user-allow-block only supports --as user")
	}
	if err := opts.checkScopes(scopes); err != nil {
		return err
	}
	if err := output.ValidateJqFlags(opts.JqExpr, "", opts.Format); err != nil {
		return err
	}
	if opts.Command.Flags().Changed("json") {
		opts.Format = "json"
	}
	return nil
}

func (opts *userAllowBlockOptions) config() *core.CliConfig {
	return opts.Config
}

func (opts *userAllowBlockOptions) checkScopes(scopes []string) error {
	if len(scopes) == 0 {
		return nil
	}
	cfg := opts.config()
	if cfg == nil || opts.Factory.Credential == nil {
		return nil
	}
	result, err := opts.Factory.Credential.ResolveToken(opts.Ctx, credential.NewTokenSpec(opts.As, cfg.AppID))
	if err != nil || result == nil || result.Scopes == "" {
		return nil
	}
	if missing := auth.MissingScopes(result.Scopes, scopes); len(missing) > 0 {
		return errs.NewPermissionError(errs.SubtypeMissingScope,
			"missing required scope(s): %s", strings.Join(missing, ", ")).
			WithIdentity(string(opts.As)).
			WithMissingScopes(missing...).
			WithHint("run `lark-cli auth login --scope \"%s\"` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login.", strings.Join(missing, " "))
	}
	return nil
}

func (opts *userAllowBlockOptions) mailboxID() string {
	if strings.TrimSpace(opts.MailboxID) == "" {
		return "me"
	}
	return strings.TrimSpace(opts.MailboxID)
}

func (opts *userAllowBlockOptions) selectedKindForWrite() (string, error) {
	if opts.Allow == opts.Block {
		return "", mailValidationError("exactly one of --allow or --block is required").
			WithParams(
				mailInvalidParam("--allow", "choose allow or block"),
				mailInvalidParam("--block", "choose allow or block"),
			)
	}
	if opts.Allow {
		return "allow", nil
	}
	return "block", nil
}

func (opts *userAllowBlockOptions) selectedKindsForGet() ([]string, error) {
	if opts.Allow && opts.Block {
		return nil, mailValidationError("--allow and --block are mutually exclusive for get")
	}
	if opts.Allow {
		return []string{"allow"}, nil
	}
	if opts.Block {
		return []string{"block"}, nil
	}
	return []string{"allow", "block"}, nil
}

func (opts *userAllowBlockOptions) call(resource, method, httpMethod string, body interface{}, params map[string]interface{}) (map[string]interface{}, error) {
	if opts.DryRun {
		return opts.dryRun(resource, method, httpMethod, body, params), nil
	}
	ac, err := opts.Factory.NewAPIClientWithConfig(opts.config())
	if err != nil {
		return nil, err
	}
	resp, err := ac.DoAPI(opts.Ctx, client.RawApiRequest{
		Method: httpMethod,
		URL:    userAllowBlockPath(opts.mailboxID(), resource, method),
		Params: params,
		Data:   body,
		As:     opts.As,
	})
	if err != nil {
		return nil, err
	}
	result, err := client.ParseJSONResponse(resp)
	if err != nil {
		return nil, client.WrapJSONResponseParseError(err, resp.RawBody)
	}
	if err := ac.CheckResponse(result, opts.As); err != nil {
		return nil, err
	}
	data, ok := result.(map[string]interface{})
	if !ok {
		return nil, mailInvalidResponseError("mail user-allow-block API returned non-object response")
	}
	if nested, ok := data["data"].(map[string]interface{}); ok {
		return nested, nil
	}
	return data, nil
}

func (opts *userAllowBlockOptions) dispatch(resource, method, httpMethod string, body interface{}, params map[string]interface{}) error {
	if opts.DryRun {
		return opts.emit(opts.dryRun(resource, method, httpMethod, body, params))
	}
	ac, err := opts.Factory.NewAPIClientWithConfig(opts.config())
	if err != nil {
		return err
	}
	resp, err := ac.DoAPI(opts.Ctx, client.RawApiRequest{
		Method: httpMethod,
		URL:    userAllowBlockPath(opts.mailboxID(), resource, method),
		Params: params,
		Data:   body,
		As:     opts.As,
	})
	if err != nil {
		return err
	}
	format, ok := output.ParseFormat(opts.Format)
	if !ok {
		fmt.Fprintf(opts.Factory.IOStreams.ErrOut, "warning: unknown format %q, falling back to json\n", opts.Format)
	}
	err = client.HandleResponse(resp, client.ResponseOptions{
		Format:      format,
		JqExpr:      opts.JqExpr,
		Out:         opts.Factory.IOStreams.Out,
		ErrOut:      opts.Factory.IOStreams.ErrOut,
		FileIO:      opts.Factory.ResolveFileIO(opts.Ctx),
		CommandPath: opts.Command.CommandPath(),
		Identity:    opts.As,
		CheckError:  ac.CheckResponse,
	})
	return mailDecorateAllowBlockProblem(err)
}

func (opts *userAllowBlockOptions) dryRun(resource, method, httpMethod string, body interface{}, params map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"dry_run": true,
		"api": []map[string]interface{}{{
			"method": httpMethod,
			"url":    userAllowBlockPath(opts.mailboxID(), resource, method),
			"params": params,
			"body":   body,
		}},
	}
}

func (opts *userAllowBlockOptions) emit(data interface{}) error {
	format, ok := output.ParseFormat(opts.Format)
	if !ok {
		fmt.Fprintf(opts.Factory.IOStreams.ErrOut, "warning: unknown format %q, falling back to json\n", opts.Format)
	}
	if opts.JqExpr != "" || format == output.FormatJSON {
		return output.WriteSuccessEnvelope(data, output.SuccessEnvelopeOptions{
			CommandPath: opts.Command.CommandPath(),
			Identity:    string(opts.As),
			JqExpr:      opts.JqExpr,
			Out:         opts.Factory.IOStreams.Out,
			ErrOut:      opts.Factory.IOStreams.ErrOut,
		})
	}
	scanResult := output.ScanForSafety(opts.Command.CommandPath(), data, opts.Factory.IOStreams.ErrOut)
	if scanResult.Blocked {
		return scanResult.BlockErr
	}
	if scanResult.Alert != nil {
		output.WriteAlertWarning(opts.Factory.IOStreams.ErrOut, scanResult.Alert)
	}
	output.FormatValue(opts.Factory.IOStreams.Out, data, format)
	return nil
}

func userAllowBlockListParams(opts *userAllowBlockOptions) map[string]interface{} {
	params := map[string]interface{}{"page_size": opts.PageSize}
	if opts.Query != "" {
		params["keyword"] = opts.Query
	}
	if opts.Cursor != "" {
		params["page_token"] = opts.Cursor
	}
	return params
}

func userAllowBlockPath(mailboxID, resource, method string) string {
	base := mailboxPath(mailboxID, resource)
	if method == "list" {
		return base
	}
	return base + "/" + validate.EncodePathSegment(method)
}

func userAllowBlockResource(kind string) string {
	if kind == "allow" {
		return "allow_senders"
	}
	return "blocked_senders"
}

func validateUserAllowBlockType(kind string, allowAll bool) error {
	switch kind {
	case "allow", "block":
		return nil
	case "all":
		if allowAll {
			return nil
		}
	}
	if allowAll {
		return mailValidationParamError("--type", "--type must be one of: allow, block, all")
	}
	return mailValidationParamError("--type", "--type must be one of: allow, block")
}

func normalizeUserAllowBlockRecords(raw []string) ([]string, error) {
	out := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for i, record := range raw {
		normalized, err := normalizeUserAllowBlockRecord(record, fmt.Sprintf("record[%d]", i))
		if err != nil {
			return nil, err
		}
		key := strings.ToLower(normalized)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, normalized)
	}
	return out, nil
}

func normalizeUserAllowBlockRecord(raw, param string) (string, error) {
	record := strings.TrimSpace(raw)
	if record == "" {
		return "", mailValidationParamError(param, "%s must not be empty", param)
	}
	if record != raw {
		return "", mailValidationParamError(param, "%s %q must not contain leading or trailing whitespace", param, raw)
	}
	for _, r := range record {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return "", mailValidationParamError(param, "%s %q must not contain whitespace or control characters", param, raw)
		}
	}
	if strings.Count(record, "@") > 1 || strings.HasPrefix(record, "@") || strings.HasSuffix(record, "@") {
		return "", mailValidationParamError(param, "%s %q must be an email address or domain", param, raw)
	}
	return record, nil
}

func userAllowBlockItems(records []string) []map[string]interface{} {
	items := make([]map[string]interface{}, 0, len(records))
	for _, record := range records {
		items = append(items, map[string]interface{}{
			"sender":      record,
			"sender_type": userAllowBlockSenderType(record),
		})
	}
	return items
}

func userAllowBlockSenderType(record string) int {
	if strings.Contains(record, "@") {
		return 1
	}
	return 2
}

func extractUserAllowBlockItems(data map[string]interface{}) []interface{} {
	if items, ok := data["items"].([]interface{}); ok {
		return items
	}
	return []interface{}{}
}

func mergeUserAllowBlockItems(allowData, blockData map[string]interface{}) []interface{} {
	out := make([]interface{}, 0, len(extractUserAllowBlockItems(allowData))+len(extractUserAllowBlockItems(blockData)))
	for _, item := range extractUserAllowBlockItems(allowData) {
		out = append(out, userAllowBlockItemWithType(item, "allow"))
	}
	for _, item := range extractUserAllowBlockItems(blockData) {
		out = append(out, userAllowBlockItemWithType(item, "block"))
	}
	return out
}

func userAllowBlockItemWithType(item interface{}, kind string) interface{} {
	m, ok := item.(map[string]interface{})
	if !ok {
		return item
	}
	clone := make(map[string]interface{}, len(m)+1)
	for k, v := range m {
		clone[k] = v
	}
	clone["type"] = kind
	return clone
}

func findUserAllowBlockRecord(data map[string]interface{}, record string) map[string]interface{} {
	want := strings.ToLower(record)
	for _, item := range extractUserAllowBlockItems(data) {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		for _, key := range []string{"sender", "record", "email", "domain"} {
			if strings.EqualFold(strVal(m[key]), want) {
				return m
			}
		}
	}
	return nil
}

func userAllowBlockBoolVal(v interface{}) bool {
	switch typed := v.(type) {
	case bool:
		return typed
	case string:
		b, _ := strconv.ParseBool(typed)
		return b
	default:
		return false
	}
}

func mailDecorateAllowBlockProblem(err error) error {
	if err == nil {
		return nil
	}
	if p, ok := errs.ProblemOf(err); ok {
		msg := strings.ToLower(p.Message)
		switch {
		case p.Subtype == errs.SubtypeRateLimit || p.Code == output.LarkErrRateLimit || strings.Contains(msg, "rate limit"):
			p.Hint = appendHint(p.Hint, "Retry later with a smaller --page-size or fewer records.")
		case p.Code == 456 || strings.Contains(msg, "cache"):
			p.Hint = appendHint(p.Hint, "Search depends on the mail allow/block cache; retry after the cache is ready, or run list without a keyword.")
		case strings.Contains(msg, "self_address") || strings.Contains(msg, "self address"):
			p.Hint = appendHint(p.Hint, "Remove your own mailbox address or alias from the input.")
		case strings.Contains(msg, "self_domain") || strings.Contains(msg, "self domain"):
			p.Hint = appendHint(p.Hint, "Remove your tenant/internal domain from the input.")
		case strings.Contains(msg, "invalid"):
			p.Hint = appendHint(p.Hint, "Pass email addresses like alice@example.com or domains like example.com; add/delete accepts at most 100 records.")
		}
	}
	return err
}

func appendHint(existing, extra string) string {
	if strings.TrimSpace(existing) == "" {
		return extra
	}
	if strings.Contains(existing, extra) {
		return existing
	}
	return existing + " " + extra
}

func findChildCommand(parent *cobra.Command, name string) *cobra.Command {
	for _, child := range parent.Commands() {
		if child.Name() == name {
			return child
		}
	}
	return nil
}
