/**
 * OneRole High-Fidelity In-Process Mock ATS HTTP Server
 * File: tests/e2e/helpers/mock_ats.js
 * 
 * Implements deterministic DOM & API mocks for:
 * 1. Greenhouse (React-Select comboboxes, aria-labelledby, #resume file input, confirmation)
 * 2. Ashby (Input fields, location autocomplete popup, Yes/No pills, mock GraphQL endpoint)
 * 3. Lever (Inputs, resume upload, separate pronoun checkboxes, pronoun hygiene validation)
 * 4. Workday (Multi-step wizard: Account creation -> OTP input -> Personal/Source -> Submit)
 */

const http = require('http');
const { parse } = require('url');

const DEFAULT_PORT = 9876;
let serverInstance = null;
const recordedSubmissions = {
  greenhouse: [],
  ashby: [],
  lever: [],
  workday: []
};

const scenarioState = {
  ashbyGraphqlMode: 'success', // 'success' | 'graphql_error' | 'inverted_trap'
  workdayStep: 1,
  leverPronounStatus: 'clean'
};

function buildGreenhouseHtml(company, jobId) {
  return `<!DOCTYPE html>
<html>
<head><title>Careers at ${company}</title></head>
<body>
  <h1>Apply for Job #${jobId} at ${company}</h1>
  <form id="application_form" action="/greenhouse/${company}/jobs/${jobId}/submit" method="POST" enctype="multipart/form-data">
    <div class="field">
      <label id="first_name-label" for="first_name">First Name *</label>
      <input type="text" id="first_name" name="first_name" required />
    </div>
    <div class="field">
      <label id="last_name-label" for="last_name">Last Name *</label>
      <input type="text" id="last_name" name="last_name" required />
    </div>
    <div class="field">
      <label id="email-label" for="email">Email *</label>
      <input type="email" id="email" name="email" required />
    </div>
    <div class="field">
      <label id="phone-label" for="phone">Phone *</label>
      <input type="tel" id="phone" name="phone" required />
    </div>
    <div class="field">
      <label id="job_application_location-label" for="job_application_location">Location *</label>
      <input type="text" id="job_application_location" name="job_application_location" required />
    </div>
    <div class="field" data-qa="resume-upload-section">
      <label for="resume">Attach Resume *</label>
      <input type="file" id="resume" name="resume" required />
    </div>

    <!-- Optional extra file inputs for testing scoped upload -->
    <div class="field" data-qa="cover-letter-section">
      <label for="cover_letter_upload">Cover Letter (Optional)</label>
      <input type="file" id="cover_letter_upload" name="cover_letter" />
    </div>
    <div class="field" data-qa="transcript-section">
      <label for="transcript_upload">Transcripts (Optional)</label>
      <input type="file" id="transcript_upload" name="transcript" />
    </div>

    <!-- Hidden telephone country listbox (to verify it gets ignored) -->
    <div class="iti__country-list" style="display:none">
      <div class="iti__country" role="option">United States (+1)</div>
      <div class="iti__country" role="option">Canada (+1)</div>
    </div>

    <!-- Native HTML select fallback test -->
    <div class="field">
      <label for="work_auth">Native Work Authorization Select</label>
      <select id="work_auth" name="work_auth">
        <option value="">Select...</option>
        <option value="Yes">Yes</option>
        <option value="No">No</option>
        <option value="Require Sponsorship">Require Sponsorship</option>
      </select>
    </div>

    <!-- React-Select Custom Question 1: Degree Level -->
    <div class="field application-question" data-field-id="question_degree">
      <label id="lbl_question_degree">Degree Level *</label>
      <div class="selectWrapper" aria-labelledby="lbl_question_degree">
        <div class="select__control" onclick="document.getElementById('menu_degree').style.display='block'">
          <div class="select__input-container">
            <input class="select__input" id="question_101" role="combobox" aria-labelledby="lbl_question_degree" />
          </div>
        </div>
        <div id="menu_degree" class="select__menu" style="display:none">
          <div class="select__option" role="option" onclick="selectOpt('degree', 'Bachelor\\'s Degree')">Bachelor's Degree</div>
          <div class="select__option" role="option" onclick="selectOpt('degree', 'Master\\'s Degree')">Master's Degree</div>
        </div>
      </div>
    </div>

    <!-- React-Select Custom Question 2: Work Authorization -->
    <div class="field application-question" data-field-id="question_auth">
      <label id="lbl_question_auth">Authorized to work in the US? *</label>
      <div class="selectWrapper" aria-labelledby="lbl_question_auth">
        <div class="select__control" onclick="document.getElementById('menu_auth').style.display='block'">
          <div class="select__input-container">
            <input class="select__input" id="question_102" role="combobox" aria-labelledby="lbl_question_auth" />
          </div>
        </div>
        <div id="menu_auth" class="select__menu" style="display:none">
          <div class="select__option" role="option" onclick="selectOpt('auth', 'Yes')">Yes</div>
          <div class="select__option" role="option" onclick="selectOpt('auth', 'No')">No</div>
        </div>
      </div>
    </div>

    <button type="submit" id="submit_app">Submit Application</button>
  </form>
  <script>
    function selectOpt(field, val) {
      const inp = document.querySelector('[data-field-id="question_' + field + '"] .select__input');
      if (inp) inp.value = val;
      const m = document.getElementById('menu_' + field);
      if (m) m.style.display = 'none';
    }
  </script>
</body>
</html>`;
}

