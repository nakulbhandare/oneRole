/**
 * OneRole Custom Opaque Assertion Helpers
 * File: tests/e2e/helpers/assertions.js
 */

const assert = require('assert');
const fs = require('fs');

function assertCandidateProfile(cand, msg = '') {
  assert.ok(cand, `Candidate payload must not be null: ${msg}`);
  const fullName = cand.name || `${cand.first_name || ''} ${cand.last_name || ''}`.trim();
  assert.strictEqual(fullName, 'Nakul Bhandare', `Candidate name must be Nakul Bhandare, got '${fullName}': ${msg}`);
  assert.strictEqual(cand.email, 'bhandarenakul11@gmail.com', `Candidate email must be bhandarenakul11@gmail.com: ${msg}`);
  assert.strictEqual(cand.phone, '+1 (350)-355-6570', `Candidate phone must match +1 (350)-355-6570: ${msg}`);
  const loc = cand.location || `${cand.city || ''}, ${cand.state || ''}`.trim();
  assert.ok(loc && loc.includes('San Francisco'), `Candidate location must contain San Francisco: ${msg}`);
}

function assertResumePdfIntegrity(filePathOrBuffer, msg = '') {
  let buf;
  if (typeof filePathOrBuffer === 'string') {
    assert.ok(fs.existsSync(filePathOrBuffer), `Resume file must exist at path ${filePathOrBuffer}: ${msg}`);
    buf = fs.readFileSync(filePathOrBuffer);
  } else {
    buf = filePathOrBuffer;
  }

  assert.ok(buf && buf.length > 1024, `Resume PDF buffer must be > 1KB (got ${buf ? buf.length : 0} bytes): ${msg}`);
  const header = buf.slice(0, 5).toString('ascii');
  assert.strictEqual(header, '%PDF-', `Resume file must start with '%PDF-' magic bytes: ${msg}`);
}

function assertTopRowSorted(applications, msg = '') {
  assert.ok(Array.isArray(applications), `Applications must be an array: ${msg}`);
  assert.ok(applications.length > 0, `Applications must not be empty: ${msg}`);

  // Ensure timestamps are monotonically non-increasing (descending)
  for (let i = 0; i < applications.length - 1; i++) {
    const a = applications[i];
    const b = applications[i + 1];

    const aTop = (a.status === 'in_flight' || a.applied === 'Just now');
    const bTop = (b.status === 'in_flight' || b.applied === 'Just now');

    if (aTop && !bTop) continue;
    if (!aTop && bTop) {
      assert.fail(`Row ${i} is not top-row prioritized over in-flight item at Row ${i + 1}: ${msg}`);
    }

    const tA = new Date(a.submitted_at || a.created_at || a.updated_at || 0).getTime();
    const tB = new Date(b.submitted_at || b.created_at || b.updated_at || 0).getTime();
    if (!isNaN(tA) && !isNaN(tB)) {
      assert.ok(tA >= tB, `Tracking table sorting violation: Row ${i} (${tA}) is older than Row ${i + 1} (${tB}): ${msg}`);
    }
  }
}

function assertQueueDeduplicated(queueCards, msg = '') {
  assert.strictEqual(queueCards.length, 5, `Active match card queue must contain exactly 5 cards, got ${queueCards.length}: ${msg}`);
  const companies = queueCards.map(c => (c.company || '').toLowerCase().trim());
  const uniqueCompanies = new Set(companies);
  assert.strictEqual(uniqueCompanies.size, 5, `Queue contains duplicate companies: [${companies.join(', ')}]: ${msg}`);
}

async function assertMongoApplicationLifecycle(db, applicationId, expectedStatus = 'submitted') {
  const appDoc = await db.collection('applications').findOne({ _id: applicationId });
  assert.ok(appDoc, `Application document ${applicationId} must exist in MongoDB`);
  assert.strictEqual(appDoc.status, expectedStatus, `Application status must be '${expectedStatus}', got '${appDoc.status}'`);

  if (expectedStatus === 'submitted') {
    assert.ok(appDoc.verified_receipt_id, `Submitted application ${applicationId} must have verified_receipt_id`);
  }

  const events = await db.collection('application_events').find({ application_id: applicationId }).toArray();
  assert.ok(events.length >= 1, `Application ${applicationId} must have lifecycle events (found ${events.length})`);
  const eventTypes = events.map(e => e.event_type);
  assert.ok(
    eventTypes.includes('application.enqueued') || eventTypes.includes('application.submitting') || eventTypes.includes('application.submitted'),
    `Missing initial dispatch event in lifecycle: [${eventTypes.join(', ')}]`
  );
  if (expectedStatus === 'submitted') {
    assert.ok(eventTypes.includes('application.submitted'), `Missing application.submitted event in lifecycle: [${eventTypes.join(', ')}]`);
  }
}

