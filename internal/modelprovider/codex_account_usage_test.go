package modelprovider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCodexAccountUsageReadsMostConstrainedActiveWindow(t *testing.T) {
	now := time.Now()
	reset := now.Add(time.Hour).Unix()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer test-token" ||
			r.Header.Get("ChatGPT-Account-Id") != "test-account" {
			t.Error("unexpected account usage request")
		}
		fmt.Fprintf(w, `{"rate_limit":{"primary_window":{"used_percent":0,"reset_at":%d},"secondary_window":{"used_percent":100,"reset_at":%d}}}`, reset+60, reset)
	}))
	defer upstream.Close()

	usage := fetchCodexAccountUsage(upstream.Client(), upstream.URL, "test-token", "test-account")
	if usage == nil || *usage.Remaining != 0 || *usage.Total != 100 || usage.ResetAt != time.Unix(reset, 0).UTC().Format(time.RFC3339) {
		t.Fatalf("unexpected account usage: %+v", usage)
	}
}
