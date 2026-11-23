# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
from __future__ import annotations

import itertools
import json
import subprocess
import sys
import tempfile
import unittest
import xml.etree.ElementTree as ET
from pathlib import Path
from unittest import mock

import sxsd_validator
import xml_lint


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
        script_path = Path(xml_lint.__file__).resolve()
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
            f"xml-lint error: unexpected argument: {input_path}, need --input\n",
        )

    def test_cli_preserves_requested_symlink_path_in_result(self) -> None:
        script_path = Path(xml_lint.__file__).resolve()
        with tempfile.TemporaryDirectory() as temp_dir:
            temp_path = Path(temp_dir)
            resolved_path = temp_path / "resolved.xml"
            requested_path = temp_path / "requested.xml"
            resolved_path.write_text(
                '<presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">'
                '<slide xmlns="https://www.larkoffice.com/sml/2.0"><data>'
                '<shape type="text" topLeftX="10" topLeftY="10" width="100" height="30">'
                '<content><p><span>Test</span></p></content>'
                '</shape></data></slide>'
                '</presentation>',
                encoding="utf-8",
            )
            requested_path.symlink_to(resolved_path)

            completed = subprocess.run(
                [sys.executable, str(script_path), "--input", str(requested_path)],
                capture_output=True,
                check=False,
                text=True,
            )

        self.assertEqual(completed.returncode, 0, completed.stderr)
        result = json.loads(completed.stdout)
        self.assertEqual(result["file"], str(requested_path))

    def test_cli_reports_structured_slide_sxsd_error_outside_skill_directory(self) -> None:
        script_path = Path(xml_lint.__file__).resolve()
        with tempfile.TemporaryDirectory() as temp_dir:
            input_path = Path(temp_dir) / "invalid-slide.xml"
            input_path.write_text(
                """
                <slide xmlns="https://www.larkoffice.com/sml/2.0">
                  <data><shape type="text" topLeftX="10" topLeftY="20" width="300"/></data>
                </slide>
                """,
                encoding="utf-8",
            )

            completed = subprocess.run(
                [sys.executable, str(script_path), "--input", str(input_path)],
                cwd=temp_dir,
                capture_output=True,
                check=False,
                text=True,
            )

        result = json.loads(completed.stdout)
        issue = result["slides"][0]["errors"][0]
        self.assertEqual(completed.returncode, 1)
        self.assertEqual(completed.stderr, "")
        self.assertEqual(issue["code"], "sxsd_missing_required_attr")
        self.assertEqual(issue["path"], "slide/data/shape")
        self.assertEqual(issue["attr"], "height")
        self.assertEqual(issue["target"]["slide_number"], 1)
        self.assertTrue(issue["hint"])

    def test_xml_lint_accepts_inline_fixture_xml_samples(self) -> None:
        samples = {
            "image-led-cover": """
                <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
                  <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
                <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
                  <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
                result = xml_lint.lint_xml(
                    sample_xml,
                    sample_name,
                )
                self.assertNoXmlTextOverlapLintErrors(result, sample_name)

    def test_lint_xml_reports_unescaped_ampersand_in_text(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        self.assertEqual(issue["related_objects"], [])
        self.assertNotIn("Locate via related_objects[].xml_path.", issue["hint"])

    def test_lint_xml_rejects_prefixed_sml_tags(self) -> None:
        result = xml_lint.lint_xml(
            """
            <ns0:slide xmlns:ns0="https://www.larkoffice.com/sml/2.0">
              <ns0:data>
                <sml:shape xmlns:sml="https://www.larkoffice.com/sml/2.0" type="text" topLeftX="80" topLeftY="80" width="300" height="60">
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
        self.assertEqual({issue["namespace"] for issue in issues}, {SML_NAMESPACE})

    def test_lint_xml_reports_bound_namespace_for_prefixed_sml_tags(self) -> None:
        for namespace in (
            "http://www.larkoffice.com/sml/2.0",
            "/sml/2.0",
            SML_NAMESPACE,
        ):
            with self.subTest(namespace=namespace):
                result = xml_lint.lint_xml(
                    f"""
                    <sml:slide xmlns:sml="{namespace}">
                      <sml:data>
                        <sml:shape type="text" topLeftX="80" topLeftY="80" width="300" height="60">
                          <sml:content textType="body"><sml:p>Prefixed SML</sml:p></sml:content>
                        </sml:shape>
                      </sml:data>
                    </sml:slide>
                    """
                )
                issues = result["issues"]
                self.assertEqual([issue["code"] for issue in issues], ["sml_prefixed_tag"] * 5)
                self.assertEqual({issue["namespace"] for issue in issues}, {namespace})

    def test_lint_xml_accepts_server_readback_slide_without_namespace(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide id="server-slide-id">
              <data>
                <shape id="server-shape-id" type="text" topLeftX="80" topLeftY="80" width="300" height="60">
                  <content textType="body"><p>Unprefixed SML</p></content>
                </shape>
              </data>
            </slide>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 0)

    def test_lint_xml_accepts_server_readback_presentation_short_namespace(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="/sml/2.0" width="960" height="540" id="server-presentation-id">
              <slide id="server-slide-id">
                <data>
                  <shape id="server-shape-id" type="text" topLeftX="80" topLeftY="80" width="300" height="60">
                    <content textType="body"><p>Server readback presentation</p></content>
                  </shape>
                </data>
              </slide>
            </presentation>
            """
        )

        self.assertEqual(result["summary"]["slide_count"], 1)
        self.assertEqual(result["summary"]["error_count"], 0)

    def test_lint_xml_accepts_server_readback_presentation_https_namespace(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540" id="server-presentation-id">
              <slide id="server-slide-id">
                <data>
                  <shape id="server-shape-id" type="text" topLeftX="80" topLeftY="80" width="300" height="60">
                    <content textType="body"><p>Server readback presentation</p></content>
                  </shape>
                </data>
              </slide>
            </presentation>
            """
        )

        self.assertEqual(result["summary"]["slide_count"], 1)
        self.assertEqual(result["summary"]["error_count"], 0)

    def test_lint_xml_accepts_legacy_http_namespace(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540" id="legacy-http-id">
              <slide id="server-slide-id">
                <data>
                  <shape id="server-shape-id" type="text" topLeftX="80" topLeftY="80" width="300" height="60">
                    <content textType="body"><p>Legacy HTTP namespace</p></content>
                  </shape>
                </data>
              </slide>
            </presentation>
            """
        )

        self.assertEqual(result["summary"]["slide_count"], 1)
        self.assertEqual(result["summary"]["error_count"], 0)

    def test_lint_xml_rejects_server_readback_presentation_wrong_namespace(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://example.com/not-sml" width="960" height="540">
              <slide/>
            </presentation>
            """
        )

        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertEqual(result["issues"][0]["code"], "sxsd_invalid_namespace")

    def test_lint_xml_rejects_xml_declaration(self) -> None:
        result = xml_lint.lint_xml(
            '<?xml version="1.0"?><slide xmlns="https://www.larkoffice.com/sml/2.0"><data/></slide>'
        )

        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertEqual(result["issues"][0]["code"], "sxsd_unsupported_declaration")

    def test_lint_xml_reports_missing_required_sxsd_attribute(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data><shape type="text" topLeftX="80" topLeftY="80" width="300"/></data>
            </slide>
            """
        )

        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertNotIn("issues", result)
        issue = result["slides"][0]["issues"][0]
        self.assertEqual(issue["code"], "sxsd_missing_required_attr")
        self.assertEqual(issue["attr"], "height")

    def test_lint_xml_rejects_child_order_that_violates_xsd_sequence(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide/>
              <title>Late title</title>
            </presentation>
            """
        )

        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertEqual(result["issues"][0]["code"], "sxsd_invalid_child_order")

    def test_lint_xml_accepts_escaped_entities_without_suspicious_entity_warning(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="1280" height="720">
              <slide xmlns="https://www.larkoffice.com/sml/2.0">
                <data>
                  <shape id="first" type="text" topLeftX="80" topLeftY="80" width="300" height="60">
                    <content textType="body"><p>First slide</p></content>
                  </shape>
                </data>
              </slide>
              <slide xmlns="https://www.larkoffice.com/sml/2.0">
                <data>
                  <img id="second" src="tok" topLeftX="100" topLeftY="120" width="240" height="160"/>
                  <shape id="second-shape" type="text" topLeftX="80" topLeftY="80" width="300" height="60">
                    <content textType="body"><p>Second slide</p></content>
                  </shape>
                </data>
              </slide>
            </presentation>
            """
        )
        self.assertEqual(result["slide_size"], {"width": 1280, "height": 720})
        self.assertEqual(result["summary"]["slide_count"], 2)
        self.assertEqual([slide["slide_number"] for slide in result["slides"]], [1, 2])
        self.assertEqual([slide["element_count"] for slide in result["slides"]], [1, 2])
        self.assertEqual(result["summary"]["error_count"], 0)
        self.assertEqual(result["summary"]["warning_count"], 0)

    def test_lint_xml_handles_self_closing_slide_before_normal_slide(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide/>
              <slide>
                <data>
                  <shape type="text" topLeftX="80" topLeftY="80" width="300" height="60">
                    <content textType="body"><p>Second slide</p></content>
                  </shape>
                </data>
              </slide>
            </presentation>
            """
        )

        self.assertEqual(result["summary"]["slide_count"], 2)
        self.assertEqual(
            [issue["code"] for issue in result["slides"][0]["errors"]],
            ["blank_slide"],
        )
        self.assertEqual(result["slides"][1]["status"], "passed")

    def test_lint_xml_keeps_trailing_self_closing_slide(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide>
                <data>
                  <shape type="text" topLeftX="80" topLeftY="80" width="300" height="60">
                    <content textType="body"><p>First slide</p></content>
                  </shape>
                </data>
              </slide>
              <slide/>
            </presentation>
            """
        )

        self.assertEqual(result["summary"]["slide_count"], 2)
        self.assertEqual(
            [issue["code"] for issue in result["slides"][1]["errors"]],
            ["blank_slide"],
        )

    def test_lint_xml_ignores_slide_markup_inside_xml_comments(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide>
                <data>
                  <shape type="text" topLeftX="80" topLeftY="80" width="300" height="60">
                    <content textType="body"><p>Real slide</p></content>
                  </shape>
                </data>
              </slide>
              <!-- Old draft: <slide id="ghost"/> -->
            </presentation>
            """
        )

        self.assertEqual(result["summary"]["slide_count"], 1)
        self.assertEqual(result["slides"][0]["status"], "passed")

    def test_lint_xml_ignores_invalid_slide_markup_inside_xml_comments(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide><data/></slide>
              <!-- Old draft: <slide bogus="x"/> -->
            </presentation>
            """
        )

        self.assertEqual(result["summary"]["slide_count"], 1)
        self.assertEqual(
            [issue["code"] for issue in result["slides"][0]["errors"]],
            ["blank_slide"],
        )

    def test_lint_xml_skips_only_invalid_slide_and_continues_geometry_checks(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide>
                <data>
                  <shape id="invalid" type="text" topLeftX="1000" topLeftY="80" width="300">
                    <content textType="body"><p>Missing height</p></content>
                  </shape>
                </data>
              </slide>
              <slide>
                <data>
                  <shape id="outside" type="text" topLeftX="1000" topLeftY="80" width="120" height="60">
                    <content textType="body"><p>Outside canvas</p></content>
                  </shape>
                </data>
              </slide>
            </presentation>
            """
        )

        self.assertEqual(result["summary"]["slide_count"], 2)
        self.assertEqual(result["slides"][0]["element_count"], 0)
        self.assertEqual(
            [issue["code"] for issue in result["slides"][0]["errors"]],
            ["sxsd_missing_required_attr"],
        )
        self.assertNotIn(
            "shape_out_of_canvas",
            [issue["code"] for issue in result["slides"][0]["issues"]],
        )
        self.assertIn(
            "shape_out_of_canvas",
            [issue["code"] for issue in result["slides"][1]["errors"]],
        )

    def test_lint_xml_scopes_invalid_slide_root_attribute_to_that_slide(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide bogusAttr="oops">
                <data><shape type="rect" topLeftX="80" topLeftY="80" width="100" height="100"/></data>
              </slide>
              <slide>
                <data><shape type="rect" topLeftX="1000" topLeftY="80" width="100" height="100"/></data>
              </slide>
            </presentation>
            """
        )

        self.assertEqual(result["summary"]["slide_count"], 2)
        self.assertEqual(
            [issue["code"] for issue in result["slides"][0]["errors"]],
            ["sxsd_unsupported_attr"],
        )
        self.assertIn(
            "shape_out_of_canvas",
            [issue["code"] for issue in result["slides"][1]["errors"]],
        )

    def test_lint_xml_allows_svg_subtree_inside_embed(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <embed topLeftX="80" topLeftY="120" width="240" height="140">
                  <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 240 140">
                    <rect x="10" y="10" width="220" height="120" rx="12" fill="#EFF6FF"/>
                    <circle cx="70" cy="70" r="34" fill="#2563EB"/>
                    <text x="130" y="76" font-size="18" fill="#1E3A8A">SVG OK</text>
                    <foreignObject x="0" y="0" width="1" height="1">
                      <embed xmlns="http://www.w3.org/1999/xhtml">
                        <rect xmlns="http://www.w3.org/2000/svg" width="1" height="1"/>
                      </embed>
                    </foreignObject>
                  </svg>
                </embed>
              </data>
            </slide>
            """
        )
        codes = [issue["code"] for issue in result["slides"][0]["issues"]]
        self.assertNotIn("sxsd_unsupported_tag", codes)
        self.assertNotIn("sxsd_unsupported_attr", codes)
        self.assertEqual(result["summary"]["error_count"], 0)

    def test_lint_xml_enforces_embed_svg_contract(self) -> None:
        cases = [
            ("missing SVG", "", "sxsd_missing_required_child"),
            ("wrong namespace", '<svg viewBox="0 0 20 20"/>', "sxsd_unsupported_tag"),
            (
                "multiple SVG roots",
                '<svg xmlns="http://www.w3.org/2000/svg"/><svg xmlns="http://www.w3.org/2000/svg"/>',
                "sxsd_too_many_children",
            ),
            (
                "non-SVG root element",
                '<rect xmlns="http://www.w3.org/2000/svg" width="20" height="20"/>',
                "sxsd_unexpected_child",
            ),
            (
                "SVG after reflection",
                '<reflection/><svg xmlns="http://www.w3.org/2000/svg"/>',
                "sxsd_invalid_child_order",
            ),
        ]
        for name, children, expected_code in cases:
            with self.subTest(name=name):
                result = xml_lint.lint_xml(
                    f"""
                    <slide xmlns="http://www.larkoffice.com/sml/2.0">
                      <data>
                        <embed topLeftX="80" topLeftY="120" width="240" height="140">
                          {children}
                        </embed>
                      </data>
                    </slide>
                    """
                )
                codes = [issue["code"] for issue in result["slides"][0]["issues"]]
                self.assertIn(expected_code, codes)

    def test_lint_xml_enforces_embed_svg_root_for_readback_namespaces(self) -> None:
        document_templates = [
            ("bare slide", "<slide>{content}</slide>"),
            (
                "short namespace",
                '<presentation xmlns="/sml/2.0" width="960" height="540">'
                "<slide>{content}</slide>"
                "</presentation>",
            ),
            (
                "HTTPS namespace",
                '<presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">'
                "<slide>{content}</slide>"
                "</presentation>",
            ),
        ]
        embed_template = """
            <data>
              <embed topLeftX="80" topLeftY="120" width="240" height="140">
                {svg_root}
              </embed>
            </data>
        """
        roots = [
            ("valid SVG root", '<svg xmlns="http://www.w3.org/2000/svg"/>', False),
            (
                "non-SVG root element",
                '<rect xmlns="http://www.w3.org/2000/svg" width="20" height="20"/>',
                True,
            ),
        ]

        for document_name, document_template in document_templates:
            for root_name, svg_root, should_error in roots:
                with self.subTest(document=document_name, root=root_name):
                    content = embed_template.format(svg_root=svg_root)
                    result = xml_lint.lint_xml(
                        document_template.format(content=content)
                    )
                    codes = [
                        issue["code"]
                        for slide in result["slides"]
                        for issue in slide["issues"]
                    ]
                    if should_error:
                        self.assertIn("sxsd_unexpected_child", codes)
                    else:
                        self.assertNotIn("sxsd_unexpected_child", codes)
                        self.assertEqual(result["summary"]["error_count"], 0)

    def test_lint_xml_reports_sxsd_unsupported_tag_with_alias_hint(self) -> None:
        cases = [
            ("textbox", '<textbox topLeftX="80" topLeftY="80" width="300" height="60">Text</textbox>', '<shape type="text">'),
            ("image", '<image src="tok" topLeftX="80" topLeftY="80" width="300" height="180"/>', "<img>"),
        ]
        for tag_name, element_xml, expected_hint in cases:
            with self.subTest(tag=tag_name):
                result = xml_lint.lint_xml(
                    f"""
                    <slide xmlns="https://www.larkoffice.com/sml/2.0">
                      <data>{element_xml}</data>
                    </slide>
                    """
                )
                slide_issues = result["slides"][0]["issues"]
                issue = next(
                    issue for issue in slide_issues if issue["code"] == "sxsd_unsupported_tag"
                )
                self.assertEqual(result["summary"]["error_count"], 1)
                self.assertEqual(
                    [reported["code"] for reported in slide_issues],
                    ["sxsd_unsupported_tag"],
                )
                self.assertEqual(issue["code"], "sxsd_unsupported_tag")
                self.assertEqual(issue["tag"], tag_name)
                self.assertIn(expected_hint, issue["hint"])

    def test_lint_xml_reports_sxsd_unsupported_attr_with_alias_hint(self) -> None:
        cases = [
            ("shape", "x", "topLeftX", '<shape type="text" x="80" topLeftY="80" width="300" height="60"><content><p>Text</p></content></shape>'),
            ("shape", "heigth", "height", '<shape type="text" topLeftX="80" topLeftY="80" width="300" heigth="60"><content><p>Text</p></content></shape>'),
            ("content", "fontColor", "color", '<shape type="text" topLeftX="80" topLeftY="80" width="300" height="60"><content fontColor="rgba(0, 0, 0, 1)"><p>Text</p></content></shape>'),
        ]
        for tag_name, attr_name, expected_attr, element_xml in cases:
            with self.subTest(attr=attr_name):
                result = xml_lint.lint_xml(
                    f"""
                    <slide xmlns="https://www.larkoffice.com/sml/2.0">
                      <data>{element_xml}</data>
                    </slide>
                    """
                )
                slide_issues = result["slides"][0]["issues"]
                issue = next(
                    issue for issue in slide_issues if issue["code"] == "sxsd_unsupported_attr"
                )
                self.assertEqual(result["summary"]["error_count"], 1)
                self.assertEqual(
                    [reported["code"] for reported in slide_issues],
                    ["sxsd_unsupported_attr"],
                )
                self.assertEqual(issue["code"], "sxsd_unsupported_attr")
                self.assertEqual(issue["tag"], tag_name)
                self.assertEqual(issue["attr"], attr_name)
                self.assertIn(expected_attr, issue["hint"])

    def test_lint_xml_keeps_unrelated_unsupported_and_missing_attrs(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape type="text" bogus="x" topLeftX="80" topLeftY="80" width="300"/>
              </data>
            </slide>
            """
        )

        self.assertEqual(
            [issue["code"] for issue in result["slides"][0]["issues"]],
            ["sxsd_unsupported_attr", "sxsd_missing_required_attr"],
        )

    def test_lint_xml_keeps_missing_attrs_when_suggestion_is_ambiguous(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape type="text" topLeft="80" width="300" height="60"/>
              </data>
            </slide>
            """
        )

        self.assertEqual(
            [issue["code"] for issue in result["slides"][0]["issues"]],
            [
                "sxsd_unsupported_attr",
                "sxsd_missing_required_attr",
                "sxsd_missing_required_attr",
            ],
        )

    def test_lint_xml_suppresses_only_missing_attr_that_resolves_ambiguous_suggestion(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape type="text" topLeftXX="80" topLeftY="80" width="300" height="60"/>
              </data>
            </slide>
            """
        )

        self.assertEqual(
            [
                (issue["code"], issue.get("attr"))
                for issue in result["slides"][0]["issues"]
            ],
            [("sxsd_unsupported_attr", "topLeftXX")],
        )

    def test_lint_xml_ignores_server_filled_id_attrs(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide id="server-slide-id" xmlns="https://www.larkoffice.com/sml/2.0">
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
        self.assertEqual(
            [(issue["tag"], issue["attr"]) for issue in result["slides"][0]["issues"]],
            [("fill", "unexpected")],
        )

    def test_lint_xml_ignores_chart_roundtrip_attrs(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <chart updated="true" topLeftX="80" topLeftY="80" width="300" height="160">
                  <chartPlotArea><chartPlot type="line"/></chartPlotArea>
                  <chartData isStaticData="true">
                    <dim1><chartField name="category" valueType="string">A</chartField></dim1>
                    <dim2><chartField name="value" valueType="number">1</chartField></dim2>
                  </chartData>
                </chart>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["summary"]["error_count"], 0)

    def test_lint_xml_ignores_chart_parsed_values_roundtrip_tags(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <chart topLeftX="80" topLeftY="80" width="300" height="160">
                  <chartPlotArea><chartPlot type="line"/></chartPlotArea>
                  <chartData isStaticData="true">
                    <dim1>
                      <chartField name="category" valueType="string">
                        A<chartParsedValues>A</chartParsedValues>
                      </chartField>
                    </dim1>
                    <dim2><chartField name="value" valueType="number">1</chartField></dim2>
                  </chartData>
                </chart>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["summary"]["error_count"], 0)

    def test_lint_xml_ignores_chart_parsed_values_roundtrip_tag(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <chart topLeftX="80" topLeftY="80" width="300" height="160">
                  <chartPlotArea><chartPlot type="line"/></chartPlotArea>
                  <chartData>
                    <dim1>
                      <chartField name="category" valueType="string">
                        Africa<chartParsedValues>Africa</chartParsedValues>
                      </chartField>
                    </dim1>
                    <dim2><chartField name="value" valueType="number">1</chartField></dim2>
                  </chartData>
                </chart>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["summary"]["error_count"], 0)
        self.assertNotIn("issues", result)

    def test_lint_xml_rejects_chart_parsed_values_outside_chart_field(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape type="rect" topLeftX="80" topLeftY="80" width="300" height="160">
                  <chartParsedValues>unexpected</chartParsedValues>
                </shape>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertEqual(
            [issue["code"] for issue in result["slides"][0]["issues"]],
            ["sxsd_unsupported_tag"],
        )
        self.assertEqual(
            result["slides"][0]["issues"][0]["path"],
            "slide/data/shape/chartParsedValues",
        )

    def test_lint_xml_limits_chart_roundtrip_attrs_to_matching_tags(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <chart isStaticData="true" topLeftX="80" topLeftY="80" width="300" height="160">
                  <chartPlotArea><chartPlot type="line"/></chartPlotArea>
                  <chartData updated="true">
                    <dim1><chartField name="category" valueType="string">A</chartField></dim1>
                    <dim2><chartField name="value" valueType="number">1</chartField></dim2>
                  </chartData>
                </chart>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["summary"]["error_count"], 2)
        slide_issues = result["slides"][0]["issues"]
        self.assertEqual(
            {(issue["tag"], issue["attr"]) for issue in slide_issues},
            {("chart", "isStaticData"), ("chartData", "updated")},
        )
        self.assertTrue(all(issue["code"] == "sxsd_unsupported_attr" for issue in slide_issues))

    def test_lint_xml_reports_gradient_shorthand_attrs_on_fill_color(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        slide_issues = result["slides"][0]["issues"]
        unsupported_attrs = {issue["attr"] for issue in slide_issues}
        self.assertEqual(result["summary"]["error_count"], 6)
        self.assertEqual(
            unsupported_attrs,
            {"type", "color1", "color2", "angle", "stop1", "stop2"},
        )
        self.assertTrue(all(issue["code"] == "sxsd_unsupported_attr" for issue in slide_issues))
        self.assertTrue(all(issue["tag"] == "fillColor" for issue in slide_issues))

    def test_lint_xml_accepts_chart_field_simple_content_attrs(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        original_loader = xml_lint.load_iconpark_icon_types

        def fail_if_loaded() -> set[str]:
            raise AssertionError("iconpark index should not be loaded without <icon iconType>")

        xml_lint.load_iconpark_icon_types = fail_if_loaded
        try:
            result = xml_lint.lint_xml(
                """
                <slide xmlns="https://www.larkoffice.com/sml/2.0">
                  <data>
                    <shape type="text" topLeftX="80" topLeftY="80" width="300" height="60">
                      <content><p>No icons here</p></content>
                    </shape>
                  </data>
                </slide>
                """
            )
        finally:
            xml_lint.load_iconpark_icon_types = original_loader

        self.assertEqual(result["summary"]["error_count"], 0)
        self.assertNotIn("issues", result)

    def test_lint_xml_accepts_iconpark_icon_type_from_index(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
                result = xml_lint.lint_xml(
                    f"""
                    <slide xmlns="https://www.larkoffice.com/sml/2.0">
                      <data>{icon_xml}</data>
                    </slide>
                    """
                )

                issue = result["issues"][0]
                self.assertEqual(result["summary"]["error_count"], 1)
                self.assertEqual(issue["code"], "icon_missing_fill_color")
                self.assertEqual(issue["tag"], "icon")

    def test_lint_xml_reports_icon_transparent_fill_color(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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

    def test_lint_xml_skips_iconpark_validation_inside_embedded_svg(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <embed topLeftX="80" topLeftY="120" width="240" height="140">
                  <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 240 140">
                    <rect x="10" y="10" width="220" height="120" fill="#EFF6FF"/>
                    <icon iconType="not-an-iconpark-name"/>
                    <foreignObject x="0" y="0" width="60" height="60">
                      <icon xmlns="http://www.w3.org/1999/xhtml" iconType="not-an-iconpark-name"/>
                    </foreignObject>
                  </svg>
                </embed>
              </data>
            </slide>
            """
        )
        codes = [issue["code"] for issue in result["document"]["errors"]]
        self.assertNotIn("iconpark_unsupported_icon_type", codes)
        self.assertNotIn("icon_missing_fill_color", codes)
        self.assertEqual(result["summary"]["error_count"], 0)

    def test_lint_xml_detects_overlapping_text_boxes(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        result = xml_lint.lint_xml(
            """
            <slide id="pQO" xmlns="https://www.larkoffice.com/sml/2.0">
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
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertEqual(result["summary"]["warning_count"], 0)
        self.assertEqual(result["slides"][0]["issues"][0]["code"], "text_may_overflow_shape")
        self.assertEqual(result["slides"][0]["issues"][0]["level"], "error")
        self.assertEqual(result["slides"][0]["issues"][0]["elements"], ["source"])

    def test_lint_xml_reports_text_out_of_canvas_and_warns_for_text_height(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        self.assertEqual(result["summary"]["error_count"], 2)
        self.assertEqual(result["summary"]["warning_count"], 0)
        self.assertEqual(issue["code"], "shape_out_of_canvas")
        self.assertEqual(issue["overflow"], {"left": 0, "top": 0, "right": 160, "bottom": 40})

    def test_lint_xml_warns_when_text_may_overflow_its_own_shape(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="overflowing" type="text" topLeftX="80" topLeftY="80" width="360" height="80">
                  <content fontSize="20" lineSpacing="multiple:1.5" autoFit="no-auto-fit">
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
                <shape id="shape-auto-fit" type="text" topLeftX="480" topLeftY="240" width="360" height="30">
                  <content fontSize="20" lineSpacing="multiple:1.5" autoFit="shape-auto-fit">
                    <p>第一段</p><p>第二段</p><p>第三段</p>
                  </content>
                </shape>
              </data>
            </slide>
            """
        )
        issues = result["slides"][0]["issues"]
        overflow_issues = [issue for issue in issues if issue["code"] == "text_may_overflow_shape"]
        self.assertEqual(result["summary"]["error_count"], 1)
        overflow_ids = {issue["elements"][0] for issue in overflow_issues}
        self.assertIn("overflowing", overflow_ids)
        self.assertNotIn("auto-fit", overflow_ids)
        self.assertNotIn("shape-auto-fit", overflow_ids)
        self.assertNotIn("fitting", overflow_ids)
        overflowing_issue = next(issue for issue in overflow_issues if issue["elements"] == ["overflowing"])
        self.assertEqual(overflowing_issue["line_count"], 4)
        self.assertEqual(overflowing_issue["estimated_height"], 110)
        self.assertEqual(overflowing_issue["available_height"], 80)
        self.assertEqual(overflowing_issue["overflow"], 30)
        self.assertIn('wrap="true" autoFit="normal-auto-fit"', overflowing_issue["message"])

    def test_lint_xml_uses_fixed_line_spacing_for_text_height_warning(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="fixed-overflow" type="text" topLeftX="80" topLeftY="80" width="360" height="50">
                  <content fontSize="20" lineSpacing="fixed:20" autoFit="no-auto-fit">
                    <p>第一段</p><p>第二段</p><p>第三段</p>
                  </content>
                </shape>
              </data>
            </slide>
            """
        )
        issue = result["slides"][0]["issues"][0]
        self.assertEqual(result["summary"]["warning_count"], 1)
        self.assertEqual(result["summary"]["error_count"], 0)
        self.assertEqual(issue["level"], "warning")
        self.assertEqual(issue["line_height"], 20)
        self.assertEqual(issue["estimated_height"], 60)
        self.assertEqual(issue["overflow"], 10)

    def test_lint_xml_ignores_subpixel_text_height_overflow_tolerance(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="minor-overflow" type="text" topLeftX="80" topLeftY="80" width="360" height="39.8">
                  <content fontSize="20" lineSpacing="fixed:20" autoFit="no-auto-fit">
                    <p>第一段</p><p>第二段</p>
                  </content>
                </shape>
              </data>
            </slide>
            """
        )
        overflow_issues = [
            issue
            for issue in result["slides"][0]["issues"]
            if issue["code"] == "text_may_overflow_shape"
        ]
        self.assertEqual(overflow_issues, [])

    def test_lint_xml_allows_single_line_width_estimation_jitter(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="metric" type="text" topLeftX="80" topLeftY="80" width="152" height="54">
                  <content fontSize="36" lineSpacing="multiple:1.2" autoFit="no-auto-fit"><p>4.16万亿</p></content>
                </shape>
              </data>
            </slide>
            """
        )
        overflow_issues = [
            issue
            for issue in result["slides"][0]["issues"]
            if issue["code"] == "text_may_overflow_shape"
        ]
        self.assertEqual(overflow_issues, [])

    def test_lint_xml_allows_short_metric_text_with_separators_as_single_line(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="metric" type="text" topLeftX="80" topLeftY="80" width="150" height="50">
                  <content textType="title" fontSize="36" autoFit="no-auto-fit"><p>4.16万亿</p></content>
                </shape>
                <shape id="table-number" type="text" topLeftX="80" topLeftY="160" width="25" height="20">
                  <content fontSize="10" textAlign="center" autoFit="no-auto-fit"><p>1,380</p></content>
                </shape>
              </data>
            </slide>
            """
        )
        overflow_issues = [
            issue
            for issue in result["slides"][0]["issues"]
            if issue["code"] == "text_may_overflow_shape"
        ]
        self.assertEqual(overflow_issues, [])

    def test_lint_xml_reports_plain_short_metric_when_it_wraps(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="plain-age" type="text" topLeftX="80" topLeftY="80" width="50" height="80">
                  <content textType="title" fontSize="36" bold="true" autoFit="no-auto-fit"><p>82岁</p></content>
                </shape>
              </data>
            </slide>
            """
        )
        overflow_issues = [
            issue
            for issue in result["slides"][0]["issues"]
            if issue["code"] == "text_may_overflow_shape"
        ]
        self.assertEqual(len(overflow_issues), 1)
        self.assertEqual(overflow_issues[0]["elements"], ["plain-age"])

    def test_lint_xml_allows_centered_short_label_near_fit_as_single_line(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="centered-label" type="text" topLeftX="80" topLeftY="80" width="200" height="30">
                  <content fontSize="14" bold="true" textAlign="center" autoFit="no-auto-fit">
                    <p>参数服务器 (Parameter Server)</p>
                  </content>
                </shape>
              </data>
            </slide>
            """
        )
        overflow_issues = [
            issue
            for issue in result["slides"][0]["issues"]
            if issue["code"] == "text_may_overflow_shape"
        ]
        self.assertEqual(overflow_issues, [])

    def test_lint_xml_allows_headline_near_fit_as_single_line(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="headline" type="text" topLeftX="80" topLeftY="80" width="700" height="50">
                  <content textType="headline" fontSize="26" bold="true" lineSpacing="multiple:1.3" autoFit="no-auto-fit">
                    <p>全球半导体市场规模持续高速增长，AI驱动新一轮景气周期</p>
                  </content>
                </shape>
              </data>
            </slide>
            """
        )
        overflow_issues = [
            issue
            for issue in result["slides"][0]["issues"]
            if issue["code"] == "text_may_overflow_shape"
        ]
        self.assertEqual(overflow_issues, [])

    def test_lint_xml_allows_dense_body_line_spacing_estimation_slack(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="dense-body" type="text" topLeftX="80" topLeftY="80" width="360" height="140">
                  <content fontSize="13" bold="true" lineSpacing="multiple:1.7" autoFit="no-auto-fit">
                    <p>总体目标：</p>
                    <p>建立深度神经网络高效训练的统一理论框架，实现训练效率与模型性能的协同优化。</p>
                    <p>具体目标：</p>
                    <p>提出自适应优化算法，收敛速度提升 2-3 倍</p>
                    <p>实现结构化压缩方法，模型体积减少 10 倍以上</p>
                    <p>构建分布式训练策略，64 GPU 加速比 &gt; 50x</p>
                  </content>
                </shape>
              </data>
            </slide>
            """
        )
        overflow_issues = [
            issue
            for issue in result["slides"][0]["issues"]
            if issue["code"] == "text_may_overflow_shape"
        ]
        self.assertEqual(overflow_issues, [])

    def test_lint_xml_reports_dense_body_when_adjusted_height_still_overflows(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="dense-body" type="text" topLeftX="80" topLeftY="80" width="360" height="100">
                  <content fontSize="13" bold="true" lineSpacing="multiple:1.7" autoFit="no-auto-fit">
                    <p>总体目标：</p>
                    <p>建立深度神经网络高效训练的统一理论框架，实现训练效率与模型性能的协同优化。</p>
                    <p>具体目标：</p>
                    <p>提出自适应优化算法，收敛速度提升 2-3 倍</p>
                    <p>实现结构化压缩方法，模型体积减少 10 倍以上</p>
                    <p>构建分布式训练策略，64 GPU 加速比 &gt; 50x</p>
                  </content>
                </shape>
              </data>
            </slide>
            """
        )
        overflow_issues = [
            issue
            for issue in result["slides"][0]["issues"]
            if issue["code"] == "text_may_overflow_shape"
        ]
        self.assertEqual(len(overflow_issues), 1)
        self.assertEqual(overflow_issues[0]["elements"], ["dense-body"])

    def test_lint_xml_reports_letter_spaced_caption_near_fit(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="caption" type="text" topLeftX="80" topLeftY="80" width="120" height="20">
                  <content textType="caption" fontSize="11" letterSpacing="1" autoFit="no-auto-fit">
                    <p>RISKS &amp; CHALLENGES</p>
                  </content>
                </shape>
              </data>
            </slide>
            """
        )
        overflow_issues = [
            issue
            for issue in result["slides"][0]["issues"]
            if issue["code"] == "text_may_overflow_shape"
        ]
        self.assertEqual(len(overflow_issues), 1)
        self.assertEqual(overflow_issues[0]["elements"], ["caption"])

    def test_lint_xml_reports_micro_caption_when_wrapping_overflows(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="micro-caption" type="text" topLeftX="80" topLeftY="60" width="200" height="16">
                  <content textType="caption" fontSize="3" lineSpacing="multiple:1.3" letterSpacing="160" autoFit="no-auto-fit">
                    <p>MARKET INSIGHT · 市场洞察</p>
                  </content>
                </shape>
              </data>
            </slide>
            """
        )
        overflow_issues = [
            issue
            for issue in result["slides"][0]["issues"]
            if issue["code"] == "text_may_overflow_shape"
        ]
        self.assertEqual(len(overflow_issues), 1)
        self.assertEqual(overflow_issues[0]["elements"], ["micro-caption"])

    def test_lint_xml_text_may_overflow_shape_upgrades_to_error_above_threshold(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="just-warning" type="text" topLeftX="80" topLeftY="80" width="360" height="50">
                  <content fontSize="20" lineSpacing="fixed:20" autoFit="no-auto-fit">
                    <p>第一段</p><p>第二段</p><p>第三段</p>
                  </content>
                </shape>
                <shape id="error-overflow" type="text" topLeftX="80" topLeftY="200" width="360" height="30">
                  <content fontSize="20" lineSpacing="fixed:20" autoFit="no-auto-fit">
                    <p>第一段</p><p>第二段</p><p>第三段</p>
                  </content>
                </shape>
              </data>
            </slide>
            """
        )
        issues = {issue["elements"][0]: issue for issue in result["slides"][0]["issues"]}
        self.assertEqual(issues["just-warning"]["level"], "warning")
        self.assertEqual(issues["just-warning"]["overflow"], 10)
        self.assertEqual(issues["error-overflow"]["level"], "error")
        self.assertEqual(issues["error-overflow"]["overflow"], 30)
        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertEqual(result["summary"]["warning_count"], 1)

    def test_lint_xml_text_may_overflow_shape_downgrades_background_decoration_to_info(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="bg-deco" type="text" topLeftX="0" topLeftY="0" width="600" height="80" alpha="0.3">
                  <content fontSize="120" lineSpacing="fixed:120" autoFit="no-auto-fit"><p>2026</p></content>
                </shape>
                <shape id="foreground" type="text" topLeftX="40" topLeftY="20" width="400" height="60">
                  <content fontSize="20" lineSpacing="fixed:24"><p>Annual Report</p></content>
                </shape>
              </data>
            </slide>
            """
        )
        issues = {
            issue["elements"][0]: issue
            for issue in result["slides"][0]["issues"]
            if issue["code"] == "text_may_overflow_shape"
        }
        self.assertEqual(issues["bg-deco"]["level"], "info")
        self.assertEqual(result["summary"]["warning_count"], 0)
        self.assertEqual(result["summary"]["info_count"], 1)
        self.assertEqual(result["slides"][0]["infos"], [issues["bg-deco"]])
        self.assertIn("background decoration", issues["bg-deco"]["message"])

    def test_lint_xml_reports_shape_alpha_ghost_text_out_of_canvas_but_allows_overlap(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="ghost-number" type="text" topLeftX="-60" topLeftY="30" width="360" height="180" alpha="0.2">
                  <content fontSize="160" lineSpacing="fixed:160" wrap="false"><p>01</p></content>
                </shape>
                <shape id="title" type="text" topLeftX="80" topLeftY="80" width="360" height="80">
                  <content fontSize="30" lineSpacing="fixed:36"><p>Annual Review</p></content>
                </shape>
              </data>
            </slide>
            """
        )
        codes = [issue["code"] for issue in result["slides"][0]["issues"]]
        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertIn("shape_out_of_canvas", codes)
        self.assertNotIn("bbox_overlap", codes)

    def test_lint_xml_reports_content_color_alpha_ghost_text_out_of_canvas_but_allows_overlap(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="ghost-year" type="text" topLeftX="760" topLeftY="20" width="260" height="160">
                  <content fontSize="140" color="rgba(0,0,0,0.2)" lineSpacing="fixed:140" wrap="false"><p>2026</p></content>
                </shape>
                <shape id="headline" type="text" topLeftX="700" topLeftY="70" width="220" height="80">
                  <content fontSize="28" lineSpacing="fixed:34"><p>Forecast</p></content>
                </shape>
              </data>
            </slide>
            """
        )
        codes = [issue["code"] for issue in result["slides"][0]["issues"]]
        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertIn("shape_out_of_canvas", codes)
        self.assertNotIn("bbox_overlap", codes)

    def test_lint_xml_reports_faint_medium_ghost_text_out_of_canvas_but_allows_overlap(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="medium-ghost" type="text" topLeftX="820" topLeftY="300" width="270" height="72" alpha="0.32">
                  <content fontSize="40" lineSpacing="fixed:40" wrap="false"><p>OFF EDGE</p></content>
                </shape>
                <shape id="caption" type="text" topLeftX="760" topLeftY="315" width="180" height="36">
                  <content fontSize="16" lineSpacing="fixed:20"><p>Readable caption</p></content>
                </shape>
              </data>
            </slide>
            """
        )
        codes = [issue["code"] for issue in result["slides"][0]["issues"]]
        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertIn("shape_out_of_canvas", codes)
        self.assertNotIn("bbox_overlap", codes)

    def test_lint_xml_allows_ghost_text_image_overlap(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="ghost-label" type="text" topLeftX="100" topLeftY="40" width="560" height="160" alpha="0.2">
                  <content fontSize="120" lineSpacing="fixed:120" wrap="false"><p>2026</p></content>
                </shape>
                <img id="photo" src="token" topLeftX="160" topLeftY="70" width="260" height="160"/>
                <shape id="title" type="text" topLeftX="610" topLeftY="95" width="320" height="60">
                  <content fontSize="28" lineSpacing="fixed:34"><p>Annual Review</p></content>
                </shape>
              </data>
            </slide>
            """
        )
        codes = [issue["code"] for issue in result["slides"][0]["issues"]]
        self.assertNotIn("image_covers_text", codes)
        self.assertNotIn("bbox_overlap", codes)

    def test_lint_slide_allows_ghost_text_whiteboard_overlap(self) -> None:
        result = xml_lint.lint_slide(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <whiteboard id="board" topLeftX="180" topLeftY="70" width="420" height="300"/>
                <shape id="ghost-label" type="text" topLeftX="100" topLeftY="40" width="560" height="160" alpha="0.2">
                  <content fontSize="120" lineSpacing="fixed:120" wrap="false"><p>2026</p></content>
                </shape>
                <shape id="title" type="text" topLeftX="610" topLeftY="95" width="220" height="60">
                  <content fontSize="28" lineSpacing="fixed:34"><p>Annual Review</p></content>
                </shape>
              </data>
            </slide>
            """,
            1,
        )
        codes = [issue["code"] for issue in result["issues"]]
        self.assertNotIn("whiteboard_external_overlap", codes)

    def test_lint_xml_reports_faint_ghost_text_out_of_canvas_without_area_threshold(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="small-ghost" type="text" topLeftX="940" topLeftY="300" width="40" height="40" alpha="0.32">
                  <content fontSize="36" lineSpacing="fixed:36" wrap="false"><p>土</p></content>
                </shape>
              </data>
            </slide>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertEqual(result["slides"][0]["issues"][0]["code"], "shape_out_of_canvas")

    def test_lint_xml_keeps_out_of_canvas_error_for_medium_text_without_faint_alpha(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="medium-not-ghost" type="text" topLeftX="820" topLeftY="300" width="270" height="72" alpha="0.36">
                  <content fontSize="54" lineSpacing="fixed:54" wrap="false"><p>OFF EDGE</p></content>
                </shape>
              </data>
            </slide>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertEqual(result["slides"][0]["issues"][0]["code"], "shape_out_of_canvas")
        self.assertEqual(result["slides"][0]["issues"][0]["elements"], ["medium-not-ghost"])

    def test_lint_xml_keeps_out_of_canvas_error_for_half_alpha_large_text(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="half-alpha" type="text" topLeftX="760" topLeftY="20" width="260" height="160">
                  <content fontSize="140" color="rgba(0,0,0,0.5)" lineSpacing="fixed:140" wrap="false"><p>2026</p></content>
                </shape>
              </data>
            </slide>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertEqual(result["slides"][0]["issues"][0]["code"], "shape_out_of_canvas")
        self.assertEqual(result["slides"][0]["issues"][0]["elements"], ["half-alpha"])

    def test_lint_xml_text_may_overflow_shape_keeps_error_when_alpha_not_low(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="opaque-big" type="text" topLeftX="0" topLeftY="0" width="600" height="80" alpha="0.9">
                  <content fontSize="120" lineSpacing="fixed:120" autoFit="no-auto-fit"><p>2026</p></content>
                </shape>
                <shape id="foreground" type="text" topLeftX="40" topLeftY="20" width="400" height="60">
                  <content fontSize="20" lineSpacing="fixed:24"><p>Annual Report</p></content>
                </shape>
              </data>
            </slide>
            """
        )
        issue = next(
            issue
            for issue in result["slides"][0]["issues"]
            if issue["code"] == "text_may_overflow_shape" and issue["elements"] == ["opaque-big"]
        )
        self.assertEqual(issue["level"], "error")

    def test_lint_xml_text_may_overflow_shape_keeps_error_when_no_foreground_text(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="lonely-big" type="text" topLeftX="0" topLeftY="0" width="600" height="80" alpha="0.3">
                  <content fontSize="120" lineSpacing="fixed:120" autoFit="no-auto-fit"><p>2026</p></content>
                </shape>
              </data>
            </slide>
            """
        )
        issue = next(
            issue
            for issue in result["slides"][0]["issues"]
            if issue["code"] == "text_may_overflow_shape" and issue["elements"] == ["lonely-big"]
        )
        self.assertEqual(issue["level"], "error")

    def test_lint_xml_text_may_overflow_shape_keeps_error_when_foreground_alpha_zero(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="bg-deco" type="text" topLeftX="0" topLeftY="0" width="600" height="80" alpha="0.3">
                  <content fontSize="120" lineSpacing="fixed:120" autoFit="no-auto-fit"><p>2026</p></content>
                </shape>
                <shape id="transparent-foreground" type="text" topLeftX="40" topLeftY="20" width="400" height="60" alpha="0">
                  <content fontSize="20" lineSpacing="fixed:24"><p>Annual Report</p></content>
                </shape>
              </data>
            </slide>
            """
        )
        issue = next(
            issue
            for issue in result["slides"][0]["issues"]
            if issue["code"] == "text_may_overflow_shape" and issue["elements"] == ["bg-deco"]
        )
        self.assertEqual(issue["level"], "error")

    def test_lint_xml_text_may_overflow_shape_keeps_error_when_foreground_is_below_in_order(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="foreground" type="text" topLeftX="40" topLeftY="20" width="400" height="60">
                  <content fontSize="20" lineSpacing="fixed:24"><p>Annual Report</p></content>
                </shape>
                <shape id="top-big" type="text" topLeftX="0" topLeftY="0" width="600" height="80" alpha="0.3">
                  <content fontSize="120" lineSpacing="fixed:120" autoFit="no-auto-fit"><p>2026</p></content>
                </shape>
              </data>
            </slide>
            """
        )
        issue = next(
            issue
            for issue in result["slides"][0]["issues"]
            if issue["code"] == "text_may_overflow_shape" and issue["elements"] == ["top-big"]
        )
        self.assertEqual(issue["level"], "error")

    def test_lint_xml_uses_paragraph_spacing_overrides_for_text_height_warning(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="paragraph-overflow" type="text" topLeftX="80" topLeftY="80" width="360" height="30">
                  <content fontSize="20" lineSpacing="multiple:1.5" autoFit="no-auto-fit">
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
        self.assertEqual(issues[0]["overflow"], 10)

    def test_lint_xml_uses_letter_spacing_for_text_overflow_warning(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="baseline" type="text" topLeftX="0" topLeftY="0" width="120" height="30">
                  <content fontSize="20" lineSpacing="multiple:1.5" autoFit="no-auto-fit"><p>一二三四五六</p></content>
                </shape>
                <shape id="content-spaced" type="text" topLeftX="200" topLeftY="0" width="120" height="30">
                  <content fontSize="20" lineSpacing="multiple:1.5" letterSpacing="2" autoFit="no-auto-fit"><p>一二三四五六</p></content>
                </shape>
                <shape id="paragraph-spaced" type="text" topLeftX="400" topLeftY="0" width="120" height="30">
                  <content fontSize="20" lineSpacing="multiple:1.5" autoFit="no-auto-fit"><p letterSpacing="2">一二三四五六</p></content>
                </shape>
              </data>
            </slide>
            """
        )
        issues = result["slides"][0]["issues"]
        overflow_ids = [issue["elements"][0] for issue in issues if issue["code"] == "text_may_overflow_shape"]
        self.assertNotIn("baseline", overflow_ids)
        self.assertIn("content-spaced", overflow_ids)
        self.assertIn("paragraph-spaced", overflow_ids)
        by_id = {issue["elements"][0]: issue for issue in issues if issue["code"] == "text_may_overflow_shape"}
        self.assertEqual(by_id["content-spaced"]["line_count"], 2)
        self.assertEqual(by_id["content-spaced"]["estimated_height"], 50)
        self.assertEqual(by_id["content-spaced"]["overflow"], 20)
        self.assertEqual(by_id["paragraph-spaced"]["line_count"], 2)

    def test_strip_xml_paragraphs_preserves_br_as_hard_line_break(self) -> None:
        self.assertEqual(
            xml_lint.strip_xml_paragraphs("<p>第一行<br/>第二行<br />第三行</p>"),
            "第一行\n第二行\n第三行",
        )

    def test_lint_xml_allows_template_style_images_outside_canvas(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        elements = xml_lint.extract_elements(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
                <embed id="emb" topLeftX="600" topLeftY="320" width="240" height="140">
                  <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 240 140"><rect x="0" y="0" width="240" height="140"/></svg>
                </embed>
                <shape id="missing-height" type="text" topLeftX="80" topLeftY="480" width="320">
                  <content><p>Skipped</p></content>
                </shape>
              </data>
            </slide>
            """
        )
        self.assertEqual([element["id"] for element in elements], ["photo", "headline", "table", "chart", "wb", "emb"])
        self.assertEqual([element["kind"] for element in elements], ["img", "shape", "table", "chart", "whiteboard", "embed"])
        self.assertEqual([element["order"] for element in elements], [0, 1, 2, 3, 4, 5])
        self.assertEqual(elements[1]["type"], "text")
        self.assertEqual(elements[1]["textType"], "headline")
        self.assertEqual(elements[1]["textAlign"], "center")
        self.assertEqual(elements[1]["autoFit"], "normal-auto-fit")
        self.assertEqual(elements[1]["fontSize"], 28)
        self.assertEqual(elements[1]["text"], "Growth & scale\nFocused execution")

    def test_lint_xml_ignores_small_out_of_bounds_images(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="https://www.larkoffice.com/sml/2.0">
                <data>
                  <img src="tok" topLeftX="-20" topLeftY="20" width="120" height="120"/>
                </data>
              </slide>
            </presentation>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 0)

    def test_lint_xml_ignores_out_of_canvas_images(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="https://www.larkoffice.com/sml/2.0">
                <data>
                  <img src="right" topLeftX="780" topLeftY="0" width="500" height="540"/>
                  <img src="bottom" topLeftX="0" topLeftY="430" width="900" height="280"/>
                </data>
              </slide>
            </presentation>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 0)

    def test_lint_xml_ignores_full_bleed_images_outside_canvas(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="https://www.larkoffice.com/sml/2.0">
                <data>
                  <img src="tok" topLeftX="-80" topLeftY="-20" width="1080" height="600"/>
                </data>
              </slide>
            </presentation>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 0)

    def test_lint_xml_reports_text_and_chart_but_not_image_out_of_canvas(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="https://www.larkoffice.com/sml/2.0">
                <data>
                  <shape id="outside-shape" type="text" topLeftX="-10" topLeftY="40" width="50" height="50"/>
                  <img id="outside-img" src="token" topLeftX="120" topLeftY="-20" width="50" height="50"/>
                  <chart id="outside-chart" topLeftX="900" topLeftY="100" width="100" height="100">
                    <chartPlotArea><chartPlot type="line"/></chartPlotArea>
                    <chartData>
                      <dim1><chartField name="category" valueType="string">A</chartField></dim1>
                      <dim2><chartField name="value" valueType="number">1</chartField></dim2>
                    </chartData>
                  </chart>
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

    def test_lint_xml_ignores_line_out_of_canvas(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="body" type="text" topLeftX="80" topLeftY="80" width="300" height="60">
                  <content fontSize="18"><p>Visible content</p></content>
                </shape>
                <line id="connector" startX="80" startY="120" endX="980" endY="120"><border/></line>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["summary"]["error_count"], 0)
        self.assertEqual(result["slides"][0]["issues"], [])

    def test_lint_xml_reports_horizontal_line_crossing_headline_glyphs(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="title" type="text" topLeftX="80" topLeftY="200" width="500" height="90">
                  <content fontSize="60"><p>测试文字 ABC</p></content>
                </shape>
                <line id="strike" startX="80" startY="245" endX="560" endY="245">
                  <border color="rgb(255, 0, 0)" width="4"/>
                </line>
              </data>
            </slide>
            """
        )
        crossing = [
            issue for issue in result["slides"][0]["errors"] if set(issue["elements"]) == {"strike", "title"}
        ]
        self.assertEqual(len(crossing), 1)
        self.assertEqual(crossing[0]["code"], "bbox_overlap")

    def test_lint_xml_reports_vertical_line_crossing_multiline_text(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="col" type="text" topLeftX="700" topLeftY="180" width="240" height="180">
                  <content fontSize="20"><p>第一行文字内容</p><p>第二行文字内容</p><p>第三行文字内容</p></content>
                </shape>
                <line id="vbar" startX="740" startY="170" endX="740" endY="360">
                  <border color="rgb(0, 0, 255)" width="3"/>
                </line>
              </data>
            </slide>
            """
        )
        crossing = [
            issue for issue in result["slides"][0]["errors"] if set(issue["elements"]) == {"vbar", "col"}
        ]
        self.assertEqual(len(crossing), 1)

    def test_lint_xml_reports_diagonal_line_crossing_text_block(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="para" type="text" topLeftX="80" topLeftY="400" width="420" height="140">
                  <content fontSize="18"><p>这是一段测试文字用于验证线条穿过</p></content>
                </shape>
                <line id="diag" startX="80" startY="410" endX="500" endY="530">
                  <border color="rgb(255, 0, 0)" width="3"/>
                </line>
              </data>
            </slide>
            """
        )
        crossing = [
            issue for issue in result["slides"][0]["errors"] if set(issue["elements"]) == {"diag", "para"}
        ]
        self.assertEqual(len(crossing), 1)

    def test_lint_xml_ignores_diagonal_line_whose_bbox_but_not_segment_crosses_text(self) -> None:
        # The diagonal's axis-aligned bounding box overlaps the text, but the segment itself passes
        # through empty space in the opposite corner -- a naive bbox test would false-positive here.
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="corner-text" type="text" topLeftX="80" topLeftY="80" width="120" height="40">
                  <content fontSize="18"><p>corner</p></content>
                </shape>
                <line id="far-diag" startX="700" startY="80" endX="90" endY="500">
                  <border color="rgb(255, 0, 0)" width="3"/>
                </line>
              </data>
            </slide>
            """
        )
        crossing = [
            issue for issue in result["slides"][0]["errors"] if set(issue["elements"]) == {"far-diag", "corner-text"}
        ]
        self.assertEqual(crossing, [])

    def test_lint_xml_ignores_line_touching_text_frame_but_not_glyphs(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="lbl" type="text" topLeftX="80" topLeftY="80" width="300" height="200">
                  <content fontSize="18" verticalAlign="top"><p>短标签</p></content>
                </shape>
                <line id="below" startX="80" startY="270" endX="380" endY="270">
                  <border color="rgb(255, 0, 0)" width="2"/>
                </line>
              </data>
            </slide>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 0)

    def test_lint_xml_ignores_invisible_line_crossing_text(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="title" type="text" topLeftX="80" topLeftY="200" width="500" height="90">
                  <content fontSize="60"><p>测试文字 ABC</p></content>
                </shape>
                <line id="ghost-line" startX="80" startY="245" endX="560" endY="245">
                  <border color="rgba(255, 0, 0, 0.03)" width="4"/>
                </line>
              </data>
            </slide>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 0)

    def test_lint_xml_ignores_vertical_line_grazing_text_left_edge(self) -> None:
        # Verbatim from deck GpGusGCwplQyK8dFN9LczmBXnwQ slide 4: a vertical line sitting on the text
        # frame's left edge renders before the first glyph, so it must not be flagged.
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape width="240" height="60" topLeftX="120" topLeftY="100" type="text" id="bmm">
                  <content fontSize="20" fontFamily="Arial" color="rgba(31, 35, 41, 1)" lineSpacing="fixed:24">
                    <p>Vertical edge graze</p>
                  </content>
                </shape>
                <line id="bmX" startX="120.00000000000001" startY="90" endX="120.00000000000001" endY="150.00833275470998">
                  <border color="rgba(0, 0, 0, 1)"/>
                </line>
              </data>
            </slide>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 0)

    def test_lint_xml_ignores_polyline_crossing_text(self) -> None:
        # Verbatim from deck GpGusGCwplQyK8dFN9LczmBXnwQ slide 6: the crossing check is scoped to
        # <line> only, so a <polyline> over text is not flagged.
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape width="240" height="60" topLeftX="120" topLeftY="100" type="text" id="bmr">
                  <content fontSize="20" fontFamily="Arial" color="rgba(31, 35, 41, 1)" lineSpacing="fixed:24">
                    <p>Polyline target</p>
                  </content>
                </shape>
                <polyline id="bmH" width="270" height="55" topLeftX="110" topLeftY="95">
                  <border color="rgba(0, 0, 0, 1)"/>
                </polyline>
              </data>
            </slide>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 0)

    def test_lint_xml_ignores_line_below_visual_glyph_height(self) -> None:
        # Verbatim from deck GpGusGCwplQyK8dFN9LczmBXnwQ slide 7: the shape frame is 80px tall but the
        # single 20px line of glyphs occupies only its top; a line at the frame's lower region grazes
        # under the visual glyph box (underline look) and must not be flagged.
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape width="240" height="80" topLeftX="120" topLeftY="100" type="text" id="bmB">
                  <content fontSize="20" fontFamily="Arial" color="rgba(31, 35, 41, 1)" lineSpacing="fixed:24">
                    <p>Visual height target</p>
                  </content>
                </shape>
                <line id="bmQ" startX="110" startY="150" endX="380.00185184550116" endY="150">
                  <border color="rgba(0, 0, 0, 1)"/>
                </line>
              </data>
            </slide>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 0)

    def test_lint_xml_uses_rotated_text_and_chart_bounds_for_canvas_validation(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="https://www.larkoffice.com/sml/2.0">
                <data>
                  <shape id="rotated-text" type="text" topLeftX="0" topLeftY="0" width="100" height="100" rotation="45"/>
                  <chart id="rotated-chart" topLeftX="860" topLeftY="200" width="100" height="100" rotation="45">
                    <chartPlotArea><chartPlot type="line"/></chartPlotArea>
                    <chartData>
                      <dim1><chartField name="category" valueType="string">A</chartField></dim1>
                      <dim2><chartField name="value" valueType="number">1</chartField></dim2>
                    </chartData>
                  </chart>
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

    def test_lint_xml_uses_declared_bounds_for_rect_and_ignores_images(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="https://www.larkoffice.com/sml/2.0">
                <data>
                  <shape id="rotated-rect" type="rect" topLeftX="900" topLeftY="0" width="100" height="100" rotation="45"/>
                  <img id="rotated-image" src="token" topLeftX="860" topLeftY="200" width="100" height="100" rotation="45"/>
                </data>
              </slide>
            </presentation>
            """
        )
        issues_by_element = {issue["elements"][0]: issue for issue in result["slides"][0]["issues"]}
        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertEqual(issues_by_element["rotated-rect"]["code"], "shape_out_of_canvas")
        self.assertEqual(issues_by_element["rotated-rect"]["overflow"], {"left": 0, "top": 0, "right": 40, "bottom": 0})
        self.assertNotIn("rotated-image", issues_by_element)

    def test_detect_elements_out_of_canvas_limits_detection_to_whitelist(self) -> None:
        issues = xml_lint.detect_elements_out_of_canvas(
            [
                {"id": "table", "kind": "table", "x": 95, "y": 0, "width": 10, "height": 10, "rotation": 45},
                {"id": "chart", "kind": "chart", "x": 95, "y": 0, "width": 10, "height": 10, "rotation": 0},
                {
                    "id": "text",
                    "kind": "shape",
                    "type": "text",
                    "x": 95,
                    "y": 0,
                    "width": 10,
                    "height": 10,
                    "rotation": 0,
                },
                {
                    "id": "rect",
                    "kind": "shape",
                    "type": "rect",
                    "x": 95,
                    "y": 0,
                    "width": 10,
                    "height": 10,
                    "rotation": 45,
                },
                {"id": "image", "kind": "img", "x": 95, "y": 0, "width": 10, "height": 10, "rotation": 0},
                {
                    "id": "ellipse",
                    "kind": "shape",
                    "type": "ellipse",
                    "x": 95,
                    "y": 0,
                    "width": 10,
                    "height": 10,
                    "rotation": 0,
                },
            ],
            100,
            100,
        )

        self.assertEqual([issue["elements"] for issue in issues], [["table"], ["chart"], ["text"], ["rect"]])
        self.assertEqual(issues[-1]["bbox"], {"x": 95, "y": 0, "width": 10, "height": 10})

    def test_lint_xml_rejects_non_finite_rotation_values_from_xsd(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="https://www.larkoffice.com/sml/2.0">
                <data>
                  <shape id="infinite" type="text" topLeftX="-10" topLeftY="0" width="20" height="20" rotation="inf"/>
                  <shape id="negative-infinite" type="text" topLeftX="0" topLeftY="-10" width="20" height="20" rotation="-inf"/>
                  <chart id="not-a-number" topLeftX="950" topLeftY="0" width="20" height="20" rotation="nan">
                    <chartPlotArea><chartPlot type="line"/></chartPlotArea>
                    <chartData>
                      <dim1><chartField name="category" valueType="string">A</chartField></dim1>
                      <dim2><chartField name="value" valueType="number">1</chartField></dim2>
                    </chartData>
                  </chart>
                </data>
              </slide>
            </presentation>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 3)
        slide_issues = result["slides"][0]["issues"]
        self.assertTrue(all(issue["code"] == "sxsd_invalid_scalar" for issue in slide_issues))
        self.assertEqual({issue["actual"] for issue in slide_issues}, {"inf", "-inf", "nan"})

    def test_lint_xml_reports_table_bottom_overflow_from_declared_bounds(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="https://www.larkoffice.com/sml/2.0">
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

    def test_lint_xml_xml_path_preserves_source_index_after_filtered_table(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <table id="t1" topLeftX="20" topLeftY="20">
                  <tr><td/></tr>
                </table>
                <table id="t2" topLeftX="20" topLeftY="100" width="9999" height="100">
                  <tr><td/></tr>
                </table>
              </data>
            </slide>
            """
        )

        issue = next(
            issue
            for issue in result["slides"][0]["issues"]
            if issue["code"] == "table_out_of_canvas"
        )
        self.assertEqual(issue["element_ids"], ["t2"])
        self.assertEqual(
            issue["related_objects"][0]["xml_path"],
            "slide[1]/data/table[2]",
        )

    def test_lint_xml_duplicate_id_keeps_issue_bound_to_original_shape(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="dup" type="rect" topLeftX="-20" topLeftY="40" width="50" height="50"/>
                <shape id="dup" type="rect" topLeftX="100" topLeftY="40" width="50" height="50"/>
              </data>
            </slide>
            """
        )

        canvas_issue = next(
            issue for issue in result["slides"][0]["issues"]
            if issue["code"] == "shape_out_of_canvas"
        )
        self.assertEqual(canvas_issue["element_ids"], ["dup"])
        self.assertEqual(
            canvas_issue["related_objects"],
            [
                {
                    "element_id": "dup",
                    "kind": "shape",
                    "type": "rect",
                    "bbox": {"x": -20, "y": 40, "width": 50, "height": 50},
                    "xml_path": "slide[1]/data/shape[1]",
                }
            ],
        )
        self.assertTrue(
            canvas_issue["hint"].startswith(
                "Locate via related_objects[].xml_path. "
            )
        )
        duplicate_issue = next(
            issue for issue in result["slides"][0]["issues"]
            if issue["code"] == "duplicate_element_id"
        )
        self.assertEqual(
            duplicate_issue["hint"],
            "Locate via related_objects[].xml_path. "
            "Do not invent replacement IDs. For newly authored elements, remove the id attribute. "
            "When updating read-back XML, keep the server ID on the original element only and remove it "
            "from copied or new elements.",
        )
        self.assertEqual(duplicate_issue["element_ids"], ["dup", "dup"])
        self.assertEqual(
            [obj["xml_path"] for obj in duplicate_issue["related_objects"]],
            ["slide[1]/data/shape[1]", "slide[1]/data/shape[2]"],
        )

    def test_lint_xml_blocks_duplicate_table_cell_ids(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <table id="table-1" topLeftX="80" topLeftY="80" width="800" height="120">
                  <colgroup><col width="400"/><col width="400"/></colgroup>
                  <tr height="120">
                    <td id="bjs"><content fontSize="24"><p>Original cell</p></content></td>
                    <td id="bjs"><content fontSize="24"><p>Copied cell</p></content></td>
                  </tr>
                </table>
              </data>
            </slide>
            """
        )

        issue = next(
            issue
            for issue in result["slides"][0]["issues"]
            if issue["code"] == "duplicate_element_id"
        )
        self.assertFalse(result["summary"]["release_ready"])
        self.assertEqual(issue["element_ids"], ["bjs", "bjs"])
        self.assertEqual(
            issue["related_objects"],
            [
                {
                    "element_id": "bjs",
                    "kind": "td",
                    "type": "td",
                    "xml_path": "slide[1]/data/table[1]/tr[1]/td[1]",
                },
                {
                    "element_id": "bjs",
                    "kind": "td",
                    "type": "td",
                    "xml_path": "slide[1]/data/table[1]/tr[1]/td[2]",
                },
            ],
        )

    def test_lint_xml_blocks_duplicate_id_shared_by_shape_and_table_cell(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="baa" type="rect" topLeftX="40" topLeftY="40" width="80" height="80"/>
                <table id="table-1" topLeftX="160" topLeftY="40" width="200" height="80">
                  <tr height="80"><td id="baa"><content fontSize="12"><p>Cell</p></content></td></tr>
                </table>
              </data>
            </slide>
            """
        )

        issue = next(
            issue
            for issue in result["slides"][0]["issues"]
            if issue["code"] == "duplicate_element_id"
        )
        self.assertEqual(issue["element_ids"], ["baa", "baa"])
        self.assertEqual(
            [obj["xml_path"] for obj in issue["related_objects"]],
            ["slide[1]/data/shape[1]", "slide[1]/data/table[1]/tr[1]/td[1]"],
        )

    def test_lint_xml_blocks_duplicate_id_shared_by_shape_and_undefined(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="dup" type="rect" topLeftX="40" topLeftY="40" width="80" height="80"/>
                <undefined id="dup" type="video"/>
              </data>
            </slide>
            """
        )

        issue = next(
            issue
            for issue in result["slides"][0]["issues"]
            if issue["code"] == "duplicate_element_id"
        )
        self.assertFalse(result["summary"]["release_ready"])
        self.assertEqual(issue["element_ids"], ["dup", "dup"])
        self.assertEqual(
            issue["related_objects"],
            [
                {
                    "element_id": "dup",
                    "kind": "shape",
                    "type": "rect",
                    "bbox": {"x": 40, "y": 40, "width": 80, "height": 80},
                    "xml_path": "slide[1]/data/shape[1]",
                },
                {
                    "element_id": "dup",
                    "kind": "undefined",
                    "type": "video",
                    "xml_path": "slide[1]/data/undefined[1]",
                },
            ],
        )

    def test_lint_xml_does_not_report_unique_table_cell_and_note_ids(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <table id="table-1" topLeftX="80" topLeftY="80" width="800" height="120">
                  <colgroup><col width="400"/><col width="400"/></colgroup>
                  <tr height="120"><td id="baa"/><td id="bab"/></tr>
                </table>
              </data>
              <note id="bac"><content fontSize="12"><p>Note</p></content></note>
            </slide>
            """
        )

        self.assertNotIn(
            "duplicate_element_id",
            [issue["code"] for issue in result["slides"][0]["issues"]],
        )

    def test_lint_xml_blocks_duplicate_ids_across_slides(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide>
                <data>
                  <shape id="dup" type="rect" topLeftX="40" topLeftY="40" width="80" height="80"/>
                </data>
              </slide>
              <slide>
                <data>
                  <shape id="dup" type="rect" topLeftX="140" topLeftY="40" width="80" height="80"/>
                </data>
              </slide>
            </presentation>
            """
        )

        issue = next(
            issue
            for issue in result["document"]["errors"]
            if issue["code"] == "duplicate_element_id"
        )
        self.assertFalse(result["summary"]["release_ready"])
        self.assertEqual(issue["element_ids"], ["dup", "dup"])
        self.assertEqual(
            issue["related_objects"],
            [
                {
                    "element_id": "dup",
                    "kind": "shape",
                    "type": "rect",
                    "bbox": {"x": 40, "y": 40, "width": 80, "height": 80},
                    "xml_path": "slide[1]/data/shape[1]",
                },
                {
                    "element_id": "dup",
                    "kind": "shape",
                    "type": "rect",
                    "bbox": {"x": 140, "y": 40, "width": 80, "height": 80},
                    "xml_path": "slide[2]/data/shape[1]",
                },
            ],
        )

    def test_lint_xml_does_not_treat_slide_ids_as_element_ids(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide id="dup"><data/></slide>
              <slide id="dup"><data/></slide>
            </presentation>
            """
        )

        self.assertNotIn(
            "duplicate_element_id",
            [issue["code"] for issue in result["document"]["errors"]],
        )

    def test_lint_xml_does_not_treat_presentation_id_as_element_id(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" id="dup" width="960" height="540">
              <slide>
                <data><undefined id="dup" type="video"/></data>
              </slide>
            </presentation>
            """
        )

        self.assertNotIn(
            "duplicate_element_id",
            [
                issue["code"]
                for issue in [
                    *result["document"]["errors"],
                    *result["slides"][0]["errors"],
                ]
            ],
        )

    def test_lint_xml_blocks_duplicate_note_ids_across_slides(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide>
                <note id="baa"><content fontSize="12"><p>First note</p></content></note>
              </slide>
              <slide>
                <note id="baa"><content fontSize="12"><p>Copied note</p></content></note>
              </slide>
            </presentation>
            """
        )

        issue = next(
            issue
            for issue in result["document"]["errors"]
            if issue["code"] == "duplicate_element_id"
        )
        self.assertFalse(result["summary"]["release_ready"])
        self.assertEqual(issue["element_ids"], ["baa", "baa"])
        self.assertEqual(
            issue["related_objects"],
            [
                {
                    "element_id": "baa",
                    "kind": "note",
                    "type": "note",
                    "xml_path": "slide[1]/note[1]",
                },
                {
                    "element_id": "baa",
                    "kind": "note",
                    "type": "note",
                    "xml_path": "slide[2]/note[1]",
                },
            ],
        )

    def test_lint_xml_cross_kind_duplicate_id_does_not_change_related_object_kind(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="dup" type="rect" topLeftX="-20" topLeftY="40" width="50" height="50"/>
                <img id="dup" src="token" topLeftX="100" topLeftY="40" width="50" height="50"/>
              </data>
            </slide>
            """
        )

        issue = next(
            issue for issue in result["slides"][0]["issues"]
            if issue["code"] == "shape_out_of_canvas"
        )
        self.assertEqual(issue["related_objects"][0]["kind"], "shape")
        self.assertEqual(
            issue["related_objects"][0]["xml_path"],
            "slide[1]/data/shape[1]",
        )
        duplicate_issue = next(
            issue
            for issue in result["slides"][0]["issues"]
            if issue["code"] == "duplicate_element_id"
        )
        self.assertEqual(
            [obj["xml_path"] for obj in duplicate_issue["related_objects"]],
            ["slide[1]/data/shape[1]", "slide[1]/data/img[1]"],
        )

    def test_normalize_issue_does_not_repeat_xml_path_hint_prefix(self) -> None:
        xml_path = "slide[1]/data/shape[1]"
        element = {
            "id": "box",
            "_source_id": "box",
            "_ref": xml_path,
            "xml_path": xml_path,
            "kind": "shape",
            "type": "rect",
            "x": 0,
            "y": 0,
            "width": 40,
            "height": 40,
        }
        prefix = "Locate via related_objects[].xml_path."

        issue = xml_lint.normalize_issue(
            {
                "level": "error",
                "code": "shape_out_of_canvas",
                "elements": [xml_path],
                "canvas": {"width": 960, "height": 540},
                "bbox": {"x": -10, "y": 0, "width": 40, "height": 40},
                "overflow": {"left": 10, "top": 0, "right": 0, "bottom": 0},
                "hint": f"{prefix} Move the shape inside the canvas.",
            },
            1,
            {xml_path: element},
        )

        self.assertEqual(issue["hint"].count(prefix), 1)

    def test_lint_xml_elements_keep_locator_for_anonymous_related_object(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape type="text" topLeftX="100" topLeftY="100" width="300" height="100">
                  <content fontSize="24"><p>Important text</p></content>
                </shape>
                <img id="srv-42" src="token" topLeftX="100" topLeftY="100" width="300" height="100"/>
              </data>
            </slide>
            """
        )

        issue = next(
            issue
            for issue in result["slides"][0]["issues"]
            if issue["code"] == "image_covers_text"
        )
        self.assertEqual(
            issue["elements"],
            ["srv-42", "slide[1]/data/shape[1]"],
        )
        self.assertEqual(issue["element_ids"], ["srv-42"])
        self.assertEqual(len(issue["related_objects"]), 2)

    def test_normalize_issue_deduplicates_repeated_element_refs(self) -> None:
        xml_path = "slide[1]/data/shape[1]"
        element = {
            "id": "srv-42",
            "_source_id": "srv-42",
            "_ref": xml_path,
            "xml_path": xml_path,
            "kind": "shape",
            "type": "rect",
            "x": 0,
            "y": 0,
            "width": 40,
            "height": 40,
        }

        issue = xml_lint.normalize_issue(
            {
                "level": "warning",
                "code": "blank_slide",
                "measurement": {
                    "visible_element_count": 0,
                    "declared_element_count": 1,
                },
                "elements": [xml_path, xml_path],
            },
            1,
            {xml_path: element},
        )

        self.assertEqual(issue["elements"], ["srv-42"])
        self.assertEqual(issue["element_ids"], ["srv-42"])
        self.assertEqual(len(issue["related_objects"]), 1)

    def test_lint_xml_reports_duplicate_ids_for_every_linted_element_kind(self) -> None:
        duplicate_pairs = {
            "shape": (
                '<shape id="dup" type="rect" topLeftX="10" topLeftY="10" width="40" height="40"/>',
                '<shape id="dup" type="rect" topLeftX="60" topLeftY="10" width="40" height="40"/>',
            ),
            "chart": (
                '<chart id="dup" topLeftX="10" topLeftY="10" width="40" height="40"><chartPlotArea><chartPlot type="line"/></chartPlotArea><chartData><dim1><chartField name="category" valueType="string">A</chartField></dim1><dim2><chartField name="value" valueType="number">1</chartField></dim2></chartData></chart>',
                '<chart id="dup" topLeftX="60" topLeftY="10" width="40" height="40"><chartPlotArea><chartPlot type="line"/></chartPlotArea><chartData><dim1><chartField name="category" valueType="string">A</chartField></dim1><dim2><chartField name="value" valueType="number">1</chartField></dim2></chartData></chart>',
            ),
            "table": (
                '<table id="dup" topLeftX="10" topLeftY="10" width="40" height="40"><colgroup><col width="40"/></colgroup><tr height="40"><td/></tr></table>',
                '<table id="dup" topLeftX="60" topLeftY="10" width="40" height="40"><colgroup><col width="40"/></colgroup><tr height="40"><td/></tr></table>',
            ),
            "img": (
                '<img id="dup" src="token" topLeftX="10" topLeftY="10" width="40" height="40"/>',
                '<img id="dup" src="token" topLeftX="60" topLeftY="10" width="40" height="40"/>',
            ),
            "line": (
                '<line id="dup" startX="10" startY="10" endX="40" endY="40"><border color="rgb(0, 0, 0)"/></line>',
                '<line id="dup" startX="60" startY="10" endX="90" endY="40"><border color="rgb(0, 0, 0)"/></line>',
            ),
            "icon": (
                '<icon id="dup" iconType="iconpark/Base/setting.svg" topLeftX="10" topLeftY="10" width="40" height="40"><fill><fillColor color="rgb(0, 0, 0)"/></fill></icon>',
                '<icon id="dup" iconType="iconpark/Base/setting.svg" topLeftX="60" topLeftY="10" width="40" height="40"><fill><fillColor color="rgb(0, 0, 0)"/></fill></icon>',
            ),
            "polyline": (
                '<polyline id="dup" topLeftX="10" topLeftY="10" width="40" height="40"><border color="rgb(0, 0, 0)"/></polyline>',
                '<polyline id="dup" topLeftX="60" topLeftY="10" width="40" height="40"><border color="rgb(0, 0, 0)"/></polyline>',
            ),
        }
        for kind, pair in duplicate_pairs.items():
            with self.subTest(kind=kind):
                result = xml_lint.lint_xml(
                    f'<slide xmlns="https://www.larkoffice.com/sml/2.0"><data>{pair[0]}{pair[1]}</data></slide>'
                )
                issue = next(
                    issue
                    for issue in result["slides"][0]["issues"]
                    if issue["code"] == "duplicate_element_id"
                )
                self.assertEqual(issue["element_ids"], ["dup", "dup"])
                self.assertEqual(
                    [obj["xml_path"] for obj in issue["related_objects"]],
                    [
                        f"slide[1]/data/{kind}[1]",
                        f"slide[1]/data/{kind}[2]",
                    ],
                )

    def test_lint_xml_missing_id_does_not_collide_with_explicit_synthetic_like_id(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape type="rect" topLeftX="-20" topLeftY="40" width="50" height="50"/>
                <shape id="shape-1" type="rect" topLeftX="100" topLeftY="40" width="50" height="50"/>
              </data>
            </slide>
            """
        )

        issue = next(
            issue for issue in result["slides"][0]["issues"]
            if issue["code"] == "shape_out_of_canvas"
        )
        self.assertEqual(issue["element_ids"], [])
        self.assertNotIn("element_id", issue["related_objects"][0])
        self.assertEqual(
            issue["related_objects"][0]["xml_path"],
            "slide[1]/data/shape[1]",
        )
        self.assertTrue(
            issue["hint"].startswith("Locate via related_objects[].xml_path. ")
        )
        self.assertNotIn(
            "duplicate_element_id",
            [candidate["code"] for candidate in result["slides"][0]["issues"]],
        )

    def test_lint_xml_empty_id_is_not_exposed_as_an_element_id(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="" type="rect" topLeftX="-20" topLeftY="40" width="50" height="50"/>
              </data>
            </slide>
            """
        )

        issue = next(
            issue
            for issue in result["slides"][0]["issues"]
            if issue["code"] == "shape_out_of_canvas"
        )
        self.assertEqual(issue["element_ids"], [])
        self.assertNotIn("element_id", issue["related_objects"][0])
        self.assertEqual(
            issue["related_objects"][0]["xml_path"],
            "slide[1]/data/shape[1]",
        )

    def test_lint_xml_uses_resolved_table_bounds_for_canvas_validation(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="https://www.larkoffice.com/sml/2.0">
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

    def test_lint_xml_uses_the_same_anonymous_table_path_for_all_table_diagnostics(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        self.assertEqual(canvas_issue["elements"], ["slide[1]/data/table[1]"])
        self.assertEqual(mismatch_issue["elements"], ["slide[1]/data/table[1]"])
        self.assertEqual(
            canvas_issue["related_objects"][0]["xml_path"],
            "slide[1]/data/table[1]",
        )
        self.assertEqual(
            mismatch_issue["related_objects"][0]["xml_path"],
            "slide[1]/data/table[1]",
        )
        self.assertNotIn("element_id", canvas_issue["related_objects"][0])
        self.assertNotIn("element_id", mismatch_issue["related_objects"][0])

    def test_lint_xml_reports_info_when_table_target_size_resolves_larger_than_declared(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        final_sizes = xml_lint.fill_last_size_gap([10, 10], 3)
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
        script_path = Path(xml_lint.__file__).resolve()
        with tempfile.TemporaryDirectory() as temp_dir:
            for name, (table_xml, expected_info_count) in cases.items():
                with self.subTest(case=name):
                    input_path = Path(temp_dir) / f"{name}.xml"
                    input_path.write_text(
                        f"""
                        <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
                          <slide xmlns="https://www.larkoffice.com/sml/2.0"><data>{table_xml}</data></slide>
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
                result = xml_lint.lint_xml(
                    f"""
                    <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
                      <slide xmlns="https://www.larkoffice.com/sml/2.0">
                        <data>{shapes}</data>
                      </slide>
                    </presentation>
                    """
                )
                self.assertEqual(result["summary"]["error_count"], 1)
                self.assertEqual(result["slides"][0]["issues"][0]["code"], "bbox_overlap")


    def test_lint_xml_reports_vertical_text_image_overlap_as_warning(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0"><data>
              <shape id="text" type="text" vert="vert" topLeftX="100" topLeftY="100" width="100" height="100">
                <content><p>Vertical</p></content>
              </shape>
              <img id="image" src="token" topLeftX="120" topLeftY="120" width="20" height="20"/>
            </data></slide>
            """
        )
        issue = next(issue for issue in result["slides"][0]["issues"] if issue["code"] == "image_may_cover_vertical_text")
        self.assertEqual(issue["level"], "info")
        self.assertEqual(result["summary"]["error_count"], 0)
        self.assertEqual(result["summary"]["info_count"], 1)

    def test_lint_xml_related_objects_include_source_xml_paths(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide>
                <data>
                  <shape type="text" topLeftX="80" topLeftY="80" width="400" height="60">
                    <content fontSize="24"><p>Control slide</p></content>
                  </shape>
                </data>
              </slide>
              <slide>
                <data>
                  <shape type="text" topLeftX="70" topLeftY="55" width="820" height="70">
                    <content fontSize="32"><p>shape-3 mapping experiment</p></content>
                  </shape>
                  <shape type="text" topLeftX="80" topLeftY="165" width="400" height="70">
                    <content fontSize="18"><p>First, preserve source order.</p></content>
                  </shape>
                  <shape type="text" topLeftX="80" topLeftY="315" width="400" height="64">
                    <content fontSize="26"><p>TARGET_SHAPE_THREE</p></content>
                  </shape>
                  <img src="token" topLeftX="80" topLeftY="305" width="400" height="110"/>
                </data>
              </slide>
            </presentation>
            """
        )

        issue = next(
            issue
            for issue in result["slides"][1]["issues"]
            if issue["code"] == "image_covers_text"
        )
        self.assertEqual(
            [(obj["kind"], obj["xml_path"]) for obj in issue["related_objects"]],
            [
                ("img", "slide[2]/data/img[1]"),
                ("shape", "slide[2]/data/shape[3]"),
            ],
        )
        self.assertTrue(
            all("element_id" not in obj for obj in issue["related_objects"])
        )

    def test_lint_xml_related_objects_include_line_xml_path(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="label" type="text" topLeftX="100" topLeftY="100" width="200" height="80">
                  <content fontSize="24"><p>Crossed text</p></content>
                </shape>
                <line id="connector" startX="80" startY="130" endX="330" endY="130">
                  <border color="rgb(15, 23, 42)" width="2"/>
                </line>
              </data>
            </slide>
            """
        )

        issue = next(
            issue
            for issue in result["slides"][0]["issues"]
            if issue["code"] == "bbox_overlap" and issue["elements"][0] == "connector"
        )
        self.assertEqual(
            {
                obj["element_id"]: obj["xml_path"]
                for obj in issue["related_objects"]
            },
            {
                "connector": "slide[1]/data/line[1]",
                "label": "slide[1]/data/shape[1]",
            },
        )


class XmlTextOverlapLintDensityTest(unittest.TestCase):
    def test_lint_xml_sparse_container_related_objects_include_icon_xml_path(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="card" type="rect" topLeftX="60" topLeftY="120" width="400" height="300"/>
                <icon id="visual" iconType="iconpark/Base/setting.svg" topLeftX="80" topLeftY="140" width="32" height="32">
                  <fill><fillColor color="rgb(37, 99, 235)"/></fill>
                </icon>
              </data>
            </slide>
            """
        )

        issue = next(
            issue
            for issue in result["slides"][0]["issues"]
            if issue["code"] == "sparse_container_content"
        )
        self.assertEqual(
            {
                obj["element_id"]: obj["xml_path"]
                for obj in issue["related_objects"]
            },
            {
                "card": "slide[1]/data/shape[1]",
                "visual": "slide[1]/data/icon[1]",
            },
        )

    def test_lint_xml_blocks_blank_slide(self) -> None:
        result = xml_lint.lint_xml(
            """
            <presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540">
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
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <img id="ghost" src="token" topLeftX="60" topLeftY="60" width="200" height="200" alpha="0"/>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["summary"]["error_count"], 1)
        issue = result["slides"][0]["errors"][0]
        self.assertEqual(issue["code"], "blank_slide")

    def test_lint_xml_warns_when_large_container_is_mostly_empty(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
            "container_xml_path": "slide[1]/data/shape[1]",
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
        self.assertEqual(issue["measurement"]["content_coverage_ratio"], 0.03)
        self.assertEqual(issue["elements"], ["trend-card", "trend-title", "trend-copy"])
        self.assertEqual(issue["element_ids"], ["trend-card", "trend-title", "trend-copy"])
        self.assertEqual(
            [obj["element_id"] for obj in issue["related_objects"]],
            ["trend-card", "trend-title", "trend-copy"],
        )
        self.assertEqual(result["slides"][0]["status"], "needs_screenshot_review")
        self.assertEqual(result["slides"][0]["warnings"], result["slides"][0]["issues"])

    def test_lint_xml_uses_xml_path_in_anonymous_sparse_container_message(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape type="rect" topLeftX="500" topLeftY="135" width="410" height="370"/>
                <shape id="trend-title" type="text" topLeftX="515" topLeftY="147" width="380" height="28">
                  <content fontSize="15"><p>Core trends</p></content>
                </shape>
              </data>
            </slide>
            """
        )

        issue = next(
            issue
            for issue in result["slides"][0]["issues"]
            if issue["code"] == "sparse_container_content"
        )
        self.assertNotIn("container_id", issue["target"])
        self.assertEqual(
            issue["target"]["container_xml_path"],
            "slide[1]/data/shape[1]",
        )
        self.assertEqual(
            issue["message"],
            "large card slide[1]/data/shape[1] content coverage 1.0% is below 15.0%",
        )

    def test_lint_xml_warns_for_sparse_short_cards(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        self_only = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="card" type="rect" topLeftX="60" topLeftY="140" width="220" height="184">
                  <content fontSize="12"><p>A</p></content>
                </shape>
              </data>
            </slide>
            """
        )
        with_overlapping_child = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        elements = xml_lint.extract_density_elements(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        elements = xml_lint.extract_density_elements(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="chart-card" type="rect" topLeftX="500" topLeftY="135" width="410" height="300"/>
                <chart id="chart" topLeftX="525" topLeftY="170" width="350" height="220">
                  <chartPlotArea><chartPlot type="line"/></chartPlotArea>
                  <chartData>
                    <dim1><chartField name="category" valueType="string">A</chartField></dim1>
                    <dim2><chartField name="value" valueType="number">1</chartField></dim2>
                  </chartData>
                </chart>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["summary"]["warning_count"], 0)

    def test_lint_xml_does_not_let_transparent_visual_child_suppress_sparse_warning(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="title" type="text" topLeftX="40" topLeftY="40" width="300" height="40">
                  <content fontSize="20"><p>Section title</p></content>
                </shape>
                <shape id="chart-card" type="rect" topLeftX="500" topLeftY="135" width="410" height="300"/>
                <chart id="chart" topLeftX="525" topLeftY="170" width="350" height="220" alpha="0">
                  <chartPlotArea><chartPlot type="line"/></chartPlotArea>
                  <chartData>
                    <dim1><chartField name="category" valueType="string">A</chartField></dim1>
                    <dim2><chartField name="value" valueType="number">1</chartField></dim2>
                  </chartData>
                </chart>
              </data>
            </slide>
            """
        )

        issue = next(
            issue for issue in result["slides"][0]["issues"] if issue["code"] == "sparse_container_content"
        )
        self.assertEqual(issue["target"]["container_id"], "chart-card")

    def test_lint_xml_warns_for_small_empty_visual_placeholder_cards(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <img id="hero" topLeftX="560" topLeftY="0" width="400" height="540"/>
                <shape id="tint" type="rect" topLeftX="560" topLeftY="0" width="400" height="540"/>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["summary"]["warning_count"], 0)

    def test_lint_xml_does_not_let_transparent_image_overlay_suppress_sparse_warning(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="title" type="text" topLeftX="40" topLeftY="40" width="300" height="40">
                  <content fontSize="20"><p>Section title</p></content>
                </shape>
                <shape id="card" type="rect" topLeftX="330" topLeftY="120" width="300" height="300"/>
                <img id="ghost-overlay" src="token" topLeftX="330" topLeftY="120" width="300" height="300" alpha="0"/>
              </data>
            </slide>
            """
        )

        issue = next(
            issue for issue in result["slides"][0]["issues"] if issue["code"] == "sparse_container_content"
        )
        self.assertEqual(issue["target"]["container_id"], "card")

    def test_lint_xml_allows_edge_spanning_layout_panel_and_nested_decoration(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="panel" type="rect" topLeftX="600" topLeftY="0" width="360" height="540"/>
                <shape id="decoration" type="rect" topLeftX="660" topLeftY="150" width="240" height="240"/>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["summary"]["warning_count"], 0)

    def test_lint_xml_counts_icons_as_visible_content(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="metric-card" type="rect" topLeftX="80" topLeftY="140" width="360" height="300"/>
                <shape id="metric" type="text" topLeftX="104" topLeftY="190" width="340" height="90">
                  <content fontSize="12"><p><strong><span fontSize="62">400</span></strong>+ 项</p></content>
                </shape>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["slides"][0]["issues"], [])

    def test_lint_xml_does_not_report_blank_slide_for_embed_only_content(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <embed id="emb" topLeftX="280" topLeftY="130" width="400" height="280">
                  <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 400 280">
                    <circle cx="200" cy="140" r="100" fill="#2563EB"/>
                  </svg>
                </embed>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["summary"]["error_count"], 0)
        codes = [issue["code"] for issue in result["slides"][0]["issues"]]
        self.assertNotIn("blank_slide", codes)

    def test_lint_xml_does_not_report_blank_slide_for_line_only_content(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
              <data>
                <line id="l1" startX="100" startY="100" endX="800" endY="100"><border/></line>
                <line id="l2" startX="100" startY="200" endX="800" endY="200"><border/></line>
                <line id="l3" startX="100" startY="300" endX="800" endY="300"><border/></line>
                <line id="l4" startX="100" startY="400" endX="800" endY="400"><border/></line>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["summary"]["error_count"], 0)
        codes = [issue["code"] for issue in result["slides"][0]["issues"]]
        self.assertNotIn("blank_slide", codes)

    def test_lint_xml_reports_bbox_overlap_measurement_from_decision_time_visual_bbox(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        self.assertEqual(issue["measurement"]["intersection_width"], 109.2)
        self.assertEqual(issue["measurement"]["intersection_height"], 6.8)
        self.assertEqual(issue["measurement"]["intersection_area"], 742.56)

    def test_has_similar_short_card_peer_excludes_the_element_itself(self) -> None:
        card_a = {"kind": "shape", "type": "rect", "x": 0, "y": 0, "width": 300, "height": 100}
        card_b = {"kind": "shape", "type": "rect", "x": 400, "y": 0, "width": 300, "height": 100}
        card_c = {"kind": "shape", "type": "rect", "x": 0, "y": 200, "width": 300, "height": 100}

        self.assertFalse(
            xml_lint.has_similar_short_card_peer(card_a, [card_a, card_b])
        )
        self.assertTrue(
            xml_lint.has_similar_short_card_peer(card_a, [card_a, card_b, card_c])
        )

    def test_lint_xml_reports_schema_version_2_for_sparse_issues(self) -> None:
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
        result = xml_lint.lint_xml(
            """
            <slide xmlns="https://www.larkoffice.com/sml/2.0">
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
            xml_lint.has_similar_short_card_peer(
                visible_card, [visible_card, ghost_1, ghost_2]
            )
        )


SML_NAMESPACE = "https://www.larkoffice.com/sml/2.0"


class SxsdSyntaxTestCase(unittest.TestCase):
    def validate(self, xml: str) -> list[dict[str, object]]:
        result = xml_lint.lint_xml(xml)
        return [
            *result.get("issues", []),
            *(issue for slide in result["slides"] for issue in slide["issues"]),
        ]

    def assert_issue(
        self,
        issues: list[dict[str, object]],
        code: str,
        *,
        path: str | None = None,
        attr: str | None = None,
    ) -> dict[str, object]:
        for issue in issues:
            if issue.get("code") != code:
                continue
            if path is not None and issue.get("path") != path:
                continue
            if attr is not None and issue.get("attr") != attr:
                continue
            return issue
        self.fail(f"missing issue code={code!r} path={path!r} attr={attr!r}: {issues!r}")

    def assert_no_issue(self, issues: list[dict[str, object]], code: str) -> None:
        self.assertNotIn(code, [issue.get("code") for issue in issues])


class SxsdSyntaxAttributeTest(SxsdSyntaxTestCase):

    def test_xsd_pattern_translation_only_expands_whitespace_classes(self) -> None:
        self.assertEqual(
            sxsd_validator.python_pattern_for_xsd(r"\s+\S+\w+\d+"),
            "[ \\t\\n\\r]+[^ \\t\\n\\r]+\\w+\\d+",
        )

    def test_href_domain_pattern_does_not_use_backtracking_regex(self) -> None:
        pattern = r"[\w.-]+[.:]\S*"
        adversarial_value = ("a." * 20_000) + " "
        original_fullmatch = sxsd_validator.re.fullmatch
        translated_pattern = sxsd_validator.python_pattern_for_xsd(pattern)

        def reject_unsafe_pattern(candidate: str, value: str):
            if candidate == translated_pattern:
                raise AssertionError("href domain pattern must not use re.fullmatch")
            return original_fullmatch(candidate, value)

        with mock.patch.object(sxsd_validator.re, "fullmatch", side_effect=reject_unsafe_pattern):
            self.assertFalse(sxsd_validator.xsd_pattern_matches(pattern, adversarial_value))

    def test_href_domain_pattern_keeps_xsd_matching_behavior(self) -> None:
        pattern = r"[\w.-]+[.:]\S*"
        reference_pattern = sxsd_validator.re.compile(
            sxsd_validator.python_pattern_for_xsd(pattern)
        )

        for length in range(5):
            for characters in itertools.product("a.:-/ ©", repeat=length):
                value = "".join(characters)
                with self.subTest(value=value):
                    self.assertEqual(
                        sxsd_validator.xsd_pattern_matches(pattern, value),
                        reference_pattern.fullmatch(value) is not None,
                    )

    def test_accepts_valid_shape_attributes(self) -> None:
        issues = self.validate(
            f"""
            <slide xmlns="{SML_NAMESPACE}">
              <data>
                <shape type="text" topLeftX="10" topLeftY="20" width="300" height="80">
                  <content textType="body"><p>Valid</p></content>
                </shape>
              </data>
            </slide>
            """
        )

        self.assertEqual(issues, [])

    def test_reports_missing_required_shape_attribute(self) -> None:
        issues = self.validate(
            f"""
            <slide xmlns="{SML_NAMESPACE}">
              <data><shape type="text" topLeftX="10" topLeftY="20" width="300"/></data>
            </slide>
            """
        )

        issue = self.assert_issue(
            issues,
            "sxsd_missing_required_attr",
            path="slide/data/shape",
            attr="height",
        )
        self.assertEqual(issue["expected"], "required attribute of type PositiveSize")
        self.assertIsNone(issue["actual"])

    def test_reports_invalid_scalar_value(self) -> None:
        issues = self.validate(
            f"""
            <slide xmlns="{SML_NAMESPACE}">
              <data>
                <shape type="text" topLeftX="NaN" topLeftY="20" width="300" height="80"/>
              </data>
            </slide>
            """
        )

        issue = self.assert_issue(issues, "sxsd_invalid_scalar", attr="topLeftX")
        self.assertEqual(issue["actual"], "NaN")

    def test_rejects_python_only_numeric_separator(self) -> None:
        issues = self.validate(
            f"""
            <slide xmlns="{SML_NAMESPACE}">
              <data>
                <shape type="rect" topLeftX="1_0" topLeftY="20" width="300" height="80"/>
              </data>
            </slide>
            """
        )

        self.assert_issue(issues, "sxsd_invalid_scalar", attr="topLeftX")

    def test_accepts_xsd_double_lexical_forms(self) -> None:
        for top_left_x in ("10", "-0.5", ".5", "1.", "1e2"):
            with self.subTest(top_left_x=top_left_x):
                issues = self.validate(
                    f"""
                    <slide xmlns="{SML_NAMESPACE}">
                      <data>
                        <shape type="rect" topLeftX="{top_left_x}" topLeftY="20" width="300" height="80"/>
                      </data>
                    </slide>
                    """
                )

                self.assertEqual(issues, [])

    def test_accepts_bullet_char_length_boundaries(self) -> None:
        for bullet_char in ("A", "12345678"):
            with self.subTest(bullet_char=bullet_char):
                issues = self.validate(
                    f"""
                    <slide xmlns="{SML_NAMESPACE}">
                      <data>
                        <shape type="text" topLeftX="10" topLeftY="20" width="300" height="80">
                          <content bulletChar="{bullet_char}"><p>Text</p></content>
                        </shape>
                      </data>
                    </slide>
                    """
                )

                self.assertEqual(issues, [])

    def test_rejects_bullet_char_outside_length_boundaries(self) -> None:
        for bullet_char in ("", "123456789"):
            with self.subTest(bullet_char=bullet_char):
                issues = self.validate(
                    f"""
                    <slide xmlns="{SML_NAMESPACE}">
                      <data>
                        <shape type="text" topLeftX="10" topLeftY="20" width="300" height="80">
                          <content bulletChar="{bullet_char}"><p>Text</p></content>
                        </shape>
                      </data>
                    </slide>
                    """
                )

                self.assert_issue(issues, "sxsd_value_out_of_range", attr="bulletChar")

    def test_rejects_zero_size_that_violates_xsd(self) -> None:
        issues = self.validate(
            f"""
            <slide xmlns="{SML_NAMESPACE}">
              <data>
                <shape type="text" topLeftX="10" topLeftY="20" width="0" height="80"/>
              </data>
            </slide>
            """
        )

        self.assert_issue(issues, "sxsd_value_out_of_range", attr="width")

    def test_reports_negative_size_rejected_by_xsd(self) -> None:
        issues = self.validate(
            f"""
            <slide xmlns="{SML_NAMESPACE}">
              <data>
                <shape type="text" topLeftX="10" topLeftY="20" width="-1" height="80"/>
              </data>
            </slide>
            """
        )

        self.assert_issue(issues, "sxsd_value_out_of_range", attr="width")

    def test_rejects_shape_enum_that_violates_xsd(self) -> None:
        issues = self.validate(
            f"""
            <slide xmlns="{SML_NAMESPACE}">
              <data>
                <shape type="not-a-shape" topLeftX="10" topLeftY="20" width="300" height="80"/>
              </data>
            </slide>
            """
        )

        issue = self.assert_issue(issues, "sxsd_invalid_enum", attr="type")
        self.assertLess(len(str(issue["message"])), 300)
        self.assertEqual(issue["actual"], "not-a-shape")

    def test_rejects_rotation_upper_bound_that_violates_xsd(self) -> None:
        issues = self.validate(
            f"""
            <slide xmlns="{SML_NAMESPACE}">
              <data>
                <shape type="text" topLeftX="10" topLeftY="20" width="300" height="80" rotation="360"/>
              </data>
            </slide>
            """
        )

        self.assert_issue(issues, "sxsd_value_out_of_range", attr="rotation")

    def test_rejects_fill_color_that_violates_xsd(self) -> None:
        issues = self.validate(
            f"""
            <slide xmlns="{SML_NAMESPACE}">
              <style><fill><fillColor color="red"/></fill></style>
              <data/>
            </slide>
            """
        )

        self.assert_issue(issues, "sxsd_pattern_mismatch", attr="color")

    def test_reports_missing_required_image_src(self) -> None:
        issues = self.validate(
            f"""
            <slide xmlns="{SML_NAMESPACE}">
              <data><img topLeftX="10" topLeftY="20" width="300" height="80"/></data>
            </slide>
            """
        )

        self.assert_issue(issues, "sxsd_missing_required_attr", attr="src")

    def test_accepts_inline_attribute_simple_type(self) -> None:
        issues = self.validate(
            f"""
            <slide xmlns="{SML_NAMESPACE}">
              <data>
                <shape type="text" topLeftX="10" topLeftY="20" width="300" height="80">
                  <content><p><a href="https://example.com">Link</a></p></content>
                </shape>
              </data>
            </slide>
            """
        )

        self.assertEqual(issues, [])

    def test_reports_inline_attribute_pattern_mismatch(self) -> None:
        issues = self.validate(
            f"""
            <slide xmlns="{SML_NAMESPACE}">
              <data>
                <shape type="text" topLeftX="10" topLeftY="20" width="300" height="80">
                  <content><p><a href="not a uri">Link</a></p></content>
                </shape>
              </data>
            </slide>
            """
        )

        self.assert_issue(issues, "sxsd_pattern_mismatch", attr="href")

    def test_accepts_values_matching_inline_union_members(self) -> None:
        for bullet_size in ("25%", "100%", "400%", "6", "14", "400"):
            with self.subTest(bullet_size=bullet_size):
                issues = self.validate(
                    f"""
                    <slide xmlns="{SML_NAMESPACE}">
                      <data>
                        <shape type="text" topLeftX="10" topLeftY="20" width="300" height="80">
                          <content bulletSize="{bullet_size}"><p>Text</p></content>
                        </shape>
                      </data>
                    </slide>
                    """
                )

                self.assertEqual(issues, [])

    def test_rejects_values_outside_inline_union_members(self) -> None:
        for bullet_size in ("24%", "401%", "5", "401", "abc"):
            with self.subTest(bullet_size=bullet_size):
                issues = self.validate(
                    f"""
                    <slide xmlns="{SML_NAMESPACE}">
                      <data>
                        <shape type="text" topLeftX="10" topLeftY="20" width="300" height="80">
                          <content bulletSize="{bullet_size}"><p>Text</p></content>
                        </shape>
                      </data>
                    </slide>
                    """
                )

                self.assert_issue(issues, "sxsd_pattern_mismatch", attr="bulletSize")

    def test_rejects_symbol_outside_python_word_semantics_in_href(self) -> None:
        issues = self.validate(
            f"""
            <slide xmlns="{SML_NAMESPACE}">
              <data>
                <shape type="text" topLeftX="10" topLeftY="20" width="300" height="80">
                  <content><p><a href="©:resource">Link</a></p></content>
                </shape>
              </data>
            </slide>
            """
        )

        self.assert_issue(issues, "sxsd_pattern_mismatch", attr="href")

    def test_accepts_common_email_href_with_python_regex_semantics(self) -> None:
        issues = self.validate(
            f"""
            <slide xmlns="{SML_NAMESPACE}">
              <data>
                <shape type="text" topLeftX="10" topLeftY="20" width="300" height="80">
                  <content><p><a href="mailto:user@example.com">Email</a></p></content>
                </shape>
              </data>
            </slide>
            """
        )

        self.assertEqual(issues, [])

    def test_accepts_common_gradient_with_python_regex_semantics(self) -> None:
        issues = self.validate(
            f"""
            <slide xmlns="{SML_NAMESPACE}">
              <style>
                <fill>
                  <fillColor color="linear-gradient(90deg, rgb(255, 0, 0) 0%, rgb(0, 0, 255) 100%)"/>
                </fill>
              </style>
              <data>
                <shape type="text" topLeftX="10" topLeftY="20" width="300" height="80">
                  <content><p>Gradient</p></content>
                </shape>
              </data>
            </slide>
            """
        )

        self.assertEqual(issues, [])

    def test_rejects_non_xsd_whitespace_in_color_pattern(self) -> None:
        issues = self.validate(
            f"""
            <slide xmlns="{SML_NAMESPACE}">
              <style><fill><fillColor color="rgb(1,\u00a02,3)"/></fill></style>
              <data/>
            </slide>
            """
        )

        self.assert_issue(issues, "sxsd_pattern_mismatch", attr="color")

class SxsdSyntaxStructureTest(SxsdSyntaxTestCase):
    def test_accepts_nested_content_in_referenced_rich_text_shadow(self) -> None:
        issues = self.validate(
            f"""
            <slide xmlns="{SML_NAMESPACE}">
              <data>
                <shape type="text" topLeftX="10" topLeftY="20" width="300" height="80">
                  <content>
                    <p><span><shadow color="rgba(0, 0, 0, 1)"><strong>Text</strong></shadow></span></p>
                  </content>
                </shape>
              </data>
            </slide>
            """
        )

        self.assertEqual(issues, [])

    def test_keeps_shape_effect_shadow_as_childless_local_type(self) -> None:
        issues = self.validate(
            f"""
            <slide xmlns="{SML_NAMESPACE}">
              <data>
                <shape type="rect" topLeftX="10" topLeftY="20" width="300" height="80">
                  <shadow><strong>Not rich text</strong></shadow>
                </shape>
              </data>
            </slide>
            """
        )

        self.assert_issue(
            issues,
            "sxsd_unexpected_child",
            path="slide/data/shape/shadow/strong",
        )

    def test_accepts_standalone_slide_fragment_without_namespace(self) -> None:
        issues = self.validate(
            '<slide><data><shape type="text" topLeftX="10" topLeftY="20" width="300" height="80">'
            '<content><p>Text</p></content></shape></data></slide>'
        )

        self.assertEqual(issues, [])

    def test_rejects_presentation_without_namespace(self) -> None:
        issues = self.validate(
            '<presentation width="960" height="540"><slide/></presentation>'
        )

        self.assert_issue(issues, "sxsd_invalid_namespace", path="presentation")

    def test_rejects_wrong_namespace_that_violates_xsd(self) -> None:
        issues = self.validate('<slide xmlns="https://example.com/not-sml"><data/></slide>')

        self.assert_issue(issues, "sxsd_invalid_namespace", path="slide")

    def test_rejects_descendant_outside_document_namespace(self) -> None:
        issues = self.validate(
            f"""
            <slide xmlns="{SML_NAMESPACE}">
              <data xmlns="">
                <shape xmlns="{SML_NAMESPACE}" type="rect" topLeftX="10" topLeftY="20" width="300" height="80"/>
              </data>
            </slide>
            """
        )

        self.assert_issue(issues, "sxsd_invalid_namespace", path="slide/data")

    def test_rejects_unexpected_child_that_violates_xsd(self) -> None:
        issues = self.validate(
            f"""
            <slide xmlns="{SML_NAMESPACE}">
              <shape type="text" topLeftX="10" topLeftY="20" width="300" height="80"/>
            </slide>
            """
        )

        self.assert_issue(issues, "sxsd_unexpected_child", path="slide/shape")

    def test_rejects_child_order_that_violates_xsd(self) -> None:
        issues = self.validate(
            f"""
            <presentation xmlns="{SML_NAMESPACE}" width="1920" height="1080">
              <slide/>
              <title>Late title</title>
            </presentation>
            """
        )

        self.assert_issue(issues, "sxsd_invalid_child_order", path="presentation/title")

    def test_enforces_presentation_slide_minimum_from_xsd(self) -> None:
        issues = self.validate(
            f'<presentation xmlns="{SML_NAMESPACE}" width="1920" height="1080"/>'
        )

        self.assert_issue(issues, "sxsd_missing_required_child", path="presentation")

    def test_enforces_presentation_slide_maximum_from_xsd(self) -> None:
        slides = "".join("<slide/>" for _ in range(101))
        issues = self.validate(
            f'<presentation xmlns="{SML_NAMESPACE}" width="1920" height="1080">{slides}</presentation>'
        )

        self.assert_issue(issues, "sxsd_too_many_children", path="presentation/slide")

    def test_rejects_multiple_choice_children_that_violate_xsd(self) -> None:
        issues = self.validate(
            f"""
            <slide xmlns="{SML_NAMESPACE}">
              <style>
                <fill><fillColor/><fillImg src="token"/></fill>
              </style>
            </slide>
            """
        )

        self.assert_issue(issues, "sxsd_too_many_children", path="slide/style/fill")

    def test_rejects_line_without_required_border_from_xsd(self) -> None:
        issues = self.validate(
            f"""
            <slide xmlns="{SML_NAMESPACE}">
              <data><line startX="0" startY="0" endX="100" endY="100"/></data>
            </slide>
            """
        )

        self.assert_issue(issues, "sxsd_missing_required_child", path="slide/data/line")

    def test_reports_missing_required_chart_structure(self) -> None:
        issues = self.validate(
            f"""
            <slide xmlns="{SML_NAMESPACE}">
              <data><chart topLeftX="0" topLeftY="0" width="300" height="200"/></data>
            </slide>
            """
        )

        self.assert_issue(issues, "sxsd_missing_required_child", path="slide/data/chart")

    def test_reports_missing_required_nested_sequence_child(self) -> None:
        issues = self.validate(
            f"""
            <slide xmlns="{SML_NAMESPACE}">
              <data><table topLeftX="0" topLeftY="0"><tr/></table></data>
            </slide>
            """
        )

        issue = self.assert_issue(issues, "sxsd_missing_required_child", path="slide/data/table/tr")
        self.assertEqual(issue["expected"], "td (at least 1)")


class SxsdSchemaModelTest(unittest.TestCase):
    def test_reports_unsupported_xsd_pattern_without_crashing(self) -> None:
        schema = rf"""
        <xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
                   xmlns:sml="{SML_NAMESPACE}"
                   targetNamespace="{SML_NAMESPACE}"
                   elementFormDefault="qualified">
          <xs:simpleType name="UnsupportedPatternType">
            <xs:union>
              <xs:simpleType>
                <xs:restriction base="xs:string"><xs:pattern value="[\S]"/></xs:restriction>
              </xs:simpleType>
              <xs:simpleType>
                <xs:restriction base="xs:string"><xs:pattern value="z+"/></xs:restriction>
              </xs:simpleType>
            </xs:union>
          </xs:simpleType>
          <xs:complexType name="SlideType">
            <xs:attribute name="value" type="sml:UnsupportedPatternType"/>
          </xs:complexType>
        </xs:schema>
        """
        with tempfile.TemporaryDirectory() as temp_dir:
            schema_path = Path(temp_dir) / "schema.xsd"
            schema_path.write_text(schema, encoding="utf-8")
            try:
                issues = sxsd_validator.validate_sxsd(
                    ET.fromstring(f'<slide xmlns="{SML_NAMESPACE}" value="A"/>'),
                    schema_path,
                )
            except (ValueError, sxsd_validator.re.error) as error:
                self.fail(f"SXSD pattern capability errors must be reported, not raised: {error}")

        self.assertEqual([issue["code"] for issue in issues], ["sxsd_unsupported_pattern"])
        self.assertEqual(issues[0]["attr"], "value")
        self.assertIn("pattern interpreter", str(issues[0]["hint"]).lower())

    def test_standalone_slide_uses_slide_type_without_global_element(self) -> None:
        schema = f"""
        <xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
                   xmlns:sml="{SML_NAMESPACE}"
                   targetNamespace="{SML_NAMESPACE}"
                   elementFormDefault="qualified">
          <xs:complexType name="SlideType"><xs:sequence/></xs:complexType>
          <xs:complexType name="PresentationType">
            <xs:sequence><xs:element name="slide" type="sml:SlideType"/></xs:sequence>
          </xs:complexType>
          <xs:element name="presentation" type="sml:PresentationType"/>
        </xs:schema>
        """
        with tempfile.TemporaryDirectory() as temp_dir:
            schema_path = Path(temp_dir) / "schema.xsd"
            schema_path.write_text(schema, encoding="utf-8")
            issues = sxsd_validator.validate_sxsd(
                ET.fromstring(f'<slide xmlns="{SML_NAMESPACE}"/>'),
                schema_path,
            )

        self.assertEqual(issues, [])

    def test_standalone_slide_requires_slide_type_in_xsd(self) -> None:
        schema = f"""
        <xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
                   xmlns:sml="{SML_NAMESPACE}"
                   targetNamespace="{SML_NAMESPACE}"
                   elementFormDefault="qualified">
          <xs:complexType name="PresentationType"><xs:sequence/></xs:complexType>
          <xs:element name="presentation" type="sml:PresentationType"/>
        </xs:schema>
        """
        with tempfile.TemporaryDirectory() as temp_dir:
            schema_path = Path(temp_dir) / "schema.xsd"
            schema_path.write_text(schema, encoding="utf-8")
            issues = sxsd_validator.validate_sxsd(
                ET.fromstring(f'<slide xmlns="{SML_NAMESPACE}"/>'),
                schema_path,
            )

        self.assertEqual([issue["code"] for issue in issues], ["sxsd_unexpected_root"])


if __name__ == "__main__":
    unittest.main()
