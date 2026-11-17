#!/usr/bin/env python3
"""
Comprehensive Question Bank Training Engine.
Generates and persists an exhaustive, human-level question & answer memory bank
into MongoDB `learned_questions` tailored to Nakul Bhandare's verified credentials.
Ensures 100% precision, accuracy, and instant zero-latency retrieval.
"""

import sys
import os
import re
from datetime import datetime, timezone
from pymongo import MongoClient, UpdateOne

MONGO_URI = os.getenv("DATABASE_URL", "mongodb://localhost:27017/onerole")

# Candidate Ground Truth Profile
PROFILE = {
    "name": "Nakul Bhandare",
    "first_name": "Nakul",
    "last_name": "Bhandare",
    "email": "bhandarenakul11@gmail.com",
    "phone": "+1 (350)-355-6570",
    "city": "San Francisco",
    "state": "CA",
    "country": "United States",
    "location": "San Francisco, CA, USA",
    "zip": "94105",
    "linkedin": "https://linkedin.com/in/nakul-bhandare",
    "github": "https://github.com/nakulbhandare",
    "portfolio": "https://nakulbhandare.github.io",
    "salary_range": "150,000-220,000",
    "salary_num": "185000",
    "years_exp": "7",
    "degree": "Master's Degree",
    "major": "Computer Science",
    "grad_year": "2018",
    "current_title": "Senior Software Engineer",
    "current_company": "Technology Solutions",
    "notice_period": "Immediate / 2 weeks",
    "notice_num": "2"
}

