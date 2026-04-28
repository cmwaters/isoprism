package github

import "testing"

func TestParseInstallationReposPayloadCapturesDefaultBranch(t *testing.T) {
	payload := []byte(`{
		"action": "added",
		"installation": { "id": 123 },
		"repositories_added": [{
			"id": 456,
			"full_name": "cmwaters/celestia-core",
			"name": "celestia-core",
			"default_branch": "v0.34.x-celestia"
		}]
	}`)

	parsed, err := ParseInstallationReposPayload(payload)
	if err != nil {
		t.Fatalf("ParseInstallationReposPayload returned error: %v", err)
	}
	if got := parsed.RepositoriesAdded[0].DefaultBranch; got != "v0.34.x-celestia" {
		t.Fatalf("DefaultBranch = %q, want %q", got, "v0.34.x-celestia")
	}
}
