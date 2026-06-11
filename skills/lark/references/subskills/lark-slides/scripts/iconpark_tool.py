#!/usr/bin/env python3
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

from __future__ import annotations

import json
import re
import sys
from pathlib import Path
from typing import Any


SKILL_ROOT = Path(__file__).resolve().parent.parent
REFERENCES_DIR = SKILL_ROOT / "references"
DEFAULT_INDEX_PATH = REFERENCES_DIR / "iconpark-index.json"
DEFAULT_LIMIT = 8
CURATED_ICON_BOOSTS = {
    "设置": {"iconpark/Base/setting.svg"},
    "配置": {"iconpark/Base/setting.svg", "iconpark/Base/config.svg"},
    "目标": {"iconpark/Base/aiming.svg", "iconpark/Sports/target-one.svg"},
    "增长": {"iconpark/Charts/positive-dynamics.svg"},
    "趋势": {"iconpark/Charts/chart-line.svg", "iconpark/Charts/positive-dynamics.svg"},
    "占比": {"iconpark/Charts/chart-proportion.svg"},
    "数据": {"iconpark/Charts/data-screen.svg"},
    "看板": {"iconpark/Charts/data-screen.svg"},
    "成功": {"iconpark/Character/check-one.svg"},
    "完成": {"iconpark/Character/check-one.svg"},
    "失败": {"iconpark/Character/close-one.svg"},
    "风险": {"iconpark/Character/close-one.svg"},
    "团队": {"iconpark/Peoples/peoples.svg"},
    "用户": {"iconpark/Peoples/peoples.svg", "iconpark/Peoples/user.svg"},
    "安全": {"iconpark/Safe/protect.svg"},
    "防护": {"iconpark/Safe/protect.svg"},
    "全球": {"iconpark/Travel/world.svg"},
    "市场": {"iconpark/Travel/world.svg"},
    "邮件": {"iconpark/Office/envelope-one.svg"},
    "联系": {"iconpark/Office/envelope-one.svg"},
    "会议": {"iconpark/Office/schedule.svg"},
    "日程": {"iconpark/Office/schedule.svg"},
    "飞书": {"iconpark/Brand/bydesign.svg"},
}
CURATED_BOOST_SCORE = 40


class IconParkToolError(Exception):
    pass


def fail(message: str) -> None:
    raise IconParkToolError(message)


def normalize_whitespace(value: str) -> str:
    return re.sub(r"\s+", " ", value).strip()


def normalize_token(value: str) -> str:
    return normalize_whitespace(value.lower().replace("_", "-"))


def append_unique(target: list[str], token: str) -> None:
    normalized = normalize_token(token)
    if normalized and normalized not in target:
        target.append(normalized)


def tokenize_query(value: str) -> list[str]:
    normalized = normalize_token(value)
    if not normalized:
        return []

    tokens: list[str] = []
    for item in re.split(r"[\s,/|，。；;：:（）()【】\[\]《》<>]+", normalized):
        append_unique(tokens, item)

    for phrase in re.findall(r"[\u3400-\u9fff]+", normalized):
        if len(phrase) < 2:
            continue
        max_size = min(6, len(phrase))
        for size in range(max_size, 1, -1):
            for start in range(0, len(phrase) - size + 1):
                append_unique(tokens, phrase[start : start + size])

    synonym_tokens = {
        "目标": ["aim", "target", "goal"],
        "聚焦": ["focus", "target"],
        "增长": ["growth", "trend", "positive"],
        "趋势": ["trend", "chart", "line"],
        "数据": ["data", "analytics", "chart"],
        "指标": ["metric", "data"],
        "看板": ["dashboard", "screen", "data"],
        "成功": ["success", "check", "done"],
        "完成": ["done", "success", "check"],
        "失败": ["fail", "close", "risk"],
        "风险": ["risk", "fail", "protect"],
        "安全": ["safe", "security", "protect"],
        "配置": ["config", "setting", "system"],
        "设置": ["setting", "config"],
        "团队": ["team", "people", "users"],
        "用户": ["user", "people"],
        "全球": ["global", "world", "earth"],
        "市场": ["market", "world", "business"],
        "邮件": ["mail", "message"],
        "mail": ["message", "envelope", "envelope-one"],
        "计划": ["plan", "schedule"],
        "时间": ["time", "schedule"],
        "学习": ["learning", "education", "book"],
        "培训": ["training", "education"],
        "自动化": ["automation", "ai"],
        "ai": ["ai", "automation", "magic"],
    }
    for token in list(tokens):
        for keyword, aliases in synonym_tokens.items():
            if is_ascii_token(keyword):
                matches = token == keyword
            else:
                matches = keyword in token
            if matches:
                for alias in aliases:
                    append_unique(tokens, alias)

    return tokens


def is_ascii_token(value: str) -> bool:
    return bool(re.fullmatch(r"[a-z0-9-]+", value))


def allows_substring_match(value: str) -> bool:
    return not is_ascii_token(value) or len(value) >= 3


def field_tokens(*values: str) -> set[str]:
    tokens: set[str] = set()
    for value in values:
        normalized = normalize_token(value)
        if not normalized:
            continue
        tokens.add(normalized)
        for part in re.split(r"[-\s]+", normalized):
            if part:
                tokens.add(part)
    return tokens


