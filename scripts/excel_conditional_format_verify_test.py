#!/usr/bin/env python3
"""Run the shipped conditional-format gate using only the Python standard library."""
import json
import subprocess
import sys
import tempfile
import unittest
import zipfile
from pathlib import Path


SCRIPT = Path(__file__).resolve().parents[1] / "skills/lark-sheets/scripts/excel_conditional_format_verify.py"
NS = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
REL_NS = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"


def workbook(path, pattern, target="worksheets/sheet1.xml", dxf_id=0,
             strict=False, styles_target="styles.xml", extension="", invalid_root=False):
    parts = {
        "[Content_Types].xml": '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">'
        '<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>'
        '<Default Extension="xml" ContentType="application/xml"/>'
        '<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>'
        '<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>'
        '<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>',
        "_rels/.rels": '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
        '<Relationship Id="rId1" Type="' + REL_NS + '/officeDocument" Target="xl/workbook.xml"/></Relationships>',
        "xl/workbook.xml": '<workbook xmlns="' + NS + '" xmlns:r="' + REL_NS + '">'
        '<sheets><sheet name="甘特图" sheetId="1" r:id="rId1"/></sheets></workbook>',
        "xl/_rels/workbook.xml.rels": '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
        '<Relationship Id="rId1" Type="' + REL_NS + '/worksheet" Target="' + target + '"/>'
        '<Relationship Id="rId2" Type="' + REL_NS + '/styles" Target="styles.xml"/></Relationships>',
        "xl/styles.xml": '<styleSheet xmlns="' + NS + '">'
        '<fills count="1"><fill><patternFill patternType="solid"><fgColor rgb="FF66BB6A"/></patternFill></fill></fills>'
        '<dxfs count="1"><dxf><fill>' + pattern + '</fill></dxf></dxfs></styleSheet>',
        "xl/worksheets/sheet1.xml": '<worksheet xmlns="' + NS + '"><sheetData/>'
        '<conditionalFormatting sqref="I5:BP40"><cfRule type="expression" priority="1" dxfId="' + str(dxf_id) + '">'
        '<formula>TRUE</formula></cfRule></conditionalFormatting></worksheet>',
    }
    if strict:
        parts = {name: text.replace(NS, "http://purl.oclc.org/ooxml/spreadsheetml/main").replace(
            REL_NS, "http://purl.oclc.org/ooxml/officeDocument/relationships")
            for name, text in parts.items()}
    if styles_target != "styles.xml":
        part = styles_target.lstrip("/") if styles_target.startswith("/") else "xl/" + styles_target
        parts[part] = parts.pop("xl/styles.xml")
        parts["xl/_rels/workbook.xml.rels"] = parts["xl/_rels/workbook.xml.rels"].replace(
            'Target="styles.xml"', 'Target="' + styles_target + '"')
        parts["[Content_Types].xml"] = parts["[Content_Types].xml"].replace("/xl/styles.xml", "/" + part)
    if extension:
        parts["xl/worksheets/sheet1.xml"] = parts["xl/worksheets/sheet1.xml"].replace(
            "</worksheet>", extension + "</worksheet>")
    if invalid_root:
        parts["xl/workbook.xml"] = "<not-workbook/>"
    with zipfile.ZipFile(path, "w") as archive:
        for name, text in parts.items():
            archive.writestr(name, text.encode("utf-8"))


