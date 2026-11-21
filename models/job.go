package models

import "time"

type Job struct {
	ID                     string                      `json:"id"`
	CompanyID              *string                     `json:"company_id"`
	Company                string                      `json:"company"`
	Title                  string                      `json:"title"`
	Description            string                      `json:"description"`
	DescriptionHTML        string                      `json:"description_html,omitempty"`
	ResponsibilitiesText   string                      `json:"responsibilities_text,omitempty"`
	QualificationsText     string                      `json:"qualifications_text,omitempty"`
	Location               string                      `json:"location"`
	CountryCode            *string                     `json:"country_code"`
	Region                 *string                     `json:"region"`
	City                   *string                     `json:"city"`
	WorkplaceModes         []WorkplaceMode             `json:"workplace_modes"`
	RoleIDs                []string                    `json:"role_ids"`
	JobType                *JobType                    `json:"job_type"`
	EmploymentTypes        []EmploymentType            `json:"employment_types"`
	DegreeLevels           []DegreeLevel               `json:"degree_levels"`
	DegreeRequirement      *DegreeRequirement          `json:"degree_requirement"`
	ExperienceMinYears     *int                        `json:"experience_min_years"`
	ExperienceMaxYears     *int                        `json:"experience_max_years"`
	VisaSponsorship        VisaSponsorship             `json:"visa_sponsorship"`
	FilterProvenance       map[string]FilterProvenance `json:"filter_provenance"`
	PublishedAt            *time.Time                  `json:"published_at"`
	PublicationTimeQuality PublicationTimeQuality      `json:"publication_time_quality"`
	FirstSeenAt            time.Time                   `json:"first_seen_at"`
	LastSeenAt             time.Time                   `json:"last_seen_at"`
	ApplicationCapability  ApplicationCapability       `json:"application_capability"`
	SourceURL              string                      `json:"source_url"`
	Highlights             map[string][]string         `json:"highlights,omitempty"`
	JobDomain              string                      `json:"job_domain,omitempty" bson:"job_domain,omitempty"`
	CompanyTier            string                      `json:"company_tier,omitempty" bson:"company_tier,omitempty"`
	TargetCountry          string                      `json:"target_country,omitempty" bson:"target_country,omitempty"`
}

type ProvenanceStatus string

const (
	ProvenanceSourceExplicit ProvenanceStatus = "source_explicit"
	ProvenanceNormalized     ProvenanceStatus = "normalized"
	ProvenanceInferred       ProvenanceStatus = "inferred"
	ProvenanceUnknown        ProvenanceStatus = "unknown"
)

type FilterProvenance struct {
	Status     ProvenanceStatus `json:"status"`
	SourceURL  *string          `json:"source_url"`
	ObservedAt *time.Time       `json:"observed_at"`
	Confidence *float64         `json:"confidence"`
}

type PublicationTimeQuality string

const (
	PublicationSourceProvided PublicationTimeQuality = "source_provided"
	PublicationUnknown        PublicationTimeQuality = "unknown"
)

type ApplicationCapability string

const (
	CapabilityAutomatedSupported     ApplicationCapability = "automated_supported"
	CapabilityAssistedSupported      ApplicationCapability = "assisted_supported"
	CapabilityManualRequired         ApplicationCapability = "manual_required"
	CapabilityUnsupported            ApplicationCapability = "unsupported"
	CapabilityRequiresAuthentication ApplicationCapability = "requires_authentication"
	CapabilityRequiresInformation    ApplicationCapability = "requires_information"
)

type JobSearchResponse struct {
	Items          []Job            `json:"items"`
	AppliedFilters JobSearchRequest `json:"applied_filters"`
	Facets         FacetSet         `json:"facets"`
	NextCursor     *string          `json:"next_cursor"`
}

type FacetSet map[string]Facet

type Facet struct {
	Options         []FacetOption `json:"options"`
	UnknownCount    int           `json:"unknown_count"`
	TaxonomyVersion *string       `json:"taxonomy_version"`
}

type FacetOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

type JobPage struct {
	Items      []Job   `json:"items"`
	NextCursor *string `json:"next_cursor"`
}

// SourceSummary is a high-level representation of an ingested company source.
type SourceSummary struct {
	ID           string     `json:"id" bson:"_id"`
	CompanyID    string     `json:"company_id" bson:"company_id"`
	CompanyName  string     `json:"company_name,omitempty" bson:"company_name,omitempty"`
	SourceType   string     `json:"source_type" bson:"source_type"`
	ProviderType string     `json:"provider_type" bson:"provider_type"`
	BaseURL      string     `json:"base_url" bson:"base_url"`
	Status       string     `json:"status" bson:"status"`
	LastSyncAt   *time.Time `json:"last_sync_at,omitempty" bson:"last_sync_at,omitempty"`
	LogCount     int64      `json:"log_count" bson:"log_count"`
}

// SourceHealth details current operational metrics for an ingestion source.
type SourceHealth struct {
	SourceID       string     `json:"source_id"`
	Status         string     `json:"status"`
	LastErrorClass string     `json:"last_error_class,omitempty"`
	LastSyncAt     *time.Time `json:"last_sync_at,omitempty"`
	SyncLagSeconds int64      `json:"sync_lag_seconds"`
	Capability     string     `json:"capability"`
	LogCount       int64      `json:"log_count"`
}
