#!/usr/bin/env python3
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

"""Generate notices and verify the licenses covered by current automation."""

from __future__ import annotations

import argparse
import dataclasses
import json
import os
import re
import shutil
import stat
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import Iterable


MAX_FILE_BYTES = 2 * 1024 * 1024
MAX_TOTAL_BYTES = 16 * 1024 * 1024
LICENSE_BASENAMES = ("LICENSE", "COPYING")
NOTICE_BASENAMES = ("NOTICE",)
PROHIBITED_LICENSE_WORDS = ("GPL", "LGPL", "AGPL", "SSPL", "GENERAL PUBLIC LICENSE", "SERVER SIDE PUBLIC")
ADDITIONAL_RESTRICTION_PATTERN = re.compile(
    r"\b(?:non[- ]commercial|commercial use (?:is )?(?:prohibited|forbidden|restricted)|not for commercial (?:use|purposes))\b",
    re.IGNORECASE,
)

# SPDX license texts, with copyright headers and the non-operative Apache appendix
# intentionally omitted. Matching requires the complete operative text in order;
# whitespace and quotation-mark variants are normalized below.
LICENSE_TEMPLATES = {
    "MIT": (
        """Permission is hereby granted, free of charge, to any person obtaining a copy of this software and
associated documentation files (the \"Software\"), to deal in the Software without restriction, including
without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the
following conditions:
The above copyright notice and this permission notice shall be included in all copies or substantial
portions of the Software.
THE SOFTWARE IS PROVIDED \"AS IS\", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT
LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO
EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE
USE OR OTHER DEALINGS IN THE SOFTWARE.""",
    ),
    "ISC": (
        """Permission to use, copy, modify, and/or distribute this software for any purpose with or without fee is hereby granted, provided that the above copyright notice and this permission notice appear in all copies.
THE SOFTWARE IS PROVIDED \"AS IS\" AND ISC DISCLAIMS ALL WARRANTIES WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL ISC BE LIABLE FOR ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.""",
        """Permission to use, copy, modify, and/or distribute this software for any purpose with or without fee is hereby granted, provided that the above copyright notice and this permission notice are included in all copies.
THE SOFTWARE IS PROVIDED \"AS IS\" AND THE AUTHOR DISCLAIMS ALL WARRANTIES WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.""",
    ),
    "BSD-2-Clause": (
        """Redistribution and use in source and binary forms, with or without modification, are permitted provided that the following conditions are met:
1. Redistributions of source code must retain the above copyright notice, this list of conditions and the following disclaimer.
2. Redistributions in binary form must reproduce the above copyright notice, this list of conditions and the following disclaimer in the documentation and/or other materials provided with the distribution.
THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS \"AS IS\" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED.
IN NO EVENT SHALL THE COPYRIGHT <<role>> OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.""",
    ),
    "BSD-3-Clause": (
        """Redistribution and use in source and binary forms, with or without modification, are permitted provided that the following conditions are met:
1. Redistributions of source code must retain the above copyright notice, this list of conditions and the following disclaimer.
2. Redistributions in binary form must reproduce the above copyright notice, this list of conditions and the following disclaimer in the documentation and/or other materials provided with the distribution.
3. Neither the name of <<holder>> nor the names of its contributors may be used to endorse or promote products derived from this software without specific prior written permission.
THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS \"AS IS\" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED.
IN NO EVENT SHALL THE COPYRIGHT <<role>> OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.""",
    ),
    "Apache-2.0": (
        """TERMS AND CONDITIONS FOR USE, REPRODUCTION, AND DISTRIBUTION
1. Definitions.
\"License\" shall mean the terms and conditions for use, reproduction, and distribution as defined by Sections 1 through 9 of this document.
\"Licensor\" shall mean the copyright owner or entity authorized by the copyright owner that is granting the License.
\"Legal Entity\" shall mean the union of the acting entity and all other entities that control, are controlled by, or are under common control with that entity. For the purposes of this definition, \"control\" means (i) the power, direct or indirect, to cause the direction or management of such entity, whether by contract or otherwise, or (ii) ownership of fifty percent (50%) or more of the outstanding shares, or (iii) beneficial ownership of such entity.
\"You\" (or \"Your\") shall mean an individual or Legal Entity exercising permissions granted by this License.
\"Source\" form shall mean the preferred form for making modifications, including but not limited to software source code, documentation source, and configuration files.
\"Object\" form shall mean any form resulting from mechanical transformation or translation of a Source form, including but not limited to compiled object code, generated documentation, and conversions to other media types.
\"Work\" shall mean the work of authorship, whether in Source or Object form, made available under the License, as indicated by a copyright notice that is included in or attached to the work (an example is provided in the Appendix below).
\"Derivative Works\" shall mean any work, whether in Source or Object form, that is based on (or derived from) the Work and for which the editorial revisions, annotations, elaborations, or other modifications represent, as a whole, an original work of authorship. For the purposes of this License, Derivative Works shall not include works that remain separable from, or merely link (or bind by name) to the interfaces of, the Work and Derivative Works thereof.
\"Contribution\" shall mean any work of authorship, including the original version of the Work and any modifications or additions to that Work or Derivative Works thereof, that is intentionally submitted to Licensor for inclusion in the Work by the copyright owner or by an individual or Legal Entity authorized to submit on behalf of the copyright owner.
For the purposes of this definition, \"submitted\" means any form of electronic, verbal, or written communication sent to the Licensor or its representatives, including but not limited to communication on electronic mailing lists, source code control systems, and issue tracking systems that are managed by, or on behalf of, the Licensor for the purpose of discussing and improving the Work, but excluding communication that is conspicuously marked or otherwise designated in writing by the copyright owner as \"Not a Contribution.\"
\"Contributor\" shall mean Licensor and any individual or Legal Entity on behalf of whom a Contribution has been received by Licensor and subsequently incorporated within the Work.
2. Grant of Copyright License. Subject to the terms and conditions of this License, each Contributor hereby grants to You a perpetual, worldwide, non-exclusive, no-charge, royalty-free, irrevocable copyright license to reproduce, prepare Derivative Works of, publicly display, publicly perform, sublicense, and distribute the Work and such Derivative Works in Source or Object form.
3. Grant of Patent License.
Subject to the terms and conditions of this License, each Contributor hereby grants to You a perpetual, worldwide, non-exclusive, no-charge, royalty-free, irrevocable (except as stated in this section) patent license to make, have made, use, offer to sell, sell, import, and otherwise transfer the Work, where such license applies only to those patent claims licensable by such Contributor that are necessarily infringed by their Contribution(s) alone or by combination of their Contribution(s) with the Work to which such Contribution(s) was submitted.
If You institute patent litigation against any entity (including a cross-claim or counterclaim in a lawsuit) alleging that the Work or a Contribution incorporated within the Work constitutes direct or contributory patent infringement, then any patent licenses granted to You under this License for that Work shall terminate as of the date such litigation is filed.
4. Redistribution. You may reproduce and distribute copies of the Work or Derivative Works thereof in any medium, with or without modifications, and in Source or Object form, provided that You meet the following conditions:
(a) You must give any other recipients of the Work or Derivative Works a copy of this License; and
(b) You must cause any modified files to carry prominent notices stating that You changed the files; and
(c) You must retain, in the Source form of any Derivative Works that You distribute, all copyright, patent, trademark, and attribution notices from the Source form of the Work, excluding those notices that do not pertain to any part of the Derivative Works; and
(d) If the Work includes a \"NOTICE\" text file as part of its distribution, then any Derivative Works that You distribute must include a readable copy of the attribution notices contained within such NOTICE file, excluding those notices that do not pertain to any part of the Derivative Works, in at least one of the following places: within a NOTICE text file distributed as part of the Derivative Works; within the Source form or documentation, if provided along with the Derivative Works; or, within a display generated by the Derivative Works, if and wherever such third-party notices normally appear.
The contents of the NOTICE file are for informational purposes only and do not modify the License. You may add Your own attribution notices within Derivative Works that You distribute, alongside or as an addendum to the NOTICE text from the Work, provided that such additional attribution notices cannot be construed as modifying the License.
You may add Your own copyright statement to Your modifications and may provide additional or different license terms and conditions for use, reproduction, or distribution of Your modifications, or for any such Derivative Works as a whole, provided Your use, reproduction, and distribution of the Work otherwise complies with the conditions stated in this License.
5. Submission of Contributions. Unless You explicitly state otherwise, any Contribution intentionally submitted for inclusion in the Work by You to the Licensor shall be under the terms and conditions of this License, without any additional terms or conditions. Notwithstanding the above, nothing herein shall supersede or modify the terms of any separate license agreement you may have executed with Licensor regarding such Contributions.
6. Trademarks. This License does not grant permission to use the trade names, trademarks, service marks, or product names of the Licensor, except as required for reasonable and customary use in describing the origin of the Work and reproducing the content of the NOTICE file.
7. Disclaimer of Warranty. Unless required by applicable law or agreed to in writing, Licensor provides the Work (and each Contributor provides its Contributions) on an \"AS IS\" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied, including, without limitation, any warranties or conditions of TITLE, NON-INFRINGEMENT, MERCHANTABILITY, or FITNESS FOR A PARTICULAR PURPOSE.
You are solely responsible for determining the appropriateness of using or redistributing the Work and assume any risks associated with Your exercise of permissions under this License.
8. Limitation of Liability.
In no event and under no legal theory, whether in tort (including negligence), contract, or otherwise, unless required by applicable law (such as deliberate and grossly negligent acts) or agreed to in writing, shall any Contributor be liable to You for damages, including any direct, indirect, special, incidental, or consequential damages of any character arising as a result of this License or out of the use or inability to use the Work (including but not limited to damages for loss of goodwill, work stoppage, computer failure or malfunction, or any and all other commercial damages or losses), even if such Contributor has been advised of the possibility of such damages.
9. Accepting Warranty or Additional Liability. While redistributing the Work or Derivative Works thereof, You may choose to offer, and charge a fee for, acceptance of support, warranty, indemnity, or other liability obligations and/or rights consistent with this License.
However, in accepting such obligations, You may act only on Your own behalf and on Your sole responsibility, not on behalf of any other Contributor, and only if You agree to indemnify, defend, and hold each Contributor harmless for any liability incurred by, or claims asserted against, such Contributor by reason of your accepting any such warranty or additional liability.""",
    ),
}
RELEASE_TARGETS = (
    ("darwin", "amd64"),
    ("darwin", "arm64"),
    ("linux", "amd64"),
    ("linux", "arm64"),
    ("linux", "riscv64"),
    ("windows", "amd64"),
    ("windows", "arm64"),
)
NODE_OS_ALIASES = {"windows": "win32"}
NODE_CPU_ALIASES = {"amd64": "x64"}
NODE_RELEASE_TARGETS = tuple(
    (NODE_OS_ALIASES.get(goos, goos), NODE_CPU_ALIASES.get(goarch, goarch)) for goos, goarch in RELEASE_TARGETS
)


