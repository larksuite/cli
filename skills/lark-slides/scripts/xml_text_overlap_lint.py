#!/usr/bin/env python3
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
"""Validate Slides XML structure and page layout through one release gate."""

from __future__ import annotations

import json
import math
import re
import sys
import unicodedata
import xml.parsers.expat as expat
import xml.etree.ElementTree as ET
from difflib import SequenceMatcher, get_close_matches
from pathlib import Path
from typing import Any


XS_NS = "{http://www.w3.org/2001/XMLSchema}"
XML_NS = "{http://www.w3.org/XML/1998/namespace}"
SVG_NS = "{http://www.w3.org/2000/svg}"
SML_NAMESPACE = "http://www.larkoffice.com/sml/2.0"
SXSD_SCHEMA_PATH = Path(__file__).resolve().parents[1] / "references" / "slides_xml_schema_definition.xml"
ICONPARK_INDEX_PATH = Path(__file__).resolve().parents[1] / "references" / "iconpark-index.json"
SXSD_TAG_ALIASES = {
    "textbox": "<shape type=\"text\">",
    "textBox": "<shape type=\"text\">",
    "image": "<img>",
    "picture": "<img>",
}
SXSD_ATTR_ALIASES = {
    "x": "topLeftX",
    "left": "topLeftX",
    "y": "topLeftY",
    "top": "topLeftY",
    "w": "width",
    "h": "height",
    "fontColor": "color",
}
SERVER_FILLED_SXSD_ATTRS = {"id"}
ROUNDTRIP_SXSD_ATTRS = {
    ("chart", "updated"),
    ("chartData", "isStaticData"),
}
# Slides readback echoes each chartField's CSV text as per-value <chartParsedValues> children;
# it's server-emitted, absent from the write schema, and appears on virtually every chart-bearing
# deck, so treating it as an unsupported tag would block per-slide linting document-wide.
ROUNDTRIP_SXSD_TAGS = {"chartParsedValues"}
DEFAULT_TABLE_COLUMN_WIDTH = 110
DEFAULT_TABLE_ROW_HEIGHT = 37
# Sub-pixel canvas overflow is floating-point rounding noise (e.g. rotated-bbox math), not a
# visible defect; keep this well under 1px so real overflow is still always caught.
CANVAS_OVERFLOW_TOLERANCE = 0.5
_SXSD_TAG_ATTRIBUTES_CACHE: dict[str, set[str]] | None = None
_ICONPARK_ICON_TYPES_CACHE: set[str] | None = None


class XmlLayoutLintError(Exception):
    pass


def fail(message: str) -> None:
    raise XmlLayoutLintError(message)


def read_file(file_path: str | Path) -> str:
    return Path(file_path).read_text(encoding="utf-8")


def parse_args(argv: list[str]) -> dict[str, Any]:
    options: dict[str, Any] = {}
    index = 0
    while index < len(argv):
        token = argv[index]
        if not token.startswith("--"):
            fail(f"unexpected argument: {token}, need --input")
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
    match = re.search(
        fr"(?:^|\s){re.escape(name)}\s*=\s*(?:\"([^\"]+)\"|'([^']+)')", tag_source
    )
    if not match:
        return None
    return match.group(1) if match.group(1) is not None else match.group(2)


def extract_numeric_attribute(tag_source: str, name: str) -> int | float | None:
    raw = extract_attribute(tag_source, name)
    if raw is None:
        return None
    try:
        value = float(raw)
    except ValueError:
        return None
    return int(value) if value.is_integer() else value


def sum_sizes(sizes: list[int | float]) -> int | float:
    return sum(sizes)


def is_filled_size(size: int | float | None) -> bool:
    return isinstance(size, (int, float)) and math.isfinite(size) and size > 0


def fill_last_size_gap(sizes: list[int | float], target_size: int | float) -> list[int | float]:
    if not sizes:
        return sizes
    final_sizes = [
        size if index == len(sizes) - 1 else max(1, math.floor(size + 0.5))
        for index, size in enumerate(sizes)
    ]
    remaining_size = target_size - sum_sizes(final_sizes[:-1])
    if remaining_size >= 1:
        final_sizes[-1] = remaining_size
        return final_sizes

    size_to_redistribute = 1 - remaining_size
    for index in range(len(final_sizes) - 2, -1, -1):
        reduction = min(final_sizes[index] - 1, size_to_redistribute)
        final_sizes[index] -= reduction
        size_to_redistribute -= reduction
        if size_to_redistribute == 0:
            final_sizes[-1] = 1
            return final_sizes

    final_sizes[-1] = 1
    return final_sizes


def solve_weighted_min_layout(
    input_sizes: list[int | float | None], default_size: int | float, target_min_size: int | float | None
) -> dict[str, Any]:
    filled_indexes: list[int] = []
    empty_indexes: list[int] = []
    base_sizes: list[int | float] = []
    for index, size in enumerate(input_sizes):
        if is_filled_size(size):
            filled_indexes.append(index)
            base_sizes.append(size)
        else:
            empty_indexes.append(index)
            base_sizes.append(0)
    filled_sum = sum_sizes(base_sizes)

    if target_min_size is None:
        final_sizes = [default_size if index in empty_indexes else size for index, size in enumerate(base_sizes)]
        return {"final_sizes": final_sizes, "actual_size": sum_sizes(final_sizes), "ratio": 1}

    if not filled_indexes:
        average_size = target_min_size / len(input_sizes)
        final_sizes = fill_last_size_gap([average_size] * len(input_sizes), target_min_size)
        return {"final_sizes": final_sizes, "actual_size": sum_sizes(final_sizes), "ratio": 1}

    if empty_indexes:
        remaining_size = target_min_size - filled_sum
        final_sizes = [*base_sizes]
        if remaining_size > 0:
            average_size = remaining_size / len(empty_indexes)
            empty_sizes = fill_last_size_gap([average_size] * len(empty_indexes), remaining_size)
            for index, empty_size in zip(empty_indexes, empty_sizes):
                final_sizes[index] = empty_size
        else:
            for index in empty_indexes:
                final_sizes[index] = default_size
        return {"final_sizes": final_sizes, "actual_size": sum_sizes(final_sizes), "ratio": 1}

    ratio = max(1, target_min_size / filled_sum)
    actual_size = max(target_min_size, filled_sum)
    if ratio == 1:
        return {"final_sizes": [*base_sizes], "actual_size": actual_size, "ratio": ratio}
    final_sizes = fill_last_size_gap([size * ratio for size in base_sizes], actual_size)
    return {"final_sizes": final_sizes, "actual_size": sum_sizes(final_sizes), "ratio": ratio}


def strip_xml(value: str, preserve_line_breaks: bool = False) -> str:
    stripped = re.sub(r"<!\[CDATA\[([\s\S]*?)\]\]>", r"\1", value)
    if preserve_line_breaks:
        stripped = re.sub(r"<br\b[^>]*>", "\n", stripped)
    stripped = re.sub(r"<[^>]+>", " ", stripped)
    stripped = stripped.replace("&nbsp;", " ")
    stripped = stripped.replace("&amp;", "&")
    stripped = stripped.replace("&lt;", "<")
    stripped = stripped.replace("&gt;", ">")
    stripped = stripped.replace("&quot;", '"')
    stripped = stripped.replace("&#39;", "'")
    if preserve_line_breaks:
        return "\n".join(re.sub(r"\s+", " ", line).strip() for line in stripped.split("\n"))
    return re.sub(r"\s+", " ", stripped).strip()


def strip_xml_paragraphs(value: str) -> str:
    paragraphs = re.findall(r"<p\b[^>]*>([\s\S]*?)</p\s*>", value)
    if paragraphs:
        return "\n".join(strip_xml(paragraph, preserve_line_breaks=True) for paragraph in paragraphs)
    return strip_xml(value, preserve_line_breaks=True)


def extract_text_paragraphs(value: str, default_font_size: int | float) -> list[dict[str, Any]]:
    paragraphs = []
    for attrs, body in re.findall(r"<p\b([^>]*)>([\s\S]*?)</p\s*>", value):
        paragraphs.append(
            {
                "text": strip_xml(body, preserve_line_breaks=True),
                "fontSize": extract_max_span_font_size(body, default_font_size),
                "textAlign": extract_attribute(attrs, "textAlign"),
                "lineSpacing": extract_attribute(attrs, "lineSpacing"),
                "beforeLineSpacing": extract_attribute(attrs, "beforeLineSpacing"),
                "afterLineSpacing": extract_attribute(attrs, "afterLineSpacing"),
            }
        )
    return paragraphs


def extract_max_span_font_size(value: str, default_font_size: int | float) -> int | float:
    font_sizes = [
        font_size
        for attrs in re.findall(r"<span\b([^>]*)>", value)
        if (font_size := extract_numeric_attribute(attrs, "fontSize")) is not None
    ]
    return max([default_font_size, *font_sizes])


def extract_tag_attributes(value: str, tag: str) -> str:
    match = re.search(fr"<{re.escape(tag)}\b([^>]*)>", value)
    return match.group(1) if match else ""


def xml_local_name(tag: str) -> str:
    return tag.rsplit("}", 1)[-1] if tag.startswith("{") else tag


def xml_namespace(tag: str) -> str | None:
    return tag.split("}", 1)[0] + "}" if tag.startswith("{") else None


def strip_xsd_prefix(value: str | None) -> str | None:
    if value is None:
        return None
    return value.rsplit(":", 1)[-1]


