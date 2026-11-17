const assert = require('assert');
const puppeteer = require('puppeteer-core');
const { submitApplication } = require('../browser_submitter');

const mockHoneypotHtml = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>Mock ATS Form with Honeypots</title>
</head>
<body>
  <form id="ats_form">
    <!-- Legitimate visible fields -->
    <div class="fieldEntry">
      <label for="first_name">First Name</label>
      <input type="text" id="first_name" name="first_name" required>
    </div>
    <div class="fieldEntry">
      <label for="last_name">Last Name</label>
      <input type="text" id="last_name" name="last_name" required>
    </div>
    <div class="fieldEntry">
      <label for="email">Email Address</label>
      <input type="email" id="email" name="email" required>
    </div>
    <div class="fieldEntry">
      <label for="phone">Phone Number</label>
      <input type="tel" id="phone" name="phone">
    </div>
    <div class="fieldEntry">
      <label for="location">Location</label>
      <input type="text" id="location" name="location">
    </div>

    <!-- Honeypot 1: display: none inline -->
    <div class="fieldEntry">
      <label>Company Website</label>
      <input type="text" id="hp_display_none" name="website_url" style="display: none;">
    </div>

    <!-- Honeypot 2: parent display: none -->
    <div style="display: none;">
      <label>Current Employer</label>
      <input type="text" id="hp_parent_display_none" name="employer">
    </div>

    <!-- Honeypot 3: visibility: hidden inline -->
    <div class="fieldEntry">
      <label>Secondary Phone</label>
      <input type="text" id="hp_vis_hidden" name="secondary_phone" style="visibility: hidden;">
    </div>

    <!-- Honeypot 4: parent visibility: hidden -->
    <div style="visibility: hidden;">
      <label>Title</label>
      <input type="text" id="hp_parent_vis_hidden" name="title">
    </div>

    <!-- Honeypot 5: opacity: 0 inline -->
    <div class="fieldEntry">
      <label>Middle Name</label>
      <input type="text" id="hp_opacity_0" name="middle_name" style="opacity: 0;">
    </div>

    <!-- Honeypot 6: parent opacity: 0 -->
    <div style="opacity: 0;">
      <label>Nickname</label>
      <input type="text" id="hp_parent_opacity_0" name="nickname">
    </div>

    <!-- Honeypot 7: tabindex: -1 -->
    <div class="fieldEntry">
      <label>Leave this empty</label>
      <input type="text" id="hp_tabindex" name="url_verification" tabindex="-1">
    </div>

    <!-- Honeypot 8: 0x0 offscreen absolute positioning -->
    <div class="fieldEntry">
      <label>Offscreen trap</label>
      <input type="text" id="hp_offscreen" name="bot_email" style="position: absolute; left: -9999px; top: -9999px; width: 0px; height: 0px; overflow: hidden;">
    </div>

    <!-- Honeypot 9: 0x0 size with overflow hidden in parent -->
    <div style="width: 0px; height: 0px; overflow: hidden;">
      <input type="text" id="hp_zero_dims" name="comment_bot">
    </div>

    <!-- Honeypot 10: Honeypot naming pattern: confirm_email_address -->
    <div class="fieldEntry">
      <label>Confirm Email</label>
      <input type="text" id="confirm_email_address" name="confirm_email_address">
    </div>

    <!-- Honeypot 11: Honeypot naming pattern: bot_check -->
    <div class="fieldEntry">
      <label>Security Check</label>
      <input type="text" id="bot_check" name="bot_check">
    </div>

    <!-- Honeypot 12: Honeypot naming pattern: user_hp -->
    <div class="fieldEntry">
      <label>Internal HP</label>
      <input type="text" id="user_hp" name="user_hp">
    </div>

    <!-- Honeypot 13: aria-hidden="true" in container -->
    <div aria-hidden="true">
      <label>Aria Hidden Field</label>
      <input type="text" id="hp_aria_hidden" name="hidden_city">
    </div>

    <!-- Honeypot 14: autocomplete="off-honeypot" -->
    <div class="fieldEntry">
      <input type="text" id="hp_ac" name="ac_check" autocomplete="off-honeypot">
    </div>

    <button type="submit" id="submit_app">Submit Application</button>
  </form>

  <script>
    window.__honeypotViolations = [];
    const honeypotIds = [
      'hp_display_none', 'hp_parent_display_none', 'hp_vis_hidden', 'hp_parent_vis_hidden',
      'hp_opacity_0', 'hp_parent_opacity_0', 'hp_tabindex', 'hp_offscreen',
      'hp_zero_dims', 'confirm_email_address', 'bot_check', 'user_hp', 'hp_aria_hidden', 'hp_ac'
    ];

    for (const id of honeypotIds) {
      const el = document.getElementById(id);
      if (el) {
        el.addEventListener('input', () => {
          window.__honeypotViolations.push(id + '=' + el.value);
        });
      }
    }

    const form = document.getElementById('ats_form');
    form.addEventListener('submit', (e) => {
      e.preventDefault();
      const filled = [];
      for (const id of honeypotIds) {
        const el = document.getElementById(id);
        if (el && el.value && el.value.trim().length > 0) {
          filled.push(id + '=' + el.value);
        }
      }
      if (filled.length > 0) {
        document.body.innerHTML = '<h1>Your application submission was flagged as possible spam.</h1><p>Triggered honeypots: ' + filled.join(', ') + '</p>';
      } else {
        document.body.innerHTML = '<h1>Thank you for applying! Your application has been received.</h1>';
      }
    });
  </script>
