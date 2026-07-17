#!/usr/bin/env python3
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

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
DEFAULT_TABLE_COLUMN_WIDTH = 110
DEFAULT_TABLE_ROW_HEIGHT = 37
_SXSD_TAG_ATTRIBUTES_CACHE: dict[str, set[str]] | None = None
_ICONPARK_ICON_TYPES_CACHE: set[str] | None = None


class XmlTextOverlapLintError(Exception):
    pass


def fail(message: str) -> None:
    raise XmlTextOverlapLintError(message)


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


def strip_xml_paragraphs(value: str) -> str:
    paragraphs = re.findall(r"<p\b[^>]*>([\s\S]*?)</p\s*>", value)
    if paragraphs:
        return "\n".join(strip_xml(paragraph) for paragraph in paragraphs)
    return strip_xml(value)


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


def should_skip_sxsd_attribute(attr_name: str) -> bool:
    return attr_name in SERVER_FILLED_SXSD_ATTRS


def validate_sxsd_tag_attributes(root: ET.Element) -> list[dict[str, Any]]:
    tag_attributes = load_sxsd_tag_attributes()
    supported_tags = set(tag_attributes)
    issues: list[dict[str, Any]] = []

    def visit(element: ET.Element, ancestors: list[str], path: str) -> None:
        if should_skip_sxsd_subtree(element, ancestors):
            return

        tag_name = xml_local_name(element.tag)
        current_path = f"{path}/{tag_name}" if path else tag_name
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
                if should_skip_sxsd_attribute(attr_name):
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
        content = ""
        if kind in {"shape", "table"}:
            close_index = slide_xml.find(f"</{kind}>", match.end())
            if close_index != -1:
                content = slide_xml[match.end() : close_index]

        element_id = extract_attribute(attrs, "id") or f"{kind}-{len(elements) + 1}"
        x = extract_numeric_attribute(attrs, "topLeftX")
        y = extract_numeric_attribute(attrs, "topLeftY")
        width = extract_numeric_attribute(attrs, "width")
        height = extract_numeric_attribute(attrs, "height")
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
                element.update(
                    {
                        "textType": extract_attribute(content, "textType"),
                        "textAlign": extract_attribute(content, "textAlign"),
                        "autoFit": extract_attribute(content, "autoFit"),
                        "fontSize": float(
                            extract_attribute(content, "fontSize") or extract_attribute(attrs, "fontSize") or 16
                        ),
                        "text": strip_xml_paragraphs(content),
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


def estimate_text_line_count(element: dict[str, Any]) -> int:
    font_size = element["fontSize"] if isinstance(element["fontSize"], (int, float)) else 16
    paragraphs = [paragraph for paragraph in re.split(r"\n+", element["text"]) if paragraph]
    line_count = 0
    for paragraph in paragraphs:
        logical_width = max(estimate_text_width(paragraph, font_size), 1)
        line_count += max(1, math.ceil(logical_width / max(element["width"], 1)))
    return max(line_count, 1)


def estimate_text_visual_bbox(element: dict[str, Any]) -> dict[str, int | float] | None:
    if not is_text_element(element) or not has_text_content(element) or is_decorative_text(element):
        return None

    font_size = element["fontSize"] if isinstance(element["fontSize"], (int, float)) else 16
    line_count = estimate_text_line_count(element)
    visual_width = min(element["width"], max(1, estimate_text_max_line_width(element)))
    visual_height = min(element["height"], max(1, line_count * font_size * 1.2))
    return {
        "x": element["x"],
        "y": element["y"],
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


def detect_table_out_of_canvas(
    elements: list[dict[str, Any]], slide_width: int | float, slide_height: int | float
) -> list[dict[str, Any]]:
    issues: list[dict[str, Any]] = []
    for table in (element for element in elements if element["kind"] == "table"):
        overflow = {
            "left": max(-table["x"], 0),
            "top": max(-table["y"], 0),
            "right": max(table["x"] + table["width"] - slide_width, 0),
            "bottom": max(table["y"] + table["height"] - slide_height, 0),
        }
        overflow_details = [
            f"{side} by {amount:g}px" for side, amount in overflow.items() if amount > 0
        ]
        if not overflow_details:
            continue
        issues.append(
            {
                "level": "error",
                "code": "table_out_of_canvas",
                "elements": [table["id"]],
                "canvas": {"width": slide_width, "height": slide_height},
                "bbox": {
                    "x": table["x"],
                    "y": table["y"],
                    "width": table["width"],
                    "height": table["height"],
                },
                "overflow": overflow,
                "message": (
                    f'table {table["id"]} exceeds the {slide_width:g}x{slide_height:g} canvas '
                    f'({", ".join(overflow_details)})'
                ),
                "hint": (
                    "Move the table inside the canvas, reduce table.width/table.height, or split the table across "
                    "slides."
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
        *detect_table_out_of_canvas(elements, slide_width, slide_height),
        *detect_table_layout_size_mismatches(elements),
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
                }
            )

    return {"slide_number": slide_number, "element_count": len(elements), "issues": issues}


def lint_xml(xml: str, source_path: str | None = None) -> dict[str, Any]:
    root, xml_error = parse_xml_root(xml)
    if xml_error:
        return {
            "file": source_path,
            "slide_size": {"width": 960, "height": 540},
            "summary": {"slide_count": 0, "error_count": 1, "warning_count": 0, "info_count": 0},
            "issues": [xml_error],
            "slides": [],
        }

    namespace_issues = validate_sml_tag_prefixes(xml)
    sxsd_issues = validate_sxsd_tag_attributes(root) if root is not None else []
    iconpark_issues = validate_iconpark_icon_types(root) if root is not None else []
    top_level_issues = [*namespace_issues, *sxsd_issues, *iconpark_issues]
    if namespace_issues:
        error_count = sum(1 for issue in top_level_issues if issue["level"] == "error")
        warning_count = sum(1 for issue in top_level_issues if issue["level"] == "warning")
        info_count = sum(1 for issue in top_level_issues if issue["level"] == "info")
        return {
            "file": source_path,
            "slide_size": {"width": 960, "height": 540},
            "summary": {
                "slide_count": 0,
                "error_count": error_count,
                "warning_count": warning_count,
                "info_count": info_count,
            },
            "issues": top_level_issues,
            "slides": [],
        }
    presentation = parse_presentation(xml)
    slides = [
        lint_slide(slide_xml, index + 1, presentation["width"], presentation["height"])
        for index, slide_xml in enumerate(presentation["slides"])
    ]
    error_count = sum(1 for issue in top_level_issues if issue["level"] == "error")
    error_count += sum(1 for slide in slides for issue in slide["issues"] if issue["level"] == "error")
    warning_count = sum(1 for issue in top_level_issues if issue["level"] == "warning")
    warning_count += sum(1 for slide in slides for issue in slide["issues"] if issue["level"] == "warning")
    info_count = sum(1 for issue in top_level_issues if issue["level"] == "info")
    info_count += sum(1 for slide in slides for issue in slide["issues"] if issue["level"] == "info")
    result = {
        "file": source_path,
        "slide_size": {"width": presentation["width"], "height": presentation["height"]},
        "summary": {
            "slide_count": len(slides),
            "error_count": error_count,
            "warning_count": warning_count,
            "info_count": info_count,
        },
        "slides": slides,
    }
    if top_level_issues:
        result["issues"] = top_level_issues
    return result


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
    except XmlTextOverlapLintError as error:
        print(f"xml-text-overlap-lint error: {error}", file=sys.stderr)
        raise SystemExit(1) from error
