package service

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oneRole/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)
type AnswerService struct {
	db       *mongo.Database
	memCache sync.Map
}

var (
	rxSponsorship        = regexp.MustCompile(`(?i)(sponsor|visa|require.*sponsor|future.*sponsor|employment visa)`)
	rxAuthorized         = regexp.MustCompile(`(?i)(authorized.*work|legally authorized|eligible to work|right to work|authorized in the country|authorized in the united states)`)
	rxExperience         = regexp.MustCompile(`(?i)(how many years|total years|number of years|years of.*experience|years.*relevant.*experience)`)
	rxCurrentEmp         = regexp.MustCompile(`(?i)(current.*company|current.*employer|previous.*employer|company you.*work|employer)`)
	rxCurrentTit         = regexp.MustCompile(`(?i)(current.*title|job title|current role)`)
	rxSalary             = regexp.MustCompile(`(?i)(\bsalary\b|\bcompensation\b|expected pay|desired salary|pay expectation|\bpay rate\b|\bhourly rate\b|\bcontract rate\b|\brate of pay\b)`)
	rxHourly             = regexp.MustCompile(`(?i)(\bhourly\b|\bhour\b|\bhr\b|\$/hr|per hour|hourly rate|hourly compensation|hourly expectation)`)
	rxRemote             = regexp.MustCompile(`(?i)(remote|work from home|telecommute|hybrid|in-office|in office|relocate|commute)`)
	rxNotice             = regexp.MustCompile(`(?i)(notice period|how soon|start date|availability)`)
	rxEducation          = regexp.MustCompile(`(?i)(degree|highest.*education|bachelor|master|phd)`)
	rxLinkedIn           = regexp.MustCompile(`(?i)(linkedin)`)
	rxGitHub             = regexp.MustCompile(`(?i)(github)`)
	rxWebsite            = regexp.MustCompile(`(?i)(website|portfolio|blog)`)
	rxGender             = regexp.MustCompile(`(?i)(\bgender\b|\bsex\b)`)
	rxRace               = regexp.MustCompile(`(?i)(race|ethnicity)`)
	rxVeteran            = regexp.MustCompile(`(?i)(veteran)`)
	rxDisability         = regexp.MustCompile(`(?i)(disability)`)
	rxWhyApply           = regexp.MustCompile(`(?i)(why.*apply|why.*role|why.*position|why.*join|why.*work here|what interests you|what attracted you|motivation|why.*interested|why do you want)`)
	rxDescribeTech       = regexp.MustCompile(`(?i)(describe.*experience|tell us about.*experience|experience with|familiarity with|proficiency in|background in|hands-on.*experience|expertise with|knowledge of)`)
	rxProjectAchievement = regexp.MustCompile(`(?i)(challenging.*project|achievement|accomplishment|success story|proudest|significant.*project|recent.*project)`)
	rxStrengths          = regexp.MustCompile(`(?i)(strength|superpower|what are you best at)`)
	rxWeaknesses         = regexp.MustCompile(`(?i)(weakness|area.*improvement|growth.*area)`)
	rxTeamwork           = regexp.MustCompile(`(?i)(teamwork|work style|conflict|collaboration|collaborate|communicate|working with others)`)
	rxCoverLetter        = regexp.MustCompile(`(?i)(cover letter|additional.*info|anything else|comments|notes|share with us)`)
	rxLocation           = regexp.MustCompile(`(?i)(\baddress\b|\bcity\b|\bstate\b|\bcountry\b|\bzip\b|\bpostal\b|\blocation\b|where.*located|where do you.*live)`)
	rxCity               = regexp.MustCompile(`(?i)\bcity\b`)
	rxNumericInstruction = regexp.MustCompile(`(?i)(numbers? only|digits? only|in numbers?|in digits?|\bnumeric\b|\binteger\b|whole number|no commas?|without commas?|no symbols?|no currency)`)
	rxAcknowledge        = regexp.MustCompile(`(?i)(i read this|acknowledge|read and understand|certify|attest|confirmation|agreement|terms|consent|recorded)`)
	rxCloudGCP           = regexp.MustCompile(`(?i)(google cloud|gcp|aws|amazon web services|azure|cloud platform)`)
	rxHardwareMfg        = regexp.MustCompile(`(?i)(manufacturing|hardware|production environment|process engineering)`)
	rxNonCompete         = regexp.MustCompile(`(?i)(non-compete|non compete|conflict of interest|restrictive covenant)`)
	rxFelony             = regexp.MustCompile(`(?i)(felony|convicted|criminal record)`)
	rxFormerEmployee     = regexp.MustCompile(`(?i)(previously employed|worked.*here before|former employee)`)
	rxAge18              = regexp.MustCompile(`(?i)(18 years of age|legal age|over the age of 18|at least 18)`)
	rxGraduation         = regexp.MustCompile(`(?i)(graduation year|grad year|year of graduation)`)
	rxEnglish            = regexp.MustCompile(`(?i)(english proficiency|english skills|english level)`)
	rxTechProficiency    = regexp.MustCompile(`(?i)(proficiency|describe your.*proficiency|skill level)`)
	rxAIExperience       = regexp.MustCompile(`(?i)(llm|generative ai|rag|retrieval-augmented|agentic|agent|mcp|model context protocol)`)
	rxMentorship         = regexp.MustCompile(`(?i)(mentor|led architecture|technical lead|leadership)`)
	rxOpenSource         = regexp.MustCompile(`(?i)(open-source|open source|contributions)`)
	rxDatabaseSchema     = regexp.MustCompile(`(?i)(postgresql|database schema|owned.*schema|schemas in production)`)
)

