// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"fmt"
	"io"
	"sort"
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
	"github.com/larksuite/cli/internal/imcontract"
	"github.com/larksuite/cli/internal/meta"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/recovery"
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
	refs := apicatalog.ServiceMethods(svc, nil)

	// Collect each resource's verbs up front so resourceShort can summarize a
	// resource as its verb list from the first ensureChildCommand call.
	verbs := map[string][]string{}
	for _, ref := range refs {
		key := strings.Join(ref.ResourcePath, ".")
		verbs[key] = append(verbs[key], ref.Method.Name)
	}

	for _, ref := range refs {
		resCmd := svcCmd
		var path []string
		for _, seg := range ref.ResourcePath {
			path = append(path, seg)
			resCmd = ensureChildCommand(resCmd, seg, resourceShort(seg, verbs[strings.Join(path, ".")]))
		}
		resCmd.AddCommand(buildMethodCommand(ctx, f, newMethodCommandSpec(ref), nil, parent.PersistentFlags()))
	}
}

// resourceShort summarizes a resource as its sorted verb list, or the
// "<name> operations" placeholder for an intermediate group with no methods.
func resourceShort(seg string, verbs []string) string {
	if len(verbs) == 0 {
		return seg + " operations"
	}
	sorted := append([]string(nil), verbs...)
	sort.Strings(sorted)
	return strings.Join(sorted, ", ")
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
	ContractKey imcontract.ContractKey

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

	identityDefaulted   bool
	identityWarningSent bool

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
	paginates    bool   // method accepts a page_token param (so --page-all is meaningful)
	serviceName  string // owning service name (e.g. "approval"), for the lazy affordance lookup
	contractKey  imcontract.ContractKey
}

// methodPaginates reports whether a method takes a page_token param, the signal
// that makes the --page-all/--page-limit/--page-delay flags meaningful.
func methodPaginates(m meta.Method) bool {
	for _, f := range m.Params() {
		if f.Name == "page_token" {
			return true
		}
	}
	return false
}

func newMethodCommandSpec(ref apicatalog.MethodRef) methodCommandSpec {
	m := ref.Method
	spec := methodCommandSpec{
		method:       m,
		schemaPath:   ref.SchemaPath(),
		servicePath:  ref.Service.ServicePath,
		serviceName:  ref.Service.Name,
		risk:         m.Risk,
		restricts:    m.RestrictsIdentity(),
		identities:   m.Identities(),
		params:       m.Params(),
		fileFields:   detectFileFields(m),
		acceptsBody:  methodTakesBody(m.HTTPMethod),
		declaresBody: len(m.Data()) > 0 || len(m.Files()) > 0,
		paginates:    methodPaginates(m),
	}
	spec.contractKey = generatedContractKey(ref.Service.Name, m.ID)
	return spec
}

