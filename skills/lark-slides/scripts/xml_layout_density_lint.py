#!/usr/bin/env python3
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
"""Warn when a large layout container has too little visible content inside it."""

from __future__ import annotations

import json
import re
import sys
from xml.etree import ElementTree as ET
from pathlib import Path
from typing import Any

import xml_text_overlap_lint as xml_lint


MIN_CONTAINER_WIDTH = 140
MIN_CONTAINER_HEIGHT = 160
MIN_CONTAINER_AREA = 20_000
MIN_CONTENT_COVERAGE_RATIO = 0.15
LARGE_VISUAL_CHILD_RATIO = 0.35
LAYOUT_PANEL_SPAN_RATIO = 0.90
IMAGE_OVERLAY_MATCH_RATIO = 0.90
DENSITY_CONTAINMENT_TOLERANCE = 8


def clipped_bbox(element: dict[str, Any], container: dict[str, Any]) -> dict[str, int | float] | None:
    left = max(element["x"], container["x"])
    top = max(element["y"], container["y"])
    right = min(element["x"] + element["width"], container["x"] + container["width"])
    bottom = min(element["y"] + element["height"], container["y"] + container["height"])
    if right <= left or bottom <= top:
        return None
    return {"x": left, "y": top, "width": right - left, "height": bottom - top}


def rectangle_union_area(rectangles: list[dict[str, int | float]]) -> int | float:
    x_coordinates = sorted({coordinate for rect in rectangles for coordinate in (rect["x"], rect["x"] + rect["width"])})
    area = 0
    for left, right in zip(x_coordinates, x_coordinates[1:]):
        intervals = sorted(
            (rect["y"], rect["y"] + rect["height"])
            for rect in rectangles
            if rect["x"] < right and rect["x"] + rect["width"] > left
        )
        covered_height = 0
        interval_end: int | float | None = None
        for top, bottom in intervals:
            if interval_end is None:
                covered_height += bottom - top
                interval_end = bottom
            elif bottom > interval_end:
                covered_height += bottom - max(top, interval_end)
                interval_end = bottom
        area += (right - left) * covered_height
    return area


def is_layout_container(element: dict[str, Any], slide_width: int | float, slide_height: int | float) -> bool:
    return (
        element["kind"] == "shape"
        and element["type"] == "rect"
        and element["width"] >= MIN_CONTAINER_WIDTH
        and element["height"] >= MIN_CONTAINER_HEIGHT
        and xml_lint.element_area(element) >= MIN_CONTAINER_AREA
        and not (
            element["x"] <= 2
            and element["y"] <= 2
            and element["width"] >= slide_width - 4
            and element["height"] >= slide_height - 4
        )
    )


def is_edge_spanning_layout_panel(
    element: dict[str, Any], slide_width: int | float, slide_height: int | float
) -> bool:
    touches_horizontal_edge = element["x"] <= 2 or element["x"] + element["width"] >= slide_width - 2
    touches_vertical_edge = element["y"] <= 2 or element["y"] + element["height"] >= slide_height - 2
    return (touches_horizontal_edge and element["height"] >= slide_height * LAYOUT_PANEL_SPAN_RATIO) or (
        touches_vertical_edge and element["width"] >= slide_width * LAYOUT_PANEL_SPAN_RATIO
    )


def has_matching_image_overlay(container: dict[str, Any], elements: list[dict[str, Any]]) -> bool:
    container_area = xml_lint.element_area(container)
    return any(
        element["kind"] == "img"
        and xml_lint.intersection_area(container, element)
        / max(1, min(container_area, xml_lint.element_area(element)))
        >= IMAGE_OVERLAY_MATCH_RATIO
        for element in elements
    )


def is_nested_in_layout_panel(
    container: dict[str, Any], elements: list[dict[str, Any]], slide_width: int | float, slide_height: int | float
) -> bool:
    return any(
        element is not container
        and element["kind"] == "shape"
        and element["type"] == "rect"
        and is_edge_spanning_layout_panel(element, slide_width, slide_height)
        and xml_lint.contains(element, container, tolerance=DENSITY_CONTAINMENT_TOLERANCE)
        for element in elements
    )


