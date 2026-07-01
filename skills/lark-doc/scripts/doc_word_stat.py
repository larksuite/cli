#!/usr/bin/env python3
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
"""Standalone Lark Docs word and character counter for XML or Markdown input."""

from __future__ import annotations

import argparse
import json
import re
import sys
import unicodedata
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Literal, Protocol
from xml.etree import ElementTree as ET

# ---------------------------------------------------------------------------
# Data model
# ---------------------------------------------------------------------------

@dataclass
class TextRun:
    text: str
    attrs: dict[str, Any] = field(default_factory=dict)


@dataclass
class Block:
    type: str
    attrs: dict[str, Any] = field(default_factory=dict)
    children: list["Block"] = field(default_factory=list)
    text_runs: list[TextRun] = field(default_factory=list)
    raw: Any = None


@dataclass
class Segment:
    text: str
    block_type: str
    block_id: str | None = None
    kind: str = "text"
    boundary_before: bool = True
    boundary_after: bool = True

    def to_dict(self) -> dict[str, Any]:
        return {
            "text": self.text,
            "block_type": self.block_type,
            "block_id": self.block_id,
            "kind": self.kind,
            "boundary_before": self.boundary_before,
            "boundary_after": self.boundary_after,
        }


@dataclass(frozen=True)
class UnknownBlock:
    type: str
    block_id: str | None = None
    action: str = "recurse_children"

    def to_dict(self) -> dict[str, str | None]:
        return {
            "type": self.type,
            "block_id": self.block_id,
            "action": self.action,
        }

# ---------------------------------------------------------------------------
# Counting rules
# ---------------------------------------------------------------------------

CHINESE_PUNCTUATION = set("，。！？；：、（）《》〈〉“”‘’【】「」『』〔〕…—～·￥")
ENGLISH_PUNCTUATION = set(
    r"""!"#$%&'()*+,-./:;<=>?@[\]^_`{|}~"""
)


LexemeKind = Literal["english", "number"]


@dataclass
class Stats:
    word_count: int = 0
    char_count: int = 0
    han_chars: int = 0
    english_words: int = 0
    number_words: int = 0
    chinese_punctuations: int = 0
    english_letters: int = 0
    digits: int = 0
    english_punctuations: int = 0
    symbol_words: int = 0
    symbol_chars: int = 0

    def to_dict(self) -> dict[str, object]:
        return {
            "word_count": self.word_count,
            "char_count": self.char_count,
            "breakdown": {
                "han_chars": self.han_chars,
                "english_words": self.english_words,
                "number_words": self.number_words,
                "chinese_punctuations": self.chinese_punctuations,
                "english_letters": self.english_letters,
                "digits": self.digits,
                "english_punctuations": self.english_punctuations,
                "symbol_words": self.symbol_words,
                "symbol_chars": self.symbol_chars,
            },
        }


def is_han(ch: str) -> bool:
    code = ord(ch)
    return (
        0x3400 <= code <= 0x4DBF
        or 0x4E00 <= code <= 0x9FFF
        or 0xF900 <= code <= 0xFAFF
        or 0x20000 <= code <= 0x2A6DF
        or 0x2A700 <= code <= 0x2B73F
        or 0x2B740 <= code <= 0x2B81F
        or 0x2B820 <= code <= 0x2CEAF
        or 0x30000 <= code <= 0x3134F
    )


def is_ascii_letter(ch: str) -> bool:
    return ("a" <= ch <= "z") or ("A" <= ch <= "Z")


def is_digit(ch: str) -> bool:
    return "0" <= ch <= "9"


def is_chinese_punctuation(ch: str) -> bool:
    if ch in CHINESE_PUNCTUATION:
        return True
    return unicodedata.category(ch).startswith("P") and unicodedata.east_asian_width(ch) in {
        "W",
        "F",
    }


def is_english_punctuation(ch: str) -> bool:
    return ch in ENGLISH_PUNCTUATION


def is_unicode_symbol(ch: str) -> bool:
    return unicodedata.category(ch).startswith("S")


def utf16_units(ch: str) -> int:
    return len(ch.encode("utf-16-le")) // 2


