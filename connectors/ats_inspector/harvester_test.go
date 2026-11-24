package ats_inspector

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/oneRole/models"
)

type mockTransport struct {
	handler func(req *http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.handler(req)
}

func newMockClient(handler func(req *http.Request) (*http.Response, error)) *http.Client {
	return &http.Client{
		Transport: &mockTransport{handler: handler},
		Timeout:   5 * time.Second,
	}
}

func TestGreenhouseHarvester_MockHarvest(t *testing.T) {
	mockJSON := `{
		"id": 8172510,
		"title": "Software Engineer",
		"questions": [
			{
				"label": "First Name",
				"name": null,
				"required": true,
				"fields": [{"name": "first_name", "type": "input_text"}]
			},
			{
				"label": "Email",
				"name": null,
				"required": true,
				"fields": [{"name": "email", "type": "input_text"}]
			},
			{
				"label": "Resume/CV",
				"name": null,
				"required": true,
				"fields": [
					{"name": "resume", "type": "input_file"},
					{"name": "resume_text", "type": "textarea"}
				]
			},
			{
				"label": "Are you authorized to work in the United States?",
				"name": "question_12497125004",
				"required": true,
				"fields": [
					{
						"name": "question_12497125004",
						"type": "multi_value_single_select",
						"values": [
							{"label": "Yes", "value": 1},
							{"label": "No", "value": 0}
						]
					}
				]
			}
		],
		"compliance": [
			{
				"type": "eeoc",
				"questions": [
					{
						"label": "Gender",
						"required": false,
						"fields": [
							{
								"name": "gender",
								"type": "multi_value_single_select",
								"values": [
									{"label": "Decline to Self-Identify", "value": "decline"}
								]
							}
						]
					}
				]
			}
		]
	}`

	client := newMockClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(mockJSON)),
		}, nil
	})

	normalizer := NewASTSchemaNormalizer(nil)
	harvester := NewGreenhouseHarvester(client, normalizer)

	jobURL := "https://boards.greenhouse.io/stripe/jobs/8172510"
	if !harvester.CanHarvest(jobURL) {
		t.Fatalf("expected CanHarvest to return true for %s", jobURL)
	}

	schema, err := harvester.Harvest(context.Background(), jobURL)
	if err != nil {
		t.Fatalf("harvest failed: %v", err)
	}

	expectedID := "schema-greenhouse-stripe-8172510"
	if schema.ID != expectedID {
		t.Errorf("expected ID %s, got %s", expectedID, schema.ID)
	}
	if schema.SubmissionTarget.Tier != 1 {
		t.Errorf("expected Tier 1, got %d", schema.SubmissionTarget.Tier)
	}

	// Verify pre-compiled AST
	if schema.CompiledAST == nil {
		t.Fatalf("expected CompiledAST to be populated")
	}

	field, exists := schema.LookupCompiledField("first_name")
	if !exists {
		t.Errorf("expected first_name in CompiledAST")
	}
	fMap := field.(map[string]any)
	if fMap["mapping_key"] != "candidate.first_name" {
		t.Errorf("expected mapping_key candidate.first_name, got %v", fMap["mapping_key"])
	}

	// Verify option values in CompiledAST
	optVals, ok := schema.CompiledAST["option_values"].(map[string]map[string]any)
	if !ok || optVals["question_12497125004"] == nil {
		t.Errorf("expected option_values for question_12497125004 in CompiledAST")
	}
}

