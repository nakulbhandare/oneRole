/**
 * worker_daemon_empirical_test.js
 * 
 * Milestone 2 Empirical Challenge Test Harness for:
 * connectors/ats_submit/worker_daemon.js
 * 
 * Tests and stress-tests:
 * 1. Retry logic on transient errors (msg.nak with exponential delay and jitter)
 * 2. Terminal error categorization (invalid JSON, invalid schema, non-retryable errors -> msg.term)
 * 3. Retry exhaustion (deliveryCount >= MAX_RETRIES -> msg.term('max_retries_exceeded'))
 * 4. Per-job watchdog timeout (JOB_TIMEOUT_MS rejection & heartbeat timer cleanup)
 * 5. Backoff calculation fuzzing (10,000 samples across delivery counts 1-10 with bounds & jitter verification)
 * 6. Concurrency slot leak stress test (100 interleaved mixed-outcome jobs -> activeJobs returns to 0)
 */

const fs = require('fs');
const path = require('path');
const assert = require('assert');
const { StringCodec } = require('nats');

const sc = StringCodec();
const TARGET_FILE = path.resolve(__dirname, '../worker_daemon.js');

if (!fs.existsSync(TARGET_FILE)) {
  console.error(`Target file does not exist: ${TARGET_FILE}`);
  process.exit(1);
}

const daemonSource = fs.readFileSync(TARGET_FILE, 'utf8');

// Extract top-level helpers and processMessage from the source file
const helpersMatch = daemonSource.match(/(function isCaptchaOrBot[\s\S]*?)(?=async function start)/);
if (!helpersMatch) {
  throw new Error('Failed to extract helper functions from worker_daemon.js');
}
const processMsgMatch = daemonSource.match(/(async function processMessage\(msg\)[\s\S]*?\n  \})/);
if (!processMsgMatch) {
  throw new Error('Failed to extract processMessage from worker_daemon.js');
}

/**
 * Creates an isolated execution harness for processMessage and daemon helper functions
 */
function createHarness(options = {}) {
  const config = {
    MAX_RETRIES: options.MAX_RETRIES !== undefined ? options.MAX_RETRIES : 3,
    JOB_TIMEOUT_MS: options.JOB_TIMEOUT_MS !== undefined ? options.JOB_TIMEOUT_MS : 120000,
    HEARTBEAT_INTERVAL_MS: options.HEARTBEAT_INTERVAL_MS !== undefined ? options.HEARTBEAT_INTERVAL_MS : 15000,
    BASE_BACKOFF_MS: options.BASE_BACKOFF_MS !== undefined ? options.BASE_BACKOFF_MS : 3000,
    MAX_BACKOFF_MS: options.MAX_BACKOFF_MS !== undefined ? options.MAX_BACKOFF_MS : 30000
  };

  const dbUpdates = [];
  const events = [];
  const state = { activeJobs: 0, jobsProcessed: 0 };

  const appsColl = {
    updateOne: async (query, update) => {
      dbUpdates.push({ query, update, timestamp: new Date() });
      return { matchedCount: 1, modifiedCount: 1 };
    }
  };

  const eventsColl = {
    insertOne: async (doc) => {
      events.push({ ...doc, inserted_at: new Date() });
      return { insertedId: doc._id };
    }
  };

  const submitApplication = options.submitApplication || (async () => ({
    receipt_id: 'rec_mock_success',
    confirmation_url: 'https://example.com/confirmation',
    provider: 'ashby'
  }));

  const factory = new Function(
    'sc', 'appsColl', 'eventsColl', 'submitApplication', 'config', 'state',
    `
    let { MAX_RETRIES, JOB_TIMEOUT_MS, HEARTBEAT_INTERVAL_MS, BASE_BACKOFF_MS, MAX_BACKOFF_MS } = config;
    let { activeJobs, jobsProcessed } = state;
    ${helpersMatch[1]}
    ${processMsgMatch[1]}
    return {
      processMessage,
      calculateBackoffDelay,
      isCaptchaOrBot,
      isNonRetryableError,
      getState: () => ({ activeJobs, jobsProcessed })
    };
    `
  );

  const instance = factory(sc, appsColl, eventsColl, submitApplication, config, state);

  return {
    ...instance,
    dbUpdates,
    events,
    config
  };
}

