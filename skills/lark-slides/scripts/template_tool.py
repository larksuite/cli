#!/usr/bin/env python3
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

from __future__ import annotations

import json
import re
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


SKILL_ROOT = Path(__file__).resolve().parent.parent
ASSETS_DIR = SKILL_ROOT / "assets"
REFERENCES_DIR = SKILL_ROOT / "references"
TEMPLATES_DIR = ASSETS_DIR / "templates"
CATALOG_PATH = REFERENCES_DIR / "template-catalog.md"
DEFAULT_INDEX_PATH = REFERENCES_DIR / "template-index.json"
LIGHTWEIGHT_INDEX_SCHEMA_VERSION = "1.1.0"


class TemplateToolError(Exception):
    pass


def fail(message: str) -> None:
    raise TemplateToolError(message)


def read_file(file_path: str | Path) -> str:
    return Path(file_path).read_text(encoding="utf-8")


def normalize_whitespace(value: str) -> str:
    return re.sub(r"\s+", " ", value).strip()


def strip_markdown(value: str) -> str:
    return normalize_whitespace(
        re.sub(r"^-\s*", "", re.sub(r"`([^`]+)`", r"\1", value.replace("**", "")))
    )


def strip_xml(value: str) -> str:
    stripped = re.sub(r"<!\[CDATA\[([\s\S]*?)\]\]>", r"\1", value)
    stripped = re.sub(r"<[^>]+>", " ", stripped)
    stripped = stripped.replace("&nbsp;", " ")
    stripped = stripped.replace("&amp;", "&")
    stripped = stripped.replace("&lt;", "<")
    stripped = stripped.replace("&gt;", ">")
    stripped = stripped.replace("&quot;", '"')
    stripped = stripped.replace("&#39;", "'")
    return normalize_whitespace(stripped)


def compact_object(value: dict[str, Any]) -> dict[str, Any]:
    return {key: entry for key, entry in value.items() if entry is not None}


def tokenize_query(value: str) -> list[str]:
    normalized = normalize_whitespace(value.lower())
    if not normalized:
        return []

    def append_unique(target: list[str], token: str) -> None:
        token = token.strip()
        if token and token not in target:
            target.append(token)

    tokens: list[str] = []
    for item in [item.strip() for item in re.split(r"[\s,/|，。；;：:（）()【】\[\]《》<>]+", normalized) if item.strip()]:
        append_unique(tokens, item)

    # Chinese prompts are often complete sentences without separators, e.g.
    # "帮我做一个季度工作汇报PPT". Add CJK n-grams so domain phrases such as
    # "工作汇报" or "季度复盘" can still match catalog metadata.
    for phrase in re.findall(r"[\u3400-\u9fff]+", normalized):
        if len(phrase) < 2:
            continue
        max_size = min(6, len(phrase))
        for size in range(max_size, 1, -1):
            for start in range(0, len(phrase) - size + 1):
                append_unique(tokens, phrase[start : start + size])

    synonym_tokens = {
        "浅色": ["light"],
        "白底": ["light"],
        "明亮": ["light"],
        "深色": ["dark"],
        "黑底": ["dark"],
        "暗色": ["dark"],
        "多彩": ["colorful"],
        "活泼": ["colorful", "casual"],
        "正式": ["formal"],
        "商务": ["formal"],
        "轻松": ["casual"],
        "创意": ["creative"],
    }
    for token in list(tokens):
        for keyword, aliases in synonym_tokens.items():
            if keyword in token:
                for alias in aliases:
                    append_unique(tokens, alias)

    unique_tokens: list[str] = []
    for token in tokens or [normalized]:
        if token not in unique_tokens:
            unique_tokens.append(token)
    return unique_tokens


def parse_range_spec(range_spec: str) -> list[int]:
    if not range_spec or not range_spec.strip():
        fail("range is required")
    numbers: set[int] = set()
    for part in range_spec.split(","):
        trimmed = part.strip()
        if not trimmed:
            continue
        match = re.fullmatch(r"(\d+)(?:-(\d+))?", trimmed)
        if not match:
            fail(f"invalid range token: {trimmed}")
        start = int(match.group(1))
        end = int(match.group(2) or match.group(1))
        if start < 1 or end < start:
            fail(f"invalid range token: {trimmed}")
        for index in range(start, end + 1):
            numbers.add(index)
    return sorted(numbers)