func TestAshbyHarvester_MockHarvest(t *testing.T) {
	mockGraphQL := `{
		"data": {
			"jobPosting": {
				"id": "d3bc1ced-3ce4-4086-a050-555055dbb1ff",
				"title": "Senior Engineer",
				"applicationForm": {
					"id": "form-1",
					"sections": [
						{
							"title": "Contact",
							"fieldEntries": [
								{
									"id": "entry-name",
									"isRequired": true,
									"field": {
										"id": "f-1",
										"path": "_systemfield_name",
										"title": "Name",
										"type": "String"
									}
								},
								{
									"id": "entry-email",
									"isRequired": true,
									"field": {
										"id": "f-2",
										"path": "_systemfield_email",
										"title": "Email",
										"type": "Email"
									}
								},
								{
									"id": "entry-work-auth",
									"isRequired": true,
									"field": {
										"id": "f-3",
										"path": "custom_work_auth",
										"title": "Are you authorized to work in the US?",
										"type": "Boolean"
									}
								}
							]
						}
					]
				}
			}
		}
	}`

	client := newMockClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(mockGraphQL)),
		}, nil
	})

	normalizer := NewASTSchemaNormalizer(nil)
	harvester := NewAshbyHarvester(client, normalizer)

	jobURL := "https://jobs.ashbyhq.com/linear/d3bc1ced-3ce4-4086-a050-555055dbb1ff"
	schema, err := harvester.Harvest(context.Background(), jobURL)
	if err != nil {
		t.Fatalf("harvest failed: %v", err)
	}

	expectedID := "schema-ashby-linear-d3bc1ced-3ce4-4086-a050-555055dbb1ff"
	if schema.ID != expectedID {
		t.Errorf("expected ID %s, got %s", expectedID, schema.ID)
	}
	if schema.SubmissionTarget.Tier != 1 {
		t.Errorf("expected Tier 1, got %d", schema.SubmissionTarget.Tier)
	}
	if schema.SubmissionTarget.Encoding != "application/json" {
		t.Errorf("expected application/json encoding, got %s", schema.SubmissionTarget.Encoding)
	}
}

func TestLeverHarvester_MockHarvest(t *testing.T) {
	mockApplyHTML := `
	<html>
	<body>
		<form>
			<input name="name" required />
			<input name="email" required />
			<input name="cards[card-1][field0]" type="checkbox" required />
			<input name="cards[card-2][field1]" type="text" />
		</form>
	</body>
	</html>`

	client := newMockClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(mockApplyHTML)),
		}, nil
	})

	normalizer := NewASTSchemaNormalizer(nil)
	harvester := NewLeverHarvester(client, normalizer)

	jobURL := "https://jobs.lever.co/palantir/ac978161-6f46-4f6b-ad9e-a258e642751c"
	schema, err := harvester.Harvest(context.Background(), jobURL)
	if err != nil {
		t.Fatalf("harvest failed: %v", err)
	}

	expectedID := "schema-lever-palantir-ac978161-6f46-4f6b-ad9e-a258e642751c"
	if schema.ID != expectedID {
		t.Errorf("expected ID %s, got %s", expectedID, schema.ID)
	}
	if schema.SubmissionTarget.Tier != 1 {
		t.Errorf("expected Tier 1, got %d", schema.SubmissionTarget.Tier)
	}

	// Verify card input was detected
	_, exists := schema.LookupCompiledField("cards[card-1][field0]")
	if !exists {
		t.Errorf("expected cards[card-1][field0] in CompiledAST")
	}
}

func TestSmartRecruitersHarvester_MockHarvest(t *testing.T) {
	mockPostingJSON := `{
		"id": "744000162024500",
		"name": "Senior Software Engineer",
		"customField": [
			{
				"fieldId": "COUNTRY",
				"fieldLabel": "Country",
				"valueLabel": "United States"
			}
		]
	}`

	client := newMockClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(mockPostingJSON)),
		}, nil
	})

	normalizer := NewASTSchemaNormalizer(nil)
	harvester := NewSmartRecruitersHarvester(client, normalizer)

	jobURL := "https://jobs.smartrecruiters.com/WesternDigital/744000162024500"
	schema, err := harvester.Harvest(context.Background(), jobURL)
	if err != nil {
		t.Fatalf("harvest failed: %v", err)
	}

	expectedID := "schema-smartrecruiters-westerndigital-744000162024500"
	if schema.ID != expectedID {
		t.Errorf("expected ID %s, got %s", expectedID, schema.ID)
	}
	if schema.SubmissionTarget.Tier != 2 {
		t.Errorf("expected Tier 2, got %d", schema.SubmissionTarget.Tier)
	}
	if schema.SubmissionTarget.Encoding != "window.__oneRoleApply" {
		t.Errorf("expected window.__oneRoleApply, got %s", schema.SubmissionTarget.Encoding)
	}

	// Verify selector for first name
	selectors, ok := schema.CompiledAST["selectors"].(map[string]string)
	if !ok || selectors["firstName"] == "" {
		t.Errorf("expected DOM selector for firstName in CompiledAST")
	}
}

