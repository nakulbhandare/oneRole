/**
 * Unit & Integration Test Suite for OneRole Tier 1 Fast Submitter (fast_submitter.js)
 * Tests direct headless API / GraphQL submissions in Node.js (<350ms, zero browser sessions).
 */

const assert = require('assert');
const http = require('http');
const {
  submitFastApplication,
  resolveCandidateProfile,
  extractGreenhouseParams,
  extractLeverParams,
  extractAshbyParams,
  CANONICAL_TARGET_EMAIL
} = require('../fast_submitter');

const sleep = ms => new Promise(r => setTimeout(r, ms));

async function runTests() {
  console.log('================================================================');
  console.log('       FAST SUBMITTER TIER 1 INTEGRATION TEST SUITE');
  console.log('================================================================\n');

  let passed = 0;
  let failed = 0;

  function test(name, fn) {
    return Promise.resolve()
      .then(fn)
      .then(() => {
        console.log(`✅ PASS: ${name}`);
        passed++;
      })
      .catch(err => {
        console.error(`❌ FAIL: ${name}`);
        console.error(`       ${err.message}`);
        failed++;
      });
  }

  // 1. Candidate Resolver & Email Identity Enforcement (<15ms)
  await test('Candidate Profile Resolution strictly enforces bhandarenakul11@gmail.com in <15ms', async () => {
    const start = Date.now();
    const candidate = {
      first_name: 'Nakul',
      last_name: 'Bhandare',
      email: 'attacker@evil.com', // MUST be overridden
      phone: '+1 (350)-355-6570',
      city: 'San Francisco, CA'
    };

    const profile = resolveCandidateProfile(candidate);
    const duration = Date.now() - start;

    assert.strictEqual(profile.email, CANONICAL_TARGET_EMAIL);
    assert.strictEqual(profile.target_email, 'bhandarenakul11@gmail.com');
    assert.strictEqual(profile.first_name, 'Nakul');
    assert.strictEqual(profile.last_name, 'Bhandare');
    assert.strictEqual(profile.full_name, 'Nakul Bhandare');
    assert.strictEqual(profile.answers.work_auth, 'Yes');
    assert.strictEqual(profile.answers.sponsorship, 'No');
    assert.ok(duration < 15, `Candidate resolution took ${duration}ms, expected < 15ms`);
  });

  // 2. Greenhouse API Headless Submitter (<250ms target)
  await test('Greenhouse Direct API Submitter executes in <250ms with zero browser session', async () => {
    let receivedPayload = null;
    const server = http.createServer((req, res) => {
      let body = [];
      req.on('data', chunk => body.push(chunk));
      req.on('end', () => {
        receivedPayload = Buffer.concat(body).toString();
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ id: 556677, application_id: 556677 }));
      });
    });

    await new Promise(r => server.listen(0, '127.0.0.1', r));
    const port = server.address().port;

    try {
      const jobUrl = 'https://boards.greenhouse.io/stripe/jobs/123456';
      const mockResume = Buffer.from('%PDF-1.4 Mock PDF Resume Buffer');

      const start = Date.now();
      const result = await submitFastApplication({
        url: jobUrl,
        candidate: { email: 'wrong@test.com' },
        resumeBuffer: mockResume,
        endpoints: {
          greenhouse: `http://127.0.0.1:${port}/v1/boards/stripe/jobs/123456`
        }
      });
      const duration = Date.now() - start;

      assert.strictEqual(result.success, true);
      assert.strictEqual(result.status, 'SUBMITTED_CONFIRMED');
      assert.strictEqual(result.receipt_id, 'GH-556677');
      assert.strictEqual(result.target_email, 'bhandarenakul11@gmail.com');
      assert.strictEqual(result.provider, 'greenhouse');
      assert.ok(result.duration_ms < 250, `Greenhouse duration was ${result.duration_ms}ms, expected < 250ms`);
      assert.ok(receivedPayload.includes('bhandarenakul11@gmail.com'), 'Payload must contain target email');
    } finally {
      server.close();
    }
  });

  // 3. Ashby Direct GraphQL Submitter (<320ms target)
  await test('Ashby 2-step GraphQL Submitter executes in <320ms', async () => {
    let stepCount = 0;
    const server = http.createServer((req, res) => {
      let body = [];
      req.on('data', chunk => body.push(chunk));
      req.on('end', () => {
        const raw = Buffer.concat(body).toString();
        if (req.url.includes('ApiCreateFileUploadHandle')) {
          stepCount++;
          res.writeHead(200, { 'Content-Type': 'application/json' });
          res.end(JSON.stringify({ data: { fileUploadHandle: { handle: 'ashby_mock_handle_abc' } } }));
          return;
        }
        if (req.url.includes('ApiSubmitSingleApplicationFormAction')) {
          stepCount++;
          res.writeHead(200, { 'Content-Type': 'application/json' });
          res.end(JSON.stringify({
            data: {
              submitApplicationFormAction: {
                applicationFormResult: { __typename: 'FormSubmitSuccess' },
                applicationId: 'app_ashby_998877'
              }
            }
          }));
          return;
        }
        res.writeHead(404);
        res.end();
      });
    });

    await new Promise(r => server.listen(0, '127.0.0.1', r));
    const port = server.address().port;

    try {
      const jobUrl = 'https://jobs.ashbyhq.com/linear/job-123';
      const mockResume = Buffer.from('%PDF-1.4 Ashby Test Resume');

      const start = Date.now();
      const result = await submitFastApplication({
        url: jobUrl,
        candidate: { email: 'fake@example.com' },
        resumeBuffer: mockResume,
        endpoints: {
          ashby: `http://127.0.0.1:${port}/api/non-user-graphql`
        }
      });
      const duration = Date.now() - start;

      assert.strictEqual(result.success, true);
      assert.strictEqual(result.status, 'SUBMITTED_CONFIRMED');
      assert.strictEqual(result.receipt_id, 'ASH-app_ashby_998877');
      assert.strictEqual(result.target_email, 'bhandarenakul11@gmail.com');
      assert.strictEqual(result.provider, 'ashby');
      assert.strictEqual(stepCount, 2, 'Must execute both file upload handle and mutation');
      assert.ok(result.duration_ms < 320, `Ashby duration was ${result.duration_ms}ms, expected < 320ms`);
    } finally {
      server.close();
    }
  });

  // 4. Lever Direct API Submitter (<220ms target)
  await test('Lever Direct Multipart Submitter executes in <220ms', async () => {
    let leverReceived = false;
    const server = http.createServer((req, res) => {
      let body = [];
      req.on('data', chunk => body.push(chunk));
      req.on('end', () => {
        leverReceived = true;
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ data: { id: 'lever-app-receipt-443322' } }));
      });
    });

    await new Promise(r => server.listen(0, '127.0.0.1', r));
    const port = server.address().port;

    try {
      const jobUrl = 'https://jobs.lever.co/netflix/eng-101';
      const mockResume = Buffer.from('%PDF-1.4 Lever Test Resume');

      const start = Date.now();
      const result = await submitFastApplication({
        url: jobUrl,
        candidate: { email: 'invalid@lever.co' },
        resumeBuffer: mockResume,
        endpoints: {
          lever: `http://127.0.0.1:${port}/netflix/eng-101/apply`
        }
      });
      const duration = Date.now() - start;

      assert.strictEqual(result.success, true);
      assert.strictEqual(result.status, 'SUBMITTED_CONFIRMED');
      assert.strictEqual(result.receipt_id, 'LEVER-lever-app-receipt-443322');
      assert.strictEqual(result.target_email, 'bhandarenakul11@gmail.com');
      assert.strictEqual(result.provider, 'lever');
      assert.ok(leverReceived, 'Lever endpoint received submission');
      assert.ok(result.duration_ms < 220, `Lever duration was ${result.duration_ms}ms, expected < 220ms`);
    } finally {
      server.close();
    }
  });

  // 5. Total Headless Runner Budget (<350ms target)
  await test('Headless API runner completes overall pipeline in <350ms with zero browser session', async () => {
    const server = http.createServer((req, res) => {
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ id: 1234 }));
    });
    await new Promise(r => server.listen(0, '127.0.0.1', r));
    const port = server.address().port;

    try {
      const start = Date.now();
      const result = await submitFastApplication({
        url: 'https://boards.greenhouse.io/datadog/jobs/998877',
        candidate: { email: 'any@domain.com' },
        resumeBuffer: Buffer.from('%PDF-1.4 Minimal PDF'),
        endpoints: {
          greenhouse: `http://127.0.0.1:${port}/v1/boards/datadog/jobs/998877`
        }
      });
      const duration = Date.now() - start;

      assert.strictEqual(result.success, true);
      assert.strictEqual(result.status, 'SUBMITTED_CONFIRMED');
      assert.strictEqual(result.target_email, 'bhandarenakul11@gmail.com');
      assert.ok(duration < 350, `Overall execution took ${duration}ms, expected < 350ms`);
    } finally {
      server.close();
    }
  });

  console.log('\n----------------------------------------------------------------');
  console.log(`Results: ${passed} passed, ${failed} failed`);
  console.log('----------------------------------------------------------------\n');

  if (failed > 0) {
    process.exit(1);
  }
}

runTests().catch(err => {
  console.error('Test harness exception:', err);
  process.exit(1);
});
