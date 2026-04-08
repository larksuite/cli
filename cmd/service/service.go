// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/internal/util"
	"github.com/larksuite/cli/internal/validate"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/spf13/cobra"
)

// RegisterServiceCommands registers all service commands from from_meta specs.
func RegisterServiceCommands(parent *cobra.Command, f *cmdutil.Factory) {
	for _, project := range registry.ListFromMetaProjects() {
		spec := registry.LoadFromMeta(project)
		if spec == nil {
			continue
		}
		specName := registry.GetStrFromMap(spec, "name")
		servicePath := registry.GetStrFromMap(spec, "servicePath")
		if specName == "" || servicePath == "" {
			continue
		}
		resources, _ := spec["resources"].(map[string]interface{})
		if resources == nil {
			continue
		}
		registerService(parent, spec, resources, f)
	}
}

func registerService(parent *cobra.Command, spec map[string]interface{}, resources map[string]interface{}, f *cmdutil.Factory) {
	specName := registry.GetStrFromMap(spec, "name")
	specDesc := registry.GetServiceDescription(specName, "en")
	if specDesc == "" {
		specDesc = registry.GetStrFromMap(spec, "description")
	}

	// Find existing service command or create one
	var svc *cobra.Command
	for _, c := range parent.Commands() {
		if c.Name() == specName {
			svc = c
			break
		}
	}
	if svc == nil {
		svc = &cobra.Command{
			Use:   specName,
			Short: specDesc,
		}
		parent.AddCommand(svc)
	}

	for resName, resource := range resources {
		resMap, _ := resource.(map[string]interface{})
		if resMap == nil {
			continue
		}
		registerResource(svc, spec, resName, resMap, f)
	}
}

func registerResource(parent *cobra.Command, spec map[string]interface{}, name string, resource map[string]interface{}, f *cmdutil.Factory) {
	res := &cobra.Command{
		Use:   name,
		Short: name + " operations",
	}
	parent.AddCommand(res)

	methods, _ := resource["methods"].(map[string]interface{})
	for methodName, method := range methods {
		methodMap, _ := method.(map[string]interface{})
		if methodMap == nil {
			continue
		}
		registerMethod(res, spec, methodMap, methodName, name, f)
	}
}

// ServiceMethodOptions holds all inputs for a dynamically registered service method command.
type ServiceMethodOptions struct {
	Factory    *cmdutil.Factory
	Cmd        *cobra.Command
	Ctx        context.Context
	Spec       map[string]interface{}
	Method     map[string]interface{}
	SchemaPath string

	// Flags
	Params    string
	Data      string
	As        core.Identity
	Output    string
	PageAll   bool
	PageLimit int
	PageDelay int
	Format    string
	JqExpr    string
	DryRun    bool
}

func registerMethod(parent *cobra.Command, spec map[string]interface{}, method map[string]interface{}, name string, resName string, f *cmdutil.Factory) {
	parent.AddCommand(NewCmdServiceMethod(f, spec, method, name, resName, nil))
}

