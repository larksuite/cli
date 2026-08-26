#!/usr/bin/env python3
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
"""Check whether Lark Sheet charts have obvious placement problems.

The single required argument is a spreadsheet URL or spreadsheet token. By
default every worksheet is checked; pass --worksheet-id to restrict the check
to one worksheet reference_id.

Exit codes:
  0: check completed and no layout issue was found
  1: the check could not be completed (CLI/read/response error)
  2: check completed and at least one layout issue was found
"""

from __future__ import annotations

import argparse
import json
from typing import Any

from lark_sheet_read_cli import (
    LarkCliError,
    emit_error,
    envelope_data,
    resolve_target_sheets,
    run_sheets,
    sheet_identifier,
    sheet_title,
)

ACTION = "chart_layout_check"
DEFAULT_COLUMN_WIDTH = 105.0
DEFAULT_ROW_HEIGHT = 27.0


def column_to_index(column: str) -> int:
    value = 0
    text = str(column).strip().upper()
    if not text or not text.isalpha():
        raise ValueError(f"Invalid column: {column!r}")
    for char in text:
        value = value * 26 + ord(char) - ord("A") + 1
    return value - 1


def index_to_column(index: int) -> str:
    if index < 0:
        raise ValueError(f"Invalid column index: {index}")
    chars: list[str] = []
    value = index + 1
    while value:
        value, remainder = divmod(value - 1, 26)
        chars.append(chr(ord("A") + remainder))
    return "".join(reversed(chars))


def _span_bounds(span: str, *, columns: bool) -> tuple[int, int]:
    start, separator, end = str(span).partition(":")
    end = end if separator else start
    if columns:
        return column_to_index(start), column_to_index(end)
    return int(start) - 1, int(end) - 1


def _size_edges(
    groups: Any,
    *,
    count: int,
    span_key: str,
    size_key: str,
    columns: bool,
    default_size: float,
) -> tuple[list[float], bool]:
    sizes: list[float | None] = [None] * count
    if isinstance(groups, list):
        for group in groups:
            if not isinstance(group, dict) or group.get(span_key) is None:
                continue
            start, end = _span_bounds(str(group[span_key]), columns=columns)
            size = float(group.get(size_key, default_size))
            for index in range(max(0, start), min(count - 1, end) + 1):
                sizes[index] = max(0.0, size)

    used_default = any(size is None for size in sizes)
    resolved = [default_size if size is None else size for size in sizes]
    edges = [0.0]
    for size in resolved:
        edges.append(edges[-1] + size)
    return edges, used_default


def build_layout(
    structure: dict[str, Any], row_count: int, column_count: int
) -> tuple[list[float], list[float], list[str]]:
    row_groups = structure.get("row_heights")
    column_groups = structure.get("col_widths", structure.get("column_widths"))
    row_edges, row_defaulted = _size_edges(
        row_groups,
        count=row_count,
        span_key="rows",
        size_key="height",
        columns=False,
        default_size=DEFAULT_ROW_HEIGHT,
    )
    column_edges, column_defaulted = _size_edges(
        column_groups,
        count=column_count,
        span_key="cols",
        size_key="width",
        columns=True,
        default_size=DEFAULT_COLUMN_WIDTH,
    )
    warnings: list[str] = []
    if row_defaulted:
        warnings.append("部分行缺少高度信息，按 27 px 估算")
    if column_defaulted:
        warnings.append("部分列缺少宽度信息，按 105 px 估算")
    return row_edges, column_edges, warnings


def _first_dict(value: Any) -> dict[str, Any] | None:
    if isinstance(value, dict):
        return value
    if isinstance(value, list):
        return next((item for item in value if isinstance(item, dict)), None)
    return None


def extract_sheet_structure(data: dict[str, Any]) -> dict[str, Any]:
    sheet = _first_dict(data.get("sheets")) or _first_dict(data.get("sheet"))
    return sheet or data


