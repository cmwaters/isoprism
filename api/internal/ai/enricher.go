// Package ai provides Gemini-based enrichment for pull request changes.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultModel  = "gemini-2.5-flash"
	PromptVersion = "pr-analysis-v1"

	geminiEndpointFormat = "https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s"
	maxOutputTokens      = 4096
	maxPromptDiffChars   = 12000
)

// PRChangeInput is one changed production component sent to the model.
type PRChangeInput struct {
	FullName   string
	FilePath   string
	ChangeType string
	DiffHunk   string
}

// PRTestInput is one changed test component sent to the model.
type PRTestInput struct {
	Name     string
	FilePath string
	DiffHunk string
}

// PROtherFileInput is a non-code file diff sent as PR-level context.
type PROtherFileInput struct {
	Path     string
	Status   string
	DiffHunk string
}

// PRAnalysisInput contains all context for one PR-level analysis call.
type PRAnalysisInput struct {
	Title       string
	Description string
	Changes     []PRChangeInput
	TestChanges []PRTestInput
	OtherFiles  []PROtherFileInput
}

type PRChangeOutput struct {
	FullName      string `json:"full_name"`
	ChangeSummary string `json:"change_summary"`
}

type PRTestAssertionOutput struct {
	Name             string `json:"name"`
	AssertionSummary string `json:"assertion_summary"`
}

// PRAnalysisOutput is the strict JSON object expected from Gemini.
type PRAnalysisOutput struct {
	Changes        []PRChangeOutput        `json:"changes"`
	TestAssertions []PRTestAssertionOutput `json:"test_assertions"`
	PRSummary      string                  `json:"pr_summary"`
	RiskScore      int                     `json:"risk_score"`
}

type Enricher struct {
	APIKey string
	Model  string
	client *http.Client
}

func NewEnricher(apiKey string) *Enricher {
	return NewEnricherWithModel(apiKey, DefaultModel)
}