class Counter:
    def __init__(self) -> None:
        self.stats = Stats()
        self._lexeme_kind: LexemeKind | None = None
        self._lexeme_has_digit = False
        self._symbol_run_length = 0
        self._at_boundary = True

    def count_segments(self, segments: list[Segment]) -> Stats:
        for segment in segments:
            if segment.boundary_before:
                self._end_unit()
                self._at_boundary = True
            if segment.kind == "marker":
                self.write_marker(segment.text)
            elif segment.kind == "code":
                self.write_code(segment.text)
            else:
                self.write(segment.text)
            if segment.boundary_after:
                self._end_unit()
                self._at_boundary = True
        self._end_unit()
        return self.stats

    def write(self, text: str) -> None:
        for ch in text:
            self._write_char(ch)

    def write_marker(self, text: str) -> None:
        for ch in text:
            if ch.isspace():
                continue
            self._end_unit()
            self.stats.word_count += 1
            self.stats.char_count += 1
            self._at_boundary = False

    def write_code(self, text: str) -> None:
        for ch in text:
            self._write_code_char(ch)

    def _write_code_char(self, ch: str) -> None:
        if ch.isspace():
            self._end_unit()
            self._at_boundary = True
            return

        if is_han(ch):
            self._end_lexeme()
            self._end_symbol_run(count_word=False)
            self.stats.han_chars += 1
            self.stats.word_count += 1
            self.stats.char_count += 1
            self._at_boundary = False
            return

        if is_ascii_letter(ch):
            self._end_symbol_run(count_word=False)
            self.stats.english_letters += 1
            self.stats.char_count += 1
            if self._lexeme_kind is None:
                self._lexeme_kind = "english"
            elif self._lexeme_kind == "number":
                self._lexeme_kind = "english"
            self._at_boundary = False
            return

        if is_digit(ch):
            self._end_symbol_run(count_word=False)
            self.stats.digits += 1
            self.stats.char_count += 1
            self._at_boundary = False
            return

        if is_chinese_punctuation(ch):
            self._end_lexeme()
            self._end_symbol_run(count_word=False)
            self.stats.chinese_punctuations += 1
            self.stats.word_count += 1
            self.stats.char_count += 1
            self._at_boundary = False
            return

        if is_english_punctuation(ch):
            keeps_lexeme = self._lexeme_kind == "english" and ch in {"'", "-"}
            if not keeps_lexeme:
                had_lexeme = self._lexeme_kind is not None
                self._end_lexeme()
                if not had_lexeme and (self._symbol_run_length > 0 or self._at_boundary):
                    self._symbol_run_length += 1
            self.stats.english_punctuations += 1
            self.stats.char_count += 1
            if keeps_lexeme:
                self._at_boundary = False
            return

        if is_unicode_symbol(ch):
            self._write_symbol_char(ch)
            return

        self._end_lexeme()
        self._end_symbol_run(count_word=False)
        self._at_boundary = False

    def _write_char(self, ch: str) -> None:
        if ch.isspace():
            self._end_unit()
            self._at_boundary = True
            return

        if is_han(ch):
            self._end_lexeme()
            self._end_symbol_run(count_word=False)
            self.stats.han_chars += 1
            self.stats.word_count += 1
            self.stats.char_count += 1
            self._at_boundary = False
            return

        if is_ascii_letter(ch):
            self._end_symbol_run(count_word=False)
            self.stats.english_letters += 1
            self.stats.char_count += 1
            if self._lexeme_kind is None:
                self._lexeme_kind = "english"
            elif self._lexeme_kind == "number":
                self._lexeme_kind = "english"
            self._at_boundary = False
            return

        if is_digit(ch):
            self._end_symbol_run(count_word=False)
            self.stats.digits += 1
            self.stats.char_count += 1
            self._lexeme_has_digit = True
            if self._lexeme_kind is None:
                self._lexeme_kind = "number"
            self._at_boundary = False
            return

        if is_chinese_punctuation(ch):
            self._end_lexeme()
            self._end_symbol_run(count_word=False)
            self.stats.chinese_punctuations += 1
            self.stats.word_count += 1
            self.stats.char_count += 1
            self._at_boundary = False
            return

        if is_english_punctuation(ch):
            # Apostrophes/hyphens can connect English runs. Dot/comma/hyphen
            # can format numeric runs such as 3.14, 1,000, 2026-06-30, or
            # 7-9. Alphanumeric versions like v1.2.3 should remain one semantic
            # run too. These punctuations still count as characters.
            keeps_lexeme = (
                self._lexeme_kind == "english"
                and (ch in {"'", "-"} or (self._lexeme_has_digit and ch == "."))
            ) or (
                self._lexeme_kind == "number"
                and ch in {".", ",", "-"}
            )
            if not keeps_lexeme:
                had_lexeme = self._lexeme_kind is not None
                self._end_lexeme()
                if not had_lexeme and (self._symbol_run_length > 0 or self._at_boundary):
                    self._symbol_run_length += 1
            self.stats.english_punctuations += 1
            self.stats.char_count += 1
            if keeps_lexeme:
                self._at_boundary = False
            return

        if is_unicode_symbol(ch):
            self._write_symbol_char(ch)
            return

        self._end_lexeme()
        self._end_symbol_run(count_word=False)
        self._at_boundary = False

    def _write_symbol_char(self, ch: str) -> None:
        self._end_lexeme()
        self._end_symbol_run(count_word=False)
        units = utf16_units(ch)
        self.stats.symbol_words += 1
        self.stats.symbol_chars += units
        self.stats.word_count += 1
        self.stats.char_count += units
        self._at_boundary = False

    def _end_unit(self) -> None:
        self._end_lexeme()
        self._end_symbol_run(count_word=True)

    def _end_lexeme(self) -> None:
        if self._lexeme_kind == "english":
            self.stats.english_words += 1
            self.stats.word_count += 1
        elif self._lexeme_kind == "number":
            self.stats.number_words += 1
            self.stats.word_count += 1
        self._lexeme_kind = None
        self._lexeme_has_digit = False

    def _end_symbol_run(self, *, count_word: bool) -> None:
        if self._symbol_run_length >= 2 and count_word:
            self.stats.symbol_words += 1
            self.stats.word_count += 1
        if self._symbol_run_length:
            self._at_boundary = False
        self._symbol_run_length = 0

