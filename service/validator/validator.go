package validator

import (
	"errors"
	"fmt"
	"strings"

	"github.com/oneRole/connectors/job_sources"
)

var (
	ErrInvalidPosting = errors.New("invalid posting data")
)

// ValidatePosting checks if the posting has required fields populated.
func ValidatePosting(posting job_sources.Posting) error {
	if strings.TrimSpace(posting.ExternalID) == "" {
		return fmt.Errorf("%w: missing external ID", ErrInvalidPosting)
	}
	if strings.TrimSpace(posting.Title) == "" {
		return fmt.Errorf("%w: missing title", ErrInvalidPosting)
	}
	if strings.TrimSpace(posting.CanonicalURL) == "" {
		return fmt.Errorf("%w: missing canonical URL", ErrInvalidPosting)
	}
	return nil
}
