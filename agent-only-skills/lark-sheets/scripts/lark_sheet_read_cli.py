# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
"""Read-only Lark Sheet subset wrapper for the helper scripts."""

from __future__ import annotations

import json
import subprocess
import sys
from typing import Any


class LarkCliError(RuntimeError):
    def __init__(self, message: str, *, cmd: list[str] | None = None):
        super().__init__(message)
        self.cmd = cmd or []


def add_spreadsheet_args(
    parser,
    *,
    require_sheet: bool = False,
    allow_sheet: bool = True,
) -> None:
    spreadsheet = parser.add_mutually_exclusive_group(required=True)
    spreadsheet.add_argument("--url")
    spreadsheet.add_argument("--spreadsheet-token")
    if allow_sheet:
        sheet = parser.add_mutually_exclusive_group(required=require_sheet)
        sheet.add_argument("--sheet-id")
        sheet.add_argument("--sheet-name")


def _append_flag(cmd: list[str], name: str, value: Any) -> None:
    flag = f"--{name.replace('_', '-')}"
    if value is None:
        return
    if isinstance(value, bool):
        cmd.append(flag if value else f"{flag}=false")
        return
    cmd.extend([flag, str(value)])


def run_sheets(
    shortcut: str,
    *,
    url: str | None = None,
    spreadsheet_token: str | None = None,
    sheet_id: str | None = None,
    sheet_name: str | None = None,
    flags: dict[str, Any] | None = None,
    timeout: int = 60,
) -> dict[str, Any]:
    if bool(url) == bool(spreadsheet_token):
        raise LarkCliError("Pass exactly one of --url or --spreadsheet-token")
    if sheet_id and sheet_name:
        raise LarkCliError("Pass only one of --sheet-id or --sheet-name")

    cmd = ["lark-cli", "sheets", shortcut]
    _append_flag(cmd, "url", url)
    _append_flag(cmd, "spreadsheet_token", spreadsheet_token)
    _append_flag(cmd, "sheet_id", sheet_id)
    _append_flag(cmd, "sheet_name", sheet_name)
    for key, value in (flags or {}).items():
        _append_flag(cmd, key, value)

    try:
        completed = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            timeout=timeout,
            check=False,
        )
    except FileNotFoundError as exc:
        raise LarkCliError("lark-cli not found", cmd=cmd) from exc
    except subprocess.TimeoutExpired as exc:
        raise LarkCliError(f"lark-cli timed out after {timeout}s", cmd=cmd) from exc

    if completed.returncode != 0:
        detail = (completed.stderr or completed.stdout or "").strip()
        raise LarkCliError(detail or f"lark-cli exited with {completed.returncode}", cmd=cmd)

    try:
        envelope = json.loads(completed.stdout)
    except json.JSONDecodeError as exc:
        snippet = completed.stdout[:500].replace("\n", "\\n")
        raise LarkCliError(f"lark-cli stdout was not JSON: {snippet}", cmd=cmd) from exc

    if isinstance(envelope, dict) and envelope.get("ok") is False:
        raise LarkCliError(json.dumps(envelope, ensure_ascii=False), cmd=cmd)
    if not isinstance(envelope, dict):
        raise LarkCliError("lark-cli returned a non-object JSON payload", cmd=cmd)
    return envelope


def envelope_data(envelope: dict[str, Any]) -> dict[str, Any]:
    data = envelope.get("data")
    return data if isinstance(data, dict) else envelope


def emit_success(action: str, data: dict[str, Any], warnings: list[str] | None = None) -> None:
    print(
        json.dumps(
            {
                "ok": True,
                "engine": "lark",
                "action": action,
                "data": data,
                "warnings": warnings or [],
            },
            ensure_ascii=False,
            indent=2,
        )
    )


def emit_error(action: str, message: str, warnings: list[str] | None = None) -> None:
    print(
        json.dumps(
            {
                "ok": False,
                "engine": "lark",
                "action": action,
                "error": message,
                "warnings": warnings or [],
            },
            ensure_ascii=False,
            indent=2,
        )
    )
    sys.exit(1)


def sheet_title(sheet: dict[str, Any]) -> str:
    return str(sheet.get("title") or sheet.get("sheet_name") or sheet.get("name") or "")


def sheet_identifier(sheet: dict[str, Any]) -> str:
    return str(sheet.get("sheet_id") or sheet.get("id") or "")


