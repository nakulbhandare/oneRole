package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/oneRole/models"
)

type applicationWorkflowStoreStub struct {
	accepted WorkflowAcceptance
	batch    BatchView
	app      models.Application
	acceptID IdempotencyIdentity
	scope    ActorScope
	command  CreateApplicationCommand
	err      error
}

func (s *applicationWorkflowStoreStub) AcceptApplication(_ context.Context, scope ActorScope, command CreateApplicationCommand, idem IdempotencyIdentity, _ time.Time) (WorkflowAcceptance, error) {
	s.scope, s.command, s.acceptID = scope, command, idem
	return s.accepted, s.err
}
func (s *applicationWorkflowStoreStub) AcceptSubmission(_ context.Context, scope ActorScope, _ string, idem IdempotencyIdentity, _ time.Time) (WorkflowAcceptance, error) {
	s.scope, s.acceptID = scope, idem
	return s.accepted, s.err
}
func (s *applicationWorkflowStoreStub) AcceptBatch(_ context.Context, scope ActorScope, _ CreateBatchCommand, idem IdempotencyIdentity, _ time.Time) (WorkflowAcceptance, error) {
	s.scope, s.acceptID = scope, idem
	return s.accepted, s.err
}
func (s *applicationWorkflowStoreStub) GetApplication(context.Context, ActorScope, string) (models.Application, error) {
	return s.app, s.err
}
func (s *applicationWorkflowStoreStub) GetBatch(context.Context, ActorScope, string) (BatchView, error) {
	return s.batch, s.err
}
func (s *applicationWorkflowStoreStub) ListApplications(context.Context, ActorScope, *string, string, int) ([]models.Application, *string, error) {
	return []models.Application{s.app}, nil, s.err
}
func (s *applicationWorkflowStoreStub) GetApplicationTimeline(context.Context, ActorScope, string) ([]models.ApplicationEvent, error) {
	return nil, s.err
}
func (s *applicationWorkflowStoreStub) GetApplicationReceipt(context.Context, ActorScope, string) (*models.ApplicationReceipt, error) {
	return nil, s.err
}

func TestApplicationServiceHashesIdempotencyInputsAndUsesTrustedScope(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	store := &applicationWorkflowStoreStub{accepted: WorkflowAcceptance{
		WorkflowID: "33333333-3333-4333-8333-333333333333", Status: "preparing", AcceptedAt: now,
	}}
	service, err := NewApplicationService(store, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }
	scope := ActorScope{TenantID: "11111111-1111-4111-8111-111111111111", UserID: "22222222-2222-4222-8222-222222222222"}
	command := CreateApplicationCommand{
		JobID: "44444444-4444-4444-8444-444444444444", ResumeVersionID: "55555555-5555-4555-8555-555555555555",
		ConsentID: "66666666-6666-4666-8666-666666666666", Mode: ApplicationModeReviewBeforeSubmit,
	}
	accepted, err := service.CreateApplication(context.Background(), scope, command, "user-supplied-idempotency-key")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if accepted.WorkflowID != store.accepted.WorkflowID || store.scope != scope || store.command != command {
		t.Fatalf("trusted scope or command did not reach store: %#v %#v", store.scope, store.command)
	}
	if store.acceptID.KeyHash == ([32]byte{}) || store.acceptID.RequestHash == ([32]byte{}) {
		t.Fatal("idempotency hashes must be populated")
	}
	if string(store.acceptID.KeyHash[:]) == "user-supplied-idempotency-key" {
		t.Fatal("raw idempotency key was passed to persistence")
	}
	changed, err := service.idempotencyIdentity("create_application", "user-supplied-idempotency-key", CreateApplicationCommand{
		JobID: command.JobID, ResumeVersionID: command.ResumeVersionID, ConsentID: command.ConsentID,
		Mode: ApplicationModeSubmitWhenReady,
	})
	if err != nil {
		t.Fatal(err)
	}
	if changed.KeyHash != store.acceptID.KeyHash || changed.RequestHash == store.acceptID.RequestHash {
		t.Fatal("same key must hash consistently while changed body has a different request hash")
	}
}

func TestCreateBatchRequiresDistinctBoundedIDs(t *testing.T) {
	command := CreateBatchCommand{
		JobIDs:          []string{"88888888-8888-4888-8888-888888888888", "99999999-9999-4999-8999-999999999999"},
		ResumeVersionID: "55555555-5555-4555-8555-555555555555", ConsentID: "66666666-6666-4666-8666-666666666666",
		Mode: ApplicationModeSubmitWhenReady,
	}
	if err := command.Validate(); err != nil {
		t.Fatalf("valid batch rejected: %v", err)
	}
	command.JobIDs = []string{"88888888-8888-4888-8888-888888888888", "88888888-8888-4888-8888-888888888888"}
	if !errors.Is(command.Validate(), ErrApplicationRequest) {
		t.Fatal("duplicate job IDs must be rejected")
	}
	command.JobIDs = make([]string, MaxApplicationBatchSize+1)
	for i := range command.JobIDs {
		command.JobIDs[i] = fmt.Sprintf("00000000-0000-4000-8000-%012x", i+1)
	}
	if !errors.Is(command.Validate(), ErrApplicationRequest) {
		t.Fatal("batch above the 500 item cap must be rejected")
	}
}

func TestConsentScopeV1RequiresExplicitCompanyAndDataUse(t *testing.T) {
	valid := ConsentScopeV1{
		SchemaVersion: 1,
		Application: ConsentApplicationScope{
			Prepare: true, Submit: true,
			Modes:      []ApplicationMode{ApplicationModeReviewBeforeSubmit, ApplicationModeSubmitWhenReady},
			CompanyIDs: []string{"*"},
		},
		DataUse: ConsentDataUseScope{
			ResumeProcessing: true, EvidenceBasedAnswerGeneration: true, SharingWithEmployer: true,
		},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid consent rejected: %v", err)
	}
	invalid := valid
	invalid.Application.CompanyIDs = nil
	if !errors.Is(invalid.Validate(), ErrConsentInvalid) {
		t.Fatal("company scope must be explicit")
	}
	invalid = valid
	invalid.Application.CompanyIDs = []string{"*", "company-1"}
	if !errors.Is(invalid.Validate(), ErrConsentInvalid) {
		t.Fatal("wildcard cannot be combined with explicit companies")
	}
	invalid = valid
	invalid.DataUse.SharingWithEmployer = false
	if !errors.Is(invalid.Validate(), ErrConsentInvalid) {
		t.Fatal("employer sharing must be explicit")
	}
}

func TestBatchViewValidatesCounts(t *testing.T) {
	batch := BatchView{
		ID: "77777777-7777-4777-8777-777777777777", Total: 2,
		Counts:    map[string]int{"accepted": 1, "preparing": 1},
		CreatedAt: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC),
	}
	if err := batch.Validate(); err != nil {
		t.Fatalf("valid batch view rejected: %v", err)
	}
	batch.Counts["preparing"] = -1
	if !errors.Is(batch.Validate(), ErrApplicationStore) {
		t.Fatal("negative status count must be rejected")
	}
}
