// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/internal/schema"
	"github.com/larksuite/cli/internal/util"
	"github.com/spf13/cobra"
)

// SchemaOptions holds all inputs for the schema command.
type SchemaOptions struct {
	Factory *cmdutil.Factory
	Ctx     context.Context

	// Positional args
	Path      string   // first positional, when only one is given
	ExtraArgs []string // 2nd+ positional args (space-separated form)

	// Flags
	Format string
}

func printServices(w io.Writer) {
	services := registry.ListFromMetaProjects()
	fmt.Fprintf(w, "%sAvailable services:%s\n\n", output.Bold, output.Reset)
	for _, s := range services {
		spec := registry.LoadFromMeta(s)
		title := registry.GetStrFromMap(spec, "title")
		if title == "" {
			title = registry.GetStrFromMap(spec, "description")
		}
		fmt.Fprintf(w, "  %s%s%s  %s%s%s\n", output.Cyan, s, output.Reset, output.Dim, title, output.Reset)
	}
	fmt.Fprintf(w, "\n%sUsage: lark-cli schema <service>.<resource>.<method>%s\n", output.Dim, output.Reset)
}

func printResourceList(w io.Writer, spec map[string]interface{}, mode core.StrictMode) {
	name := registry.GetStrFromMap(spec, "name")
	version := registry.GetStrFromMap(spec, "version")
	title := registry.GetStrFromMap(spec, "title")
	if title == "" {
		title = registry.GetStrFromMap(spec, "description")
	}
	servicePath := registry.GetStrFromMap(spec, "servicePath")

	fmt.Fprintf(w, "%s%s%s (%s) — %s\n\n", output.Bold, name, output.Reset, version, title)
	fmt.Fprintf(w, "%sBase path: %s%s\n\n", output.Dim, servicePath, output.Reset)

	resources, _ := spec["resources"].(map[string]interface{})
	for _, resName := range sortedKeys(resources) {
		resMap, _ := resources[resName].(map[string]interface{})
		methods, _ := resMap["methods"].(map[string]interface{})
		methods = filterMethodsByStrictMode(methods, mode)
		if len(methods) == 0 {
			continue
		}
		fmt.Fprintf(w, "  %s%s%s\n", output.Cyan, resName, output.Reset)
		for _, methodName := range sortedKeys(methods) {
			m, _ := methods[methodName].(map[string]interface{})
			httpMethod := registry.GetStrFromMap(m, "httpMethod")
			desc := registry.GetStrFromMap(m, "description")
			danger := ""
			if d, _ := m["danger"].(bool); d {
				danger = fmt.Sprintf(" %s[danger]%s", output.Red, output.Reset)
			}
			fmt.Fprintf(w, "    %-7s %s%s%s  %s%s%s%s\n", httpMethod, output.Bold, methodName, output.Reset, output.Dim, desc, output.Reset, danger)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "%sUsage: lark-cli schema %s.<resource>.<method>%s\n", output.Dim, name, output.Reset)
}

// hasFileFields returns true if any requestBody field has type "file".
func hasFileFields(method map[string]interface{}) (bool, []string) {
	names := cmdutil.DetectFileFields(method)
	return len(names) > 0, names
}

func printMethodDetail(w io.Writer, spec map[string]interface{}, resName, methodName string, method map[string]interface{}) {
	servicePath := registry.GetStrFromMap(spec, "servicePath")
	specName := registry.GetStrFromMap(spec, "name")
	methodPath := registry.GetStrFromMap(method, "path")
	fullPath := servicePath + "/" + methodPath
	httpMethod := registry.GetStrFromMap(method, "httpMethod")
	desc := registry.GetStrFromMap(method, "description")
	isFileUpload, fileFieldNames := hasFileFields(method)

	fmt.Fprintf(w, "%s%s.%s.%s%s\n\n", output.Bold, specName, resName, methodName, output.Reset)

	httpColor := output.Yellow
	if httpMethod == "GET" {
		httpColor = output.Green
	} else if httpMethod == "DELETE" {
		httpColor = output.Red
	}
	fmt.Fprintf(w, "  %s%s%s %s\n", httpColor, httpMethod, output.Reset, fullPath)
	if desc != "" {
		fmt.Fprintf(w, "  %s\n", desc)
	}
	fmt.Fprintln(w)

	// Parameters
	params, _ := method["parameters"].(map[string]interface{})
	if len(params) > 0 {
		fmt.Fprintf(w, "%sParameters:%s\n\n", output.Bold, output.Reset)
		fmt.Fprintf(w, "  %s--params%s  <json>  %soptional%s\n", output.Cyan, output.Reset, output.Dim, output.Reset)
		for _, paramName := range sortedParamKeys(params) {
			p, _ := params[paramName].(map[string]interface{})
			pType := registry.GetStrFromMap(p, "type")
			if pType == "" {
				pType = "string"
			}
			location := registry.GetStrFromMap(p, "location")
			required, _ := p["required"].(bool)
			reqStr := fmt.Sprintf("%soptional%s", output.Dim, output.Reset)
			if required {
				reqStr = fmt.Sprintf("%srequired%s", output.Red, output.Reset)
			}
			locColor := output.Dim
			if location == "path" {
				locColor = output.Yellow
			}
			// Options (enum values)
			optStr := formatOptions(p)
			fmt.Fprintf(w, "      - %s%s%s (%s, %s%s%s, %s)%s\n", output.Cyan, paramName, output.Reset, pType, locColor, location, output.Reset, reqStr, optStr)
			if pdesc := registry.GetStrFromMap(p, "description"); pdesc != "" {
				pdesc = util.TruncateStrWithEllipsis(pdesc, 100)
				fmt.Fprintf(w, "        %s%s%s\n", output.Dim, pdesc, output.Reset)
			}
			if ex := registry.GetStrFromMap(p, "example"); ex != "" {
				fmt.Fprintf(w, "        %se.g. %s%s\n", output.Dim, ex, output.Reset)
			}
			if rangeStr := formatRange(p); rangeStr != "" {
				fmt.Fprintf(w, "        %srange: %s%s\n", output.Dim, rangeStr, output.Reset)
			}
		}
		fmt.Fprintln(w)
	}

	// --data for write methods
	if httpMethod == "POST" || httpMethod == "PUT" || httpMethod == "PATCH" || httpMethod == "DELETE" {
		if len(params) == 0 {
			fmt.Fprintf(w, "%sParameters:%s\n\n", output.Bold, output.Reset)
		}
		fileUploadTag := ""
		if isFileUpload {
			fileUploadTag = fmt.Sprintf("  %s[file upload]%s", output.Yellow, output.Reset)
		}
		fmt.Fprintf(w, "  %s--data%s  <json>  %soptional%s%s\n", output.Cyan, output.Reset, output.Dim, output.Reset, fileUploadTag)
		requestBody, _ := method["requestBody"].(map[string]interface{})
		if len(requestBody) > 0 {
			printNestedFields(w, requestBody, "      ", "")
		}

		if isFileUpload {
			if len(fileFieldNames) == 1 {
				fmt.Fprintf(w, "\n  %s--file%s  <[field=]path>  %sfile upload%s\n", output.Cyan, output.Reset, output.Dim, output.Reset)
				fmt.Fprintf(w, "      Upload file as multipart/form-data. Default field: %q\n", fileFieldNames[0])
			} else {
				fmt.Fprintf(w, "\n  %s--file%s  <field=path>  %sfile upload%s\n", output.Cyan, output.Reset, output.Dim, output.Reset)
				fmt.Fprintf(w, "      Upload file as multipart/form-data. Fields: %s\n", strings.Join(fileFieldNames, ", "))
			}
		}
		fmt.Fprintln(w)
	}

	// Response
	responseBody, _ := method["responseBody"].(map[string]interface{})
	if len(responseBody) > 0 {
		fmt.Fprintf(w, "%sResponse:%s\n\n", output.Bold, output.Reset)
		printNestedFields(w, responseBody, "  ", "")
		fmt.Fprintln(w)
	}

	// Identity
	if tokens, ok := method["accessTokens"].([]interface{}); ok && len(tokens) > 0 {
		var identities []string
		for _, t := range tokens {
			if s, ok := t.(string); ok {
				switch s {
				case "user":
					identities = append(identities, "user")
				case "tenant":
					identities = append(identities, "bot")
				}
			}
		}
		if len(identities) > 0 {
			fmt.Fprintf(w, "%sIdentity:%s %s\n", output.Bold, output.Reset, strings.Join(identities, ", "))
		}
	}

	// Scopes (all)
	if scopes, ok := method["scopes"].([]interface{}); ok && len(scopes) > 0 {
		var scopeStrs []string
		for _, s := range scopes {
			if str, ok := s.(string); ok {
				scopeStrs = append(scopeStrs, str)
			}
		}
		fmt.Fprintf(w, "%sScopes:%s   %s\n", output.Bold, output.Reset, strings.Join(scopeStrs, ", "))
	}

	// CLI example
	if isFileUpload && len(fileFieldNames) == 1 {
		fmt.Fprintf(w, "%sCLI:%s      lark-cli %s %s %s --file <path>\n", output.Bold, output.Reset, specName, resName, methodName)
	} else if isFileUpload {
		fmt.Fprintf(w, "%sCLI:%s      lark-cli %s %s %s --file <field=path>\n", output.Bold, output.Reset, specName, resName, methodName)
	} else {
		fmt.Fprintf(w, "%sCLI:%s      lark-cli %s %s %s\n", output.Bold, output.Reset, specName, resName, methodName)
	}

	// Docs
	if docUrl := registry.GetStrFromMap(method, "docUrl"); docUrl != "" {
		fmt.Fprintf(w, "%sDocs:%s     %s\n", output.Bold, output.Reset, docUrl)
	}
}

func printNestedFields(w io.Writer, fields map[string]interface{}, indent, prefix string) {
	for _, fieldName := range sortedFieldKeys(fields) {
		f, _ := fields[fieldName].(map[string]interface{})
		fullName := fieldName
		if prefix != "" {
			fullName = prefix + "." + fieldName
		}
		fType := registry.GetStrFromMap(f, "type")
		required, _ := f["required"].(bool)
		reqStr := fmt.Sprintf("%soptional%s", output.Dim, output.Reset)
		if required {
			reqStr = fmt.Sprintf("%srequired%s", output.Red, output.Reset)
		}
		optStr := formatOptions(f)
		fmt.Fprintf(w, "%s- %s%s%s (%s, %s)%s\n", indent, output.Cyan, fullName, output.Reset, fType, reqStr, optStr)
		desc := registry.GetStrFromMap(f, "description")
		if desc != "" {
			desc = util.TruncateStrWithEllipsis(desc, 100)
			fmt.Fprintf(w, "%s  %s%s%s\n", indent, output.Dim, desc, output.Reset)
		}
		if ex := registry.GetStrFromMap(f, "example"); ex != "" {
			fmt.Fprintf(w, "%s  %se.g. %s%s\n", indent, output.Dim, ex, output.Reset)
		}
		if rangeStr := formatRange(f); rangeStr != "" {
			fmt.Fprintf(w, "%s  %srange: %s%s\n", indent, output.Dim, rangeStr, output.Reset)
		}
		if props, ok := f["properties"].(map[string]interface{}); ok && len(props) > 0 {
			printNestedFields(w, props, indent+"  ", fullName)
		}
	}
}

// formatOptions returns " — val1 | val2 | ..." if field has options, else "".
func formatOptions(f map[string]interface{}) string {
	opts, ok := f["options"].([]interface{})
	if !ok || len(opts) == 0 {
		return ""
	}
	var vals []string
	for _, o := range opts {
		if om, ok := o.(map[string]interface{}); ok {
			if v := registry.GetStrFromMap(om, "value"); v != "" {
				vals = append(vals, v)
			}
		}
	}
	if len(vals) == 0 {
		return ""
	}
	return fmt.Sprintf(" %s— %s%s", output.Dim, strings.Join(vals, " | "), output.Reset)
}

// formatRange returns "min..max" if field has min/max, else "".
func formatRange(f map[string]interface{}) string {
	minVal := registry.GetStrFromMap(f, "min")
	maxVal := registry.GetStrFromMap(f, "max")
	if minVal == "" && maxVal == "" {
		return ""
	}
	if minVal != "" && maxVal != "" {
		return minVal + ".." + maxVal
	}
	if minVal != "" {
		return ">=" + minVal
	}
	return "<=" + maxVal
}

// sortedKeys returns map keys in alphabetical order.
func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// sortedParamKeys returns parameter keys sorted: required first, then alphabetical.
func sortedParamKeys(params map[string]interface{}) []string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		pi, _ := params[keys[i]].(map[string]interface{})
		pj, _ := params[keys[j]].(map[string]interface{})
		ri, _ := pi["required"].(bool)
		rj, _ := pj["required"].(bool)
		if ri != rj {
			return ri
		}
		return keys[i] < keys[j]
	})
	return keys
}