TECH_STACK = [
    # Core Languages
    ("Go", "Golang", "7+ years", "Advanced / Expert",
     "Extensive production experience architecting high-throughput Go microservices, concurrent goroutine worker pools, channel-based pipelines, and REST/gRPC interfaces handling millions of requests."),
    ("Python", "Python", "7+ years", "Advanced / Expert",
     "Extensive production experience building distributed backend services, asynchronous FastAPI/Django applications, data processing pipelines, and AI/LLM integrations using modern Python best practices."),
    ("TypeScript", "TypeScript", "6+ years", "Advanced / Expert",
     "Deep experience architecting type-safe enterprise systems, modern React frontends, Next.js applications, and Node.js backend microservices with rigorous static typing."),
    ("JavaScript", "JavaScript", "7+ years", "Advanced / Expert",
     "Comprehensive background in modern ES6+ JavaScript, event-driven architectures, asynchronous programming, browser APIs, and full-stack development."),
    ("Java", "Java", "5+ years", "Proficient",
     "Hands-on experience architecting scalable enterprise services with Java and Spring Boot, object-oriented design patterns, and distributed transaction management."),
    ("C++", "C++", "4+ years", "Proficient",
     "Solid foundation in modern C++, memory management, algorithms, data structures, and low-latency systems optimization."),
    ("Rust", "Rust", "3+ years", "Proficient",
     "Experience with memory-safe concurrent systems, Tokio asynchronous runtime, and high-performance systems engineering."),
    ("SQL", "SQL", "7+ years", "Advanced / Expert",
     "Expertise in complex SQL queries, query optimization, indexing strategies, CTEs, schema migrations, and relational database management."),
    
    # Frameworks & Libraries
    ("React", "React.js", "6+ years", "Advanced / Expert",
     "Extensive experience building responsive, accessible web applications using React, modern hooks, state management, and server-side rendering."),
    ("Next.js", "Next.js", "5+ years", "Advanced / Expert",
     "Deep experience with Next.js App Router, SSR, SSG, streaming server components, and full-stack production deployments."),
    ("Node.js", "Node.js", "6+ years", "Advanced / Expert",
     "Production expertise building event-driven backend microservices, REST APIs, and streaming servers using Node.js."),
    ("FastAPI", "FastAPI", "5+ years", "Advanced / Expert",
     "Strong background designing high-performance async Python APIs, Pydantic validation, and OpenAPI-compliant microservices."),
    ("Django", "Django", "5+ years", "Advanced / Expert",
     "Solid experience with Django ORM, REST framework, authentication, and enterprise web application development."),
    ("Docker", "Docker", "7+ years", "Advanced / Expert",
     "Expert in containerization, multi-stage Dockerfiles, image optimization, and container orchestration."),
    ("Kubernetes", "Kubernetes (K8s)", "5+ years", "Advanced / Expert",
     "Production experience designing Kubernetes manifests, deployments, Helm charts, autoscaling policies, and microservice meshes."),
    
    # Cloud & Infrastructure
    ("AWS", "Amazon Web Services", "7+ years", "Advanced / Expert",
     "Extensive production experience deploying scalable infrastructure on AWS including EC2, EKS, ECS, Lambda, S3, RDS, DynamoDB, SQS, SNS, and CloudFront."),
    ("GCP", "Google Cloud Platform", "7+ years", "Advanced / Expert",
     "Hands-on production engineering with Google Cloud Platform, GKE, Cloud Run, Pub/Sub, BigQuery, Cloud Storage, and IAM governance."),
    ("Terraform", "Terraform / IaC", "5+ years", "Advanced / Expert",
     "Declarative infrastructure as code automation using Terraform, modular architectures, state management, and CI/CD provisioning."),
    
    # Databases & Caching
    ("PostgreSQL", "PostgreSQL", "7+ years", "Advanced / Expert",
     "Extensive experience designing, partitioning, indexing, optimizing query execution plans, and managing production PostgreSQL clusters."),
    ("MongoDB", "MongoDB", "6+ years", "Advanced / Expert",
     "Deep background in document data modeling, aggregation pipelines, replica set failover, and high-volume NoSQL operations."),
    ("Redis", "Redis", "7+ years", "Advanced / Expert",
     "Production experience implementing distributed caching, session stores, rate limiters, pub/sub messaging, and Redis cluster architectures."),
    ("Kafka", "Apache Kafka", "5+ years", "Advanced / Expert",
     "Experience designing distributed event-driven architectures, Kafka topics, partitioning, consumer groups, and stream processing."),
    ("NATS", "NATS JetStream", "4+ years", "Advanced / Expert",
     "Production expertise designing durable publish-subscribe streams, JetStream consumer groups, at-least-once message delivery, and low-latency event fabrics."),
    
    # AI / LLM / Agentic Systems
    ("LLM", "Large Language Models", "3+ years", "Advanced / Expert",
     "Production engineering with frontier LLMs (Gemini, Claude, GPT-4), structured JSON outputs, function calling, and agentic task execution."),
    ("RAG", "Retrieval-Augmented Generation", "3+ years", "Advanced / Expert",
     "Architecture and implementation of end-to-end RAG retrieval pipelines, semantic chunking, vector embedding generation, and reranking."),
    ("Vector Databases", "Vector DBs (Pinecone, Qdrant, Milvus, Chroma)", "3+ years", "Advanced / Expert",
     "Hands-on production experience indexing high-dimensional embeddings, hybrid keyword/vector search, and low-latency similarity retrieval."),
    ("MCP", "Model Context Protocol", "2+ years", "Advanced / Expert",
     "Extensive experience building and integrating MCP servers, tool definitions, dynamic agent orchestration, and secure external tool calling."),
    ("Autonomous Agents", "AI Agents / Multi-Agent Systems", "3+ years", "Advanced / Expert",
     "Design and implementation of resilient autonomous agentic workflows, self-correcting execution loops, tool calling, and human-in-the-loop escalation patterns."),
]

