package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"csgclaw/internal/diagnostics"
)

func TestLLMIngressFailureEvidenceHTTP(t *testing.T) {
	for _, test := range []struct {
		name, body string
		declared   int64
		status     int
	}{{"invalid_json", "{broken", 0, 400}, {"oversize_header", "", maxLLMRequestBodyBytes + 1, 413}, {"truncated_body", `{"input":`, 64, 400}} {
		t.Run(test.name, func(t *testing.T) {
			store := diagnostics.New(t.TempDir())
			record := store.Begin("r", "s", "", "a", "turn")
			record.Running()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r = r.WithContext(diagnostics.WithRecord(r.Context(), record))
				if body, ok := readLLMRequestBody(w, r); ok && validateLLMRequestJSON(w, r, body) {
					w.WriteHeader(200)
				}
			}))
			defer server.Close()
			var status int
			if test.declared > 0 {
				connection, err := net.Dial("tcp", strings.TrimPrefix(server.URL, "http://"))
				if err != nil {
					t.Fatal(err)
				}
				defer connection.Close()
				fmt.Fprintf(connection, "POST /llm/responses HTTP/1.1\r\nHost: test\r\nContent-Length: %d\r\nConnection: close\r\n\r\n", test.declared)
				_, _ = fmt.Fprint(connection, test.body)
				_ = connection.(*net.TCPConn).CloseWrite()
				response, err := http.ReadResponse(bufio.NewReader(connection), nil)
				if err != nil {
					t.Fatal(err)
				}
				status = response.StatusCode
				response.Body.Close()
			} else {
				response, err := http.Post(server.URL+"/llm/responses", "application/json", strings.NewReader(test.body))
				if err != nil {
					t.Fatal(err)
				}
				status = response.StatusCode
				response.Body.Close()
			}
			if status != test.status {
				t.Fatalf("HTTP status=%d want %d", status, test.status)
			}
			var evidence map[string]any
			err := store.ReadContext(context.Background(), record.Snapshot().ID, func(entry diagnostics.ContextEntry) error {
				if entry.Kind == "llm.ingress" {
					return json.Unmarshal(entry.Data, &evidence)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if evidence != nil && int(evidence["observed_bytes"].(float64)) != len(test.body) {
				t.Fatalf("actual read length=%v want %d", evidence["observed_bytes"], len(test.body))
			}
			if evidence == nil {
				t.Fatal("missing failed LLM ingress evidence in exported context")
			}
			if int(evidence["http_status"].(float64)) != test.status {
				t.Fatalf("evidence=%+v", evidence)
			}
		})
	}
}

func TestFailedIngressBeforeNativeBindingKeepsExactEvidence(t *testing.T) {
	store := diagnostics.New(t.TempDir())
	record := store.Begin("r", "s", "", "a", "turn")
	record.Running()
	other := store.Begin("r", "other", "", "a", "other-turn")
	other.Running()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(diagnostics.WithNativeLookup(r.Context(), store, "a", "session", "native"))
		_, _ = readLLMRequestBody(w, r)
	}))
	defer server.Close()
	connection, err := net.Dial("tcp", strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	fmt.Fprintf(connection, "POST /llm/responses HTTP/1.1\r\nHost: test\r\nContent-Length: %d\r\nConnection: close\r\n\r\n", maxLLMRequestBodyBytes+1)
	_ = connection.(*net.TCPConn).CloseWrite()
	response, err := http.ReadResponse(bufio.NewReader(connection), nil)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 413 {
		t.Fatal(response.StatusCode)
	}
	other.RuntimeRef("session", "other-native", "")
	record.RuntimeRef("session", "native", "")
	for _, test := range []struct {
		record *diagnostics.Record
		want   int
	}{{record, 1}, {other, 0}} {
		count := 0
		if err := store.ReadContext(context.Background(), test.record.Snapshot().ID, func(entry diagnostics.ContextEntry) error {
			if entry.Kind == "llm.ingress" {
				count++
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if count != test.want {
			t.Fatalf("late native failure assigned incorrectly: count=%d want=%d", count, test.want)
		}
	}
}
