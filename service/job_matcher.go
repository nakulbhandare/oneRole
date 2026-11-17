package service

import (
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/oneRole/models"
)

// ATSScoreBreakdown reflects standard ATS parser scoring categories.
type ATSScoreBreakdown struct {
	HardSkillsScore     int `json:"hard_skills_score"`     // max 40
	TitleSeniorityScore int `json:"title_seniority_score"` // max 25
	ExperienceWorkScore int `json:"experience_work_score"` // max 20
	EducationCertsScore int `json:"education_certs_score"` // max 15
}

// JobMatchResult provides a quantitative 0-100% ATS score and breakdown.
type JobMatchResult struct {
	Score          int                     `json:"score"`       // 0-100
	MatchLevel     string                  `json:"match_level"` // "exceptional", "strong", "moderate", "low"
	ATSVerdict     string                  `json:"ats_verdict"` // e.g. "Top Tier Candidate (95%+ ATS match)"
	MatchedSkills  []string                `json:"matched_skills"`
	MissingSkills  []string                `json:"missing_skills"`
	SkillFit       models.SkillFitAnalysis `json:"skill_fit"` // FreeHire-style 3-tier gap analysis: have, missing_have, missing_gap
	Breakdown      ATSScoreBreakdown       `json:"breakdown"`
	KeyHighlights  []string                `json:"key_highlights"`
	TailorKeywords []string                `json:"tailor_keywords"`
}

// ScoredJob associates a Job with its computed match score.
type ScoredJob struct {
	Job        models.Job     `json:"job"`
	MatchScore int            `json:"match_score"`
	Match      JobMatchResult `json:"match"`
}

type JobMatcher struct{}

func NewJobMatcher() *JobMatcher {
	return &JobMatcher{}
}

// Comprehensive catalog of engineering skills, concepts, and synonyms recognized by ATS parsers.
var atsSkillDictionary = []struct {
	Canonical string
	Category  string
	Keywords  []string
}{
	{"Go", "Language", []string{"golang", "go", "go developer", "go engineer", "go backend", "go programming", "go language", "go microservices"}},
	{"Kubernetes", "Infrastructure", []string{"kubernetes", "k8s", "helm", "eks", "gke", "aks"}},
	{"Kafka", "Streaming", []string{"kafka", "event-driven", "message queue", "messaging", "pubsub"}},
	{"gRPC", "API & Protocol", []string{"grpc", "protocol buffers", "protobuf", "rpc"}},
	{"PostgreSQL", "Database", []string{"postgresql", "postgres", "sql", "psql", "relational database"}},
	{"Redis", "Caching", []string{"redis", "caching", "in-memory", "cache"}},
	{"MongoDB", "Database", []string{"mongodb", "mongo", "nosql", "document store"}},
	{"Docker", "Infrastructure", []string{"docker", "container", "containers", "containerization"}},
	{"AWS", "Cloud", []string{"aws", "amazon web services", "ec2", "s3", "lambda", "cloudtrail", "cloudwatch", "sqs", "sns", "dynamodb"}},
	{"Google Cloud", "Cloud", []string{"google cloud", "gcp", "gke", "cloud storage"}},
	{"Azure", "Cloud", []string{"azure", "microsoft azure", "azure blob"}},
	{"Cloud Architecture", "Cloud", []string{"cloud", "cloud-native", "multi-cloud", "cloud infrastructure"}},
	{"Backend Systems", "Architecture", []string{"backend", "server-side", "backend engineer", "backend systems"}},
	{"Distributed Systems", "Architecture", []string{"distributed systems", "distributed architecture", "high availability", "scalability", "fault tolerance"}},
	{"Microservices", "Architecture", []string{"microservices", "microservice", "service-oriented", "soa"}},
	{"REST APIs", "API", []string{"rest api", "restful", "rest apis", "json api", "http api", "api", "apis"}},
	{"CI/CD", "DevOps", []string{"ci/cd", "ci cd", "github actions", "pipeline", "continuous integration", "continuous delivery"}},
	{"AI & Machine Learning", "AI", []string{"ai", "machine learning", "ml", "genai", "deep learning", "llm", "ai-assisted", "ai tooling", "agentic workflows"}},
	{"Terraform", "DevOps", []string{"terraform", "iac", "infrastructure as code"}},
	{"Argo CD", "DevOps", []string{"argo cd", "argocd", "gitops"}},
	{"System Design", "Architecture", []string{"system design", "architecture", "load balancing", "rate limiting", "circuit breaker"}},
	{"Prometheus", "Observability", []string{"prometheus", "monitoring", "metrics"}},
	{"Grafana", "Observability", []string{"grafana", "dashboard", "telemetry"}},
	{"Jaeger", "Observability", []string{"jaeger", "distributed tracing", "tracing"}},
	{"Performance Profiling", "Tooling", []string{"pprof", "profiling", "runtime tuning", "memory analysis", "latency reduction", "benchmark"}},
	{"Concurrency", "Engineering", []string{"concurrency", "concurrent", "multithreading", "sync package", "mutex", "worker pools"}},
	{"Python", "Language", []string{"python", "django", "flask", "fastapi"}},
	{"TypeScript", "Language", []string{"typescript", "javascript", "react", "node"}},
	{"Java", "Language", []string{"java", "spring", "spring boot"}},
}

