// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/cmdmeta"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/meta"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/internal/validate"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// RegisterServiceCommands registers all service commands from from_meta specs.
func RegisterServiceCommands(parent *cobra.Command, f *cmdutil.Factory) {
	RegisterServiceCommandsWithContext(context.Background(), parent, f)
}

func RegisterServiceCommandsWithContext(ctx context.Context, parent *cobra.Command, f *cmdutil.Factory) {
	RegisterServiceCommandsFromCatalog(ctx, parent, f, registry.RuntimeCatalog())
}

func RegisterServiceCommandsFromCatalog(ctx context.Context, parent *cobra.Command, f *cmdutil.Factory, catalog apicatalog.Catalog) {
	// Drive the service list from the same navigation catalog the method walk
	// uses, so registration is catalog-sourced end to end. Kept as a per-service
	// loop rather than a flat WalkMethods(nil) drive precisely so a service with
	// no methods still gets its bare command (WalkMethods yields one ref per
	// method, so empty services would vanish).
	for _, svc := range catalog.Services() {
		if svc.Name == "" || svc.ServicePath == "" {
			continue
		}
		registerServiceWithContext(ctx, parent, svc, f)
	}
}

func registerService(parent *cobra.Command, svc meta.Service, f *cmdutil.Factory) {
	registerServiceWithContext(context.Background(), parent, svc, f)
}

func registerServiceWithContext(ctx context.Context, parent *cobra.Command, svc meta.Service, f *cmdutil.Factory) {
	svcCmd := ensureChildCommand(parent, svc.Name, serviceShort(svc))

	// Build the service's subtree from the catalog's method walk
	// (apicatalog.ServiceMethods recurses nested resources), so the command tree
	// is sourced from the same navigation Module as schema/scope rather than a
	// hand-rolled resource/method walk. Each ref's ResourcePath becomes the
	// resource-command chain — one level for a flat dotted resource like
	// "chat.members", deeper for genuinely nested resources. A service with no
	// methods keeps its bare command (svcCmd is created above regardless).
	for _, ref := range apicatalog.ServiceMethods(svc, nil) {
		resCmd := svcCmd
		for _, seg := range ref.ResourcePath {
			resCmd = ensureChildCommand(resCmd, seg, seg+" operations")
		}
		resCmd.AddCommand(buildMethodCommand(ctx, f, newMethodCommandSpec(ref), nil, parent.PersistentFlags()))
	}
}

// serviceShort is the service command's help summary: the localized description
// from the registry, falling back to the metadata's own description.
func serviceShort(svc meta.Service) string {
	if d := registry.GetServiceDescription(svc.Name, "en"); d != "" {
		return d
	}
	return svc.Description
}

// ensureChildCommand returns the child of parent named name, creating it (with
// short) when absent — so re-registration merges into an existing command tree
// instead of duplicating a level.
func ensureChildCommand(parent *cobra.Command, name, short string) *cobra.Command {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			cmdmeta.SetSource(c, cmdmeta.SourceService, true)
			return c
		}
	}
	cmd := &cobra.Command{Use: name, Short: short}
	cmdmeta.SetSource(cmd, cmdmeta.SourceService, true)
	parent.AddCommand(cmd)
	return cmd
}

// ServiceMethodOptions holds all inputs for a dynamically registered service method command.
type ServiceMethodOptions struct {
	Factory     *cmdutil.Factory
	Cmd         *cobra.Command
	Ctx         context.Context
	ServicePath string
	Method      meta.Method
	SchemaPath  string

	// Flags
	Params     string
	Data       string
	As         core.Identity
	Output     string
	PageAll    bool
	PageLimit  int
	PageDelay  int
	Format     string
	JqExpr     string
	DryRun     bool
	File       string   // --file flag value
	FileFields []string // auto-detected file field names from metadata

	// binder owns the generated typed param flags — registration and the
	// --params overlay — replacing the raw paramFlags side-channel.
	binder *paramFlagBinder
}