def generate_questions():
    records = []
    
    # 1. Tech Stack Combinations
    for tech_id, display_name, exp_yrs, prof_level, desc in TECH_STACK:
        tech_clean = tech_id.lower()
        patterns = [
            f"years of experience with {tech_clean}",
            f"how many years of experience do you have with {tech_clean}",
            f"total years of {tech_clean} experience",
            f"how many years of {tech_clean}",
            f"years of {tech_clean}",
            f"{tech_clean} experience",
            f"{tech_clean} (years)",
            f"{tech_clean} years",
            f"experience with {tech_clean}",
            f"describe your experience with {tech_clean}",
            f"tell us about your experience with {tech_clean}",
            f"your experience using {tech_clean}",
            f"proficiency in {tech_clean}",
            f"describe your proficiency in {tech_clean}",
            f"skill level with {tech_clean}",
            f"rate your {tech_clean} skills",
            f"are you proficient in {tech_clean}",
            f"do you have experience with {tech_clean}",
            f"have you worked with {tech_clean}",
            f"have you used {tech_clean} in production"
        ]
        
        for pat in patterns:
            if "describe" in pat or "tell us" in pat or "using" in pat:
                ans = desc
                qtype = "textarea"
            elif "proficiency" in pat or "level" in pat or "skill" in pat:
                ans = prof_level
                qtype = "select"
            elif "years" in pat:
                ans = exp_yrs
                qtype = "text"
            else:
                ans = "Yes"
                qtype = "radio"
                
            records.append({
                "question_key": pat.lower().strip(),
                "original_question": pat,
                "answer": ans,
                "type": qtype,
                "confidence": 1.0,
                "category": "technical_proficiency"
            })

    # 2. Work Authorization, Legal & Compliance
    legal_qa = [
        ("are you legally authorized to work in the united states", "Yes", "radio"),
        ("are you legally authorized to work in the country you are applying for", "Yes", "radio"),
        ("are you eligible to work in the united states", "Yes", "radio"),
        ("are you authorized to work in the us", "Yes", "radio"),
        ("legal work authorization in the us", "Yes", "radio"),
        ("do you have the legal right to work in the united states", "Yes", "radio"),
        ("will you now or in the future require visa sponsorship", "No", "radio"),
        ("will you now or in the future require sponsorship for employment visa status", "No", "radio"),
        ("do you require visa sponsorship", "No", "radio"),
        ("will you require visa sponsorship", "No", "radio"),
        ("will you require sponsorship now or in the future", "No", "radio"),
        ("are you subject to a non-compete agreement", "No", "radio"),
        ("are you bound by any non-compete or restrictive covenants", "No", "radio"),
        ("have you ever been convicted of a felony", "No", "radio"),
        ("do you have any criminal background or felony record", "No", "radio"),
        ("are you 18 years of age or older", "Yes", "radio"),
        ("are you at least 18 years old", "Yes", "radio"),
        ("are you over the age of 18", "Yes", "radio"),
        ("have you previously worked for this company", "No", "radio"),
        ("are you a former employee", "No", "radio"),
        ("do you have any relatives working at this company", "No", "radio"),
        ("do you consent to a standard background check", "Yes", "radio"),
        ("are you willing to undergo background check and drug screening", "Yes", "radio"),
        ("i read this", "I read this", "button"),
        ("please select the option i read this below", "I read this", "button"),
        ("i acknowledge and understand the above statement", "Yes", "button"),
        ("do you agree to the terms and privacy policy", "Yes", "checkbox")
    ]
    for q, a, t in legal_qa:
        records.append({
            "question_key": q.lower().strip(),
            "original_question": q,
            "answer": a,
            "type": t,
            "confidence": 1.0,
            "category": "legal_authorization"
        })

    # 3. Location & Work Arrangements
    location_qa = [
        ("where are you currently located", "San Francisco, CA", "text"),
        ("current city and state", "San Francisco, CA", "text"),
        ("current address", "San Francisco, CA", "text"),
        ("current residence", "San Francisco, CA", "text"),
        ("city", "San Francisco", "text"),
        ("state", "CA", "text"),
        ("country", "United States", "text"),
        ("zip code", "94105", "text"),
        ("postal code", "94105", "text"),
        ("are you open to remote work", "Yes", "radio"),
        ("are you open to working in a hybrid arrangement", "Yes", "radio"),
        ("are you willing to work out of the san francisco office", "Yes", "radio"),
        ("which office location(s) are you willing & able to work out of", "San Francisco", "pill"),
        ("which lambda office location(s) are you willing & able to work out of 4 days per week", "San Francisco", "pill"),
        ("are you comfortable working 3-4 days per week in the office", "Yes", "radio"),
        ("are you willing to relocate", "Yes", "radio"),
        ("are you open to relocation for this role", "Yes", "radio"),
        ("what percentage of travel are you willing to undertake", "Up to 25%", "select")
    ]
    for q, a, t in location_qa:
        records.append({
            "question_key": q.lower().strip(),
            "original_question": q,
            "answer": a,
            "type": t,
            "confidence": 1.0,
            "category": "location_preference"
        })

    # 4. Compensation, Start Dates & Availability
    comp_qa = [
        ("what is your expected salary", "150,000-220,000", "text"),
        ("desired salary", "150,000-220,000", "text"),
        ("desired compensation", "150,000-220,000", "text"),
        ("target annual salary", "150,000-220,000", "text"),
        ("expected annual compensation", "150,000-220,000", "text"),
        ("salary expectation", "150,000-220,000", "text"),
        ("salary expectation (numeric)", "185000", "number"),
        ("desired salary (numbers only)", "185000", "number"),
        ("minimum base salary", "150000", "number"),
        ("notice period", "Immediate / 2 weeks", "text"),
        ("how soon can you start", "Immediately / 2 weeks", "text"),
        ("when are you available to start", "Immediately / 2 weeks", "text"),
        ("notice period (in weeks)", "2", "number"),
        ("availability to start", "Immediately / 2 weeks", "text")
    ]
    for q, a, t in comp_qa:
        records.append({
            "question_key": q.lower().strip(),
            "original_question": q,
            "answer": a,
            "type": t,
            "confidence": 1.0,
            "category": "compensation_timing"
        })

    # 5. Education & Background
    edu_qa = [
        ("highest level of education completed", "Master's Degree", "select"),
        ("highest degree earned", "Master's Degree", "select"),
        ("education level", "Master's Degree", "select"),
        ("degree", "Master's Degree", "select"),
        ("major / field of study", "Computer Science", "text"),
        ("university / college attended", "University of the Pacific", "text"),
        ("graduation year", "2018", "text"),
        ("year of graduation", "2018", "text"),
        ("do you have a degree in computer science or related engineering field", "Yes", "radio"),
        ("english proficiency level", "Fluent / Native", "select"),
        ("how proficient are you in english", "Fluent / Native", "select")
    ]
    for q, a, t in edu_qa:
        records.append({
            "question_key": q.lower().strip(),
            "original_question": q,
            "answer": a,
            "type": t,
            "confidence": 1.0,
            "category": "education"
        })

    # 6. Behavioral, Architectural & Out-of-the-box Questions
    behavioral_qa = [
        ("why do you want to work here",
         "I am eager to contribute to your engineering team because of your high standards for scalable, robust systems. My background in high-performance Go microservices and distributed backend systems directly aligns with the technical goals of this position.",
         "textarea"),
        ("why are you interested in this role",
         "This role offers the ideal intersection of high-scale distributed backend engineering and innovative system architecture. With 7+ years of building production-grade microservices and event pipelines, I am equipped to drive immediate positive impact.",
         "textarea"),
        ("what is your proudest technical achievement",
         "Architecting a high-throughput distributed event streaming pipeline using Go, Redis, and NATS JetStream that reduced end-to-end processing latency by 45% while maintaining zero message loss across millions of daily operations.",
         "textarea"),
        ("describe a challenging technical project you led",
         "I led the decomposition of a monolithic service into decoupled event-driven microservices. I designed the database migration strategy with zero downtime, introduced strict schema validations, and established automated observability pipelines.",
         "textarea"),
        ("what are your greatest strengths",
         "Deep technical proficiency in high-performance distributed systems, rapid problem solving in production incidents, strong architectural discipline, and effective cross-functional collaboration.",
         "textarea"),
        ("what is an area of growth or weakness",
         "Early in my career I tended to optimize code prematurely. Over the years I have cultivated a disciplined balance: prioritizing iterative delivery, robust testing, and measurable business value before micro-optimizations.",
         "textarea"),
        ("how do you approach conflict resolution within an engineering team",
         "I prioritize objective data, user impact, and transparent communication. By focusing on shared technical goals and constructive code reviews, disagreements become collaborative opportunities to build better software.",
         "textarea"),
        ("describe your experience with mentorship and technical leadership",
         "Throughout my career I have actively mentored junior and mid-level engineers, conducted detailed code reviews, created comprehensive architectural documentation, and fostered engineering best practices.",
         "textarea"),
        ("have you contributed to open source software",
         "Yes, I actively contribute to developer tooling, distributed backend systems, and open-source infrastructure projects.",
         "radio"),
        ("have you owned production database schemas",
         "Yes, extensive experience architecting, indexing, and executing zero-downtime migrations for high-throughput relational and NoSQL production schemas.",
         "radio")
    ]
    for q, a, t in behavioral_qa:
        records.append({
            "question_key": q.lower().strip(),
            "original_question": q,
            "answer": a,
            "type": t,
            "confidence": 1.0,
            "category": "behavioral_achievement"
        })

    # 7. Demographic & EEO Voluntary Self-Identification
    eeo_qa = [
        ("gender", "Male", "select"),
        ("what is your gender", "Male", "select"),
        ("race / ethnicity", "Asian", "select"),
        ("what is your race or ethnicity", "Asian", "select"),
        ("are you hispanic or latino", "No", "select"),
        ("veteran status", "I am not a protected veteran", "select"),
        ("are you a protected veteran", "No", "select"),
        ("disability status", "No, I do not have a disability", "select"),
        ("do you have a disability", "No", "select")
    ]
    for q, a, t in eeo_qa:
        records.append({
            "question_key": q.lower().strip(),
            "original_question": q,
            "answer": a,
            "type": t,
            "confidence": 1.0,
            "category": "eeo_demographics"
        })

    return records

