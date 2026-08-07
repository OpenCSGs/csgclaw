package api

import (
	"net/http/httptest"
	"testing"
)

func TestHubTemplateIDPathValueRestoresNamespaceSeparator(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/hub/templates/Agentic~s~resume-scorer", nil)
	req.SetPathValue("id", "Agentic~s~resume-scorer")
	if got, want := hubTemplateIDPathValue(req), "Agentic/resume-scorer"; got != want {
		t.Fatalf("hubTemplateIDPathValue() = %q, want %q", got, want)
	}
}

func TestHubTemplateIDPathValuePreservesLiteralTildes(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/hub/templates/team~~one~s~resume~~scorer", nil)
	req.SetPathValue("id", "team~~one~s~resume~~scorer")
	if got, want := hubTemplateIDPathValue(req), "team~one/resume~scorer"; got != want {
		t.Fatalf("hubTemplateIDPathValue() = %q, want %q", got, want)
	}
}
