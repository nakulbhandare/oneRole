package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/oneRole/connectors/ats_inspector"
	"github.com/oneRole/migrations"
	"github.com/oneRole/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/mongo/driver/connstring"
)

func main() {
	mongoURI := os.Getenv("DATABASE_URL")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017/onerole"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	parsed, err := connstring.ParseAndValidate(mongoURI)
	if err != nil {
		log.Fatalf("Invalid DATABASE_URL %q: %v", mongoURI, err)
	}
	dbName := parsed.Database
	if dbName == "" {
		dbName = "onerole"
	}

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer client.Disconnect(ctx)

	database := client.Database(dbName)
	log.Printf("Connected to MongoDB database: %s", dbName)

	// Apply migrations (Version 2)
	log.Println("Applying declarative migrations (v2)...")
	if err := migrations.Apply(ctx, database); err != nil {
		log.Fatalf("Failed to apply migrations: %v", err)
	}
	log.Println("Migrations applied successfully.")

	normalizer := ats_inspector.NewASTSchemaNormalizer(nil)

	// 1. Seed ATS Company Configs
	if err := seedCompanyConfigs(ctx, database); err != nil {
		log.Fatalf("Failed to seed ATS company configs: %v", err)
	}

	// 2. Seed Question Embedding Taxonomy
	if err := seedQuestionEmbeddings(ctx, database); err != nil {
		log.Fatalf("Failed to seed question embeddings: %v", err)
	}

	// 3. Seed Sample Job Form Schemas
	if err := seedJobFormSchemas(ctx, database, normalizer); err != nil {
		log.Fatalf("Failed to seed job form schemas: %v", err)
	}

	log.Println("Milestone 1 schema seeding completed successfully.")
}

