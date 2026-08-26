#!/usr/bin/env python3
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
"""Check Lark Sheet chart placement and numeric source-data issues.

The single required argument is a spreadsheet URL or spreadsheet token. By
default every worksheet is checked; pass --worksheet-id to restrict the check
to one worksheet reference_id.

Exit codes:
  0: check completed and no issue was found
  1: the check could not be completed (CLI/read/response error)
  2: check completed and at least one layout or source-data issue was found
"""

from __future__ import annotations

import argparse
import json
import re
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
MAX_CELL_READ_SIZE = 2_000
MAX_SOURCE_SAMPLE_POINTS = 50
MAX_ZERO_SCAN_CELLS = 10_000


CellBounds = tuple[int, int, int, int]
CellCache = dict[tuple[str, str, str], dict[str, Any]]


def _parse_a1_bounds(cell_range: str) -> CellBounds:
    value = str(cell_range).rsplit("!", 1)[-1].replace("$", "")
    match = re.fullmatch(r"([A-Za-z]+)(\d+)(?::([A-Za-z]+)(\d+))?", value)
    if not match:
        raise ValueError(f"Invalid A1 range: {cell_range!r}")
    start_column = column_to_index(match.group(1))
    start_row = int(match.group(2))
    end_column = column_to_index(match.group(3) or match.group(1))
    end_row = int(match.group(4) or match.group(2))
    if end_row < start_row or end_column < start_column:
        raise ValueError(f"Invalid A1 range: {cell_range!r}")
    return start_row, end_row, start_column, end_column


def _format_a1_bounds(bounds: CellBounds) -> str:
    start_row, end_row, start_column, end_column = bounds
    return (
        f"{index_to_column(start_column)}{start_row}:"
        f"{index_to_column(end_column)}{end_row}"
    )


def _bounds_area(bounds: CellBounds) -> int:
    start_row, end_row, start_column, end_column = bounds
    return (end_row - start_row + 1) * (end_column - start_column + 1)


def _merge_bounds(first: CellBounds, second: CellBounds) -> CellBounds:
    return (
        min(first[0], second[0]),
        max(first[1], second[1]),
        min(first[2], second[2]),
        max(first[3], second[3]),
    )


def _cluster_cell_reads(
    items: list[tuple[dict[str, Any], CellBounds]],
) -> list[dict[str, Any]]:
    clusters: list[dict[str, Any]] = []
    for rectangle, bounds in sorted(items, key=lambda item: (item[1][0], item[1][2])):
        if clusters:
            merged = _merge_bounds(clusters[-1]["bounds"], bounds)
            if _bounds_area(merged) <= MAX_CELL_READ_SIZE:
                clusters[-1]["bounds"] = merged
                clusters[-1]["members"].append((rectangle, bounds))
                continue
        clusters.append({"bounds": bounds, "members": [(rectangle, bounds)]})
    return clusters


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


def _iter_cells(data: dict[str, Any]):
    ranges = data.get("ranges")
    if not isinstance(ranges, list):
        return
    for result_range in ranges:
        if not isinstance(result_range, dict):
            continue
        cells = result_range.get("cells")
        rows = result_range.get("row_indices")
        columns = result_range.get("col_indices")
        if not isinstance(cells, list):
            continue
        for row_offset, row in enumerate(cells):
            if not isinstance(row, list):
                continue
            row_number = int(rows[row_offset]) if isinstance(rows, list) and row_offset < len(rows) else row_offset + 1
            for column_offset, cell in enumerate(row):
                column = columns[column_offset] if isinstance(columns, list) and column_offset < len(columns) else index_to_column(column_offset)
                yield row_number, column_to_index(str(column)), cell


def non_empty_cells(
    data: dict[str, Any], sample_limit: int, bounds: CellBounds | None = None
) -> tuple[int, list[str], bool]:
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
    for row_number, column_index, cell in _iter_cells(data):
        if bounds and not (
            bounds[0] <= row_number <= bounds[1]
            and bounds[2] <= column_index <= bounds[3]
        ):
            continue
        if not _has_content(cell):
            continue
        count += 1
        if len(samples) < sample_limit:
            samples.append(f"{index_to_column(column_index)}{row_number}")
    return count, samples, truncated


def _read_cells(
    cache: CellCache,
    locator: dict[str, str],
    *,
    sheet_id: str | None,
    sheet_name: str | None,
    cell_range: str,
    include: str,
    timeout: int,
) -> dict[str, Any]:
    selector = f"id:{sheet_id}" if sheet_id else f"name:{sheet_name}"
    key = (selector, cell_range, include)
    if key not in cache:
        cache[key] = envelope_data(
            run_sheets(
                "+cells-get",
                **locator,
                **({"sheet_id": sheet_id} if sheet_id else {"sheet_name": sheet_name}),
                flags={"range": cell_range, "include": key[2]},
                timeout=timeout,
            )
        )
    return cache[key]