class NoticeError(RuntimeError):
    """A dependency cannot be safely included in a notices document."""


@dataclasses.dataclass(frozen=True)
class Component:
    name: str
    version: str
    source: str
    license_id: str
    copyright: str
    license_text: str
    notice_text: str = ""


@dataclasses.dataclass
class ReadBudget:
    total: int = 0

    def charge(self, size: int) -> None:
        if size > MAX_FILE_BYTES:
            raise NoticeError(f"dependency file exceeds {MAX_FILE_BYTES} byte limit")
        self.total += size
        if self.total > MAX_TOTAL_BYTES:
            raise NoticeError(f"dependency files exceed {MAX_TOTAL_BYTES} byte total limit")


def _within(root: Path, target: Path) -> bool:
    try:
        return os.path.commonpath((str(root), str(target))) == str(root)
    except ValueError:
        return False


def _validate_dependency_path(root: Path, path: Path) -> Path:
    """Reject symlinks and paths that resolve outside a dependency root."""
    root = root.absolute()
    path = path.absolute()
    if not _within(root, path):
        raise NoticeError(f"dependency path escapes its root: {path}")
    try:
        relative = path.relative_to(root)
    except ValueError as error:
        raise NoticeError(f"dependency path escapes its root: {path}") from error

    current = root
    for part in (".", *relative.parts):
        if part != ".":
            current = current / part
        try:
            mode = os.lstat(current).st_mode
        except OSError as error:
            raise NoticeError(f"cannot lstat dependency path: {current}") from error
        if stat.S_ISLNK(mode):
            raise NoticeError(f"symlinked dependency path is not allowed: {current}")

    resolved_root = Path(os.path.realpath(root))
    resolved_path = Path(os.path.realpath(path))
    if not _within(resolved_root, resolved_path):
        raise NoticeError(f"dependency path resolves outside its root: {path}")
    return resolved_path