def compress_numbers(numbers: list[int]) -> str:
    if not numbers:
        return ""
    parts: list[str] = []
    start = numbers[0]
    previous = numbers[0]
    for current in numbers[1:]:
        if current == previous + 1:
            previous = current
            continue
        parts.append(f"{start}" if start == previous else f"{start}-{previous}")
        start = current
        previous = current
    parts.append(f"{start}" if start == previous else f"{start}-{previous}")
    return ",".join(parts)


def slice_array(items: list[Any], limit: int) -> list[Any]:
    return items[: max(limit, 0)]


def count_tag(xml: str, tag_name: str) -> int:
    return len(re.findall(fr"<{tag_name}\b", xml))


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


def sort_regions(regions: list[dict[str, Any]]) -> list[dict[str, Any]]:
    return sorted(regions, key=lambda region: (region["y"], region["x"], region["kind"]))


def extract_slide_regions(slide_xml: str) -> list[dict[str, Any]]:
    regions: list[dict[str, Any]] = []
    for match in re.finditer(r"<shape\b([^>]*)>([\s\S]*?)</shape>", slide_xml):
        attrs, content = match.group(1), match.group(2)
        x = extract_numeric_attribute(attrs, "topLeftX")
        y = extract_numeric_attribute(attrs, "topLeftY")
        width = extract_numeric_attribute(attrs, "width")
        height = extract_numeric_attribute(attrs, "height")
        if all(value is not None for value in [x, y, width, height]):
            text_type_match = re.search(r'textType="([^"]+)"', content)
            regions.append(
                {
                    "kind": "shape",
                    "type": extract_attribute(attrs, "type") or "shape",
                    "text_type": text_type_match.group(1) if text_type_match else None,
                    "x": x,
                    "y": y,
                    "width": width,
                    "height": height,
                    "area": width * height,
                }
            )

    for match in re.finditer(r"<(img|table|chart)\b([^>]*)/?>", slide_xml):
        kind, attrs = match.group(1), match.group(2)
        x = extract_numeric_attribute(attrs, "topLeftX")
        y = extract_numeric_attribute(attrs, "topLeftY")
        width = extract_numeric_attribute(attrs, "width")
        height = extract_numeric_attribute(attrs, "height")
        if all(value is not None for value in [x, y, width, height]):
            regions.append(
                {
                    "kind": kind,
                    "type": kind,
                    "text_type": None,
                    "x": x,
                    "y": y,
                    "width": width,
                    "height": height,
                    "area": width * height,
                }
            )
    return sort_regions(regions)


def build_bbox_summary(
    regions: list[dict[str, Any]], slide_width: int | float | None, slide_height: int | float | None
) -> dict[str, Any]:
    if not regions:
        return {
            "region_count": 0,
            "occupied_bounds": None,
            "regions": [],
            "canvas": compact_object({"width": slide_width, "height": slide_height}),
        }

    min_x = regions[0]["x"]
    min_y = regions[0]["y"]
    max_x = regions[0]["x"] + regions[0]["width"]
    max_y = regions[0]["y"] + regions[0]["height"]
    for region in regions:
        min_x = min(min_x, region["x"])
        min_y = min(min_y, region["y"])
        max_x = max(max_x, region["x"] + region["width"])
        max_y = max(max_y, region["y"] + region["height"])

    return {
        "region_count": len(regions),
        "canvas": compact_object({"width": slide_width, "height": slide_height}),
        "occupied_bounds": {
            "x": min_x,
            "y": min_y,
            "width": max_x - min_x,
            "height": max_y - min_y,
        },
        "regions": slice_array(
            [
                compact_object(
                    {
                        "id": f'{region["kind"]}-{index + 1}',
                        "kind": region["kind"],
                        "type": region["type"],
                        "text_type": region.get("text_type"),
                        "x": region["x"],
                        "y": region["y"],
                        "width": region["width"],
                        "height": region["height"],
                    }
                )
                for index, region in enumerate(regions)
            ],
            8,
        ),
    }


