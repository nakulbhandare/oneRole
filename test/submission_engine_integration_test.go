package test

import (
	"fmt"
	"testing"
	"time"

	"github.com/oneRole/connectors/ats_adapter"
	"github.com/oneRole/models"
	"github.com/oneRole/service"
)

// MockMemoryStore simulates MongoDB application collection for integration tests.
type MockMemoryStore struct {
	apps      map[string]*models.Application
	attempts  map[string]*models.SubmissionAttempt
	events    []models.ApplicationEvent
	tokens    map[string]int64
	leases    map[string]string
	expiry    map[string]time.Time
}

func NewMockMemoryStore() *MockMemoryStore {
	return &MockMemoryStore{
		apps:     make(map[string]*models.Application),
		attempts: make(map[string]*models.SubmissionAttempt),
		events:   make([]models.ApplicationEvent, 0),
		tokens:   make(map[string]int64),
		leases:   make(map[string]string),
		expiry:   make(map[string]time.Time),
	}
}

func (m *MockMemoryStore) SeedApp(appID string, status models.ApplicationStatus) {
	now := time.Now().UTC()
	m.apps[appID] = &models.Application{
		ID:              appID,
		JobID:           "job-123",
		Status:          models.PublicApplicationStatus(status),
		CanonicalStatus: status,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	m.tokens[appID] = 1
}

func (m *MockMemoryStore) AcquireLease(appID, workerID string, duration time.Duration) (*service.ExecutionLease, error) {
	app, ok := m.apps[appID]
	if !ok {
		return nil, fmt.Errorf("app not found")
	}
	if app.CanonicalStatus == models.StatusSubmittedConfirmed {
		return nil, service.ErrAlreadySubmitted
	}

	now := time.Now().UTC()
	if currentOwner, exists := m.leases[appID]; exists && currentOwner != "" && currentOwner != workerID {
		if exp := m.expiry[appID]; exp.After(now) {
			return nil, service.ErrLeaseUnavailable
		}
	}

	m.tokens[appID]++
	token := m.tokens[appID]
	m.leases[appID] = workerID
	m.expiry[appID] = now.Add(duration)
	app.CanonicalStatus = models.StatusSubmitting
	app.FencingToken = token

	return &service.ExecutionLease{
		ApplicationID: appID,
		AttemptNumber: int(token),
		FencingToken:  token,
		LeaseOwner:    workerID,
		ExpiresAt:     now.Add(duration),
	}, nil
}

func (m *MockMemoryStore) FinalizeSuccess(appID string, fencingToken int64, receiptID string) error {
	app, ok := m.apps[appID]
	if !ok {
		return fmt.Errorf("app not found")
	}
	if app.FencingToken != fencingToken {
		return service.ErrFencingTokenStale
	}
	if receiptID == "" {
		return fmt.Errorf("receipt required")
	}

	app.CanonicalStatus = models.StatusSubmittedConfirmed
	app.VerifiedReceiptID = &receiptID
	app.UpdatedAt = time.Now().UTC()
	delete(m.leases, appID)
	return nil
}

func TestSubmissionEngine_DuplicateMessagesDeduplication(t *testing.T) {
	store := NewMockMemoryStore()
	store.SeedApp("app-dup-1", models.StatusQueued)

	// Worker 1 acquires lease
	lease1, err := store.AcquireLease("app-dup-1", "worker-1", 2*time.Minute)
	if err != nil {
		t.Fatalf("worker 1 acquire lease failed: %v", err)
	}

	// Worker 1 finalizes submission
	err = store.FinalizeSuccess("app-dup-1", lease1.FencingToken, "EMP-RECEIPT-001")
	if err != nil {
		t.Fatalf("finalize success failed: %v", err)
	}

	// Duplicate message arrives for Worker 2
	_, errDup := store.AcquireLease("app-dup-1", "worker-2", 2*time.Minute)
	if errDup != service.ErrAlreadySubmitted {
		t.Fatalf("expected ErrAlreadySubmitted on duplicate delivery, got: %v", errDup)
	}
}

func TestSubmissionEngine_FencingTokenRecovery(t *testing.T) {
	store := NewMockMemoryStore()
	store.SeedApp("app-crash-1", models.StatusQueued)

	// Worker 1 claims application
	lease1, err := store.AcquireLease("app-crash-1", "worker-crashed", 100*time.Millisecond)
	if err != nil {
		t.Fatalf("lease 1 failed: %v", err)
	}

	// Worker 1 crashes (simulated by waiting for lease expiration)
	time.Sleep(150 * time.Millisecond)

	// Worker 2 takes over the abandoned lease
	lease2, err := store.AcquireLease("app-crash-1", "worker-healthy", 2*time.Minute)
	if err != nil {
		t.Fatalf("worker 2 takeover failed: %v", err)
	}
	if lease2.FencingToken <= lease1.FencingToken {
		t.Fatalf("expected fencing token to increment, got %d <= %d", lease2.FencingToken, lease1.FencingToken)
	}

	// Crashed Worker 1 wakes up late and attempts to write with stale fencing token
	errStale := store.FinalizeSuccess("app-crash-1", lease1.FencingToken, "STALE-RECEIPT")
	if errStale != service.ErrFencingTokenStale {
		t.Fatalf("expected ErrFencingTokenStale, got: %v", errStale)
	}

	// Healthy Worker 2 finalizes successfully
	errHealthy := store.FinalizeSuccess("app-crash-1", lease2.FencingToken, "HEALTHY-RECEIPT")
	if errHealthy != nil {
		t.Fatalf("healthy finalize failed: %v", errHealthy)
	}
}

func TestSubmissionEngine_AmbiguousOutcomeReconciliation(t *testing.T) {
	store := NewMockMemoryStore()
	store.SeedApp("app-ambig-1", models.StatusSubmitting)

	// Pre-submission reconciliation: verify an ambiguous application cannot blindly retry
	evidence := service.TransitionEvidence{AmbiguousOutcome: true}
	err := service.TransitionApplication(models.StatusSubmitting, models.StatusSubmittedUnverified, evidence)
	if err != nil {
		t.Fatalf("transition to SUBMITTED_UNVERIFIED failed: %v", err)
	}

	// Blind retry without reconciliation must be rejected
	errBlindRetry := service.TransitionApplication(models.StatusSubmittedUnverified, models.StatusQueued, service.TransitionEvidence{})
	if errBlindRetry == nil {
		t.Fatalf("expected transition to be rejected without reconciliation evidence")
	}

	// Valid reconciliation with safe retry allows transition
	reconciledEvidence := service.TransitionEvidence{
		KnownSafeToRetry:       true,
		RetryBudgetRemaining:   true,
		ReconciledNoSubmission: true,
	}
	errReconciled := service.TransitionApplication(models.StatusSubmittedUnverified, models.StatusFailed, reconciledEvidence)
	if errReconciled != nil {
		t.Fatalf("reconciled retry transition failed: %v", errReconciled)
	}
}

func TestSubmissionEngine_CaptchaHandoff(t *testing.T) {
	// Captcha detection transitions to ACTION_REQUIRED without retry churn
	evidence := service.TransitionEvidence{}
	err := service.TransitionApplication(models.StatusSubmitting, models.StatusActionRequired, evidence)
	if err != nil {
		t.Fatalf("transition to ACTION_REQUIRED failed: %v", err)
	}

	adapter := ats_adapter.NewAshbyAdapter(nil)
	class := adapter.ClassifyError(nil, 200, `{"error": "recaptcha enterprise challenge required"}`)
	if class != ats_adapter.ErrorClassBotGateDetected {
		t.Fatalf("expected ErrorClassBotGateDetected, got: %s", class)
	}
}
