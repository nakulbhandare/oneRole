package email_otp

import (
	"context"
	"testing"
	"time"
)

func TestExtractOTPAndMagicLink(t *testing.T) {
	reg := GetRegistry()

	tests := []struct {
		name       string
		subject    string
		body       string
		expectedCode string
		expectedLink string
	}{
		{
			name:         "Standard 6-digit numeric OTP",
			subject:      "Your verification code for Motive",
			body:         "Hello Nakul, your verification code is 482910. It expires in 10 minutes.",
			expectedCode: "482910",
		},
		{
			name:         "Contextual OTP with prefix",
			subject:      "OneRole Verification",
			body:         "Please enter code: 891234 to verify your application submission.",
			expectedCode: "891234",
		},
		{
			name:         "Alphanumeric Verification Code",
			subject:      "Security Code",
			body:         "Your Security Code: K9X42P. Enter this code into your browser.",
			expectedCode: "K9X42P",
		},
		{
			name:         "Magic Verification Link",
			subject:      "Confirm your job application",
			body:         "Thank you for applying. Click here to confirm: https://jobs.ashbyhq.com/openai/verify?token=abcdef123456",
			expectedLink: "https://jobs.ashbyhq.com/openai/verify?token=abcdef123456",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payload := reg.DeliverIncoming("bhandarenakul11@gmail.com", "motive.com", tc.subject, tc.body)
			if tc.expectedCode != "" && payload.Code != tc.expectedCode {
				t.Fatalf("expected code %q, got %q", tc.expectedCode, payload.Code)
			}
			if tc.expectedLink != "" && payload.MagicLink != tc.expectedLink {
				t.Fatalf("expected link %q, got %q", tc.expectedLink, payload.MagicLink)
			}
		})
	}
}

func TestWaitForOTP(t *testing.T) {
	reg := GetRegistry()
	email := "test-candidate@example.com"
	domain := "greenhouse.io"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Asynchronously deliver in 50ms
	go func() {
		time.Sleep(50 * time.Millisecond)
		reg.DeliverIncoming(email, domain, "Verification", "Your code is 771923")
	}()

	payload, err := reg.WaitForOTP(ctx, email, domain)
	if err != nil {
		t.Fatalf("WaitForOTP failed: %v", err)
	}
	if payload.Code != "771923" {
		t.Fatalf("expected 771923, got %s", payload.Code)
	}
}