def load_index(path: str | Path = DEFAULT_INDEX_PATH) -> dict[str, Any]:
    index_path = Path(path)
    if not index_path.exists():
        fail(f"iconpark index not found: {index_path}")
    try:
        index_data = json.loads(index_path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as error:
        fail(f"invalid iconpark index JSON: {error}")
    if not isinstance(index_data.get("icons"), list):
        fail("iconpark index must contain an icons array")
    return index_data


def icon_search_text(entry: dict[str, Any]) -> str:
    parts = [
        entry.get("iconType", ""),
        entry.get("category", ""),
        entry.get("name", ""),
        " ".join(entry.get("tags") or []),
    ]
    return normalize_token(" ".join(parts))


def score_icon(entry: dict[str, Any], query: str, tokens: list[str]) -> int:
    raw_icon_type = entry.get("iconType", "")
    icon_type = normalize_token(raw_icon_type)
    category = normalize_token(entry.get("category", ""))
    name = normalize_token(entry.get("name", ""))
    tags = [normalize_token(tag) for tag in entry.get("tags") or []]
    name_tokens = field_tokens(name)
    category_tokens = field_tokens(category)
    tag_tokens = field_tokens(*tags)
    icon_type_tokens = field_tokens(icon_type)
    search_text = icon_search_text(entry)
    normalized_query = normalize_token(query)

    score = 0
    boosted_keywords: set[str] = set()
    if normalized_query:
        if normalized_query == icon_type or normalized_query == name:
            score += 200
        elif normalized_query in tag_tokens:
            score += 120
        elif normalized_query in icon_type_tokens:
            score += 60
        elif allows_substring_match(normalized_query) and normalized_query in search_text:
            score += 30

    for token in tokens:
        for keyword, boosted_icon_types in CURATED_ICON_BOOSTS.items():
            if keyword in boosted_keywords:
                continue
            if keyword in token and raw_icon_type in boosted_icon_types:
                score += CURATED_BOOST_SCORE
                boosted_keywords.add(keyword)
        if token == name:
            score += 80
        elif token in name_tokens:
            score += 55
        elif allows_substring_match(token) and token in name:
            score += 45
        if token == category:
            score += 35
        elif token in category_tokens:
            score += 25
        elif allows_substring_match(token) and token in category:
            score += 15
        for tag in tags:
            if token == tag:
                score += 60
            elif token in field_tokens(tag):
                score += 45
            elif allows_substring_match(token) and token in tag:
                score += 20
        if token in icon_type_tokens:
            score += 20
        elif allows_substring_match(token) and token in icon_type:
            score += 15

    return score


def parse_limit(value: Any) -> int:
    if value is None or value is False:
        return DEFAULT_LIMIT
    if value is True:
        fail("limit requires an integer value")
    try:
        return int(value)
    except (TypeError, ValueError):
        fail(f"limit must be an integer: {value}")


def public_icon(entry: dict[str, Any], score: int | None = None) -> dict[str, Any]:
    result = {
        "iconType": entry["iconType"],
        "category": entry["category"],
        "name": entry["name"],
        "tags": entry.get("tags") or [],
    }
    if score is not None:
        result["score"] = score
    return result


def search_icons(index_data: dict[str, Any], options: dict[str, Any]) -> list[dict[str, Any]]:
    query = str(options.get("query") or "")
    if not normalize_whitespace(query):
        fail("query is required")
    limit = parse_limit(options.get("limit"))
    category_filter = normalize_token(str(options.get("category") or ""))
    tokens = tokenize_query(query)

    ranked: list[dict[str, Any]] = []
    for entry in index_data["icons"]:
        if category_filter and normalize_token(entry.get("category", "")) != category_filter:
            continue
        score = score_icon(entry, query, tokens)
        if query and score == 0:
            continue
        ranked.append(public_icon(entry, score))

    ranked.sort(key=lambda item: (-int(item["score"]), item["category"], item["name"]))
    return ranked[: max(limit, 0)]


def resolve_icon(index_data: dict[str, Any], name_or_type: str | None) -> dict[str, Any]:
    if not name_or_type:
        fail("name is required")
    target = normalize_token(name_or_type)
    matches = []
    for entry in index_data["icons"]:
        candidates = {
            normalize_token(entry["iconType"]),
            normalize_token(entry["name"]),
            normalize_token(f'{entry["category"]}/{entry["name"]}.svg'),
        }
        if target in candidates:
            matches.append(entry)
    if not matches:
        fail(f"icon not found: {name_or_type}")
    if len(matches) > 1:
        names = ", ".join(entry["iconType"] for entry in matches)
        fail(f"ambiguous icon name: {name_or_type}; matches: {names}")
    return public_icon(matches[0])


def list_categories(index_data: dict[str, Any]) -> list[dict[str, Any]]:
    counts: dict[str, int] = {}
    for entry in index_data["icons"]:
        counts[entry["category"]] = counts.get(entry["category"], 0) + 1
    return [{"category": category, "count": counts[category]} for category in sorted(counts)]


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
        "  python3 iconpark_tool.py search --query <text> [--category <Category>] [--limit 8]",
        "  python3 iconpark_tool.py resolve --name <name|iconType>",
        "  python3 iconpark_tool.py list-categories",
    ]
    print("\n".join(usage), file=sys.stderr)


def write_json(value: Any) -> None:
    print(json.dumps(value, ensure_ascii=False, indent=2))


def run_cli(argv: list[str] | None = None) -> None:
    command, options = parse_cli_args(argv or sys.argv[1:])
    if not command or command in {"--help", "help"}:
        print_usage()
        raise SystemExit(0)

    index_data = load_index()
    if command == "search":
        write_json(search_icons(index_data, options))
        return
    if command == "resolve":
        write_json(resolve_icon(index_data, options.get("name")))
        return
    if command == "list-categories":
        write_json(list_categories(index_data))
        return

    print_usage()
    fail(f"unknown command: {command}")


if __name__ == "__main__":
    try:
        run_cli()
    except IconParkToolError as error:
        print(f"iconpark-tool error: {error}", file=sys.stderr)
        raise SystemExit(1) from error