def iter_direct_xsd_children(element: ET.Element, local_name: str) -> list[ET.Element]:
    return [child for child in element if child.tag == f"{XS_NS}{local_name}"]


def load_sxsd_tag_attributes() -> dict[str, set[str]]:
    global _SXSD_TAG_ATTRIBUTES_CACHE
    if _SXSD_TAG_ATTRIBUTES_CACHE is not None:
        return _SXSD_TAG_ATTRIBUTES_CACHE

    schema_root = ET.parse(SXSD_SCHEMA_PATH).getroot()
    named_complex_types = {
        complex_type.attrib["name"]: complex_type
        for complex_type in schema_root.findall(f"{XS_NS}complexType")
        if complex_type.attrib.get("name")
    }
    resolving: set[str] = set()

    def attributes_for_complex_type(complex_type: ET.Element) -> set[str]:
        attrs: set[str] = {
            attribute.attrib["name"]
            for attribute in iter_direct_xsd_children(complex_type, "attribute")
            if attribute.attrib.get("name")
        }
        for content_name in ("simpleContent", "complexContent"):
            for complex_content in iter_direct_xsd_children(complex_type, content_name):
                for extension in iter_direct_xsd_children(complex_content, "extension"):
                    base_type = strip_xsd_prefix(extension.attrib.get("base"))
                    if base_type:
                        attrs.update(attributes_for_type(base_type))
                    attrs.update(
                        attribute.attrib["name"]
                        for attribute in iter_direct_xsd_children(extension, "attribute")
                        if attribute.attrib.get("name")
                    )
        return attrs

    def attributes_for_type(type_name: str) -> set[str]:
        if type_name in resolving:
            return set()
        complex_type = named_complex_types.get(type_name)
        if complex_type is None:
            return set()
        resolving.add(type_name)
        try:
            return attributes_for_complex_type(complex_type)
        finally:
            resolving.remove(type_name)

    tag_attributes: dict[str, set[str]] = {}
    for element in schema_root.iter(f"{XS_NS}element"):
        tag_name = element.attrib.get("name")
        if not tag_name:
            continue

        attrs: set[str] = set()
        type_name = strip_xsd_prefix(element.attrib.get("type"))
        if type_name:
            attrs.update(attributes_for_type(type_name))
        for complex_type in iter_direct_xsd_children(element, "complexType"):
            attrs.update(attributes_for_complex_type(complex_type))

        tag_attributes.setdefault(tag_name, set()).update(attrs)

    _SXSD_TAG_ATTRIBUTES_CACHE = tag_attributes
    return tag_attributes


def load_iconpark_icon_types() -> set[str]:
    global _ICONPARK_ICON_TYPES_CACHE
    if _ICONPARK_ICON_TYPES_CACHE is not None:
        return _ICONPARK_ICON_TYPES_CACHE

    try:
        index_data = json.loads(ICONPARK_INDEX_PATH.read_text(encoding="utf-8"))
    except json.JSONDecodeError as error:
        fail(f"invalid iconpark index JSON: {error}")
    icons = index_data.get("icons")
    if not isinstance(icons, list):
        fail("iconpark index must contain an icons array")

    icon_types = {
        icon["iconType"]
        for icon in icons
        if isinstance(icon, dict) and isinstance(icon.get("iconType"), str) and icon["iconType"]
    }
    _ICONPARK_ICON_TYPES_CACHE = icon_types
    return icon_types


def build_sxsd_tag_hint(tag_name: str, supported_tags: set[str]) -> str:
    alias = SXSD_TAG_ALIASES.get(tag_name)
    if alias:
        return f"Use {alias} instead of <{tag_name}>."
    if tag_name == "svg":
        return 'Inside <whiteboard>, write SVG as <svg xmlns="http://www.w3.org/2000/svg">...</svg>.'
    close_matches = get_close_matches(tag_name, sorted(supported_tags), n=3, cutoff=0.72)
    if close_matches:
        return "Unsupported SXSD tag. Did you mean " + ", ".join(f"<{match}>" for match in close_matches) + "?"
    return "Unsupported SXSD tag. Use only tags defined in slides_xml_schema_definition.xml."


def build_sxsd_attr_hint(tag_name: str, attr_name: str, allowed_attrs: set[str]) -> str:
    alias = SXSD_ATTR_ALIASES.get(attr_name)
    if alias and alias in allowed_attrs:
        return f'Use "{alias}" on <{tag_name}> instead of "{attr_name}".'
    close_matches = get_close_matches(attr_name, sorted(allowed_attrs), n=3, cutoff=0.68)
    if close_matches:
        return "Unsupported SXSD attribute. Did you mean " + ", ".join(f'"{match}"' for match in close_matches) + "?"
    allowed_summary = ", ".join(sorted(allowed_attrs)[:8])
    if len(allowed_attrs) > 8:
        allowed_summary += ", ..."
    return f"Unsupported SXSD attribute for <{tag_name}>. Allowed attributes include: {allowed_summary}."


def should_skip_sxsd_subtree(element: ET.Element, ancestors: list[str]) -> bool:
    return "whiteboard" in ancestors and xml_namespace(element.tag) == SVG_NS


def should_skip_sxsd_attribute(tag_name: str, attr_name: str) -> bool:
    return attr_name in SERVER_FILLED_SXSD_ATTRS or (tag_name, attr_name) in ROUNDTRIP_SXSD_ATTRS


def validate_sxsd_tag_attributes(root: ET.Element) -> list[dict[str, Any]]:
    tag_attributes = load_sxsd_tag_attributes()
    supported_tags = set(tag_attributes)
    issues: list[dict[str, Any]] = []

    def visit(element: ET.Element, ancestors: list[str], path: str) -> None:
        if should_skip_sxsd_subtree(element, ancestors):
            return

        tag_name = xml_local_name(element.tag)
        current_path = f"{path}/{tag_name}" if path else tag_name
        if tag_name in ROUNDTRIP_SXSD_TAGS:
            return
        if tag_name not in supported_tags:
            issues.append(
                {
                    "level": "error",
                    "code": "sxsd_unsupported_tag",
                    "tag": tag_name,
                    "path": current_path,
                    "message": f"unsupported SXSD tag <{tag_name}> at {current_path}",
                    "hint": build_sxsd_tag_hint(tag_name, supported_tags),
                }
            )
            return
        else:
            allowed_attrs = tag_attributes[tag_name]
            for raw_attr_name in element.attrib:
                if raw_attr_name.startswith(XML_NS):
                    continue
                attr_name = xml_local_name(raw_attr_name)
                if should_skip_sxsd_attribute(tag_name, attr_name):
                    continue
                if attr_name in allowed_attrs:
                    continue
                issues.append(
                    {
                        "level": "error",
                        "code": "sxsd_unsupported_attr",
                        "tag": tag_name,
                        "attr": attr_name,
                        "path": current_path,
                        "message": f'unsupported SXSD attribute "{attr_name}" on <{tag_name}> at {current_path}',
                        "hint": build_sxsd_attr_hint(tag_name, attr_name, allowed_attrs),
                    }
                )

        for child in element:
            visit(child, [*ancestors, tag_name], current_path)

    visit(root, [], "")
    return issues


def build_iconpark_icon_type_hint(icon_type: str, supported_icon_types: set[str]) -> str:
    close_matches = get_close_matches(icon_type, sorted(supported_icon_types), n=3, cutoff=0.58)
    if close_matches:
        return (
            "iconType must exist in iconpark-index.json. Did you mean "
            + ", ".join(f'"{match}"' for match in close_matches)
            + "?"
        )
    return "iconType must exist in iconpark-index.json. Use scripts/iconpark_tool.py to search supported icons."


def validate_iconpark_icon_types(root: ET.Element) -> list[dict[str, Any]]:
    supported_icon_types: set[str] | None = None
    issues: list[dict[str, Any]] = []

    def direct_child(element: ET.Element, local_name: str) -> ET.Element | None:
        return next((child for child in element if xml_local_name(child.tag) == local_name), None)

    def is_transparent_color(color: str) -> bool:
        normalized = re.sub(r"\s+", "", color).lower()
        if normalized == "transparent":
            return True
        rgba_match = re.fullmatch(r"rgba\([^,]+,[^,]+,[^,]+,([0-9.]+)\)", normalized)
        if not rgba_match:
            return False
        try:
            return float(rgba_match.group(1)) <= 0
        except ValueError:
            return False

    def append_missing_fill_color_issue(current_path: str) -> None:
        issues.append(
            {
                "level": "error",
                "code": "icon_missing_fill_color",
                "tag": "icon",
                "path": current_path,
                "message": f"<icon> must set explicit non-transparent fillColor for visual visibility at {current_path}",
                "hint": 'Add <fill><fillColor color="rgba(R, G, B, 1)"/></fill> inside <icon>. This is a visual lint rule, not an SXSD required field.',
            }
        )

    def visit(element: ET.Element, path: str) -> None:
        nonlocal supported_icon_types
        tag_name = xml_local_name(element.tag)
        current_path = f"{path}/{tag_name}" if path else tag_name
        if tag_name == "icon":
            icon_type = element.attrib.get("iconType")
            if icon_type is not None:
                if supported_icon_types is None:
                    supported_icon_types = load_iconpark_icon_types()
                if icon_type not in supported_icon_types:
                    issues.append(
                        {
                            "level": "error",
                            "code": "iconpark_unsupported_icon_type",
                            "tag": "icon",
                            "attr": "iconType",
                            "iconType": icon_type,
                            "path": current_path,
                            "message": f'unsupported iconpark iconType "{icon_type}" at {current_path}',
                            "hint": build_iconpark_icon_type_hint(icon_type, supported_icon_types),
                        }
                    )
            fill = direct_child(element, "fill")
            fill_color = direct_child(fill, "fillColor") if fill is not None else None
            color = fill_color.attrib.get("color") if fill_color is not None else None
            if not color:
                append_missing_fill_color_issue(current_path)
            elif is_transparent_color(color):
                issues.append(
                    {
                        "level": "error",
                        "code": "icon_transparent_fill_color",
                        "tag": "icon",
                        "attr": "fillColor",
                        "path": current_path,
                        "color": color,
                        "message": f'<icon> fillColor must not be transparent for visual visibility at {current_path}: "{color}"',
                        "hint": 'Use an opaque visible color, for example <fillColor color="rgba(37, 99, 235, 1)"/>.',
                    }
                )
        for child in element:
            visit(child, current_path)

    visit(root, "")
    return issues


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


