#!/usr/bin/env python3
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
"""Tests for publish_gate.py. Run: python3 publish_gate_test.py"""

from __future__ import annotations

import unittest

import publish_gate


def _governance(**overrides) -> dict:
    base = {
        "source": "退货政策原始 Word｜2026-08 版",
        "owner": "李四｜客服团队",
        "version_status": "v1.0｜已完成",
        "scope_visibility": "全员",
        "effective_update": "生效：2026-09-01｜更新：2026-09-01｜原因：首次入库",
        "review_policy": "类型：政策｜周期：180｜下次复核：2027-03-01",
        "page_status": "已完成",
    }
    base.update(overrides)
    return base


def _page(**overrides) -> dict:
    base = {
        "source_id": "sha_abc",
        "title": "退货政策说明",
        "publish_role": "knowledge_page",
        "write_via": "docs_update",
        "proposed_action": "add",
        "target_obj_type": "docx",
        "target_token": "wikcn_NODE",
        "sensitivity": "internal",
        "sensitive_review_status": "",
        "conflict_status": "none",
        "parse_status": "parsed",
        "attachment_confirmed": False,
        "governance": _governance(),
    }
    base.update(overrides)
    return base


def _attachment(**overrides) -> dict:
    base = {
        "source_id": "sha_file",
        "title": "退货政策原件.pdf",
        "publish_role": "source_attachment",
        "write_via": "drive_upload",
        "target_token": "wikcn_NODE",
        "sensitivity": "internal",
        "conflict_status": "none",
        "attachment_confirmed": True,
    }
    base.update(overrides)
    return base


