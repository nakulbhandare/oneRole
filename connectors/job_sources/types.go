package job_sources

import (
	"context"
	"errors"
	"time"
)

type Provider string

const (
	ProviderGreenhouse      Provider = "greenhouse"
	ProviderLever           Provider = "lever"
	ProviderAshby           Provider = "ashby"
	ProviderSmartRecruiters Provider = "smartrecruiters"
	ProviderBreezy          Provider = "breezy"
	ProviderRecruitee       Provider = "recruitee"
	ProviderBambooHR        Provider = "bamboohr"
	ProviderWorkday         Provider = "workday"
	ProviderGeneric         Provider = "generic"
)

var (
	ErrSourceNotEnabled = errors.New("job source is not enabled for discovery")
	ErrUnsupported      = errors.New("job source provider is unsupported")
	ErrInvalidConfig    = errors.New("invalid job source configuration")
)

// SourceConfig is the effective, reviewed configuration read from the source
// registry. Unknown providers and incomplete permissions fail closed.
type SourceConfig struct {
	ID                 string
	CompanyID          string
	Provider           Provider
	BoardIdentifier    string
	Endpoint           string
	AllowedHosts       []string
	Status             string
	DiscoveryEnabled   bool
	AllowedOperations  []string
	RequestsPerWindow  int
	Window             time.Duration
	MaxConcurrency     int
	ConfigVersion      int
	ConnectorVersion   string
	PolicyReviewedAt   *time.Time
	LastSuccessfulSync *time.Time
	LeaseOwner         string
	LeaseToken         string
	LeaseUntil         *time.Time
	CheckpointCursor   string
	MaxPageSize        int
	RequestTimeout     time.Duration
	MaxResponseBytes   int64
}

// Posting is a source observation, not a normalized OneRole job. Fields that
// a provider does not state remain empty; downstream enrichment may not turn
// missing data into a positive claim.
type Posting struct {
	ExternalID    string
	Title         string
	Description   string
	CanonicalURL  string
	ApplyURL      string
	PublishedAt   *time.Time
	UpdatedAt     *time.Time
	SourceStatus  string
	Locations     []string
	WorkplaceMode string
	Department    string
	Team          string
	Level         string
	Employment    string
}

type Page struct {
	Items        []Posting
	NextCursor   string
	Complete     bool
	FullSnapshot bool
	PageNumber   int
}

type Connector interface {
	Fetch(ctx context.Context, source SourceConfig, cursor string) (Page, error)
}

type SyncRun struct {
	SourceID       string
	StartedAt      time.Time
	FinishedAt     *time.Time
	PagesRead      int
	RecordsSeen    int
	RecordsCreated int
	RecordsUpdated int
	RecordsClosed  int
	Status         string
	ErrorClass     string
}

// RateLimiter coordinates reads for a source. Production workers can inject a
// shared implementation if more than one worker may hold the same source.
type RateLimiter interface {
	Acquire(context.Context, SourceConfig) (release func(), err error)
}