// sortedFieldKeys returns field keys sorted: required first, then alphabetical.
func sortedFieldKeys(fields map[string]interface{}) []string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		fi, _ := fields[keys[i]].(map[string]interface{})
		fj, _ := fields[keys[j]].(map[string]interface{})
		ri, _ := fi["required"].(bool)
		rj, _ := fj["required"].(bool)
		if ri != rj {
			return ri
		}
		return keys[i] < keys[j]
	})
	return keys
}

func findResourceByPath(resources map[string]interface{}, parts []string) (map[string]interface{}, string, []string) {
	for i := len(parts); i >= 1; i-- {
		candidateName := strings.Join(parts[:i], ".")
		if res, ok := resources[candidateName]; ok {
			if resMap, ok := res.(map[string]interface{}); ok {
				return resMap, candidateName, parts[i:]
			}
		}
	}
	return nil, "", nil
}

// NewCmdSchema creates the schema command. If runF is non-nil it is called instead of schemaRun (test hook).
func NewCmdSchema(f *cmdutil.Factory, runF func(*SchemaOptions) error) *cobra.Command {
	opts := &SchemaOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "schema [path | service resource method]",
		Short: "View API method parameters, types, and scopes",
		Args:  cobra.MaximumNArgs(8),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Path = args[0]
			}
			if len(args) > 1 {
				opts.ExtraArgs = args[1:]
			}
			opts.Ctx = cmd.Context()
			if runF != nil {
				return runF(opts)
			}
			return schemaRun(opts)
		},
	}
	cmdutil.DisableAuthCheck(cmd)

	cmd.ValidArgsFunction = completeSchemaPath(f)
	cmd.Flags().StringVar(&opts.Format, "format", "json", "output format: json (default) | pretty")
	cmdutil.RegisterFlagCompletion(cmd, "format", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"json", "pretty"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmdutil.SetRisk(cmd, cmdutil.RiskRead)

	return cmd
}