# ---------------------------------------------------------------------------
# Markdown parser
# ---------------------------------------------------------------------------

HEADING_RE = re.compile(r"^(#{1,6})\s+(.*)$")
LIST_RE = re.compile(r"^\s*(?:[-*+]|\d+[.)])\s+(.*)$")
QUOTE_RE = re.compile(r"^\s*>\s?(.*)$")
TABLE_SEP_RE = re.compile(r"^\s*\|?\s*:?-{3,}:?\s*(?:\|\s*:?-{3,}:?\s*)+\|?\s*$")


def parse_markdown(source: str) -> list[Block]:
    lines = source.splitlines()
    blocks: list[Block] = []
    paragraph: list[str] = []
    i = 0

    def flush_paragraph() -> None:
        if paragraph:
            blocks.append(Block(type="paragraph", text_runs=[TextRun(clean_inline(" ".join(paragraph)))]))
            paragraph.clear()

    while i < len(lines):
        line = lines[i]
        stripped = line.strip()
        if not stripped:
            flush_paragraph()
            i += 1
            continue

        if stripped.startswith("```") or stripped.startswith("~~~"):
            flush_paragraph()
            fence = stripped[:3]
            code_lines: list[str] = []
            i += 1
            while i < len(lines) and not lines[i].strip().startswith(fence):
                code_lines.append(lines[i])
                i += 1
            if i < len(lines):
                i += 1
            blocks.append(Block(type="code", text_runs=[TextRun("\n".join(code_lines))]))
            continue

        heading = HEADING_RE.match(line)
        if heading:
            flush_paragraph()
            blocks.append(Block(type="heading", text_runs=[TextRun(clean_inline(heading.group(2)))]))
            i += 1
            continue

        if _looks_like_table(lines, i):
            flush_paragraph()
            table, consumed = _parse_table(lines, i)
            blocks.append(table)
            i += consumed
            continue

        item = LIST_RE.match(line)
        if item:
            flush_paragraph()
            items: list[Block] = []
            while i < len(lines):
                match = LIST_RE.match(lines[i])
                if not match:
                    break
                items.append(Block(type="list_item", text_runs=[TextRun(clean_inline(match.group(1)))]))
                i += 1
            blocks.append(Block(type="list", children=items))
            continue

        quote = QUOTE_RE.match(line)
        if quote:
            flush_paragraph()
            quote_lines: list[str] = []
            while i < len(lines):
                match = QUOTE_RE.match(lines[i])
                if not match:
                    break
                quote_lines.append(match.group(1))
                i += 1
            blocks.append(Block(type="quote", text_runs=[TextRun(clean_inline(" ".join(quote_lines)))]))
            continue

        paragraph.append(stripped)
        i += 1

    flush_paragraph()
    return blocks


