const assert = require('assert');
const { matchAnswerKey } = require('../browser_submitter');

const testCases = [
  // =========================================================================
  // FALSE POSITIVE ADVERSARIAL CASES (Must return FALSE)
  // =========================================================================

  // "city" collisions (city inside other words)
  { key: 'city', text: 'What is your ethnicity?', expected: false },
  { key: 'city', text: 'Please state your authenticity.', expected: false },
  { key: 'city', text: 'We value simplicity in architecture.', expected: false },
  { key: 'city', text: 'Velocity of development sprints', expected: false },
  { key: 'city', text: 'High elasticity cloud infrastructure', expected: false },
  { key: 'city', text: 'Server capacity planning', expected: false },
  { key: 'city', text: 'Domesticity allowance', expected: false },
  { key: 'city', text: 'Toxicity screening', expected: false },
  { key: 'city', text: 'Multiplicity of microservices', expected: false },
  { key: 'city', text: 'Scarcity of resources', expected: false },
  { key: 'city', text: 'Publicity authorization', expected: false },
  { key: 'city', text: 'Eccentricity score', expected: false },

  // "state" collisions (state inside other words)
  { key: 'state', text: 'Please read this acknowledgment statement', expected: false },
  { key: 'state', text: 'Commercial real estate experience', expected: false },
  { key: 'state', text: 'Interstate commercial travel', expected: false },
  { key: 'state', text: 'Do not overstate your qualifications', expected: false },
  { key: 'state', text: 'Please restate your question', expected: false },
  { key: 'state', text: 'Devastate disaster recovery plans', expected: false },
  { key: 'state', text: 'Understate system latency', expected: false },

  // "rate" collisions (rate inside other words)
  { key: 'rate', text: 'Will you collaborate with the team?', expected: false },
  { key: 'rate', text: 'Corporate governance compliance', expected: false },
  { key: 'rate', text: 'Moderate risk tolerance', expected: false },
  { key: 'rate', text: 'Accurate time tracking', expected: false },
  { key: 'rate', text: 'Salary commensurate with experience', expected: false },
  { key: 'rate', text: 'Deliberate system design choices', expected: false },
  { key: 'rate', text: 'Generate daily metrics', expected: false },
  { key: 'rate', text: 'Accelerate database query execution', expected: false },
  { key: 'rate', text: 'Zero tolerance policy', expected: false },
  { key: 'rate', text: 'Integrate third-party APIs', expected: false },

  // "male" collisions (male inside female)
  { key: 'male', text: 'Female', expected: false },
  { key: 'male', text: 'Female applicant pool', expected: false },
  { key: 'male', text: 'Tamale catering preference', expected: false },

  // "pay" collisions (pay inside other words)
  { key: 'pay', text: 'Taxpayer identification number', expected: false },
  { key: 'pay', text: 'Standard payment terms', expected: false },
  { key: 'pay', text: 'Underpay compensation analysis', expected: false },
  { key: 'pay', text: 'Repay relocation expenses', expected: false },
  { key: 'pay', text: 'Payable accounts overview', expected: false },

  // "sign" collisions (sign inside other words)
  { key: 'sign', text: 'Distributed system design patterns', expected: false },
  { key: 'sign', text: 'Assign tickets to sprint', expected: false },
  { key: 'sign', text: 'Digital signature required', expected: false },
  { key: 'sign', text: 'Resignation notice period', expected: false },
  { key: 'sign', text: 'Consignment inventory tracking', expected: false },

  // "end" collisions (end inside other words)
  { key: 'end', text: 'Backend microservices architecture', expected: false },
  { key: 'end', text: 'Frontend React web development', expected: false },
  { key: 'end', text: 'Did someone recommend you?', expected: false },
  { key: 'end', text: 'Microservices depend on Kafka', expected: false },
  { key: 'end', text: 'Legendary performance record', expected: false },

  // "art" collisions (art inside other words)
  { key: 'art', text: 'Target start date', expected: false },
  { key: 'art', text: 'Smart answers engine', expected: false },
  { key: 'art', text: 'Engineering department head', expected: false },
  { key: 'art', text: 'Part-time or full-time', expected: false },
  { key: 'art', text: 'Business partner agreement', expected: false },

  // "age" collisions (age inside other words)
  { key: 'age', text: 'Manage engineering teams', expected: false },
  { key: 'age', text: 'English language proficiency', expected: false },
  { key: 'age', text: 'Total compensation package', expected: false },
  { key: 'age', text: 'Match percentage calculation', expected: false },

  // =========================================================================
  // TRUE POSITIVE LEGITIMATE CASES (Must return TRUE)
  // =========================================================================
  { key: 'city', text: 'Current City', expected: true },
  { key: 'city', text: 'What city do you reside in?', expected: true },
  { key: 'city', text: 'City / Town', expected: true },
  { key: 'city', text: 'city', expected: true },
  { key: 'state', text: 'State / Province', expected: true },
  { key: 'state', text: 'Current state of residence', expected: true },
  { key: 'rate', text: 'Hourly rate expectation', expected: true },
  { key: 'rate', text: 'Desired pay rate', expected: true },
  { key: 'male', text: 'Male', expected: true },
  { key: 'male', text: 'Gender: Male', expected: true },
  { key: 'work_authorization', text: 'Do you have legal work authorization in the US?', expected: true },
  { key: 'work_authorization', text: 'Work Authorization Status', expected: true },
  { key: 'years_of_experience', text: 'How many years of experience do you have with Go?', expected: true },
  { key: 'first_name', text: 'First Name', expected: true },
  { key: 'last_name', text: 'Candidate Last Name', expected: true },
  { key: 'email', text: 'Primary Email Address', expected: true },
  { key: 'phone', text: 'Mobile Phone Number', expected: true },
  { key: 'linkedin', text: 'LinkedIn Profile URL', expected: true },
  { key: 'github', text: 'GitHub Profile or Portfolio', expected: true },
  { key: 'salary', text: 'Annual Salary Expectations', expected: true },
  { key: 'sponsorship', text: 'Do you now or in the future require visa sponsorship?', expected: true },
  { key: 'notice_period', text: 'What is your notice period?', expected: true }
];

console.log(`Running ${testCases.length} adversarial word-boundary collision tests on matchAnswerKey...`);

let passed = 0;
let failed = 0;
const failures = [];

for (let i = 0; i < testCases.length; i++) {
  const { key, text, expected } = testCases[i];
  const actual = matchAnswerKey(key, text);
  if (actual === expected) {
    passed++;
  } else {
    failed++;
    failures.push({ index: i + 1, key, text, expected, actual });
  }
}

console.log(`Results: ${passed} passed, ${failed} failed out of ${testCases.length} total test cases.`);
if (failures.length > 0) {
  console.error("FAILURES DETECTED:");
  for (const f of failures) {
    console.error(`  Case #${f.index}: key="${f.key}", text="${f.text}" -> expected ${f.expected}, got ${f.actual}`);
  }
  process.exit(1);
} else {
  console.log("ALL 67 WORD-BOUNDARY COLLISION TESTS PASSED 100%!");
}
