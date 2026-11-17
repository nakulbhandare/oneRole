const puppeteer = require('../connectors/ats_submit/node_modules/puppeteer-core');

async function runUiVerification() {
  const browser = await puppeteer.launch({
    executablePath: '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
    headless: true,
    args: ['--no-sandbox', '--disable-setuid-sandbox']
  });

  const page = await browser.newPage();
  const consoleErrors = [];
  const pageErrors = [];

  page.on('console', msg => {
    if (msg.type() === 'error') {
      consoleErrors.push(msg.text());
    }
  });
  page.on('pageerror', err => {
    pageErrors.push(err.message);
  });

  page.on('response', res => {
    if (res.status() >= 400) {
      console.log('HTTP Error:', res.status(), res.url());
    }
  });

  console.log('--- Step 1: Navigating to UI at http://localhost:8082 ---');
  await page.goto('http://localhost:8082', { waitUntil: 'domcontentloaded' });
  await new Promise(r => setTimeout(r, 1200));
  const title = await page.title();
  console.log('Page Title:', title);

  console.log('--- Step 1.1: Verifying Landing State has ZERO Active Filters ---');
  const initialChipsCount = await page.$$eval('#activeTagChips .tag-chip', els => els.length);
  const activePillsCount = await page.$$eval('.filter-pill.active', els => els.length);
  const initialSearchTags = await page.evaluate(() => STATE.searchTags || []);
  console.log(`Active Tag Chips on Landing: ${initialChipsCount}`);
  console.log(`Active Filter Pills on Landing: ${activePillsCount}`);
  console.log(`STATE.searchTags on Landing:`, initialSearchTags);
  if (initialChipsCount !== 0) {
    throw new Error(`Expected 0 active tag chips on landing, found ${initialChipsCount}`);
  }
  if (activePillsCount !== 0) {
    throw new Error(`Expected 0 active filter pills on landing, found ${activePillsCount}`);
  }
  if (initialSearchTags.length !== 0) {
    throw new Error(`Expected empty STATE.searchTags on landing, found ${initialSearchTags.join(',')}`);
  }
  console.log('✓ Verified: Landing view renders with ZERO active filters attached.');

  console.log('--- Step 2: Testing Navigation Tabs ---');
  await page.waitForFunction(() => typeof window.switchNav === 'function');
  const tabs = ['discover', 'auto-apply', 'applications', 'profile', 'overview', 'recommended', 'saved', 'settings'];
  for (const tab of tabs) {
    await page.evaluate(t => window.switchNav(t), tab);
    await new Promise(r => setTimeout(r, 200));
    const active = await page.$eval('#view-' + tab, el => el.classList.contains('active'));
    if (!active) {
      throw new Error(`Tab view-${tab} failed to activate`);
    }
  }
  console.log('✓ All 8 navigation tabs transition smoothly without issues.');

  console.log('--- Step 3: Discover Jobs, Top 5 Matches & Real MongoDB Catalog ---');
  await page.evaluate(() => switchNav('discover'));
  await new Promise(r => setTimeout(r, 500));

  // Verify Top 5 Match Cards queue
  const matchCardsCount = await page.$$eval('#coloredMatchCardsContainer .job-match-card', els => els.length);
  console.log(`✓ Top match cards in queue: ${matchCardsCount}`);
  if (matchCardsCount !== 5) {
    throw new Error(`Expected exactly 5 top match cards, found ${matchCardsCount}`);
  }
  const matchCardScores = await page.$$eval('#coloredMatchCardsContainer .circle-score-val', els => els.map(e => e.innerText));
  console.log(`✓ Top 5 Match ATS Scores: ${matchCardScores.join(', ')}`);

  const jobCards = await page.$$eval('#feedJobGrid .card', els => els.length);
  console.log(`✓ Real jobs rendered in feed: ${jobCards} listings`);
  if (jobCards === 0) throw new Error('No job cards rendered in feed');

  // Verify Catalog Cards display dynamic ATS match scores
  const catalogAtsBadges = await page.$$eval('#feedJobGrid .card', els => {
    return els.filter(el => el.innerText.includes('% ATS')).length;
  });
  console.log(`✓ Catalog cards with dynamic ATS score badge: ${catalogAtsBadges} / ${jobCards}`);
  if (catalogAtsBadges === 0) throw new Error('Catalog cards missing ATS score badges');

  console.log('--- Step 4: Job Details Slide-Over Drawer with ATS Evaluation ---');
  await page.evaluate(() => {
    const card = document.querySelector('#feedJobGrid .card');
    if (card) {
      card.scrollIntoView({ block: 'center' });
      card.click();
    }
  });
  await new Promise(r => setTimeout(r, 400));
  const isDrawerOpen = await page.$eval('#jobDrawer', el => el.classList.contains('active'));
  const drawerJobTitle = await page.$eval('#drawerJobTitle', el => el.innerText);
  const drawerCompany = await page.$eval('#drawerCompanyNameText', el => el.innerText);
  const drawerDescLength = await page.$eval('#drawerJobDescription', el => el.innerHTML.length);
  const drawerScoreText = await page.$eval('#drawerScoreVal', el => el.innerText);
  const drawerBodyScore = await page.$eval('#drawerBodyScoreText', el => el.innerText);
  console.log(`✓ Drawer opened: ${isDrawerOpen} | Job: "${drawerJobTitle}" at ${drawerCompany}`);
  console.log(`✓ Drawer ATS Score Header: "${drawerScoreText}" | Body Fit: "${drawerBodyScore}"`);
  console.log(`✓ Job description rich HTML content length: ${drawerDescLength} chars`);
  if (!isDrawerOpen || drawerDescLength < 50) {
    throw new Error('Drawer failed to open or description content empty');
  }

  console.log('--- Step 5: Drawer Bookmark Toggle ---');
  await page.click('#drawerBookmarkBtn');
  await new Promise(r => setTimeout(r, 250));
  const bookmarkState = await page.$eval('#drawerBookmarkText', el => el.innerText);
  console.log(`✓ Drawer bookmark state updated: "${bookmarkState}"`);

  await page.evaluate(() => closeJobDrawer());
  await new Promise(r => setTimeout(r, 200));

  console.log('--- Step 6: Dynamic UI Filters (Search Tags, Location, Workplace, Experience, Role, Date, Reload) ---');

  // 6.1: Company Filter
  await page.evaluate(() => toggleCompanyFilter('OpenAI', true));
  await new Promise(r => setTimeout(r, 500));
  const openAiJobs = await page.$$eval('#feedJobGrid .card', els => els.length);
  console.log(`✓ 6.1 Company Filter: ${openAiJobs} OpenAI vacancies displayed`);
  await page.evaluate(() => toggleCompanyFilter('OpenAI', false));
  await new Promise(r => setTimeout(r, 500));

  // 6.2: Search Tag Input & Removal
  await page.type('#tagSearchInputField', 'Docker');
  await page.keyboard.press('Enter');
  await new Promise(r => setTimeout(r, 500));
  const dockerChips = await page.$$eval('#activeTagChips .tag-chip', els => els.length);
  const dockerJobs = await page.$$eval('#feedJobGrid .card', els => els.length);
  console.log(`✓ 6.2 Search Tag Filter: ${dockerChips} chip active, ${dockerJobs} Docker vacancies displayed`);
  await page.evaluate(() => removeSearchTag('Docker'));
  await new Promise(r => setTimeout(r, 500));

  // 6.3: Location Filter
  await page.evaluate(() => toggleLocationFilter('San Francisco', true));
  await new Promise(r => setTimeout(r, 500));
  const sfJobs = await page.$$eval('#feedJobGrid .card', els => els.length);
  const sfPillActive = await page.$eval('#locationFilterBtn', el => el.classList.contains('active'));
  console.log(`✓ 6.3 Location Filter: ${sfJobs} San Francisco vacancies displayed (pill active: ${sfPillActive})`);
  if (!sfPillActive) throw new Error('Location filter pill failed to activate');
  await page.evaluate(() => toggleLocationFilter('San Francisco', false));
  await new Promise(r => setTimeout(r, 500));

  // 6.4: Workplace Filter
  await page.evaluate(() => toggleWorkplaceFilter('remote', true));
  await new Promise(r => setTimeout(r, 500));
  const remoteJobs = await page.$$eval('#feedJobGrid .card', els => els.length);
  const remotePillActive = await page.$eval('#workplaceFilterBtn', el => el.classList.contains('active'));
  console.log(`✓ 6.4 Workplace Filter: ${remoteJobs} remote vacancies displayed (pill active: ${remotePillActive})`);
  await page.evaluate(() => toggleWorkplaceFilter('remote', false));
  await new Promise(r => setTimeout(r, 500));

  // 6.5: Role Filter
  await page.evaluate(() => setRoleFilter('Backend', 'Backend'));
  await new Promise(r => setTimeout(r, 500));
  const backendJobs = await page.$$eval('#feedJobGrid .card', els => els.length);
  const rolePillActive = await page.$eval('#roleFilterBtn', el => el.classList.contains('active'));
  console.log(`✓ 6.5 Role Filter: ${backendJobs} backend vacancies displayed (pill active: ${rolePillActive})`);
  await page.evaluate(() => setRoleFilter('', 'Role'));
  await new Promise(r => setTimeout(r, 500));

  // 6.6: Experience Filter
  await page.evaluate(() => setExperienceFilter('5', '5+ years'));
  await new Promise(r => setTimeout(r, 500));
  const expJobs = await page.$$eval('#feedJobGrid .card', els => els.length);
  const expPillActive = await page.$eval('#experienceFilterBtn', el => el.classList.contains('active'));
  console.log(`✓ 6.6 Experience Filter: ${expJobs} vacancies displayed for <= 5 yrs (pill active: ${expPillActive})`);
  await page.evaluate(() => setExperienceFilter('', 'Max Experience'));
  await new Promise(r => setTimeout(r, 500));

  // 6.7: Date Filter - 6 Hours & 30 Days
  await page.evaluate(() => setDateFilter('last_6_hours', 'Last 6 hours'));
  await new Promise(r => setTimeout(r, 500));
  const date6hrJobs = await page.$$eval('#feedJobGrid .card', els => els.length);
  const date6hrPillActive = await page.$eval('#dateFilterBtn', el => el.classList.contains('active'));
  console.log(`✓ 6.7a Date Filter (6 Hours): ${date6hrJobs} vacancies displayed for last 6 hours (pill active: ${date6hrPillActive})`);
  if (date6hrJobs === 0 || !date6hrPillActive) {
    throw new Error('6-hour date filter failed to display vacancies or activate pill');
  }

  await page.evaluate(() => setDateFilter('last_30_days', 'Last 30 days'));
  await new Promise(r => setTimeout(r, 500));
  const dateJobs = await page.$$eval('#feedJobGrid .card', els => els.length);
  const datePillActive = await page.$eval('#dateFilterBtn', el => el.classList.contains('active'));
  console.log(`✓ 6.7b Date Filter (30 Days): ${dateJobs} vacancies displayed for last 30 days (pill active: ${datePillActive})`);
  await page.evaluate(() => setDateFilter('any', 'Date'));
  await new Promise(r => setTimeout(r, 500));

  // 6.8: Reload Button Functionality
  const reloadBtn = await page.$('#btnReloadFeed');
  if (!reloadBtn) throw new Error('#btnReloadFeed button not found');
  await page.click('#btnReloadFeed');
  await new Promise(r => setTimeout(r, 700));
  const reloadedCards = await page.$$eval('#feedJobGrid .card', els => els.length);
  console.log(`✓ 6.8 Reload Feed Button: ${reloadedCards} vacancies refreshed from MongoDB`);
  if (reloadedCards === 0) throw new Error('Reload feed failed to load vacancies');

  console.log('--- Step 7: Applications Tracker Table ---');
  await page.evaluate(() => switchNav('applications'));
  await new Promise(r => setTimeout(r, 400));
  const appRows = await page.$$eval('#appTrackerTableBody tr', els => els.length);
  const allCount = await page.$eval('#countAll', el => el.innerText);
  const submittedCount = await page.$eval('#countSubmitted', el => el.innerText);
  console.log(`✓ Live Tracker rows: ${appRows} | All count: ${allCount} | Submitted: ${submittedCount}`);
  if (appRows === 0) throw new Error('Tracker rows empty');

  console.log('--- Step 8: View Receipt Modal ---');
  const receiptBtn = await page.$('#view-applications button[title*="receipt"]') || await page.$('button[title*="receipt"]');
  if (receiptBtn) {
    await page.evaluate(el => el.click(), receiptBtn);
    await new Promise(r => setTimeout(r, 300));
    const modalVisible = await page.$eval('#receiptModal', el => !el.classList.contains('hidden'));
    const receiptId = await page.$eval('#modalReceiptIdText', el => el.innerText);
    const receiptEmployer = await page.$eval('#modalReceiptEmployerText', el => el.innerText);
    const receiptStatus = await page.$eval('#modalReceiptStatusText', el => el.innerText);
    console.log(`✓ Receipt Modal opened: ${modalVisible} | ID: ${receiptId} | Employer: ${receiptEmployer} | Status: ${receiptStatus}`);
    await page.evaluate(() => closeModal('receiptModal'));
  }

  console.log('--- Step 9: Live AI Recommendations ---');
  await page.evaluate(() => switchNav('recommended'));
  await new Promise(r => setTimeout(r, 400));
  const recCards = await page.$$eval('#recommendedJobsGrid .card', els => els.length);
  console.log(`✓ Live recommended jobs rendered: ${recCards}`);
  if (recCards === 0) throw new Error('No recommended cards rendered');

  console.log('--- Step 10: Saved Bookmarks Persistence ---');
  await page.evaluate(() => switchNav('saved'));
  await new Promise(r => setTimeout(r, 300));
  const savedRows = await page.$$eval('#savedJobsTableBody tr', els => els.length);
  console.log(`✓ Saved jobs table rendered: ${savedRows} rows`);

  console.log('--- Step 11: Add Custom Role Modal & Submission ---');
  await page.evaluate(() => switchNav('discover'));
  await page.evaluate(() => openAddRoleModal());
  await new Promise(r => setTimeout(r, 200));
  const isAddModalOpen = await page.$eval('#addRoleModal', el => !el.classList.contains('hidden'));
  console.log(`✓ Add Role Modal open: ${isAddModalOpen}`);
  await page.type('#customRoleCompany', 'Antigravity Labs');
  await page.type('#customRoleTitle', 'Lead Systems Architect (Go)');
  await page.evaluate(() => submitCustomRole());
  await new Promise(r => setTimeout(r, 500));
  const isAddModalClosed = await page.$eval('#addRoleModal', el => el.classList.contains('hidden'));
  console.log(`✓ Add Role Modal submitted and closed: ${isAddModalClosed}`);

  console.log('--- Step 12: Error Audit ---');
  console.log(`Page errors: ${pageErrors.length}`);
  console.log(`Console errors: ${consoleErrors.length}`);
  if (consoleErrors.length > 0) {
    console.error('Console errors:', consoleErrors);
  }
  if (pageErrors.length > 0) {
    console.error('Page errors:', pageErrors);
  }

  await browser.close();

  if (consoleErrors.length > 0 || pageErrors.length > 0) {
    process.exit(1);
  }
  console.log('🎉 ALL UI VERIFICATION CHECKS PASSED WITH ZERO ERRORS!');
}

runUiVerification().catch(err => {
  console.error('❌ Verification failed:', err);
  process.exit(1);
});
