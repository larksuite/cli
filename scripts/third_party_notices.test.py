#!/usr/bin/env python3
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

"""Behavior tests for the third-party notice generator."""

import importlib.util
import io
import json
import sys
import tempfile
from pathlib import Path
from unittest import TestCase, main, mock


SCRIPT = Path(__file__).with_name("third_party_notices.py")
SPEC = importlib.util.spec_from_file_location("third_party_notices", SCRIPT)
notices = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
sys.modules[SPEC.name] = notices
SPEC.loader.exec_module(notices)

MIT_TEXT = notices.LICENSE_TEMPLATES["MIT"][0]
APACHE_TEXT = notices.LICENSE_TEMPLATES["Apache-2.0"][0]
BSD_2_TEXT = notices.LICENSE_TEMPLATES["BSD-2-Clause"][0].replace("<<role>>", "HOLDER")
BSD_3_TEXT = (
    notices.LICENSE_TEMPLATES["BSD-3-Clause"][0]
    .replace("<<holder>>", "Example Authors")
    .replace("<<role>>", "HOLDER")
)


def make_package(root: Path, name: str, license_name: str, license_text: str) -> Path:
    package = root / name
    package.mkdir()
    (package / "package.json").write_text(
        '{"name": "' + name + '", "version": "1.0.0", "license": "' + license_name + '"}',
        encoding="utf-8",
    )
    (package / "LICENSE").write_text(license_text, encoding="utf-8")
    return package


