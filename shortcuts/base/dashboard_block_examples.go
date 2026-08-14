// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"fmt"
	"sort"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Templates mirror the minimal shapes documented in the lark-base dashboard
// data_config reference. The validator drift test keeps this local fast path
// aligned with the existing create contract.
var dashboardBlockExampleTemplates = map[string]string{
	"area":       dashboardBlockSeriesExample,
	"bar":        dashboardBlockSeriesExample,
	"column":     dashboardBlockSeriesExample,
	"funnel":     dashboardBlockCountByGroupExample,
	"line":       dashboardBlockSeriesExample,
	"pie":        dashboardBlockCountByGroupExample,
	"ring":       dashboardBlockCountByGroupExample,
	"scatter":    dashboardBlockSeriesExample,
	"wordCloud":  dashboardBlockCountByGroupExample,
	"statistics": dashboardBlockStatisticsExample,
	"text": `{
  "text": "# 仪表盘说明\n在这里填写说明文字"
}`,
	"combo": `{
  "table_name": "表名",
  "series": [
    {"field_name": "指标1", "rollup": "SUM"},
    {"field_name": "指标2", "rollup": "SUM"}
  ],
  "group_by": [{"field_name": "分组字段", "mode": "integrated"}]
}`,
	"radar": `{
  "table_name": "表名",
  "series": [
    {"field_name": "维度1", "rollup": "SUM"},
    {"field_name": "维度2", "rollup": "SUM"},
    {"field_name": "维度3", "rollup": "SUM"}
  ],
  "group_by": [{"field_name": "分类字段", "mode": "integrated"}]
}`,
}

const dashboardBlockSeriesExample = `{
  "table_name": "表名",
  "series": [{"field_name": "数值字段", "rollup": "SUM"}],
  "group_by": [{"field_name": "分组字段", "mode": "integrated"}]
}`

const dashboardBlockCountByGroupExample = `{
  "table_name": "表名",
  "count_all": true,
  "group_by": [{"field_name": "分类字段", "mode": "integrated"}]
}`

const dashboardBlockStatisticsExample = `{
  "table_name": "表名",
  "series": [{"field_name": "数值字段", "rollup": "SUM"}]
}`

func dashboardBlockExampleTypes() []string {
	types := make([]string, 0, len(dashboardBlockExampleTemplates))
	for typ := range dashboardBlockExampleTemplates {
		types = append(types, typ)
	}
	sort.Strings(types)
	return types
}

func withDashboardBlockPrintExample(prev func(cmd *cobra.Command)) func(cmd *cobra.Command) {
	return func(cmd *cobra.Command) {
		if prev != nil {
			prev(cmd)
		}

		prevPreRunE := cmd.PreRunE
		cmd.PreRunE = func(c *cobra.Command, args []string) error {
			if typ, _ := c.Flags().GetString("print-example"); typ != "" {
				c.Flags().VisitAll(func(flag *pflag.Flag) {
					delete(flag.Annotations, cobra.BashCompOneRequiredFlag)
				})
			}
			if prevPreRunE != nil {
				return prevPreRunE(c, args)
			}
			return nil
		}

		prevRunE := cmd.RunE
		cmd.RunE = func(c *cobra.Command, args []string) error {
			typ, _ := c.Flags().GetString("print-example")
			if typ == "" {
				return prevRunE(c, args)
			}
			template, ok := dashboardBlockExampleTemplates[typ]
			if !ok {
				return common.ValidationErrorf("no example for dashboard block type %q; available: %s", typ, strings.Join(dashboardBlockExampleTypes(), ", ")).WithParam("--print-example")
			}
			fmt.Fprintln(c.OutOrStdout(), template)
			return nil
		}
	}
}