class ConditionalFormatGateTest(unittest.TestCase):
    def run_gate(self, path, cwd):
        proc = subprocess.run([sys.executable, "-S", str(SCRIPT), str(path)],
                              cwd=cwd, capture_output=True, text=True)
        self.assertEqual(proc.stderr, "")
        return proc.returncode, json.loads(proc.stdout)

    def test_foreground_only_fill_fails_from_an_unrelated_directory(self):
        with tempfile.TemporaryDirectory() as td:
            path = Path(td) / "gantt.xlsx"
            for target in ("worksheets/sheet1.xml", "/xl/worksheets/sheet1.xml"):
                workbook(path, '<patternFill patternType="solid"><fgColor rgb="0066BB6A"/></patternFill>', target)
                code, result = self.run_gate(path, td)
                self.assertEqual(code, 3)
                hit, = result["violations"]
                self.assertEqual(hit["kind"], "conditional_format_fill_compatibility")
                self.assertEqual(hit["location"], "甘特图!I5:BP40")
                self.assertEqual(hit["actual"]["fgColor"], {"rgb": "0066BB6A"})

    def test_ordinary_fills_and_supported_conditional_styles_pass(self):
        patterns = (
            '<patternFill patternType="solid"><bgColor rgb="FF66BB6A"/></patternFill>',
            '<patternFill patternType="solid"><fgColor rgb="FF66BB6A"/><bgColor rgb="FF66BB6A"/></patternFill>',
            '<patternFill patternType="darkGrid"><fgColor rgb="FF66BB6A"/></patternFill>',
            '<patternFill patternType="solid"/>',
        )
        with tempfile.TemporaryDirectory() as td:
            path = Path(td) / "gantt.xlsx"
            for pattern in patterns:
                with self.subTest(pattern=pattern):
                    workbook(path, pattern)
                    code, result = self.run_gate(path, td)
                    self.assertEqual(code, 0)
                    self.assertEqual(result, {"status": "success", "violations": []})

    def test_invalid_input_is_an_error(self):
        with tempfile.TemporaryDirectory() as td:
            path = Path(td) / "bad.xlsx"
            for content in (None, b"not a zip"):
                if content is not None:
                    path.write_bytes(content)
                code, result = self.run_gate(path, td)
                self.assertEqual((code, result["status"]), (2, "error"))
            workbook(path, '<patternFill/>', dxf_id=3)
            code, result = self.run_gate(path, td)
            self.assertEqual((code, result["status"]), (2, "error"))

    def test_strict_namespace_is_scanned_and_invalid_roots_fail(self):
        with tempfile.TemporaryDirectory() as td:
            path = Path(td) / "strict.xlsx"
            workbook(path, '<patternFill patternType="solid"><fgColor rgb="FF66BB6A"/></patternFill>', strict=True)
            code, result = self.run_gate(path, td)
            self.assertEqual((code, len(result["violations"])), (3, 1))
            workbook(path, '<patternFill/>', invalid_root=True)
            code, result = self.run_gate(path, td)
            self.assertEqual((code, result["status"]), (2, "error"))

    def test_custom_styles_relationship_is_followed(self):
        with tempfile.TemporaryDirectory() as td:
            path = Path(td) / "styles.xlsx"
            for target in ("styles/custom.xml", "/xl/styles/custom.xml"):
                for background, expected in (("", 3), ('<bgColor rgb="FF66BB6A"/>', 0)):
                    workbook(path, '<patternFill patternType="solid"><fgColor rgb="FF66BB6A"/>'
                             + background + '</patternFill>', styles_target=target)
                    code, _ = self.run_gate(path, td)
                    self.assertEqual(code, expected)

    def test_extension_rules_are_reported_as_unverified(self):
        with tempfile.TemporaryDirectory() as td:
            path = Path(td) / "extension.xlsx"
            extension = '<extLst><ext uri="test"><x14:conditionalFormattings '
            extension += 'xmlns:x14="http://schemas.microsoft.com/office/spreadsheetml/2009/9/main">'
            extension += '<x14:conditionalFormatting><x14:cfRule type="expression" priority="1">'
            extension += '<x14:dxf><fill><patternFill patternType="solid"><fgColor rgb="FF66BB6A"/>'
            extension += '</patternFill></fill></x14:dxf></x14:cfRule></x14:conditionalFormatting>'
            extension += '</x14:conditionalFormattings></ext></extLst>'
            workbook(path, '<patternFill/>', extension=extension)
            code, result = self.run_gate(path, td)
            self.assertEqual((code, result["status"]), (2, "error"))
            self.assertIn("x14", result["error"])


if __name__ == "__main__":
    unittest.main()
