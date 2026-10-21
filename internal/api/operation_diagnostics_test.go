package api

import (
	"csgclaw/internal/modelprovider"
	"fmt"
	"strings"
	"testing"
)

func TestAgentOperationDiagnosticAttrs(t *testing.T) {
	err := fmt.Errorf("validate provider: %w", &modelprovider.ResponsesAPIStatusError{
		Operation: "responses", BaseURL: "https://user:private-password@gateway.example/v1?api_key=private-key#private-fragment",
		StatusCode: 404, Body: `{"error":{"code":"not_found","message":"private-body"}}`,
	})
	values := map[string]string{}
	for _, attr := range agentOperationDiagnosticAttrs(err) {
		values[attr.Key] = fmt.Sprint(attr.Value.Any())
	}
	for key, want := range map[string]string{
		"upstream_operation": "responses", "upstream_url": "https://gateway.example/v1",
		"upstream_status": "404", "upstream_code": "not_found",
	} {
		if values[key] != want {
			t.Errorf("%s = %q, want %q", key, values[key], want)
		}
	}
	if strings.Contains(fmt.Sprint(values), "private") {
		t.Fatalf("diagnostics contain private data: %v", values)
	}
}

func TestDiagnosticEndpointRejectsMalformedURL(t *testing.T) {
	if got := diagnosticEndpoint("https://example.com/%zz?token=private"); got != "[invalid endpoint]" {
		t.Fatalf("endpoint = %q", got)
	}
	attrs := agentOperationDiagnosticAttrs(&modelprovider.UpstreamRequestError{
		Operation: "request responses", BaseURL: "https://example.com/v1?token=private", Err: fmt.Errorf("private transport error"),
	})
	if strings.Contains(fmt.Sprint(attrs), "private") {
		t.Fatalf("diagnostics contain private data: %v", attrs)
	}
}