func generatedContractKey(serviceName, methodID string) imcontract.ContractKey {
	if serviceName != "im" || methodID == "" {
		return ""
	}
	i := strings.LastIndex(methodID, ".")
	if i < 0 {
		return ""
	}
	return imcontract.ContractKey(serviceName + " " + methodID[:i] + " " + methodID[i+1:])
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
		ContractKey: spec.contractKey,
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
	// Keep the pagination flags registered (a harmless no-op if passed) but hide
	// them from help on non-paginating commands, so help doesn't imply a
	// get/write can paginate.
	if !spec.paginates {
		for _, name := range []string{"page-all", "page-limit", "page-delay"} {
			_ = cmd.Flags().MarkHidden(name)
		}
	}
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
	// Build-time Long; the agent guidance is added lazily by PrepareMethodHelp
	// (setMethodHelpData records the coordinates it needs).
	paramsOnly := opts.binder.paramsOnlyHelp()
	cmd.Long = methodLong(m.Description, spec.schemaPath, paramsOnly)
	setMethodHelpData(cmd, spec.serviceName, m.ID, spec.schemaPath, paramsOnly)
	imcontract.AnnotateHelpContract(cmd, spec.contractKey)

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
		// Keep the precedence rule on the flag's own one line (not a multi-line
		// note that breaks the one-entry-per-flag rhythm an agent parses). Only
		// meaningful when typed flags exist to override.
		if len(spec.params) > 0 {
			fl.Usage = "Raw URL/query params JSON. Supports - and @file. If both set, typed flags override matching keys in --params."
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
	contract, contractFound := imcontract.Lookup(opts.ContractKey)
	contractManagedWrite := contractFound && contract.Strategy.Kind.IsWrite()
	contractManagedRead := contractFound && contract.Strategy.Kind.IsRead()
	if contractManagedRead && opts.PageAll &&
		contract.Strategy.Kind != imcontract.CollectionReadKind &&
		contract.Strategy.Kind != imcontract.SearchReadKind {
		return newIMReadPageAllValidationError()
	}

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
	opts.identityDefaulted = contractManagedWrite &&
		serviceMethodSupportsUserAndBot(opts.Method) &&
		!serviceIdentityFlagChanged(opts.Cmd) &&
		f.IdentityAutoDetected &&
		!f.ResolveStrictMode(opts.Ctx).IsActive()

	if opts.PageAll && opts.Output != "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--output and --page-all are mutually exclusive").WithParam("--output")
	}
	if err := output.ValidateJqFlags(opts.JqExpr, opts.Output, opts.Format); err != nil {
		return err
	}
	if contractManagedWrite && opts.Output != "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--output is not supported for contract-managed IM write commands").
			WithParam("--output").
			WithHint("remove --output; read the completion result from stdout")
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
		warnServiceIdentityDefaulted(opts)
		if fileMeta != nil {
			return cmdutil.PrintDryRunWithFile(request, config, serviceDryRunOutputOptions(f, opts), *fileMeta)
		}
		return serviceDryRun(f, request, config, opts)
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
	var contractSession *imcontract.Session
	if contractManagedWrite {
		contractSession = imcontract.NewSession(contract)
		requestBody, _ := request.Data.(map[string]any)
		if uuid, ok := request.Params["uuid"].(string); ok && uuid != "" {
			cloned := make(map[string]any, len(requestBody)+1)
			for key, value := range requestBody {
				cloned[key] = value
			}
			cloned["uuid"] = uuid
			requestBody = cloned
		}
		if err := contractSession.ObserveRequest(requestBody); err != nil {
			return err
		}
	}
	var readSession *imcontract.ReadSession
	if contractManagedRead {
		readSession, err = imcontract.NewReadSession(contract, imcontract.ReadOptions{FullRead: opts.PageAll})
		if err != nil {
			return err
		}
	}

	if opts.PageAll {
		if contractSession != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--page-all is not valid for an IM write command").WithParam("--page-all")
		}
		if readSession != nil {
			return servicePaginateIMRead(opts, ac, &request, format, readSession)
		}
		return servicePaginate(opts.Ctx, ac, request, format, opts.JqExpr, out, f.IOStreams.ErrOut, opts.Cmd.CommandPath(),
			client.PaginationOptions{PageLimit: opts.PageLimit, PageDelay: opts.PageDelay}, checkErr)
	}

	if contractSession != nil {
		contractSession.RecordFact(imcontract.Fact{Kind: imcontract.FactWriteAttempted})
	}
	resp, err := ac.DoAPI(opts.Ctx, request)
	if err != nil {
		if contractSession != nil {
			return contractSession.FinalizeError(normalizeIMContractJSONError(err))
		}
		if readSession != nil {
			return readSession.FinalizeError(normalizeIMContractJSONError(err))
		}
		return err
	}
	if contractSession != nil {
		return handleIMWriteContractResponse(opts, resp, format, checkErr, contractSession)
	}
	if readSession != nil {
		return handleIMReadContractResponse(opts, resp, format, checkErr, readSession, request)
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

func handleIMReadContractResponse(
	opts *ServiceMethodOptions,
	resp *larkcore.ApiResp,
	format output.Format,
	checkErr func(interface{}, core.Identity) error,
	session *imcontract.ReadSession,
	request client.RawApiRequest,
) error {
	responseOpts := client.ResponseOptions{
		OutputPath:  opts.Output,
		Format:      format,
		JqExpr:      opts.JqExpr,
		Out:         opts.Factory.IOStreams.Out,
		ErrOut:      opts.Factory.IOStreams.ErrOut,
		FileIO:      opts.Factory.ResolveFileIO(opts.Ctx),
		CommandPath: opts.Cmd.CommandPath(),
		Identity:    opts.As,
		CheckError:  checkErr,
	}
	if resp.StatusCode >= 400 {
		responseErr := client.HandleResponse(resp, responseOpts)
		responseErr = imcontract.NormalizeHTTPError(
			resp.StatusCode,
			resp.Header.Get("x-tt-logid"),
			responseErr,
		)
		return session.FinalizeError(responseErr)
	}
	parsed, err := parseIMContractJSONResponse(resp)
	if err != nil {
		return session.FinalizeError(err)
	}
	if apiErr := checkErr(parsed, opts.As); apiErr != nil {
		return session.FinalizeError(apiErr)
	}
	data := output.SuccessEnvelopeData(parsed)
	if session.RequiresPagination() {
		status, _ := client.InspectPaginationPage(parsed, requestStringParam(request.Params, "page_token"))
		session.ObservePagination(status)
	}
	result, err := session.Finalize(data)
	if err != nil {
		return err
	}
	return writeIMReadResult(opts, format, result, parsed)
}

func servicePaginateIMRead(
	opts *ServiceMethodOptions,
	ac *client.APIClient,
	request *client.RawApiRequest,
	format output.Format,
	session *imcontract.ReadSession,
) error {
	if session == nil {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "IM paginated read requires a read session")
	}
	if !session.RequiresPagination() {
		return newIMReadPageAllValidationError()
	}
	pagOpts := client.PaginationOptions{
		PageLimit:          opts.PageLimit,
		PageDelay:          opts.PageDelay,
		Identity:           opts.As,
		NormalizeHTTPError: imcontract.NormalizeHTTPError,
	}
	if opts.JqExpr == "" && (format == output.FormatNDJSON || format == output.FormatTable || format == output.FormatCSV) {
		return streamIMReadPages(opts, ac, request, format, session, pagOpts)
	}

	merged, status, _ := ac.PaginateAllWithStatus(opts.Ctx, request, pagOpts)
	session.ObservePagination(status)
	data := output.SuccessEnvelopeData(merged)
	result, err := session.Finalize(data)
	if err != nil {
		return err
	}
	return writeIMReadResult(opts, format, result, merged)
}

