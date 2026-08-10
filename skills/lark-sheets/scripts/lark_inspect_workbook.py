#!/usr/bin/env python3
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
"""Inspect a Lark spreadsheet and emit a compact workbook profile."""

from __future__ import annotations

import argparse
from typing import Any

from lark_sheet_range import index_to_col
from lark_sheet_read_cli import (
    LarkCliError,
    add_spreadsheet_args,
    emit_error,
    emit_success,
    envelope_data,
    resolve_target_sheets,
    run_sheets,
    sheet_identifier,
    sheet_locator,
    sheet_title,
)

ACTION = "inspect_workbook"
LAYOUT_INCLUDE = "merges,row_heights,col_widths,hidden_rows,hidden_cols,groups,frozen"


def _sheet_summary(sheet: dict[str, Any]) -> dict[str, Any]:
    return {
        "sheet_id": sheet_identifier(sheet),
        "title": sheet_title(sheet),
        "index": sheet.get("index"),
        "row_count": sheet.get("row_count"),
        "column_count": sheet.get("column_count"),
        "is_hidden": sheet.get("is_hidden"),
        "merged_cells_count": sheet.get("merged_cells_count"),
        "chart_count": sheet.get("chart_count"),
        "pivot_table_count": sheet.get("pivot_table_count"),
        "float_image_count": sheet.get("float_image_count"),
    }


def _list_count(value: Any) -> int:
    return len(value) if isinstance(value, list) else 0


def _layout_summary(layout: dict[str, Any]) -> dict[str, Any]:
    """Retain layout signals without serializing unbounded per-cell metadata."""
    merges = layout.get("merged_cells") or layout.get("merges") or []
    groups = layout.get("groups") if isinstance(layout.get("groups"), dict) else {}
    row_groups = layout.get("row_groups") or groups.get("rows", [])
    col_groups = layout.get("column_groups") or groups.get("columns", [])
    return {
        "merge_count": _list_count(merges),
        "row_heights_count": _list_count(layout.get("row_heights")),
        "column_widths_count": _list_count(layout.get("col_widths")),
        "hidden_rows_count": _list_count(layout.get("hidden_rows")),
        "hidden_columns_count": _list_count(
            layout.get("hidden_cols") or layout.get("hidden_columns")
        ),
        "row_groups_count": _list_count(row_groups),
        "column_groups_count": _list_count(col_groups),
        "frozen": layout.get("frozen"),
    }


def inspect_workbook(args) -> tuple[dict[str, Any], list[str]]:
    warnings: list[str] = []
    workbook = envelope_data(
        run_sheets(
            "+workbook-info",
            url=args.url,
            spreadsheet_token=args.spreadsheet_token,
            timeout=args.timeout,
        )
    )
    # An explicit selector must resolve: without require_one a typo'd
    # --sheet-id/--sheet-name silently yields sheet_count 0, which reads as
    # "empty workbook" instead of a locator error.
    target_sheets = resolve_target_sheets(
        workbook,
        sheet_id=args.sheet_id,
        sheet_name=args.sheet_name,
        require_one=bool(args.sheet_id or args.sheet_name),
    )
    if args.max_sheets < 1:
        raise LarkCliError("--max-sheets must be at least 1")
    inspect_count = len(target_sheets)
    if not args.sheet_id and not args.sheet_name:
        inspect_count = min(len(target_sheets), args.max_sheets)
        if inspect_count < len(target_sheets):
            warnings.append(
                f"layout and preview skipped for {len(target_sheets) - inspect_count} sheets; "
                f"pass --sheet-id or --sheet-name to inspect one"
            )

    profiles = []
    for position, sheet in enumerate(target_sheets):
        sid = sheet_identifier(sheet)
        title = sheet_title(sheet)
        profile = _sheet_summary(sheet)
        if position >= inspect_count:
            profiles.append(profile)
            continue
        locator = sheet_locator(sheet)
        col_count = int(sheet.get("column_count") or args.max_preview_cols)
        preview_cols = min(col_count, args.max_preview_cols)
        if col_count > args.max_preview_cols:
            warnings.append(
                f"{title or sid}: preview clipped to first {args.max_preview_cols} columns"
            )
        end_col = index_to_col(max(1, preview_cols))
        preview_range = f"A1:{end_col}{args.preview_rows}"

        # Per-sheet, not fail-the-run: this is the first-step pre-flight, and
        # one unreadable sheet (odd type, transient error, a locator that does
        # not resolve) must not throw away the summaries already collected for
        # every other sheet. The basic summary comes from +workbook-info and is
        # already in hand, so a failure here degrades detail, not correctness —
        # same call the sibling profile_table downgrades to a warning.
        try:
            layout = envelope_data(
                run_sheets(
                    "+sheet-info",
                    url=args.url,
                    spreadsheet_token=args.spreadsheet_token,
                    **locator,
                    flags={"include": LAYOUT_INCLUDE},
                    timeout=args.timeout,
                )
            )
            preview = envelope_data(
                run_sheets(
                    "+csv-get",
                    url=args.url,
                    spreadsheet_token=args.spreadsheet_token,
                    **locator,
                    flags={"range": preview_range, "max_chars": args.max_chars},
                    timeout=args.timeout,
                )
            )
        except LarkCliError as exc:
            warnings.append(
                f"{title or sid}: layout/preview unavailable ({exc}); "
                "re-read this sheet on its own with --sheet-name, or use +sheet-info / +csv-get directly"
            )
            profiles.append(profile)
            continue
        if preview.get("has_more"):
            warnings.append(f"{title or sid}: preview range {preview_range} was truncated")

        profiles.append(
            {
                **profile,
                "layout": _layout_summary(layout),
                "preview": {
                    "range": preview_range,
                    "current_region": preview.get("current_region"),
                    "row_indices": preview.get("row_indices"),
                    "col_indices": preview.get("col_indices"),
                    "annotated_csv": preview.get("annotated_csv"),
                    "has_more": preview.get("has_more"),
                },
            }
        )

    return {"sheet_count": len(target_sheets), "sheets": profiles}, warnings


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    add_spreadsheet_args(parser, require_sheet=False, allow_sheet=True)
    parser.add_argument("--preview-rows", type=int, default=15)
    parser.add_argument("--max-preview-cols", type=int, default=100)
    parser.add_argument("--max-chars", type=int, default=8000)
    parser.add_argument("--max-sheets", type=int, default=3)
    parser.add_argument("--timeout", type=int, default=60)
    args = parser.parse_args()
    try:
        data, warnings = inspect_workbook(args)
    except (LarkCliError, ValueError, TypeError) as exc:
        emit_error(ACTION, str(exc))
    emit_success(ACTION, data, warnings)


if __name__ == "__main__":
    main()