def safe_read_text(root: Path, path: Path, budget: ReadBudget) -> str:
    """Read one UTF-8 dependency file after containment and size validation."""
    resolved = _validate_dependency_path(root, path)
    try:
        file_stat = os.lstat(resolved)
    except OSError as error:
        raise NoticeError(f"cannot lstat dependency file: {path}") from error
    if not stat.S_ISREG(file_stat.st_mode):
        raise NoticeError(f"dependency file is not a regular file: {path}")
    budget.charge(file_stat.st_size)
    try:
        with open(resolved, "rb") as handle:
            data = handle.read(MAX_FILE_BYTES + 1)
    except OSError as error:
        raise NoticeError(f"cannot read dependency file: {path}") from error
    if len(data) != file_stat.st_size or len(data) > MAX_FILE_BYTES:
        raise NoticeError(f"dependency file changed while reading: {path}")
    try:
        return data.decode("utf-8")
    except UnicodeDecodeError as error:
        raise NoticeError(f"dependency file is not valid UTF-8: {path}") from error


def _read_json(root: Path, path: Path, budget: ReadBudget) -> dict:
    try:
        value = json.loads(safe_read_text(root, path, budget))
    except json.JSONDecodeError as error:
        raise NoticeError(f"invalid JSON in dependency metadata: {path}") from error
    if not isinstance(value, dict):
        raise NoticeError(f"dependency metadata is not an object: {path}")
    return value


