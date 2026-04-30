#!/usr/bin/env python3
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

from __future__ import annotations

import json
import re
import sys
import xml.etree.ElementTree as ET
from pathlib import Path
from typing import Any


class LayoutLintError(Exception):
    pass


def fail(message: str) -> None:
    raise LayoutLintError(message)


def read_file(file_path: str | Path) -> str:
    return Path(file_path).read_text(encoding="utf-8")


def parse_args(argv: list[str]) -> dict[str, Any]:
    options: dict[str, Any] = {}
    index = 0
    while index < len(argv):
        token = argv[index]
        if not token.startswith("--"):
            fail(f"unexpected argument: {token}")
        key = token[2:]
        next_token = argv[index + 1] if index + 1 < len(argv) else None
        if next_token is None or next_token.startswith("--"):
            options[key] = True
            index += 1
            continue
        options[key] = next_token
        index += 2
    return options


def extract_attribute(tag_source: str, name: str) -> str | None:
    match = re.search(fr'{re.escape(name)}="([^"]+)"', tag_source)
    return match.group(1) if match else None


def extract_numeric_attribute(tag_source: str, name: str) -> int | float | None:
    raw = extract_attribute(tag_source, name)
    if raw is None:
        return None
    try:
        value = float(raw)
    except ValueError:
        return None
    return int(value) if value.is_integer() else value


def strip_xml(value: str) -> str:
    stripped = re.sub(r"<!\[CDATA\[([\s\S]*?)\]\]>", r"\1", value)
    stripped = re.sub(r"<[^>]+>", " ", stripped)
    stripped = stripped.replace("&nbsp;", " ")
    stripped = stripped.replace("&amp;", "&")
    stripped = stripped.replace("&lt;", "<")
    stripped = stripped.replace("&gt;", ">")
    stripped = stripped.replace("&quot;", '"')
    stripped = stripped.replace("&#39;", "'")
    return re.sub(r"\s+", " ", stripped).strip()


def xml_local_name(tag: str) -> str:
    return tag.rsplit("}", 1)[-1] if tag.startswith("{") else tag


def extract_error_context(xml: str, line: int | None, column: int | None, radius: int = 40) -> str | None:
    if line is None or column is None:
        return None
    lines = xml.splitlines()
    if line < 1 or line > len(lines):
        return None
    source_line = lines[line - 1]
    start = max(column - radius, 0)
    end = min(column + radius, len(source_line))
    return source_line[start:end].strip()


def build_xml_error_issue(error: ET.ParseError, xml: str) -> dict[str, Any]:
    line, column = getattr(error, "position", (None, None))
    return {
        "level": "error",
        "code": "xml_not_well_formed",
        "message": f"XML is not well-formed: {error}",
        "line": line,
        "column": column,
        "context": extract_error_context(xml, line, column),
        "hint": (
            "Escape raw user text before placing it in XML. In text nodes and attribute values, bare & must be "
            "written as &amp;. In text nodes, write < as &lt; and > as &gt;. For attribute URLs, use a=1&amp;b=2."
        ),
    }


def validate_xml_well_formed(xml: str) -> dict[str, Any] | None:
    try:
        root = ET.fromstring(xml)
    except ET.ParseError as error:
        return build_xml_error_issue(error, xml)

    root_name = xml_local_name(root.tag)
    if root_name not in {"presentation", "slide"}:
        fail("input must contain a <presentation> or <slide> root")
    return None


def parse_presentation(xml: str) -> dict[str, Any]:
    presentation_match = re.search(r"<presentation\b([^>]*)>", xml)
    if presentation_match:
        return {
            "width": int(float(extract_attribute(presentation_match.group(1), "width") or 960)),
            "height": int(float(extract_attribute(presentation_match.group(1), "height") or 540)),
            "slides": re.findall(r"<slide\b[\s\S]*?</slide>", xml),
        }
    slide_match = re.findall(r"<slide\b[\s\S]*?</slide>", xml)
    if slide_match:
        return {"width": 960, "height": 540, "slides": slide_match}
    fail("input must contain a <presentation> or <slide> root")