func TestWorkdayHarvester_MockHarvest(t *testing.T) {
	mockCXS := `{
		"jobPostingInfo": {
			"id": "7a186dba",
			"jobReqId": "R-0411385",
			"questionnaireId": "q-123",
			"canQuickApply": true
		}
	}`

	client := newMockClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(mockCXS)),
		}, nil
	})

	normalizer := NewASTSchemaNormalizer(nil)
	harvester := NewWorkdayHarvester(client, normalizer)

	jobURL := "https://adventhealth.wd12.myworkdayjobs.com/AH_External_Career_Site/job/Orlando-FL/Registered-Nurse_R-0411385"
	schema, err := harvester.Harvest(context.Background(), jobURL)
	if err != nil {
		t.Fatalf("harvest failed: %v", err)
	}

	expectedID := "schema-workday-adventhealth-R-0411385"
	if schema.ID != expectedID {
		t.Errorf("expected ID %s, got %s", expectedID, schema.ID)
	}
	if schema.SubmissionTarget.Tier != 2 {
		t.Errorf("expected Tier 2, got %d", schema.SubmissionTarget.Tier)
	}

	// Verify standard 5-section data-automation-id selectors
	selectors, ok := schema.CompiledAST["selectors"].(map[string]string)
	if !ok || selectors["legalNameSection_firstName"] == "" {
		t.Errorf("expected data-automation-id selector for legalNameSection_firstName")
	}
}

func TestHarvesterRegistry_Dispatch(t *testing.T) {
	registry := NewDefaultHarvesterRegistry(nil)

	testCases := []struct {
		url      string
		provider string
	}{
		{"https://boards.greenhouse.io/figma/jobs/5426468004", "greenhouse"},
		{"https://jobs.ashbyhq.com/linear/d3bc1ced-3ce4-4086-a050-555055dbb1ff", "ashby"},
		{"https://jobs.lever.co/palantir/ac978161-6f46-4f6b-ad9e-a258e642751c", "lever"},
		{"https://jobs.smartrecruiters.com/WesternDigital/744000162024500", "smartrecruiters"},
		{"https://adventhealth.wd12.myworkdayjobs.com/AH_External_Career_Site/job/Orlando/RN_R-0411385", "workday"},
	}

	for _, tc := range testCases {
		h, err := registry.GetHarvester(tc.url)
		if err != nil {
			t.Errorf("expected harvester for %s, got error: %v", tc.url, err)
			continue
		}
		if h.Provider() != tc.provider {
			t.Errorf("expected provider %s, got %s", tc.provider, h.Provider())
		}
	}
}

func TestASTSchemaNormalizer_PrecompileAST_Performance(t *testing.T) {
	normalizer := NewASTSchemaNormalizer(nil)

	// Build a schema with 50 fields (larger than realistic ATS job form)
	var rawFields []RawATSField
	for i := 0; i < 50; i++ {
		rawFields = append(rawFields, RawATSField{
			ID:       strings.Repeat("field_", 1) + string(rune('a'+(i%26))),
			Label:    "Sample Question Label Number " + string(rune('0'+(i%10))),
			Type:     "text",
			Required: i%2 == 0,
		})
	}

	target := models.SubmissionTarget{Tier: 1, Endpoint: "https://api.ats.io/apply", Method: "POST"}
	schema := normalizer.Normalize("greenhouse", "testco", "job123", "https://ats.io/job123", target, rawFields, nil)

	start := time.Now()
	compiled := normalizer.PrecompileAST(schema, rawFields)
	duration := time.Since(start)

	if duration >= 2*time.Millisecond {
		t.Fatalf("AST pre-compilation took %v, exceeding 2ms budget!", duration)
	}

	if compiled == nil || schema.CompiledAST == nil {
		t.Fatal("expected non-nil CompiledAST")
	}
}

