#!/usr/bin/env node
/**
 * OneRole Unified E2E Test Suite Runner
 * CLI: node tests/e2e/runner.js [--tier=1|2|3|4] [--all] [--filter=<regex>] [--timeout=<ms>] [--bail] [--verbose] [--json]
 * Exit Code: 0 on 100% pass and thresholds met; non-zero on failure.
 */

const path = require('path');
const setup = require('./helpers/setup');
const mockAts = require('./helpers/mock_ats');

const SUITES = [
  { tier: 1, name: 'Tier 1: Feature Coverage', file: './tier1_feature_test.js', minThreshold: 50 },
  { tier: 2, name: 'Tier 2: Boundary & Corner Cases', file: './tier2_boundary_test.js', minThreshold: 50 },
  { tier: 3, name: 'Tier 3: Pairwise Combinations', file: './tier3_combination_test.js', minThreshold: 15 },
  { tier: 4, name: 'Tier 4: Real-World Scenarios', file: './tier4_realworld_test.js', minThreshold: 8 }
];

// CLI Argument Parsing
function parseArgs() {
  const args = process.argv.slice(2);
  const options = {
    tier: null,
    filter: null,
    bail: false,
    json: false,
    verbose: false,
    timeout: 30000
  };

  for (let i = 0; i < args.length; i++) {
    const arg = args[i];
    if (arg === '--all' || arg === '-a') {
      options.tier = null;
    } else if (arg.startsWith('--tier=') || arg.startsWith('-t=')) {
      options.tier = parseInt(arg.split('=')[1], 10);
    } else if (arg === '-t' || arg === '--tier') {
      if (i + 1 < args.length && !args[i + 1].startsWith('-')) {
        options.tier = parseInt(args[++i], 10);
      }
    } else if (arg.startsWith('--filter=')) {
      options.filter = new RegExp(arg.split('=')[1], 'i');
    } else if (arg === '--filter' || arg === '-f') {
      if (i + 1 < args.length && !args[i + 1].startsWith('-')) {
        options.filter = new RegExp(args[++i], 'i');
      }
    } else if (arg.startsWith('--timeout=')) {
      options.timeout = parseInt(arg.split('=')[1], 10);
    } else if (arg === '--timeout') {
      if (i + 1 < args.length && !args[i + 1].startsWith('-')) {
        options.timeout = parseInt(args[++i], 10);
      }
    } else if (arg === '--bail' || arg === '-b') {
      options.bail = true;
    } else if (arg === '--json') {
      options.json = true;
    } else if (arg === '--verbose' || arg === '-v') {
      options.verbose = true;
    }
  }
  return options;
}

class TestContext {
  constructor(suiteName, tier, options) {
    this.name = suiteName;
    this.suiteName = suiteName;
    this.tier = tier;
    this.options = options;
    this.tests = [];
    this.passed = 0;
    this.failed = 0;
    this.skipped = 0;
    this.failures = [];
    this.startTime = Date.now();
    this.beforeEachHooks = [];
    this.afterEachHooks = [];
  }

  beforeEach(fn) {
    if (typeof fn === 'function') this.beforeEachHooks.push(fn);
  }

  afterEach(fn) {
    if (typeof fn === 'function') this.afterEachHooks.push(fn);
  }

