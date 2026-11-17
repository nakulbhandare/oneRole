#!/usr/bin/env python3
"""
Test Batch Auto-Apply 100 Software Engineering Applications.
Strict API-only interaction (no direct Mongo queries).
Submits in batches of 5, monitors NATS JetStream queue processing,
and provides live Grafana dashboard metrics.
"""

import sys
import time
import json
import urllib.request
import urllib.error
from datetime import datetime, timedelta, timezone
from urllib.parse import urlencode

BASE_URL = "http://localhost:8088"
GRAFANA_URL = "http://localhost:3000/d/nats-jetstream-flow/nats-jetstream-queue-monitor"
CANDIDATE_EMAIL = "bhandarenakul11@gmail.com"
BATCH_SIZE = 5
TARGET_COUNT = 100

SWE_KEYWORDS = [
    "software", "engineer", "developer", "backend", "frontend", "full stack", "fullstack",
    "platform", "systems", "compiler", "infrastructure", "devops", "cloud", "golang",
    "data engineer", "security engineer", "qa engineer", "sre"
]

EXCLUDE_KEYWORDS = [
    "psychologist", "therapist", "dental", "nurse", "physician", "forklift", "plumber", "painter",
    "housekeeper", "marketing", "recruiter", "sales", "account executive", "mining",
    "customer success", "admissions", "compliance", "procurement", "pastor", "teacher"
]

def is_swe_role(title):
    t = title.lower()
    for excl in EXCLUDE_KEYWORDS:
        if excl in t:
            return False
    for kw in SWE_KEYWORDS:
        if kw in t:
            return True
    return False

def dev_login():
    url = f"{BASE_URL}/api/v1/auth/dev-login"
    payload = json.dumps({"email": CANDIDATE_EMAIL}).encode("utf-8")
    req = urllib.request.Request(url, data=payload, headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req) as resp:
        data = json.loads(resp.read().decode("utf-8"))
        print(f"[Auth] Logged in as: {data.get('email')} (Tenant: {data.get('tenant_id')})")
        return data["access_token"], data["tenant_id"]

def fetch_swe_jobs(token, tenant_id, count=100):
    print(f"\n[Catalog API] Fetching Software Engineering vacancies via GET /api/v1/jobs...")
    swe_jobs = []
    cursor = None
    
    # Query in pages until we collect `count` pure Software Engineering jobs
    while len(swe_jobs) < count:
        params = {
            'limit': '100',
            'sort': 'published_desc',
            'q': 'Software Engineer'
        }
        if cursor:
            params['cursor'] = cursor
            
        url = f"{BASE_URL}/api/v1/jobs?{urlencode(params)}"
        req = urllib.request.Request(url, headers={
            "Authorization": f"Bearer {token}",
            "X-Tenant-ID": tenant_id
        })
        try:
            with urllib.request.urlopen(req) as resp:
                data = json.loads(resp.read().decode("utf-8"))
                items = data.get("items", [])
                if not items:
                    break
                for item in items:
                    title = item.get("title", "")
                    if is_swe_role(title):
                        # Avoid duplicates
                        if not any(j["id"] == item["id"] for j in swe_jobs):
                            swe_jobs.append(item)
                            if len(swe_jobs) >= count:
                                break
                cursor = data.get("next_cursor")
                if not cursor:
                    break
        except Exception as e:
            print(f"[Catalog API Error]: {e}")
            break

    print(f"[Catalog API] Collected exact {len(swe_jobs)} strictly Software Engineering vacancies.")
    return swe_jobs[:count]

def submit_batch(token, tenant_id, job_ids):
    url = f"{BASE_URL}/api/v1/applications/auto-apply/batch"
    payload = json.dumps({"job_ids": job_ids}).encode("utf-8")
    req = urllib.request.Request(url, data=payload, headers={
        "Authorization": f"Bearer {token}",
        "X-Tenant-ID": tenant_id,
        "Content-Type": "application/json"
    })
    with urllib.request.urlopen(req) as resp:
        data = json.loads(resp.read().decode("utf-8"))
        return data

def get_application_status(token, tenant_id, app_id):
    url = f"{BASE_URL}/api/v1/applications/{app_id}"
    req = urllib.request.Request(url, headers={
        "Authorization": f"Bearer {token}",
        "X-Tenant-ID": tenant_id
    })
    try:
        with urllib.request.urlopen(req) as resp:
            return json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as e:
        return {"error": f"HTTP {e.code}"}