def validate_sml_tag_prefixes(xml: str) -> list[dict[str, Any]]:
    namespace_map: dict[str, str] = {}
    pending_declarations: list[tuple[str, str | None]] = []
    declarations_by_element: list[list[tuple[str, str | None]]] = []
    element_stack: list[str] = []
    issues: list[dict[str, Any]] = []

    parser = expat.ParserCreate(namespace_separator="|")
    parser.namespace_prefixes = True

    def handle_namespace_decl(prefix: str | None, namespace: str) -> None:
        normalized_prefix = prefix or ""
        previous_namespace = namespace_map.get(normalized_prefix)
        namespace_map[normalized_prefix] = namespace
        pending_declarations.append((normalized_prefix, previous_namespace))

    def handle_start_element(name: str, _attrs: dict[str, str]) -> None:
        declarations_by_element.append(pending_declarations.copy())
        pending_declarations.clear()
        name_parts = name.rsplit("|", 2)
        if len(name_parts) == 3:
            _namespace, local_name, prefix = name_parts
            element_name = f"{prefix}:{local_name}"
        else:
            prefix = ""
            local_name = name_parts[-1]
            element_name = local_name
        element_stack.append(element_name)
        if not prefix:
            return

        if namespace_map.get(prefix) != SML_NAMESPACE:
            return
        path = "/".join(element_stack)
        issues.append(
            {
                "level": "error",
                "code": "sml_prefixed_tag",
                "tag": element_name,
                "namespace": SML_NAMESPACE,
                "path": path,
                "line": parser.CurrentLineNumber,
                "column": parser.CurrentColumnNumber,
                "message": f"SML tag <{element_name}> must not use a namespace prefix at {path}",
                "hint": (
                    f'Use <{local_name}> under the default namespace '
                    f'<{local_name} xmlns="{SML_NAMESPACE}">, or use an unprefixed SML tag.'
                ),
            }
        )

    def handle_end_element(_name: str) -> None:
        for prefix, previous_namespace in reversed(declarations_by_element.pop()):
            if previous_namespace is None:
                namespace_map.pop(prefix, None)
            else:
                namespace_map[prefix] = previous_namespace
        element_stack.pop()

    parser.StartNamespaceDeclHandler = handle_namespace_decl
    parser.StartElementHandler = handle_start_element
    parser.EndElementHandler = handle_end_element
    parser.Parse(xml, True)
    return issues


def parse_xml_root(xml: str) -> tuple[ET.Element | None, dict[str, Any] | None]:
    try:
        root = ET.fromstring(xml)
    except ET.ParseError as error:
        return None, build_xml_error_issue(error, xml)

    root_name = xml_local_name(root.tag)
    if root_name not in {"presentation", "slide"}:
        fail("input must contain a <presentation> or <slide> root")
    return root, None


def validate_xml_well_formed(xml: str) -> dict[str, Any] | None:
    _, xml_error = parse_xml_root(xml)
    return xml_error


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

    for match in re.finditer(r"<(shape|img|table|chart|whiteboard)\b([^>]*)>", slide_xml):
        kind, attrs = match.group(1), match.group(2)
        is_self_closing = attrs.rstrip().endswith("/")
        content = ""
        if kind in {"shape", "table"} and not is_self_closing:
            close_index = slide_xml.find(f"</{kind}>", match.end())
            if close_index != -1:
                content = slide_xml[match.end() : close_index]

        element_id = extract_attribute(attrs, "id") or f"{kind}-{len(elements) + 1}"
        x = extract_numeric_attribute(attrs, "topLeftX")
        y = extract_numeric_attribute(attrs, "topLeftY")
        width = extract_numeric_attribute(attrs, "width")
        height = extract_numeric_attribute(attrs, "height")
        rotation = extract_numeric_attribute(attrs, "rotation") or 0
        alpha = extract_numeric_attribute(attrs, "alpha")
        table_layouts: dict[str, dict[str, Any] | None] = {}
        if kind == "table":
            width, table_layouts["width"] = resolve_table_dimension(
                content, width, extract_table_column_sizes, DEFAULT_TABLE_COLUMN_WIDTH
            )
            height, table_layouts["height"] = resolve_table_dimension(
                content, height, extract_table_row_sizes, DEFAULT_TABLE_ROW_HEIGHT
            )
        if all(value is not None for value in [x, y, width, height]):
            element = {
                "id": element_id,
                "kind": kind,
                "type": extract_attribute(attrs, "type") or kind,
                "x": x,
                "y": y,
                "width": width,
                "height": height,
                "rotation": rotation,
                "alpha": alpha if alpha is not None else 1,
                "order": len(elements),
            }
            if kind == "table":
                element.update(
                    {
                        "declared_width": extract_numeric_attribute(attrs, "width"),
                        "declared_height": extract_numeric_attribute(attrs, "height"),
                        "table_layouts": table_layouts,
                    }
                )
            if kind == "shape":
                content_attrs = extract_tag_attributes(content, "content")
                font_size = extract_numeric_attribute(content_attrs, "fontSize")
                if font_size is None:
                    font_size = extract_numeric_attribute(attrs, "fontSize")
                element.update(
                    {
                        "textType": extract_attribute(content_attrs, "textType"),
                        "textAlign": extract_attribute(content_attrs, "textAlign"),
                        "verticalAlign": extract_attribute(content_attrs, "verticalAlign") or "middle",
                        "vert": extract_attribute(attrs, "vert") or "horz",
                        "autoFit": extract_attribute(content_attrs, "autoFit"),
                        "wrap": extract_attribute(content_attrs, "wrap"),
                        "lineSpacing": extract_attribute(content_attrs, "lineSpacing"),
                        "beforeLineSpacing": extract_attribute(content_attrs, "beforeLineSpacing"),
                        "afterLineSpacing": extract_attribute(content_attrs, "afterLineSpacing"),
                        "paddingTop": extract_numeric_attribute(content_attrs, "paddingTop") or 0,
                        "paddingRight": extract_numeric_attribute(content_attrs, "paddingRight") or 0,
                        "paddingBottom": extract_numeric_attribute(content_attrs, "paddingBottom") or 0,
                        "paddingLeft": extract_numeric_attribute(content_attrs, "paddingLeft") or 0,
                        "fontSize": font_size if font_size is not None else 16,
                        "text": strip_xml_paragraphs(content),
                        "paragraphs": extract_text_paragraphs(content, font_size if font_size is not None else 16),
                    }
                )
            elements.append(element)
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


def is_whiteboard_element(element: dict[str, Any]) -> bool:
    return element["kind"] == "whiteboard"


def has_text_content(element: dict[str, Any]) -> bool:
    return bool(element.get("text"))


def is_vertical_text(element: dict[str, Any]) -> bool:
    return element.get("vert") in {"vert", "vert270", "word-art-vert", "word-art-vert-rtl", "ea-vert"}


def detect_image_text_occlusions(elements: list[dict[str, Any]]) -> list[dict[str, Any]]:
    issues: list[dict[str, Any]] = []
    text_elements = [element for element in elements if is_text_element(element) and has_text_content(element)]
    image_elements = [element for element in elements if element["kind"] == "img" and element["alpha"] > 0]
    for text_element in text_elements:
        for image_element in image_elements:
            if image_element["order"] <= text_element["order"]:
                continue
            if is_vertical_text(text_element):
                if intersects(image_element, text_element):
                    issues.append({
                        "level": "info",
                        "code": "image_may_cover_vertical_text",
                        "elements": [image_element["id"], text_element["id"]],
                        "message": f'image {image_element["id"]} may cover vertical text shape {text_element["id"]}',
                        "hint": "Inspect the rendered slide because vertical text layout is not statically modeled.",
                    })
                continue
            text_visual_bbox = estimate_text_visual_bbox(text_element)
            if text_visual_bbox is not None and intersects(image_element, text_visual_bbox):
                issues.append({
                    "level": "error",
                    "code": "image_covers_text",
                    "elements": [image_element["id"], text_element["id"]],
                    "message": f'image {image_element["id"]} covers text shape {text_element["id"]}',
                    "hint": "Move the image before the text shape in XML order, or adjust the image and text shape coordinates or dimensions.",
                })
    return issues


def is_decorative_text(element: dict[str, Any]) -> bool:
    text = element.get("text") or ""
    return bool(text) and re.search(r"[A-Za-z0-9\u4e00-\u9fff]", text) is None