  async test(name, fn) {
    if (this.options.filter && !this.options.filter.test(name)) {
      this.skipped++;
      return;
    }
    if (this.options.bail && this.failed > 0) {
      this.skipped++;
      return;
    }

    const start = Date.now();
    const timeoutMs = this.options.timeout || 30000;
    let timer;
    const timeoutPromise = new Promise((_, reject) => {
      timer = setTimeout(() => reject(new Error(`Test timed out after ${timeoutMs}ms`)), timeoutMs);
    });

    try {
      for (const hook of this.beforeEachHooks) {
        await hook();
      }

      await Promise.race([
        Promise.resolve().then(() => fn()),
        timeoutPromise
      ]);
      const duration = Date.now() - start;
      this.passed++;
      this.tests.push({ name, status: 'pass', duration });
      if (!this.options.json) {
        console.log(`    ✅ [PASS] ${name} (${duration}ms)`);
      }
    } catch (err) {
      const duration = Date.now() - start;
      this.failed++;
      const failure = { name, status: 'fail', duration, error: err.message, stack: err.stack };
      this.tests.push(failure);
      this.failures.push(failure);
      if (!this.options.json) {
        console.error(`    ❌ [FAIL] ${name} (${duration}ms): ${err.message}`);
        if (this.options.verbose && err.stack) {
          console.error(`       Stack Trace:\n${err.stack.split('\n').map(l => '         ' + l).join('\n')}`);
        }
      }
    } finally {
      clearTimeout(timer);
      for (const hook of this.afterEachHooks) {
        try {
          await hook();
        } catch (hErr) {
          if (!this.options.json) console.error(`    ⚠️  [afterEach error]: ${hErr.message}`);
        }
      }
      try {
        if (typeof mockAts.clearSubmissions === 'function') {
          mockAts.clearSubmissions();
        } else if (typeof mockAts.resetScenario === 'function') {
          mockAts.resetScenario();
        }
      } catch (_) {}
    }
  }

  async benchmark(name, maxDurationMs, fn) {
    return this.test(`[Benchmark] ${name}`, async () => {
      const bStart = Date.now();
      await fn();
      const bDuration = Date.now() - bStart;
      if (bDuration > maxDurationMs) {
        throw new Error(`Latency benchmark '${name}' exceeded threshold: ${bDuration}ms > ${maxDurationMs}ms`);
      }
    });
  }
}

