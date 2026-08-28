#!/usr/bin/env python3
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
"""Pure sizing heuristics shared by Lark chart helper scripts."""

from __future__ import annotations

import math
import unicodedata
from typing import Any


MINIMUM_SIZES = {
    "column": (640, 400),
    "line": (640, 400),
    "area": (640, 400),
    "bar": (720, 420),
    "combo": (720, 420),
    "pie": (720, 440),
    "doughnut": (720, 440),
}
DEFAULT_MINIMUM_SIZE = (640, 400)


def display_units(value: Any) -> int:
    """Estimate visible text width; CJK/full-width characters count double."""
    lines = str(value if value is not None else "").splitlines() or [""]
    return max(
        sum(2 if unicodedata.east_asian_width(char) in {"W", "F", "A"} else 1 for char in line)
        for line in lines
    )


def _round_up(value: float, step: int = 40) -> int:
    return int(math.ceil(value / step) * step)


def _percentile(values: list[int], ratio: float) -> int:
    if not values:
        return 0
    ordered = sorted(values)
    return ordered[max(0, math.ceil(len(ordered) * ratio) - 1)]


def minimum_chart_size(chart_type: str) -> dict[str, int]:
    width, height = MINIMUM_SIZES.get(str(chart_type).lower(), DEFAULT_MINIMUM_SIZE)
    return {"width": width, "height": height}


def estimate_legend_rows(items: list[str], width: int) -> int:
    if not items:
        return 0
    available = max(240, width - 80)
    used = 0
    rows = 1
    for item in items:
        item_width = min(320, 34 + display_units(item) * 7)
        if used and used + item_width > available:
            rows += 1
            used = 0
        used += item_width
    return rows


def dense_data_labels(
    *,
    chart_type: str,
    category_count: int,
    labeled_series_count: int,
    width: float,
    height: float,
) -> dict[str, Any] | None:
    if category_count <= 0 or labeled_series_count <= 0:
        return None
    chart_type = str(chart_type).lower()
    label_count = category_count * labeled_series_count
    if chart_type in {"pie", "doughnut"}:
        labels_per_side = max(1, math.ceil(category_count / 2))
        slot = max(1.0, float(height) - 160) / labels_per_side
        dense = slot < 24
    else:
        horizontal_reserve = 230 if chart_type == "combo" else 170
        plot_width = max(1.0, float(width) - horizontal_reserve)
        slot = plot_width / label_count
        dense = (
            (category_count >= 8 and labeled_series_count >= 2 and slot < 42)
            or (category_count >= 15 and slot < 36)
        )
    if not dense:
        return None
    return {
        "estimated_label_count": label_count,
        "available_width_per_label_px": round(slot, 2),
    }


def recommend_chart_size(
    *,
    chart_type: str,
    categories: list[Any],
    series_names: list[str],
    data_labels: str = "value",
    legend_position: str = "bottom",
    title: str = "",
    values: list[float] | None = None,
) -> dict[str, Any]:
    chart_type = str(chart_type).lower()
    category_text = [str(value if value is not None else "") for value in categories]
    category_count = len(category_text)
    series_count = max(1, len(series_names))
    label_units = [display_units(value) for value in category_text]
    max_units = max(label_units, default=0)
    p75_units = _percentile(label_units, 0.75)
    max_lines = max((len(value.splitlines()) for value in category_text), default=1)
    labels_enabled = str(data_labels or "").lower() not in {"", "none"}
    minimum = minimum_chart_size(chart_type)
    width = float(minimum["width"])
    height = float(minimum["height"])
    reasons: list[str] = []
    advice: list[str] = []

    if chart_type in {"pie", "doughnut"}:
        label_reserve = max(150, min(360, max_units * 7 + 60))
        width = max(width, 420 + 2 * label_reserve)
        if labels_enabled:
            reasons.append("outside_slice_labels")
        if values:
            positive = [value for value in values if value > 0]
            total = sum(positive)
            if total:
                shares = [value / total for value in positive]
                if max(shares, default=0) >= 0.75 and sum(share < 0.05 for share in shares) >= 3:
                    height += 40
                    reasons.append("clustered_small_slices")
        if category_count > 8:
            advice.append("prefer_bar_or_top_n")
        size_alone_is_insufficient = category_count > 12
    elif chart_type == "bar":
        width = max(width, 420 + max_units * 7)
        height = max(height, 190 + category_count * 36)
        size_alone_is_insufficient = category_count > 24 and series_count > 2
        if size_alone_is_insufficient:
            advice.extend(["use_top_n", "split_chart"])
    else:
        reserve = 230 if chart_type == "combo" else 170
        base_slot = 44 if chart_type in {"line", "area"} else 52
        text_slot = 20 + p75_units * 7 * 0.72
        slot = max(base_slot, min(180, text_slot))
        if chart_type in {"column", "combo"} and series_count > 1:
            slot = max(slot, 20 + 22 * min(series_count, 5))
        if labels_enabled:
            slot += min(24, 4 * series_count)
        width = max(width, reserve + category_count * slot)
        if p75_units > 12:
            height += 40
            reasons.append("long_category_labels")
        if max_lines > 1:
            height += min(120, 40 * (max_lines - 1))
            reasons.append("multiline_category_labels")
        size_alone_is_insufficient = (
            category_count > 20
            and (p75_units > 12 or series_count > 3 or labels_enabled)
        )
        if size_alone_is_insufficient:
            advice.extend(["prefer_bar_or_top_n", "split_chart"])

    width = min(1600, _round_up(width))
    legend_items = category_text if chart_type in {"pie", "doughnut"} else series_names
    legend_rows = 0
    if str(legend_position).lower() != "hidden":
        legend_rows = estimate_legend_rows(legend_items, width)
        if legend_rows > 1:
            height += (legend_rows - 1) * 32
            reasons.append("multi_row_legend")

    label_density = dense_data_labels(
        chart_type=chart_type,
        category_count=category_count,
        labeled_series_count=series_count if labels_enabled else 0,
        width=width,
        height=height,
    )
    if label_density and chart_type not in {"pie", "doughnut"}:
        target_slot = 42 if category_count >= 8 and series_count >= 2 else 36
        required_width = reserve + category_count * series_count * target_slot
        width = min(1600, _round_up(max(width, required_width)))
        label_density = dense_data_labels(
            chart_type=chart_type,
            category_count=category_count,
            labeled_series_count=series_count if labels_enabled else 0,
            width=width,
            height=height,
        )
        if not label_density:
            reasons.append("expanded_for_data_labels")
    if label_density:
        reasons.append("dense_data_labels")
        advice.append("label_only_key_points")
        if label_density["estimated_label_count"] > 40:
            size_alone_is_insufficient = True
            advice.append("split_series_or_use_top_n")
    if title:
        reasons.append("chart_title")

    height = min(720, _round_up(height))
    if size_alone_is_insufficient:
        if chart_type in {"pie", "doughnut"}:
            height = max(height, 520)
        else:
            width = max(width, 1200)
            height = max(height, 520)

    return {
        "minimum_size": minimum,
        "recommended_size": {"width": width, "height": height},
        "create_flags": {"width": width, "height": height},
        "evidence": {
            "chart_type": chart_type,
            "category_count": category_count,
            "series_count": series_count,
            "max_category_display_units": max_units,
            "p75_category_display_units": p75_units,
            "max_category_line_count": max_lines,
            "legend_rows": legend_rows,
            "data_labels": data_labels,
        },
        "reasons": list(dict.fromkeys(reasons)),
        "layout_advice": list(dict.fromkeys(advice)),
        "size_alone_is_insufficient": size_alone_is_insufficient,
    }