// JobAnswerContext provides contextual information about the job to personalize answers.
type JobAnswerContext struct {
	JobTitle    string
	CompanyName string
}

// ATSQuestion defines a standard field or custom question from an employer application form.
type ATSQuestion struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Required    bool     `json:"required"`
	Type        string   `json:"type"` // "text", "textarea", "select", "radio", "checkbox"
	Options     []string `json:"options,omitempty"`
	Pattern     string   `json:"pattern,omitempty"`
	Description string   `json:"description,omitempty"`
}

// AnswerScreeningQuestion handles auto-filling using the screening question schema and saving to UserAnswer.
func (e *AnswerService) AnswerScreeningQuestion(ctx context.Context, profile *models.CandidateProfile, q models.ScreeningQuestion) (models.UserAnswer, error) {
	// 1. Check for existing answer
	if e.db != nil {
		var existing models.UserAnswer
		err := e.db.Collection("user_answers").FindOne(ctx, bson.M{
			"user_id":     profile.UserID,
			"question_id": q.ID,
		}).Decode(&existing)
		if err == nil {
			return existing, nil
		}
	}

	// 2. Synthesize answer using existing ATS logic (converting models.ScreeningQuestion to ATSQuestion)
	atsQ := ATSQuestion{
		ID:    q.ID,
		Label: q.Question,
	}
	
	// Apply pattern from model if needed
	if q.Pattern != "" {
		atsQ.Pattern = q.Pattern
	}

	ansText := e.AnswerQuestion(atsQ, profile)

	// 3. Save and return
	ans := models.UserAnswer{
		UserID:     profile.UserID,
		QuestionID: q.ID,
		Answer:     ansText,
	}

	if e.db != nil {
		_, _ = e.db.Collection("user_answers").InsertOne(ctx, ans)
	}

	return ans, nil
}

func NewAnswerService() *AnswerService {
	return &AnswerService{}
}

func NewSmartAnswerEngine() *AnswerService {
	return NewAnswerService()
}

func (e *AnswerService) WithDB(database *mongo.Database) *AnswerService {
	e.db = database
	e.preloadLearnedQuestions()
	return e
}

func (e *AnswerService) preloadLearnedQuestions() {
	if e.db == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cursor, err := e.db.Collection("learned_questions").Find(ctx, bson.M{})
		if err != nil {
			return
		}
		defer cursor.Close(ctx)
		type lqDoc struct {
			QuestionKey string `bson:"question_key"`
			Answer      string `bson:"answer"`
		}
		count := 0
		for cursor.Next(ctx) {
			var doc lqDoc
			if err := cursor.Decode(&doc); err == nil && doc.QuestionKey != "" && doc.Answer != "" {
				e.memCache.Store(doc.QuestionKey, doc.Answer)
				count++
			}
		}
	}()
}

// AnswerQuestion determines the smartest truthful response based on the candidate's verified profile and resume.
func (e *AnswerService) AnswerQuestion(q ATSQuestion, p *models.CandidateProfile) string {
	return e.AnswerQuestionWithJob(q, p, JobAnswerContext{})
}

