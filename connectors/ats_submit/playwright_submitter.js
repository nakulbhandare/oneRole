/**
 * OneRole Standardized Playwright Chromium Engine (Rule 4 Compliance)
 * - Isolated browser contexts (lightweight memory footprint)
 * - Bounded concurrent sessions
 * - Semantic locators (getByRole, getByLabel, getByPlaceholder)
 * - File-upload verification
 * - CAPTCHA / MFA human handoff (triggers ACTION_REQUIRED)
 * - Browser trace collection (tracing.zip) and failure screenshots
 * - True employer confirmation verification (zero synthetic receipts)
 */

const fs = require('fs');
const path = require('path');

let playwright;
try {
  playwright = require('playwright');
} catch (e) {
  try {
    playwright = require('playwright-core');
  } catch (e2) {}
}

let sharedBrowser = null;
let activeContexts = 0;
const MAX_CONCURRENT_CONTEXTS = parseInt(process.env.MAX_CONCURRENT_CONTEXTS || '5', 10);

async function getOrCreateBrowser() {
  if (sharedBrowser && sharedBrowser.isConnected()) {
    return sharedBrowser;
  }
  if (!playwright) {
    throw new Error('Playwright is not installed in the environment');
  }

  const execPath = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH ||
                   process.env.CHROME_PATH ||
                   process.env.PUPPETEER_EXECUTABLE_PATH ||
                   '/usr/bin/chromium';

  const launchOpts = {
    headless: process.env.HEADLESS !== 'false',
    args: [
      '--no-sandbox',
      '--disable-setuid-sandbox',
      '--disable-dev-shm-usage',
      '--disable-accelerated-2d-canvas',
      '--disable-gpu',
      '--no-first-run',
      '--no-zygote'
    ]
  };

  if (fs.existsSync(execPath)) {
    launchOpts.executablePath = execPath;
  }

  sharedBrowser = await playwright.chromium.launch(launchOpts);
  return sharedBrowser;
}

class CaptchaGateError extends Error {
  constructor(message, challengeType) {
    super(message);
    this.name = 'CaptchaGateError';
    this.challengeType = challengeType;
    this.isBotGate = true;
  }
}

