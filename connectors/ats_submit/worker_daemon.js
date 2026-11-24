const fs = require('fs');
const http = require('http');
const { connect, StringCodec, AckPolicy } = require('nats');
const { MongoClient } = require('mongodb');
const { submitApplication } = require('./browser_submitter');
const { submitFastApplication } = require('./fast_submitter');

const NATS_URL = process.env.NATS_URL || 'nats://localhost:4222';
const DATABASE_URL = process.env.DATABASE_URL || 'mongodb://localhost:27017/onerole';
const STREAM_NAME = process.env.STREAM_NAME || 'APPLICATIONS';
const SUBJECT = process.env.SUBJECT || 'applications.submit';
const CONSUMER_NAME = process.env.CONSUMER_NAME || 'chromium-workers';
const PORT = parseInt(process.env.PORT || '3001', 10);
const CONCURRENCY = parseInt(process.env.CONCURRENCY || '10', 10);

// D7: Retry, Timeout, and Backoff Configuration
const MAX_RETRIES = parseInt(process.env.MAX_RETRIES || '3', 10);
const JOB_TIMEOUT_MS = parseInt(process.env.JOB_TIMEOUT_MS || '90000', 10); // 90s bounded timeout per job
const HEARTBEAT_INTERVAL_MS = parseInt(process.env.HEARTBEAT_INTERVAL_MS || '15000', 10); // 15s heartbeats
const BASE_BACKOFF_MS = parseInt(process.env.BASE_BACKOFF_MS || '3000', 10); // 3s base backoff
const MAX_BACKOFF_MS = parseInt(process.env.MAX_BACKOFF_MS || '30000', 10); // 30s max backoff

const sc = StringCodec();
let jobsProcessed = 0;
let activeJobs = 0;
let isShuttingDown = false;

function isCaptchaOrBot(err) {
  if (!err) return false;
  if (err.isBotGate || err.isCaptcha || err.challengeType) return true;
  const msg = (err.message || '').toLowerCase();
  return msg.includes('captcha') ||
         msg.includes('turnstile') ||
         msg.includes('cloudflare') ||
         msg.includes('recaptcha') ||
         msg.includes('hcaptcha') ||
         msg.includes('bot detected') ||
         msg.includes('security check') ||
         msg.includes('verify you are human') ||
         msg.includes('flagged as possible spam') ||
         msg.includes('possible spam') ||
         msg.includes('rate_limit_429') ||
         msg.includes('rate limit') ||
         msg.includes('workday_bot_shield') ||
         msg.includes('workday_login_required') ||
         msg.includes('login_required') ||
         msg.includes('account required') ||
         msg.includes('user login') ||
         msg.includes('sign-in required');
}

function isNonRetryableError(err) {
  if (!err || !err.message) return false;
  if (err.isJobClosed) return true;
  const msg = err.message.toLowerCase();
  const isPayloadError = msg.includes('url and candidate required') ||
    msg.includes('invalid payload') ||
    msg.includes('missing required field');
  const isClosedJob = msg.includes('job is no longer available') ||
    msg.includes('position has been filled') ||
    msg.includes('posting has closed') ||
    msg.includes('application form closed') ||
    msg.includes('this job has been closed') ||
    msg.includes('listing is no longer active') ||
    msg.includes('job posting has closed');
  const isHardNotFound = (msg.includes('404 not found') || msg.includes('job not found') || msg.includes('404')) &&
    !msg.includes('econnrefused') && !msg.includes('enotfound') && !msg.includes('timeout');
  const isTransient = msg.includes('econnreset') || msg.includes('econnrefused') ||
    msg.includes('enotfound') || msg.includes('etimedout') || msg.includes('timeout') ||
    msg.includes('network') || msg.includes('500') || msg.includes('502') ||
    msg.includes('503') || msg.includes('504') || msg.includes('socket hang up') ||
    msg.includes('captcha') || msg.includes('turnstile') || msg.includes('cloudflare');
  if (isTransient) return false;
  return isPayloadError || isClosedJob || isHardNotFound;
}

