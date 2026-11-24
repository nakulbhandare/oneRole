/**
 * OneRole High-Fidelity In-Process Mock ATS HTTP Server
 * File: tests/e2e/helpers/mock_ats.js
 * 
 * Implements deterministic DOM & API mocks for all 5 ATS platforms:
 * 1. Greenhouse (Public API v1 with ?questions=true, multipart apply, HTML forms & React-Select)
 * 2. Ashby (Next.js AST inside <script id="__NEXT_DATA__">, 2-step GraphQL file upload & apply mutations)
 * 3. Lever (Public API v0 with lists & customQuestions, multipart submit, pronoun checkbox hygiene)
 * 4. SmartRecruiters (Candidate REST API v1 with JSON & base64 attachments, DOM apply with window.__oneRoleApply)
 * 5. Workday CXS (REST candidate experience quick-apply API + multi-step session wizard)
 * 
 * Includes Chaos & Fault Injection, Latency Simulation, and Anti-Bot Telemetry Probes.
 */

const http = require('http');

const DEFAULT_PORT = 9876;
let serverInstance = null;
let artificialLatencyMs = 0;

const recordedSubmissions = {
  greenhouse: [],
  ashby: [],
  lever: [],
  smartrecruiters: [],
  workday: []
};

const scenarioState = {
  ashbyGraphqlMode: 'success', // 'success' | 'graphql_error' | 'inverted_trap'
  workdayStep: 1,
  leverPronounStatus: 'clean',
  status: null,
  statusCode: null,
  dropConnection: false,
  bot_challenge: false,
  retryAfter: null
};

function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

function safeDecodeURIComponent(val) {
  if (typeof val !== 'string') return val;
  try {
    return decodeURIComponent(val);
  } catch (_) {
    return val;
  }
}

function safeParseUrl(reqUrl, hostHeader) {
  try {
    return new URL(reqUrl, `http://${hostHeader || 'localhost'}`);
  } catch (_) {
    try {
      return new URL(reqUrl, 'http://localhost');
    } catch (_) {
      return null;
    }
  }
}

function detectPronounConflict(rawBody) {
  if (!rawBody || typeof rawBody !== 'string') return false;
  const decodedBody = safeDecodeURIComponent(rawBody);

  const lowerRaw = rawBody.toLowerCase();
  const lowerDecoded = decodedBody.toLowerCase();

  const hasHe = lowerDecoded.includes('pronouns[he]') ||
                lowerRaw.includes('pronouns%5bhe%5d') ||
                lowerRaw.includes('pronouns[he]');
  const hasShe = lowerDecoded.includes('pronouns[she]') ||
                 lowerRaw.includes('pronouns%5bshe%5d') ||
                 lowerRaw.includes('pronouns[she]');
  const hasThey = lowerDecoded.includes('pronouns[they]') ||
                  lowerRaw.includes('pronouns%5bthey%5d') ||
                  lowerRaw.includes('pronouns[they]');

  return ((hasHe ? 1 : 0) + (hasShe ? 1 : 0) + (hasThey ? 1 : 0)) > 1;
}

function parseEmailFromBody(rawBody, headers) {
  if (!rawBody || typeof rawBody !== 'string') return null;
  try {
    const j = JSON.parse(rawBody);
    if (j.email) return j.email;
    if (j.primaryEmail) return j.primaryEmail;
    if (j.candidate) {
      if (j.candidate.primaryEmail) return j.candidate.primaryEmail;
      if (j.candidate.email) return j.candidate.email;
    }
    if (j.variables && j.variables.input && Array.isArray(j.variables.input.fieldValues)) {
      const emailField = j.variables.input.fieldValues.find(f => f.fieldEntryId && f.fieldEntryId.includes('email'));
      if (emailField && emailField.value) return emailField.value;
    }
    if (j.candidate_email) return j.candidate_email;
    if (j.target_email) return j.target_email;
    if (j.recipient) return j.recipient;
  } catch (_) {}

  const emailMatch = rawBody.match(/name="email"[\r\n\s]+([^\r\n]+)/) ||
                     rawBody.match(/name="primaryEmail"[\r\n\s]+([^\r\n]+)/) ||
                     rawBody.match(/email=([^&]+)/) ||
                     rawBody.match(/([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})/);
  if (emailMatch && emailMatch[1]) {
    return safeDecodeURIComponent(emailMatch[1].trim());
  }
  return null;
}

