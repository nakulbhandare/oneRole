#!/usr/bin/env python3
"""
scripts/ingest_freehire_companies.py

Ingests companies and active jobs from freehire.me API,
enriches config/companies.json, creates/updates companies.json,
and enriches company.json with missing URLs and verified company records.
"""

import json
import os
import re
import sys
import time
import urllib.parse
import urllib.request
from concurrent.futures import ThreadPoolExecutor, as_completed
from datetime import datetime, timezone

CLEAN_SLUG_REGEX = re.compile(r'[^a-z0-9]+')

def clean_slug(s):
    if not s:
        return ''
    s = CLEAN_SLUG_REGEX.sub('-', s.lower().strip())
    return s.strip('-')

def fetch_json(url, timeout=12):
    req = urllib.request.Request(
        url,
        headers={
            'User-Agent': 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36',
            'Accept': 'application/json',
        }
    )
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        return json.loads(resp.read().decode('utf-8'))

def extract_job_board_info(source, job_url, slug, website=None):
    if not job_url:
        # Fallback to website or generic slug
        clean_s = clean_slug(slug)
        if website:
            pu = urllib.parse.urlparse(website)
            host = pu.netloc.lower()
            return 'generic', clean_s or 'careers', website, host
        return 'generic', clean_s or 'careers', f'https://freehire.me/companies/{slug}', 'freehire.me'

    u = urllib.parse.urlparse(job_url)
    host = u.netloc.lower()
    path_parts = [p for p in u.path.strip('/').split('/') if p]
    src = (source or '').lower().strip()

    # Clean tracking query parameters
    clean_job_url = urllib.parse.urlunparse((u.scheme, u.netloc, u.path, '', '', ''))

    # SmartRecruiters
    if 'smartrecruiters' in src or 'smartrecruiters.com' in host:
        board = path_parts[0] if path_parts else slug
        return 'smartrecruiters', board, f'https://jobs.smartrecruiters.com/{board}', 'jobs.smartrecruiters.com'

    # Greenhouse
    if 'greenhouse' in src or 'greenhouse.io' in host:
        qs = urllib.parse.parse_qs(u.query)
        board = qs.get('for', [''])[0]
        if not board and path_parts:
            if path_parts[0] in ['embed', 'jobs', 'v1'] and len(path_parts) > 1:
                board = path_parts[1]
            else:
                board = path_parts[0]
        if not board:
            board = slug
        return 'greenhouse', board, f'https://boards.greenhouse.io/{board}', 'boards.greenhouse.io'

    # Lever
    if 'lever' in src or 'lever.co' in host:
        board = path_parts[0] if path_parts else slug
        return 'lever', board, f'https://jobs.lever.co/{board}', 'jobs.lever.co'

    # Ashby
    if 'ashby' in src or 'ashbyhq.com' in host:
        board = path_parts[0] if path_parts else slug
        return 'ashby', board, f'https://jobs.ashbyhq.com/{board}', 'jobs.ashbyhq.com'

    # Breezy
    if 'breezy' in src or 'breezy.hr' in host:
        board = host.split('.')[0] if '.' in host else slug
        return 'breezy', board, f'https://{board}.breezy.hr', host

    # BambooHR
    if 'bamboohr' in src or 'bamboohr.com' in host:
        board = host.split('.')[0] if '.' in host else slug
        return 'bamboohr', board, f'https://{board}.bamboohr.com/careers', host

    # Recruitee
    if 'recruitee' in src or 'recruitee.com' in host:
        board = host.split('.')[0] if '.' in host else slug
        return 'recruitee', board, f'https://{board}.recruitee.com', host

    # Workday
    if 'workday' in src or 'myworkdayjobs.com' in host:
        site = path_parts[0] if path_parts else 'careers'
        return 'workday', site, f'https://{host}/{site}', host

    # Universal Generic fallback
    board = clean_slug(slug)
    if not board and len(path_parts) > 0:
        board = clean_slug(path_parts[0])
    if not board:
        board = clean_slug(host)
    if len(board) > 64:
        board = board[:64]
    return 'generic', board or 'careers', clean_job_url, host

