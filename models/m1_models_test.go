package models

import (
	"testing"
	"time"
)

func TestATSCompanyConfig_Validation(t *testing.T) {
	cfg := ATSCompanyConfig{
		Provider:          "greenhouse",
		CompanyID:         "stripe",
		CompanyName:       "Stripe",
		CanonicalEmbedURL: "https://boards.greenhouse.io/stripe",
		APIEndpoint:       "https://boards-api.greenhouse.io/v1/boards/stripe",
		Tier:              1,
		IsActive:          true,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config, got: %v", err)
	}

	expectedID := "greenhouse-stripe"
	if cfg.ID != expectedID {
		t.Errorf("expected ID %s, got %s", expectedID, cfg.ID)
	}

	invalidCfg := ATSCompanyConfig{
		Provider:  "lever",
		CompanyID: "palantir",
		Tier:      99, // Invalid tier
	}
	if err := invalidCfg.Validate(); err == nil {
		t.Error("expected error for invalid tier, got nil")
	}
}

func TestJobFormSchema_ValidationAndCompiledASTLookup(t *testing.T) {
	schema := JobFormSchema{
		Provider:  "ashby",
		CompanyID: "linear",
		JobID:     "job-123",
		SubmissionTarget: SubmissionTarget{
			Tier:     1,
			Endpoint: "https://jobs.ashbyhq.com/api/non-user-graphql",
			Method:   "POST",
			Encoding: "application/json",
		},
		Fields: []NormalizedFormField{
			{
				FieldID:    "_systemfield_name",
				Label:      "Name",
				Type:       "text",
				Required:   true,
				MappingKey: "candidate.first_name",
			},
		},
		CompiledAST: map[string]any{
			"flat_fields": map[string]any{
				"_systemfield_name": map[string]any{
					"label":       "Name",
					"mapping_key": "candidate.first_name",
					"required":    true,
				},
			},
		},
		Version:    1,
		ResolvedAt: time.Now().UTC(),
		ExpiresAt:  time.Now().UTC().Add(48 * time.Hour),
	}

	if err := schema.Validate(); err != nil {
		t.Fatalf("expected valid schema, got: %v", err)
	}

	expectedID := "schema-ashby-linear-job-123"
	if schema.ID != expectedID {
		t.Errorf("expected ID %s, got %s", expectedID, schema.ID)
	}

	field, exists := schema.LookupCompiledField("_systemfield_name")
	if !exists {
		t.Fatalf("expected field to exist in CompiledAST")
	}
	fieldMap, ok := field.(map[string]any)
	if !ok || fieldMap["mapping_key"] != "candidate.first_name" {
		t.Errorf("unexpected field map: %v", field)
	}

	_, exists = schema.LookupCompiledField("non_existent")
	if exists {
		t.Errorf("expected non_existent field to not exist")
	}
}

func TestFormQuestionEmbedding_Matching(t *testing.T) {
	emb := FormQuestionEmbedding{
		CanonicalLabel: "work_authorization_us",
		Category:       "work_authorization",
		QuestionPatterns: []string{
			"are you legally authorized to work in the united states",
			"are you authorized to work in the us",
		},
		CandidateAttributePath: "authorized_to_work_in_us",
		AnswerType:             "boolean",
		DefaultValue:           true,
		CandidateMappedValue:   true,
		OptionMatchStrategy:    "boolean_synonym",
	}

	if err := emb.Validate(); err != nil {
		t.Fatalf("expected valid embedding, got: %v", err)
	}

	if emb.QuestionHash == "" || emb.ID == "" {
		t.Errorf("expected generated QuestionHash and ID, got hash=%s, id=%s", emb.QuestionHash, emb.ID)
	}

	if !emb.Matches("Are you legally authorized to work in the United States?") {
		t.Errorf("expected pattern match for legal authorization")
	}

	if !emb.Matches("Are you authorized to work in the US?") {
		t.Errorf("expected pattern match for US work auth")
	}

	if emb.Matches("What is your expected salary?") {
		t.Errorf("expected non-match for salary question")
	}
}