def _allowed_document_name(name: str, basenames: Iterable[str]) -> bool:
    upper_name = name.upper()
    for basename in basenames:
        if not upper_name.startswith(basename):
            continue
        if upper_name == basename:
            return True
        suffix = name[len(basename):]
        if suffix.lower() in (".md", ".txt"):
            return True
        if suffix.startswith("-") and re.fullmatch(r"-[A-Za-z0-9._-]+", suffix):
            return True
    return False


def _document_text(root: Path, basenames: Iterable[str], budget: ReadBudget, required: bool) -> str:
    documents = []
    _validate_dependency_path(root, root)
    try:
        candidates = sorted(
            (path for path in root.iterdir() if _allowed_document_name(path.name, basenames)),
            key=lambda path: path.name,
        )
    except OSError as error:
        raise NoticeError(f"cannot list dependency documents: {root}") from error
    for candidate in candidates:
        # lstat is deliberately performed even when this is the final path.
        try:
            os.lstat(candidate)
        except FileNotFoundError:
            continue
        except OSError as error:
            raise NoticeError(f"cannot inspect dependency document: {candidate}") from error
        documents.append(safe_read_text(root, candidate, budget))
    if required and not documents:
        raise NoticeError(f"missing required license document in {root}")
    return "\n\n".join(documents)


def _reject_prohibited(value: str) -> None:
    normalized = value.upper()
    if any(word in normalized for word in PROHIBITED_LICENSE_WORDS):
        raise NoticeError(f"prohibited license: {value}")


def _warn_additional_restriction(name: str, version: str, license_text: str) -> None:
    """Keep the original text, while drawing attention to obvious added restrictions."""
    if ADDITIONAL_RESTRICTION_PATTERN.search(license_text):
        print(
            f"third_party_notices: warning: {name}@{version} contains a possible additional license restriction; "
            "the original text is included in THIRD_PARTY_NOTICES.md and should be reviewed",
            file=sys.stderr,
        )


def _normalized_license_text(value: str) -> str:
    without_list_markers = re.sub(r"(?m)^\s*(?:\d+\.|\*)\s+", "", value)
    return re.sub(r"[\"“”]", "", re.sub(r"\s+", " ", without_list_markers)).casefold()


def _matches_standard_template(license_id: str, license_text: str) -> bool:
    """Require a complete SPDX template, not merely a collection of headings."""
    normalized = _normalized_license_text(license_text)
    for template in LICENSE_TEMPLATES[license_id]:
        normalized_template = _normalized_license_text(template)
        if license_id.startswith("BSD-"):
            pattern = re.escape(normalized_template)
            pattern = pattern.replace(re.escape("<<holder>>"), r".+?")
            pattern = pattern.replace(re.escape("<<role>>"), r"(?:holder|owner)")
            if re.search(pattern, normalized):
                return True
        elif normalized_template in normalized:
            return True
    return False