def sheet_locator(sheet: dict[str, Any]) -> dict[str, str]:
    sid = sheet_identifier(sheet)
    if sid:
        return {"sheet_id": sid}
    title = sheet_title(sheet)
    if title:
        return {"sheet_name": title}
    return {}


def extract_sheets(workbook_data: dict[str, Any]) -> list[dict[str, Any]]:
    sheets = workbook_data.get("sheets")
    if isinstance(sheets, list):
        return [sheet for sheet in sheets if isinstance(sheet, dict)]
    workbook = workbook_data.get("workbook")
    if isinstance(workbook, dict) and isinstance(workbook.get("sheets"), list):
        return [sheet for sheet in workbook["sheets"] if isinstance(sheet, dict)]
    return []


def sheet_resource_type(sheet: dict[str, Any]) -> str:
    return str(sheet.get("resource_type") or "")


def is_grid_sheet(sheet: dict[str, Any]) -> bool:
    resource_type = sheet_resource_type(sheet)
    if resource_type == "sheet":
        return True
    # Legacy responses may omit resource_type but still include grid dimensions.
    return not resource_type and sheet.get("row_count") is not None and sheet.get("column_count") is not None


def visible_grid_selection(
    workbook_data: dict[str, Any],
    *,
    sheet_id: str | None = None,
    sheet_name: str | None = None,
) -> dict[str, Any]:
    """Resolve explicit targets or publish a safe default-selection decision.

    Unspecified targets are never selected by index: only visible ordinary grids
    participate, and multiple candidates remain ambiguous for the caller to match
    by task wording or ask the user.
    """
    sheets = extract_sheets(workbook_data)
    specified = {"sheet_id": sheet_id, "sheet_name": sheet_name}
    if sheet_id or sheet_name:
        matches = resolve_target_sheets(workbook_data, sheet_id=sheet_id, sheet_name=sheet_name, require_one=True)
        sheet = matches[0]
        if not is_grid_sheet(sheet):
            raise LarkCliError(f"Sheet {sheet_title(sheet) or sheet_identifier(sheet)} is not a grid sheet; use the matching product API")
        warnings = ["explicit_hidden_sheet"] if sheet.get("is_hidden") is True else []
        return {"policy": "visible_grid_v1", "specified": specified, "selected": _selection_sheet(sheet), "candidates": [_selection_sheet(sheet)], "excluded": [], "ambiguous": False, "warnings": warnings, "next_action": "use selected sheet_id for grid read/write"}

    candidates, excluded = [], []
    for sheet in sheets:
        if not is_grid_sheet(sheet):
            excluded.append({**_selection_sheet(sheet), "reason": "non_grid"})
        elif sheet.get("is_hidden") is True:
            excluded.append({**_selection_sheet(sheet), "reason": "hidden"})
        elif sheet.get("is_hidden") is not False:
            excluded.append({**_selection_sheet(sheet), "reason": "visibility_unknown"})
        else:
            candidates.append(_selection_sheet(sheet))
    candidates.sort(key=lambda item: item.get("index") if isinstance(item.get("index"), int) else 10**9)
    selected = candidates[0] if len(candidates) == 1 else None
    return {"policy": "visible_grid_v1", "specified": specified, "selected": selected, "candidates": candidates, "excluded": excluded, "ambiguous": len(candidates) > 1, "warnings": [], "next_action": "use selected sheet_id for grid read/write" if selected else "match task wording/title/header; do not choose by index"}


def _selection_sheet(sheet: dict[str, Any]) -> dict[str, Any]:
    return {"sheet_id": sheet_identifier(sheet), "title": sheet_title(sheet), "index": sheet.get("index"), "resource_type": sheet_resource_type(sheet) or "legacy_grid", "is_hidden": sheet.get("is_hidden"), "row_count": sheet.get("row_count"), "column_count": sheet.get("column_count")}


def resolve_target_sheets(
    workbook_data: dict[str, Any],
    *,
    sheet_id: str | None = None,
    sheet_name: str | None = None,
    require_one: bool = False,
) -> list[dict[str, Any]]:
    sheets = extract_sheets(workbook_data)
    if sheet_id:
        matches = [sheet for sheet in sheets if sheet_identifier(sheet) == sheet_id]
    elif sheet_name:
        matches = [sheet for sheet in sheets if sheet_title(sheet) == sheet_name]
    else:
        matches = sheets

    if require_one:
        if len(matches) == 1:
            return matches
        if not matches:
            raise LarkCliError("No matching sheet found")
        raise LarkCliError("Multiple sheets matched; pass --sheet-id or --sheet-name")
    return matches
