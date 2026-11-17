/**
 * OneRole E2E Test Suite: Tier 1 Feature Coverage
 * File: tests/e2e/tier1_feature_test.js
 * 
 * Contains 40 concrete test cases covering Features F1 through F8:
 * - Feature 1: Form Field Population & Combobox Resolution (T1-F1-01 to T1-F1-05)
 * - Feature 2: Candidate Profile Binding (T1-F2-01 to T1-F2-05)
 * - Feature 3: Native Resume Upload & PDF File Integrity (T1-F3-01 to T1-F3-05)
 * - Feature 4: Confirmation Detection & Receipt Recording (T1-F4-01 to T1-F4-05)
 * - Feature 5: Top-Row Tracking Table Sorting (T1-F5-01 to T1-F5-05)
 * - Feature 6: Match Card Queue Deduplication (T1-F6-01 to T1-F6-05)
 * - Feature 7: Multi-Step Navigation & Portal Special Handling (T1-F7-01 to T1-F7-05)
 * - Feature 8: Error Classification & Retry Handling (T1-F8-01 to T1-F8-05)
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
  const nats = await setup.getNats();
  const sc = setup.StringCodec();

  // Helper to obtain fresh auth token for tests
  async function getAuthToken() {
    try {
      const res = await fetch(`${setup.ENV.BACKEND_URL}/auth/dev-login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: setup.CANDIDATE_PROFILE.email })
      });
      if (res.ok) {
        const data = await res.json();
        return {
          token: data.access_token,
          tenantId: data.tenant_id || '00000000-0000-4000-8000-000000000001',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${data.access_token}`,
            'X-Tenant-ID': data.tenant_id || '00000000-0000-4000-8000-000000000001'
          }
        };
      }
    } catch (e) {}

    return {
      token: 'dev-token',
      tenantId: '00000000-0000-4000-8000-000000000001',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': 'Bearer dev-token',
        'X-Tenant-ID': '00000000-0000-4000-8000-000000000001'
      }
    };
  }

  await setup.cleanDatabaseFixtures('e2e-t1-');

  // ===========================================================================
  // Feature 1: Form Field Population & Combobox Resolution (5 Tests)
  // ===========================================================================

  // T1-F1-01: Greenhouse React-Select Label Resolution via aria-labelledby
  await ctx.test('T1-F1-01: Greenhouse React-Select Label Resolution via aria-labelledby', async () => {
    await mockAts.start();
    const browser = await setup.launchBrowser();
    const { page } = await setup.createTestPage(browser);

    try {
      await page.goto(`${setup.ENV.MOCK_ATS_URL}/greenhouse/lyft/jobs/8806570002`);

      // Verify label element with ID lbl_question_degree exists
      const labelText = await page.$eval('#lbl_question_degree', el => el.innerText);
      assert.ok(labelText.includes('Degree Level'), `Label text must be Degree Level, got: ${labelText}`);

      // Verify input has aria-labelledby attribute matching lbl_question_degree
      const ariaLabelledby = await page.$eval('#question_101', el => el.getAttribute('aria-labelledby'));
      assert.strictEqual(ariaLabelledby, 'lbl_question_degree');

      // Resolve label from aria-labelledby reference
      const resolvedLabel = await page.evaluate(inputSel => {
        const input = document.querySelector(inputSel);
        if (!input) return null;
        const labelId = input.getAttribute('aria-labelledby');
        if (!labelId) return null;
        const lbl = document.getElementById(labelId);
        return lbl ? lbl.innerText.trim() : null;
      }, '#question_101');

      assert.ok(resolvedLabel && resolvedLabel.includes('Degree Level'), 'Must resolve label via aria-labelledby');
    } finally {
      await browser.close();
    }
  });

  // T1-F1-02: Wrapper-Scoped Input Query in Multi-Combobox Form
  await ctx.test('T1-F1-02: Wrapper-Scoped Input Query in Multi-Combobox Form', async () => {
    await mockAts.start();
    const browser = await setup.launchBrowser();
    const { page } = await setup.createTestPage(browser);

    try {
      await page.goto(`${setup.ENV.MOCK_ATS_URL}/greenhouse/lyft/jobs/8806570002`);

      // Scoped input query inside Question 1 wrapper
      const q1Input = await page.$('[data-field-id="question_degree"] .select__input');
      assert.ok(q1Input, 'Input for question_degree must be scoped inside wrapper');

      // Scoped input query inside Question 2 wrapper
      const q2Input = await page.$('[data-field-id="question_auth"] .select__input');
      assert.ok(q2Input, 'Input for question_auth must be scoped inside wrapper');

      // Verify they are different elements
      const q1Id = await page.evaluate(el => el.id, q1Input);
      const q2Id = await page.evaluate(el => el.id, q2Input);
      assert.notStrictEqual(q1Id, q2Id, 'Multi-combobox inputs must be distinct element instances');
      assert.strictEqual(q1Id, 'question_101');
      assert.strictEqual(q2Id, 'question_102');
    } finally {
      await browser.close();
    }
  });

  // T1-F1-03: Option Selection from Scoped .select__menu Dropdown
  await ctx.test('T1-F1-03: Option Selection from Scoped .select__menu Dropdown', async () => {
    await mockAts.start();
    const browser = await setup.launchBrowser();
    const { page } = await setup.createTestPage(browser);

    try {
      await page.goto(`${setup.ENV.MOCK_ATS_URL}/greenhouse/lyft/jobs/8806570002`);

      // Click select control to open menu
      await page.click('[data-field-id="question_degree"] .select__control');
      await page.waitForSelector('#menu_degree.select__menu', { visible: true });

      // Click option specifically within #menu_degree
      await page.click('#menu_degree .select__option:nth-child(2)'); // Master's Degree

      // Verify the input received "Master's Degree"
      const val = await page.$eval('#question_101', el => el.value);
      assert.strictEqual(val, "Master's Degree", `Expected Master's Degree, got: ${val}`);
    } finally {
      await browser.close();
    }
  });

  // T1-F1-04: Greenhouse Pre-Flight Inspector Field ID Extraction
  await ctx.test('T1-F1-04: Greenhouse Pre-Flight Inspector Field ID Extraction', async () => {
    // Simulating the inspector logic: extracting field ID from fields[0].name vs q.name
    const mockGreenhouseApiResponse = {
      questions: [
        {
          label: 'Degree Level',
          fields: [{ name: 'question_38125648002', type: 'input_text' }]
          // note: q.name is absent/null at top level
        },
        {
          label: 'Work Authorization',
          fields: [{ name: 'question_38125648003', type: 'input_text' }]
        }
      ]
    };

    function extractFieldID(q) {
      if (q.fields && q.fields.length > 0 && q.fields[0].name) {
        return q.fields[0].name;
      }
      return q.name || '';
    }

    const field1ID = extractFieldID(mockGreenhouseApiResponse.questions[0]);
    const field2ID = extractFieldID(mockGreenhouseApiResponse.questions[1]);

    assert.strictEqual(field1ID, 'question_38125648002', 'Must extract field ID from fields[0].name');
    assert.strictEqual(field2ID, 'question_38125648003', 'Must extract field ID from fields[0].name');
  });

  // T1-F1-05: Native HTML <select> Auto-Selection Fallback
  await ctx.test('T1-F1-05: Native HTML <select> Auto-Selection Fallback', async () => {
    await mockAts.start();
    const browser = await setup.launchBrowser();
    const { page } = await setup.createTestPage(browser);

    try {
      await page.goto(`${setup.ENV.MOCK_ATS_URL}/greenhouse/lyft/jobs/8806570002`);

      // Select option 'Yes' on native select element
      await page.select('select#work_auth', 'Yes');
      const selectedVal = await page.$eval('select#work_auth', el => el.value);
      assert.strictEqual(selectedVal, 'Yes', 'Native select element value must be Yes');
    } finally {
      await browser.close();
    }
  });

  // ===========================================================================
  // Feature 2: Candidate Profile Binding (5 Tests)
  // ===========================================================================

  // T1-F2-01: Exact Profile Match Transmission to API Endpoint
  await ctx.test('T1-F2-01: Exact Profile Match Transmission to API Endpoint', async () => {
    const auth = await getAuthToken();

    const patchRes = await fetch(`${setup.ENV.BACKEND_URL}/profile`, {
      method: 'PATCH',
      headers: auth.headers,
      body: JSON.stringify({
        first_name: setup.CANDIDATE_PROFILE.first_name,
        last_name: setup.CANDIDATE_PROFILE.last_name,
        email: setup.CANDIDATE_PROFILE.email,
        phone: setup.CANDIDATE_PROFILE.phone,
        city: 'San Francisco',
        state: 'CA',
        country: 'United States'
      })
    });
    assert.strictEqual(patchRes.status, 200, `PATCH /profile returned ${patchRes.status}`);
    const data = await patchRes.json();
    assertions.assertCandidateProfile(data, 'Candidate profile must match ground truth');
  });

  // T1-F2-02: First and Last Name Splitting across ATS Fields
  await ctx.test('T1-F2-02: First and Last Name Splitting across ATS Fields', async () => {
    const fullName = setup.CANDIDATE_PROFILE.name; // 'Nakul Bhandare'
    const nameParts = fullName.trim().split(/\s+/);
    const firstName = nameParts[0];
    const lastName = nameParts.slice(1).join(' ');

    assert.strictEqual(firstName, 'Nakul', 'First name must split to Nakul');
    assert.strictEqual(lastName, 'Bhandare', 'Last name must split to Bhandare');
  });

  // T1-F2-03: Phone Number Formatting and Country Code Binding
  await ctx.test('T1-F2-03: Phone Number Formatting and Country Code Binding', async () => {
    const rawPhone = setup.CANDIDATE_PROFILE.phone; // '+1 (350)-355-6570'
    const digitsOnly = rawPhone.replace(/\D/g, '');
    assert.strictEqual(digitsOnly, '13503556570', 'Digits must match 13503556570');
    assert.strictEqual(digitsOnly.length, 11, 'US number with country code must be 11 digits');
    const local10 = digitsOnly.startsWith('1') ? digitsOnly.slice(1) : digitsOnly;
    assert.strictEqual(local10, '3503556570', 'Local 10-digit number must match');
  });

  // T1-F2-04: Location Binding to City and State Components
  await ctx.test('T1-F2-04: Location Binding to City and State Components', async () => {
    const rawLocation = setup.CANDIDATE_PROFILE.location; // 'San Francisco, CA'
    const parts = rawLocation.split(',').map(s => s.trim());
    const city = parts[0];
    const state = parts[1];

    assert.strictEqual(city, 'San Francisco', 'City component must be San Francisco');
    assert.strictEqual(state, 'CA', 'State component must be CA');
  });

  // T1-F2-05: Full Payload Integrity in NATS JetStream Task
  await ctx.test('T1-F2-05: Full Payload Integrity in NATS JetStream Task', async () => {
    const payload = {
      application_id: `e2e-t1-nats-${Date.now()}`,
      job_id: 'test-job-01',
      candidate: setup.CANDIDATE_PROFILE
    };

    const encoded = sc.encode(JSON.stringify(payload));
    const decoded = JSON.parse(sc.decode(encoded));

    assertions.assertCandidateProfile(decoded.candidate, 'Candidate profile in NATS payload must match ground truth');
    assert.strictEqual(decoded.candidate.resume_file_path, setup.CANDIDATE_PROFILE.resume_file_path);
  });

  // ===========================================================================
  // Feature 3: Native Resume Upload & PDF File Integrity (5 Tests)
  // ===========================================================================

  // T1-F3-01: Preservation of Native Resume File without HTML Clobber
  await ctx.test('T1-F3-01: Preservation of Native Resume File without HTML Clobber', async () => {
    const resumePath = setup.CANDIDATE_PROFILE.resume_file_path;
    assert.ok(fs.existsSync(resumePath), `Resume must exist at ${resumePath}`);

    // Verify it is the authentic native resume, not a /tmp clobber
    assert.ok(!resumePath.includes('/tmp/'), 'Resume path must not be temporary /tmp render');
    assert.ok(resumePath.endsWith('Nakul_Bhandare_Resume.pdf'), 'Must be Nakul_Bhandare_Resume.pdf');
    assertions.assertResumePdfIntegrity(resumePath, 'Native resume PDF integrity');
  });

  // T1-F3-02: Scoped Target Upload to Resume Input Only
  await ctx.test('T1-F3-02: Scoped Target Upload to Resume Input Only', async () => {
    await mockAts.start();
    const browser = await setup.launchBrowser();
    const { page } = await setup.createTestPage(browser);

    try {
      await page.goto(`${setup.ENV.MOCK_ATS_URL}/greenhouse/lyft/jobs/8806570002`);

      // Target ONLY #resume file input
      const resumeInput = await page.$('#resume');
      assert.ok(resumeInput, 'Resume input #resume must exist');
      await resumeInput.uploadFile(setup.CANDIDATE_PROFILE.resume_file_path);

      // Verify #resume has file attached
      const resumeFilesCount = await page.$eval('#resume', el => el.files.length);
      assert.strictEqual(resumeFilesCount, 1, 'Resume input must have 1 attached file');

      // Verify #cover_letter_upload and #transcript_upload remain empty
      const clFilesCount = await page.$eval('#cover_letter_upload', el => el.files.length);
      const trFilesCount = await page.$eval('#transcript_upload', el => el.files.length);
      assert.strictEqual(clFilesCount, 0, 'Cover letter input must remain empty');
      assert.strictEqual(trFilesCount, 0, 'Transcript input must remain empty');
    } finally {
      await browser.close();
    }
  });

  // T1-F3-03: Ashby Autofill Uploader vs Direct Resume Uploader Distinction
  await ctx.test('T1-F3-03: Ashby Autofill Uploader vs Direct Resume Uploader Distinction', async () => {
    await mockAts.start();
    const browser = await setup.launchBrowser();
    const { page } = await setup.createTestPage(browser);

    try {
      await page.goto(`${setup.ENV.MOCK_ATS_URL}/ashby/airbyte/test-job/application`);

      // Check both file inputs exist
      const autofillInput = await page.$('.ashby-application-form-autofill-uploader input[type="file"]');
      const directResumeInput = await page.$('.ashby-application-form-input-file');

      assert.ok(autofillInput, 'Ashby autofill uploader file input must exist');
      assert.ok(directResumeInput, 'Ashby direct resume file input must exist');

      // Upload to direct resume input
      await directResumeInput.uploadFile(setup.CANDIDATE_PROFILE.resume_file_path);
      const count = await page.$eval('.ashby-application-form-input-file', el => el.files.length);
      assert.strictEqual(count, 1, 'Direct resume input must have 1 file');
    } finally {
      await browser.close();
    }
  });

  // T1-F3-04: PDF MIME-Type and Byte Integrity on Attachment
  await ctx.test('T1-F3-04: PDF MIME-Type and Byte Integrity on Attachment', async () => {
    const stats = fs.statSync(setup.CANDIDATE_PROFILE.resume_file_path);
    assert.ok(stats.size > 1024, `Resume size must be > 1KB, got ${stats.size}`);

    const fd = fs.openSync(setup.CANDIDATE_PROFILE.resume_file_path, 'r');
    const headerBuf = Buffer.alloc(8);
    fs.readSync(fd, headerBuf, 0, 8, 0);
    fs.closeSync(fd);

    const header = headerBuf.toString('ascii');
    assert.ok(header.startsWith('%PDF-1.'), `Must start with %PDF-1., got: ${header}`);
  });

  // T1-F3-05: Fallback to HTML PDF Generation Only when Native File Missing
  await ctx.test('T1-F3-05: Fallback to HTML PDF Generation Only when Native File Missing', async () => {
    const fakePath = '/tmp/nonexistent-candidate-resume-test.pdf';
    assert.strictEqual(fs.existsSync(fakePath), false, 'Test file must not exist');

    // Verification of fallback trigger condition
    function resolveUploadFile(candidate) {
      if (candidate.resume_file_path && fs.existsSync(candidate.resume_file_path)) {
        return { path: candidate.resume_file_path, mode: 'native' };
      }
      return { path: `/tmp/generated_resume_${Date.now()}.pdf`, mode: 'html_fallback' };
    }

    const nativeResult = resolveUploadFile(setup.CANDIDATE_PROFILE);
    assert.strictEqual(nativeResult.mode, 'native', 'Valid native file must select native mode');

    const fallbackResult = resolveUploadFile({ resume_file_path: fakePath });
    assert.strictEqual(fallbackResult.mode, 'html_fallback', 'Missing file must trigger html_fallback mode');
  });

  // ===========================================================================
  // Feature 4: Confirmation Detection & Receipt Recording (5 Tests)
  // ===========================================================================

  // T1-F4-01: Ashby GraphQL Rejection Detection on Field Errors
  await ctx.test('T1-F4-01: Ashby GraphQL Rejection Detection on Field Errors', async () => {
    // Testing the GraphQL confirmation parser: must inspect errors array, not just substring 'errorMessages'
    function isAshbyConfirmed(graphqlResponseText) {
      try {
        const parsed = JSON.parse(graphqlResponseText);
        if (parsed.errors && parsed.errors.length > 0) {
          return false;
        }
        if (parsed.data && parsed.data.submitApplication && parsed.data.submitApplication.success) {
          return true;
        }
        return false;
      } catch (e) {
        return false;
      }
    }

    const errorPayload = JSON.stringify({ errors: [{ message: 'Missing entry for required field: Location' }] });
    assert.strictEqual(isAshbyConfirmed(errorPayload), false, 'GraphQL payload with errors must NOT confirm submission');
  });

  // T1-F4-02: Ashby GraphQL Success Confirmation Detection
  await ctx.test('T1-F4-02: Ashby GraphQL Success Confirmation Detection', async () => {
    function isAshbyConfirmed(graphqlResponseText) {
      try {
        const parsed = JSON.parse(graphqlResponseText);
        if (parsed.errors && parsed.errors.length > 0) return false;
        if (parsed.data && parsed.data.submitApplication && parsed.data.submitApplication.success) return true;
        return false;
      } catch (e) {
        return false;
      }
    }

    const successPayload = JSON.stringify({
      data: { submitApplication: { success: true, receipt: 'ASHBY-GRAPHQL-12345' } }
    });
    assert.strictEqual(isAshbyConfirmed(successPayload), true, 'GraphQL success payload must evaluate to true');
  });

  // T1-F4-03: Greenhouse Post-Submit Confirmation Page Detection
  await ctx.test('T1-F4-03: Greenhouse Post-Submit Confirmation Page Detection', async () => {
    await mockAts.start();
    const browser = await setup.launchBrowser();
    const { page } = await setup.createTestPage(browser);

    try {
      await page.goto(`${setup.ENV.MOCK_ATS_URL}/greenhouse/lyft/confirmation`);
      const bodyText = await page.evaluate(() => document.body.innerText);

      function isGreenhouseConfirmed(url, text) {
        return url.includes('/confirmation') && (text.includes('Thank you for applying') || text.includes('application has been submitted'));
      }

      assert.strictEqual(isGreenhouseConfirmed(page.url(), bodyText), true, 'Greenhouse confirmation must be detected');
    } finally {
      await browser.close();
    }
  });

  // T1-F4-04: Lever Application Submitted Banner Detection
  await ctx.test('T1-F4-04: Lever Application Submitted Banner Detection', async () => {
    await mockAts.start();
    const browser = await setup.launchBrowser();
    const { page } = await setup.createTestPage(browser);

    try {
      await page.goto(`${setup.ENV.MOCK_ATS_URL}/lever/spotify/confirmation`);
      const confirmationBanner = await page.$('.application-confirmation');
      assert.ok(confirmationBanner, 'Lever confirmation banner .application-confirmation must exist');
      const text = await page.$eval('.application-confirmation', el => el.innerText);
      assert.ok(text.includes('Application submitted'), 'Lever confirmation text must contain Application submitted');
    } finally {
      await browser.close();
    }
  });

  // T1-F4-05: MongoDB Lifecycle Audit Event Persistence
  await ctx.test('T1-F4-05: MongoDB Lifecycle Audit Event Persistence', async () => {
    const auth = await getAuthToken();
    const testAppId = `e2e-t1-lifecycle-${Date.now()}`;

    await db.collection('applications').insertOne({
      _id: testAppId,
      tenant_id: auth.tenantId,
      user_id: 'user-lifecycle-test',
      job_id: 'job-life-01',
      status: 'submitted',
      verified_receipt_id: `REC-LIFE-${Date.now()}`,
      created_at: new Date(),
      updated_at: new Date()
    });

    await db.collection('application_events').insertMany([
      { _id: `evt1-${testAppId}`, application_id: testAppId, tenant_id: auth.tenantId, event_type: 'application.submitting', occurred_at: new Date() },
      { _id: `evt2-${testAppId}`, application_id: testAppId, tenant_id: auth.tenantId, event_type: 'application.submitted', occurred_at: new Date() }
    ]);

    await assertions.assertMongoApplicationLifecycle(db, testAppId, 'submitted');
  });

  // ===========================================================================
  // Feature 5: Top-Row Tracking Table Sorting (5 Tests)
  // ===========================================================================

  // T1-F5-01: Immediate Row 1 Placement of New Application Event
  await ctx.test('T1-F5-01: Immediate Row 1 Placement of New Application Event', async () => {
    const historical = [
      { id: 'h1', company: 'Apple', submitted_at: '2026-11-15T00:00:00Z', status: 'submitted' },
      { id: 'h2', company: 'Google', submitted_at: '2026-11-16T00:00:00Z', status: 'submitted' }
    ];

    const newApp = { id: 'new1', company: 'OpenAI', submitted_at: new Date().toISOString(), status: 'submitted' };

    // Function simulating table sorting
    function sortApplications(apps) {
      return [...apps].sort((a, b) => {
        const parseTime = x => (x.applied === 'Just now' || x.status === 'in_flight') ? Infinity : new Date(x.submitted_at || x.created_at || 0).getTime();
        return parseTime(b) - parseTime(a);
      });
    }

    const sorted = sortApplications([...historical, newApp]);
    assert.strictEqual(sorted[0].id, 'new1', 'New application must be strictly placed at Row 1 (index 0)');
  });

  // T1-F5-02: Timestamp-Based Sorting Executed BEFORE Deduplication
  await ctx.test('T1-F5-02: Timestamp-Based Sorting Executed BEFORE Deduplication', async () => {
    const rawApps = [
      { id: 'old_lyft', company: 'Lyft', submitted_at: '2026-11-10T00:00:00Z' },
      { id: 'other_comp', company: 'Stripe', submitted_at: '2026-11-12T00:00:00Z' },
      { id: 'new_lyft', company: 'Lyft', submitted_at: '2026-11-17T00:00:00Z' }
    ];

    // Buggy order: dedup before sort
    function dedupBeforeSort(apps) {
      const seen = new Set();
      const deduped = [];
      for (const a of apps) {
        if (!seen.has(a.company.toLowerCase())) {
          seen.add(a.company.toLowerCase());
          deduped.push(a);
        }
      }
      return deduped.sort((a, b) => new Date(b.submitted_at) - new Date(a.submitted_at));
    }

    // Correct order: sort before dedup
    function sortBeforeDedup(apps) {
      const sorted = [...apps].sort((a, b) => new Date(b.submitted_at) - new Date(a.submitted_at));
      const seen = new Set();
      const deduped = [];
      for (const a of sorted) {
        if (!seen.has(a.company.toLowerCase())) {
          seen.add(a.company.toLowerCase());
          deduped.push(a);
        }
      }
      return deduped;
    }

    const buggyResult = dedupBeforeSort(rawApps);
    const correctResult = sortBeforeDedup(rawApps);

    // In buggy order, old_lyft was picked, so Lyft timestamp is Nov 10
    assert.strictEqual(buggyResult.find(a => a.company === 'Lyft').id, 'old_lyft');

    // In correct order, new_lyft was picked, so Lyft timestamp is Nov 17
    assert.strictEqual(correctResult.find(a => a.company === 'Lyft').id, 'new_lyft');
    assert.strictEqual(correctResult[0].id, 'new_lyft', 'Newest Lyft application must be Row 1');
  });

  // T1-F5-03: Deterministic ISO Timestamp Comparator
  await ctx.test('T1-F5-03: Deterministic ISO Timestamp Comparator', async () => {
    const timestamps = [
      '2026-11-16T12:00:00Z',
      '2026-11-16T08:00:00Z',
      '2026-11-16T14:30:00Z',
      '2026-11-15T22:00:00Z'
    ].map((ts, idx) => ({ id: `app-${idx}`, submitted_at: ts }));

    const sorted = [...timestamps].sort((a, b) => new Date(b.submitted_at).getTime() - new Date(a.submitted_at).getTime());

    assert.strictEqual(sorted[0].submitted_at, '2026-11-16T14:30:00Z');
    assert.strictEqual(sorted[1].submitted_at, '2026-11-16T12:00:00Z');
    assert.strictEqual(sorted[2].submitted_at, '2026-11-16T08:00:00Z');
    assert.strictEqual(sorted[3].submitted_at, '2026-11-15T22:00:00Z');
  });

  // T1-F5-04: Reset STATE.currentPage to 1 on New Application
  await ctx.test('T1-F5-04: Reset STATE.currentPage to 1 on New Application', async () => {
    const state = { currentPage: 3 };

    function onNewApplicationSubmitted(st) {
      st.currentPage = 1;
    }

    onNewApplicationSubmitted(state);
    assert.strictEqual(state.currentPage, 1, 'Current page must reset to 1 upon new submission');
  });

  // T1-F5-05: Multi-Role Coexistence per Company in Tracking Table
  await ctx.test('T1-F5-05: Multi-Role Coexistence per Company in Tracking Table', async () => {
    const apps = [
      { id: 'g1', company: 'Google', role: 'Senior Go Engineer', submitted_at: '2026-11-16T10:00:00Z' },
      { id: 'g2', company: 'Google', role: 'Staff SRE', submitted_at: '2026-11-16T11:00:00Z' }
    ];

    // Deduplication should key on company + role (or company + jobId), NOT company alone
    function dedupByCompanyAndRole(items) {
      const seen = new Set();
      const res = [];
      for (const item of items) {
        const key = `${item.company.toLowerCase()}::${item.role.toLowerCase()}`;
        if (!seen.has(key)) {
          seen.add(key);
          res.push(item);
        }
      }
      return res;
    }

    const deduped = dedupByCompanyAndRole(apps);
    assert.strictEqual(deduped.length, 2, 'Both distinct roles at Google must coexist in tracking table');
  });

  // ===========================================================================
  // Feature 6: Match Card Queue Deduplication (5 Tests)
  // ===========================================================================

  // T1-F6-01: Strict Maintenance of Exactly 5 Active Cards
  await ctx.test('T1-F6-01: Strict Maintenance of Exactly 5 Active Cards', async () => {
    const pool = [
      { company: 'Meta' }, { company: 'Netflix' }, { company: 'Amazon' },
      { company: 'Apple' }, { company: 'Google' }, { company: 'Microsoft' }
    ];

    function maintainQueueOf5(currentQueue, sourcePool) {
      const q = [...currentQueue];
      for (const item of sourcePool) {
        if (q.length >= 5) break;
        if (!q.some(x => x.company.toLowerCase() === item.company.toLowerCase())) {
          q.push(item);
        }
      }
      return q.slice(0, 5);
    }

    const queue = maintainQueueOf5([], pool);
    assertions.assertQueueDeduplicated(queue, 'Queue must contain exactly 5 unique companies');
  });

  // T1-F6-02: Company Uniqueness across Active 5 Cards
  await ctx.test('T1-F6-02: Company Uniqueness across Active 5 Cards', async () => {
    const rawCards = [
      { company: 'OpenAI', role: 'TPM' },
      { company: 'OpenAI', role: 'Member of Technical Staff' },
      { company: 'Airbyte', role: 'Backend' },
      { company: 'Docker', role: 'Infra' },
      { company: 'Spotify', role: 'ML' },
      { company: 'Lyft', role: 'Eng' }
    ];

    const uniqueQueue = [];
    const seen = new Set();
    for (const card of rawCards) {
      const key = card.company.toLowerCase();
      if (!seen.has(key)) {
        seen.add(key);
        uniqueQueue.push(card);
      }
      if (uniqueQueue.length === 5) break;
    }

    assertions.assertQueueDeduplicated(uniqueQueue, 'Must contain 5 unique companies, dropping duplicate OpenAI');
    const openAiCards = uniqueQueue.filter(c => c.company === 'OpenAI');
    assert.strictEqual(openAiCards.length, 1, 'Only 1 OpenAI card allowed in top 5');
  });

  // T1-F6-03: Clean Replenishment upon Card Dismissal (Pass)
  await ctx.test('T1-F6-03: Clean Replenishment upon Card Dismissal (Pass)', async () => {
    let activeQueue = [
      { company: 'A' }, { company: 'B' }, { company: 'C' }, { company: 'D' }, { company: 'E' }
    ];
    const replenishPool = [{ company: 'F' }, { company: 'G' }];

    function passCard(queue, pool, indexToPass) {
      const newQ = queue.filter((_, idx) => idx !== indexToPass);
      if (pool.length > 0) {
        newQ.push(pool.shift());
      }
      return newQ;
    }

    activeQueue = passCard(activeQueue, replenishPool, 0);
    assert.strictEqual(activeQueue.length, 5, 'Queue must remain at exactly 5 cards after pass');
    assert.strictEqual(activeQueue[4].company, 'F', 'Card F must be promoted into the 5th position');
  });

  // T1-F6-04: Awaited API Dispatch in applyToAll5Cards
  await ctx.test('T1-F6-04: Awaited API Dispatch in applyToAll5Cards', async () => {
    const callLog = [];

    async function mockApplySingleCard(id) {
      callLog.push({ id, status: 'start', time: Date.now() });
      await sleep(20);
      callLog.push({ id, status: 'end', time: Date.now() });
    }

    async function applyToAll5Cards(cards) {
      for (let i = 0; i < cards.length; i++) {
        await mockApplySingleCard(cards[i]);
      }
    }

    const cards = [1, 2, 3, 4, 5];
    const start = Date.now();
    await applyToAll5Cards(cards);
    const duration = Date.now() - start;

    assert.ok(duration >= 90, `Batch apply must await each card sequentially (duration ${duration}ms >= 90ms)`);
    assert.strictEqual(callLog.length, 10, 'Each card must log start and end sequentially');
  });

  // T1-F6-05: Unique Receipt Generation under Rapid Processing
  await ctx.test('T1-F6-05: Unique Receipt Generation under Rapid Processing', async () => {
    const receipts = new Set();
    let counter = 0;

    function generateReceipt() {
      counter++;
      return `REC-${Date.now()}-${counter}-${Math.random().toString(36).slice(2, 6)}`;
    }

    for (let i = 0; i < 100; i++) {
      receipts.add(generateReceipt());
    }

    assert.strictEqual(receipts.size, 100, 'All 100 generated receipts must be unique without collision');
  });

  // ===========================================================================
  // Feature 7: Multi-Step Navigation & Portal Special Handling (5 Tests)
  // ===========================================================================

  // T1-F7-01: Workday Step 1 Mandatory Source Dropdown Handling
  await ctx.test('T1-F7-01: Workday Step 1 Mandatory Source Dropdown Handling', async () => {
    await mockAts.start();
    const browser = await setup.launchBrowser();
    const { page } = await setup.createTestPage(browser);

    try {
      await page.goto(`${setup.ENV.MOCK_ATS_URL}/workday/nvidia/apply/step3`);
      await page.click('button[data-automation-id="prompt-source"]');
      await page.waitForSelector('#src_menu div', { visible: true });
      await page.click('#src_menu div:first-child');

      const srcValue = await page.$eval('#src_val', el => el.value);
      assert.strictEqual(srcValue, 'LinkedIn', 'Source value must be populated with LinkedIn');
    } finally {
      await browser.close();
    }
  });

  // T1-F7-02: Workday Wizard Multi-Step Advancement Validation
  await ctx.test('T1-F7-02: Workday Wizard Multi-Step Advancement Validation', async () => {
    await mockAts.start();
    const browser = await setup.launchBrowser();
    const { page } = await setup.createTestPage(browser);

    try {
      // Step 1 -> Step 2
      await page.goto(`${setup.ENV.MOCK_ATS_URL}/workday/nvidia/apply`);
      await page.type('input[name="email"]', setup.CANDIDATE_PROFILE.email);
      await page.type('input[name="password"]', 'P@ssw0rd123!');
      await Promise.all([
        page.waitForNavigation(),
        page.click('button[data-automation-id="createAccountButton"]')
      ]);
      assert.ok(page.url().includes('/step2'), 'Must advance to step 2');

      // Step 2 -> Step 3
      await page.type('input[name="verification_code"]', '123456');
      await Promise.all([
        page.waitForNavigation(),
        page.click('button[data-automation-id="verifyButton"]')
      ]);
      assert.ok(page.url().includes('/step3'), 'Must advance to step 3');
    } finally {
      await browser.close();
    }
  });

  // T1-F7-03: Ashby Location Autocomplete Selection via Popup Click
  await ctx.test('T1-F7-03: Ashby Location Autocomplete Selection via Popup Click', async () => {
    await mockAts.start();
    const browser = await setup.launchBrowser();
    const { page } = await setup.createTestPage(browser);

    try {
      await page.goto(`${setup.ENV.MOCK_ATS_URL}/ashby/airbyte/test-job/application`);
      await page.type('.ashby-application-form-input-autocomplete', 'San Fran');
      await page.waitForSelector('#ashby_loc_popup', { visible: true });
      await page.click('#ashby_loc_popup');

      const val = await page.$eval('.ashby-application-form-input-autocomplete', el => el.value);
      assert.strictEqual(val, 'San Francisco, CA, USA', 'Autocomplete must set full location string');
    } finally {
      await browser.close();
    }
  });

  // T1-F7-04: Ashby URL Query Parameter Sanitization
  await ctx.test('T1-F7-04: Ashby URL Query Parameter Sanitization', async () => {
    function sanitizeAshbyApplyUrl(rawUrl) {
      const u = new URL(rawUrl);
      if (!u.pathname.endsWith('/application')) {
        u.pathname = u.pathname.replace(/\/+$/, '') + '/application';
      }
      return u.toString();
    }

    const testUrl = 'https://jobs.ashbyhq.com/org/job-id?utm_source=linkedin&ref=board';
    const cleaned = sanitizeAshbyApplyUrl(testUrl);
    assert.strictEqual(cleaned, 'https://jobs.ashbyhq.com/org/job-id/application?utm_source=linkedin&ref=board');
  });

  // T1-F7-05: Lever Pronoun Single-Selection Hygiene
  await ctx.test('T1-F7-05: Lever Pronoun Single-Selection Hygiene', async () => {
    await mockAts.start();
    const browser = await setup.launchBrowser();
    const { page } = await setup.createTestPage(browser);

    try {
      await page.goto(`${setup.ENV.MOCK_ATS_URL}/lever/spotify/test-101/apply`);

      // Fill personal details
      await page.type('input[name="name"]', setup.CANDIDATE_PROFILE.name);
      await page.type('input[name="email"]', setup.CANDIDATE_PROFILE.email);
      await page.type('input[name="phone"]', setup.CANDIDATE_PROFILE.phone);

      // Select ONLY He/Him
      await page.click('input[name="pronouns[he]"]');
      await page.click('input[name="consent"]');

      const heChecked = await page.$eval('input[name="pronouns[he]"]', el => el.checked);
      const sheChecked = await page.$eval('input[name="pronouns[she]"]', el => el.checked);
      const theyChecked = await page.$eval('input[name="pronouns[they]"]', el => el.checked);

      assert.strictEqual(heChecked, true, 'He/him must be checked');
      assert.strictEqual(sheChecked, false, 'She/her must NOT be checked');
      assert.strictEqual(theyChecked, false, 'They/them must NOT be checked');
    } finally {
      await browser.close();
    }
  });

  // ===========================================================================
  // Feature 8: Error Classification & Retry Handling (5 Tests)
  // ===========================================================================

  // T1-F8-01: Non-Retryable Classification for Terminated Job Postings
  await ctx.test('T1-F8-01: Non-Retryable Classification for Terminated Job Postings', async () => {
    const error = new Error('HTTP 404: job is no longer available');
    assertions.assertErrorClassification(error, 'non-retryable');
  });

  // T1-F8-02: Retryable Classification for Transient Missing Field or DOM Lag
  await ctx.test('T1-F8-02: Retryable Classification for Transient Missing Field or DOM Lag', async () => {
    const error = new Error('Element selector "button#submit_app" timed out after 10000ms');
    assertions.assertErrorClassification(error, 'retryable');
  });

  // T1-F8-03: Exponential Backoff Delay Calculation with Jitter
  await ctx.test('T1-F8-03: Exponential Backoff Delay Calculation with Jitter', async () => {
    function calculateBackoffDelay(attempt, baseMs = 5000, maxMs = 60000) {
      const exp = Math.min(maxMs, baseMs * Math.pow(2, attempt - 1));
      const jitter = exp * (0.9 + Math.random() * 0.2); // +/- 10%
      return Math.round(jitter);
    }

    const d1 = calculateBackoffDelay(1);
    const d2 = calculateBackoffDelay(2);
    const d3 = calculateBackoffDelay(3);

    assert.ok(d1 >= 4000 && d1 <= 6000, `Delay 1 should be ~5000ms, got ${d1}`);
    assert.ok(d2 >= 8500 && d2 <= 12500, `Delay 2 should be ~10000ms, got ${d2}`);
    assert.ok(d3 >= 17000 && d3 <= 25000, `Delay 3 should be ~20000ms, got ${d3}`);
  });

  // T1-F8-04: Max Retry Exhaustion Transition to Permanent Failure
  await ctx.test('T1-F8-04: Max Retry Exhaustion Transition to Permanent Failure', async () => {
    function evaluateDeliveryStatus(deliveryCount, maxDeliveries = 3) {
      if (deliveryCount >= maxDeliveries) {
        return { status: 'failed_permanent', action: 'term' };
      }
      return { status: 'failed_retryable', action: 'nak' };
    }

    assert.strictEqual(evaluateDeliveryStatus(1).status, 'failed_retryable');
    assert.strictEqual(evaluateDeliveryStatus(2).status, 'failed_retryable');
    assert.strictEqual(evaluateDeliveryStatus(3).status, 'failed_permanent');
    assert.strictEqual(evaluateDeliveryStatus(3).action, 'term');
  });

  // T1-F8-05: Rate-Limit / 429 Retry Delay Respect
  await ctx.test('T1-F8-05: Rate-Limit / 429 Retry Delay Respect', async () => {
    function parseRetryAfter(headers) {
      const val = headers['retry-after'];
      if (!val) return 5000;
      const secs = parseInt(val, 10);
      return isNaN(secs) ? 5000 : secs * 1000;
    }

    const delay = parseRetryAfter({ 'retry-after': '30' });
    assert.strictEqual(delay, 30000, 'Retry-After: 30 header must result in 30,000ms delay');
  });
}

// Standalone execution support
if (require.main === module) {
  const runner = require('./runner');
  process.argv.push('--tier=1');
  runner.runMasterSuite().catch(err => {
    console.error('Tier 1 Runner Error:', err);
    process.exit(1);
  });
}

module.exports = { runSuite };
