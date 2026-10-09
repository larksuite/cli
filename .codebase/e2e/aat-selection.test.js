// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { execFileSync } = require('node:child_process');
const { selectWorkflows } = require('./aat-selection');
const { checkResults } = require('./run-aat');
const prefix = 'github.com/larksuite/cli/tests/cli_e2e/';

test('real Go discovery includes new packages, unions explicit tests, and never executes them', () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'aat-discovery-'));
  try {
    fs.writeFileSync(path.join(root, 'go.mod'), 'module github.com/larksuite/cli\n\ngo 1.23\n');
    const packages = {
      old: ['TestLegacy', 'TestShared_AATCompatible', 'TestUserOnly'],
      added: ['TestNew_AATCompatible', 'TestLegacy', 'TestUserOnly'],
      demo: ['TestDemo_AATCompatible'],
    };
    for (const [pkg, names] of Object.entries(packages)) {
      const dir = path.join(root, 'tests/cli_e2e', pkg);
      fs.mkdirSync(dir, { recursive: true });
      fs.writeFileSync(path.join(dir, 'case_test.go'), `package ${pkg}\nimport "testing"\n` +
        names.map(name => `func ${name}(t *testing.T) { t.Fatal("discovery must not execute a test") }`).join('\n'));
    }
    const listed = execFileSync('go', ['test', '-json', '-list=^Test', '-run=^$', '-p=1', './tests/cli_e2e/...'],
      { cwd: root, env: { ...process.env, GOWORK: 'off' }, encoding: 'utf8', timeout: 60000 });
    const selection = selectWorkflows(listed, { [`${prefix}old`]: ['TestLegacy', 'TestShared_AATCompatible'] });
    assert.deepEqual(selection, {
      [`${prefix}added`]: ['TestNew_AATCompatible'],
      [`${prefix}old`]: ['TestLegacy', 'TestShared_AATCompatible'],
    });
    const passed = Object.entries(selection).flatMap(([Package, names]) => names.map(Test => JSON.stringify({ Package, Test, Action: 'pass' })));
    assert.equal(checkResults(passed.join('\n'), selection).passed, true);
    assert.equal(checkResults(passed.slice(1).join('\n'), selection).passed, false);
    assert.throws(() => selectWorkflows(listed, { [`${prefix}old`]: ['TestRenamed'] }), /missing/);
  } finally { fs.rmSync(root, { recursive: true, force: true }); }
});

test('empty, failed, malformed and incomplete discovery cannot produce a green gate', () => {
  assert.throws(() => selectWorkflows('', {}), /No AAT/);
  assert.throws(() => selectWorkflows('not json', {}));
  assert.throws(() => selectWorkflows(JSON.stringify({ Action: 'fail', Package: `${prefix}old` }), {}), /discovery failed/);
  assert.throws(() => selectWorkflows(JSON.stringify({ Action: 'output', Package: `${prefix}old`, Output: 'TestFoo_AATCompatible\n' }), {}), /missing/);
  assert.equal(checkResults('', {}).passed, false);
});
