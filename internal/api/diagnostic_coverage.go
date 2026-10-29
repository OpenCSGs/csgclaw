package api

import (
	"encoding/json"
	"fmt"

	"csgclaw/internal/diagnostics"
)

type diagnosticCoverage struct {
	TurnID   string   `json:"turn_id,omitempty"`
	Section  string   `json:"section"`
	Status   string   `json:"status"`
	Expected int      `json:"expected,omitempty"`
	Observed int      `json:"observed,omitempty"`
	Reasons  []string `json:"reasons"`
}
type exchangeEvidence struct{ response, end, failed bool }
type turnEvidence struct {
	kinds             map[string]int
	runtime           map[string]*exchangeEvidence
	upstream          map[string]*exchangeEvidence
	ingressFailures   int
	httpRequests      int
	malformedIdentity bool
	filesExpected     int
	performanceModes  map[string]int
}

func newTurnEvidence() *turnEvidence {
	return &turnEvidence{kinds: map[string]int{}, runtime: map[string]*exchangeEvidence{}, upstream: map[string]*exchangeEvidence{}, performanceModes: map[string]int{}}
}
func evidenceExchange(items map[string]*exchangeEvidence, id string) *exchangeEvidence {
	if items[id] == nil {
		items[id] = &exchangeEvidence{}
	}
	return items[id]
}
func (e *turnEvidence) Observe(entry diagnostics.ContextEntry) {
	e.kinds[entry.Kind]++
	var item map[string]json.RawMessage
	_ = json.Unmarshal(entry.Data, &item)
	var id string
	_ = json.Unmarshal(item["request_id"], &id)
	var attempt uint64
	_ = json.Unmarshal(item["attempt"], &attempt)
	upstreamID := fmt.Sprintf("%s:%d", id, attempt)
	switch entry.Kind {
	case "llm.ingress":
		var status int
		_ = json.Unmarshal(item["http_status"], &status)
		if status >= 400 {
			e.ingressFailures++
		}
	case "model.runtime_request":
		if id == "" {
			e.malformedIdentity = true
		}
		evidenceExchange(e.runtime, id)
		var parent string
		_ = json.Unmarshal(item["ingress_request_id"], &parent)
		if parent != "" {
			e.httpRequests++
		}
	case "model.upstream_request":
		if id == "" {
			e.malformedIdentity = true
		}
		evidenceExchange(e.upstream, upstreamID)
	case "model.runtime_response":
		evidenceExchange(e.runtime, id).response = true
	case "model.upstream_response":
		evidenceExchange(e.upstream, upstreamID).response = true
	case "model.runtime_error":
		evidenceExchange(e.runtime, id).failed = true
	case "model.upstream_error":
		evidenceExchange(e.upstream, upstreamID).failed = true
	case "model.body_end":
		var stage string
		var complete bool
		_ = json.Unmarshal(item["stage"], &stage)
		_ = json.Unmarshal(item["complete"], &complete)
		switch stage {
		case "model.runtime_response":
			evidenceExchange(e.runtime, id).end = complete
		case "model.upstream_response":
			evidenceExchange(e.upstream, upstreamID).end = complete
		case "websocket":
			evidenceExchange(e.runtime, id).end = complete
			evidenceExchange(e.upstream, id+":0").end = complete
		}
	case "execution.request":
		var request struct {
			Input []struct {
				Kind string `json:"kind"`
			} `json:"input"`
		}
		_ = json.Unmarshal(entry.Data, &request)
		for _, part := range request.Input {
			if part.Kind == "file" {
				e.filesExpected++
			}
		}
	case "execution.result":
		var result struct {
			Files []json.RawMessage `json:"files"`
		}
		_ = json.Unmarshal(entry.Data, &result)
		e.filesExpected += len(result.Files)
	case "performance.operation":
		var operation struct {
			Attributes struct {
				Persistence string `json:"persistence"`
			} `json:"attributes"`
		}
		_ = json.Unmarshal(entry.Data, &operation)
		e.performanceModes[operation.Attributes.Persistence]++
	}
}
func coverageItem(turn, section string, observed int, reasons ...string) diagnosticCoverage {
	status := "complete"
	if len(reasons) > 0 {
		status = "partial"
	}
	if reasons == nil {
		reasons = []string{}
	}
	return diagnosticCoverage{TurnID: turn, Section: section, Status: status, Observed: observed, Reasons: reasons}
}
func notApplicable(turn, section, reason string) diagnosticCoverage {
	return diagnosticCoverage{TurnID: turn, Section: section, Status: "not_applicable", Reasons: []string{reason}}
}
func (e *turnEvidence) Coverage(record diagnostics.Snapshot) []diagnosticCoverage {
	turn := record.TurnID
	result := []diagnosticCoverage{}
	context := coverageItem(turn, "execution_context", e.kinds["execution.request"]+e.kinds["execution.result"])
	if record.EvidenceVersion < 2 {
		context.Status = "partial"
		context.Reasons = append(context.Reasons, "historical_capture_without_section_manifest")
	}
	if e.kinds["execution.environment"] == 0 {
		context.Status = "partial"
		context.Reasons = append(context.Reasons, "execution_environment_not_collected")
	}
	if e.kinds["execution.request"] > 0 && e.kinds["execution.result"] == 0 {
		context.Status = "partial"
		context.Reasons = append(context.Reasons, "execution_result_not_collected")
	}
	if e.kinds["execution.request"] == 0 && !(e.kinds["channel.input"] > 0 && e.kinds["channel.result"] > 0) {
		context.Status = "partial"
		context.Reasons = append(context.Reasons, "execution_input_or_channel_outcome_not_collected")
	}
	result = append(result, context)
	ingress := coverageItem(turn, "llm_ingress", e.kinds["llm.ingress"])
	if e.httpRequests > e.kinds["llm.ingress"] {
		ingress.Status = "partial"
		ingress.Reasons = append(ingress.Reasons, "http_ingress_evidence_missing")
	}
	if e.kinds["llm.ingress"] == 0 {
		if record.RuntimeStartMS != nil && len(e.runtime) == 0 {
			ingress.Status = "partial"
			ingress.Reasons = append(ingress.Reasons, "llm_ingress_not_collected")
		} else {
			ingress = notApplicable(turn, "llm_ingress", "no_observed_http_ingress")
		}
	}
	result = append(result, ingress)
	model := coverageItem(turn, "model_exchange", 0)
	model.Expected = len(e.runtime) + len(e.upstream)
	for _, items := range []map[string]*exchangeEvidence{e.runtime, e.upstream} {
		for _, item := range items {
			if item.failed || (item.response && item.end) {
				model.Observed++
			}
		}
	}
	if model.Observed < model.Expected {
		model.Status = "partial"
		model.Reasons = append(model.Reasons, "model_request_response_or_terminal_evidence_missing")
	}
	if e.malformedIdentity {
		model.Status = "partial"
		model.Reasons = append(model.Reasons, "model_request_identity_missing")
	}
	if model.Expected == 0 {
		if record.RuntimeStartMS == nil || e.ingressFailures > 0 || e.kinds["llm.failed_request"] > 0 {
			model = notApplicable(turn, "model_exchange", "execution_stopped_before_model_forwarding")
		} else {
			model.Status = "partial"
			model.Reasons = append(model.Reasons, "model_exchange_not_collected")
		}
	}
	if len(e.runtime) > 0 && len(e.upstream) == 0 && e.kinds["model.runtime_error"] == 0 {
		model.Status = "partial"
		model.Reasons = append(model.Reasons, "upstream_model_evidence_missing")
	}
	result = append(result, model)
	performance := coverageItem(turn, "performance", e.kinds["performance.operation"])
	if performance.Observed == 0 {
		performance.Status = "partial"
		performance.Reasons = append(performance.Reasons, "performance_breakdown_not_collected")
	}
	for _, name := range []string{"im.lock_wait", "im.lock_hold"} {
		if _, ok := record.Performance[name]; !ok {
			performance.Status = "partial"
			performance.Reasons = append(performance.Reasons, name+"_not_collected")
		}
	}
	if e.performanceModes["disk"] > 0 {
		for _, name := range []string{"im.snapshot", "im.serialize", "im.write", "im.fsync", "im.rename"} {
			if _, ok := record.Performance[name]; !ok {
				performance.Status = "partial"
				performance.Reasons = append(performance.Reasons, name+"_not_collected")
			}
		}
	}
	result = append(result, performance)
	files := coverageItem(turn, "execution_files", e.kinds["execution.file"])
	files.Expected = e.filesExpected
	if files.Expected == 0 {
		files = notApplicable(turn, "execution_files", "no_observed_execution_files")
	} else if files.Observed < files.Expected {
		files.Status = "partial"
		files.Reasons = append(files.Reasons, "execution_file_evidence_missing")
	}
	result = append(result, files)
	for i := range result {
		if record.Status == "running" || record.Status == "queued" {
			if result[i].Status == "partial" {
				result[i].Status = "in_progress"
			}
		}
	}
	return result
}

func uniqueBundleIssues(issues []bundleIssue) []bundleIssue {
	seen := map[bundleIssue]bool{}
	result := []bundleIssue{}
	for _, issue := range issues {
		if !seen[issue] {
			seen[issue] = true
			result = append(result, issue)
		}
	}
	return result
}
