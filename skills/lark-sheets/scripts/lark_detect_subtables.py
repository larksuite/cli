#!/usr/bin/env python3
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
"""Detect occupied subtable regions in a Lark sheet."""

from __future__ import annotations

import argparse
import csv
import io
import re
from collections import deque
from dataclasses import dataclass
from typing import Any

from lark_sheet_range import RangeBounds, col_to_index, format_range, index_to_col, parse_range, range_union
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

ACTION = "detect_subtables"
ROW_PREFIX_RE = re.compile(r"^\[row=(\d+)\]\s?(.*)$")
MAX_EXTERNAL_MERGE_ANCHOR_CHECKS = 10


def _inside_quoted_field(lines: list[str]) -> bool:
    """True when the accumulated record has an unterminated quoted field.

    RFC 4180 escapes a literal quote by doubling it, so both halves of a `""`
    pair count and parity still tracks whether a field is left open. A record
    that is still open must swallow the next physical line verbatim — even one
    that looks like a `[row=N]` prefix, because inside quotes that text is
    ordinary cell content, not a new record.
    """
    return sum(line.count('"') for line in lines) % 2 == 1


@dataclass
class CsvGrid:
    row_numbers: list[int]
    col_letters: list[str]
    values: list[list[str]]
    row_numbers_inferred: bool = False


@dataclass
class Component:
    bounds: RangeBounds
    occupied_count: int


def parse_annotated_csv(
    text: str,
    col_indices: list[str] | None = None,
    row_indices: list[int] | None = None,
    source_range: str | None = None,
) -> CsvGrid:
    row_numbers: list[int] = []
    values: list[list[str]] = []
    max_cols = 0
    lines = (text or "").splitlines()
    row_numbers_inferred = False
    has_authoritative_rows = isinstance(row_indices, list) and len(row_indices) > 0

    first_meaningful = next((line for line in lines if line.strip()), "")
    if ROW_PREFIX_RE.match(first_meaningful):
        records = []
        current_lines: list[str] | None = None
        current_row_number: int | None = None
        for line in lines:
            match = ROW_PREFIX_RE.match(line)
            if match and current_lines is not None and _inside_quoted_field(current_lines):
                # Prefix-looking text inside an open quoted field is content.
                current_lines.append(line)
                continue
            if match:
                if current_lines is not None and current_row_number is not None:
                    records.append("\n".join(current_lines))
                    row_numbers.append(current_row_number)
                current_row_number = int(match.group(1))
                current_lines = [match.group(2)]
            elif current_lines is not None:
                current_lines.append(line)
        if current_lines is not None and current_row_number is not None:
            records.append("\n".join(current_lines))
            row_numbers.append(current_row_number)

        # Cross-check against the row numbers the server itself reported.
        # _inside_quoted_field decides whether a "[row=N]" line starts a new
        # record or is content inside an open quoted field; when the payload's
        # quoting is malformed (a lone unescaped quote in a cell), that call
        # goes the wrong way and every following line is swallowed into the
        # previous cell — the rows simply vanish, and everything downstream
        # (data_range, last data row, column profiles) is quietly computed from
        # a short grid. row_indices is authoritative and already in hand, so
        # refuse rather than profile a grid that does not match it.
        if has_authoritative_rows and row_numbers != [int(r) for r in row_indices]:
            raise ValueError(
                "annotated_csv did not parse into the rows the server reported "
                f"(parsed {len(row_numbers)} rows {row_numbers[:5]}…, expected "
                f"{len(row_indices)} rows {list(row_indices)[:5]}…) — most likely "
                "an unbalanced quote in a cell. Re-read a narrower --range, or use "
                "+cells-get for this region instead of the CSV path."
            )

        for record in records:
            parsed = next(csv.reader([record]))
            values.append(parsed)
            max_cols = max(max_cols, len(parsed))
    else:
        reader = csv.reader(io.StringIO(text or ""))
        fallback_start = 1
        if source_range:
            fallback_start = parse_range(
                source_range,
                max_row=1_048_576,
                max_col=18_278,
            ).start_row
        for offset, row in enumerate(reader):
            row_num = None
            if has_authoritative_rows and offset < len(row_indices):
                try:
                    row_num = int(row_indices[offset])
                except (TypeError, ValueError):
                    # Non-numeric row index: leave row_num as None so the
                    # inferred fallback numbering below takes over.
                    pass
            if row_num is None:
                row_numbers_inferred = True
            row_numbers.append(row_num if row_num is not None else fallback_start + offset)
            values.append(row)
            max_cols = max(max_cols, len(row))

    if col_indices:
        col_letters = [str(col) for col in col_indices[:max_cols]]
        while len(col_letters) < max_cols:
            next_col = col_to_index(col_letters[-1]) + 1 if col_letters else len(col_letters) + 1
            col_letters.append(index_to_col(next_col))
    else:
        col_letters = [index_to_col(i) for i in range(1, max_cols + 1)]

    for row in values:
        row.extend([""] * (len(col_letters) - len(row)))
    inferred = bool(values) and not ROW_PREFIX_RE.match(first_meaningful) and (
        row_numbers_inferred or not has_authoritative_rows
    )
    return CsvGrid(
        row_numbers=row_numbers,
        col_letters=col_letters,
        values=values,
        row_numbers_inferred=inferred,
    )