def _detect_bsd_license(license_text: str) -> str:
    normalized = _normalized_license_text(license_text)
    if "all advertising materials" in normalized:
        raise NoticeError("unsupported BSD-4-Clause license")
    if _matches_standard_template("BSD-3-Clause", license_text):
        return "BSD-3-Clause"
    if _matches_standard_template("BSD-2-Clause", license_text):
        return "BSD-2-Clause"
    raise NoticeError("incomplete BSD-2-Clause or BSD-3-Clause license")


def _detect_license_ids(license_text: str) -> set[str]:
    _reject_prohibited(license_text)
    normalized = _normalized_license_text(license_text)
    detected = set()
    if _matches_standard_template("Apache-2.0", license_text):
        detected.add("Apache-2.0")
    if _matches_standard_template("MIT", license_text):
        detected.add("MIT")
    if _matches_standard_template("ISC", license_text):
        detected.add("ISC")
    if "redistribution and use in source and binary forms" in normalized:
        detected.add(_detect_bsd_license(license_text))
    return detected


def normalize_license_id(value: object, license_text: str) -> str:
    """Return a currently auto-verified SPDX-like identifier, or fail closed."""
    declared = ""
    if isinstance(value, str):
        declared = value.strip()
    elif isinstance(value, dict) and isinstance(value.get("type"), str):
        declared = value["type"].strip()
    detected = _detect_license_ids(license_text)
    if declared:
        _reject_prohibited(declared)
        normalized = declared.upper().replace(" ", "")
        if normalized in {"MIT", "MITLICENSE"}:
            expected = "MIT"
        elif normalized in {"ISC", "ISCLICENSE"}:
            expected = "ISC"
        elif normalized.startswith("APACHE-2") or normalized in {"APACHE2.0", "APACHELICENSE2.0"}:
            expected = "Apache-2.0"
        elif normalized.startswith("BSD-2"):
            expected = "BSD-2-Clause"
        elif normalized.startswith("BSD-3"):
            expected = "BSD-3-Clause"
        elif normalized in {"BSD", "BSDLICENSE"}:
            return _detect_bsd_license(license_text)
        else:
            raise NoticeError(
                f"license requires manual review; "
                f"it is outside the current automated verification set: {declared}"
            )
        if expected not in detected:
            raise NoticeError(f"license text does not match declared license: {declared}")
        return expected
    if not detected:
        raise NoticeError(
            "license requires manual review; "
            "it cannot be identified from dependency metadata or text"
        )
    return " OR ".join(sorted(detected))


def _copyright_lines(*texts: str) -> str:
    lines = []
    for text in texts:
        for line in text.splitlines():
            stripped = line.strip()
            normalized = stripped.lower()
            if normalized.startswith(("copyright notice", "copyright license")):
                continue
            if normalized.startswith("copyright") or stripped.startswith("©"):
                lines.append(stripped)
    return "\n".join(dict.fromkeys(lines)) or "Not specified"


def _repository_source(metadata: dict, fallback: str) -> str:
    repository = metadata.get("repository")
    if isinstance(repository, dict):
        repository = repository.get("url")
    if not isinstance(repository, str) or not repository.strip():
        return fallback
    source = repository.strip()
    if source.startswith("git+"):
        source = source[4:]
    if source.endswith(".git"):
        source = source[:-4]
    return source


def _component_from_package(
    package_dir: Path, budget: ReadBudget, *, name: str, version: str, source: str, declared_license: object
) -> Component:
    license_text = _document_text(package_dir, LICENSE_BASENAMES, budget, required=True)
    notice_text = _document_text(package_dir, NOTICE_BASENAMES, budget, required=False)
    try:
        license_id = normalize_license_id(declared_license, license_text)
    except NoticeError as error:
        raise NoticeError(f"{name}@{version}: {error}") from error
    _warn_additional_restriction(name, version, license_text)
    return Component(
        name=name,
        version=version,
        source=source,
        license_id=license_id,
        copyright=_copyright_lines(license_text, notice_text),
        license_text=license_text,
        notice_text=notice_text,
    )