func TestASTSchemaNormalizer_InferMappingKeyPrecedence(t *testing.T) {
	normalizer := NewASTSchemaNormalizer(nil)

	testCases := []struct {
		name        string
		fieldID     string
		label       string
		expectedKey string
	}{
		{
			name:        "Stripe Job 8172510 Work Auth with Location Qualifier",
			fieldID:     "question_68935511",
			label:       "Are you authorized to work in the location(s) selected?",
			expectedKey: "candidate.authorized_to_work_in_us",
		},
		{
			name:        "Visa Sponsorship with Location Qualifier",
			fieldID:     "question_sponsorship_loc",
			label:       "Will you require visa sponsorship to work at this location?",
			expectedKey: "candidate.requires_sponsorship",
		},
		{
			name:        "Stripe Job 8172510 Work Permit Sponsorship",
			fieldID:     "question_68935512",
			label:       "Will you require Stripe to sponsor you for a work permit?",
			expectedKey: "candidate.requires_sponsorship",
		},
		{
			name:        "Salary Question with Location Qualifier",
			fieldID:     "salary_expectation",
			label:       "What is your expected salary for this office location?",
			expectedKey: "candidate.desired_salary_min",
		},
		{
			name:        "Experience Question with Location Qualifier",
			fieldID:     "exp_years",
			label:       "Years of experience at current location",
			expectedKey: "candidate.years_of_experience",
		},
		{
			name:        "Notice Period Question with Location Qualifier",
			fieldID:     "notice_weeks",
			label:       "Notice period / availability for this location",
			expectedKey: "candidate.notice_period_weeks",
		},
		{
			name:        "Standard Location Field",
			fieldID:     "candidate_city",
			label:       "Current Location / City",
			expectedKey: "candidate.city",
		},
		{
			name:        "Standard Address Field",
			fieldID:     "street_address",
			label:       "Address",
			expectedKey: "candidate.city",
		},
		{
			name:        "Where Are You Currently Located",
			fieldID:     "q4",
			label:       "Where are you currently located?",
			expectedKey: "candidate.city",
		},
		{
			name:        "Where Are You Located",
			fieldID:     "q_loc_1",
			label:       "Where are you located?",
			expectedKey: "candidate.city",
		},
		{
			name:        "Need Work Visa With Location",
			fieldID:     "q_visa_need",
			label:       "Do you need a work visa for this location?",
			expectedKey: "candidate.requires_sponsorship",
		},
		{
			name:        "Work Authorization Noun With Location",
			fieldID:     "q_work_auth_noun",
			label:       "What is your work authorization for this location?",
			expectedKey: "candidate.authorized_to_work_in_us",
		},
		{
			name:        "Legally Permitted To Work With Location",
			fieldID:     "q_work_perm",
			label:       "Are you legally permitted to work in this location?",
			expectedKey: "candidate.authorized_to_work_in_us",
		},
		{
			name:        "EEO Disability Accommodation With Location",
			fieldID:     "q_disab_loc",
			label:       "If you have a disability, what accommodations do you need at this location?",
			expectedKey: "candidate.custom_field_answers.eeo_disability",
		},
		{
			name:        "EEO Gender With Location",
			fieldID:     "q_gender_loc",
			label:       "What is your gender at this office location?",
			expectedKey: "candidate.custom_field_answers.eeo_gender",
		},
		{
			name:        "EEO Race With Location",
			fieldID:     "q_race_loc",
			label:       "Race / Ethnicity for this location",
			expectedKey: "candidate.custom_field_answers.eeo_race",
		},
		{
			name:        "EEO Veteran With Location",
			fieldID:     "q_vet_loc",
			label:       "Veteran status for this location",
			expectedKey: "candidate.custom_field_answers.eeo_veteran",
		},
		{
			name:        "Relocation Question Must Not Map To City",
			fieldID:     "q_relocate",
			label:       "Are you willing to relocate to this office location?",
			expectedKey: "",
		},
		{
			name:        "Exact ID Name With Empty Label",
			fieldID:     "name",
			label:       "",
			expectedKey: "candidate.full_name",
		},
		{
			name:        "Exact Label Name",
			fieldID:     "entry_name",
			label:       "Name",
			expectedKey: "candidate.full_name",
		},
		{
			name:        "First Name Precedence Over Exact Label Name",
			fieldID:     "first_name",
			label:       "Name",
			expectedKey: "candidate.first_name",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualKey := normalizer.inferMappingKey(tc.fieldID, tc.label)
			if actualKey != tc.expectedKey {
				t.Errorf("inferMappingKey(%q, %q) = %q, expected %q", tc.fieldID, tc.label, actualKey, tc.expectedKey)
			}
		})
	}
}

