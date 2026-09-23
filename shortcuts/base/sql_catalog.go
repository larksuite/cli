// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

type sqlCatalogType struct {
	Kind    string          `json:"kind"`
	Element *sqlCatalogType `json:"element"`
}

func (t sqlCatalogType) label() string {
	if t.Element != nil && t.Kind == "COLLECTION" {
		return t.Kind + "<" + t.Element.label() + ">"
	}
	return t.Kind
}

type sqlCatalog struct {
	Tables []struct {
		BaseToken string `json:"baseToken"`
		ID        string `json:"id"`
		Name      string `json:"name"`
		Fields    map[string]struct {
			SQLType   sqlCatalogType `json:"sqlType"`
			Nullable  bool           `json:"nullable"`
			Reference *struct {
				BaseToken string `json:"baseToken"`
				TableID   string `json:"tableId"`
			} `json:"reference"`
		} `json:"fields"`
	} `json:"tables"`
}

type sqlCatalogRow struct {
	Table     string `json:"table"`
	Field     string `json:"field"`
	Type      string `json:"type"`
	Nullable  bool   `json:"nullable"`
	Reference string `json:"reference"`
}

var BaseSQLCatalog = common.Shortcut{
	Service: "base", Command: "+sql-catalog", Description: `Discover visible Base SQL tables, field types, and relation targets.

Output formats (--format): json (default) and pretty return the complete schema, including options and type metadata; table displays table, field, type, nullability, and relation target; csv and ndjson export flattened field entries. NDJSON emits one entry per line and is not the complete schema.`, Risk: "read", Scopes: []string{"base:record:read"}, AuthTypes: authTypes(),
	Flags: []common.Flag{baseTokenFlag(true)},
	DryRun: func(_ context.Context, r *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().GET("/open-apis/base/v3/bases/:base_token/basesql/catalog").Set("base_token", r.Str("base-token"))
	},
	Execute: func(_ context.Context, r *common.RuntimeContext) error {
		data, err := baseV3Call(r, "GET", baseV3Path("bases", r.Str("base-token"), "basesql", "catalog"), nil, nil)
		if err != nil {
			return err
		}
		if r.JqExpr != "" || (r.Format != "table" && r.Format != "csv" && r.Format != "ndjson") {
			r.OutFormat(data, nil, nil)
			return nil
		}
		rows, err := sqlCatalogRows(data)
		if err != nil {
			return err
		}
		r.OutFormat(rows, nil, nil)
		return nil
	},
}

func sqlCatalogRows(data any) ([]sqlCatalogRow, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "SQL catalog could not be encoded").WithCause(err)
	}
	var catalog sqlCatalog
	if err := json.Unmarshal(raw, &catalog); err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "SQL catalog must contain tables").WithCause(err)
	}
	if catalog.Tables == nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "SQL catalog must contain tables")
	}
	targets := make(map[[2]string]string)
	for _, table := range catalog.Tables {
		targets[[2]string{table.BaseToken, table.ID}] = table.Name
	}
	sort.SliceStable(catalog.Tables, func(i, j int) bool { return catalog.Tables[i].Name < catalog.Tables[j].Name })
	rows := make([]sqlCatalogRow, 0)
	for _, table := range catalog.Tables {
		fields := make([]string, 0, len(table.Fields))
		for name := range table.Fields {
			fields = append(fields, name)
		}
		sort.Strings(fields)
		for _, name := range fields {
			field := table.Fields[name]
			row := sqlCatalogRow{Table: table.Name, Field: name, Type: field.SQLType.label(), Nullable: field.Nullable}
			if ref := field.Reference; ref != nil {
				row.Reference = targets[[2]string{ref.BaseToken, ref.TableID}]
				if row.Reference == "" {
					row.Reference = ref.TableID
				}
			}
			rows = append(rows, row)
		}
	}
	return rows, nil
}
