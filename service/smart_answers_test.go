package service

import (
	"testing"

	"github.com/oneRole/models"
)

func TestSmartAnswerEngine(t *testing.T) {
	profile := &models.CandidateProfile{
		FirstName:            "John",
		LastName:             "Doe",
		Email:                "john@example.com",
		Phone:                "555-123-4567",
		City:                 "San Francisco",
		State:                "CA",
		Country:              "United States",
		AuthorizedToWorkInUS: true,
		RequiresSponsorship:  false,
		YearsOfExperience:    5,
		CurrentEmployer:      "Acme Tech",
		CurrentJobTitle:      "Senior Backend Engineer",
		DesiredSalaryMin:     150000,
		OpenToRemote:         true,
		HighestDegree:        "Master's Degree",
		LinkedInURL:          "https://linkedin.com/in/johndoe",
		GitHubURL:            "https://github.com/johndoe",
	}

	engine := NewSmartAnswerEngine()

	tests := []struct {
		name     string
		question ATSQuestion
		want     string
	}{
		{
			name:     "Work Authorization",
			question: ATSQuestion{Label: "Are you legally authorized to work in the United States?", Required: true},
			want:     "Yes",
		},
		{
			name:     "Sponsorship Requirement",
			question: ATSQuestion{Label: "Will you now or in the future require visa sponsorship?", Required: true},
			want:     "No",
		},
		{
			name:     "Years of Experience",
			question: ATSQuestion{Label: "How many years of relevant software engineering experience do you have?", Required: true},
			want:     "5",
		},
		{
			name:     "Current Employer",
			question: ATSQuestion{Label: "Who is your current employer?", Required: true},
			want:     "Acme Tech",
		},
		{
			name:     "Current Job Title",
			question: ATSQuestion{Label: "What is your current job title?", Required: true},
			want:     "Senior Backend Engineer",
		},
		{
			name:     "Remote Work Interest",
			question: ATSQuestion{Label: "Are you interested in remote or hybrid work?", Required: true},
			want:     "Yes",
		},
		{
			name:     "LinkedIn URL",
			question: ATSQuestion{Label: "LinkedIn Profile URL", Required: false},
			want:     "https://linkedin.com/in/johndoe",
		},
		{
			name:     "EEO Gender Decline",
			question: ATSQuestion{Label: "Gender Identity", Options: []string{"Male", "Female", "Non-binary", "Decline to Self-Identify"}, Required: false},
			want:     "Decline to Self-Identify",
		},
		{
			name:     "Salary Text Default Range",
			question: ATSQuestion{Label: "Desired Salary", Type: "text"},
			want:     "150,000-220,000",
		},
		{
			name:     "Salary Numeric Field",
			question: ATSQuestion{Label: "Desired Salary", Type: "number"},
			want:     "185000",
		},
		{
			name:     "Salary HTML5 Numeric Pattern",
			question: ATSQuestion{Label: "Desired Salary", Pattern: "[0-9]*"},
			want:     "185000",
		},
		{
			name:     "Salary Numbers Only Label",
			question: ATSQuestion{Label: "Desired Salary (numbers only)"},
			want:     "185000",
		},
		{
			name:     "Salary Digits Only Label",
			question: ATSQuestion{Label: "Target Annual Compensation (digits only, no commas)"},
			want:     "185000",
		},
		{
			name:     "Hourly Rate Text",
			question: ATSQuestion{Label: "Desired Hourly Rate ($/hr)", Type: "text"},
			want:     "$90/hr",
		},
		{
			name:     "Hourly Rate Numeric Type",
			question: ATSQuestion{Label: "Desired Hourly Rate ($/hr)", Type: "number"},
			want:     "90",
		},
		{
			name:     "Hourly Rate Numeric Pattern",
			question: ATSQuestion{Label: "Hourly Rate expectation", Pattern: "^[0-9]+$"},
			want:     "90",
		},
		{
			name:     "Hourly Rate Numbers Only Label",
			question: ATSQuestion{Label: "What is your hourly rate? (numbers only)"},
			want:     "90",
		},
		{
			name:     "Hourly Rate Dropdown Selection",
			question: ATSQuestion{Label: "Hourly rate range", Options: []string{"$50-70/hr", "$70-90/hr", "$90-110/hr"}},
			want:     "$90-110/hr",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := engine.AnswerQuestion(tt.question, profile)
			if got != tt.want {
				t.Errorf("AnswerQuestion() = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("Zero NA Answers And Rich Technical Question Generation", func(t *testing.T) {
		q1 := ATSQuestion{Label: "Why did you apply for this role?", Required: true}
		ans1 := engine.AnswerQuestionWithJob(q1, profile, JobAnswerContext{JobTitle: "Lead Golang Architect", CompanyName: "Stripe"})
		if ans1 == "N/A" || ans1 == "" || !containsSubstring(ans1, "Stripe") {
			t.Fatalf("expected tailored response containing Stripe, got %q", ans1)
		}

		q2 := ATSQuestion{Label: "Describe your experience with Go and distributed systems", Required: true}
		ans2 := engine.AnswerQuestion(q2, profile)
		if ans2 == "N/A" || ans2 == "" || !containsSubstring(ans2, "Go (Golang)") {
			t.Fatalf("expected tailored tech response containing Go (Golang), got %q", ans2)
		}

		q3 := ATSQuestion{Label: "Tell us about a challenging project you delivered", Required: true}
		ans3 := engine.AnswerQuestion(q3, profile)
		if ans3 == "N/A" || ans3 == "" || !containsSubstring(ans3, "Acme Tech") {
			t.Fatalf("expected tailored achievement response, got %q", ans3)
		}

		q4 := ATSQuestion{Label: "What is your approach to handling cross-functional team feedback?", Required: true}
		ans4 := engine.AnswerQuestion(q4, profile)
		if ans4 == "N/A" || ans4 == "" {
			t.Fatalf("expected professional fallback, got %q", ans4)
		}

		// Keyword Collision Protection Tests:
		// 1. "collaborate" contains "rate", must not trigger rxSalary
		qCollab := ATSQuestion{Label: "How do you collaborate with engineering teams?", Type: "textarea"}
		ansCollab := engine.AnswerQuestion(qCollab, profile)
		if containsSubstring(ansCollab, "150,000") || containsSubstring(ansCollab, "220,000") || containsSubstring(ansCollab, "185000") {
			t.Fatalf("collaboration question collided with salary handler, got %q", ansCollab)
		}
		if !containsSubstring(ansCollab, "collaborative alignment") && !containsSubstring(ansCollab, "technical disagreements") {
			t.Fatalf("expected teamwork response for collaboration, got %q", ansCollab)
		}

		// 2. "ethnicity" contains "city", must not trigger rxLocation
		qEth := ATSQuestion{Label: "What is your race / ethnicity?", Options: []string{"Asian", "White", "Black", "Decline to specify"}}
		ansEth := engine.AnswerQuestion(qEth, profile)
		if ansEth == "San Francisco, CA" || ansEth == "California" || ansEth == "United States" {
			t.Fatalf("ethnicity collided with location handler, got %q", ansEth)
		}
		if ansEth != "Asian" {
			t.Fatalf("expected ethnicity to resolve Asian or Decline, got %q", ansEth)
		}
	})
}

func TestSmartAnswerEngine_UserPreferencesAndBasicQuestions(t *testing.T) {
	profile := &models.CandidateProfile{
		FirstName:            "Nakul",
		LastName:             "Bhandare",
		Email:                "bhandarenakul11@gmail.com",
		City:                 "Austin",
		State:                "TX",
		Country:              "Canada",
		PortfolioURL:         "https://nakulbhandare.dev",
		AuthorizedToWorkInUS: true,
		RequiresSponsorship:  true,
		CustomFieldAnswers: map[string]string{
			"work_authorization": "Yes",
			"visa_sponsorship":   "Yes",
			"country":            "Canada",
			"portfolio_url":      "https://nakulbhandare.dev",
		},
	}

	engine := NewSmartAnswerEngine()

	// 1. Sponsorship should resolve to Yes based on candidate preference
	qSpons := ATSQuestion{Label: "Will you now or in the future require visa sponsorship?", Options: []string{"Yes", "No"}}
	ansSpons := engine.AnswerQuestion(qSpons, profile)
	if ansSpons != "Yes" {
		t.Errorf("expected sponsorship 'Yes', got %q", ansSpons)
	}

	// 2. Portfolio URL
	qPort := ATSQuestion{Label: "Portfolio or Personal Website URL"}
	ansPort := engine.AnswerQuestion(qPort, profile)
	if ansPort != "https://nakulbhandare.dev" {
		t.Errorf("expected portfolio 'https://nakulbhandare.dev', got %q", ansPort)
	}

	// 3. Country of residence
	qCountry := ATSQuestion{Label: "Country of residence"}
	ansCountry := engine.AnswerQuestion(qCountry, profile)
	if ansCountry != "Canada" {
		t.Errorf("expected country 'Canada', got %q", ansCountry)
	}

	// 4. FormatCandidatePayload should preserve custom field answers and basic fields
	candMap := FormatCandidatePayload(profile, "Cover letter", nil)
	if candMap["country"] != "Canada" {
		t.Errorf("expected candidate country 'Canada', got %v", candMap["country"])
	}
	if candMap["portfolio"] != "https://nakulbhandare.dev" {
		t.Errorf("expected candidate portfolio 'https://nakulbhandare.dev', got %v", candMap["portfolio"])
	}
	answers, ok := candMap["answers"].(map[string]string)
	if !ok {
		t.Fatalf("expected answers map in candidate payload")
	}
	if answers["sponsorship"] != "Yes" {
		t.Errorf("expected sponsorship 'Yes' in answers, got %q", answers["sponsorship"])
	}
	if answers["country"] != "Canada" {
		t.Errorf("expected country 'Canada' in answers, got %q", answers["country"])
	}
	if answers["portfolio_url"] != "https://nakulbhandare.dev" {
		t.Errorf("expected portfolio_url 'https://nakulbhandare.dev' in answers, got %q", answers["portfolio_url"])
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && len(sub) > 0 && (s[:len(sub)] == sub || s[len(s)-len(sub):] == sub || len(s) > len(sub) && searchIn(s, sub)))
}

func searchIn(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