// detectFileFields returns the request-body file-upload field names.
func detectFileFields(m meta.Method) []string {
	files := m.Files()
	if len(files) == 0 {
		return nil
	}
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = f.Name
	}
	return names
}

// NewCmdServiceMethod creates a command for a dynamically registered service method.
func NewCmdServiceMethod(f *cmdutil.Factory, svc meta.Service, m meta.Method, name, resName string, runF func(*ServiceMethodOptions) error) *cobra.Command {
	return NewCmdServiceMethodWithContext(context.Background(), f, svc, m, name, resName, runF)
}

// NewCmdServiceMethodWithContext builds the command for one service method from
// its (service, resource, method) coordinates, deriving the methodCommandSpec
// via an apicatalog.MethodRef so direct callers and the catalog-driven
// registration assemble the command identically.
func NewCmdServiceMethodWithContext(ctx context.Context, f *cmdutil.Factory, svc meta.Service, m meta.Method, name, resName string, runF func(*ServiceMethodOptions) error) *cobra.Command {
	m.Name = name
	ref := apicatalog.MethodRef{Service: svc, ResourcePath: []string{resName}, Method: m}
	// No root in scope here; persistent-flag collisions don't apply to a
	// standalone command, and local/standard-flag collisions are still caught.
	return buildMethodCommand(ctx, f, newMethodCommandSpec(ref), runF, nil)
}

// methodCommandSpec is the static description of one generated service method
// command, read off an apicatalog.MethodRef — the single place command
// construction gets the method's facts (schema path, HTTP base path, risk,
// identities, params, file fields, request-body support), so the cobra command
// is assembled from a typed spec rather than recomputing paths/flags inline.
type methodCommandSpec struct {
	method      meta.Method
	schemaPath  string       // "service.resource.method", for the --help hint
	servicePath string       // service HTTP base path
	risk        string       // RiskRead | RiskWrite | RiskHighRiskWrite
	restricts   bool         // method declares accessTokens (identity-restricted)
	identities  []string     // permitted --as values; empty when unrestricted
	params      []meta.Field // path/query params -> typed flags
	fileFields  []string     // request-body file-upload field names
	// acceptsBody is whether the HTTP method allows a request body at all (so
	// --data is offered as a raw escape hatch). declaresBody is whether the
	// metadata documents body fields (data or file). They differ for e.g. a POST
	// with no documented requestBody: --data still works, but help must not imply
	// the API declares a body.
	acceptsBody  bool
	declaresBody bool
	affordance   string // rendered hand-authored usage guidance (when-to-use, examples); "" if none
}

func newMethodCommandSpec(ref apicatalog.MethodRef) methodCommandSpec {
	m := ref.Method
	return methodCommandSpec{
		method:       m,
		schemaPath:   ref.SchemaPath(),
		servicePath:  ref.Service.ServicePath,
		risk:         m.Risk,
		restricts:    m.RestrictsIdentity(),
		identities:   m.Identities(),
		params:       m.Params(),
		fileFields:   detectFileFields(m),
		acceptsBody:  methodTakesBody(m.HTTPMethod),
		declaresBody: len(m.Data()) > 0 || len(m.Files()) > 0,
		affordance:   renderAffordance(m),
	}
}

// methodTakesBody reports whether the HTTP method allows a request body, i.e.
// whether --data applies (as a raw escape hatch even when no body is declared).
func methodTakesBody(httpMethod string) bool {
	switch httpMethod {
	case "POST", "PUT", "PATCH", "DELETE":
		return true
	}
	return false
}

