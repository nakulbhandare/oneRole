const path = require('path');
const puppeteer = require(path.resolve(__dirname, '../connectors/ats_submit/node_modules/puppeteer-core'));

const chromePath = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';

(async () => {
  const browser = await puppeteer.launch({
    executablePath: chromePath,
    headless: 'new',
    pipe: true,
    args: ['--no-sandbox', '--disable-setuid-sandbox']
  });
  const page = await browser.newPage();
  await page.setViewport({ width: 1440, height: 900 });

  console.log('Navigating to http://localhost:8082...');
  await page.goto('http://localhost:8082', { waitUntil: 'networkidle2', timeout: 15000 });

  // 1. Check match cards
  await page.waitForSelector('.job-match-card', { timeout: 10000 });
  const initialCards = await page.evaluate(() => {
    return Array.from(document.querySelectorAll('.job-match-card')).map(c => {
      const title = c.querySelector('h3') ? c.querySelector('h3').innerText.trim() : '';
      const company = c.querySelector('.truncate') ? c.querySelector('.truncate').innerText.trim() : '';
      return { title, company };
    });
  });
  console.log('Initial Top Match Cards:', initialCards.slice(0, 5));

  // 2. Click Apply on the 1st card
  const firstCardComp = initialCards[0].company;
  const firstCardTitle = initialCards[0].title;
  console.log('Clicking Apply on Card 1:', firstCardComp, '-', firstCardTitle);

  const applyBtn = await page.$('.job-match-card .btn-card-apply');
  if (applyBtn) {
    await applyBtn.click();
  }
  await new Promise(r => setTimeout(r, 2000));

  // 3. Check Match Cards queue: ensure the applied job is GONE and queue maintains 5 cards
  await new Promise(r => setTimeout(r, 4000));
  const updatedCards = await page.evaluate(() => {
    return Array.from(document.querySelectorAll('.job-match-card')).map(c => {
      const title = c.querySelector('h3') ? c.querySelector('h3').innerText.trim() : '';
      const company = c.querySelector('.truncate') ? c.querySelector('.truncate').innerText.trim() : '';
      return { title, company };
    });
  });
  console.log('Updated Cards Queue:', updatedCards);
  const appliedJobStillThere = updatedCards.some(c => c.title === firstCardTitle && c.company === firstCardComp);
  console.log('Is applied job still in queue cards?', appliedJobStillThere ? 'FAIL (still there)' : 'PASS (permanently removed)');
  console.log('Queue maintains exactly 5 cards?', updatedCards.length === 5 ? 'PASS (5 cards)' : 'FAIL (' + updatedCards.length + ')');

  // 4. Switch to Applications View
  console.log('Switching to Applications view...');
  await page.evaluate(() => {
    if (typeof switchNav === 'function') switchNav('applications');
  });
  await new Promise(r => setTimeout(r, 2500));

  // 5. Inspect top row of Applications Tracker table
  const topRows = await page.evaluate(() => {
    const rows = Array.from(document.querySelectorAll('#appTrackerTableBody tr'));
    return rows.slice(0, 5).map(r => r.innerText.replace(/\s+/g, ' ').trim());
  });
  console.log('Top rows in Tracker table:');
  topRows.forEach((r, idx) => console.log('  Row', idx + 1, ':', r));

  const isAtTop = topRows[0] && topRows[0].includes(firstCardComp);
  console.log('VERDICT: Is newly applied role at Row 1 of Tracker table?', isAtTop ? 'PASS (at Top Row 1)' : 'FAIL');

  const artifactDir = '/Users/nakulbhandare/.gemini/antigravity/brain/4a56cc37-703a-413f-aa12-a46dfbfb2370';
  await page.screenshot({ path: `${artifactDir}/verified_tsenta_agentic_tracker.png` });
  console.log('Screenshot saved to verified_tsenta_agentic_tracker.png');

  await browser.close();
  if (isAtTop && !appliedJobStillThere && updatedCards.length === 5) {
    console.log('🎉 ALL VERIFICATIONS PASSED SUCCESSFULLY!');
    process.exit(0);
  } else {
    console.log('⚠️ Verification condition failed.');
    process.exit(1);
  }
})();
