package service

import (
	"fmt"
	"strings"
)

// MasterResume represents the candidate's verified master resume.
type MasterResume struct {
	Name           string
	Title          string
	ContactLine    string
	Email          string
	Phone          string
	Location       string
	LinkedIn       string
	GitHub         string
	Summary        string
	Education      []ResumeEducation
	SkillGroups    []ResumeSkillGroup
	Experience     []ResumeExperience
	Projects       []ResumeProject
	Certifications []string
}

type ResumeEducation struct {
	Degree      string
	Institution string
	Location    string
	Dates       string
}

type ResumeSkillGroup struct {
	Category string
	Skills   string
}

type ResumeExperience struct {
	Title    string
	Company  string
	Location string
	Dates    string
	Bullets  []string
}

type ResumeProject struct {
	Name      string
	TechStack string
	Dates     string
	Bullets   []string
}

// GetNakulMasterResume returns Nakul Bhandare's exact verified master resume from the PDF.
func GetNakulMasterResume() *MasterResume {
	return &MasterResume{
		Name:        "Nakul Bhandare",
		Title:       "Senior Software Engineer",
		Email:       "bhandarenakul11@gmail.com",
		Phone:       "+1 (350)-355-6570",
		Location:    "San Francisco, CA, USA",
		LinkedIn:    "linkedin.com/in/nakul-bhandare",
		GitHub:      "github.com/nakulbhandare",
		ContactLine: "San Francisco, CA, USA | +1 (350)-355-6570 | bhandarenakul11@gmail.com | linkedin.com/in/nakul-bhandare | github.com/nakulbhandare",
		Summary: "Senior Software Engineer with 7 years of experience designing distributed systems, cloud-native platforms, and high-performance backend services in Go. " +
			"Expertise in Kubernetes, Kafka, gRPC, and multi-cloud architectures across AWS, Azure, and Google Cloud, with a strong focus on scalability, " +
			"observability, and performance engineering.",
		Education: []ResumeEducation{
			{
				Degree:      "Master of Science in Computer Science",
				Institution: "University of the Pacific",
				Location:    "Stockton, CA",
				Dates:       "Aug 2024 – May 2026",
			},
			{
				Degree:      "Bachelor of Engineering in Computer Science",
				Institution: "Savitribai Phule Pune University",
				Location:    "Pune, India",
				Dates:       "Jun 2015 – May 2019",
			},
		},
		SkillGroups: []ResumeSkillGroup{
			{
				Category: "Go",
				Skills:   "goroutines, channels, select, sync package, context, interfaces, struct embedding, composition, generics, reflection, closures, defer, panic/recover, type assertions, type switches, error handling, custom errors, errgroup, worker pools, dependency injection, middleware patterns, gRPC, Protocol Buffers, JSON/YAML processing, pprof, race detector, memory profiling, CPU profiling, runtime tuning, garbage collection, escape analysis, benchmark testing, table-driven testing, concurrency patterns, design patterns, Go modules, package design, REST APIs, Code Architecture",
			},
			{
				Category: "System Design",
				Skills:   "distributed systems, scalability, high availability, fault tolerance, load balancing, caching strategies, event-driven architecture, CAP theorem, rate limiting, circuit breakers, message queuing (Kafka, RabbitMQ)",
			},
			{
				Category: "Database & Data Modeling",
				Skills:   "schema design, indexing strategies, query optimization, normalization, sharding, replication, ACID transactions, connection pooling, database migrations",
			},
			{
				Category: "Software Architecture",
				Skills:   "microservices architecture, domain-driven design, clean/hexagonal architecture, API design, service mesh, API gateway design, design patterns",
			},
			{
				Category: "Solution Engineering",
				Skills:   "requirement analysis, solution design, proof-of-concept development, technical documentation, stakeholder communication, vendor/tool evaluation, cost-benefit analysis",
			},
			{
				Category: "Cloud & Infrastructure",
				Skills:   "AWS, Azure, Google Cloud, Kubernetes, Docker, Helm, Terraform, Argo CD, GitHub Actions",
			},
			{
				Category: "Data & Observability",
				Skills:   "PostgreSQL, MongoDB, Redis, DynamoDB, Cassandra, Prometheus, Grafana, Jaeger, ELK Stack",
			},
			{
				Category: "Performance & Tooling",
				Skills:   "pprof, race detector, memory/CPU profiling, benchmark & table-driven testing, garbage collection tuning",
			},
			{
				Category: "AI Tooling & Automation",
				Skills:   "AI-assisted development, LLM-powered automation, agentic workflows, prompt engineering, developer productivity tooling",
			},
			{
				Category: "Other Languages",
				Skills:   "Python, Java, TypeScript, C#, .NET, SQL",
			},
		},
		Experience: []ResumeExperience{
			{
				Title:    "Research Assistant (Graduate Research, part-time)",
				Company:  "University of the Pacific",
				Location: "Stockton, CA",
				Dates:    "Jan 2026 – May 2026",
				Bullets: []string{
					"Eliminated 10+ recurring heap-saturation failures through pprof-driven concurrency and memory analysis under sustained runtime workloads.",
					"Increased throughput by 12% by optimizing 4 Go garbage collector configurations across synthetic stress-testing environments.",
					"Added a benchmarking framework evaluating latency, memory usage, and throughput across 20+ workload scenarios and runtime profiles.",
				},
			},
			{
				Title:    "Software Engineer Intern (Graduate Internship)",
				Company:  "Toricents Laboratory",
				Location: "Remote",
				Dates:    "Jul 2025 – Dec 2025",
				Bullets: []string{
					"Delivered 5 Go microservices using Gin, typed contracts, and JWT authentication, enabling secure deployment across Kubernetes clusters.",
					"Accelerated build execution time by 20% by designing concurrent dispatch pipelines with buffered channels, goroutines, and wait groups.",
					"Implemented structured logging, config management, and health checks across 30+ microservices, shrinking incident response time by 35%.",
				},
			},
			{
				Title:    "Senior Software Engineer",
				Company:  "GS Lab",
				Location: "Pune, India",
				Dates:    "Jun 2022 – Aug 2024",
				Bullets: []string{
					"Optimized Go services handling 50,000+ daily requests, reducing latency by 40% through performance profiling and runtime tuning.",
					"Architected a multi-cloud Go adapter integrating AWS, Azure, and GCP storage services, trimming latency by 45%.",
					"Standardized observability across 15+ services using Prometheus and Jaeger, shrinking mean-time-to-detection by 50% during incidents.",
					"Designed Kyndryl Bridge, a Go discovery engine unifying 500+ cloud services, cutting comparison time by 40% via a JSON-diff framework.",
				},
			},
			{
				Title:    "Software Engineer",
				Company:  "GS Lab",
				Location: "Pune, India",
				Dates:    "Aug 2019 – Jun 2022",
				Bullets: []string{
					"Prevented 30+ cascading failures by strengthening concurrent cache layers using synchronization primitives, context propagation, and timeouts.",
					"Increased service throughput by 22% through gRPC migration of 100+ critical REST endpoints, significantly reducing serialization costs.",
					"Slashed PostgreSQL query latency from 800 ms to 90 ms across 12 critical workloads, improving system responsiveness.",
					"Built an asynchronous Go monitoring service over AWS Lambda, SQS, SNS, S3, and CloudTrail, cutting telemetry lag by 25%.",
				},
			},
		},
		Projects: []ResumeProject{
			{
				Name:      "PayPro",
				TechStack: "Go, Kafka, Redis, PostgreSQL, Kubernetes",
				Dates:     "Feb 2026 – Present",
				Bullets: []string{
					"Orchestrated a reliable, multi-tenant Go payment platform processing 1,000 transactions per second across 100+ broker environments.",
					"Architected a saga-based transaction orchestration pattern, ensuring consistency across 5+ downstream payment services.",
					"Built a React merchant dashboard for real-time transaction monitoring across 100+ broker accounts.",
					"Developed an internal admin console for dispute resolution and manual review, cutting review time by 40%.",
					"Designed a partitioned PostgreSQL schema for transaction records, supporting 10M+ rows with sub-20 ms query latency.",
					"Implemented Redis-backed rate-limit counters and request-deduplication caching, cutting duplicate-check latency to under 5 ms.",
					"Deployed the platform on Kubernetes with horizontal autoscaling across 3 availability zones, supporting 250+ concurrent broker connections.",
					"Built a GitOps CI/CD pipeline with Argo CD, cutting release cycles from 2 days to 3 hours.",
					"Implemented canary deployments with automatic rollback triggers, reducing production incident rate by 60%.",
					"Achieved sub-100 ms latency and 99.95% uptime through Kafka-driven event processing and Redis-backed distributed caching.",
					"Prevented duplicate charges via idempotency controls and HMAC-signed webhooks across 250,000+ transactions.",
					"Resolved intermittent webhook delivery failures during broker outages with retry-and-replay queues, recovering 98.5% of missed events.",
				},
			},
			{
				Name:      "Order Processing System",
				TechStack: "Go, Kafka, Kubernetes, MongoDB, Redis",
				Dates:     "Jan 2025 – May 2025",
				Bullets: []string{
					"Built a React dashboard for real-time order tracking and a MongoDB schema with compound indexes, cutting query time by 45% and tickets by 30%.",
					"Added a mobile-responsive order-status widget to the customer-facing UI, lifting self-service order lookups by 25%.",
					"Implemented sharded MongoDB collections for 1M+ records and architected an event-driven design decoupling intake, payment, and fulfillment.",
					"Designed a fault-tolerant pipeline with dead-letter queues and deployed on AWS EKS with auto-scaling, cutting infra cost by 20%.",
					"Set up a GitHub Actions CI/CD pipeline and containerized the service on Kubernetes, cutting deployment time from 45 to 6 minutes.",
					"Processed 500,000+ orders with zero critical incidents while fixing out-of-sequence updates via partition-key-based Kafka routing.",
					"Resolved flash-sale traffic spikes with backpressure and rate-limiting, cutting error rates from 12% to under 0.5%.",
				},
			},
			{
				Name:      "Kyndryl Bridge (KCMP)",
				TechStack: "Go, MongoDB, REST APIs, Multi-Cloud, JSON",
				Dates:     "Mar 2021 – Aug 2024",
				Bullets: []string{
					"Built an internal drift-visualization dashboard and optimized MongoDB indexing, cutting query latency from 250 ms to 60 ms.",
					"Added interactive filtering and drill-down views to the discovery dashboard, cutting average investigation time by 28%.",
					"Engineered a normalized MongoDB schema for multi-cloud metadata and a plugin-based framework, trimming connector onboarding time by 30%.",
					"Designed a distributed caching layer cutting redundant API calls by 35% and deployed the platform across AWS and Azure for unification.",
					"Automated releases with a CI/CD pipeline and rolled out blue-green deployments, cutting deploy time by 50% with zero downtime.",
					"Adopted as the standard multi-cloud discovery tool across 3 enterprise engagements spanning 500+ integrated services.",
					"Resolved inconsistent provider rate limits via adaptive throttling and corrected inventory drift through scheduled reconciliation, reaching 99.9% accuracy.",
				},
			},
			{
				Name:      "IBM Real-Time Cloud Discovery",
				TechStack: "Go, AWS (Lambda, SQS, SNS, S3, CloudTrail), CloudFormation",
				Dates:     "Aug 2019 – Jun 2022",
				Bullets: []string{
					"Built a monitoring dashboard and modeled DynamoDB tables for telemetry events, supporting 10,000+ writes per minute.",
					"Introduced customizable alert widgets on the monitoring dashboard, reducing alert triage time by 20%.",
					"Optimized S3 storage with lifecycle policies and architected a serverless pipeline via Lambda and SNS/SQS, cutting cost by 30%.",
					"Engineered a fan-out notification system and leveraged CloudTrail and Config, routing alerts to 15+ downstream consumers in real time.",
					"Deployed infrastructure-as-code via CloudFormation and added automated rollback, cutting recovery time by 70%.",
					"Enabled real-time visibility into 1,000+ cloud resources, cutting mean-time-to-detection from 2 hours to 5 minutes.",
					"Centralized discovery across 12+ AWS accounts, eliminating 8+ hours of manual inventory reconciliation weekly.",
				},
			},
		},
		Certifications: []string{
			"Google Cloud Professional Developer",
			"AWS Cloud Practitioner",
			"Database Management and IoT",
		},
	}
}