async function assertEmailNotificationDispatched(db, applicationId, expectedRecipient = 'bhandarenakul11@gmail.com') {
  const notif = await db.collection('email_notifications').findOne({ application_id: applicationId });
  assert.ok(notif, `Email notification record must exist in MongoDB for application ${applicationId}`);
  assert.strictEqual(notif.recipient, expectedRecipient, `Email recipient must be ${expectedRecipient}`);
  assert.strictEqual(notif.status, 'delivered', `Email status must be 'delivered'`);
  assert.ok(notif.body, 'Email body must not be empty');
}

function assertPronounHygiene(submittedData, msg = '') {
  assert.ok(
    !submittedData.hasPronounConflict,
    `Indiscriminate checkbox ticking defect detected: multiple conflicting pronouns checked: ${msg}`
  );
}

function assertErrorClassification(error, expectedType = 'retryable') {
  const isRetryable = !error.message.includes('missing required field') &&
                      !error.message.includes('job is no longer available') &&
                      !error.message.includes('404 not found');
  if (expectedType === 'retryable') {
    assert.ok(isRetryable, `Error '${error.message}' should be classified as RETRYABLE`);
  } else {
    assert.ok(!isRetryable, `Error '${error.message}' should be classified as NON-RETRYABLE`);
  }
}

function assertLatency(durationMs, maxThresholdMs, metricName = 'Operation') {
  assert.strictEqual(typeof durationMs, 'number', `durationMs must be a number for ${metricName}, got ${typeof durationMs}`);
  assert.strictEqual(typeof maxThresholdMs, 'number', `maxThresholdMs must be a number for ${metricName}, got ${typeof maxThresholdMs}`);
  assert.ok(
    durationMs <= maxThresholdMs,
    `Latency benchmark exceeded for ${metricName}: duration ${durationMs}ms > max threshold ${maxThresholdMs}ms`
  );
}

function assertZeroBotShield(applicationDocOrEvents, contextMsg = '') {
  assert.ok(applicationDocOrEvents, `Target document or events must not be null/undefined: ${contextMsg}`);

  const items = Array.isArray(applicationDocOrEvents) ? applicationDocOrEvents : [applicationDocOrEvents];

  for (const item of items) {
    if (!item) continue;

    // Check status
    if (typeof item.status !== 'undefined') {
      assert.notStrictEqual(
        item.status,
        'requires_manual_action',
        `Bot Shield violation: status is 'requires_manual_action' (Action Required): ${contextMsg}`
      );
      assert.notStrictEqual(
        item.status,
        'ACTION_REQUIRED',
        `Bot Shield violation: status is 'ACTION_REQUIRED': ${contextMsg}`
      );
    }

    // Check pause_reason
    if (typeof item.pause_reason !== 'undefined') {
      assert.notStrictEqual(
        item.pause_reason,
        'captcha_verification_required',
        `Bot Shield violation: pause_reason is 'captcha_verification_required': ${contextMsg}`
      );
    }

    // Check boolean flags if present
    if (typeof item.bot_shield_triggered !== 'undefined') {
      assert.strictEqual(
        item.bot_shield_triggered,
        false,
        `Bot Shield violation: bot_shield_triggered is true: ${contextMsg}`
      );
    }
    if (typeof item.BotShieldTriggered !== 'undefined') {
      assert.strictEqual(
        item.BotShieldTriggered,
        false,
        `Bot Shield violation: BotShieldTriggered is true: ${contextMsg}`
      );
    }

    // Check candidate email
    const candidateEmail = item.candidate_email ||
      (item.candidate && (item.candidate.email || item.candidate.primaryEmail)) ||
      item.email ||
      item.recipient ||
      item.target_email ||
      item.TargetEmail;

    if (candidateEmail) {
      assert.strictEqual(
        candidateEmail,
        'bhandarenakul11@gmail.com',
        `Bot Shield candidate email mismatch: expected 'bhandarenakul11@gmail.com', got '${candidateEmail}': ${contextMsg}`
      );
    }
  }
}

module.exports = {
  assertCandidateProfile,
  assertResumePdfIntegrity,
  assertTopRowSorted,
  assertQueueDeduplicated,
  assertMongoApplicationLifecycle,
  assertEmailNotificationDispatched,
  assertPronounHygiene,
  assertErrorClassification,
  assertLatency,
  assertZeroBotShield
};
