// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package api

import (
	"context"
	"regexp"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/spf13/cobra"
)

// APIOptions holds all inputs for the api command.
type APIOptions struct {
	Factory *cmdutil.Factory
	Cmd     *cobra.Command
	Ctx     context.Context

	// Positional args
	Method string
	Path   string

	// Flags
	Params    string
	Data      string
	As        core.Identity
	Output    string
	PageAll   bool
	PageSize  int
	PageLimit int
	PageDelay int
	Format    string
	JqExpr    string
	DryRun    bool
	File      string
}

var urlPrefixRe = regexp.MustCompile(`https?://[^/]+(/open-apis/.+)`)

func normalisePath(raw string) string {
	if matches := urlPrefixRe.FindStringSubmatch(raw); len(matches) > 1 {
		raw = matches[1]
	} else if !strings.HasPrefix(raw, "/open-apis/") {
		raw = "/open-apis/" + strings.TrimPrefix(raw, "/")
	}
	return validate.StripQueryFragment(raw)
}

// NewCmdApi creates the api command. If runF is non-nil it is called instead of apiRun (test hook).
func NewCmdApi(f *cmdutil.Factory, runF func(*APIOptions) error) *cobra.Command {
	return NewCmdApiWithContext(context.Background(), f, runF)
}

func NewCmdApiWithContext(ctx context.Context, f *cmdutil.Factory, runF func(*APIOptions) error) *cobra.Command {
	opts := &APIOptions{Factory: f}
	var asStr string

	cmd := &cobra.Command{
		Use:   "api <method> <path>",
		Short: "Raw HTTP escape hatch — call any endpoint by path (fallback when no typed command exists)",
		Long: `Raw HTTP escape hatch: send any Lark API request by HTTP method + path.

Prefer the typed domain command when one exists — it validates parameters,
shows the Risk level, gates destructive calls behind --yes, and carries usage
guidance that this raw command does not. If a domain command covers your task
(browse with ` + "`lark-cli <domain> --help`" + `), use it instead of this.

Reach for ` + "`api`" + ` only for endpoints that have no typed command yet (e.g.
newer/preview APIs), where you already have the HTTP path from the Lark docs.

Examples:
  lark-cli api GET /open-apis/calendar/v4/calendars
  lark-cli api POST /open-apis/im/v1/messages --params '{"receive_id_type":"open_id"}' --data @body.json`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Method = strings.ToUpper(args[0])
			opts.Path = args[1]
			opts.Cmd = cmd
			opts.Ctx = cmd.Context()
			opts.As = core.Identity(asStr)
			if runF != nil {
				return runF(opts)
			}
			return apiRun(opts)
		},
	}

	cmd.Flags().StringVar(&opts.Params, "params", "", "query parameters JSON (supports - for stdin, @file for file input)")
	cmd.Flags().StringVar(&opts.Data, "data", "", "request body JSON (supports - for stdin, @file for file input)")
	cmdutil.AddAPIIdentityFlag(ctx, cmd, f, &asStr)
	cmd.Flags().StringVarP(&opts.Output, "output", "o", "", "output file path for binary responses")
	cmd.Flags().BoolVar(&opts.PageAll, "page-all", false, "automatically paginate through all pages")
	cmd.Flags().IntVar(&opts.PageSize, "page-size", 0, "page size (0 = use API default)")
	cmd.Flags().IntVar(&opts.PageLimit, "page-limit", 10, "max pages to fetch with --page-all (0 = unlimited)")
	cmd.Flags().IntVar(&opts.PageDelay, "page-delay", 200, "delay in ms between pages")
	cmd.Flags().StringVar(&opts.Format, "format", "json", "output format: json|ndjson|table|csv")
	cmd.Flags().Bool("json", false, "shorthand for --format json")
	cmd.Flags().StringVarP(&opts.JqExpr, "jq", "q", "", "jq expression to filter JSON output")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "print request without executing")
	cmd.Flags().StringVar(&opts.File, "file", "", "file to upload as multipart/form-data ([field=]path, supports - for stdin)")

	cmd.ValidArgsFunction = func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return []string{"GET", "POST", "PUT", "PATCH", "DELETE"}, cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	cmdutil.RegisterFlagCompletion(cmd, "format", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"json", "ndjson", "table", "csv"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmdutil.SetRisk(cmd, "write")

	return cmd
}