// completeSchemaPath provides tab-completion for the schema path argument.
// It handles both legacy dotted resource names (e.g. app.table.fields) and the
// newer space-separated form (e.g. `schema im messages reply`).
func completeSchemaPath(f *cmdutil.Factory) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		mode := f.ResolveStrictMode(cmd.Context())

		// Case 1: legacy "single dotted arg" path — no previous args yet
		if len(args) == 0 {
			parts := strings.Split(toComplete, ".")
			if len(parts) <= 1 {
				var completions []string
				for _, s := range registry.ListFromMetaProjects() {
					if strings.HasPrefix(s, toComplete) {
						completions = append(completions, s+".")
					}
				}
				return completions, cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveNoSpace
			}
			serviceName := parts[0]
			spec := registry.LoadFromMeta(serviceName)
			if spec == nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			spec = filterSpecByStrictMode(spec, mode)
			resources, _ := spec["resources"].(map[string]interface{})
			if resources == nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			afterService := strings.Join(parts[1:], ".")
			completions := completeSchemaPathForSpec(serviceName, resources, afterService)
			allTrailingDot := len(completions) > 0
			for _, c := range completions {
				if !strings.HasSuffix(c, ".") {
					allTrailingDot = false
					break
				}
			}
			directive := cobra.ShellCompDirectiveNoFileComp
			if allTrailingDot {
				directive |= cobra.ShellCompDirectiveNoSpace
			}
			return completions, directive
		}

		// Case 2: space-form, args already has segments
		// Walk down service -> resource(s) -> method based on existing args
		serviceName := args[0]
		spec := registry.LoadFromMeta(serviceName)
		if spec == nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		spec = filterSpecByStrictMode(spec, mode)
		resources, _ := spec["resources"].(map[string]interface{})
		if resources == nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		// args[1:] are resource path segments (possibly partial); current
		// toComplete is the next segment under cursor.
		consumed := args[1:]
		resource, _, remaining := findResourceByPath(resources, consumed)
		if resource == nil {
			// Suggest top-level resource names that match toComplete
			var completions []string
			for resName := range resources {
				if strings.HasPrefix(resName, toComplete) {
					completions = append(completions, resName)
				}
			}
			sort.Strings(completions)
			return completions, cobra.ShellCompDirectiveNoFileComp
		}
		if len(remaining) > 0 {
			// Already typed past the resource — suggest methods
			methods, _ := resource["methods"].(map[string]interface{})
			methods = filterMethodsByStrictMode(methods, mode)
			var completions []string
			for mName := range methods {
				if strings.HasPrefix(mName, toComplete) {
					completions = append(completions, mName)
				}
			}
			sort.Strings(completions)
			return completions, cobra.ShellCompDirectiveNoFileComp
		}
		// Resource matched exactly, suggest methods
		methods, _ := resource["methods"].(map[string]interface{})
		methods = filterMethodsByStrictMode(methods, mode)
		var completions []string
		for mName := range methods {
			if strings.HasPrefix(mName, toComplete) {
				completions = append(completions, mName)
			}
		}
		sort.Strings(completions)
		return completions, cobra.ShellCompDirectiveNoFileComp
	}
}