function buildAshbyHtml(company, jobId) {
  return `<!DOCTYPE html>
<html>
<head><title>Application - ${company}</title></head>
<body>
  <div id="ashby_app">
    <h1>Apply to ${company}</h1>
    <!-- Autofill uploader section -->
    <div class="ashby-application-form-autofill-uploader">
      <label>Autofill from resume</label>
      <input type="file" id="ashby_autofill_file" />
    </div>

    <div data-field-path="_candidate_name" class="fieldEntry">
      <label>Full Name *</label>
      <input type="text" name="name" required />
    </div>
    <div data-field-path="_candidate_email" class="fieldEntry">
      <label>Email *</label>
      <input type="email" name="email" required />
    </div>
    <div data-field-path="_candidate_phone" class="fieldEntry">
      <label>Phone *</label>
      <input type="tel" name="phone" required />
    </div>
    <div data-field-path="_candidate_location" class="fieldEntry">
      <label>Location *</label>
      <input class="ashby-application-form-input-autocomplete" placeholder="Type city..." oninput="showLocPopup()" />
      <div id="ashby_loc_popup" class="ashby-application-form-input-autocomplete-popup-result" style="display:none" onclick="pickLoc('San Francisco, CA, USA')">
        San Francisco, CA, USA
      </div>
    </div>
    <div data-field-path="sponsorship" class="fieldEntry">
      <label>Require Visa Sponsorship? *</label>
      <button type="button" class="ashby-application-form-input-yesno-option" aria-pressed="false" onclick="toggleYesNo(this)">Yes</button>
      <button type="button" class="ashby-application-form-input-yesno-option" aria-pressed="false" onclick="toggleYesNo(this)">No</button>
    </div>
    <div data-field-path="_candidate_resume" class="fieldEntry">
      <label>Resume *</label>
      <input type="file" class="ashby-application-form-input-file" />
    </div>
    <button type="button" class="ashby-application-form-submit-button" onclick="submitAshby()">Submit Application</button>
  </div>
  <script>
    function showLocPopup() { document.getElementById('ashby_loc_popup').style.display = 'block'; }
    function pickLoc(loc) {
      document.querySelector('.ashby-application-form-input-autocomplete').value = loc;
      document.getElementById('ashby_loc_popup').style.display = 'none';
    }
    function toggleYesNo(btn) {
      btn.parentElement.querySelectorAll('button').forEach(b => b.setAttribute('aria-pressed', 'false'));
      btn.setAttribute('aria-pressed', 'true');
    }
    async function submitAshby() {
      try {
        const res = await fetch('/ashby/api/candidate-portal/graphql', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ action: 'ApiSubmitMultipleFormsAction', company: '${company}', jobId: '${jobId}' })
        });
        const data = await res.json();
        if (data && data.data && data.data.submitApplication && data.data.submitApplication.success) {
          document.body.innerHTML = '<h1>Thank you for applying! Your application has been received.</h1>';
        } else {
          let errContainer = document.querySelector('.ashby-error');
          if (!errContainer) {
            errContainer = document.createElement('div');
            errContainer.className = 'ashby-error';
            document.body.appendChild(errContainer);
          }
          errContainer.innerText = (data && data.errors && data.errors[0] && data.errors[0].message) ? data.errors[0].message : 'GraphQL Submission Error';
        }
      } catch (e) {
        let errContainer = document.querySelector('.ashby-error');
        if (!errContainer) {
          errContainer = document.createElement('div');
          errContainer.className = 'ashby-error';
          document.body.appendChild(errContainer);
        }
        errContainer.innerText = e.message;
      }
    }
  </script>
</body>
</html>`;
}

