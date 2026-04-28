# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

import template_tool


class TemplateToolTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.index_data = template_tool.build_index_data()

    def test_build_index_data_exposes_light_general_metadata(self) -> None:
        template = next(
            entry for entry in self.index_data["templates"] if entry["template_id"] == "office--light_general"
        )
        expected_keys = {
            "template_id",
            "category",
            "category_label",
            "scene",
            "tone",
            "formality",
            "is_general_template",
            "slide_count",
            "presentation_title",
            "palette",
            "structure",
            "page_types",
            "layout_tags",
            "use_cases",
            "ranges",
        }
        self.assertEqual(set(template.keys()), expected_keys)
        self.assertEqual(template["tone"], "light")
        self.assertEqual(template["formality"], "formal")
        self.assertEqual(template["slide_count"], 54)
        self.assertEqual(template["presentation_title"], "白底通用模板")
        self.assertIsInstance(template["layout_tags"], list)
        self.assertNotIn("theme_summary", template)
        self.assertNotIn("editable_regions", template)
        self.assertNotIn("bbox_summary", template)

    def test_search_templates_keeps_work_report_templates_in_top_results(self) -> None:
        results = template_tool.search_templates(self.index_data, {"query": "工作汇报", "limit": 3})
        self.assertTrue(results)
        self.assertTrue(any(entry["template_id"] == "office--work_report" for entry in results))

    def test_search_templates_extracts_scene_from_long_chinese_prompt(self) -> None:
        results = template_tool.search_templates(
            self.index_data,
            {"query": "帮我做一个季度工作汇报PPT，偏正式", "limit": 3},
        )
        self.assertTrue(results)
        self.assertTrue(any(entry["template_id"] == "office--work_report" for entry in results))

    def test_search_templates_maps_chinese_tone_words(self) -> None:
        results = template_tool.search_templates(
            self.index_data,
            {"query": "深色科技感产品发布", "limit": 5},
        )
        self.assertTrue(results)
        self.assertTrue(any(entry["tone"] == "dark" for entry in results))

    def test_search_templates_finds_product_launch_and_promotion_defense(self) -> None:
        product_results = template_tool.search_templates(
            self.index_data,
            {"query": "产品发布会新品介绍", "limit": 5},
        )
        self.assertTrue(product_results)
        self.assertTrue(
            any(
                entry["template_id"]
                in {"office--project_kickoff", "product--product_intro", "product--product_promotion"}
                for entry in product_results
            )
        )

        defense_results = template_tool.search_templates(
            self.index_data,
            {"query": "晋升答辩 个人述职", "limit": 5},
        )
        self.assertTrue(defense_results)
        self.assertTrue(any(entry["template_id"] == "personal--promotion_defense" for entry in defense_results))

    def test_extract_selection_xml_keeps_only_requested_slides_and_theme(self) -> None:
        xml = template_tool.extract_selection_xml(self.index_data, "office--light_general", {"label": "封面"})
        self.assertEqual(len(template_tool.re.findall(r"<slide\b", xml)), 2)
        self.assertIn("<theme>", xml)
        self.assertIn("<title>白底通用模板</title>", xml)

    def test_summarize_selection_aggregates_slide_titles_and_counts(self) -> None:
        summary = template_tool.summarize_selection(self.index_data, "office--light_general", {"label": "封面"})
        self.assertEqual(summary["selection"]["range"], "1-2")
        self.assertEqual(summary["summary"]["slide_count"], 2)
        self.assertTrue(summary["theme_summary"]["has_theme_node"])
        self.assertIn("通用模板", summary["summary"]["title_hints"])
        self.assertGreater(summary["summary"]["element_totals"]["shape"], 0)
        self.assertIsInstance(summary["slides"][0]["layout_tags"], list)
        self.assertIn("bbox_summary", summary["slides"][0])
        self.assertIn("editable_regions", summary["slides"][0])

    def test_template_selector_accepts_catalog_visible_filename(self) -> None:
        entry = template_tool.resolve_template_entry(self.index_data, "work_report.xml")
        self.assertEqual(entry["template_id"], "office--work_report")

    def test_template_path_uses_user_supplied_file(self) -> None:
        source_path = template_tool.TEMPLATES_DIR / "office--work_report.xml"
        with tempfile.TemporaryDirectory() as temp_dir:
            copied_path = Path(temp_dir) / "work_report.xml"
            copied_path.write_text(
                source_path.read_text(encoding="utf-8").replace(
                    "<title>工作汇报</title>",
                    "<title>Copied Path Template</title>",
                    1,
                ),
                encoding="utf-8",
            )

            xml = template_tool.extract_selection_xml(
                self.index_data,
                str(copied_path),
                {"range": "1"},
            )

        self.assertIn("<title>Copied Path Template</title>", xml)

    def test_template_path_accepts_unindexed_xml_with_range(self) -> None:
        xml = (
            '<presentation xmlns="http://www.larkoffice.com/sml/2.0">'
            "<title>Generated Template</title>"
            "<slide><data></data></slide>"
            "</presentation>"
        )
        with tempfile.TemporaryDirectory() as temp_dir:
            template_path = Path(temp_dir) / "generated.xml"
            template_path.write_text(xml, encoding="utf-8")

            extracted = template_tool.extract_selection_xml(
                self.index_data,
                str(template_path),
                {"range": "1"},
            )

        self.assertIn("<title>Generated Template</title>", extracted)

    def test_search_templates_supports_layout_tag_filtering(self) -> None:
        results = template_tool.search_templates(
            self.index_data,
            {"query": "", "layout-tag": "full-bleed-image-caption", "limit": 10},
        )
        self.assertTrue(results)
        self.assertTrue(
            any("full-bleed-image-caption" in entry["layout_tags"] for entry in results)
        )

    def test_all_template_files_are_cataloged_and_indexed(self) -> None:
        template_files = sorted(path.stem for path in template_tool.TEMPLATES_DIR.glob("*.xml"))
        indexed_templates = sorted(entry["template_id"] for entry in self.index_data["templates"])
        self.assertEqual(indexed_templates, template_files)
        self.assertEqual(self.index_data["template_count"], len(template_files))
        self.assertTrue(template_files)

    def test_catalog_range_parser_keeps_comma_separated_ranges(self) -> None:
        template = next(
            entry for entry in self.index_data["templates"] if entry["template_id"] == "operations--product_promotion"
        )
        content_range = next(item for item in template["ranges"] if item["label"] == "内容")
        self.assertEqual(content_range["range"], "3-8, 10-12")


if __name__ == "__main__":
    unittest.main()