func newIMReadPageAllValidationError() error {
	return errs.NewValidationError(
		errs.SubtypeInvalidArgument,
		"--page-all is not valid for this IM read command",
	).WithParam("--page-all")
}

func streamIMReadPages(
	opts *ServiceMethodOptions,
	ac *client.APIClient,
	request *client.RawApiRequest,
	format output.Format,
	session *imcontract.ReadSession,
	pagOpts client.PaginationOptions,
) error {
	errOut := opts.Factory.IOStreams.ErrOut
	emitter := newIMServiceEmitter(opts, nil)
	var firstPage map[string]interface{}
	hasItems := false
	status, pageErr := ac.StreamPagesWithStatus(opts.Ctx, request, pagOpts, func(page map[string]interface{}) error {
		if firstPage == nil {
			firstPage = page
		}
		data, _ := page["data"].(map[string]interface{})
		arrayField := output.FindArrayField(data)
		if arrayField == "" {
			return nil
		}
		items, _ := data[arrayField].([]interface{})
		hasItems = true
		return emitter.StreamPage(items, output.StreamOptions{Format: format.String()})
	})
	if pageErr != nil && status.StopReason == "" {
		return session.FinalizeError(pageErr)
	}
	session.ObservePagination(status)
	result, err := session.Finalize(map[string]interface{}{})
	if err != nil {
		return err
	}
	if !hasItems && firstPage != nil {
		fmt.Fprintf(errOut, "warning: this API does not return a list, format %q is not supported, falling back to json\n", format)
		if writeErr := emitIMServiceResult(
			opts,
			output.FormatJSON,
			output.SuccessEnvelopeData(firstPage),
			result.OK,
			result.Meta,
			result.Error,
			result.Hint,
			result.Notice,
			false,
		); writeErr != nil {
			return writeErr
		}
	} else if err := emitter.Hint(result.Hint); err != nil {
		return err
	}
	return readResultExit(result)
}

