// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const fs = require("fs");
const path = require("path");

const { classifyIssueText } = require("./index.js");

const samplesPath = path.join(__dirname, "samples.json");
const samples = JSON.parse(fs.readFileSync(samplesPath, "utf8"));

/**
 * Convert an array-like value into a sorted string array.
 *
 * @param {Array<unknown>|undefined|null} arr
 * @returns {string[]}
 */
function sortArray(arr) {
  return (arr || []).map(String).sort();
}

/**
 * Check whether every element in sub exists in sup.
 *
 * @param {string[]} sub
 * @param {string[]} sup
 * @returns {boolean}
 */
function isSubset(sub, sup) {
  const set = new Set(sup || []);
  for (const x of sub || []) {
    if (!set.has(x)) return false;
  }
  return true;
}

let passed = 0;
let failed = 0;

for (const sample of samples) {
  try {
    const result = classifyIssueText(sample.title, sample.body);

    const hasExpectedType = Object.prototype.hasOwnProperty.call(sample, "expected_type");
    const expectedType = hasExpectedType ? sample.expected_type : undefined;
    const matchType = hasExpectedType ? (result.type || null) === expectedType : true;
    const actualDomains = sortArray(result.domains);
    const expectedDomains = sortArray(sample.expected_domains);
    const hasExpectedDomains = Object.prototype.hasOwnProperty.call(sample, "expected_domains");
    const matchDomains = !hasExpectedDomains
      ? true
      : expectedDomains.length === 0
        ? actualDomains.length === 0
        : isSubset(expectedDomains, actualDomains);

    if (matchType && matchDomains) {
      console.log(`✅ Passed: ${sample.name}`);
      passed += 1;
    } else {
      console.log(`❌ Failed: ${sample.name}`);
      console.log(`   Type expected: ${expectedType}, got: ${result.type}`);
      console.log(`   Domains expected(subset): ${expectedDomains}, got: ${actualDomains}`);
      failed += 1;
    }
  } catch (e) {
    console.log(`❌ Failed: ${sample.name} (Execution error)`);
    console.error(e && e.message ? e.message : String(e));
    failed += 1;
  }
}

console.log(`\nTest Summary: ${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
