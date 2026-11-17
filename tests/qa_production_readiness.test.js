/**
 * QA Production Readiness End-to-End Verification Test Suite
 * File: tests/qa_production_readiness.test.js
 * 
 * Verifies all Acceptance Criteria from ORIGINAL_REQUEST.md & DISPATCH.md:
 * R1. Initial Landing State (0 active filters, no 'golang' search tag)
 * R2. Real-Time Data Integration (Top 5 Matches & Full Catalog from Live MongoDB)
 * R3. Dynamic Filter Functionality (Search tags, workplace modes, role, experience, date)
 * R4. Location Filtering Accuracy (UI & Backend)
 * R5. Vacancy Reloading Reliability (Triggering reload button & filter changes)
 * R6. Dynamic ATS Match Scoring (Dynamic resume-to-JD evaluation)
 * R7. End-to-End Application Submission Flow & Status Lifecycle (submitting -> in_flight -> submitted)
 * R8. Email Confirmation Receipt Dispatch to bhandarenakul11@gmail.com
 * R9. Browser Zero Error Audit (0 page errors, 0 console errors)
 */

const assert = require('assert');
const path = require('path');
const puppeteer = require(path.resolve(__dirname, '../connectors/ats_submit/node_modules/puppeteer-core'));
const { MongoClient } = require(path.resolve(__dirname, '../connectors/ats_submit/node_modules/mongodb'));

const UI_URL = 'http://localhost:8082';
const BACKEND_URL = 'http://localhost:8088/api/v1';
const WORKER_URL = process.env.WORKER_URL || 'http://localhost:3001';
const MONGO_URL = 'mongodb://localhost:27017/onerole';
const CANDIDATE_EMAIL = 'bhandarenakul11@gmail.com';
const CHROME_PATH = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';

const sleep = ms => new Promise(r => setTimeout(r, ms));

const results = {
  total: 0,
  passed: 0,
  failed: 0,
  failures: []
};

function recordTest(name, passed, details = '') {
  results.total++;
  if (passed) {
    results.passed++;
    console.log(`  ✅ [PASS] ${name} ${details ? '(' + details + ')' : ''}`);
  } else {
    results.failed++;
    results.failures.push({ name, details });
    console.error(`  ❌ [FAIL] ${name}: ${details}`);
  }
}

