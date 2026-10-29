package api

import (
	"encoding/json"
	"testing"

	"csgclaw/internal/diagnostics"
)

func TestDiagnosticCoverageRequiresEveryModelExchangeTerminal(t *testing.T) {
	evidence := newTurnEvidence()
	observe := func(kind, body string) {
		evidence.Observe(diagnostics.ContextEntry{Kind: kind, Data: json.RawMessage(body)})
	}
	observe("model.runtime_request", `{"request_id":"one"}`)
	observe("model.upstream_request", `{"request_id":"one","attempt":1}`)
	observe("model.runtime_request", `{"request_id":"two"}`)
	observe("model.upstream_request", `{"request_id":"two","attempt":1}`)
	for _, kind := range []string{"model.runtime_response", "model.upstream_response"} {
		observe(kind, `{"request_id":"one","attempt":1}`)
	}
	for _, stage := range []string{"model.runtime_response", "model.upstream_response"} {
		observe("model.body_end", `{"request_id":"one","attempt":1,"stage":"`+stage+`","complete":true}`)
	}
	record := diagnostics.Snapshot{TurnID: "turn", EvidenceVersion: 2, RuntimeStartMS: new(float64), Status: "succeeded"}
	find := func() diagnosticCoverage {
		for _, item := range evidence.Coverage(record) {
			if item.Section == "model_exchange" {
				return item
			}
		}
		t.Fatal("no model coverage")
		return diagnosticCoverage{}
	}
	section := find()
	if section.Status != "partial" || section.Expected != 4 || section.Observed != 2 {
		t.Fatalf("incomplete second exchange overlooked: %+v", section)
	}
	observe("model.upstream_error", `{"request_id":"two","attempt":1}`)
	observe("model.runtime_error", `{"request_id":"two"}`)
	section = find()
	if section.Status != "complete" || section.Expected != section.Observed {
		t.Fatalf("observed transport errors should complete the evidence: %+v", section)
	}
}
func TestHistoricalDiagnosticCannotClaimFullNewEvidence(t *testing.T) {
	evidence := newTurnEvidence()
	evidence.Observe(diagnostics.ContextEntry{Kind: "execution.environment", Data: json.RawMessage(`{}`)})
	sections := evidence.Coverage(diagnostics.Snapshot{TurnID: "old", Status: "succeeded"})
	missing := map[string]bool{}
	for _, section := range sections {
		if section.Status == "partial" {
			missing[section.Section] = true
		}
	}
	if !missing["execution_context"] || !missing["performance"] {
		t.Fatalf("historical gaps not disclosed: %+v", sections)
	}
}