func TestWorkdayHarvester_ParseWorkdayURL_Locale(t *testing.T) {
	normalizer := NewASTSchemaNormalizer(nil)
	harvester := NewWorkdayHarvester(nil, normalizer)

	testCases := []struct {
		name                 string
		rawURL               string
		expectedTenant       string
		expectedSite         string
		expectedJobID        string
		expectedExternalPath string
	}{
		{
			name:                 "Workday URL with en-US locale prefix",
			rawURL:               "https://adventhealth.wd12.myworkdayjobs.com/en-US/AH_External_Career_Site/job/Orlando-FL/Registered-Nurse_R-0411385",
			expectedTenant:       "adventhealth",
			expectedSite:         "AH_External_Career_Site",
			expectedJobID:        "R-0411385",
			expectedExternalPath: "/job/Orlando-FL/Registered-Nurse_R-0411385",
		},
		{
			name:                 "Workday URL without locale prefix",
			rawURL:               "https://adventhealth.wd12.myworkdayjobs.com/AH_External_Career_Site/job/Orlando-FL/Registered-Nurse_R-0411385",
			expectedTenant:       "adventhealth",
			expectedSite:         "AH_External_Career_Site",
			expectedJobID:        "R-0411385",
			expectedExternalPath: "/job/Orlando-FL/Registered-Nurse_R-0411385",
		},
		{
			name:                 "Workday URL with en-GB locale prefix",
			rawURL:               "https://target.myworkdayjobs.com/en-GB/target_careers/job/London/Sales-Associate_12345",
			expectedTenant:       "target",
			expectedSite:         "target_careers",
			expectedJobID:        "12345",
			expectedExternalPath: "/job/London/Sales-Associate_12345",
		},
		{
			name:                 "Workday URL with 2-letter locale prefix",
			rawURL:               "https://sony.myworkdayjobs.com/fr/Sony_Careers/job/Paris/Software-Dev_999",
			expectedTenant:       "sony",
			expectedSite:         "Sony_Careers",
			expectedJobID:        "999",
			expectedExternalPath: "/job/Paris/Software-Dev_999",
		},
		{
			name:                 "Workday URL with BCP-47 script and region zh-Hans-CN",
			rawURL:               "https://company.wd5.myworkdayjobs.com/zh-Hans-CN/Careers/job/Beijing/Eng_R123",
			expectedTenant:       "company",
			expectedSite:         "Careers",
			expectedJobID:        "R123",
			expectedExternalPath: "/job/Beijing/Eng_R123",
		},
		{
			name:                 "Workday URL with BCP-47 numeric region es-419",
			rawURL:               "https://company.wd5.myworkdayjobs.com/es-419/Careers/job/Mexico-City/Eng_R123",
			expectedTenant:       "company",
			expectedSite:         "Careers",
			expectedJobID:        "R123",
			expectedExternalPath: "/job/Mexico-City/Eng_R123",
		},
		{
			name:                 "Workday URL with underscore locale en_US",
			rawURL:               "https://company.wd5.myworkdayjobs.com/en_US/Careers/job/NYC/Eng_R123",
			expectedTenant:       "company",
			expectedSite:         "Careers",
			expectedJobID:        "R123",
			expectedExternalPath: "/job/NYC/Eng_R123",
		},
		{
			name:                 "Workday URL with trailing /apply",
			rawURL:               "https://acme.wd5.myworkdayjobs.com/en-US/Careers/job/NYC/Eng_R123/apply",
			expectedTenant:       "acme",
			expectedSite:         "Careers",
			expectedJobID:        "R123",
			expectedExternalPath: "/job/NYC/Eng_R123",
		},
		{
			name:                 "Workday URL with trailing /details",
			rawURL:               "https://acme.wd5.myworkdayjobs.com/en-US/Careers/job/NYC/Eng_R123/details",
			expectedTenant:       "acme",
			expectedSite:         "Careers",
			expectedJobID:        "R123",
			expectedExternalPath: "/job/NYC/Eng_R123",
		},
		{
			name:                 "Workday URL with /details/ job route",
			rawURL:               "https://acme.wd5.myworkdayjobs.com/en-US/Careers/details/Eng_R123",
			expectedTenant:       "acme",
			expectedSite:         "Careers",
			expectedJobID:        "R123",
			expectedExternalPath: "/details/Eng_R123",
		},
		{
			name:                 "Workday URL with double slash",
			rawURL:               "https://acme.wd5.myworkdayjobs.com/en-US//Careers/job/NYC/Eng_R123",
			expectedTenant:       "acme",
			expectedSite:         "Careers",
			expectedJobID:        "R123",
			expectedExternalPath: "/job/NYC/Eng_R123",
		},
		{
			name:                 "Workday URL with locale-only path",
			rawURL:               "https://acme.wd5.myworkdayjobs.com/en-US",
			expectedTenant:       "acme",
			expectedSite:         "",
			expectedJobID:        "",
			expectedExternalPath: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			u, err := url.Parse(tc.rawURL)
			if err != nil {
				t.Fatalf("failed to parse url: %v", err)
			}
			tenant, site, jobID, externalPath := harvester.parseWorkdayURL(u)
			if tenant != tc.expectedTenant {
				t.Errorf("tenant: got %q, expected %q", tenant, tc.expectedTenant)
			}
			if site != tc.expectedSite {
				t.Errorf("site: got %q, expected %q", site, tc.expectedSite)
			}
			if jobID != tc.expectedJobID {
				t.Errorf("jobID: got %q, expected %q", jobID, tc.expectedJobID)
			}
			if externalPath != tc.expectedExternalPath {
				t.Errorf("externalPath: got %q, expected %q", externalPath, tc.expectedExternalPath)
			}
		})
	}
}