func completeSchemaPathForSpec(serviceName string, resources map[string]interface{}, afterService string) []string {
	var completions []string

	for resName, resVal := range resources {
		if strings.HasPrefix(resName, afterService) {
			completions = append(completions, serviceName+"."+resName+".")
			continue
		}
		if !strings.HasPrefix(afterService, resName+".") {
			continue
		}
		methodPrefix := afterService[len(resName)+1:]
		resMap, _ := resVal.(map[string]interface{})
		if resMap == nil {
			continue
		}
		methods, _ := resMap["methods"].(map[string]interface{})
		for methodName := range methods {
			if strings.HasPrefix(methodName, methodPrefix) {
				completions = append(completions, serviceName+"."+resName+"."+methodName)
			}
		}
	}

	sort.Strings(completions)
	return completions
}

func schemaRun(opts *SchemaOptions) error {
	out := opts.Factory.IOStreams.Out
	mode := opts.Factory.ResolveStrictMode(opts.Ctx)

	// args may have arrived as a single string (legacy single-arg path) or
	// split into multiple — normalize to a single args slice.
	var rawArgs []string
	if opts.Path != "" {
		rawArgs = []string{opts.Path}
	}
	if len(opts.ExtraArgs) > 0 {
		if opts.Path != "" {
			rawArgs = append([]string{opts.Path}, opts.ExtraArgs...)
		} else {
			rawArgs = append([]string(nil), opts.ExtraArgs...)
		}
	}
	parts := schema.ParsePath(rawArgs)

	if opts.Format == "pretty" {
		return runPrettyMode(out, parts, mode)
	}
	return runJSONMode(out, parts, mode)
}