// buildMethodCommand assembles the cobra command for a service method from its
// static spec: the standard flags, the conditional --data/--file/--yes flags,
// the generated typed param flags (via paramFlagBinder), and the risk/identity
// policy annotations.
func buildMethodCommand(ctx context.Context, f *cmdutil.Factory, spec methodCommandSpec, runF func(*ServiceMethodOptions) error, reserved *pflag.FlagSet) *cobra.Command {
	m := spec.method
	opts := &ServiceMethodOptions{
		Factory:     f,
		ServicePath: spec.servicePath,
		Method:      m,
		SchemaPath:  spec.schemaPath,
		FileFields:  spec.fileFields,
	}
	var asStr string

	cmd := &cobra.Command{
		Use:   m.Name,
		Short: m.Description,
		// Long is assembled below, once the binder knows which params got no
		// typed flag.
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
	cmdmeta.SetSource(cmd, cmdmeta.SourceService, true)

	cmd.Flags().StringVar(&opts.Params, "params", "", "Raw URL/query params JSON. Supports - and @file.")
	if spec.acceptsBody {
		dataUsage := "JSON request body. Supports - and @file."
		if !spec.declaresBody {
			// POST/etc. with no documented body fields: --data is a raw escape
			// hatch, not a declared body — say so rather than imply structure.
			dataUsage = "Raw JSON request body (no documented fields; see schema). Supports - and @file."
		}
		cmd.Flags().StringVar(&opts.Data, "data", "", dataUsage)
	}
	cmdutil.AddAPIIdentityFlag(ctx, cmd, f, &asStr)
	cmd.Flags().StringVarP(&opts.Output, "output", "o", "", "output file path for binary responses")
	cmd.Flags().BoolVar(&opts.PageAll, "page-all", false, "automatically paginate through all pages")
	cmd.Flags().IntVar(&opts.PageLimit, "page-limit", 10, "max pages to fetch with --page-all (0 = unlimited)")
	cmd.Flags().IntVar(&opts.PageDelay, "page-delay", 200, "delay in ms between pages")
	cmd.Flags().StringVar(&opts.Format, "format", "json", "output format: json|ndjson|table|csv")
	cmd.Flags().Bool("json", false, "shorthand for --format json")
	cmd.Flags().StringVarP(&opts.JqExpr, "jq", "q", "", "jq expression to filter JSON output")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "print request without executing")
	if spec.risk == cmdutil.RiskHighRiskWrite {
		cmd.Flags().Bool("yes", false, "confirm high-risk operation")
	}
	// --file only for body methods that actually declare file-type fields.
	if len(spec.fileFields) > 0 && spec.acceptsBody {
		cmd.Flags().StringVar(&opts.File, "file", "", "File upload [field=]path. Supports - and stdin.")
	}
	cmdutil.RegisterFlagCompletion(cmd, "format", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"json", "ndjson", "table", "csv"}, cobra.ShellCompDirectiveNoFileComp
	})

	// Registered last so the collision guard sees the standard flags above.
	opts.binder = newParamFlagBinder(cmd, spec.params, reserved)
	// Single composition point for Long: description, affordance, schema
	// pointer, and the binder's params-only addendum (params whose flag name is
	// taken, reachable via --params only).
	cmd.Long = methodLong(m.Description, spec.affordance, spec.schemaPath, opts.binder.paramsOnlyHelp())

	// Group flags for the grouped --help renderer (typed param flags are grouped
	// as API Parameters by the binder). tagFlagGroup is a no-op for flags not
	// registered above (e.g. --data/--file/--yes only exist for some methods).
	// --data sits under Request Body only when the metadata documents body
	// fields; otherwise it's a raw escape hatch, grouped with --params so help
	// doesn't imply a declared body the API doesn't have.
	if fl := cmd.Flags().Lookup("data"); fl != nil {
		if spec.declaresBody {
			annotate(fl, flagGroupAnnotation, []string{groupBody})
		} else {
			annotate(fl, flagGroupAnnotation, []string{groupRaw})
		}
	}
	tagFlagGroup(cmd.Flags(), "file", groupBody)
	if fl := cmd.Flags().Lookup("params"); fl != nil {
		annotate(fl, flagGroupAnnotation, []string{groupRaw})
		// State the precedence rule where the agent reads it: --params is the
		// base, typed flags override. Only meaningful when typed flags exist.
		if len(spec.params) > 0 {
			annotate(fl, flagNoteAnnotation, []string{
				"Typed API parameter flags above are preferred.",
				"If both are set, typed flags override matching keys in --params.",
			})
		}
	}
	for _, name := range []string{"as", "dry-run", "page-all", "page-limit", "page-delay", "yes"} {
		tagFlagGroup(cmd.Flags(), name, groupExecution)
	}
	for _, name := range []string{"output", "format", "jq"} {
		tagFlagGroup(cmd.Flags(), name, groupOutput)
	}
	applyGroupedUsage(cmd)

	cmdutil.SetTips(cmd, m.Tips)
	cmdutil.SetRisk(cmd, spec.risk)
	if spec.restricts {
		cmdutil.SetSupportedIdentities(cmd, spec.identities)
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
	if opts.Method.RestrictsIdentity() {
		if err := f.CheckIdentity(opts.As, opts.Method.Identities()); err != nil {
			return err
		}
	}

	if opts.PageAll && opts.Output != "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--output and --page-all are mutually exclusive").WithParam("--output")
	}
	if err := output.ValidateJqFlags(opts.JqExpr, opts.Output, opts.Format); err != nil {
		return err
	}

	config, err := f.Config()
	if err != nil {
		return err
	}
	// Identity is not printed to stderr here: it is part of the JSON envelope.

	if !opts.As.IsBot() {
		if err := checkServiceScopes(opts.Ctx, f.Credential, opts.As, config, opts.Method); err != nil {
			return err
		}
	}

	request, fileMeta, err := buildServiceRequest(opts)
	if err != nil {
		return err
	}

	if opts.DryRun {
		if fileMeta != nil {
			return cmdutil.PrintDryRunWithFile(f.IOStreams.Out, request, config, opts.Format, fileMeta.FieldName, fileMeta.FilePath, fileMeta.FormFields)
		}
		return serviceDryRun(f, request, config, opts.Format)
	}

	if opts.Method.Risk == cmdutil.RiskHighRiskWrite {
		if yes, _ := opts.Cmd.Flags().GetBool("yes"); !yes {
			return cmdutil.RequireConfirmation(opts.SchemaPath)
		}
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

	// Scope-insufficient (99991679) and all other Lark API codes route through
	// errclass.BuildAPIError via ac.CheckResponse, producing *errs.PermissionError
	// with MissingScopes / Identity / ConsoleURL populated from the response.
	checkErr := ac.CheckResponse

	if opts.PageAll {
		return servicePaginate(opts.Ctx, ac, request, format, opts.JqExpr, out, f.IOStreams.ErrOut, opts.Cmd.CommandPath(),
			client.PaginationOptions{PageLimit: opts.PageLimit, PageDelay: opts.PageDelay}, checkErr)
	}

	resp, err := ac.DoAPI(opts.Ctx, request)
	if err != nil {
		return err
	}
	return client.HandleResponse(resp, client.ResponseOptions{
		OutputPath:  opts.Output,
		Format:      format,
		JqExpr:      opts.JqExpr,
		Out:         out,
		ErrOut:      f.IOStreams.ErrOut,
		FileIO:      f.ResolveFileIO(opts.Ctx),
		CommandPath: opts.Cmd.CommandPath(),
		Identity:    opts.As,
		CheckError:  checkErr,
	})
}

