package service

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/oneRole/models"
)

// ============================================================================
// TASK 1: UNUSUAL CASING, UNICODE SPACES, TRAILING PUNCTUATION, COMPLEX EEO
// ============================================================================

func TestAdversarialCasingUnicodePunctuation(t *testing.T) {
	profile := &models.CandidateProfile{
		FirstName:            "Nakul",
		LastName:             "Bhandare",
		Email:                "bhandarenakul11@gmail.com",
		Phone:                "+1 (350)-355-6570",
		City:                 "San Francisco",
		State:                "CA",
		Country:              "United States",
		AuthorizedToWorkInUS: true,
		RequiresSponsorship:  false,
		YearsOfExperience:    7,
		CurrentEmployer:      "Kyndryl",
		CurrentJobTitle:      "Senior Backend Engineer",
		DesiredSalaryMin:     150000,
		OpenToRemote:         true,
		HighestDegree:        "Master's Degree",
		LinkedInURL:          "https://linkedin.com/in/nakul-bhandare",
		GitHubURL:            "https://github.com/nakulbhandare",
	}

	engine := NewAnswerService()

	tests := []struct {
		name     string
		question ATSQuestion
		validate func(t *testing.T, got string)
	}{
		// 1. Unusual Casing
		{
			name: "All-caps Work Authorization",
			question: ATSQuestion{
				Label: "ARE YOU LEGALLY AUTHORIZED TO WORK IN THE UNITED STATES?",
			},
			validate: func(t *testing.T, got string) {
				if got != "Yes" {
					t.Errorf("got %q, want Yes", got)
				}
			},
		},
		{
			name: "All-caps Sponsorship Requirement",
			question: ATSQuestion{
				Label: "WILL YOU NOW OR IN THE FUTURE REQUIRE EMPLOYMENT VISA SPONSORSHIP?",
			},
			validate: func(t *testing.T, got string) {
				if got != "No" {
					t.Errorf("got %q, want No", got)
				}
			},
		},
		{
			name: "SpOnGeBoB Casing Desired Salary Text",
			question: ATSQuestion{
				Label: "dEsIrEd SaLaRy AnD cOmPeNsAtIoN",
				Type:  "text",
			},
			validate: func(t *testing.T, got string) {
				if got != "150,000-220,000" {
					t.Errorf("got %q, want 150,000-220,000", got)
				}
			},
		},
		{
			name: "Mixed Casing Years of Experience",
			question: ATSQuestion{
				Label: "HoW mAnY yEaRs Of ReLeVaNt ExPeRiEnCe Do YoU hAvE?",
			},
			validate: func(t *testing.T, got string) {
				if got != "7" {
					t.Errorf("got %q, want 7", got)
				}
			},
		},
		// 2. Unicode Whitespace (NBSP, Em Space, Thin Space, Tabs, Newlines)
		{
			name: "NBSP in Work Authorization",
			question: ATSQuestion{
				Label: "Are\u00A0you\u00A0legally\u00A0authorized\u00A0to\u00A0work\u00A0in\u00A0the\u00A0United\u00A0States?",
			},
			validate: func(t *testing.T, got string) {
				if got != "Yes" {
					t.Errorf("got %q, want Yes", got)
				}
			},
		},
		{
			name: "Em Space and Thin Space in Salary",
			question: ATSQuestion{
				Label: "\u2003Desired\u2009Salary\u2003",
				Type:  "text",
			},
			validate: func(t *testing.T, got string) {
				if got != "150,000-220,000" {
					t.Errorf("got %q, want 150,000-220,000", got)
				}
			},
		},
		{
			name: "Tabs and Newlines in Employer Label",
			question: ATSQuestion{
				Label: "\t\r\nWho is your current employer?\r\n\t",
			},
			validate: func(t *testing.T, got string) {
				if got != "Kyndryl" {
					t.Errorf("got %q, want Kyndryl", got)
				}
			},
		},
		// 3. Trailing and Leading Punctuation
		{
			name: "Multiple question marks and trailing spaces in Sponsorship",
			question: ATSQuestion{
				Label: "Will you require visa sponsorship???   ",
			},
			validate: func(t *testing.T, got string) {
				if got != "No" {
					t.Errorf("got %q, want No", got)
				}
			},
		},
		{
			name: "Interrobang and quotes in Remote interest",
			question: ATSQuestion{
				Label: `"Are you open to remote or hybrid work?!"`,
			},
			validate: func(t *testing.T, got string) {
				if got != "Yes" {
					t.Errorf("got %q, want Yes", got)
				}
			},
		},
		{
			name: "Trailing colon and parenthetical in Current Title",
			question: ATSQuestion{
				Label: "Current Job Title (most recent position):",
			},
			validate: func(t *testing.T, got string) {
				if got != "Senior Backend Engineer" {
					t.Errorf("got %q, want Senior Backend Engineer", got)
				}
			},
		},
		{
			name: "Brackets and tags in LinkedIn URL",
			question: ATSQuestion{
				Label: "[Profile] LinkedIn URL (optional):",
			},
			validate: func(t *testing.T, got string) {
				if got != "https://linkedin.com/in/nakul-bhandare" {
					t.Errorf("got %q, want https://linkedin.com/in/nakul-bhandare", got)
				}
			},
		},
		{
			name: "Ellipsis and semicolon in Notice period",
			question: ATSQuestion{
				Label: "Availability / Notice Period...;",
			},
			validate: func(t *testing.T, got string) {
				if got != "Immediately / 2 weeks" {
					t.Errorf("got %q, want Immediately / 2 weeks", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := engine.AnswerQuestion(tt.question, profile)
			tt.validate(t, got)
		})
	}
}

func TestAdversarialEEOMultiOptions(t *testing.T) {
	profile := &models.CandidateProfile{
		FirstName: "Nakul",
		LastName:  "Bhandare",
	}

	engine := NewAnswerService()

	t.Run("EEO Gender Decline Placed Last in List", func(t *testing.T) {
		q := ATSQuestion{
			Label:   "Gender Identity",
			Options: []string{"Male", "Female", "Non-Binary / Gender Non-Conforming", "I prefer not to say"},
		}
		got := engine.AnswerQuestion(q, profile)
		if got != "I prefer not to say" {
			t.Errorf("got %q, want 'I prefer not to say'", got)
		}
	})

	t.Run("EEO Gender Various Opt-Out Formats", func(t *testing.T) {
		variants := []struct {
			name    string
			options []string
			want    string
		}{
			{
				name:    "Decline to specify",
				options: []string{"Male", "Female", "Decline to specify"},
				want:    "Decline to specify",
			},
			{
				name:    "Do not wish to self-identify",
				options: []string{"Male", "Female", "I do not wish to self-identify"},
				want:    "I do not wish to self-identify",
			},
			{
				name:    "Opt out phrasing",
				options: []string{"Male", "Female", "Opt out of question"},
				want:    "Opt out of question",
			},
			{
				name:    "Prefer not to answer",
				options: []string{"Man", "Woman", "Non-binary", "Prefer not to answer"},
				want:    "Prefer not to answer",
			},
			{
				name:    "Not specified option",
				options: []string{"Male", "Female", "Not specified"},
				want:    "Not specified",
			},
		}

		for _, v := range variants {
			t.Run(v.name, func(t *testing.T) {
				q := ATSQuestion{Label: "Gender", Options: v.options}
				got := engine.AnswerQuestion(q, profile)
				if got != v.want {
					t.Errorf("got %q, want %q", got, v.want)
				}
			})
		}
	})

	t.Run("EEO Race Standard EEOC 7-Category List", func(t *testing.T) {
		eeocOptions := []string{
			"Hispanic or Latino",
			"White (Not Hispanic or Latino)",
			"Black or African American (Not Hispanic or Latino)",
			"Native Hawaiian or Other Pacific Islander (Not Hispanic or Latino)",
			"Asian (Not Hispanic or Latino)",
			"American Indian or Alaska Native (Not Hispanic or Latino)",
			"Two or More Races (Not Hispanic or Latino)",
			"Decline to State",
		}
		q := ATSQuestion{Label: "Race / Ethnicity", Options: eeocOptions}
		got := engine.AnswerQuestion(q, profile)
		if got != "Asian (Not Hispanic or Latino)" {
			t.Errorf("got %q, want 'Asian (Not Hispanic or Latino)'", got)
		}

		// When Asian is absent, fallback to Decline
		noAsianOptions := []string{
			"Hispanic or Latino",
			"White",
			"Black",
			"Prefer not to disclose",
		}
		q2 := ATSQuestion{Label: "Ethnicity", Options: noAsianOptions}
		got2 := engine.AnswerQuestion(q2, profile)
		if got2 != "Prefer not to disclose" {
			t.Errorf("got %q, want 'Prefer not to disclose'", got2)
		}
	})

	t.Run("EEO Veteran Protection Status", func(t *testing.T) {
		veteranOptions := []string{
			"I am a protected veteran",
			"I am not a protected veteran",
			"I do not wish to answer",
		}
		q := ATSQuestion{Label: "Veteran Status", Options: veteranOptions}
		got := engine.AnswerQuestion(q, profile)
		if got != "I am not a protected veteran" {
			t.Errorf("got %q, want 'I am not a protected veteran'", got)
		}
	})

	t.Run("EEO Disability Identification", func(t *testing.T) {
		disabilityOptions := []string{
			"Yes, I have a disability, or have a history/record of having a disability",
			"No, I don't have a disability, or have a history/record of having a disability",
			"I do not wish to answer",
		}
		q := ATSQuestion{Label: "Voluntary Self-Identification of Disability", Options: disabilityOptions}
		got := engine.AnswerQuestion(q, profile)
		if got != "No, I don't have a disability, or have a history/record of having a disability" {
			t.Errorf("got %q, want 'No, I don't have a disability...'", got)
		}
	})
}

// ============================================================================
// TASK 2: STRESS TEST isNumericQuestion
// ============================================================================

func TestStressIsNumericQuestion(t *testing.T) {
	tests := []struct {
		name     string
		q        ATSQuestion
		label    string
		expected bool
	}{
		// Type checks
		{name: "Type number lower", q: ATSQuestion{Type: "number"}, expected: true},
		{name: "Type number upper", q: ATSQuestion{Type: "NUMBER"}, expected: true},
		{name: "Type numeric lower", q: ATSQuestion{Type: "numeric"}, expected: true},
		{name: "Type numeric mixed", q: ATSQuestion{Type: "NuMeRiC"}, expected: true},
		{name: "Type integer lower", q: ATSQuestion{Type: "integer"}, expected: true},
		{name: "Type integer upper", q: ATSQuestion{Type: "INTEGER"}, expected: true},
		{name: "Type text non-numeric", q: ATSQuestion{Type: "text"}, expected: false},
		{name: "Type textarea non-numeric", q: ATSQuestion{Type: "textarea"}, expected: false},
		{name: "Type select non-numeric", q: ATSQuestion{Type: "select"}, expected: false},

		// Pattern checks
		{name: "Pattern [0-9]*", q: ATSQuestion{Pattern: "[0-9]*"}, expected: true},
		{name: "Pattern ^[0-9]+$", q: ATSQuestion{Pattern: "^[0-9]+$"}, expected: true},
		{name: "Pattern \\d+", q: ATSQuestion{Pattern: `\d+`}, expected: true},
		{name: "Pattern \\d{1,6}", q: ATSQuestion{Pattern: `\d{1,6}`}, expected: true},
		{name: "Pattern [0-9]{3,7}", q: ATSQuestion{Pattern: `[0-9]{3,7}`}, expected: true},
		{name: "Pattern ^[0-9]*$", q: ATSQuestion{Pattern: `^[0-9]*$`}, expected: true},
		{name: "Pattern non-numeric regex", q: ATSQuestion{Pattern: `^[a-zA-Z\s]+$`}, expected: false},
		{name: "Pattern email regex", q: ATSQuestion{Pattern: `.+@.+\..+`}, expected: false},

		// Label checks
		{name: "Label numbers only", label: "desired salary (numbers only)", expected: true},
		{name: "Label number only", label: "desired salary (number only)", expected: true},
		{name: "Label digits only", label: "salary in digits only", expected: true},
		{name: "Label digit only", label: "salary in digit only", expected: true},
		{name: "Label in numbers", label: "what is your compensation in numbers?", expected: true},
		{name: "Label in digits", label: "compensation in digits please", expected: true},
		{name: "Label numeric keyword", label: "numeric salary expectation", expected: true},
		{name: "Label integer keyword", label: "expected integer rate", expected: true},
		{name: "Label whole number", label: "salary as whole number", expected: true},
		{name: "Label no commas", label: "salary (no commas)", expected: true},
		{name: "Label without commas", label: "salary without commas", expected: true},
		{name: "Label no symbols", label: "salary (no symbols)", expected: true},
		{name: "Label no currency", label: "salary with no currency symbol", expected: true},
		{name: "Label standard text without numeric instruction", label: "what is your desired salary?", expected: false},

		// Description checks
		{
			name:     "Description numbers only",
			q:        ATSQuestion{Description: "Please enter numbers only without currency symbols or commas."},
			label:    "desired salary",
			expected: true,
		},
		{
			name:     "Description digits only",
			q:        ATSQuestion{Description: "Digits only, e.g. 185000"},
			label:    "annual compensation",
			expected: true,
		},
		{
			name:     "Description no commas",
			q:        ATSQuestion{Description: "Please provide your answer with no commas or periods"},
			label:    "expected compensation",
			expected: true,
		},
		{
			name:     "Description non-numeric",
			q:        ATSQuestion{Description: "Include target base salary and any bonus expectations"},
			label:    "expected compensation",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lbl := tt.label
			if lbl == "" && tt.q.Label != "" {
				lbl = tt.q.Label
			}
			got := isNumericQuestion(tt.q, lbl)
			if got != tt.expected {
				t.Errorf("isNumericQuestion(%+v, %q) = %v, want %v", tt.q, lbl, got, tt.expected)
			}
		})
	}
}

// ============================================================================
// TASK 3: CONCURRENT 50 GOROUTINES RACE DETECTOR (go test -race)
// ============================================================================

func TestConcurrentGoroutinesRaceDetector(t *testing.T) {
	profile := &models.CandidateProfile{
		FirstName:            "Nakul",
		LastName:             "Bhandare",
		Email:                "bhandarenakul11@gmail.com",
		Phone:                "+1 (350)-355-6570",
		City:                 "San Francisco",
		State:                "CA",
		Country:              "United States",
		AuthorizedToWorkInUS: true,
		RequiresSponsorship:  false,
		YearsOfExperience:    7,
		CurrentEmployer:      "Kyndryl",
		CurrentJobTitle:      "Senior Backend Engineer",
		DesiredSalaryMin:     150000,
		OpenToRemote:         true,
		HighestDegree:        "Master's Degree",
		LinkedInURL:          "https://linkedin.com/in/nakul-bhandare",
		GitHubURL:            "https://github.com/nakulbhandare",
	}

	engine := NewAnswerService()

	questions := []ATSQuestion{
		{Label: "Are you legally authorized to work in the United States?"},
		{Label: "Will you now or in the future require visa sponsorship?"},
		{Label: "How many years of software engineering experience do you have?"},
		{Label: "Who is your current employer?"},
		{Label: "What is your current job title?"},
		{Label: "Desired Salary", Type: "text"},
		{Label: "Desired Salary", Type: "number"},
		{Label: "Desired Salary (numbers only)", Type: "text"},
		{Label: "Desired Hourly Rate ($/hr)", Type: "text"},
		{Label: "Desired Hourly Rate ($/hr)", Type: "number"},
		{Label: "Gender Identity", Options: []string{"Male", "Female", "Non-binary", "Decline to Self-Identify"}},
		{Label: "Race / Ethnicity", Options: []string{"Asian", "White", "Black", "Decline to specify"}},
		{Label: "Veteran Status", Options: []string{"I am a protected veteran", "I am not a protected veteran", "Decline"}},
		{Label: "Disability Status", Options: []string{"Yes", "No", "Decline"}},
		{Label: "Why do you want to work at our company?"},
		{Label: "Describe your experience with Go and microservices"},
		{Label: "Tell us about a challenging project you delivered"},
		{Label: "LinkedIn Profile URL"},
		{Label: "GitHub Profile URL"},
		{Label: "Notice Period / Availability"},
		{Label: "Are you open to hybrid office cadence in San Francisco?"},
		{Label: "Have you ever been convicted of a felony?"},
		{Label: "Are you at least 18 years of age?"},
		{Label: "Please confirm you have read and acknowledge the agreement", Options: []string{"I read this", "Decline"}},
	}

	const numGoroutines = 50
	const iterationsPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errChan := make(chan error, numGoroutines*iterationsPerGoroutine)

	for g := 0; g < numGoroutines; g++ {
		go func(goroutineID int) {
			defer wg.Done()
			for i := 0; i < iterationsPerGoroutine; i++ {
				q := questions[(goroutineID+i)%len(questions)]

				if i%2 == 0 {
					ans := engine.AnswerQuestion(q, profile)
					if ans == "" {
						errChan <- fmt.Errorf("goroutine %d iteration %d: AnswerQuestion returned empty string for %q", goroutineID, i, q.Label)
						return
					}
				} else {
					jobCtx := JobAnswerContext{
						JobTitle:    fmt.Sprintf("Staff Engineer %d", i%5),
						CompanyName: fmt.Sprintf("Enterprise Co %d", goroutineID%10),
					}
					ans := engine.AnswerQuestionWithJob(q, profile, jobCtx)
					if ans == "" {
						errChan <- fmt.Errorf("goroutine %d iteration %d: AnswerQuestionWithJob returned empty string for %q", goroutineID, i, q.Label)
						return
					}
				}
			}
		}(g)
	}

	wg.Wait()
	close(errChan)

	var errorsFound []error
	for err := range errChan {
		errorsFound = append(errorsFound, err)
	}

	if len(errorsFound) > 0 {
		t.Fatalf("encountered %d errors during concurrent execution; first error: %v", len(errorsFound), errorsFound[0])
	}
}

func TestAdversarialFallbackNeverReturnsNA(t *testing.T) {
	profile := &models.CandidateProfile{
		FirstName:         "Nakul",
		LastName:          "Bhandare",
		CurrentJobTitle:   "Senior Backend Engineer",
		YearsOfExperience: 7,
		PrimarySkills:     []string{"Go", "Distributed Systems", "PostgreSQL", "Kafka"},
	}

	engine := NewAnswerService()

	bizarreQuestions := []ATSQuestion{
		{Label: "If you were an animal in an agile sprint, which animal would you be and why?"},
		{Label: "What is your favorite distributed consensus algorithm and how does it inspire your breakfast routine?"},
		{Label: "Describe a scenario where you had to debug an issue without electricity."},
		{Label: "Can you fly a spaceship to Mars if required for this position?"},
		{Label: "Do you enjoy optimizing database b-trees on weekends?"},
		{Label: "Are you capable of writing 10,000 lines of bug-free assembly code in one sitting?"},
		{Label: ""}, // Completely blank question label
	}

	for _, q := range bizarreQuestions {
		ans := engine.AnswerQuestion(q, profile)
		if ans == "N/A" || strings.TrimSpace(ans) == "" {
			t.Errorf("question %q returned invalid answer %q; expected non-empty professional fallback", q.Label, ans)
		}
	}
}

// ============================================================================
// EMPIRICAL DEFECT REPRODUCTIONS (EVIDENCE FOR REQUEST_CHANGES VERDICT)
// ============================================================================

func TestEmpiricalDefects(t *testing.T) {
	profile := &models.CandidateProfile{
		FirstName:            "Nakul",
		LastName:             "Bhandare",
		CurrentJobTitle:      "Senior Backend Engineer",
		CurrentEmployer:      "Kyndryl",
		YearsOfExperience:    7,
		AuthorizedToWorkInUS: true,
	}

	t.Run("Defect1_CrossCompanyCachePoisoning", func(t *testing.T) {
		engine := NewAnswerService()
		qWhy := ATSQuestion{Label: "Why do you want to work here?"}

		// First apply to Stripe
		ansStripe := engine.AnswerQuestionWithJob(qWhy, profile, JobAnswerContext{
			JobTitle:    "Staff Backend Engineer",
			CompanyName: "Stripe",
		})
		if !strings.Contains(ansStripe, "Stripe") {
			t.Fatalf("first answer expected Stripe, got %q", ansStripe)
		}

		// Second apply to Netflix
		ansNetflix := engine.AnswerQuestionWithJob(qWhy, profile, JobAnswerContext{
			JobTitle:    "Principal Distributed Systems Engineer",
			CompanyName: "Netflix",
		})

		if strings.Contains(ansNetflix, "Stripe") {
			t.Errorf("[DEFECT REPRODUCED] Cross-company cache poisoning! Netflix application contains Stripe: %q", ansNetflix)
		}
	})

	t.Run("Defect2_EEOGenderBinaryFemaleMaleSubstringCollision", func(t *testing.T) {
		engine := NewAnswerService()
		// When an ATS form offers only binary Female/Male (no decline option)
		q := ATSQuestion{
			Label:   "Gender",
			Options: []string{"Female", "Male"},
		}
		got := engine.AnswerQuestion(q, profile)
		// Nakul Bhandare is Male, but matchOption checks if "female" contains "male" -> true!
		if got == "Female" {
			t.Errorf("[DEFECT REPRODUCED] Gender 'male' matched substring in 'Female'! Got %q, want 'Male'", got)
		}
	})

	t.Run("Defect3_EEORaceCaucasianAsianSubstringCollision", func(t *testing.T) {
		engine := NewAnswerService()
		// When options list Caucasian before Asian
		q := ATSQuestion{
			Label:   "Race / Ethnicity",
			Options: []string{"Caucasian", "Asian", "Black or African American"},
		}
		got := engine.AnswerQuestion(q, profile)
		// Nakul Bhandare is Asian, but "caucasian" contains "asian" -> returns Caucasian!
		if got == "Caucasian" {
			t.Errorf("[DEFECT REPRODUCED] Race 'asian' matched substring in 'Caucasian'! Got %q, want 'Asian'", got)
		}
	})

	t.Run("Defect4_RegexNonDigitPatternInversion", func(t *testing.T) {
		// Pattern ^\D+$ explicitly specifies non-digits (e.g. for Name fields)
		q := ATSQuestion{Pattern: `^\D+$`}
		got := isNumericQuestion(q, "legal name")
		if got {
			t.Errorf("[DEFECT REPRODUCED] strings.ToLower transformed '\\D' to '\\d', classifying non-digit pattern as numeric!")
		}
	})

	t.Run("Defect5_BrokenWordBoundariesCPlusPlusCSharp", func(t *testing.T) {
		techCPP := extractTechFromLabel("Describe your experience with C++ in production")
		if techCPP == "" {
			t.Errorf("[DEFECT REPRODUCED] extractTechFromLabel failed for C++ due to trailing \\b after '+'")
		}

		techCS := extractTechFromLabel("Describe your experience with C# in production")
		if techCS == "" {
			t.Errorf("[DEFECT REPRODUCED] extractTechFromLabel failed for C# due to trailing \\b after '#'")
		}
	})
}

// ============================================================================
// MILESTONE 1 ITERATION 2 ADVERSARIAL CHALLENGE PROBES
// Probing: Option Matching, Cache Isolation, and Custom Field Overrides
// ============================================================================

func TestIteration2AdversarialProbes(t *testing.T) {
	profile := &models.CandidateProfile{
		FirstName:            "Nakul",
		LastName:             "Bhandare",
		Email:                "bhandarenakul11@gmail.com",
		Phone:                "+1 (350)-355-6570",
		City:                 "San Francisco",
		State:                "CA",
		Country:              "United States",
		AuthorizedToWorkInUS: true,
		RequiresSponsorship:  false,
		YearsOfExperience:    7,
		CurrentEmployer:      "Kyndryl",
		CurrentJobTitle:      "Senior Backend Engineer",
		DesiredSalaryMin:     150000,
		OpenToRemote:         true,
		HighestDegree:        "Master's Degree",
		LinkedInURL:          "https://linkedin.com/in/nakul-bhandare",
		GitHubURL:            "https://github.com/nakulbhandare",
	}

	engine := NewAnswerService()

	// ------------------------------------------------------------------------
	// PROBE 1: ADVERSARIAL OPTION MATCHING & DROPDOWN COLLISION RESISTANCE
	// ------------------------------------------------------------------------
	t.Run("OptionMatching_DemographicAndDropdownEdgeCases", func(t *testing.T) {
		// 1. Notice Period Dropdowns
		qNotice1 := ATSQuestion{
			Label:   "What is your notice period / availability?",
			Options: []string{"3 months", "1 month", "2 weeks", "Immediately"},
		}
		ansNotice1 := engine.AnswerQuestion(qNotice1, profile)
		if ansNotice1 != "Immediately" && ansNotice1 != "2 weeks" {
			t.Errorf("Notice period expected Immediately or 2 weeks, got %q", ansNotice1)
		}

		qNotice2 := ATSQuestion{
			Label:   "How soon can you join?",
			Options: []string{"More than 1 month", "Two weeks notice", "Immediate start"},
		}
		ansNotice2 := engine.AnswerQuestion(qNotice2, profile)
		if ansNotice2 != "Immediate start" && ansNotice2 != "Two weeks notice" {
			t.Errorf("Notice period expected Immediate start or Two weeks notice, got %q", ansNotice2)
		}

		// 2. Binary Gender collision (Female before Male)
		qGenderBinary := ATSQuestion{
			Label:   "Gender",
			Options: []string{"Female", "Male"},
		}
		ansGenderBinary := engine.AnswerQuestion(qGenderBinary, profile)
		if ansGenderBinary != "Male" {
			t.Errorf("Binary gender (Female first) matched %q, want 'Male'", ansGenderBinary)
		}

		// 3. Gender with Woman / Man:
		// 3a. When opt-out is present, verify opt-out priority over gender keywords (Feature 1)
		qGenderOptOut := ATSQuestion{
			Label:   "Gender",
			Options: []string{"Woman", "Man", "Non-binary", "Prefer not to disclose"},
		}
		ansGenderOptOut := engine.AnswerQuestion(qGenderOptOut, profile)
		if ansGenderOptOut != "Prefer not to disclose" {
			t.Errorf("Gender with opt-out present matched %q, want 'Prefer not to disclose'", ansGenderOptOut)
		}

		// 3b. When CustomFieldAnswers specifies "Man", verify exact word-boundary override (Woman before Man collision resistance)
		profileWithCustomMan := &models.CandidateProfile{
			FirstName:          "Nakul",
			LastName:           "Bhandare",
			CustomFieldAnswers: map[string]string{"gender": "Man"},
		}
		qGenderForced := ATSQuestion{
			Label:   "Gender",
			Options: []string{"Woman", "Man", "Non-binary"},
		}
		ansGenderForced := engine.AnswerQuestion(qGenderForced, profileWithCustomMan)
		if ansGenderForced != "Man" {
			t.Errorf("Gender with CustomFieldAnswers='Man' (Woman first) matched %q, want 'Man'", ansGenderForced)
		}

		// 4. Multi-category Race (Caucasian before Asian)
		qRace := ATSQuestion{
			Label:   "Race / Ethnicity",
			Options: []string{"Caucasian / White", "Asian", "Black / African American"},
		}
		ansRace := engine.AnswerQuestion(qRace, profile)
		if ansRace != "Asian" {
			t.Errorf("Race Caucasian before Asian matched %q, want 'Asian'", ansRace)
		}

		// 5. Disability multi-sentence options
		qDisability := ATSQuestion{
			Label: "Disability Self-ID",
			Options: []string{
				"Yes, I have a disability (or previously had a disability)",
				"No, I do not have a disability (or previously had a disability)",
				"I do not wish to self-identify",
			},
		}
		ansDisability := engine.AnswerQuestion(qDisability, profile)
		if ansDisability != "No, I do not have a disability (or previously had a disability)" {
			t.Errorf("Disability matched %q, want 'No, I do not have a disability...'", ansDisability)
		}

		// 6. Veteran status options
		qVeteran := ATSQuestion{
			Label: "Protected Veteran Status",
			Options: []string{
				"I identify as one or more of the classifications of protected veteran",
				"I am not a protected veteran",
				"I choose not to self-identify",
			},
		}
		ansVeteran := engine.AnswerQuestion(qVeteran, profile)
		if ansVeteran != "I am not a protected veteran" {
			t.Errorf("Veteran matched %q, want 'I am not a protected veteran'", ansVeteran)
		}

		// 7. Single option fallback
		qSingle := ATSQuestion{
			Label:   "Please confirm acknowledging our Privacy Policy",
			Options: []string{"I confirm and agree"},
		}
		ansSingle := engine.AnswerQuestion(qSingle, profile)
		if ansSingle != "I confirm and agree" {
			t.Errorf("Single option fallback got %q, want 'I confirm and agree'", ansSingle)
		}
	})

	// ------------------------------------------------------------------------
	// PROBE 2: ADVERSARIAL CACHE ISOLATION ACROSS COMPANIES AND CANDIDATES
	// ------------------------------------------------------------------------
	t.Run("CacheIsolation_CrossCompanyAndCrossCandidate", func(t *testing.T) {
		companies := []string{
			"Stripe", "Netflix", "Google", "Meta", "Apple",
			"Amazon", "Microsoft", "Uber", "Airbnb", "Databricks",
		}

		qWhy := ATSQuestion{Label: "Why are you interested in joining our company?"}

		// Test interleaved queries across all 10 companies multiple times
		for round := 0; round < 5; round++ {
			for _, company := range companies {
				ans := engine.AnswerQuestionWithJob(qWhy, profile, JobAnswerContext{
					JobTitle:    "Senior Distributed Systems Engineer",
					CompanyName: company,
				})

				// Must contain current company
				if !strings.Contains(ans, company) {
					t.Fatalf("round %d company %s: answer did not contain company name: %q", round, company, ans)
				}

				// Must NOT contain any of the other 9 companies
				for _, other := range companies {
					if other != company && strings.Contains(ans, other) {
						t.Fatalf("CACHE LEAK DETECTED! Company %s answer contains %s in round %d: %q",
							company, other, round, ans)
					}
				}
			}
		}

		// Test candidate isolation for employer achievements
		candA := &models.CandidateProfile{
			UserID:            "user-aaa",
			FirstName:         "Alice",
			LastName:          "Smith",
			CurrentEmployer:   "Kyndryl",
			CurrentJobTitle:   "Cloud Architect",
			YearsOfExperience: 8,
		}
		candB := &models.CandidateProfile{
			UserID:            "user-bbb",
			FirstName:         "Bob",
			LastName:          "Jones",
			CurrentEmployer:   "Palantir",
			CurrentJobTitle:   "Forward Deployed Engineer",
			YearsOfExperience: 6,
		}
		candC := &models.CandidateProfile{
			UserID:            "user-ccc",
			FirstName:         "Charlie",
			LastName:          "Brown",
			CurrentEmployer:   "Datadog",
			CurrentJobTitle:   "Core Infrastructure Engineer",
			YearsOfExperience: 9,
		}

		qProject := ATSQuestion{Label: "Describe a challenging project you delivered at your employer"}
		qEmp := ATSQuestion{Label: "Who is your current employer?"}

		// Interleaved queries
		for iter := 0; iter < 3; iter++ {
			ansA := engine.AnswerQuestion(qProject, candA)
			ansB := engine.AnswerQuestion(qProject, candB)
			ansC := engine.AnswerQuestion(qProject, candC)

			if !strings.Contains(ansA, "Kyndryl") || strings.Contains(ansA, "Palantir") || strings.Contains(ansA, "Datadog") {
				t.Fatalf("Candidate A project achievement cache leak: %q", ansA)
			}
			if !strings.Contains(ansB, "Palantir") || strings.Contains(ansB, "Kyndryl") || strings.Contains(ansB, "Datadog") {
				t.Fatalf("Candidate B project achievement cache leak: %q", ansB)
			}
			if !strings.Contains(ansC, "Datadog") || strings.Contains(ansC, "Kyndryl") || strings.Contains(ansC, "Palantir") {
				t.Fatalf("Candidate C project achievement cache leak: %q", ansC)
			}

			empA := engine.AnswerQuestion(qEmp, candA)
			empB := engine.AnswerQuestion(qEmp, candB)
			empC := engine.AnswerQuestion(qEmp, candC)

			if empA != "Kyndryl" {
				t.Fatalf("Candidate A employer got %q, want 'Kyndryl'", empA)
			}
			if empB != "Palantir" {
				t.Fatalf("Candidate B employer got %q, want 'Palantir'", empB)
			}
			if empC != "Datadog" {
				t.Fatalf("Candidate C employer got %q, want 'Datadog'", empC)
			}
		}
	})

	// ------------------------------------------------------------------------
	// PROBE 3: CUSTOM FIELD OVERRIDES AND RATING VS SALARY SEPARATION
	// ------------------------------------------------------------------------
	t.Run("CustomFieldOverrides_And_SkillRatingIsolation", func(t *testing.T) {
		// 1. Skill rating must NOT collide with rxSalary
		ratingPrompts := []string{
			"Rate your proficiency in Go from 1 to 5",
			"Please rate your experience with Kubernetes on a scale of 1-10",
			"Rate your familiarity with microservice design patterns",
			"What rating would you give your SQL optimization skills?",
		}
		for _, prompt := range ratingPrompts {
			ans := engine.AnswerQuestion(ATSQuestion{Label: prompt}, profile)
			if strings.Contains(ans, "150,000") || strings.Contains(ans, "$150,000") || strings.Contains(ans, "$90") {
				t.Fatalf("Skill rating prompt %q falsely classified as salary/compensation! Got: %q", prompt, ans)
			}
		}

		// 2. FormatCandidatePayload custom overrides retention
		customAnswers := map[string]string{
			"salary":                 "$250,000",
			"salary_numeric":         "250000",
			"work_authorization":     "No",
			"authorized_to_work":     "No",
			"sponsorship":            "Yes",
			"custom_security_clear":  "Top Secret / SCI",
			"custom_referral_source": "Tech Conference 2026",
		}

		candMap := FormatCandidatePayload(profile, "My tailored cover letter", customAnswers)
		ansMap, ok := candMap["answers"].(map[string]string)
		if !ok {
			t.Fatalf("FormatCandidatePayload did not produce map[string]string for answers")
		}

		// Verify custom values were NOT clobbered by default values
		if ansMap["salary"] != "$250,000" {
			t.Errorf("Custom salary override was clobbered! Got %q, want '$250,000'", ansMap["salary"])
		}
		if ansMap["salary_numeric"] != "250000" {
			t.Errorf("Custom salary_numeric override was clobbered! Got %q, want '250000'", ansMap["salary_numeric"])
		}
		if ansMap["work_authorization"] != "No" {
			t.Errorf("Custom work_authorization override was clobbered! Got %q, want 'No'", ansMap["work_authorization"])
		}
		if ansMap["sponsorship"] != "Yes" {
			t.Errorf("Custom sponsorship override was clobbered! Got %q, want 'Yes'", ansMap["sponsorship"])
		}
		if ansMap["custom_security_clear"] != "Top Secret / SCI" {
			t.Errorf("Custom question field was lost! Got %q, want 'Top Secret / SCI'", ansMap["custom_security_clear"])
		}
		if ansMap["custom_referral_source"] != "Tech Conference 2026" {
			t.Errorf("Custom question field was lost! Got %q, want 'Tech Conference 2026'", ansMap["custom_referral_source"])
		}

		// 3. FormatCandidatePayload with nil profile
		nilMap := FormatCandidatePayload(nil, "cover", nil)
		if nilMap["name"] != "Nakul Bhandare" {
			t.Errorf("Nil profile should fallback to default name, got %v", nilMap["name"])
		}
		nilAnswers, ok := nilMap["answers"].(map[string]string)
		if !ok || nilAnswers["salary_numeric"] != "185000" {
			t.Errorf("Nil profile should populate default salary_numeric, got %v", nilAnswers["salary_numeric"])
		}
	})
}