// NewCmdServiceMethod creates a command for a dynamically registered service method.
func NewCmdServiceMethod(f *cmdutil.Factory, spec, method map[string]interface{}, name, resName string, runF func(*ServiceMethodOptions) error) *cobra.Command {
	desc := registry.GetStrFromMap(method, "description")
	httpMethod := registry.GetStrFromMap(method, "httpMethod")
	specName := registry.GetStrFromMap(spec, "name")
	schemaPath := fmt.Sprintf("%s.%s.%s", specName, resName, name)

	opts := &ServiceMethodOptions{
		Factory:    f,
		Spec:       spec,
		Method:     method,
		SchemaPath: schemaPath,
	}
	var asStr string

	cmd := &cobra.Command{
		Use:   name,
		Short: desc,
		Long:  fmt.Sprintf("%s\n\nView parameter definitions before calling:\n  lark-cli schema %s", desc, schemaPath),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Cmd = cmd
			opts.Ctx = cmd.Context()
			opts.As = core.Identity(asStr)
			if runF != nil {
				return runF(opts)
			}
			return serviceMethodRun(opts)
		},
	}

	cmd.Flags().StringVar(&opts.Params, "params", "", "URL/query parameters JSON")
	switch httpMethod {
	case "POST", "PUT", "PATCH", "DELETE":
		cmd.Flags().StringVar(&opts.Data, "data", "", "request body JSON")
	}
	cmd.Flags().StringVar(&asStr, "as", "auto", "identity type: user | bot | auto (default)")
	cmd.Flags().StringVarP(&opts.Output, "output", "o", "", "output file path for binary responses")
	cmd.Flags().BoolVar(&opts.PageAll, "page-all", false, "automatically paginate through all pages")
	cmd.Flags().IntVar(&opts.PageLimit, "page-limit", 10, "max pages to fetch with --page-all (0 = unlimited)")
	cmd.Flags().IntVar(&opts.PageDelay, "page-delay", 200, "delay in ms between pages")
	cmd.Flags().StringVar(&opts.Format, "format", "json", "output format: json|ndjson|table|csv")
	cmd.Flags().StringVarP(&opts.JqExpr, "jq", "q", "", "jq expression to filter JSON output")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "print request without executing")

	_ = cmd.RegisterFlagCompletionFunc("as", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"user", "bot"}, cobra.ShellCompDirectiveNoFileComp
	})
	_ = cmd.RegisterFlagCompletionFunc("format", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"json", "ndjson", "table", "csv"}, cobra.ShellCompDirectiveNoFileComp
	})

	cmdutil.SetTips(cmd, registry.GetStrSliceFromMap(method, "tips"))
	if tokens, ok := method["accessTokens"].([]interface{}); ok && len(tokens) > 0 {
		cmdutil.SetSupportedIdentities(cmd, cmdutil.AccessTokensToIdentities(tokens))
	}

	return cmd
}

func serviceMethodRun(opts *ServiceMethodOptions) error {
	f := opts.Factory
	opts.As = f.ResolveAs(opts.Ctx, opts.Cmd, opts.As)

	if err := f.CheckStrictMode(opts.Ctx, opts.As); err != nil {
		return err
	}

	// Check if this API method supports the resolved identity.
	if tokens, ok := opts.Method["accessTokens"].([]interface{}); ok && len(tokens) > 0 {
		if err := f.CheckIdentity(opts.As, cmdutil.AccessTokensToIdentities(tokens)); err != nil {
			return err
		}
	}

	if opts.PageAll && opts.Output != "" {
		return output.ErrValidation("--output and --page-all are mutually exclusive")
	}
	if err := output.ValidateJqFlags(opts.JqExpr, opts.Output, opts.Format); err != nil {
		return err
	}

	config, err := f.Config()
	if err != nil {
		return err
	}
	// Identity info is now included in the JSON envelope; skip stderr printing.
	// cmdutil.PrintIdentity(f.IOStreams.ErrOut, opts.As, config, f.IdentityAutoDetected)

	scopes, _ := opts.Method["scopes"].([]interface{})
	if !opts.As.IsBot() {
		if err := checkServiceScopes(opts.Ctx, f.Credential, opts.As, config, opts.Method, scopes); err != nil {
			return err
		}
	}

	request, err := buildServiceRequest(opts)
	if err != nil {
		return err
	}

	if opts.DryRun {
		return serviceDryRun(f, request, config, opts.Format)
	}

	ac, err := f.NewAPIClientWithConfig(config)
	if err != nil {
		return err
	}

	out := f.IOStreams.Out
	format, formatOK := output.ParseFormat(opts.Format)
	if !formatOK {
		fmt.Fprintf(f.IOStreams.ErrOut, "warning: unknown format %q, falling back to json\n", opts.Format)
	}

	checkErr := scopeAwareChecker(scopes, opts.As.IsBot())

	if opts.PageAll {
		return servicePaginate(opts.Ctx, ac, request, format, opts.JqExpr, out, f.IOStreams.ErrOut,
			client.PaginationOptions{PageLimit: opts.PageLimit, PageDelay: opts.PageDelay}, checkErr)
	}

	resp, err := ac.DoAPI(opts.Ctx, request)
	if err != nil {
		return output.ErrNetwork("API call failed: %s", err)
	}
	return client.HandleResponse(resp, client.ResponseOptions{
		OutputPath: opts.Output,
		Format:     format,
		JqExpr:     opts.JqExpr,
		Out:        out,
		ErrOut:     f.IOStreams.ErrOut,
		CheckError: checkErr,
	})
}