// checkServiceScopes pre-checks user scopes before making the API call.
func checkServiceScopes(ctx context.Context, cred *credential.CredentialProvider, identity core.Identity, config *core.CliConfig, method meta.Method) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	result, err := cred.ResolveToken(ctx, credential.NewTokenSpec(identity, config.AppID))
	if err != nil || result == nil || result.Scopes == "" {
		return nil //nolint:nilerr // skip scope check when token resolution fails or has no scopes
	}

	if len(method.RequiredScopes) > 0 {
		// Strict: ALL requiredScopes must be present
		if missing := auth.MissingScopes(result.Scopes, method.RequiredScopes); len(missing) > 0 {
			return newPreflightMissingScopeError(string(config.Brand), config.AppID, string(identity), missing)
		}
		return nil
	}

	if len(method.Scopes) == 0 {
		return nil
	}

	// Default: ANY one of the declared scopes is sufficient
	grantedSet := make(map[string]bool)
	for _, s := range strings.Fields(result.Scopes) {
		grantedSet[s] = true
	}
	for _, s := range method.Scopes {
		if grantedSet[s] {
			return nil
		}
	}
	recommended := registry.SelectRecommendedScopeFromStrings(method.Scopes, "user")
	return newPreflightMissingScopeError(string(config.Brand), config.AppID, string(identity), []string{recommended})
}

