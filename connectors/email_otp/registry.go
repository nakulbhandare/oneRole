package email_otp

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	otpContextRegex = regexp.MustCompile(`(?i)(?:verification\s+code|security\s+code|passcode|one-time\s+password|otp|pin|enter\s+code)[:\s*]+([A-Z0-9]{4,8})\b`)
	otpNumeric6Regex = regexp.MustCompile(`\b\d{6}\b`)
	otpNumeric4Regex = regexp.MustCompile(`\b\d{4}\b`)
	otpAlphaNumRegex = regexp.MustCompile(`\b[A-Z0-9]{6,8}\b`)
	linkRegex        = regexp.MustCompile(`https?://[^\s"'<>]+(?:verify|confirm|activate|token|code|login)[^\s"'<>]*`)
)

// VerificationPayload holds the extracted OTP code and/or magic link from an employer email.
type VerificationPayload struct {
	Email        string    `json:"email"`
	Domain       string    `json:"domain"`
	Code         string    `json:"code,omitempty"`
	MagicLink    string    `json:"magic_link,omitempty"`
	Subject      string    `json:"subject"`
	ReceivedAt   time.Time `json:"received_at"`
}

// Registry coordinates real-time waiting between browser automation workers and incoming email webhooks/IMAP.
type Registry struct {
	mu       sync.RWMutex
	pending  map[string]chan *VerificationPayload
	history  map[string]*VerificationPayload
}

var (
	defaultRegistry     *Registry
	defaultRegistryOnce sync.Once
)

// GetRegistry returns the singleton verification registry.
func GetRegistry() *Registry {
	defaultRegistryOnce.Do(func() {
		defaultRegistry = &Registry{
			pending: make(map[string]chan *VerificationPayload),
			history: make(map[string]*VerificationPayload),
		}
	})
	return defaultRegistry
}

// Key builds the routing key for a pending email verification (e.g. "bhandarenakul11@gmail.com:myworkdayjobs.com").
func (r *Registry) key(email, domain string) string {
	e := strings.ToLower(strings.TrimSpace(email))
	d := strings.ToLower(strings.TrimSpace(domain))
	return fmt.Sprintf("%s:%s", e, d)
}

// RegisterExpectation prepares a listener channel for an upcoming verification email.
func (r *Registry) RegisterExpectation(email, domain string) <-chan *VerificationPayload {
	r.mu.Lock()
	defer r.mu.Unlock()

	k := r.key(email, domain)
	ch := make(chan *VerificationPayload, 1)

	// Check if already in recent history (arrived within last 60 seconds)
	if payload, ok := r.history[k]; ok && time.Since(payload.ReceivedAt) < 60*time.Second {
		ch <- payload
		return ch
	}

	r.pending[k] = ch
	return ch
}

// ClearExpectation removes an expectation once fulfilled or timed out.
func (r *Registry) ClearExpectation(email, domain string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	k := r.key(email, domain)
	delete(r.pending, k)
}

// DeliverIncoming processes an incoming email body and delivers to waiting workers if matched.
func (r *Registry) DeliverIncoming(recipientEmail, senderDomain, subject, body string) *VerificationPayload {
	r.mu.Lock()
	defer r.mu.Unlock()

	var code string
	if m := otpContextRegex.FindStringSubmatch(body); len(m) > 1 {
		code = m[1]
	} else if m := otpContextRegex.FindStringSubmatch(subject); len(m) > 1 {
		code = m[1]
	} else if c := otpNumeric6Regex.FindString(body); c != "" {
		code = c
	} else if c := otpNumeric6Regex.FindString(subject); c != "" {
		code = c
	} else if c := otpNumeric4Regex.FindString(body); c != "" {
		code = c
	} else if c := otpAlphaNumRegex.FindString(body); c != "" {
		code = c
	}

	link := linkRegex.FindString(body)

	payload := &VerificationPayload{
		Email:      recipientEmail,
		Domain:     senderDomain,
		Code:       code,
		MagicLink:  link,
		Subject:    subject,
		ReceivedAt: time.Now().UTC(),
	}

	// Try matching exact key and domain substring
	for k, ch := range r.pending {
		parts := strings.Split(k, ":")
		if len(parts) == 2 {
			pendEmail := parts[0]
			pendDomain := parts[1]

			if strings.EqualFold(pendEmail, recipientEmail) &&
				(strings.Contains(strings.ToLower(senderDomain), pendDomain) ||
					strings.Contains(strings.ToLower(body), pendDomain) ||
					pendDomain == "*") {
				select {
				case ch <- payload:
				default:
				}
				delete(r.pending, k)
				r.history[k] = payload
				return payload
			}
		}
	}

	// Store in history
	k := r.key(recipientEmail, senderDomain)
	r.history[k] = payload
	return payload
}

// WaitForOTP blocks until an OTP arrives or context expires.
func (r *Registry) WaitForOTP(ctx context.Context, email, domain string) (*VerificationPayload, error) {
	ch := r.RegisterExpectation(email, domain)
	defer r.ClearExpectation(email, domain)

	select {
	case payload := <-ch:
		return payload, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("timeout waiting for email verification for %s from %s: %w", email, domain, ctx.Err())
	}
}
