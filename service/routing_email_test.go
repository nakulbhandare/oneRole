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

func TestExtractEmployerReceiptID(t *testing.T) {
	// 1. Greenhouse style confirmation
	ghSubject := "Thank you for applying to Gomotive - Staff Agentic AI Platform Engineer"
	ghBody := "Hi Nakul,\n\nThank you for applying to Motive. Your application reference number is #GH-9281052.\n\nBest,\nMotive Talent"
	ghReceipt := extractEmployerReceiptID(ghSubject, ghBody, "")
	if ghReceipt != "GH-9281052" {
		t.Fatalf("expected GH-9281052, got %q", ghReceipt)
	}

	// 2. Ashby style confirmation
	ashbySubject := "Application received: OpenAI - Research Engineer"
	ashbyBody := "We have received your application. Confirmation Number: ASHBY-70459619. We will review your materials shortly."
	ashbyReceipt := extractEmployerReceiptID(ashbySubject, ashbyBody, "")
	if ashbyReceipt != "ASHBY-70459619" {
		t.Fatalf("expected ASHBY-70459619, got %q", ashbyReceipt)
	}

	// 3. Lever style confirmation with submission ID
	leverSubject := "Your application to Pigment"
	leverBody := "Thanks for applying! Submission code: LEV-CONF-391852. You can track status anytime."
	leverReceipt := extractEmployerReceiptID(leverSubject, leverBody, "")
	if leverReceipt != "LEV-CONF-391852" {
		t.Fatalf("expected LEV-CONF-391852, got %q", leverReceipt)
	}

	// 4. Email without explicit receipt - must return empty string (zero synthetic fallback)
	plainSubject := "Thank you for your interest in Anthropic"
	plainBody := "We have received your resume and our team will get back to you if there is a match."
	plainReceipt := extractEmployerReceiptID(plainSubject, plainBody, "")
	if plainReceipt != "" {
		t.Fatalf("expected empty string for plain email, got %q", plainReceipt)
	}
}

func TestDetectAtsFromEmail(t *testing.T) {
	tests := []struct {
		from     string
		subject  string
		body     string
		expected string
	}{
		{"no-reply@greenhouse.io", "Thank you for applying", "Greenhouse application", "greenhouse"},
		{"notifications@gh-mail.io", "Application received", "Processed via Greenhouse", "greenhouse"},
		{"no-reply@hire.lever.co", "Application confirmation", "Powered by Lever", "lever"},
		{"talent@lever.co", "Thanks for applying", "", "lever"},
		{"no-reply@ashbyhq.com", "We received your application", "Ashby Careers", "ashby"},
		{"careers@workday.com", "Application submitted", "MyWorkdayJobs portal", "workday"},
		{"recruiting@stripe.com", "Thank you for applying to Stripe", "Stripe Talent Acquisition", "direct_employer"},
	}

	for _, tt := range tests {
		got := detectAtsFromEmail(tt.from, tt.subject, tt.body)
		if got != tt.expected {
			t.Errorf("detectAtsFromEmail(%q) = %q, want %q", tt.from, got, tt.expected)
		}
	}
}