// newPreflightMissingScopeError constructs a PermissionError for the local
// pre-flight scope check that converges byte-for-byte with the dispatcher's
// BuildAPIError path. Uses the canonical helpers in internal/errclass so
// Hint and Message stay in lock-step with the server-response classifier.
// ConsoleURL is deliberately omitted: the dispatcher only sets it for
// SubtypeAppScopeNotApplied (bot-perspective dev-action recovery), and this
// pre-flight path is user-perspective SubtypeMissingScope whose recovery is
// `lark-cli auth login --scope ...`, not a console deep-link.
func newPreflightMissingScopeError(brand, appID, identity string, missing []string) *errs.PermissionError {
	consoleURL := errclass.ConsoleURL(brand, appID, missing)
	return errs.NewPermissionError(errs.SubtypeMissingScope,
		"%s", errclass.CanonicalPermissionMessage(errs.SubtypeMissingScope, appID, missing, "")).
		WithHint("%s", errclass.PermissionHint(missing, identity, errs.SubtypeMissingScope, consoleURL)).
		WithMissingScopes(missing...).
		WithIdentity(identity)
}

// unusableParamValue reports whether a provided path/query parameter value
// cannot form a usable request value: nil or an empty string. A key's presence
// in params is the intent signal — a typed flag is overlaid only when
// explicitly Changed, and a --params JSON key is deliberately written — so
// false and 0 are real values and must not be conflated with "unset"
// (reflect.IsZero would drop an explicit --with-deleted=false or --foo 0).
// Only nil/"" stay treated as missing: that keeps the friendly pre-flight
// error when a required param is fed an empty placeholder, and never emits a
// declared param as an empty path segment or query value. Undeclared keys are
// not judged by this rule — they pass through verbatim as the raw escape hatch.
func unusableParamValue(v interface{}) bool {
	if v == nil {
		return true
	}
	s, ok := v.(string)
	return ok && s == ""
}

// missingParamHint is the recovery hint for a missing required parameter. It
// names both input paths — the typed flag when the binder registered one, and
// the --params fallback — plus the schema pointer. A params-only field gets
// only the --params form: a flag with its kebab name exists but belongs to
// something else (e.g. the output --format), and the hint must not steer
// there. Asking the binder, not cmd.Flags(), is what tells those apart.
func missingParamHint(opts *ServiceMethodOptions, f meta.Field) string {
	paramsForm := fmt.Sprintf("--params '{%q: \"<value>\"}'", f.Name)
	if opts.binder.hasTypedFlag(f.Name) {
		return fmt.Sprintf("set --%s <value> (or %s); see: lark-cli schema %s", f.FlagName(), paramsForm, opts.SchemaPath)
	}
	return fmt.Sprintf("set %s; see: lark-cli schema %s", paramsForm, opts.SchemaPath)
}