function buildBotChallengeHtml(atsName = 'Cloudflare') {
  return `<!DOCTYPE html>
<html>
<head>
  <title>Security Verification - ${atsName}</title>
  <meta name="viewport" content="width=device-width, initial-scale=1">
</head>
<body>
  <div id="challenge-container">
    <h1>Security Check Required</h1>
    <p>Please complete verification below before proceeding to the job application.</p>
    <div class="cf-turnstile" data-state="interactive" data-sitekey="0x4AAAAAAABcdEF123456789"></div>
    <div class="g-recaptcha" data-sitekey="6Le-wvkSAAAAAPBMRTvw0Q4Muexq9bi0DJwx_mJ-"></div>
    <div id="challenge-running" class="challenge-status">Verification pending...</div>
  </div>
</body>
</html>`;
}

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
  const nextData = {
    props: {
      pageProps: {
        jobPosting: {
          id: String(jobId || '12345'),
          title: 'Software Engineer',
          department: 'Engineering',
          location: 'San Francisco, CA',
          applicationForm: {
            sections: [
              {
                title: 'Personal Information',
                fields: [
                  { fieldEntryId: '_systemfield_name', title: 'Full Name', isRequired: true, type: 'String' },
                  { fieldEntryId: '_systemfield_email', title: 'Email', isRequired: true, type: 'String' },
                  { fieldEntryId: '_systemfield_phone', title: 'Phone', isRequired: true, type: 'String' },
                  { fieldEntryId: '_systemfield_location', title: 'Location', isRequired: true, type: 'Location' },
                  { fieldEntryId: '_systemfield_resume', title: 'Resume', isRequired: true, type: 'File' },
                  { fieldEntryId: 'sponsorship', title: 'Require Visa Sponsorship?', isRequired: true, type: 'Boolean' }
                ]
              }
            ]
          }
        }
      }
    }
  };

  return `<!DOCTYPE html>
<html>
<head>
  <title>Application - ${company}</title>
  <script id="__NEXT_DATA__" type="application/json">
${JSON.stringify(nextData, null, 2)}
  </script>
</head>
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

function buildSmartRecruitersHtml(company, jobId) {
  return `<!DOCTYPE html>