function buildLeverHtml(company, jobId) {
  return `<!DOCTYPE html>
<html>
<head><title>Apply - ${company}</title></head>
<body>
  <h1>Apply at ${company}</h1>
  <form id="lever_form" action="/lever/${company}/${jobId}/submit" method="POST" enctype="multipart/form-data">
    <div class="field"><input type="text" name="name" placeholder="Full name" required /></div>
    <div class="field"><input type="email" name="email" placeholder="Email" required /></div>
    <div class="field"><input type="tel" name="phone" placeholder="Phone" required /></div>
    <div class="field"><input type="text" name="org" placeholder="Current company" /></div>
    <div class="field"><input type="file" name="resume" required /></div>

    <div class="application-question pronouns">
      <label>Pronouns (select one)</label>
      <label><input type="checkbox" name="pronouns[he]" value="he/him" /> He/Him</label>
      <label><input type="checkbox" name="pronouns[she]" value="she/her" /> She/Her</label>
      <label><input type="checkbox" name="pronouns[they]" value="they/them" /> They/Them</label>
    </div>

    <div class="application-question consent">
      <label><input type="checkbox" name="consent" required /> I agree to data terms</label>
    </div>

    <button type="submit" data-qa="btn-submit">Submit Application</button>
  </form>
</body>
</html>`;
}