def component_from_node_package(package_dir: Path, budget: ReadBudget) -> Component:
    metadata = _read_json(package_dir, package_dir / "package.json", budget)
    name, version = metadata.get("name"), metadata.get("version")
    if not isinstance(name, str) or not name or not isinstance(version, str) or not version:
        raise NoticeError(f"missing name or version in dependency metadata: {package_dir}")
    return _component_from_package(
        package_dir,
        budget,
        name=name,
        version=version,
        source=_repository_source(metadata, f"https://www.npmjs.com/package/{name}/v/{version}"),
        declared_license=metadata.get("license"),
    )


def component_from_go_module(module: dict, budget: ReadBudget) -> Component:
    name, version, directory = module.get("Path"), module.get("Version"), module.get("Dir")
    if not isinstance(name, str) or not isinstance(version, str) or not isinstance(directory, str):
        raise NoticeError("go list returned a module with missing path, version, or directory")
    return _component_from_package(
        Path(directory), budget, name=name, version=version, source=f"https://pkg.go.dev/{name}@{version}", declared_license=None
    )


def _parse_json_stream(value: str) -> list[dict]:
    decoder = json.JSONDecoder()
    position = 0
    records = []
    while position < len(value):
        while position < len(value) and value[position].isspace():
            position += 1
        if position == len(value):
            break
        try:
            record, position = decoder.raw_decode(value, position)
        except json.JSONDecodeError as error:
            raise NoticeError("invalid JSON from Go command") from error
        if not isinstance(record, dict):
            raise NoticeError("invalid module record from Go command")
        records.append(record)
    return records


def _go_runtime_module_records(repo_root: Path) -> list[dict]:
    modules: dict[tuple[str, str], dict] = {}
    with tempfile.TemporaryDirectory(prefix="third-party-notices-go-") as temporary:
        temp_root = Path(temporary)
        _copy_input_file(repo_root, temp_root, "go.mod")
        _copy_input_file(repo_root, temp_root, "go.sum")
        modfile = temp_root / "go.mod"
        for goos, goarch in RELEASE_TARGETS:
            environment = dict(os.environ, CGO_ENABLED="0", GOOS=goos, GOARCH=goarch)
            try:
                result = subprocess.run(
                    ["go", "list", "-mod=mod", f"-modfile={modfile}", "-deps", "-json", "."],
                    cwd=repo_root,
                    capture_output=True,
                    text=True,
                    check=True,
                    env=environment,
                )
            except (OSError, subprocess.CalledProcessError) as error:
                raise NoticeError(f"go list failed for {goos}/{goarch}") from error
            for package in _parse_json_stream(result.stdout):
                module = package.get("Module")
                if not isinstance(module, dict) or module.get("Main"):
                    continue
                name, version = module.get("Path"), module.get("Version")
                if not isinstance(name, str) or not isinstance(version, str):
                    raise NoticeError(f"go list returned invalid module metadata for {goos}/{goarch}")
                modules[(name, version)] = module
    if not modules:
        raise NoticeError("go list did not find any third-party runtime modules")
    return [modules[key] for key in sorted(modules)]


def collect_go_components(repo_root: Path, budget: ReadBudget) -> list[Component]:
    records = _go_runtime_module_records(repo_root)
    missing_directories = [record for record in records if not isinstance(record.get("Dir"), str)]
    if missing_directories:
        modules = ", ".join(f"{record.get('Path')}@{record.get('Version')}" for record in missing_directories)
        raise NoticeError(f"go list did not locate module source: {modules}")
    return [component_from_go_module(record, budget) for record in records]


def _copy_input_file(repo_root: Path, destination: Path, name: str) -> None:
    source = repo_root / name
    _validate_dependency_path(repo_root, source)
    try:
        shutil.copy2(source, destination / name)
    except OSError as error:
        raise NoticeError(f"cannot copy {name} into isolated npm directory") from error


