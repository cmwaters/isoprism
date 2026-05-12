package ai

import (
	"strings"
	"testing"
)

func TestBuildPRAnalysisPromptIncludesOtherFilesAsContextOnly(t *testing.T) {
	prompt := BuildPRAnalysisPrompt(PRAnalysisInput{
		Title:       "Fix checkout handling",
		Description: "Repairs webhook handling.",
		Changes: []PRChangeInput{{
			FullName:   "api.HandleCheckout",
			FilePath:   "api/checkout.go",
			ChangeType: "modified",
			DiffHunk:   "+validateCheckout()",
		}},
		TestChanges: []PRTestInput{{
			Name:     "api.TestHandleCheckout",
			FilePath: "api/checkout_test.go",
			DiffHunk: "+assertEmailSent()",
		}},
		OtherFiles: []PROtherFileInput{{
			Path:     "docs/checkout.md",
			Status:   "modified",
			DiffHunk: "+document retry behavior",
		}},
	})

	for _, want := range []string{
		"Fix checkout handling",
		"api.HandleCheckout",
		"api.TestHandleCheckout",
		"docs/checkout.md",
		"Do not produce per-file summaries",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "other_changes") {
		t.Fatalf("prompt should not ask for structured other_changes output:\n%s", prompt)
	}
}

func TestParsePRAnalysisOutputValidatesShape(t *testing.T) {
	raw := "```json\n{\"changes\":[{\"full_name\":\"api.Handle\",\"change_summary\":\"Validates the checkout before sending mail.\"}],\"test_assertions\":[{\"name\":\"api.TestHandle\",\"assertion_summary\":\"Asserts checkout mail is sent.\"}],\"pr_summary\":\"This updates checkout handling and its tests.\",\"risk_score\":4}\n```"

	out, err := ParsePRAnalysisOutput(raw)
	if err != nil {
		t.Fatalf("ParsePRAnalysisOutput returned error: %v", err)
	}
	if out.RiskScore != 4 || out.PRSummary == "" {
		t.Fatalf("unexpected output: %+v", out)
	}
	if got := out.ChangeSummariesByFullName()["api.Handle"]; got == "" {
		t.Fatalf("missing change summary map entry")
	}
	if got := out.TestAssertionsByName()["api.TestHandle"]; got == "" {
		t.Fatalf("missing test assertion map entry")
	}
}

func TestParsePRAnalysisOutputRejectsInvalidRiskScore(t *testing.T) {
	_, err := ParsePRAnalysisOutput(`{"changes":[],"test_assertions":[],"pr_summary":"Summary.","risk_score":11}`)
	if err == nil {
		t.Fatal("expected invalid risk score error")
	}
}

func TestParsePRAnalysisOutputRejectsMissingSummary(t *testing.T) {
	_, err := ParsePRAnalysisOutput(`{"changes":[],"test_assertions":[],"risk_score":5}`)
	if err == nil {
		t.Fatal("expected missing summary error")
	}
}
