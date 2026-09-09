// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Shipped as scripts/postinstall.js of the internal package. The package
// carries a binary for every platform; this removes the ones this machine
// cannot run so a global install does not keep seven copies. Best effort only:
// a failure here must never fail the install, the launcher only needs its own
// binary to be present.

const fs = require("fs");
const path = require("path");
const { binaryName } = require("./run.js");

function prune(binDir = path.join(__dirname, "..", "bin"), keep = binaryName()) {
  let removed = [];
  try {
    for (const file of fs.readdirSync(binDir)) {
      if (file === keep) continue;
      fs.rmSync(path.join(binDir, file), { force: true });
      removed.push(file);
    }
  } catch (_) {
    // Missing directory or a read-only install: leave everything in place.
  }
  return removed;
}

module.exports = { prune };

if (require.main === module) prune();
