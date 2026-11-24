/**
 * OneRole High-Speed ATS Direct Submitter (Tier 1 Fast-Path Engine)
 * Sub-second submission via Direct ATS APIs (Greenhouse, Lever, Ashby).
 * Eliminates Chromium cold-start overhead, providing 150ms-400ms execution per application.
 * Falls back to Headless Chromium only when custom unmapped forms or bot gates are present.
 */

const fs = require('fs');
const path = require('path');
const { extractWorkdayParams, submitWorkdayCXS } = require('./workday_cxs');

function resolveResumeBuffer() {
  const possiblePaths = [
    '/app/Nakul_Bhandare_Resume.pdf',
    '/app/oneRole_UI/public/Nakul_Bhandare_Resume.pdf',
    path.join(__dirname, '../../oneRole_UI/public/Nakul_Bhandare_Resume.pdf'),
    path.join(__dirname, '../Nakul_Bhandare_Resume.pdf')
  ];
  for (const p of possiblePaths) {
    if (fs.existsSync(p)) {
      try {
        return fs.readFileSync(p);
      } catch (e) {}
    }
  }
  return null;
}

function extractGreenhouseParams(url) {
  if (!url) return null;
  try {
    const u = new URL(url);
    const search = u.searchParams;
    const ghJid = search.get('gh_jid');
    const token = search.get('token');
    const forBoard = search.get('for');

    if (forBoard && token) {
      return { board: forBoard.trim(), jobId: token.trim() };
    }

    if (ghJid) {
      const host = u.hostname.replace(/^(www\.|jobs\.|careers\.)/, '');
      const domainBoard = host.split('.')[0];
      return { board: domainBoard, jobId: ghJid };
    }

    const segments = u.pathname.split('/').filter(Boolean);
    if (segments.length >= 3 && segments[1] === 'jobs') {
      return { board: segments[0], jobId: segments[2] };
    }
    if (segments.length >= 4 && segments[0] === 'job-board' && segments[2] === 'job') {
      return { board: segments[1], jobId: segments[3] };
    }
    if (segments.length >= 3 && (segments[0] === 'job-boards' || segments[0] === 'job-board')) {
      return { board: segments[1], jobId: segments[3] || segments[2] };
    }
  } catch (e) {}
  return null;
}

function extractLeverParams(url) {
  if (!url) return null;
  try {
    const u = new URL(url);
    if (u.hostname.includes('lever.co')) {
      const segments = u.pathname.split('/').filter(Boolean);
      if (segments.length >= 2) {
        return { board: segments[0], jobId: segments[1] };
      }
    }
  } catch (e) {}
  return null;
}

function extractSmartRecruitersParams(url) {
  if (!url) return null;
  try {
    const u = new URL(url);
    if (u.hostname.includes('smartrecruiters.com')) {
      const segments = u.pathname.split('/').filter(Boolean);
      if (segments.length >= 2) {
        let postingId = segments[1];
        if (postingId.includes('-')) {
          postingId = postingId.split('-')[0];
        }
        return { company: segments[0], postingId };
      }
    }
  } catch (e) {}
  return null;
}

function extractAshbyParams(url) {
  if (!url) return null;
  try {
    const u = new URL(url);
    if (u.hostname.includes('ashbyhq.com')) {
      const segments = u.pathname.split('/').filter(Boolean);
      if (segments.length >= 2) {
        return { org: segments[0], jobId: segments[1] };
      }
    }
  } catch (e) {}
  return null;
}

/**
 * Fast-path direct submitter function
 */
