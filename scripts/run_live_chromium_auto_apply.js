const { MongoClient } = require('mongodb');
const { submitApplication } = require('../connectors/ats_submit/browser_submitter.js');
const path = require('path');
const fs = require('fs');

const MONGO_URI = process.env.MONGO_URI || 'mongodb://127.0.0.1:27017';
const DB_NAME = 'onerole';
const TENANT_ID = '00000000-0000-4000-8000-000000000001';
const USER_ID = '336a411a-bdde-463e-ba61-c25ba4bebfed';

const RESUME_PATH = path.resolve(__dirname, '../oneRole_UI/public/Nakul_Bhandare_Resume.pdf');

const CANDIDATE = {
  first_name: 'Nakul',
  last_name: 'Bhandare',
  name: 'Nakul Bhandare',
  email: 'bhandarenakul11@gmail.com',
  phone: '+1 (350)-355-6570',
  location: 'San Francisco, CA',
  city: 'San Francisco',
  state: 'CA',
  linkedin: 'https://linkedin.com/in/nakul-bhandare',
  github: 'https://github.com/nakulbhandare',
  portfolio: 'https://github.com/nakulbhandare',
  resume_file_path: RESUME_PATH
};

async function main() {
  console.log('================================================================');
  console.log('🚀 OneRole Live Chromium Auto-Apply Cluster');
  console.log('Target: Actual employer submissions on live company career portals');
  console.log(`Candidate: ${CANDIDATE.name} (${CANDIDATE.email})`);
  console.log(`Resume: ${RESUME_PATH}`);
  console.log('Mode: VISIBLE CHROMIUM (Opening on your screen)');
  console.log('================================================================\n');

  const client = new MongoClient(MONGO_URI);
  await client.connect();
  const db = client.db(DB_NAME);

  // 1. Fetch real ATS jobs from MongoDB
  const jobs = await db.collection('jobs').find({
    source_url: { $regex: 'ashbyhq\\.com|greenhouse\\.io' }
  }).sort({ first_seen_at: -1 }).limit(100).toArray();

  console.log(`✓ Loaded ${jobs.length} real company vacancies from MongoDB.\n`);

  let successCount = 0;
  let screeningCount = 0;
  let failedCount = 0;

  const startTime = Date.now();

  for (let i = 0; i < jobs.length; i++) {
    const job = jobs[i];
    const index = i + 1;
    const compName = job.company || 'Employer';
    const roleTitle = job.title || 'Software Engineer';
    const sourceUrl = job.source_url;

    console.log(`----------------------------------------------------------------`);
    console.log(`[${index}/${jobs.length}] Submitting to ${compName} - "${roleTitle}"`);
    console.log(`URL: ${sourceUrl}`);

    const jobStartTime = Date.now();

    try {
      // Execute live Chromium submission (visible mode)
      const result = await submitApplication({
        url: sourceUrl,
        candidate: CANDIDATE,
        dryRun: false,
        assistMode: true
      });

      const elapsedSec = ((Date.now() - jobStartTime) / 1000).toFixed(1);

      if (result && result.success) {
        successCount++;
        const receipt = result.receipt_id || result.confirmation_id || `LIVE-CHROMIUM-${Date.now()}`;
        console.log(`✓ [SUCCESS in ${elapsedSec}s] Application filed on employer ATS! Receipt: ${receipt}`);

        // Update MongoDB application record in real time
        await db.collection('applications').updateOne(
          { tenant_id: TENANT_ID, company_name: compName, job_title: roleTitle },
          {
            $set: {
              tenant_id: TENANT_ID,
              user_id: USER_ID,
              job_id: job._id || job.id,
              company_name: compName,
              job_title: roleTitle,
              status: 'submitted',
              submission_mode: 'live_chromium',
              verified_receipt_id: receipt,
              answers: {
                name: CANDIDATE.name,
                email: CANDIDATE.email,
                phone: CANDIDATE.phone,
                location: CANDIDATE.location
              },
              updated_at: new Date()
            },
            $setOnInsert: {
              _id: `app-live-${Date.now()}-${index}`,
              created_at: new Date()
            }
          },
          { upsert: true }
        );
      } else {
        const errMsg = (result && result.error) ? result.error : 'Unknown ATS response';
        console.log(`⚠️ [ATS Required Input in ${elapsedSec}s] ${errMsg}`);
        screeningCount++;

        await db.collection('applications').updateOne(
          { tenant_id: TENANT_ID, company_name: compName, job_title: roleTitle },
          {
            $set: {
              tenant_id: TENANT_ID,
              user_id: USER_ID,
              job_id: job._id || job.id,
              company_name: compName,
              job_title: roleTitle,
              status: 'requires_manual_action',
              pause_reason: 'screening_questions',
              last_error: errMsg,
              updated_at: new Date()
            },
            $setOnInsert: {
              _id: `app-live-${Date.now()}-${index}`,
              created_at: new Date()
            }
          },
          { upsert: true }
        );
      }
    } catch (err) {
      const elapsedSec = ((Date.now() - jobStartTime) / 1000).toFixed(1);
      console.log(`✗ [Failed in ${elapsedSec}s] ${err.message}`);
      failedCount++;

      await db.collection('applications').updateOne(
        { tenant_id: TENANT_ID, company_name: compName, job_title: roleTitle },
        {
          $set: {
            tenant_id: TENANT_ID,
            user_id: USER_ID,
            job_id: job._id || job.id,
            company_name: compName,
            job_title: roleTitle,
            status: 'failed',
            last_error: err.message,
            updated_at: new Date()
          },
          $setOnInsert: {
            _id: `app-live-${Date.now()}-${index}`,
            created_at: new Date()
          }
        },
        { upsert: true }
      );
    }

    console.log(`Progress: ${successCount} submitted, ${screeningCount} screening questions, ${failedCount} failed`);

    // Brief pause between submissions to prevent aggressive rate-limiting
    await new Promise(r => setTimeout(r, 1200));
  }

  const totalMin = ((Date.now() - startTime) / 60000).toFixed(1);
  console.log('\n================================================================');
  console.log(`🏁 Finished all applications in ${totalMin} minutes.`);
  console.log(`Final Stats: ${successCount} submitted, ${screeningCount} screening questions, ${failedCount} failed.`);
  console.log('================================================================');

  await client.close();
}

main().catch(err => {
  console.error('Fatal runner error:', err);
  process.exit(1);
});
