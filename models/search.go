package models

import "time"

// JobSearchRequest is the normalized domain representation of the public
// /api/v1/jobs/search filter request. Unknown-value switches are separate
// from known categorical values so missing enrichment is never guessed.
type JobSearchRequest struct {
	JobIDs                       []string          `json:"job_ids,omitempty"`
	JobID                        string            `json:"job_id,omitempty"`
	ID                           string            `json:"id,omitempty"`
	Query                        string            `json:"query,omitempty"`
	ExcludeTerms                 []string          `json:"exclude_terms,omitempty"`
	Date                         DateFilter        `json:"date"`
	IncludeUnknownLocation       bool              `json:"include_unknown_location"`
	IncludeUnknownCompany        bool              `json:"include_unknown_company"`
	IncludeUnknownDegree         bool              `json:"include_unknown_degree"`
	IncludeUnknownWorkplace      bool              `json:"include_unknown_workplace"`
	IncludeUnknownRole           bool              `json:"include_unknown_role"`
	IncludeUnknownJobType        bool              `json:"include_unknown_job_type"`
	IncludeUnknownEmploymentType bool              `json:"include_unknown_employment_type"`
	LocationIDs                  []string          `json:"location_ids,omitempty"`
	Locations                    []LocationFilter  `json:"locations,omitempty"`
	WorkplaceModes               []WorkplaceMode   `json:"workplace_modes,omitempty"`
	CompanyIDs                   []string          `json:"company_ids,omitempty"`
	ExcludedCompanyIDs           []string          `json:"excluded_company_ids,omitempty"`
	DegreeLevels                 []DegreeLevel     `json:"degree_levels,omitempty"`
	DegreeRequirement            DegreeRequirement `json:"degree_requirement"`
	MaxExperienceYears           *int              `json:"max_experience_years,omitempty"`
	IncludeUnknownExperience     bool              `json:"include_unknown_experience"`
	VisaSponsorship              VisaSponsorship   `json:"visa_sponsorship"`
	RoleIDs                      []string          `json:"role_ids,omitempty"`
	JobTypes                     []JobType         `json:"job_types,omitempty"`
	EmploymentTypes              []EmploymentType  `json:"employment_types,omitempty"`
	Sort                         JobSort           `json:"sort"`
	Cursor                       *string           `json:"cursor,omitempty"`
	Limit                        int               `json:"limit"`
	IncludeFacets                bool              `json:"include_facets"`
	JobDomain                    string            `json:"job_domain,omitempty"`
	Industries                   []string          `json:"industries,omitempty"`
	TargetCountry                string            `json:"target_country,omitempty"`
}

type DateFilter struct {
	Preset                        DatePreset `json:"preset"`
	PublishedAfter                *time.Time `json:"published_after,omitempty"`
	PublishedBefore               *time.Time `json:"published_before,omitempty"`
	FirstSeenAfter                *time.Time `json:"first_seen_after,omitempty"`
	IncludeUnknownPublicationTime bool       `json:"include_unknown_publication_time"`
}

// LocationFilter can identify a normalized location and/or provide a
// country, region, or city with an optional radius in kilometers.
type LocationFilter struct {
	LocationID  string `json:"location_id,omitempty"`
	CountryCode string `json:"country_code,omitempty"`
	Region      string `json:"region,omitempty"`
	City        string `json:"city,omitempty"`
	RadiusKM    *int   `json:"radius_km,omitempty"`
}

type DatePreset string

const (
	DateLast6Hours  DatePreset = "last_6_hours"
	DateLast24Hours DatePreset = "last_24_hours"
	DateLast1Day    DatePreset = "last_1_day"
	DateLast3Days   DatePreset = "last_3_days"
	DateLast5Days   DatePreset = "last_5_days"
	DateLast1Week   DatePreset = "last_1_week"
	DateLast7Days   DatePreset = "last_7_days"
	DateLast2Weeks  DatePreset = "last_2_weeks"
	DateLast4Weeks  DatePreset = "last_4_weeks"
	DateLast1Month  DatePreset = "last_1_month"
	DateLast30Days  DatePreset = "last_30_days"
	DateLast60Days  DatePreset = "last_60_days"
	DateCustom      DatePreset = "custom"
	DateAny         DatePreset = "any"
)

type WorkplaceMode string

const (
	WorkplaceRemote  WorkplaceMode = "remote"
	WorkplaceHybrid  WorkplaceMode = "hybrid"
	WorkplaceOnSite  WorkplaceMode = "on_site"
	WorkplaceUnknown WorkplaceMode = "unknown"
)

type DegreeLevel string

const (
	DegreeHighSchool     DegreeLevel = "high_school"
	DegreeAssociate      DegreeLevel = "associate"
	DegreeBachelor       DegreeLevel = "bachelor"
	DegreeMaster         DegreeLevel = "master"
	DegreeDoctorate      DegreeLevel = "doctorate"
	DegreeNoDegreeStated DegreeLevel = "no_degree_stated"
	DegreeUnknown        DegreeLevel = "unknown"
)

type DegreeRequirement string

const (
	DegreeRequired  DegreeRequirement = "required"
	DegreePreferred DegreeRequirement = "preferred"
	DegreeAny       DegreeRequirement = "any"
)

type VisaSponsorship string

const (
	VisaYes     VisaSponsorship = "yes"
	VisaNo      VisaSponsorship = "no"
	VisaUnknown VisaSponsorship = "unknown"
	VisaAny     VisaSponsorship = "any"
)

type JobType string

const (
	JobInternship JobType = "internship"
	JobEntryLevel JobType = "entry_level"
	JobAssociate  JobType = "associate"
	JobMidLevel   JobType = "mid_level"
	JobSenior     JobType = "senior"
	JobLead       JobType = "lead"
	JobManager    JobType = "manager"
	JobDirector   JobType = "director"
	JobExecutive  JobType = "executive"
)

type EmploymentType string

const (
	EmploymentFullTime       EmploymentType = "full_time"
	EmploymentPartTime       EmploymentType = "part_time"
	EmploymentContract       EmploymentType = "contract"
	EmploymentTemporary      EmploymentType = "temporary"
	EmploymentSeasonal       EmploymentType = "seasonal"
	EmploymentFreelance      EmploymentType = "freelance"
	EmploymentInternship     EmploymentType = "internship"
	EmploymentApprenticeship EmploymentType = "apprenticeship"
	EmploymentOther          EmploymentType = "other"
)

type JobSort string

const (
	SortRelevance     JobSort = "relevance"
	SortPublishedDesc JobSort = "published_desc"
	SortFirstSeenDesc JobSort = "first_seen_desc"
	SortSalaryDesc    JobSort = "salary_desc"
)
