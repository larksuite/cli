#!/usr/bin/env python3
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
"""Check Lark Sheet chart quality and prepare rendered thumbnails for review.

The single required argument is a spreadsheet URL or spreadsheet token. By
default every worksheet is checked; pass --worksheet-id to restrict the check
to one worksheet reference_id.

Numeric source checks sample at most 50 data points per series and request at
most 2000 source cells per chart, including headers and gaps between series.
Sampled zero/constant values do not establish a whole-series issue.

Exit codes:
  0: static checks passed and every thumbnail asset is valid; model review remains required
  1: the check could not be completed or a thumbnail asset is unavailable
  2: check completed and at least one chart-quality issue was found
"""

from __future__ import annotations

import argparse
import base64
import binascii
import io
import json
import re
import struct
import tempfile
import zlib
from pathlib import Path
from typing import Any

from lark_sheet_read_cli import (
    LarkCliError,
    envelope_data,
    resolve_target_sheets,
    run_sheets,
    sheet_identifier,
    sheet_title,
)
from lark_chart_size_rules import MAX_ASPECT_RATIO, MAX_CHART_WIDTH, minimum_chart_size

ACTION = "chart_quality_check"
DEFAULT_COLUMN_WIDTH = 105.0
DEFAULT_ROW_HEIGHT = 27.0
MAX_CELL_READ_SIZE = 2_000
MAX_SOURCE_SAMPLE_POINTS = 50
PNG_SIGNATURE = b"\x89PNG\r\n\x1a\n"
JPEG_SIGNATURE = b"\xff\xd8\xff"
MIN_NON_WHITE_RATIO = 0.001
MIN_CONTINUOUS_AXIS_DATA_UTILIZATION = 0.25


CellBounds = tuple[int, int, int, int]
CellCache = dict[tuple[str, str, str, bool], dict[str, Any]]
SeriesProfile = dict[str, Any]


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


def _paeth(left: int, up: int, upper_left: int) -> int:
    estimate = left + up - upper_left
    left_distance = abs(estimate - left)
    up_distance = abs(estimate - up)
    upper_left_distance = abs(estimate - upper_left)
    if left_distance <= up_distance and left_distance <= upper_left_distance:
        return left
    return up if up_distance <= upper_left_distance else upper_left