async function submitFastApplication({ url, candidate, dryRun }) {
  if (!url || !candidate) {
    throw new Error('url and candidate required');
  }

  const startTime = Date.now();
  const sr = extractSmartRecruitersParams(url);
  const gh = extractGreenhouseParams(url);
  const lever = extractLeverParams(url);
  const ashby = extractAshbyParams(url);

  const candidateEmail = (!candidate.email || candidate.email === 'test@onerole.ai' || candidate.email.includes('mail.onerole.com') || candidate.email.includes('apply.onerole.com'))
    ? 'bhandarenakul11@gmail.com'
    : candidate.email;
  const firstName = candidate.first_name || 'Nakul';
  const lastName = candidate.last_name || 'Bhandare';
  const fullName = `${firstName} ${lastName}`.trim();
  const phone = candidate.phone || '+1 (350)-355-6570';
  const resumeBuf = resolveResumeBuffer();

  // =========================================================================
  // 1. Direct SmartRecruiters Candidate API (Priority #1 - Sub-300ms)
  // =========================================================================
  if (sr && sr.company && sr.postingId) {
    console.log(`[Fast-Path Engine] Dispatched direct SmartRecruiters API: company=${sr.company}, posting=${sr.postingId}`);
    try {
      const endpoint = `https://api.smartrecruiters.com/v1/companies/${sr.company}/postings/${sr.postingId}/candidates`;
      const body = {
        firstName: firstName,
        lastName: lastName,
        email: candidateEmail,
        phoneNumber: phone,
        location: {
          city: candidate.city || 'San Francisco',
          country: 'us'
        },
        web: {
          linkedin: candidate.linkedin || 'https://linkedin.com/in/nakul-bhandare',
          github: candidate.github || 'https://github.com/nakulbhandare'
        }
      };
      if (resumeBuf) {
        body.attachments = [
          {
            name: 'Nakul_Bhandare_Resume.pdf',
            contentType: 'application/pdf',
            content: resumeBuf.toString('base64')
          }
        ];
      }

      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), 8000);
      const res = await fetch(endpoint, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'User-Agent': 'OneRole-HighSpeedSubmitter/3.0'
        },
        body: JSON.stringify(body),
        signal: controller.signal
      });
      clearTimeout(timeoutId);

      const status = res.status;
      const respText = await res.text().catch(() => '');

      if (status >= 200 && status < 300) {
        const duration = Date.now() - startTime;
        let receipt = null;
        try {
          const parsed = JSON.parse(respText);
          if (parsed && (parsed.id || parsed.candidateId)) {
            receipt = String(parsed.id || parsed.candidateId);
          }
        } catch (_) {}
        console.log(`[Fast-Path Engine] SmartRecruiters Direct API SUCCESS (${duration}ms): Receipt=${receipt || 'Awaiting Confirmation'}`);
        return {
          success: true,
          receipt_id: receipt,
          confirmed: !!receipt,
          unverified: !receipt,
          provider: 'smartrecruiters_direct_api',
          confirmation_url: url,
          duration_ms: duration
        };
      } else {
        console.warn(`[Fast-Path Engine] SmartRecruiters returned HTTP ${status}: ${respText.slice(0, 100)}. Falling back.`);
        return {
          success: false,
          fallback_to_browser: true,
          reason: `SmartRecruiters HTTP ${status}: ${respText.slice(0, 100)}`
        };
      }
    } catch (e) {
      console.warn(`[Fast-Path Engine] SmartRecruiters Direct API error: ${e.message}`);
      return {
        success: false,
        fallback_to_browser: true,
        reason: e.message
      };
    }
  }

  // =========================================================================
  // 2. Direct Greenhouse API Ingestion (Sub-250ms)
  // =========================================================================
  if (gh && gh.board && gh.jobId) {
    console.log(`[Fast-Path Engine] Dispatched direct Greenhouse API submission: board=${gh.board}, job=${gh.jobId}`);
    try {
      const formData = new FormData();
      formData.append('first_name', firstName);
      formData.append('last_name', lastName);
      formData.append('email', candidateEmail);
      formData.append('phone', phone);
      if (candidate.city || candidate.location) {
        formData.append('location', candidate.city || candidate.location || 'San Francisco, CA');
      }

      if (resumeBuf) {
        const resumeBlob = new Blob([resumeBuf], { type: 'application/pdf' });
        formData.append('resume', resumeBlob, 'Nakul_Bhandare_Resume.pdf');
      }

      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), 6000);

      const endpoint = `https://boards-api.greenhouse.io/v1/boards/${gh.board}/jobs/${gh.jobId}`;
      const res = await fetch(endpoint, {
        method: 'POST',
        body: formData,
        headers: {
          'User-Agent': 'OneRole-HighSpeedSubmitter/3.0'
        },
        signal: controller.signal
      });
      clearTimeout(timeoutId);

      const status = res.status;
      const respText = await res.text().catch(() => '');

      if (status >= 200 && status < 300) {
        const duration = Date.now() - startTime;
        let ghReceipt = null;
        try {
          const parsed = JSON.parse(respText);
          if (parsed && (parsed.id || parsed.application_id)) {
            ghReceipt = String(parsed.id || parsed.application_id);
          }
        } catch (_) {}

        console.log(`[Fast-Path Engine] Greenhouse Direct API submission SUCCESS (${duration}ms): Receipt=${ghReceipt || 'Awaiting Confirmation'}`);
        return {
          success: true,
          receipt_id: ghReceipt,
          confirmed: !!ghReceipt,
          unverified: !ghReceipt,
          provider: 'greenhouse_direct_api',
          confirmation_url: url,
          duration_ms: duration
        };
      } else {
        console.warn(`[Fast-Path Engine] Greenhouse Direct API returned ${status} (${respText.slice(0, 100)}). Falling back to Chromium.`);
        return {
          success: false,
          fallback_to_browser: true,
          reason: `Greenhouse API HTTP ${status}: ${respText.slice(0, 100)}`
        };
      }
    } catch (e) {
      console.warn(`[Fast-Path Engine] Greenhouse Direct API error: ${e.message}. Falling back to Chromium.`);
      return {
        success: false,
        fallback_to_browser: true,
        reason: e.message
      };
    }
  }

  // =========================================================================
  // 2. Direct Lever Ingestion API (Sub-200ms)
  // =========================================================================
  if (lever && lever.board && lever.jobId) {
    console.log(`[Fast-Path Engine] Dispatched direct Lever API submission: board=${lever.board}, job=${lever.jobId}`);
    try {
      const formData = new FormData();
      formData.append('name', fullName);
      formData.append('email', candidateEmail);
      formData.append('phone', phone);
      if (candidate.linkedin) {
        formData.append('urls[LinkedIn]', candidate.linkedin);
      } else {
        formData.append('urls[LinkedIn]', 'https://linkedin.com/in/nakul-bhandare');
      }
      if (candidate.github) {
        formData.append('urls[GitHub]', candidate.github);
      }

      if (resumeBuf) {
        const resumeBlob = new Blob([resumeBuf], { type: 'application/pdf' });
        formData.append('resume', resumeBlob, 'Nakul_Bhandare_Resume.pdf');
      }

      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), 6000);

      // First try public web form apply endpoint, then fallback to API endpoint
      const webEndpoint = `https://jobs.lever.co/${lever.board}/${lever.jobId}/apply`;
      const res = await fetch(webEndpoint, {
        method: 'POST',
        body: formData,
        headers: {
          'User-Agent': 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36',
          'Origin': 'https://jobs.lever.co',
          'Referer': webEndpoint
        },
        signal: controller.signal
      });
      clearTimeout(timeoutId);

      const status = res.status;
      const respText = await res.text().catch(() => '');

      if (status >= 200 && status < 300) {
        const duration = Date.now() - startTime;
        let leverReceipt = null;
        try {
          const parsed = JSON.parse(respText);
          if (parsed && parsed.data && parsed.data.id) {
            leverReceipt = String(parsed.data.id);
          }
        } catch (_) {}

        console.log(`[Fast-Path Engine] Lever Direct API submission SUCCESS (${duration}ms): Receipt=${leverReceipt || 'Awaiting Confirmation'}`);
        return {
          success: true,
          receipt_id: leverReceipt,
          confirmed: !!leverReceipt,
          unverified: !leverReceipt,
          provider: 'lever_direct_api',
          confirmation_url: url,
          duration_ms: duration
        };
      } else {
        console.warn(`[Fast-Path Engine] Lever Direct API returned ${status} (${respText.slice(0, 100)}). Falling back to Chromium.`);
        return {
          success: false,
          fallback_to_browser: true,
          reason: `Lever API HTTP ${status}: ${respText.slice(0, 100)}`
        };
      }
    } catch (e) {
      console.warn(`[Fast-Path Engine] Lever Direct API error: ${e.message}. Falling back to Chromium.`);
      return {
        success: false,
        fallback_to_browser: true,
        reason: e.message
      };
    }
  }

  // =========================================================================
  // 3. Direct Ashby GraphQL API (Sub-350ms)
  // =========================================================================
  if (ashby && ashby.org && ashby.jobId) {
    console.log(`[Fast-Path Engine] Dispatched direct Ashby GraphQL submission: org=${ashby.org}, job=${ashby.jobId}`);
    try {
      let fileHandle = null;

      // Step 3a: Upload file handle to Ashby
      if (resumeBuf) {
        try {
          const uploadFormData = new FormData();
          const resumeBlob = new Blob([resumeBuf], { type: 'application/pdf' });
          uploadFormData.append('file', resumeBlob, 'Nakul_Bhandare_Resume.pdf');

          const uploadRes = await fetch('https://jobs.ashbyhq.com/api/non-user-graphql?op=ApiCreateFileUploadHandle', {
            method: 'POST',
            body: uploadFormData,
            headers: {
              'User-Agent': 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36'
            }
          });
          if (uploadRes.ok) {
            const upJson = await uploadRes.json().catch(() => null);
            if (upJson && upJson.data && upJson.data.fileUploadHandle) {
              fileHandle = upJson.data.fileUploadHandle.handle;
            }
          }
        } catch (upErr) {
          console.warn(`[Fast-Path Engine] Ashby direct upload handle warning: ${upErr.message}`);
        }
      }

      // Step 3b: Post ApiSubmitSingleApplicationFormAction mutation
      if (fileHandle) {
        const mutation = `
          mutation ApiSubmitSingleApplicationFormAction($input: ApiSubmitSingleApplicationFormInput!) {
            submitApplicationFormAction(input: $input) {
              applicationFormResult {
                __typename
              }
              messages {
                field
                message
              }
            }
          }
        `;

        const variables = {
          input: {
            organizationHostedJobsPageName: ashby.org,
            jobPostingId: ashby.jobId,
            fieldValues: [
              { fieldEntryId: '_systemfield_name', value: fullName },
              { fieldEntryId: '_systemfield_email', value: candidateEmail },
              { fieldEntryId: '_systemfield_phone', value: phone },
              { fieldEntryId: '_systemfield_resume', value: { fileUploadHandle: fileHandle } }
            ]
          }
        };

        const ashbyRes = await fetch('https://jobs.ashbyhq.com/api/non-user-graphql?op=ApiSubmitSingleApplicationFormAction', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'User-Agent': 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36'
          },
          body: JSON.stringify({
            operationName: 'ApiSubmitSingleApplicationFormAction',
            variables: variables,
            query: mutation
          })
        });

        if (ashbyRes.ok) {
          const ashbyJson = await ashbyRes.json().catch(() => null);
          const hasSuccess = ashbyJson && ashbyJson.data &&
            ashbyJson.data.submitApplicationFormAction &&
            ashbyJson.data.submitApplicationFormAction.applicationFormResult &&
            ashbyJson.data.submitApplicationFormAction.applicationFormResult.__typename === 'FormSubmitSuccess';

          if (hasSuccess) {
            const duration = Date.now() - startTime;
            let receipt = null;
            if (ashbyJson && ashbyJson.data && ashbyJson.data.submitApplicationFormAction && ashbyJson.data.submitApplicationFormAction.applicationId) {
              receipt = String(ashbyJson.data.submitApplicationFormAction.applicationId);
            }
            console.log(`[Fast-Path Engine] Ashby Direct GraphQL submission SUCCESS (${duration}ms): Receipt=${receipt || 'Awaiting Confirmation'}`);
            return {
              success: true,
              receipt_id: receipt,
              confirmed: !!receipt,
              unverified: !receipt,
              provider: 'ashby_direct_graphql',
              confirmation_url: url,
              duration_ms: duration
            };
          } else {
            console.warn(`[Fast-Path Engine] Ashby Direct GraphQL requires custom fields / bot check. Falling back to Chromium.`);
          }
        }
      }
    } catch (e) {
      console.warn(`[Fast-Path Engine] Ashby Direct GraphQL error: ${e.message}. Falling back to Chromium.`);
    }

    return {
      success: false,
      fallback_to_browser: true,
      reason: 'Ashby direct GraphQL requires full form DOM interaction'
    };
  }

  // =========================================================================
  // 4. Workday CXS Direct REST Engine (Sub-1.5s)
  // =========================================================================
  const wd = extractWorkdayParams(url);
  if (wd) {
    console.log(`[Fast-Path Engine] Dispatched Workday CXS engine: tenant=${wd.tenant}, site=${wd.site}, job=${wd.jobId}`);
    try {
      const wdResult = await submitWorkdayCXS({ url, candidate, resumeBuf });
      if (wdResult && wdResult.success) {
        return wdResult;
      }
      return wdResult || { success: false, fallback_to_browser: true, reason: 'Workday CXS require browser' };
    } catch (e) {
      console.warn(`[Fast-Path Engine] Workday CXS error: ${e.message}. Falling back to Chromium.`);
      return {
        success: false,
        fallback_to_browser: true,
        reason: e.message
      };
    }
  }

  // Fallback for non-supported direct API platforms (custom, etc.)
  return {
    success: false,
    fallback_to_browser: true,
    reason: 'Platform requires browser submission'
  };
}

module.exports = {
  submitFastApplication,
  extractGreenhouseParams,
  extractLeverParams,
  extractAshbyParams,
  extractWorkdayParams,
  submitWorkdayCXS,
  resolveResumeBuffer
};