def _chart_snapshot(chart: dict[str, Any]) -> dict[str, Any]:
    details = chart.get("details") if isinstance(chart.get("details"), dict) else chart
    snapshot = details.get("snapshot")
    return snapshot if isinstance(snapshot, dict) else {}


def _chart_type(snapshot: dict[str, Any]) -> str:
    plot_area = snapshot.get("plotArea")
    plot = plot_area.get("plot") if isinstance(plot_area, dict) else None
    return str(plot.get("type") or "").lower() if isinstance(plot, dict) else ""


def _numeric_dimensions(snapshot: dict[str, Any]) -> list[tuple[int, str]]:
    data = snapshot.get("data")
    if not isinstance(data, dict):
        return []
    dim2 = data.get("dim2")
    series = dim2.get("series") if isinstance(dim2, dict) else None
    chart_type = _chart_type(snapshot)
    dimensions: list[tuple[int, str]] = []
    if isinstance(series, list):
        for offset, serie in enumerate(series):
            if not isinstance(serie, dict) or serie.get("index") is None:
                continue
            if str(serie.get("aggregateType") or "").lower() == "counta":
                continue
            role = str(serie.get("role") or "").lower()
            if chart_type == "bubble":
                role = role or ("x", "y", "group", "size")[min(offset, 3)]
                if role not in {"x", "y", "size"}:
                    continue
            dimensions.append((int(serie["index"]), role or "value"))

    plot_area = snapshot.get("plotArea")
    axes = plot_area.get("axes") if isinstance(plot_area, dict) else None
    continuous_x = chart_type == "scatter"
    if isinstance(axes, list):
        continuous_x = continuous_x or any(
            isinstance(axis, dict)
            and str(axis.get("axisPosition") or axis.get("position") or "").lower() in {"bottom", "x"}
            and str(axis.get("valueType") or "").lower() == "linear"
            for axis in axes
        )
    dim1 = data.get("dim1")
    serie = dim1.get("serie") if isinstance(dim1, dict) else None
    if continuous_x and chart_type != "bubble" and isinstance(serie, dict) and serie.get("index") is not None:
        dimensions.append((int(serie["index"]), "x"))

    return list(dict.fromkeys(dimensions))


def _parse_chart_ref(value: str, default_sheet: str) -> tuple[str, str, CellBounds]:
    raw = str(value).strip()
    sheet_name = default_sheet
    cell_range = raw
    if "!" in raw:
        sheet_name, cell_range = raw.rsplit("!", 1)
        sheet_name = sheet_name.strip()
        if len(sheet_name) >= 2 and sheet_name[0] == sheet_name[-1] == "'":
            sheet_name = sheet_name[1:-1].replace("''", "'")
    return sheet_name, cell_range.replace("$", ""), _parse_a1_bounds(cell_range)


def _looks_numeric(value: str) -> bool:
    return _numeric_text_value(value) is not None


def _numeric_text_value(value: str) -> float | None:
    text = value.strip().replace(" ", "").replace(",", "")
    if not text:
        return None
    text = re.sub(r"^([+-]?)[\$\u00a5\uffe5\u20ac\u00a3]", r"\1", text)
    if text.endswith("%"):
        text = text[:-1]
    if not re.fullmatch(r"[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?", text):
        return None
    return float(text)


def _zero_state(value: Any) -> str:
    if value in (None, ""):
        return "empty"
    if isinstance(value, (int, float)) and not isinstance(value, bool):
        return "zero" if value == 0 else "nonzero"
    if isinstance(value, str):
        numeric_value = _numeric_text_value(value)
        if numeric_value is not None:
            return "zero" if numeric_value == 0 else "nonzero"
    return "nonzero"


def _cells_truncated(data: dict[str, Any]) -> bool:
    ranges = data.get("ranges")
    return bool(data.get("has_more")) or any(
        isinstance(item, dict) and item.get("truncated")
        for item in (ranges if isinstance(ranges, list) else [])
    )


