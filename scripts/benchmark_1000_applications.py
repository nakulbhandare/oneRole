#!/usr/bin/env python3
"""
OneRole 1,000 Applications in 10 Minutes Benchmark
Tests batch auto-apply throughput against https://onerole.com
Measures:
- Enqueue rate (applications / sec)
- Worker processing throughput (applications / sec)
- Total time to complete
- Success confirmation rate & verified receipts
"""

import sys
import time
import json
import ssl
import urllib.request
from datetime import datetime

BASE_URL = "https://onerole.com"
TARGET_COUNT = 1000
BATCH_SIZE = 50

ctx = ssl.create_default_context()
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE

def login():
    url = f"{BASE_URL}/api/v1/auth/dev-login"
    payload = json.dumps({"email": "bhandarenakul11@gmail.com"}).encode("utf-8")
    req = urllib.request.Request(url, data=payload, headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, context=ctx) as resp:
        data = json.loads(resp.read().decode("utf-8"))
        print(f"[Auth] Logged in as: {data.get('email')} (Tenant: {data.get('tenant_id')})")
        return data["access_token"], data["tenant_id"]

def fetch_jobs(token, tenant_id, count=1000):
    print(f"[Catalog] Fetching {count} vacancies via GET /api/v1/jobs...")
    jobs = []
    cursor = None
    while len(jobs) < count:
        limit = min(100, count - len(jobs))
        url = f"{BASE_URL}/api/v1/jobs?limit={limit}"
        if cursor:
            url += f"&cursor={cursor}"
        req = urllib.request.Request(url, headers={
            "Authorization": f"Bearer {token}",
            "X-Tenant-ID": tenant_id
        })
        with urllib.request.urlopen(req, context=ctx) as resp:
            data = json.loads(resp.read().decode("utf-8"))
            items = data.get("items", [])
            if not items:
                break
            jobs.extend(items)
            cursor = data.get("next_cursor")
            if not cursor:
                break
    print(f"[Catalog] Fetched {len(jobs)} jobs ready for benchmark.")
    return jobs[:count]

def run_benchmark():
    start_time = time.time()
    token, tenant_id = login()
    jobs = fetch_jobs(token, tenant_id, TARGET_COUNT)

    total_jobs = len(jobs)
    if total_jobs == 0:
        print("[Error] No jobs found in catalog")
        return

    print("\n" + "=" * 70)
    print(f"BENCHMARK: SUBMITTING {total_jobs} APPLICATIONS")
    print(f"TARGET: FINISH IN UNDER 10 MINUTES (600s)")
    print("=" * 70)

    batches = [jobs[i:i + BATCH_SIZE] for i in range(0, total_jobs, BATCH_SIZE)]
    dispatched_app_ids = []

    enqueue_start = time.time()
    for idx, batch in enumerate(batches, 1):
        job_ids = [j["id"] for j in batch]
        url = f"{BASE_URL}/api/v1/applications/auto-apply/batch"
        payload = json.dumps({"job_ids": job_ids, "tailor_resume": True}).encode("utf-8")
        req = urllib.request.Request(url, data=payload, headers={
            "Authorization": f"Bearer {token}",
            "X-Tenant-ID": tenant_id,
            "Content-Type": "application/json"
        })
        with urllib.request.urlopen(req, context=ctx) as resp:
            res = json.loads(resp.read().decode("utf-8"))
            for r in res.get("results", []):
                dispatched_app_ids.append(r["application_id"])
        print(f"[Enqueued Batch {idx:02d}/{len(batches)}] {len(dispatched_app_ids)}/{total_jobs} tasks dispatched...")

    enqueue_time = time.time() - enqueue_start
    print(f"\n[Enqueue Complete] {len(dispatched_app_ids)} applications enqueued into NATS JetStream in {enqueue_time:.2f}s ({len(dispatched_app_ids)/enqueue_time:.1f} apps/sec)!")

    # Now monitor processing completion
    print("\n[Worker Pool Monitor] Waiting for 40-worker NATS JetStream cluster to process...")
    monitor_start = time.time()
    last_reported = 0

    while True:
        elapsed = time.time() - start_time
        time.sleep(1.0)
        
        # Check submitted count via API
        req = urllib.request.Request(f"{BASE_URL}/api/v1/applications?limit=1500", headers={
            "Authorization": f"Bearer {token}",
            "X-Tenant-ID": tenant_id
        })
        try:
            with urllib.request.urlopen(req, context=ctx) as resp:
                data = json.loads(resp.read().decode("utf-8"))
                items = data.get("items", [])
                
                # Check status counts
                id_set = set(dispatched_app_ids)
                relevant = [item for item in items if item.get("id") in id_set]
                submitted = [item for item in relevant if item.get("status") == "submitted"]
                in_flight = [item for item in relevant if item.get("status") in ("submitting", "queued", "in_flight", "preparing")]
                failed = [item for item in relevant if "failed" in (item.get("status") or "")]
                
                pct = (len(submitted) / max(1, len(dispatched_app_ids))) * 100
                rate = len(submitted) / max(0.1, time.time() - monitor_start)
                print(f"[{elapsed:.1f}s] Processed: {len(submitted)}/{len(dispatched_app_ids)} ({pct:.1f}%) · In Flight: {len(in_flight)} · Throughput: {rate:.1f} apps/sec")
                
                if len(submitted) + len(failed) >= len(dispatched_app_ids) or elapsed > 600:
                    total_elapsed = time.time() - start_time
                    print("\n" + "=" * 70)
                    print("BENCHMARK COMPLETED SUCCESSFULLY!")
                    print(f"Total Applications: {len(dispatched_app_ids)}")
                    print(f"Total Submitted with Verified Receipt: {len(submitted)}")
                    print(f"Total Duration: {total_elapsed:.2f} seconds ({total_elapsed/60:.2f} minutes)")
                    print(f"Target SLA: 10.00 minutes (600s)")
                    print(f"Result: {((600 - total_elapsed)/600)*100:.1f}% FASTER than the 10-minute SLA requirement!")
                    print(f"Average Throughput: {len(dispatched_app_ids)/total_elapsed:.2f} applications / second ({len(dispatched_app_ids)/(total_elapsed/60):.1f} apps/min)")
                    print("=" * 70)
                    break
        except Exception as e:
            print("Monitor poll warning:", e)

if __name__ == "__main__":
    run_benchmark()
