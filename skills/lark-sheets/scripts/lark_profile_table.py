#!/usr/bin/env python3
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
"""Profile a candidate table range in a Lark sheet."""

from __future__ import annotations

import argparse
import re
from typing import Any

from lark_sheet_range import col_to_index, format_range, index_to_col, parse_range
from lark_sheet_read_cli import (
    LarkCliError,
    add_spreadsheet_args,
    emit_error,
    emit_success,
    envelope_data,
    run_sheets,
)
from lark_detect_subtables import CsvGrid, parse_annotated_csv

ACTION = "profile_table"
ERROR_VALUES = ("#VALUE!", "#DIV/0!", "#REF!", "#NAME?", "#NULL!", "#NUM!", "#N/A")
TOTAL_KEYWORDS = ("合计", "总计", "小计", "汇总", "累计")
TOTAL_EN_RE = re.compile(r"\b(?:grand total|subtotal|total)\b", re.IGNORECASE)
SIGNATURE_KEYWORDS = ("编制人", "审核人", "审批人", "负责人", "经理", "签名")


def _is_number(value: str) -> bool:
    text = value.strip().replace(",", "")
    if text.endswith("%"):
        text = text[:-1]
    try:
        float(text)
        return True
    except ValueError:
        return False


def _is_date_like(value: str) -> bool:
    text = value.strip()
    if re.match(r"^\d{4}[-/]\d{1,2}[-/]\d{1,2}(?:\s+\d{1,2}:\d{2}(:\d{2})?)?$", text):
        return True
    if re.match(r"^\d{8}$", text):
        year = int(text[:4])
        month = int(text[4:6])
        day = int(text[6:8])
        return 1900 <= year <= 2100 and 1 <= month <= 12 and 1 <= day <= 31
    if re.match(r"^\d{6}$", text):
        year = int(text[:4])
        month = int(text[4:6])
        return 1900 <= year <= 2100 and 1 <= month <= 12
    return False


def _classify_value(value: str) -> str:
    text = value.strip()
    if not text:
        return "empty"
    if any(err in text for err in ERROR_VALUES):
        return "error"
    if re.match(r"^0\d+$", text) or re.match(r"^\d{11,}$", text):
        return "string_id"
    if _is_date_like(text):
        return "date"
    if _is_number(text):
        return "number"
    if text.lower() in {"true", "false", "yes", "no"}:
        return "bool"
    return "string"


def _row_values(grid: CsvGrid, row_index: int) -> list[str]:
    return grid.values[row_index]


def _header_score(row: list[str]) -> tuple[int, int]:
    non_empty = [value for value in row if value.strip()]
    if not non_empty:
        return 0, 0
    strings = sum(1 for value in non_empty if _classify_value(value) in {"string", "string_id"})
    data_like = sum(
        1 for value in non_empty if _classify_value(value) in {"number", "date", "bool"}
    )
    return strings * 2 + len(non_empty) - data_like * 3, len(non_empty)


def _header_candidate_score(grid: CsvGrid, index: int) -> int:
    score, _ = _header_score(grid.values[index])
    following = grid.values[index + 1 : index + 4]
    following_non_empty = [value for row in following for value in row if value.strip()]
    following_data_like = sum(
        1 for value in following_non_empty if _classify_value(value) in {"number", "date", "bool"}
    )
    # A schema row is usually followed by data-shaped values; use this as a
    # small tie-breaker while preferring earlier candidates with equal evidence.
    score += min(3, following_data_like)
    return score - index // 2


def detect_header_row(grid: CsvGrid, scan_rows: int = 20) -> int | None:
    best_row = None
    best_score = None
    for idx, row in enumerate(grid.values[:scan_rows]):
        _, non_empty_count = _header_score(row)
        if not non_empty_count:
            continue
        score = _header_candidate_score(grid, idx)
        if best_score is None or score > best_score:
            best_score = score
            best_row = grid.row_numbers[idx]
    return best_row