func writeIMReadResult(
	opts *ServiceMethodOptions,
	format output.Format,
	result imcontract.ReadResult,
	presentation interface{},
) error {
	if opts.JqExpr != "" || format == output.FormatJSON {
		if err := emitIMServiceResult(
			opts,
			format,
			result.Data,
			result.OK,
			result.Meta,
			result.Error,
			result.Hint,
			result.Notice,
			true,
		); err != nil {
			return err
		}
		return readResultExitForProjection(result, opts.JqExpr != "")
	}

	if err := emitIMServiceResult(
		opts,
		format,
		presentation,
		result.OK,
		result.Meta,
		result.Error,
		result.Hint,
		result.Notice,
		false,
	); err != nil {
		return err
	}
	return readResultExitForProjection(result, true)
}

func newIMServiceEmitter(opts *ServiceMethodOptions, readNotice map[string]interface{}) *output.Emitter {
	return output.NewEmitter(output.EmitterConfig{
		Out:         opts.Factory.IOStreams.Out,
		ErrOut:      opts.Factory.IOStreams.ErrOut,
		CommandPath: opts.Cmd.CommandPath(),
		Identity:    string(opts.As),
		NoticeProvider: func() map[string]interface{} {
			base := output.GetNotice()
			if opts.identityDefaulted {
				base = imcontract.WithIdentityDefaultedNotice(base, string(opts.As))
			}
			return mergeIMServiceNotices(base, readNotice)
		},
	})
}

func mergeIMServiceNotices(base, extra map[string]interface{}) map[string]interface{} {
	if len(base) == 0 && len(extra) == 0 {
		return nil
	}
	merged := make(map[string]interface{}, len(base)+len(extra))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range extra {
		merged[key] = value
	}
	return merged
}

func emitIMServiceResult(
	opts *ServiceMethodOptions,
	format output.Format,
	data interface{},
	ok bool,
	meta *output.Meta,
	resultError *errs.Problem,
	hint string,
	readNotice map[string]interface{},
	projectedRead bool,
) error {
	warnServiceIdentityDefaulted(opts)
	var errorValue interface{}
	if resultError != nil {
		errorValue = resultError
	}
	emitOpts := output.EmitOptions{
		Format: format.String(),
		JQ:     opts.JqExpr,
		Meta:   meta,
		Error:  errorValue,
		Hint:   hint,
		HintToStderr: hint != "" &&
			((projectedRead && opts.JqExpr != "") ||
				(opts.JqExpr == "" && format != output.FormatJSON)),
	}
	emitter := newIMServiceEmitter(opts, readNotice)
	if !ok && (opts.JqExpr != "" || format == output.FormatJSON) {
		return emitter.PartialFailure(data, emitOpts)
	}
	return emitter.Success(data, emitOpts)
}

func serviceMethodSupportsUserAndBot(method meta.Method) bool {
	return method.SupportsToken(meta.TokenUser) && method.SupportsToken(meta.TokenTenant)
}

func serviceIdentityFlagChanged(cmd *cobra.Command) bool {
	return cmd != nil && cmd.Flags().Changed("as")
}

func warnServiceIdentityDefaulted(opts *ServiceMethodOptions) {
	if opts == nil || !opts.identityDefaulted || opts.identityWarningSent {
		return
	}
	opts.identityWarningSent = true
	fmt.Fprintf(opts.Factory.IOStreams.ErrOut, "warning: %s: %s\n",
		imcontract.IdentityDefaultedNoticeKey,
		imcontract.IdentityDefaultedMessage(string(opts.As)))
}