def _png_pixel_profile(raw: bytes) -> dict[str, Any]:
    if not raw.startswith(PNG_SIGNATURE):
        raise ValueError("invalid PNG signature")
    offset = len(PNG_SIGNATURE)
    width = height = bit_depth = color_type = interlace = None
    palette: list[tuple[int, int, int]] = []
    transparency = b""
    compressed = bytearray()
    while offset + 12 <= len(raw):
        length = struct.unpack(">I", raw[offset : offset + 4])[0]
        chunk_type = raw[offset + 4 : offset + 8]
        payload = raw[offset + 8 : offset + 8 + length]
        offset += 12 + length
        if chunk_type == b"IHDR":
            width, height, bit_depth, color_type, _, _, interlace = struct.unpack(">IIBBBBB", payload)
        elif chunk_type == b"PLTE":
            palette = [tuple(payload[index : index + 3]) for index in range(0, len(payload), 3)]
        elif chunk_type == b"tRNS":
            transparency = payload
        elif chunk_type == b"IDAT":
            compressed.extend(payload)
        elif chunk_type == b"IEND":
            break
    if not width or not height or bit_depth != 8 or interlace != 0:
        raise ValueError("unsupported PNG encoding")
    channels = {0: 1, 2: 3, 3: 1, 4: 2, 6: 4}.get(color_type)
    if channels is None:
        raise ValueError("unsupported PNG color type")
    pixels = zlib.decompress(bytes(compressed))
    stride = width * channels
    expected = height * (stride + 1)
    if len(pixels) != expected:
        raise ValueError("unexpected PNG payload length")
    rows: list[bytearray] = []
    cursor = 0
    for _ in range(height):
        filter_type = pixels[cursor]
        source = pixels[cursor + 1 : cursor + 1 + stride]
        cursor += stride + 1
        previous = rows[-1] if rows else bytearray(stride)
        current = bytearray(stride)
        for index, value in enumerate(source):
            left = current[index - channels] if index >= channels else 0
            up = previous[index]
            upper_left = previous[index - channels] if index >= channels else 0
            if filter_type == 0:
                decoded = value
            elif filter_type == 1:
                decoded = value + left
            elif filter_type == 2:
                decoded = value + up
            elif filter_type == 3:
                decoded = value + ((left + up) // 2)
            elif filter_type == 4:
                decoded = value + _paeth(left, up, upper_left)
            else:
                raise ValueError("unsupported PNG filter")
            current[index] = decoded & 0xFF
        rows.append(current)

    total_pixels = width * height
    sample_step = max(1, total_pixels // 100_000)
    sampled = non_white = 0
    for pixel_index in range(0, total_pixels, sample_step):
        row = rows[pixel_index // width]
        start = (pixel_index % width) * channels
        values = row[start : start + channels]
        if color_type == 0:
            red = green = blue = values[0]
            alpha = 255
        elif color_type == 2:
            red, green, blue = values
            alpha = 255
        elif color_type == 3:
            palette_index = values[0]
            if palette_index >= len(palette):
                raise ValueError("invalid PNG palette index")
            red, green, blue = palette[palette_index]
            alpha = transparency[palette_index] if palette_index < len(transparency) else 255
        elif color_type == 4:
            red = green = blue = values[0]
            alpha = values[1]
        else:
            red, green, blue, alpha = values
        composited = [
            (channel * alpha + 255 * (255 - alpha)) // 255
            for channel in (red, green, blue)
        ]
        sampled += 1
        if min(composited) < 250:
            non_white += 1
    ratio = non_white / sampled if sampled else 0.0
    return {
        "width": width,
        "height": height,
        "sampled_pixel_count": sampled,
        "non_white_ratio": round(ratio, 6),
        "blank": ratio < MIN_NON_WHITE_RATIO,
        "pixel_check": "png_stdlib",
    }


def inspect_image_bytes(raw: bytes, mime_type: str) -> dict[str, Any]:
    mime = str(mime_type or "").lower()
    if raw.startswith(PNG_SIGNATURE):
        profile = _png_pixel_profile(raw)
        return {"format": "png", "magic_valid": True, **profile}
    if raw.startswith(JPEG_SIGNATURE):
        try:
            from PIL import Image, ImageStat  # type: ignore

            with Image.open(io.BytesIO(raw)) as image:
                rgb = image.convert("RGB")
                extrema = ImageStat.Stat(rgb).extrema
                blank = all(low >= 250 for low, _ in extrema)
                return {
                    "format": "jpeg",
                    "magic_valid": True,
                    "width": image.width,
                    "height": image.height,
                    "non_white_ratio": 0.0 if blank else None,
                    "blank": blank,
                    "pixel_check": "pillow_extrema",
                }
        except (ImportError, OSError) as exc:
            return {
                "format": "jpeg",
                "magic_valid": True,
                "blank": None,
                "pixel_check": "unavailable",
                "pixel_check_error": str(exc),
            }
    return {
        "format": mime or "unknown",
        "magic_valid": False,
        "blank": None,
        "pixel_check": "not_run",
    }


def _error_log_ids(value: Any) -> list[str]:
    return sorted(set(re.findall(r"20\d{12}[A-Fa-f0-9]{12,40}", str(value))))


def fetch_thumbnail_assets(
    locator: dict[str, str],
    sheet: dict[str, Any],
    *,
    expected_chart_ids: list[str],
    output_dir: Path,
    timeout: int,
    chart_id: str | None = None,
) -> dict[str, Any]:
    sheet_id = sheet_identifier(sheet)
    title = sheet_title(sheet)
    try:
        flags: dict[str, Any] = {"only_thumbnail": True}
        if chart_id:
            flags["chart_id"] = chart_id
        thumbnail_data = envelope_data(
            run_sheets(
                "+chart-list",
                **locator,
                sheet_id=sheet_id,
                flags=flags,
                timeout=timeout,
            )
        )
    except LarkCliError as exc:
        return {
            "status": "error",
            "expected_chart_ids": expected_chart_ids,
            "received_chart_ids": [],
            "valid_chart_ids": [],
            "missing_chart_ids": expected_chart_ids,
            "unavailable_chart_ids": expected_chart_ids,
            "coverage": {
                "valid": 0,
                "expected": len(expected_chart_ids),
                "rate": 0.0 if expected_chart_ids else None,
            },
            "files": [],
            "error": str(exc),
            "error_log_ids": _error_log_ids(exc),
        }

    thumbnails = extract_charts(thumbnail_data, sheet_id, title)
    output_dir.mkdir(parents=True, exist_ok=True)
    received_ids: list[str] = []
    valid_ids: list[str] = []
    files: list[dict[str, Any]] = []
    for chart in thumbnails:
        chart_id = str(chart.get("chart_id") or chart.get("id") or "")
        if not chart_id:
            continue
        received_ids.append(chart_id)
        details = chart.get("details") if isinstance(chart.get("details"), dict) else chart
        thumbnail = details.get("thumbnail") if isinstance(details.get("thumbnail"), dict) else {}
        encoded = thumbnail.get("base64")
        item: dict[str, Any] = {
            "chart_id": chart_id,
            "mime_type": str(thumbnail.get("mime_type") or thumbnail.get("mime") or ""),
            "version": str(thumbnail.get("version") or ""),
            "reported_width": thumbnail.get("width"),
            "reported_height": thumbnail.get("height"),
        }
        if not isinstance(encoded, str) or not encoded.strip():
            item.update({"status": "empty", "reason": "thumbnail.base64 is empty"})
            files.append(item)
            continue
        try:
            raw = base64.b64decode(encoded, validate=True)
            inspection = inspect_image_bytes(raw, item["mime_type"])
        except (binascii.Error, ValueError, zlib.error, struct.error) as exc:
            item.update({"status": "invalid", "reason": str(exc)})
            files.append(item)
            continue
        suffix = ".png" if inspection.get("format") == "png" else ".jpg"
        path = output_dir / f"{sheet_id}_{chart_id}{suffix}"
        path.write_bytes(raw)
        status = "blank" if inspection.get("blank") is True else "valid"
        if inspection.get("blank") is None:
            status = "unverifiable"
        item.update({"status": status, "path": str(path), "bytes": len(raw), **inspection})
        files.append(item)
        if status == "valid":
            valid_ids.append(chart_id)

    missing = [chart_id for chart_id in expected_chart_ids if chart_id not in received_ids]
    expected_set = set(expected_chart_ids)
    valid_set = set(valid_ids)
    valid_expected_count = len(expected_set & valid_set)
    unavailable = [chart_id for chart_id in expected_chart_ids if chart_id not in valid_set]
    if not received_ids or all(item.get("status") == "empty" for item in files):
        status = "empty"
    elif not missing and expected_set == valid_set:
        status = "ok"
    else:
        status = "partial"
    return {
        "status": status,
        "expected_chart_ids": expected_chart_ids,
        "received_chart_ids": received_ids,
        "valid_chart_ids": valid_ids,
        "missing_chart_ids": missing,
        "unavailable_chart_ids": unavailable,
        "coverage": {
            "valid": valid_expected_count,
            "expected": len(expected_set),
            "rate": valid_expected_count / len(expected_set) if expected_set else None,
        },
        "files": files,
        "error_log_ids": [],
    }


def fetch_thumbnail_assets_with_fallback(
    locator: dict[str, str],
    sheet: dict[str, Any],
    *,
    expected_chart_ids: list[str],
    output_dir: Path,
    timeout: int,
    retries: int,
) -> dict[str, Any]:
    result = fetch_thumbnail_assets(
        locator,
        sheet,
        expected_chart_ids=expected_chart_ids,
        output_dir=output_dir,
        timeout=timeout,
    )
    remaining = [
        chart_id
        for chart_id in expected_chart_ids
        if chart_id not in result.get("valid_chart_ids", [])
    ]
    attempts: list[dict[str, Any]] = []
    for attempt in range(1, retries + 1):
        if not remaining:
            break
        for chart_id in remaining:
            retry = fetch_thumbnail_assets(
                locator,
                sheet,
                expected_chart_ids=[chart_id],
                output_dir=output_dir,
                timeout=timeout,
                chart_id=chart_id,
            )
            attempts.append(
                {
                    "attempt": attempt,
                    "chart_id": chart_id,
                    "status": retry.get("status"),
                    "error_log_ids": retry.get("error_log_ids", []),
                }
            )
            result.setdefault("received_chart_ids", []).extend(
                retry.get("received_chart_ids", [])
            )
            if retry.get("files"):
                result["files"] = [
                    item
                    for item in result.get("files", [])
                    if item.get("chart_id") != chart_id
                ] + retry["files"]
            if chart_id in retry.get("valid_chart_ids", []):
                result.setdefault("valid_chart_ids", []).append(chart_id)
            result.setdefault("error_log_ids", []).extend(retry.get("error_log_ids", []))
        result["valid_chart_ids"] = list(dict.fromkeys(result.get("valid_chart_ids", [])))
        result["received_chart_ids"] = list(dict.fromkeys(result.get("received_chart_ids", [])))
        remaining = [
            chart_id
            for chart_id in expected_chart_ids
            if chart_id not in result["valid_chart_ids"]
        ]
    received_set = set(result.get("received_chart_ids", []))
    valid_set = set(result.get("valid_chart_ids", []))
    expected_set = set(expected_chart_ids)
    result["missing_chart_ids"] = [
        chart_id for chart_id in expected_chart_ids if chart_id not in received_set
    ]
    result["unavailable_chart_ids"] = remaining
    result["coverage"] = {
        "valid": len(expected_set & valid_set),
        "expected": len(expected_set),
        "rate": len(expected_set & valid_set) / len(expected_set) if expected_set else None,
    }
    result["error_log_ids"] = sorted(set(result.get("error_log_ids", [])))
    result["fallback_attempts"] = attempts
    if not remaining and set(result.get("valid_chart_ids", [])) == set(expected_chart_ids):
        result["status"] = "ok"
        result.pop("error", None)
    elif result.get("valid_chart_ids"):
        result["status"] = "partial"
    else:
        result["status"] = "error"
    return result


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
    skip_hidden: bool = False,
    timeout: int,
) -> dict[str, Any]:
    selector = f"id:{sheet_id}" if sheet_id else f"name:{sheet_name}"
    key = (selector, cell_range, include, skip_hidden)
    if key not in cache:
        flags: dict[str, Any] = {"range": cell_range, "include": include}
        if skip_hidden:
            flags["skip-hidden"] = True
        cache[key] = envelope_data(
            run_sheets(
                "+cells-get",
                **locator,
                **({"sheet_id": sheet_id} if sheet_id else {"sheet_name": sheet_name}),
                flags=flags,
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


def _plot(snapshot: dict[str, Any]) -> dict[str, Any]:
    plot_area = snapshot.get("plotArea")
    plot = plot_area.get("plot") if isinstance(plot_area, dict) else None
    return plot if isinstance(plot, dict) else {}


def _static_series_profiles(snapshot: dict[str, Any]) -> list[SeriesProfile]:
    data = snapshot.get("data")
    dim2 = data.get("dim2") if isinstance(data, dict) else None
    fields = dim2.get("fields") if isinstance(dim2, dict) else None
    if not isinstance(fields, list):
        return []
    profiles: list[SeriesProfile] = []
    for offset, field in enumerate(fields, start=1):
        if not isinstance(field, dict):
            continue
        values = []
        for value in field.get("parsedValues") or []:
            numeric = (
                float(value)
                if isinstance(value, (int, float)) and not isinstance(value, bool)
                else _numeric_text_value(value) if isinstance(value, str) else None
            )
            if numeric is not None:
                values.append(numeric)
        profiles.append(
            {
                "dimension_index": offset,
                "series_name": str(field.get("name") or f"Series {offset}"),
                "point_count": len(field.get("parsedValues") or []),
                "numeric_value_count": len(values),
                "unique_numeric_values": list(dict.fromkeys(values))[:2],
                "numeric_min": min(values) if values else None,
                "numeric_max": max(values) if values else None,
                "source_sheet": "",
                "source_range": "",
                "series_range": "",
            }
        )
    return profiles


def _labeled_series_indexes(
    snapshot: dict[str, Any], profiles: list[SeriesProfile]
) -> set[int]:
    plot = _plot(snapshot)
    available = {
        int(profile["dimension_index"])
        for profile in profiles
        if profile.get("dimension_index") is not None
    }
    labeled = set(available) if isinstance(plot.get("labels"), dict) else set()
    series = plot.get("series")
    if isinstance(series, list):
        labeled.update(
            int(item["index"])
            for item in series
            if isinstance(item, dict)
            and item.get("index") is not None
            and isinstance(item.get("labels"), dict)
        )
    return labeled & available


def _constant_labeled_series(
    chart: dict[str, Any], profiles: list[SeriesProfile]
) -> list[dict[str, Any]]:
    chart_id = str(chart.get("chart_id") or chart.get("id") or "")
    snapshot = _chart_snapshot(chart)
    labeled = _labeled_series_indexes(snapshot, profiles)
    return [
        {
            "chart_id": chart_id,
            "dimension_index": profile["dimension_index"],
            "series_name": profile["series_name"],
            "source_sheet": profile["source_sheet"],
            "source_range": profile["source_range"],
            "series_range": profile["series_range"],
            "reason": "constant_labeled_series",
            "data_point_count": profile["point_count"],
            "constant_value": profile["unique_numeric_values"][0],
            "suggested_fix": "remove_series_labels_or_use_one_sparse_marker",
        }
        for profile in profiles
        if int(profile.get("dimension_index", -1)) in labeled
        and profile.get("constant_check_unverifiable") is not True
        and int(profile.get("numeric_value_count", 0)) >= 2
        and len(profile.get("unique_numeric_values") or []) == 1
    ]


def _unbound_secondary_axis(chart: dict[str, Any]) -> dict[str, Any] | None:
    snapshot = _chart_snapshot(chart)
    plot_area = snapshot.get("plotArea")
    if not isinstance(plot_area, dict):
        return None
    plot = plot_area.get("plot")
    if not isinstance(plot, dict) or str(plot.get("type") or "").lower() != "combo":
        return None

    axes = plot_area.get("axes")
    has_right_axis = isinstance(axes, list) and any(
        isinstance(axis, dict)
        and str(axis.get("type") or "").lower() == "y"
        and str(axis.get("position") or axis.get("axisPosition") or "").lower() == "right"
        for axis in axes
    )
    series = plot.get("series")
    configured_series = (
        [item for item in series if isinstance(item, dict)]
        if isinstance(series, list)
        else []
    )
    if not has_right_axis or not configured_series:
        return None
    if str(plot.get("yAxisPosition") or "").lower() == "right" or any(
        str(item.get("yAxisPosition") or "").lower() == "right"
        for item in configured_series
    ):
        return None

    return {
        "chart_id": str(chart.get("chart_id") or chart.get("id") or ""),
        "reason": "secondary_axis_has_no_bound_series",
        "series_indexes": [
            int(item["index"])
            for item in configured_series
            if isinstance(item.get("index"), (int, float))
        ],
        "suggested_fix": "bind_the_intended_combo_series_to_the_right_axis",
    }


def _continuous_x_axis_utilization_warning(
    chart: dict[str, Any], profiles: list[SeriesProfile]
) -> dict[str, Any] | None:
    snapshot = _chart_snapshot(chart)
    chart_type = _chart_type(snapshot)
    if chart_type not in {"bubble", "scatter"}:
        return None
    profile = next(
        (
            item
            for item in profiles
            if item.get("role") == "x"
            and item.get("sampled") is not True
            and int(item.get("numeric_value_count", 0)) >= 2
        ),
        None,
    )
    if not profile:
        return None
    minimum = profile.get("numeric_min")
    maximum = profile.get("numeric_max")
    if not isinstance(minimum, (int, float)) or not isinstance(maximum, (int, float)):
        return None
    data_span = float(maximum) - float(minimum)
    if data_span <= 0 or float(minimum) <= 0 <= float(maximum):
        return None

    plot_area = snapshot.get("plotArea")
    axes = plot_area.get("axes") if isinstance(plot_area, dict) else None
    x_axis = next(
        (
            axis
            for axis in (axes if isinstance(axes, list) else [])
            if isinstance(axis, dict)
            and (
                str(axis.get("type") or "").lower() == "x"
                or str(axis.get("position") or axis.get("axisPosition") or "").lower()
                == "bottom"
            )
        ),
        {},
    )
    if x_axis.get("min") is not None or x_axis.get("max") is not None:
        return None
    zero_based_span = abs(float(maximum)) if float(minimum) > 0 else abs(float(minimum))
    if zero_based_span <= 0:
        return None
    utilization = data_span / zero_based_span
    if utilization >= MIN_CONTINUOUS_AXIS_DATA_UTILIZATION:
        return None
    padding = data_span * 0.05
    return {
        "chart_id": str(chart.get("chart_id") or chart.get("id") or ""),
        "axis": "x",
        "reason": "continuous_x_axis_may_be_underutilized",
        "data_min": float(minimum),
        "data_max": float(maximum),
        "estimated_data_span_ratio": round(utilization, 4),
        "minimum_recommended_ratio": MIN_CONTINUOUS_AXIS_DATA_UTILIZATION,
        "suggested_bounds": {
            "min": round(float(minimum) - padding, 6),
            "max": round(float(maximum) + padding, 6),
        },
        "review_note": "Keep zero when it is a meaningful baseline; otherwise tighten the X-axis bounds and review labels.",
        "suggested_fix": "review_x_axis_min_and_max",
    }


def _undersized_chart(chart: dict[str, Any]) -> dict[str, Any] | None:
    details = chart.get("details") if isinstance(chart.get("details"), dict) else chart
    snapshot = _chart_snapshot(chart)
    chart_type = _chart_type(snapshot)
    size = details.get("size")
    if not isinstance(size, dict):
        return None
    width = size.get("width")
    height = size.get("height")
    if (
        not isinstance(width, (int, float))
        or isinstance(width, bool)
        or width <= 0
        or not isinstance(height, (int, float))
        or isinstance(height, bool)
        or height <= 0
    ):
        return None
    actual = {
        "width": float(width),
        "height": float(height),
    }
    minimum = minimum_chart_size(chart_type)
    if actual["width"] >= minimum["width"] and actual["height"] >= minimum["height"]:
        return None
    return {
        "chart_id": str(chart.get("chart_id") or chart.get("id") or ""),
        "reason": "chart_below_minimum_size",
        "chart_type": chart_type,
        "actual_size": actual,
        "minimum_size": minimum,
        "suggested_fix": "run_lark_chart_size_advisor_before_resizing",
    }


def _overwide_chart(chart: dict[str, Any]) -> dict[str, Any] | None:
    details = chart.get("details") if isinstance(chart.get("details"), dict) else chart
    snapshot = _chart_snapshot(chart)
    chart_type = _chart_type(snapshot)
    size = details.get("size") if isinstance(details.get("size"), dict) else {}
    width = float(size.get("width") or 0)
    height = float(size.get("height") or 0)
    minimum = minimum_chart_size(chart_type)
    aspect_ratio = width / height if height > 0 else 0
    width_exceeded = width > MAX_CHART_WIDTH
    aspect_ratio_exceeded = (
        width >= minimum["width"]
        and height >= minimum["height"]
        and aspect_ratio > MAX_ASPECT_RATIO
    )
    if not width_exceeded and not aspect_ratio_exceeded:
        return None
    return {
        "chart_id": str(chart.get("chart_id") or chart.get("id") or ""),
        "reason": "chart_too_wide",
        "chart_type": chart_type,
        "actual_size": {"width": width, "height": height},
        "actual_aspect_ratio": round(aspect_ratio, 2),
        "maximum_width": MAX_CHART_WIDTH,
        "maximum_aspect_ratio": MAX_ASPECT_RATIO,
        "suggested_fix": "use_size_advisor_create_flags_or_change_chart_structure",
    }


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


def _aggregation_can_change_constant(data: dict[str, Any], dimension_index: int) -> bool:
    dim1 = data.get("dim1")
    category_series = dim1.get("serie") if isinstance(dim1, dict) else None
    if not isinstance(category_series, dict) or category_series.get("aggregate") is False:
        return False
    dim2 = data.get("dim2")
    value_series = dim2.get("series") if isinstance(dim2, dict) else None
    source_series = next(
        (
            item
            for item in (value_series if isinstance(value_series, list) else [])
            if isinstance(item, dict)
            and item.get("index") is not None
            and int(item["index"]) == dimension_index
        ),
        {},
    )
    aggregate_type = str(source_series.get("aggregateType") or "sum").lower()
    return aggregate_type in {"sum", "count", "counta"}


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
    text = value.strip()
    if not text:
        return None
    text = re.sub(r"^([+-]?)[\$\u00a5\uffe5\u20ac\u00a3]", r"\1", text)
    if text.endswith("%"):
        text = text[:-1]
    # Group separators must be consistent and separate groups of three digits.
    if not re.fullmatch(
        r"[+-]?(?:(?:\d+|\d{1,3}([, ])\d{3}(?:\1\d{3})*)(?:\.\d*)?|\.\d+)"
        r"(?:[eE][+-]?\d+)?",
        text,
    ):
        return None
    return float(text.replace(" ", "").replace(",", ""))


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


def _update_series_state(state: dict[str, Any], value: Any) -> None:
    state[_zero_state(value)] = True
    numeric = (
        float(value)
        if isinstance(value, (int, float)) and not isinstance(value, bool)
        else _numeric_text_value(value) if isinstance(value, str) else None
    )
    if numeric is None:
        return
    state["numeric_value_count"] += 1
    state["numeric_min"] = (
        numeric if state["numeric_min"] is None else min(state["numeric_min"], numeric)
    )
    state["numeric_max"] = (
        numeric if state["numeric_max"] is None else max(state["numeric_max"], numeric)
    )
    if len(state["unique_numeric_values"]) < 2:
        state["unique_numeric_values"].add(numeric)


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
) -> tuple[
    list[dict[str, Any]],
    list[dict[str, Any]],
    list[dict[str, str]],
    list[SeriesProfile],
]:
    chart_id = str(chart.get("chart_id") or chart.get("id") or "")
    snapshot = _chart_snapshot(chart)
    data = snapshot.get("data")
    if not isinstance(data, dict):
        return [], [], [{"chart_id": chart_id, "reason": "chart snapshot.data is missing"}], []
    if data.get("isStaticData") is True:
        return [], [], [], _static_series_profiles(snapshot)
    dimensions = _numeric_dimensions(snapshot)
    if not dimensions:
        return [], [], [], []
    refs = data.get("refs")
    if not isinstance(refs, list) or not refs:
        return [], [], [{"chart_id": chart_id, "reason": "chart data.refs is missing"}], []

    parsed_refs: list[tuple[str, str, CellBounds]] = []
    unverifiable: list[dict[str, str]] = []
    for ref in refs:
        raw_ref = ref.get("value") if isinstance(ref, dict) else ref
        try:
            parsed_refs.append(_parse_chart_ref(str(raw_ref), owner_sheet_name))
        except (TypeError, ValueError) as exc:
            unverifiable.append({"chart_id": chart_id, "reason": str(exc)})
            return [], [], unverifiable, []

    direction = str(data.get("direction") or "column").lower()
    skip_hidden = data.get("includeHiddenOrFilter") is not True
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
    series_profiles: list[SeriesProfile] = []
    remaining_source_cells = MAX_CELL_READ_SIZE
    for (source_sheet, source_range, bounds), ref_dimensions in mapped_by_ref.items():
        dimension_start = bounds[2] if direction == "column" else bounds[0]
        selected = {
            dimension_start + local_index - 1: (dimension_index, role)
            for dimension_index, role, local_index in ref_dimensions
        }
        dimension_span = max(selected) - min(selected) + 1
        header_points = 0 if detached else 1
        point_count = min(
            MAX_SOURCE_SAMPLE_POINTS + header_points,
            remaining_source_cells // dimension_span,
        )
        if point_count <= header_points:
            unverifiable.append({
                "chart_id": chart_id,
                "reason": f"source sampling 2000-cell budget cannot cover {source_sheet}!{source_range}",
            })
            continue
        if direction == "column":
            checked_bounds = (
                bounds[0],
                min(bounds[1], bounds[0] + point_count - 1),
                min(selected),
                max(selected),
            )
        else:
            checked_bounds = (
                min(selected),
                max(selected),
                bounds[2],
                min(bounds[3], bounds[2] + point_count - 1),
            )
        remaining_source_cells -= _bounds_area(checked_bounds)
        checked_range = _format_a1_bounds(checked_bounds)
        same_sheet = source_sheet == owner_sheet_name
        cells_data = _read_cells(
            cache,
            locator,
            sheet_id=owner_sheet_id if same_sheet else None,
            sheet_name=None if same_sheet else source_sheet,
            cell_range=checked_range,
            include="value,style,raw_value",
            skip_hidden=skip_hidden,
            timeout=timeout,
        )
        truncated = _cells_truncated(cells_data)
        if truncated:
            unverifiable.append(
                {"chart_id": chart_id, "reason": f"cells-get truncated for {source_sheet}!{checked_range}"}
            )
        states = {
            coordinate: {
                "zero": False,
                "empty": False,
                "nonzero": False,
                "sample_point_count": 0,
                "numeric_value_count": 0,
                "unique_numeric_values": set(),
                "numeric_min": None,
                "numeric_max": None,
            }
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
            states[coordinate]["sample_point_count"] += 1
            value = cell.get("value") if isinstance(cell, dict) else None
            _update_series_state(states[coordinate], value)
            number_format = (
                cell.get("cell_styles", {}).get("number_format")
                if isinstance(cell, dict) and isinstance(cell.get("cell_styles"), dict)
                else None
            )
            reason = ""
            uses_text_format = str(number_format or "").strip() == "@"
            if isinstance(value, (int, float)) and not isinstance(value, bool) and uses_text_format:
                reason = "numeric_value_uses_text_format"
            elif isinstance(value, str) and _looks_numeric(value):
                reason = "numeric_value_stored_as_text"
            if not reason:
                continue
            dimension_index, role = dimension
            key = (dimension_index, role, source_sheet, source_range, checked_range, reason)
            issue_counts[key] = issue_counts.get(key, 0) + 1
            samples = issue_groups.setdefault(key, [])
            if len(samples) < sample_limit:
                samples.append(f"{index_to_column(column_index)}{row_number}")

        if truncated:
            continue
        zero_candidates = {
            coordinate for coordinate, state in states.items() if not state["nonzero"]
        }
        data_start = bounds[0] + (0 if detached else 1)
        data_column = bounds[2] + (0 if detached else 1)
        for coordinate, state in states.items():
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
            source_series = next(
                (
                    item
                    for item in (data.get("dim2", {}).get("series") or [])
                    if isinstance(item, dict)
                    and item.get("index") is not None
                    and int(item["index"]) == dimension_index
                ),
                {},
            )
            profile = {
                "dimension_index": dimension_index,
                "series_name": str(
                    source_series.get("name")
                    or source_series.get("nameRef")
                    or f"Series {dimension_index}"
                ),
                "point_count": point_total,
                "sample_point_count": state["sample_point_count"],
                "checked_range": checked_range,
                "sampled": (
                    checked_bounds[1] < bounds[1]
                    if direction == "column"
                    else checked_bounds[3] < bounds[3]
                ),
                "numeric_value_count": state["numeric_value_count"],
                "unique_numeric_values": list(state["unique_numeric_values"]),
                "numeric_min": state["numeric_min"],
                "numeric_max": state["numeric_max"],
                "role": role,
                "source_sheet": source_sheet,
                "source_range": source_range,
                "series_range": series_range,
            }
            constant_labeled = (
                state["numeric_value_count"] >= 2
                and len(state["unique_numeric_values"]) == 1
                and dimension_index
                in _labeled_series_indexes(snapshot, [{"dimension_index": dimension_index}])
            )
            if profile["sampled"]:
                profile["constant_check_unverifiable"] = True
                if coordinate in zero_candidates or constant_labeled:
                    unverifiable.append({
                        "chart_id": chart_id,
                        "reason": (
                            "zero/constant-series check is unverifiable outside sampled source "
                            f"{source_sheet}!{checked_range} for dimension {dimension_index}"
                        ),
                    })
            elif constant_labeled and _aggregation_can_change_constant(data, dimension_index):
                profile["constant_check_unverifiable"] = True
                unverifiable.append(
                    {
                        "chart_id": chart_id,
                        "reason": (
                            "constant-series check is unverifiable after category "
                            f"aggregation for dimension {dimension_index}"
                        ),
                    }
                )
            series_profiles.append(profile)
            if coordinate not in zero_candidates or profile["sampled"]:
                continue
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
    return issues, degenerate_series, unverifiable, series_profiles


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
            "chart_ids": [],
            "sheet_size_px": None,
            "chart_overlaps": [],
            "cell_content_overlaps": [],
            "numeric_source_format_issues": [],
            "numeric_source_samples": [],
            "degenerate_numeric_series": [],
            "constant_labeled_series": [],
            "unbound_secondary_axes": [],
            "continuous_axis_utilization_warnings": [],
            "undersized_charts": [],
            "overwide_charts": [],
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
    numeric_source_samples: list[dict[str, Any]] = []
    degenerate_numeric_series: list[dict[str, Any]] = []
    constant_series_issues: list[dict[str, Any]] = []
    unbound_secondary_axes: list[dict[str, Any]] = []
    continuous_axis_utilization_warnings: list[dict[str, Any]] = []
    undersized_charts: list[dict[str, Any]] = []
    overwide_charts: list[dict[str, Any]] = []
    for chart in charts:
        issues, degenerate, source_unverifiable, profiles = _numeric_source_issues(
            chart,
            owner_sheet_id=sheet_id,
            owner_sheet_name=title,
            cache=cell_cache,
            locator=locator,
            timeout=timeout,
            sample_limit=sample_limit,
        )
        numeric_source_issues.extend(issues)
        numeric_source_samples.extend(
            {"chart_id": str(chart.get("chart_id") or chart.get("id") or ""), **profile}
            for profile in profiles
            if "checked_range" in profile
        )
        degenerate_numeric_series.extend(degenerate)
        unverifiable.extend(source_unverifiable)
        constant_series_issues.extend(_constant_labeled_series(chart, profiles))
        unbound_secondary_axis = _unbound_secondary_axis(chart)
        if unbound_secondary_axis:
            unbound_secondary_axes.append(unbound_secondary_axis)
        axis_utilization_warning = _continuous_x_axis_utilization_warning(chart, profiles)
        if axis_utilization_warning:
            continuous_axis_utilization_warnings.append(axis_utilization_warning)
        undersized = _undersized_chart(chart)
        if undersized:
            undersized_charts.append(undersized)
        overwide = _overwide_chart(chart)
        if overwide:
            overwide_charts.append(overwide)

    warnings.extend(
        f"{item['chart_id']}: continuous X axis may be underutilized; review whether zero is a meaningful baseline"
        for item in continuous_axis_utilization_warnings
    )

    issue_count = (
        len(overlaps)
        + len(out_of_bounds)
        + len(content_overlaps)
        + len(numeric_source_issues)
        + len(degenerate_numeric_series)
        + len(constant_series_issues)
        + len(unbound_secondary_axes)
        + len(undersized_charts)
        + len(overwide_charts)
    )
    return {
        "sheet_id": sheet_id,
        "sheet_name": title,
        "chart_count": len(charts),
        "chart_ids": [
            str(chart.get("chart_id") or chart.get("id") or "")
            for chart in charts
            if chart.get("chart_id") or chart.get("id")
        ],
        "sheet_size_px": {"width": round(sheet_width, 2), "height": round(sheet_height, 2)},
        "chart_overlaps": overlaps,
        "cell_content_overlaps": content_overlaps,
        "numeric_source_format_issues": numeric_source_issues,
        "numeric_source_samples": numeric_source_samples,
        "degenerate_numeric_series": degenerate_numeric_series,
        "constant_labeled_series": constant_series_issues,
        "unbound_secondary_axes": unbound_secondary_axes,
        "continuous_axis_utilization_warnings": continuous_axis_utilization_warnings,
        "undersized_charts": undersized_charts,
        "overwide_charts": overwide_charts,
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
            "minimum size, excessive width, constant labeled series, numeric source-cell "
            "formats, all-zero/empty numeric series, and unbound combo-chart secondary axes; "
            "also decode and validate every rendered chart thumbnail for model review."
        )
    )
    parser.add_argument("sheet_id", help="Spreadsheet URL or spreadsheet token")
    parser.add_argument("--worksheet-id", help="Only check this worksheet reference_id")
    parser.add_argument("--timeout", type=int, default=60)
    parser.add_argument("--sample-limit", type=int, default=10)
    parser.add_argument(
        "--thumbnail-output-dir",
        help="Directory for decoded chart thumbnails; defaults to a temporary directory",
    )
    parser.add_argument(
        "--thumbnail-retries",
        type=int,
        choices=range(0, 4),
        default=1,
        help="Per-chart fallback retries after a batch thumbnail failure (default: 1)",
    )
    parser.add_argument(
        "--verbose",
        action="store_true",
        help="Print the full report; the default stdout is compact and the full report is always saved",
    )
    return parser.parse_args()


def success_envelope(
    results: list[dict[str, Any]],
    thumbnail_results: list[dict[str, Any]] | None = None,
) -> dict[str, Any]:
    issue_count = sum(result["issue_count"] for result in results)
    unverifiable_count = sum(result["unverifiable_count"] for result in results)
    static_quality_passed = issue_count == 0 and unverifiable_count == 0
    thumbnails_checked = thumbnail_results is not None
    thumbnail_assets_passed = (
        all(result.get("status") in {"ok", "not_applicable"} for result in thumbnail_results)
        if thumbnail_results is not None
        else None
    )
    chart_count = sum(result["chart_count"] for result in results)
    thumbnail_expected_count = sum(
        len(result.get("expected_chart_ids", []))
        for result in (thumbnail_results or [])
    )
    thumbnail_valid_count = sum(
        len(result.get("valid_chart_ids", []))
        for result in (thumbnail_results or [])
    )
    missing_chart_ids = list(
        dict.fromkeys(
            chart_id
            for result in (thumbnail_results or [])
            for chart_id in result.get("missing_chart_ids", [])
        )
    )
    unavailable_chart_ids = list(
        dict.fromkeys(
            chart_id
            for result in (thumbnail_results or [])
            for chart_id in result.get("unavailable_chart_ids", [])
        )
    )
    read_required_files = [
        item
        for result in (thumbnail_results or [])
        for item in result.get("files", [])
        if item.get("status") == "valid" and item.get("path")
    ]
    read_required = [
        {"chart_id": str(item.get("chart_id")), "path": str(item["path"])}
        for item in read_required_files
    ]
    valid_chart_ids = list(dict.fromkeys(item["chart_id"] for item in read_required))
    error_log_ids = sorted(
        {
            log_id
            for result in (thumbnail_results or [])
            for log_id in result.get("error_log_ids", [])
        }
    )
    if issue_count > 0:
        result_type = "quality_failed"
        next_action = "fix_chart_then_rerun"
    elif thumbnail_assets_passed is False:
        result_type = "thumbnail_unavailable"
        next_action = "retry_thumbnail_or_report"
    elif unverifiable_count > 0:
        result_type = "execution_error"
        next_action = "retry_or_report"
    else:
        result_type = "ready_for_data_and_visual_review"
        next_action = "read_images_and_complete_data_visual_review"
    automated_checks_passed = static_quality_passed and thumbnail_assets_passed is not False
    if not chart_count:
        visual_status = "not_applicable"
        visual_passed = None
    elif not thumbnails_checked:
        visual_status = "not_checked"
        visual_passed = None
    elif thumbnail_assets_passed:
        visual_status = "pending_model_review"
        visual_passed = None
    else:
        visual_status = "unavailable"
        visual_passed = False
    warnings = [
        f"{result['sheet_name'] or result['sheet_id']}: {warning}"
        for result in results
        for warning in result["warnings"]
    ]
    warnings.extend(
        f"{result.get('sheet_name') or result.get('sheet_id')}: thumbnail status={result.get('status')}"
        for result in (thumbnail_results or [])
        if result.get("status") not in {"ok", "not_applicable"}
    )
    return {
        "ok": True,
        "engine": "lark",
        "action": ACTION,
        "data": {
            "result_type": result_type,
            "next_action": next_action,
            "automated_checks_passed": automated_checks_passed,
            "automated_checks_scope": "static_quality_and_thumbnail_assets_only",
            "acceptance": {
                "overall": {
                    "passed": None,
                    "status": "pending_data_and_visual_review" if chart_count else "not_applicable",
                },
                "data_semantics": {
                    "passed": None,
                    "status": "manual_review_required" if chart_count else "not_applicable",
                    "note": "Validate source data, formulas, aggregation, series mapping, and axis semantics separately.",
                },
                "static_quality": {
                    "passed": static_quality_passed,
                    "status": "passed" if static_quality_passed else "failed",
                },
                "visual_review": {
                    "passed": visual_passed,
                    "status": visual_status,
                    "note": (
                        "Read every file returned in thumbnail_fetch before declaring visual success."
                        if visual_status == "pending_model_review"
                        else "Visual acceptance is incomplete."
                    ),
                },
            },
            "thumbnail_fetch": {
                "checked": thumbnails_checked,
                "assets_ready": thumbnail_assets_passed,
                "result_type": (
                    "ready_for_visual_review"
                    if thumbnail_assets_passed is True
                    else "thumbnail_unavailable"
                    if thumbnail_assets_passed is False
                    else "not_checked"
                ),
                "next_action": (
                    "read_images_then_review"
                    if thumbnail_assets_passed is True
                    else "retry_thumbnail_or_report"
                    if thumbnail_assets_passed is False
                    else "none"
                ),
                "sheets": thumbnail_results or [],
                "expected_chart_ids": list(
                    dict.fromkeys(
                        chart_id
                        for result in (thumbnail_results or [])
                        for chart_id in result.get("expected_chart_ids", [])
                    )
                ),
                "valid_chart_ids": valid_chart_ids,
                "missing_chart_ids": missing_chart_ids,
                "unavailable_chart_ids": unavailable_chart_ids,
                "coverage": {
                    "valid": thumbnail_valid_count,
                    "expected": thumbnail_expected_count,
                    "rate": (
                        thumbnail_valid_count / thumbnail_expected_count
                        if thumbnail_expected_count
                        else None
                    ),
                },
                "read_required": read_required,
                "error_log_ids": error_log_ids,
            },
            "scope_note": (
                "out_of_visible_range checks worksheet drawable bounds, not a device-specific browser viewport; "
                "numeric source checks sample at most the first 50 data points of each chart value dimension "
                "for formats and scan candidate series fully for all-zero/empty and constant-value checks; "
                "label-density checks are advisory deterministic heuristics, not renderer collision detection, "
                "and are excluded from issue_count"
            ),
            "summary": {
                "worksheet_count": len(results),
                "chart_count": chart_count,
                "issue_count": issue_count,
                "unverifiable_count": unverifiable_count,
                "thumbnail_expected_count": thumbnail_expected_count,
                "thumbnail_valid_count": thumbnail_valid_count,
                "thumbnail_coverage": {
                    "valid": thumbnail_valid_count,
                    "expected": thumbnail_expected_count,
                    "rate": (
                        thumbnail_valid_count / thumbnail_expected_count
                        if thumbnail_expected_count
                        else None
                    ),
                },
            },
            "sheets": results,
        },
        "warnings": warnings,
    }


def compact_report(report: dict[str, Any]) -> dict[str, Any]:
    data = report["data"]
    thumbnail_fetch = data["thumbnail_fetch"]
    compact_thumbnail_sheets = [
        {
            key: sheet[key]
            for key in (
                "sheet_id",
                "sheet_name",
                "status",
                "expected_chart_ids",
                "received_chart_ids",
                "valid_chart_ids",
                "missing_chart_ids",
                "unavailable_chart_ids",
                "coverage",
                "error_log_ids",
                "fallback_attempts",
            )
            if key in sheet
        }
        for sheet in thumbnail_fetch.get("sheets", [])
    ]
    compact_sheets = [
        {
            key: sheet[key]
            for key in (
                "sheet_id",
                "sheet_name",
                "chart_count",
                "chart_ids",
                "chart_overlaps",
                "cell_content_overlaps",
                "numeric_source_format_issues",
                "degenerate_numeric_series",
                "constant_labeled_series",
                "unbound_secondary_axes",
                "continuous_axis_utilization_warnings",
                "undersized_charts",
                "overwide_charts",
                "out_of_visible_range",
                "unverifiable_charts",
                "issue_count",
                "unverifiable_count",
                "warnings",
            )
            if key in sheet
        }
        for sheet in data.get("sheets", [])
    ]
    return {
        "ok": report["ok"],
        "engine": report["engine"],
        "action": report["action"],
        "data": {
            "result_type": data["result_type"],
            "next_action": data["next_action"],
            "automated_checks_passed": data["automated_checks_passed"],
            "automated_checks_scope": data["automated_checks_scope"],
            "acceptance": data["acceptance"],
            "thumbnail_fetch": {
                key: value
                for key, value in thumbnail_fetch.items()
                if key != "sheets"
            }
            | {"sheets": compact_thumbnail_sheets},
            "summary": data["summary"],
            "sheets": compact_sheets,
        },
        "warnings": report["warnings"],
    }


def report_exit_code(report: dict[str, Any]) -> int:
    if report["data"]["automated_checks_passed"]:
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
        thumbnail_root = (
            Path(getattr(args, "thumbnail_output_dir", None)).expanduser().resolve()
            if getattr(args, "thumbnail_output_dir", None)
            else Path(tempfile.mkdtemp(prefix="lark_chart_thumbnails_"))
        )
        thumbnail_results = []
        for sheet, result in zip(sheets, results):
            chart_ids = result.get("chart_ids", [])
            if not chart_ids:
                thumbnail_results.append(
                    {
                        "sheet_id": sheet_identifier(sheet),
                        "sheet_name": sheet_title(sheet),
                        "status": "not_applicable",
                        "expected_chart_ids": [],
                        "received_chart_ids": [],
                        "valid_chart_ids": [],
                        "missing_chart_ids": [],
                        "unavailable_chart_ids": [],
                        "coverage": {"valid": 0, "expected": 0, "rate": None},
                        "files": [],
                        "error_log_ids": [],
                    }
                )
                continue
            thumbnail_result = fetch_thumbnail_assets_with_fallback(
                locator,
                sheet,
                expected_chart_ids=chart_ids,
                output_dir=thumbnail_root,
                timeout=args.timeout,
                retries=getattr(args, "thumbnail_retries", 1),
            )
            thumbnail_results.append(
                {
                    "sheet_id": sheet_identifier(sheet),
                    "sheet_name": sheet_title(sheet),
                    **thumbnail_result,
                }
            )
    except (LarkCliError, KeyError, TypeError, ValueError) as exc:
        print(
            json.dumps(
                {
                    "ok": False,
                    "engine": "lark",
                    "action": ACTION,
                    "error": str(exc),
                    "data": {
                        "result_type": "execution_error",
                        "next_action": "retry_or_report",
                        "error_log_ids": _error_log_ids(exc),
                    },
                    "warnings": [],
                },
                ensure_ascii=False,
                indent=2,
            )
        )
        raise SystemExit(1) from exc

    report = success_envelope(results, thumbnail_results)
    manifest_path = thumbnail_root / "quality_manifest.json"
    report["data"]["thumbnail_fetch"]["manifest_path"] = str(manifest_path)
    manifest_path.parent.mkdir(parents=True, exist_ok=True)
    manifest_path.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    output = report if getattr(args, "verbose", False) else compact_report(report)
    print(json.dumps(output, ensure_ascii=False, indent=2))
    exit_code = report_exit_code(report)
    if exit_code:
        raise SystemExit(exit_code)


if __name__ == "__main__":
    main()