def extract_elements(slide_xml: str) -> list[dict[str, Any]]:
    elements: list[dict[str, Any]] = []
    for match in re.finditer(r"<shape\b([^>]*)>([\s\S]*?)</shape>", slide_xml):
        attrs, content = match.group(1), match.group(2)
        x = extract_numeric_attribute(attrs, "topLeftX")
        y = extract_numeric_attribute(attrs, "topLeftY")
        width = extract_numeric_attribute(attrs, "width")
        height = extract_numeric_attribute(attrs, "height")
        if all(value is not None for value in [x, y, width, height]):
            font_size = float(extract_attribute(content, "fontSize") or extract_attribute(attrs, "fontSize") or 16)
            elements.append(
                {
                    "id": f"shape-{len(elements) + 1}",
                    "kind": "shape",
                    "type": extract_attribute(attrs, "type") or "shape",
                    "textType": extract_attribute(content, "textType"),
                    "x": x,
                    "y": y,
                    "width": width,
                    "height": height,
                    "fontSize": font_size,
                    "text": strip_xml(content),
                }
            )

    for match in re.finditer(r"<(img|table|chart)\b([^>]*)/?>", slide_xml):
        attrs = match.group(2)
        x = extract_numeric_attribute(attrs, "topLeftX")
        y = extract_numeric_attribute(attrs, "topLeftY")
        width = extract_numeric_attribute(attrs, "width")
        height = extract_numeric_attribute(attrs, "height")
        if all(value is not None for value in [x, y, width, height]):
            elements.append(
                {
                    "id": f"{match.group(1)}-{len(elements) + 1}",
                    "kind": match.group(1),
                    "type": match.group(1),
                    "x": x,
                    "y": y,
                    "width": width,
                    "height": height,
                }
            )
    return elements


def intersects(left: dict[str, Any], right: dict[str, Any]) -> bool:
    return (
        left["x"] < right["x"] + right["width"]
        and left["x"] + left["width"] > right["x"]
        and left["y"] < right["y"] + right["height"]
        and left["y"] + left["height"] > right["y"]
    )


def is_text_element(element: dict[str, Any]) -> bool:
    return element["kind"] == "shape" and element["type"] == "text"


def is_backgroundish(element: dict[str, Any], slide_area: int | float) -> bool:
    if slide_area <= 0:
        return False
    area = element["width"] * element["height"]
    if element["kind"] == "img":
        return area >= slide_area * 0.45
    if element["kind"] == "shape" and element["type"] != "text":
        return area >= slide_area * 0.35
    return False


def should_flag_overlap(left: dict[str, Any], right: dict[str, Any], slide_area: int | float) -> bool:
    if is_backgroundish(left, slide_area) or is_backgroundish(right, slide_area):
        return False
    if is_text_element(left) and is_text_element(right):
        return True
    allowed_companions = {"img", "table", "chart"}
    return (
        is_text_element(left) and right["kind"] in allowed_companions
    ) or (
        is_text_element(right) and left["kind"] in allowed_companions
    )


