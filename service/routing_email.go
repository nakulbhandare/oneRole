package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// DefaultMailDomain is the standard multi-tenant inbound email domain.
const DefaultMailDomain = "mail.onerole.com"

var nonAlphaNumRegex = regexp.MustCompile(`[^a-z0-9]`)

// GetMailDomain returns the configured inbound routing domain.
func GetMailDomain() string {
	d := strings.TrimSpace(os.Getenv("ONEROLE_MAIL_DOMAIN"))
	if d != "" {
		return d
	}
	return DefaultMailDomain
}

// GenerateRoutingToken creates a deterministic 8-character token for a user.
func GenerateRoutingToken(userID string) string {
	secret := os.Getenv("IDEMPOTENCY_HMAC_SECRET")
	if secret == "" {
		secret = os.Getenv("ROUTING_HMAC_SECRET")
	}
	if secret == "" {
		secret = "onerole-enterprise-salt-v1-982184"
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(userID))
	sum := mac.Sum(nil)
	return hex.EncodeToString(sum)[:8]
}

// GenerateUserRoutingEmail constructs a unique, collision-free routing email for a candidate.
// Format: <firstname>.<lastname>.<routing_token>@mail.onerole.com
func GenerateUserRoutingEmail(userID, firstName, lastName string) string {
	fn := sanitizeNamePart(firstName)
	if fn == "" {
		fn = "candidate"
	}
	ln := sanitizeNamePart(lastName)
	if ln == "" {
		ln = "app"
	}

	token := GenerateRoutingToken(userID)
	domain := GetMailDomain()

	return fmt.Sprintf("%s.%s.%s@%s", fn, ln, token, domain)
}

// ExtractRoutingToken extracts the token from a routing email address.
func ExtractRoutingToken(emailAddress string) string {
	cleaned := strings.TrimSpace(strings.ToLower(emailAddress))
	parts := strings.Split(cleaned, "@")
	if len(parts) != 2 {
		return ""
	}
	localPart := parts[0]
	segments := strings.Split(localPart, ".")
	if len(segments) >= 3 {
		return segments[len(segments)-1]
	}
	return ""
}

func sanitizeNamePart(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	clean := nonAlphaNumRegex.ReplaceAllString(lower, "")
	if len(clean) > 20 {
		clean = clean[:20]
	}
	return clean
}