def normalize_text_for_overlap(text: str) -> str:
    return re.sub(r"\s+", "", text)


def estimate_character_width(character: str, font_size: int | float) -> int | float:
    if character.isspace():
        return font_size * 0.33
    if unicodedata.east_asian_width(character) in {"F", "W"}:
        return font_size
    return font_size * 0.55


def estimate_text_width(text: str, font_size: int | float) -> int | float:
    return sum(estimate_character_width(character, font_size) for character in text)


def estimate_text_max_line_width(element: dict[str, Any]) -> int | float:
    font_size = element["fontSize"] if isinstance(element["fontSize"], (int, float)) else 16
    paragraphs = [paragraph for paragraph in re.split(r"\n+", element["text"]) if paragraph]
    return max([estimate_text_width(paragraph, font_size) for paragraph in paragraphs] or [1])


def is_similar_text_overlay(left: dict[str, Any], right: dict[str, Any]) -> bool:
    left_text = normalize_text_for_overlap(left.get("text") or "")
    right_text = normalize_text_for_overlap(right.get("text") or "")
    if not left_text or not right_text:
        return False
    if left_text == right_text or left_text in right_text or right_text in left_text:
        return True
    return SequenceMatcher(None, left_text, right_text).ratio() >= 0.75


def estimate_text_line_count_for_text(element: dict[str, Any], text: str) -> int:
    font_size = element["fontSize"] if isinstance(element["fontSize"], (int, float)) else 16
    hard_lines = text.split("\n")
    if not text:
        return 0
    line_count = 0
    for hard_line in hard_lines:
        if element.get("wrap") in {"false", "0"}:
            line_count += 1
            continue
        logical_width = max(estimate_text_width(hard_line, font_size), 1)
        line_count += max(1, math.ceil(logical_width / max(element["width"], 1)))
    return line_count


def estimate_text_line_count(element: dict[str, Any]) -> int:
    return max(estimate_text_line_count_for_text(element, element["text"]), 1)


def estimate_text_line_height(element: dict[str, Any], line_spacing: str | None = None) -> int | float | None:
    font_size = element["fontSize"] if isinstance(element["fontSize"], (int, float)) else 16
    line_spacing = line_spacing or "multiple:1.5"
    match = re.fullmatch(r"(multiple|fixed):([0-9]+(?:\.[0-9]+)?)", line_spacing)
    if match is None:
        return None
    spacing_type, value = match.groups()
    return font_size * float(value) if spacing_type == "multiple" else float(value)


def detect_text_may_overflow_shapes(elements: list[dict[str, Any]]) -> list[dict[str, Any]]:
    issues: list[dict[str, Any]] = []
    for element in elements:
        if not is_text_element(element) or not has_text_content(element):
            continue
        if element.get("autoFit") in {"normal-auto-fit", "shape-auto-fit"}:
            continue

        font_size = element["fontSize"] if isinstance(element["fontSize"], (int, float)) else 16
        paragraphs = element.get("paragraphs") or [
            {
                "text": element["text"],
                "lineSpacing": None,
                "beforeLineSpacing": None,
                "afterLineSpacing": None,
            }
        ]
        line_count = 0
        estimated_height = 0.0
        line_heights: list[int | float] = []
        for paragraph in paragraphs:
            paragraph_line_count = estimate_text_line_count_for_text(element, paragraph["text"])
            if paragraph_line_count == 0:
                continue
            line_height = estimate_text_line_height(element, paragraph["lineSpacing"] or element["lineSpacing"])
            before_spacing = estimate_text_line_height(
                element, paragraph["beforeLineSpacing"] or element["beforeLineSpacing"] or "fixed:0"
            )
            after_spacing = estimate_text_line_height(
                element, paragraph["afterLineSpacing"] or element["afterLineSpacing"] or "fixed:0"
            )
            if line_height is None or before_spacing is None or after_spacing is None:
                line_count = 0
                break
            first_line_height = font_size if line_count == 0 else line_height
            line_count += paragraph_line_count
            line_heights.append(line_height)
            estimated_height += (
                before_spacing + first_line_height + max(paragraph_line_count - 1, 0) * line_height + after_spacing
            )
        if line_count == 0:
            continue
        available_height = max(element["height"] - element["paddingTop"] - element["paddingBottom"], 0)
        overflow = estimated_height - available_height
        if overflow <= 0:
            continue

        issues.append(
            {
                "level": "warning",
                "code": "text_may_overflow_shape",
                "elements": [element["id"]],
                "line_count": line_count,
                "line_height": max(line_heights),
                "estimated_height": estimated_height,
                "available_height": available_height,
                "overflow": overflow,
                "message": (
                    f'text shape {element["id"]} may overflow its own content box '
                    f'(estimated {estimated_height:g}px, available {available_height:g}px); '
                    'consider setting content wrap="true" autoFit="normal-auto-fit"'
                ),
                "hint": (
                    "Increase shape.height, reduce the text, or set content wrap=\"true\" "
                    "autoFit=\"normal-auto-fit\". "
                    "This is an estimate based on font size, line spacing, and wrapped line count."
                ),
            }
        )
    return issues


def estimate_text_visual_bbox(element: dict[str, Any]) -> dict[str, int | float] | None:
    if not is_text_element(element) or not has_text_content(element) or is_decorative_text(element):
        return None

    padding_left = element.get("paddingLeft", 0)
    padding_right = element.get("paddingRight", 0)
    padding_top = element.get("paddingTop", 0)
    padding_bottom = element.get("paddingBottom", 0)
    content_width = max(element["width"] - padding_left - padding_right, 0)
    content_height = max(element["height"] - padding_top - padding_bottom, 0)
    font_size = element["fontSize"] if isinstance(element["fontSize"], (int, float)) else 16
    line_count = estimate_text_line_count(element)
    estimated_width = max(1, estimate_text_max_line_width(element))
    visual_width = estimated_width if element.get("wrap") in {"false", "0"} else min(content_width, estimated_width)
    visual_height = min(content_height, max(1, line_count * font_size * 1.2))
    x = element["x"] + padding_left
    if element.get("textAlign") == "center":
        x += (content_width - visual_width) / 2
    elif element.get("textAlign") == "right":
        x += content_width - visual_width
    y = element["y"] + padding_top
    if element.get("verticalAlign") == "middle":
        y += (content_height - visual_height) / 2
    elif element.get("verticalAlign") == "bottom":
        y += content_height - visual_height
    return {
        "x": x,
        "y": y,
        "width": visual_width,
        "height": visual_height,
    }


def intersection_area(left: dict[str, Any], right: dict[str, Any]) -> int | float:
    width = min(left["x"] + left["width"], right["x"] + right["width"]) - max(left["x"], right["x"])
    height = min(left["y"] + left["height"], right["y"] + right["height"]) - max(left["y"], right["y"])
    if width <= 0 or height <= 0:
        return 0
    return width * height


def intersection_height(left: dict[str, Any], right: dict[str, Any]) -> int | float:
    height = min(left["y"] + left["height"], right["y"] + right["height"]) - max(left["y"], right["y"])
    return max(height, 0)


def intersection_width(left: dict[str, Any], right: dict[str, Any]) -> int | float:
    width = min(left["x"] + left["width"], right["x"] + right["width"]) - max(left["x"], right["x"])
    return max(width, 0)


def element_area(element: dict[str, Any]) -> int | float:
    return max(element["width"], 0) * max(element["height"], 0)


def contains(outer: dict[str, Any], inner: dict[str, Any], tolerance: int | float = 2) -> bool:
    return (
        inner["x"] >= outer["x"] - tolerance
        and inner["y"] >= outer["y"] - tolerance
        and inner["x"] + inner["width"] <= outer["x"] + outer["width"] + tolerance
        and inner["y"] + inner["height"] <= outer["y"] + outer["height"] + tolerance
    )


def is_bottom_layer_full_slide_whiteboard(
    whiteboard: dict[str, Any], other: dict[str, Any], slide_width: int | float, slide_height: int | float
) -> bool:
    return (
        whiteboard["order"] < other["order"]
        and whiteboard["x"] <= 2
        and whiteboard["y"] <= 2
        and whiteboard["width"] >= slide_width - 4
        and whiteboard["height"] >= slide_height - 4
    )


def is_background_container_for_whiteboard(container: dict[str, Any], whiteboard: dict[str, Any]) -> bool:
    if container["order"] > whiteboard["order"]:
        return False
    if is_text_element(container):
        return False
    return contains(container, whiteboard)


def is_template_text_stack(left: dict[str, Any], right: dict[str, Any]) -> bool:
    if not (is_text_element(left) and is_text_element(right)):
        return False
    if not (has_text_content(left) and has_text_content(right)):
        return True
    top, bottom = sorted([left, right], key=lambda element: element["y"])
    top_type = top.get("textType")
    bottom_type = bottom.get("textType")
    allowed_pairs = {
        ("title", "sub-headline"),
        ("title", None),
        ("headline", "headline"),
        ("headline", None),
    }
    if (top_type, bottom_type) not in allowed_pairs:
        return False
    same_column = abs(top["x"] - bottom["x"]) <= 4
    vertical_offset = bottom["y"] - top["y"]
    top_font_size = float(top.get("fontSize", 16))
    return same_column and vertical_offset >= top_font_size * 0.75


