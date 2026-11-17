package service

import (
	"errors"
	"testing"
	"time"
)

func TestOAuthTransactionValidateForCreate(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	valid := OAuthTransaction{
		Provider:               OAuthProviderGoogle,
		StateHash:              make([]byte, 32),
		PKCEVerifierCiphertext: []byte("encrypted-verifier"),
		OIDCNonceHash:          make([]byte, 32),
		BrowserBindingHash:     make([]byte, 32),
		FixedReturnPath:        "/auth/callback",
		CreatedAt:              now,
		ExpiresAt:              now.Add(5 * time.Minute),
	}
	if err := valid.ValidateForCreate(); err != nil {
		t.Fatalf("valid transaction rejected: %v", err)
	}

	invalid := valid
	invalid.StateHash = []byte("raw state must not be persisted")
	if err := invalid.ValidateForCreate(); !errors.Is(err, ErrInvalidOAuthTransaction) {
		t.Fatalf("expected invalid transaction error, got %v", err)
	}

	invalid = valid
	invalid.ExpiresAt = invalid.CreatedAt
	if err := invalid.ValidateForCreate(); !errors.Is(err, ErrInvalidOAuthTransaction) {
		t.Fatalf("expected invalid expiry error, got %v", err)
	}

	invalid = valid
	invalid.Provider = OAuthProviderGitHub
	if err := invalid.ValidateForCreate(); !errors.Is(err, ErrInvalidOAuthTransaction) {
		t.Fatalf("GitHub transaction must not carry an OIDC nonce: %v", err)
	}
}

func TestAuthIdentityInputRequiresProviderSubjectAndUTC(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	valid := AuthIdentityInput{
		Provider:        OAuthProviderGitHub,
		ProviderSubject: "stable-provider-subject",
		Email:           "person@example.com",
		EmailVerified:   true,
		Now:             now,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid identity rejected: %v", err)
	}

	invalid := valid
	invalid.ProviderSubject = ""
	if err := invalid.Validate(); !errors.Is(err, ErrIdentityConflict) {
		t.Fatalf("expected identity conflict for missing subject, got %v", err)
	}

	invalid = valid
	invalid.Email = ""
	if err := invalid.Validate(); !errors.Is(err, ErrIdentityConflict) {
		t.Fatalf("verified email must be present, got %v", err)
	}

	invalid = valid
	invalid.Now = time.Date(2026, 10, 30, 4, 0, 0, 0, time.FixedZone("PDT", -8*60*60))
	if err := invalid.Validate(); !errors.Is(err, ErrIdentityConflict) {
		t.Fatalf("non-UTC timestamps must be rejected, got %v", err)
	}
}

func TestAuthSessionAndRefreshRotationValidateHashesAndExpiry(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	session := AuthSession{
		ID:                "44444444-4444-4444-8444-444444444444",
		UserID:            "33333333-3333-4333-8333-333333333333",
		FamilyID:          "55555555-5555-4555-8555-555555555555",
		RefreshTokenHash:  make([]byte, 32),
		CreatedAt:         now,
		LastUsedAt:        now,
		IdleExpiresAt:     now.Add(24 * time.Hour),
		AbsoluteExpiresAt: now.Add(30 * 24 * time.Hour),
	}
	if err := session.ValidateForCreate(); err != nil {
		t.Fatalf("valid session rejected: %v", err)
	}

	rotation := RefreshRotation{
		PresentedTokenHash:   make([]byte, 32),
		ReplacementTokenHash: append([]byte(nil), make([]byte, 32)...),
		Now:                  now,
		NewIdleExpiresAt:     now.Add(24 * time.Hour),
	}
	rotation.ReplacementTokenHash[0] = 1
	if err := rotation.Validate(); err != nil {
		t.Fatalf("valid rotation rejected: %v", err)
	}

	rotation.ReplacementTokenHash = append([]byte(nil), rotation.PresentedTokenHash...)
	if err := rotation.Validate(); !errors.Is(err, ErrAuthSessionUnavailable) {
		t.Fatalf("reusing the same refresh hash must be rejected, got %v", err)
	}

	session.RefreshTokenHash = []byte("raw token must not be stored")
	if err := session.ValidateForCreate(); !errors.Is(err, ErrAuthSessionUnavailable) {
		t.Fatalf("expected invalid session hash error, got %v", err)
	}
}
