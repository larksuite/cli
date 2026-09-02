#!/usr/bin/env python3
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
"""Gate the knowledge_base_bootstrap write plan before any node is written.

This script is the deterministic guard for the `knowledge_base_bootstrap`
workflow's WRITE_CONFIRM state. It reads the write plan (one entry per target
node) and decides, per node, whether the node may be written.

Design principle (mirrors enterprise-kb-ops build_graph.py): an agent's own
readiness claim can only narrow the outcome, never bypass a gate. Hard gates
block the write outright; consistency issues do not block a "先框架后补" draft
but force the page status back to 进行中 so nothing incomplete is passed off
as 已完成.

Input: a JSON object (via --plan <file> or stdin) shaped as:

    {
      "nodes": [
        {
          "node_token": "wikcn_ROOT",
          "title": "建材经贸大厦改造项目验收",
          "obj_type": "docx",              # wiki node object type
          "write_mode": "overwrite",       # overwrite | append | new_docx | skip
          "draft_state": "empty_placeholder",  # empty_placeholder | has_draft
          "parent_node_token": "",         # new_docx: confirmed parent node (alt to space_id)
          "space_id": "",                   # new_docx: confirmed target space (alt to parent)
          "overwrite_confirmed": false,    # user explicitly confirmed rewriting a draft
          "governance": {                  # the 6-row governance table fields
            "source": "据业务常识制定",
            "owner": "待确认",
            "version_status": "v1.0｜进行中",
            "scope_visibility": "全员",
            "effective_update": "生效：2026-09-01｜更新：2026-09-01｜原因：首次创建",
            "review_policy": "类型：流程指引｜周期：90｜下次复核：2026-12-01",
            "page_status": "进行中"
          }
        }
      ]
    }

Output: a JSON object on stdout:

    {
      "ok": true,
      "summary": {"total": N, "writable": X, "blocked": Y, "narrowed": Z},
      "nodes": [
        {"node_token": "...", "title": "...", "ready": true/false,
         "blocked_reasons": [...], "narrowed": true/false,
         "effective_page_status": "进行中|已完成|已废弃"}
      ]
    }

Exit code: 0 when the plan parsed and was evaluated (even if some nodes are
blocked); 2 on malformed input. A non-empty `blocked` count is a normal
result, not a script error.
"""

from __future__ import annotations

import argparse
import json
import sys


GOVERNANCE_FIELDS = {
    "source": "来源",
    "owner": "负责人",
    "version_status": "版本与状态",
    "scope_visibility": "适用与可见范围",
    "effective_update": "生效与更新",
    "review_policy": "复核策略",
    "page_status": "页面状态",
}

# Fields that must carry a real value; "待确认" here does not block a draft but
# does prevent the page from being marked 已完成 (handled as a narrowing).
REQUIRED_NON_EMPTY = ("source", "scope_visibility")

VALID_PAGE_STATUSES = {"进行中", "已完成", "已废弃"}
UNRESOLVED_MARKERS = ("待确认", "待补充", "待指定", "未确认", "未解决", "tbd", "unknown")
VALID_WRITE_MODES = {"overwrite", "append", "new_docx", "skip"}
VALID_DRAFT_STATES = {"empty_placeholder", "has_draft"}


def parse_args() -> argparse.Namespace:
    """Parse CLI arguments (--plan path or stdin)."""
    parser = argparse.ArgumentParser(
        description="Gate the knowledge_base_bootstrap write plan."
    )
    parser.add_argument(
        "--plan",
        help="Path to the write-plan JSON; omit or use '-' to read from stdin",
    )
    return parser.parse_args()


def load_plan(path: str | None) -> dict:
    """Load and shape-check the write-plan JSON from a file or stdin."""
    if not path or path == "-":
        raw = sys.stdin.read()
    else:
        with open(path, encoding="utf-8") as handle:
            raw = handle.read()
    data = json.loads(raw)
    if not isinstance(data, dict) or not isinstance(data.get("nodes"), list):
        raise ValueError("plan must be a JSON object with a 'nodes' array")
    return data


def has_unresolved(value) -> bool:
    """Return True if the value contains an unresolved marker (待确认, TBD, ...)."""
    normalized = str(value or "").strip().lower()
    if not normalized:
        return False
    return any(marker in normalized for marker in UNRESOLVED_MARKERS)


def is_empty(value) -> bool:
    """Return True if the value is empty or whitespace-only."""
    return not str(value or "").strip()