def _node_package_directories(node_modules: Path) -> Iterable[Path]:
    _validate_dependency_path(node_modules, node_modules)
    for current, directories, filenames in os.walk(node_modules, topdown=True, followlinks=False):
        current_path = Path(current)
        kept = []
        for directory in directories:
            path = current_path / directory
            try:
                is_link = stat.S_ISLNK(os.lstat(path).st_mode)
            except OSError as error:
                raise NoticeError(f"cannot inspect npm dependency directory: {path}") from error
            if is_link:
                raise NoticeError(f"symlinked npm dependency directory is not allowed: {path}")
            kept.append(directory)
        directories[:] = kept
        if "package.json" not in filenames:
            continue
        parent = current_path.parent
        is_unscoped = parent.name == "node_modules"
        is_scoped = parent.parent.name == "node_modules" and parent.name.startswith("@")
        if is_unscoped or is_scoped:
            yield current_path


def collect_node_components(repo_root: Path, budget: ReadBudget) -> list[Component]:
    with tempfile.TemporaryDirectory(prefix="third-party-notices-") as temporary:
        temp_root = Path(temporary)
        components: dict[tuple[str, str, str], Component] = {}
        for node_os, node_cpu in NODE_RELEASE_TARGETS:
            target_root = temp_root / f"{node_os}-{node_cpu}"
            target_root.mkdir()
            _copy_input_file(repo_root, target_root, "package.json")
            _copy_input_file(repo_root, target_root, "package-lock.json")
            try:
                subprocess.run(
                    [
                        "npm",
                        "ci",
                        "--ignore-scripts",
                        "--omit=dev",
                        f"--os={node_os}",
                        f"--cpu={node_cpu}",
                    ],
                    cwd=target_root,
                    capture_output=True,
                    text=True,
                    check=True,
                )
            except (OSError, subprocess.CalledProcessError) as error:
                raise NoticeError(f"npm ci failed for {node_os}/{node_cpu}") from error
            for directory in _node_package_directories(target_root / "node_modules"):
                component = component_from_node_package(directory, budget)
                key = (component.name, component.version, component.source)
                existing = components.get(key)
                if existing and existing != component:
                    raise NoticeError(
                        f"npm dependency metadata differs across release targets: {component.name}@{component.version}"
                    )
                components[key] = component
        return [components[key] for key in sorted(components)]


def render_notices(components: Iterable[Component]) -> str:
    lines = ["# Third-Party Notices", ""]
    for component in sorted(components, key=lambda item: (item.name.lower(), item.name, item.version, item.source)):
        lines.extend((
            f"## {component.name} {component.version}",
            "",
            f"- Component: {component.name}",
            f"- Version: {component.version}",
            f"- Source: {component.source}",
            f"- License: {component.license_id}",
            f"- Copyright: {component.copyright}",
            "",
            "### License Text",
            "```text",
            component.license_text.rstrip("\n"),
            "```",
        ))
        if component.notice_text:
            lines.extend(("", "### NOTICE", "```text", component.notice_text.rstrip("\n"), "```"))
        lines.append("")
    return "\n".join(lines)


def generate(repo_root: Path, output: Path) -> str:
    budget = ReadBudget()
    components = collect_go_components(repo_root, budget) + collect_node_components(repo_root, budget)
    document = render_notices(components)
    try:
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(document, encoding="utf-8", newline="\n")
    except OSError as error:
        raise NoticeError(f"cannot write output: {output}") from error
    return document


def check(repo_root: Path, output: Path) -> None:
    if not output.is_file():
        raise NoticeError(f"notices output does not exist: {output}")
    with tempfile.TemporaryDirectory(prefix="third-party-notices-check-") as temporary:
        generated = Path(temporary) / "THIRD_PARTY_NOTICES.md"
        generate(repo_root, generated)
        try:
            expected = output.read_bytes()
            actual = generated.read_bytes()
        except OSError as error:
            raise NoticeError("cannot read notices output for comparison") from error
    if actual != expected:
        raise NoticeError("third-party notices are out of date; run generate with --output")


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("generate", "check"))
    parser.add_argument("--output", required=True, type=Path, help="explicit path to THIRD_PARTY_NOTICES.md")
    parser.add_argument("--repo-root", type=Path, default=Path(__file__).resolve().parent.parent)
    args = parser.parse_args(argv)
    try:
        if args.command == "generate":
            generate(args.repo_root.resolve(), args.output.resolve())
        else:
            check(args.repo_root.resolve(), args.output.resolve())
    except NoticeError as error:
        print(f"third_party_notices: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