def clean_inline(text: str) -> str:
    text = re.sub(r"!\[([^\]]*)\]\([^)]+\)", r"\1", text)
    text = re.sub(r"\[([^\]]+)\]\([^)]+\)", r"\1", text)
    text = re.sub(r"([*_`~]{1,3})(.*?)\1", r"\2", text)
    text = text.replace("\\", "")
    return text


def _looks_like_table(lines: list[str], i: int) -> bool:
    return i + 1 < len(lines) and "|" in lines[i] and TABLE_SEP_RE.match(lines[i + 1]) is not None


def _parse_table(lines: list[str], i: int) -> tuple[Block, int]:
    rows: list[Block] = []
    consumed = 0
    while i + consumed < len(lines):
        line = lines[i + consumed]
        stripped = line.strip()
        if not stripped or "|" not in stripped:
            break
        if consumed == 1 and TABLE_SEP_RE.match(stripped):
            consumed += 1
            continue
        cells = [cell.strip() for cell in stripped.strip("|").split("|")]
        row = Block(
            type="tr",
            children=[
                Block(type="table_cell", text_runs=[TextRun(clean_inline(cell))])
                for cell in cells
                if cell
            ],
        )
        rows.append(row)
        consumed += 1
    return Block(type="table", children=rows), consumed

# ---------------------------------------------------------------------------
# XML parser
# ---------------------------------------------------------------------------

INLINE_TAGS = {
    "b",
    "strong",
    "i",
    "em",
    "u",
    "s",
    "del",
    "span",
    "text",
    "plain_text",
    "code",
    "a",
    "link",
    "mention",
    "mention-doc",
    "mention-user",
}

TYPE_ALIASES = {
    "doc": "document",
    "document": "document",
    "fragment": "fragment",
    "p": "paragraph",
    "paragraph": "paragraph",
    "heading": "heading",
    "h1": "heading",
    "h2": "heading",
    "h3": "heading",
    "h4": "heading",
    "h5": "heading",
    "h6": "heading",
    "h7": "heading",
    "h8": "heading",
    "h9": "heading",
    "ul": "list",
    "ol": "list",
    "li": "list_item",
    "task": "task",
    "todo": "list_item",
    "blockquote": "quote",
    "quote": "quote",
    "br": "br",
    "hr": "hr",
    "title": "title",
    "checkbox": "checkbox",
    "grid": "grid",
    "column": "column",
    "table": "table",
    "colgroup": "colgroup",
    "col": "col",
    "tr": "tr",
    "td": "table_cell",
    "th": "table_cell",
    "pre": "code",
    "code_block": "code",
    "callout": "callout",
    "figure": "figure",
    "toggle": "toggle",
    "img": "image",
    "source": "source",
    "file": "file",
    "media": "media",
    "latex": "latex",
    "cite": "cite",
    "bookmark": "bookmark",
    "button": "button",
    "whiteboard": "whiteboard",
    "mermaid": "mermaid",
    "plantuml": "plantuml",
    "poll": "poll",
    "isv": "isv",
    "mindnote": "mindnote",
    "diagram": "diagram",
    "sheet": "sheet",
    "bitable": "bitable",
    "base-ref": "base_ref",
    "base_ref": "base_ref",
    "base-refer": "base_ref",
    "base_refer": "base_ref",
    "synced-reference": "synced_reference",
    "synced_reference": "synced_reference",
    "synced-source": "synced_source",
    "synced_source": "synced_source",
    "okr": "okr",
    "chat-card": "chat_card",
    "chat_card": "chat_card",
    "sub_page_list": "sub-page-list",
    "sub-page-list": "sub-page-list",
}

SUBTYPE_ATTR_TAGS = {
    "a",
    "button",
    "cite",
    "img",
    "sheet",
    "source",
    "whiteboard",
    "base-ref",
    "base_ref",
    "base-refer",
    "base_refer",
    "synced-reference",
    "synced_reference",
    "synced-source",
    "synced_source",
    "okr",
    "chat-card",
    "chat_card",
    "sub_page_list",
    "sub-page-list",
}

MAX_XML_INPUT_CHARS = 20_000_000
FORBIDDEN_XML_DECL_RE = re.compile(r"<!\s*(?:DOCTYPE|ENTITY)\b", re.IGNORECASE)


class UserInputError(ValueError):
    pass