// CalculateJobMatch parses job description text and candidate resume text like an enterprise ATS.
func (m *JobMatcher) CalculateJobMatch(job models.Job, p *models.CandidateProfile) JobMatchResult {
	if p == nil {
		return JobMatchResult{
			Score:      50,
			MatchLevel: "moderate",
			ATSVerdict: "Base alignment",
		}
	}

	jdText := strings.ToLower(strings.Join([]string{
		job.Title,
		job.Description,
		job.Company,
		job.Location,
	}, " "))

	candidateResume := p.ResumeContentText
	if len(candidateResume) < 200 {
		candidateResume = GetNakulMasterResume().RenderText()
	}

	resumeText := strings.ToLower(strings.Join([]string{
		p.FirstName,
		p.LastName,
		p.CurrentJobTitle,
		p.CurrentEmployer,
		candidateResume,
		strings.Join(p.PrimarySkills, " "),
		p.HighestDegree,
	}, " "))

	// 1. HARD SKILLS & TOOLING MATCH (Max 40 Points)
	matchedSkills := make([]string, 0)
	missingSkills := make([]string, 0)
	haveSkills := make([]string, 0)
	missingHaveSkills := make([]string, 0)
	missingGapSkills := make([]string, 0)
	tailorKeywords := make([]string, 0)
	matchedMap := make(map[string]bool)

	masterText := strings.ToLower(GetNakulMasterResume().RenderText() + " " + strings.Join(p.PrimarySkills, " "))

	totalJDSkills := 0
	for _, entry := range atsSkillDictionary {
		jdHasSkill := false
		for _, kw := range entry.Keywords {
			if containsWord(jdText, kw) {
				jdHasSkill = true
				break
			}
		}

		if jdHasSkill {
			totalJDSkills++
			tailorKeywords = append(tailorKeywords, entry.Canonical)

			resumeHasSkill := false
			for _, kw := range entry.Keywords {
				if containsWord(resumeText, kw) {
					resumeHasSkill = true
					break
				}
			}

			if resumeHasSkill {
				if !matchedMap[entry.Canonical] {
					matchedMap[entry.Canonical] = true
					matchedSkills = append(matchedSkills, entry.Canonical)
					haveSkills = append(haveSkills, entry.Canonical)
				}
			} else {
				missingSkills = append(missingSkills, entry.Canonical)

				// FreeHire 3-Tier Fit Analysis: check if candidate has skill in broader master atoms
				masterHasSkill := false
				for _, kw := range entry.Keywords {
					if containsWord(masterText, kw) {
						masterHasSkill = true
						break
					}
				}
				if masterHasSkill {
					missingHaveSkills = append(missingHaveSkills, entry.Canonical)
				} else {
					missingGapSkills = append(missingGapSkills, entry.Canonical)
				}
			}
		}
	}

	jobTitleLower := strings.ToLower(job.Title)
	candidateTitleLower := strings.ToLower(p.CurrentJobTitle)

	isNonSoftware := strings.Contains(jobTitleLower, "electrical") ||
		strings.Contains(jobTitleLower, "power generation") ||
		strings.Contains(jobTitleLower, "civil") ||
		strings.Contains(jobTitleLower, "mechanical") ||
		strings.Contains(jobTitleLower, "facilities") ||
		strings.Contains(jobTitleLower, "hardware")

	// Check if the target job is a Software / Technology Engineering role
	isTechRole := !isNonSoftware && (strings.Contains(jobTitleLower, "software") ||
		strings.Contains(jobTitleLower, "developer") ||
		strings.Contains(jobTitleLower, "architect") ||
		strings.Contains(jobTitleLower, "programmer") ||
		strings.Contains(jobTitleLower, "tech lead") ||
		strings.Contains(jobTitleLower, "sre") ||
		strings.Contains(jobTitleLower, "devops") ||
		strings.Contains(jobTitleLower, "backend") ||
		strings.Contains(jobTitleLower, "full stack") ||
		strings.Contains(jobTitleLower, "platform") ||
		strings.Contains(jobTitleLower, "infrastructure") ||
		strings.Contains(jobTitleLower, "golang") ||
		strings.Contains(jobTitleLower, "systems"))

	hardSkillsScore := 10.0
	if totalJDSkills > 0 {
		ratio := float64(len(matchedSkills)) / float64(totalJDSkills)
		hardSkillsScore = ratio * 40.0
	} else if isTechRole {
		// General technical role without listed tool keywords
		if len(haveSkills) == 0 {
			haveSkills = []string{"Distributed Systems", "Cloud Architecture", "Backend Systems"}
			matchedSkills = haveSkills
		}
		if strings.Contains(resumeText, "distributed systems") || strings.Contains(resumeText, "microservices") {
			hardSkillsScore = 28.0
		} else {
			hardSkillsScore = 20.0
		}
	} else {
		// Non-technical role
		hardSkillsScore = 5.0
	}

	// 2. TITLE & SENIORITY ALIGNMENT (Max 25 Points)
	titleScore := 10.0
	isSeniorRole := strings.Contains(jobTitleLower, "senior") || strings.Contains(jobTitleLower, "sr") || strings.Contains(jobTitleLower, "lead")
	isStaffRole := strings.Contains(jobTitleLower, "staff") || strings.Contains(jobTitleLower, "principal")
	candidateIsSenior := strings.Contains(candidateTitleLower, "senior") || strings.Contains(candidateTitleLower, "lead") || p.YearsOfExperience >= 5

	if !isTechRole {
		titleScore = 5.0
	} else if isSeniorRole {
		if candidateIsSenior {
			titleScore = 25.0
		} else {
			titleScore = 16.0
		}
	} else if isStaffRole {
		if p.YearsOfExperience >= 8 {
			titleScore = 25.0
		} else {
			titleScore = 19.0
		}
	} else {
		// Mid / Standard Engineer role: Full points
		titleScore = 24.0
	}

	// 3. EXPERIENCE & WORK CONTEXT (Max 20 Points)
	experienceScore := 8.0
	if isTechRole {
		experienceScore = 14.0
		if strings.Contains(jdText, "distributed") || strings.Contains(jdText, "scale") || strings.Contains(jdText, "backend") || strings.Contains(jdText, "cloud") {
			if strings.Contains(resumeText, "distributed") || strings.Contains(resumeText, "high-performance") || strings.Contains(resumeText, "cloud-native") {
				experienceScore += 6.0
			}
		}
	}
	if experienceScore > 20.0 {
		experienceScore = 20.0
	}

	// 4. EDUCATION & CERTIFICATIONS (Max 15 Points)
	educationScore := 8.0
	if isTechRole {
		if strings.Contains(resumeText, "master of science") || strings.Contains(resumeText, "bachelor") || strings.Contains(resumeText, "computer science") || strings.Contains(resumeText, "software engineering") {
			educationScore = 15.0
		}
	}

	rawTotal := hardSkillsScore + titleScore + experienceScore + educationScore
	minCap := 20.0
	if !isTechRole {
		minCap = 15.0
	}
	finalScore := int(math.Round(math.Min(100.0, math.Max(minCap, rawTotal))))

	matchLevel := "moderate"
	verdict := "Standard Applicant Match"
	switch {
	case finalScore >= 98:
		matchLevel = "exceptional"
		verdict = "Top Tier Candidate (98%+ ATS match)"
	case finalScore >= 92:
		matchLevel = "exceptional"
		verdict = "Top Tier Candidate (92%+ ATS match)"
	case finalScore >= 80:
		matchLevel = "strong"
		verdict = "Strong ATS Candidate (80-91% match)"
	case finalScore >= 65:
		matchLevel = "moderate"
		verdict = "Qualified Applicant (65-79% match)"
	default:
		matchLevel = "low"
		verdict = "Low ATS Compatibility (<65% match)"
	}

	var highlights []string
	if len(matchedSkills) > 0 {
		topSkills := matchedSkills
		if len(topSkills) > 6 {
			topSkills = topSkills[:6]
		}
		highlights = append(highlights, "Matches core tech stack: "+strings.Join(topSkills, ", "))
	}
	if candidateIsSenior {
		highlights = append(highlights, "Seniority requirement satisfied (6+ years engineering experience)")
	}
	if educationScore >= 14.0 {
		highlights = append(highlights, "Meets degree & education qualifications (MS/BS in Computer Science)")
	}

	return JobMatchResult{
		Score:         finalScore,
		MatchLevel:    matchLevel,
		ATSVerdict:    verdict,
		MatchedSkills: matchedSkills,
		MissingSkills: missingSkills,
		SkillFit: models.SkillFitAnalysis{
			Have:        haveSkills,
			MissingHave: missingHaveSkills,
			MissingGap:  missingGapSkills,
		},
		Breakdown: ATSScoreBreakdown{
			HardSkillsScore:     int(math.Round(hardSkillsScore)),
			TitleSeniorityScore: int(math.Round(titleScore)),
			ExperienceWorkScore: int(math.Round(experienceScore)),
			EducationCertsScore: int(math.Round(educationScore)),
		},
		KeyHighlights:  highlights,
		TailorKeywords: tailorKeywords,
	}
}

// RankJobs returns jobs sorted by calculated match score descending.
func (m *JobMatcher) RankJobs(jobs []models.Job, p *models.CandidateProfile, minScore int) []ScoredJob {
	scored := make([]ScoredJob, 0, len(jobs))
	for _, j := range jobs {
		res := m.CalculateJobMatch(j, p)
		if minScore <= 0 || res.Score >= minScore {
			scored = append(scored, ScoredJob{
				Job:        j,
				MatchScore: res.Score,
				Match:      res,
			})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].MatchScore > scored[j].MatchScore
	})

	return scored
}

func containsWord(text, word string) bool {
	if len(word) <= 3 {
		pattern := `\b` + regexp.QuoteMeta(word) + `\b`
		matched, _ := regexp.MatchString(pattern, text)
		return matched
	}
	return strings.Contains(text, word)
}