def should_flag_horizontal_text_overflow(left: dict[str, Any], right: dict[str, Any]) -> bool:
    if not (is_text_element(left) and is_text_element(right)):
        return False
    if not (has_text_content(left) and has_text_content(right)):
        return False
    if is_template_text_stack(left, right) or is_similar_text_overlay(left, right):
        return False

    source, target = sorted([left, right], key=lambda element: element["x"])
    if source["x"] == target["x"]:
        return False
    wrap_enabled = source.get("wrap") not in {"false", "0"}
    has_horizontal_gap = source["x"] + source["width"] <= target["x"]
    if wrap_enabled and has_horizontal_gap:
        return False
    if source.get("autoFit") == "normal-auto-fit":
        return False
    if source.get("textAlign") in {"center", "right"}:
        return False

    font_size = source["fontSize"] if isinstance(source["fontSize"], (int, float)) else 16
    visual_width = estimate_text_max_line_width(source)
    overflow_width = visual_width - source["width"]
    min_overflow = max(font_size * 1.5, source["width"] * 0.08)
    if overflow_width < min_overflow:
        return False

    intrusion_width = source["x"] + visual_width - target["x"]
    min_intrusion = max(font_size * 1.5, target["width"] * 0.08)
    if intrusion_width < min_intrusion:
        return False

    vertical_overlap = intersection_height(source, target)
    min_vertical_overlap = min(source["height"], target["height"]) * 0.40
    return vertical_overlap >= min_vertical_overlap


def horizontal_text_overflow_measurement(left: dict[str, Any], right: dict[str, Any]) -> dict[str, int | float]:
    source, target = sorted([left, right], key=lambda element: element["x"])
    visual_width = estimate_text_max_line_width(source)
    source_visual_bbox = {"x": source["x"], "y": source["y"], "width": visual_width, "height": source["height"]}
    width = intersection_width(source_visual_bbox, target)
    height = intersection_height(source_visual_bbox, target)
    return {
        "intersection_width": round(width, 3),
        "intersection_height": round(height, 3),
        "intersection_area": round(width * height, 3),
    }


def should_flag_overlap(left: dict[str, Any], right: dict[str, Any]) -> bool:
    if is_text_element(left) and not has_text_content(left):
        return False
    if is_text_element(right) and not has_text_content(right):
        return False
    if is_template_text_stack(left, right):
        return False
    if is_text_element(left) and is_text_element(right):
        if is_similar_text_overlay(left, right):
            return False
        left_visual = estimate_text_visual_bbox(left)
        right_visual = estimate_text_visual_bbox(right)
        if left_visual is None or right_visual is None:
            return False
        overlap_area = intersection_area(left_visual, right_visual)
        if overlap_area <= 0:
            return False
        smaller_area = min(
            left_visual["width"] * left_visual["height"],
            right_visual["width"] * right_visual["height"],
        )
        return smaller_area > 0 and overlap_area / smaller_area >= 0.30
    return False


def build_whiteboard_external_overlap_issue(
    whiteboard: dict[str, Any], overlap_details: list[dict[str, Any]]
) -> dict[str, Any]:
    element_ids = [detail["element"] for detail in overlap_details]
    return {
        "level": "warning",
        "code": "whiteboard_external_overlap",
        "elements": [whiteboard["id"], *element_ids],
        "message": f'whiteboard {whiteboard["id"]} overlaps {len(element_ids)} sibling elements across its boundary',
        "hint": (
            "Treat this as a static whiteboard container-bbox risk, not final visual proof. "
            "After moving or accepting the overlap, use screenshot QA or equivalent rendered visual inspection as "
            "the final authority because XML readback does not include whiteboard SVG/Mermaid internals."
        ),
        "overlaps": overlap_details,
    }


def should_report_whiteboard_overlap(
    whiteboard: dict[str, Any],
    other: dict[str, Any],
    slide_width: int | float,
    slide_height: int | float,
) -> dict[str, Any] | None:
    if other is whiteboard or not intersects(whiteboard, other):
        return None
    if contains(whiteboard, other):
        return None
    if is_bottom_layer_full_slide_whiteboard(whiteboard, other, slide_width, slide_height):
        return None
    if is_background_container_for_whiteboard(other, whiteboard):
        return None

    overlap_width = intersection_width(whiteboard, other)
    overlap_height = intersection_height(whiteboard, other)
    if overlap_width < 8 or overlap_height < 8:
        return None

    other_area = element_area(other)
    if other_area <= 0:
        return None
    overlap_area = overlap_width * overlap_height
    overlap_ratio = overlap_area / other_area
    if overlap_ratio < 0.15:
        return None

    return {
        "element": other["id"],
        "kind": other["kind"],
        "type": other.get("type"),
        "overlap_width": overlap_width,
        "overlap_height": overlap_height,
        "target_overlap_ratio": round(overlap_ratio, 3),
    }


def prune_contained_text_overlap_details(
    overlap_details: list[dict[str, Any]], elements_by_id: dict[str, dict[str, Any]]
) -> list[dict[str, Any]]:
    pruned: list[dict[str, Any]] = []
    for detail in overlap_details:
        element = elements_by_id[detail["element"]]
        if is_text_element(element):
            has_reported_container = any(
                detail["element"] != other_detail["element"]
                and not is_text_element(elements_by_id[other_detail["element"]])
                and contains(elements_by_id[other_detail["element"]], element)
                for other_detail in overlap_details
            )
            if has_reported_container:
                continue
        pruned.append(detail)
    return pruned


def detect_whiteboard_external_overlaps(
    elements: list[dict[str, Any]], slide_width: int | float, slide_height: int | float
) -> list[dict[str, Any]]:
    issues: list[dict[str, Any]] = []
    elements_by_id = {element["id"]: element for element in elements}
    for whiteboard in [element for element in elements if is_whiteboard_element(element)]:
        overlap_details = [
            detail
            for element in elements
            if (
                detail := should_report_whiteboard_overlap(
                    whiteboard,
                    element,
                    slide_width,
                    slide_height,
                )
            )
            is not None
        ]
        overlap_details = prune_contained_text_overlap_details(overlap_details, elements_by_id)
        if overlap_details:
            issues.append(build_whiteboard_external_overlap_issue(whiteboard, overlap_details))
    return issues


def element_canvas_bbox(element: dict[str, Any]) -> dict[str, int | float]:
    bbox = {key: element[key] for key in ("x", "y", "width", "height")}
    rotation = element["rotation"]
    if not isinstance(rotation, (int, float)) or not math.isfinite(rotation):
        rotation = 0
    rotation %= 360
    if math.isclose(rotation, 0, abs_tol=1e-9):
        return bbox
    radians = math.radians(rotation)
    sine = abs(math.sin(radians))
    cosine = abs(math.cos(radians))
    sine = 0 if math.isclose(sine, 0, abs_tol=1e-12) else sine
    cosine = 0 if math.isclose(cosine, 0, abs_tol=1e-12) else cosine
    rotated_width = element["width"] * cosine + element["height"] * sine
    rotated_height = element["width"] * sine + element["height"] * cosine
    return {
        "x": element["x"] - (rotated_width - element["width"]) / 2,
        "y": element["y"] - (rotated_height - element["height"]) / 2,
        "width": rotated_width,
        "height": rotated_height,
    }


def detect_elements_out_of_canvas(
    elements: list[dict[str, Any]], slide_width: int | float, slide_height: int | float
) -> list[dict[str, Any]]:
    issues: list[dict[str, Any]] = []
    for element in elements:
        bbox = element_canvas_bbox(element)
        overflow = {
            "left": max(-bbox["x"], 0),
            "top": max(-bbox["y"], 0),
            "right": max(bbox["x"] + bbox["width"] - slide_width, 0),
            "bottom": max(bbox["y"] + bbox["height"] - slide_height, 0),
        }
        overflow_details = [
            f"{side} by {amount:g}px"
            for side, amount in overflow.items()
            if amount > CANVAS_OVERFLOW_TOLERANCE
        ]
        if not overflow_details:
            continue
        issues.append(
            {
                "level": "error",
                "code": f'{element["kind"]}_out_of_canvas',
                "elements": [element["id"]],
                "canvas": {"width": slide_width, "height": slide_height},
                "bbox": bbox,
                "overflow": overflow,
                "message": (
                    f'{element["kind"]} {element["id"]} exceeds the {slide_width:g}x{slide_height:g} canvas '
                    f'({", ".join(overflow_details)})'
                ),
                "hint": (
                    "Move the table inside the canvas, reduce table.width/table.height, or split the table across "
                    "slides."
                    if element["kind"] == "table"
                    else f'Move the {element["kind"]} inside the canvas or reduce its width/height.'
                ),
            }
        )
    return issues


def extract_table_column_sizes(table_xml: str) -> list[int | float | None]:
    sizes: list[int | float | None] = []
    for match in re.finditer(r"<col\b([^>]*)/?>", table_xml):
        attrs = match.group(1)
        span = extract_numeric_attribute(attrs, "span") or 1
        span_count = int(span) if math.isfinite(span) and span > 0 and float(span).is_integer() else 1
        sizes.extend([extract_numeric_attribute(attrs, "width")] * span_count)
    return sizes


def extract_table_row_sizes(table_xml: str) -> list[int | float | None]:
    return [extract_numeric_attribute(match.group(1), "height") for match in re.finditer(r"<tr\b([^>]*)>", table_xml)]


