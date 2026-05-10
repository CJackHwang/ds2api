'use strict';

/**
 * Tool-call Go/Node parity driver.
 *
 * Runs the same fixture set used by TestGoCompatToolCallFixtures (Go) through
 * the Node-side parseToolCallsDetailed, then compares against the shared
 * expected files in tests/compat/expected/.
 *
 * Any discrepancy between Go and Node output will cause this test to fail,
 * making cross-language divergence visible in CI.
 */

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const {
  parseToolCallsDetailed,
} = require('../../internal/js/helpers/stream-tool-sieve');

const ROOT = path.resolve(__dirname, '..', '..');
const FIXTURES_ROOT = path.join(ROOT, 'tests', 'compat', 'fixtures', 'toolcalls');
const EXPECTED_ROOT = path.join(ROOT, 'tests', 'compat', 'expected');

// Walk fixtures/toolcalls/**/*.json and collect { fixturePath, expectedPath, name }
function collectFixtures(dir, relPrefix) {
  const entries = fs.readdirSync(dir, { withFileTypes: true });
  const cases = [];
  for (const entry of entries) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      const subRel = relPrefix ? `${relPrefix}/${entry.name}` : entry.name;
      cases.push(...collectFixtures(full, subRel));
    } else if (entry.name.endsWith('.json')) {
      const baseName = entry.name.replace(/\.json$/, '');
      const relName = relPrefix ? `${relPrefix}/${baseName}` : baseName;
      // e.g. true_positive/dsml_single_cdata_read
      //   -> toolcalls_true_positive_dsml_single_cdata_read.json
      const expectedFile = 'toolcalls_' + relName.replace(/\//g, '_') + '.json';
      cases.push({
        name: relName,
        fixturePath: full,
        expectedPath: path.join(EXPECTED_ROOT, expectedFile),
      });
    }
  }
  return cases;
}

if (!fs.existsSync(FIXTURES_ROOT)) {
  throw new Error(`toolcall-parity: fixture directory not found: ${FIXTURES_ROOT}`);
}

const fixtures = collectFixtures(FIXTURES_ROOT, '');

if (fixtures.length === 0) {
  throw new Error(`toolcall-parity: no fixture files found under ${FIXTURES_ROOT}`);
}

for (const { name, fixturePath, expectedPath } of fixtures) {
  test(`toolcall parity: ${name}`, () => {
    const fix = JSON.parse(fs.readFileSync(fixturePath, 'utf8'));
    const expected = JSON.parse(fs.readFileSync(expectedPath, 'utf8'));

    const result = parseToolCallsDetailed(fix.text, fix.tool_names || []);

    // Normalise: ensure calls[].input defaults to {} when absent (matches Go behaviour)
    const gotCalls = (result.calls || []).map(c => ({
      name: c.name,
      input: (c.input && typeof c.input === 'object' && !Array.isArray(c.input))
        ? c.input
        : {},
    }));
    const wantCalls = (expected.calls || []).map(c => ({
      name: c.name,
      input: c.input || {},
    }));

    assert.strictEqual(
      result.sawToolCallSyntax,
      expected.sawToolCallSyntax,
      `[${name}] sawToolCallSyntax mismatch: node=${result.sawToolCallSyntax} expected=${expected.sawToolCallSyntax}`,
    );

    assert.strictEqual(
      result.rejectedByPolicy,
      expected.rejectedByPolicy,
      `[${name}] rejectedByPolicy mismatch: node=${result.rejectedByPolicy} expected=${expected.rejectedByPolicy}`,
    );

    assert.strictEqual(
      gotCalls.length,
      wantCalls.length,
      `[${name}] calls.length mismatch: node=${gotCalls.length} expected=${wantCalls.length}\n` +
      `  node calls:     ${JSON.stringify(gotCalls)}\n` +
      `  expected calls: ${JSON.stringify(wantCalls)}`,
    );

    for (let i = 0; i < wantCalls.length; i++) {
      assert.strictEqual(
        gotCalls[i].name,
        wantCalls[i].name,
        `[${name}] calls[${i}].name mismatch: node=${gotCalls[i].name} expected=${wantCalls[i].name}`,
      );
      assert.deepEqual(
        gotCalls[i].input,
        wantCalls[i].input,
        `[${name}] calls[${i}].input mismatch:\n` +
        `  node:     ${JSON.stringify(gotCalls[i].input)}\n` +
        `  expected: ${JSON.stringify(wantCalls[i].input)}`,
      );
    }

    const gotRejected = (result.rejectedToolNames || []).slice().sort();
    const wantRejected = (expected.rejectedToolNames || []).slice().sort();
    assert.deepEqual(
      gotRejected,
      wantRejected,
      `[${name}] rejectedToolNames mismatch: node=${JSON.stringify(gotRejected)} expected=${JSON.stringify(wantRejected)}`,
    );
  });
}
