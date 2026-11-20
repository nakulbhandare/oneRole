package service

import (
	"context"
	"strings"
	"testing"
)

func TestRoutingEmailGenerationAndParsing(t *testing.T) {
	userID := "usr-9afbe74b-b9ad-4094-a25e-bd4c63e1794c"
	firstName := "Nakul"
	lastName := "Bhandare"

	email := GenerateUserRoutingEmail(userID, firstName, lastName)
	if !strings.HasPrefix(email, "nakul.bhandare.") {
		t.Fatalf("expected email to start with nakul.bhandare., got %s", email)
	}
	if !strings.HasSuffix(email, "@"+GetMailDomain()) {
		t.Fatalf("expected domain @%s, got %s", GetMailDomain(), email)
	}

	token := ExtractRoutingToken(email)
	if token == "" {
		t.Fatalf("expected non-empty routing token from %s", email)
	}

	expectedToken := GenerateRoutingToken(userID)
	if token != expectedToken {
		t.Fatalf("expected token %s, got %s", expectedToken, token)
	}
}

func TestProcessInboundEmailClassification(t *testing.T) {
	svc := &EnterpriseService{}

	payload := InboundEmailPayload{
		To:       "nakul.bhandare.8f92a1@mail.onerole.com",
		From:     "Stripe Talent Acquisition <recruiting@stripe.com>",
		Subject:  "Interview Invitation: Senior Software Engineer at Stripe",
		TextBody: "Hi Nakul, we'd love to schedule a 45-minute technical conversation. Please choose a time: https://calendly.com/stripe-talent/tech-interview",
	}

	res, err := svc.ProcessInboundEmail(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success to be true")
	}
	if res.Category != "interview_invite" {
		t.Fatalf("expected category interview_invite, got %s", res.Category)
	}
	if res.Company != "Stripe" {
		t.Fatalf("expected company Stripe, got %s", res.Company)
	}
	if res.CandidateEmail != "bhandarenakul11@gmail.com" {
		t.Fatalf("expected candidate email bhandarenakul11@gmail.com, got %s", res.CandidateEmail)
	}
}
