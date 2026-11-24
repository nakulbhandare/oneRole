package models

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// FormQuestionEmbedding maps custom ATS screening questions to candidate profile attributes and default responses.
// Collection: "form_question_embeddings"
// Document Key: "emb-{hash}"
type FormQuestionEmbedding struct {
	ID                     string    `json:"id" bson:"_id"`
	QuestionHash           string    `json:"question_hash" bson:"question_hash"`
	CanonicalLabel         string    `json:"canonical_label" bson:"canonical_label"`
	Category               string    `json:"category" bson:"category"` // "work_authorization", "visa_sponsorship", "experience_skills", etc.
	QuestionPatterns       []string  `json:"question_patterns" bson:"question_patterns"`
	CandidateAttributePath string    `json:"candidate_attribute_path" bson:"candidate_attribute_path"` // e.g. "authorized_to_work_in_us"
	AnswerType             string    `json:"answer_type" bson:"answer_type"`                           // "boolean", "string", "number", "select_option", "textarea"
	DefaultValue           any       `json:"default_value" bson:"default_value"`
	CandidateMappedValue   any       `json:"candidate_mapped_value" bson:"candidate_mapped_value"` // Value for bhandarenakul11@gmail.com
	OptionMatchStrategy    string    `json:"option_match_strategy" bson:"option_match_strategy"`   // "exact", "substring_case_insensitive", "boolean_synonym"
	Vector                 []float32 `json:"vector,omitempty" bson:"vector,omitempty"`             // Semantic embedding vector (optional)
	ConfidenceThreshold    float64   `json:"confidence_threshold" bson:"confidence_threshold"`
	CreatedAt              time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt              time.Time `json:"updated_at" bson:"updated_at"`
}

// ComputeQuestionHash computes a deterministic SHA-256 hash prefix (16 chars) for a question or label.
func ComputeQuestionHash(text string) string {
	normalized := strings.ToLower(strings.TrimSpace(text))
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:8]) // 16 hex chars
}

// GenerateEmbeddingID returns the canonical document key: "emb-{hash}".
func GenerateEmbeddingID(hash string) string {
	return fmt.Sprintf("emb-%s", strings.TrimSpace(hash))
}

// Validate checks the integrity of the question embedding record.
func (e *FormQuestionEmbedding) Validate() error {
	if strings.TrimSpace(e.CanonicalLabel) == "" {
		return fmt.Errorf("canonical_label is required")
	}
	if strings.TrimSpace(e.QuestionHash) == "" {
		e.QuestionHash = ComputeQuestionHash(e.CanonicalLabel)
	}
	if e.ID == "" {
		e.ID = GenerateEmbeddingID(e.QuestionHash)
	}
	if strings.TrimSpace(e.Category) == "" {
		return fmt.Errorf("category is required")
	}
	return nil
}

// Matches tests whether any registered question pattern matches the given input text.
func (e *FormQuestionEmbedding) Matches(questionText string) bool {
	q := strings.ToLower(strings.TrimSpace(questionText))
	if q == "" {
		return false
	}
	if strings.EqualFold(q, e.CanonicalLabel) {
		return true
	}
	for _, pattern := range e.QuestionPatterns {
		p := strings.ToLower(strings.TrimSpace(pattern))
		if p != "" && strings.Contains(q, p) {
			return true
		}
	}
	return false
}
