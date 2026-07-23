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


class XmlTextOverlapLintGeometryTest(unittest.TestCase):
    def assertNoXmlTextOverlapLintErrors(self, result: dict, sample_name: str) -> None:
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

    def test_cli_suggests_input_flag_for_positional_argument(self) -> None:
        script_path = Path(xml_text_overlap_lint.__file__).resolve()
        input_path = "/sandboxdata/workspace/file/full_presentation.xml"

        completed = subprocess.run(
            [sys.executable, str(script_path), input_path],
            capture_output=True,
            check=False,
            text=True,
        )

        self.assertEqual(completed.returncode, 1)
        self.assertEqual(completed.stdout, "")
        self.assertEqual(
            completed.stderr,
            f"xml-text-overlap-lint error: unexpected argument: {input_path}, need --input\n",
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
                self.assertNoXmlTextOverlapLintErrors(result, sample_name)

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

    def test_lint_xml_single_slide_reports_out_of_canvas_and_blank_slide_errors(self) -> None:
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
        self.assertEqual(result["summary"]["error_count"], 2)
        self.assertEqual(
            [issue["code"] for issue in result["slides"][0]["errors"]],
            ["shape_out_of_canvas", "blank_slide"],
        )

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
                  <content fontSize="18" wrap="false"><p>这是一个足够长的中文文本用于检测跨越间隙的横向溢出</p></content>
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
        issue = result["slides"][0]["issues"][0]
        self.assertEqual(issue["code"], "bbox_overlap")
        self.assertEqual(issue["elements"], ["source", "target"])
        self.assertGreater(issue["measurement"]["intersection_area"], 0)
        self.assertIsNotNone(issue.get("hint"))

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
        self.assertEqual(result["summary"]["warning_count"], 1)
        self.assertEqual(result["slides"][0]["issues"][0]["code"], "text_may_overflow_shape")
        self.assertEqual(result["slides"][0]["issues"][0]["elements"], ["source"])

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

    def test_lint_xml_blocks_template_style_bleed_outside_canvas(self) -> None:
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
        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertEqual(result["summary"]["warning_count"], 0)
        self.assertEqual(result["slides"][0]["errors"][0]["code"], "img_out_of_canvas")

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

    def test_lint_xml_blocks_small_out_of_bounds_images(self) -> None:
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
        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertEqual(result["slides"][0]["errors"][0]["code"], "img_out_of_canvas")

    def test_lint_xml_blocks_out_of_canvas_images(self) -> None:
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
        self.assertEqual(result["summary"]["error_count"], 2)
        self.assertEqual(
            [issue["code"] for issue in result["slides"][0]["errors"]],
            ["img_out_of_canvas", "img_out_of_canvas"],
        )

    def test_lint_xml_blocks_full_bleed_images_outside_canvas(self) -> None:
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
        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertEqual(result["slides"][0]["errors"][0]["code"], "img_out_of_canvas")

    def test_lint_xml_reports_text_and_chart_out_of_canvas(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="http://www.larkoffice.com/sml/2.0">
                <data>
                  <shape id="outside-shape" type="text" topLeftX="-10" topLeftY="40" width="50" height="50"/>
                  <img id="outside-img" src="token" topLeftX="120" topLeftY="-20" width="50" height="50"/>
                  <chart id="outside-chart" topLeftX="900" topLeftY="100" width="100" height="100"/>
                </data>
              </slide>
            </presentation>
            """
        )
        issues = result["slides"][0]["issues"]
        self.assertEqual(result["summary"]["error_count"], 3)
        self.assertEqual(
            [(issue["code"], issue["elements"], issue["overflow"]) for issue in issues],
            [
                ("shape_out_of_canvas", ["outside-shape"], {"left": 10, "top": 0, "right": 0, "bottom": 0}),
                ("img_out_of_canvas", ["outside-img"], {"left": 0, "top": 20, "right": 0, "bottom": 0}),
                ("chart_out_of_canvas", ["outside-chart"], {"left": 0, "top": 0, "right": 40, "bottom": 0}),
            ],
        )

    def test_lint_xml_reports_line_out_of_canvas_with_structured_geometry(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="body" type="text" topLeftX="80" topLeftY="80" width="300" height="60">
                  <content fontSize="18"><p>Visible content</p></content>
                </shape>
                <line id="connector" startX="80" startY="120" endX="980" endY="120"/>
              </data>
            </slide>
            """
        )

        issue = result["slides"][0]["errors"][0]
        self.assertEqual(issue["code"], "line_out_of_canvas")
        self.assertEqual(issue["element_ids"], ["connector"])
        self.assertEqual(issue["measurement"]["overflow"]["right"], 20)
        self.assertEqual(issue["related_objects"][0]["kind"], "line")

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

    def test_lint_xml_uses_rotated_bounds_for_rect_and_image_canvas_validation(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="http://www.larkoffice.com/sml/2.0">
                <data>
                  <shape id="rotated-rect" type="rect" topLeftX="0" topLeftY="0" width="100" height="100" rotation="45"/>
                  <img id="rotated-image" topLeftX="860" topLeftY="200" width="100" height="100" rotation="45"/>
                </data>
              </slide>
            </presentation>
            """
        )
        issues_by_element = {issue["elements"][0]: issue for issue in result["slides"][0]["issues"]}
        self.assertEqual(result["summary"]["error_count"], 2)
        self.assertEqual(issues_by_element["rotated-rect"]["code"], "shape_out_of_canvas")
        self.assertAlmostEqual(issues_by_element["rotated-rect"]["overflow"]["left"], 20.710678, places=5)
        self.assertAlmostEqual(issues_by_element["rotated-rect"]["overflow"]["top"], 20.710678, places=5)
        self.assertEqual(issues_by_element["rotated-image"]["code"], "img_out_of_canvas")
        self.assertAlmostEqual(issues_by_element["rotated-image"]["overflow"]["right"], 20.710678, places=5)

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
        self.assertEqual(result["summary"]["warning_count"], 2)
        self.assertEqual(issues_by_dimension["width"]["level"], "warning")
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
            for name, (table_xml, expected_warning_count) in cases.items():
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
                    self.assertEqual(result["summary"]["warning_count"], expected_warning_count)
                    self.assertTrue(
                        all(issue["level"] == "warning" for issue in result["slides"][0]["issues"]),
                        result["slides"][0]["issues"],
                    )

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


    def test_lint_xml_reports_vertical_text_image_overlap_as_warning(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0"><data>
              <shape id="text" type="text" vert="vert" topLeftX="100" topLeftY="100" width="100" height="100">
                <content><p>Vertical</p></content>
              </shape>
              <img id="image" src="token" topLeftX="120" topLeftY="120" width="20" height="20"/>
            </data></slide>
            """
        )
        issue = next(issue for issue in result["slides"][0]["issues"] if issue["code"] == "image_may_cover_vertical_text")
        self.assertEqual(issue["level"], "warning")
        self.assertEqual(result["summary"]["error_count"], 0)


class XmlTextOverlapLintDensityTest(unittest.TestCase):
    def test_lint_xml_blocks_blank_slide(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide id="content-slide">
                <data>
                  <shape id="title" type="text" topLeftX="60" topLeftY="60" width="400" height="50">
                    <content fontSize="28"><p>Investment report</p></content>
                  </shape>
                </data>
              </slide>
              <slide id="blank-slide">
                <style><fill><fillColor color="rgba(255, 255, 255, 1)"/></fill></style>
                <data/>
                <note><content/></note>
              </slide>
            </presentation>
            """
        )

        self.assertEqual(result["summary"]["slide_count"], 2)
        self.assertEqual(result["summary"]["warning_count"], 0)
        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertEqual(result["summary"]["status"], "blocked")
        self.assertFalse(result["summary"]["release_ready"])
        self.assertEqual(result["slides"][0]["issues"], [])
        self.assertEqual(result["slides"][1]["element_count"], 0)
        issue = result["slides"][1]["errors"][0]
        self.assertEqual(issue["level"], "error")
        self.assertEqual(issue["code"], "blank_slide")
        self.assertEqual(issue["element_ids"], [])
        self.assertEqual(issue["rule"]["id"], "blank_slide")
        self.assertEqual(issue["measurement"]["visible_element_count"], 0)
        self.assertEqual(issue["related_objects"], [])

    def test_lint_xml_blocks_blank_slide_with_only_transparent_image(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <img id="ghost" topLeftX="60" topLeftY="60" width="200" height="200" alpha="0"/>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["summary"]["error_count"], 1)
        issue = result["slides"][0]["errors"][0]
        self.assertEqual(issue["code"], "blank_slide")

    def test_lint_xml_warns_when_large_container_is_mostly_empty(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="trend-card" type="rect" topLeftX="500" topLeftY="135" width="410" height="370"/>
                <shape id="trend-title" type="text" topLeftX="515" topLeftY="147" width="380" height="28">
                  <content fontSize="15"><p>Core trends</p></content>
                </shape>
                <shape id="trend-copy" type="text" topLeftX="515" topLeftY="177" width="380" height="315">
                  <content fontSize="12"><p>First point</p><p>Second point</p><p>Third point</p></content>
                </shape>
              </data>
            </slide>
            """
        )

        issue = result["slides"][0]["issues"][0]
        self.assertEqual(issue["code"], "sparse_container_content")
        self.assertEqual(issue["target"]["container_id"], "trend-card")
        self.assertEqual(issue["target"], {
            "slide_number": 1,
            "container_id": "trend-card",
            "container_type": "rect",
            "bbox": {"x": 500, "y": 135, "width": 410, "height": 370},
        })
        self.assertLess(issue["measurement"]["content_coverage_ratio"], 0.15)
        self.assertEqual(issue["rule"], {
            "name": "large_container_visible_content_coverage",
            "threshold": 0.15,
            "comparison": "content_coverage_ratio < threshold",
            "id": "sparse_container_content",
        })
        self.assertEqual(issue["measurement"]["container_area"], 151700)
        self.assertEqual(issue["measurement"]["content_coverage_ratio"], 0.032)
        self.assertEqual(issue["elements"], ["trend-card", "trend-title", "trend-copy"])
        self.assertEqual(issue["element_ids"], ["trend-card", "trend-title", "trend-copy"])
        self.assertEqual(
            [obj["element_id"] for obj in issue["related_objects"]],
            ["trend-card", "trend-title", "trend-copy"],
        )
        self.assertEqual(result["slides"][0]["status"], "needs_screenshot_review")
        self.assertEqual(result["slides"][0]["warnings"], result["slides"][0]["issues"])

    def test_lint_xml_warns_for_sparse_short_cards(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="card-1" type="rect" topLeftX="60" topLeftY="180" width="400" height="105"/>
                <shape id="text-1" type="text" topLeftX="80" topLeftY="220" width="360" height="30">
                  <content fontSize="14"><p>期待认识大家</p></content>
                </shape>
                <shape id="card-2" type="rect" topLeftX="490" topLeftY="180" width="400" height="105"/>
                <shape id="text-2" type="text" topLeftX="510" topLeftY="220" width="360" height="30">
                  <content fontSize="14"><p>化学一起讨论</p></content>
                </shape>
                <shape id="card-3" type="rect" topLeftX="60" topLeftY="310" width="400" height="105"/>
                <shape id="text-3" type="text" topLeftX="80" topLeftY="350" width="360" height="30">
                  <content fontSize="14"><p>吉他随时交流</p></content>
                </shape>
                <shape id="card-4" type="rect" topLeftX="490" topLeftY="310" width="400" height="105"/>
                <shape id="text-4" type="text" topLeftX="510" topLeftY="350" width="360" height="30">
                  <content fontSize="14"><p>共度美好四年</p></content>
                </shape>
              </data>
            </slide>
            """
        )

        container_issues = [
            issue for issue in result["slides"][0]["issues"] if issue["code"] == "sparse_container_content"
        ]
        self.assertEqual(
            [issue["target"]["container_id"] for issue in container_issues],
            ["card-1", "card-2", "card-3", "card-4"],
        )
        self.assertTrue(all(issue["target"]["bbox"]["height"] == 105 for issue in container_issues))
        self.assertTrue(all(issue["measurement"]["content_coverage_ratio"] < 0.15 for issue in container_issues))
        self.assertEqual(
            [issue["code"] for issue in result["slides"][0]["issues"]],
            [
                "sparse_container_content",
                "sparse_container_content",
                "sparse_container_content",
                "sparse_container_content",
                "sparse_slide_content",
            ],
        )

    def test_lint_xml_warns_when_whole_slide_has_too_little_effective_content(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="background" type="rect" topLeftX="0" topLeftY="0" width="960" height="540"/>
                <shape id="text-1" type="text" topLeftX="60" topLeftY="80" width="200" height="30">
                  <content fontSize="14"><p>One short line</p></content>
                </shape>
                <shape id="text-2" type="text" topLeftX="500" topLeftY="180" width="200" height="30">
                  <content fontSize="14"><p>Another line</p></content>
                </shape>
                <shape id="text-3" type="text" topLeftX="60" topLeftY="310" width="200" height="30">
                  <content fontSize="14"><p>Third line</p></content>
                </shape>
                <shape id="text-4" type="text" topLeftX="500" topLeftY="410" width="200" height="30">
                  <content fontSize="14"><p>Fourth line</p></content>
                </shape>
              </data>
            </slide>
            """
        )

        issues = [issue for issue in result["slides"][0]["issues"] if issue["code"] == "sparse_slide_content"]
        self.assertEqual(len(issues), 1)
        issue = issues[0]
        self.assertEqual(issue["target"]["bbox"], {"x": 0, "y": 0, "width": 960, "height": 540})
        self.assertEqual(issue["rule"]["threshold"], 0.035)
        self.assertLess(issue["measurement"]["content_coverage_ratio"], 0.035)
        self.assertEqual(issue["measurement"]["content_element_count"], 4)
        self.assertNotIn("background", issue["elements"])

    def test_lint_xml_ignores_isolated_short_layout_bar(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="summary-bar" type="rect" topLeftX="52" topLeftY="82" width="856" height="105"/>
                <shape id="summary" type="text" topLeftX="72" topLeftY="115" width="816" height="30">
                  <content fontSize="14"><p>One concise summary</p></content>
                </shape>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["slides"][0]["issues"], [])

    def test_lint_xml_counts_rect_own_content_as_visible_content(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="load-card" type="rect" topLeftX="60" topLeftY="140" width="220" height="184">
                  <content fontSize="18">
                    <p>被吊物</p>
                    <p><span fontSize="36">32.0 t</span></p>
                    <p>钢结构模块</p>
                  </content>
                </shape>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["slides"][0]["issues"], [])

    def test_lint_xml_reports_nonzero_coverage_for_rect_own_content_reproduction(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="load-card" type="rect" topLeftX="60" topLeftY="140" width="220" height="184">
                  <content fontSize="18">
                    <p>被吊物</p>
                    <p>32.0 t</p>
                    <p>钢结构模块</p>
                  </content>
                </shape>
              </data>
            </slide>
            """
        )

        issue = result["slides"][0]["issues"][0]
        self.assertGreater(issue["measurement"]["visible_content_area"], 0)
        self.assertEqual(issue["measurement"]["content_element_count"], 1)
        self.assertGreater(issue["measurement"]["content_coverage_ratio"], 0)

    def test_lint_xml_still_warns_for_sparse_rect_own_content(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="sparse-card" type="rect" topLeftX="60" topLeftY="140" width="220" height="184">
                  <content fontSize="12"><p>A</p></content>
                </shape>
              </data>
            </slide>
            """
        )

        issue = result["slides"][0]["issues"][0]
        self.assertEqual(issue["target"]["container_id"], "sparse-card")
        self.assertGreater(issue["measurement"]["visible_content_area"], 0)
        self.assertEqual(issue["measurement"]["content_element_count"], 1)
        self.assertEqual(issue["elements"], ["sparse-card"])

    def test_lint_xml_unions_rect_own_content_with_child_content(self) -> None:
        self_only = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="card" type="rect" topLeftX="60" topLeftY="140" width="220" height="184">
                  <content fontSize="12"><p>A</p></content>
                </shape>
              </data>
            </slide>
            """
        )
        with_overlapping_child = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="card" type="rect" topLeftX="60" topLeftY="140" width="220" height="184">
                  <content fontSize="12"><p>A</p></content>
                </shape>
                <shape id="child" type="text" topLeftX="60" topLeftY="140" width="220" height="184">
                  <content fontSize="12"><p>A</p></content>
                </shape>
              </data>
            </slide>
            """
        )

        self_issue = self_only["slides"][0]["issues"][0]
        mixed_issue = with_overlapping_child["slides"][0]["issues"][0]
        self.assertEqual(
            mixed_issue["measurement"]["visible_content_area"],
            self_issue["measurement"]["visible_content_area"],
        )
        self.assertEqual(mixed_issue["measurement"]["content_element_count"], 2)

    def test_extract_density_elements_reads_nested_font_size_from_rect_content(self) -> None:
        elements = xml_text_overlap_lint.extract_density_elements(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="card" type="rect" topLeftX="60" topLeftY="140" width="220" height="184">
                  <content fontSize="12"><p><span fontSize="36">32.0 t</span></p></content>
                </shape>
              </data>
            </slide>
            """
        )

        self.assertEqual(elements[0]["fontSize"], 36)

    def test_extract_density_elements_does_not_attach_following_text_to_self_closing_rect(self) -> None:
        elements = xml_text_overlap_lint.extract_density_elements(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="card" type="rect" topLeftX="60" topLeftY="140" width="220" height="184"/>
                <shape id="title" type="text" topLeftX="80" topLeftY="160" width="180" height="30">
                  <content fontSize="18"><p>Following title</p></content>
                </shape>
              </data>
            </slide>
            """
        )

        self.assertEqual(elements[0]["text"], "")
        self.assertEqual(elements[1]["text"], "Following title")

    def test_lint_xml_allows_container_with_large_visual_child(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="chart-card" type="rect" topLeftX="500" topLeftY="135" width="410" height="300"/>
                <chart id="chart" topLeftX="525" topLeftY="170" width="350" height="220"/>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["summary"]["warning_count"], 0)

    def test_lint_xml_does_not_let_transparent_visual_child_suppress_sparse_warning(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="title" type="text" topLeftX="40" topLeftY="40" width="300" height="40">
                  <content fontSize="20"><p>Section title</p></content>
                </shape>
                <shape id="chart-card" type="rect" topLeftX="500" topLeftY="135" width="410" height="300"/>
                <chart id="chart" topLeftX="525" topLeftY="170" width="350" height="220" alpha="0"/>
              </data>
            </slide>
            """
        )

        issue = next(
            issue for issue in result["slides"][0]["issues"] if issue["code"] == "sparse_container_content"
        )
        self.assertEqual(issue["target"]["container_id"], "chart-card")

    def test_lint_xml_warns_for_small_empty_visual_placeholder_cards(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="letter-placeholder" type="rect" topLeftX="520" topLeftY="180" width="200" height="200"/>
                <shape id="letter" type="text" topLeftX="540" topLeftY="250" width="160" height="70">
                  <content fontSize="46"><p>Z</p></content>
                </shape>
                <shape id="empty-placeholder" type="rect" topLeftX="744" topLeftY="180" width="144" height="200"/>
              </data>
            </slide>
            """
        )

        issues = result["slides"][0]["issues"]
        self.assertEqual(
            [issue["target"]["container_id"] for issue in issues],
            ["letter-placeholder", "empty-placeholder"],
        )
        self.assertEqual(issues[1]["measurement"]["content_element_count"], 0)

    def test_lint_xml_applies_global_threshold_to_normal_text_card(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="card" type="rect" topLeftX="70" topLeftY="184" width="260" height="288"/>
                <shape id="title" type="text" topLeftX="90" topLeftY="215" width="220" height="30">
                  <content fontSize="18"><p>梦境与现实</p></content>
                </shape>
                <shape id="copy" type="text" topLeftX="90" topLeftY="330" width="220" height="70">
                  <content fontSize="13"><p>边界溶解，逻辑失效。观众被拽入潜意识的迷宫。</p></content>
                </shape>
              </data>
            </slide>
            """
        )

        issue = result["slides"][0]["issues"][0]
        self.assertEqual(issue["target"]["container_id"], "card")
        self.assertEqual(issue["rule"]["threshold"], 0.15)

    def test_lint_xml_allows_image_overlay_rect(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <img id="hero" topLeftX="560" topLeftY="0" width="400" height="540"/>
                <shape id="tint" type="rect" topLeftX="560" topLeftY="0" width="400" height="540"/>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["summary"]["warning_count"], 0)

    def test_lint_xml_does_not_let_transparent_image_overlay_suppress_sparse_warning(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="title" type="text" topLeftX="40" topLeftY="40" width="300" height="40">
                  <content fontSize="20"><p>Section title</p></content>
                </shape>
                <shape id="card" type="rect" topLeftX="330" topLeftY="120" width="300" height="300"/>
                <img id="ghost-overlay" topLeftX="330" topLeftY="120" width="300" height="300" alpha="0"/>
              </data>
            </slide>
            """
        )

        issue = next(
            issue for issue in result["slides"][0]["issues"] if issue["code"] == "sparse_container_content"
        )
        self.assertEqual(issue["target"]["container_id"], "card")

    def test_lint_xml_allows_edge_spanning_layout_panel_and_nested_decoration(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="panel" type="rect" topLeftX="600" topLeftY="0" width="360" height="540"/>
                <shape id="decoration" type="rect" topLeftX="660" topLeftY="150" width="240" height="240"/>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["summary"]["warning_count"], 0)

    def test_lint_xml_counts_icons_as_visible_content(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="card" type="rect" topLeftX="80" topLeftY="140" width="320" height="240"/>
                <icon id="visual" iconType="iconpark/Safe/shield.svg" topLeftX="100" topLeftY="160" width="180" height="180">
                  <fill><fillColor color="rgba(37, 99, 235, 1)"/></fill>
                </icon>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["summary"]["warning_count"], 0)

    def test_lint_xml_does_not_count_transparent_icon_as_visible_content(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="title" type="text" topLeftX="40" topLeftY="40" width="300" height="40">
                  <content fontSize="20"><p>Section title</p></content>
                </shape>
                <shape id="card" type="rect" topLeftX="80" topLeftY="140" width="320" height="240"/>
                <icon id="visual" iconType="iconpark/Safe/shield.svg" topLeftX="100" topLeftY="160" width="180" height="180" alpha="0">
                  <fill><fillColor color="rgba(37, 99, 235, 1)"/></fill>
                </icon>
              </data>
            </slide>
            """
        )

        issue = next(
            issue for issue in result["slides"][0]["issues"] if issue["code"] == "sparse_container_content"
        )
        self.assertEqual(issue["target"]["container_id"], "card")
        self.assertEqual(issue["measurement"]["content_coverage_ratio"], 0)

    def test_lint_xml_warns_when_coverage_is_below_global_threshold(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="card" type="rect" topLeftX="80" topLeftY="140" width="200" height="200"/>
                <icon id="visual" iconType="iconpark/Safe/shield.svg" topLeftX="100" topLeftY="160" width="70" height="70">
                  <fill><fillColor color="rgba(37, 99, 235, 1)"/></fill>
                </icon>
              </data>
            </slide>
            """
        )

        issue = result["slides"][0]["issues"][0]
        self.assertEqual(issue["target"]["container_id"], "card")
        self.assertEqual(issue["measurement"]["content_coverage_ratio"], 0.122)
        self.assertEqual(issue["rule"]["threshold"], 0.15)

    def test_lint_xml_allows_quarter_coverage_under_lower_threshold(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="card" type="rect" topLeftX="80" topLeftY="140" width="200" height="200"/>
                <icon id="visual" iconType="iconpark/Safe/shield.svg" topLeftX="100" topLeftY="160" width="100" height="100">
                  <fill><fillColor color="rgba(37, 99, 235, 1)"/></fill>
                </icon>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["slides"][0]["issues"], [])

    def test_lint_xml_allows_large_metric_card_above_lower_threshold(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="metric-card" type="rect" topLeftX="80" topLeftY="140" width="360" height="300"/>
                <shape id="metric" type="text" topLeftX="104" topLeftY="190" width="340" height="90">
                  <content fontSize="12.4"><p><strong><span fontSize="62">400</span></strong>+ 项</p></content>
                </shape>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["slides"][0]["issues"], [])

    def test_lint_xml_does_not_report_blank_slide_for_line_only_content(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <line id="l1" startX="100" startY="100" endX="800" endY="100"/>
                <line id="l2" startX="100" startY="200" endX="800" endY="200"/>
                <line id="l3" startX="100" startY="300" endX="800" endY="300"/>
                <line id="l4" startX="100" startY="400" endX="800" endY="400"/>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["summary"]["error_count"], 0)
        codes = [issue["code"] for issue in result["slides"][0]["issues"]]
        self.assertNotIn("blank_slide", codes)

    def test_lint_xml_reports_bbox_overlap_measurement_from_decision_time_visual_bbox(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="left" type="text" topLeftX="80" topLeftY="80" width="300" height="60">
                  <content fontSize="14"><p>overlap text <span fontSize="96">big</span></p></content>
                </shape>
                <shape id="right" type="text" topLeftX="80" topLeftY="80" width="300" height="80">
                  <content fontSize="14"><p>other overlap text</p></content>
                </shape>
              </data>
            </slide>
            """
        )

        issue = result["slides"][0]["issues"][0]
        self.assertEqual(issue["code"], "bbox_overlap")
        # Must match the visual bbox that should_flag_overlap actually decided with (fontSize=14
        # from extract_elements), not the fontSize=96 max-descendant value that
        # extract_density_elements computes for the same "left" element id.
        self.assertEqual(issue["measurement"]["intersection_width"], 117.04)
        self.assertEqual(issue["measurement"]["intersection_height"], 6.8)
        self.assertEqual(issue["measurement"]["intersection_area"], 795.872)

    def test_has_similar_short_card_peer_excludes_the_element_itself(self) -> None:
        card_a = {"kind": "shape", "type": "rect", "x": 0, "y": 0, "width": 300, "height": 100}
        card_b = {"kind": "shape", "type": "rect", "x": 400, "y": 0, "width": 300, "height": 100}
        card_c = {"kind": "shape", "type": "rect", "x": 0, "y": 200, "width": 300, "height": 100}

        self.assertFalse(
            xml_text_overlap_lint.has_similar_short_card_peer(card_a, [card_a, card_b])
        )
        self.assertTrue(
            xml_text_overlap_lint.has_similar_short_card_peer(card_a, [card_a, card_b, card_c])
        )

    def test_lint_xml_reports_schema_version_2_for_sparse_issues(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="card" type="rect" topLeftX="60" topLeftY="140" width="220" height="184"/>
              </data>
            </slide>
            """
        )

        issue = next(
            issue for issue in result["slides"][0]["issues"] if issue["code"] == "sparse_container_content"
        )
        self.assertEqual(issue["schema_version"], "2.0")

    def test_lint_xml_does_not_report_blank_slide_for_textless_decorative_shapes(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="deco1" type="ellipse" topLeftX="60" topLeftY="60" width="300" height="300">
                  <fill><fillColor color="rgba(37, 99, 235, 1)"/></fill>
                </shape>
                <shape id="deco2" type="triangle" topLeftX="500" topLeftY="200" width="200" height="200">
                  <fill><fillColor color="rgba(220, 38, 38, 1)"/></fill>
                </shape>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["summary"]["error_count"], 0)
        codes = [issue["code"] for issue in result["slides"][0]["issues"]]
        self.assertNotIn("blank_slide", codes)

    def test_lint_xml_still_warns_for_sparse_slide_content_despite_full_bleed_background(self) -> None:
        # A plain textless shape now counts as "not blank" (see the test above), but a
        # full-bleed background rect must still NOT count toward sparse_slide_content's
        # meaningful-content coverage ratio -- otherwise every slide with a background would
        # trivially "pass" that density check.
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="background" type="rect" topLeftX="0" topLeftY="0" width="960" height="540"/>
                <shape id="text-1" type="text" topLeftX="60" topLeftY="80" width="200" height="30">
                  <content fontSize="14"><p>One short line</p></content>
                </shape>
                <shape id="text-2" type="text" topLeftX="500" topLeftY="180" width="200" height="30">
                  <content fontSize="14"><p>Another line</p></content>
                </shape>
                <shape id="text-3" type="text" topLeftX="60" topLeftY="310" width="200" height="30">
                  <content fontSize="14"><p>Third line</p></content>
                </shape>
                <shape id="text-4" type="text" topLeftX="500" topLeftY="410" width="200" height="30">
                  <content fontSize="14"><p>Fourth line</p></content>
                </shape>
              </data>
            </slide>
            """
        )

        codes = [issue["code"] for issue in result["slides"][0]["issues"]]
        self.assertIn("sparse_slide_content", codes)

    def test_lint_xml_accepts_whitespace_around_attribute_equals_sign(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="visible" type="text" topLeftX = "80" topLeftY = "80" width = "300" height = "60">
                  <content><p>hello</p></content>
                </shape>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["slides"][0]["element_count"], 1)
        codes = [issue["code"] for issue in result["slides"][0]["issues"]]
        self.assertNotIn("blank_slide", codes)

    def test_lint_xml_reports_blank_slide_for_full_canvas_background_only(self) -> None:
        result = xml_text_overlap_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="background" type="rect" topLeftX="0" topLeftY="0" width="960" height="540">
                  <fill><fillColor color="rgba(240, 235, 220, 1)"/></fill>
                </shape>
              </data>
            </slide>
            """
        )

        codes = [issue["code"] for issue in result["slides"][0]["issues"]]
        self.assertIn("blank_slide", codes)

    def test_has_similar_short_card_peer_ignores_invisible_peers(self) -> None:
        visible_card = {"kind": "shape", "type": "rect", "x": 0, "y": 0, "width": 300, "height": 100}
        ghost_1 = {
            "kind": "shape", "type": "rect", "x": 400, "y": 0, "width": 300, "height": 100, "alpha": 0,
        }
        ghost_2 = {
            "kind": "shape", "type": "rect", "x": 800, "y": 0, "width": 300, "height": 100, "alpha": 0,
        }

        self.assertFalse(
            xml_text_overlap_lint.has_similar_short_card_peer(
                visible_card, [visible_card, ghost_1, ghost_2]
            )
        )


if __name__ == "__main__":
    unittest.main()