def build_editable_regions(regions: list[dict[str, Any]]) -> list[dict[str, Any]]:
    sorted_regions = sorted(regions, key=lambda region: region["area"], reverse=True)
    return slice_array(
        [
            compact_object(
                {
                    "id": f'{region["kind"]}-{index + 1}',
                    "kind": region["kind"],
                    "role": "image"
                    if region["kind"] == "img"
                    else region.get("text_type") or region["type"] or region["kind"],
                    "x": region["x"],
                    "y": region["y"],
                    "width": region["width"],
                    "height": region["height"],
                }
            )
            for index, region in enumerate(sorted_regions)
        ],
        8,
    )


def detect_slide_layout_tags(
    slide_xml: str, regions: list[dict[str, Any]], slide_width: int | float | None, slide_height: int | float | None
) -> list[str]:
    tags: set[str] = set()
    text_regions = [region for region in regions if region["kind"] == "shape" and region["type"] == "text"]
    image_regions = [region for region in regions if region["kind"] == "img"]
    table_regions = [region for region in regions if region["kind"] == "table"]

    if table_regions:
        tags.add("comparison-table")

    if "<fillImg" in slide_xml:
        tags.add("image-backed")
        if text_regions:
            tags.add("full-bleed-image-caption")

    if text_regions and len(image_regions) == 1:
        biggest_text = sorted(text_regions, key=lambda region: region["area"], reverse=True)[0]
        biggest_image = image_regions[0]
        if (
            slide_width and biggest_image["width"] >= slide_width * 0.75
        ) or (
            slide_height and biggest_image["height"] >= slide_height * 0.75
        ):
            tags.add("full-bleed-image-caption")
        elif biggest_text["x"] <= biggest_image["x"]:
            tags.add("hero-text-left-image-right")
        else:
            tags.add("hero-image-left-text-right")

    if len(text_regions) >= 4 and not image_regions:
        distinct_x = len({round(region["x"] / 20) for region in text_regions})
        distinct_y = len({round(region["y"] / 20) for region in text_regions})
        if distinct_x >= 2 and distinct_y >= 2:
            tags.add("2x2-metric-cards")

    if len(text_regions) >= 2 and not image_regions:
        width = slide_width or 960
        left = any(region["x"] < width / 2 - 40 for region in text_regions)
        right = any(region["x"] + region["width"] > width / 2 + 40 for region in text_regions)
        if left and right:
            tags.add("two-column-text")

    if len(text_regions) <= 2 and not image_regions:
        top_most = sorted(text_regions, key=lambda region: region["y"])[0] if text_regions else None
        if top_most and top_most["y"] <= 120 and top_most["height"] <= 140:
            tags.add("section-divider")

    if not tags and text_regions:
        tags.add("text-focused")

    return sorted(tags)


def parse_theme_summary(theme_xml: str | None) -> dict[str, Any]:
    if not theme_xml:
        return {"has_theme_node": False, "text_styles": []}

    text_styles_block = re.search(r"<textStyles>([\s\S]*?)</textStyles>", theme_xml)
    text_styles: list[dict[str, Any]] = []
    if text_styles_block:
        for match in re.finditer(r"<(title|headline|sub-headline|body|caption)\b([^>]*)/?>", text_styles_block.group(1)):
            text_styles.append(
                compact_object(
                    {
                        "text_type": match.group(1),
                        "font_color": extract_attribute(match.group(2), "fontColor"),
                        "font_size": extract_attribute(match.group(2), "fontSize"),
                        "font_family": extract_attribute(match.group(2), "fontFamily"),
                    }
                )
            )
    return {"has_theme_node": True, "text_styles": text_styles}


def extract_background_hint(slide_xml: str) -> str | None:
    fill_color_match = re.search(r'<fillColor color="([^"]+)"', slide_xml)
    if fill_color_match:
        return fill_color_match.group(1)
    fill_img_match = re.search(r'<fillImg src="([^"]+)"', slide_xml)
    if fill_img_match:
        return f"fillImg:{fill_img_match.group(1)}"
    return None


