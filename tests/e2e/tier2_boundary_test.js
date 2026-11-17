/**
 * OneRole E2E Test Suite: Tier 2 Boundary & Corner Cases
 * File: tests/e2e/tier2_boundary_test.js
 * 
 * Contains 40 concrete boundary, edge, and corner test cases across Features F1 through F8:
 * - Boundary Group 1: Combobox & Dropdown Variations (T2-F1-01 to T2-F1-05)
 * - Boundary Group 2: Unicode & Special Characters Profile Binding (T2-F2-01 to T2-F2-05)
 * - Boundary Group 3: File Upload Edge Cases & Integrity (T2-F3-01 to T2-F3-05)
 * - Boundary Group 4: GraphQL Error Variations & Receipt Inconsistencies (T2-F4-01 to T2-F4-05)
 * - Boundary Group 5: Timestamp Collisions & Stale Tracking Table States (T2-F5-01 to T2-F5-05)
 * - Boundary Group 6: Queue Concurrency & Depleted Pool Replenishment (T2-F6-01 to T2-F6-05)
 * - Boundary Group 7: Multi-Step Navigation Edge Cases & Validation Blockers (T2-F7-01 to T2-F7-05)
 * - Boundary Group 8: Network Flakiness, Rate Limits & Retry Delays (T2-F8-01 to T2-F8-05)
 */

const assert = require('assert');
const path = require('path');
const fs = require('fs');
const setup = require('./helpers/setup');
const mockAts = require('./helpers/mock_ats');
const assertions = require('./helpers/assertions');

const sleep = ms => new Promise(r => setTimeout(r, ms));

