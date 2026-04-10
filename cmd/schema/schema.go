// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/internal/util"
	"github.com/spf13/cobra"
)

// SchemaOptions holds all inputs for the schema command.
type SchemaOptions struct {
	Factory *cmdutil.Factory

	// Positional args
	Path string

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

func printResourceList(w io.Writer, spec map[string]interface{}) {
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
		fmt.Fprintf(w, "  %s%s%s\n", output.Cyan, resName, output.Reset)
		resMap, _ := resources[resName].(map[string]interface{})
		methods, _ := resMap["methods"].(map[string]interface{})
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
		Use:   "schema [path]",
		Short: "View API method parameters, types, and scopes",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Path = args[0]
			}
			if runF != nil {
				return runF(opts)
			}
			return schemaRun(opts)
		},
	}
	cmdutil.DisableAuthCheck(cmd)

	cmd.ValidArgsFunction = completeSchemaPath
	cmd.Flags().StringVar(&opts.Format, "format", "json", "output format: json (default) | pretty")
	_ = cmd.RegisterFlagCompletionFunc("format", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"json", "pretty"}, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

// completeSchemaPath provides tab-completion for the schema path argument.
// It handles dotted resource names (e.g. app.table.fields) by iterating all
// resources and classifying each as a prefix-match or fully-matched.
func completeSchemaPath(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	parts := strings.Split(toComplete, ".")

	// Level 1: complete service names
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
	resources, _ := spec["resources"].(map[string]interface{})
	if resources == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// afterService = everything user typed after "serviceName."
	afterService := strings.Join(parts[1:], ".")

	var completions []string

	for resName, resVal := range resources {
		if strings.HasPrefix(resName, afterService) {
			// afterService is a prefix of this resource name → resource candidate
			completions = append(completions, serviceName+"."+resName+".")
		} else if strings.HasPrefix(afterService, resName+".") {
			// This resource is fully matched; remainder is method prefix
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
	}

	sort.Strings(completions)

	// If all completions end with ".", user is still navigating resources → NoSpace
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

func schemaRun(opts *SchemaOptions) error {
	out := opts.Factory.IOStreams.Out

	if opts.Path == "" {
		printServices(out)
		return nil
	}

	parts := strings.Split(opts.Path, ".")

	serviceName := parts[0]
	spec := registry.LoadFromMeta(serviceName)
	if spec == nil {
		return output.ErrWithHint(output.ExitValidation, "validation",
			fmt.Sprintf("Unknown service: %s", serviceName),
			fmt.Sprintf("Available: %s", strings.Join(registry.ListFromMetaProjects(), ", ")))
	}

	if len(parts) == 1 {
		if opts.Format == "pretty" {
			printResourceList(out, spec)
		} else {
			output.PrintJson(out, spec)
		}
		return nil
	}

	resources, _ := spec["resources"].(map[string]interface{})
	resource, resName, remaining := findResourceByPath(resources, parts[1:])
	if resource == nil {
		var resNames []string
		for k := range resources {
			resNames = append(resNames, k)
		}
		return output.ErrWithHint(output.ExitValidation, "validation",
			fmt.Sprintf("Unknown resource: %s.%s", serviceName, strings.Join(parts[1:], ".")),
			fmt.Sprintf("Available: %s", strings.Join(resNames, ", ")))
	}

	if len(remaining) == 0 {
		if opts.Format == "pretty" {
			fmt.Fprintf(out, "%s%s.%s%s\n\n", output.Bold, serviceName, resName, output.Reset)
			methods, _ := resource["methods"].(map[string]interface{})
			for _, mName := range sortedKeys(methods) {
				m, _ := methods[mName].(map[string]interface{})
				httpMethod := registry.GetStrFromMap(m, "httpMethod")
				desc := registry.GetStrFromMap(m, "description")
				fmt.Fprintf(out, "  %-7s %s%s%s  %s%s%s\n", httpMethod, output.Bold, mName, output.Reset, output.Dim, desc, output.Reset)
			}
			fmt.Fprintf(out, "\n%sUsage: lark-cli schema %s.%s.<method>%s\n", output.Dim, serviceName, resName, output.Reset)
		} else {
			output.PrintJson(out, resource)
		}
		return nil
	}

	methodName := remaining[0]
	methods, _ := resource["methods"].(map[string]interface{})
	method, ok := methods[methodName].(map[string]interface{})
	if !ok {
		var mNames []string
		for k := range methods {
			mNames = append(mNames, k)
		}
		return output.ErrWithHint(output.ExitValidation, "validation",
			fmt.Sprintf("Unknown method: %s.%s.%s", serviceName, resName, methodName),
			fmt.Sprintf("Available: %s", strings.Join(mNames, ", ")))
	}

	if opts.Format == "pretty" {
		printMethodDetail(out, spec, resName, methodName, method)
	} else {
		output.PrintJson(out, method)
	}
	return nil
}