def _merged_ranges(layout: dict[str, Any]) -> list[str]:
    merges = layout.get("merged_cells") or layout.get("merges") or []
    result = []
    for item in merges:
        if isinstance(item, str):
            result.append(item)
        elif isinstance(item, dict):
            value = item.get("range") or item.get("a1_range") or item.get("range_ref")
            if isinstance(value, str):
                result.append(value)
    return result


def _scan_bounds(grid: CsvGrid) -> RangeBounds | None:
    col_numbers = [col_to_index(col) for col in grid.col_letters]
    if grid.row_numbers and grid.col_letters:
        return RangeBounds(
            min(grid.row_numbers),
            min(col_numbers),
            max(grid.row_numbers),
            max(col_numbers),
        )
    return None


def _external_merge_anchors(grid: CsvGrid, layout: dict[str, Any]) -> dict[str, str]:
    scan_bounds = _scan_bounds(grid)
    if scan_bounds is None:
        return {}
    result = {}
    for merge_ref in _merged_ranges(layout):
        try:
            bounds = parse_range(merge_ref)
        except ValueError:
            continue
        intersects = not (
            bounds.end_row < scan_bounds.start_row
            or bounds.start_row > scan_bounds.end_row
            or bounds.end_col < scan_bounds.start_col
            or bounds.start_col > scan_bounds.end_col
        )
        anchor_in_scan = (
            scan_bounds.start_row <= bounds.start_row <= scan_bounds.end_row
            and scan_bounds.start_col <= bounds.start_col <= scan_bounds.end_col
        )
        if intersects and not anchor_in_scan:
            result[merge_ref] = format_range(
                bounds.start_row, bounds.start_col, bounds.start_row, bounds.start_col
            )
    return result


def _has_value(grid: CsvGrid) -> bool:
    return any(value.strip() for row in grid.values for value in row)


def build_occupancy(
    grid: CsvGrid,
    layout: dict[str, Any],
    *,
    confirmed_external_merges: set[str] | None = None,
) -> set[tuple[int, int]]:
    col_numbers = [col_to_index(col) for col in grid.col_letters]
    occupied: set[tuple[int, int]] = set()
    for row_idx, row_num in enumerate(grid.row_numbers):
        for col_idx, value in enumerate(grid.values[row_idx]):
            if value.strip():
                occupied.add((row_num, col_numbers[col_idx]))

    scan_bounds = _scan_bounds(grid)

    for merge_ref in _merged_ranges(layout):
        try:
            bounds = parse_range(merge_ref)
        except ValueError:
            continue
        if scan_bounds is None:
            continue
        anchor_in_scan = (
            scan_bounds.start_row <= bounds.start_row <= scan_bounds.end_row
            and scan_bounds.start_col <= bounds.start_col <= scan_bounds.end_col
        )
        if anchor_in_scan and (bounds.start_row, bounds.start_col) not in occupied:
            continue
        if not anchor_in_scan and merge_ref not in (confirmed_external_merges or set()):
            continue
        sr = max(bounds.start_row, scan_bounds.start_row)
        er = min(bounds.end_row, scan_bounds.end_row)
        sc = max(bounds.start_col, scan_bounds.start_col)
        ec = min(bounds.end_col, scan_bounds.end_col)
        if sr > er or sc > ec:
            continue
        for row in range(sr, er + 1):
            for col in range(sc, ec + 1):
                occupied.add((row, col))
    return occupied


