#!/usr/bin/env python3
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
"""Tests for kb_gate.py. Run: python3 kb_gate_test.py"""

from __future__ import annotations

import unittest

import kb_gate


def _governance(**overrides) -> dict:
    base = {
        "source": "据业务常识制定",
        "owner": "张三｜业务团队",
        "version_status": "v1.0｜已完成",
        "scope_visibility": "全员",
        "effective_update": "生效：2026-09-01｜更新：2026-09-01｜原因：首次创建",
        "review_policy": "类型：流程指引｜周期：90｜下次复核：2026-12-01",
        "page_status": "已完成",
    }
    base.update(overrides)
    return base


def _node(**overrides) -> dict:
    base = {
        "node_token": "wikcn_A",
        "title": "消防验收",
        "obj_type": "docx",
        "write_mode": "overwrite",
        "draft_state": "empty_placeholder",
        "overwrite_confirmed": False,
        "governance": _governance(),
    }
    base.update(overrides)
    return base


class KbGateTest(unittest.TestCase):
    def test_complete_docx_is_writable(self):
        result = kb_gate.evaluate_node(_node())
        self.assertTrue(result["ready"])
        self.assertEqual(result["blocked_reasons"], [])
        self.assertFalse(result["narrowed"])

    def test_missing_obj_type_blocks(self):
        result = kb_gate.evaluate_node(_node(obj_type=""))
        self.assertFalse(result["ready"])
        self.assertTrue(any("载体" in r for r in result["blocked_reasons"]))

    def test_non_docx_carrier_blocks(self):
        result = kb_gate.evaluate_node(_node(obj_type="sheet"))
        self.assertFalse(result["ready"])
        self.assertTrue(any("不是文档节点" in r for r in result["blocked_reasons"]))

    def test_new_docx_allows_non_docx_source(self):
        # new_docx creates a fresh docx page beside a non-docx node; not blocked
        # when a confirmed destination is given.
        result = kb_gate.evaluate_node(
            _node(obj_type="sheet", write_mode="new_docx", draft_state="empty_placeholder",
                  parent_node_token="wikcn_PARENT")
        )
        self.assertTrue(result["ready"])

    def test_overwrite_draft_without_confirm_blocks(self):
        result = kb_gate.evaluate_node(
            _node(write_mode="overwrite", draft_state="has_draft", overwrite_confirmed=False)
        )
        self.assertFalse(result["ready"])
        self.assertTrue(any("覆盖有草稿" in r for r in result["blocked_reasons"]))

    def test_overwrite_unknown_draft_state_fails_closed(self):
        # Missing / unknown draft_state must block overwrite, not silently wipe.
        result = kb_gate.evaluate_node(_node(write_mode="overwrite", draft_state=""))
        self.assertFalse(result["ready"])
        self.assertTrue(any("草稿状态未知" in r for r in result["blocked_reasons"]))

    def test_overwrite_bogus_draft_state_fails_closed(self):
        result = kb_gate.evaluate_node(_node(write_mode="overwrite", draft_state="maybe"))
        self.assertFalse(result["ready"])
        self.assertTrue(any("草稿状态未知" in r for r in result["blocked_reasons"]))

    def test_overwrite_draft_with_confirm_ok(self):
        result = kb_gate.evaluate_node(
            _node(write_mode="overwrite", draft_state="has_draft", overwrite_confirmed=True)
        )
        self.assertTrue(result["ready"])

    def test_append_draft_is_writable(self):
        result = kb_gate.evaluate_node(_node(write_mode="append", draft_state="has_draft"))
        self.assertTrue(result["ready"])

    def test_missing_governance_blocks(self):
        result = kb_gate.evaluate_node(_node(governance={}))
        self.assertFalse(result["ready"])
        self.assertTrue(any("6 行治理表" in r for r in result["blocked_reasons"]))

    def test_empty_required_field_blocks(self):
        result = kb_gate.evaluate_node(_node(governance=_governance(source="")))
        self.assertFalse(result["ready"])
        self.assertTrue(any("来源" in r for r in result["blocked_reasons"]))

    def test_invalid_page_status_blocks(self):
        result = kb_gate.evaluate_node(_node(governance=_governance(page_status="草稿")))
        self.assertFalse(result["ready"])
        self.assertTrue(any("页面状态非法" in r for r in result["blocked_reasons"]))

    def test_missing_page_status_blocks(self):
        # An empty page_status must hard-block, not fall through as writable.
        result = kb_gate.evaluate_node(_node(governance=_governance(page_status="")))
        self.assertFalse(result["ready"])
        self.assertTrue(any("页面状态" in r for r in result["blocked_reasons"]))

    def test_unresolved_owner_with_done_status_narrows(self):
        # owner 待确认 but marked 已完成 -> writable draft, status narrowed to 进行中.
        result = kb_gate.evaluate_node(
            _node(governance=_governance(owner="待确认", page_status="已完成"))
        )
        self.assertTrue(result["ready"])
        self.assertTrue(result["narrowed"])
        self.assertEqual(result["effective_page_status"], "进行中")

    def test_unresolved_owner_with_inprogress_not_narrowed(self):
        # owner 待确认 and already 进行中 -> writable, no narrowing needed.
        result = kb_gate.evaluate_node(
            _node(governance=_governance(owner="待确认", page_status="进行中"))
        )
        self.assertTrue(result["ready"])
        self.assertFalse(result["narrowed"])
        self.assertEqual(result["effective_page_status"], "进行中")

    def test_skip_mode_is_not_writable_without_hard_block(self):
        result = kb_gate.evaluate_node(_node(write_mode="skip"))
        self.assertFalse(result["ready"])
        self.assertEqual(result["blocked_reasons"], ["计划为 skip，不写入"])

    def test_unknown_write_mode_blocks(self):
        result = kb_gate.evaluate_node(_node(write_mode="frobnicate"))
        self.assertFalse(result["ready"])
        self.assertTrue(any("未知写法" in r for r in result["blocked_reasons"]))

    def test_skip_with_nondict_governance_does_not_crash(self):
        # Regression: a truthy non-dict governance on a skip entry must not raise.
        result = kb_gate.evaluate_node(_node(write_mode="skip", governance="待确认"))
        self.assertFalse(result["ready"])
        self.assertEqual(result["blocked_reasons"], ["计划为 skip，不写入"])
        self.assertEqual(result["effective_page_status"], "")

    def test_nondict_governance_normalized_blocks(self):
        # A non-dict governance on a real write is treated as missing table.
        result = kb_gate.evaluate_node(_node(governance="oops"))
        self.assertFalse(result["ready"])
        self.assertTrue(any("6 行治理表" in r for r in result["blocked_reasons"]))

    def test_empty_node_token_blocks(self):
        result = kb_gate.evaluate_node(_node(node_token=""))
        self.assertFalse(result["ready"])
        self.assertTrue(any("node_token" in r for r in result["blocked_reasons"]))

    def test_new_docx_without_token_ok(self):
        # new_docx creates a fresh node, so it does not require an existing token,
        # but it does require a confirmed destination (parent or space).
        result = kb_gate.evaluate_node(
            _node(node_token="", obj_type="", write_mode="new_docx", parent_node_token="wikcn_PARENT")
        )
        self.assertTrue(result["ready"])

    def test_new_docx_with_space_ok(self):
        result = kb_gate.evaluate_node(
            _node(node_token="", obj_type="", write_mode="new_docx", space_id="spc_X")
        )
        self.assertTrue(result["ready"])

    def test_new_docx_without_destination_blocks(self):
        # No parent and no space: a user-mode create would fall back to my_library.
        result = kb_gate.evaluate_node(
            _node(node_token="", obj_type="", write_mode="new_docx")
        )
        self.assertFalse(result["ready"])
        self.assertTrue(any("建节点位置" in r for r in result["blocked_reasons"]))

    def test_new_docx_on_existing_docx_blocks(self):
        # new_docx must not target a node that is already docx.
        result = kb_gate.evaluate_node(
            _node(obj_type="docx", write_mode="new_docx", parent_node_token="wikcn_PARENT")
        )
        self.assertFalse(result["ready"])
        self.assertTrue(any("new_docx" in r for r in result["blocked_reasons"]))


if __name__ == "__main__":
    unittest.main()
