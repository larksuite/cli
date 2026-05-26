// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const { describe, it } = require("node:test");
const assert = require("node:assert/strict");

process.env.LARK_CLI_INSTALL_WIZARD_TEST = "1";
const {
  skillsAddArgs,
  skillsAddCommand,
  skillsScopeFlag,
} = require("./install-wizard.js");

describe("install wizard skills scope", () => {
  it("keeps global skills install by default", () => {
    assert.equal(skillsScopeFlag(false), "-g");
    assert.deepEqual(
      skillsAddArgs("larksuite/cli", false),
      ["-y", "skills", "add", "larksuite/cli", "-y", "-g"],
    );
  });

  it("uses project scope when requested", () => {
    assert.equal(skillsScopeFlag(true), "-p");
    assert.deepEqual(
      skillsAddArgs("larksuite/cli", true),
      ["-y", "skills", "add", "larksuite/cli", "-y", "-p"],
    );
  });

  it("prints retry command with matching scope", () => {
    assert.equal(
      skillsAddCommand("larksuite/cli", true),
      "npx -y skills add larksuite/cli -y -p",
    );
  });
});
