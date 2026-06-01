package handlers

import (
	"strings"
	"testing"
)

// TestPilotInviteEmailHTML verifies pilot invite email HTML.
func TestPilotInviteEmailHTML(t *testing.T) {
	html := pilotInviteEmailHTML("Euge", "https://isoprism.com/pilot/invite-token")

	wantParts := []string{
		"<p>Hey Euge,</p>",
		"Thanks for your interest in the <strong>Isoprism</strong> pilot.",
		"<p>All the best,</p><p>Callum</p>",
	}
	for _, want := range wantParts {
		if !strings.Contains(html, want) {
			t.Fatalf("invite email missing %q in %s", want, html)
		}
	}
	if strings.Contains(html, "Cheers,") {
		t.Fatalf("invite email still contains old sign-off: %s", html)
	}
}

// TestPilotReviewEmailHTML verifies pilot review email HTML.
func TestPilotReviewEmailHTML(t *testing.T) {
	html := pilotReviewEmailHTML("Euge", "https://isoprism.com/pilot/review/review-token")

	wantParts := []string{
		"<p>Hi Euge,</p>",
		"Thanks for piloting the <strong>Isoprism</strong> prototype. We'd love if you could take a moment to answer a few questions so we can learn how to best improve this product.",
		"<p>Best Regards,</p><p>Callum</p>",
	}
	for _, want := range wantParts {
		if !strings.Contains(html, want) {
			t.Fatalf("review email missing %q in %s", want, html)
		}
	}
}