def local_name(tag: str) -> str:
    if "}" in tag:
        return tag.rsplit("}", 1)[1]
    return tag


def block_type_for(elem: ET.Element) -> str:
    tag = local_name(elem.tag)
    explicit = elem.attrib.get("block_type")
    if explicit is None and tag not in SUBTYPE_ATTR_TAGS:
        explicit = elem.attrib.get("type")
    if explicit:
        return TYPE_ALIASES.get(explicit, explicit)
    return TYPE_ALIASES.get(tag, tag)


def ensure_safe_xml_source(source: str) -> None:
    if len(source) > MAX_XML_INPUT_CHARS:
        raise UserInputError(
            f"XML input is too large ({len(source)} chars, limit {MAX_XML_INPUT_CHARS})"
        )
    if FORBIDDEN_XML_DECL_RE.search(source):
        raise UserInputError("XML input must not contain DOCTYPE or ENTITY declarations")


def parse_xml(source: str) -> list[Block]:
    source = source.strip()
    if not source:
        return []
    ensure_safe_xml_source(source)
    try:
        root = ET.fromstring(source)
    except ET.ParseError:
        # docs +fetch raw output can occasionally include adjacent top-level
        # blocks. Wrap them so standard ElementTree can parse the stream.
        root = ET.fromstring(f"<fragment>{source}</fragment>")
    return [_parse_block(root)]


def _parse_block(elem: ET.Element) -> Block:
    block = Block(type=block_type_for(elem), attrs=dict(elem.attrib), raw=elem)
    _collect_content(elem, block)
    if not block.text_runs and not block.children:
        if block.type == "image":
            display = elem.attrib.get("caption")
        else:
            display = (
                elem.attrib.get("text")
                or elem.attrib.get("name")
                or elem.attrib.get("title")
                or elem.attrib.get("alt")
                or elem.attrib.get("caption")
            )
        if display:
            block.text_runs.append(TextRun(display, dict(elem.attrib)))
    return block


def _collect_content(elem: ET.Element, block: Block) -> None:
    if elem.text:
        block.text_runs.append(TextRun(elem.text))

    for child in list(elem):
        tag = local_name(child.tag)
        if tag == "br":
            block.text_runs.append(TextRun("\n"))
        elif tag in INLINE_TAGS:
            _collect_inline(child, block)
        else:
            block.children.append(_parse_block(child))
        if child.tail:
            block.text_runs.append(TextRun(child.tail))


def _collect_inline(elem: ET.Element, block: Block) -> None:
    if local_name(elem.tag) == "br":
        block.text_runs.append(TextRun("\n", dict(elem.attrib)))
        return

    display = (
        elem.attrib.get("text")
        or elem.attrib.get("name")
        or elem.attrib.get("title")
        or elem.attrib.get("alt")
    )
    if display:
        block.text_runs.append(TextRun(display, dict(elem.attrib)))
        return

    if elem.text:
        block.text_runs.append(TextRun(elem.text, dict(elem.attrib)))
    for child in list(elem):
        _collect_inline(child, block)
        if child.tail:
            block.text_runs.append(TextRun(child.tail))

# ---------------------------------------------------------------------------
# Block extraction registry
# ---------------------------------------------------------------------------

@dataclass
class ExtractContext:
    unknown_blocks: list[UnknownBlock] = field(default_factory=list)
    resource_texts: dict[str, str] = field(default_factory=dict)


class Handler(Protocol):
    def extract(self, block: Block, registry: "Registry", ctx: ExtractContext) -> list[Segment]:
        raise NotImplementedError


def block_id(block: Block) -> str | None:
    for key in ("id", "block_id", "block-id", "token"):
        value = block.attrs.get(key)
        if isinstance(value, str) and value:
            return value
    return None


def runs_text(block: Block) -> str:
    return "".join(run.text for run in block.text_runs)


def raw_tag(block: Block) -> str:
    tag = getattr(getattr(block, "raw", None), "tag", "") or ""
    if "}" in tag:
        return tag.rsplit("}", 1)[1]
    return tag


class TextBlockHandler:
    def __init__(self, kind: str = "text") -> None:
        self.kind = kind

    def extract(self, block: Block, registry: "Registry", ctx: ExtractContext) -> list[Segment]:
        segments: list[Segment] = []
        text = runs_text(block)
        if text.strip():
            segments.append(
                Segment(
                    text=text,
                    block_type=block.type,
                    block_id=block_id(block),
                    kind=self.kind,
                )
            )
        for child in block.children:
            segments.extend(registry.extract(child, ctx))
        return segments