// buildAPIRequest validates flags and builds a RawApiRequest.
// When dryRun is true and a file is provided, file reading is skipped and
// FileUploadMeta is returned instead so the caller can render dry-run output.
func buildAPIRequest(opts *APIOptions) (client.RawApiRequest, *cmdutil.FileUploadMeta, error) {
	stdin := opts.Factory.IOStreams.In
	fileIO := opts.Factory.ResolveFileIO(opts.Ctx)

	if opts.Method == "" {
		return client.RawApiRequest{}, nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"HTTP method must not be empty").
			WithHint("pass the verb as the first argument, e.g. lark-cli api GET /open-apis/...").
			WithParam("<method>")
	}

	// Validate --file mutual exclusions first.
	if err := cmdutil.ValidateFileFlag(opts.File, opts.Params, opts.Data, opts.Output, opts.PageAll, opts.Method); err != nil {
		return client.RawApiRequest{}, nil, err
	}

	// stdin conflict: --params and --data cannot both read from stdin, regardless of --file.
	if opts.Params == "-" && opts.Data == "-" {
		return client.RawApiRequest{}, nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--params and --data cannot both read from stdin (-)").
			WithHint("pass at most one flag as '-'; give the other inline JSON or @file").
			WithParams(
				errs.InvalidParam{Name: "--params", Reason: "reads from stdin (-)"},
				errs.InvalidParam{Name: "--data", Reason: "reads from stdin (-)"},
			)
	}

	params, err := cmdutil.ParseJSONMap(opts.Params, "--params", stdin, fileIO)
	if err != nil {
		return client.RawApiRequest{}, nil, err
	}
	if opts.PageSize > 0 {
		params["page_size"] = opts.PageSize
	}

	request := client.RawApiRequest{
		Method: opts.Method,
		URL:    normalisePath(opts.Path),
		Params: params,
		As:     opts.As,
	}

	if opts.File != "" {
		// File upload path: build formdata.
		fieldName, filePath, isStdin := cmdutil.ParseFileFlag(opts.File, "file")

		// Parse --data as JSON map for form fields (not as body).
		var dataFields any
		if opts.Data != "" {
			dataFields, err = cmdutil.ParseOptionalBody(opts.Method, opts.Data, stdin, fileIO)
			if err != nil {
				return client.RawApiRequest{}, nil, err
			}
			if _, ok := dataFields.(map[string]any); !ok {
				return client.RawApiRequest{}, nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
					"--data must be a JSON object when used with --file").
					WithHint(`with --file, --data carries multipart form fields, e.g. --data '{"image_type":"message"}'`).
					WithParam("--data")
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
		// Normal path: JSON body.
		data, err := cmdutil.ParseOptionalBody(opts.Method, opts.Data, stdin, fileIO)
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

func apiRun(opts *APIOptions) error {
	f := opts.Factory
	opts.As = f.ResolveAs(opts.Ctx, opts.Cmd, opts.As)

	if err := f.CheckStrictMode(opts.Ctx, opts.As); err != nil {
		return err
	}

	if opts.PageAll && opts.Output != "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--output and --page-all are mutually exclusive").
			WithHint("drop --page-all to save a binary response, or drop --output to paginate JSON").
			WithParams(
				errs.InvalidParam{Name: "--output", Reason: "conflicts with --page-all"},
				errs.InvalidParam{Name: "--page-all", Reason: "conflicts with --output"},
			)
	}
	// Parse before the dry-run branch so both dry-run and emit reject unknown
	// values. Raw API responses accept four formats; pretty remains available
	// only for the dry-run request preview handled below.
	format, ok := output.ParseFormat(opts.Format)
	if !ok {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"unknown output format %q (want json, ndjson, table, or csv)", opts.Format).
			WithParam("--format")
	}
	if err := output.ValidateJqFlags(opts.JqExpr, opts.Output, opts.Format); err != nil {
		return err
	}

	request, fileMeta, err := buildAPIRequest(opts)
	if err != nil {
		return err
	}

	config, err := f.Config()
	if err != nil {
		return err
	}

	if opts.DryRun {
		if fileMeta != nil {
			return cmdutil.PrintDryRunWithFile(request, config, dryRunOutputOptions(f, opts, format), *fileMeta)
		}
		return apiDryRun(f, request, config, opts, format)
	}
	// pretty is a shortcut-only presentation format; the raw api command has no
	// pretty renderer for responses, so reject it before client init rather than
	// fall back. (Dry-run keeps its own plain-text pretty preview, handled above.)
	if format == output.FormatPretty {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--format pretty is not supported here (use json, ndjson, table, or csv)").
			WithParam("--format")
	}
	// Identity info is now included in the JSON envelope; skip stderr printing.
	// cmdutil.PrintIdentity(f.IOStreams.ErrOut, opts.As, config, f.IdentityAutoDetected)

	ac, err := f.NewAPIClientWithConfig(config)
	if err != nil {
		return err
	}

	out := f.IOStreams.Out

	if opts.PageAll {
		return client.PaginateToOutput(opts.Ctx, client.PaginateOutputOptions{
			Client:      ac,
			Request:     request,
			Format:      format,
			JqExpr:      opts.JqExpr,
			Out:         out,
			ErrOut:      f.IOStreams.ErrOut,
			CommandPath: opts.Cmd.CommandPath(),
			Pagination:  client.PaginationOptions{PageLimit: opts.PageLimit, PageDelay: opts.PageDelay},
			CheckErr:    ac.CheckResponse,
			MarkErr:     errs.MarkRaw,
		})
	}

	resp, err := ac.DoAPI(opts.Ctx, request)
	if err != nil {
		// MarkRaw tells the dispatcher to skip the legacy enrichPermissionError
		// pass on *output.ExitError values. Typed *errs.* errors that flow
		// through here keep their canonical message / hint from BuildAPIError;
		// MarkRaw is a no-op on those (it only flips a flag on *ExitError).
		return errs.MarkRaw(err)
	}
	err = client.HandleResponse(resp, client.ResponseOptions{
		OutputPath:  opts.Output,
		Format:      format,
		JqExpr:      opts.JqExpr,
		Out:         out,
		ErrOut:      f.IOStreams.ErrOut,
		FileIO:      f.ResolveFileIO(opts.Ctx),
		CommandPath: opts.Cmd.CommandPath(),
		Identity:    opts.As,
		// CheckResponse routes through errclass.BuildAPIError for known Lark
		// codes (typed PermissionError / AuthenticationError / ...). For
		// unknown codes it falls back to *errs.APIError. The Brand+AppID on
		// the client populate identity-aware fields (ConsoleURL etc.).
		CheckError: ac.CheckResponse,
	})
	// MarkRaw: see comment above on the DoAPI path. Skips legacy
	// *ExitError enrichment; typed errors flow through unchanged.
	if err != nil {
		return errs.MarkRaw(err)
	}
	return nil
}

func apiDryRun(f *cmdutil.Factory, request client.RawApiRequest, config *core.CliConfig, opts *APIOptions, format output.Format) error {
	return cmdutil.PrintDryRun(request, config, dryRunOutputOptions(f, opts, format))
}

func dryRunOutputOptions(f *cmdutil.Factory, opts *APIOptions, format output.Format) cmdutil.DryRunOutputOptions {
	return cmdutil.DryRunOutputOptions{
		Format:      format.String(),
		JqExpr:      opts.JqExpr,
		CommandPath: opts.Cmd.CommandPath(),
		Identity:    opts.As,
		Out:         f.IOStreams.Out,
		ErrOut:      f.IOStreams.ErrOut,
	}
}