def _possible_multi_row_header(grid: CsvGrid, header_row: int | None) -> bool:
    if header_row is None or header_row not in grid.row_numbers:
        return False
    header_index = grid.row_numbers.index(header_row)
    if header_index + 1 >= len(grid.values):
        return False
    header_score, header_non_empty = _header_score(grid.values[header_index])
    next_score, next_non_empty = _header_score(grid.values[header_index + 1])
    if header_non_empty < 2 or next_non_empty < 2:
        return False
    return next_score >= max(4, (header_score * 3 + 3) // 4)


def _header_row_not_first(grid: CsvGrid, header_row: int | None) -> bool:
    if header_row is None:
        return False
    first_non_empty = next(
        (
            row_num
            for row_num, row in zip(grid.row_numbers, grid.values)
            if any(value.strip() for value in row)
        ),
        None,
    )
    return first_non_empty is not None and first_non_empty != header_row


def detect_special_rows(grid: CsvGrid, header_row: int | None) -> list[dict[str, Any]]:
    specials = []
    for idx, row in enumerate(grid.values):
        row_num = grid.row_numbers[idx]
        if header_row is not None and row_num <= header_row:
            continue
        joined = " ".join(value.strip() for value in row if value.strip())
        if not joined:
            specials.append({"row": row_num, "reason": "empty_row", "sample": ""})
            continue
        if any(keyword in joined for keyword in TOTAL_KEYWORDS) or TOTAL_EN_RE.search(joined):
            specials.append({"row": row_num, "reason": "total_row", "sample": joined[:120]})
        elif any(keyword in joined for keyword in SIGNATURE_KEYWORDS):
            specials.append({"row": row_num, "reason": "signature_row", "sample": joined[:120]})
    return specials


def _last_data_row(grid: CsvGrid, header_row: int | None, specials: list[dict[str, Any]]) -> int | None:
    special_rows = {item["row"] for item in specials if item["reason"] != "empty_row"}
    for idx in range(len(grid.row_numbers) - 1, -1, -1):
        row_num = grid.row_numbers[idx]
        if header_row is not None and row_num <= header_row:
            continue
        if row_num in special_rows:
            continue
        if any(value.strip() for value in grid.values[idx]):
            return row_num
    return None


def _column_values(grid: CsvGrid, col_index: int, start_row: int, end_row: int | None) -> list[str]:
    values = []
    for idx, row_num in enumerate(grid.row_numbers):
        if row_num < start_row:
            continue
        if end_row is not None and row_num > end_row:
            continue
        row = grid.values[idx]
        values.append(row[col_index] if col_index < len(row) else "")
    return values


def _type_guess(type_counts: dict[str, int]) -> str:
    non_empty_counts = {key: value for key, value in type_counts.items() if key != "empty" and value}
    if not non_empty_counts:
        return "empty"
    if len(non_empty_counts) == 1:
        return next(iter(non_empty_counts))
    if "error" in non_empty_counts:
        return "mixed_with_errors"
    if "string_id" in non_empty_counts and set(non_empty_counts) <= {"string_id", "string"}:
        return "string_id"
    return "mixed"


def profile_columns(
    grid: CsvGrid,
    *,
    header_row: int | None,
    data_start_row: int,
    data_end_row: int | None,
) -> list[dict[str, Any]]:
    header_values = []
    if header_row in grid.row_numbers:
        header_values = _row_values(grid, grid.row_numbers.index(header_row))

    profiles = []
    for idx, col in enumerate(grid.col_letters):
        name = ""
        if idx < len(header_values):
            name = header_values[idx].strip()
        if not name:
            name = f"unnamed_{col}"

        values = _column_values(grid, idx, data_start_row, data_end_row)
        type_counts: dict[str, int] = {}
        examples = []
        error_count = 0
        for value in values:
            value_type = _classify_value(value)
            type_counts[value_type] = type_counts.get(value_type, 0) + 1
            if value_type == "error":
                error_count += 1
            if value.strip() and len(examples) < 5:
                examples.append(value)
        non_empty = len(values) - type_counts.get("empty", 0)
        warnings = []
        if type_counts.get("string_id"):
            warnings.append("long_numeric_like_id")
        if error_count:
            warnings.append("formula_or_value_errors")
        if non_empty and type_counts.get("empty", 0) / len(values) > 0.5:
            warnings.append("many_empty_cells")
        if _type_guess(type_counts) == "mixed":
            warnings.append("mixed_value_types")

        profiles.append(
            {
                "name": name,
                "col": col,
                "non_empty": non_empty,
                "empty": type_counts.get("empty", 0),
                "type_guess": _type_guess(type_counts),
                "type_distribution": type_counts,
                "examples": examples,
                "warnings": warnings,
            }
        )
    return profiles


def _field_map(columns: list[dict[str, Any]]) -> dict[str, list[dict[str, Any]]]:
    result: dict[str, list[dict[str, Any]]] = {}
    for column in columns:
        name = column["name"]
        result.setdefault(name, []).append(
            {
                "col": column["col"],
                "type_guess": column["type_guess"],
                "warnings": column["warnings"],
            }
        )
    return result


def _risk_warnings(
    *,
    columns: list[dict[str, Any]],
    special_rows: list[dict[str, Any]],
    header_row: int | None,
    data_range: str | None,
    possible_multi_row_header: bool,
    header_row_not_first: bool,
    data_range_has_gaps: bool,
    data_range_has_col_gaps: bool,
    hidden_rows: list[int],
    hidden_columns: list[str],
) -> list[str]:
    warnings = {warning for column in columns for warning in column["warnings"]}
    names = [column["name"] for column in columns]
    if len(names) != len(set(names)):
        warnings.add("duplicate_headers")
    if any(name.startswith("unnamed_") for name in names):
        warnings.add("unnamed_columns")
    if header_row is None:
        warnings.add("header_not_detected")
    if data_range is None:
        warnings.add("data_range_not_detected")
    if any(item["reason"] != "empty_row" for item in special_rows):
        warnings.add("special_rows_present")
    if any(item["reason"] == "empty_row" for item in special_rows):
        warnings.add("empty_rows_present")
    if possible_multi_row_header:
        warnings.add("possible_multi_row_header")
    if header_row_not_first:
        warnings.add("header_row_not_first")
    if data_range_has_gaps:
        warnings.add("data_range_has_gaps")
    if data_range_has_col_gaps:
        warnings.add("data_range_has_col_gaps")
    if hidden_rows:
        warnings.add("hidden_rows_in_range")
    if hidden_columns:
        warnings.add("hidden_columns_in_range")
    return sorted(warnings)


def _col_segments(col_letters: list[str]) -> list[list[str]]:
    """Group column letters into contiguous runs of real column indices.

    With --skip-hidden the returned columns can be non-consecutive (A, C when B
    is hidden). Reporting a single data_range across that gap would let a caller
    write the hidden-column-free data back as if it were contiguous, shifting
    every value right of the gap.
    """
    if not col_letters:
        return []
    indices = [col_to_index(col) for col in col_letters]
    segments = []
    start = previous = indices[0]
    for current in indices[1:]:
        if current == previous + 1:
            previous = current
            continue
        segments.append([index_to_col(start), index_to_col(previous)])
        start = previous = current
    segments.append([index_to_col(start), index_to_col(previous)])
    return segments


def _row_segments(row_numbers: list[int]) -> list[list[int]]:
    if not row_numbers:
        return []
    segments = []
    start = end = row_numbers[0]
    for row in row_numbers[1:]:
        if row == end + 1:
            end = row
            continue
        segments.append([start, end])
        start = end = row
    segments.append([start, end])
    return segments


def _write_hints(
    grid: CsvGrid,
    *,
    header_row: int | None,
    data_start: int,
    bounds,
    hidden_columns: list[str] | None = None,
    skip_hidden: bool = False,
) -> dict[str, Any]:
    last_non_empty_col = None
    for idx, col in enumerate(grid.col_letters):
        if any(idx < len(row) and row[idx].strip() for row in grid.values):
            last_non_empty_col = col_to_index(col)
    safe_col_num = (last_non_empty_col + 1) if last_non_empty_col else bounds.start_col
    # Step over hidden columns. Under --skip-hidden they are absent from the
    # grid entirely, so "one past the last visible column" can land ON a hidden
    # column that holds data — and when the hidden columns sit at the right edge
    # there is no gap in the returned letters either, so data_range_has_col_gaps
    # stays silent too. Appending there would overwrite data nobody can see.
    hidden_indices = {col_to_index(col) for col in (hidden_columns or [])}
    skipped_hidden: list[str] = []
    while safe_col_num in hidden_indices:
        skipped_hidden.append(index_to_col(safe_col_num))
        safe_col_num += 1
    safe_col = index_to_col(safe_col_num)
    hints = {
        "last_non_empty_col": index_to_col(last_non_empty_col) if last_non_empty_col else None,
    }
    if not (skip_hidden and hidden_columns is None):
        hints.update(
            {
                "safe_append_col": safe_col,
                "safe_append_header_cell": f"{safe_col}{header_row}" if header_row else None,
                "safe_append_data_start_cell": f"{safe_col}{data_start}",
            }
        )
    if skipped_hidden:
        hints["skipped_hidden_cols"] = skipped_hidden
    return hints


def profile_grid(
    grid: CsvGrid,
    source_range: str,
    *,
    skip_hidden: bool = False,
    hidden_rows: list[int] | None = None,
    hidden_columns: list[str] | None = None,
    all_hidden_columns: list[str] | None = None,
    header_scan_rows: int = 20,
) -> dict[str, Any]:
    max_row = max(grid.row_numbers, default=1)
    max_col = max((col_to_index(col) for col in grid.col_letters), default=1)
    bounds = parse_range(source_range, max_row=max_row, max_col=max_col)
    header_row = detect_header_row(grid, header_scan_rows)
    data_start = (header_row + 1) if header_row else bounds.start_row
    specials = detect_special_rows(grid, header_row)
    data_end = _last_data_row(grid, header_row, specials)
    data_range = None
    if data_end is not None and data_end >= data_start:
        data_range = format_range(data_start, bounds.start_col, data_end, bounds.end_col)
    columns = profile_columns(
        grid,
        header_row=header_row,
        data_start_row=data_start,
        data_end_row=data_end,
    )
    data_row_numbers = []
    if data_end is not None and data_end >= data_start:
        data_row_numbers = [
            row for row in grid.row_numbers if data_start <= row <= data_end
        ]
    data_row_segments = _row_segments(data_row_numbers)
    data_range_has_gaps = bool(data_row_numbers) and (
        data_row_numbers[0] != data_start or len(data_row_segments) > 1
    )
    data_rows = len(data_row_numbers)
    data_col_segments = _col_segments(grid.col_letters)
    data_range_has_col_gaps = len(data_col_segments) > 1
    hidden_rows = hidden_rows or []
    hidden_columns = hidden_columns or []
    possible_multi_row_header = _possible_multi_row_header(grid, header_row)
    header_row_not_first = _header_row_not_first(grid, header_row)

    return {
        "summary": {
            "range": source_range,
            "header_row": header_row,
            "data_range": data_range,
            "data_rows": data_rows,
            "data_row_segments": data_row_segments,
            "data_col_segments": data_col_segments,
            "column_count": len(columns),
            "special_rows_count": len(specials),
        },
        "range": source_range,
        "header_row": header_row,
        "data_range": data_range,
        "data_row_segments": data_row_segments,
        "data_col_segments": data_col_segments,
        "columns": columns,
        "field_map": _field_map(columns),
        "risk_warnings": _risk_warnings(
            columns=columns,
            special_rows=specials,
            header_row=header_row,
            data_range=data_range,
            possible_multi_row_header=possible_multi_row_header,
            header_row_not_first=header_row_not_first,
            data_range_has_gaps=data_range_has_gaps,
            data_range_has_col_gaps=data_range_has_col_gaps,
            hidden_rows=hidden_rows,
            hidden_columns=hidden_columns,
        ),
        "visibility": {
            "skip_hidden": skip_hidden,
            "hidden_rows_in_range": hidden_rows,
            "hidden_columns_in_range": hidden_columns,
        },
        "write_hints": _write_hints(
            grid,
            header_row=header_row,
            data_start=data_start,
            bounds=bounds,
            hidden_columns=all_hidden_columns,
            skip_hidden=skip_hidden,
        ),
        "special_rows": specials,
    }


def _hidden_rows_and_columns(grid: CsvGrid, layout: dict[str, Any]) -> tuple[list[int], list[str]]:
    def indexes(key: str) -> set[int]:
        values = layout.get(key)
        if not isinstance(values, list):
            return set()
        result = set()
        for value in values:
            try:
                result.add(int(value) + 1)  # +sheet-info uses zero-based indices.
            except (TypeError, ValueError):
                continue
        return result

    hidden_row_indexes = indexes("hidden_rows")
    hidden_columns = layout.get("hidden_cols") or layout.get("hidden_columns") or []
    hidden_column_letters = set()
    for value in hidden_columns if isinstance(hidden_columns, list) else []:
        if isinstance(value, str) and value.isalpha():
            hidden_column_letters.add(value.upper())
        else:
            try:
                hidden_column_letters.add(index_to_col(int(value) + 1))
            except (TypeError, ValueError):
                continue
    rows = sorted(row for row in grid.row_numbers if row in hidden_row_indexes)
    columns = [col for col in grid.col_letters if col.upper() in hidden_column_letters]
    return rows, columns


def _all_hidden_columns(layout: dict[str, Any]) -> list[str]:
    """Every hidden column on the sheet, not just those inside the grid.

    _hidden_rows_and_columns intersects with the returned grid, which is the
    right scope for the "there are hidden rows/columns in what you read"
    warnings. The append hint needs the opposite: the columns that are NOT in
    the grid precisely because they are hidden.
    """
    letters: list[str] = []
    raw = layout.get("hidden_cols") or layout.get("hidden_columns") or []
    for value in raw if isinstance(raw, list) else []:
        if isinstance(value, str) and value.isalpha():
            letters.append(value.upper())
            continue
        try:
            letters.append(index_to_col(int(value) + 1))
        except (TypeError, ValueError):
            continue
    return letters


def profile_table(args) -> tuple[dict[str, Any], list[str]]:
    warnings = []
    if args.header_scan_rows < 1:
        raise ValueError("--header-scan-rows must be at least 1")
    csv_data = envelope_data(
        run_sheets(
            "+csv-get",
            url=args.url,
            spreadsheet_token=args.spreadsheet_token,
            sheet_id=args.sheet_id,
            sheet_name=args.sheet_name,
            flags={
                "range": args.range,
                "max_chars": args.max_chars,
                "skip_hidden": True if args.skip_hidden else None,
            },
            timeout=args.timeout,
        )
    )
    source_range = str(csv_data.get("actual_range") or args.range)
    if csv_data.get("has_more"):
        raise LarkCliError(
            f"+csv-get truncated the requested range at {source_range}; narrow --range before profiling"
        )
    grid = parse_annotated_csv(
        csv_data.get("annotated_csv", ""),
        csv_data.get("col_indices"),
        csv_data.get("row_indices"),
        source_range,
    )
    if grid.row_numbers_inferred:
        warnings.append("CSV row numbers were inferred from the requested range")
    hidden_rows: list[int] = []
    hidden_columns: list[str] = []
    all_hidden_columns: list[str] | None = None
    # Fetched in BOTH modes. Under --skip-hidden the hidden rows/columns are
    # absent from the grid, so hidden_rows/hidden_columns (which are scoped to
    # what the grid contains) come back empty and no warning fires — but the
    # write hints still need to know where the hidden columns are, or
    # safe_append_col can point at one that holds data. all_hidden_columns is
    # the unscoped list used for exactly that.
    try:
        layout = envelope_data(
            run_sheets(
                "+sheet-info",
                url=args.url,
                spreadsheet_token=args.spreadsheet_token,
                sheet_id=args.sheet_id,
                sheet_name=args.sheet_name,
                flags={"include": "hidden_rows,hidden_cols"},
                timeout=args.timeout,
            )
        )
        hidden_rows, hidden_columns = _hidden_rows_and_columns(grid, layout)
        all_hidden_columns = _all_hidden_columns(layout)
    except LarkCliError as exc:
        warnings.append(f"hidden row/column detection unavailable: {exc}")
    return profile_grid(
        grid,
        source_range,
        skip_hidden=args.skip_hidden,
        hidden_rows=hidden_rows,
        hidden_columns=hidden_columns,
        all_hidden_columns=all_hidden_columns,
        header_scan_rows=args.header_scan_rows,
    ), warnings


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    add_spreadsheet_args(parser, require_sheet=True, allow_sheet=True)
    parser.add_argument("--range", required=True)
    parser.add_argument("--max-chars", type=int, default=25000)
    parser.add_argument("--header-scan-rows", type=int, default=20)
    parser.add_argument("--skip-hidden", action="store_true")
    parser.add_argument("--timeout", type=int, default=60)
    args = parser.parse_args()
    try:
        data, warnings = profile_table(args)
    except (LarkCliError, ValueError, TypeError) as exc:
        emit_error(ACTION, str(exc))
    emit_success(ACTION, data, warnings)


if __name__ == "__main__":
    main()