def evaluate_node(node: dict) -> dict:
    """Evaluate one write-plan node and return its gate verdict.

    Applies hard gates (block the write) and a consistency narrowing (allow the
    write but force page status back to 进行中). A node's own claims can only
    narrow the outcome; they can never bypass a hard gate.
    """
    title = str(node.get("title") or node.get("node_token") or "<未命名节点>")
    token = str(node.get("node_token") or "").strip()
    write_mode = str(node.get("write_mode") or "")
    # Normalize a non-dict / missing governance to {} so every downstream
    # access is safe (a truthy non-dict such as a string would otherwise crash
    # governance.get(...), including on the skip branch).
    governance = node.get("governance")
    if not isinstance(governance, dict):
        governance = {}

    hard_reasons: list[str] = []
    narrow_reasons: list[str] = []

    # skip mode is not a write; report it as not-writable without blocking noise.
    if write_mode == "skip":
        return {
            "node_token": token,
            "title": title,
            "ready": False,
            "blocked_reasons": ["计划为 skip，不写入"],
            "narrowed": False,
            "narrow_reasons": [],
            "effective_page_status": str(governance.get("page_status") or ""),
        }

    if write_mode not in VALID_WRITE_MODES:
        hard_reasons.append(f"未知写法：{write_mode or '（空）'}")

    # --- Hard gate 0: a real write needs a stable node target ---
    if not token and write_mode != "new_docx":
        hard_reasons.append("缺少 node_token，无法定位写入目标")
    # new_docx must carry a confirmed destination; otherwise a user-mode create
    # silently falls back to my_library instead of the target space.
    if write_mode == "new_docx":
        if is_empty(node.get("parent_node_token")) and is_empty(node.get("space_id")):
            hard_reasons.append("new_docx 缺少确认的建节点位置（parent_node_token 或 space_id）")

    # --- Hard gate 1: carrier must be a document node ---
    obj_type = str(node.get("obj_type") or "").strip().lower()
    if write_mode == "new_docx":
        # new_docx creates a fresh docx page; it must not target a node that is
        # already a docx (that should be overwrite/append instead).
        if obj_type == "docx":
            hard_reasons.append("new_docx 不能用于已是 docx 的节点，应改用 overwrite/append")
    elif not obj_type:
        hard_reasons.append("未提供节点 obj_type，无法确认载体")
    elif obj_type != "docx":
        hard_reasons.append(f"载体不是文档节点：obj_type={node.get('obj_type')}")

    # --- Hard gate 2: overwrite must know the draft state, and overwriting a
    # real draft needs explicit confirmation. Fails closed: a missing or unknown
    # draft_state blocks overwrite rather than risking a silent draft wipe. ---
    if write_mode == "overwrite":
        draft_state = str(node.get("draft_state") or "").strip()
        if draft_state not in VALID_DRAFT_STATES:
            hard_reasons.append(
                f"覆盖写入的草稿状态未知（draft_state={node.get('draft_state')}），拒绝覆盖"
            )
        elif draft_state == "has_draft" and node.get("overwrite_confirmed") is not True:
            hard_reasons.append("覆盖有草稿的节点但缺少用户显式确认")

    # --- Governance completeness ---
    if not governance:
        hard_reasons.append("缺少 6 行治理表")
    else:
        for field in REQUIRED_NON_EMPTY:
            if is_empty(governance.get(field)):
                hard_reasons.append(f"治理字段缺失：{GOVERNANCE_FIELDS[field]}")

        page_status = str(governance.get("page_status") or "").strip()
        if page_status and page_status not in VALID_PAGE_STATUSES:
            hard_reasons.append(f"页面状态非法：{page_status}")

        # Consistency narrowing: any 待确认 field cannot be sold as 已完成.
        unresolved_fields = [
            GOVERNANCE_FIELDS[key]
            for key in GOVERNANCE_FIELDS
            if key != "page_status" and has_unresolved(governance.get(key))
        ]
        if unresolved_fields and page_status == "已完成":
            narrow_reasons.append(
                "存在待确认字段（" + "、".join(unresolved_fields) + "），状态收紧为进行中"
            )

    ready = not hard_reasons
    effective_status = str(governance.get("page_status") or "")
    narrowed = bool(narrow_reasons)
    if narrowed:
        effective_status = "进行中"

    return {
        "node_token": token,
        "title": title,
        "ready": ready,
        "blocked_reasons": hard_reasons,
        "narrowed": narrowed,
        "narrow_reasons": narrow_reasons,
        "effective_page_status": effective_status,
    }


def main() -> int:
    """Load the plan, evaluate every node, and print the gate result JSON."""
    args = parse_args()
    try:
        plan = load_plan(args.plan)
    except (OSError, ValueError, json.JSONDecodeError) as exc:
        print(json.dumps({"ok": False, "error": str(exc)}, ensure_ascii=False))
        return 2

    results = [evaluate_node(node if isinstance(node, dict) else {}) for node in plan["nodes"]]
    writable = sum(1 for item in results if item["ready"])
    blocked = sum(1 for item in results if not item["ready"])
    narrowed = sum(1 for item in results if item["narrowed"])

    print(json.dumps({
        "ok": True,
        "summary": {
            "total": len(results),
            "writable": writable,
            "blocked": blocked,
            "narrowed": narrowed,
        },
        "nodes": results,
    }, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