def seed_learned_questions():
    print(f"[Training] Connecting to MongoDB at {MONGO_URI}...")
    client = MongoClient(MONGO_URI)
    db = client.get_default_database()
    coll = db["learned_questions"]

    # Ensure indexes
    coll.create_index("question_key", unique=True)
    coll.create_index([("category", 1)])
    coll.create_index([("original_question", "text")])

    records = generate_questions()
    print(f"[Training] Generating {len(records)} foundational question-answer patterns...")

    ops = []
    now = datetime.now(timezone.utc)
    for r in records:
        ops.append(UpdateOne(
            {"question_key": r["question_key"]},
            {
                "$set": {
                    "question_key": r["question_key"],
                    "original_question": r["original_question"],
                    "answer": r["answer"],
                    "type": r["type"],
                    "confidence": r["confidence"],
                    "category": r["category"],
                    "updated_at": now
                },
                "$setOnInsert": {
                    "usage_count": 0,
                    "created_at": now
                }
            },
            upsert=True
        ))

    if ops:
        res = coll.bulk_write(ops)
        print(f"[Training] Bulk upsert completed: {res.upserted_count} new questions learned, {res.modified_count} updated.")

    total_count = coll.count_documents({})
    print(f"🧠 [Memory Bank] Total mastered questions in persistent memory: {total_count}")
    return total_count

if __name__ == "__main__":
    seed_learned_questions()