def extract_title_hint(slide_xml: str) -> dict[str, str] | None:
    type_priority = {"title": 5, "headline": 4, "sub-headline": 3, "body": 2, "caption": 1}
    content_pattern = re.compile(
        r'<content\b([^>]*)textType="(title|headline|sub-headline|body|caption)"([^>]*)>([\s\S]*?)</content>'
    )
    candidates: list[dict[str, Any]] = []
    for match in content_pattern.finditer(slide_xml):
        attrs = f"{match.group(1)} {match.group(3)}"
        text = strip_xml(match.group(4))
        if text:
            candidates.append(
                {
                    "text_type": match.group(2),
                    "text": text[:80],
                    "font_size": int(extract_attribute(attrs, "fontSize") or "0"),
                    "priority": type_priority.get(match.group(2), 0),
                }
            )
    candidates.sort(key=lambda item: (-item["priority"], -item["font_size"], -len(item["text"])))
    if candidates:
        return {"text_type": candidates[0]["text_type"], "text": candidates[0]["text"]}
    return None


def summarize_slide(slide_xml: str, slide_number: int, presentation_info: dict[str, Any] | None = None) -> dict[str, Any]:
    presentation_info = presentation_info or {}
    raw_width = presentation_info.get("width")
    raw_height = presentation_info.get("height")
    slide_width = int(float(raw_width)) if raw_width else None
    slide_height = int(float(raw_height)) if raw_height else None
    regions = extract_slide_regions(slide_xml)
    return {
        "slide_number": slide_number,
        "title_hint": extract_title_hint(slide_xml),
        "background_hint": extract_background_hint(slide_xml),
        "layout_tags": detect_slide_layout_tags(slide_xml, regions, slide_width, slide_height),
        "bbox_summary": build_bbox_summary(regions, slide_width, slide_height),
        "editable_regions": build_editable_regions(regions),
        "element_counts": {
            "shape": count_tag(slide_xml, "shape"),
            "img": count_tag(slide_xml, "img"),
            "table": count_tag(slide_xml, "table"),
            "chart": count_tag(slide_xml, "chart"),
            "icon": count_tag(slide_xml, "icon"),
            "line": count_tag(slide_xml, "line"),
            "polyline": count_tag(slide_xml, "polyline"),
        },
    }


def aggregate_slides(slide_summaries: list[dict[str, Any]]) -> dict[str, Any]:
    totals = {"shape": 0, "img": 0, "table": 0, "chart": 0, "icon": 0, "line": 0, "polyline": 0}
    title_hints: list[str] = []
    background_hints: list[str] = []
    layout_tags: list[str] = []
    for slide in slide_summaries:
        for key, value in slide["element_counts"].items():
            totals[key] += value
        if slide.get("title_hint") and slide["title_hint"]["text"] not in title_hints:
            title_hints.append(slide["title_hint"]["text"])
        if slide.get("background_hint") and slide["background_hint"] not in background_hints:
            background_hints.append(slide["background_hint"])
        for tag in slide.get("layout_tags") or []:
            if tag not in layout_tags:
                layout_tags.append(tag)
    return {
        "slide_count": len(slide_summaries),
        "title_hints": slice_array(title_hints, 4),
        "background_hints": slice_array(background_hints, 4),
        "layout_tags": layout_tags,
        "element_totals": totals,
    }


def parse_template_xml(template_path: str | Path) -> dict[str, Any]:
    xml = read_file(template_path)
    presentation_match = re.search(r"<presentation\b([^>]*)>", xml)
    if not presentation_match:
        fail(f"template missing presentation root: {template_path}")
    opening_tag = presentation_match.group(0)
    title_xml_match = re.search(r"<title>[\s\S]*?</title>", xml)
    theme_xml_match = re.search(r"<theme>[\s\S]*?</theme>", xml)
    slides = re.findall(r"<slide\b[\s\S]*?</slide>", xml)
    slide_summaries = [
        summarize_slide(
            slide_xml,
            index + 1,
            {
                "width": extract_attribute(presentation_match.group(1), "width"),
                "height": extract_attribute(presentation_match.group(1), "height"),
            },
        )
        for index, slide_xml in enumerate(slides)
    ]
    return {
        "xml": xml,
        "opening_tag": opening_tag,
        "width": extract_attribute(presentation_match.group(1), "width"),
        "height": extract_attribute(presentation_match.group(1), "height"),
        "title_xml": title_xml_match.group(0) if title_xml_match else None,
        "title_text": strip_xml(title_xml_match.group(0)) if title_xml_match else None,
        "theme_xml": theme_xml_match.group(0) if theme_xml_match else None,
        "theme_summary": parse_theme_summary(theme_xml_match.group(0) if theme_xml_match else None),
        "slides": slides,
        "slide_summaries": slide_summaries,
    }