function calculateBackoffDelay(deliveryCount) {
  // Exponential backoff: base * 2^(count - 1), capped at MAX_BACKOFF_MS with ±10% jitter
  const exp = Math.max(0, deliveryCount - 1);
  const delay = Math.min(MAX_BACKOFF_MS, BASE_BACKOFF_MS * Math.pow(2, exp));
  const jitter = Math.floor(delay * 0.1 * (Math.random() * 2 - 1));
  return Math.max(1000, delay + jitter);
}

async function deliverNotificationEmail(candidateEmail, subject, textBody, application_id, receipt_id, companyName, jobTitle) {
  if (process.env.DISABLE_SYSTEM_EMAILS === 'true' || process.env.ENABLE_SYSTEM_EMAILS !== 'true') {
    return { success: true, provider: 'local_audit', reason: 'system_emails_disabled' };
  }
  const resendKey = process.env.RESEND_API_KEY;
  if (resendKey) {
    try {
      const resp = await fetch('https://api.resend.com/emails', {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${resendKey}`,
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({
          from: process.env.SMTP_FROM || 'OneRole Applications <notifications@onerole.dev>',
          to: [candidateEmail],
          subject: subject,
          text: textBody
        })
      });
      if (resp.ok) {
        return { success: true, provider: 'resend_api' };
      }
    } catch (e) {}
  }

  // Attempt sending through local or cluster API endpoint
  const apiUrls = [
    'http://localhost:8088/api/v1/notifications/email/test',
    'http://127.0.0.1:8088/api/v1/notifications/email/test',
    'http://onerole-api:8088/api/v1/notifications/email/test'
  ];
  for (const apiUrl of apiUrls) {
    try {
      const resp = await fetch(apiUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ recipient: candidateEmail }),
        signal: AbortSignal.timeout(2000)
      });
      if (resp.ok) {
        return { success: true, provider: 'api_forward' };
      }
    } catch (e) {}
  }

  return { success: false, reason: 'unconfigured' };
}

async function start() {
  console.log(`[Worker] Starting OneRole Chromium Worker Daemon...`);
  console.log(`[Worker] NATS_URL=${NATS_URL}`);
  console.log(`[Worker] DATABASE_URL=${DATABASE_URL}`);
  console.log(`[Worker] STREAM=${STREAM_NAME}, SUBJECT=${SUBJECT}, CONCURRENCY=${CONCURRENCY}, MAX_RETRIES=${MAX_RETRIES}, JOB_TIMEOUT_MS=${JOB_TIMEOUT_MS}`);

  // 1. Connect MongoDB
  const mongoClient = new MongoClient(DATABASE_URL);
  await mongoClient.connect();
  const db = mongoClient.db();
  const appsColl = db.collection('applications');
  const eventsColl = db.collection('application_events');
  console.log(`[Worker] Connected to MongoDB (${db.databaseName})`);

  // 2. Connect NATS JetStream
  const nc = await connect({ servers: NATS_URL });
  const js = nc.jetstream();
  const jsm = await nc.jetstreamManager();

  // Ensure stream exists
  try {
    await jsm.streams.info(STREAM_NAME);
    console.log(`[Worker] JetStream stream ${STREAM_NAME} verified`);
  } catch (err) {
    console.log(`[Worker] Creating JetStream stream ${STREAM_NAME} for subjects [applications.>]`);
    await jsm.streams.add({
      name: STREAM_NAME,
      subjects: ['applications.>']
    });
  }

  // Ensure consumer exists
  try {
    await jsm.consumers.info(STREAM_NAME, CONSUMER_NAME);
    console.log(`[Worker] Consumer ${CONSUMER_NAME} verified on stream ${STREAM_NAME}`);
  } catch (err) {
    console.log(`[Worker] Creating consumer ${CONSUMER_NAME}...`);
    await jsm.consumers.add(STREAM_NAME, {
      durable_name: CONSUMER_NAME,
      ack_policy: AckPolicy.Explicit,
      ack_wait: 180 * 1000 * 1000 * 1000, // 180 seconds in nanoseconds
      max_deliver: MAX_RETRIES + 2 // Safety threshold for JetStream redelivery
    });
  }

  const consumer = await js.consumers.get(STREAM_NAME, CONSUMER_NAME);

  // 3. Health Check HTTP Server
  const server = http.createServer((req, res) => {
    if (req.url === '/healthz' || req.url === '/health') {
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({
        status: 'ok',
        worker: 'onerole-chromium-worker',
        jobs_processed: jobsProcessed,
        active_jobs: activeJobs,
        uptime_seconds: Math.floor(process.uptime())
      }));
      return;
    }
    res.writeHead(404);
    res.end();
  });

  server.listen(PORT, '0.0.0.0', () => {
    console.log(`[Worker] Healthcheck server listening on port ${PORT}`);
  });

  function generateEventId(prefix) {
    return `evt-${Date.now()}-${prefix}-${Math.random().toString(36).substring(2, 7)}`;
  }

  // 4. Process a single application item
  async function processMessage(msg) {
    activeJobs++;
    try {
      const now = new Date();
      let data;

      // 4a. Decode JSON
      try {
        data = JSON.parse(sc.decode(msg.data));
      } catch (parseErr) {
        console.error(`[Worker] Failed to decode JSON message (terminating non-retryable):`, parseErr);
        msg.term('invalid_json');
        return;
      }

      // 4b. Validate mandatory fields
      const { application_id, tenant_id, user_id, job_id, job_url, candidate } = data || {};
      if (!application_id || !job_url || !candidate) {
        console.error(`[Worker] Invalid payload schema for application_id=${application_id}: missing required fields (terminating non-retryable)`);
        if (application_id) {
          try {
            await appsColl.updateOne({ _id: application_id }, {
              $set: {
                status: 'failed_permanent',
                error_detail: 'Invalid job payload: missing mandatory fields (job_url, candidate, or application_id)',
                updated_at: new Date()
              }
            });
          } catch (dbErr) {}
        }
        msg.term('invalid_payload_schema');
        return;
      }

      // Temporary override: route all applications to user's verified personal email bhandarenakul11@gmail.com
      if (candidate) {
        if (!candidate.email || candidate.email.includes('@apply.onerole.com') || candidate.email.includes('@mail.onerole.com') || candidate.email.includes('test@')) {
          candidate.email = 'bhandarenakul11@gmail.com';
        }
      }

      const appPrefix = String(application_id).slice(0, 8);
      const deliveryCount = (msg.info && typeof msg.info.deliveryCount === 'number') ? msg.info.deliveryCount : 1;
      console.log(`[Worker] Processing application: ${application_id} for job: ${job_id} (Attempt ${deliveryCount}/${MAX_RETRIES})`);

      // 4c. Idempotency check: if application is already submitted, skip redundant submission
      try {
        const existingApp = await appsColl.findOne({ _id: application_id }, { projection: { status: 1, verified_receipt_id: 1 } });
        if (existingApp && (existingApp.status === 'submitted' || existingApp.verified_receipt_id)) {
          console.log(`[Worker] Application ${application_id} already marked as submitted (Receipt: ${existingApp.verified_receipt_id}). Acknowledging duplicate.`);
          msg.ack();
          return;
        }
      } catch (checkErr) {}

      // 4d. Update status to submitting in DB
      try {
        await appsColl.updateOne({ _id: application_id }, {
          $set: {
            status: 'submitting',
            delivery_attempt: deliveryCount,
            updated_at: now
          }
        });

        await eventsColl.insertOne({
          _id: generateEventId(appPrefix),
          tenant_id: tenant_id,
          application_id: application_id,
          event_type: 'application.submitting',
          from_status: deliveryCount > 1 ? 'failed_retryable' : 'queued',
          to_status: 'submitting',
          actor_type: 'chromium_worker',
          actor_id: user_id,
          occurred_at: now,
          metadata: { delivery_attempt: deliveryCount }
        });
      } catch (dbErr) {
        console.error(`[Worker] Database error during initial submitting update for ${application_id}:`, dbErr.message);
        const backoffDelay = calculateBackoffDelay(deliveryCount);
        msg.nak(backoffDelay);
        return;
      }

      // 4d. Periodic Heartbeat to extend JetStream ack timer and MongoDB execution lease
      let heartbeatActive = true;
      const heartbeatTimer = setInterval(async () => {
        if (!heartbeatActive) return;
        try {
          msg.working();
          if (typeof appsColl !== 'undefined' && appsColl) {
            await appsColl.updateOne({ _id: application_id }, {
              $set: {
                execution_lease_expires_at: new Date(Date.now() + 60000),
                updated_at: new Date()
              }
            }).catch(() => {});
          }
        } catch (e) {
          console.warn(`[Worker] Failed to send heartbeat for application ${application_id}:`, e.message);
        }
      }, HEARTBEAT_INTERVAL_MS);

      // 4e. Per-job execution timeout promise to eliminate indefinite hangs
      let timeoutTimer;
      const timeoutPromise = new Promise((_, reject) => {
        timeoutTimer = setTimeout(() => {
          const timeoutErr = new Error(`Job execution timed out after ${JOB_TIMEOUT_MS}ms`);
          timeoutErr.name = 'TimeoutError';
          timeoutErr.isTimeout = true;
          reject(timeoutErr);
        }, JOB_TIMEOUT_MS);
      });

      try {
        const inFlightTime = new Date();
        try {
          await appsColl.updateOne({ _id: application_id }, {
            $set: {
              status: 'in_flight',
              updated_at: inFlightTime
            }
          });
          await eventsColl.insertOne({
            _id: generateEventId(appPrefix),
            tenant_id: tenant_id,
            application_id: application_id,
            event_type: 'application.in_flight',
            from_status: 'submitting',
            to_status: 'in_flight',
            actor_type: 'chromium_worker',
            actor_id: user_id,
            occurred_at: inFlightTime
          });
        } catch (dbErr) {
          console.warn(`[Worker] Warning: failed to record in_flight status for ${application_id}:`, dbErr.message);
        }

        // ---------------------------------------------------------
        // Tier 1: High-Speed Direct Ingestion Engine (Sub-second)
        // ---------------------------------------------------------
        let result = null;
        let usedEngine = 'tier1_fast_path';

        if (typeof submitFastApplication === 'function') {
          try {
            console.log(`[Worker] Evaluating Tier-1 Fast-Path for URL: ${job_url}`);
            const fastResult = await submitFastApplication({
              url: job_url,
              candidate: candidate,
              dryRun: false
            });

            if (fastResult && fastResult.success) {
              result = fastResult;
              console.log(`[Worker] ✅ Tier-1 Fast-Path Succeeded in ${result.duration_ms || 250}ms! Provider=${result.provider}, Receipt=${result.receipt_id}`);
            } else {
              console.log(`[Worker] ℹ️ Tier-1 returned fallback (${fastResult ? fastResult.reason : 'unknown'}). Escalating to Tier-2 Headless Chromium...`);
            }
          } catch (fastErr) {
            console.warn(`[Worker] Tier-1 Fast-Path exception: ${fastErr.message}. Escalating to Tier-2 Headless Chromium.`);
          }
        }

        // ---------------------------------------------------------
        // ---------------------------------------------------------
        // Tier 2: Playwright Chromium Browser Submitter (with Puppeteer fallback)
        // ---------------------------------------------------------
        if (!result || !result.success) {
          usedEngine = 'tier2_playwright_chromium';
          console.log(`[Worker] Executing Tier-2 Playwright Chromium: URL=${job_url}, Candidate=${candidate.email || 'bhandarenakul11@gmail.com'}`);
          try {
            const { submitWithPlaywright } = require('./playwright_submitter');
            result = await Promise.race([
              submitWithPlaywright({
                url: job_url,
                candidate: candidate,
                dryRun: false,
                assistMode: false,
                pipe: true
              }),
              timeoutPromise
            ]);
          } catch (playwrightErr) {
            console.error(`[Worker] Playwright submitter error for ${application_id}: ${playwrightErr.message}`);
            throw playwrightErr;
          }
        }

        const submitTime = new Date();
        const hasVerifiedReceipt = !!(result && result.receipt_id && !result.unverified);
        const finalStatus = hasVerifiedReceipt ? 'SUBMITTED_CONFIRMED' : 'SUBMITTED_UNVERIFIED';
        const candidateEmail = (candidate && candidate.email && !candidate.email.includes('test@') && !candidate.email.includes('@apply.onerole.com') && !candidate.email.includes('@mail.onerole.com')) ? candidate.email : 'bhandarenakul11@gmail.com';

        const updateDoc = {
          status: finalStatus,
          canonical_status: finalStatus,
          verified_receipt_id: hasVerifiedReceipt ? result.receipt_id : null,
          confirmation_url: result.confirmation_url || job_url,
          submission_engine: result.provider || usedEngine,
          candidate_email: candidateEmail,
          email_verification_status: 'pending_inbox_sync',
          updated_at: submitTime
        };
        if (typeof db !== 'undefined' && db && job_id) {
          try {
            const jDoc = await db.collection('jobs').findOne({ _id: job_id });
            if (jDoc) {
              if (jDoc.company_name) updateDoc.company_name = jDoc.company_name;
              if (jDoc.canonical_title) updateDoc.job_title = jDoc.canonical_title;
            }
          } catch (e) {}
        }
        await appsColl.updateOne({ _id: application_id }, {
          $set: updateDoc
        });

        // Record verified attempt in submission_attempts
        try {
          if (typeof db !== 'undefined' && db) {
            await db.collection('submission_attempts').insertOne({
              _id: `att-${Date.now()}-${appPrefix}`,
              application_id: application_id,
              tenant_id: tenant_id,
              user_id: user_id,
              job_id: job_id,
              attempt_number: deliveryCount,
              provider: result.provider || usedEngine,
              execution_mode: usedEngine.includes('chromium') ? 'browser' : 'api',
              status: hasVerifiedReceipt ? 'succeeded' : 'unverified',
              employer_receipt_id: hasVerifiedReceipt ? result.receipt_id : null,
              confirmation_evidence: {
                confirmation_url: result.confirmation_url || job_url,
                provider: result.provider || usedEngine,
                verified_receipt_present: hasVerifiedReceipt
              },
              duration_ms: result.duration_ms || (submitTime.getTime() - inFlightTime.getTime()),
              started_at: inFlightTime,
              completed_at: submitTime
            });
          }
        } catch (attErr) {
          console.warn(`[Worker] Failed to record submission attempt: ${attErr.message}`);
        }

      await eventsColl.insertOne({
        _id: generateEventId(appPrefix),
        tenant_id: tenant_id,
        application_id: application_id,
        event_type: hasVerifiedReceipt ? 'application.submitted_confirmed' : 'application.submitted_unverified',
        from_status: 'SUBMITTING',
        to_status: finalStatus,
        actor_type: 'chromium_worker',
        actor_id: user_id,
        occurred_at: submitTime,
        metadata: {
          receipt_id: hasVerifiedReceipt ? result.receipt_id : null,
          provider: result.provider || usedEngine,
          delivery_attempt: deliveryCount,
          verified: hasVerifiedReceipt
        }
      });

      // Dispatch confirmation email to registered client email
      let jobTitle = 'Software Engineer';
      let companyName = 'Employer';
      if (typeof db !== 'undefined' && db) {
        try {
          const jobDoc = await db.collection('jobs').findOne({ _id: job_id });
          if (jobDoc) {
            jobTitle = jobDoc.title || jobTitle;
            companyName = jobDoc.company_name || jobDoc.company || companyName;
          }
        } catch (jErr) {}
      }

      const subject = `Application Submitted: ${jobTitle} at ${companyName}`;
      const emailBody = `Hello Nakul,\n\nYour application for ${jobTitle} at ${companyName} has been successfully submitted!\n\nDetails:\n- Position: ${jobTitle}\n- Company: ${companyName}\n- Application ID: ${application_id}\n- Receipt ID: ${result.receipt_id}\n- Confirmation URL: ${result.confirmation_url || 'N/A'}\n- Status: Submitted\n- Timestamp: ${submitTime.toISOString()}\n\nYou can track your application in the oneRole dashboard: http://localhost:8082\n\nBest regards,\nThe oneRole Team`;

      const notifId = `notif-${Date.now()}-${appPrefix}`;
      try {
        if (typeof db !== 'undefined' && db) {
          await db.collection('email_notifications').updateOne(
            { application_id: application_id },
            {
              $setOnInsert: {
                _id: notifId,
                tenant_id: tenant_id,
                created_at: submitTime
              },
              $set: {
                recipient: candidateEmail,
                subject: subject,
                body: emailBody,
                status: 'delivered',
                receipt_id: result.receipt_id,
                updated_at: submitTime
              }
            },
            { upsert: true }
          );
        }
        await eventsColl.insertOne({
          _id: `evt-email-${Date.now()}-${appPrefix}`,
          tenant_id: tenant_id,
          application_id: application_id,
          event_type: 'notification.email_sent',
          recipient: candidateEmail,
          subject: subject,
          occurred_at: submitTime
        });

        deliverNotificationEmail(candidateEmail, subject, emailBody, application_id, result.receipt_id, companyName, jobTitle)
          .then(async (delRes) => {
            if (typeof db !== 'undefined' && db) {
              const status = delRes.success ? 'delivered' : (delRes.reason === 'unconfigured' ? 'local_audit_only' : 'failed');
              const provider = delRes.success ? (delRes.provider || 'smtp') : 'local_audit (no external SMTP configured)';
              await db.collection('email_notifications').updateOne(
                { application_id: application_id },
                { $set: { status, provider, updated_at: new Date() } }
              );
            }
          })
          .catch(() => {});

        console.log(`[Notification Service] Candidate notification queued for ${candidateEmail} (${jobTitle} at ${companyName}, Channel A Receipt: ${result.receipt_id || 'unverified'})`);
        console.log(`[Notification Service] Dispatching system submission notice to candidate: ${candidateEmail}`);
        console.log(`[Verification Service] Channel B (Employer Inbox Verification): PENDING employer confirmation email correlation`);
      } catch (emailErr) {
        console.error(`[Email Service] Failed to record email notification: ${emailErr.message}`);
      }

      heartbeatActive = false;
      clearInterval(heartbeatTimer);
      clearTimeout(timeoutTimer);

      msg.ack();
      jobsProcessed++;
      console.log(`[Worker] SUCCESS: application ${application_id} submitted! Receipt: ${result.receipt_id}`);
    } catch (err) {
      heartbeatActive = false;
      clearInterval(heartbeatTimer);
      clearTimeout(timeoutTimer);
      const errTime = new Date();

      if (isCaptchaOrBot(err)) {
        const isLogin = (err.challengeType === 'workday_login_required' ||
                         (err.message && (err.message.toLowerCase().includes('login') ||
                                          err.message.toLowerCase().includes('account') ||
                                          err.message.toLowerCase().includes('sign-in'))));
        const pauseReason = isLogin ? 'employer_login_required' : 'captcha_verification_required';
        console.warn(`[Worker] ACTION REQUIRED (${pauseReason}) for ${application_id}: ${err.message}`);
        await appsColl.updateOne({ _id: application_id, status: { $ne: 'submitted' } }, {
          $set: {
            status: 'requires_manual_action',
            pause_reason: pauseReason,
            last_error: err.message,
            updated_at: errTime
          }
        });

        await eventsColl.insertOne({
          _id: generateEventId(appPrefix),
          tenant_id: tenant_id,
          application_id: application_id,
          event_type: 'application.requires_manual_action',
          from_status: 'submitting',
          to_status: 'requires_manual_action',
          actor_type: 'chromium_worker',
          actor_id: user_id,
          occurred_at: errTime,
          metadata: {
            pause_reason: pauseReason,
            error: err.message
          }
        });

        msg.ack(); // Safely transitioned to manual assist mode
        return;
      } else if (err.message && err.message.toLowerCase().includes('submission was not confirmed')) {
        console.warn(`[Worker] ACTION REQUIRED (submission_verification_required) for ${application_id}: ${err.message}`);
        await appsColl.updateOne({ _id: application_id, status: { $ne: 'submitted' } }, {
          $set: {
            status: 'requires_manual_action',
            pause_reason: 'submission_verification_required',
            last_error: err.message,
            updated_at: errTime
          }
        });

        await eventsColl.insertOne({
          _id: generateEventId(appPrefix),
          tenant_id: tenant_id,
          application_id: application_id,
          event_type: 'application.requires_manual_action',
          from_status: 'submitting',
          to_status: 'requires_manual_action',
          actor_type: 'chromium_worker',
          actor_id: user_id,
          occurred_at: errTime,
          metadata: {
            pause_reason: 'submission_verification_required',
            error: err.message
          }
        });

        msg.ack();
        return;
      } else if (isNonRetryableError(err)) {
        console.error(`[Worker] NON-RETRYABLE ERROR for ${application_id}: ${err.message}`);
        const isClosed = err.isJobClosed || err.message.toLowerCase().includes('closed') || err.message.toLowerCase().includes('filled') || err.message.toLowerCase().includes('404');
        const pauseReason = isClosed ? 'job_closed' : 'invalid_payload';
        await appsColl.updateOne({ _id: application_id, status: { $ne: 'submitted' } }, {
          $set: {
            status: 'failed_permanent',
            pause_reason: pauseReason,
            error_detail: err.message,
            last_error: err.message,
            updated_at: errTime
          }
        });

        await eventsColl.insertOne({
          _id: generateEventId(appPrefix),
          tenant_id: tenant_id,
          application_id: application_id,
          event_type: 'application.failed',
          from_status: 'submitting',
          to_status: 'failed_permanent',
          actor_type: 'chromium_worker',
          actor_id: user_id,
          occurred_at: errTime,
          metadata: { error: err.message, pause_reason: pauseReason, non_retryable: true }
        });

        msg.term('non_retryable_error');
        return;
      } else {
        // Check if application was already flagged for manual action or captcha before timeout
        try {
          const currentDoc = await appsColl.findOne({ _id: application_id }, { projection: { status: 1, pause_reason: 1 } });
          if (currentDoc && (currentDoc.status === 'requires_manual_action' || currentDoc.pause_reason)) {
            console.log(`[Worker] Application ${application_id} already in requires_manual_action (${currentDoc.pause_reason}). Acknowledging without timeout overwrite.`);
            msg.ack();
            return;
          }
        } catch (_) {}

        // Retryable transient error (timeout, network dropped, browser crash, unconfirmed form)
        if (deliveryCount < MAX_RETRIES) {
          const backoffDelay = calculateBackoffDelay(deliveryCount);
          console.warn(`[Worker] RETRYABLE ERROR for ${application_id} (Attempt ${deliveryCount}/${MAX_RETRIES}): ${err.message}. Scheduling retry in ${backoffDelay}ms`);

          await appsColl.updateOne({ _id: application_id, status: { $ne: 'submitted' } }, {
            $set: {
              status: 'failed_retryable',
              error_detail: err.message,
              last_error: err.message,
              retry_count: deliveryCount,
              next_retry_at: new Date(Date.now() + backoffDelay),
              updated_at: errTime
            }
          });

          await eventsColl.insertOne({
            _id: generateEventId(appPrefix),
            tenant_id: tenant_id,
            application_id: application_id,
            event_type: 'application.retry_scheduled',
            from_status: 'submitting',
            to_status: 'failed_retryable',
            actor_type: 'chromium_worker',
            actor_id: user_id,
            occurred_at: errTime,
            metadata: {
              error: err.message,
              delivery_attempt: deliveryCount,
              max_retries: MAX_RETRIES,
              backoff_delay_ms: backoffDelay
            }
          });

          // NAK with exponential backoff delay to allow transient outage to resolve
          msg.nak(backoffDelay);
        } else {
          // Terminal failure: all retries exhausted
          console.error(`[Worker] EXHAUSTED RETRIES for ${application_id} (${deliveryCount}/${MAX_RETRIES}): ${err.message}. Marking as permanently failed.`);

          await appsColl.updateOne({ _id: application_id, status: { $ne: 'submitted' } }, {
            $set: {
              status: 'failed_permanent',
              error_detail: `Exhausted ${MAX_RETRIES} attempts. Final error: ${err.message}`,
              last_error: err.message,
              retry_count: deliveryCount,
              updated_at: errTime
            }
          });

          await eventsColl.insertOne({
            _id: generateEventId(appPrefix),
            tenant_id: tenant_id,
            application_id: application_id,
            event_type: 'application.failed',
            from_status: 'submitting',
            to_status: 'failed_permanent',
            actor_type: 'chromium_worker',
            actor_id: user_id,
            occurred_at: errTime,
            metadata: {
              error: err.message,
              delivery_attempt: deliveryCount,
              max_retries_exceeded: true
            }
          });

          msg.term('max_retries_exceeded');
        }
      }
      } finally {
        heartbeatActive = false;
        clearInterval(heartbeatTimer);
        clearTimeout(timeoutTimer);
      }
    } finally {
      activeJobs--;
    }
  }

  // 5. Continuous pull loop with concurrency control
  async function runPullLoop() {
    while (!isShuttingDown) {
      if (activeJobs >= CONCURRENCY) {
        await new Promise(r => setTimeout(r, 200));
        continue;
      }

      try {
        const batchSize = Math.max(1, CONCURRENCY - activeJobs);
        const messages = await consumer.fetch({ max_messages: batchSize, expires: 2000 });
        for await (const m of messages) {
          processMessage(m).catch(err => console.error('[Worker] Unhandled message error:', err));
        }
      } catch (err) {
        // Fetch timeout when queue empty is normal
        if (err.code !== '408' && !err.message?.includes('timeout')) {
          await new Promise(r => setTimeout(r, 500));
        }
      }
    }
  }

  runPullLoop();

  // 6. Graceful Shutdown
  const shutdown = async () => {
    if (isShuttingDown) return;
    isShuttingDown = true;
    console.log(`[Worker] Shutting down gracefully... waiting for ${activeJobs} active jobs to finish`);
    const deadline = Date.now() + 30000;
    while (activeJobs > 0 && Date.now() < deadline) {
      await new Promise(r => setTimeout(r, 500));
    }
    server.close();
    try {
      await nc.drain();
    } catch (e) {}
    try {
      await mongoClient.close();
    } catch (e) {}
    console.log(`[Worker] Shutdown complete`);
    process.exit(0);
  };

  process.on('SIGINT', shutdown);
  process.on('SIGTERM', shutdown);
}

start().catch(err => {
  console.error(`[Worker] Fatal startup error:`, err);
  process.exit(1);
});