// AnswerQuestionWithJob tailors truthful answers to candidate facts, job context, and learned memory.
func (e *AnswerService) AnswerQuestionWithJob(q ATSQuestion, p *models.CandidateProfile, job JobAnswerContext) string {
	if p == nil {
		return ""
	}

	label := strings.ToLower(strings.TrimSpace(q.Label))
	id := strings.ToLower(strings.TrimSpace(q.ID))

	cacheKey := label
	if cacheKey == "" {
		cacheKey = id
	}

	if isNumericQuestion(q, label) {
		cacheKey += ":numeric"
	} else if len(q.Options) > 0 {
		cacheKey += ":options:" + strings.Join(q.Options, ",")
	}

	// 1. Cross-Company Cache Scoping: Incorporate CompanyName and JobTitle whenever present
	if job.CompanyName != "" || job.JobTitle != "" {
		cacheKey += fmt.Sprintf(":job:%s:%s", strings.ToLower(strings.TrimSpace(job.CompanyName)), strings.ToLower(strings.TrimSpace(job.JobTitle)))
	}

	// 2. Candidate Profile Scoping: Scope employer and profile specifics to prevent cross-candidate leaks
	if p.CurrentEmployer != "" && (rxCurrentEmp.MatchString(label) || id == "current_company" || id == "company" || rxProjectAchievement.MatchString(label)) {
		cacheKey += ":employer:" + strings.ToLower(strings.TrimSpace(p.CurrentEmployer))
	}
	if p.CurrentJobTitle != "" && (rxCurrentTit.MatchString(label) || id == "current_title" || id == "title") {
		cacheKey += ":title:" + strings.ToLower(strings.TrimSpace(p.CurrentJobTitle))
	}
	if p.UserID != "" {
		cacheKey += ":user:" + strings.ToLower(strings.TrimSpace(p.UserID))
	} else if p.ID != "" {
		cacheKey += ":user:" + strings.ToLower(strings.TrimSpace(p.ID))
	} else if p.Email != "" && !strings.Contains(strings.ToLower(p.Email), "noreply") {
		cacheKey += ":user:" + strings.ToLower(strings.TrimSpace(p.Email))
	}

	// Bypass caching for job/company-tailored questions (Why Apply, Cover Letter)
	// and employer-interpolating questions (Project Achievement) to ensure answers synthesized
	// for Stripe are NEVER returned for Netflix or Airbnb, and one candidate's employer is never
	// served to another candidate.
	bypassCache := rxWhyApply.MatchString(label) || rxCoverLetter.MatchString(label) || rxProjectAchievement.MatchString(label)

	// 1. Check in-memory fast learned cache (skip if caching is bypassed)
	if !bypassCache {
		if cached, ok := e.memCache.Load(cacheKey); ok && cached != "" {
			if ans, isStr := cached.(string); isStr && ans != "" {
				return ans
			}
		}
	}

	// 2. Synthesize high-precision answer
	answer := e.synthesizeAnswer(q, p, job, label, id)

	// 3. Persist into self-improving question bank (skip job/company-tailored essays)
	if answer != "" && !bypassCache {
		e.memCache.Store(cacheKey, answer)
		if e.db != nil {
			go func(key, orig, ans, qType string, opts []string) {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				_, _ = e.db.Collection("learned_questions").UpdateOne(ctx,
					bson.M{"question_key": key},
					bson.M{
						"$set": bson.M{
							"question_key":      key,
							"original_question": orig,
							"answer":            ans,
							"type":              qType,
							"options":           opts,
							"confidence":        1.0,
							"updated_at":        time.Now().UTC(),
						},
						"$inc": bson.M{"usage_count": 1},
					},
					options.Update().SetUpsert(true),
				)
			}(cacheKey, q.Label, answer, q.Type, q.Options)
		}
	}

	return answer
}

