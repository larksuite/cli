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
    return {
        "name": name,
        **json.loads(df.to_json(orient="split", date_format="iso")),
        "dtypes": df.dtypes.astype(str).to_dict(),
        **({"formats": formats} if formats else {}),
    }


def sheet_to_df(sheet):
    """Restore one `+table-get` sheet dict into a typed DataFrame."""
    return pd.DataFrame(sheet["data"], columns=sheet["columns"]).astype(sheet["dtypes"])
