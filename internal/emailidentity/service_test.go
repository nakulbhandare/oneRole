package emailidentity

import (
	"context"
	"testing"
)

func TestEmailIdentityGenerationAndResolution(t *testing.T) {
	svc := NewService(nil) // nil DB uses deterministic in-memory logic

	userID := "user-nakul-948210"
	tenantID := "00000000-0000-4000-8000-000000000001"
	firstName := "Nakul"
	lastName := "Bhandare"
	personalEmail := "bhandarenakul11@gmail.com"

	ident, err := svc.GetOrCreateIdentity(context.Background(), userID, tenantID, firstName, lastName, personalEmail)
	if err != nil {
		t.Fatalf("GetOrCreateIdentity failed: %v", err)
	}

	if ident.AssignedEmail == "" {
		t.Fatal("expected non-empty assigned email")
	}
	if !ident.ForwardingEnabled {
		t.Fatal("expected forwarding to be enabled")
	}
	if ident.PersonalForwardingEmail != personalEmail {
		t.Fatalf("expected personal email %q, got %q", personalEmail, ident.PersonalForwardingEmail)
	}

	// Test ResolveRecipient
	resolved, err := svc.ResolveRecipient(context.Background(), ident.AssignedEmail)
	if err != nil {
		t.Fatalf("ResolveRecipient failed: %v", err)
	}
	if resolved.PersonalForwardingEmail != personalEmail {
		t.Fatalf("expected resolved personal email %q, got %q", personalEmail, resolved.PersonalForwardingEmail)
	}
}

func TestSanitizePart(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"Nakul", "nakul"},
		{"John Doe", "johndoe"},
		{"O'Connor", "oconnor"},
		{"!!!", "user"},
		{"VeryLongNameExceedingTwentyCharacters", "verylongnameexceedin"},
	}

	for _, c := range cases {
		got := SanitizePart(c.input)
		if got != c.expected {
			t.Errorf("SanitizePart(%q) = %q, expected %q", c.input, got, c.expected)
		}
	}
}