/**
 * Creates a mock NATS JetStream message
 */
function createMockMsg(payload, options = {}) {
  let encodedData;
  if (typeof payload === 'string') {
    encodedData = sc.encode(payload);
  } else if (payload instanceof Uint8Array) {
    encodedData = payload;
  } else {
    encodedData = sc.encode(JSON.stringify(payload));
  }

  const calls = {
    ack: 0,
    working: 0,
    nak: [],
    term: []
  };

  return {
    data: encodedData,
    info: { deliveryCount: options.deliveryCount !== undefined ? options.deliveryCount : 1 },
    ack: () => { calls.ack++; },
    working: () => { calls.working++; },
    nak: (delay) => { calls.nak.push(delay); },
    term: (reason) => { calls.term.push(reason); },
    calls
  };
}

const standardPayload = {
  application_id: 'app-standard-001',
  tenant_id: 'tenant-enterprise',
  user_id: 'user-nakul',
  job_id: 'job-swe-101',
  job_url: 'https://jobs.ashbyhq.com/techcorp/swe-backend',
  candidate: {
    name: 'Nakul Bhandare',
    email: 'bhandarenakul11@gmail.com',
    location: 'San Francisco, CA'
  }
};

let testsRun = 0;
let testsPassed = 0;

function runTest(name, fn) {
  testsRun++;
  try {
    const result = fn();
    if (result && typeof result.then === 'function') {
      return result.then(() => {
        testsPassed++;
        console.log(`  ✓ ${name}`);
      }).catch(err => {
        console.error(`  ✗ ${name}`);
        console.error(err);
        process.exitCode = 1;
      });
    } else {
      testsPassed++;
      console.log(`  ✓ ${name}`);
      return Promise.resolve();
    }
  } catch (err) {
    console.error(`  ✗ ${name}`);
    console.error(err);
    process.exitCode = 1;
    return Promise.resolve();
  }
}