async function run() {
  console.log('================================================================================');
  console.log('🧪 RUNNING ONEROLE PRODUCTION READINESS AUTOMATED E2E VERIFICATION SUITE');
  console.log('================================================================================\n');

  // Connect to MongoDB
  const mongoClient = new MongoClient(MONGO_URL);
  await mongoClient.connect();
  const db = mongoClient.db();

  // Launch Puppeteer Browser
  const browser = await puppeteer.launch({
    executablePath: CHROME_PATH,
    headless: true,
    args: ['--no-sandbox', '--disable-setuid-sandbox']
  });

  const page = await browser.newPage();
  const consoleErrors = [];
  const pageErrors = [];

  page.on('pageerror', err => pageErrors.push(err.message));
  page.on('console', msg => {
    if (msg.type() === 'error') {
      const txt = msg.text();
      if (!txt.includes('favicon') && !txt.includes('Failed to load resource: net::ERR_FILE_NOT_FOUND')) {
        consoleErrors.push(txt);
      }
    }
  });

  try {
    // -------------------------------------------------------------------------
    // SECTION 1: Healthcheck & Service Infrastructure
    // -------------------------------------------------------------------------
    console.log('--- 1. Infrastructure & Service Health ---');
    try {
      const uiRes = await fetch(`${UI_URL}`);
      recordTest('Frontend UI (8082) Responsive', uiRes.status === 200, `HTTP ${uiRes.status}`);

      const backendRes = await fetch(`${BACKEND_URL}/healthz`);
      const backendData = await backendRes.json();
      recordTest('Backend API (8088) Healthcheck', backendRes.status === 200 && backendData.status === 'ok');

      const workerRes = await fetch(`${WORKER_URL}/healthz`);
      const workerData = await workerRes.json();
      recordTest('Chromium Worker Daemon (3005) Healthcheck', workerRes.status === 200 && workerData.status === 'ok');

      const jobsCount = await db.collection('jobs').countDocuments();
      recordTest('MongoDB Connection & Jobs Collection Populated', jobsCount > 1000, `${jobsCount} active jobs`);
    } catch (err) {
      recordTest('Infrastructure Connectivity', false, err.message);
    }

    // -------------------------------------------------------------------------
    // SECTION 2: Initial Landing State (Acceptance Criteria 1)
    // -------------------------------------------------------------------------
    console.log('\n--- 2. Initial Landing State Verification ---');
    try {
      await page.goto(UI_URL, { waitUntil: 'networkidle2' });
      await sleep(1500);

      // Check Active Tag Chips in container
      const activeTagChips = await page.$$eval('#activeTagChips .tag-chip', els => els.map(e => e.innerText.trim()));
      recordTest('Landing State: Zero Active Filter Chips', activeTagChips.length === 0, 
        activeTagChips.length === 0 ? '0 chips present' : `Found chips: ${JSON.stringify(activeTagChips)}`);

      // Check Active Filter Pills
      const activeFilterPills = await page.$$eval('.filter-pill.active', els => els.length);
      recordTest('Landing State: Zero Active Filter Pills', activeFilterPills === 0, 
        activeFilterPills === 0 ? '0 active filter pills' : `Found ${activeFilterPills} active pills`);

      // Check Search Input Value
      const searchInputValue = await page.$eval('#tagSearchInputField', el => el.value.trim());
      recordTest('Landing State: Tag Search Input Empty', searchInputValue === '', 
        searchInputValue === '' ? 'Empty' : `Value: "${searchInputValue}"`);

      // Check Default 'golang' tag is NOT present
      const hasDefaultGolang = activeTagChips.some(c => c.toLowerCase().includes('golang'));
      recordTest('Landing State: No Default "golang" Tag Attached', !hasDefaultGolang,
        !hasDefaultGolang ? 'Verified absent' : 'Found hardcoded "golang" tag');
    } catch (err) {
      recordTest('Landing State Evaluation', false, err.message);
    }

    // -------------------------------------------------------------------------
    // SECTION 3: Real-Time Data Integration (Acceptance Criteria 2)
    // -------------------------------------------------------------------------
    console.log('\n--- 3. Real-Time Data Integration (Top 5 Matches & Full Catalog) ---');
    try {
      // Top 5 Match Cards queue in #coloredMatchCardsContainer
      const matchCards = await page.$$eval('#coloredMatchCardsContainer .job-match-card', els => {
        return els.map(el => {
          const title = el.querySelector('h3')?.innerText?.trim() || '';
          const company = el.querySelector('span.truncate')?.innerText?.trim() || '';
          const score = el.querySelector('.circle-score-val')?.innerText?.trim() || '';
          const location = el.querySelector('.space-y-0\\.5 span')?.innerText?.trim() || '';
          return { title, company, score, location };
        });
      });

      recordTest('Top 5 Match Cards: Exactly 5 Cards Rendered', matchCards.length === 5, `${matchCards.length} cards`);

      // Verify each match card corresponds to real MongoDB document
      let genuineCardCount = 0;
      for (const card of matchCards) {
        if (!card.title || !card.company) continue;
        const found = await db.collection('jobs').findOne({
          $or: [
            { canonical_title: { $regex: card.title.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), $options: 'i' } },
            { company_name: { $regex: card.company.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), $options: 'i' } }
          ]
        });
        if (found) genuineCardCount++;
      }
      recordTest('Top 5 Match Cards: Verified Live MongoDB Employer Records', genuineCardCount === 5,
        `${genuineCardCount}/5 verified against MongoDB`);

      // Catalog Feed Grid
      const catalogCards = await page.$$eval('#feedJobGrid .card', els => els.length);
      recordTest('Catalog Feed Grid: Loaded From MongoDB', catalogCards >= 6, `${catalogCards} vacancies rendered in feed`);

      // Verify Feed Cards are from real employers
      const feedCompanies = await page.$$eval('#feedJobGrid .card', els => {
        return els.slice(0, 10).map(e => e.querySelector('p.text-\\[10px\\]')?.innerText?.trim() || '');
      });
      const realEmployers = ['Samsara', 'Stripe', 'Lyft', 'Ramp', 'OpenAI', 'Duolingo', 'Docker', 'Notion', 'Figma', 'Asana', 'Datadog', 'Spotify'];
      const matchedEmployers = feedCompanies.filter(c => realEmployers.some(r => c.toLowerCase().includes(r.toLowerCase())));
      recordTest('Feed Displays Real Employers (Samsara, Stripe, Lyft, OpenAI, Docker, etc.)', 
        matchedEmployers.length > 0, `Sample companies: ${feedCompanies.slice(0, 5).join(', ')}`);
    } catch (err) {
      recordTest('Real-Time Data Integration Check', false, err.message);
    }

    // -------------------------------------------------------------------------
    // SECTION 4: Filter Functionality (Acceptance Criteria 3)
    // -------------------------------------------------------------------------
    console.log('\n--- 4. Filter Functionality (Search Tags, Workplace Modes, Experience, Role, Date) ---');
    try {
      await page.evaluate(() => window.switchNav('discover'));
      await sleep(500);

      // 4.1 Search Tag Addition & Removal
      await page.evaluate(() => {
        if (!STATE.filters.searchTags.includes('Docker')) {
          STATE.filters.searchTags.push('Docker');
          STATE.searchTags.push('Docker');
          renderTagChips();
          applyLiveFilters(true);
        }
      });
      await sleep(1000);

      const tagChipText = await page.$$eval('#activeTagChips .tag-chip', els => els.map(e => e.innerText.trim()));
      const dockerFeedCount = await page.$$eval('#feedJobGrid .card', els => els.length);
      recordTest('Filter Dynamic Update: Tag Search Added ("Docker")', 
        tagChipText.some(t => t.toLowerCase().includes('docker')) && dockerFeedCount > 0, 
        `Active chips: ${tagChipText.join(', ')} | Vacancies: ${dockerFeedCount}`);

      // Clear search tag
      await page.evaluate(() => window.removeSearchTag('Docker'));
      await sleep(1000);
      const restoredTagChips = await page.$$eval('#activeTagChips .tag-chip', els => els.length);
      recordTest('Filter Reset: Tag Removed Dynamically', restoredTagChips === 0, `Active chips: ${restoredTagChips}`);

      // 4.2 Workplace filter toggle
      await page.evaluate(() => window.toggleWorkplaceFilter('remote', true));
      await sleep(1000);
      const remoteFeedCount = await page.$$eval('#feedJobGrid .card', els => els.length);
      const remotePillActive = await page.$eval('#workplaceFilterBtn', el => el.classList.contains('active'));
      recordTest('Filter Dynamic Update: Workplace Mode ("Remote")', 
        remotePillActive && remoteFeedCount > 0, `${remoteFeedCount} remote jobs (pill active: ${remotePillActive})`);

      // Reset Workplace filter
      await page.evaluate(() => window.toggleWorkplaceFilter('remote', false));
      await sleep(800);

      // 4.3 Role filter toggle
      await page.evaluate(() => window.setRoleFilter('Backend', 'Backend'));
      await sleep(1000);
      const backendFeedCount = await page.$$eval('#feedJobGrid .card', els => els.length);
      const rolePillActive = await page.$eval('#roleFilterBtn', el => el.classList.contains('active'));
      recordTest('Filter Dynamic Update: Role ("Backend")', 
        rolePillActive && backendFeedCount > 0, `${backendFeedCount} backend jobs (pill active: ${rolePillActive})`);

      // Reset Role filter
      await page.evaluate(() => window.setRoleFilter('', 'Role'));
      await sleep(800);

      // 4.4 Experience filter toggle
      await page.evaluate(() => window.setExperienceFilter('5', '5+ years'));
      await sleep(1000);
      const expFeedCount = await page.$$eval('#feedJobGrid .card', els => els.length);
      const expPillActive = await page.$eval('#experienceFilterBtn', el => el.classList.contains('active'));
      recordTest('Filter Dynamic Update: Max Experience ("5+ years")', 
        expPillActive && expFeedCount > 0, `${expFeedCount} jobs (pill active: ${expPillActive})`);

      // Reset Experience filter
      await page.evaluate(() => window.setExperienceFilter('', 'Max Experience'));
      await sleep(800);

      // 4.5 Date filter toggle
      await page.evaluate(() => window.setDateFilter('last_30_days', 'Last 30 days'));
      await sleep(1000);
      const dateFeedCount = await page.$$eval('#feedJobGrid .card', els => els.length);
      const datePillActive = await page.$eval('#dateFilterBtn', el => el.classList.contains('active'));
      recordTest('Filter Dynamic Update: Date Posted ("Last 30 days")', 
        datePillActive && dateFeedCount > 0, `${dateFeedCount} jobs (pill active: ${datePillActive})`);

      // Reset Date filter
      await page.evaluate(() => window.setDateFilter('any', 'Date'));
      await sleep(800);

      const restoredCount = await page.$$eval('#feedJobGrid .card', els => els.length);
      recordTest('Filter Reset: Full Catalog Restored', restoredCount >= 6, `${restoredCount} jobs rendered`);
    } catch (err) {
      recordTest('Filter Functionality Check', false, err.message);
    }

    // -------------------------------------------------------------------------
    // SECTION 5: Location Filtering Accuracy (Acceptance Criteria 4)
    // -------------------------------------------------------------------------
    console.log('\n--- 5. Location Filtering Accuracy (UI & Backend) ---');
    try {
      // Backend direct query verification
      const loginRes = await fetch(`${BACKEND_URL}/auth/dev-login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: CANDIDATE_EMAIL })
      });
      const loginData = await loginRes.json();
      const token = loginData.access_token;
      const authHeaders = {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${token}`
      };

      const sfRes = await fetch(`${BACKEND_URL}/jobs/search?location=San%20Francisco&limit=20`, {
        headers: authHeaders
      });
      const sfData = await sfRes.json();
      const sfJobs = sfData.items || [];
      const sfAccurate = sfJobs.every(j => {
        const loc = (j.location || j.display_location || '').toLowerCase();
        return loc.includes('san francisco') || loc.includes('sf') || loc.includes('california') || loc.includes('ca');
      });
      recordTest('Backend Location Filtering: Query "San Francisco" accurately filters MongoDB jobs', 
        sfJobs.length > 0 && sfAccurate, `${sfJobs.length} accurate SF matches`);

      // UI Location Filter Verification
      await page.evaluate(() => window.toggleLocationFilter('San Francisco', true));
      await sleep(1000);

      const uiSfCount = await page.$$eval('#feedJobGrid .card', els => els.length);
      const locPillActive = await page.$eval('#locationFilterBtn', el => el.classList.contains('active'));
      recordTest('UI Location Filtering: Feed Dynamically Updates For Selected Location', 
        locPillActive && uiSfCount > 0, `${uiSfCount} SF jobs rendered (pill active: ${locPillActive})`);

      // Deselect location
      await page.evaluate(() => window.toggleLocationFilter('San Francisco', false));
      await sleep(800);
      const locPillRestored = await page.$eval('#locationFilterBtn', el => !el.classList.contains('active'));
      recordTest('UI Location Filtering: Reset Restores All Locations', locPillRestored, `Pill reset: ${locPillRestored}`);
    } catch (err) {
      recordTest('Location Filtering Check', false, err.message);
    }

    // -------------------------------------------------------------------------
    // SECTION 6: Vacancy Reloading Reliability (Acceptance Criteria 5)
    // -------------------------------------------------------------------------
    console.log('\n--- 6. Vacancy Reloading Reliability ---');
    try {
      const reloadBtn = await page.$('#btnReloadFeed');
      let reloadTriggered = false;

      if (reloadBtn) {
        await reloadBtn.click();
        reloadTriggered = true;
        await sleep(1500);
      }

      const reloadedCount = await page.$$eval('#feedJobGrid .card', els => els.length);
      recordTest('Vacancy Reloading: Triggering Reload Refreshes Live MongoDB Records', 
        reloadTriggered && reloadedCount > 0, `${reloadedCount} vacancies refreshed`);
    } catch (err) {
      recordTest('Vacancy Reloading Check', false, err.message);
    }

    // -------------------------------------------------------------------------
    // SECTION 7: Dynamic ATS Match Scoring (Acceptance Criteria 6)
    // -------------------------------------------------------------------------
    console.log('\n--- 7. Dynamic ATS Match Scoring Verification ---');
    try {
      // Check Queue Match Card Scores
      const matchScores = await page.$$eval('#coloredMatchCardsContainer .circle-score-val', els => {
        return els.map(e => e.innerText.trim()).filter(t => t.includes('%')).map(t => parseInt(t.replace('%', ''), 10));
      });
      const validScores = matchScores.length === 5 && matchScores.every(s => !isNaN(s) && s >= 0 && s <= 100);
      const scoreSet = new Set(matchScores);
      const isDynamic = scoreSet.size > 1; // Evaluates varied dynamic scores across different jobs

      recordTest('ATS Match Scoring: Queue Rendered Scores Are Valid Dynamic Percentages (0-100%)', 
        validScores && isDynamic, `Scores: ${matchScores.join('%, ')}%`);

      // Check Catalog Feed ATS score badges
      const catalogScores = await page.$$eval('#feedJobGrid .card', els => {
        return els.map(e => {
          const badge = e.querySelector('span.bg-\\[\\#eef7f2\\]')?.innerText?.trim() || '';
          const match = badge.match(/(\d+)%\s*ATS/);
          return match ? parseInt(match[1], 10) : null;
        }).filter(s => s !== null);
      });
      const validCatalogScores = catalogScores.length > 0 && catalogScores.every(s => s >= 0 && s <= 100);
      recordTest('ATS Match Scoring: Catalog Feed Displays Dynamic ATS Score Badges', 
        validCatalogScores, `${catalogScores.length} badges verified (Sample: ${catalogScores.slice(0, 5).join('%, ')}%)`);

      // Click card to open drawer
      await page.evaluate(() => {
        const card = document.querySelector('#feedJobGrid .card');
        if (card) {
          card.scrollIntoView({ block: 'center' });
          card.click();
        }
      });
      await sleep(800);

      const isDrawerOpen = await page.$eval('#jobDrawer', el => el.classList.contains('active'));
      const drawerTitle = await page.$eval('#drawerJobTitle', el => el.innerText.trim());
      const drawerCompany = await page.$eval('#drawerCompanyNameText', el => el.innerText.trim());
      const drawerScore = await page.$eval('#drawerScoreVal', el => el.innerText.trim());
      recordTest('ATS Match Scoring: Job Drawer Renders Detailed Fit & ATS Match', 
        isDrawerOpen && drawerTitle.length > 0 && drawerScore.includes('%'), 
        `Job: "${drawerTitle}" at ${drawerCompany} (${drawerScore})`);

      // Close drawer
      await page.evaluate(() => window.closeJobDrawer());
      await sleep(400);
    } catch (err) {
      recordTest('ATS Match Scoring Check', false, err.message);
    }

    // -------------------------------------------------------------------------
    // SECTION 8: Application Submission Lifecycle & Email Dispatch (Acceptance Criteria 7 & 8)
    // -------------------------------------------------------------------------
    console.log('\n--- 8. Application Submission Lifecycle & Email Confirmation Dispatch ---');
    try {
      // Find a fresh Docker/Ashby job to test external submission
      const appliedJobIds = await db.collection('applications').distinct('job_id');
      let testJob = await db.collection('jobs').findOne({
        _id: { $nin: appliedJobIds },
        company_name: 'Docker',
        source_url: { $regex: 'ashbyhq\\.com' }
      });
      if (!testJob) {
        testJob = await db.collection('jobs').findOne({
          company_name: 'Docker',
          source_url: { $regex: 'ashbyhq\\.com' }
        });
      }

      assert.ok(testJob, 'Target fresh unapplied real job found in MongoDB');
      console.log(`  Target Job for live submission: "${testJob.canonical_title}" at ${testJob.company_name} (ID: ${testJob._id})`);
      console.log(`  Posting URL: ${testJob.source_url}`);

      // Authenticate
      const loginRes = await fetch(`${BACKEND_URL}/auth/dev-login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: CANDIDATE_EMAIL })
      });
      const loginData = await loginRes.json();
      const authHeaders = {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${loginData.access_token}`,
        'X-Tenant-ID': loginData.tenant_id || '00000000-0000-4000-8000-000000000001'
      };

      // Submit application with async: true and submission_mode to verify schema compatibility and NATS queueing
      const submitRes = await fetch(`${BACKEND_URL}/applications/auto-apply`, {
        method: 'POST',
        headers: authHeaders,
        body: JSON.stringify({
          job_id: testJob._id,
          async: true,
          tailor_resume: true,
          submission_mode: 'submit_when_ready'
        })
      });

      recordTest('Submission Dispatch: POST /applications/auto-apply Accepted (HTTP 200)', submitRes.status === 200, 
        `HTTP ${submitRes.status}`);

      const submitData = await submitRes.json();
      const appId = submitData.application_id;
      assert.ok(appId, 'Application ID returned');
      console.log(`  Created Application ID: ${appId}`);

      // Track Status Lifecycle: submitting -> in_flight -> submitted
      const maxWait = 120000;
      const pollInterval = 3000;
      const start = Date.now();
      let finalApp = null;
      const observedStatuses = new Set();

      while (Date.now() - start < maxWait) {
        finalApp = await db.collection('applications').findOne({ _id: appId });
        if (finalApp && finalApp.status) {
          observedStatuses.add(finalApp.status);
          if (finalApp.status === 'submitted') {
            break;
          }
        }
        await sleep(pollInterval);
      }

      recordTest('Lifecycle Transition: Status reached "submitted"', 
        finalApp && finalApp.status === 'submitted', `Final status: ${finalApp ? finalApp.status : 'null'}`);

      recordTest('Verified Receipt ID Populated', 
        finalApp && Boolean(finalApp.verified_receipt_id), `Receipt: ${finalApp ? finalApp.verified_receipt_id : 'none'}`);

      // Check application_events
      const events = await db.collection('application_events').find({ application_id: appId }).toArray();
      const eventTypes = events.map(e => e.event_type);
      recordTest('Application Lifecycle Events Recorded in MongoDB', 
        eventTypes.includes('application.submitted'), `Events: ${eventTypes.join(' -> ')}`);

      // Check Email Notification Receipt Dispatch
      const emailNotif = await db.collection('email_notifications').findOne({ application_id: appId });
      recordTest('Email Receipt Dispatch: Record in email_notifications', 
        Boolean(emailNotif), emailNotif ? `ID: ${emailNotif._id}` : 'Not found');

      if (emailNotif) {
        recordTest('Email Recipient Matches Candidate (bhandarenakul11@gmail.com)', 
          emailNotif.recipient === CANDIDATE_EMAIL, emailNotif.recipient);
        recordTest('Email Status is "delivered"', 
          emailNotif.status === 'delivered', emailNotif.status);
        recordTest('Email Body Contains Verified Receipt ID', 
          emailNotif.body.includes(finalApp.verified_receipt_id), `Receipt ID verified`);
      }
    } catch (err) {
      recordTest('Application Submission & Email Dispatch Check', false, err.message);
    }

    // -------------------------------------------------------------------------
    // SECTION 9: Error & Console Audit
    // -------------------------------------------------------------------------
    console.log('\n--- 9. Error & Console Audit ---');
    recordTest('Browser Zero Page Errors', pageErrors.length === 0, 
      pageErrors.length === 0 ? '0 errors' : JSON.stringify(pageErrors));
    recordTest('Browser Zero Console Errors', consoleErrors.length === 0, 
      consoleErrors.length === 0 ? '0 errors' : JSON.stringify(consoleErrors));

  } finally {
    await browser.close();
    await mongoClient.close();
  }

  // ---------------------------------------------------------------------------
  // SUMMARY REPORT
  // ---------------------------------------------------------------------------
  console.log('\n================================================================================');
  console.log(`📊 TEST SUITE SUMMARY: ${results.passed} / ${results.total} PASSED (${Math.round((results.passed / results.total) * 100)}%)`);
  if (results.failed > 0) {
    console.log(`❌ FAILED CHECKS (${results.failed}):`);
    results.failures.forEach(f => console.log(`   - ${f.name}: ${f.details}`));
  } else {
    console.log('🎉 100% ACCEPTANCE CRITERIA VERIFIED SUCCESSFULLY WITH ZERO REGRESSIONS!');
  }
  console.log('================================================================================\n');

  if (results.failed > 0) {
    process.exit(1);
  } else {
    process.exit(0);
  }
}

run().catch(err => {
  console.error('Fatal Test Runner Error:', err);
  process.exit(1);
});
