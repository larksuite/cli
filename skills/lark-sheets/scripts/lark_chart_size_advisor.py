#!/usr/bin/env python3
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
"""Recommend a Lark Sheet chart size before creating the chart object."""

from __future__ import annotations

import argparse
import json
import re
from typing import Any

from lark_chart_size_rules import recommend_chart_size
from lark_sheet_read_cli import (
    LarkCliError,
    emit_error,
    emit_success,
    envelope_data,
    extract_sheets,
    run_sheets,
    sheet_identifier,
    sheet_title,
)

ACTION = "chart_size_advisor"


def _locator(target: str) -> dict[str, str]:
    return {"url": target} if target.startswith(("http://", "https://")) else {"spreadsheet_token": target}


def _split_range_refs(value: str) -> list[str]:
    refs: list[str] = []
    start = 0
    quoted = False
    index = 0
    while index < len(value):
        char = value[index]
        if char == "'":
            if quoted and index + 1 < len(value) and value[index + 1] == "'":
                index += 2
                continue
            quoted = not quoted
        elif char == "," and not quoted:
            ref = value[start:index].strip()
            if ref:
                refs.append(ref)
            start = index + 1
        index += 1
    tail = value[start:].strip()
    if tail:
        refs.append(tail)
    if not refs or quoted:
        raise ValueError(f"Invalid data range: {value!r}")
    return refs


def _parse_ref(value: str) -> tuple[str | None, str]:
    raw = value.strip()
    sheet_name = None
    cell_range = raw
    if "!" in raw:
        sheet_name, cell_range = raw.rsplit("!", 1)
        sheet_name = sheet_name.strip()
        if len(sheet_name) >= 2 and sheet_name[0] == sheet_name[-1] == "'":
            sheet_name = sheet_name[1:-1].replace("''", "'")
    cell_range = cell_range.replace("$", "")
    if not re.fullmatch(r"[A-Za-z]+\d+:[A-Za-z]+\d+", cell_range):
        raise ValueError(f"Invalid A1 range: {value!r}")
    return sheet_name, cell_range


def _cell_value(cell: Any) -> Any:
    if not isinstance(cell, dict):
        return None
    return cell.get("value", cell.get("raw_value"))


def _matrix(data: dict[str, Any]) -> list[list[Any]]:
    ranges = data.get("ranges")
    if not isinstance(ranges, list) or not ranges:
        return []
    result = ranges[0]
    cells = result.get("cells") if isinstance(result, dict) else None
    if not isinstance(cells, list):
        return []
    return [
        [_cell_value(cell) for cell in row]
        for row in cells
        if isinstance(row, list)
    ]


def _sheet_selector(
    sheets: list[dict[str, Any]],
    *,
    explicit_name: str | None,
    worksheet_id: str | None,
    worksheet_name: str | None,
) -> dict[str, str]:
    if explicit_name:
        matches = [sheet for sheet in sheets if sheet_title(sheet) == explicit_name]
    elif worksheet_id:
        matches = [sheet for sheet in sheets if sheet_identifier(sheet) == worksheet_id]
    elif worksheet_name:
        matches = [sheet for sheet in sheets if sheet_title(sheet) == worksheet_name]
    elif len(sheets) == 1:
        matches = sheets
    else:
        raise LarkCliError("Unqualified data ranges require --worksheet-id or --worksheet-name")
    if len(matches) != 1:
        target = explicit_name or worksheet_id or worksheet_name or ""
        raise LarkCliError(f"Worksheet not found or ambiguous: {target!r}")
    sheet_id = sheet_identifier(matches[0])
    if sheet_id:
        return {"sheet_id": sheet_id}
    return {"sheet_name": sheet_title(matches[0])}


def _read_ranges(
    locator: dict[str, str],
    sheets: list[dict[str, Any]],
    value: str,
    *,
    worksheet_id: str | None,
    worksheet_name: str | None,
    timeout: int,
) -> list[list[list[Any]]]:
    matrices: list[list[list[Any]]] = []
    for ref in _split_range_refs(value):
        explicit_name, cell_range = _parse_ref(ref)
        selector = _sheet_selector(
            sheets,
            explicit_name=explicit_name,
            worksheet_id=worksheet_id,
            worksheet_name=worksheet_name,
        )
        data = envelope_data(
            run_sheets(
                "+cells-get",
                **locator,
                **selector,
                flags={"range": cell_range, "include": "value,raw_value,style"},
                timeout=timeout,
            )
        )
        matrix = _matrix(data)
        if not matrix:
            raise LarkCliError(f"No cells returned for {ref}")
        matrices.append(matrix)
    return matrices


def _combine(matrices: list[list[list[Any]]], direction: str) -> list[list[Any]]:
    if direction == "column":
        row_count = len(matrices[0])
        if any(len(matrix) != row_count for matrix in matrices):
            raise ValueError("Column-direction ranges must contain the same number of rows")
        return [sum((matrix[row] for matrix in matrices), []) for row in range(row_count)]
    column_count = max((len(row) for row in matrices[0]), default=0)
    if any(max((len(row) for row in matrix), default=0) != column_count for matrix in matrices):
        raise ValueError("Row-direction ranges must contain the same number of columns")
    return sum(matrices, [])