def finalize_catalog_entry(entry: dict[str, Any] | None) -> dict[str, Any] | None:
    if not entry:
        return None
    filename_stem = re.sub(r"\.xml$", "", entry["filename"])
    template_id = f'{entry["category"]}--{filename_stem}'
    return {
        "template_id": template_id,
        "filename": f"{template_id}.xml",
        "category": entry["category"],
        "category_label": entry["category_label"],
        "scene": entry["scene"],
        "is_general_template": entry["is_general_template"],
        "catalog_slide_count": entry["catalog_slide_count"],
        "tone": entry["tone"],
        "formality": entry["formality"],
        "palette": entry["palette"],
        "structure": entry["structure"],
        "page_types": entry["page_types"],
        "use_cases": entry["use_cases"],
        "ranges": entry["ranges"],
    }


def parse_catalog(catalog_path: str | Path = CATALOG_PATH) -> list[dict[str, Any]]:
    lines = read_file(catalog_path).splitlines()
    entries: list[dict[str, Any]] = []
    current_category: str | None = None
    current_category_label: str | None = None
    current_entry: dict[str, Any] | None = None

    def push_current() -> None:
        nonlocal current_entry
        finalized = finalize_catalog_entry(current_entry)
        if finalized:
            entries.append(finalized)
        current_entry = None

    for raw_line in lines:
        line = raw_line.rstrip()
        if line.startswith("## 快速匹配索引"):
            break

        category_match = re.match(r"^##\s+([a-z]+)\s+—\s+(.+)$", line)
        if category_match:
            push_current()
            current_category = category_match.group(1)
            current_category_label = category_match.group(2).strip()
            continue

        template_match = re.match(r"^###\s+(⭐\s+)?([^ ]+\.xml)\s+—\s+(.+)$", line)
        if template_match:
            push_current()
            current_entry = {
                "category": current_category,
                "category_label": current_category_label,
                "filename": template_match.group(2).strip(),
                "scene": template_match.group(3).strip(),
                "is_general_template": bool(template_match.group(1)),
                "catalog_slide_count": None,
                "tone": None,
                "formality": None,
                "palette": None,
                "structure": None,
                "page_types": None,
                "use_cases": None,
                "ranges": [],
            }
            continue

        if not current_entry:
            continue

        plain = strip_markdown(line)
        slide_count_match = re.search(r"(\d+)\s*页", plain)
        if slide_count_match:
            current_entry["catalog_slide_count"] = int(slide_count_match.group(1))

        tone_match = re.search(r"色调：([^|]+)\|\s*正式度：(.+)$", plain)
        if tone_match:
            current_entry["tone"] = tone_match.group(1).strip()
            current_entry["formality"] = tone_match.group(2).strip()
            continue

        if plain.startswith("配色："):
            current_entry["palette"] = plain[len("配色：") :].strip()
            continue

        if plain.startswith("结构："):
            current_entry["structure"] = plain[len("结构：") :].strip()
            continue

        if plain.startswith("页面类型："):
            current_entry["page_types"] = plain[len("页面类型：") :].strip()
            continue

        if plain.startswith("页型索引"):
            _, _, ranges_raw = plain.partition("：")
            ranges: list[dict[str, Any]] = []
            for item in [part.strip() for part in ranges_raw.split("|") if part.strip()]:
                match = re.match(r"^(.+?)\s+([0-9,\-\s无]+)$", item)
                if not match:
                    ranges.append({"label": item, "range": "", "slide_numbers": []})
                    continue
                range_text = normalize_whitespace(match.group(2))
                range_text = "" if range_text == "无" else range_text
                ranges.append(
                    {
                        "label": match.group(1).strip(),
                        "range": range_text,
                        "slide_numbers": parse_range_spec(range_text) if range_text else [],
                    }
                )
            current_entry["ranges"] = ranges
            continue

        if plain.startswith("适用："):
            current_entry["use_cases"] = plain[len("适用：") :].strip()

    push_current()
    return entries