// runJSONMode dispatches list/single envelope output based on parts.
// JSON mode uses embedded data only (bypasses remote overlay) so envelope
// output is deterministic across machines.
func runJSONMode(out io.Writer, parts []string, mode core.StrictMode) error {
	filter := strictModeFilter(mode)

	switch len(parts) {
	case 0:
		envs := schema.AssembleAll(filter)
		output.PrintJson(out, envs)
		return nil
	case 1:
		spec := registry.EmbeddedSpec(parts[0])
		if spec == nil {
			return errUnknownEmbeddedService(parts[0])
		}
		envs := schema.AssembleService(parts[0], spec, filter)
		output.PrintJson(out, envs)
		return nil
	default:
		return runJSONForPath(out, parts, filter)
	}
}

// runJSONForPath handles len(parts) >= 2: try resource match first, fallback
// to single-method match. Uses embedded data only.
func runJSONForPath(out io.Writer, parts []string, filter schema.MethodFilter) error {
	serviceName := parts[0]
	spec := registry.EmbeddedSpec(serviceName)
	if spec == nil {
		return errUnknownEmbeddedService(serviceName)
	}
	resources, _ := spec["resources"].(map[string]interface{})
	resource, resName, remaining := findResourceByPath(resources, parts[1:])
	if resource == nil {
		var names []string
		for k := range resources {
			names = append(names, k)
		}
		sort.Strings(names)
		return output.ErrWithHint(output.ExitValidation, "validation",
			fmt.Sprintf("Unknown resource: %s.%s", serviceName, strings.Join(parts[1:], ".")),
			fmt.Sprintf("Available: %s", strings.Join(names, ", ")))
	}
	if len(remaining) == 0 {
		// Resource-scoped envelope array
		envs := assembleResource(serviceName, resName, resource, filter)
		output.PrintJson(out, envs)
		return nil
	}
	methodName := remaining[0]
	methods, _ := resource["methods"].(map[string]interface{})
	method, ok := methods[methodName].(map[string]interface{})
	if !ok {
		var names []string
		for k := range methods {
			names = append(names, k)
		}
		sort.Strings(names)
		return output.ErrWithHint(output.ExitValidation, "validation",
			fmt.Sprintf("Unknown method: %s.%s.%s", serviceName, resName, methodName),
			fmt.Sprintf("Available: %s", strings.Join(names, ", ")))
	}
	if len(remaining) > 1 {
		// Method exists but caller appended extra segments — reject so they
		// don't silently get this method's schema when they typo'd the path.
		return output.ErrWithHint(output.ExitValidation, "validation",
			fmt.Sprintf("Unknown path: %s.%s.%s",
				serviceName, resName, strings.Join(remaining, ".")),
			fmt.Sprintf("Method %q exists but the trailing segments %q do not resolve",
				methodName, strings.Join(remaining[1:], ".")))
	}
	if filter != nil && !filter(method) {
		// Method exists in spec but filtered out by strict mode
		return output.ErrWithHint(output.ExitValidation, "validation",
			fmt.Sprintf("Method %s.%s.%s not available in current identity mode", serviceName, resName, methodName),
			"Use --as user / --as bot to switch")
	}
	env := schema.AssembleEnvelope(serviceName, []string{resName}, methodName, method)
	output.PrintJson(out, env)
	return nil
}

