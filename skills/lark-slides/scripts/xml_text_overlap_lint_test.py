# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
from __future__ import annotations

import unittest
from pathlib import Path

import xml_text_overlap_lint


TEMPLATES_DIR = Path(__file__).resolve().parents[1] / "assets" / "templates"


class XmlTextOverlapLintTest(unittest.TestCase):
    def assertNoXmlTextOverlapLintIssues(self, result: dict, template_path: Path) -> None:
        issue_summaries = []
        for slide in result.get("slides", []):
            for issue in slide.get("issues", []):
                issue_summaries.append(
                    f"slide {slide['slide_number']}: {issue['level']} {issue['code']} {issue['message']}"
                )
        if result.get("issues"):
            for issue in result["issues"]:
                issue_summaries.append(f"{issue['level']} {issue['code']} {issue['message']}")
        self.assertEqual(
            result["summary"]["error_count"],
            0,
            f"{template_path.name} has XML text overlap lint errors:\n" + "\n".join(issue_summaries),
        )
        self.assertEqual(
            result["summary"]["warning_count"],
            0,
            f"{template_path.name} has XML text overlap lint warnings:\n" + "\n".join(issue_summaries),
        )

    def test_xml_text_overlap_lint_accepts_all_template_xml_files(self) -> None:
        template_paths = sorted(TEMPLATES_DIR.glob("*.xml"))
        self.assertTrue(template_paths)
        for template_path in template_paths:
            with self.subTest(template=template_path.name):
                result = xml_text_overlap_lint.lint_xml(
                    template_path.read_text(encoding="utf-8"),
                    str(template_path),
                )
                self.assertNoXmlTextOverlapLintIssues(result, template_path)

    def test_lint_xml_reports_unescaped_ampersand_in_text(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape type="text" topLeftX="80" topLeftY="80" width="300" height="60">
                  <content textType="body"><p>Q&A</p></content>
                </shape>
              </data>
            </slide>
            """
        )
        issue = result["issues"][0]
        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertEqual(issue["code"], "xml_not_well_formed")
        self.assertIsInstance(issue["line"], int)
        self.assertIsInstance(issue["column"], int)
        self.assertIn("Q&A", issue["context"])
        self.assertIn("&amp;", issue["hint"])

    def test_lint_xml_reports_unescaped_ampersand_in_attribute(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape type="text" topLeftX="80" topLeftY="80" width="300" height="60">
                  <content textType="body"><p><a href="https://example.com/?a=1&b=2">link</a></p></content>
                </shape>
              </data>
            </slide>
            """
        )
        issue = result["issues"][0]
        self.assertEqual(issue["code"], "xml_not_well_formed")
        self.assertIn("attribute", issue["hint"])
        self.assertIn("a=1&amp;b=2", issue["hint"])

    def test_lint_xml_accepts_escaped_entities_without_suspicious_entity_warning(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape type="text" topLeftX="80" topLeftY="80" width="300" height="60">
                  <content textType="body"><p>Q&amp;A</p></content>
                </shape>
              </data>
            </slide>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 0)
        self.assertNotIn("issues", result)

    def test_lint_xml_accepts_chinese_full_width_punctuation(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape type="text" topLeftX="80" topLeftY="80" width="620" height="90">
                  <content textType="body"><p>承诺：按期交付；持续复盘｜风险透明</p></content>
                </shape>
              </data>
            </slide>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 0)

    def test_lint_xml_single_slide_uses_default_canvas_without_bounds_checks(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape type="text" topLeftX="1000" topLeftY="500" width="120" height="80">
                  <content textType="body"><p>Body text outside the canvas</p></content>
                </shape>
              </data>
            </slide>
            """
        )
        self.assertEqual(result["slide_size"], {"width": 960, "height": 540})
        self.assertEqual(result["summary"]["slide_count"], 1)
        self.assertEqual(result["summary"]["error_count"], 0)

    def test_lint_xml_detects_overlapping_text_boxes(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="http://www.larkoffice.com/sml/2.0">
                <data>
                  <shape type="text" topLeftX="80" topLeftY="80" width="300" height="60">
                    <content textType="title"><p>Title</p></content>
                  </shape>
                  <shape type="text" topLeftX="80" topLeftY="80" width="300" height="80">
                    <content textType="body"><p>Body</p></content>
                  </shape>
                </data>
              </slide>
            </presentation>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertEqual(result["slides"][0]["issues"][0]["code"], "bbox_overlap")

    def test_lint_xml_does_not_check_bounds_or_text_height(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="http://www.larkoffice.com/sml/2.0">
                <data>
                  <shape type="text" topLeftX="80" topLeftY="80" width="180" height="20">
                    <content textType="body" fontSize="18"><p>This paragraph is intentionally much longer than the box can safely contain.</p></content>
                  </shape>
                  <shape type="text" topLeftX="1000" topLeftY="500" width="120" height="80">
                    <content textType="body"><p>Body text outside the canvas</p></content>
                  </shape>
                </data>
              </slide>
            </presentation>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 0)
        self.assertEqual(result["summary"]["warning_count"], 0)

    def test_lint_xml_allows_template_style_bleed_and_text_over_images(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="http://www.larkoffice.com/sml/2.0">
                <data>
                  <img src="tok" topLeftX="-120" topLeftY="20" width="360" height="360"/>
                  <shape type="text" topLeftX="40" topLeftY="80" width="180" height="80">
                    <content textType="title" fontSize="44"><p>Title</p></content>
                  </shape>
                  <shape type="text" topLeftX="40" topLeftY="120" width="180" height="40">
                    <content textType="sub-headline" fontSize="20"><p>Subtitle</p></content>
                  </shape>
                </data>
              </slide>
            </presentation>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 0)
        self.assertEqual(result["summary"]["warning_count"], 0)

    def test_lint_xml_does_not_check_small_out_of_bounds_elements(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="http://www.larkoffice.com/sml/2.0">
                <data>
                  <img src="tok" topLeftX="-20" topLeftY="20" width="120" height="120"/>
                </data>
              </slide>
            </presentation>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 0)

    def test_lint_xml_ignores_obviously_misplaced_large_visuals(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="http://www.larkoffice.com/sml/2.0">
                <data>
                  <img src="right" topLeftX="780" topLeftY="0" width="500" height="540"/>
                  <img src="bottom" topLeftX="0" topLeftY="430" width="900" height="280"/>
                </data>
              </slide>
            </presentation>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 0)

    def test_lint_xml_allows_reasonable_large_visual_bleed(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="http://www.larkoffice.com/sml/2.0">
                <data>
                  <img src="tok" topLeftX="-80" topLeftY="-20" width="1080" height="600"/>
                </data>
              </slide>
            </presentation>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 0)

    def test_lint_xml_detects_invalid_template_text_stack_overlap(self) -> None:
        cases = [
            (
                "subtitle-too-high",
                """
                <shape type="text" topLeftX="40" topLeftY="80" width="240" height="90">
                  <content textType="title" fontSize="44"><p>Title</p></content>
                </shape>
                <shape type="text" topLeftX="40" topLeftY="90" width="240" height="80">
                  <content textType="sub-headline" fontSize="20"><p>Subtitle</p></content>
                </shape>
                """,
            ),
        ]
        for name, shapes in cases:
            with self.subTest(name=name):
                result = xml_text_overlap_lint.lint_xml(
                    f"""
                    <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
                      <slide xmlns="http://www.larkoffice.com/sml/2.0">
                        <data>{shapes}</data>
                      </slide>
                    </presentation>
                    """
                )
                self.assertEqual(result["summary"]["error_count"], 1)
                self.assertEqual(result["slides"][0]["issues"][0]["code"], "bbox_overlap")


if __name__ == "__main__":
    unittest.main()