def build_search_text(entry: dict[str, Any]) -> str:
    values: list[str] = [
        entry.get("template_id"),
        entry.get("category"),
        entry.get("category_label"),
        entry.get("scene"),
        entry.get("tone"),
        entry.get("formality"),
        entry.get("palette"),
        entry.get("structure"),
        entry.get("page_types"),
        *(entry.get("layout_tags") or []),
        entry.get("use_cases"),
        *[f'{entry_range["label"]} {entry_range["range"]}' for entry_range in entry.get("ranges", [])],
    ]
    return " ".join(str(value) for value in values if value).lower()


def build_index_data() -> dict[str, Any]:
    catalog_entries = parse_catalog()
    templates: list[dict[str, Any]] = []
    for entry in catalog_entries:
        template_path = TEMPLATES_DIR / entry["filename"]
        xml_info = parse_template_xml(template_path)
        layout_tags = sorted({tag for slide in xml_info["slide_summaries"] for tag in slide.get("layout_tags", [])})
        templates.append(
            {
                "template_id": entry["template_id"],
                "category": entry["category"],
                "category_label": entry["category_label"],
                "scene": entry["scene"],
                "tone": entry["tone"],
                "formality": entry["formality"],
                "is_general_template": entry["is_general_template"],
                "slide_count": len(xml_info["slides"]),
                "presentation_title": xml_info["title_text"],
                "palette": entry["palette"],
                "structure": entry["structure"],
                "page_types": entry["page_types"],
                "layout_tags": layout_tags,
                "use_cases": entry["use_cases"],
                "ranges": [{"label": entry_range["label"], "range": entry_range["range"]} for entry_range in entry["ranges"]],
            }
        )

    return {
        "schema_version": LIGHTWEIGHT_INDEX_SCHEMA_VERSION,
        "generated_at": datetime.now(timezone.utc).isoformat(timespec="milliseconds").replace("+00:00", "Z"),
        "template_count": len(templates),
        "templates": templates,
    }


def load_index(index_path: str | Path = DEFAULT_INDEX_PATH) -> dict[str, Any]:
    index_path = Path(index_path)
    if index_path.exists():
        existing = json.loads(read_file(index_path))
        first_template = existing.get("templates", [None])[0] if existing.get("templates") else None
        if first_template and first_template.get("layout_tags") and "bbox_summary" not in first_template:
            return existing
    return build_index_data()


def catalog_filename(entry: dict[str, Any]) -> str:
    return f'{entry["template_id"].split("--", 1)[-1]}.xml'


def build_external_template_entry(template_path: Path) -> dict[str, Any]:
    xml_info = parse_template_xml(template_path)
    return {
        "template_id": template_path.stem,
        "scene": None,
        "tone": None,
        "formality": None,
        "slide_count": len(xml_info["slides"]),
        "presentation_title": xml_info["title_text"],
        "palette": None,
        "structure": None,
        "page_types": [],
        "layout_tags": sorted(
            {tag for slide in xml_info["slide_summaries"] for tag in slide.get("layout_tags", [])}
        ),
        "use_cases": None,
        "theme_summary": xml_info["theme_summary"],
        "ranges": [],
    }


def find_template_entry(
    index_data: dict[str, Any],
    selector: str,
    *,
    fail_on_ambiguous: bool = True,
) -> dict[str, Any] | None:
    normalized = re.sub(r"\.xml$", "", selector)
    matches = [
        entry
        for entry in index_data["templates"]
        if entry["template_id"] == normalized
        or f'{entry["template_id"]}.xml' == selector
        or catalog_filename(entry) == selector
        or catalog_filename(entry) == f"{normalized}.xml"
    ]
    if len(matches) > 1 and fail_on_ambiguous:
        template_ids = ", ".join(entry["template_id"] for entry in matches)
        fail(f"template selector is ambiguous: {selector}; use one of: {template_ids}")
    if len(matches) > 1:
        return None
    return matches[0] if matches else None


def resolve_template_reference(index_data: dict[str, Any], template_selector: str) -> dict[str, Any]:
    if not template_selector:
        fail("template selector is required")

    input_path = Path(template_selector)
    as_path = input_path
    if not input_path.is_absolute():
        as_path = (Path.cwd() / as_path).resolve()
    if as_path.exists():
        entry = find_template_entry(
            index_data,
            as_path.name,
            fail_on_ambiguous=False,
        ) or build_external_template_entry(as_path)
        return {"entry": entry, "path": as_path}
    if input_path.is_absolute() or input_path.parent != Path("."):
        fail(f"template not found: {template_selector}")

    selector_name = Path(template_selector).name
    entry = find_template_entry(index_data, selector_name)
    if entry:
        return {"entry": entry, "path": get_template_path(entry)}

    fail(f"template not found: {template_selector}")


