package email_otp

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	otpContextRegex  = regexp.MustCompile(`(?i)(?:verification\s+code|security\s+code|passcode|one-time\s+password|otp|pin|enter\s+code)(?:\s+(?:is|field))?[:\s*]+([A-Za-z0-9]{4,10})\b`)
	otpNumeric6Regex = regexp.MustCompile(`\b\d{6}\b`)
	otpNumeric4Regex = regexp.MustCompile(`\b\d{4}\b`)
	otpMixedCode     = regexp.MustCompile(`\b(?:[A-Za-z0-9]*\d[A-Za-z0-9]*)\b`)
	htmlTagRegex     = regexp.MustCompile(`<[^>]+>`)
	linkRegex        = regexp.MustCompile(`https?://[^\s"'<>]+(?:verify|confirm|activate|token|code|login)[^\s"'<>]*`)
)

func extractOTPCode(subject, rawBody string) string {
	cleanBody := htmlTagRegex.ReplaceAllString(rawBody, " ")
	cleanBody = strings.Join(strings.Fields(cleanBody), " ")

	stopwords := map[string]bool{
		"field": true, "code": true, "below": true, "above": true, "here": true,
		"into": true, "form": true, "your": true, "this": true, "that": true,
		"application": true, "email": true, "page": true, "greenhouse": true,
	}
	isValidCode := func(c string) bool {
		lc := strings.ToLower(strings.TrimSpace(c))
		if len(lc) < 4 || len(lc) > 10 || stopwords[lc] {
			return false
		}
		// Must contain at least one digit OR be mixed uppercase+lowercase 6-8 chars (like Greenhouse's 8-char tokens)
		hasDigit, hasUpper, hasLower := false, false, false
		for _, r := range c {
			if r >= '0' && r <= '9' {
				hasDigit = true
			} else if r >= 'A' && r <= 'Z' {
				hasUpper = true
			} else if r >= 'a' && r <= 'z' {
				hasLower = true
			}
		}
		return hasDigit || (hasUpper && hasLower && len(c) >= 6)
	}

	for _, text := range []string{cleanBody, subject} {
		for _, m := range otpContextRegex.FindAllStringSubmatch(text, -1) {
			if len(m) > 1 && isValidCode(m[1]) {
				return m[1]
			}
		}
	}
	// Greenhouse specific: "security code" followed shortly by an 8-char alphanumeric code
	if idx := strings.Index(strings.ToLower(cleanBody), "security code"); idx != -1 {
		window := cleanBody[idx:]
		if len(window) > 300 {
			window = window[:300]
		}
		for _, tok := range strings.Fields(window) {
			tok = strings.Trim(tok, ".,:;!?\"'()")
			if len(tok) == 8 && isValidCode(tok) {
				return tok
			}
		}
	}
	if c := otpNumeric6Regex.FindString(cleanBody); c != "" {
		return c
	}
	if c := otpNumeric6Regex.FindString(subject); c != "" {
		return c
	}
	if c := otpNumeric4Regex.FindString(cleanBody); c != "" {
		return c
	}
	return ""
}

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
	mu      sync.RWMutex
	pending map[string]chan *VerificationPayload
	history map[string]*VerificationPayload
	db      *mongo.Database
	nc      *nats.Conn
}

// SetDB attaches a MongoDB database so OTPs are shared across all API replicas.
func (r *Registry) SetDB(db *mongo.Database) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.db = db
}

// SetNATS attaches a NATS connection to push incoming OTPs instantly to Chromium workers.
func (r *Registry) SetNATS(nc *nats.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nc = nc
}

