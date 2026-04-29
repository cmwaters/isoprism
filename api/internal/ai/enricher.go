// Package ai provides Claude-based enrichment for code nodes.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const anthropicAPI = "https://api.anthropic.com/v1/messages"
const model = "claude-sonnet-4-6"

// NodeInput is a code node to be summarised.
type NodeInput struct {
	FullName string
	Body     string
	DiffHunk string // empty for base nodes; set for PR delta nodes
}

// NodeOutput holds the AI-generated summaries.
type NodeOutput struct {
	FullName      string
	Summary       string // what the function does (2 sentences)
	ChangeSummary string // what changed (2 sentences); empty for unchanged nodes
}

// PROutput holds the PR-level summary and risk assessment.
type PROutput struct {
	Summary   string
	RiskScore int    // 1–10
	RiskLabel string // low | medium | high
}

type Enricher struct {
	APIKey string
	client *http.Client
}

func NewEnricher(apiKey string) *Enricher {
	return &Enricher{
		APIKey: apiKey,
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

// EnrichNodes generates summaries for a set of base code nodes.
// Returns a map from full_name → summary.
func (e *Enricher) EnrichNodes(ctx context.Context, nodes []NodeInput) (map[string]string, error) {
	if len(nodes) == 0 || e.APIKey == "" {
		return map[string]string{}, nil
	}

	var sb strings.Builder
	sb.WriteString("For each function below, write exactly 2 plain-English sentences describing what the function does. Be specific and concise. Return a JSON array: [{\"full_name\": \"...\", \"summary\": \"...\"}]\n\nFunctions:\n\n")
	for _, n := range nodes {
		sb.WriteString(fmt.Sprintf("--- %s ---\n%s\n\n", n.FullName, n.Body))
	}

	resp, err := e.call(ctx, sb.String(), 4096)
	if err != nil {
		return nil, err
	}

	var results []struct {
		FullName string `json:"full_name"`
		Summary  string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(extractJSON(resp)), &results); err != nil {
		return map[string]string{}, nil
	}

	out := make(map[string]string, len(results))
	for _, r := range results {
		out[r.FullName] = r.Summary
	}
	return out, nil
}

// EnrichPRChanges generates change summaries and a PR-level analysis.
func (e *Enricher) EnrichPRChanges(ctx context.Context, nodes []NodeInput) (map[string]string, PROutput, error) {
	if len(nodes) == 0 || e.APIKey == "" {
		return map[string]string{}, PROutput{}, nil
	}

	var sb strings.Builder
	sb.WriteString(`You are analysing a pull request. For each changed function, write exactly 2 sentences describing what specifically changed (not what the function does, but what changed in this PR). Return a JSON object:
{
  "changes": [{"full_name": "...", "change_summary": "..."}],
  "pr_summary": "one sentence describing the overall PR",
  "risk_score": 5,
  "risk_label": "medium"
}
risk_score is 1-10; risk_label is "low" (1-3), "medium" (4-6), or "high" (7-10).

Changed functions and their diffs:

`)
	for _, n := range nodes {
		sb.WriteString(fmt.Sprintf("--- %s ---\nDiff:\n%s\n\n", n.FullName, n.DiffHunk))
	}

	resp, err := e.call(ctx, sb.String(), 4096)
	if err != nil {
		return nil, PROutput{}, err
	}

	var result struct {
		Changes []struct {
			FullName      string `json:"full_name"`
			ChangeSummary string `json:"change_summary"`
		} `json:"changes"`
		PRSummary string `json:"pr_summary"`
		RiskScore int    `json:"risk_score"`
		RiskLabel string `json:"risk_label"`
	}
	if err := json.Unmarshal([]byte(extractJSON(resp)), &result); err != nil {
		return map[string]string{}, PROutput{Summary: "PR analysis unavailable."}, nil
	}

	changes := make(map[string]string, len(result.Changes))
	for _, c := range result.Changes {
		changes[c.FullName] = c.ChangeSummary
	}

	pr := PROutput{
		Summary:   result.PRSummary,
		RiskScore: result.RiskScore,
		RiskLabel: result.RiskLabel,
	}
	if pr.RiskLabel == "" {
		pr.RiskLabel = "medium"
	}
	if pr.RiskScore == 0 {
		pr.RiskScore = 5
	}

	return changes, pr, nil
}

// call sends a single-turn message to Claude and returns the text response.
func (e *Enricher) call(ctx context.Context, prompt string, maxTokens int) (string, error) {
	body := map[string]interface{}{
		"model":      model,
		"max_tokens": maxTokens,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	data, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", anthropicAPI, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", e.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := e.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("anthropic API error: status %d", resp.StatusCode)
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	for _, c := range result.Content {
		if c.Type == "text" {
			return c.Text, nil
		}
	}
	return "", fmt.Errorf("no text content in response")
}

// extractJSON finds the first JSON array or object in the text.
func extractJSON(s string) string {
	for _, open := range []string{"[", "{"} {
		idx := strings.Index(s, open)
		if idx == -1 {
			continue
		}
		close := "]"
		if open == "{" {
			close = "}"
		}
		depth := 0
		for i := idx; i < len(s); i++ {
			switch string(s[i]) {
			case open:
				depth++
			case close:
				depth--
				if depth == 0 {
					return s[idx : i+1]
				}
			}
		}
	}
	return s
}
