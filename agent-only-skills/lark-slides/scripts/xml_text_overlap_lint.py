#!/usr/bin/env python3
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
"""Compatibility entry point for the renamed xml_lint module."""

import sys

from xml_lint import *  # noqa: F401,F403
from xml_lint import XmlLayoutLintError, run_cli


if __name__ == "__main__":
    try:
        run_cli()
    except XmlLayoutLintError as error:
        print(f"xml-text-overlap-lint error: {error}", file=sys.stderr)
        raise SystemExit(1) from error