func seedCompanyConfigs(ctx context.Context, database *mongo.Database) error {
	coll := database.Collection("ats_company_configs")
	now := time.Now().UTC()

	configs := []models.ATSCompanyConfig{
		// Greenhouse: Figma
		{
			ID:                "greenhouse-figma",
			Provider:          "greenhouse",
			CompanyID:         "figma",
			CompanyName:       "Figma",
			CanonicalEmbedURL: "https://boards.greenhouse.io/figma",
			APIEndpoint:       "https://boards-api.greenhouse.io/v1/boards/figma",
			Tier:              1,
			ScreeningRules: map[string]string{
				"join Figma": "I am passionate about collaborative design systems and mission-critical software architecture.",
			},
			RateLimitPerSec: 10,
			MaxConcurrency:  5,
			AuthType:        "public",
			IsActive:        true,
			CreatedAt:       now,
			UpdatedAt:       now,
		},
		// Greenhouse: Stripe
		{
			ID:                "greenhouse-stripe",
			Provider:          "greenhouse",
			CompanyID:         "stripe",
			CompanyName:       "Stripe",
			CanonicalEmbedURL: "https://boards.greenhouse.io/stripe",
			APIEndpoint:       "https://boards-api.greenhouse.io/v1/boards/stripe",
			Tier:              1,
			ScreeningRules: map[string]string{
				"work remotely": "Yes",
			},
			RateLimitPerSec: 10,
			MaxConcurrency:  5,
			AuthType:        "public",
			IsActive:        true,
			CreatedAt:       now,
			UpdatedAt:       now,
		},
		// Ashby: Linear
		{
			ID:                "ashby-linear",
			Provider:          "ashby",
			CompanyID:         "linear",
			CompanyName:       "Linear",
			CanonicalEmbedURL: "https://jobs.ashbyhq.com/linear",
			APIEndpoint:       "https://jobs.ashbyhq.com/api/non-user-graphql",
			Tier:              1,
			ScreeningRules: map[string]string{
				"Notice Period": "Immediately / 2 weeks",
			},
			RateLimitPerSec: 10,
			MaxConcurrency:  5,
			AuthType:        "public",
			IsActive:        true,
			CreatedAt:       now,
			UpdatedAt:       now,
		},
		// Ashby: Notion
		{
			ID:                "ashby-notion",
			Provider:          "ashby",
			CompanyID:         "notion",
			CompanyName:       "Notion",
			CanonicalEmbedURL: "https://jobs.ashbyhq.com/notion",
			APIEndpoint:       "https://jobs.ashbyhq.com/api/non-user-graphql",
			Tier:              1,
			ScreeningRules: map[string]string{
				"Anchor Days": "Yes",
			},
			RateLimitPerSec: 10,
			MaxConcurrency:  5,
			AuthType:        "public",
			IsActive:        true,
			CreatedAt:       now,
			UpdatedAt:       now,
		},
		// Lever: Palantir
		{
			ID:                "lever-palantir",
			Provider:          "lever",
			CompanyID:         "palantir",
			CompanyName:       "Palantir",
			CanonicalEmbedURL: "https://jobs.lever.co/palantir",
			APIEndpoint:       "https://api.lever.co/v0/postings/palantir",
			Tier:              1,
			ScreeningRules: map[string]string{
				"Language Skill": "English",
			},
			RateLimitPerSec: 5,
			MaxConcurrency:  3,
			AuthType:        "public",
			IsActive:        true,
			CreatedAt:       now,
			UpdatedAt:       now,
		},
		// Lever: Spotify
		{
			ID:                "lever-spotify",
			Provider:          "lever",
			CompanyID:         "spotify",
			CompanyName:       "Spotify",
			CanonicalEmbedURL: "https://jobs.lever.co/spotify",
			APIEndpoint:       "https://api.lever.co/v0/postings/spotify",
			Tier:              1,
			RateLimitPerSec:   5,
			MaxConcurrency:    3,
			AuthType:          "public",
			IsActive:          true,
			CreatedAt:         now,
			UpdatedAt:         now,
		},
		// SmartRecruiters: Western Digital
		{
			ID:                "smartrecruiters-westerndigital",
			Provider:          "smartrecruiters",
			CompanyID:         "westerndigital",
			CompanyName:       "Western Digital",
			CanonicalEmbedURL: "https://jobs.smartrecruiters.com/WesternDigital",
			APIEndpoint:       "https://api.smartrecruiters.com/v1/companies/WesternDigital",
			Tier:              2,
			RateLimitPerSec:   5,
			MaxConcurrency:    3,
			AuthType:          "public",
			IsActive:          true,
			CreatedAt:         now,
			UpdatedAt:         now,
		},
		// Workday: AdventHealth
		{
			ID:                "workday-adventhealth",
			Provider:          "workday",
			CompanyID:         "adventhealth",
			CompanyName:       "AdventHealth",
			CanonicalEmbedURL: "https://adventhealth.wd12.myworkdayjobs.com/AH_External_Career_Site",
			APIEndpoint:       "https://adventhealth.wd12.myworkdayjobs.com/wday/cxs/adventhealth/AH_External_Career_Site",
			Tier:              2,
			RateLimitPerSec:   2,
			MaxConcurrency:    2,
			AuthType:          "public",
			IsActive:          true,
			CreatedAt:         now,
			UpdatedAt:         now,
		},
	}

	for _, cfg := range configs {
		if err := cfg.Validate(); err != nil {
			return fmt.Errorf("validate config %s: %w", cfg.ID, err)
		}
		_, err := coll.UpdateOne(
			ctx,
			bson.M{"_id": cfg.ID},
			bson.M{"$set": cfg},
			options.Update().SetUpsert(true),
		)
		if err != nil {
			return fmt.Errorf("upsert config %s: %w", cfg.ID, err)
		}
	}
	log.Printf("Seeded %d ATS company configs.", len(configs))
	return nil
}

