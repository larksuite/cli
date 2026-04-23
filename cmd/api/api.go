// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package api

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"

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
		Short: "Generic Lark API requests",
		Args:  cobra.ExactArgs(2),
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

	cmd.Flags().StringVar(&opts.Params, "params", "", "query parameters JSON (supports - for stdin)")
	cmd.Flags().StringVar(&opts.Data, "data", "", "request body JSON (supports - for stdin)")
	cmdutil.AddAPIIdentityFlag(ctx, cmd, f, &asStr)
	cmd.Flags().StringVarP(&opts.Output, "output", "o", "", "output file path for binary responses")
	cmd.Flags().BoolVar(&opts.PageAll, "page-all", false, "automatically paginate through all pages")
	cmd.Flags().IntVar(&opts.PageSize, "page-size", 0, "page size (0 = use API default)")
	cmd.Flags().IntVar(&opts.PageLimit, "page-limit", 10, "max pages to fetch with --page-all (0 = unlimited)")
	cmd.Flags().IntVar(&opts.PageDelay, "page-delay", 200, "delay in ms between pages")
	cmd.Flags().StringVar(&opts.Format, "format", "json", "output format: json|ndjson|table|csv")
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

	return cmd
}

// buildAPIRequest validates flags and builds a RawApiRequest.
// When dryRun is true and a file is provided, file reading is skipped and
// FileUploadMeta is returned instead so the caller can render dry-run output.
func buildAPIRequest(opts *APIOptions) (client.RawApiRequest, *cmdutil.FileUploadMeta, error) {
	stdin := opts.Factory.IOStreams.In

	// Validate --file mutual exclusions first.
	if err := cmdutil.ValidateFileFlag(opts.File, opts.Params, opts.Data, opts.Output, opts.PageAll, opts.Method); err != nil {
		return client.RawApiRequest{}, nil, err
	}

	// stdin conflict: --params and --data cannot both read from stdin, regardless of --file.
	if opts.Params == "-" && opts.Data == "-" {
		return client.RawApiRequest{}, nil, output.ErrValidation("--params and --data cannot both read from stdin (-)")
	}

	params, err := cmdutil.ParseJSONMap(opts.Params, "--params", stdin)
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
			dataFields, err = cmdutil.ParseOptionalBody(opts.Method, opts.Data, stdin)
			if err != nil {
				return client.RawApiRequest{}, nil, err
			}
			if _, ok := dataFields.(map[string]any); !ok {
				return client.RawApiRequest{}, nil, output.ErrValidation("--data must be a JSON object when used with --file")
			}
		}

		if opts.DryRun {
			return request, &cmdutil.FileUploadMeta{
				FieldName: fieldName, FilePath: filePath, FormFields: dataFields,
			}, nil
		}

		fd, err := cmdutil.BuildFormdata(
			opts.Factory.ResolveFileIO(opts.Ctx),
			fieldName, filePath, isStdin, stdin, dataFields,
		)
		if err != nil {
			return client.RawApiRequest{}, nil, err
		}
		request.Data = fd
		request.ExtraOpts = append(request.ExtraOpts, larkcore.WithFileUpload())
	} else {
		// Normal path: JSON body.
		data, err := cmdutil.ParseOptionalBody(opts.Method, opts.Data, stdin)
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
		return output.ErrValidation("--output and --page-all are mutually exclusive")
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
			return cmdutil.PrintDryRunWithFile(f.IOStreams.Out, request, config, opts.Format, fileMeta.FieldName, fileMeta.FilePath, fileMeta.FormFields)
		}
		return apiDryRun(f, request, config, opts.Format)
	}
	// Identity info is now included in the JSON envelope; skip stderr printing.
	// cmdutil.PrintIdentity(f.IOStreams.ErrOut, opts.As, config, f.IdentityAutoDetected)

	ac, err := f.NewAPIClientWithConfig(config)
	if err != nil {
		return err
	}

	out := f.IOStreams.Out
	format, formatOK := output.ParseFormat(opts.Format)
	if !formatOK {
		fmt.Fprintf(f.IOStreams.ErrOut, "warning: unknown format %q, falling back to json\n", opts.Format)
	}

	if opts.PageAll {
		return apiPaginate(opts.Ctx, ac, request, format, opts.JqExpr, out, f.IOStreams.ErrOut,
			client.PaginationOptions{PageLimit: opts.PageLimit, PageDelay: opts.PageDelay})
	}

	resp, err := ac.DoAPI(opts.Ctx, request)
	if err != nil {
		return output.MarkRaw(client.WrapDoAPIError(err))
	}
	err = client.HandleResponse(resp, client.ResponseOptions{
		OutputPath: opts.Output,
		Format:     format,
		JqExpr:     opts.JqExpr,
		Out:        out,
		ErrOut:     f.IOStreams.ErrOut,
		FileIO:     f.ResolveFileIO(opts.Ctx),
	})
	// MarkRaw tells root error handler to skip enrichPermissionError,
	// preserving the original API error detail (log_id, troubleshooter, etc.).
	if err != nil {
		return output.MarkRaw(err)
	}
	return nil
}

func apiDryRun(f *cmdutil.Factory, request client.RawApiRequest, config *core.CliConfig, format string) error {
	return cmdutil.PrintDryRun(f.IOStreams.Out, request, config, format)
}

func apiPaginate(ctx context.Context, ac *client.APIClient, request client.RawApiRequest, format output.Format, jqExpr string, out, errOut io.Writer, pagOpts client.PaginationOptions) error {
	// When jq is set, always aggregate all pages then filter.
	if jqExpr != "" {
		if err := client.PaginateWithJq(ctx, ac, request, jqExpr, out, pagOpts, client.CheckLarkResponse); err != nil {
			return output.MarkRaw(err)
		}
		return nil
	}

	switch format {
	case output.FormatNDJSON, output.FormatTable, output.FormatCSV:
		pf := output.NewPaginatedFormatter(out, format)
		result, hasItems, err := ac.StreamPages(ctx, request, func(items []interface{}) {
			pf.FormatPage(items)
		}, pagOpts)
		if err != nil {
			return output.MarkRaw(output.ErrNetwork("API call failed: %v", err))
		}
		if apiErr := client.CheckLarkResponse(result); apiErr != nil {
			output.FormatValue(out, result, output.FormatJSON)
			return output.MarkRaw(apiErr)
		}
		if !hasItems {
			fmt.Fprintf(errOut, "warning: this API does not return a list, format %q is not supported, falling back to json\n", format)
			output.FormatValue(out, result, output.FormatJSON)
		}
		return nil
	default:
		result, err := ac.PaginateAll(ctx, request, pagOpts)
		if err != nil {
			return output.MarkRaw(output.ErrNetwork("API call failed: %v", err))
		}
		if apiErr := client.CheckLarkResponse(result); apiErr != nil {
			output.FormatValue(out, result, output.FormatJSON)
			return output.MarkRaw(apiErr)
		}
		output.FormatValue(out, result, format)
		return nil
	}
}