def extract_charts(data: dict[str, Any], sheet_id: str, title: str) -> list[dict[str, Any]]:
    sheets = data.get("sheets")
    if isinstance(sheets, list):
        for sheet in sheets:
            if not isinstance(sheet, dict):
                continue
            if sheet_identifier(sheet) == sheet_id or sheet_title(sheet) == title:
                charts = sheet.get("charts")
                return [chart for chart in charts if isinstance(chart, dict)] if isinstance(charts, list) else []
    charts = data.get("charts")
    return [chart for chart in charts if isinstance(chart, dict)] if isinstance(charts, list) else []


def chart_rectangle(
    chart: dict[str, Any], row_edges: list[float], column_edges: list[float]
) -> dict[str, Any]:
    details = chart.get("details") if isinstance(chart.get("details"), dict) else chart
    position = details.get("position") if isinstance(details.get("position"), dict) else {}
    offset = details.get("offset") if isinstance(details.get("offset"), dict) else {}
    size = details.get("size") if isinstance(details.get("size"), dict) else {}

    row = int(position["row"])
    column = column_to_index(str(position["col"]))
    if row < 0 or column < 0 or row >= len(row_edges) - 1 or column >= len(column_edges) - 1:
        raise ValueError(f"anchor outside sheet: {position!r}")

    width = float(size["width"])
    height = float(size["height"])
    if width <= 0 or height <= 0:
        raise ValueError(f"invalid chart size: {size!r}")

    left = column_edges[column] + float(offset.get("col_offset", 0) or 0)
    top = row_edges[row] + float(offset.get("row_offset", 0) or 0)
    return {
        "chart_id": str(chart.get("chart_id") or chart.get("id") or ""),
        "anchor_cell": f"{index_to_column(column)}{row + 1}",
        "left": left,
        "top": top,
        "right": left + width,
        "bottom": top + height,
        "width": width,
        "height": height,
    }


def intersection(first: dict[str, Any], second: dict[str, Any]) -> dict[str, float] | None:
    left = max(float(first["left"]), float(second["left"]))
    top = max(float(first["top"]), float(second["top"]))
    right = min(float(first["right"]), float(second["right"]))
    bottom = min(float(first["bottom"]), float(second["bottom"]))
    if right <= left or bottom <= top:
        return None
    return {
        "width": round(right - left, 2),
        "height": round(bottom - top, 2),
        "area": round((right - left) * (bottom - top), 2),
    }


def chart_context(rectangle: dict[str, Any]) -> dict[str, Any]:
    return {
        "chart_id": rectangle["chart_id"],
        "anchor_cell": rectangle["anchor_cell"],
        "rectangle_px": {
            "left": round(rectangle["left"], 2),
            "top": round(rectangle["top"], 2),
            "right": round(rectangle["right"], 2),
            "bottom": round(rectangle["bottom"], 2),
            "width": round(rectangle["width"], 2),
            "height": round(rectangle["height"], 2),
        },
    }


def _covered_indexes(edges: list[float], start: float, end: float) -> list[int]:
    return [
        index
        for index in range(len(edges) - 1)
        if edges[index + 1] > start and edges[index] < end
    ]


def rectangle_cell_range(
    rectangle: dict[str, Any], row_edges: list[float], column_edges: list[float]
) -> str | None:
    rows = _covered_indexes(row_edges, max(0.0, rectangle["top"]), rectangle["bottom"])
    columns = _covered_indexes(column_edges, max(0.0, rectangle["left"]), rectangle["right"])
    if not rows or not columns:
        return None
    return f"{index_to_column(columns[0])}{rows[0] + 1}:{index_to_column(columns[-1])}{rows[-1] + 1}"


def _has_content(cell: Any) -> bool:
    if not isinstance(cell, dict):
        return False
    for key in ("value", "formula", "note"):
        value = cell.get(key)
        if value not in (None, ""):
            return True
    return bool(cell.get("rich_text") or cell.get("multiple_values"))