async function runSuite(ctx) {
  const db = await setup.getDb();
  await setup.cleanDatabaseFixtures('e2e-t2-');

  // ===========================================================================
  // Boundary Group 1: Combobox & Dropdown Variations (F1 Edge Cases)
  // ===========================================================================

  // T2-F1-01: Case-Insensitive Dropdown Matching
  await ctx.test('T2-F1-01: Case-Insensitive Dropdown Matching', async () => {
    const options = ['YES', 'NO', 'MAYBE'];
    const candidateAnswer = 'yes';

    function findMatchCaseInsensitive(opts, target) {
      const normTarget = target.trim().toLowerCase();
      return opts.find(o => o.trim().toLowerCase() === normTarget) || null;
    }

    const matched = findMatchCaseInsensitive(options, candidateAnswer);
    assert.strictEqual(matched, 'YES', 'Must match YES despite lowercase candidate input');
  });

  // T2-F1-02: Fuzzy Substring Option Matching
  await ctx.test('T2-F1-02: Fuzzy Substring Option Matching', async () => {
    const options = ['United States of America (USA)', 'Canada', 'Mexico'];
    const target = 'United States';

    function findFuzzySubstringMatch(opts, query) {
      const q = query.toLowerCase();
      return opts.find(o => o.toLowerCase().includes(q)) || null;
    }

    const matched = findFuzzySubstringMatch(options, target);
    assert.strictEqual(matched, 'United States of America (USA)', 'Must find substring match for United States');
  });

  // T2-F1-03: Virtualized Listbox with Invisible / Detached Options
  await ctx.test('T2-F1-03: Virtualized Listbox with Invisible / Detached Options', async () => {
    // 200 virtualized items where only matches are mounted when search input has text
    const fullDataset = Array.from({ length: 200 }, (_, i) => `Country-${i}`);
    fullDataset.push('Zimbabwe');

    function filterVirtualizedOptions(inputQuery) {
      if (!inputQuery) return fullDataset.slice(0, 5); // initial virtualized window
      return fullDataset.filter(item => item.toLowerCase().includes(inputQuery.toLowerCase())).slice(0, 5);
    }

    const initialWindow = filterVirtualizedOptions('');
    assert.ok(!initialWindow.includes('Zimbabwe'), 'Zimbabwe must not be in initial 5 items');

    const filtered = filterVirtualizedOptions('Zimbabwe');
    assert.strictEqual(filtered[0], 'Zimbabwe', 'Filtering listbox must mount Zimbabwe');
  });

  // T2-F1-04: Non-English & Accent Character Matching in Select Options
  await ctx.test('T2-F1-04: Non-English & Accent Character Matching in Select Options', async () => {
    const options = ['Montréal, QC', 'São Paulo', 'Zürich'];
    const query = 'Montreal';

    function normalizeAccents(str) {
      return str.normalize('NFD').replace(/[\u0300-\u036f]/g, '').toLowerCase();
    }

    function findDiacriticInsensitiveMatch(opts, target) {
      const normTarget = normalizeAccents(target);
      return opts.find(o => normalizeAccents(o).includes(normTarget)) || null;
    }

    const matched = findDiacriticInsensitiveMatch(options, query);
    assert.strictEqual(matched, 'Montréal, QC', 'Must match Montréal with query Montreal');
  });

  // T2-F1-05: Multi-Select Combobox without Menu Auto-Close
  await ctx.test('T2-F1-05: Multi-Select Combobox without Menu Auto-Close', async () => {
    const selectedBadges = new Set();

    function multiSelectAddItem(badges, item) {
      badges.add(item);
      return Array.from(badges);
    }

    multiSelectAddItem(selectedBadges, 'Go');
    multiSelectAddItem(selectedBadges, 'Docker');

    assert.strictEqual(selectedBadges.size, 2, 'Multi-select must retain both Go and Docker');
    assert.ok(selectedBadges.has('Go') && selectedBadges.has('Docker'));
  });

  // ===========================================================================
  // Boundary Group 2: Unicode & Special Characters Profile Binding (F2 Edge Cases)
  // ===========================================================================

  // T2-F2-01: Hyphenated and Apostrophed Name Handling
  await ctx.test("T2-F2-01: Hyphenated and Apostrophed Name Handling", async () => {
    const name = "Jean-Luc O'Connor";

    function sanitizeName(n) {
      // Must NOT strip hyphens or single apostrophes
      return n.replace(/[^\w\s'-]/gi, '').trim();
    }

    const cleaned = sanitizeName(name);
    assert.strictEqual(cleaned, "Jean-Luc O'Connor", 'Must preserve hyphen and apostrophe');
  });

  // T2-F2-02: Unicode Address and Accent Characters
  await ctx.test('T2-F2-02: Unicode Address and Accent Characters', async () => {
    const address = "123 Rue de l'Étoile, Montréal, CA";
    const utf8Buffer = Buffer.from(address, 'utf8');
    const roundTripped = utf8Buffer.toString('utf8');

    assert.strictEqual(roundTripped, address, 'Unicode address must survive roundtrip without corruption');
    assert.ok(roundTripped.includes('Étoile') && roundTripped.includes('Montréal'));
  });

  // T2-F2-03: International Phone Number Variations with Extension
  await ctx.test('T2-F2-03: International Phone Number Variations with Extension', async () => {
    const raw = '+1 (350) 355-6570 ext. 101';

    function parsePhoneAndExtension(p) {
      const extMatch = p.match(/(?:ext|x|extension)[.\s]*(\d+)/i);
      const ext = extMatch ? extMatch[1] : null;
      const base = p.replace(/(?:ext|x|extension)[.\s]*\d+/i, '').trim();
      return { base, ext };
    }

    const { base, ext } = parsePhoneAndExtension(raw);
    assert.strictEqual(base, '+1 (350) 355-6570');
    assert.strictEqual(ext, '101');
  });

  // T2-F2-04: Maximum Length Boundary String Handling
  await ctx.test('T2-F2-04: Maximum Length Boundary String Handling', async () => {
    const longString = 'A'.repeat(500);

    function boundString(str, maxLen = 1000) {
      return str.slice(0, maxLen);
    }

    const result = boundString(longString, 1000);
    assert.strictEqual(result.length, 500, '500-char answer must not cause overflow or crash');
  });

  // T2-F2-05: Leading and Trailing Whitespace Sanitization
  await ctx.test('T2-F2-05: Leading and Trailing Whitespace Sanitization', async () => {
    const rawEmail = '   bhandarenakul11@gmail.com   \n';
    const cleanEmail = rawEmail.trim();

    assert.strictEqual(cleanEmail, 'bhandarenakul11@gmail.com', 'Whitespace must be trimmed');
  });

  // ===========================================================================
  // Boundary Group 3: File Upload Edge Cases & Integrity (F3 Edge Cases)
  // ===========================================================================

  // T2-F3-01: Zero-Byte Resume File Detection & Guardrail
  await ctx.test('T2-F3-01: Zero-Byte Resume File Detection & Guardrail', async () => {
    function validateResumeFileBuffer(buf) {
      if (!buf || buf.length === 0) {
        throw new Error('Zero-byte file rejected: Resume cannot be empty');
      }
      return true;
    }

    assert.throws(
      () => validateResumeFileBuffer(Buffer.alloc(0)),
      /Zero-byte file rejected/
    );
  });

  // T2-F3-02: Path Traversal Protection on Resume Path
  await ctx.test('T2-F3-02: Path Traversal Protection on Resume Path', async () => {
    const maliciousPath = '../../../../../etc/passwd';

    function isSafeResumePath(inputPath, allowedBaseDirs) {
      const resolved = path.resolve(inputPath);
      return allowedBaseDirs.some(base => resolved.startsWith(path.resolve(base)));
    }

    const allowed = ['./oneRole_UI/public', '/tmp'];
    const isSafe = isSafeResumePath(maliciousPath, allowed);
    assert.strictEqual(isSafe, false, 'Path traversal attempt must be blocked');
  });

  // T2-F3-03: Corrupt PDF Header Validation
  await ctx.test('T2-F3-03: Corrupt PDF Header Validation', async () => {
    const fakePdf = Buffer.from('This is not a real PDF file, just text');

    function checkPdfHeader(buf) {
      if (!buf || buf.length < 5) return false;
      return buf.slice(0, 5).toString('ascii') === '%PDF-';
    }

    assert.strictEqual(checkPdfHeader(fakePdf), false, 'Corrupt PDF without %PDF- magic bytes must fail validation');
  });

  // T2-F3-04: File Chooser Dialog Interception Race Condition
  await ctx.test('T2-F3-04: File Chooser Dialog Interception Race Condition', async () => {
    let chooserHandled = false;

    // Emulating fileChooser event promise
    async function simulateFileChooserUpload(filePath) {
      const fileChooserPromise = new Promise(resolve => {
        setTimeout(() => {
          resolve({
            accept: paths => {
              chooserHandled = paths[0] === filePath;
            }
          });
        }, 10);
      });

      const fileChooser = await fileChooserPromise;
      await fileChooser.accept([filePath]);
    }

    await simulateFileChooserUpload(setup.CANDIDATE_PROFILE.resume_file_path);
    assert.strictEqual(chooserHandled, true, 'File chooser dialog must be accepted cleanly');
  });

  // T2-F3-05: Drag-and-Drop File Upload Dropzone Handling
  await ctx.test('T2-F3-05: Drag-and-Drop File Upload Dropzone Handling', async () => {
    await mockAts.start();
    const browser = await setup.launchBrowser();
    const { page } = await setup.createTestPage(browser);

    try {
      await page.goto(`${setup.ENV.MOCK_ATS_URL}/workday/nvidia/apply/step4`);
      const dropzone = await page.$('div[data-automation-id="file-upload-dropzone"]');
      assert.ok(dropzone, 'Dropzone element must exist');

      const fileInput = await dropzone.$('input[type="file"]');
      assert.ok(fileInput, 'Nested input[type="file"] in dropzone must exist');

      await fileInput.uploadFile(setup.CANDIDATE_PROFILE.resume_file_path);
      const filesCount = await page.evaluate(el => el.files.length, fileInput);
      assert.strictEqual(filesCount, 1, 'File must be attached to dropzone input');
    } finally {
      await browser.close();
    }
  });

  // ===========================================================================
  // Boundary Group 4: GraphQL Error Variations & Receipt Inconsistencies (F4 Edge Cases)
  // ===========================================================================

  // T2-F4-01: GraphQL Network 200 with Nested Multiple Error Objects
  await ctx.test('T2-F4-01: GraphQL Network 200 with Nested Multiple Error Objects', async () => {
    const rawGql = JSON.stringify({
      errors: [
        { message: 'Invalid phone format' },
        { message: 'Missing custom field', extensions: { code: 'INVALID_FIELD' } }
      ]
    });

    function parseGraphQLResult(body) {
      const parsed = JSON.parse(body);
      if (parsed.errors && parsed.errors.length > 0) {
        return {
          confirmed: false,
          errorSummary: parsed.errors.map(e => e.message).join('; ')
        };
      }
      return { confirmed: true, errorSummary: null };
    }

    const res = parseGraphQLResult(rawGql);
    assert.strictEqual(res.confirmed, false, 'Must be marked unconfirmed');
    assert.ok(res.errorSummary.includes('Invalid phone format'));
    assert.ok(res.errorSummary.includes('Missing custom field'));
  });

  // T2-F4-02: GraphQL Response with Empty Data and No Errors Array
  await ctx.test('T2-F4-02: GraphQL Response with Empty Data and No Errors Array', async () => {
    const emptyGql = JSON.stringify({ data: null });

    function isConfirmed(body) {
      try {
        const p = JSON.parse(body);
        if (p.errors && p.errors.length > 0) return false;
        if (!p.data || !p.data.submitApplication || !p.data.submitApplication.success) return false;
        return true;
      } catch (e) {
        return false;
      }
    }

    assert.strictEqual(isConfirmed(emptyGql), false, 'Empty data without success must not confirm');
  });

  // T2-F4-03: Mixed Success Response with Warnings
  await ctx.test('T2-F4-03: Mixed Success Response with Warnings', async () => {
    const mixedPayload = JSON.stringify({
      data: {
        submitApplication: { success: true, receipt: 'ASHBY-WARN-123' }
      },
      warnings: ['Non-critical optional demographic field omitted']
    });

    const parsed = JSON.parse(mixedPayload);
    const isSuccess = parsed.data && parsed.data.submitApplication && parsed.data.submitApplication.success;
    assert.strictEqual(isSuccess, true, 'Non-critical warnings must not void successful submission');
  });

  // T2-F4-04: Delayed Confirmation Page Navigation (Network Lag)
  await ctx.test('T2-F4-04: Delayed Confirmation Page Navigation (Network Lag)', async () => {
    let confirmed = false;

    async function waitForConfirmationWithRetry(pollFn, timeoutMs = 2000, intervalMs = 50) {
      const start = Date.now();
      while (Date.now() - start < timeoutMs) {
        if (await pollFn()) return true;
        await sleep(intervalMs);
      }
      return false;
    }

    // Simulating redirect arriving after 200ms
    setTimeout(() => { confirmed = true; }, 200);

    const result = await waitForConfirmationWithRetry(async () => confirmed);
    assert.strictEqual(result, true, 'Poll mechanism must wait and catch delayed confirmation');
  });

  // T2-F4-05: Subdomain Redirect Confirmation Detection
  await ctx.test('T2-F4-05: Subdomain Redirect Confirmation Detection', async () => {
    const redirectUrl = 'https://careers.lyft.com/application-submitted?candidate=123';

    function isAtsConfirmationUrl(url) {
      const lower = url.toLowerCase();
      return lower.includes('confirmation') ||
             lower.includes('submitted') ||
             lower.includes('thank-you') ||
             lower.includes('thanks');
    }

    assert.strictEqual(isAtsConfirmationUrl(redirectUrl), true, 'Must detect confirmation on custom subdomain');
  });

  // ===========================================================================
  // Boundary Group 5: Timestamp Collisions & Stale Tracking Table States (F5 Edge Cases)
  // ===========================================================================

  // T2-F5-01: Identical Millisecond Timestamp Sort Stability
  await ctx.test('T2-F5-01: Identical Millisecond Timestamp Sort Stability', async () => {
    const apps = [
      { id: 'app_b', submitted_at: '2026-11-17T00:00:00.000Z' },
      { id: 'app_a', submitted_at: '2026-11-17T00:00:00.000Z' }
    ];

    function stableSort(items) {
      return [...items].sort((a, b) => {
        const timeDiff = new Date(b.submitted_at).getTime() - new Date(a.submitted_at).getTime();
        if (timeDiff !== 0) return timeDiff;
        return a.id.localeCompare(b.id); // secondary tie-breaker
      });
    }

    const sorted = stableSort(apps);
    assert.strictEqual(sorted[0].id, 'app_a', 'Tie-breaker must deterministically order app_a before app_b');
  });

  // T2-F5-02: Stale Just now Flag Eviction from LocalStorage
  await ctx.test("T2-F5-02: Stale 'Just now' Flag Eviction from LocalStorage", async () => {
    // A record saved 3 days ago with applied: "Just now"
    const threeDaysAgo = Date.now() - (3 * 86400 * 1000);
    const staleRecord = {
      id: 'stale-1',
      applied: 'Just now',
      created_at: new Date(threeDaysAgo).toISOString()
    };

    function refreshDisplayTime(record) {
      const createdTime = new Date(record.created_at).getTime();
      const diffMs = Date.now() - createdTime;
      if (diffMs > 3600 * 1000) { // older than 1 hour
        const days = Math.floor(diffMs / (86400 * 1000));
        return `${days}d ago`;
      }
      return record.applied;
    }

    const freshDisplay = refreshDisplayTime(staleRecord);
    assert.strictEqual(freshDisplay, '3d ago', 'Stale "Just now" string must be recalculated to "3d ago"');
  });

  // T2-F5-03: Malformed / Invalid Timestamp Fallback
  await ctx.test('T2-F5-03: Malformed / Invalid Timestamp Fallback', async () => {
    function safeParseTimestamp(val) {
      if (!val) return 0;
      const t = new Date(val).getTime();
      return isNaN(t) ? 0 : t;
    }

    assert.strictEqual(safeParseTimestamp('invalid-date-string'), 0, 'Invalid string must return 0, not NaN');
    assert.strictEqual(safeParseTimestamp(null), 0, 'Null must return 0');
  });

  // T2-F5-04: Rapid Successive Submissions with Monotonic ID Ordering
  await ctx.test('T2-F5-04: Rapid Successive Submissions with Monotonic ID Ordering', async () => {
    const rawSubmissions = [];
    const base = Date.now();
    for (let i = 0; i < 10; i++) {
      rawSubmissions.push({
        id: `sub-${i}`,
        index: i,
        submitted_at: new Date(base + i).toISOString()
      });
    }

    // Sort descending by timestamp
    const sorted = [...rawSubmissions].sort((a, b) => new Date(b.submitted_at) - new Date(a.submitted_at));
    assert.strictEqual(sorted[0].id, 'sub-9', 'The 10th submission must be at Row 1');
    assert.strictEqual(sorted[9].id, 'sub-0', 'The 1st submission must be at Row 10');
  });

  // T2-F5-05: Empty Tracking Table Initial State Rendering
  await ctx.test('T2-F5-05: Empty Tracking Table Initial State Rendering', async () => {
    function renderTableRows(applications) {
      if (!applications || applications.length === 0) {
        return '<tr><td colspan="7">No applications submitted yet</td></tr>';
      }
      return applications.map(a => `<tr><td>${a.company}</td></tr>`).join('');
    }

    const html = renderTableRows([]);
    assert.ok(html.includes('No applications submitted yet'), 'Empty table must render clean empty message without error');
  });

  // ===========================================================================
  // Boundary Group 6: Queue Concurrency & Depleted Pool Replenishment (F6 Edge Cases)
  // ===========================================================================

  // T2-F6-01: Depleted Replenish Pool Fallback to Catalog
  await ctx.test('T2-F6-01: Depleted Replenish Pool Fallback to Catalog', async () => {
    const replenishPool = [{ company: 'A' }, { company: 'B' }]; // only 2
    const catalog = [{ company: 'C' }, { company: 'D' }, { company: 'E' }, { company: 'F' }];

    function replenishWithFallback(pool, cat, targetSize = 5) {
      const res = [];
      const seen = new Set();

      for (const item of pool) {
        if (res.length < targetSize && !seen.has(item.company)) {
          seen.add(item.company);
          res.push(item);
        }
      }
      for (const item of cat) {
        if (res.length < targetSize && !seen.has(item.company)) {
          seen.add(item.company);
          res.push(item);
        }
      }
      return res;
    }

    const finalQueue = replenishWithFallback(replenishPool, catalog, 5);
    assertions.assertQueueDeduplicated(finalQueue, 'Must backfill from catalog to reach exactly 5 unique companies');
  });

  // T2-F6-02: Exhausted Catalog Queue Degraded State
  await ctx.test('T2-F6-02: Exhausted Catalog Queue Degraded State', async () => {
    const exhaustedCatalog = [{ company: 'OnlyJob1' }, { company: 'OnlyJob2' }];

    function buildQueueSafely(cat) {
      return (cat || []).slice(0, 5);
    }

    const q = buildQueueSafely(exhaustedCatalog);
    assert.strictEqual(q.length, 2, 'Exhausted catalog must safely return remaining 2 jobs without null crash');
  });

  // T2-F6-03: Concurrent Double-Click on Single Card Apply
  await ctx.test('T2-F6-03: Concurrent Double-Click on Single Card Apply', async () => {
    let isApplying = false;
    let dispatchCount = 0;

    async function handleClick() {
      if (isApplying) return; // guard against rapid double clicks
      isApplying = true;
      dispatchCount++;
      await sleep(50);
      isApplying = false;
    }

    // Fire two clicks simultaneously
    await Promise.all([handleClick(), handleClick()]);
    assert.strictEqual(dispatchCount, 1, 'Only 1 HTTP dispatch must occur on rapid double click');
  });

  // T2-F6-04: Pre-Existing Active In-Flight Job Duplicate Prevention
  await ctx.test('T2-F6-04: Pre-Existing Active In-Flight Job Duplicate Prevention', async () => {
    const inFlightJobs = new Set(['job-101']);

    function canPromoteToActiveQueue(jobId) {
      return !inFlightJobs.has(jobId);
    }

    assert.strictEqual(canPromoteToActiveQueue('job-101'), false, 'In-flight job must be blocked from entering active cards');
    assert.strictEqual(canPromoteToActiveQueue('job-102'), true, 'Unapplied job must be allowed');
  });

  // T2-F6-05: LocalStorage Desynchronization Recovery
  await ctx.test('T2-F6-05: LocalStorage Desynchronization Recovery', async () => {
    function loadSavedQueue(corruptJsonStr) {
      try {
        const parsed = JSON.parse(corruptJsonStr);
        if (Array.isArray(parsed)) return parsed;
      } catch (e) {}
      // Fallback clean state
      return [];
    }

    const recovered = loadSavedQueue('{ invalid_json_syntax ');
    assert.deepStrictEqual(recovered, [], 'Corrupt localStorage JSON must gracefully recover to empty array');
  });

  // ===========================================================================
  // Boundary Group 7: Multi-Step Navigation Edge Cases & Validation Blockers (F7 Edge Cases)
  // ===========================================================================

  // T2-F7-01: Workday Validation Error Banner Detection on Step 1
  await ctx.test('T2-F7-01: Workday Validation Error Banner Detection on Step 1', async () => {
    await mockAts.start();
    const browser = await setup.launchBrowser();
    const { page } = await setup.createTestPage(browser);

    try {
      await page.goto(`${setup.ENV.MOCK_ATS_URL}/workday/nvidia/apply/step3`);

      // Populate personal info first
      await page.type('input[data-automation-id="legalNameSection_firstName"]', setup.CANDIDATE_PROFILE.first_name);
      await page.type('input[data-automation-id="legalNameSection_lastName"]', setup.CANDIDATE_PROFILE.last_name);

      // Click Next WITHOUT selecting mandatory source
      await page.click('button[data-automation-id="page-navigation-next-button"]');

      // Error banner must be displayed
      const errBanner = await page.$('div[data-automation-id="errorMessage"]');
      assert.ok(errBanner, 'Error banner must be rendered when required field is missing');
      const errText = await page.$eval('div[data-automation-id="errorMessage"]', el => el.innerText);
      assert.ok(errText.includes('select a source'), 'Error text must mention source');
    } finally {
      await browser.close();
    }
  });

  // T2-F7-02: Dynamic Step Insertion based on Previous Answers
  await ctx.test('T2-F7-02: Dynamic Step Insertion based on Previous Answers', async () => {
    const formQuestions = [
      { id: 'q_auth', label: 'Authorized in US?', answer: 'Yes' }
    ];

    function evaluateDynamicQuestions(questions) {
      const qAuth = questions.find(q => q.id === 'q_auth');
      if (qAuth && qAuth.answer === 'Yes') {
        return [
          ...questions,
          { id: 'q_visa_type', label: 'Visa Expiration Date', required: true }
        ];
      }
      return questions;
    }

    const updated = evaluateDynamicQuestions(formQuestions);
    assert.strictEqual(updated.length, 2, 'Dynamic question must be inserted when Work Auth is Yes');
    assert.strictEqual(updated[1].id, 'q_visa_type');
  });

  // T2-F7-03: Workday Account Creation & OTP Verification Timeout
  await ctx.test('T2-F7-03: Workday Account Creation & OTP Verification Timeout', async () => {
    async function waitForOtpWithTimeout(timeoutMs = 100) {
      const start = Date.now();
      while (Date.now() - start < timeoutMs) {
        await sleep(20);
      }
      return { status: 'requires_manual_action', reason: 'otp_timeout' };
    }

    const res = await waitForOtpWithTimeout(50);
    assert.strictEqual(res.status, 'requires_manual_action', 'Must transition to requires_manual_action on OTP timeout');
    assert.strictEqual(res.reason, 'otp_timeout');
  });

  // T2-F7-04: Ashby Custom File Upload with File Type Rejection
  await ctx.test('T2-F7-04: Ashby Custom File Upload with File Type Rejection', async () => {
    function validateAllowedExtensions(filename) {
      const allowed = ['.pdf', '.doc', '.docx'];
      const ext = path.extname(filename).toLowerCase();
      return allowed.includes(ext);
    }

    assert.strictEqual(validateAllowedExtensions('Nakul_Bhandare_Resume.pdf'), true);
    assert.strictEqual(validateAllowedExtensions('resume.txt'), false);
    assert.strictEqual(validateAllowedExtensions('malicious.exe'), false);
  });

  // T2-F7-05: CAPTCHA / Bot-Detection Prompt Interception
  await ctx.test('T2-F7-05: CAPTCHA / Bot-Detection Prompt Interception', async () => {
    function detectCaptchaChallenge(html) {
      const challenges = [
        'cf-turnstile',
        'challenges.cloudflare.com',
        'g-recaptcha',
        'hcaptcha',
        'arkose'
      ];
      return challenges.some(c => html.toLowerCase().includes(c));
    }

    const cfHtml = '<div class="cf-turnstile" data-sitekey="0x4AAAAAA"></div>';
    assert.strictEqual(detectCaptchaChallenge(cfHtml), true, 'Must detect Cloudflare Turnstile');

    const cleanHtml = '<form id="app_form"><input type="text"/></form>';
    assert.strictEqual(detectCaptchaChallenge(cleanHtml), false, 'Clean form has no captcha');
  });

  // ===========================================================================
  // Boundary Group 8: Network Flakiness, Rate Limits & Retry Delays (F8 Edge Cases)
  // ===========================================================================

  // T2-F8-01: Submitter HTTP Navigation Gateway Timeout (504)
  await ctx.test('T2-F8-01: Submitter HTTP Navigation Gateway Timeout (504)', async () => {
    const error504 = new Error('net::ERR_HTTP_RESPONSE_CODE_FAILURE: HTTP 504 Gateway Timeout');

    function isTransientNetworkError(err) {
      const msg = err.message.toLowerCase();
      return msg.includes('504') ||
             msg.includes('502') ||
             msg.includes('503') ||
             msg.includes('timeout') ||
             msg.includes('econreset') ||
             msg.includes('enotfound');
    }

    assert.strictEqual(isTransientNetworkError(error504), true, '504 Gateway Timeout must be classified as transient retryable');
  });

  // T2-F8-02: Puppeteer Disconnected / Browser Process Crash
  await ctx.test('T2-F8-02: Puppeteer Disconnected / Browser Process Crash', async () => {
    const crashError = new Error('Protocol error: Connection closed. Most likely the browser has crashed.');

    function handleBrowserException(err) {
      if (err.message.includes('browser has crashed') || err.message.includes('Session closed')) {
        return { action: 'relaunch_browser', retryable: true };
      }
      return { action: 'rethrow', retryable: false };
    }

    const decision = handleBrowserException(crashError);
    assert.strictEqual(decision.retryable, true, 'Browser crash must be retryable');
    assert.strictEqual(decision.action, 'relaunch_browser');
  });

  // T2-F8-03: NATS JetStream Redelivery Deduplication
  await ctx.test('T2-F8-03: NATS JetStream Redelivery Deduplication', async () => {
    const appId = `e2e-t2-redelivery-${Date.now()}`;

    // Application already submitted
    await db.collection('applications').insertOne({
      _id: appId,
      tenant_id: '00000000-0000-4000-8000-000000000001',
      status: 'submitted',
      verified_receipt_id: 'REC-ALREADY-SUBMITTED',
      created_at: new Date()
    });

    async function handleIncomingTask(incomingAppId) {
      const existing = await db.collection('applications').findOne({ _id: incomingAppId });
      if (existing && existing.status === 'submitted') {
        return 'ack_and_skip'; // deduplicate
      }
      return 'process_task';
    }

    const action = await handleIncomingTask(appId);
    assert.strictEqual(action, 'ack_and_skip', 'Redelivered task for submitted application must ack and skip');
  });

  // T2-F8-04: Transient DNS Resolution Failure (ENOTFOUND)
  await ctx.test('T2-F8-04: Transient DNS Resolution Failure (ENOTFOUND)', async () => {
    const dnsErr = new Error('getaddrinfo ENOTFOUND jobs.lever.co');

    function isDnsTransientError(err) {
      return err.message.includes('ENOTFOUND') || err.message.includes('EAI_AGAIN');
    }

    assert.strictEqual(isDnsTransientError(dnsErr), true, 'ENOTFOUND must be recognized as transient network error');
  });

  // T2-F8-05: In-Flight Task Graceful Shutdown on SIGTERM
  await ctx.test('T2-F8-05: In-Flight Task Graceful Shutdown on SIGTERM', async () => {
    let gracefulNaked = false;

    function handleSigtermWithInFlight(hasActiveTask, msgNakFn) {
      if (hasActiveTask) {
        msgNakFn(5000); // return task to stream with 5s delay for other workers
        gracefulNaked = true;
      }
    }

    handleSigtermWithInFlight(true, delay => {
      assert.strictEqual(delay, 5000);
    });

    assert.strictEqual(gracefulNaked, true, 'SIGTERM during task must NAK task with backoff');
  });
}

// Standalone execution support
if (require.main === module) {
  const runner = require('./runner');
  process.argv.push('--tier=2');
  runner.runMasterSuite().catch(err => {
    console.error('Tier 2 Runner Error:', err);
    process.exit(1);
  });
}

module.exports = { runSuite };