class ContainerHandler:
    def extract(self, block: Block, registry: "Registry", ctx: ExtractContext) -> list[Segment]:
        segments: list[Segment] = []
        text = runs_text(block)
        if text.strip():
            segments.append(
                Segment(text=text, block_type=block.type, block_id=block_id(block), kind="text")
            )
        for child in block.children:
            segments.extend(registry.extract(child, ctx))
        return segments


class ListHandler:
    def extract(self, block: Block, registry: "Registry", ctx: ExtractContext) -> list[Segment]:
        tag = raw_tag(block)
        if tag not in {"ol", "ul"}:
            return ContainerHandler().extract(block, registry, ctx)

        segments: list[Segment] = []
        text = runs_text(block)
        if text.strip():
            segments.append(
                Segment(text=text, block_type=block.type, block_id=block_id(block), kind="text")
            )

        next_seq = 1
        for child in block.children:
            if child.type == "list_item":
                if tag == "ol":
                    seq = child.attrs.get("seq")
                    if isinstance(seq, str) and seq.isdigit():
                        marker = seq
                        next_seq = int(seq) + 1
                    else:
                        marker = str(next_seq)
                        next_seq += 1
                    segments.append(
                        Segment(
                            text=f"{marker}.",
                            block_type="list_marker",
                            block_id=block_id(child),
                            kind="text",
                        )
                    )
                else:
                    segments.append(
                        Segment(
                            text="•",
                            block_type="list_marker",
                            block_id=block_id(child),
                            kind="marker",
                        )
                    )
            segments.extend(registry.extract(child, ctx))
        return segments


class CheckboxHandler(TextBlockHandler):
    def extract(self, block: Block, registry: "Registry", ctx: ExtractContext) -> list[Segment]:
        return [
            Segment(
                text="☑" if block.attrs.get("done") == "true" else "☐",
                block_type="checkbox_marker",
                block_id=block_id(block),
                kind="marker",
            ),
            *super().extract(block, registry, ctx),
        ]


class UnknownHandler:
    def extract(self, block: Block, registry: "Registry", ctx: ExtractContext) -> list[Segment]:
        ctx.unknown_blocks.append(UnknownBlock(type=block.type, block_id=block_id(block)))
        return ContainerHandler().extract(block, registry, ctx)


class IgnoreHandler:
    def __init__(self, action: str = "ignored") -> None:
        self.action = action

    def extract(self, block: Block, registry: "Registry", ctx: ExtractContext) -> list[Segment]:
        ctx.unknown_blocks.append(UnknownBlock(type=block.type, block_id=block_id(block), action=self.action))
        return []


class TaskHandler:
    def extract(self, block: Block, registry: "Registry", ctx: ExtractContext) -> list[Segment]:
        task_id = block.attrs.get("task-id") or block.attrs.get("task_id")
        if isinstance(task_id, str) and task_id:
            text = ctx.resource_texts.get(f"task:{task_id}")
            if text and text.strip():
                marker = "☑" if block.attrs.get("status") in {"done", "completed", "complete"} else "☐"
                return [
                    Segment(text=marker, block_type="task_marker", block_id=block_id(block), kind="marker"),
                    Segment(text=text, block_type="task", block_id=block_id(block), kind="resource_title"),
                ]

        ctx.unknown_blocks.append(UnknownBlock(type=block.type, block_id=block_id(block), action="ignored_resource"))
        return []


class WhiteboardHandler:
    def extract(self, block: Block, registry: "Registry", ctx: ExtractContext) -> list[Segment]:
        board_type = block.attrs.get("type")
        is_empty_resource_shell = not board_type and not block.children and not runs_text(block).strip()
        action = (
            "ignored_resource"
            if board_type in {"blank", "mermaid", "plantuml", "svg"} or is_empty_resource_shell
            else "unsupported_resource"
        )
        ctx.unknown_blocks.append(UnknownBlock(type=block.type, block_id=block_id(block), action=action))
        return []


class SyncedSourceHandler:
    def extract(self, block: Block, registry: "Registry", ctx: ExtractContext) -> list[Segment]:
        if block.children:
            segments: list[Segment] = []
            for child in block.children:
                segments.extend(registry.extract(child, ctx))
            return segments

        ctx.unknown_blocks.append(UnknownBlock(type=block.type, block_id=block_id(block), action="unsupported_resource"))
        return []


