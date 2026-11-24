package models

import (
	"fmt"
	"strings"
	"time"
)

// ATSCompanyConfig holds company-specific ATS routing rules, embed URLs, and default screening rules.
// Collection: "ats_company_configs"
// Document Key: "{provider}-{company}" (e.g. "greenhouse-stripe", "ashby-linear")
type ATSCompanyConfig struct {
	ID                string            `json:"id" bson:"_id"`
	Provider          string            `json:"provider" bson:"provider"` // "greenhouse" | "ashby" | "lever" | "smartrecruiters" | "workday"
	CompanyID         string            `json:"company_id" bson:"company_id"`
	CompanyName       string            `json:"company_name" bson:"company_name"`
	CanonicalEmbedURL string            `json:"canonical_embed_url" bson:"canonical_embed_url"`
	APIEndpoint       string            `json:"api_endpoint" bson:"api_endpoint"`
	Tier              int               `json:"tier" bson:"tier"` // 1 = Direct API/GraphQL, 2 = Chromium Pool
	ScreeningRules    map[string]string `json:"screening_rules" bson:"screening_rules"` // Question pattern -> default answer
	RateLimitPerSec   float64           `json:"rate_limit_per_sec" bson:"rate_limit_per_sec"`
	MaxConcurrency    int               `json:"max_concurrency" bson:"max_concurrency"`
	AuthType          string            `json:"auth_type,omitempty" bson:"auth_type,omitempty"` // "public", "api_key", "session_cookie"
	Headers           map[string]string `json:"headers,omitempty" bson:"headers,omitempty"`
	IsActive          bool              `json:"is_active" bson:"is_active"`
	CreatedAt         time.Time         `json:"created_at" bson:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at" bson:"updated_at"`
}

// GenerateATSCompanyConfigID produces the canonical document ID: "{provider}-{company}".
func GenerateATSCompanyConfigID(provider, companyID string) string {
	return fmt.Sprintf("%s-%s", strings.ToLower(strings.TrimSpace(provider)), strings.ToLower(strings.TrimSpace(companyID)))
}

// Validate ensures all required fields are populated and valid.
func (c *ATSCompanyConfig) Validate() error {
	if strings.TrimSpace(c.Provider) == "" {
		return fmt.Errorf("provider is required")
	}
	if strings.TrimSpace(c.CompanyID) == "" {
		return fmt.Errorf("company_id is required")
	}
	expectedID := GenerateATSCompanyConfigID(c.Provider, c.CompanyID)
	if c.ID == "" {
		c.ID = expectedID
	}
	if c.Tier != 1 && c.Tier != 2 {
		return fmt.Errorf("invalid tier %d: must be 1 (Headless API) or 2 (Chromium)", c.Tier)
	}
	return nil
}