// Tailor creates a new MasterResume specifically targeted to the JD while strictly keeping the exact visual 1-page layout.
func (r *MasterResume) Tailor(jobTitle, companyName string, matchResult JobMatchResult, jdText ...string) *MasterResume {
	tailored := *r

	// Combine matched skills, missing skills, and keywords from the job description
	seenSkills := make(map[string]bool)
	var allTargetSkills []string
	addSkill := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" && !seenSkills[strings.ToLower(s)] {
			seenSkills[strings.ToLower(s)] = true
			allTargetSkills = append(allTargetSkills, s)
		}
	}
	for _, s := range matchResult.MatchedSkills {
		addSkill(s)
	}
	for _, s := range matchResult.SkillFit.MissingHave {
		addSkill(s)
	}
	for _, s := range matchResult.TailorKeywords {
		addSkill(s)
	}
	for _, s := range matchResult.MissingSkills {
		addSkill(s)
	}

	// 1. Tailor the Professional Summary with job title and matching keywords
	topSkillsStr := "Kubernetes, Kafka, gRPC, and multi-cloud architectures across AWS, Azure, and Google Cloud"
	if len(allTargetSkills) >= 3 {
		topSkillsStr = strings.Join(allTargetSkills[:min(5, len(allTargetSkills))], ", ") + " systems"
	}
	roleTarget := jobTitle
	if roleTarget == "" {
		roleTarget = "Senior Software Engineer"
	}

	tailored.Summary = fmt.Sprintf("Senior Software Engineer with 7 years of experience designing distributed systems, cloud-native platforms, and high-performance backend services in Go for roles like %s. Expertise in %s, with a strong focus on scalability, observability, and performance engineering.", roleTarget, topSkillsStr)

	// 2. Re-prioritize Skill Groups based on matchResult keywords without expanding line height
	if len(allTargetSkills) > 0 {
		var prioritized []ResumeSkillGroup
		var remaining []ResumeSkillGroup

		for _, group := range r.SkillGroups {
			matchedGroup := false
			for _, skill := range allTargetSkills {
				if strings.Contains(strings.ToLower(group.Category), strings.ToLower(skill)) || strings.Contains(strings.ToLower(group.Skills), strings.ToLower(skill)) {
					matchedGroup = true
					break
				}
			}
			if matchedGroup {
				prioritized = append(prioritized, group)
			} else {
				remaining = append(remaining, group)
			}
		}
		tailored.SkillGroups = append(prioritized, remaining...)
	}

	// 3. Strictly preserve 1-page constraints: exact copy of experiences and projects with identical bullet counts
	tailored.Experience = make([]ResumeExperience, len(r.Experience))
	for i, exp := range r.Experience {
		tailored.Experience[i] = ResumeExperience{
			Title:    exp.Title,
			Company:  exp.Company,
			Location: exp.Location,
			Dates:    exp.Dates,
			Bullets:  append([]string{}, exp.Bullets...),
		}
	}

	tailored.Projects = make([]ResumeProject, len(r.Projects))
	for i, proj := range r.Projects {
		tailored.Projects[i] = ResumeProject{
			Name:      proj.Name,
			TechStack: proj.TechStack,
			Dates:     proj.Dates,
			Bullets:   append([]string{}, proj.Bullets...),
		}
	}

	return &tailored
}