func TestWorkdayHarvester_InvalidURLReturnsError(t *testing.T) {
	client := newMockClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{}`)),
		}, nil
	})
	harvester := NewWorkdayHarvester(client, NewASTSchemaNormalizer(nil))

	invalidURLs := []string{
		"",
		"not-a-valid-url",
		"https://example.com/not-workday",
		"https://acme.wd5.myworkdayjobs.com/",
		"https://acme.wd5.myworkdayjobs.com/en-US",
		"https://acme.wd5.myworkdayjobs.com/en-US/Careers",
		"https://acme.wd5.myworkdayjobs.com/en-US/Careers/apply",
		"https://acme.wd5.myworkdayjobs.com/en-US/Careers/job",
	}

	for _, rawURL := range invalidURLs {
		t.Run(rawURL, func(t *testing.T) {
			schema, err := harvester.Harvest(context.Background(), rawURL)
			if err == nil {
				t.Fatalf("expected error for invalid Workday URL %q, got nil (schema=%+v)", rawURL, schema)
			}
			if schema != nil {
				t.Fatalf("expected nil schema on error for %q, got %+v", rawURL, schema)
			}
		})
	}
}

func BenchmarkPrecompileAST(b *testing.B) {
	normalizer := NewASTSchemaNormalizer(nil)

	var rawFields []RawATSField
	for i := 0; i < 25; i++ {
		rawFields = append(rawFields, RawATSField{
			ID:       "field_" + string(rune('a'+(i%26))),
			Label:    "Screening Question " + string(rune('0'+(i%10))),
			Type:     "select",
			Required: true,
			Options:  []string{"Yes", "No", "Decline"},
			OptionValues: map[string]any{
				"Yes":     1,
				"No":      0,
				"Decline": -1,
			},
		})
	}

	schema := &models.JobFormSchema{
		ID:        "schema-benchmark-job",
		Provider:  "greenhouse",
		CompanyID: "benchmark",
		JobID:     "bench-1",
		SubmissionTarget: models.SubmissionTarget{
			Tier:     1,
			Endpoint: "https://api.ats.com/apply",
			Method:   "POST",
			Encoding: "multipart/form-data",
		},
	}

	for _, rf := range rawFields {
		schema.Fields = append(schema.Fields, models.NormalizedFormField{
			FieldID:    rf.ID,
			Label:      rf.Label,
			Type:       rf.Type,
			Required:   rf.Required,
			Options:    rf.Options,
			MappingKey: "candidate.first_name",
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = normalizer.PrecompileAST(schema, rawFields)
	}
}