func assembleResource(serviceName, resName string, resource map[string]interface{}, filter schema.MethodFilter) []schema.Envelope {
	methods, _ := resource["methods"].(map[string]interface{})
	resourcePath := []string{resName}
	var envs []schema.Envelope
	for methodName, raw := range methods {
		method, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if filter != nil && !filter(method) {
			continue
		}
		envs = append(envs, schema.AssembleEnvelope(serviceName, resourcePath, methodName, method))
	}
	sort.Slice(envs, func(i, j int) bool { return envs[i].Name < envs[j].Name })
	return envs
}

// runPrettyMode preserves the existing legacy pretty rendering verbatim.
// All printServices/printResourceList/printMethodDetail calls stay unchanged.
func runPrettyMode(out io.Writer, parts []string, mode core.StrictMode) error {
	if len(parts) == 0 {
		printServices(out)
		return nil
	}
	serviceName := parts[0]
	spec := registry.LoadFromMeta(serviceName)
	if spec == nil {
		return errUnknownService(serviceName)
	}
	if len(parts) == 1 {
		printResourceList(out, spec, mode)
		return nil
	}
	resources, _ := spec["resources"].(map[string]interface{})
	resource, resName, remaining := findResourceByPath(resources, parts[1:])
	if resource == nil {
		var names []string
		for k := range resources {
			names = append(names, k)
		}
		sort.Strings(names)
		return output.ErrWithHint(output.ExitValidation, "validation",
			fmt.Sprintf("Unknown resource: %s.%s", serviceName, strings.Join(parts[1:], ".")),
			fmt.Sprintf("Available: %s", strings.Join(names, ", ")))
	}
	if len(remaining) == 0 {
		fmt.Fprintf(out, "%s%s.%s%s\n\n", output.Bold, serviceName, resName, output.Reset)
		methods, _ := resource["methods"].(map[string]interface{})
		methods = filterMethodsByStrictMode(methods, mode)
		for _, mName := range sortedKeys(methods) {
			m, _ := methods[mName].(map[string]interface{})
			httpMethod := registry.GetStrFromMap(m, "httpMethod")
			desc := registry.GetStrFromMap(m, "description")
			fmt.Fprintf(out, "  %-7s %s%s%s  %s%s%s\n", httpMethod, output.Bold, mName, output.Reset, output.Dim, desc, output.Reset)
		}
		fmt.Fprintf(out, "\n%sUsage: lark-cli schema %s.%s.<method>%s\n", output.Dim, serviceName, resName, output.Reset)
		return nil
	}
	methodName := remaining[0]
	methods, _ := resource["methods"].(map[string]interface{})
	methods = filterMethodsByStrictMode(methods, mode)
	method, ok := methods[methodName].(map[string]interface{})
	if !ok {
		var names []string
		for k := range methods {
			names = append(names, k)
		}
		sort.Strings(names)
		return output.ErrWithHint(output.ExitValidation, "validation",
			fmt.Sprintf("Unknown method: %s.%s.%s", serviceName, resName, methodName),
			fmt.Sprintf("Available: %s", strings.Join(names, ", ")))
	}
	if len(remaining) > 1 {
		return output.ErrWithHint(output.ExitValidation, "validation",
			fmt.Sprintf("Unknown path: %s.%s.%s",
				serviceName, resName, strings.Join(remaining, ".")),
			fmt.Sprintf("Method %q exists but the trailing segments %q do not resolve",
				methodName, strings.Join(remaining[1:], ".")))
	}
	printMethodDetail(out, spec, resName, methodName, method)
	return nil
}