// checkServiceScopes pre-checks user scopes before making the API call.
func checkServiceScopes(ctx context.Context, cred *credential.CredentialProvider, identity core.Identity, config *core.CliConfig, method map[string]interface{}, scopes []interface{}) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	result, err := cred.ResolveToken(ctx, credential.NewTokenSpec(identity, config.AppID))
	if err != nil || result == nil || result.Scopes == "" {
		return nil //nolint:nilerr // skip scope check when token resolution fails or has no scopes
	}

	requiredScopes, hasRequired := method["requiredScopes"].([]interface{})

	if hasRequired && len(requiredScopes) > 0 {
		// Strict: ALL requiredScopes must be present
		required := make([]string, 0, len(requiredScopes))
		for _, s := range requiredScopes {
			if str, ok := s.(string); ok {
				required = append(required, str)
			}
		}
		if missing := auth.MissingScopes(result.Scopes, required); len(missing) > 0 {
			return output.ErrWithHint(output.ExitAuth, "missing_scope",
				fmt.Sprintf("missing required scope(s): %s", strings.Join(missing, ", ")),
				fmt.Sprintf("run `lark-cli auth login --scope \"%s\"` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login.", strings.Join(missing, " ")))
		}
		return nil
	}

	if len(scopes) == 0 {
		return nil
	}

	// Default: ANY one of the declared scopes is sufficient
	grantedSet := make(map[string]bool)
	for _, s := range strings.Fields(result.Scopes) {
		grantedSet[s] = true
	}
	for _, s := range scopes {
		if str, ok := s.(string); ok && grantedSet[str] {
			return nil
		}
	}
	recommended := registry.SelectRecommendedScope(scopes, "user")
	return output.ErrWithHint(output.ExitAPI, "permission",
		fmt.Sprintf("insufficient permissions (required scope: %s)", recommended),
		fmt.Sprintf(`run `+"`"+`lark-cli auth login --scope "%s"`+"`"+` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login.`, recommended))
}

// buildServiceRequest parses flags, builds the URL with path/query params, and returns a RawApiRequest.
func buildServiceRequest(opts *ServiceMethodOptions) (client.RawApiRequest, error) {
	spec := opts.Spec
	method := opts.Method
	schemaPath := opts.SchemaPath
	httpMethod := registry.GetStrFromMap(method, "httpMethod")

	var params map[string]interface{}
	if opts.Params != "" {
		if err := json.Unmarshal([]byte(opts.Params), &params); err != nil {
			return client.RawApiRequest{}, output.ErrValidation("--params invalid JSON format")
		}
	} else {
		params = map[string]interface{}{}
	}

	url := registry.GetStrFromMap(spec, "servicePath") + "/" + registry.GetStrFromMap(method, "path")

	parameters, _ := method["parameters"].(map[string]interface{})
	for name, param := range parameters {
		p, _ := param.(map[string]interface{})
		if registry.GetStrFromMap(p, "location") != "path" {
			continue
		}
		val, ok := params[name]
		if !ok || util.IsEmptyValue(val) {
			return client.RawApiRequest{}, output.ErrWithHint(output.ExitValidation, "validation",
				fmt.Sprintf("missing required path parameter: %s", name),
				fmt.Sprintf("lark-cli schema %s", schemaPath))
		}
		valStr := fmt.Sprintf("%v", val)
		if err := validate.ResourceName(valStr, name); err != nil {
			return client.RawApiRequest{}, output.ErrValidation("%s", err)
		}
		url = strings.Replace(url, "{"+name+"}", validate.EncodePathSegment(valStr), 1)
		delete(params, name)
	}

	queryParams := map[string]interface{}{}
	for name, param := range parameters {
		p, _ := param.(map[string]interface{})
		if registry.GetStrFromMap(p, "location") != "query" {
			continue
		}
		value, exists := params[name]
		required, _ := p["required"].(bool)
		isPaginationParam := opts.PageAll && (name == "page_token" || name == "page_size")
		if required && !isPaginationParam && (!exists || util.IsEmptyValue(value)) {
			return client.RawApiRequest{}, output.ErrWithHint(output.ExitValidation, "validation",
				fmt.Sprintf("missing required query parameter: %s", name),
				fmt.Sprintf("lark-cli schema %s", schemaPath))
		}
		if exists && !util.IsEmptyValue(value) {
			queryParams[name] = value
		}
	}
	for name, value := range params {
		if _, ok := queryParams[name]; !ok {
			queryParams[name] = value
		}
	}

	data, err := cmdutil.ParseOptionalBody(httpMethod, opts.Data)
	if err != nil {
		return client.RawApiRequest{}, err
	}

	request := client.RawApiRequest{
		Method: httpMethod,
		URL:    url,
		Params: queryParams,
		Data:   data,
		As:     opts.As,
	}
	if opts.Output != "" {
		request.ExtraOpts = append(request.ExtraOpts, larkcore.WithFileDownload())
	}
	return request, nil
}

