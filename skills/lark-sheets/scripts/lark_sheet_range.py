# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
"""Range and coordinate helpers for Lark Sheet scripts."""

from __future__ import annotations

import re
from dataclasses import dataclass

CELL_RE = re.compile(r"^\$?([A-Za-z]+)\$?([1-9][0-9]*)$")
ROW_RE = re.compile(r"^\$?([1-9][0-9]*)$")
COLUMN_RE = re.compile(r"^\$?([A-Za-z]+)$")


@dataclass(frozen=True)
class RangeBounds:
    start_row: int
    start_col: int
    end_row: int
    end_col: int

    @property
    def row_count(self) -> int:
        return self.end_row - self.start_row + 1

    @property
    def col_count(self) -> int:
        return self.end_col - self.start_col + 1


def col_to_index(col: str) -> int:
    value = 0
    for char in col.strip().upper():
        if not ("A" <= char <= "Z"):
            raise ValueError(f"Invalid column: {col}")
        value = value * 26 + (ord(char) - ord("A") + 1)
    if value <= 0:
        raise ValueError(f"Invalid column: {col}")
    return value


def index_to_col(index: int) -> str:
    if index < 1:
        raise ValueError(f"Column index must be >= 1: {index}")
    chars = []
    n = index
    while n:
        n, rem = divmod(n - 1, 26)
        chars.append(chr(ord("A") + rem))
    return "".join(reversed(chars))


def parse_cell(cell_ref: str) -> tuple[int, int]:
    match = CELL_RE.match(cell_ref.strip())
    if not match:
        raise ValueError(f"Invalid cell reference: {cell_ref}")
    col, row = match.groups()
    return int(row), col_to_index(col)


def _parse_endpoint(endpoint: str) -> tuple[str, int, int] | tuple[str, int]:
    cell = CELL_RE.match(endpoint)
    if cell:
        col, row = cell.groups()
        return "cell", int(row), col_to_index(col)
    row = ROW_RE.match(endpoint)
    if row:
        return "row", int(row.group(1))
    column = COLUMN_RE.match(endpoint)
    if column:
        return "column", col_to_index(column.group(1))
    raise ValueError(f"Invalid A1 range endpoint: {endpoint}")


def parse_range(
    range_ref: str,
    *,
    max_row: int | None = None,
    max_col: int | None = None,
) -> RangeBounds:
    """Parse the A1 range forms accepted by ``+csv-get``.

    Open-ended forms need the caller's actual returned grid dimensions. This
    keeps generated ranges finite without guessing a spreadsheet-wide limit.
    """
    ref = range_ref.strip()
    if "!" in ref:
        _, ref = ref.rsplit("!", 1)
    if not ref:
        raise ValueError(f"Invalid A1 range: {range_ref}")
    parts = ref.split(":")
    if len(parts) > 2:
        raise ValueError(f"Invalid A1 range: {range_ref}")
    start = _parse_endpoint(parts[0])
    end = _parse_endpoint(parts[-1])

    if len(parts) == 1:
        if start[0] != "cell":
            raise ValueError(f"A1 range must include a cell or ':' separator: {range_ref}")
        _, row, col = start
        return RangeBounds(row, col, row, col)

    if start[0] == end[0] == "cell":
        _, start_row, start_col = start
        _, end_row, end_col = end
    elif start[0] == end[0] == "row":
        _, start_row = start
        _, end_row = end
        start_col = 1
        if max_col is None:
            raise ValueError(f"Range needs a maximum column: {range_ref}")
        end_col = max_col
    elif start[0] == end[0] == "column":
        _, start_col = start
        _, end_col = end
        start_row = 1
        if max_row is None:
            raise ValueError(f"Range needs a maximum row: {range_ref}")
        end_row = max_row
    elif start[0] == "cell" and end[0] == "column":
        _, start_row, start_col = start
        _, end_col = end
        if max_row is None:
            raise ValueError(f"Range needs a maximum row: {range_ref}")
        end_row = max(start_row, max_row)
    elif start[0] == "cell" and end[0] == "row":
        _, start_row, start_col = start
        _, end_row = end
        if max_col is None:
            raise ValueError(f"Range needs a maximum column: {range_ref}")
        end_col = max(start_col, max_col)
    else:
        raise ValueError(f"Invalid A1 range: {range_ref}")
    return RangeBounds(min(start_row, end_row), min(start_col, end_col), max(start_row, end_row), max(start_col, end_col))


def format_cell(row: int, col: int) -> str:
    if row < 1:
        raise ValueError(f"Row must be >= 1: {row}")
    return f"{index_to_col(col)}{row}"


def format_range(start_row: int, start_col: int, end_row: int, end_col: int) -> str:
    bounds = RangeBounds(
        min(start_row, end_row),
        min(start_col, end_col),
        max(start_row, end_row),
        max(start_col, end_col),
    )
    start = format_cell(bounds.start_row, bounds.start_col)
    end = format_cell(bounds.end_row, bounds.end_col)
    return start if start == end else f"{start}:{end}"


def iter_cells(bounds: RangeBounds):
    for row in range(bounds.start_row, bounds.end_row + 1):
        for col in range(bounds.start_col, bounds.end_col + 1):
            yield row, col


def ranges_intersect(a: RangeBounds, b: RangeBounds) -> bool:
    return not (
        a.end_row < b.start_row
        or b.end_row < a.start_row
        or a.end_col < b.start_col
        or b.end_col < a.start_col
    )


def range_union(a: RangeBounds, b: RangeBounds) -> RangeBounds:
    return RangeBounds(
        min(a.start_row, b.start_row),
        min(a.start_col, b.start_col),
        max(a.end_row, b.end_row),
        max(a.end_col, b.end_col),
    )