def run_load_test():
    start_time = time.time()
    token, tenant_id = dev_login()

    jobs = fetch_swe_jobs(token, tenant_id, TARGET_COUNT)
    if len(jobs) < TARGET_COUNT:
        print(f"[Warning] Catalog yielded {len(jobs)} Software Engineering vacancies.")
    
    total_to_process = len(jobs)
    batches = [jobs[i:i + BATCH_SIZE] for i in range(0, total_to_process, BATCH_SIZE)]
    
    print(f"\n" + "="*70)
    print(f"STARTING LOAD TEST: {total_to_process} SOFTWARE ENGINEERING ROLES")
    print(f"GRAFANA REAL-TIME QUEUE MONITOR: {GRAFANA_URL}")
    print("="*70)

    dispatched_apps = []
    
    # 1. Enqueue in batches of 5
    for b_idx, batch in enumerate(batches, 1):
        job_ids = [j["id"] for j in batch]
        print(f"\n[Batch {b_idx}/{len(batches)}] Dispatching {len(job_ids)} SWE jobs...")
        for j in batch:
            print(f"  • {j.get('company')} - {j.get('title')}")
        
        batch_res = submit_batch(token, tenant_id, job_ids)
        for r in batch_res.get("results", []):
            job_obj = next((j for j in batch if j["id"] == r["job_id"]), {})
            dispatched_apps.append({
                "app_id": r["application_id"],
                "job_id": r["job_id"],
                "company": job_obj.get("company", "Unknown"),
                "title": job_obj.get("title", "Unknown"),
                "url": job_obj.get("apply_url") or job_obj.get("source_url") or "",
                "status": "queued",
                "receipt": None,
                "error": None
            })
        time.sleep(0.5)

    print(f"\n[NATS JetStream] All {len(dispatched_apps)} SWE tasks successfully enqueued!")
    print(f"[Monitoring] Watching live execution across 4 Chromium Worker instances...\n")

    # 2. Monitor status until completion or 5m timeout
    deadline = time.time() + 300
    completed_statuses = {"submitted", "failed_retryable", "failed_permanent", "requires_manual_action"}
    
    while time.time() < deadline:
        active = 0
        submitted = 0
        failed = 0
        manual = 0
        
        for app in dispatched_apps:
            if app["status"] in completed_statuses:
                if app["status"] == "submitted":
                    submitted += 1
                elif app["status"] == "requires_manual_action":
                    manual += 1
                else:
                    failed += 1
                continue
            
            st = get_application_status(token, tenant_id, app["app_id"])
            status_str = st.get("status", "queued")
            app["status"] = status_str
            app["receipt"] = st.get("verified_receipt_id")
            app["error"] = st.get("last_error")
            
            if status_str in completed_statuses:
                if status_str == "submitted":
                    submitted += 1
                    print(f"  [SUCCESS] {app['company']} ({app['title']}) -> Submitted! Receipt: {app['receipt']}")
                elif status_str == "requires_manual_action":
                    manual += 1
                    print(f"  [MANUAL ACTION REQUIRED] {app['company']} ({app['title']}) -> Reason: {st.get('pause_reason')}")
                else:
                    failed += 1
                    print(f"  [FAILED] {app['company']} ({app['title']}) -> Error: {app['error']}")
            else:
                active += 1

        elapsed = int(time.time() - start_time)
        print(f"[Progress @ {elapsed}s] Submitted: {submitted} | In-Progress: {active} | Failed: {failed} | Manual: {manual}")
        
        if active == 0:
            print("\n[Complete] All applications finished processing!")
            break
            
        time.sleep(3)

    total_time = int(time.time() - start_time)
    print("\n" + "="*70)
    print(f"SOFTWARE ENGINEERING LOAD TEST SUMMARY ({total_time}s duration)")
    print("="*70)
    print(f"Total Applications: {len(dispatched_apps)}")
    submitted_count = sum(1 for a in dispatched_apps if a["status"] == "submitted")
    manual_count = sum(1 for a in dispatched_apps if a["status"] == "requires_manual_action")
    failed_count = sum(1 for a in dispatched_apps if a["status"] == "failed_retryable")
    queued_count = sum(1 for a in dispatched_apps if a["status"] == "queued" or a["status"] == "submitting")
    
    print(f"  • Confirmed Submitted: {submitted_count}")
    print(f"  • Requires Manual Action (CAPTCHA/Challenge): {manual_count}")
    print(f"  • Failed with Retryable Error: {failed_count}")
    print(f"  • Incomplete/Timeout: {queued_count}")
    
    missing_questions = {}
    for a in dispatched_apps:
        err = a.get("error")
        if err and "Unanswered questions:" in err:
            parts = err.split("Unanswered questions:")[-1].strip()
            for q in parts.split(","):
                q_clean = q.strip()
                if q_clean and len(q_clean) > 3:
                    missing_questions[q_clean] = missing_questions.get(q_clean, 0) + 1
        elif err and err not in ("Submission was not confirmed by employer ATS portal",):
            missing_questions[err] = missing_questions.get(err, 0) + 1

    if missing_questions:
        print("\n[Questions / Fields Requiring User Input]:")
        for q, cnt in sorted(missing_questions.items(), key=lambda x: -x[1]):
            print(f"  - [{cnt}x] {q}")
    else:
        print("\n[All questions were answered automatically by the engine!]")

    report = {
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "duration_seconds": total_time,
        "total": len(dispatched_apps),
        "submitted": submitted_count,
        "manual_action": manual_count,
        "failed": failed_count,
        "missing_questions": missing_questions,
        "applications": dispatched_apps
    }
    with open("scripts/swe_100_load_test_report.json", "w") as f:
        json.dump(report, f, indent=2)
    print("\nDetailed run report saved to scripts/swe_100_load_test_report.json")

if __name__ == "__main__":
    run_load_test()
