#!/usr/bin/env python3
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
"""Gate the knowledge_ingest publish plan before any material is written.

This script is the deterministic guard for the `knowledge_ingest` workflow's
PUBLISH_PLAN state. It reads the publish plan (one entry per material -> target
node mapping) and decides, per item, whether that material may be converted and
written into the knowledge base.

Design principle (mirrors kb_gate.py and enterprise-kb-ops build_graph.py): an
agent's own readiness claim can only narrow the outcome, never bypass a gate.
Hard gates block the write outright; consistency issues do not block a
"先框架后补" draft but force the page status back to 进行中 so nothing
incomplete or partially-parsed is passed off as 已完成.

The core iron rules this gate enforces:
  - a `knowledge_page` must land on an `obj_type=docx` node with a searchable
    body; a non-docx carrier is a hard block;
  - `drive +upload` (write_via=drive_upload) only produces a `file` node, so it
    can never complete a knowledge_page -- it is allowed ONLY for a
    `source_attachment`, and even then only when the user has explicitly opted
    in (attachment_confirmed), and it never advances page status;
  - sensitive (prohibited, or restricted without an approved review) and
    unresolved-conflict materials are blocked from production.

Input: a JSON object (via --plan <file> or stdin) shaped as:

    {
      "items": [
        {
          "source_id": "<sha256 or stable id>",
          "title": "退货政策说明",
          "publish_role": "knowledge_page",   # knowledge_page | source_attachment
          "write_via": "docs_update",          # docs_update | import_docx | node_create_docx | drive_upload
          "proposed_action": "add",             # add | update | merge | reference | review | skip (fail-closed)
          "target_obj_type": "docx",           # target wiki node object type
          "target_token": "wikcn_NODE",        # node/obj token (blank ok for node_create_docx)
          "parent_token": "",                   # node_create_docx: confirmed parent node token
          "space_id": "",                       # node_create_docx: confirmed target space (alt to parent)
          "sensitivity": "internal",           # public | internal | restricted | prohibited (fail-closed)
          "sensitive_review_status": "",        # for restricted: approved | pending | ...
          "conflict_status": "none",           # none | suspected | confirmed | resolved (fail-closed)
          "parse_status": "parsed",            # parsed | partial | unsupported | failed (fail-closed)
          "attachment_confirmed": false,        # user opted in to uploading the original file
          "governance": {                       # 6-row governance table (knowledge_page only)
            "source": "退货政策原始 Word｜2026-08 版",
            "owner": "李四｜客服团队",
            "version_status": "v1.0｜已完成",
            "scope_visibility": "全员",
            "effective_update": "生效：2026-09-01｜更新：2026-09-01｜原因：首次入库",
            "review_policy": "类型：政策｜周期：180｜下次复核：2027-03-01",
            "page_status": "已完成"
          }
        }
      ]
    }

Output: a JSON object on stdout:

    {
      "ok": true,
      "summary": {"total": N, "writable": X, "blocked": Y, "narrowed": Z, "attachments": A},
      "items": [
        {"source_id": "...", "title": "...", "publish_role": "...",
         "ready": true/false, "blocked_reasons": [...], "narrowed": true/false,
         "narrow_reasons": [...], "counts_as_page": true/false,
         "effective_page_status": "进行中|已完成|已废弃|"}
      ]
    }

Exit code: 0 when the plan parsed and was evaluated (even if some items are
blocked); 2 on malformed input. A non-empty `blocked` count is a normal result,
not a script error.
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

# Fields that must carry a real value on a knowledge_page; "待确认" here does not
# block a draft but does prevent the page from being marked 已完成 (a narrowing).
REQUIRED_NON_EMPTY = ("source", "scope_visibility")

VALID_PAGE_STATUSES = {"进行中", "已完成", "已废弃"}
UNRESOLVED_MARKERS = ("待确认", "待补充", "待指定", "未确认", "未解决", "tbd", "unknown")

VALID_PUBLISH_ROLES = {"knowledge_page", "source_attachment"}
# write_via paths that produce a searchable docx knowledge page.
PAGE_WRITE_VIA = {"docs_update", "import_docx", "node_create_docx"}
# write_via that only uploads the original file as an attachment node.
ATTACHMENT_WRITE_VIA = "drive_upload"
VALID_WRITE_VIA = PAGE_WRITE_VIA | {ATTACHMENT_WRITE_VIA}

# Closed triage enums. Values outside these sets are rejected (fail closed)
# rather than treated as safe, so a typo or an unclassified item cannot bypass
# the sensitivity / conflict / parse gates.
VALID_SENSITIVITY = {"public", "internal", "restricted", "prohibited"}
VALID_CONFLICT = {"none", "suspected", "confirmed", "resolved"}
VALID_PARSE = {"parsed", "partial", "unsupported", "failed"}
# Closed proposed_action enum. Actions split into those that publish a page and
# those that do not; a non-publishing action must never reach a write.
VALID_ACTIONS = {"add", "update", "merge", "reference", "review", "skip"}
# Actions that update an existing page: they must use docs_update (import_docx or
# node_create_docx would create a duplicate child instead of updating).
UPDATE_ACTIONS = {"update", "merge"}
# Non-publishing actions: they carry no write and must not be marked ready.
NONPUBLISH_ACTIONS = {"reference", "review", "skip"}
# Conflict states that block production until a human resolves them.
BLOCKING_CONFLICTS = {"suspected", "confirmed"}
# Parse states that cannot yield a searchable body at all.
UNUSABLE_PARSE = {"unsupported", "failed"}


def parse_args() -> argparse.Namespace:
    """Parse CLI arguments (--plan path or stdin)."""
    parser = argparse.ArgumentParser(
        description="Gate the knowledge_ingest publish plan."
    )
    parser.add_argument(
        "--plan",
        help="Path to the publish-plan JSON; omit or use '-' to read from stdin",
    )
    return parser.parse_args()


def load_plan(path: str | None) -> dict:
    """Load and shape-check the publish-plan JSON from a file or stdin."""
    if not path or path == "-":
        raw = sys.stdin.read()
    else:
        with open(path, encoding="utf-8") as handle:
            raw = handle.read()
    data = json.loads(raw)
    if not isinstance(data, dict) or not isinstance(data.get("items"), list):
        raise ValueError("plan must be a JSON object with an 'items' array")
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


def _check_sensitivity_conflict(item: dict, hard_reasons: list[str]) -> None:
    """Shared gates: sensitive and unresolved-conflict material never ships.

    Both enums fail closed: a missing or unrecognized value is blocked rather
    than treated as safe, so an unclassified or misspelled triage state cannot
    slip past the gate.
    """
    sensitivity = str(item.get("sensitivity") or "").strip().lower()
    if sensitivity not in VALID_SENSITIVITY:
        hard_reasons.append(f"敏感等级未分类或非法（sensitivity={item.get('sensitivity')}）")
    elif sensitivity == "prohibited":
        hard_reasons.append("敏感等级 prohibited，禁止入库")
    elif sensitivity == "restricted":
        review = str(item.get("sensitive_review_status") or "").strip().lower()
        if review != "approved":
            hard_reasons.append("受限敏感内容未通过审核（sensitive_review_status 非 approved）")

    conflict = str(item.get("conflict_status") or "").strip().lower()
    if conflict not in VALID_CONFLICT:
        hard_reasons.append(f"冲突状态未分类或非法（conflict_status={item.get('conflict_status')}）")
    elif conflict in BLOCKING_CONFLICTS:
        hard_reasons.append(f"存在未裁决冲突（conflict_status={conflict}）")


def _evaluate_attachment(item: dict, title: str, source_id: str) -> dict:
    """Evaluate a source_attachment item.

    An attachment is the original file uploaded via drive +upload. It never
    carries a governance table and never counts as a completed knowledge page;
    it requires an explicit user opt-in (attachment_confirmed).
    """
    hard_reasons: list[str] = []
    write_via = str(item.get("write_via") or "")

    if write_via != ATTACHMENT_WRITE_VIA:
        hard_reasons.append(
            f"source_attachment 只能用 drive_upload，当前 write_via={write_via or '（空）'}"
        )
    if item.get("attachment_confirmed") is not True:
        hard_reasons.append("上传原文件需用户显式确认（attachment_confirmed）")
    # The upload must be anchored to a confirmed Wiki node; without it the file
    # lands in the Drive root instead of the knowledge base the user approved.
    if is_empty(item.get("target_token")):
        hard_reasons.append("附件缺少目标 Wiki 节点（target_token），拒绝上传到未确认位置")

    _check_sensitivity_conflict(item, hard_reasons)

    return {
        "source_id": source_id,
        "title": title,
        "publish_role": "source_attachment",
        "ready": not hard_reasons,
        "blocked_reasons": hard_reasons,
        "narrowed": False,
        "narrow_reasons": [],
        "counts_as_page": False,
        "effective_page_status": "",
    }


def _evaluate_knowledge_page(item: dict, title: str, source_id: str) -> dict:
    """Evaluate a knowledge_page item against the iron rules and governance.

    A knowledge_page must land on a docx node with a searchable body. Its own
    claims can only narrow the outcome; they can never bypass a hard gate.
    """
    hard_reasons: list[str] = []
    narrow_reasons: list[str] = []

    write_via = str(item.get("write_via") or "")
    token = str(item.get("target_token") or "").strip()
    obj_type = str(item.get("target_obj_type") or "").strip().lower()
    action = str(item.get("proposed_action") or "").strip().lower()

    # --- Hard gate: write_via must be a page-producing path ---
    if write_via not in VALID_WRITE_VIA:
        hard_reasons.append(f"未知写法：{write_via or '（空）'}")
    elif write_via == ATTACHMENT_WRITE_VIA:
        # The single most important rule: uploading the original file cannot
        # complete a knowledge page.
        hard_reasons.append("drive_upload 只能上传原文件，不能完成知识页")

    # --- Hard gate: proposed_action must be known, must be a publishing action,
    # and must match the write_via path. update/merge require docs_update (any
    # other path would create a duplicate child instead of updating the target);
    # add requires a page-creating path (import_docx / docs_update /
    # node_create_docx). A non-publishing action must never reach a write. ---
    if action not in VALID_ACTIONS:
        hard_reasons.append(f"处置动作未分类或非法（proposed_action={item.get('proposed_action')}）")
    elif action in NONPUBLISH_ACTIONS:
        hard_reasons.append(f"非发布动作（{action}）不应作为知识页写入")
    elif action in UPDATE_ACTIONS and write_via != "docs_update":
        hard_reasons.append(f"update/merge 只能用 docs_update 更新既有页，当前 write_via={write_via or '（空）'}（避免新增重复页）")

    # --- Hard gate: a real write needs a stable target ---
    if not token and write_via != "node_create_docx":
        hard_reasons.append("缺少 target_token，无法定位写入目标")
    # A new node must carry a confirmed destination; otherwise a user-mode
    # create silently falls back to my_library instead of the target space.
    if write_via == "node_create_docx":
        if is_empty(item.get("parent_token")) and is_empty(item.get("space_id")):
            hard_reasons.append("new_docx 缺少确认的建节点位置（parent_token 或 space_id）")

    # --- Hard gate: carrier must be a docx node (the core iron rule) ---
    if not obj_type:
        hard_reasons.append("未提供 target_obj_type，无法确认载体")
    elif obj_type != "docx":
        hard_reasons.append(f"知识页载体不是 docx 节点：target_obj_type={item.get('target_obj_type')}")

    # --- Hard gate: material must parse into a searchable body ---
    # parse_status fails closed: an unclassified or misspelled value is blocked
    # rather than assumed parseable.
    parse_status = str(item.get("parse_status") or "").strip().lower()
    if parse_status not in VALID_PARSE:
        hard_reasons.append(f"解析状态未分类或非法（parse_status={item.get('parse_status')}）")
    elif parse_status in UNUSABLE_PARSE:
        hard_reasons.append(f"资料无法解析为可检索正文（parse_status={parse_status}）")

    # --- Shared gates: sensitivity + conflict ---
    _check_sensitivity_conflict(item, hard_reasons)

    # --- Governance completeness ---
    governance = item.get("governance")
    if not isinstance(governance, dict):
        governance = {}
    if not governance:
        hard_reasons.append("缺少 6 行治理表")
        page_status = ""
    else:
        for field in REQUIRED_NON_EMPTY:
            if is_empty(governance.get(field)):
                hard_reasons.append(f"治理字段缺失：{GOVERNANCE_FIELDS[field]}")

        page_status = str(governance.get("page_status") or "").strip()
        if not page_status:
            hard_reasons.append(f"治理字段缺失：{GOVERNANCE_FIELDS['page_status']}")
        elif page_status not in VALID_PAGE_STATUSES:
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
        # Consistency narrowing: a partial parse must not be sold as 已完成.
        if parse_status == "partial" and page_status == "已完成":
            narrow_reasons.append("资料仅部分解析（parse_status=partial），状态收紧为进行中")

    ready = not hard_reasons
    effective_status = page_status
    narrowed = bool(narrow_reasons)
    if narrowed:
        effective_status = "进行中"

    return {
        "source_id": source_id,
        "title": title,
        "publish_role": "knowledge_page",
        "ready": ready,
        "blocked_reasons": hard_reasons,
        "narrowed": narrowed,
        "narrow_reasons": narrow_reasons,
        "counts_as_page": ready,
        "effective_page_status": effective_status,
    }


def evaluate_item(item: dict) -> dict:
    """Evaluate one publish-plan item and return its gate verdict."""
    title = str(item.get("title") or item.get("source_id") or "<未命名资料>")
    source_id = str(item.get("source_id") or "").strip()
    role = str(item.get("publish_role") or "").strip()

    if role == "source_attachment":
        return _evaluate_attachment(item, title, source_id)
    if role == "knowledge_page":
        return _evaluate_knowledge_page(item, title, source_id)

    return {
        "source_id": source_id,
        "title": title,
        "publish_role": role,
        "ready": False,
        "blocked_reasons": [f"未知 publish_role：{role or '（空）'}"],
        "narrowed": False,
        "narrow_reasons": [],
        "counts_as_page": False,
        "effective_page_status": "",
    }


def main() -> int:
    """Load the plan, evaluate every item, and print the gate result JSON."""
    args = parse_args()
    try:
        plan = load_plan(args.plan)
    except (OSError, ValueError, json.JSONDecodeError) as exc:
        print(json.dumps({"ok": False, "error": str(exc)}, ensure_ascii=False))
        return 2

    results = [evaluate_item(item if isinstance(item, dict) else {}) for item in plan["items"]]
    writable = sum(1 for item in results if item["ready"])
    blocked = sum(1 for item in results if not item["ready"])
    narrowed = sum(1 for item in results if item["narrowed"])
    attachments = sum(1 for item in results if item["publish_role"] == "source_attachment")

    print(json.dumps({
        "ok": True,
        "summary": {
            "total": len(results),
            "writable": writable,
            "blocked": blocked,
            "narrowed": narrowed,
            "attachments": attachments,
        },
        "items": results,
    }, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
