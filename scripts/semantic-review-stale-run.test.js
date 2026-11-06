// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;
const workflow = fs.readFileSync('.github/workflows/semantic-review.yml', 'utf8');

for (const name of ['Verify workflow run and pull request for summary', 'Verify workflow run and pull request']) {
  const step = workflow.split(`      - name: ${name}\n`)[1].split(/\n      - (?:name|uses):/)[0];
  const script = step.split('          script: |\n')[1].split('\n').filter(line => line.startsWith('            ')).map(line => line.slice(12)).join('\n');
  const execute = new AsyncFunction('github', 'context', 'core', script);

  async function run(candidates, sourceId = 2) {
    const outputs = {};
    const context = { repo: { owner: 'example', repo: 'cli' }, payload: {
      repository: { id: 1, full_name: 'example/cli' },
      workflow_run: { id: 10, workflow_id: 20, path: '.github/workflows/ci.yml', event: 'pull_request', conclusion: 'success', repository: { id: 1, full_name: 'example/cli' }, head_sha: 'a'.repeat(40), head_repository: { id: sourceId }, head_branch: 'feature', pull_requests: [] },
    }};
    const github = { rest: {
      actions: { getWorkflow: async () => ({ data: { name: 'CI', path: '.github/workflows/ci.yml' } }), listWorkflowRunArtifacts: async () => ({ data: { artifacts: [] } }) },
      repos: { listPullRequestsAssociatedWithCommit: async () => ({ data: [] }) },
      pulls: { list: () => {}, get: async () => ({ data: candidates[0] }) },
    }, paginate: async () => candidates };
    await execute(github, context, { notice: () => {}, warning: () => {}, setOutput: (key, value) => { outputs[key] = value; } });
    return outputs;
  }
  const candidate = { number: 42, state: 'open', base: { repo: { id: 1 }, sha: 'c'.repeat(40) }, head: { repo: { id: 2, name: 'cli', owner: { login: 'contributor' } }, ref: 'feature', sha: 'b'.repeat(40) } };
  test(`${name}: current exact head still resolves`, async () => {
    const outputs = await run([{ ...candidate, head: { ...candidate.head, sha: 'a'.repeat(40) } }]);
    assert.equal(outputs.stale, 'false');
    assert.equal(outputs.head_sha, 'a'.repeat(40));
    assert.equal(outputs.pr_number, '42');
  });
  test(`${name}: wrong base repository is not trusted`, async () => {
    await assert.rejects(run([{ ...candidate, base: { ...candidate.base, repo: { id: 3 } } }]), /expected one open PR/);
  });
  test(`${name}: superseded head skips without publishing target outputs`, async () => {
    assert.deepEqual(await run([candidate]), { stale: 'true' });
  });
  test(`${name}: closed superseded target skips`, async () => {
    assert.deepEqual(await run([{ ...candidate, state: 'closed' }]), { stale: 'true' });
  });
  test(`${name}: branch name in another repository is not trusted`, async () => {
    await assert.rejects(run([candidate], 3), /expected one open PR/);
  });
  test(`${name}: ambiguous source branches fail closed`, async () => {
    await assert.rejects(run([candidate, { ...candidate, number: 43 }]), /expected one open PR/);
  });
  test(`${name}: missing source identity fails closed`, async () => {
    await assert.rejects(run([candidate], null), /expected one open PR/);
  });
  test(`${name}: unmatched run fails closed`, async () => {
    await assert.rejects(run([]), /expected one open PR/);
  });
}
