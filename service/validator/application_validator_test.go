package validator

import (
	"testing"
	"github.com/oneRole/models"
)

func TestValidateApplication(t *testing.T) {
	tests := []struct {
		name      string
		appID     string
		appStatus string
		jobID     string
		wantErr   bool
	}{
		{"Valid preparing", "app1", string(models.ApplicationPreparing), "job1", false},
		{"Valid queued", "app1", string(models.ApplicationQueued), "job1", false},
		{"Valid failed retryable", "app1", string(models.ApplicationFailedRetryable), "job1", false},
		{"Invalid status", "app1", string(models.ApplicationSubmitted), "job1", true},
		{"Empty appID", "", string(models.ApplicationPreparing), "job1", true},
		{"Empty jobID", "app1", string(models.ApplicationPreparing), "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateApplication(tt.appID, tt.appStatus, tt.jobID); (err != nil) != tt.wantErr {
				t.Errorf("ValidateApplication() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