def non_empty_cells(data: dict[str, Any], sample_limit: int) -> tuple[int, list[str], bool]:
    count = 0
    samples: list[str] = []
    truncated = bool(data.get("has_more"))
    ranges = data.get("ranges")
    if not isinstance(ranges, list):
        return 0, [], truncated
    for result_range in ranges:
        if not isinstance(result_range, dict):
            continue
        truncated = truncated or bool(result_range.get("truncated"))
        cells = result_range.get("cells")
        rows = result_range.get("row_indices")
        columns = result_range.get("col_indices")
        if not isinstance(cells, list):
            continue
        for row_offset, row in enumerate(cells):
            if not isinstance(row, list):
                continue
            row_number = rows[row_offset] if isinstance(rows, list) and row_offset < len(rows) else row_offset + 1
            for column_offset, cell in enumerate(row):
                if not _has_content(cell):
                    continue
                count += 1
                if len(samples) < sample_limit:
                    column = columns[column_offset] if isinstance(columns, list) and column_offset < len(columns) else index_to_column(column_offset)
                    samples.append(f"{column}{row_number}")
    return count, samples, truncated


def _locator(target: str) -> dict[str, str]:
    return {"url": target} if target.startswith(("http://", "https://")) else {"spreadsheet_token": target}


def _sheet_counts(sheet: dict[str, Any]) -> tuple[int, int]:
    row_count = int(sheet.get("row_count") or sheet.get("rowCount") or 0)
    column_count = int(sheet.get("column_count") or sheet.get("columnCount") or 0)
    if row_count <= 0 or column_count <= 0:
        raise LarkCliError(f"Missing row_count/column_count for sheet {sheet_title(sheet)!r}")
    return row_count, column_count


def check_sheet(
    locator: dict[str, str], sheet: dict[str, Any], *, timeout: int, sample_limit: int
) -> dict[str, Any]:
    sheet_id = sheet_identifier(sheet)
    title = sheet_title(sheet)
    row_count, column_count = _sheet_counts(sheet)
    if not sheet_id:
        raise LarkCliError(f"Missing sheet_id for sheet {title!r}")

    structure_data = envelope_data(
        run_sheets(
            "+sheet-info",
            **locator,
            sheet_id=sheet_id,
            flags={"include": "row_heights,col_widths"},
            timeout=timeout,
        )
    )
    row_edges, column_edges, warnings = build_layout(
        extract_sheet_structure(structure_data), row_count, column_count
    )
    chart_data = envelope_data(
        run_sheets("+chart-list", **locator, sheet_id=sheet_id, timeout=timeout)
    )
    charts = extract_charts(chart_data, sheet_id, title)

    rectangles: list[dict[str, Any]] = []
    unverifiable: list[dict[str, str]] = []
    expected_chart_count = sheet.get("chart_count")
    if expected_chart_count is not None and int(expected_chart_count) != len(charts):
        unverifiable.append(
            {
                "chart_id": "",
                "reason": (
                    f"chart-list returned {len(charts)} charts, "
                    f"but workbook-info reported {int(expected_chart_count)}"
                ),
            }
        )
    for chart in charts:
        chart_id = str(chart.get("chart_id") or chart.get("id") or "")
        if not chart_id:
            unverifiable.append({"chart_id": "", "reason": "chart is missing chart_id"})
            continue
        try:
            rectangles.append(chart_rectangle(chart, row_edges, column_edges))
        except (KeyError, TypeError, ValueError) as exc:
            unverifiable.append({"chart_id": chart_id, "reason": str(exc)})

    overlaps: list[dict[str, Any]] = []
    for index, first in enumerate(rectangles):
        for second in rectangles[index + 1 :]:
            overlap = intersection(first, second)
            if overlap:
                overlaps.append(
                    {
                        "chart_ids": [first["chart_id"], second["chart_id"]],
                        "charts": [chart_context(first), chart_context(second)],
                        "intersection": overlap,
                    }
                )

    sheet_width = column_edges[-1]
    sheet_height = row_edges[-1]
    out_of_bounds: list[dict[str, Any]] = []
    content_overlaps: list[dict[str, Any]] = []
    for rectangle in rectangles:
        overflow = {
            "left": round(max(0.0, -rectangle["left"]), 2),
            "top": round(max(0.0, -rectangle["top"]), 2),
            "right": round(max(0.0, rectangle["right"] - sheet_width), 2),
            "bottom": round(max(0.0, rectangle["bottom"] - sheet_height), 2),
        }
        if any(overflow.values()):
            out_of_bounds.append({**chart_context(rectangle), "overflow_px": overflow})

        covered_range = rectangle_cell_range(rectangle, row_edges, column_edges)
        if not covered_range:
            continue
        cells_data = envelope_data(
            run_sheets(
                "+cells-get",
                **locator,
                sheet_id=sheet_id,
                flags={"range": covered_range, "include": "value,formula,comment"},
                timeout=timeout,
            )
        )
        count, samples, truncated = non_empty_cells(cells_data, sample_limit)
        if truncated:
            unverifiable.append(
                {"chart_id": rectangle["chart_id"], "reason": f"cells-get truncated for {covered_range}"}
            )
        if count:
            content_overlaps.append(
                {
                    **chart_context(rectangle),
                    "covered_range": covered_range,
                    "non_empty_cell_count": count,
                    "sample_cells": samples,
                }
            )

    issue_count = len(overlaps) + len(out_of_bounds) + len(content_overlaps)
    return {
        "sheet_id": sheet_id,
        "sheet_name": title,
        "chart_count": len(charts),
        "sheet_size_px": {"width": round(sheet_width, 2), "height": round(sheet_height, 2)},
        "chart_overlaps": overlaps,
        "cell_content_overlaps": content_overlaps,
        "out_of_visible_range": out_of_bounds,
        "unverifiable_charts": unverifiable,
        "issue_count": issue_count,
        "unverifiable_count": len(unverifiable),
        "warnings": warnings,
    }


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Check chart overlap, covered cell content, and worksheet boundary overflow."
    )
    parser.add_argument("sheet_id", help="Spreadsheet URL or spreadsheet token")
    parser.add_argument("--worksheet-id", help="Only check this worksheet reference_id")
    parser.add_argument("--timeout", type=int, default=60)
    parser.add_argument("--sample-limit", type=int, default=10)
    return parser.parse_args()


