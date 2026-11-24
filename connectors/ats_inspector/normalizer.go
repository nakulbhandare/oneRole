package ats_inspector

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/oneRole/models"
)

var (
	rxFirstName       = regexp.MustCompile(`(?i)\b(first[\s_-]?name|given[\s_-]?name)\b`)
	rxLastName        = regexp.MustCompile(`(?i)\b(last[\s_-]?name|family[\s_-]?name|surname)\b`)
	rxFullName        = regexp.MustCompile(`(?i)\b(full[\s_-]?name|^name$)\b`)
	rxEmail           = regexp.MustCompile(`(?i)\b(email|e-mail)\b`)
	rxPhone           = regexp.MustCompile(`(?i)\b(phone|mobile|telephone|cell)\b`)
	rxResume          = regexp.MustCompile(`(?i)\b(resume|cv)\b`)
	rxCoverLetter     = regexp.MustCompile(`(?i)\b(cover[\s_-]?letter)\b`)
	rxLinkedIn        = regexp.MustCompile(`(?i)\b(linkedin)\b`)
	rxGitHub          = regexp.MustCompile(`(?i)\b(github)\b`)
	rxPortfolio       = regexp.MustCompile(`(?i)\b(portfolio|website|urls\[portfolio\])\b`)
	rxLocation        = regexp.MustCompile(`(?i)\b(location|located|city|address)\b`)
	rxWorkAuth        = regexp.MustCompile(`(?i)(authorized\s+to\s+work|work\s+authorization|legal(ly)?\s+right\s+to\s+work|eligible\s+to\s+work|permitted\s+to\s+work)`)
	rxSponsorship     = regexp.MustCompile(`(?i)(sponsor(ship)?|visa\s+status|(need|require).*visa|work\s+visa)`)
	rxSalary          = regexp.MustCompile(`(?i)(salary|compensation|expected\s+pay|hourly\s+rate)`)
	rxExperience      = regexp.MustCompile(`(?i)(years\s+of\s+experience|total\s+experience)`)
	rxNotice          = regexp.MustCompile(`(?i)(notice\s+period|start\s+date|availability)`)
	rxEEOGender       = regexp.MustCompile(`(?i)\b(gender)\b`)
	rxEEORace         = regexp.MustCompile(`(?i)\b(race|ethnicity|hispanic|latino)\b`)
	rxEEOVeteran      = regexp.MustCompile(`(?i)\b(veteran)\b`)
	rxEEODisability   = regexp.MustCompile(`(?i)\b(disability)\b`)
)

