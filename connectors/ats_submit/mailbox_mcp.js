/**
 * OneRole Mailbox MCP Engine
 * Direct, atomic IMAP + NATS Push OTP resolution engine with zero race conditions.
 * Credentials: bhandarenakul11@gmail.com (App Password)
 */

const tls = require('tls');

const GMAIL_USER = process.env.CANDIDATE_EMAIL || 'bhandarenakul11@gmail.com';
const GMAIL_APP_PASS = process.env.GMAIL_APP_PASSWORD || 'qmwe tbfj lqtc yhhw';
const IMAP_HOST = 'imap.gmail.com';
const IMAP_PORT = 993;

// Ring buffer of recently received OTPs (60s TTL)
const OTP_RING_BUFFER = [];
const BUFFER_TTL_MS = 60000;

function pruneRingBuffer() {
  const now = Date.now();
  while (OTP_RING_BUFFER.length > 0 && (now - OTP_RING_BUFFER[0].timestamp) > BUFFER_TTL_MS) {
    OTP_RING_BUFFER.shift();
  }
}

/**
 * Push an incoming OTP (from NATS or webhook) into the in-memory ring buffer
 */
function recordIncomingOtp(payload) {
  if (!payload || !payload.code) return;
  pruneRingBuffer();
  OTP_RING_BUFFER.push({
    code: String(payload.code).trim(),
    email: (payload.email || '').toLowerCase().trim(),
    domain: (payload.domain || '').toLowerCase().trim(),
    company: (payload.company || '').toLowerCase().trim(),
    subject: (payload.subject || '').toLowerCase().trim(),
    timestamp: payload.timestamp || Date.now()
  });
  console.log(`[Mailbox MCP] Buffered OTP ${payload.code} for ${payload.email} (buffer size: ${OTP_RING_BUFFER.length})`);
}

/**
 * Check ring buffer for a matching OTP arriving after sinceTimestamp
 */
function findBufferedOtp(email, domain, company, sinceTimestamp = 0) {
  pruneRingBuffer();
  const searchEmail = (email || '').toLowerCase().trim();
  const searchDomain = (domain || '').toLowerCase().trim();
  const searchCompany = (company || '').toLowerCase().trim();
  const minTime = sinceTimestamp > 0 ? (sinceTimestamp - 10000) : (Date.now() - 60000);

  // Search in reverse (newest first)
  for (let i = OTP_RING_BUFFER.length - 1; i >= 0; i--) {
    const item = OTP_RING_BUFFER[i];
    if (item.timestamp < minTime) continue;

    const emailMatch = !searchEmail || item.email.includes(searchEmail) || searchEmail.includes(item.email);
    const domainMatch = !searchDomain || item.domain.includes(searchDomain) || searchDomain.includes(item.domain) || item.subject.includes(searchDomain);
    const compMatch = !searchCompany || item.company.includes(searchCompany) || searchCompany.includes(item.company) || item.subject.includes(searchCompany);

    if (emailMatch && (domainMatch || compMatch || !searchDomain)) {
      console.log(`[Mailbox MCP] Instant buffer hit! Found OTP ${item.code} (age: ${Date.now() - item.timestamp}ms)`);
      return item.code;
    }
  }
  return null;
}

/**
 * Fetch latest OTP directly from Gmail IMAP over TLS
 */