func domainBrandMatch(candidateDomain, subject, expectedDomain string) bool {
	if expectedDomain == "" || expectedDomain == "*" {
		return true
	}
	exp := strings.ToLower(strings.TrimSpace(expectedDomain))
	cand := strings.ToLower(strings.TrimSpace(candidateDomain))
	if strings.Contains(cand, exp) || strings.Contains(strings.ToLower(subject), exp) {
		return true
	}
	brand := strings.Split(exp, ".")[0]
	return len(brand) >= 3 && (strings.Contains(cand, brand) || strings.Contains(strings.ToLower(subject), brand))
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

	code := extractOTPCode(subject, body)

	link := linkRegex.FindString(body)

	payload := &VerificationPayload{
		Email:      recipientEmail,
		Domain:     senderDomain,
		Code:       code,
		MagicLink:  link,
		Subject:    subject,
		ReceivedAt: time.Now().UTC(),
	}

	// Instant NATS push to all listening Chromium workers (< 1ms delivery, zero polling)
	if r.nc != nil && (payload.Code != "" || payload.MagicLink != "") {
		if b, mErr := json.Marshal(payload); mErr == nil {
			_ = r.nc.Publish("email.otp.push", b)
		}
	}

	// Persist to MongoDB for cross-replica coordination
	if r.db != nil && (payload.Code != "" || payload.MagicLink != "") {
		dbCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_, _ = r.db.Collection("email_otp_challenges").InsertOne(dbCtx, bson.M{
			"recipient_email": strings.ToLower(strings.TrimSpace(recipientEmail)),
			"employer_domain": strings.ToLower(strings.TrimSpace(senderDomain)),
			"code":            payload.Code,
			"magic_link":      payload.MagicLink,
			"subject":         subject,
			"received_at":     payload.ReceivedAt,
			"expires_at":      payload.ReceivedAt.Add(10 * time.Minute),
			"consumed":        false,
		})
		cancel()
	}

	// Try matching exact key and domain/brand substring
	for k, ch := range r.pending {
		parts := strings.Split(k, ":")
		if len(parts) == 2 {
			pendEmail := parts[0]
			pendDomain := parts[1]

			if strings.EqualFold(pendEmail, recipientEmail) &&
				(domainBrandMatch(senderDomain, subject, pendDomain) ||
					strings.Contains(strings.ToLower(body), pendDomain)) {
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

// WaitForOTP blocks until an OTP arrives (in-memory or via MongoDB across replicas) or context expires.
func (r *Registry) WaitForOTP(ctx context.Context, email, domain string, company ...string) (*VerificationPayload, error) {
	compFilter := ""
	if len(company) > 0 {
		compFilter = strings.ToLower(strings.TrimSpace(company[0]))
	}
	ch := r.RegisterExpectation(email, domain)
	defer r.ClearExpectation(email, domain)

	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()

	checkDB := func() *VerificationPayload {
		if db == nil {
			return nil
		}
		qCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		since := time.Now().UTC().Add(-2 * time.Minute)
		filter := bson.M{
			"recipient_email": strings.ToLower(strings.TrimSpace(email)),
			"received_at":     bson.M{"$gte": since},
			"consumed":        bson.M{"$ne": true},
		}
		cur, err := db.Collection("email_otp_challenges").Find(qCtx, filter, options.Find().SetSort(bson.D{{Key: "received_at", Value: -1}}).SetLimit(15))
		if err != nil {
			return nil
		}
		defer cur.Close(qCtx)
		for cur.Next(qCtx) {
			var doc struct {
				ID         any       `bson:"_id"`
				Email      string    `bson:"recipient_email"`
				Domain     string    `bson:"employer_domain"`
				Code       string    `bson:"code"`
				MagicLink  string    `bson:"magic_link"`
				Subject    string    `bson:"subject"`
				ReceivedAt time.Time `bson:"received_at"`
			}
			if cur.Decode(&doc) == nil && domainBrandMatch(doc.Domain, doc.Subject, domain) && (doc.Code != "" || doc.MagicLink != "") {
				if compFilter != "" && !strings.Contains(strings.ToLower(doc.Subject), compFilter) {
					continue
				}
				_, _ = db.Collection("email_otp_challenges").UpdateOne(qCtx, bson.M{"_id": doc.ID}, bson.M{"$set": bson.M{"consumed": true}})
				return &VerificationPayload{
					Email:      doc.Email,
					Domain:     doc.Domain,
					Code:       doc.Code,
					MagicLink:  doc.MagicLink,
					Subject:    doc.Subject,
					ReceivedAt: doc.ReceivedAt,
				}
			}
		}
		return nil
	}

	if p := checkDB(); p != nil {
		return p, nil
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case payload := <-ch:
			return payload, nil
		case <-ticker.C:
			if p := checkDB(); p != nil {
				return p, nil
			}
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting for email verification for %s from %s: %w", email, domain, ctx.Err())
		}
	}
}
