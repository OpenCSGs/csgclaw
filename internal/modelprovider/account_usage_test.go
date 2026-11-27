package modelprovider

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAccountUsageEndpoints(t *testing.T) {
	for _, tc := range []struct{ preset, baseURL, want string }{
		{"deepseek", "https://api.deepseek.com/v1", "https://api.deepseek.com/user/balance"},
		{"minimax-plan", "https://api.minimax.cn/v1", "https://www.minimax.cn/v1/token_plan/remains"},
		{"minimax-plan", "https://api.minimax.io/v1", "https://www.minimax.io/v1/token_plan/remains"},
		{"minimax", "https://api.minimax.io/v1", ""},
		{"deepseek", "https://api.deepseek.com.example.com", ""},
		{"deepseek", "http://api.deepseek.com", ""},
	} {
		if got := accountUsageEndpoint(tc.preset, tc.baseURL); got != tc.want {
			t.Errorf("%s %s: got %q, want %q", tc.preset, tc.baseURL, got, tc.want)
		}
	}
}

func TestDecodeDeepSeekUsagePreservesZero(t *testing.T) {
	usage := decodeDeepSeekUsage(json.NewDecoder(strings.NewReader(`{"balance_infos":[{"currency":"CNY","total_balance":"0.00"}]}`)))
	if usage == nil || usage.Kind != "balance" || usage.Currency != "CNY" || usage.Remaining == nil || *usage.Remaining != 0 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
}

func TestDecodeMiniMaxUsage(t *testing.T) {
	now := time.UnixMilli(1776370000000)
	for _, tc := range []struct {
		name, payload    string
		remaining, total float64
	}{
		{"percentage", `{"model_remains":[{"model_name":"MiniMax-M3","current_interval_remaining_percent":0,"current_interval_total_count":0,"end_time":1776373200000}]}`, 0, 100},
		{"legacy counts", `{"model_remains":[{"model_name":"MiniMax-M*","current_interval_usage_count":228,"current_interval_total_count":1500,"end_time":1776373200000}]}`, 228, 1500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			usage := decodeMiniMaxUsage(json.NewDecoder(strings.NewReader(tc.payload)), now)
			if usage == nil || usage.Kind != "quota" || *usage.Remaining != tc.remaining || *usage.Total != tc.total ||
				usage.ResetAt != time.UnixMilli(1776373200000).UTC().Format(time.RFC3339) {
				t.Fatalf("unexpected usage: %+v", usage)
			}
		})
	}
}

func TestDecodeMiniMaxUsageSkipsUnavailableQuota(t *testing.T) {
	now := time.UnixMilli(1776370000000)
	for _, payload := range []string{
		`{"base_resp":{"status_code":1},"model_remains":[{"model_name":"MiniMax-M3","current_interval_remaining_percent":100}]}`,
		`{"model_remains":[{"model_name":"image-01","current_interval_remaining_percent":100}]}`,
		`{"model_remains":[{"model_name":"MiniMax-M3","current_interval_remaining_percent":100,"end_time":1776360000000}]}`,
		`{"model_remains":[{"model_name":"MiniMax-M3","current_interval_remaining_percent":-1,"current_interval_total_count":0}]}`,
	} {
		if usage := decodeMiniMaxUsage(json.NewDecoder(strings.NewReader(payload)), now); usage != nil {
			t.Fatalf("unexpected usage: %+v", usage)
		}
	}
}

func TestOpenCSGAccountBalance(t *testing.T) {
	for _, tc := range []struct {
		payload   string
		want      float64
		available bool
	}{
		{`{"data":{"balance":12345}}`, 123.45, true},
		{`{"data":{"balance":0}}`, 0, true},
		{`{"data":{"balance":-120}}`, -1.2, true},
		{`{"data":{"balance":null}}`, 0, false},
		{`{"data":{}}`, 0, false},
	} {
		usage := decodeOpenCSGUsage(json.NewDecoder(strings.NewReader(tc.payload)))
		if (usage != nil) != tc.available {
			t.Fatalf("%s: unexpected usage %v", tc.payload, usage)
		}
		if usage != nil && (*usage.Remaining != tc.want || usage.Currency != "CNY") {
			t.Fatalf("%s: unexpected CNY amount %v", tc.payload, *usage.Remaining)
		}
	}
}

func TestCSGHubLiteAccountBalance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/cloud/balance" || r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected account request: %s, authorization present: %t", r.URL.Path, r.Header.Get("Authorization") != "")
		}
		_, _ = io.WriteString(w, `{"balance":250,"user_uuid":"example"}`)
	}))
	defer server.Close()

	usage := LookupCSGHubLiteBalance(t.Context(), server.URL+"/v1")
	if usage == nil || *usage.Remaining != 2.5 || usage.Currency != "CNY" {
		t.Fatalf("unexpected Lite usage: %+v", usage)
	}
}
