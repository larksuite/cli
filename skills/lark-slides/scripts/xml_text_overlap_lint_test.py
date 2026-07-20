# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

import xml_text_overlap_lint


class XmlTextOverlapLintTest(unittest.TestCase):
    def assertNoXmlTextOverlapLintIssues(self, result: dict, sample_name: str) -> None:
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
            f"{sample_name} has XML text overlap lint errors:\n" + "\n".join(issue_summaries),
        )
        self.assertEqual(
            result["summary"]["warning_count"],
            0,
            f"{sample_name} has XML text overlap lint warnings:\n" + "\n".join(issue_summaries),
        )

    def test_xml_text_overlap_lint_accepts_inline_fixture_xml_samples(self) -> None:
        samples = {
            "image-led-cover": """
                <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
                  <slide xmlns="http://www.larkoffice.com/sml/2.0">
                    <style><fill><fillColor color="rgb(15,23,42)"/></fill></style>
                    <data>
                      <img src="tok" topLeftX="560" topLeftY="0" width="400" height="540"/>
                      <shape type="text" topLeftX="64" topLeftY="150" width="420" height="70">
                        <content textType="title"><p><span fontSize="42">Quarterly Review</span></p></content>
                      </shape>
                      <shape type="text" topLeftX="64" topLeftY="235" width="420" height="36">
                        <content textType="sub-headline"><p><span fontSize="20">Focus, progress, and next steps</span></p></content>
                      </shape>
                    </data>
                  </slide>
                </presentation>
            """,
            "content-grid": """
                <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
                  <slide xmlns="http://www.larkoffice.com/sml/2.0">
                    <data>
                      <shape type="text" topLeftX="60" topLeftY="44" width="620" height="46">
                        <content textType="title"><p><span fontSize="30">Execution Snapshot</span></p></content>
                      </shape>
                      <shape type="rect" topLeftX="60" topLeftY="126" width="250" height="150"/>
                      <shape type="text" topLeftX="84" topLeftY="152" width="200" height="36">
                        <content textType="headline"><p><span fontSize="22">Plan</span></p></content>
                      </shape>
                      <shape type="rect" topLeftX="355" topLeftY="126" width="250" height="150"/>
                      <shape type="text" topLeftX="379" topLeftY="152" width="200" height="36">
                        <content textType="headline"><p><span fontSize="22">Build</span></p></content>
                      </shape>
                      <shape type="rect" topLeftX="650" topLeftY="126" width="250" height="150"/>
                      <shape type="text" topLeftX="674" topLeftY="152" width="200" height="36">
                        <content textType="headline"><p><span fontSize="22">Launch</span></p></content>
                      </shape>
                    </data>
                  </slide>
                </presentation>
            """,
        }
        self.assertTrue(samples)
        for sample_name, sample_xml in samples.items():
            with self.subTest(sample=sample_name):
                result = xml_text_overlap_lint.lint_xml(
                    sample_xml,
                    sample_name,
                )
                self.assertNoXmlTextOverlapLintIssues(result, sample_name)

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

    def test_lint_xml_reports_mismatched_xml_tag(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape type="text" topLeftX="80" topLeftY="80" width="300" height="60">
                  <content textType="body"><p>Broken XML</content>
                </shape>
              </data>
            </slide>
            """
        )
        issue = result["issues"][0]
        self.assertEqual(result["summary"]["slide_count"], 0)
        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertEqual(issue["code"], "xml_not_well_formed")
        self.assertIsInstance(issue["line"], int)
        self.assertIsInstance(issue["column"], int)
        self.assertIn("Broken XML", issue["context"])

    def test_lint_xml_rejects_prefixed_sml_tags(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <ns0:slide xmlns:ns0="http://www.larkoffice.com/sml/2.0">
              <ns0:data>
                <sml:shape xmlns:sml="http://www.larkoffice.com/sml/2.0" type="text" topLeftX="80" topLeftY="80" width="300" height="60">
                  <sml:content textType="body"><sml:p>Prefixed SML</sml:p></sml:content>
                </sml:shape>
              </ns0:data>
            </ns0:slide>
            """
        )
        issues = result["issues"]
        self.assertEqual(result["summary"]["error_count"], 5)
        self.assertEqual([issue["code"] for issue in issues], ["sml_prefixed_tag"] * 5)
        self.assertEqual(issues[0]["tag"], "ns0:slide")
        self.assertIn("default namespace", issues[0]["hint"])

    def test_lint_xml_allows_unprefixed_tags_without_namespace(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide>
              <data>
                <shape type="text" topLeftX="80" topLeftY="80" width="300" height="60">
                  <content textType="body"><p>Unprefixed SML</p></content>
                </shape>
              </data>
            </slide>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 0)

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
        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertEqual(result["slides"][0]["issues"][0]["code"], "shape_out_of_canvas")

    def test_lint_xml_preserves_presentation_canvas_and_slide_order(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="1280" height="720">
              <slide xmlns="http://www.larkoffice.com/sml/2.0">
                <data>
                  <shape id="first" type="text" topLeftX="80" topLeftY="80" width="300" height="60">
                    <content textType="body"><p>First slide</p></content>
                  </shape>
                </data>
              </slide>
              <slide xmlns="http://www.larkoffice.com/sml/2.0">
                <data>
                  <img id="second" src="tok" topLeftX="100" topLeftY="120" width="240" height="160"/>
                  <shape id="skipped" type="text" topLeftX="80" topLeftY="80" width="300">
                    <content textType="body"><p>Missing height</p></content>
                  </shape>
                </data>
              </slide>
            </presentation>
            """
        )
        self.assertEqual(result["slide_size"], {"width": 1280, "height": 720})
        self.assertEqual(result["summary"]["slide_count"], 2)
        self.assertEqual([slide["slide_number"] for slide in result["slides"]], [1, 2])
        self.assertEqual([slide["element_count"] for slide in result["slides"]], [1, 1])
        self.assertEqual(result["summary"]["error_count"], 0)
        self.assertEqual(result["summary"]["warning_count"], 0)

    def test_lint_xml_reports_sxsd_unsupported_tag_with_alias_hint(self) -> None:
        cases = [
            ("textbox", '<textbox topLeftX="80" topLeftY="80" width="300" height="60">Text</textbox>', '<shape type="text">'),
            ("image", '<image src="tok" topLeftX="80" topLeftY="80" width="300" height="180"/>', "<img>"),
        ]
        for tag_name, element_xml, expected_hint in cases:
            with self.subTest(tag=tag_name):
                result = xml_text_overlap_lint.lint_xml(
                    f"""
                    <slide xmlns="http://www.larkoffice.com/sml/2.0">
                      <data>{element_xml}</data>
                    </slide>
                    """
                )
                issue = result["issues"][0]
                self.assertEqual(result["summary"]["error_count"], 1)
                self.assertEqual(issue["code"], "sxsd_unsupported_tag")
                self.assertEqual(issue["tag"], tag_name)
                self.assertIn(expected_hint, issue["hint"])

    def test_lint_xml_reports_sxsd_unsupported_attr_with_alias_hint(self) -> None:
        cases = [
            ("shape", "x", "topLeftX", '<shape type="text" x="80" topLeftY="80" width="300" height="60"><content><p>Text</p></content></shape>'),
            ("content", "fontColor", "color", '<shape type="text" topLeftX="80" topLeftY="80" width="300" height="60"><content fontColor="rgba(0, 0, 0, 1)"><p>Text</p></content></shape>'),
        ]
        for tag_name, attr_name, expected_attr, element_xml in cases:
            with self.subTest(attr=attr_name):
                result = xml_text_overlap_lint.lint_xml(
                    f"""
                    <slide xmlns="http://www.larkoffice.com/sml/2.0">
                      <data>{element_xml}</data>
                    </slide>
                    """
                )
                issue = result["issues"][0]
                self.assertEqual(result["summary"]["error_count"], 1)
                self.assertEqual(issue["code"], "sxsd_unsupported_attr")
                self.assertEqual(issue["tag"], tag_name)
                self.assertEqual(issue["attr"], attr_name)
                self.assertIn(expected_attr, issue["hint"])

    def test_lint_xml_ignores_server_filled_id_attrs(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide id="server-slide-id" xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="server-shape-id" type="rect" topLeftX="80" topLeftY="80" width="300" height="160">
                  <fill id="server-fill-id" unexpected="value">
                    <fillColor color="rgba(255, 255, 255, 1)"/>
                  </fill>
                </shape>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["summary"]["error_count"], 1)
        issue = result["issues"][0]
        self.assertEqual(issue["code"], "sxsd_unsupported_attr")
        self.assertEqual(issue["tag"], "fill")
        self.assertEqual(issue["attr"], "unexpected")

    def test_lint_xml_ignores_chart_roundtrip_attrs(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <chart updated="true" topLeftX="80" topLeftY="80" width="300" height="160">
                  <chartData isStaticData="true"/>
                </chart>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["summary"]["error_count"], 0)
        self.assertNotIn("issues", result)

    def test_lint_xml_limits_chart_roundtrip_attrs_to_matching_tags(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <chart isStaticData="true" topLeftX="80" topLeftY="80" width="300" height="160">
                  <chartData updated="true"/>
                </chart>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["summary"]["error_count"], 2)
        self.assertEqual(
            {(issue["tag"], issue["attr"]) for issue in result["issues"]},
            {("chart", "isStaticData"), ("chartData", "updated")},
        )
        self.assertTrue(all(issue["code"] == "sxsd_unsupported_attr" for issue in result["issues"]))

    def test_lint_xml_reports_gradient_shorthand_attrs_on_fill_color(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape type="rect" topLeftX="80" topLeftY="80" width="300" height="160">
                  <fill>
                    <fillColor
                      type="gradient"
                      color1="rgba(255, 0, 0, 1)"
                      color2="rgba(0, 0, 255, 1)"
                      angle="45"
                      stop1="0%"
                      stop2="100%"/>
                  </fill>
                </shape>
              </data>
            </slide>
            """
        )
        unsupported_attrs = {issue["attr"] for issue in result["issues"]}
        self.assertEqual(result["summary"]["error_count"], 6)
        self.assertEqual(
            unsupported_attrs,
            {"type", "color1", "color2", "angle", "stop1", "stop2"},
        )
        self.assertTrue(all(issue["code"] == "sxsd_unsupported_attr" for issue in result["issues"]))
        self.assertTrue(all(issue["tag"] == "fillColor" for issue in result["issues"]))

    def test_lint_xml_accepts_chart_field_simple_content_attrs(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <chart topLeftX="80" topLeftY="80" width="520" height="320">
                  <chartPlotArea>
                    <chartPlot type="line"/>
                  </chartPlotArea>
                  <chartData>
                    <dim1>
                      <chartField name="month" valueType="string">Jan, Feb</chartField>
                    </dim1>
                    <dim2>
                      <chartField name="value" valueType="number">1, 2</chartField>
                    </dim2>
                  </chartData>
                </chart>
              </data>
            </slide>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 0)
        self.assertNotIn("issues", result)

    def test_lint_xml_does_not_load_iconpark_index_without_icons(self) -> None:
        original_loader = xml_text_overlap_lint.load_iconpark_icon_types

        def fail_if_loaded() -> set[str]:
            raise AssertionError("iconpark index should not be loaded without <icon iconType>")

        xml_text_overlap_lint.load_iconpark_icon_types = fail_if_loaded
        try:
            result = xml_text_overlap_lint.lint_xml(
                """
                <slide xmlns="http://www.larkoffice.com/sml/2.0">
                  <data>
                    <shape type="text" topLeftX="80" topLeftY="80" width="300" height="60">
                      <content><p>No icons here</p></content>
                    </shape>
                  </data>
                </slide>
                """
            )
        finally:
            xml_text_overlap_lint.load_iconpark_icon_types = original_loader

        self.assertEqual(result["summary"]["error_count"], 0)
        self.assertNotIn("issues", result)

    def test_lint_xml_accepts_iconpark_icon_type_from_index(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <icon iconType="iconpark/Base/setting.svg" topLeftX="80" topLeftY="80" width="48" height="48">
                  <fill><fillColor color="rgba(37, 99, 235, 1)"/></fill>
                </icon>
              </data>
            </slide>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 0)
        self.assertNotIn("issues", result)

    def test_lint_xml_reports_icon_missing_fill_color(self) -> None:
        cases = [
            '<icon iconType="iconpark/Base/setting.svg" topLeftX="80" topLeftY="80" width="48" height="48"/>',
            '<icon iconType="iconpark/Base/setting.svg" topLeftX="80" topLeftY="80" width="48" height="48"><fill/></icon>',
            (
                '<icon iconType="iconpark/Base/setting.svg" topLeftX="80" topLeftY="80" width="48" height="48">'
                "<fill><fillColor/></fill></icon>"
            ),
        ]
        for icon_xml in cases:
            with self.subTest(icon=icon_xml):
                result = xml_text_overlap_lint.lint_xml(
                    f"""
                    <slide xmlns="http://www.larkoffice.com/sml/2.0">
                      <data>{icon_xml}</data>
                    </slide>
                    """
                )

                issue = result["issues"][0]
                self.assertEqual(result["summary"]["error_count"], 1)
                self.assertEqual(issue["code"], "icon_missing_fill_color")
                self.assertEqual(issue["tag"], "icon")

    def test_lint_xml_reports_icon_transparent_fill_color(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <icon iconType="iconpark/Base/setting.svg" topLeftX="80" topLeftY="80" width="48" height="48">
                  <fill><fillColor color="rgba(37, 99, 235, 0)"/></fill>
                </icon>
              </data>
            </slide>
            """
        )
        issue = result["issues"][0]
        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertEqual(issue["code"], "icon_transparent_fill_color")
        self.assertEqual(issue["tag"], "icon")
        self.assertEqual(issue["attr"], "fillColor")

    def test_lint_xml_reports_iconpark_icon_type_outside_index(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <icon iconType="iconpark/Base/settng.svg" topLeftX="80" topLeftY="80" width="48" height="48">
                  <fill><fillColor color="rgba(37, 99, 235, 1)"/></fill>
                </icon>
              </data>
            </slide>
            """
        )
        issue = result["issues"][0]
        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertEqual(issue["code"], "iconpark_unsupported_icon_type")
        self.assertEqual(issue["tag"], "icon")
        self.assertEqual(issue["attr"], "iconType")
        self.assertEqual(issue["iconType"], "iconpark/Base/settng.svg")
        self.assertIn("iconpark-index.json", issue["hint"])
        self.assertIn("iconpark/Base/setting.svg", issue["hint"])

    def test_lint_xml_allows_namespaced_svg_inside_whiteboard(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <whiteboard id="wb" topLeftX="80" topLeftY="80" width="300" height="180">
                  <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 300 180">
                    <rect x="10" y="10" width="120" height="60"/>
                  </svg>
                </whiteboard>
              </data>
            </slide>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 0)
        self.assertNotIn("issues", result)

    def test_lint_xml_reports_unqualified_svg_inside_whiteboard_with_namespace_hint(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <whiteboard id="wb" topLeftX="80" topLeftY="80" width="300" height="180">
                  <svg viewBox="0 0 300 180">
                    <rect x="10" y="10" width="120" height="60"/>
                  </svg>
                </whiteboard>
              </data>
            </slide>
            """
        )
        issue = result["issues"][0]
        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertEqual(issue["code"], "sxsd_unsupported_tag")
        self.assertEqual(issue["tag"], "svg")
        self.assertIn('xmlns="http://www.w3.org/2000/svg"', issue["hint"])

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
        self.assertEqual(result["summary"]["warning_count"], 0)
        self.assertEqual(result["slides"][0]["issues"][0]["code"], "bbox_overlap")

    def test_lint_xml_detects_current_itinerary_cjk_caption_occlusion(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide id="pQO" xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape width="190" height="80" topLeftX="580" topLeftY="170" presetHandlers="0" type="rect" id="blI">
                  <fill><fillColor color="rgba(255, 255, 255, 0.9)"/></fill>
                  <border color="rgba(220, 205, 185, 1)" width="1"/>
                  <content fontSize="16" fontFamily="思源黑体" color="rgba(31, 35, 41, 1)"/>
                </shape>
                <shape width="160" height="25" topLeftX="595" topLeftY="180" type="text" id="blX">
                  <content fontSize="14" fontFamily="思源黑体" color="rgba(120, 80, 40, 1)" bold="true"><p>日照金山</p></content>
                </shape>
                <shape width="160" height="40" topLeftX="595" topLeftY="205" type="text" id="blY">
                  <content textType="caption" fontSize="11" fontFamily="思源黑体" color="rgba(130, 100, 70, 1)"><p>清晨躺在床上看玉龙雪山日照金山奇观</p></content>
                </shape>
                <shape width="180" height="80" topLeftX="730" topLeftY="170" presetHandlers="0" type="rect" id="blH">
                  <fill><fillColor color="rgba(255, 255, 255, 0.9)"/></fill>
                  <border color="rgba(220, 205, 185, 1)" width="1"/>
                  <content fontSize="16" fontFamily="思源黑体" color="rgba(31, 35, 41, 1)"/>
                </shape>
                <shape width="150" height="25" topLeftX="745" topLeftY="180" type="text" id="blp">
                  <content fontSize="14" fontFamily="思源黑体" color="rgba(120, 80, 40, 1)" bold="true"><p>午餐返程</p></content>
                </shape>
                <shape width="150" height="40" topLeftX="745" topLeftY="205" type="text" id="blV">
                  <content textType="caption" fontSize="11" fontFamily="思源黑体" color="rgba(130, 100, 70, 1)"><p>享用特色午餐，带着美好回忆返程</p></content>
                </shape>
                <shape width="190" height="80" topLeftX="580" topLeftY="310" presetHandlers="0" type="rect" id="blP">
                  <fill><fillColor color="rgba(255, 255, 255, 0.9)"/></fill>
                  <border color="rgba(220, 205, 185, 1)" width="1"/>
                  <content fontSize="16" fontFamily="思源黑体" color="rgba(31, 35, 41, 1)"/>
                </shape>
                <shape width="160" height="25" topLeftX="595" topLeftY="320" type="text" id="blG">
                  <content fontSize="14" fontFamily="思源黑体" color="rgba(120, 80, 40, 1)" bold="true"><p>高路徒步</p></content>
                </shape>
                <shape width="160" height="40" topLeftX="595" topLeftY="345" type="text" id="blQ">
                  <content textType="caption" fontSize="11" fontFamily="思源黑体" color="rgba(130, 100, 70, 1)"><p>经典高路徒步，28道拐，龙洞瀑布，中虎跳峡</p></content>
                </shape>
                <shape width="180" height="80" topLeftX="730" topLeftY="310" presetHandlers="0" type="rect" id="blw">
                  <fill><fillColor color="rgba(255, 255, 255, 0.9)"/></fill>
                  <border color="rgba(220, 205, 185, 1)" width="1"/>
                  <content fontSize="16" fontFamily="思源黑体" color="rgba(31, 35, 41, 1)"/>
                </shape>
                <shape width="150" height="25" topLeftX="745" topLeftY="320" type="text" id="blZ">
                  <content fontSize="14" fontFamily="思源黑体" color="rgba(120, 80, 40, 1)" bold="true"><p>伴手礼</p></content>
                </shape>
                <shape width="150" height="40" topLeftX="745" topLeftY="345" type="text" id="blS">
                  <content textType="caption" fontSize="11" fontFamily="思源黑体" color="rgba(130, 100, 70, 1)"><p>酒店精心准备的归途伴手礼，留下难忘纪念</p></content>
                </shape>
              </data>
            </slide>
            """
        )
        overlap_pairs = {tuple(issue["elements"]) for issue in result["slides"][0]["issues"]}
        self.assertEqual(result["summary"]["error_count"], 2)
        self.assertIn(("blY", "blV"), overlap_pairs)
        self.assertIn(("blQ", "blS"), overlap_pairs)

    def test_lint_xml_detects_horizontal_text_overflow_across_declared_box_gap(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="source" type="text" topLeftX="80" topLeftY="100" width="160" height="40">
                  <content fontSize="18"><p>这是一个足够长的中文文本用于检测跨越间隙的横向溢出</p></content>
                </shape>
                <shape id="target" type="text" topLeftX="260" topLeftY="100" width="160" height="40">
                  <content fontSize="18"><p>目标</p></content>
                </shape>
              </data>
            </slide>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertEqual(result["summary"]["warning_count"], 0)
        self.assertEqual(result["slides"][0]["issues"][0]["code"], "bbox_overlap")
        self.assertEqual(result["slides"][0]["issues"][0]["elements"], ["source", "target"])

    def test_lint_xml_allows_horizontal_text_with_default_wrap(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="source" type="text" topLeftX="80" topLeftY="100" width="160" height="40">
                  <content fontSize="18"><p>这是一个足够长的中文文本用于检测默认自动换行</p></content>
                </shape>
                <shape id="target" type="text" topLeftX="260" topLeftY="100" width="160" height="40">
                  <content fontSize="18"><p>目标</p></content>
                </shape>
              </data>
            </slide>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 0)

    def test_lint_xml_reports_text_out_of_canvas_and_warns_for_text_height(self) -> None:
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
        issue = result["slides"][0]["issues"][0]
        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertEqual(result["summary"]["warning_count"], 1)
        self.assertEqual(issue["code"], "shape_out_of_canvas")
        self.assertEqual(issue["overflow"], {"left": 0, "top": 0, "right": 160, "bottom": 40})

    def test_lint_xml_warns_when_text_may_overflow_its_own_shape(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="overflowing" type="text" topLeftX="80" topLeftY="80" width="360" height="80">
                  <content fontSize="20" lineSpacing="multiple:1.5">
                    <p>第一段</p><p>第二段</p><p>第三段</p><p>第四段</p>
                  </content>
                </shape>
                <shape id="fitting" type="text" topLeftX="480" topLeftY="80" width="360" height="120">
                  <content fontSize="20" lineSpacing="multiple:1.5">
                    <p>第一段</p><p>第二段</p><p>第三段</p><p>第四段</p>
                  </content>
                </shape>
                <shape id="auto-fit" type="text" topLeftX="80" topLeftY="240" width="360" height="80">
                  <content fontSize="20" lineSpacing="multiple:1.5" autoFit="normal-auto-fit">
                    <p>第一段</p><p>第二段</p><p>第三段</p><p>第四段</p>
                  </content>
                </shape>
              </data>
            </slide>
            """
        )
        issues = result["slides"][0]["issues"]
        self.assertEqual(result["summary"]["error_count"], 0)
        self.assertEqual(result["summary"]["warning_count"], 1)
        self.assertEqual(issues[0]["code"], "text_may_overflow_shape")
        self.assertEqual(issues[0]["elements"], ["overflowing"])
        self.assertEqual(issues[0]["line_count"], 4)
        self.assertEqual(issues[0]["estimated_height"], 110)
        self.assertEqual(issues[0]["available_height"], 80)
        self.assertEqual(issues[0]["overflow"], 30)
        self.assertIn('wrap="true" autoFit="normal-auto-fit"', issues[0]["message"])

    def test_lint_xml_uses_fixed_line_spacing_for_text_height_warning(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="fixed-overflow" type="text" topLeftX="80" topLeftY="80" width="360" height="50">
                  <content fontSize="20" lineSpacing="fixed:20">
                    <p>第一段</p><p>第二段</p><p>第三段</p>
                  </content>
                </shape>
              </data>
            </slide>
            """
        )
        issue = result["slides"][0]["issues"][0]
        self.assertEqual(result["summary"]["warning_count"], 1)
        self.assertEqual(issue["line_height"], 20)
        self.assertEqual(issue["estimated_height"], 60)
        self.assertEqual(issue["overflow"], 10)

    def test_lint_xml_uses_paragraph_spacing_overrides_for_text_height_warning(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="paragraph-overflow" type="text" topLeftX="80" topLeftY="80" width="360" height="35">
                  <content fontSize="20" lineSpacing="multiple:1.5">
                    <p lineSpacing="fixed:10" beforeLineSpacing="fixed:5" afterLineSpacing="fixed:5">第一行<br/>第二行</p>
                  </content>
                </shape>
                <shape id="paragraph-fitting" type="text" topLeftX="480" topLeftY="80" width="360" height="40">
                  <content fontSize="20" lineSpacing="multiple:1.5">
                    <p lineSpacing="fixed:10">第一行<br/>第二行<br/>第三行</p>
                  </content>
                </shape>
              </data>
            </slide>
            """
        )
        issues = result["slides"][0]["issues"]
        self.assertEqual(result["summary"]["warning_count"], 1)
        self.assertEqual(issues[0]["elements"], ["paragraph-overflow"])
        self.assertEqual(issues[0]["line_count"], 2)
        self.assertEqual(issues[0]["line_height"], 10)
        self.assertEqual(issues[0]["estimated_height"], 40)
        self.assertEqual(issues[0]["overflow"], 5)

    def test_strip_xml_paragraphs_preserves_br_as_hard_line_break(self) -> None:
        self.assertEqual(
            xml_text_overlap_lint.strip_xml_paragraphs("<p>第一行<br/>第二行<br />第三行</p>"),
            "第一行\n第二行\n第三行",
        )

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

    def test_extract_elements_preserves_supported_element_geometry_order_and_text_metadata(self) -> None:
        elements = xml_text_overlap_lint.extract_elements(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <img id="photo" src="tok" topLeftX="10" topLeftY="20" width="100" height="80"/>
                <shape id="headline" type="text" topLeftX="40" topLeftY="60" width="320" height="90">
                  <content textType="headline" textAlign="center" autoFit="normal-auto-fit" fontSize="28">
                    <p><![CDATA[Growth & scale]]></p>
                    <p>Focused execution</p>
                  </content>
                </shape>
                <table id="table" topLeftX="400" topLeftY="60" width="220" height="120"></table>
                <chart id="chart" topLeftX="640" topLeftY="60" width="220" height="120"/>
                <whiteboard id="wb" topLeftX="80" topLeftY="220" width="760" height="240"/>
                <shape id="missing-height" type="text" topLeftX="80" topLeftY="480" width="320">
                  <content><p>Skipped</p></content>
                </shape>
              </data>
            </slide>
            """
        )
        self.assertEqual([element["id"] for element in elements], ["photo", "headline", "table", "chart", "wb"])
        self.assertEqual([element["kind"] for element in elements], ["img", "shape", "table", "chart", "whiteboard"])
        self.assertEqual([element["order"] for element in elements], [0, 1, 2, 3, 4])
        self.assertEqual(elements[1]["type"], "text")
        self.assertEqual(elements[1]["textType"], "headline")
        self.assertEqual(elements[1]["textAlign"], "center")
        self.assertEqual(elements[1]["autoFit"], "normal-auto-fit")
        self.assertEqual(elements[1]["fontSize"], 28)
        self.assertEqual(elements[1]["text"], "Growth & scale\nFocused execution")

    def test_lint_xml_allows_small_out_of_bounds_images(self) -> None:
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

    def test_lint_xml_allows_out_of_canvas_images(self) -> None:
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

    def test_lint_xml_allows_full_bleed_images(self) -> None:
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

    def test_lint_xml_reports_text_and_chart_out_of_canvas(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="http://www.larkoffice.com/sml/2.0">
                <data>
                  <shape id="outside-shape" type="text" topLeftX="-10" topLeftY="40" width="50" height="50"/>
                  <img id="outside-img" src="token" topLeftX="120" topLeftY="-20" width="50" height="50"/>
                  <chart id="outside-chart" topLeftX="900" topLeftY="100" width="100" height="100"/>
                  <whiteboard id="outside-whiteboard" topLeftX="100" topLeftY="500" width="100" height="100"/>
                </data>
              </slide>
            </presentation>
            """
        )
        issues = result["slides"][0]["issues"]
        self.assertEqual(result["summary"]["error_count"], 2)
        self.assertEqual(
            [(issue["code"], issue["elements"], issue["overflow"]) for issue in issues],
            [
                ("shape_out_of_canvas", ["outside-shape"], {"left": 10, "top": 0, "right": 0, "bottom": 0}),
                ("chart_out_of_canvas", ["outside-chart"], {"left": 0, "top": 0, "right": 40, "bottom": 0}),
            ],
        )

    def test_lint_xml_uses_rotated_text_and_chart_bounds_for_canvas_validation(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="http://www.larkoffice.com/sml/2.0">
                <data>
                  <shape id="rotated-text" type="text" topLeftX="0" topLeftY="0" width="100" height="100" rotation="45"/>
                  <chart id="rotated-chart" topLeftX="860" topLeftY="200" width="100" height="100" rotation="45"/>
                </data>
              </slide>
            </presentation>
            """
        )
        issues_by_element = {issue["elements"][0]: issue for issue in result["slides"][0]["issues"]}
        self.assertEqual(result["summary"]["error_count"], 2)
        self.assertEqual(issues_by_element["rotated-text"]["code"], "shape_out_of_canvas")
        self.assertAlmostEqual(issues_by_element["rotated-text"]["overflow"]["left"], 20.710678, places=5)
        self.assertAlmostEqual(issues_by_element["rotated-text"]["overflow"]["top"], 20.710678, places=5)
        self.assertEqual(issues_by_element["rotated-chart"]["code"], "chart_out_of_canvas")
        self.assertAlmostEqual(issues_by_element["rotated-chart"]["overflow"]["right"], 20.710678, places=5)

    def test_lint_xml_treats_non_finite_rotations_as_zero(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="http://www.larkoffice.com/sml/2.0">
                <data>
                  <shape id="infinite" type="text" topLeftX="-10" topLeftY="0" width="20" height="20" rotation="inf"/>
                  <shape id="negative-infinite" type="text" topLeftX="0" topLeftY="-10" width="20" height="20" rotation="-inf"/>
                  <chart id="not-a-number" topLeftX="950" topLeftY="0" width="20" height="20" rotation="nan"/>
                </data>
              </slide>
            </presentation>
            """
        )
        issues_by_element = {issue["elements"][0]: issue for issue in result["slides"][0]["issues"]}
        self.assertEqual(result["summary"]["error_count"], 3)
        self.assertEqual(issues_by_element["infinite"]["overflow"], {"left": 10, "top": 0, "right": 0, "bottom": 0})
        self.assertEqual(issues_by_element["negative-infinite"]["overflow"], {"left": 0, "top": 10, "right": 0, "bottom": 0})
        self.assertEqual(issues_by_element["not-a-number"]["overflow"], {"left": 0, "top": 0, "right": 10, "bottom": 0})

    def test_lint_xml_reports_table_bottom_overflow_from_declared_bounds(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="http://www.larkoffice.com/sml/2.0">
                <data>
                  <table id="score-table" topLeftX="54" topLeftY="238" width="414" height="385">
                    <tr><td><content><p>Score</p></content></td></tr>
                  </table>
                </data>
              </slide>
            </presentation>
            """
        )
        issue = result["slides"][0]["issues"][0]
        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertEqual(issue["code"], "table_out_of_canvas")
        self.assertEqual(issue["elements"], ["score-table"])
        self.assertEqual(issue["overflow"], {"left": 0, "top": 0, "right": 0, "bottom": 83})
        self.assertEqual(issue["bbox"], {"x": 54, "y": 238, "width": 414, "height": 385})

    def test_lint_xml_reports_table_right_overflow_from_declared_bounds(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="http://www.larkoffice.com/sml/2.0">
                <data>
                  <table id="wide-table" topLeftX="850" topLeftY="80" width="180" height="120">
                    <tr><td><content><p>Score</p></content></td></tr>
                  </table>
                </data>
              </slide>
            </presentation>
            """
        )
        issue = result["slides"][0]["issues"][0]
        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertEqual(issue["code"], "table_out_of_canvas")
        self.assertEqual(issue["overflow"], {"left": 0, "top": 0, "right": 70, "bottom": 0})

    def test_lint_xml_allows_table_with_declared_bounds_inside_canvas(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="http://www.larkoffice.com/sml/2.0">
                <data>
                  <table id="inside-table" topLeftX="40" topLeftY="120" width="880" height="360">
                    <tr><td><content><p>Score</p></content></td></tr>
                  </table>
                </data>
              </slide>
            </presentation>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 0)
        self.assertEqual(result["summary"]["warning_count"], 0)

    def test_lint_xml_reports_resolved_table_bounds_when_declared_sizes_are_missing(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="http://www.larkoffice.com/sml/2.0">
                <data>
                  <table id="implicit-size-table" topLeftX="850" topLeftY="480">
                    <colgroup><col/><col/></colgroup>
                    <tr><td/><td/></tr>
                    <tr><td/><td/></tr>
                  </table>
                </data>
              </slide>
            </presentation>
            """
        )
        issue = result["slides"][0]["issues"][0]
        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertEqual(issue["code"], "table_out_of_canvas")
        self.assertEqual(issue["bbox"], {"x": 850, "y": 480, "width": 220, "height": 74})
        self.assertEqual(issue["overflow"], {"left": 0, "top": 0, "right": 110, "bottom": 14})

    def test_lint_xml_uses_resolved_table_bounds_for_canvas_validation(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="http://www.larkoffice.com/sml/2.0">
                <data>
                  <table id="resolved-overflow-table" topLeftX="800" topLeftY="80" width="100" height="40">
                    <colgroup><col width="100"/><col width="100"/></colgroup>
                    <tr height="40"><td/><td/></tr>
                  </table>
                </data>
              </slide>
            </presentation>
            """
        )
        issues = result["slides"][0]["issues"]
        canvas_issue = next(issue for issue in issues if issue["code"] == "table_out_of_canvas")
        mismatch_issue = next(issue for issue in issues if issue["code"] == "table_resolved_size_mismatch")
        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertEqual(canvas_issue["bbox"], {"x": 800, "y": 80, "width": 200, "height": 40})
        self.assertEqual(canvas_issue["overflow"]["right"], 40)
        self.assertEqual(mismatch_issue["dimension"], "width")
        self.assertEqual(mismatch_issue["resolved_size"], canvas_issue["bbox"]["width"])

    def test_lint_xml_uses_the_same_anonymous_table_id_for_all_table_diagnostics(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="http://www.larkoffice.com/sml/2.0">
                <data>
                  <shape id="title" type="text" topLeftX="40" topLeftY="40" width="200" height="40"/>
                  <img id="logo" src="token" topLeftX="40" topLeftY="100" width="40" height="40"/>
                  <table topLeftX="900" topLeftY="80" width="100" height="40">
                    <colgroup><col width="100"/><col width="100"/></colgroup>
                    <tr height="40"><td/><td/></tr>
                  </table>
                </data>
              </slide>
            </presentation>
            """
        )
        issues = result["slides"][0]["issues"]
        canvas_issue = next(issue for issue in issues if issue["code"] == "table_out_of_canvas")
        mismatch_issue = next(issue for issue in issues if issue["code"] == "table_resolved_size_mismatch")
        self.assertEqual(canvas_issue["elements"], ["table-3"])
        self.assertEqual(mismatch_issue["elements"], ["table-3"])

    def test_lint_xml_reports_info_when_table_target_size_resolves_larger_than_declared(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="http://www.larkoffice.com/sml/2.0">
                <data>
                  <table id="size-mismatch" topLeftX="40" topLeftY="120" width="200" height="80">
                    <colgroup><col span="2" width="100"/><col width="50"/></colgroup>
                    <tr height="40"><td/><td/><td/></tr>
                    <tr height="60"><td/><td/><td/></tr>
                  </table>
                </data>
              </slide>
            </presentation>
            """
        )
        issues_by_dimension = {issue["dimension"]: issue for issue in result["slides"][0]["issues"]}
        self.assertEqual(result["summary"]["error_count"], 0)
        self.assertEqual(result["summary"]["warning_count"], 0)
        self.assertEqual(result["summary"]["info_count"], 2)
        self.assertEqual(issues_by_dimension["width"]["level"], "info")
        self.assertEqual(issues_by_dimension["width"]["code"], "table_resolved_size_mismatch")
        self.assertEqual(issues_by_dimension["width"]["resolved_sizes"], [100, 100, 50])
        self.assertEqual(issues_by_dimension["width"]["resolved_size"], 250)
        self.assertEqual(issues_by_dimension["height"]["resolved_sizes"], [40, 60])
        self.assertEqual(issues_by_dimension["height"]["resolved_size"], 100)

    def test_lint_xml_does_not_report_info_when_table_target_size_is_resolved_exactly(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="http://www.larkoffice.com/sml/2.0">
                <data>
                  <table id="size-match" topLeftX="40" topLeftY="120" width="300" height="100">
                    <colgroup><col width="100"/><col/></colgroup>
                    <tr height="40"><td/><td/></tr>
                    <tr><td/><td/></tr>
                  </table>
                </data>
              </slide>
            </presentation>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 0)
        self.assertEqual(result["summary"]["warning_count"], 0)
        self.assertEqual(result["summary"]["info_count"], 0)
        self.assertEqual(result["slides"][0]["issues"], [])

    def test_lint_xml_keeps_resolved_table_sizes_positive_when_target_is_too_small(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="http://www.larkoffice.com/sml/2.0">
                <data>
                  <table id="narrow-table" topLeftX="40" topLeftY="120" width="1">
                    <colgroup><col/><col/></colgroup>
                    <tr><td/><td/></tr>
                  </table>
                </data>
              </slide>
            </presentation>
            """
        )
        issue = result["slides"][0]["issues"][0]
        self.assertEqual(issue["dimension"], "width")
        self.assertEqual(issue["resolved_sizes"], [1, 1])
        self.assertEqual(issue["resolved_size"], 2)

    def test_fill_last_size_gap_preserves_target_when_positive_sizes_are_possible(self) -> None:
        final_sizes = xml_text_overlap_lint.fill_last_size_gap([10, 10], 3)
        self.assertEqual(final_sizes, [2, 1])
        self.assertEqual(sum(final_sizes), 3)

    def test_cli_reports_table_layout_size_info_for_weighted_min_layout_cases(self) -> None:
        cases = {
            "target-exact": (
                """
                <table topLeftX="40" topLeftY="120" width="360" height="150">
                  <colgroup><col width="100"/><col width="200"/></colgroup>
                  <tr height="40"><td/><td/></tr><tr height="60"><td/><td/></tr>
                </table>
                """,
                0,
            ),
            "declared-size-exceeds-target": (
                """
                <table topLeftX="40" topLeftY="120" width="200" height="80">
                  <colgroup><col span="2" width="100"/><col width="50"/></colgroup>
                  <tr height="40"><td/><td/><td/></tr><tr height="60"><td/><td/><td/></tr>
                </table>
                """,
                2,
            ),
            "remaining-space-insufficient": (
                """
                <table topLeftX="40" topLeftY="120" width="80" height="30">
                  <colgroup><col width="80"/><col/></colgroup>
                  <tr height="40"><td/><td/></tr><tr><td/><td/></tr>
                </table>
                """,
                2,
            ),
            "no-target-size": (
                """
                <table topLeftX="40" topLeftY="120">
                  <colgroup><col width="80"/><col/></colgroup>
                  <tr height="40"><td/><td/></tr><tr><td/><td/></tr>
                </table>
                """,
                0,
            ),
        }
        script_path = Path(xml_text_overlap_lint.__file__).resolve()
        with tempfile.TemporaryDirectory() as temp_dir:
            for name, (table_xml, expected_info_count) in cases.items():
                with self.subTest(case=name):
                    input_path = Path(temp_dir) / f"{name}.xml"
                    input_path.write_text(
                        f"""
                        <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
                          <slide xmlns="http://www.larkoffice.com/sml/2.0"><data>{table_xml}</data></slide>
                        </presentation>
                        """,
                        encoding="utf-8",
                    )
                    completed = subprocess.run(
                        [sys.executable, str(script_path), "--input", str(input_path)],
                        capture_output=True,
                        check=False,
                        text=True,
                    )
                    result = json.loads(completed.stdout)
                    self.assertEqual(completed.returncode, 0, completed.stderr)
                    self.assertEqual(result["summary"]["error_count"], 0)
                    self.assertEqual(result["summary"]["warning_count"], 0)
                    self.assertEqual(result["summary"]["info_count"], expected_info_count)
                    self.assertTrue(
                        all(issue["level"] == "info" for issue in result["slides"][0]["issues"]),
                        result["slides"][0]["issues"],
                    )

    def test_lint_xml_warns_for_whiteboard_external_boundary_overlap(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="http://www.larkoffice.com/sml/2.0">
                <data>
                  <whiteboard id="wb" topLeftX="50" topLeftY="140" width="860" height="360"/>
                  <shape id="card" type="rect" topLeftX="50" topLeftY="450" width="200" height="60">
                    <fill><fillColor color="rgba(232, 186, 176, 0.2)"/></fill>
                    <content/>
                  </shape>
                  <shape id="label" type="text" topLeftX="65" topLeftY="460" width="170" height="20">
                    <content fontSize="12"><p>基础保障</p></content>
                  </shape>
                </data>
              </slide>
            </presentation>
            """
        )
        issues = result["slides"][0]["issues"]
        self.assertEqual(result["summary"]["error_count"], 0)
        self.assertEqual(result["summary"]["warning_count"], 1)
        self.assertEqual(issues[0]["code"], "whiteboard_external_overlap")
        self.assertIn("static whiteboard container-bbox risk", issues[0]["hint"])
        self.assertIn("screenshot QA", issues[0]["hint"])
        self.assertEqual(issues[0]["elements"], ["wb", "card"])
        self.assertEqual(issues[0]["overlaps"][0]["overlap_height"], 50)

    def test_lint_xml_allows_whiteboard_internal_labels(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="http://www.larkoffice.com/sml/2.0">
                <data>
                  <whiteboard id="wb" topLeftX="100" topLeftY="120" width="420" height="260"/>
                  <shape id="label" type="text" topLeftX="150" topLeftY="180" width="180" height="40">
                    <content fontSize="14"><p>内部标注</p></content>
                  </shape>
                </data>
              </slide>
            </presentation>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 0)
        self.assertEqual(result["summary"]["warning_count"], 0)

    def test_lint_xml_allows_bottom_layer_full_slide_whiteboard_background(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="http://www.larkoffice.com/sml/2.0">
                <data>
                  <whiteboard id="background" topLeftX="0" topLeftY="0" width="960" height="540"/>
                  <shape id="title" type="text" topLeftX="80" topLeftY="80" width="420" height="80">
                    <content textType="title" fontSize="36"><p>Title</p></content>
                  </shape>
                </data>
              </slide>
            </presentation>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 0)
        self.assertEqual(result["summary"]["warning_count"], 0)

    def test_lint_xml_allows_whiteboard_inside_background_panel(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="http://www.larkoffice.com/sml/2.0">
                <data>
                  <shape id="panel" type="rect" topLeftX="40" topLeftY="100" width="880" height="340">
                    <fill><fillColor color="rgba(232, 186, 176, 0.2)"/></fill>
                    <content/>
                  </shape>
                  <whiteboard id="wb" topLeftX="60" topLeftY="120" width="840" height="300"/>
                </data>
              </slide>
            </presentation>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 0)
        self.assertEqual(result["summary"]["warning_count"], 0)

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