func readResultExit(result imcontract.ReadResult) error {
	if result.ExitCode == 0 {
		return nil
	}
	if result.Cause != nil {
		return result.Cause
	}
	return output.PartialFailure(result.ExitCode)
}

func readResultExitForProjection(result imcontract.ReadResult, projected bool) error {
	if result.ExitCode == 0 {
		return nil
	}
	if projected && result.Cause != nil {
		return result.Cause
	}
	return output.PartialFailure(result.ExitCode)
}

func requestStringParam(params map[string]interface{}, name string) string {
	value, _ := params[name].(string)
	return value
}

func handleIMWriteContractResponse(
	opts *ServiceMethodOptions,
	resp *larkcore.ApiResp,
	format output.Format,
	checkErr func(interface{}, core.Identity) error,
	session *imcontract.Session,
) error {
	responseOpts := client.ResponseOptions{
		OutputPath:  opts.Output,
		Format:      format,
		JqExpr:      opts.JqExpr,
		Out:         opts.Factory.IOStreams.Out,
		ErrOut:      opts.Factory.IOStreams.ErrOut,
		FileIO:      opts.Factory.ResolveFileIO(opts.Ctx),
		CommandPath: opts.Cmd.CommandPath(),
		Identity:    opts.As,
		CheckError:  checkErr,
	}
	if resp.StatusCode >= 400 {
		responseErr := client.HandleResponse(resp, responseOpts)
		responseErr = imcontract.NormalizeHTTPError(
			resp.StatusCode,
			resp.Header.Get("x-tt-logid"),
			responseErr,
		)
		return session.FinalizeError(responseErr)
	}
	parsed, err := parseIMContractJSONResponse(resp)
	if err != nil {
		return session.FinalizeError(err)
	}
	if apiErr := checkErr(parsed, opts.As); apiErr != nil {
		return session.FinalizeError(apiErr)
	}
	data := output.SuccessEnvelopeData(parsed)
	if m, ok := data.(map[string]any); ok {
		session.ObserveResponse(m)
	}
	result, err := session.FinalizeSuccess(data)
	if err != nil {
		return err
	}

	emitErr := emitIMServiceResult(
		opts,
		format,
		result.Data,
		result.OK,
		nil,
		nil,
		result.Hint,
		nil,
		false,
	)
	if emitErr != nil {
		if errs.IsContentSafety(emitErr) {
			return writeIMContentSafetyFallback(opts, result)
		}
		if opts.JqExpr != "" {
			writeIMJQDiagnostic(opts.Factory.IOStreams.ErrOut)
			return writeIMJQFallback(opts, result)
		}
		return emitErr
	}
	if result.ExitCode != 0 {
		return output.PartialFailure(result.ExitCode)
	}
	return nil
}

func parseIMContractJSONResponse(resp *larkcore.ApiResp) (interface{}, error) {
	if resp == nil {
		return nil, newIMContractJSONResponseError(resp)
	}
	parsed, err := client.ParseJSONResponse(resp)
	if err != nil {
		return nil, newIMContractJSONResponseError(resp)
	}
	return parsed, nil
}

func newIMContractJSONResponseError(resp *larkcore.ApiResp) *errs.InternalError {
	contractErr := errs.NewInternalError(
		errs.SubtypeInvalidResponse,
		"IM contract response must be valid JSON",
	)
	if resp == nil {
		return contractErr
	}
	if logID := resp.Header.Get("x-tt-logid"); logID != "" {
		contractErr.WithLogID(logID)
	}
	return contractErr
}

func normalizeIMContractJSONError(err error) error {
	problem, ok := errs.ProblemOf(err)
	if ok && problem.Subtype == errs.SubtypeInvalidResponse {
		normalized := newIMContractJSONResponseError(nil)
		if problem.Code != 0 {
			normalized.WithCode(problem.Code)
		}
		if problem.LogID != "" {
			normalized.WithLogID(problem.LogID)
		}
		return normalized
	}
	return err
}