class ThirdPartyNoticesTests(TestCase):
    def test_render_sorts_components_stably(self):
        components = [
            notices.Component("zeta", "2.0.0", "https://z", "MIT", "Copyright Z", "z text"),
            notices.Component("alpha", "1.0.0", "https://a", "ISC", "Copyright A", "a text"),
            notices.Component("alpha", "0.9.0", "https://a", "ISC", "Copyright A", "old text"),
        ]

        rendered = notices.render_notices(components)

        self.assertLess(rendered.index("## alpha 0.9.0"), rendered.index("## alpha 1.0.0"))
        self.assertLess(rendered.index("## alpha 1.0.0"), rendered.index("## zeta 2.0.0"))

    def test_apache_notice_is_preserved_in_component(self):
        with tempfile.TemporaryDirectory() as directory:
            package = make_package(Path(directory), "apache-package", "Apache-2.0", APACHE_TEXT)
            (package / "NOTICE").write_text("Example Apache NOTICE\n", encoding="utf-8")

            component = notices.component_from_node_package(package, notices.ReadBudget())

        self.assertEqual(component.license_id, "Apache-2.0")
        self.assertEqual(component.notice_text, "Example Apache NOTICE\n")

    def test_license_outside_automated_set_fails_closed(self):
        with tempfile.TemporaryDirectory() as directory:
            package = make_package(Path(directory), "unknown-package", "Proprietary", MIT_TEXT)

            with self.assertRaises(notices.NoticeError):
                notices.component_from_node_package(package, notices.ReadBudget())

    def test_declared_license_must_match_its_text(self):
        with self.assertRaises(notices.NoticeError):
            notices.normalize_license_id("MIT", APACHE_TEXT)

    def test_bsd_license_is_classified_by_its_text(self):
        self.assertEqual(notices.normalize_license_id("BSD", BSD_2_TEXT), "BSD-2-Clause")
        self.assertEqual(notices.normalize_license_id("BSD", BSD_3_TEXT), "BSD-3-Clause")

    def test_truncated_approved_license_text_fails_closed(self):
        cases = (
            ("MIT", "Permission is hereby granted, free of charge"),
            ("Apache-2.0", "Apache License\nVersion 2.0"),
            ("BSD-2-Clause", "Redistribution and use in source and binary forms"),
            ("ISC", "Permission to use, copy, modify, and/or distribute"),
        )

        for declared, text in cases:
            with self.subTest(declared=declared):
                with self.assertRaises(notices.NoticeError):
                    notices.normalize_license_id(declared, text)

    def test_apache_headings_without_standard_terms_fail_closed(self):
        headings_only = """Apache License Version 2.0
Terms and Conditions for Use, Reproduction, and Distribution
1. Definitions.
2. Grant of Copyright License.
3. Grant of Patent License.
4. Redistribution.
5. Submission of Contributions.
6. Trademarks.
7. Disclaimer of Warranty.
8. Limitation of Liability.
9. Accepting Warranty or Additional Liability.
END OF TERMS AND CONDITIONS"""

        with self.assertRaises(notices.NoticeError):
            notices.normalize_license_id("Apache-2.0", headings_only)

    def test_additional_restriction_warns_without_blocking_notice_generation(self):
        with tempfile.TemporaryDirectory() as directory:
            package = make_package(
                Path(directory), "restricted-package", "MIT", MIT_TEXT + "\nCommercial use is prohibited.\n"
            )
            stderr = io.StringIO()

            with mock.patch.object(notices.sys, "stderr", stderr):
                component = notices.component_from_node_package(package, notices.ReadBudget())

        self.assertEqual(component.license_id, "MIT")
        self.assertIn("possible additional license restriction", stderr.getvalue())

    def test_copyright_clauses_are_not_rendered_as_attribution(self):
        self.assertEqual(
            notices._copyright_lines(
                "Copyright 2024 Example\nCopyright notice, this list of conditions must be retained\n© 2025 Another"
            ),
            "Copyright 2024 Example\n© 2025 Another",
        )

    def test_runtime_module_collection_deduplicates_release_targets(self):
        module = {"Path": "example.com/runtime", "Version": "v1.2.3", "Dir": "/tmp/runtime"}
        main = {"Path": "github.com/larksuite/cli", "Main": True}
        output = "\n".join(json.dumps(value) for value in (main, {"Module": module}))
        completed = mock.Mock(stdout=output)

        with tempfile.TemporaryDirectory() as directory:
            repo = Path(directory)
            (repo / "go.mod").write_text("module example.com/project\n", encoding="utf-8")
            (repo / "go.sum").write_text("", encoding="utf-8")
            with mock.patch.object(notices.subprocess, "run", return_value=completed) as run:
                records = notices._go_runtime_module_records(repo)

        self.assertEqual(records, [module])
        self.assertEqual(run.call_count, len(notices.RELEASE_TARGETS))
        self.assertEqual(run.call_args.kwargs["env"]["CGO_ENABLED"], "0")

    def test_node_component_collection_scans_and_deduplicates_release_targets(self):
        common = notices.Component("common", "1.0.0", "https://common.example", "MIT", "Copyright", "text")
        darwin_only = notices.Component("darwin-only", "1.0.0", "https://darwin.example", "MIT", "Copyright", "text")

        def package_directories(node_modules: Path):
            target = node_modules.parent.name
            yield node_modules / "common"
            if target == "darwin-arm64":
                yield node_modules / "darwin-only"

        def component_from_package(directory: Path, budget: notices.ReadBudget):
            if directory.name == "darwin-only":
                return darwin_only
            return common

        with tempfile.TemporaryDirectory() as directory:
            repo = Path(directory)
            (repo / "package.json").write_text('{"name":"example"}', encoding="utf-8")
            (repo / "package-lock.json").write_text('{"lockfileVersion":3}', encoding="utf-8")
            with mock.patch.object(notices.subprocess, "run", return_value=mock.Mock()) as run, \
                    mock.patch.object(notices, "_node_package_directories", side_effect=package_directories), \
                    mock.patch.object(notices, "component_from_node_package", side_effect=component_from_package):
                components = notices.collect_node_components(repo, notices.ReadBudget())

        self.assertEqual(components, [common, darwin_only])
        self.assertEqual(run.call_count, len(notices.NODE_RELEASE_TARGETS))
        self.assertEqual(
            {tuple(call.args[0][-2:]) for call in run.call_args_list},
            {(f"--os={node_os}", f"--cpu={node_cpu}") for node_os, node_cpu in notices.NODE_RELEASE_TARGETS},
        )

    def test_missing_module_directory_fails_closed(self):
        module = {"Path": "example.com/runtime", "Version": "v1.2.3"}
        with tempfile.TemporaryDirectory() as directory:
            repo = Path(directory)
            with mock.patch.object(notices, "_go_runtime_module_records", return_value=[module]):
                with self.assertRaisesRegex(notices.NoticeError, "example.com/runtime@v1.2.3"):
                    notices.collect_go_components(repo, notices.ReadBudget())

    def test_license_hyphen_variant_is_accepted(self):
        with tempfile.TemporaryDirectory() as directory:
            package = make_package(Path(directory), "renamed-license-package", "MIT", MIT_TEXT)
            (package / "LICENSE").rename(package / "LICENSE-MIT")

            component = notices.component_from_node_package(package, notices.ReadBudget())

        self.assertEqual(component.license_id, "MIT")

    def test_symlinked_license_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            package = make_package(root, "symlink-package", "MIT", MIT_TEXT)
            outside = root / "outside-license"
            outside.write_text(MIT_TEXT, encoding="utf-8")
            (package / "LICENSE").unlink()
            (package / "LICENSE").symlink_to(outside)

            with self.assertRaises(notices.NoticeError):
                notices.component_from_node_package(package, notices.ReadBudget())

    def test_path_outside_dependency_root_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "dependency"
            root.mkdir()
            outside = Path(directory) / "outside"
            outside.write_text("outside", encoding="utf-8")

            with self.assertRaises(notices.NoticeError):
                notices.safe_read_text(root, outside, notices.ReadBudget())

    def test_oversize_license_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            package = make_package(Path(directory), "large-package", "MIT", MIT_TEXT)
            (package / "LICENSE").write_bytes(b"x" * (notices.MAX_FILE_BYTES + 1))

            with self.assertRaises(notices.NoticeError):
                notices.component_from_node_package(package, notices.ReadBudget())

    def test_total_read_limit_is_shared_between_components(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            budget = notices.ReadBudget()
            text = MIT_TEXT + "x" * (notices.MAX_FILE_BYTES - 200_000)
            packages = [make_package(root, f"package-{index}", "MIT", text) for index in range(9)]

            for package in packages[:8]:
                notices.component_from_node_package(package, budget)
            with self.assertRaises(notices.NoticeError):
                notices.component_from_node_package(packages[8], budget)

    def test_invalid_utf8_license_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            package = make_package(Path(directory), "bad-utf8-package", "MIT", MIT_TEXT)
            (package / "LICENSE").write_bytes(b"\xff\xfe\x00")

            with self.assertRaises(notices.NoticeError):
                notices.component_from_node_package(package, notices.ReadBudget())

    def test_check_compares_without_mutating_the_output(self):
        component = notices.Component("example", "1.0.0", "https://example.invalid", "MIT", "Copyright", "text")
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "THIRD_PARTY_NOTICES.md"
            expected = notices.render_notices([component]).encode("utf-8")
            output.write_bytes(expected)
            before = output.stat()
            with mock.patch.object(notices, "collect_go_components", return_value=[component]), \
                    mock.patch.object(notices, "collect_node_components", return_value=[]):
                notices.check(Path(directory), output)
            after = output.stat()
            self.assertEqual(output.read_bytes(), expected)

        self.assertEqual(before.st_mtime_ns, after.st_mtime_ns)


if __name__ == "__main__":
    main()