// GenerateCoverLetter crafts a high-signal, metrics-driven cover letter tailored to the target role.
func (r *MasterResume) GenerateCoverLetter(jobTitle, companyName string, match JobMatchResult) string {
	role := jobTitle
	if role == "" {
		role = "Senior Software Engineer"
	}
	company := companyName
	if company == "" {
		company = "your engineering team"
	}

	topSkills := "Go, distributed systems, and cloud infrastructure"
	if len(match.MatchedSkills) >= 3 {
		topSkills = strings.Join(match.MatchedSkills[:min(4, len(match.MatchedSkills))], ", ")
	}

	return fmt.Sprintf(`Dear %s Hiring Team,

I am writing to express my strong interest in the %s position at %s. With over 7 years of engineering experience architecting high-throughput distributed systems, scalable microservices, and multi-cloud platforms in Go, I have consistently focused on building reliable, production-grade infrastructure that drives business results.

Throughout my career at GS Lab and advanced graduate research at University of the Pacific, my work has centered on the core technical competencies critical to this role, specifically in %s:

• High-Performance Backend Engineering: Spearheaded Go microservices processing 50,000+ daily requests, cutting latency by 40%% through deep runtime profiling (pprof) and memory allocation tuning.
• Cloud & Platform Infrastructure: Engineered multi-cloud discovery engines across AWS, Azure, and Google Cloud, unifying 500+ services and cutting telemetry latency across Kubernetes clusters.
• Event-Driven Scalability: Architected resilient payment and event-driven order processing pipelines using Go, Kafka, and Redis, maintaining 99.95%% uptime and processing 1,000 TPS with zero transaction loss.

What excites me most about %s is your commitment to engineering excellence and scalable developer tooling. I thrive in collaborative, high-impact environments where I can take complete ownership of complex technical challenges from design through production rollout.

Thank you for your time and consideration. I welcome the opportunity to discuss how my technical background and problem-solving approach can make an immediate contribution to your team.

Sincerely,
%s
%s | %s | %s`, company, role, company, topSkills, company, r.Name, r.Email, r.Phone, r.LinkedIn)
}