def _raw_components(
    occupied: set[tuple[int, int]],
    *,
    adjacent_rows: dict[int, set[int]] | None = None,
    adjacent_cols: dict[int, set[int]] | None = None,
) -> list[Component]:
    remaining = set(occupied)
    components = []
    while remaining:
        start = remaining.pop()
        queue = deque([start])
        cells = [start]
        while queue:
            row, col = queue.popleft()
            vertical_rows = adjacent_rows.get(row, set()) if adjacent_rows else {row - 1, row + 1}
            neighbors = [(neighbor_row, col) for neighbor_row in vertical_rows]
            # Columns get the same treatment as rows: with --skip-hidden the
            # returned columns can be non-consecutive (A, C when B is hidden),
            # so col±1 would split visually adjacent data into two components.
            horizontal_cols = adjacent_cols.get(col, set()) if adjacent_cols else {col - 1, col + 1}
            neighbors.extend((row, neighbor_col) for neighbor_col in horizontal_cols)
            for neighbor in neighbors:
                if neighbor in remaining:
                    remaining.remove(neighbor)
                    queue.append(neighbor)
                    cells.append(neighbor)
        rows = [cell[0] for cell in cells]
        cols = [cell[1] for cell in cells]
        components.append(
            Component(
                RangeBounds(min(rows), min(cols), max(rows), max(cols)),
                len(cells),
            )
        )
    return components


def _box_gap_mergeable(a: RangeBounds, b: RangeBounds, gap_rows: int, gap_cols: int) -> bool:
    cols_overlap = not (a.end_col < b.start_col or b.end_col < a.start_col)
    rows_overlap = not (a.end_row < b.start_row or b.end_row < a.start_row)
    vertical_gap = max(b.start_row - a.end_row - 1, a.start_row - b.end_row - 1, 0)
    horizontal_gap = max(b.start_col - a.end_col - 1, a.start_col - b.end_col - 1, 0)
    return (cols_overlap and vertical_gap <= gap_rows) or (
        rows_overlap and horizontal_gap <= gap_cols
    )


def merge_components(
    components: list[Component], *, gap_rows: int = 1, gap_cols: int = 0
) -> list[Component]:
    merged = components[:]
    changed = True
    while changed:
        changed = False
        next_components: list[Component] = []
        used = [False] * len(merged)
        for i, comp in enumerate(merged):
            if used[i]:
                continue
            current = Component(comp.bounds, comp.occupied_count)
            used[i] = True
            for j in range(i + 1, len(merged)):
                if used[j]:
                    continue
                other = merged[j]
                if _box_gap_mergeable(current.bounds, other.bounds, gap_rows, gap_cols):
                    current = Component(
                        range_union(current.bounds, other.bounds),
                        current.occupied_count + other.occupied_count,
                    )
                    used[j] = True
                    changed = True
            next_components.append(current)
        merged = next_components
    return merged


def _row_values(grid: CsvGrid, bounds: RangeBounds, row_num: int) -> list[str]:
    if row_num not in grid.row_numbers:
        return []
    row = grid.values[grid.row_numbers.index(row_num)]
    values = []
    for col_idx, col_letter in enumerate(grid.col_letters):
        col_num = col_to_index(col_letter)
        if bounds.start_col <= col_num <= bounds.end_col:
            values.append(row[col_idx] if col_idx < len(row) else "")
    return values


def header_candidates(grid: CsvGrid, bounds: RangeBounds) -> list[int]:
    candidates = []
    for row in range(bounds.start_row, min(bounds.end_row, bounds.start_row + 4) + 1):
        values = _row_values(grid, bounds, row)
        non_empty = [value for value in values if value.strip()]
        if len(non_empty) >= max(1, min(2, bounds.col_count)):
            candidates.append(row)
    return candidates


def kind_guess(bounds: RangeBounds, density: float) -> str:
    if bounds.row_count >= 3 and bounds.col_count >= 2 and density >= 0.25:
        return "data_table"
    if bounds.row_count <= 2 or bounds.col_count <= 1:
        return "note_or_label"
    if density < 0.25:
        return "sparse_block"
    return "summary_block"