def _numeric_source_issues(
    chart: dict[str, Any],
    *,
    owner_sheet_id: str,
    owner_sheet_name: str,
    cache: CellCache,
    locator: dict[str, str],
    timeout: int,
    sample_limit: int,
) -> tuple[list[dict[str, Any]], list[dict[str, Any]], list[dict[str, str]]]:
    chart_id = str(chart.get("chart_id") or chart.get("id") or "")
    snapshot = _chart_snapshot(chart)
    data = snapshot.get("data")
    if not isinstance(data, dict):
        return [], [], [{"chart_id": chart_id, "reason": "chart snapshot.data is missing"}]
    if data.get("isStaticData") is True:
        return [], [], []
    dimensions = _numeric_dimensions(snapshot)
    if not dimensions:
        return [], [], []
    refs = data.get("refs")
    if not isinstance(refs, list) or not refs:
        return [], [], [{"chart_id": chart_id, "reason": "chart data.refs is missing"}]

    parsed_refs: list[tuple[str, str, CellBounds]] = []
    unverifiable: list[dict[str, str]] = []
    for ref in refs:
        raw_ref = ref.get("value") if isinstance(ref, dict) else ref
        try:
            parsed_refs.append(_parse_chart_ref(str(raw_ref), owner_sheet_name))
        except (TypeError, ValueError) as exc:
            unverifiable.append({"chart_id": chart_id, "reason": str(exc)})
            return [], [], unverifiable

    direction = str(data.get("direction") or "column").lower()
    mapped: list[tuple[int, str, int, str, str, CellBounds]] = []
    for dimension_index, role in dimensions:
        offset = 0
        for source_sheet, source_range, bounds in parsed_refs:
            dimension_count = (
                bounds[3] - bounds[2] + 1 if direction == "column" else bounds[1] - bounds[0] + 1
            )
            if offset < dimension_index <= offset + dimension_count:
                mapped.append(
                    (
                        dimension_index,
                        role,
                        dimension_index - offset,
                        source_sheet,
                        source_range,
                        bounds,
                    )
                )
                break
            offset += dimension_count
        else:
            unverifiable.append(
                {
                    "chart_id": chart_id,
                    "reason": f"numeric dimension index {dimension_index} is outside data.refs",
                }
            )

    detached = str(data.get("headerMode") or "").lower() == "detached"
    mapped_by_ref: dict[
        tuple[str, str, CellBounds], list[tuple[int, str, int]]
    ] = {}
    for dimension_index, role, local_index, source_sheet, source_range, bounds in mapped:
        mapped_by_ref.setdefault((source_sheet, source_range, bounds), []).append(
            (dimension_index, role, local_index)
        )

    issue_groups: dict[tuple[int, str, str, str, str, str], list[str]] = {}
    issue_counts: dict[tuple[int, str, str, str, str, str], int] = {}
    degenerate_series: list[dict[str, Any]] = []
    for (source_sheet, source_range, bounds), ref_dimensions in mapped_by_ref.items():
        if direction == "column":
            selected = {
                bounds[2] + local_index - 1: (dimension_index, role)
                for dimension_index, role, local_index in ref_dimensions
            }
            point_count = MAX_SOURCE_SAMPLE_POINTS + (0 if detached else 1)
            checked_bounds = (
                bounds[0],
                min(bounds[1], bounds[0] + point_count - 1),
                min(selected),
                max(selected),
            )
        else:
            selected = {
                bounds[0] + local_index - 1: (dimension_index, role)
                for dimension_index, role, local_index in ref_dimensions
            }
            point_count = MAX_SOURCE_SAMPLE_POINTS + (0 if detached else 1)
            checked_bounds = (
                min(selected),
                max(selected),
                bounds[2],
                min(bounds[3], bounds[2] + point_count - 1),
            )
        checked_range = _format_a1_bounds(checked_bounds)
        same_sheet = source_sheet == owner_sheet_name
        cells_data = _read_cells(
            cache,
            locator,
            sheet_id=owner_sheet_id if same_sheet else None,
            sheet_name=None if same_sheet else source_sheet,
            cell_range=checked_range,
            include="value,style",
            timeout=timeout,
        )
        truncated = _cells_truncated(cells_data)
        if truncated:
            unverifiable.append(
                {"chart_id": chart_id, "reason": f"cells-get truncated for {source_sheet}!{checked_range}"}
            )
        states = {
            coordinate: {"zero": False, "empty": False, "nonzero": False}
            for coordinate in selected
        }
        for row_number, column_index, cell in _iter_cells(cells_data):
            coordinate = column_index if direction == "column" else row_number
            dimension = selected.get(coordinate)
            if dimension is None:
                continue
            if not detached and (
                (direction == "column" and row_number == bounds[0])
                or (direction != "column" and column_index == bounds[2])
            ):
                continue
            value = cell.get("value") if isinstance(cell, dict) else None
            states[coordinate][_zero_state(value)] = True
            number_format = (
                cell.get("cell_styles", {}).get("number_format")
                if isinstance(cell, dict) and isinstance(cell.get("cell_styles"), dict)
                else None
            )
            reason = ""
            uses_text_format = str(number_format or "").strip() == "@"
            if isinstance(value, (int, float)) and not isinstance(value, bool) and uses_text_format:
                reason = "numeric_value_uses_text_format"
            elif isinstance(value, str) and _looks_numeric(value) and uses_text_format:
                reason = "numeric_value_stored_as_text"
            if not reason:
                continue
            dimension_index, role = dimension
            key = (dimension_index, role, source_sheet, source_range, checked_range, reason)
            issue_counts[key] = issue_counts.get(key, 0) + 1
            samples = issue_groups.setdefault(key, [])
            if len(samples) < sample_limit:
                samples.append(f"{index_to_column(column_index)}{row_number}")

        candidates = {
            coordinate for coordinate, state in states.items() if not state["nonzero"]
        }
        if not truncated and candidates:
            cursor = checked_bounds[1] + 1 if direction == "column" else checked_bounds[3] + 1
            end = bounds[1] if direction == "column" else bounds[3]
            while candidates and cursor <= end:
                fixed_span = max(candidates) - min(candidates) + 1
                points_per_window = max(1, MAX_ZERO_SCAN_CELLS // fixed_span)
                window_end = min(end, cursor + points_per_window - 1)
                scan_bounds = (
                    (cursor, window_end, min(candidates), max(candidates))
                    if direction == "column"
                    else (min(candidates), max(candidates), cursor, window_end)
                )
                scan_range = _format_a1_bounds(scan_bounds)
                scan_data = _read_cells(
                    cache,
                    locator,
                    sheet_id=owner_sheet_id if same_sheet else None,
                    sheet_name=None if same_sheet else source_sheet,
                    cell_range=scan_range,
                    include="value",
                    timeout=timeout,
                )
                if _cells_truncated(scan_data):
                    truncated = True
                    unverifiable.append(
                        {
                            "chart_id": chart_id,
                            "reason": f"cells-get truncated for {source_sheet}!{scan_range}",
                        }
                    )
                    break
                for row_number, column_index, cell in _iter_cells(scan_data):
                    coordinate = column_index if direction == "column" else row_number
                    if coordinate not in candidates:
                        continue
                    value = cell.get("value") if isinstance(cell, dict) else None
                    states[coordinate][_zero_state(value)] = True
                candidates = {
                    coordinate for coordinate in candidates if not states[coordinate]["nonzero"]
                }
                cursor = window_end + 1

        if truncated:
            continue
        data_start = bounds[0] + (0 if detached else 1)
        data_column = bounds[2] + (0 if detached else 1)
        for coordinate in candidates:
            dimension_index, role = selected[coordinate]
            if direction == "column":
                point_total = max(0, bounds[1] - data_start + 1)
                series_range = (
                    f"{index_to_column(coordinate)}{data_start}:"
                    f"{index_to_column(coordinate)}{bounds[1]}"
                    if point_total
                    else ""
                )
            else:
                point_total = max(0, bounds[3] - data_column + 1)
                series_range = (
                    f"{index_to_column(data_column)}{coordinate}:"
                    f"{index_to_column(bounds[3])}{coordinate}"
                    if point_total
                    else ""
                )
            degenerate_series.append(
                {
                    "chart_id": chart_id,
                    "dimension_index": dimension_index,
                    "role": role,
                    "source_sheet": source_sheet,
                    "source_range": source_range,
                    "series_range": series_range,
                    "reason": (
                        "numeric_series_all_zero_or_empty"
                        if states[coordinate]["zero"]
                        else "numeric_series_all_empty"
                    ),
                    "data_point_count": point_total,
                }
            )

    issues = [
        {
            "chart_id": chart_id,
            "dimension_index": key[0],
            "role": key[1],
            "source_sheet": key[2],
            "source_range": key[3],
            "checked_range": key[4],
            "reason": key[5],
            "suggested_fix": (
                "set_numeric_number_format"
                if key[5] == "numeric_value_uses_text_format"
                else "rewrite_as_number_and_set_numeric_number_format"
            ),
            "affected_sample_cell_count": issue_counts[key],
            "sample_cells": samples,
        }
        for key, samples in issue_groups.items()
    ]
    return issues, degenerate_series, unverifiable


def _locator(target: str) -> dict[str, str]:
    return {"url": target} if target.startswith(("http://", "https://")) else {"spreadsheet_token": target}


def _sheet_counts(sheet: dict[str, Any]) -> tuple[int, int]:
    row_count = int(sheet.get("row_count") or sheet.get("rowCount") or 0)
    column_count = int(sheet.get("column_count") or sheet.get("columnCount") or 0)
    if row_count <= 0 or column_count <= 0:
        raise LarkCliError(f"Missing row_count/column_count for sheet {sheet_title(sheet)!r}")
    return row_count, column_count


def check_sheet(
    locator: dict[str, str],
    sheet: dict[str, Any],
    *,
    timeout: int,
    sample_limit: int,
    cell_cache: CellCache | None = None,
) -> dict[str, Any]:
    cell_cache = cell_cache if cell_cache is not None else {}
    sheet_id = sheet_identifier(sheet)
    title = sheet_title(sheet)
    if not sheet_id:
        raise LarkCliError(f"Missing sheet_id for sheet {title!r}")

    chart_data = envelope_data(
        run_sheets("+chart-list", **locator, sheet_id=sheet_id, timeout=timeout)
    )
    charts = extract_charts(chart_data, sheet_id, title)
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
    if not charts:
        return {
            "sheet_id": sheet_id,
            "sheet_name": title,
            "chart_count": 0,
            "sheet_size_px": None,
            "chart_overlaps": [],
            "cell_content_overlaps": [],
            "numeric_source_format_issues": [],
            "degenerate_numeric_series": [],
            "out_of_visible_range": [],
            "unverifiable_charts": unverifiable,
            "issue_count": 0,
            "unverifiable_count": len(unverifiable),
            "warnings": [],
        }

    row_count, column_count = _sheet_counts(sheet)
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

    rectangles: list[dict[str, Any]] = []
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
    covered_items: list[tuple[dict[str, Any], CellBounds]] = []
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
        covered_items.append((rectangle, _parse_a1_bounds(covered_range)))

    for cluster in _cluster_cell_reads(covered_items):
        read_range = _format_a1_bounds(cluster["bounds"])
        cells_data = _read_cells(
            cell_cache,
            locator,
            sheet_id=sheet_id,
            sheet_name=None,
            cell_range=read_range,
            include="value,formula,comment",
            timeout=timeout,
        )
        for rectangle, bounds in cluster["members"]:
            covered_range = _format_a1_bounds(bounds)
            count, samples, truncated = non_empty_cells(cells_data, sample_limit, bounds)
            if truncated:
                unverifiable.append(
                    {
                        "chart_id": rectangle["chart_id"],
                        "reason": f"cells-get truncated for {read_range}",
                    }
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

    numeric_source_issues: list[dict[str, Any]] = []
    degenerate_numeric_series: list[dict[str, Any]] = []
    for chart in charts:
        issues, degenerate, source_unverifiable = _numeric_source_issues(
            chart,
            owner_sheet_id=sheet_id,
            owner_sheet_name=title,
            cache=cell_cache,
            locator=locator,
            timeout=timeout,
            sample_limit=sample_limit,
        )
        numeric_source_issues.extend(issues)
        degenerate_numeric_series.extend(degenerate)
        unverifiable.extend(source_unverifiable)

    issue_count = (
        len(overlaps)
        + len(out_of_bounds)
        + len(content_overlaps)
        + len(numeric_source_issues)
        + len(degenerate_numeric_series)
    )
    return {
        "sheet_id": sheet_id,
        "sheet_name": title,
        "chart_count": len(charts),
        "sheet_size_px": {"width": round(sheet_width, 2), "height": round(sheet_height, 2)},
        "chart_overlaps": overlaps,
        "cell_content_overlaps": content_overlaps,
        "numeric_source_format_issues": numeric_source_issues,
        "degenerate_numeric_series": degenerate_numeric_series,
        "out_of_visible_range": out_of_bounds,
        "unverifiable_charts": unverifiable,
        "issue_count": issue_count,
        "unverifiable_count": len(unverifiable),
        "warnings": warnings,
    }


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Check chart overlap, covered cell content, worksheet boundary overflow, "
            "numeric source-cell formats, and all-zero/empty numeric series."
        )
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
            "scope_note": (
                "out_of_visible_range checks worksheet drawable bounds, not a device-specific browser viewport; "
                "numeric source checks sample at most the first 50 data points of each chart value dimension "
                "for formats and scan the full series for all-zero/empty values"
            ),
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
    cell_cache: CellCache = {}
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
            check_sheet(
                locator,
                sheet,
                timeout=args.timeout,
                sample_limit=args.sample_limit,
                cell_cache=cell_cache,
            )
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