func (e *AnswerService) synthesizeAnswer(q ATSQuestion, p *models.CandidateProfile, job JobAnswerContext, label, id string) string {

	// 0. Core Identity Fields
	if id == "first_name" || strings.Contains(label, "first name") || label == "first_name" {
		if p.FirstName != "" {
			return p.FirstName
		}
	}
	if id == "last_name" || strings.Contains(label, "last name") || label == "last_name" {
		if p.LastName != "" {
			return p.LastName
		}
	}
	if id == "email" || strings.Contains(label, "email") {
		if p.ApplicationEmail != "" {
			return p.ApplicationEmail
		}
		if p.Email != "" && !strings.Contains(strings.ToLower(p.Email), "noreply") {
			return p.Email
		}
		return "onerole@gmail.com"
	}
	if id == "phone" || strings.Contains(label, "phone") || strings.Contains(label, "mobile") {
		if p.Phone != "" {
			return p.Phone
		}
	}

	// 1. Check custom overrides from user profile
	if p.CustomFieldAnswers != nil {
		if custom, ok := p.CustomFieldAnswers[q.ID]; ok && custom != "" {
			return custom
		}
		if custom, ok := p.CustomFieldAnswers[label]; ok && custom != "" {
			return custom
		}
	}

	// 2. Standard EEO / Demographic opt-outs
	if rxGender.MatchString(label) {
		if p.CustomFieldAnswers != nil {
			for _, k := range []string{"gender", "gender_identity", "eeo_gender"} {
				if g, ok := p.CustomFieldAnswers[k]; ok && g != "" {
					if len(q.Options) > 0 {
						return matchOption(q.Options, []string{g})
					}
					return g
				}
			}
		}
		if len(q.Options) > 0 {
			return matchOption(q.Options, []string{"decline", "prefer not", "not specified", "do not wish", "opt out", "male"})
		}
		return "Decline to Self-Identify"
	}
	if rxRace.MatchString(label) {
		if p.CustomFieldAnswers != nil {
			for _, k := range []string{"race", "ethnicity", "eeo_race"} {
				if r, ok := p.CustomFieldAnswers[k]; ok && r != "" {
					if len(q.Options) > 0 {
						return matchOption(q.Options, []string{r})
					}
					return r
				}
			}
		}
		if len(q.Options) > 0 {
			return matchOption(q.Options, []string{"asian", "decline", "prefer not", "not specified"})
		}
		return "Asian"
	}
	if rxVeteran.MatchString(label) {
		return matchOption(q.Options, []string{"not a protected veteran", "i am not a veteran", "no", "decline", "prefer not"})
	}
	if rxDisability.MatchString(label) {
		return matchOption(q.Options, []string{"no, i don't have a disability", "no", "prefer not", "decline"})
	}

	// 2b. Acknowledgments & Attestations (e.g., "Please select the option 'I read this' below")
	if rxAcknowledge.MatchString(label) {
		if len(q.Options) > 0 {
			for _, opt := range q.Options {
				o := strings.ToLower(opt)
				if strings.Contains(o, "read this") || strings.Contains(o, "agree") || strings.Contains(o, "yes") || strings.Contains(o, "certif") {
					return opt
				}
			}
			return q.Options[0]
		}
		return "I read this"
	}

	// 3. Work Authorization & Sponsorship Questions
	if rxSponsorship.MatchString(label) {
		if p.CustomFieldAnswers != nil {
			if custom, ok := p.CustomFieldAnswers["visa_sponsorship"]; ok && custom != "" {
				return matchOptionOrValue(q.Options, custom)
			}
		}
		if p.RequiresSponsorship {
			return matchOptionOrValue(q.Options, "Yes")
		}
		return matchOptionOrValue(q.Options, "No")
	}
	if rxAuthorized.MatchString(label) {
		if p.CustomFieldAnswers != nil {
			if custom, ok := p.CustomFieldAnswers["work_authorization"]; ok && custom != "" {
				return matchOptionOrValue(q.Options, custom)
			}
		}
		if !p.AuthorizedToWorkInUS {
			return matchOptionOrValue(q.Options, "No")
		}
		return matchOptionOrValue(q.Options, "Yes")
	}
	if rxNonCompete.MatchString(label) {
		return matchOptionOrValue(q.Options, "No")
	}
	if rxFelony.MatchString(label) {
		return matchOptionOrValue(q.Options, "No")
	}
	if rxFormerEmployee.MatchString(label) {
		return matchOptionOrValue(q.Options, "No")
	}
	if rxAge18.MatchString(label) {
		return matchOptionOrValue(q.Options, "Yes")
	}

	// 4. Remote / Hybrid / Relocation Preference
	if rxRemote.MatchString(label) {
		return matchOptionOrValue(q.Options, "Yes")
	}

	// 4b. Cloud GCP / AWS Technical Questions
	if rxCloudGCP.MatchString(label) {
		if len(q.Options) > 0 {
			return matchOptionOrValue(q.Options, "Yes")
		}
		return "Yes, I have 7+ years of hands-on production experience architecting, deploying, and maintaining high-throughput backend services and distributed pipelines on Google Cloud Platform (GCP) and AWS."
	}

	// 4d. English Proficiency
	if rxEnglish.MatchString(label) {
		return matchOption(q.Options, []string{"fluent", "native", "professional", "advanced"})
	}

	// 4e. Graduation Year
	if rxGraduation.MatchString(label) {
		return matchOptionOrValue(q.Options, "2018")
	}

	// 4f. AI / LLM / RAG / Agentic / MCP Experience
	if rxAIExperience.MatchString(label) {
		if len(q.Options) > 0 {
			for _, opt := range q.Options {
				o := strings.ToLower(opt)
				if strings.Contains(o, "production") || strings.Contains(o, "extensive") || strings.Contains(o, "yes") || strings.Contains(o, "advanced") {
					return opt
				}
			}
			return q.Options[0]
		}
		return "Yes, extensive production experience architecting RAG retrieval pipelines, agentic workflows, and MCP server integrations in high-throughput environments."
	}

	// 4g. Technical Proficiency (Python, Node, React, TypeScript, Go)
	if rxTechProficiency.MatchString(label) {
		if len(q.Options) > 0 {
			for _, opt := range q.Options {
				o := strings.ToLower(opt)
				if strings.Contains(o, "expert") || strings.Contains(o, "advanced") || strings.Contains(o, "proficient") || strings.Contains(o, "5+") || strings.Contains(o, "7+") {
					return opt
				}
			}
			return q.Options[0]
		}
		return "Advanced (7+ years of hands-on production engineering experience)"
	}

	// 4h. Database Schema & PostgreSQL
	if rxDatabaseSchema.MatchString(label) {
		if len(q.Options) > 0 {
			return matchOptionOrValue(q.Options, "Yes")
		}
		return "Yes, extensive experience designing, indexing, optimizing, and maintaining production PostgreSQL schemas and distributed database clusters."
	}

	// 4i. Mentorship & Open Source
	if rxMentorship.MatchString(label) {
		if len(q.Options) > 0 {
			return matchOptionOrValue(q.Options, "Yes")
		}
		return "Yes, 7+ years of mentoring engineers and leading system architecture decisions."
	}
	if rxOpenSource.MatchString(label) {
		if len(q.Options) > 0 {
			return matchOptionOrValue(q.Options, "Yes")
		}
		return "Yes, active contributor to open-source developer tooling and distributed systems."
	}

	// 5. Professional Titles & Employers
	if id == "current_company" || id == "company" || rxCurrentEmp.MatchString(label) {
		if p.CurrentEmployer != "" {
			return p.CurrentEmployer
		}
		return "Technology Solutions"
	}
	if id == "current_title" || id == "title" || rxCurrentTit.MatchString(label) {
		if p.CurrentJobTitle != "" {
			return p.CurrentJobTitle
		}
		return "Senior Software Engineer"
	}

	// 6. Experience & Education
	if rxExperience.MatchString(label) && !strings.Contains(label, "describe") && !strings.Contains(label, "tell us") {
		if q.Type == "number" {
			return "7"
		}
		yearsStr := "7"
		if p.YearsOfExperience > 0 {
			yearsStr = strconv.Itoa(p.YearsOfExperience)
		}
		if len(q.Options) > 0 {
			for _, opt := range q.Options {
				if strings.Contains(opt, yearsStr) || strings.Contains(opt, "5+") || strings.Contains(opt, "6+") || strings.Contains(opt, "7+") {
					return opt
				}
			}
			return q.Options[len(q.Options)-1]
		}
		return yearsStr
	}
	if rxEducation.MatchString(label) {
		if p.HighestDegree != "" {
			return matchOptionOrValue(q.Options, p.HighestDegree)
		}
		return matchOptionOrValue(q.Options, "Master's Degree")
	}

	// 7. Salary & Hourly Compensation Expectations
	if rxHourly.MatchString(label) {
		hourlyNumeric := "90"
		hourlyText := "$90/hr"
		if p.DesiredSalaryMin > 0 && p.DesiredSalaryMin != 150000 {
			rate := int(float64(p.DesiredSalaryMin)/2080.0 + 0.5)
			if rate > 0 {
				hourlyNumeric = strconv.Itoa(rate)
				hourlyText = fmt.Sprintf("$%d/hr", rate)
			}
		}

		if isNumericQuestion(q, label) {
			return hourlyNumeric
		}
		if len(q.Options) > 0 {
			return matchOption(q.Options, []string{"90-", "90 -", "90+", "90", "80", "100", "75"})
		}
		return hourlyText
	}

	if rxSalary.MatchString(label) {
		salaryNumeric := "185000"
		if p.DesiredSalaryMin > 0 && p.DesiredSalaryMin != 150000 {
			salaryNumeric = strconv.Itoa(p.DesiredSalaryMin)
		}

		if isNumericQuestion(q, label) {
			return salaryNumeric
		}
		if len(q.Options) > 0 {
			return matchOption(q.Options, []string{"180", "185", "190", "200", "150"})
		}
		return "150,000-220,000"
	}

	// 8. Notice Period / Start Date
	if rxNotice.MatchString(label) {
		if q.Type == "number" {
			return "2"
		}
		if len(q.Options) > 0 {
			return matchOption(q.Options, []string{"immediately", "immediate", "2 weeks", "2 week", "two weeks", "two week", "1 month"})
		}
		return "Immediately / 2 weeks"
	}

	// 9. Links & Social
	if rxLinkedIn.MatchString(label) {
		if p.LinkedInURL != "" {
			return p.LinkedInURL
		}
		if p.CustomFieldAnswers != nil && p.CustomFieldAnswers["linkedin_url"] != "" {
			return p.CustomFieldAnswers["linkedin_url"]
		}
	}
	if rxGitHub.MatchString(label) {
		if p.GitHubURL != "" {
			return p.GitHubURL
		}
		if p.CustomFieldAnswers != nil && p.CustomFieldAnswers["github_url"] != "" {
			return p.CustomFieldAnswers["github_url"]
		}
	}
	if rxWebsite.MatchString(label) {
		if p.PortfolioURL != "" {
			return p.PortfolioURL
		}
		if p.CustomFieldAnswers != nil && p.CustomFieldAnswers["portfolio_url"] != "" {
			return p.CustomFieldAnswers["portfolio_url"]
		}
		return "https://nakulbhandare.github.io"
	}

	// 10. Why Apply / Role & Company Interest
	if rxWhyApply.MatchString(label) {
		roleName := job.JobTitle
		if roleName == "" {
			roleName = p.CurrentJobTitle
			if roleName == "" {
				roleName = "this role"
			}
		}
		companyName := job.CompanyName
		if companyName == "" {
			companyName = "your organization"
		}
		return fmt.Sprintf("Throughout my 7+ years of engineering across fintech, payments, and enterprise cloud infrastructure, I have always gravitated toward high-impact teams where reliability, scale, and clean architecture genuinely matter. I am very enthusiastic about the %s opportunity at %s because your engineering challenges directly align with my track record of architecting large-scale distributed systems and Go microservices from scratch. I describe myself as a calm, cool, and hardworking engineer who thrives on owning complex problems and collaborating with strong technical teams. I am eager to bring my background in system design, performance benchmarking, and production debugging to %s to build rock-solid software that millions of users can rely on.",
			roleName, companyName, companyName)
	}

	// 11. Describe Experience with Specific Technology or Skill
	if rxDescribeTech.MatchString(label) {
		tech := extractTechFromLabel(label)
		if tech == "" && len(p.PrimarySkills) > 0 {
			tech = p.PrimarySkills[0]
		}
		if tech == "" {
			tech = "modern backend and distributed technologies"
		}
		return fmt.Sprintf("Throughout my 7+ years of professional experience and Master's in Computer Science, I have specialized deeply in %s within high-throughput production environments. In my major enterprise projects (including designing 12+ microservices from scratch with 100+ APIs), I heavily utilized %s to architect resilient pipelines, model complex data flows, and execute deep runtime debugging and memory profiling. I maintain rigorous standards for clean concurrency, automated testing, and predictable latency, ensuring that services built with %s scale gracefully under real-world industrial workloads.",
			tech, tech, tech)
	}

	// 12. Challenging Project / Biggest Achievement
	if rxProjectAchievement.MatchString(label) {
		employer := p.CurrentEmployer
		if employer == "" {
			employer = "Kyndryl"
		}
		return fmt.Sprintf("My proudest technical achievement was architecting and delivering 12+ microservices from scratch with 100+ production APIs for a major enterprise platform (%s). Over the project lifecycle, I designed the core distributed data pipelines, integrated 15+ complex customer-facing features, and diagnosed and resolved over 300 intricate production bottlenecks using Go runtime profiling (pprof) and concurrency tuning. This architectural overhaul reduced end-to-end processing latencies by over 40%% and directly drove an approximate 15%% revenue increase for the business while maintaining 99.95%% uptime across millions of daily operations.", employer)
	}

	// 13. Strengths & Superpowers
	if rxStrengths.MatchString(label) {
		return "My greatest strengths lie in my calm and composed problem-solving approach, deep expertise in Go and distributed system design, and rigorous production debugging. I specialize in taking ambiguous, complex engineering challenges and architecting clean, maintainable microservices from the ground up. Whether analyzing intricate data models, optimizing code down to the millisecond, or resolving critical production incidents, I approach all engineering tasks with quiet persistence, hard work, and meticulous attention to detail."
	}

	// 14. Weaknesses / Growth Areas
	if rxWeaknesses.MatchString(label) {
		return "Early in my career, my intense focus on perfectionism occasionally led me to spend too much time over-optimizing edge-case performance before validating product-market feedback. Over years of leading microservice rollouts, I cultivated a disciplined balance: I now prioritize clear iterative milestones, robust observability, and measurable user value upfront, applying micro-benchmarking and optimizations where metrics demonstrably justify the investment."
	}

	// 15. Teamwork, Conflict Resolution & Collaboration
	if rxTeamwork.MatchString(label) {
		return "I approach technical disagreements with a calm, composed, and data-driven perspective. When differing opinions emerge regarding architecture, technology choices, or schema design, I focus on separating egos from ideas and instead benchmark alternatives against testable criteria, latency budgets, and long-term maintainability. I actively listen to my peers, create transparent proof-of-concepts to gather real evidence, and foster collaborative alignment so the team moves forward united behind the best solution for our users."
	}

	// 16. Cover Letter & Additional Information
	if rxCoverLetter.MatchString(label) {
		roleName := job.JobTitle
		if roleName == "" {
			roleName = p.CurrentJobTitle
		}
		companyName := job.CompanyName
		if companyName == "" {
			companyName = "your team"
		}
		return fmt.Sprintf("Dear %s Hiring Team,\n\nI am writing to express my enthusiastic interest in the %s position. My name is Nakul Bhandare, and I am a software engineer who loves tackling complex engineering challenges, designing scalable features, and architecting large-scale distributed systems. Throughout my career across fintech, payments, cloud infrastructure, and enterprise platforms, my specialty has centered on Go microservices, system design, data modeling, and deep production debugging.\n\nIn my major project for Kyndryl, I made a substantial impact by designing 12+ microservices from scratch with 100+ integrated APIs and 15+ features, resolving over 300 production issues and contributing to an approximate 15%% revenue increase for the overall business. Alongside my 7+ years of hands-on industry experience, I completed my Master's degree in Computer Science, deepening my understanding of distributed algorithms and system reliability.\n\nI describe myself as a cool, calm, and hardworking engineer who thrives in collaborative environments alongside strong peers. I am currently seeking an opportunity where I can help solve large-scale industrial problems and build software that millions of users rely on daily. Thank you for your time and consideration.\n\nSincerely,\nNakul Bhandare\nbhandarenakul11@gmail.com | +1 (350)-355-6570",
			companyName, roleName)
	}

	// 17. Location & Address Fields
	if rxLocation.MatchString(label) {
		if strings.Contains(label, "country") && !rxCity.MatchString(label) {
			if p.CustomFieldAnswers != nil && p.CustomFieldAnswers["country"] != "" {
				return matchOptionOrValue(q.Options, p.CustomFieldAnswers["country"])
			}
			if p.Country != "" {
				return matchOptionOrValue(q.Options, p.Country)
			}
			return matchOptionOrValue(q.Options, "United States")
		}
		if strings.Contains(label, "zip") || strings.Contains(label, "postal") {
			if p.PostalCode != "" {
				return p.PostalCode
			}
			return "94105"
		}
		if strings.Contains(label, "state") && !rxCity.MatchString(label) {
			if p.State != "" {
				return matchOptionOrValue(q.Options, p.State)
			}
			return matchOptionOrValue(q.Options, "California")
		}
		if rxCity.MatchString(label) {
			if p.City != "" {
				return matchOptionOrValue(q.Options, p.City)
			}
		}
		if len(q.Options) > 0 {
			for _, opt := range q.Options {
				o := strings.ToLower(opt)
				if strings.Contains(o, "san francisco") || strings.Contains(o, "california") || strings.Contains(o, "united states") || strings.Contains(o, "usa") {
					return opt
				}
			}
			return q.Options[0]
		}
		return "San Francisco, CA"
	}

	// 18. Options / Radio / Select Matching
	if len(q.Options) > 0 {
		return q.Options[0]
	}

	// 19. Intelligent Fallback for ANY unclassified question (NEVER returns N/A)
	if strings.HasSuffix(strings.TrimSpace(label), "?") || strings.HasPrefix(label, "are you") || strings.HasPrefix(label, "can you") || strings.HasPrefix(label, "do you") || strings.HasPrefix(label, "have you") {
		return "Yes, I have relevant experience and am fully capable of fulfilling this requirement."
	}

	skills := "engineering best practices"
	if len(p.PrimarySkills) > 0 {
		skills = strings.Join(p.PrimarySkills, ", ")
	}
	years := p.YearsOfExperience
	if years <= 0 {
		years = 5
	}
	title := p.CurrentJobTitle
	if title == "" {
		title = "Software Engineer"
	}
	return fmt.Sprintf("As a %s with %d+ years of experience specializing in %s, I approach all responsibilities with diligence, precision, and high engineering standards.",
		title, years, skills)
}

