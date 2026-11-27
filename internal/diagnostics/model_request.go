package diagnostics

import (
	"context"
	"encoding/json"
	"time"
)

const modelRequestEvent = "llm.model_request_status"

// ModelRequestStatus carries bounded failure and recovery information to the
// transcript. Details is user-facing guidance, never raw provider or transport
// text. The upstream response is captured separately in diagnostics.
type ModelRequestStatus struct {
	State            string     `json:"state"`
	Category         string     `json:"category"`
	Source           string     `json:"source"`
	RequestID        string     `json:"request_id"`
	GatewayRequestID string     `json:"gateway_request_id,omitempty"`
	Code             string     `json:"code,omitempty"`
	HTTPStatus       int        `json:"http_status,omitempty"`
	Details          string     `json:"details,omitempty"`
	Attempt          int        `json:"attempt,omitempty"`
	MaxAttempts      int        `json:"max_attempts,omitempty"`
	RetryAt          *time.Time `json:"retry_at,omitempty"`
	At               time.Time  `json:"-"`
}

func ReportModelRequest(ctx context.Context, status ModelRequestStatus) {
	Capture(ctx, modelRequestEvent, status)
}

// ObserveModelRequest connects a live transcript to its exact diagnostic
// record. Early native events are replayed when the native turn binds.
func (r *Record) ObserveModelRequest(observer func(ModelRequestStatus)) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.modelRequestObserver = observer
	latest := r.modelRequestStatus
	r.mu.Unlock()
	if observer != nil && latest != nil {
		observer(*latest)
	}
}

func (r *Record) updateModelRequest(entry ContextEntry) {
	var status ModelRequestStatus
	if json.Unmarshal(entry.Data, &status) != nil {
		return
	}
	status.At = entry.At
	r.mu.Lock()
	if r.modelRequestStatus != nil && !r.modelRequestStatus.At.Before(status.At) {
		r.mu.Unlock()
		return
	}
	r.modelRequestStatus = &status
	observe := r.modelRequestObserver
	r.mu.Unlock()
	if observe != nil {
		observe(status)
	}
}