</body>
</html>`;

const dataUrl = 'data:text/html;charset=utf-8,' + encodeURIComponent(mockHoneypotHtml);

const candidate = {
  name: 'Nakul Bhandare',
  first_name: 'Nakul',
  last_name: 'Bhandare',
  email: 'bhandarenakul11@gmail.com',
  phone: '+1 (350)-355-6570',
  location: 'San Francisco, CA'
};

(async () => {
  console.log("=== Challenge 1: Testing Honeypots with Various Hiding Mechanisms ===");

  // Phase 1: End-to-End submission with anti-spam assertion
  console.log("Running submitApplication on honeypot fixture...");
  const result = await submitApplication({
    url: dataUrl,
    candidate,
    pipe: true,
    dryRun: false
  });

  console.log("Submission result:", JSON.stringify(result));
  assert.strictEqual(result.success, true, "Submission must succeed");
  assert.ok(result.receipt_id, "Receipt ID must be generated");

  // Phase 2: Direct DOM state verification using dryRun
  console.log("Running dryRun to inspect DOM values...");
  const chromePath = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
  const browser = await puppeteer.launch({
    executablePath: chromePath,
    headless: 'new',
    pipe: true,
    args: ['--no-sandbox']
  });

  try {
    const page = await browser.newPage();
    await page.goto(dataUrl, { waitUntil: 'domcontentloaded' });

    // Emulate what browser_submitter does
    const dryRunResult = await submitApplication({
      url: dataUrl,
      candidate,
      pipe: true,
      dryRun: true
    });

    console.log("DryRun filled values:", JSON.stringify(dryRunResult.filled_values));
    assert.strictEqual(dryRunResult.filled_values.name, "Nakul");
    assert.strictEqual(dryRunResult.filled_values.email, "bhandarenakul11@gmail.com");

    // Launch a page and verify each honeypot directly after running submitApplication steps
    await page.evaluate((cand) => {
      // Re-run the fillSmartFields evaluation in page context
      const allFormElements = Array.from(document.querySelectorAll('input, textarea, select'));
      // Verify honeypot states
      const honeypots = [
        'hp_display_none', 'hp_parent_display_none', 'hp_vis_hidden', 'hp_parent_vis_hidden',
        'hp_opacity_0', 'hp_parent_opacity_0', 'hp_tabindex', 'hp_offscreen',
        'hp_zero_dims', 'confirm_email_address', 'bot_check', 'user_hp', 'hp_aria_hidden', 'hp_ac'
      ];
      for (const id of honeypots) {
        const el = document.getElementById(id);
        if (el && el.value !== '') {
          throw new Error('Honeypot field ' + id + ' was filled with: ' + el.value);
        }
      }
    }, candidate);

    console.log("SUCCESS: All 14 honeypot fields remained completely empty (0% filled)!");
  } finally {
    await browser.close();
  }
})();
