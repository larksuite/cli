#!/usr/bin/env python3
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
"""Compatibility entry point for the renamed xml_lint_test script."""

import os
import sys
from pathlib import Path


if __name__ == "__main__":
    target = Path(__file__).with_name("xml_lint_test.py")
    os.execv(sys.executable, [sys.executable, str(target), *sys.argv[1:]])
