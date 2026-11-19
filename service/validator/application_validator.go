package validator

import (
	"errors"
	"fmt"
	"strings"

	"github.com/oneRole/models"
)

var (
	ErrInvalidApplication = errors.New("invalid application data")
)

// ValidateApplication checks if the application has required fields and a valid state for submission.
func ValidateApplication(appID string, appStatus string, jobID string) error {
	if strings.TrimSpace(appID) == "" {
		return fmt.Errorf("%w: missing application ID", ErrInvalidApplication)
	}
	if strings.TrimSpace(jobID) == "" {
		return fmt.Errorf("%w: missing job ID", ErrInvalidApplication)
	}

	// Ensure the application is in a state that allows submission
	validStatuses := []string{
		string(models.ApplicationPreparing),
		string(models.ApplicationQueued),
		string(models.ApplicationFailedRetryable),
	}

	isValid := false
	for _, status := range validStatuses {
		if appStatus == status {
			isValid = true
			break
		}
	}

	if !isValid {
		return fmt.Errorf("%w: application in invalid state for submission: %s", ErrInvalidApplication, appStatus)
	}

	return nil
}