func writeIMJQFallback(opts *ServiceMethodOptions, result imcontract.Result) error {
	env, signal := imcontract.BuildJQOutputFallback(result)
	if err := newIMServiceEmitter(opts, nil).RedactedFallback(env); err != nil {
		return err
	}
	return signal
}

func writeIMJQDiagnostic(errOut io.Writer) {
	fmt.Fprintln(errOut, "error: jq projection failed after the IM write completed; inspect --jq")
}

func writeIMContentSafetyFallback(opts *ServiceMethodOptions, result imcontract.Result) error {
	env, signal := imcontract.BuildContentSafetyOutputFallback(result)
	if err := newIMServiceEmitter(opts, nil).RedactedFallback(env); err != nil {
		return err
	}
	return signal
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
// BuildAPIError path. It records the same typed facts and canonical message;
// the root presenter supplies identity-appropriate recovery at the final
// command boundary.
// ConsoleURL is deliberately omitted: the dispatcher only sets it for
// SubtypeAppScopeNotApplied (bot-perspective dev-action recovery), and this
// pre-flight path is user-perspective SubtypeMissingScope whose recovery is
// `lark-cli auth login --scope ...`, not a console deep-link.
func newPreflightMissingScopeError(brand, appID, identity string, missing []string) error {
	return errclass.NewMissingScopeError(brand, appID, identity, missing)
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
func missingParamHint(opts *ServiceMethodOptions, f meta.Field) recovery.Hint {
	paramsForm := fmt.Sprintf("--params '{%q: \"<value>\"}'", f.Name)
	var input string
	if opts.binder.hasTypedFlag(f.Name) {
		input = fmt.Sprintf("set --%s <value> (or %s)", f.FlagName(), paramsForm)
	} else {
		input = fmt.Sprintf("set %s", paramsForm)
	}
	return recovery.Join("; ",
		recovery.Text(input),
		recovery.Command(recovery.TargetSchema, "see: lark-cli schema "+opts.SchemaPath),
	)
}

func missingRequiredParamError(opts *ServiceMethodOptions, f meta.Field, location string) error {
	hint := missingParamHint(opts, f)
	return recovery.Attach(
		errs.NewValidationError(errs.SubtypeInvalidArgument,
			"missing required %s parameter: %s", location, f.Name).
			WithParam(f.Name),
		hint,
	)
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
			return client.RawApiRequest{}, nil, missingRequiredParamError(opts, s, "path")
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
			return client.RawApiRequest{}, nil, missingRequiredParamError(opts, s, "query")
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

func serviceDryRun(f *cmdutil.Factory, request client.RawApiRequest, config *core.CliConfig, opts *ServiceMethodOptions) error {
	return cmdutil.PrintDryRun(request, config, serviceDryRunOutputOptions(f, opts))
}

func serviceDryRunOutputOptions(f *cmdutil.Factory, opts *ServiceMethodOptions) cmdutil.DryRunOutputOptions {
	return cmdutil.DryRunOutputOptions{
		Format:      opts.Format,
		JqExpr:      opts.JqExpr,
		CommandPath: opts.Cmd.CommandPath(),
		Identity:    opts.As,
		Out:         f.IOStreams.Out,
		ErrOut:      f.IOStreams.ErrOut,
		NoticeProvider: func() map[string]interface{} {
			base := output.GetNotice()
			if !opts.identityDefaulted {
				return base
			}
			return imcontract.WithIdentityDefaultedNotice(base, string(opts.As))
		},
	}
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
		emitter := output.NewEmitter(output.EmitterConfig{
			Out:            out,
			ErrOut:         errOut,
			CommandPath:    commandPath,
			Identity:       string(pagOpts.Identity),
			NoticeProvider: output.GetNotice,
		})
		result, hasItems, err := ac.StreamPages(ctx, request, func(items []interface{}) error {
			// Streaming formats intentionally emit each page after that page has
			// passed safety scanning. A later page may still fail, so callers
			// must use the exit code to distinguish complete vs partial output.
			return emitter.StreamPage(items, output.StreamOptions{Format: format.String()})
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