class KnowledgePageGateTest(unittest.TestCase):
    def test_complete_page_is_writable(self):
        result = publish_gate.evaluate_item(_page())
        self.assertTrue(result["ready"])
        self.assertEqual(result["blocked_reasons"], [])
        self.assertTrue(result["counts_as_page"])

    def test_non_docx_carrier_blocks(self):
        result = publish_gate.evaluate_item(_page(target_obj_type="file"))
        self.assertFalse(result["ready"])
        self.assertTrue(any("载体不是 docx" in r for r in result["blocked_reasons"]))

    def test_missing_obj_type_blocks(self):
        result = publish_gate.evaluate_item(_page(target_obj_type=""))
        self.assertFalse(result["ready"])
        self.assertTrue(any("target_obj_type" in r for r in result["blocked_reasons"]))

    def test_drive_upload_cannot_complete_page(self):
        # The single most important iron rule.
        result = publish_gate.evaluate_item(_page(write_via="drive_upload"))
        self.assertFalse(result["ready"])
        self.assertTrue(any("drive_upload 只能上传原文件" in r for r in result["blocked_reasons"]))
        self.assertFalse(result["counts_as_page"])

    def test_unknown_write_via_blocks(self):
        result = publish_gate.evaluate_item(_page(write_via="magic"))
        self.assertFalse(result["ready"])
        self.assertTrue(any("未知写法" in r for r in result["blocked_reasons"]))

    def test_update_via_import_docx_blocks(self):
        # update/merge must not route through import_docx (would dup a child page).
        for action in ("update", "merge"):
            result = publish_gate.evaluate_item(
                _page(proposed_action=action, write_via="import_docx")
            )
            self.assertFalse(result["ready"], action)
            self.assertTrue(any("import_docx" in r for r in result["blocked_reasons"]), action)

    def test_update_via_docs_update_ok(self):
        result = publish_gate.evaluate_item(_page(proposed_action="update", write_via="docs_update"))
        self.assertTrue(result["ready"])

    def test_add_via_import_docx_ok(self):
        result = publish_gate.evaluate_item(_page(proposed_action="add", write_via="import_docx"))
        self.assertTrue(result["ready"])

    def test_unknown_action_fails_closed(self):
        result = publish_gate.evaluate_item(_page(proposed_action="publish"))
        self.assertFalse(result["ready"])
        self.assertTrue(any("处置动作未分类或非法" in r for r in result["blocked_reasons"]))

    def test_missing_action_fails_closed(self):
        result = publish_gate.evaluate_item(_page(proposed_action=""))
        self.assertFalse(result["ready"])
        self.assertTrue(any("处置动作未分类或非法" in r for r in result["blocked_reasons"]))

    def test_missing_token_blocks(self):
        result = publish_gate.evaluate_item(_page(target_token=""))
        self.assertFalse(result["ready"])
        self.assertTrue(any("target_token" in r for r in result["blocked_reasons"]))

    def test_node_create_docx_with_parent_ok(self):
        # node_create_docx makes a fresh node, so it needs no existing token,
        # but it does need a confirmed destination (parent or space).
        result = publish_gate.evaluate_item(
            _page(write_via="node_create_docx", target_token="", parent_token="wikcn_PARENT")
        )
        self.assertTrue(result["ready"])

    def test_node_create_docx_with_space_ok(self):
        result = publish_gate.evaluate_item(
            _page(write_via="node_create_docx", target_token="", space_id="spc_X")
        )
        self.assertTrue(result["ready"])

    def test_node_create_docx_without_destination_blocks(self):
        # No parent and no space: a user-mode create would fall back to my_library.
        result = publish_gate.evaluate_item(
            _page(write_via="node_create_docx", target_token="")
        )
        self.assertFalse(result["ready"])
        self.assertTrue(any("建节点位置" in r for r in result["blocked_reasons"]))

    def test_import_docx_is_writable(self):
        result = publish_gate.evaluate_item(_page(write_via="import_docx"))
        self.assertTrue(result["ready"])

    def test_prohibited_sensitivity_blocks(self):
        result = publish_gate.evaluate_item(_page(sensitivity="prohibited"))
        self.assertFalse(result["ready"])
        self.assertTrue(any("prohibited" in r for r in result["blocked_reasons"]))

    def test_restricted_without_approval_blocks(self):
        result = publish_gate.evaluate_item(
            _page(sensitivity="restricted", sensitive_review_status="pending")
        )
        self.assertFalse(result["ready"])
        self.assertTrue(any("受限敏感" in r for r in result["blocked_reasons"]))

    def test_restricted_with_approval_ok(self):
        result = publish_gate.evaluate_item(
            _page(sensitivity="restricted", sensitive_review_status="approved")
        )
        self.assertTrue(result["ready"])

    def test_unresolved_conflict_blocks(self):
        for state in ("suspected", "confirmed"):
            result = publish_gate.evaluate_item(_page(conflict_status=state))
            self.assertFalse(result["ready"], state)
            self.assertTrue(any("未裁决冲突" in r for r in result["blocked_reasons"]), state)

    def test_resolved_conflict_ok(self):
        result = publish_gate.evaluate_item(_page(conflict_status="resolved"))
        self.assertTrue(result["ready"])

    def test_unsupported_parse_blocks(self):
        result = publish_gate.evaluate_item(_page(parse_status="unsupported"))
        self.assertFalse(result["ready"])
        self.assertTrue(any("无法解析" in r for r in result["blocked_reasons"]))

    def test_unknown_sensitivity_fails_closed(self):
        # A misspelled / unclassified sensitivity must block, not pass as safe.
        result = publish_gate.evaluate_item(_page(sensitivity="機密"))
        self.assertFalse(result["ready"])
        self.assertTrue(any("敏感等级未分类或非法" in r for r in result["blocked_reasons"]))

    def test_missing_sensitivity_fails_closed(self):
        result = publish_gate.evaluate_item(_page(sensitivity=""))
        self.assertFalse(result["ready"])
        self.assertTrue(any("敏感等级未分类或非法" in r for r in result["blocked_reasons"]))

    def test_unknown_conflict_fails_closed(self):
        result = publish_gate.evaluate_item(_page(conflict_status="conflict"))
        self.assertFalse(result["ready"])
        self.assertTrue(any("冲突状态未分类或非法" in r for r in result["blocked_reasons"]))

    def test_unknown_parse_fails_closed(self):
        result = publish_gate.evaluate_item(_page(parse_status="ok"))
        self.assertFalse(result["ready"])
        self.assertTrue(any("解析状态未分类或非法" in r for r in result["blocked_reasons"]))

    def test_missing_page_status_blocks(self):
        result = publish_gate.evaluate_item(_page(governance=_governance(page_status="")))
        self.assertFalse(result["ready"])
        self.assertTrue(any("页面状态" in r for r in result["blocked_reasons"]))

    def test_missing_governance_blocks(self):
        result = publish_gate.evaluate_item(_page(governance={}))
        self.assertFalse(result["ready"])
        self.assertTrue(any("6 行治理表" in r for r in result["blocked_reasons"]))

    def test_empty_required_field_blocks(self):
        result = publish_gate.evaluate_item(_page(governance=_governance(source="")))
        self.assertFalse(result["ready"])
        self.assertTrue(any("来源" in r for r in result["blocked_reasons"]))

    def test_invalid_page_status_blocks(self):
        result = publish_gate.evaluate_item(_page(governance=_governance(page_status="草稿")))
        self.assertFalse(result["ready"])
        self.assertTrue(any("页面状态非法" in r for r in result["blocked_reasons"]))

    def test_unresolved_field_with_done_narrows(self):
        result = publish_gate.evaluate_item(
            _page(governance=_governance(owner="待确认", page_status="已完成"))
        )
        self.assertTrue(result["ready"])
        self.assertTrue(result["narrowed"])
        self.assertEqual(result["effective_page_status"], "进行中")

    def test_partial_parse_with_done_narrows(self):
        result = publish_gate.evaluate_item(
            _page(parse_status="partial", governance=_governance(page_status="已完成"))
        )
        self.assertTrue(result["ready"])
        self.assertTrue(result["narrowed"])
        self.assertEqual(result["effective_page_status"], "进行中")
        self.assertTrue(any("部分解析" in r for r in result["narrow_reasons"]))

    def test_partial_parse_already_inprogress_not_narrowed(self):
        result = publish_gate.evaluate_item(
            _page(parse_status="partial", governance=_governance(page_status="进行中"))
        )
        self.assertTrue(result["ready"])
        self.assertFalse(result["narrowed"])

    def test_nondict_governance_blocks(self):
        result = publish_gate.evaluate_item(_page(governance="oops"))
        self.assertFalse(result["ready"])
        self.assertTrue(any("6 行治理表" in r for r in result["blocked_reasons"]))