async function runMasterSuite() {
  const options = parseArgs();
  const startTime = Date.now();

  if (!options.json) {
    console.log('================================================================================');
    console.log('🚀 ONEROLE E2E VERIFICATION TEST RUNNER');
    console.log('================================================================================');
    console.log(`Execution Mode: ${options.tier ? 'Tier ' + options.tier : 'All Tiers (1-4)'}`);
    console.log(`Filter: ${options.filter ? options.filter : 'None'} | Bail: ${options.bail} | Timeout: ${options.timeout}ms | Verbose: ${options.verbose}`);
    console.log('--------------------------------------------------------------------------------\n');
  }

  // Pre-flight Environment & Health Check
  const health = await setup.healthCheck();
  if (!options.json) {
    console.log('--- Pre-Flight Environment Status ---');
    console.log(`  MongoDB (27017):       ${health.mongodb ? 'ONLINE' : 'OFFLINE'}`);
    console.log(`  NATS JetStream (4222): ${health.nats ? 'ONLINE' : 'OFFLINE'}`);
    console.log(`  Backend API (8088):    ${health.backend ? 'ONLINE' : 'OFFLINE'}`);
    console.log(`  UI Microservice (8082): ${health.ui ? 'ONLINE' : 'OFFLINE'}`);
    console.log(`  Worker Daemon (3001):  ${health.worker ? 'ONLINE' : 'OFFLINE'}`);
    console.log(`  Chrome Binary:         ${health.chrome ? 'FOUND' : 'MISSING'}\n`);
  }

  // Start Mock ATS Server on Port 9876
  try {
    await mockAts.start();
  } catch (e) {
    console.error('Warning starting Mock ATS:', e.message);
  }

  const selectedSuites = SUITES.filter(s => options.tier === null || s.tier === options.tier);
  const suiteResults = [];

  try {
    for (const suiteDef of selectedSuites) {
      if (!options.json) {
        console.log(`\n================================================================================`);
        console.log(`📦 Running ${suiteDef.name} (Min Required: ${suiteDef.minThreshold})`);
        console.log(`================================================================================`);
      }

      const suitePath = path.resolve(__dirname, suiteDef.file);
      let suiteModule;
      try {
        suiteModule = require(suitePath);
      } catch (loadErr) {
        console.error(`Failed to load suite ${suiteDef.file}:`, loadErr);
        suiteResults.push({
          tier: suiteDef.tier,
          name: suiteDef.name,
          passed: 0,
          failed: 1,
          total: 1,
          thresholdMet: false,
          failures: [{ name: 'Load Suite', error: loadErr.message, stack: loadErr.stack }]
        });
        continue;
      }

      mockAts.clearSubmissions();
      const ctx = new TestContext(suiteDef.name, suiteDef.tier, options);
      try {
        if (typeof suiteModule.runSuite === 'function') {
          await suiteModule.runSuite(ctx);
        }
      } finally {
        mockAts.clearSubmissions();
      }

      ctx.totalDuration = Date.now() - ctx.startTime;
      ctx.total = ctx.passed + ctx.failed;
      // If filtering, don't fail threshold check if all executed tests passed
      ctx.thresholdMet = options.filter ? (ctx.failed === 0) : (ctx.passed >= suiteDef.minThreshold && ctx.failed === 0);

      suiteResults.push(ctx);

      if (!options.json) {
        console.log(`\n  Tier ${suiteDef.tier} Summary: ${ctx.passed}/${ctx.total} Passed (${ctx.totalDuration}ms) | Threshold (≥${suiteDef.minThreshold}): ${ctx.thresholdMet ? 'MET' : 'FAILED'}`);
      }

      if (options.bail && ctx.failed > 0) {
        break;
      }
    }
  } finally {
    // Teardown Mock ATS Server and DB connections
    try { await mockAts.stop(); } catch (e) {}
    try { await setup.teardown(); } catch (e) {}
  }

  // Aggregate Reporting
  const totalPassed = suiteResults.reduce((acc, s) => acc + s.passed, 0);
  const totalFailed = suiteResults.reduce((acc, s) => acc + s.failed, 0);
  const totalTests = totalPassed + totalFailed;
  const allThresholdsMet = suiteResults.every(s => s.thresholdMet);
  const grandDuration = Date.now() - startTime;

  if (options.json) {
    console.log(JSON.stringify({
      passed: totalPassed,
      failed: totalFailed,
      total: totalTests,
      duration: grandDuration,
      allThresholdsMet,
      suites: suiteResults
    }, null, 2));
  } else {
    console.log('\n================================================================================');
    console.log('📊 FINAL TEST SUITE EXECUTION SUMMARY');
    console.log('================================================================================');
    console.log(' Tier | Suite Name                          | Passed | Failed | Total | Threshold');
    console.log('------+-------------------------------------+--------+--------+-------+----------');
    for (const s of suiteResults) {
      const tStr = `Tier ${s.tier}`.padEnd(5);
      const nameStr = s.name.padEnd(35);
      const passStr = String(s.passed).padStart(6);
      const failStr = String(s.failed).padStart(6);
      const totStr = String(s.total).padStart(5);
      const thStr = s.thresholdMet ? '  MET  ' : ' FAILED ';
      console.log(` ${tStr}| ${nameStr} | ${passStr} | ${failStr} | ${totStr} | ${thStr}`);
    }
    console.log('------+-------------------------------------+--------+--------+-------+----------');
    console.log(` TOTAL| All Executed Tiers                  | ${String(totalPassed).padStart(6)} | ${String(totalFailed).padStart(6)} | ${String(totalTests).padStart(5)} | ${allThresholdsMet ? '  MET  ' : ' FAILED '}`);
    console.log('================================================================================');
    console.log(`Duration: ${grandDuration}ms | Success Rate: ${totalTests > 0 ? Math.round((totalPassed / totalTests) * 100) : 0}%`);

    if (totalFailed > 0) {
      console.log(`\n❌ FAILED CHECKS (${totalFailed}):`);
      for (const s of suiteResults) {
        for (const f of s.failures) {
          console.log(`   - [Tier ${s.tier}] ${f.name}: ${f.error}`);
          if (options.verbose && f.stack) {
            console.log(`     ${f.stack.split('\n').join('\n     ')}`);
          }
        }
      }
    } else if (allThresholdsMet) {
      console.log('\n🎉 ALL VERIFICATION CRITERIA AND COVERAGE THRESHOLDS SATISFIED WITH 100% PASS!');
    }
    console.log('================================================================================\n');
  }

  if (totalFailed > 0 || !allThresholdsMet) {
    process.exit(1);
  } else {
    process.exit(0);
  }
}

if (require.main === module) {
  runMasterSuite().catch(err => {
    console.error('Fatal Runner Error:', err);
    process.exit(1);
  });
}

module.exports = { runMasterSuite, SUITES, parseArgs, TestContext };