class Registry:
    def __init__(self) -> None:
        self._handlers: dict[str, Handler] = {}
        self._unknown = UnknownHandler()

    def register(self, *types: str, handler: Handler) -> None:
        for typ in types:
            self._handlers[typ] = handler

    def extract(self, block: Block, ctx: ExtractContext) -> list[Segment]:
        handler = self._handlers.get(block.type, self._unknown)
        return handler.extract(block, self, ctx)


def default_registry() -> Registry:
    registry = Registry()
    registry.register("document", "fragment", "root", handler=ContainerHandler())
    registry.register("title", handler=TextBlockHandler("title"))
    registry.register("paragraph", "p", handler=TextBlockHandler("text"))
    registry.register(
        "heading",
        "h",
        "h1",
        "h2",
        "h3",
        "h4",
        "h5",
        "h6",
        "h7",
        "h8",
        "h9",
        handler=TextBlockHandler("heading"),
    )
    registry.register("list", "ul", "ol", handler=ListHandler())
    registry.register("list_item", "li", "todo", handler=TextBlockHandler("list_item"))
    registry.register("checkbox", handler=CheckboxHandler("list_item"))
    registry.register(
        "quote",
        "blockquote",
        "callout",
        "toggle",
        "grid",
        "column",
        "figure",
        handler=ContainerHandler(),
    )
    registry.register("table", "thead", "tbody", "tr", handler=ContainerHandler())
    registry.register("table_cell", "td", "th", handler=TextBlockHandler("table_cell"))
    registry.register("code", "code_block", "pre", handler=TextBlockHandler("code"))
    registry.register("link", "a", "mention", "mention-doc", "mention-user", "time", handler=TextBlockHandler("inline"))
    registry.register("image", "img", handler=TextBlockHandler("caption"))
    registry.register("colgroup", "col", "br", "hr", handler=IgnoreHandler("ignored_structure"))
    registry.register("button", "cite", "latex", "bookmark", handler=IgnoreHandler("ignored_inline"))
    registry.register("task", handler=TaskHandler())
    registry.register("whiteboard", handler=WhiteboardHandler())
    registry.register("synced_source", handler=SyncedSourceHandler())
    registry.register(
        "mermaid",
        "sheet",
        "source",
        "file",
        "media",
        "chat_card",
        "base_ref",
        "bitable",
        "synced_reference",
        "poll",
        "isv",
        "mindnote",
        "diagram",
        "sub-page-list",
        handler=IgnoreHandler("ignored_resource"),
    )
    registry.register(
        "okr",
        "plantuml",
        handler=IgnoreHandler("unsupported_resource"),
    )
    return registry

# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------

VERSION = "0.1-alpha"


def build_diagnostics(items: list) -> dict[str, object]:
    actions: dict[str, int] = {}
    types: dict[str, int] = {}
    unsupported_types: dict[str, int] = {}
    unknown_types: dict[str, int] = {}
    for item in items:
        actions[item.action] = actions.get(item.action, 0) + 1
        types[item.type] = types.get(item.type, 0) + 1
        if item.action == "unsupported_resource":
            unsupported_types[item.type] = unsupported_types.get(item.type, 0) + 1
        if item.action == "recurse_children":
            unknown_types[item.type] = unknown_types.get(item.type, 0) + 1
    return {
        "actions": actions,
        "types": types,
        "unsupported_types": unsupported_types,
        "unknown_types": unknown_types,
        "has_unsupported": bool(unsupported_types),
        "has_unknown": bool(unknown_types),
    }


def read_input(path: str) -> str:
    if path == "-":
        return sys.stdin.read()
    return Path(path).read_text(encoding="utf-8")


def read_resource_texts(path: str | None) -> dict[str, str]:
    if not path:
        return {}
    payload = json.loads(Path(path).read_text(encoding="utf-8"))
    if not isinstance(payload, dict):
        raise ValueError("--resource-texts must be a JSON object")
    return {str(key): str(value) for key, value in payload.items()}