// buildServiceRequest parses flags, builds the URL with path/query params, and returns a RawApiRequest.
// When dryRun is true and a file is provided, file reading is skipped and
// FileUploadMeta is returned instead so the caller can render dry-run output.
func buildServiceRequest(opts *ServiceMethodOptions) (client.RawApiRequest, *cmdutil.FileUploadMeta, error) {
	method := opts.Method
	httpMethod := method.HTTPMethod

	// stdin is an io.Reader consumed at most once. Only one of --params/--data
	// may use "-" (stdin); the conflict check below prevents silent data loss.
	stdin := opts.Factory.IOStreams.In
	fileIO := opts.Factory.ResolveFileIO(opts.Ctx)

	// Validate --file mutual exclusions.
	if err := cmdutil.ValidateFileFlag(opts.File, opts.Params, opts.Data, opts.Output, opts.PageAll, httpMethod); err != nil {
		return client.RawApiRequest{}, nil, err
	}
	if opts.Params == "-" && opts.Data == "-" {
		return client.RawApiRequest{}, nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--params and --data cannot both read from stdin (-)").WithParam("--params")
	}
	params, err := cmdutil.ParseJSONMap(opts.Params, "--params", stdin, fileIO)
	if err != nil {
		return client.RawApiRequest{}, nil, err
	}
	opts.binder.overlay(opts.Cmd, params)

	url := opts.ServicePath + "/" + method.Path

	specs := method.Params()
	for _, s := range specs {
		if s.Location != "path" {
			continue
		}
		val, ok := params[s.Name]
		if !ok || unusableParamValue(val) {
			return client.RawApiRequest{}, nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"missing required path parameter: %s", s.Name).
				WithHint("%s", missingParamHint(opts, s)).
				WithParam(s.Name)
		}
		valStr := fmt.Sprintf("%v", val)
		if err := validate.ResourceName(valStr, s.Name); err != nil {
			return client.RawApiRequest{}, nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam(s.Name).WithCause(err)
		}
		url = strings.Replace(url, "{"+s.Name+"}", validate.EncodePathSegment(valStr), 1)
		delete(params, s.Name)
	}

	queryParams := map[string]interface{}{}
	for _, s := range specs {
		if s.Location != "query" {
			continue
		}
		value, exists := params[s.Name]
		isPaginationParam := opts.PageAll && (s.Name == "page_token" || s.Name == "page_size")
		if s.Required && !isPaginationParam && (!exists || unusableParamValue(value)) {
			return client.RawApiRequest{}, nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"missing required query parameter: %s", s.Name).
				WithHint("%s", missingParamHint(opts, s)).
				WithParam(s.Name)
		}
		if exists && !unusableParamValue(value) {
			queryParams[s.Name] = value
		}
		// This loop owns declared query params: consume the key so the
		// passthrough below can't resurrect a value the gate dropped (an
		// unusable "" would otherwise be sent as an empty query value).
		delete(params, s.Name)
	}
	// Whatever remains is undeclared — the raw escape hatch for params the
	// metadata doesn't (yet) describe; passed through verbatim, no filtering.
	for name, value := range params {
		queryParams[name] = value
	}

	request := client.RawApiRequest{
		Method: httpMethod,
		URL:    url,
		Params: queryParams,
		As:     opts.As,
	}

	if opts.File != "" {
		// File upload: determine default field name from metadata.
		defaultField := "file"
		if len(opts.FileFields) == 1 {
			defaultField = opts.FileFields[0]
		}
		fieldName, filePath, isStdin := cmdutil.ParseFileFlag(opts.File, defaultField)

		// Parse --data as form fields.
		var dataFields any
		if opts.Data != "" {
			dataFields, err = cmdutil.ParseOptionalBody(httpMethod, opts.Data, stdin, fileIO)
			if err != nil {
				return client.RawApiRequest{}, nil, err
			}
			if _, ok := dataFields.(map[string]any); !ok {
				return client.RawApiRequest{}, nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--data must be a JSON object when used with --file").WithParam("--data")
			}
		}

		if opts.DryRun {
			return request, &cmdutil.FileUploadMeta{
				FieldName: fieldName, FilePath: filePath, FormFields: dataFields,
			}, nil
		}

		fd, err := cmdutil.BuildFormdata(
			fileIO,
			fieldName, filePath, isStdin, stdin, dataFields,
		)
		if err != nil {
			return client.RawApiRequest{}, nil, err
		}
		request.Data = fd
		request.ExtraOpts = append(request.ExtraOpts, larkcore.WithFileUpload())
	} else {
		data, err := cmdutil.ParseOptionalBody(httpMethod, opts.Data, stdin, fileIO)
		if err != nil {
			return client.RawApiRequest{}, nil, err
		}
		request.Data = data
		if opts.Output != "" {
			request.ExtraOpts = append(request.ExtraOpts, larkcore.WithFileDownload())
		}
	}

	return request, nil, nil
}