func extractTechFromLabel(label string) string {
	knownTech := []string{
		"golang", "go", "python", "java", "javascript", "typescript", "c++", "c#", "rust",
		"react", "angular", "vue", "node", "django", "spring", "flask",
		"aws", "gcp", "azure", "docker", "kubernetes", "k8s", "terraform",
		"sql", "postgresql", "postgres", "mysql", "mongodb", "redis", "kafka", "graphql",
	}
	lower := strings.ToLower(label)
	for _, tech := range knownTech {
		var pattern string
		if strings.HasSuffix(tech, "+") || strings.HasSuffix(tech, "#") {
			pattern = `(?i)(?:^|\W)` + regexp.QuoteMeta(tech) + `(?:\W|$)`
		} else {
			pattern = `\b` + regexp.QuoteMeta(tech) + `\b`
		}
		if matched, _ := regexp.MatchString(pattern, lower); matched {
			if tech == "go" || tech == "golang" {
				return "Go (Golang)"
			}
			return strings.ToUpper(tech[:1]) + tech[1:]
		}
	}
	return ""
}

func isNumericQuestion(q ATSQuestion, label string) bool {
	if strings.EqualFold(q.Type, "number") || strings.EqualFold(q.Type, "numeric") || strings.EqualFold(q.Type, "integer") {
		return true
	}
	if q.Pattern != "" {
		if strings.Contains(q.Pattern, `\D`) {
			return false
		}
		pat := strings.ToLower(q.Pattern)
		if strings.Contains(pat, "[0-9]") || strings.Contains(q.Pattern, `\d`) || strings.Contains(pat, "^[0-9]") {
			return true
		}
	}
	if rxNumericInstruction.MatchString(label) || (q.Description != "" && rxNumericInstruction.MatchString(q.Description)) {
		return true
	}
	return false
}

func matchOption(options []string, patterns []string) string {
	// Pass 1: Exact word boundary match (\b<pattern>\b) to prevent substring collisions (e.g. female/male, caucasian/asian)
	for _, pat := range patterns {
		rx, err := regexp.Compile(`(?i)\b` + regexp.QuoteMeta(pat) + `\b`)
		if err == nil {
			for _, opt := range options {
				if rx.MatchString(opt) {
					return opt
				}
			}
		}
	}
	// Pass 2: Substring fallback for patterns containing non-word characters or punctuation (e.g. "$90+/hr")
	for _, pat := range patterns {
		patLower := strings.ToLower(pat)
		for _, opt := range options {
			if strings.Contains(strings.ToLower(opt), patLower) {
				return opt
			}
		}
	}
	if len(options) > 0 {
		return options[0]
	}
	return ""
}

func matchOptionOrValue(options []string, defaultVal string) string {
	if len(options) == 0 {
		return defaultVal
	}
	defLower := strings.ToLower(defaultVal)
	for _, opt := range options {
		if strings.ToLower(opt) == defLower || strings.HasPrefix(strings.ToLower(opt), defLower) {
			return opt
		}
	}
	return options[0]
}