// strictModeFilter adapts core.StrictMode into a schema.MethodFilter, or returns
// nil if strict mode is not active.
func strictModeFilter(mode core.StrictMode) schema.MethodFilter {
	if !mode.IsActive() {
		return nil
	}
	token := registry.IdentityToAccessToken(string(mode.ForcedIdentity()))
	return func(method map[string]interface{}) bool {
		tokens, _ := method["accessTokens"].([]interface{})
		if tokens == nil {
			return true // permissive when meta_data lacks accessTokens
		}
		for _, t := range tokens {
			if s, _ := t.(string); s == token {
				return true
			}
		}
		return false
	}
}

func errUnknownService(name string) error {
	return output.ErrWithHint(output.ExitValidation, "validation",
		fmt.Sprintf("Unknown service: %s", name),
		fmt.Sprintf("Available: %s", strings.Join(registry.ListFromMetaProjects(), ", ")))
}

// errUnknownEmbeddedService is the JSON-mode variant: it lists only embedded
// services (no overlay) because JSON mode itself bypasses overlay; suggesting
// overlay-only services would mislead callers when those services subsequently
// fail to resolve in envelope output.
func errUnknownEmbeddedService(name string) error {
	return output.ErrWithHint(output.ExitValidation, "validation",
		fmt.Sprintf("Unknown service: %s", name),
		fmt.Sprintf("Available: %s", strings.Join(registry.EmbeddedServiceNames(), ", ")))
}

// filterSpecByStrictMode returns a shallow copy of spec with each resource's methods
// filtered by strict mode. Returns the original spec when strict mode is off.
func filterSpecByStrictMode(spec map[string]interface{}, mode core.StrictMode) map[string]interface{} {
	if !mode.IsActive() {
		return spec
	}
	result := make(map[string]interface{}, len(spec))
	for k, v := range spec {
		result[k] = v
	}
	resources, _ := spec["resources"].(map[string]interface{})
	if resources == nil {
		return result
	}
	filteredRes := make(map[string]interface{}, len(resources))
	for resName, resVal := range resources {
		resMap, ok := resVal.(map[string]interface{})
		if !ok {
			continue
		}
		methods, _ := resMap["methods"].(map[string]interface{})
		filtered := filterMethodsByStrictMode(methods, mode)
		if len(filtered) == 0 {
			continue
		}
		resCopy := make(map[string]interface{}, len(resMap))
		for k, v := range resMap {
			resCopy[k] = v
		}
		resCopy["methods"] = filtered
		filteredRes[resName] = resCopy
	}
	result["resources"] = filteredRes
	return result
}

// filterMethodsByStrictMode removes methods incompatible with the active strict mode.
// Returns the original map unmodified when strict mode is off.
func filterMethodsByStrictMode(methods map[string]interface{}, mode core.StrictMode) map[string]interface{} {
	if !mode.IsActive() || methods == nil {
		return methods
	}
	token := registry.IdentityToAccessToken(string(mode.ForcedIdentity()))
	filtered := make(map[string]interface{}, len(methods))
	for name, val := range methods {
		m, ok := val.(map[string]interface{})
		if !ok {
			continue
		}
		tokens, _ := m["accessTokens"].([]interface{})
		if tokens == nil {
			filtered[name] = val
			continue
		}
		for _, t := range tokens {
			if ts, ok := t.(string); ok && ts == token {
				filtered[name] = val
				break
			}
		}
	}
	return filtered
}
