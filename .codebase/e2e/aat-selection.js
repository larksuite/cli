// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
'use strict';
const { CredentialError } = require('./fetch-uat');

// Consume Go's package-aware test listing, never source text or test execution.
function selectWorkflows(events, explicit) {
  const available = new Map();
  const completed = new Set();
  for (const line of events.split('\n').filter(Boolean)) {
    const event = JSON.parse(line);
    if (event.Action === 'fail') throw new CredentialError('AAT test discovery failed');
    const pkg = event.Package;
    if (typeof pkg !== 'string' || !/^github\.com\/larksuite\/cli\/tests\/cli_e2e\/(?!demo(?:\/|$))[A-Za-z0-9_/-]+$/.test(pkg)) continue;
    if (event.Action === 'pass' && !event.Test) completed.add(pkg);
    if (event.Action !== 'output' || typeof event.Output !== 'string') continue;
    for (const name of event.Output.trim().split('\n')) {
      if (!/^Test[A-Z0-9_][A-Za-z0-9_]*$/.test(name)) continue;
      if (!available.has(pkg)) available.set(pkg, new Set());
      available.get(pkg).add(name);
    }
  }
  const selected = new Map();
  function add(pkg, name) {
    if (!completed.has(pkg) || !available.get(pkg)?.has(name)) {
      throw new CredentialError('An explicitly selected AAT test is missing from the Go test listing');
    }
    if (!selected.has(pkg)) selected.set(pkg, new Set());
    selected.get(pkg).add(name);
  }
  for (const [pkg, names] of Object.entries(explicit)) {
    for (const name of names) add(pkg, name);
  }
  for (const [pkg, names] of available) {
    for (const name of names) if (name.endsWith('_AATCompatible')) add(pkg, name);
  }
  if (selected.size === 0) throw new CredentialError('No AAT workflows selected');
  return Object.fromEntries([...selected].sort(([a], [b]) => a.localeCompare(b))
    .map(([pkg, names]) => [pkg, [...names].sort()]));
}

module.exports = { selectWorkflows };
