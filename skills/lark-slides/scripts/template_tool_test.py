# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
from __future__ import annotations

import importlib.util
import unittest
from pathlib import Path


SCRIPT_DIR = Path(__file__).resolve().parent
TEMPLATE_TOOL_PATH = SCRIPT_DIR / "template-tool.py"

spec = importlib.util.spec_from_file_location("template_tool", TEMPLATE_TOOL_PATH)
template_tool = importlib.util.module_from_spec(spec)
assert spec and spec.loader
spec.loader.exec_module(template_tool)


class TemplateToolTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.index_data = template_tool.build_index_data()

    def test_build_index_data_exposes_light_general_metadata(self) -> None:
        template = next(
            entry for entry in self.index_data["templates"] if entry["template_id"] == "office--light_general"
        )
        self.assertEqual(template["tone"], "light")
        self.assertEqual(template["formality"], "formal")
        self.assertEqual(template["slide_count"], 54)
        self.assertEqual(template["presentation_title"], "白底通用模板")
        self.assertTrue(template["theme_summary"]["has_theme_node"])
        self.assertIsInstance(template["layout_tags"], list)
        self.assertIn("bbox_summary", template)

    def test_search_templates_keeps_work_report_templates_in_top_results(self) -> None:
        results = template_tool.search_templates(self.index_data, {"query": "工作汇报", "limit": 3})
        self.assertTrue(results)
        self.assertTrue(any(entry["template_id"] == "office--work_report" for entry in results))

    def test_extract_selection_xml_keeps_only_requested_slides_and_theme(self) -> None:
        xml = template_tool.extract_selection_xml(self.index_data, "office--light_general", {"label": "封面"})
        self.assertEqual(len(template_tool.re.findall(r"<slide\b", xml)), 2)
        self.assertIn("<theme>", xml)
        self.assertIn("<title>白底通用模板</title>", xml)

    def test_summarize_selection_aggregates_slide_titles_and_counts(self) -> None:
        summary = template_tool.summarize_selection(self.index_data, "office--light_general", {"label": "封面"})
        self.assertEqual(summary["selection"]["range"], "1-2")
        self.assertEqual(summary["summary"]["slide_count"], 2)
        self.assertIn("通用模板", summary["summary"]["title_hints"])
        self.assertGreater(summary["summary"]["element_totals"]["shape"], 0)
        self.assertIsInstance(summary["slides"][0]["layout_tags"], list)
        self.assertIn("bbox_summary", summary["slides"][0])

    def test_search_templates_supports_layout_tag_filtering(self) -> None:
        results = template_tool.search_templates(
            self.index_data,
            {"query": "", "layout-tag": "full-bleed-image-caption", "limit": 10},
        )
        self.assertTrue(results)
        self.assertTrue(
            any("full-bleed-image-caption" in entry["layout_tags"] for entry in results)
        )


if __name__ == "__main__":
    unittest.main()
