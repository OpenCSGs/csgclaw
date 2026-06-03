package bot

import "testing"

func TestParseEnvAssignments(t *testing.T) {
	t.Parallel()

	got, err := parseEnvAssignments([]string{"GITLAB_TOKEN=secret", "GITLAB_URL=https://gitlab.example.com"})
	if err != nil {
		t.Fatalf("parseEnvAssignments() error = %v", err)
	}
	if got["GITLAB_TOKEN"] != "secret" || got["GITLAB_URL"] != "https://gitlab.example.com" {
		t.Fatalf("parseEnvAssignments() = %#v", got)
	}
}

func TestParseEnvAssignmentsRejectsInvalid(t *testing.T) {
	t.Parallel()

	if _, err := parseEnvAssignments([]string{"NOT_A_PAIR"}); err == nil {
		t.Fatal("parseEnvAssignments() error = nil, want invalid env")
	}
}

func TestParseEnvAssignmentsRejectsDuplicateKey(t *testing.T) {
	t.Parallel()

	if _, err := parseEnvAssignments([]string{"A=1", "A=2"}); err == nil {
		t.Fatal("parseEnvAssignments() error = nil, want duplicate key")
	}
}
