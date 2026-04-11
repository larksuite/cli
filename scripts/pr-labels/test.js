// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const fs = require('fs');
const { execFileSync } = require('child_process');
const path = require('path');

const samplesPath = path.join(__dirname, 'samples.json');
const indexPath = path.join(__dirname, 'index.js');
const samples = JSON.parse(fs.readFileSync(samplesPath, 'utf8'));

if (!process.env.GITHUB_TOKEN) {
  console.error("❌ Error: GITHUB_TOKEN environment variable is required to run tests without hitting API rate limits.");
  console.error("Please run: GITHUB_TOKEN=$(gh auth token) node scripts/pr-labels/test.js");
  process.exit(1);
}

let passed = 0;
let failed = 0;

for (const sample of samples) {
  try {
    const output = execFileSync(
      process.execPath,
      [indexPath, '--dry-run', '--json', '--pr-url', sample.pr_url],
      { encoding: 'utf8', env: process.env }
    );
    const result = JSON.parse(output);

    const matchLabel = result.label === sample.expected_label;
    
    // Sort before comparing to ignore order
    const actualDomains = (result.businessDomains || []).sort();
    const expectedDomains = (sample.expected_domains || []).map(d => d.replace('domain/', '')).sort();
    
    const matchDomains = JSON.stringify(actualDomains) === JSON.stringify(expectedDomains);

    if (matchLabel && matchDomains) {
      console.log(`✅ Passed: ${sample.name}`);
      passed++;
    } else {
      console.log(`❌ Failed: ${sample.name}`);
      console.log(`   Label expected: ${sample.expected_label}, got: ${result.label}`);
      console.log(`   Domains expected: ${expectedDomains}, got: ${actualDomains}`);
      failed++;
    }
  } catch (e) {
    console.log(`❌ Failed: ${sample.name} (Execution error)`);
    console.error(e.message);
    failed++;
  }
}

console.log(`\nTest Summary: ${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
