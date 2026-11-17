/**
 * Adversarial Empirical Challenge Test:
 * Substring Collisions & Honeypot Evasion in Address Field Selectors
 * Target: connectors/ats_submit/browser_submitter.js (Lines 1387-1440)
 */

const assert = require('assert');
const puppeteer = require('puppeteer-core');
const { submitApplication, findCityInput, findStateInput } = require('../browser_submitter');

const CHROME_PATH = process.env.CHROME_PATH ||
  process.env.PUPPETEER_EXECUTABLE_PATH ||
  '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';

async function runTests() {
  console.log('=== Empirical Challenge: Address Field Substring Collisions & Honeypot Traps ===');
  let failures = 0;

  // Test 1: City selector substring collision with opacity honeypot
  console.log('\n--- Test 1: City selector collision with hp_opacity_0 ---');
  try {
    const html = `<!DOCTYPE html><html><body>
      <form id="ats_form">
        <input type="text" id="first_name" name="first_name" value="Nakul">
        <input type="text" id="last_name" name="last_name" value="Bhandare">
        <input type="email" id="email" name="email" value="bhandarenakul11@gmail.com">
        <!-- Honeypot with ID containing "city" via substring "opacity" -->
        <input type="text" id="hp_opacity_0" name="middle_name" style="opacity: 0;">
        <input type="text" id="real_city" name="city">
        <button type="submit" id="submit_app">Submit</button>
      </form>
    </body></html>`;

    const browser = await puppeteer.launch({ executablePath: CHROME_PATH, headless: 'new', pipe: true, args: ['--no-sandbox'] });
    const page = await browser.newPage();
    await page.goto('data:text/html;charset=utf-8,' + encodeURIComponent(html));

    // Evaluate what cityInput selector finds
    const cityInput = typeof findCityInput === 'function'
      ? await findCityInput(page)
      : await page.$('input[name*="city" i], input[id*="city" i], input[data-automation-id*="city" i]');
    const matchedId = await page.evaluate(el => el ? el.id : null, cityInput);
    await browser.close();

    console.log(`Matched element for city selector: id="${matchedId}"`);
    assert.notStrictEqual(matchedId, 'hp_opacity_0', 'CRITICAL BUG: input[id*="city" i] matched hp_opacity_0 due to substring collision with "opacity"');
    console.log('✅ Test 1 PASSED');
  } catch (err) {
    console.error('❌ Test 1 FAILED:', err.message);
    failures++;
  }

  // Test 2: City selector collision with ethnicity, simplicity, velocity
  console.log('\n--- Test 2: City selector collision with ethnicity, simplicity, velocity ---');
  try {
    const html = `<!DOCTYPE html><html><body>
      <form id="ats_form">
        <input type="text" id="first_name" name="first_name">
        <input type="email" id="email" name="email">
        <!-- False positives preceding city: simplicity, velocity, ethnicity -->
        <input type="text" id="simplicity_portal" name="simplicity_portal">
        <input type="text" id="velocity_ats" name="velocity_ats">
        <input type="text" id="ethnicity" name="ethnicity">
        <input type="text" id="city" name="city">
        <button type="submit" id="submit_app">Submit</button>
      </form>
    </body></html>`;

    const browser = await puppeteer.launch({ executablePath: CHROME_PATH, headless: 'new', pipe: true, args: ['--no-sandbox'] });
    const page = await browser.newPage();
    await page.goto('data:text/html;charset=utf-8,' + encodeURIComponent(html));

    const cityInput = typeof findCityInput === 'function'
      ? await findCityInput(page)
      : await page.$('input[name*="city" i], input[id*="city" i], input[data-automation-id*="city" i]');
    const matchedName = await page.evaluate(el => el ? el.name : null, cityInput);
    await browser.close();

    console.log(`Matched element for city selector: name="${matchedName}"`);
    assert.strictEqual(matchedName, 'city', `CRITICAL BUG: city selector matched "${matchedName}" instead of "city"`);
    console.log('✅ Test 2 PASSED');
  } catch (err) {
    console.error('❌ Test 2 FAILED:', err.message);
    failures++;
  }

  // Test 3: State selector collision with statement, real_estate, interstate
  console.log('\n--- Test 3: State selector collision with statement, real_estate, interstate ---');
  try {
    const html = `<!DOCTYPE html><html><body>
      <form id="ats_form">
        <input type="text" id="first_name" name="first_name">
        <input type="email" id="email" name="email">
        <!-- False positives preceding state: statement, estate, interstate -->
        <input type="text" id="real_estate_license" name="real_estate_license">
        <input type="text" id="interstate_travel" name="interstate_travel">
        <input type="text" id="statement_acknowledgement" name="statement_acknowledgement">
        <input type="text" id="state" name="state">
        <button type="submit" id="submit_app">Submit</button>
      </form>
    </body></html>`;

    const browser = await puppeteer.launch({ executablePath: CHROME_PATH, headless: 'new', pipe: true, args: ['--no-sandbox'] });
    const page = await browser.newPage();
    await page.goto('data:text/html;charset=utf-8,' + encodeURIComponent(html));

    const stateInput = typeof findStateInput === 'function'
      ? await findStateInput(page)
      : await page.$('input[name*="state" i], input[id*="state" i], select[name*="state" i], input[data-automation-id*="region" i], input[data-automation-id*="state" i]');
    const matchedName = await page.evaluate(el => el ? el.name : null, stateInput);
    await browser.close();

    console.log(`Matched element for state selector: name="${matchedName}"`);
    assert.strictEqual(matchedName, 'state', `CRITICAL BUG: state selector matched "${matchedName}" instead of "state"`);
    console.log('✅ Test 3 PASSED');
  } catch (err) {
    console.error('❌ Test 3 FAILED:', err.message);
    failures++;
  }

  // Test 4: End-to-end Honeypot test execution
  console.log('\n--- Test 4: End-to-end test_honeypots.js execution ---');
  try {
    const { execFileSync } = require('child_process');
    const path = require('path');
    execFileSync('node', [path.resolve(__dirname, 'test_honeypots.js')], { encoding: 'utf8' });
    console.log('✅ Test 4 PASSED');
  } catch (err) {
    console.error('❌ Test 4 FAILED: connectors/ats_submit/test/test_honeypots.js exited with non-zero code');
    failures++;
  }

  console.log('\n================================================================');
  console.log(`Address Substring Collision Tests: ${4 - failures}/4 passed, ${failures} failed`);
  console.log('================================================================');
  process.exit(failures === 0 ? 0 : 1);
}

runTests();