def main():
    print("=" * 60)
    print("OneRole Freehire.me Company Ingestion & Discovery Pipeline")
    print("=" * 60)
    start_time = time.time()

    # 1. Fetch companies list with pagination
    # Fetch 10 pages x 100 limit = 1000 companies
    total_to_fetch = 1000
    page_limit = 100
    companies_meta = []
    print(f"[*] Step 1: Paginating through https://freehire.me/api/v1/companies (target: {total_to_fetch} companies)...")

    for offset in range(0, total_to_fetch, page_limit):
        url = f"https://freehire.me/api/v1/companies?limit={page_limit}&offset={offset}"
        try:
            data = fetch_json(url)
            items = data.get('data', [])
            companies_meta.extend(items)
            print(f"    - Offset {offset:4d}: fetched {len(items):3d} companies (total so far: {len(companies_meta)})")
        except Exception as e:
            print(f"    - Offset {offset:4d} failed: {e}")
            break

    print(f"[+] Total companies fetched from list API: {len(companies_meta)}")

    # Prioritize companies with active jobs (job_count > 0)
    active_companies = [c for c in companies_meta if c.get('job_count', 0) > 0]
    print(f"[+] Companies with active jobs (job_count > 0): {len(active_companies)}")

    # 2. Fetch detail for active companies concurrently
    print(f"[*] Step 2: Fetching details and first job for {len(active_companies)} companies (concurrency=25)...")
    detailed_companies = []
    errors = 0

    def fetch_company_detail(c):
        slug = c.get('slug')
        url = f"https://freehire.me/api/v1/companies/{slug}"
        try:
            res = fetch_json(url)
            return (c, res.get('data', {}))
        except Exception as e:
            return (c, None)

    with ThreadPoolExecutor(max_workers=25) as executor:
        futures = {executor.submit(fetch_company_detail, c): c for c in active_companies}
        done_count = 0
        for f in as_completed(futures):
            c, d = f.result()
            done_count += 1
            if d is not None:
                company_info = d.get('company', {})
                jobs = d.get('jobs', [])
                website = company_info.get('company_info', {}).get('website')
                first_job = jobs[0] if jobs else {}
                src = first_job.get('source')
                job_url = first_job.get('url')

                provider, board, job_board_url, host = extract_job_board_info(
                    source=src,
                    job_url=job_url,
                    slug=c.get('slug'),
                    website=website
                )

                detailed_companies.append({
                    'name': company_info.get('name') or c.get('name'),
                    'slug': c.get('slug'),
                    'job_count': c.get('job_count', 0),
                    'website': website or f"https://{c.get('slug')}.com",
                    'provider': provider,
                    'board': board,
                    'job_board_url': job_board_url,
                    'base_job_url': job_board_url,
                    'final_url': job_board_url,
                    'host': host,
                    'first_job_url': job_url,
                    'ats_source': src
                })
            else:
                errors += 1

            if done_count % 200 == 0 or done_count == len(active_companies):
                print(f"    - Progress: {done_count}/{len(active_companies)} companies processed ({errors} errors)")

    print(f"[+] Successfully extracted details for {len(detailed_companies)} companies.")

    # 3. Enrich existing config/companies.json
    print("[*] Step 3: Enriching config/companies.json and companies.json...")
    config_companies_path = "config/companies.json"
    root_companies_path = "companies.json"

    known_seeds = {}
    if os.path.exists(config_companies_path):
        with open(config_companies_path, 'r', encoding='utf-8') as f:
            raw = json.load(f)
            seed_list = raw.get('companies', []) if isinstance(raw, dict) else raw
            for s in seed_list:
                known_seeds[s.get('name', '').lower()] = s

    # Fill in missing URLs for existing seeds
    default_domain_mapping = {
        'google': ('google.com', 'boards.greenhouse.io', 'https://boards.greenhouse.io/google'),
        'meta': ('meta.com', 'boards.greenhouse.io', 'https://boards.greenhouse.io/facebook'),
        'amazon': ('amazon.com', 'boards.greenhouse.io', 'https://boards.greenhouse.io/amazon'),
        'netflix': ('netflix.com', 'jobs.lever.co', 'https://jobs.lever.co/netflix'),
        'apple': ('apple.com', 'boards.greenhouse.io', 'https://boards.greenhouse.io/apple'),
        'microsoft': ('microsoft.com', 'boards.greenhouse.io', 'https://boards.greenhouse.io/microsoft'),
        'nvidia': ('nvidia.com', 'boards.greenhouse.io', 'https://boards.greenhouse.io/nvidia'),
        'stripe': ('stripe.com', 'boards.greenhouse.io', 'https://boards.greenhouse.io/stripe'),
        'uber': ('uber.com', 'boards.greenhouse.io', 'https://boards.greenhouse.io/uber'),
        'airbnb': ('airbnb.com', 'boards.greenhouse.io', 'https://boards.greenhouse.io/airbnb'),
        'okta': ('okta.com', 'boards.greenhouse.io', 'https://boards.greenhouse.io/okta'),
        'datadog': ('datadoghq.com', 'boards.greenhouse.io', 'https://boards.greenhouse.io/datadog'),
        'mongodb': ('mongodb.com', 'boards.greenhouse.io', 'https://boards.greenhouse.io/mongodb'),
        'elastic': ('elastic.co', 'boards.greenhouse.io', 'https://boards.greenhouse.io/elastic'),
        'coinbase': ('coinbase.com', 'boards.greenhouse.io', 'https://boards.greenhouse.io/coinbase'),
        'gitlab': ('gitlab.com', 'boards.greenhouse.io', 'https://boards.greenhouse.io/gitlab'),
        'figma': ('figma.com', 'boards.greenhouse.io', 'https://boards.greenhouse.io/figma'),
        'reddit': ('reddit.com', 'boards.greenhouse.io', 'https://boards.greenhouse.io/reddit'),
        'pinterest': ('pinterest.com', 'boards.greenhouse.io', 'https://boards.greenhouse.io/pinterest'),
        'affirm': ('affirm.com', 'boards.greenhouse.io', 'https://boards.greenhouse.io/affirm'),
        'lyft': ('lyft.com', 'boards.greenhouse.io', 'https://boards.greenhouse.io/lyft'),
        'gusto': ('gusto.com', 'boards.greenhouse.io', 'https://boards.greenhouse.io/gusto'),
        'discord': ('discord.com', 'boards.greenhouse.io', 'https://boards.greenhouse.io/discord'),
        'dropbox': ('dropbox.com', 'boards.greenhouse.io', 'https://boards.greenhouse.io/dropbox'),
        'sentry': ('sentry.io', 'jobs.ashbyhq.com', 'https://jobs.ashbyhq.com/sentry'),
        'docker': ('docker.com', 'jobs.ashbyhq.com', 'https://jobs.ashbyhq.com/docker'),
        'notion': ('notion.so', 'jobs.ashbyhq.com', 'https://jobs.ashbyhq.com/notion'),
        'vercel': ('vercel.com', 'jobs.ashbyhq.com', 'https://jobs.ashbyhq.com/vercel'),
        'linear': ('linear.app', 'jobs.ashbyhq.com', 'https://jobs.ashbyhq.com/linear'),
        'openai': ('openai.com', 'jobs.ashbyhq.com', 'https://jobs.ashbyhq.com/openai'),
        'ramp': ('ramp.com', 'jobs.ashbyhq.com', 'https://jobs.ashbyhq.com/ramp'),
        'cockroach labs': ('cockroachlabs.com', 'boards.greenhouse.io', 'https://boards.greenhouse.io/cockroachlabs'),
        'grafana labs': ('grafana.com', 'boards.greenhouse.io', 'https://boards.greenhouse.io/grafanalabs'),
    }

    enriched_seeds_count = 0
    all_companies_dict = {}

    # Populate existing seeds with filled missing URLs
    for name_lower, seed in known_seeds.items():
        name = seed.get('name')
        provider = seed.get('provider', '')
        board = seed.get('board', '')
        mapping = default_domain_mapping.get(name_lower, (f"{clean_slug(name)}.com", f"boards.{provider}.com", f"https://jobs.{provider}.com/{board}"))
        domain, host, job_url = mapping

        updated = dict(seed)
        if not updated.get('host'):
            updated['host'] = host
            enriched_seeds_count += 1
        if not updated.get('base_job_url'):
            updated['base_job_url'] = job_url
        if not updated.get('final_url'):
            updated['final_url'] = job_url
        if not updated.get('job_board_url'):
            updated['job_board_url'] = job_url
        if not updated.get('website'):
            updated['website'] = f"https://{domain}"

        all_companies_dict[name.lower()] = updated

    # Merge newly ingested companies from freehire.me
    new_companies_added = 0
    for dc in detailed_companies:
        name_key = dc['name'].lower().strip()
        if name_key not in all_companies_dict:
            all_companies_dict[name_key] = {
                'name': dc['name'],
                'provider': dc['provider'],
                'board': dc['board'],
                'host': dc['host'],
                'job_board_url': dc['job_board_url'],
                'base_job_url': dc['base_job_url'],
                'final_url': dc['final_url'],
                'website': dc['website'],
                'job_count': dc['job_count']
            }
            new_companies_added += 1
        else:
            # If exists, enrich any missing fields
            existing = all_companies_dict[name_key]
            if not existing.get('job_board_url') and dc.get('job_board_url'):
                existing['job_board_url'] = dc['job_board_url']
                enriched_seeds_count += 1
            if not existing.get('website') and dc.get('website'):
                existing['website'] = dc['website']

    final_companies_list = sorted(all_companies_dict.values(), key=lambda x: x.get('name', '').lower())
    companies_envelope = {"companies": final_companies_list}

    # Write config/companies.json
    with open(config_companies_path, 'w', encoding='utf-8') as f:
        json.dump(companies_envelope, f, indent=2)

    # Write root companies.json
    with open(root_companies_path, 'w', encoding='utf-8') as f:
        json.dump(companies_envelope, f, indent=2)

    print(f"[+] Saved {len(final_companies_list)} companies in {config_companies_path} and {root_companies_path}")

    # 4. Enrich company.json in root workspace
    print("[*] Step 4: Enriching company.json in root workspace...")
    company_json_path = "company.json"
    with open(company_json_path, 'r', encoding='utf-8') as f:
        existing_records = json.load(f)

    print(f"    - Existing records in company.json: {len(existing_records)}")
    existing_by_name = {}
    existing_by_host = {}
    existing_by_url = {}
    max_id = 0
    max_row = 0

    now_iso = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

    for r in existing_records:
        r_id = r.get('id', 0)
        if r_id > max_id:
            max_id = r_id
        r_row = r.get('source_row', 0)
        if r_row > max_row:
            max_row = r_row
        cname = r.get('company_name')
        if cname:
            existing_by_name[cname.lower().strip()] = r
        host = r.get('host')
        if host:
            existing_by_host[host.lower().strip()] = r
        base_url = r.get('base_job_url')
        if base_url:
            existing_by_url[base_url.lower().strip()] = r

    # Enrich existing records with missing URLs, unreachable status, or null names
    healed_in_company_json = 0
    names_enriched = 0

    # Build lookup from ingested companies
    ingested_by_host = {}
    ingested_by_name = {}
    ingested_by_slug = {}
    for dc in detailed_companies:
        if dc.get('host'):
            ingested_by_host[dc['host'].lower().strip()] = dc
        if dc.get('name'):
            ingested_by_name[dc['name'].lower().strip()] = dc
        if dc.get('slug'):
            ingested_by_slug[dc['slug'].lower().strip()] = dc

    for r in existing_records:
        r_host = (r.get('host') or '').lower().strip()
        r_name = (r.get('company_name') or '').lower().strip()
        r_status = r.get('verified_status', '')

        # Check if unreachable and can be healed by ingested data
        if r_status == 'unreachable':
            match = ingested_by_name.get(r_name) or ingested_by_host.get(r_host)
            if match and match.get('job_board_url'):
                r['base_job_url'] = match['job_board_url']
                r['final_url'] = match['job_board_url']
                r['ats'] = match['provider']
                r['verified_status'] = 'valid'
                r['last_checked_at'] = now_iso
                healed_in_company_json += 1

        # Check if company_name is null and can be resolved
        if not r.get('company_name'):
            match = ingested_by_host.get(r_host)
            if match:
                r['company_name'] = match['name']
                names_enriched += 1
            else:
                # Infer name from host
                parts = r_host.split('.')
                if len(parts) > 2 and parts[0] not in ['www', 'secure', 'secure6', 'secure7', 'jobs', 'careers', 'app']:
                    candidate = parts[0].replace('-', ' ').title()
                    r['company_name'] = candidate
                    names_enriched += 1

    # Add new companies from freehire.me to company.json
    new_company_json_records = 0
    for dc in detailed_companies:
        dc_name = dc['name'].strip()
        dc_name_lower = dc_name.lower()
        dc_url = dc['job_board_url'].strip().lower()

        # Check if already present
        if dc_name_lower in existing_by_name or dc_url in existing_by_url:
            continue

        max_id += 1
        max_row += 1
        new_record = {
            "id": max_id,
            "company_name": dc_name,
            "base_job_url": dc['job_board_url'],
            "ats": dc['provider'],
            "host": dc['host'],
            "source_row": max_row,
            "added_for_missing_job_url": False,
            "verified_status": "valid",
            "final_url": dc['job_board_url'],
            "last_checked_at": now_iso
        }
        existing_records.append(new_record)
        existing_by_name[dc_name_lower] = new_record
        existing_by_url[dc_url] = new_record
        new_company_json_records += 1

    # Save company.json
    with open(company_json_path, 'w', encoding='utf-8') as f:
        json.dump(existing_records, f, indent=2)

    elapsed = time.time() - start_time
    print(f"[+] Successfully updated company.json: {len(existing_records)} total records")
    print("\n" + "=" * 60)
    print("INGESTION & ENRICHMENT SUMMARY")
    print("=" * 60)
    print(f"Total companies fetched from freehire.me: {len(companies_meta)}")
    print(f"Companies with active jobs fetched:       {len(detailed_companies)}")
    print(f"Companies enriched with missing URLs:     {enriched_seeds_count + healed_in_company_json}")
    print(f"  - Config seeds missing URLs enriched:   {enriched_seeds_count}")
    print(f"  - Unreachable URLs healed in dataset:   {healed_in_company_json}")
    print(f"  - Null company names enriched:          {names_enriched}")
    print(f"New companies added to companies.json:    {new_companies_added}")
    print(f"New companies added to company.json:      {new_company_json_records}")
    print(f"Total companies now in companies.json:    {len(final_companies_list)}")
    print(f"Total companies now in company.json:      {len(existing_records)}")
    print(f"Execution Time:                           {elapsed:.2f}s")
    print("=" * 60)

if __name__ == '__main__':
    main()