def resolve_template_entry(index_data: dict[str, Any], template_selector: str) -> dict[str, Any]:
    return resolve_template_reference(index_data, template_selector)["entry"]


def resolve_range_selection(entry: dict[str, Any], options: dict[str, Any]) -> dict[str, Any]:
    if options.get("label"):
        matched_range = next((item for item in entry["ranges"] if item["label"] == options["label"]), None)
        if not matched_range:
            fail(f'range label not found: {options["label"]}')
        slide_numbers = parse_range_spec(matched_range["range"]) if matched_range["range"] else []
        if not slide_numbers:
            fail(f'range label has no slides: {options["label"]}')
        return {"label": matched_range["label"], "range": matched_range["range"], "slide_numbers": slide_numbers}

    if not options.get("range"):
        fail("either --range or --label is required")
    slide_numbers = parse_range_spec(options["range"])
    return {"label": None, "range": compress_numbers(slide_numbers), "slide_numbers": slide_numbers}


def get_template_path(entry: dict[str, Any]) -> Path:
    return TEMPLATES_DIR / f'{entry["template_id"]}.xml'


def summarize_selection(index_data: dict[str, Any], template_selector: str, options: dict[str, Any]) -> dict[str, Any]:
    reference = resolve_template_reference(index_data, template_selector)
    entry = reference["entry"]
    selection = resolve_range_selection(entry, options)
    xml_info = parse_template_xml(reference["path"])
    slide_summaries = [
        xml_info["slide_summaries"][slide_number - 1]
        for slide_number in selection["slide_numbers"]
        if 0 < slide_number <= len(xml_info["slide_summaries"])
    ]
    return {
        "template": {
            "template_id": entry["template_id"],
            "scene": entry["scene"],
            "tone": entry["tone"],
            "formality": entry["formality"],
            "slide_count": len(xml_info["slides"]),
            "presentation_title": xml_info["title_text"],
            "palette": entry["palette"],
            "structure": entry["structure"],
            "page_types": entry["page_types"],
            "layout_tags": sorted({tag for slide in xml_info["slide_summaries"] for tag in slide.get("layout_tags", [])}),
            "use_cases": entry["use_cases"],
        },
        "selection": selection,
        "theme_summary": xml_info["theme_summary"],
        "summary": aggregate_slides(slide_summaries),
        "slides": slide_summaries,
    }


def extract_selection_xml(index_data: dict[str, Any], template_selector: str, options: dict[str, Any]) -> str:
    reference = resolve_template_reference(index_data, template_selector)
    entry = reference["entry"]
    selection = resolve_range_selection(entry, options)
    xml_info = parse_template_xml(reference["path"])
    selected_slides: list[str] = []
    for slide_number in selection["slide_numbers"]:
        if slide_number - 1 >= len(xml_info["slides"]) or slide_number <= 0:
            fail(f"slide {slide_number} is out of range for {entry['template_id']}")
        selected_slides.append(xml_info["slides"][slide_number - 1])

    chunks = [xml_info["opening_tag"]]
    if xml_info["title_xml"]:
        chunks.append(f'  {xml_info["title_xml"]}')
    if xml_info["theme_xml"]:
        chunks.append(f'  {xml_info["theme_xml"]}')
    chunks.extend(selected_slides)
    chunks.append("</presentation>")
    return "\n".join(chunks)


