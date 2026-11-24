package models

import (
	"fmt"
	"strings"
	"time"
)

// SubmissionTarget encapsulates the dispatch protocol target for online execution.
type SubmissionTarget struct {
	Tier     int    `json:"tier" bson:"tier"`         // 1 = Direct API/GraphQL, 2 = Chromium Pool
	Endpoint string `json:"endpoint" bson:"endpoint"` // Target URL or mutation endpoint
	Method   string `json:"method" bson:"method"`     // POST, PUT, etc.
	Encoding string `json:"encoding" bson:"encoding"` // "multipart/form-data", "application/json", "graphql", "window.__oneRoleApply"
}

// NormalizedFormField describes a canonicalized field in a job application form.
type NormalizedFormField struct {
	FieldID      string   `json:"field_id" bson:"field_id"`
	Selector     string   `json:"selector,omitempty" bson:"selector,omitempty"` // CSS / data-automation selector for Tier 2
	Label        string   `json:"label" bson:"label"`
	Type         string   `json:"type" bson:"type"` // text, textarea, select, multiselect, radio, checkbox, file, number, date, boolean
	Required     bool     `json:"required" bson:"required"`
	MappingKey   string   `json:"mapping_key,omitempty" bson:"mapping_key,omitempty"`       // Candidate profile dot path (e.g. "candidate.first_name")
	EmbeddingID  string   `json:"embedding_id,omitempty" bson:"embedding_id,omitempty"`     // Reference to form_question_embeddings _id
	Options      []string `json:"options,omitempty" bson:"options,omitempty"`               // Available options for select/radio/checkbox
	DefaultValue string   `json:"default_value,omitempty" bson:"default_value,omitempty"`   // Fallback value
	Validation   string   `json:"validation,omitempty" bson:"validation,omitempty"`         // Regex or validation rule
}

// JobFormSchema stores pre-compiled, normalized form ASTs for sub-2ms online candidate resolution.
// Collection: "job_form_schemas"
// Document Key: "schema-{provider}-{company}-{jobId}"
type JobFormSchema struct {
	ID               string                `json:"id" bson:"_id"`
	Provider         string                `json:"provider" bson:"provider"` // "greenhouse" | "ashby" | "lever" | "smartrecruiters" | "workday"
	CompanyID        string                `json:"company_id" bson:"company_id"`
	JobID            string                `json:"job_id" bson:"job_id"`
	JobURL           string                `json:"job_url,omitempty" bson:"job_url,omitempty"`
	SubmissionTarget SubmissionTarget      `json:"submission_target" bson:"submission_target"`
	Fields           []NormalizedFormField `json:"fields" bson:"fields"`
	CompiledAST      map[string]any        `json:"compiled_ast" bson:"compiled_ast"` // Pre-compiled flat AST for <2ms lookup
	Version          int                   `json:"version" bson:"version"`
	ResolvedAt       time.Time             `json:"resolved_at" bson:"resolved_at"`
	ExpiresAt        time.Time             `json:"expires_at" bson:"expires_at"`
}

// GenerateJobFormSchemaID produces the canonical document ID: "schema-{provider}-{company}-{jobId}".
func GenerateJobFormSchemaID(provider, companyID, jobID string) string {
	return fmt.Sprintf("schema-%s-%s-%s",
		strings.ToLower(strings.TrimSpace(provider)),
		strings.ToLower(strings.TrimSpace(companyID)),
		strings.TrimSpace(jobID),
	)
}

// Validate ensures all required fields are present.
func (s *JobFormSchema) Validate() error {
	if strings.TrimSpace(s.Provider) == "" {
		return fmt.Errorf("provider is required")
	}
	if strings.TrimSpace(s.CompanyID) == "" {
		return fmt.Errorf("company_id is required")
	}
	if strings.TrimSpace(s.JobID) == "" {
		return fmt.Errorf("job_id is required")
	}
	expectedID := GenerateJobFormSchemaID(s.Provider, s.CompanyID, s.JobID)
	if s.ID == "" {
		s.ID = expectedID
	}
	if s.SubmissionTarget.Tier != 1 && s.SubmissionTarget.Tier != 2 {
		return fmt.Errorf("invalid submission tier %d: must be 1 or 2", s.SubmissionTarget.Tier)
	}
	return nil
}

// LookupCompiledField performs sub-microsecond O(1) attribute lookup from the pre-compiled AST flat map.
func (s *JobFormSchema) LookupCompiledField(fieldID string) (any, bool) {
	if s.CompiledAST == nil {
		return nil, false
	}
	flatFields, ok := s.CompiledAST["flat_fields"].(map[string]any)
	if !ok {
		return nil, false
	}
	val, exists := flatFields[fieldID]
	return val, exists
}