function buildWorkdayStepHtml(company, step, hasError = false) {
  if (step === 1) {
    return `<!DOCTYPE html>
<html>
<head><title>Workday - Account</title></head>
<body>
  <h2>Create Account</h2>
  <form action="/workday/${company}/apply/step2" method="POST">
    <input type="email" name="email" placeholder="Email Address" required />
    <input type="password" name="password" placeholder="Password" required />
    <button type="submit" data-automation-id="createAccountButton">Create Account</button>
  </form>
</body>
</html>`;
  } else if (step === 2) {
    return `<!DOCTYPE html>
<html>
<head><title>Workday - OTP</title></head>
<body>
  <h2>Verify Your Identity</h2>
  <form action="/workday/${company}/apply/step3" method="POST">
    <input type="text" name="verification_code" placeholder="Enter code" required />
    <button type="submit" data-automation-id="verifyButton">Verify</button>
  </form>
</body>
</html>`;
  } else if (step === 3) {
    return `<!DOCTYPE html>
<html>
<head><title>Workday - Info & Source</title></head>
<body>
  <h2>Candidate Information</h2>
  ${hasError ? '<div data-automation-id="errorMessage" class="error">Please select a source</div>' : ''}
  <form id="wd_step3_form" action="/workday/${company}/apply/step4" method="POST" onsubmit="return validateStep3(event)">
    <input type="text" data-automation-id="legalNameSection_firstName" name="firstName" required />
    <input type="text" data-automation-id="legalNameSection_lastName" name="lastName" required />
    <button type="button" data-automation-id="prompt-source" onclick="document.getElementById('src_menu').style.display='block'">Select Source</button>
    <div id="src_menu" style="display:none">
      <div onclick="setSource('LinkedIn')">LinkedIn</div>
      <div onclick="setSource('Indeed')">Indeed</div>
    </div>
    <input type="hidden" id="src_val" name="source" />
    <button type="submit" data-automation-id="page-navigation-next-button">Next</button>
  </form>
  <script>
    function setSource(val) {
      document.getElementById('src_val').value = val;
      document.getElementById('src_menu').style.display = 'none';
      const err = document.querySelector('[data-automation-id="errorMessage"]');
      if (err) err.remove();
    }
    function validateStep3(e) {
      const src = document.getElementById('src_val').value;
      if (!src) {
        e.preventDefault();
        if (!document.querySelector('[data-automation-id="errorMessage"]')) {
          const div = document.createElement('div');
          div.setAttribute('data-automation-id', 'errorMessage');
          div.className = 'error';
          div.innerText = 'Please select a source';
          document.body.insertBefore(div, document.getElementById('wd_step3_form'));
        }
        return false;
      }
      return true;
    }
  </script>
</body>
</html>`;
  } else if (step === 4) {
    return `<!DOCTYPE html>
<html>
<head><title>Workday - Resume & Submit</title></head>
<body>
  <h2>Resume Upload</h2>
  <form action="/workday/${company}/apply/complete" method="POST">
    <div data-automation-id="file-upload-dropzone" class="dropzone">
      <input type="file" data-automation-id="file-upload-input-drop-zone" name="file" required />
    </div>
    <button type="submit" data-automation-id="page-navigation-submit-button">Submit Application</button>
  </form>
</body>
</html>`;
  } else {
    return `<!DOCTYPE html><html><body><h1>Your application was submitted successfully. Thank you!</h1></body></html>`;
  }
}