// RenderHTML returns the pixel-perfect HTML/CSS matching the uploaded PDF format.
func (r *MasterResume) RenderHTML() string {
	var b strings.Builder

	b.WriteString(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<style>
  @page { size: letter; margin: 0.45in 0.5in; }
  body {
    font-family: 'Times New Roman', Times, serif;
    font-size: 9.5pt;
    line-height: 1.25;
    color: #000;
    margin: 0;
    padding: 0;
  }
  a { color: #0044cc; text-decoration: none; }
  .header { text-align: center; margin-bottom: 6pt; }
  .header h1 { font-size: 19pt; font-weight: bold; margin: 0 0 2pt 0; letter-spacing: -0.2px; }
  .header .sub { font-size: 11pt; margin: 0 0 3pt 0; }
  .header .contact { font-size: 9pt; color: #111; }
  .section-title {
    font-size: 10.5pt;
    font-weight: bold;
    text-transform: uppercase;
    margin-top: 6pt;
    margin-bottom: 2pt;
    border-bottom: 0.75pt solid #000;
    padding-bottom: 1pt;
    letter-spacing: 0.5px;
  }
  .entry-row {
    display: flex;
    justify-content: space-between;
    margin-top: 3.5pt;
    margin-bottom: 1pt;
    font-size: 9.5pt;
  }
  .entry-left { font-weight: normal; }
  .entry-left b { font-weight: bold; }
  .entry-right { font-weight: bold; text-align: right; white-space: nowrap; }
  ul {
    margin: 1.5pt 0 4pt 14pt;
    padding: 0;
    list-style-type: disc;
  }
  li {
    margin-bottom: 1.5pt;
    text-align: justify;
  }
  .skills-p {
    margin: 2pt 0;
    text-align: justify;
  }
  .skills-p b { font-weight: bold; }
</style>
</head>
<body>
`)

	// Header
	b.WriteString(fmt.Sprintf(`
<div class="header">
  <h1>%s</h1>
  <div class="sub">%s</div>
  <div class="contact">
    %s | %s | <a href="mailto:%s">%s</a> | <a href="https://%s">%s</a> | <a href="https://%s">%s</a>
  </div>
</div>
`, r.Name, r.Title, r.Location, r.Phone, r.Email, r.Email, r.LinkedIn, r.LinkedIn, r.GitHub, r.GitHub))

	// Summary
	b.WriteString(`
<div class="section-title">SUMMARY</div>
<p style="margin: 3pt 0 5pt 0; text-align: justify;">` + r.Summary + `</p>
`)

	// Education
	b.WriteString(`<div class="section-title">EDUCATION</div>` + "\n")
	for _, edu := range r.Education {
		b.WriteString(fmt.Sprintf(`
<div class="entry-row">
  <div class="entry-left"><b>%s</b> | %s, %s</div>
  <div class="entry-right">%s</div>
</div>
`, edu.Degree, edu.Institution, edu.Location, edu.Dates))
	}

	// Technical Skills
	b.WriteString(`<div class="section-title">TECHNICAL SKILLS</div>` + "\n")
	for _, g := range r.SkillGroups {
		b.WriteString(fmt.Sprintf(`<p class="skills-p"><b>%s:</b> %s</p>`+"\n", g.Category, g.Skills))
	}

	// Experience
	b.WriteString(`<div class="section-title">EXPERIENCE</div>` + "\n")
	for _, exp := range r.Experience {
		b.WriteString(fmt.Sprintf(`
<div class="entry-row">
  <div class="entry-left"><b>%s</b> | %s, %s</div>
  <div class="entry-right">%s</div>
</div>
<ul>
`, exp.Title, exp.Company, exp.Location, exp.Dates))
		for _, bullet := range exp.Bullets {
			b.WriteString(fmt.Sprintf("  <li>%s</li>\n", bullet))
		}
		b.WriteString("</ul>\n")
	}

	// Personal Projects
	b.WriteString(`<div class="section-title">PERSONAL PROJECTS</div>` + "\n")
	for _, proj := range r.Projects {
		b.WriteString(fmt.Sprintf(`
<div class="entry-row">
  <div class="entry-left"><b>%s</b> | %s</div>
  <div class="entry-right">%s</div>
</div>
<ul>
`, proj.Name, proj.TechStack, proj.Dates))
		for _, bullet := range proj.Bullets {
			b.WriteString(fmt.Sprintf("  <li>%s</li>\n", bullet))
		}
		b.WriteString("</ul>\n")
	}

	// Certifications
	b.WriteString(`<div class="section-title">CERTIFICATIONS</div>` + "\n")
	b.WriteString(fmt.Sprintf(`<div style="margin-top: 2pt;">%s</div>`+"\n", strings.Join(r.Certifications, " | ")))

	b.WriteString("</body>\n</html>\n")
	return b.String()
}

// RenderText produces clean, machine-parsable plain text for ATS parsers.
func (r *MasterResume) RenderText() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s\n%s\n%s\n\n", r.Name, r.Title, r.ContactLine))
	b.WriteString(fmt.Sprintf("SUMMARY\n%s\n\n", r.Summary))
	b.WriteString("EDUCATION\n")
	for _, edu := range r.Education {
		b.WriteString(fmt.Sprintf("%s | %s, %s   %s\n", edu.Degree, edu.Institution, edu.Location, edu.Dates))
	}
	b.WriteString("\nTECHNICAL SKILLS\n")
	for _, g := range r.SkillGroups {
		b.WriteString(fmt.Sprintf("%s: %s\n", g.Category, g.Skills))
	}
	b.WriteString("\nEXPERIENCE\n")
	for _, exp := range r.Experience {
		b.WriteString(fmt.Sprintf("%s | %s, %s   %s\n", exp.Title, exp.Company, exp.Location, exp.Dates))
		for _, bullet := range exp.Bullets {
			b.WriteString(fmt.Sprintf("• %s\n", bullet))
		}
	}
	b.WriteString("\nPERSONAL PROJECTS\n")
	for _, proj := range r.Projects {
		b.WriteString(fmt.Sprintf("%s | %s   %s\n", proj.Name, proj.TechStack, proj.Dates))
		for _, bullet := range proj.Bullets {
			b.WriteString(fmt.Sprintf("• %s\n", bullet))
		}
	}
	b.WriteString("\nCERTIFICATIONS\n")
	b.WriteString(strings.Join(r.Certifications, " | ") + "\n")
	return b.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