def search_templates(index_data: dict[str, Any], options: dict[str, Any]) -> list[dict[str, Any]]:
    query = options.get("query", "") or ""
    tokens = tokenize_query(query)
    tone = options.get("tone")
    formality = options.get("formality")
    category = options.get("category")
    layout_tag = options.get("layoutTag") or options.get("layout-tag")
    limit = int(options.get("limit", 5))

    ranked: list[dict[str, Any]] = []
    for entry in index_data["templates"]:
        if tone and entry["tone"] != tone:
            continue
        if formality and entry["formality"] != formality:
            continue
        if category and entry["category"] != category:
            continue
        if layout_tag and layout_tag not in (entry.get("layout_tags") or []):
            continue

        score = 0
        if not query:
            score = 1
        else:
            search_text = build_search_text(entry)
            exact_id = entry["template_id"].lower() == query.lower()
            if exact_id:
                score += 100
            for token in tokens:
                if token in search_text:
                    score += len(token) * 10 if re.search(r"[\u3400-\u9fff]", token) else len(token) * 6
                if entry.get("scene") and token in entry["scene"]:
                    score += 12
                if entry.get("use_cases") and token in entry["use_cases"]:
                    score += 8
            if entry.get("scene") and query in entry["scene"]:
                score += 40
            if entry.get("use_cases") and query in entry["use_cases"]:
                score += 30
            if score == 0:
                continue

        if entry.get("is_general_template"):
            score -= 5
        ranked.append(
            {
                "template_id": entry["template_id"],
                "scene": entry["scene"],
                "tone": entry["tone"],
                "formality": entry["formality"],
                "is_general_template": entry["is_general_template"],
                "use_cases": entry["use_cases"],
                "layout_tags": entry.get("layout_tags") or [],
                "slide_count": entry["slide_count"],
                "ranges": entry["ranges"],
                "score": score,
            }
        )

    ranked.sort(key=lambda item: (-item["score"], item["template_id"]))
    return ranked[:limit]


def parse_cli_args(argv: list[str]) -> tuple[str | None, dict[str, Any]]:
    if not argv:
        return None, {}
    command, *rest = argv
    options: dict[str, Any] = {}
    index = 0
    while index < len(rest):
        token = rest[index]
        if not token.startswith("--"):
            fail(f"unexpected argument: {token}")
        key = token[2:]
        next_token = rest[index + 1] if index + 1 < len(rest) else None
        if next_token is None or next_token.startswith("--"):
            options[key] = True
            index += 1
            continue
        options[key] = next_token
        index += 2
    return command, options


def print_usage() -> None:
    usage = [
        "Usage:",
        "  python3 template_tool.py build-index [--out <path>]",
        "  python3 template_tool.py search --query <text> [--tone light|dark|colorful] [--formality formal|casual|creative] [--layout-tag <tag>] [--limit 3]",
        "  python3 template_tool.py summarize --template <template-id|path> (--range 1-2,5 | --label 封面)",
        "  python3 template_tool.py extract --template <template-id|path> (--range 1-2,5 | --label 封面) [--with-summary] [--out <path>]",
    ]
    print("\n".join(usage), file=sys.stderr)


def write_json(value: Any) -> None:
    print(json.dumps(value, ensure_ascii=False, indent=2))


def run_cli(argv: list[str] | None = None) -> None:
    command, options = parse_cli_args(argv or sys.argv[1:])
    if not command or command in {"--help", "help"}:
        print_usage()
        raise SystemExit(0)

    if command == "build-index":
        index_data = build_index_data()
        output_path = Path(options["out"]).resolve() if options.get("out") else DEFAULT_INDEX_PATH
        output_path.write_text(f'{json.dumps(index_data, ensure_ascii=False, indent=2)}\n', encoding="utf-8")
        print(output_path)
        return

    if command == "search":
        write_json(search_templates(load_index(), options))
        return

    if command == "summarize":
        write_json(summarize_selection(load_index(), options.get("template"), options))
        return

    if command == "extract":
        index_data = load_index()
        xml = extract_selection_xml(index_data, options.get("template"), options)
        if options.get("with-summary"):
            summary = summarize_selection(index_data, options.get("template"), options)
            write_json({"xml": xml, "selection": summary["selection"], "summary": summary["summary"], "slides": summary["slides"]})
            return
        if options.get("out"):
            output_path = Path(options["out"]).resolve()
            output_path.write_text(f"{xml}\n", encoding="utf-8")
            print(output_path)
            return
        sys.stdout.write(f"{xml}\n")
        return

    print_usage()
    fail(f"unknown command: {command}")


if __name__ == "__main__":
    try:
        run_cli()
    except TemplateToolError as error:
        print(f"template-tool error: {error}", file=sys.stderr)
        raise SystemExit(1) from error