async function start(port = DEFAULT_PORT) {
  if (serverInstance) return;

  serverInstance = http.createServer((req, res) => {
    const parsed = parse(req.url, true);
    const pathname = parsed.pathname;

    let bodyChunks = [];
    req.on('data', chunk => bodyChunks.push(chunk));
    req.on('end', () => {
      const rawBody = Buffer.concat(bodyChunks).toString();

      // Greenhouse Routes
      if (pathname.startsWith('/greenhouse/') && pathname.includes('/jobs/')) {
        const parts = pathname.split('/');
        const company = parts[2];
        const jobId = parts[4];
        if (pathname.endsWith('/submit')) {
          recordedSubmissions.greenhouse.push({ company, jobId, body: rawBody, time: Date.now() });
          res.writeHead(302, { Location: `/greenhouse/${company}/confirmation` });
          res.end();
          return;
        }
        res.writeHead(200, { 'Content-Type': 'text/html' });
        res.end(buildGreenhouseHtml(company, jobId));
        return;
      }
      if (pathname.includes('/greenhouse/') && pathname.endsWith('/confirmation')) {
        res.writeHead(200, { 'Content-Type': 'text/html' });
        res.end('<h1>Thank you for applying! Your application has been submitted.</h1>');
        return;
      }

      // Ashby Routes
      if (pathname.startsWith('/ashby/') && pathname.endsWith('/application')) {
        const parts = pathname.split('/');
        res.writeHead(200, { 'Content-Type': 'text/html' });
        res.end(buildAshbyHtml(parts[2], parts[3]));
        return;
      }
      if (pathname === '/ashby/api/candidate-portal/graphql') {
        recordedSubmissions.ashby.push({ body: rawBody, time: Date.now() });
        res.writeHead(200, { 'Content-Type': 'application/json' });
        if (scenarioState.ashbyGraphqlMode === 'success') {
          res.end(JSON.stringify({ data: { submitApplication: { success: true, receipt: 'ASHBY-MOCK-' + Date.now() } } }));
        } else if (scenarioState.ashbyGraphqlMode === 'inverted_trap') {
          // Trap: Has errors array, but lacks the literal string "errorMessages"
          res.end(JSON.stringify({ data: null, errors: [{ message: 'Missing entry for required field: Location' }] }));
        } else {
          res.end(JSON.stringify({ errors: [{ message: 'Submission failed' }] }));
        }
        return;
      }

      // Lever Routes
      if (pathname.startsWith('/lever/') && pathname.endsWith('/apply')) {
        const parts = pathname.split('/');
        res.writeHead(200, { 'Content-Type': 'text/html' });
        res.end(buildLeverHtml(parts[2], parts[3]));
        return;
      }
      if (pathname.startsWith('/lever/') && pathname.endsWith('/submit')) {
        const parts = pathname.split('/');
        const isConflict = (rawBody.includes('pronouns%5Bhe%5D') && rawBody.includes('pronouns%5Bshe%5D')) ||
                           (rawBody.includes('pronouns[he]') && rawBody.includes('pronouns[she]')) ||
                           (rawBody.includes('pronouns%5Bhe%5D') && rawBody.includes('pronouns%5Bthey%5D')) ||
                           (rawBody.includes('pronouns%5Bshe%5D') && rawBody.includes('pronouns%5Bthey%5D'));
        recordedSubmissions.lever.push({
          company: parts[2],
          jobId: parts[3],
          rawBody,
          hasPronounConflict: isConflict,
          time: Date.now()
        });
        res.writeHead(302, { Location: `/lever/${parts[2]}/confirmation` });
        res.end();
        return;
      }
      if (pathname.includes('/lever/') && pathname.endsWith('/confirmation')) {
        res.writeHead(200, { 'Content-Type': 'text/html' });
        res.end('<div class="application-confirmation"><h1>Application submitted. Thanks for applying!</h1></div>');
        return;
      }

      // Workday Routes
      if (pathname.startsWith('/workday/')) {
        const parts = pathname.split('/');
        const company = parts[2];
        if (pathname.includes('/step2')) {
          res.writeHead(200, { 'Content-Type': 'text/html' });
          res.end(buildWorkdayStepHtml(company, 2));
        } else if (pathname.includes('/step3')) {
          res.writeHead(200, { 'Content-Type': 'text/html' });
          res.end(buildWorkdayStepHtml(company, 3));
        } else if (pathname.includes('/step4')) {
          res.writeHead(200, { 'Content-Type': 'text/html' });
          res.end(buildWorkdayStepHtml(company, 4));
        } else if (pathname.includes('/complete')) {
          recordedSubmissions.workday.push({ company, time: Date.now() });
          res.writeHead(200, { 'Content-Type': 'text/html' });
          res.end(buildWorkdayStepHtml(company, 5));
        } else {
          res.writeHead(200, { 'Content-Type': 'text/html' });
          res.end(buildWorkdayStepHtml(company, 1));
        }
        return;
      }

      // Default 404
      res.writeHead(404, { 'Content-Type': 'text/plain' });
      res.end('Not Found');
    });
  });

  return new Promise((resolve) => {
    serverInstance.listen(port, () => resolve());
  });
}

function stop() {
  if (!serverInstance) return Promise.resolve();
  return new Promise(resolve => {
    serverInstance.close(() => {
      serverInstance = null;
      resolve();
    });
  });
}

function setScenario(key, val) {
  scenarioState[key] = val;
}

function getSubmissions(ats) {
  return recordedSubmissions[ats] || [];
}

function clearSubmissions() {
  for (const k of Object.keys(recordedSubmissions)) {
    recordedSubmissions[k] = [];
  }
}

module.exports = {
  start,
  stop,
  setScenario,
  getSubmissions,
  clearSubmissions,
  buildGreenhouseHtml,
  buildAshbyHtml,
  buildLeverHtml,
  buildWorkdayStepHtml
};