<html>
<head><title>Careers at ${company} - SmartRecruiters</title></head>
<body>
  <h1>Apply for Job #${jobId} at ${company}</h1>
  <form id="st-apply" action="/smartrecruiters/${company}/${jobId}/submit" method="POST" enctype="multipart/form-data">
    <div class="field">
      <label for="first-name-input">First Name *</label>
      <input type="text" id="first-name-input" name="first_name" required />
    </div>
    <div class="field">
      <label for="last-name-input">Last Name *</label>
      <input type="text" id="last-name-input" name="last_name" required />
    </div>
    <div class="field">
      <label for="email-input">Email *</label>
      <input type="email" id="email-input" name="email" required />
    </div>
    <div class="field">
      <label for="phone-number-input">Phone Number *</label>
      <input type="tel" id="phone-number-input" name="phone" required />
    </div>
    <div class="field">
      <label for="resume-input">Resume *</label>
      <input type="file" id="resume-input" name="resume" required />
    </div>
    <button type="submit" id="st-submitBtn">Submit Application</button>
  </form>
  <div id="st-confirmation" style="display:none">
    <h1>Application submitted successfully!</h1>
    <p>Receipt: <span id="st-receipt-id">SR-RECEIPT-${Date.now()}</span></p>
  </div>
  <script>
    window.__oneRoleApply = function(candidate, fieldMap) {
      if (candidate) {
        if (candidate.first_name) document.getElementById('first-name-input').value = candidate.first_name;
        if (candidate.last_name) document.getElementById('last-name-input').value = candidate.last_name;
        if (candidate.email) document.getElementById('email-input').value = candidate.email;
        if (candidate.phone) document.getElementById('phone-number-input').value = candidate.phone;
      }
      document.getElementById('st-confirmation').style.display = 'block';
      return { success: true, receipt: 'SR-DOM-' + Date.now() };
    };
  </script>
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

  serverInstance = http.createServer(async (req, res) => {
    const reqStartTime = Date.now();
    const parsed = safeParseUrl(req.url, req.headers.host);
    if (!parsed) {
      res.writeHead(400, { 'Content-Type': 'text/plain' });
      res.end('Bad Request: Invalid URL');
      return;
    }
    const pathname = parsed.pathname;
    const query = Object.fromEntries(parsed.searchParams.entries());

    // 1. Connection Drop Chaos Simulation
    if (scenarioState.dropConnection) {
      req.socket.destroy();
      return;
    }

    // 2. Artificial Latency Simulation
    if (artificialLatencyMs > 0) {
      await sleep(artificialLatencyMs);
    }

    // 3. Status Override Chaos Simulation (400, 404, 429, 500, 503, 504, etc.)
    const overrideStatus = scenarioState.status || scenarioState.statusCode;
    if (overrideStatus) {
      const statusCode = Number(overrideStatus);
      const headers = { 'Content-Type': 'application/json' };
      if (statusCode === 429) {
        headers['Retry-After'] = String(scenarioState.retryAfter || '5');
      }
      res.writeHead(statusCode, headers);
      res.end(JSON.stringify({
        error: `Simulated HTTP ${statusCode} Fault`,
        status: statusCode,
        retryAfter: headers['Retry-After'] || null
      }));
      return;
    }

    // 4. Anti-Bot Telemetry Probes
    if (scenarioState.bot_challenge) {
      const acceptsHtml = (req.headers.accept && req.headers.accept.includes('text/html')) ||
                          (!pathname.includes('/api/') && !pathname.includes('/v1/') && !pathname.includes('/v0/'));
      if (acceptsHtml) {
        res.writeHead(200, { 'Content-Type': 'text/html' });
        res.end(buildBotChallengeHtml());
        return;
      }
    }

    let bodyChunks = [];
    req.on('data', chunk => bodyChunks.push(chunk));
    req.on('end', () => {
      const rawBody = Buffer.concat(bodyChunks).toString();
      const parsedEmail = parseEmailFromBody(rawBody, req.headers);

      // =======================================================================
      // GREENHOUSE ROUTES
      // =======================================================================
      // Greenhouse Direct API v1 Job Details: GET /v1/boards/:board/jobs/:jobId
      const ghApiMatch = pathname.match(/^\/v1\/boards\/([^/]+)\/jobs\/([^/]+)$/);
      if (ghApiMatch) {
        const board = ghApiMatch[1];
        const jobId = ghApiMatch[2];

        if (req.method === 'POST') {
          const receipt = `GH-REC-${Date.now()}`;
          const duration = Date.now() - reqStartTime;
          recordedSubmissions.greenhouse.push({
            company: board,
            jobId: jobId,
            email: parsedEmail,
            receipt: receipt,
            duration: duration,
            headers: req.headers,
            time: Date.now(),
            rawBody: rawBody
          });
          res.writeHead(200, { 'Content-Type': 'application/json' });
          res.end(JSON.stringify({
            id: Date.now(),
            application_id: Date.now(),
            status: 'submitted',
            receipt: receipt
          }));
          return;
        }

        // GET method: Return job details AST with questions if requested
        const jobDetails = {
          id: parseInt(jobId, 10) || jobId,
          title: 'Software Engineer',
          updated_at: '2026-01-01T00:00:00Z',
          requisition_id: `REQ-${jobId}`,
          notes: 'Mock Greenhouse AST Job',
          location: { name: 'San Francisco, CA' },
          departments: [{ id: 1, name: 'Engineering' }],
          offices: [{ id: 1, name: 'Headquarters', location: 'San Francisco, CA' }],
          content: '<p>OneRole Software Engineer role</p>',
          questions: [
            { id: 101, label: 'First Name', required: true, fields: [{ name: 'first_name', type: 'input_text' }] },
            { id: 102, label: 'Last Name', required: true, fields: [{ name: 'last_name', type: 'input_text' }] },
            { id: 103, label: 'Email', required: true, fields: [{ name: 'email', type: 'input_text' }] },
            { id: 104, label: 'Phone', required: true, fields: [{ name: 'phone', type: 'input_text' }] },
            { id: 105, label: 'Resume', required: true, fields: [{ name: 'resume', type: 'input_file' }] },
            { id: 106, label: 'Degree Level', required: false, fields: [{ name: 'question_degree', type: 'multi_value_single_select' }] },
            { id: 107, label: 'Authorized to work in the US?', required: true, fields: [{ name: 'question_auth', type: 'multi_value_single_select' }] }
          ]
        };
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify(jobDetails));
        return;
      }

      // Greenhouse HTML Form Routes
      if (pathname.startsWith('/greenhouse/') && pathname.includes('/jobs/')) {
        const parts = pathname.split('/');
        const company = parts[2];
        const jobId = parts[4];
        if (pathname.endsWith('/submit')) {
          const receipt = `GH-HTML-${Date.now()}`;
          const duration = Date.now() - reqStartTime;
          recordedSubmissions.greenhouse.push({
            company,
            jobId,
            email: parsedEmail,
            receipt: receipt,
            duration: duration,
            headers: req.headers,
            time: Date.now(),
            rawBody
          });
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

      // =======================================================================
      // ASHBY ROUTES
      // =======================================================================
      // Ashby 2-Step GraphQL Mutations
      if (pathname.includes('/api/non-user-graphql')) {
        const op = query.op || query.operationName || parsed.searchParams.get('op') || parsed.searchParams.get('operationName');
        if (op === 'ApiCreateFileUploadHandle' || rawBody.includes('ApiCreateFileUploadHandle')) {
          res.writeHead(200, { 'Content-Type': 'application/json' });
          res.end(JSON.stringify({
            data: {
              fileUploadHandle: {
                handle: `ashby_handle_${Date.now()}`
              }
            }
          }));
          return;
        }

        if (op === 'ApiSubmitSingleApplicationFormAction' || rawBody.includes('ApiSubmitSingleApplicationFormAction')) {
          const receipt = `ASHBY-REC-${Date.now()}`;
          const duration = Date.now() - reqStartTime;
          recordedSubmissions.ashby.push({
            company: 'ashby',
            jobId: 'unknown',
            email: parsedEmail,
            receipt: receipt,
            duration: duration,
            headers: req.headers,
            time: Date.now(),
            rawBody: rawBody
          });
          res.writeHead(200, { 'Content-Type': 'application/json' });
          res.end(JSON.stringify({
            data: {
              submitApplicationFormAction: {
                applicationFormResult: {
                  __typename: 'FormSubmitSuccess'
                },
                applicationId: `ASH-APP-${Date.now()}`,
                receipt: receipt,
                messages: []
              }
            }
          }));
          return;
        }
      }

      // Ashby Legacy Portal GraphQL
      if (pathname === '/ashby/api/candidate-portal/graphql') {
        const receipt = 'ASHBY-MOCK-' + Date.now();
        const duration = Date.now() - reqStartTime;
        recordedSubmissions.ashby.push({
          body: rawBody,
          email: parsedEmail,
          receipt: receipt,
          duration: duration,
          headers: req.headers,
          time: Date.now()
        });
        res.writeHead(200, { 'Content-Type': 'application/json' });
        if (scenarioState.ashbyGraphqlMode === 'success') {
          res.end(JSON.stringify({ data: { submitApplication: { success: true, receipt: receipt } } }));
        } else if (scenarioState.ashbyGraphqlMode === 'inverted_trap') {
          res.end(JSON.stringify({ data: null, errors: [{ message: 'Missing entry for required field: Location' }] }));
        } else {
          res.end(JSON.stringify({ errors: [{ message: 'Submission failed' }] }));
        }
        return;
      }

      // Ashby HTML Page with Next.js AST
      if (pathname.startsWith('/ashby/')) {
        const parts = pathname.split('/').filter(Boolean);
        const company = parts[1] || 'techcorp';
        const jobId = parts[2] || 'swe-backend';
        res.writeHead(200, { 'Content-Type': 'text/html' });
        res.end(buildAshbyHtml(company, jobId));
        return;
      }

      // =======================================================================
      // LEVER ROUTES
      // =======================================================================
      // Lever Direct API v0 Posting Details: GET /v0/postings/:company/:jobId
      const leverApiMatch = pathname.match(/^\/v0\/postings\/([^/]+)\/([^/]+)$/);
      if (leverApiMatch) {
        const company = leverApiMatch[1];
        const jobId = leverApiMatch[2];

        if (req.method === 'POST') {
          const receipt = `LEVER-REC-${Date.now()}`;
          const duration = Date.now() - reqStartTime;
          const isConflict = detectPronounConflict(rawBody);
          recordedSubmissions.lever.push({
            company: company,
            jobId: jobId,
            email: parsedEmail,
            receipt: receipt,
            hasPronounConflict: isConflict,
            duration: duration,
            headers: req.headers,
            time: Date.now(),
            rawBody: rawBody
          });
          res.writeHead(200, { 'Content-Type': 'application/json' });
          res.end(JSON.stringify({
            data: {
              id: receipt,
              applicationId: receipt,
              receipt: receipt
            }
          }));
          return;
        }

        // GET: Return Lever posting details with lists and customQuestions
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({
          id: jobId,
          text: 'Senior Software Engineer',
          createdAt: Date.now(),
          categories: {
            team: 'Engineering',
            department: 'Infrastructure',
            location: 'San Francisco, CA',
            commitment: 'Full-time'
          },
          content: {
            description: '<div>OneRole high-scale backend role</div>',
            lists: [
              { text: 'Team', content: 'Core Infrastructure' },
              { text: 'Requirements', content: 'Go, Node.js, Distributed Systems' }
            ]
          },
          lists: [
            { text: 'Team', content: 'Core Infrastructure' },
            { text: 'Requirements', content: 'Go, Node.js, Distributed Systems' }
          ],
          customQuestions: [
            {
              id: 'cq_pronouns',
              text: 'Pronouns',
              type: 'checkbox',
              options: ['He/Him', 'She/Her', 'They/Them'],
              required: false
            },
            {
              id: 'cq_sponsorship',
              text: 'Do you now or in the future require visa sponsorship?',
              type: 'dropdown',
              options: ['Yes', 'No'],
              required: true
            }
          ]
        }));
        return;
      }

      // Lever Multipart Submit: POST /jobs.lever.co/:company/:jobId/apply
      const leverWebApplyMatch = pathname.match(/^\/jobs\.lever\.co\/([^/]+)\/([^/]+)\/apply$/);
      if (leverWebApplyMatch && req.method === 'POST') {
        const company = leverWebApplyMatch[1];
        const jobId = leverWebApplyMatch[2];
        const receipt = `LEVER-REC-${Date.now()}`;
        const duration = Date.now() - reqStartTime;
        const isConflict = detectPronounConflict(rawBody);
        recordedSubmissions.lever.push({
          company: company,
          jobId: jobId,
          email: parsedEmail,
          receipt: receipt,
          hasPronounConflict: isConflict,
          duration: duration,
          headers: req.headers,
          time: Date.now(),
          rawBody: rawBody
        });
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({
          data: {
            id: receipt,
            receipt: receipt
          }
        }));
        return;
      }

      // Lever HTML Form Routes
      if (pathname.startsWith('/lever/') && pathname.endsWith('/apply')) {
        const parts = pathname.split('/');
        res.writeHead(200, { 'Content-Type': 'text/html' });
        res.end(buildLeverHtml(parts[2], parts[3]));
        return;
      }
      if (pathname.startsWith('/lever/') && pathname.endsWith('/submit')) {
        const parts = pathname.split('/');
        const isConflict = detectPronounConflict(rawBody);
        const receipt = `LEVER-REC-${Date.now()}`;
        const duration = Date.now() - reqStartTime;
        recordedSubmissions.lever.push({
          company: parts[2],
          jobId: parts[3],
          email: parsedEmail,
          receipt: receipt,
          rawBody,
          hasPronounConflict: isConflict,
          duration: duration,
          headers: req.headers,
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

      // =======================================================================
      // SMARTRECRUITERS ROUTES
      // =======================================================================
      // SmartRecruiters Posting Details AST: GET /v1/companies/:company/postings/:postingId
      const srPostingMatch = pathname.match(/^\/v1\/companies\/([^/]+)\/postings\/([^/]+)$/);
      if (srPostingMatch) {
        const company = srPostingMatch[1];
        const postingId = srPostingMatch[2];
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({
          id: postingId,
          name: 'Staff Software Engineer',
          company: { identifier: company, name: company },
          releasedDate: '2026-01-01T00:00:00Z',
          location: { city: 'San Francisco', region: 'CA', country: 'us' },
          questions: [
            { id: 'firstName', label: 'First Name', type: 'text', required: true },
            { id: 'lastName', label: 'Last Name', type: 'text', required: true },
            { id: 'email', label: 'Email', type: 'email', required: true },
            { id: 'phoneNumber', label: 'Phone Number', type: 'text', required: true },
            { id: 'resume', label: 'Resume', type: 'attachment', required: true },
            { id: 'linkedin', label: 'LinkedIn Profile', type: 'url', required: false },
            { id: 'github', label: 'GitHub Profile', type: 'url', required: false }
          ]
        }));
        return;
      }

      // SmartRecruiters Candidate Submission: POST /v1/companies/:company/postings/:postingId/candidates
      const srCandidateMatch = pathname.match(/^\/v1\/companies\/([^/]+)\/postings\/([^/]+)\/candidates$/);
      if (srCandidateMatch && req.method === 'POST') {
        const company = srCandidateMatch[1];
        const postingId = srCandidateMatch[2];
        const receipt = `SR-CAND-${Date.now()}`;
        const duration = Date.now() - reqStartTime;
        recordedSubmissions.smartrecruiters.push({
          company: company,
          jobId: postingId,
          postingId: postingId,
          email: parsedEmail,
          receipt: receipt,
          duration: duration,
          headers: req.headers,
          time: Date.now(),
          rawBody: rawBody
        });
        res.writeHead(201, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({
          id: receipt,
          candidateId: receipt,
          status: 'CREATED',
          receipt: receipt
        }));
        return;
      }

      // SmartRecruiters HTML Form & window.__oneRoleApply
      if (pathname.startsWith('/smartrecruiters/')) {
        const parts = pathname.split('/').filter(Boolean);
        const company = parts[1] || 'acme';
        const jobId = parts[2] || 'sr-001';

        if (pathname.endsWith('/submit')) {
          const receipt = `SR-HTML-${Date.now()}`;
          const duration = Date.now() - reqStartTime;
          recordedSubmissions.smartrecruiters.push({
            company: company,
            jobId: jobId,
            postingId: jobId,
            email: parsedEmail,
            receipt: receipt,
            duration: duration,
            headers: req.headers,
            time: Date.now(),
            rawBody: rawBody
          });
          res.writeHead(200, { 'Content-Type': 'text/html' });
          res.end(`<div id="st-confirmation"><h1>Application submitted successfully!</h1><span id="st-receipt-id">${receipt}</span></div>`);
          return;
        }

        res.writeHead(200, { 'Content-Type': 'text/html' });
        res.end(buildSmartRecruitersHtml(company, jobId));
        return;
      }

      // =======================================================================
      // WORKDAY CXS & MULTI-STEP WIZARD ROUTES
      // =======================================================================
      // Workday CXS Job Details: GET /wday/cxs/:tenant/:site/jobs/:jobId
      const cxsJobMatch = pathname.match(/^\/wday\/cxs\/([^/]+)\/([^/]+)\/jobs\/([^/]+)$/);
      if (cxsJobMatch && req.method === 'GET') {
        const tenant = cxsJobMatch[1];
        const site = cxsJobMatch[2];
        const jobId = cxsJobMatch[3];
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({
          jobPostingInfo: {
            id: jobId,
            title: 'Senior Systems Engineer',
            canQuickApply: true,
            hasQuickApply: true,
            jobReqId: jobId,
            externalPosting: true,
            timeType: 'Full time',
            location: 'San Francisco, CA'
          }
        }));
        return;
      }

      // Workday CXS Apply: POST /wday/cxs/:tenant/:site/jobs/:jobId/apply
      const cxsApplyMatch = pathname.match(/^\/wday\/cxs\/([^/]+)\/([^/]+)\/jobs\/([^/]+)\/apply$/);
      if (cxsApplyMatch && req.method === 'POST') {
        const tenant = cxsApplyMatch[1];
        const site = cxsApplyMatch[2];
        const jobId = cxsApplyMatch[3];
        const receipt = `WD-CXS-${tenant.toUpperCase()}-${Date.now().toString().slice(-8)}`;
        const duration = Date.now() - reqStartTime;
        recordedSubmissions.workday.push({
          company: tenant,
          tenant: tenant,
          site: site,
          jobId: jobId,
          email: parsedEmail,
          receipt: receipt,
          duration: duration,
          headers: req.headers,
          time: Date.now(),
          rawBody: rawBody
        });
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({
          receipt: receipt,
          receiptId: receipt,
          status: 'SUCCESS'
        }));
        return;
      }

      // Workday Multi-Step Wizard Routes
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
          const receipt = `WD-WIZARD-${Date.now()}`;
          const duration = Date.now() - reqStartTime;
          recordedSubmissions.workday.push({
            company,
            jobId: 'wizard-job',
            email: parsedEmail,
            receipt: receipt,
            duration: duration,
            headers: req.headers,
            time: Date.now(),
            rawBody: rawBody
          });
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

  return new Promise((resolve, reject) => {
    const onError = (err) => {
      try {
        if (typeof serverInstance.closeAllConnections === 'function') {
          serverInstance.closeAllConnections();
        }
        serverInstance.close();
      } catch (_) {}
      serverInstance = null;
      reject(err);
    };

    serverInstance.once('error', onError);
    serverInstance.listen(port, () => {
      serverInstance.removeListener('error', onError);
      // Attach non-crashing logger for any subsequent runtime errors
      serverInstance.on('error', (err) => {
        console.error('[MockATS] Server runtime error:', err.message);
      });
      resolve();
    });
  });
}

function stop() {
  if (!serverInstance) return Promise.resolve();
  return new Promise(resolve => {
    const inst = serverInstance;
    serverInstance = null;
    try {
      if (typeof inst.closeAllConnections === 'function') {
        inst.closeAllConnections();
      }
    } catch (_) {}
    inst.close(() => {
      resolve();
    });
  });
}

function setScenario(keyOrOpts, val) {
  if (typeof keyOrOpts === 'string') {
    if (val !== undefined) {
      scenarioState[keyOrOpts] = val;
      if (keyOrOpts === 'bot_challenge') scenarioState.bot_challenge = !!val;
    } else {
      if (keyOrOpts === 'bot_challenge') {
        scenarioState.bot_challenge = true;
      } else if (keyOrOpts === 'success') {
        resetScenario();
      } else {
        scenarioState[keyOrOpts] = true;
      }
    }
  } else if (typeof keyOrOpts === 'object' && keyOrOpts !== null) {
    Object.assign(scenarioState, keyOrOpts);
    if (keyOrOpts.bot_challenge !== undefined) {
      scenarioState.bot_challenge = !!keyOrOpts.bot_challenge;
    }
    if (keyOrOpts.status !== undefined) {
      scenarioState.status = keyOrOpts.status;
    }
    if (keyOrOpts.statusCode !== undefined) {
      scenarioState.status = keyOrOpts.statusCode;
    }
  }
}

function setLatency(ms) {
  artificialLatencyMs = typeof ms === 'number' ? ms : 0;
}

function resetScenario() {
  scenarioState.ashbyGraphqlMode = 'success';
  scenarioState.workdayStep = 1;
  scenarioState.leverPronounStatus = 'clean';
  scenarioState.status = null;
  scenarioState.statusCode = null;
  scenarioState.dropConnection = false;
  scenarioState.bot_challenge = false;
  scenarioState.retryAfter = null;
  artificialLatencyMs = 0;
}

function getSubmissions(ats) {
  return recordedSubmissions[ats] || [];
}

function getAllSubmissions() {
  return recordedSubmissions;
}

function clearSubmissions() {
  for (const k of Object.keys(recordedSubmissions)) {
    recordedSubmissions[k] = [];
  }
  resetScenario();
}

module.exports = {
  start,
  stop,
  setScenario,
  setLatency,
  resetScenario,
  getSubmissions,
  getAllSubmissions,
  clearSubmissions,
  buildGreenhouseHtml,
  buildAshbyHtml,
  buildLeverHtml,
  buildSmartRecruitersHtml,
  buildWorkdayStepHtml,
  buildBotChallengeHtml
};
