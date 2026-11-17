/**
 * OneRole E2E Test Fixtures, Setup, and Environment Manager
 * File: tests/e2e/helpers/setup.js
 */

const path = require('path');
const fs = require('fs');

// Resolve dependencies from root and connectors/ats_submit
const atsDir = path.resolve(__dirname, '../../../connectors/ats_submit/node_modules');
const { MongoClient } = require(path.join(atsDir, 'mongodb'));
const { connect, StringCodec } = require(path.join(atsDir, 'nats'));
const puppeteer = require('puppeteer-core');

const ENV = {
  BACKEND_URL: process.env.BACKEND_URL || 'http://localhost:8088/api/v1',
  UI_URL: process.env.UI_URL || 'http://localhost:8082',
  WORKER_URL: process.env.WORKER_URL || 'http://localhost:3001',
  MONGO_URL: process.env.DATABASE_URL || 'mongodb://localhost:27017/onerole',
  NATS_URL: process.env.NATS_URL || 'nats://localhost:4222',
  MOCK_ATS_PORT: parseInt(process.env.MOCK_ATS_PORT || '9876', 10),
  MOCK_ATS_URL: `http://localhost:${process.env.MOCK_ATS_PORT || '9876'}`,
  CHROME_PATH: process.env.CHROME_PATH ||
               (fs.existsSync('/Applications/Google Chrome.app/Contents/MacOS/Google Chrome')
                 ? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
                 : '/usr/bin/google-chrome')
};

// Exact Candidate Profile matching R2
const CANDIDATE_PROFILE = {
  first_name: 'Nakul',
  last_name: 'Bhandare',
  name: 'Nakul Bhandare',
  email: 'bhandarenakul11@gmail.com',
  phone: '+1 (350)-355-6570',
  location: 'San Francisco, CA',
  resume_file_path: path.resolve(__dirname, '../../../oneRole_UI/public/Nakul_Bhandare_Resume.pdf'),
  resume_filename: 'Nakul_Bhandare_Resume.pdf',
  linkedin: 'https://linkedin.com/in/nakul-bhandare',
  github: 'https://github.com/nakulbhandare'
};

let cachedMongoClient = null;
let cachedNatsClient = null;
const activeBrowsers = new Set();

async function getDb() {
  if (!cachedMongoClient) {
    cachedMongoClient = new MongoClient(ENV.MONGO_URL);
    await cachedMongoClient.connect();
  }
  return cachedMongoClient.db();
}

async function getNats() {
  if (!cachedNatsClient) {
    cachedNatsClient = await connect({ servers: ENV.NATS_URL });
  }
  return cachedNatsClient;
}

async function launchBrowser(options = {}) {
  const browser = await puppeteer.launch({
    executablePath: ENV.CHROME_PATH,
    headless: options.headless !== undefined ? options.headless : true,
    pipe: options.pipe !== undefined ? options.pipe : true,
    args: [
      '--no-sandbox',
      '--disable-setuid-sandbox',
      '--disable-dev-shm-usage',
      '--disable-gpu',
      '--disable-blink-features=AutomationControlled',
      '--no-first-run',
      ...(options.args || [])
    ]
  });
  activeBrowsers.add(browser);
  return browser;
}

async function createTestPage(browser) {
  const page = await browser.newPage();
  const consoleErrors = [];
  const pageErrors = [];

  page.on('pageerror', err => pageErrors.push(err.message));
  page.on('console', msg => {
    if (msg.type() === 'error') {
      const txt = msg.text();
      if (!txt.includes('favicon') && !txt.includes('ERR_FILE_NOT_FOUND')) {
        consoleErrors.push(txt);
      }
    }
  });

  return {
    page,
    errors: () => ({ consoleErrors, pageErrors })
  };
}

async function cleanDatabaseFixtures(prefix = 'e2e-') {
  try {
    const db = await getDb();
    await db.collection('applications').deleteMany({
      $or: [
        { _id: { $regex: `^${prefix}` } },
        { 'candidate.email': 'e2e-test@onerole.dev' }
      ]
    });
    await db.collection('application_events').deleteMany({
      application_id: { $regex: `^${prefix}` }
    });
    await db.collection('email_notifications').deleteMany({
      application_id: { $regex: `^${prefix}` }
    });
  } catch (e) {}
}

async function healthCheck() {
  const status = {
    mongodb: false,
    nats: false,
    backend: false,
    ui: false,
    worker: false,
    chrome: fs.existsSync(ENV.CHROME_PATH)
  };

  try {
    const db = await getDb();
    await db.command({ ping: 1 });
    status.mongodb = true;
  } catch (e) {}

  try {
    const nc = await getNats();
    status.nats = !nc.isClosed();
  } catch (e) {}

  try {
    const res = await fetch(`${ENV.BACKEND_URL}/healthz`, { signal: AbortSignal.timeout(3000) });
    status.backend = res.ok;
  } catch (e) {}

  try {
    const res = await fetch(`${ENV.UI_URL}/healthz`, { signal: AbortSignal.timeout(3000) });
    status.ui = res.ok;
  } catch (e) {}

  try {
    const res = await fetch(`${ENV.WORKER_URL}/healthz`, { signal: AbortSignal.timeout(3000) });
    status.worker = res.ok;
  } catch (e) {}

  return status;
}

async function teardown() {
  for (const b of activeBrowsers) {
    try { await b.close(); } catch (e) {}
  }
  activeBrowsers.clear();

  if (cachedMongoClient) {
    try { await cachedMongoClient.close(); } catch (e) {}
    cachedMongoClient = null;
  }
  if (cachedNatsClient) {
    try { await cachedNatsClient.close(); } catch (e) {}
    cachedNatsClient = null;
  }
}

module.exports = {
  ENV,
  CANDIDATE_PROFILE,
  getDb,
  getNats,
  launchBrowser,
  createTestPage,
  cleanDatabaseFixtures,
  healthCheck,
  teardown,
  MongoClient,
  connect,
  StringCodec,
  puppeteer
};