class AttachmentGateTest(unittest.TestCase):
    def test_confirmed_attachment_ok(self):
        result = publish_gate.evaluate_item(_attachment())
        self.assertTrue(result["ready"])
        self.assertFalse(result["counts_as_page"])
        self.assertEqual(result["effective_page_status"], "")

    def test_attachment_without_confirm_blocks(self):
        result = publish_gate.evaluate_item(_attachment(attachment_confirmed=False))
        self.assertFalse(result["ready"])
        self.assertTrue(any("显式确认" in r for r in result["blocked_reasons"]))

    def test_attachment_wrong_write_via_blocks(self):
        result = publish_gate.evaluate_item(_attachment(write_via="docs_update"))
        self.assertFalse(result["ready"])
        self.assertTrue(any("只能用 drive_upload" in r for r in result["blocked_reasons"]))

    def test_attachment_never_counts_as_page(self):
        result = publish_gate.evaluate_item(_attachment())
        self.assertFalse(result["counts_as_page"])

    def test_attachment_without_target_blocks(self):
        result = publish_gate.evaluate_item(_attachment(target_token=""))
        self.assertFalse(result["ready"])
        self.assertTrue(any("目标 Wiki 节点" in r for r in result["blocked_reasons"]))

    def test_prohibited_attachment_blocks(self):
        result = publish_gate.evaluate_item(_attachment(sensitivity="prohibited"))
        self.assertFalse(result["ready"])


class UnknownRoleTest(unittest.TestCase):
    def test_unknown_role_blocks(self):
        result = publish_gate.evaluate_item(_page(publish_role="whatever"))
        self.assertFalse(result["ready"])
        self.assertTrue(any("未知 publish_role" in r for r in result["blocked_reasons"]))

    def test_missing_role_blocks(self):
        item = _page()
        del item["publish_role"]
        result = publish_gate.evaluate_item(item)
        self.assertFalse(result["ready"])


if __name__ == "__main__":
    unittest.main()
