package service

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMailer_LoadConfigFromEnv(t *testing.T) {
	origHost := os.Getenv("SMTP_HOST")
	origPort := os.Getenv("SMTP_PORT")
	origUser := os.Getenv("SMTP_USER")
	origPass := os.Getenv("SMTP_PASS")
	defer func() {
		os.Setenv("SMTP_HOST", origHost)
		os.Setenv("SMTP_PORT", origPort)
		os.Setenv("SMTP_USER", origUser)
		os.Setenv("SMTP_PASS", origPass)
	}()

	os.Setenv("SMTP_HOST", "smtp.gmail.com")
	os.Setenv("SMTP_PORT", "587")
	os.Setenv("SMTP_USER", "testuser@gmail.com")
	os.Setenv("SMTP_PASS", "secret-app-pass")

	cfg := LoadMailerConfigFromEnv()
	if cfg.SMTPHost != "smtp.gmail.com" {
		t.Errorf("expected SMTPHost smtp.gmail.com, got %s", cfg.SMTPHost)
	}
	if cfg.SMTPPort != 587 {
		t.Errorf("expected SMTPPort 587, got %d", cfg.SMTPPort)
	}
	if cfg.SMTPUser != "testuser@gmail.com" {
		t.Errorf("expected SMTPUser testuser@gmail.com, got %s", cfg.SMTPUser)
	}
	if cfg.SMTPPass != "secret-app-pass" {
		t.Errorf("expected SMTPPass secret-app-pass, got %s", cfg.SMTPPass)
	}
}

func TestMailer_UnconfiguredHandling(t *testing.T) {
	mailer := NewMailerWithConfig(MailerConfig{})
	status := mailer.GetConfigStatus()

	if status["active_provider"] != "none" {
		t.Errorf("expected active_provider 'none', got %v", status["active_provider"])
	}
	if status["is_ready"] != false {
		t.Errorf("expected is_ready false, got %v", status["is_ready"])
	}

	res, err := mailer.SendEmail(context.Background(), EmailPayload{
		Recipient: "bhandarenakul11@gmail.com",
		Subject:   "Test Application",
		TextBody:  "Test body",
	})

	if err == nil {
		t.Fatalf("expected error when mailer unconfigured, got nil")
	}
	if res.Success {
		t.Errorf("expected success to be false, got true")
	}
	if res.Provider != "unconfigured" {
		t.Errorf("expected provider 'unconfigured', got %s", res.Provider)
	}
	if !strings.Contains(err.Error(), "unconfigured") {
		t.Errorf("expected error message to mention unconfigured, got %v", err)
	}
}

func TestMailer_GenerateConfirmationEmailContent(t *testing.T) {
	text, html := GenerateConfirmationEmailContent("Software Engineer", "Ramp", "app-12345", "rcpt-67890", time.Now().UTC())

	if !strings.Contains(text, "Software Engineer") || !strings.Contains(text, "Ramp") {
		t.Errorf("text content missing job title or company name: %s", text)
	}
	if !strings.Contains(text, "rcpt-67890") {
		t.Errorf("text content missing receipt ID: %s", text)
	}

	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Errorf("html content missing DOCTYPE")
	}
	if !strings.Contains(html, "Software Engineer") || !strings.Contains(html, "Ramp") {
		t.Errorf("html content missing job title or company name")
	}
	if !strings.Contains(html, "rcpt-67890") {
		t.Errorf("html content missing receipt ID")
	}
}