func NewEnricherWithModel(apiKey, model string) *Enricher {
	model = strings.TrimSpace(model)
	if model == "" {
		model = DefaultModel
	}
	return &Enricher{
		APIKey: apiKey,
		Model:  model,
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

func (e *Enricher) HasAPIKey() bool {
	return e != nil && strings.TrimSpace(e.APIKey) != ""
}

func (e *Enricher) EnrichPRChanges(ctx context.Context, input PRAnalysisInput) (PRAnalysisOutput, error) {
	if !e.HasAPIKey() {
		return PRAnalysisOutput{}, nil
	}

	prompt := BuildPRAnalysisPrompt(input)
	if strings.TrimSpace(prompt) == "" {
		return PRAnalysisOutput{}, nil
	}

	start := time.Now()
	var respText string
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		respText, err = e.call(ctx, prompt)
		if err == nil {
			break
		}
		if attempt == 2 || !isTransientProviderError(err) {
			break
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return PRAnalysisOutput{}, ctx.Err()
		case <-timer.C:
		}
	}
	if err != nil {
		return PRAnalysisOutput{}, err
	}

	out, err := ParsePRAnalysisOutput(respText)
	log.Printf(
		"AI PR analysis: task=pr_analysis model=%s changes=%d tests=%d other_files=%d latency_ms=%d parse_ok=%t",
		e.Model,
		len(input.Changes),
		len(input.TestChanges),
		len(input.OtherFiles),
		time.Since(start).Milliseconds(),
		err == nil,
	)
	if err != nil {
		return PRAnalysisOutput{}, err
	}
	return out, nil
}

func BuildPRAnalysisPrompt(input PRAnalysisInput) string {
	if len(input.Changes) == 0 && len(input.TestChanges) == 0 && len(input.OtherFiles) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(`You are analysing a pull request. For each changed production function, write up to 2 sentences describing what specifically changed in this PR. Do not describe what the function generally does unless that is necessary to explain the change.

For tests, describe what behavior is being asserted. Do not summarize test implementation mechanics unless they are important to the assertion.

Use documentation, config, migrations, generated contracts, dependency manifests, build files, or deployment files as context for the PR summary and risk score. Do not produce per-file summaries for these other changes.

Use the PR title and description as intent/context, but ground your output in the diffs. If the PR description conflicts with the diff, trust the diff.

Return only a JSON object with this shape:
{
  "changes": [{"full_name": "...", "change_summary": "..."}],
  "test_assertions": [{"name": "...", "assertion_summary": "..."}],
  "pr_summary": "two to three sentence describing the overall PR with an emphasis on what this changes and why this change is necessary",
  "risk_score": 5
}

risk_score is an integer from 1 to 10.

PR title:
`)
	sb.WriteString(strings.TrimSpace(input.Title))
	sb.WriteString("\n\nPR description:\n")
	if strings.TrimSpace(input.Description) == "" {
		sb.WriteString("(none)")
	} else {
		sb.WriteString(strings.TrimSpace(input.Description))
	}
	sb.WriteString("\n\nChanged functions and their diffs:\n\n")
	for _, change := range input.Changes {
		sb.WriteString(fmt.Sprintf("--- %s ---\nFile: %s\nChange type: %s\nDiff:\n%s\n\n",
			change.FullName,
			change.FilePath,
			change.ChangeType,
			truncateDiff(change.DiffHunk),
		))
	}

	sb.WriteString("Test changes:\n\n")
	for _, test := range input.TestChanges {
		name := test.Name
		if name == "" {
			name = test.FilePath
		}
		sb.WriteString(fmt.Sprintf("--- %s ---\nFile: %s\nDiff:\n%s\n\n",
			name,
			test.FilePath,
			truncateDiff(test.DiffHunk),
		))
	}

	sb.WriteString("Other changed files:\n\n")
	for _, file := range input.OtherFiles {
		sb.WriteString(fmt.Sprintf("--- %s ---\nStatus: %s\nDiff:\n%s\n\n",
			file.Path,
			file.Status,
			truncateDiff(file.DiffHunk),
		))
	}

	return sb.String()
}

func ParsePRAnalysisOutput(raw string) (PRAnalysisOutput, error) {
	var out PRAnalysisOutput
	if err := json.Unmarshal([]byte(extractJSON(raw)), &out); err != nil {
		return PRAnalysisOutput{}, fmt.Errorf("parse PR analysis JSON: %w", err)
	}
	if err := ValidatePRAnalysisOutput(out); err != nil {
		return PRAnalysisOutput{}, err
	}
	return out, nil
}

func ValidatePRAnalysisOutput(out PRAnalysisOutput) error {
	if strings.TrimSpace(out.PRSummary) == "" {
		return fmt.Errorf("pr_summary is required")
	}
	if out.RiskScore < 1 || out.RiskScore > 10 {
		return fmt.Errorf("risk_score must be between 1 and 10")
	}
	for i, change := range out.Changes {
		if strings.TrimSpace(change.FullName) == "" {
			return fmt.Errorf("changes[%d].full_name is required", i)
		}
		if strings.TrimSpace(change.ChangeSummary) == "" {
			return fmt.Errorf("changes[%d].change_summary is required", i)
		}
	}
	for i, assertion := range out.TestAssertions {
		if strings.TrimSpace(assertion.Name) == "" {
			return fmt.Errorf("test_assertions[%d].name is required", i)
		}
		if strings.TrimSpace(assertion.AssertionSummary) == "" {
			return fmt.Errorf("test_assertions[%d].assertion_summary is required", i)
		}
	}
	return nil
}

func (out PRAnalysisOutput) ChangeSummariesByFullName() map[string]string {
	summaries := make(map[string]string, len(out.Changes))
	for _, change := range out.Changes {
		summaries[change.FullName] = change.ChangeSummary
	}
	return summaries
}

func (out PRAnalysisOutput) TestAssertionsByName() map[string]string {
	summaries := make(map[string]string, len(out.TestAssertions))
	for _, assertion := range out.TestAssertions {
		summaries[assertion.Name] = assertion.AssertionSummary
	}
	return summaries
}

func (e *Enricher) call(ctx context.Context, prompt string) (string, error) {
	body := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"role": "user",
				"parts": []map[string]string{
					{"text": prompt},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"responseMimeType": "application/json",
			"maxOutputTokens":  maxOutputTokens,
		},
	}
	data, _ := json.Marshal(body)

	endpoint := fmt.Sprintf(geminiEndpointFormat, url.PathEscape(e.Model), url.QueryEscape(e.APIKey))
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
		return "", transientProviderError{status: resp.StatusCode}
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("gemini API error: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	for _, candidate := range result.Candidates {
		for _, part := range candidate.Content.Parts {
			if strings.TrimSpace(part.Text) != "" {
				return part.Text, nil
			}
		}
	}
	return "", fmt.Errorf("no text content in Gemini response")
}

type transientProviderError struct {
	status int
}

func (e transientProviderError) Error() string {
	return fmt.Sprintf("gemini API transient error: status %d", e.status)
}

func isTransientProviderError(err error) bool {
	_, ok := err.(transientProviderError)
	return ok
}

func truncateDiff(diff string) string {
	if len(diff) <= maxPromptDiffChars {
		return diff
	}
	return diff[:maxPromptDiffChars] + "\n...[truncated]"
}

// extractJSON finds the first JSON object in the text.
func extractJSON(s string) string {
	idx := strings.Index(s, "{")
	if idx == -1 {
		return s
	}
	depth := 0
	for i := idx; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[idx : i+1]
			}
		}
	}
	return s
}
