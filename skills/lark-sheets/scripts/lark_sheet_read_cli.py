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