// RawATSField is an intermediate representation of a field discovered by a harvester.
type RawATSField struct {
	ID           string            `json:"id"`
	Selector     string            `json:"selector,omitempty"`
	Label        string            `json:"label"`
	Type         string            `json:"type"`
	Required     bool              `json:"required"`
	Options      []string          `json:"options,omitempty"`
	OptionValues map[string]any    `json:"option_values,omitempty"` // Map of Option Label -> Raw Option Value/ID
	DefaultValue string            `json:"default_value,omitempty"`
	Validation   string            `json:"validation,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// ASTSchemaNormalizer converts raw ATS fields into canonical models.JobFormSchema
// with a pre-compiled flat AST for sub-2ms online candidate resolution.
type ASTSchemaNormalizer struct {
	screeningRules map[string]string
}

// NewASTSchemaNormalizer initializes a normalizer with optional screening rule overrides.
func NewASTSchemaNormalizer(rules map[string]string) *ASTSchemaNormalizer {
	if rules == nil {
		rules = make(map[string]string)
	}
	return &ASTSchemaNormalizer{
		screeningRules: rules,
	}
}

// Normalize transforms raw fields and metadata into a canonical models.JobFormSchema.
func (n *ASTSchemaNormalizer) Normalize(
	provider string,
	companyID string,
	jobID string,
	jobURL string,
	target models.SubmissionTarget,
	rawFields []RawATSField,
	cfg *models.ATSCompanyConfig,
) *models.JobFormSchema {
	provider = strings.ToLower(strings.TrimSpace(provider))
	companyID = strings.ToLower(strings.TrimSpace(companyID))
	jobID = strings.TrimSpace(jobID)

	var normalizedFields []models.NormalizedFormField
	for _, raw := range rawFields {
		field := n.normalizeField(provider, raw, cfg)
		normalizedFields = append(normalizedFields, field)
	}

	schema := &models.JobFormSchema{
		ID:               models.GenerateJobFormSchemaID(provider, companyID, jobID),
		Provider:         provider,
		CompanyID:        companyID,
		JobID:            jobID,
		JobURL:           jobURL,
		SubmissionTarget: target,
		Fields:           normalizedFields,
		Version:          1,
		ResolvedAt:       time.Now().UTC(),
		ExpiresAt:        time.Now().UTC().Add(48 * time.Hour), // 48-hour cache TTL
	}

	// Pre-compile flat AST for <2ms online resolution
	schema.CompiledAST = n.PrecompileAST(schema, rawFields)

	return schema
}

// normalizeField maps an individual raw field to a canonical NormalizedFormField.
func (n *ASTSchemaNormalizer) normalizeField(provider string, raw RawATSField, cfg *models.ATSCompanyConfig) models.NormalizedFormField {
	cleanType := n.normalizeFieldType(raw.Type)
	mappingKey := n.inferMappingKey(raw.ID, raw.Label)
	embeddingID := ""

	// If not a standard profile contact field, compute question hash for embedding lookup
	if mappingKey == "" || strings.HasPrefix(mappingKey, "candidate.custom_field_answers") {
		hash := n.computeFieldHash(raw.Label)
		embeddingID = fmt.Sprintf("emb-%s", hash)
	}

	selector := raw.Selector
	if selector == "" {
		selector = n.inferSelector(provider, raw.ID, cleanType)
	}

	defaultValue := raw.DefaultValue
	if cfg != nil && cfg.ScreeningRules != nil {
		for pattern, answer := range cfg.ScreeningRules {
			if strings.Contains(strings.ToLower(raw.Label), strings.ToLower(pattern)) {
				defaultValue = answer
				break
			}
		}
	}
	if defaultValue == "" && n.screeningRules != nil {
		for pattern, answer := range n.screeningRules {
			if strings.Contains(strings.ToLower(raw.Label), strings.ToLower(pattern)) {
				defaultValue = answer
				break
			}
		}
	}

	return models.NormalizedFormField{
		FieldID:      raw.ID,
		Selector:     selector,
		Label:        raw.Label,
		Type:         cleanType,
		Required:     raw.Required,
		MappingKey:   mappingKey,
		EmbeddingID:  embeddingID,
		Options:      raw.Options,
		DefaultValue: defaultValue,
		Validation:   raw.Validation,
	}
}

// normalizeFieldType standardizes heterogeneous ATS field types.
func (n *ASTSchemaNormalizer) normalizeFieldType(rawType string) string {
	t := strings.ToLower(strings.TrimSpace(rawType))
	switch {
	case strings.Contains(t, "file") || strings.Contains(t, "attachment") || strings.Contains(t, "upload"):
		return "file"
	case strings.Contains(t, "textarea") || strings.Contains(t, "longtext") || strings.Contains(t, "paragraph"):
		return "textarea"
	case strings.Contains(t, "multi_value_multi_select") || strings.Contains(t, "multiselect") || strings.Contains(t, "multivalueselect"):
		return "multiselect"
	case strings.Contains(t, "multi_value_single_select") || strings.Contains(t, "select") || strings.Contains(t, "dropdown") || strings.Contains(t, "valueselect"):
		return "select"
	case strings.Contains(t, "radio"):
		return "radio"
	case strings.Contains(t, "checkbox"):
		return "checkbox"
	case strings.Contains(t, "bool"):
		return "boolean"
	case strings.Contains(t, "number") || strings.Contains(t, "integer") || strings.Contains(t, "decimal"):
		return "number"
	case strings.Contains(t, "date"):
		return "date"
	case strings.Contains(t, "email"):
		return "email"
	case strings.Contains(t, "phone") || strings.Contains(t, "tel"):
		return "tel"
	default:
		return "text"
	}
}

// inferMappingKey resolves standard candidate profile attributes from field names/labels.
func (n *ASTSchemaNormalizer) inferMappingKey(id, label string) string {
	cleanID := strings.TrimSpace(id)
	cleanLabel := strings.TrimSpace(label)
	target := strings.TrimSpace(strings.ToLower(cleanID + " " + cleanLabel))

	switch {
	case rxFirstName.MatchString(target):
		return "candidate.first_name"
	case rxLastName.MatchString(target):
		return "candidate.last_name"
	case strings.EqualFold(cleanID, "name") || strings.EqualFold(cleanLabel, "name") || rxFullName.MatchString(target):
		return "candidate.full_name"
	case rxEmail.MatchString(target):
		return "candidate.email"
	case rxPhone.MatchString(target):
		return "candidate.phone"
	case rxResume.MatchString(target):
		return "candidate.resume"
	case rxCoverLetter.MatchString(target):
		return "candidate.cover_letter"
	case rxLinkedIn.MatchString(target):
		return "candidate.linkedin_url"
	case rxGitHub.MatchString(target):
		return "candidate.github_url"
	case rxPortfolio.MatchString(target):
		return "candidate.portfolio_url"
	case rxWorkAuth.MatchString(target):
		return "candidate.authorized_to_work_in_us"
	case rxSponsorship.MatchString(target):
		return "candidate.requires_sponsorship"
	case rxSalary.MatchString(target):
		return "candidate.desired_salary_min"
	case rxExperience.MatchString(target):
		return "candidate.years_of_experience"
	case rxNotice.MatchString(target):
		return "candidate.notice_period_weeks"
	case rxEEOGender.MatchString(target):
		return "candidate.custom_field_answers.eeo_gender"
	case rxEEORace.MatchString(target):
		return "candidate.custom_field_answers.eeo_race"
	case rxEEOVeteran.MatchString(target):
		return "candidate.custom_field_answers.eeo_veteran"
	case rxEEODisability.MatchString(target):
		return "candidate.custom_field_answers.eeo_disability"
	case rxLocation.MatchString(target) && !strings.Contains(target, "relocat"):
		return "candidate.city"
	default:
		return ""
	}
}

// inferSelector creates default DOM CSS selectors for Tier 2 execution.
func (n *ASTSchemaNormalizer) inferSelector(provider, fieldID, fieldType string) string {
	switch provider {
	case "smartrecruiters":
		switch {
		case strings.Contains(fieldID, "first"):
			return "input#first-name-input, input[name*='firstName' i]"
		case strings.Contains(fieldID, "last"):
			return "input#last-name-input, input[name*='lastName' i]"
		case strings.Contains(fieldID, "email"):
			return "input#email-input, input[name*='email' i]"
		case strings.Contains(fieldID, "phone"):
			return "input#phone-number-input, input[name*='phone' i]"
		case strings.Contains(fieldID, "location") || strings.Contains(fieldID, "city"):
			return "input#location-input, input[name*='location' i]"
		case fieldType == "file":
			return "input[type='file']"
		default:
			return fmt.Sprintf("input[name='%s'], textarea[name='%s']", fieldID, fieldID)
		}
	case "workday":
		return fmt.Sprintf("[data-automation-id='%s']", fieldID)
	case "lever":
		return fmt.Sprintf("[name='%s']", fieldID)
	default:
		return fmt.Sprintf("#%s, [name='%s']", fieldID, fieldID)
	}
}

// computeFieldHash generates a 16-character SHA-256 hex hash from a question label.
func (n *ASTSchemaNormalizer) computeFieldHash(label string) string {
	clean := strings.ToLower(strings.TrimSpace(label))
	sum := sha256.Sum256([]byte(clean))
	return hex.EncodeToString(sum[:8])
}

// PrecompileAST generates the optimized flat AST map enabling sub-2ms online candidate resolution.
func (n *ASTSchemaNormalizer) PrecompileAST(schema *models.JobFormSchema, rawFields []RawATSField) map[string]any {
	flatFields := make(map[string]any, len(schema.Fields))
	fieldIDs := make([]string, 0, len(schema.Fields))
	requiredKeys := make([]string, 0, len(schema.Fields))
	selectors := make(map[string]string, len(schema.Fields))
	candidateBindings := make(map[string]string, len(schema.Fields))
	optionValues := make(map[string]map[string]any, len(schema.Fields))

	// Index raw option values
	rawOptMap := make(map[string]map[string]any, len(rawFields))
	for _, rf := range rawFields {
		if len(rf.OptionValues) > 0 {
			rawOptMap[rf.ID] = rf.OptionValues
		}
	}

	for _, f := range schema.Fields {
		fieldIDs = append(fieldIDs, f.FieldID)
		if f.Required {
			requiredKeys = append(requiredKeys, f.FieldID)
		}
		if f.Selector != "" {
			selectors[f.FieldID] = f.Selector
		}
		if f.MappingKey != "" {
			candidateBindings[f.MappingKey] = f.FieldID
		}

		fEntry := map[string]any{
			"field_id":      f.FieldID,
			"label":         f.Label,
			"type":          f.Type,
			"required":      f.Required,
			"selector":      f.Selector,
			"mapping_key":   f.MappingKey,
			"embedding_id":  f.EmbeddingID,
			"default_value": f.DefaultValue,
			"options":       f.Options,
		}

		if opts, ok := rawOptMap[f.FieldID]; ok {
			optionValues[f.FieldID] = opts
			fEntry["option_values"] = opts
		}

		flatFields[f.FieldID] = fEntry
	}

	return map[string]any{
		"schema_id":          schema.ID,
		"provider":           schema.Provider,
		"company_id":         schema.CompanyID,
		"job_id":             schema.JobID,
		"tier":               schema.SubmissionTarget.Tier,
		"endpoint":           schema.SubmissionTarget.Endpoint,
		"method":             schema.SubmissionTarget.Method,
		"encoding":           schema.SubmissionTarget.Encoding,
		"field_ids":          fieldIDs,
		"required_keys":      requiredKeys,
		"selectors":          selectors,
		"candidate_bindings": candidateBindings,
		"option_values":      optionValues,
		"flat_fields":        flatFields,
		"precompiled_at":     time.Now().UTC().Format(time.RFC3339Nano),
	}
}