func serviceDryRun(f *cmdutil.Factory, request client.RawApiRequest, config *core.CliConfig, format string) error {
	return cmdutil.PrintDryRun(f.IOStreams.Out, request, config, format)
}

func servicePaginate(ctx context.Context, ac *client.APIClient, request client.RawApiRequest, format output.Format, jqExpr string, out, errOut io.Writer, commandPath string, pagOpts client.PaginationOptions, checkErr func(interface{}, core.Identity) error) error {
	if pagOpts.Identity == "" {
		pagOpts.Identity = request.As
	}
	// When jq is set, always aggregate all pages then filter.
	if jqExpr != "" {
		result, err := ac.PaginateAll(ctx, request, pagOpts)
		if err != nil {
			return err
		}
		if apiErr := checkErr(result, pagOpts.Identity); apiErr != nil {
			output.FormatValue(out, result, output.FormatJSON)
			return apiErr
		}
		return output.WriteSuccessEnvelope(output.SuccessEnvelopeData(result), output.SuccessEnvelopeOptions{
			CommandPath: commandPath,
			Identity:    string(pagOpts.Identity),
			JqExpr:      jqExpr,
			Out:         out,
			ErrOut:      errOut,
		})
	}

	switch format {
	case output.FormatNDJSON, output.FormatTable, output.FormatCSV:
		pf := output.NewPaginatedFormatter(out, format)
		result, hasItems, err := ac.StreamPages(ctx, request, func(items []interface{}) error {
			// Streaming formats intentionally emit each page after that page has
			// passed safety scanning. A later page may still fail, so callers
			// must use the exit code to distinguish complete vs partial output.
			scanResult := output.ScanForSafety(commandPath, items, errOut)
			if scanResult.Blocked {
				return scanResult.BlockErr
			}
			if scanResult.Alert != nil {
				output.WriteAlertWarning(errOut, scanResult.Alert)
			}
			pf.FormatPage(items)
			return nil
		}, pagOpts)
		if err != nil {
			return err
		}
		if apiErr := checkErr(result, pagOpts.Identity); apiErr != nil {
			return apiErr
		}
		if !hasItems {
			fmt.Fprintf(errOut, "warning: this API does not return a list, format %q is not supported, falling back to json\n", format)
			return output.WriteSuccessEnvelope(output.SuccessEnvelopeData(result), output.SuccessEnvelopeOptions{
				CommandPath: commandPath,
				Identity:    string(pagOpts.Identity),
				Out:         out,
				ErrOut:      errOut,
			})
		}
		return nil
	default:
		result, err := ac.PaginateAll(ctx, request, pagOpts)
		if err != nil {
			return err
		}
		if apiErr := checkErr(result, pagOpts.Identity); apiErr != nil {
			output.FormatValue(out, result, output.FormatJSON)
			return apiErr
		}
		return output.WriteSuccessEnvelope(output.SuccessEnvelopeData(result), output.SuccessEnvelopeOptions{
			CommandPath: commandPath,
			Identity:    string(pagOpts.Identity),
			Out:         out,
			ErrOut:      errOut,
		})
	}
}