func serviceDryRun(f *cmdutil.Factory, request client.RawApiRequest, config *core.CliConfig, format string) error {
	return cmdutil.PrintDryRun(f.IOStreams.Out, request, config, format)
}

// scopeAwareChecker returns an error checker that enriches scope-related errors with login hints.
func scopeAwareChecker(scopes []interface{}, isBotMode bool) func(interface{}) error {
	return func(result interface{}) error {
		resultMap, ok := result.(map[string]interface{})
		if !ok || resultMap == nil {
			return nil
		}
		code, _ := util.ToFloat64(resultMap["code"])
		if code == 0 {
			return nil
		}
		larkCode := int(code)
		msg := registry.GetStrFromMap(resultMap, "msg")

		if larkCode == output.LarkErrUserScopeInsufficient && len(scopes) > 0 {
			identity := "user"
			if isBotMode {
				identity = "tenant"
			}
			recommended := registry.SelectRecommendedScope(scopes, identity)
			return output.ErrWithHint(output.ExitAPI, "permission",
				fmt.Sprintf("insufficient permissions: [%d] %s", larkCode, msg),
				fmt.Sprintf(`run `+"`"+`lark-cli auth login --scope "%s"`+"`"+` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login.`, recommended))
		}

		return output.ErrAPI(larkCode, fmt.Sprintf("API error: [%d] %s", larkCode, msg), resultMap["error"])
	}
}

func servicePaginate(ctx context.Context, ac *client.APIClient, request client.RawApiRequest, format output.Format, jqExpr string, out, errOut io.Writer, pagOpts client.PaginationOptions, checkErr func(interface{}) error) error {
	// When jq is set, always aggregate all pages then filter.
	if jqExpr != "" {
		return client.PaginateWithJq(ctx, ac, request, jqExpr, out, pagOpts, checkErr)
	}

	switch format {
	case output.FormatNDJSON, output.FormatTable, output.FormatCSV:
		pf := output.NewPaginatedFormatter(out, format)
		result, hasItems, err := ac.StreamPages(ctx, request, func(items []interface{}) {
			pf.FormatPage(items)
		}, pagOpts)
		if err != nil {
			return output.ErrNetwork("API call failed: %s", err)
		}
		if apiErr := checkErr(result); apiErr != nil {
			return apiErr
		}
		if !hasItems {
			fmt.Fprintf(errOut, "warning: this API does not return a list, format %q is not supported, falling back to json\n", format)
			output.FormatValue(out, result, output.FormatJSON)
		}
		return nil
	default:
		result, err := ac.PaginateAll(ctx, request, pagOpts)
		if err != nil {
			return output.ErrNetwork("API call failed: %s", err)
		}
		if apiErr := checkErr(result); apiErr != nil {
			return apiErr
		}
		output.FormatValue(out, result, format)
		return nil
	}
}