def summarize_components(grid: CsvGrid, components: list[Component], min_cells: int) -> list[dict[str, Any]]:
    result = []
    for idx, comp in enumerate(
        sorted(components, key=lambda item: (item.bounds.start_row, item.bounds.start_col)),
        start=1,
    ):
        if comp.occupied_count < min_cells:
            continue
        area = comp.bounds.row_count * comp.bounds.col_count
        density = comp.occupied_count / area if area else 0
        samples = []
        for row in range(comp.bounds.start_row, min(comp.bounds.end_row, comp.bounds.start_row + 2) + 1):
            samples.append(_row_values(grid, comp.bounds, row))
        result.append(
            {
                "id": f"T{idx}",
                "range": format_range(
                    comp.bounds.start_row,
                    comp.bounds.start_col,
                    comp.bounds.end_row,
                    comp.bounds.end_col,
                ),
                "rows": comp.bounds.row_count,
                "cols": comp.bounds.col_count,
                "occupied_cells": comp.occupied_count,
                "density": round(density, 4),
                "header_candidates": header_candidates(grid, comp.bounds),
                "kind_guess": kind_guess(comp.bounds, density),
                "sample": samples,
            }
        )
    return result


def detect_subtables(args) -> tuple[dict[str, Any], list[str]]:
    warnings: list[str] = []
    workbook = envelope_data(
        run_sheets(
            "+workbook-info",
            url=args.url,
            spreadsheet_token=args.spreadsheet_token,
            timeout=args.timeout,
        )
    )
    sheet = resolve_target_sheets(
        workbook,
        sheet_id=args.sheet_id,
        sheet_name=args.sheet_name,
        require_one=True,
    )[0]
    sid = sheet_identifier(sheet)
    title = sheet_title(sheet)
    locator = sheet_locator(sheet)

    col_count = min(int(sheet.get("column_count") or args.max_scan_cols), args.max_scan_cols)
    row_count = min(int(sheet.get("row_count") or args.max_scan_rows), args.max_scan_rows)
    scan_range = args.range or f"A1:{index_to_col(max(1, col_count))}{max(1, row_count)}"
    if not args.range:
        if int(sheet.get("column_count") or 0) > args.max_scan_cols:
            warnings.append(f"scan clipped to first {args.max_scan_cols} columns")
        if int(sheet.get("row_count") or 0) > args.max_scan_rows:
            warnings.append(f"scan clipped to first {args.max_scan_rows} rows")

    layout = envelope_data(
        run_sheets(
            "+sheet-info",
            url=args.url,
            spreadsheet_token=args.spreadsheet_token,
            **locator,
            flags={"include": "merges,hidden_rows,hidden_cols"},
            timeout=args.timeout,
        )
    )
    csv_data = envelope_data(
        run_sheets(
            "+csv-get",
            url=args.url,
            spreadsheet_token=args.spreadsheet_token,
            **locator,
            flags={
                "range": scan_range,
                "max_chars": args.max_chars,
                "skip_hidden": True if args.skip_hidden else None,
            },
            timeout=args.timeout,
        )
    )
    actual_range = str(csv_data.get("actual_range") or scan_range)
    if csv_data.get("has_more"):
        raise LarkCliError(
            f"+csv-get truncated the scan range at {actual_range}; narrow --range before detecting subtables"
        )

    grid = parse_annotated_csv(
        csv_data.get("annotated_csv", ""),
        csv_data.get("col_indices"),
        csv_data.get("row_indices"),
        actual_range,
    )
    if grid.row_numbers_inferred:
        warnings.append("CSV row numbers were inferred from the requested range")
    hidden_rows_raw = layout.get("hidden_rows") or []
    hidden_row_indexes = {
        int(value) + 1
        for value in hidden_rows_raw
        if isinstance(value, (int, str)) and str(value).isdigit()
    }
    hidden_columns_raw = layout.get("hidden_cols") or layout.get("hidden_columns") or []
    hidden_col_letters = set()
    for value in hidden_columns_raw if isinstance(hidden_columns_raw, list) else []:
        if isinstance(value, str) and value.isalpha():
            hidden_col_letters.add(value.upper())
        elif isinstance(value, (int, str)) and str(value).isdigit():
            hidden_col_letters.add(index_to_col(int(value) + 1))
    hidden_rows = sorted(row for row in grid.row_numbers if row in hidden_row_indexes)
    hidden_columns = [col for col in grid.col_letters if col.upper() in hidden_col_letters]
    if hidden_rows or hidden_columns:
        warnings.append("scan includes hidden rows or columns; pass --skip-hidden to exclude them")
    confirmed_external_merges = set()
    external_merge_anchors = list(_external_merge_anchors(grid, layout).items())
    if len(external_merge_anchors) > MAX_EXTERNAL_MERGE_ANCHOR_CHECKS:
        warnings.append(
            f"skipped confirmation for {len(external_merge_anchors) - MAX_EXTERNAL_MERGE_ANCHOR_CHECKS} "
            f"external merge anchors (limit: {MAX_EXTERNAL_MERGE_ANCHOR_CHECKS})"
        )
    for merge_ref, anchor in external_merge_anchors[:MAX_EXTERNAL_MERGE_ANCHOR_CHECKS]:
        try:
            anchor_data = envelope_data(
                run_sheets(
                    "+csv-get",
                    url=args.url,
                    spreadsheet_token=args.spreadsheet_token,
                    **locator,
                    flags={
                        "range": anchor,
                        "max_chars": 1024,
                        "skip_hidden": True if args.skip_hidden else None,
                    },
                    timeout=args.timeout,
                )
            )
            anchor_grid = parse_annotated_csv(
                anchor_data.get("annotated_csv", ""),
                anchor_data.get("col_indices"),
                anchor_data.get("row_indices"),
                anchor,
            )
            if _has_value(anchor_grid):
                confirmed_external_merges.add(merge_ref)
        except (LarkCliError, ValueError) as exc:
            warnings.append(f"could not confirm merge anchor {anchor}: {exc}")
    occupied = build_occupancy(
        grid,
        layout,
        confirmed_external_merges=confirmed_external_merges,
    )
    adjacent_rows = None
    adjacent_cols = None
    if args.skip_hidden:
        adjacent_rows = {}
        for previous, current in zip(grid.row_numbers, grid.row_numbers[1:]):
            adjacent_rows.setdefault(previous, set()).add(current)
            adjacent_rows.setdefault(current, set()).add(previous)
        col_numbers = [col_to_index(col) for col in grid.col_letters]
        adjacent_cols = {}
        for previous, current in zip(col_numbers, col_numbers[1:]):
            adjacent_cols.setdefault(previous, set()).add(current)
            adjacent_cols.setdefault(current, set()).add(previous)
    raw_components = _raw_components(
        occupied,
        adjacent_rows=adjacent_rows,
        adjacent_cols=adjacent_cols,
    )
    if len(raw_components) > args.max_merge_components:
        warnings.append(
            f"skipped gap-based component merging for {len(raw_components)} components "
            f"(limit: {args.max_merge_components})"
        )
        components = raw_components
    else:
        components = merge_components(
            raw_components,
            gap_rows=args.gap_rows,
            gap_cols=args.gap_cols,
        )
    subtables = summarize_components(grid, components, args.min_cells)
    return {
        "sheet_id": sid,
        "sheet": title,
        "scan_range": scan_range,
        "actual_range": actual_range,
        "visibility": {
            "skip_hidden": args.skip_hidden,
            "hidden_rows_in_range": hidden_rows,
            "hidden_columns_in_range": hidden_columns,
        },
        "subtables": subtables,
    }, warnings


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    add_spreadsheet_args(parser, require_sheet=True, allow_sheet=True)
    parser.add_argument("--range")
    parser.add_argument("--max-scan-rows", type=int, default=5000)
    parser.add_argument("--max-scan-cols", type=int, default=200)
    parser.add_argument("--gap-rows", type=int, default=1)
    parser.add_argument("--gap-cols", type=int, default=0)
    parser.add_argument("--min-cells", type=int, default=2)
    parser.add_argument("--max-merge-components", type=int, default=2000)
    parser.add_argument("--max-chars", type=int, default=25000)
    parser.add_argument("--skip-hidden", action="store_true")
    parser.add_argument("--timeout", type=int, default=60)
    args = parser.parse_args()
    try:
        data, warnings = detect_subtables(args)
    except (LarkCliError, ValueError, TypeError) as exc:
        emit_error(ACTION, str(exc))
    emit_success(ACTION, data, warnings)


if __name__ == "__main__":
    main()