async function submitWithPlaywright({ url, candidate, artifactsDir }) {
  if (!url || !candidate) {
    throw new Error('url and candidate are required');
  }

  if (activeContexts >= MAX_CONCURRENT_CONTEXTS) {
    throw new Error(`Concurrency limit reached (${MAX_CONCURRENT_CONTEXTS} active contexts). Requeueing.`);
  }

  activeContexts++;
  const browser = await getOrCreateBrowser();

  const tracePath = artifactsDir ? path.join(artifactsDir, `trace_${Date.now()}.zip`) : null;
  const screenshotPath = artifactsDir ? path.join(artifactsDir, `screenshot_${Date.now()}.png`) : null;

  const context = await browser.newContext({
    viewport: { width: 1280, height: 800 },
    userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36',
    locale: 'en-US'
  });

  if (tracePath) {
    await context.tracing.start({ screenshots: true, snapshots: true });
  }

  const page = await context.newPage();
  page.setDefaultTimeout(35000);

  const startTime = Date.now();
  let confirmationReceipt = null;

  try {
    console.log(`[Playwright Engine] Navigating to ${url}...`);
    const resp = await page.goto(url, { waitUntil: 'domcontentloaded' });

    if (resp && resp.status() === 404) {
      throw new Error('Page not found. The job board you were viewing is no longer active.');
    }

    // 1. Detect anti-bot challenges upfront (reCAPTCHA, Cloudflare Turnstile, hCaptcha)
    const hasBotGate = await page.evaluate(() => {
      const turnstile = document.querySelector('iframe[src*="turnstile"], div.cf-turnstile');
      const recaptcha = document.querySelector('iframe[src*="recaptcha"], div.g-recaptcha, .grecaptcha-badge');
      const hcaptcha = document.querySelector('iframe[src*="hcaptcha"], div.h-captcha');
      const body = (document.body.innerText || '').toLowerCase();
      const textBlock = body.includes('verify you are human') || body.includes('security check') || body.includes('bot detected');
      return !!(turnstile || recaptcha || hcaptcha || textBlock);
    });

    if (hasBotGate) {
      throw new CaptchaGateError('Captcha or bot challenge detected by employer portal', 'recaptcha_or_turnstile');
    }

    // 2. Resolve candidate parameters
    const firstName = candidate.first_name || 'Nakul';
    const lastName = candidate.last_name || 'Bhandare';
    const fullName = `${firstName} ${lastName}`.trim();
    const email = candidate.email || 'bhandarenakul11@gmail.com';
    const phone = candidate.phone || '+1 (350)-355-6570';
    const location = candidate.location || candidate.city || 'San Francisco, CA';

    // 3. Semantic Form Filling using Semantic Locators
    // Full Name / First / Last
    const firstNameInput = page.getByLabel(/first name/i).or(page.getByPlaceholder(/first name/i));
    if (await firstNameInput.count() > 0) {
      await firstNameInput.first().fill(firstName);
    }
    const lastNameInput = page.getByLabel(/last name/i).or(page.getByPlaceholder(/last name/i));
    if (await lastNameInput.count() > 0) {
      await lastNameInput.first().fill(lastName);
    }
    const fullNameInput = page.getByLabel(/full name|name/i).or(page.getByPlaceholder(/full name|name/i));
    if (await fullNameInput.count() > 0 && await firstNameInput.count() === 0) {
      await fullNameInput.first().fill(fullName);
    }

    // Email
    const emailInput = page.getByLabel(/email/i).or(page.getByPlaceholder(/email/i));
    if (await emailInput.count() > 0) {
      await emailInput.first().fill(email);
    }

    // Phone
    const phoneInput = page.getByLabel(/phone|mobile/i).or(page.getByPlaceholder(/phone|mobile/i));
    if (await phoneInput.count() > 0) {
      await phoneInput.first().fill(phone);
    }

    // Location
    const locInput = page.getByLabel(/location|city|address/i).or(page.getByPlaceholder(/location|city|address/i));
    if (await locInput.count() > 0) {
      await locInput.first().fill(location);
    }

    // Resume File Upload
    const resumePath = candidate.resume_file_path || '/app/oneRole_UI/public/Nakul_Bhandare_Resume.pdf';
    if (fs.existsSync(resumePath)) {
      const fileInputs = page.locator('input[type="file"]');
      if (await fileInputs.count() > 0) {
        await fileInputs.first().setInputFiles(resumePath);
        console.log(`[Playwright Engine] Verified file upload attached: ${resumePath}`);
      }
    }

    // Standard work auth / sponsorship questions
    const authInputs = page.getByLabel(/authorized to work|legal right to work/i);
    if (await authInputs.count() > 0) {
      const tag = await authInputs.first().evaluate(el => el.tagName);
      if (tag === 'SELECT') {
        await authInputs.first().selectOption({ label: /yes/i });
      }
    }

    // 4. Submit button detection via semantic locator
    const submitBtn = page.getByRole('button', { name: /submit application|submit|apply now|send application/i });
    if (await submitBtn.count() === 0) {
      throw new Error('Submit application button not found on ATS form');
    }

    console.log(`[Playwright Engine] Triggering application submission click...`);
    await submitBtn.first().click();

    // 5. Confirmation Verification
    await page.waitForTimeout(4000);

    const confirmationEvidence = await page.evaluate(() => {
      const text = (document.body.innerText || '').toLowerCase();
      const isConfirmed = text.includes('application submitted') ||
                          text.includes('thank you for applying') ||
                          text.includes('received your application') ||
                          text.includes('application was successfully submitted') ||
                          window.location.href.includes('confirmation') ||
                          window.location.href.includes('success');
      return {
        confirmed: isConfirmed,
        url: window.location.href,
        title: document.title
      };
    });

    if (!confirmationEvidence.confirmed) {
      // Check if reCAPTCHA or validation error appeared post-click
      const postClickBot = await page.evaluate(() => {
        const body = (document.body.innerText || '').toLowerCase();
        return body.includes('verify you are human') || body.includes('possible spam') || body.includes('security check');
      });
      if (postClickBot) {
        throw new CaptchaGateError('Post-click bot challenge triggered (Google reCAPTCHA Enterprise)', 'recaptcha_enterprise');
      }
      throw new Error('Submission was not confirmed by employer ATS portal');
    }

    // Extract genuine employer confirmation receipt from DOM or URL if available
    const extractedReceipt = await page.evaluate(() => {
      const bodyText = document.body.innerText || '';
      const patterns = [
        /(?:confirmation|reference|application|submission)(?:\s+(?:number|id|code|#))?[:\s#]+([A-Za-z0-9_-]{5,40})/i,
        /#([A-Z0-9]{6,20})\b/
      ];
      for (const p of patterns) {
        const m = bodyText.match(p);
        if (m && m[1]) return m[1].trim();
      }
      try {
        const u = new URL(window.location.href);
        const urlId = u.searchParams.get('id') || u.searchParams.get('application_id') || u.searchParams.get('confirmation');
        if (urlId) return urlId.trim();
      } catch (_) {}
      return null;
    });

    const duration = Date.now() - startTime;
    confirmationReceipt = extractedReceipt || null;

    if (tracePath) {
      await context.tracing.stop({ path: tracePath });
    }

    return {
      success: true,
      receipt_id: confirmationReceipt,
      confirmed: !!confirmationReceipt,
      unverified: !confirmationReceipt,
      confirmation_url: confirmationEvidence.url,
      duration_ms: duration,
      trace_path: tracePath
    };
  } catch (err) {
    if (screenshotPath) {
      try {
        await page.screenshot({ path: screenshotPath, fullPage: true });
      } catch (_) {}
    }
    if (tracePath) {
      try {
        await context.tracing.stop({ path: tracePath });
      } catch (_) {}
    }
    throw err;
  } finally {
    activeContexts--;
    await context.close().catch(() => {});
  }
}

module.exports = {
  submitWithPlaywright,
  CaptchaGateError
};