def extract_density_elements(slide_xml: str) -> list[dict[str, Any]]:
    elements = xml_lint.extract_elements(slide_xml)
    elements_by_id = {element["id"]: element for element in elements}
    root = ET.fromstring(slide_xml)
    for node in root.iter():
        if xml_lint.xml_local_name(node.tag) != "shape":
            continue
        element = elements_by_id.get(node.attrib.get("id", ""))
        if element is None:
            continue
        content_node = next(
            (child for child in node if xml_lint.xml_local_name(child.tag) == "content"),
            None,
        )
        paragraphs = (
            [
                " ".join("".join(paragraph.itertext()).split())
                for paragraph in content_node.iter()
                if xml_lint.xml_local_name(paragraph.tag) == "p"
            ]
            if content_node is not None
            else []
        )
        raw_font_size = (
            content_node.attrib.get("fontSize") if content_node is not None else None
        ) or node.attrib.get("fontSize")
        try:
            base_font_size = float(raw_font_size or 16)
        except ValueError:
            base_font_size = 16.0
        element.update(
            {
                "textType": content_node.attrib.get("textType") if content_node is not None else None,
                "textAlign": content_node.attrib.get("textAlign") if content_node is not None else None,
                "autoFit": content_node.attrib.get("autoFit") if content_node is not None else None,
                "fontSize": base_font_size,
                "text": "\n".join(paragraph for paragraph in paragraphs if paragraph),
            }
        )
        if not xml_lint.has_text_content(element):
            continue
        declared_font_sizes = [
            float(descendant.attrib["fontSize"])
            for descendant in node.iter()
            if descendant.attrib.get("fontSize") is not None
        ]
        if declared_font_sizes:
            element["fontSize"] = max(declared_font_sizes)
    for match in re.finditer(r"<icon\b([^>]*)>", slide_xml):
        attrs = match.group(1)
        x = xml_lint.extract_numeric_attribute(attrs, "topLeftX")
        y = xml_lint.extract_numeric_attribute(attrs, "topLeftY")
        width = xml_lint.extract_numeric_attribute(attrs, "width")
        height = xml_lint.extract_numeric_attribute(attrs, "height")
        if any(value is None for value in (x, y, width, height)):
            continue
        elements.append(
            {
                "id": xml_lint.extract_attribute(attrs, "id") or f"icon-{len(elements) + 1}",
                "kind": "icon",
                "type": "icon",
                "x": x,
                "y": y,
                "width": width,
                "height": height,
                "rotation": xml_lint.extract_numeric_attribute(attrs, "rotation") or 0,
                "order": len(elements),
            }
        )
    return elements


def visual_bbox(element: dict[str, Any], container: dict[str, Any]) -> dict[str, int | float] | None:
    if xml_lint.is_text_element(element):
        estimated = xml_lint.estimate_text_visual_bbox(element)
        return clipped_bbox(estimated, container) if estimated else None
    return clipped_bbox(element, container)


def own_text_visual_bbox(container: dict[str, Any]) -> dict[str, int | float] | None:
    if container["kind"] != "shape" or not xml_lint.has_text_content(container):
        return None
    text_proxy = {**container, "type": "text"}
    estimated = xml_lint.estimate_text_visual_bbox(text_proxy)
    return clipped_bbox(estimated, container) if estimated else None


def is_large_visual_child(element: dict[str, Any], container: dict[str, Any]) -> bool:
    if element["kind"] not in {"img", "chart", "table", "whiteboard"}:
        return False
    return xml_lint.element_area(element) / xml_lint.element_area(container) >= LARGE_VISUAL_CHILD_RATIO