def resolve_table_dimension(
    table_xml: str,
    declared_size: int | float | None,
    extract_sizes: Any,
    default_size: int | float,
) -> tuple[int | float | None, dict[str, Any] | None]:
    input_sizes = extract_sizes(table_xml)
    if not input_sizes:
        return declared_size, None
    layout = solve_weighted_min_layout(
        input_sizes, default_size, declared_size if is_filled_size(declared_size) else None
    )
    return layout["actual_size"], layout


def format_size(size: int | float) -> str:
    return f"{size:g}"


def detect_table_layout_size_mismatches(elements: list[dict[str, Any]]) -> list[dict[str, Any]]:
    issues: list[dict[str, Any]] = []
    dimensions = {
        "width": ("col", "column widths"),
        "height": ("tr", "row heights"),
    }
    for table in (element for element in elements if element["kind"] == "table"):
        for dimension, (child_tag, child_description) in dimensions.items():
            target_size = table[f"declared_{dimension}"]
            if not is_filled_size(target_size):
                continue
            layout = table["table_layouts"][dimension]
            if layout is None:
                continue
            actual_size = layout["actual_size"]
            if math.isclose(actual_size, target_size, rel_tol=1e-9, abs_tol=1e-9):
                continue
            issues.append(
                {
                    "level": "info",
                    "code": "table_resolved_size_mismatch",
                    "elements": [table["id"]],
                    "dimension": dimension,
                    "declared_size": target_size,
                    "resolved_size": actual_size,
                    "resolved_sizes": layout["final_sizes"],
                    "message": (
                        f'table {table["id"]} declares {dimension}={format_size(target_size)}px, but its '
                        f"{child_description} resolve to {format_size(actual_size)}px"
                    ),
                    "hint": (
                        f"Set table.{dimension} to {format_size(actual_size)}px, or adjust <{child_tag}> sizes "
                        f"so their resolved total matches {format_size(target_size)}px."
                    ),
                }
            )
    return issues


def lint_slide(
    slide_xml: str, slide_number: int, slide_width: int | float = 960, slide_height: int | float = 540
) -> dict[str, Any]:
    elements = extract_elements(slide_xml)
    issues: list[dict[str, Any]] = [
        *detect_whiteboard_external_overlaps(elements, slide_width, slide_height),
        *detect_elements_out_of_canvas(elements, slide_width, slide_height),
        *detect_table_layout_size_mismatches(elements),
        *detect_text_may_overflow_shapes(elements),
        *detect_image_text_occlusions(elements),
    ]

    for index, left in enumerate(elements):
        for right in elements[index + 1 :]:
            horizontal_overflow = should_flag_horizontal_text_overflow(left, right)
            if not horizontal_overflow and (not intersects(left, right) or not should_flag_overlap(left, right)):
                continue
            issues.append(
                {
                    "level": "error",
                    "code": "bbox_overlap",
                    "elements": [left["id"], right["id"]],
                    "message": f'{left["id"]} overlaps {right["id"]}',
                    "hint": "Move or resize the elements so their visual bounds no longer intersect.",
                    **(
                        {"measurement": horizontal_text_overflow_measurement(left, right)}
                        if horizontal_overflow
                        else {}
                    ),
                }
            )

    return {
        "slide_number": slide_number,
        "element_count": len(elements),
        "elements": elements,
        "issues": issues,
    }



MIN_CONTAINER_WIDTH = 140
MIN_CONTAINER_HEIGHT = 160
MIN_SHORT_CARD_HEIGHT = 80
MIN_CONTAINER_AREA = 20_000
MIN_CONTENT_COVERAGE_RATIO = 0.15
MIN_SLIDE_CONTENT_COVERAGE_RATIO = 0.035
MIN_SLIDE_CONTENT_ELEMENT_COUNT = 4
SHORT_CARD_SIZE_TOLERANCE_RATIO = 0.10
MIN_SIMILAR_SHORT_CARD_COUNT = 2
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


def has_similar_short_card_peer(element: dict[str, Any], elements: list[dict[str, Any]]) -> bool:
    return sum(
        other is not element
        and is_visually_rendered(other)
        and other["kind"] == "shape"
        and other["type"] == "rect"
        and other["width"] >= MIN_CONTAINER_WIDTH
        and other["height"] >= MIN_SHORT_CARD_HEIGHT
        and element_area(other) >= MIN_CONTAINER_AREA
        and abs(other["width"] - element["width"]) / max(other["width"], element["width"])
        <= SHORT_CARD_SIZE_TOLERANCE_RATIO
        and abs(other["height"] - element["height"]) / max(other["height"], element["height"])
        <= SHORT_CARD_SIZE_TOLERANCE_RATIO
        for other in elements
    ) >= MIN_SIMILAR_SHORT_CARD_COUNT