function fetchLatestOtpFromGmail(sinceTimestamp = 0, targetCompany = '') {
  return new Promise((resolve, reject) => {
    const minTime = sinceTimestamp > 0 ? (sinceTimestamp - 15000) : (Date.now() - 60000);
    const socket = tls.connect(IMAP_PORT, IMAP_HOST, { rejectUnauthorized: false }, () => {
      socket.write(`A01 LOGIN ${GMAIL_USER} "${GMAIL_APP_PASS}"\r\n`);
    });

    let buffer = '';
    let foundCode = null;
    let step = 'LOGIN';

    const timer = setTimeout(() => {
      try { socket.end(); } catch (_) {}
      resolve(null);
    }, 12000);

    socket.on('data', data => {
      buffer += data.toString();

      if (step === 'LOGIN' && buffer.includes('A01 OK')) {
        buffer = '';
        step = 'SELECT';
        socket.write('A02 SELECT INBOX\r\n');
      } else if (step === 'SELECT' && buffer.includes('A02 OK')) {
        buffer = '';
        step = 'SEARCH';
        // Search recent greenhouse or verification messages
        socket.write('A03 SEARCH FROM "greenhouse" ALL\r\n');
      } else if (step === 'SEARCH' && buffer.includes('A03 OK')) {
        const match = buffer.match(/\* SEARCH\s+([\d\s]+)/);
        buffer = '';
        if (match && match[1]) {
          const ids = match[1].trim().split(/\s+/).map(Number).filter(n => !isNaN(n));
          if (ids.length > 0) {
            // Take up to the last 3 message IDs
            const latestIds = ids.slice(-3);
            step = 'FETCH';
            socket.write(`A04 FETCH ${latestIds.join(',')} (BODY[HEADER.FIELDS (DATE SUBJECT FROM)] BODY[TEXT])\r\n`);
            return;
          }
        }
        // No messages found
        step = 'LOGOUT';
        socket.write('A05 LOGOUT\r\n');
        socket.end();
        clearTimeout(timer);
        resolve(null);
      } else if (step === 'FETCH' && buffer.includes('A04 OK')) {
        clearTimeout(timer);
        const fetchOutput = buffer;
        buffer = '';
        step = 'LOGOUT';
        try { socket.write('A05 LOGOUT\r\n'); socket.end(); } catch (_) {}

        // Parse individual FETCH responses
        const msgChunks = fetchOutput.split(/\* \d+ FETCH/);
        for (let c = msgChunks.length - 1; c >= 1; c--) {
          const chunk = msgChunks[c];
          
          // Check Date header
          let msgTime = 0;
          const dateMatch = chunk.match(/Date:\s*([^\r\n]+)/i);
          if (dateMatch) {
            const parsed = Date.parse(dateMatch[1]);
            if (!isNaN(parsed)) msgTime = parsed;
          }

          // If message is older than submission window, skip
          if (msgTime > 0 && msgTime < minTime) {
            continue;
          }

          // Extract 6 to 8 digit code
          const h1Match = chunk.match(/<h1[^>]*>\s*(\d{6,8})\s*<\/h1>/i);
          const strongMatch = chunk.match(/<strong>\s*(\d{6,8})\s*<\/strong>/i);
          const copyPasteMatch = chunk.match(/(?:security code|verification code|paste this code)[^:]*:\s*(?:<\/p>\s*<[^>]+>)?\s*(\d{6,8})/i);
          const rawMatch = chunk.match(/\b([0-9]{8})\b/);

          const candidateCode = (h1Match && h1Match[1]) ||
                                (strongMatch && strongMatch[1]) ||
                                (copyPasteMatch && copyPasteMatch[1]) ||
                                (rawMatch && rawMatch[1]);

          if (candidateCode) {
            foundCode = candidateCode;
            console.log(`[Mailbox MCP] Extracted OTP code ${foundCode} from Gmail message (msgTime=${msgTime ? new Date(msgTime).toISOString() : 'unknown'})`);
            break;
          }
        }
        resolve(foundCode);
      }
    });

    socket.on('error', err => {
      console.warn(`[Mailbox MCP] IMAP socket error: ${err.message}`);
      clearTimeout(timer);
      resolve(null);
    });
  });
}

/**
 * Unified OTP Resolver: combines Ring Buffer, NATS Push, and IMAP Fallback
 */
async function resolveOtpWithFailover({ email, domain, company, sinceTimestamp = 0, timeoutMs = 45000, natsWaiter = null }) {
  console.log(`[Mailbox MCP] Resolving OTP for email=${email} domain=${domain} company=${company} (since=${new Date(sinceTimestamp || Date.now()).toISOString()})...`);

  // 1. Instant check in Ring Buffer
  const cached = findBufferedOtp(email, domain, company, sinceTimestamp);
  if (cached) {
    return cached;
  }

  // 2. Race between NATS Push waiter and periodic on-demand IMAP poll
  const startTime = Date.now();
  let resolved = false;

  return new Promise((resolve) => {
    const endTimeout = setTimeout(() => {
      resolved = true;
      resolve(null);
    }, timeoutMs);

    // Branch A: NATS waiter
    if (typeof natsWaiter === 'function') {
      natsWaiter(email, domain, company, timeoutMs).then(code => {
        if (!resolved && code) {
          resolved = true;
          clearTimeout(endTimeout);
          console.log(`[Mailbox MCP] Resolved via NATS Push stream: ${code}`);
          resolve(code);
        }
      }).catch(() => {});
    }

    // Branch B: Polling Ring Buffer and on-demand Gmail IMAP
    (async () => {
      // Small initial delay to allow Greenhouse delivery (1.5-2s)
      await new Promise(r => setTimeout(r, 1200));

      while (!resolved && (Date.now() - startTime) < timeoutMs) {
        // Check buffer first (updated by background NATS push)
        const inBuf = findBufferedOtp(email, domain, company, sinceTimestamp);
        if (inBuf) {
          resolved = true;
          clearTimeout(endTimeout);
          console.log(`[Mailbox MCP] Resolved via buffered push: ${inBuf}`);
          return resolve(inBuf);
        }

        // On-demand Gmail IMAP fetch
        try {
          const directCode = await fetchLatestOtpFromGmail(sinceTimestamp, company);
          if (directCode && !resolved) {
            resolved = true;
            clearTimeout(endTimeout);
            console.log(`[Mailbox MCP] Resolved directly via Gmail IMAP: ${directCode}`);
            recordIncomingOtp({ code: directCode, email, domain, company, timestamp: Date.now() });
            return resolve(directCode);
          }
        } catch (_) {}

        await new Promise(r => setTimeout(r, 2000));
      }
    })();
  });
}

module.exports = {
  recordIncomingOtp,
  findBufferedOtp,
  fetchLatestOtpFromGmail,
  resolveOtpWithFailover
};
