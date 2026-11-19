// ============================================================================
// Real-Time ATS Score Engine (Dynamic Resume vs Job Description Evaluation)
// ============================================================================

export function computeRealtimeATSScore(job) {
  if (!job) return { score: 88, matchedSkills: [] };

  if (job.match && typeof job.match.score === 'number' && job.match.score > 0) {
    return { score: job.match.score, matchedSkills: job.match.matchedSkills || [] };
  }
  if (typeof job.match_score === 'number' && job.match_score > 0) {
    return { score: job.match_score, matchedSkills: [] };
  }

  const title = (job.title || job.canonical_title || '').trim();
  const desc = (job.description || job.description_text || '').trim();
  const quals = (job.qualifications_text || '').trim();
  const resps = (job.responsibilities_text || '').trim();
  const fullJd = `${title} ${desc} ${quals} ${resps}`.toLowerCase();
  const titleLower = title.toLowerCase();

  const isSoftwareTech = /(software|backend|frontend|full\s*stack|systems|platform|infrastructure|devops|sre|cloud|api|data|golang|python|distributed|architect|programmer|tech\s*lead|engineering\s*manager|firmware|quality)/i.test(titleLower);
  const isNonSoftwareEngineer = /(electrical|power\s*generation|civil|mechanical|optical|chemical|facilities|hvac|structural|recruiter|recruiting|talent|sourcer|sales|account\s*executive|counsel|legal|marketing)/i.test(titleLower);
  const isTechRole = isSoftwareTech && !isNonSoftwareEngineer;

  if (!isTechRole) {
    return { score: 26, matchedSkills: [] };
  }

  const candidateSkills = [
    { name: 'Go', terms: [/\bgolang\b/, /\bgo\b/, /go developer/, /go engineer/, /go backend/], weight: 1.5 },
    { name: 'Distributed Systems', terms: [/distributed systems?/, /scalability/, /fault tolerance/], weight: 1.2 },
    { name: 'Kubernetes', terms: [/kubernetes/, /\bk8s\b/, /\bhelm\b/, /\beks\b/, /\bgke\b/], weight: 1.0 },
    { name: 'Kafka', terms: [/kafka/, /event-driven/, /message queue/, /pubsub/], weight: 1.0 },
    { name: 'gRPC', terms: [/grpc/, /protobuf/, /protocol buffers/], weight: 1.0 },
    { name: 'Docker', terms: [/docker/, /containers?/, /containerization/], weight: 0.9 },
    { name: 'AWS', terms: [/\baws\b/, /amazon web services/, /\bec2\b/, /\bs3\b/, /lambda/], weight: 0.9 },
    { name: 'GCP', terms: [/google cloud/, /\bgcp\b/], weight: 0.8 },
    { name: 'Microservices', terms: [/microservices?/, /\bsoa\b/], weight: 1.0 },
    { name: 'Concurrency', terms: [/concurrency/, /concurrent/, /goroutines?/, /channels?/], weight: 1.0 },
    { name: 'CI/CD', terms: [/ci\/cd/, /github actions/, /pipeline/], weight: 0.8 },
    { name: 'Databases', terms: [/postgresql/, /postgres/, /redis/, /mongodb/, /\bsql\b/], weight: 0.9 },
    { name: 'APIs', terms: [/rest api/, /restful/, /\bapi\b/, /\bapis\b/], weight: 0.7 },
    { name: 'Networking', terms: [/networking/, /tcp/, /socket/, /proxy/], weight: 0.6 }
  ];

  const matchedSkills = [];
  let matchedWeight = 0;
  let totalWeight = 0;

  for (const s of candidateSkills) {
    totalWeight += s.weight;
    if (s.terms.some(re => re.test(fullJd))) {
      matchedWeight += s.weight;
      matchedSkills.push(s.name);
    }
  }

  const divergentSkills = [
    { name: 'Java', terms: [/\bjava\b/, /\bspring\b/, /spring boot/], weight: 1.4 },
    { name: 'C++', terms: [/\bc\+\+\b/, /embedded/, /\brust\b/], weight: 1.3 },
    { name: 'Python', terms: [/\bpython\b/, /django/, /fastapi/], weight: 1.0 },
    { name: 'Elasticsearch', terms: [/elasticsearch/, /elastic/, /opensearch/], weight: 1.0 },
    { name: 'Security', terms: [/penetration testing/, /infosec/, /soc2/, /threat/, /cryptography/], weight: 1.1 },
    { name: 'QA', terms: [/quality engineer/, /\bqa\b/, /test automation/, /sdet/, /selenium/, /cypress/], weight: 1.3 },
    { name: 'ML/AI', terms: [/machine learning/, /deep learning/, /pytorch/, /tensorflow/, /\bllm\b/, /\bnlp\b/, /model training/], weight: 1.3 },
    { name: 'Frontend', terms: [/\breact\b/, /\bvue\b/, /\bangular\b/, /frontend engineer/, /ui engineer/], weight: 1.0 },
    { name: 'Data Warehousing', terms: [/spark/, /hadoop/, /snowflake/, /databricks/], weight: 1.1 },
    { name: 'Firmware', terms: [/firmware/, /rtos/, /microcontroller/], weight: 1.2 }
  ];

  let penaltyWeight = 0;
  for (const s of divergentSkills) {
    if (s.terms.some(re => re.test(fullJd))) {
      penaltyWeight += s.weight;
    }
  }

  const skillScore = (matchedWeight / (totalWeight || 1)) * 40;
  let roleTitleScore = 0;
  if (/golang|go developer|go engineer/i.test(titleLower)) {
    roleTitleScore = 30;
  } else if (/backend|distributed|systems|infrastructure|platform/i.test(titleLower)) {
    roleTitleScore = 26;
  } else if (/software engineer|staff software|principal/i.test(titleLower)) {
    roleTitleScore = 22;
  } else if (/cloud|api|devops|sre/i.test(titleLower)) {
    roleTitleScore = 18;
  } else {
    roleTitleScore = 10;
  }

  let seniorityScore = 15;
  if (/senior|staff|lead|principal|architect/i.test(titleLower)) {
    seniorityScore = 15;
  } else if (/mid|intermediate/i.test(titleLower)) {
    seniorityScore = 13;
  } else if (/junior|entry|intern|graduate/i.test(titleLower)) {
    seniorityScore = 8;
  }

  let techPenalty = penaltyWeight * 4.5;
  let baseScore = skillScore + roleTitleScore + seniorityScore + 15 - techPenalty;

  const company = (job.company || '').toLowerCase();
  let companyAffinity = 0;
  if (company.includes('samsara')) companyAffinity = 3;
  else if (company.includes('stripe')) companyAffinity = 2;
  else if (company.includes('lyft')) companyAffinity = 1;
  else if (company.includes('ramp')) companyAffinity = 2;
  else if (company.includes('openai')) companyAffinity = 0;
  else if (company.includes('duolingo')) companyAffinity = -5;

  let finalScore = Math.round(baseScore + companyAffinity);
  if (finalScore > 98) finalScore = 98;
  if (finalScore < 30) finalScore = 30;

  return { score: finalScore, matchedSkills };
}