def estimate_text_height(element: dict[str, Any]) -> int | None:
    if element["kind"] != "shape" or element["type"] != "text" or not element.get("text"):
        return None
    font_size = element["fontSize"] if isinstance(element["fontSize"], (int, float)) else 16
    chars_per_line = max(1, int(element["width"] // max(font_size * 0.55, 1)))
    paragraphs = [paragraph for paragraph in re.split(r"\n+", element["text"]) if paragraph]
    line_count = 0
    for paragraph in paragraphs:
        logical_length = max(len(paragraph), 1)
        line_count += max(1, -(-logical_length // chars_per_line))
    return int((line_count * font_size * 1.35) + 12 + 0.999999)


def lint_slide(slide_xml: str, slide_number: int, width: int, height: int) -> dict[str, Any]:
    elements = extract_elements(slide_xml)
    issues: list[dict[str, Any]] = []
    slide_area = width * height

    for element in elements:
        if (
            element["x"] < 0
            or element["y"] < 0
            or element["x"] + element["width"] > width
            or element["y"] + element["height"] > height
        ):
            issues.append(
                {
                    "level": "error",
                    "code": "out_of_bounds",
                    "element": element["id"],
                    "message": f'{element["id"]} exceeds slide bounds',
                }
            )
        estimated_height = estimate_text_height(element)
        if estimated_height is not None and estimated_height > element["height"]:
            issues.append(
                {
                    "level": "warning",
                    "code": "text_height_risk",
                    "element": element["id"],
                    "message": f'{element["id"]} may need {estimated_height}px height but only has {element["height"]}px',
                }
            )

    for index, left in enumerate(elements):
        for right in elements[index + 1 :]:
            if not intersects(left, right) or not should_flag_overlap(left, right, slide_area):
                continue
            issues.append(
                {
                    "level": "error",
                    "code": "bbox_overlap",
                    "elements": [left["id"], right["id"]],
                    "message": f'{left["id"]} overlaps {right["id"]}',
                }
            )

    footer_candidates = [
        element
        for element in elements
        if element["kind"] == "shape"
        and element["type"] == "text"
        and element["y"] >= height - 80
        and element["height"] <= 60
    ]
    for footer in footer_candidates:
        for element in elements:
            if (
                element["id"] == footer["id"]
                or not intersects(footer, element)
                or not should_flag_overlap(footer, element, slide_area)
            ):
                continue
            issues.append(
                {
                    "level": "warning",
                    "code": "footer_collision",
                    "elements": [footer["id"], element["id"]],
                    "message": f'{footer["id"]} is being crowded by {element["id"]}',
                }
            )

    return {"slide_number": slide_number, "element_count": len(elements), "issues": issues}


def lint_xml(xml: str, source_path: str | None = None) -> dict[str, Any]:
    xml_error = validate_xml_well_formed(xml)
    if xml_error:
        return {
            "file": source_path,
            "slide_size": {"width": 960, "height": 540},
            "summary": {"slide_count": 0, "error_count": 1, "warning_count": 0},
            "issues": [xml_error],
            "slides": [],
        }

    presentation = parse_presentation(xml)
    slides = [
        lint_slide(slide_xml, index + 1, presentation["width"], presentation["height"])
        for index, slide_xml in enumerate(presentation["slides"])
    ]
    error_count = sum(1 for slide in slides for issue in slide["issues"] if issue["level"] == "error")
    warning_count = sum(1 for slide in slides for issue in slide["issues"] if issue["level"] == "warning")
    return {
        "file": source_path,
        "slide_size": {"width": presentation["width"], "height": presentation["height"]},
        "summary": {"slide_count": len(slides), "error_count": error_count, "warning_count": warning_count},
        "slides": slides,
    }


def print_usage() -> None:
    print("Usage:\n  python3 layout_lint.py --input <presentation.xml>", file=sys.stderr)


def run_cli(argv: list[str] | None = None) -> None:
    options = parse_args(argv or sys.argv[1:])
    if options.get("help") or options.get("--help"):
        print_usage()
        raise SystemExit(0)
    if not options.get("input"):
        print_usage()
        fail("--input is required")
    input_path = Path(options["input"]).resolve()
    result = lint_xml(read_file(input_path), str(input_path))
    print(json.dumps(result, ensure_ascii=False, indent=2))
    if result["summary"]["error_count"] > 0:
        raise SystemExit(1)


if __name__ == "__main__":
    try:
        run_cli()
    except LayoutLintError as error:
        print(f"layout-lint error: {error}", file=sys.stderr)
        raise SystemExit(1) from error
