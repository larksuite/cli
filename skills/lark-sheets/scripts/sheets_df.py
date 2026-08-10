#!/usr/bin/env python3
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
"""DataFrame ↔ Feishu Sheet typed-JSON helpers.

This is the same 7-line snippet the skill docs already inline (see
`lark-sheets-write-cells` "DataFrame → 协议（5 行 helper）" and
`lark-sheets-read-data` "输出 → DataFrame（2 行 helper）"), pulled out
so callers can `import` it instead of copy-pasting:

    from sheets_df import df_to_sheet, sheet_to_df

Callers run lark-cli themselves; this file is a library, not a CLI.
"""
import json

import pandas as pd


def df_to_sheet(df, name, formats=None):
    """Pack one DataFrame into one entry of a `+table-put --sheets` payload."""
    packed = json.loads(df.to_json(orient="split", date_format="iso"))
    # The protocol requires string column names. pandas keeps integer labels
    # (e.g. the default RangeIndex columns 0/1/2) as JSON numbers, while the
    # dtypes dict keys get stringified during JSON serialization — the CLI
    # then rejects `columns` ("cannot unmarshal number into … type string")
    # and the dtype lookup would miss anyway. Stringify every key once, and
    # refuse to continue when that conversion silently merges two columns.
    normalized_labels = [str(c) for c in df.columns]
    columns = [str(c) for c in packed["columns"]]
    if normalized_labels != columns:
        columns = normalized_labels
    if len(set(columns)) != len(columns):
        raise ValueError(
            "column labels collide after str() conversion; "
            "rename the DataFrame columns before packing"
        )
    packed["columns"] = columns
    dtype_values = list(df.dtypes)
    return {
        "name": name,
        **packed,
        "dtypes": {key: str(dtype) for key, dtype in zip(columns, dtype_values)},
        **({"formats": {str(k): v for k, v in formats.items()}} if formats else {}),
    }


def sheet_to_df(sheet):
    """Restore one `+table-get` sheet dict into a typed DataFrame."""
    return pd.DataFrame(sheet["data"], columns=sheet["columns"]).astype(sheet["dtypes"])