def _parse_indexes(value: str | None, *, dimension_count: int, dim1_index: int) -> list[int]:
    indexes = (
        [int(item.strip()) for item in value.split(",") if item.strip()]
        if value
        else [index for index in range(1, dimension_count + 1) if index != dim1_index]
    )
    if not indexes or any(index < 1 or index > dimension_count for index in indexes):
        raise ValueError("--dim2-indexes contains an out-of-range dimension index")
    if dim1_index in indexes:
        raise ValueError("--dim1-index cannot also appear in --dim2-indexes")
    return indexes


def _numeric(value: Any) -> float | None:
    if isinstance(value, (int, float)) and not isinstance(value, bool):
        return float(value)
    if isinstance(value, str):
        text = value.strip().replace(",", "")
        if text.endswith("%"):
            text = text[:-1]
        try:
            return float(text)
        except ValueError:
            return None
    return None


def profile_matrix(
    matrix: list[list[Any]],
    *,
    direction: str,
    dim1_index: int,
    dim2_indexes: str | None,
    detached_headers: list[Any] | None = None,
) -> dict[str, Any]:
    if not matrix:
        raise ValueError("Data range is empty")
    dimension_count = max((len(row) for row in matrix), default=0) if direction == "column" else len(matrix)
    if dim1_index < 1 or dim1_index > dimension_count:
        raise ValueError("--dim1-index is outside the data range")
    selected = _parse_indexes(
        dim2_indexes,
        dimension_count=dimension_count,
        dim1_index=dim1_index,
    )
    detached = detached_headers is not None
    if direction == "column":
        data_rows = matrix if detached else matrix[1:]
        categories = [row[dim1_index - 1] if len(row) >= dim1_index else None for row in data_rows]
        headers = detached_headers or matrix[0]
        series_names = [str(headers[index - 1]) if len(headers) >= index else f"Series {index}" for index in selected]
        first_values = [row[selected[0] - 1] if len(row) >= selected[0] else None for row in data_rows]
    else:
        category_row = matrix[dim1_index - 1]
        categories = category_row if detached else category_row[1:]
        headers = detached_headers or [row[0] if row else None for row in matrix]
        series_names = [str(headers[index - 1]) if len(headers) >= index else f"Series {index}" for index in selected]
        first_row = matrix[selected[0] - 1]
        first_values = first_row if detached else first_row[1:]
    nonempty_categories = [value for value in categories if value not in (None, "")]
    return {
        "categories": nonempty_categories,
        "series_names": series_names,
        "values": [number for value in first_values if (number := _numeric(value)) is not None],
        "dim2_indexes": selected,
    }


def _header_values(matrices: list[list[list[Any]]], direction: str) -> list[Any]:
    matrix = _combine(matrices, direction)
    if direction == "column":
        if len(matrix) != 1:
            raise ValueError("Column-direction --header-range must contain one row")
        return matrix[0]
    if any(len(row) != 1 for row in matrix):
        raise ValueError("Row-direction --header-range must contain one column")
    return [row[0] for row in matrix]


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Recommend chart width and height before +chart-create-basic")
    parser.add_argument("target", help="Spreadsheet URL or spreadsheet token")
    worksheet = parser.add_mutually_exclusive_group()
    worksheet.add_argument("--worksheet-id")
    worksheet.add_argument("--worksheet-name")
    parser.add_argument("--chart-type", required=True)
    parser.add_argument("--data-range", required=True)
    parser.add_argument("--header-range")
    parser.add_argument("--data-direction", choices=("column", "row"), default="column")
    parser.add_argument("--dim1-index", type=int, default=1)
    parser.add_argument("--dim2-indexes")
    parser.add_argument("--data-labels", default="value")
    parser.add_argument("--legend-position", default="bottom")
    parser.add_argument("--title", default="")
    parser.add_argument("--timeout", type=int, default=60)
    args, _ = parser.parse_known_args(argv)
    return args


def main() -> None:
    args = parse_args()
    locator = _locator(args.target)
    try:
        workbook = envelope_data(run_sheets("+workbook-info", **locator, timeout=args.timeout))
        sheets = extract_sheets(workbook)
        matrices = _read_ranges(
            locator,
            sheets,
            args.data_range,
            worksheet_id=args.worksheet_id,
            worksheet_name=args.worksheet_name,
            timeout=args.timeout,
        )
        headers = None
        if args.header_range:
            header_matrices = _read_ranges(
                locator,
                sheets,
                args.header_range,
                worksheet_id=args.worksheet_id,
                worksheet_name=args.worksheet_name,
                timeout=args.timeout,
            )
            headers = _header_values(header_matrices, args.data_direction)
        profile = profile_matrix(
            _combine(matrices, args.data_direction),
            direction=args.data_direction,
            dim1_index=args.dim1_index,
            dim2_indexes=args.dim2_indexes,
            detached_headers=headers,
        )
        result = recommend_chart_size(
            chart_type=args.chart_type,
            categories=profile["categories"],
            series_names=profile["series_names"],
            data_labels=args.data_labels,
            legend_position=args.legend_position,
            title=args.title,
            values=profile["values"],
        )
        result["data_profile"] = {
            "dim2_indexes": profile["dim2_indexes"],
            **result.pop("evidence"),
        }
    except (LarkCliError, KeyError, TypeError, ValueError) as exc:
        emit_error(ACTION, str(exc))
        raise SystemExit(1) from exc
    emit_success(ACTION, result)


if __name__ == "__main__":
    main()
