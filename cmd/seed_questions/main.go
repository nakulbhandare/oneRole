package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type TechItem struct {
	ID          string
	Display     string
	Years       string
	Proficiency string
	Description string
}

func main() {
	mongoURI := os.Getenv("DATABASE_URL")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017/onerole"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer client.Disconnect(ctx)

	db := client.Database("onerole")
	coll := db.Collection("learned_questions")

	// Create Indexes
	_, _ = coll.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "question_key", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "category", Value: 1}},
		},
	})

	techStack := []TechItem{
		{"Go", "Golang", "7+ years", "Advanced / Expert", "Extensive production experience architecting high-throughput Go microservices, concurrent goroutine worker pools, channel-based pipelines, and REST/gRPC interfaces handling millions of requests."},
		{"Python", "Python", "7+ years", "Advanced / Expert", "Extensive production experience building distributed backend services, asynchronous FastAPI/Django applications, data processing pipelines, and AI/LLM integrations using modern Python best practices."},
		{"TypeScript", "TypeScript", "6+ years", "Advanced / Expert", "Deep experience architecting type-safe enterprise systems, modern React frontends, Next.js applications, and Node.js backend microservices with rigorous static typing."},
		{"JavaScript", "JavaScript", "7+ years", "Advanced / Expert", "Comprehensive background in modern ES6+ JavaScript, event-driven architectures, asynchronous programming, browser APIs, and full-stack development."},
		{"Java", "Java", "5+ years", "Proficient", "Hands-on experience architecting scalable enterprise services with Java and Spring Boot, object-oriented design patterns, and distributed transaction management."},
		{"C++", "C++", "4+ years", "Proficient", "Solid foundation in modern C++, memory management, algorithms, data structures, and low-latency systems optimization."},
		{"Rust", "Rust", "3+ years", "Proficient", "Experience with memory-safe concurrent systems, Tokio asynchronous runtime, and high-performance systems engineering."},
		{"SQL", "SQL", "7+ years", "Advanced / Expert", "Expertise in complex SQL queries, query optimization, indexing strategies, CTEs, schema migrations, and relational database management."},
		{"React", "React.js", "6+ years", "Advanced / Expert", "Extensive experience building responsive, accessible web applications using React, modern hooks, state management, and server-side rendering."},
		{"Next.js", "Next.js", "5+ years", "Advanced / Expert", "Deep experience with Next.js App Router, SSR, SSG, streaming server components, and full-stack production deployments."},
		{"Node.js", "Node.js", "6+ years", "Advanced / Expert", "Production expertise building event-driven backend microservices, REST APIs, and streaming servers using Node.js."},
		{"FastAPI", "FastAPI", "5+ years", "Advanced / Expert", "Strong background designing high-performance async Python APIs, Pydantic validation, and OpenAPI-compliant microservices."},
		{"Django", "Django", "5+ years", "Advanced / Expert", "Solid experience with Django ORM, REST framework, authentication, and enterprise web application development."},
		{"Docker", "Docker", "7+ years", "Advanced / Expert", "Expert in containerization, multi-stage Dockerfiles, image optimization, and container orchestration."},
		{"Kubernetes", "Kubernetes (K8s)", "5+ years", "Advanced / Expert", "Production experience designing Kubernetes manifests, deployments, Helm charts, autoscaling policies, and microservice meshes."},
		{"AWS", "Amazon Web Services", "7+ years", "Advanced / Expert", "Extensive production experience deploying scalable infrastructure on AWS including EC2, EKS, ECS, Lambda, S3, RDS, DynamoDB, SQS, SNS, and CloudFront."},
		{"GCP", "Google Cloud Platform", "7+ years", "Advanced / Expert", "Hands-on production engineering with Google Cloud Platform, GKE, Cloud Run, Pub/Sub, BigQuery, Cloud Storage, and IAM governance."},
		{"Terraform", "Terraform / IaC", "5+ years", "Advanced / Expert", "Declarative infrastructure as code automation using Terraform, modular architectures, state management, and CI/CD provisioning."},
		{"PostgreSQL", "PostgreSQL", "7+ years", "Advanced / Expert", "Extensive experience designing, partitioning, indexing, optimizing query execution plans, and managing production PostgreSQL clusters."},
		{"MongoDB", "MongoDB", "6+ years", "Advanced / Expert", "Deep background in document data modeling, aggregation pipelines, replica set failover, and high-volume NoSQL operations."},
		{"Redis", "Redis", "7+ years", "Advanced / Expert", "Production experience implementing distributed caching, session stores, rate limiters, pub/sub messaging, and Redis cluster architectures."},
		{"Kafka", "Apache Kafka", "5+ years", "Advanced / Expert", "Experience designing distributed event-driven architectures, Kafka topics, partitioning, consumer groups, and stream processing."},
		{"NATS", "NATS JetStream", "4+ years", "Advanced / Expert", "Production expertise designing durable publish-subscribe streams, JetStream consumer groups, at-least-once message delivery, and low-latency event fabrics."},
		{"LLM", "Large Language Models", "3+ years", "Advanced / Expert", "Production engineering with frontier LLMs (Gemini, Claude, GPT-4), structured JSON outputs, function calling, and agentic task execution."},
		{"RAG", "Retrieval-Augmented Generation", "3+ years", "Advanced / Expert", "Architecture and implementation of end-to-end RAG retrieval pipelines, semantic chunking, vector embedding generation, and reranking."},
		{"Vector Databases", "Vector DBs", "3+ years", "Advanced / Expert", "Hands-on production experience indexing high-dimensional embeddings, hybrid keyword/vector search, and low-latency similarity retrieval."},
		{"MCP", "Model Context Protocol", "2+ years", "Advanced / Expert", "Extensive experience building and integrating MCP servers, tool definitions, dynamic agent orchestration, and secure external tool calling."},
		{"Autonomous Agents", "AI Agents", "3+ years", "Advanced / Expert", "Design and implementation of resilient autonomous agentic workflows, self-correcting execution loops, tool calling, and human-in-the-loop escalation patterns."},
	}

	var modelsToWrite []mongo.WriteModel
	now := time.Now().UTC()

	addRecord := func(key, orig, ans, qtype, category string) {
		k := strings.ToLower(strings.TrimSpace(key))
		update := bson.M{
			"$set": bson.M{
				"question_key":      k,
				"original_question": orig,
				"answer":            ans,
				"type":              qtype,
				"confidence":        1.0,
				"category":          category,
				"updated_at":        now,
			},
			"$setOnInsert": bson.M{
				"usage_count": 0,
				"created_at":  now,
			},
		}
		modelsToWrite = append(modelsToWrite, mongo.NewUpdateOneModel().SetFilter(bson.M{"question_key": k}).SetUpdate(update).SetUpsert(true))
	}

	// 1. Technical Proficiency & Experience
	for _, t := range techStack {
		tc := strings.ToLower(t.ID)
		patterns := []string{
			fmt.Sprintf("years of experience with %s", tc),
			fmt.Sprintf("how many years of experience do you have with %s", tc),
			fmt.Sprintf("total years of %s experience", tc),
			fmt.Sprintf("how many years of %s", tc),
			fmt.Sprintf("years of %s", tc),
			fmt.Sprintf("%s experience", tc),
			fmt.Sprintf("%s (years)", tc),
			fmt.Sprintf("%s years", tc),
			fmt.Sprintf("experience with %s", tc),
			fmt.Sprintf("describe your experience with %s", tc),
			fmt.Sprintf("tell us about your experience with %s", tc),
			fmt.Sprintf("your experience using %s", tc),
			fmt.Sprintf("proficiency in %s", tc),
			fmt.Sprintf("describe your proficiency in %s", tc),
			fmt.Sprintf("skill level with %s", tc),
			fmt.Sprintf("rate your %s skills", tc),
			fmt.Sprintf("are you proficient in %s", tc),
			fmt.Sprintf("do you have experience with %s", tc),
			fmt.Sprintf("have you worked with %s", tc),
			fmt.Sprintf("have you used %s in production", tc),
		}
		for _, pat := range patterns {
			if strings.Contains(pat, "describe") || strings.Contains(pat, "tell us") || strings.Contains(pat, "using") {
				addRecord(pat, pat, t.Description, "textarea", "technical_proficiency")
			} else if strings.Contains(pat, "proficiency") || strings.Contains(pat, "level") || strings.Contains(pat, "skill") {
				addRecord(pat, pat, t.Proficiency, "select", "technical_proficiency")
			} else if strings.Contains(pat, "years") {
				addRecord(pat, pat, t.Years, "text", "technical_proficiency")
			} else {
				addRecord(pat, pat, "Yes", "radio", "technical_proficiency")
			}
		}
	}

	// 2. Legal & Work Authorization
	legalQuestions := [][]string{
		{"are you legally authorized to work in the united states", "Yes", "radio"},
		{"are you legally authorized to work in the country you are applying for", "Yes", "radio"},
		{"are you eligible to work in the united states", "Yes", "radio"},
		{"are you authorized to work in the us", "Yes", "radio"},
		{"legal work authorization in the us", "Yes", "radio"},
		{"do you have the legal right to work in the united states", "Yes", "radio"},
		{"will you now or in the future require visa sponsorship", "No", "radio"},
		{"will you now or in the future require sponsorship for employment visa status", "No", "radio"},
		{"do you require visa sponsorship", "No", "radio"},
		{"will you require visa sponsorship", "No", "radio"},
		{"will you require sponsorship now or in the future", "No", "radio"},
		{"are you subject to a non-compete agreement", "No", "radio"},
		{"are you bound by any non-compete or restrictive covenants", "No", "radio"},
		{"have you ever been convicted of a felony", "No", "radio"},
		{"do you have any criminal background or felony record", "No", "radio"},
		{"are you 18 years of age or older", "Yes", "radio"},
		{"are you at least 18 years old", "Yes", "radio"},
		{"are you over the age of 18", "Yes", "radio"},
		{"have you previously worked for this company", "No", "radio"},
		{"are you a former employee", "No", "radio"},
		{"do you have any relatives working at this company", "No", "radio"},
		{"do you consent to a standard background check", "Yes", "radio"},
		{"are you willing to undergo background check and drug screening", "Yes", "radio"},
		{"i read this", "I read this", "button"},
		{"please select the option i read this below", "I read this", "button"},
		{"i acknowledge and understand the above statement", "Yes", "button"},
		{"do you agree to the terms and privacy policy", "Yes", "checkbox"},
	}
	for _, q := range legalQuestions {
		addRecord(q[0], q[0], q[1], q[2], "legal_authorization")
	}

	// 3. Location & Office Preferences
	locationQuestions := [][]string{
		{"where are you currently located", "San Francisco, CA", "text"},
		{"current city and state", "San Francisco, CA", "text"},
		{"current address", "San Francisco, CA", "text"},
		{"current residence", "San Francisco, CA", "text"},
		{"city", "San Francisco", "text"},
		{"state", "CA", "text"},
		{"country", "United States", "text"},
		{"zip code", "94105", "text"},
		{"postal code", "94105", "text"},
		{"are you open to remote work", "Yes", "radio"},
		{"are you open to working in a hybrid arrangement", "Yes", "radio"},
		{"are you willing to work out of the san francisco office", "Yes", "radio"},
		{"which office location(s) are you willing & able to work out of", "San Francisco", "pill"},
		{"which lambda office location(s) are you willing & able to work out of 4 days per week", "San Francisco", "pill"},
		{"are you comfortable working 3-4 days per week in the office", "Yes", "radio"},
		{"are you willing to relocate", "Yes", "radio"},
		{"are you open to relocation for this role", "Yes", "radio"},
		{"what percentage of travel are you willing to undertake", "Up to 25%", "select"},
	}
	for _, q := range locationQuestions {
		addRecord(q[0], q[0], q[1], q[2], "location_preference")
	}

	// 4. Compensation, Start Dates & Availability
	compQuestions := [][]string{
		{"what is your expected salary", "150,000-220,000", "text"},
		{"desired salary", "150,000-220,000", "text"},
		{"desired compensation", "150,000-220,000", "text"},
		{"target annual salary", "150,000-220,000", "text"},
		{"expected annual compensation", "150,000-220,000", "text"},
		{"salary expectation", "150,000-220,000", "text"},
		{"salary expectation (numeric)", "185000", "number"},
		{"desired salary (numbers only)", "185000", "number"},
		{"minimum base salary", "150000", "number"},
		{"notice period", "Immediate / 2 weeks", "text"},
		{"how soon can you start", "Immediately / 2 weeks", "text"},
		{"when are you available to start", "Immediately / 2 weeks", "text"},
		{"notice period (in weeks)", "2", "number"},
		{"availability to start", "Immediately / 2 weeks", "text"},
	}
	for _, q := range compQuestions {
		addRecord(q[0], q[0], q[1], q[2], "compensation_timing")
	}

	// 5. Education & Background
	eduQuestions := [][]string{
		{"highest level of education completed", "Master's Degree", "select"},
		{"highest degree earned", "Master's Degree", "select"},
		{"education level", "Master's Degree", "select"},
		{"degree", "Master's Degree", "select"},
		{"major / field of study", "Computer Science", "text"},
		{"university / college attended", "University of the Pacific", "text"},
		{"graduation year", "2018", "text"},
		{"year of graduation", "2018", "text"},
		{"do you have a degree in computer science or related engineering field", "Yes", "radio"},
		{"english proficiency level", "Fluent / Native", "select"},
		{"how proficient are you in english", "Fluent / Native", "select"},
	}
	for _, q := range eduQuestions {
		addRecord(q[0], q[0], q[1], q[2], "education")
	}

	// 6. Behavioral & Architecture (Human-like 4+ line in-depth answers incorporating Nakul Bhandare profile)
	behavioralQuestions := [][]string{
		{
			"why do you want to work here",
			"Throughout my 7+ years of engineering across fintech, payments, and enterprise cloud infrastructure, I have always gravitated toward high-impact teams where reliability, scale, and clean architecture genuinely matter. I am very enthusiastic about this opportunity because your engineering challenges directly align with my track record of architecting large-scale distributed systems and Go microservices from scratch. I describe myself as a calm, cool, and hardworking engineer who thrives on owning complex problems and collaborating with strong technical peers. I am eager to bring my background in system design, performance benchmarking, and production debugging to your team to build rock-solid software that millions of users can rely on.",
			"textarea",
		},
		{
			"why are you interested in this role",
			"This position represents the ideal intersection of high-scale backend systems engineering and complex architectural problem solving. Having designed 12+ microservices from scratch with 100+ integrated APIs for enterprise clients like Kyndryl, I enjoy solving deep technical bottlenecks, modeling data efficiently, and tuning services for ultra-low latency. I am looking for a long-term role where I can tackle demanding industrial challenges alongside talented engineers. I want to build robust software systems that drive measurable business growth while delivering dependable reliability to users every single day.",
			"textarea",
		},
		{
			"what is your proudest technical achievement",
			"My proudest technical achievement was architecting and delivering 12+ microservices from scratch with 100+ production APIs for a major enterprise platform (Kyndryl). Over the project lifecycle, I designed the core distributed data pipelines, integrated 15+ complex customer-facing features, and diagnosed and resolved over 300 intricate production bottlenecks using Go runtime profiling (pprof) and concurrency tuning. This architectural overhaul reduced end-to-end processing latencies by over 40% and directly drove an approximate 15% revenue increase for the business while maintaining 99.95% uptime across millions of daily operations.",
			"textarea",
		},
		{
			"describe a challenging technical project you led",
			"During my work on large-scale distributed systems, I was tasked with designing a mission-critical payment and transaction processing engine that required strict ACID consistency with zero data loss under massive peak load. I architected decoupled event-driven microservices in Go utilizing Kafka topics and Redis distributed caching, carefully designed PostgreSQL database schemas with optimized compound indexing, and established automated observability pipelines. Navigating through intricate concurrency edge cases and rigorous load testing taught me how to engineer defensively and build systems that remain resilient during unexpected infrastructure failures.",
			"textarea",
		},
		{
			"what are your greatest strengths",
			"My greatest strengths lie in my calm and composed problem-solving approach, deep expertise in Go and distributed system design, and rigorous production debugging. I specialize in taking ambiguous, complex engineering challenges and architecting clean, maintainable microservices from the ground up. Whether analyzing intricate data models, optimizing code down to the millisecond, or resolving critical production incidents, I approach all engineering tasks with quiet persistence, hard work, and meticulous attention to detail.",
			"textarea",
		},
		{
			"what is an area of growth or weakness",
			"Early in my career, my intense focus on perfectionism occasionally led me to spend too much time over-optimizing edge-case performance before validating product-market feedback. Over years of leading microservice rollouts, I cultivated a disciplined balance: I now prioritize clear iterative milestones, robust observability, and measurable user value upfront, applying micro-benchmarking and optimizations where metrics demonstrably justify the investment.",
			"textarea",
		},
		{
			"how do you approach conflict resolution within an engineering team",
			"I approach technical disagreements with a calm, composed, and data-driven perspective. When differing opinions emerge regarding architecture, technology choices, or schema design, I focus on separating egos from ideas and instead benchmark alternatives against testable criteria, latency budgets, and long-term maintainability. I actively listen to my peers, create transparent proof-of-concepts to gather real evidence, and foster collaborative alignment so the team moves forward united behind the best solution for our users.",
			"textarea",
		},
		{
			"describe your experience with mentorship and technical leadership",
			"Throughout my career across fintech and enterprise platforms, I have regularly mentored junior and mid-level engineers on idiomatic Go programming, microservice patterns, and production incident response. I lead thorough, constructive code reviews that emphasize readability and clean architecture, and I author comprehensive RFCs and system design documents to ensure organizational alignment. I believe the strongest technical leaders elevate the entire team by creating an encouraging, transparent engineering culture.",
			"textarea",
		},
		{
			"tell us about yourself",
			"I am a Senior Software Engineer with over 7 years of hands-on experience and a Master's degree in Computer Science, specializing in Go, distributed systems, and enterprise cloud architecture. Throughout my career across fintech, payments, and infrastructure, I have specialized in designing microservices from scratch, complex data modeling, and deep production debugging. People describe me as a cool, calm, and hardworking person who stays focused and analytical during high-pressure situations. I am currently seeking an opportunity where I can collaborate with high-caliber engineers, solve meaningful industrial challenges, and build reliable software that millions of users rely upon daily.",
			"textarea",
		},
		{
			"have you contributed to open source software",
			"Yes, I actively contribute to developer tooling, distributed backend systems, and open-source infrastructure projects.",
			"radio",
		},
		{
			"have you owned production database schemas",
			"Yes, extensive experience architecting, indexing, and executing zero-downtime migrations for high-throughput relational and NoSQL production schemas.",
			"radio",
		},
	}
	for _, q := range behavioralQuestions {
		addRecord(q[0], q[0], q[1], q[2], "behavioral_achievement")
	}

	// 7. Demographic & EEO Voluntary Disclosures with Decline / Self-Identify Priorities
	eeoExpandedQuestions := [][]string{
		{"gender identity", "Decline to Self-Identify", "select"},
		{"gender", "Decline to Self-Identify", "select"},
		{"what is your gender", "Decline to Self-Identify", "select"},
		{"gender (self-identify)", "Male", "select"},
		{"gender decline", "Decline to Self-Identify", "select"},
		{"how do you identify your gender", "Decline to Self-Identify", "select"},
		{"race / ethnicity", "Decline to Self-Identify", "select"},
		{"race / ethnic background", "Decline to Self-Identify", "select"},
		{"what is your race or ethnicity", "Decline to Self-Identify", "select"},
		{"race / ethnicity (self-identify)", "Asian", "select"},
		{"are you hispanic or latino", "No", "select"},
		{"hispanic / latino", "Decline to Self-Identify", "select"},
		{"veteran status", "I am not a protected veteran", "select"},
		{"veteran status (opt out)", "I choose not to identify", "select"},
		{"are you a protected veteran", "No", "select"},
		{"disability status", "No, I do not have a disability", "select"},
		{"disability status (opt out)", "I do not wish to answer", "select"},
		{"do you have a disability", "No", "select"},
		{"preferred pronouns", "He/Him", "select"},
		{"pronouns", "He/Him", "select"},
		{"what are your preferred pronouns", "He/Him", "select"},
		{"sexual orientation", "Decline to Self-Identify", "select"},
		{"voluntary self-identification of disability", "I do not wish to answer", "select"},
		{"voluntary self-identification of race/ethnicity", "Decline to Self-Identify", "select"},
		{"voluntary self-identification of veteran status", "I am not a protected veteran", "select"},
		{"do you wish to self-identify", "Decline to Self-Identify", "select"},
	}
	for _, q := range eeoExpandedQuestions {
		addRecord(q[0], q[0], q[1], q[2], "eeo_demographics")
	}

	// 8. Referral & Sourcing Channels (Ashby / SmartRecruiters / Workday)
	sourceQuestions := [][]string{
		{"how did you hear about us", "LinkedIn", "select"},
		{"how did you hear about this job", "LinkedIn", "select"},
		{"how did you hear about this role", "LinkedIn", "select"},
		{"how did you hear about this opportunity", "LinkedIn", "select"},
		{"how did you find this job", "LinkedIn", "select"},
		{"where did you hear about us", "LinkedIn", "select"},
		{"where did you find this job", "LinkedIn", "select"},
		{"referral source", "LinkedIn", "select"},
		{"source", "LinkedIn", "select"},
		{"how did you hear about this position", "LinkedIn", "select"},
		{"application source", "LinkedIn", "select"},
		{"what prompted you to apply", "LinkedIn", "select"},
		{"how did you learn about this opportunity", "LinkedIn", "select"},
		{"how did you find out about this role", "LinkedIn", "select"},
		{"primary source", "LinkedIn", "select"},
		{"recruitment source", "LinkedIn", "select"},
		{"how did you first hear about this opening", "LinkedIn", "select"},
		{"source / referral", "LinkedIn", "select"},
		{"source tracking", "LinkedIn", "select"},
		{"please specify how you heard about us", "LinkedIn", "select"},
	}
	for _, q := range sourceQuestions {
		addRecord(q[0], q[0], q[1], q[2], "source_referral")
	}

	// 9. Enhanced Legal & Work Authorization Nuances
	workAuthNuanceQuestions := [][]string{
		{"are you currently in the us on a visa", "No", "radio"},
		{"are you currently in the united states on a visa", "No", "radio"},
		{"are you currently on a visa", "No", "radio"},
		{"what is your current visa status", "U.S. Citizen / Permanent Resident", "select"},
		{"current us work authorization status", "U.S. Citizen / Permanent Resident", "select"},
		{"current work authorization status", "U.S. Citizen / Permanent Resident", "select"},
		{"are you currently authorized to work for any employer in the united states", "Yes", "radio"},
		{"are you authorized to work lawfully in the united states", "Yes", "radio"},
		{"will you now or in the future require visa sponsorship", "No", "radio"},
		{"will you require sponsorship now or in the future", "No", "radio"},
		{"will you now or in the future require sponsorship for an employment visa", "No", "radio"},
		{"do you require sponsorship to work in the us", "No", "radio"},
		{"do you now or will you in the future require sponsorship for employment visa status (e.g., h-1b, opt, stem opt)", "No", "radio"},
		{"do you require visa sponsorship now or in the future", "No", "radio"},
		{"will you now, or in the future, require company sponsorship to obtain or maintain valid immigration or work status", "No", "radio"},
		{"are you legally permitted to work in the country of this job", "Yes", "radio"},
		{"do you have unrestricted authorization to work in the united states", "Yes", "radio"},
		{"are you currently holding an f-1, opt, stem opt, or j-1 visa", "No", "radio"},
		{"will you require an h-1b transfer or new petition", "No", "radio"},
		{"do you hold valid us citizenship or permanent residency", "Yes", "radio"},
		{"are you a us citizen or green card holder", "Yes", "radio"},
		{"are you eligible to work without visa restriction", "Yes", "radio"},
		{"do you have unrestricted work authorization", "Yes", "radio"},
	}
	for _, q := range workAuthNuanceQuestions {
		addRecord(q[0], q[0], q[1], q[2], "legal_authorization")
	}

	// 10. Structured Notice Period Options (Ashby / SmartRecruiters / Workday)
	noticePeriodQuestions := [][]string{
		{"notice period", "Immediately", "select"},
		{"what is your notice period", "Immediately", "select"},
		{"how much notice do you need to give your current employer", "2 weeks", "select"},
		{"notice period options", "Immediately", "select"},
		{"notice period required", "2 weeks", "select"},
		{"earliest start date", "Immediately", "text"},
		{"when would you be available to begin work", "Immediately", "text"},
		{"when are you able to start", "Immediately", "text"},
		{"availability to begin", "Immediately", "text"},
		{"what is your current availability", "Immediately", "text"},
		{"available start date", "Immediately", "text"},
		{"notice period: immediately / 2 weeks", "Immediately", "select"},
		{"can you start immediately", "Yes", "radio"},
		{"are you available to start immediately", "Yes", "radio"},
		{"standard notice period", "2 weeks", "select"},
		{"current notice period length", "Immediately", "select"},
		{"availability to start work", "Immediately", "select"},
		{"notice period duration", "Immediately", "select"},
	}
	for _, q := range noticePeriodQuestions {
		addRecord(q[0], q[0], q[1], q[2], "compensation_timing")
	}

	// 11. San Francisco Relocation & In-Office Preferences
	relocationQuestions := [][]string{
		{"are you willing to relocate to san francisco", "Yes", "radio"},
		{"are you open to relocating to san francisco, ca", "Yes", "radio"},
		{"are you willing to relocate to san francisco bay area", "Yes", "radio"},
		{"are you willing to relocate to the bay area", "Yes", "radio"},
		{"are you willing to relocate", "Yes", "radio"},
		{"are you willing to relocate for this position", "Yes", "radio"},
		{"willingness to relocate to sf bay area", "Yes", "radio"},
		{"would you be willing to relocate if required", "Yes", "radio"},
		{"relocation assistance required", "No", "radio"},
		{"do you require relocation assistance", "No", "radio"},
		{"do you require relocation support", "No", "radio"},
		{"are you currently located in or willing to relocate to san francisco", "Yes", "radio"},
		{"are you able to commute to san francisco", "Yes", "radio"},
		{"willing to relocate at own expense", "Yes", "radio"},
	}
	for _, q := range relocationQuestions {
		addRecord(q[0], q[0], q[1], q[2], "location_preference")
	}

	// 12. Remote / Hybrid Preferences & Office Cadence
	hybridPreferences := [][]string{
		{"comfortable with hybrid 2-3 days in sf office", "Yes", "radio"},
		{"are you comfortable with a hybrid schedule (2-3 days in office)", "Yes", "radio"},
		{"are you willing to work in a hybrid schedule in san francisco", "Yes", "radio"},
		{"are you comfortable with hybrid 3 days in office", "Yes", "radio"},
		{"are you comfortable working hybrid (2-3 days a week) in our san francisco office", "Yes", "radio"},
		{"are you comfortable coming into the office 2-3 days a week", "Yes", "radio"},
		{"are you willing to work on-site in san francisco 3 days a week", "Yes", "radio"},
		{"this role is hybrid in san francisco. are you able to commit to this", "Yes", "radio"},
		{"preferred work environment", "Hybrid", "select"},
		{"work arrangement preference", "Hybrid", "select"},
		{"are you open to hybrid work", "Yes", "radio"},
		{"are you open to full-time in office", "Yes", "radio"},
		{"are you comfortable working on-site", "Yes", "radio"},
		{"are you comfortable with in-office attendance", "Yes", "radio"},
		{"can you work on-site in our sf headquarters", "Yes", "radio"},
		{"are you willing to work in-office 2-3 days per week", "Yes", "radio"},
		{"are you comfortable with a hybrid work model", "Yes", "radio"},
	}
	for _, q := range hybridPreferences {
		addRecord(q[0], q[0], q[1], q[2], "location_preference")
	}

	fmt.Printf("[Training] Upserting %d high-precision question-answer templates into MongoDB...\n", len(modelsToWrite))
	bRes, err := coll.BulkWrite(ctx, modelsToWrite)
	if err != nil {
		log.Fatalf("BulkWrite error: %v", err)
	}

	count, _ := coll.CountDocuments(ctx, bson.M{})
	fmt.Printf("🧠 [Memory Bank Ready] Upserted: %d, Modified: %d. Total learned questions in persistent memory: %d\n",
		bRes.UpsertedCount, bRes.ModifiedCount, count)
}