def extract_lark_json_content(source: str) -> str:
    try:
        envelope = json.loads(source)
    except json.JSONDecodeError as exc:
        raise UserInputError(f"could not parse lark-cli JSON envelope: {exc}") from exc

    if not isinstance(envelope, dict):
        raise UserInputError("lark-cli JSON envelope must be an object")
    data = envelope.get("data")
    if not isinstance(data, dict):
        raise UserInputError("lark-cli JSON envelope is missing object field data")
    document = data.get("document")
    if not isinstance(document, dict):
        raise UserInputError("lark-cli JSON envelope is missing object field data.document")
    content = document.get("content")
    if not isinstance(content, str):
        raise UserInputError("lark-cli JSON envelope is missing string field data.document.content")
    return content


HELP_EPILOG = """
Examples:
  Local XML file:
    python3 doc_word_stat.py --protocol xml /absolute/path/doc.xml

  Local Markdown file:
    python3 doc_word_stat.py --protocol md /absolute/path/doc.md

  Pipe an extracted local file:
    cat /absolute/path/doc.xml | python3 doc_word_stat.py --protocol xml --pretty

  Lark CLI XML fetch, JSON envelope output:
    lark-cli docs +fetch --doc "$URL" --doc-format xml --detail full --format json \\
      | python3 doc_word_stat.py --protocol xml --lark-json --pretty

  Lark CLI Markdown fetch, raw content output:
    lark-cli docs +fetch --doc "$URL" --doc-format markdown \\
      | python3 doc_word_stat.py --protocol md

  Strict integration for agents or automation:
    lark-cli docs +fetch --doc "$URL" --doc-format xml --detail full --format json \\
      | python3 doc_word_stat.py --protocol xml --lark-json --fail-on-unsupported --fail-on-unknown
"""


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Count semantic words and visible characters in Lark Docs XML or Markdown.",
        epilog=HELP_EPILOG,
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument(
        "--version",
        action="version",
        version=f"%(prog)s {VERSION}",
    )
    parser.add_argument(
        "input",
        nargs="?",
        default="-",
        help="input file path, or '-' / omitted for stdin",
    )
    parser.add_argument(
        "--protocol",
        choices=("xml", "md"),
        required=True,
        help="input protocol produced by docs +fetch",
    )
    parser.add_argument(
        "--pretty",
        action="store_true",
        help="pretty-print JSON output",
    )
    parser.add_argument(
        "--segments",
        action="store_true",
        help="include extracted text segments for debugging",
    )
    parser.add_argument(
        "--lark-json",
        action="store_true",
        help="read lark-cli docs +fetch JSON and count data.document.content",
    )
    parser.add_argument(
        "--resource-texts",
        help='optional JSON object mapping resource keys to visible text, e.g. {"task:<task-id>": "title"}',
    )
    parser.add_argument(
        "--fail-on-unsupported",
        action="store_true",
        help="exit with code 2 when unsupported_blocks is non-empty",
    )
    parser.add_argument(
        "--fail-on-unknown",
        action="store_true",
        help="exit with code 3 when unknown XML/Markdown block types are encountered",
    )
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    source = read_input(args.input)
    if args.lark_json:
        try:
            source = extract_lark_json_content(source)
        except UserInputError as exc:
            print(f"error: {exc}", file=sys.stderr)
            return 1

    if args.protocol == "xml":
        try:
            blocks = parse_xml(source)
        except (ET.ParseError, UserInputError) as exc:
            print(f"error: could not parse XML input: {exc}", file=sys.stderr)
            return 1
    else:
        blocks = parse_markdown(source)

    ctx = ExtractContext(resource_texts=read_resource_texts(args.resource_texts))
    registry = default_registry()
    segments = []
    for block in blocks:
        segments.extend(registry.extract(block, ctx))

    stats = Counter().count_segments(segments)
    payload = stats.to_dict()
    payload["protocol"] = args.protocol
    payload["unknown_blocks"] = [item.to_dict() for item in ctx.unknown_blocks]
    payload["unsupported_blocks"] = [
        item.to_dict() for item in ctx.unknown_blocks if item.action == "unsupported_resource"
    ]
    payload["diagnostics"] = build_diagnostics(ctx.unknown_blocks)
    if args.segments:
        payload["segments"] = [segment.to_dict() for segment in segments]

    indent = 2 if args.pretty else None
    print(json.dumps(payload, ensure_ascii=False, indent=indent, sort_keys=args.pretty))
    if args.fail_on_unsupported and payload["unsupported_blocks"]:
        return 2
    if args.fail_on_unknown and payload["diagnostics"]["has_unknown"]:
        return 3
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

