// ============================================================================
// OneRole API Service — Enterprise Integration Client
// ============================================================================

let authToken = localStorage.getItem('onerole_auth_token') || '';
let tenantId = localStorage.getItem('onerole_tenant_id') || '00000000-0000-4000-8000-000000000001';

export function getAuthToken() {
  return authToken;
}

export function setAuthToken(token) {
  authToken = token;
  if (token) {
    localStorage.setItem('onerole_auth_token', token);
  } else {
    localStorage.removeItem('onerole_auth_token');
  }
}

export async function authenticatedFetch(url, options = {}) {
  const headers = { ...(options.headers || {}) };
  if (authToken) {
    headers['Authorization'] = `Bearer ${authToken}`;
  }
  if (tenantId) {
    headers['X-Tenant-ID'] = tenantId;
  }
  if (!headers['Content-Type'] && !(options.body instanceof FormData)) {
    headers['Content-Type'] = 'application/json';
  }

  const res = await fetch(url, { ...options, headers });
  if (res.status === 401) {
    const success = await doDevLogin();
    if (success && authToken) {
      headers['Authorization'] = `Bearer ${authToken}`;
      return fetch(url, { ...options, headers });
    }
  }
  return res;
}

export async function doDevLogin(email = 'bhandarenakul11@gmail.com') {
  try {
    const res = await fetch('/api/v1/auth/dev-login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email })
    });
    if (!res.ok) return null;
    const data = await res.json();
    const token = data.access_token || (data.session && data.session.access_token);
    if (token) {
      setAuthToken(token);
    }
    if (data.tenant_id) {
      tenantId = data.tenant_id;
      localStorage.setItem('onerole_tenant_id', tenantId);
    }
    return data;
  } catch (err) {
    console.warn('[API] Dev login failed:', err.message);
    return null;
  }
}

export async function searchJobs(filters = {}) {
  const params = new URLSearchParams();
  params.set('limit', '30');
  params.set('sort', 'published_desc');

  let queryParts = [];
  if (filters.searchTags && filters.searchTags.length > 0) {
    queryParts.push(filters.searchTags.join(' '));
  }
  if (filters.role) {
    queryParts.push(filters.role);
  }
  if (queryParts.length > 0) {
    params.set('q', queryParts.join(' '));
  }
  if (filters.exclude) {
    params.set('exclude', filters.exclude);
  }
  if (filters.datePreset && filters.datePreset !== 'any') {
    params.set('date_preset', filters.datePreset);
  }
  if (filters.degreeLevel) {
    params.set('degree_level', filters.degreeLevel);
  }
  if (filters.maxExperience) {
    params.set('max_experience_years', filters.maxExperience);
  }
  if (filters.visa) {
    params.set('visa_sponsorship', 'yes');
  }
  if (filters.jobType) {
    params.set('job_type', filters.jobType);
  }
  if (filters.employmentType) {
    params.set('employment_type', filters.employmentType);
  }
  if (filters.domain || filters.job_domain) {
    params.set('domain', filters.domain || filters.job_domain);
  }
  if (filters.targetCountry || filters.country) {
    params.set('country', filters.targetCountry || filters.country);
  }

  (filters.workplaces || []).forEach(w => params.append('workplace', w));
  (filters.locations || []).forEach(l => params.append('location', l));

  const res = await authenticatedFetch(`/api/v1/jobs/search?${params.toString()}`);
  if (!res.ok) throw new Error(`Search failed: ${res.statusText}`);
  return res.json();
}

export async function fetchFilterMetadata() {
  try {
    const res = await authenticatedFetch('/api/v1/jobs/filters');
    if (!res || !res.ok) return {};
    const data = await res.json();
    return data.facets || {};
  } catch (e) {
    console.warn('[API] Filter metadata error:', e);
    return {};
  }
}

export async function fetchApplications() {
  try {
    const res = await authenticatedFetch('/api/v1/applications');
    if (!res || !res.ok) return [];
    const data = await res.json();
    return data.items || [];
  } catch (e) {
    console.warn('[API] Fetch applications error:', e);
    return [];
  }
}

export async function submitJobApplication(job) {
  const payload = {
    job_id: job.id,
    company: job.company || 'Enterprise Company',
    title: job.title || 'Software Engineer',
    source_url: job.sourceUrl || job.source_url || '#',
    candidate_email: 'bhandarenakul11@gmail.com',
    candidate_name: 'Nakul Bhandare'
  };

  const res = await authenticatedFetch('/api/v1/applications', {
    method: 'POST',
    body: JSON.stringify(payload)
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.message || 'Application dispatch failed');
  }
  return res.json();
}

export async function fetchTelemetry() {
  try {
    const res = await fetch('/telemetry/nats');
    if (!res.ok) return null;
    return res.json();
  } catch (e) {
    return null;
  }
}
