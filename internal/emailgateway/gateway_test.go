package emailgateway

import (
	"context"
	"testing"
	"time"

	"github.com/oneRole/connectors/email_otp"
	"github.com/oneRole/internal/emailidentity"
	"github.com/oneRole/models"
)

func TestEmailGatewayProcessInbound(t *testing.T) {
	identitySvc := emailidentity.NewService(nil)
	gw := NewGateway(nil, identitySvc, nil, GatewayConfig{})

	payload := InboundWebhookPayload{
		To:        "Nakul Bhandare <nakul.bhandare.89afcd63@apply.onerole.com>",
		From:      "Greenhouse Careers <no-reply@greenhouse.io>",
		Subject:   "Your verification code for Motive application",
		Text:      "Hello Nakul, your verification code is 482910. Please enter this code.",
		HTML:      "<p>Your verification code is <strong>482910</strong></p>",
		MessageID: "gh-msg-12345",
	}

	res, err := gw.ProcessInbound(context.Background(), payload)
	if err != nil {
		t.Fatalf("ProcessInbound failed: %v", err)
	}

	if res.Category != models.CategoryOTP {
		t.Fatalf("expected CategoryOTP, got %s", res.Category)
	}
	if !res.ContainsOTP {
		t.Fatal("expected ContainsOTP to be true")
	}

	// Verify that the OTP Registry received it
	reg := email_otp.GetRegistry()
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	otpPayload, err := reg.WaitForOTP(ctx, "nakul.bhandare.89afcd63@apply.onerole.com", "greenhouse.io")
	if err != nil {
		t.Fatalf("Registry WaitForOTP failed: %v", err)
	}
	if otpPayload.Code != "482910" {
		t.Fatalf("expected code 482910, got %s", otpPayload.Code)
	}
}

func TestClassifyCategory(t *testing.T) {
	cases := []struct {
		subject  string
		text     string
		expected models.EmailCategory
	}{
		{"Your one-time passcode", "Enter code: 928123", models.CategoryOTP},
		{"Invitation to Interview: Senior SRE", "Schedule via Calendly", models.CategoryInterview},
		{"Coding Assessment for OpenAI", "Please complete on CodeSignal", models.CategoryAssessment},
		{"Thank you for applying to Figma", "We received your application #FIG-8921", models.CategoryConfirmation},
		{"Status Update on your application", "Unfortunately, we are not moving forward", models.CategoryRejection},
	}

	for _, c := range cases {
		got := classifyCategory(c.subject, c.text, "")
		if got != c.expected {
			t.Errorf("classifyCategory(%q) = %s, expected %s", c.subject, got, c.expected)
		}
	}
}