def success_envelope(results: list[dict[str, Any]]) -> dict[str, Any]:
    issue_count = sum(result["issue_count"] for result in results)
    unverifiable_count = sum(result["unverifiable_count"] for result in results)
    warnings = [
        f"{result['sheet_name'] or result['sheet_id']}: {warning}"
        for result in results
        for warning in result["warnings"]
    ]
    return {
        "ok": True,
        "engine": "lark",
        "action": ACTION,
        "data": {
            "passed": issue_count == 0 and unverifiable_count == 0,
            "scope_note": "out_of_visible_range checks worksheet drawable bounds, not a device-specific browser viewport",
            "summary": {
                "worksheet_count": len(results),
                "chart_count": sum(result["chart_count"] for result in results),
                "issue_count": issue_count,
                "unverifiable_count": unverifiable_count,
            },
            "sheets": results,
        },
        "warnings": warnings,
    }


def report_exit_code(report: dict[str, Any]) -> int:
    if report["data"]["passed"]:
        return 0
    if report["data"]["summary"]["issue_count"] > 0:
        return 2
    return 1


def main() -> None:
    args = parse_args()
    locator = _locator(args.sheet_id)
    try:
        workbook_data = envelope_data(
            run_sheets("+workbook-info", **locator, timeout=args.timeout)
        )
        sheets = resolve_target_sheets(workbook_data, sheet_id=args.worksheet_id)
        if not args.worksheet_id:
            sheets = [sheet for sheet in sheets if not bool(sheet.get("is_hidden"))]
        if not sheets:
            raise LarkCliError("No visible worksheet matched")
        results = [
            check_sheet(locator, sheet, timeout=args.timeout, sample_limit=args.sample_limit)
            for sheet in sheets
        ]
    except (LarkCliError, KeyError, TypeError, ValueError) as exc:
        emit_error(ACTION, str(exc))
        raise SystemExit(1) from exc

    report = success_envelope(results)
    print(json.dumps(report, ensure_ascii=False, indent=2))
    exit_code = report_exit_code(report)
    if exit_code:
        raise SystemExit(exit_code)


if __name__ == "__main__":
    main()
