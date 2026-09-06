package api

import (
	"net/http/httptest"
	"sync"
)

// ResponseRecorder is not safe to inspect while an SSE handler writes.
type sseRecorder struct {
	*httptest.ResponseRecorder
	mu sync.Mutex
}

func newSSERecorder() *sseRecorder { return &sseRecorder{ResponseRecorder: httptest.NewRecorder()} }
func (r *sseRecorder) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ResponseRecorder.Write(p)
}
func (r *sseRecorder) WriteString(s string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ResponseRecorder.WriteString(s)
}
func (r *sseRecorder) Flush()             { r.mu.Lock(); defer r.mu.Unlock(); r.ResponseRecorder.Flush() }
func (r *sseRecorder) BodyString() string { r.mu.Lock(); defer r.mu.Unlock(); return r.Body.String() }