func seedQuestionEmbeddings(ctx context.Context, database *mongo.Database) error {
	coll := database.Collection("form_question_embeddings")
	now := time.Now().UTC()

	embeddings := []models.FormQuestionEmbedding{
		// 1. Work Authorization
		{
			ID:             "emb-work-auth-us",
			QuestionHash:   "c50d32168a2bf194",
			CanonicalLabel: "work_authorization_us",
			Category:       "work_authorization",
			QuestionPatterns: []string{
				"are you legally authorized to work in the united states",
				"are you legally authorized to work in the us",
				"are you authorized to work in the united states",
				"are you authorized to work in the us",
				"are you eligible to work in the united states",
				"do you have the legal right to work in the united states",
				"legal work authorization in the us",
				"are you authorized to work lawfully in the united states",
				"do you have unrestricted authorization to work in the united states",
				"do you have unrestricted work authorization",
			},
			CandidateAttributePath: "authorized_to_work_in_us",
			AnswerType:             "boolean",
			DefaultValue:           true,
			CandidateMappedValue:   true,
			OptionMatchStrategy:    "boolean_synonym",
			ConfidenceThreshold:    0.95,
			CreatedAt:              now,
			UpdatedAt:              now,
		},
		// 2. Visa Sponsorship
		{
			ID:             "emb-visa-sponsorship-future",
			QuestionHash:   "e81d7f62c0b8915e",
			CanonicalLabel: "visa_sponsorship_future",
			Category:       "visa_sponsorship",
			QuestionPatterns: []string{
				"will you now or in the future require visa sponsorship",
				"will you now or in the future require sponsorship for employment visa status",
				"do you require visa sponsorship",
				"will you require visa sponsorship",
				"will you require sponsorship now or in the future",
				"do you require sponsorship to work in the us",
				"do you now or will you in the future require sponsorship for employment visa status (e.g., h-1b, opt, stem opt)",
				"will you require an h-1b transfer or new petition",
			},
			CandidateAttributePath: "requires_sponsorship",
			AnswerType:             "boolean",
			DefaultValue:           false,
			CandidateMappedValue:   false,
			OptionMatchStrategy:    "boolean_synonym",
			ConfidenceThreshold:    0.95,
			CreatedAt:              now,
			UpdatedAt:              now,
		},
		// 3. Notice Period
		{
			ID:             "emb-notice-period-duration",
			QuestionHash:   "a47d2f981e4c3b65",
			CanonicalLabel: "notice_period_duration",
			Category:       "notice_period",
			QuestionPatterns: []string{
				"what is your notice period",
				"notice period",
				"how much notice do you need to give your current employer",
				"notice period required",
				"earliest start date",
				"when would you be available to begin work",
				"when are you able to start",
				"how soon can you start",
				"availability to start",
				"can you start immediately",
				"standard notice period",
			},
			CandidateAttributePath: "notice_period_weeks",
			AnswerType:             "string",
			DefaultValue:           "Immediately / 2 weeks",
			CandidateMappedValue:   "Immediately / 2 weeks",
			OptionMatchStrategy:    "substring_case_insensitive",
			ConfidenceThreshold:    0.90,
			CreatedAt:              now,
			UpdatedAt:              now,
		},
		// 4. Experience & Skills: Total Experience
		{
			ID:             "emb-total-years-experience",
			QuestionHash:   "9b12e35a7f4c8d01",
			CanonicalLabel: "total_years_experience",
			Category:       "experience_skills",
			QuestionPatterns: []string{
				"how many years of total professional experience do you have",
				"total years of experience",
				"how many years of relevant experience do you have",
				"years of relevant experience",
				"years of software engineering experience",
				"total software engineering experience",
			},
			CandidateAttributePath: "years_of_experience",
			AnswerType:             "number",
			DefaultValue:           7,
			CandidateMappedValue:   7,
			OptionMatchStrategy:    "substring_case_insensitive",
			ConfidenceThreshold:    0.95,
			CreatedAt:              now,
			UpdatedAt:              now,
		},
		// 4b. Experience & Skills: Go
		{
			ID:             "emb-skills-golang-experience",
			QuestionHash:   "5d2e8b417c9a3f06",
			CanonicalLabel: "skills_golang_experience",
			Category:       "experience_skills",
			QuestionPatterns: []string{
				"years of experience with go",
				"years of experience with golang",
				"how many years of experience do you have with go",
				"total years of go experience",
				"go experience",
				"golang experience",
				"proficiency in go",
			},
			CandidateAttributePath: "primary_skills",
			AnswerType:             "string",
			DefaultValue:           "7+ years",
			CandidateMappedValue:   "7+ years",
			OptionMatchStrategy:    "substring_case_insensitive",
			ConfidenceThreshold:    0.90,
			CreatedAt:              now,
			UpdatedAt:              now,
		},
		// 4c. Experience & Skills: Kubernetes
		{
			ID:             "emb-skills-kubernetes-experience",
			QuestionHash:   "3f9c7a124e8d5b06",
			CanonicalLabel: "skills_kubernetes_experience",
			Category:       "experience_skills",
			QuestionPatterns: []string{
				"years of experience with kubernetes",
				"years of experience with k8s",
				"experience with kubernetes (k8s)",
				"proficiency in kubernetes",
				"kubernetes production experience",
			},
			CandidateAttributePath: "primary_skills",
			AnswerType:             "string",
			DefaultValue:           "5+ years",
			CandidateMappedValue:   "5+ years",
			OptionMatchStrategy:    "substring_case_insensitive",
			ConfidenceThreshold:    0.90,
			CreatedAt:              now,
			UpdatedAt:              now,
		},
		// 5. Compensation: Desired Salary
		{
			ID:             "emb-comp-desired-salary",
			QuestionHash:   "6b4d9a281c7e3f05",
			CanonicalLabel: "compensation_desired_salary",
			Category:       "compensation",
			QuestionPatterns: []string{
				"what is your expected salary",
				"desired salary",
				"desired compensation",
				"target annual salary",
				"expected annual compensation",
				"salary expectation",
				"minimum base salary",
			},
			CandidateAttributePath: "desired_salary_min",
			AnswerType:             "number",
			DefaultValue:           185000,
			CandidateMappedValue:   185000,
			OptionMatchStrategy:    "substring_case_insensitive",
			ConfidenceThreshold:    0.90,
			CreatedAt:              now,
			UpdatedAt:              now,
		},
		// 6. Location Preferences: Remote Preference
		{
			ID:             "emb-loc-remote-preference",
			QuestionHash:   "8f1c4e927a3d6b05",
			CanonicalLabel: "location_remote_preference",
			Category:       "location_preferences",
			QuestionPatterns: []string{
				"are you open to remote work",
				"open to remote",
				"work arrangement preference",
				"preferred work environment",
				"are you open to working in a hybrid arrangement",
			},
			CandidateAttributePath: "open_to_remote",
			AnswerType:             "boolean",
			DefaultValue:           true,
			CandidateMappedValue:   true,
			OptionMatchStrategy:    "boolean_synonym",
			ConfidenceThreshold:    0.90,
			CreatedAt:              now,
			UpdatedAt:              now,
		},
		// 6b. Location: SF Office / Hybrid
		{
			ID:             "emb-loc-in-office-sf",
			QuestionHash:   "7e3b9f412a8d5c06",
			CanonicalLabel: "location_in_office_sf",
			Category:       "location_preferences",
			QuestionPatterns: []string{
				"are you willing to work out of the san francisco office",
				"which office location(s) are you willing & able to work out of",
				"comfortable with hybrid 2-3 days in sf office",
				"are you comfortable with a hybrid schedule (2-3 days in office)",
				"are you willing to work in a hybrid schedule in san francisco",
			},
			CandidateAttributePath: "custom_field_answers.in_office_sf",
			AnswerType:             "string",
			DefaultValue:           "Yes",
			CandidateMappedValue:   "Yes",
			OptionMatchStrategy:    "substring_case_insensitive",
			ConfidenceThreshold:    0.90,
			CreatedAt:              now,
			UpdatedAt:              now,
		},
		// 7. EEO: Gender
		{
			ID:             "emb-eeo-gender-disclosure",
			QuestionHash:   "1b5d8f924c7e3a06",
			CanonicalLabel: "eeo_gender_disclosure",
			Category:       "demographics_eeo",
			QuestionPatterns: []string{
				"gender",
				"gender identity",
				"what is your gender",
				"voluntary self-identification of gender",
			},
			CandidateAttributePath: "custom_field_answers.eeo_gender",
			AnswerType:             "select_option",
			DefaultValue:           "Decline to Self-Identify",
			CandidateMappedValue:   "Decline to Self-Identify",
			OptionMatchStrategy:    "substring_case_insensitive",
			ConfidenceThreshold:    0.95,
			CreatedAt:              now,
			UpdatedAt:              now,
		},
		// 7b. EEO: Race
		{
			ID:             "emb-eeo-race-disclosure",
			QuestionHash:   "8a3c7e194f2b5d06",
			CanonicalLabel: "eeo_race_disclosure",
			Category:       "demographics_eeo",
			QuestionPatterns: []string{
				"race / ethnicity",
				"race / ethnic background",
				"what is your race or ethnicity",
				"voluntary self-identification of race/ethnicity",
				"are you hispanic or latino",
			},
			CandidateAttributePath: "custom_field_answers.eeo_race",
			AnswerType:             "select_option",
			DefaultValue:           "Decline to Self-Identify",
			CandidateMappedValue:   "Decline to Self-Identify",
			OptionMatchStrategy:    "substring_case_insensitive",
			ConfidenceThreshold:    0.95,
			CreatedAt:              now,
			UpdatedAt:              now,
		},
		// 7c. EEO: Veteran
		{
			ID:             "emb-eeo-veteran-disclosure",
			QuestionHash:   "3d9f1a7e8c2b5e04",
			CanonicalLabel: "eeo_veteran_disclosure",
			Category:       "demographics_eeo",
			QuestionPatterns: []string{
				"veteran status",
				"are you a protected veteran",
				"voluntary self-identification of veteran status",
			},
			CandidateAttributePath: "custom_field_answers.eeo_veteran",
			AnswerType:             "select_option",
			DefaultValue:           "I am not a protected veteran",
			CandidateMappedValue:   "I am not a protected veteran",
			OptionMatchStrategy:    "substring_case_insensitive",
			ConfidenceThreshold:    0.95,
			CreatedAt:              now,
			UpdatedAt:              now,
		},
		// 7d. EEO: Disability
		{
			ID:             "emb-eeo-disability-disclosure",
			QuestionHash:   "5e1a9c3b7f4d8e02",
			CanonicalLabel: "eeo_disability_disclosure",
			Category:       "demographics_eeo",
			QuestionPatterns: []string{
				"disability status",
				"do you have a disability",
				"voluntary self-identification of disability",
				"individual with a disability",
			},
			CandidateAttributePath: "custom_field_answers.eeo_disability",
			AnswerType:             "select_option",
			DefaultValue:           "No, I do not have a disability",
			CandidateMappedValue:   "No, I do not have a disability",
			OptionMatchStrategy:    "substring_case_insensitive",
			ConfidenceThreshold:    0.95,
			CreatedAt:              now,
			UpdatedAt:              now,
		},
		// 8. Compliance & Consent: Age 18+
		{
			ID:             "emb-compliance-age-18",
			QuestionHash:   "7a2f9c4d1e8b3e05",
			CanonicalLabel: "compliance_age_18_or_older",
			Category:       "compliance_consent",
			QuestionPatterns: []string{
				"are you 18 years of age or older",
				"are you at least 18 years old",
				"legal age requirement",
			},
			CandidateAttributePath: "custom_field_answers.is_18_or_older",
			AnswerType:             "boolean",
			DefaultValue:           true,
			CandidateMappedValue:   true,
			OptionMatchStrategy:    "boolean_synonym",
			ConfidenceThreshold:    0.95,
			CreatedAt:              now,
			UpdatedAt:              now,
		},
		// 9. Education: Highest Degree
		{
			ID:             "emb-education-highest-degree",
			QuestionHash:   "2f9d7c1a8e3b4f05",
			CanonicalLabel: "education_highest_degree",
			Category:       "education",
			QuestionPatterns: []string{
				"highest level of education completed",
				"highest degree earned",
				"education level",
				"degree",
			},
			CandidateAttributePath: "highest_degree",
			AnswerType:             "select_option",
			DefaultValue:           "Master's Degree",
			CandidateMappedValue:   "Master's Degree",
			OptionMatchStrategy:    "substring_case_insensitive",
			ConfidenceThreshold:    0.95,
			CreatedAt:              now,
			UpdatedAt:              now,
		},
		// 10. Sourcing Referral: LinkedIn
		{
			ID:             "emb-source-channel-referral",
			QuestionHash:   "6c1f9b3e7a2d8e04",
			CanonicalLabel: "source_channel_referral",
			Category:       "sourcing_referral",
			QuestionPatterns: []string{
				"how did you hear about us",
				"how did you hear about this job",
				"how did you hear about this role",
				"referral source",
			},
			CandidateAttributePath: "custom_field_answers.source_channel",
			AnswerType:             "select_option",
			DefaultValue:           "LinkedIn",
			CandidateMappedValue:   "LinkedIn",
			OptionMatchStrategy:    "substring_case_insensitive",
			ConfidenceThreshold:    0.90,
			CreatedAt:              now,
			UpdatedAt:              now,
		},
	}

	for _, emb := range embeddings {
		if err := emb.Validate(); err != nil {
			return fmt.Errorf("validate embedding %s: %w", emb.ID, err)
		}
		_, err := coll.UpdateOne(
			ctx,
			bson.M{"_id": emb.ID},
			bson.M{"$set": emb},
			options.Update().SetUpsert(true),
		)
		if err != nil {
			return fmt.Errorf("upsert embedding %s: %w", emb.ID, err)
		}
	}
	log.Printf("Seeded %d question embeddings across 10 categories.", len(embeddings))
	return nil
}