async function main() {
  console.log('================================================================');
  console.log('Worker Daemon Empirical Challenge & Stress Test Suite');
  console.log(`Target: ${TARGET_FILE}`);
  console.log('================================================================\n');

  // ==========================================================================
  // Suite 1: Retry Logic on Transient Errors
  // ==========================================================================
  console.log('--- Suite 1: Retry Logic on Transient Errors ---');

  const transientErrors = [
    new Error('Navigation timeout of 30000 ms exceeded'),
    new Error('net::ERR_CONNECTION_RESET at https://jobs.lever.co'),
    new Error('net::ERR_CONNECTION_REFUSED'),
    new Error('Protocol error (Page.navigate): Target closed'),
    new Error('Waiting for selector `#submit-button` failed: timeout 30000ms exceeded'),
    new Error('Session closed. Most likely the page has been closed')
  ];

  for (const err of transientErrors) {
    await runTest(`Transient error "${err.message.slice(0, 40)}..." invokes msg.nak(backoffDelay) on attempt 1`, async () => {
      const harness = createHarness({
        submitApplication: async () => { throw err; }
      });
      const msg = createMockMsg(standardPayload, { deliveryCount: 1 });

      await harness.processMessage(msg);

      assert.strictEqual(msg.calls.nak.length, 1, 'msg.nak should be called exactly once');
      assert.strictEqual(msg.calls.term.length, 0, 'msg.term should NOT be called');
      assert.strictEqual(msg.calls.ack, 0, 'msg.ack should NOT be called');

      const delay = msg.calls.nak[0];
      assert.strictEqual(typeof delay, 'number', 'Delay must be numeric');
      assert.ok(delay >= 2700 && delay <= 3300, `Delay (${delay}) must be within [2700, 3300] for attempt 1`);

      // Check DB update
      const lastUpdate = harness.dbUpdates[harness.dbUpdates.length - 1];
      assert.strictEqual(lastUpdate.update.$set.status, 'failed_retryable');
      assert.strictEqual(lastUpdate.update.$set.retry_count, 1);
      assert.strictEqual(lastUpdate.update.$set.error_detail, err.message);
      assert.ok(lastUpdate.update.$set.next_retry_at instanceof Date);

      // Check Audit Event
      const retryEvent = harness.events.find(e => e.event_type === 'application.retry_scheduled');
      assert.ok(retryEvent, 'application.retry_scheduled event must be emitted');
      assert.strictEqual(retryEvent.to_status, 'failed_retryable');
      assert.strictEqual(retryEvent.metadata.backoff_delay_ms, delay);
      assert.strictEqual(retryEvent.metadata.delivery_attempt, 1);

      // Check active jobs decremented
      assert.strictEqual(harness.getState().activeJobs, 0, 'activeJobs must return to 0');
    });
  }

  await runTest('Transient error on attempt 2 invokes msg.nak with doubled base backoff [5400, 6600]', async () => {
    const harness = createHarness({
      submitApplication: async () => { throw new Error('net::ERR_TIMED_OUT'); }
    });
    const msg = createMockMsg(standardPayload, { deliveryCount: 2 });

    await harness.processMessage(msg);

    assert.strictEqual(msg.calls.nak.length, 1);
    const delay = msg.calls.nak[0];
    assert.ok(delay >= 5400 && delay <= 6600, `Delay (${delay}) must be within [5400, 6600] for attempt 2`);
    assert.strictEqual(harness.getState().activeJobs, 0);
  });

  // ==========================================================================
  // Suite 2: Terminal Error Categorization
  // ==========================================================================
  console.log('\n--- Suite 2: Terminal Error Categorization ---');

  await runTest('Corrupted JSON syntax invokes msg.term("invalid_json")', async () => {
    const harness = createHarness();
    const msg = createMockMsg('<<<INVALID_JSON_CORRUPTED_BYTES>>>');

    await harness.processMessage(msg);

    assert.strictEqual(msg.calls.term.length, 1);
    assert.strictEqual(msg.calls.term[0], 'invalid_json');
    assert.strictEqual(msg.calls.nak.length, 0);
    assert.strictEqual(msg.calls.ack, 0);
    assert.strictEqual(harness.getState().activeJobs, 0);
  });

  const invalidSchemas = [
    { desc: 'empty payload object', payload: {} },
    { desc: 'missing application_id', payload: { job_url: 'https://...', candidate: { name: 'A' } } },
    { desc: 'missing job_url', payload: { application_id: 'app_1', candidate: { name: 'A' } } },
    { desc: 'missing candidate', payload: { application_id: 'app_1', job_url: 'https://...' } },
    { desc: 'null candidate field', payload: { application_id: 'app_1', job_url: 'https://...', candidate: null } }
  ];

  for (const tc of invalidSchemas) {
    await runTest(`Invalid schema (${tc.desc}) invokes msg.term("invalid_payload_schema")`, async () => {
      const harness = createHarness();
      const msg = createMockMsg(tc.payload);

      await harness.processMessage(msg);

      assert.strictEqual(msg.calls.term.length, 1);
      assert.strictEqual(msg.calls.term[0], 'invalid_payload_schema');
      assert.strictEqual(msg.calls.nak.length, 0);
      assert.strictEqual(msg.calls.ack, 0);
      assert.strictEqual(harness.getState().activeJobs, 0);

      // If application_id was present, status should be transitioned to failed_permanent
      if (tc.payload.application_id) {
        const update = harness.dbUpdates.find(u => u.query._id === tc.payload.application_id);
        assert.ok(update, 'DB update must be recorded for valid application_id');
        assert.strictEqual(update.update.$set.status, 'failed_permanent');
      }
    });
  }

  const nonRetryableErrors = [
    'url and candidate required',
    'invalid payload: missing required field',
    'job is no longer available on career portal',
    'position has been filled by another candidate',
    'posting has closed',
    'application form closed',
    'HTTP 404 Not Found: Job posting does not exist',
    'job not found in lever database'
  ];

  for (const errStr of nonRetryableErrors) {
    await runTest(`Non-retryable operational error "${errStr}" invokes msg.term("non_retryable_error")`, async () => {
      const harness = createHarness({
        submitApplication: async () => { throw new Error(errStr); }
      });
      const msg = createMockMsg(standardPayload, { deliveryCount: 1 });

      await harness.processMessage(msg);

      assert.strictEqual(msg.calls.term.length, 1);
      assert.strictEqual(msg.calls.term[0], 'non_retryable_error');
      assert.strictEqual(msg.calls.nak.length, 0);
      assert.strictEqual(msg.calls.ack, 0);

      const lastUpdate = harness.dbUpdates[harness.dbUpdates.length - 1];
      assert.strictEqual(lastUpdate.update.$set.status, 'failed_permanent');
      assert.strictEqual(lastUpdate.update.$set.error_detail, errStr);

      const failEvent = harness.events.find(e => e.event_type === 'application.failed');
      assert.ok(failEvent);
      assert.strictEqual(failEvent.to_status, 'failed_permanent');
      assert.strictEqual(failEvent.metadata.non_retryable, true);
      assert.strictEqual(harness.getState().activeJobs, 0);
    });
  }

  const captchaErrors = [
    'Cloudflare Turnstile token verification required',
    'reCAPTCHA v2 checkbox was presented',
    'hCaptcha bot challenge detected',
    'Please complete the security check to proceed',
    'Verify you are human before applying',
    'Your application submission was flagged as possible spam'
  ];

  for (const captchaErr of captchaErrors) {
    await runTest(`CAPTCHA / Bot detection "${captchaErr}" invokes msg.ack() and pauses in requires_manual_action`, async () => {
      const harness = createHarness({
        submitApplication: async () => { throw new Error(captchaErr); }
      });
      const msg = createMockMsg(standardPayload, { deliveryCount: 1 });

      await harness.processMessage(msg);

      assert.strictEqual(msg.calls.ack, 1, 'msg.ack must be called so message is not redelivered');
      assert.strictEqual(msg.calls.nak.length, 0);
      assert.strictEqual(msg.calls.term.length, 0);

      const lastUpdate = harness.dbUpdates[harness.dbUpdates.length - 1];
      assert.strictEqual(lastUpdate.update.$set.status, 'requires_manual_action');
      assert.strictEqual(lastUpdate.update.$set.pause_reason, 'captcha_verification_required');

      const pauseEvent = harness.events.find(e => e.event_type === 'application.requires_manual_action');
      assert.ok(pauseEvent);
      assert.strictEqual(pauseEvent.to_status, 'requires_manual_action');
      assert.strictEqual(pauseEvent.metadata.pause_reason, 'captcha_verification_required');
      assert.strictEqual(harness.getState().activeJobs, 0);
    });
  }

  // ==========================================================================
  // Suite 3: Retry Exhaustion
  // ==========================================================================
  console.log('\n--- Suite 3: Retry Exhaustion ---');

  const exhaustionCounts = [3, 4, 10];
  for (const count of exhaustionCounts) {
    await runTest(`Retry exhaustion at deliveryCount=${count} (MAX_RETRIES=3) invokes msg.term("max_retries_exceeded")`, async () => {
      const harness = createHarness({
        MAX_RETRIES: 3,
        submitApplication: async () => { throw new Error('net::ERR_CONNECTION_RESET'); }
      });
      const msg = createMockMsg(standardPayload, { deliveryCount: count });

      await harness.processMessage(msg);

      assert.strictEqual(msg.calls.term.length, 1, 'msg.term must be called');
      assert.strictEqual(msg.calls.term[0], 'max_retries_exceeded', 'Reason must be max_retries_exceeded');
      assert.strictEqual(msg.calls.nak.length, 0, 'msg.nak must NOT be called when retries exhausted');
      assert.strictEqual(msg.calls.ack, 0, 'msg.ack must NOT be called');

      const lastUpdate = harness.dbUpdates[harness.dbUpdates.length - 1];
      assert.strictEqual(lastUpdate.update.$set.status, 'failed_permanent');
      assert.ok(lastUpdate.update.$set.error_detail.includes('Exhausted 3 attempts'));
      assert.strictEqual(lastUpdate.update.$set.retry_count, count);

      const failEvent = harness.events.find(e => e.event_type === 'application.failed');
      assert.ok(failEvent);
      assert.strictEqual(failEvent.to_status, 'failed_permanent');
      assert.strictEqual(failEvent.metadata.max_retries_exceeded, true);
      assert.strictEqual(harness.getState().activeJobs, 0);
    });
  }

  // ==========================================================================
  // Suite 4: Per-Job Watchdog Timeout & Heartbeat Cleared
  // ==========================================================================
  console.log('\n--- Suite 4: Per-Job Watchdog Timeout ---');

  await runTest('Watchdog timeout triggers at JOB_TIMEOUT_MS, clears heartbeat timer, and calls msg.nak', async () => {
    const JOB_TIMEOUT_MS = 60;
    const HEARTBEAT_INTERVAL_MS = 15;

    const harness = createHarness({
      JOB_TIMEOUT_MS,
      HEARTBEAT_INTERVAL_MS,
      MAX_RETRIES: 3,
      submitApplication: async () => {
        // Deliberately hang past the watchdog timeout
        await new Promise(r => setTimeout(r, 250));
        return { receipt_id: 'never_reached' };
      }
    });

    const msg = createMockMsg(standardPayload, { deliveryCount: 1 });
    const startTime = Date.now();

    await harness.processMessage(msg);
    const duration = Date.now() - startTime;

    assert.ok(duration >= 50 && duration <= 120, `Execution duration (${duration}ms) must match timeout threshold (~60ms)`);
    assert.ok(msg.calls.working >= 2, `Heartbeat working() should have been called >=2 times (was ${msg.calls.working})`);

    assert.strictEqual(msg.calls.nak.length, 1, 'Timeout must invoke msg.nak for transient retry');
    assert.strictEqual(msg.calls.term.length, 0);

    const workingCountAtFinish = msg.calls.working;

    // Wait an additional 50ms (more than 3 heartbeat intervals)
    await new Promise(r => setTimeout(r, 50));

    // Verify heartbeat timer was strictly cleared and does NOT keep firing
    assert.strictEqual(msg.calls.working, workingCountAtFinish, 'Heartbeat timer must be completely cleared after finish');
    assert.strictEqual(harness.getState().activeJobs, 0, 'activeJobs must return to 0');
  });

  await runTest('Watchdog timeout with deliveryCount >= MAX_RETRIES terminates with max_retries_exceeded', async () => {
    const harness = createHarness({
      JOB_TIMEOUT_MS: 40,
      HEARTBEAT_INTERVAL_MS: 10,
      MAX_RETRIES: 3,
      submitApplication: async () => {
        await new Promise(r => setTimeout(r, 200));
        return {};
      }
    });

    const msg = createMockMsg(standardPayload, { deliveryCount: 3 });
    await harness.processMessage(msg);

    assert.strictEqual(msg.calls.term.length, 1);
    assert.strictEqual(msg.calls.term[0], 'max_retries_exceeded');
    assert.strictEqual(msg.calls.nak.length, 0);
    assert.strictEqual(harness.getState().activeJobs, 0);
  });

  // ==========================================================================
  // Suite 5: Backoff Calculation Fuzzing
  // ==========================================================================
  console.log('\n--- Suite 5: Backoff Calculation Fuzzing ---');

  await runTest('Fuzz 10,000 backoff delay iterations across delivery counts 1 to 10', () => {
    const harness = createHarness();
    const BASE_BACKOFF_MS = 3000;
    const MAX_BACKOFF_MS = 30000;

    for (let count = 1; count <= 10; count++) {
      const exp = Math.max(0, count - 1);
      const expectedBase = Math.min(MAX_BACKOFF_MS, BASE_BACKOFF_MS * Math.pow(2, exp));
      const minAllowed = Math.max(1000, Math.floor(expectedBase * 0.9));
      const maxAllowed = Math.ceil(expectedBase * 1.1);

      const samples = [];
      for (let i = 0; i < 1000; i++) {
        const delay = harness.calculateBackoffDelay(count);
        assert.ok(
          delay >= minAllowed && delay <= maxAllowed,
          `Count ${count} iter ${i}: delay ${delay} outside [${minAllowed}, ${maxAllowed}]`
        );
        samples.push(delay);
      }

      // Verify jitter variance: min != max and standard deviation > 0
      const min = Math.min(...samples);
      const max = Math.max(...samples);
      const avg = samples.reduce((a, b) => a + b, 0) / samples.length;
      const stddev = Math.sqrt(samples.reduce((a, b) => a + Math.pow(b - avg, 2), 0) / samples.length);
      const distinctValues = new Set(samples).size;

      assert.ok(distinctValues > 50, `Count ${count} must have >50 distinct values (got ${distinctValues})`);
      assert.ok(stddev > 0, `Count ${count} standard deviation must be > 0 (got ${stddev})`);
      assert.ok(min < expectedBase, `Count ${count} min (${min}) should be below base (${expectedBase})`);
      assert.ok(max > expectedBase, `Count ${count} max (${max}) should be above base (${expectedBase})`);
    }
  });

  await runTest('Adversarial inputs to calculateBackoffDelay (0, negative, huge numbers, NaN)', () => {
    const harness = createHarness();
    
    // Delivery count <= 0 should default to base backoff [2700, 3300]
    for (const c of [0, -1, -100]) {
      const delay = harness.calculateBackoffDelay(c);
      assert.ok(delay >= 2700 && delay <= 3300, `Delay for count ${c} (${delay}) must be in base range [2700, 3300]`);
    }

    // Huge delivery count (100, 1000) must stay capped at MAX_BACKOFF_MS [27000, 33000]
    for (const c of [20, 50, 100, 1000]) {
      const delay = harness.calculateBackoffDelay(c);
      assert.ok(delay >= 27000 && delay <= 33000, `Delay for count ${c} (${delay}) must be in capped range [27000, 33000]`);
      assert.ok(Number.isFinite(delay), `Delay for count ${c} must be finite`);
    }
  });

  // ==========================================================================
  // Suite 6: Concurrency Slot Leak Stress Test
  // ==========================================================================
  console.log('\n--- Suite 6: Concurrency Slot Leak Stress Test ---');

  await runTest('100 interleaved mixed-outcome jobs preserve concurrency slots with 0 leaks', async () => {
    let modeCounter = 0;
    const harness = createHarness({
      MAX_RETRIES: 3,
      JOB_TIMEOUT_MS: 30,
      HEARTBEAT_INTERVAL_MS: 10,
      submitApplication: async () => {
        const mode = modeCounter++ % 5;
        if (mode === 0) {
          // Success
          return { receipt_id: 'rec_stress', provider: 'ashby' };
        } else if (mode === 1) {
          // Transient error
          throw new Error('net::ERR_RESET');
        } else if (mode === 2) {
          // Non-retryable error
          throw new Error('HTTP 404 Not Found: job posting has closed');
        } else if (mode === 3) {
          // CAPTCHA
          throw new Error('Turnstile token verification required');
        } else {
          // Hang and trigger watchdog
          await new Promise(r => setTimeout(r, 80));
          return {};
        }
      }
    });

    const promises = [];
    for (let i = 0; i < 100; i++) {
      let payload;
      if (i % 15 === 0) {
        // Corrupted JSON
        payload = 'CORRUPTED';
      } else if (i % 12 === 0) {
        // Missing candidate
        payload = { application_id: `app-stress-${i}`, job_url: 'https://...' };
      } else {
        payload = {
          ...standardPayload,
          application_id: `app-stress-${i}`
        };
      }
      const msg = createMockMsg(payload, { deliveryCount: (i % 4) + 1 });
      promises.push(harness.processMessage(msg));
    }

    await Promise.all(promises);

    const finalState = harness.getState();
    assert.strictEqual(finalState.activeJobs, 0, `activeJobs must return to 0 (was ${finalState.activeJobs})`);
    console.log(`    Processed 100 mixed adversarial jobs with activeJobs strictly 0!`);
  });

  // ==========================================================================
  // Summary
  // ==========================================================================
  console.log('\n================================================================');
  console.log(`Test Execution Summary: ${testsPassed} / ${testsRun} PASSED`);
  if (testsPassed === testsRun) {
    console.log('ALL EMPIRICAL VERIFICATION TESTS PASSED 100%!');
    console.log('Worker daemon queue lease, retry, timeout, and backoff logic verified correct.');
  } else {
    console.error(`SOME TESTS FAILED: ${testsRun - testsPassed} failed.`);
    process.exit(1);
  }
  console.log('================================================================\n');
}

main().catch(err => {
  console.error('Fatal test error:', err);
  process.exit(1);
});