def is_layout_container(
    element: dict[str, Any],
    slide_width: int | float,
    slide_height: int | float,
    elements: list[dict[str, Any]] | None = None,
) -> bool:
    has_supported_height = element["height"] >= MIN_CONTAINER_HEIGHT or (
        elements is not None
        and element["height"] >= MIN_SHORT_CARD_HEIGHT
        and has_similar_short_card_peer(element, elements)
    )
    return (
        element["kind"] == "shape"
        and element["type"] == "rect"
        and is_visually_rendered(element)
        and element["width"] >= MIN_CONTAINER_WIDTH
        and has_supported_height
        and element_area(element) >= MIN_CONTAINER_AREA
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
    container_area = element_area(container)
    return any(
        element["kind"] == "img"
        and is_visually_rendered(element)
        and intersection_area(container, element) / max(1, container_area) >= IMAGE_OVERLAY_MATCH_RATIO
        for element in elements
    )


def is_nested_in_layout_panel(
    container: dict[str, Any], elements: list[dict[str, Any]], slide_width: int | float, slide_height: int | float
) -> bool:
    return any(
        element is not container
        and element["kind"] == "shape"
        and element["type"] == "rect"
        and is_visually_rendered(element)
        and is_edge_spanning_layout_panel(element, slide_width, slide_height)
        and contains(element, container, tolerance=DENSITY_CONTAINMENT_TOLERANCE)
        for element in elements
    )


def extract_density_elements(slide_xml: str) -> list[dict[str, Any]]:
    elements = extract_elements(slide_xml)
    elements_by_id = {element["id"]: element for element in elements}
    root = ET.fromstring(slide_xml)
    for node in root.iter():
        if xml_local_name(node.tag) != "shape":
            continue
        element = elements_by_id.get(node.attrib.get("id", ""))
        if element is None:
            continue
        content_node = next(
            (child for child in node if xml_local_name(child.tag) == "content"),
            None,
        )
        paragraphs = (
            [
                " ".join("".join(paragraph.itertext()).split())
                for paragraph in content_node.iter()
                if xml_local_name(paragraph.tag) == "p"
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
        if not has_text_content(element):
            continue
        declared_font_sizes = []
        for descendant in node.iter():
            raw_declared_font_size = descendant.attrib.get("fontSize")
            if raw_declared_font_size is None:
                continue
            try:
                declared_font_sizes.append(float(raw_declared_font_size))
            except ValueError:
                continue
        if declared_font_sizes:
            element["fontSize"] = max(declared_font_sizes)
    for match in re.finditer(r"<icon\b([^>]*)>", slide_xml):
        attrs = match.group(1)
        x = extract_numeric_attribute(attrs, "topLeftX")
        y = extract_numeric_attribute(attrs, "topLeftY")
        width = extract_numeric_attribute(attrs, "width")
        height = extract_numeric_attribute(attrs, "height")
        if any(value is None for value in (x, y, width, height)):
            continue
        icon_alpha = extract_numeric_attribute(attrs, "alpha")
        elements.append(
            {
                "id": extract_attribute(attrs, "id") or f"icon-{len(elements) + 1}",
                "kind": "icon",
                "type": "icon",
                "x": x,
                "y": y,
                "width": width,
                "height": height,
                "rotation": extract_numeric_attribute(attrs, "rotation") or 0,
                "alpha": icon_alpha if icon_alpha is not None else 1,
                "order": len(elements),
            }
        )
    for match in re.finditer(r"<polyline\b([^>]*)>", slide_xml):
        attrs = match.group(1)
        x = extract_numeric_attribute(attrs, "topLeftX")
        y = extract_numeric_attribute(attrs, "topLeftY")
        width = extract_numeric_attribute(attrs, "width")
        height = extract_numeric_attribute(attrs, "height")
        if any(value is None for value in (x, y, width, height)):
            continue
        polyline_alpha = extract_numeric_attribute(attrs, "alpha")
        elements.append(
            {
                "id": extract_attribute(attrs, "id") or f"polyline-{len(elements) + 1}",
                "kind": "polyline",
                "type": "polyline",
                "x": x,
                "y": y,
                "width": width,
                "height": height,
                "rotation": extract_numeric_attribute(attrs, "rotation") or 0,
                "alpha": polyline_alpha if polyline_alpha is not None else 1,
                "order": len(elements),
            }
        )
    for line_element in extract_line_elements(slide_xml):
        line_element["order"] = len(elements)
        elements.append(line_element)
    return elements


def is_visually_rendered(element: dict[str, Any]) -> bool:
    return element.get("alpha", 1) > 0


def visual_bbox(element: dict[str, Any], container: dict[str, Any]) -> dict[str, int | float] | None:
    if not is_visually_rendered(element):
        return None
    if is_text_element(element):
        estimated = estimate_text_visual_bbox(element)
        return clipped_bbox(estimated, container) if estimated else None
    return clipped_bbox(element, container)


def own_text_visual_bbox(container: dict[str, Any]) -> dict[str, int | float] | None:
    if container["kind"] != "shape" or not has_text_content(container):
        return None
    text_proxy = {**container, "type": "text"}
    estimated = estimate_text_visual_bbox(text_proxy)
    return clipped_bbox(estimated, container) if estimated else None


def slide_content_visual_bbox(
    element: dict[str, Any], slide_bbox: dict[str, int | float]
) -> dict[str, int | float] | None:
    if not is_visually_rendered(element):
        return None
    if is_text_element(element):
        estimated = estimate_text_visual_bbox(element)
        return clipped_bbox(estimated, slide_bbox) if estimated else None
    if element["kind"] == "shape" and has_text_content(element):
        estimated = own_text_visual_bbox(element)
        return clipped_bbox(estimated, slide_bbox) if estimated else None
    if element["kind"] == "line":
        # a straight horizontal/vertical line has zero width or height in one axis; clipped_bbox
        # treats zero-area rects as invisible, so pad to its rendered stroke thickness instead.
        return clipped_bbox(line_stroke_bbox(element), slide_bbox)
    if element["kind"] in {"img", "chart", "table", "whiteboard", "icon", "polyline"}:
        return clipped_bbox(element, slide_bbox)
    return None


def line_stroke_bbox(element: dict[str, Any]) -> dict[str, Any]:
    return {**element, "width": max(element["width"], 1), "height": max(element["height"], 1)}


def is_slide_content_present(
    element: dict[str, Any], slide_bbox: dict[str, int | float]
) -> bool:
    # Deliberately permissive, unlike slide_content_visual_bbox: blank_slide is asking "is
    # *anything* rendered here", not the richer "counts toward meaningful content density" bar
    # that sparse_slide_content/sparse_container_content apply. A plain shape with no text (a
    # decorative rect/ellipse/etc.), <undefined>, or any future SXSD data element should all
    # count here — deny-list only what's actually invisible (alpha<=0 or zero on-canvas area)
    # instead of maintaining an allow-list that silently treats unlisted kinds as blank.
    if not is_visually_rendered(element):
        return False
    if (
        element["kind"] == "shape"
        and element["type"] == "rect"
        and not has_text_content(element)
        and element["x"] <= 2
        and element["y"] <= 2
        and element["width"] >= slide_bbox["width"] - 4
        and element["height"] >= slide_bbox["height"] - 4
    ):
        # A full-canvas plain rect is a background panel, not content -- same reasoning as
        # is_layout_container's existing background exclusion. A slide with nothing else on it
        # is still effectively blank.
        return False
    bbox = line_stroke_bbox(element) if element["kind"] == "line" else element
    return clipped_bbox(bbox, slide_bbox) is not None


def is_large_visual_child(element: dict[str, Any], container: dict[str, Any]) -> bool:
    if element["kind"] not in {"img", "chart", "table", "whiteboard"}:
        return False
    if not is_visually_rendered(element):
        return False
    return element_area(element) / element_area(container) >= LARGE_VISUAL_CHILD_RATIO


def detect_sparse_container_content(
    elements: list[dict[str, Any]], slide_number: int, slide_width: int | float, slide_height: int | float
) -> list[dict[str, Any]]:
    issues: list[dict[str, Any]] = []
    for container in (
        element for element in elements if is_layout_container(element, slide_width, slide_height, elements)
    ):
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
            and contains(container, element, tolerance=DENSITY_CONTAINMENT_TOLERANCE)
        ]
        if any(is_large_visual_child(child, container) for child in children):
            continue
        own_text_bbox = own_text_visual_bbox(container)
        rectangles = ([own_text_bbox] if own_text_bbox else []) + [
            bbox for child in children if (bbox := visual_bbox(child, container)) is not None
        ]
        content_area = rectangle_union_area(rectangles) if rectangles else 0
        coverage_ratio = content_area / element_area(container)
        if coverage_ratio >= MIN_CONTENT_COVERAGE_RATIO:
            continue
        issues.append(
            {
                "level": "warning",
                "code": "sparse_container_content",
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
                    "container_area": element_area(container),
                    "visible_content_area": round(content_area, 3),
                    "content_coverage_ratio": round(coverage_ratio, 3),
                    "content_element_count": len(children) + (1 if own_text_bbox else 0),
                },
                "elements": [container["id"], *[child["id"] for child in children]],
            }
        )
    return issues


def detect_sparse_slide_content(
    elements: list[dict[str, Any]], slide_number: int, slide_width: int | float, slide_height: int | float
) -> list[dict[str, Any]]:
    slide_bbox = {"x": 0, "y": 0, "width": slide_width, "height": slide_height}
    content = [
        (element, bbox)
        for element in elements
        if (bbox := slide_content_visual_bbox(element, slide_bbox)) is not None
    ]
    if len(content) < MIN_SLIDE_CONTENT_ELEMENT_COUNT:
        return []
    content_area = rectangle_union_area([bbox for _, bbox in content])
    slide_area = slide_width * slide_height
    coverage_ratio = content_area / slide_area
    if coverage_ratio >= MIN_SLIDE_CONTENT_COVERAGE_RATIO:
        return []
    return [
        {
            "level": "warning",
            "code": "sparse_slide_content",
            "target": {
                "slide_number": slide_number,
                "bbox": slide_bbox,
            },
            "rule": {
                "name": "slide_visible_content_coverage",
                "threshold": MIN_SLIDE_CONTENT_COVERAGE_RATIO,
                "comparison": "content_coverage_ratio < threshold",
            },
            "measurement": {
                "slide_area": slide_area,
                "visible_content_area": round(content_area, 3),
                "content_coverage_ratio": round(coverage_ratio, 3),
                "content_element_count": len(content),
            },
            "elements": [element["id"] for element, _ in content],
        }
    ]


def detect_blank_slide(
    elements: list[dict[str, Any]],
    slide_number: int,
    slide_width: int | float,
    slide_height: int | float,
) -> list[dict[str, Any]]:
    slide_bbox = {"x": 0, "y": 0, "width": slide_width, "height": slide_height}
    visible_elements = [
        element for element in elements if is_slide_content_present(element, slide_bbox)
    ]
    if visible_elements:
        return []
    return [
        {
            "level": "error",
            "code": "blank_slide",
            "schema_version": "2.0",
            "target": {"slide_number": slide_number},
            "rule": {
                "name": "slide_has_visible_content",
                "comparison": "visible_element_count == 0",
            },
            "measurement": {
                "visible_element_count": 0,
                "declared_element_count": len(elements),
            },
            "elements": [element["id"] for element in elements],
            "message": "slide has no visible content beyond empty layout shapes",
            "hint": "Add visible text, an image, a chart, a table, a whiteboard, or an icon before creating the slide.",
        }
    ]



RULE_METADATA: dict[str, dict[str, Any]] = {
    "xml_not_well_formed": {
        "name": "xml_is_well_formed",
        "comparison": "xml_parse_error == false",
    },
    "sml_prefixed_tag": {
        "name": "sml_uses_default_namespace",
        "comparison": "prefixed_sml_tag_count == 0",
    },
    "sxsd_unsupported_tag": {
        "name": "tag_is_supported_by_slides_xml_schema",
        "comparison": "unsupported_tag_count == 0",
    },
    "sxsd_unsupported_attr": {
        "name": "attribute_is_supported_by_slides_xml_schema",
        "comparison": "unsupported_attribute_count == 0",
    },
    "icon_missing_fill_color": {
        "name": "icon_has_visible_fill_color",
        "comparison": "fill_color_present == true",
    },
    "icon_transparent_fill_color": {
        "name": "icon_has_visible_fill_color",
        "comparison": "fill_alpha > 0",
    },
    "iconpark_unsupported_icon_type": {
        "name": "iconpark_type_is_supported",
        "comparison": "icon_type in iconpark_index",
    },
    "bbox_overlap": {
        "name": "text_visual_bounds_do_not_overlap",
        "comparison": "intersection_area == 0",
    },
    "text_may_overflow_shape": {
        "name": "estimated_text_fits_declared_shape",
        "comparison": "estimated_height <= available_height",
    },
    "whiteboard_external_overlap": {
        "name": "whiteboard_does_not_cross_sibling_content",
        "comparison": "external_overlap_count == 0",
    },
    "image_covers_text": {
        "name": "image_does_not_cover_text",
        "comparison": "intersection_area == 0",
    },
    "image_may_cover_vertical_text": {
        "name": "image_vertical_text_occlusion_requires_review",
        "comparison": "intersection_area == 0",
    },
    "table_resolved_size_mismatch": {
        "name": "table_declared_size_matches_resolved_grid",
        "comparison": "declared_size == resolved_size",
    },
    "blank_slide": {
        "name": "slide_has_visible_content",
        "comparison": "visible_element_count > 0",
    },
}


def issue_rule(issue: dict[str, Any]) -> dict[str, Any]:
    if issue.get("rule"):
        return {**issue["rule"], "id": issue["code"]}
    if issue["code"].endswith("_out_of_canvas"):
        return {
            "id": issue["code"],
            "name": "element_stays_within_slide_canvas",
            "comparison": "max(left, top, right, bottom overflow) == 0",
        }
    return {
        "id": issue["code"],
        **RULE_METADATA.get(
            issue["code"],
            {"name": issue["code"], "comparison": "violation_count == 0"},
        ),
    }


def issue_measurement(
    issue: dict[str, Any], elements_by_id: dict[str, dict[str, Any]]
) -> dict[str, Any]:
    if issue.get("measurement") is not None:
        return issue["measurement"]
    if issue["code"] == "bbox_overlap" and len(issue.get("elements", [])) == 2:
        left = elements_by_id.get(issue["elements"][0])
        right = elements_by_id.get(issue["elements"][1])
        if left and right:
            left_box = (estimate_text_visual_bbox(left) if is_text_element(left) else None) or left
            right_box = (estimate_text_visual_bbox(right) if is_text_element(right) else None) or right
            width = intersection_width(left_box, right_box)
            height = intersection_height(left_box, right_box)
            return {
                "intersection_width": round(width, 3),
                "intersection_height": round(height, 3),
                "intersection_area": round(width * height, 3),
            }
    if issue["code"].endswith("_out_of_canvas"):
        return {
            "canvas": issue.get("canvas"),
            "bbox": issue.get("bbox"),
            "overflow": issue.get("overflow"),
        }
    measurement_keys = (
        "line",
        "column",
        "tag",
        "attr",
        "iconType",
        "line_count",
        "line_height",
        "estimated_height",
        "available_height",
        "overflow",
        "dimension",
        "declared_size",
        "resolved_size",
        "resolved_sizes",
        "overlaps",
    )
    measured = {key: issue[key] for key in measurement_keys if key in issue}
    return measured or {"violation_count": 1}


def related_object(element: dict[str, Any]) -> dict[str, Any]:
    return {
        "element_id": element["id"],
        "kind": element["kind"],
        "type": element["type"],
        "bbox": {key: element[key] for key in ("x", "y", "width", "height")},
    }


def extract_line_elements(slide_xml: str) -> list[dict[str, Any]]:
    elements: list[dict[str, Any]] = []
    for match in re.finditer(r"<line\b([^>]*)>", slide_xml):
        attrs = match.group(1)
        start_x = extract_numeric_attribute(attrs, "startX")
        start_y = extract_numeric_attribute(attrs, "startY")
        end_x = extract_numeric_attribute(attrs, "endX")
        end_y = extract_numeric_attribute(attrs, "endY")
        if any(value is None for value in (start_x, start_y, end_x, end_y)):
            continue
        line_alpha = extract_numeric_attribute(attrs, "alpha")
        elements.append(
            {
                "id": extract_attribute(attrs, "id") or f"line-{len(elements) + 1}",
                "kind": "line",
                "type": "line",
                "x": min(start_x, end_x),
                "y": min(start_y, end_y),
                "width": abs(end_x - start_x),
                "height": abs(end_y - start_y),
                "rotation": 0,
                "alpha": line_alpha if line_alpha is not None else 1,
                "order": len(elements),
            }
        )
    return elements


def normalize_issue(
    issue: dict[str, Any],
    slide_number: int | None,
    elements_by_id: dict[str, dict[str, Any]],
) -> dict[str, Any]:
    normalized = dict(issue)
    if normalized.get("level") == "info":
        normalized["level"] = "warning"
    element_ids = list(dict.fromkeys(normalized.get("elements", [])))
    normalized["schema_version"] = "2.0"
    normalized["element_ids"] = element_ids
    normalized["target"] = {
        **({"slide_number": slide_number} if slide_number is not None else {}),
        **normalized.get("target", {}),
    }
    normalized["rule"] = issue_rule(normalized)
    normalized["measurement"] = issue_measurement(normalized, elements_by_id)
    normalized["related_objects"] = [
        related_object(elements_by_id[element_id])
        for element_id in element_ids
        if element_id in elements_by_id
    ]
    if normalized["code"] == "sparse_container_content":
        ratio = normalized["measurement"]["content_coverage_ratio"]
        threshold = normalized["rule"]["threshold"]
        container_id = normalized["target"].get("container_id", "unknown")
        normalized.setdefault(
            "message",
            f"large card {container_id} content coverage {ratio:.1%} is below {threshold:.1%}",
        )
        normalized.setdefault(
            "hint",
            "Review the rendered screenshot; add or enlarge meaningful content if the whitespace is not intentional.",
        )
    elif normalized["code"] == "sparse_slide_content":
        ratio = normalized["measurement"]["content_coverage_ratio"]
        threshold = normalized["rule"]["threshold"]
        normalized.setdefault(
            "message",
            f"slide visible content coverage {ratio:.1%} is below {threshold:.1%}",
        )
        normalized.setdefault(
            "hint",
            "Review the rendered screenshot to decide whether the page is intentionally sparse.",
        )
    else:
        normalized.setdefault("message", normalized["code"].replace("_", " "))
    normalized.setdefault(
        "hint", "Inspect the reported elements and adjust them to satisfy the rule comparison."
    )
    return normalized


def slide_status(errors: list[dict[str, Any]], warnings: list[dict[str, Any]]) -> str:
    if errors:
        return "blocked"
    if warnings:
        return "needs_screenshot_review"
    return "passed"


def build_result(
    source_path: str | None,
    slide_size: dict[str, int | float],
    top_level_issues: list[dict[str, Any]],
    slides: list[dict[str, Any]],
) -> dict[str, Any]:
    document_errors = [issue for issue in top_level_issues if issue["level"] == "error"]
    document_warnings = [issue for issue in top_level_issues if issue["level"] == "warning"]
    error_count = len(document_errors) + sum(len(slide["errors"]) for slide in slides)
    warning_count = len(document_warnings) + sum(len(slide["warnings"]) for slide in slides)
    all_errors = document_errors + [issue for slide in slides for issue in slide["errors"]]
    all_warnings = document_warnings + [issue for slide in slides for issue in slide["warnings"]]
    status = slide_status(all_errors, all_warnings)
    result: dict[str, Any] = {
        "schema_version": "2.0",
        "tool": "xml_text_overlap_lint",
        "file": source_path,
        "slide_size": slide_size,
        "summary": {
            "slide_count": len(slides),
            "error_count": error_count,
            "warning_count": warning_count,
            "status": status,
            "release_ready": error_count == 0,
            "screenshot_review_required": warning_count > 0,
        },
        "document": {
            "errors": document_errors,
            "warnings": document_warnings,
        },
        "slides": slides,
    }
    if top_level_issues:
        result["issues"] = top_level_issues
    return result


def lint_xml(xml: str, source_path: str | None = None) -> dict[str, Any]:
    root, xml_error = parse_xml_root(xml)
    if xml_error:
        issue = normalize_issue(xml_error, None, {})
        return build_result(
            source_path,
            {"width": 960, "height": 540},
            [issue],
            [],
        )
    if root is None:
        raise AssertionError("parse_xml_root must return a root or error")

    namespace_issues = validate_sml_tag_prefixes(xml)
    sxsd_issues = validate_sxsd_tag_attributes(root)
    iconpark_issues = validate_iconpark_icon_types(root)
    top_level_issues = [
        normalize_issue(issue, None, {})
        for issue in [*namespace_issues, *sxsd_issues, *iconpark_issues]
    ]
    if any(issue["level"] == "error" for issue in top_level_issues):
        return build_result(
            source_path,
            {"width": 960, "height": 540},
            top_level_issues,
            [],
        )

    presentation = parse_presentation(xml)
    slides: list[dict[str, Any]] = []
    for index, slide_xml in enumerate(presentation["slides"]):
        slide_number = index + 1
        geometry = lint_slide(
            slide_xml,
            slide_number,
            presentation["width"],
            presentation["height"],
        )
        density_elements = extract_density_elements(slide_xml)
        extra_elements = [
            element for element in density_elements if element["kind"] in {"icon", "polyline", "line"}
        ]
        elements_by_id = {
            element["id"]: element for element in [*density_elements, *extra_elements]
        }
        # geometry["elements"] are the exact objects should_flag_overlap/detect_elements_out_of_canvas
        # decided with inside lint_slide; prefer them so measurement/related_objects stay consistent
        # with whatever actually triggered the issue, instead of density_elements' separate re-parse.
        elements_by_id.update({element["id"]: element for element in geometry["elements"]})
        extra_overflow_issues = detect_elements_out_of_canvas(
            extra_elements,
            presentation["width"],
            presentation["height"],
        )
        raw_issues = [
            *geometry["issues"],
            *extra_overflow_issues,
            *detect_blank_slide(
                density_elements,
                slide_number,
                presentation["width"],
                presentation["height"],
            ),
            *detect_sparse_container_content(
                density_elements,
                slide_number,
                presentation["width"],
                presentation["height"],
            ),
            *detect_sparse_slide_content(
                density_elements,
                slide_number,
                presentation["width"],
                presentation["height"],
            ),
        ]
        issues = [
            normalize_issue(issue, slide_number, elements_by_id)
            for issue in raw_issues
        ]
        errors = [issue for issue in issues if issue["level"] == "error"]
        warnings = [issue for issue in issues if issue["level"] == "warning"]
        slides.append(
            {
                "slide_number": slide_number,
                "status": slide_status(errors, warnings),
                "element_count": len(elements_by_id),
                "errors": errors,
                "warnings": warnings,
                "issues": issues,
            }
        )

    return build_result(
        source_path,
        {"width": presentation["width"], "height": presentation["height"]},
        top_level_issues,
        slides,
    )


def print_usage() -> None:
    print("Usage:\n  python3 xml_text_overlap_lint.py --input <presentation.xml>", file=sys.stderr)


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
    except XmlLayoutLintError as error:
        print(f"xml-text-overlap-lint error: {error}", file=sys.stderr)
        raise SystemExit(1) from error