func seedJobFormSchemas(ctx context.Context, database *mongo.Database, normalizer *ats_inspector.ASTSchemaNormalizer) error {
	coll := database.Collection("job_form_schemas")

	schemasToSeed := []struct {
		provider  string
		companyID string
		jobID     string
		jobURL    string
		target    models.SubmissionTarget
		rawFields []ats_inspector.RawATSField
	}{
		// 1. Greenhouse: Figma (Job 5426468004)
		{
			provider:  "greenhouse",
			companyID: "figma",
			jobID:     "5426468004",
			jobURL:    "https://boards.greenhouse.io/figma/jobs/5426468004",
			target: models.SubmissionTarget{
				Tier:     1,
				Endpoint: "https://boards-api.greenhouse.io/v1/boards/figma/jobs/5426468004",
				Method:   "POST",
				Encoding: "multipart/form-data",
			},
			rawFields: []ats_inspector.RawATSField{
				{ID: "first_name", Label: "First Name", Type: "text", Required: true},
				{ID: "last_name", Label: "Last Name", Type: "text", Required: true},
				{ID: "email", Label: "Email", Type: "email", Required: true},
				{ID: "phone", Label: "Phone", Type: "tel", Required: true},
				{ID: "resume", Label: "Resume/CV", Type: "file", Required: true},
				{ID: "question_12497118004", Label: "LinkedIn Profile", Type: "text", Required: false},
				{ID: "question_12497120004", Label: "Why do you want to join Figma?", Type: "textarea", Required: true},
				{ID: "question_12497123004", Label: "From where do you intend to work?", Type: "text", Required: true},
				{ID: "question_12497125004", Label: "Are you authorized to work in the country for which you applied?", Type: "select", Required: true, Options: []string{"Yes", "No"}, OptionValues: map[string]any{"Yes": 1, "No": 0}},
			},
		},
		// 2. Greenhouse: Stripe (Job 8172510)
		{
			provider:  "greenhouse",
			companyID: "stripe",
			jobID:     "8172510",
			jobURL:    "https://boards.greenhouse.io/stripe/jobs/8172510",
			target: models.SubmissionTarget{
				Tier:     1,
				Endpoint: "https://boards-api.greenhouse.io/v1/boards/stripe/jobs/8172510",
				Method:   "POST",
				Encoding: "multipart/form-data",
			},
			rawFields: []ats_inspector.RawATSField{
				{ID: "first_name", Label: "First Name", Type: "text", Required: true},
				{ID: "last_name", Label: "Last Name", Type: "text", Required: true},
				{ID: "email", Label: "Email", Type: "email", Required: true},
				{ID: "phone", Label: "Phone", Type: "tel", Required: true},
				{ID: "resume", Label: "Resume/CV", Type: "file", Required: true},
				{ID: "question_68935507", Label: "Who is your current or previous employer?", Type: "text", Required: true},
				{ID: "question_68935511", Label: "Are you authorized to work in the location(s) selected?", Type: "select", Required: true, Options: []string{"Yes", "No"}, OptionValues: map[string]any{"Yes": 1, "No": 0}},
				{ID: "question_68935512", Label: "Will you require Stripe to sponsor you for a work permit?", Type: "select", Required: true, Options: []string{"Yes", "No"}, OptionValues: map[string]any{"Yes": 1, "No": 0}},
			},
		},
		// 3. Ashby: Linear (Job d3bc1ced-3ce4-4086-a050-555055dbb1ff)
		{
			provider:  "ashby",
			companyID: "linear",
			jobID:     "d3bc1ced-3ce4-4086-a050-555055dbb1ff",
			jobURL:    "https://jobs.ashbyhq.com/linear/d3bc1ced-3ce4-4086-a050-555055dbb1ff",
			target: models.SubmissionTarget{
				Tier:     1,
				Endpoint: "https://jobs.ashbyhq.com/api/non-user-graphql?op=ApiSubmitSingleApplicationFormAction",
				Method:   "POST",
				Encoding: "application/json",
			},
			rawFields: []ats_inspector.RawATSField{
				{ID: "6ae11654-56b7-4e26-910b-a424feafe680__systemfield_name", Label: "Full Name", Type: "text", Required: true},
				{ID: "6ae11654-56b7-4e26-910b-a424feafe680__systemfield_email", Label: "Email", Type: "email", Required: true},
				{ID: "6ae11654-56b7-4e26-910b-a424feafe680__systemfield_resume", Label: "Resume", Type: "file", Required: true},
				{ID: "6ae11654-56b7-4e26-910b-a424feafe680_1c843a13-4fe3-43df-afcf-37a69d88591b", Label: "GitHub", Type: "text", Required: true},
				{ID: "6ae11654-56b7-4e26-910b-a424feafe680_ca5c9d78-ec13-4bf2-86b8-bce6ff32e45e", Label: "LinkedIn", Type: "text", Required: false},
				{ID: "6ae11654-56b7-4e26-910b-a424feafe680_ce587bdf-31fa-4768-9e92-7741b7ad2317", Label: "Notice Period", Type: "text", Required: true},
			},
		},
		// 4. Lever: Palantir (Job ac978161-6f46-4f6b-ad9e-a258e642751c)
		{
			provider:  "lever",
			companyID: "palantir",
			jobID:     "ac978161-6f46-4f6b-ad9e-a258e642751c",
			jobURL:    "https://jobs.lever.co/palantir/ac978161-6f46-4f6b-ad9e-a258e642751c",
			target: models.SubmissionTarget{
				Tier:     1,
				Endpoint: "https://jobs.lever.co/palantir/ac978161-6f46-4f6b-ad9e-a258e642751c/apply",
				Method:   "POST",
				Encoding: "multipart/form-data",
			},
			rawFields: []ats_inspector.RawATSField{
				{ID: "name", Label: "Full Name", Type: "text", Required: true},
				{ID: "email", Label: "Email", Type: "email", Required: true},
				{ID: "phone", Label: "Phone", Type: "tel", Required: false},
				{ID: "location", Label: "Current Location", Type: "text", Required: true},
				{ID: "resume", Label: "Resume/CV", Type: "file", Required: true},
				{ID: "urls[LinkedIn]", Label: "LinkedIn", Type: "text", Required: false},
				{ID: "urls[GitHub]", Label: "GitHub", Type: "text", Required: false},
			},
		},
		// 5. SmartRecruiters: Western Digital (Job 744000162024500)
		{
			provider:  "smartrecruiters",
			companyID: "westerndigital",
			jobID:     "744000162024500",
			jobURL:    "https://jobs.smartrecruiters.com/WesternDigital/744000162024500",
			target: models.SubmissionTarget{
				Tier:     2,
				Endpoint: "https://jobs.smartrecruiters.com/WesternDigital/744000162024500/apply",
				Method:   "POST",
				Encoding: "window.__oneRoleApply",
			},
			rawFields: []ats_inspector.RawATSField{
				{ID: "firstName", Selector: "input#first-name-input, input[name*='firstName' i]", Label: "First Name", Type: "text", Required: true},
				{ID: "lastName", Selector: "input#last-name-input, input[name*='lastName' i]", Label: "Last Name", Type: "text", Required: true},
				{ID: "email", Selector: "input#email-input, input[name*='email' i]", Label: "Email", Type: "email", Required: true},
				{ID: "phoneNumber", Selector: "input#phone-number-input, input[name*='phone' i]", Label: "Phone", Type: "tel", Required: false},
				{ID: "city", Selector: "input#location-input, input[name*='location' i]", Label: "City", Type: "text", Required: false},
				{ID: "resume", Selector: "input[type='file']", Label: "Resume", Type: "file", Required: true},
				{ID: "consent_privacy", Selector: "input[type='checkbox']", Label: "Consent", Type: "checkbox", Required: true},
			},
		},
		// 6. Workday: AdventHealth (Job R-0411385)
		{
			provider:  "workday",
			companyID: "adventhealth",
			jobID:     "R-0411385",
			jobURL:    "https://adventhealth.wd12.myworkdayjobs.com/AH_External_Career_Site/job/Orlando/RN_R-0411385",
			target: models.SubmissionTarget{
				Tier:     2,
				Endpoint: "https://adventhealth.wd12.myworkdayjobs.com/wday/cxs/adventhealth/AH_External_Career_Site/jobs/R-0411385/apply",
				Method:   "POST",
				Encoding: "window.__oneRoleApply",
			},
			rawFields: []ats_inspector.RawATSField{
				{ID: "legalNameSection_firstName", Selector: "[data-automation-id='legalNameSection_firstName']", Label: "First Name", Type: "text", Required: true},
				{ID: "legalNameSection_lastName", Selector: "[data-automation-id='legalNameSection_lastName']", Label: "Last Name", Type: "text", Required: true},
				{ID: "addressSection_city", Selector: "[data-automation-id='addressSection_city']", Label: "City", Type: "text", Required: false},
				{ID: "email", Selector: "[data-automation-id='email']", Label: "Email", Type: "email", Required: true},
				{ID: "phone-number", Selector: "[data-automation-id='phone-number']", Label: "Phone", Type: "tel", Required: false},
				{ID: "file-upload-input", Selector: "input[type='file'][data-automation-id='file-upload-input']", Label: "Resume", Type: "file", Required: true},
				{ID: "workAuthQuestion", Selector: "div[data-automation-id*='questionnaire'] input[type='radio']", Label: "Are you authorized to work in the US?", Type: "radio", Required: true, Options: []string{"Yes", "No"}},
				{ID: "agreement", Selector: "input[type='checkbox'][data-automation-id*='agreement']", Label: "Agreement", Type: "checkbox", Required: true},
			},
		},
	}

	for _, s := range schemasToSeed {
		schema := normalizer.Normalize(
			s.provider,
			s.companyID,
			s.jobID,
			s.jobURL,
			s.target,
			s.rawFields,
			nil,
		)
		if err := schema.Validate(); err != nil {
			return fmt.Errorf("validate schema %s: %w", schema.ID, err)
		}
		_, err := coll.UpdateOne(
			ctx,
			bson.M{"_id": schema.ID},
			bson.M{"$set": schema},
			options.Update().SetUpsert(true),
		)
		if err != nil {
			return fmt.Errorf("upsert schema %s: %w", schema.ID, err)
		}
	}

	log.Printf("Seeded %d normalized job form schemas with pre-compiled ASTs.", len(schemasToSeed))
	return nil
}