def detect_sparse_container_content(
    elements: list[dict[str, Any]], slide_number: int, slide_width: int | float, slide_height: int | float
) -> list[dict[str, Any]]:
    issues: list[dict[str, Any]] = []
    for container in (element for element in elements if is_layout_container(element, slide_width, slide_height)):
        if (
            is_edge_spanning_layout_panel(container, slide_width, slide_height)
            or is_nested_in_layout_panel(container, elements, slide_width, slide_height)
            or has_matching_image_overlay(container, elements)
        ):
            continue
        children = [
            element
            for element in elements
            if element is not container
            and xml_lint.contains(container, element, tolerance=DENSITY_CONTAINMENT_TOLERANCE)
        ]
        if any(is_large_visual_child(child, container) for child in children):
            continue
        own_text_bbox = own_text_visual_bbox(container)
        rectangles = ([own_text_bbox] if own_text_bbox else []) + [
            bbox for child in children if (bbox := visual_bbox(child, container)) is not None
        ]
        content_area = rectangle_union_area(rectangles) if rectangles else 0
        coverage_ratio = content_area / xml_lint.element_area(container)
        if coverage_ratio >= MIN_CONTENT_COVERAGE_RATIO:
            continue
        issues.append(
            {
                "level": "warning",
                "code": "sparse_container_content",
                "schema_version": "1.0",
                "target": {
                    "slide_number": slide_number,
                    "container_id": container["id"],
                    "container_type": container["type"],
                    "bbox": {key: container[key] for key in ("x", "y", "width", "height")},
                },
                "rule": {
                    "name": "large_container_visible_content_coverage",
                    "threshold": MIN_CONTENT_COVERAGE_RATIO,
                    "comparison": "content_coverage_ratio < threshold",
                },
                "measurement": {
                    "container_area": xml_lint.element_area(container),
                    "visible_content_area": round(content_area, 3),
                    "content_coverage_ratio": round(coverage_ratio, 3),
                    "content_element_count": len(children) + (1 if own_text_bbox else 0),
                },
                "elements": [container["id"], *[child["id"] for child in children]],
            }
        )
    return issues


def detect_blank_slide(elements: list[dict[str, Any]], slide_number: int) -> list[dict[str, Any]]:
    if elements:
        return []
    return [
        {
            "level": "warning",
            "code": "blank_slide",
            "schema_version": "1.0",
            "target": {"slide_number": slide_number},
            "rule": {
                "name": "slide_has_visible_content",
                "comparison": "visible_element_count == 0",
            },
            "measurement": {"visible_element_count": 0},
            "elements": [],
        }
    ]


def lint_xml(xml: str, source_path: str | None = None) -> dict[str, Any]:
    root, xml_error = xml_lint.parse_xml_root(xml)
    if xml_error:
        return {
            "file": source_path,
            "summary": {"slide_count": 0, "warning_count": 0, "error_count": 1},
            "issues": [xml_error],
            "slides": [],
        }
    if root is None:
        raise AssertionError("parse_xml_root must return a root or error")
    presentation = xml_lint.parse_presentation(xml)
    slides = []
    for index, slide_xml in enumerate(presentation["slides"]):
        elements = extract_density_elements(slide_xml)
        slide_number = index + 1
        slides.append(
            {
                "slide_number": slide_number,
                "element_count": len(elements),
                "issues": detect_blank_slide(elements, slide_number)
                + detect_sparse_container_content(elements, slide_number, presentation["width"], presentation["height"]),
            }
        )
    warning_count = sum(len(slide["issues"]) for slide in slides)
    return {
        "file": source_path,
        "slide_size": {"width": presentation["width"], "height": presentation["height"]},
        "summary": {"slide_count": len(slides), "warning_count": warning_count, "error_count": 0},
        "slides": slides,
    }


def print_usage() -> None:
    print("Usage:\n  python3 xml_layout_density_lint.py --input <presentation.xml>", file=sys.stderr)


def run_cli(argv: list[str] | None = None) -> None:
    options = xml_lint.parse_args(argv or sys.argv[1:])
    if options.get("help") or options.get("--help"):
        print_usage()
        raise SystemExit(0)
    if not options.get("input"):
        print_usage()
        raise xml_lint.XmlTextOverlapLintError("--input is required")
    input_path = Path(options["input"]).resolve()
    print(json.dumps(lint_xml(xml_lint.read_file(input_path), str(input_path)), ensure_ascii=False, indent=2))


if __name__ == "__main__":
    try:
        run_cli()
    except xml_lint.XmlTextOverlapLintError as error:
        print(f"xml-layout-density-lint error: {error}", file=sys.stderr)
        raise SystemExit(1) from error
