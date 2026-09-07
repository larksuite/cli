// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"fmt"
	"sort"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

// ─── +chart-create --print-example ─────────────────────────────────────
//
// chart-create's --properties schema is ~1,750 pretty-printed lines; eval
// traces show agents paging through the full --print-schema dump for every
// chart (25 round trips in one 35-task batch) and still missing deep
// required fields. A ready-to-edit minimal template per chart type answers
// the actual question ("what does a valid payload look like") in one local
// call. Wired through PostMount, same pattern as +csv-put's flag-group
// tweaks — no framework change.
//
// Templates mirror the canonical examples in the lark-sheets-chart
// reference (sheet-skill-spec canonical-spec/references/lark_sheet_chart):
// inline headerMode with refs covering the header row, 1-based indices,
// quoted sheet prefix in refs.

var chartExampleTemplates = map[string]string{
	"column": chartSimpleExample("column"),
	"bar":    chartSimpleExample("bar"),
	"line":   chartSimpleExample("line"),
	"area":   chartSimpleExample("area"),
	"radar":  chartSimpleExample("radar"),
	"bubble": `{
  "position": {"row": 1, "col": "G"},
  "size": {"width": 600, "height": 400},
  "snapshot": {
    "title": {"text": "气泡图标题"},
    "plotArea": {"plot": {
      "type": "bubble",
      "extra": {"bubble": {
        "aggregate": false,
        "showNegativeSize": false,
        "opacityGradientStyle": "linear",
        "idLabel": {"visible": true}
      }}
    }},
    "data": {
      "refs": [{"value": "'Sheet1'!A1:E20"}],
      "dim1": {"serie": {"index": 1}},
      "dim2": {"series": [
        {"index": 2, "role": "x"},
        {"index": 3, "role": "y"},
        {"index": 4, "role": "group"},
        {"index": 5, "role": "size"}
      ]}
    }
  }
}`,
	"waterfall": `{
  "position": {"row": 1, "col": "F"},
  "size": {"width": 600, "height": 400},
  "snapshot": {
    "title": {"text": "瀑布图标题"},
    "plotArea": {"plot": {
      "type": "waterfall",
      "extra": {"waterfall": {
        "firstValueAsTotal": true,
        "lastValueAsSubtotal": true,
        "connectorLine": {"style": "solid", "width": 1},
        "totalLabels": {"template": "{{value}}"}
      }}
    }},
    "data": {
      "refs": [{"value": "'Sheet1'!A1:B10"}],
      "dim1": {"serie": {"index": 1}},
      "dim2": {"series": [{"index": 2}]}
    }
  }
}`,
	"pareto": `{
  "position": {"row": 1, "col": "F"},
  "size": {"width": 600, "height": 400},
  "snapshot": {
    "title": {"text": "排列图标题"},
    "plotArea": {"plot": {
      "type": "pareto",
      "extra": {"pareto": {"aggregateType": "sum", "categoryNumber": 5}},
      "series": [
        {"index": 1, "bars": {"gap": 0.25}, "labels": {"value": true}},
        {"index": 2, "line": {"width": 2}, "points": {"shape": "circle", "size": 6}, "labels": {"percentage": true, "format": "0%"}}
      ]
    }},
    "data": {
      "refs": [{"value": "'Sheet1'!A1:B20"}],
      "dim1": {"serie": {"index": 1, "aggregate": true}},
      "dim2": {"series": [{"index": 2, "aggregateType": "sum"}]}
    }
  }
}`,
	"scatter": `{
  "position": {"row": 1, "col": "F"},
  "size": {"width": 600, "height": 400},
  "snapshot": {
    "title": {"text": "图表标题"},
    "plotArea": {"plot": {"type": "scatter"}},
    "data": {
      "refs": [{"value": "'Sheet1'!A1:B20"}],
      "dim1": {"serie": {"index": 1}},
      "dim2": {"series": [{"index": 2}]}
    }
  }
}`,
	"pie": `{
  "position": {"row": 1, "col": "F"},
  "size": {"width": 600, "height": 450},
  "snapshot": {
    "title": {"text": "占比标题"},
    "plotArea": {"plot": {
      "type": "pie",
      "series": [{
        "index": 1,
        "sectors": {"sector": [{"index": 1, "offsetRadius": 0.05}]}
      }]
    }},
    "data": {
      "refs": [{"value": "'Sheet1'!A1:B11"}],
      "dim1": {"serie": {"index": 1, "aggregate": true}},
      "dim2": {"series": [{"index": 2, "aggregateType": "sum"}]}
    }
  }
}`,
	"combo": `{
  "position": {"row": 1, "col": "F"},
  "size": {"width": 700, "height": 400},
  "snapshot": {
    "title": {"text": "柱线组合"},
    "plotArea": {"plot": {
      "type": "combo",
      "series": [
        {"index": 2, "comboType": "column"},
        {"index": 3, "comboType": "line"}
      ]
    }},
    "data": {
      "refs": [{"value": "'Sheet1'!A1:C13"}],
      "dim1": {"serie": {"index": 1}},
      "dim2": {"series": [{"index": 2}, {"index": 3}]}
    }
  }
}`,
}

// chartSimpleExample renders the shared minimal shape for plot types that
// need nothing beyond plot.type (column / bar / line / area / radar).
func chartSimpleExample(typ string) string {
	return fmt.Sprintf(`{
  "position": {"row": 1, "col": "F"},
  "size": {"width": 600, "height": 400},
  "snapshot": {
    "title": {"text": "图表标题"},
    "plotArea": {"plot": {"type": %q}},
    "data": {
      "refs": [{"value": "'Sheet1'!A1:C10"}],
      "dim1": {"serie": {"index": 1}},
      "dim2": {"series": [{"index": 2}, {"index": 3}]}
    }
  }
}`, typ)
}

func chartExampleTypes() []string {
	types := make([]string, 0, len(chartExampleTemplates))
	for t := range chartExampleTemplates {
		types = append(types, t)
	}
	sort.Strings(types)
	return types
}

// withChartPrintExample wraps +chart-create's PostMount so --print-example
// short-circuits execution and prints a minimal ready-to-edit --properties
// template — purely local, no identity or network. The flag itself is
// declared in flag-defs.json like every other own flag (so it shows up in the
// generated reference tables); only the interception lives here.
// --properties' cobra-level required annotation is relaxed (the input builder
// still enforces it on the real path, same trick as +csv-put's --csv).
func withChartPrintExample(prev func(cmd *cobra.Command)) func(cmd *cobra.Command) {
	return func(cmd *cobra.Command) {
		if prev != nil {
			prev(cmd)
		}
		// Only --properties carries a cobra-level required annotation (the
		// locator flags are xor pairs, enforced later); the input builder
		// still errors "--properties is required" on the real path.
		if fl := cmd.Flags().Lookup("properties"); fl != nil {
			delete(fl.Annotations, cobra.BashCompOneRequiredFlag)
		}
		prevRunE := cmd.RunE
		cmd.RunE = func(c *cobra.Command, args []string) error {
			typ, _ := c.Flags().GetString("print-example")
			if typ == "" {
				return prevRunE(c, args)
			}
			tmpl, ok := chartExampleTemplates[typ]
			if !ok {
				return common.ValidationErrorf("no example for chart type %q; available: %s",
					typ, strings.Join(chartExampleTypes(), ", ")).WithParam("--print-example")
			}
			fmt.Fprintln(c.OutOrStdout(), tmpl)
			return nil
		}
	}
}
